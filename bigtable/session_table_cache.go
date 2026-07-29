// Copyright 2026 Google LLC
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//      http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package bigtable

import (
	"context"
	"sync"
	"sync/atomic"
	"time"

	btpb "cloud.google.com/go/bigtable/apiv2/bigtablepb"
	"cloud.google.com/go/bigtable/internal/session"
)

// Default eviction parameters for sessionTableCache. Vars (not consts)
// so tests can tighten the sweep interval and TTL to milliseconds
// without touching the code under test.
var (
	sessionTableCacheTTL      = 1 * time.Hour
	sessionTableCacheSweepInt = 10 * time.Minute
)

// sessionTableHandle wraps a session.TableAPI with a back-reference
// to its cache and an atomically-updated last-access timestamp. It IS
// the cache entry — one type does both jobs, so ReadRow / MutateRow
// implicitly touch the entry without requiring the caller (TableShim)
// to know about the cache.
//
// Close() runs the eviction: removes the handle from the cache map
// (guarded by closeOnce so double-close from any combination of
// caller + sweeper is safe), then calls the underlying api.Close().
//
// Caveat about the underlying Close: the session.TableAPI godoc
// promises per-resource pool teardown, but sessionTable.Close in
// internal/session/table.go is a no-op today — pools are torn down
// only when session.Client.Close fires. Until the session package
// grows real per-resource teardown, this cache's eviction frees the
// bigtable-side map entry but not the session pools.
type sessionTableHandle struct {
	api            session.TableAPI
	key            string
	cache          *sessionTableCache
	lastAccessNano atomic.Int64
	closeOnce      sync.Once
}

func (h *sessionTableHandle) touch() {
	h.lastAccessNano.Store(h.cache.now().UnixNano())
}

func (h *sessionTableHandle) ReadRow(ctx context.Context, req *btpb.SessionReadRowRequest) (*btpb.SessionReadRowResponse, error) {
	h.touch()
	return h.api.ReadRow(ctx, req)
}

func (h *sessionTableHandle) MutateRow(ctx context.Context, req *btpb.SessionMutateRowRequest) (*btpb.SessionMutateRowResponse, error) {
	h.touch()
	return h.api.MutateRow(ctx, req)
}

// Close evicts the handle from its cache and Close()s the underlying
// session.TableAPI. Idempotent — safe to call from multiple paths
// (explicit caller Close, TTL sweep) concurrently. First call wins;
// subsequent calls no-op past the closeOnce guard but still return
// the underlying Close error (which itself is expected to be a
// no-op today; see the type-level caveat).
func (h *sessionTableHandle) Close() error {
	h.closeOnce.Do(func() {
		h.cache.removeEntry(h.key, h)
	})
	return h.api.Close()
}

// sessionTableCache holds per-resource sessionTableHandles with
// TTL-on-idle eviction. Zero size cap — cardinality is naturally
// bounded by the caller's Open* pattern.
type sessionTableCache struct {
	openFn        func(key string) session.TableAPI // supplied by consumer; called on cache miss
	ttl           time.Duration
	sweepInterval time.Duration

	mu      sync.Mutex
	entries map[string]*sessionTableHandle

	stopOnce sync.Once
	stop     chan struct{}
	sweeperG sync.WaitGroup

	// now is a func for tests to inject synthetic time. Defaults to
	// time.Now.
	now func() time.Time
}

// newSessionTableCache constructs a cache and starts its background
// sweeper. openFn is invoked (WITHOUT holding the cache mutex) on
// each cache miss to construct a session.TableAPI for the requested
// key; the returned api gets wrapped in a sessionTableHandle before
// being stored + returned to the caller. Production callers pass
// time.Now for time.Now; tests inject a controllable clock. Passing
// nil for now defaults to time.Now.
func newSessionTableCache(openFn func(key string) session.TableAPI, ttl, sweepInterval time.Duration, now func() time.Time) *sessionTableCache {
	if now == nil {
		now = time.Now
	}
	c := &sessionTableCache{
		openFn:        openFn,
		ttl:           ttl,
		sweepInterval: sweepInterval,
		entries:       make(map[string]*sessionTableHandle),
		stop:          make(chan struct{}),
		now:           now,
	}
	c.sweeperG.Add(1)
	go c.sweeperLoop()
	return c
}

// getOrOpen returns the cached handle for key, opening a fresh one
// via openFn on cache miss. Returns nil when the cache itself is nil
// or openFn is nil (hand-built Clients that skipped session-backend
// wiring — see Client.getOrCreateSessionTable).
func (c *sessionTableCache) getOrOpen(key string) session.TableAPI {
	if c == nil || c.openFn == nil {
		return nil
	}

	c.mu.Lock()
	if h, ok := c.entries[key]; ok {
		c.mu.Unlock()
		h.touch() // touch after releasing the mutex — atomic write, no ordering issue.
		return h
	}
	// Cache miss — open under the mutex so concurrent same-key callers
	// coalesce on the same handle. session.Client.OpenTable is cheap
	// (no dial), so brief mutex hold is fine.
	api := c.openFn(key)
	if api == nil {
		c.mu.Unlock()
		return nil
	}
	h := &sessionTableHandle{api: api, key: key, cache: c}
	h.lastAccessNano.Store(c.now().UnixNano())
	c.entries[key] = h
	c.mu.Unlock()
	return h
}

// removeEntry deletes key from the map iff the current entry is
// exactly h. Guards against the race where Close and a concurrent
// Open both raced: Close should not evict the freshly-inserted
// handle installed by the new Open.
func (c *sessionTableCache) removeEntry(key string, h *sessionTableHandle) {
	c.mu.Lock()
	if c.entries[key] == h {
		delete(c.entries, key)
	}
	c.mu.Unlock()
}

// sweeperLoop walks the cache every sweepInterval and evicts entries
// idle for > ttl. Exits when close() signals stop.
func (c *sessionTableCache) sweeperLoop() {
	defer c.sweeperG.Done()
	t := time.NewTicker(c.sweepInterval)
	defer t.Stop()
	for {
		select {
		case <-c.stop:
			return
		case <-t.C:
			c.sweepOnce()
		}
	}
}

// sweepOnce snapshots the currently-idle handles under the mutex,
// releases the mutex, then Close()s each one outside the lock.
// Handle.Close is idempotent (closeOnce) and self-removes from the
// map, so a concurrent Open on the same key mid-sweep sees either
// (a) the old handle before we snapshot — still safe, its Close will
// no-op the removeEntry because we already deleted, or (b) a fresh
// one after we deleted — protected by the identity check in
// removeEntry.
func (c *sessionTableCache) sweepOnce() {
	cutoff := c.now().Add(-c.ttl).UnixNano()

	c.mu.Lock()
	var evicted []*sessionTableHandle
	for k, h := range c.entries {
		if h.lastAccessNano.Load() < cutoff {
			evicted = append(evicted, h)
			delete(c.entries, k)
		}
	}
	c.mu.Unlock()

	for _, h := range evicted {
		_ = h.Close()
	}
}

// close stops the sweeper, waits for it to exit, then Close()s every
// remaining handle. Safe to call multiple times.
func (c *sessionTableCache) close() {
	if c == nil {
		return
	}
	c.stopOnce.Do(func() {
		close(c.stop)
	})
	c.sweeperG.Wait()

	c.mu.Lock()
	remaining := make([]*sessionTableHandle, 0, len(c.entries))
	for k, h := range c.entries {
		remaining = append(remaining, h)
		delete(c.entries, k)
	}
	c.mu.Unlock()

	for _, h := range remaining {
		_ = h.Close()
	}
}

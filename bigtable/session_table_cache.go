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
// AND calls the underlying api.Close(), both guarded by closeOnce so
// double-close from any combination of caller + sweeper is safe. The
// underlying Close error is memoized in closeErr and returned by
// every subsequent Close() call — that keeps the observable behavior
// stable for callers that treat Close as a query.
//
// Underlying Close: session.TableAPI.Close (sessionTable.Close in
// internal/session/table.go) releases the per-resource read + write
// pool entries from sessionClient.sessionPools. Cache eviction — TTL
// sweep, explicit handle Close, or cache-wide shutdown — therefore
// actually reclaims the session pools for the resource. Safety of
// per-handle teardown depends on this cache's at-most-one-handle-per-
// key invariant, which acts as an implicit refcount of size 1; see
// sessionTable.Close's doc.
type sessionTableHandle struct {
	api session.TableAPI
	// openFn is the factory captured at cache-miss insertion time
	// (session_table_cache.go's getOrOpen, driven from open.go via
	// c.sessionImpl.OpenTable / OpenAuthorizedView / OpenMaterializedView).
	// Invoked ONLY through cache.getOrOpen, which nil-guards on the
	// closed cache; do not call this field directly or the post-close
	// eviction guard is lost. Safe to hold for the process lifetime:
	// *Client.sessionImpl is set-once at NewClient and never
	// reassigned, so the closure never staless. If that ever becomes
	// reconfigurable, self-heal will stale — revisit here.
	openFn         func() session.TableAPI
	key            string
	cache          *sessionTableCache
	lastAccessNano atomic.Int64
	// evicted flips to true inside Close() BEFORE api.Close runs. The
	// happy-path cost is one atomic Load per RPC — the cache-map lookup
	// only happens when a reader observes evicted=true. Semantics:
	//
	//   - A reader whose Load observes evicted=true is guaranteed to
	//     see the flag before api.Close begins tearing down, and routes
	//     through dispatch() → resolveSuccessor to find the live handle.
	//     THIS is the fix — TableShim's cached pointer stops going
	//     stale across the many subsequent RPCs that follow eviction.
	//   - A reader whose Load observes evicted=false and then races a
	//     concurrent Close still hits the pre-existing Load-false
	//     TOCTOU on that single RPC — self-heal is a next-RPC recovery,
	//     not a same-RPC one. The Load-false-then-preempt-then-dispatch
	//     window can span arbitrary wall-clock under scheduler pressure;
	//     do not assume it's tight.
	evicted   atomic.Bool
	closeOnce sync.Once
	closeErr  error
}

func (h *sessionTableHandle) touch() {
	h.lastAccessNano.Store(h.cache.now().UnixNano())
}

// dispatch returns the live handle that should service this RPC.
//
//   - Happy path (evicted=false): returns h. One atomic Load, no cache
//     map lookup.
//   - Eviction path: routes through resolveSuccessor to install-or-find
//     the live successor in the cache. Loops so an aggressive-TTL test
//     where the fresh handle gets evicted mid-flight can still resolve
//     to a live one instead of recursing. Falls through to h when the
//     cache is closed / openFn refuses so the caller sees the honest
//     terminal-state error from h.api rather than a hang.
//
// Callers dispatch onto the returned handle and pay the touch() there,
// so ReadRow / MutateRow avoid the wasted touch on the doomed original.
func (h *sessionTableHandle) dispatch() *sessionTableHandle {
	cur := h
	for cur.evicted.Load() {
		next := cur.resolveSuccessor()
		if next == nil {
			return cur // reopen refused; let cur.api surface the terminal error
		}
		cur = next
	}
	return cur
}

// ReadRow: atomic-Load fast path, self-heal on eviction. Happy path
// adds one atomic Load vs a bare api.ReadRow; the cache-map lookup
// only fires on the recovery branch inside dispatch().
func (h *sessionTableHandle) ReadRow(ctx context.Context, req *btpb.SessionReadRowRequest) (*btpb.SessionReadRowResponse, error) {
	d := h.dispatch()
	d.touch()
	return d.api.ReadRow(ctx, req)
}

// MutateRow: same shape as ReadRow. Kept minimal so any future proxied
// method (BulkMutate, SampleRowKeys, etc.) drops in as a 3-line pass-
// through without duplicating the self-heal conditional.
func (h *sessionTableHandle) MutateRow(ctx context.Context, req *btpb.SessionMutateRowRequest) (*btpb.SessionMutateRowResponse, error) {
	d := h.dispatch()
	d.touch()
	return d.api.MutateRow(ctx, req)
}

// resolveSuccessor returns the currently-live handle registered under
// h.key, minting a fresh one via openFn on cache miss. Returns nil when
// the cache is closed or openFn refused. Called exclusively from
// dispatch() on the evicted branch.
//
// The removeEntry call covers the interleaving where Close.Do fired
// evicted.Store(true) but has not yet reached its own removeEntry (h is
// still installed in the map). Without it, our subsequent getOrOpen
// fast-path would return h itself and dispatch would loop forever. On
// the sweeper path (where sweepOnce already removed h before firing
// Close), this is a harmless no-op — removeEntry is identity-scoped, so
// it won't clobber a peer's fresh install either.
func (h *sessionTableHandle) resolveSuccessor() *sessionTableHandle {
	h.cache.removeEntry(h.key, h)
	api := h.cache.getOrOpen(h.key, h.openFn)
	if api == nil {
		return nil
	}
	// Type assertion narrows the interface back to the concrete cache
	// entry. Guaranteed non-nil-and-not-h by getOrOpen's insertion path
	// (the only construction site is inside getOrOpen's slow path,
	// which mints a fresh *sessionTableHandle) — the assertion is a
	// defensive check against a future getOrOpen contributor changing
	// what the cache stores. If that ever fires, dispatch() falls
	// through to h.api, which will surface the closed-api error.
	fresh, _ := api.(*sessionTableHandle)
	return fresh
}

// Close evicts the handle from its cache and Close()s the underlying
// session.TableAPI. Fully idempotent: subsequent calls return the
// error captured on the first call without invoking api.Close again.
// Safe to call from multiple paths (explicit caller Close, TTL sweep,
// cache-wide shutdown) concurrently.
//
// The three steps run in strict order inside closeOnce.Do:
//
//  1. evicted.Store(true) — readers observing true after this point
//     route through dispatch() → resolveSuccessor instead of taking
//     the fast path onto the doomed api.
//  2. cache.removeEntry — future getOrOpen callers stop finding this
//     handle in the map, so cache misses fall through to openFn and
//     mint a fresh live successor.
//  3. api.Close — actually tears down the underlying pool.
//
// The (1) → (3) ordering is the load-bearing invariant for self-heal.
// (2) between them just tightens the window in which a getOrOpen
// fast-path could still return this now-evicted handle.
func (h *sessionTableHandle) Close() error {
	h.closeOnce.Do(func() {
		h.evicted.Store(true)
		h.cache.removeEntry(h.key, h)
		h.closeErr = h.api.Close()
	})
	return h.closeErr
}

// sessionTableCache holds per-resource sessionTableHandles with
// TTL-on-idle eviction. Zero size cap — cardinality is naturally
// bounded by the caller's Open* pattern.
//
// The cache is opener-agnostic: getOrOpen takes an openFn per call
// so a single cache can back tables, authorized views, and
// materialized views without needing to encode the resource kind
// in the key or dispatch on a prefix. Keys are opaque strings; the
// consumer (bigtable.Client) uses the fully-qualified resource name
// (projects/P/instances/I/tables/T, etc.) so the cache key is the
// same identity Cloud Bigtable uses over the wire.
type sessionTableCache struct {
	ttl           time.Duration
	sweepInterval time.Duration

	mu      sync.Mutex
	entries map[string]*sessionTableHandle
	// closed is flipped by close() under mu. getOrOpen's slow path
	// re-checks it before inserting the freshly-opened handle so a
	// slow-path insert straddling close() can't orphan a live
	// underlying api (which would leak its per-resource session pool
	// once sessionTable.Close does real teardown).
	closed bool

	stopOnce sync.Once
	stop     chan struct{}
	sweeperG sync.WaitGroup

	// now is a func for tests to inject synthetic time. Defaults to
	// time.Now when nil is passed to newSessionTableCache.
	now func() time.Time
}

// newSessionTableCache constructs a cache and starts its background
// sweeper. Production callers pass nil for now (→ time.Now); tests
// inject a controllable clock.
func newSessionTableCache(ttl, sweepInterval time.Duration, now func() time.Time) *sessionTableCache {
	if now == nil {
		now = time.Now
	}
	c := &sessionTableCache{
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
// via openFn on cache miss. Returns nil when openFn returns nil.
//
// openFn is invoked OUTSIDE the cache mutex — safe for callers whose
// open path may itself take locks (avoids lock inversion). If two
// callers race to open the same key concurrently, both may invoke
// their openFn; the loser's api gets Close()d and the winner's is
// returned to both callers.
func (c *sessionTableCache) getOrOpen(key string, openFn func() session.TableAPI) session.TableAPI {
	if c == nil {
		return nil
	}

	// Fast path — cache hit.
	c.mu.Lock()
	if h, ok := c.entries[key]; ok {
		c.mu.Unlock()
		h.touch()
		return h
	}
	c.mu.Unlock()

	// Slow path — miss. Open outside the mutex; another goroutine may
	// win the race and insert first, in which case we discard our api
	// and return theirs.
	api := openFn()
	if api == nil {
		return nil
	}

	c.mu.Lock()
	if c.closed {
		// close() ran between the slow-path openFn and our re-lock.
		// If we inserted this handle now, nothing would ever call
		// its Close (sweeper stopped, close() already snapshotted an
		// empty map, cache-Close is stopOnce-guarded). Release the
		// freshly-opened api ourselves and return nil so the caller
		// falls back to classic (TableShim treats nil session-side
		// as classic-only).
		c.mu.Unlock()
		_ = api.Close()
		return nil
	}
	if h, ok := c.entries[key]; ok {
		c.mu.Unlock()
		_ = api.Close() // duplicate opened concurrently; release its resources
		h.touch()
		return h
	}
	h := &sessionTableHandle{api: api, openFn: openFn, key: key, cache: c}
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
// (a) the old handle before we snapshot — its Close will no-op the
// removeEntry because we already deleted, or (b) a fresh one after
// we deleted — protected by the identity check in removeEntry.
//
// Sweep is O(N) over the cache map. Fine for this cache: entry count
// is bounded by the caller's Open* pattern (typically tens, not
// millions), sweep runs every 10 minutes (default), and the cache
// isn't on the RPC hot path. If cardinality ever grows to matter, a
// time-ordered heap keyed by lastAccess would trim this to O(k) per
// sweep where k = evicted count.
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
	c.closed = true
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

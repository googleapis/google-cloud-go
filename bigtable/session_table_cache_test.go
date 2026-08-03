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

// sessionTableCache + sessionTableHandle unit tests. Split out from
// open_test.go after it crossed the 1000-line guideline. Cache-level
// fakes (noopSessionTable, unwrapSession) live in open_test.go because
// the open-path tests also consume them; cache-only helpers
// (fakeClock, newTestSessionTableCache, openNoop, openClosing,
// closeCountingTable) live here alongside their only callers.

package bigtable

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	btpb "cloud.google.com/go/bigtable/apiv2/bigtablepb"
	"cloud.google.com/go/bigtable/internal/session"
)


// ─── sessionTableCache tests ──────────────────────────────────────────

// newTestSessionTableCache builds a cache with an injectable clock.
// Sweep interval is intentionally long — tests drive sweepOnce
// directly instead of waiting on the background ticker, which is
// wall-clock and flaky under CI scheduler pressure (#20266). TTL is
// set per test.
func newTestSessionTableCache(t *testing.T, ttl time.Duration, clock *fakeClock) *sessionTableCache {
	t.Helper()
	c := newSessionTableCache(ttl, 1*time.Hour, clock.now)
	t.Cleanup(c.close)
	return c
}

// openNoop returns an openFn that constructs a *noopSessionTable
// stamped with the given key. Used by cache-internal tests that
// want to inspect which key the cache asked to open.
func openNoop(key string) func() session.TableAPI {
	return func() session.TableAPI { return &noopSessionTable{key: key} }
}

// openClosing returns an openFn that constructs a
// *closeCountingTable stamped with the given key, atomically
// incrementing counter on every Close call. Counter is atomic so
// sweeper-goroutine writes don't race the test's Load.
func openClosing(key string, counter *atomic.Int32) func() session.TableAPI {
	return func() session.TableAPI {
		return &closeCountingTable{noopSessionTable{key: key}, counter}
	}
}

// fakeClock is a monotonic clock that only advances on explicit
// advance() calls. Concurrency: single writer via advance(), many
// readers via now() — protected by an atomic.
type fakeClock struct{ nano atomic.Int64 }

func newFakeClock(start time.Time) *fakeClock {
	c := &fakeClock{}
	c.nano.Store(start.UnixNano())
	return c
}
func (c *fakeClock) now() time.Time          { return time.Unix(0, c.nano.Load()) }
func (c *fakeClock) advance(d time.Duration) { c.nano.Add(int64(d)) }

// TestSessionTableCache_HandleIsCacheEntry pins that the returned
// handle satisfies session.TableAPI, wraps the underlying api, and
// is the same value the cache holds (identity check on repeat Open).
func TestSessionTableCache_HandleIsCacheEntry(t *testing.T) {
	clock := newFakeClock(time.Unix(1_700_000_000, 0))
	c := newTestSessionTableCache(t, 1*time.Hour, clock)

	h1 := c.getOrOpen("tbl:t", openNoop("tbl:t")).(*sessionTableHandle)
	h2 := c.getOrOpen("tbl:t", openNoop("tbl:t")).(*sessionTableHandle)
	if h1 != h2 {
		t.Errorf("repeat getOrOpen on same key = distinct handles: h1=%p h2=%p", h1, h2)
	}
	if _, ok := h1.api.(*noopSessionTable); !ok {
		t.Errorf("handle.api = %T, want *noopSessionTable", h1.api)
	}
}

// TestSessionTableCache_ReadRowTouchesLastAccess pins that the
// wrapper's ReadRow updates lastAccess so a caller polling every
// ReadRow keeps the entry alive.
func TestSessionTableCache_ReadRowTouchesLastAccess(t *testing.T) {
	clock := newFakeClock(time.Unix(1_700_000_000, 0))
	c := newTestSessionTableCache(t, 1*time.Hour, clock)

	h := c.getOrOpen("tbl:t", openNoop("tbl:t")).(*sessionTableHandle)
	before := h.lastAccessNano.Load()

	clock.advance(30 * time.Minute)
	_, _ = h.ReadRow(context.Background(), &btpb.SessionReadRowRequest{Key: []byte("r")})

	after := h.lastAccessNano.Load()
	if after <= before {
		t.Errorf("ReadRow did not bump lastAccess: before=%d after=%d", before, after)
	}
}

// TestSessionTableCache_CloseEvictsAndFires pins that handle.Close
// removes the entry from the cache map AND calls the underlying
// api.Close, and that a subsequent getOrOpen mints a fresh handle
// (the closed one is not resurrected).
func TestSessionTableCache_CloseEvictsAndFires(t *testing.T) {
	clock := newFakeClock(time.Unix(1_700_000_000, 0))
	var closeCount atomic.Int32
	c := newSessionTableCache(1*time.Hour, 1*time.Second, clock.now)
	t.Cleanup(c.close)

	h1 := c.getOrOpen("tbl:t", openClosing("tbl:t", &closeCount)).(*sessionTableHandle)
	if err := h1.Close(); err != nil {
		t.Fatalf("h1.Close: %v", err)
	}
	if got := closeCount.Load(); got != 1 {
		t.Errorf("underlying Close called %d times, want 1", got)
	}
	// Map should no longer contain the key.
	c.mu.Lock()
	_, still := c.entries["tbl:t"]
	c.mu.Unlock()
	if still {
		t.Error("entry still present after handle.Close()")
	}
	// Second Open mints a fresh handle.
	h2 := c.getOrOpen("tbl:t", openClosing("tbl:t", &closeCount)).(*sessionTableHandle)
	if h2 == h1 {
		t.Error("getOrOpen after Close returned the evicted handle")
	}
	// Double-Close on h1 is fully idempotent — closeOnce guards both
	// the map removal AND the underlying api.Close call, so the
	// counter stays at 1 even after a second h1.Close().
	if err := h1.Close(); err != nil {
		t.Errorf("h1.Close (idempotent) err = %v, want nil", err)
	}
	if got := closeCount.Load(); got != 1 {
		t.Errorf("underlying Close called %d times after double-Close, want 1 (Close is fully idempotent)", got)
	}
}

// TestSessionTableCache_TTLSweepEvictsIdle pins that a sweep evicts
// entries whose lastAccess is older than TTL, and calls the
// underlying Close on eviction.
//
// Drives sweepOnce directly instead of polling on the background
// ticker: the ticker fires at a real-wall-clock cadence and, under CI
// scheduler pressure, may not run within the assertion's deadline
// window even at a 1ms interval — see #20266 for the flake pattern.
// Same-package access to sweepOnce lets us exercise the sweep logic
// deterministically without any wall-clock dependency.
func TestSessionTableCache_TTLSweepEvictsIdle(t *testing.T) {
	clock := newFakeClock(time.Unix(1_700_000_000, 0))
	var closeCount atomic.Int32
	// Use a long sweepInterval so the background ticker never races the
	// direct sweepOnce call below — the test asserts sweep behavior,
	// not scheduler timing.
	c := newSessionTableCache(1*time.Hour, 1*time.Hour, clock.now)
	t.Cleanup(c.close)

	// Open two handles, touch neither.
	_ = c.getOrOpen("tbl:a", openClosing("tbl:a", &closeCount))
	_ = c.getOrOpen("tbl:b", openClosing("tbl:b", &closeCount))

	// Advance past TTL and drive a sweep synchronously.
	clock.advance(2 * time.Hour)
	c.sweepOnce()

	c.mu.Lock()
	n := len(c.entries)
	c.mu.Unlock()
	if n != 0 {
		t.Errorf("entries remaining after TTL sweep = %d, want 0", n)
	}
	if got := closeCount.Load(); got != 2 {
		t.Errorf("underlying Close called %d times on TTL evict, want 2", got)
	}
}

// TestSessionTableCache_TouchDefersEviction pins that a ReadRow
// touch resets the idle timer — an entry touched every half-TTL
// stays alive indefinitely.
func TestSessionTableCache_TouchDefersEviction(t *testing.T) {
	clock := newFakeClock(time.Unix(1_700_000_000, 0))
	c := newTestSessionTableCache(t, 1*time.Hour, clock)

	h := c.getOrOpen("tbl:t", openNoop("tbl:t")).(*sessionTableHandle)
	// Every half-TTL, touch and step past a full TTL from the LAST
	// touch. Each touch resets the clock reference so eviction never
	// triggers.
	for i := 0; i < 4; i++ {
		clock.advance(30 * time.Minute)
		_, _ = h.ReadRow(context.Background(), &btpb.SessionReadRowRequest{Key: []byte("r")})
	}
	// Drive a sweep directly; touch-driven lastAccess should keep the
	// entry alive despite the clock advance.
	c.sweepOnce()
	c.mu.Lock()
	_, present := c.entries["tbl:t"]
	c.mu.Unlock()
	if !present {
		t.Error("touched entry got evicted; touch is not deferring eviction")
	}
}

// TestSessionTableCache_CloseEvictsAll pins that closing the cache
// itself stops the sweeper and closes every remaining entry.
func TestSessionTableCache_CloseEvictsAll(t *testing.T) {
	clock := newFakeClock(time.Unix(1_700_000_000, 0))
	var closeCount atomic.Int32
	c := newSessionTableCache(1*time.Hour, 1*time.Hour, clock.now)

	_ = c.getOrOpen("tbl:a", openClosing("tbl:a", &closeCount))
	_ = c.getOrOpen("tbl:b", openClosing("tbl:b", &closeCount))
	_ = c.getOrOpen("mv:v", openClosing("mv:v", &closeCount))

	c.close()

	if got := closeCount.Load(); got != 3 {
		t.Errorf("close(): underlying Close called %d times, want 3", got)
	}
	// close() is idempotent.
	c.close()
}

// TestSessionTableCache_ClosedGate_SlowPathInsertNoLeak reproduces
// audit finding #5: without the closed-gate in getOrOpen's slow path,
// a caller whose openFn straddles cache.close() would leak the
// freshly-opened api (installed into a cache the sweeper has already
// stopped clearing). This test forces that interleaving via a
// synchronization channel on the openFn.
//
// Shape: caller A begins getOrOpen("k") on an empty cache. Fast-path
// misses. Slow-path calls openFn. openFn blocks until we signal it to
// return. Meanwhile close() runs on the cache (flips closed, walks the
// empty map). Then openFn returns. getOrOpen re-locks, sees closed,
// releases the fresh api itself, and returns nil. Underlying api's
// Close counter must reach 1.
func TestSessionTableCache_ClosedGate_SlowPathInsertNoLeak(t *testing.T) {
	clock := newFakeClock(time.Unix(1_700_000_000, 0))
	var closeCount atomic.Int32
	c := newSessionTableCache(1*time.Hour, 1*time.Hour, clock.now)

	release := make(chan struct{})
	opened := make(chan struct{})
	openFn := func() session.TableAPI {
		close(opened)
		<-release
		return &closeCountingTable{noopSessionTable{key: "k"}, &closeCount}
	}

	var result session.TableAPI
	done := make(chan struct{})
	go func() {
		result = c.getOrOpen("k", openFn)
		close(done)
	}()

	// Wait until openFn is in-flight, THEN close the cache. This is the
	// race window: openFn returns AFTER close() completed.
	<-opened
	c.close()
	close(release)
	<-done

	if result != nil {
		t.Errorf("getOrOpen returned non-nil after cache close: %v (want nil so TableShim falls back to classic)", result)
	}
	if got := closeCount.Load(); got != 1 {
		t.Errorf("underlying api Close called %d times, want 1 (slow-path insert must release its api when cache is closed)", got)
	}
	// Cache map must be empty — the fresh api MUST NOT have been
	// installed into an already-closed cache.
	c.mu.Lock()
	n := len(c.entries)
	c.mu.Unlock()
	if n != 0 {
		t.Errorf("cache.entries has %d entries after close-race; want 0 (fresh handle must not be installed)", n)
	}
}

// closeCountingTable is a noopSessionTable that atomically
// increments a counter on Close so tests can assert eviction actually
// called Close from any goroutine (including the cache sweeper).
type closeCountingTable struct {
	noopSessionTable
	counter *atomic.Int32
}

func (c *closeCountingTable) Close() error {
	c.counter.Add(1)
	return nil
}

// TestSessionTableHandle_EvictedSelfHeals pins the Design C invariant
// that makes bug #6 fixable without TableShim changes: a
// *sessionTableHandle whose Close() has run (sweeper eviction, or
// direct handle.Close from any owner) transparently re-opens via the
// cache on the next RPC. The caller — TableShim or any other holder —
// keeps its *sessionTableHandle pointer forever; the wrapper does the
// self-heal.
//
// Verifies:
//   - After handle.Close(), evicted is true.
//   - Subsequent ReadRow / MutateRow succeed by delegating to a
//     freshly-minted successor handle.
//   - openFn IS invoked a second time (fresh sessionTable is minted).
//   - Handle identity is stable — the same *sessionTableHandle
//     transparently dispatches to whichever underlying api is live.
func TestSessionTableHandle_EvictedSelfHeals(t *testing.T) {
	clock := newFakeClock(time.Unix(1_700_000_000, 0))
	c := newSessionTableCache(1*time.Hour, 1*time.Hour, clock.now)
	t.Cleanup(c.close)

	var opens atomic.Int32
	openFn := func() session.TableAPI {
		opens.Add(1)
		return &noopSessionTable{key: "tbl:t"}
	}

	// Install h1 via first getOrOpen.
	h1 := c.getOrOpen("tbl:t", openFn).(*sessionTableHandle)
	if opens.Load() != 1 {
		t.Fatalf("openFn calls after first install = %d, want 1", opens.Load())
	}

	// Sweeper-equivalent: evict h1 by calling its Close directly.
	if err := h1.Close(); err != nil {
		t.Fatalf("h1.Close: %v", err)
	}
	if !h1.evicted.Load() {
		t.Fatal("h1.evicted = false after Close(); want true (self-heal fast-path check would miss)")
	}

	// Next ReadRow on the (evicted) h1 must transparently succeed via
	// a fresh handle. openFn should fire a second time; a new handle
	// should now be installed in the cache.
	if _, err := h1.ReadRow(context.Background(), &btpb.SessionReadRowRequest{Key: []byte("r")}); err != nil {
		t.Fatalf("post-evict h1.ReadRow: %v (self-heal failed)", err)
	}
	if got := opens.Load(); got != 2 {
		t.Errorf("openFn calls after post-evict ReadRow = %d, want 2 (self-heal must re-invoke openFn)", got)
	}
	c.mu.Lock()
	installed, ok := c.entries["tbl:t"]
	c.mu.Unlock()
	if !ok {
		t.Fatal("cache.entries missing 'tbl:t' after self-heal; getOrOpen must have installed the successor")
	}
	if installed == h1 {
		t.Error("cache.entries['tbl:t'] == h1 (evicted); self-heal must have installed a distinct successor")
	}

	// MutateRow must also self-heal via the same successor (or an
	// equivalent one if the cache TTL evicted between calls; not
	// possible here with 1h TTL + injected clock).
	if _, err := h1.MutateRow(context.Background(), &btpb.SessionMutateRowRequest{Key: []byte("r")}); err != nil {
		t.Fatalf("post-evict h1.MutateRow: %v", err)
	}
	if got := opens.Load(); got != 2 {
		t.Errorf("openFn calls after MutateRow = %d, want 2 (successor should be reused on the same key)", got)
	}
}

// TestSessionTableHandle_EvictedReopenFailsGracefully pins the
// terminal branch: when the cache is closed, dispatch() must NOT
// loop — it falls through to the doomed handle so h.api.ReadRow can
// surface the honest terminal error.
//
// Asserts two invariants:
//   - ReadRow returns within a bounded time. If dispatch()'s loop
//     escape breaks, this hangs, and the timeout catches it long
//     before the default go-test deadline (which reads as an
//     unrelated CI hang).
//   - The cache stays empty after the call. openFn may have fired
//     one extra time (getOrOpen's slow path invokes openFn OUTSIDE
//     the mutex — the closed-cache guard fires AFTER, releasing the
//     wasted api) but the freshly-opened api MUST NOT install into
//     the closed cache.
func TestSessionTableHandle_EvictedReopenFailsGracefully(t *testing.T) {
	clock := newFakeClock(time.Unix(1_700_000_000, 0))
	c := newSessionTableCache(1*time.Hour, 1*time.Hour, clock.now)

	openFn := func() session.TableAPI { return &noopSessionTable{key: "tbl:t"} }
	h := c.getOrOpen("tbl:t", openFn).(*sessionTableHandle)

	// Close the cache itself so getOrOpen refuses to install new entries.
	c.close()
	if !h.evicted.Load() {
		t.Fatal("cache.close() did not propagate to h.evicted; expected true")
	}

	// Bounded wait — hang detection.
	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Second)
	defer cancel()
	done := make(chan struct{})
	go func() {
		_, _ = h.ReadRow(ctx, &btpb.SessionReadRowRequest{Key: []byte("r")})
		close(done)
	}()
	select {
	case <-done:
	case <-ctx.Done():
		t.Fatal("post-cache-close ReadRow did not return within 1s — dispatch() likely looping")
	}

	// Cache must remain empty — the closed-cache guard inside
	// getOrOpen must have refused to install the freshly-opened api,
	// releasing it via api.Close instead. If a successor slipped in,
	// it would leak (sweeper has stopped, cache.close is stopOnce-
	// guarded, nothing would ever tear it down).
	c.mu.Lock()
	n := len(c.entries)
	c.mu.Unlock()
	if n != 0 {
		t.Errorf("cache.entries has %d entries after ReadRow on closed cache; want 0", n)
	}
}

// TestSessionTableHandle_SweeperEvictionSelfHeals pins the actual bug
// #6 production trigger: the TTL sweeper (not an explicit Close) evicts
// the handle while TableShim still holds it. Direct-Close tests above
// don't exercise the sweeper path; this one does.
func TestSessionTableHandle_SweeperEvictionSelfHeals(t *testing.T) {
	clock := newFakeClock(time.Unix(1_700_000_000, 0))
	c := newSessionTableCache(1*time.Hour, 1*time.Hour, clock.now)
	t.Cleanup(c.close)

	var opens atomic.Int32
	openFn := func() session.TableAPI {
		opens.Add(1)
		return &noopSessionTable{key: "tbl:t"}
	}
	h := c.getOrOpen("tbl:t", openFn).(*sessionTableHandle)

	// Advance past TTL and run one sweep — this is the exact production
	// path that poisoned TableShim's held pointer before Design C.
	clock.advance(2 * time.Hour)
	c.sweepOnce()

	if !h.evicted.Load() {
		t.Fatal("sweeper did not flip h.evicted; sweepOnce must Close swept handles")
	}

	// The critical assertion: TableShim (represented here by our held h
	// pointer) can keep using the same *sessionTableHandle after the
	// sweeper evicted its underlying pool.
	if _, err := h.ReadRow(context.Background(), &btpb.SessionReadRowRequest{Key: []byte("r")}); err != nil {
		t.Fatalf("post-sweep h.ReadRow: %v (self-heal from sweeper path failed)", err)
	}
	if got := opens.Load(); got != 2 {
		t.Errorf("openFn calls after sweeper-eviction ReadRow = %d, want 2 (self-heal must mint a successor)", got)
	}
}

// TestSessionTableHandle_ConcurrentEvictAndReadRace runs under -race:
// one goroutine spins Close/getOrOpen cycles while another spins
// ReadRow through the same held handle. dispatch() must not race, and
// the returned errors (or successes) must be well-defined — no panic,
// no data race, no infinite loop.
func TestSessionTableHandle_ConcurrentEvictAndReadRace(t *testing.T) {
	clock := newFakeClock(time.Unix(1_700_000_000, 0))
	c := newSessionTableCache(1*time.Hour, 1*time.Hour, clock.now)
	t.Cleanup(c.close)

	var opens atomic.Int32
	openFn := func() session.TableAPI {
		opens.Add(1)
		return &noopSessionTable{key: "tbl:t"}
	}
	h := c.getOrOpen("tbl:t", openFn).(*sessionTableHandle)

	stop := make(chan struct{})
	var wg sync.WaitGroup

	// Evictor: repeatedly Close current handle + reopen to install a
	// fresh successor. Uses the same key so the reader keeps hitting
	// dispatch's evicted branch on the stale h.
	wg.Add(1)
	go func() {
		defer wg.Done()
		for {
			select {
			case <-stop:
				return
			default:
			}
			if cur, ok := c.getOrOpen("tbl:t", openFn).(*sessionTableHandle); ok {
				_ = cur.Close()
			}
		}
	}()

	// Reader: spins ReadRow through the original h pointer. Every call
	// after the first evictor iteration goes through dispatch's evicted
	// branch and resolveSuccessor.
	wg.Add(1)
	go func() {
		defer wg.Done()
		for i := 0; i < 500; i++ {
			select {
			case <-stop:
				return
			default:
			}
			if _, err := h.ReadRow(context.Background(), &btpb.SessionReadRowRequest{Key: []byte("r")}); err != nil {
				// noopSessionTable never returns an error, so any err
				// here is a bug (nil deref, closed pool leaking through).
				t.Errorf("racing ReadRow returned err = %v", err)
				return
			}
		}
	}()

	// Let both goroutines run for a bit, then quiesce.
	time.Sleep(50 * time.Millisecond)
	close(stop)
	wg.Wait()
}

// TestSessionTableHandle_EvictionStormConvergesOnSingleInstalled pins
// the invariant getOrOpen actually guarantees: N concurrent post-
// eviction RPCs may each invoke openFn (getOrOpen is NOT single-flight
// on openFn per line 205-207's doc), but the cache converges on
// EXACTLY ONE installed successor, and every loser api gets Close()d.
// This is the leak-avoidance invariant — without it, a storm would
// leak N-1 session pools under a single key.
//
// (Making openFn genuinely single-flight would be a real perf win but
// is a separate feature — this test would need to tighten if that
// ever lands.)
func TestSessionTableHandle_EvictionStormConvergesOnSingleInstalled(t *testing.T) {
	clock := newFakeClock(time.Unix(1_700_000_000, 0))
	c := newSessionTableCache(1*time.Hour, 1*time.Hour, clock.now)
	t.Cleanup(c.close)

	var opens, closes atomic.Int32
	// Slow openFn so concurrent callers actually pile up in the slow
	// path and produce a real storm.
	openFn := func() session.TableAPI {
		opens.Add(1)
		time.Sleep(5 * time.Millisecond)
		return &closeCountingTable{noopSessionTable{key: "tbl:t"}, &closes}
	}
	h := c.getOrOpen("tbl:t", openFn).(*sessionTableHandle)
	if err := h.Close(); err != nil {
		t.Fatalf("h.Close: %v", err)
	}

	// Launch N concurrent ReadRows on the evicted h.
	const N = 32
	var wg sync.WaitGroup
	wg.Add(N)
	for i := 0; i < N; i++ {
		go func() {
			defer wg.Done()
			_, _ = h.ReadRow(context.Background(), &btpb.SessionReadRowRequest{Key: []byte("r")})
		}()
	}
	wg.Wait()

	// Cache must hold exactly one successor entry for the key — no
	// duplicate installs.
	c.mu.Lock()
	n := len(c.entries)
	c.mu.Unlock()
	if n != 1 {
		t.Errorf("cache.entries len = %d after storm, want 1 (one installed successor)", n)
	}

	// Every openFn call except the winner must have had its api closed
	// by getOrOpen's loser-close path. The initial h was also closed
	// (the h.Close() above), so total closes == opens - 1 (one live
	// installed successor).
	opensLoaded := opens.Load()
	closesLoaded := closes.Load()
	if opensLoaded < 2 {
		t.Fatalf("opens = %d, want >= 2 (initial + at least one successor)", opensLoaded)
	}
	if wantCloses := opensLoaded - 1; closesLoaded != wantCloses {
		t.Errorf("closes = %d, want %d (opens=%d, one installed live)", closesLoaded, wantCloses, opensLoaded)
	}
}

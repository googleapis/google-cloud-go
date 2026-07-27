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

package internal

import (
	"errors"
	"fmt"
	"sync"
	"testing"
	"time"
)

// --- close-reason ledger ---------------------------------------------------

func TestBumpCloseReason_Increments(t *testing.T) {
	p := newTestPool(t, 1, 10)
	p.bumpCloseReason("GoAway")
	p.bumpCloseReason("GoAway")
	p.bumpCloseReason("MissedHeartbeat")
	got := p.snapshotCloseReasons()
	if got["GoAway"] != 2 {
		t.Errorf("GoAway count = %d, want 2", got["GoAway"])
	}
	if got["MissedHeartbeat"] != 1 {
		t.Errorf("MissedHeartbeat count = %d, want 1", got["MissedHeartbeat"])
	}
}

func TestBumpCloseReason_EmptyLabelBecomesUnspecified(t *testing.T) {
	p := newTestPool(t, 1, 10)
	p.bumpCloseReason("")
	if got := p.snapshotCloseReasons()["Unspecified"]; got != 1 {
		t.Errorf("Unspecified count = %d, want 1 (empty label must fold here)", got)
	}
}

func TestRecordSessionClose_OnceFlag(t *testing.T) {
	p := newTestPool(t, 1, 10)
	sh := injectActiveSession(t, p, "s1", time.Now())
	s := sh.session

	// Multiple calls dedupe via s.poolCloseRecorded.
	p.recordSessionClose(s, "First")
	p.recordSessionClose(s, "SecondShouldNotFire")
	p.recordSessionClose(s, "ThirdShouldNotFire")

	if got := p.m.sessionsClosed.Load(); got != 1 {
		t.Errorf("sessionsClosed = %d, want 1 (once-flag violated)", got)
	}
	reasons := p.snapshotCloseReasons()
	if reasons["First"] != 1 {
		t.Errorf("First count = %d, want 1", reasons["First"])
	}
	if _, ok := reasons["SecondShouldNotFire"]; ok {
		t.Errorf("second-call fallback leaked into ledger: %+v", reasons)
	}
}

func TestRecordSessionClose_UsesSessionReasonOverFallback(t *testing.T) {
	p := newTestPool(t, 1, 10)
	sh := injectActiveSession(t, p, "s1", time.Now())
	sh.session.setCloseReason("GoAway")

	p.recordSessionClose(sh.session, "IgnoredFallback")
	if got := p.snapshotCloseReasons()["GoAway"]; got != 1 {
		t.Errorf("GoAway count = %d, want 1 (session's own reason should win)", got)
	}
	if _, ok := p.snapshotCloseReasons()["IgnoredFallback"]; ok {
		t.Errorf("fallback leaked despite session reason set: %+v", p.snapshotCloseReasons())
	}
}

func TestRecordSessionClose_NilSessionIsNoOp(t *testing.T) {
	p := newTestPool(t, 1, 10)
	p.recordSessionClose(nil, "Anything")
	if got := p.m.sessionsClosed.Load(); got != 0 {
		t.Errorf("sessionsClosed = %d, want 0 for nil session", got)
	}
}

func TestBumpStartingClose_UsesFailedToStart(t *testing.T) {
	p := newTestPool(t, 1, 10)
	sh := injectActiveSession(t, p, "s1", time.Now())
	p.bumpStartingClose(sh.session)
	if got := p.snapshotCloseReasons()["FailedToStart"]; got != 1 {
		t.Errorf("FailedToStart count = %d, want 1", got)
	}
}

// --- OnActive / OnClose ---------------------------------------------------

func TestOnActive_DuplicateSessionSkipped(t *testing.T) {
	p := newTestPool(t, 1, 10)
	sh := injectActiveSession(t, p, "s1", time.Now())
	before := p.sl.ReadyCount()
	// Firing onActive on a handle already activated must not
	// double-register (sh.activated CAS gate catches it).
	p.onActive(sh)
	if got := p.sl.ReadyCount(); got != before {
		t.Errorf("sl.ReadyCount = %d, want %d (duplicate register)", got, before)
	}
}

func TestOnActive_SignalsFree(t *testing.T) {
	p := newTestPool(t, 1, 10)
	// Park a waiter directly on the queue. onActive should wake it.
	w := &waiter{ready: make(chan struct{})}
	p.waitersMu.Lock()
	w.elem = p.waiters.PushBack(w)
	p.waitersMu.Unlock()

	// Craft a fresh session + handle and fire onActive — must wake the
	// parked waiter.
	sh := newSessionHandle(nil, time.Now())
	stream := newFakeStream()
	s := NewSession("s-fresh", stream, SessionHooks{
		OnStart:  p.onStart,
		OnActive: func(_ *Session) { p.onActive(sh) },
		OnClose:  func(_ *Session, err error) { p.onClose(sh, err) },
	}, SessionTypeTable)
	s.state.Store(int32(StateReady))
	sh.session = s
	p.onActive(sh)
	select {
	case <-w.ready:
		// expected
	case <-time.After(100 * time.Millisecond):
		t.Error("onActive did not wake the parked waiter within 100ms")
	}
}

func TestOnClose_StartingSessionBumpsFailedToStart(t *testing.T) {
	p := newTestPool(t, 1, 10)
	// Handle in startingSessions but never activated.
	stream := newFakeStream()
	sh := newSessionHandle(nil, time.Time{})
	s := NewSession("s-starting", stream, SessionHooks{
		OnClose: func(_ *Session, err error) { p.onClose(sh, err) },
	}, SessionTypeTable)
	sh.session = s
	p.mu.Lock()
	p.startingSessions[sh] = struct{}{}
	p.mu.Unlock()

	p.onClose(sh, nil)

	p.mu.Lock()
	_, stillThere := p.startingSessions[sh]
	p.mu.Unlock()
	if stillThere {
		t.Error("handle still in startingSessions after onClose")
	}
	if got := p.snapshotCloseReasons()["FailedToStart"]; got != 1 {
		t.Errorf("FailedToStart count = %d, want 1", got)
	}
}

// --- OnClosing ------------------------------------------------------------

func TestOnClosing_DropsFromReadyCountAndRecordsLifetime(t *testing.T) {
	p := newTestPool(t, 1, 10)
	sh := injectActiveSession(t, p, "s1", time.Now().Add(-time.Second))
	if p.sl.ReadyCount() != 1 {
		t.Fatalf("ReadyCount before onClosing = %d, want 1", p.sl.ReadyCount())
	}

	p.onClosing(sh)

	if got := p.sl.ReadyCount(); got != 0 {
		t.Errorf("ReadyCount after onClosing = %d, want 0 (dying sessions must free the budget)", got)
	}
	if got := len(p.snapshotLifetimes()); got != 1 {
		t.Errorf("lifetimes len = %d, want 1 (createdAt was set → recordLifetime should fire)", got)
	}
}

func TestOnClosing_StartingSessionIsNoOp(t *testing.T) {
	p := newTestPool(t, 1, 10)
	stream := newFakeStream()
	sh := newSessionHandle(nil, time.Time{})
	s := NewSession("s-starting", stream, SessionHooks{
		OnClosing: func(_ *Session) { p.onClosing(sh) },
	}, SessionTypeTable)
	sh.session = s
	p.mu.Lock()
	p.startingSessions[sh] = struct{}{}
	p.mu.Unlock()

	p.onClosing(sh)

	// Starting-branch short-circuits — closingRecorded stays unset so a
	// later onClosing (once promoted) could still run.
	if sh.closingRecorded.Load() {
		t.Error("sh.closingRecorded set for starting-only handle; starting-branch must not consume the flag")
	}
}

func TestOnClosing_ThenOnClose_UsesHandleDirectly(t *testing.T) {
	p := newTestPool(t, 1, 10)
	sh := injectActiveSession(t, p, "s1", time.Now().Add(-time.Second))

	p.onClosing(sh)
	p.onClose(sh, nil)

	if got := p.m.sessionsClosed.Load(); got != 1 {
		t.Errorf("sessionsClosed = %d, want 1", got)
	}
}

// TestPoolClose_DrainsParkedWaitersWithSentinel pins Close's contract
// to unblock every FIFO-parked CheckoutSession caller with
// ErrPoolClosed. Without the drain in Phase-1, long-poll callers whose
// ctx is context.Background hang past Close forever — the in-flight
// vRPC teardown fires signalFree for only the N in-flight calls, not
// the M > N parked waiters typical under saturation.
//
// Enqueue directly on the waiter queue (mirroring
// TestConsecutiveFailures_TripWakesParkedWaitersWithSentinel) so the
// test exercises the wake/err contract without racing auto-scaling.
func TestPoolClose_DrainsParkedWaitersWithSentinel(t *testing.T) {
	p := newTestPool(t, 1, 10)

	const nWaiters = 3
	results := make(chan error, nWaiters)
	var wg sync.WaitGroup
	for i := 0; i < nWaiters; i++ {
		w := &waiter{ready: make(chan struct{})}
		p.waitersMu.Lock()
		w.elem = p.waiters.PushBack(w)
		p.waitersMu.Unlock()
		p.waitersCount.Add(1)

		wg.Add(1)
		go func(w *waiter) {
			defer wg.Done()
			<-w.ready
			p.waitersCount.Add(-1)
			results <- w.err
		}(w)
	}

	done := make(chan error, 1)
	go func() { done <- p.Close() }()

	// Waiters must wake within a bounded window driven by Phase-1
	// drainWaitersWithErr — NOT by the in-flight-vRPC signalFree wave
	// (there are no in-flight vRPCs in this test).
	waitCh := make(chan struct{})
	go func() { wg.Wait(); close(waitCh) }()
	select {
	case <-waitCh:
	case <-time.After(5 * time.Second):
		t.Fatal("parked waiters not woken by Close within 5s — Close doesn't drain the FIFO queue")
	}

	select {
	case err := <-done:
		if err != nil {
			t.Errorf("Close err = %v", err)
		}
	case <-time.After(30 * time.Second):
		t.Fatal("Close did not return within 30s")
	}

	close(results)
	for err := range results {
		if !errors.Is(err, ErrPoolClosed) {
			t.Errorf("waiter err = %v, want ErrPoolClosed", err)
		}
	}

	// After the drain, waiter queue must be empty.
	p.waitersMu.Lock()
	remaining := p.waiters.Len()
	p.waitersMu.Unlock()
	if remaining != 0 {
		t.Errorf("waiter queue after Close has %d entries, want 0", remaining)
	}
}

func TestPoolClose_RecordsLifetimeOncePerSession(t *testing.T) {
	// Regression: Phase-1 of Pool.Close records lifetime up-front for every
	// snapshotted handle. Phase-2 then fires s.Close per session, which
	// drives notifyClosing → p.onClosing → recordLifetime again unless
	// Phase-1 has tripped sh.closingRecorded. Without the flag, the
	// lifetimes ring double-counts every session torn down during pool
	// teardown.
	p := newTestPool(t, 1, 10)
	const n = 3
	for i := 0; i < n; i++ {
		injectActiveSession(t, p, fmt.Sprintf("s%d", i), time.Now().Add(-time.Second))
	}

	if err := p.Close(); err != nil {
		t.Fatalf("Close returned err = %v", err)
	}

	// Pool.Close waits on wg for all Phase-2 s.Close goroutines, and s.Close
	// fires notifyClosing synchronously → onClosing has run by return.
	if got := len(p.snapshotLifetimes()); got != n {
		t.Errorf("lifetimes len = %d, want %d (one per session, not double-counted)", got, n)
	}
}

// TestPoolClose_OnClosingBeforePhase1_NoDoubleLifetime pins the reverse
// race ordering to TestPoolClose_RecordsLifetimeOncePerSession: a
// notifyClosing (GOAWAY / heartbeat trip) that fires BEFORE Pool.Close's
// Phase-1 loop must still leave exactly one lifetime recorded.
// Previously Phase-1 did `sh.closingRecorded.Store(true)` unconditionally,
// so the racing onClosing (which already recorded lifetime under its own
// CAS) followed by Phase-1's Store meant Phase-1 also called
// recordLifetime — double-count. Fix uses CompareAndSwap in Phase-1 too.
func TestPoolClose_OnClosingBeforePhase1_NoDoubleLifetime(t *testing.T) {
	p := newTestPool(t, 1, 10)
	sh := injectActiveSession(t, p, "raced", time.Now().Add(-time.Second))

	// Simulate the race: onClosing wins before Pool.Close's Phase-1
	// snapshot loop reaches this handle.
	p.onClosing(sh)
	if got := len(p.snapshotLifetimes()); got != 1 {
		t.Fatalf("precondition: onClosing should have recorded 1 lifetime, got %d", got)
	}

	if err := p.Close(); err != nil {
		t.Fatalf("Close returned err = %v", err)
	}

	if got := len(p.snapshotLifetimes()); got != 1 {
		t.Errorf("lifetimes len after Close = %d, want 1 (Phase-1 must CAS-guard recordLifetime; racing onClosing already recorded it)", got)
	}
}

func TestOnClosing_FiresBeforeOnClose_ViaSessionClose(t *testing.T) {
	// End-to-end: driving Session.ForceClose() must fire OnClosing
	// (dropping the session from sl.ReadyCount) and the eventual
	// notifyClosed must fire OnClose.
	p := newTestPool(t, 1, 10)
	sh := injectActiveSession(t, p, "s1", time.Now().Add(-time.Second))

	// Session.Close transitions to Closing and eventually the fakeStream
	// close hooks flush through to notifyClosed. Use ForceClose so we
	// don't wait on the server-side CloseSession round-trip.
	sh.session.ForceClose(nil)

	// Wait briefly for the async close path to complete.
	deadline := time.Now().Add(500 * time.Millisecond)
	for time.Now().Before(deadline) {
		if p.sl.ReadyCount() == 0 && p.m.sessionsClosed.Load() == 1 {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}

	t.Errorf("after ForceClose: sl.ReadyCount=%d (want 0), sessionsClosed=%d (want 1)",
		p.sl.ReadyCount(), p.m.sessionsClosed.Load())
}

// --- OnClose --------------------------------------------------------------

func TestOnClose_IdxNotFoundStillRecords(t *testing.T) {
	p := newTestPool(t, 1, 10)
	// A handle neither in sl nor startingSessions — the "already
	// proactively removed" path (onClosing already ran, or a bare-metal
	// caller invoked onClose directly). recordSessionClose still fires
	// so the ledger reflects reality.
	stream := newFakeStream()
	sh := newSessionHandle(nil, time.Time{})
	s := NewSession("s-ghost", stream, SessionHooks{}, SessionTypeTable)
	sh.session = s

	p.onClose(sh, nil)
	if got := p.m.sessionsClosed.Load(); got != 1 {
		t.Errorf("sessionsClosed = %d, want 1", got)
	}
}

// --- sweepStuckSessions ---------------------------------------------------

func TestSweepStuckSessions_LeavesFreshAlone(t *testing.T) {
	p := newTestPool(t, 1, 10)
	sh := injectActiveSession(t, p, "s1", time.Now())
	// Transition into WaitServerClose but keep the last-state-change
	// stamp fresh — should be skipped.
	sh.session.state.Store(int32(StateWaitServerClose))
	sh.session.lastStateChangeNano.Store(time.Now().UnixNano())

	p.sweepStuckSessions()

	if State(sh.session.state.Load()) != StateWaitServerClose {
		t.Errorf("fresh WaitServerClose session was swept; state = %v", State(sh.session.state.Load()))
	}
}

func TestSweepStuckSessions_ForceClosesStale(t *testing.T) {
	p := newTestPool(t, 1, 10)
	sh := injectActiveSession(t, p, "s1", time.Now())
	// Fake a session that's been in WaitServerClose past the grace window.
	sh.session.state.Store(int32(StateWaitServerClose))
	stale := time.Now().Add(-waitServerCloseGrace - time.Second).UnixNano()
	sh.session.lastStateChangeNano.Store(stale)

	p.sweepStuckSessions()

	if State(sh.session.state.Load()) != StateClosed {
		t.Errorf("stale WaitServerClose session state = %v, want Closed (ForceClose should have fired)",
			State(sh.session.state.Load()))
	}
}

// --- OnStart no-op --------------------------------------------------------

func TestOnStart_NoOp(t *testing.T) {
	p := newTestPool(t, 1, 10)
	// OnStart takes a ctx and does nothing. Just verify it doesn't
	// panic and doesn't touch any counters.
	before := p.m.sessionsOpened.Load()
	p.onStart(nil)
	if got := p.m.sessionsOpened.Load(); got != before {
		t.Errorf("OnStart mutated sessionsOpened: %d → %d", before, got)
	}
}

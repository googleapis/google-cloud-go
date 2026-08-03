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
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	spb "cloud.google.com/go/bigtable/apiv2/bigtablepb"
)

// newStubStreamFactory returns a streamFactory that hands out fresh
// fakeStreams and a closeAll function that closes every stream the
// factory produced. Tests that use SessionPoolImpl.CheckoutSession end
// up triggering Tick → createSession → Session.Start, which
// spawns a readLoop parked on fakeStream.Recv forever unless the recv
// channel is closed. Wire closeAll into t.Cleanup so those readLoops
// exit at end of test instead of accumulating across -count=N runs.
func newStubStreamFactory() (factory func(context.Context) (Stream, error), closeAll func()) {
	var mu sync.Mutex
	var streams []*fakeStream
	var closed bool
	factory = func(_ context.Context) (Stream, error) {
		mu.Lock()
		defer mu.Unlock()
		if closed {
			// After closeAll ran, refuse new streams so a fire-and-forget
			// createSession spawned by (e.g.) onClosing → spawnTickOnce
			// after cleanup can't produce a fresh unblocked fakeStream
			// that leaks its readLoop past p.Close.
			return nil, errors.New("newStubStreamFactory: closed")
		}
		s := newFakeStream()
		streams = append(streams, s)
		return s, nil
	}
	closeAll = func() {
		mu.Lock()
		defer mu.Unlock()
		closed = true
		for _, s := range streams {
			s.Close()
		}
	}
	return factory, closeAll
}

// injectActiveSession builds a fakeStream-backed Session in StateReady,
// wraps it in a SessionHandle, and registers it with the pool's
// sessionList. Bypasses the real Start/handshake path so tests can
// exercise pool-level logic in milliseconds. Since sl is now the sole
// store of active handles, this mirrors createSession + onActive's
// minimum required state: build sh, wire hook closures, register into
// sl. PeerInfo stays nil, so the handle lands in the AfeID=0 bucket —
// fine for pool-level tests that don't care about AFE fanout.
//
// sh.activated is stamped true to match production's onActive path,
// which is the "session reached StateReady" signal noteAbnormalCloseIfAny
// keys off of. Tests that want a "never activated" (FailedToStart-style)
// handle should use injectStartingSession instead.
func injectActiveSession(t testing.TB, p *SessionPoolImpl, name string, createdAt time.Time) *SessionHandle {
	t.Helper()
	stream := newFakeStream()
	sh := newSessionHandle(nil, createdAt)
	s := NewSession(name, stream, SessionHooks{
		OnStart:   p.onStart,
		OnActive:  func(_ *Session) { p.onActive(sh) },
		OnClosing: func(_ *Session) { p.onClosing(sh) },
		OnClose:   func(_ *Session, err error) { p.onClose(sh, err) },
	}, SessionTypeTable)
	s.state.Store(int32(StateReady))
	sh.session = s
	sh.activated.Store(true)

	p.sl.OnSessionStarted(sh)
	return sh
}

// injectStartingSession builds a fakeStream-backed Session that has NOT
// yet reached StateReady (StateStarting), registers it in the pool's
// startingSessions set, and returns the handle. Used by consecutive-
// failure tests to drive the state-based abnormal-close classification
// (`sh.activated == false` → abnormal). Does NOT register the handle
// with sl — mirrors production's createSession failure window between
// startingSessions insert and onActive.
func injectStartingSession(t testing.TB, p *SessionPoolImpl, name string) *SessionHandle {
	t.Helper()
	stream := newFakeStream()
	sh := newSessionHandle(nil, time.Now())
	s := NewSession(name, stream, SessionHooks{
		OnStart:   p.onStart,
		OnActive:  func(_ *Session) { p.onActive(sh) },
		OnClosing: func(_ *Session) { p.onClosing(sh) },
		OnClose:   func(_ *Session, err error) { p.onClose(sh, err) },
	}, SessionTypeTable)
	s.state.Store(int32(StateStarting))
	sh.session = s

	p.mu.Lock()
	p.startingSessions[sh] = struct{}{}
	p.mu.Unlock()
	return sh
}

func newTestPool(t testing.TB, min, max int) *SessionPoolImpl {
	t.Helper()
	factory, closeStreams := newStubStreamFactory()
	p := NewSessionPoolImpl(
		uint64(1),
		"test-pool",
		factory,
		&spb.OpenSessionRequest{ProtocolVersion: 1},
		nil,
		SessionTypeTable, true,
	)
	p.sizer.UpdateConfig(&spb.SessionClientConfiguration_SessionPoolConfiguration{
		MinSessionCount: int32(min), MaxSessionCount: int32(max),
	})
	// Close streams before closing the pool: the pool's Close waits for
	// per-session teardown, which requires the readLoop goroutines to
	// exit — and they won't while parked on fakeStream.Recv.
	t.Cleanup(func() {
		closeStreams()
		_ = p.Close()
	})
	return p
}

// TestSessionPool_Close_CompletesWithIdleSessions verifies that Close()
// returns promptly when sessions have nothing in flight, well within the 30s
// internal budget, and that poolCtx is cancelled after.
func TestSessionPool_Close_CompletesWithIdleSessions(t *testing.T) {
	p := newTestPool(t, 1, 10)

	// Three idle sessions registered with the pool. None have in-flight
	// VRPCs, so Session.Close should flip them to Closing and signal
	// quiescent immediately, letting the pool drain fast.
	for i := 0; i < 3; i++ {
		injectActiveSession(t, p, "idle", time.Now())
	}

	done := make(chan error, 1)
	go func() { done <- p.Close() }()

	select {
	case err := <-done:
		if err != nil {
			t.Errorf("Close returned err = %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("Close did not return within 5s for idle sessions (budget is 30s)")
	}

	// poolCtx must be cancelled after Close.
	select {
	case <-p.poolCtx.Done():
	default:
		t.Error("poolCtx not cancelled after Close()")
	}
}

// TestSessionPool_Close_BoundedByTimeout is skipped: constructing a stuck
// session that ignores Close() requires real readLoop/Send plumbing that
// blocks unrecoverably, which is hard to do safely in a unit test. The 30s
// timeout path is exercised in integration tests.
func TestSessionPool_Close_BoundedByTimeout(t *testing.T) {
	t.Skip("requires a session that ignores Close — see integration tests")
}

// TestTick_NoLongerPrunesOverprovisioned verifies the passive-
// shrink design: Tick with a scale-down delta must be a no-op
// — the pool shrinks only via OnClose's replace-on-death gate, not via a
// periodic prune. Regression guard against re-introducing the burst-then-
// lull oscillation that the earlier active-scale-down design produced.
func TestTick_NoLongerPrunesOverprovisioned(t *testing.T) {
	p := newTestPool(t, 1, 20)
	// 10 idle sessions, none in-flight → sizer will compute
	// desired ≈ minSessions (5-ish) and delta will be negative.
	// Tick must observe the negative delta and NOT shrink
	// the pool — regression guard for the removed active-scale-down.
	for i := 0; i < 10; i++ {
		sh := injectActiveSession(t, p, "idle", time.Now().Add(-time.Hour))
		_ = sh
	}

	before := p.sl.ReadyCount()
	p.Tick(context.Background())
	after := p.sl.ReadyCount()

	if after != before {
		t.Errorf("Tick pruned sessions: before=%d after=%d — scale-down must be advisory, not proactive", before, after)
	}
}

// TestSizer_ScaleDownIsAdvisory confirms the sizer still RETURNS a
// negative delta on overprovision (so ScalingHistory / callers can see
// the intent) but the calling site (Tick) must not act on it.
// The paired assertion above proves the caller is well-behaved; this one
// pins the sizer contract.
func TestSizer_ScaleDownIsAdvisory(t *testing.T) {
	// InUse=1, Pending=0, Ready=10 → desired ≈ 2, immediate=10.
	// Expect delta = 2 - 10 = -8 with Branch = "scale-down".
	stats := &PoolStats{ReadyCount: 10, InUseCount: 1}
	sizer := NewPoolSizer(func() *PoolStats { return stats }, 1, 20, 0.5)
	d := sizer.Decide()
	if d.Branch != "scale-down" {
		t.Fatalf("Branch = %q, want scale-down", d.Branch)
	}
	if d.Delta >= 0 {
		t.Errorf("Delta = %d, want negative (advisory scale-down)", d.Delta)
	}
}

// TestSessionPool_Invoke_RecordsSlowCheckoutFailure pins the fix for the
// pool-exhaustion blind spot: if CheckoutSession errors out (typically ctx
// deadline while parked waiting on freeSignal), Invoke must still push a
// row into the slow-vRPC ring — otherwise the exact incident an operator
// opens sessionz to debug ("pool saturated, everything timing out") leaves
// no evidence in the debug UI.
func TestSessionPool_Invoke_RecordsSlowCheckoutFailure(t *testing.T) {
	// Dial that blocks until its own ctx (poolCtx, wrapped) fires. No
	// session ever lands, so CheckoutSession is forced to park on
	// freeSignal until the caller's ctx cancels.
	neverDialing := func(ctx context.Context) (Stream, error) {
		<-ctx.Done()
		return nil, ctx.Err()
	}
	p := NewSessionPoolImpl(
		uint64(1),
		"test-pool",
		neverDialing,
		&spb.OpenSessionRequest{ProtocolVersion: 1},
		nil,
		SessionTypeTable, true,
	)
	p.sizer.UpdateConfig(&spb.SessionClientConfiguration_SessionPoolConfiguration{
		MinSessionCount: 0, MaxSessionCount: 1,
	})
	defer p.Close()

	// 50ms ctx budget vs the 10ms defaultSlowThreshold means the checkout
	// wait will comfortably exceed the slow-vRPC record threshold even on
	// slow CI, so the record path fires deterministically.
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	_, err := p.Invoke(ctx, newRoundTripDesc(), "hello")
	if err == nil {
		t.Fatal("Invoke on a pool that cannot dial succeeded; want ctx deadline error")
	}

	events := p.snapshotSlowVRpcs()
	if len(events) != 1 {
		t.Fatalf("snapshotSlowVRpcs len = %d, want 1 — checkout failure was not recorded", len(events))
	}
	ev := events[0]
	if ev.Method != "RoundTrip" {
		t.Errorf("Method = %q, want RoundTrip", ev.Method)
	}
	if ev.Success {
		t.Error("Success = true, want false (checkout failed)")
	}
	if ev.PoolWait <= 0 {
		t.Errorf("PoolWait = %v, want > 0", ev.PoolWait)
	}
	if ev.Latency != ev.PoolWait {
		t.Errorf("Latency = %v, PoolWait = %v — must match when all time was in checkout", ev.Latency, ev.PoolWait)
	}
	if ev.Session != "" {
		t.Errorf("Session = %q, want empty (no handle was ever returned)", ev.Session)
	}
	if ev.ErrCode != "DeadlineExceeded" {
		t.Errorf("ErrCode = %q, want DeadlineExceeded", ev.ErrCode)
	}
}

// TestCheckoutSession_ParkedWaiter_DeadlineExceeded pins the ctx-done
// unwind for a caller that made it into the waiter queue: the returned
// error must unwrap to context.DeadlineExceeded, and the waiter must be
// dequeued cleanly (no ghost entry left for a later signalFree to burn
// on). Together these are the contract callers rely on when their
// per-request deadline shorter than session-open time expires while
// they're parked — the shape of pool-side backpressure.
func TestCheckoutSession_ParkedWaiter_DeadlineExceeded(t *testing.T) {
	// Dial that blocks until its own ctx (poolCtx, wrapped) fires. No
	// session ever lands, so any CheckoutSession caller is forced to
	// park on the waiter queue until their ctx cancels.
	neverDialing := func(ctx context.Context) (Stream, error) {
		<-ctx.Done()
		return nil, ctx.Err()
	}
	p := NewSessionPoolImpl(
		uint64(1),
		"test-pool",
		neverDialing,
		&spb.OpenSessionRequest{ProtocolVersion: 1},
		nil,
		SessionTypeTable, true,
	)
	p.sizer.UpdateConfig(&spb.SessionClientConfiguration_SessionPoolConfiguration{
		MinSessionCount: 0, MaxSessionCount: 1,
	})
	defer p.Close()

	// Short deadline so the test doesn't loiter, long enough that the
	// caller is definitely parked before it fires. Kept as a named
	// const so the wall-time floor below stays coupled to it.
	const parkDeadline = 50 * time.Millisecond

	// Positive parking assertion: observer goroutine watches
	// waitersCount and closes `parked` the moment the caller enqueues.
	// Direct evidence of the waiter path — doesn't rely on timing to
	// infer that we parked. Stopped after CheckoutSession returns so
	// it can't outlive the test if parking never happened.
	parked := make(chan struct{})
	stop := make(chan struct{})
	go func() {
		tick := time.NewTicker(time.Millisecond)
		defer tick.Stop()
		for {
			if p.waitersCount.Load() > 0 {
				close(parked)
				return
			}
			select {
			case <-stop:
				return
			case <-tick.C:
			}
		}
	}()

	ctx, cancel := context.WithTimeout(context.Background(), parkDeadline)
	defer cancel()

	start := time.Now()
	sh, err := p.CheckoutSession(ctx)
	waited := time.Since(start)
	close(stop)

	if err == nil {
		t.Fatalf("CheckoutSession succeeded (handle=%v); want ctx deadline error", sh)
	}
	if sh != nil {
		t.Errorf("handle = %v, want nil on error", sh)
	}
	if !errors.Is(err, ErrNoSessionsAvailable) {
		t.Errorf("err = %v (type %T), want unwrappable to ErrNoSessionsAvailable (was the pool-exhaustion sentinel dropped?)", err, err)
	}
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Errorf("err = %v (type %T), want unwrappable to context.DeadlineExceeded (was the ctx cause dropped?)", err, err)
	}
	// Positive proof we parked (independent of timing).
	select {
	case <-parked:
	default:
		t.Error("waitersCount never observed > 0 — CheckoutSession may have fast-failed before enqueuing")
	}
	// Wall-time sanity: floor derived from parkDeadline (80%) so
	// scaling the deadline auto-scales the floor. Kept as a soft
	// diagnostic — if the observer above missed a very brief parking
	// window, a much-less-than-deadline wall time would still surface
	// the fast-fail regression here.
	if minPark := parkDeadline * 8 / 10; waited < minPark {
		t.Errorf("CheckoutSession returned after %v; want ≥ %v (parkDeadline=%v)", waited, minPark, parkDeadline)
	}

	// waitersCount must have been decremented on the ctx-done path.
	if got := p.waitersCount.Load(); got != 0 {
		t.Errorf("waitersCount = %d after cancelled checkout, want 0", got)
	}
	// Direct queue inspection — belt-and-suspenders since the counter
	// and the list are updated separately. A leftover entry here is the
	// "ghost waiter" bug: a subsequent signalFree would consume a wake
	// token on a caller that already returned.
	p.waitersMu.Lock()
	qlen := p.waiters.Len()
	p.waitersMu.Unlock()
	if qlen != 0 {
		t.Errorf("waiter queue length = %d, want 0 (ghost waiter left after ctx cancel)", qlen)
	}
}

// TestRecordPickDecision_RingWrap verifies the O(1) circular-buffer
// implementation of pickHistory: pre-wrap events are in insertion order,
// post-wrap events preserve oldest-first ordering, and the ring keeps
// exactly maxPickHistory entries. Regression guard against the previous
// shift-based implementation that memmoved the whole buffer per record
// (~24µs p99 CheckoutSession regression at moderate QPS).
func TestRecordPickDecision_RingWrap(t *testing.T) {
	p := newTestPool(t, 1, 10)

	// Phase 1: fill up to cap-1 → snapshot should be insertion-ordered.
	for i := 0; i < maxPickHistory-1; i++ {
		p.recordPickDecision(PickDecision{Reason: "phase1", Winner: AfeID(i + 1)}, "test")
	}
	snap := p.snapshotPickHistory()
	if len(snap) != maxPickHistory-1 {
		t.Fatalf("pre-wrap snapshot len = %d, want %d", len(snap), maxPickHistory-1)
	}
	if snap[0].Decision.Winner != AfeID(1) || snap[len(snap)-1].Decision.Winner != AfeID(maxPickHistory-1) {
		t.Errorf("pre-wrap ordering broken: first=%d last=%d",
			snap[0].Decision.Winner, snap[len(snap)-1].Decision.Winner)
	}

	// Phase 2: overshoot cap by 100 → ring wraps.
	for i := maxPickHistory - 1; i < maxPickHistory+100; i++ {
		p.recordPickDecision(PickDecision{Reason: "phase2", Winner: AfeID(i + 1)}, "test")
	}
	snap = p.snapshotPickHistory()
	if len(snap) != maxPickHistory {
		t.Fatalf("post-wrap snapshot len = %d, want %d (ring must cap)", len(snap), maxPickHistory)
	}
	// After 100 overshoots, the oldest surviving Winner is 101; newest is
	// maxPickHistory+100.
	wantOldest := AfeID(101)
	wantNewest := AfeID(maxPickHistory + 100)
	if snap[0].Decision.Winner != wantOldest {
		t.Errorf("post-wrap oldest = %d, want %d", snap[0].Decision.Winner, wantOldest)
	}
	if snap[len(snap)-1].Decision.Winner != wantNewest {
		t.Errorf("post-wrap newest = %d, want %d", snap[len(snap)-1].Decision.Winner, wantNewest)
	}
	// Ordering must be monotonic (oldest-first).
	for i := 1; i < len(snap); i++ {
		if snap[i].Decision.Winner <= snap[i-1].Decision.Winner {
			t.Fatalf("ordering broken at i=%d: snap[i-1].Winner=%d snap[i].Winner=%d",
				i, snap[i-1].Decision.Winner, snap[i].Decision.Winner)
		}
	}
}

// --- core pool setters + hot-path helpers (session_pool.go) ----------------

func TestNewSessionPoolImpl_Identity(t *testing.T) {
	factory, closeStreams := newStubStreamFactory()
	t.Cleanup(closeStreams)

	p := NewSessionPoolImpl(
		uint64(42),
		"test-pool", factory, &spb.OpenSessionRequest{ProtocolVersion: 1}, nil, SessionTypeTable, true,
	)
	if p.poolID != 42 {
		t.Errorf("poolID = %d, want 42", p.poolID)
	}
	p.Close()
}

// TestSignalFree_NoWaitersIsNoOp verifies the queue-based signalFree
// exits fast when no CheckoutSession caller is parked (nothing to
// wake). Regression guard against re-introducing a cap-1 channel or
// any other "buffer a spare wake-up" scheme.
func TestSignalFree_NoWaitersIsNoOp(t *testing.T) {
	p := newTestPool(t, 1, 10)
	done := make(chan struct{})
	go func() {
		p.signalFree()
		p.signalFree()
		p.signalFree()
		close(done)
	}()
	select {
	case <-done:
		// expected — no blocking
	case <-time.After(100 * time.Millisecond):
		t.Fatal("signalFree blocked with empty waiter queue")
	}
	// No accumulated tokens: signalFree with no waiters is truly a no-op.
	p.waitersMu.Lock()
	remaining := p.waiters.Len()
	p.waitersMu.Unlock()
	if remaining != 0 {
		t.Errorf("waiters queue = %d, want 0 (signalFree must not add anything)", remaining)
	}
}

// TestSignalFree_WakesHeadWaiter parks two waiters directly on the
// queue and verifies the first signalFree wakes exactly the first one
// (FIFO), and a second signalFree wakes the second. Expected
// pendingRpcs.removeFirst semantic.
func TestSignalFree_WakesHeadWaiter(t *testing.T) {
	p := newTestPool(t, 1, 10)

	w1 := &waiter{ready: make(chan struct{})}
	w2 := &waiter{ready: make(chan struct{})}
	p.waitersMu.Lock()
	w1.elem = p.waiters.PushBack(w1)
	w2.elem = p.waiters.PushBack(w2)
	p.waitersMu.Unlock()

	p.signalFree()
	select {
	case <-w1.ready:
		// expected
	case <-time.After(100 * time.Millisecond):
		t.Fatal("head waiter not woken by first signalFree")
	}
	select {
	case <-w2.ready:
		t.Error("second waiter woken by first signalFree — FIFO broken")
	default:
	}

	p.signalFree()
	select {
	case <-w2.ready:
		// expected
	case <-time.After(100 * time.Millisecond):
		t.Fatal("second waiter not woken by second signalFree")
	}
}

// TestRemoveWaiter_IdempotentWithSignalFree verifies the concurrency
// contract: signalFree and removeWaiter never double-close w.ready.
// signalFree removes+closes; if the caller subsequently ctx-cancels,
// removeWaiter checks w.elem == nil and skips the close.
func TestRemoveWaiter_IdempotentWithSignalFree(t *testing.T) {
	p := newTestPool(t, 1, 10)

	w := &waiter{ready: make(chan struct{})}
	p.waitersMu.Lock()
	w.elem = p.waiters.PushBack(w)
	p.waitersMu.Unlock()

	// signalFree removes and closes.
	p.signalFree()
	select {
	case <-w.ready:
	case <-time.After(100 * time.Millisecond):
		t.Fatal("waiter not woken by signalFree")
	}

	// removeWaiter called AFTER signalFree must not panic (would
	// double-close if it re-issued close on an already-closed channel).
	p.removeWaiter(w)
	// Sanity: w.elem stays nil.
	p.waitersMu.Lock()
	elem := w.elem
	p.waitersMu.Unlock()
	if elem != nil {
		t.Errorf("w.elem = %p, want nil after signalFree", elem)
	}
}

func TestStats_CountsReadyInUsePending(t *testing.T) {
	p := newTestPool(t, 1, 10)
	// Two ready sessions; one has outstanding=1 (in-use), other is idle.
	sh1 := injectActiveSession(t, p, "s1", time.Now())
	sh2 := injectActiveSession(t, p, "s2", time.Now())
	sh1.IncOutstanding()
	_ = sh2 // idle

	// Fake three parked waiters via waitersCount (the real path is inside
	// CheckoutSession's select; we bracket it directly here).
	p.waitersCount.Add(3)

	st := p.Stats()
	if st.ReadyCount != 2 {
		t.Errorf("ReadyCount = %d, want 2", st.ReadyCount)
	}
	if st.InUseCount != 1 {
		t.Errorf("InUseCount = %d, want 1", st.InUseCount)
	}
	if st.PendingCount != 3 {
		t.Errorf("PendingCount = %d, want 3 (must be waitersCount, not sum of outstanding)", st.PendingCount)
	}
	if st.StartingCount != 0 {
		t.Errorf("StartingCount = %d, want 0", st.StartingCount)
	}
}

// TestStats_StartingCountIncludesPendingStarts guards the sizer
// double-count regression introduced by removing wg.Wait() in Tick
// (commit 203aa07ecf). Tick reserves pendingStarts BEFORE spawning
// fire-and-forget goroutines; createSession transfers each reservation
// into startingSessions once streamFactory succeeds. During the
// streamFactory window Stats().StartingCount MUST see the reservation
// so a burst of Ticks doesn't re-request the same delta.
func TestStats_StartingCountIncludesPendingStarts(t *testing.T) {
	p := newTestPool(t, 1, 10)

	// Simulate Tick's pre-spawn reservation for delta=3.
	p.mu.Lock()
	p.pendingStarts = 3
	p.mu.Unlock()

	if got := p.Stats().StartingCount; got != 3 {
		t.Errorf("StartingCount with pendingStarts=3, startingSessions=empty: got %d, want 3", got)
	}

	// Simulate one goroutine reaching the transfer point: reservation
	// consumed, session added to startingSessions. Sum must be unchanged.
	sh := newSessionHandle(nil, time.Now())
	p.mu.Lock()
	p.pendingStarts--
	p.startingSessions[sh] = struct{}{}
	p.mu.Unlock()

	if got := p.Stats().StartingCount; got != 3 {
		t.Errorf("StartingCount after transfer (pendingStarts=2, startingSessions=1): got %d, want 3", got)
	}
}

func TestUpdateConfig_SwapsPickerAndBounds(t *testing.T) {
	p := newTestPool(t, 1, 10)
	// Default picker is LeastInFlight.
	if got := p.picker.Name(); got != "least-inflight" {
		t.Errorf("default picker = %q, want least-inflight", got)
	}

	// Switch to Random.
	p.UpdateConfig(&spb.SessionClientConfiguration_SessionPoolConfiguration{
		MinSessionCount: 4,
		MaxSessionCount: 40,
		LoadBalancingOptions: &spb.LoadBalancingOptions{
			LoadBalancingStrategy: &spb.LoadBalancingOptions_Random_{
				Random: &spb.LoadBalancingOptions_Random{},
			},
		},
	})
	if got := p.picker.Name(); got != "simple" {
		t.Errorf("after Random swap, picker = %q, want simple", got)
	}
	if p.sizer.MinSessions() != 4 || p.sizer.MaxSessions() != 40 {
		t.Errorf("min/max = %d/%d, want 4/40", p.sizer.MinSessions(), p.sizer.MaxSessions())
	}

	// Switch to PeakEwma with an explicit subset size.
	p.UpdateConfig(&spb.SessionClientConfiguration_SessionPoolConfiguration{
		MinSessionCount: 1,
		MaxSessionCount: 10,
		LoadBalancingOptions: &spb.LoadBalancingOptions{
			LoadBalancingStrategy: &spb.LoadBalancingOptions_PeakEwma_{
				PeakEwma: &spb.LoadBalancingOptions_PeakEwma{RandomSubsetSize: 3},
			},
		},
	})
	if got := p.picker.Name(); got != "least-latency" {
		t.Errorf("after PeakEwma swap, picker = %q, want least-latency", got)
	}
	// listenerFires counter bumps once per UpdateConfig.
	if got := p.m.listenerFires.Load(); got != 2 {
		t.Errorf("listenerFires = %d, want 2 (one per UpdateConfig)", got)
	}
}

// TestUpdateConfig_HonorsRandomSubsetSize is a regression guard against the
// bug where UpdateConfig's LeastInFlight branch ignored its own
// RandomSubsetSize (only PeakEwma read it). Server-driven K must reach the
// picker for both K-choice strategies.
func TestUpdateConfig_HonorsRandomSubsetSize(t *testing.T) {
	cases := []struct {
		name string
		lbo  *spb.LoadBalancingOptions
		want int
	}{
		{
			name: "LeastInFlight with K=5",
			lbo: &spb.LoadBalancingOptions{
				LoadBalancingStrategy: &spb.LoadBalancingOptions_LeastInFlight_{
					LeastInFlight: &spb.LoadBalancingOptions_LeastInFlight{RandomSubsetSize: 5},
				},
			},
			want: 5,
		},
		{
			name: "PeakEwma with K=7",
			lbo: &spb.LoadBalancingOptions{
				LoadBalancingStrategy: &spb.LoadBalancingOptions_PeakEwma_{
					PeakEwma: &spb.LoadBalancingOptions_PeakEwma{RandomSubsetSize: 7},
				},
			},
			want: 7,
		},
		{
			name: "LeastInFlight with omitted K falls back to default",
			lbo: &spb.LoadBalancingOptions{
				LoadBalancingStrategy: &spb.LoadBalancingOptions_LeastInFlight_{
					LeastInFlight: &spb.LoadBalancingOptions_LeastInFlight{}, // K=0 → fallback
				},
			},
			want: defaultAfeRandomSubsetSize,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			p := newTestPool(t, 1, 10)
			p.UpdateConfig(&spb.SessionClientConfiguration_SessionPoolConfiguration{
				MinSessionCount:      1,
				MaxSessionCount:      10,
				LoadBalancingOptions: tc.lbo,
			})
			var gotK int
			switch pk := p.picker.(type) {
			case *LeastInFlightAfePicker:
				gotK = pk.RandomSubsetSize
			case *LeastLatencyAfePicker:
				gotK = pk.RandomSubsetSize
			default:
				t.Fatalf("unexpected picker type %T", p.picker)
			}
			if gotK != tc.want {
				t.Errorf("picker RandomSubsetSize = %d, want %d", gotK, tc.want)
			}
		})
	}
}

// TestPickerFromLoadBalancing_NilFallback verifies the constructor's
// bootstrap path — a nil LoadBalancingOptions gives the default
// (LeastInFlight with K=defaultAfeRandomSubsetSize).
func TestPickerFromLoadBalancing_NilFallback(t *testing.T) {
	picker := pickerFromLoadBalancing(nil, true)
	li, ok := picker.(*LeastInFlightAfePicker)
	if !ok {
		t.Fatalf("nil LBO → %T, want *LeastInFlightAfePicker", picker)
	}
	if li.RandomSubsetSize != defaultAfeRandomSubsetSize {
		t.Errorf("K = %d, want %d", li.RandomSubsetSize, defaultAfeRandomSubsetSize)
	}
}

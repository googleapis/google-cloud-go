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
	"strings"
	"sync"
	"testing"
	"time"

	spb "cloud.google.com/go/bigtable/apiv2/bigtablepb"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/durationpb"
)

// stampCloseReason sets a Session's close reason directly. Bypasses
// handleClose so tests can drive the noteAbnormalCloseIfAny gate
// without wiring a fake stream teardown.
func stampCloseReason(s *Session, reason string) {
	s.setCloseReason(reason)
}

// abnormalOnCloseFor injects a fresh Session into the pool, stamps a
// close reason, then invokes p.onClose(sh, nil) so the abnormal-close
// counter path runs. abnormal picks a reason isAbnormalCloseReason
// classifies as abnormal ("StreamEnd:Unavailable"); a false argument
// picks a clean reason ("StreamEnd:EOF").
func abnormalOnCloseFor(t testing.TB, p *SessionPoolImpl, abnormal bool) {
	t.Helper()
	sh := injectActiveSession(t, p, "sess", time.Now())
	reason := "StreamEnd:EOF"
	if abnormal {
		reason = "StreamEnd:Unavailable"
	}
	stampCloseReason(sh.session, reason)
	p.onClose(sh, nil)
}

func TestConsecutiveFailures_AbnormalOnCloseIncrements(t *testing.T) {
	p := newTestPool(t, 1, 10)

	if got := p.consecutiveFailures.Load(); got != 0 {
		t.Fatalf("initial consecutiveFailures = %d, want 0", got)
	}

	// Two abnormal closes below the default threshold of 10.
	abnormalOnCloseFor(t, p, true)
	abnormalOnCloseFor(t, p, true)

	if got := p.consecutiveFailures.Load(); got != 2 {
		t.Fatalf("after 2 abnormal closes = %d, want 2", got)
	}
}

func TestConsecutiveFailures_CleanCloseDoesNotIncrement(t *testing.T) {
	p := newTestPool(t, 1, 10)
	abnormalOnCloseFor(t, p, false)
	if got := p.consecutiveFailures.Load(); got != 0 {
		t.Fatalf("clean close bumped counter to %d, want 0", got)
	}
}

// TestConsecutiveFailures_UserReasonNotAbnormal pins the fix that keeps
// pool-driven teardown from tripping the consecutive-failure circuit
// breaker. Pool.Close's Phase-2 fires s.Close with
// CLOSE_SESSION_REASON_USER on every session; closeReasonLabel maps that
// to "User". Without the whitelist entry, isAbnormalCloseReason would
// classify each close as abnormal and a 100-session Pool.Close would trip
// the breaker AND drain any late-arriving waiters with
// ErrConsecutiveFailures instead of ErrPoolClosed.
func TestConsecutiveFailures_UserReasonNotAbnormal(t *testing.T) {
	p := newTestPool(t, 1, 10)

	// Drop threshold to 2 so a leaked-classification bug would trip on
	// the second close instead of needing 10.
	p.consecutiveFailureThreshold.Store(2)

	// Fire "User" close-reason twice — the label Pool.Close's Phase-2
	// stamps on every session.
	for i := 0; i < 2; i++ {
		sh := injectActiveSession(t, p, "user-close", time.Now())
		stampCloseReason(sh.session, "User")
		p.onClose(sh, nil)
	}

	if got := p.consecutiveFailures.Load(); got != 0 {
		t.Fatalf("consecutiveFailures after 2 User closes = %d, want 0 (User must be whitelisted; Pool.Close would otherwise trip the breaker on every teardown)", got)
	}
}

// TestConsecutiveFailures_ResetOnSessionOpen pins Java-parity reset
// semantics: a successful session-open (onActive) clears the
// consecutive-failure counter — NOT a successful vRPC. Under sustained
// failures no session reaches Ready, so the counter grows toward the
// trip threshold; a fresh session opening is the only "sustained
// transport health" signal that clears it.
func TestConsecutiveFailures_ResetOnSessionOpen(t *testing.T) {
	p := newTestPool(t, 1, 10)
	abnormalOnCloseFor(t, p, true)
	abnormalOnCloseFor(t, p, true)
	if got := p.consecutiveFailures.Load(); got != 2 {
		t.Fatalf("precondition: counter = %d, want 2", got)
	}

	// Fire onActive on a fresh handle — mirrors production's
	// createSession → hooks.onActive → p.onActive(sh) chain.
	sh := newSessionHandle(nil, time.Now())
	stream := newFakeStream()
	s := NewSession("healthy", stream, SessionHooks{
		OnStart:  p.onStart,
		OnActive: func(_ *Session) { p.onActive(sh) },
		OnClose:  func(_ *Session, err error) { p.onClose(sh, err) },
	}, SessionTypeTable)
	s.state.Store(int32(StateReady))
	sh.session = s
	p.onActive(sh)

	if got := p.consecutiveFailures.Load(); got != 0 {
		t.Fatalf("after successful session-open = %d, want 0", got)
	}
}

// TestConsecutiveFailures_VRpcOutcomeDoesNotReset pins the flip side
// of ResetOnSessionOpen: neither an OK nor a failed vRPC affects the
// counter. Only session-open events (onActive) reset it.
func TestConsecutiveFailures_VRpcOutcomeDoesNotReset(t *testing.T) {
	p := newTestPool(t, 1, 10)
	abnormalOnCloseFor(t, p, true)
	abnormalOnCloseFor(t, p, true)
	if got := p.consecutiveFailures.Load(); got != 2 {
		t.Fatalf("precondition: counter = %d, want 2", got)
	}

	sh := injectActiveSession(t, p, "healthy", time.Now())
	// OK vRPC — must NOT clear the counter.
	p.noteVRpcOutcome(sh, time.Millisecond, time.Millisecond, true)
	if got := p.consecutiveFailures.Load(); got != 2 {
		t.Errorf("counter after OK vRPC = %d, want 2 (only onActive resets)", got)
	}
	// Failed vRPC — also must NOT touch the counter (increment happens
	// via onClose's noteAbnormalCloseIfAny, not via vRPC outcome).
	p.noteVRpcOutcome(sh, time.Millisecond, time.Millisecond, false)
	if got := p.consecutiveFailures.Load(); got != 2 {
		t.Errorf("counter after failed vRPC = %d, want 2 (vRPC outcome must not touch the counter)", got)
	}
}

func TestConsecutiveFailures_UpdateConfigHonoredByTrip(t *testing.T) {
	p := newTestPool(t, 1, 10)

	// Drop the threshold to 3 via UpdateConfig — same code path
	// ClientConfigurationManager would take.
	p.UpdateConfig(&spb.SessionClientConfiguration_SessionPoolConfiguration{
		MinSessionCount:                    1,
		MaxSessionCount:                    10,
		ConsecutiveSessionFailureThreshold: 3,
		NewSessionCreationBudget:           50,
		NewSessionCreationPenalty:          durationpb.New(60 * time.Second),
	})
	if got := p.consecutiveFailureThreshold.Load(); got != 3 {
		t.Fatalf("threshold after UpdateConfig = %d, want 3", got)
	}

	// Two abnormal closes stay under the trip.
	abnormalOnCloseFor(t, p, true)
	abnormalOnCloseFor(t, p, true)
	if got := p.consecutiveFailures.Load(); got != 2 {
		t.Fatalf("pre-trip counter = %d, want 2", got)
	}

	// Third abnormal close crosses the threshold and resets.
	abnormalOnCloseFor(t, p, true)
	if got := p.consecutiveFailures.Load(); got != 0 {
		t.Fatalf("post-trip counter = %d, want 0 (reset by trip)", got)
	}
}

func TestConsecutiveFailures_TripWakesParkedWaitersWithSentinel(t *testing.T) {
	p := newTestPool(t, 1, 10)

	// Drop threshold to 2 for a snappy test. Any live handles the pool
	// might have from setup are irrelevant to the waiter path: we park
	// callers directly on the waiter queue via drainWaitersWithErr's
	// wake channel semantics, which is what the trip actually calls.
	p.consecutiveFailureThreshold.Store(2)

	// Two goroutines block in the waiter queue. Rather than call
	// CheckoutSession (which would race with any auto-scaling scheduled
	// off the pool ctx), enqueue directly — the test exercises the
	// wake/err contract, and CheckoutSession is separately covered.
	const nWaiters = 3
	results := make(chan error, nWaiters)
	var wg sync.WaitGroup
	waiters := make([]*waiter, 0, nWaiters)
	for i := 0; i < nWaiters; i++ {
		w := &waiter{ready: make(chan struct{})}
		p.waitersMu.Lock()
		w.elem = p.waiters.PushBack(w)
		p.waitersMu.Unlock()
		p.waitersCount.Add(1)
		waiters = append(waiters, w)

		wg.Add(1)
		go func(w *waiter) {
			defer wg.Done()
			<-w.ready
			p.waitersCount.Add(-1)
			results <- w.err
		}(w)
	}

	// Trip the threshold.
	abnormalOnCloseFor(t, p, true)
	abnormalOnCloseFor(t, p, true)

	// All waiters must wake with ErrConsecutiveFailures within a
	// generous but bounded window (no polling — direct chan wait
	// backed by drainWaitersWithErr closing every ready chan).
	waitCh := make(chan struct{})
	go func() { wg.Wait(); close(waitCh) }()
	select {
	case <-waitCh:
	case <-time.After(2 * time.Second):
		t.Fatal("waiters not woken after trip")
	}

	close(results)
	got := 0
	for err := range results {
		if !errors.Is(err, ErrConsecutiveFailures) {
			t.Fatalf("waiter err = %v, want ErrConsecutiveFailures", err)
		}
		got++
	}
	if got != nWaiters {
		t.Fatalf("woke %d waiters, want %d", got, nWaiters)
	}

	// Counter must have been reset by the trip.
	if got := p.consecutiveFailures.Load(); got != 0 {
		t.Fatalf("counter after trip = %d, want 0", got)
	}

	// After the trip, drainWaitersWithErr has emptied the queue.
	p.waitersMu.Lock()
	remaining := p.waiters.Len()
	p.waitersMu.Unlock()
	if remaining != 0 {
		t.Fatalf("waiter queue after trip has %d entries, want 0", remaining)
	}
	_ = waiters // silence unused when the test is trimmed
}

// TestConsecutiveFailures_TripCarriesLastCauseAndGRPCStatus pins the
// error propagation contract added alongside the abnormal-close
// counter: when the breaker trips, waiters wake with an error that
// (a) satisfies errors.Is(err, ErrConsecutiveFailures) — retry paths
// still work; (b) unwraps to the last session's raw Recv error — the
// operator sees WHY the trip happened; (c) inherits the underlying
// gRPC status code via GRPCStatus so status.Code(err) returns the
// server's rejection code (e.g. FailedPrecondition) instead of Unknown.
// Without this, a still-being-created MV manifests as
// "session pool tripped consecutive-failure threshold" with code
// Unknown — masking the actionable server signal.
func TestConsecutiveFailures_TripCarriesLastCauseAndGRPCStatus(t *testing.T) {
	p := newTestPool(t, 1, 10)
	p.consecutiveFailureThreshold.Store(2)

	// The last abnormal close will carry a synthetic FailedPrecondition
	// mirroring the shape the server emits when MV backfill is running.
	const wantMsg = "materialized view is still being created"
	serverErr := status.Error(codes.FailedPrecondition, wantMsg)

	// One waiter is enough to observe the drain-time wrap.
	w := &waiter{ready: make(chan struct{})}
	p.waitersMu.Lock()
	w.elem = p.waiters.PushBack(w)
	p.waitersMu.Unlock()
	p.waitersCount.Add(1)

	done := make(chan error, 1)
	go func() {
		<-w.ready
		p.waitersCount.Add(-1)
		done <- w.err
	}()

	// First bump: benign cause, doesn't trip.
	sh1 := injectActiveSession(t, p, "s1", time.Now())
	stampCloseReason(sh1.session, "StreamEnd:Unavailable")
	sh1.session.setCloseErr(errors.New("transient"))
	p.onClose(sh1, nil)

	// Second bump: the cause we want propagated to the waiter.
	sh2 := injectActiveSession(t, p, "s2", time.Now())
	stampCloseReason(sh2.session, "StreamEnd:FailedPrecondition")
	sh2.session.setCloseErr(serverErr)
	p.onClose(sh2, nil)

	var got error
	select {
	case got = <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("waiter not woken after trip")
	}

	if !errors.Is(got, ErrConsecutiveFailures) {
		t.Fatalf("errors.Is(err, ErrConsecutiveFailures) = false; err = %v", got)
	}
	if code := status.Code(got); code != codes.FailedPrecondition {
		t.Fatalf("status.Code(err) = %v, want FailedPrecondition; err = %v", code, got)
	}
	if !strings.Contains(got.Error(), wantMsg) {
		t.Fatalf("err text %q missing underlying cause %q", got.Error(), wantMsg)
	}
}

func TestConsecutiveFailures_CheckoutSessionReturnsSentinelOnTrip(t *testing.T) {
	p := newTestPool(t, 1, 10)
	p.consecutiveFailureThreshold.Store(1)

	// No idle sessions, no scaling wired to make one — CheckoutSession
	// will park immediately. Give it a long-ish ctx so the ctx path
	// doesn't win the race with the trip.
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	got := make(chan error, 1)
	go func() {
		_, err := p.CheckoutSession(ctx)
		got <- err
	}()

	// Wait until the caller is parked, then trip.
	deadline := time.Now().Add(500 * time.Millisecond)
	for time.Now().Before(deadline) {
		if p.waitersCount.Load() > 0 {
			break
		}
		time.Sleep(2 * time.Millisecond)
	}
	if p.waitersCount.Load() == 0 {
		t.Fatal("CheckoutSession did not park within 500ms")
	}

	abnormalOnCloseFor(t, p, true)

	select {
	case err := <-got:
		if !errors.Is(err, ErrConsecutiveFailures) {
			t.Fatalf("CheckoutSession err = %v, want ErrConsecutiveFailures", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("CheckoutSession did not return after trip")
	}
}

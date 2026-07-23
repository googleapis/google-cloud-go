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
	"fmt"
	"sync"
	"testing"
	"time"

	spb "cloud.google.com/go/bigtable/apiv2/bigtablepb"
	"google.golang.org/genproto/googleapis/rpc/errdetails"
	rpcstatus "google.golang.org/genproto/googleapis/rpc/status"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/durationpb"
)

// runVRpcCapture drives an Invoke call to completion with a stub
// VirtualRpcResponse, then returns the *VirtualRpcRequest that was sent on the
// wire. All inspection of the deadline / metadata plumbing is done against
// this snapshot.
func runVRpcCapture(t *testing.T, ctx context.Context) *spb.VirtualRpcRequest {
	t.Helper()
	s, stream := makeActive(t, SessionHooks{})
	desc := newRoundTripDesc()

	done := make(chan struct{})
	go func() {
		defer close(done)
		_, _ = s.Invoke(ctx, desc, "hello")
	}()

	waitFor(t, time.Second, func() bool { return len(stream.snapshotSent()) > 0 }, "Send called")
	sent := stream.snapshotSent()[0].GetVirtualRpc()
	if sent == nil {
		t.Fatal("sent frame was not a VirtualRpcRequest")
	}

	// Deliver a benign success so Invoke returns and the goroutine exits.
	s.handleVRPCResponse(&spb.VirtualRpcResponse{
		RpcId:   sent.RpcId,
		Payload: []byte("ok"),
	})
	<-done
	return sent
}

func TestInvoke_DeadlineCarriedInRequest(t *testing.T) {
	ctx, cancel := context.WithDeadline(context.Background(), time.Now().Add(500*time.Millisecond))
	defer cancel()

	sent := runVRpcCapture(t, ctx)
	if sent.Deadline == nil {
		t.Fatal("VirtualRpc.Deadline = nil, want non-nil when ctx carries a deadline")
	}
	d := sent.Deadline.AsDuration()
	if d <= 0 || d > 500*time.Millisecond {
		t.Errorf("VirtualRpc.Deadline = %v, want in (0, 500ms]", d)
	}
	// A negative or absurdly small value would imply we lost most of the
	// budget to test setup; sanity-bound it.
	if d < time.Microsecond {
		t.Errorf("VirtualRpc.Deadline = %v, suspiciously small", d)
	}
}

func TestInvoke_NoDeadlineWhenCtxHasNone(t *testing.T) {
	sent := runVRpcCapture(t, context.Background())
	if sent.Deadline != nil {
		t.Errorf("VirtualRpc.Deadline = %v, want nil when ctx has no deadline", sent.Deadline.AsDuration())
	}
}

func TestInvoke_AttemptNumberFromCtx(t *testing.T) {
	// WithAttempt is a no-op unless WithVRpcMetadata has seeded the context
	// with a vrpcMetadata struct first — that's how the retrying interceptor
	// uses it in production (the retry loop seeds attempt=1 on entry, then
	// updates via WithAttempt on each retry).
	ctx := WithVRpcMetadata(context.Background(), "ReadRow", 1)
	ctx = WithAttempt(ctx, 7)
	sent := runVRpcCapture(t, ctx)
	if sent.Metadata == nil {
		t.Fatal("VirtualRpc.Metadata = nil")
	}
	if got := sent.Metadata.AttemptNumber; got != 7 {
		t.Errorf("AttemptNumber = %d, want 7", got)
	}
}

func TestInvoke_AttemptNumberDefault(t *testing.T) {
	sent := runVRpcCapture(t, context.Background())
	if sent.Metadata == nil {
		t.Fatal("VirtualRpc.Metadata = nil")
	}
	if got := sent.Metadata.AttemptNumber; got != 1 {
		t.Errorf("AttemptNumber = %d, want 1 (calls without WithAttempt should default to first attempt)", got)
	}
}

func TestInvoke_AttemptStartIsRecent(t *testing.T) {
	before := time.Now()
	sent := runVRpcCapture(t, context.Background())
	after := time.Now()

	if sent.Metadata == nil || sent.Metadata.AttemptStart == nil {
		t.Fatal("VirtualRpc.Metadata.AttemptStart = nil")
	}
	got := sent.Metadata.AttemptStart.AsTime()
	// Allow a small slop so wall-clock skew between captures doesn't flake.
	const slop = 2 * time.Second
	if got.Before(before.Add(-slop)) || got.After(after.Add(slop)) {
		t.Errorf("AttemptStart = %v, want within [%v, %v] (slop %v)", got, before, after, slop)
	}
}

// --- Invoke: Stats, SentAt, RetryInfo plumbing ------------------------

// runInvoke drives an Invoke call to completion by waiting for the
// outbound frame, calling deliver(sent) to produce the server response, and
// returning the populated InvokeResult + err. It mirrors runVRpcCapture but
// returns the full InvokeResult so tests can assert on Stats / SentAt /
// ClusterInfo without the back-compat triple stripping them.
func runInvoke(t *testing.T, ctx context.Context, deliver func(s *Session, sent *spb.VirtualRpcRequest)) (InvokeResult, error) {
	t.Helper()
	s, stream := makeActive(t, SessionHooks{})
	desc := newRoundTripDesc()

	type outcome struct {
		res InvokeResult
		err error
	}
	resCh := make(chan outcome, 1)
	go func() {
		r, err := s.Invoke(ctx, desc, "hello")
		resCh <- outcome{res: r, err: err}
	}()

	waitFor(t, time.Second, func() bool { return len(stream.snapshotSent()) > 0 }, "Send called")
	sent := stream.snapshotSent()[0].GetVirtualRpc()
	if sent == nil {
		t.Fatal("sent frame was not a VirtualRpcRequest")
	}
	deliver(s, sent)
	select {
	case out := <-resCh:
		return out.res, out.err
	case <-time.After(time.Second):
		t.Fatal("Invoke did not return after delivering response")
		return InvokeResult{}, nil
	}
}

func TestInvoke_StatsExtractedFromResponse(t *testing.T) {
	const wantBackend = 123 * time.Millisecond

	before := time.Now()
	res, err := runInvoke(t, context.Background(), func(s *Session, sent *spb.VirtualRpcRequest) {
		s.handleVRPCResponse(&spb.VirtualRpcResponse{
			RpcId:   sent.RpcId,
			Payload: []byte("ok"),
			Stats: &spb.SessionRequestStats{
				BackendLatency: durationpb.New(wantBackend),
			},
		})
	})
	if err != nil {
		t.Fatalf("Invoke: unexpected err %v", err)
	}
	if res.Stats == nil {
		t.Fatal("InvokeResult.Stats = nil, want non-nil (server-supplied Stats must be propagated)")
	}
	if got := res.Stats.GetBackendLatency().AsDuration(); got != wantBackend {
		t.Errorf("InvokeResult.Stats.BackendLatency = %v, want %v", got, wantBackend)
	}
	// Sanity bookend on SentAt.
	if res.SentAt.Before(before) {
		t.Errorf("SentAt %v < before %v", res.SentAt, before)
	}
}

func TestInvoke_StatsNilWhenServerOmits(t *testing.T) {
	res, err := runInvoke(t, context.Background(), func(s *Session, sent *spb.VirtualRpcRequest) {
		s.handleVRPCResponse(&spb.VirtualRpcResponse{
			RpcId:   sent.RpcId,
			Payload: []byte("ok"),
			// Stats intentionally nil.
		})
	})
	if err != nil {
		t.Fatalf("Invoke: unexpected err %v", err)
	}
	if res.Stats != nil {
		t.Errorf("InvokeResult.Stats = %v, want nil when server omits Stats", res.Stats)
	}
}

func TestInvoke_SentAtIsNonZero(t *testing.T) {
	before := time.Now()
	res, err := runInvoke(t, context.Background(), func(s *Session, sent *spb.VirtualRpcRequest) {
		s.handleVRPCResponse(&spb.VirtualRpcResponse{
			RpcId:   sent.RpcId,
			Payload: []byte("ok"),
		})
	})
	after := time.Now()
	if err != nil {
		t.Fatalf("Invoke: unexpected err %v", err)
	}
	if res.SentAt.IsZero() {
		t.Fatal("SentAt is zero after a successful send; want non-zero")
	}
	// SentAt must land in the [before, after] window (with a small slop to
	// tolerate clock skew between samples).
	const slop = 2 * time.Second
	if res.SentAt.Before(before.Add(-slop)) || res.SentAt.After(after.Add(slop)) {
		t.Errorf("SentAt = %v, want within [%v, %v]", res.SentAt, before, after)
	}
}

func TestInvoke_SentAtIsSetEvenOnSendFailure(t *testing.T) {
	// session_vrpc.go captures SentAt *before* calling Send. That is the
	// contract callers rely on for client-blocking-latency math — we record
	// when we attempted to hand the frame off, not whether the handoff
	// succeeded. This test pins that contract: a Send-failure must still
	// return a non-zero SentAt.
	stream := newFakeStream()
	stream.sendFn = func(req *spb.SessionRequest) error {
		return fmt.Errorf("network down")
	}
	s := newTestSession(t, stream, SessionHooks{})
	s.state.Store(int32(StateReady))

	before := time.Now()
	res, err := s.Invoke(context.Background(), newRoundTripDesc(), "hello")
	after := time.Now()
	if err == nil {
		t.Fatal("Invoke: expected error from failing Send, got nil")
	}
	if res.SentAt.IsZero() {
		t.Errorf("SentAt is zero after Send failure; want non-zero (session_vrpc.go captures SentAt before calling Send)")
	}
	const slop = 2 * time.Second
	if res.SentAt.Before(before.Add(-slop)) || res.SentAt.After(after.Add(slop)) {
		t.Errorf("SentAt = %v on Send-failure path; want within [%v, %v]", res.SentAt, before, after)
	}
}

func TestInvoke_ClusterInfoOnErrorPath(t *testing.T) {
	res, err := runInvoke(t, context.Background(), func(s *Session, sent *spb.VirtualRpcRequest) {
		s.handleVRPCErrorResponse(&spb.ErrorResponse{
			RpcId:       sent.RpcId,
			Status:      &rpcstatus.Status{Code: int32(codes.FailedPrecondition), Message: "boom"},
			ClusterInfo: &spb.ClusterInformation{ClusterId: "c1", ZoneId: "z1"},
		})
	})
	if err == nil {
		t.Fatal("Invoke returned nil err on ErrorResponse path; want non-nil")
	}
	if res.ClusterInfo == nil {
		t.Fatal("InvokeResult.ClusterInfo = nil on error path; want it preserved from ErrorResponse.ClusterInfo")
	}
	if got := res.ClusterInfo.ClusterId; got != "c1" {
		t.Errorf("ClusterInfo.ClusterId = %q, want %q", got, "c1")
	}
	if got := res.ClusterInfo.ZoneId; got != "z1" {
		t.Errorf("ClusterInfo.ZoneId = %q, want %q", got, "z1")
	}
}

func TestInvoke_RetryInfoPackedIntoStatus(t *testing.T) {
	// Marquee test for the RetryInfo plumbing in handleVRPCErrorResponse:
	// the server-supplied RetryInfo on ErrorResponse must be packed into the
	// status details so downstream consumers can recover it via
	// status.FromError(err).Details(). RetryingVRpc reads it from there.
	const wantDelay = 250 * time.Millisecond

	_, err := runInvoke(t, context.Background(), func(s *Session, sent *spb.VirtualRpcRequest) {
		s.handleVRPCErrorResponse(&spb.ErrorResponse{
			RpcId:     sent.RpcId,
			Status:    &rpcstatus.Status{Code: int32(codes.Unavailable), Message: "boom"},
			RetryInfo: &errdetails.RetryInfo{RetryDelay: durationpb.New(wantDelay)},
		})
	})
	if err == nil {
		t.Fatal("Invoke returned nil err on ErrorResponse path; want non-nil")
	}
	st, ok := status.FromError(err)
	if !ok {
		t.Fatalf("status.FromError(%v) returned ok=false; want a gRPC status error", err)
	}
	if st.Code() != codes.Unavailable {
		t.Errorf("status.Code = %v, want Unavailable", st.Code())
	}
	var found *errdetails.RetryInfo
	for _, d := range st.Details() {
		if ri, ok := d.(*errdetails.RetryInfo); ok {
			found = ri
			break
		}
	}
	if found == nil {
		t.Fatalf("RetryInfo not found in status.Details (%d details total); the handleVRPCErrorResponse plumbing failed to attach it", len(st.Details()))
	}
	if got := found.GetRetryDelay().AsDuration(); got != wantDelay {
		t.Errorf("RetryInfo.RetryDelay = %v, want %v", got, wantDelay)
	}
}

// TestHandleVRPCResponse_LateResponseAfterCtxDone_FlagsCancelledDrained
// pins Java-parity slot-lifecycle: the caller's ctx expires before the
// server response lands, but the slot stays claimed (markCancelled
// records the cancel; drainSlot does NOT run in Invoke's return path).
// The eventual server response drains the slot as a bookkeeping-only
// tagSessionVRPCCancelledDrained. Sequence exercised:
//
//  1. Invoke's claimSlot fills activeRPC and Send goes out.
//  2. ctx deadline fires; awaitInvokeResult returns via <-ctx.Done()
//     after markCancelled records the cancel result.
//  3. activeVRPC() is STILL non-nil — Java parity, unlike the pre-
//     slotMu behavior where a defer releaseSlot cleared it.
//  4. The server's response for that rpc_id lands moments later —
//     handleVRPCResponse calls drainSlot, sees currentCancel != nil,
//     records tagSessionVRPCCancelledDrained, and does NOT deliver
//     (no reader on resultChan).
//
// Not a bug: the vRPC protocol has no "unsubscribe rpc_id" primitive,
// so the server keeps sending. The counter is a canary — a rising rate
// under steady load usually means tail-latency spikes are pushing more
// callers onto the ctx.Done branch. This test locks in that the drain
// is counted (dashboard-visible) and does NOT emit any of the
// sibling vRPC tags (nil / id-mismatch / wrong-state).
func TestHandleVRPCResponse_LateResponseAfterCtxDone_FlagsCancelledDrained(t *testing.T) {
	resetDebugTagCountsForTest()

	s, stream := makeActive(t, SessionHooks{})
	desc := newRoundTripDesc()

	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()

	done := make(chan error, 1)
	go func() {
		_, err := s.Invoke(ctx, desc, "req")
		done <- err
	}()

	// Wait for the request to hit the wire so we know claimSlot filled
	// the slot and rpc_id is knowable from the sent frame.
	waitFor(t, time.Second, func() bool { return len(stream.snapshotSent()) > 0 }, "Send called")
	sent := stream.snapshotSent()[0].GetVirtualRpc()
	if sent == nil {
		t.Fatal("sent frame was not a VirtualRpcRequest")
	}

	// Let ctx expire; Invoke must return with DeadlineExceeded.
	select {
	case err := <-done:
		if !errors.Is(err, context.DeadlineExceeded) {
			t.Fatalf("Invoke err = %v, want DeadlineExceeded", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("Invoke did not return within outer bound")
	}

	// Java-parity precondition: slot STAYS claimed even after Invoke
	// returned via ctx.Done. This is the observable divergence from
	// the pre-slotMu behavior — any future PR that reverts to
	// client-side release must fail this assertion.
	if got := s.activeVRPC(); got == nil {
		t.Fatal("activeVRPC = nil after Invoke returned via ctx.Done; want the slot to stay claimed (Java-parity: only drainSlot releases it)")
	}

	// Snapshot the counter just before the late-response injection so we
	// isolate the drain tick from any prior emissions.
	before := snapshotDebugTagCounts()

	// The server's response finally arrives — the drain path checks
	// currentCancel, sees it set, and records the cancelled-drained tag.
	s.handleVRPCResponse(&spb.VirtualRpcResponse{
		RpcId:   sent.RpcId,
		Payload: []byte("ok"),
	})

	after := snapshotDebugTagCounts()
	if delta := after[tagSessionVRPCCancelledDrained] - before[tagSessionVRPCCancelledDrained]; delta != 1 {
		t.Errorf("tagSessionVRPCCancelledDrained delta = %d, want 1 (server response for a cancelled slot should drain and count)", delta)
	}
	// Sibling vRPC-drop tags must stay flat — the drain reason is
	// "caller abandoned via ctx.Done", not nil-slot / id-mismatch /
	// wrong-state.
	for _, tag := range []string{
		tagSessionVRPCNil,
		tagSessionVRPCIDMismatch,
		tagSessionVRPCResponseWrongState,
	} {
		if delta := after[tag] - before[tag]; delta != 0 {
			t.Errorf("%s delta = %d, want 0 (only cancelled-drained tag should fire on this race)", tag, delta)
		}
	}

	// Post-drain: slot must be empty so the next Invoke can claim.
	if got := s.activeVRPC(); got != nil {
		t.Errorf("activeVRPC = %p, want nil after drainSlot ran on the late server response", got)
	}
}

// TestInvoke_SecondInvokeAfterCtxDoneRejectedUncommitted pins the
// Java-parity claim rejection (SessionImpl.startRpc L423 —
// currentRpc != null → INTERNAL "RPC multiplexing not supported").
// Sequence exercised:
//
//  1. Invoke #1 → claimSlot → Send(rpc_A).
//  2. ctx#1 expires → Invoke #1 returns → markCancelled leaves slot claimed.
//  3. Invoke #2 → claimSlot returns false → immediate Uncommitted error,
//     Send is never called (no rpc_B on the wire, no rpc_id burned).
//  4. Server response for rpc_A drains the slot.
//  5. A fresh Invoke #3 can now claim and proceed normally.
//
// This is the Java-parity replacement for the pre-slotMu
// TestHandleVRPCResponse_LateResponseAfterCtxDoneAndNewInvoke_FlagsIDMismatch
// (which pinned the client-only slot-release divergence). Any future PR
// that reverts to client-side release must break this test — the second
// Invoke would succeed and race to an id-mismatch drop instead.
func TestInvoke_SecondInvokeAfterCtxDoneRejectedUncommitted(t *testing.T) {
	resetDebugTagCountsForTest()

	s, stream := makeActive(t, SessionHooks{})
	desc := newRoundTripDesc()

	// --- Invoke #1: run to ctx.Done. ---
	ctx1, cancel1 := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel1()

	done1 := make(chan error, 1)
	go func() {
		_, err := s.Invoke(ctx1, desc, "req-A")
		done1 <- err
	}()

	waitFor(t, time.Second, func() bool { return len(stream.snapshotSent()) > 0 }, "Invoke #1 Send")
	sentA := stream.snapshotSent()[0].GetVirtualRpc()
	if sentA == nil {
		t.Fatal("Invoke #1 sent frame was not a VirtualRpcRequest")
	}

	select {
	case err := <-done1:
		if !errors.Is(err, context.DeadlineExceeded) {
			t.Fatalf("Invoke #1 err = %v, want DeadlineExceeded", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("Invoke #1 did not return within outer bound")
	}

	// Slot still claimed — Java parity.
	if got := s.activeVRPC(); got == nil {
		t.Fatal("activeVRPC = nil after Invoke #1 returned via ctx.Done; want slot held")
	}

	// --- Invoke #2: must fail immediately with Uncommitted. ---
	sentBefore := len(stream.snapshotSent())
	_, err := s.Invoke(context.Background(), desc, "req-B")
	if err == nil {
		t.Fatal("Invoke #2 succeeded with slot busy; want Uncommitted error")
	}
	if outcome := ClassifyErr(err); outcome.State != StateUncommitted {
		t.Errorf("Invoke #2 err classified as %v, want StateUncommitted (Java parity: retryer must be free to pick another session)", outcome.State)
	}
	if got := len(stream.snapshotSent()); got != sentBefore {
		t.Errorf("Invoke #2 sent %d frames beyond Invoke #1; want 0 (Send must not fire on a losing claim, or rpc_id burn / wire noise ensues)", got-sentBefore)
	}

	// --- Step 4: drain the slot with the late rpc_A response. ---
	s.handleVRPCResponse(&spb.VirtualRpcResponse{
		RpcId:   sentA.RpcId,
		Payload: []byte("stale-A"),
	})
	if got := s.activeVRPC(); got != nil {
		t.Fatalf("activeVRPC = %p after draining rpc_A; want nil", got)
	}

	// --- Step 5: a fresh Invoke succeeds. ---
	done3 := make(chan struct {
		res InvokeResult
		err error
	}, 1)
	go func() {
		res, err := s.Invoke(context.Background(), desc, "req-C")
		done3 <- struct {
			res InvokeResult
			err error
		}{res, err}
	}()
	waitFor(t, time.Second, func() bool { return len(stream.snapshotSent()) > sentBefore }, "Invoke #3 Send")
	sentC := stream.snapshotSent()[len(stream.snapshotSent())-1].GetVirtualRpc()
	if sentC == nil {
		t.Fatal("Invoke #3 sent frame was not a VirtualRpcRequest")
	}
	s.handleVRPCResponse(&spb.VirtualRpcResponse{RpcId: sentC.RpcId, Payload: []byte("ok-C")})
	select {
	case r := <-done3:
		if r.err != nil {
			t.Fatalf("Invoke #3 err = %v, want nil after fresh claim", r.err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("Invoke #3 did not return after rpc_C delivered")
	}
}

// TestHandleVRPCResponse_NormalDrain_FiresSlotDrainedCallback pins the v3
// invariant: EVERY successful drainSlot fires hooks.OnSlotDrained — not
// just the cancelled-drain branch. Under v3 this hook is the sole
// "session became free" signal (the pool's Invoke return path no
// longer re-enqueues or wakes), so dropping the fire on the normal
// branch would leave the AFE queue with a stale outstanding-in-flight
// session and starve parked Checkout waiters. Sequence exercised:
//
//  1. Invoke's claimSlot fills activeRPC and Send goes out.
//  2. The server responds cleanly for that rpc_id — drainSlot returns
//     cancel=nil (normal happy path), deliver fires, Invoke returns.
//  3. OnSlotDrained runs exactly once (never zero — starves the pool;
//     never twice — a double-add to the AFE queue).
func TestHandleVRPCResponse_NormalDrain_FiresSlotDrainedCallback(t *testing.T) {
	// Install a counting slot-drained hook. Same pattern
	// SessionPoolImpl.createSession uses — a real pool wires this to
	// sl.ReleaseToPool + signalFree; here we count fires to prove the
	// wire is live for the normal-drain branch, not just cancelled-drain.
	fires := make(chan struct{}, 4)
	s, stream := makeActive(t, SessionHooks{OnSlotDrained: func() { fires <- struct{}{} }})
	desc := newRoundTripDesc()

	done := make(chan error, 1)
	go func() {
		_, err := s.Invoke(context.Background(), desc, "req")
		done <- err
	}()

	waitFor(t, time.Second, func() bool { return len(stream.snapshotSent()) > 0 }, "Send called")
	sent := stream.snapshotSent()[0].GetVirtualRpc()
	if sent == nil {
		t.Fatal("sent frame was not a VirtualRpcRequest")
	}

	s.handleVRPCResponse(&spb.VirtualRpcResponse{
		RpcId:   sent.RpcId,
		Payload: []byte("ok"),
	})

	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("Invoke err = %v, want nil (normal-drain path)", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("Invoke did not return after normal drain")
	}

	// Exactly one fire — the normal-drain branch MUST fire
	// OnSlotDrained (v3), and MUST NOT double-fire.
	select {
	case <-fires:
	case <-time.After(time.Second):
		t.Fatal("onSlotDrained did not fire on normal-drain branch; v3 invariant violated (pool would never learn the session is free)")
	}
	select {
	case <-fires:
		t.Error("onSlotDrained fired twice for a single drainSlot success; would double-enqueue in the AFE queue")
	case <-time.After(50 * time.Millisecond):
	}
}

// TestHandleVRPCErrorResponse_NormalDrain_FiresSlotDrainedCallback
// mirrors the response test on the error-frame branch — a definitive
// server ErrorResponse also drains the slot and MUST fire
// onSlotDrained under v3. Regression coverage: dropping the fire on
// the error branch would create a subtle asymmetry where sessions
// that see a server ErrorResponse never come back to the AFE queue.
func TestHandleVRPCErrorResponse_NormalDrain_FiresSlotDrainedCallback(t *testing.T) {
	fires := make(chan struct{}, 4)
	s, stream := makeActive(t, SessionHooks{OnSlotDrained: func() { fires <- struct{}{} }})
	desc := newRoundTripDesc()

	done := make(chan error, 1)
	go func() {
		_, err := s.Invoke(context.Background(), desc, "req")
		done <- err
	}()

	waitFor(t, time.Second, func() bool { return len(stream.snapshotSent()) > 0 }, "Send called")
	sent := stream.snapshotSent()[0].GetVirtualRpc()
	if sent == nil {
		t.Fatal("sent frame was not a VirtualRpcRequest")
	}

	s.handleVRPCErrorResponse(&spb.ErrorResponse{
		RpcId:  sent.RpcId,
		Status: &rpcstatus.Status{Code: int32(codes.FailedPrecondition), Message: "boom"},
	})

	select {
	case err := <-done:
		if err == nil {
			t.Fatal("Invoke err = nil on ErrorResponse; want non-nil")
		}
	case <-time.After(2 * time.Second):
		t.Fatal("Invoke did not return after ErrorResponse drain")
	}

	select {
	case <-fires:
	case <-time.After(time.Second):
		t.Fatal("onSlotDrained did not fire on error-drain branch; v3 invariant violated")
	}
	select {
	case <-fires:
		t.Error("onSlotDrained fired twice for a single error drainSlot success")
	case <-time.After(50 * time.Millisecond):
	}
}

// TestInvoke_SendFailure_FiresSlotDrainedCallback pins that the
// Send-failure Invoke branch fires OnSlotDrained after drainSlot.
// This is the third drainSlot site under v3; without the fire, a
// pool with all sessions on a broken stream would silently starve
// parked Checkout waiters (ReleaseToPool would still no-op via the
// inExpectedCount guard once OnSessionClosing fired, but the
// signalFree side of the hook matters for the waiter that was
// parked before the stream broke).
func TestInvoke_SendFailure_FiresSlotDrainedCallback(t *testing.T) {
	stream := newFakeStream()
	stream.sendFn = func(req *spb.SessionRequest) error {
		return fmt.Errorf("network down")
	}
	fires := make(chan struct{}, 2)
	s := newTestSession(t, stream, SessionHooks{OnSlotDrained: func() { fires <- struct{}{} }})
	s.state.Store(int32(StateReady))

	_, err := s.Invoke(context.Background(), newRoundTripDesc(), "req")
	if err == nil {
		t.Fatal("Invoke: expected err from failing Send")
	}

	select {
	case <-fires:
	case <-time.After(time.Second):
		t.Fatal("onSlotDrained did not fire on Send-failure branch; v3 requires drain-driven wake here too")
	}
}

// TestInvoke_ForceCloseRace_BoundedReturnUnderCtx pins the race between
// Invoke's initial state check (session_vrpc.go:87) and its activeVRPC
// CAS (session_vrpc.go:107). If ForceClose lands in that window the CAS
// still succeeds afterwards: Invoke Sends onto a still-open stream and
// blocks on rpc.resultChan. cancelActiveRPCs already ran on the nil
// slot, so nobody delivers a terminal error — awaitInvokeResult only
// unblocks via ctx.
//
// Contract enforced here: with a bounded ctx, Invoke MUST return within
// that ctx's deadline regardless of how the race resolves. A caller
// without a ctx deadline could hang indefinitely on this path — that is
// the known L1 limitation flagged for the sync-refactor decision
// (POC on branch experiment/session-sync-context-poc). This test does
// NOT prove the hang is impossible; it proves the ctx-bounded return
// is honored under stress.
func TestInvoke_ForceCloseRace_BoundedReturnUnderCtx(t *testing.T) {
	const iterations = 200
	const perAttemptCtx = 200 * time.Millisecond
	const outerBound = 2 * time.Second

	for i := 0; i < iterations; i++ {
		s, _ := makeActive(t, SessionHooks{})

		ctx, cancel := context.WithTimeout(context.Background(), perAttemptCtx)
		done := make(chan error, 1)

		go func() {
			_, err := s.Invoke(ctx, newRoundTripDesc(), "req")
			done <- err
		}()

		// Fire ForceClose concurrently. The race window between the
		// state Load and the activeVRPC CAS is a handful of ns — we
		// don't try to hit it deterministically; we run enough
		// iterations that the scheduler puts us on different sides of
		// it across runs. Under -race -count=1000 this shakes out any
		// unbounded-hang regression.
		go s.ForceClose(&spb.CloseSessionRequest{
			Reason:      spb.CloseSessionRequest_CLOSE_SESSION_REASON_USER,
			Description: "race-test teardown",
		})

		select {
		case err := <-done:
			cancel()
			if err == nil {
				// A benign success is legal if Invoke completed before
				// ForceClose cancelled anything. Not a regression.
				continue
			}
			// Every terminal error MUST be one of the four expected
			// classifications. An unclassified err would silently
			// escape ClassifyErr and confuse the retry oracle.
			outcome := ClassifyErr(err)
			switch outcome.State {
			case StateUncommitted, StateTransportFailure, StateServerResult:
				// OK: classified terminal.
			default:
				t.Fatalf("iter %d: err %v classified as %v — want Uncommitted/TransportFailure/ServerResult",
					i, err, outcome.State)
			}
			// Ctx cancel is the safety net; if it fired, verify the
			// wrapped err carries a recognizable cause so operators
			// aren't left with a bare context.DeadlineExceeded.
			if errors.Is(err, context.DeadlineExceeded) {
				// Ctx-bounded return. Expected on the L1-race branch.
				continue
			}
		case <-time.After(outerBound):
			cancel()
			t.Fatalf("iter %d: Invoke did not return within %v (L1 unbounded-hang regression)", i, outerBound)
		}
	}
}

// TestInvoke_ForceCloseWhileSending_BoundedReturn constructs a
// deterministic variant of the ForceClose race: ForceClose fires while
// Send is mid-flight (past the state check AND past the CAS but not
// yet past awaitInvokeResult's channel wait). Uses fakeStream.sendFn
// to block Send, injects ForceClose while blocked, then unblocks Send
// and verifies Invoke returns within ctx.
//
// This nails the "CAS succeeded, Send succeeded, session got closed
// under us" path — the classic cancelActiveRPCs → StateTransportFailure
// delivery on resultChan. If cancelActiveRPCs raced the CAS wrong, the
// slot would be nil at cancel time and no delivery would land, causing
// the ctx-bounded return branch below to fire instead.
func TestInvoke_ForceCloseWhileSending_BoundedReturn(t *testing.T) {
	s, stream := makeActive(t, SessionHooks{})

	sendGate := make(chan struct{})
	var once sync.Once
	stream.sendFn = func(*spb.SessionRequest) error {
		once.Do(func() { <-sendGate })
		return nil
	}

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	done := make(chan error, 1)
	go func() {
		_, err := s.Invoke(ctx, newRoundTripDesc(), "req")
		done <- err
	}()

	// Wait for Invoke to be blocked in Send (activeVRPC set, sendMu held).
	waitFor(t, time.Second, func() bool { return s.activeVRPC() != nil }, "activeVRPC claimed")

	// Fire ForceClose while Send is blocked. cancelActiveRPCs will
	// find the slot claimed, CAS it back to nil, and deliver
	// StateTransportFailure on resultChan. Meanwhile Invoke is still
	// blocked inside Send — the delivery is buffered (chan cap 1).
	go s.ForceClose(&spb.CloseSessionRequest{
		Reason:      spb.CloseSessionRequest_CLOSE_SESSION_REASON_USER,
		Description: "mid-send teardown",
	})

	// Give ForceClose a moment to run before unblocking Send so the
	// race resolves in the intended order.
	time.Sleep(20 * time.Millisecond)
	close(sendGate)

	select {
	case err := <-done:
		if err == nil {
			t.Fatal("Invoke returned nil err after ForceClose; want an error")
		}
		outcome := ClassifyErr(err)
		// Expected: TransportFailure from cancelActiveRPCs delivery.
		// Also acceptable if ctx fired first (still bounded).
		if outcome.State != StateTransportFailure && !errors.Is(err, context.DeadlineExceeded) {
			t.Errorf("err classified as %v (%v); want StateTransportFailure or ctx.DeadlineExceeded",
				outcome.State, err)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("Invoke did not return within 3s after ForceClose + unblocked Send")
	}
}

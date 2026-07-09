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
	"fmt"
	"time"

	spb "cloud.google.com/go/bigtable/apiv2/bigtablepb"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/durationpb"
	"google.golang.org/protobuf/types/known/timestamppb"
)

// InvokeResult carries the full set of outputs from a single Invoke call.
//
// Fields:
//   - Response: decoded vRPC payload (typed per VRpcDescriptor.Decode); nil on error.
//   - ClusterInfo: server-reported routing/cluster identity; may be set on
//     both success and error paths if the server included it.
//   - Stats: server-reported per-request statistics (notably BackendLatency);
//     nil if the server did not populate Stats on the success frame.
//   - SentAt: local monotonic timestamp captured immediately before the vRPC
//     frame was handed to the bidi Send. Used downstream to derive
//     client-side blocking latency (sentAt - attemptStart).
//
// ErrorResponse.RetryInfo from the server is plumbed via the returned error
// using gRPC status details — callers can extract it with
// status.FromError(err).Details() and type-asserting to *errdetails.RetryInfo
// (this is exactly how RetryingVRpc already consumes it).
type InvokeResult struct {
	Response    interface{}
	ClusterInfo *spb.ClusterInformation
	Stats       *spb.SessionRequestStats
	SentAt      time.Time
	// PeerInfo is the serving session's parsed peer info (from the
	// bigtable-peer-info header the server sent on session open). Bound to
	// the session, not this specific vRPC — every InvokeResult on the same
	// session carries the same pointer. Populated by SessionPoolImpl.Invoke;
	// nil when the pool bypassed a session (checkout failure). Feeds the
	// outer metrics tracer's transport_type / transport_region /
	// transport_zone / transport_subzone attributes on attempt_latencies2.
	PeerInfo *spb.PeerInfo
	// RpcIDOnSession is the per-session monotonic id of this call
	// (1, 2, 3, …). Distinguishes warm-up vRPCs (small id) from
	// established-session vRPCs.
	RpcIDOnSession int64
	// TransportLatency is the time between the vRPC frame being handed
	// to the bidi Send and the response (or server-side error) arriving
	// on the stream. Approximates network RTT + server queue + Backend;
	// (TransportLatency - BackendLatency) surfaces "everything except
	// server processing". Zero when Invoke returned before a Recv event
	// (context cancellation or pre-Send failure).
	TransportLatency time.Duration
}

// Invoke executes a single virtual RPC on this session and returns every
// observable output of the roundtrip — decoded response, cluster info,
// server-reported Stats, and the local SentAt timestamp — so callers can
// populate metrics (client_blocking_latency, server_backend_latency) and
// respect server-supplied retry hints without losing data on the way out of
// the transport.
//
// The caller MUST have exclusive access to this Session for the duration
// of the call — the pool guarantees this via CheckoutSession's per-session
// idle-slot gate. The single-in-flight invariant is enforced with a
// CompareAndSwap on activeRPC; a failing CAS means the caller bypassed
// the pool gate and is a programming error, not a runtime condition. This
// replaces a golang.org/x/sync/semaphore that added two channel ops per
// call on the hot path.
func (s *Session) Invoke(ctx context.Context, desc VRpcDescriptor, req interface{}) (result InvokeResult, err error) {
	startTime := time.Now()

	if st := State(s.state.Load()); st != StateReady {
		return result, tagErr(StateUncommitted,
			unavailable(ErrSessionNotActive, "session is not active (state: %v)", st))
	}

	reqBytes, err := desc.Encode(req)
	if err != nil {
		return result, tagErr(StateUncommitted, fmt.Errorf("encode vRPC request: %w", err))
	}

	rpcID := s.nextRPCID.Add(1)
	result.RpcIDOnSession = rpcID
	rpc := &vrpcImpl{
		id:         rpcID,
		method:     desc.Method(),
		resultChan: make(chan vrpcResult, 1),
	}

	// Claim the single in-flight slot. See method doc — a losing CAS is
	// a caller-side serialization bug, not a runtime backoff condition.
	if !s.activeRPC.CompareAndSwap(nil, rpc) {
		return result, tagErr(StateUncommitted,
			unavailable(ErrSessionNotActive,
				"concurrent Invoke on session %q: multiPlexingLimit=1 violated", s.logName))
	}
	defer s.releaseSlot(rpc)

	// Reset the heartbeat deadline whenever we send an outbound frame: the
	// server's keepalive clock is implicitly reset by our activity.
	s.resetHeartbeatDeadline()

	attempt := currentAttempt(ctx)
	if attempt > 1 {
		s.noteRetryAttempt(ctx, desc.Method(), attempt)
	}
	sessionReq := buildInvokeRequest(rpcID, reqBytes, attempt, startTime, ctx)

	// Capture SentAt immediately before the frame is handed to Send so
	// downstream metrics can compute client-side blocking latency as
	// (SentAt - attemptStart) without double-counting encode/setup overhead.
	result.SentAt = time.Now()
	if err := s.Send(sessionReq); err != nil {
		// Send returning error means the frame did not reach the wire,
		// so the server saw nothing. StateUncommitted lets the retry
		// interceptor retry even non-idempotent ops (Java parity —
		// java-bigtable classifies pre-wire Send errors as Uncommitted).
		return result, tagErr(StateUncommitted, fmt.Errorf("send vRPC request: %w", err))
	}

	err = s.awaitInvokeResult(ctx, rpc, desc, &result)
	return result, err
}

// releaseSlot clears the active-RPC slot and signals quiescence when the
// session is closing. Runs from Invoke's defer; safe to call after any
// completion path (success, error, cancel, panic).
//
// Order matters: clear the slot first, THEN check state. Close()
// transitions to StateClosing first and only then observes activeRPC —
// so at least one side signals. signalQuiescent is once-guarded, so a
// double-signal is harmless.
func (s *Session) releaseSlot(rpc *vrpcImpl) {
	s.activeRPC.CompareAndSwap(rpc, nil)
	if State(s.state.Load()) == StateClosing {
		s.signalQuiescent()
	}
}

// currentAttempt reads the retry-interceptor's attempt tag from ctx. Calls
// that bypass RetryingVRpc have no tag; treat them as attempt 1.
func currentAttempt(ctx context.Context) int64 {
	if a := int64(VRpcAttempt(ctx)); a > 0 {
		return a
	}
	return 1
}

// noteRetryAttempt bumps the per-session retries counter and — if the
// retry interceptor stashed the prior attempt's error on ctx — emits a
// debug + sessionz event tagged with the prior gRPC code so operators
// can trace WHY the retry fired without cross-referencing logs.
func (s *Session) noteRetryAttempt(ctx context.Context, method string, attempt int64) {
	s.retries.Add(1)
	prev := PrevAttemptErr(ctx)
	if prev == nil {
		return
	}
	prevCode := status.Code(prev).String()
	s.debugf("retry attempt=%d method=%s prev_code=%s prev_err=%v",
		attempt, method, prevCode, prev)
	s.recordEvent("retry", "attempt=%d method=%s prev_code=%s prev_err=%v",
		attempt, method, prevCode, prev)
}

// buildInvokeRequest constructs the wire-level SessionRequest envelope
// for a single vRPC attempt. Pure — no I/O, no side effects. The
// VirtualRpc.Deadline field carries the remaining budget as a duration
// so the server measures from receive time rather than an absolute wall
// clock. Omitted when ctx has no deadline or the budget is already
// non-positive (the client-side ctx.Done branch will fire immediately).
func buildInvokeRequest(rpcID int64, reqBytes []byte, attempt int64, startTime time.Time, ctx context.Context) *spb.SessionRequest {
	virtRpc := &spb.VirtualRpcRequest{
		RpcId:   rpcID,
		Payload: reqBytes,
		Metadata: &spb.VirtualRpcRequest_Metadata{
			AttemptNumber: attempt,
			AttemptStart:  timestamppb.New(startTime),
		},
	}
	if deadline, ok := ctx.Deadline(); ok {
		if remaining := time.Until(deadline); remaining > 0 {
			virtRpc.Deadline = durationpb.New(remaining)
		}
	}
	return &spb.SessionRequest{
		Payload: &spb.SessionRequest_VirtualRpc{VirtualRpc: virtRpc},
	}
}

// awaitInvokeResult blocks until either ctx cancels or the server
// delivers a result on rpc.resultChan. Populates the tail of result
// (TransportLatency, ClusterInfo, Stats, Response) and returns a
// fully-tagged error on failure. res.err from resultChan is already
// tagged StateTransportFailure at the source (cancelActiveRPCs); server
// error frames arrive as res.errResp and are unpacked here into a
// StateServerResult-tagged error.
func (s *Session) awaitInvokeResult(ctx context.Context, rpc *vrpcImpl, desc VRpcDescriptor, result *InvokeResult) error {
	select {
	case <-ctx.Done():
		s.recordCtxDone(ctx, rpc, desc.Method(), result.SentAt)
		return tagErr(StateTransportFailure, ctx.Err())
	case res := <-rpc.resultChan:
		result.TransportLatency = time.Since(result.SentAt)
		ci := res.ClusterInfo()
		result.ClusterInfo = ci
		if ci != nil {
			s.recordCluster(ci.GetClusterId())
		}
		if res.err != nil {
			return res.err
		}
		if res.errResp != nil {
			return tagErr(StateServerResult, errorResponseToErr(res.errResp))
		}
		if res.resp == nil {
			// deliver() only publishes a vrpcResult when at least one of
			// {resp, errResp, err} is set; a zero-value slip through here
			// is a bookkeeping bug rather than a server-side event.
			return tagErr(StateServerResult, fmt.Errorf("internal: received empty vRPC result"))
		}
		if res.resp.RpcId != rpc.id {
			// This is a bookkeeping bug: deliver() only publishes into
			// resultChan for the matching activeRPC, so a mismatch here
			// means state got out from under us. Counter separate from
			// handleVRPC*'s id-mismatch tag so we can distinguish "wrong
			// response reached deliver" from "wrong response dropped
			// earlier."
			recordDebugTagAt(lvl.Error, tagSessionVRPCIDMismatch)
			return tagErr(StateServerResult,
				fmt.Errorf("internal: response RpcId %d does not match request RpcId %d", res.resp.RpcId, rpc.id))
		}
		respMsg, decodeErr := desc.Decode(res.resp.Payload)
		if decodeErr != nil {
			return tagErr(StateServerResult, fmt.Errorf("decode vRPC response: %w", decodeErr))
		}
		result.Response = respMsg
		result.Stats = res.resp.Stats
		if res.resp.Stats != nil && res.resp.Stats.BackendLatency != nil {
			s.recordLatency(res.resp.Stats.BackendLatency.AsDuration())
		}
		return nil
	}
}

// recordCtxDone emits the debug + sessionz event for a ctx cancellation
// or deadline fire while a vRPC was in flight. Captures whether the RPC
// was still holding the slot at cancel time — useful for spotting races
// between our cancel and a late server response.
func (s *Session) recordCtxDone(ctx context.Context, rpc *vrpcImpl, method string, sentAt time.Time) {
	stillActive := s.activeRPC.Load() == rpc
	sessState := State(s.state.Load())
	waited := time.Since(sentAt)
	peer := s.peerInfoSummary()
	s.debugf("vRPC %s rpc_id=%d ctx.Done waited=%v err=%v session_state=%v still_in_flight=%v %s",
		method, rpc.id, waited, ctx.Err(), sessState, stillActive, peer)
	s.recordEvent("ctx-done", "method=%s rpc_id=%d waited=%v err=%v session_state=%v still_in_flight=%v %s",
		method, rpc.id, waited, ctx.Err(), sessState, stillActive, peer)
}

// handleVRPCResponse delivers a server VirtualRpcResponse to the waiting
// Invoke caller, if any.
func (s *Session) handleVRPCResponse(resp *spb.VirtualRpcResponse) {
	if resp == nil {
		return
	}
	// A vRPC response is only expected while the session is Ready or
	// Closing (drain window). Any other state means either a bug in
	// state tracking or a server retransmit after teardown — drop.
	st := s.State()
	if !assertDebugTagf(st == StateReady || st == StateClosing, tagSessionVRPCResponseWrongState,
		"vRPC response for rpc_id=%d arrived in state %s", resp.RpcId, st) {
		return
	}
	rpc := s.activeRPC.Load()
	if rpc == nil {
		recordDebugTag(tagSessionVRPCNil)
		s.debugf("dropping VirtualRpcResponse for rpc_id=%d — no in-flight RPC tracked", resp.RpcId)
		return
	}
	if rpc.id != resp.RpcId {
		recordDebugTag(tagSessionVRPCIDMismatch)
		s.debugf("dropping VirtualRpcResponse rpc_id=%d != in-flight rpc_id=%d", resp.RpcId, rpc.id)
		return
	}
	s.okRpcs.Add(1)
	if !s.deliver(rpc, vrpcResult{resp: resp}) {
		recordDebugTag(tagSessionVRPCDuplicateResult)
	}
}

// handleVRPCErrorResponse routes per-vRPC errors to the waiting caller.
// Session-level errors (rpc_id == 0) are handled in handleSessionResponse.
func (s *Session) handleVRPCErrorResponse(errResp *spb.ErrorResponse) {
	if errResp == nil {
		return
	}
	st := s.State()
	if !assertDebugTagf(st == StateReady || st == StateClosing, tagSessionVRPCResponseWrongState,
		"vRPC error for rpc_id=%d arrived in state %s", errResp.RpcId, st) {
		return
	}
	rpc := s.activeRPC.Load()
	if rpc == nil {
		recordDebugTag(tagSessionVRPCErrorNil)
		s.debugf("dropping ErrorResponse for rpc_id=%d — no in-flight RPC tracked", errResp.RpcId)
		return
	}
	if rpc.id != errResp.RpcId {
		recordDebugTag(tagSessionVRPCIDMismatch)
		s.debugf("dropping ErrorResponse rpc_id=%d != in-flight rpc_id=%d", errResp.RpcId, rpc.id)
		return
	}
	s.errorRpcs.Add(1)
	if !s.deliver(rpc, vrpcResult{errResp: errResp}) {
		recordDebugTag(tagSessionVRPCDuplicateResult)
	}
}

// errorResponseToErr converts a server ErrorResponse frame into a Go
// error carrying the gRPC status code and any RetryInfo the server
// attached. RetryInfo is packed into the status details so downstream
// consumers (notably RetryingVRpc) can recover it via status.FromError.
// WithDetails returns a fresh *Status on success; on the rare failure
// (e.g. anypb marshal) we fall back to the bare status so the error path
// still propagates the server's code.
func errorResponseToErr(errResp *spb.ErrorResponse) error {
	if errResp.Status == nil {
		return fmt.Errorf("unknown vRPC error (rpc_id=%d)", errResp.RpcId)
	}
	st := status.FromProto(errResp.Status)
	if errResp.RetryInfo != nil {
		if withDetails, derr := st.WithDetails(errResp.RetryInfo); derr == nil {
			st = withDetails
		}
	}
	return st.Err()
}

// deliver writes a result onto the RPC's buffered (cap 1) channel and
// returns true. Returns false if the slot is already full (a duplicate
// frame or a cancel racing a completion) — the first wins, subsequent
// ones are dropped. Every default-branch drop is stamped into the
// per-session event ring ("dup-deliver") so operators can see the race
// in sessionz regardless of which caller lost. Callers on the
// server-frame path additionally tag it as a metric
// (tagSessionVRPCDuplicateResult); cancelActiveRPCs ignores false
// because a filled slot means the completion already landed and no
// metric warning is warranted.
func (s *Session) deliver(rpc *vrpcImpl, res vrpcResult) bool {
	select {
	case rpc.resultChan <- res:
		return true
	default:
		s.debugf("duplicate result for rpc_id=%d (%s) dropped", rpc.id, rpc.method)
		s.recordEvent("dup-deliver", "rpc_id=%d method=%s dropped (channel full)", rpc.id, rpc.method)
		return false
	}
}

// cancelActiveRPCs cancels the in-flight vRPC (if any) with the given
// error. With multiPlexingLimit=1 there is at most one such vRPC.
// Clear-then-deliver so a racing handleVRPCResponse can't double-deliver
// on the same slot.
func (s *Session) cancelActiveRPCs(err error) {
	rpc := s.activeRPC.Load()
	if rpc == nil {
		return
	}
	if !s.activeRPC.CompareAndSwap(rpc, nil) {
		// Concurrent completion cleared the slot; the caller already
		// received a result. Nothing to cancel.
		return
	}
	// Session-side cancellation: session died / GoAway / heartbeat missed
	// / benign shutdown while an RPC was in-flight. Server may or may not
	// have processed — TransportFailure classification lets idempotent ops
	// retry and prevents non-idempotent ones from double-applying.
	s.deliver(rpc, vrpcResult{err: tagErr(StateTransportFailure, err)})
}

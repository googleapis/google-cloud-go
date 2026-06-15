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
}

// Invoke executes a single virtual RPC on this session and returns every
// observable output of the roundtrip — decoded response, cluster info,
// server-reported Stats, and the local SentAt timestamp — so callers can
// populate metrics (client_blocking_latency, server_backend_latency) and
// respect server-supplied retry hints without losing data on the way out of
// the transport.
//
// Calls are serialized by vrpcSem; concurrent callers queue behind the
// in-flight RPC until the semaphore is released.
func (s *Session) Invoke(ctx context.Context, desc VRpcDescriptor, req interface{}) (result InvokeResult, err error) {
	if err := s.vrpcSem.Acquire(ctx, multiPlexingLimit); err != nil {
		return InvokeResult{}, err
	}
	defer s.vrpcSem.Release(multiPlexingLimit)

	startTime := time.Now()
	// TODO: defer tracer.recordOperation(ctx, startTime, desc.Method(), err)
	// once sessionTracer lands.

	s.mu.Lock()
	if s.state != StateActive {
		st := s.state
		s.mu.Unlock()
		return InvokeResult{}, unavailable(ErrSessionNotActive, "session is not active (state: %v)", st)
	}
	s.mu.Unlock()

	reqBytes, err := desc.Encode(req)
	if err != nil {
		return InvokeResult{}, fmt.Errorf("encode vRPC request: %w", err)
	}

	rpcID := s.nextRPCID.Add(1)
	rpc := &vrpcImpl{
		id:         rpcID,
		method:     desc.Method(),
		resultChan: make(chan vrpcResult, 1),
	}

	s.mu.Lock()
	s.activeRPCs[rpcID] = rpc
	s.mu.Unlock()
	defer func() {
		s.mu.Lock()
		delete(s.activeRPCs, rpcID)
		drained := s.state == StateClosing && len(s.activeRPCs) == 0
		s.mu.Unlock()
		if drained {
			s.signalQuiescent()
		}
	}()

	// Reset the heartbeat deadline whenever we send an outbound frame: the
	// server's keepalive clock is implicitly reset by our activity.
	s.resetHeartbeatDeadline()

	attempt := int64(VRpcAttempt(ctx))
	if attempt == 0 {
		// Calls bypassing the retry interceptor have no attempt set in the
		// context; treat them as the first attempt.
		attempt = 1
	}
	virtRpc := &spb.VirtualRpcRequest{
		RpcId:   rpcID,
		Payload: reqBytes,
		Metadata: &spb.VirtualRpcRequest_Metadata{
			AttemptNumber: attempt,
			AttemptStart:  timestamppb.New(startTime),
		},
	}
	if dl, ok := ctx.Deadline(); ok {
		if remaining := time.Until(dl); remaining > 0 {
			virtRpc.Deadline = durationpb.New(remaining)
		}
	}
	sessionReq := &spb.SessionRequest{
		Payload: &spb.SessionRequest_VirtualRpc{
			VirtualRpc: virtRpc,
		},
	}
	// Capture SentAt immediately before the frame is handed to Send so
	// downstream metrics can compute client-side blocking latency as
	// (SentAt - attemptStart) without double-counting encode/setup overhead.
	sentAt := time.Now()
	result.SentAt = sentAt
	if err := s.Send(sessionReq); err != nil {
		return result, fmt.Errorf("send vRPC request: %w", err)
	}

	select {
	case <-ctx.Done():
		return result, ctx.Err()
	case res := <-rpc.resultChan:
		result.ClusterInfo = res.clusterInfo
		if res.err != nil {
			return result, res.err
		}
		if res.resp.RpcId != rpcID {
			return result, fmt.Errorf("internal: response RpcId %d does not match request RpcId %d", res.resp.RpcId, rpcID)
		}
		respMsg, decodeErr := desc.Decode(res.resp.Payload)
		if decodeErr != nil {
			return result, fmt.Errorf("decode vRPC response: %w", decodeErr)
		}
		result.Response = respMsg
		result.Stats = res.resp.Stats
		return result, nil
	}
}

// handleVRPCResponse delivers a server VirtualRpcResponse to the waiting
// Invoke caller, if any.
func (s *Session) handleVRPCResponse(resp *spb.VirtualRpcResponse) {
	s.mu.Lock()
	rpc, ok := s.activeRPCs[resp.RpcId]
	if ok {
		s.hasOkRpcs = true
	}
	s.mu.Unlock()

	if !ok {
		s.debugf("dropping VirtualRpcResponse for unknown rpc_id=%d", resp.RpcId)
		return
	}
	s.deliver(rpc, vrpcResult{resp: resp, clusterInfo: resp.ClusterInfo})
}

// handleVRPCErrorResponse routes per-vRPC errors to the waiting caller.
// Session-level errors (rpc_id == 0) are handled in handleSessionResponse.
func (s *Session) handleVRPCErrorResponse(errResp *spb.ErrorResponse) {
	s.mu.Lock()
	rpc, ok := s.activeRPCs[errResp.RpcId]
	if ok {
		s.hasErrorRpcs = true
	}
	s.mu.Unlock()

	if !ok {
		s.debugf("dropping ErrorResponse for unknown rpc_id=%d", errResp.RpcId)
		return
	}

	var goErr error
	if errResp.Status != nil {
		st := status.FromProto(errResp.Status)
		// If the server attached RetryInfo to the ErrorResponse envelope,
		// pack it into the status details so downstream consumers
		// (notably RetryingVRpc) can recover it via status.FromError(err).
		// .Details() — the same path they already use for inline retry
		// hints. WithDetails returns a fresh *Status on success; on the
		// rare failure (e.g. anypb marshal) we fall back to the bare
		// status so the error path still propagates the server's code.
		if errResp.RetryInfo != nil {
			if withDetails, derr := st.WithDetails(errResp.RetryInfo); derr == nil {
				st = withDetails
			}
		}
		goErr = st.Err()
	} else {
		goErr = fmt.Errorf("unknown vRPC error (rpc_id=%d)", errResp.RpcId)
	}
	s.deliver(rpc, vrpcResult{err: goErr, clusterInfo: errResp.ClusterInfo})
}

// deliver writes a result onto the RPC's buffered (cap 1) channel. The
// non-blocking send protects against duplicate server frames for the same
// rpc_id; the first wins, subsequent ones are dropped.
func (s *Session) deliver(rpc *vrpcImpl, res vrpcResult) {
	select {
	case rpc.resultChan <- res:
	default:
		s.debugf("duplicate result for rpc_id=%d (%s) dropped", rpc.id, rpc.method)
	}
}

// cancelActiveRPCs removes and notifies every in-flight RPC matching filter
// (or all, if filter is nil) with the given error.
func (s *Session) cancelActiveRPCs(err error, filter func(rpcID int64) bool) {
	s.mu.Lock()
	cancelled := make([]*vrpcImpl, 0, len(s.activeRPCs))
	for id, rpc := range s.activeRPCs {
		if filter == nil || filter(id) {
			cancelled = append(cancelled, rpc)
			delete(s.activeRPCs, id)
		}
	}
	s.mu.Unlock()

	for _, rpc := range cancelled {
		s.deliver(rpc, vrpcResult{err: err})
	}
}

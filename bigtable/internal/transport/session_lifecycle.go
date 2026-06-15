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
	"encoding/base64"
	"fmt"
	"time"

	spb "cloud.google.com/go/bigtable/apiv2/bigtablepb"
	"google.golang.org/protobuf/proto"
)

const peerInfoHeaderKey = "bigtable-peer-info"

// Start opens the session by sending OpenSessionRequest, then launches the
// read and heartbeat loops. ctx governs the loops; cancelling it forces the
// session closed. Unblocking the underlying Recv requires the caller to also
// cancel the stream's context.
func (s *Session) Start(ctx context.Context, req *spb.OpenSessionRequest) error {
	if prev, ok := s.transitionTo(StateStarting, isState(StateNew)); !ok {
		return fmt.Errorf("session already started or closed (state: %v)", prev)
	}

	openReq := &spb.SessionRequest{
		Payload: &spb.SessionRequest_OpenSession{OpenSession: req},
	}
	if err := s.Send(openReq); err != nil {
		s.ForceClose(&spb.CloseSessionRequest{
			Reason:      spb.CloseSessionRequest_CLOSE_SESSION_REASON_ERROR,
			Description: "failed to send open session request",
		})
		return fmt.Errorf("send open session request: %w", err)
	}

	go s.readLoop(ctx)
	go s.heartBeatLoop(ctx)

	s.hooks.onStart(ctx)
	return nil
}

// ForceClose immediately transitions the session to StateClosed and cancels
// every in-flight RPC. It is safe to call multiple times; only the first call
// fires listener/tracer callbacks.
func (s *Session) ForceClose(req *spb.CloseSessionRequest) {
	if _, ok := s.transitionTo(StateClosed, notState(StateClosed)); !ok {
		return
	}

	desc := "session force closed"
	if req != nil && req.Description != "" {
		desc = "session force closed: " + req.Description
	}
	s.cancelActiveRPCs(unavailable(closeReasonToCause(req), "%s", desc), nil)
	s.signalQuiescent()
	s.notifyClosed(nil)
}

// notifyClosed fires listener.OnClose exactly once over the lifetime of a
// Session. TODO: also call tracer.recordClose once sessionTracer lands.
func (s *Session) notifyClosed(streamErr error) {
	s.closeOnce.Do(func() {
		s.hooks.onClose(s, streamErr)
	})
}

// Close requests a graceful shutdown: it transitions to StateClosing, waits
// for in-flight RPCs to drain (or for ctx to fire), then sends
// CloseSessionRequest. The server's EOF eventually drives handleClose, which
// moves to StateClosed.
func (s *Session) Close(ctx context.Context, req *spb.CloseSessionRequest) error {
	if _, ok := s.transitionTo(StateClosing, isState(StateNew, StateStarting, StateActive)); !ok {
		return nil
	}

	// Wait for active RPCs to drain. The quiescent channel is closed by the
	// last RPC's cleanup defer when it sees state==Closing && empty, or by
	// ForceClose if it races us.
	s.mu.Lock()
	empty := len(s.activeRPCs) == 0
	s.mu.Unlock()
	if empty {
		s.signalQuiescent()
	} else {
		select {
		case <-s.quiescent:
		case <-ctx.Done():
			s.ForceClose(nil)
			return ctx.Err()
		}
	}

	// If ForceClose raced us during the drain, our work is done.
	if s.State() == StateClosed {
		return nil
	}

	closeReq := &spb.SessionRequest{
		Payload: &spb.SessionRequest_CloseSession{CloseSession: req},
	}
	if err := s.Send(closeReq); err != nil {
		s.ForceClose(nil)
		return fmt.Errorf("send close session request: %w", err)
	}
	return nil
}

// Send writes a SessionRequest under sendMu so concurrent producers don't
// corrupt the underlying stream.
func (s *Session) Send(req *spb.SessionRequest) error {
	s.sendMu.Lock()
	defer s.sendMu.Unlock()
	return s.stream.Send(req)
}

// readLoop drives the inbound side of the stream until Recv returns an error.
func (s *Session) readLoop(ctx context.Context) {
	// Extract peer info from the header metadata asynchronously so we don't
	// block reads on the header arriving.
	go func() {
		headerMD, err := s.stream.Header()
		if err != nil {
			s.debugf("stream Header() failed: %v", err)
			return
		}
		s.peerInfoExtracter(headerMD.Get(peerInfoHeaderKey))
	}()

	// Supervisor: if ctx is cancelled, mark the session closed so callers
	// observe state immediately. Unblocking Recv() requires the underlying
	// stream's context to be cancelled by the caller.
	readLoopDone := make(chan struct{})
	go func() {
		select {
		case <-ctx.Done():
			s.ForceClose(&spb.CloseSessionRequest{
				Reason:      spb.CloseSessionRequest_CLOSE_SESSION_REASON_USER,
				Description: "client context cancelled",
			})
		case <-readLoopDone:
		}
	}()
	defer close(readLoopDone)

	for {
		resp, err := s.stream.Recv()
		if err != nil {
			s.handleClose(err)
			return
		}
		s.handleSessionResponse(resp)
	}
}

// handleSessionResponse dispatches every SessionResponse oneof variant.
// Receiving any recognized frame resets the heartbeat watchdog; unknown
// frames do NOT, so a misbehaving server cannot keep the watchdog satisfied
// with junk payloads.
func (s *Session) handleSessionResponse(resp *spb.SessionResponse) {
	switch p := resp.GetPayload().(type) {
	case *spb.SessionResponse_OpenSession:
		s.handleOpenSession(p.OpenSession)
	case *spb.SessionResponse_VirtualRpc:
		s.handleVRPCResponse(p.VirtualRpc)
	case *spb.SessionResponse_Error:
		s.handleErrorResponse(p.Error)
	case *spb.SessionResponse_SessionParameters:
		s.handleSessionParameters(p.SessionParameters)
	case *spb.SessionResponse_Heartbeat:
		// Server emits Heartbeats during long-running VRPCs; the deadline
		// reset below is what keeps the watchdog from firing on them.
	case *spb.SessionResponse_GoAway:
		s.handleGoAway(p.GoAway)
	case *spb.SessionResponse_SessionRefreshConfig:
		s.handleSessionRefreshConfig(p.SessionRefreshConfig)
	default:
		s.debugf("received SessionResponse with unknown payload type %T", p)
		return
	}
	s.resetHeartbeatDeadline()
}

// handleOpenSession transitions Starting -> Active and signals listeners.
func (s *Session) handleOpenSession(_ *spb.OpenSessionResponse) {
	if _, ok := s.transitionTo(StateActive, isState(StateStarting)); !ok {
		return
	}
	// TODO: tracer.recordOpen(ctx, nil) once sessionTracer lands.
	s.hooks.onActive(s)
}

// handleErrorResponse splits per-RPC errors (rpc_id != 0) from session-level
// errors (rpc_id == 0). Session-level errors force-close the session;
// ForceClose then cancels all in-flight RPCs with ErrUnavailableSessionError.
func (s *Session) handleErrorResponse(errResp *spb.ErrorResponse) {
	if errResp.GetRpcId() != 0 {
		s.handleVRPCErrorResponse(errResp)
		return
	}
	desc := "server reported session-level error"
	if errResp.Status != nil && errResp.Status.Message != "" {
		desc = fmt.Sprintf("server session error: %s", errResp.Status.Message)
	}
	s.debugf("%s", desc)
	s.ForceClose(&spb.CloseSessionRequest{
		Reason:      spb.CloseSessionRequest_CLOSE_SESSION_REASON_ERROR,
		Description: desc,
	})
}

// handleSessionParameters updates the heartbeat interval negotiated by the
// server and immediately recomputes the watchdog deadline against the new
// interval.
func (s *Session) handleSessionParameters(params *spb.SessionParametersResponse) {
	if params.KeepAlive == nil {
		return
	}
	interval := params.KeepAlive.AsDuration()
	if interval <= 0 {
		return
	}
	s.mu.Lock()
	s.heartbeatInterval = interval
	s.nextHeartbeatDeadline = time.Now().Add(3 * interval)
	s.mu.Unlock()
}

// handleSessionRefreshConfig stores the server-provided refresh configuration
// for later use by reconnection logic.
func (s *Session) handleSessionRefreshConfig(cfg *spb.SessionRefreshConfig) {
	s.mu.Lock()
	s.refreshConfig = cfg
	s.mu.Unlock()
	s.debugf("stored SessionRefreshConfig (optimized_open=%t, metadata_entries=%d)",
		cfg.GetOptimizedOpenRequest() != nil, len(cfg.GetMetadata()))
}

// handleGoAway transitions to StateClosing and cancels every RPC with an id
// greater than the last admitted one. A late-arriving GOAWAY from an already
// terminal session is ignored — we do not move backwards.
func (s *Session) handleGoAway(goAway *spb.GoAwayResponse) {
	s.transitionTo(StateClosing, notState(StateClosing, StateClosed))

	lastAdmitted := goAway.GetLastRpcIdAdmitted()
	s.debugf("received GOAWAY reason=%q description=%q last_rpc_id_admitted=%d",
		goAway.GetReason(), goAway.GetDescription(), lastAdmitted)

	err := unavailable(ErrUnavailableGoAway,
		"vRPC not admitted before GOAWAY (last_admitted=%d)", lastAdmitted)
	s.cancelActiveRPCs(err, func(id int64) bool { return id > lastAdmitted })
}

// handleClose is invoked when Recv returns an error. It transitions to
// StateClosed and cancels every remaining in-flight RPC.
func (s *Session) handleClose(err error) {
	if _, ok := s.transitionTo(StateClosed, notState(StateClosed)); !ok {
		return
	}
	s.cancelActiveRPCs(unavailable(err, "session closed: %v", err), nil)
	s.signalQuiescent()
	s.notifyClosed(err)
}

// resetHeartbeatDeadline pushes out the watchdog to (3 * heartbeatInterval)
// from now. The 3x multiplier follows the protocol guidance of tolerating two
// missed heartbeats.
func (s *Session) resetHeartbeatDeadline() {
	s.mu.Lock()
	s.nextHeartbeatDeadline = time.Now().Add(3 * s.heartbeatInterval)
	s.mu.Unlock()
}

// heartBeatLoop watches the session's heartbeat deadline using a single Timer
// that re-arms itself when a frame extends the deadline. The watchdog is
// only enforced while at least one VRPC is in flight: the server emits
// Heartbeats during long-running VRPCs, so an idle session legitimately
// receives no heartbeats and must not be torn down.
func (s *Session) heartBeatLoop(ctx context.Context) {
	s.mu.Lock()
	deadline := s.nextHeartbeatDeadline
	s.mu.Unlock()

	timer := time.NewTimer(time.Until(deadline))
	defer timer.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-timer.C:
			s.mu.Lock()
			if s.state == StateClosed {
				s.mu.Unlock()
				return
			}
			active := len(s.activeRPCs)
			remaining := time.Until(s.nextHeartbeatDeadline)
			interval := s.heartbeatInterval
			s.mu.Unlock()

			if active == 0 {
				// Idle session: no heartbeats are expected. Re-check after
				// one interval so a freshly-started VRPC is picked up.
				timer.Reset(interval)
				continue
			}
			if remaining > 0 {
				// Deadline was pushed out while we were sleeping; re-arm.
				timer.Reset(remaining)
				continue
			}
			s.ForceClose(&spb.CloseSessionRequest{
				Reason:      spb.CloseSessionRequest_CLOSE_SESSION_REASON_MISSED_HEARTBEAT,
				Description: "client terminated session due to missed server heartbeats",
			})
			return
		}
	}
}

// peerInfoExtracter parses the base64-encoded peer info header and caches
// the decoded PeerInfo on the session.
func (s *Session) peerInfoExtracter(peerInfoData []string) {
	if len(peerInfoData) == 0 {
		return
	}
	encodings := []*base64.Encoding{
		base64.RawURLEncoding,
		base64.StdEncoding,
		base64.RawStdEncoding,
	}
	var decoded []byte
	var lastErr error
	for _, enc := range encodings {
		d, err := enc.DecodeString(peerInfoData[0])
		if err == nil {
			decoded = d
			lastErr = nil
			break
		}
		lastErr = err
	}
	if lastErr != nil {
		s.debugf("decode base64 PeerInfo failed: %v", lastErr)
		return
	}
	var peerInfo spb.PeerInfo
	if err := proto.Unmarshal(decoded, &peerInfo); err != nil {
		s.debugf("unmarshal PeerInfo proto failed: %v", err)
		return
	}
	s.mu.Lock()
	s.peerInfo = &peerInfo
	s.mu.Unlock()
	// TODO: capture s.logName under s.mu above and pass it to
	// tracer.setPeerInfo(&peerInfo, logName) once sessionTracer lands.
	s.debugf("parsed PeerInfo: transport_type=%s afe=%s",
		peerInfo.GetTransportType(), peerInfo.GetApplicationFrontendSubzone())
}

// closeReasonToCause maps a CloseSessionRequest reason to a sentinel error,
// or nil when the close is benign (user-initiated, downsize, unset). Callers
// of unavailable(nil, …) get a codes.Unavailable error without a wrapped
// sentinel, so errors.Is against the session sentinels returns false — which
// is the correct signal for "this was an expected shutdown."
func closeReasonToCause(req *spb.CloseSessionRequest) error {
	if req == nil {
		return nil
	}
	switch req.Reason {
	case spb.CloseSessionRequest_CLOSE_SESSION_REASON_MISSED_HEARTBEAT:
		return ErrUnavailableHeartBeatMissed
	case spb.CloseSessionRequest_CLOSE_SESSION_REASON_GOAWAY:
		return ErrUnavailableGoAway
	case spb.CloseSessionRequest_CLOSE_SESSION_REASON_ERROR:
		return ErrUnavailableSessionError
	default:
		return nil
	}
}

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
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/proto"
)

// Start starts the session by sending the OpenSessionRequest and beginning the read loop.
func (s *Session) Start(ctx context.Context, req *spb.OpenSessionRequest, md metadata.MD) error {
	s.mu.Lock()
	if s.state != StateNew {
		s.mu.Unlock()
		return fmt.Errorf("session already started or closed")
	}
	s.state = StateStarting
	s.lastStateChange = time.Now()
	s.mu.Unlock()

	openReq := &spb.SessionRequest{
		Payload: &spb.SessionRequest_OpenSession{
			OpenSession: req,
		},
	}

	if err := s.Send(openReq); err != nil {
		s.ForceClose(&spb.CloseSessionRequest{
			Reason:      spb.CloseSessionRequest_CLOSE_SESSION_REASON_ERROR,
			Description: "failed to send open session request",
		})
		return fmt.Errorf("failed to send open session request: %w", err)
	}

	go s.readLoop(ctx)
	go s.heartBeatLoop(ctx)

	if s.listener != nil {
		s.listener.OnStart(ctx)
	}

	return nil
}

// cancelActiveRPCs extracts active RPCs matching the filter (or all if filter is nil),
// deletes them from the map, and notifies them of failure.
func (s *Session) cancelActiveRPCs(err error, filter func(rpcID int64) bool) {
	s.mu.Lock()
	var active []*VRPCImpl
	for id, rpc := range s.activeRPCs {
		if filter == nil || filter(id) {
			active = append(active, rpc)
			delete(s.activeRPCs, id)
		}
	}
	s.mu.Unlock()

	for _, rpc := range active {
		select {
		case rpc.resultChan <- vrpcResult{err: err}:
		default:
		}
	}
}

// ForceClose immediately closes the session, transitioning it to StateClosed.
func (s *Session) ForceClose(req *spb.CloseSessionRequest) {
	s.mu.Lock()
	if s.state == StateClosed {
		s.mu.Unlock()
		return
	}
	s.state = StateClosed
	s.lastStateChange = time.Now()
	s.mu.Unlock()

	s.tracer.recordClose(context.Background())

	desc := "session force closed"
	if req != nil && req.Description != "" {
		desc = fmt.Sprintf("session force closed: %s", req.Description)
	}
	s.cancelActiveRPCs(status.Errorf(codes.Unavailable, "%s", desc), nil)

	if s.listener != nil {
		s.listener.OnClose(s, nil)
	}
}

// Close initiates a graceful shutdown of the session, draining active RPCs,
// sending CloseSessionRequest, and transitioning to StateWaitServerClose.
func (s *Session) Close(req *spb.CloseSessionRequest) error {
	s.mu.Lock()
	if s.state == StateClosed || s.state == StateClosing || s.state == StateWaitServerClose {
		s.mu.Unlock()
		return nil
	}
	s.state = StateClosing
	s.lastStateChange = time.Now()
	s.mu.Unlock()

	// Wait for active RPCs to drain safely
	ticker := time.NewTicker(10 * time.Millisecond)
	defer ticker.Stop()

	for {
		s.mu.Lock()
		activeCount := len(s.activeRPCs)
		s.mu.Unlock()

		if activeCount == 0 {
			break
		}

		select {
		case <-ticker.C:
		}
	}

	s.mu.Lock()
	s.state = StateWaitServerClose
	s.lastStateChange = time.Now()
	s.mu.Unlock()

	closeReq := &spb.SessionRequest{
		Payload: &spb.SessionRequest_CloseSession{
			CloseSession: req,
		},
	}

	if err := s.Send(closeReq); err != nil {
		s.ForceClose(req)
		return fmt.Errorf("failed to send close session request: %w", err)
	}

	return nil
}

func (s *Session) readLoop(ctx context.Context) {
	// 1. Parse peer information from stream header metadata asynchronously (non-blocking)!
	go func() {
		headerMD, err := s.stream.Header()
		if err != nil {
			fmt.Printf(">>> SESSION %s Header() returned error: %v <<<\n", s.logName, err)
			return
		}
		s.peerInfoExtracter(headerMD.Get("bigtable-peer-info"))
	}()

	// Supervisor goroutine leak shield:
	// If the context is cancelled, we force close the session to unblock Recv().
	readLoopDone := make(chan struct{})
	go func() {
		select {
		case <-ctx.Done():
			s.ForceClose(&spb.CloseSessionRequest{
				Reason:      spb.CloseSessionRequest_CLOSE_SESSION_REASON_USER,
				Description: "client context cancelled, closing stream",
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

func (s *Session) handleClose(err error) {
	s.mu.Lock()
	if s.state == StateClosed {
		s.mu.Unlock()
		return
	}
	isStarting := s.state == StateStarting || s.state == StateNew
	s.state = StateClosed
	s.lastStateChange = time.Now()
	if isStarting {
		s.handshakeErr = err
		select {
		case <-s.handshakeDone:
		default:
			close(s.handshakeDone)
		}
	}
	s.mu.Unlock()

	s.tracer.recordClose(context.Background())

	s.cancelActiveRPCs(status.Errorf(codes.Unavailable, "session closed handle close: %v", err), nil)

	if s.listener != nil {
		s.listener.OnClose(s, err)
	}
}

func (s *Session) handleSessionResponse(resp *spb.SessionResponse) {
	if openResp := resp.GetOpenSession(); openResp != nil {
		s.mu.Lock()
		isStarting := s.state == StateStarting
		if isStarting {
			s.state = StateActive
			s.lastStateChange = time.Now()
			close(s.handshakeDone)
		}
		s.mu.Unlock()
		if isStarting {
			s.tracer.recordOpen(context.Background(), nil)
		}
		if isStarting && s.listener != nil {
			s.listener.OnActive(s)
		}
		return
	}

	if vrpc := resp.GetVirtualRpc(); vrpc != nil {
		s.handleVRPCResponse(vrpc)
		return
	}

	if errResp := resp.GetError(); errResp != nil {
		s.handleVRPCErrorResponse(errResp)
		return
	}

	if goAway := resp.GetGoAway(); goAway != nil {
		s.handleGoAway(goAway)
		return
	}

	if params := resp.GetSessionParameters(); params != nil {
		s.mu.Lock()
		if params.KeepAlive != nil {
			interval := time.Duration(params.KeepAlive.Seconds)*time.Second + time.Duration(params.KeepAlive.Nanos)*time.Nanosecond
			if interval > 0 {
				s.heartbeatInterval = interval
			}
		}
		s.mu.Unlock()
		s.resetHeartbeatDeadline()
		return
	}

	if hb := resp.GetHeartbeat(); hb != nil {
		s.resetHeartbeatDeadline()
		return
	}

	s.resetHeartbeatDeadline() // Reset deadline on any successfully received server frame!
}

func (s *Session) resetHeartbeatDeadline() {
	s.mu.Lock()
	s.nextHeartbeatDeadline = time.Now().Add(3 * s.heartbeatInterval)
	s.mu.Unlock()
}

func (s *Session) heartBeatLoop(ctx context.Context) {
	ticker := time.NewTicker(50 * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			s.mu.Lock()
			if s.state == StateClosed {
				s.mu.Unlock()
				return
			}

			// Missed heartbeats read watchdog check!
			if time.Now().After(s.nextHeartbeatDeadline) {
				s.mu.Unlock()
				s.ForceClose(&spb.CloseSessionRequest{
					Reason:      spb.CloseSessionRequest_CLOSE_SESSION_REASON_MISSED_HEARTBEAT,
					Description: "session client terminated due to missed server heartbeats keepalive",
				})
				return
			}
			s.mu.Unlock()
		}
	}
}

// Send sends a SessionRequest thread-safely using sendMu to prevent concurrent stream corruption.
func (s *Session) Send(req *spb.SessionRequest) error {
	s.sendMu.Lock()
	defer s.sendMu.Unlock()
	return s.stream.Send(req)
}

func (s *Session) peerInfoExtracter(peerInfoData []string) {
	if len(peerInfoData) == 0 {
		return
	}
	decoded, err := base64.RawURLEncoding.DecodeString(peerInfoData[0])
	if err != nil {
		decoded, err = base64.StdEncoding.DecodeString(peerInfoData[0])
		if err != nil {
			decoded, err = base64.RawStdEncoding.DecodeString(peerInfoData[0])
		}
	}
	if err != nil {
		fmt.Printf(">>> SESSION %s failed to decode base64 PeerInfo: %v <<<\n", s.logName, err)
		return
	}
	var peerInfo spb.PeerInfo
	if err := proto.Unmarshal(decoded, &peerInfo); err != nil {
		fmt.Printf(">>> SESSION %s failed to unmarshal PeerInfo proto: %v <<<\n", s.logName, err)
		return
	}
	s.mu.Lock()
	s.peerInfo = &peerInfo
	s.mu.Unlock()
	s.tracer.setPeerInfo(&peerInfo, s.logName)
	fmt.Printf(">>> SESSION %s parsed PeerInfo: %+v <<<\n", s.logName, &peerInfo)
}

func (s *Session) handleGoAway(goAway *spb.GoAwayResponse) {
	s.mu.Lock()
	s.state = StateClosing
	s.lastStateChange = time.Now()
	s.mu.Unlock()

	lastAdmitted := goAway.GetLastRpcIdAdmitted()
	fmt.Printf(">>> SESSION %s received GOAWAY: last_rpc_id_admitted=%d <<<\n", s.logName, lastAdmitted)

	err := status.Errorf(codes.Unavailable, "vRPC not admitted by server before GOAWAY")
	s.cancelActiveRPCs(err, func(id int64) bool {
		return id > lastAdmitted
	})
}

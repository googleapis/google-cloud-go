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
	"errors"
	"fmt"
	"reflect"
	"sync"
	"testing"
	"time"

	spb "cloud.google.com/go/bigtable/apiv2/bigtablepb"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/durationpb"
)

func TestCloseReasonToCause(t *testing.T) {
	tests := []struct {
		name string
		req  *spb.CloseSessionRequest
		want error
	}{
		{"nil request maps to nil", nil, nil},
		{"unset reason → nil", &spb.CloseSessionRequest{Reason: spb.CloseSessionRequest_CLOSE_SESSION_REASON_UNSET}, nil},
		{"user-initiated → nil", &spb.CloseSessionRequest{Reason: spb.CloseSessionRequest_CLOSE_SESSION_REASON_USER}, nil},
		{"missed heartbeat", &spb.CloseSessionRequest{Reason: spb.CloseSessionRequest_CLOSE_SESSION_REASON_MISSED_HEARTBEAT}, ErrUnavailableHeartBeatMissed},
		{"goaway", &spb.CloseSessionRequest{Reason: spb.CloseSessionRequest_CLOSE_SESSION_REASON_GOAWAY}, ErrUnavailableGoAway},
		{"error", &spb.CloseSessionRequest{Reason: spb.CloseSessionRequest_CLOSE_SESSION_REASON_ERROR}, ErrUnavailableSessionError},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := closeReasonToCause(tt.req); !errors.Is(got, tt.want) && got != tt.want {
				t.Errorf("closeReasonToCause(%v) = %v, want %v", tt.req, got, tt.want)
			}
		})
	}
}

// --- handleOpenSession -------------------------------------------------------

func TestHandleOpenSession_TransitionsToActive(t *testing.T) {
	stream := newFakeStream()
	listener := &hookCounts{}
	s := newTestSession(t, stream, listener.hooks())
	s.state.Store(int32(StateStarting))

	s.handleOpenSession(&spb.OpenSessionResponse{})

	if got := s.State(); got != StateReady {
		t.Errorf("state = %v, want StateReady", got)
	}
	if _, active, _ := listener.counts(); active != 1 {
		t.Errorf("OnActive called %d times, want 1", active)
	}

	// Re-delivery (idempotent): no extra listener firings.
	s.handleOpenSession(&spb.OpenSessionResponse{})
	if _, active, _ := listener.counts(); active != 1 {
		t.Errorf("OnActive called %d times after re-delivery, want 1", active)
	}
}

// TestHandleOpenSession_PeerInfoBeforeOnActive verifies PeerInfo is populated
// before onActive fires. The AFE-grouping picker reads s.AfeID() (which reads
// PeerInfo) inside its OnActive path, so a nil PeerInfo would silently bucket
// the session under AfeID=0.
func TestHandleOpenSession_PeerInfoBeforeOnActive(t *testing.T) {
	pi := &spb.PeerInfo{
		ApplicationFrontendSubzone: "us-central1-a",
		TransportType:              spb.PeerInfo_TRANSPORT_TYPE_DIRECT_ACCESS,
	}
	raw, err := proto.Marshal(pi)
	if err != nil {
		t.Fatalf("proto.Marshal: %v", err)
	}
	encoded := base64.RawURLEncoding.EncodeToString(raw)

	stream := newFakeStream()
	stream.hdr = metadata.MD{peerInfoHeaderKey: []string{encoded}}

	var peerInfoAtActive *spb.PeerInfo
	hooks := SessionHooks{
		OnActive: func(s *Session) {
			peerInfoAtActive = s.PeerInfo()
		},
	}
	s := newTestSession(t, stream, hooks)
	s.state.Store(int32(StateStarting))

	s.handleOpenSession(&spb.OpenSessionResponse{})

	if peerInfoAtActive == nil {
		t.Fatal("PeerInfo was nil when onActive fired; expected populated")
	}
	if got := peerInfoAtActive.GetApplicationFrontendSubzone(); got != "us-central1-a" {
		t.Errorf("PeerInfo.ApplicationFrontendSubzone = %q, want us-central1-a", got)
	}
}

// TestHandleOpenSession_MissingHeaderStillFiresOnActive covers older backends
// / tests that don't send the bigtable-peer-info header. onActive must still
// fire; PeerInfo simply remains nil.
func TestHandleOpenSession_MissingHeaderStillFiresOnActive(t *testing.T) {
	stream := newFakeStream()
	// stream.hdr is an empty MD by default; no peer-info header.

	listener := &hookCounts{}
	s := newTestSession(t, stream, listener.hooks())
	s.state.Store(int32(StateStarting))

	s.handleOpenSession(&spb.OpenSessionResponse{})

	if _, active, _ := listener.counts(); active != 1 {
		t.Errorf("OnActive called %d times, want 1", active)
	}
	if s.PeerInfo() != nil {
		t.Error("PeerInfo should be nil when header absent")
	}
}

// --- handleGoAway ------------------------------------------------------------

// TestHandleGoAway_PreservesInFlightRPC: GOAWAY transitions to Closing (pool
// stops handing this session out) but does NOT cancel the in-flight RPC. If
// the server sends its response before dropping the stream, the RPC still
// completes cleanly — critical for non-idempotent Apply on graceful drains.
func TestHandleGoAway_PreservesInFlightRPC(t *testing.T) {
	s, _ := makeActive(t, SessionHooks{})

	rpc := &vrpcImpl{id: 1, method: "ReadRow", resultChan: make(chan vrpcResult, 1)}
	s.setSlotForTest(rpc)
	s.handleGoAway(&spb.GoAwayResponse{Reason: "test"})
	if got := s.State(); got != StateClosing {
		t.Errorf("state = %v, want StateClosing", got)
	}
	if s.activeVRPC() != rpc {
		t.Error("in-flight RPC should remain in slot; GOAWAY must not cancel it")
	}
	select {
	case res := <-rpc.resultChan:
		t.Errorf("in-flight RPC was cancelled (err=%v) but GOAWAY should let it complete", res.err)
	default:
	}

	// Response arriving AFTER the GOAWAY still completes the RPC.
	s.handleVRPCResponse(&spb.VirtualRpcResponse{RpcId: 1, Payload: []byte("late-but-real")})
	select {
	case res := <-rpc.resultChan:
		if res.err != nil {
			t.Errorf("post-GOAWAY response delivered with err=%v; want success", res.err)
		}
		if got := string(res.resp.Payload); got != "late-but-real" {
			t.Errorf("post-GOAWAY response payload = %q, want %q", got, "late-but-real")
		}
	default:
		t.Error("post-GOAWAY response was not delivered — RPC would hang")
	}
}

// --- handleSessionParameters / RefreshConfig / handleSessionResponse ---------

func TestHandleSessionParameters_UpdatesIntervalAndDeadline(t *testing.T) {
	s, _ := makeActive(t, SessionHooks{})
	before := time.Now()

	s.handleSessionParameters(&spb.SessionParametersResponse{
		KeepAlive: durationpb.New(2 * time.Second),
	})

	gotInterval := time.Duration(s.heartbeatIntervalNano.Load())
	gotDeadline := time.Unix(0, s.nextHeartbeatDeadlineNano.Load())

	if gotInterval != 2*time.Second {
		t.Errorf("heartbeatInterval = %v, want 2s", gotInterval)
	}
	// Deadline should be about before+interval (1x, matching
	// resetHeartbeatDeadline). Below-interval floor tolerates the
	// scheduling gap between capturing `before` and the atomic store.
	wantMin := before.Add(time.Second)
	if gotDeadline.Before(wantMin) {
		t.Errorf("nextHeartbeatDeadline = %v, want >= %v", gotDeadline, wantMin)
	}

	// Zero / nil keepalive should be ignored.
	s.handleSessionParameters(&spb.SessionParametersResponse{})
	s.handleSessionParameters(&spb.SessionParametersResponse{KeepAlive: durationpb.New(0)})

	if got := time.Duration(s.heartbeatIntervalNano.Load()); got != 2*time.Second {
		t.Errorf("heartbeatInterval changed after no-op updates: %v", got)
	}
}

func TestHandleSessionRefreshConfig_Stored(t *testing.T) {
	s, _ := makeActive(t, SessionHooks{})

	cfg := &spb.SessionRefreshConfig{
		OptimizedOpenRequest: &spb.OpenSessionRequest{ProtocolVersion: 7},
	}
	s.handleSessionRefreshConfig(cfg)

	got := s.RefreshConfig()
	if got != cfg {
		t.Errorf("RefreshConfig = %v, want %v", got, cfg)
	}
}

func TestHandleSessionResponse_UnknownDoesNotResetDeadline(t *testing.T) {
	s, _ := makeActive(t, SessionHooks{})
	s.nextHeartbeatDeadlineNano.Store(0) // way in the past

	// SessionResponse with no oneof set → unknown payload path.
	s.handleSessionResponse(&spb.SessionResponse{})

	got := time.Unix(0, s.nextHeartbeatDeadlineNano.Load())
	if !got.Equal(time.Unix(0, 0)) {
		t.Errorf("unknown payload reset deadline to %v; expected unchanged", got)
	}

	// A recognized variant must reset.
	s.handleSessionResponse(&spb.SessionResponse{
		Payload: &spb.SessionResponse_Heartbeat{Heartbeat: &spb.HeartbeatResponse{}},
	})
	got = time.Unix(0, s.nextHeartbeatDeadlineNano.Load())
	if got.Equal(time.Unix(0, 0)) {
		t.Error("recognized heartbeat did not reset deadline")
	}
}

// --- ForceClose / Close ------------------------------------------------------

func TestForceClose_Idempotent(t *testing.T) {
	listener := &hookCounts{}
	s, _ := makeActive(t, listener.hooks())

	s.ForceClose(nil)
	s.ForceClose(nil)
	s.ForceClose(&spb.CloseSessionRequest{Reason: spb.CloseSessionRequest_CLOSE_SESSION_REASON_ERROR})

	if got := s.State(); got != StateClosed {
		t.Errorf("state = %v, want StateClosed", got)
	}
	if _, _, closed := listener.counts(); closed != 1 {
		t.Errorf("OnClose fired %d times, want 1", closed)
	}
}

func TestForceClose_CancelsInflightWithReason(t *testing.T) {
	s, _ := makeActive(t, SessionHooks{})
	rpc := &vrpcImpl{id: 1, resultChan: make(chan vrpcResult, 1)}
	s.setSlotForTest(rpc)

	s.ForceClose(&spb.CloseSessionRequest{
		Reason:      spb.CloseSessionRequest_CLOSE_SESSION_REASON_MISSED_HEARTBEAT,
		Description: "no heartbeat",
	})

	select {
	case res := <-rpc.resultChan:
		if !errors.Is(res.err, ErrUnavailableHeartBeatMissed) {
			t.Errorf("cancelled cause = %v, want ErrUnavailableHeartBeatMissed", res.err)
		}
	default:
		t.Fatal("in-flight RPC not notified")
	}
}

func TestClose_Graceful_NoInflightSendsCloseRequest(t *testing.T) {
	stream := newFakeStream()
	s := newTestSession(t, stream, SessionHooks{})
	s.state.Store(int32(StateReady))

	if err := s.Close(context.Background(), &spb.CloseSessionRequest{
		Reason: spb.CloseSessionRequest_CLOSE_SESSION_REASON_USER,
	}); err != nil {
		t.Fatalf("Close returned error: %v", err)
	}
	// No in-flight → drained immediately, CloseSession sent, session advances
	// to WaitServerClose. handleClose (server EOF) then moves to Closed.
	if got := s.State(); got != StateWaitServerClose {
		t.Errorf("state = %v, want StateWaitServerClose", got)
	}
	sent := stream.snapshotSent()
	if len(sent) != 1 || sent[0].GetCloseSession() == nil {
		t.Errorf("expected one CloseSession frame, got %d sent frames", len(sent))
	}
}

func TestClose_AlreadyClosingIsNoop(t *testing.T) {
	stream := newFakeStream()
	s := newTestSession(t, stream, SessionHooks{})
	s.state.Store(int32(StateClosed))

	if err := s.Close(context.Background(), nil); err != nil {
		t.Errorf("Close on closed session = %v, want nil", err)
	}
	if got := s.State(); got != StateClosed {
		t.Errorf("state changed to %v", got)
	}
	if len(stream.snapshotSent()) != 0 {
		t.Error("Close on closed session should not send")
	}
}

func TestClose_CtxCancelDuringDrainForceCloses(t *testing.T) {
	s, _ := makeActive(t, SessionHooks{})
	// Pin an in-flight RPC so the drain loop is forced to wait.
	rpc := &vrpcImpl{id: 1, resultChan: make(chan vrpcResult, 1)}
	s.setSlotForTest(rpc)

	ctx, cancel := context.WithCancel(context.Background())
	cancelDone := make(chan struct{})
	go func() {
		time.Sleep(20 * time.Millisecond)
		cancel()
		close(cancelDone)
	}()

	err := s.Close(ctx, &spb.CloseSessionRequest{
		Reason: spb.CloseSessionRequest_CLOSE_SESSION_REASON_USER,
	})
	<-cancelDone
	if !errors.Is(err, context.Canceled) {
		t.Errorf("Close err = %v, want context.Canceled", err)
	}
	if got := s.State(); got != StateClosed {
		t.Errorf("state = %v, want StateClosed after ctx-cancel ForceClose", got)
	}
	// In-flight RPC should have been cancelled by ForceClose.
	select {
	case res := <-rpc.resultChan:
		if res.err == nil {
			t.Error("expected cancellation error on in-flight RPC")
		}
	default:
		t.Error("in-flight RPC not cancelled")
	}
}

// TestForceClose_SendsNoCloseSessionRequest: SESSION_SPEC #5 — ForceClose MUST
// NOT send a CloseSessionRequest on the wire (stream is presumed dead).
func TestForceClose_SendsNoCloseSessionRequest(t *testing.T) {
	s, stream := makeActive(t, SessionHooks{})

	s.ForceClose(&spb.CloseSessionRequest{
		Reason:      spb.CloseSessionRequest_CLOSE_SESSION_REASON_ERROR,
		Description: "test-induced",
	})

	if got := s.State(); got != StateClosed {
		t.Errorf("state = %v, want StateClosed after ForceClose", got)
	}
	for i, req := range stream.snapshotSent() {
		if req.GetCloseSession() != nil {
			t.Errorf("sent frame [%d] contains CloseSession, but ForceClose MUST NOT send CloseSessionRequest: %v", i, req)
		}
	}
}

// TestClose_IdempotentAcrossNCalls: SESSION_SPEC #5 — Close is "safe to call
// any number of times from any goroutine/thread." A single repeat wouldn't
// catch a bug in a counter-based idempotency check.
func TestClose_IdempotentAcrossNCalls(t *testing.T) {
	listener := &hookCounts{}
	s, stream := makeActive(t, listener.hooks())

	// First close is the real one (in-flight drain + CloseSessionRequest).
	if err := s.Close(context.Background(), &spb.CloseSessionRequest{
		Reason: spb.CloseSessionRequest_CLOSE_SESSION_REASON_USER,
	}); err != nil {
		t.Fatalf("first Close: %v", err)
	}
	sentAfterFirst := len(stream.snapshotSent())

	for i := 0; i < 5; i++ {
		if err := s.Close(context.Background(), nil); err != nil {
			t.Errorf("Close call #%d returned err = %v, want nil (spec #5: idempotent)", i+2, err)
		}
	}
	if got := len(stream.snapshotSent()); got != sentAfterFirst {
		t.Errorf("sent frame count = %d, want %d — additional Close calls MUST NOT send frames", got, sentAfterFirst)
	}
	if _, _, closed := listener.counts(); closed > 1 {
		t.Errorf("OnClose fired %d times, want <= 1", closed)
	}
}

// --- heartbeat ---------------------------------------------------------------

func TestHeartBeatLoop_ForceClosesOnMissedHeartbeat(t *testing.T) {
	s, _ := makeActive(t, SessionHooks{})
	rpc := &vrpcImpl{id: 1, resultChan: make(chan vrpcResult, 1)}
	s.setSlotForTest(rpc)
	s.nextHeartbeatDeadlineNano.Store(time.Now().Add(-time.Second).UnixNano()) // already missed

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go s.heartBeatLoop(ctx)

	waitFor(t, time.Second, func() bool { return s.State() == StateClosed }, "ForceClose from missed heartbeat")

	select {
	case res := <-rpc.resultChan:
		if !errors.Is(res.err, ErrUnavailableHeartBeatMissed) {
			t.Errorf("cancelled cause = %v, want ErrUnavailableHeartBeatMissed", res.err)
		}
		if code := status.Code(res.err); code != codes.Unavailable {
			t.Errorf("status.Code = %v, want Unavailable", code)
		}
	default:
		t.Error("in-flight RPC not cancelled on heartbeat miss")
	}
}

// TestHeartBeatLoop_HeartbeatsKeepInflightVRPCAlive: while a vRPC is in flight
// and the server is sending Heartbeats, the watchdog must NOT fire — every
// recognized inbound frame resets the deadline via handleSessionResponse.
func TestHeartBeatLoop_HeartbeatsKeepInflightVRPCAlive(t *testing.T) {
	s, _ := makeActive(t, SessionHooks{})
	s.heartbeatIntervalNano.Store(int64(30 * time.Millisecond))
	s.nextHeartbeatDeadlineNano.Store(time.Now().Add(30 * time.Millisecond).UnixNano()) // 1 * interval
	s.setSlotForTest(&vrpcImpl{id: 1, resultChan: make(chan vrpcResult, 1)})

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go s.heartBeatLoop(ctx)

	for i := 0; i < 8; i++ {
		time.Sleep(25 * time.Millisecond)
		s.handleSessionResponse(&spb.SessionResponse{
			Payload: &spb.SessionResponse_Heartbeat{Heartbeat: &spb.HeartbeatResponse{}},
		})
	}

	if got := s.State(); got != StateReady {
		t.Errorf("session torn down despite arriving heartbeats; state = %v", got)
	}
}

// TestHeartBeatLoop_IdleSessionIsNotTornDown: server sends Heartbeats only
// during in-flight vRPCs. An idle session with an elapsed deadline must NOT be
// force-closed.
func TestHeartBeatLoop_IdleSessionIsNotTornDown(t *testing.T) {
	s, _ := makeActive(t, SessionHooks{})
	s.heartbeatIntervalNano.Store(int64(20 * time.Millisecond))
	s.nextHeartbeatDeadlineNano.Store(time.Now().Add(-time.Hour).UnixNano())

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan struct{})
	go func() {
		s.heartBeatLoop(ctx)
		close(done)
	}()

	time.Sleep(150 * time.Millisecond)
	if s.State() == StateClosed {
		t.Fatal("idle session was force-closed despite having no in-flight VRPCs")
	}
	cancel()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("heartBeatLoop did not exit on ctx cancel")
	}
}

func TestHeartBeatLoop_ExitsOnCtxCancel(t *testing.T) {
	s, _ := makeActive(t, SessionHooks{})
	s.nextHeartbeatDeadlineNano.Store(time.Now().Add(time.Hour).UnixNano()) // never expires

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		s.heartBeatLoop(ctx)
		close(done)
	}()
	cancel()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("heartBeatLoop did not exit on ctx cancel")
	}
	if s.State() != StateReady {
		t.Errorf("state = %v, want StateReady (no force-close on ctx exit)", s.State())
	}
}

// --- extractPeerInfo -------------------------------------------------------

func TestPeerInfoExtracter_ParsesValidHeader(t *testing.T) {
	pi := &spb.PeerInfo{
		ApplicationFrontendSubzone: "us-central1-a",
		TransportType:              spb.PeerInfo_TRANSPORT_TYPE_DIRECT_ACCESS,
	}
	raw, err := proto.Marshal(pi)
	if err != nil {
		t.Fatalf("proto.Marshal: %v", err)
	}
	// Server uses URL-safe base64. Java's Base64.getUrlEncoder() emits padded
	// output; RawURLEncoding emits unpadded. Extracter must accept both.
	cases := []struct {
		name    string
		encoded string
	}{
		{"raw_url_unpadded", base64.RawURLEncoding.EncodeToString(raw)},
		{"url_padded", base64.URLEncoding.EncodeToString(raw)},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			s := newTestSession(t, newFakeStream(), SessionHooks{})
			s.extractPeerInfo([]string{tc.encoded})
			got := s.PeerInfo()
			if got == nil {
				t.Fatal("PeerInfo nil after extraction")
			}
			if got.GetApplicationFrontendSubzone() != "us-central1-a" {
				t.Errorf("AFE = %q, want us-central1-a", got.GetApplicationFrontendSubzone())
			}
			if got.GetTransportType() != spb.PeerInfo_TRANSPORT_TYPE_DIRECT_ACCESS {
				t.Errorf("TransportType = %v, want DIRECT_ACCESS", got.GetTransportType())
			}
		})
	}
}

func TestPeerInfoExtracter_EmptyAndBadInputs(t *testing.T) {
	s := newTestSession(t, newFakeStream(), SessionHooks{})

	s.extractPeerInfo(nil)
	s.extractPeerInfo([]string{})
	if s.PeerInfo() != nil {
		t.Error("PeerInfo should remain nil for empty input")
	}

	s.extractPeerInfo([]string{"!!!not-base64!!!"})
	if s.PeerInfo() != nil {
		t.Error("PeerInfo should remain nil for undecodable input")
	}
}

// --- Start integration -------------------------------------------------------

func TestStart_HandshakeAndClose(t *testing.T) {
	stream := newFakeStream()
	listener := &hookCounts{}
	s := newTestSession(t, stream, listener.hooks())

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	if err := s.Start(ctx, &spb.OpenSessionRequest{ProtocolVersion: 1}); err != nil {
		t.Fatalf("Start: %v", err)
	}
	waitFor(t, time.Second, func() bool { start, _, _ := listener.counts(); return start == 1 }, "OnStart")

	// Initial Open frame sent.
	if sent := stream.snapshotSent(); len(sent) != 1 || sent[0].GetOpenSession() == nil {
		t.Fatalf("expected OpenSession frame as first send, got %d frames", len(sent))
	}

	// Deliver the OpenSession response, then EOF.
	stream.recv <- recvOp{resp: &spb.SessionResponse{
		Payload: &spb.SessionResponse_OpenSession{OpenSession: &spb.OpenSessionResponse{}},
	}}
	waitFor(t, time.Second, func() bool { return s.State() == StateReady }, "StateReady")
	if _, active, _ := listener.counts(); active != 1 {
		t.Errorf("OnActive fired %d times, want 1", active)
	}

	stream.recv <- recvOp{err: fmt.Errorf("server EOF")}
	// Wait for BOTH the state transition and the OnClose hook — sequenced in
	// handleClose but not atomic, so a state-only check races the hook fire.
	waitFor(t, time.Second, func() bool {
		_, _, closed := listener.counts()
		return s.State() == StateClosed && closed == 1
	}, "StateClosed + OnClose after EOF")
}

func TestStart_RejectsIfNotNew(t *testing.T) {
	s, _ := makeActive(t, SessionHooks{})
	if err := s.Start(context.Background(), &spb.OpenSessionRequest{}); err == nil {
		t.Error("Start on active session should return error")
	}
}

// TestHooks_FiredInSpecOrder enforces SESSION_SPEC #4: lifecycle hooks fire in
// the fixed order OnStart → OnActive → OnClosing → OnClose, exactly once each,
// over a session's full lifetime.
func TestHooks_FiredInSpecOrder(t *testing.T) {
	var (
		mu    sync.Mutex
		order []string
	)
	record := func(name string) {
		mu.Lock()
		defer mu.Unlock()
		order = append(order, name)
	}
	hooks := SessionHooks{
		OnStart:   func(context.Context) { record("OnStart") },
		OnActive:  func(*Session) { record("OnActive") },
		OnClosing: func(*Session) { record("OnClosing") },
		OnClose:   func(*Session, error) { record("OnClose") },
	}

	stream := newFakeStream()
	s := newTestSession(t, stream, hooks)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	if err := s.Start(ctx, &spb.OpenSessionRequest{ProtocolVersion: 1}); err != nil {
		t.Fatalf("Start: %v", err)
	}

	stream.recv <- recvOp{resp: &spb.SessionResponse{
		Payload: &spb.SessionResponse_OpenSession{OpenSession: &spb.OpenSessionResponse{}},
	}}
	waitFor(t, time.Second, func() bool {
		mu.Lock()
		defer mu.Unlock()
		return len(order) >= 2
	}, "OnStart + OnActive")

	// ForceClose drives OnClosing then OnClose via the closingOnce → closeOnce
	// safety net — avoids the stream-EOF wait of graceful Close.
	s.ForceClose(&spb.CloseSessionRequest{
		Reason: spb.CloseSessionRequest_CLOSE_SESSION_REASON_USER,
	})
	waitFor(t, time.Second, func() bool {
		mu.Lock()
		defer mu.Unlock()
		return len(order) == 4
	}, "OnClosing + OnClose")

	mu.Lock()
	got := append([]string(nil), order...)
	mu.Unlock()

	want := []string{"OnStart", "OnActive", "OnClosing", "OnClose"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("hook order = %v, want %v", got, want)
	}
}

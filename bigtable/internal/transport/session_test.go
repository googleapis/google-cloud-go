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
	"sync"
	"testing"
	"time"

	spb "cloud.google.com/go/bigtable/apiv2/bigtablepb"
	rpcstatus "google.golang.org/genproto/googleapis/rpc/status"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/durationpb"
)

// fakeStream implements Stream and exposes channels so tests can drive both
// sides of the conversation.
type fakeStream struct {
	sentMu sync.Mutex
	sent   []*spb.SessionRequest
	recv   chan recvOp
	hdr    metadata.MD
	hdrErr error
	sendFn func(*spb.SessionRequest) error
}

type recvOp struct {
	resp *spb.SessionResponse
	err  error
}

func newFakeStream() *fakeStream {
	return &fakeStream{
		recv: make(chan recvOp, 32),
		hdr:  metadata.MD{},
	}
}

func (f *fakeStream) Send(req *spb.SessionRequest) error {
	if f.sendFn != nil {
		if err := f.sendFn(req); err != nil {
			return err
		}
	}
	f.sentMu.Lock()
	f.sent = append(f.sent, req)
	f.sentMu.Unlock()
	return nil
}

func (f *fakeStream) Recv() (*spb.SessionResponse, error) {
	op, ok := <-f.recv
	if !ok {
		return nil, fmt.Errorf("stream closed")
	}
	return op.resp, op.err
}

func (f *fakeStream) Header() (metadata.MD, error) {
	return f.hdr, f.hdrErr
}

func (f *fakeStream) snapshotSent() []*spb.SessionRequest {
	f.sentMu.Lock()
	defer f.sentMu.Unlock()
	out := make([]*spb.SessionRequest, len(f.sent))
	copy(out, f.sent)
	return out
}

// hookCounts captures lifecycle callbacks via a SessionHooks value.
type hookCounts struct {
	mu          sync.Mutex
	startCount  int
	activeCount int
	closeCount  int
	closeErr    error
}

func (c *hookCounts) hooks() SessionHooks {
	return SessionHooks{
		OnStart: func(context.Context) {
			c.mu.Lock()
			defer c.mu.Unlock()
			c.startCount++
		},
		OnActive: func(*Session) {
			c.mu.Lock()
			defer c.mu.Unlock()
			c.activeCount++
		},
		OnClose: func(_ *Session, err error) {
			c.mu.Lock()
			defer c.mu.Unlock()
			c.closeCount++
			c.closeErr = err
		},
	}
}

func (c *hookCounts) counts() (start, active, closed int) {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.startCount, c.activeCount, c.closeCount
}

// fakeDesc is a minimal VRpcDescriptor for Invoke tests.
type fakeDesc struct {
	method string
	enc    func(req interface{}) ([]byte, error)
	dec    func(buf []byte) (interface{}, error)
}

func (f *fakeDesc) Method() string                              { return f.method }
func (f *fakeDesc) Encode(req interface{}) ([]byte, error)      { return f.enc(req) }
func (f *fakeDesc) Decode(buf []byte) (interface{}, error)      { return f.dec(buf) }

func newTestSession(t *testing.T, stream Stream, hooks SessionHooks) *Session {
	t.Helper()
	return NewSession("test-session", stream, hooks, SessionTypeTable)
}

// waitFor polls cond every 5ms up to timeout, failing the test if cond never
// becomes true.
func waitFor(t *testing.T, timeout time.Duration, cond func() bool, msg string) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if cond() {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatalf("timed out after %v waiting for: %s", timeout, msg)
}

// --- pure value tests --------------------------------------------------------

func TestMultiPlexingLimit(t *testing.T) {
	if multiPlexingLimit != 1 {
		t.Errorf("multiPlexingLimit = %d, want 1 (multiplexing unsupported)", multiPlexingLimit)
	}
}

func TestState_String(t *testing.T) {
	tests := []struct {
		s    State
		want string
	}{
		{StateNew, "New"},
		{StateStarting, "Starting"},
		{StateActive, "Active"},
		{StateClosing, "Closing"},
		{StateClosed, "Closed"},
		{State(99), "Unknown"},
	}
	for _, tt := range tests {
		if got := tt.s.String(); got != tt.want {
			t.Errorf("State(%d).String() = %q, want %q", int(tt.s), got, tt.want)
		}
	}
}

func TestNewSession_Defaults(t *testing.T) {
	stream := newFakeStream()
	s := NewSession("log", stream, SessionHooks{}, SessionTypeAuthorizedView)

	if got := s.State(); got != StateNew {
		t.Errorf("initial state = %v, want StateNew", got)
	}
	if got := s.LogName(); got != "log" {
		t.Errorf("LogName = %q, want %q", got, "log")
	}
	if got := s.sessionType; got != SessionTypeAuthorizedView {
		t.Errorf("sessionType = %v, want SessionTypeAuthorizedView", got)
	}
	if s.activeRPCs == nil {
		t.Error("activeRPCs map not initialized")
	}
	if s.quiescent == nil {
		t.Error("quiescent channel not initialized")
	}
	if s.heartbeatInterval != defaultHeartbeatInterval {
		t.Errorf("heartbeatInterval = %v, want %v", s.heartbeatInterval, defaultHeartbeatInterval)
	}
	if s.vrpcSem == nil {
		t.Error("vrpcSem not initialized")
	}
	if s.PeerInfo() != nil {
		t.Error("PeerInfo should start nil")
	}
	if s.RefreshConfig() != nil {
		t.Error("RefreshConfig should start nil")
	}
	if s.HasOkRpcs() || s.HasErrorRpcs() {
		t.Error("HasOkRpcs/HasErrorRpcs should start false")
	}
}

func TestCloseReasonToCause(t *testing.T) {
	tests := []struct {
		name string
		req  *spb.CloseSessionRequest
		want error
	}{
		{"nil request maps to nil", nil, nil},
		{"unset reason → nil", &spb.CloseSessionRequest{Reason: spb.CloseSessionRequest_CLOSE_SESSION_REASON_UNSET}, nil},
		{"user-initiated → nil", &spb.CloseSessionRequest{Reason: spb.CloseSessionRequest_CLOSE_SESSION_REASON_USER}, nil},
		{"downsize → nil", &spb.CloseSessionRequest{Reason: spb.CloseSessionRequest_CLOSE_SESSION_REASON_DOWNSIZE}, nil},
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

func TestUnavailable_WrapsCauseAndStatus(t *testing.T) {
	err := unavailable(ErrUnavailableHeartBeatMissed, "heartbeat dead for %s", "test")

	if !errors.Is(err, ErrUnavailableHeartBeatMissed) {
		t.Error("errors.Is should match ErrUnavailableHeartBeatMissed")
	}
	if errors.Is(err, ErrUnavailableGoAway) {
		t.Error("errors.Is should not match unrelated sentinel")
	}
	if code := status.Code(err); code != codes.Unavailable {
		t.Errorf("status.Code = %v, want Unavailable", code)
	}
	if msg := err.Error(); msg == "" {
		t.Error("error string should be non-empty")
	}

	// nil cause should not crash and should still be Unavailable.
	err = unavailable(nil, "no cause")
	if code := status.Code(err); code != codes.Unavailable {
		t.Errorf("nil-cause: status.Code = %v, want Unavailable", code)
	}
	if errors.Is(err, ErrUnavailableHeartBeatMissed) {
		t.Error("nil-cause: errors.Is should not match any sentinel")
	}
}

// --- handler-level tests (no Start/readLoop) ---------------------------------

// makeActive constructs a session and forces it into StateActive without going
// through the handshake.
func makeActive(t *testing.T, hooks SessionHooks) (*Session, *fakeStream) {
	t.Helper()
	stream := newFakeStream()
	s := newTestSession(t, stream, hooks)
	s.mu.Lock()
	s.state = StateActive
	s.mu.Unlock()
	return s, stream
}

func TestHandleOpenSession_TransitionsToActive(t *testing.T) {
	stream := newFakeStream()
	listener := &hookCounts{}
	s := newTestSession(t, stream, listener.hooks())
	s.mu.Lock()
	s.state = StateStarting
	s.mu.Unlock()

	s.handleOpenSession(&spb.OpenSessionResponse{})

	if got := s.State(); got != StateActive {
		t.Errorf("state = %v, want StateActive", got)
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

func TestHandleVRPCResponse_RoutesByRpcID(t *testing.T) {
	s, _ := makeActive(t, SessionHooks{})

	rpc := &vrpcImpl{id: 7, method: "ReadRow", resultChan: make(chan vrpcResult, 1)}
	s.mu.Lock()
	s.activeRPCs[7] = rpc
	s.mu.Unlock()

	resp := &spb.VirtualRpcResponse{RpcId: 7, Payload: []byte("p")}
	s.handleVRPCResponse(resp)

	select {
	case res := <-rpc.resultChan:
		if res.resp != resp {
			t.Errorf("got resp %p, want %p", res.resp, resp)
		}
	default:
		t.Fatal("no result delivered")
	}
	if !s.HasOkRpcs() {
		t.Error("HasOkRpcs should be true after successful response")
	}
	// Unknown rpc_id is dropped silently.
	s.handleVRPCResponse(&spb.VirtualRpcResponse{RpcId: 999})
}

func TestHandleVRPCErrorResponse_RoutesByRpcID(t *testing.T) {
	s, _ := makeActive(t, SessionHooks{})

	rpc := &vrpcImpl{id: 3, method: "ReadRow", resultChan: make(chan vrpcResult, 1)}
	s.mu.Lock()
	s.activeRPCs[3] = rpc
	s.mu.Unlock()

	errResp := &spb.ErrorResponse{
		RpcId:  3,
		Status: &rpcstatus.Status{Code: int32(codes.FailedPrecondition), Message: "boom"},
	}
	s.handleVRPCErrorResponse(errResp)

	select {
	case res := <-rpc.resultChan:
		if res.err == nil {
			t.Fatal("expected error result")
		}
		if got := status.Code(res.err); got != codes.FailedPrecondition {
			t.Errorf("status code = %v, want FailedPrecondition", got)
		}
	default:
		t.Fatal("no result delivered")
	}
	if !s.HasErrorRpcs() {
		t.Error("HasErrorRpcs should be true after error response")
	}
}

func TestHandleErrorResponse_SessionFatalForcesClose(t *testing.T) {
	listener := &hookCounts{}
	s, _ := makeActive(t, listener.hooks())

	// Pre-existing in-flight RPC; should be cancelled by ForceClose.
	rpc := &vrpcImpl{id: 11, method: "ReadRow", resultChan: make(chan vrpcResult, 1)}
	s.mu.Lock()
	s.activeRPCs[11] = rpc
	s.mu.Unlock()

	s.handleErrorResponse(&spb.ErrorResponse{
		RpcId:  0,
		Status: &rpcstatus.Status{Code: int32(codes.Internal), Message: "fatal"},
	})

	if got := s.State(); got != StateClosed {
		t.Errorf("state = %v, want StateClosed", got)
	}
	select {
	case res := <-rpc.resultChan:
		if !errors.Is(res.err, ErrUnavailableSessionError) {
			t.Errorf("cancelled cause = %v, want ErrUnavailableSessionError", res.err)
		}
	default:
		t.Fatal("in-flight RPC not cancelled by session-fatal error")
	}
	if _, _, closed := listener.counts(); closed != 1 {
		t.Errorf("OnClose called %d times, want 1", closed)
	}
}

func TestHandleGoAway_CancelsBeyondAdmitted(t *testing.T) {
	s, _ := makeActive(t, SessionHooks{})

	// rpc 1 and 2 admitted; 3 and 4 should be cancelled.
	for _, id := range []int64{1, 2, 3, 4} {
		s.mu.Lock()
		s.activeRPCs[id] = &vrpcImpl{id: id, resultChan: make(chan vrpcResult, 1)}
		s.mu.Unlock()
	}

	s.handleGoAway(&spb.GoAwayResponse{LastRpcIdAdmitted: 2, Reason: "test"})

	if got := s.State(); got != StateClosing {
		t.Errorf("state = %v, want StateClosing", got)
	}
	s.mu.Lock()
	_, has1 := s.activeRPCs[1]
	_, has2 := s.activeRPCs[2]
	_, has3 := s.activeRPCs[3]
	_, has4 := s.activeRPCs[4]
	s.mu.Unlock()
	if !has1 || !has2 {
		t.Error("admitted RPCs (1, 2) should remain in activeRPCs")
	}
	if has3 || has4 {
		t.Error("RPCs beyond admitted (3, 4) should have been removed")
	}
}

func TestHandleSessionParameters_UpdatesIntervalAndDeadline(t *testing.T) {
	s, _ := makeActive(t, SessionHooks{})
	before := time.Now()

	s.handleSessionParameters(&spb.SessionParametersResponse{
		KeepAlive: durationpb.New(2 * time.Second),
	})

	s.mu.Lock()
	gotInterval := s.heartbeatInterval
	gotDeadline := s.nextHeartbeatDeadline
	s.mu.Unlock()

	if gotInterval != 2*time.Second {
		t.Errorf("heartbeatInterval = %v, want 2s", gotInterval)
	}
	// Expect deadline ≈ now + 6s (3 * 2s).
	wantMin := before.Add(5 * time.Second)
	if gotDeadline.Before(wantMin) {
		t.Errorf("nextHeartbeatDeadline = %v, want >= %v", gotDeadline, wantMin)
	}

	// Zero / nil keepalive should be ignored.
	s.handleSessionParameters(&spb.SessionParametersResponse{})
	s.handleSessionParameters(&spb.SessionParametersResponse{KeepAlive: durationpb.New(0)})

	s.mu.Lock()
	if s.heartbeatInterval != 2*time.Second {
		t.Errorf("heartbeatInterval changed after no-op updates: %v", s.heartbeatInterval)
	}
	s.mu.Unlock()
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
	s.mu.Lock()
	s.nextHeartbeatDeadline = time.Unix(0, 0) // way in the past
	s.mu.Unlock()

	// SessionResponse with no oneof set → unknown payload path.
	s.handleSessionResponse(&spb.SessionResponse{})

	s.mu.Lock()
	got := s.nextHeartbeatDeadline
	s.mu.Unlock()
	if !got.Equal(time.Unix(0, 0)) {
		t.Errorf("unknown payload reset deadline to %v; expected unchanged", got)
	}

	// A recognized variant must reset.
	s.handleSessionResponse(&spb.SessionResponse{
		Payload: &spb.SessionResponse_Heartbeat{Heartbeat: &spb.HeartbeatResponse{}},
	})
	s.mu.Lock()
	got = s.nextHeartbeatDeadline
	s.mu.Unlock()
	if got.Equal(time.Unix(0, 0)) {
		t.Error("recognized heartbeat did not reset deadline")
	}
}

// --- Invoke tests -------------------------------------------------------

func newRoundTripDesc() *fakeDesc {
	return &fakeDesc{
		method: "RoundTrip",
		enc: func(req interface{}) ([]byte, error) {
			return []byte(fmt.Sprintf("req:%v", req)), nil
		},
		dec: func(buf []byte) (interface{}, error) {
			return string(buf), nil
		},
	}
}

func TestInvoke_RejectsWhenNotActive(t *testing.T) {
	stream := newFakeStream()
	s := newTestSession(t, stream, SessionHooks{}) // state = New
	_, err := s.Invoke(context.Background(), newRoundTripDesc(), "hello")
	if !errors.Is(err, ErrSessionNotActive) {
		t.Errorf("err = %v, want ErrSessionNotActive in chain", err)
	}
	if code := status.Code(err); code != codes.Unavailable {
		t.Errorf("status.Code = %v, want Unavailable", code)
	}
}

func TestInvoke_HappyPath(t *testing.T) {
	s, stream := makeActive(t, SessionHooks{})
	desc := newRoundTripDesc()

	done := make(chan struct{})
	var res InvokeResult
	var execErr error
	go func() {
		defer close(done)
		res, execErr = s.Invoke(context.Background(), desc, "hello")
	}()

	// Wait for the request to be sent.
	waitFor(t, time.Second, func() bool { return len(stream.snapshotSent()) > 0 }, "Send called")

	sent := stream.snapshotSent()[0].GetVirtualRpc()
	if sent == nil {
		t.Fatal("sent frame is not a VirtualRpcRequest")
	}
	if string(sent.Payload) != "req:hello" {
		t.Errorf("encoded payload = %q, want %q", sent.Payload, "req:hello")
	}

	// Deliver response.
	s.handleVRPCResponse(&spb.VirtualRpcResponse{
		RpcId:       sent.RpcId,
		Payload:     []byte("world"),
		ClusterInfo: &spb.ClusterInformation{ClusterId: "c1"},
	})

	<-done
	if execErr != nil {
		t.Fatalf("Invoke error: %v", execErr)
	}
	if got := res.Response.(string); got != "world" {
		t.Errorf("resp = %q, want %q", got, "world")
	}
	if res.ClusterInfo == nil || res.ClusterInfo.ClusterId != "c1" {
		t.Errorf("clusterInfo = %v, want ClusterId=c1", res.ClusterInfo)
	}
}

func TestInvoke_ContextCancel(t *testing.T) {
	s, _ := makeActive(t, SessionHooks{})
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err := s.Invoke(ctx, newRoundTripDesc(), "hello")
	if !errors.Is(err, context.Canceled) {
		t.Errorf("err = %v, want context.Canceled", err)
	}
}

func TestInvoke_SendFailureCleansUpMap(t *testing.T) {
	stream := newFakeStream()
	stream.sendFn = func(req *spb.SessionRequest) error {
		return fmt.Errorf("network down")
	}
	s := newTestSession(t, stream, SessionHooks{})
	s.mu.Lock()
	s.state = StateActive
	s.mu.Unlock()

	_, err := s.Invoke(context.Background(), newRoundTripDesc(), "hello")
	if err == nil {
		t.Fatal("expected error from failed Send")
	}
	s.mu.Lock()
	leftOver := len(s.activeRPCs)
	s.mu.Unlock()
	if leftOver != 0 {
		t.Errorf("activeRPCs = %d, want 0 (defer should clean up on Send failure)", leftOver)
	}
}

// --- Close / ForceClose ------------------------------------------------------

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
	s.mu.Lock()
	s.activeRPCs[1] = rpc
	s.mu.Unlock()

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
	s.mu.Lock()
	s.state = StateActive
	s.mu.Unlock()

	if err := s.Close(context.Background(), &spb.CloseSessionRequest{
		Reason: spb.CloseSessionRequest_CLOSE_SESSION_REASON_USER,
	}); err != nil {
		t.Fatalf("Close returned error: %v", err)
	}
	if got := s.State(); got != StateClosing {
		t.Errorf("state = %v, want StateClosing", got)
	}
	sent := stream.snapshotSent()
	if len(sent) != 1 || sent[0].GetCloseSession() == nil {
		t.Errorf("expected one CloseSession frame, got %d sent frames", len(sent))
	}
}

func TestClose_AlreadyClosingIsNoop(t *testing.T) {
	stream := newFakeStream()
	s := newTestSession(t, stream, SessionHooks{})
	s.mu.Lock()
	s.state = StateClosed
	s.mu.Unlock()

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
	s.mu.Lock()
	s.activeRPCs[1] = rpc
	s.mu.Unlock()

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

// --- heartbeat ---------------------------------------------------------------

func TestHeartBeatLoop_ForceClosesOnMissedHeartbeat(t *testing.T) {
	s, _ := makeActive(t, SessionHooks{})
	rpc := &vrpcImpl{id: 1, resultChan: make(chan vrpcResult, 1)}
	s.mu.Lock()
	s.activeRPCs[1] = rpc
	s.nextHeartbeatDeadline = time.Now().Add(-time.Second) // already missed
	s.mu.Unlock()

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
			t.Errorf("status.Code = %v, want Unavailable (so existing retry plumbing applies)", code)
		}
	default:
		t.Error("in-flight RPC not cancelled on heartbeat miss")
	}
}

func TestHeartBeatLoop_HeartbeatsKeepInflightVRPCAlive(t *testing.T) {
	// Positive case: while a VRPC is in flight and the server is sending
	// Heartbeats, the watchdog must NOT fire. This proves the dispatch in
	// handleSessionResponse correctly resets the deadline on every
	// recognized frame.
	s, _ := makeActive(t, SessionHooks{})
	s.mu.Lock()
	s.heartbeatInterval = 30 * time.Millisecond
	s.nextHeartbeatDeadline = time.Now().Add(90 * time.Millisecond) // 3 * interval
	s.activeRPCs[1] = &vrpcImpl{id: 1, resultChan: make(chan vrpcResult, 1)}
	s.mu.Unlock()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go s.heartBeatLoop(ctx)

	// Pump heartbeats for longer than 3*interval; without the deadline
	// reset on each frame, the watchdog would have fired by now.
	for i := 0; i < 8; i++ {
		time.Sleep(25 * time.Millisecond)
		s.handleSessionResponse(&spb.SessionResponse{
			Payload: &spb.SessionResponse_Heartbeat{Heartbeat: &spb.HeartbeatResponse{}},
		})
	}

	if got := s.State(); got != StateActive {
		t.Errorf("session torn down despite arriving heartbeats; state = %v", got)
	}
}

func TestHeartBeatLoop_IdleSessionIsNotTornDown(t *testing.T) {
	// Server sends Heartbeats only during in-flight VRPCs. An idle session
	// with an elapsed deadline must NOT be force-closed; the loop should
	// keep checking until activity returns.
	s, _ := makeActive(t, SessionHooks{})
	s.mu.Lock()
	s.heartbeatInterval = 20 * time.Millisecond // make idle re-check fast
	s.nextHeartbeatDeadline = time.Now().Add(-time.Hour)
	s.mu.Unlock()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan struct{})
	go func() {
		s.heartBeatLoop(ctx)
		close(done)
	}()

	// Give the loop time to wake up several times on the idle interval.
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
	s.mu.Lock()
	s.nextHeartbeatDeadline = time.Now().Add(time.Hour) // never expires
	s.mu.Unlock()

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
	if s.State() != StateActive {
		t.Errorf("state = %v, want StateActive (no force-close on ctx exit)", s.State())
	}
}

// --- peerInfoExtracter -------------------------------------------------------

func TestPeerInfoExtracter_ParsesValidHeader(t *testing.T) {
	s := newTestSession(t, newFakeStream(), SessionHooks{})

	pi := &spb.PeerInfo{
		ApplicationFrontendSubzone: "us-central1-a",
		TransportType:              spb.PeerInfo_TRANSPORT_TYPE_DIRECT_ACCESS,
	}
	raw, err := proto.Marshal(pi)
	if err != nil {
		t.Fatalf("proto.Marshal: %v", err)
	}
	encoded := base64.RawURLEncoding.EncodeToString(raw)

	s.peerInfoExtracter([]string{encoded})

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
}

func TestPeerInfoExtracter_EmptyAndBadInputs(t *testing.T) {
	s := newTestSession(t, newFakeStream(), SessionHooks{})

	s.peerInfoExtracter(nil)
	s.peerInfoExtracter([]string{})
	if s.PeerInfo() != nil {
		t.Error("PeerInfo should remain nil for empty input")
	}

	s.peerInfoExtracter([]string{"!!!not-base64!!!"})
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
	waitFor(t, time.Second, func() bool { _, _, _ = listener.counts(); start, _, _ := listener.counts(); return start == 1 }, "OnStart")

	// Initial Open frame sent.
	if sent := stream.snapshotSent(); len(sent) != 1 || sent[0].GetOpenSession() == nil {
		t.Fatalf("expected OpenSession frame as first send, got %d frames", len(sent))
	}

	// Deliver the OpenSession response, then EOF.
	stream.recv <- recvOp{resp: &spb.SessionResponse{
		Payload: &spb.SessionResponse_OpenSession{OpenSession: &spb.OpenSessionResponse{}},
	}}
	waitFor(t, time.Second, func() bool { return s.State() == StateActive }, "StateActive")
	if _, active, _ := listener.counts(); active != 1 {
		t.Errorf("OnActive fired %d times, want 1", active)
	}

	stream.recv <- recvOp{err: fmt.Errorf("server EOF")}
	waitFor(t, time.Second, func() bool { return s.State() == StateClosed }, "StateClosed after EOF")
	if _, _, closed := listener.counts(); closed != 1 {
		t.Errorf("OnClose fired %d times, want 1", closed)
	}
}

func TestStart_RejectsIfNotNew(t *testing.T) {
	s, _ := makeActive(t, SessionHooks{})
	if err := s.Start(context.Background(), &spb.OpenSessionRequest{}); err == nil {
		t.Error("Start on active session should return error")
	}
}

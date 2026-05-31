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
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
)

type mockSessionStream struct {
	mu        sync.Mutex
	sent      []*spb.SessionRequest
	recvChan  chan *spb.SessionResponse
	recvErr   error
	headerMD  metadata.MD
	headerErr error
}

func newMockSessionStream() *mockSessionStream {
	return &mockSessionStream{
		recvChan: make(chan *spb.SessionResponse, 100),
		headerMD: metadata.New(nil),
	}
}

func (m *mockSessionStream) Send(req *spb.SessionRequest) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.sent = append(m.sent, req)
	return nil
}

func (m *mockSessionStream) Recv() (*spb.SessionResponse, error) {
	if m.recvErr != nil {
		return nil, m.recvErr
	}
	select {
	case resp, ok := <-m.recvChan:
		if !ok {
			return nil, errors.New("stream closed")
		}
		return resp, nil
	}
}

func (m *mockSessionStream) Header() (metadata.MD, error) {
	return m.headerMD, m.headerErr
}

type mockSessionListener struct {
	mu       sync.Mutex
	started  bool
	active   bool
	closed   bool
	closeErr error
}

func (m *mockSessionListener) OnStart(ctx context.Context) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.started = true
}

func (m *mockSessionListener) OnActive(s *Session) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.active = true
}

func (m *mockSessionListener) OnClose(s *Session, err error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.closed = true
	m.closeErr = err
}

type mockVRpcDescriptor struct {
	method string
}

func (d *mockVRpcDescriptor) Method() string { return d.method }
func (d *mockVRpcDescriptor) Encode(req interface{}) ([]byte, error) {
	return []byte(req.(string)), nil
}
func (d *mockVRpcDescriptor) Decode(buf []byte) (interface{}, error) {
	return string(buf), nil
}

func TestSession_Lifecycle(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	stream := newMockSessionStream()
	listener := &mockSessionListener{}
	session := NewSession("test-session", stream, listener, SessionTypeTable)

	if session.State() != StateNew {
		t.Errorf("Expected state to be StateNew, got %v", session.State())
	}

	// Start the session
	req := &spb.OpenSessionRequest{}
	err := session.Start(ctx, req, nil)
	if err != nil {
		t.Fatalf("Failed to start session: %v", err)
	}

	// Check initial state transition and listener
	if session.State() != StateStarting {
		t.Errorf("Expected state to be StateStarting, got %v", session.State())
	}

	listener.mu.Lock()
	if !listener.started {
		t.Error("Expected OnStart to be called on listener")
	}
	listener.mu.Unlock()

	// Complete handshake by writing OpenSessionResponse
	stream.recvChan <- &spb.SessionResponse{
		Payload: &spb.SessionResponse_OpenSession{
			OpenSession: &spb.OpenSessionResponse{},
		},
	}

	// Wait for handshake completion
	select {
	case <-session.handshakeDone:
	case <-time.After(2 * time.Second):
		t.Fatal("Timed out waiting for handshake completion")
	}

	if session.State() != StateActive {
		t.Errorf("Expected state to be StateActive, got %v", session.State())
	}

	// Wait briefly for the asynchronous OnActive listener hook to run
	time.Sleep(20 * time.Millisecond)

	listener.mu.Lock()
	if !listener.active {
		t.Error("Expected OnActive to be called on listener")
	}
	listener.mu.Unlock()

	// Force close session
	session.ForceClose(&spb.CloseSessionRequest{Description: "test close"})

	if session.State() != StateClosed {
		t.Errorf("Expected state to be StateClosed, got %v", session.State())
	}

	listener.mu.Lock()
	if !listener.closed {
		t.Error("Expected OnClose to be called on listener")
	}
	listener.mu.Unlock()
}

func TestSession_ExecuteVRpc_Success(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	stream := newMockSessionStream()
	session := NewSession("test-session", stream, nil, SessionTypeTable)

	// Pre-activate the session for direct testing of ExecuteVRpc
	session.state = StateActive
	close(session.handshakeDone)

	// Start background readLoop
	go session.readLoop(ctx)

	// Start executing vRPC in a separate goroutine so we can inject the response
	var respVal interface{}
	var errVal error
	desc := &mockVRpcDescriptor{method: "TestMethod"}
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		respVal, _, errVal = session.ExecuteVRpc(ctx, desc, "request_payload")
	}()

	// Wait a brief moment to ensure request is sent to mockStream
	time.Sleep(50 * time.Millisecond)

	stream.mu.Lock()
	if len(stream.sent) == 0 {
		stream.mu.Unlock()
		t.Fatal("Expected request to be sent on stream")
	}
	sentReq := stream.sent[0]
	stream.mu.Unlock()

	vrpcReq := sentReq.GetVirtualRpc()
	if vrpcReq == nil {
		t.Fatal("Expected SessionRequest to contain VirtualRpc payload")
	}

	if string(vrpcReq.Payload) != "request_payload" {
		t.Errorf("Expected payload %q, got %q", "request_payload", string(vrpcReq.Payload))
	}

	// Inject successful mock response matching the rpcID
	stream.recvChan <- &spb.SessionResponse{
		Payload: &spb.SessionResponse_VirtualRpc{
			VirtualRpc: &spb.VirtualRpcResponse{
				RpcId:   vrpcReq.RpcId,
				Payload: []byte("response_payload"),
			},
		},
	}

	wg.Wait()

	if errVal != nil {
		t.Fatalf("Expected successful vRPC execution, got error: %v", errVal)
	}

	if respVal.(string) != "response_payload" {
		t.Errorf("Expected response payload %q, got %q", "response_payload", respVal.(string))
	}
}

func TestSession_ExecuteVRpc_ClosedSession(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	stream := newMockSessionStream()
	session := NewSession("test-session", stream, nil, SessionTypeTable)

	// Ensure state is closed
	session.state = StateClosed

	desc := &mockVRpcDescriptor{method: "TestMethod"}
	_, _, err := session.ExecuteVRpc(ctx, desc, "request")
	if err == nil {
		t.Fatal("Expected error executing vRPC on closed session, got success")
	}

	st, ok := status.FromError(err)
	if !ok || st.Code() != codes.Unavailable {
		t.Errorf("Expected Unavailable status error, got %v", err)
	}
}

func TestSession_GoAway(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	stream := newMockSessionStream()
	session := NewSession("test-session", stream, nil, SessionTypeTable)

	// Pre-activate the session
	session.state = StateActive
	close(session.handshakeDone)

	// Start background readLoop
	go session.readLoop(ctx)

	resChan10 := make(chan vrpcResult, 1)
	resChan11 := make(chan vrpcResult, 1)

	session.mu.Lock()
	session.activeRPCs[10] = &VRPCImpl{
		id:         10,
		method:     "TestMethod",
		resultChan: resChan10,
	}
	session.activeRPCs[11] = &VRPCImpl{
		id:         11,
		method:     "TestMethod",
		resultChan: resChan11,
	}
	session.mu.Unlock()

	// Inject GoAway with last_rpc_id_admitted equal to 10.
	// This means 10 was admitted (should not be aborted),
	// and 11 was NOT admitted (should be aborted immediately).
	stream.recvChan <- &spb.SessionResponse{
		Payload: &spb.SessionResponse_GoAway{
			GoAway: &spb.GoAwayResponse{
				LastRpcIdAdmitted: 10,
			},
		},
	}

	// Provide 10 response payload on the stream to let it succeed
	stream.recvChan <- &spb.SessionResponse{
		Payload: &spb.SessionResponse_VirtualRpc{
			VirtualRpc: &spb.VirtualRpcResponse{
				RpcId:   10,
				Payload: []byte("resp10"),
			},
		},
	}

	// Verify 10 (admitted) succeeded
	var res10 vrpcResult
	select {
	case res10 = <-resChan10:
	case <-time.After(2 * time.Second):
		t.Fatal("Timed out waiting for vRPC 10 response")
	}

	if res10.err != nil {
		t.Errorf("Expected vRPC 10 (admitted) to succeed, got error: %v", res10.err)
	}
	if string(res10.resp.Payload) != "resp10" {
		t.Errorf("Expected vRPC 10 response payload 'resp10', got: %s", string(res10.resp.Payload))
	}

	// Verify 11 (unadmitted) failed with codes.Unavailable
	var res11 vrpcResult
	select {
	case res11 = <-resChan11:
	case <-time.After(2 * time.Second):
		t.Fatal("Timed out waiting for vRPC 11 response")
	}

	if res11.err == nil {
		t.Fatal("Expected vRPC 11 (unadmitted) to fail, but got success")
	}

	st, ok := status.FromError(res11.err)
	if !ok || st.Code() != codes.Unavailable {
		t.Errorf("Expected Unavailable status error for unadmitted vRPC, got: %v", res11.err)
	}
}



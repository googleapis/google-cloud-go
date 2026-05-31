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

//go:build synctest

package internal

import (
	"context"
	"sync"
	"testing"
	"testing/synctest"
	"time"

	spb "cloud.google.com/go/bigtable/apiv2/bigtablepb"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/durationpb"
)

func TestSession_MissedHeartbeat(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		stream := newMockSessionStream()
		defer close(stream.recvChan)
		listener := &mockSessionListener{}
		session := NewSession("test-session", stream, listener, SessionTypeTable)

		// Pre-activate the session
		session.state = StateActive
		close(session.handshakeDone)

		// Register an active VRPC
		resChan := make(chan vrpcResult, 1)
		session.mu.Lock()
		session.activeRPCs[42] = &VRPCImpl{
			id:         42,
			method:     "TestMethod",
			resultChan: resChan,
		}
		// Configure watchdog to trigger immediately: set interval small, and next deadline to past
		session.heartbeatInterval = 10 * time.Millisecond
		session.nextHeartbeatDeadline = time.Now().Add(-1 * time.Second)
		session.mu.Unlock()

		// Start the heartbeat loop
		go session.heartBeatLoop(ctx)

		// Wait for the active RPC to be aborted due to missed heartbeat
		var res vrpcResult
		select {
		case res = <-resChan:
		case <-time.After(2 * time.Second):
			t.Fatal("Timed out waiting for vRPC to be aborted by missed heartbeat")
		}

		if res.err == nil {
			t.Fatal("Expected vRPC to fail, got success")
		}

		st, ok := status.FromError(res.err)
		if !ok || st.Code() != codes.Unavailable {
			t.Errorf("Expected Unavailable status error, got: %v", res.err)
		}

		session.mu.Lock()
		state := session.state
		session.mu.Unlock()

		if state != StateClosed {
			t.Errorf("Expected session state to transition to StateClosed, got %v", state)
		}

		listener.mu.Lock()
		if !listener.closed {
			t.Error("Expected OnClose to be called on listener")
		}
		listener.mu.Unlock()
	})
}

type testMockPicker struct {
	sh1 *SessionHandle
	sh2 *SessionHandle
	cnt int
}

func (p *testMockPicker) PickSession() *SessionHandle {
	if p.cnt == 0 {
		p.cnt++
		return p.sh1
	}
	return p.sh2
}

func TestSessionPool_RetryingVRpc_ChannelFailover(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		// 1. Setup mock streams and sessions
		stream1 := newMockSessionStream()
		defer close(stream1.recvChan)
		session1 := NewSession("test-session-1", stream1, nil, SessionTypeTable)
		session1.state = StateActive
		close(session1.handshakeDone)
		go session1.readLoop(ctx)

		stream2 := newMockSessionStream()
		defer close(stream2.recvChan)
		session2 := NewSession("test-session-2", stream2, nil, SessionTypeTable)
		session2.state = StateActive
		close(session2.handshakeDone)
		go session2.readLoop(ctx)

		sh1 := NewSessionHandle(session1)
		sh2 := NewSessionHandle(session2)

		// 2. Construct a SessionPoolImpl pre-populated with both sessions
		pool := &SessionPoolImpl{
			poolName: "test-pool",
			sessions: []*SessionHandle{sh1, sh2},
		}
		pool.picker = &testMockPicker{sh1: sh1, sh2: sh2}

		// 3. Define the baseHandler that calls pool.ExecuteVRpc
		desc := &mockVRpcDescriptor{method: "TestMethod"}
		baseHandler := func(attemptCtx context.Context, request interface{}) (interface{}, error) {
			resp, _, err := pool.ExecuteVRpc(attemptCtx, desc, request)
			return resp, err
		}

		// 4. Wrap baseHandler with RetryingVRpc interceptor
		retryInterceptor := RetryingVRpc(RetryingOptions{
			MaxAttempts:    3,
			InitialBackoff: 1 * time.Millisecond,
		})

		// 5. Execute the vRPC in a separate goroutine so we can control/fail the first session
		var respVal interface{}
		var errVal error
		var wg sync.WaitGroup
		wg.Add(1)
		go func() {
			defer wg.Done()
			respVal, errVal = retryInterceptor(ctx, "request_payload", baseHandler)
		}()

		// Wait a brief moment to ensure session1 is selected and request is sent
		time.Sleep(50 * time.Millisecond)

		stream1.mu.Lock()
		if len(stream1.sent) == 0 {
			stream1.mu.Unlock()
			t.Fatal("Expected request to be sent on stream1")
		}
		sentReq1 := stream1.sent[0]
		stream1.mu.Unlock()

		vrpcReq1 := sentReq1.GetVirtualRpc()
		if vrpcReq1 == nil {
			t.Fatal("Expected SessionRequest to contain VirtualRpc payload")
		}

		// Simulate missed heartbeat / ForceClose on session1
		session1.ForceClose(&spb.CloseSessionRequest{
			Reason:      spb.CloseSessionRequest_CLOSE_SESSION_REASON_MISSED_HEARTBEAT,
			Description: "session client terminated due to missed server heartbeats keepalive",
		})

		// Wait for retry and check that the request is sent to session2 / stream2
		time.Sleep(50 * time.Millisecond)

		stream2.mu.Lock()
		if len(stream2.sent) == 0 {
			stream2.mu.Unlock()
			t.Fatal("Expected request to be sent on stream2 after session1 failover")
		}
		sentReq2 := stream2.sent[0]
		stream2.mu.Unlock()

		vrpcReq2 := sentReq2.GetVirtualRpc()
		if vrpcReq2 == nil {
			t.Fatal("Expected SessionRequest on stream2 to contain VirtualRpc payload")
		}

		// Inject successful mock response on stream2 matching its rpcID
		stream2.recvChan <- &spb.SessionResponse{
			Payload: &spb.SessionResponse_VirtualRpc{
				VirtualRpc: &spb.VirtualRpcResponse{
					RpcId:   vrpcReq2.RpcId,
					Payload: []byte("response_payload_from_session2"),
				},
			},
		}

		wg.Wait()

		if errVal != nil {
			t.Fatalf("Expected successful vRPC execution after failover, got error: %v", errVal)
		}

		if respVal.(string) != "response_payload_from_session2" {
			t.Errorf("Expected response payload %q, got %q", "response_payload_from_session2", respVal.(string))
		}
	})
}

type testSuccessivePicker struct {
	shs []*SessionHandle
	cnt int
}

func (p *testSuccessivePicker) PickSession() *SessionHandle {
	if p.cnt >= len(p.shs) {
		return nil
	}
	res := p.shs[p.cnt]
	p.cnt++
	return res
}

func TestSessionPool_RetryingVRpc_SuccessiveFailover(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		// 1. Setup 3 mock streams and sessions
		stream1 := newMockSessionStream()
		defer close(stream1.recvChan)
		session1 := NewSession("test-session-1", stream1, nil, SessionTypeTable)
		session1.state = StateActive
		close(session1.handshakeDone)
		go session1.readLoop(ctx)

		stream2 := newMockSessionStream()
		defer close(stream2.recvChan)
		session2 := NewSession("test-session-2", stream2, nil, SessionTypeTable)
		session2.state = StateActive
		close(session2.handshakeDone)
		go session2.readLoop(ctx)

		stream3 := newMockSessionStream()
		defer close(stream3.recvChan)
		session3 := NewSession("test-session-3", stream3, nil, SessionTypeTable)
		session3.state = StateActive
		close(session3.handshakeDone)
		go session3.readLoop(ctx)

		sh1 := NewSessionHandle(session1)
		sh2 := NewSessionHandle(session2)
		sh3 := NewSessionHandle(session3)

		// 2. Construct pool
		pool := &SessionPoolImpl{
			poolName: "test-pool",
			sessions: []*SessionHandle{sh1, sh2, sh3},
		}
		pool.picker = &testSuccessivePicker{shs: []*SessionHandle{sh1, sh2, sh3}}

		desc := &mockVRpcDescriptor{method: "TestMethod"}
		baseHandler := func(attemptCtx context.Context, request interface{}) (interface{}, error) {
			resp, _, err := pool.ExecuteVRpc(attemptCtx, desc, request)
			return resp, err
		}

		retryInterceptor := RetryingVRpc(RetryingOptions{
			MaxAttempts:    4,
			InitialBackoff: 1 * time.Millisecond,
		})

		var respVal interface{}
		var errVal error
		var wg sync.WaitGroup
		wg.Add(1)
		go func() {
			defer wg.Done()
			respVal, errVal = retryInterceptor(ctx, "request_payload", baseHandler)
		}()

		// Fail session1
		time.Sleep(50 * time.Millisecond)
		session1.ForceClose(&spb.CloseSessionRequest{
			Reason:      spb.CloseSessionRequest_CLOSE_SESSION_REASON_MISSED_HEARTBEAT,
			Description: "session client terminated due to missed server heartbeats keepalive",
		})

		// Fail session2
		time.Sleep(50 * time.Millisecond)
		session2.ForceClose(&spb.CloseSessionRequest{
			Reason:      spb.CloseSessionRequest_CLOSE_SESSION_REASON_MISSED_HEARTBEAT,
			Description: "session client terminated due to missed server heartbeats keepalive",
		})

		// Succeed session3
		time.Sleep(50 * time.Millisecond)
		stream3.mu.Lock()
		if len(stream3.sent) == 0 {
			stream3.mu.Unlock()
			t.Fatal("Expected request to be sent on stream3 after multiple failovers")
		}
		sentReq3 := stream3.sent[0]
		stream3.mu.Unlock()

		vrpcReq3 := sentReq3.GetVirtualRpc()
		if vrpcReq3 == nil {
			t.Fatal("Expected SessionRequest on stream3 to contain VirtualRpc payload")
		}

		stream3.recvChan <- &spb.SessionResponse{
			Payload: &spb.SessionResponse_VirtualRpc{
				VirtualRpc: &spb.VirtualRpcResponse{
					RpcId:   vrpcReq3.RpcId,
					Payload: []byte("success_on_session3"),
				},
			},
		}

		wg.Wait()

		if errVal != nil {
			t.Fatalf("Expected successful execution on third attempt, got error: %v", errVal)
		}
		if respVal.(string) != "success_on_session3" {
			t.Errorf("Expected 'success_on_session3', got %q", respVal.(string))
		}
	})
}

func TestSessionPool_RetryingVRpc_MaxAttemptsExceeded(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		stream1 := newMockSessionStream()
		defer close(stream1.recvChan)
		session1 := NewSession("test-session-1", stream1, nil, SessionTypeTable)
		session1.state = StateActive
		close(session1.handshakeDone)
		go session1.readLoop(ctx)

		stream2 := newMockSessionStream()
		defer close(stream2.recvChan)
		session2 := NewSession("test-session-2", stream2, nil, SessionTypeTable)
		session2.state = StateActive
		close(session2.handshakeDone)
		go session2.readLoop(ctx)

		sh1 := NewSessionHandle(session1)
		sh2 := NewSessionHandle(session2)

		pool := &SessionPoolImpl{
			poolName: "test-pool",
			sessions: []*SessionHandle{sh1, sh2},
		}
		pool.picker = &testSuccessivePicker{shs: []*SessionHandle{sh1, sh2}}

		desc := &mockVRpcDescriptor{method: "TestMethod"}
		baseHandler := func(attemptCtx context.Context, request interface{}) (interface{}, error) {
			resp, _, err := pool.ExecuteVRpc(attemptCtx, desc, request)
			return resp, err
		}

		// MaxAttempts = 2
		retryInterceptor := RetryingVRpc(RetryingOptions{
			MaxAttempts:    2,
			InitialBackoff: 1 * time.Millisecond,
		})

		var respVal interface{}
		var errVal error
		var wg sync.WaitGroup
		wg.Add(1)
		go func() {
			defer wg.Done()
			respVal, errVal = retryInterceptor(ctx, "request_payload", baseHandler)
		}()

		// Fail session1
		time.Sleep(50 * time.Millisecond)
		session1.ForceClose(&spb.CloseSessionRequest{
			Reason:      spb.CloseSessionRequest_CLOSE_SESSION_REASON_MISSED_HEARTBEAT,
			Description: "session client terminated due to missed server heartbeats keepalive",
		})

		// Fail session2
		time.Sleep(50 * time.Millisecond)
		session2.ForceClose(&spb.CloseSessionRequest{
			Reason:      spb.CloseSessionRequest_CLOSE_SESSION_REASON_MISSED_HEARTBEAT,
			Description: "session client terminated due to missed server heartbeats keepalive",
		})

		wg.Wait()

		if errVal == nil {
			t.Fatal("Expected error due to max attempts exceeded, got success")
		}
		st, ok := status.FromError(errVal)
		if !ok || st.Code() != codes.Unavailable {
			t.Errorf("Expected Unavailable status, got %v", errVal)
		}
		_ = respVal
	})
}

func TestSessionPool_RetryingVRpc_ContextCancelled(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())

		stream1 := newMockSessionStream()
		defer close(stream1.recvChan)
		session1 := NewSession("test-session-1", stream1, nil, SessionTypeTable)
		session1.state = StateActive
		close(session1.handshakeDone)
		go session1.readLoop(ctx)

		stream2 := newMockSessionStream()
		defer close(stream2.recvChan)
		session2 := NewSession("test-session-2", stream2, nil, SessionTypeTable)
		session2.state = StateActive
		close(session2.handshakeDone)
		go session2.readLoop(ctx)

		sh1 := NewSessionHandle(session1)
		sh2 := NewSessionHandle(session2)

		pool := &SessionPoolImpl{
			poolName: "test-pool",
			sessions: []*SessionHandle{sh1, sh2},
		}
		pool.picker = &testSuccessivePicker{shs: []*SessionHandle{sh1, sh2}}

		desc := &mockVRpcDescriptor{method: "TestMethod"}
		baseHandler := func(attemptCtx context.Context, request interface{}) (interface{}, error) {
			resp, _, err := pool.ExecuteVRpc(attemptCtx, desc, request)
			return resp, err
		}

		retryInterceptor := RetryingVRpc(RetryingOptions{
			MaxAttempts:    3,
			InitialBackoff: 100 * time.Millisecond,
		})

		var respVal interface{}
		var errVal error
		var wg sync.WaitGroup
		wg.Add(1)
		go func() {
			defer wg.Done()
			respVal, errVal = retryInterceptor(ctx, "request_payload", baseHandler)
		}()

		// Fail session1
		time.Sleep(50 * time.Millisecond)
		session1.ForceClose(&spb.CloseSessionRequest{
			Reason:      spb.CloseSessionRequest_CLOSE_SESSION_REASON_MISSED_HEARTBEAT,
			Description: "session client terminated due to missed server heartbeats keepalive",
		})

		// Cancel the parent context while it is backoff waiting
		cancel()

		wg.Wait()

		if errVal == nil {
			t.Fatal("Expected error due to context cancellation, got success")
		}
		if errVal != context.Canceled {
			t.Errorf("Expected context.Canceled error, got %v", errVal)
		}
		_ = respVal
	})
}

func TestSession_SessionParametersUpdatesHeartbeatInterval(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		stream := newMockSessionStream()
		defer close(stream.recvChan)
		session := NewSession("test-session", stream, nil, SessionTypeTable)

		// Pre-activate the session
		session.state = StateActive
		close(session.handshakeDone)
		go session.readLoop(ctx)

		// Verify default heartbeat interval
		session.mu.Lock()
		defaultInterval := session.heartbeatInterval
		session.mu.Unlock()
		if defaultInterval != 10 * time.Second {
			t.Errorf("Expected default heartbeatInterval to be 10s, got %v", defaultInterval)
		}

		// Inject SessionParametersResponse containing KeepAlive = 5s
		stream.recvChan <- &spb.SessionResponse{
			Payload: &spb.SessionResponse_SessionParameters{
				SessionParameters: &spb.SessionParametersResponse{
					KeepAlive: durationpb.New(5 * time.Second),
				},
			},
		}

		// Wait for readLoop to process response
		time.Sleep(50 * time.Millisecond)

		session.mu.Lock()
		updatedInterval := session.heartbeatInterval
		session.mu.Unlock()

		if updatedInterval != 5 * time.Second {
			t.Errorf("Expected updated heartbeatInterval to be 5s, got %v", updatedInterval)
		}
	})
}



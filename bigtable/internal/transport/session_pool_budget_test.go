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

// Budget lifecycle tests that exercise the full createSession → Start() →
// readLoop → onActive/onClose path against a real gRPC server. These tests
// exist to pin two bugs that direct-injection tests (using injectStartingSession
// + p.onActive / p.onClose) cannot catch:
//
//  1. Budget slot freed too early — Release(true) was called right after
//     Start() returned instead of waiting for OpenSessionResponse.
//  2. Penalty not applied on async rejection — Release(false) was never
//     reached because Release(true) fired first.

package internal

import (
	"context"
	"net"
	"runtime"
	"testing"
	"time"

	spb "cloud.google.com/go/bigtable/apiv2/bigtablepb"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/status"
	"google.golang.org/grpc/test/bufconn"
)

// openTableStreamHandler drives a single server-side OpenTable bidi stream.
type openTableStreamHandler func(spb.Bigtable_OpenTableServer) error

// openTableBigtableServer is a minimal BigtableServer that routes each
// OpenTable call to a test-supplied handler.
type openTableBigtableServer struct {
	spb.UnimplementedBigtableServer
	handler openTableStreamHandler
}

func (s *openTableBigtableServer) OpenTable(stream spb.Bigtable_OpenTableServer) error {
	return s.handler(stream)
}

// newGRPCBackedPool starts a bufconn gRPC server whose OpenTable RPC is
// controlled by handler, and returns a SessionPoolImpl whose streamFactory
// dials it. Budget cap is 1 so slot occupancy is easy to observe. The pool
// and server are both torn down via t.Cleanup.
func newGRPCBackedPool(t *testing.T, handler openTableStreamHandler) *SessionPoolImpl {
	t.Helper()
	lis := bufconn.Listen(1 << 20)
	srv := grpc.NewServer()
	spb.RegisterBigtableServer(srv, &openTableBigtableServer{handler: handler})
	go srv.Serve(lis) //nolint:errcheck

	conn, err := grpc.Dial( //nolint:staticcheck
		"passthrough:///bufnet",
		grpc.WithContextDialer(func(context.Context, string) (net.Conn, error) { return lis.Dial() }),
		grpc.WithTransportCredentials(insecure.NewCredentials()),
	)
	if err != nil {
		t.Fatalf("grpc.Dial: %v", err)
	}
	btClient := spb.NewBigtableClient(conn)
	factory := func(ctx context.Context) (Stream, error) { return btClient.OpenTable(ctx) }

	p := NewSessionPoolImpl(1, "budget-test", factory, &spb.OpenSessionRequest{}, nil, SessionTypeTable, false)
	p.budget.UpdateConfig(1, time.Minute) // cap=1 makes slot occupancy easy to observe
	p.sizer.UpdateConfig(&spb.SessionClientConfiguration_SessionPoolConfiguration{
		MinSessionCount: 0, MaxSessionCount: 10,
	})
	t.Cleanup(func() {
		_ = p.Close()
		srv.Stop()
		_ = conn.Close()
		_ = lis.Close()
	})
	return p
}

// spawnOneCreateSession mirrors Tick's goroutine management: increments
// pendingStarts and spawns.Add(1) before launching createSession, exactly
// as createSession expects when called from outside Tick.
func spawnOneCreateSession(p *SessionPoolImpl) {
	p.mu.Lock()
	p.pendingStarts++
	p.spawns.Add(1)
	p.mu.Unlock()
	go func() {
		defer p.spawns.Done()
		_ = p.createSession(p.poolCtx)
	}()
}

// pollUntil spins-sleeps until cond() returns true or timeout elapses,
// then calls t.Fatal if the condition was never met.
func pollUntil(t *testing.T, timeout time.Duration, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if cond() {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatal("pollUntil: condition not met within timeout")
}

// TestBudget_SlotHeldUntilHandshake pins that the budget slot acquired by
// createSession is NOT released when Start() returns — it must be held until
// the OpenSessionResponse frame arrives and onActive fires. The test uses a
// real gRPC server (bufconn) so the full createSession → Start() → readLoop
// → handleOpenSession → onActive path is exercised.
//
// Before the fix, Release(true) fired immediately after Start() returned,
// freeing the slot while the handshake was still in-flight over the wire.
func TestBudget_SlotHeldUntilHandshake(t *testing.T) {
	requestReceived := make(chan struct{})
	okToRespond := make(chan struct{})

	p := newGRPCBackedPool(t, func(srv spb.Bigtable_OpenTableServer) error {
		// Drain the client's OpenSessionRequest.
		if _, err := srv.Recv(); err != nil {
			return err
		}
		// Signal that the client's Send completed (Start() has returned in
		// createSession), then pause before replying.
		close(requestReceived)
		select {
		case <-okToRespond:
		case <-srv.Context().Done():
			return srv.Context().Err()
		}
		// Confirm the session — triggers onActive on the client side.
		if err := srv.Send(&spb.SessionResponse{
			Payload: &spb.SessionResponse_OpenSession{OpenSession: &spb.OpenSessionResponse{}},
		}); err != nil {
			return err
		}
		// Hold the stream alive until the pool tears it down.
		<-srv.Context().Done()
		return nil
	})

	spawnOneCreateSession(p)

	// Wait for the server to receive the OpenSessionRequest. At this point
	// Start() has returned in the createSession goroutine. Sleep briefly so
	// any Release(true) that the old code placed immediately after Start()
	// would have had time to execute before we assert.
	select {
	case <-requestReceived:
	case <-time.After(5 * time.Second):
		t.Fatal("timeout waiting for server to receive OpenSessionRequest")
	}
	runtime.Gosched()
	time.Sleep(20 * time.Millisecond)

	// Slot must still be held: OpenSessionResponse not yet sent.
	if snap := p.budget.Snapshot(); snap.InUse != 1 {
		t.Errorf("InUse before OpenSessionResponse = %d, want 1 (slot must be held during handshake)", snap.InUse)
	}

	// Unblock the server → client readLoop receives OpenSessionResponse →
	// handleOpenSession → onActive → Release(true) frees the slot.
	close(okToRespond)
	pollUntil(t, 5*time.Second, func() bool { return p.sl.ReadyCount() == 1 })

	if snap := p.budget.Snapshot(); snap.InUse != 0 {
		t.Errorf("InUse after OpenSessionResponse = %d, want 0 (slot released on handshake)", snap.InUse)
	}
}

// TestBudget_PenaltyOnAsyncRejection pins that when the server rejects the
// session asynchronously (stream closed without an OpenSessionResponse),
// onClose's starting branch calls Release(false) so the failure penalty slot
// is recorded. The test uses a real gRPC server (bufconn) to drive the full
// createSession → Start() → readLoop → handleClose → onClose path.
//
// Before the fix, Release(true) fired right after Start() and Release(false)
// was never reached — the retry loop ran unthrottled.
func TestBudget_PenaltyOnAsyncRejection(t *testing.T) {
	requestReceived := make(chan struct{})

	p := newGRPCBackedPool(t, func(srv spb.Bigtable_OpenTableServer) error {
		// Drain the OpenSessionRequest then immediately close with an error.
		if _, err := srv.Recv(); err != nil {
			return err
		}
		close(requestReceived)
		return status.Errorf(codes.Unavailable, "server rejected OpenSession")
	})

	spawnOneCreateSession(p)

	select {
	case <-requestReceived:
	case <-time.After(5 * time.Second):
		t.Fatal("timeout waiting for server to receive OpenSessionRequest")
	}

	// Wait for onClose's starting branch to delete sh from startingSessions.
	// After that delete (under p.mu), Release(false) is called in the same
	// goroutine. Sleep briefly so that Release(false) has run before we check.
	pollUntil(t, 5*time.Second, func() bool {
		p.mu.Lock()
		defer p.mu.Unlock()
		return len(p.startingSessions) == 0
	})
	time.Sleep(20 * time.Millisecond)

	// InUse (= live slots + penalty slots) must be 1: Release(false) recorded
	// a penalty for penaltyDuration instead of freeing the slot immediately.
	if snap := p.budget.Snapshot(); snap.InUse != 1 {
		t.Errorf("InUse after async rejection = %d, want 1 (penalty slot must hold)", snap.InUse)
	}
}

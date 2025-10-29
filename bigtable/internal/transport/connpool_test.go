// Copyright 2025 Google LLC
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
	"io"
	"log"
	"math"
	"net"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	btpb "cloud.google.com/go/bigtable/apiv2/bigtablepb"

	btopt "cloud.google.com/go/bigtable/internal/option"
	"go.opentelemetry.io/otel/attribute"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/metric/metricdata"
	"google.golang.org/api/option"
	gtransport "google.golang.org/api/transport/grpc"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/connectivity"

	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/status"

	testpb "google.golang.org/grpc/interop/grpc_testing"
)

type fakeService struct {
	testpb.UnimplementedBenchmarkServiceServer
	btpb.UnimplementedBigtableServer // Embed BigtableServer
	mu                               sync.Mutex
	pingCount                        int
	callCount                        int
	streamSema                       chan struct{} // To control stream lifetime
	delay                            time.Duration // To simulate work
	serverErr                        error         // Error to return from server
	pingErr                          error         // Error to return from PingAndWarm
	pingErrMu                        sync.Mutex    // Protects pingErr

	streamRecvErr error // Error to return from stream.Recv()
	streamSendErr error // Error to return from stream.Send()
}

func (s *fakeService) setPingErr(err error) {
	s.pingErrMu.Lock()
	defer s.pingErrMu.Unlock()
	s.pingErr = err
}

func (s *fakeService) setDelay(duration time.Duration) {
	s.pingErrMu.Lock()
	defer s.pingErrMu.Unlock()
	s.delay = duration
}

func (s *fakeService) getDelay() time.Duration {
	s.pingErrMu.Lock()
	defer s.pingErrMu.Unlock()
	return s.delay
}

func (s *fakeService) getPingErr() error {
	s.pingErrMu.Lock()
	defer s.pingErrMu.Unlock()
	return s.pingErr
}

func (s *fakeService) UnaryCall(ctx context.Context, req *testpb.SimpleRequest) (*testpb.SimpleResponse, error) {
	s.mu.Lock()
	s.callCount++
	s.mu.Unlock()
	if s.delay > 0 {
		time.Sleep(s.delay)
	}
	if s.serverErr != nil {
		return nil, s.serverErr
	}
	return &testpb.SimpleResponse{Payload: req.GetPayload()}, nil
}

func (s *fakeService) StreamingCall(stream testpb.BenchmarkService_StreamingCallServer) error {
	s.mu.Lock()
	s.callCount++
	s.mu.Unlock()

	if s.serverErr != nil {
		return s.serverErr
	}

	if s.streamSema != nil {
		<-s.streamSema
	}

	for {
		req, err := stream.Recv()
		if err == io.EOF {
			return nil
		}
		if err != nil {
			return err
		}
		if s.streamRecvErr != nil {
			return s.streamRecvErr
		}

		if err := stream.Send(&testpb.SimpleResponse{Payload: req.GetPayload()}); err != nil {
			return err
		}
		if s.streamSendErr != nil {
			return s.streamSendErr
		}
	}
}

func (s *fakeService) PingAndWarm(ctx context.Context, req *btpb.PingAndWarmRequest) (*btpb.PingAndWarmResponse, error) {
	s.mu.Lock()
	s.pingCount++
	defer s.mu.Unlock()

	delay := s.getDelay()

	if delay > 0 {
		select {
		case <-time.After(delay):
			// Delay finished
		case <-ctx.Done():
			// Context cancelled during delay
			return nil, ctx.Err()
		}
	}

	// Check context again after potential delay
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	if err := s.getPingErr(); err != nil {
		return nil, err
	}
	return &btpb.PingAndWarmResponse{}, nil
}

func (s *fakeService) getCallCount() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.callCount
}

func (s *fakeService) getPingCount() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.pingCount
}

func (s *fakeService) setPingCount(count int) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.pingCount = count
}

func setupTestServer(t testing.TB, service *fakeService) string {
	t.Helper()
	lis, err := net.Listen("tcp", "localhost:0")
	if err != nil {
		t.Fatalf("Failed to listen: %v", err)
	}
	srv := grpc.NewServer()
	testpb.RegisterBenchmarkServiceServer(srv, service)
	btpb.RegisterBigtableServer(srv, service)
	go func() {
		if err := srv.Serve(lis); err != nil {
			// t.Logf("gRPC server error: %v", err) // Avoid logging in tight test loops if server is stopped
		}
	}()

	t.Cleanup(func() {
		srv.Stop()
		lis.Close()
	})

	return lis.Addr().String()
}

func dialBigtableserver(addr string) (*BigtableConn, error) {
	conn, err := grpc.Dial(addr, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		return nil, err
	}
	return NewBigtableConn(conn, "test-instance", "test-profile"), nil
}

func dialBigtableserverWithInstanceNameAndAppProfile(addr string, instanceName, appProfile string) (*BigtableConn, error) {
	conn, err := grpc.Dial(addr, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		return nil, err
	}
	return NewBigtableConn(conn, instanceName, appProfile), nil
}

func TestSelectRoundRobin(t *testing.T) {
	pool := &BigtableChannelPool{rrIndex: 0}

	// Test empty pool
	entry, err := pool.selectRoundRobin()
	if entry != nil {
		t.Errorf("selectRoundRobin on empty pool got entry, want nil")
	}
	if !errors.Is(err, errNoConnections) {
		t.Errorf("selectRoundRobin on empty pool got error %v, want %v", err, errNoConnections)
	}

	// Test single connection pool
	pool.conns.Store([]*connEntry{{}})
	entry, err = pool.selectRoundRobin()
	if entry == nil {
		t.Errorf("selectRoundRobin on single conn pool got nil entry")
	}
	if err != nil {
		t.Errorf("selectRoundRobin on single conn pool got error %v, want nil", err)
	}

	// Test multiple connections
	poolSize := 3
	conns := make([]*connEntry, poolSize)
	for i := 0; i < poolSize; i++ {
		conns[i] = &connEntry{}
	}
	pool.conns.Store(conns)
	pool.rrIndex = 0

	for i := 0; i < poolSize*2; i++ {
		expectedIdx := i % poolSize
		entry, err = pool.selectRoundRobin()
		if err != nil {
			t.Errorf("selectRoundRobin call %d got error %v, want nil", i+1, err)
			continue
		}
		if entry != conns[expectedIdx] {
			t.Errorf("selectRoundRobin call %d got entry for index %d, want %d", i+1, entryIndex(conns, entry), expectedIdx)
		}
	}
}

func entryIndex(s []*connEntry, e *connEntry) int {
	for i, item := range s {
		if item == e {
			return i
		}
	}
	return -1
}

func TestNewBigtableChannelPoolEdgeCases(t *testing.T) {
	ctx := context.Background()
	fake := &fakeService{}
	addr := setupTestServer(t, fake)
	dialFunc := func() (*BigtableConn, error) { return dialBigtableserver(addr) }

	tests := []struct {
		name     string
		size     int
		dial     func() (*BigtableConn, error)
		wantErr  bool
		errMatch string
	}{
		{name: "ZeroSize", size: 0, dial: dialFunc, wantErr: true, errMatch: "must be positive"},
		{name: "NegativeSize", size: -1, dial: dialFunc, wantErr: true, errMatch: "must be positive"},
		{name: "NilDial", size: 1, dial: nil, wantErr: true, errMatch: "dial function cannot be nil"},
		{name: "Valid", size: 1, dial: dialFunc, wantErr: false},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			pool, err := NewBigtableChannelPool(ctx, tc.size, btopt.RoundRobin, tc.dial, nil, nil)
			if tc.wantErr {
				if err == nil {
					t.Errorf("NewBigtableChannelPool(%d) succeeded, want error containing %q", tc.size, tc.errMatch)
				} else if !strings.Contains(err.Error(), tc.errMatch) {
					t.Errorf("NewBigtableChannelPool(%d) got error %v, want error containing %q", tc.size, err, tc.errMatch)
				}
				if pool != nil {
					pool.Close() // Cleanup if unexpectedly created
				}
			} else {
				if err != nil {
					t.Errorf("NewBigtableChannelPool(%d) failed: %v", tc.size, err)
				}
				if pool != nil {
					pool.Close()
				}
			}
		})
	}
}

func TestBigtableConn_Prime(t *testing.T) {
	ctx := context.Background()
	fake := &fakeService{}
	addr := setupTestServer(t, fake)

	t.Run("SuccessfulPrime", func(t *testing.T) {
		conn, err := dialBigtableserverWithInstanceNameAndAppProfile(addr, "my-instance", "my-profile")
		if err != nil {
			t.Fatalf("Failed to dial: %v", err)
		}
		defer conn.Close()
		fake.setPingCount(0)
		isALTS, err := conn.Prime(ctx)
		if err != nil {
			t.Errorf("Prime() failed: %v", err)
		}
		if isALTS {
			t.Errorf("Prime() got isALTS true, want false")
		}
		if fake.getPingCount() != 1 {
			t.Errorf("PingAndWarm call count got %d, want 1", fake.getPingCount())
		}
	})

	testCases := []struct {
		name         string
		instanceName string
		appProfile   string
		pingErr      error
		wantErrCode  codes.Code
		wantErrMsg   string
	}{
		{
			name:         "MissingInstanceName",
			instanceName: "",
			appProfile:   "my-profile",
			wantErrCode:  codes.FailedPrecondition,
			wantErrMsg:   "instanceName is required",
		},
		{
			name:         "MissingAppProfile",
			instanceName: "my-instance",
			appProfile:   "",
			wantErrCode:  codes.FailedPrecondition,
			wantErrMsg:   "appProfile is required",
		},
		{
			name:         "ServerPingError",
			instanceName: "my-instance",
			appProfile:   "my-profile",
			pingErr:      status.Error(codes.Internal, "simulated ping error"),
			wantErrCode:  codes.Internal,
			wantErrMsg:   "simulated ping error",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			conn, err := dialBigtableserverWithInstanceNameAndAppProfile(addr, tc.instanceName, tc.appProfile)
			if err != nil {
				t.Fatalf("Failed to dial: %v", err)
			}
			defer conn.Close()

			fake.setPingErr(tc.pingErr)
			defer func() { fake.setPingErr(nil) }()

			_, err = conn.Prime(ctx)
			if err == nil {
				t.Fatalf("Prime() succeeded, want error")
			}

			st, ok := status.FromError(err)
			if !ok {
				t.Fatalf("Prime() returned non-status error: %v", err)
			}
			if st.Code() != tc.wantErrCode {
				t.Errorf("Prime() got error code %v, want %v", st.Code(), tc.wantErrCode)
			}
			if !strings.Contains(st.Message(), tc.wantErrMsg) {
				t.Errorf("Prime() got error message %q, want message containing %q", st.Message(), tc.wantErrMsg)
			}
		})
	}

	t.Run("PrimeTimeout", func(t *testing.T) {
		conn, err := dialBigtableserverWithInstanceNameAndAppProfile(addr, "my-instance", "my-profile")
		if err != nil {
			t.Fatalf("Failed to dial: %v", err)
		}
		defer conn.Close()

		origDelay := fake.delay
		fake.delay = 20 * time.Second // Longer than primeRPCTimeout
		defer func() {
			fake.setDelay(origDelay)
		}()

		primeCtx, cancel := context.WithTimeout(ctx, 100*time.Millisecond) // Shorter than primeRPCTimeout
		defer cancel()

		_, err = conn.Prime(primeCtx)
		if err == nil {
			t.Fatalf("Prime() succeeded, want timeout error")
		}
		st, ok := status.FromError(err)
		if !ok {
			t.Fatalf("Prime() returned non-status error: %v", err)
		}
		if st.Code() != codes.DeadlineExceeded {
			t.Errorf("Prime() got error code %v, want %v", st.Code(), codes.DeadlineExceeded)
		}
	})
}

func TestSelectLeastLoadedRandomOfTwo(t *testing.T) {
	pool := &BigtableChannelPool{}

	entry, err := pool.selectLeastLoadedRandomOfTwo()
	if entry != nil || !errors.Is(err, errNoConnections) {
		t.Errorf("Empty pool: got %v, %v, want nil, %v", entry, err, errNoConnections)
	}

	conns := []*connEntry{{}}
	pool.conns.Store(conns)
	entry, err = pool.selectLeastLoadedRandomOfTwo()
	if entry != conns[0] || err != nil {
		t.Errorf("Single conn: got %v, %v, want %v, nil", entry, err, conns[0])
	}

	// streamingLoadFactor is 2, unaryLoadFactor is 1
	testLoads := []struct{ unary, stream int32 }{
		{10, 0}, // Load: 10
		{0, 4},  // Load: 8
		{20, 5}, // Load: 30
		{2, 1},  // Load: 4
		{0, 20}, // Load: 40
	}
	conns = make([]*connEntry, len(testLoads))
	for i, loads := range testLoads {
		conns[i] = &connEntry{}
		atomic.StoreInt32(&conns[i].unaryLoad, loads.unary)
		atomic.StoreInt32(&conns[i].streamingLoad, loads.stream)
	}
	pool.conns.Store(conns)
	for i := 0; i < 100; i++ {
		entry, err = pool.selectLeastLoadedRandomOfTwo()
		if err != nil {
			t.Fatalf("Multi conn: got unexpected error: %v", err)
		}
		if entry == nil {
			t.Fatalf("Multi conn: got nil entry")
		}
		// We can't deterministically know which one was chosen, just that one was.
	}
}

func TestNewBigtableChannelPool(t *testing.T) {
	ctx := context.Background()

	t.Run("SuccessfulCreation", func(t *testing.T) {
		poolSize := 5
		fake := &fakeService{}
		addr := setupTestServer(t, fake)
		dialFunc := func() (*BigtableConn, error) { return dialBigtableserver(addr) }

		pool, err := NewBigtableChannelPool(ctx, poolSize, btopt.LeastInFlight, dialFunc, nil, nil)
		if err != nil {
			t.Fatalf("NewBigtableChannelPool failed: %v", err)
		}
		defer pool.Close()

		if pool.Num() != poolSize {
			t.Errorf("Pool size got %d, want %d", pool.Num(), poolSize)
		}
		conns := pool.getConns()
		if len(conns) != poolSize {
			t.Fatalf("getConns() length got %d, want %d", len(conns), poolSize)
		}
		// Wait for priming goroutines to likely complete
		time.Sleep(100 * time.Millisecond)

		for i, entry := range conns {
			if entry == nil || entry.conn == nil {
				t.Errorf("conn at index %d is nil", i)
				continue
			}
			if entry.isALTSUsed() {
				t.Errorf("conn at index %d isALTSUsed() got true, want false", i)
			}
		}
		if pool.healthMonitor != nil {
			t.Errorf("Health monitor was  created")
		}

		if pool.dynamicMonitor != nil {
			t.Errorf("Dynamic monitor was  created")
		}
	})

	t.Run("DialFailure", func(t *testing.T) {
		poolSize := 3
		dialCount := 0
		dialFunc := func() (*BigtableConn, error) {
			dialCount++
			if dialCount > 1 {
				return nil, errors.New("simulated dial error")
			}
			fake := &fakeService{}
			addr := setupTestServer(t, fake)
			return dialBigtableserver(addr)
		}

		_, err := NewBigtableChannelPool(ctx, poolSize, btopt.LeastInFlight, dialFunc, nil, nil)
		if err == nil {
			t.Errorf("NewBigtableChannelPool should have failed due to dial error")
		}
	})
}

func TestPoolInvoke(t *testing.T) {
	ctx := context.Background()
	strategies := []btopt.LoadBalancingStrategy{
		btopt.LeastInFlight,
		btopt.RoundRobin,
		btopt.PowerOfTwoLeastInFlight,
	}

	for _, strategy := range strategies {
		t.Run(fmt.Sprintf("Strategy_%s", strategy), func(t *testing.T) {
			poolSize := 3
			fake := &fakeService{}
			addr := setupTestServer(t, fake)
			dialFunc := func() (*BigtableConn, error) { return dialBigtableserver(addr) }

			pool, err := NewBigtableChannelPool(ctx, poolSize, strategy, dialFunc, nil, nil)
			if err != nil {
				t.Fatalf("Failed to create pool: %v", err)
			}
			defer pool.Close()

			req := &testpb.SimpleRequest{Payload: &testpb.Payload{Body: []byte("hello")}}
			res := &testpb.SimpleResponse{}
			if err := pool.Invoke(context.Background(), "/grpc.testing.BenchmarkService/UnaryCall", req, res); err != nil {
				t.Errorf("Invoke failed: %v", err)
			}
			if string(res.GetPayload().GetBody()) != "hello" {
				t.Errorf("Invoke response got %q, want %q", string(res.GetPayload().GetBody()), "hello")
			}
			if fake.getCallCount() != 1 {
				t.Errorf("Server call count got %d, want 1", fake.getCallCount())
			}

			for _, entry := range pool.getConns() {
				if atomic.LoadInt32(&entry.unaryLoad) != 0 {
					t.Errorf("Unary load is non-zero after Invoke: %d", atomic.LoadInt32(&entry.unaryLoad))
				}
			}

			// Test invoke with server error
			fake.callCount = 0
			fake.serverErr = status.Error(codes.Internal, "simulated invoke error")
			err = pool.Invoke(context.Background(), "/grpc.testing.BenchmarkService/UnaryCall", req, res)
			if err == nil {
				t.Errorf("Invoke succeeded, want error")
			} else {
				st, ok := status.FromError(err)
				if !ok || st.Code() != codes.Internal || !strings.Contains(st.Message(), "simulated invoke error") {
					t.Errorf("Invoke got error %v, want Internal server error", err)
				}
			}
			fake.serverErr = nil
			if fake.getCallCount() != 1 {
				t.Errorf("Server call count got %d, want 1 after error", fake.getCallCount())
			}
			for _, entry := range pool.getConns() {
				if atomic.LoadInt32(&entry.unaryLoad) != 0 {
					t.Errorf("Unary load is non-zero after failed Invoke: %d", atomic.LoadInt32(&entry.unaryLoad))
				}
			}
		})
	}
	t.Run("EmptyPoolInvoke", func(t *testing.T) {
		pool := &BigtableChannelPool{} // No connections
		pool.selectFunc = pool.selectRoundRobin
		err := pool.Invoke(ctx, "/grpc.testing.BenchmarkService/UnaryCall", &testpb.SimpleRequest{}, &testpb.SimpleResponse{})
		if !errors.Is(err, errNoConnections) {
			t.Errorf("Invoke on empty pool got error %v, want %v", err, errNoConnections)
		}
	})
}

func TestPoolNewStream(t *testing.T) {
	ctx := context.Background()
	strategies := []btopt.LoadBalancingStrategy{
		btopt.LeastInFlight,
		btopt.RoundRobin,
		btopt.PowerOfTwoLeastInFlight,
	}

	for _, strategy := range strategies {
		t.Run(fmt.Sprintf("Strategy_%s", strategy), func(t *testing.T) {
			poolSize := 2
			fake := &fakeService{}
			addr := setupTestServer(t, fake)
			dialFunc := func() (*BigtableConn, error) { return dialBigtableserver(addr) }

			pool, err := NewBigtableChannelPool(ctx, poolSize, strategy, dialFunc, nil, nil)
			if err != nil {
				t.Fatalf("Failed to create pool: %v", err)
			}
			defer pool.Close()

			streamCtx := context.Background()
			stream, err := pool.NewStream(streamCtx, &grpc.StreamDesc{StreamName: "StreamingCall"}, "/grpc.testing.BenchmarkService/StreamingCall")
			if err != nil {
				t.Fatalf("NewStream failed: %v", err)
			}

			loadSum := int32(0)
			for _, entry := range pool.getConns() {
				loadSum += atomic.LoadInt32(&entry.streamingLoad)
			}
			if loadSum != 1 {
				t.Errorf("Total streaming load after NewStream got %d, want 1.", loadSum)
			}

			// ... stream interaction ...
			req := &testpb.SimpleRequest{Payload: &testpb.Payload{Body: []byte("msg1")}}
			if err := stream.SendMsg(req); err != nil {
				t.Fatalf("SendMsg failed: %v", err)
			}
			res := &testpb.SimpleResponse{}
			if err := stream.RecvMsg(res); err != nil {
				t.Fatalf("RecvMsg failed: %v", err)
			}
			stream.CloseSend()
			for {
				if err := stream.RecvMsg(res); err != nil {
					if err == io.EOF {
						break
					}
					t.Fatalf("RecvMsg after close failed: %v", err)
				}
			}

			time.Sleep(20 * time.Millisecond) // Allow decrements to complete
			loadSum = int32(0)
			for _, entry := range pool.getConns() {
				loadSum += atomic.LoadInt32(&entry.streamingLoad)
			}
			if loadSum != 0 {
				t.Errorf("Total streaming load after stream completion got %d, want 0.", loadSum)
			}
		})
	}

	t.Run("EmptyPoolNewStream", func(t *testing.T) {
		pool := &BigtableChannelPool{} // No connections
		pool.selectFunc = pool.selectRoundRobin
		_, err := pool.NewStream(ctx, &grpc.StreamDesc{StreamName: "StreamingCall"}, "/grpc.testing.BenchmarkService/StreamingCall")
		if !errors.Is(err, errNoConnections) {
			t.Errorf("NewStream on empty pool got error %v, want %v", err, errNoConnections)
		}
	})

	t.Run("NewStreamServerError", func(t *testing.T) {
		poolSize := 1
		fake := &fakeService{}
		addr := setupTestServer(t, fake)
		dialFunc := func() (*BigtableConn, error) { return dialBigtableserver(addr) }
		pool, err := NewBigtableChannelPool(ctx, poolSize, btopt.RoundRobin, dialFunc, nil, nil)
		if err != nil {
			t.Fatalf("Failed to create pool: %v", err)
		}
		defer pool.Close()

		wantErr := status.Error(codes.Unavailable, "simulated stream creation error")
		fake.serverErr = wantErr
		defer func() { fake.serverErr = nil }()

		stream, err := pool.NewStream(ctx, &grpc.StreamDesc{StreamName: "StreamingCall"}, "/grpc.testing.BenchmarkService/StreamingCall")
		if err == nil { // NewStream in grpc-go doesn't return an error if the connection is up, the error is on first Recv/Send
			// t.Fatalf("NewStream should have failed")
		} else {
			// This case should ideally not happen based on grpc behavior
			if atomic.LoadInt32(&pool.getConns()[0].streamingLoad) != 0 {
				t.Errorf("Load is non-zero after NewStream failed: %d", atomic.LoadInt32(&pool.getConns()[0].streamingLoad))
			}
			return
		}

		if atomic.LoadInt32(&pool.getConns()[0].streamingLoad) != 1 {
			t.Fatalf("Load is %d, want 1 after NewStream", atomic.LoadInt32(&pool.getConns()[0].streamingLoad))
		}

		err = stream.RecvMsg(&testpb.SimpleResponse{})
		if err == nil {
			t.Errorf("RecvMsg succeeded, want server error")
		} else {
			st, ok := status.FromError(err)
			if !ok || st.Code() != codes.Unavailable || !strings.Contains(st.Message(), "simulated stream creation error") {
				t.Errorf("RecvMsg got error %v, want %v", err, wantErr)
			}
		}

		time.Sleep(20 * time.Millisecond)

		if atomic.LoadInt32(&pool.getConns()[0].streamingLoad) != 0 {
			t.Errorf("Load is %d, want 0 after stream error", atomic.LoadInt32(&pool.getConns()[0].streamingLoad))
		}
	})
}

func TestSelectLeastLoaded(t *testing.T) {
	pool := &BigtableChannelPool{}

	entry, err := pool.selectLeastLoaded()
	if entry != nil || !errors.Is(err, errNoConnections) {
		t.Errorf("Empty pool: got %v, %v, want nil, %v", entry, err, errNoConnections)
	}

	conns := []*connEntry{{}}
	pool.conns.Store(conns)
	entry, err = pool.selectLeastLoaded()
	if entry != conns[0] || err != nil {
		t.Errorf("Single conn: got %v, %v, want %v, nil", entry, err, conns[0])
	}

	// streamingLoadFactor is 2, unaryLoadFactor is 1
	testLoads := []struct{ unary, stream int32 }{
		{3, 0}, // Load: 3
		{1, 1}, // Load: 1*1 + 2*1 = 3
		{0, 2}, // Load: 4
		{5, 0}, // Load: 5
		{1, 0}, // Load: 1 (Smallest)
	}
	conns = make([]*connEntry, len(testLoads))
	expectedMinIndex := 4
	for i, loads := range testLoads {
		conns[i] = &connEntry{}
		atomic.StoreInt32(&conns[i].unaryLoad, loads.unary)
		atomic.StoreInt32(&conns[i].streamingLoad, loads.stream)
	}
	pool.conns.Store(conns)

	entry, err = pool.selectLeastLoaded()
	if err != nil {
		t.Errorf("Multi conn: got error %v, want nil", err)
	}
	if entry != conns[expectedMinIndex] {
		t.Errorf("Multi conn: selected entry with load %d, want entry with load 1 (index %d)", entry.calculateWeightedLoad(), expectedMinIndex)
	}
}

func TestPoolClose(t *testing.T) {
	ctx := context.Background()
	poolSize := 2
	fake := &fakeService{}
	addr := setupTestServer(t, fake)
	dialFunc := func() (*BigtableConn, error) { return dialBigtableserver(addr) }

	pool, err := NewBigtableChannelPool(ctx, poolSize, btopt.LeastInFlight, dialFunc, nil, nil, WithHealthCheckConfig(DefaultHealthCheckConfig()))
	if err != nil {
		t.Fatalf("Failed to create pool: %v", err)
	}

	doneChan := pool.healthMonitor.done
	if err := pool.Close(); err != nil {
		t.Errorf("Close failed: %v", err)
	}

	if pool.getConns() != nil {
		t.Errorf("pool.getConns() got non-nil after Close, want nil")
	}

	select {
	case <-doneChan:
	case <-time.After(1 * time.Second):
		t.Errorf("Health checker did not stop after Close")
	}

	select {
	case <-pool.poolCtx.Done():
	case <-time.After(1 * time.Second):
		t.Errorf("Pool context not cancelled after Close")
	}
}

func TestMultipleStreamsSingleConn(t *testing.T) {
	ctx := context.Background()
	poolSize := 1
	fake := &fakeService{}
	addr := setupTestServer(t, fake)
	dialFunc := func() (*BigtableConn, error) { return dialBigtableserver(addr) }

	pool, err := NewBigtableChannelPool(ctx, poolSize, btopt.LeastInFlight, dialFunc, nil, nil)
	if err != nil {
		t.Fatalf("Failed to create pool: %v", err)
	}
	defer pool.Close()

	numStreams := 5
	streams := make([]grpc.ClientStream, numStreams)

	connEntry := pool.getConns()[0]

	for i := 0; i < numStreams; i++ {
		stream, err := pool.NewStream(ctx, &grpc.StreamDesc{StreamName: "StreamingCall"}, "/grpc.testing.BenchmarkService/StreamingCall")
		if err != nil {
			t.Fatalf("NewStream %d failed: %v", i, err)
		}
		streams[i] = stream
		expectedLoad := int32(i + 1)
		if atomic.LoadInt32(&connEntry.streamingLoad) != expectedLoad {
			t.Errorf("Load after opening stream %d is %d, want %d", i, atomic.LoadInt32(&connEntry.streamingLoad), expectedLoad)
		}
	}

	for i, stream := range streams {
		msg := fmt.Sprintf("stream%d", i)
		req := &testpb.SimpleRequest{Payload: &testpb.Payload{Body: []byte(msg)}}
		if err := stream.SendMsg(req); err != nil {
			t.Errorf("SendMsg on stream %d failed: %v", i, err)
		}
		res := &testpb.SimpleResponse{}
		if err := stream.RecvMsg(res); err != nil {
			t.Errorf("RecvMsg on stream %d failed: %v", i, err)
		}
		if string(res.GetPayload().GetBody()) != msg {
			t.Errorf("RecvMsg on stream %d got %q, want %q", i, string(res.GetPayload().GetBody()), msg)
		}
	}

	if fake.getCallCount() != numStreams {
		t.Errorf("Server call count got %d, want %d", fake.getCallCount(), numStreams)
	}

	for i, stream := range streams {
		if err := stream.CloseSend(); err != nil {
			t.Errorf("CloseSend on stream %d failed: %v", i, err)
		}
		for {
			if err := stream.RecvMsg(&testpb.SimpleResponse{}); err != nil {
				if err != io.EOF {
					t.Errorf("RecvMsg on stream %d after close failed unexpectedly: %v", i, err)
				}
				break
			}
		}
		time.Sleep(20 * time.Millisecond)

		expectedLoad := int32(numStreams - 1 - i)
		currentLoad := atomic.LoadInt32(&connEntry.streamingLoad)
		if currentLoad != expectedLoad {
			t.Errorf("Load after closing stream %d is %d, want %d", i, currentLoad, expectedLoad)
		}
	}

	finalLoad := atomic.LoadInt32(&connEntry.streamingLoad)
	if finalLoad != 0 {
		t.Errorf("Final load is %d, want 0", finalLoad)
	}
}

func TestCachingStreamDecrement(t *testing.T) {
	ctx := context.Background()
	poolSize := 1
	fake := &fakeService{}
	addr := setupTestServer(t, fake)
	dialFunc := func() (*BigtableConn, error) {
		conn, err := grpc.Dial(addr, grpc.WithTransportCredentials(insecure.NewCredentials()))
		if err != nil {
			return nil, err
		}
		return NewBigtableConn(conn, "test-instance", "test-profile"), nil
	}

	pool, err := NewBigtableChannelPool(ctx, poolSize, btopt.LeastInFlight, dialFunc, nil, nil)
	if err != nil {
		t.Fatalf("Failed to create pool: %v", err)
	}
	defer pool.Close()
	entry := pool.getConns()[0]

	t.Run("DecrementOnRecvError", func(t *testing.T) {
		fake.serverErr = errors.New("stream recv error")
		defer func() { fake.serverErr = nil }()
		atomic.StoreInt32(&entry.streamingLoad, 0)

		stream, err := pool.NewStream(context.Background(), &grpc.StreamDesc{StreamName: "StreamingCall"}, "/grpc.testing.BenchmarkService/StreamingCall")
		if err != nil {
			t.Fatalf("NewStream failed: %v", err)
		}
		if atomic.LoadInt32(&entry.streamingLoad) != 1 {
			t.Errorf("Load is %d, want 1 after NewStream", atomic.LoadInt32(&entry.streamingLoad))
		}

		stream.RecvMsg(&testpb.SimpleResponse{})
		time.Sleep(20 * time.Millisecond)

		if atomic.LoadInt32(&entry.streamingLoad) != 0 {
			t.Errorf("Load is %d, want 0 after RecvMsg error", atomic.LoadInt32(&entry.streamingLoad))
		}
	})

	t.Run("DecrementOnSendError", func(t *testing.T) {
		atomic.StoreInt32(&entry.streamingLoad, 0)
		fake.serverErr = nil

		stream, err := pool.NewStream(context.Background(), &grpc.StreamDesc{StreamName: "StreamingCall"}, "/grpc.testing.BenchmarkService/StreamingCall")
		if err != nil {
			t.Fatalf("NewStream failed: %v", err)
		}
		if atomic.LoadInt32(&entry.streamingLoad) != 1 {
			t.Errorf("Load is %d, want 1 after NewStream", atomic.LoadInt32(&entry.streamingLoad))
		}

		if err := stream.CloseSend(); err != nil {
			t.Fatalf("CloseSend failed: %v", err)
		}
		for {
			if err := stream.RecvMsg(&testpb.SimpleResponse{}); err != nil {
				if err == io.EOF {
					break
				}
				t.Fatalf("RecvMsg failed unexpectedly: %v", err)
			}
		}

		err = stream.SendMsg(&testpb.SimpleRequest{Payload: &testpb.Payload{Body: []byte("wont send")}})
		if err == nil {
			t.Errorf("SendMsg should have failed after stream is closed")
		}

		time.Sleep(20 * time.Millisecond)
		if atomic.LoadInt32(&entry.streamingLoad) != 0 {
			t.Errorf("Load is %d, want 0 after SendMsg error", atomic.LoadInt32(&entry.streamingLoad))
		}
	})

	t.Run("NoDecrementOnSuccessfulSend", func(t *testing.T) {
		atomic.StoreInt32(&entry.streamingLoad, 0)
		fake.serverErr = nil
		fake.streamSema = make(chan struct{})

		stream, err := pool.NewStream(context.Background(), &grpc.StreamDesc{StreamName: "StreamingCall"}, "/grpc.testing.BenchmarkService/StreamingCall")
		if err != nil {
			t.Fatalf("NewStream failed: %v", err)
		}
		if atomic.LoadInt32(&entry.streamingLoad) != 1 {
			t.Errorf("Load is %d, want 1", atomic.LoadInt32(&entry.streamingLoad))
		}

		if err := stream.SendMsg(&testpb.SimpleRequest{Payload: &testpb.Payload{Body: []byte("test")}}); err != nil {
			t.Fatalf("SendMsg failed: %v", err)
		}
		if atomic.LoadInt32(&entry.streamingLoad) != 1 {
			t.Errorf("Load is %d, want 1 after successful SendMsg", atomic.LoadInt32(&entry.streamingLoad))
		}

		close(fake.streamSema)
		stream.CloseSend()
		for {
			if err := stream.RecvMsg(&testpb.SimpleResponse{}); err != nil {
				break
			}
		}
		time.Sleep(20 * time.Millisecond)
		if atomic.LoadInt32(&entry.streamingLoad) != 0 {
			t.Errorf("Load is %d, want 0 after stream cleanup", atomic.LoadInt32(&entry.streamingLoad))
		}
	})
}

func TestConnHealthStateAddProbeResult(t *testing.T) {
	chs := &connHealthState{}
	config := DefaultHealthCheckConfig()
	chs.addProbeResult(true, config.WindowDuration)
	if len(chs.probeHistory) != 1 || !chs.probeHistory[0].successful || chs.successfulProbes != 1 || chs.failedProbes != 0 {
		t.Errorf("Add successful probe failed: %+v", chs)
	}
	chs.addProbeResult(false, config.WindowDuration)
	if len(chs.probeHistory) != 2 || chs.probeHistory[1].successful || chs.successfulProbes != 1 || chs.failedProbes != 1 {
		t.Errorf("Add failed probe failed: %+v", chs)
	}
}

func TestConnHealthStatePruneHistory(t *testing.T) {
	chs := &connHealthState{}
	config := DefaultHealthCheckConfig()
	now := time.Now()
	chs.mu.Lock()
	chs.probeHistory = []probeResult{
		{t: now.Add(-config.WindowDuration - time.Second), successful: true},
		{t: now.Add(-config.WindowDuration + time.Millisecond), successful: false},
	}
	chs.successfulProbes = 1
	chs.failedProbes = 1
	chs.mu.Unlock()

	chs.addProbeResult(true, config.WindowDuration) // This triggers prune

	chs.mu.Lock()
	defer chs.mu.Unlock()
	if len(chs.probeHistory) != 2 || chs.successfulProbes != 1 || chs.failedProbes != 1 {
		t.Errorf("Prune failed, history length %d, successful %d, failed %d", len(chs.probeHistory), chs.successfulProbes, chs.failedProbes)
	}
}

func TestChannelHealthMonitor_Stop(t *testing.T) {
	t.Run("Enabled", func(t *testing.T) {
		config := DefaultHealthCheckConfig()
		if !config.Enabled {
			t.Fatal("DefaultHealthCheckConfig.Enabled should be true for this test")
		}

		// The pool can be nil for this unit test since Stop() doesn't use it.
		chm := NewChannelHealthMonitor(config, nil)

		// Test double stop
		chm.Stop()
		chm.Stop() // The sync.Once should prevent a panic on double close

		// Check if channel is closed
		select {
		case <-chm.done:
			// Expected
		default:
			t.Errorf("chm.done not closed after Stop()")
		}
	})

	t.Run("Disabled", func(t *testing.T) {
		config := DefaultHealthCheckConfig()
		config.Enabled = false // Explicitly disable

		chm := NewChannelHealthMonitor(config, nil)

		chm.Stop()
		chm.Stop()

		// Check that the channel is NOT closed, since the monitor should
		// have returned immediately.
		select {
		case <-chm.done:
			t.Errorf("chm.done was closed, but monitor was disabled")
		default:
			// Expected
		}
	})
}

func TestRunProbesWhenContextDone(t *testing.T) {
	ctx := context.Background()
	fake := &fakeService{}
	addr := setupTestServer(t, fake)
	dialFunc := func() (*BigtableConn, error) { return dialBigtableserver(addr) }
	pool, err := NewBigtableChannelPool(ctx, 2, btopt.RoundRobin, dialFunc, nil, nil, WithHealthCheckConfig(DefaultHealthCheckConfig()))
	if err != nil {
		t.Fatalf("Failed to create pool: %v", err)
	}
	defer pool.Close()

	probeCtx, cancel := context.WithCancel(ctx)
	cancel() // Immediately cancel

	pool.runProbes(probeCtx, pool.hcConfig)

	conns := pool.getConns()
	for i, entry := range conns {
		entry.health.mu.Lock()
		if len(entry.health.probeHistory) != 1 || entry.health.probeHistory[0].successful {
			t.Errorf("Entry %d: Expected 1 failed probe due to context done, got %+v", i, entry.health.probeHistory)
		}
		entry.health.mu.Unlock()
	}
}

func TestConnHealthStateIsHealthy(t *testing.T) {
	config := HealthCheckConfig{
		MinProbesForEval:     3,
		FailurePercentThresh: 50,
		// Other fields don't matter for this test
	}

	tests := []struct {
		name       string
		results    []bool
		isHealthy  bool
		numSuccess int
		numFailed  int
	}{
		{"NotEnoughProbes", []bool{true, false}, true, 1, 1},
		{"Healthy", []bool{true, true, false}, true, 2, 1},                      // 33% failure
		{"Unhealthy", []bool{true, false, false, false}, false, 1, 3},           // 75% failure
		{"JustUnhealthy", []bool{true, true, false, false, false}, false, 2, 3}, // 60% failure
		{"AllSuccessful", []bool{true, true, true}, true, 3, 0},
		{"AllFailed", []bool{false, false, false}, false, 0, 3},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			chs := &connHealthState{}
			for _, r := range tc.results {
				chs.addProbeResult(r, time.Minute) // WindowDuration doesn't impact isHealthy logic itself
			}

			if got := chs.isHealthy(config.MinProbesForEval, config.FailurePercentThresh); got != tc.isHealthy {
				t.Errorf("isHealthy() got %v, want %v", got, tc.isHealthy)
			}
			if chs.successfulProbes != tc.numSuccess {
				t.Errorf("successfulProbes got %d, want %d", chs.successfulProbes, tc.numSuccess)
			}
			if chs.failedProbes != tc.numFailed {
				t.Errorf("failedProbes got %d, want %d", chs.failedProbes, tc.numFailed)
			}
			if chs.getFailedProbes() != tc.numFailed {
				t.Errorf("getFailedProbes() got %d, want %d", chs.getFailedProbes(), tc.numFailed)
			}
		})
	}
}

func TestDetectAndEvictUnhealthy(t *testing.T) {
	ctx := context.Background() // Use context.Background() for tests
	const poolSize = 10

	testConfig := HealthCheckConfig{
		Enabled:                  true,
		ProbeInterval:            30 * time.Second,
		ProbeTimeout:             1 * time.Second,
		WindowDuration:           5 * time.Minute,
		MinProbesForEval:         5,
		FailurePercentThresh:     20,
		PoolwideBadThreshPercent: 50,
		MinEvictionInterval:      0, // Allow immediate eviction for test
	}

	fake := &fakeService{}
	addr := setupTestServer(t, fake)
	dialFunc := func() (*BigtableConn, error) { return dialBigtableserver(addr) }

	setupHealth := func(entry *connEntry, successful, failed int) {
		entry.health.mu.Lock()
		defer entry.health.mu.Unlock()
		entry.health.successfulProbes, entry.health.failedProbes = successful, failed
		// Add enough history to be evaluated
		entry.health.probeHistory = make([]probeResult, successful+failed)
		for i := 0; i < successful+failed; i++ {
			entry.health.probeHistory[i] = probeResult{t: time.Now()}
		}
	}

	t.Run("EvictOneUnhealthy", func(t *testing.T) {
		pool, err := NewBigtableChannelPool(ctx, poolSize, btopt.RoundRobin, dialFunc, nil, nil, WithHealthCheckConfig(testConfig))
		if err != nil {
			t.Fatalf("Failed to create pool: %v", err)
		}
		defer pool.Close()
		unhealthyIdx := 3
		conns := pool.getConns()
		for i, entry := range conns {
			if i == unhealthyIdx {
				setupHealth(entry, 7, 3) // 30% failure
			} else {
				setupHealth(entry, 10, 0) // 0% failure
			}
		}
		pool.conns.Store(conns)

		oldConn := pool.getConns()[unhealthyIdx].conn
		if !pool.detectAndEvictUnhealthy(pool.hcConfig, pool.healthMonitor.AllowEviction, pool.healthMonitor.RecordEviction) {
			t.Errorf("Connection was not evicted")
		}
		if pool.getConns()[unhealthyIdx].conn == oldConn {
			t.Errorf("Connection at index %d was not evicted", unhealthyIdx)
		}
	})

	t.Run("CircuitBreakerTooManyUnhealthy", func(t *testing.T) {
		pool, err := NewBigtableChannelPool(ctx, poolSize, btopt.RoundRobin, dialFunc, nil, nil, WithHealthCheckConfig(testConfig))
		if err != nil {
			t.Fatalf("Failed to create pool: %v", err)
		}
		defer pool.Close()
		conns := pool.getConns()
		initialConns := make([]*BigtableConn, poolSize)
		for i := 0; i < poolSize; i++ {
			initialConns[i] = conns[i].conn
			if i < 6 { // 6 out of 10 unhealthy
				setupHealth(conns[i], 5, 5) // 50% failure
			} else {
				setupHealth(conns[i], 10, 0) // 0% failure
			}
		}
		pool.conns.Store(conns)
		if pool.detectAndEvictUnhealthy(pool.hcConfig, pool.healthMonitor.AllowEviction, pool.healthMonitor.RecordEviction) {
			t.Errorf("Connection was evicted")
		}
	})
}

func TestReplaceConnection(t *testing.T) {
	ctx := context.Background()
	fake := &fakeService{}
	addr := setupTestServer(t, fake)
	idxToReplace := 0

	var dialSucceed bool
	var dialCount int32
	var mu sync.Mutex // To protect dialSucceed

	dialFunc := func() (*BigtableConn, error) {
		atomic.AddInt32(&dialCount, 1)
		mu.Lock()
		ds := dialSucceed
		mu.Unlock()
		if !ds {
			return nil, errors.New("simulated redial failure")
		}
		return dialBigtableserver(addr)
	}

	t.Run("SuccessfulReplace", func(t *testing.T) {
		mu.Lock()
		dialSucceed = true
		mu.Unlock()
		atomic.StoreInt32(&dialCount, 0)

		pool, err := NewBigtableChannelPool(ctx, 2, btopt.RoundRobin, dialFunc, log.Default(), nil)
		if err != nil {
			t.Fatalf("Failed to create pool: %v", err)
		}
		defer pool.Close()

		atomic.StoreInt32(&dialCount, 0) // Reset count for replaceConnection call

		oldEntry := pool.getConns()[idxToReplace]
		pool.replaceConnection(oldEntry)

		if atomic.LoadInt32(&dialCount) != 1 {
			t.Errorf("Dial function called %d times by replaceConnection, want 1", atomic.LoadInt32(&dialCount))
		}
		newEntry := pool.getConns()[idxToReplace]
		if newEntry == oldEntry || newEntry.conn == oldEntry.conn {
			t.Errorf("Connection not replaced")
		}
		if atomic.LoadInt32(&newEntry.unaryLoad) != 0 || atomic.LoadInt32(&newEntry.streamingLoad) != 0 {
			t.Errorf("New entry load not zero")
		}
		time.Sleep(50 * time.Millisecond) // Wait for prime to finish
		if newEntry.isALTSUsed() {
			t.Errorf("New entry isALTSUsed() got true, want false")
		}
	})

	t.Run("FailedRedial", func(t *testing.T) {
		// Pool creation should succeed
		mu.Lock()
		dialSucceed = true
		mu.Unlock()
		atomic.StoreInt32(&dialCount, 0)

		pool, err := NewBigtableChannelPool(ctx, 2, btopt.RoundRobin, dialFunc, log.Default(), nil)
		if err != nil {
			t.Fatalf("Failed to create pool: %v", err)
		}
		defer pool.Close()

		// Make the *next* dial fail (the one in replaceConnection)
		mu.Lock()
		dialSucceed = false
		mu.Unlock()
		atomic.StoreInt32(&dialCount, 0) // Reset count for replaceConnection call

		currentEntry := pool.getConns()[idxToReplace]
		pool.replaceConnection(currentEntry)

		if atomic.LoadInt32(&dialCount) != 1 {
			t.Errorf("Dial function called %d times by replaceConnection, want 1", atomic.LoadInt32(&dialCount))
		}
		if pool.getConns()[idxToReplace] != currentEntry {
			t.Errorf("Connection entry changed despite redial failure")
		}
	})

	t.Run("PoolContextDone", func(t *testing.T) {
		mu.Lock()
		dialSucceed = true
		mu.Unlock()
		atomic.StoreInt32(&dialCount, 0)

		poolCancelled, err := NewBigtableChannelPool(ctx, 2, btopt.RoundRobin, dialFunc, log.Default(), nil)
		if err != nil {
			t.Fatalf("Failed to create poolCancelled: %v", err)
		}
		// Intentionally not closing poolCancelled normally, just cancelling context

		poolCancelled.poolCancel()       // Cancel the context
		atomic.StoreInt32(&dialCount, 0) // Reset count for replaceConnection call

		currentEntry := poolCancelled.getConns()[idxToReplace]
		poolCancelled.replaceConnection(currentEntry)

		if atomic.LoadInt32(&dialCount) != 0 {
			t.Errorf("Dial function called %d times by replaceConnection, want 0 because context is done", atomic.LoadInt32(&dialCount))
		}
		if poolCancelled.getConns()[idxToReplace] != currentEntry {
			t.Errorf("Connection entry changed despite context done")
		}
		poolCancelled.Close() // Still close to free resources
	})
}

func TestHealthCheckerIntegration(t *testing.T) {
	ctx := context.Background()
	// Shorten times for testing
	testHCConfig := HealthCheckConfig{
		Enabled:                  true,
		ProbeInterval:            50 * time.Millisecond,
		ProbeTimeout:             1 * time.Second, // Keep timeout reasonable
		WindowDuration:           500 * time.Millisecond,
		MinProbesForEval:         2,
		FailurePercentThresh:     40,
		PoolwideBadThreshPercent: 70, // Or as needed
		MinEvictionInterval:      100 * time.Millisecond,
	}
	fake1, fake2 := &fakeService{}, &fakeService{}
	addr1, addr2 := setupTestServer(t, fake1), setupTestServer(t, fake2)
	dialOpts := []string{addr1, addr2}
	var dialIdx int32

	dialFunc := func() (*BigtableConn, error) {
		idx := atomic.AddInt32(&dialIdx, 1) - 1
		addr := dialOpts[idx%2]
		if idx >= 2 { // Replacements always go to addr2
			addr = addr2
		}
		return dialBigtableserver(addr)
	}

	pool, err := NewBigtableChannelPool(ctx, 2, btopt.RoundRobin, dialFunc, nil, nil, WithHealthCheckConfig(testHCConfig))
	if err != nil {
		t.Fatalf("Failed to create pool: %v", err)
	}
	defer pool.Close()

	time.Sleep(2 * testHCConfig.WindowDuration) // Let initial probes run

	fake1.setPingErr(errors.New("server1 unhealthy")) // Make conn 0 fail;

	evicted := false
	// Check frequently for a limited time
	maxWait := 5 * time.Second
	checkInterval := testHCConfig.ProbeInterval * 2
	numChecks := int(maxWait / checkInterval)

	for i := 0; i < numChecks; i++ {
		time.Sleep(checkInterval)
		conns := pool.getConns()
		if len(conns) > 0 && conns[0].conn.ClientConn.Target() == addr2 {
			evicted = true
			break
		}
	}
	if !evicted {
		t.Errorf("Connection 0 not evicted to addr2 within %s", maxWait)
	}
	if len(pool.getConns()) > 1 && pool.getConns()[1].conn.ClientConn.Target() != addr2 {
		t.Errorf("Connection 1 target changed unexpectedly")
	}
}

func findMetric(rm metricdata.ResourceMetrics, name string) (metricdata.Metrics, bool) {
	for _, sm := range rm.ScopeMetrics {
		for _, m := range sm.Metrics {
			if m.Name == name {
				return m, true
			}
		}
	}
	return metricdata.Metrics{}, false
}

func TestMetricsExporting(t *testing.T) {
	ctx := context.Background()
	fake := &fakeService{}
	addr := setupTestServer(t, fake)
	dialFunc := func() (*BigtableConn, error) { return dialBigtableserver(addr) }

	reader := sdkmetric.NewManualReader()
	provider := sdkmetric.NewMeterProvider(sdkmetric.WithReader(reader))

	poolSize := 2
	strategy := btopt.RoundRobin
	pool, err := NewBigtableChannelPool(ctx, poolSize, strategy, dialFunc, nil, provider)
	if err != nil {
		t.Fatalf("Failed to create pool: %v", err)
	}
	defer pool.Close()

	// Wait for initial priming to settle
	time.Sleep(100 * time.Millisecond)

	// Action 1: Successful Invoke on conn 0
	req := &testpb.SimpleRequest{Payload: &testpb.Payload{Body: []byte("hello")}}
	res := &testpb.SimpleResponse{}
	if err := pool.Invoke(ctx, "/grpc.testing.BenchmarkService/UnaryCall", req, res); err != nil {
		t.Errorf("Invoke failed: %v", err)
	}

	// Action 2: Failed Invoke on conn 1
	fake.serverErr = errors.New("simulated error")
	pool.Invoke(ctx, "/grpc.testing.BenchmarkService/UnaryCall", req, res) // Error expected
	fake.serverErr = nil

	// Action 3: Successful Stream on conn 0
	stream, err := pool.NewStream(ctx, &grpc.StreamDesc{StreamName: "StreamingCall"}, "/grpc.testing.BenchmarkService/StreamingCall")
	if err != nil {
		t.Fatalf("NewStream failed: %v", err)
	}
	stream.SendMsg(req)
	stream.RecvMsg(res)
	stream.CloseSend()
	for {
		if err := stream.RecvMsg(res); err != nil {
			break
		}
	}

	// Action 4: Stream with SendMsg error on conn 1
	fake.streamSendErr = errors.New("simulated send error")
	stream2, err := pool.NewStream(ctx, &grpc.StreamDesc{StreamName: "StreamingCall"}, "/grpc.testing.BenchmarkService/StreamingCall")
	if err != nil {
		t.Fatalf("NewStream 2 failed: %v", err)
	}
	stream2.SendMsg(req) // Expect error, triggers load decrement and error count
	stream2.CloseSend()
	for {
		if err := stream2.RecvMsg(res); err != nil {
			break
		}
	}
	fake.streamSendErr = nil

	time.Sleep(20 * time.Millisecond) // Allow stream error handling to complete

	// Force metrics collection
	pool.snapshotAndRecordMetrics(ctx)

	rm := metricdata.ResourceMetrics{}
	if err := reader.Collect(ctx, &rm); err != nil {
		t.Fatalf("Failed to collect metrics: %v", err)
	}

	// --- Check outstanding_rpcs ---
	outstandingRPCs, ok := findMetric(rm, "connection_pool/outstanding_rpcs")
	if !ok {
		t.Fatalf("Metric connection_pool/outstanding_rpcs not found")
	}

	hist, ok := outstandingRPCs.Data.(metricdata.Histogram[float64])
	if !ok {
		t.Fatalf("Metric connection_pool/outstanding_rpcs is not a Histogram[float64]: %T", outstandingRPCs.Data)
	}

	// Expected: poolSize * 2 data points (unary and streaming for each connection)
	if len(hist.DataPoints) != 2 {
		t.Errorf("Outstanding RPCs histogram has %d data points, want %d", len(hist.DataPoints), poolSize*2)
	}

	var totalUnary, totalStreaming float64
	for _, dp := range hist.DataPoints {
		attrMap := make(map[string]attribute.Value)
		for _, kv := range dp.Attributes.ToSlice() {
			attrMap[string(kv.Key)] = kv.Value
		}

		if val, ok := attrMap["lb_policy"]; !ok || val.AsString() != strategy.String() {
			t.Errorf("Missing or incorrect lb_policy attribute: want %v, got %v", strategy.String(), val)
		}
		if val, ok := attrMap["transport_type"]; !ok || val.AsString() != "CLOUDPATH" {
			t.Errorf("Missing or incorrect transport_type attribute: want CLOUDPATH, got %v", val)
		}

		streamingVal, ok := attrMap["streaming"]
		if !ok {
			t.Errorf("Missing streaming attribute")
			continue
		}
		if streamingVal.Type() != attribute.BOOL {
			t.Errorf("streaming attribute is not a BOOL: got %v", streamingVal.Type())
			continue
		}
		isStreaming := streamingVal.AsBool()

		if isStreaming {
			totalStreaming += dp.Sum
		} else {
			totalUnary += dp.Sum
		}
	}
	if totalUnary != 0 {
		t.Errorf("Total Unary load sum is %f, want 0", totalUnary)
	}
	if totalStreaming != 0 {
		t.Errorf("Total Streaming load sum is %f, want 0", totalStreaming)
	}

	// --- Check per_connection_error_count ---
	errorCount, ok := findMetric(rm, "per_connection_error_count")
	if !ok {
		t.Fatalf("Metric per_connection_error_count not found")
	}
	errorHist, ok := errorCount.Data.(metricdata.Histogram[float64])
	if !ok {
		t.Fatalf("Metric per_connection_error_count is not a Histogram[float64]: %T", errorCount.Data)
	}

	if len(errorHist.DataPoints) != 1 {
		t.Errorf("Error Count histogram has %d data points, want %d", len(errorHist.DataPoints), poolSize)
	}

	var totalErrorSum float64
	for _, dp := range errorHist.DataPoints {
		totalErrorSum += dp.Sum
	}
	// Expected errors: 1 from Invoke, 1 from Stream SendMsg
	if totalErrorSum != 2 {
		t.Errorf("Total Error Count sum is %f, want 2", totalErrorSum)
	}

	// Check if error counts on entries are reset
	for _, entry := range pool.getConns() {
		if atomic.LoadInt64(&entry.errorCount) != 0 {
			t.Errorf("entry.errorCount is %d after metric collection, want 0", atomic.LoadInt64(&entry.errorCount))
		}
	}
}

func setConnLoads(conns []*connEntry, unary, stream int32) {
	for _, entry := range conns {
		atomic.StoreInt32(&entry.unaryLoad, unary)
		atomic.StoreInt32(&entry.streamingLoad, stream)
	}
}

func TestDynamicChannelScaling(t *testing.T) {
	ctx := context.Background()
	fake := &fakeService{}
	addr := setupTestServer(t, fake)
	dialFunc := func() (*BigtableConn, error) { return dialBigtableserver(addr) }

	baseConfig := DynamicChannelPoolConfig{
		Enabled:              true,
		MinConns:             2,
		MaxConns:             10,
		AvgLoadHighThreshold: 10,               // Scale up if avg load >= 10
		AvgLoadLowThreshold:  3,                // Scale down if avg load <= 3
		MinScalingInterval:   0,                // Disable time throttling for most tests
		CheckInterval:        10 * time.Second, // Not directly used by calling evaluateAndScale
		MaxRemoveConns:       3,
	}
	targetLoadFactor := float64(baseConfig.AvgLoadLowThreshold+baseConfig.AvgLoadHighThreshold) / 2.0

	tests := []struct {
		name        string
		initialSize int
		configOpt   func(*DynamicChannelPoolConfig)
		setLoad     func(conns []*connEntry)
		wantSize    int
	}{
		{
			name:        "ScaleUp",
			initialSize: 3,
			setLoad: func(conns []*connEntry) {
				setConnLoads(conns, 12, 0) // Avg load 12 > 10
			},
			// Total load = 3 * 12 = 36. Desired = ceil(36 / 6.5) = 6
			wantSize: 6,
		},
		{
			name:        "ScaleUpCappedAtMax",
			initialSize: 8,
			setLoad: func(conns []*connEntry) {
				setConnLoads(conns, 20, 0) // Avg load 20 > 10
			},
			// Total load = 8 * 20 = 160. Desired = ceil(160 / 6.5) = 25. Capped at MaxConns = 10
			wantSize: 10,
		},
		{
			name:        "ScaleDown",
			initialSize: 9,
			setLoad: func(conns []*connEntry) {
				setConnLoads(conns, 1, 0) // Avg load 1 < 3
			},
			// Total load = 9 * 1 = 9. Desired = ceil(9 / 6.5) = 2.
			wantSize: 6,
		},
		{
			name:        "ScaleDownCappedAtMin",
			initialSize: 3,
			setLoad: func(conns []*connEntry) {
				setConnLoads(conns, 1, 0) // Avg load 1 < 3
			},
			// Total load = 3 * 1 = 3. Desired = ceil(3 / 6.5) = 1. Capped at MinConns = 2
			wantSize: 2,
		},
		{
			name:        "ScaleDownLimitedByMaxRemove",
			initialSize: 10,
			configOpt: func(cfg *DynamicChannelPoolConfig) {
				cfg.MaxRemoveConns = 2
			},
			setLoad: func(conns []*connEntry) {
				setConnLoads(conns, 0, 0) // Avg load 0 < 3
			},
			// Total load = 0. Desired = 2 (MinConns). removeCount = 10 - 2 = 8. Limited by MaxRemoveConns = 2.
			wantSize: 10 - 2,
		},
		{
			name:        "NoScaleUp",
			initialSize: 5,
			setLoad: func(conns []*connEntry) {
				setConnLoads(conns, 7, 0) // 3 < Avg load 7 < 10
			},
			wantSize: 5,
		},
		{
			name:        "NoScaleDown",
			initialSize: 5,
			setLoad: func(conns []*connEntry) {
				setConnLoads(conns, 5, 1) // Weighted load 5*1 + 1*2 = 7.  3 < 7 < 10
			},
			wantSize: 5,
		},
		{
			name:        "ScaleUpAddAtLeastOne",
			initialSize: 2,
			setLoad: func(conns []*connEntry) {
				setConnLoads(conns, 10, 0) // Avg load 10, right at threshold.
			},
			// Total load = 20. Desired = ceil(20 / 6.5) = 4. Add 2.
			wantSize: 4,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			config := baseConfig
			if tc.configOpt != nil {
				tc.configOpt(&config)
			}

			pool, err := NewBigtableChannelPool(ctx, tc.initialSize, btopt.RoundRobin, dialFunc, nil, nil, WithDynamicChannelPool(config))
			if err != nil {
				t.Fatalf("Failed to create pool: %v", err)
			}
			defer pool.Close()

			if tc.setLoad != nil {
				tc.setLoad(pool.getConns())
			}

			// Capture the load for debugging
			var totalLoad int32
			conns := pool.getConns()
			for _, entry := range conns {
				totalLoad += entry.calculateWeightedLoad()
			}
			avgLoad := float64(totalLoad) / float64(len(conns))
			desiredConns := int(math.Ceil(float64(totalLoad) / targetLoadFactor))
			t.Logf("Initial size: %d, Avg load: %.2f, Total load: %d, Target desired conns: %d", tc.initialSize, avgLoad, totalLoad, desiredConns)

			pool.dynamicMonitor.evaluateAndScale()

			if gotSize := pool.Num(); gotSize != tc.wantSize {
				t.Errorf("evaluateAndScale() resulted in pool size %d, want %d", gotSize, tc.wantSize)
			}
		})
	}

	t.Run("MinScalingInterval", func(t *testing.T) {
		config := baseConfig
		config.MinScalingInterval = 5 * time.Minute
		initialSize := 3

		pool, err := NewBigtableChannelPool(ctx, initialSize, btopt.RoundRobin, dialFunc, nil, nil, WithDynamicChannelPool(config))
		if err != nil {
			t.Fatalf("Failed to create pool: %v", err)
		}
		defer pool.Close()

		// Set load to trigger scale up
		setConnLoads(pool.getConns(), 15, 0)

		// 1. Simulate recent scaling
		pool.dynamicMonitor.mu.Lock()
		pool.dynamicMonitor.lastScalingTime = time.Now()
		pool.dynamicMonitor.mu.Unlock()

		pool.dynamicMonitor.evaluateAndScale()
		if gotSize := pool.Num(); gotSize != initialSize {
			t.Errorf("Pool size changed to %d, want %d (should be throttled)", gotSize, initialSize)
		}

		// 2. Allow scaling again by moving lastScalingTime to the past
		pool.dynamicMonitor.mu.Lock()
		pool.dynamicMonitor.lastScalingTime = time.Now().Add(-10 * time.Minute)
		pool.dynamicMonitor.mu.Unlock()

		pool.dynamicMonitor.evaluateAndScale()
		if gotSize := pool.Num(); gotSize == initialSize {
			t.Errorf("Pool size %d, want > %d (should have scaled up)", gotSize, initialSize)
		} else {
			t.Logf("Scaled up to %d connections", gotSize)
		}
	})

	t.Run("EmptyPoolScaleUp", func(t *testing.T) {
		config := baseConfig
		// Pool creation requires size > 0. So, create and then manually empty it.
		pool, err := NewBigtableChannelPool(ctx, config.MinConns, btopt.RoundRobin, dialFunc, nil, nil, WithDynamicChannelPool(config))
		if err != nil {
			t.Fatalf("Failed to create pool: %v", err)
		}
		defer pool.Close()

		// Manually empty the pool to test the zero-connection code path
		pool.conns.Store(([]*connEntry)(nil))

		pool.dynamicMonitor.evaluateAndScale()
		if gotSize := pool.Num(); gotSize != config.MinConns {
			t.Errorf("Pool size after empty scale-up is %d, want %d", gotSize, config.MinConns)
		}
	})
}

func TestDynamicScalingAndHealthCheckingInteraction(t *testing.T) {
	ctx := context.Background()

	healthyFake := &fakeService{}
	unhealthyFake := &fakeService{}
	healthyAddr := setupTestServer(t, healthyFake)
	unhealthyAddr := setupTestServer(t, unhealthyFake)

	var dialCount int32
	dialFunc := func() (*BigtableConn, error) {
		count := atomic.AddInt32(&dialCount, 1)
		// The first connection goes to unhealthyFake, the rest and replacements go to healthyFake
		addr := healthyAddr
		if count == 1 {
			addr = unhealthyAddr
		}
		return dialBigtableserver(addr)
	}

	dynConfig := DynamicChannelPoolConfig{
		Enabled:              true,
		MinConns:             2,
		MaxConns:             5,
		AvgLoadHighThreshold: 10,
		AvgLoadLowThreshold:  3,
		MinScalingInterval:   0,
		CheckInterval:        20 * time.Millisecond,
		MaxRemoveConns:       2,
	}

	hcConfig := HealthCheckConfig{
		Enabled:                  true,
		ProbeInterval:            15 * time.Millisecond,
		ProbeTimeout:             1 * time.Second,
		WindowDuration:           100 * time.Millisecond,
		MinProbesForEval:         2,
		FailurePercentThresh:     40,
		PoolwideBadThreshPercent: 70,
		MinEvictionInterval:      0,
	}

	initialSize := 2
	pool, err := NewBigtableChannelPool(ctx, initialSize, btopt.RoundRobin, dialFunc, log.Default(), nil,
		WithDynamicChannelPool(dynConfig),
		WithHealthCheckConfig(hcConfig),
	)
	if err != nil {
		t.Fatalf("Failed to create pool: %v", err)
	}
	defer pool.Close()

	// Allow initial health checks to run
	time.Sleep(2 * hcConfig.WindowDuration)

	// --- Phase 1: Scale Up ---
	t.Log("Phase 1: Triggering Scale Up")
	setConnLoads(pool.getConns(), 15, 0) // High load

	time.Sleep(3 * dynConfig.CheckInterval) // Wait for scaling to occur

	if pool.Num() <= initialSize {
		t.Errorf("Pool size should have increased from %d, got %d", initialSize, pool.Num())
	}
	if pool.Num() > dynConfig.MaxConns {
		t.Errorf("Pool size %d exceeded MaxConns %d", pool.Num(), dynConfig.MaxConns)
	}
	t.Logf("Pool scaled up to %d connections", pool.Num())

	// --- Phase 2: Inject Unhealthiness ---
	t.Log("Phase 2: Triggering Unhealthiness")
	unhealthyFake.setPingErr(errors.New("simulated ping failure"))

	// Wait for health checker to detect and evict
	evicted := false
	for i := 0; i < 40; i++ { // Wait up to 600ms
		time.Sleep(hcConfig.ProbeInterval)
		conns := pool.getConns()
		foundUnhealthyTarget := false
		for _, entry := range conns {
			if entry.conn.ClientConn.Target() == unhealthyAddr {
				foundUnhealthyTarget = true
				break
			}
		}
		if !foundUnhealthyTarget {
			evicted = true
			break
		}
	}

	if !evicted {
		t.Errorf("Connection to %s was not evicted", unhealthyAddr)
	} else {
		t.Logf("Connection to %s was evicted", unhealthyAddr)
	}
	unhealthyFake.setPingErr(nil) // Clear error

	// Check all current connections point to healthyAddr
	for i, entry := range pool.getConns() {
		if entry.conn.ClientConn.Target() != healthyAddr {
			t.Errorf("Connection at index %d points to %s, want %s", i, entry.conn.ClientConn.Target(), healthyAddr)
		}
	}

	// --- Phase 3: Scale Down ---
	t.Log("Phase 3: Triggering Scale Down")
	setConnLoads(pool.getConns(), 1, 0) // Low load

	time.Sleep(4 * dynConfig.CheckInterval) // Wait for scaling

	currentSize := pool.Num()
	if currentSize >= dynConfig.MaxConns && currentSize > dynConfig.MinConns {
		t.Errorf("Pool size should have decreased, got %d", currentSize)
	}
	if currentSize < dynConfig.MinConns {
		t.Errorf("Pool size %d went below MinConns %d", currentSize, dynConfig.MinConns)
	}
	t.Logf("Pool scaled down to %d connections", currentSize)

	// Final check: ensure all connections are healthy
	time.Sleep(2 * hcConfig.WindowDuration) // Let probes run on new/remaining conns
	for i, entry := range pool.getConns() {
		if !entry.health.isHealthy(hcConfig.MinProbesForEval, hcConfig.FailurePercentThresh) {
			t.Errorf("Connection at index %d is not healthy after test cycles", i)
		}
	}
}

// isConnClosed checks if a grpc.ClientConn has been closed.
func isConnClosed(conn *grpc.ClientConn) bool {
	// The state will be Shutdown if the connection has been terminated.
	return conn.GetState() == connectivity.Shutdown
}

func TestGracefulDraining(t *testing.T) {
	ctx := context.Background()
	fake := &fakeService{}
	addr := setupTestServer(t, fake)
	dialFunc := func() (*BigtableConn, error) { return dialBigtableserver(addr) }

	t.Run("DrainingOnReplaceConnection", func(t *testing.T) {
		pool, err := NewBigtableChannelPool(ctx, 1, btopt.RoundRobin, dialFunc, nil, nil)
		if err != nil {
			t.Fatalf("Failed to create pool: %v", err)
		}
		defer pool.Close()

		oldEntry := pool.getConns()[0]

		// Create a long-lived stream to simulate in-flight traffic
		fake.streamSema = make(chan struct{})
		stream, err := pool.NewStream(ctx, &grpc.StreamDesc{}, "/grpc.testing.BenchmarkService/StreamingCall")
		if err != nil {
			t.Fatalf("NewStream failed: %v", err)
		}

		if atomic.LoadInt32(&oldEntry.streamingLoad) != 1 {
			t.Fatalf("Streaming load should be 1, got %d", atomic.LoadInt32(&oldEntry.streamingLoad))
		}

		// Trigger the replacement, which should start draining the old connection
		pool.replaceConnection(oldEntry)

		if !oldEntry.isDraining() {
			t.Fatal("Old connection was not marked as draining")
		}
		if isConnClosed(oldEntry.conn.ClientConn) {
			t.Fatal("Old connection was closed immediately instead of draining")
		}

		// Verify the new connection is in the pool and is not draining
		newEntry := pool.getConns()[0]
		if newEntry == oldEntry {
			t.Fatal("Connection was not replaced in the pool")
		}
		if newEntry.isDraining() {
			t.Fatal("New connection is incorrectly marked as draining")
		}

		// Verify new requests go to the new connection
		selectedEntry, err := pool.selectFunc()
		if err != nil {
			t.Fatalf("Failed to select a connection: %v", err)
		}
		if selectedEntry != newEntry {
			t.Fatalf("A new request was routed to the old draining connection")
		}

		// Finish the stream on the old connection
		close(fake.streamSema) // Unblock server
		stream.CloseSend()
		for {
			if err := stream.RecvMsg(&testpb.SimpleResponse{}); err == io.EOF {
				break
			}
		}

		// Wait for the waitForDrainAndClose goroutine to finish
		time.Sleep(500 * time.Millisecond)

		if atomic.LoadInt32(&oldEntry.streamingLoad) != 0 {
			t.Errorf("Old connection load is still %d after stream completion", atomic.LoadInt32(&oldEntry.streamingLoad))
		}
		if !isConnClosed(oldEntry.conn.ClientConn) {
			t.Error("Old connection was not closed after its load dropped to zero")
		}
	})

	t.Run("SelectionSkipsDrainingConns", func(t *testing.T) {
		pool, err := NewBigtableChannelPool(ctx, 3, btopt.RoundRobin, dialFunc, nil, nil)
		if err != nil {
			t.Fatalf("Failed to create pool: %v", err)
		}
		defer pool.Close()

		conns := pool.getConns()
		drainingEntry := conns[1]
		drainingEntry.drainingState.Store(true) // Manually mark as draining

		// Run selection many times and ensure the draining one is never picked
		for i := 0; i < 20; i++ {
			entry, err := pool.selectRoundRobin()
			if err != nil {
				t.Fatalf("Selection failed: %v", err)
			}
			if entry == drainingEntry {
				t.Fatal("Selection logic picked a connection that is draining")
			}
		}

		// Mark all as draining and expect an error
		for _, entry := range conns {
			entry.drainingState.Store(true)
		}
		_, err = pool.selectRoundRobin()
		if !errors.Is(err, errNoConnections) {
			t.Errorf("Expected errNoConnections when all connections are draining, got %v", err)
		}
	})

	t.Run("DrainingTimeout", func(t *testing.T) {
		// Temporarily shorten the timeout for this specific test
		originalTimeout := maxDrainingTimeout
		maxDrainingTimeout = 100 * time.Millisecond
		defer func() { maxDrainingTimeout = originalTimeout }()

		pool, err := NewBigtableChannelPool(ctx, 1, btopt.RoundRobin, dialFunc, nil, nil)
		if err != nil {
			t.Fatalf("Failed to create pool: %v", err)
		}
		defer pool.Close()

		oldEntry := pool.getConns()[0]

		// Create a stream that will never finish
		fake.streamSema = make(chan struct{})
		pool.NewStream(ctx, &grpc.StreamDesc{}, "/grpc.testing.BenchmarkService/StreamingCall")

		// Trigger replacement
		pool.replaceConnection(oldEntry)

		if isConnClosed(oldEntry.conn.ClientConn) {
			t.Fatal("Connection was closed immediately")
		}

		// Wait for the drain timeout to fire
		time.Sleep(maxDrainingTimeout + 50*time.Millisecond)

		if !isConnClosed(oldEntry.conn.ClientConn) {
			t.Error("Connection was not force-closed after the draining timeout")
		}
		// In a real scenario, we'd log that the load was still > 0, e.g.,
		if atomic.LoadInt32(&oldEntry.streamingLoad) == 0 {
			t.Error("Load was unexpectedly 0, timeout should not have been the reason for closing")
		}
	})
}

// --- Benchmarks ---

func createBenchmarkFake() *fakeService {
	return &fakeService{delay: 1 * time.Microsecond} // Simulate a tiny bit of work
}

func setupBenchmarkPool(b *testing.B, strategy btopt.LoadBalancingStrategy, poolSize int, serverAddr string) *BigtableChannelPool {
	b.Helper()

	dialFunc := func() (*BigtableConn, error) {
		return dialBigtableserver(serverAddr)
	}

	ctx := context.Background()
	pool, err := NewBigtableChannelPool(ctx, poolSize, strategy, dialFunc, nil, nil)
	if err != nil {
		b.Fatalf("Failed to create pool: %v", err)
	}
	pool.healthMonitor.Stop()

	b.Cleanup(func() {
		pool.Close()
	})
	return pool
}

func BenchmarkPoolInvoke(b *testing.B) {
	fake := createBenchmarkFake()
	serverAddr := setupTestServer(b, fake) // Server lives for all sub-benchmarks of BenchmarkPoolInvoke

	strategies := []btopt.LoadBalancingStrategy{
		btopt.RoundRobin,
		btopt.LeastInFlight,
		btopt.PowerOfTwoLeastInFlight,
	}
	poolSizes := []int{1, 8, 64}

	req := &testpb.SimpleRequest{Payload: &testpb.Payload{Body: []byte("benchmark")}}
	ctx := context.Background()

	for _, size := range poolSizes {
		for _, strategy := range strategies {
			b.Run(fmt.Sprintf("%s_PoolSize%d", strategy, size), func(b *testing.B) {
				pool := setupBenchmarkPool(b, strategy, size, serverAddr)

				b.ResetTimer()
				b.RunParallel(func(pb *testing.PB) {
					for pb.Next() {
						res := &testpb.SimpleResponse{}

						err := pool.Invoke(ctx, "/grpc.testing.BenchmarkService/UnaryCall", req, res)
						if err != nil {
							b.Fatalf("Invoke failed: %v", err) // Fail fast
						}
					}
				})
			})
		}
	}
	// --- Standard gtransport.DialPool ---
	for _, size := range poolSizes {
		b.Run(fmt.Sprintf("StandardPool_Size%d", size), func(b *testing.B) {
			stdPool, err := gtransport.DialPool(ctx,
				option.WithEndpoint(serverAddr),
				option.WithGRPCConnectionPool(size),
				option.WithoutAuthentication(),
				option.WithGRPCDialOption(grpc.WithTransportCredentials(insecure.NewCredentials())),
			)
			if err != nil {
				b.Fatalf("gtransport.DialPool failed: %v", err)
			}
			b.Cleanup(func() { stdPool.Close() })

			client := testpb.NewBenchmarkServiceClient(stdPool)
			b.ResetTimer()
			b.RunParallel(func(pb *testing.PB) {
				for pb.Next() {
					if _, err := client.UnaryCall(ctx, req); err != nil {
						b.Fatalf("UnaryCall failed: %v", err)
					}
				}
			})
		})
	}
}

func BenchmarkPoolNewStream(b *testing.B) {
	fake := createBenchmarkFake()
	serverAddr := setupTestServer(b, fake)

	strategies := []btopt.LoadBalancingStrategy{
		btopt.RoundRobin,
		btopt.LeastInFlight,
		btopt.PowerOfTwoLeastInFlight,
	}
	poolSizes := []int{1, 8, 64}
	ctx := context.Background()

	for _, size := range poolSizes {
		for _, strategy := range strategies {
			b.Run(fmt.Sprintf("%s_PoolSize%d", strategy, size), func(b *testing.B) {
				pool := setupBenchmarkPool(b, strategy, size, serverAddr)

				b.ResetTimer()
				b.RunParallel(func(pb *testing.PB) {
					for pb.Next() {
						stream, err := pool.NewStream(ctx, &grpc.StreamDesc{StreamName: "StreamingCall"}, "/grpc.testing.BenchmarkService/StreamingCall")
						if err != nil {
							b.Fatalf("NewStream failed: %v", err)
						}
						req := &testpb.SimpleRequest{Payload: &testpb.Payload{Body: []byte("a")}}
						if err := stream.SendMsg(req); err != nil {
							st, ok := status.FromError(err)
							if ok && st.Code() == codes.Unavailable {
								b.Fatalf("SendMsg failed with Unavailable: %v", err)
							}
							b.Logf("SendMsg failed: %v", err)
						}
						stream.CloseSend()
					}
				})
			})
		}
	}
}

func BenchmarkSelectionStrategies(b *testing.B) {
	fake := createBenchmarkFake()
	serverAddr := setupTestServer(b, fake)

	poolSizes := []int{1, 8, 64, 256}

	for _, size := range poolSizes {
		b.Run(fmt.Sprintf("PoolSize%d", size), func(b *testing.B) {
			poolRR := setupBenchmarkPool(b, btopt.RoundRobin, size, serverAddr)
			poolLIF := setupBenchmarkPool(b, btopt.LeastInFlight, size, serverAddr)
			poolP2 := setupBenchmarkPool(b, btopt.PowerOfTwoLeastInFlight, size, serverAddr)

			b.Run("RoundRobin", func(b *testing.B) {
				for i := 0; i < b.N; i++ {
					poolRR.selectRoundRobin()
				}
			})

			b.Run("LeastInFlight", func(b *testing.B) {
				for i := 0; i < b.N; i++ {
					poolLIF.selectLeastLoaded()
				}
			})

			b.Run("PowerOfTwoLeastInFlight", func(b *testing.B) {
				for i := 0; i < b.N; i++ {
					poolP2.selectLeastLoadedRandomOfTwo()
				}
			})
		})
	}
}

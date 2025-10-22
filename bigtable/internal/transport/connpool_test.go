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
	"net"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	btpb "cloud.google.com/go/bigtable/apiv2/bigtablepb"

	btopt "cloud.google.com/go/bigtable/internal/option"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"

	testgrpc "google.golang.org/grpc/interop/grpc_testing"
	testpb "google.golang.org/grpc/interop/grpc_testing"
	"google.golang.org/grpc/status"
)

type fakeService struct {
	testgrpc.UnimplementedBenchmarkServiceServer
	btpb.UnimplementedBigtableServer // Embed BigtableServer
	mu                               sync.Mutex
	callCount                        int
	streamSema                       chan struct{} // To control stream lifetime
	delay                            time.Duration // To simulate work
	serverErr                        error         // Error to return from server
	pingErr                          error         // Error to return from PingAndWarm
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
	// Echo the payload
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
		<-s.streamSema // Wait until released
	}

	for {
		req, err := stream.Recv()
		if err == io.EOF {
			return nil
		}
		if err != nil {
			return err
		}
		if err := stream.Send(&testpb.SimpleResponse{Payload: req.GetPayload()}); err != nil {
			return err
		}
	}
}

func (s *fakeService) PingAndWarm(ctx context.Context, req *btpb.PingAndWarmRequest) (*btpb.PingAndWarmResponse, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.pingErr != nil {
		return nil, s.pingErr
	}
	return &btpb.PingAndWarmResponse{}, nil
}

func (s *fakeService) getCallCount() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.callCount
}

func setupTestServer(t *testing.T, service *fakeService) string {
	t.Helper()
	lis, err := net.Listen("tcp", ":0")
	if err != nil {
		t.Fatalf("Failed to listen: %v", err)

	}
	srv := grpc.NewServer()
	testgrpc.RegisterBenchmarkServiceServer(srv, service)
	btpb.RegisterBigtableServer(srv, service) // Register BigtableServer
	go func() {
		if err := srv.Serve(lis); err != nil {
			// t.Logf is used here as t.Fatalf cannot be called in a separate goroutine
			t.Logf("gRPC server error: %v", err)
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

// ####################################################################################
// ## MODIFIED TESTS
// ####################################################################################

func TestSelectRoundRobin(t *testing.T) {
	pool := &BigtableChannelPool{rrIndex: 0}

	// Test empty pool
	idx, err := pool.selectRoundRobin()
	if idx != -1 {
		t.Errorf("selectRoundRobin on empty pool got index %d, want -1", idx)
	}
	if !errors.Is(err, errNoConnections) {
		t.Errorf("selectRoundRobin on empty pool got error %v, want %v", err, errNoConnections)
	}

	// Test single connection pool
	pool.conns = []*connEntry{{load: 0}}
	idx, err = pool.selectRoundRobin()
	if idx != 0 {
		t.Errorf("selectRoundRobin on single conn pool got index %d, want 0", idx)
	}
	if err != nil {
		t.Errorf("selectRoundRobin on single conn pool got error %v, want nil", err)
	}

	// Test multiple connections
	poolSize := 3
	pool.conns = make([]*connEntry, poolSize)
	pool.rrIndex = 0

	// Test wrapping around
	for i := 0; i < poolSize*2; i++ {
		expectedIdx := i % poolSize
		idx, err = pool.selectRoundRobin()
		if idx != expectedIdx {
			t.Errorf("selectRoundRobin call %d got index %d, want %d", i+1, idx, expectedIdx)
		}
		if err != nil {
			t.Errorf("selectRoundRobin call %d got error %v, want nil", i+1, err)
		}
	}
}

func TestSelectLeastLoadedRandomOfTwo(t *testing.T) {
	pool := &BigtableChannelPool{}

	// Test empty pool
	idx, err := pool.selectLeastLoadedRandomOfTwo()
	if idx != -1 {
		t.Errorf("selectLeastLoadedRandomOfTwo on empty pool got index %d, want -1", idx)
	}
	if !errors.Is(err, errNoConnections) {
		t.Errorf("selectLeastLoadedRandomOfTwo on empty pool got error %v, want %v", err, errNoConnections)
	}

	// Test single connection pool
	pool.conns = []*connEntry{{load: 0}}

	idx, err = pool.selectLeastLoadedRandomOfTwo()
	if idx != 0 {
		t.Errorf("selectLeastLoadedRandomOfTwo on single conn pool got index %d, want 0", idx)
	}
	if err != nil {
		t.Errorf("selectLeastLoadedRandomOfTwo on single conn pool got error %v, want nil", err)
	}

	pool.conns = make([]*connEntry, 5)
	// Test multiple connections
	testLoads := []int64{10, 2, 30, 4, 50} // Loads for indices 0, 1, 2, 3, 4
	for i := 0; i < len(testLoads); i++ {
		pool.conns[i] = &connEntry{}
		atomic.StoreInt64(&pool.conns[i].load, testLoads[i])
	}
	for i := 0; i < 100; i++ { // Run multiple times due to randomness
		idx, err = pool.selectLeastLoadedRandomOfTwo()
		if err != nil {
			t.Fatalf("selectLeastLoadedRandomOfTwo got unexpected error: %v", err)
		}
		if idx < 0 || idx >= len(pool.conns) {
			t.Fatalf("Selected index %d is out of bounds", idx)
		}
	}

	// Test case where loads are distinct
	pool.conns = make([]*connEntry, 3)
	testLoads = []int64{5, 1, 10}
	for i := 0; i < len(testLoads); i++ {
		pool.conns[i] = &connEntry{}
		atomic.StoreInt64(&pool.conns[i].load, testLoads[i])
	}
	for i := 0; i < 100; i++ {
		idx, err = pool.selectLeastLoadedRandomOfTwo()
		if err != nil {
			t.Errorf("selectLeastLoadedRandomOfTwo got unexpected error: %v", err)
			continue
		}
		if idx < 0 || idx >= 3 {
			t.Errorf("selectLeastLoadedRandomOfTwo got index %d, want index in [0, 2]", idx)
			continue
		}
	}

	// Test with all equal loads
	testLoads = []int64{5, 5, 5}
	pool.conns = make([]*connEntry, 3)
	for i := 0; i < len(testLoads); i++ {
		pool.conns[i] = &connEntry{}
		atomic.StoreInt64(&pool.conns[i].load, testLoads[i])
	}
	for i := 0; i < 100; i++ {
		idx, err = pool.selectLeastLoadedRandomOfTwo()
		if err != nil {
			t.Errorf("selectLeastLoadedRandomOfTwo got unexpected error: %v", err)
			continue
		}
		if idx < 0 || idx >= 3 {
			t.Errorf("Index %d out of bounds", idx)
		}
	}
}

func TestNewBigtableChannelPool(t *testing.T) { // Renamed from TestNewLeastLoadedChannelPool
	t.Run("SuccessfulCreation", func(t *testing.T) {
		poolSize := 5
		fake := &fakeService{}
		addr := setupTestServer(t, fake)

		dialFunc := func() (*BigtableConn, error) {
			return dialBigtableserver(addr)
		}

		pool, err := NewBigtableChannelPool(poolSize, btopt.LeastInFlight, dialFunc)
		if err != nil {
			t.Fatalf("NewBigtableChannelPool failed: %v", err)
		}
		defer pool.Close()

		if pool.Num() != poolSize {
			t.Errorf("Pool size got %d, want %d", pool.Num(), poolSize)
		}
		for i, conn := range pool.conns {
			if conn == nil || conn.conn == nil {
				t.Errorf("conn at index %d is nil", i)
			}
		}
		if pool.healthCheckTicker == nil {
			t.Errorf("Health check ticker was not started")
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

		_, err := NewBigtableChannelPool(poolSize, btopt.LeastInFlight, dialFunc)
		if err == nil {
			t.Errorf("NewBigtableChannelPool should have failed due to dial error, but got no error")
		}
	})
}

func TestPoolInvoke(t *testing.T) {
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
			dialFunc := func() (*BigtableConn, error) {
				return dialBigtableserver(addr)
			}

			pool, err := NewBigtableChannelPool(poolSize, strategy, dialFunc)
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

			for _, entry := range pool.conns {
				if atomic.LoadInt64(&entry.load) != 0 {
					t.Errorf("Load is non-zero after Invoke: %d", atomic.LoadInt64(&entry.load))
				}
			}
		})
	}
}

func TestPoolNewStream(t *testing.T) {
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
			dialFunc := func() (*BigtableConn, error) {
				return dialBigtableserver(addr)
			}

			pool, err := NewBigtableChannelPool(poolSize, strategy, dialFunc)
			if err != nil {
				t.Fatalf("Failed to create pool: %v", err)
			}
			defer pool.Close()

			ctx := context.Background()
			stream, err := pool.NewStream(ctx, &grpc.StreamDesc{StreamName: "StreamingCall"}, "/grpc.testing.BenchmarkService/StreamingCall")
			if err != nil {
				t.Fatalf("NewStream failed: %v", err)
			}

			loadSum := int64(0)
			for _, entry := range pool.conns {
				loadSum += atomic.LoadInt64(&entry.load)
			}
			if loadSum != 1 {
				t.Errorf("Total load after NewStream got %d, want 1.", loadSum)
			}

			req := &testpb.SimpleRequest{Payload: &testpb.Payload{Body: []byte("msg1")}}
			if err := stream.SendMsg(req); err != nil {
				t.Fatalf("SendMsg failed: %v", err)
			}
			res := &testpb.SimpleResponse{}
			if err := stream.RecvMsg(res); err != nil {
				t.Fatalf("RecvMsg failed: %v", err)
			}
			if string(res.GetPayload().GetBody()) != "msg1" {
				t.Errorf("RecvMsg got %q, want %q", string(res.GetPayload().GetBody()), "msg1")
			}

			if err := stream.CloseSend(); err != nil {
				t.Fatalf("CloseSend failed: %v", err)
			}

			if err := stream.RecvMsg(res); err != io.EOF {
				t.Errorf("Expected io.EOF after CloseSend, got %v", err)
			}

			time.Sleep(10 * time.Millisecond)
			loadSum = int64(0)
			for _, entry := range pool.conns {
				if atomic.LoadInt64(&entry.load) < 0 {
					t.Errorf("Load went negative: %d", atomic.LoadInt64(&entry.load))
				}
				loadSum += atomic.LoadInt64(&entry.load)
			}
			if loadSum != 0 {
				t.Errorf("Total load after stream completion got %d, want 0.", loadSum)
			}
		})
	}
}

func TestSelectLeastLoaded(t *testing.T) {
	pool := &BigtableChannelPool{}

	// Test empty pool
	idx, err := pool.selectLeastLoaded()
	if idx != -1 {
		t.Errorf("selectLeastLoaded on empty pool got index %d, want -1", idx)
	}
	if !errors.Is(err, errNoConnections) {
		t.Errorf("selectLeastLoaded on empty pool got error %v, want %v", err, errNoConnections)
	}

	// Test single connection pool
	pool.conns = []*connEntry{{load: 0}}
	idx, err = pool.selectLeastLoaded()
	if idx != 0 {
		t.Errorf("selectLeastLoaded on single conn pool got index %d, want 0", idx)
	}
	if err != nil {
		t.Errorf("selectLeastLoaded on single conn pool got error %v, want nil", err)
	}

	// Test multiple connections
	pool.conns = make([]*connEntry, 5)
	testLoads := []int64{3, 1, 4, 1, 5}
	for i := 0; i < len(testLoads); i++ {
		pool.conns[i] = &connEntry{}
		atomic.StoreInt64(&pool.conns[i].load, testLoads[i])

	}

	idx, err = pool.selectLeastLoaded()
	if idx != 1 {
		t.Errorf("selectLeastLoaded got index %d, want 1 for loads %v", idx, testLoads)
	}
	if err != nil {
		t.Errorf("selectLeastLoaded got error %v, want nil for loads %v", err, testLoads)
	}
}

func TestPoolClose(t *testing.T) {
	poolSize := 2
	fake := &fakeService{}
	addr := setupTestServer(t, fake)
	dialFunc := func() (*BigtableConn, error) {
		return dialBigtableserver(addr)
	}

	pool, err := NewBigtableChannelPool(poolSize, btopt.LeastInFlight, dialFunc)
	if err != nil {
		t.Fatalf("Failed to create pool: %v", err)
	}

	if err := pool.Close(); err != nil {
		t.Errorf("Close failed: %v", err)
	}

	if pool.healthCheckDone == nil {
		return // Already closed
	}
	select {
	case <-pool.healthCheckDone:
		// as expected
	case <-time.After(100 * time.Millisecond):
		t.Errorf("Health checker did not stop after Close")
	}
}

func TestMultipleStreamsSingleConn(t *testing.T) {
	poolSize := 1 // Force all streams to use the same connection
	fake := &fakeService{}
	addr := setupTestServer(t, fake)
	dialFunc := func() (*BigtableConn, error) {
		return dialBigtableserver(addr)
	}

	pool, err := NewBigtableChannelPool(poolSize, btopt.LeastInFlight, dialFunc)
	if err != nil {
		t.Fatalf("Failed to create pool: %v", err)
	}
	defer pool.Close()

	numStreams := 5
	streams := make([]grpc.ClientStream, numStreams)
	ctx := context.Background()

	// Open streams and check load
	for i := 0; i < numStreams; i++ {
		stream, err := pool.NewStream(ctx, &grpc.StreamDesc{StreamName: "StreamingCall"}, "/grpc.testing.BenchmarkService/StreamingCall")
		if err != nil {
			t.Fatalf("NewStream %d failed: %v", i, err)
		}
		streams[i] = stream
		expectedLoad := int64(i + 1)
		if atomic.LoadInt64(&pool.conns[0].load) != expectedLoad {
			t.Errorf("Load after opening stream %d is %d, want %d", i, atomic.LoadInt64(&pool.conns[0].load), expectedLoad)
		}
	}

	// Basic interaction with each stream
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

	// Close streams and check load
	for i, stream := range streams {
		if err := stream.CloseSend(); err != nil {
			t.Errorf("CloseSend on stream %d failed: %v", i, err)
		}
		// Drain the stream
		for {
			if err := stream.RecvMsg(&testpb.SimpleResponse{}); err != nil {
				if err != io.EOF {
					t.Errorf("RecvMsg on stream %d after close failed unexpectedly: %v", i, err)
				}
				break
			}
		}
		time.Sleep(10 * time.Millisecond) // Allow decrement to propagate

		expectedLoad := int64(numStreams - 1 - i)
		if atomic.LoadInt64(&pool.conns[0].load) != expectedLoad {
			t.Errorf("Load after closing stream %d is %d, want %d", i, atomic.LoadInt64(&pool.conns[0].load), expectedLoad)
		}
	}

	if atomic.LoadInt64(&pool.conns[0].load) != 0 {
		t.Errorf("Final load is %d, want 0", atomic.LoadInt64(&pool.conns[0].load))
	}
}

func TestCachingStreamDecrement(t *testing.T) {
	poolSize := 1
	fake := &fakeService{}
	addr := setupTestServer(t, fake)
	dialFunc := func() (*BigtableConn, error) {
		return dialBigtableserver(addr)
	}

	pool, err := NewBigtableChannelPool(poolSize, btopt.LeastInFlight, dialFunc)
	if err != nil {
		t.Fatalf("Failed to create pool: %v", err)
	}
	defer pool.Close()
	entry := pool.conns[0]

	t.Run("DecrementOnRecvError", func(t *testing.T) {
		fake.serverErr = errors.New("stream recv error")
		defer func() { fake.serverErr = nil }()

		ctx := context.Background()
		stream, err := pool.NewStream(ctx, &grpc.StreamDesc{StreamName: "StreamingCall"}, "/grpc.testing.BenchmarkService/StreamingCall")
		if err != nil {
			t.Fatalf("NewStream failed: %v", err)
		}
		if atomic.LoadInt64(&entry.load) != 1 {
			t.Errorf("Load is %d, want 1 after NewStream", atomic.LoadInt64(&entry.load))
		}

		err = stream.RecvMsg(&testpb.SimpleResponse{})
		if err == nil {
			t.Errorf("RecvMsg should have failed")
		}

		time.Sleep(10 * time.Millisecond)
		if atomic.LoadInt64(&entry.load) != 0 {
			t.Errorf("Load is %d, want 0 after RecvMsg error", atomic.LoadInt64(&entry.load))
		}
	})

	t.Run("DecrementOnSendError", func(t *testing.T) {
		ctx := context.Background()
		stream, err := pool.NewStream(ctx, &grpc.StreamDesc{StreamName: "StreamingCall"}, "/grpc.testing.BenchmarkService/StreamingCall")
		if err != nil {
			t.Fatalf("NewStream failed: %v", err)
		}
		if atomic.LoadInt64(&entry.load) != 1 {
			t.Errorf("Load is %d, want 1 after NewStream", atomic.LoadInt64(&entry.load))
		}

		// Close the sending side of the stream.
		if err := stream.CloseSend(); err != nil {
			t.Fatalf("CloseSend failed: %v", err)
		}

		// Wait for the server to acknowledge the closure by receiving io.EOF.
		for {
			if err := stream.RecvMsg(&testpb.SimpleResponse{}); err != nil {
				if err == io.EOF {
					break // Normal stream end.
				}
				t.Fatalf("RecvMsg failed unexpectedly while draining: %v", err)
			}
		}

		// Any subsequent SendMsg call must return an error.
		err = stream.SendMsg(&testpb.SimpleRequest{Payload: &testpb.Payload{Body: []byte("wont send")}})
		if err == nil {
			t.Errorf("SendMsg should have failed after stream is closed (RecvMsg returned io.EOF)")
		} else {
			st, ok := status.FromError(err)
			if ok {
				t.Logf("SendMsg failed as expected with status: %v", st)
			} else {
				t.Logf("SendMsg failed as expected with error: %v", err)
			}
		}

		time.Sleep(10 * time.Millisecond)
		if atomic.LoadInt64(&entry.load) != 0 {
			t.Errorf("Load is %d, want 0 after SendMsg error on closed stream", atomic.LoadInt64(&entry.load))
		}
	})

	t.Run("NoDecrementOnSuccessfulSend", func(t *testing.T) {
		fake.streamSema = make(chan struct{})

		ctx := context.Background()
		stream, err := pool.NewStream(ctx, &grpc.StreamDesc{StreamName: "StreamingCall"}, "/grpc.testing.BenchmarkService/StreamingCall")
		if err != nil {
			t.Fatalf("NewStream failed: %v", err)
		}
		if atomic.LoadInt64(&entry.load) != 1 {
			t.Errorf("Load is %d, want 1", atomic.LoadInt64(&entry.load))
		}

		if err := stream.SendMsg(&testpb.SimpleRequest{Payload: &testpb.Payload{Body: []byte("test")}}); err != nil {
			t.Fatalf("SendMsg failed: %v", err)
		}
		if atomic.LoadInt64(&entry.load) != 1 {
			t.Errorf("Load is %d, want 1 after successful SendMsg", atomic.LoadInt64(&entry.load))
		}
		close(fake.streamSema) // Release the server stream
		// Clean up the stream
		stream.CloseSend()
		for {
			if err := stream.RecvMsg(&testpb.SimpleResponse{}); err != nil {
				break
			}
		}
		time.Sleep(10 * time.Millisecond)
		if atomic.LoadInt64(&entry.load) != 0 {
			t.Errorf("Load is %d, want 0 after stream end", atomic.LoadInt64(&entry.load))
		}
	})
}

// // ####################################################################################
// // ## NEW TESTS for Health Checking & Eviction
// // ####################################################################################

func TestProbeConnection(t *testing.T) {
	fake := &fakeService{}
	addr := setupTestServer(t, fake)
	conn, err := dialBigtableserver(addr)
	if err != nil {
		t.Fatalf("Failed to dial test server: %v", err)
	}
	defer conn.Close()

	entry := &connEntry{conn: conn}
	pool := &BigtableChannelPool{}

	t.Run("SuccessfulProbe", func(t *testing.T) {
		fake.pingErr = nil
		pool.probeConnection(0, entry)
		entry.mu.Lock()
		defer entry.mu.Unlock()
		if len(entry.probeHistory) != 1 {
			t.Fatalf("probeHistory length got %d, want 1", len(entry.probeHistory))
		}
		if !entry.probeHistory[0].successful {
			t.Errorf("Probe result successful got false, want true")
		}
		if entry.successfulProbes != 1 {
			t.Errorf("successfulProbes got %d, want 1", entry.successfulProbes)
		}
		if entry.failedProbes != 0 {
			t.Errorf("failedProbes got %d, want 0", entry.failedProbes)
		}
	})

	t.Run("FailedProbe", func(t *testing.T) {
		fake.pingErr = errors.New("simulated ping error")
		pool.probeConnection(0, entry)
		entry.mu.Lock()
		defer entry.mu.Unlock()
		if len(entry.probeHistory) != 2 {
			t.Fatalf("probeHistory length got %d, want 2", len(entry.probeHistory))
		}
		if entry.probeHistory[1].successful {
			t.Errorf("Probe result successful got true, want false")
		}
		if entry.successfulProbes != 1 {
			t.Errorf("successfulProbes got %d, want 1", entry.successfulProbes)
		}
		if entry.failedProbes != 1 {
			t.Errorf("failedProbes got %d, want 1", entry.failedProbes)
		}
	})
}

func TestPruneHistory(t *testing.T) {
	entry := &connEntry{}
	pool := &BigtableChannelPool{}
	now := time.Now()

	// Add some old and new probes
	entry.probeHistory = []probeResult{
		{t: now.Add(-WindowDuration - time.Second), successful: true},       // Should be pruned
		{t: now.Add(-WindowDuration - time.Millisecond), successful: false}, // Should be pruned
		{t: now.Add(-WindowDuration + time.Millisecond), successful: true},
		{t: now, successful: false},
	}
	entry.successfulProbes = 2
	entry.failedProbes = 2

	pool.pruneHistoryLocked(entry)

	entry.mu.Lock()
	defer entry.mu.Unlock()

	if len(entry.probeHistory) != 2 {
		t.Errorf("probeHistory length got %d, want 2", len(entry.probeHistory))
	}
	if entry.successfulProbes != 1 {
		t.Errorf("successfulProbes got %d, want 1", entry.successfulProbes)
	}
	if entry.failedProbes != 1 {
		t.Errorf("failedProbes got %d, want 1", entry.failedProbes)
	}
	if !entry.probeHistory[0].t.Equal(now.Add(-WindowDuration + time.Millisecond)) {
		t.Errorf("First element in history is not the expected one")
	}
}

func TestIsHealthy(t *testing.T) {
	tests := []struct {
		name             string
		successfulProbes int
		failedProbes     int
		wantHealthy      bool
	}{
		{"NotEnoughProbes", MinProbesForEval - 1, MinProbesForEval - 1, true},
		{"HealthyBelowThreshold", 100 - FailurePercentThresh + 1, FailurePercentThresh - 1, true},
		{"UnhealthyAtThreshold", 100 - FailurePercentThresh, FailurePercentThresh, false},
		{"UnhealthyAboveThreshold", 100 - FailurePercentThresh - 1, FailurePercentThresh + 1, false},
		{"AllSuccessful", MinProbesForEval * 2, 0, true},
		{"AllFailed", 0, MinProbesForEval * 2, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			entry := &connEntry{
				successfulProbes: tt.successfulProbes,
				failedProbes:     tt.failedProbes,
			}
			if got := entry.isHealthy(); got != tt.wantHealthy {
				t.Errorf("isHealthy() = %v, want %v", got, tt.wantHealthy)
			}
		})
	}
}

func TestDetectAndEvictUnhealthy(t *testing.T) {
	const poolSize = 10
	// Modify btopt constants for easier testing
	origMinEvictionInterval := MinEvictionInterval
	origPoolwideBadThreshPercent := PoolwideBadThreshPercent
	origFailurePercentThresh := FailurePercentThresh
	origMinProbesForEval := MinProbesForEval

	MinEvictionInterval = 0 // Allow frequent evictions for test
	PoolwideBadThreshPercent = 50
	FailurePercentThresh = 20
	MinProbesForEval = 5

	defer func() {
		MinEvictionInterval = origMinEvictionInterval
		PoolwideBadThreshPercent = origPoolwideBadThreshPercent
		FailurePercentThresh = origFailurePercentThresh
		MinProbesForEval = origMinProbesForEval
	}()

	fake := &fakeService{}
	addr := setupTestServer(t, fake)
	dialFunc := func() (*BigtableConn, error) {
		return dialBigtableserver(addr)
	}

	t.Run("NoEvictionHealthyPool", func(t *testing.T) {
		pool, _ := NewBigtableChannelPool(poolSize, btopt.RoundRobin, dialFunc)
		defer pool.Close()
		for _, entry := range pool.conns {
			entry.successfulProbes = 10
			entry.failedProbes = 0
		}
		pool.detectAndEvictUnhealthy() // Should not evict anything
		if pool.Num() != poolSize {
			t.Errorf("Pool size changed, got %d, want %d", pool.Num(), poolSize)
		}
	})

	t.Run("EvictOneUnhealthy", func(t *testing.T) {
		pool, _ := NewBigtableChannelPool(poolSize, btopt.RoundRobin, dialFunc)
		defer pool.Close()
		// Make conn at index 3 unhealthy
		unhealthyIdx := 3
		for i, entry := range pool.conns {
			if i == unhealthyIdx {
				entry.successfulProbes = 7
				entry.failedProbes = 3 // 30% failure
			} else {
				entry.successfulProbes = 10
				entry.failedProbes = 0
			}
		}
		oldConn := pool.conns[unhealthyIdx].conn
		pool.detectAndEvictUnhealthy()
		if pool.conns[unhealthyIdx].conn == oldConn {
			t.Errorf("Connection at index %d was not evicted", unhealthyIdx)
		}
		if pool.Num() != poolSize {
			t.Errorf("Pool size changed, got %d, want %d", pool.Num(), poolSize)
		}
	})

	t.Run("CircuitBreakerTooManyUnhealthy", func(t *testing.T) {
		pool, _ := NewBigtableChannelPool(poolSize, btopt.RoundRobin, dialFunc)
		defer pool.Close()
		initialConns := make([]*BigtableConn, poolSize)
		// Make > 50% unhealthy
		for i := 0; i < poolSize; i++ {
			initialConns[i] = pool.conns[i].conn
			if i < 6 {
				pool.conns[i].successfulProbes = 5
				pool.conns[i].failedProbes = 5 // 50% failure
			} else {
				pool.conns[i].successfulProbes = 10
				pool.conns[i].failedProbes = 0
			}
		}
		pool.detectAndEvictUnhealthy()
		for i := 0; i < poolSize; i++ {
			if pool.conns[i].conn != initialConns[i] {
				t.Errorf("Connection at index %d was unexpectedly evicted", i)
			}
		}
	})

	t.Run("MinEvictionIntervalRespected", func(t *testing.T) {
		MinEvictionInterval = 1 * time.Hour // Set high interval
		defer func() { MinEvictionInterval = 0 }()

		pool, _ := NewBigtableChannelPool(poolSize, btopt.RoundRobin, dialFunc)
		defer pool.Close()
		pool.conns[0].failedProbes = 10 // Make it very unhealthy
		pool.lastEvictionTime = time.Now().Add(-time.Minute)

		oldConn := pool.conns[0].conn
		pool.detectAndEvictUnhealthy()
		if pool.conns[0].conn != oldConn {
			t.Errorf("Connection evicted despite MinEvictionInterval")
		}

		pool.lastEvictionTime = time.Now().Add(-2 * time.Hour) // Move last eviction time back
		pool.detectAndEvictUnhealthy()
		if pool.conns[0].conn == oldConn {
			t.Errorf("Connection not evicted after MinEvictionInterval passed")
		}
	})
}

func TestReplaceConnection(t *testing.T) {
	fake := &fakeService{}
	addr := setupTestServer(t, fake)
	dialSucceed := true
	dialFunc := func() (*BigtableConn, error) {
		if !dialSucceed {
			return nil, errors.New("simulated redial failure")
		}
		return dialBigtableserver(addr)
	}

	pool, _ := NewBigtableChannelPool(2, btopt.RoundRobin, dialFunc)
	defer pool.Close()

	idxToReplace := 0
	oldEntry := pool.conns[idxToReplace]
	atomic.StoreInt64(&oldEntry.load, 5) // Set some load

	t.Run("SuccessfulReplace", func(t *testing.T) {
		dialSucceed = true
		pool.replaceConnection(idxToReplace)
		newEntry := pool.conns[idxToReplace]

		if newEntry == oldEntry {
			t.Errorf("Connection entry was not replaced")
		}
		if newEntry.conn == oldEntry.conn {
			t.Errorf("Underlying conn was not replaced")
		}
		if atomic.LoadInt64(&oldEntry.load) != 0 {
			t.Errorf("Old entry load not reset, got %d", atomic.LoadInt64(&oldEntry.load))
		}
		// We can't easily check if oldEntry.conn.Close() was called without more mocking
	})

	t.Run("FailedRedial", func(t *testing.T) {
		dialSucceed = false
		currentEntry := pool.conns[idxToReplace]
		pool.replaceConnection(idxToReplace)
		if pool.conns[idxToReplace] != currentEntry {
			t.Errorf("Connection entry changed despite redial failure")
		}
	})
}

// Integration test for Health Checker
func TestHealthCheckerIntegration(t *testing.T) {
	// Shorten times for testing
	origProbeInterval := ProbeInterval
	origWindowDuration := WindowDuration
	origMinProbesForEval := MinProbesForEval
	origFailurePercentThresh := FailurePercentThresh
	origMinEvictionInterval := MinEvictionInterval

	ProbeInterval = 5 * time.Millisecond
	WindowDuration = 50 * time.Millisecond
	MinProbesForEval = 2
	MinEvictionInterval = 0

	defer func() {
		ProbeInterval = origProbeInterval
		WindowDuration = origWindowDuration
		MinProbesForEval = origMinProbesForEval
		FailurePercentThresh = origFailurePercentThresh
		MinEvictionInterval = origMinEvictionInterval
	}()

	fake1 := &fakeService{}
	addr1 := setupTestServer(t, fake1)
	fake2 := &fakeService{}
	addr2 := setupTestServer(t, fake2)

	dialCount := 0
	dialFunc := func() (*BigtableConn, error) {
		dialCount++
		// The first connection goes to fake1, subsequent ones go to fake2
		if dialCount == 1 {
			return dialBigtableserver(addr1)
		}
		return dialBigtableserver(addr2)
	}

	pool, err := NewBigtableChannelPool(2, btopt.RoundRobin, dialFunc)
	if err != nil {
		t.Fatalf("Failed to create pool: %v", err)
	}
	defer pool.Close()

	// Initial state: all conns good
	time.Sleep(2 * WindowDuration) // Let some probes run

	// Make server 1 start failing pings
	fake1.pingErr = errors.New("server1 unhealthy")

	// Wait for eviction to happen, check that new conns are from addr2
	evicted := false
	for i := 0; i < 200; i++ { // Timeout loop
		time.Sleep(10 * time.Millisecond)
		pool.mu.Lock()
		conn1Addr := pool.conns[0].conn.ClientConn.Target()
		conn2Addr := pool.conns[1].conn.ClientConn.Target()
		pool.mu.Unlock()

		if conn1Addr == addr2 && conn2Addr == addr2 {
			evicted = true
			break
		}
	}

	if !evicted {
		t.Errorf("Connections were not evicted and replaced with new ones from addr2")
	}

	// Check if the pool is healthy again
	fake2.pingErr = nil            // Ensure server 2 is healthy
	time.Sleep(2 * WindowDuration) // Let probes stabilize

	for i, entry := range pool.conns {
		entry.mu.Lock()
		if entry.failedProbes > 0 && time.Since(entry.lastProbeTime) < WindowDuration {
			entry.mu.Unlock()
			t.Errorf("Connection %d still shows recent failed probes after recovery", i)
		}
		entry.mu.Unlock()
		if !entry.isHealthy() {
			t.Errorf("Connection %d is not healthy after recovery", i)
		}
	}
}

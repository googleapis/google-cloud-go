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

	btopt "cloud.google.com/go/bigtable/internal/option"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"

	testgrpc "google.golang.org/grpc/interop/grpc_testing"
	testpb "google.golang.org/grpc/interop/grpc_testing"
	"google.golang.org/grpc/status"
)

type fakeService struct {
	testgrpc.UnimplementedBenchmarkServiceServer
	mu         sync.Mutex
	callCount  int
	streamSema chan struct{} // To control stream lifetime
	delay      time.Duration // To simulate work
	serverErr  error         // Error to return from server
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

func TestSelectRoundRobin(t *testing.T) {
	pool := &BigtableChannelPool{}

	// Test empty pool
	idx, err := pool.selectRoundRobin()
	if idx != -1 {
		t.Errorf("selectRoundRobin on empty pool got index %d, want -1", idx)
	}
	if err == nil {
		t.Errorf("selectRoundRobin on empty pool got nil error, want non-nil")
	}
	if err != errNoConnections {
		t.Errorf("selectRoundRobin on empty pool got error %v, want %v", err, errNoConnections)
	}

	// Test single connection pool
	pool.conns = make([]*grpc.ClientConn, 1)
	pool.load = make([]int64, 1)
	idx, err = pool.selectRoundRobin()
	if idx != 0 {
		t.Errorf("selectRoundRobin on single conn pool got index %d, want 0", idx)
	}
	if err != nil {
		t.Errorf("selectRoundRobin on single conn pool got error %v, want nil", err)
	}

	// Test multiple connections
	poolSize := 3
	pool.conns = make([]*grpc.ClientConn, poolSize)
	pool.load = make([]int64, poolSize)
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
	if err == nil {
		t.Errorf("selectLeastLoadedRandomOfTwo on empty pool got nil error, want non-nil")
	}
	if err != errNoConnections {
		t.Errorf("selectLeastLoadedRandomOfTwo on empty pool got error %v, want %v", err, errNoConnections)
	}

	// Test single connection pool
	pool.conns = make([]*grpc.ClientConn, 1)
	pool.load = make([]int64, 1)
	idx, err = pool.selectLeastLoadedRandomOfTwo()
	if idx != 0 {
		t.Errorf("selectLeastLoadedRandomOfTwo on single conn pool got index %d, want 0", idx)
	}
	if err != nil {
		t.Errorf("selectLeastLoadedRandomOfTwo on single conn pool got error %v, want nil", err)
	}

	// Test multiple connections
	pool.conns = make([]*grpc.ClientConn, 5)
	pool.load = []int64{10, 2, 30, 4, 50} // Loads for indices 0, 1, 2, 3, 4

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
	pool.load = []int64{5, 1, 10}
	pool.conns = make([]*grpc.ClientConn, 3)
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
	pool.load = []int64{5, 5, 5}
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

func TestNewLeastLoadedChannelPool(t *testing.T) {
	t.Run("SuccessfulCreation", func(t *testing.T) {
		poolSize := 5
		fake := &fakeService{}
		addr := setupTestServer(t, fake)

		dialFunc := func() (*grpc.ClientConn, error) {
			return grpc.Dial(addr, grpc.WithTransportCredentials(insecure.NewCredentials()))
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
			if conn == nil {
				t.Errorf("conn at index %d is nil", i)
			}
		}
	})

	t.Run("DialFailure", func(t *testing.T) {
		poolSize := 3
		dialCount := 0
		dialFunc := func() (*grpc.ClientConn, error) {
			dialCount++
			if dialCount > 1 {
				return nil, errors.New("simulated dial error")
			}
			fake := &fakeService{}
			addr := setupTestServer(t, fake)
			return grpc.Dial(addr, grpc.WithTransportCredentials(insecure.NewCredentials()))
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
			dialFunc := func() (*grpc.ClientConn, error) {
				return grpc.Dial(addr, grpc.WithTransportCredentials(insecure.NewCredentials()))
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

			for i, load := range pool.load {
				if load != 0 {
					t.Errorf("Load at index %d is non-zero after Invoke: %d", i, load)
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
			dialFunc := func() (*grpc.ClientConn, error) {
				return grpc.Dial(addr, grpc.WithTransportCredentials(insecure.NewCredentials()))
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
			for _, l := range pool.load {
				loadSum += l
			}
			if loadSum != 1 {
				t.Errorf("Total load after NewStream got %d, want 1. Loads: %v", loadSum, pool.load)
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
			for i, l := range pool.load {
				if l < 0 {
					t.Errorf("Load at index %d went negative: %d", i, l)
				}
				loadSum += l
			}
			if loadSum != 0 {
				t.Errorf("Total load after stream completion got %d, want 0. Loads: %v", loadSum, pool.load)
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
	if err == nil {
		t.Errorf("selectLeastLoaded on empty pool got nil error, want non-nil")
	}
	if err != errNoConnections {
		t.Errorf("selectLeastLoaded on empty pool got error %v, want %v", err, errNoConnections)
	}

	// Test single connection pool
	pool.conns = make([]*grpc.ClientConn, 1)
	pool.load = make([]int64, 1)
	idx, err = pool.selectLeastLoaded()
	if idx != 0 {
		t.Errorf("selectLeastLoaded on single conn pool got index %d, want 0", idx)
	}
	if err != nil {
		t.Errorf("selectLeastLoaded on single conn pool got error %v, want nil", err)
	}

	// Test multiple connections
	pool.conns = make([]*grpc.ClientConn, 5)
	pool.load = []int64{3, 1, 4, 1, 5}
	idx, err = pool.selectLeastLoaded()
	if idx != 1 {
		t.Errorf("selectLeastLoadedIterative got index %d, want 1 for loads %v", idx, pool.load)
	}
	if err != nil {
		t.Errorf("selectLeastLoadedIterative got error %v, want nil for loads %v", err, pool.load)
	}
}

func TestPoolClose(t *testing.T) {
	poolSize := 2
	fake := &fakeService{}
	addr := setupTestServer(t, fake)
	dialFunc := func() (*grpc.ClientConn, error) {
		return grpc.Dial(addr, grpc.WithTransportCredentials(insecure.NewCredentials()))
	}

	pool, err := NewBigtableChannelPool(poolSize, btopt.LeastInFlight, dialFunc)
	if err != nil {
		t.Fatalf("Failed to create pool: %v", err)
	}

	if err := pool.Close(); err != nil {
		t.Errorf("Close failed: %v", err)
	}
}

func TestMultipleStreamsSingleConn(t *testing.T) {
	poolSize := 1 // Force all streams to use the same connection
	fake := &fakeService{}
	addr := setupTestServer(t, fake)
	dialFunc := func() (*grpc.ClientConn, error) {
		return grpc.Dial(addr, grpc.WithTransportCredentials(insecure.NewCredentials()))
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
		if atomic.LoadInt64(&pool.load[0]) != expectedLoad {
			t.Errorf("Load after opening stream %d is %d, want %d", i, atomic.LoadInt64(&pool.load[0]), expectedLoad)
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
		if atomic.LoadInt64(&pool.load[0]) != expectedLoad {
			t.Errorf("Load after closing stream %d is %d, want %d", i, atomic.LoadInt64(&pool.load[0]), expectedLoad)
		}
	}

	if atomic.LoadInt64(&pool.load[0]) != 0 {
		t.Errorf("Final load is %d, want 0", atomic.LoadInt64(&pool.load[0]))
	}
}

func TestCachingStreamDecrement(t *testing.T) {
	t.Skip("skipped as impacting generation https://github.com/googleapis/google-cloud-go/issues/13383")
	poolSize := 1
	fake := &fakeService{}
	addr := setupTestServer(t, fake)
	dialFunc := func() (*grpc.ClientConn, error) {
		return grpc.Dial(addr, grpc.WithTransportCredentials(insecure.NewCredentials()))
	}

	pool, err := NewBigtableChannelPool(poolSize, btopt.LeastInFlight, dialFunc)
	if err != nil {
		t.Fatalf("Failed to create pool: %v", err)
	}
	defer pool.Close()

	t.Run("DecrementOnRecvError", func(t *testing.T) {
		fake.serverErr = errors.New("stream recv error")
		defer func() { fake.serverErr = nil }()

		ctx := context.Background()
		stream, err := pool.NewStream(ctx, &grpc.StreamDesc{StreamName: "StreamingCall"}, "/grpc.testing.BenchmarkService/StreamingCall")
		if err != nil {
			t.Fatalf("NewStream failed: %v", err)
		}
		if atomic.LoadInt64(&pool.load[0]) != 1 {
			t.Errorf("Load is %d, want 1 after NewStream", atomic.LoadInt64(&pool.load[0]))
		}

		err = stream.RecvMsg(&testpb.SimpleResponse{})
		if err == nil {
			t.Errorf("RecvMsg should have failed")
		}

		time.Sleep(10 * time.Millisecond)
		if atomic.LoadInt64(&pool.load[0]) != 0 {
			t.Errorf("Load is %d, want 0 after RecvMsg error", atomic.LoadInt64(&pool.load[0]))
		}
	})

	t.Run("DecrementOnSendError", func(t *testing.T) {
		ctx := context.Background()
		stream, err := pool.NewStream(ctx, &grpc.StreamDesc{StreamName: "StreamingCall"}, "/grpc.testing.BenchmarkService/StreamingCall")
		if err != nil {
			t.Fatalf("NewStream failed: %v", err)
		}
		if atomic.LoadInt64(&pool.load[0]) != 1 {
			t.Errorf("Load is %d, want 1 after NewStream", atomic.LoadInt64(&pool.load[0]))
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
			// Optionally check the error type. It's often related to a closed stream.
			st, ok := status.FromError(err)
			if ok {
				t.Logf("SendMsg failed as expected with status: %v", st)
			} else {
				t.Logf("SendMsg failed as expected with error: %v", err)
			}
		}

		// The decrement should have occurred when SendMsg returned an error.
		time.Sleep(10 * time.Millisecond) // Give a moment for the decrement to be visible.
		if atomic.LoadInt64(&pool.load[0]) != 0 {
			t.Errorf("Load is %d, want 0 after SendMsg error on closed stream", atomic.LoadInt64(&pool.load[0]))
		}
	})

	t.Run("NoDecrementOnSuccessfulSend", func(t *testing.T) {
		fake.streamSema = make(chan struct{})
		defer close(fake.streamSema)

		ctx := context.Background()
		stream, err := pool.NewStream(ctx, &grpc.StreamDesc{StreamName: "StreamingCall"}, "/grpc.testing.BenchmarkService/StreamingCall")
		if err != nil {
			t.Fatalf("NewStream failed: %v", err)
		}
		if atomic.LoadInt64(&pool.load[0]) != 1 {
			t.Errorf("Load is %d, want 1", atomic.LoadInt64(&pool.load[0]))
		}

		if err := stream.SendMsg(&testpb.SimpleRequest{Payload: &testpb.Payload{Body: []byte("test")}}); err != nil {
			t.Fatalf("SendMsg failed: %v", err)
		}
		if atomic.LoadInt64(&pool.load[0]) != 1 {
			t.Errorf("Load is %d, want 1 after successful SendMsg", atomic.LoadInt64(&pool.load[0]))
		}
	})
}

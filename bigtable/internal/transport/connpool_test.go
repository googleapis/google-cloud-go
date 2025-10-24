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
	lis, err := net.Listen("tcp", "localhost:0")
	if err != nil {
		t.Fatalf("Failed to listen: %v", err)
	}
	srv := grpc.NewServer()
	testgrpc.RegisterBenchmarkServiceServer(srv, service)
	btpb.RegisterBigtableServer(srv, service)
	go func() {
		if err := srv.Serve(lis); err != nil {
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
	pool.conns.Store([]*connEntry{{load: 0}})
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

func TestSelectLeastLoadedRandomOfTwo(t *testing.T) {
	pool := &BigtableChannelPool{}

	entry, err := pool.selectLeastLoadedRandomOfTwo()
	if entry != nil || !errors.Is(err, errNoConnections) {
		t.Errorf("Empty pool: got %v, %v, want nil, %v", entry, err, errNoConnections)
	}

	conns := []*connEntry{{load: 0}}
	pool.conns.Store(conns)
	entry, err = pool.selectLeastLoadedRandomOfTwo()
	if entry != conns[0] || err != nil {
		t.Errorf("Single conn: got %v, %v, want %v, nil", entry, err, conns[0])
	}

	testLoads := []int64{10, 2, 30, 4, 50}
	conns = make([]*connEntry, len(testLoads))
	for i := range testLoads {
		conns[i] = &connEntry{}
		atomic.StoreInt64(&conns[i].load, testLoads[i])
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
	}
}

func TestNewBigtableChannelPool(t *testing.T) {
	ctx := t.Context()

	t.Run("SuccessfulCreation", func(t *testing.T) {
		poolSize := 5
		fake := &fakeService{}
		addr := setupTestServer(t, fake)
		dialFunc := func() (*BigtableConn, error) { return dialBigtableserver(addr) }

		pool, err := NewBigtableChannelPool(ctx, poolSize, btopt.LeastInFlight, dialFunc)
		if err != nil {
			t.Fatalf("NewBigtableChannelPool failed: %v", err)
		}
		defer pool.Close()

		if pool.Num() != poolSize {
			t.Errorf("Pool size got %d, want %d", pool.Num(), poolSize)
		}
		conns := pool.getConns()
		for i, conn := range conns {
			if conn == nil || conn.conn == nil {
				t.Errorf("conn at index %d is nil", i)
			}
		}
		if pool.healthMonitor == nil {
			t.Errorf("Health monitor was not created")
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

		_, err := NewBigtableChannelPool(ctx, poolSize, btopt.LeastInFlight, dialFunc)
		if err == nil {
			t.Errorf("NewBigtableChannelPool should have failed due to dial error")
		}
	})
}

func TestPoolInvoke(t *testing.T) {
	ctx := t.Context()
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

			pool, err := NewBigtableChannelPool(ctx, poolSize, strategy, dialFunc)
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
				if atomic.LoadInt64(&entry.load) != 0 {
					t.Errorf("Load is non-zero after Invoke: %d", atomic.LoadInt64(&entry.load))
				}
			}
		})
	}
}

func TestPoolNewStream(t *testing.T) {
	ctx := t.Context()
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

			pool, err := NewBigtableChannelPool(ctx, poolSize, strategy, dialFunc)
			if err != nil {
				t.Fatalf("Failed to create pool: %v", err)
			}
			defer pool.Close()

			streamCtx := context.Background()
			stream, err := pool.NewStream(streamCtx, &grpc.StreamDesc{StreamName: "StreamingCall"}, "/grpc.testing.BenchmarkService/StreamingCall")
			if err != nil {
				t.Fatalf("NewStream failed: %v", err)
			}

			loadSum := int64(0)
			for _, entry := range pool.getConns() {
				loadSum += atomic.LoadInt64(&entry.load)
			}
			if loadSum != 1 {
				t.Errorf("Total load after NewStream got %d, want 1.", loadSum)
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
			loadSum = int64(0)
			for _, entry := range pool.getConns() {
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

	entry, err := pool.selectLeastLoaded()
	if entry != nil || !errors.Is(err, errNoConnections) {
		t.Errorf("Empty pool: got %v, %v, want nil, %v", entry, err, errNoConnections)
	}

	conns := []*connEntry{{load: 0}}
	pool.conns.Store(conns)
	entry, err = pool.selectLeastLoaded()
	if entry != conns[0] || err != nil {
		t.Errorf("Single conn: got %v, %v, want %v, nil", entry, err, conns[0])
	}

	testLoads := []int64{3, 1, 4, 1, 5}
	conns = make([]*connEntry, len(testLoads))
	for i := range testLoads {
		conns[i] = &connEntry{}
		atomic.StoreInt64(&conns[i].load, testLoads[i])
	}
	pool.conns.Store(conns)

	entry, err = pool.selectLeastLoaded()
	if err != nil {
		t.Errorf("Multi conn: got error %v, want nil", err)
	}
	if entry != conns[1] {
		t.Errorf("Multi conn: selected entry with load %d, want entry with load 1 (index 1 or 3)", atomic.LoadInt64(&entry.load))
	}
}

func TestPoolClose(t *testing.T) {
	ctx := t.Context()
	poolSize := 2
	fake := &fakeService{}
	addr := setupTestServer(t, fake)
	dialFunc := func() (*BigtableConn, error) { return dialBigtableserver(addr) }

	pool, err := NewBigtableChannelPool(ctx, poolSize, btopt.LeastInFlight, dialFunc)
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
	ctx := t.Context()
	poolSize := 1
	fake := &fakeService{}
	addr := setupTestServer(t, fake)
	dialFunc := func() (*BigtableConn, error) { return dialBigtableserver(addr) }

	pool, err := NewBigtableChannelPool(ctx, poolSize, btopt.LeastInFlight, dialFunc)
	if err != nil {
		t.Fatalf("Failed to create pool: %v", err)
	}
	defer pool.Close()

	numStreams := 5
	for i := 0; i < numStreams; i++ {
		stream, err := pool.NewStream(context.Background(), &grpc.StreamDesc{StreamName: "StreamingCall"}, "/grpc.testing.BenchmarkService/StreamingCall")
		if err != nil {
			t.Fatalf("NewStream %d failed: %v", i, err)
		}
		defer stream.CloseSend()
		expectedLoad := int64(i + 1)
		if atomic.LoadInt64(&pool.getConns()[0].load) != expectedLoad {
			t.Errorf("Load after opening stream %d is %d, want %d", i, atomic.LoadInt64(&pool.getConns()[0].load), expectedLoad)
		}
	}
}

func TestCachingStreamDecrement(t *testing.T) {
	ctx := t.Context()
	poolSize := 1
	fake := &fakeService{}
	addr := setupTestServer(t, fake)
	dialFunc := func() (*BigtableConn, error) { return dialBigtableserver(addr) }

	pool, err := NewBigtableChannelPool(ctx, poolSize, btopt.LeastInFlight, dialFunc)
	if err != nil {
		t.Fatalf("Failed to create pool: %v", err)
	}
	defer pool.Close()
	entry := pool.getConns()[0]

	t.Run("DecrementOnRecvError", func(t *testing.T) {
		fake.serverErr = errors.New("stream recv error")
		defer func() { fake.serverErr = nil }()
		atomic.StoreInt64(&entry.load, 0) // Reset load

		stream, err := pool.NewStream(context.Background(), &grpc.StreamDesc{StreamName: "StreamingCall"}, "/grpc.testing.BenchmarkService/StreamingCall")
		if err != nil {
			t.Fatalf("NewStream failed: %v", err)
		}
		stream.RecvMsg(&testpb.SimpleResponse{}) // This will error
		time.Sleep(10 * time.Millisecond)
		if atomic.LoadInt64(&entry.load) != 0 {
			t.Errorf("Load is %d, want 0 after RecvMsg error", atomic.LoadInt64(&entry.load))
		}
	})
}

// ####################################################################################
// ## Health Checking & Eviction Tests
// ####################################################################################

func TestConnHealthStateAddProbeResult(t *testing.T) {
	chs := &connHealthState{}
	chs.addProbeResult(true)
	if len(chs.probeHistory) != 1 || !chs.probeHistory[0].successful || chs.successfulProbes != 1 || chs.failedProbes != 0 {
		t.Errorf("Add successful probe failed: %+v", chs)
	}
	chs.addProbeResult(false)
	if len(chs.probeHistory) != 2 || chs.probeHistory[1].successful || chs.successfulProbes != 1 || chs.failedProbes != 1 {
		t.Errorf("Add failed probe failed: %+v", chs)
	}
}

func TestConnHealthStatePruneHistory(t *testing.T) {
	chs := &connHealthState{}
	now := time.Now()
	chs.mu.Lock()
	chs.probeHistory = []probeResult{
		{t: now.Add(-WindowDuration - time.Second), successful: true},
		{t: now.Add(-WindowDuration + time.Millisecond), successful: false},
	}
	chs.successfulProbes = 1
	chs.failedProbes = 1
	chs.mu.Unlock()

	chs.addProbeResult(true) // This triggers prune

	chs.mu.Lock()
	defer chs.mu.Unlock()
	if len(chs.probeHistory) != 2 || chs.successfulProbes != 1 || chs.failedProbes != 1 {
		t.Errorf("Prune failed, history length %d, successful %d, failed %d", len(chs.probeHistory), chs.successfulProbes, chs.failedProbes)
	}
}

func TestConnHealthStateIsHealthy(t *testing.T) {
	// ... (IsHealthy test cases as provided before) ...
}

func TestDetectAndEvictUnhealthy(t *testing.T) {
	ctx := context.Background() // Use context.Background() for tests
	const poolSize = 10
	origMinEvictionInterval, origPoolwideBadThreshPercent, origFailurePercentThresh, origMinProbesForEval := MinEvictionInterval, PoolwideBadThreshPercent, FailurePercentThresh, MinProbesForEval
	MinEvictionInterval, PoolwideBadThreshPercent, FailurePercentThresh, MinProbesForEval = 0, 50, 20, 5
	defer func() {
		MinEvictionInterval, PoolwideBadThreshPercent, FailurePercentThresh, MinProbesForEval = origMinEvictionInterval, origPoolwideBadThreshPercent, origFailurePercentThresh, origMinProbesForEval
	}()

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
		pool, err := NewBigtableChannelPool(ctx, poolSize, btopt.RoundRobin, dialFunc)
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
		pool.detectAndEvictUnhealthy()
		if pool.getConns()[unhealthyIdx].conn == oldConn {
			t.Errorf("Connection at index %d was not evicted", unhealthyIdx)
		}
	})

	t.Run("CircuitBreakerTooManyUnhealthy", func(t *testing.T) {
		pool, err := NewBigtableChannelPool(ctx, poolSize, btopt.RoundRobin, dialFunc)
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
		pool.detectAndEvictUnhealthy()
		currentConns := pool.getConns()
		for i := 0; i < poolSize; i++ {
			if currentConns[i].conn != initialConns[i] {
				t.Errorf("Connection at index %d was unexpectedly evicted", i)
			}
		}
	})
}

func TestReplaceConnection(t *testing.T) {
	ctx := t.Context()
	fake := &fakeService{}
	addr := setupTestServer(t, fake)
	dialSucceed := true
	dialFunc := func() (*BigtableConn, error) {
		if !dialSucceed {
			return nil, errors.New("simulated redial failure")
		}
		return dialBigtableserver(addr)
	}

	pool, _ := NewBigtableChannelPool(ctx, 2, btopt.RoundRobin, dialFunc)
	defer pool.Close()

	idxToReplace := 0
	oldEntry := pool.getConns()[idxToReplace]

	t.Run("SuccessfulReplace", func(t *testing.T) {
		dialSucceed = true
		pool.replaceConnection(idxToReplace)
		newEntry := pool.getConns()[idxToReplace]
		if newEntry == oldEntry || newEntry.conn == oldEntry.conn {
			t.Errorf("Connection not replaced")
		}
		if atomic.LoadInt64(&newEntry.load) != 0 {
			t.Errorf("New entry load not zero")
		}
	})

	t.Run("FailedRedial", func(t *testing.T) {
		dialSucceed = false
		currentEntry := pool.getConns()[idxToReplace]
		pool.replaceConnection(idxToReplace)
		if pool.getConns()[idxToReplace] != currentEntry {
			t.Errorf("Connection entry changed despite redial failure")
		}
	})
}

func TestHealthCheckerIntegration(t *testing.T) {
	ctx := t.Context()
	// Shorten times for testing
	origProbeInterval, origWindowDuration, origMinProbesForEval, origFailurePercentThresh, origMinEvictionInterval := ProbeInterval, WindowDuration, MinProbesForEval, FailurePercentThresh, MinEvictionInterval
	ProbeInterval, WindowDuration, MinProbesForEval, FailurePercentThresh, MinEvictionInterval = 50*time.Millisecond, 500*time.Millisecond, 2, 40, 100*time.Millisecond
	defer func() {
		ProbeInterval, WindowDuration, MinProbesForEval, FailurePercentThresh, MinEvictionInterval = origProbeInterval, origWindowDuration, origMinProbesForEval, origFailurePercentThresh, origMinEvictionInterval
	}()

	fake1, fake2 := &fakeService{}, &fakeService{}
	addr1, addr2 := setupTestServer(t, fake1), setupTestServer(t, fake2)
	dialOpts := []string{addr1, addr2}
	dialIdx := int32(0)
	dialFunc := func() (*BigtableConn, error) {
		idx := atomic.AddInt32(&dialIdx, 1) - 1
		addr := dialOpts[idx%2]
		if idx >= 2 { // Replacements always go to addr2
			addr = addr2
		}
		return dialBigtableserver(addr)
	}

	pool, err := NewBigtableChannelPool(ctx, 2, btopt.RoundRobin, dialFunc)
	if err != nil {
		t.Fatalf("Failed to create pool: %v", err)
	}
	defer pool.Close()

	time.Sleep(2 * WindowDuration) // Let initial probes run

	fake1.pingErr = errors.New("server1 unhealthy") // Make conn 0 fail

	evicted := false
	for i := 0; i < 30; i++ { // Timeout loop
		time.Sleep(ProbeInterval + MinEvictionInterval)
		conns := pool.getConns()
		if conns[0].conn.ClientConn.Target() == addr2 {
			evicted = true
			break
		}
	}
	if !evicted {
		t.Errorf("Connection 0 not evicted to addr2")
	}
	if pool.getConns()[1].conn.ClientConn.Target() != addr2 {
		t.Errorf("Connection 1 target changed unexpectedly")
	}
}

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
	"net/url"
	"reflect"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/metric/metricdata"
	"google.golang.org/grpc/codes"
	testpb "google.golang.org/grpc/interop/grpc_testing"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"

	btopt "cloud.google.com/go/bigtable/internal/option"
	"google.golang.org/api/option"

	gtransport "google.golang.org/api/transport/grpc"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

const (
	testInstanceName = "projects/test-project/instances/test-instance"
	testAppProfile   = "default"
)

// Helper function to provide default options for pool creation
func poolOpts() []BigtableChannelPoolOption {
	return []BigtableChannelPoolOption{
		WithInstanceName(testInstanceName),
		WithAppProfile(testAppProfile),
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
	conns := []*connEntry{{}}
	pool.conns.Store(&conns)
	entry, err = pool.selectRoundRobin()
	if entry == nil {
		t.Errorf("selectRoundRobin on single conn pool got nil entry")
	}
	if err != nil {
		t.Errorf("selectRoundRobin on single conn pool got error %v, want nil", err)
	}

	// Test multiple connections
	poolSize := 3
	conns = make([]*connEntry, poolSize)
	for i := 0; i < poolSize; i++ {
		conns[i] = &connEntry{}
	}
	pool.conns.Store(&conns)
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
			pool, err := NewBigtableChannelPool(ctx, tc.size, btopt.RoundRobin, tc.dial, time.Now(), poolOpts()...)
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
		conn, err := dialBigtableserver(addr)
		if err != nil {
			t.Fatalf("Failed to dial: %v", err)
		}
		defer conn.Close()
		fake.setPingCount(0)
		err = conn.Prime(ctx, testInstanceName, testAppProfile, nil)
		if err != nil {
			t.Errorf("Prime() failed: %v", err)
		}
		if conn.isALTSConn.Load() {
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
			conn, err := dialBigtableserver(addr)
			if err != nil {
				t.Fatalf("Failed to dial: %v", err)
			}
			defer conn.Close()

			fake.setPingErr(tc.pingErr)
			defer func() { fake.setPingErr(nil) }()

			err = conn.Prime(ctx, testInstanceName, testAppProfile, nil)
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
		conn, err := dialBigtableserver(addr)
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

		err = conn.Prime(primeCtx, testInstanceName, testAppProfile, nil)
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

	t.Run("PrimeSendsCorrectHeaders", func(t *testing.T) {
		conn, err := dialBigtableserver(addr)
		if err != nil {
			t.Fatalf("Failed to dial: %v", err)
		}
		defer conn.Close()

		featureFlags := metadata.Pairs("bigtable-features", "test_feature_values")
		err = conn.Prime(ctx, testInstanceName, testAppProfile, featureFlags)
		if err != nil {
			t.Fatalf("Prime() failed: %v", err)
		}

		md := fake.getPrimeMetadata()
		if md == nil {
			t.Fatalf("Prime() did not send any grpc metadata headers")
		}

		params := md.Get("x-goog-request-params")
		if len(params) != 1 {
			t.Errorf("Expected only 1 'x-goog-request-params' header, got %d", len(params))
		} else {
			expectedParams := fmt.Sprintf("name=%s&app_profile_id=%s", url.QueryEscape(testInstanceName), url.QueryEscape(testAppProfile))
			if params[0] != expectedParams {
				t.Errorf("'x-goog-request-params' header got %q, want %q", params[0], expectedParams)
			}
		}

		// Check for feature flags
		features := md.Get("bigtable-features")
		if len(features) != 1 {
			t.Errorf("Expected only 1 'bigtable-features' header, got %d", len(features))
		} else if features[0] != "test_feature_values" {
			t.Errorf("'bigtable-features' header got %q, want %q", features[0], "test_feature_values")
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

			pool, err := NewBigtableChannelPool(ctx, poolSize, strategy, dialFunc, time.Now(), poolOpts()...)
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
				if entry.unaryLoad.Load() != 0 {
					t.Errorf("Unary load is non-zero after Invoke: %d", entry.unaryLoad.Load())
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
				if entry.unaryLoad.Load() != 0 {
					t.Errorf("Unary load is non-zero after failed Invoke: %d", entry.unaryLoad.Load())
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

			pool, err := NewBigtableChannelPool(ctx, poolSize, strategy, dialFunc, time.Now(), poolOpts()...)
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
				loadSum += entry.streamingLoad.Load()
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
				loadSum += entry.streamingLoad.Load()
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
		pool, err := NewBigtableChannelPool(ctx, poolSize, btopt.RoundRobin, dialFunc, time.Now(), poolOpts()...)
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
			if pool.getConns()[0].streamingLoad.Load() != 0 {
				t.Errorf("Load is non-zero after NewStream failed: %d", pool.getConns()[0].streamingLoad.Load())
			}
			return
		}

		if pool.getConns()[0].streamingLoad.Load() != 1 {
			t.Fatalf("Load is %d, want 1 after NewStream", pool.getConns()[0].streamingLoad.Load())
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

		if pool.getConns()[0].streamingLoad.Load() != 0 {
			t.Errorf("Load is %d, want 0 after stream error", pool.getConns()[0].streamingLoad.Load())
		}
	})
}

func TestNewBigtableChannelPool(t *testing.T) {
	ctx := context.Background()
	t.Run("SuccessfulCreation", func(t *testing.T) {
		poolSize := 5
		fake := &fakeService{}
		addr := setupTestServer(t, fake)
		dialFunc := func() (*BigtableConn, error) { return dialBigtableserver(addr) }

		pool, err := NewBigtableChannelPool(ctx, poolSize, btopt.LeastInFlight, dialFunc, time.Now(), poolOpts()...)
		if err != nil {
			t.Fatalf("NewBigtableChannelPool failed: %v", err)
		}
		defer pool.Close()

		if pool.Num() != poolSize {
			t.Errorf("Pool size got %d, want %d", pool.Num(), poolSize)
		}
		// Wait for priming goroutines to likely complete
		time.Sleep(100 * time.Millisecond)

		if fake.getPingCount() < 1 {
			t.Errorf("Connections were not primed, ping count is %d", fake.getPingCount())
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

		_, err := NewBigtableChannelPool(ctx, poolSize, btopt.LeastInFlight, dialFunc, time.Now(), poolOpts()...)
		if err == nil {
			t.Errorf("NewBigtableChannelPool should have failed due to dial error")
		}
	})
}

func TestSelectLeastLoaded(t *testing.T) {
	pool := &BigtableChannelPool{}
	pool.conns.Store((&[]*connEntry{}))
	_, err := pool.selectLeastLoaded()
	if !errors.Is(err, errNoConnections) {
		t.Errorf("Empty pool: got err %v, want %v", err, errNoConnections)
	}

	testLoads := []struct{ unary, stream int32 }{
		{3, 0}, // Load: 3
		{1, 1}, // Load: 2
		{0, 2}, // Load: 2
		{5, 0}, // Load: 5
		{1, 0}, // Load: 1 (Smallest)
	}
	conns := make([]*connEntry, len(testLoads))
	expectedMinIndex := 4
	for i, loads := range testLoads {
		conns[i] = &connEntry{}
		conns[i].unaryLoad.Store(loads.unary)
		conns[i].streamingLoad.Store(loads.stream)
	}
	pool.conns.Store(&conns)

	entry, err := pool.selectLeastLoaded()
	if err != nil {
		t.Errorf("Multi conn: got error %v, want nil", err)
	}
	if entry != conns[expectedMinIndex] {
		t.Errorf("Multi conn: selected entry with load %d, want entry with load 1 (index %d)", entry.calculateConnLoad(), expectedMinIndex)
	}
}

func TestSelectLeastLoadedRandomOfTwo(t *testing.T) {
	pool := &BigtableChannelPool{}

	entry, err := pool.selectLeastLoadedRandomOfTwo()
	if entry != nil || !errors.Is(err, errNoConnections) {
		t.Errorf("Empty pool: got %v, %v, want nil, %v", entry, err, errNoConnections)
	}

	conns := []*connEntry{{}}
	pool.conns.Store(&conns)
	entry, err = pool.selectLeastLoadedRandomOfTwo()
	if entry != conns[0] || err != nil {
		t.Errorf("Single conn: got %v, %v, want %v, nil", entry, err, conns[0])
	}

	testLoads := []struct{ unary, stream int32 }{
		{10, 0}, // Load: 10
		{0, 4},  // Load: 4
		{20, 5}, // Load: 25
		{2, 1},  // Load: 3
		{0, 20}, // Load: 20
	}
	conns = make([]*connEntry, len(testLoads))
	for i, loads := range testLoads {
		conns[i] = &connEntry{}
		conns[i].unaryLoad.Store(loads.unary)
		conns[i].streamingLoad.Store(loads.stream)
	}
	pool.conns.Store(&conns)
	counts := make([]int, len(testLoads))
	iters := 1000
	for i := 0; i < iters; i++ {
		entry, err = pool.selectLeastLoadedRandomOfTwo()
		if err != nil {
			t.Fatalf("Multi conn: got unexpected error: %v", err)
		}
		if entry == nil {
			t.Fatalf("Multi conn: got nil entry")
		}

		for j, conn := range conns {
			if entry == conn {
				counts[j]++
				break
			}
		}
	}

	// numConns = 5, iterations = 1000.
	// P(most loaded, most loaded conn) = (1/5)^2 = 1/25
	// Expected conn being Most loaded = 1000/25 == 40

	mostLoadedSelections := counts[2]
	if mostLoadedSelections > iters/10 {
		t.Errorf("Most loaded entry (index 2, load 25) selected too often: %d times out of %d", mostLoadedSelections, iters)
	}
}

func TestCachingStreamDecrement(t *testing.T) {
	t.Skip("skipped as impacting generation https://github.com/googleapis/google-cloud-go/issues/13383")
	ctx := context.Background()
	poolSize := 1
	fake := &fakeService{}
	addr := setupTestServer(t, fake)
	dialFunc := func() (*BigtableConn, error) { return dialBigtableserver(addr) }

	pool, err := NewBigtableChannelPool(ctx, poolSize, btopt.LeastInFlight, dialFunc, time.Now(), poolOpts()...)
	if err != nil {
		t.Fatalf("Failed to create pool: %v", err)
	}
	defer pool.Close()
	entry := pool.getConns()[0]

	t.Run("DecrementOnRecvError", func(t *testing.T) {
		fake.serverErr = errors.New("stream recv error")
		defer func() { fake.serverErr = nil }()
		entry.streamingLoad.Store(0)

		stream, err := pool.NewStream(context.Background(), &grpc.StreamDesc{StreamName: "StreamingCall"}, "/grpc.testing.BenchmarkService/StreamingCall")
		if err != nil {
			t.Fatalf("NewStream failed: %v", err)
		}
		if entry.streamingLoad.Load() != 1 {
			t.Errorf("Load is %d, want 1 after NewStream", entry.streamingLoad.Load())
		}

		stream.RecvMsg(&testpb.SimpleResponse{})
		time.Sleep(20 * time.Millisecond)

		if entry.streamingLoad.Load() != 0 {
			t.Errorf("Load is %d, want 0 after RecvMsg error", entry.streamingLoad.Load())
		}
	})

	t.Run("DecrementOnSendError", func(t *testing.T) {
		entry.streamingLoad.Store(0)
		fake.serverErr = nil

		stream, err := pool.NewStream(context.Background(), &grpc.StreamDesc{StreamName: "StreamingCall", ServerStreams: true, ClientStreams: false}, "/grpc.testing.BenchmarkService/StreamingCall")
		if err != nil {
			t.Fatalf("NewStream failed: %v", err)
		}
		if entry.streamingLoad.Load() != 1 {
			t.Errorf("Load is %d, want 1 after NewStream", entry.streamingLoad.Load())
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
		if entry.streamingLoad.Load() != 0 {
			t.Errorf("Load is %d, want 0 after SendMsg error", entry.streamingLoad.Load())
		}
	})

	t.Run("NoDecrementOnSuccessfulSend", func(t *testing.T) {
		entry.streamingLoad.Store(0)
		fake.serverErr = nil
		fake.streamSema = make(chan struct{})

		stream, err := pool.NewStream(context.Background(), &grpc.StreamDesc{StreamName: "StreamingCall"}, "/grpc.testing.BenchmarkService/StreamingCall")
		if err != nil {
			t.Fatalf("NewStream failed: %v", err)
		}
		if entry.streamingLoad.Load() != 1 {
			t.Errorf("Load is %d, want 1", entry.streamingLoad.Load())
		}

		if err := stream.SendMsg(&testpb.SimpleRequest{Payload: &testpb.Payload{Body: []byte("test")}}); err != nil {
			t.Fatalf("SendMsg failed: %v", err)
		}
		if entry.streamingLoad.Load() != 1 {
			t.Errorf("Load is %d, want 1 after successful SendMsg", entry.streamingLoad.Load())
		}

		close(fake.streamSema)
		stream.CloseSend()
		for {
			if err := stream.RecvMsg(&testpb.SimpleResponse{}); err != nil {
				break
			}
		}
		time.Sleep(20 * time.Millisecond)
		if entry.streamingLoad.Load() != 0 {
			t.Errorf("Load is %d, want 0 after stream cleanup", entry.streamingLoad.Load())
		}
	})
}

func TestMultipleStreamsSingleConn(t *testing.T) {
	ctx := context.Background()
	poolSize := 1
	fake := &fakeService{}
	addr := setupTestServer(t, fake)
	dialFunc := func() (*BigtableConn, error) { return dialBigtableserver(addr) }

	pool, err := NewBigtableChannelPool(ctx, poolSize, btopt.LeastInFlight, dialFunc, time.Now(), poolOpts()...)
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
		if connEntry.streamingLoad.Load() != expectedLoad {
			t.Errorf("Load after opening stream %d is %d, want %d", i, connEntry.streamingLoad.Load(), expectedLoad)
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
		currentLoad := connEntry.streamingLoad.Load()
		if currentLoad != expectedLoad {
			t.Errorf("Load after closing stream %d is %d, want %d", i, currentLoad, expectedLoad)
		}
	}

	finalLoad := connEntry.streamingLoad.Load()
	if finalLoad != 0 {
		t.Errorf("Final load is %d, want 0", finalLoad)
	}
}

func TestPoolClose(t *testing.T) {
	ctx := context.Background()
	poolSize := 2
	fake := &fakeService{}
	addr := setupTestServer(t, fake)
	dialFunc := func() (*BigtableConn, error) { return dialBigtableserver(addr) }
	pool, err := NewBigtableChannelPool(ctx, poolSize, btopt.LeastInFlight, dialFunc, time.Now(), poolOpts()...)
	if err != nil {
		t.Fatalf("Failed to create pool: %v", err)
	}

	pool.Close()

	if pool.Num() != 0 {
		t.Errorf("pool.Num() got %d, want 0", pool.Num())
	}

	if len(pool.getConns()) != 0 {
		t.Errorf("pool.getConns() got non-nil after Close, want nil")
	}
}

func TestGracefulDraining(t *testing.T) {
	ctx := context.Background()
	fake := &fakeService{}
	addr := setupTestServer(t, fake)
	dialFunc := func() (*BigtableConn, error) { return dialBigtableserver(addr) }

	t.Run("DrainingOnReplaceConnection", func(t *testing.T) {
		pool, err := NewBigtableChannelPool(ctx, 1, btopt.RoundRobin, dialFunc, time.Now(), poolOpts()...)
		if err != nil {
			t.Fatalf("Failed to create pool: %v", err)
		}
		defer pool.Close()

		oldEntry := pool.getConns()[0]

		// Create a long-lived stream to simulate in-flight traffic
		fake.streamSema = make(chan struct{})
		stream, err := pool.NewStream(ctx, &grpc.StreamDesc{ServerStreams: true}, "/grpc.testing.BenchmarkService/StreamingCall")
		if err != nil {
			t.Fatalf("NewStream failed: %v", err)
		}

		if oldEntry.streamingLoad.Load() != 1 {
			t.Fatalf("Streaming load should be 1, got %d", oldEntry.streamingLoad.Load())
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

		if oldEntry.streamingLoad.Load() != 0 {
			t.Errorf("Old connection load is still %d after stream completion", oldEntry.streamingLoad.Load())
		}
		if !isConnClosed(oldEntry.conn.ClientConn) {
			t.Error("Old connection was not closed after its load dropped to zero")
		}
	})

	t.Run("SelectionSkipsDrainingConns", func(t *testing.T) {
		pool, err := NewBigtableChannelPool(ctx, 3, btopt.RoundRobin, dialFunc, time.Now(), poolOpts()...)
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

		pool, err := NewBigtableChannelPool(ctx, 1, btopt.RoundRobin, dialFunc, time.Now(), poolOpts()...)
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
		if oldEntry.streamingLoad.Load() == 0 {
			t.Error("Load was unexpectedly 0, timeout should not have been the reason for closing")
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

		pool, err := NewBigtableChannelPool(ctx, 2, btopt.RoundRobin, dialFunc, time.Now(), poolOpts()...)
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
		if newEntry.unaryLoad.Load() != 0 || newEntry.streamingLoad.Load() != 0 {
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

		pool, err := NewBigtableChannelPool(ctx, 2, btopt.RoundRobin, dialFunc, time.Now(), poolOpts()...)
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

		poolCancelled, err := NewBigtableChannelPool(ctx, 2, btopt.RoundRobin, dialFunc, time.Now(), poolOpts()...)
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

	t.Run("FailedPrime", func(t *testing.T) {
		// Pool creation should succeed
		mu.Lock()
		dialSucceed = true
		mu.Unlock()
		pingErr := status.Error(codes.Internal, "simulated ping error")
		fake.setPingErr(pingErr)
		atomic.StoreInt32(&dialCount, 0)

		pool, err := NewBigtableChannelPool(ctx, 2, btopt.RoundRobin, dialFunc, time.Now(), poolOpts()...)
		if err != nil {
			t.Fatalf("Failed to create pool: %v", err)
		}
		defer pool.Close()

		mu.Lock()
		dialSucceed = true
		mu.Unlock()
		atomic.StoreInt32(&dialCount, 0)

		currentEntry := pool.getConns()[idxToReplace]
		pool.replaceConnection(currentEntry)

		// Dial should be called
		if atomic.LoadInt32(&dialCount) != 1 {
			t.Errorf("Dial function called %d times by replaceConnection, want 1", atomic.LoadInt32(&dialCount))
		}
		// Connection should NOT be replaced as Prime() fails
		if pool.getConns()[idxToReplace] != currentEntry {
			t.Errorf("Connection entry changed despite Prime() failure")
		}
	})
}

func TestAddConnections(t *testing.T) {
	ctx := context.Background()
	fake := &fakeService{}
	addr := setupTestServer(t, fake)

	baseDialFunc := func() (*BigtableConn, error) { return dialBigtableserver(addr) }

	tests := []struct {
		name           string
		initialSize    int
		increaseDelta  int
		maxConns       int
		dialFunc       func() (*BigtableConn, error)
		primeErr       error
		wantChange     bool
		wantFinalSize  int
		wantDials      int32 // dial() for each conn
		wantPrimes     int32 // Prime() for each conn
		primeShouldErr bool
	}{
		{
			name:          "AddBelowMaxConns",
			initialSize:   1,
			increaseDelta: 2,
			maxConns:      5,
			dialFunc:      baseDialFunc,
			wantChange:    true,
			wantFinalSize: 3,
			wantDials:     2,
			wantPrimes:    2,
		},
		{
			name:          "AddUptoMaxConns",
			initialSize:   3,
			increaseDelta: 3,
			maxConns:      5,
			dialFunc:      baseDialFunc,
			wantChange:    true,
			wantFinalSize: 5,
			wantDials:     2,
			wantPrimes:    2,
		},
		{
			name:          "NoChangeIfMaxConnsReached",
			initialSize:   5,
			increaseDelta: 1,
			maxConns:      5,
			dialFunc:      baseDialFunc,
			wantChange:    false,
			wantFinalSize: 5,
			wantDials:     0,
		},
		{
			name:          "PartialDialFailurePartialAdd",
			initialSize:   1,
			increaseDelta: 3, // we want 3 delta, 4 conns
			maxConns:      5,
			dialFunc: func() func() (*BigtableConn, error) {
				var count int32
				return func() (*BigtableConn, error) {
					if atomic.AddInt32(&count, 1) > 1 {
						return nil, errors.New("simulated dial fail")
					}
					return baseDialFunc()
				}
			}(),
			wantChange:    true,
			wantFinalSize: 2, // only add 1 conns, as all dial attempt except 1 fails.
			wantDials:     3, // Attempts all 3
			wantPrimes:    1,
		},
		{
			name:          "PrimeFailureCausesNoIncrease",
			initialSize:   1,
			increaseDelta: 2,
			maxConns:      5,
			dialFunc:      baseDialFunc,
			primeErr:      errors.New("simulated prime fail"),
			wantChange:    false,
			wantFinalSize: 1, // Same as initialSize as prime fails. No point in adding
			wantDials:     2,
			wantPrimes:    2,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			fake.reset()
			if tc.primeErr != nil {
				fake.setPingErr(tc.primeErr)
			}

			innerCtx, cancel := context.WithCancel(ctx)
			defer cancel()

			pool, err := NewBigtableChannelPool(innerCtx, tc.initialSize, btopt.RoundRobin, baseDialFunc, time.Now(), poolOpts()...)
			if err != nil {
				t.Fatalf("Failed to create pool: %v", err)
			}
			defer pool.Close()

			pool.dial = tc.dialFunc // Override dial func for test

			changed := pool.addConnections(tc.increaseDelta, tc.maxConns)

			if changed != tc.wantChange {
				t.Errorf("addConnections() got change %v, want %v", changed, tc.wantChange)
			}

			finalSize := pool.Num()
			if tc.wantDials != -1 && finalSize != tc.wantFinalSize {
				t.Errorf("addConnections() final size got %d, want %d", finalSize, tc.wantFinalSize)
			}
			if int32(fake.getPingCount()) != tc.wantPrimes {
				// t.Errorf("addConnections() prime attempts got %d, want %d", fake.getPingCount(), tc.wantPrimes)
			}

		})
	}
}

func TestRemoveConnections(t *testing.T) {
	ctx := context.Background()
	fake := &fakeService{}
	addr := setupTestServer(t, fake)

	dialFunc := func() (*BigtableConn, error) {
		time.Sleep(time.Millisecond * 2)
		return dialBigtableserver(addr)
	}

	tests := []struct {
		name           string
		initialSize    int
		decreaseDelta  int
		minConns       int
		maxRemoveConns int
		wantChange     bool
		wantFinalSize  int
	}{
		{
			name:           "RemoveAboveMinConns",
			initialSize:    5,
			decreaseDelta:  2,
			minConns:       1,
			maxRemoveConns: 5,
			wantChange:     true,
			wantFinalSize:  3,
		},
		{
			name:           "RemoveToMinConns",
			initialSize:    3,
			decreaseDelta:  3,
			minConns:       1,
			maxRemoveConns: 5,
			wantChange:     true,
			wantFinalSize:  1,
		},
		{
			name:           "NoChange",
			initialSize:    3,
			decreaseDelta:  3,
			minConns:       2,
			maxRemoveConns: 5,
			wantChange:     true,
			wantFinalSize:  2,
		},
		{
			name:           "CapToMaxRemoveConns",
			initialSize:    10,
			decreaseDelta:  8,
			minConns:       1,
			maxRemoveConns: 3,
			wantChange:     true,
			wantFinalSize:  7,
		},
		{
			name:           "NoRemoveAtMinConns",
			initialSize:    1,
			decreaseDelta:  1,
			minConns:       1,
			maxRemoveConns: 5,
			wantChange:     false,
			wantFinalSize:  1,
		},
		{
			name:           "DeltaZeroNoChnage",
			initialSize:    5,
			decreaseDelta:  0,
			minConns:       1,
			maxRemoveConns: 5,
			wantChange:     false,
			wantFinalSize:  5,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			pool, err := NewBigtableChannelPool(ctx, tc.initialSize, btopt.RoundRobin, dialFunc, time.Now(), poolOpts()...)
			if err != nil {
				t.Fatalf("Failed to create pool: %v", err)
			}
			defer pool.Close()
			time.Sleep(time.Duration(tc.initialSize*10) * time.Millisecond)

			changed := pool.removeConnections(tc.decreaseDelta, tc.minConns, tc.maxRemoveConns)

			if changed != tc.wantChange {
				t.Errorf("removeConnections() got change %v, want %v", changed, tc.wantChange)
			}

			finalConns := pool.getConns()
			if len(finalConns) != tc.wantFinalSize {
				t.Errorf("removeConnections() final size got %d, want %d", len(finalConns), tc.wantFinalSize)
			}
		})
	}

	t.Run("VerifyOldestIsRemoved", func(t *testing.T) {
		poolSize := 5
		pool, err := NewBigtableChannelPool(ctx, poolSize, btopt.RoundRobin, dialFunc, time.Now(), poolOpts()...)
		if err != nil {
			t.Fatalf("Failed to create pool: %v", err)
		}
		defer pool.Close()

		initialConns := make([]*connEntry, poolSize)
		copy(initialConns, pool.getConns())

		sort.Slice(initialConns, func(i, j int) bool {
			return initialConns[i].createdAt() < initialConns[j].createdAt()
		})

		numToRemove := 2
		pool.removeConnections(numToRemove, 1, 5)

		finalConns := pool.getConns()
		if len(finalConns) != poolSize-numToRemove {
			t.Fatalf("Final pool size %d, want %d", len(finalConns), poolSize-numToRemove)
		}

		finalConnsSet := make(map[*connEntry]bool)
		for _, entry := range finalConns {
			finalConnsSet[entry] = true
		}

		for i := 0; i < numToRemove; i++ {
			oldestEntry := initialConns[i]
			if finalConnsSet[oldestEntry] {
				t.Errorf("Oldest connection at index %d was not removed", i)
			}
			if !oldestEntry.isDraining() {
				t.Errorf("Oldest connection at index %d was not marked draining", i)
			}
		}
		for i := numToRemove; i < poolSize; i++ {
			newerEntry := initialConns[i]
			if !finalConnsSet[newerEntry] {
				t.Errorf("Newer connection at index %d was unexpectedly removed", i)
			}
		}
	})
}

func TestConnPoolStatisticsVisitor(t *testing.T) {
	ctx := context.Background()
	poolSize := 3
	fake := &fakeService{}
	addr := setupTestServer(t, fake)
	dialFunc := func() (*BigtableConn, error) { return dialBigtableserver(addr) }

	pool, err := NewBigtableChannelPool(ctx, poolSize, btopt.RoundRobin, dialFunc, time.Now(), poolOpts()...)
	if err != nil {
		t.Fatalf("Failed to create pool: %v", err)
	}
	defer pool.Close()

	// Wait for connections to be established
	time.Sleep(100 * time.Millisecond)

	conns := pool.getConns()
	if len(conns) != poolSize {
		t.Fatalf("Pool size mismatch: got %d, want %d", len(conns), poolSize)
	}

	testData := []struct {
		unary     int32
		streaming int32
		errors    int64
		isALTS    bool
	}{
		{unary: 5, streaming: 2, errors: 10, isALTS: false},
		{unary: 0, streaming: 1, errors: 0, isALTS: true},
		{unary: 3, streaming: 0, errors: 5, isALTS: false},
	}

	if len(testData) != poolSize {
		t.Fatalf("Test data size: pool size mismatch: got %d, want %d", len(testData), poolSize)
	}

	for i, data := range testData {
		if i < len(conns) {
			conns[i].unaryLoad.Store(data.unary)
			conns[i].streamingLoad.Store(data.streaming)
			conns[i].errorCount.Store(data.errors)
			conns[i].conn.isALTSConn.Store(data.isALTS)
		}
	}

	// Get the snapshot
	stats := pool.connPoolStatsSupplier()

	if len(stats) != poolSize {
		t.Errorf("Snapshot size mismatch: got %d, want %d", len(stats), poolSize)
	}

	expectedStats := make([]connPoolStats, poolSize)
	for i, data := range testData {
		expectedStats[i] = connPoolStats{
			OutstandingUnaryLoad:     data.unary,
			OutstandingStreamingLoad: data.streaming,
			ErrorCount:               data.errors,
			IsALTSUsed:               data.isALTS,
			LBPolicy:                 btopt.RoundRobin.String(),
		}
	}

	sort.Slice(stats, func(i, j int) bool { return stats[i].OutstandingUnaryLoad < stats[j].OutstandingUnaryLoad })
	sort.Slice(expectedStats, func(i, j int) bool {
		return expectedStats[i].OutstandingUnaryLoad < expectedStats[j].OutstandingUnaryLoad
	})

	if !reflect.DeepEqual(stats, expectedStats) {
		t.Errorf("Snapshot data mismatch:\ngot:  %v\nwant: %v", stats, expectedStats)
	}

	// Verify error counts are reset
	connsAfter := pool.getConns()
	for i, entry := range connsAfter {
		if entry.errorCount.Load() != 0 {
			t.Errorf("entry[%d].errorCount was not reset: got %d, want 0", i, entry.errorCount.Load())
		}
	}
}

func TestRecordClientStartUp(t *testing.T) {
	ctx := context.Background()
	reader := metric.NewManualReader()
	provider := metric.NewMeterProvider(metric.WithReader(reader))

	fake := &fakeService{}
	addr := setupTestServer(t, fake)
	dialFunc := func() (*BigtableConn, error) { return dialBigtableserver(addr) }

	poolSize := 1
	startTime := time.Now().Add(-500 * time.Millisecond)
	channelPoolOptions := append(poolOpts(), WithMeterProvider(provider))
	pool, err := NewBigtableChannelPool(ctx, poolSize, btopt.RoundRobin, dialFunc, startTime, channelPoolOptions...)

	if err != nil {
		t.Fatalf("NewBigtableChannelPool failed: %v", err)
	}

	defer pool.Close()

	// Collect metrics
	rm := metricdata.ResourceMetrics{}
	if err := reader.Collect(ctx, &rm); err != nil {
		t.Fatalf("Failed to collect metrics: %v", err)
	}

	if len(rm.ScopeMetrics) == 0 {
		t.Fatalf("No scope metrics found")
	}
	sm := rm.ScopeMetrics[0]
	if sm.Scope.Name != clientMeterName {
		t.Errorf("Scope name got %q, want %q", sm.Scope.Name, clientMeterName)
	}

	if len(sm.Metrics) == 0 {
		t.Fatalf("No metrics found")
	}
	m := sm.Metrics[0]

	if m.Name != "startup_time" {
		t.Errorf("Metric name got %q, want %q", m.Name, "startup_time")
	}
	if m.Unit != "ms" {
		t.Errorf("Metric unit got %q, want %q", m.Unit, "ms")
	}

	hist, ok := m.Data.(metricdata.Histogram[float64])
	if !ok {
		t.Fatalf("Metric data is not a Histogram: %T", m.Data)
	}

	if len(hist.DataPoints) != 1 {
		t.Fatalf("Expected 1 data point, got %d", len(hist.DataPoints))
	}
	dp := hist.DataPoints[0]
	expectedAttrs := attribute.NewSet(
		attribute.String("transport_type", "unknown"),
		attribute.String("status", "OK"),
	)
	if !dp.Attributes.Equals(&expectedAttrs) {
		t.Errorf("Attributes got %v, want %v", dp.Attributes, expectedAttrs)
	}
	// Check count on bucket
	if dp.Count != 1 {
		t.Errorf("Data point count got %d, want 1", dp.Count)
	}
	// sanity check to see if it is greater than 500.
	if dp.Sum < 500 {
		t.Errorf("Expected positive sum > 500, got %f", dp.Sum)
	}
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
	pool, err := NewBigtableChannelPool(ctx, poolSize, strategy, dialFunc, time.Now(), poolOpts()...)
	if err != nil {
		b.Fatalf("Failed to create pool: %v", err)
	}

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

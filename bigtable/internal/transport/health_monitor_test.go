// Copyright 2025 Google LLC
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//	http://www.apache.org/licenses/LICENSE-2.0
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
	"io"
	"log"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	btopt "cloud.google.com/go/bigtable/internal/option"
	"google.golang.org/grpc"
	testpb "google.golang.org/grpc/interop/grpc_testing"
)

func TestConnHealthStateAddProbeResult(t *testing.T) {
	chs := &connHealthState{}
	config := btopt.DefaultHealthCheckConfig()
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
	config := btopt.DefaultHealthCheckConfig()
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
		config := btopt.DefaultHealthCheckConfig()
		if !config.Enabled {
			t.Fatal("DefaultHealthCheckConfig.Enabled should be true for this test")
		}
		chm := NewChannelHealthMonitor(config, nil)
		chm.Stop()
		chm.Stop() // The sync.Once should prevent a panic on double close
		select {
		case <-chm.done:
		default:
			t.Errorf("chm.done not closed after Stop()")
		}
	})

	t.Run("Disabled", func(t *testing.T) {
		config := btopt.DefaultHealthCheckConfig()
		config.Enabled = false
		chm := NewChannelHealthMonitor(config, nil)
		chm.Stop()
		select {
		case <-chm.done:
			t.Errorf("chm.done was closed, but monitor was disabled")
		default:
		}
	})
}

func TestRunProbesWhenContextDone(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	fake := &fakeService{}
	addr := setupTestServer(t, fake)
	dialFunc := func() (*BigtableConn, error) { return dialBigtableserver(addr) }
	pool, err := NewBigtableChannelPool(ctx, 2, btopt.RoundRobin, dialFunc, log.Default(), nil, WithHealthCheckConfig(btopt.DefaultHealthCheckConfig()))
	if err != nil {
		t.Fatalf("Failed to create pool: %v", err)
	}
	defer pool.Close()

	probeCtx, cancelProbe := context.WithCancel(ctx)
	cancelProbe()

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
	config := btopt.HealthCheckConfig{MinProbesForEval: 3, FailurePercentThresh: 50}
	tests := []struct {
		name       string
		results    []bool
		isHealthy  bool
		numSuccess int
		numFailed  int
	}{
		{"NotEnoughProbes", []bool{true, false}, true, 1, 1},
		{"Healthy", []bool{true, true, false}, true, 2, 1},
		{"Unhealthy", []bool{true, false, false, false}, false, 1, 3},
		{"JustUnhealthy", []bool{true, true, false, false, false}, false, 2, 3},
		{"AllSuccessful", []bool{true, true, true}, true, 3, 0},
		{"AllFailed", []bool{false, false, false}, false, 0, 3},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			chs := &connHealthState{}
			for _, r := range tc.results {
				chs.addProbeResult(r, time.Minute)
			}

			if got := chs.isHealthy(config.MinProbesForEval, config.FailurePercentThresh); got != tc.isHealthy {
				t.Errorf("isHealthy() got %v, want %v", got, tc.isHealthy)
			}
			if chs.successfulProbes != tc.numSuccess || chs.failedProbes != tc.numFailed {
				t.Errorf("counts got success=%d, failed=%d; want success=%d, failed=%d", chs.successfulProbes, chs.failedProbes, tc.numSuccess, tc.numFailed)
			}
		})
	}
}

func TestDetectAndEvictUnhealthy(t *testing.T) {
	ctx := context.Background()
	const poolSize = 10
	testConfig := btopt.HealthCheckConfig{
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
		for i := 0; i < successful+failed; i++ {
			entry.health.probeHistory = append(entry.health.probeHistory, probeResult{t: time.Now()})
		}
	}

	t.Run("EvictOneUnhealthy", func(t *testing.T) {
		pool, err := NewBigtableChannelPool(ctx, poolSize, btopt.RoundRobin, dialFunc, log.Default(), nil, WithHealthCheckConfig(testConfig))
		if err != nil {
			t.Fatalf("Failed to create pool: %v", err)
		}
		defer pool.Close()

		monitor, ok := findMonitor[*ChannelHealthMonitor](pool)
		if !ok {
			t.Fatal("Could not find ChannelHealthMonitor in pool")
		}

		unhealthyIdx := 3
		conns := pool.getConns()
		for _, entry := range conns {
			setupHealth(entry, 10, 0) // Healthy
		}
		setupHealth(conns[unhealthyIdx], 7, 3) // 30% failure > 20% thresh -> Unhealthy
		pool.conns.Store(conns)

		oldConn := pool.getConns()[unhealthyIdx].conn
		if !pool.detectAndEvictUnhealthy(pool.hcConfig, monitor.AllowEviction, monitor.RecordEviction) {
			t.Fatal("Connection was not evicted")
		}
		time.Sleep(50 * time.Millisecond) // Allow replacement goroutine to run
		if pool.getConns()[unhealthyIdx].conn == oldConn {
			t.Errorf("Connection at index %d was not replaced", unhealthyIdx)
		}
	})

	t.Run("CircuitBreakerTooManyUnhealthy", func(t *testing.T) {
		pool, err := NewBigtableChannelPool(ctx, poolSize, btopt.RoundRobin, dialFunc, log.Default(), nil, WithHealthCheckConfig(testConfig))
		if err != nil {
			t.Fatalf("Failed to create pool: %v", err)
		}
		defer pool.Close()

		monitor, ok := findMonitor[*ChannelHealthMonitor](pool)
		if !ok {
			t.Fatal("Could not find ChannelHealthMonitor in pool")
		}

		conns := pool.getConns()
		for i, entry := range conns {
			if i < 6 { // 60% unhealthy > 50% PoolwideBadThreshPercent
				setupHealth(entry, 5, 5)
			} else {
				setupHealth(entry, 10, 0)
			}
		}
		pool.conns.Store(conns)
		if pool.detectAndEvictUnhealthy(pool.hcConfig, monitor.AllowEviction, monitor.RecordEviction) {
			t.Error("Connection was evicted when circuit breaker should have tripped")
		}
	})
}

func TestHealthCheckerIntegration(t *testing.T) {
	ctx := context.Background()
	// Shorten times for testing
	testHCConfig := btopt.HealthCheckConfig{
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

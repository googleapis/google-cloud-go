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
	"log"
	"math"
	"sync/atomic"
	"testing"
	"time"

	btopt "cloud.google.com/go/bigtable/internal/option"
)

func TestDynamicChannelScaling(t *testing.T) {
	ctx := context.Background()
	fake := &fakeService{}
	addr := setupTestServer(t, fake)
	dialFunc := func() (*BigtableConn, error) { return dialBigtableserver(addr) }

	baseConfig := btopt.DynamicChannelPoolConfig{
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
		configOpt   func(*btopt.DynamicChannelPoolConfig)
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
			configOpt: func(cfg *btopt.DynamicChannelPoolConfig) {
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

			dynamicMonitor, ok := findMonitor[*DynamicScaleMonitor](pool)
			if !ok {
				t.Fatal("Could not find ChannelHealthMonitor in pool")
			}
			dynamicMonitor.evaluateAndScale()

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
		dynamicMonitor, ok := findMonitor[*DynamicScaleMonitor](pool)
		if !ok {
			t.Fatal("Could not find ChannelHealthMonitor in pool")
		}
		dynamicMonitor.mu.Lock()
		dynamicMonitor.lastScalingTime = time.Now()
		dynamicMonitor.mu.Unlock()

		dynamicMonitor.evaluateAndScale()
		if gotSize := pool.Num(); gotSize != initialSize {
			t.Errorf("Pool size changed to %d, want %d (should be throttled)", gotSize, initialSize)
		}

		// 2. Allow scaling again by moving lastScalingTime to the past
		dynamicMonitor.mu.Lock()
		dynamicMonitor.lastScalingTime = time.Now().Add(-10 * time.Minute)
		dynamicMonitor.mu.Unlock()

		dynamicMonitor.evaluateAndScale()
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

		dynamicMonitor, ok := findMonitor[*DynamicScaleMonitor](pool)
		if !ok {
			t.Fatal("Could not find ChannelHealthMonitor in pool")
		}
		dynamicMonitor.evaluateAndScale()
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

	dynConfig := btopt.DynamicChannelPoolConfig{
		Enabled:              true,
		MinConns:             2,
		MaxConns:             5,
		AvgLoadHighThreshold: 10,
		AvgLoadLowThreshold:  3,
		MinScalingInterval:   0,
		CheckInterval:        20 * time.Millisecond,
		MaxRemoveConns:       2,
	}

	hcConfig := btopt.HealthCheckConfig{
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

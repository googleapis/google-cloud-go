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
		Enabled:                          true,
		MinConns:                         2,
		MaxConns:                         10,
		AvgLoadHighThreshold:             10.0,             // Scale up if avg load >= 10
		AvgLoadLowThreshold:              3.0,              // Scale down if avg load <= 3
		MinScalingInterval:               0,                // Disable time throttling for most tests
		CheckInterval:                    10 * time.Second, // Not directly used by calling evaluateAndScale
		MaxRemoveConns:                   3,
		ContinuousDownscaleRunsThreshold: 3,
		MaxScaleUpPercentage:             30,
	}
	tests := []struct {
		name          string
		initialSize   int
		configOpt     func(*btopt.DynamicChannelPoolConfig)
		setLoad       func(conns []*connEntry)
		wantSize      int
		evaluateCalls int
	}{
		{
			name:        "ScaleUp",
			initialSize: 3,
			setLoad: func(conns []*connEntry) {
				// Target load per conn is 6.5.
				// Total load = 12 * 3 = 36. Desired = ceil(36 / 6.5) = 6.
				// try to add 3 conns
				setConnLoads(conns, 12, 0)
			},
			// Total load = 3 * 12 = 36. Desired = ceil(36 / 6.5) = 6
			// capped by 30% of 3, ceil (0.9) => 1
			wantSize:      4,
			evaluateCalls: 1,
		},
		{
			name:        "ScaleUpCappedAtMax",
			initialSize: 8,
			setLoad: func(conns []*connEntry) {
				setConnLoads(conns, 20, 0) // Avg load 20 > 10
			},
			// Total load = 8 * 20 = 160. Desired = ceil(160 / 6.5) = 25. Capped at MaxConns = 10
			wantSize:      10,
			evaluateCalls: 1,
		},
		{
			name:        "ScaleDown",
			initialSize: 9,
			setLoad: func(conns []*connEntry) {
				setConnLoads(conns, 1, 0) // Avg load 1 < 3
			},
			// Total load = 9 * 1 = 9. Desired = ceil(9 / 6.5) = 2.
			wantSize:      6,
			evaluateCalls: 3,
		},
		{
			name:        "ScaleDownCappedAtMin",
			initialSize: 3,
			setLoad: func(conns []*connEntry) {
				setConnLoads(conns, 1, 0) // Avg load 1 < 3
			},
			// Total load = 3 * 1 = 3. Desired = ceil(3 / 6.5) = 1. Capped at MinConns = 2
			wantSize:      2,
			evaluateCalls: 3,
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
			wantSize:      10 - 2,
			evaluateCalls: 3,
		},
		{
			name:        "NoScaleUp",
			initialSize: 5,
			setLoad: func(conns []*connEntry) {
				setConnLoads(conns, 7, 0) // 3 < Avg load 7 < 10
			},
			wantSize:      5,
			evaluateCalls: 1,
		},
		{
			name:        "NoScaleDown",
			initialSize: 5,
			setLoad: func(conns []*connEntry) {
				setConnLoads(conns, 5, 1) // Weighted load 5*1 + 1*2 = 7.  3 < 7 < 10
			},
			wantSize:      5,
			evaluateCalls: 3,
		},
		{
			name:        "ScaleUpAddAtLeastOne",
			initialSize: 2,
			setLoad: func(conns []*connEntry) {
				// 10*2= 20 total load, conn count = ceil(20/6.5) = 4
				setConnLoads(conns, 10, 0) // Avg load 10, right at threshold.
			},
			// Total load = 20. Desired = ceil(20 / 6.5) = 4. Add 2 but capped by 30%
			wantSize:      3,
			evaluateCalls: 1,
		},
		{
			name:        "NoScaleDownWithEvaluations<ContinuousDownscaleRunsThreshold",
			initialSize: 6,
			setLoad: func(conns []*connEntry) {
				setConnLoads(conns, 1, 0) // Avg load 1 < 3 (Low load)
			},
			wantSize:      6,
			evaluateCalls: 2,
		},
		{
			name:        "ScaleUpCappedByMaxScaleUpPercentage",
			initialSize: 10,
			configOpt: func(cfg *btopt.DynamicChannelPoolConfig) {
				cfg.MaxConns = 50             // Increase max conns so it doesn't artificially cap the test
				cfg.MaxScaleUpPercentage = 20 // 20% of 10 = 2 max additions allowed
			},
			setLoad: func(conns []*connEntry) {
				// Target load per conn is 6.5.
				// Total load = 10 * 20 = 200. Desired = ceil(200 / 6.5) = 31.
				// Normally it would try to add 21 connections.
				setConnLoads(conns, 20, 0)
			},
			// Capped at adding 2 connections
			wantSize:      12,
			evaluateCalls: 1,
		},
		{
			name:        "ScaleUpNotCappedByMaxScaleUpPercentage",
			initialSize: 10,
			configOpt: func(cfg *btopt.DynamicChannelPoolConfig) {
				cfg.MaxConns = 50
				cfg.MaxScaleUpPercentage = 100 // 100% of 10 = 10
			},
			setLoad: func(conns []*connEntry) {
				// Total load = 10 * 10 = 100. Desired = ceil(100 / 6.5) = 16.
				setConnLoads(conns, 10, 0)
			},
			//  max allowed 10, so 6 is fine
			wantSize:      16,
			evaluateCalls: 1,
		},
		{
			name:        "ScaleUpCeilFractionalCap",
			initialSize: 4,
			configOpt: func(cfg *btopt.DynamicChannelPoolConfig) {
				cfg.MaxConns = 20
				cfg.MaxScaleUpPercentage = 30 // 30% of 4 = 1.2 -> ceil(1.2) = 2 allowed
			},
			setLoad: func(conns []*connEntry) {
				// Total load = 4 * 20 = 80. Desired = ceil(80 / 6.5) = 13.
				// Wants to add 9 connections.
				setConnLoads(conns, 20, 0)
			},
			// Capped at adding 2 connections due to math.Ceil(1.2)
			wantSize:      6,
			evaluateCalls: 1,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			config := baseConfig
			if tc.configOpt != nil {
				tc.configOpt(&config)
			}

			pool, err := NewBigtableChannelPool(ctx, tc.initialSize, btopt.RoundRobin, dialFunc, time.Now(), poolOpts()...)
			if err != nil {
				t.Fatalf("Failed to create pool: %v", err)
			}
			defer pool.Close()

			dsm := NewDynamicScaleMonitor(config, pool)

			if tc.setLoad != nil {
				tc.setLoad(pool.getConns())
			}

			calls := tc.evaluateCalls
			if calls == 0 {
				calls = 1
			}

			// Simulate the ticker calling evaluateAndScale
			for i := 0; i < calls; i++ {
				dsm.evaluateAndScale()
			}
			time.Sleep(50 * time.Millisecond) // Allow add/remove goroutines to potentially run

			if gotSize := pool.Num(); gotSize != tc.wantSize {
				t.Errorf("evaluateAndScale() resulted in pool size %d, want %d", gotSize, tc.wantSize)
			}
		})
	}

	t.Run("MinScalingInterval", func(t *testing.T) {
		config := baseConfig
		config.MinScalingInterval = 5 * time.Minute
		initialSize := 3

		pool, err := NewBigtableChannelPool(ctx, initialSize, btopt.RoundRobin, dialFunc, time.Now(), poolOpts()...)
		if err != nil {
			t.Fatalf("Failed to create pool: %v", err)
		}
		defer pool.Close()

		dsm := NewDynamicScaleMonitor(config, pool)

		// Set load to trigger scale up
		setConnLoads(pool.getConns(), 15, 0)

		dsm.mu.Lock()
		dsm.lastScalingTime = time.Now() // Simulate recent scaling
		dsm.mu.Unlock()

		dsm.evaluateAndScale()
		if gotSize := pool.Num(); gotSize != initialSize {
			t.Errorf("Pool size changed to %d, want %d (should be throttled)", gotSize, initialSize)
		}

		// 2. Allow scaling again by moving lastScalingTime to the past
		dsm.mu.Lock()
		dsm.lastScalingTime = time.Now().Add(-10 * time.Minute) // Allow scaling again
		dsm.mu.Unlock()

		dsm.evaluateAndScale()
		if gotSize := pool.Num(); gotSize == initialSize {
			t.Errorf("Pool size %d, want > %d (should have scaled up)", gotSize, initialSize)
		} else {
			t.Logf("Scaled up to %d connections", gotSize)
		}
	})
	t.Run("EmptyPoolNoAction", func(t *testing.T) {
		config := baseConfig

		pool, err := NewBigtableChannelPool(ctx, 1, btopt.RoundRobin, dialFunc, time.Now(), poolOpts()...)
		if err != nil {
			t.Fatalf("Failed to create pool: %v", err)
		}
		defer pool.Close()

		conns := []*connEntry{}
		// use an empty slice.
		pool.conns.Store(&conns)

		dsm := NewDynamicScaleMonitor(config, pool)
		// record lastscaling time
		dsm.mu.Lock()
		lastScalingTime := time.Now().Add(-1 * time.Minute)
		dsm.lastScalingTime = lastScalingTime
		dsm.mu.Unlock()

		dsm.evaluateAndScale() // no-op.

		if gotSize := pool.Num(); gotSize != 0 {
			t.Errorf("evaluateAndScale() with empty pool resulted in size %d, want 0", gotSize)
		}

		// Check that lastScalingTime was NOT updated.
		dsm.mu.Lock()
		defer dsm.mu.Unlock()
		if !dsm.lastScalingTime.Equal(lastScalingTime) {
			t.Errorf("lastScalingTime was updated to %v on empty pool, but should not have been", dsm.lastScalingTime)
		}
	})

}

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
	"fmt"
	"math"
	"sync"
	"time"

	btopt "cloud.google.com/go/bigtable/internal/option"
)

// DynamicScaleMonitor manages dynamic scaling of the connection pool.
type DynamicScaleMonitor struct {
	config          btopt.DynamicChannelPoolConfig
	pool            *BigtableChannelPool
	lastScalingTime time.Time
	mu              sync.Mutex
	ticker          *time.Ticker
	done            chan struct{}
	stopOnce        sync.Once
}

// NewDynamicScaleMonitor creates a new DynamicScaleMonitor.
func NewDynamicScaleMonitor(config btopt.DynamicChannelPoolConfig, pool *BigtableChannelPool) *DynamicScaleMonitor {
	return &DynamicScaleMonitor{
		config: config,
		pool:   pool,
		done:   make(chan struct{}),
	}
}

// Start begins the periodic scaling check loop.
func (dsm *DynamicScaleMonitor) Start(ctx context.Context) {
	if !dsm.config.Enabled {
		return
	}
	dsm.ticker = time.NewTicker(dsm.config.CheckInterval)
	go func() {
		defer dsm.ticker.Stop()
		for {
			select {
			case <-dsm.ticker.C:
				dsm.evaluateAndScale()
			case <-dsm.done:
				return
			case <-ctx.Done():
				return
			}
		}
	}()
}

// Stop terminates the scaling check loop.
func (dsm *DynamicScaleMonitor) Stop() {
	if !dsm.config.Enabled {
		return
	}
	dsm.stopOnce.Do(func() {
		close(dsm.done)
	})
}

func (dsm *DynamicScaleMonitor) evaluateAndScale() {
	dsm.mu.Lock()
	defer dsm.mu.Unlock()

	if time.Since(dsm.lastScalingTime) < dsm.config.MinScalingInterval {
		return // Too soon since last scaling operation
	}

	conns := dsm.pool.getConns()
	numConns := len(conns)
	if numConns == 0 {
		if dsm.config.MinConns > 0 {
			btopt.Debugf(dsm.pool.logger, "bigtable_connpool: WARNING: Pool empty, attempting to scale up to MinConns\n")
			if dsm.pool.addConnections(dsm.config.MinConns) {
				dsm.lastScalingTime = time.Now()
			}
		}
		return
	}

	var totalWeightedLoad int32
	for _, entry := range conns {
		totalWeightedLoad += entry.calculateWeightedLoad()
	}
	avgLoad := totalWeightedLoad / int32(numConns)

	targetLoad := (dsm.config.AvgLoadLowThreshold + dsm.config.AvgLoadHighThreshold) / 2
	if targetLoad == 0 {
		targetLoad = 1
	} // Avoid division by zero

	if avgLoad >= dsm.config.AvgLoadHighThreshold && numConns < dsm.config.MaxConns {
		// Scale Up
		desiredConns := int(math.Ceil(float64(totalWeightedLoad) / float64(targetLoad)))
		addCount := desiredConns - numConns
		if addCount < 1 {
			addCount = 1 // Add at least one
		}
		if numConns+addCount > dsm.config.MaxConns {
			addCount = dsm.config.MaxConns - numConns
		}

		if addCount > 0 {
			btopt.Debugf(dsm.pool.logger, "bigtable_connpool: Scaling up: AvgLoad=%d, CurrentSize=%d, Adding=%d\n", avgLoad, numConns, addCount)
			if dsm.pool.addConnections(addCount) {
				dsm.lastScalingTime = time.Now()
			}
		}
	} else if avgLoad <= dsm.config.AvgLoadLowThreshold && numConns > dsm.config.MinConns {
		// Scale Down
		desiredConns := int(math.Ceil(float64(totalWeightedLoad) / float64(targetLoad)))
		if desiredConns < dsm.config.MinConns {
			desiredConns = dsm.config.MinConns
		}
		removeCount := numConns - desiredConns
		if removeCount < 1 && numConns > dsm.config.MinConns {
			removeCount = 1 // Try to remove at least one if needed.
		}

		if removeCount > dsm.config.MaxRemoveConns {
			removeCount = dsm.config.MaxRemoveConns
		}

		if numConns-removeCount < dsm.config.MinConns {
			removeCount = numConns - dsm.config.MinConns
		}

		if removeCount > 0 {
			btopt.Debugf(dsm.pool.logger, "bigtable_connpool: Scaling down: AvgLoad=%d, CurrentSize=%d, Removing=%d\n", avgLoad, numConns, removeCount)
			if dsm.pool.removeConnections(removeCount) {
				dsm.lastScalingTime = time.Now()
			}
		}
	}
}

// validateDynamicConfig is a helper to centralize validation logic.
func validateDynamicConfig(config btopt.DynamicChannelPoolConfig, connPoolSize int) error {
	if config.MinConns <= 0 {
		return fmt.Errorf("bigtable_connpool: DynamicChannelPoolConfig.MinConns must be positive")
	}
	if config.MaxConns < config.MinConns {
		return fmt.Errorf("bigtable_connpool: DynamicChannelPoolConfig.MaxConns (%d) was less than MinConns (%d)", config.MaxConns, config.MinConns)
	}
	if connPoolSize < config.MinConns || connPoolSize > config.MaxConns {
		return fmt.Errorf("bigtable_connpool: initial connPoolSize (%d) must be between DynamicChannelPoolConfig.MinConns (%d) and MaxConns (%d)", connPoolSize, config.MinConns, config.MaxConns)
	}
	if config.AvgLoadLowThreshold >= config.AvgLoadHighThreshold {
		return fmt.Errorf("bigtable_connpool: DynamicChannelPoolConfig.AvgLoadLowThreshold (%d) must be less than AvgLoadHighThreshold (%d)", config.AvgLoadLowThreshold, config.AvgLoadHighThreshold)
	}
	if config.CheckInterval <= 0 {
		return fmt.Errorf("bigtable_connpool: DynamicChannelPoolConfig.CheckInterval must be positive")
	}
	if config.MinScalingInterval < 0 {
		return fmt.Errorf("bigtable_connpool: DynamicChannelPoolConfig.MinScalingInterval cannot be negative")
	}
	if config.MaxRemoveConns <= 0 {
		return fmt.Errorf("bigtable_connpool: DynamicChannelPoolConfig.MaxRemoveConns must be positive")
	}
	return nil
}

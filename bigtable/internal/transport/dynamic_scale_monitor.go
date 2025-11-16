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

// DynamicScaleMonitor manages upscale and downscale of the connection pool.
type DynamicScaleMonitor struct {
	config            btopt.DynamicChannelPoolConfig
	pool              *BigtableChannelPool
	lastScalingTime   time.Time
	mu                sync.Mutex
	ticker            *time.Ticker
	done              chan struct{}
	stopOnce          sync.Once
	targetLoadPerConn float64 // target average load

}

// NewDynamicScaleMonitor creates a new DynamicScaleMonitor.
func NewDynamicScaleMonitor(config btopt.DynamicChannelPoolConfig, pool *BigtableChannelPool) *DynamicScaleMonitor {

	targetLoadPerConn := math.Floor(config.AvgLoadLowThreshold+config.AvgLoadHighThreshold) / 2.0
	if targetLoadPerConn < 1.0 {
		targetLoadPerConn = 1.0 //  targetLoad is at least 1 per channel
	}
	return &DynamicScaleMonitor{
		config:            config,
		pool:              pool,
		done:              make(chan struct{}),
		targetLoadPerConn: targetLoadPerConn,
	}
}

// Start logic
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
	// we use mu for making sure only one evaluateAndScale runs.
	dsm.mu.Lock()
	defer dsm.mu.Unlock()

	if time.Since(dsm.lastScalingTime) < dsm.config.MinScalingInterval {
		return // lastScalingTime is populated after removeConn or addConn succeeds
	}

	conns := dsm.pool.getConns()
	numConns := len(conns)

	var loadSum int32
	for _, entry := range conns {
		loadSum += entry.calculateConnLoad()
	}
	avgLoadPerConn := float64(loadSum) / float64(numConns)

	if avgLoadPerConn >= dsm.config.AvgLoadHighThreshold {
		dsm.scaleUp(loadSum, numConns)
	} else if avgLoadPerConn <= dsm.config.AvgLoadLowThreshold {
		dsm.scaleDown(loadSum, numConns)
	}
}

// ValidateDynamicConfig is a helper to centralize validation logic.
func ValidateDynamicConfig(config btopt.DynamicChannelPoolConfig, connPoolSize int) error {
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
		return fmt.Errorf("bigtable_connpool: DynamicChannelPoolConfig.AvgLoadLowThreshold (%f) must be less than AvgLoadHighThreshold (%f)", config.AvgLoadLowThreshold, config.AvgLoadHighThreshold)
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

// scaleUp handles the logic for increasing the number of connections.
//
//	dsm.mu is already held.
func (dsm *DynamicScaleMonitor) scaleUp(loadSum int32, numConns int) {
	fmt.Println("LoadSum", loadSum, "targetLoadPerConn", dsm.targetLoadPerConn)
	desiredConns := int(math.Ceil(float64(loadSum) / dsm.targetLoadPerConn))
	fmt.Println("desiredConns: ", desiredConns, "numConns: ", numConns)
	addCount := desiredConns - numConns
	fmt.Println("addCount: ", addCount, "numConns: ", numConns, "desiredConns: ", desiredConns)
	if addCount > 0 {
		btopt.Debugf(dsm.pool.logger, "bigtable_connpool: Scaling up: CurrentSize=%d, Adding=%d, TargetLoadPerConn=%.2f\n", numConns, addCount, dsm.targetLoadPerConn)
		if dsm.pool.addConnections(addCount, dsm.config.MaxConns) {
			dsm.lastScalingTime = time.Now()
		}
	}
}

// scaleDown handles the logic for decreasing the number of connections.
//
//	dsm.mu is already held.
func (dsm *DynamicScaleMonitor) scaleDown(loadSum int32, numConns int) {
	desiredConns := int(math.Ceil(float64(loadSum) / dsm.targetLoadPerConn))
	removeCount := numConns - desiredConns
	if removeCount > 0 {
		btopt.Debugf(dsm.pool.logger, "bigtable_connpool: Scaling down: CurrentSize=%d, Removing=%d, TargetLoadPerConn=%.2f\n", numConns, removeCount, dsm.targetLoadPerConn)
		if dsm.pool.removeConnections(removeCount, dsm.config.MinConns, dsm.config.MaxRemoveConns) {
			dsm.lastScalingTime = time.Now()
		}
	}
}

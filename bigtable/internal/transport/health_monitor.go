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
	"time"

	btopt "cloud.google.com/go/bigtable/internal/option"
)

// NewChannelHealthMonitor creates a new ChannelHealthMonitor.
func NewChannelHealthMonitor(config btopt.HealthCheckConfig, pool *BigtableChannelPool) *ChannelHealthMonitor {
	return &ChannelHealthMonitor{
		config:       config,
		pool:         pool,
		done:         make(chan struct{}),
		evictionDone: make(chan struct{}, 1),
	}
}

// Start begins the periodic health checking loop. It takes functions to probe all connections
// and to evict unhealthy ones.
func (chm *ChannelHealthMonitor) Start(ctx context.Context) {
	if !chm.config.Enabled {
		return
	}
	chm.ticker = time.NewTicker(chm.config.ProbeInterval)
	go func() {
		defer chm.ticker.Stop()
		for {
			select {
			case <-chm.ticker.C:
				chm.pool.runProbes(ctx, chm.config)

				// Check if the eviction method returned true
				if chm.pool.detectAndEvictUnhealthy(chm.config, chm.AllowEviction, chm.RecordEviction) {
					// The notification logic now lives here, inside the monitor.
					select {
					case chm.evictionDone <- struct{}{}:
					default: // Don't block if the channel is full or nil
					}
				}
			case <-chm.done:
				return
			case <-ctx.Done():
				return
			}
		}
	}()
}

// Stop terminates the health checking loop.
func (chm *ChannelHealthMonitor) Stop() {
	if chm.config.Enabled {
		chm.stopOnce.Do(func() {
			close(chm.done)
		})
	}
}

// AllowEviction checks if enough time has passed since the last eviction.
func (chm *ChannelHealthMonitor) AllowEviction() bool {
	chm.evictionMu.Lock()
	defer chm.evictionMu.Unlock()
	return time.Since(chm.lastEvictionTime) >= chm.config.MinEvictionInterval
}

// RecordEviction updates the last eviction time to the current time.
func (chm *ChannelHealthMonitor) RecordEviction() {
	chm.evictionMu.Lock()
	defer chm.evictionMu.Unlock()
	chm.lastEvictionTime = time.Now()
}

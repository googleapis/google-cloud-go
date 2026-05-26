// Copyright 2026 Google LLC
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
	"math"
	"sync"

	spb "cloud.google.com/go/bigtable/apiv2/bigtablepb"
)

// PoolStats defines the snapshot statistics of a session pool.
type PoolStats struct {
	ReadyCount    int
	StartingCount int
	InUseCount    int
	PendingCount  int
}

// StatsFetcher is a function type that retrieves the current PoolStats.
type StatsFetcher func() *PoolStats

// PoolSizer calculates the optimal session pool size based on workload metrics.
type PoolSizer struct {
	mu          sync.Mutex
	fetcher     StatsFetcher
	minSessions int
	maxSessions int
	headroomPct float64 // Headroom percentage cushion (e.g., 0.10 for 10%)
}

// NewPoolSizer creates a new PoolSizer.
func NewPoolSizer(fetcher StatsFetcher, minSessions, maxSessions int, headroomPct float64) *PoolSizer {
	if headroomPct <= 0 {
		headroomPct = 0.10 // Default to 10% headroom
	}
	return &PoolSizer{
		fetcher:     fetcher,
		minSessions: minSessions,
		maxSessions: maxSessions,
		headroomPct: headroomPct,
	}
}

// UpdateConfig dynamically adjusts the sizer capacity bounds and headroom cushions at runtime.
func (s *PoolSizer) UpdateConfig(config *spb.SessionClientConfiguration_SessionPoolConfiguration) {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.minSessions = int(config.MinSessionCount)
	s.maxSessions = int(config.MaxSessionCount)
	s.headroomPct = float64(config.Headroom)
}

// GetScaleDelta evaluates the current statistics and calculates the required scaling delta
// to maintain the desired headroom cushion and satisfy pending calls.
func (s *PoolSizer) GetScaleDelta() int {
	s.mu.Lock()
	defer s.mu.Unlock()

	stats := s.fetcher()
	if stats == nil {
		return 0
	}

	// Formula: sessionsInUse = InUseCount + ceil(PendingCount / 10.0)
	effectivePending := int(math.Ceil(float64(stats.PendingCount) / 10.0))
	sessionsInUse := stats.InUseCount + effectivePending

	// Formula: desiredCapacity = clamp(sessionsInUse + ceil(sessionsInUse * headroomPct), minSessions, maxSessions)
	unboundedIdle := int(math.Ceil(float64(sessionsInUse) * s.headroomPct))
	desiredCapacity := sessionsInUse + unboundedIdle

	if desiredCapacity < s.minSessions {
		desiredCapacity = s.minSessions
	}
	if desiredCapacity > s.maxSessions {
		desiredCapacity = s.maxSessions
	}

	eventualCapacity := stats.ReadyCount + stats.StartingCount

	if desiredCapacity > eventualCapacity {
		return desiredCapacity - eventualCapacity
	}

	if desiredCapacity < stats.ReadyCount {
		return desiredCapacity - stats.ReadyCount
	}

	return 0
}

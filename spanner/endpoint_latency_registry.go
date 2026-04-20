/*
Copyright 2026 Google LLC

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package spanner

import (
	"math"
	"sync"
	"sync/atomic"
	"time"
)

const (
	endpointLatencyDefaultPenaltyValue = 1_000_000.0
	endpointLatencyRegistryShardCount  = 64
)

var (
	endpointLatencyDefaultRTT           = 10 * time.Millisecond
	endpointLatencyDefaultErrorPenalty  = 10 * time.Second
	endpointLatencyTrackerExpireAfter   = 10 * time.Minute
	endpointLatencyTrackerPruneInterval = time.Minute
	defaultEndpointLatencyRegistry      atomic.Pointer[endpointLatencyRegistry]
)

type endpointLatencyTrackerKey struct {
	operationUID uint64
	preferLeader bool
	address      string
}

type endpointLatencyTrackerEntry struct {
	tracker    *ewmaLatencyTracker
	lastAccess atomic.Int64
}

type endpointLatencyRegistryShard struct {
	mu         sync.RWMutex
	trackers   map[endpointLatencyTrackerKey]*endpointLatencyTrackerEntry
	lastPruned time.Time
}

type endpointLatencyRegistry struct {
	now           func() time.Time
	pruneInterval time.Duration
	shards        [endpointLatencyRegistryShardCount]endpointLatencyRegistryShard
}

func init() {
	defaultEndpointLatencyRegistry.Store(newEndpointLatencyRegistry(time.Now))
}

func newEndpointLatencyRegistry(now func() time.Time) *endpointLatencyRegistry {
	if now == nil {
		now = time.Now
	}
	registry := &endpointLatencyRegistry{
		now:           now,
		pruneInterval: endpointLatencyTrackerPruneInterval,
	}
	for i := range registry.shards {
		registry.shards[i].trackers = make(map[endpointLatencyTrackerKey]*endpointLatencyTrackerEntry)
	}
	return registry
}

func endpointLatencyRegistryHasScore(operationUID uint64, preferLeader bool, address string) bool {
	registry := currentEndpointLatencyRegistry()
	return registry.hasScore(operationUID, preferLeader, address)
}

func endpointLatencyRegistrySelectionCost(operationUID uint64, preferLeader bool, endpoint channelEndpoint, address string) float64 {
	registry := currentEndpointLatencyRegistry()
	return registry.selectionCost(operationUID, preferLeader, endpoint, address)
}

func endpointLatencyRegistryRecordLatency(operationUID uint64, preferLeader bool, address string, latency time.Duration) {
	registry := currentEndpointLatencyRegistry()
	registry.recordLatency(operationUID, preferLeader, address, latency)
}

func endpointLatencyRegistryRecordError(operationUID uint64, preferLeader bool, address string) {
	registry := currentEndpointLatencyRegistry()
	registry.recordError(operationUID, preferLeader, address, endpointLatencyDefaultErrorPenalty)
}

func clearEndpointLatencyRegistry() {
	current := currentEndpointLatencyRegistry()
	defaultEndpointLatencyRegistry.Store(newEndpointLatencyRegistry(current.now))
}

func currentEndpointLatencyRegistry() *endpointLatencyRegistry {
	if registry := defaultEndpointLatencyRegistry.Load(); registry != nil {
		return registry
	}
	registry := newEndpointLatencyRegistry(time.Now)
	if defaultEndpointLatencyRegistry.CompareAndSwap(nil, registry) {
		return registry
	}
	return defaultEndpointLatencyRegistry.Load()
}

func (r *endpointLatencyRegistry) hasScore(operationUID uint64, preferLeader bool, address string) bool {
	key, ok := r.trackerKey(operationUID, preferLeader, address)
	if !ok {
		return false
	}
	shard := r.shardForKey(key)
	shard.mu.RLock()
	entry, ok := shard.trackers[key]
	shard.mu.RUnlock()
	if ok {
		entry.lastAccess.Store(r.now().UnixNano())
	}
	return ok
}

func (r *endpointLatencyRegistry) selectionCost(operationUID uint64, preferLeader bool, endpoint channelEndpoint, address string) float64 {
	key, ok := r.trackerKey(operationUID, preferLeader, address)
	if !ok {
		return math.MaxFloat64
	}

	activeRequests := 0.0
	if endpoint != nil {
		activeRequests = float64(endpoint.ActiveRequestCount())
	}

	shard := r.shardForKey(key)
	shard.mu.RLock()
	entry, ok := shard.trackers[key]
	shard.mu.RUnlock()
	if ok {
		entry.lastAccess.Store(r.now().UnixNano())
		tracker := entry.tracker
		return tracker.scoreValue() * (activeRequests + 1)
	}
	if activeRequests > 0 {
		return endpointLatencyDefaultPenaltyValue + activeRequests
	}
	return (float64(endpointLatencyDefaultRTT) / 1e3) * (activeRequests + 1)
}

func (r *endpointLatencyRegistry) recordLatency(operationUID uint64, preferLeader bool, address string, latency time.Duration) {
	key, ok := r.trackerKey(operationUID, preferLeader, address)
	if !ok {
		return
	}
	entry := r.getOrCreateTracker(key)
	entry.tracker.update(latency)
}

func (r *endpointLatencyRegistry) recordError(operationUID uint64, preferLeader bool, address string, penalty time.Duration) {
	key, ok := r.trackerKey(operationUID, preferLeader, address)
	if !ok {
		return
	}
	entry := r.getOrCreateTracker(key)
	entry.tracker.recordError(penalty)
}

func (r *endpointLatencyRegistry) getOrCreateTracker(key endpointLatencyTrackerKey) *endpointLatencyTrackerEntry {
	now := r.now()
	shard := r.shardForKey(key)
	shard.mu.Lock()
	defer shard.mu.Unlock()
	shard.maybePruneLocked(now, r.pruneInterval)
	if entry, ok := shard.trackers[key]; ok {
		entry.lastAccess.Store(now.UnixNano())
		return entry
	}
	entry := &endpointLatencyTrackerEntry{
		tracker: newEWMALatencyTrackerWithOptions(defaultEWMADecayTime, r.now),
	}
	entry.lastAccess.Store(now.UnixNano())
	shard.trackers[key] = entry
	return entry
}

func (r *endpointLatencyRegistry) shardForKey(key endpointLatencyTrackerKey) *endpointLatencyRegistryShard {
	hash := mixUint64(key.operationUID)
	if key.preferLeader {
		hash ^= 0x9e3779b97f4a7c15
	}
	for i := 0; i < len(key.address); i++ {
		hash = mixUint64(hash ^ uint64(key.address[i]))
	}
	return &r.shards[hash&(endpointLatencyRegistryShardCount-1)]
}

func (s *endpointLatencyRegistryShard) maybePruneLocked(now time.Time, interval time.Duration) {
	if interval > 0 && !s.lastPruned.IsZero() && now.Sub(s.lastPruned) < interval {
		return
	}
	s.lastPruned = now
	for key, entry := range s.trackers {
		lastAccess := time.Unix(0, entry.lastAccess.Load())
		if now.Sub(lastAccess) > endpointLatencyTrackerExpireAfter {
			delete(s.trackers, key)
		}
	}
}

func (r *endpointLatencyRegistry) trackerKey(operationUID uint64, preferLeader bool, address string) (endpointLatencyTrackerKey, bool) {
	if operationUID == 0 || address == "" {
		return endpointLatencyTrackerKey{}, false
	}
	return endpointLatencyTrackerKey{
		operationUID: operationUID,
		preferLeader: preferLeader,
		address:      address,
	}, true
}

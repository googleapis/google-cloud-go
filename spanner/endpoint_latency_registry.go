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

const endpointLatencyDefaultPenaltyValue = 1_000_000.0

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
	lastAccess time.Time
}

type endpointLatencyRegistry struct {
	mu         sync.Mutex
	now        func() time.Time
	lastPruned time.Time
	trackers   map[endpointLatencyTrackerKey]*endpointLatencyTrackerEntry
}

func init() {
	defaultEndpointLatencyRegistry.Store(newEndpointLatencyRegistry(time.Now))
}

func newEndpointLatencyRegistry(now func() time.Time) *endpointLatencyRegistry {
	if now == nil {
		now = time.Now
	}
	return &endpointLatencyRegistry{
		now:      now,
		trackers: make(map[endpointLatencyTrackerKey]*endpointLatencyTrackerEntry),
	}
}

func endpointLatencyRegistrySelectionCost(operationUID uint64, preferLeader bool, endpoint channelEndpoint, address string) float64 {
	return currentEndpointLatencyRegistry().selectionCost(operationUID, preferLeader, endpoint, address)
}

func endpointLatencyRegistryRecordLatency(operationUID uint64, preferLeader bool, address string, latency time.Duration) {
	currentEndpointLatencyRegistry().recordLatency(operationUID, preferLeader, address, latency)
}

func endpointLatencyRegistryRecordError(operationUID uint64, preferLeader bool, address string) {
	currentEndpointLatencyRegistry().recordError(operationUID, preferLeader, address, endpointLatencyDefaultErrorPenalty)
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
	now := r.now()
	r.mu.Lock()
	r.maybePruneLocked(now)
	entry := r.trackers[key]
	if entry != nil {
		entry.lastAccess = now
	}
	r.mu.Unlock()
	return entry != nil && entry.tracker.hasScore()
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

	now := r.now()
	r.mu.Lock()
	r.maybePruneLocked(now)
	entry := r.trackers[key]
	if entry != nil {
		entry.lastAccess = now
	}
	r.mu.Unlock()

	if entry != nil {
		return entry.tracker.scoreValue() * (activeRequests + 1)
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
	now := r.now()
	entry := r.getOrCreateTracker(key, now)
	entry.tracker.update(latency)
}

func (r *endpointLatencyRegistry) recordError(operationUID uint64, preferLeader bool, address string, penalty time.Duration) {
	key, ok := r.trackerKey(operationUID, preferLeader, address)
	if !ok {
		return
	}
	now := r.now()
	entry := r.getOrCreateTracker(key, now)
	entry.tracker.recordError(penalty)
}

func (r *endpointLatencyRegistry) getOrCreateTracker(key endpointLatencyTrackerKey, now time.Time) *endpointLatencyTrackerEntry {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.maybePruneLocked(now)
	if entry := r.trackers[key]; entry != nil {
		entry.lastAccess = now
		return entry
	}

	entry := &endpointLatencyTrackerEntry{
		tracker:    newEWMALatencyTrackerWithOptions(defaultEWMADecayTime, r.now),
		lastAccess: now,
	}
	r.trackers[key] = entry
	return entry
}

func (r *endpointLatencyRegistry) maybePruneLocked(now time.Time) {
	if endpointLatencyTrackerPruneInterval > 0 && !r.lastPruned.IsZero() && now.Sub(r.lastPruned) < endpointLatencyTrackerPruneInterval {
		return
	}
	r.lastPruned = now
	for key, entry := range r.trackers {
		if now.Sub(entry.lastAccess) > endpointLatencyTrackerExpireAfter {
			delete(r.trackers, key)
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

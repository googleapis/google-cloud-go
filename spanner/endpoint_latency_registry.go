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
	"sort"
	"sync"
	"sync/atomic"
	"time"
)

const (
	endpointLatencyDefaultPenaltyValue = 1_000_000.0
	endpointLatencyMaxTrackers         = 100_000
	endpointLatencyCleanupInterval     = time.Minute
)

var (
	endpointLatencyDefaultRTT          = 10 * time.Millisecond
	endpointLatencyDefaultErrorPenalty = 10 * time.Second
	endpointLatencyTrackerExpireAfter  = 10 * time.Minute
	defaultEndpointLatencyRegistry     atomic.Pointer[endpointLatencyRegistry]
)

type endpointLatencyTrackerKey struct {
	operationUID uint64
	preferLeader bool
	address      string
}

type endpointLatencyTrackerEntry struct {
	key             endpointLatencyTrackerKey
	tracker         *ewmaLatencyTracker
	lastAccessNanos atomic.Int64
}

type endpointLatencyRegistry struct {
	mu               sync.RWMutex
	now              func() time.Time
	maxTrackers      int
	expireAfter      time.Duration
	cleanupInterval  time.Duration
	trackers         map[endpointLatencyTrackerKey]*endpointLatencyTrackerEntry
	lastCleanupNanos atomic.Int64
}

func init() {
	defaultEndpointLatencyRegistry.Store(newEndpointLatencyRegistry(time.Now))
}

func newEndpointLatencyRegistry(now func() time.Time) *endpointLatencyRegistry {
	if now == nil {
		now = time.Now
	}
	registry := &endpointLatencyRegistry{
		now:             now,
		maxTrackers:     endpointLatencyMaxTrackers,
		expireAfter:     endpointLatencyTrackerExpireAfter,
		cleanupInterval: endpointLatencyCleanupInterval,
		trackers:        make(map[endpointLatencyTrackerKey]*endpointLatencyTrackerEntry),
	}
	return registry
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
	replacement := newEndpointLatencyRegistry(current.now)
	defaultEndpointLatencyRegistry.Store(replacement)
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
	entry := r.lookupTracker(key, r.now(), true)
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

	entry := r.lookupTracker(key, r.now(), true)
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
	entry := r.getOrCreateTracker(key, r.now())
	entry.tracker.update(latency)
}

func (r *endpointLatencyRegistry) recordError(operationUID uint64, preferLeader bool, address string, penalty time.Duration) {
	key, ok := r.trackerKey(operationUID, preferLeader, address)
	if !ok {
		return
	}
	entry := r.getOrCreateTracker(key, r.now())
	entry.tracker.recordError(penalty)
}

func (r *endpointLatencyRegistry) lookupTracker(key endpointLatencyTrackerKey, now time.Time, refresh bool) *endpointLatencyTrackerEntry {
	r.maybeCleanup(now)

	r.mu.RLock()
	entry := r.trackers[key]
	r.mu.RUnlock()
	if entry == nil {
		return nil
	}
	if r.isExpiredEntry(entry, now) {
		return nil
	}
	if refresh {
		entry.lastAccessNanos.Store(now.UnixNano())
	}
	return entry
}

func (r *endpointLatencyRegistry) getOrCreateTracker(key endpointLatencyTrackerKey, now time.Time) *endpointLatencyTrackerEntry {
	if entry := r.lookupTracker(key, now, true); entry != nil {
		return entry
	}

	r.mu.Lock()

	if entry := r.trackers[key]; entry != nil {
		if r.isExpiredEntry(entry, now) {
			delete(r.trackers, key)
		} else {
			entry.lastAccessNanos.Store(now.UnixNano())
			r.mu.Unlock()
			return entry
		}
	}

	entry := &endpointLatencyTrackerEntry{
		key:     key,
		tracker: newEWMALatencyTrackerWithOptions(defaultEWMADecayTime, r.now),
	}
	entry.lastAccessNanos.Store(now.UnixNano())
	r.trackers[key] = entry
	overLimit := r.maxTrackers > 0 && len(r.trackers) > r.maxTrackers
	r.mu.Unlock()
	if overLimit {
		r.cleanup(now)
	}
	return entry
}

func (r *endpointLatencyRegistry) isExpiredEntry(entry *endpointLatencyTrackerEntry, now time.Time) bool {
	if entry == nil || r.expireAfter <= 0 {
		return false
	}
	lastAccessNanos := entry.lastAccessNanos.Load()
	if lastAccessNanos == 0 {
		return false
	}
	lastAccess := time.Unix(0, lastAccessNanos)
	return !lastAccess.Add(r.expireAfter).After(now)
}

func (r *endpointLatencyRegistry) maybeCleanup(now time.Time) {
	if r == nil {
		return
	}

	nowNanos := now.UnixNano()
	lastCleanupNanos := r.lastCleanupNanos.Load()
	if r.cleanupInterval > 0 && lastCleanupNanos != 0 && nowNanos-lastCleanupNanos < int64(r.cleanupInterval) {
		return
	}
	if !r.lastCleanupNanos.CompareAndSwap(lastCleanupNanos, nowNanos) {
		return
	}
	r.cleanup(now)
}

func (r *endpointLatencyRegistry) cleanup(now time.Time) {
	r.mu.Lock()
	defer r.mu.Unlock()

	if len(r.trackers) == 0 {
		return
	}

	entries := make([]*endpointLatencyTrackerEntry, 0, len(r.trackers))
	for key, entry := range r.trackers {
		if r.isExpiredEntry(entry, now) {
			delete(r.trackers, key)
			continue
		}
		entries = append(entries, entry)
	}

	if r.maxTrackers <= 0 || len(entries) <= r.maxTrackers {
		return
	}

	sort.Slice(entries, func(i, j int) bool {
		return entries[i].lastAccessNanos.Load() < entries[j].lastAccessNanos.Load()
	})

	excess := len(entries) - r.maxTrackers
	for i := 0; i < excess; i++ {
		delete(r.trackers, entries[i].key)
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

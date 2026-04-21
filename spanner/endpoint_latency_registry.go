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
	"container/heap"
	"math"
	"sync"
	"sync/atomic"
	"time"
)

const (
	endpointLatencyDefaultPenaltyValue = 1_000_000.0
	endpointLatencyMaxTrackers         = 100_000
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
	key       endpointLatencyTrackerKey
	tracker   *ewmaLatencyTracker
	expiresAt time.Time
	heapIndex int
}

type endpointLatencyExpiryHeap []*endpointLatencyTrackerEntry

func (h endpointLatencyExpiryHeap) Len() int { return len(h) }

func (h endpointLatencyExpiryHeap) Less(i, j int) bool {
	return h[i].expiresAt.Before(h[j].expiresAt)
}

func (h endpointLatencyExpiryHeap) Swap(i, j int) {
	h[i], h[j] = h[j], h[i]
	h[i].heapIndex = i
	h[j].heapIndex = j
}

func (h *endpointLatencyExpiryHeap) Push(x any) {
	entry := x.(*endpointLatencyTrackerEntry)
	entry.heapIndex = len(*h)
	*h = append(*h, entry)
}

func (h *endpointLatencyExpiryHeap) Pop() any {
	old := *h
	n := len(old)
	entry := old[n-1]
	entry.heapIndex = -1
	*h = old[:n-1]
	return entry
}

type endpointLatencyRegistry struct {
	mu          sync.Mutex
	now         func() time.Time
	maxTrackers int
	expireAfter time.Duration
	trackers    map[endpointLatencyTrackerKey]*endpointLatencyTrackerEntry
	expiryHeap  endpointLatencyExpiryHeap
}

func init() {
	defaultEndpointLatencyRegistry.Store(newEndpointLatencyRegistry(time.Now))
}

func newEndpointLatencyRegistry(now func() time.Time) *endpointLatencyRegistry {
	if now == nil {
		now = time.Now
	}
	registry := &endpointLatencyRegistry{
		now:         now,
		maxTrackers: endpointLatencyMaxTrackers,
		expireAfter: endpointLatencyTrackerExpireAfter,
		trackers:    make(map[endpointLatencyTrackerKey]*endpointLatencyTrackerEntry),
	}
	heap.Init(&registry.expiryHeap)
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
	r.pruneExpiredLocked(now)
	entry := r.trackers[key]
	if entry != nil {
		r.touchEntryLocked(entry, now)
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
	r.pruneExpiredLocked(now)
	entry := r.trackers[key]
	if entry != nil {
		r.touchEntryLocked(entry, now)
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

func (r *endpointLatencyRegistry) getOrCreateTracker(key endpointLatencyTrackerKey, now time.Time) *endpointLatencyTrackerEntry {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.pruneExpiredLocked(now)
	if entry := r.trackers[key]; entry != nil {
		r.touchEntryLocked(entry, now)
		return entry
	}
	if r.maxTrackers > 0 && len(r.trackers) >= r.maxTrackers {
		r.evictOldestLocked()
	}
	entry := &endpointLatencyTrackerEntry{
		key:       key,
		tracker:   newEWMALatencyTrackerWithOptions(defaultEWMADecayTime, r.now),
		expiresAt: now.Add(r.expireAfter),
		heapIndex: -1,
	}
	r.trackers[key] = entry
	heap.Push(&r.expiryHeap, entry)
	return entry
}

func (r *endpointLatencyRegistry) touchEntryLocked(entry *endpointLatencyTrackerEntry, now time.Time) {
	if entry == nil {
		return
	}
	entry.expiresAt = now.Add(r.expireAfter)
	if entry.heapIndex >= 0 {
		heap.Fix(&r.expiryHeap, entry.heapIndex)
	}
}

func (r *endpointLatencyRegistry) pruneExpiredLocked(now time.Time) {
	for r.expiryHeap.Len() > 0 {
		entry := r.expiryHeap[0]
		if entry == nil || entry.expiresAt.After(now) {
			return
		}
		heap.Pop(&r.expiryHeap)
		delete(r.trackers, entry.key)
	}
}

func (r *endpointLatencyRegistry) evictOldestLocked() {
	if r.expiryHeap.Len() == 0 {
		return
	}
	entry := heap.Pop(&r.expiryHeap).(*endpointLatencyTrackerEntry)
	delete(r.trackers, entry.key)
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

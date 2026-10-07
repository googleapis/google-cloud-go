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
	"fmt"
	"runtime"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

var endpointLatencyRegistryTestMu sync.Mutex

func withIsolatedEndpointLatencyRegistry(t *testing.T) {
	t.Helper()

	endpointLatencyRegistryTestMu.Lock()
	clearEndpointLatencyRegistry()
	t.Cleanup(func() {
		clearEndpointLatencyRegistry()
		endpointLatencyRegistryTestMu.Unlock()
	})
}

func endpointLatencyRegistryHasScore(operationUID uint64, preferLeader bool, address string) bool {
	registry := currentEndpointLatencyRegistry()
	return registry.hasScore(operationUID, preferLeader, address)
}

func TestEndpointLatencyRegistryKeysByOperationUID(t *testing.T) {
	withIsolatedEndpointLatencyRegistry(t)

	endpointLatencyRegistryRecordLatency(7, false, "server-a:443", 25*time.Millisecond)

	if !endpointLatencyRegistryHasScore(7, false, "server-a:443") {
		t.Fatal("expected score for recorded operation/address")
	}
	if endpointLatencyRegistryHasScore(8, false, "server-a:443") {
		t.Fatal("expected different operation UID to have no score")
	}
	if endpointLatencyRegistryHasScore(7, true, "server-a:443") {
		t.Fatal("expected preferLeader to remain part of the key")
	}
}

func TestEndpointLatencyRegistryLookupRefreshesAccess(t *testing.T) {
	now := time.Unix(1_000, 0)
	registry := newEndpointLatencyRegistry(func() time.Time { return now })

	registry.recordLatency(7, false, "server-a:443", 25*time.Millisecond)
	if !registry.hasScore(7, false, "server-a:443") {
		t.Fatal("expected score after initial write")
	}

	key, ok := registry.trackerKey(7, false, "server-a:443")
	if !ok {
		t.Fatal("expected valid tracker key")
	}
	registry.mu.RLock()
	entry := registry.trackers[key]
	registry.mu.RUnlock()
	if entry == nil {
		t.Fatal("expected tracker entry to exist")
	}
	lastAccess := entry.lastAccessNanos.Load()

	now = now.Add(time.Minute)

	if !registry.hasScore(7, false, "server-a:443") {
		t.Fatal("expected score to remain present during lookup")
	}
	touchedAfterHasScore := entry.lastAccessNanos.Load()
	if touchedAfterHasScore <= lastAccess {
		t.Fatal("expected hasScore lookup to refresh last access")
	}

	now = now.Add(time.Second)
	if cost := registry.selectionCost(7, false, nil, "server-a:443"); cost == 0 {
		t.Fatal("expected non-zero selection cost during lookup")
	}
	touchedAfterSelection := entry.lastAccessNanos.Load()
	if touchedAfterSelection <= touchedAfterHasScore {
		t.Fatal("expected selection lookup to refresh last access")
	}
}

func TestEndpointLatencyRegistryExpiredEntryIsCleanedUpOnAccess(t *testing.T) {
	now := time.Unix(1_500, 0)
	registry := newEndpointLatencyRegistry(func() time.Time { return now })
	registry.expireAfter = time.Minute

	registry.recordLatency(7, false, "server-a:443", 25*time.Millisecond)
	now = now.Add(2 * time.Minute)

	if registry.hasScore(7, false, "server-a:443") {
		t.Fatal("expected expired entry to be hidden")
	}
	if got := registry.selectionCost(7, false, nil, "server-a:443"); got == 0 {
		t.Fatal("expected fallback selection cost after expiry")
	}

	registry.mu.RLock()
	_, ok := registry.trackers[endpointLatencyTrackerKey{
		operationUID: 7,
		preferLeader: false,
		address:      "server-a:443",
	}]
	registry.mu.RUnlock()
	if ok {
		t.Fatal("expected access-driven cleanup to remove expired entry")
	}
}

func TestEndpointLatencyRegistryCleanupEvictsLeastRecentlyAccessedWhenBounded(t *testing.T) {
	now := time.Unix(2_000, 0)
	registry := newEndpointLatencyRegistry(func() time.Time { return now })
	registry.maxTrackers = 2
	registry.expireAfter = 10 * time.Minute

	registry.recordLatency(1, false, "server-a:443", 10*time.Millisecond)
	now = now.Add(time.Second)
	registry.recordLatency(2, false, "server-b:443", 10*time.Millisecond)

	now = now.Add(time.Second)
	if !registry.hasScore(1, false, "server-a:443") {
		t.Fatal("expected tracker 1 to exist before refresh")
	}

	now = now.Add(time.Second)
	registry.recordLatency(3, false, "server-c:443", 10*time.Millisecond)
	registry.cleanup(now)

	if !registry.hasScore(1, false, "server-a:443") {
		t.Fatal("expected refreshed tracker 1 to remain present")
	}
	if registry.hasScore(2, false, "server-b:443") {
		t.Fatal("expected least recently accessed tracker 2 to be evicted")
	}
	if !registry.hasScore(3, false, "server-c:443") {
		t.Fatal("expected newly added tracker 3 to exist")
	}
}

func TestEndpointLatencyRegistryRateLimitsCleanupWhenOverLimit(t *testing.T) {
	now := time.Unix(3_000, 0)
	registry := newEndpointLatencyRegistry(func() time.Time { return now })
	registry.maxTrackers = 2

	registry.recordLatency(1, false, "server-a:443", 10*time.Millisecond)
	lastCleanupNanos := registry.lastCleanupNanos.Load()
	registry.recordLatency(2, false, "server-b:443", 10*time.Millisecond)
	registry.recordLatency(3, false, "server-c:443", 10*time.Millisecond)

	registry.mu.RLock()
	trackerCount := len(registry.trackers)
	registry.mu.RUnlock()
	if trackerCount != registry.maxTrackers {
		t.Fatalf("tracker count after over-cap insert = %d, want %d", trackerCount, registry.maxTrackers)
	}
	if got := registry.lastCleanupNanos.Load(); got != lastCleanupNanos {
		t.Fatalf("last cleanup timestamp after sampled eviction = %d, want %d", got, lastCleanupNanos)
	}
}

func TestEndpointLatencyRegistryConcurrentAccessDuringEviction(t *testing.T) {
	const (
		hotTrackerCount  = 10
		coldTrackerCount = 90
		coldInsertCount  = 200
	)

	var currentTime atomic.Int64
	currentTime.Store(int64(time.Hour))
	nowFunc := func() time.Time {
		return time.Unix(0, currentTime.Load())
	}
	registry := newEndpointLatencyRegistry(nowFunc)
	registry.expireAfter = 24 * time.Hour
	registry.maxTrackers = hotTrackerCount + coldTrackerCount

	coldNow := time.Unix(0, 1)
	for i := 0; i < coldTrackerCount; i++ {
		key := endpointLatencyTrackerKey{
			operationUID: uint64(1_000 + i),
			address:      fmt.Sprintf("cold-%d:443", i),
		}
		registry.getOrCreateTracker(key, coldNow)
	}
	hotEntries := make([]*endpointLatencyTrackerEntry, hotTrackerCount)
	for i := 0; i < hotTrackerCount; i++ {
		key := endpointLatencyTrackerKey{
			operationUID: uint64(i + 1),
			address:      fmt.Sprintf("hot-%d:443", i),
		}
		registry.recordLatency(key.operationUID, false, key.address, time.Millisecond)
		registry.mu.RLock()
		hotEntries[i] = registry.trackers[key]
		registry.mu.RUnlock()
	}

	stop := make(chan struct{})
	var ready sync.WaitGroup
	var workers sync.WaitGroup
	ready.Add(hotTrackerCount)
	workers.Add(hotTrackerCount)
	for i := 0; i < hotTrackerCount; i++ {
		operationUID := uint64(i + 1)
		address := fmt.Sprintf("hot-%d:443", i)
		go func() {
			defer workers.Done()
			currentTime.Add(int64(time.Nanosecond))
			registry.hasScore(operationUID, false, address)
			registry.recordLatency(operationUID, false, address, time.Millisecond)
			ready.Done()
			for {
				select {
				case <-stop:
					return
				default:
					currentTime.Add(int64(time.Nanosecond))
					registry.hasScore(operationUID, false, address)
					registry.recordLatency(operationUID, false, address, time.Millisecond)
				}
			}
		}()
	}
	ready.Wait()

	for i := 0; i < coldInsertCount; i++ {
		key := endpointLatencyTrackerKey{
			operationUID: uint64(2_000 + i),
			address:      fmt.Sprintf("cold-insert-%d:443", i),
		}
		registry.getOrCreateTracker(key, coldNow)
		runtime.Gosched()
	}
	close(stop)
	workers.Wait()

	hotSurvivors := 0
	for i := 0; i < hotTrackerCount; i++ {
		key := endpointLatencyTrackerKey{
			operationUID: uint64(i + 1),
			address:      fmt.Sprintf("hot-%d:443", i),
		}
		registry.mu.RLock()
		entry := registry.trackers[key]
		registry.mu.RUnlock()
		if entry == hotEntries[i] && registry.hasScore(key.operationUID, false, key.address) {
			hotSurvivors++
		}
	}
	// With 90 cold entries and a sample of 8, two hot evictions among 200
	// insertions have probability below 1e-15. Allowing one avoids a flaky
	// assertion while still detecting eviction that ignores concurrent access.
	if hotSurvivors < hotTrackerCount-1 {
		t.Fatalf("hot tracker survivors = %d, want at least %d", hotSurvivors, hotTrackerCount-1)
	}
}

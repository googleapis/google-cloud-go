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
	"sync"
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
	defer registry.close()

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

func TestEndpointLatencyRegistryExpiredEntryIsHiddenBeforeCleanup(t *testing.T) {
	now := time.Unix(1_500, 0)
	registry := newEndpointLatencyRegistry(func() time.Time { return now })
	defer registry.close()
	registry.expireAfter = time.Minute

	registry.recordLatency(7, false, "server-a:443", 25*time.Millisecond)
	now = now.Add(2 * time.Minute)

	if registry.hasScore(7, false, "server-a:443") {
		t.Fatal("expected expired entry to be hidden before janitor cleanup")
	}
	if got := registry.selectionCost(7, false, nil, "server-a:443"); got == 0 {
		t.Fatal("expected fallback selection cost after expiry")
	}

	registry.cleanup(now)
	registry.mu.RLock()
	_, ok := registry.trackers[endpointLatencyTrackerKey{
		operationUID: 7,
		preferLeader: false,
		address:      "server-a:443",
	}]
	registry.mu.RUnlock()
	if ok {
		t.Fatal("expected cleanup to remove expired entry")
	}
}

func TestEndpointLatencyRegistryCleanupEvictsLeastRecentlyAccessedWhenBounded(t *testing.T) {
	now := time.Unix(2_000, 0)
	registry := newEndpointLatencyRegistry(func() time.Time { return now })
	defer registry.close()
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

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
	"testing"
	"time"
)

func endpointLatencyRegistryHasScore(operationUID uint64, preferLeader bool, address string) bool {
	registry := currentEndpointLatencyRegistry()
	return registry.hasScore(operationUID, preferLeader, address)
}

func TestEndpointLatencyRegistryKeysByOperationUID(t *testing.T) {
	clearEndpointLatencyRegistry()
	defer clearEndpointLatencyRegistry()

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
	registry.mu.Lock()
	entry := registry.trackers[key]
	registry.mu.Unlock()
	if entry == nil {
		t.Fatal("expected tracker entry to exist")
	}
	lastExpiry := entry.expiresAt

	now = now.Add(time.Minute)

	if !registry.hasScore(7, false, "server-a:443") {
		t.Fatal("expected score to remain present during lookup")
	}
	registry.mu.Lock()
	touchedAfterHasScore := entry.expiresAt
	registry.mu.Unlock()
	if !touchedAfterHasScore.After(lastExpiry) {
		t.Fatal("expected hasScore lookup to refresh expiry")
	}
	now = now.Add(time.Second)
	if cost := registry.selectionCost(7, false, nil, "server-a:443"); cost == 0 {
		t.Fatal("expected non-zero selection cost during lookup")
	}
	registry.mu.Lock()
	touchedAfterSelection := entry.expiresAt
	registry.mu.Unlock()
	if !touchedAfterSelection.After(touchedAfterHasScore) {
		t.Fatal("expected selection lookup to refresh expiry")
	}
}

func TestEndpointLatencyRegistryEvictsOldestTrackerWhenBounded(t *testing.T) {
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

	if !registry.hasScore(1, false, "server-a:443") {
		t.Fatal("expected refreshed tracker 1 to remain present")
	}
	if registry.hasScore(2, false, "server-b:443") {
		t.Fatal("expected oldest unrefreshed tracker 2 to be evicted")
	}
	if !registry.hasScore(3, false, "server-c:443") {
		t.Fatal("expected newly added tracker 3 to exist")
	}
}

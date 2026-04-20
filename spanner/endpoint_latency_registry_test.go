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

func TestEndpointLatencyRegistryLookupDoesNotRefreshExpiry(t *testing.T) {
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
	shard := registry.shardForKey(key)
	shard.mu.RLock()
	entry := shard.trackers[key]
	shard.mu.RUnlock()
	if entry == nil {
		t.Fatal("expected tracker entry to exist")
	}
	lastUpdated := entry.lastUpdatedNanos.Load()

	now = now.Add(time.Minute)

	// Reads alone should not extend the tracker's lifetime.
	if cost := registry.selectionCost(7, false, nil, "server-a:443"); cost == 0 {
		t.Fatal("expected non-zero selection cost before pruning")
	}
	if updated := entry.lastUpdatedNanos.Load(); updated != lastUpdated {
		t.Fatal("expected selectionCost lookup to leave lastUpdated unchanged")
	}
	if !registry.hasScore(7, false, "server-a:443") {
		t.Fatal("expected score to remain present after read-only lookup")
	}
	if updated := entry.lastUpdatedNanos.Load(); updated != lastUpdated {
		t.Fatal("expected hasScore lookup to leave lastUpdated unchanged")
	}
}

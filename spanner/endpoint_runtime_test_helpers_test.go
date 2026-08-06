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

func withIsolatedEndpointLatencyRegistry(t *testing.T) {
	t.Helper()
	clearEndpointLatencyRegistry()
	t.Cleanup(clearEndpointLatencyRegistry)
}

func clearEndpointLatencyRegistry() {
	endpointCacheRegistry.Range(func(key, _ any) bool {
		cache, ok := key.(*endpointClientCache)
		if ok && cache != nil {
			cache.clearLatencyScores()
		}
		return true
	})
}

func endpointLatencyRegistryHasScore(operationUID uint64, preferLeader bool, address string) bool {
	found := false
	endpointCacheRegistry.Range(func(key, _ any) bool {
		cache, ok := key.(*endpointClientCache)
		if ok && cache != nil && cache.hasScore(operationUID, preferLeader, address) {
			found = true
			return false
		}
		return true
	})
	return found
}

func setEndpointRoutingConfigForTest(t *testing.T, endpointCache channelEndpointCache, now func() time.Time) {
	t.Helper()
	cfg := defaultEndpointRoutingConfig()
	cfg.now = now
	cfg.initialCooldown = time.Minute
	cfg.maxCooldown = time.Minute
	cfg.resetAfter = 10 * time.Minute
	cfg.randInt63n = func(n int64) int64 { return n - 1 }
	switch cache := endpointCache.(type) {
	case *endpointClientCache:
		cache.setRoutingConfigForTest(cfg)
	case *mockEndpointCache:
		cache.setRoutingConfigForTest(cfg)
	case *testEndpointCache:
		cache.setRoutingConfigForTest(cfg)
	default:
		t.Fatalf("endpoint cache type %T does not support routing config override", endpointCache)
	}
}

func endpointRoutingStateEntries(endpointCache channelEndpointCache) map[string]endpointRoutingStateSnapshot {
	entries := make(map[string]endpointRoutingStateSnapshot)
	switch cache := endpointCache.(type) {
	case *endpointClientCache:
		cache.mu.RLock()
		addresses := make([]string, 0, len(cache.endpoints))
		for address := range cache.endpoints {
			addresses = append(addresses, address)
		}
		cache.mu.RUnlock()
		for _, address := range addresses {
			if snapshot, ok := cache.routingStateSnapshot(address); ok {
				entries[address] = snapshot
			}
		}
	case *mockEndpointCache:
		cache.mu.Lock()
		defer cache.mu.Unlock()
		for address, snapshot := range cache.routingState {
			entries[address] = snapshot
		}
	case *testEndpointCache:
		for address, snapshot := range cache.routingState {
			entries[address] = snapshot
		}
	}
	return entries
}

func testRoutingStateIsCoolingDownLocked(state map[string]endpointRoutingStateSnapshot, cfg endpointRoutingConfig, address string) bool {
	if address == "" {
		return false
	}
	cfg = cfg.normalize()
	now := cfg.now()
	snapshot, ok := state[address]
	if !ok {
		return false
	}
	if snapshot.cooldownUntil.After(now) {
		return true
	}
	if snapshot.lastFailureAt.IsZero() || now.Sub(snapshot.lastFailureAt) < cfg.resetAfter {
		return false
	}
	delete(state, address)
	return false
}

func testRoutingStateRemainingCooldownLocked(state map[string]endpointRoutingStateSnapshot, cfg endpointRoutingConfig, address string) time.Duration {
	if address == "" {
		return 0
	}
	cfg = cfg.normalize()
	now := cfg.now()
	snapshot, ok := state[address]
	if !ok || !snapshot.cooldownUntil.After(now) {
		return 0
	}
	return snapshot.cooldownUntil.Sub(now)
}

func testRoutingStateRecordFailureLocked(state map[string]endpointRoutingStateSnapshot, cfg endpointRoutingConfig, address string) {
	if address == "" {
		return
	}
	cfg = cfg.normalize()
	now := cfg.now()
	snapshot := state[address]
	if snapshot.lastFailureAt.IsZero() || now.Sub(snapshot.lastFailureAt) >= cfg.resetAfter {
		snapshot.consecutiveFailures = 0
	}
	snapshot.consecutiveFailures++
	snapshot.lastFailureAt = now
	snapshot.cooldownUntil = now.Add(endpointCooldownForFailures(cfg, snapshot.consecutiveFailures))
	state[address] = snapshot
}

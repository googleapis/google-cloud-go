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

func testEndpointRoutingConfig(clock *lifecycleTestClock, initial, max, reset time.Duration, randInt63n func(int64) int64) endpointRoutingConfig {
	cfg := defaultEndpointRoutingConfig()
	cfg.initialCooldown = initial
	cfg.maxCooldown = max
	cfg.resetAfter = reset
	cfg.now = clock.Now
	cfg.randInt63n = randInt63n
	return cfg.normalize()
}

func TestEndpointRuntimeCooldown_SuccessDoesNotClearFailureState(t *testing.T) {
	clock := newLifecycleTestClock(time.Unix(100, 0))
	cfg := testEndpointRoutingConfig(clock, time.Minute, time.Minute, 10*time.Minute, func(n int64) int64 { return n - 1 })
	endpoint := &grpcChannelEndpoint{address: "replica-a:443"}

	endpoint.recordFailure(cfg)
	if !endpoint.isCoolingDown(cfg) {
		t.Fatal("expected endpoint to be cooling down after failure")
	}

	clock.Advance(2 * time.Minute)
	if endpoint.isCoolingDown(cfg) {
		t.Fatal("expected cooldown to expire after advancing test clock")
	}

	endpoint.stateMu.Lock()
	lastFailureAt := endpoint.lastFailureAt
	endpoint.stateMu.Unlock()
	if lastFailureAt.IsZero() {
		t.Fatal("expected expired cooldown to retain failure state until reset window passes")
	}

	clock.Advance(9 * time.Minute)
	if endpoint.isCoolingDown(cfg) {
		t.Fatal("expected endpoint not to be cooling down after extended quiet period")
	}
	endpoint.stateMu.Lock()
	lastFailureAt = endpoint.lastFailureAt
	endpoint.stateMu.Unlock()
	if !lastFailureAt.IsZero() {
		t.Fatal("expected failure state to clear only after reset window passes")
	}
}

func TestEndpointRuntimeCooldown_UsesFullJitterWithinCooldownRange(t *testing.T) {
	clock := newLifecycleTestClock(time.Unix(100, 0))
	cfg := testEndpointRoutingConfig(clock, 10*time.Second, 10*time.Second, 10*time.Minute, func(n int64) int64 { return 0 })
	endpoint := &grpcChannelEndpoint{address: "replica-a:443"}

	endpoint.recordFailure(cfg)

	endpoint.stateMu.Lock()
	got := endpoint.cooldownUntil.Sub(clock.Now())
	endpoint.stateMu.Unlock()
	if got != 5*time.Second {
		t.Fatalf("cooldown = %v, want %v from deterministic jitter floor", got, 5*time.Second)
	}
}

func TestEndpointRuntimeCooldown_UsesFullJitterWhenCooldownCapsAtMax(t *testing.T) {
	clock := newLifecycleTestClock(time.Unix(100, 0))
	cfg := testEndpointRoutingConfig(clock, 5*time.Second, 60*time.Second, 10*time.Minute, func(n int64) int64 {
		if n != int64(30*time.Second)+1 {
			t.Fatalf("randInt63n called with %d, want %d", n, int64(30*time.Second)+1)
		}
		return 0
	})

	got := endpointCooldownForFailures(cfg, 5)
	if got != 30*time.Second {
		t.Fatalf("cooldown = %v, want %v from deterministic jitter floor at capped max cooldown", got, 30*time.Second)
	}
}

func TestEndpointRuntimeCooldown_ResetsPenaltyOnlyAfterQuietWindow(t *testing.T) {
	clock := newLifecycleTestClock(time.Unix(100, 0))
	cfg := testEndpointRoutingConfig(clock, time.Second, 8*time.Second, 10*time.Minute, func(n int64) int64 { return n - 1 })
	endpoint := &grpcChannelEndpoint{address: "replica-a:443"}

	endpoint.recordFailure(cfg)
	endpoint.stateMu.Lock()
	firstFailures := endpoint.consecutiveFailures
	endpoint.stateMu.Unlock()

	clock.Advance(2 * time.Minute)
	endpoint.recordFailure(cfg)
	endpoint.stateMu.Lock()
	secondFailures := endpoint.consecutiveFailures
	endpoint.stateMu.Unlock()
	if secondFailures != firstFailures+1 {
		t.Fatalf("consecutiveFailures = %d, want %d", secondFailures, firstFailures+1)
	}

	clock.Advance(11 * time.Minute)
	endpoint.recordFailure(cfg)
	endpoint.stateMu.Lock()
	thirdFailures := endpoint.consecutiveFailures
	endpoint.stateMu.Unlock()
	if thirdFailures != 1 {
		t.Fatalf("expected quiet window to reset failure count, got %d", thirdFailures)
	}
}

func TestEndpointRuntimeCooldown_PruneStaleRoutingStateClearsUntouchedExpiredEntries(t *testing.T) {
	clock := newLifecycleTestClock(time.Unix(100, 0))
	cfg := testEndpointRoutingConfig(clock, time.Minute, time.Minute, 10*time.Minute, func(n int64) int64 { return n - 1 })
	endpoint := &grpcChannelEndpoint{address: "replica-a:443"}

	endpoint.recordFailure(cfg)

	clock.Advance(15 * time.Minute)
	endpoint.pruneRoutingState(cfg, 20*time.Minute)
	endpoint.stateMu.Lock()
	lastFailureAt := endpoint.lastFailureAt
	endpoint.stateMu.Unlock()
	if lastFailureAt.IsZero() {
		t.Fatal("expected entry to be retained before background cleanup window passes")
	}

	clock.Advance(5 * time.Minute)
	endpoint.pruneRoutingState(cfg, 20*time.Minute)
	endpoint.stateMu.Lock()
	lastFailureAt = endpoint.lastFailureAt
	endpoint.stateMu.Unlock()
	if !lastFailureAt.IsZero() {
		t.Fatal("expected stale entry to be pruned after background cleanup window passes")
	}
}

func TestEndpointRuntimeCooldown_PruneStaleRoutingStateKeepsActiveCooldowns(t *testing.T) {
	clock := newLifecycleTestClock(time.Unix(100, 0))
	cfg := testEndpointRoutingConfig(clock, 30*time.Minute, 30*time.Minute, 10*time.Minute, func(n int64) int64 { return n - 1 })
	endpoint := &grpcChannelEndpoint{address: "replica-a:443"}

	endpoint.recordFailure(cfg)

	clock.Advance(25 * time.Minute)
	endpoint.pruneRoutingState(cfg, 20*time.Minute)
	endpoint.stateMu.Lock()
	lastFailureAt := endpoint.lastFailureAt
	endpoint.stateMu.Unlock()
	if lastFailureAt.IsZero() {
		t.Fatal("expected active cooldown entry to be retained during background cleanup")
	}
	if !endpoint.isCoolingDown(cfg) {
		t.Fatal("expected endpoint to remain in cooldown while cooldown window is active")
	}
}

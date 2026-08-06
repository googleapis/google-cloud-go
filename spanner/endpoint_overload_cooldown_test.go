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

func TestEndpointOverloadCooldownTracker_SuccessDoesNotClearFailureState(t *testing.T) {
	clock := newLifecycleTestClock(time.Unix(100, 0))
	tracker := newEndpointOverloadCooldownTrackerWithOptions(
		time.Minute,
		time.Minute,
		10*time.Minute,
		clock.Now,
		func(n int64) int64 {
			return n - 1
		},
	)

	tracker.recordFailure("replica-a:443")
	if !tracker.isCoolingDown("replica-a:443") {
		t.Fatal("expected endpoint to be cooling down after failure")
	}

	clock.Advance(2 * time.Minute)
	if tracker.isCoolingDown("replica-a:443") {
		t.Fatal("expected cooldown to expire after advancing test clock")
	}

	if _, ok := tracker.entries["replica-a:443"]; !ok {
		t.Fatal("expected expired cooldown to retain failure state until reset window passes")
	}

	clock.Advance(9 * time.Minute)
	if tracker.isCoolingDown("replica-a:443") {
		t.Fatal("expected endpoint not to be cooling down after extended quiet period")
	}
	if _, ok := tracker.entries["replica-a:443"]; ok {
		t.Fatal("expected failure state to clear only after the reset window passes")
	}
}

func TestEndpointOverloadCooldownTracker_UsesFullJitterWithinCooldownRange(t *testing.T) {
	clock := newLifecycleTestClock(time.Unix(100, 0))
	tracker := newEndpointOverloadCooldownTrackerWithOptions(
		10*time.Second,
		10*time.Second,
		10*time.Minute,
		clock.Now,
		func(n int64) int64 {
			return 0
		},
	)

	tracker.recordFailure("replica-a:443")

	state := tracker.entries["replica-a:443"]
	got := state.cooldownUntil.Sub(clock.Now())
	if got != 5*time.Second {
		t.Fatalf("cooldown = %v, want %v from deterministic jitter floor", got, 5*time.Second)
	}
}

func TestEndpointOverloadCooldownTracker_UsesFullJitterWhenCooldownCapsAtMax(t *testing.T) {
	clock := newLifecycleTestClock(time.Unix(100, 0))
	tracker := newEndpointOverloadCooldownTrackerWithOptions(
		5*time.Second,
		60*time.Second,
		10*time.Minute,
		clock.Now,
		func(n int64) int64 {
			if n != int64(30*time.Second)+1 {
				t.Fatalf("randInt63n called with %d, want %d", n, int64(30*time.Second)+1)
			}
			return 0
		},
	)

	got := tracker.cooldownForFailures(5)
	if got != 30*time.Second {
		t.Fatalf("cooldown = %v, want %v from deterministic jitter floor at capped max cooldown", got, 30*time.Second)
	}
}

func TestEndpointOverloadCooldownTracker_ResetsPenaltyOnlyAfterQuietWindow(t *testing.T) {
	clock := newLifecycleTestClock(time.Unix(100, 0))
	tracker := newEndpointOverloadCooldownTrackerWithOptions(
		time.Second,
		8*time.Second,
		10*time.Minute,
		clock.Now,
		func(n int64) int64 {
			return n - 1
		},
	)

	tracker.recordFailure("replica-a:443")
	first := tracker.entries["replica-a:443"]

	clock.Advance(2 * time.Minute)
	tracker.recordFailure("replica-a:443")
	second := tracker.entries["replica-a:443"]
	if second.consecutiveFailures != first.consecutiveFailures+1 {
		t.Fatalf("consecutiveFailures = %d, want %d", second.consecutiveFailures, first.consecutiveFailures+1)
	}

	clock.Advance(11 * time.Minute)
	tracker.recordFailure("replica-a:443")
	third := tracker.entries["replica-a:443"]
	if third.consecutiveFailures != 1 {
		t.Fatalf("expected quiet window to reset failure count, got %d", third.consecutiveFailures)
	}
}

func TestEndpointOverloadCooldownTracker_PruneStaleEntriesClearsUntouchedExpiredEntries(t *testing.T) {
	clock := newLifecycleTestClock(time.Unix(100, 0))
	tracker := newEndpointOverloadCooldownTrackerWithOptions(
		time.Minute,
		time.Minute,
		10*time.Minute,
		clock.Now,
		func(n int64) int64 {
			return n - 1
		},
	)

	tracker.recordFailure("replica-a:443")

	clock.Advance(15 * time.Minute)
	tracker.pruneStaleEntries(20 * time.Minute)
	if _, ok := tracker.entries["replica-a:443"]; !ok {
		t.Fatal("expected entry to be retained before background cleanup window passes")
	}

	clock.Advance(5 * time.Minute)
	tracker.pruneStaleEntries(20 * time.Minute)
	if _, ok := tracker.entries["replica-a:443"]; ok {
		t.Fatal("expected stale entry to be pruned after background cleanup window passes")
	}
}

func TestEndpointOverloadCooldownTracker_PruneStaleEntriesKeepsActiveCooldowns(t *testing.T) {
	clock := newLifecycleTestClock(time.Unix(100, 0))
	tracker := newEndpointOverloadCooldownTrackerWithOptions(
		30*time.Minute,
		30*time.Minute,
		10*time.Minute,
		clock.Now,
		func(n int64) int64 {
			return n - 1
		},
	)

	tracker.recordFailure("replica-a:443")

	clock.Advance(25 * time.Minute)
	tracker.pruneStaleEntries(20 * time.Minute)
	if _, ok := tracker.entries["replica-a:443"]; !ok {
		t.Fatal("expected active cooldown entry to be retained during background cleanup")
	}
	if !tracker.isCoolingDown("replica-a:443") {
		t.Fatal("expected endpoint to remain in cooldown while cooldown window is active")
	}
}

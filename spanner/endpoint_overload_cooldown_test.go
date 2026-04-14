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

func TestEndpointOverloadCooldownTracker_RequiresTwoSuccessesToClearState(t *testing.T) {
	clock := newLifecycleTestClock(time.Unix(100, 0))
	tracker := newEndpointOverloadCooldownTrackerWithOptions(
		time.Minute,
		time.Minute,
		10*time.Minute,
		2,
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

	tracker.recordSuccess("replica-a:443")
	if _, ok := tracker.entries["replica-a:443"]; !ok {
		t.Fatal("expected first success not to clear failure state")
	}

	tracker.recordSuccess("replica-a:443")
	if _, ok := tracker.entries["replica-a:443"]; ok {
		t.Fatal("expected second consecutive success to clear failure state")
	}
}

func TestEndpointOverloadCooldownTracker_UsesFullJitterWithinCooldownRange(t *testing.T) {
	clock := newLifecycleTestClock(time.Unix(100, 0))
	tracker := newEndpointOverloadCooldownTrackerWithOptions(
		10*time.Second,
		10*time.Second,
		10*time.Minute,
		2,
		clock.Now,
		func(n int64) int64 {
			return 0
		},
	)

	tracker.recordFailure("replica-a:443")

	state := tracker.entries["replica-a:443"]
	got := state.cooldownUntil.Sub(clock.Now())
	if got != 0 {
		t.Fatalf("cooldown = %v, want 0 from deterministic jitter hook", got)
	}
}

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

	"google.golang.org/grpc/codes"
)

func TestEndpointOverloadCooldownTracker_HintedOverloadHonorsFloorJitterAndTierCap(t *testing.T) {
	clock := newLifecycleTestClock(time.Unix(100, 0))
	tracker := newEndpointOverloadCooldownTrackerWithOptions(
		10*time.Second, time.Minute, 10*time.Minute, clock.Now,
		func(n int64) int64 { return n - 1 },
	)
	zero := time.Duration(0)
	tracker.recordFailureWithStatus("replica-a:443", codes.ResourceExhausted, &zero)
	state := tracker.entries["replica-a:443"]
	if got, want := state.overloadUntil.Sub(clock.Now()), 125*time.Millisecond; got != want {
		t.Fatalf("hinted cooldown = %v, want %v", got, want)
	}

	tracker = newEndpointOverloadCooldownTrackerWithOptions(
		10*time.Second, time.Minute, 10*time.Minute, clock.Now,
		func(int64) int64 { return 0 },
	)
	hint := 100 * time.Millisecond
	for i := 0; i < 20; i++ {
		tracker.recordFailureWithStatus("replica-a:443", codes.ResourceExhausted, &hint)
	}
	state = tracker.entries["replica-a:443"]
	if state.overloadFailures != defaultEndpointCooldownMaxTier {
		t.Fatalf("overloadFailures = %d, want %d", state.overloadFailures, defaultEndpointCooldownMaxTier)
	}
	if got, want := state.overloadUntil.Sub(clock.Now()), 2*time.Second; got != want {
		t.Fatalf("capped hinted cooldown = %v, want %v", got, want)
	}
}

func TestEndpointOverloadCooldownTracker_UnhintedOverloadKeepsLongBackoff(t *testing.T) {
	clock := newLifecycleTestClock(time.Unix(100, 0))
	tracker := newEndpointOverloadCooldownTrackerWithOptions(
		10*time.Second, time.Minute, 10*time.Minute, clock.Now,
		func(int64) int64 { return 0 },
	)
	tracker.recordFailureWithStatus("replica-a:443", codes.ResourceExhausted, nil)
	tracker.recordFailureWithStatus("replica-a:443", codes.ResourceExhausted, nil)
	if got, want := tracker.entries["replica-a:443"].overloadUntil.Sub(clock.Now()), 10*time.Second; got != want {
		t.Fatalf("unhinted cooldown = %v, want %v", got, want)
	}
}

func TestEndpointOverloadCooldownTracker_IndependentFailureLanes(t *testing.T) {
	clock := newLifecycleTestClock(time.Unix(100, 0))
	tracker := newEndpointOverloadCooldownTrackerWithOptions(
		10*time.Second, time.Minute, 10*time.Minute, clock.Now,
		func(int64) int64 { return 0 },
	)
	hint := 100 * time.Millisecond
	tracker.recordFailureWithStatus("replica-a:443", codes.ResourceExhausted, &hint)
	ignoredHint := time.Hour
	tracker.recordFailureWithStatus("replica-a:443", codes.Unavailable, &ignoredHint)
	state := tracker.entries["replica-a:443"]
	if state.overloadFailures != 1 || state.unavailableFailures != 1 {
		t.Fatalf("failure lanes = (%d, %d), want (1, 1)", state.overloadFailures, state.unavailableFailures)
	}
	clock.Advance(100 * time.Millisecond)
	if !tracker.isCoolingDown("replica-a:443") {
		t.Fatal("unavailable lane ended with hinted overload lane")
	}
	clock.Advance(4900 * time.Millisecond)
	if tracker.isCoolingDown("replica-a:443") {
		t.Fatal("unavailable cooldown did not use independent unhinted backoff")
	}
}

func TestEndpointOverloadCooldownTracker_ThreeSuccessesRepairBothLanes(t *testing.T) {
	clock := newLifecycleTestClock(time.Unix(100, 0))
	tracker := newEndpointOverloadCooldownTrackerWithOptions(
		10*time.Second, time.Minute, 10*time.Minute, clock.Now,
		func(int64) int64 { return 0 },
	)
	hint := 100 * time.Millisecond
	tracker.recordFailureWithStatus("replica-a:443", codes.ResourceExhausted, &hint)
	tracker.recordFailureWithStatus("replica-a:443", codes.ResourceExhausted, &hint)
	tracker.recordFailureWithStatus("replica-a:443", codes.Unavailable, nil)
	before := tracker.entries["replica-a:443"]
	tracker.recordSuccess("replica-a:443")
	tracker.recordSuccess("replica-a:443")
	tracker.recordSuccess("replica-a:443")
	after := tracker.entries["replica-a:443"]
	if after.overloadFailures != 1 || after.unavailableFailures != 0 || after.successesTowardRepair != 0 {
		t.Fatalf("repaired state = %+v", after)
	}
	if !after.overloadUntil.Equal(before.overloadUntil) || !after.unavailableUntil.Equal(before.unavailableUntil) {
		t.Fatal("repair shortened active cooldown deadline")
	}
}

func TestEndpointOverloadCooldownTracker_RepairDeletesZeroTierExpiredEntry(t *testing.T) {
	clock := newLifecycleTestClock(time.Unix(100, 0))
	tracker := newEndpointOverloadCooldownTrackerWithOptions(
		10*time.Second, time.Minute, 10*time.Minute, clock.Now,
		func(int64) int64 { return 0 },
	)
	hint := 100 * time.Millisecond
	tracker.recordFailureWithStatus("replica-a:443", codes.ResourceExhausted, &hint)
	clock.Advance(hint)
	for i := 0; i < defaultEndpointCooldownSuccessesToRepair; i++ {
		tracker.recordSuccess("replica-a:443")
	}
	if _, ok := tracker.entries["replica-a:443"]; ok {
		t.Fatal("zero-tier expired entry was retained")
	}
	assertEndpointCooldownEntryCount(t, tracker, 0)
}

func TestEndpointOverloadCooldownTracker_EntryCountTracksMapMutations(t *testing.T) {
	newTracker := func(clock *lifecycleTestClock) *endpointOverloadCooldownTracker {
		return newEndpointOverloadCooldownTrackerWithOptions(
			time.Second, time.Second, 10*time.Minute, clock.Now,
			func(int64) int64 { return 0 },
		)
	}

	t.Run("insert and repair delete", func(t *testing.T) {
		clock := newLifecycleTestClock(time.Unix(100, 0))
		tracker := newTracker(clock)
		assertEndpointCooldownEntryCount(t, tracker, 0)
		hint := 100 * time.Millisecond
		tracker.recordFailureWithStatus("replica-a:443", codes.ResourceExhausted, &hint)
		tracker.recordFailureWithStatus("replica-a:443", codes.ResourceExhausted, &hint)
		tracker.recordFailureWithStatus("replica-b:443", codes.ResourceExhausted, &hint)
		assertEndpointCooldownEntryCount(t, tracker, 2)

		clock.Advance(2 * hint)
		for i := 0; i < 2*defaultEndpointCooldownSuccessesToRepair; i++ {
			tracker.recordSuccess("replica-a:443")
		}
		assertEndpointCooldownEntryCount(t, tracker, 1)
	})

	t.Run("idle delete", func(t *testing.T) {
		clock := newLifecycleTestClock(time.Unix(100, 0))
		tracker := newTracker(clock)
		tracker.recordFailure("replica-a:443")
		assertEndpointCooldownEntryCount(t, tracker, 1)
		clock.Advance(10 * time.Minute)
		tracker.isCoolingDown("replica-a:443")
		assertEndpointCooldownEntryCount(t, tracker, 0)
	})

	t.Run("prune delete", func(t *testing.T) {
		clock := newLifecycleTestClock(time.Unix(100, 0))
		tracker := newTracker(clock)
		tracker.recordFailure("replica-a:443")
		tracker.recordFailure("replica-b:443")
		assertEndpointCooldownEntryCount(t, tracker, 2)
		clock.Advance(20 * time.Minute)
		tracker.pruneStaleEntries(20 * time.Minute)
		assertEndpointCooldownEntryCount(t, tracker, 0)
	})
}

func TestEndpointOverloadCooldownTracker_EmptyRecordSuccessSkipsClock(t *testing.T) {
	tracker := newEndpointOverloadCooldownTrackerWithOptions(
		time.Second, time.Second, 10*time.Minute,
		func() time.Time { t.Fatal("recordSuccess called clock for empty tracker"); return time.Time{} },
		func(int64) int64 { return 0 },
	)

	tracker.recordSuccess("replica-a:443")
	assertEndpointCooldownEntryCount(t, tracker, 0)
}

func assertEndpointCooldownEntryCount(t *testing.T, tracker *endpointOverloadCooldownTracker, want int64) {
	t.Helper()
	if got := tracker.entryCount.Load(); got != want {
		t.Fatalf("entryCount = %d, want %d", got, want)
	}
	if got := int64(len(tracker.entries)); got != want {
		t.Fatalf("len(entries) = %d, want %d", got, want)
	}
}

func TestEndpointOverloadCooldownTracker_FailureResetsRepairAndIdleResetsLanes(t *testing.T) {
	clock := newLifecycleTestClock(time.Unix(100, 0))
	tracker := newEndpointOverloadCooldownTrackerWithOptions(
		10*time.Second, time.Minute, 10*time.Minute, clock.Now,
		func(int64) int64 { return 0 },
	)
	hint := 100 * time.Millisecond
	tracker.recordFailureWithStatus("replica-a:443", codes.ResourceExhausted, &hint)
	tracker.recordSuccess("replica-a:443")
	tracker.recordSuccess("replica-a:443")
	tracker.recordFailureWithStatus("replica-a:443", codes.Unavailable, nil)
	if got := tracker.entries["replica-a:443"].successesTowardRepair; got != 0 {
		t.Fatalf("successesTowardRepair = %d, want 0", got)
	}

	tracker.recordFailureWithStatus("replica-a:443", codes.ResourceExhausted, &hint)
	tracker.recordFailureWithStatus("replica-a:443", codes.Unavailable, nil)
	clock.Advance(10 * time.Minute)
	tracker.recordFailureWithStatus("replica-a:443", codes.ResourceExhausted, &hint)
	state := tracker.entries["replica-a:443"]
	if state.overloadFailures != 1 || state.unavailableFailures != 0 {
		t.Fatalf("post-idle lanes = (%d, %d), want (1, 0)", state.overloadFailures, state.unavailableFailures)
	}
	clock.Advance(10 * time.Minute)
	tracker.recordSuccess("replica-a:443")
	if _, ok := tracker.entries["replica-a:443"]; ok {
		t.Fatal("idle entry was repaired instead of removed")
	}
}

func TestEndpointOverloadCooldownTracker_ProbeReservation(t *testing.T) {
	clock := newLifecycleTestClock(time.Unix(100, 0))
	tracker := newEndpointOverloadCooldownTrackerWithOptions(
		10*time.Second, time.Minute, 10*time.Minute, clock.Now,
		func(int64) int64 { return 0 },
	)
	hint := 100 * time.Millisecond
	for i := 0; i < defaultEndpointCooldownMaxTier; i++ {
		tracker.recordFailureWithStatus("replica-a:443", codes.ResourceExhausted, &hint)
	}
	clock.Advance(99 * time.Millisecond)
	if tracker.tryReserveProbe("replica-a:443") {
		t.Fatal("probe reserved before server delay")
	}
	clock.Advance(time.Millisecond)
	if !tracker.tryReserveProbe("replica-a:443") || tracker.tryReserveProbe("replica-a:443") {
		t.Fatal("probe reservation was not exclusive")
	}
	clock.Advance(249 * time.Millisecond)
	if tracker.tryReserveProbe("replica-a:443") {
		t.Fatal("probe reservation expired early")
	}
	clock.Advance(time.Millisecond)
	if !tracker.tryReserveProbe("replica-a:443") {
		t.Fatal("probe reservation did not reopen")
	}
}

func TestEndpointOverloadCooldownTracker_UnavailablePreventsProbeAndUnknownStatusIgnored(t *testing.T) {
	clock := newLifecycleTestClock(time.Unix(100, 0))
	tracker := newEndpointOverloadCooldownTrackerWithOptions(
		10*time.Second, time.Minute, 10*time.Minute, clock.Now,
		func(int64) int64 { return 0 },
	)
	hint := 100 * time.Millisecond
	tracker.recordFailureWithStatus("replica-a:443", codes.Aborted, &hint)
	if _, ok := tracker.entries["replica-a:443"]; ok {
		t.Fatal("untracked status created state")
	}
	for i := 0; i < defaultEndpointCooldownMaxTier; i++ {
		tracker.recordFailureWithStatus("replica-a:443", codes.ResourceExhausted, &hint)
	}
	tracker.recordFailureWithStatus("replica-a:443", codes.Unavailable, nil)
	clock.Advance(100 * time.Millisecond)
	if tracker.tryReserveProbe("replica-a:443") {
		t.Fatal("unavailable lane allowed early overload probe")
	}
}

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
	got := state.overloadUntil.Sub(clock.Now())
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
	if second.overloadFailures != first.overloadFailures+1 {
		t.Fatalf("consecutiveFailures = %d, want %d", second.overloadFailures, first.overloadFailures+1)
	}

	clock.Advance(11 * time.Minute)
	tracker.recordFailure("replica-a:443")
	third := tracker.entries["replica-a:443"]
	if third.overloadFailures != 1 {
		t.Fatalf("expected quiet window to reset failure count, got %d", third.overloadFailures)
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

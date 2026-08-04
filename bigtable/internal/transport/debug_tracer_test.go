// Copyright 2026 Google LLC
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//      http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package internal

import (
	"sync"
	"testing"
	"time"
)

// TestDebugTag_RecordCounts verifies the in-memory counter tracks every
// recordDebugTag call, including repeats, and snapshotDebugTagCounts returns an
// independent snapshot (mutating the returned map doesn't affect state).
func TestDebugTag_RecordCounts(t *testing.T) {
	resetDebugTagCountsForTest()
	recordDebugTag("tag_a")
	recordDebugTag("tag_a")
	recordDebugTagAt(lvl.Error, "tag_b")

	got := snapshotDebugTagCounts()
	if got["tag_a"] != 2 {
		t.Errorf("tag_a: got %d, want 2", got["tag_a"])
	}
	if got["tag_b"] != 1 {
		t.Errorf("tag_b: got %d, want 1", got["tag_b"])
	}

	// Mutation on the returned map must not bleed into the tracer.
	got["tag_a"] = 999
	if again := snapshotDebugTagCounts(); again["tag_a"] != 2 {
		t.Errorf("snapshot leaked mutation: got %d, want 2", again["tag_a"])
	}
}

// TestDebugTag_LevelFloor verifies emissions below the runtime floor are
// dropped in-process. Restores the default floor at the end so downstream
// tests aren't polluted.
func TestDebugTag_LevelFloor(t *testing.T) {
	resetDebugTagCountsForTest()
	t.Cleanup(func() { setDebugTagLevelFloor(lvl.Warn) })

	setDebugTagLevelFloor(lvl.Error)
	recordDebugTag("warn_below_floor")
	recordDebugTagAt(lvl.Error, "error_at_floor")

	got := snapshotDebugTagCounts()
	if _, present := got["warn_below_floor"]; present {
		t.Errorf("warn_below_floor should have been dropped, got %v", got)
	}
	if got["error_at_floor"] != 1 {
		t.Errorf("error_at_floor: got %d, want 1", got["error_at_floor"])
	}
}

// TestDebugTag_AssertPassAndFail verifies both assert forms
// (formatted and format-free) return the predicate result and
// increment the counter only on the failing branch.
func TestDebugTag_AssertPassAndFail(t *testing.T) {
	resetDebugTagCountsForTest()

	// Formatted form — captures diagnostic context in the log message.
	if ok := assertDebugTagf(true, "assert_pass_f", "should not fire"); !ok {
		t.Errorf("assertDebugTagf(true) returned false")
	}
	if ok := assertDebugTagf(false, "assert_fail_f", "context=%s", "test"); ok {
		t.Errorf("assertDebugTagf(false) returned true")
	}

	// Format-free form — the tag name is the whole message.
	if ok := assertDebugTag(true, "assert_pass"); !ok {
		t.Errorf("assertDebugTag(true) returned false")
	}
	if ok := assertDebugTag(false, "assert_fail"); ok {
		t.Errorf("assertDebugTag(false) returned true")
	}

	got := snapshotDebugTagCounts()
	for _, name := range []string{"assert_pass_f", "assert_pass"} {
		if _, present := got[name]; present {
			t.Errorf("%s fired despite predicate holding: %v", name, got)
		}
	}
	for _, name := range []string{"assert_fail_f", "assert_fail"} {
		if got[name] != 1 {
			t.Errorf("%s: got %d, want 1", name, got[name])
		}
	}
}

// TestDebugTag_FirstAndLastSeen verifies that FirstSeen is stamped
// exactly once at first emission and LastSeen advances with every
// subsequent emission. Uses a short sleep between the two record calls
// so timestamps land in distinct nanoseconds even on fast machines.
func TestDebugTag_FirstAndLastSeen(t *testing.T) {
	resetDebugTagCountsForTest()

	recordDebugTag("first_last_tag")
	firstPass := DebugTags()
	if len(firstPass) != 1 {
		t.Fatalf("DebugTags length = %d, want 1", len(firstPass))
	}
	first := firstPass[0]
	if first.Name != "first_last_tag" {
		t.Errorf("Name = %q, want %q", first.Name, "first_last_tag")
	}
	if first.Count != 1 || first.FirstSeen.IsZero() || first.LastSeen.IsZero() {
		t.Errorf("first snapshot missing fields: %+v", first)
	}
	if !first.FirstSeen.Equal(first.LastSeen) {
		t.Errorf("first-emission FirstSeen != LastSeen: %v vs %v", first.FirstSeen, first.LastSeen)
	}

	time.Sleep(2 * time.Millisecond)
	recordDebugTag("first_last_tag")
	second := DebugTags()[0]
	if second.Count != 2 {
		t.Errorf("Count after 2 emissions = %d, want 2", second.Count)
	}
	if !second.FirstSeen.Equal(first.FirstSeen) {
		t.Errorf("FirstSeen changed after re-emission: was %v, now %v", first.FirstSeen, second.FirstSeen)
	}
	if !second.LastSeen.After(first.LastSeen) {
		t.Errorf("LastSeen did not advance: %v -> %v", first.LastSeen, second.LastSeen)
	}
}

// TestDebugTag_SortedByLastSeen verifies DebugTags returns entries
// sorted by LastSeen descending — the debugview page depends on this
// ordering to surface just-fired tags at the top.
func TestDebugTag_SortedByLastSeen(t *testing.T) {
	resetDebugTagCountsForTest()

	recordDebugTag("sorted_older")
	time.Sleep(2 * time.Millisecond)
	recordDebugTag("sorted_newer")
	time.Sleep(2 * time.Millisecond)
	recordDebugTag("sorted_middle")
	// Emit "sorted_older" again to keep its Count > 1 but leave its
	// LastSeen stale — no, actually the point of the test is order; skip.

	snaps := DebugTags()
	if len(snaps) < 3 {
		t.Fatalf("want 3 snapshots, got %d: %+v", len(snaps), snaps)
	}
	if snaps[0].Name != "sorted_middle" {
		t.Errorf("top row = %q, want %q (most-recently emitted)", snaps[0].Name, "sorted_middle")
	}
	// The other two entries should follow in reverse-time order.
	for i := 1; i < len(snaps); i++ {
		if snaps[i-1].LastSeen.Before(snaps[i].LastSeen) {
			t.Errorf("out-of-order at index %d: %v then %v", i, snaps[i-1].LastSeen, snaps[i].LastSeen)
		}
	}
}

// TestDebugTag_ConcurrentEmission is a smoke test for the RWMutex-guarded
// map: many goroutines hammering the same tag should tally exactly, no
// dropped increments, no data race under -race.
func TestDebugTag_ConcurrentEmission(t *testing.T) {
	resetDebugTagCountsForTest()

	const goroutines = 16
	const perGoroutine = 500
	var wg sync.WaitGroup
	wg.Add(goroutines)
	for i := 0; i < goroutines; i++ {
		go func() {
			defer wg.Done()
			for j := 0; j < perGoroutine; j++ {
				recordDebugTag("hot_tag")
			}
		}()
	}
	wg.Wait()

	want := int64(goroutines * perGoroutine)
	if got := snapshotDebugTagCounts()["hot_tag"]; got != want {
		t.Errorf("hot_tag: got %d, want %d", got, want)
	}
}

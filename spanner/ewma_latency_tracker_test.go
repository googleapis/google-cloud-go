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
	"math"
	"testing"
	"time"
)

func TestEWMALatencyTrackerInitialization(t *testing.T) {
	clock := newLifecycleTestClock(time.Unix(100, 0))
	tracker := newEWMALatencyTrackerWithOptions(10*time.Second, clock.Now)
	tracker.update(100 * time.Microsecond)
	if got, want := tracker.scoreValue(), 100.0; got != want {
		t.Fatalf("scoreValue() = %v, want %v", got, want)
	}
}

func TestEWMALatencyTrackerUninitializedScore(t *testing.T) {
	tracker := newEWMALatencyTracker()
	if got := tracker.scoreValue(); got != math.MaxFloat64 {
		t.Fatalf("scoreValue() = %v, want MaxFloat64", got)
	}
}

func TestEWMALatencyTrackerFixedAlpha(t *testing.T) {
	clock := newLifecycleTestClock(time.Unix(100, 0))
	tracker := newEWMALatencyTrackerWithAlpha(0.5, clock.Now)
	tracker.update(100 * time.Microsecond)
	tracker.update(200 * time.Microsecond)
	tracker.update(300 * time.Microsecond)
	if got, want := tracker.scoreValue(), 225.0; got != want {
		t.Fatalf("scoreValue() = %v, want %v", got, want)
	}
}

func TestEWMALatencyTrackerTimeBasedAlpha(t *testing.T) {
	clock := newLifecycleTestClock(time.Unix(100, 0))
	tracker := newEWMALatencyTrackerWithOptions(10*time.Second, clock.Now)
	tracker.update(100 * time.Microsecond)
	clock.Advance(10 * time.Second)
	tracker.update(200 * time.Microsecond)

	alpha := 1 - math.Exp(-1)
	want := alpha*200 + (1-alpha)*100
	if got := tracker.scoreValue(); math.Abs(got-want) > 0.001 {
		t.Fatalf("scoreValue() = %v, want %v", got, want)
	}
}

func TestEWMALatencyTrackerRecordError(t *testing.T) {
	clock := newLifecycleTestClock(time.Unix(100, 0))
	tracker := newEWMALatencyTrackerWithAlpha(0.5, clock.Now)
	tracker.update(100 * time.Microsecond)
	tracker.recordError(10 * time.Millisecond)
	if got, want := tracker.scoreValue(), 5050.0; got != want {
		t.Fatalf("scoreValue() = %v, want %v", got, want)
	}
}

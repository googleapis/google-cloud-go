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

func TestEWMALatencyTracker_DefaultScore(t *testing.T) {
	tracker := newEWMALatencyTracker()
	if got := tracker.getScore(); got != math.MaxFloat64 {
		t.Fatalf("getScore() = %v, want %v", got, math.MaxFloat64)
	}
}

func TestEWMALatencyTracker_Update(t *testing.T) {
	tracker := newEWMALatencyTrackerWithAlpha(0.5)

	tracker.update(100 * time.Microsecond)
	if got := tracker.getScore(); got != 100 {
		t.Fatalf("first getScore() = %v, want 100", got)
	}

	tracker.update(300 * time.Microsecond)
	if got := tracker.getScore(); got != 200 {
		t.Fatalf("second getScore() = %v, want 200", got)
	}
}

func TestEWMALatencyTracker_RecordError(t *testing.T) {
	tracker := newEWMALatencyTrackerWithAlpha(0.5)
	tracker.update(100 * time.Microsecond)
	tracker.recordError(500 * time.Microsecond)

	if got := tracker.getScore(); got != 300 {
		t.Fatalf("getScore() after recordError = %v, want 300", got)
	}
}

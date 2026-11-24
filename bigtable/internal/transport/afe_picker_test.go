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
	"testing"
	"time"
)

// makeTestAfe builds an *afeHandle with idle sessions, active in-flight
// sessions, and seeded E2eEwma for unit-testing AfePicker implementations.
func makeTestAfe(id AfeID, idle, inflight int, e2eCost time.Duration) *afeHandle {
	sessions := make([]*SessionHandle, idle)
	for i := range sessions {
		sessions[i] = &SessionHandle{}
	}
	return &afeHandle{
		id:            id,
		sessions:      sessions,
		refCount:      idle + inflight,
		transportEwma: NewPeakEwmaSeeded(afePeakEwmaTau, afeTransportEwmaSeed),
		e2eEwma:       NewPeakEwmaSeeded(afePeakEwmaTau, e2eCost),
	}
}

func indexTestAfes(afes []*afeHandle) []*afeHandle {
	out := make([]*afeHandle, len(afes))
	copy(out, afes)
	for i, a := range out {
		a.readyIdx = i
	}
	return out
}

func TestSimpleAfePicker_EmptyReturnsNil(t *testing.T) {
	got, dec := NewSimpleAfePicker(true).pickLocked(nil)
	if got != nil {
		t.Errorf("pickLocked(nil) = %v, want nil", got)
	}
	if dec.Reason != "no-candidates" {
		t.Errorf("pickLocked(nil) reason = %q, want no-candidates", dec.Reason)
	}
}

func TestSimpleAfePicker_DistributesRoughly(t *testing.T) {
	afes := indexTestAfes([]*afeHandle{
		makeTestAfe(1, 1, 0, time.Millisecond),
		makeTestAfe(2, 1, 0, time.Millisecond),
		makeTestAfe(3, 1, 0, time.Millisecond),
	})
	counts := map[AfeID]int{}
	p := NewSimpleAfePicker(true)
	const N = 3000
	for i := 0; i < N; i++ {
		w, _ := p.pickLocked(afes)
		counts[w.id]++
	}
	for _, id := range []AfeID{1, 2, 3} {
		got := counts[id]
		if got < 750 || got > 1250 {
			t.Errorf("AFE %d picked %d/%d times, want ~1000 ±25%%", id, got, N)
		}
	}
}

func TestLeastInFlightAfePicker_EmptyReturnsNil(t *testing.T) {
	got, _ := NewLeastInFlightAfePicker(0, true).pickLocked(nil)
	if got != nil {
		t.Errorf("pickLocked(nil) = %v, want nil", got)
	}
}

func TestLeastInFlightAfePicker_PicksMinWhenSubsetCoversAll(t *testing.T) {
	afes := indexTestAfes([]*afeHandle{
		makeTestAfe(1, 1, 5, time.Millisecond),
		makeTestAfe(2, 1, 1, time.Millisecond), // min
		makeTestAfe(3, 1, 3, time.Millisecond),
	})
	p := NewLeastInFlightAfePicker(len(afes), true)
	for i := 0; i < 100; i++ {
		w, _ := p.pickLocked(afes)
		if w == nil || w.id != 2 {
			t.Fatalf("iter %d: picked %v, want AFE 2", i, w)
		}
	}
}

func TestLeastInFlightAfePicker_KChoicePrefersLowInflight(t *testing.T) {
	afes := indexTestAfes([]*afeHandle{
		makeTestAfe(1, 1, 100, time.Millisecond),
		makeTestAfe(2, 1, 100, time.Millisecond),
		makeTestAfe(3, 1, 1, time.Millisecond), // min
		makeTestAfe(4, 1, 100, time.Millisecond),
	})
	p := NewLeastInFlightAfePicker(2, true)
	counts := map[AfeID]int{}
	const N = 4000
	for i := 0; i < N; i++ {
		w, _ := p.pickLocked(afes)
		counts[w.id]++
	}
	share := float64(counts[3]) / float64(N)
	if share < 0.4 || share > 0.6 {
		t.Errorf("K=2 share for min-inflight AFE = %.3f, want ~0.5", share)
	}
}

func TestLeastLatencyAfePicker_PicksMinCost(t *testing.T) {
	afes := indexTestAfes([]*afeHandle{
		makeTestAfe(1, 1, 0, 50*time.Millisecond),
		makeTestAfe(2, 1, 0, 5*time.Millisecond), // min cost
		makeTestAfe(3, 1, 0, 30*time.Millisecond),
	})
	p := NewLeastLatencyAfePicker(len(afes), true)
	for i := 0; i < 100; i++ {
		w, _ := p.pickLocked(afes)
		if w == nil || w.id != 2 {
			t.Fatalf("iter %d: picked %v, want AFE 2 (min E2eCost)", i, w)
		}
	}
}

func TestLeastLatencyAfePicker_EmptyReturnsNil(t *testing.T) {
	got, _ := NewLeastLatencyAfePicker(0, true).pickLocked(nil)
	if got != nil {
		t.Errorf("pickLocked(nil) = %v, want nil", got)
	}
}

func TestKChoiceMinCostLocked_PreservesReadyIdxOnSwap(t *testing.T) {
	afes := indexTestAfes([]*afeHandle{
		makeTestAfe(1, 1, 0, time.Millisecond),
		makeTestAfe(2, 2, 0, time.Millisecond),
		makeTestAfe(3, 3, 0, time.Millisecond),
	})
	NewLeastInFlightAfePicker(3, true).pickLocked(afes)

	seen := map[AfeID]bool{}
	for idx, a := range afes {
		seen[a.id] = true
		if a.readyIdx != idx {
			t.Errorf("afes[%d] (id=%d) has readyIdx=%d, want %d", idx, a.id, a.readyIdx, idx)
		}
	}
	for _, want := range []AfeID{1, 2, 3} {
		if !seen[want] {
			t.Errorf("after pickLocked, ID %d missing from slice (%v)", want, seen)
		}
	}
}

func TestPickDecision_RecordsCandidatesAndReason(t *testing.T) {
	afes := indexTestAfes([]*afeHandle{
		makeTestAfe(1, 1, 5, 50*time.Millisecond),
		makeTestAfe(2, 1, 1, 5*time.Millisecond),
		makeTestAfe(3, 1, 3, 30*time.Millisecond),
	})
	_, dec := NewLeastInFlightAfePicker(2, true).pickLocked(afes)
	if len(dec.Candidates) != 2 {
		t.Fatalf("Candidates len = %d, want 2 (K-choice-2)", len(dec.Candidates))
	}
	if dec.Reason != "min-inflight" {
		t.Errorf("Reason = %q, want min-inflight", dec.Reason)
	}
	if dec.Winner == 0 {
		t.Error("Winner is zero; expected the chosen AFE id")
	}

	_, dec2 := NewLeastLatencyAfePicker(len(afes), true).pickLocked(afes)
	if dec2.Reason != "min-latency" {
		t.Errorf("LeastLatency reason = %q, want min-latency", dec2.Reason)
	}
	if len(dec2.Candidates) != len(afes) {
		t.Errorf("LeastLatency Candidates len = %d, want %d (K covered all)", len(dec2.Candidates), len(afes))
	}
}

func TestAfePicker_Names(t *testing.T) {
	for _, tc := range []struct {
		p    AfePicker
		want string
	}{
		{NewSimpleAfePicker(true), "simple"},
		{NewLeastInFlightAfePicker(0, true), "least-inflight"},
		{NewLeastLatencyAfePicker(0, true), "least-latency"},
	} {
		if got := tc.p.Name(); got != tc.want {
			t.Errorf("%T.Name() = %q, want %q", tc.p, got, tc.want)
		}
	}
}

func TestLeastInFlightAfePicker_TieBreaksTowardHigherIdleCount(t *testing.T) {
	tests := []struct {
		name string
		afes []*afeHandle
		want AfeID
	}{
		{
			name: "higher_idle_after_single_idle",
			afes: []*afeHandle{
				makeTestAfe(1, 1, 0, time.Millisecond),
				makeTestAfe(2, 6, 0, time.Millisecond),
			},
			want: 2,
		},
		{
			name: "higher_idle_before_single_idle",
			afes: []*afeHandle{
				makeTestAfe(2, 6, 0, time.Millisecond),
				makeTestAfe(1, 1, 0, time.Millisecond),
			},
			want: 2,
		},
		{
			name: "lower_inflight_still_beats_higher_idle",
			afes: []*afeHandle{
				makeTestAfe(1, 1, 0, time.Millisecond),
				makeTestAfe(2, 10, 1, time.Millisecond),
			},
			want: 1,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			p := NewLeastInFlightAfePicker(len(tc.afes), true)
			for i := 0; i < 50; i++ {
				in := indexTestAfes(tc.afes)
				got, dec := p.pickLocked(in)
				if got == nil {
					t.Fatalf("iter %d: pickLocked returned nil", i)
				}
				if got.id != tc.want {
					t.Fatalf("iter %d: pickLocked = %d, want %d", i, got.id, tc.want)
				}
				if dec.Winner != tc.want {
					t.Fatalf("iter %d: dec.Winner = %d, want %d", i, dec.Winner, tc.want)
				}
			}
		})
	}
}

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
	"context"
	"testing"
	"time"
)

// fakeAfeSource returns a fixed slice of AfeSnapshotRow. Simulates a
// sessionList for scorer tests without standing up the real one.
type fakeAfeSource struct {
	rows []AfeSnapshotRow
}

func (f *fakeAfeSource) Snapshot() []AfeSnapshotRow { return f.rows }

// row is a compact test constructor for AfeSnapshotRow. e2e and
// transport are given as time.Duration for readability.
func row(id AfeID, e2e, transport time.Duration) AfeSnapshotRow {
	return AfeSnapshotRow{
		ID:            id,
		RefCount:      1,
		IdleCount:     1,
		E2eEwma:       e2e,
		TransportEwma: transport,
		LastConnected: time.Now(),
	}
}

// spyScorer records SetOutlierScorer + LifecycleScorer.Start invocations
// so tests can assert the pool wired it correctly.
type spyScorer struct {
	scoreCalls int64
	startCalls int64
}

func (s *spyScorer) Score(AfeID) float64 { s.scoreCalls++; return 1.0 }
func (*spyScorer) Name() string          { return "spy" }
func (s *spyScorer) Start(context.Context) {
	s.startCalls++
}

// TestSessionPoolImpl_LifecycleScorerStartInvoked pins the wire-in that
// SessionPoolImpl.Start calls scorer.Start(poolCtx) when the plugged
// scorer implements LifecycleScorer. Downstream from
// ClientConfig.OutlierScorerFactory — if this ever regresses, factories
// wiring LatencyOutlierScorer (which needs the ctx to run its tick
// loop) would silently no-op.
func TestSessionPoolImpl_LifecycleScorerStartInvoked(t *testing.T) {
	pool := NewSessionPoolImpl(
		1, "test-pool", nil, nil, nil, SessionType(0), false,
	)
	spy := &spyScorer{}
	pool.SetOutlierScorer(spy)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	pool.Start(ctx)
	// Give the goroutine a chance if Start's scheduling is async;
	// current Start invokes ls.Start synchronously.
	if got := spy.startCalls; got != 1 {
		t.Errorf("LifecycleScorer.Start called %d times, want 1", got)
	}
}

// TestSessionPoolImpl_AfeSnapshotSourceReturnsPoolList verifies the
// AfeSnapshotSource accessor exposes the pool's sessionList — the
// interface an OutlierScorerFactory receives to build stateful scorers
// like LatencyOutlierScorer.
func TestSessionPoolImpl_AfeSnapshotSourceReturnsPoolList(t *testing.T) {
	pool := NewSessionPoolImpl(
		2, "test-pool", nil, nil, nil, SessionType(0), false,
	)
	src := pool.AfeSnapshotSource()
	if src == nil {
		t.Fatal("AfeSnapshotSource() = nil, want non-nil (pool.sl)")
	}
	// Snapshot must be callable and return an empty slice on a fresh pool.
	rows := src.Snapshot()
	if len(rows) != 0 {
		t.Errorf("fresh pool Snapshot() = %d rows, want 0", len(rows))
	}
}

func TestNoopScorer_AlwaysReturnsOne(t *testing.T) {
	var s OutlierScorer = NoopScorer{}
	for _, id := range []AfeID{0, 1, 42, -1, 1 << 62} {
		if got := s.Score(id); got != 1.0 {
			t.Errorf("NoopScorer.Score(%d) = %v, want 1.0", id, got)
		}
	}
}

func TestLatencyOutlierScorer_UnknownAfeReturnsOne(t *testing.T) {
	src := &fakeAfeSource{}
	s := NewLatencyOutlierScorer(src, LatencyOutlierConfig{})
	if got := s.Score(42); got != 1.0 {
		t.Errorf("Score(42) before first tick = %v, want 1.0", got)
	}
}

func TestLatencyOutlierScorer_PenalizesSlowAfe(t *testing.T) {
	src := &fakeAfeSource{
		rows: []AfeSnapshotRow{
			row(1, 1*time.Millisecond, 500*time.Microsecond),
			row(2, 1*time.Millisecond, 500*time.Microsecond),
			row(3, 1*time.Millisecond, 500*time.Microsecond),
			row(4, 1*time.Millisecond, 500*time.Microsecond),
			row(5, 100*time.Millisecond, 500*time.Microsecond), // 100x the peers
		},
	}
	s := NewLatencyOutlierScorer(src, LatencyOutlierConfig{})
	s.tick()
	for _, id := range []AfeID{1, 2, 3, 4} {
		if got := s.Score(id); got != 1.0 {
			t.Errorf("Score(healthy AFE %d) = %v, want 1.0", id, got)
		}
	}
	if got := s.Score(5); got != 10.0 {
		t.Errorf("Score(slow AFE 5) = %v, want 10.0", got)
	}
}

func TestLatencyOutlierScorer_RecoversWhenLatencyDrops(t *testing.T) {
	src := &fakeAfeSource{
		rows: []AfeSnapshotRow{
			row(1, 1*time.Millisecond, 500*time.Microsecond),
			row(2, 1*time.Millisecond, 500*time.Microsecond),
			row(3, 1*time.Millisecond, 500*time.Microsecond),
			row(4, 1*time.Millisecond, 500*time.Microsecond),
			row(5, 100*time.Millisecond, 500*time.Microsecond),
		},
	}
	s := NewLatencyOutlierScorer(src, LatencyOutlierConfig{})
	s.tick()
	if got := s.Score(5); got != 10.0 {
		t.Fatalf("pre-recovery Score(5) = %v, want 10.0", got)
	}
	// AFE 5 recovers.
	src.rows[4] = row(5, 1*time.Millisecond, 500*time.Microsecond)
	s.tick()
	if got := s.Score(5); got != 1.0 {
		t.Errorf("post-recovery Score(5) = %v, want 1.0", got)
	}
}

func TestLatencyOutlierScorer_BelowMinCohortSkips(t *testing.T) {
	src := &fakeAfeSource{
		rows: []AfeSnapshotRow{
			row(1, 1*time.Millisecond, 500*time.Microsecond),
			row(2, 100*time.Millisecond, 500*time.Microsecond),
		},
	}
	s := NewLatencyOutlierScorer(src, LatencyOutlierConfig{MinCohortSize: 3})
	s.tick()
	// AFE 2 would look like a huge outlier but cohort has only 2 members.
	if got := s.Score(2); got != 1.0 {
		t.Errorf("Score(2) with cohort=2 = %v, want 1.0 (skipped by min-cohort)", got)
	}
}

func TestLatencyOutlierScorer_BelowLatencyFloorSkips(t *testing.T) {
	src := &fakeAfeSource{
		rows: []AfeSnapshotRow{
			row(1, 100*time.Microsecond, 100*time.Microsecond),
			row(2, 100*time.Microsecond, 100*time.Microsecond),
			row(3, 100*time.Microsecond, 100*time.Microsecond),
			row(4, 100*time.Microsecond, 100*time.Microsecond),
			row(5, 900*time.Microsecond, 100*time.Microsecond), // 9× the cohort but below 20ms floor
		},
	}
	s := NewLatencyOutlierScorer(src, LatencyOutlierConfig{})
	s.tick()
	if got := s.Score(5); got != 1.0 {
		t.Errorf("Score(5) at sub-floor latency = %v, want 1.0 (below floor)", got)
	}
}

func TestLatencyOutlierScorer_TransitionOnlyEmission(t *testing.T) {
	src := &fakeAfeSource{
		rows: []AfeSnapshotRow{
			row(1, 1*time.Millisecond, 500*time.Microsecond),
			row(2, 1*time.Millisecond, 500*time.Microsecond),
			row(3, 1*time.Millisecond, 500*time.Microsecond),
			row(4, 1*time.Millisecond, 500*time.Microsecond),
			row(5, 100*time.Millisecond, 500*time.Microsecond),
		},
	}
	s := NewLatencyOutlierScorer(src, LatencyOutlierConfig{})

	resetDebugTagCountsForTest()
	s.tick() // AFE 5: 1.0 → 10.0 (penalized fires once)
	s.tick() // AFE 5: 10.0 → 10.0 (no transition; no tag)
	s.tick() // AFE 5: 10.0 → 10.0 (still no tag)

	// AFE 5 recovers on tick 4: 10.0 → 1.0 (recovered fires once)
	src.rows[4] = row(5, 1*time.Millisecond, 500*time.Microsecond)
	s.tick()
	// Steady healthy on tick 5: no transition
	s.tick()

	counts := snapshotDebugTagCounts()
	if got := counts[tagOutlierAfePenalizedLatency]; got != 1 {
		t.Errorf("penalized tag fired %d times, want 1 (transition-only)", got)
	}
	if got := counts[tagOutlierAfeRecoveredLatency]; got != 1 {
		t.Errorf("recovered tag fired %d times, want 1 (transition-only)", got)
	}
}

func TestLatencyOutlierScorer_AuditRingBounded(t *testing.T) {
	cfg := LatencyOutlierConfig{AuditRingSize: 3}
	src := &fakeAfeSource{
		rows: []AfeSnapshotRow{
			row(1, 1*time.Millisecond, 500*time.Microsecond),
			row(2, 1*time.Millisecond, 500*time.Microsecond),
			row(3, 1*time.Millisecond, 500*time.Microsecond),
		},
	}
	s := NewLatencyOutlierScorer(src, cfg)
	// Force 5 transitions on AFE 3.
	for i := 0; i < 5; i++ {
		if i%2 == 0 {
			src.rows[2] = row(3, 100*time.Millisecond, 500*time.Microsecond)
		} else {
			src.rows[2] = row(3, 1*time.Millisecond, 500*time.Microsecond)
		}
		s.tick()
	}
	got := s.OutlierSnapshot()
	if len(got) != 3 {
		t.Errorf("OutlierSnapshot len = %d, want 3 (ring cap)", len(got))
	}
	// Oldest evicted: we'd have 5 decisions but only the last 3 survive.
	for _, d := range got {
		if d.AfeID != 3 {
			t.Errorf("decision AfeID = %d, want 3", d.AfeID)
		}
	}
}

func TestLatencyOutlierScorer_PickerIntegration(t *testing.T) {
	// Populate the scorer's score map by running one tick against a
	// synthetic snapshot where AFE 1 is a heavy outlier.
	src := &fakeAfeSource{
		rows: []AfeSnapshotRow{
			row(1, 100*time.Millisecond, 500*time.Microsecond),
			row(2, 1*time.Millisecond, 500*time.Microsecond),
			row(3, 1*time.Millisecond, 500*time.Microsecond),
			row(4, 1*time.Millisecond, 500*time.Microsecond),
			row(5, 1*time.Millisecond, 500*time.Microsecond),
		},
	}
	scorer := NewLatencyOutlierScorer(src, LatencyOutlierConfig{})
	scorer.tick()
	if got := scorer.Score(1); got != 10.0 {
		t.Fatalf("Score(1) = %v, want 10.0 after penalty tick", got)
	}

	// Build picker-facing snapshots and decorate with scores as
	// SessionPoolImpl.decorateReady would.
	snaps := []AfeSnapshot{
		{ID: 1, IdleCount: 1, E2eCost: float64(1 * time.Millisecond)},
		{ID: 2, IdleCount: 1, E2eCost: float64(1 * time.Millisecond)},
		{ID: 3, IdleCount: 1, E2eCost: float64(1 * time.Millisecond)},
		{ID: 4, IdleCount: 1, E2eCost: float64(1 * time.Millisecond)},
		{ID: 5, IdleCount: 1, E2eCost: float64(1 * time.Millisecond)},
	}
	for i := range snaps {
		snaps[i].OutlierScore = scorer.Score(snaps[i].ID)
	}

	// Run 5000 K-choice picks with K=2. All AFEs have the same base
	// E2eCost; only AFE 1 has an inflated score, so K-choice should
	// almost never pick AFE 1.
	picker := NewLeastLatencyAfePicker(2, false)
	counts := map[AfeID]int{}
	const N = 5000
	for i := 0; i < N; i++ {
		// Copy snaps because kChoiceMinCost mutates in place.
		draw := make([]AfeSnapshot, len(snaps))
		copy(draw, snaps)
		id, _, _ := picker.PickAfe(draw)
		counts[id]++
	}
	// Uniform expected = 1000 per AFE. AFE 1 should be picked far less.
	if counts[1] > N/20 {
		t.Errorf("AFE 1 (penalized) picked %d/%d times (%.1f%%), want < 5%%",
			counts[1], N, 100*float64(counts[1])/float64(N))
	}
	// Healthy AFEs collectively should absorb roughly all traffic.
	healthy := counts[2] + counts[3] + counts[4] + counts[5]
	if healthy < N-N/20 {
		t.Errorf("healthy AFEs got %d/%d picks, want > 95%%", healthy, N)
	}
}

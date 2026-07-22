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
	"sync/atomic"
	"testing"

	spb "cloud.google.com/go/bigtable/apiv2/bigtablepb"
)

// fixedFetcher returns the same *PoolStats on every call. Pass nil to
// simulate a pool that hasn't started yet (no-stats branch).
func fixedFetcher(s *PoolStats) StatsFetcher {
	return func() *PoolStats { return s }
}

func TestPoolSizer_NoStatsBranch(t *testing.T) {
	s := NewPoolSizer(fixedFetcher(nil), 1, 10, 0.10)
	d := s.Decide()
	if d.Branch != "no-stats" {
		t.Errorf("Branch = %q, want no-stats", d.Branch)
	}
	if d.Delta != 0 {
		t.Errorf("Delta = %d, want 0 (no-stats must not scale)", d.Delta)
	}
	// Inputs must be zero — the fetcher returned nil, so nothing to
	// copy in.
	if d.ReadyCount != 0 || d.InUseCount != 0 || d.PendingCount != 0 {
		t.Errorf("input fields non-zero on no-stats: %+v", d)
	}
	// Config snapshot MUST still be filled — operators read it to
	// diagnose "why did the sizer choose nothing" without a stats
	// snapshot.
	if d.MinSessions != 1 || d.MaxSessions != 10 || d.HeadroomPct != 0.10 {
		t.Errorf("config snapshot not populated on no-stats: %+v", d)
	}
}

// TestPoolSizer_ScaleUpFromZero pins the cold-start path: an empty
// pool with pending demand must scale up to at least MinSessions.
func TestPoolSizer_ScaleUpFromZero(t *testing.T) {
	// 5 waiters, qlen=10 → EffectivePending=1; in-use=0; SessionsInUse=1;
	// headroom=max(1, ceil(1*0.10))=1; DesiredRaw=2; clamp[3,10]=3;
	// ready=0, starting=0, eventual=0; delta = 3 − 0 = 3.
	s := NewPoolSizer(fixedFetcher(&PoolStats{PendingCount: 5}), 3, 10, 0.10)
	d := s.Decide()
	if d.Branch != "scale-up" {
		t.Errorf("Branch = %q, want scale-up", d.Branch)
	}
	if d.Delta != 3 {
		t.Errorf("Delta = %d, want 3 (clamp to MinSessions)", d.Delta)
	}
	if d.DesiredCapacity != 3 {
		t.Errorf("DesiredCapacity = %d, want 3 (min-clamp fired)", d.DesiredCapacity)
	}
	if d.EffectivePending != 1 {
		t.Errorf("EffectivePending = %d, want 1 (ceil(5/10))", d.EffectivePending)
	}
}

// TestPoolSizer_ScaleUpAbsorbedByStarting pins the dead-band case
// where starting sessions already cover the new demand.
func TestPoolSizer_ScaleUpAbsorbedByStarting(t *testing.T) {
	// InUse=4, Pending=0 → SessionsInUse=4; headroom=ceil(4*0.10)=1;
	// DesiredRaw=5; clamp[1,20]=5. Ready=3, Starting=2, Eventual=5.
	// Desired (5) == Eventual (5) → dead-band.
	s := NewPoolSizer(fixedFetcher(&PoolStats{
		ReadyCount:    3,
		StartingCount: 2,
		InUseCount:    4,
	}), 1, 20, 0.10)
	d := s.Decide()
	if d.Branch != "dead-band" {
		t.Errorf("Branch = %q, want dead-band (Desired=Eventual)", d.Branch)
	}
	if d.Delta != 0 {
		t.Errorf("Delta = %d, want 0", d.Delta)
	}
}

// TestPoolSizer_ScaleDownAdvisoryOnly pins the passive-shrink
// contract: delta is negative when the pool is overprovisioned, but
// the sizer NEVER instructs the caller to kill sessions — it's
// advisory for OnClose replacement decisions.
func TestPoolSizer_ScaleDownAdvisoryOnly(t *testing.T) {
	// Ready=10, Starting=0, InUse=1, Pending=0. SessionsInUse=1;
	// headroom=max(1, ceil(1*0.10))=1; DesiredRaw=2; clamp[1,20]=2.
	// Immediate=10, Eventual=10. Desired (2) < Immediate (10) → scale-down.
	// Delta = 2 − 10 = −8.
	s := NewPoolSizer(fixedFetcher(&PoolStats{
		ReadyCount: 10,
		InUseCount: 1,
	}), 1, 20, 0.10)
	d := s.Decide()
	if d.Branch != "scale-down" {
		t.Errorf("Branch = %q, want scale-down", d.Branch)
	}
	if d.Delta != -8 {
		t.Errorf("Delta = %d, want -8 (advisory shrink hint)", d.Delta)
	}
}

// TestPoolSizer_MaxSessionsClamp pins the top-end clamp: even under
// extreme demand, DesiredCapacity cannot exceed MaxSessions.
func TestPoolSizer_MaxSessionsClamp(t *testing.T) {
	// 500 waiters, qlen=10 → EffectivePending=50; InUse=200;
	// SessionsInUse=250; headroom=ceil(250*0.10)=25; DesiredRaw=275;
	// clamp[1,100]=100. Ready=0, Starting=0, Eventual=0; delta=100.
	s := NewPoolSizer(fixedFetcher(&PoolStats{
		InUseCount:   200,
		PendingCount: 500,
	}), 1, 100, 0.10)
	d := s.Decide()
	if d.DesiredCapacity != 100 {
		t.Errorf("DesiredCapacity = %d, want 100 (max-clamped)", d.DesiredCapacity)
	}
	if d.DesiredRaw != 275 {
		t.Errorf("DesiredRaw = %d, want 275 (pre-clamp)", d.DesiredRaw)
	}
	if d.Delta != 100 {
		t.Errorf("Delta = %d, want 100", d.Delta)
	}
}

// TestPoolSizer_HeadroomFloorPreventsCushionCollapse pins the
// MinIdleSessions floor: a pool with tiny in-use count must still
// carry at least 1 idle slot so cold-start doesn't stall the next
// checkout.
func TestPoolSizer_HeadroomFloorPreventsCushionCollapse(t *testing.T) {
	// InUse=1, Pending=0. SessionsInUse=1; ceil(1*0.10) = 1 (already
	// above floor). Now try InUse=0 with a huge headroom multiplier
	// that would still round down to 0 without the floor.
	//
	// InUse=0, Pending=0 → SessionsInUse=0; ceil(0*anything)=0.
	// Floor kicks in: IdleHeadroom = MinIdleSessions = 1.
	// DesiredRaw = 0 + 1 = 1; clamp[1,10]=1.
	s := NewPoolSizer(fixedFetcher(&PoolStats{}), 1, 10, 0.10)
	d := s.Decide()
	if d.IdleHeadroom != 1 {
		t.Errorf("IdleHeadroom = %d, want 1 (MinIdleSessions floor)", d.IdleHeadroom)
	}
	if d.DesiredCapacity != 1 {
		t.Errorf("DesiredCapacity = %d, want 1 (floor + min-clamp)", d.DesiredCapacity)
	}
	if d.Branch != "scale-up" {
		t.Errorf("Branch = %q, want scale-up (need to reach floor)", d.Branch)
	}
	if d.Delta != 1 {
		t.Errorf("Delta = %d, want 1", d.Delta)
	}
}

// TestPoolSizer_EffectivePendingCeil pins the ceil() semantics on
// pending-to-sessions conversion: any leftover waiter (even 1 out of
// qlen) requires a full extra session, never a fractional one.
func TestPoolSizer_EffectivePendingCeil(t *testing.T) {
	tests := []struct {
		name    string
		pending int
		qlen    int
		want    int
	}{
		{"exact-multiple", 20, 10, 2},
		{"one-over", 21, 10, 3},
		{"single-waiter", 1, 10, 1},
		{"zero", 0, 10, 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := NewPoolSizer(fixedFetcher(&PoolStats{PendingCount: tt.pending}), 1, 100, 0.10)
			if tt.qlen != defaultNewSessionQueueLength {
				s.newSessionQLen = tt.qlen
			}
			d := s.Decide()
			if d.EffectivePending != tt.want {
				t.Errorf("EffectivePending(pending=%d, qlen=%d) = %d, want %d",
					tt.pending, tt.qlen, d.EffectivePending, tt.want)
			}
		})
	}
}

// TestPoolSizer_HeadroomDefaultOnNonPositive pins the constructor
// guard: headroomPct <= 0 falls back to 0.10 so a mis-configured
// callsite doesn't shrink the pool to the exact in-use count and
// starve the next checkout.
func TestPoolSizer_HeadroomDefaultOnNonPositive(t *testing.T) {
	for _, hp := range []float64{0, -1, -0.5} {
		t.Run("", func(t *testing.T) {
			s := NewPoolSizer(fixedFetcher(&PoolStats{}), 1, 10, hp)
			if got := s.Decide().HeadroomPct; got != 0.10 {
				t.Errorf("headroomPct=%v → decision.HeadroomPct=%v, want default 0.10",
					hp, got)
			}
		})
	}
}

// TestPoolSizer_UpdateConfigTakesEffect pins the runtime-swap
// contract: UpdateConfig mutates the sizer atomically w.r.t. Decide,
// and the next Decide reflects the new values.
func TestPoolSizer_UpdateConfigTakesEffect(t *testing.T) {
	s := NewPoolSizer(fixedFetcher(&PoolStats{InUseCount: 5}), 1, 10, 0.10)

	// Before: max=10, headroom=0.10.
	before := s.Decide()
	if before.MaxSessions != 10 {
		t.Fatalf("pre-update MaxSessions = %d, want 10", before.MaxSessions)
	}

	s.UpdateConfig(&spb.SessionClientConfiguration_SessionPoolConfiguration{
		MinSessionCount:       2,
		MaxSessionCount:       50,
		Headroom:              0.25,
		NewSessionQueueLength: 20,
	})

	after := s.Decide()
	if after.MinSessions != 2 || after.MaxSessions != 50 ||
		after.HeadroomPct != 0.25 || after.NewSessionQLen != 20 {
		t.Errorf("post-update snapshot mismatch: %+v", after)
	}
}

// TestPoolSizer_UpdateConfigNormalizesNonPositiveHeadroom pins the
// contract that UpdateConfig mirrors the constructor's headroom guard:
// a zero or negative Headroom from the server falls back to 0.10
// instead of being written through as-is. Without this, HeadroomPct
// on the ScaleDecision trace would render as 0 on loadz/sessionz and
// mislead operators — the pool would still work (MinIdleSessions
// floor prevents cushion-collapse), but the diagnostic trace would
// lie.
func TestPoolSizer_UpdateConfigNormalizesNonPositiveHeadroom(t *testing.T) {
	for _, hp := range []float32{0, -1, -0.5} {
		t.Run("", func(t *testing.T) {
			s := NewPoolSizer(fixedFetcher(&PoolStats{}), 1, 10, 0.25)
			s.UpdateConfig(&spb.SessionClientConfiguration_SessionPoolConfiguration{
				MinSessionCount:       1,
				MaxSessionCount:       10,
				Headroom:              hp,
				NewSessionQueueLength: 10,
			})
			if got := s.Decide().HeadroomPct; got != 0.10 {
				t.Errorf("UpdateConfig headroom=%v → HeadroomPct=%v, want default 0.10", hp, got)
			}
		})
	}
}

// TestPoolSizer_UpdateConfigIgnoresZeroQueueLength pins the guard on
// NewSessionQueueLength: a zero from the server would divide-by-zero
// the pending calculation, so the sizer keeps its prior (or default)
// value instead of overwriting.
func TestPoolSizer_UpdateConfigIgnoresZeroQueueLength(t *testing.T) {
	s := NewPoolSizer(fixedFetcher(&PoolStats{}), 1, 10, 0.10)
	s.newSessionQLen = 7 // simulate a prior non-default value

	s.UpdateConfig(&spb.SessionClientConfiguration_SessionPoolConfiguration{
		MinSessionCount:       1,
		MaxSessionCount:       10,
		Headroom:              0.10,
		NewSessionQueueLength: 0, // must be ignored
	})

	if got := s.Decide().NewSessionQLen; got != 7 {
		t.Errorf("NewSessionQLen after zero-update = %d, want prior 7", got)
	}
}

// TestPoolSizer_DecideTraceFieldsPopulated pins the ScaleDecision
// contract that ops read: every intermediate MUST be filled from the
// same evaluation so operators can trace the arithmetic without
// re-running.
func TestPoolSizer_DecideTraceFieldsPopulated(t *testing.T) {
	stats := &PoolStats{
		ReadyCount:    2,
		StartingCount: 1,
		InUseCount:    3,
		PendingCount:  15,
	}
	s := NewPoolSizer(fixedFetcher(stats), 1, 100, 0.20)
	d := s.Decide()

	if d.ReadyCount != 2 || d.StartingCount != 1 || d.InUseCount != 3 || d.PendingCount != 15 {
		t.Errorf("input fields not copied through: %+v", d)
	}
	// EffectivePending = ceil(15 / 10) = 2.
	if d.EffectivePending != 2 {
		t.Errorf("EffectivePending = %d, want 2", d.EffectivePending)
	}
	// SessionsInUse = 3 + 2 = 5.
	if d.SessionsInUse != 5 {
		t.Errorf("SessionsInUse = %d, want 5", d.SessionsInUse)
	}
	// IdleHeadroom = max(1, ceil(5 * 0.20)) = 1.
	if d.IdleHeadroom != 1 {
		t.Errorf("IdleHeadroom = %d, want 1", d.IdleHeadroom)
	}
	// DesiredRaw = 5 + 1 = 6.
	if d.DesiredRaw != 6 {
		t.Errorf("DesiredRaw = %d, want 6", d.DesiredRaw)
	}
	// ImmediateCapacity = ReadyCount = 2.
	if d.ImmediateCapacity != 2 {
		t.Errorf("ImmediateCapacity = %d, want 2", d.ImmediateCapacity)
	}
	// EventualCapacity = 2 + 1 = 3.
	if d.EventualCapacity != 3 {
		t.Errorf("EventualCapacity = %d, want 3", d.EventualCapacity)
	}
}

// TestPoolSizer_GetScaleDeltaMatchesDecide pins that the convenience
// wrapper returns the same delta the trace-carrying Decide() returns.
func TestPoolSizer_GetScaleDeltaMatchesDecide(t *testing.T) {
	s := NewPoolSizer(fixedFetcher(&PoolStats{
		InUseCount:   4,
		PendingCount: 12,
	}), 1, 20, 0.10)
	want := s.Decide().Delta
	if got := s.GetScaleDelta(); got != want {
		t.Errorf("GetScaleDelta() = %d, want %d (Decide().Delta)", got, want)
	}
}

// TestPoolSizer_ConcurrentDecideAndUpdateConfig stresses the sizer's
// mutex under concurrent UpdateConfig + Decide calls. Meaningful
// under -race — the Decide return type is a value copy, so no shared
// state escapes.
func TestPoolSizer_ConcurrentDecideAndUpdateConfig(t *testing.T) {
	var callCount atomic.Int64
	fetcher := func() *PoolStats {
		callCount.Add(1)
		return &PoolStats{InUseCount: 5, PendingCount: 3}
	}
	s := NewPoolSizer(fetcher, 1, 50, 0.10)

	var wg sync.WaitGroup
	const workers = 8
	const iters = 500

	wg.Add(workers * 2)
	for i := 0; i < workers; i++ {
		go func() {
			defer wg.Done()
			for j := 0; j < iters; j++ {
				_ = s.Decide()
			}
		}()
		go func(seed int) {
			defer wg.Done()
			for j := 0; j < iters; j++ {
				s.UpdateConfig(&spb.SessionClientConfiguration_SessionPoolConfiguration{
					MinSessionCount:       int32(1 + seed%3),
					MaxSessionCount:       int32(50 + seed%10),
					Headroom:              0.10,
					NewSessionQueueLength: int32(10 + seed%5),
				})
			}
		}(i)
	}
	wg.Wait()

	if callCount.Load() == 0 {
		t.Error("fetcher never called; Decide loop may have short-circuited")
	}
}

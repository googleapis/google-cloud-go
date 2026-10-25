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
	"sync/atomic"
	"testing"
	"time"

	spb "cloud.google.com/go/bigtable/apiv2/bigtablepb"
)

// TestComputeTarget_FormulaAndClamps exercises the pure sizing function
// across the corner cases the monitor relies on: cold-start (0 sessions
// → min), typical scale-up (crosses softMax boundary), overflow clamp
// (target > max), and softMax<=0 (config not usable).
func TestComputeTarget_FormulaAndClamps(t *testing.T) {
	cases := []struct {
		name                   string
		sessions               int
		softMax, min, max      int
		want                   int
	}{
		{"cold-start-min", 0, 4, 2, 8, 2},
		{"one-session-min", 1, 4, 2, 8, 2},
		{"headroom-boundary", 4, 4, 2, 8, 2},
		{"crosses-softmax", 8, 4, 2, 8, 4},
		{"grows-to-mid", 12, 4, 2, 8, 6},
		{"clamps-at-max", 32, 4, 2, 8, 8},
		{"clamps-at-max-way-over", 1000, 4, 2, 8, 8},
		{"softmax-zero-returns-zero", 100, 0, 2, 8, 0},
		{"softmax-negative-returns-zero", 100, -1, 2, 8, 0},
		{"softmax-one-headroom-scales", 3, 1, 1, 10, 6},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := ComputeTarget(tc.sessions, tc.softMax, tc.min, tc.max)
			if got != tc.want {
				t.Errorf("ComputeTarget(sessions=%d, softMax=%d, min=%d, max=%d) = %d, want %d",
					tc.sessions, tc.softMax, tc.min, tc.max, got, tc.want)
			}
		})
	}
}

// TestComputeTarget_HeadroomMatchesConstant guards against a silent
// change to SessionAwareHeadroom — the sizing behavior is documented
// as HEADROOM=2 across languages, so any bump needs to be intentional
// (and this test forces it into the PR).
func TestComputeTarget_HeadroomMatchesConstant(t *testing.T) {
	if got := SessionAwareHeadroom; got != 2 {
		t.Errorf("SessionAwareHeadroom = %d, want 2 (cross-language contract)", got)
	}
}

// fakeScaleTarget captures addConnections / removeConnections calls in
// place of a real BigtableChannelPool. The monitor's real methods take
// a *BigtableChannelPool so we can't hand it a fake directly — instead
// the tick-level integration tests below run against
// scaleDecisionRecorder which mirrors the monitor's tick() logic
// without the pool dependency, letting us assert the decisions the
// monitor would take.

// scaleDecisionRecorder is a stand-in for the pool's mutation side.
// It exposes the same "have vs target → grow/shrink delta" contract
// the monitor drives, but records the decision instead of dialing.
type scaleDecisionRecorder struct {
	have     atomic.Int32
	adds     atomic.Int32
	removes  atomic.Int32
	lastAdd  atomic.Int32
	lastRem  atomic.Int32
}

// reconcile mirrors SessionAwareScaleMonitor.tick's grow/shrink branch
// so we can assert what the monitor would drive without wiring a
// real pool. Reads ComputeTarget for the pure formula.
func (r *scaleDecisionRecorder) reconcile(sessions, softMax, min, max int) {
	target := ComputeTarget(sessions, softMax, min, max)
	if target <= 0 {
		return
	}
	have := int(r.have.Load())
	switch {
	case target > have:
		delta := target - have
		r.adds.Add(1)
		r.lastAdd.Store(int32(delta))
		r.have.Store(int32(target))
	case target < have:
		delta := have - target
		r.removes.Add(1)
		r.lastRem.Store(int32(delta))
		r.have.Store(int32(target))
	}
}

// TestSessionAwareScaleMonitor_ScaleUpOnSessionRamp drives session count
// upward and asserts the recorder observes growth deltas that would
// bring the pool from min to max on a big-enough ramp. Uses
// scaleDecisionRecorder (not a real pool) so the test is
// deterministic and doesn't dial network.
func TestSessionAwareScaleMonitor_ScaleUpOnSessionRamp(t *testing.T) {
	r := &scaleDecisionRecorder{}
	r.have.Store(2) // start at min

	// softMax=4 min=2 max=8. Ramp session count 0..32 and assert the
	// pool reaches 8 (ceiling) by ramp end.
	for sessions := 0; sessions <= 32; sessions += 4 {
		r.reconcile(sessions, 4, 2, 8)
	}
	if got := r.have.Load(); got != 8 {
		t.Errorf("final pool size = %d, want 8 (should reach max)", got)
	}
	if got := r.adds.Load(); got == 0 {
		t.Error("adds counter = 0, want > 0 (never grew)")
	}
}

// TestSessionAwareScaleMonitor_ScaleDownOnSessionRetire drives session
// count downward from a saturated state and asserts the recorder
// shrinks back to the floor.
func TestSessionAwareScaleMonitor_ScaleDownOnSessionRetire(t *testing.T) {
	r := &scaleDecisionRecorder{}
	r.have.Store(8) // start at max

	for sessions := 32; sessions >= 0; sessions -= 4 {
		r.reconcile(sessions, 4, 2, 8)
	}
	if got := r.have.Load(); got != 2 {
		t.Errorf("final pool size = %d, want 2 (should reach min floor)", got)
	}
	if got := r.removes.Load(); got == 0 {
		t.Error("removes counter = 0, want > 0 (never shrunk)")
	}
}

// TestSessionAwareScaleMonitor_ConfigUpdateResizes fixes session count
// and changes softMax mid-run; the next reconcile must retarget.
func TestSessionAwareScaleMonitor_ConfigUpdateResizes(t *testing.T) {
	r := &scaleDecisionRecorder{}
	r.have.Store(2)

	// sessions=20, softMax=4 → target=ceil(40/4)=10, clamped to max=8.
	r.reconcile(20, 4, 2, 8)
	if got := r.have.Load(); got != 8 {
		t.Fatalf("after first reconcile: pool=%d, want 8", got)
	}

	// softMax doubles to 8 → target=ceil(40/8)=5. Pool should shrink.
	r.reconcile(20, 8, 2, 8)
	if got := r.have.Load(); got != 5 {
		t.Errorf("after softMax bump: pool=%d, want 5", got)
	}
}

// TestSessionAwareScaleMonitor_OnConfigStoresSnapshot exercises the
// OnConfig callback: the atomic snapshot is loadable and reflects
// every field. Runs against a real monitor instance so any drift in
// the OnConfig↔tick contract is caught.
func TestSessionAwareScaleMonitor_OnConfigStoresSnapshot(t *testing.T) {
	m := NewSessionAwareScaleMonitor(nil)

	// Nil config is ignored — no snapshot stored, no panic.
	m.OnConfig(nil)
	if got := m.config.Load(); got != nil {
		t.Errorf("OnConfig(nil) stored %+v, want no store", got)
	}

	// Valid config populates all three fields.
	m.OnConfig(&spb.SessionClientConfiguration_ChannelPoolConfiguration{
		MinServerCount:        3,
		MaxServerCount:        12,
		PerServerSessionCount: 6,
	})
	snap := m.config.Load()
	if snap == nil {
		t.Fatal("OnConfig(valid) stored nil")
	}
	if snap.min != 3 || snap.max != 12 || snap.softMax != 6 {
		t.Errorf("snapshot = {min=%d max=%d softMax=%d}, want {3 12 6}",
			snap.min, snap.max, snap.softMax)
	}
}

// TestSessionAwareScaleMonitor_StartStopExitsCleanly verifies the
// production Start/Stop lifecycle: Start spawns the tick goroutine,
// Stop cancels the context and blocks on the goroutine's exit via
// the done channel. If either half were racy, Stop would either
// deadlock (goroutine never exits) or return before the goroutine
// finishes (Stop returning while the tick is mid-formula).
func TestSessionAwareScaleMonitor_StartStopExitsCleanly(t *testing.T) {
	prevInterval := SessionAwareTickInterval
	SessionAwareTickInterval = 10 * time.Millisecond
	t.Cleanup(func() { SessionAwareTickInterval = prevInterval })

	m := NewSessionAwareScaleMonitor(nil)
	// No config set — tick short-circuits, so a nil pool is safe.
	m.Start(context.Background())

	// Let a couple of ticks land.
	time.Sleep(30 * time.Millisecond)

	// Stop must return promptly (< 1s is generous) and the done
	// channel must be closed by the goroutine's defer.
	stopDone := make(chan struct{})
	go func() {
		m.Stop()
		close(stopDone)
	}()
	select {
	case <-stopDone:
	case <-time.After(time.Second):
		t.Fatal("Stop did not return within 1s (goroutine leak)")
	}

	// Double-Stop must be a no-op, not a panic.
	m.Stop()
}

// TestSessionAwareScaleMonitor_TickForTest_NoConfigIsNoOp asserts the
// tick short-circuits before touching the pool when no config has been
// received. Uses the test-only constructor so the ticker is disabled
// and we can drive tick manually. A nil pool must be safe here —
// tick() returns before calling pool.TotalStreamCount, so no panic.
func TestSessionAwareScaleMonitor_TickForTest_NoConfigIsNoOp(t *testing.T) {
	m := NewSessionAwareScaleMonitorForTest(nil)
	m.Start(context.Background())
	defer m.Stop()

	// If tick didn't short-circuit on nil config, this would panic on
	// pool.TotalStreamCount(). The test passing means short-circuit
	// works.
	m.TickForTest()
}

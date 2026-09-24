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
	"errors"
	"testing"
	"time"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// --- latencyHist ------------------------------------------------------------

func TestLatencyHist_RecordAndSnapshot(t *testing.T) {
	var h latencyHist
	// Record 100 samples uniformly across 4 orders of magnitude. p50/p95/p99
	// should land in monotonically-increasing buckets.
	for i := 0; i < 25; i++ {
		h.record(1 * time.Microsecond)
	}
	for i := 0; i < 25; i++ {
		h.record(1 * time.Millisecond)
	}
	for i := 0; i < 25; i++ {
		h.record(10 * time.Millisecond)
	}
	for i := 0; i < 25; i++ {
		h.record(100 * time.Millisecond)
	}
	p50, p95, p99, n := h.snapshot()
	if n != 100 {
		t.Errorf("n = %d, want 100", n)
	}
	if !(p50 < p95 && p95 <= p99) {
		t.Errorf("percentiles must be non-decreasing: p50=%v p95=%v p99=%v", p50, p95, p99)
	}
	// p50 falls at the boundary between the 1µs and 1ms clusters. With
	// log2 buckets + linear-in-bucket interpolation the reported value
	// is the top of bucket 19 (≈1.048ms). Widen to include that.
	if p50 < time.Microsecond || p50 > 2*time.Millisecond {
		t.Errorf("p50 = %v, want in [1µs, 2ms]", p50)
	}
	// p99 should land in the 100ms bucket range.
	if p99 < 10*time.Millisecond {
		t.Errorf("p99 = %v, want ≥10ms", p99)
	}
}

func TestLatencyHist_IgnoresNonPositive(t *testing.T) {
	var h latencyHist
	h.record(0)
	h.record(-1 * time.Second)
	_, _, _, n := h.snapshot()
	if n != 0 {
		t.Errorf("n = %d, want 0 for non-positive records", n)
	}
}

func TestLatencyHist_SnapshotEmptyReturnsZero(t *testing.T) {
	var h latencyHist
	p50, p95, p99, n := h.snapshot()
	if p50 != 0 || p95 != 0 || p99 != 0 || n != 0 {
		t.Errorf("empty snapshot = (%v,%v,%v,%d), want all zero", p50, p95, p99, n)
	}
}

func TestLatencyHist_HandlesTailOverflow(t *testing.T) {
	var h latencyHist
	// A duration beyond the highest bucket (~1000s) must land in the last
	// bucket, not panic or wrap.
	h.record(time.Duration(1<<62) * time.Nanosecond)
	_, _, _, n := h.snapshot()
	if n != 1 {
		t.Errorf("n = %d, want 1", n)
	}
}

func TestInterpLatencyPercentile_LinearInBucket(t *testing.T) {
	// Two samples in bucket 10 (covers [1024ns, 2048ns)). Target p50 falls
	// mid-bucket → ~1536ns.
	counts := make([]uint64, latencyHistBuckets)
	counts[10] = 2
	got := interpLatencyPercentile(counts, 2, 50)
	if got < 1024 || got > 2048 {
		t.Errorf("p50 = %v ns, want in [1024, 2048]", int64(got))
	}
}

func TestInterpLatencyPercentile_EdgeFallback(t *testing.T) {
	// Force the "target rounded above cum" fallback: n=1, target computed as
	// max(1, floor(n*pct/100)). With pct=100, target=1 and cum reaches
	// exactly 1 in bucket 5, so the normal path returns. Use pct>100 to
	// stress the fallback — interpLatencyPercentile is only called with
	// pct∈{50,95,99} in production, so we just verify it doesn't panic.
	counts := make([]uint64, latencyHistBuckets)
	counts[5] = 1
	got := interpLatencyPercentile(counts, 1, 200) // beyond spec range
	if got == 0 {
		t.Errorf("expected non-zero fallback, got 0")
	}
}

// --- lifetime ring ----------------------------------------------------------

func TestRecordLifetime_ClampsNonPositive(t *testing.T) {
	p := newTestPool(t, 1, 10)
	p.recordLifetime(0)
	p.recordLifetime(-1 * time.Second)
	if got := len(p.snapshotLifetimes()); got != 0 {
		t.Errorf("snapshotLifetimes len = %d, want 0", got)
	}
}

func TestRecordLifetime_RingCaps(t *testing.T) {
	p := newTestPool(t, 1, 10)
	// Fill to cap + 5. Oldest 5 must drop.
	for i := 0; i < maxLifetimes+5; i++ {
		p.recordLifetime(time.Duration(i+1) * time.Millisecond)
	}
	snap := p.snapshotLifetimes()
	if len(snap) != maxLifetimes {
		t.Fatalf("len = %d, want %d", len(snap), maxLifetimes)
	}
	// First surviving sample is the 6th one appended → 6ms.
	if snap[0] != 6*time.Millisecond {
		t.Errorf("snap[0] = %v, want 6ms (ring must drop oldest 5)", snap[0])
	}
	// Last is the newest → (maxLifetimes+5)ms.
	if want := time.Duration(maxLifetimes+5) * time.Millisecond; snap[len(snap)-1] != want {
		t.Errorf("snap[last] = %v, want %v", snap[len(snap)-1], want)
	}
}

// --- time series ------------------------------------------------------------

func TestRecordTimeSeries_ComputesRates(t *testing.T) {
	p := newTestPool(t, 1, 10)
	// Inject a session with baseline counters.
	sh := injectActiveSession(t, p, "s1", time.Now())
	sh.session.okRpcs.Store(100)
	sh.session.errorRpcs.Store(10)

	// First sample: no prior → rates are 0.
	p.recordTimeSeries()
	snap := p.snapshotTimeSeries()
	if len(snap) != 1 {
		t.Fatalf("after first record, snapshot len = %d, want 1", len(snap))
	}
	if snap[0].OkPerSec != 0 || snap[0].ErrPerSec != 0 {
		t.Errorf("first sample rates = (%v, %v), want (0, 0)", snap[0].OkPerSec, snap[0].ErrPerSec)
	}
	if snap[0].Sessions != 1 {
		t.Errorf("Sessions = %d, want 1", snap[0].Sessions)
	}

	// Advance counters + record again. Delta rates fire.
	sh.session.okRpcs.Store(200)
	sh.session.errorRpcs.Store(15)
	// Sleep briefly so dt > 0 (recordTimeSeries divides by dt).
	time.Sleep(2 * time.Millisecond)
	p.recordTimeSeries()
	snap = p.snapshotTimeSeries()
	if len(snap) != 2 {
		t.Fatalf("after second record, len = %d, want 2", len(snap))
	}
	if snap[1].OkPerSec <= 0 {
		t.Errorf("OkPerSec = %v, want positive (delta 100 over ~2ms)", snap[1].OkPerSec)
	}
	if snap[1].ErrPerSec <= 0 {
		t.Errorf("ErrPerSec = %v, want positive (delta 5 over ~2ms)", snap[1].ErrPerSec)
	}
}

func TestRecordTimeSeries_RingCaps(t *testing.T) {
	p := newTestPool(t, 1, 10)
	for i := 0; i < maxTimeSeries+3; i++ {
		p.recordTimeSeries()
	}
	snap := p.snapshotTimeSeries()
	if len(snap) != maxTimeSeries {
		t.Errorf("len = %d, want %d (ring must cap)", len(snap), maxTimeSeries)
	}
}

// --- slow-vRPC log ----------------------------------------------------------

func TestRecordSlowVRpc_RingCaps(t *testing.T) {
	p := newTestPool(t, 1, 10)
	for i := 0; i < maxSlowVRpcs+7; i++ {
		p.recordSlowVRpc(SlowVRpcEvent{Method: "M", Latency: time.Duration(i+1) * time.Millisecond})
	}
	snap := p.snapshotSlowVRpcs()
	if len(snap) != maxSlowVRpcs {
		t.Fatalf("len = %d, want %d", len(snap), maxSlowVRpcs)
	}
	// Newest is (maxSlowVRpcs+7)ms; oldest surviving is 8ms.
	if snap[0].Latency != 8*time.Millisecond {
		t.Errorf("snap[0].Latency = %v, want 8ms", snap[0].Latency)
	}
	want := time.Duration(maxSlowVRpcs+7) * time.Millisecond
	if snap[len(snap)-1].Latency != want {
		t.Errorf("snap[last].Latency = %v, want %v", snap[len(snap)-1].Latency, want)
	}
}

type fakeVRpcDesc struct{ method string }

func (f fakeVRpcDesc) Method() string                     { return f.method }
func (f fakeVRpcDesc) Encode(interface{}) ([]byte, error) { return nil, nil }
func (f fakeVRpcDesc) Decode([]byte) (interface{}, error) { return nil, nil }

func TestRecordCheckoutFailure_SkipsUnderThreshold(t *testing.T) {
	p := newTestPool(t, 1, 10)
	// Simulate a fast checkout failure. Threshold is 10ms by default.
	p.recordCheckoutFailure(time.Now(), fakeVRpcDesc{method: "M"}, context.DeadlineExceeded)
	if got := len(p.snapshotSlowVRpcs()); got != 0 {
		t.Errorf("under-threshold failure recorded: got %d slow events, want 0", got)
	}
}

func TestRecordCheckoutFailure_ClassifiesErr(t *testing.T) {
	cases := []struct {
		name     string
		err      error
		wantCode string
	}{
		{"DeadlineExceeded", context.DeadlineExceeded, "DeadlineExceeded"},
		{"Canceled", context.Canceled, "Canceled"},
		{"gRPC Unavailable", status.Error(codes.Unavailable, "boom"), "Unavailable"},
		{"unknown Go error", errors.New("mystery"), "Unknown"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			p := newTestPool(t, 1, 10)
			// Backdate checkoutStart so poolWait > threshold.
			start := time.Now().Add(-time.Second)
			p.recordCheckoutFailure(start, fakeVRpcDesc{method: "M"}, tc.err)
			snap := p.snapshotSlowVRpcs()
			if len(snap) != 1 {
				t.Fatalf("len = %d, want 1", len(snap))
			}
			if snap[0].ErrCode != tc.wantCode {
				t.Errorf("ErrCode = %q, want %q", snap[0].ErrCode, tc.wantCode)
			}
			if snap[0].Session != "" {
				t.Errorf("Session = %q, want empty (checkout failure has no session)", snap[0].Session)
			}
		})
	}
}

// --- pick history / AFE counts ---------------------------------------------

func TestSnapshotAfePickCounts_ReturnsCopy(t *testing.T) {
	p := newTestPool(t, 1, 10)
	p.recordPickDecision(PickDecision{Winner: AfeID(42), Reason: "test"}, "least-inflight")
	p.recordPickDecision(PickDecision{Winner: AfeID(42), Reason: "test"}, "least-inflight")
	p.recordPickDecision(PickDecision{Winner: AfeID(7), Reason: "test"}, "least-inflight")

	snap := p.snapshotAfePickCounts()
	if snap[42] != 2 {
		t.Errorf("counts[42] = %d, want 2", snap[42])
	}
	if snap[7] != 1 {
		t.Errorf("counts[7] = %d, want 1", snap[7])
	}

	// Mutating the snapshot must not touch the pool's live map.
	snap[42] = 999
	live := p.snapshotAfePickCounts()
	if live[42] != 2 {
		t.Errorf("mutating snapshot altered live counts: live[42] = %d, want 2", live[42])
	}
}

func TestRecordPickDecision_ZeroWinnerNotCounted(t *testing.T) {
	p := newTestPool(t, 1, 10)
	// Winner == 0 marks "no-candidates" — must not populate afePickCounts.
	p.recordPickDecision(PickDecision{Reason: "no-candidates"}, "least-inflight")
	snap := p.snapshotAfePickCounts()
	if len(snap) != 0 {
		t.Errorf("afePickCounts populated with zero winner: %+v", snap)
	}
	// But the ring buffer still records the event.
	if got := len(p.snapshotPickHistory()); got != 1 {
		t.Errorf("pickHistory len = %d, want 1 (no-candidate events must still be logged)", got)
	}
}

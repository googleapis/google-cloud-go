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

	spb "cloud.google.com/go/bigtable/apiv2/bigtablepb"
)

// injectActiveOnAfe is injectActiveSession but stamps the session's PeerInfo
// with the given AFE id before registering it with sessionList so the
// two-tier picker sees the correct bucket.
func injectActiveOnAfe(t *testing.T, p *SessionPoolImpl, name string, afe AfeID) *SessionHandle {
	t.Helper()
	stream := newFakeStream()
	sh := newSessionHandle(nil, time.Now())
	s := NewSession(name, stream, SessionHooks{
		OnStart:  p.onStart,
		OnActive: func(_ *Session) { p.onActive(sh) },
		OnClose:  func(_ *Session, err error) { p.onClose(sh, err) },
	}, SessionTypeTable)
	s.state.Store(int32(StateReady))
	// Stamp PeerInfo BEFORE registering — sessionList reads AfeID() at
	// OnSessionStarted, matching production's sync-PeerInfo invariant.
	s.peerInfo.Store(&spb.PeerInfo{ApplicationFrontendId: int64(afe)})
	sh.session = s

	p.sl.OnSessionStarted(sh)
	return sh
}

// checkoutAndRelease drives one round-trip through the two-tier picker:
// CheckoutSession → simulated Invoke return (DecOutstanding + latency
// record) plus simulated drainSlot effects (ReleaseToPool + signalFree —
// what SessionHandle.onSlotDrained would fire in production under v3).
// These tests bypass Session entirely (no real activeRPC slot to drain),
// so the simulator collapses both phases into one call. Skips the real
// Invoke path so tests don't depend on a fake vRPC responder.
func checkoutAndRelease(t *testing.T, p *SessionPoolImpl, recordLatency time.Duration, ok bool) *SessionHandle {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	sh, err := p.CheckoutSession(ctx)
	if err != nil {
		t.Fatalf("CheckoutSession: %v", err)
	}
	sh.DecOutstanding()
	p.sl.ReleaseToPool(sh)
	if recordLatency > 0 {
		p.sl.RecordVRpcOutcome(sh, recordLatency, 0, ok)
	}
	p.signalFree()
	return sh
}

// TestPool_LeastInFlight_FansOutAcrossAFEs verifies the default picker
// spreads traffic across per-AFE buckets rather than concentrating on
// one AFE. Distribution should be roughly even under LeastInFlight when
// every AFE has the same number of idle sessions.
func TestPool_LeastInFlight_FansOutAcrossAFEs(t *testing.T) {
	p := newTestPool(t, 1, 30)

	// 3 AFEs × 2 sessions each = 6 sessions total.
	afes := []AfeID{101, 202, 303}
	for _, a := range afes {
		injectActiveOnAfe(t, p, "s", a)
		injectActiveOnAfe(t, p, "s", a)
	}

	counts := map[AfeID]int{}
	const N = 900 // 3-way fair expected value = 300.
	for i := 0; i < N; i++ {
		sh := checkoutAndRelease(t, p, 0, true)
		counts[sh.session.AfeID()]++
	}

	for _, a := range afes {
		got := counts[a]
		if got < 220 || got > 380 {
			t.Errorf("AFE %d picked %d/%d times, want ~300 ±25%%", a, got, N)
		}
	}
}

// TestPool_LeastLatency_PrefersLowCostAFE verifies that once
// LeastLatencyAfePicker is engaged and per-AFE PeakEwma has warmed up,
// picks steer toward the AFE with lower e2e cost. Two AFEs, one seeded
// with a much lower latency history.
func TestPool_LeastLatency_PrefersLowCostAFE(t *testing.T) {
	p := newTestPool(t, 1, 20)

	// Switch picker to LeastLatency (subset-size = 2 = both AFEs).
	p.picker = NewLeastLatencyAfePicker(2, true)

	slow := injectActiveOnAfe(t, p, "slow", 1)
	fast := injectActiveOnAfe(t, p, "fast", 2)

	// Warm the per-AFE PeakEwma so the picker has real cost signal.
	// Both AFEs get 20 OK samples; slow at 100ms, fast at 5ms.
	for i := 0; i < 20; i++ {
		p.sl.RecordVRpcOutcome(slow, 100*time.Millisecond, 0, true)
		p.sl.RecordVRpcOutcome(fast, 5*time.Millisecond, 0, true)
	}

	counts := map[AfeID]int{}
	const N = 400
	for i := 0; i < N; i++ {
		sh := checkoutAndRelease(t, p, 0, true)
		counts[sh.session.AfeID()]++
	}

	if counts[2] <= counts[1] {
		t.Errorf("fast AFE picks (%d) should exceed slow AFE picks (%d)", counts[2], counts[1])
	}
	// Expect strong preference — fast AFE should win ≥ 80% under K=2 =
	// full-scan on 2 candidates (deterministic min-cost).
	if float64(counts[2])/float64(N) < 0.90 {
		t.Errorf("fast AFE share = %.2f%%, want ≥ 90%%", 100*float64(counts[2])/float64(N))
	}
}

// TestPool_LeastLatency_IgnoresFailingAFELatency verifies the OK-gate
// on per-AFE PeakEwma updates: a fast-failing AFE cannot pretend to be
// the fastest by feeding synthetic 1ns non-OK samples. Per the
// (SessionList.java:181-187).
func TestPool_LeastLatency_IgnoresFailingAFELatency(t *testing.T) {
	p := newTestPool(t, 1, 20)

	p.picker = NewLeastLatencyAfePicker(2, true)

	failing := injectActiveOnAfe(t, p, "failing", 1)
	healthy := injectActiveOnAfe(t, p, "healthy", 2)

	// The failing AFE gets 50 non-OK samples with an absurdly low
	// latency. If the OK-gate is broken, these poison PeakEwma to 1ns
	// and the picker will always pick this AFE.
	for i := 0; i < 50; i++ {
		p.sl.RecordVRpcOutcome(failing, 1*time.Nanosecond, 0, false)
	}
	// Healthy AFE gets legitimate 10ms OK samples.
	for i := 0; i < 20; i++ {
		p.sl.RecordVRpcOutcome(healthy, 10*time.Millisecond, 0, true)
	}

	counts := map[AfeID]int{}
	const N = 200
	for i := 0; i < N; i++ {
		sh := checkoutAndRelease(t, p, 0, true)
		counts[sh.session.AfeID()]++
	}

	// The failing AFE has E2eCost == afeE2eEwmaSeed (no OK samples ever
	// landed, so the seed is untouched), the healthy AFE has
	// E2eCost == ~10ms. With the seed at 1ms both AFEs look plausible
	// to LeastLatencyPicker on first pick — but the point of THIS test
	// is confirming the OK-gate: the failing AFE's PeakEwma stays at
	// the seed, not 1ns. Either winner is fine; what we assert is that
	// the failing AFE's tracker didn't get polluted by the non-OK
	// samples.
	if got, want := p.sl.afeHandles[1].e2eEwma.Value(), float64(afeE2eEwmaSeed); got != want {
		t.Errorf("failing-AFE e2eEwma = %g, want %g (seed unchanged; OK-gate broken?)", got, want)
	}
	if got := p.sl.afeHandles[2].e2eEwma.Value(); got == 0 {
		t.Errorf("healthy-AFE e2eEwma = 0, want > 0 (OK samples not recorded)")
	}
	_ = counts // pick distribution not asserted; the OK-gate invariant is
	// what matters.
}

// TestPool_UnknownAFE_BucketedAtZero verifies the AfeID=0 sentinel path
// used when sessions handshake without a peer-info header (older
// backends / test injection helpers that skip peer info). Those
// sessions still get picked, just from the shared unknown bucket.
func TestPool_UnknownAFE_BucketedAtZero(t *testing.T) {
	p := newTestPool(t, 1, 20)

	// injectActiveSession (no On-Afe variant) — leaves PeerInfo nil so
	// AfeID() returns 0.
	injectActiveSession(t, p, "no-peer-info", time.Now())

	sh := checkoutAndRelease(t, p, 0, true)
	if got := sh.session.AfeID(); got != 0 {
		t.Errorf("AfeID = %d, want 0 (unknown-bucket sentinel)", got)
	}
	// Snapshot exposes the bucket.
	rows := p.sl.Snapshot()
	if len(rows) != 1 || rows[0].ID != 0 {
		t.Errorf("Snapshot = %+v, want a single AFE 0 row", rows)
	}
}

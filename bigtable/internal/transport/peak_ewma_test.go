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
	"math"
	"sync"
	"testing"
	"time"
)

func TestPeakEwma_UnseededValueIsZero(t *testing.T) {
	e := NewPeakEwma(1 * time.Second)
	if got := e.Value(); got != 0 {
		t.Errorf("Value() before any Update = %v, want 0", got)
	}
}

func TestPeakEwma_SeededValueBeforeUpdate(t *testing.T) {
	seed := 750 * time.Microsecond
	e := NewPeakEwmaSeeded(1*time.Second, seed)
	if got := e.Value(); got != float64(seed) {
		t.Errorf("Seeded Value() = %v, want %v", got, float64(seed))
	}
}

// TestPeakEwma_PeakSnapOnHigherSample pins the "peak" in PeakEwma:
// samples strictly higher than the current value snap up immediately
// with no blend. Without this, the tracker degenerates into a plain
// symmetric EWMA and a new AFE with one lucky-fast sample would look
// instantly cheap to the least-latency picker.
func TestPeakEwma_PeakSnapOnHigherSample(t *testing.T) {
	e := NewPeakEwmaSeeded(1*time.Second, 1*time.Millisecond)
	e.Update(50 * time.Millisecond)
	if got := e.Value(); got != float64(50*time.Millisecond) {
		t.Errorf("Value() after higher sample = %v, want peak-snap to %v",
			got, float64(50*time.Millisecond))
	}
}

// TestPeakEwma_SeedRetainedOnLowerFirstSample pins the contract that
// a seeded PeakEwma's first Update does NOT snap to the sample when
// the sample is lower than the seed. The seed remains the
// authoritative baseline; the sample decays into it per e^(-dt/tau).
//
// Regression this test prevents: an earlier impl used a
// `lastUpdate.IsZero()` branch that snapped to the sample on the
// first Update, discarding the seed and defeating cold-start
// weighting.
func TestPeakEwma_SeedRetainedOnLowerFirstSample(t *testing.T) {
	seed := 1 * time.Millisecond
	sample := 100 * time.Microsecond // below seed
	// Large tau so decay in the microseconds between construction and
	// Update is negligible; value should stay very close to seed.
	e := NewPeakEwmaSeeded(1*time.Hour, seed)
	e.Update(sample)
	got := e.Value()
	// Sample is 10% of seed. With near-zero elapsed vs tau=1h, decay ~ 1
	// so value ~ seed * 1 + sample * ~0 ~= seed. Allow a 1% band for
	// non-zero elapsed.
	if math.Abs(got-float64(seed)) > float64(seed)*0.01 {
		t.Errorf("Value() = %v after lower-sample Update, want ~seed (%v) — seed must not be snapped away",
			got, float64(seed))
	}
	if got == float64(sample) {
		t.Error("Value() snapped to lower sample; seed was discarded")
	}
}

// TestPeakEwma_NonPositiveSampleIgnored pins the rule that zero or
// negative latencies are ignored, not blended in. A negative rtt is a
// bug at the source (clock skew, underflow) and must not corrupt the
// tracker.
func TestPeakEwma_NonPositiveSampleIgnored(t *testing.T) {
	const seed = 5 * time.Millisecond
	e := NewPeakEwmaSeeded(1*time.Second, seed)
	e.Update(0)
	e.Update(-1 * time.Millisecond)
	if got := e.Value(); got != float64(seed) {
		t.Errorf("Value() after non-positive updates = %v, want unchanged %v",
			got, float64(seed))
	}
}

// TestPeakEwma_ConvergesOnConstantSample proves the tracker is exact
// on a constant stream: for any decay weight, w*L + (1-w)*L == L. The
// peak-step is a no-op when sample == current, so the decay-step
// dominates. Holds regardless of the wall-clock gap between updates.
func TestPeakEwma_ConvergesOnConstantSample(t *testing.T) {
	const L = 5 * time.Millisecond
	e := NewPeakEwma(100 * time.Millisecond)
	for i := 0; i < 20; i++ {
		e.Update(L)
	}
	if got := e.Value(); got != float64(L) {
		t.Errorf("Value() after 20 identical updates = %v, want exactly %v",
			got, float64(L))
	}
}

// TestPeakEwma_ValueBoundedByObservedSamples pins the convex-
// combination invariant on the decay-step: after any sequence of
// samples, Value stays within [min, max] of samples seen. Peak-step
// preserves this trivially (snap-up to an observed sample); decay-step
// preserves it via the blend. Guards against a math typo that would
// let value overshoot.
func TestPeakEwma_ValueBoundedByObservedSamples(t *testing.T) {
	samples := []time.Duration{
		3 * time.Millisecond,
		9 * time.Millisecond,
		5 * time.Millisecond,
		7 * time.Millisecond,
	}
	var lo, hi time.Duration = samples[0], samples[0]
	for _, s := range samples[1:] {
		if s < lo {
			lo = s
		}
		if s > hi {
			hi = s
		}
	}

	e := NewPeakEwma(50 * time.Millisecond)
	for _, s := range samples {
		e.Update(s)
		got := e.Value()
		if got < float64(lo) || got > float64(hi) {
			t.Errorf("Value() = %v after sample %v, want in [%v, %v]",
				got, s, float64(lo), float64(hi))
		}
	}
}

// TestPeakEwma_ValueIsPureRead ensures Value() does not mutate state:
// calling it twice with no interleaving Update must return the exact
// same float64. Guards against a future refactor that folds decay into
// the reader (which would make Value time-sensitive and break the
// picker's assumption that a snapshot is stable across scoring loops).
func TestPeakEwma_ValueIsPureRead(t *testing.T) {
	e := NewPeakEwma(1 * time.Second)
	e.Update(4 * time.Millisecond)
	first := e.Value()
	second := e.Value()
	if first != second {
		t.Errorf("Value() mutated state: first=%v, second=%v", first, second)
	}
}

// TestPeakEwma_LargeGapDominatesLowerSample verifies the decay math on
// the decay-step: with dt >> tau, weight → 0 and Value tracks the
// (lower) new sample. Peak-snap doesn't fire because the second sample
// is smaller than the first. Uses tau=1ms + 50ms sleep so residual
// weight on the first sample is ~e^-50 ≈ 2e-22.
func TestPeakEwma_LargeGapDominatesLowerSample(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping under -short: relies on a real 50ms sleep")
	}
	const (
		tau        = 1 * time.Millisecond
		firstSamp  = 100 * time.Millisecond
		secondSamp = 5 * time.Millisecond
	)
	e := NewPeakEwma(tau)
	e.Update(firstSamp)
	time.Sleep(50 * time.Millisecond)
	e.Update(secondSamp)

	got := e.Value()
	tolerance := float64(secondSamp) * 0.01
	if math.Abs(got-float64(secondSamp)) > tolerance {
		t.Errorf("Value() = %v after wide gap + lower sample, want within %v of %v",
			got, tolerance, float64(secondSamp))
	}
}

// TestPeakEwma_ZeroTauSafe pins the defensive guard against a
// NaN-producing edge case: tau == 0 combined with dt == 0 would
// otherwise evaluate exp(-0/0) = exp(NaN) = NaN. With the tau <= 0
// branch collapsing decay to 0, the new sample fully replaces the old
// — the natural limit as tau → 0. Reported by gemini-code-assist on
// PR #20187.
func TestPeakEwma_ZeroTauSafe(t *testing.T) {
	e := NewPeakEwmaSeeded(0, 5*time.Millisecond)
	e.Update(3 * time.Millisecond)
	got := e.Value()
	if math.IsNaN(got) {
		t.Fatal("Value() = NaN after tau=0 Update")
	}
	if got != float64(3*time.Millisecond) {
		t.Errorf("Value() = %v after tau=0, want new sample %v (zero-memory limit)",
			got, float64(3*time.Millisecond))
	}
}

// TestPeakEwma_BackwardClockNoOvershoot pins the defensive clamp
// against a backward clock jump: dt < 0 would otherwise produce a
// blend weight > 1 (exp of a positive number), which would let the
// new sample push Value outside [min,max] of observed samples. Also
// addresses the second part of gemini-code-assist's PR #20187 review.
//
// We can't rewind the wall clock in a test, so instead reach into the
// state and set lastUpdate to the future; the next Update's dt is
// then negative and must be clamped.
func TestPeakEwma_BackwardClockNoOvershoot(t *testing.T) {
	const (
		seed   = 10 * time.Millisecond
		sample = 3 * time.Millisecond // must be < seed so peak-snap doesn't fire
	)
	e := NewPeakEwmaSeeded(1*time.Second, seed)
	// Simulate a wall clock rewind by pushing lastUpdate 1 minute
	// ahead. Without the clamp, dt = -1min → decay = exp(+60/1) → huge;
	// value = seed*huge + sample*(1-huge) → unbounded (or negative).
	e.mu.Lock()
	e.lastUpdate = time.Now().Add(1 * time.Minute)
	e.mu.Unlock()

	e.Update(sample)
	got := e.Value()
	// With dt clamped to 0, decay = 1, so value = seed*1 + sample*0 = seed.
	if math.IsNaN(got) || math.IsInf(got, 0) {
		t.Fatalf("Value() = %v after backward-clock Update; want finite", got)
	}
	if got < float64(sample) || got > float64(seed) {
		t.Errorf("Value() = %v, want in [%v, %v] (backward clock must not push value outside sample range)",
			got, float64(sample), float64(seed))
	}
}

// TestPeakEwma_ConcurrentUpdatesRaceSafe stresses the mutex on Update
// and Value: N goroutines hammer the same PeakEwma with a mix of writes
// and reads. Meaningful only under `go test -race` — the tolerance is
// wide because the final value depends on interleaving, but Value MUST
// stay within the sample range no matter what.
func TestPeakEwma_ConcurrentUpdatesRaceSafe(t *testing.T) {
	const (
		writers        = 8
		readers        = 4
		updatesPerGoro = 500
		tau            = 10 * time.Millisecond
		sample         = 3 * time.Millisecond
	)
	e := NewPeakEwma(tau)

	var writerWG, readerWG sync.WaitGroup
	writerWG.Add(writers)
	readerWG.Add(readers)
	stop := make(chan struct{})

	for i := 0; i < writers; i++ {
		go func() {
			defer writerWG.Done()
			for j := 0; j < updatesPerGoro; j++ {
				e.Update(sample)
			}
		}()
	}
	for i := 0; i < readers; i++ {
		go func() {
			defer readerWG.Done()
			for {
				select {
				case <-stop:
					return
				default:
					_ = e.Value()
				}
			}
		}()
	}

	writerWG.Wait() // all writers done → at least one Update landed.
	close(stop)     // release readers.
	readerWG.Wait()

	// All samples were `sample`, so the tracker must equal `sample`
	// exactly regardless of interleaving (constant-sample invariant).
	if got := e.Value(); got != float64(sample) {
		t.Errorf("Value() after concurrent constant-sample updates = %v, want %v",
			got, float64(sample))
	}
}

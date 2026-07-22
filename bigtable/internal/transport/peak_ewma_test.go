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

// TestPeakEwma_FirstUpdateOverridesSeed pins the "seed is authoritative
// only until the first Update" behavior documented on NewPeakEwmaSeeded.
// If this ever silently changed to blend the seed into the first sample,
// AFE picker cold-start behavior would drift for a full tau window.
func TestPeakEwma_FirstUpdateOverridesSeed(t *testing.T) {
	seed := 1 * time.Millisecond
	sample := 42 * time.Millisecond
	e := NewPeakEwmaSeeded(1*time.Second, seed)
	e.Update(sample)
	if got := e.Value(); got != float64(sample) {
		t.Errorf("Value() after first Update = %v, want %v (seed must be overridden)",
			got, float64(sample))
	}
}

// TestPeakEwma_ConvergesOnConstantSample proves the EWMA is exact on a
// constant stream: for any weight, w*L + (1-w)*L == L. This holds
// regardless of the wall-clock gap between updates, so the test is
// deterministic without time control.
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

// TestPeakEwma_ValueBoundedByObservedSamples pins the convex-combination
// invariant: the EWMA can never leave the [min, max] range of samples it
// has seen. Guards against a math typo (e.g. blending with a negative
// weight) that would let the value overshoot in either direction.
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

// TestPeakEwma_LargeGapDominatesWithNewSample verifies the decay math:
// when dt >> tau, weight = e^(-dt/tau) collapses toward 0 and Value
// tracks the newest sample. Uses tau=1ms and a 50ms real sleep so
// weight ≈ e^-50 ≈ 2e-22 — five orders of magnitude below the tolerance
// so scheduler jitter cannot flake the test.
func TestPeakEwma_LargeGapDominatesWithNewSample(t *testing.T) {
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
	// After a 50ms gap with tau=1ms the residual weight on firstSamp is
	// vanishing; Value should be within 1% of secondSamp.
	tolerance := float64(secondSamp) * 0.01
	if math.Abs(got-float64(secondSamp)) > tolerance {
		t.Errorf("Value() = %v after wide gap, want within %v of %v",
			got, tolerance, float64(secondSamp))
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

	// All samples were `sample`, so the EWMA must equal `sample` exactly
	// regardless of interleaving (constant-sample invariant).
	if got := e.Value(); got != float64(sample) {
		t.Errorf("Value() after concurrent constant-sample updates = %v, want %v",
			got, float64(sample))
	}
}

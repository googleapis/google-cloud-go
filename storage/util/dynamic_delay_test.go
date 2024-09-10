// Copyright 2024 Google LLC
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//      http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF

package util

import (
	"math"
	"math/rand"
	"testing"
	"time"

	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"
)

func applySamples(numSamples int, expectedValue float64, rnd *rand.Rand, d *Delay) int {
	var samplesOverThreshold int
	for i := 0; i < numSamples; i++ {
		randomDelay := time.Duration(-math.Log(rnd.Float64()) * expectedValue * float64(time.Second))
		if randomDelay > d.Value() {
			samplesOverThreshold++
			d.Increase()
		} else {
			d.Decrease()
		}
	}
	return samplesOverThreshold
}

func applySamplesWithUpdate(numSamples int, expectedValue float64, rnd *rand.Rand, d *Delay) {
	for i := 0; i < numSamples; i++ {
		randomDelay := time.Duration(-math.Log(rnd.Float64()) * expectedValue * float64(time.Second))
		d.Update(randomDelay)
	}
}

func TestNewDelay(t *testing.T) {
	d, err := NewDelay(1-0.01, 15, 1*time.Millisecond, 1*time.Millisecond, 1*time.Hour)
	if err != nil {
		t.Fatal(err)
	}

	want := &Delay{
		increaseFactor: 1.047294,
		decreaseFactor: 0.999533,
		minDelay:       1 * time.Millisecond,
		maxDelay:       1 * time.Hour,
		value:          1 * time.Millisecond,
	}

	if diff := cmp.Diff(d.increaseFactor, want.increaseFactor, cmpopts.EquateApprox(0, 0.000001)); diff != "" {
		t.Fatalf("unexpected diff (-got +want):\n%s", diff)
	}

	if diff := cmp.Diff(d.decreaseFactor, want.decreaseFactor, cmpopts.EquateApprox(0, 0.000001)); diff != "" {
		t.Fatalf("unexpected diff (-got +want):\n%s", diff)
	}

	if diff := cmp.Diff(d.minDelay, want.minDelay, cmpopts.EquateApprox(0, 0.000001)); diff != "" {
		t.Fatalf("unexpected diff (-got +want):\n%s", diff)
	}

	if diff := cmp.Diff(d.maxDelay, want.maxDelay, cmpopts.EquateApprox(0, 0.000001)); diff != "" {
		t.Fatalf("unexpected diff (-got +want):\n%s", diff)
	}

	if diff := cmp.Diff(d.value, want.value, cmpopts.EquateApprox(0, 0.000001)); diff != "" {
		t.Fatalf("unexpected diff (-got +want):\n%s", diff)
	}

	if d.mu == nil {
		t.Fatalf("unexpted mutex value")
	}
}

func TestConvergence99(t *testing.T) {
	// d should converge to the 99-percentile value.
	d, err := NewDelay(1-0.01, 15, 1*time.Millisecond, 1*time.Millisecond, 1*time.Hour)
	if err != nil {
		t.Fatal(err)
	}

	rnd := rand.New(rand.NewSource(1))

	// Warm up.
	applySamplesWithUpdate(1000, 0.005, rnd, d)

	// We would end up sending hedged calls at ~1% (between 0.2% and 5%).
	{
		samplesOverThreshold := applySamples(1000, 0.005, rnd, d)
		if samplesOverThreshold < (1000 * 0.002) {
			t.Errorf("samplesOverThreshold = %d < 1000*0.002", samplesOverThreshold)
		}
		if samplesOverThreshold > (1000 * 0.05) {
			t.Errorf("samplesOverThreshold = %d > 1000*0.05", samplesOverThreshold)
		}
	}

	// Apply samples from a different distribution.
	applySamplesWithUpdate(1000, 1, rnd, d)

	// delay.value should have now converged to the new distribution.
	{
		samplesOverThreshold := applySamples(1000, 1, rnd, d)
		if samplesOverThreshold < (1000 * 0.002) {
			t.Errorf("samplesOverThreshold = %d < 1000*0.002", samplesOverThreshold)
		}
		if samplesOverThreshold > (1000 * 0.05) {
			t.Errorf("samplesOverThreshold = %d > 1000*0.05", samplesOverThreshold)
		}
	}
}

func TestConvergence90(t *testing.T) {
	// d should converge to the 90-percentile value.
	d, err := NewDelay(1-0.1, 15, 1*time.Millisecond, 1*time.Millisecond, 1*time.Hour)
	if err != nil {
		t.Fatal(err)
	}

	rnd := rand.New(rand.NewSource(1))

	// Warm up.
	applySamplesWithUpdate(1000, 0.005, rnd, d)

	// We would end up sending hedged calls at ~10% (between 5% and 20%).
	{
		samplesOverThreshold := applySamples(1000, 0.005, rnd, d)
		if samplesOverThreshold < (1000 * 0.05) {
			t.Errorf("samplesOverThreshold = %d < 1000*0.05", samplesOverThreshold)
		}
		if samplesOverThreshold > (1000 * 0.2) {
			t.Errorf("samplesOverThreshold = %d > 1000*0.2", samplesOverThreshold)
		}
	}
}

func TestOverflow(t *testing.T) {
	d, err := NewDelay(1-0.1, 15, 1*time.Millisecond, 1*time.Millisecond, 1*time.Hour)
	if err != nil {
		t.Fatal(err)
	}

	n := 10000
	for i := 0; i < n; i++ {
		d.Increase()
	}
	t.Log(d.Value())
	for i := 0; i < 100*n; i++ {
		d.Decrease()
	}
	if got, want := d.Value(), 1*time.Millisecond; got != want {
		t.Fatalf("unexpected d.Value: got %v, want %v", got, want)
	}
}

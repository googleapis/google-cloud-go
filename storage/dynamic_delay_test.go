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

package storage

import (
	"fmt"
	"math"
	"math/rand"
	"sync"
	"testing"
	"time"

	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"
)

func applySamples(numSamples int, expectedValue float64, rnd *rand.Rand, d *dynamicDelay) int {
	var samplesOverThreshold int
	for i := 0; i < numSamples; i++ {
		randomDelay := time.Duration(-math.Log(rnd.Float64()) * expectedValue * float64(time.Second))
		if randomDelay > d.getValue() {
			samplesOverThreshold++
			d.increase()
		} else {
			d.decrease()
		}
	}
	return samplesOverThreshold
}

func applySamplesWithUpdate(numSamples int, expectedValue float64, rnd *rand.Rand, d *dynamicDelay) {
	for i := 0; i < numSamples; i++ {
		randomDelay := time.Duration(-math.Log(rnd.Float64()) * expectedValue * float64(time.Second))
		d.update(randomDelay)
	}
}

func TestNewDelay(t *testing.T) {
	d := newDynamicDelay(1-0.01, 15, 1*time.Millisecond, 1*time.Millisecond, 1*time.Hour)

	want := &dynamicDelay{
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
	d := newDynamicDelay(1-0.01, 15, 1*time.Millisecond, 1*time.Millisecond, 1*time.Hour)

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
	d := newDynamicDelay(1-0.1, 15, 1*time.Millisecond, 1*time.Millisecond, 1*time.Hour)

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
	d := newDynamicDelay(1-0.1, 15, 1*time.Millisecond, 1*time.Millisecond, 1*time.Hour)

	n := 10000
	// Should converge to maxDelay.
	for i := 0; i < n; i++ {
		d.increase()
	}
	if got, want := d.getValue(), 1*time.Hour; got != want {
		t.Fatalf("unexpected d.Value: got %v, want %v", got, want)
	}

	// Should converge to minDelay.
	for i := 0; i < 100*n; i++ {
		d.decrease()
	}
	if got, want := d.getValue(), 1*time.Millisecond; got != want {
		t.Fatalf("unexpected d.Value: got %v, want %v", got, want)
	}
}

func TestValidateDynamicDelayParams(t *testing.T) {
	testCases := []struct {
		name             string
		targetPercentile float64
		increaseRate     float64
		minDelay         time.Duration
		maxDelay         time.Duration
		expectErr        bool
	}{
		// Valid parameters
		{"valid", 0.5, 0.1, 1 * time.Second, 10 * time.Second, false},

		// Invalid targetPercentile
		{"invalid targetPercentile (< 0)", -0.1, 0.1, 1 * time.Second, 10 * time.Second, true},
		{"invalid targetPercentile (> 1)", 1.1, 0.1, 1 * time.Second, 10 * time.Second, true},

		// Invalid increaseRate
		{"invalid increaseRate (<= 0)", 0.5, 0, 1 * time.Second, 10 * time.Second, true},

		// Invalid delay combination
		{"invalid delay combination (minDelay >= maxDelay)", 0.5, 0.1, 10 * time.Second, 1 * time.Second, true},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			err := validateDynamicDelayParams(tc.targetPercentile, tc.increaseRate, tc.minDelay, tc.maxDelay)
			if tc.expectErr && err == nil {
				t.Errorf("Expected an error, but got none")
			}
			if !tc.expectErr && err != nil {
				t.Errorf("Unexpected error: %v", err)
			}
		})
	}
}

func applySamplesBucket(numSamples int, expectedValue float64, rnd *rand.Rand, b *bucketDelayManager, bucketName string) int {
	var samplesOverThreshold int
	for i := 0; i < numSamples; i++ {
		randomDelay := time.Duration(-math.Log(rnd.Float64()) * expectedValue * float64(time.Second))
		if randomDelay > b.getValue(bucketName) {
			samplesOverThreshold++
			b.increase(bucketName)
		} else {
			b.decrease(bucketName)
		}
	}
	return samplesOverThreshold
}

func applySamplesWithUpdateBucket(numSamples int, expectedValue float64, rnd *rand.Rand, b *bucketDelayManager, bucketName string) {
	for i := 0; i < numSamples; i++ {
		randomDelay := time.Duration(-math.Log(rnd.Float64()) * expectedValue * float64(time.Second))
		b.update(bucketName, randomDelay)
	}
}

func TestBucketDelayManager(t *testing.T) {
	b, err := newBucketDelayManager(0.99, 1.5, 100*time.Millisecond, 100*time.Millisecond, 10*time.Second)
	if err != nil {
		t.Errorf("while creating bucketDelayManager: %v", err)
	}

	t.Logf("testing")

	// Test increase and getValue
	b.increase("bucket1")
	delay1 := b.getValue("bucket1")
	if delay1 <= 100*time.Millisecond {
		t.Errorf("Expected delay for bucket1 to be > 100ms after increase, got %v", delay1)
	}

	// Test decrease and getValue
	b.decrease("bucket1")
	delay2 := b.getValue("bucket1")
	if delay2 >= delay1 {
		t.Errorf("Expected delay for bucket1 to be < %v after decrease, got %v", delay1, delay2)
	}

	// Test update with latency > current delay
	b.update("bucket2", 200*time.Millisecond)
	delay3 := b.getValue("bucket2")
	if delay3 <= 100*time.Millisecond {
		t.Errorf("Expected delay for bucket2 to be > 100ms after update with higher latency, got %v", delay3)
	}

	// Test update with latency < current delay
	b.update("bucket2", 50*time.Millisecond)
	delay4 := b.getValue("bucket2")
	if delay4 >= delay3 {
		t.Errorf("Expected delay for bucket2 to be < %v after update with lower latency, got %v", delay3, delay4)
	}
}

func TestBucketDelayManagerConcurrentAccess(t *testing.T) {
	b, err := newBucketDelayManager(1-0.1, 15, 1*time.Millisecond, 1*time.Millisecond, 1*time.Hour)
	if err != nil {
		t.Errorf("while creating bucketDelayManager: %v", err)
	}

	// Test concurrent access
	var wg sync.WaitGroup
	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			bucketName := fmt.Sprintf("bucket%d", i%3) // 3 buckets
			b.increase(bucketName)
			b.decrease(bucketName)
			b.update(bucketName, time.Duration(i)*time.Millisecond)
		}(i)
	}
	wg.Wait()

	// Check if the map size is as expected
	b.mu.Lock() // Lock to access the map safely
	defer b.mu.Unlock()
	if len(b.delays) != 3 {
		t.Errorf("Expected %d buckets in the map, but got %d", 3, len(b.delays))
	}
}

func TestBucketDelayManagerInvalidArgument(t *testing.T) {
	// Test with invalid targetPercentile
	_, err := newBucketDelayManager(1.1, 15, 1*time.Millisecond, 1*time.Hour, 2*time.Hour)
	if err == nil {
		t.Fatal("unexpected, should throw error as targetPercentile is greater than 1")
	}

	// Test with invalid increaseRate
	_, err = newBucketDelayManager(0.9, -1, 1*time.Millisecond, 1*time.Hour, 2*time.Hour)
	if err == nil {
		t.Fatal("unexpected, should throw error as increaseRate can't be negative")
	}

	// Test with invalid minDelay and maxDelay combination
	_, err = newBucketDelayManager(0.9, 15, 1*time.Millisecond, 2*time.Hour, 1*time.Hour)
	if err == nil {
		t.Fatal("unexpected, should throw error as minDelay is greater than maxDelay")
	}
}

func TestBucketDelayManagerOverflow(t *testing.T) {
	b, err := newBucketDelayManager(1-0.1, 15, 1*time.Millisecond, 1*time.Millisecond, 1*time.Hour)
	if err != nil {
		t.Errorf("while creating bucketDelayManager: %v", err)
	}

	bucketName := "testBucket"
	n := 10000

	// Should converge to maxDelay.
	for i := 0; i < n; i++ {
		b.increase(bucketName)
	}

	if got, want := b.getValue(bucketName), 1*time.Hour; got != want {
		t.Fatalf("unexpected delay value: got %v, want %v", got, want)
	}

	// Should converge to minDelay.
	for i := 0; i < 100*n; i++ {
		b.decrease(bucketName)
	}
	if got, want := b.getValue(bucketName), 1*time.Millisecond; got != want {
		t.Fatalf("unexpected delay value: got %v, want %v", got, want)
	}
}

func TestBucketDelayManagerConvergence90(t *testing.T) {
	b, err := newBucketDelayManager(1-0.1, 15, 1*time.Millisecond, 1*time.Millisecond, 1*time.Hour)
	if err != nil {
		t.Errorf("while creating bucketDelayManager: %v", err)
	}
	bucket1 := "bucket1"
	bucket2 := "bucket2"

	rnd := rand.New(rand.NewSource(1))

	// Warm up both buckets
	applySamplesWithUpdateBucket(1000, 0.005, rnd, b, bucket1)
	applySamplesWithUpdateBucket(1000, 0.005, rnd, b, bucket2)

	// Check convergence for bucket1
	{
		samplesOverThreshold := applySamplesBucket(1000, 0.005, rnd, b, bucket1)
		if samplesOverThreshold < (1000 * 0.05) {
			t.Errorf("bucket1: samplesOverThreshold = %d < 1000*0.05", samplesOverThreshold)
		}
		if samplesOverThreshold > (1000 * 0.2) {
			t.Errorf("bucket1: samplesOverThreshold = %d > 1000*0.2", samplesOverThreshold)
		}
	}

	// Check convergence for bucket2
	{
		samplesOverThreshold := applySamplesBucket(1000, 0.005, rnd, b, bucket2)
		if samplesOverThreshold < (1000 * 0.05) {
			t.Errorf("bucket2: samplesOverThreshold = %d < 1000*0.05", samplesOverThreshold)
		}
		if samplesOverThreshold > (1000 * 0.2) {
			t.Errorf("bucket2: samplesOverThreshold = %d > 1000*0.2", samplesOverThreshold)
		}
	}
}

func TestBucketDelayManagerMapSize(t *testing.T) {
	b, err := newBucketDelayManager(1-0.1, 15, 1*time.Millisecond, 1*time.Millisecond, 1*time.Hour)
	if err != nil {
		t.Errorf("while creating bucketDelayManager: %v", err)
	}
	// Add delays for multiple buckets
	numBuckets := 10
	for i := 0; i < numBuckets; i++ {
		bucketName := fmt.Sprintf("bucket%d", i)
		b.increase(bucketName)
	}

	// Check if the map size is as expected
	b.mu.Lock() // Lock to access the map safely
	defer b.mu.Unlock()
	if len(b.delays) != numBuckets {
		t.Errorf("Expected %d buckets in the map, but got %d", numBuckets, len(b.delays))
	}
}

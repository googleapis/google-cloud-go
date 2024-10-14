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
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package storage

import (
	"fmt"
	"math"
	"math/rand"
	"sync"
	"testing"
	"time"
)

func applySamplesBucket(numSamples int, expectedValue float64, rnd *rand.Rand, b *bucketDelay, bucketName string) int {
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

func applySamplesWithUpdateBucket(numSamples int, expectedValue float64, rnd *rand.Rand, b *bucketDelay, bucketName string) {
	for i := 0; i < numSamples; i++ {
		randomDelay := time.Duration(-math.Log(rnd.Float64()) * expectedValue * float64(time.Second))
		b.update(bucketName, randomDelay)
	}
}

func TestBucketDelay(t *testing.T) {
	b, err := newBucketDelay(0.99, 1.5, 100*time.Millisecond, 100*time.Millisecond, 10*time.Second)
	if err != nil {
		t.Errorf("while creating bucketDelay: %v", err)
	}

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

func TestBucketDelayConcurrentAccess(t *testing.T) {
	b, err := newBucketDelay(1-0.1, 15, 1*time.Millisecond, 1*time.Millisecond, 1*time.Hour)
	if err != nil {
		t.Errorf("while creating bucketDelay: %v", err)
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

func TestBucketDelayInvalidArgument(t *testing.T) {
	// Test with invalid targetPercentile
	_, err := newDynamicDelay(1.1, 15, 1*time.Millisecond, 1*time.Hour, 2*time.Hour)
	if err == nil {
		t.Fatal("unexpected, should throw error as targetPercentile is greater than 1")
	}

	// Test with invalid increaseRate
	_, err = newDynamicDelay(0.9, -1, 1*time.Millisecond, 1*time.Hour, 2*time.Hour)
	if err == nil {
		t.Fatal("unexpected, should throw error as increaseRate can't be negative")
	}

	// Test with invalid minDelay and maxDelay combination
	_, err = newDynamicDelay(0.9, 15, 1*time.Millisecond, 2*time.Hour, 1*time.Hour)
	if err == nil {
		t.Fatal("unexpected, should throw error as minDelay is greater than maxDelay")
	}
}

func TestBucketDelayOverflow(t *testing.T) {
	b, err := newBucketDelay(1-0.1, 15, 1*time.Millisecond, 1*time.Millisecond, 1*time.Hour)
	if err != nil {
		t.Errorf("while creating bucketDelay: %v", err)
	}

	bucketName := "testBucket"

	n := 10000
	for i := 0; i < n; i++ {
		b.increase(bucketName)
	}
	for i := 0; i < 100*n; i++ {
		b.decrease(bucketName)
	}
	if got, want := b.getValue(bucketName), 1*time.Millisecond; got != want {
		t.Fatalf("unexpected delay value: got %v, want %v", got, want)
	}
}

func TestBucketDelayConvergence90(t *testing.T) {
	b, err := newBucketDelay(1-0.1, 15, 1*time.Millisecond, 1*time.Millisecond, 1*time.Hour)
	if err != nil {
		t.Errorf("while creating bucketDelay: %v", err)
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

func TestBucketDelayMapSize(t *testing.T) {
	b, err := newBucketDelay(1-0.1, 15, 1*time.Millisecond, 1*time.Millisecond, 1*time.Hour)
	if err != nil {
		t.Errorf("while creating bucketDelay: %v", err)
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

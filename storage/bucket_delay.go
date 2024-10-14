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
	"sync"
	"time"
)

// bucketDelay wraps dynamicDelay to provide bucket-specific delays.
type bucketDelay struct {
	targetPercentile float64
	increaseRate     float64
	initialDelay     time.Duration
	minDelay         time.Duration
	maxDelay         time.Duration

	delays map[string]*dynamicDelay
	mu     sync.Mutex
}

// newBucketDelay returns a new bucketDelay instance.
func newBucketDelay(targetPercentile float64, increaseRate float64, initialDelay, minDelay, maxDelay time.Duration) (*bucketDelay, error) {
	if targetPercentile < 0 || targetPercentile > 1 {
		return nil, fmt.Errorf("invalid targetPercentile (%v): must be within [0, 1]", targetPercentile)
	}
	if increaseRate <= 0 {
		return nil, fmt.Errorf("invalid increaseRate (%v): must be > 0", increaseRate)
	}
	if minDelay >= maxDelay {
		return nil, fmt.Errorf("invalid minDelay (%v) and maxDelay (%v) combination: minDelay must be smaller than maxDelay", minDelay, maxDelay)
	}

	return &bucketDelay{
		targetPercentile: targetPercentile,
		increaseRate:     increaseRate,
		initialDelay:     initialDelay,
		minDelay:         minDelay,
		maxDelay:         maxDelay,
		delays:           make(map[string]*dynamicDelay),
	}, nil
}

// getDelay retrieves the dynamicDelay instance for the given bucket name. If no delay
// exists for the bucket, a new one is created with the configured parameters.
func (b *bucketDelay) getDelay(bucketName string) *dynamicDelay {
	delay, ok := b.delays[bucketName]
	if !ok {
		// Create a new dynamicDelay for the bucket if it doesn't exist
		delay = newDynamicDelayInternal(b.targetPercentile, b.increaseRate, b.initialDelay, b.minDelay, b.maxDelay)
		b.delays[bucketName] = delay
	}
	return delay
}

// increase notes that the operation took longer than the delay for the given bucket.
func (b *bucketDelay) increase(bucketName string) {
	b.mu.Lock()
	defer b.mu.Unlock()

	b.getDelay(bucketName).unsafeIncrease()
}

// decrease notes that the operation completed before the delay for the given bucket.
func (b *bucketDelay) decrease(bucketName string) {
	b.mu.Lock()
	defer b.mu.Unlock()

	b.getDelay(bucketName).unsafeDecrease()
}

// update updates the delay value for the bucket depending on the specified latency.
func (b *bucketDelay) update(bucketName string, latency time.Duration) {
	b.mu.Lock()
	defer b.mu.Unlock()

	b.getDelay(bucketName).unsafeUpdate(latency)
}

// getValue returns the desired delay to wait before retrying the operation for the given bucket.
func (b *bucketDelay) getValue(bucketName string) time.Duration {
	b.mu.Lock()
	defer b.mu.Unlock()

	return b.getDelay(bucketName).getValueUnsafe()
}

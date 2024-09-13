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

package util

import (
	"fmt"
	"math"
	"sync"
	"time"
)

// Delay dynamically calculates the delay at a fixed percentile, based on
// delay samples.
//
// Delay is goroutine-safe.
type Delay struct {
	increaseFactor float64
	decreaseFactor float64
	minDelay       time.Duration
	maxDelay       time.Duration
	value          time.Duration

	// Guards the value
	mu *sync.RWMutex
}

// NewDelay returns a Delay.
//
// targetPercentile is the desired percentile to be computed. For example, a
// targetPercentile of 0.99 computes the delay at the 99th percentile. Must be
// in the range [0, 1].
//
// increaseRate (must be > 0) determines how many Increase calls it takes for
// Value to double.
//
// initialDelay is the start value of the delay.
//
// Decrease can never lower the delay past minDelay, Increase can never raise
// the delay past maxDelay.
func NewDelay(targetPercentile float64, increaseRate float64, initialDelay, minDelay, maxDelay time.Duration) (*Delay, error) {
	if targetPercentile < 0 || targetPercentile > 1 {
		return nil, fmt.Errorf("invalid targetPercentile (%v): must be within [0, 1]", targetPercentile)
	}
	if increaseRate <= 0 {
		return nil, fmt.Errorf("invalid increaseRate (%v): must be > 0", increaseRate)
	}
	if minDelay >= maxDelay {
		return nil, fmt.Errorf("invalid minDelay (%v) and maxDelay (%v) combination: minDelay must be smaller than maxDelay", minDelay, maxDelay)
	}
	if initialDelay < minDelay {
		initialDelay = minDelay
	}
	if initialDelay > maxDelay {
		initialDelay = maxDelay
	}

	// Compute increaseFactor and decreaseFactor such that:
	// (increaseFactor ^ (1 - targetPercentile)) * (decreaseFactor ^ targetPercentile) = 1
	increaseFactor := math.Exp(math.Log(2) / increaseRate)
	if increaseFactor < 1.001 {
		increaseFactor = 1.001
	}
	decreaseFactor := math.Exp(-math.Log(increaseFactor) * (1 - targetPercentile) / targetPercentile)
	if decreaseFactor > 0.9999 {
		decreaseFactor = 0.9999
	}

	return &Delay{
		increaseFactor: increaseFactor,
		decreaseFactor: decreaseFactor,
		minDelay:       minDelay,
		maxDelay:       maxDelay,
		value:          initialDelay,
		mu:             &sync.RWMutex{},
	}, nil
}

func (d *Delay) increase() {
	v := time.Duration(float64(d.value) * d.increaseFactor)
	if v > d.maxDelay {
		d.value = d.maxDelay
	} else {
		d.value = v
	}
}

// Increase notes that the operation took longer than the delay returned by Value.
func (d *Delay) Increase() {
	d.mu.Lock()
	defer d.mu.Unlock()

	d.increase()
}

func (d *Delay) decrease() {
	v := time.Duration(float64(d.value) * d.decreaseFactor)
	if v < d.minDelay {
		d.value = d.minDelay
	} else {
		d.value = v
	}
}

// Decrease notes that the operation completed before the delay returned by Value.
func (d *Delay) Decrease() {
	d.mu.Lock()
	defer d.mu.Unlock()

	d.decrease()
}

// Update notes that the RPC either took longer than the delay or completed
// before the delay, depending on the specified latency.
func (d *Delay) Update(latency time.Duration) {
	d.mu.Lock()
	defer d.mu.Unlock()

	if latency > d.value {
		d.increase()
	} else {
		d.decrease()
	}
}

// Value returns the desired delay to wait before retry the operation.
func (d *Delay) Value() time.Duration {
	d.mu.RLock()
	defer d.mu.RUnlock()

	return d.value
}

// PrintDelay prints the state of delay, helpful in debugging.
func (d *Delay) PrintDelay() {
	d.mu.RLock()
	defer d.mu.RUnlock()

	fmt.Println("IncreaseFactor: ", d.increaseFactor)
	fmt.Println("DecreaseFactor: ", d.decreaseFactor)
	fmt.Println("MinDelay: ", d.minDelay)
	fmt.Println("MaxDelay: ", d.maxDelay)
	fmt.Println("Value: ", d.value)
}

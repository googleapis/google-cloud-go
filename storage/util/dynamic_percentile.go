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
)

// MetricType is the metric type DynamicPercentile algo expects.
type MetricType int64

// DynamicPercentile dynamically calculates the delay at a fixed percentile, based on
// provided sample metricx.
//
// DynamicPercentile is goroutine-safe.
type DynamicPercentile struct {
	increaseFactor float64
	decreaseFactor float64
	min            MetricType
	max            MetricType
	value          MetricType

	// Guards the value
	mu *sync.RWMutex
}

// NewDynamicPercentile returns a DynamicPercentile.
//
// targetPercentile is the desired percentile to be computed. For example, a
// targetPercentile of 0.99 computes the delay at the 99th percentile. Must be
// in the range [0, 1].
//
// increaseRate (must be > 0) determines how many Increase calls it takes for
// Value to double.
//
// start is the start value of the delay.
//
// Decrease can never lower the delay past min, Increase can never raise
// the delay past max.
func NewDynamicPercentile(targetPercentile float64, increaseRate float64, startVal, minVal, maxVal MetricType) (*DynamicPercentile, error) {
	if targetPercentile < 0 || targetPercentile > 1 {
		return nil, fmt.Errorf("invalid targetPercentile (%v): must be within [0, 1]", targetPercentile)
	}
	if increaseRate <= 0 {
		return nil, fmt.Errorf("invalid increaseRate (%v): must be > 0", increaseRate)
	}
	if minVal >= maxVal {
		return nil, fmt.Errorf("invalid min (%v) and max (%v) combination: min must be smaller than max", minVal, maxVal)
	}
	if startVal < minVal {
		startVal = minVal
	}
	if startVal > maxVal {
		startVal = maxVal
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

	return &DynamicPercentile{
		increaseFactor: increaseFactor,
		decreaseFactor: decreaseFactor,
		min:            minVal,
		max:            maxVal,
		value:          startVal,
		mu:             &sync.RWMutex{},
	}, nil
}

func (dp *DynamicPercentile) increase() {
	v := MetricType(float64(dp.value) * dp.increaseFactor)
	if v > dp.max {
		dp.value = dp.max
	} else {
		dp.value = v
	}
}

// Increase notes that the operation took longer than the delay returned by Value.
func (dp *DynamicPercentile) Increase() {
	dp.mu.Lock()
	defer dp.mu.Unlock()

	dp.increase()
}

func (dp *DynamicPercentile) decrease() {
	v := MetricType(float64(dp.value) * dp.decreaseFactor)
	if v < dp.min {
		dp.value = dp.min
	} else {
		dp.value = v
	}
}

// Decrease notes that the operation completed before the delay returned by Value.
func (dp *DynamicPercentile) Decrease() {
	dp.mu.Lock()
	defer dp.mu.Unlock()

	dp.decrease()
}

// Update notes that the RPC either took longer than the delay or completed
// before the delay, depending on the specified latency.
func (dp *DynamicPercentile) Update(latency MetricType) {
	dp.mu.Lock()
	defer dp.mu.Unlock()

	if latency > dp.value {
		dp.increase()
	} else {
		dp.decrease()
	}
}

// Value returns the approximate percentile (given) of all the previously given samples.
func (dp *DynamicPercentile) Value() MetricType {
	dp.mu.RLock()
	defer dp.mu.RUnlock()

	return dp.value
}

// PrintDynamicPercentile prints the state of delay, helpful in debugging.
func (dp *DynamicPercentile) PrintDynamicPercentile() {
	dp.mu.RLock()
	defer dp.mu.RUnlock()

	fmt.Println("IncreaseFactor: ", dp.increaseFactor)
	fmt.Println("DecreaseFactor: ", dp.decreaseFactor)
	fmt.Println("Min: ", dp.min)
	fmt.Println("Max: ", dp.max)
	fmt.Println("Value: ", dp.value)
}

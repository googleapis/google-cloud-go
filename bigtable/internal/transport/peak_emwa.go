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
	"time"
)

// PeakEwma implements a thread-safe moving average latency continuous time-decay cost calculator.
type PeakEwma struct {
	mu         sync.Mutex
	tau        time.Duration
	value      float64
	lastUpdate time.Time
}

// NewPeakEwma creates a new PeakEwma calculator with the given decay time constant.
func NewPeakEwma(tau time.Duration) *PeakEwma {
	return &PeakEwma{
		tau:   tau,
	}
}

// Update updates the EWMA with a new latency sample.
func (e *PeakEwma) Update(latency time.Duration) {
	e.mu.Lock()
	defer e.mu.Unlock()
	now := time.Now()
	if e.lastUpdate.IsZero() {
		e.value = float64(latency)
		e.lastUpdate = now
		return
	}
	dt := now.Sub(e.lastUpdate)
	e.lastUpdate = now
	// Continuous time-decay weight: e^(-dt/tau)
	weight := math.Exp(-float64(dt) / float64(e.tau))
	// EWMA = EWMA * weight + latency * (1 - weight)
	e.value = e.value*weight + float64(latency)*(1-weight)
}

// Value returns the current EWMA value.
func (e *PeakEwma) Value() float64 {
	e.mu.Lock()
	defer e.mu.Unlock()
	return e.value
}
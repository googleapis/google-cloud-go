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

// PeakEwma is a thread-safe continuous-time exponentially-weighted moving
// average of latency samples. The AFE picker uses one instance per bucket
// to score candidates; see sessionList.
type PeakEwma struct {
	mu         sync.Mutex
	tau        time.Duration
	value      float64
	lastUpdate time.Time
}

// NewPeakEwma creates a PeakEwma with the given decay time constant tau.
// The initial value is zero — Value() returns 0 until the first Update.
func NewPeakEwma(tau time.Duration) *PeakEwma {
	return &PeakEwma{
		tau: tau,
	}
}

// NewPeakEwmaSeeded returns a PeakEwma pre-seeded with the given cost so
// Value() returns non-zero before the first Update lands. The seed is
// authoritative only until the first Update, which resets value to the
// real sample (see Update's lastUpdate.IsZero branch). Java parity —
// SessionList.java seeds transport at 500µs and e2e at 1ms so a
// brand-new AFE doesn't win the least-latency picker by looking
// free-cost.
func NewPeakEwmaSeeded(tau, seed time.Duration) *PeakEwma {
	return &PeakEwma{
		tau:   tau,
		value: float64(seed),
	}
}

// Update folds a new latency sample into the EWMA. The first Update
// after construction snaps value to the sample (overriding any seed);
// subsequent samples decay the prior value by e^(-dt/tau) and blend the
// remainder toward the new sample.
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
	// Continuous time-decay weight: e^(-dt/tau).
	weight := math.Exp(-float64(dt) / float64(e.tau))
	// EWMA = EWMA * weight + latency * (1 - weight).
	e.value = e.value*weight + float64(latency)*(1-weight)
}

// Value returns the current EWMA in the same units as the samples fed
// into Update (nanoseconds, as float64 — the picker consumes this raw).
func (e *PeakEwma) Value() float64 {
	e.mu.Lock()
	defer e.mu.Unlock()
	return e.value
}

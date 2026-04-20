/*
Copyright 2026 Google LLC

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package spanner

import (
	"math"
	"sync"
	"sync/atomic"
	"time"
)

const defaultEWMADecayTime = 10 * time.Second

type ewmaLatencyTracker struct {
	updateMu sync.Mutex

	fixedAlpha *float64
	now        func() time.Time
	tau        time.Duration

	scoreBits        atomic.Uint64
	initialized      atomic.Bool
	lastUpdatedNanos atomic.Int64
}

func newEWMALatencyTracker() *ewmaLatencyTracker {
	return newEWMALatencyTrackerWithOptions(defaultEWMADecayTime, time.Now)
}

func newEWMALatencyTrackerWithAlpha(alpha float64, now func() time.Time) *ewmaLatencyTracker {
	if alpha <= 0 || alpha > 1 {
		panic("alpha must be in (0, 1]")
	}
	alphaCopy := alpha
	if now == nil {
		now = time.Now
	}
	return &ewmaLatencyTracker{
		fixedAlpha: &alphaCopy,
		now:        now,
	}
}

func newEWMALatencyTrackerWithOptions(decayTime time.Duration, now func() time.Time) *ewmaLatencyTracker {
	if decayTime <= 0 {
		panic("decayTime must be > 0")
	}
	if now == nil {
		now = time.Now
	}
	return &ewmaLatencyTracker{
		now: now,
		tau: decayTime,
	}
}

func (t *ewmaLatencyTracker) scoreValue() float64 {
	if !t.initialized.Load() {
		return math.MaxFloat64
	}
	return math.Float64frombits(t.scoreBits.Load())
}

func (t *ewmaLatencyTracker) update(latency time.Duration) {
	latencyMicros := float64(latency.Nanoseconds()) / 1e3
	now := t.now()

	t.updateMu.Lock()
	defer t.updateMu.Unlock()

	if !t.initialized.Load() {
		t.scoreBits.Store(math.Float64bits(latencyMicros))
		t.initialized.Store(true)
		t.lastUpdatedNanos.Store(now.UnixNano())
		return
	}

	alpha := t.calculateAlphaLocked(now)
	score := math.Float64frombits(t.scoreBits.Load())
	score = alpha*latencyMicros + (1-alpha)*score
	t.scoreBits.Store(math.Float64bits(score))
	t.lastUpdatedNanos.Store(now.UnixNano())
}

func (t *ewmaLatencyTracker) recordError(penalty time.Duration) {
	t.update(penalty)
}

func (t *ewmaLatencyTracker) calculateAlphaLocked(now time.Time) float64 {
	if t.fixedAlpha != nil {
		return *t.fixedAlpha
	}
	lastUpdated := time.Unix(0, t.lastUpdatedNanos.Load())
	delta := now.Sub(lastUpdated)
	if delta <= 0 {
		return 1
	}
	alpha := 1 - math.Exp(-float64(delta)/float64(t.tau))
	if alpha < 0 {
		return 0
	}
	if alpha > 1 {
		return 1
	}
	return alpha
}

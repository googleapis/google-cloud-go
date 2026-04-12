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
	"time"
)

const defaultEWMALatencyAlpha = 0.05

type latencyTracker interface {
	getScore() float64
	update(latency time.Duration)
	recordError(penalty time.Duration)
}

type ewmaLatencyTracker struct {
	mu          sync.Mutex
	alpha       float64
	score       float64
	initialized bool
}

var _ latencyTracker = (*ewmaLatencyTracker)(nil)

func newEWMALatencyTracker() *ewmaLatencyTracker {
	return newEWMALatencyTrackerWithAlpha(defaultEWMALatencyAlpha)
}

func newEWMALatencyTrackerWithAlpha(alpha float64) *ewmaLatencyTracker {
	if alpha <= 0 || alpha > 1 {
		alpha = defaultEWMALatencyAlpha
	}
	return &ewmaLatencyTracker{alpha: alpha}
}

func (t *ewmaLatencyTracker) getScore() float64 {
	if t == nil {
		return math.MaxFloat64
	}
	t.mu.Lock()
	defer t.mu.Unlock()

	if !t.initialized {
		return math.MaxFloat64
	}
	return t.score
}

func (t *ewmaLatencyTracker) update(latency time.Duration) {
	if t == nil {
		return
	}
	latencyMicros := durationToMicroseconds(latency)

	t.mu.Lock()
	defer t.mu.Unlock()

	if !t.initialized {
		t.score = latencyMicros
		t.initialized = true
		return
	}
	t.score = t.alpha*latencyMicros + (1-t.alpha)*t.score
}

func (t *ewmaLatencyTracker) recordError(penalty time.Duration) {
	t.update(penalty)
}

func durationToMicroseconds(latency time.Duration) float64 {
	if latency <= 0 {
		return 0
	}
	return float64(latency.Nanoseconds()) / float64(time.Microsecond)
}

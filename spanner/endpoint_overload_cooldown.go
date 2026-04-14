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
	"math/rand"
	"sync"
	"time"
)

const (
	defaultEndpointOverloadInitialCooldown = 3 * time.Second
	defaultEndpointOverloadMaxCooldown     = time.Minute
	defaultEndpointOverloadResetAfter      = 5 * time.Minute
	defaultEndpointOverloadRecoverySuccess = 2
)

type endpointOverloadCooldownState struct {
	consecutiveFailures int
	consecutiveSuccess  int
	cooldownUntil       time.Time
	lastFailureAt       time.Time
}

// endpointOverloadCooldownTracker keeps routed endpoints out of selection for a
// short period after RESOURCE_EXHAUSTED so the router can try another replica.
type endpointOverloadCooldownTracker struct {
	mu              sync.Mutex
	entries         map[string]endpointOverloadCooldownState
	initialCooldown time.Duration
	maxCooldown     time.Duration
	resetAfter      time.Duration
	recoverySuccess int
	now             func() time.Time
	randInt63n      func(int64) int64
}

func newEndpointOverloadCooldownTracker() *endpointOverloadCooldownTracker {
	return newEndpointOverloadCooldownTrackerWithOptions(
		defaultEndpointOverloadInitialCooldown,
		defaultEndpointOverloadMaxCooldown,
		defaultEndpointOverloadResetAfter,
		defaultEndpointOverloadRecoverySuccess,
		time.Now,
		rand.Int63n,
	)
}

func newEndpointOverloadCooldownTrackerWithOptions(
	initialCooldown time.Duration,
	maxCooldown time.Duration,
	resetAfter time.Duration,
	recoverySuccess int,
	now func() time.Time,
	randInt63n func(int64) int64,
) *endpointOverloadCooldownTracker {
	if initialCooldown <= 0 {
		initialCooldown = defaultEndpointOverloadInitialCooldown
	}
	if maxCooldown <= 0 {
		maxCooldown = defaultEndpointOverloadMaxCooldown
	}
	if maxCooldown < initialCooldown {
		maxCooldown = initialCooldown
	}
	if resetAfter <= 0 {
		resetAfter = defaultEndpointOverloadResetAfter
	}
	if recoverySuccess <= 0 {
		recoverySuccess = defaultEndpointOverloadRecoverySuccess
	}
	if now == nil {
		now = time.Now
	}
	if randInt63n == nil {
		randInt63n = rand.Int63n
	}
	return &endpointOverloadCooldownTracker{
		entries:         make(map[string]endpointOverloadCooldownState),
		initialCooldown: initialCooldown,
		maxCooldown:     maxCooldown,
		resetAfter:      resetAfter,
		recoverySuccess: recoverySuccess,
		now:             now,
		randInt63n:      randInt63n,
	}
}

func (t *endpointOverloadCooldownTracker) isCoolingDown(address string) bool {
	if t == nil || address == "" {
		return false
	}

	now := t.now()

	t.mu.Lock()
	defer t.mu.Unlock()

	state, ok := t.entries[address]
	if !ok {
		return false
	}
	if !state.cooldownUntil.After(now) {
		if now.Sub(state.lastFailureAt) >= t.resetAfter {
			delete(t.entries, address)
		}
		return false
	}
	return true
}

func (t *endpointOverloadCooldownTracker) recordFailure(address string) {
	if t == nil || address == "" {
		return
	}

	now := t.now()

	t.mu.Lock()
	defer t.mu.Unlock()

	state := t.entries[address]
	if state.lastFailureAt.IsZero() || now.Sub(state.lastFailureAt) >= t.resetAfter {
		state.consecutiveFailures = 0
	}
	state.consecutiveFailures++
	state.consecutiveSuccess = 0
	state.lastFailureAt = now
	state.cooldownUntil = now.Add(t.cooldownForFailures(state.consecutiveFailures))
	t.entries[address] = state
}

func (t *endpointOverloadCooldownTracker) recordSuccess(address string) {
	if t == nil || address == "" {
		return
	}

	t.mu.Lock()
	defer t.mu.Unlock()
	state, ok := t.entries[address]
	if !ok {
		return
	}
	state.consecutiveSuccess++
	if state.consecutiveSuccess >= t.recoverySuccess {
		delete(t.entries, address)
		return
	}
	t.entries[address] = state
}

func (t *endpointOverloadCooldownTracker) cooldownForFailures(failures int) time.Duration {
	cooldown := t.initialCooldown
	for i := 1; i < failures; i++ {
		if cooldown >= t.maxCooldown {
			return t.maxCooldown
		}
		if cooldown > t.maxCooldown/2 {
			return t.maxCooldown
		}
		cooldown *= 2
	}
	if cooldown > t.maxCooldown {
		cooldown = t.maxCooldown
	}
	if cooldown <= 0 {
		return 0
	}
	if cooldown == 1 {
		return time.Duration(t.randInt63n(2))
	}
	return time.Duration(t.randInt63n(int64(cooldown) + 1))
}

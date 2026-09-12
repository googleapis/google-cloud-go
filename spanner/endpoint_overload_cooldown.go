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
	"sync/atomic"
	"time"

	"google.golang.org/grpc/codes"
)

const (
	defaultEndpointOverloadInitialCooldown   = 10 * time.Second
	defaultEndpointOverloadMaxCooldown       = time.Minute
	defaultEndpointOverloadResetAfter        = 10 * time.Minute
	defaultEndpointCooldownSuccessesToRepair = 3
	defaultEndpointMinHintedCooldown         = 100 * time.Millisecond
	defaultEndpointMaxHintedClientFloor      = 2 * time.Second
	defaultEndpointMaxHintedJitter           = 500 * time.Millisecond
	defaultEndpointCooldownMaxTier           = 6
	minEndpointProbeReservation              = 250 * time.Millisecond
	maxEndpointProbeReservation              = 500 * time.Millisecond
)

type endpointOverloadCooldownState struct {
	overloadFailures         int
	unavailableFailures      int
	successesTowardRepair    int
	overloadUntil            time.Time
	unavailableUntil         time.Time
	overloadProbeNotBefore   time.Time
	probeReservedUntil       time.Time
	lastOverloadFailureAt    time.Time
	lastUnavailableFailureAt time.Time
}

type hintedEndpointCooldown struct {
	cooldown   time.Duration
	probeDelay time.Duration
}

// endpointOverloadCooldownTracker keeps routed endpoints out of selection after
// overload or availability failures. The two failure classes escalate
// independently so a short server-hinted load shed does not weaken connection
// failure protection.
type endpointOverloadCooldownTracker struct {
	mu              sync.RWMutex
	entries         map[string]endpointOverloadCooldownState
	entryCount      atomic.Int64
	initialCooldown time.Duration
	maxCooldown     time.Duration
	resetAfter      time.Duration
	now             func() time.Time
	randInt63n      func(int64) int64
}

func newEndpointOverloadCooldownTracker() *endpointOverloadCooldownTracker {
	return newEndpointOverloadCooldownTrackerWithOptions(
		defaultEndpointOverloadInitialCooldown,
		defaultEndpointOverloadMaxCooldown,
		defaultEndpointOverloadResetAfter,
		time.Now,
		rand.Int63n,
	)
}

func newEndpointOverloadCooldownTrackerWithOptions(
	initialCooldown time.Duration,
	maxCooldown time.Duration,
	resetAfter time.Duration,
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
		now:             now,
		randInt63n:      randInt63n,
	}
}

func isEndpointCoolingDown(cooldowns *endpointOverloadCooldownTracker, address string) bool {
	return cooldowns != nil && cooldowns.isCoolingDown(address)
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
	if state.overloadUntil.After(now) || state.unavailableUntil.After(now) {
		return true
	}
	if t.isIdle(state, now) {
		t.deleteEntryLocked(address)
	}
	return false
}

func (t *endpointOverloadCooldownTracker) recordFailure(address string) {
	t.recordFailureWithStatus(address, codes.ResourceExhausted, nil)
}

func (t *endpointOverloadCooldownTracker) recordFailureWithStatus(address string, code codes.Code, serverRetryDelay *time.Duration) {
	if t == nil || address == "" || (code != codes.ResourceExhausted && code != codes.Unavailable) {
		return
	}
	now := t.now()
	t.mu.Lock()
	defer t.mu.Unlock()
	state, ok := t.entries[address]
	if !ok || t.isIdle(state, now) {
		state = t.emptyState(now)
	}
	state.successesTowardRepair = 0
	if code == codes.ResourceExhausted {
		failures := t.nextFailureTier(state.overloadFailures, state.lastOverloadFailureAt, now)
		cooldown := t.cooldownForFailures(failures)
		probeDelay := cooldown
		if serverRetryDelay != nil && *serverRetryDelay >= 0 {
			hinted := t.hintedOverloadCooldown(failures, *serverRetryDelay)
			cooldown = hinted.cooldown
			probeDelay = hinted.probeDelay
		}
		state.overloadFailures = failures
		state.overloadUntil = laterTime(state.overloadUntil, now.Add(cooldown))
		state.overloadProbeNotBefore = laterTime(state.overloadProbeNotBefore, now.Add(probeDelay))
		state.lastOverloadFailureAt = now
	} else {
		failures := t.nextFailureTier(state.unavailableFailures, state.lastUnavailableFailureAt, now)
		state.unavailableFailures = failures
		state.unavailableUntil = laterTime(state.unavailableUntil, now.Add(t.cooldownForFailures(failures)))
		state.lastUnavailableFailureAt = now
	}
	t.storeEntryLocked(address, state)
}

func (t *endpointOverloadCooldownTracker) recordSuccess(address string) {
	if t == nil || address == "" {
		return
	}
	if t.entryCount.Load() == 0 {
		// A concurrent insertion can miss one success credit, delaying repair by
		// one call without corrupting state.
		return
	}
	// A stale nonzero read falls through to the locked lookup and finds no entry.
	now := t.now()
	t.mu.Lock()
	defer t.mu.Unlock()
	state, ok := t.entries[address]
	if !ok {
		return
	}
	if t.isIdle(state, now) {
		t.deleteEntryLocked(address)
		return
	}
	state.successesTowardRepair++
	if state.successesTowardRepair < defaultEndpointCooldownSuccessesToRepair {
		t.storeEntryLocked(address, state)
		return
	}
	if state.overloadFailures > 0 {
		state.overloadFailures--
	}
	if state.unavailableFailures > 0 {
		state.unavailableFailures--
	}
	state.successesTowardRepair = 0
	if state.overloadFailures == 0 && state.unavailableFailures == 0 &&
		!state.overloadUntil.After(now) && !state.unavailableUntil.After(now) {
		t.deleteEntryLocked(address)
		return
	}
	t.storeEntryLocked(address, state)
}

func (t *endpointOverloadCooldownTracker) tryReserveProbe(address string) bool {
	if t == nil || address == "" {
		return false
	}
	now := t.now()
	t.mu.Lock()
	defer t.mu.Unlock()
	state, ok := t.entries[address]
	if !ok || !state.overloadUntil.After(now) || state.unavailableUntil.After(now) ||
		state.overloadProbeNotBefore.After(now) || state.probeReservedUntil.After(now) {
		return false
	}
	minMillis := minEndpointProbeReservation.Milliseconds()
	rangeMillis := maxEndpointProbeReservation.Milliseconds() - minMillis + 1
	state.probeReservedUntil = now.Add(time.Duration(minMillis+t.randInt63n(rangeMillis)) * time.Millisecond)
	t.storeEntryLocked(address, state)
	return true
}

func (t *endpointOverloadCooldownTracker) lastOverloadFailure(address string) time.Time {
	if t == nil || address == "" {
		return time.Time{}
	}
	if t.entryCount.Load() == 0 {
		return time.Time{}
	}
	t.mu.RLock()
	defer t.mu.RUnlock()
	return t.entries[address].lastOverloadFailureAt
}

func (t *endpointOverloadCooldownTracker) emptyState(now time.Time) endpointOverloadCooldownState {
	return endpointOverloadCooldownState{
		overloadUntil:          now,
		unavailableUntil:       now,
		overloadProbeNotBefore: now,
		probeReservedUntil:     now,
	}
}

func (t *endpointOverloadCooldownTracker) isRecent(lastFailureAt, now time.Time) bool {
	return !lastFailureAt.IsZero() && now.Sub(lastFailureAt) < t.resetAfter
}

func (t *endpointOverloadCooldownTracker) isIdle(state endpointOverloadCooldownState, now time.Time) bool {
	return !state.overloadUntil.After(now) && !state.unavailableUntil.After(now) &&
		!t.isRecent(state.lastOverloadFailureAt, now) && !t.isRecent(state.lastUnavailableFailureAt, now)
}

func (t *endpointOverloadCooldownTracker) nextFailureTier(previous int, lastFailureAt, now time.Time) int {
	if previous == 0 || !t.isRecent(lastFailureAt, now) {
		return 1
	}
	if previous >= defaultEndpointCooldownMaxTier {
		return defaultEndpointCooldownMaxTier
	}
	return previous + 1
}

func (t *endpointOverloadCooldownTracker) hintedOverloadCooldown(failures int, serverRetryDelay time.Duration) hintedEndpointCooldown {
	serverFloor := maxCooldownDuration(serverRetryDelay, defaultEndpointMinHintedCooldown)
	clientFloor := defaultEndpointMinHintedCooldown
	for i := 1; i < failures && clientFloor < defaultEndpointMaxHintedClientFloor; i++ {
		clientFloor = minCooldownDuration(clientFloor*2, defaultEndpointMaxHintedClientFloor)
	}
	base := maxCooldownDuration(serverFloor, clientFloor)
	jitterLimit := minCooldownDuration(base/4, defaultEndpointMaxHintedJitter)
	jitter := time.Duration(t.randInt63n(jitterLimit.Milliseconds()+1)) * time.Millisecond
	return hintedEndpointCooldown{cooldown: base + jitter, probeDelay: serverFloor + jitter}
}

func (t *endpointOverloadCooldownTracker) cooldownForFailures(failures int) time.Duration {
	cooldown := t.initialCooldown
	for i := 1; i < failures; i++ {
		if cooldown > t.maxCooldown/2 {
			cooldown = t.maxCooldown
			break
		}
		cooldown *= 2
	}
	cooldownNanos := int64(cooldown)
	if cooldownNanos < 1 {
		cooldownNanos = 1
	}
	floorNanos := cooldownNanos / 2
	if floorNanos < 1 {
		floorNanos = 1
	}
	rangeSize := cooldownNanos - floorNanos + 1
	if rangeSize < 1 {
		rangeSize = 1
	}
	return time.Duration(floorNanos + t.randInt63n(rangeSize))
}

func (t *endpointOverloadCooldownTracker) pruneStaleEntries(maxAge time.Duration) {
	if t == nil || maxAge <= 0 {
		return
	}
	if t.entryCount.Load() == 0 {
		return
	}
	now := t.now()
	t.mu.Lock()
	defer t.mu.Unlock()
	for address, state := range t.entries {
		if state.overloadUntil.After(now) || state.unavailableUntil.After(now) {
			continue
		}
		lastFailureAt := laterTime(state.lastOverloadFailureAt, state.lastUnavailableFailureAt)
		if lastFailureAt.IsZero() || now.Sub(lastFailureAt) >= maxAge {
			t.deleteEntryLocked(address)
		}
	}
}

// storeEntryLocked stores state and tracks first insertion. t.mu must be held.
func (t *endpointOverloadCooldownTracker) storeEntryLocked(address string, state endpointOverloadCooldownState) {
	if _, ok := t.entries[address]; !ok {
		t.entryCount.Add(1)
	}
	t.entries[address] = state
}

// deleteEntryLocked deletes state and tracks removal. t.mu must be held.
func (t *endpointOverloadCooldownTracker) deleteEntryLocked(address string) {
	if _, ok := t.entries[address]; !ok {
		return
	}
	delete(t.entries, address)
	t.entryCount.Add(-1)
}

func minCooldownDuration(first, second time.Duration) time.Duration {
	if first <= second {
		return first
	}
	return second
}

func maxCooldownDuration(first, second time.Duration) time.Duration {
	if first >= second {
		return first
	}
	return second
}

func laterTime(first, second time.Time) time.Time {
	if first.After(second) {
		return first
	}
	return second
}

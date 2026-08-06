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
	"math/rand"
	"time"
)

const (
	defaultEWMADecayTime                   = 10 * time.Second
	defaultEndpointOverloadInitialCooldown = 10 * time.Second
	defaultEndpointOverloadMaxCooldown     = time.Minute
	defaultEndpointOverloadResetAfter      = 10 * time.Minute
	endpointLatencyDefaultPenaltyValue     = 1_000_000.0
	endpointLatencyDefaultErrorPenalty     = 10 * time.Second
	endpointLatencyDefaultRTT              = 10 * time.Millisecond
	endpointRoutingScoreExpireAfter        = 10 * time.Minute
)

type endpointScoreKey struct {
	operationUID uint64
	preferLeader bool
}

type endpointScoreState struct {
	scoreMicros      float64
	initialized      bool
	lastUpdatedNanos int64
	lastAccess       time.Time
}

type endpointRoutingConfig struct {
	now              func() time.Time
	randInt63n       func(int64) int64
	initialCooldown  time.Duration
	maxCooldown      time.Duration
	resetAfter       time.Duration
	scoreExpireAfter time.Duration
}

func defaultEndpointRoutingConfig() endpointRoutingConfig {
	return endpointRoutingConfig{
		now:              time.Now,
		randInt63n:       rand.Int63n,
		initialCooldown:  defaultEndpointOverloadInitialCooldown,
		maxCooldown:      defaultEndpointOverloadMaxCooldown,
		resetAfter:       defaultEndpointOverloadResetAfter,
		scoreExpireAfter: endpointRoutingScoreExpireAfter,
	}
}

func (cfg endpointRoutingConfig) normalize() endpointRoutingConfig {
	if cfg.now == nil {
		cfg.now = time.Now
	}
	if cfg.randInt63n == nil {
		cfg.randInt63n = rand.Int63n
	}
	if cfg.initialCooldown <= 0 {
		cfg.initialCooldown = defaultEndpointOverloadInitialCooldown
	}
	if cfg.maxCooldown <= 0 {
		cfg.maxCooldown = defaultEndpointOverloadMaxCooldown
	}
	if cfg.maxCooldown < cfg.initialCooldown {
		cfg.maxCooldown = cfg.initialCooldown
	}
	if cfg.resetAfter <= 0 {
		cfg.resetAfter = defaultEndpointOverloadResetAfter
	}
	if cfg.scoreExpireAfter <= 0 {
		cfg.scoreExpireAfter = endpointRoutingScoreExpireAfter
	}
	return cfg
}

func (e *grpcChannelEndpoint) selectionCost(cfg endpointRoutingConfig, operationUID uint64, preferLeader bool) float64 {
	cfg = cfg.normalize()
	if operationUID == 0 {
		return math.MaxFloat64
	}

	now := cfg.now()
	activeRequests := float64(e.ActiveRequestCount())

	e.stateMu.Lock()
	defer e.stateMu.Unlock()

	score, ok := e.lookupScoreLocked(endpointScoreKey{operationUID: operationUID, preferLeader: preferLeader}, now, cfg.scoreExpireAfter)
	if ok {
		return score * (activeRequests + 1)
	}
	if activeRequests > 0 {
		return endpointLatencyDefaultPenaltyValue + activeRequests
	}
	return (float64(endpointLatencyDefaultRTT) / 1e3) * (activeRequests + 1)
}

func (e *grpcChannelEndpoint) recordLatency(cfg endpointRoutingConfig, operationUID uint64, preferLeader bool, latency time.Duration) {
	cfg = cfg.normalize()
	if operationUID == 0 {
		return
	}
	now := cfg.now()

	e.stateMu.Lock()
	defer e.stateMu.Unlock()

	key := endpointScoreKey{operationUID: operationUID, preferLeader: preferLeader}
	state := e.getOrCreateScoreLocked(key, now)
	state.update(latency, now)
}

func (e *grpcChannelEndpoint) recordError(cfg endpointRoutingConfig, operationUID uint64, preferLeader bool) {
	cfg = cfg.normalize()
	if operationUID == 0 {
		return
	}
	now := cfg.now()

	e.stateMu.Lock()
	defer e.stateMu.Unlock()

	key := endpointScoreKey{operationUID: operationUID, preferLeader: preferLeader}
	state := e.getOrCreateScoreLocked(key, now)
	state.update(endpointLatencyDefaultErrorPenalty, now)
}

func (e *grpcChannelEndpoint) hasScore(cfg endpointRoutingConfig, operationUID uint64, preferLeader bool) bool {
	cfg = cfg.normalize()
	if operationUID == 0 {
		return false
	}
	now := cfg.now()

	e.stateMu.Lock()
	defer e.stateMu.Unlock()

	_, ok := e.lookupScoreLocked(endpointScoreKey{operationUID: operationUID, preferLeader: preferLeader}, now, cfg.scoreExpireAfter)
	return ok
}

func (e *grpcChannelEndpoint) isCoolingDown(cfg endpointRoutingConfig) bool {
	cfg = cfg.normalize()
	now := cfg.now()

	e.stateMu.Lock()
	defer e.stateMu.Unlock()

	if e.cooldownUntil.After(now) {
		return true
	}
	if e.lastFailureAt.IsZero() || now.Sub(e.lastFailureAt) < cfg.resetAfter {
		return false
	}
	e.resetCooldownLocked()
	return false
}

func (e *grpcChannelEndpoint) remainingCooldown(cfg endpointRoutingConfig) time.Duration {
	cfg = cfg.normalize()
	now := cfg.now()

	e.stateMu.Lock()
	defer e.stateMu.Unlock()

	if e.cooldownUntil.After(now) {
		return e.cooldownUntil.Sub(now)
	}
	return 0
}

func (e *grpcChannelEndpoint) recordFailure(cfg endpointRoutingConfig) {
	cfg = cfg.normalize()
	now := cfg.now()

	e.stateMu.Lock()
	defer e.stateMu.Unlock()

	if e.lastFailureAt.IsZero() || now.Sub(e.lastFailureAt) >= cfg.resetAfter {
		e.consecutiveFailures = 0
	}
	e.consecutiveFailures++
	e.lastFailureAt = now
	e.cooldownUntil = now.Add(endpointCooldownForFailures(cfg, e.consecutiveFailures))
}

func endpointCooldownForFailures(cfg endpointRoutingConfig, failures int) time.Duration {
	cooldown := cfg.initialCooldown
	for i := 1; i < failures; i++ {
		if cooldown > cfg.maxCooldown/2 {
			cooldown = cfg.maxCooldown
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
	return time.Duration(floorNanos + cfg.randInt63n(rangeSize))
}

func (e *grpcChannelEndpoint) pruneRoutingState(cfg endpointRoutingConfig, maxAge time.Duration) {
	cfg = cfg.normalize()
	now := cfg.now()

	e.stateMu.Lock()
	defer e.stateMu.Unlock()

	if maxAge > 0 && !e.cooldownUntil.After(now) && !e.lastFailureAt.IsZero() && now.Sub(e.lastFailureAt) >= maxAge {
		e.resetCooldownLocked()
	}
	if len(e.scores) == 0 {
		return
	}
	for key, state := range e.scores {
		if now.Sub(state.lastAccess) >= cfg.scoreExpireAfter {
			delete(e.scores, key)
		}
	}
}

func (e *grpcChannelEndpoint) clearScores() {
	e.stateMu.Lock()
	defer e.stateMu.Unlock()
	clear(e.scores)
}

func (e *grpcChannelEndpoint) resetCooldownLocked() {
	e.consecutiveFailures = 0
	e.cooldownUntil = time.Time{}
	e.lastFailureAt = time.Time{}
}

func (e *grpcChannelEndpoint) lookupScoreLocked(key endpointScoreKey, now time.Time, expireAfter time.Duration) (float64, bool) {
	if len(e.scores) == 0 {
		return 0, false
	}
	state, ok := e.scores[key]
	if !ok {
		return 0, false
	}
	if now.Sub(state.lastAccess) >= expireAfter {
		delete(e.scores, key)
		return 0, false
	}
	if !state.initialized {
		return 0, false
	}
	state.lastAccess = now
	return state.scoreMicros, true
}

func (e *grpcChannelEndpoint) getOrCreateScoreLocked(key endpointScoreKey, now time.Time) *endpointScoreState {
	if e.scores == nil {
		e.scores = make(map[endpointScoreKey]*endpointScoreState)
	}
	if state, ok := e.scores[key]; ok {
		state.lastAccess = now
		return state
	}
	state := &endpointScoreState{lastAccess: now}
	e.scores[key] = state
	return state
}

func (s *endpointScoreState) update(latency time.Duration, now time.Time) {
	latencyMicros := float64(latency) / float64(time.Microsecond)
	nowNanos := now.UnixNano()
	if !s.initialized {
		s.scoreMicros = latencyMicros
		s.initialized = true
		s.lastUpdatedNanos = nowNanos
		s.lastAccess = now
		return
	}

	deltaNanos := nowNanos - s.lastUpdatedNanos
	alpha := 1.0
	if deltaNanos > 0 {
		alpha = 1 - math.Exp(-float64(deltaNanos)/float64(defaultEWMADecayTime))
		if alpha < 0 {
			alpha = 0
		}
		if alpha > 1 {
			alpha = 1
		}
	}
	s.scoreMicros = alpha*latencyMicros + (1-alpha)*s.scoreMicros
	s.lastUpdatedNanos = nowNanos
	s.lastAccess = now
}

type endpointRoutingStateSnapshot struct {
	consecutiveFailures int
	cooldownUntil       time.Time
	lastFailureAt       time.Time
}

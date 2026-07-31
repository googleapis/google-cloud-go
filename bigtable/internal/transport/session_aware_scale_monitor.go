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
	"context"
	"sync/atomic"
	"time"

	spb "cloud.google.com/go/bigtable/apiv2/bigtablepb"
)

// SessionAwareHeadroom is the multiplier applied to per-channel session
// capacity when picking the target channel count. Sessions are
// long-lived bidi streams pinned to one channel — an AFE-side hiccup
// affects every session on that channel, so oversizing the pool is
// the correct safety-margin tradeoff. Kept as a named const rather
// than an inline literal so the intent shows up at every callsite.
const SessionAwareHeadroom = 2

// SessionAwareTickInterval is the default cadence for target
// recomputation. Sessions live minutes-to-hours; picking a shorter
// interval only adds jitter, not signal. Exposed as a package-level
// var (not const) so tests can override it — the timer runs off the
// value at Start time.
var SessionAwareTickInterval = 1 * time.Minute

// SessionAwareScaleMonitor sizes a BigtableChannelPool from live
// session count instead of RPC load. Complements (and, in the session
// client, replaces) DynamicScaleMonitor. On each tick:
//
//   1. Read sessionCountFn() — the caller's summary of live sessions
//      across every session pool sharing this channel pool.
//   2. Read the latest ChannelPoolConfiguration snapshot fed via
//      OnConfig (min/max/perServerSessionCount).
//   3. Compute target = ceil((sessions × SessionAwareHeadroom) /
//      perServerSessionCount), clamped to [min, max].
//   4. Call pool.addConnections / removeConnections to reach target.
//
// Threading model:
//   - OnConfig races with the tick — snapshot is stored atomically.
//   - sessionCountFn is called from the tick goroutine only, so it
//     doesn't need to be reentrant-safe.
//   - Start / Stop are once-only; Stop blocks on the goroutine exit
//     via the done channel so callers can rely on "no more mutations
//     after Stop returns".
//
// Not thread-safe for concurrent Start / Stop — production callers
// invoke each once from client construction / Close.
type SessionAwareScaleMonitor struct {
	pool            *BigtableChannelPool
	sessionCountFn  func() int
	config          atomic.Pointer[channelPoolConfigSnapshot]
	tickInterval    time.Duration
	cancel          context.CancelFunc
	done            chan struct{}
	// tickHook, when non-nil, replaces the ticker-driven loop with a
	// manual-drive contract: tests call m.TickForTest() to invoke tick()
	// once. Kept optional so production wiring stays timer-based.
	tickHookEnabled bool
}

// channelPoolConfigSnapshot is the immutable triple loaded on every
// tick. Kept together in a pointer so the tick reads the tuple
// atomically — otherwise a config update mid-formula could feed a
// half-old / half-new (min, max, softMax) into clamp and produce a
// nonsense target.
type channelPoolConfigSnapshot struct {
	min, max, softMax int
}

// NewSessionAwareScaleMonitor constructs a monitor bound to pool.
// sessionCountFn must return the current total session count across
// every session pool that shares pool. Called from the tick goroutine
// only.
func NewSessionAwareScaleMonitor(pool *BigtableChannelPool, sessionCountFn func() int) *SessionAwareScaleMonitor {
	return &SessionAwareScaleMonitor{
		pool:           pool,
		sessionCountFn: sessionCountFn,
		tickInterval:   SessionAwareTickInterval,
		done:           make(chan struct{}),
	}
}

// OnConfig accepts a ChannelPoolConfiguration update — designed as
// the callback registered with ClientConfigurationManager.
// AddChannelPoolConfigListener. nil config is ignored. Non-positive
// bounds fall through to the tick, which sees softMax<=0 and no-ops.
func (m *SessionAwareScaleMonitor) OnConfig(cfg *spb.SessionClientConfiguration_ChannelPoolConfiguration) {
	if cfg == nil {
		return
	}
	m.config.Store(&channelPoolConfigSnapshot{
		min:     int(cfg.GetMinServerCount()),
		max:     int(cfg.GetMaxServerCount()),
		softMax: int(cfg.GetPerServerSessionCount()),
	})
}

// Start begins the tick loop under ctx. The goroutine exits when ctx
// is cancelled or Stop is called (both routes close the done channel).
// Safe to call at most once per monitor instance.
func (m *SessionAwareScaleMonitor) Start(ctx context.Context) {
	loopCtx, cancel := context.WithCancel(ctx)
	m.cancel = cancel
	if m.tickHookEnabled {
		// Test mode: no timer, tests drive tick via TickForTest.
		// Still spawn a small goroutine so Stop / cancel semantics stay
		// uniform with the production path.
		go func() {
			<-loopCtx.Done()
			close(m.done)
		}()
		return
	}
	go m.loop(loopCtx)
}

// Stop cancels the tick goroutine and blocks until it returns. Safe to
// call more than once (subsequent calls are no-ops once cancel/done
// have fired).
func (m *SessionAwareScaleMonitor) Stop() {
	if m.cancel != nil {
		m.cancel()
	}
	<-m.done
}

// TickForTest runs one tick synchronously. Only meaningful for
// instances constructed via NewSessionAwareScaleMonitorForTest — the
// production Start path drives ticks off a timer.
func (m *SessionAwareScaleMonitor) TickForTest() {
	m.tick()
}

// NewSessionAwareScaleMonitorForTest is the test-only constructor that
// disables the internal timer so tests can call TickForTest to drive
// the loop deterministically. Production code should always use
// NewSessionAwareScaleMonitor.
func NewSessionAwareScaleMonitorForTest(pool *BigtableChannelPool, sessionCountFn func() int) *SessionAwareScaleMonitor {
	m := NewSessionAwareScaleMonitor(pool, sessionCountFn)
	m.tickHookEnabled = true
	return m
}

// ComputeTarget is the pure sizing function extracted so tests can
// assert the formula in isolation without wiring a real pool. Public
// so external harnesses (e.g. simulators) can call it with the same
// inputs the production monitor sees.
//
// target = ceil((sessions × SessionAwareHeadroom) / softMax)
// target = clamp(target, min, max)
//
// softMax <= 0 returns 0 — signals "config not usable yet".
func ComputeTarget(sessions, softMax, min, max int) int {
	if softMax <= 0 {
		return 0
	}
	target := (sessions*SessionAwareHeadroom + softMax - 1) / softMax
	if target > max {
		target = max
	}
	if target < min {
		target = min
	}
	return target
}

func (m *SessionAwareScaleMonitor) loop(ctx context.Context) {
	defer close(m.done)
	ticker := time.NewTicker(m.tickInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			m.tick()
		}
	}
}

func (m *SessionAwareScaleMonitor) tick() {
	cfg := m.config.Load()
	if cfg == nil {
		return // Config listener hasn't fired yet.
	}
	target := ComputeTarget(m.sessionCountFn(), cfg.softMax, cfg.min, cfg.max)
	if target <= 0 {
		return
	}
	have := m.pool.Num()
	switch {
	case target > have:
		m.pool.addConnections(target-have, cfg.max)
	case target < have:
		m.pool.removeConnections(have-target, cfg.min, have-target)
	}
}

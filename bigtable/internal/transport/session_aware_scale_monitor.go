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
	"sync"
	"sync/atomic"
	"time"

	spb "cloud.google.com/go/bigtable/apiv2/bigtablepb"
	btopt "cloud.google.com/go/bigtable/internal/option"
)

// SessionAwareHeadroom is the multiplier applied to per-channel session
// capacity when picking the target channel count. Sessions are
// long-lived bidi streams pinned to one channel — an AFE-side hiccup
// affects every session on that channel, so oversizing the pool is
// the correct safety-margin tradeoff.
//
// Note: SessionClientConfiguration.ChannelPoolConfiguration in
// google/bigtable/v2/session.proto only defines min_server_count,
// max_server_count, per_server_session_count, and mode (unlike
// SessionPoolConfiguration, it has no headroom field), so this mirrors
// the client-side constant in Java's ChannelPoolDpImpl (HEADROOM = 2.0).
// TODO(b/445071688): Source channel-pool headroom from
// ChannelPoolConfiguration / defaultClientConfig if a proto field is added.
const SessionAwareHeadroom = 2

// SessionAwareTickInterval is the default cadence for target
// recomputation. Sessions live minutes-to-hours; picking a shorter
// interval only adds jitter, not signal.
const SessionAwareTickInterval = 1 * time.Minute

// SessionAwareScaleMonitor sizes a BigtableChannelPool from live
// streaming-RPC count instead of RPC load. Complements (and, in the
// session client, replaces) DynamicScaleMonitor. On each tick:
//
//  1. Read pool.TotalStreamCount() — sum of streamingLoad across
//     every connEntry. Sessions are bidi streams (one streaming RPC
//     per open session), so this equals the total session count
//     riding on the pool without any cross-package plumbing.
//  2. Read the latest ChannelPoolConfiguration snapshot fed via
//     OnConfig (minServerCount/maxServerCount/perServerSessionCount).
//  3. Compute target = ceil((sessions * SessionAwareHeadroom) /
//     perServerSessionCount), clamped to [minServerCount, maxServerCount].
//  4. Call pool.addConnections / removeLeastStreamedConnections to reach target.
//
// Threading model:
//   - OnConfig races with the tick — snapshot is stored atomically.
//   - pool.TotalStreamCount is called from the tick goroutine only.
//   - Start and Stop are serialized via mu; Stop blocks on goroutine
//     exit via the done channel so callers can rely on "no more
//     mutations after Stop returns".
type SessionAwareScaleMonitor struct {
	// TODO: Support DirectAccess fallback pool sizing per
	// ChannelPoolConfiguration.Mode (DirectAccessWithFallback /
	// DirectAccessOnly / CloudPathOnly) in a follow-up.
	pool         *BigtableChannelPool
	config       atomic.Pointer[channelPoolConfigSnapshot]
	tickInterval time.Duration

	mu              sync.Mutex
	started         bool
	cancel          context.CancelFunc
	done            chan struct{}
	tickHookEnabled bool
}

// channelPoolConfigSnapshot is the immutable triple loaded on every
// tick, mirroring ChannelPoolConfiguration's sizing fields. Kept together in a
// pointer so the tick reads the tuple atomically — otherwise a config update
// mid-formula could feed a half-old / half-new
// (minServerCount, maxServerCount, perServerSessionCount) into clamp and
// produce a nonsense target.
//
// TODO: Also enforce perServerSessionCount per channel/AFE during session
// stream placement and opportunistic scale-up (matching Java's
// ChannelPoolDpImpl.newStream) in a follow-up; today it drives periodic pool
// sizing here.
type channelPoolConfigSnapshot struct {
	minServerCount        int
	maxServerCount        int
	perServerSessionCount int
}

// NewSessionAwareScaleMonitor constructs a monitor bound to pool.
// Reads pool.TotalStreamCount() on every tick — no cross-package
// closure needed because sessions register as streaming RPCs on the
// pool naturally when they open (bidi stream = one streamingLoad
// increment on the picked connEntry).
func NewSessionAwareScaleMonitor(pool *BigtableChannelPool) *SessionAwareScaleMonitor {
	return &SessionAwareScaleMonitor{
		pool:         pool,
		tickInterval: SessionAwareTickInterval,
		done:         make(chan struct{}),
	}
}

// OnConfig accepts a ChannelPoolConfiguration update — designed as
// the callback registered with ClientConfigurationManager.
// AddChannelPoolConfigListener. nil config is ignored. Any unset or
// non-positive field falls back to defaultClientConfig's
// ChannelConfiguration so partial server configs inherit client defaults.
func (m *SessionAwareScaleMonitor) OnConfig(cfg *spb.SessionClientConfiguration_ChannelPoolConfiguration) {
	if cfg == nil {
		return
	}
	defaults := defaultClientConfig.GetSessionConfiguration().GetChannelConfiguration()
	minServers := int(cfg.GetMinServerCount())
	maxServers := int(cfg.GetMaxServerCount())
	perServerSessions := int(cfg.GetPerServerSessionCount())
	if defaults != nil {
		if maxServers <= 0 {
			maxServers = int(defaults.GetMaxServerCount())
		}
		if minServers <= 0 {
			minServers = min(int(defaults.GetMinServerCount()), maxServers)
		}
		if perServerSessions <= 0 {
			perServerSessions = int(defaults.GetPerServerSessionCount())
		}
	}
	m.config.Store(&channelPoolConfigSnapshot{
		minServerCount:        minServers,
		maxServerCount:        maxServers,
		perServerSessionCount: perServerSessions,
	})
}

// Start begins the tick loop under ctx. The goroutine exits when ctx
// is cancelled or Stop is called (both routes close the done channel).
// Safe to call multiple times; subsequent calls after the first are no-ops.
func (m *SessionAwareScaleMonitor) Start(ctx context.Context) {
	m.mu.Lock()
	if m.started {
		m.mu.Unlock()
		return
	}
	m.started = true
	loopCtx, cancel := context.WithCancel(ctx)
	m.cancel = cancel
	tickHook := m.tickHookEnabled
	m.mu.Unlock()

	if tickHook {
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
// call before Start or more than once (subsequent calls are no-ops).
func (m *SessionAwareScaleMonitor) Stop() {
	m.mu.Lock()
	if !m.started {
		m.mu.Unlock()
		return
	}
	cancel := m.cancel
	m.mu.Unlock()

	cancel()
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
func NewSessionAwareScaleMonitorForTest(pool *BigtableChannelPool) *SessionAwareScaleMonitor {
	m := NewSessionAwareScaleMonitor(pool)
	m.tickHookEnabled = true
	return m
}

// ComputeTarget is the pure sizing function extracted so tests can
// assert the formula in isolation without wiring a real pool. Public
// so external harnesses (e.g. simulators) can call it with the same
// inputs the production monitor sees.
//
//	target = ceil((sessions * SessionAwareHeadroom) / softMax)
//	target = clamp(target, minConns, maxConns)
//
// Non-positive softMax or maxConns, or inverted bounds (minConns > maxConns),
// return 0 — signaling "config not usable yet".
func ComputeTarget(sessions, softMax, minConns, maxConns int) int {
	if softMax <= 0 || maxConns <= 0 || minConns > maxConns {
		return 0
	}
	if minConns < 1 {
		minConns = 1
	}
	if sessions < 0 {
		sessions = 0
	}
	target := (sessions*SessionAwareHeadroom + softMax - 1) / softMax
	if target > maxConns {
		target = maxConns
	}
	if target < minConns {
		target = minConns
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
		btopt.Debugf(nil, "bigtable_connpool: SessionAwareScaleMonitor tick: no config yet, skip\n")
		return
	}
	sessions := m.pool.TotalStreamCount()
	target := ComputeTarget(sessions, cfg.perServerSessionCount, cfg.minServerCount, cfg.maxServerCount)
	have := m.pool.Num()
	// One line per tick: inputs + decision. Operators tailing the debug
	// log see the sizer's reasoning without a separate metric.
	action := "no-op"
	if target > 0 {
		switch {
		case target > have:
			action = "grow"
		case target < have:
			action = "shrink"
		}
	}
	btopt.Debugf(nil, "bigtable_connpool: SessionAwareScaleMonitor tick: streams=%d perServerSessionCount=%d min=%d max=%d target=%d have=%d action=%s\n",
		sessions, cfg.perServerSessionCount, cfg.minServerCount, cfg.maxServerCount, target, have, action)
	if target <= 0 {
		return
	}
	minFloor := max(cfg.minServerCount, 1)
	switch {
	case target > have:
		m.pool.addConnections(target-have, cfg.maxServerCount)
	case target < have:
		m.pool.removeLeastStreamedConnections(have-target, minFloor, have-target)
	}
}

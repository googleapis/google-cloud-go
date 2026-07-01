// Copyright 2026 Google LLC
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//	http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package internal

import (
	"context"
	"log"
	"math/rand"
	"sync"
	"sync/atomic"
	"time"

	bigtablepb "cloud.google.com/go/bigtable/apiv2/bigtablepb"
	btopt "cloud.google.com/go/bigtable/internal/option"
	"google.golang.org/grpc"
	"google.golang.org/grpc/metadata"
	"google.golang.org/protobuf/proto"
)

// minPollingInterval is the minimum interval enforced between successive
// GetClientConfiguration polls, regardless of the server-supplied value. It
// protects the control plane from misconfigured clients overwhelming it.
const minPollingInterval = 1 * time.Minute

// maxBackoffSeconds caps the per-attempt randomized exponential backoff used
// between failed GetClientConfiguration retries. Capping prevents the 1<<i
// shift from overflowing int when the configured retry count is large, and
// keeps a prolonged outage from stretching the retry window to absurd
// durations.
const maxBackoffSeconds = 30

// pollDeadline is the per-RPC timeout applied to every GetClientConfiguration
// attempt — both the eager initial poll spawned by Start() and each periodic
// poll spawned by pollingLoop(). Kept short so a stuck control-plane request
// can't pin a polling slot for the full polling interval.
const pollDeadline = 5 * time.Second

// clientConfig holds configuration for the client.
type clientConfig struct {
	Polling          pollingConfig
	Session          sessionConfig
	HasSessionConfig bool
}

// Clone returns a deep copy of the clientConfig.
func (c clientConfig) Clone() clientConfig {
	// return by value so it copies but future proofing
	return c
}

type pollingConfig struct {
	// PollingInterval is the wall-clock period between successive
	// GetClientConfiguration polls. Floored at minPollingInterval.
	PollingInterval time.Duration
	// ValidityDuration is how long a successfully-polled configuration
	// remains in effect before, on a failed poll, the manager falls back to
	// the default config. Capped at 100 years to avoid time.Duration overflow.
	ValidityDuration time.Duration
	// MaxRPCRetryCount is the number of additional GetClientConfiguration
	// retries (beyond the first attempt) before a single poll() call gives
	// up. Each retry is preceded by randomized exponential backoff capped at
	// maxBackoffSeconds.
	MaxRPCRetryCount int
	// StopPolling, when true, tells the pollingLoop to exit. It is set when
	// the server picks the StopPolling case of the ClientConfiguration.Polling
	// oneof — a backstop the control plane can flip to halt excessive
	// GetClientConfiguration RPCs. The current configuration stays in effect
	// (no fallback to defaults); the loop just stops issuing further polls.
	StopPolling bool
}

type sessionConfig struct {
	// SessionLoad is the server-driven fraction of requests to route through
	// the session pool vs. the classic channel pool. 0.0 = all-classic,
	// 1.0 = all-session.
	SessionLoad float64
	// ChannelPool configures the session pool's underlying channel pool.
	ChannelPool channelPoolConfig
	// SessionPool configures the session pool itself (size, churn budget,
	// load balancing).
	SessionPool sessionPoolConfig
}

// channelPoolMode mirrors the SessionClientConfiguration_ChannelPoolConfiguration
// Mode oneof and tells session-pool factories whether the server wants the
// client to use Direct Access, fall back to Cloud Path on errors, or skip
// Direct Access entirely. The clientConfigDirectAccessChecker consults this
// when deciding whether to adopt a direct-access connection.
type channelPoolMode int

const (
	// modeDirectAccessWithFallback (server default) — try Direct Access, but
	// fall back to Cloud Path when the failure rate breaches the threshold.
	modeDirectAccessWithFallback channelPoolMode = iota
	// modeDirectAccessOnly — Direct Access only, no Cloud Path fallback.
	modeDirectAccessOnly
	// modeCloudPathOnly — never use Direct Access.
	modeCloudPathOnly
)

type channelPoolConfig struct {
	// MinServerCount is the lower bound on the number of subchannels the
	// session pool's channel pool is allowed to shrink to.
	MinServerCount int
	// MaxServerCount is the upper bound on the number of subchannels the
	// session pool's channel pool is allowed to grow to.
	MaxServerCount int
	// PerServerSessionCount is the target number of sessions hosted on each
	// subchannel; the session pool uses it to derive how many subchannels to
	// keep open for a given session count.
	PerServerSessionCount int
	// Mode is the Direct Access decision the server wants the client to
	// honor. See the channelPoolMode constants.
	Mode channelPoolMode
	// DirectAccessCheckInterval is how often clientConfigDirectAccessChecker
	// re-evaluates whether to adopt a direct-access connection.
	DirectAccessCheckInterval time.Duration
	// DirectAccessErrorThreshold is the failure-rate threshold above which
	// modeDirectAccessWithFallback gives up on Direct Access and falls back
	// to Cloud Path.
	DirectAccessErrorThreshold float32
}

type sessionPoolConfig struct {
	// Headroom is the fraction of spare capacity the session pool keeps
	// above current demand (e.g., 0.5 = pool is sized to 1.5x demand).
	Headroom float32
	// MinSessionCount is the lower bound on the pool size; the pool will
	// not shrink below this count even if demand drops.
	MinSessionCount int
	// MaxSessionCount is the upper bound on the pool size; the pool will
	// not grow above this count even if demand spikes.
	MaxSessionCount int
	// NewSessionCreationBudget caps how many concurrent CreateSession RPCs
	// the pool will have in flight; further requests queue behind the budget.
	NewSessionCreationBudget int
	// NewSessionCreationPenalty is the cool-down a CreateSession failure
	// imposes on its target subchannel before the pool will retry it.
	NewSessionCreationPenalty time.Duration
	// ConsecutiveSessionFailureThreshold is how many back-to-back vRPC
	// failures on a session before the pool retires it.
	ConsecutiveSessionFailureThreshold int
	// NewSessionQueueLength caps the depth of the queue of pending
	// CreateSession requests waiting on NewSessionCreationBudget.
	NewSessionQueueLength int
	// LoadBalancing selects how the pool picks a session for a vRPC.
	LoadBalancing loadBalancingOptions
}

type loadBalancingStrategy int

// Load-balancing strategy variants surfaced via the SessionPool's
// LoadBalancingOptions. Mirrors the variants of the
// SessionClientConfiguration.LoadBalancingOptions oneof.
const (
	StrategyLeastInFlight loadBalancingStrategy = iota
	StrategyRandom
	StrategyPeakEwma
)

type loadBalancingOptions struct {
	// Strategy is the picking algorithm — see the StrategyXxx constants.
	Strategy loadBalancingStrategy
	// RandomSubsetSize, when non-zero, restricts strategies like
	// StrategyLeastInFlight or StrategyPeakEwma to scoring only this many
	// randomly-sampled sessions per pick instead of the whole pool, to
	// bound per-pick cost on large pools.
	RandomSubsetSize int
}

// configListener is a callback function for configuration changes.
//
// seq is the manager's monotonically-increasing configuration sequence
// number (m.configSeq) at the moment of the fire. It is bumped each time
// the polled configuration genuinely changes and on the validity-window
// fallback to default. addListener and poll() both serialize their fires
// through m.mu, so a listener observes seq values in strictly increasing
// order and no explicit listener-side guard is required.
type configListener func(newConfig clientConfig, seq int64)

// ClientConfigurationManager manages the dynamic client configuration for Bigtable.
// It periodically polls for client configuration updates via GetClientConfiguration RPCs.
type ClientConfigurationManager struct {
	client        bigtablepb.BigtableClient
	instanceName  string
	appProfileID  string
	metadata      metadata.MD
	defaultConfig clientConfig
	logger        *log.Logger

	// ctx scopes the polling loop and every per-poll RPC. Start() derives it
	// from its parent ctx; Close() invokes cancel to wake pollingLoop and
	// abort any in-flight poll(). cancel is initialized to a no-op so a
	// Close-before-Start call is safe.
	ctx    context.Context
	cancel context.CancelFunc
	// closed is flipped true by the first Close() via CompareAndSwap, which
	// both makes Close idempotent and arms the read-side gate consulted by
	// poll() before listener fan-out. pollsWG.Wait() guarantees poll() has
	// returned before Close() returns, but not that listeners haven't already
	// fired in the interim — closed closes that window.
	closed atomic.Bool
	// pollsWG tracks in-flight poll() invocations spawned by Start() and
	// pollingLoop(). Close() waits on it so callers can rely on no listener
	// callbacks firing after Close() returns.
	pollsWG sync.WaitGroup

	mu            sync.RWMutex
	currentConfig clientConfig
	// configSeq is a monotonically increasing sequence number incremented every time the configuration changes.
	configSeq      int64
	validUntil     time.Time
	listeners      map[int]configListener
	nextListenerID int
}

// NewClientConfigurationManager creates a new ClientConfigurationManager.
func NewClientConfigurationManager(
	client bigtablepb.BigtableClient,
	instanceName string,
	appProfileID string,
	md metadata.MD,
	logger *log.Logger,
) *ClientConfigurationManager {
	return &ClientConfigurationManager{
		client:        client,
		instanceName:  instanceName,
		appProfileID:  appProfileID,
		metadata:      md,
		defaultConfig: defaultClientConfig,
		currentConfig: defaultClientConfig,
		validUntil:    time.Now().Add(time.Hour * 24 * 365 * 100), //  default far in future
		listeners:     make(map[int]configListener),
		logger:        logger,
		// No-op until Start() wires the real cancel; makes Close-before-Start safe.
		cancel: func() {},
	}
}

// Start begins the polling process. The passed-in ctx is the parent for the
// manager's own lifetime ctx — cancelling it (or calling Close) tears down
// the polling loop and any in-flight poll RPC.
func (m *ClientConfigurationManager) Start(ctx context.Context) {
	btopt.Debugf(m.logger, "bigtable: starting client configuration manager for instance %q, app profile %q", m.instanceName, m.appProfileID)
	m.ctx, m.cancel = context.WithCancel(ctx)

	// Eager initial poll, scoped to m.ctx so Close()'s cancel reaches it too.
	m.pollsWG.Add(1)
	go func() {
		defer m.pollsWG.Done()
		pollCtx, cancel := context.WithTimeout(m.ctx, pollDeadline)
		defer cancel()
		m.poll(pollCtx)
	}()

	// Start background polling
	go m.pollingLoop()
}

// Close stops the polling process.
//
// Close is safe to call multiple times; only the first call performs teardown
// (the CompareAndSwap on m.closed is the idempotency gate). It also waits for
// any in-flight poll() invocations spawned by Start() and pollingLoop() to
// return before it itself returns. Combined with the closed gate that poll()
// consults before firing listeners, this guarantees no listener callbacks
// fire after Close() returns — so SessionPools registered via
// AddSessionPoolListener can be Close'd immediately after this returns
// without racing against a late configuration callback.
func (m *ClientConfigurationManager) Close() {
	// CAS makes Close idempotent; it also arms the read-side gate inside
	// poll() before we cancel and start waiting.
	if !m.closed.CompareAndSwap(false, true) {
		return
	}
	btopt.Debugf(m.logger, "bigtable: closing client configuration manager")
	m.cancel()
	// Wait for in-flight polls (including their listener callbacks, which
	// are short-circuited by isClosed()) to finish before returning.
	m.pollsWG.Wait()
}

// isClosed reports whether Close() has begun teardown. poll() consults this
// before invoking listeners so callbacks do not fire after Close() returns.
func (m *ClientConfigurationManager) isClosed() bool {
	return m.closed.Load()
}

// getConfig returns the current configuration.
func (m *ClientConfigurationManager) getConfig() clientConfig {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.currentConfig.Clone()
}

// addListener adds a listener for configuration changes.
//
// The registration-time fire happens under m.mu, so it is serialized
// with poll()'s fan-out: poll() cannot snapshot the listener map (and
// therefore cannot fire this listener with a newer seq) while we are
// delivering the initial (cfg, seq) here. That guarantees the listener
// observes seq values in strictly increasing order without any per-
// listener wrapping.
//
// As a consequence, the listener callback runs under m.mu. Listeners
// must not call back into the manager (e.g. getConfig, or any other
// method that takes m.mu) from within the callback or they will
// deadlock. The wrappers AddSessionPoolListener / AddSessionLoadListener
// and their downstream consumers stay self-contained, so the invariant
// holds today.
func (m *ClientConfigurationManager) addListener(listener configListener) func() {
	m.mu.Lock()
	defer m.mu.Unlock()

	id := m.nextListenerID
	m.nextListenerID++
	if m.listeners == nil {
		m.listeners = make(map[int]configListener)
	}
	m.listeners[id] = listener

	btopt.Debugf(m.logger, "bigtable: adding configuration listener (id: %d)", id)
	listener(m.currentConfig.Clone(), m.configSeq)

	return func() {
		m.mu.Lock()
		delete(m.listeners, id)
		m.mu.Unlock()
	}
}

// AddSessionPoolListener registers a callback that receives raw
// SessionPoolConfiguration updates.
//
// The wrapper extracts only the SessionPool slice from each incoming
// clientConfig and short-circuits when that slice is unchanged from the
// previous delivery (proto-equality), so unrelated changes — e.g., a new
// PollingInterval — won't redundantly fire a SessionPool resize. Mirrors
// the per-listener diff in Java's ListenerEntry.maybeNotify.
func (m *ClientConfigurationManager) AddSessionPoolListener(listener func(*bigtablepb.SessionClientConfiguration_SessionPoolConfiguration)) func() {
	var (
		diffMu  sync.Mutex
		hasPrev bool
		prev    *bigtablepb.SessionClientConfiguration_SessionPoolConfiguration
	)
	return m.addListener(func(cfg clientConfig, seq int64) {
		spCfg := &bigtablepb.SessionClientConfiguration_SessionPoolConfiguration{
			MinSessionCount: int32(cfg.Session.SessionPool.MinSessionCount),
			MaxSessionCount: int32(cfg.Session.SessionPool.MaxSessionCount),
		}
		diffMu.Lock()
		if hasPrev && proto.Equal(prev, spCfg) {
			diffMu.Unlock()
			return
		}
		hasPrev = true
		prev = spCfg
		diffMu.Unlock()
		listener(spCfg)
	})
}

// AddSessionLoadListener registers a callback that receives the server-driven
// SessionLoad value (0.0 = all-classic, 1.0 = all-session) on every
// configuration update from a successful poll. Returns an unregister thunk.
//
// Unlike AddSessionPoolListener, this skips the immediate registration-time
// fire that would otherwise deliver the default config (seq=0). Callers rely
// on the bootstrap value they passed to NewDiverter remaining in effect until
// the control plane actually responds — firing with seq=0's default
// SessionLoad=0 would silently clobber that bootstrap.
//
// Subsequent fires are filtered by per-listener load diff: if the polled
// SessionLoad equals the value last delivered to this listener, the inner
// callback is skipped so SessionLoad reassignment doesn't ripple to the
// Diverter on polls where only unrelated fields (e.g., PollingInterval)
// changed.
func (m *ClientConfigurationManager) AddSessionLoadListener(listener func(load float64)) func() {
	var (
		diffMu  sync.Mutex
		hasPrev bool
		prev    float64
	)
	return m.addListener(func(cfg clientConfig, seq int64) {
		if seq == 0 {
			return
		}
		load := cfg.Session.SessionLoad
		diffMu.Lock()
		if hasPrev && prev == load {
			diffMu.Unlock()
			return
		}
		hasPrev = true
		prev = load
		diffMu.Unlock()
		listener(load)
	})
}

// pollingLoop continuously polls the Bigtable control plane at the configured interval.
// It enforces a minimum interval (minPollingInterval) to protect the control plane from DDoSes.
func (m *ClientConfigurationManager) pollingLoop() {
	for {
		m.mu.RLock()
		cfg := m.currentConfig
		m.mu.RUnlock()

		if cfg.Polling.StopPolling {
			btopt.Debugf(m.logger, "bigtable: server requested polling stop; exiting pollingLoop")
			return
		}

		interval := cfg.Polling.PollingInterval
		if interval < minPollingInterval {
			interval = minPollingInterval
		}

		select {
		case <-m.ctx.Done():
			return
		case <-time.After(interval):
			// Track each poll with pollsWG so Close() can wait for in-flight
			// polls (and their listener callbacks) to finish before returning.
			m.pollsWG.Add(1)
			pollCtx, pollCancel := context.WithTimeout(m.ctx, pollDeadline)
			m.poll(pollCtx)
			pollCancel()
			m.pollsWG.Done()
		}
	}
}

// fetchClientConfiguration issues GetClientConfiguration with randomized
// exponential backoff between attempts (per the current config's
// MaxRPCRetryCount; backoff capped at maxBackoffSeconds). It returns the
// server's response on the first successful attempt, or the last RPC error
// after exhausting retries. If ctx is cancelled mid-backoff it returns
// ctx.Err() immediately so the caller can distinguish a shutdown from a
// real RPC failure and skip any fallback-to-default work.
func (m *ClientConfigurationManager) fetchClientConfiguration(ctx context.Context) (*bigtablepb.ClientConfiguration, error) {
	req := &bigtablepb.GetClientConfigurationRequest{
		InstanceName: m.instanceName,
		AppProfileId: m.appProfileID,
	}
	ctx = metadata.NewOutgoingContext(ctx, m.metadata)

	m.mu.RLock()
	maxRetries := m.currentConfig.Polling.MaxRPCRetryCount
	m.mu.RUnlock()

	var resp *bigtablepb.ClientConfiguration
	var err error
	for i := 0; i <= maxRetries; i++ {
		var header, trailer metadata.MD
		rpcStart := time.Now()
		resp, err = m.client.GetClientConfiguration(ctx, req, grpc.Header(&header), grpc.Trailer(&trailer))
		rpcDuration := time.Since(rpcStart)
		if err == nil {
			btopt.Debugf(m.logger, "bigtable: GetClientConfiguration RPC attempt %d completed successfully in %v", i, rpcDuration)
			return resp, nil
		}
		btopt.Debugf(m.logger, "bigtable: GetClientConfiguration RPC attempt %d failed in %v: %v", i, rpcDuration, err)
		if i == maxRetries {
			break
		}
		// Cap the shift width so 1<<i can't overflow on absurd
		// MaxRPCRetryCount values, then cap the wait at maxBackoffSeconds
		// so prolonged outages don't stretch a single retry into hours.
		backoffLimit := min(1<<min(i, 30), maxBackoffSeconds)
		delay := time.Duration(rand.Intn(backoffLimit)) * time.Second
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-time.After(delay):
		}
	}
	return nil, err
}

// poll queries the GetClientConfiguration API and triggers registered listeners with the new configuration.
// If the poll fails and the previous configuration's validity has expired, it falls back to the default config.
func (m *ClientConfigurationManager) poll(ctx context.Context) {
	btopt.Debugf(m.logger, "bigtable: polling client configuration...")
	resp, err := m.fetchClientConfiguration(ctx)

	// Caller cancelled (typically Close()). Don't trigger fallback-to-default
	// or fire any listeners — pools they target are about to be torn down.
	if ctx.Err() != nil {
		return
	}

	if err != nil {
		btopt.Debugf(m.logger, "bigtable: failed to poll client configuration: %v", err)
		m.mu.Lock()
		var listeners []configListener
		var cfgToNotify clientConfig
		var seq int64
		// Fall back to default configuration if validity window has expired.
		// Reset validUntil to the same far-future sentinel used at
		// construction so subsequent failed polls don't re-trigger this
		// branch every interval and spam listeners with the same default.
		if time.Now().After(m.validUntil) {
			btopt.Debugf(m.logger, "bigtable: client configuration validity window expired, falling back to default config")
			m.currentConfig = m.defaultConfig
			m.configSeq++
			seq = m.configSeq
			m.validUntil = time.Now().Add(time.Hour * 24 * 365 * 100)
			listeners = make([]configListener, 0, len(m.listeners))
			for _, l := range m.listeners {
				listeners = append(listeners, l)
			}
			cfgToNotify = m.currentConfig
		}
		m.mu.Unlock()

		// Suppress listener fires after Close() has begun teardown — the
		// SessionPools they target may already be Close'd.
		if listeners != nil && !m.isClosed() {
			for _, l := range listeners {
				l(cfgToNotify.Clone(), seq)
			}
		}
		return
	}

	parsedResp := parseConfig(resp, m.defaultConfig)
	btopt.Debugf(m.logger, "bigtable: successfully polled new client configuration. validityDuration: %v", parsedResp.Polling.ValidityDuration)

	m.mu.Lock()
	// Always refresh the validity window — the server's reply re-affirms
	// the current config, so we shouldn't later fall back to default just
	// because the prior ValidityDuration elapsed during a no-op poll.
	m.validUntil = time.Now().Add(parsedResp.Polling.ValidityDuration)
	// clientConfig is a fully-comparable value struct (no slices/maps/
	// pointers/funcs), so == is a complete deep equality check. When the
	// server returns the same config, skip the seq bump and listener
	// fan-out so downstream consumers don't re-run idempotent work
	// (channel-pool reshapes, session-pool resizes) every poll.
	if parsedResp == m.currentConfig {
		seq := m.configSeq
		m.mu.Unlock()
		btopt.Debugf(m.logger, "bigtable: client configuration unchanged (seq=%d), refreshed validity, skipping listener fan-out", seq)
		return
	}
	m.currentConfig = parsedResp
	m.configSeq++
	seq := m.configSeq

	listeners := make([]configListener, 0, len(m.listeners))
	for _, l := range m.listeners {
		listeners = append(listeners, l)
	}
	cfgToNotify := m.currentConfig
	m.mu.Unlock()

	// Suppress listener fires after Close() has begun teardown — the
	// SessionPools they target may already be Close'd.
	if m.isClosed() {
		return
	}
	for _, l := range listeners {
		l(cfgToNotify.Clone(), seq)
	}
}

// parseConfig converts the protobuf ClientConfiguration message into the internal clientConfig structure,
// validating bounds such as minPollingInterval and capping validity duration to prevent integer overflows.
func parseConfig(protoCfg *bigtablepb.ClientConfiguration, defaultCfg clientConfig) clientConfig {
	res := defaultCfg

	if protoCfg == nil {
		return res
	}

	if p := protoCfg.GetPollingConfiguration(); p != nil {
		res.Polling = parsePollingConfig(p, res.Polling)
	}
	if protoCfg.GetStopPolling() {
		res.Polling.StopPolling = true
	}

	if protoCfg.SessionConfiguration != nil {
		s := protoCfg.SessionConfiguration
		res.Session = parseSessionConfig(s, res.Session)
		res.HasSessionConfig = s.SessionLoad > 0
	} else {
		res.HasSessionConfig = false
	}

	return res
}

func parsePollingConfig(p *bigtablepb.ClientConfiguration_PollingConfiguration, defaultCfg pollingConfig) pollingConfig {
	res := defaultCfg
	if p == nil {
		return res
	}
	if p.PollingInterval != nil {
		res.PollingInterval = p.PollingInterval.AsDuration()
	}
	if res.PollingInterval < minPollingInterval {
		res.PollingInterval = minPollingInterval
	}
	if p.ValidityDuration != nil {
		res.ValidityDuration = p.ValidityDuration.AsDuration()
		if res.ValidityDuration > 100*365*24*time.Hour {
			res.ValidityDuration = 100 * 365 * 24 * time.Hour
		}
	}
	res.MaxRPCRetryCount = int(p.MaxRpcRetryCount)
	return res
}

func parseSessionConfig(s *bigtablepb.SessionClientConfiguration, defaultCfg sessionConfig) sessionConfig {
	res := defaultCfg
	if s == nil {
		return res
	}
	res.SessionLoad = float64(s.SessionLoad)
	if s.ChannelConfiguration != nil {
		res.ChannelPool = parseChannelPoolConfig(s.ChannelConfiguration, res.ChannelPool)
	}
	if s.SessionPoolConfiguration != nil {
		res.SessionPool = parseSessionPoolConfig(s.SessionPoolConfiguration, res.SessionPool)
	}
	return res
}

func parseChannelPoolConfig(cc *bigtablepb.SessionClientConfiguration_ChannelPoolConfiguration, defaultCfg channelPoolConfig) channelPoolConfig {
	res := defaultCfg
	if cc == nil {
		return res
	}
	res.MinServerCount = int(cc.MinServerCount)
	res.MaxServerCount = int(cc.MaxServerCount)
	res.PerServerSessionCount = int(cc.PerServerSessionCount)
	// Switch on the proto's Mode oneof so the manager preserves the server's
	// Direct Access decision. clientConfigDirectAccessChecker reads
	// channelPoolConfig.Mode to decide whether to adopt the direct-access
	// dialer for a session-pool channel.
	switch m := cc.Mode.(type) {
	case *bigtablepb.SessionClientConfiguration_ChannelPoolConfiguration_DirectAccessWithFallback_:
		res.Mode = modeDirectAccessWithFallback
		if fb := m.DirectAccessWithFallback; fb != nil {
			if fb.CheckInterval != nil {
				res.DirectAccessCheckInterval = fb.CheckInterval.AsDuration()
			}
			res.DirectAccessErrorThreshold = fb.ErrorRateThreshold
		}
	case *bigtablepb.SessionClientConfiguration_ChannelPoolConfiguration_DirectAccessOnly_:
		res.Mode = modeDirectAccessOnly
	case *bigtablepb.SessionClientConfiguration_ChannelPoolConfiguration_CloudPathOnly_:
		res.Mode = modeCloudPathOnly
	}
	return res
}

func parseSessionPoolConfig(sp *bigtablepb.SessionClientConfiguration_SessionPoolConfiguration, defaultCfg sessionPoolConfig) sessionPoolConfig {
	res := defaultCfg
	if sp == nil {
		return res
	}
	res.Headroom = sp.Headroom
	res.MinSessionCount = int(sp.MinSessionCount)
	res.MaxSessionCount = int(sp.MaxSessionCount)
	res.NewSessionCreationBudget = int(sp.NewSessionCreationBudget)
	if sp.NewSessionCreationPenalty != nil {
		res.NewSessionCreationPenalty = sp.NewSessionCreationPenalty.AsDuration()
	}
	res.ConsecutiveSessionFailureThreshold = int(sp.ConsecutiveSessionFailureThreshold)
	res.NewSessionQueueLength = int(sp.NewSessionQueueLength)

	if sp.LoadBalancingOptions != nil {
		lbo := sp.LoadBalancingOptions
		switch opt := lbo.LoadBalancingStrategy.(type) {
		case *bigtablepb.LoadBalancingOptions_Random_:
			res.LoadBalancing.Strategy = StrategyRandom
		case *bigtablepb.LoadBalancingOptions_LeastInFlight_:
			res.LoadBalancing.Strategy = StrategyLeastInFlight
			if opt.LeastInFlight != nil {
				res.LoadBalancing.RandomSubsetSize = int(opt.LeastInFlight.RandomSubsetSize)
			}
		case *bigtablepb.LoadBalancingOptions_PeakEwma_:
			res.LoadBalancing.Strategy = StrategyPeakEwma
			if opt.PeakEwma != nil {
				res.LoadBalancing.RandomSubsetSize = int(opt.PeakEwma.RandomSubsetSize)
			}
		}
	}
	return res
}

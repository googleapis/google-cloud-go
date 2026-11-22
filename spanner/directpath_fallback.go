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

package spanner

import (
	"context"
	"errors"
	"log"
	"maps"
	"math/rand/v2"
	"os"
	"reflect"
	"slices"
	"strconv"
	"sync"
	"sync/atomic"
	"time"

	"cloud.google.com/go/spanner/internal"
	"cloud.google.com/go/spanner/internal/transport"
	"github.com/GoogleCloudPlatform/grpc-gcp-go/grpcgcp"
	"github.com/googleapis/gax-go/v2"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/metric"
	"google.golang.org/api/option"
	"google.golang.org/api/option/internaloption"
	gtransport "google.golang.org/api/transport/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials/alts"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/peer"
	"google.golang.org/grpc/status"
)

// Fallback triggers when at least one DirectPath RPC finishes during a window
// and 100% of them fail. These match the thresholds used with grpc-gcp-go's
// GCPFallback.
const (
	directPathFallbackErrorRateThreshold = 1.0
	directPathFallbackMinFailedCalls     = 1

	// Channel names of the eef.* metrics, as grpc-gcp-go reports them.
	directPathFallbackPrimaryChannelName  = "primary"
	directPathFallbackFallbackChannelName = "fallback"

	directPathFallbackRecoveryEnv              = "GOOGLE_SPANNER_EXPERIMENTAL_GCP_FALLBACK_RECOVERY"
	directPathRecoveryProbeNotDirectPathReason = "NOT_DIRECTPATH"
)

// directPathFallbackPeriod is the length of one error-rate window. Tests
// lower it before creating clients.
var directPathFallbackPeriod = 3 * time.Minute

// Recovery probing defaults; tests override before creating clients.
var (
	defaultDirectPathRecoveryConfig = directPathFallbackRecoveryConfig{
		probeInterval:       1 * time.Minute,
		probeRPCTimeout:     10 * time.Second,
		minRecoveryProbes:   10,
		minRecoveryDuration: 10 * time.Minute,
		probeJitter: func(d time.Duration) time.Duration {
			if maxJitter := d / 4; maxJitter > 0 {
				return rand.N(maxJitter)
			}
			return 0
		},
	}
	isDirectPathPeerFn = func(p *peer.Peer) bool {
		if p == nil {
			return false
		}
		_, isALTS := p.AuthInfo.(alts.AuthInfo)
		return isALTS
	}
)

func isDirectPathFallbackRecoveryEnabled() bool {
	enabled, _ := strconv.ParseBool(os.Getenv(directPathFallbackRecoveryEnv))
	return enabled
}

// dialDirectPathFallbackCloudPath dials CloudPath connections. Tests replace
// it before creating clients.
var dialDirectPathFallbackCloudPath = gtransport.DialPool

// Layout of fallbackWindow.outcomes: bit 63 marks a sealed window, bits 32-62
// count failures and bits 0-31 count successes.
const (
	fallbackWindowSealed  uint64 = 1 << 63
	fallbackWindowFailure uint64 = 1 << 32
	fallbackWindowSuccess uint64 = 1
)

// fallbackWindow represents one error-rate evaluation window. Once stored in
// directPathFallback.window, all fields except outcomes are immutable, so
// concurrent RPCs can read generation, fallback, and start without locking.
type fallbackWindow struct {
	generation uint64 // increases on fallback
	fallback   bool   // new attempts go to CloudPath
	start      time.Time
	// outcomes packs both counts so that one atomic operation reads or seals
	// a consistent pair.
	outcomes atomic.Uint64
}

func unpackFallbackOutcomes(v uint64) (failures, successes uint64) {
	return (v &^ fallbackWindowSealed) >> 32, v & (fallbackWindowFailure - 1)
}

type fallbackWindowResult struct {
	failures, successes uint64
	switched            bool
}

// directPathFallback is the DirectPath to CloudPath switch shared by every
// channelSlot of a channel pool. By default the switch is one way; when
// recovery.enabled is true, slots independently probe DirectPath and recover.
type directPathFallback struct {
	window  atomic.Pointer[fallbackWindow] // the only routing and counting state
	closeMu sync.Mutex                     // serializes window closes and recovery resets
	period  time.Duration

	// dialCloud dials one connection; errNoCloudPath if the client has none.
	dialCloud func(context.Context) (gtransport.ConnPool, error)

	// ctx bounds CloudPath dials, recovery probes, and the window loop.
	ctx    context.Context
	cancel context.CancelFunc
	done   chan struct{}

	metrics *directPathFallbackMetrics
	logger  *log.Logger

	hooksMu  sync.Mutex
	onSwitch []func()

	// Recovery configuration and state.
	recovery        directPathFallbackRecoveryConfig
	probeSession    atomic.Value // string
	probingDisabled atomic.Bool

	slotsMu sync.Mutex
	slots   map[*channelSlot]struct{}
}

type directPathFallbackRecoveryConfig struct {
	probeMD             metadata.MD
	enabled             bool
	probeInterval       time.Duration
	probeRPCTimeout     time.Duration
	minRecoveryProbes   int
	minRecoveryDuration time.Duration
	probeJitter         func(time.Duration) time.Duration
}

// newDirectPathFallback returns the fallback state of a pool on DirectPath
// and starts closing a window every period. The caller must call close. No
// eef.* metrics are recorded if mp is nil or counter creation fails.
func newDirectPathFallback(mp metric.MeterProvider, logger *log.Logger, dialCloud func(context.Context) (gtransport.ConnPool, error), period time.Duration, recovery directPathFallbackRecoveryConfig) *directPathFallback {
	metrics, err := newDirectPathFallbackMetrics(mp, recovery.enabled)
	if err != nil {
		logf(logger, "spanner: DirectPath fallback metrics are disabled: %v", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	f := &directPathFallback{
		period:    period,
		dialCloud: dialCloud,
		ctx:       ctx,
		cancel:    cancel,
		done:      make(chan struct{}),
		metrics:   metrics,
		logger:    logger,
		recovery:  recovery,
	}
	if recovery.enabled {
		f.slots = make(map[*channelSlot]struct{})
	}
	f.window.Store(&fallbackWindow{start: time.Now()})
	go f.run()
	return f
}

func (f *directPathFallback) run() {
	defer close(f.done)
	t := time.NewTimer(f.period)
	defer t.Stop()
	for {
		select {
		case <-f.ctx.Done():
			return
		case now := <-t.C:
			if f.recovery.enabled {
				// Give a freshly re-armed post-recovery window a full period before closing.
				if wait := f.current().start.Add(f.period).Sub(now); wait > 0 {
					t.Reset(min(wait, f.period))
					continue
				}
			}
			if f.closeWindow(now).switched && !f.recovery.enabled {
				return
			}
			t.Reset(f.period)
		}
	}
}

// close stops the window loop and cancels pending CloudPath dials and probes.
// It is a no-op on a nil receiver.
func (f *directPathFallback) close() {
	if f == nil {
		return
	}
	f.cancel()
	<-f.done
}

func (f *directPathFallback) current() *fallbackWindow {
	return f.window.Load()
}

func (f *directPathFallback) setProbeSession(sessionID string) {
	if f != nil && f.recovery.enabled && sessionID != "" {
		f.probeSession.Store(sessionID)
	}
}

func (f *directPathFallback) registerSlot(s *channelSlot) {
	if f == nil || !f.recovery.enabled {
		return
	}
	f.slotsMu.Lock()
	f.slots[s] = struct{}{}
	f.slotsMu.Unlock()

	if w := f.current(); w.fallback {
		s.enterFallback(w.generation)
	}
}

func (f *directPathFallback) unregisterSlot(s *channelSlot) {
	if f == nil || !f.recovery.enabled {
		return
	}
	f.slotsMu.Lock()
	delete(f.slots, s)
	f.slotsMu.Unlock()
}

// tryRecoverWindow resets the shared window out of fallback on the first slot
// recovery for fallbackGen. Returns allowed=false if a newer pool-wide fallback
// cycle already started. Callers may hold channelSlot.mu (closeMu is a leaf lock).
func (f *directPathFallback) tryRecoverWindow(fallbackGen uint64) (allowed, reset bool) {
	f.closeMu.Lock()
	defer f.closeMu.Unlock()
	w := f.window.Load()
	if w.fallback {
		if w.generation != fallbackGen {
			return false, false
		}
		f.window.Store(&fallbackWindow{
			generation: w.generation + 1,
			fallback:   false,
			start:      time.Now(),
		})
		return true, true
	}
	return true, false
}

func (f *directPathFallback) finishSlotRecovery(reset bool) {
	if reset {
		logf(f.logger, "spanner: DirectPath slot recovered; re-arming DirectPath fallback evaluation")
		f.metrics.recordFallback(f.ctx, true)
		f.runSwitchHooks()
		return
	}
	logf(f.logger, "spanner: DirectPath slot recovered")
}

func (f *directPathFallback) addSwitchHook(fn func()) {
	f.hooksMu.Lock()
	defer f.hooksMu.Unlock()
	f.onSwitch = append(f.onSwitch, fn)
}

func (f *directPathFallback) runSwitchHooks() {
	f.hooksMu.Lock()
	hooks := append([]func(){}, f.onSwitch...)
	f.hooksMu.Unlock()
	for _, fn := range hooks {
		fn()
	}
}

var errNoCloudPath = errors.New("spanner: the client's connection pool is supplied by its options and has no separate CloudPath")

// newDirectPathProbeMetadata builds static headers for recovery GetSession probes.
func newDirectPathProbeMetadata(database string, disableRouteToLeader bool) metadata.MD {
	md := metadata.MD{}
	if database != "" {
		md.Set(resourcePrefixHeader, database)
	}
	if !disableRouteToLeader {
		md.Set(routeToLeaderHeader, "true")
	}
	md.Set("x-goog-api-client", gax.XGoogHeader("gl-go", gax.GoVersion, "gccl", internal.Version))
	return md
}

// newPoolFallback returns the fallback state of a channel pool that dials
// with opts. It returns nil if fallback is off, or if opts supply the client's
// gRPC connection, which has no CloudPath to fall back to.
func newPoolFallback(enabled bool, database string, clientCfg ClientConfig, cfg transport.PoolConfig, opts []option.ClientOption, isDirect func(gtransport.ConnPool) bool) *directPathFallback {
	if !enabled || suppliesGRPCConn(opts) {
		return nil
	}
	dialCloud := newCloudPathDialer(cfg, opts, isDirect)
	period := directPathFallbackPeriod
	var recovery directPathFallbackRecoveryConfig
	if isDirectPathFallbackRecoveryEnabled() {
		period = min(directPathFallbackPeriod, time.Minute)
		recovery = defaultDirectPathRecoveryConfig
		recovery.enabled = true
		recovery.probeMD = newDirectPathProbeMetadata(database, clientCfg.DisableRouteToLeader)
	}
	return newDirectPathFallback(cfg.MeterProvider, clientCfg.Logger, dialCloud, period, recovery)
}

func suppliesGRPCConn(opts []option.ClientOption) bool {
	r, err := internaloption.NewUnsafeResolver(opts...)
	return err != nil || r.ResolvedGRPCConnIsCustom()
}

// newCloudPathDialer returns a function that dials one CloudPath connection
// with opts. It returns errNoCloudPath if isDirect matches the dialed pool,
// which happens when opts supply a fixed pool via gtransport.WithConnPool.
func newCloudPathDialer(cfg transport.PoolConfig, opts []option.ClientOption, isDirect func(gtransport.ConnPool) bool) func(context.Context) (gtransport.ConnPool, error) {
	// Clip spare capacity so concurrent dial calls cannot race on append.
	cloudOpts := slices.Clip(append(cfg.CloudPathClientOptions(opts...), option.WithGRPCConnectionPool(1)))
	dial := dialDirectPathFallbackCloudPath
	return func(ctx context.Context) (gtransport.ConnPool, error) {
		cloud, err := dial(ctx, cloudOpts...)
		if err != nil {
			return nil, err
		}
		if isDirect(cloud) {
			return nil, errNoCloudPath
		}
		return cloud, nil
	}
}

// dialWithBoundCtx dials a connection using a context derived from f.ctx (so
// credentials that retain the dial context for token refreshes stay valid until
// the connection closes), bounding only the dial itself by ctx and timeout.
func (f *directPathFallback) dialWithBoundCtx(ctx context.Context, timeout time.Duration, dial func(context.Context) (gtransport.ConnPool, error), timeoutErr error) (gtransport.ConnPool, error) {
	dialCtx, cancel := context.WithCancel(f.ctx)
	stop := context.AfterFunc(ctx, cancel)
	timer := time.AfterFunc(timeout, cancel)
	conn, err := dial(dialCtx)
	timedOut, callerDone := !timer.Stop(), !stop()
	if err == nil && !timedOut && !callerDone {
		return scopedConn{ConnPool: conn, cancel: cancel}, nil
	}
	cancel()
	if err == nil {
		conn.Close()
	}
	switch {
	case timedOut:
		return nil, timeoutErr
	case callerDone:
		return nil, ctx.Err()
	}
	return nil, err
}

// sameConnPool reports whether a and b are the same pool. Pools of a type
// that cannot be compared count as the same, so that the client never takes
// ownership of a pool it cannot tell apart from the caller's.
func sameConnPool(a, b gtransport.ConnPool) bool {
	if a == nil || b == nil {
		return false
	}
	va, vb := reflect.ValueOf(a), reflect.ValueOf(b)
	if va.Type() != vb.Type() {
		return false
	}
	return !va.Comparable() || va.Equal(vb)
}

// errStreamStoppedByReader is the cancellation cause of a stream that its
// iterator stopped. It lets fallback tell such a stream from one that the
// caller cancelled.
var errStreamStoppedByReader = errors.New("spanner: the stream's reader stopped it")

// recordAttempt records the outcome of an attempt that was sent under w with
// ctx. An attempt the caller cancelled says nothing about the path, so it is
// not counted and not reported in eef.call_status.
func (f *directPathFallback) recordAttempt(ctx context.Context, w *fallbackWindow, err error) {
	f.recordSlotAttempt(ctx, w, w.fallback, err)
}

// recordSlotAttempt records an attempt's outcome over CloudPath (usedFallback=true) or DirectPath.
// Explicit usedFallback keeps unrecovered CloudPath calls out of DirectPath error-rate accounting.
func (f *directPathFallback) recordSlotAttempt(ctx context.Context, w *fallbackWindow, usedFallback bool, err error) {
	if ctx.Value(fallbackAccountingOffKey{}) != nil {
		return
	}
	code := status.Code(err)
	if code == codes.Canceled && errors.Is(ctx.Err(), context.Canceled) {
		return
	}
	f.metrics.recordCall(ctx, usedFallback, code)
	if !usedFallback && !w.fallback {
		f.record(w.generation, isDirectPathFallbackFailure(code))
	}
}

type fallbackAccountingOffKey struct{}

// withoutFallbackAccounting marks ctx so priming RPCs on a new channel are
// excluded from fallback accounting.
func withoutFallbackAccounting(ctx context.Context) context.Context {
	return context.WithValue(ctx, fallbackAccountingOffKey{}, true)
}

// record counts one DirectPath outcome of an attempt sent under generation
// gen, in the window that is current when the attempt finishes. It drops the
// outcome if gen is no longer current.
func (f *directPathFallback) record(gen uint64, failed bool) {
	delta := fallbackWindowSuccess
	if failed {
		delta = fallbackWindowFailure
	}
	w := f.window.Load()
	for {
		if w.fallback || w.generation != gen {
			return
		}
		if w.outcomes.Add(delta)&fallbackWindowSealed == 0 {
			return
		}
		// The window was sealed first, so its decision did not see this
		// outcome. The closer publishes the next window before it releases
		// closeMu; count the outcome there.
		f.closeMu.Lock()
		w = f.window.Load()
		f.closeMu.Unlock()
	}
}

// closeWindow ends the current window and decides whether the pool falls back.
func (f *directPathFallback) closeWindow(now time.Time) fallbackWindowResult {
	f.closeMu.Lock()
	w := f.window.Load()
	if w.fallback || (f.recovery.enabled && now.Before(w.start)) {
		f.closeMu.Unlock()
		return fallbackWindowResult{}
	}
	failures, successes := unpackFallbackOutcomes(w.outcomes.Or(fallbackWindowSealed))
	next := &fallbackWindow{generation: w.generation, start: now}
	switched := shouldFallBackToCloudPath(failures, successes)
	if switched {
		next.generation++
		next.fallback = true
	}
	f.window.Store(next)
	f.closeMu.Unlock()

	if switched {
		logf(f.logger, "spanner: every DirectPath call in the last %v failed (%d failures); switching to CloudPath", now.Sub(w.start).Round(time.Second), failures)
		f.notifyFallback(next.generation)
	}
	return fallbackWindowResult{failures: failures, successes: successes, switched: switched}
}

func (f *directPathFallback) notifyFallback(gen uint64) {
	f.metrics.recordFallback(f.ctx, false)
	f.runSwitchHooks()
	if f.recovery.enabled {
		f.slotsMu.Lock()
		slots := slices.Collect(maps.Keys(f.slots))
		f.slotsMu.Unlock()
		for _, s := range slots {
			s.enterFallback(gen)
		}
	}
}

func shouldFallBackToCloudPath(failures, successes uint64) bool {
	if failures < directPathFallbackMinFailedCalls {
		return false
	}
	return float64(failures)/float64(failures+successes) >= directPathFallbackErrorRateThreshold
}

// isDirectPathFallbackFailure reports whether code counts against DirectPath.
// These are grpc-gcp-go's default codes: DEADLINE_EXCEEDED catches blackholed
// connections, and UNAUTHENTICATED catches credentials that only CloudPath
// accepts. Every other code counts as a success.
func isDirectPathFallbackFailure(code codes.Code) bool {
	switch code {
	case codes.DeadlineExceeded, codes.Unavailable, codes.Unauthenticated:
		return true
	}
	return false
}

// directPathFallbackMetrics records eef.fallback_count, eef.call_status, and
// eef.probe_result under grpc-gcp-go's meter scope, names, and attributes to
// match grpc-gcp-go's exported series.
type directPathFallbackMetrics struct {
	fallbackCount metric.Int64Counter
	callStatus    metric.Int64Counter
	probeResult   metric.Int64Counter
	// callAttrs caches the call_status attributes by [fallback][code].
	callAttrs    [2][codes.Unauthenticated + 1]metric.AddOption
	switchAttr   metric.AddOption
	recoveryAttr metric.AddOption
}

func newDirectPathFallbackMetrics(mp metric.MeterProvider, recoveryEnabled bool) (*directPathFallbackMetrics, error) {
	if mp == nil {
		return nil, nil
	}
	meter := mp.Meter(grpcGcpMetricMeterName, metric.WithInstrumentationVersion(grpcgcp.Version))
	fallbackCount, err := meter.Int64Counter(
		metricNameEEFFallbackCount,
		metric.WithDescription("Number of fallbacks occurred from one channel to another."),
		metric.WithUnit("{occurrence}"),
	)
	if err != nil {
		return nil, err
	}
	callStatus, err := meter.Int64Counter(
		metricNameEEFCallStatus,
		metric.WithDescription("Number of calls with a status and channel."),
		metric.WithUnit("{call}"),
	)
	if err != nil {
		return nil, err
	}
	m := &directPathFallbackMetrics{
		fallbackCount: fallbackCount,
		callStatus:    callStatus,
		switchAttr: metric.WithAttributeSet(attribute.NewSet(
			attribute.String(metricLabelKeyFromChannelName, directPathFallbackPrimaryChannelName),
			attribute.String(metricLabelKeyToChannelName, directPathFallbackFallbackChannelName),
		)),
	}
	if recoveryEnabled {
		probeResult, err := meter.Int64Counter(
			metricNameEEFProbeResult,
			metric.WithDescription("Results of probing functions execution."),
			metric.WithUnit("{result}"),
		)
		if err != nil {
			return nil, err
		}
		m.probeResult = probeResult
		m.recoveryAttr = metric.WithAttributeSet(attribute.NewSet(
			attribute.String(metricLabelKeyFromChannelName, directPathFallbackFallbackChannelName),
			attribute.String(metricLabelKeyToChannelName, directPathFallbackPrimaryChannelName),
		))
	}
	for i, name := range []string{directPathFallbackPrimaryChannelName, directPathFallbackFallbackChannelName} {
		for c := range m.callAttrs[i] {
			m.callAttrs[i][c] = callStatusAttr(name, codes.Code(c))
		}
	}
	return m, nil
}

func callStatusAttr(channelName string, code codes.Code) metric.AddOption {
	return metric.WithAttributeSet(attribute.NewSet(
		attribute.String(metricLabelKeyChannelName, channelName),
		attribute.String(metricLabelKeyStatusCode, code.String()),
	))
}

func (m *directPathFallbackMetrics) recordCall(ctx context.Context, fallback bool, code codes.Code) {
	if m == nil {
		return
	}
	i := 0
	name := directPathFallbackPrimaryChannelName
	if fallback {
		i = 1
		name = directPathFallbackFallbackChannelName
	}
	if code < codes.Code(len(m.callAttrs[i])) {
		m.callStatus.Add(ctx, 1, m.callAttrs[i][code])
		return
	}
	m.callStatus.Add(ctx, 1, callStatusAttr(name, code))
}

func (m *directPathFallbackMetrics) recordFallback(ctx context.Context, recovery bool) {
	if m == nil {
		return
	}
	attr := m.switchAttr
	if recovery {
		if m.recoveryAttr == nil {
			return
		}
		attr = m.recoveryAttr
	}
	m.fallbackCount.Add(ctx, 1, attr)
}

func (m *directPathFallbackMetrics) recordProbe(ctx context.Context, channelName, reason string) {
	if m == nil || m.probeResult == nil {
		return
	}
	m.probeResult.Add(ctx, 1, metric.WithAttributeSet(attribute.NewSet(
		attribute.String(metricLabelKeyChannelName, channelName),
		attribute.Bool(metricLabelKeySuccess, reason == ""),
		attribute.String(metricLabelKeyReason, reason),
	)))
}

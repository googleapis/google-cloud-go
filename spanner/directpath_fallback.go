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
	"reflect"
	"sync"
	"sync/atomic"
	"time"

	"github.com/GoogleCloudPlatform/grpc-gcp-go/grpcgcp"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/metric"
	"google.golang.org/api/option"
	"google.golang.org/api/option/internaloption"
	gtransport "google.golang.org/api/transport/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// The pool falls back when every DirectPath attempt that finished in one
// window failed, and at least one did. These are the values the client used
// with grpc-gcp-go's GCPFallback.
const (
	directPathFallbackErrorRateThreshold = 1.0
	directPathFallbackMinFailedCalls     = 1

	// Channel names of the eef.* metrics, as grpc-gcp-go reports them.
	directPathFallbackPrimaryChannelName  = "primary"
	directPathFallbackFallbackChannelName = "fallback"
)

// directPathFallbackPeriod is the length of one error-rate window. Tests
// lower it before creating clients.
var directPathFallbackPeriod = 3 * time.Minute

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

// fallbackWindow is one error-rate window. Only outcomes changes after a
// window is published.
type fallbackWindow struct {
	generation uint64 // increases with every switch between paths
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
// channelSlot of a channel pool. The switch is one way.
type directPathFallback struct {
	window  atomic.Pointer[fallbackWindow] // the only routing and counting state
	closeMu sync.Mutex                     // serializes window closes

	// dialCloud dials one connection; errNoCloudPath if the client has none.
	dialCloud func(context.Context) (gtransport.ConnPool, error)

	// ctx bounds CloudPath dials and the window loop.
	ctx    context.Context
	cancel context.CancelFunc
	done   chan struct{}

	metrics *directPathFallbackMetrics
	logger  *log.Logger

	hooksMu  sync.Mutex
	onSwitch []func()
}

// newDirectPathFallback returns the fallback state of a pool on DirectPath
// and starts closing a window every period. The caller must call close. No
// eef.* metrics are recorded if mp is nil.
func newDirectPathFallback(mp metric.MeterProvider, logger *log.Logger, dialCloud func(context.Context) (gtransport.ConnPool, error), period time.Duration) (*directPathFallback, error) {
	metrics, err := newDirectPathFallbackMetrics(mp)
	if err != nil {
		return nil, err
	}
	ctx, cancel := context.WithCancel(context.Background())
	f := &directPathFallback{
		dialCloud: dialCloud,
		ctx:       ctx,
		cancel:    cancel,
		done:      make(chan struct{}),
		metrics:   metrics,
		logger:    logger,
	}
	f.window.Store(&fallbackWindow{start: time.Now()})
	go f.run(period)
	return f, nil
}

func (f *directPathFallback) run(period time.Duration) {
	defer close(f.done)
	t := time.NewTicker(period)
	defer t.Stop()
	for {
		select {
		case <-f.ctx.Done():
			return
		case now := <-t.C:
			if f.closeWindow(now).switched {
				return
			}
		}
	}
}

// close stops the window loop and cancels pending CloudPath dials. It is a
// no-op on a nil receiver.
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

func (f *directPathFallback) addSwitchHook(fn func()) {
	f.hooksMu.Lock()
	defer f.hooksMu.Unlock()
	f.onSwitch = append(f.onSwitch, fn)
}

var errNoCloudPath = errors.New("spanner: the client's connection pool is supplied by its options and has no separate CloudPath")

// newPoolFallback returns the fallback state of a channel pool that dials
// with opts. It returns nil if fallback is off, or if opts supply the client's
// gRPC connection, which has no CloudPath to fall back to.
func newPoolFallback(enabled bool, config ClientConfig, mp metric.MeterProvider, opts []option.ClientOption, isDirect func(gtransport.ConnPool) bool) (*directPathFallback, error) {
	if !enabled || suppliesGRPCConn(opts) {
		return nil, nil
	}
	dialCloud := newCloudPathDialer(config.Compression, opts, isDirect)
	return newDirectPathFallback(mp, config.Logger, dialCloud, directPathFallbackPeriod)
}

func suppliesGRPCConn(opts []option.ClientOption) bool {
	r, err := internaloption.NewUnsafeResolver(opts...)
	return err != nil || r.ResolvedGRPCConnIsCustom()
}

// newCloudPathDialer returns a dial of one CloudPath connection with opts.
// The dial fails with errNoCloudPath if isDirect reports its result as the
// client's direct pool: the transport returns a pool supplied with
// gtransport.WithConnPool from every dial.
func newCloudPathDialer(compression string, opts []option.ClientOption, isDirect func(gtransport.ConnPool) bool) func(context.Context) (gtransport.ConnPool, error) {
	cloudOpts := append(allClientOpts(1, compression, false, opts...), option.WithGRPCConnectionPool(1))
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
	if ctx.Value(fallbackAccountingOffKey{}) != nil {
		return
	}
	code := status.Code(err)
	if code == codes.Canceled && errors.Is(ctx.Err(), context.Canceled) {
		return
	}
	f.metrics.recordCall(ctx, w.fallback, code)
	if !w.fallback {
		f.record(w.generation, isDirectPathFallbackFailure(code))
	}
}

type fallbackAccountingOffKey struct{}

// withoutFallbackAccounting is for priming a new channel. A channel that
// fails priming is discarded before any request uses it.
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
	if w.fallback {
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
		f.metrics.recordFallback(f.ctx)
		f.hooksMu.Lock()
		hooks := append([]func(){}, f.onSwitch...)
		f.hooksMu.Unlock()
		for _, fn := range hooks {
			fn()
		}
	}
	return fallbackWindowResult{failures: failures, successes: successes, switched: switched}
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

// directPathFallbackMetrics records eef.fallback_count and eef.call_status
// under grpc-gcp-go's meter scope, names and attributes, so that the exported
// series continue.
type directPathFallbackMetrics struct {
	fallbackCount metric.Int64Counter
	callStatus    metric.Int64Counter
	// callAttrs caches the call_status attributes by [fallback][code].
	callAttrs  [2][codes.Unauthenticated + 1]metric.AddOption
	switchAttr metric.AddOption
}

func newDirectPathFallbackMetrics(mp metric.MeterProvider) (*directPathFallbackMetrics, error) {
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
	if int(code) < len(m.callAttrs[i]) {
		m.callStatus.Add(ctx, 1, m.callAttrs[i][code])
		return
	}
	m.callStatus.Add(ctx, 1, callStatusAttr(name, code))
}

func (m *directPathFallbackMetrics) recordFallback(ctx context.Context) {
	if m == nil {
		return
	}
	m.fallbackCount.Add(ctx, 1, m.switchAttr)
}

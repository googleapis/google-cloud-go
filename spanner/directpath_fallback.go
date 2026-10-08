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
	"sync"
	"sync/atomic"
	"time"

	"cloud.google.com/go/spanner/internal"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/metric"
	gtransport "google.golang.org/api/transport/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// DirectPath to CloudPath fallback policy. The client used the same values
// with grpc-gcp-go's GCPFallback before fallback moved into this package: the
// pool falls back when every DirectPath attempt that finished in one window
// failed, and at least one did.
const (
	directPathFallbackErrorRateThreshold = 1.0
	directPathFallbackMinFailedCalls     = 1

	// Channel names of the eef.* metrics, as grpc-gcp-go reported them.
	directPathFallbackPrimaryChannelName  = "primary"
	directPathFallbackFallbackChannelName = "fallback"
)

// directPathFallbackPeriod is the length of one error-rate window. Tests lower
// it before creating clients.
var directPathFallbackPeriod = 3 * time.Minute

// dialDirectPathFallbackCloudPath dials the CloudPath side of a channel slot.
// A client uses the value it had when the client was created. Tests replace
// it to serve CloudPath from a different server than DirectPath.
var dialDirectPathFallbackCloudPath = gtransport.DialPool

// Layout of fallbackWindow.outcomes. Bit 63 marks a sealed window, bits 32-62
// count failures and bits 0-31 count successes. A window would need more than
// 2^31 failures or 2^32 successes within one period to overflow.
const (
	fallbackWindowSealed  uint64 = 1 << 63
	fallbackWindowFailure uint64 = 1 << 32
	fallbackWindowSuccess uint64 = 1
)

// fallbackWindow is one error-rate window of the pool-wide DirectPath fallback
// state. Only outcomes changes after the window is published; every other
// change publishes a new window.
type fallbackWindow struct {
	// generation increases with every switch between DirectPath and CloudPath.
	// An attempt keeps the window it was dispatched under, and its outcome only
	// counts while that generation is current.
	generation uint64
	// fallback routes new attempts to CloudPath.
	fallback bool
	start    time.Time
	// outcomes packs the DirectPath outcomes of the window so that one atomic
	// operation reads or seals a consistent pair of counts.
	outcomes atomic.Uint64
}

func unpackFallbackOutcomes(v uint64) (failures, successes uint64) {
	return (v &^ fallbackWindowSealed) >> 32, v & (fallbackWindowFailure - 1)
}

// fallbackWindowResult describes one closed window.
type fallbackWindowResult struct {
	failures, successes uint64
	// switched reports whether closing the window moved the pool to CloudPath.
	switched bool
}

// directPathFallback is the pool-wide DirectPath to CloudPath switch shared by
// every channelSlot of a channel pool. The switch is one way: once the pool
// falls back it stays on CloudPath.
//
// The current window is the only routing and counting state. Dispatching an
// attempt loads it once; counting an outcome adds to its packed counter; a
// periodic close seals the counter, decides, and publishes the next window.
type directPathFallback struct {
	window atomic.Pointer[fallbackWindow]
	// closeMu serializes window closes. Recorders that find a sealed window wait
	// on it until the next window is published.
	closeMu sync.Mutex

	// ctx bounds CloudPath dials and the window loop; Close cancels it.
	ctx       context.Context
	cancel    context.CancelFunc
	startOnce sync.Once
	started   atomic.Bool
	closeOnce sync.Once
	done      chan struct{}

	metrics *directPathFallbackMetrics
	logger  *log.Logger

	hooksMu  sync.Mutex
	onSwitch []func()
}

// newDirectPathFallback creates the fallback state on DirectPath. mp may be
// nil, in which case no eef.* metrics are recorded.
func newDirectPathFallback(mp metric.MeterProvider, logger *log.Logger) (*directPathFallback, error) {
	metrics, err := newDirectPathFallbackMetrics(mp)
	if err != nil {
		return nil, err
	}
	ctx, cancel := context.WithCancel(context.Background())
	f := &directPathFallback{
		ctx:     ctx,
		cancel:  cancel,
		done:    make(chan struct{}),
		metrics: metrics,
		logger:  logger,
	}
	f.window.Store(&fallbackWindow{start: time.Now()})
	return f, nil
}

// start closes a window every period until the pool falls back or the state is
// closed.
func (f *directPathFallback) start(period time.Duration) {
	f.startOnce.Do(func() {
		f.started.Store(true)
		go f.run(period)
	})
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
				// One way: there is nothing left to evaluate.
				return
			}
		}
	}
}

// close stops the window loop and cancels pending CloudPath dials.
func (f *directPathFallback) close() {
	f.closeOnce.Do(func() {
		f.cancel()
		if f.started.Load() {
			<-f.done
		}
	})
}

// current returns the window that new attempts are dispatched under.
func (f *directPathFallback) current() *fallbackWindow {
	return f.window.Load()
}

// addSwitchHook registers fn to run after the pool moves to CloudPath.
func (f *directPathFallback) addSwitchHook(fn func()) {
	f.hooksMu.Lock()
	defer f.hooksMu.Unlock()
	f.onSwitch = append(f.onSwitch, fn)
}

// recordAttempt records the outcome of an attempt that was dispatched under w
// with context ctx. An attempt the caller cancelled, for example a query
// stream that RowIterator.Stop ended before its last message, says nothing
// about the path: it is neither a success nor a failure, and it is not
// reported in eef.call_status. Outcomes count in the window that is current
// when they finish, so a hung attempt that times out after a window boundary
// still counts against DirectPath.
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

// withoutFallbackAccounting returns a context whose attempts neither feed the
// fallback decision nor eef.call_status. The dynamic pool primes a scaled-up
// channel with it: a channel that fails priming is discarded before any
// request uses it, which says nothing about the pool's other channels.
func withoutFallbackAccounting(ctx context.Context) context.Context {
	return context.WithValue(ctx, fallbackAccountingOffKey{}, true)
}

// record counts one DirectPath outcome of an attempt dispatched under
// generation gen. It drops the outcome if the generation is no longer current
// or the pool has fallen back.
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
		// The window was sealed before this add, so the add is not part of the
		// snapshot that decided it. The closer publishes the next window before
		// it releases closeMu; count the outcome there.
		f.closeMu.Lock()
		w = f.window.Load()
		f.closeMu.Unlock()
	}
}

// closeWindow seals the current window, decides whether the pool falls back,
// and publishes the next window.
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

// isDirectPathFallbackFailure reports whether a DirectPath attempt that ended
// with code counts against DirectPath. These are grpc-gcp-go's default
// erroneous codes: DEADLINE_EXCEEDED catches blackholed connections that never
// answer, and UNAUTHENTICATED catches credentials that only CloudPath accepts.
// Every other code, including OK, counts as a success, except a cancellation
// by the caller, which recordAttempt does not count at all.
func isDirectPathFallbackFailure(code codes.Code) bool {
	switch code {
	case codes.DeadlineExceeded, codes.Unavailable, codes.Unauthenticated:
		return true
	}
	return false
}

// directPathFallbackMetrics records eef.fallback_count and eef.call_status with
// the names, attributes, and meter scope grpc-gcp-go used, so that the
// exported spanner.googleapis.com/internal/client/eef/* series continue.
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
	meter := mp.Meter(grpcGcpMetricMeterName, metric.WithInstrumentationVersion(internal.Version))
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

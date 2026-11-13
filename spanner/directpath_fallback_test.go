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
	"io"
	"log"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	adminpb "cloud.google.com/go/spanner/admin/database/apiv1/databasepb"
	"go.opentelemetry.io/otel/attribute"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/metric/metricdata"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// sawGoogleSQLPass records that TestMain's GoogleSQL pass ran in this process.
var sawGoogleSQLPass atomic.Bool

// skipDialectRerun skips a dialect-independent test in TestMain's PostgreSQL
// pass when the GoogleSQL pass already ran it. TestMain runs the whole suite
// once per dialect, and the channel pool and DirectPath fallback tests do not
// depend on the dialect. A run with only the PostgreSQL pass still runs them.
func skipDialectRerun(t *testing.T) {
	t.Helper()
	if testDialect != adminpb.DatabaseDialect_POSTGRESQL {
		sawGoogleSQLPass.Store(true)
		return
	}
	if sawGoogleSQLPass.Load() {
		t.Skip("dialect independent; already ran in the GoogleSQL pass")
	}
}

func newTestDirectPathFallback(t *testing.T) *directPathFallback {
	t.Helper()
	f, err := newDirectPathFallback(nil, log.New(io.Discard, "", 0))
	if err != nil {
		t.Fatalf("newDirectPathFallback() failed: %v", err)
	}
	t.Cleanup(f.close)
	return f
}

// recordDirectPathOutcomes records DirectPath outcomes dispatched under the
// current window.
func recordDirectPathOutcomes(f *directPathFallback, outcomes ...codes.Code) {
	for _, c := range outcomes {
		var err error
		if c != 0 {
			err = status.Error(c, c.String())
		}
		f.recordAttempt(context.Background(), f.current(), err)
	}
}

func repeatCode(c codes.Code, n int) []codes.Code {
	out := make([]codes.Code, n)
	for i := range out {
		out[i] = c
	}
	return out
}

func TestDirectPathFallbackSwitchRule(t *testing.T) {
	skipDialectRerun(t)
	tests := []struct {
		name     string
		outcomes []codes.Code
		want     bool
	}{
		{name: "empty window", want: false},
		{name: "single UNAVAILABLE", outcomes: []codes.Code{codes.Unavailable}, want: true},
		{name: "single DEADLINE_EXCEEDED", outcomes: []codes.Code{codes.DeadlineExceeded}, want: true},
		{name: "single UNAUTHENTICATED", outcomes: []codes.Code{codes.Unauthenticated}, want: true},
		{name: "every call failed", outcomes: repeatCode(codes.Unavailable, 50), want: true},
		{name: "one success among failures", outcomes: append(repeatCode(codes.Unavailable, 50), codes.OK), want: false},
		// The first outcome of the window decides nothing on its own; the PR
		// 14612 slot tripped here.
		{name: "failure first then successes", outcomes: append([]codes.Code{codes.Unavailable}, repeatCode(codes.OK, 1000)...), want: false},
		{name: "successes then a failure", outcomes: append(repeatCode(codes.OK, 1000), codes.Unavailable), want: false},
		{name: "one channel of four failing", outcomes: []codes.Code{codes.OK, codes.OK, codes.OK, codes.Unavailable}, want: false},
		{name: "non-fallback errors count as successes", outcomes: []codes.Code{codes.Internal, codes.ResourceExhausted, codes.Canceled, codes.Aborted}, want: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := newTestDirectPathFallback(t)
			recordDirectPathOutcomes(f, tt.outcomes...)
			if f.current().fallback {
				t.Fatal("pool fell back before the window closed")
			}
			res := f.closeWindow(time.Now())
			if res.switched != tt.want {
				t.Fatalf("switched mismatch for %d failures and %d successes\n Got: %v\nWant: %v", res.failures, res.successes, res.switched, tt.want)
			}
			if got := f.current().fallback; got != tt.want {
				t.Fatalf("current window fallback mismatch\n Got: %v\nWant: %v", got, tt.want)
			}
		})
	}
}

func TestDirectPathFallbackIsOneWay(t *testing.T) {
	skipDialectRerun(t)
	f := newTestDirectPathFallback(t)
	before := f.current()
	recordDirectPathOutcomes(f, codes.Unavailable)
	if res := f.closeWindow(time.Now()); !res.switched {
		t.Fatal("pool did not fall back after a window of failures")
	}
	after := f.current()
	if !after.fallback || after.generation != before.generation+1 {
		t.Fatalf("window after switch = {generation: %d, fallback: %v}, want {generation: %d, fallback: true}", after.generation, after.fallback, before.generation+1)
	}

	// Neither DirectPath successes from attempts sent before the switch nor
	// later windows bring the pool back.
	f.recordAttempt(context.Background(), before, nil)
	for i := 0; i < 3; i++ {
		f.recordAttempt(context.Background(), f.current(), nil)
		if res := f.closeWindow(time.Now()); res.switched {
			t.Fatal("closing a fallback window switched again")
		}
	}
	if got := f.current(); got != after {
		t.Fatalf("fallback window replaced: got generation %d, want the switch window", got.generation)
	}
}

func TestDirectPathFallbackIgnoresOutcomesOfOtherGenerations(t *testing.T) {
	skipDialectRerun(t)
	f := newTestDirectPathFallback(t)
	// Simulate a pool that already moved past generation 0, for example after a
	// future recovery. Outcomes of attempts dispatched under generation 0 must
	// not count toward the current generation's window.
	f.window.Store(&fallbackWindow{generation: 2, start: time.Now()})
	f.record(0, true)
	f.record(1, true)
	if res := f.closeWindow(time.Now()); res.failures != 0 || res.switched {
		t.Fatalf("stale outcomes counted: %+v", res)
	}
	f.record(2, true)
	if res := f.closeWindow(time.Now()); res.failures != 1 || !res.switched {
		t.Fatalf("current-generation outcome not counted: %+v", res)
	}
}

func TestDirectPathFallbackInFlightAttemptFinishingAfterSwitch(t *testing.T) {
	skipDialectRerun(t)
	f := newTestDirectPathFallback(t)
	inFlight := f.current() // dispatched on DirectPath before the switch
	recordDirectPathOutcomes(f, codes.Unavailable)
	f.closeWindow(time.Now())
	// The in-flight attempt fails after the switch: it is dropped, and it does
	// not touch the fallback window's counter.
	f.recordAttempt(context.Background(), inFlight, status.Error(codes.Unavailable, "late"))
	if got := f.current().outcomes.Load(); got != 0 {
		t.Fatalf("fallback window outcomes = %#x, want 0", got)
	}
}

// TestDirectPathFallbackWindowCloseLosesNoOutcome records outcomes from many
// goroutines while windows close concurrently. Every outcome must be counted
// in exactly one window, and each decision must use a consistent snapshot.
// This is the race the PR 14612 review found in resetting two counters.
func TestDirectPathFallbackWindowCloseLosesNoOutcome(t *testing.T) {
	skipDialectRerun(t)
	f := newTestDirectPathFallback(t)
	const (
		writers   = 8
		perWriter = 20000
	)
	stop := make(chan struct{})
	var closed []fallbackWindowResult
	closerDone := make(chan struct{})
	go func() {
		defer close(closerDone)
		for {
			select {
			case <-stop:
				return
			default:
			}
			// Keep the pool on DirectPath: only count, never decide to switch.
			// A real switch would end counting, which is tested elsewhere.
			res := f.closeWindowForTest(time.Now())
			closed = append(closed, res)
		}
	}()
	var wg sync.WaitGroup
	for w := 0; w < writers; w++ {
		wg.Add(1)
		go func(w int) {
			defer wg.Done()
			for i := 0; i < perWriter; i++ {
				f.record(0, (i+w)%10 == 0)
			}
		}(w)
	}
	wg.Wait()
	close(stop)
	<-closerDone
	closed = append(closed, f.closeWindowForTest(time.Now()))

	var failures, successes uint64
	for _, res := range closed {
		failures += res.failures
		successes += res.successes
	}
	const total = writers * perWriter
	if got := failures + successes; got != total {
		t.Fatalf("outcomes counted across %d windows = %d, want %d", len(closed), got, total)
	}
	if want := uint64(total / 10); failures != want {
		t.Fatalf("failures counted = %d, want %d", failures, want)
	}
}

// closeWindowForTest closes the current window like closeWindow but never
// switches, so counting can be observed over many windows.
func (f *directPathFallback) closeWindowForTest(now time.Time) fallbackWindowResult {
	f.closeMu.Lock()
	defer f.closeMu.Unlock()
	w := f.window.Load()
	failures, successes := unpackFallbackOutcomes(w.outcomes.Or(fallbackWindowSealed))
	f.window.Store(&fallbackWindow{generation: w.generation, start: now})
	return fallbackWindowResult{failures: failures, successes: successes}
}

func TestDirectPathFallbackLoopSwitchesAndStops(t *testing.T) {
	skipDialectRerun(t)
	f := newTestDirectPathFallback(t)
	var hookCalls int
	var mu sync.Mutex
	f.addSwitchHook(func() {
		mu.Lock()
		hookCalls++
		mu.Unlock()
	})
	recordDirectPathOutcomes(f, codes.Unavailable)
	f.start(5 * time.Millisecond)
	select {
	case <-f.done:
	case <-time.After(10 * time.Second):
		t.Fatal("window loop did not stop after switching")
	}
	if !f.current().fallback {
		t.Fatal("pool did not fall back")
	}
	mu.Lock()
	defer mu.Unlock()
	if hookCalls != 1 {
		t.Fatalf("switch hook calls = %d, want 1", hookCalls)
	}
}

func TestDirectPathFallbackCloseWithoutStart(t *testing.T) {
	skipDialectRerun(t)
	f, err := newDirectPathFallback(nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	f.close()
	f.close()
	if f.ctx.Err() == nil {
		t.Fatal("close did not cancel the fallback context")
	}
}

func TestDirectPathFallbackMetrics(t *testing.T) {
	skipDialectRerun(t)
	reader := sdkmetric.NewManualReader()
	provider := sdkmetric.NewMeterProvider(sdkmetric.WithReader(reader))
	t.Cleanup(func() { provider.Shutdown(context.Background()) })
	f, err := newDirectPathFallback(provider, log.New(io.Discard, "", 0))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(f.close)

	recordDirectPathOutcomes(f, codes.Unavailable, codes.Unavailable)
	f.closeWindow(time.Now())
	recordDirectPathOutcomes(f, codes.OK)

	var rm metricdata.ResourceMetrics
	if err := reader.Collect(context.Background(), &rm); err != nil {
		t.Fatal(err)
	}
	var scope string
	got := map[string]int64{}
	for _, sm := range rm.ScopeMetrics {
		for _, m := range sm.Metrics {
			sum, ok := m.Data.(metricdata.Sum[int64])
			if !ok {
				t.Fatalf("metric %q data type = %T, want metricdata.Sum[int64]", m.Name, m.Data)
			}
			scope = sm.Scope.Name
			for _, p := range sum.DataPoints {
				got[m.Name+"|"+attrString(p.Attributes)] += p.Value
			}
		}
	}
	if scope != grpcGcpMetricMeterName {
		t.Fatalf("meter scope = %q, want %q", scope, grpcGcpMetricMeterName)
	}
	want := map[string]int64{
		metricNameEEFFallbackCount + "|from_channel_name=primary,to_channel_name=fallback": 1,
		metricNameEEFCallStatus + "|channel_name=primary,status_code=Unavailable":          2,
		metricNameEEFCallStatus + "|channel_name=fallback,status_code=OK":                  1,
	}
	if len(got) != len(want) {
		t.Fatalf("metric points mismatch\n Got: %v\nWant: %v", got, want)
	}
	for k, v := range want {
		if got[k] != v {
			t.Fatalf("metric points mismatch\n Got: %v\nWant: %v", got, want)
		}
	}
}

func attrString(set attribute.Set) string {
	var s string
	iter := set.Iter()
	for iter.Next() {
		kv := iter.Attribute()
		if s != "" {
			s += ","
		}
		s += string(kv.Key) + "=" + kv.Value.AsString()
	}
	return s
}

// TestDirectPathFallbackProductionCloseLosesNoOutcome repeats the conservation
// check through closeWindow itself, with outcomes that never switch the pool.
func TestDirectPathFallbackProductionCloseLosesNoOutcome(t *testing.T) {
	skipDialectRerun(t)
	f := newTestDirectPathFallback(t)
	const writers, perWriter = 8, 20000
	stop := make(chan struct{})
	closerDone := make(chan uint64)
	go func() {
		var counted uint64
		for {
			select {
			case <-stop:
				closerDone <- counted
				return
			default:
			}
			counted += f.closeWindow(time.Now()).successes
		}
	}()
	var wg sync.WaitGroup
	for w := 0; w < writers; w++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := 0; i < perWriter; i++ {
				f.record(0, false)
			}
		}()
	}
	wg.Wait()
	close(stop)
	counted := <-closerDone + f.closeWindow(time.Now()).successes
	if counted != writers*perWriter {
		t.Fatalf("outcomes counted through closeWindow = %d, want %d", counted, writers*perWriter)
	}
}

func TestDirectPathFallbackIgnoresCallerCancellation(t *testing.T) {
	skipDialectRerun(t)
	reader := sdkmetric.NewManualReader()
	provider := sdkmetric.NewMeterProvider(sdkmetric.WithReader(reader))
	t.Cleanup(func() { provider.Shutdown(context.Background()) })
	f, err := newDirectPathFallback(provider, log.New(io.Discard, "", 0))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(f.close)

	cancelled, cancel := context.WithCancel(context.Background())
	cancel()
	// The caller stopped the call: no outcome and no eef.call_status point.
	f.recordAttempt(cancelled, f.current(), status.Error(codes.Canceled, "context canceled"))
	if res := f.closeWindow(time.Now()); res.failures != 0 || res.successes != 0 {
		t.Fatalf("caller cancellation counted: %+v", res)
	}
	var rm metricdata.ResourceMetrics
	if err := reader.Collect(context.Background(), &rm); err != nil {
		t.Fatal(err)
	}
	if _, ok := findTestMetric(rm, metricNameEEFCallStatus); ok {
		t.Fatal("caller cancellation reported in eef.call_status")
	}

	// A CANCELLED status the caller did not cause still counts as a success,
	// as it did with grpc-gcp-go.
	f.recordAttempt(context.Background(), f.current(), status.Error(codes.Canceled, "cancelled by the server"))
	if res := f.closeWindow(time.Now()); res.successes != 1 {
		t.Fatalf("server cancellation not counted as a success: %+v", res)
	}

	// Callers that give up on a blackholed DirectPath do not hide it.
	f.recordAttempt(cancelled, f.current(), status.Error(codes.Canceled, "context canceled"))
	f.recordAttempt(context.Background(), f.current(), status.Error(codes.DeadlineExceeded, "deadline exceeded"))
	if res := f.closeWindow(time.Now()); !res.switched {
		t.Fatalf("pool did not fall back: %+v", res)
	}
}

// TestDirectPathFallbackCountsHungAttemptInTheWindowItEnds covers an attempt
// dispatched in one window that hangs and times out in the next: it counts in
// the window where it ends. Counting only attempts that end in the window they
// were dispatched in would drop exactly the hung calls of a blackhole.
func TestDirectPathFallbackCountsHungAttemptInTheWindowItEnds(t *testing.T) {
	skipDialectRerun(t)
	f := newTestDirectPathFallback(t)
	dispatched := f.current()
	if res := f.closeWindow(time.Now()); res.switched {
		t.Fatal("empty window switched")
	}
	f.recordAttempt(context.Background(), dispatched, status.Error(codes.DeadlineExceeded, "deadline exceeded"))
	if res := f.closeWindow(time.Now()); !res.switched {
		t.Fatalf("hung attempt did not count in the window it ended in: %+v", res)
	}
}

func TestNativeMeterProviderWithoutFactory(t *testing.T) {
	skipDialectRerun(t)
	var factory *builtinMetricsTracerFactory
	if mp := factory.nativeMeterProvider(); mp != nil {
		t.Fatalf("nativeMeterProvider() of a nil factory = %v, want nil", mp)
	}
}

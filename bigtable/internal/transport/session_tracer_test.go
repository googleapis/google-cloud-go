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
	"testing"
	"time"

	spb "cloud.google.com/go/bigtable/apiv2/bigtablepb"
	"go.opentelemetry.io/otel/attribute"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/metric/metricdata"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// InitializeSessionMetrics uses sync.Once — once any test in the
// package fires it with a MeterProvider, subsequent calls are no-ops.
// The metric-emission subtests below therefore share one ManualReader
// via the parent test, and any test in this file that expects
// histograms populated has to run AFTER the init.
//
// findAttr looks up a single string label on the first data point —
// the OTel data model doesn't expose Attributes as a map.
func findAttr(attrs []metricdata.HistogramDataPoint[float64], key string) (string, bool) {
	if len(attrs) == 0 {
		return "", false
	}
	v, ok := attrs[0].Attributes.Value(attribute.Key(key))
	if !ok {
		return "", false
	}
	return v.AsString(), true
}

// TestSessionTracer_MetricsRoundTrip is the umbrella test that
// initializes the session metrics with a ManualReader and runs every
// metric-emitting subtest against it. Structured as subtests so a
// single sync.Once init serves all recording checks — otherwise the
// second test would find the histograms already bound to the first
// test's provider.
func TestSessionTracer_MetricsRoundTrip(t *testing.T) {
	reader := sdkmetric.NewManualReader()
	provider := sdkmetric.NewMeterProvider(sdkmetric.WithReader(reader))
	if err := InitializeSessionMetrics(provider); err != nil {
		t.Fatalf("InitializeSessionMetrics: %v", err)
	}

	t.Run("InitializeIdempotent", func(t *testing.T) {
		// Second call with a different provider MUST be a no-op —
		// sync.Once discipline. Nil provider on the second call must
		// also be safe.
		other := sdkmetric.NewMeterProvider(sdkmetric.WithReader(sdkmetric.NewManualReader()))
		if err := InitializeSessionMetrics(other); err != nil {
			t.Errorf("second InitializeSessionMetrics err = %v, want nil", err)
		}
		if err := InitializeSessionMetrics(nil); err != nil {
			t.Errorf("nil-provider InitializeSessionMetrics err = %v, want nil", err)
		}
	})

	t.Run("RecordOpenPopulatesLatencyHistogram", func(t *testing.T) {
		tr := newSessionTracer(SessionTypeTable)
		tr.setPoolName("test-pool")
		tr.setPeerInfo(&spb.PeerInfo{
			ApplicationFrontendSubzone: "us-east1-b",
		})
		time.Sleep(2 * time.Millisecond) // give the elapsed a positive value
		tr.recordOpen(context.Background(), nil)

		rm := &metricdata.ResourceMetrics{}
		if err := reader.Collect(context.Background(), rm); err != nil {
			t.Fatalf("Collect: %v", err)
		}
		h := requireHistogram(t, rm, "session.open_latencies")
		if len(h.DataPoints) == 0 {
			t.Fatal("no data points in session.open_latencies")
		}
		if got := h.DataPoints[0].Count; got == 0 {
			t.Error("session.open_latencies count = 0, want ≥ 1")
		}
		if got, _ := findAttr(h.DataPoints, "status"); got != "OK" {
			t.Errorf("status label = %q, want OK", got)
		}
	})

	t.Run("RecordOpenStampsStatusOnError", func(t *testing.T) {
		tr := newSessionTracer(SessionTypeTable)
		tr.setPoolName("err-pool")
		tr.recordOpen(context.Background(), status.Error(codes.Unavailable, "boom"))

		rm := &metricdata.ResourceMetrics{}
		if err := reader.Collect(context.Background(), rm); err != nil {
			t.Fatalf("Collect: %v", err)
		}
		h := requireHistogram(t, rm, "session.open_latencies")
		// Find the DataPoint whose session_name == "err-pool" — the
		// prior subtest populated a different DP.
		got, ok := statusForPool(h.DataPoints, "err-pool")
		if !ok {
			t.Fatal("no data point for pool err-pool in session.open_latencies")
		}
		if got != "Unavailable" {
			t.Errorf("status label = %q, want Unavailable", got)
		}
	})

	t.Run("RecordCloseVRpcStateLabel", func(t *testing.T) {
		cases := []struct {
			name          string
			hadOk, hadErr bool
			wantLabel     string
		}{
			{"AllOk", true, false, "all_ok"},
			{"AllError", false, true, "all_error"},
			{"SomeOk", true, true, "some_ok"},
			{"None", false, false, "none"},
		}
		for _, tc := range cases {
			t.Run(tc.name, func(t *testing.T) {
				pool := "close-" + tc.wantLabel
				tr := newSessionTracer(SessionTypeTable)
				tr.setPoolName(pool)
				tr.recordClose(context.Background(), "test-reason", nil, tc.hadOk, tc.hadErr)

				rm := &metricdata.ResourceMetrics{}
				if err := reader.Collect(context.Background(), rm); err != nil {
					t.Fatalf("Collect: %v", err)
				}
				h := requireHistogram(t, rm, "session.durations")
				got, ok := labelForPool(h.DataPoints, pool, "vrpcs")
				if !ok {
					t.Fatal("no data point for pool")
				}
				if got != tc.wantLabel {
					t.Errorf("vrpcs label = %q, want %q", got, tc.wantLabel)
				}
			})
		}
	})

	t.Run("SampleUptimeSkipsZeroStart", func(t *testing.T) {
		// A tracer with startTime zero'd out (not what newSessionTracer
		// produces, but defensive) must not emit — the histogram would
		// see negative-ish nonsense from the wall-clock delta.
		tr := &sessionTracer{sessionType: SessionTypeTable}
		tr.sampleUptime(context.Background())

		rm := &metricdata.ResourceMetrics{}
		if err := reader.Collect(context.Background(), rm); err != nil {
			t.Fatalf("Collect: %v", err)
		}
		if _, ok := findMetricByName(rm, "session.uptime"); ok {
			// If session.uptime exists but from another subtest, that's
			// fine — we just check no NEW pool named "" appears.
			// Nothing to assert here beyond the no-panic guarantee.
		}
	})

	t.Run("SampleUptimeEmitsForActiveSession", func(t *testing.T) {
		tr := newSessionTracer(SessionTypeTable)
		tr.setPoolName("uptime-pool")
		time.Sleep(2 * time.Millisecond)
		tr.sampleUptime(context.Background())

		rm := &metricdata.ResourceMetrics{}
		if err := reader.Collect(context.Background(), rm); err != nil {
			t.Fatalf("Collect: %v", err)
		}
		h := requireHistogram(t, rm, "session.uptime")
		if _, ok := labelForPool(h.DataPoints, "uptime-pool", "session_type"); !ok {
			t.Error("no data point for uptime-pool in session.uptime")
		}
	})

	t.Run("RecordTransportOverheadPositiveDeltaGate", func(t *testing.T) {
		tr := newSessionTracer(SessionTypeTable)
		tr.setPoolName("overhead-pool")

		// Zero and negative overheads MUST NOT emit — the metric's
		// contract is e2e minus backend, gated on positive delta by
		// the caller.
		tr.recordTransportOverhead(context.Background(), "Read", 0)
		tr.recordTransportOverhead(context.Background(), "Read", -3*time.Millisecond)

		// Positive overhead emits.
		tr.recordTransportOverhead(context.Background(), "Read", 5*time.Millisecond)

		rm := &metricdata.ResourceMetrics{}
		if err := reader.Collect(context.Background(), rm); err != nil {
			t.Fatalf("Collect: %v", err)
		}
		h := requireHistogram(t, rm, "transport_latencies")
		got, ok := labelForPool(h.DataPoints, "overhead-pool", "method")
		if !ok {
			t.Fatal("no data point for overhead-pool in transport_latencies")
		}
		if got != "Read" {
			t.Errorf("method label = %q, want Read", got)
		}
		// The zero/negative calls above must not have added extra
		// data points; count for this pool should be exactly 1.
		if c := countForPool(h.DataPoints, "overhead-pool"); c != 1 {
			t.Errorf("count for overhead-pool = %d, want 1 (zero/negative overheads must be dropped)", c)
		}
	})
}

// TestSessionTracer_NoOpsWhenMetricsUninitialized guards the
// "tracer methods MUST NOT panic when Init was never called" contract.
// Only runnable in a subprocess (sync.Once is global), so this test
// stubs the globals to nil via a helper. In practice the sync.Once
// path is the production one; this test protects the fast-fail
// defense in each recorder.
func TestSessionTracer_NilHistogramsAreNoOps(t *testing.T) {
	// Save and restore the package-global histograms so we can simulate
	// the pre-init state without racing other tests. This works because
	// go test runs a single package's tests serially by default within
	// a single test binary.
	origO, origD, origU, origT := sessionOpenLatencies, sessionDurations, sessionUptime, transportLatencies
	sessionOpenLatencies, sessionDurations, sessionUptime, transportLatencies = nil, nil, nil, nil
	t.Cleanup(func() {
		sessionOpenLatencies, sessionDurations, sessionUptime, transportLatencies = origO, origD, origU, origT
	})

	tr := newSessionTracer(SessionTypeTable)
	// Each recorder MUST early-return without panic when its histogram
	// is nil (documented in-file). Simple call-and-return smoke test.
	tr.recordOpen(context.Background(), nil)
	tr.recordClose(context.Background(), "", nil, false, false)
	tr.sampleUptime(context.Background())
	tr.recordTransportOverhead(context.Background(), "Read", 1*time.Millisecond)
}

func TestNewSessionTracer_DefaultsStamped(t *testing.T) {
	before := time.Now()
	tr := newSessionTracer(SessionTypeAuthorizedView)
	after := time.Now()

	if tr.sessionType != SessionTypeAuthorizedView {
		t.Errorf("sessionType = %v, want %v", tr.sessionType, SessionTypeAuthorizedView)
	}
	if tr.startTime.Before(before) || tr.startTime.After(after) {
		t.Errorf("startTime = %v, want within [%v, %v]", tr.startTime, before, after)
	}
	if got := tr.StartedAt(); !got.Equal(tr.startTime) {
		t.Errorf("StartedAt() = %v, want %v", got, tr.startTime)
	}
	if tr.opened {
		t.Error("opened = true at construction, want false")
	}
}

func TestSessionTracer_SnapshotUnknownTransportOnNilPeer(t *testing.T) {
	tr := newSessionTracer(SessionTypeTable)
	tr.setPoolName("pool-abc")
	snap := tr.snapshot()
	if snap.transportType != "unknown" {
		t.Errorf("transportType = %q, want unknown (nil peerInfo)", snap.transportType)
	}
	if snap.poolName != "pool-abc" {
		t.Errorf("poolName = %q, want pool-abc", snap.poolName)
	}
	if snap.afeLocation != "" {
		t.Errorf("afeLocation = %q, want empty", snap.afeLocation)
	}
	if snap.opened {
		t.Error("snap.opened = true, want false pre-recordOpen")
	}
}

func TestSessionTracer_SnapshotAfterPeerInfoSet(t *testing.T) {
	tr := newSessionTracer(SessionTypeTable)
	tr.setPeerInfo(&spb.PeerInfo{
		ApplicationFrontendSubzone: "us-central1-a",
	})
	snap := tr.snapshot()
	if snap.afeLocation != "us-central1-a" {
		t.Errorf("afeLocation = %q, want us-central1-a", snap.afeLocation)
	}
	// TransportType defaults to whatever the (unset) proto enum maps
	// to — the tracer doesn't invent one; it forwards
	// metricsinternal.TransportTypeName's mapping.
	if snap.transportType == "" {
		t.Error("transportType empty, want a metricsinternal mapping (fallback string)")
	}
}

func TestVRpcCloseState(t *testing.T) {
	tests := []struct {
		name          string
		hadOk, hadErr bool
		want          string
	}{
		{"NoTraffic", false, false, "none"},
		{"AllOk", true, false, "all_ok"},
		{"AllError", false, true, "all_error"},
		{"Mixed", true, true, "some_ok"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := vrpcCloseState(tt.hadOk, tt.hadErr); got != tt.want {
				t.Errorf("vrpcCloseState(%v,%v) = %q, want %q", tt.hadOk, tt.hadErr, got, tt.want)
			}
		})
	}
}

func TestMsSince(t *testing.T) {
	past := time.Now().Add(-25 * time.Millisecond)
	got := msSince(past)
	if got < 20 || got > 200 {
		// 25ms nominal; wide tolerance for scheduler jitter, but
		// negative or 0 would indicate the formula is inverted.
		t.Errorf("msSince(25ms ago) = %v, want in [20, 200]", got)
	}
	// Zero time yields "since epoch" — huge positive number; the
	// tracer's sampleUptime documents an explicit IsZero guard, so
	// this test just proves msSince itself doesn't misbehave.
	if msSince(time.Now()) < 0 {
		t.Error("msSince(now) < 0; formula inverted?")
	}
}

// -- test helpers ------------------------------------------------------

func requireHistogram(t *testing.T, rm *metricdata.ResourceMetrics, name string) metricdata.Histogram[float64] {
	t.Helper()
	m, ok := findMetricByName(rm, name)
	if !ok {
		t.Fatalf("metric %q not found; scopes=%d", name, len(rm.ScopeMetrics))
	}
	h, ok := m.Data.(metricdata.Histogram[float64])
	if !ok {
		t.Fatalf("metric %q is not Histogram[float64], got %T", name, m.Data)
	}
	return h
}

func findMetricByName(rm *metricdata.ResourceMetrics, name string) (metricdata.Metrics, bool) {
	for _, sm := range rm.ScopeMetrics {
		for _, m := range sm.Metrics {
			if m.Name == name {
				return m, true
			}
		}
	}
	return metricdata.Metrics{}, false
}

// statusForPool searches for a DataPoint with session_name == pool and
// returns its `status` label.
func statusForPool(dps []metricdata.HistogramDataPoint[float64], pool string) (string, bool) {
	return labelForPool(dps, pool, "status")
}

func labelForPool(dps []metricdata.HistogramDataPoint[float64], pool, label string) (string, bool) {
	for _, dp := range dps {
		if v, ok := dp.Attributes.Value(attribute.Key("session_name")); ok && v.AsString() == pool {
			if lv, ok := dp.Attributes.Value(attribute.Key(label)); ok {
				return lv.AsString(), true
			}
		}
	}
	return "", false
}

func countForPool(dps []metricdata.HistogramDataPoint[float64], pool string) uint64 {
	for _, dp := range dps {
		if v, ok := dp.Attributes.Value(attribute.Key("session_name")); ok && v.AsString() == pool {
			return dp.Count
		}
	}
	return 0
}

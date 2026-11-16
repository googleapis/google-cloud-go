// Copyright 2026 Google LLC
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//      https://www.apache.org/licenses/LICENSE-2.0
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
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"

	"cloud.google.com/go/monitoring/apiv3/v2/monitoringpb"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/metric"
	"go.opentelemetry.io/otel/metric/noop"
	"go.opentelemetry.io/otel/sdk/metric/metricdata"
	"google.golang.org/api/iterator"
	googlemetricpb "google.golang.org/genproto/googleapis/api/metric"
	"google.golang.org/protobuf/proto"
)

const channelCountMetricName = nativeMetricsPrefix + metricNameChannelPoolChannelCount

// channelCountByState returns the channel_count values keyed by the state
// label, and checks that every point has the expected pool_mode and lane.
func channelCountByState(t *testing.T, rm metricdata.ResourceMetrics, wantLane string) map[string]int64 {
	t.Helper()
	m := requireDCPMetric(t, rm, channelCountMetricName)
	sum, ok := m.Data.(metricdata.Sum[int64])
	if !ok {
		t.Fatalf("channel_count data type mismatch:\n Got: %T\nWant: metricdata.Sum[int64]", m.Data)
	}
	if sum.IsMonotonic {
		t.Fatal("channel_count is monotonic, want an up-down counter")
	}
	got := make(map[string]int64)
	for _, dp := range sum.DataPoints {
		if mode, _ := dp.Attributes.Value(attribute.Key(metricLabelKeyPoolMode)); mode.AsString() != poolModeDynamic {
			t.Fatalf("pool_mode mismatch:\n Got: %q\nWant: %q", mode.AsString(), poolModeDynamic)
		}
		if lane, _ := dp.Attributes.Value(attribute.Key(metricLabelKeyLane)); lane.AsString() != wantLane {
			t.Fatalf("lane mismatch:\n Got: %q\nWant: %q", lane.AsString(), wantLane)
		}
		state, _ := dp.Attributes.Value(attribute.Key(metricLabelKeyState))
		got[state.AsString()] = dp.Value
	}
	return got
}

func requireChannelCounts(t *testing.T, got map[string]int64, wantActive, wantDraining int64) {
	t.Helper()
	if len(got) != 2 || got[channelStateActive] != wantActive || got[channelStateDraining] != wantDraining {
		t.Fatalf("channel_count mismatch:\n Got: %v\nWant: map[active:%d draining:%d]", got, wantActive, wantDraining)
	}
}

func TestChannelPoolBuiltinMetricsConfig(t *testing.T) {
	_, mp := newDCPManualReader()
	tests := []struct {
		name                string
		tf                  *builtinMetricsTracerFactory
		isDirectPathEnabled bool
		disableEnv          string
		wantProvider        bool
		wantLane            string
	}{
		{name: "built-in metrics on", tf: &builtinMetricsTracerFactory{meterProvider: mp}, wantProvider: true, wantLane: laneCloudPath},
		{name: "DirectPath on", tf: &builtinMetricsTracerFactory{meterProvider: mp}, isDirectPathEnabled: true, wantProvider: true, wantLane: laneDirectPath},
		{name: "no tracer factory", tf: nil, wantLane: laneCloudPath},
		{name: "built-in metrics off", tf: &builtinMetricsTracerFactory{}, wantLane: laneCloudPath},
		{name: "off switch set", tf: &builtinMetricsTracerFactory{meterProvider: mp}, disableEnv: "true", wantLane: laneCloudPath},
		{name: "off switch false", tf: &builtinMetricsTracerFactory{meterProvider: mp}, disableEnv: "false", wantProvider: true, wantLane: laneCloudPath},
		{name: "off switch 1", tf: &builtinMetricsTracerFactory{meterProvider: mp}, disableEnv: "1", wantLane: laneCloudPath},
		{name: "off switch TRUE", tf: &builtinMetricsTracerFactory{meterProvider: mp}, disableEnv: "TRUE", wantLane: laneCloudPath},
		{name: "off switch unreadable", tf: &builtinMetricsTracerFactory{meterProvider: mp}, disableEnv: "abc", wantProvider: true, wantLane: laneCloudPath},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv(disableChannelPoolBuiltinMetricsEnvVar, tt.disableEnv)
			cfg := newChannelPoolBuiltinMetricsConfig(tt.tf, tt.isDirectPathEnabled)
			if got := cfg.meterProvider != nil; got != tt.wantProvider {
				t.Fatalf("has meter provider mismatch:\n Got: %v\nWant: %v", got, tt.wantProvider)
			}
			if cfg.lane != tt.wantLane {
				t.Fatalf("lane mismatch:\n Got: %q\nWant: %q", cfg.lane, tt.wantLane)
			}
		})
	}
}

func TestChannelPoolBuiltinMetricsReportsActiveAndDrainingChannels(t *testing.T) {
	cfg := testDCPConfig(3, 1, 4)
	cfg.DCPScaleDownCheckInterval = time.Hour
	_, client, teardown := setupDCPMockedTestServer(t, cfg)
	defer teardown()
	p := client.sc.dynamicPool
	reader, mp := newDCPManualReader()
	m := newChannelPoolBuiltinMetrics(p, channelPoolBuiltinMetricsConfig{meterProvider: mp, lane: laneCloudPath})
	defer m.close(nil)

	requireChannelCounts(t, channelCountByState(t, collectDCPMetrics(t, reader), laneCloudPath), 3, 0)

	drained := p.getEntries()[0]
	removeDCPEntryForTest(p, drained)
	requireChannelCounts(t, channelCountByState(t, collectDCPMetrics(t, reader), laneCloudPath), 2, 1)

	done := make(chan struct{})
	go func() { p.waitForDrainAndClose(drained); close(done) }()
	select {
	case <-done:
	case <-time.After(10 * time.Second):
		t.Fatal("drained channel was not closed")
	}
	requireChannelCounts(t, channelCountByState(t, collectDCPMetrics(t, reader), laneCloudPath), 2, 0)
}

func TestChannelPoolBuiltinMetricsCountsScaledUpChannels(t *testing.T) {
	cfg := testDCPConfig(1, 1, 4)
	cfg.DCPScaleDownCheckInterval = time.Hour
	_, client, teardown := setupDCPMockedTestServer(t, cfg)
	defer teardown()
	p := client.sc.dynamicPool
	reader, mp := newDCPManualReader()
	m := newChannelPoolBuiltinMetrics(p, channelPoolBuiltinMetricsConfig{meterProvider: mp, lane: laneCloudPath})
	defer m.close(nil)

	// Drive scaleUp synchronously, as in the existing DCP metrics tests.
	p.primeSession.Store(client.sm.multiplexedSession.id)
	p.getEntries()[0].unaryLoad.Store(3)
	p.totalRPCLoad.Store(3)
	p.scaleUp()
	p.getEntries()[0].unaryLoad.Store(0)
	p.totalRPCLoad.Store(0)

	if got := p.Num(); got != 3 {
		t.Fatalf("channels after scale-up mismatch:\n Got: %d\nWant: 3", got)
	}
	requireChannelCounts(t, channelCountByState(t, collectDCPMetrics(t, reader), laneCloudPath), 3, 0)
}

func TestChannelPoolBuiltinMetricsDirectPathLane(t *testing.T) {
	cfg := testDCPConfig(2, 1, 4)
	cfg.DCPScaleDownCheckInterval = time.Hour
	_, client, teardown := setupDCPMockedTestServer(t, cfg)
	defer teardown()
	reader, mp := newDCPManualReader()
	m := newChannelPoolBuiltinMetrics(client.sc.dynamicPool, channelPoolBuiltinMetricsConfig{meterProvider: mp, lane: laneDirectPath})
	defer m.close(nil)

	requireChannelCounts(t, channelCountByState(t, collectDCPMetrics(t, reader), laneDirectPath), 2, 0)
}

func TestChannelPoolBuiltinMetricsDisabledWithoutMeterProvider(t *testing.T) {
	_, client, teardown := setupDCPMockedTestServer(t, testDCPConfig(1, 1, 2))
	defer teardown()
	if got := newChannelPoolBuiltinMetrics(client.sc.dynamicPool, channelPoolBuiltinMetricsConfig{lane: laneCloudPath}); got != nil {
		t.Fatalf("metrics without a meter provider mismatch:\n Got: %+v\nWant: nil", got)
	}
	// DisableNativeMetrics is set by the test setup, so the pool has none.
	if got := client.sc.dynamicPool.builtinMetrics; got != nil {
		t.Fatalf("pool built-in metrics with native metrics disabled mismatch:\n Got: %+v\nWant: nil", got)
	}
}

func TestChannelPoolBuiltinMetricsWorksWithNoopMeterProvider(t *testing.T) {
	_, client, teardown := setupDCPMockedTestServer(t, testDCPConfig(1, 1, 2))
	defer teardown()
	m := newChannelPoolBuiltinMetrics(client.sc.dynamicPool, channelPoolBuiltinMetricsConfig{meterProvider: noop.NewMeterProvider(), lane: laneCloudPath})
	if m == nil || m.registration == nil {
		t.Fatalf("metrics with a no-op meter provider mismatch:\n Got: %+v\nWant: a registered callback", m)
	}
	m.close(nil)
	if m.registration != nil {
		t.Fatal("callback still registered after close")
	}
}

func TestChannelPoolBuiltinMetricsPoolCloseUnregistersCallback(t *testing.T) {
	cfg := testDCPConfig(1, 1, 2)
	cfg.DCPScaleDownCheckInterval = time.Hour
	_, client, teardown := setupDCPMockedTestServer(t, cfg)
	defer teardown()
	p := client.sc.dynamicPool
	reader, mp := newDCPManualReader()
	p.builtinMetrics = newChannelPoolBuiltinMetrics(p, channelPoolBuiltinMetricsConfig{meterProvider: mp, lane: laneCloudPath})

	requireChannelCounts(t, channelCountByState(t, collectDCPMetrics(t, reader), laneCloudPath), 1, 0)
	p.Close()
	if _, ok := findDCPMetric(collectDCPMetrics(t, reader), channelCountMetricName); ok {
		t.Fatal("channel_count still reported after dynamicChannelPool.Close")
	}
}

type failingChannelPoolMeterProvider struct {
	noop.MeterProvider
	meter metric.Meter
}

func (p *failingChannelPoolMeterProvider) Meter(string, ...metric.MeterOption) metric.Meter {
	return p.meter
}

type failingChannelPoolMeter struct {
	noop.Meter
	failInstrument bool
	failCallback   bool
}

func (m *failingChannelPoolMeter) Int64ObservableUpDownCounter(name string, opts ...metric.Int64ObservableUpDownCounterOption) (metric.Int64ObservableUpDownCounter, error) {
	if m.failInstrument {
		return nil, errors.New("test instrument registration failure")
	}
	return m.Meter.Int64ObservableUpDownCounter(name, opts...)
}

func (m *failingChannelPoolMeter) RegisterCallback(f metric.Callback, instruments ...metric.Observable) (metric.Registration, error) {
	if m.failCallback {
		return nil, errors.New("test callback registration failure")
	}
	return m.Meter.RegisterCallback(f, instruments...)
}

func TestChannelPoolBuiltinMetricsRegistrationErrorsDisableMetrics(t *testing.T) {
	_, client, teardown := setupDCPMockedTestServer(t, testDCPConfig(1, 1, 2))
	defer teardown()
	tests := []struct {
		name  string
		meter *failingChannelPoolMeter
	}{
		{name: "instrument fails", meter: &failingChannelPoolMeter{failInstrument: true}},
		{name: "callback fails", meter: &failingChannelPoolMeter{failCallback: true}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mp := &failingChannelPoolMeterProvider{meter: tt.meter}
			if got := newChannelPoolBuiltinMetrics(client.sc.dynamicPool, channelPoolBuiltinMetricsConfig{meterProvider: mp, lane: laneCloudPath}); got != nil {
				t.Fatalf("metrics after registration failure mismatch:\n Got: %+v\nWant: nil", got)
			}
			runSelect1(t, client)
		})
	}
}

func TestChannelPoolBuiltinMetricsCollectWhilePoolChanges(t *testing.T) {
	cfg := testDCPConfig(2, 1, 4)
	cfg.DCPScaleDownCheckInterval = time.Hour
	_, client, teardown := setupDCPMockedTestServer(t, cfg)
	defer teardown()
	p := client.sc.dynamicPool
	p.primeSession.Store(client.sm.multiplexedSession.id)
	reader, mp := newDCPManualReader()
	m := newChannelPoolBuiltinMetrics(p, channelPoolBuiltinMetricsConfig{meterProvider: mp, lane: laneCloudPath})
	defer m.close(nil)

	ctx, cancel := context.WithCancel(context.Background())
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		for ctx.Err() == nil {
			rm := metricdata.ResourceMetrics{}
			if err := reader.Collect(context.Background(), &rm); err != nil {
				t.Errorf("Collect() failed: %v", err)
				return
			}
			time.Sleep(time.Millisecond)
		}
	}()
	for i := 0; i < 20; i++ {
		p.getEntries()[0].unaryLoad.Store(3)
		p.totalRPCLoad.Store(3)
		p.scaleUp()
		p.getEntries()[0].unaryLoad.Store(0)
		p.totalRPCLoad.Store(0)
		p.removeEntries(cfg.DCPMaxChannels)
	}
	cancel()
	wg.Wait()

	deadline := time.Now().Add(10 * time.Second)
	for p.drainingCount.Load() != 0 {
		if time.Now().After(deadline) {
			t.Fatalf("draining channels were not closed:\n Got: %d\nWant: 0", p.drainingCount.Load())
		}
		time.Sleep(50 * time.Millisecond)
	}
	requireChannelCounts(t, channelCountByState(t, collectDCPMetrics(t, reader), laneCloudPath), int64(p.Num()), 0)
}

func runSelect1(t *testing.T, client *Client) {
	t.Helper()
	iter := client.Single().Query(context.Background(), NewStatement("SELECT 1"))
	defer iter.Stop()
	for {
		_, err := iter.Next()
		if err == iterator.Done {
			return
		}
		if err != nil {
			t.Fatalf("SELECT 1 failed: %v", err)
		}
	}
}

// exportedChannelCounts returns the channel_count values received by the fake
// Cloud Monitoring server, keyed by the state label. It also checks the metric
// kind, value type, interval and the exact label set of every point, because
// Cloud Monitoring rejects the whole request if one time series is invalid.
func exportedChannelCounts(t *testing.T, reqs []*monitoringpb.CreateTimeSeriesRequest) map[string]int64 {
	t.Helper()
	got := make(map[string]int64)
	for _, req := range reqs {
		for _, ts := range req.TimeSeries {
			if ts.Metric.Type != channelCountMetricName {
				continue
			}
			if ts.MetricKind != googlemetricpb.MetricDescriptor_GAUGE {
				t.Fatalf("metric kind mismatch:\n Got: %v\nWant: GAUGE", ts.MetricKind)
			}
			if ts.ValueType != googlemetricpb.MetricDescriptor_INT64 {
				t.Fatalf("value type mismatch:\n Got: %v\nWant: INT64", ts.ValueType)
			}
			if len(ts.Points) != 1 {
				t.Fatalf("points mismatch:\n Got: %d\nWant: 1", len(ts.Points))
			}
			interval := ts.Points[0].GetInterval()
			if !proto.Equal(interval.GetStartTime(), interval.GetEndTime()) {
				t.Fatalf("gauge interval start and end mismatch:\n Got: %v\nWant: %v", interval.GetStartTime(), interval.GetEndTime())
			}
			labels := ts.Metric.Labels
			state := labels[metricLabelKeyState]
			if state != channelStateActive && state != channelStateDraining {
				t.Fatalf("state label mismatch:\n Got: %q\nWant: %q or %q", state, channelStateActive, channelStateDraining)
			}
			if labels[metricLabelKeyClientUID] == "" {
				t.Fatalf("label %q missing in %v", metricLabelKeyClientUID, labels)
			}
			wantLabels := map[string]string{
				metricLabelKeyPoolMode:   poolModeDynamic,
				metricLabelKeyState:      state,
				metricLabelKeyLane:       laneCloudPath,
				metricLabelKeyClientUID:  labels[metricLabelKeyClientUID],
				metricLabelKeyClientName: clientName,
				metricLabelKeyDatabase:   "[DATABASE]",
			}
			if !reflect.DeepEqual(labels, wantLabels) {
				t.Fatalf("labels mismatch:\n Got: %v\nWant: %v", labels, wantLabels)
			}
			got[state] = ts.Points[0].GetValue().GetInt64Value()
		}
	}
	return got
}

func exportedMetricTypes(reqs []*monitoringpb.CreateTimeSeriesRequest) map[string]bool {
	types := make(map[string]bool)
	for _, req := range reqs {
		for _, ts := range req.TimeSeries {
			types[ts.Metric.Type] = true
		}
	}
	return types
}

// TestChannelPoolBuiltinMetricsExportedToCloudMonitoring checks the whole
// path: client config, pool, built-in meter provider and the Cloud Monitoring
// exporter, using a fake Cloud Monitoring server.
func TestChannelPoolBuiltinMetricsExportedToCloudMonitoring(t *testing.T) {
	if isEmulatorEnvSet() {
		t.Skip("built-in metrics are off with the emulator")
	}
	t.Setenv("SPANNER_DISABLE_BUILTIN_METRICS", "")
	disableDCPDrainIdleFloor(t)
	monitoringServer := installTestMonitoringExporter(t).server

	tests := []struct {
		name       string
		dcp        bool
		drainOne   bool
		disableEnv string
		wantCounts map[string]int64
	}{
		{name: "dynamic pool", dcp: true, drainOne: true, wantCounts: map[string]int64{channelStateActive: 2, channelStateDraining: 1}},
		{name: "dynamic pool with off switch", dcp: true, disableEnv: "true"},
		{name: "static pool", dcp: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv(disableChannelPoolBuiltinMetricsEnvVar, tt.disableEnv)
			config := ClientConfig{}
			if tt.dcp {
				cfg := testDCPConfig(3, 1, 4)
				cfg.DCPScaleDownCheckInterval = time.Hour
				config.DynamicChannelPoolConfig = cfg
			}
			server, client, teardown := setupMockedTestServerWithConfig(t, config)
			defer teardown()
			if got := client.sc.dynamicPool != nil; got != tt.dcp {
				t.Fatalf("dynamic pool enabled mismatch:\n Got: %v\nWant: %v", got, tt.dcp)
			}
			addSelect1Result(server)
			monitoringServer.CreateServiceTimeSeriesRequests()
			runSelect1(t, client)
			if tt.drainOne {
				p := client.sc.dynamicPool
				drainDCPEntryForTest(t, p, p.getEntries()[0])
			}

			// Close flushes the built-in metrics before the pool closes.
			client.Close()
			reqs := monitoringServer.CreateServiceTimeSeriesRequests()
			for _, name := range []string{metricNameOperationCount, metricNameAttemptCount} {
				if !exportedMetricTypes(reqs)[nativeMetricsPrefix+name] {
					t.Fatalf("existing built-in metric %q not exported", nativeMetricsPrefix+name)
				}
			}
			got := exportedChannelCounts(t, reqs)
			if tt.wantCounts == nil {
				if len(got) != 0 {
					t.Fatalf("channel_count points mismatch:\n Got: %v\nWant: none", got)
				}
				return
			}
			requireChannelCounts(t, got, tt.wantCounts[channelStateActive], tt.wantCounts[channelStateDraining])
		})
	}
}

func TestChannelPoolBuiltinMetricsNotSentToCustomerMeterProviders(t *testing.T) {
	if isEmulatorEnvSet() {
		t.Skip("built-in metrics are off with the emulator")
	}
	t.Setenv("SPANNER_DISABLE_BUILTIN_METRICS", "")
	t.Setenv(disableChannelPoolBuiltinMetricsEnvVar, "")
	enableOpenTelemetryMetricsForTest(t)
	disableDCPDrainIdleFloor(t)
	monitoringServer := installTestMonitoringExporter(t).server
	dcpReader, dcpMP := newDCPManualReader()
	clientMetricsReader, clientMetricsMP := newDCPManualReader()
	cfg := testDCPConfig(2, 1, 4)
	cfg.DCPScaleDownCheckInterval = time.Hour
	server, client, teardown := setupMockedTestServerWithConfig(t, ClientConfig{
		DynamicChannelPoolConfig:   cfg,
		OpenTelemetryMeterProvider: dcpMP,
		ClientMetricsProvider:      clientMetricsMP,
	})
	defer teardown()
	addSelect1Result(server)
	runSelect1(t, client)

	customerMetrics := map[string]metricdata.ResourceMetrics{
		"OpenTelemetryMeterProvider": collectDCPMetrics(t, dcpReader),
		"ClientMetricsProvider":      collectDCPMetrics(t, clientMetricsReader),
	}
	// Both customer providers still receive their own metrics.
	requireDCPMetric(t, customerMetrics["OpenTelemetryMeterProvider"], dcpMetricsPrefix+"num_channels")
	requireDCPMetric(t, customerMetrics["ClientMetricsProvider"], clientMetricsPrefix+metricNameOperationCount)
	for provider, rm := range customerMetrics {
		for _, sm := range rm.ScopeMetrics {
			for _, m := range sm.Metrics {
				if strings.HasSuffix(m.Name, metricNameChannelPoolChannelCount) {
					t.Fatalf("%s received %q, want channel_count sent to Cloud Monitoring only", provider, m.Name)
				}
			}
		}
	}

	client.Close()
	requireChannelCounts(t, exportedChannelCounts(t, monitoringServer.CreateServiceTimeSeriesRequests()), 2, 0)
}

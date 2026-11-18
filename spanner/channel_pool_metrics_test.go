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
	"slices"
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
	"google.golang.org/grpc"
	"google.golang.org/protobuf/proto"
)

const (
	channelCountMetricName = nativeMetricsPrefix + metricNameChannelPoolChannelCount
	channelLoadMetricName  = nativeMetricsPrefix + metricNameChannelPoolChannelLoad
)

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

func channelLoadPoint(t *testing.T, rm metricdata.ResourceMetrics, wantLane string) metricdata.HistogramDataPoint[float64] {
	t.Helper()
	m := requireDCPMetric(t, rm, channelLoadMetricName)
	hist, ok := m.Data.(metricdata.Histogram[float64])
	if !ok {
		t.Fatalf("channel_load data type mismatch:\n Got: %T\nWant: metricdata.Histogram[float64]", m.Data)
	}
	if len(hist.DataPoints) != 1 {
		t.Fatalf("channel_load points mismatch:\n Got: %d\nWant: 1", len(hist.DataPoints))
	}
	dp := hist.DataPoints[0]
	wantAttrs := attribute.NewSet(
		attribute.String(metricLabelKeyPoolMode, poolModeDynamic),
		attribute.String(metricLabelKeyLane, wantLane),
	)
	if !dp.Attributes.Equals(&wantAttrs) {
		t.Fatalf("channel_load attributes mismatch:\n Got: %v\nWant: %v", dp.Attributes.ToSlice(), wantAttrs.ToSlice())
	}
	if !slices.Equal(dp.Bounds, channelLoadBucketBounds) {
		t.Fatalf("channel_load buckets mismatch:\n Got: %v\nWant: %v", dp.Bounds, channelLoadBucketBounds)
	}
	return dp
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
	if got := client.sc.dynamicPool.builtinMetrics; got != nil {
		t.Fatalf("pool built-in metrics with native metrics disabled mismatch:\n Got: %+v\nWant: nil", got)
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
	metric.Meter
	failHistogram  bool
	failInstrument bool
	failCallback   bool
}

func (m *failingChannelPoolMeter) Float64Histogram(name string, opts ...metric.Float64HistogramOption) (metric.Float64Histogram, error) {
	if m.failHistogram {
		return nil, errors.New("test histogram registration failure")
	}
	return m.Meter.Float64Histogram(name, opts...)
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

func TestChannelPoolBuiltinMetricsRegistrationErrorKeepsOtherMetric(t *testing.T) {
	cfg := testDCPConfig(1, 1, 2)
	cfg.DCPScaleDownCheckInterval = time.Hour
	server, client, teardown := setupDCPMockedTestServer(t, cfg)
	defer teardown()
	addSelect1Result(server)
	p := client.sc.dynamicPool
	tests := []struct {
		name      string
		fail      func(m *failingChannelPoolMeter)
		wantCount bool
		wantLoad  bool
	}{
		{name: "histogram fails", fail: func(m *failingChannelPoolMeter) { m.failHistogram = true }, wantCount: true},
		{name: "instrument fails", fail: func(m *failingChannelPoolMeter) { m.failInstrument = true }, wantLoad: true},
		{name: "callback fails", fail: func(m *failingChannelPoolMeter) { m.failCallback = true }, wantLoad: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			reader, sdkMP := newDCPManualReader()
			meter := &failingChannelPoolMeter{Meter: sdkMP.Meter(builtInMetricsMeterName)}
			tt.fail(meter)
			p.builtinMetrics = newChannelPoolBuiltinMetrics(p, channelPoolBuiltinMetricsConfig{meterProvider: &failingChannelPoolMeterProvider{meter: meter}, lane: laneCloudPath})
			defer func() {
				p.builtinMetrics.close(nil)
				p.builtinMetrics = nil
			}()
			runSelect1(t, client)

			rm := collectDCPMetrics(t, reader)
			if _, got := findDCPMetric(rm, channelCountMetricName); got != tt.wantCount {
				t.Fatalf("channel_count reported mismatch:\n Got: %v\nWant: %v", got, tt.wantCount)
			}
			if _, got := findDCPMetric(rm, channelLoadMetricName); got != tt.wantLoad {
				t.Fatalf("channel_load reported mismatch:\n Got: %v\nWant: %v", got, tt.wantLoad)
			}
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
	requireChannelCounts(t, channelCountByState(t, collectDCPMetrics(t, reader), laneCloudPath), int64(cfg.DCPMinChannels), 0)
}

func TestChannelPoolBuiltinMetricsRecordsChannelLoad(t *testing.T) {
	tests := []struct {
		name  string
		start func(p *dynamicChannelPool, e *dcpEntry) error
	}{
		{name: "ConnPool Invoke", start: func(p *dynamicChannelPool, e *dcpEntry) error {
			return p.Invoke(context.Background(), "method", nil, nil)
		}},
		{name: "ConnPool NewStream", start: func(p *dynamicChannelPool, e *dcpEntry) error {
			_, err := p.NewStream(context.Background(), &grpc.StreamDesc{}, "method")
			return err
		}},
		{name: "spannerClient unary", start: func(p *dynamicChannelPool, e *dcpEntry) error {
			(&dcpSpannerClient{entry: e}).startUnary(context.Background())(nil)
			return nil
		}},
		{name: "spannerClient stream", start: func(p *dynamicChannelPool, e *dcpEntry) error {
			(&dcpSpannerClient{entry: e}).startStream(context.Background()).done(nil)
			return nil
		}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p, entries := newDCPRefTestPool(t, 1)
			e := entries[0]
			e.pool = &fakeDCPConnPool{}
			reader, mp := newDCPManualReader()
			p.builtinMetrics = newChannelPoolBuiltinMetrics(p, channelPoolBuiltinMetricsConfig{meterProvider: mp, lane: laneDirectPath})
			defer p.builtinMetrics.close(nil)
			e.unaryLoad.Store(1)
			e.streamLoad.Store(1)

			if err := tt.start(p, e); err != nil {
				t.Fatalf("call failed: %v", err)
			}
			dp := channelLoadPoint(t, collectDCPMetrics(t, reader), laneDirectPath)
			if dp.Count != 1 || dp.Sum != 3 {
				t.Fatalf("channel_load mismatch:\n Got: count %d, sum %v\nWant: count 1, sum 3", dp.Count, dp.Sum)
			}
		})
	}
}

func TestChannelPoolBuiltinMetricsRecordsConcurrentChannelLoad(t *testing.T) {
	p, entries := newDCPRefTestPool(t, 2)
	reader, mp := newDCPManualReader()
	p.builtinMetrics = newChannelPoolBuiltinMetrics(p, channelPoolBuiltinMetricsConfig{meterProvider: mp, lane: laneCloudPath})
	defer p.builtinMetrics.close(nil)

	const goroutines, calls = 8, 100
	var wg sync.WaitGroup
	for i := 0; i < goroutines; i++ {
		client := &dcpSpannerClient{entry: entries[i%len(entries)]}
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < calls; j++ {
				client.startUnary(context.Background())(nil)
				if err := reader.Collect(context.Background(), &metricdata.ResourceMetrics{}); err != nil {
					t.Errorf("Collect() failed: %v", err)
					return
				}
			}
		}()
	}
	wg.Wait()

	dp := channelLoadPoint(t, collectDCPMetrics(t, reader), laneCloudPath)
	if dp.Count != goroutines*calls {
		t.Fatalf("channel_load count mismatch:\n Got: %d\nWant: %d", dp.Count, goroutines*calls)
	}
	if min, ok := dp.Min.Value(); !ok || min < 1 {
		t.Fatalf("channel_load min mismatch:\n Got: %v\nWant: at least 1", min)
	}
	if max, ok := dp.Max.Value(); !ok || max > float64(goroutines/len(entries)) {
		t.Fatalf("channel_load max mismatch:\n Got: %v\nWant: at most %d", max, goroutines/len(entries))
	}
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

func exportedChannelLoadCount(t *testing.T, reqs []*monitoringpb.CreateTimeSeriesRequest) int64 {
	t.Helper()
	var count int64
	for _, req := range reqs {
		for _, ts := range req.TimeSeries {
			if ts.Metric.Type != channelLoadMetricName {
				continue
			}
			if ts.MetricKind != googlemetricpb.MetricDescriptor_CUMULATIVE {
				t.Fatalf("metric kind mismatch:\n Got: %v\nWant: CUMULATIVE", ts.MetricKind)
			}
			if ts.ValueType != googlemetricpb.MetricDescriptor_DISTRIBUTION {
				t.Fatalf("value type mismatch:\n Got: %v\nWant: DISTRIBUTION", ts.ValueType)
			}
			if len(ts.Points) != 1 {
				t.Fatalf("points mismatch:\n Got: %d\nWant: 1", len(ts.Points))
			}
			labels := ts.Metric.Labels
			if labels[metricLabelKeyClientUID] == "" {
				t.Fatalf("label %q missing in %v", metricLabelKeyClientUID, labels)
			}
			wantLabels := map[string]string{
				metricLabelKeyPoolMode:   poolModeDynamic,
				metricLabelKeyLane:       laneCloudPath,
				metricLabelKeyClientUID:  labels[metricLabelKeyClientUID],
				metricLabelKeyClientName: clientName,
				metricLabelKeyDatabase:   "[DATABASE]",
			}
			if !reflect.DeepEqual(labels, wantLabels) {
				t.Fatalf("labels mismatch:\n Got: %v\nWant: %v", labels, wantLabels)
			}
			dist := ts.Points[0].GetValue().GetDistributionValue()
			if got := dist.GetBucketOptions().GetExplicitBuckets().GetBounds(); !slices.Equal(got, channelLoadBucketBounds) {
				t.Fatalf("channel_load buckets mismatch:\n Got: %v\nWant: %v", got, channelLoadBucketBounds)
			}
			count += dist.GetCount()
		}
	}
	return count
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
			if got := client.sc.dynamicPool != nil && !client.sc.dynamicPool.fixed; got != tt.dcp {
				t.Fatalf("dynamic pool enabled mismatch:\n Got: %v\nWant: %v", got, tt.dcp)
			}
			addSelect1Result(server)
			monitoringServer.CreateServiceTimeSeriesRequests()
			runSelect1(t, client)
			if tt.drainOne {
				p := client.sc.dynamicPool
				drainDCPEntryForTest(t, p, p.getEntries()[0])
			}

			client.Close()
			reqs := monitoringServer.CreateServiceTimeSeriesRequests()
			for _, name := range []string{metricNameOperationCount, metricNameAttemptCount} {
				if !exportedMetricTypes(reqs)[nativeMetricsPrefix+name] {
					t.Fatalf("existing built-in metric %q not exported", nativeMetricsPrefix+name)
				}
			}
			got := exportedChannelCounts(t, reqs)
			loadCount := exportedChannelLoadCount(t, reqs)
			if tt.wantCounts == nil {
				if len(got) != 0 || loadCount != 0 {
					t.Fatalf("channel pool points mismatch:\n Got: channel_count %v, channel_load count %d\nWant: none", got, loadCount)
				}
				return
			}
			requireChannelCounts(t, got, tt.wantCounts[channelStateActive], tt.wantCounts[channelStateDraining])
			if loadCount == 0 {
				t.Fatal("channel_load not exported")
			}
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
	requireDCPMetric(t, customerMetrics["OpenTelemetryMeterProvider"], dcpMetricsPrefix+"num_channels")
	requireDCPMetric(t, customerMetrics["ClientMetricsProvider"], clientMetricsPrefix+metricNameOperationCount)
	for provider, rm := range customerMetrics {
		for _, sm := range rm.ScopeMetrics {
			for _, m := range sm.Metrics {
				if strings.HasSuffix(m.Name, metricNameChannelPoolChannelCount) || strings.HasSuffix(m.Name, metricNameChannelPoolChannelLoad) {
					t.Fatalf("%s received %q, want channel pool metrics sent to Cloud Monitoring only", provider, m.Name)
				}
			}
		}
	}

	client.Close()
	reqs := monitoringServer.CreateServiceTimeSeriesRequests()
	requireChannelCounts(t, exportedChannelCounts(t, reqs), 2, 0)
	if exportedChannelLoadCount(t, reqs) == 0 {
		t.Fatal("channel_load not exported")
	}
}

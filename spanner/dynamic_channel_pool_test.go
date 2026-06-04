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
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"cloud.google.com/go/spanner/internal"
	. "cloud.google.com/go/spanner/internal/testutil"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/metric"
	"go.opentelemetry.io/otel/metric/noop"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/metric/metricdata"
	"go.opentelemetry.io/otel/sdk/metric/metricdata/metricdatatest"
	"golang.org/x/sync/errgroup"
	"google.golang.org/api/iterator"
	"google.golang.org/api/option"
	gtransport "google.golang.org/api/transport/grpc"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func testDCPConfig(initial, min, max int) DynamicChannelPoolConfig {
	return DynamicChannelPoolConfig{
		DCPEnabled:                           true,
		DCPInitialChannels:                   initial,
		DCPMinChannels:                       min,
		DCPMaxChannels:                       max,
		DCPMaxRPCPerChannel:                  1,
		DCPMinRPCPerChannel:                  0.5,
		DCPScaleDownCheckInterval:            20 * time.Millisecond,
		DCPScaleUpCooldown:                   time.Millisecond,
		DCPDownscaleConsecutiveLowLoadChecks: 1,
		DCPMaxScaleUpPercent:                 100,
		DCPMaxRemoveChannels:                 max,
		DCPDrainIdleGrace:                    10 * time.Millisecond,
		DCPPrimeTimeout:                      time.Second,
		DCPPrimeMaxAttempts:                  3,
	}
}

func setupDCPMockedTestServer(t *testing.T, dcp DynamicChannelPoolConfig) (*MockedSpannerInMemTestServer, *Client, func()) {
	t.Helper()
	return setupDCPMockedTestServerWithMeterProvider(t, dcp, nil)
}

func setupDCPMockedTestServerWithMeterProvider(t *testing.T, dcp DynamicChannelPoolConfig, mp metric.MeterProvider) (*MockedSpannerInMemTestServer, *Client, func()) {
	t.Helper()
	server, client, teardown := setupMockedTestServerWithConfig(t, ClientConfig{
		DisableNativeMetrics:       true,
		DynamicChannelPoolConfig:   dcp,
		OpenTelemetryMeterProvider: mp,
	})
	addSelect1Result(server)
	if client.sc.dynamicPool == nil {
		teardown()
		t.Fatal("dynamic channel pool not enabled")
	}
	return server, client, teardown
}

func newDCPManualReader() (*sdkmetric.ManualReader, *sdkmetric.MeterProvider) {
	reader := sdkmetric.NewManualReader()
	mp := sdkmetric.NewMeterProvider(sdkmetric.WithReader(reader))
	return reader, mp
}

func enableOpenTelemetryMetricsForTest(t *testing.T) {
	t.Helper()
	setOpenTelemetryMetricsFlag(false)
	t.Cleanup(func() { setOpenTelemetryMetricsFlag(false) })
	EnableOpenTelemetryMetrics()
}

func collectDCPMetrics(t *testing.T, reader *sdkmetric.ManualReader) metricdata.ResourceMetrics {
	t.Helper()
	rm := metricdata.ResourceMetrics{}
	if err := reader.Collect(context.Background(), &rm); err != nil {
		t.Fatalf("Collect() failed: %v", err)
	}
	return rm
}

func findDCPMetric(rm metricdata.ResourceMetrics, name string) (metricdata.Metrics, bool) {
	for _, sm := range rm.ScopeMetrics {
		for _, m := range sm.Metrics {
			if m.Name == name {
				return m, true
			}
		}
	}
	return metricdata.Metrics{}, false
}

func requireDCPMetric(t *testing.T, rm metricdata.ResourceMetrics, name string) metricdata.Metrics {
	t.Helper()
	m, ok := findDCPMetric(rm, name)
	if !ok {
		t.Fatalf("metric %q not found in %+v", name, rm.ScopeMetrics)
	}
	return m
}

func requireDCPGaugeValue(t *testing.T, rm metricdata.ResourceMetrics, name string, want int64, attrs []attribute.KeyValue) {
	t.Helper()
	m := requireDCPMetric(t, rm, name)
	gauge, ok := m.Data.(metricdata.Gauge[int64])
	if !ok {
		t.Fatalf("metric %q data type = %T, want metricdata.Gauge[int64]", name, m.Data)
	}
	if got, want := len(gauge.DataPoints), 1; got != want {
		t.Fatalf("metric %q datapoints = %d, want %d", name, got, want)
	}
	if got := gauge.DataPoints[0].Value; got != want {
		t.Fatalf("metric %q value = %d, want %d", name, got, want)
	}
	metricdatatest.AssertHasAttributes[metricdata.DataPoint[int64]](t, gauge.DataPoints[0], attrs...)
}

func dcpCommonAttrs(clientID string) []attribute.KeyValue {
	return []attribute.KeyValue{
		attributeKeyClientID.String(clientID),
		attributeKeyDatabase.String("[DATABASE]"),
		attributeKeyInstance.String("[INSTANCE]"),
		attributeKeyLibVersion.String(internal.Version),
	}
}

func drainDCPQuery(ctx context.Context, client *Client) error {
	iter := client.Single().Query(ctx, NewStatement(SelectSingerIDAlbumIDAlbumTitleFromAlbums))
	defer iter.Stop()
	for {
		_, err := iter.Next()
		if err == iterator.Done {
			return nil
		}
		if err != nil {
			return err
		}
	}
}

func TestDynamicChannelPoolOptInCreatesInitialChannels(t *testing.T) {
	cfg := testDCPConfig(2, 1, 4)
	// This test asserts startup state only. Keep background scale-down from
	// racing the initial channel-count assertion on slow/race builds.
	cfg.DCPScaleDownCheckInterval = time.Hour
	_, client, teardown := setupDCPMockedTestServer(t, cfg)
	defer teardown()

	if got, want := client.sc.dynamicPool.Num(), 2; got != want {
		t.Fatalf("DCP initial channel count = %d, want %d", got, want)
	}
}

func TestDynamicChannelPoolScaleUpPrimesNewChannels(t *testing.T) {
	server, client, teardown := setupDCPMockedTestServer(t, testDCPConfig(1, 1, 4))
	defer teardown()
	server.TestSpanner.PutExecutionTime(MethodExecuteStreamingSql, SimulatedExecutionTime{MinimumExecutionTime: 2 * time.Second})
	if got := len(server.TestSpanner.DumpPings()); got != 0 {
		t.Fatalf("initial DCP channel priming count = %d, want 0", got)
	}

	ctx := context.Background()
	var g errgroup.Group
	for i := 0; i < 3; i++ {
		g.Go(func() error { return drainDCPQuery(ctx, client) })
	}

	waitFor(t, func() error {
		if got := client.sc.dynamicPool.Num(); got <= 1 {
			return fmt.Errorf("DCP channel count = %d, want > 1", got)
		}
		if got := len(server.TestSpanner.DumpPings()); got == 0 {
			return fmt.Errorf("DCP scale-up priming SELECT 1 count = %d, want > 0", got)
		}
		return nil
	})
	if err := g.Wait(); err != nil {
		t.Fatalf("query workload failed: %v", err)
	}
}

func TestDynamicChannelPoolScaleDownRemovesIdleChannelsToMin(t *testing.T) {
	cfg := testDCPConfig(3, 1, 3)
	cfg.DCPDrainIdleGrace = 200 * time.Millisecond
	_, client, teardown := setupDCPMockedTestServer(t, cfg)
	defer teardown()

	if err := drainDCPQuery(context.Background(), client); err != nil {
		t.Fatalf("query failed: %v", err)
	}
	waitFor(t, func() error {
		if got, want := client.sc.dynamicPool.Num(), 1; got != want {
			return fmt.Errorf("DCP channel count after scale-down = %d, want %d", got, want)
		}
		if got := client.sc.dynamicPool.drainingCount.Load(); got == 0 {
			return fmt.Errorf("DCP draining channel count = %d, want > 0 during drain grace", got)
		}
		return nil
	})
	waitFor(t, func() error {
		if got := client.sc.dynamicPool.drainingCount.Load(); got != 0 {
			return fmt.Errorf("DCP draining channel count after grace = %d, want 0", got)
		}
		return nil
	})
}

func TestDynamicChannelPoolScaleDownRequiresRepeatedLowLoad(t *testing.T) {
	cfg := testDCPConfig(3, 1, 3)
	cfg.DCPDownscaleConsecutiveLowLoadChecks = 2
	// This test drives evaluateScaleDown manually. Keep the background monitor
	// from consuming a low-load check first and making the assertion flaky.
	cfg.DCPScaleDownCheckInterval = time.Hour
	_, client, teardown := setupDCPMockedTestServer(t, cfg)
	defer teardown()
	p := client.sc.dynamicPool

	p.evaluateScaleDown()
	if got, want := p.Num(), 3; got != want {
		t.Fatalf("DCP channel count after first low-load check = %d, want %d", got, want)
	}
	p.evaluateScaleDown()
	waitFor(t, func() error {
		if got, want := p.Num(), 1; got != want {
			return fmt.Errorf("DCP channel count after repeated low-load checks = %d, want %d", got, want)
		}
		return nil
	})
}

func TestDynamicChannelPoolPickerSkipsDrainingEntries(t *testing.T) {
	_, client, teardown := setupDCPMockedTestServer(t, testDCPConfig(3, 3, 3))
	defer teardown()
	p := client.sc.dynamicPool
	entries := p.getEntries()
	for _, e := range entries[:2] {
		e.state.Store(dcpStateDraining)
	}
	for i := 0; i < 20; i++ {
		e, err := p.pick(context.Background())
		if err != nil {
			t.Fatalf("pick failed: %v", err)
		}
		if e != entries[2] {
			t.Fatalf("picker returned draining entry %d, want active entry %d", e.id, entries[2].id)
		}
	}
}

func TestDynamicChannelPoolRoundRobinSkipsDrainingEntries(t *testing.T) {
	cfg := testDCPConfig(3, 3, 3)
	cfg.DCPSelectionStrategy = DCPRoundRobin
	_, client, teardown := setupDCPMockedTestServer(t, cfg)
	defer teardown()
	p := client.sc.dynamicPool
	entries := p.getEntries()
	entries[1].state.Store(dcpStateDraining)

	var got []uint64
	for i := 0; i < 4; i++ {
		e, err := p.pick(context.Background())
		if err != nil {
			t.Fatalf("pick failed: %v", err)
		}
		got = append(got, e.id)
	}
	for i, id := range got {
		if id == entries[1].id {
			t.Fatalf("round-robin sequence = %v, picked draining entry %d", got, id)
		}
		if id != entries[0].id && id != entries[2].id {
			t.Fatalf("round-robin sequence = %v, picked unexpected entry %d", got, id)
		}
		if i > 0 && got[i] == got[i-1] {
			t.Fatalf("round-robin sequence = %v, want active entries to alternate", got)
		}
	}
}

func TestDynamicChannelPoolMaxChannelsCapsScaleUp(t *testing.T) {
	server, client, teardown := setupDCPMockedTestServer(t, testDCPConfig(1, 1, 2))
	defer teardown()
	server.TestSpanner.PutExecutionTime(MethodExecuteStreamingSql, SimulatedExecutionTime{MinimumExecutionTime: 300 * time.Millisecond})

	var g errgroup.Group
	for i := 0; i < 8; i++ {
		g.Go(func() error { return drainDCPQuery(context.Background(), client) })
	}
	waitFor(t, func() error {
		if got, want := client.sc.dynamicPool.Num(), 2; got != want {
			return fmt.Errorf("DCP channel count under load = %d, want %d", got, want)
		}
		return nil
	})
	if err := g.Wait(); err != nil {
		t.Fatalf("query workload failed: %v", err)
	}
	if got, max := client.sc.dynamicPool.Num(), 2; got > max {
		t.Fatalf("DCP channel count = %d, want <= %d", got, max)
	}
}

func TestDynamicChannelPoolLocationAwareDisablesDCP(t *testing.T) {
	_, client, teardown := setupMockedTestServerWithConfig(t, ClientConfig{
		DisableNativeMetrics:     true,
		IsExperimentalHost:       true,
		DynamicChannelPoolConfig: testDCPConfig(1, 1, 2),
	})
	defer teardown()
	if client.sc.dynamicPool != nil {
		t.Fatal("DCP enabled with location-aware routing, want disabled")
	}
}

func TestDynamicChannelPoolOTMetricsRequireOpenTelemetryMetricsEnabled(t *testing.T) {
	setOpenTelemetryMetricsFlag(false)
	t.Cleanup(func() { setOpenTelemetryMetricsFlag(false) })
	reader, mp := newDCPManualReader()
	_, _, teardown := setupDCPMockedTestServerWithMeterProvider(t, testDCPConfig(1, 1, 2), mp)
	defer teardown()

	rm := collectDCPMetrics(t, reader)
	if _, ok := findDCPMetric(rm, "spanner/dynamic_channel_pool/num_channels"); ok {
		t.Fatal("DCP metric exported without EnableOpenTelemetryMetrics")
	}
}

func TestDynamicChannelPoolOTMetricsFallbackToGlobalMeterProvider(t *testing.T) {
	enableOpenTelemetryMetricsForTest(t)
	reader, mp := newDCPManualReader()
	oldMP := otel.GetMeterProvider()
	otel.SetMeterProvider(mp)
	t.Cleanup(func() { otel.SetMeterProvider(oldMP) })
	_, client, teardown := setupDCPMockedTestServer(t, testDCPConfig(1, 1, 2))
	defer teardown()

	rm := collectDCPMetrics(t, reader)
	requireDCPGaugeValue(t, rm, "spanner/dynamic_channel_pool/num_channels", 1, dcpCommonAttrs(client.ClientID()))
}

func TestDynamicChannelPoolOTMetricsObserveGaugesWithCommonAttributes(t *testing.T) {
	enableOpenTelemetryMetricsForTest(t)
	reader, mp := newDCPManualReader()
	cfg := testDCPConfig(2, 1, 4)
	cfg.DCPScaleDownCheckInterval = time.Hour
	_, client, teardown := setupDCPMockedTestServerWithMeterProvider(t, cfg, mp)
	defer teardown()
	entries := client.sc.dynamicPool.getEntries()
	entries[0].unaryLoad.Store(3)
	entries[1].streamLoad.Store(4)
	client.sc.dynamicPool.totalRPCLoad.Store(7)

	rm := collectDCPMetrics(t, reader)
	attrs := dcpCommonAttrs(client.ClientID())
	requireDCPGaugeValue(t, rm, "spanner/dynamic_channel_pool/num_channels", 2, attrs)
	requireDCPGaugeValue(t, rm, "spanner/dynamic_channel_pool/draining_channel_count", 0, attrs)
	requireDCPGaugeValue(t, rm, "spanner/dynamic_channel_pool/max_allowed_channels", 4, attrs)
	requireDCPGaugeValue(t, rm, "spanner/dynamic_channel_pool/active_rpc_count", 7, attrs)
	requireDCPGaugeValue(t, rm, "spanner/dynamic_channel_pool/max_active_rpc_per_channel", 4, attrs)
	if _, ok := findDCPMetric(rm, "spanner/dynamic_channel_pool/max_rpc_per_channel"); ok {
		t.Fatal("exported stale max_rpc_per_channel metric, want max_active_rpc_per_channel")
	}
}

func TestDynamicChannelPoolOTMetricsScalingCounterUsesChannelDeltaAndDirection(t *testing.T) {
	enableOpenTelemetryMetricsForTest(t)
	reader, mp := newDCPManualReader()
	cfg := testDCPConfig(1, 1, 4)
	cfg.DCPScaleDownCheckInterval = time.Hour
	_, client, teardown := setupDCPMockedTestServerWithMeterProvider(t, cfg, mp)
	defer teardown()
	p := client.sc.dynamicPool
	p.setPrimeSession(client.sm.multiplexedSession.id)
	p.getEntries()[0].unaryLoad.Store(3)
	p.totalRPCLoad.Store(3)
	p.scaleUp()
	p.getEntries()[0].unaryLoad.Store(0)
	p.totalRPCLoad.Store(0)
	p.removeEntries(1)

	rm := collectDCPMetrics(t, reader)
	m := requireDCPMetric(t, rm, "spanner/dynamic_channel_pool/channel_pool_scaling")
	sum, ok := m.Data.(metricdata.Sum[int64])
	if !ok {
		t.Fatalf("channel_pool_scaling data type = %T, want metricdata.Sum[int64]", m.Data)
	}
	attrs := dcpCommonAttrs(client.ClientID())
	want := map[string]int64{"up": 2, "down": 1}
	if got, want := len(sum.DataPoints), 2; got != want {
		t.Fatalf("channel_pool_scaling datapoints = %d, want %d: %+v", got, want, sum.DataPoints)
	}
	for _, dp := range sum.DataPoints {
		metricdatatest.AssertHasAttributes[metricdata.DataPoint[int64]](t, dp, attrs...)
		direction, ok := dp.Attributes.Value(attribute.Key("direction"))
		if !ok {
			t.Fatalf("channel_pool_scaling datapoint missing direction attr: %+v", dp)
		}
		directionValue := direction.AsString()
		if got, ok := want[directionValue]; !ok || dp.Value != got {
			t.Fatalf("channel_pool_scaling{%s} = %d, want map %v", directionValue, dp.Value, want)
		}
		delete(want, directionValue)
	}
	if len(want) != 0 {
		t.Fatalf("missing channel_pool_scaling directions: %v", want)
	}
}

func TestDynamicChannelPoolOTMetricsCloseUnregistersCallback(t *testing.T) {
	enableOpenTelemetryMetricsForTest(t)
	reader, mp := newDCPManualReader()
	cfg := testDCPConfig(1, 1, 2)
	cfg.DCPScaleDownCheckInterval = time.Hour
	_, client, teardown := setupDCPMockedTestServerWithMeterProvider(t, cfg, mp)
	defer teardown()

	rm := collectDCPMetrics(t, reader)
	requireDCPGaugeValue(t, rm, "spanner/dynamic_channel_pool/num_channels", 1, dcpCommonAttrs(client.ClientID()))
	client.sc.dynamicPool.Close()

	rm = collectDCPMetrics(t, reader)
	if _, ok := findDCPMetric(rm, "spanner/dynamic_channel_pool/num_channels"); ok {
		t.Fatal("DCP metric still exported after dynamicChannelPool.Close")
	}
}

func TestDynamicChannelPoolOTMetricsInstrumentErrorsDisableMetrics(t *testing.T) {
	enableOpenTelemetryMetricsForTest(t)
	_, client, teardown := setupDCPMockedTestServer(t, testDCPConfig(1, 1, 2))
	defer teardown()
	p := client.sc.dynamicPool

	gaugeFailure := &failingDCPMeterProvider{meter: &failingDCPMeter{failGaugeName: dcpMetricsPrefix + "num_channels"}}
	if got := newDCPMetrics(p, gaugeFailure); got != nil {
		t.Fatalf("newDCPMetrics with gauge registration failure = %+v, want nil", got)
	}

	counterFailure := &failingDCPMeterProvider{meter: &failingDCPMeter{failCounterName: dcpMetricsPrefix + "channel_pool_scaling"}}
	if got := newDCPMetrics(p, counterFailure); got != nil {
		t.Fatalf("newDCPMetrics with counter registration failure = %+v, want nil", got)
	}
}

func TestDynamicChannelPoolOTMetricsRecordScalingNoopsWithoutCounter(t *testing.T) {
	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("recordScaling with nil counter panicked: %v", r)
		}
	}()
	(&dcpMetrics{}).recordScaling(context.Background(), 1, "up")
}

func TestDynamicChannelPoolCloseUnregistersMetricsOnce(t *testing.T) {
	cfg := testDCPConfig(1, 1, 2)
	cfg.DCPScaleDownCheckInterval = time.Hour
	_, client, teardown := setupDCPMockedTestServer(t, cfg)
	defer teardown()
	reg := newBlockingMetricRegistration()
	client.sc.dynamicPool.metrics = &dcpMetrics{registration: reg}

	var wg sync.WaitGroup
	for i := 0; i < 2; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_ = client.sc.dynamicPool.Close()
		}()
	}

	select {
	case <-reg.entered:
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for metric unregistration")
	}
	calledTwice := false
	select {
	case <-reg.entered:
		calledTwice = true
	case <-time.After(20 * time.Millisecond):
	}
	close(reg.release)
	wg.Wait()
	if calledTwice {
		t.Fatal("metric unregistration called more than once")
	}
	if got := reg.count.Load(); got != 1 {
		t.Fatalf("metric unregistration count = %d, want 1", got)
	}
}

type failingDCPMeterProvider struct {
	noop.MeterProvider
	meter metric.Meter
}

func (p *failingDCPMeterProvider) Meter(string, ...metric.MeterOption) metric.Meter {
	return p.meter
}

type failingDCPMeter struct {
	noop.Meter
	failGaugeName   string
	failCounterName string
}

func (m *failingDCPMeter) Int64ObservableGauge(name string, opts ...metric.Int64ObservableGaugeOption) (metric.Int64ObservableGauge, error) {
	if name == m.failGaugeName {
		return nil, errors.New("test gauge registration failure")
	}
	return m.Meter.Int64ObservableGauge(name, opts...)
}

func (m *failingDCPMeter) Int64Counter(name string, opts ...metric.Int64CounterOption) (metric.Int64Counter, error) {
	if name == m.failCounterName {
		return nil, errors.New("test counter registration failure")
	}
	return m.Meter.Int64Counter(name, opts...)
}

type blockingMetricRegistration struct {
	noop.Registration
	entered chan struct{}
	release chan struct{}
	count   atomic.Int64
}

func newBlockingMetricRegistration() *blockingMetricRegistration {
	return &blockingMetricRegistration{
		entered: make(chan struct{}, 2),
		release: make(chan struct{}),
	}
}

func (r *blockingMetricRegistration) Unregister() error {
	r.count.Add(1)
	r.entered <- struct{}{}
	<-r.release
	return nil
}

type fakeDCPConnPool struct {
	invokeErr   error
	invokeCount int
	closed      bool
}

func (f *fakeDCPConnPool) Conn() *grpc.ClientConn { return nil }
func (f *fakeDCPConnPool) Num() int               { return 1 }
func (f *fakeDCPConnPool) Close() error {
	f.closed = true
	return nil
}
func (f *fakeDCPConnPool) Invoke(ctx context.Context, method string, args, reply interface{}, opts ...grpc.CallOption) error {
	f.invokeCount++
	return f.invokeErr
}
func (f *fakeDCPConnPool) NewStream(ctx context.Context, desc *grpc.StreamDesc, method string, opts ...grpc.CallOption) (grpc.ClientStream, error) {
	return nil, f.invokeErr
}

func TestDynamicChannelPoolScaleUpPrimeFailureDoesNotPublishEntry(t *testing.T) {
	server, client, teardown := setupDCPMockedTestServer(t, testDCPConfig(1, 1, 2))
	defer teardown()
	server.TestSpanner.PutExecutionTime(MethodExecuteStreamingSql, SimulatedExecutionTime{MinimumExecutionTime: 300 * time.Millisecond})
	server.TestSpanner.PutExecutionTime(MethodExecuteSql, SimulatedExecutionTime{
		Errors:    []error{status.Error(codes.Internal, "prime failed")},
		KeepError: true,
	})

	var g errgroup.Group
	for i := 0; i < 3; i++ {
		g.Go(func() error { return drainDCPQuery(context.Background(), client) })
	}
	waitFor(t, func() error {
		if got := client.sc.dynamicPool.totalRPCLoad.Load(); got == 0 {
			return fmt.Errorf("DCP total RPC load = %d, want in-flight workload", got)
		}
		return nil
	})
	client.sc.dynamicPool.scaleUp()
	if got, want := client.sc.dynamicPool.Num(), 1; got != want {
		t.Fatalf("DCP channel count after failed prime = %d, want %d", got, want)
	}
	for _, e := range client.sc.dynamicPool.getEntries() {
		if e.state.Load() != dcpStateActive {
			t.Fatalf("active slice contains non-active entry state=%d", e.state.Load())
		}
	}
	if _, err := client.sc.dynamicPool.pick(context.Background()); err != nil {
		t.Fatalf("pick after failed scale-up failed: %v", err)
	}
	if err := g.Wait(); err != nil {
		t.Fatalf("query workload failed: %v", err)
	}
}

func TestDynamicChannelPoolPowerOfTwoPrefersLeastLoadedEntry(t *testing.T) {
	_, client, teardown := setupDCPMockedTestServer(t, testDCPConfig(3, 3, 3))
	defer teardown()
	p := client.sc.dynamicPool
	entries := p.getEntries()
	entries[1].unaryLoad.Store(100)
	entries[2].streamLoad.Store(100)

	counts := map[uint64]int{}
	for i := 0; i < 2000; i++ {
		e, err := p.pick(context.Background())
		if err != nil {
			t.Fatalf("pick failed: %v", err)
		}
		counts[e.id]++
	}
	low := counts[entries[0].id]
	high := counts[entries[1].id] + counts[entries[2].id]
	if low <= high {
		t.Fatalf("least-loaded entry picked %d times, higher-load entries picked %d times; want least-loaded preference", low, high)
	}
}

func TestDynamicChannelPoolCloseClosesActiveAndDrainingEntries(t *testing.T) {
	_, client, teardown := setupDCPMockedTestServer(t, testDCPConfig(3, 3, 3))
	defer teardown()
	p := client.sc.dynamicPool
	entries := append([]*dcpEntry(nil), p.getEntries()...)
	entries[1].state.Store(dcpStateDraining)
	p.drainingCount.Add(1)

	client.Close()
	if got := p.Num(); got != 0 {
		t.Fatalf("DCP pool entries after close = %d, want 0", got)
	}
	for _, e := range entries {
		if got := e.state.Load(); got != dcpStateClosed {
			t.Fatalf("entry %d state after close = %d, want closed", e.id, got)
		}
	}
}

func TestDynamicChannelPoolRequestIDUsesEntryChannelID(t *testing.T) {
	interceptorTracker := newInterceptorTracker()
	clientOpts := []option.ClientOption{
		option.WithGRPCDialOption(grpc.WithUnaryInterceptor(interceptorTracker.unaryClientInterceptor)),
		option.WithGRPCDialOption(grpc.WithStreamInterceptor(interceptorTracker.streamClientInterceptor)),
	}
	dcpConfig := testDCPConfig(1, 1, 3)
	dcpConfig.DCPSelectionStrategy = DCPRoundRobin
	server, client, teardown := setupMockedTestServerWithConfigAndClientOptions(t, ClientConfig{
		DisableNativeMetrics:     true,
		DynamicChannelPoolConfig: dcpConfig,
	}, clientOpts)
	defer teardown()
	addSelect1Result(server)
	server.TestSpanner.PutExecutionTime(MethodExecuteStreamingSql, SimulatedExecutionTime{MinimumExecutionTime: 300 * time.Millisecond})

	var g errgroup.Group
	for i := 0; i < 4; i++ {
		g.Go(func() error { return drainDCPQuery(context.Background(), client) })
	}
	waitFor(t, func() error {
		if got := client.sc.dynamicPool.Num(); got <= 1 {
			return fmt.Errorf("DCP channel count = %d, want scale-up", got)
		}
		return nil
	})
	// Run enough post-scale-up public queries to cycle through the active entries
	// and observe the newly added DCP channel id.
	for i := 0; i < client.sc.dynamicPool.Num(); i++ {
		if err := drainDCPQuery(context.Background(), client); err != nil {
			t.Fatalf("post-scale-up query failed: %v", err)
		}
	}
	if err := g.Wait(); err != nil {
		t.Fatalf("query workload failed: %v", err)
	}

	observedChannelIDs := map[uint32]bool{}
	for _, segments := range interceptorTracker.streamClientRequestIDSegments {
		if segments.ChannelID == 0 {
			t.Fatal("request id channel id is zero")
		}
		observedChannelIDs[segments.ChannelID] = true
	}
	if len(observedChannelIDs) <= 1 {
		t.Fatalf("distinct DCP request-id channel ids = %v, want cardinality growth after scale-up", observedChannelIDs)
	}
	if err := interceptorTracker.validateRequestIDsMonotonicity(); err != nil {
		t.Fatal(err)
	}
}

func TestDynamicChannelPoolFullScanFallbackFindsOnlyActiveEntry(t *testing.T) {
	_, client, teardown := setupDCPMockedTestServer(t, testDCPConfig(4, 4, 4))
	defer teardown()
	p := client.sc.dynamicPool
	entries := p.getEntries()
	for _, e := range entries[:3] {
		e.state.Store(dcpStateDraining)
	}
	entries[3].unaryLoad.Store(7)

	e, err := p.pickLeastLoaded()
	if err != nil {
		t.Fatalf("pickLeastLoaded failed: %v", err)
	}
	if e != entries[3] {
		t.Fatalf("full-scan fallback returned entry %d, want only active entry %d", e.id, entries[3].id)
	}
	picked, err := p.pick(context.Background())
	if err != nil {
		t.Fatalf("pick fallback failed: %v", err)
	}
	if picked != entries[3] {
		t.Fatalf("power-of-two fallback returned entry %d, want only active entry %d", picked.id, entries[3].id)
	}
}

func TestDCPResolvingClientRebindsDrainingEntry(t *testing.T) {
	p := &dynamicChannelPool{cfg: testDCPConfig(2, 1, 2)}
	entry1 := &dcpEntry{id: 1, client: &mockSpannerClient{}, parent: p}
	entry2 := &dcpEntry{id: 2, client: &mockSpannerClient{}, parent: p}
	entry1.state.Store(dcpStateActive)
	entry2.state.Store(dcpStateActive)
	entries := []*dcpEntry{entry1, entry2}
	p.entries.Store(&entries)

	resolver := newDCPResolvingSpannerClient(p, entry1.id)
	entry1.state.Store(dcpStateDraining)

	client, err := resolver.resolve(context.Background())
	if err != nil {
		t.Fatalf("resolve failed: %v", err)
	}
	if client != entry2.client {
		t.Fatalf("resolved client = %p, want entry2 client %p", client, entry2.client)
	}
	if got, want := resolver.entryID.Load(), entry2.id; got != want {
		t.Fatalf("resolver entry id = %d, want %d", got, want)
	}
}

func TestDCPResolvingRequestIDReturnsErrorWhenNoEntry(t *testing.T) {
	p := &dynamicChannelPool{cfg: testDCPConfig(1, 1, 1)}
	entries := []*dcpEntry{}
	p.entries.Store(&entries)
	resolver := newDCPResolvingSpannerClient(p, 1)

	if _, err := resolver.requestIDHeaderInjector(context.Background()); err == nil {
		t.Fatal("requestIDHeaderInjector succeeded, want error")
	}
}

func TestDynamicChannelPoolDrainWaitsForActiveStreamLoad(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	cfg := testDCPConfig(1, 1, 1)
	cfg.DCPDrainIdleGrace = 10 * time.Millisecond
	p := &dynamicChannelPool{cfg: cfg, ctx: ctx}
	entry := &dcpEntry{id: 1, pool: &fakeDCPConnPool{}, client: &mockSpannerClient{}, parent: p}
	entry.state.Store(dcpStateDraining)
	entry.streamLoad.Store(1)
	entry.lastActivity.Store(time.Now().Add(-time.Second).UnixNano())
	p.drainingCount.Store(1)

	done := make(chan struct{})
	go func() {
		p.waitForDrainAndClose(entry)
		close(done)
	}()

	select {
	case <-done:
		t.Fatal("drain closed entry with active stream load")
	case <-time.After(50 * time.Millisecond):
	}
	entry.streamLoad.Store(0)
	entry.lastActivity.Store(time.Now().Add(-time.Second).UnixNano())
	waitFor(t, func() error {
		select {
		case <-done:
			return nil
		default:
			return fmt.Errorf("drain did not close after stream load reached zero")
		}
	})
	if got := entry.state.Load(); got != dcpStateClosed {
		t.Fatalf("entry state = %d, want closed", got)
	}
}

func TestDCPStreamContextCancelReleasesStreamLoad(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	p := &dynamicChannelPool{cfg: testDCPConfig(1, 1, 1)}
	entry := &dcpEntry{id: 1, parent: p}
	client := &dcpSpannerClient{entry: entry}

	_ = client.startStream(ctx)
	if got := entry.streamLoad.Load(); got != 1 {
		t.Fatalf("stream load after start = %d, want 1", got)
	}
	cancel()
	waitFor(t, func() error {
		if got := entry.streamLoad.Load(); got != 0 {
			return fmt.Errorf("stream load after context cancel = %d, want 0", got)
		}
		return nil
	})
}

func TestDynamicChannelPoolPowerOfTwoSpreadDoesNotHerd(t *testing.T) {
	_, client, teardown := setupDCPMockedTestServer(t, testDCPConfig(4, 4, 4))
	defer teardown()
	p := client.sc.dynamicPool
	entries := p.getEntries()
	overloaded := entries[0]
	overloaded.unaryLoad.Store(200)

	const workers = 400
	start := make(chan struct{})
	picked := make(chan *dcpEntry, workers)
	var wg sync.WaitGroup
	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			e, err := p.pick(context.Background())
			if err != nil {
				picked <- nil
				return
			}
			picked <- e
		}()
	}
	close(start)

	counts := map[uint64]int{}
	for i := 0; i < workers; i++ {
		e := <-picked
		if e == nil {
			t.Fatalf("worker pick failed")
		}
		counts[e.id]++
	}
	if got := counts[overloaded.id]; got > 60 {
		t.Fatalf("overloaded entry picked %d times, want <= 60; counts=%v", got, counts)
	}
	for _, e := range entries[1:] {
		if got := counts[e.id]; got < 70 {
			t.Fatalf("entry %d picked %d times, want spread across low-load entries; counts=%v", e.id, got, counts)
		}
	}
	var maxLow int
	for _, e := range entries[1:] {
		if got := counts[e.id]; got > maxLow {
			maxLow = got
		}
	}
	if maxLow > 190 {
		t.Fatalf("parallel power-of-two picks herded onto one low-load entry: maxLow=%d counts=%v", maxLow, counts)
	}
	wg.Wait()
}

func TestDynamicChannelPoolScaleUpFloorsCapAtTwo(t *testing.T) {
	// max=8 leaves room above the floor (maxAdd=4), so the floor stays the
	// binding constraint and the assertion is robust to background
	// scaleUpWorker firing before or after the test's explicit scaleUp call.
	// Cooldown=Hour blocks any second scaleUp.
	cfg := testDCPConfig(4, 1, 8)
	cfg.DCPMaxScaleUpPercent = 25 // ceil(4*0.25)=1, floored to 2.
	cfg.DCPScaleUpCooldown = time.Hour
	// This test drives scaleUp manually. Keep background scale-down from
	// removing idle channels before the explicit scale-up assertion.
	cfg.DCPScaleDownCheckInterval = time.Hour
	_, client, teardown := setupDCPMockedTestServer(t, cfg)
	defer teardown()
	p := client.sc.dynamicPool
	waitForDCPScaleUpWorkerIdle(p)
	for _, e := range p.getEntries() {
		e.unaryLoad.Store(10)
	}

	p.scaleUp()
	if got, want := p.Num(), 6; got != want {
		t.Fatalf("DCP channel count after floored scale-up = %d, want %d", got, want)
	}
}

func TestDynamicChannelPoolScaleUpHonorsMaxScaleUpPercent(t *testing.T) {
	// max=20 leaves room above the percent cap (maxAdd=8), so the percent cap
	// stays the binding constraint regardless of worker race ordering.
	// Cooldown=Hour blocks any second scaleUp.
	cfg := testDCPConfig(12, 1, 20)
	cfg.DCPMaxScaleUpPercent = 25 // ceil(12*0.25)=3, above floor.
	cfg.DCPScaleUpCooldown = time.Hour
	// This test drives scaleUp manually. Keep background scale-down from
	// removing idle channels before the explicit scale-up assertion.
	cfg.DCPScaleDownCheckInterval = time.Hour
	_, client, teardown := setupDCPMockedTestServer(t, cfg)
	defer teardown()
	p := client.sc.dynamicPool
	waitForDCPScaleUpWorkerIdle(p)
	for _, e := range p.getEntries() {
		e.unaryLoad.Store(10)
	}

	p.scaleUp()
	if got, want := p.Num(), 15; got != want {
		t.Fatalf("DCP channel count after percent-capped scale-up = %d, want %d", got, want)
	}
}

func waitForDCPScaleUpWorkerIdle(p *dynamicChannelPool) {
	for {
		select {
		case <-p.scaleUpSignal:
			continue
		default:
		}
		break
	}
	p.dialMu.Lock()
	p.dialMu.Unlock()
}

func TestDynamicChannelPoolScaleUpDialFailureDoesNotPublishEntry(t *testing.T) {
	_, client, teardown := setupDCPMockedTestServer(t, testDCPConfig(1, 1, 2))
	defer teardown()
	p := client.sc.dynamicPool
	p.setPrimeSession(client.sm.multiplexedSession.id)
	p.dial = func(context.Context) (gtransport.ConnPool, error) {
		return nil, status.Error(codes.Unavailable, "dial failed")
	}
	initialEntries := append([]*dcpEntry(nil), p.getEntries()...)
	p.getEntries()[0].unaryLoad.Store(10)

	p.scaleUp()
	if got, want := p.Num(), 1; got != want {
		t.Fatalf("DCP channel count after failed dial = %d, want %d", got, want)
	}
	if got := p.getEntries()[0]; got != initialEntries[0] {
		t.Fatalf("active entry pointer changed after failed dial")
	}
	if got := p.lastScaleUp.Load(); got == 0 {
		t.Fatal("lastScaleUp after failed dial = 0, want cooldown to be consumed")
	}
	for _, e := range p.getEntries() {
		if e.state.Load() != dcpStateActive {
			t.Fatalf("active slice contains non-active entry state=%d", e.state.Load())
		}
	}
}

func TestDynamicChannelPoolConfigDefaultsInitialChannelsToMinWhenInitialUnset(t *testing.T) {
	cfg, err := normalizeDCPConfig(DynamicChannelPoolConfig{DCPEnabled: true, DCPMinChannels: 8, DCPMaxChannels: 10})
	if err != nil {
		t.Fatalf("normalizeDCPConfig failed: %v", err)
	}
	if got, want := cfg.DCPInitialChannels, 8; got != want {
		t.Fatalf("DCPInitialChannels = %d, want min channels %d", got, want)
	}
}

func TestDynamicChannelPoolConfigRejectsExplicitInitialBelowMin(t *testing.T) {
	_, err := normalizeDCPConfig(DynamicChannelPoolConfig{DCPEnabled: true, DCPInitialChannels: 4, DCPMinChannels: 8, DCPMaxChannels: 10})
	if err == nil {
		t.Fatal("normalizeDCPConfig succeeded, want error")
	}
}

func TestDynamicChannelPoolConfigRejectsNegativeScaleDownInterval(t *testing.T) {
	_, err := normalizeDCPConfig(DynamicChannelPoolConfig{DCPEnabled: true, DCPScaleDownCheckInterval: -time.Second})
	if err == nil {
		t.Fatal("normalizeDCPConfig succeeded, want error")
	}
}

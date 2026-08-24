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
	"io"
	"math"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"cloud.google.com/go/spanner/apiv1/spannerpb"
	"cloud.google.com/go/spanner/internal"
	. "cloud.google.com/go/spanner/internal/testutil"
	"github.com/googleapis/gax-go/v2/apierror"
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
		DCPErrorPenaltyStep:                  1,
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
		t.Fatalf("metric %q data type mismatch:\n Got: %T\nWant: metricdata.Gauge[int64]", name, m.Data)
	}
	if got, want := len(gauge.DataPoints), 1; got != want {
		t.Fatalf("metric %q datapoints mismatch:\n Got: %d\nWant: %d", name, got, want)
	}
	if got := gauge.DataPoints[0].Value; got != want {
		t.Fatalf("metric %q value mismatch:\n Got: %d\nWant: %d", name, got, want)
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
		t.Fatalf("DCP initial channel count mismatch:\n Got: %d\nWant: %d", got, want)
	}
}

func TestDynamicChannelPoolScaleUpPrimesNewChannels(t *testing.T) {
	server, client, teardown := setupDCPMockedTestServer(t, testDCPConfig(1, 1, 4))
	defer teardown()
	server.TestSpanner.PutExecutionTime(MethodExecuteStreamingSql, SimulatedExecutionTime{MinimumExecutionTime: 2 * time.Second})
	if got := len(server.TestSpanner.DumpPings()); got != 0 {
		t.Fatalf("initial DCP channel priming count mismatch:\n Got: %d\nWant: 0", got)
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
		t.Fatalf("DCP channel count after first low-load check mismatch:\n Got: %d\nWant: %d", got, want)
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
			t.Fatalf("picked entry mismatch:\n Got: draining entry %d\nWant: active entry %d", e.id, entries[2].id)
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
			t.Fatalf("round-robin sequence mismatch:\n Got: %v\nWant: active entries to alternate", got)
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
		t.Fatalf("DCP channel count mismatch:\n Got: %d\nWant: <= %d", got, max)
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
	// This test drives scaleUp synchronously; seed priming without also waking the
	// background worker and racing the explicit call.
	p.primeSession.Store(client.sm.multiplexedSession.id)
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
		t.Fatalf("channel_pool_scaling data type mismatch:\n Got: %T\nWant: metricdata.Sum[int64]", m.Data)
	}
	attrs := dcpCommonAttrs(client.ClientID())
	want := map[string]int64{"up": 2, "down": 1}
	if got, want := len(sum.DataPoints), 2; got != want {
		t.Fatalf("channel_pool_scaling datapoints mismatch:\n Got: %d\nWant: %d: %+v", got, want, sum.DataPoints)
	}
	for _, dp := range sum.DataPoints {
		metricdatatest.AssertHasAttributes[metricdata.DataPoint[int64]](t, dp, attrs...)
		direction, ok := dp.Attributes.Value(attribute.Key("direction"))
		if !ok {
			t.Fatalf("channel_pool_scaling datapoint missing direction attr: %+v", dp)
		}
		directionValue := direction.AsString()
		if got, ok := want[directionValue]; !ok || dp.Value != got {
			t.Fatalf("channel_pool_scaling{%s} mismatch:\n Got: %d\nWant: map %v", directionValue, dp.Value, want)
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
		t.Fatalf("newDCPMetrics with gauge registration failure mismatch:\n Got: %+v\nWant: nil", got)
	}

	counterFailure := &failingDCPMeterProvider{meter: &failingDCPMeter{failCounterName: dcpMetricsPrefix + "channel_pool_scaling"}}
	if got := newDCPMetrics(p, counterFailure); got != nil {
		t.Fatalf("newDCPMetrics with counter registration failure mismatch:\n Got: %+v\nWant: nil", got)
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
		t.Fatalf("metric unregistration count mismatch:\n Got: %d\nWant: 1", got)
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
	invokeErr      error
	invokeCount    int
	newStreamCount int
	closed         bool
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
	f.newStreamCount++
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
		t.Fatalf("DCP channel count after failed prime mismatch:\n Got: %d\nWant: %d", got, want)
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
		t.Fatalf("DCP pool entries after close mismatch:\n Got: %d\nWant: 0", got)
	}
	for _, e := range entries {
		if got := e.state.Load(); got != dcpStateClosed {
			t.Fatalf("entry %d state after close mismatch:\n Got: %d\nWant: closed", e.id, got)
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
		t.Fatalf("distinct DCP request-id channel ids mismatch:\n Got: %v\nWant: cardinality growth after scale-up", observedChannelIDs)
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
		t.Fatalf("full-scan fallback entry mismatch:\n Got: entry %d\nWant: only active entry %d", e.id, entries[3].id)
	}
	picked, err := p.pick(context.Background())
	if err != nil {
		t.Fatalf("pick fallback failed: %v", err)
	}
	if picked != entries[3] {
		t.Fatalf("power-of-two fallback entry mismatch:\n Got: entry %d\nWant: only active entry %d", picked.id, entries[3].id)
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
		t.Fatalf("resolved client mismatch:\n Got: %p\nWant: entry2 client %p", client, entry2.client)
	}
	if got, want := resolver.entryID.Load(), entry2.id; got != want {
		t.Fatalf("resolver entry id mismatch:\n Got: %d\nWant: %d", got, want)
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
		t.Fatalf("entry state mismatch:\n Got: %d\nWant: closed", got)
	}
}

func TestDCPStreamContextCancelReleasesStreamLoad(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	p := &dynamicChannelPool{cfg: testDCPConfig(1, 1, 1)}
	entry := &dcpEntry{id: 1, parent: p}
	client := &dcpSpannerClient{entry: entry}

	_ = client.startStream(ctx)
	if got := entry.streamLoad.Load(); got != 1 {
		t.Fatalf("stream load after start mismatch:\n Got: %d\nWant: 1", got)
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
		t.Fatalf("overloaded entry pick count mismatch:\n Got: %d\nWant: <= 60\ncounts: %v", got, counts)
	}
	for _, e := range entries[1:] {
		if got := counts[e.id]; got < 70 {
			t.Fatalf("entry %d pick count mismatch:\n Got: %d\nWant: spread across low-load entries\ncounts: %v", e.id, got, counts)
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
		t.Fatalf("DCP channel count after floored scale-up mismatch:\n Got: %d\nWant: %d", got, want)
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
		t.Fatalf("DCP channel count after percent-capped scale-up mismatch:\n Got: %d\nWant: %d", got, want)
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
	// This test drives scaleUp synchronously; seed priming without also waking the
	// background worker and racing the explicit call.
	p.primeSession.Store(client.sm.multiplexedSession.id)
	p.dial = func(context.Context) (gtransport.ConnPool, error) {
		return nil, status.Error(codes.Unavailable, "dial failed")
	}
	initialEntries := append([]*dcpEntry(nil), p.getEntries()...)
	p.getEntries()[0].unaryLoad.Store(10)

	p.scaleUp()
	if got, want := p.Num(), 1; got != want {
		t.Fatalf("DCP channel count after failed dial mismatch:\n Got: %d\nWant: %d", got, want)
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

func TestDefaultDynamicChannelPoolConfigValues(t *testing.T) {
	// Pins the documented DCP default values via a full-struct comparison, so any
	// default change (or a new knob added without a pinned default) is a deliberate,
	// test-visible decision rather than a silently green diff.
	got := DefaultDynamicChannelPoolConfig()
	want := DynamicChannelPoolConfig{
		DCPInitialChannels:                   4,
		DCPMinChannels:                       2,
		DCPMaxChannels:                       10,
		DCPMaxRPCPerChannel:                  25,
		DCPMinRPCPerChannel:                  15,
		DCPErrorPenaltyStep:                  5,
		DCPErrorPenaltyDuration:              5 * time.Second,
		DCPScaleDownCheckInterval:            3 * time.Minute,
		DCPScaleUpCooldown:                   10 * time.Second,
		DCPDownscaleConsecutiveLowLoadChecks: 3,
		DCPMaxScaleUpPercent:                 30,
		DCPMaxRemoveChannels:                 2,
		DCPDrainIdleGrace:                    time.Minute,
		DCPPrimeTimeout:                      10 * time.Second,
		DCPPrimeMaxAttempts:                  3,
		DCPSelectionStrategy:                 DCPPowerOfTwoLeastBusy,
	}
	if got != want {
		t.Fatalf("DefaultDynamicChannelPoolConfig() mismatch:\n Got: %+v\nWant: %+v", got, want)
	}
}

func TestDynamicChannelPoolConfigDefaultsInitialChannelsToMinWhenInitialUnset(t *testing.T) {
	cfg, err := normalizeDCPConfig(DynamicChannelPoolConfig{DCPEnabled: true, DCPMinChannels: 8, DCPMaxChannels: 10})
	if err != nil {
		t.Fatalf("normalizeDCPConfig failed: %v", err)
	}
	if got, want := cfg.DCPInitialChannels, 8; got != want {
		t.Fatalf("DCPInitialChannels mismatch:\n Got: %d\nWant: min channels %d", got, want)
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

func TestDCPApplyErrorPenaltyCodes(t *testing.T) {
	apiErr, ok := apierror.FromError(status.Error(codes.Unavailable, "unavailable"))
	if !ok {
		t.Fatal("apierror.FromError() did not recognize Unavailable status")
	}
	tests := []struct {
		name        string
		err         error
		duration    time.Duration
		wantPenalty bool
	}{
		{name: "nil", duration: time.Second},
		{name: "OK", err: status.Error(codes.OK, "ok"), duration: time.Second},
		{name: "Unavailable", err: status.Error(codes.Unavailable, "unavailable"), duration: time.Second, wantPenalty: true},
		{name: "ResourceExhausted", err: status.Error(codes.ResourceExhausted, "resource exhausted"), duration: time.Second, wantPenalty: true},
		{name: "APIError Unavailable", err: apiErr, duration: time.Second, wantPenalty: true},
		{name: "Internal", err: status.Error(codes.Internal, "internal"), duration: time.Second},
		{name: "DeadlineExceeded", err: status.Error(codes.DeadlineExceeded, "deadline"), duration: time.Second},
		{name: "Canceled", err: status.Error(codes.Canceled, "canceled"), duration: time.Second},
		{name: "Aborted", err: status.Error(codes.Aborted, "aborted"), duration: time.Second},
		{name: "EOF", err: io.EOF, duration: time.Second},
		{name: "wrapped spanner error", err: fmt.Errorf("wrapped: %w", spannerErrorf(codes.Unavailable, "unavailable")), duration: time.Second, wantPenalty: true},
		{name: "disabled", err: status.Error(codes.Unavailable, "unavailable"), duration: -time.Second},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			p := &dynamicChannelPool{penaltyMax: 25, cfg: DynamicChannelPoolConfig{
				DCPErrorPenaltyStep:     5,
				DCPErrorPenaltyDuration: test.duration,
			}}
			e := &dcpEntry{parent: p}
			before := time.Now().UnixNano()
			e.applyErrorPenalty(test.err)
			expiry := e.penaltyExpiry.Load()
			if test.wantPenalty {
				if expiry <= before {
					t.Fatalf("penalty expiry mismatch:\n Got: %d\nWant: after %d", expiry, before)
				}
				if got, want := e.penaltyLoad.Load(), int32(5); got != want {
					t.Fatalf("penalty load mismatch:\n Got: %d\nWant: %d", got, want)
				}
				return
			}
			if expiry != 0 {
				t.Fatalf("penalty expiry mismatch:\n Got: %d\nWant: 0", expiry)
			}
			if got := e.penaltyLoad.Load(); got != 0 {
				t.Fatalf("penalty load mismatch:\n Got: %d\nWant: 0", got)
			}
		})
	}
}

func TestDCPApplyErrorPenaltyAccumulatesCapsAndRestarts(t *testing.T) {
	p := &dynamicChannelPool{penaltyMax: 15, cfg: DynamicChannelPoolConfig{
		DCPMaxRPCPerChannel:     15,
		DCPErrorPenaltyStep:     5,
		DCPErrorPenaltyDuration: time.Hour,
	}}
	e := &dcpEntry{parent: p}
	errUnavailable := status.Error(codes.Unavailable, "unavailable")

	e.applyErrorPenalty(errUnavailable)
	if got, want := e.penaltyLoad.Load(), int32(5); got != want {
		t.Fatalf("penalty after one error mismatch:\n Got: %d\nWant: %d", got, want)
	}
	firstExpiry := e.penaltyExpiry.Load()
	time.Sleep(time.Millisecond)
	e.applyErrorPenalty(errUnavailable)
	e.applyErrorPenalty(errUnavailable)
	if got, want := e.penaltyLoad.Load(), int32(15); got != want {
		t.Fatalf("penalty after three errors mismatch:\n Got: %d\nWant: %d", got, want)
	}
	if got := e.penaltyExpiry.Load(); got <= firstExpiry {
		t.Fatalf("penalty expiry after later error mismatch:\n Got: %d\nWant: after %d", got, firstExpiry)
	}
	e.applyErrorPenalty(errUnavailable)
	if got, want := e.penaltyLoad.Load(), int32(15); got != want {
		t.Fatalf("penalty beyond cap mismatch:\n Got: %d\nWant: %d", got, want)
	}

	e.penaltyExpiry.Store(time.Now().Add(-time.Second).UnixNano())
	e.applyErrorPenalty(errUnavailable)
	if got, want := e.penaltyLoad.Load(), int32(5); got != want {
		t.Fatalf("penalty after expired window mismatch:\n Got: %d\nWant: fresh step %d", got, want)
	}
}

func TestDCPErrorPenaltyAggregateAppliesCappedDeltas(t *testing.T) {
	p := &dynamicChannelPool{penaltyMax: 12, cfg: DynamicChannelPoolConfig{
		DCPMaxRPCPerChannel:     12,
		DCPErrorPenaltyStep:     5,
		DCPErrorPenaltyDuration: time.Hour,
	}}
	first := &dcpEntry{parent: p}
	second := &dcpEntry{parent: p}
	errUnavailable := status.Error(codes.Unavailable, "unavailable")

	first.applyErrorPenalty(errUnavailable)
	second.applyErrorPenalty(errUnavailable)
	if got, want := p.totalPenaltyLoad.Load(), int64(10); got != want {
		t.Fatalf("total penalty after two entries mismatch:\n Got: %d\nWant: %d", got, want)
	}
	first.applyErrorPenalty(errUnavailable)
	first.applyErrorPenalty(errUnavailable)
	first.applyErrorPenalty(errUnavailable)
	if got, want := first.currentPenalty(), int32(12); got != want {
		t.Fatalf("first entry penalty at cap mismatch:\n Got: %d\nWant: %d", got, want)
	}
	if got, want := p.totalPenaltyLoad.Load(), int64(17); got != want {
		t.Fatalf("total penalty after capped increments mismatch:\n Got: %d\nWant: %d", got, want)
	}
}

func TestDCPErrorPenaltyAggregateDoesNotOverflowInt32(t *testing.T) {
	p := &dynamicChannelPool{penaltyMax: math.MaxInt32, cfg: DynamicChannelPoolConfig{
		DCPErrorPenaltyStep:     math.MaxInt32,
		DCPErrorPenaltyDuration: time.Hour,
	}}
	errUnavailable := status.Error(codes.Unavailable, "unavailable")
	for range 2 {
		e := &dcpEntry{parent: p}
		e.applyErrorPenalty(errUnavailable)
	}
	if got, want := int64(p.totalPenaltyLoad.Load()), int64(2*math.MaxInt32); got != want {
		t.Fatalf("total penalty mismatch:\n Got: %d\nWant: %d", got, want)
	}
}

func TestDCPErrorPenaltyAggregateExpirySubtractsExactlyOnce(t *testing.T) {
	p := &dynamicChannelPool{penaltyMax: 25, cfg: DynamicChannelPoolConfig{
		DCPMaxRPCPerChannel:     25,
		DCPErrorPenaltyStep:     5,
		DCPErrorPenaltyDuration: time.Hour,
	}}
	e := &dcpEntry{parent: p}
	e.applyErrorPenalty(status.Error(codes.Unavailable, "unavailable"))
	e.penaltyExpiry.Store(time.Now().Add(-time.Second).UnixNano())

	const readers = 100
	start := make(chan struct{})
	var wg sync.WaitGroup
	wg.Add(readers)
	for range readers {
		go func() {
			defer wg.Done()
			<-start
			if got := e.currentPenalty(); got != 0 {
				t.Errorf("currentPenalty() after expiry mismatch:\n Got: %d\nWant: 0", got)
			}
		}()
	}
	close(start)
	wg.Wait()

	if got := p.totalPenaltyLoad.Load(); got != 0 {
		t.Fatalf("total penalty after concurrent expiry mismatch:\n Got: %d\nWant: 0", got)
	}
}

func TestDCPCurrentPenaltyAndPickLoadWindow(t *testing.T) {
	p := &dynamicChannelPool{}
	e := &dcpEntry{parent: p}
	e.unaryLoad.Store(2)
	e.streamLoad.Store(3)
	e.penaltyLoad.Store(10)
	e.penaltyExpiry.Store(time.Now().Add(time.Second).UnixNano())
	p.totalPenaltyLoad.Store(10)

	if got, want := e.currentPenalty(), int32(10); got != want {
		t.Fatalf("currentPenalty() during window mismatch:\n Got: %d\nWant: %d", got, want)
	}
	if got, want := e.pickLoad(), int32(15); got != want {
		t.Fatalf("pickLoad() during penalty mismatch:\n Got: %d\nWant: %d", got, want)
	}
	if got, want := e.rpcLoad(), int32(5); got != want {
		t.Fatalf("rpcLoad() during penalty mismatch:\n Got: %d\nWant: %d", got, want)
	}
	if got, want := e.weightedLoad(), int32(5); got != want {
		t.Fatalf("weightedLoad() during penalty mismatch:\n Got: %d\nWant: %d", got, want)
	}

	e.penaltyExpiry.Store(time.Now().Add(-time.Second).UnixNano())
	if got, want := e.pickLoad(), int32(5); got != want {
		t.Fatalf("pickLoad() after penalty mismatch:\n Got: %d\nWant: %d", got, want)
	}
	if got := e.penaltyExpiry.Load(); got != 0 {
		t.Fatalf("expired penalty mismatch:\n Got: %d\nWant: CAS reset to 0", got)
	}
	if got := e.currentPenalty(); got != 0 {
		t.Fatalf("currentPenalty() after lazy reset mismatch:\n Got: %d\nWant: 0", got)
	}
}

func TestDCPCurrentPenaltyAfterNewWindow(t *testing.T) {
	p := &dynamicChannelPool{penaltyMax: 25, cfg: DynamicChannelPoolConfig{
		DCPMaxRPCPerChannel:     25,
		DCPErrorPenaltyStep:     5,
		DCPErrorPenaltyDuration: time.Second,
	}}
	e := &dcpEntry{parent: p}
	e.penaltyLoad.Store(25)
	e.penaltyExpiry.Store(time.Now().Add(-time.Second).UnixNano())
	p.totalPenaltyLoad.Store(25)

	e.applyErrorPenalty(status.Error(codes.Unavailable, "unavailable"))
	if got, want := e.currentPenalty(), int32(5); got != want {
		t.Fatalf("currentPenalty() after new window mismatch:\n Got: %d\nWant: %d", got, want)
	}
	// A unit test cannot observe the atomic update's transient states, but this
	// pins its net-delta contract: replace the old counted contribution with new.
	if got, want := p.totalPenaltyLoad.Load(), int64(5); got != want {
		t.Fatalf("total penalty after new window mismatch:\n Got: %d\nWant: net contribution %d", got, want)
	}
}

func TestDynamicChannelPoolPowerOfTwoErrorPenalty(t *testing.T) {
	p := &dynamicChannelPool{penaltyMax: 25, cfg: DynamicChannelPoolConfig{
		DCPMaxRPCPerChannel:     25,
		DCPErrorPenaltyStep:     5,
		DCPErrorPenaltyDuration: time.Hour,
	}}
	healthy := &dcpEntry{id: 1, parent: p}
	penalized := &dcpEntry{id: 2, parent: p}
	healthy.unaryLoad.Store(8)
	penalized.unaryLoad.Store(2)
	penalized.applyErrorPenalty(status.Error(codes.Unavailable, "unavailable"))
	entries := []*dcpEntry{healthy, penalized}
	p.entries.Store(&entries)

	const picks = 1000
	penalizedPicks := 0
	for range picks {
		e, err := p.pickPowerOfTwo()
		if err != nil {
			t.Fatalf("pickPowerOfTwo() failed: %v", err)
		}
		if e == penalized {
			penalizedPicks++
		}
	}
	if penalizedPicks < 650 || penalizedPicks > 850 {
		t.Fatalf("single-step entry pick share mismatch:\n Got: %d/%d\nWant: mixed-pair wins (roughly 75%%)", penalizedPicks, picks)
	}

	healthy.unaryLoad.Store(0)
	penalized.unaryLoad.Store(0)
	penalized.penaltyLoad.Store(25)
	penalized.penaltyExpiry.Store(time.Now().Add(time.Hour).UnixNano())
	penalizedPicks = 0
	for range picks {
		e, err := p.pickPowerOfTwo()
		if err != nil {
			t.Fatalf("pickPowerOfTwo() at penalty cap failed: %v", err)
		}
		if e == penalized {
			penalizedPicks++
		}
	}
	if penalizedPicks < 175 || penalizedPicks > 325 {
		t.Fatalf("max-penalty entry pick share mismatch:\n Got: %d/%d\nWant: only same-entry draws (roughly 25%%)", penalizedPicks, picks)
	}
}

func TestDynamicChannelPoolLeastLoadedErrorPenalty(t *testing.T) {
	p := &dynamicChannelPool{}
	penalized := &dcpEntry{id: 1, parent: p}
	healthy := &dcpEntry{id: 2, parent: p}
	penalized.penaltyLoad.Store(25)
	penalized.penaltyExpiry.Store(time.Now().Add(time.Hour).UnixNano())
	entries := []*dcpEntry{penalized, healthy}
	p.entries.Store(&entries)

	got, err := p.pickLeastLoaded()
	if err != nil {
		t.Fatalf("pickLeastLoaded() failed: %v", err)
	}
	if got != healthy {
		t.Fatalf("pickLeastLoaded() mismatch:\n Got: entry %d\nWant: healthy entry %d", got.id, healthy.id)
	}

	healthy.penaltyLoad.Store(25)
	healthy.penaltyExpiry.Store(time.Now().Add(time.Hour).UnixNano())
	got, err = p.pickLeastLoaded()
	if err != nil {
		t.Fatalf("pickLeastLoaded() with all entries penalized failed: %v", err)
	}
	if got == nil {
		t.Fatal("pickLeastLoaded() with all entries penalized returned nil")
	}
}

func TestDynamicChannelPoolRoundRobinIgnoresErrorPenalty(t *testing.T) {
	p := &dynamicChannelPool{}
	penalized := &dcpEntry{id: 1, parent: p}
	healthy := &dcpEntry{id: 2, parent: p}
	penalized.penaltyLoad.Store(25)
	penalized.penaltyExpiry.Store(time.Now().Add(time.Hour).UnixNano())
	entries := []*dcpEntry{penalized, healthy}
	p.entries.Store(&entries)

	const picks = 1000
	penalizedPicks := 0
	for range picks {
		e, err := p.pickRoundRobin()
		if err != nil {
			t.Fatalf("pickRoundRobin() failed: %v", err)
		}
		if e == penalized {
			penalizedPicks++
		}
	}
	if penalizedPicks < 450 || penalizedPicks > 550 {
		t.Fatalf("penalized entry pick share mismatch:\n Got: %d/%d\nWant: roughly even share", penalizedPicks, picks)
	}
}

func TestDCPErrorPenaltyCallPaths(t *testing.T) {
	penaltyConfig := DynamicChannelPoolConfig{
		DCPMaxRPCPerChannel:     25,
		DCPErrorPenaltyStep:     5,
		DCPErrorPenaltyDuration: time.Hour,
	}
	newPenaltyPool := func() *dynamicChannelPool {
		return &dynamicChannelPool{cfg: penaltyConfig, penaltyMax: 25}
	}
	errUnavailable := status.Error(codes.Unavailable, "unavailable")

	t.Run("ConnPool Invoke", func(t *testing.T) {
		pool := &fakeDCPConnPool{invokeErr: errUnavailable}
		p := newPenaltyPool()
		e := &dcpEntry{id: 1, pool: pool, parent: p}
		entries := []*dcpEntry{e}
		p.entries.Store(&entries)
		if err := p.Invoke(context.Background(), "method", nil, nil); !errors.Is(err, errUnavailable) {
			t.Fatalf("Invoke() error mismatch:\n Got: %v\nWant: %v", err, errUnavailable)
		}
		if pool.invokeCount != 1 {
			t.Fatalf("underlying Invoke count mismatch:\n Got: %d\nWant: 1", pool.invokeCount)
		}
		if e.penaltyExpiry.Load() == 0 {
			t.Fatal("Invoke() did not apply error penalty")
		}
		if got, want := e.penaltyLoad.Load(), int32(5); got != want {
			t.Fatalf("Invoke() penalty mismatch:\n Got: %d\nWant: exactly one step %d", got, want)
		}
	})

	t.Run("ConnPool tracked stream", func(t *testing.T) {
		p := newPenaltyPool()
		e := &dcpEntry{parent: p}
		e.streamLoad.Store(1)
		s := &dcpConnPoolTrackedStream{entry: e}
		s.finish(errUnavailable)
		expiry := e.penaltyExpiry.Load()
		if expiry == 0 {
			t.Fatal("tracked stream did not apply error penalty")
		}
		time.Sleep(time.Millisecond)
		s.finish(errUnavailable)
		if got := e.penaltyExpiry.Load(); got != expiry {
			t.Fatalf("tracked stream applied penalty more than once: first %d, then %d", expiry, got)
		}
		if got, want := e.penaltyLoad.Load(), int32(5); got != want {
			t.Fatalf("tracked stream penalty mismatch:\n Got: %d\nWant: exactly one step %d", got, want)
		}
	})

	t.Run("ConnPool stream creation", func(t *testing.T) {
		pool := &fakeDCPConnPool{invokeErr: errUnavailable}
		p := newPenaltyPool()
		e := &dcpEntry{id: 1, pool: pool, parent: p}
		entries := []*dcpEntry{e}
		p.entries.Store(&entries)
		if _, err := p.NewStream(context.Background(), &grpc.StreamDesc{}, "method"); !errors.Is(err, errUnavailable) {
			t.Fatalf("NewStream() error mismatch:\n Got: %v\nWant: %v", err, errUnavailable)
		}
		if pool.newStreamCount != 1 {
			t.Fatalf("underlying NewStream count mismatch:\n Got: %d\nWant: 1", pool.newStreamCount)
		}
		if e.penaltyExpiry.Load() == 0 {
			t.Fatal("NewStream() creation failure did not apply error penalty")
		}
		if got, want := e.penaltyLoad.Load(), int32(5); got != want {
			t.Fatalf("NewStream() creation penalty mismatch:\n Got: %d\nWant: exactly one step %d", got, want)
		}
	})

	t.Run("spannerClient unary", func(t *testing.T) {
		p := newPenaltyPool()
		e := &dcpEntry{parent: p}
		delegate := &mockSpannerClient{executeSQLErr: errUnavailable}
		client := &dcpSpannerClient{entry: e, delegate: delegate}
		if _, err := client.ExecuteSql(context.Background(), &spannerpb.ExecuteSqlRequest{}); !errors.Is(err, errUnavailable) {
			t.Fatalf("ExecuteSql() error mismatch:\n Got: %v\nWant: %v", err, errUnavailable)
		}
		if delegate.executeSQLCount != 1 {
			t.Fatalf("underlying ExecuteSql count mismatch:\n Got: %d\nWant: 1", delegate.executeSQLCount)
		}
		if e.penaltyExpiry.Load() == 0 {
			t.Fatal("unary call did not apply error penalty")
		}
		if got, want := e.penaltyLoad.Load(), int32(5); got != want {
			t.Fatalf("unary call penalty mismatch:\n Got: %d\nWant: exactly one step %d", got, want)
		}
	})

	t.Run("spannerClient stream", func(t *testing.T) {
		p := newPenaltyPool()
		e := &dcpEntry{parent: p}
		client := &dcpSpannerClient{entry: e}
		ref := client.startStream(context.Background())
		ref.done(errUnavailable)
		expiry := e.penaltyExpiry.Load()
		if expiry == 0 {
			t.Fatal("stream call did not apply error penalty")
		}
		time.Sleep(time.Millisecond)
		ref.done(errUnavailable)
		if got := e.penaltyExpiry.Load(); got != expiry {
			t.Fatalf("stream call applied penalty more than once: first %d, then %d", expiry, got)
		}
		if got, want := e.penaltyLoad.Load(), int32(5); got != want {
			t.Fatalf("stream call penalty mismatch:\n Got: %d\nWant: exactly one step %d", got, want)
		}
	})
}

func TestDCPErrorPenaltyAffectsScaleUp(t *testing.T) {
	tests := []struct {
		name       string
		maxRPC     float64
		loads      []int32
		penalties  []int32
		selected   int
		wantSignal bool
	}{
		{name: "selected entry at cap", maxRPC: 25, loads: []int32{5}, penalties: []int32{25}, wantSignal: true},
		{name: "selected entry at one step", maxRPC: 25, loads: []int32{5}, penalties: []int32{5}},
		{name: "two entries at one step", maxRPC: 20, loads: []int32{10, 10}, penalties: []int32{5, 0}},
		{name: "penalty-inclusive average at threshold", maxRPC: 20, loads: []int32{10, 10}, penalties: []int32{20, 0}, selected: 1},
		{name: "penalty-inclusive average", maxRPC: 20, loads: []int32{10, 10}, penalties: []int32{25, 0}, selected: 1, wantSignal: true},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			p := &dynamicChannelPool{
				cfg:           DynamicChannelPoolConfig{DCPMaxRPCPerChannel: test.maxRPC},
				scaleUpSignal: make(chan struct{}, 1),
			}
			entries := make([]*dcpEntry, len(test.loads))
			for i := range entries {
				entries[i] = &dcpEntry{id: uint64(i + 1), parent: p}
				entries[i].unaryLoad.Store(test.loads[i])
				entries[i].penaltyLoad.Store(test.penalties[i])
				if test.penalties[i] > 0 {
					entries[i].penaltyExpiry.Store(time.Now().Add(time.Hour).UnixNano())
				}
			}
			p.entries.Store(&entries)
			for _, load := range test.loads {
				p.totalRPCLoad.Add(load)
			}
			for _, penalty := range test.penalties {
				p.totalPenaltyLoad.Add(int64(penalty))
			}
			p.maybeSignalScaleUp(entries[test.selected])
			select {
			case <-p.scaleUpSignal:
				if !test.wantSignal {
					t.Fatal("maybeSignalScaleUp() signaled, want no signal")
				}
			default:
				if test.wantSignal {
					t.Fatal("maybeSignalScaleUp() did not signal")
				}
			}
		})
	}
}

func TestDCPErrorPenaltyIncludedInScaleUpSizing(t *testing.T) {
	cfg := testDCPConfig(2, 1, 4)
	cfg.DCPMaxRPCPerChannel = 25
	cfg.DCPMinRPCPerChannel = 15
	cfg.DCPErrorPenaltyStep = 5
	cfg.DCPErrorPenaltyDuration = time.Hour
	cfg.DCPScaleDownCheckInterval = time.Hour
	cfg.DCPScaleUpCooldown = time.Hour
	_, client, teardown := setupDCPMockedTestServer(t, cfg)
	defer teardown()
	p := client.sc.dynamicPool
	waitForDCPScaleUpWorkerIdle(p)
	entries := p.getEntries()
	entries[0].unaryLoad.Store(10)
	entries[1].unaryLoad.Store(10)
	entries[0].penaltyLoad.Store(25)
	entries[0].penaltyExpiry.Store(time.Now().Add(time.Hour).UnixNano())
	p.totalPenaltyLoad.Store(25)

	p.scaleUp()
	if got, want := p.Num(), 3; got != want {
		t.Fatalf("DCP channel count after penalty-inclusive scale-up mismatch:\n Got: %d\nWant: %d", got, want)
	}
}

func TestDCPErrorPenaltyDoesNotAffectScaleDownOrDrain(t *testing.T) {
	cfg := testDCPConfig(3, 2, 3)
	cfg.DCPErrorPenaltyStep = 5
	cfg.DCPErrorPenaltyDuration = time.Hour
	cfg.DCPMaxRPCPerChannel = 10
	cfg.DCPMinRPCPerChannel = 9
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	p := &dynamicChannelPool{
		cfg:                 cfg,
		ctx:                 ctx,
		targetRPCPerChannel: 10,
		penaltyMax:          10,
		scaleUpSignal:       make(chan struct{}, 1),
	}
	penalized := &dcpEntry{id: 1, parent: p, pool: &fakeDCPConnPool{}}
	middle := &dcpEntry{id: 2, parent: p, pool: &fakeDCPConnPool{}}
	busy := &dcpEntry{id: 3, parent: p, pool: &fakeDCPConnPool{}}
	now := time.Now().UnixNano()
	penalized.createdAt.Store(now - 3)
	middle.createdAt.Store(now - 2)
	busy.createdAt.Store(now - 1)
	middle.unaryLoad.Store(1)
	busy.unaryLoad.Store(2)
	errUnavailable := status.Error(codes.Unavailable, "unavailable")
	for range 5 {
		penalized.applyErrorPenalty(errUnavailable)
	}
	entries := []*dcpEntry{penalized, middle, busy}
	p.entries.Store(&entries)

	p.removeEntries(1)
	got := p.getEntries()
	if len(got) != 2 {
		t.Fatalf("scale-down kept entry count mismatch:\n Got: %d\nWant: 2", len(got))
	}
	if got[0] != middle || got[1] != busy {
		t.Fatalf("scale-down kept entries mismatch:\n Got: %v\nWant: unpenalized entries 2 and 3", []uint64{got[0].id, got[1].id})
	}
	if got := p.totalPenaltyLoad.Load(); got != 0 {
		t.Fatalf("total penalty after removing penalized entry mismatch:\n Got: %d\nWant: 0", got)
	}
	waitFor(t, func() error {
		if got := penalized.state.Load(); got != dcpStateClosed {
			return fmt.Errorf("penalized drained entry state = %d, want closed", got)
		}
		return nil
	})
}

func TestDCPErrorPenaltyCapFollowsMaxRPCFromDefaultConfig(t *testing.T) {
	cfg := DefaultDynamicChannelPoolConfig()
	cfg.DCPEnabled = true
	cfg.DCPMaxRPCPerChannel = 40
	_, client, teardown := setupDCPMockedTestServer(t, cfg)
	defer teardown()
	p := client.sc.dynamicPool
	if got, want := p.penaltyMax, int32(40); got != want {
		t.Fatalf("penalty max mismatch:\n Got: %d\nWant: ceil(DCPMaxRPCPerChannel) %d", got, want)
	}
	e := p.getEntries()[0]
	errUnavailable := status.Error(codes.Unavailable, "unavailable")
	for range 10 {
		e.applyErrorPenalty(errUnavailable)
	}
	if got, want := e.currentPenalty(), int32(40); got != want {
		t.Fatalf("accumulated penalty mismatch:\n Got: %d\nWant: derived cap %d", got, want)
	}
}

func TestDynamicChannelPoolErrorPenaltyConfigNormalization(t *testing.T) {
	t.Run("zero step clamps to derived cap", func(t *testing.T) {
		cfg, err := normalizeDCPConfig(DynamicChannelPoolConfig{
			DCPMaxRPCPerChannel: 3,
			DCPMinRPCPerChannel: 2,
		})
		if err != nil {
			t.Fatalf("normalizeDCPConfig() failed: %v", err)
		}
		if got, want := cfg.DCPErrorPenaltyStep, int32(3); got != want {
			t.Fatalf("DCPErrorPenaltyStep mismatch:\n Got: %d\nWant: derived cap %d", got, want)
		}
		p := &dynamicChannelPool{cfg: cfg, penaltyMax: 3}
		e := &dcpEntry{parent: p}
		errUnavailable := status.Error(codes.Unavailable, "unavailable")
		e.applyErrorPenalty(errUnavailable)
		if got, want := e.currentPenalty(), int32(3); got != want {
			t.Fatalf("penalty after one error mismatch:\n Got: %d\nWant: cap %d", got, want)
		}
		e.applyErrorPenalty(errUnavailable)
		if got, want := e.currentPenalty(), int32(3); got != want {
			t.Fatalf("penalty after second error mismatch:\n Got: %d\nWant: capped at %d", got, want)
		}
	})

	t.Run("zero uses defaults", func(t *testing.T) {
		cfg, err := normalizeDCPConfig(DynamicChannelPoolConfig{DCPMaxRPCPerChannel: 25.1})
		if err != nil {
			t.Fatalf("normalizeDCPConfig() failed: %v", err)
		}
		if got, want := cfg.DCPErrorPenaltyStep, int32(5); got != want {
			t.Fatalf("DCPErrorPenaltyStep mismatch:\n Got: %d\nWant: %d", got, want)
		}
		if got, want := cfg.DCPErrorPenaltyDuration, 5*time.Second; got != want {
			t.Fatalf("DCPErrorPenaltyDuration mismatch:\n Got: %v\nWant: %v", got, want)
		}
	})

	t.Run("negative duration disables", func(t *testing.T) {
		cfg, err := normalizeDCPConfig(DynamicChannelPoolConfig{DCPErrorPenaltyDuration: -time.Second})
		if err != nil {
			t.Fatalf("normalizeDCPConfig() failed: %v", err)
		}
		if got, want := cfg.DCPErrorPenaltyDuration, -time.Second; got != want {
			t.Fatalf("DCPErrorPenaltyDuration mismatch:\n Got: %v\nWant: %v", got, want)
		}
	})

	t.Run("negative step rejected", func(t *testing.T) {
		_, err := normalizeDCPConfig(DynamicChannelPoolConfig{DCPErrorPenaltyStep: -1})
		if err == nil {
			t.Fatal("normalizeDCPConfig() succeeded, want negative-step error")
		}
	})

	t.Run("step above derived cap rejected", func(t *testing.T) {
		_, err := normalizeDCPConfig(DynamicChannelPoolConfig{
			DCPMaxRPCPerChannel: 5,
			DCPMinRPCPerChannel: 1,
			DCPErrorPenaltyStep: 6,
		})
		if err == nil {
			t.Fatal("normalizeDCPConfig() succeeded, want step-above-derived-cap error")
		}
		if got, want := err.Error(), "DCPErrorPenaltyStep must be <= ceil(DCPMaxRPCPerChannel)"; got != want {
			t.Fatalf("normalizeDCPConfig() error mismatch:\n Got: %q\nWant: %q", got, want)
		}
	})
}

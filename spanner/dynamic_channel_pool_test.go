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
	"cloud.google.com/go/spanner/internal/transport"
	"github.com/googleapis/gax-go/v2"
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
	"google.golang.org/grpc/connectivity"
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
	disableDCPDrainIdleFloor(t)
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

// disableDCPDrainIdleFloor lets pools created by the test close drained
// entries after just DCPDrainIdleGrace, so short test graces keep working.
func disableDCPDrainIdleFloor(t *testing.T) {
	t.Helper()
	floor := transport.DCPDrainIdleFloor
	transport.DCPDrainIdleFloor = 0
	t.Cleanup(func() { transport.DCPDrainIdleFloor = floor })
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
		if got := client.sc.dynamicPool.DrainingCount.Load(); got == 0 {
			return fmt.Errorf("DCP draining channel count = %d, want > 0 during drain grace", got)
		}
		return nil
	})
	waitFor(t, func() error {
		if got := client.sc.dynamicPool.DrainingCount.Load(); got != 0 {
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

	p.EvaluateScaleDown()
	if got, want := p.Num(), 3; got != want {
		t.Fatalf("DCP channel count after first low-load check mismatch:\n Got: %d\nWant: %d", got, want)
	}
	p.EvaluateScaleDown()
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
	entries := p.GetEntries()
	for _, e := range entries[:2] {
		e.State.Store(dcpStateDraining)
	}
	for i := 0; i < 20; i++ {
		e, err := p.Pick(context.Background())
		if err != nil {
			t.Fatalf("pick failed: %v", err)
		}
		if e != entries[2] {
			t.Fatalf("picked entry mismatch:\n Got: draining entry %d\nWant: active entry %d", e.ID, entries[2].ID)
		}
	}
}

func TestDynamicChannelPoolRoundRobinSkipsDrainingEntries(t *testing.T) {
	cfg := testDCPConfig(3, 3, 3)
	cfg.DCPSelectionStrategy = DCPRoundRobin
	_, client, teardown := setupDCPMockedTestServer(t, cfg)
	defer teardown()
	p := client.sc.dynamicPool
	entries := p.GetEntries()
	entries[1].State.Store(dcpStateDraining)

	var got []uint64
	for i := 0; i < 4; i++ {
		e, err := p.Pick(context.Background())
		if err != nil {
			t.Fatalf("pick failed: %v", err)
		}
		got = append(got, e.ID)
	}
	for i, id := range got {
		if id == entries[1].ID {
			t.Fatalf("round-robin sequence = %v, picked draining entry %d", got, id)
		}
		if id != entries[0].ID && id != entries[2].ID {
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
	entries := client.sc.dynamicPool.GetEntries()
	entries[0].UnaryLoad.Store(3)
	entries[1].StreamLoad.Store(4)
	client.sc.dynamicPool.TotalRPCLoad.Store(7)

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
	p.PrimeSession.Store(client.sm.multiplexedSession.id)
	p.GetEntries()[0].UnaryLoad.Store(3)
	p.TotalRPCLoad.Store(3)
	p.ScaleUp()
	p.GetEntries()[0].UnaryLoad.Store(0)
	p.TotalRPCLoad.Store(0)
	p.RemoveEntries(1)

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
	if got := newDCPMetrics(p, client.sc, gaugeFailure); got != nil {
		t.Fatalf("newDCPMetrics with gauge registration failure mismatch:\n Got: %+v\nWant: nil", got)
	}

	counterFailure := &failingDCPMeterProvider{meter: &failingDCPMeter{failCounterName: dcpMetricsPrefix + "channel_pool_scaling"}}
	if got := newDCPMetrics(p, client.sc, counterFailure); got != nil {
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
	client.sc.dynamicPool.Metrics = &dcpMetrics{registration: reg}

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
		if got := client.sc.dynamicPool.TotalRPCLoad.Load(); got == 0 {
			return fmt.Errorf("DCP total RPC load = %d, want in-flight workload", got)
		}
		return nil
	})
	client.sc.dynamicPool.ScaleUp()
	if got, want := client.sc.dynamicPool.Num(), 1; got != want {
		t.Fatalf("DCP channel count after failed prime mismatch:\n Got: %d\nWant: %d", got, want)
	}
	for _, e := range client.sc.dynamicPool.GetEntries() {
		if e.State.Load() != dcpStateActive {
			t.Fatalf("active slice contains non-active entry state=%d", e.State.Load())
		}
	}
	if _, err := client.sc.dynamicPool.Pick(context.Background()); err != nil {
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
	entries := p.GetEntries()
	entries[1].UnaryLoad.Store(100)
	entries[2].StreamLoad.Store(100)

	counts := map[uint64]int{}
	for i := 0; i < 2000; i++ {
		e, err := p.Pick(context.Background())
		if err != nil {
			t.Fatalf("pick failed: %v", err)
		}
		counts[e.ID]++
	}
	low := counts[entries[0].ID]
	high := counts[entries[1].ID] + counts[entries[2].ID]
	if low <= high {
		t.Fatalf("least-loaded entry picked %d times, higher-load entries picked %d times; want least-loaded preference", low, high)
	}
}

func TestDynamicChannelPoolCloseClosesActiveAndDrainingEntries(t *testing.T) {
	_, client, teardown := setupDCPMockedTestServer(t, testDCPConfig(3, 3, 3))
	defer teardown()
	p := client.sc.dynamicPool
	entries := append([]*dcpEntry(nil), p.GetEntries()...)
	entries[1].State.Store(dcpStateDraining)
	p.DrainingCount.Add(1)

	client.Close()
	if got := p.Num(); got != 0 {
		t.Fatalf("DCP pool entries after close mismatch:\n Got: %d\nWant: 0", got)
	}
	for _, e := range entries {
		if got := e.State.Load(); got != dcpStateClosed {
			t.Fatalf("entry %d state after close mismatch:\n Got: %d\nWant: closed", e.ID, got)
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
	entries := p.GetEntries()
	for _, e := range entries[:3] {
		e.State.Store(dcpStateDraining)
	}
	entries[3].UnaryLoad.Store(7)

	e, err := p.PickLeastLoaded()
	if err != nil {
		t.Fatalf("pickLeastLoaded failed: %v", err)
	}
	if e != entries[3] {
		t.Fatalf("full-scan fallback entry mismatch:\n Got: entry %d\nWant: only active entry %d", e.ID, entries[3].ID)
	}
	picked, err := p.Pick(context.Background())
	if err != nil {
		t.Fatalf("pick fallback failed: %v", err)
	}
	if picked != entries[3] {
		t.Fatalf("power-of-two fallback entry mismatch:\n Got: entry %d\nWant: only active entry %d", picked.ID, entries[3].ID)
	}
}

func TestDCPResolvingClientKeepsDrainingEntryUntilClosed(t *testing.T) {
	p := &dynamicChannelPool{Cfg: toTransportDCPConfig(testDCPConfig(2, 1, 2))}
	entry1 := &dcpEntry{ID: 1, Client: &mockSpannerClient{}, Parent: p}
	entry2 := &dcpEntry{ID: 2, Client: &mockSpannerClient{}, Parent: p}
	entries := []*dcpEntry{entry1, entry2}
	p.Entries.Store(&entries)

	resolver := newDCPResolvingSpannerClient(p, entry1)
	drainDCPEntryForTest(t, p, entry1)

	client, err := resolver.resolve(context.Background())
	if err != nil {
		t.Fatalf("resolve while draining failed: %v", err)
	}
	if client != entry1.Client {
		t.Fatalf("resolved client while draining mismatch:\n Got: %p\nWant: draining entry1 client %p", client, entry1.Client)
	}

	entry1.CloseNow()
	client, err = resolver.resolve(context.Background())
	if err != nil {
		t.Fatalf("resolve after close failed: %v", err)
	}
	if client != entry2.Client {
		t.Fatalf("resolved client after close mismatch:\n Got: %p\nWant: entry2 client %p", client, entry2.Client)
	}
	if got, want := resolver.Entry.Load(), entry2; got != want {
		t.Fatalf("resolver entry mismatch:\n Got: entry %d\nWant: entry %d", got.ID, want.ID)
	}
}

type countingDCPConnPool struct {
	fakeDCPConnPool
	mu     sync.Mutex
	closes int
}

func (f *countingDCPConnPool) Close() error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.closes++
	return nil
}

func (f *countingDCPConnPool) closeCount() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.closes
}

func newDCPRefTestPool(t *testing.T, n int) (*dynamicChannelPool, []*dcpEntry) {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	p := &dynamicChannelPool{Cfg: toTransportDCPConfig(testDCPConfig(n, 1, n)), Ctx: ctx}
	p.Cfg.DCPSelectionStrategy = transport.DCPRoundRobin
	entries := make([]*dcpEntry, n)
	for i := range entries {
		entries[i] = &dcpEntry{ID: uint64(i + 1), Pool: &countingDCPConnPool{}, Client: &mockSpannerClient{}, Parent: p}
	}
	active := append([]*dcpEntry(nil), entries...)
	p.Entries.Store(&active)
	return p, entries
}

func dcpCloses(e *dcpEntry) int { return e.Pool.(*countingDCPConnPool).closeCount() }

// A reference taken by the resolver blocks drain close for as long as it is
// held, however long that is, and independent of the idle grace.
func TestDCPReferenceBlocksDrainClose(t *testing.T) {
	p, entries := newDCPRefTestPool(t, 2)
	bound := entries[0]
	resolver := newDCPResolvingSpannerClient(p, bound)
	drainDCPEntryForTest(t, p, bound)
	bound.LastActivity.Store(time.Now().Add(-time.Hour).UnixNano())

	e, err := resolver.Acquire(context.Background())
	if err != nil || e != bound {
		t.Fatalf("acquire = entry %v, %v; want draining bound entry", e, err)
	}
	if bound.CloseIfIdle(0) {
		t.Fatal("drain closed an entry an RPC holds a reference to")
	}
	if got := dcpCloses(bound); got != 0 {
		t.Fatalf("conn closes while referenced = %d, want 0", got)
	}
	if got := bound.State.Load(); got != dcpStateDraining {
		t.Fatalf("state while referenced = %d, want draining", got)
	}
	e.Release()
	if !bound.CloseIfIdle(0) {
		t.Fatal("drain did not close an idle unreferenced entry")
	}
	if got := dcpCloses(bound); got != 1 {
		t.Fatalf("conn closes = %d, want 1", got)
	}
	// Closed is final: a later acquire re-picks instead of reviving it.
	e, err = resolver.Acquire(context.Background())
	if err != nil || e != entries[1] {
		t.Fatalf("acquire after close = entry %v, %v; want active entry 2", e, err)
	}
	e.Release()
}

// The real drain worker must go through the reference gate.
func TestDCPDrainWorkerWaitsForReference(t *testing.T) {
	p, entries := newDCPRefTestPool(t, 2)
	bound := entries[0]
	removeDCPEntryForTest(p, bound)
	bound.LastActivity.Store(time.Now().Add(-time.Hour).UnixNano())
	if !bound.Acquire() {
		t.Fatal("acquire on draining entry failed")
	}
	done := make(chan struct{})
	go func() { p.WaitForDrainAndClose(bound); close(done) }()
	select {
	case <-done:
		t.Fatal("drain worker closed an entry an RPC holds a reference to")
	case <-time.After(700 * time.Millisecond): // more than two worker ticks
	}
	bound.Release()
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("drain worker did not close after the reference was released")
	}
	if got := dcpCloses(bound); got != 1 {
		t.Fatalf("conn closes = %d, want 1", got)
	}
}

// A draining entry stays open past a short grace until it has been idle for the
// floor, so a read-write transaction keeps its channel between statements.
func TestDCPDrainIdleFloorKeepsDrainingEntryOpen(t *testing.T) {
	p, entries := newDCPRefTestPool(t, 2)
	p.Cfg.DCPDrainIdleGrace = time.Second
	p.DrainIdleFloor = 15 * time.Second
	bound := entries[0]
	removeDCPEntryForTest(p, bound)
	bound.LastActivity.Store(time.Now().Add(-2 * time.Second).UnixNano())
	done := make(chan struct{})
	go func() { p.WaitForDrainAndClose(bound); close(done) }()
	select {
	case <-done:
		t.Fatal("drain closed an entry idle longer than the grace but shorter than the floor")
	case <-time.After(700 * time.Millisecond): // more than two worker ticks
	}
	bound.LastActivity.Store(time.Now().Add(-16 * time.Second).UnixNano())
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("drain did not close an entry idle longer than the floor")
	}
	if got := dcpCloses(bound); got != 1 {
		t.Fatalf("conn closes = %d, want 1", got)
	}
}

// Shutdown closes immediately even when referenced, exactly once, and drain
// cannot undo or repeat it.
func TestDCPShutdownCloseWinsOverReferences(t *testing.T) {
	p, entries := newDCPRefTestPool(t, 2)
	bound := entries[0]
	drainDCPEntryForTest(t, p, bound)
	if !bound.Acquire() {
		t.Fatal("acquire on draining entry failed")
	}
	var wg sync.WaitGroup
	for i := 0; i < 8; i++ {
		wg.Add(2)
		go func() { defer wg.Done(); bound.CloseNow() }()
		go func() { defer wg.Done(); bound.CloseIfIdle(0) }()
	}
	wg.Wait()
	if got := dcpCloses(bound); got != 1 {
		t.Fatalf("conn closes = %d, want exactly 1", got)
	}
	bound.Release()
	if bound.Acquire() {
		t.Fatal("acquire succeeded on a closed entry")
	}
	if !bound.CloseIfIdle(0) {
		t.Fatal("closeIfIdle does not report a shut down entry as closed")
	}
	if got, want := dcpCloses(bound), 1; got != want {
		t.Fatalf("conn closes after late calls = %d, want %d", got, want)
	}
	if got := bound.State.Load(); got != dcpStateClosed {
		t.Fatalf("state = %d, want closed", got)
	}
}

// A stream keeps its entry open after the call that started it returned.
func TestDCPStreamHoldsReferenceUntilFinished(t *testing.T) {
	p, entries := newDCPRefTestPool(t, 2)
	bound := entries[0]
	drainDCPEntryForTest(t, p, bound)
	client := &dcpSpannerClient{entry: bound}
	ref := client.startStream(context.Background())
	// The drain worker passed its idle check just before the stream started.
	if bound.CloseIfUnreferenced() {
		t.Fatal("drain closed an entry with an open stream")
	}
	ref.Done(nil)
	ref.Done(nil)
	if got := bound.Refs.Load(); got != 0 {
		t.Fatalf("references after stream finish = %d, want 0", got)
	}
	if !bound.CloseIfUnreferenced() {
		t.Fatal("drain did not close after the stream finished")
	}
}

// Concurrent RPCs of one transaction agree on the replacement entry.
func TestDCPConcurrentRebindAgreesOnOneEntry(t *testing.T) {
	for i := 0; i < 200; i++ {
		p, entries := newDCPRefTestPool(t, 5)
		bound := entries[0]
		resolver := newDCPResolvingSpannerClient(p, bound)
		drainDCPEntryForTest(t, p, bound)
		bound.CloseNow()
		const n = 16
		got := make([]*dcpEntry, n)
		start := make(chan struct{})
		var wg sync.WaitGroup
		for g := 0; g < n; g++ {
			wg.Add(1)
			go func() {
				defer wg.Done()
				<-start
				e, err := resolver.Acquire(context.Background())
				if err != nil {
					t.Error(err)
					return
				}
				got[g] = e
			}()
		}
		close(start)
		wg.Wait()
		for g := 1; g < n; g++ {
			if got[g] != got[0] {
				t.Fatalf("iteration %d: concurrent RPCs used entries %d and %d", i, got[0].ID, got[g].ID)
			}
		}
		if got[0] == bound || got[0] != resolver.Entry.Load() {
			t.Fatalf("iteration %d: rebound entry mismatch", i)
		}
	}
}

// dcpGapClient pauses an RPC after the resolver acquired its entry and before
// the entry client counts it as load.
type dcpGapClient struct {
	spannerClient
	entered, resume chan struct{}
	once            sync.Once
}

func (c *dcpGapClient) pause() {
	c.once.Do(func() { close(c.entered) })
	<-c.resume
}

func (c *dcpGapClient) Commit(ctx context.Context, req *spannerpb.CommitRequest, opts ...gax.CallOption) (*spannerpb.CommitResponse, error) {
	c.pause()
	return c.spannerClient.Commit(ctx, req, opts...)
}

func (c *dcpGapClient) ExecuteStreamingSql(ctx context.Context, req *spannerpb.ExecuteSqlRequest, opts ...gax.CallOption) (spannerpb.Spanner_ExecuteStreamingSqlClient, error) {
	c.pause()
	return c.spannerClient.ExecuteStreamingSql(ctx, req, opts...)
}

// An RPC of a read-write transaction that is delayed between resolving its
// draining channel and counting as load must still run on that channel: the
// real drain worker has to leave the channel open until the RPC is done.
func TestDCPReadWriteTransactionSurvivesResolveToLoadGap(t *testing.T) {
	for _, streaming := range []bool{false, true} {
		name := "unary commit"
		if streaming {
			name = "streaming query"
		}
		t.Run(name, func(t *testing.T) {
			cfg := testDCPConfig(2, 1, 2)
			cfg.DCPScaleDownCheckInterval = time.Hour
			_, client, teardown := setupDCPMockedTestServer(t, cfg)
			defer teardown()
			p := client.sc.dynamicPool
			gap := &dcpGapClient{entered: make(chan struct{}), resume: make(chan struct{})}
			var resumeOnce sync.Once
			resume := func() { resumeOnce.Do(func() { close(gap.resume) }) }
			defer resume()
			ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
			defer cancel()

			boundCh := make(chan *dcpEntry, 1)
			result := make(chan error, 1)
			go func() {
				_, err := client.ReadWriteTransaction(ctx, func(ctx context.Context, tx *ReadWriteTransaction) error {
					if _, err := tx.Update(ctx, NewStatement(UpdateBarSetFoo)); err != nil {
						return err
					}
					bound := tx.txReadOnly.sh.getClient().(*dcpResolvingSpannerClient).Entry.Load()
					removeDCPEntryForTest(p, bound)
					gap.spannerClient = bound.Client
					bound.Client = gap
					boundCh <- bound
					if streaming {
						// Call the session handle client directly: the gap client is
						// not a request-id header provider, which tx.Query needs.
						sh := tx.txReadOnly.sh
						stream, err := sh.getClient().ExecuteStreamingSql(contextWithOutgoingMetadata(ctx, sh.getMetadata(), false), &spannerpb.ExecuteSqlRequest{
							Session:     sh.getID(),
							Transaction: tx.getTransactionSelector(),
							Sql:         "SELECT 1",
						})
						if err != nil {
							return err
						}
						for {
							if _, err := stream.Recv(); err == io.EOF {
								return nil
							} else if err != nil {
								return err
							}
						}
					}
					return nil
				})
				result <- err
			}()
			var bound *dcpEntry
			select {
			case bound = <-boundCh:
			case err := <-result:
				t.Fatalf("transaction ended before the gap: %v", err)
			}
			select {
			case <-gap.entered:
			case err := <-result:
				t.Fatalf("transaction ended before the gap: %v", err)
			}
			if got := bound.RPCLoad(); got != 0 {
				t.Fatalf("load in the gap = %d, want 0", got)
			}
			// Far longer than the 10ms grace: several real drain worker ticks.
			drained := make(chan struct{})
			go func() { p.WaitForDrainAndClose(bound); close(drained) }()
			select {
			case <-drained:
				t.Fatalf("drain closed the channel inside the resolve/load gap: connection=%s", bound.Pool.Conn().GetState())
			case <-time.After(700 * time.Millisecond):
			}
			if got := bound.Pool.Conn().GetState(); got == connectivity.Shutdown {
				t.Fatal("connection shut down inside the resolve/load gap")
			}
			resume()
			if err := <-result; err != nil {
				t.Fatalf("ReadWriteTransaction failed after resolve/load gap: %v", err)
			}
			// No leak: the worker closes the channel once the transaction is done.
			select {
			case <-drained:
			case <-time.After(10 * time.Second):
				t.Fatal("drain never closed the channel after the transaction finished")
			}
			if got := bound.Pool.Conn().GetState(); got != connectivity.Shutdown {
				t.Fatalf("connection after drain = %s, want SHUTDOWN", got)
			}
			if got := p.DrainingCount.Load(); got != 0 {
				t.Fatalf("drainingCount = %d, want 0", got)
			}
		})
	}
}

// dcpChannelRecorder records the DCP channel id, taken from the request-id
// header, of every unary RPC sent for the given methods.
type dcpChannelRecorder struct {
	methods map[string]bool

	mu    sync.Mutex
	calls []dcpRecordedCall
}

type dcpRecordedCall struct {
	method    string
	channelID uint64
}

func (r *dcpChannelRecorder) unaryInterceptor(ctx context.Context, method string, req, reply any, cc *grpc.ClientConn, invoker grpc.UnaryInvoker, opts ...grpc.CallOption) error {
	if r.methods[method] {
		reqID, err := checkForMissingSpannerRequestIDHeader(opts)
		if err != nil {
			return err
		}
		r.mu.Lock()
		r.calls = append(r.calls, dcpRecordedCall{method: method, channelID: uint64(reqID.ChannelID)})
		r.mu.Unlock()
	}
	return invoker(ctx, method, req, reply, cc, opts...)
}

func (r *dcpChannelRecorder) recorded() []dcpRecordedCall {
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]dcpRecordedCall(nil), r.calls...)
}

// drainDCPEntryForTest moves a specific entry to draining the way
// removeEntries does on scale-down, without starting the drain-and-close loop.
// The pool no longer owns the entry, so it is closed at test cleanup.
func drainDCPEntryForTest(t *testing.T, p *dynamicChannelPool, target *dcpEntry) {
	t.Helper()
	removeDCPEntryForTest(p, target)
	t.Cleanup(func() {
		target.CloseNow()
		p.DrainingCount.Add(-1)
	})
}

// removeDCPEntryForTest does what removeEntries does to an entry before it
// starts waitForDrainAndClose, which then owns closing it.
func removeDCPEntryForTest(p *dynamicChannelPool, target *dcpEntry) {
	p.DialMu.Lock()
	defer p.DialMu.Unlock()
	target.State.Store(dcpStateDraining)
	target.ClearErrorPenalty()
	var keep []*dcpEntry
	for _, e := range p.GetEntries() {
		if e != target {
			keep = append(keep, e)
		}
	}
	p.Entries.Store(&keep)
	p.DrainingCount.Add(1)
}

func TestDCPReadWriteTransactionChannelAffinityAcrossDrain(t *testing.T) {
	const (
		executeSQL = "/google.spanner.v1.Spanner/ExecuteSql"
		commit     = "/google.spanner.v1.Spanner/Commit"
	)
	tests := []struct {
		name string
		// closeBound closes the bound channel after draining it, as
		// waitForDrainAndClose eventually does.
		closeBound bool
		// wantSameChannel reports whether the second and third operations must
		// use the channel of the first operation.
		wantSameChannel bool
	}{
		{name: "draining channel keeps serving transaction", closeBound: false, wantSameChannel: true},
		{name: "closed channel moves transaction to active channel", closeBound: true, wantSameChannel: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			recorder := &dcpChannelRecorder{methods: map[string]bool{executeSQL: true, commit: true}}
			cfg := testDCPConfig(2, 1, 2)
			// Disable background scale-down so only the test changes entry state.
			cfg.DCPScaleDownCheckInterval = time.Hour
			server, client, teardown := setupMockedTestServerWithConfigAndClientOptions(t, ClientConfig{
				DisableNativeMetrics:     true,
				DynamicChannelPoolConfig: cfg,
			}, []option.ClientOption{
				option.WithGRPCDialOption(grpc.WithUnaryInterceptor(recorder.unaryInterceptor)),
			})
			defer teardown()
			addSelect1Result(server)
			p := client.sc.dynamicPool
			if p == nil {
				t.Fatal("dynamic channel pool not enabled")
			}
			if got, want := p.Num(), 2; got != want {
				t.Fatalf("DCP channel count mismatch:\n Got: %d\nWant: %d", got, want)
			}

			var bound *dcpEntry
			_, err := client.ReadWriteTransaction(context.Background(), func(ctx context.Context, tx *ReadWriteTransaction) error {
				if _, err := tx.Update(ctx, NewStatement(UpdateBarSetFoo)); err != nil {
					return err
				}
				if bound == nil {
					calls := recorder.recorded()
					if len(calls) != 1 {
						return fmt.Errorf("recorded calls after first operation = %+v, want exactly one", calls)
					}
					for _, e := range p.GetEntries() {
						if e.ID == calls[0].channelID {
							bound = e
						}
					}
					if bound == nil {
						return fmt.Errorf("first operation channel %d is not an active DCP entry", calls[0].channelID)
					}
					drainDCPEntryForTest(t, p, bound)
					if tt.closeBound {
						bound.CloseNow()
					}
				}
				_, err := tx.Update(ctx, NewStatement(UpdateBarSetFoo))
				return err
			})
			if err != nil {
				t.Fatalf("ReadWriteTransaction failed: %v", err)
			}

			calls := recorder.recorded()
			if got, want := len(calls), 3; got != want {
				t.Fatalf("recorded call count mismatch:\n Got: %d (%+v)\nWant: %d", got, calls, want)
			}
			for i, want := range []string{executeSQL, executeSQL, commit} {
				if calls[i].method != want {
					t.Fatalf("operation %d method mismatch:\n Got: %s\nWant: %s", i+1, calls[i].method, want)
				}
			}
			first := calls[0].channelID
			for i, c := range calls[1:] {
				if tt.wantSameChannel && c.channelID != first {
					t.Errorf("operation %d (%s) channel mismatch after draining:\n Got: channel %d\nWant: draining bound channel %d\nAll operations: %+v", i+2, c.method, c.channelID, first, calls)
				}
				if !tt.wantSameChannel && c.channelID == first {
					t.Errorf("operation %d (%s) channel mismatch after close:\n Got: closed bound channel %d\nWant: a different active channel\nAll operations: %+v", i+2, c.method, c.channelID, calls)
				}
			}
		})
	}
}

func TestDCPResolvingRequestIDReturnsErrorWhenNoEntry(t *testing.T) {
	p := &dynamicChannelPool{Cfg: toTransportDCPConfig(testDCPConfig(1, 1, 1))}
	entries := []*dcpEntry{}
	p.Entries.Store(&entries)
	resolver := newDCPResolvingSpannerClient(p, nil)

	if _, err := resolver.requestIDHeaderInjector(context.Background()); err == nil {
		t.Fatal("requestIDHeaderInjector succeeded, want error")
	}
}

func TestDynamicChannelPoolDrainWaitsForActiveStreamLoad(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	cfg := testDCPConfig(1, 1, 1)
	cfg.DCPDrainIdleGrace = 10 * time.Millisecond
	p := &dynamicChannelPool{Cfg: toTransportDCPConfig(cfg), Ctx: ctx}
	entry := &dcpEntry{ID: 1, Pool: &fakeDCPConnPool{}, Client: &mockSpannerClient{}, Parent: p}
	entry.State.Store(dcpStateDraining)
	entry.StreamLoad.Store(1)
	entry.LastActivity.Store(time.Now().Add(-time.Second).UnixNano())
	p.DrainingCount.Store(1)

	done := make(chan struct{})
	go func() {
		p.WaitForDrainAndClose(entry)
		close(done)
	}()

	select {
	case <-done:
		t.Fatal("drain closed entry with active stream load")
	case <-time.After(50 * time.Millisecond):
	}
	entry.StreamLoad.Store(0)
	entry.LastActivity.Store(time.Now().Add(-time.Second).UnixNano())
	waitFor(t, func() error {
		select {
		case <-done:
			return nil
		default:
			return fmt.Errorf("drain did not close after stream load reached zero")
		}
	})
	if got := entry.State.Load(); got != dcpStateClosed {
		t.Fatalf("entry state mismatch:\n Got: %d\nWant: closed", got)
	}
}

func TestDCPStreamContextCancelReleasesStreamLoad(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	p := &dynamicChannelPool{Cfg: toTransportDCPConfig(testDCPConfig(1, 1, 1))}
	entry := &dcpEntry{ID: 1, Parent: p}
	client := &dcpSpannerClient{entry: entry}

	_ = client.startStream(ctx)
	if got := entry.StreamLoad.Load(); got != 1 {
		t.Fatalf("stream load after start mismatch:\n Got: %d\nWant: 1", got)
	}
	cancel()
	waitFor(t, func() error {
		if got := entry.StreamLoad.Load(); got != 0 {
			return fmt.Errorf("stream load after context cancel = %d, want 0", got)
		}
		return nil
	})
}

func TestDynamicChannelPoolPowerOfTwoSpreadDoesNotHerd(t *testing.T) {
	_, client, teardown := setupDCPMockedTestServer(t, testDCPConfig(4, 4, 4))
	defer teardown()
	p := client.sc.dynamicPool
	entries := p.GetEntries()
	overloaded := entries[0]
	overloaded.UnaryLoad.Store(200)

	const workers = 400
	start := make(chan struct{})
	picked := make(chan *dcpEntry, workers)
	var wg sync.WaitGroup
	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			e, err := p.Pick(context.Background())
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
		counts[e.ID]++
	}
	if got := counts[overloaded.ID]; got > 60 {
		t.Fatalf("overloaded entry pick count mismatch:\n Got: %d\nWant: <= 60\ncounts: %v", got, counts)
	}
	for _, e := range entries[1:] {
		if got := counts[e.ID]; got < 70 {
			t.Fatalf("entry %d pick count mismatch:\n Got: %d\nWant: spread across low-load entries\ncounts: %v", e.ID, got, counts)
		}
	}
	var maxLow int
	for _, e := range entries[1:] {
		if got := counts[e.ID]; got > maxLow {
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
	for _, e := range p.GetEntries() {
		e.UnaryLoad.Store(10)
	}

	p.ScaleUp()
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
	for _, e := range p.GetEntries() {
		e.UnaryLoad.Store(10)
	}

	p.ScaleUp()
	if got, want := p.Num(), 15; got != want {
		t.Fatalf("DCP channel count after percent-capped scale-up mismatch:\n Got: %d\nWant: %d", got, want)
	}
}

func waitForDCPScaleUpWorkerIdle(p *dynamicChannelPool) {
	for {
		select {
		case <-p.ScaleUpSignal:
			continue
		default:
		}
		break
	}
	p.DialMu.Lock()
	p.DialMu.Unlock()
}

func TestDynamicChannelPoolScaleUpDialFailureDoesNotPublishEntry(t *testing.T) {
	_, client, teardown := setupDCPMockedTestServer(t, testDCPConfig(1, 1, 2))
	defer teardown()
	p := client.sc.dynamicPool
	// This test drives scaleUp synchronously; seed priming without also waking the
	// background worker and racing the explicit call.
	p.PrimeSession.Store(client.sm.multiplexedSession.id)
	p.Dial = func(context.Context) (gtransport.ConnPool, error) {
		return nil, status.Error(codes.Unavailable, "dial failed")
	}
	initialEntries := append([]*dcpEntry(nil), p.GetEntries()...)
	p.GetEntries()[0].UnaryLoad.Store(10)

	p.ScaleUp()
	if got, want := p.Num(), 1; got != want {
		t.Fatalf("DCP channel count after failed dial mismatch:\n Got: %d\nWant: %d", got, want)
	}
	if got := p.GetEntries()[0]; got != initialEntries[0] {
		t.Fatalf("active entry pointer changed after failed dial")
	}
	if got := p.LastScaleUp.Load(); got == 0 {
		t.Fatal("lastScaleUp after failed dial = 0, want cooldown to be consumed")
	}
	for _, e := range p.GetEntries() {
		if e.State.Load() != dcpStateActive {
			t.Fatalf("active slice contains non-active entry state=%d", e.State.Load())
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
			p := &dynamicChannelPool{PenaltyMax: 25, Cfg: toTransportDCPConfig(DynamicChannelPoolConfig{
				DCPErrorPenaltyStep:     5,
				DCPErrorPenaltyDuration: test.duration,
			})}
			e := &dcpEntry{Parent: p}
			before := time.Now().UnixNano()
			e.ApplyErrorPenalty(test.err)
			expiry := e.PenaltyExpiry.Load()
			if test.wantPenalty {
				if expiry <= before {
					t.Fatalf("penalty expiry mismatch:\n Got: %d\nWant: after %d", expiry, before)
				}
				if got, want := e.PenaltyLoad.Load(), int32(5); got != want {
					t.Fatalf("penalty load mismatch:\n Got: %d\nWant: %d", got, want)
				}
				return
			}
			if expiry != 0 {
				t.Fatalf("penalty expiry mismatch:\n Got: %d\nWant: 0", expiry)
			}
			if got := e.PenaltyLoad.Load(); got != 0 {
				t.Fatalf("penalty load mismatch:\n Got: %d\nWant: 0", got)
			}
		})
	}
}

func TestDCPApplyErrorPenaltyAccumulatesCapsAndRestarts(t *testing.T) {
	p := &dynamicChannelPool{PenaltyMax: 15, Cfg: toTransportDCPConfig(DynamicChannelPoolConfig{
		DCPMaxRPCPerChannel:     15,
		DCPErrorPenaltyStep:     5,
		DCPErrorPenaltyDuration: time.Hour,
	})}
	e := &dcpEntry{Parent: p}
	errUnavailable := status.Error(codes.Unavailable, "unavailable")

	e.ApplyErrorPenalty(errUnavailable)
	if got, want := e.PenaltyLoad.Load(), int32(5); got != want {
		t.Fatalf("penalty after one error mismatch:\n Got: %d\nWant: %d", got, want)
	}
	firstExpiry := e.PenaltyExpiry.Load()
	time.Sleep(time.Millisecond)
	e.ApplyErrorPenalty(errUnavailable)
	e.ApplyErrorPenalty(errUnavailable)
	if got, want := e.PenaltyLoad.Load(), int32(15); got != want {
		t.Fatalf("penalty after three errors mismatch:\n Got: %d\nWant: %d", got, want)
	}
	if got := e.PenaltyExpiry.Load(); got <= firstExpiry {
		t.Fatalf("penalty expiry after later error mismatch:\n Got: %d\nWant: after %d", got, firstExpiry)
	}
	e.ApplyErrorPenalty(errUnavailable)
	if got, want := e.PenaltyLoad.Load(), int32(15); got != want {
		t.Fatalf("penalty beyond cap mismatch:\n Got: %d\nWant: %d", got, want)
	}

	e.PenaltyExpiry.Store(time.Now().Add(-time.Second).UnixNano())
	e.ApplyErrorPenalty(errUnavailable)
	if got, want := e.PenaltyLoad.Load(), int32(5); got != want {
		t.Fatalf("penalty after expired window mismatch:\n Got: %d\nWant: fresh step %d", got, want)
	}
}

func TestDCPErrorPenaltyAggregateAppliesCappedDeltas(t *testing.T) {
	p := &dynamicChannelPool{PenaltyMax: 12, Cfg: toTransportDCPConfig(DynamicChannelPoolConfig{
		DCPMaxRPCPerChannel:     12,
		DCPErrorPenaltyStep:     5,
		DCPErrorPenaltyDuration: time.Hour,
	})}
	first := &dcpEntry{Parent: p}
	second := &dcpEntry{Parent: p}
	errUnavailable := status.Error(codes.Unavailable, "unavailable")

	first.ApplyErrorPenalty(errUnavailable)
	second.ApplyErrorPenalty(errUnavailable)
	if got, want := p.TotalPenaltyLoad.Load(), int64(10); got != want {
		t.Fatalf("total penalty after two entries mismatch:\n Got: %d\nWant: %d", got, want)
	}
	first.ApplyErrorPenalty(errUnavailable)
	first.ApplyErrorPenalty(errUnavailable)
	first.ApplyErrorPenalty(errUnavailable)
	if got, want := first.CurrentPenalty(), int32(12); got != want {
		t.Fatalf("first entry penalty at cap mismatch:\n Got: %d\nWant: %d", got, want)
	}
	if got, want := p.TotalPenaltyLoad.Load(), int64(17); got != want {
		t.Fatalf("total penalty after capped increments mismatch:\n Got: %d\nWant: %d", got, want)
	}
}

func TestDCPErrorPenaltyAggregateDoesNotOverflowInt32(t *testing.T) {
	p := &dynamicChannelPool{PenaltyMax: math.MaxInt32, Cfg: toTransportDCPConfig(DynamicChannelPoolConfig{
		DCPErrorPenaltyStep:     math.MaxInt32,
		DCPErrorPenaltyDuration: time.Hour,
	})}
	errUnavailable := status.Error(codes.Unavailable, "unavailable")
	for range 2 {
		e := &dcpEntry{Parent: p}
		e.ApplyErrorPenalty(errUnavailable)
	}
	if got, want := int64(p.TotalPenaltyLoad.Load()), int64(2*math.MaxInt32); got != want {
		t.Fatalf("total penalty mismatch:\n Got: %d\nWant: %d", got, want)
	}
}

func TestDCPErrorPenaltyAggregateExpirySubtractsExactlyOnce(t *testing.T) {
	p := &dynamicChannelPool{PenaltyMax: 25, Cfg: toTransportDCPConfig(DynamicChannelPoolConfig{
		DCPMaxRPCPerChannel:     25,
		DCPErrorPenaltyStep:     5,
		DCPErrorPenaltyDuration: time.Hour,
	})}
	e := &dcpEntry{Parent: p}
	e.ApplyErrorPenalty(status.Error(codes.Unavailable, "unavailable"))
	e.PenaltyExpiry.Store(time.Now().Add(-time.Second).UnixNano())

	const readers = 100
	start := make(chan struct{})
	var wg sync.WaitGroup
	wg.Add(readers)
	for range readers {
		go func() {
			defer wg.Done()
			<-start
			if got := e.CurrentPenalty(); got != 0 {
				t.Errorf("currentPenalty() after expiry mismatch:\n Got: %d\nWant: 0", got)
			}
		}()
	}
	close(start)
	wg.Wait()

	if got := p.TotalPenaltyLoad.Load(); got != 0 {
		t.Fatalf("total penalty after concurrent expiry mismatch:\n Got: %d\nWant: 0", got)
	}
}

func TestDCPCurrentPenaltyAndPickLoadWindow(t *testing.T) {
	p := &dynamicChannelPool{}
	e := &dcpEntry{Parent: p}
	e.UnaryLoad.Store(2)
	e.StreamLoad.Store(3)
	e.PenaltyLoad.Store(10)
	e.PenaltyExpiry.Store(time.Now().Add(time.Second).UnixNano())
	p.TotalPenaltyLoad.Store(10)

	if got, want := e.CurrentPenalty(), int32(10); got != want {
		t.Fatalf("currentPenalty() during window mismatch:\n Got: %d\nWant: %d", got, want)
	}
	if got, want := e.PickLoad(), int32(15); got != want {
		t.Fatalf("pickLoad() during penalty mismatch:\n Got: %d\nWant: %d", got, want)
	}
	if got, want := e.RPCLoad(), int32(5); got != want {
		t.Fatalf("rpcLoad() during penalty mismatch:\n Got: %d\nWant: %d", got, want)
	}
	if got, want := e.WeightedLoad(), int32(5); got != want {
		t.Fatalf("weightedLoad() during penalty mismatch:\n Got: %d\nWant: %d", got, want)
	}

	e.PenaltyExpiry.Store(time.Now().Add(-time.Second).UnixNano())
	if got, want := e.PickLoad(), int32(5); got != want {
		t.Fatalf("pickLoad() after penalty mismatch:\n Got: %d\nWant: %d", got, want)
	}
	if got := e.PenaltyExpiry.Load(); got != 0 {
		t.Fatalf("expired penalty mismatch:\n Got: %d\nWant: CAS reset to 0", got)
	}
	if got := e.CurrentPenalty(); got != 0 {
		t.Fatalf("currentPenalty() after lazy reset mismatch:\n Got: %d\nWant: 0", got)
	}
}

func TestDCPCurrentPenaltyAfterNewWindow(t *testing.T) {
	p := &dynamicChannelPool{PenaltyMax: 25, Cfg: toTransportDCPConfig(DynamicChannelPoolConfig{
		DCPMaxRPCPerChannel:     25,
		DCPErrorPenaltyStep:     5,
		DCPErrorPenaltyDuration: time.Second,
	})}
	e := &dcpEntry{Parent: p}
	e.PenaltyLoad.Store(25)
	e.PenaltyExpiry.Store(time.Now().Add(-time.Second).UnixNano())
	p.TotalPenaltyLoad.Store(25)

	e.ApplyErrorPenalty(status.Error(codes.Unavailable, "unavailable"))
	if got, want := e.CurrentPenalty(), int32(5); got != want {
		t.Fatalf("currentPenalty() after new window mismatch:\n Got: %d\nWant: %d", got, want)
	}
	// A unit test cannot observe the atomic update's transient states, but this
	// pins its net-delta contract: replace the old counted contribution with new.
	if got, want := p.TotalPenaltyLoad.Load(), int64(5); got != want {
		t.Fatalf("total penalty after new window mismatch:\n Got: %d\nWant: net contribution %d", got, want)
	}
}

func TestDynamicChannelPoolPowerOfTwoErrorPenalty(t *testing.T) {
	p := &dynamicChannelPool{PenaltyMax: 25, Cfg: toTransportDCPConfig(DynamicChannelPoolConfig{
		DCPMaxRPCPerChannel:     25,
		DCPErrorPenaltyStep:     5,
		DCPErrorPenaltyDuration: time.Hour,
	})}
	healthy := &dcpEntry{ID: 1, Parent: p}
	penalized := &dcpEntry{ID: 2, Parent: p}
	healthy.UnaryLoad.Store(8)
	penalized.UnaryLoad.Store(2)
	penalized.ApplyErrorPenalty(status.Error(codes.Unavailable, "unavailable"))
	entries := []*dcpEntry{healthy, penalized}
	p.Entries.Store(&entries)

	const picks = 1000
	penalizedPicks := 0
	for range picks {
		e, err := p.PickPowerOfTwo()
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

	healthy.UnaryLoad.Store(0)
	penalized.UnaryLoad.Store(0)
	penalized.PenaltyLoad.Store(25)
	penalized.PenaltyExpiry.Store(time.Now().Add(time.Hour).UnixNano())
	penalizedPicks = 0
	for range picks {
		e, err := p.PickPowerOfTwo()
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
	penalized := &dcpEntry{ID: 1, Parent: p}
	healthy := &dcpEntry{ID: 2, Parent: p}
	penalized.PenaltyLoad.Store(25)
	penalized.PenaltyExpiry.Store(time.Now().Add(time.Hour).UnixNano())
	entries := []*dcpEntry{penalized, healthy}
	p.Entries.Store(&entries)

	got, err := p.PickLeastLoaded()
	if err != nil {
		t.Fatalf("pickLeastLoaded() failed: %v", err)
	}
	if got != healthy {
		t.Fatalf("pickLeastLoaded() mismatch:\n Got: entry %d\nWant: healthy entry %d", got.ID, healthy.ID)
	}

	healthy.PenaltyLoad.Store(25)
	healthy.PenaltyExpiry.Store(time.Now().Add(time.Hour).UnixNano())
	got, err = p.PickLeastLoaded()
	if err != nil {
		t.Fatalf("pickLeastLoaded() with all entries penalized failed: %v", err)
	}
	if got == nil {
		t.Fatal("pickLeastLoaded() with all entries penalized returned nil")
	}
}

func TestDynamicChannelPoolRoundRobinIgnoresErrorPenalty(t *testing.T) {
	p := &dynamicChannelPool{}
	penalized := &dcpEntry{ID: 1, Parent: p}
	healthy := &dcpEntry{ID: 2, Parent: p}
	penalized.PenaltyLoad.Store(25)
	penalized.PenaltyExpiry.Store(time.Now().Add(time.Hour).UnixNano())
	entries := []*dcpEntry{penalized, healthy}
	p.Entries.Store(&entries)

	const picks = 1000
	penalizedPicks := 0
	for range picks {
		e, err := p.PickRoundRobin()
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
		return &dynamicChannelPool{Cfg: toTransportDCPConfig(penaltyConfig), PenaltyMax: 25}
	}
	errUnavailable := status.Error(codes.Unavailable, "unavailable")

	t.Run("ConnPool Invoke", func(t *testing.T) {
		pool := &fakeDCPConnPool{invokeErr: errUnavailable}
		p := newPenaltyPool()
		e := &dcpEntry{ID: 1, Pool: pool, Parent: p}
		entries := []*dcpEntry{e}
		p.Entries.Store(&entries)
		if err := p.Invoke(context.Background(), "method", nil, nil); !errors.Is(err, errUnavailable) {
			t.Fatalf("Invoke() error mismatch:\n Got: %v\nWant: %v", err, errUnavailable)
		}
		if pool.invokeCount != 1 {
			t.Fatalf("underlying Invoke count mismatch:\n Got: %d\nWant: 1", pool.invokeCount)
		}
		if e.PenaltyExpiry.Load() == 0 {
			t.Fatal("Invoke() did not apply error penalty")
		}
		if got, want := e.PenaltyLoad.Load(), int32(5); got != want {
			t.Fatalf("Invoke() penalty mismatch:\n Got: %d\nWant: exactly one step %d", got, want)
		}
	})

	t.Run("ConnPool tracked stream", func(t *testing.T) {
		p := newPenaltyPool()
		e := &dcpEntry{Parent: p}
		e.StreamLoad.Store(1)
		s := &dcpConnPoolTrackedStream{Entry: e}
		s.Finish(errUnavailable)
		expiry := e.PenaltyExpiry.Load()
		if expiry == 0 {
			t.Fatal("tracked stream did not apply error penalty")
		}
		time.Sleep(time.Millisecond)
		s.Finish(errUnavailable)
		if got := e.PenaltyExpiry.Load(); got != expiry {
			t.Fatalf("tracked stream applied penalty more than once: first %d, then %d", expiry, got)
		}
		if got, want := e.PenaltyLoad.Load(), int32(5); got != want {
			t.Fatalf("tracked stream penalty mismatch:\n Got: %d\nWant: exactly one step %d", got, want)
		}
	})

	t.Run("ConnPool stream creation", func(t *testing.T) {
		pool := &fakeDCPConnPool{invokeErr: errUnavailable}
		p := newPenaltyPool()
		e := &dcpEntry{ID: 1, Pool: pool, Parent: p}
		entries := []*dcpEntry{e}
		p.Entries.Store(&entries)
		if _, err := p.NewStream(context.Background(), &grpc.StreamDesc{}, "method"); !errors.Is(err, errUnavailable) {
			t.Fatalf("NewStream() error mismatch:\n Got: %v\nWant: %v", err, errUnavailable)
		}
		if pool.newStreamCount != 1 {
			t.Fatalf("underlying NewStream count mismatch:\n Got: %d\nWant: 1", pool.newStreamCount)
		}
		if e.PenaltyExpiry.Load() == 0 {
			t.Fatal("NewStream() creation failure did not apply error penalty")
		}
		if got, want := e.PenaltyLoad.Load(), int32(5); got != want {
			t.Fatalf("NewStream() creation penalty mismatch:\n Got: %d\nWant: exactly one step %d", got, want)
		}
	})

	t.Run("spannerClient unary", func(t *testing.T) {
		p := newPenaltyPool()
		e := &dcpEntry{Parent: p}
		delegate := &mockSpannerClient{executeSQLErr: errUnavailable}
		client := &dcpSpannerClient{entry: e, delegate: delegate}
		if _, err := client.ExecuteSql(context.Background(), &spannerpb.ExecuteSqlRequest{}); !errors.Is(err, errUnavailable) {
			t.Fatalf("ExecuteSql() error mismatch:\n Got: %v\nWant: %v", err, errUnavailable)
		}
		if delegate.executeSQLCount != 1 {
			t.Fatalf("underlying ExecuteSql count mismatch:\n Got: %d\nWant: 1", delegate.executeSQLCount)
		}
		if e.PenaltyExpiry.Load() == 0 {
			t.Fatal("unary call did not apply error penalty")
		}
		if got, want := e.PenaltyLoad.Load(), int32(5); got != want {
			t.Fatalf("unary call penalty mismatch:\n Got: %d\nWant: exactly one step %d", got, want)
		}
	})

	t.Run("spannerClient stream", func(t *testing.T) {
		p := newPenaltyPool()
		e := &dcpEntry{Parent: p}
		client := &dcpSpannerClient{entry: e}
		ref := client.startStream(context.Background())
		ref.Done(errUnavailable)
		expiry := e.PenaltyExpiry.Load()
		if expiry == 0 {
			t.Fatal("stream call did not apply error penalty")
		}
		time.Sleep(time.Millisecond)
		ref.Done(errUnavailable)
		if got := e.PenaltyExpiry.Load(); got != expiry {
			t.Fatalf("stream call applied penalty more than once: first %d, then %d", expiry, got)
		}
		if got, want := e.PenaltyLoad.Load(), int32(5); got != want {
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
				Cfg:           toTransportDCPConfig(DynamicChannelPoolConfig{DCPMaxRPCPerChannel: test.maxRPC}),
				ScaleUpSignal: make(chan struct{}, 1),
			}
			entries := make([]*dcpEntry, len(test.loads))
			for i := range entries {
				entries[i] = &dcpEntry{ID: uint64(i + 1), Parent: p}
				entries[i].UnaryLoad.Store(test.loads[i])
				entries[i].PenaltyLoad.Store(test.penalties[i])
				if test.penalties[i] > 0 {
					entries[i].PenaltyExpiry.Store(time.Now().Add(time.Hour).UnixNano())
				}
			}
			p.Entries.Store(&entries)
			for _, load := range test.loads {
				p.TotalRPCLoad.Add(load)
			}
			for _, penalty := range test.penalties {
				p.TotalPenaltyLoad.Add(int64(penalty))
			}
			p.MaybeSignalScaleUp(entries[test.selected])
			select {
			case <-p.ScaleUpSignal:
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
	entries := p.GetEntries()
	entries[0].UnaryLoad.Store(10)
	entries[1].UnaryLoad.Store(10)
	entries[0].PenaltyLoad.Store(25)
	entries[0].PenaltyExpiry.Store(time.Now().Add(time.Hour).UnixNano())
	p.TotalPenaltyLoad.Store(25)

	p.ScaleUp()
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
		Cfg:                 toTransportDCPConfig(cfg),
		Ctx:                 ctx,
		TargetRPCPerChannel: 10,
		PenaltyMax:          10,
		ScaleUpSignal:       make(chan struct{}, 1),
	}
	penalized := &dcpEntry{ID: 1, Parent: p, Pool: &fakeDCPConnPool{}}
	middle := &dcpEntry{ID: 2, Parent: p, Pool: &fakeDCPConnPool{}}
	busy := &dcpEntry{ID: 3, Parent: p, Pool: &fakeDCPConnPool{}}
	now := time.Now().UnixNano()
	penalized.CreatedAt.Store(now - 3)
	middle.CreatedAt.Store(now - 2)
	busy.CreatedAt.Store(now - 1)
	middle.UnaryLoad.Store(1)
	busy.UnaryLoad.Store(2)
	errUnavailable := status.Error(codes.Unavailable, "unavailable")
	for range 5 {
		penalized.ApplyErrorPenalty(errUnavailable)
	}
	entries := []*dcpEntry{penalized, middle, busy}
	p.Entries.Store(&entries)

	p.RemoveEntries(1)
	got := p.GetEntries()
	if len(got) != 2 {
		t.Fatalf("scale-down kept entry count mismatch:\n Got: %d\nWant: 2", len(got))
	}
	if got[0] != middle || got[1] != busy {
		t.Fatalf("scale-down kept entries mismatch:\n Got: %v\nWant: unpenalized entries 2 and 3", []uint64{got[0].ID, got[1].ID})
	}
	if got := p.TotalPenaltyLoad.Load(); got != 0 {
		t.Fatalf("total penalty after removing penalized entry mismatch:\n Got: %d\nWant: 0", got)
	}
	waitFor(t, func() error {
		if got := penalized.State.Load(); got != dcpStateClosed {
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
	if got, want := p.PenaltyMax, int32(40); got != want {
		t.Fatalf("penalty max mismatch:\n Got: %d\nWant: ceil(DCPMaxRPCPerChannel) %d", got, want)
	}
	e := p.GetEntries()[0]
	errUnavailable := status.Error(codes.Unavailable, "unavailable")
	for range 10 {
		e.ApplyErrorPenalty(errUnavailable)
	}
	if got, want := e.CurrentPenalty(), int32(40); got != want {
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
		p := &dynamicChannelPool{Cfg: toTransportDCPConfig(cfg), PenaltyMax: 3}
		e := &dcpEntry{Parent: p}
		errUnavailable := status.Error(codes.Unavailable, "unavailable")
		e.ApplyErrorPenalty(errUnavailable)
		if got, want := e.CurrentPenalty(), int32(3); got != want {
			t.Fatalf("penalty after one error mismatch:\n Got: %d\nWant: cap %d", got, want)
		}
		e.ApplyErrorPenalty(errUnavailable)
		if got, want := e.CurrentPenalty(), int32(3); got != want {
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

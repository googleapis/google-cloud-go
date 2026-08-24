/*
Copyright 2026 Google LLC

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package spanner

import (
	"context"
	"errors"
	"testing"
	"time"

	"go.opentelemetry.io/otel/attribute"
	otelmetric "go.opentelemetry.io/otel/metric"
	"go.opentelemetry.io/otel/metric/noop"
	"go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/metric/metricdata"
	"google.golang.org/api/option"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

type testMonitoringExporter struct {
	server             *MetricsTestServer
	createOptionsCalls int
}

func installTestMonitoringExporter(t *testing.T) *testMonitoringExporter {
	t.Helper()
	server, err := NewMetricTestServer()
	if err != nil {
		t.Fatalf("NewMetricTestServer() failed: %v", err)
	}
	go server.Serve()

	testExporter := &testMonitoringExporter{server: server}
	original := createExporterOptions
	createExporterOptions = func(...option.ClientOption) []option.ClientOption {
		testExporter.createOptionsCalls++
		return []option.ClientOption{
			option.WithEndpoint(server.Endpoint),
			option.WithoutAuthentication(),
			option.WithGRPCDialOption(grpc.WithTransportCredentials(insecure.NewCredentials())),
		}
	}
	t.Cleanup(func() {
		createExporterOptions = original
		server.Shutdown()
	})
	return testExporter
}

func newTestMeterProvider() (*metric.ManualReader, *metric.MeterProvider) {
	reader := metric.NewManualReader()
	options := ClientMetricsMeterProviderOptions()
	options = append(options, metric.WithReader(reader))
	return reader, metric.NewMeterProvider(options...)
}

func collectTestMetrics(t *testing.T, reader *metric.ManualReader) metricdata.ResourceMetrics {
	t.Helper()
	var rm metricdata.ResourceMetrics
	if err := reader.Collect(context.Background(), &rm); err != nil {
		t.Fatalf("Collect() failed: %v", err)
	}
	return rm
}

func findTestMetric(rm metricdata.ResourceMetrics, name string) (metricdata.Metrics, bool) {
	for _, sm := range rm.ScopeMetrics {
		for _, m := range sm.Metrics {
			if m.Name == name {
				return m, true
			}
		}
	}
	return metricdata.Metrics{}, false
}

func requireTestMetric(t *testing.T, rm metricdata.ResourceMetrics, name string) metricdata.Metrics {
	t.Helper()
	m, ok := findTestMetric(rm, name)
	if !ok {
		t.Fatalf("metric %q not found in %+v", name, rm.ScopeMetrics)
	}
	return m
}

func sumInt64Metric(t *testing.T, rm metricdata.ResourceMetrics, name string) int64 {
	t.Helper()
	m := requireTestMetric(t, rm, name)
	sum, ok := m.Data.(metricdata.Sum[int64])
	if !ok {
		t.Fatalf("metric %q data type = %T, want metricdata.Sum[int64]", name, m.Data)
	}
	var value int64
	for _, point := range sum.DataPoints {
		value += point.Value
	}
	return value
}

func hasAttrs(set attribute.Set, want map[attribute.Key]string) bool {
	for key, value := range want {
		got, ok := set.Value(key)
		if !ok || got.AsString() != value {
			return false
		}
	}
	return true
}

func requireOperationCountAttrs(t *testing.T, rm metricdata.ResourceMetrics, prefix string) {
	t.Helper()
	m := requireTestMetric(t, rm, prefix+metricNameOperationCount)
	sum, ok := m.Data.(metricdata.Sum[int64])
	if !ok {
		t.Fatalf("metric %q data type = %T, want metricdata.Sum[int64]", m.Name, m.Data)
	}
	want := map[attribute.Key]string{
		metricLabelKeyMethod:     "Spanner.StreamingRead",
		metricLabelKeyStatus:     "OK",
		metricLabelKeyDatabase:   "[DATABASE]",
		metricLabelKeyClientName: clientName,
	}
	for _, point := range sum.DataPoints {
		if point.Value > 0 && hasAttrs(point.Attributes, want) {
			return
		}
	}
	t.Fatalf("metric %q has no positive point with attributes %v: %+v", m.Name, want, sum.DataPoints)
}

func issueClientMetricsRead(t *testing.T, config ClientConfig) metricdata.ResourceMetrics {
	t.Helper()
	reader, provider := newTestMeterProvider()
	config.ClientMetricsProvider = provider
	_, client, teardown := setupMockedTestServerWithConfig(t, config)
	defer teardown()
	if _, err := client.Single().ReadRow(context.Background(), "Albums", Key{"foo"}, []string{"SingerId", "AlbumId", "AlbumTitle"}); err != nil {
		t.Fatalf("ReadRow() failed: %v", err)
	}
	return collectTestMetrics(t, reader)
}

func TestClientMetricsProviderRecordsBuiltInMetrics(t *testing.T) {
	rm := issueClientMetricsRead(t, ClientConfig{DisableNativeMetrics: true})
	requireOperationCountAttrs(t, rm, clientMetricsPrefix)
	requireTestMetric(t, rm, clientMetricsPrefix+metricNameAttemptLatencies)
	if _, ok := findTestMetric(rm, nativeMetricsPrefix+metricNameOperationCount); ok {
		t.Fatalf("custom provider contains reserved metric %q", nativeMetricsPrefix+metricNameOperationCount)
	}
}

func TestClientMetricsProviderIndependentOfNativeDisableControls(t *testing.T) {
	for _, test := range []struct {
		name       string
		disable    bool
		disableEnv string
	}{
		{name: "config", disable: true},
		{name: "environment", disableEnv: "true"},
	} {
		t.Run(test.name, func(t *testing.T) {
			t.Setenv("SPANNER_DISABLE_BUILTIN_METRICS", test.disableEnv)
			rm := issueClientMetricsRead(t, ClientConfig{DisableNativeMetrics: test.disable})
			requireOperationCountAttrs(t, rm, clientMetricsPrefix)
		})
	}
}

func TestClientMetricsProviderNilPreservesDisabledBehavior(t *testing.T) {
	_, client, teardown := setupMockedTestServerWithConfig(t, ClientConfig{DisableNativeMetrics: true})
	defer teardown()
	if client.metricsTracerFactory.enabled {
		t.Fatal("metrics tracer enabled with native metrics disabled and nil ClientMetricsProvider")
	}
}

func TestOpenTelemetryMeterProviderDoesNotEnableClientMetrics(t *testing.T) {
	_, provider := newTestMeterProvider()
	_, client, teardown := setupMockedTestServerWithConfig(t, ClientConfig{
		DisableNativeMetrics:       true,
		OpenTelemetryMeterProvider: provider,
	})
	defer teardown()
	if client.metricsTracerFactory.enabled {
		t.Fatal("OpenTelemetryMeterProvider enabled built-in client metrics")
	}
}

func TestClientMetricsOmniMatrix(t *testing.T) {
	for _, test := range []struct {
		name   string
		config ClientConfig
		custom bool
	}{
		{
			name: "omni default",
			config: ClientConfig{
				Type:         OMNI,
				UsePlainText: true,
			},
		},
		{
			name: "omni custom provider",
			config: ClientConfig{
				Type:         OMNI,
				UsePlainText: true,
			},
			custom: true,
		},
		{
			name: "deprecated experimental host",
			config: ClientConfig{
				IsExperimentalHost: true,
			},
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			t.Setenv("SPANNER_DISABLE_BUILTIN_METRICS", "")
			testExporter := installTestMonitoringExporter(t)
			var reader *metric.ManualReader
			if test.custom {
				var provider *metric.MeterProvider
				reader, provider = newTestMeterProvider()
				test.config.ClientMetricsProvider = provider
			}

			_, client, teardown := setupMockedTestServerWithConfig(t, test.config)
			defer teardown()
			if testExporter.createOptionsCalls != 0 {
				t.Fatalf("native monitoring exporter constructed %d times for Omni", testExporter.createOptionsCalls)
			}
			if test.custom {
				if !client.metricsTracerFactory.enabled || len(client.metricsTracerFactory.sinks) != 1 {
					t.Fatalf("custom Omni sinks = %d, enabled = %v, want one enabled sink", len(client.metricsTracerFactory.sinks), client.metricsTracerFactory.enabled)
				}
				if client.metricsTracerFactory.meterProvider != nil {
					t.Fatal("custom provider wired to native DirectPath fallback metrics")
				}
				if _, err := client.Single().ReadRow(context.Background(), "Albums", Key{"foo"}, []string{"SingerId", "AlbumId", "AlbumTitle"}); err != nil {
					t.Fatalf("ReadRow() failed: %v", err)
				}
				requireOperationCountAttrs(t, collectTestMetrics(t, reader), clientMetricsPrefix)
			} else if client.metricsTracerFactory.enabled || len(client.metricsTracerFactory.sinks) != 0 {
				t.Fatalf("default Omni sinks = %d, enabled = %v, want none", len(client.metricsTracerFactory.sinks), client.metricsTracerFactory.enabled)
			}
		})
	}
}

func TestClientMetricsBothProvidersRecord(t *testing.T) {
	nativeReader, nativeProvider := newTestMeterProvider()
	clientReader, clientProvider := newTestMeterProvider()
	factory, err := newBuiltinMetricsTracerFactory(
		context.Background(),
		"projects/p/instances/i/databases/d",
		"identity",
		true,
		false,
		nativeProvider,
		clientProvider,
	)
	if err != nil {
		t.Fatalf("newBuiltinMetricsTracerFactory() failed: %v", err)
	}
	recordSuccessfulTestOperation(factory)
	nativeMetrics := collectTestMetrics(t, nativeReader)
	clientMetrics := collectTestMetrics(t, clientReader)
	requireTestMetric(t, nativeMetrics, nativeMetricsPrefix+metricNameOperationCount)
	requireTestMetric(t, clientMetrics, clientMetricsPrefix+metricNameOperationCount)
	if _, ok := findTestMetric(nativeMetrics, clientMetricsPrefix+metricNameOperationCount); ok {
		t.Fatal("native provider contains caller-owned metric namespace")
	}
	if _, ok := findTestMetric(clientMetrics, nativeMetricsPrefix+metricNameOperationCount); ok {
		t.Fatal("caller-owned provider contains native metric namespace")
	}
}

func TestClientMetricsBothSinksThroughClient(t *testing.T) {
	t.Setenv("SPANNER_DISABLE_BUILTIN_METRICS", "")
	t.Setenv("SPANNER_DISABLE_DIRECT_ACCESS_GRPC_BUILTIN_METRICS", "false")
	testExporter := installTestMonitoringExporter(t)
	reader, provider := newTestMeterProvider()
	_, client, teardown := setupMockedTestServerWithConfig(t, ClientConfig{ClientMetricsProvider: provider})
	defer teardown()

	before := collectTestMetrics(t, reader)
	beforeGRPC := sumInt64Metric(t, before, clientMetricsPrefix+"grpc/client/attempt/started")
	beforeAttempts := sumInt64Metric(t, before, clientMetricsPrefix+metricNameAttemptCount)
	if _, err := client.Single().ReadRow(context.Background(), "Albums", Key{"foo"}, []string{"SingerId", "AlbumId", "AlbumTitle"}); err != nil {
		t.Fatalf("ReadRow() failed: %v", err)
	}
	after := collectTestMetrics(t, reader)
	grpcDelta := sumInt64Metric(t, after, clientMetricsPrefix+"grpc/client/attempt/started") - beforeGRPC
	attemptDelta := sumInt64Metric(t, after, clientMetricsPrefix+metricNameAttemptCount) - beforeAttempts
	if grpcDelta != 1 || attemptDelta != 1 || grpcDelta != attemptDelta {
		t.Fatalf("read metric deltas: gRPC attempts = %d, client attempts = %d, want one each", grpcDelta, attemptDelta)
	}
	requireTestMetric(t, after, clientMetricsPrefix+metricNameAttemptLatencies)
	requireTestMetric(t, after, clientMetricsPrefix+metricNameGFELatencies)

	client.Close()
	if testExporter.createOptionsCalls != 1 {
		t.Fatalf("native monitoring exporter construction count = %d, want 1", testExporter.createOptionsCalls)
	}
	wantNative := map[string]bool{
		nativeMetricsPrefix + metricNameAttemptLatencies: false,
		nativeMetricsPrefix + metricNameGFELatencies:     false,
	}
	for _, req := range testExporter.server.CreateServiceTimeSeriesRequests() {
		for _, series := range req.TimeSeries {
			if _, ok := wantNative[series.GetMetric().GetType()]; ok {
				wantNative[series.GetMetric().GetType()] = true
			}
		}
	}
	for name, found := range wantNative {
		if !found {
			t.Errorf("native sink metric %q not exported", name)
		}
	}
}

func recordSuccessfulTestOperation(factory *builtinMetricsTracerFactory) {
	mt := factory.createBuiltinMetricsTracer(context.Background())
	mt.method = "/google.spanner.v1.Spanner/Read"
	mt.currOp.incrementAttemptCount()
	mt.currOp.setStatus("OK")
	mt.currOp.currAttempt = &attemptTracer{status: "OK", serverTimingMetrics: map[string]time.Duration{}}
	recordOperationCompletion(&mt)
}

func TestClientMetricsSuppressedForEmulator(t *testing.T) {
	reader, provider := newTestMeterProvider()
	t.Setenv("SPANNER_EMULATOR_HOST", "localhost:9010")
	if !isSpannerEmulatorEnabled() {
		t.Fatal("isSpannerEmulatorEnabled() = false, want true")
	}
	factory, err := newBuiltinMetricsTracerFactory(
		context.Background(),
		"projects/p/instances/i/databases/d",
		"identity",
		true,
		true,
		nil,
		provider,
	)
	if err != nil {
		t.Fatalf("newBuiltinMetricsTracerFactory() failed: %v", err)
	}
	if factory.enabled {
		t.Fatal("metrics tracer enabled for emulator")
	}
	recordSuccessfulTestOperation(factory)
	if got := collectTestMetrics(t, reader).ScopeMetrics; len(got) != 0 {
		t.Fatalf("emulator metrics = %+v, want none", got)
	}
}

func TestEmulatorDetectionUsesEnvironmentOnly(t *testing.T) {
	t.Setenv("SPANNER_EMULATOR_HOST", "")
	if isSpannerEmulatorEnabled() {
		t.Fatal("isSpannerEmulatorEnabled() = true without SPANNER_EMULATOR_HOST")
	}
}

func TestClientMetricsProviderReceivesGRPCMetrics(t *testing.T) {
	t.Setenv("SPANNER_DISABLE_DIRECT_ACCESS_GRPC_BUILTIN_METRICS", "false")
	rm := issueClientMetricsRead(t, ClientConfig{DisableNativeMetrics: true})
	m := requireTestMetric(t, rm, clientMetricsPrefix+"grpc/client/attempt/started")
	sum, ok := m.Data.(metricdata.Sum[int64])
	if !ok {
		t.Fatalf("metric %q data type = %T, want metricdata.Sum[int64]", m.Name, m.Data)
	}
	for _, point := range sum.DataPoints {
		if _, ok := point.Attributes.Value("grpc.method"); ok {
			return
		}
	}
	t.Fatalf("metric %q has no point with grpc.method: %+v", m.Name, sum.DataPoints)
}

func TestClientCloseDoesNotShutdownClientMetricsProvider(t *testing.T) {
	reader, provider := newTestMeterProvider()
	_, client, teardown := setupMockedTestServerWithConfig(t, ClientConfig{
		DisableNativeMetrics:  true,
		ClientMetricsProvider: provider,
	})
	defer func() {
		teardown()
		provider.Shutdown(context.Background())
	}()
	client.Close()

	counter, err := provider.Meter("client-close-test").Int64Counter("client_close_test")
	if err != nil {
		t.Fatalf("Int64Counter() failed after Client.Close(): %v", err)
	}
	counter.Add(context.Background(), 1)
	rm := collectTestMetrics(t, reader)
	if got := sumInt64Metric(t, rm, "client_close_test"); got != 1 {
		t.Fatalf("client_close_test = %d, want 1", got)
	}
}

func TestClientMetricsOptionsDoNotRenameEEFMetrics(t *testing.T) {
	reader, provider := newTestMeterProvider()
	t.Cleanup(func() { provider.Shutdown(context.Background()) })
	counter, err := provider.Meter(grpcGcpMetricMeterName).Int64Counter(metricNameEEFFallbackCount)
	if err != nil {
		t.Fatalf("Int64Counter() failed: %v", err)
	}
	counter.Add(context.Background(), 1)
	rm := collectTestMetrics(t, reader)
	if got := sumInt64Metric(t, rm, metricNameEEFFallbackCount); got != 1 {
		t.Fatalf("%s = %d, want 1", metricNameEEFFallbackCount, got)
	}
	if _, ok := findTestMetric(rm, clientMetricsPrefix+"eef/fallback/count"); ok {
		t.Fatal("EEF metric renamed into caller-owned client namespace")
	}
}

type failingInstrumentMeter struct {
	noop.Meter
}

func (failingInstrumentMeter) Float64Histogram(string, ...otelmetric.Float64HistogramOption) (otelmetric.Float64Histogram, error) {
	return nil, errors.New("instrument creation failed")
}

type failingInstrumentMeterProvider struct {
	noop.MeterProvider
}

func (failingInstrumentMeterProvider) Meter(string, ...otelmetric.MeterOption) otelmetric.Meter {
	return failingInstrumentMeter{}
}

func TestNewBuiltinMetricsTracerFactoryReleasesNativeProviderOnClientSinkError(t *testing.T) {
	installTestMonitoringExporter(t)
	factory, err := newBuiltinMetricsTracerFactory(
		context.Background(),
		"projects/p/instances/i/databases/d",
		"identity",
		true,
		false,
		nil,
		failingInstrumentMeterProvider{},
	)
	if err == nil {
		t.Fatal("newBuiltinMetricsTracerFactory() succeeded with a failing client metrics provider")
	}
	native, ok := factory.meterProvider.(*metric.MeterProvider)
	if !ok {
		t.Fatalf("native meter provider type = %T, want *metric.MeterProvider", factory.meterProvider)
	}
	if err := native.ForceFlush(context.Background()); !errors.Is(err, metric.ErrReaderShutdown) {
		t.Fatalf("native meter provider ForceFlush() error = %v, want %v after constructor failure", err, metric.ErrReaderShutdown)
	}
	// The failed constructor already released what it owned; a later shutdown must be a no-op.
	factory.shutdown(context.Background())
}

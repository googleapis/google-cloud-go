/*
Copyright 2024 Google LLC

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
	"flag"
	"fmt"
	"net"
	"slices"
	"sort"
	"testing"

	"cloud.google.com/go/internal/testutil"
	. "cloud.google.com/go/spanner/internal/testutil"
	"github.com/google/go-cmp/cmp/cmpopts"
	"go.opentelemetry.io/otel/attribute"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/metric/metricdata"
	"google.golang.org/api/option"
	"google.golang.org/genproto/googleapis/api/metric"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials/alts"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/experimental/stats"
	"google.golang.org/grpc/health"
	healthpb "google.golang.org/grpc/health/grpc_health_v1"
	"google.golang.org/grpc/peer"
	"google.golang.org/grpc/stats/opentelemetry"
	"google.golang.org/grpc/status"
)

func TestNewBuiltinMetricsTracerFactory(t *testing.T) {
	flag.Parse() // Needed for testing.Short().
	if testing.Short() {
		t.Skip("TestNewBuiltinMetricsTracerFactory tests skipped in -short mode.")
	}
	t.Setenv("SPANNER_DISABLE_DIRECT_ACCESS_GRPC_BUILTIN_METRICS", "false")

	ctx := context.Background()
	clientUID := "test-uid"
	createSessionRPC := "Spanner.BatchCreateSessions"
	if isMultiplexEnabled {
		createSessionRPC = "Spanner.CreateSession"
	}

	wantClientAttributes := []attribute.KeyValue{
		attribute.String(monitoredResLabelKeyProject, "[PROJECT]"),
		attribute.String(monitoredResLabelKeyInstance, "[INSTANCE]"),
		attribute.String(metricLabelKeyDatabase, "[DATABASE]"),
		attribute.String(metricLabelKeyClientUID, clientUID),
		attribute.String(metricLabelKeyClientName, clientName),
		attribute.String(monitoredResLabelKeyClientHash, "0000ed"),
		attribute.String(monitoredResLabelKeyInstanceConfig, "unknown"),
		attribute.String(monitoredResLabelKeyLocation, "global"),
	}
	wantMetricNamesStdout := []string{metricNameAttemptCount, metricNameAttemptLatencies, metricNameOperationCount, metricNameOperationLatencies, metricNameGFELatencies}
	wantMetricTypesGCM := []string{}
	for _, wantMetricName := range wantMetricNamesStdout {
		wantMetricTypesGCM = append(wantMetricTypesGCM, nativeMetricsPrefix+wantMetricName)
	}

	// return constant client UID instead of random, so that attributes can be compared
	origGenerateClientUID := generateClientUID
	origDetectClientLocation := detectClientLocation
	generateClientUID = func() (string, error) {
		return clientUID, nil
	}
	detectClientLocation = func(ctx context.Context) string {
		return "global"
	}
	defer func() {
		generateClientUID = origGenerateClientUID
		detectClientLocation = origDetectClientLocation
	}()

	// Setup mock monitoring server
	monitoringServer, err := NewMetricTestServer()
	if err != nil {
		t.Fatalf("Error setting up metrics test server")
	}
	go monitoringServer.Serve()
	defer monitoringServer.Shutdown()

	// Override exporter options
	origCreateExporterOptions := createExporterOptions
	createExporterOptions = func(opts ...option.ClientOption) []option.ClientOption {
		return []option.ClientOption{
			option.WithEndpoint(monitoringServer.Endpoint), // Connect to mock
			option.WithoutAuthentication(),
			option.WithGRPCDialOption(grpc.WithTransportCredentials(insecure.NewCredentials())),
		}
	}
	defer func() {
		createExporterOptions = origCreateExporterOptions
	}()

	tests := []struct {
		desc                   string
		config                 ClientConfig
		wantBuiltinEnabled     bool
		runOnlyInEmulator      bool
		wantCreateTSCallsCount int // No. of CreateTimeSeries calls
		wantMethods            []string
		wantOTELValue          map[string]map[string]int64
		wantOTELMetrics        map[string][]string
	}{
		{
			desc:              "should create a new tracer factory with default meter provider",
			runOnlyInEmulator: isEmulatorEnvSet(),
			config: ClientConfig{
				SessionPoolConfig: SessionPoolConfig{
					MinOpened: 0,
					MaxOpened: 1,
				},
			},

			wantBuiltinEnabled:     true,
			wantCreateTSCallsCount: 1,
			wantMethods:            []string{createSessionRPC, "Spanner.StreamingRead"},
			wantOTELValue: map[string]map[string]int64{
				createSessionRPC: {
					nativeMetricsPrefix + metricNameAttemptCount:   1,
					nativeMetricsPrefix + metricNameOperationCount: 1,
				},
				"Spanner.StreamingRead": {
					nativeMetricsPrefix + metricNameAttemptCount:              2,
					nativeMetricsPrefix + metricNameOperationCount:            1,
					nativeMetricsPrefix + metricNameGFEConnectivityErrorCount: 1,
				},
			},
			wantOTELMetrics: map[string][]string{
				createSessionRPC: {
					nativeMetricsPrefix + metricNameAttemptCount,
					nativeMetricsPrefix + metricNameAttemptLatencies,
					nativeMetricsPrefix + metricNameGFELatencies,
					nativeMetricsPrefix + metricNameOperationCount,
					nativeMetricsPrefix + metricNameOperationLatencies,
				},
				"Spanner.StreamingRead": {
					nativeMetricsPrefix + metricNameAttemptCount,
					nativeMetricsPrefix + metricNameAttemptLatencies,
					nativeMetricsPrefix + metricNameAttemptLatencies,
					nativeMetricsPrefix + metricNameGFEConnectivityErrorCount,
					nativeMetricsPrefix + metricNameGFELatencies,
					nativeMetricsPrefix + metricNameOperationCount,
					nativeMetricsPrefix + metricNameOperationLatencies,
				},
			},
		},
		{
			desc:               "should not create instruments when SPANNER_EMULATOR_HOST is set",
			runOnlyInEmulator:  !isEmulatorEnvSet(),
			config:             ClientConfig{},
			wantBuiltinEnabled: false,
		},
	}

	for _, test := range tests {
		t.Run(test.desc, func(t *testing.T) {
			if test.runOnlyInEmulator {
				t.Skip("Skipping test that should only run in emulator")
			}
			server, client, teardown := setupMockedTestServerWithConfig(t, test.config)
			defer teardown()
			server.TestSpanner.PutExecutionTime(MethodStreamingRead,
				SimulatedExecutionTime{
					Errors: []error{status.Error(codes.Unavailable, "Temporary unavailable")},
				})

			if client.metricsTracerFactory.enabled != test.wantBuiltinEnabled {
				t.Errorf("builtinEnabled: got: %v, want: %v", client.metricsTracerFactory.enabled, test.wantBuiltinEnabled)
			}

			if diff := testutil.Diff(client.metricsTracerFactory.clientAttributes, wantClientAttributes, cmpopts.EquateComparable(attribute.KeyValue{}, attribute.Value{})); diff != "" {
				t.Errorf("clientAttributes: got=-, want=+ \n%v", diff)
			}

			// Check instruments
			gotNonNilInstruments := client.metricsTracerFactory.operationLatencies != nil &&
				client.metricsTracerFactory.operationCount != nil &&
				client.metricsTracerFactory.attemptLatencies != nil &&
				client.metricsTracerFactory.attemptCount != nil
			if test.wantBuiltinEnabled != gotNonNilInstruments {
				t.Errorf("NonNilInstruments: got: %v, want: %v", gotNonNilInstruments, test.wantBuiltinEnabled)
			}

			// pop out all old requests
			monitoringServer.CreateServiceTimeSeriesRequests()

			// Perform single use read-only transaction
			_, err = client.Single().ReadRow(ctx, "Albums", Key{"foo"}, []string{"SingerId", "AlbumId", "AlbumTitle"})
			if err != nil {
				t.Fatalf("ReadRows failed: %v", err)
			}

			client.Close()
			// Get new CreateServiceTimeSeriesRequests
			gotCreateTSCalls := monitoringServer.CreateServiceTimeSeriesRequests()
			var gotExpectedMethods []string
			gotOTELCountValues := make(map[string]map[string]int64)
			gotOTELLatencyValues := make(map[string]map[string]float64)
			gotGRPCBuiltInMetric := false
			for _, gotCreateTSCall := range gotCreateTSCalls {
				gotMetricTypesPerMethod := make(map[string][]string)
				for _, ts := range gotCreateTSCall.TimeSeries {
					if ts.Metric.Type == nativeMetricsPrefix+"grpc/client/attempt/started" {
						gotGRPCBuiltInMetric = true
						wantLabels := map[string]string{
							metricLabelKeyInstanceID:  "[INSTANCE]",
							metricLabelKeyDatabaseID:  "[DATABASE]",
							"grpc_client_call_custom": "[INSTANCE]|[DATABASE]",
						}
						for key, want := range wantLabels {
							if got := ts.Metric.Labels[key]; got != want {
								t.Errorf("gRPC built-in metric label %q = %q, want %q; all labels: %v", key, got, want, ts.Metric.Labels)
							}
						}
					}
					method := ts.Metric.GetLabels()["method"]
					if method == "" {
						continue
					}
					gotMetricTypesPerMethod[method] = append(gotMetricTypesPerMethod[method], ts.Metric.Type)
					if _, ok := gotOTELCountValues[method]; !ok {
						gotOTELCountValues[method] = make(map[string]int64)
						gotOTELLatencyValues[method] = make(map[string]float64)
						gotExpectedMethods = append(gotExpectedMethods, method)
					}
					if ts.MetricKind == metric.MetricDescriptor_CUMULATIVE && ts.GetValueType() == metric.MetricDescriptor_INT64 && len(ts.Points) > 0 {
						gotOTELCountValues[method][ts.Metric.Type] = ts.Points[0].GetValue().GetInt64Value()
					} else {
						for _, p := range ts.Points {
							if _, ok := gotOTELLatencyValues[method][ts.Metric.Type]; !ok {
								if dist := p.GetValue().GetDistributionValue(); dist != nil {
									gotOTELLatencyValues[method][ts.Metric.Type] = dist.Mean
								}
							} else {
								// sum up all attempt latencies
								if dist := p.GetValue().GetDistributionValue(); dist != nil {
									gotOTELLatencyValues[method][ts.Metric.Type] += dist.Mean
								}
							}
						}
					}
				}
				for method, gotMetricTypes := range gotMetricTypesPerMethod {
					sort.Strings(gotMetricTypes)
					sort.Strings(test.wantOTELMetrics[method])
					if !testutil.Equal(gotMetricTypes, test.wantOTELMetrics[method]) {
						t.Errorf("Metric types missing in req. %s got: %v, want: %v", method, gotMetricTypes, test.wantOTELMetrics[method])
					}
				}
			}
			sort.Strings(gotExpectedMethods)
			if !testutil.Equal(gotExpectedMethods, test.wantMethods) {
				t.Errorf("Expected methods missing in req. got: %v, want: %v", gotExpectedMethods, test.wantMethods)
			}
			for method, wantOTELValues := range test.wantOTELValue {
				for metricName, wantValue := range wantOTELValues {
					if gotOTELCountValues[method][metricName] != wantValue {
						t.Errorf("OTEL value for %s, %s: got: %v, want: %v", method, metricName, gotOTELCountValues[method][metricName], wantValue)
					}
				}
				// For StreamingRead, verify operation latency includes all attempt latencies
				opLatency := gotOTELLatencyValues[method][nativeMetricsPrefix+metricNameOperationLatencies]
				attemptLatency := gotOTELLatencyValues[method][nativeMetricsPrefix+metricNameAttemptLatencies]
				gfeLatency := gotOTELLatencyValues[method][nativeMetricsPrefix+metricNameGFELatencies]
				// expect opLatency and attemptLatency to be non-zero
				if opLatency == 0 || attemptLatency == 0 {
					t.Errorf("Operation and attempt latencies should be non-zero for %s: operation_latency=%v, attempt_latency=%v",
						method, opLatency, attemptLatency)
				}
				if gfeLatency != 123 {
					t.Errorf("GFE latency should be 123 for %s: gfe_latency=%v", method, gfeLatency)
				}
				if opLatency <= attemptLatency {
					t.Errorf("Operation latency should be greater than attempt latency for %s: operation_latency=%v, attempt_latency=%v",
						method, opLatency, attemptLatency)
				}
			}
			gotCreateTSCallsCount := len(gotCreateTSCalls)
			if gotCreateTSCallsCount < test.wantCreateTSCallsCount {
				t.Errorf("No. of CreateServiceTimeSeriesRequests: got: %v,  want: %v", gotCreateTSCalls, test.wantCreateTSCallsCount)
			}
			if test.wantBuiltinEnabled && !gotGRPCBuiltInMetric {
				t.Error("grpc.client.attempt.started metric missing from Cloud Monitoring export")
			}
		})
	}
}

func TestGRPCMetricFixedAttributesSurviveViews(t *testing.T) {
	ctx := context.Background()
	reader := sdkmetric.NewManualReader()
	provider := sdkmetric.NewMeterProvider(
		sdkmetric.WithReader(reader),
		sdkmetric.WithView(builtInMetricViews()...),
	)
	t.Cleanup(func() {
		if err := provider.Shutdown(ctx); err != nil {
			t.Errorf("provider.Shutdown() returned error: %v", err)
		}
	})

	const (
		instanceID = "test-instance"
		databaseID = "test-database"
	)
	meter := newFixedAttributeMeterProvider(provider, []attribute.KeyValue{
		attribute.String(metricLabelKeyInstanceID, instanceID),
		attribute.String(metricLabelKeyDatabaseID, databaseID),
	}).Meter(grpcMetricMeterName)

	for _, name := range grpcMetricsToEnable {
		desc := stats.DescriptorForMetric(name)
		switch {
		case name == "grpc.client.attempt.started":
			counter, err := meter.Int64Counter(name)
			if err != nil {
				t.Fatalf("Int64Counter(%q) returned error: %v", name, err)
			}
			counter.Add(ctx, 1)
		case desc == nil:
			t.Fatalf("stats.DescriptorForMetric(%q) returned nil", name)
		case desc.Type == stats.MetricTypeIntCount:
			counter, err := meter.Int64Counter(name)
			if err != nil {
				t.Fatalf("Int64Counter(%q) returned error: %v", name, err)
			}
			counter.Add(ctx, 1)
		case desc.Type == stats.MetricTypeIntUpDownCount:
			counter, err := meter.Int64UpDownCounter(name)
			if err != nil {
				t.Fatalf("Int64UpDownCounter(%q) returned error: %v", name, err)
			}
			counter.Add(ctx, 1)
		default:
			t.Fatalf("metric %q has unsupported type %v", name, desc.Type)
		}
	}

	var got metricdata.ResourceMetrics
	if err := reader.Collect(ctx, &got); err != nil {
		t.Fatalf("reader.Collect() returned error: %v", err)
	}
	gotMetrics := make(map[string]metricdata.Metrics)
	for _, scope := range got.ScopeMetrics {
		for _, m := range scope.Metrics {
			gotMetrics[m.Name] = m
		}
	}
	for _, name := range grpcMetricsToEnable {
		m, ok := gotMetrics[name]
		if !ok {
			t.Errorf("metric %q missing after collection", name)
			continue
		}
		attrs := metricAttributes(t, m)
		assertMetricAttribute(t, name, attrs, metricLabelKeyInstanceID, instanceID)
		assertMetricAttribute(t, name, attrs, metricLabelKeyDatabaseID, databaseID)
	}
}

func TestGRPCCustomLabelOnOutgoingCalls(t *testing.T) {
	ctx := context.Background()
	lis, err := net.Listen("tcp", "localhost:0")
	if err != nil {
		t.Fatalf("net.Listen() returned error: %v", err)
	}
	server := grpc.NewServer()
	healthpb.RegisterHealthServer(server, health.NewServer())
	go server.Serve(lis)
	t.Cleanup(func() {
		server.Stop()
		lis.Close()
	})

	reader := sdkmetric.NewManualReader()
	provider := sdkmetric.NewMeterProvider(
		sdkmetric.WithReader(reader),
		sdkmetric.WithView(builtInMetricViews()...),
	)
	t.Cleanup(func() {
		if err := provider.Shutdown(ctx); err != nil {
			t.Errorf("provider.Shutdown() returned error: %v", err)
		}
	})

	const (
		instanceID  = "test-instance"
		databaseID  = "test-database"
		customLabel = instanceID + "|" + databaseID
	)
	grpcProvider := newFixedAttributeMeterProvider(provider, []attribute.KeyValue{
		attribute.String(metricLabelKeyInstanceID, instanceID),
		attribute.String(metricLabelKeyDatabaseID, databaseID),
	})
	metricsOptions := opentelemetry.MetricsOptions{
		MeterProvider:  grpcProvider,
		Metrics:        stats.NewMetrics(grpcMetricsToEnable...),
		OptionalLabels: grpcOptionalLabels,
	}
	conn, err := grpc.NewClient(
		lis.Addr().String(),
		grpc.WithTransportCredentials(insecure.NewCredentials()),
		opentelemetry.DialOption(opentelemetry.Options{MetricsOptions: metricsOptions}),
		grpc.WithChainUnaryInterceptor(grpcCustomLabelUnaryInterceptor(customLabel)),
		grpc.WithChainStreamInterceptor(grpcCustomLabelStreamInterceptor(customLabel)),
	)
	if err != nil {
		t.Fatalf("grpc.NewClient() returned error: %v", err)
	}
	t.Cleanup(func() {
		if err := conn.Close(); err != nil {
			t.Errorf("conn.Close() returned error: %v", err)
		}
	})

	healthClient := healthpb.NewHealthClient(conn)
	if _, err := healthClient.Check(ctx, &healthpb.HealthCheckRequest{}); err != nil {
		t.Fatalf("Health.Check() returned error: %v", err)
	}
	watch, err := healthClient.Watch(ctx, &healthpb.HealthCheckRequest{})
	if err != nil {
		t.Fatalf("Health.Watch() returned error: %v", err)
	}
	if _, err := watch.Recv(); err != nil {
		t.Fatalf("Health.Watch().Recv() returned error: %v", err)
	}
	if err := watch.CloseSend(); err != nil {
		t.Fatalf("Health.Watch().CloseSend() returned error: %v", err)
	}

	var got metricdata.ResourceMetrics
	if err := reader.Collect(ctx, &got); err != nil {
		t.Fatalf("reader.Collect() returned error: %v", err)
	}
	for _, scope := range got.ScopeMetrics {
		for _, m := range scope.Metrics {
			if m.Name != "grpc.client.attempt.started" {
				continue
			}
			attrs := metricAttributes(t, m)
			assertMetricAttribute(t, m.Name, attrs, metricLabelKeyGRPCClientCallCustom, customLabel)
			assertMetricAttribute(t, m.Name, attrs, metricLabelKeyInstanceID, instanceID)
			assertMetricAttribute(t, m.Name, attrs, metricLabelKeyDatabaseID, databaseID)
			return
		}
	}
	t.Fatal("grpc.client.attempt.started metric missing after RPC")
}

func TestGRPCCustomLabelConfiguredForPerAttemptAndRLSPickMetrics(t *testing.T) {
	if !slices.Contains(grpcOptionalLabels, metricLabelKeyGRPCClientCallCustom) {
		t.Errorf("grpcOptionalLabels missing %q", metricLabelKeyGRPCClientCallCustom)
	}
	for _, name := range []string{
		"grpc.lb.rls.default_target_picks",
		"grpc.lb.rls.target_picks",
	} {
		desc := stats.DescriptorForMetric(name)
		if desc == nil {
			t.Errorf("stats.DescriptorForMetric(%q) returned nil", name)
			continue
		}
		if !slices.Contains(desc.OptionalLabels, metricLabelKeyGRPCClientCallCustom) {
			t.Errorf("%s optional labels %v missing %q", name, desc.OptionalLabels, metricLabelKeyGRPCClientCallCustom)
		}
	}
	for _, key := range []string{
		metricLabelKeyInstanceID,
		metricLabelKeyDatabaseID,
		metricLabelKeyGRPCClientCallCustom,
	} {
		if !allowedMetricLabels[key] {
			t.Errorf("allowedMetricLabels missing %q", key)
		}
	}
}

func metricAttributes(t *testing.T, m metricdata.Metrics) attribute.Set {
	t.Helper()
	switch data := m.Data.(type) {
	case metricdata.Sum[int64]:
		if len(data.DataPoints) != 1 {
			t.Fatalf("%s data point count = %d, want 1", m.Name, len(data.DataPoints))
		}
		return data.DataPoints[0].Attributes
	default:
		t.Fatalf("%s data type = %T, want metricdata.Sum[int64]", m.Name, m.Data)
		return attribute.Set{}
	}
}

func assertMetricAttribute(t *testing.T, metricName string, attrs attribute.Set, key, want string) {
	t.Helper()
	got, ok := attrs.Value(attribute.Key(key))
	if !ok {
		t.Errorf("%s attributes missing %q: %v", metricName, key, attrs)
		return
	}
	if got.AsString() != want {
		t.Errorf("%s attribute %q = %q, want %q", metricName, key, got.AsString(), want)
	}
}

// TestGenerateClientHash tests the generateClientHash function.
func TestGenerateClientHash(t *testing.T) {
	tests := []struct {
		name             string
		clientUID        string
		expectedValue    string
		expectedLength   int
		expectedMaxValue int64
	}{
		{"Simple UID", "exampleUID", "00006b", 6, 0x3FF},
		{"Empty UID", "", "000000", 6, 0x3FF},
		{"Special Characters", "!@#$%^&*()", "000389", 6, 0x3FF},
		{"Very Long UID", "aVeryLongUniqueIdentifierThatExceedsNormalLength", "000125", 6, 0x3FF},
		{"Numeric UID", "1234567890", "00003e", 6, 0x3FF},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			hash := generateClientHash(tt.clientUID)
			if hash != tt.expectedValue {
				t.Errorf("expected hash value %s, got %s", tt.expectedValue, hash)
			}
			// Check if the hash length is 6
			if len(hash) != tt.expectedLength {
				t.Errorf("expected hash length %d, got %d", tt.expectedLength, len(hash))
			}

			// Check if the hash is in the range [000000, 0003ff]
			hashValue, err := parseHex(hash)
			if err != nil {
				t.Errorf("failed to parse hash: %v", err)
			}
			if hashValue < 0 || hashValue > tt.expectedMaxValue {
				t.Errorf("expected hash value in range [0, %d], got %d", tt.expectedMaxValue, hashValue)
			}
		})
	}
}

// parseHex converts a hexadecimal string to an int64.
func parseHex(hexStr string) (int64, error) {
	var value int64
	_, err := fmt.Sscanf(hexStr, "%x", &value)
	return value, err
}

type mockALTSAuthInfo struct {
	alts.AuthInfo
}

func (m mockALTSAuthInfo) AuthType() string { return "alts" }

type mockOtherAuthInfo struct{}

func (m mockOtherAuthInfo) AuthType() string { return "other" }

func TestSetDirectPathUsed(t *testing.T) {
	tests := []struct {
		name string
		peer *peer.Peer
		want bool
	}{
		{
			name: "ALTS AuthInfo",
			peer: &peer.Peer{
				AuthInfo: mockALTSAuthInfo{},
			},
			want: true,
		},
		{
			name: "Other AuthInfo",
			peer: &peer.Peer{
				AuthInfo: mockOtherAuthInfo{},
			},
			want: false,
		},
		{
			name: "No AuthInfo",
			peer: &peer.Peer{},
			want: false,
		},
		{
			name: "No Peer",
			peer: nil,
			want: false,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			tracer := &attemptTracer{}
			ctx := context.Background()
			if tc.peer != nil {
				ctx = peer.NewContext(ctx, tc.peer)
			}
			tracer.setDirectPathUsed(ctx)
			if tracer.directPathUsed != tc.want {
				t.Errorf("setDirectPathUsed() = %v, want %v", tracer.directPathUsed, tc.want)
			}
		})
	}
}

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
	"bytes"
	"context"
	"flag"
	"fmt"
	"log"
	"net"
	"sort"
	"strings"
	"sync"
	"testing"

	"cloud.google.com/go/internal/testutil"
	"cloud.google.com/go/monitoring/apiv3/v2/monitoringpb"
	. "cloud.google.com/go/spanner/internal/testutil"
	"github.com/GoogleCloudPlatform/grpc-gcp-go/grpcgcp"
	"github.com/GoogleCloudPlatform/grpc-gcp-go/grpcgcp/multiendpoint"
	"github.com/google/go-cmp/cmp/cmpopts"
	"go.opentelemetry.io/otel/attribute"
	"golang.org/x/oauth2"
	"golang.org/x/oauth2/google"
	"google.golang.org/api/option"
	gtransport "google.golang.org/api/transport/grpc"
	"google.golang.org/genproto/googleapis/api/metric"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/connectivity"
	"google.golang.org/grpc/credentials/alts"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/peer"
	"google.golang.org/grpc/resolver"
	"google.golang.org/grpc/status"
)

func TestNewBuiltinMetricsTracerFactory(t *testing.T) {
	flag.Parse() // Needed for testing.Short().
	if testing.Short() {
		t.Skip("TestNewBuiltinMetricsTracerFactory tests skipped in -short mode.")
	}

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
	t.Setenv("SPANNER_MONITORING_HOST", monitoringServer.Endpoint)

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
			server, client, teardown := setupAuthenticatedMockedTestServerWithConfig(t, test.config)
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
			gotNonNilInstruments := len(client.metricsTracerFactory.sinks) > 0 &&
				client.metricsTracerFactory.sinks[0].operationLatencies != nil &&
				client.metricsTracerFactory.sinks[0].operationCount != nil &&
				client.metricsTracerFactory.sinks[0].attemptLatencies != nil &&
				client.metricsTracerFactory.sinks[0].attemptCount != nil
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
			for _, gotCreateTSCall := range gotCreateTSCalls {
				gotMetricTypesPerMethod := make(map[string][]string)
				for _, ts := range gotCreateTSCall.TimeSeries {
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
		})
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

func TestMonitoringExporterEndpoint(t *testing.T) {
	tests := []struct {
		name              string
		envUniverseDomain string
		envMonitoringHost string
		spannerOpts       []option.ClientOption
		wantTarget        string
	}{
		{
			name:       "default GDU endpoint",
			wantTarget: "monitoring.googleapis.com:443",
		},
		{
			name:        "WithUniverseDomain routes to universe monitoring endpoint",
			spannerOpts: []option.ClientOption{option.WithUniverseDomain("example-universe.test")},
			wantTarget:  "monitoring.example-universe.test:443",
		},
		{
			name:              "GOOGLE_CLOUD_UNIVERSE_DOMAIN env var routes to universe monitoring endpoint",
			envUniverseDomain: "example-universe.test",
			wantTarget:        "monitoring.example-universe.test:443",
		},
		{
			name:              "WithUniverseDomain overrides GOOGLE_CLOUD_UNIVERSE_DOMAIN env var",
			envUniverseDomain: "env-universe.test",
			spannerOpts:       []option.ClientOption{option.WithUniverseDomain("opt-universe.test")},
			wantTarget:        "monitoring.opt-universe.test:443",
		},
		{
			name: "Spanner WithEndpoint is stripped while preserving WithUniverseDomain",
			spannerOpts: []option.ClientOption{
				option.WithUniverseDomain("example-universe.test"),
				option.WithEndpoint("spanner.example-universe.test:443"),
			},
			wantTarget: "monitoring.example-universe.test:443",
		},
		{
			name: "Spanner WithEndpoint is stripped in default GDU",
			spannerOpts: []option.ClientOption{
				option.WithEndpoint("spanner.custom.local:443"),
			},
			wantTarget: "monitoring.googleapis.com:443",
		},
		{
			name:              "SPANNER_MONITORING_HOST explicitly overrides monitoring endpoint",
			envMonitoringHost: "custom-monitoring.local:443",
			spannerOpts: []option.ClientOption{
				option.WithUniverseDomain("example-universe.test"),
				option.WithEndpoint("spanner.example-universe.test:443"),
			},
			wantTarget: "custom-monitoring.local:443",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv("GOOGLE_CLOUD_UNIVERSE_DOMAIN", tc.envUniverseDomain)
			t.Setenv("SPANNER_MONITORING_HOST", tc.envMonitoringHost)
			// Keep GDU endpoint resolution hermetic on hosts with ambient mTLS client certificates.
			t.Setenv("GOOGLE_API_USE_CLIENT_CERTIFICATE", "false")
			t.Setenv("GOOGLE_API_USE_MTLS_ENDPOINT", "auto")

			ctx := context.Background()
			opts := append([]option.ClientOption{option.WithoutAuthentication()}, tc.spannerOpts...)
			exporter, err := newMonitoringExporter(ctx, "test-project", "", nil, createExporterOptions(opts...)...)
			if err != nil {
				t.Fatalf("newMonitoringExporter() failed: %v", err)
			}
			defer exporter.Shutdown(ctx)

			if got := exporter.client.Connection().Target(); got != tc.wantTarget {
				t.Errorf("monitoring client target = %q, want %q", got, tc.wantTarget)
			}
		})
	}
}

type wrappedEndpointOption struct{ option.ClientOption }

func TestMonitoringExporterEndpointWrappedOption(t *testing.T) {
	tests := []struct {
		name              string
		envUniverseDomain string
		envMonitoringHost string
		spannerOpts       []option.ClientOption
		wantTarget        string
	}{
		{
			name: "wrapped Spanner WithEndpoint is reset while preserving WithUniverseDomain",
			spannerOpts: []option.ClientOption{
				option.WithUniverseDomain("example-universe.test"),
				wrappedEndpointOption{option.WithEndpoint("spanner.example-universe.test:443")},
			},
			wantTarget: "monitoring.example-universe.test:443",
		},
		{
			name: "wrapped Spanner WithEndpoint is reset in default GDU",
			spannerOpts: []option.ClientOption{
				wrappedEndpointOption{option.WithEndpoint("spanner.custom.local:443")},
			},
			wantTarget: "monitoring.googleapis.com:443",
		},
		{
			name:              "wrapped Spanner WithEndpoint is overridden by SPANNER_MONITORING_HOST",
			envMonitoringHost: "custom-monitoring.local:443",
			spannerOpts: []option.ClientOption{
				option.WithUniverseDomain("example-universe.test"),
				wrappedEndpointOption{option.WithEndpoint("spanner.example-universe.test:443")},
			},
			wantTarget: "custom-monitoring.local:443",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv("GOOGLE_CLOUD_UNIVERSE_DOMAIN", tc.envUniverseDomain)
			t.Setenv("SPANNER_MONITORING_HOST", tc.envMonitoringHost)
			// Keep GDU endpoint resolution hermetic on hosts with ambient mTLS client certificates.
			t.Setenv("GOOGLE_API_USE_CLIENT_CERTIFICATE", "false")
			t.Setenv("GOOGLE_API_USE_MTLS_ENDPOINT", "auto")

			ctx := context.Background()
			opts := append([]option.ClientOption{option.WithoutAuthentication()}, tc.spannerOpts...)
			exporter, err := newMonitoringExporter(ctx, "test-project", "", nil, createExporterOptions(opts...)...)
			if err != nil {
				t.Fatalf("newMonitoringExporter() failed: %v", err)
			}
			defer exporter.Shutdown(ctx)

			if w, g := tc.wantTarget, exporter.client.Connection().Target(); w != g {
				t.Errorf("monitoring client target mismatch:\n Want: %v\n  Got: %v", w, g)
			}
		})
	}
}

// staticDNSResolverBuilder resolves any "dns" target directly to its literal
// endpoint string so tests can intercept dialed hostnames via WithContextDialer
// without performing real DNS lookups or modifying global resolver state.
type staticDNSResolverBuilder struct{}

func (staticDNSResolverBuilder) Build(target resolver.Target, cc resolver.ClientConn, _ resolver.BuildOptions) (resolver.Resolver, error) {
	endpoint := target.Endpoint()
	host, _, err := net.SplitHostPort(endpoint)
	if err != nil {
		host = endpoint
	}
	if err := cc.UpdateState(resolver.State{
		Addresses: []resolver.Address{{Addr: endpoint, ServerName: host}},
	}); err != nil {
		return nil, err
	}
	return noopResolver{}, nil
}

func (staticDNSResolverBuilder) Scheme() string { return "dns" }

type noopResolver struct{}

func (noopResolver) ResolveNow(resolver.ResolveNowOptions) {}
func (noopResolver) Close()                                {}

// TestBuiltinMetricsExporterUniverseDomainEndToEnd verifies end-to-end that a
// Spanner client configured for a non-default universe domain exports built-in
// metrics with its credentials to monitoring.<universe_domain>:443 and never
// dials monitoring.googleapis.com:443.
func TestBuiltinMetricsExporterUniverseDomainEndToEnd(t *testing.T) {
	const universeDomain = "example-universe.test"
	const wantSpannerAddr = "spanner." + universeDomain + ":443"
	const wantMonitoringAddr = "monitoring." + universeDomain + ":443"
	const wantToken = "test-universe-token"

	origDetectClientLocation := detectClientLocation
	detectClientLocation = func(context.Context) string { return "global" }
	defer func() { detectClientLocation = origDetectClientLocation }()

	creds := &google.Credentials{
		ProjectID:              "test-project",
		TokenSource:            oauth2.StaticTokenSource(&oauth2.Token{AccessToken: wantToken}),
		UniverseDomainProvider: func() (string, error) { return universeDomain, nil },
	}

	tests := []struct {
		name              string
		envUniverseDomain string
		extraOpts         []option.ClientOption
	}{
		{
			name:      "WithUniverseDomain option",
			extraOpts: []option.ClientOption{option.WithUniverseDomain(universeDomain)},
		},
		{
			name:              "GOOGLE_CLOUD_UNIVERSE_DOMAIN env var",
			envUniverseDomain: universeDomain,
		},
		{
			name: "WithUniverseDomain and Spanner WithEndpoint",
			extraOpts: []option.ClientOption{
				option.WithUniverseDomain(universeDomain),
				option.WithEndpoint(wantSpannerAddr),
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv("SPANNER_EMULATOR_HOST", "")
			t.Setenv("SPANNER_DISABLE_BUILTIN_METRICS", "")
			t.Setenv("SPANNER_MONITORING_HOST", "")
			t.Setenv("GOOGLE_CLOUD_DISABLE_DIRECT_PATH", "true")
			t.Setenv("GOOGLE_CLOUD_UNIVERSE_DOMAIN", tc.envUniverseDomain)

			spannerServer, _, teardownSpanner := NewMockedSpannerInMemTestServer(t)
			defer teardownSpanner()

			monitoringServer, err := NewMetricTestServer()
			if err != nil {
				t.Fatalf("NewMetricTestServer() failed: %v", err)
			}
			go monitoringServer.Serve()
			defer monitoringServer.Shutdown()

			var mu sync.Mutex
			dialedAddrs := map[string]int{}
			unexpectedAddrs := []string{}

			dialer := func(ctx context.Context, addr string) (net.Conn, error) {
				mu.Lock()
				dialedAddrs[addr]++
				mu.Unlock()

				var target string
				switch addr {
				case wantSpannerAddr:
					target = spannerServer.ServerAddress
				case wantMonitoringAddr:
					target = monitoringServer.Endpoint
				default:
					mu.Lock()
					unexpectedAddrs = append(unexpectedAddrs, addr)
					mu.Unlock()
					return nil, fmt.Errorf("unexpected dial to %q", addr)
				}
				return (&net.Dialer{}).DialContext(ctx, "tcp", target)
			}

			opts := []option.ClientOption{
				option.WithCredentials(creds),
				option.WithGRPCDialOption(grpc.WithTransportCredentials(insecure.NewCredentials())),
				option.WithGRPCDialOption(grpc.WithResolvers(staticDNSResolverBuilder{})),
				option.WithGRPCDialOption(grpc.WithContextDialer(dialer)),
			}
			opts = append(opts, tc.extraOpts...)

			ctx := context.Background()
			client, err := NewClientWithConfig(ctx, "projects/test-project/instances/test-instance/databases/test-db", ClientConfig{
				NumChannels: 1,
			}, opts...)
			if err != nil {
				t.Fatalf("NewClientWithConfig() failed: %v", err)
			}

			iter := client.Single().Query(ctx, NewStatement(SelectFooFromBar))
			var rowCount int
			err = iter.Do(func(*Row) error {
				rowCount++
				return nil
			})
			if err != nil {
				client.Close()
				t.Fatalf("Query() failed: %v", err)
			}
			if rowCount == 0 {
				client.Close()
				t.Fatalf("Query() returned 0 rows, want > 0")
			}

			// Closing the client flushes the built-in metrics PeriodicReader.
			client.Close()

			mu.Lock()
			gotUnexpected := append([]string(nil), unexpectedAddrs...)
			gotSpannerDials := dialedAddrs[wantSpannerAddr]
			gotMonitoringDials := dialedAddrs[wantMonitoringAddr]
			mu.Unlock()

			if len(gotUnexpected) > 0 {
				t.Errorf("client attempted to dial unexpected endpoints: %v", gotUnexpected)
			}
			if gotSpannerDials == 0 {
				t.Errorf("expected at least one dial to %q, got 0", wantSpannerAddr)
			}
			if gotMonitoringDials == 0 {
				t.Errorf("expected at least one dial to %q, got 0", wantMonitoringAddr)
			}

			reqs := monitoringServer.CreateServiceTimeSeriesRequests()
			if len(reqs) == 0 {
				t.Fatalf("expected CreateServiceTimeSeries requests at %q, got 0", wantMonitoringAddr)
			}
			wantAuth := "Bearer " + wantToken
			if authHeaders := monitoringServer.AuthHeaders(); len(authHeaders) == 0 || authHeaders[0] != wantAuth {
				t.Errorf("CreateServiceTimeSeries authorization header = %v, want [%q]", authHeaders, wantAuth)
			}
		})
	}
}

func TestMonitoringExporterEndpointAdditionalCases(t *testing.T) {
	tests := []struct {
		name               string
		envUniverseDomain  string
		envMonitoringHost  string
		envUseClientCert   string
		envUseMTLSEndpoint string
		envDisableNewAuth  string
		spannerOpts        []option.ClientOption
		wantTarget         string
		wantErr            bool
	}{
		{
			name:               "GOOGLE_CLOUD_UNIVERSE_DOMAIN with Spanner WithEndpoint",
			envUniverseDomain:  "example-universe.test",
			envUseClientCert:   "false",
			envUseMTLSEndpoint: "auto",
			spannerOpts: []option.ClientOption{
				option.WithoutAuthentication(),
				option.WithEndpoint("spanner.example-universe.test:443"),
			},
			wantTarget: "monitoring.example-universe.test:443",
		},
		{
			name:               "empty WithUniverseDomain falls back to GOOGLE_CLOUD_UNIVERSE_DOMAIN",
			envUniverseDomain:  "env-universe.test",
			envUseClientCert:   "false",
			envUseMTLSEndpoint: "auto",
			spannerOpts: []option.ClientOption{
				option.WithoutAuthentication(),
				option.WithUniverseDomain(""),
			},
			wantTarget: "monitoring.env-universe.test:443",
		},
		{
			name:               "empty WithUniverseDomain falls back to default GDU",
			envUseClientCert:   "false",
			envUseMTLSEndpoint: "auto",
			spannerOpts: []option.ClientOption{
				option.WithoutAuthentication(),
				option.WithUniverseDomain(""),
			},
			wantTarget: "monitoring.googleapis.com:443",
		},
		{
			name:               "multiple WithEndpoint options are all cleared",
			envUseClientCert:   "false",
			envUseMTLSEndpoint: "auto",
			spannerOpts: []option.ClientOption{
				option.WithoutAuthentication(),
				option.WithUniverseDomain("example-universe.test"),
				option.WithEndpoint("spanner-1.example-universe.test:443"),
				option.WithEndpoint("spanner-2.example-universe.test:443"),
			},
			wantTarget: "monitoring.example-universe.test:443",
		},
		{
			name:               "SPANNER_MONITORING_HOST overrides default GDU endpoint",
			envMonitoringHost:  "custom-monitoring.local:443",
			envUseClientCert:   "false",
			envUseMTLSEndpoint: "auto",
			spannerOpts: []option.ClientOption{
				option.WithoutAuthentication(),
			},
			wantTarget: "custom-monitoring.local:443",
		},
		{
			name:               "unhappy path: conflicting auth options fail",
			envUseClientCert:   "false",
			envUseMTLSEndpoint: "auto",
			spannerOpts: []option.ClientOption{
				option.WithoutAuthentication(),
				option.WithCredentials(&google.Credentials{ProjectID: "test-project"}),
			},
			wantErr: true,
		},
		{
			name:               "unhappy path: mTLS always mode with non-GDU universe fails under legacy auth",
			envUseClientCert:   "false",
			envUseMTLSEndpoint: "always",
			envDisableNewAuth:  "true",
			spannerOpts: []option.ClientOption{
				option.WithoutAuthentication(),
				option.WithUniverseDomain("example-universe.test"),
			},
			wantErr: true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv("GOOGLE_CLOUD_UNIVERSE_DOMAIN", tc.envUniverseDomain)
			t.Setenv("SPANNER_MONITORING_HOST", tc.envMonitoringHost)
			t.Setenv("GOOGLE_API_USE_CLIENT_CERTIFICATE", tc.envUseClientCert)
			t.Setenv("GOOGLE_API_USE_MTLS_ENDPOINT", tc.envUseMTLSEndpoint)
			t.Setenv("GOOGLE_API_GO_EXPERIMENTAL_DISABLE_NEW_AUTH_LIB", tc.envDisableNewAuth)

			ctx := context.Background()
			exporter, err := newMonitoringExporter(ctx, "test-project", "", nil, createExporterOptions(tc.spannerOpts...)...)
			if tc.wantErr {
				if err == nil {
					exporter.Shutdown(ctx)
					t.Fatal("newMonitoringExporter() succeeded, want error")
				}
				return
			}
			if err != nil {
				t.Fatalf("newMonitoringExporter() failed: %v", err)
			}
			defer exporter.Shutdown(ctx)

			if w, g := tc.wantTarget, exporter.client.Connection().Target(); w != g {
				t.Errorf("monitoring client target mismatch:\n Want: %v\n  Got: %v", w, g)
			}
		})
	}
}

func TestCreateExporterOptionsDoesNotMutateCallerSlice(t *testing.T) {
	t.Setenv("SPANNER_MONITORING_HOST", "")

	sentinel := option.WithUserAgent("caller-sentinel")
	backing := []option.ClientOption{
		option.WithUniverseDomain("example-universe.test"),
		sentinel,
		sentinel,
	}

	got := createExporterOptions(backing[:1]...)

	if w, g := 4, len(got); w != g {
		t.Fatalf("len(createExporterOptions) mismatch:\n Want: %v\n  Got: %v", w, g)
	}
	if w, g := sentinel, backing[1]; w != g {
		t.Errorf("caller backing slice was mutated:\n Want: %v\n  Got: %v", w, g)
	}
}

type trackingConnPool struct {
	*grpc.ClientConn
	closed bool
}

func (p *trackingConnPool) Conn() *grpc.ClientConn { return p.ClientConn }
func (p *trackingConnPool) Num() int               { return 1 }
func (p *trackingConnPool) Close() error {
	p.closed = true
	return p.ClientConn.Close()
}

func TestCreateExporterOptionsClearsSpannerConnAndPool(t *testing.T) {
	const customMonitoringAddr = "custom-monitoring.local:443"

	tests := []struct {
		name     string
		usePool  bool
		envMon   string
		wantAddr string
	}{
		{
			name:     "WithGRPCConn cleared when SPANNER_MONITORING_HOST is set",
			envMon:   customMonitoringAddr,
			wantAddr: customMonitoringAddr,
		},
		{
			name:     "WithConnPool cleared when SPANNER_MONITORING_HOST is set",
			usePool:  true,
			envMon:   customMonitoringAddr,
			wantAddr: customMonitoringAddr,
		},
		{
			name:     "WithGRPCConn cleared when SPANNER_MONITORING_HOST is empty",
			wantAddr: "monitoring.googleapis.com:443",
		},
		{
			name:     "WithConnPool cleared when SPANNER_MONITORING_HOST is empty",
			usePool:  true,
			wantAddr: "monitoring.googleapis.com:443",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv("GOOGLE_CLOUD_UNIVERSE_DOMAIN", "")
			t.Setenv("SPANNER_MONITORING_HOST", tc.envMon)
			t.Setenv("GOOGLE_API_USE_CLIENT_CERTIFICATE", "false")
			t.Setenv("GOOGLE_API_USE_MTLS_ENDPOINT", "auto")

			conn, err := grpc.NewClient("passthrough:///spanner-custom:443", grpc.WithTransportCredentials(insecure.NewCredentials()))
			if err != nil {
				t.Fatalf("grpc.NewClient() failed: %v", err)
			}
			defer conn.Close()

			pool := &trackingConnPool{ClientConn: conn}
			opts := []option.ClientOption{option.WithoutAuthentication()}
			if tc.usePool {
				opts = append(opts, gtransport.WithConnPool(pool))
			} else {
				opts = append(opts, option.WithGRPCConn(conn))
			}

			ctx := context.Background()
			exporter, err := newMonitoringExporter(ctx, "test-project", "", nil, createExporterOptions(opts...)...)
			if err != nil {
				t.Fatalf("newMonitoringExporter() failed: %v", err)
			}
			if got := exporter.client.Connection().Target(); got != tc.wantAddr {
				exporter.Shutdown(ctx)
				t.Fatalf("monitoring client target = %q, want %q", got, tc.wantAddr)
			}
			if err := exporter.Shutdown(ctx); err != nil {
				t.Fatalf("exporter.Shutdown() failed: %v", err)
			}
			if conn.GetState() == connectivity.Shutdown {
				t.Fatal("exporter.Shutdown() closed caller's Spanner grpc.ClientConn")
			}
			if tc.usePool && pool.closed {
				t.Fatal("exporter.Shutdown() closed caller's Spanner ConnPool")
			}
		})
	}
}

func TestBuiltinMetricsExporterAdditionalEndToEndAndErrorPaths(t *testing.T) {
	const universeDomain = "example-universe.test"
	const wantSpannerAddr = "spanner." + universeDomain + ":443"
	const customMonitoringAddr = "custom-monitoring.local:443"
	const wantToken = "test-universe-token"

	origDetectClientLocation := detectClientLocation
	detectClientLocation = func(context.Context) string { return "global" }
	defer func() { detectClientLocation = origDetectClientLocation }()

	creds := &google.Credentials{
		ProjectID:              "test-project",
		TokenSource:            oauth2.StaticTokenSource(&oauth2.Token{AccessToken: wantToken}),
		UniverseDomainProvider: func() (string, error) { return universeDomain, nil },
	}

	t.Run("GOOGLE_CLOUD_UNIVERSE_DOMAIN and Spanner WithEndpoint", func(t *testing.T) {
		t.Setenv("SPANNER_EMULATOR_HOST", "")
		t.Setenv("SPANNER_DISABLE_BUILTIN_METRICS", "")
		t.Setenv("SPANNER_MONITORING_HOST", "")
		t.Setenv("GOOGLE_CLOUD_DISABLE_DIRECT_PATH", "true")
		t.Setenv("GOOGLE_SPANNER_ENABLE_DIRECT_ACCESS", "false")
		t.Setenv("GOOGLE_API_USE_CLIENT_CERTIFICATE", "false")
		t.Setenv("GOOGLE_API_USE_MTLS_ENDPOINT", "auto")
		t.Setenv("GOOGLE_CLOUD_UNIVERSE_DOMAIN", universeDomain)

		wantMonitoringAddr := "monitoring." + universeDomain + ":443"
		spannerServer, _, teardownSpanner := NewMockedSpannerInMemTestServer(t)
		defer teardownSpanner()

		monitoringServer, err := NewMetricTestServer()
		if err != nil {
			t.Fatalf("NewMetricTestServer() failed: %v", err)
		}
		go monitoringServer.Serve()
		defer monitoringServer.Shutdown()

		var mu sync.Mutex
		dialedAddrs := map[string]int{}
		unexpectedAddrs := []string{}

		dialer := func(ctx context.Context, addr string) (net.Conn, error) {
			mu.Lock()
			dialedAddrs[addr]++
			mu.Unlock()

			var target string
			switch addr {
			case wantSpannerAddr:
				target = spannerServer.ServerAddress
			case wantMonitoringAddr:
				target = monitoringServer.Endpoint
			default:
				mu.Lock()
				unexpectedAddrs = append(unexpectedAddrs, addr)
				mu.Unlock()
				return nil, fmt.Errorf("unexpected dial to %q", addr)
			}
			return (&net.Dialer{}).DialContext(ctx, "tcp", target)
		}

		ctx := context.Background()
		client, err := NewClientWithConfig(ctx, "projects/test-project/instances/test-instance/databases/test-db", ClientConfig{
			NumChannels: 1,
		},
			option.WithCredentials(creds),
			option.WithEndpoint(wantSpannerAddr),
			option.WithGRPCDialOption(grpc.WithTransportCredentials(insecure.NewCredentials())),
			option.WithGRPCDialOption(grpc.WithResolvers(staticDNSResolverBuilder{})),
			option.WithGRPCDialOption(grpc.WithContextDialer(dialer)),
		)
		if err != nil {
			t.Fatalf("NewClientWithConfig() failed: %v", err)
		}

		iter := client.Single().Query(ctx, NewStatement(SelectFooFromBar))
		if err := iter.Do(func(*Row) error { return nil }); err != nil {
			client.Close()
			t.Fatalf("Query() failed: %v", err)
		}
		client.Close()

		mu.Lock()
		gotUnexpected := append([]string(nil), unexpectedAddrs...)
		gotMonitoringDials := dialedAddrs[wantMonitoringAddr]
		mu.Unlock()

		if w, g := 0, len(gotUnexpected); w != g {
			t.Errorf("unexpected dialed endpoints:\n Want: %v\n  Got: %v (%v)", w, g, gotUnexpected)
		}
		if gotMonitoringDials == 0 {
			t.Errorf("expected at least one dial to %q, got 0", wantMonitoringAddr)
		}
		if len(monitoringServer.CreateServiceTimeSeriesRequests()) == 0 {
			t.Fatalf("expected CreateServiceTimeSeries requests at %q, got 0", wantMonitoringAddr)
		}
	})

	t.Run("SPANNER_MONITORING_HOST override end-to-end", func(t *testing.T) {
		t.Setenv("SPANNER_EMULATOR_HOST", "")
		t.Setenv("SPANNER_DISABLE_BUILTIN_METRICS", "")
		t.Setenv("SPANNER_MONITORING_HOST", customMonitoringAddr)
		t.Setenv("GOOGLE_CLOUD_DISABLE_DIRECT_PATH", "true")
		t.Setenv("GOOGLE_SPANNER_ENABLE_DIRECT_ACCESS", "false")
		t.Setenv("GOOGLE_API_USE_CLIENT_CERTIFICATE", "false")
		t.Setenv("GOOGLE_API_USE_MTLS_ENDPOINT", "auto")
		t.Setenv("GOOGLE_CLOUD_UNIVERSE_DOMAIN", "")

		spannerServer, _, teardownSpanner := NewMockedSpannerInMemTestServer(t)
		defer teardownSpanner()

		monitoringServer, err := NewMetricTestServer()
		if err != nil {
			t.Fatalf("NewMetricTestServer() failed: %v", err)
		}
		go monitoringServer.Serve()
		defer monitoringServer.Shutdown()

		var mu sync.Mutex
		unexpectedAddrs := []string{}
		dialer := func(ctx context.Context, addr string) (net.Conn, error) {
			var target string
			switch addr {
			case wantSpannerAddr:
				target = spannerServer.ServerAddress
			case customMonitoringAddr:
				target = monitoringServer.Endpoint
			default:
				mu.Lock()
				unexpectedAddrs = append(unexpectedAddrs, addr)
				mu.Unlock()
				return nil, fmt.Errorf("unexpected dial to %q", addr)
			}
			return (&net.Dialer{}).DialContext(ctx, "tcp", target)
		}

		ctx := context.Background()
		client, err := NewClientWithConfig(ctx, "projects/test-project/instances/test-instance/databases/test-db", ClientConfig{
			NumChannels: 1,
		},
			option.WithCredentials(creds),
			option.WithUniverseDomain(universeDomain),
			option.WithEndpoint(wantSpannerAddr),
			option.WithGRPCDialOption(grpc.WithTransportCredentials(insecure.NewCredentials())),
			option.WithGRPCDialOption(grpc.WithResolvers(staticDNSResolverBuilder{})),
			option.WithGRPCDialOption(grpc.WithContextDialer(dialer)),
		)
		if err != nil {
			t.Fatalf("NewClientWithConfig() failed: %v", err)
		}

		iter := client.Single().Query(ctx, NewStatement(SelectFooFromBar))
		if err := iter.Do(func(*Row) error { return nil }); err != nil {
			client.Close()
			t.Fatalf("Query() failed: %v", err)
		}
		client.Close()

		mu.Lock()
		gotUnexpected := append([]string(nil), unexpectedAddrs...)
		mu.Unlock()

		if w, g := 0, len(gotUnexpected); w != g {
			t.Errorf("unexpected dialed endpoints:\n Want: %v\n  Got: %v (%v)", w, g, gotUnexpected)
		}
		if len(monitoringServer.CreateServiceTimeSeriesRequests()) == 0 {
			t.Fatalf("expected CreateServiceTimeSeries requests at %q, got 0", customMonitoringAddr)
		}
		wantAuth := "Bearer " + wantToken
		for _, auth := range monitoringServer.AuthHeaders() {
			if w, g := wantAuth, auth; w != g {
				t.Errorf("CreateServiceTimeSeries authorization header mismatch:\n Want: %v\n  Got: %v", w, g)
			}
		}
	})

	t.Run("unhappy path: NewClientWithConfig propagates monitoring exporter creation error", func(t *testing.T) {
		t.Setenv("SPANNER_EMULATOR_HOST", "")
		t.Setenv("SPANNER_DISABLE_BUILTIN_METRICS", "")
		t.Setenv("SPANNER_MONITORING_HOST", "")

		// Keep the Spanner client authenticated so native metrics initialization
		// executes, while forcing exporter creation to fail with conflicting auth
		// options so metrics.go's exporter-error return path stays exercised.
		origCreateExporterOptions := createExporterOptions
		createOptionsCalls := 0
		createExporterOptions = func(spannerOpts ...option.ClientOption) []option.ClientOption {
			createOptionsCalls++
			return append(origCreateExporterOptions(spannerOpts...), option.WithoutAuthentication())
		}
		t.Cleanup(func() { createExporterOptions = origCreateExporterOptions })

		ctx := context.Background()
		client, err := NewClientWithConfig(ctx, "projects/test-project/instances/test-instance/databases/test-db", ClientConfig{
			NumChannels: 1,
		},
			option.WithCredentials(creds),
			option.WithUniverseDomain(universeDomain),
		)
		if err == nil {
			client.Close()
			t.Fatal("NewClientWithConfig() succeeded, want monitoring exporter creation error")
		}
		if createOptionsCalls != 1 {
			t.Fatalf("createExporterOptions call count = %d, want 1", createOptionsCalls)
		}
	})
}

func TestMonitoringExporterUniverseDomainMismatch(t *testing.T) {
	tests := []struct {
		name           string
		clientUniverse string
		credsUniverse  string
		wantTarget     string
	}{
		{
			name:           "non-GDU client with mismatched non-GDU credentials",
			clientUniverse: "example-universe.test",
			credsUniverse:  "other-universe.test",
			wantTarget:     "monitoring.example-universe.test:443",
		},
		{
			name:           "default GDU client with non-GDU credentials",
			clientUniverse: "",
			credsUniverse:  "example-universe.test",
			wantTarget:     "monitoring.googleapis.com:443",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv("GOOGLE_CLOUD_UNIVERSE_DOMAIN", "")
			t.Setenv("SPANNER_MONITORING_HOST", "")
			t.Setenv("GOOGLE_API_USE_CLIENT_CERTIFICATE", "false")
			t.Setenv("GOOGLE_API_USE_MTLS_ENDPOINT", "auto")

			monitoringServer, err := NewMetricTestServer()
			if err != nil {
				t.Fatalf("NewMetricTestServer() failed: %v", err)
			}
			go monitoringServer.Serve()
			defer monitoringServer.Shutdown()

			dialer := func(ctx context.Context, addr string) (net.Conn, error) {
				if addr != tc.wantTarget {
					return nil, fmt.Errorf("unexpected dial to %q, want %q", addr, tc.wantTarget)
				}
				return (&net.Dialer{}).DialContext(ctx, "tcp", monitoringServer.Endpoint)
			}

			creds := &google.Credentials{
				ProjectID:              "test-project",
				TokenSource:            oauth2.StaticTokenSource(&oauth2.Token{AccessToken: "secret-token"}),
				UniverseDomainProvider: func() (string, error) { return tc.credsUniverse, nil },
			}

			opts := []option.ClientOption{
				option.WithCredentials(creds),
				option.WithGRPCDialOption(grpc.WithTransportCredentials(insecure.NewCredentials())),
				option.WithGRPCDialOption(grpc.WithResolvers(staticDNSResolverBuilder{})),
				option.WithGRPCDialOption(grpc.WithContextDialer(dialer)),
			}
			if tc.clientUniverse != "" {
				opts = append(opts, option.WithUniverseDomain(tc.clientUniverse))
			}

			ctx := context.Background()
			exporter, err := newMonitoringExporter(ctx, "test-project", "", nil, createExporterOptions(opts...)...)
			if err != nil {
				t.Fatalf("newMonitoringExporter() failed: %v", err)
			}
			defer exporter.Shutdown(ctx)

			err = exporter.client.CreateServiceTimeSeries(ctx, &monitoringpb.CreateTimeSeriesRequest{
				Name: "projects/test-project",
			})
			if err == nil {
				t.Fatal("CreateServiceTimeSeries() succeeded with mismatched universe domains, want error")
			}

			if w, g := 0, len(monitoringServer.CreateServiceTimeSeriesRequests()); w != g {
				t.Errorf("received requests on universe mismatch:\n Want: %v\n  Got: %v", w, g)
			}
			if w, g := 0, len(monitoringServer.AuthHeaders()); w != g {
				t.Errorf("received authorization headers on universe mismatch:\n Want: %v\n  Got: %v", w, g)
			}
		})
	}
}

func TestMonitoringExporterIsDefaultSpannerEndpoint(t *testing.T) {
	tests := []struct {
		name           string
		endpoint       string
		universeDomain string
		want           bool
	}{
		// Accepted default endpoints.
		{name: "empty endpoint in GDU", endpoint: "", universeDomain: "googleapis.com", want: true},
		{name: "empty endpoint in custom universe", endpoint: "", universeDomain: "example-universe.test", want: true},
		{name: "bare default GDU host", endpoint: "spanner.googleapis.com", universeDomain: "googleapis.com", want: true},
		{name: "default GDU host with port 443", endpoint: "spanner.googleapis.com:443", universeDomain: "googleapis.com", want: true},
		{name: "uppercase default GDU host with port 443", endpoint: "SPANNER.GOOGLEAPIS.COM:443", universeDomain: "googleapis.com", want: true},
		{name: "https default GDU host", endpoint: "https://spanner.googleapis.com", universeDomain: "googleapis.com", want: true},
		{name: "https default GDU host with trailing slash", endpoint: "https://spanner.googleapis.com/", universeDomain: "googleapis.com", want: true},
		{name: "https default GDU host with port 443", endpoint: "https://spanner.googleapis.com:443", universeDomain: "googleapis.com", want: true},
		{name: "https default GDU host with port 443 and trailing slash", endpoint: "https://spanner.googleapis.com:443/", universeDomain: "googleapis.com", want: true},
		{name: "uppercase https default GDU host", endpoint: "HTTPS://SPANNER.GOOGLEAPIS.COM:443/", universeDomain: "googleapis.com", want: true},
		{name: "dns empty authority default GDU host", endpoint: "dns:///spanner.googleapis.com", universeDomain: "googleapis.com", want: true},
		{name: "dns empty authority default GDU host with port 443", endpoint: "dns:///spanner.googleapis.com:443", universeDomain: "googleapis.com", want: true},
		{name: "uppercase dns default GDU host", endpoint: "DNS:///SPANNER.GOOGLEAPIS.COM:443", universeDomain: "googleapis.com", want: true},
		{name: "bare default custom universe host", endpoint: "spanner.example-universe.test", universeDomain: "example-universe.test", want: true},
		{name: "default custom universe host with port 443", endpoint: "spanner.example-universe.test:443", universeDomain: "example-universe.test", want: true},
		{name: "https default custom universe host", endpoint: "https://spanner.example-universe.test:443/", universeDomain: "example-universe.test", want: true},
		{name: "dns default custom universe host", endpoint: "dns:///spanner.example-universe.test:443", universeDomain: "example-universe.test", want: true},

		// Rejected non-default and malformed endpoints.
		{name: "regional endpoint GDU with port 443", endpoint: "spanner.us-central1.rep.googleapis.com:443", universeDomain: "googleapis.com", want: false},
		{name: "regional endpoint GDU bare", endpoint: "spanner.us-central1.rep.googleapis.com", universeDomain: "googleapis.com", want: false},
		{name: "regional endpoint custom universe", endpoint: "spanner.us-central1.rep.example-universe.test:443", universeDomain: "example-universe.test", want: false},
		{name: "explicit mTLS endpoint GDU", endpoint: "spanner.mtls.googleapis.com:443", universeDomain: "googleapis.com", want: false},
		{name: "custom PSC or proxy host", endpoint: "psc-spanner.internal:443", universeDomain: "googleapis.com", want: false},
		{name: "localhost custom port", endpoint: "localhost:9010", universeDomain: "googleapis.com", want: false},
		{name: "default host with non-443 port", endpoint: "spanner.googleapis.com:8443", universeDomain: "googleapis.com", want: false},
		{name: "default host with empty port", endpoint: "spanner.googleapis.com:", universeDomain: "googleapis.com", want: false},
		{name: "insecure http scheme", endpoint: "http://spanner.googleapis.com:443", universeDomain: "googleapis.com", want: false},
		{name: "passthrough scheme", endpoint: "passthrough:///spanner.googleapis.com:443", universeDomain: "googleapis.com", want: false},
		{name: "triple slash https scheme", endpoint: "https:///spanner.googleapis.com:443", universeDomain: "googleapis.com", want: false},
		{name: "https double trailing slash", endpoint: "https://spanner.googleapis.com:443//", universeDomain: "googleapis.com", want: false},
		{name: "dns with custom authority", endpoint: "dns://8.8.8.8/spanner.googleapis.com:443", universeDomain: "googleapis.com", want: false},
		{name: "dns with extra leading slash", endpoint: "dns:////spanner.googleapis.com:443", universeDomain: "googleapis.com", want: false},
		{name: "https with non-root path", endpoint: "https://spanner.googleapis.com/v1", universeDomain: "googleapis.com", want: false},
		{name: "https with query", endpoint: "https://spanner.googleapis.com?a=1", universeDomain: "googleapis.com", want: false},
		{name: "https with empty query", endpoint: "https://spanner.googleapis.com?", universeDomain: "googleapis.com", want: false},
		{name: "https with fragment", endpoint: "https://spanner.googleapis.com#frag", universeDomain: "googleapis.com", want: false},
		{name: "https with userinfo", endpoint: "https://user@spanner.googleapis.com:443", universeDomain: "googleapis.com", want: false},
		{name: "bracketed host", endpoint: "[spanner.googleapis.com]:443", universeDomain: "googleapis.com", want: false},
		{name: "whitespace endpoint", endpoint: "   ", universeDomain: "googleapis.com", want: false},
		{name: "untrimmed endpoint", endpoint: " spanner.googleapis.com:443 ", universeDomain: "googleapis.com", want: false},
		{name: "suffix spoofed host", endpoint: "spanner.googleapis.com.evil.com:443", universeDomain: "googleapis.com", want: false},
		{name: "cross-universe mismatch", endpoint: "spanner.googleapis.com:443", universeDomain: "example-universe.test", want: false},
		{name: "empty universe with explicit endpoint", endpoint: "spanner.googleapis.com:443", universeDomain: "", want: false},
		{name: "untrimmed universe with explicit endpoint", endpoint: "spanner.googleapis.com:443", universeDomain: " googleapis.com ", want: false},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := isDefaultSpannerEndpoint(tc.endpoint, tc.universeDomain); got != tc.want {
				t.Errorf("isDefaultSpannerEndpoint(%q, %q) = %v, want %v", tc.endpoint, tc.universeDomain, got, tc.want)
			}
		})
	}
}

func TestMonitoringExporterShouldEnableDefault(t *testing.T) {
	conn, err := grpc.NewClient("passthrough:///unused", grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		t.Fatalf("grpc.NewClient() failed: %v", err)
	}
	defer conn.Close()
	pool := &trackingConnPool{ClientConn: conn}

	tests := []struct {
		name               string
		envUniverseDomain  string
		envMonitoringHost  string
		envUseClientCert   string
		envUseMTLSEndpoint string
		envDisableNewAuth  string
		opts               []option.ClientOption
		want               bool
	}{
		{
			name: "unset endpoint in default GDU",
			want: true,
		},
		{
			name: "explicit default GDU endpoint",
			opts: []option.ClientOption{option.WithEndpoint("spanner.googleapis.com:443")},
			want: true,
		},
		{
			name: "custom endpoint disables default export",
			opts: []option.ClientOption{option.WithEndpoint("psc-spanner.internal:443")},
			want: false,
		},
		{
			name: "wrapped custom endpoint disables default export",
			opts: []option.ClientOption{wrappedEndpointOption{option.WithEndpoint("psc-spanner.internal:443")}},
			want: false,
		},
		{
			name: "last WithEndpoint wins when default overrides custom",
			opts: []option.ClientOption{
				option.WithEndpoint("psc-spanner.internal:443"),
				option.WithEndpoint("spanner.googleapis.com:443"),
			},
			want: true,
		},
		{
			name: "last WithEndpoint wins when custom overrides default",
			opts: []option.ClientOption{
				option.WithEndpoint("spanner.googleapis.com:443"),
				option.WithEndpoint("psc-spanner.internal:443"),
			},
			want: false,
		},
		{
			name: "empty WithEndpoint resets prior custom endpoint",
			opts: []option.ClientOption{
				option.WithEndpoint("psc-spanner.internal:443"),
				option.WithEndpoint(""),
			},
			want: true,
		},
		{
			name: "nil option is safely ignored",
			opts: []option.ClientOption{nil, option.WithEndpoint("spanner.googleapis.com:443")},
			want: true,
		},
		{
			name: "custom WithGRPCConn disables default export",
			opts: []option.ClientOption{option.WithGRPCConn(conn)},
			want: false,
		},
		{
			name: "WithGRPCConn(nil) resets prior custom conn",
			opts: []option.ClientOption{option.WithGRPCConn(conn), option.WithGRPCConn(nil)},
			want: true,
		},
		{
			name: "custom WithConnPool disables default export",
			opts: []option.ClientOption{gtransport.WithConnPool(pool)},
			want: false,
		},
		{
			name: "wrapped custom WithConnPool disables default export",
			opts: []option.ClientOption{wrappedEndpointOption{gtransport.WithConnPool(pool)}},
			want: false,
		},
		{
			name: "WithConnPool(nil) resets prior custom conn pool",
			opts: []option.ClientOption{gtransport.WithConnPool(pool), gtransport.WithConnPool(nil)},
			want: true,
		},
		{
			name:              "custom WithConnPool disables default export under legacy auth",
			envDisableNewAuth: "true",
			opts:              []option.ClientOption{gtransport.WithConnPool(pool)},
			want:              false,
		},
		{
			name:              "unset conn pool stays enabled under legacy auth",
			envDisableNewAuth: "true",
			want:              true,
		},
		{
			name:              "SPANNER_MONITORING_HOST enables export for custom WithConnPool",
			envMonitoringHost: "custom-monitoring.local:443",
			opts:              []option.ClientOption{gtransport.WithConnPool(pool)},
			want:              true,
		},
		{
			name:              "WithUniverseDomain overrides env and matches explicit default",
			envUniverseDomain: "env-universe.test",
			opts: []option.ClientOption{
				option.WithUniverseDomain("opt-universe.test"),
				option.WithEndpoint("spanner.opt-universe.test:443"),
			},
			want: true,
		},
		{
			name:              "WithUniverseDomain overrides env and rejects mismatched endpoint",
			envUniverseDomain: "env-universe.test",
			opts: []option.ClientOption{
				option.WithUniverseDomain("opt-universe.test"),
				option.WithEndpoint("spanner.env-universe.test:443"),
			},
			want: false,
		},
		{
			name:              "GOOGLE_CLOUD_UNIVERSE_DOMAIN matches explicit default",
			envUniverseDomain: "example-universe.test",
			opts:              []option.ClientOption{option.WithEndpoint("spanner.example-universe.test:443")},
			want:              true,
		},
		{
			name:               "non-GDU with unset endpoint stays enabled under mTLS always mode",
			envUseMTLSEndpoint: "always",
			opts:               []option.ClientOption{option.WithUniverseDomain("example-universe.test")},
			want:               true,
		},
		{
			name:             "client certificate enabled with custom endpoint still fails closed",
			envUseClientCert: "true",
			opts:             []option.ClientOption{option.WithEndpoint("psc-spanner.internal:443")},
			want:             false,
		},
		{
			name:              "SPANNER_MONITORING_HOST enables export for custom endpoint",
			envMonitoringHost: "custom-monitoring.local:443",
			opts:              []option.ClientOption{option.WithEndpoint("psc-spanner.internal:443")},
			want:              true,
		},
		{
			name:              "SPANNER_MONITORING_HOST enables export for custom WithGRPCConn",
			envMonitoringHost: "custom-monitoring.local:443",
			opts:              []option.ClientOption{option.WithGRPCConn(conn)},
			want:              true,
		},
		{
			name:              "whitespace-only SPANNER_MONITORING_HOST does not override custom endpoint",
			envMonitoringHost: "   ",
			opts:              []option.ClientOption{option.WithEndpoint("psc-spanner.internal:443")},
			want:              false,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv("GOOGLE_CLOUD_UNIVERSE_DOMAIN", tc.envUniverseDomain)
			t.Setenv("SPANNER_MONITORING_HOST", tc.envMonitoringHost)
			if tc.envUseClientCert == "" {
				t.Setenv("GOOGLE_API_USE_CLIENT_CERTIFICATE", "false")
			} else {
				t.Setenv("GOOGLE_API_USE_CLIENT_CERTIFICATE", tc.envUseClientCert)
			}
			if tc.envUseMTLSEndpoint == "" {
				t.Setenv("GOOGLE_API_USE_MTLS_ENDPOINT", "auto")
			} else {
				t.Setenv("GOOGLE_API_USE_MTLS_ENDPOINT", tc.envUseMTLSEndpoint)
			}
			t.Setenv("GOOGLE_API_GO_EXPERIMENTAL_DISABLE_NEW_AUTH_LIB", tc.envDisableNewAuth)

			if got := shouldEnableDefaultMonitoringExporter(tc.opts...); got != tc.want {
				t.Errorf("shouldEnableDefaultMonitoringExporter() = %v, want %v", got, tc.want)
			}
		})
	}
}

type builtinMetricsE2EHarness struct {
	spannerServer    *MockedSpannerInMemTestServer
	monitoringServer *MetricsTestServer
	creds            *google.Credentials
	dialer           func(context.Context, string) (net.Conn, error)

	mu              sync.Mutex
	dialedAddrs     map[string]int
	unexpectedAddrs []string
}

func newBuiltinMetricsE2EHarness(t *testing.T, universeDomain string, spannerAddrs, monitoringAddrs []string, token string) *builtinMetricsE2EHarness {
	t.Helper()

	origDetectClientLocation := detectClientLocation
	detectClientLocation = func(context.Context) string { return "global" }
	t.Cleanup(func() { detectClientLocation = origDetectClientLocation })

	t.Setenv("SPANNER_EMULATOR_HOST", "")
	t.Setenv("SPANNER_DISABLE_BUILTIN_METRICS", "")
	t.Setenv("SPANNER_MONITORING_HOST", "")
	t.Setenv("GOOGLE_CLOUD_DISABLE_DIRECT_PATH", "true")
	t.Setenv("GOOGLE_SPANNER_ENABLE_DIRECT_ACCESS", "false")
	t.Setenv("GOOGLE_API_USE_CLIENT_CERTIFICATE", "false")
	t.Setenv("GOOGLE_API_USE_MTLS_ENDPOINT", "auto")
	t.Setenv("GOOGLE_CLOUD_UNIVERSE_DOMAIN", "")

	spannerServer, _, teardownSpanner := NewMockedSpannerInMemTestServer(t)
	t.Cleanup(teardownSpanner)

	monitoringServer, err := NewMetricTestServer()
	if err != nil {
		t.Fatalf("NewMetricTestServer() failed: %v", err)
	}
	go monitoringServer.Serve()
	t.Cleanup(monitoringServer.Shutdown)

	spannerSet := make(map[string]bool, len(spannerAddrs))
	for _, addr := range spannerAddrs {
		if addr != "" {
			spannerSet[addr] = true
		}
	}
	monitoringSet := make(map[string]bool, len(monitoringAddrs))
	for _, addr := range monitoringAddrs {
		if addr != "" {
			monitoringSet[addr] = true
		}
	}

	h := &builtinMetricsE2EHarness{
		spannerServer:    spannerServer,
		monitoringServer: monitoringServer,
		dialedAddrs:      map[string]int{},
		creds: &google.Credentials{
			ProjectID:              "test-project",
			TokenSource:            oauth2.StaticTokenSource(&oauth2.Token{AccessToken: token}),
			UniverseDomainProvider: func() (string, error) { return universeDomain, nil },
		},
	}
	h.dialer = func(ctx context.Context, addr string) (net.Conn, error) {
		h.mu.Lock()
		h.dialedAddrs[addr]++
		h.mu.Unlock()
		switch {
		case spannerSet[addr]:
			return (&net.Dialer{}).DialContext(ctx, "tcp", spannerServer.ServerAddress)
		case monitoringSet[addr]:
			return (&net.Dialer{}).DialContext(ctx, "tcp", monitoringServer.Endpoint)
		default:
			h.mu.Lock()
			h.unexpectedAddrs = append(h.unexpectedAddrs, addr)
			h.mu.Unlock()
			return nil, fmt.Errorf("unexpected dial to %q", addr)
		}
	}
	return h
}

func (h *builtinMetricsE2EHarness) baseClientOpts() []option.ClientOption {
	return []option.ClientOption{
		option.WithCredentials(h.creds),
		option.WithGRPCDialOption(grpc.WithTransportCredentials(insecure.NewCredentials())),
		option.WithGRPCDialOption(grpc.WithResolvers(staticDNSResolverBuilder{})),
		option.WithGRPCDialOption(grpc.WithContextDialer(h.dialer)),
	}
}

func (h *builtinMetricsE2EHarness) dialCount(addr string) int {
	h.mu.Lock()
	defer h.mu.Unlock()
	return h.dialedAddrs[addr]
}

func (h *builtinMetricsE2EHarness) unexpectedDials() []string {
	h.mu.Lock()
	defer h.mu.Unlock()
	return append([]string(nil), h.unexpectedAddrs...)
}

func runMetricsE2EQueryAndClose(t *testing.T, ctx context.Context, client *Client) {
	t.Helper()
	iter := client.Single().Query(ctx, NewStatement(SelectFooFromBar))
	if err := iter.Do(func(*Row) error { return nil }); err != nil {
		client.Close()
		t.Fatalf("Query() failed: %v", err)
	}
	client.Close()
}

func TestBuiltinMetricsExporterCustomEndpointDisablesDefaultCloudMonitoring(t *testing.T) {
	tests := []struct {
		name           string
		universeDomain string
		spannerAddr    string
		useWrapped     bool
		useCustomConn  bool
		useCustomPool  bool
	}{
		{
			name:           "custom PSC endpoint in GDU",
			universeDomain: "googleapis.com",
			spannerAddr:    "psc-spanner.internal:443",
		},
		{
			name:           "regional endpoint in GDU",
			universeDomain: "googleapis.com",
			spannerAddr:    "spanner.us-central1.rep.googleapis.com:443",
		},
		{
			name:           "wrapped custom endpoint in GDU",
			universeDomain: "googleapis.com",
			spannerAddr:    "psc-spanner.internal:443",
			useWrapped:     true,
		},
		{
			name:           "cross-universe endpoint mismatch",
			universeDomain: "example-universe.test",
			spannerAddr:    "spanner.googleapis.com:443",
		},
		{
			name:           "custom pre-dialed WithGRPCConn",
			universeDomain: "googleapis.com",
			useCustomConn:  true,
		},
		{
			name:           "custom pre-dialed WithConnPool",
			universeDomain: "googleapis.com",
			useCustomPool:  true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			monitoringAddr := "monitoring." + tc.universeDomain + ":443"
			h := newBuiltinMetricsE2EHarness(t, tc.universeDomain, []string{tc.spannerAddr}, []string{monitoringAddr}, "secret-token")

			var opts []option.ClientOption
			if tc.useCustomConn || tc.useCustomPool {
				conn, err := grpc.NewClient(
					h.spannerServer.ServerAddress,
					grpc.WithTransportCredentials(insecure.NewCredentials()),
				)
				if err != nil {
					t.Fatalf("grpc.NewClient() failed: %v", err)
				}
				defer conn.Close()
				if tc.useCustomPool {
					opts = []option.ClientOption{
						gtransport.WithConnPool(&trackingConnPool{ClientConn: conn}),
					}
				} else {
					opts = []option.ClientOption{
						option.WithGRPCConn(conn),
					}
				}
			} else {
				opts = h.baseClientOpts()
				if tc.universeDomain != "googleapis.com" {
					opts = append(opts, option.WithUniverseDomain(tc.universeDomain))
				}
				epOpt := option.WithEndpoint(tc.spannerAddr)
				if tc.useWrapped {
					epOpt = wrappedEndpointOption{epOpt}
				}
				opts = append(opts, epOpt)
			}

			ctx := context.Background()
			client, err := NewClientWithConfig(ctx, "projects/test-project/instances/test-instance/databases/test-db", ClientConfig{
				NumChannels: 1,
			}, opts...)
			if err != nil {
				t.Fatalf("NewClientWithConfig() failed: %v", err)
			}

			if client.metricsTracerFactory.enabled {
				client.Close()
				t.Fatal("metricsTracerFactory.enabled = true, want false")
			}
			if client.metricsTracerFactory.meterProvider != nil {
				client.Close()
				t.Fatal("metricsTracerFactory.meterProvider != nil, want nil")
			}

			runMetricsE2EQueryAndClose(t, ctx, client)

			if got := h.dialCount(monitoringAddr); got != 0 {
				t.Errorf("dialed %q %d times, want 0", monitoringAddr, got)
			}
			if got := h.unexpectedDials(); len(got) != 0 {
				t.Errorf("unexpected dialed endpoints: %v", got)
			}
			if reqs := h.monitoringServer.CreateServiceTimeSeriesRequests(); len(reqs) != 0 {
				t.Errorf("received %d CreateServiceTimeSeries requests, want 0", len(reqs))
			}
		})
	}
}

func TestBuiltinMetricsExporterCustomEndpointEnabledWithMonitoringHost(t *testing.T) {
	const customSpannerAddr = "psc-spanner.internal:443"
	const customMonitoringAddr = "custom-monitoring.local:443"
	const wantToken = "custom-monitoring-token"

	h := newBuiltinMetricsE2EHarness(t, "googleapis.com", []string{customSpannerAddr}, []string{customMonitoringAddr}, wantToken)
	t.Setenv("SPANNER_MONITORING_HOST", customMonitoringAddr)

	ctx := context.Background()
	opts := append(h.baseClientOpts(), option.WithEndpoint(customSpannerAddr))
	client, err := NewClientWithConfig(ctx, "projects/test-project/instances/test-instance/databases/test-db", ClientConfig{
		NumChannels: 1,
	}, opts...)
	if err != nil {
		t.Fatalf("NewClientWithConfig() failed: %v", err)
	}

	runMetricsE2EQueryAndClose(t, ctx, client)

	if got := h.unexpectedDials(); len(got) != 0 {
		t.Errorf("unexpected dialed endpoints: %v", got)
	}
	if got := h.dialCount("monitoring.googleapis.com:443"); got != 0 {
		t.Errorf("dialed default monitoring.googleapis.com:443 %d times, want 0", got)
	}
	if got := h.dialCount(customMonitoringAddr); got == 0 {
		t.Errorf("expected at least one dial to %q, got 0", customMonitoringAddr)
	}
	if len(h.monitoringServer.CreateServiceTimeSeriesRequests()) == 0 {
		t.Fatalf("expected CreateServiceTimeSeries requests at %q, got 0", customMonitoringAddr)
	}
	wantAuth := "Bearer " + wantToken
	if authHeaders := h.monitoringServer.AuthHeaders(); len(authHeaders) == 0 || authHeaders[0] != wantAuth {
		t.Errorf("CreateServiceTimeSeries authorization header = %v, want [%q]", authHeaders, wantAuth)
	}
}

func TestBuiltinMetricsExporterExplicitDefaultEndpointRemainsEnabled(t *testing.T) {
	tests := []struct {
		name           string
		universeDomain string
		spannerOptAddr string
	}{
		{
			name:           "explicit default GDU endpoint with port 443",
			universeDomain: "googleapis.com",
			spannerOptAddr: "spanner.googleapis.com:443",
		},
		{
			name:           "explicit bare default GDU endpoint",
			universeDomain: "googleapis.com",
			spannerOptAddr: "spanner.googleapis.com",
		},
		{
			name:           "explicit default custom universe endpoint",
			universeDomain: "example-universe.test",
			spannerOptAddr: "spanner.example-universe.test:443",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			wantMonitoringAddr := "monitoring." + tc.universeDomain + ":443"
			wantSpannerHost := "spanner." + tc.universeDomain
			h := newBuiltinMetricsE2EHarness(t, tc.universeDomain, []string{wantSpannerHost, wantSpannerHost + ":443"}, []string{wantMonitoringAddr}, "default-endpoint-token")

			opts := append(h.baseClientOpts(), option.WithEndpoint(tc.spannerOptAddr))
			if tc.universeDomain != "googleapis.com" {
				opts = append(opts, option.WithUniverseDomain(tc.universeDomain))
			}

			ctx := context.Background()
			client, err := NewClientWithConfig(ctx, "projects/test-project/instances/test-instance/databases/test-db", ClientConfig{
				NumChannels: 1,
			}, opts...)
			if err != nil {
				t.Fatalf("NewClientWithConfig() failed: %v", err)
			}

			if !client.metricsTracerFactory.enabled || client.metricsTracerFactory.meterProvider == nil {
				client.Close()
				t.Fatalf("expected enabled native metrics for explicit default endpoint %q", tc.spannerOptAddr)
			}

			runMetricsE2EQueryAndClose(t, ctx, client)

			if got := h.unexpectedDials(); len(got) != 0 {
				t.Errorf("unexpected dialed endpoints: %v", got)
			}
			if got := h.dialCount(wantMonitoringAddr); got == 0 {
				t.Errorf("expected at least one dial to %q, got 0", wantMonitoringAddr)
			}
			if len(h.monitoringServer.CreateServiceTimeSeriesRequests()) == 0 {
				t.Fatalf("expected CreateServiceTimeSeries requests at %q, got 0", wantMonitoringAddr)
			}
		})
	}
}

func TestBuiltinMetricsExporterMultiEndpointClientDisablesDefaultCloudMonitoring(t *testing.T) {
	const defaultSpannerAddr = "spanner.googleapis.com:443"
	const defaultMonitoringAddr = "monitoring.googleapis.com:443"
	const customMonitoringAddr = "custom-monitoring.local:443"

	tests := []struct {
		name                 string
		envMonitoringHost    string
		withClientProvider   bool
		wantEnabled          bool
		wantNativeProvider   bool
		wantMonitoringDials  bool
		wantCustomMonitoring bool
	}{
		{
			name:                "disabled by default without SPANNER_MONITORING_HOST even with default Spanner endpoint",
			wantEnabled:         false,
			wantNativeProvider:  false,
			wantMonitoringDials: false,
		},
		{
			name:                "preserves ClientMetricsProvider when default Cloud Monitoring is disabled",
			withClientProvider:  true,
			wantEnabled:         true,
			wantNativeProvider:  false,
			wantMonitoringDials: false,
		},
		{
			name:                 "enabled when SPANNER_MONITORING_HOST is set",
			envMonitoringHost:    customMonitoringAddr,
			wantEnabled:          true,
			wantNativeProvider:   true,
			wantMonitoringDials:  true,
			wantCustomMonitoring: true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			monitoringAddrs := []string{defaultMonitoringAddr}
			if tc.wantCustomMonitoring {
				monitoringAddrs = append(monitoringAddrs, customMonitoringAddr)
			}
			h := newBuiltinMetricsE2EHarness(t, "googleapis.com", []string{defaultSpannerAddr}, monitoringAddrs, "gme-token")
			t.Setenv("SPANNER_MONITORING_HOST", tc.envMonitoringHost)

			gmeCfg := &grpcgcp.GCPMultiEndpointOptions{
				MultiEndpoints: map[string]*multiendpoint.MultiEndpointOptions{
					"default": {
						Endpoints: []string{defaultSpannerAddr},
					},
				},
				Default: "default",
			}

			cfg := ClientConfig{}
			reader, provider := newTestMeterProvider()
			defer provider.Shutdown(context.Background())
			if tc.withClientProvider {
				cfg.ClientMetricsProvider = provider
			}

			ctx := context.Background()
			client, _, err := NewMultiEndpointClientWithConfig(
				ctx,
				"projects/test-project/instances/test-instance/databases/test-db",
				cfg,
				gmeCfg,
				h.baseClientOpts()...,
			)
			if err != nil {
				t.Fatalf("NewMultiEndpointClientWithConfig() failed: %v", err)
			}

			if client.metricsTracerFactory.enabled != tc.wantEnabled {
				client.Close()
				t.Fatalf("metricsTracerFactory.enabled = %v, want %v", client.metricsTracerFactory.enabled, tc.wantEnabled)
			}
			if (client.metricsTracerFactory.meterProvider != nil) != tc.wantNativeProvider {
				client.Close()
				t.Fatalf("has native meterProvider = %v, want %v", client.metricsTracerFactory.meterProvider != nil, tc.wantNativeProvider)
			}

			runMetricsE2EQueryAndClose(t, ctx, client)

			if got := h.unexpectedDials(); len(got) != 0 {
				t.Errorf("unexpected dialed endpoints: %v", got)
			}
			if got := h.dialCount(defaultMonitoringAddr); got != 0 {
				t.Errorf("dialed default %q %d times, want 0", defaultMonitoringAddr, got)
			}
			if tc.wantMonitoringDials {
				if got := h.dialCount(customMonitoringAddr); got == 0 {
					t.Errorf("expected at least one dial to %q, got 0", customMonitoringAddr)
				}
				if len(h.monitoringServer.CreateServiceTimeSeriesRequests()) == 0 {
					t.Fatalf("expected CreateServiceTimeSeries requests at %q, got 0", customMonitoringAddr)
				}
			} else if reqs := h.monitoringServer.CreateServiceTimeSeriesRequests(); len(reqs) != 0 {
				t.Errorf("received %d CreateServiceTimeSeries requests, want 0", len(reqs))
			}

			if tc.withClientProvider {
				rm := collectTestMetrics(t, reader)
				requireTestMetric(t, rm, clientMetricsPrefix+metricNameOperationCount)
			}
		})
	}
}

func TestBuiltinMetricsExporterCustomConnEnabledWithMonitoringHost(t *testing.T) {
	const customMonitoringAddr = "custom-monitoring.local:443"
	const wantToken = "custom-conn-monitoring-token"

	tests := []struct {
		name    string
		usePool bool
	}{
		{name: "WithGRPCConn", usePool: false},
		{name: "WithConnPool", usePool: true},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			h := newBuiltinMetricsE2EHarness(t, "googleapis.com", nil, []string{customMonitoringAddr}, wantToken)
			t.Setenv("SPANNER_MONITORING_HOST", customMonitoringAddr)

			conn, err := grpc.NewClient(
				h.spannerServer.ServerAddress,
				grpc.WithTransportCredentials(insecure.NewCredentials()),
				grpc.WithChainUnaryInterceptor(addNativeMetricsInterceptor()...),
				grpc.WithChainStreamInterceptor(addStreamNativeMetricsInterceptor()...),
			)
			if err != nil {
				t.Fatalf("grpc.NewClient() failed: %v", err)
			}
			defer conn.Close()

			pool := &trackingConnPool{ClientConn: conn}
			opts := h.baseClientOpts()
			if tc.usePool {
				opts = append(opts, gtransport.WithConnPool(pool))
			} else {
				opts = append(opts, option.WithGRPCConn(conn))
			}

			ctx := context.Background()
			client, err := NewClientWithConfig(ctx, "projects/test-project/instances/test-instance/databases/test-db", ClientConfig{
				NumChannels: 1,
			}, opts...)
			if err != nil {
				t.Fatalf("NewClientWithConfig() failed: %v", err)
			}
			defer client.Close()

			if !client.metricsTracerFactory.enabled || client.metricsTracerFactory.meterProvider == nil {
				t.Fatal("expected enabled native metrics when SPANNER_MONITORING_HOST is set")
			}

			iter := client.Single().Query(ctx, NewStatement(SelectFooFromBar))
			if err := iter.Do(func(*Row) error { return nil }); err != nil {
				t.Fatalf("Query() failed: %v", err)
			}

			// Flush and shut down the metrics exporter before Client.Close to verify
			// exporter shutdown does not close the caller's Spanner conn or pool.
			client.metricsTracerFactory.shutdown(ctx)

			if conn.GetState() == connectivity.Shutdown {
				t.Fatal("metrics exporter shutdown closed caller's Spanner grpc.ClientConn")
			}
			if tc.usePool && pool.closed {
				t.Fatal("metrics exporter shutdown closed caller's Spanner ConnPool")
			}
			if got := h.unexpectedDials(); len(got) != 0 {
				t.Errorf("unexpected dialed endpoints: %v", got)
			}
			if got := h.dialCount("monitoring.googleapis.com:443"); got != 0 {
				t.Errorf("dialed default monitoring.googleapis.com:443 %d times, want 0", got)
			}
			if got := h.dialCount(customMonitoringAddr); got == 0 {
				t.Errorf("expected at least one dial to %q, got 0", customMonitoringAddr)
			}
			if len(h.monitoringServer.CreateServiceTimeSeriesRequests()) == 0 {
				t.Fatalf("expected CreateServiceTimeSeries requests at %q, got 0", customMonitoringAddr)
			}
			wantAuth := "Bearer " + wantToken
			if authHeaders := h.monitoringServer.AuthHeaders(); len(authHeaders) == 0 || authHeaders[0] != wantAuth {
				t.Errorf("CreateServiceTimeSeries authorization header = %v, want [%q]", authHeaders, wantAuth)
			}
		})
	}
}

func TestBuiltinMetricsExporterDynamicChannelPoolCustomEndpoint(t *testing.T) {
	const customSpannerAddr = "psc-spanner.internal:443"
	const defaultMonitoringAddr = "monitoring.googleapis.com:443"

	h := newBuiltinMetricsE2EHarness(t, "googleapis.com", []string{customSpannerAddr}, []string{defaultMonitoringAddr}, "dcp-custom-token")
	t.Setenv("GOOGLE_SPANNER_ENABLE_LEGACY_STATIC_CHANNEL_POOL", "")
	t.Setenv("SPANNER_DISABLE_CHANNEL_POOL_BUILTIN_METRICS", "")

	ctx := context.Background()
	opts := append(h.baseClientOpts(), option.WithEndpoint(customSpannerAddr))
	client, err := NewClientWithConfig(ctx, "projects/test-project/instances/test-instance/databases/test-db", ClientConfig{
		DynamicChannelPoolConfig: DynamicChannelPoolConfig{
			DCPEnabled:         true,
			DCPMinChannels:     1,
			DCPMaxChannels:     2,
			DCPInitialChannels: 1,
		},
	}, opts...)
	if err != nil {
		t.Fatalf("NewClientWithConfig() failed: %v", err)
	}

	if client.sc.dynamicPool == nil || client.sc.dynamicPool.IsFixed() {
		client.Close()
		t.Fatal("expected dynamic channel pool to be enabled")
	}
	if client.metricsTracerFactory.enabled {
		client.Close()
		t.Fatal("metricsTracerFactory.enabled = true, want false")
	}
	if client.metricsTracerFactory.meterProvider != nil {
		client.Close()
		t.Fatal("metricsTracerFactory.meterProvider != nil, want nil")
	}

	runMetricsE2EQueryAndClose(t, ctx, client)

	if got := h.dialCount(defaultMonitoringAddr); got != 0 {
		t.Errorf("dialed %q %d times, want 0", defaultMonitoringAddr, got)
	}
	if got := h.unexpectedDials(); len(got) != 0 {
		t.Errorf("unexpected dialed endpoints: %v", got)
	}
	if reqs := h.monitoringServer.CreateServiceTimeSeriesRequests(); len(reqs) != 0 {
		t.Errorf("received %d CreateServiceTimeSeries requests, want 0", len(reqs))
	}
}

func TestIsWithoutAuthentication(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name string
		opts []option.ClientOption
		want bool
	}{
		{
			name: "empty options",
			want: false,
		},
		{
			name: "authenticated credentials option",
			opts: []option.ClientOption{
				option.WithCredentials(&google.Credentials{ProjectID: "test-project"}),
			},
			want: false,
		},
		{
			name: "WithoutAuthentication option",
			opts: []option.ClientOption{
				option.WithoutAuthentication(),
			},
			want: true,
		},
		{
			name: "wrapped WithoutAuthentication option",
			opts: []option.ClientOption{
				wrappedEndpointOption{option.WithoutAuthentication()},
			},
			want: true,
		},
		{
			name: "nil option element",
			opts: []option.ClientOption{nil},
			want: false,
		},
		{
			name: "token source option",
			opts: []option.ClientOption{
				option.WithTokenSource(oauth2.StaticTokenSource(&oauth2.Token{AccessToken: "test-token"})),
			},
			want: false,
		},
		{
			name: "API key option",
			opts: []option.ClientOption{
				option.WithAPIKey("test-api-key"),
			},
			want: false,
		},
		{
			name: "custom GRPCConn option",
			opts: []option.ClientOption{
				option.WithGRPCConn(new(grpc.ClientConn)),
			},
			want: false,
		},
		{
			name: "WithoutAuthentication alongside nil option",
			opts: []option.ClientOption{
				nil,
				option.WithoutAuthentication(),
			},
			want: true,
		},
		{
			name: "conflicting WithoutAuthentication and WithCredentials",
			opts: []option.ClientOption{
				option.WithoutAuthentication(),
				option.WithCredentials(&google.Credentials{ProjectID: "test-project"}),
			},
			want: true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got, err := isWithoutAuthentication(tc.opts)
			if err != nil {
				t.Fatalf("isWithoutAuthentication() returned unexpected error: %v", err)
			}
			if got != tc.want {
				t.Errorf("isWithoutAuthentication() = %v, want %v", got, tc.want)
			}
		})
	}
}

func TestBuiltinMetricsDisabledForUnauthenticatedClients(t *testing.T) {
	const dbName = "projects/[PROJECT]/instances/[INSTANCE]/databases/[DATABASE]"

	origDetectClientLocation := detectClientLocation
	detectClientLocation = func(context.Context) string { return "global" }
	defer func() { detectClientLocation = origDetectClientLocation }()

	tests := []struct {
		name            string
		emulator        bool
		directAccessEnv bool
		newClient       func(ctx context.Context, serverAddr string, opts []option.ClientOption) (*Client, error)
	}{
		{
			name:            "WithoutAuthentication with SPANNER_MONITORING_HOST and gRPC metrics",
			directAccessEnv: true,
			newClient: func(ctx context.Context, _ string, opts []option.ClientOption) (*Client, error) {
				return NewClientWithConfig(ctx, dbName, ClientConfig{
					NumChannels:        1,
					EnableDirectAccess: true,
				}, opts...)
			},
		},
		{
			name: "wrapped WithoutAuthentication",
			newClient: func(ctx context.Context, serverAddr string, _ []option.ClientOption) (*Client, error) {
				return NewClientWithConfig(ctx, dbName, ClientConfig{NumChannels: 1},
					option.WithEndpoint(serverAddr),
					option.WithGRPCDialOption(grpc.WithTransportCredentials(insecure.NewCredentials())),
					wrappedEndpointOption{option.WithoutAuthentication()},
				)
			},
		},
		{
			name:     "SPANNER_EMULATOR_HOST with SPANNER_MONITORING_HOST",
			emulator: true,
			newClient: func(ctx context.Context, _ string, _ []option.ClientOption) (*Client, error) {
				return NewClientWithConfig(ctx, dbName, ClientConfig{
					NumChannels:        1,
					EnableDirectAccess: true,
				})
			},
		},
		{
			name: "NewMultiEndpointClientWithConfig WithoutAuthentication",
			newClient: func(ctx context.Context, serverAddr string, opts []option.ClientOption) (*Client, error) {
				gmeCfg := &grpcgcp.GCPMultiEndpointOptions{
					MultiEndpoints: map[string]*multiendpoint.MultiEndpointOptions{
						"default": {
							Endpoints: []string{serverAddr},
						},
					},
					Default: "default",
				}
				client, _, err := NewMultiEndpointClientWithConfig(ctx, dbName, ClientConfig{}, gmeCfg, opts...)
				return client, err
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			server, opts, teardownServer := NewMockedSpannerInMemTestServer(t)
			defer teardownServer()

			if tc.emulator {
				t.Setenv("SPANNER_EMULATOR_HOST", server.ServerAddress)
			} else {
				t.Setenv("SPANNER_EMULATOR_HOST", "")
				t.Setenv("GOOGLE_CLOUD_DISABLE_DIRECT_PATH", "true")
			}
			t.Setenv("SPANNER_DISABLE_BUILTIN_METRICS", "")
			t.Setenv("SPANNER_MONITORING_HOST", "custom-monitoring.local:443")
			t.Setenv("SPANNER_DISABLE_DIRECT_ACCESS_GRPC_BUILTIN_METRICS", "false")
			if tc.directAccessEnv {
				t.Setenv("GOOGLE_SPANNER_ENABLE_DIRECT_ACCESS", "true")
			}

			testExporter := installTestMonitoringExporter(t)
			ctx := context.Background()
			client, err := tc.newClient(ctx, server.ServerAddress, opts)
			if err != nil {
				t.Fatalf("client creation failed: %v", err)
			}
			closed := false
			closeClient := func() {
				if !closed {
					closed = true
					client.Close()
				}
			}
			defer closeClient()

			if testExporter.createOptionsCalls != 0 {
				t.Fatalf("createExporterOptions call count = %d, want 0", testExporter.createOptionsCalls)
			}
			if client.metricsTracerFactory.enabled {
				t.Fatal("metricsTracerFactory.enabled = true, want false")
			}
			if client.metricsTracerFactory.meterProvider != nil {
				t.Fatal("metricsTracerFactory.meterProvider != nil, want nil")
			}
			if len(client.metricsTracerFactory.sinks) != 0 {
				t.Fatalf("len(metricsTracerFactory.sinks) = %d, want 0", len(client.metricsTracerFactory.sinks))
			}
			if len(client.metricsTracerFactory.clientOpts) != 0 {
				t.Fatalf("len(metricsTracerFactory.clientOpts) = %d, want 0", len(client.metricsTracerFactory.clientOpts))
			}

			iter := client.Single().Query(ctx, NewStatement(SelectFooFromBar))
			if err := iter.Do(func(*Row) error { return nil }); err != nil {
				t.Fatalf("Query() failed: %v", err)
			}
			closeClient()

			if got := len(testExporter.server.CreateServiceTimeSeriesRequests()); got != 0 {
				t.Fatalf("CreateServiceTimeSeries request count = %d, want 0", got)
			}
		})
	}
}

func TestBuiltinMetricsWithoutAuthenticationPreservesClientAndGRPCMetrics(t *testing.T) {
	origDetectClientLocation := detectClientLocation
	detectClientLocation = func(context.Context) string { return "global" }
	defer func() { detectClientLocation = origDetectClientLocation }()

	isolateChannelPoolEnv(t)
	t.Setenv("SPANNER_EMULATOR_HOST", "")
	t.Setenv("SPANNER_DISABLE_BUILTIN_METRICS", "")
	t.Setenv("SPANNER_MONITORING_HOST", "custom-monitoring.local:443")
	t.Setenv("SPANNER_DISABLE_DIRECT_ACCESS_GRPC_BUILTIN_METRICS", "false")
	t.Setenv("GOOGLE_CLOUD_DISABLE_DIRECT_PATH", "true")
	t.Setenv("GOOGLE_CLOUD_SPANNER_ENABLE_LOG_CLIENT_OPTIONS", "true")

	testExporter := installTestMonitoringExporter(t)
	reader, provider := newTestMeterProvider()
	defer provider.Shutdown(context.Background())

	var logBuf bytes.Buffer
	_, client, teardown := setupMockedTestServerWithConfig(t, ClientConfig{
		ClientMetricsProvider: provider,
		Logger:                log.New(&logBuf, "", 0),
	})
	tornDown := false
	teardownOnce := func() {
		if !tornDown {
			tornDown = true
			teardown()
		}
	}
	defer teardownOnce()

	if w, g := 0, testExporter.createOptionsCalls; w != g {
		t.Fatalf("createExporterOptions call count mismatch:\n Want: %v\n  Got: %v", w, g)
	}
	if !client.metricsTracerFactory.enabled {
		t.Fatal("metricsTracerFactory.enabled = false, want true for caller-owned ClientMetricsProvider")
	}
	if client.metricsTracerFactory.meterProvider != nil {
		t.Fatal("metricsTracerFactory.meterProvider != nil, want nil for native sink")
	}
	if len(client.metricsTracerFactory.sinks) != 1 {
		t.Fatalf("len(metricsTracerFactory.sinks) = %d, want 1", len(client.metricsTracerFactory.sinks))
	}
	if _, err := client.Single().ReadRow(context.Background(), "Albums", Key{"foo"}, []string{"SingerId", "AlbumId", "AlbumTitle"}); err != nil {
		t.Fatalf("ReadRow() failed: %v", err)
	}

	rm := collectTestMetrics(t, reader)
	requireOperationCountAttrs(t, rm, clientMetricsPrefix)
	requireTestMetric(t, rm, clientMetricsPrefix+"grpc/client/attempt/started")
	if _, ok := findTestMetric(rm, nativeMetricsPrefix+metricNameOperationCount); ok {
		t.Fatalf("ClientMetricsProvider received reserved native metric %q", nativeMetricsPrefix+metricNameOperationCount)
	}

	teardownOnce()

	logged := logBuf.String()
	if !strings.Contains(logged, "Built-in metrics enabled: false") {
		t.Errorf("client options log missing %q:\n%s", "Built-in metrics enabled: false", logged)
	}
	if !strings.Contains(logged, "gRPC metrics enabled: true") {
		t.Errorf("client options log missing %q:\n%s", "gRPC metrics enabled: true", logged)
	}
	if w, g := 0, len(testExporter.server.CreateServiceTimeSeriesRequests()); w != g {
		t.Fatalf("CreateServiceTimeSeries request count mismatch:\n Want: %v\n  Got: %v", w, g)
	}
}

func TestBuiltinMetricsDisabledForUnauthenticatedChannelPoolsAndFallback(t *testing.T) {
	const dbName = "projects/[PROJECT]/instances/[INSTANCE]/databases/[DATABASE]"

	origDetectClientLocation := detectClientLocation
	detectClientLocation = func(context.Context) string { return "global" }
	defer func() { detectClientLocation = origDetectClientLocation }()

	tests := []struct {
		name       string
		dcpEnabled bool
	}{
		{
			name:       "fixed pool with DirectPath fallback",
			dcpEnabled: false,
		},
		{
			name:       "dynamic channel pool with DirectPath fallback",
			dcpEnabled: true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			isolateChannelPoolEnv(t)
			t.Setenv("SPANNER_EMULATOR_HOST", "")
			t.Setenv("SPANNER_DISABLE_BUILTIN_METRICS", "")
			t.Setenv("SPANNER_MONITORING_HOST", "custom-monitoring.local:443")
			t.Setenv("SPANNER_DISABLE_DIRECT_ACCESS_GRPC_BUILTIN_METRICS", "false")
			t.Setenv(directAccessEnv, "true")
			t.Setenv(gcpFallbackEnv, "true")
			t.Setenv("GOOGLE_CLOUD_DISABLE_DIRECT_PATH", "true")

			testExporter := installTestMonitoringExporter(t)
			_, opts, teardownServer := NewMockedSpannerInMemTestServer(t)
			defer teardownServer()

			cfg := ClientConfig{
				NumChannels:        1,
				EnableDirectAccess: true,
			}
			if tc.dcpEnabled {
				cfg.NumChannels = 0
				cfg.DynamicChannelPoolConfig = testDCPConfig(2, 1, 4)
			}

			ctx := context.Background()
			client, err := NewClientWithConfig(ctx, dbName, cfg, opts...)
			if err != nil {
				t.Fatalf("NewClientWithConfig() failed: %v", err)
			}
			closed := false
			closeClient := func() {
				if !closed {
					closed = true
					client.Close()
				}
			}
			defer closeClient()

			if w, g := 0, testExporter.createOptionsCalls; w != g {
				t.Fatalf("createExporterOptions call count mismatch:\n Want: %v\n  Got: %v", w, g)
			}
			if w, g := false, client.metricsTracerFactory.enabled; w != g {
				t.Fatalf("metricsTracerFactory.enabled mismatch:\n Want: %v\n  Got: %v", w, g)
			}
			if client.metricsTracerFactory.meterProvider != nil {
				t.Fatalf("metricsTracerFactory.meterProvider mismatch:\n Want: <nil>\n  Got: %v", client.metricsTracerFactory.meterProvider)
			}
			if w, g := 0, len(client.metricsTracerFactory.clientOpts); w != g {
				t.Fatalf("len(metricsTracerFactory.clientOpts) mismatch:\n Want: %v\n  Got: %v", w, g)
			}
			fb := poolFallback(client.sc.dynamicPool)
			if fb == nil {
				t.Fatal("poolFallback(client.sc.dynamicPool) = nil, want non-nil fallback")
			}
			if fb.metrics != nil {
				t.Fatalf("directPathFallback.metrics mismatch:\n Want: <nil>\n  Got: %v", fb.metrics)
			}

			iter := client.Single().Query(ctx, NewStatement(SelectFooFromBar))
			if err := iter.Do(func(*Row) error { return nil }); err != nil {
				t.Fatalf("Query() failed: %v", err)
			}
			closeClient()

			if w, g := 0, len(testExporter.server.CreateServiceTimeSeriesRequests()); w != g {
				t.Fatalf("CreateServiceTimeSeries request count mismatch:\n Want: %v\n  Got: %v", w, g)
			}
		})
	}
}

func TestBuiltinMetricsWithoutAuthenticationEndToEndNoMonitoringDial(t *testing.T) {
	const dbName = "projects/[PROJECT]/instances/[INSTANCE]/databases/[DATABASE]"
	const spannerAddr = "spanner.example.test:443"

	origDetectClientLocation := detectClientLocation
	detectClientLocation = func(context.Context) string { return "global" }
	defer func() { detectClientLocation = origDetectClientLocation }()

	isolateChannelPoolEnv(t)
	t.Setenv("SPANNER_EMULATOR_HOST", "")
	t.Setenv("SPANNER_DISABLE_BUILTIN_METRICS", "")
	t.Setenv("SPANNER_MONITORING_HOST", "custom-monitoring.local:443")
	t.Setenv("SPANNER_DISABLE_DIRECT_ACCESS_GRPC_BUILTIN_METRICS", "false")
	t.Setenv("GOOGLE_CLOUD_DISABLE_DIRECT_PATH", "true")

	spannerServer, _, teardownSpanner := NewMockedSpannerInMemTestServer(t)
	defer teardownSpanner()

	var mu sync.Mutex
	var unexpectedAddrs []string
	dialer := func(ctx context.Context, addr string) (net.Conn, error) {
		if addr == spannerAddr {
			return (&net.Dialer{}).DialContext(ctx, "tcp", spannerServer.ServerAddress)
		}
		mu.Lock()
		unexpectedAddrs = append(unexpectedAddrs, addr)
		mu.Unlock()
		return nil, fmt.Errorf("unexpected dial to %q", addr)
	}

	ctx := context.Background()
	client, err := NewClientWithConfig(ctx, dbName, ClientConfig{NumChannels: 1},
		option.WithoutAuthentication(),
		option.WithEndpoint(spannerAddr),
		option.WithGRPCDialOption(grpc.WithTransportCredentials(insecure.NewCredentials())),
		option.WithGRPCDialOption(grpc.WithResolvers(staticDNSResolverBuilder{})),
		option.WithGRPCDialOption(grpc.WithContextDialer(dialer)),
	)
	if err != nil {
		t.Fatalf("NewClientWithConfig() failed: %v", err)
	}

	iter := client.Single().Query(ctx, NewStatement(SelectFooFromBar))
	if err := iter.Do(func(*Row) error { return nil }); err != nil {
		client.Close()
		t.Fatalf("Query() failed: %v", err)
	}
	client.Close()

	mu.Lock()
	gotUnexpected := append([]string(nil), unexpectedAddrs...)
	mu.Unlock()
	if w, g := 0, len(gotUnexpected); w != g {
		t.Fatalf("unexpected dialed endpoints:\n Want: %v\n  Got: %v (%v)", w, g, gotUnexpected)
	}
}

func TestBuiltinMetricsConflictingWithoutAuthenticationAndCredentials(t *testing.T) {
	const dbName = "projects/[PROJECT]/instances/[INSTANCE]/databases/[DATABASE]"

	origDetectClientLocation := detectClientLocation
	detectClientLocation = func(context.Context) string { return "global" }
	defer func() { detectClientLocation = origDetectClientLocation }()

	t.Setenv("SPANNER_EMULATOR_HOST", "")
	t.Setenv("SPANNER_DISABLE_BUILTIN_METRICS", "")
	t.Setenv("SPANNER_MONITORING_HOST", "")

	testExporter := installTestMonitoringExporter(t)
	creds := &google.Credentials{
		ProjectID:   "test-project",
		TokenSource: oauth2.StaticTokenSource(&oauth2.Token{AccessToken: "test-token"}),
	}
	client, err := NewClientWithConfig(context.Background(), dbName, ClientConfig{NumChannels: 1},
		option.WithoutAuthentication(),
		option.WithCredentials(creds),
	)
	if err == nil {
		client.Close()
		t.Fatal("NewClientWithConfig() succeeded, want conflicting auth error")
	}
	if testExporter.createOptionsCalls != 0 {
		t.Fatalf("createExporterOptions call count = %d, want 0", testExporter.createOptionsCalls)
	}
}

func TestBuiltinMetricsLogClientOptions(t *testing.T) {
	const dbName = "projects/[PROJECT]/instances/[INSTANCE]/databases/[DATABASE]"

	origDetectClientLocation := detectClientLocation
	detectClientLocation = func(context.Context) string { return "global" }
	defer func() { detectClientLocation = origDetectClientLocation }()

	tests := []struct {
		name          string
		authenticated bool
		wantBuiltin   string
		wantGRPC      string
	}{
		{
			name:          "unauthenticated client reports disabled",
			authenticated: false,
			wantBuiltin:   "Built-in metrics enabled: false",
			wantGRPC:      "gRPC metrics enabled: false",
		},
		{
			name:          "authenticated client reports enabled",
			authenticated: true,
			wantBuiltin:   "Built-in metrics enabled: true",
			wantGRPC:      "gRPC metrics enabled: true",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv("SPANNER_EMULATOR_HOST", "")
			t.Setenv("SPANNER_DISABLE_BUILTIN_METRICS", "")
			t.Setenv("GOOGLE_CLOUD_SPANNER_ENABLE_LOG_CLIENT_OPTIONS", "true")
			t.Setenv("SPANNER_DISABLE_DIRECT_ACCESS_GRPC_BUILTIN_METRICS", "false")
			t.Setenv("GOOGLE_CLOUD_DISABLE_DIRECT_PATH", "true")

			var logBuf bytes.Buffer
			if tc.authenticated {
				installTestMonitoringExporter(t)
				_, _, teardown := setupAuthenticatedMockedTestServerWithConfig(t, ClientConfig{
					Logger: log.New(&logBuf, "", 0),
				})
				// Close before reading so no background goroutine writes to logBuf concurrently.
				teardown()
			} else {
				_, opts, teardownServer := NewMockedSpannerInMemTestServer(t)
				defer teardownServer()

				client, err := NewClientWithConfig(context.Background(), dbName, ClientConfig{
					NumChannels:        1,
					EnableDirectAccess: true,
					Logger:             log.New(&logBuf, "", 0),
				}, opts...)
				if err != nil {
					t.Fatalf("NewClientWithConfig() failed: %v", err)
				}
				client.Close()
			}

			logged := logBuf.String()
			if !strings.Contains(logged, tc.wantBuiltin) {
				t.Errorf("client options log missing %q:\n%s", tc.wantBuiltin, logged)
			}
			if !strings.Contains(logged, tc.wantGRPC) {
				t.Errorf("client options log missing %q:\n%s", tc.wantGRPC, logged)
			}
		})
	}
}

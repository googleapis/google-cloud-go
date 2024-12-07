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
	"sort"
	"testing"
	"time"

	"cloud.google.com/go/internal/testutil"
	. "cloud.google.com/go/spanner/internal/testutil"
	"github.com/google/go-cmp/cmp/cmpopts"
	"go.opentelemetry.io/otel/attribute"
	"google.golang.org/api/option"
	"google.golang.org/genproto/googleapis/api/metric"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/status"
)

func TestNewBuiltinMetricsTracerFactory(t *testing.T) {
	flag.Parse() // Needed for testing.Short().
	if testing.Short() {
		t.Skip("TestNewBuiltinMetricsTracerFactory tests skipped in -short mode.")
	}
	t.Parallel()

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
	wantMetricNamesStdout := []string{metricNameAttemptCount, metricNameAttemptLatencies, metricNameOperationCount, metricNameOperationLatencies}
	wantMetricTypesGCM := []string{}
	for _, wantMetricName := range wantMetricNamesStdout {
		wantMetricTypesGCM = append(wantMetricTypesGCM, nativeMetricsPrefix+wantMetricName)
	}

	// Reduce sampling period to reduce test run time
	origSamplePeriod := defaultSamplePeriod
	defaultSamplePeriod = 5 * time.Second
	defer func() {
		defaultSamplePeriod = origSamplePeriod
	}()

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
		setEmulator            bool
		wantCreateTSCallsCount int // No. of CreateTimeSeries calls
		wantMethods            []string
		wantOTELValue          map[string]map[string]int64
		wantOTELMetrics        map[string][]string
	}{
		{
			desc:                   "should create a new tracer factory with default meter provider",
			config:                 ClientConfig{},
			wantBuiltinEnabled:     true,
			wantCreateTSCallsCount: 2,
			wantMethods:            []string{createSessionRPC, "Spanner.StreamingRead"},
			wantOTELValue: map[string]map[string]int64{
				createSessionRPC: {
					nativeMetricsPrefix + metricNameAttemptCount:   1,
					nativeMetricsPrefix + metricNameOperationCount: 1,
				},
				"Spanner.StreamingRead": {
					nativeMetricsPrefix + metricNameAttemptCount:   2,
					nativeMetricsPrefix + metricNameOperationCount: 1,
				},
			},
			wantOTELMetrics: map[string][]string{
				createSessionRPC: wantMetricTypesGCM,
				// since operation will be retries once we will have extra attempt latency for this operation
				"Spanner.StreamingRead": append(wantMetricTypesGCM, nativeMetricsPrefix+metricNameAttemptLatencies),
			},
		},
		{
			desc:        "should not create instruments when SPANNER_EMULATOR_HOST is set",
			config:      ClientConfig{},
			setEmulator: true,
		},
	}
	for _, test := range tests {
		t.Run(test.desc, func(t *testing.T) {
			if test.setEmulator {
				// Set environment variable
				t.Setenv("SPANNER_EMULATOR_HOST", "localhost:9010")
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
			// record start time
			testStartTime := time.Now()

			// pop out all old requests
			monitoringServer.CreateServiceTimeSeriesRequests()

			// Perform single use read-only transaction
			_, err = client.Single().ReadRow(ctx, "Albums", Key{"foo"}, []string{"SingerId", "AlbumId", "AlbumTitle"})
			if err != nil {
				t.Fatalf("ReadRows failed: %v", err)
			}

			// Calculate elapsed time
			elapsedTime := time.Since(testStartTime)
			if elapsedTime < 3*defaultSamplePeriod {
				// Ensure at least 2 datapoints are recorded
				time.Sleep(3*defaultSamplePeriod - elapsedTime)
			}

			// Get new CreateServiceTimeSeriesRequests
			gotCreateTSCalls := monitoringServer.CreateServiceTimeSeriesRequests()
			var gotExpectedMethods []string
			gotOTELCountValues := make(map[string]map[string]int64)
			gotOTELLatencyValues := make(map[string]map[string]float64)
			for _, gotCreateTSCall := range gotCreateTSCalls {
				gotMetricTypesPerMethod := make(map[string][]string)
				for _, ts := range gotCreateTSCall.TimeSeries {
					gotMetricTypesPerMethod[ts.Metric.GetLabels()["method"]] = append(gotMetricTypesPerMethod[ts.Metric.GetLabels()["method"]], ts.Metric.Type)
					if _, ok := gotOTELCountValues[ts.Metric.GetLabels()["method"]]; !ok {
						gotOTELCountValues[ts.Metric.GetLabels()["method"]] = make(map[string]int64)
						gotOTELLatencyValues[ts.Metric.GetLabels()["method"]] = make(map[string]float64)
						gotExpectedMethods = append(gotExpectedMethods, ts.Metric.GetLabels()["method"])
					}
					if ts.MetricKind == metric.MetricDescriptor_CUMULATIVE && ts.GetValueType() == metric.MetricDescriptor_INT64 {
						gotOTELCountValues[ts.Metric.GetLabels()["method"]][ts.Metric.Type] = ts.Points[0].Value.GetInt64Value()
					} else {
						for _, p := range ts.Points {
							if _, ok := gotOTELCountValues[ts.Metric.GetLabels()["method"]][ts.Metric.Type]; !ok {
								gotOTELLatencyValues[ts.Metric.GetLabels()["method"]][ts.Metric.Type] = p.Value.GetDistributionValue().Mean
							} else {
								// sum up all attempt latencies
								gotOTELLatencyValues[ts.Metric.GetLabels()["method"]][ts.Metric.Type] += p.Value.GetDistributionValue().Mean
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
				// expect opLatency and attemptLatency to be non-zero
				if opLatency == 0 || attemptLatency == 0 {
					t.Errorf("Operation and attempt latencies should be non-zero for %s: operation_latency=%v, attempt_latency=%v",
						method, opLatency, attemptLatency)
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

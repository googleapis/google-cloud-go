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

package bigtable

import (
	"bytes"
	"context"
	"encoding/base64"
	"fmt"
	"io"
	"reflect"
	"slices"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	btpb "cloud.google.com/go/bigtable/apiv2/bigtablepb"
	"cloud.google.com/go/internal/testutil"
	"github.com/google/go-cmp/cmp/cmpopts"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"google.golang.org/api/option"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/wrapperspb"

	"cloud.google.com/go/bigtable/bttest"
	"google.golang.org/grpc/metadata"
)

// bigtableReadRowsServerWrapper wraps a generic grpc.ServerStream to implement the
// btpb.Bigtable_ReadRowsServer interface, specifically the Send(*ReadRowsResponse) error method.
type bigtableReadRowsServerWrapper struct {
	grpc.ServerStream
}

func (x *bigtableReadRowsServerWrapper) Send(m *btpb.ReadRowsResponse) error {
	return x.ServerStream.SendMsg(m)
}

var (
	clusterID1 = "cluster-id-1"
	clusterID2 = "cluster-id-2"
	zoneID1    = "zone-id-1"

	testHeaders, _ = proto.Marshal(&btpb.ResponseParams{
		ClusterId: &clusterID1,
		ZoneId:    &zoneID1,
	})
	testTrailers, _ = proto.Marshal(&btpb.ResponseParams{
		ClusterId: &clusterID2,
		ZoneId:    &zoneID1,
	})

	testHeaderMD = &metadata.MD{
		locationMDKey:     []string{string(testHeaders)},
		serverTimingMDKey: []string{"gfet4t7; dur=1234"},
	}
	testTrailerMD = &metadata.MD{
		locationMDKey:     []string{string(testTrailers)},
		serverTimingMDKey: []string{"gfet4t7; dur=5678"},
	}

	sleepDurationForTest = 200 * time.Millisecond // Used in ReadRowsWithDelay
)

// ReadRowsWithDelay implements the core logic for a ReadRows RPC that introduces a delay.
// This is designed to be used within a gRPC stream interceptor.
func ReadRowsWithDelay(_ any, stream btpb.Bigtable_ReadRowsServer) error {
	// 1. Send headers immediately
	if err := stream.SendHeader(metadata.MD{
		serverTimingMDKey: []string{"gfet4t7; dur=10"}, // Small initial server latency
		locationMDKey:     []string{string(testHeaders)},
	}); err != nil {
		return err
	}

	// 2. Send first chunk/response
	if err := stream.Send(&btpb.ReadRowsResponse{
		Chunks: []*btpb.ReadRowsResponse_CellChunk{
			{
				RowKey:     []byte("row1"),
				FamilyName: &wrapperspb.StringValue{Value: "cf"},
				Qualifier:  &wrapperspb.BytesValue{Value: []byte("q1")},
				Value:      []byte("val1"),
				RowStatus:  &btpb.ReadRowsResponse_CellChunk_CommitRow{CommitRow: true},
			},
		},
	}); err != nil {
		return err
	}

	// 3. Sleep
	time.Sleep(sleepDurationForTest)

	// 4. Send second chunk/response
	if err := stream.Send(&btpb.ReadRowsResponse{
		Chunks: []*btpb.ReadRowsResponse_CellChunk{
			{
				RowKey:     []byte("row2"),
				FamilyName: &wrapperspb.StringValue{Value: "cf"},
				Qualifier:  &wrapperspb.BytesValue{Value: []byte("q2")},
				Value:      []byte("val2"),
				RowStatus:  &btpb.ReadRowsResponse_CellChunk_CommitRow{CommitRow: true},
			},
		},
	}); err != nil {
		return err
	}

	return nil // Indicates successful end of stream
}

// setupFakeServerWithCustomHandler sets up a fake server with a custom stream handler for ReadRows.
// It returns a configured Table, a cleanup function, and any error during setup.
func setupFakeServerWithCustomHandler(projectID, instanceID string, cfg ClientConfig,
	customReadRowsHandler func(srv any, stream btpb.Bigtable_ReadRowsServer) error) (*Table, func(), error) {

	streamInterceptor := func(srv interface{}, ss grpc.ServerStream, info *grpc.StreamServerInfo, handler grpc.StreamHandler) error {
		if info.FullMethod == "/google.bigtable.v2.Bigtable/ReadRows" {
			// req is nil because ReadRowsWithDelay does not use the request argument.
			// Wrap the generic ServerStream with our specific wrapper.
			wrappedStream := &bigtableReadRowsServerWrapper{ss}
			return customReadRowsHandler(srv, wrappedStream)
		}
		return handler(srv, ss) // Default handling for other methods
	}

	rawGrpcServer, err := bttest.NewServer("localhost:0", grpc.StreamInterceptor(streamInterceptor))
	if err != nil {
		return nil, nil, fmt.Errorf("failed to start bttest server: %w", err)
	}

	conn, err := grpc.Dial(rawGrpcServer.Addr, grpc.WithTransportCredentials(insecure.NewCredentials()), grpc.WithBlock())
	if err != nil {
		rawGrpcServer.Close()
		return nil, nil, fmt.Errorf("failed to dial test server: %w", err)
	}

	clientOpts := []option.ClientOption{option.WithGRPCConn(conn)}

	ctx := context.Background()
	client, err := NewClientWithConfig(ctx, projectID, instanceID, cfg, clientOpts...)
	if err != nil {
		conn.Close()
		rawGrpcServer.Close()
		return nil, nil, fmt.Errorf("failed to create client: %w", err)
	}

	tbl := client.Open("test-table")

	cleanup := func() {
		client.Close()
		conn.Close()
		rawGrpcServer.Close()
	}
	return tbl, cleanup, nil
}

func equalErrs(gotErr error, wantErr error) bool {
	if gotErr == nil && wantErr == nil {
		return true
	}
	if gotErr == nil || wantErr == nil {
		return false
	}
	return strings.Contains(gotErr.Error(), wantErr.Error())
}

// readRowsWithAppBlockingDelayLogic implements the core logic for a ReadRows RPC that introduces
// sendTwoRowsHandler is a simple server-side stream handler that sends two predefined rows.
func sendTwoRowsHandler(_ any, stream btpb.Bigtable_ReadRowsServer) error {
	// 1. Send headers immediately
	if err := stream.SendHeader(metadata.MD{
		locationMDKey: []string{string(testHeaders)}, // Send cluster/zone info
	}); err != nil {
		return err
	}

	// 2. Send first chunk/response
	if err := stream.Send(&btpb.ReadRowsResponse{
		Chunks: []*btpb.ReadRowsResponse_CellChunk{
			{
				RowKey:     []byte("row1"),
				FamilyName: &wrapperspb.StringValue{Value: "cf"},
				Qualifier:  &wrapperspb.BytesValue{Value: []byte("q1")},
				Value:      []byte("val1"),
				RowStatus:  &btpb.ReadRowsResponse_CellChunk_CommitRow{CommitRow: true},
			},
		},
	}); err != nil {
		return err
	}

	// 3. Send second chunk/response
	if err := stream.Send(&btpb.ReadRowsResponse{
		Chunks: []*btpb.ReadRowsResponse_CellChunk{
			{
				RowKey:     []byte("row2"),
				FamilyName: &wrapperspb.StringValue{Value: "cf"},
				Qualifier:  &wrapperspb.BytesValue{Value: []byte("q2")},
				Value:      []byte("val2"),
				RowStatus:  &btpb.ReadRowsResponse_CellChunk_CommitRow{CommitRow: true},
			},
		},
	}); err != nil {
		return err
	}
	return nil
}

func TestNewBuiltinMetricsTracerFactory(t *testing.T) {
	ctx := context.Background()
	project := "test-project"
	instance := "test-instance"
	appProfile := "test-app-profile"
	clientUID := "test-uid"

	wantMetricNamesStdout := []string{
		metricNameAttemptLatencies, metricNameAttemptLatencies,
		metricNameFirstRespLatencies,
		metricNameConnErrCount, metricNameConnErrCount,
		metricNameOperationLatencies,
		metricNameRetryCount,
		metricNameServerLatencies,
		metricNameClientBlockingLatencies, metricNameClientBlockingLatencies,
		metricNameAppBlockingLatencies,
	}
	wantMetricTypesGCM := []string{}
	for _, wantMetricName := range wantMetricNamesStdout {
		wantMetricTypesGCM = append(wantMetricTypesGCM, builtInMetricsMeterName+wantMetricName)
	}
	sort.Strings(wantMetricTypesGCM)

	// Reduce sampling period to reduce test run time
	origSamplePeriod := defaultSamplePeriod
	defaultSamplePeriod = 5 * time.Second
	defer func() {
		defaultSamplePeriod = origSamplePeriod
	}()

	// return constant client UID instead of random, so that attributes can be compared
	origGenerateClientUID := generateClientUID
	generateClientUID = func() (string, error) {
		return clientUID, nil
	}
	defer func() {
		generateClientUID = origGenerateClientUID
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

	// Setup fake Bigtable server
	isFirstAttempt := true
	receivedHeader := metadata.MD{}
	serverStreamInterceptor := func(srv interface{}, ss grpc.ServerStream, info *grpc.StreamServerInfo, handler grpc.StreamHandler) error {
		// Capture incoming metadata
		receivedHeader, _ = metadata.FromIncomingContext(ss.Context())
		if strings.HasSuffix(info.FullMethod, "ReadRows") {
			if isFirstAttempt {
				// Fail first attempt
				isFirstAttempt = false
				return status.Error(codes.Unavailable, "Mock Unavailable error")
			}

			// Send server headers
			header := metadata.New(map[string]string{
				serverTimingMDKey: "gfet4t7; dur=123",
				locationMDKey:     string(testHeaders),
			})
			ss.SendHeader(header)
		}
		return handler(srv, ss)
	}

	tests := []struct {
		desc                   string
		config                 ClientConfig
		wantBuiltinEnabled     bool
		setEmulator            bool
		wantCreateTSCallsCount int // No. of CreateTimeSeries calls
		wantClientAttributes   []attribute.KeyValue
	}{
		{
			desc:                   "should create a new tracer factory with default meter provider",
			config:                 ClientConfig{AppProfile: appProfile},
			wantBuiltinEnabled:     true,
			wantCreateTSCallsCount: 2,
			wantClientAttributes: []attribute.KeyValue{
				attribute.String(monitoredResLabelKeyProject, project),
				attribute.String(monitoredResLabelKeyInstance, instance),
				attribute.String(metricLabelKeyAppProfile, appProfile),
				attribute.String(metricLabelKeyClientUID, clientUID),
				attribute.String(metricLabelKeyClientName, clientName),
			},
		},
		{
			desc:   "should create a new tracer factory with noop meter provider",
			config: ClientConfig{MetricsProvider: NoopMetricsProvider{}, AppProfile: appProfile},
		},
		{
			desc:        "should not create instruments when BIGTABLE_EMULATOR_HOST is set",
			config:      ClientConfig{AppProfile: appProfile},
			setEmulator: true,
		},
	}
	for _, test := range tests {
		t.Run(test.desc, func(t *testing.T) {
			if test.setEmulator {
				// Set environment variable
				t.Setenv("BIGTABLE_EMULATOR_HOST", "localhost:8086")
			}

			// open table and compare errors
			tbl, cleanup, gotErr := setupFakeServer(project, instance, test.config, grpc.StreamInterceptor(serverStreamInterceptor))
			defer cleanup()
			if gotErr != nil {
				t.Fatalf("err: got: %v, want: %v", gotErr, nil)
				return
			}

			gotClient := tbl.c

			if gotClient.metricsTracerFactory.enabled != test.wantBuiltinEnabled {
				t.Errorf("builtinEnabled: got: %v, want: %v", gotClient.metricsTracerFactory.enabled, test.wantBuiltinEnabled)
			}

			if !equalsKeyValue(gotClient.metricsTracerFactory.clientAttributes, test.wantClientAttributes) {
				t.Errorf("clientAttributes: got: %+v, want: %+v", gotClient.metricsTracerFactory.clientAttributes, test.wantClientAttributes)
			}

			// Check instruments
			gotNonNilInstruments := gotClient.metricsTracerFactory.operationLatencies != nil &&
				gotClient.metricsTracerFactory.serverLatencies != nil &&
				gotClient.metricsTracerFactory.attemptLatencies != nil &&
				gotClient.metricsTracerFactory.appBlockingLatencies != nil &&
				gotClient.metricsTracerFactory.firstRespLatencies != nil &&
				gotClient.metricsTracerFactory.retryCount != nil &&
				gotClient.metricsTracerFactory.connErrCount != nil
			if test.wantBuiltinEnabled != gotNonNilInstruments {
				t.Errorf("NonNilInstruments: got: %v, want: %v", gotNonNilInstruments, test.wantBuiltinEnabled)
			}

			// record start time
			testStartTime := time.Now()

			// pop out all old requests
			monitoringServer.CreateServiceTimeSeriesRequests()

			// Perform read rows operation
			isFirstAttempt = true
			err := tbl.ReadRows(ctx, NewRange("a", "z"), func(r Row) bool {
				return true
			})
			if err != nil {
				t.Fatalf("ReadRows failed: %v", err)
			}

			// Check feature flags
			ffStrs := receivedHeader.Get(featureFlagsHeaderKey)
			if len(ffStrs) < 1 {
				t.Errorf("Feature flags not sent by client")
			}
			ffBytes, err := base64.URLEncoding.DecodeString(ffStrs[0])
			if err != nil {
				t.Errorf("Feature flags not encoded correctly: %v", err)
			}
			ff := &btpb.FeatureFlags{}
			if err = proto.Unmarshal(ffBytes, ff); err != nil {
				t.Errorf("Feature flags not marshalled correctly: %v", err)
			}
			if ff.ClientSideMetricsEnabled != test.wantBuiltinEnabled || !ff.LastScannedRowResponses || !ff.ReverseScans {
				t.Errorf("Feature flags: ClientSideMetricsEnabled got: %v, want: %v\n"+
					"LastScannedRowResponses got: %v, want: %v\n"+
					"ReverseScans got: %v, want: %v\n",
					ff.ClientSideMetricsEnabled, test.wantBuiltinEnabled,
					ff.LastScannedRowResponses, true,
					ff.ReverseScans, true,
				)
			}

			// Calculate elapsed time
			elapsedTime := time.Since(testStartTime)
			if elapsedTime < 3*defaultSamplePeriod {
				// Ensure at least 2 datapoints are recorded
				time.Sleep(3*defaultSamplePeriod - elapsedTime)
			}

			// Get new CreateServiceTimeSeriesRequests
			gotCreateTSCalls := monitoringServer.CreateServiceTimeSeriesRequests()
			for _, gotCreateTSCall := range gotCreateTSCalls {
				gotMetricTypes := []string{}
				for _, ts := range gotCreateTSCall.TimeSeries {
					// ts.Metric.Type is of the form "bigtable.googleapis.com/internal/client/server_latencies"
					gotMetricTypes = append(gotMetricTypes, ts.Metric.Type)

					// Assert "streaming" metric label is correct
					gotStreaming, gotStreamingExists := ts.Metric.Labels[metricLabelKeyStreamingOperation]
					splitMetricType := strings.Split(ts.Metric.Type, "/")
					internalMetricName := splitMetricType[len(splitMetricType)-1] // server_latencies
					wantStreamingExists := slices.Contains(metricsDetails[internalMetricName].additionalAttrs, metricLabelKeyStreamingOperation)
					if wantStreamingExists && (!gotStreamingExists || gotStreaming != "true") {
						t.Errorf("Metric label key: %s, value: got: %v, want: %v", metricLabelKeyStreamingOperation, gotStreaming, "true")
					}
					if !wantStreamingExists && gotStreamingExists {
						t.Errorf("Metric label key: %s exists, value: got: %v, want: %v", metricLabelKeyStreamingOperation, gotStreamingExists, wantStreamingExists)
					}

					// Assert "method" metric label is correct
					wantMethod := "Bigtable.ReadRows"
					if gotLabel, ok := ts.Metric.Labels[metricLabelKeyMethod]; !ok || gotLabel != wantMethod {
						t.Errorf("Metric label key: %s, value: got: %v, want: %v", metricLabelKeyMethod, gotLabel, wantMethod)
					}
				}
				sort.Strings(gotMetricTypes)
				if !testutil.Equal(gotMetricTypes, wantMetricTypesGCM) {
					t.Errorf("Metric types missing in req. \ngot: %v, \nwant: %v\ndiff: %v", gotMetricTypes, wantMetricTypesGCM, testutil.Diff(gotMetricTypes, wantMetricTypesGCM))
				}
			}

			gotCreateTSCallsCount := len(gotCreateTSCalls)
			if gotCreateTSCallsCount < test.wantCreateTSCallsCount {
				t.Errorf("No. of CreateServiceTimeSeriesRequests: got: %v,  want: %v", gotCreateTSCalls, test.wantCreateTSCallsCount)
			}
		})
	}
}

func TestConnectivityErrorCount(t *testing.T) {
	ctx := context.Background()
	project := "test-project"
	instance := "test-instance"
	appProfile := "test-app-profile"

	// Increase sampling period to simulate potential delays
	origSamplePeriod := defaultSamplePeriod
	defaultSamplePeriod = 500 * time.Millisecond
	defer func() {
		defaultSamplePeriod = origSamplePeriod
	}()

	// Setup mock monitoring server
	monitoringServer, err := NewMetricTestServer()
	if err != nil {
		t.Fatalf("Error setting up metrics test server: %v", err)
	}
	go monitoringServer.Serve()
	defer monitoringServer.Shutdown()

	// Override exporter options to connect to the mock server
	origCreateExporterOptions := createExporterOptions
	createExporterOptions = func(opts ...option.ClientOption) []option.ClientOption {
		return []option.ClientOption{
			option.WithEndpoint(monitoringServer.Endpoint),
			option.WithoutAuthentication(),
			option.WithGRPCDialOption(grpc.WithTransportCredentials(insecure.NewCredentials())),
		}
	}
	defer func() {
		createExporterOptions = origCreateExporterOptions
	}()

	// Control structure for mock server behavior during the specific ReadRows call.
	// We use a channel to signal the interceptor that the ReadRows call under test is active.
	readRowsCallActive := make(chan bool, 1)
	var testSpecificAttemptCount int32

	serverStreamInterceptor := func(srv interface{}, ss grpc.ServerStream, info *grpc.StreamServerInfo, handler grpc.StreamHandler) error {
		if strings.HasSuffix(info.FullMethod, "ReadRows") {
			select {
			case <-readRowsCallActive:
				currentTestAttempt := atomic.AddInt32(&testSpecificAttemptCount, 1)
				if currentTestAttempt == 1 {
					// Put the token back for subsequent retries of this specific call.
					readRowsCallActive <- true
					return status.Error(codes.Unavailable, "Mock Unavailable error for connectivity test")
				}
				if currentTestAttempt == 2 {
					header := metadata.New(map[string]string{
						locationMDKey: string(testHeaders),
					})
					if errH := ss.SendHeader(header); errH != nil {
						t.Errorf("[ServerInterceptor Attempt 2] Error sending header: %v", errH)
					}

					// Send a minimal successful message to ensure headers are processed by the client.
					emptyResp := &btpb.ReadRowsResponse{}
					if errS := ss.SendMsg(emptyResp); errS != nil {
						t.Errorf("[ServerInterceptor Attempt 2] Error sending empty message: %v", errS)
						return status.Errorf(codes.Internal, "mock server failed to send empty message: %v", errS)
					}

					readRowsCallActive <- true
					return status.Error(codes.Unavailable, "Mock Unavailable error with location headers")
				}

				// On the third and final attempt, cause a non-retriable error.
				atomic.StoreInt32(&testSpecificAttemptCount, 0)
				// Do not put the token back, as this is the final attempt for this ReadRows sequence.
				return status.Error(codes.Internal, "non-retriable error")
			default:
				return handler(srv, ss)
			}
		}
		return handler(srv, ss)
	}

	config := ClientConfig{AppProfile: appProfile}
	tbl, cleanup, gotErr := setupFakeServer(project, instance, config, grpc.StreamInterceptor(serverStreamInterceptor))
	defer cleanup()
	if gotErr != nil {
		t.Fatalf("setupFakeServer error: got: %v, want: nil", gotErr)
	}

	// Pop out any old requests from the monitoring server to ensure a clean state.
	monitoringServer.CreateServiceTimeSeriesRequests()
	atomic.StoreInt32(&testSpecificAttemptCount, 0)

	readRowsCallActive <- true

	// Perform a read rows operation that will undergo a specific retry sequence:
	// Attempt 1: Fails with Unavailable (no server headers) -> conn error count = 1
	// Attempt 2: Fails with Unavailable (with location header) -> conn error count = 0
	// Attempt 3: Fails with Internal (no server headers) -> conn error count = 1
	// The overall operation fails with the final Internal error.
	err = tbl.ReadRows(ctx, NewRange("a", "z"), func(r Row) bool { return true })
	if err == nil {
		t.Fatal("ReadRows: got nil error, want an error")
	}
	if status.Code(err) != codes.Internal {
		t.Fatalf("ReadRows: got error code %v, want %v", status.Code(err), codes.Internal)
	}

	// Wait a bit for metrics to be exported. The defaultSamplePeriod is 500ms,
	// so waiting slightly longer should be sufficient.
	// If tests are flaky, this might need adjustment or a more sophisticated wait.
	time.Sleep(defaultSamplePeriod + 200*time.Millisecond)

	var totalConnectivityErrorsFromMetrics int64
	statusesReported := make(map[string]int64)
	foundConnErrMetricForTest := false

	exportedMetricBatches := monitoringServer.CreateServiceTimeSeriesRequests()
	for _, batch := range exportedMetricBatches {
		for _, ts := range batch.TimeSeries {
			if strings.HasSuffix(ts.Metric.Type, metricNameConnErrCount) {
				methodLabel, ok := ts.Metric.Labels[metricLabelKeyMethod]
				if !ok || methodLabel != "Bigtable.ReadRows" {
					continue
				}
				foundConnErrMetricForTest = true
				statusKey := ts.Metric.Labels[metricLabelKeyStatus]
				for _, point := range ts.Points {
					// Summing up values from points. For a counter, this is the delta.
					// We expect each reported error to be a single point with a value of 1.
					statusesReported[statusKey] += point.GetValue().GetInt64Value()
					totalConnectivityErrorsFromMetrics += point.GetValue().GetInt64Value()
				}
			}
		}
	}

	if !foundConnErrMetricForTest {
		t.Fatalf("Metric %s for method Bigtable.ReadRows was not found in exported metrics. Batches received: %+v", metricNameConnErrCount, exportedMetricBatches)
	}

	if statusesReported[codes.Unavailable.String()] != 1 {
		t.Errorf("Metric %s for status %s: got cumulative value %d, want 1. All statuses: %v",
			metricNameConnErrCount, codes.Unavailable.String(), statusesReported[codes.Unavailable.String()], statusesReported)
	}
	if statusesReported[codes.Internal.String()] != 1 {
		t.Errorf("Metric %s for status %s: got cumulative value %d, want 1. All statuses: %v",
			metricNameConnErrCount, codes.Internal.String(), statusesReported[codes.Internal.String()], statusesReported)
	}

	// The total connectivity errors should be 2.
	// Attempt 2 (Unavailable, with location) should not increment the error count.
	if totalConnectivityErrorsFromMetrics != 2 {
		t.Errorf("Metric %s: got cumulative value %d, want 2. Statuses reported: %v",
			metricNameConnErrCount, totalConnectivityErrorsFromMetrics, statusesReported)
	}
}
func setMockErrorHandler(t *testing.T, mockErrorHandler *MockErrorHandler) {
	origErrHandler := otel.GetErrorHandler()
	otel.SetErrorHandler(mockErrorHandler)
	t.Cleanup(func() {
		otel.SetErrorHandler(origErrHandler)
	})
}

func equalsKeyValue(gotAttrs, wantAttrs []attribute.KeyValue) bool {
	if len(gotAttrs) != len(wantAttrs) {
		return false
	}

	gotJSONVals, err := keyValueToKeyJSONValue(gotAttrs)
	if err != nil {
		return false
	}
	wantJSONVals, err := keyValueToKeyJSONValue(wantAttrs)
	if err != nil {
		return false
	}
	return testutil.Equal(gotJSONVals, wantJSONVals)
}

func keyValueToKeyJSONValue(attrs []attribute.KeyValue) (map[string]string, error) {
	keyJSONVal := map[string]string{}
	for _, attr := range attrs {
		jsonVal, err := attr.Value.MarshalJSON()
		if err != nil {
			return nil, err
		}
		keyJSONVal[string(attr.Key)] = string(jsonVal)
	}
	return keyJSONVal, nil
}

func TestExporterLogs(t *testing.T) {
	ctx := context.Background()
	project := "test-project"
	instance := "test-instance"

	// Reduce sampling period to reduce test run time
	origSamplePeriod := defaultSamplePeriod
	defaultSamplePeriod = 5 * time.Second
	defer func() {
		defaultSamplePeriod = origSamplePeriod
	}()

	tbl, cleanup, gotErr := setupFakeServer(project, instance, ClientConfig{})
	t.Cleanup(func() { defer cleanup() })
	if gotErr != nil {
		t.Fatalf("err: got: %v, want: %v", gotErr, nil)
		return
	}

	// Set up mock error handler
	mer := &MockErrorHandler{
		buffer: new(bytes.Buffer),
	}
	setMockErrorHandler(t, mer)

	// record start time
	testStartTime := time.Now()

	// Perform read rows operation
	tbl.ReadRows(ctx, NewRange("a", "z"), func(r Row) bool {
		return true
	})

	// Calculate elapsed time
	elapsedTime := time.Since(testStartTime)
	if elapsedTime < 3*defaultSamplePeriod {
		// Ensure at least 2 datapoints are recorded
		time.Sleep(3*defaultSamplePeriod - elapsedTime)
	}

	data, readErr := mer.read()
	if readErr != nil {
		t.Errorf("Failed to read errBuf: %v", readErr)
	}
	if !strings.Contains(data, metricsErrorPrefix) {
		t.Errorf("Expected %v to contain %v", data, metricsErrorPrefix)
	}
}

type MockErrorHandler struct {
	buffer      *bytes.Buffer
	bufferMutex sync.Mutex
}

func (m *MockErrorHandler) Handle(err error) {
	m.bufferMutex.Lock()
	defer m.bufferMutex.Unlock()
	fmt.Fprintln(m.buffer, err)
}

func (m *MockErrorHandler) read() (string, error) {
	m.bufferMutex.Lock()
	defer m.bufferMutex.Unlock()
	data, err := io.ReadAll(m.buffer)
	if err != nil {
		return "", err
	}
	return string(data), nil
}

func TestToOtelMetricAttrs(t *testing.T) {
	mt := builtinMetricsTracer{
		tableName:   "my-table",
		method:      "ReadRows",
		isStreaming: true,
		currOp: opTracer{
			status: codes.OK.String(),
			currAttempt: attemptTracer{
				startTime: time.Now(),
				clusterID: "my-cluster",
				zoneID:    "my-zone",
			},
			attemptCount: 1,
		},
	}
	tests := []struct {
		desc       string
		mt         builtinMetricsTracer
		metricName string
		wantAttrs  []attribute.KeyValue
		wantError  error
	}{
		{
			desc:       "Known metric",
			mt:         mt,
			metricName: metricNameOperationLatencies,
			wantAttrs: []attribute.KeyValue{
				attribute.String(monitoredResLabelKeyTable, "my-table"),
				attribute.String(metricLabelKeyMethod, "ReadRows"),
				attribute.Bool(metricLabelKeyStreamingOperation, true),
				attribute.String(metricLabelKeyStatus, codes.OK.String()),
				attribute.String(monitoredResLabelKeyCluster, clusterID1),
				attribute.String(monitoredResLabelKeyZone, zoneID1),
			},
			wantError: nil,
		},
		{
			desc:       "Unknown metric",
			mt:         mt,
			metricName: "unknown_metric",
			wantAttrs:  []attribute.KeyValue{}, // Expect empty slice on error
			wantError:  fmt.Errorf("unable to create attributes list for unknown metric: unknown_metric"),
		},
	}

	lessKeyValue := func(a, b attribute.KeyValue) bool { return a.Key < b.Key }
	for _, test := range tests {
		t.Run(test.desc, func(t *testing.T) {
			gotAttrSet, gotErr := test.mt.toOtelMetricAttrs(test.metricName)
			if !equalErrs(gotErr, test.wantError) {
				t.Errorf("error got: %v, want: %v", gotErr, test.wantError)
			}
			gotAttrsSlice := gotAttrSet.ToSlice() // Convert Set to Slice
			if gotAttrsSlice == nil {             // Ensure nil slice is treated as empty slice for comparison
				gotAttrsSlice = []attribute.KeyValue{}
			}
			if diff := testutil.Diff(gotAttrsSlice, test.wantAttrs,
				cmpopts.IgnoreUnexported(attribute.KeyValue{}, attribute.Value{}),
				cmpopts.SortSlices(lessKeyValue)); diff != "" {
				t.Errorf("got=-, want=+ \n%v", diff)
			}
		})
	}
}

func TestCreateExporterOptionsFiltering(t *testing.T) {
	endpointOpt := option.WithEndpoint("test.endpoint")
	apiKeyOpt := option.WithAPIKey("test.apikey")
	audiencesOpt := option.WithAudiences("test.audience")

	inputOpts := []option.ClientOption{
		endpointOpt,
		apiKeyOpt,
		audiencesOpt,
	}

	filteredOpts := createExporterOptions(inputOpts...)

	foundEndpointOpt := false
	foundAPIKeyOpt := false
	foundAudiencesOpt := false

	for _, opt := range filteredOpts {
		if reflect.TypeOf(opt) == reflect.TypeOf(endpointOpt) {
			foundEndpointOpt = true
		}
		if reflect.TypeOf(opt) == reflect.TypeOf(apiKeyOpt) {
			foundAPIKeyOpt = true
		}
		if reflect.TypeOf(opt) == reflect.TypeOf(audiencesOpt) {
			foundAudiencesOpt = true
		}
	}

	if foundEndpointOpt {
		t.Errorf("option.WithEndpoint was found in filtered options, but it should have been filtered out")
	}

	if !foundAPIKeyOpt {
		t.Errorf("option.WithAPIKey was not found in filtered options, but it should have been preserved")
	}

	if !foundAudiencesOpt {
		t.Errorf("option.WithAudiences was not found in filtered options, but it should have been preserved")
	}

	if len(filteredOpts) != 2 {
		t.Errorf("Expected 2 options to be returned, got %d", len(filteredOpts))
	}
}

func TestFirstResponseLatencyWithDelayedStream(t *testing.T) {
	ctx := context.Background()
	project := "test-project"
	instance := "test-instance"
	appProfile := "test-app-profile"
	clientUID := "test-uid-delayed"

	// Reduce sampling period to reduce test run time
	origSamplePeriod := defaultSamplePeriod
	defaultSamplePeriod = 100 * time.Millisecond
	defer func() {
		defaultSamplePeriod = origSamplePeriod
	}()

	// return constant client UID instead of random, so that attributes can be compared
	origGenerateClientUID := generateClientUID
	generateClientUID = func() (string, error) {
		return clientUID, nil
	}
	defer func() {
		generateClientUID = origGenerateClientUID
	}()

	// Set up a mock error handler to swallow expected shutdown errors
	mer := &MockErrorHandler{buffer: new(bytes.Buffer)}
	origErrHandler := otel.GetErrorHandler()
	otel.SetErrorHandler(mer)
	t.Cleanup(func() {
		otel.SetErrorHandler(origErrHandler)
	})

	// Setup mock monitoring server
	monitoringServer, err := NewMetricTestServer()
	if err != nil {
		t.Fatalf("Error setting up metrics test server: %v", err)
	}
	go monitoringServer.Serve()
	defer monitoringServer.Shutdown()

	// Override exporter options
	origCreateExporterOptions := createExporterOptions
	createExporterOptions = func(opts ...option.ClientOption) []option.ClientOption {
		return []option.ClientOption{
			option.WithEndpoint(monitoringServer.Endpoint),
			option.WithoutAuthentication(),
			option.WithGRPCDialOption(grpc.WithTransportCredentials(insecure.NewCredentials())),
		}
	}
	defer func() {
		createExporterOptions = origCreateExporterOptions
	}()

	// Setup fake Bigtable server with delayed stream handler
	// Define the custom ReadRows handler that uses ReadRowsWithDelay
	readRowsHandler := func(srv any, stream btpb.Bigtable_ReadRowsServer) error {
		return ReadRowsWithDelay(srv, stream) // req can be nil if ReadRowsWithDelay is adapted
	}

	tbl, cleanup, err := setupFakeServerWithCustomHandler(project, instance, ClientConfig{AppProfile: appProfile}, readRowsHandler)
	defer cleanup()
	if err != nil {
		t.Fatalf("setupFakeServerWithMock error: got: %v, want: nil", err)
		return
	}

	// Pop out any old requests from other tests
	monitoringServer.CreateServiceTimeSeriesRequests()

	// Perform read rows operation
	readErr := tbl.ReadRows(ctx, NewRange("a", "z"), func(r Row) bool {
		// Consume all rows
		return true
	})
	if readErr != nil {
		t.Fatalf("ReadRows failed: %v", readErr)
	}

	// Allow time for metrics to be exported
	// Wait for at least 3 export cycles to be reasonably sure metrics are flushed.
	time.Sleep(defaultSamplePeriod * 10)

	// Fetch and analyze metrics
	requests := monitoringServer.CreateServiceTimeSeriesRequests()
	if len(requests) == 0 {
		t.Fatalf("No CreateTimeSeriesRequests received from mock monitoring server")
	}

	var firstRespLatencyValue float64 = -1
	var opLatencyValue float64 = -1
	var foundMetricsForClientUID []string

	for _, req := range requests {
		for _, ts := range req.TimeSeries {
			metricType := ts.GetMetric().GetType()
			// Check client UID label first
			clientUIDLabel, ok := ts.GetMetric().GetLabels()[metricLabelKeyClientUID]
			if !ok || clientUIDLabel != clientUID {
				// Metric does not match target client UID. Skipping
				continue
			}
			// If we reach here, the metric belongs to our test client instance
			foundMetricsForClientUID = append(foundMetricsForClientUID, metricType)

			points := ts.GetPoints()
			if len(points) == 0 {
				continue
			}
			typedVal := points[0].GetValue()
			if typedVal == nil {
				continue
			}
			distVal := typedVal.GetDistributionValue()
			if distVal == nil {
				continue
			}

			if distVal.GetCount() != 1 {
				continue
			}

			// Already filtered by clientUID above.
			if strings.HasSuffix(metricType, metricNameFirstRespLatencies) {
				firstRespLatencyValue = distVal.GetMean()
			} else if strings.HasSuffix(metricType, metricNameOperationLatencies) {
				opLatencyValue = distVal.GetMean()
			}
		}
	}

	if firstRespLatencyValue == -1 {
		t.Fatalf("first_response_latencies metric not found or had count != 1 for client UID '%s'. foundMetricsForClientUID: %+v", clientUID, foundMetricsForClientUID)
	}
	if opLatencyValue == -1 {
		t.Fatalf("operation_latencies metric not found or had count != 1 for client UID '%s'. foundMetricsForClientUID: %+v", clientUID, foundMetricsForClientUID)
	}

	// Assertions
	if firstRespLatencyValue <= 0 {
		t.Errorf("firstRespLatencyValue: got %v, want > 0", firstRespLatencyValue)
	}
	if opLatencyValue <= firstRespLatencyValue {
		t.Errorf("opLatencyValue (%v) should be greater than firstRespLatencyValue (%v)", opLatencyValue, firstRespLatencyValue)
	}

	expectedMinOpLatency := float64(sleepDurationForTest / time.Millisecond)
	if opLatencyValue <= expectedMinOpLatency {
		t.Errorf("opLatencyValue: got %v, want > %v (sleepDuration)", opLatencyValue, expectedMinOpLatency)
	}

	// The primary assertion: firstRespLatencyValue should not include the sleepDuration.
	// opLatencyValue = firstRespLatencyValue + serverProcessingTimeBetweenFirstAndLastResponse + clientProcessingTimeAfterLastChunk
	// serverProcessingTimeBetweenFirstAndLastResponse includes the sleepDurationForTest
	// So, opLatencyValue should be roughly firstRespLatencyValue + sleepDurationForTest + other_small_overheads
	epsilon := 150.0 // Epsilon in milliseconds
	// It can be that firstRespLatencyValue is very small (e.g. <1ms) and opLatencyValue is dominated by sleepDuration + server processing of 2nd chunk.
	// Assert that opLatencyValue is greater than or equal to firstRespLatency + sleepDuration, minus epsilon.
	// opLatencyValue >= firstRespLatencyValue + float64(sleepDurationForTest/time.Millisecond) - epsilon
	// This is equivalent to: firstRespLatencyValue + float64(sleepDurationForTest/time.Millisecond) <= opLatencyValue + epsilon
	// This ensures that the first response latency is recorded *before* the artificial delay.
	lowerBoundForOpLatency := firstRespLatencyValue + float64(sleepDurationForTest/time.Millisecond)
	if opLatencyValue < lowerBoundForOpLatency-epsilon {
		t.Errorf("opLatencyValue (%v) is too small. Expected it to be >= firstRespLatencyValue (%v) + sleepDuration (%v ms) - epsilon (%v ms). Difference: %v",
			opLatencyValue, firstRespLatencyValue, float64(sleepDurationForTest/time.Millisecond), epsilon, lowerBoundForOpLatency-opLatencyValue)
	}

	// first response latency should be significantly smaller than the sleep duration.
	if firstRespLatencyValue >= float64(sleepDurationForTest/time.Millisecond) {
		t.Errorf("firstRespLatencyValue (%v) should be significantly smaller than sleepDuration (%v ms)", firstRespLatencyValue, float64(sleepDurationForTest/time.Millisecond))
	}

}

func TestApplicationLatencies(t *testing.T) {
	ctx := context.Background()
	project := "test-project"
	instance := "test-instance"
	tableID := "test-table" // Defined for assertion consistency
	appProfile := "test-app-profile-app-latency"
	clientUID := "test-uid-app-latency" // Unique UID for this test
	// appProcessingDelay will be used in the ReadRows callback
	const appProcessingDelay = 150 * time.Millisecond

	// Reduce sampling period to reduce test run time
	origSamplePeriod := defaultSamplePeriod
	defaultSamplePeriod = 100 * time.Millisecond // Shorter for quicker metric export
	defer func() {
		defaultSamplePeriod = origSamplePeriod
	}()

	// return constant client UID instead of random, so that attributes can be compared
	origGenerateClientUID := generateClientUID
	generateClientUID = func() (string, error) {
		return clientUID, nil
	}
	defer func() {
		generateClientUID = origGenerateClientUID
	}()

	// Set up a mock error handler to swallow expected shutdown errors
	mer := &MockErrorHandler{buffer: new(bytes.Buffer)}
	origErrHandler := otel.GetErrorHandler()
	otel.SetErrorHandler(mer)
	t.Cleanup(func() {
		otel.SetErrorHandler(origErrHandler)
	})

	// Setup mock monitoring server
	monitoringServer, err := NewMetricTestServer()
	if err != nil {
		t.Fatalf("Error setting up metrics test server: %v", err)
	}
	go monitoringServer.Serve()
	defer monitoringServer.Shutdown()

	// Override exporter options
	origCreateExporterOptions := createExporterOptions
	createExporterOptions = func(opts ...option.ClientOption) []option.ClientOption {
		return []option.ClientOption{
			option.WithEndpoint(monitoringServer.Endpoint),
			option.WithoutAuthentication(),
			option.WithGRPCDialOption(grpc.WithTransportCredentials(insecure.NewCredentials())),
		}
	}
	defer func() {
		createExporterOptions = origCreateExporterOptions
	}()

	// Setup fake Bigtable server which returns 2 rows
	tbl, cleanup, err := setupFakeServerWithCustomHandler(project, instance, ClientConfig{AppProfile: appProfile}, sendTwoRowsHandler)
	defer cleanup()
	if err != nil {
		t.Fatalf("setupFakeServerWithMock error: got: %v, want: nil", err)
		return
	}

	// Pop out any old requests from other tests
	monitoringServer.CreateServiceTimeSeriesRequests()

	// Perform read rows operation
	var rowsProcessed int
	readErr := tbl.ReadRows(ctx, NewRange("a", "z"), func(r Row) bool {
		// Simulate application processing time for each row
		time.Sleep(appProcessingDelay)
		rowsProcessed++
		return rowsProcessed < 2 // Process 2 rows
	})
	if readErr != nil {
		t.Fatalf("ReadRows failed: %v", readErr)
	}

	// Allow time for metrics to be exported
	// Wait for at least 3 export cycles to be reasonably sure metrics are flushed.
	// Increased sleep duration to see if it's a timing issue.
	time.Sleep(defaultSamplePeriod * 10)

	// Fetch and analyze metrics
	requests := monitoringServer.CreateServiceTimeSeriesRequests()
	if len(requests) == 0 {
		t.Fatalf("No CreateTimeSeriesRequests received from mock monitoring server")
	}

	foundAppLatencyMetric := false
	expectedMetricType := builtInMetricsMeterName + metricNameAppBlockingLatencies
	expectedTotalAppLatency := float64(rowsProcessed) * float64(appProcessingDelay/time.Millisecond)
	epsilon := 50.0 // Epsilon in milliseconds, allow for some overhead

	for _, req := range requests {
		for _, ts := range req.TimeSeries {
			metricType := ts.GetMetric().GetType()
			metricLabels := ts.GetMetric().GetLabels()

			if clientUIDLabel, ok := metricLabels[metricLabelKeyClientUID]; !ok || clientUIDLabel != clientUID {
				continue // Not for this client
			}

			if metricType == expectedMetricType {
				foundAppLatencyMetric = true
				points := ts.GetPoints()
				if len(points) == 0 {
					t.Errorf("No points found for metric %s", expectedMetricType)
					continue
				}
				distVal := points[0].GetValue().GetDistributionValue()
				if distVal == nil {
					t.Errorf("DistributionValue is nil for metric %s", expectedMetricType)
					continue
				}

				if distVal.GetCount() != 1 {
					t.Errorf("GetCount(): got %d, want 1 for metric %s", distVal.GetCount(), expectedMetricType)
				}

				recordedMean := distVal.GetMean()
				if recordedMean < expectedTotalAppLatency-epsilon || recordedMean > expectedTotalAppLatency+epsilon {
					t.Errorf("App latency mean: got %v, want within %v ± %v", recordedMean, expectedTotalAppLatency, epsilon)
				}

				// Assert standard labels
				if method, ok := metricLabels[metricLabelKeyMethod]; !ok || method != "Bigtable.ReadRows" {
					t.Errorf("Label %s: got %v, want Bigtable.ReadRows", metricLabelKeyMethod, method)
				}

				// Assert streaming label is not present (as per metricsDetails)
				if _, exists := metricLabels[metricLabelKeyStreamingOperation]; exists {
					t.Errorf("Label %s should not be present for %s", metricLabelKeyStreamingOperation, expectedMetricType)
				}

				resLabels := ts.GetResource().GetLabels()
				if tblName, ok := resLabels[monitoredResLabelKeyTable]; (ok && tblName != tableID && tblName != "") || !ok {
					t.Errorf("Label %s: got %q, want %q for resource %s", monitoredResLabelKeyTable, tblName, tableID, ts.GetResource())
				}
			}
		}
	}

	if !foundAppLatencyMetric {
		t.Errorf("Failed to find metric %s for client UID %s", expectedMetricType, clientUID)
	}
}

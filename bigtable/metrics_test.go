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
	"sort"
	"strings"
	"sync"
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

	"google.golang.org/grpc/metadata"
)

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
)

func equalErrs(gotErr error, wantErr error) bool {
	if gotErr == nil && wantErr == nil {
		return true
	}
	if gotErr == nil || wantErr == nil {
		return false
	}
	return strings.Contains(gotErr.Error(), wantErr.Error())
}

func TestNewBuiltinMetricsTracerFactory(t *testing.T) {
	ctx := context.Background()
	project := "test-project"
	instance := "test-instance"
	appProfile := "test-app-profile"
	clientUID := "test-uid"

	wantClientAttributes := []attribute.KeyValue{
		attribute.String(monitoredResLabelKeyProject, project),
		attribute.String(monitoredResLabelKeyInstance, instance),
		attribute.String(metricLabelKeyAppProfile, appProfile),
		attribute.String(metricLabelKeyClientUID, clientUID),
		attribute.String(metricLabelKeyClientName, clientName),
	}

	wantMetricNames := []string{metricNameAttemptLatencies, metricNameAttemptLatencies, metricNameConnErrCount, metricNameConnErrCount, metricNameFirstRespLatencies, metricNameOperationLatencies, metricNameRetryCount, metricNameServerLatencies}
	wantMetricTypesGCM := []string{}
	for _, wantMetricName := range wantMetricNames {
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
	}{
		{
			desc:                   "should create a new tracer factory with default meter provider",
			config:                 ClientConfig{},
			wantBuiltinEnabled:     true,
			wantCreateTSCallsCount: 2,
		},
		{
			desc:   "should create a new tracer factory with noop meter provider",
			config: ClientConfig{MetricsProvider: NoopMetricsProvider{}},
		},
		{
			desc:        "should not create instruments when BIGTABLE_EMULATOR_HOST is set",
			config:      ClientConfig{},
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

			if diff := testutil.Diff(gotClient.metricsTracerFactory.clientAttributes, wantClientAttributes,
				cmpopts.IgnoreUnexported(attribute.KeyValue{}, attribute.Value{})); diff != "" {
				t.Errorf("clientAttributes: got=-, want=+ \n%v", diff)
			}

			// Check instruments
			gotNonNilInstruments := gotClient.metricsTracerFactory.operationLatencies != nil &&
				gotClient.metricsTracerFactory.serverLatencies != nil &&
				gotClient.metricsTracerFactory.attemptLatencies != nil &&
				gotClient.metricsTracerFactory.firstRespLatencies != nil &&
				gotClient.metricsTracerFactory.connErrCount != nil &&
				gotClient.metricsTracerFactory.retryCount != nil
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
					gotMetricTypes = append(gotMetricTypes, ts.Metric.Type)
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

func setMockErrorHandler(t *testing.T, mockErrorHandler *MockErrorHandler) {
	origErrHandler := otel.GetErrorHandler()
	otel.SetErrorHandler(mockErrorHandler)
	t.Cleanup(func() {
		otel.SetErrorHandler(origErrHandler)
	})
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

	// In setupFakeServer above, Bigtable client is created with options :
	// option.WithGRPCConn(conn), option.WithGRPCDialOption(grpc.WithBlock())
	// These same options will be used to create Monitoring client but since there
	// is no fake Monitoring server at that grpc conn, all the exports result in failure.
	// Thus, there should be errors in errBuf.
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
			wantAttrs: []attribute.KeyValue{
				attribute.String(monitoredResLabelKeyTable, "my-table"),
				attribute.String(metricLabelKeyMethod, "ReadRows"),
				attribute.String(monitoredResLabelKeyCluster, clusterID1),
				attribute.String(monitoredResLabelKeyZone, zoneID1),
			},
			wantError: fmt.Errorf("unable to create attributes list for unknown metric: unknown_metric"),
		},
	}

	lessKeyValue := func(a, b attribute.KeyValue) bool { return a.Key < b.Key }
	for _, test := range tests {
		t.Run(test.desc, func(t *testing.T) {
			gotAttrs, gotErr := test.mt.toOtelMetricAttrs(test.metricName)
			if !equalErrs(gotErr, test.wantError) {
				t.Errorf("error got: %v, want: %v", gotErr, test.wantError)
			}
			if diff := testutil.Diff(gotAttrs, test.wantAttrs,
				cmpopts.IgnoreUnexported(attribute.KeyValue{}, attribute.Value{}),
				cmpopts.SortSlices(lessKeyValue)); diff != "" {
				t.Errorf("got=-, want=+ \n%v", diff)
			}
		})
	}
}

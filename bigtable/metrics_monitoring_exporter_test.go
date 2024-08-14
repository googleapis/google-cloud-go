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
	"context"
	"errors"
	"fmt"
	"net"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"

	"go.opentelemetry.io/otel/attribute"
	otelmetric "go.opentelemetry.io/otel/metric"
	"go.opentelemetry.io/otel/sdk/instrumentation"
	"go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/metric/metricdata"
	"go.opentelemetry.io/otel/sdk/resource"
	semconv "go.opentelemetry.io/otel/semconv/v1.24.0"

	"cloud.google.com/go/internal/testutil"
	"cloud.google.com/go/monitoring/apiv3/v2/monitoringpb"
	"google.golang.org/api/option"
	googlemetricpb "google.golang.org/genproto/googleapis/api/metric"
	metricpb "google.golang.org/genproto/googleapis/api/metric"
	monitoredrespb "google.golang.org/genproto/googleapis/api/monitoredres"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/keepalive"
	"google.golang.org/grpc/metadata"
	"google.golang.org/protobuf/types/known/emptypb"
)

type MetricsTestServer struct {
	lis                         net.Listener
	srv                         *grpc.Server
	Endpoint                    string
	userAgent                   string
	createMetricDescriptorReqs  []*monitoringpb.CreateMetricDescriptorRequest
	createServiceTimeSeriesReqs []*monitoringpb.CreateTimeSeriesRequest
	RetryCount                  int
	mu                          sync.Mutex
}

func (m *MetricsTestServer) Shutdown() {
	// this will close mts.lis
	m.srv.GracefulStop()
}

// Pops out the UserAgent from the most recent CreateTimeSeriesRequests or CreateServiceTimeSeriesRequests.
func (m *MetricsTestServer) UserAgent() string {
	m.mu.Lock()
	defer m.mu.Unlock()
	ua := m.userAgent
	m.userAgent = ""
	return ua
}

// Pops out the CreateServiceTimeSeriesRequests which the test server has received so far.
func (m *MetricsTestServer) CreateServiceTimeSeriesRequests() []*monitoringpb.CreateTimeSeriesRequest {
	m.mu.Lock()
	defer m.mu.Unlock()
	reqs := m.createServiceTimeSeriesReqs
	m.createServiceTimeSeriesReqs = nil
	return reqs
}

func (m *MetricsTestServer) appendCreateMetricDescriptorReq(ctx context.Context, req *monitoringpb.CreateMetricDescriptorRequest) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.createMetricDescriptorReqs = append(m.createMetricDescriptorReqs, req)
}

func (m *MetricsTestServer) appendCreateServiceTimeSeriesReq(ctx context.Context, req *monitoringpb.CreateTimeSeriesRequest) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.createServiceTimeSeriesReqs = append(m.createServiceTimeSeriesReqs, req)
	if md, ok := metadata.FromIncomingContext(ctx); ok {
		m.userAgent = strings.Join(md.Get("User-Agent"), ";")
	}
}

func (m *MetricsTestServer) Serve() error {
	return m.srv.Serve(m.lis)
}

type fakeMetricServiceServer struct {
	monitoringpb.UnimplementedMetricServiceServer
	metricsTestServer *MetricsTestServer
}

func (f *fakeMetricServiceServer) CreateServiceTimeSeries(
	ctx context.Context,
	req *monitoringpb.CreateTimeSeriesRequest,
) (*emptypb.Empty, error) {
	f.metricsTestServer.appendCreateServiceTimeSeriesReq(ctx, req)
	return &emptypb.Empty{}, nil
}

func (f *fakeMetricServiceServer) CreateMetricDescriptor(
	ctx context.Context,
	req *monitoringpb.CreateMetricDescriptorRequest,
) (*metricpb.MetricDescriptor, error) {
	f.metricsTestServer.appendCreateMetricDescriptorReq(ctx, req)
	return &metricpb.MetricDescriptor{}, nil
}

func NewMetricTestServer() (*MetricsTestServer, error) {
	srv := grpc.NewServer(grpc.KeepaliveParams(keepalive.ServerParameters{Time: 5 * time.Minute}))
	lis, err := net.Listen("tcp", "localhost:0")
	if err != nil {
		return nil, err
	}
	testServer := &MetricsTestServer{
		Endpoint: lis.Addr().String(),
		lis:      lis,
		srv:      srv,
	}

	monitoringpb.RegisterMetricServiceServer(
		srv,
		&fakeMetricServiceServer{metricsTestServer: testServer},
	)

	return testServer, nil
}

func requireNoError(t *testing.T, err error) {
	if err != nil {
		t.Fatalf("Received unexpected error: \n%v", err)
	}
}

func assertNoError(t *testing.T, err error) {
	if err != nil {
		t.Errorf("Received unexpected error: \n%v", err)
	}
}

func assertErrorIs(t *testing.T, gotErr error, wantErr error) {
	if !errors.Is(gotErr, wantErr) {
		t.Errorf("error got: %v, want: %v", gotErr, wantErr)
	}
}

func assertEqual(t *testing.T, got, want interface{}) {
	if !testutil.Equal(got, want) {
		t.Errorf("got: %+v, want: %+v", got, want)
	}

}

func TestExportMetrics(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Minute)
	defer cancel()

	testServer, err := NewMetricTestServer()
	//nolint:errcheck
	go testServer.Serve()
	defer testServer.Shutdown()
	assertNoError(t, err)

	res := &resource.Resource{}

	clientOpts := []option.ClientOption{
		option.WithEndpoint(testServer.Endpoint),
		option.WithoutAuthentication(),
		option.WithGRPCDialOption(grpc.WithTransportCredentials(insecure.NewCredentials())),
	}
	exporter, err := newMonitoringExporter(ctx, "PROJECT_ID_NOT_REAL", clientOpts...)
	if err != nil {
		t.Errorf("Error occurred when creating exporter: %v", err)
	}

	// Reduce sampling period to reduce test run time
	origSamplePeriod := defaultSamplePeriod
	defaultSamplePeriod = 5 * time.Second
	defer func() {
		defaultSamplePeriod = origSamplePeriod
	}()
	provider := metric.NewMeterProvider(
		metric.WithReader(metric.NewPeriodicReader(exporter, metric.WithInterval(defaultSamplePeriod))),
		metric.WithResource(res),
	)

	//nolint:errcheck
	defer func() {
		err = provider.Shutdown(ctx)
		assertNoError(t, err)
	}()

	meterBuiltIn := provider.Meter(builtInMetricsMeterName)
	counterBuiltIn, err := meterBuiltIn.Int64Counter("name.lastvalue")
	requireNoError(t, err)

	meterNameNotBuiltIn := "testing"
	meterNotbuiltIn := provider.Meter(meterNameNotBuiltIn)
	counterNotBuiltIn, err := meterNotbuiltIn.Int64Counter("name.lastvalue")
	requireNoError(t, err)

	// record start time
	testStartTime := time.Now()

	// record data points
	counterBuiltIn.Add(ctx, 1)
	counterNotBuiltIn.Add(ctx, 1)

	// Calculate elapsed time
	elapsedTime := time.Since(testStartTime)
	if elapsedTime < 3*defaultSamplePeriod {
		// Ensure at least 2 datapoints are recorded
		time.Sleep(3*defaultSamplePeriod - elapsedTime)
	}

	gotCalls := testServer.CreateServiceTimeSeriesRequests()
	for _, gotCall := range gotCalls {
		for _, ts := range gotCall.TimeSeries {
			if strings.Contains(ts.Metric.Type, meterNameNotBuiltIn) {
				t.Errorf("Exporter should only export builtin metrics")
			}
		}
	}
}

func TestExportCounter(t *testing.T) {
	ctx := context.Background()
	testServer, err := NewMetricTestServer()
	//nolint:errcheck
	go testServer.Serve()
	defer testServer.Shutdown()
	assertNoError(t, err)

	clientOpts := []option.ClientOption{
		option.WithEndpoint(testServer.Endpoint),
		option.WithoutAuthentication(),
		option.WithGRPCDialOption(grpc.WithTransportCredentials(insecure.NewCredentials())),
	}
	exporter, err := newMonitoringExporter(ctx, "PROJECT_ID_NOT_REAL", clientOpts...)
	assertNoError(t, err)
	provider := metric.NewMeterProvider(
		metric.WithReader(metric.NewPeriodicReader(exporter)),
		metric.WithResource(
			resource.NewWithAttributes(
				semconv.SchemaURL,
				attribute.String("test_id", "abc123"),
			)),
	)

	//nolint:errcheck
	defer func() {
		err = provider.Shutdown(ctx)
		assertNoError(t, err)
	}()

	// Start meter
	meter := provider.Meter(builtInMetricsMeterName)

	// Register counter value
	counter, err := meter.Int64Counter("counter-a")
	assertNoError(t, err)
	clabels := []attribute.KeyValue{attribute.Key("key").String("value")}
	counter.Add(ctx, 100, otelmetric.WithAttributes(clabels...))
}

func TestExportHistogram(t *testing.T) {
	ctx := context.Background()
	testServer, err := NewMetricTestServer()
	//nolint:errcheck
	go testServer.Serve()
	defer testServer.Shutdown()
	assertNoError(t, err)

	clientOpts := []option.ClientOption{
		option.WithEndpoint(testServer.Endpoint),
		option.WithoutAuthentication(),
		option.WithGRPCDialOption(grpc.WithTransportCredentials(insecure.NewCredentials())),
	}
	exporter, err := newMonitoringExporter(ctx, "PROJECT_ID_NOT_REAL", clientOpts...)
	assertNoError(t, err)
	provider := metric.NewMeterProvider(
		metric.WithReader(metric.NewPeriodicReader(exporter)),
		metric.WithResource(
			resource.NewWithAttributes(
				semconv.SchemaURL,
				attribute.String("test_id", "abc123"),
			),
		),
	)
	assertNoError(t, err)

	//nolint:errcheck
	defer func() {
		err = provider.Shutdown(ctx)
		assertNoError(t, err)
	}()

	// Start meter
	meter := provider.Meter(builtInMetricsMeterName)

	// Register counter value
	counter, err := meter.Float64Histogram("counter-a")
	assertNoError(t, err)
	clabels := []attribute.KeyValue{attribute.Key("key").String("value")}
	counter.Record(ctx, 100, otelmetric.WithAttributes(clabels...))
	counter.Record(ctx, 50, otelmetric.WithAttributes(clabels...))
	counter.Record(ctx, 200, otelmetric.WithAttributes(clabels...))
}

func TestRecordToMpb(t *testing.T) {
	metricName := "testing"

	me := &monitoringExporter{}

	monitoredResLabelValueProject := "project01"
	monitoredResLabelValueInstance := "instance01"
	monitoredResLabelValueZone := "zone01"
	monitoredResLabelValueTable := "table01"
	monitoredResLabelValueCluster := "cluster01"

	inputAttributes := attribute.NewSet(
		attribute.Key("a").String("A"),
		attribute.Key("b").Int64(100),
		attribute.Key(monitoredResLabelKeyProject).String(monitoredResLabelValueProject),
		attribute.Key(monitoredResLabelKeyInstance).String(monitoredResLabelValueInstance),
		attribute.Key(monitoredResLabelKeyZone).String(monitoredResLabelValueZone),
		attribute.Key(monitoredResLabelKeyTable).String(monitoredResLabelValueTable),
		attribute.Key(monitoredResLabelKeyCluster).String(monitoredResLabelValueCluster),
	)
	inputMetrics := metricdata.Metrics{
		Name: metricName,
	}

	wantMetric := &googlemetricpb.Metric{
		Type: fmt.Sprintf("%v%s", builtInMetricsMeterName, metricName),
		Labels: map[string]string{
			"a": "A",
			"b": "100",
		},
	}

	wantMonitoredResource := &monitoredrespb.MonitoredResource{
		Type: "bigtable_client_raw",
		Labels: map[string]string{
			monitoredResLabelKeyProject:  monitoredResLabelValueProject,
			monitoredResLabelKeyInstance: monitoredResLabelValueInstance,
			monitoredResLabelKeyZone:     monitoredResLabelValueZone,
			monitoredResLabelKeyTable:    monitoredResLabelValueTable,
			monitoredResLabelKeyCluster:  monitoredResLabelValueCluster,
		},
	}

	gotMetric, gotMonitoredResource := me.recordToMetricAndMonitoredResourcePbs(inputMetrics, inputAttributes)
	if !reflect.DeepEqual(wantMetric, gotMetric) {
		t.Errorf("Metric: expected: %v, actual: %v", wantMetric, gotMetric)
	}
	if !reflect.DeepEqual(wantMonitoredResource, gotMonitoredResource) {
		t.Errorf("Monitored resource: expected: %v, actual: %v", wantMonitoredResource, gotMonitoredResource)
	}
}

func TestTimeIntervalStaggering(t *testing.T) {
	var tm time.Time

	interval, err := toNonemptyTimeIntervalpb(tm, tm)
	if err != nil {
		t.Fatalf("conversion to PB failed: %v", err)
	}

	if err := interval.StartTime.CheckValid(); err != nil {
		t.Fatalf("unable to convert start time from PB: %v", err)
	}
	start := interval.StartTime.AsTime()

	if err := interval.EndTime.CheckValid(); err != nil {
		t.Fatalf("unable to convert end time to PB: %v", err)
	}
	end := interval.EndTime.AsTime()

	if end.Before(start.Add(time.Millisecond)) {
		t.Fatalf("expected end=%v to be at least %v after start=%v, but it wasn't", end, time.Millisecond, start)
	}
}

func TestTimeIntervalPassthru(t *testing.T) {
	var tm time.Time

	interval, err := toNonemptyTimeIntervalpb(tm, tm.Add(time.Second))
	if err != nil {
		t.Fatalf("conversion to PB failed: %v", err)
	}

	if err := interval.StartTime.CheckValid(); err != nil {
		t.Fatalf("unable to convert start time from PB: %v", err)
	}
	start := interval.StartTime.AsTime()

	if err := interval.EndTime.CheckValid(); err != nil {
		t.Fatalf("unable to convert end time to PB: %v", err)
	}
	end := interval.EndTime.AsTime()

	assertEqual(t, start, tm)
	assertEqual(t, end, tm.Add(time.Second))
}

func TestConcurrentCallsAfterShutdown(t *testing.T) {
	testServer, err := NewMetricTestServer()
	//nolint:errcheck
	go testServer.Serve()
	defer testServer.Shutdown()
	assertNoError(t, err)

	ctx := context.Background()
	clientOpts := []option.ClientOption{
		option.WithEndpoint(testServer.Endpoint),
		option.WithoutAuthentication(),
		option.WithGRPCDialOption(grpc.WithTransportCredentials(insecure.NewCredentials())),
	}
	exporter, err := newMonitoringExporter(ctx, "PROJECT_ID_NOT_REAL", clientOpts...)
	assertNoError(t, err)

	err = exporter.Shutdown(ctx)
	assertNoError(t, err)

	var wg sync.WaitGroup
	wg.Add(3)

	go func() {
		err := exporter.Shutdown(ctx)
		assertErrorIs(t, err, errShutdown)
		wg.Done()
	}()
	go func() {
		err := exporter.ForceFlush(ctx)
		assertNoError(t, err)
		wg.Done()
	}()
	go func() {
		err := exporter.Export(ctx, &metricdata.ResourceMetrics{})
		assertErrorIs(t, err, errShutdown)
		wg.Done()
	}()

	wg.Wait()
}

func TestConcurrentExport(t *testing.T) {
	testServer, err := NewMetricTestServer()
	//nolint:errcheck
	go testServer.Serve()
	defer testServer.Shutdown()
	assertNoError(t, err)

	ctx := context.Background()
	clientOpts := []option.ClientOption{
		option.WithEndpoint(testServer.Endpoint),
		option.WithoutAuthentication(),
		option.WithGRPCDialOption(grpc.WithTransportCredentials(insecure.NewCredentials())),
	}
	exporter, err := newMonitoringExporter(ctx, "PROJECT_ID_NOT_REAL", clientOpts...)
	assertNoError(t, err)

	defer func() {
		err := exporter.Shutdown(ctx)
		assertNoError(t, err)
	}()

	var wg sync.WaitGroup
	wg.Add(2)

	go func() {
		err := exporter.Export(ctx, &metricdata.ResourceMetrics{
			ScopeMetrics: []metricdata.ScopeMetrics{
				{
					Metrics: []metricdata.Metrics{
						{Name: "testing", Data: metricdata.Histogram[float64]{}},
						{Name: "test/of/path", Data: metricdata.Histogram[float64]{}},
					},
				},
			},
		})
		assertNoError(t, err)
		wg.Done()
	}()
	go func() {
		err := exporter.Export(ctx, &metricdata.ResourceMetrics{
			ScopeMetrics: []metricdata.ScopeMetrics{
				{
					Metrics: []metricdata.Metrics{
						{Name: "testing", Data: metricdata.Histogram[float64]{}},
						{Name: "test/of/path", Data: metricdata.Histogram[float64]{}},
					},
				},
			},
		})
		assertNoError(t, err)
		wg.Done()
	}()

	wg.Wait()
}

func TestBatchingExport(t *testing.T) {
	ctx := context.Background()
	setup := func(t *testing.T) (metric.Exporter, *MetricsTestServer) {
		testServer, err := NewMetricTestServer()
		//nolint:errcheck
		go testServer.Serve()
		t.Cleanup(testServer.Shutdown)

		assertNoError(t, err)

		clientOpts := []option.ClientOption{
			option.WithEndpoint(testServer.Endpoint),
			option.WithoutAuthentication(),
			option.WithGRPCDialOption(grpc.WithTransportCredentials(insecure.NewCredentials())),
		}
		exporter, err := newMonitoringExporter(ctx, "PROJECT_ID_NOT_REAL", clientOpts...)
		assertNoError(t, err)

		t.Cleanup(func() {
			ctx := context.Background()
			err := exporter.Shutdown(ctx)
			assertNoError(t, err)
		})

		return exporter, testServer
	}

	createMetrics := func(n int) []metricdata.Metrics {
		inputMetrics := make([]metricdata.Metrics, n)
		for i := 0; i < n; i++ {
			inputMetrics[i] = metricdata.Metrics{Name: "testing", Data: metricdata.Histogram[float64]{
				DataPoints: []metricdata.HistogramDataPoint[float64]{
					{},
				},
			}}
		}

		return inputMetrics
	}

	for _, tc := range []struct {
		desc                  string
		numMetrics            int
		expectedCreateTSCalls int
	}{
		{desc: "0 metrics"},
		{
			desc:                  "150 metrics",
			numMetrics:            150,
			expectedCreateTSCalls: 1,
		},
		{
			desc:                  "200 metrics",
			numMetrics:            200,
			expectedCreateTSCalls: 1,
		},
		{
			desc:                  "201 metrics",
			numMetrics:            201,
			expectedCreateTSCalls: 2,
		},
		{
			desc:                  "500 metrics",
			numMetrics:            500,
			expectedCreateTSCalls: 3,
		},
		{
			desc:                  "1199 metrics",
			numMetrics:            1199,
			expectedCreateTSCalls: 6,
		},
	} {
		t.Run(tc.desc, func(t *testing.T) {
			exporter, testServer := setup(t)
			input := createMetrics(tc.numMetrics)

			err := exporter.Export(ctx, &metricdata.ResourceMetrics{
				ScopeMetrics: []metricdata.ScopeMetrics{
					{
						Scope: instrumentation.Scope{
							Name: builtInMetricsMeterName,
						},
						Metrics: input,
					},
				},
			})
			assertNoError(t, err)

			gotCalls := testServer.CreateServiceTimeSeriesRequests()
			assertEqual(t, len(gotCalls), tc.expectedCreateTSCalls)
		})
	}
}

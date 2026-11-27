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

package storage

import (
	"context"
	"errors"
	"fmt"
	"net"
	"sync"
	"testing"
	"time"

	"cloud.google.com/go/monitoring/apiv3/v2/monitoringpb"
	"github.com/google/go-cmp/cmp"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/metric"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/metric/exemplar"
	"go.opentelemetry.io/otel/sdk/metric/metricdata"
	"go.opentelemetry.io/otel/sdk/resource"
	"go.opentelemetry.io/otel/trace"
	"google.golang.org/api/option"
	googlemetricpb "google.golang.org/genproto/googleapis/api/metric"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/emptypb"
)

type fakeMetricServiceServer struct {
	monitoringpb.UnimplementedMetricServiceServer

	mu            sync.Mutex
	getMDCalls    int
	createdMDs    map[string]*googlemetricpb.MetricDescriptor
	tsRequests    []*monitoringpb.CreateTimeSeriesRequest
	svcTSRequests []*monitoringpb.CreateTimeSeriesRequest
}

func newFakeMetricServiceServer(t *testing.T) (*fakeMetricServiceServer, *grpc.ClientConn) {
	t.Helper()
	lis, err := net.Listen("tcp", "localhost:0")
	if err != nil {
		t.Fatalf("net.Listen: %v", err)
	}
	srv := &fakeMetricServiceServer{
		createdMDs: make(map[string]*googlemetricpb.MetricDescriptor),
	}
	gsrv := grpc.NewServer()
	monitoringpb.RegisterMetricServiceServer(gsrv, srv)
	go func() {
		_ = gsrv.Serve(lis)
	}()
	t.Cleanup(gsrv.Stop)

	conn, err := grpc.NewClient(lis.Addr().String(), grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		t.Fatalf("grpc.NewClient: %v", err)
	}
	t.Cleanup(func() { _ = conn.Close() })
	return srv, conn
}

func (s *fakeMetricServiceServer) GetMetricDescriptor(_ context.Context, req *monitoringpb.GetMetricDescriptorRequest) (*googlemetricpb.MetricDescriptor, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.getMDCalls++
	if md, ok := s.createdMDs[req.GetName()]; ok {
		return md, nil
	}
	return nil, status.Error(codes.NotFound, "metric descriptor not found")
}

func (s *fakeMetricServiceServer) CreateMetricDescriptor(_ context.Context, req *monitoringpb.CreateMetricDescriptorRequest) (*googlemetricpb.MetricDescriptor, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	md := req.GetMetricDescriptor()
	name := fmt.Sprintf("%s/metricDescriptors/%s", req.GetName(), md.GetType())
	s.createdMDs[name] = md
	return md, nil
}

func (s *fakeMetricServiceServer) CreateTimeSeries(_ context.Context, req *monitoringpb.CreateTimeSeriesRequest) (*emptypb.Empty, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.tsRequests = append(s.tsRequests, req)
	return &emptypb.Empty{}, nil
}

func (s *fakeMetricServiceServer) CreateServiceTimeSeries(_ context.Context, req *monitoringpb.CreateTimeSeriesRequest) (*emptypb.Empty, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.svcTSRequests = append(s.svcTSRequests, req)
	return &emptypb.Empty{}, nil
}

func TestMonitoringExporter_ServiceTimeSeriesAndExemplars(t *testing.T) {
	ctx := context.Background()
	srv, conn := newFakeMetricServiceServer(t)

	smr, err := newStorageMonitoredResource(ctx, "test-proj", "grpc", resource.WithAttributes(
		attribute.String("cloud.region", "us-central1"),
		attribute.String("cloud.platform", "gcp_compute_engine"),
		attribute.String("host.id", "vm-123"),
	))
	if err != nil {
		t.Fatalf("newStorageMonitoredResource: %v", err)
	}

	exp, err := smr.exporter(ctx, option.WithGRPCConn(conn))
	if err != nil {
		t.Fatalf("smr.exporter: %v", err)
	}
	defer func() { _ = exp.Shutdown(ctx) }()

	reader := sdkmetric.NewManualReader()
	provider := sdkmetric.NewMeterProvider(
		sdkmetric.WithReader(reader),
		sdkmetric.WithResource(smr.resource),
		sdkmetric.WithExemplarFilter(exemplar.TraceBasedFilter),
		sdkmetric.WithView(
			sdkmetric.NewView(
				sdkmetric.Instrument{Name: "gcp.storage.client.ttfb", Kind: sdkmetric.InstrumentKindHistogram},
				sdkmetric.Stream{Aggregation: sdkmetric.AggregationExplicitBucketHistogram{Boundaries: []float64{0.01, 0.1, 1.0}}},
			),
		),
	)
	defer func() { _ = provider.Shutdown(ctx) }()

	meter := provider.Meter("cloud.google.com/go/storage")
	ops, err := meter.Int64Counter("gcp.storage.client.operations", metric.WithUnit("{operation}"))
	if err != nil {
		t.Fatalf("Int64Counter: %v", err)
	}
	active, err := meter.Int64UpDownCounter("gcp.storage.client.request.active", metric.WithUnit("{request}"))
	if err != nil {
		t.Fatalf("Int64UpDownCounter: %v", err)
	}
	ttfb, err := meter.Float64Histogram("gcp.storage.client.ttfb", metric.WithUnit("s"))
	if err != nil {
		t.Fatalf("Float64Histogram: %v", err)
	}

	traceID, _ := trace.TraceIDFromHex("0102030405060708090a0b0c0d0e0f10")
	spanID, _ := trace.SpanIDFromHex("1112131415161718")
	sampledCtx := trace.ContextWithSpanContext(ctx, trace.NewSpanContext(trace.SpanContextConfig{
		TraceID:    traceID,
		SpanID:     spanID,
		TraceFlags: trace.FlagsSampled,
	}))

	ops.Add(ctx, 3, metric.WithAttributes(
		attribute.String("gcp.storage.operation", "ReadObject"),
		attribute.Int("http.response.status_code", 200),
	))
	active.Add(ctx, 2, metric.WithAttributes(attribute.String("rpc.system.name", "grpc")))
	ttfb.Record(sampledCtx, 0.05, metric.WithAttributes(attribute.String("network.path", "directpath")))

	var rm metricdata.ResourceMetrics
	if err := reader.Collect(ctx, &rm); err != nil {
		t.Fatalf("reader.Collect: %v", err)
	}
	if err := exp.Export(ctx, &rm); err != nil {
		t.Fatalf("exp.Export: %v", err)
	}

	srv.mu.Lock()
	defer srv.mu.Unlock()

	if len(srv.createdMDs) != 0 || len(srv.tsRequests) != 0 {
		t.Fatalf("expected 0 CreateMetricDescriptor and 0 CreateTimeSeries calls, got %d MDs and %d TS", len(srv.createdMDs), len(srv.tsRequests))
	}
	if len(srv.svcTSRequests) != 1 {
		t.Fatalf("expected 1 CreateServiceTimeSeries call, got %d", len(srv.svcTSRequests))
	}
	req := srv.svcTSRequests[0]
	if req.GetName() != "projects/test-proj" {
		t.Errorf("request Name = %q, want %q", req.GetName(), "projects/test-proj")
	}
	if len(req.GetTimeSeries()) != 3 {
		t.Fatalf("got %d TimeSeries, want 3", len(req.GetTimeSeries()))
	}

	byType := make(map[string]*monitoringpb.TimeSeries)
	for _, ts := range req.GetTimeSeries() {
		byType[ts.GetMetric().GetType()] = ts
		if got := ts.GetResource().GetType(); got != monitoredResourceName {
			t.Errorf("Resource.Type = %q, want %q", got, monitoredResourceName)
		}
		wantResourceLabels := map[string]string{
			"project_id":     "test-proj",
			"location":       "us-central1",
			"cloud_platform": "gcp_compute_engine",
			"host_id":        "vm-123",
			"instance_id":    smr.instance,
			"api":            "grpc",
		}
		if diff := cmp.Diff(wantResourceLabels, ts.GetResource().GetLabels()); diff != "" {
			t.Errorf("Resource.Labels mismatch (-want +got):\n%s", diff)
		}
	}

	opsTS := byType["storage.googleapis.com/client/gcp/storage/client/operations"]
	if opsTS == nil {
		t.Fatal("missing operations TimeSeries")
	}
	if opsTS.GetMetricKind() != googlemetricpb.MetricDescriptor_CUMULATIVE {
		t.Errorf("operations MetricKind = %v, want CUMULATIVE", opsTS.GetMetricKind())
	}
	if got := opsTS.GetMetric().GetLabels()["gcp_storage_operation"]; got != "ReadObject" {
		t.Errorf("gcp_storage_operation = %q, want ReadObject", got)
	}
	if got := opsTS.GetMetric().GetLabels()["http_response_status_code"]; got != "200" {
		t.Errorf("http_response_status_code = %q, want 200", got)
	}

	activeTS := byType["storage.googleapis.com/client/gcp/storage/client/request/active"]
	if activeTS == nil {
		t.Fatal("missing request.active TimeSeries")
	}
	if activeTS.GetMetricKind() != googlemetricpb.MetricDescriptor_GAUGE {
		t.Errorf("request.active MetricKind = %v, want GAUGE", activeTS.GetMetricKind())
	}
	if activeTS.GetPoints()[0].GetInterval().GetStartTime() != nil {
		t.Errorf("GAUGE point StartTime = %v, want nil", activeTS.GetPoints()[0].GetInterval().GetStartTime())
	}

	ttfbTS := byType["storage.googleapis.com/client/gcp/storage/client/ttfb"]
	if ttfbTS == nil {
		t.Fatal("missing ttfb TimeSeries")
	}
	if len(ttfbTS.GetPoints()) == 0 {
		t.Fatalf("ttfb timeseries has 0 points")
	}
	dist := ttfbTS.GetPoints()[0].GetValue().GetDistributionValue()
	if dist.GetCount() != 1 || dist.GetMean() != 0.05 {
		t.Errorf("ttfb distribution Count=%d Mean=%v, want Count=1 Mean=0.05", dist.GetCount(), dist.GetMean())
	}
	if len(dist.GetExemplars()) != 1 {
		t.Fatalf("ttfb exemplars count = %d, want 1", len(dist.GetExemplars()))
	}
	if len(dist.GetExemplars()[0].GetAttachments()) == 0 {
		t.Fatalf("ttfb exemplar has 0 attachments")
	}
	var spanCtx monitoringpb.SpanContext
	if err := dist.GetExemplars()[0].GetAttachments()[0].UnmarshalTo(&spanCtx); err != nil {
		t.Fatalf("UnmarshalTo(SpanContext): %v", err)
	}
	wantSpan := "projects/test-proj/traces/0102030405060708090a0b0c0d0e0f10/spans/1112131415161718"
	if spanCtx.GetSpanName() != wantSpan {
		t.Errorf("exemplar SpanName = %q, want %q", spanCtx.GetSpanName(), wantSpan)
	}
}

func TestMonitoringExporter_CustomTimeSeriesBatchingAndDescriptors(t *testing.T) {
	ctx := context.Background()
	srv, conn := newFakeMetricServiceServer(t)

	exp, err := newMetricsGCMExporter(ctx, "custom-proj", option.WithGRPCConn(conn))
	if err != nil {
		t.Fatalf("newMetricsGCMExporter: %v", err)
	}

	res, err := resource.New(ctx, resource.WithAttributes(
		attribute.String("gcp.client.service", "storage"),
		attribute.String("cloud.region", "europe-west1"),
		attribute.String("host.id", "node-7"),
		attribute.String("ignored.attr", "dropped"),
	))
	if err != nil {
		t.Fatalf("resource.New: %v", err)
	}

	now := time.Now()
	points := make([]metricdata.DataPoint[int64], 205)
	for i := range points {
		points[i] = metricdata.DataPoint[int64]{
			Attributes: attribute.NewSet(attribute.Int("shard", i)),
			StartTime:  now.Add(-time.Second),
			Time:       now,
			Value:      int64(i + 1),
		}
	}

	rm := &metricdata.ResourceMetrics{
		Resource: res,
		ScopeMetrics: []metricdata.ScopeMetrics{{
			Metrics: []metricdata.Metrics{{
				Name: "gcp.storage.client.attempts",
				Unit: "{attempt}",
				Data: metricdata.Sum[int64]{
					Temporality: metricdata.CumulativeTemporality,
					IsMonotonic: true,
					DataPoints:  points,
				},
			}},
		}},
	}

	// Export twice to verify MetricDescriptor caching after initial creation.
	if err := exp.Export(ctx, rm); err != nil {
		t.Fatalf("first Export: %v", err)
	}
	if err := exp.Export(ctx, rm); err != nil {
		t.Fatalf("second Export: %v", err)
	}

	srv.mu.Lock()
	if srv.getMDCalls != 1 || len(srv.createdMDs) != 1 {
		t.Errorf("GetMetricDescriptor=%d CreatedMDs=%d, want 1 and 1", srv.getMDCalls, len(srv.createdMDs))
	} else {
		wantType := formatMetricWithPrefix(metricdata.Metrics{Name: "gcp.storage.client.attempts"}, customMetricPrefix)
		md := srv.createdMDs["projects/custom-proj/metricDescriptors/"+wantType]
		if md == nil || md.GetType() != wantType || md.GetUnit() != "{attempt}" {
			t.Errorf("MetricDescriptor = %v, want type %q with unit {attempt}", md, wantType)
		}
	}
	// 205 points per export -> 2 batches (200 + 5) per export -> 4 CreateTimeSeries calls total.
	if len(srv.tsRequests) != 4 {
		t.Fatalf("CreateTimeSeries calls = %d, want 4", len(srv.tsRequests))
	}
	for i, wantLen := range []int{200, 5, 200, 5} {
		if got := len(srv.tsRequests[i].GetTimeSeries()); got != wantLen {
			t.Fatalf("batch %d len = %d, want %d", i, got, wantLen)
		}
	}
	firstTS := srv.tsRequests[0].GetTimeSeries()[0]
	if firstTS.GetResource().GetType() != "generic_node" {
		t.Errorf("Resource.Type = %q, want generic_node", firstTS.GetResource().GetType())
	}
	if firstTS.GetResource().GetLabels()["location"] != "europe-west1" {
		t.Errorf("Resource.Labels[location] = %q, want europe-west1", firstTS.GetResource().GetLabels()["location"])
	}
	if firstTS.GetMetric().GetLabels()["gcp_client_service"] != "storage" {
		t.Errorf("Metric.Labels[gcp_client_service] = %q, want storage", firstTS.GetMetric().GetLabels()["gcp_client_service"])
	}
	if _, exists := firstTS.GetMetric().GetLabels()["ignored_attr"]; exists {
		t.Errorf("unexpected filtered resource attribute ignored_attr in metric labels")
	}
	srv.mu.Unlock()

	if err := exp.Shutdown(ctx); err != nil {
		t.Fatalf("Shutdown: %v", err)
	}
	if err := exp.Export(ctx, rm); !errors.Is(err, errMonitoringExporterShutdown) {
		t.Errorf("Export after Shutdown = %v, want %v", err, errMonitoringExporterShutdown)
	}
}

func TestMapDefaultMonitoredResource(t *testing.T) {
	for _, tc := range []struct {
		name     string
		attrs    []attribute.KeyValue
		wantType string
		wantLoc  string
	}{
		{
			name:     "fallback generic_node global",
			attrs:    nil,
			wantType: "generic_node",
			wantLoc:  "global",
		},
		{
			name: "gce_instance",
			attrs: []attribute.KeyValue{
				attribute.String("cloud.platform", "gcp_compute_engine"),
				attribute.String("cloud.availability_zone", "us-east1-b"),
				attribute.String("host.id", "12345"),
			},
			wantType: "gce_instance",
			wantLoc:  "us-east1-b",
		},
		{
			name: "k8s_container",
			attrs: []attribute.KeyValue{
				attribute.String("cloud.region", "us-west1"),
				attribute.String("k8s.cluster.name", "prod-cluster"),
				attribute.String("k8s.namespace.name", "default"),
				attribute.String("k8s.pod.name", "pod-1"),
				attribute.String("k8s.container.name", "app"),
			},
			wantType: "k8s_container",
			wantLoc:  "us-west1",
		},
		{
			name: "generic_task",
			attrs: []attribute.KeyValue{
				attribute.String("service.name", "worker"),
				attribute.String("service.instance.id", "inst-1"),
			},
			wantType: "generic_task",
			wantLoc:  "global",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			set := attribute.NewSet(tc.attrs...)
			mr := mapDefaultMonitoredResource(&set)
			if mr.GetType() != tc.wantType {
				t.Errorf("Type = %q, want %q", mr.GetType(), tc.wantType)
			}
			locKey := "location"
			if tc.wantType == "gce_instance" {
				locKey = "zone"
			}
			if mr.GetLabels()[locKey] != tc.wantLoc {
				t.Errorf("Labels[%q] = %q, want %q", locKey, mr.GetLabels()[locKey], tc.wantLoc)
			}
		})
	}
}

func TestMonitoringExporter_ExemplarEmptyTraceAndTimeInterval(t *testing.T) {
	// Empty TraceID/SpanID slices (when no sampled span is active) must not emit
	// an invalid SpanContext attachment.
	exemplars := toDistributionExemplars([]metricdata.Exemplar[float64]{
		{Value: 0.05, Time: time.Unix(100, 0)},
	}, "test-proj")
	if len(exemplars) != 1 {
		t.Fatalf("len(exemplars) = %d, want 1", len(exemplars))
	}
	if len(exemplars[0].GetAttachments()) != 0 {
		t.Errorf("expected 0 attachments for exemplar without active span, got %v", exemplars[0].GetAttachments())
	}

	// Sub-millisecond intervals are bumped to 1ms, while >= 1ms intervals are preserved.
	start := time.Unix(200, 0)
	subMS, err := toNonemptyTimeInterval(start, start.Add(500*time.Microsecond))
	if err != nil {
		t.Fatalf("toNonemptyTimeInterval(<1ms): %v", err)
	}
	if got := subMS.GetEndTime().AsTime().Sub(subMS.GetStartTime().AsTime()); got != time.Millisecond {
		t.Errorf("sub-ms interval = %v, want 1ms", got)
	}

	onePointFiveMS, err := toNonemptyTimeInterval(start, start.Add(1500*time.Microsecond))
	if err != nil {
		t.Fatalf("toNonemptyTimeInterval(1.5ms): %v", err)
	}
	if got := onePointFiveMS.GetEndTime().AsTime().Sub(onePointFiveMS.GetStartTime().AsTime()); got != 1500*time.Microsecond {
		t.Errorf("1.5ms interval = %v, want 1.5ms", got)
	}
}

type unsyncFakeExporter struct {
	sdkmetric.Exporter
	calls int
}

func (u *unsyncFakeExporter) Temporality(sdkmetric.InstrumentKind) metricdata.Temporality {
	return metricdata.CumulativeTemporality
}

func (u *unsyncFakeExporter) Aggregation(k sdkmetric.InstrumentKind) sdkmetric.Aggregation {
	return sdkmetric.DefaultAggregationSelector(k)
}

func (u *unsyncFakeExporter) Export(context.Context, *metricdata.ResourceMetrics) error {
	u.calls++
	return nil
}

func (u *unsyncFakeExporter) ForceFlush(context.Context) error {
	u.calls++
	return nil
}

func (u *unsyncFakeExporter) Shutdown(context.Context) error {
	return nil
}

func TestSharedCustomExporter_SerializesAndPreservesCallerLifecycle(t *testing.T) {
	ctx := context.Background()

	// Verify concurrent Export and ForceFlush calls serialize safely under -race
	// against an unsynchronized exporter.
	unsync := &unsyncFakeExporter{}
	unsyncExp := sdkmetric.Exporter(unsync)
	wrappedUnsync := wrapCustomExporter(&unsyncExp)
	if again := wrapCustomExporter(wrappedUnsync); again != wrappedUnsync {
		t.Fatalf("wrapCustomExporter was not idempotent")
	}
	var wg sync.WaitGroup
	for i := 0; i < 4; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_ = (*wrappedUnsync).Export(ctx, &metricdata.ResourceMetrics{})
			_ = (*wrappedUnsync).ForceFlush(ctx)
		}()
	}
	wg.Wait()
	if unsync.calls != 8 {
		t.Errorf("unsyncFakeExporter calls = %d, want 8", unsync.calls)
	}

	srv, conn := newFakeMetricServiceServer(t)
	rawExp, err := newMonitoringExporter(ctx, &monitoringExporterOptions{
		projectID:               "shared-proj",
		monitoringClientOptions: []option.ClientOption{option.WithGRPCConn(conn)},
	})
	if err != nil {
		t.Fatalf("newMonitoringExporter: %v", err)
	}
	defer func() { _ = rawExp.Shutdown(ctx) }()

	customExp := sdkmetric.Exporter(rawExp)
	wrapped := wrapCustomExporter(&customExp)

	cfg := storageConfig{
		enableOtelMetrics: true,
		metricExporter:    wrapped,
	}

	// disableExporter: true (as passed when default GCM export is disabled) must
	// still wire a caller-supplied customExporter.
	mc, err := newGRPCMetricContext(ctx, metricsConfig{
		project:         "shared-proj",
		customExporter:  cfg.metricExporter,
		disableExporter: true,
	})
	if err != nil {
		t.Fatalf("newGRPCMetricContext: %v", err)
	}
	cm, cleanup, err := initMetrics(ctx, "shared-proj", &cfg)
	if err != nil {
		t.Fatalf("initMetrics: %v", err)
	}

	// Record a metric on mc.provider before mc.close() to prove customExporter
	// is wired even when disableExporter is true, then verify closing mc does
	// not shut down rawExp before cleanup() flushes pending GCS metrics.
	c, err := mc.provider.Meter("test").Int64Counter("test.grpc.metric")
	if err != nil {
		t.Fatalf("Int64Counter: %v", err)
	}
	c.Add(ctx, 1)
	mc.close()

	srv.mu.Lock()
	afterMCClose := len(srv.tsRequests)
	srv.mu.Unlock()
	if afterMCClose != 1 {
		t.Fatalf("tsRequests after mc.close() = %d, want 1", afterMCClose)
	}

	cm.operations.Add(ctx, 1, metric.WithAttributes(attribute.String("gcp.storage.operation", "GetObject")))
	cleanup()

	srv.mu.Lock()
	defer srv.mu.Unlock()
	if len(srv.tsRequests) != 2 {
		t.Fatalf("tsRequests after cleanup() = %d, want 2", len(srv.tsRequests))
	}
}

func TestMonitoringExporter_HelpersAndValidation(t *testing.T) {
	ctx := context.Background()
	if _, err := newMonitoringExporter(ctx, &monitoringExporterOptions{}); !errors.Is(err, errBlankProjectID) {
		t.Errorf("newMonitoringExporter(blank project) = %v, want %v", err, errBlankProjectID)
	}
	for _, tc := range []struct {
		in   string
		want string
	}{
		{in: "gcp.storage.operation", want: "gcp_storage_operation"},
		{in: "123metric", want: "key_123metric"},
		{in: "a/b-c", want: "a_b_c"},
	} {
		if got := normalizeMonitoringLabelKey(tc.in); got != tc.want {
			t.Errorf("normalizeMonitoringLabelKey(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
	if got := sanitizeMonitoringUTF8("ok-\xff-end"); got != "ok-\ufffd-end" {
		t.Errorf("sanitizeMonitoringUTF8 = %q, want replacement char", got)
	}

	// Verify canceled context short-circuit, nil m.Data skip, and Histogram[int64].
	srv, conn := newFakeMetricServiceServer(t)
	exp, err := newMetricsGCMExporter(ctx, "edge-proj", option.WithGRPCConn(conn))
	if err != nil {
		t.Fatalf("newMetricsGCMExporter: %v", err)
	}
	defer func() { _ = exp.Shutdown(ctx) }()

	now := time.Now()
	rm := &metricdata.ResourceMetrics{
		ScopeMetrics: []metricdata.ScopeMetrics{{
			Metrics: []metricdata.Metrics{
				{Name: "nil.data.metric", Data: nil},
				{
					Name: "gcp.storage.client.request.body.size",
					Unit: "By",
					Data: metricdata.Histogram[int64]{
						Temporality: metricdata.CumulativeTemporality,
						DataPoints: []metricdata.HistogramDataPoint[int64]{{
							StartTime:    now.Add(-time.Second),
							Time:         now,
							Count:        1,
							Bounds:       []float64{1024},
							BucketCounts: []uint64{1, 0},
							Sum:          64,
						}},
					},
				},
			},
		}},
	}
	if err := exp.Export(ctx, rm); err != nil {
		t.Fatalf("Export(int64 histogram + nil data): %v", err)
	}
	srv.mu.Lock()
	if len(srv.createdMDs) != 1 || len(srv.tsRequests) != 1 {
		t.Errorf("createdMDs=%d tsRequests=%d, want 1 and 1", len(srv.createdMDs), len(srv.tsRequests))
	}
	srv.mu.Unlock()

	cancelCtx, cancel := context.WithCancel(ctx)
	cancel()
	if err := exp.Export(cancelCtx, rm); !errors.Is(err, context.Canceled) {
		t.Errorf("Export(canceled ctx) = %v, want context.Canceled", err)
	}
}

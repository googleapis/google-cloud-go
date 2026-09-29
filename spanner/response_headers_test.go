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
	"fmt"
	"maps"
	"reflect"
	"slices"
	"sort"
	"sync"
	"testing"
	"time"

	"cloud.google.com/go/spanner/apiv1/spannerpb"
	. "cloud.google.com/go/spanner/internal/testutil"
	"github.com/googleapis/gax-go/v2"
	"go.opencensus.io/stats/view"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/sdk/metric/metricdata"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
	oteltrace "go.opentelemetry.io/otel/trace"
	"go.opentelemetry.io/otel/trace/noop"
	"google.golang.org/api/option"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
)

func TestGFELatencySinksEnabled(t *testing.T) {
	origFlag := getGFELatencyMetricsFlag()
	t.Cleanup(func() { setGFELatencyMetricsFlag(origFlag) })

	for _, tc := range []struct {
		name                 string
		ocFlag               bool
		hasOpenCensusContext bool
		otConfig             *openTelemetryConfig
		want                 bool
	}{
		{name: "no sinks", want: false},
		{name: "OpenCensus flag without context", ocFlag: true, want: false},
		{name: "OpenCensus context without flag", hasOpenCensusContext: true, want: false},
		{name: "OpenCensus flag and context", ocFlag: true, hasOpenCensusContext: true, want: true},
		{name: "OpenTelemetry disabled", otConfig: &openTelemetryConfig{}, want: false},
		{name: "OpenTelemetry enabled", otConfig: &openTelemetryConfig{enabled: true}, want: true},
		{name: "OpenCensus and OpenTelemetry enabled", ocFlag: true, hasOpenCensusContext: true, otConfig: &openTelemetryConfig{enabled: true}, want: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			setGFELatencyMetricsFlag(tc.ocFlag)
			if got := gfeLatencySinksEnabled(tc.hasOpenCensusContext, tc.otConfig); got != tc.want {
				t.Fatalf("gfeLatencySinksEnabled(%v, %+v) = %v, want %v", tc.hasOpenCensusContext, tc.otConfig, got, tc.want)
			}
		})
	}
}

// headerCountingStream is a grpc.ClientStream that counts calls to Header.
type headerCountingStream struct {
	grpc.ClientStream
	md          metadata.MD
	headerCalls int
}

func (s *headerCountingStream) Header() (metadata.MD, error) {
	s.headerCalls++
	return s.md, nil
}

func (s *headerCountingStream) Context() context.Context {
	return context.Background()
}

type fakeExecuteStreamingSQLClient struct {
	spannerpb.Spanner_ExecuteStreamingSqlClient
	stream *headerCountingStream
}

func (c *fakeExecuteStreamingSQLClient) Header() (metadata.MD, error) {
	return c.stream.Header()
}

type fakeStreamingReadClient struct {
	spannerpb.Spanner_StreamingReadClient
	stream *headerCountingStream
}

func (c *fakeStreamingReadClient) Header() (metadata.MD, error) {
	return c.stream.Header()
}

func TestCachedStreamClientsCallHeaderOnce(t *testing.T) {
	md := metadata.Pairs("server-timing", "gfet4t7; dur=123")
	for _, tc := range []struct {
		name string
		wrap func(*headerCountingStream) grpc.ClientStream
	}{
		{
			name: "ExecuteStreamingSql",
			wrap: func(s *headerCountingStream) grpc.ClientStream {
				return &cachedExecuteStreamingSQLClient{Spanner_ExecuteStreamingSqlClient: &fakeExecuteStreamingSQLClient{stream: s}}
			},
		},
		{
			name: "StreamingRead",
			wrap: func(s *headerCountingStream) grpc.ClientStream {
				return &cachedStreamingReadClient{Spanner_StreamingReadClient: &fakeStreamingReadClient{stream: s}}
			},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			stream := &headerCountingStream{md: md}
			client := tc.wrap(stream)
			for i := 0; i < 3; i++ {
				got, err := client.Header()
				if err != nil {
					t.Fatalf("Header() failed: %v", err)
				}
				if !reflect.DeepEqual(got, md) {
					t.Fatalf("Header() = %v, want %v", got, md)
				}
			}
			if stream.headerCalls != 1 {
				t.Fatalf("underlying Header() calls = %d, want 1", stream.headerCalls)
			}
		})
	}
}

func TestCaptureStreamServerTiming(t *testing.T) {
	md := metadata.Pairs("server-timing", "gfet4t7; dur=123", "server-timing", "afe; dur=45")
	tracer := sdktrace.NewTracerProvider().Tracer("test")
	nonRecordingSpan := oteltrace.SpanFromContext(context.Background())

	for _, tc := range []struct {
		name            string
		builtInEnabled  bool
		recording       bool
		noTracer        bool
		noAttempt       bool
		wantHeaderCalls int
		wantSpanAttrs   map[string]float64
	}{
		{name: "built-in metrics disabled and span not recording"},
		{name: "nil tracer and span recording", noTracer: true, recording: true, wantHeaderCalls: 1, wantSpanAttrs: map[string]float64{"gfe.latency_ms": 123, "afe.latency_ms": 45}},
		{name: "nil tracer and span not recording", noTracer: true},
		{name: "no current attempt and span not recording", builtInEnabled: true, noAttempt: true},
		{name: "no current attempt and span recording", builtInEnabled: true, noAttempt: true, recording: true, wantHeaderCalls: 1, wantSpanAttrs: map[string]float64{"gfe.latency_ms": 123, "afe.latency_ms": 45}},
		{name: "built-in metrics enabled", builtInEnabled: true, wantHeaderCalls: 1},
		{name: "span recording", recording: true, wantHeaderCalls: 1, wantSpanAttrs: map[string]float64{"gfe.latency_ms": 123, "afe.latency_ms": 45}},
		{name: "built-in metrics enabled and span recording", builtInEnabled: true, recording: true, wantHeaderCalls: 1, wantSpanAttrs: map[string]float64{"gfe.latency_ms": 123, "afe.latency_ms": 45}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			span := nonRecordingSpan
			if tc.recording {
				_, span = tracer.Start(context.Background(), "stream")
			}
			var mt *builtinMetricsTracer
			if !tc.noTracer {
				mt = &builtinMetricsTracer{builtInEnabled: tc.builtInEnabled, currOp: &opTracer{}}
				if !tc.noAttempt {
					mt.currOp.currAttempt = &attemptTracer{}
				}
			}
			stream := &headerCountingStream{md: md}
			captureStreamServerTiming(span, mt, stream)
			if stream.headerCalls != tc.wantHeaderCalls {
				t.Fatalf("Header() calls = %d, want %d", stream.headerCalls, tc.wantHeaderCalls)
			}
			if tc.wantHeaderCalls > 0 && !tc.noTracer && !tc.noAttempt {
				if got, want := mt.currOp.currAttempt.serverTimingMetrics[gfeTimingHeader], 123*time.Millisecond; got != want {
					t.Errorf("server timing %q = %v, want %v", gfeTimingHeader, got, want)
				}
			}
			if tc.recording {
				gotSpanAttrs := map[string]float64{}
				for _, attr := range span.(sdktrace.ReadOnlySpan).Attributes() {
					gotSpanAttrs[string(attr.Key)] = attr.Value.AsFloat64()
				}
				if len(tc.wantSpanAttrs) == 0 && len(gotSpanAttrs) != 0 {
					t.Errorf("span attributes = %v, want none", gotSpanAttrs)
				}
				for k, want := range tc.wantSpanAttrs {
					if got, ok := gotSpanAttrs[k]; !ok || got != want {
						t.Errorf("span attribute %q = %v (present %v), want %v", k, got, ok, want)
					}
				}
			}
		})
	}
}

// streamHeaderCounter counts Header calls on the gRPC client streams created
// for each method.
type streamHeaderCounter struct {
	mu    sync.Mutex
	calls map[string]int
}

func (c *streamHeaderCounter) interceptor(ctx context.Context, desc *grpc.StreamDesc, cc *grpc.ClientConn, method string, streamer grpc.Streamer, opts ...grpc.CallOption) (grpc.ClientStream, error) {
	s, err := streamer(ctx, desc, cc, method, opts...)
	if err != nil {
		return s, err
	}
	return &countedClientStream{ClientStream: s, counter: c, method: method}, nil
}

func (c *streamHeaderCounter) get(method string) int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.calls[method]
}

type countedClientStream struct {
	grpc.ClientStream
	counter *streamHeaderCounter
	method  string
}

func (s *countedClientStream) Header() (metadata.MD, error) {
	s.counter.mu.Lock()
	s.counter.calls[s.method]++
	s.counter.mu.Unlock()
	return s.ClientStream.Header()
}

// TestStreamHeadersOnlyReadWhenUsed runs single and partitioned queries and
// reads through the public client, and verifies that the stream headers are
// read once per stream only when a sink uses them, and that every enabled sink
// still records the latency of each method.
func TestStreamHeadersOnlyReadWhenUsed(t *testing.T) {
	const (
		executeStreamingSQL = "/google.spanner.v1.Spanner/ExecuteStreamingSql"
		streamingRead       = "/google.spanner.v1.Spanner/StreamingRead"
	)
	// The mock server sends "gfet4t7; dur=123" for StreamingRead. Send a
	// different value for ExecuteStreamingSql to tell the methods apart.
	queryTiming := grpc.StreamInterceptor(func(srv any, stream grpc.ServerStream, info *grpc.StreamServerInfo, handler grpc.StreamHandler) error {
		if info.FullMethod == executeStreamingSQL {
			if err := stream.SetHeader(metadata.Pairs("server-timing", "gfet4t7; dur=456")); err != nil {
				return err
			}
		}
		return handler(srv, stream)
	})
	// Other tests in this package enable OpenCensus GFE latency metrics and
	// install a recording tracer provider, so set every sink explicitly.
	origOTFlag := IsOpenTelemetryMetricsEnabled()
	origOCFlag := getGFELatencyMetricsFlag()
	origTracerProvider := otel.GetTracerProvider()
	t.Cleanup(func() {
		setOpenTelemetryMetricsFlag(origOTFlag)
		setGFELatencyMetricsFlag(origOCFlag)
		otel.SetTracerProvider(origTracerProvider)
	})

	for _, tc := range []struct {
		name                               string
		openCensus, openTelemetry, builtIn bool
		tracing                            bool
		wantHeaderCallsPerMethod           int
	}{
		{name: "no sinks", wantHeaderCallsPerMethod: 0},
		{name: "OpenCensus", openCensus: true, wantHeaderCallsPerMethod: 2},
		{name: "OpenTelemetry", openTelemetry: true, wantHeaderCallsPerMethod: 2},
		{name: "built-in metrics", builtIn: true, wantHeaderCallsPerMethod: 2},
		{name: "tracing", tracing: true, wantHeaderCallsPerMethod: 2},
		{name: "all sinks", openCensus: true, openTelemetry: true, builtIn: true, tracing: true, wantHeaderCallsPerMethod: 2},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ctx := context.Background()
			setGFELatencyMetricsFlag(tc.openCensus)
			setOpenTelemetryMetricsFlag(tc.openTelemetry)
			if err := view.Register(GFELatencyView); err != nil {
				t.Fatal(err)
			}
			defer view.Unregister(GFELatencyView)
			spans := tracetest.NewInMemoryExporter()
			if tc.tracing {
				tp := sdktrace.NewTracerProvider(sdktrace.WithSyncer(spans))
				defer tp.Shutdown(ctx)
				otel.SetTracerProvider(tp)
			} else {
				otel.SetTracerProvider(noop.NewTracerProvider())
			}
			config := ClientConfig{DisableNativeMetrics: true}
			otReader, otProvider := newTestMeterProvider()
			config.OpenTelemetryMeterProvider = otProvider
			builtInReader, builtInProvider := newTestMeterProvider()
			if tc.builtIn {
				config.ClientMetricsProvider = builtInProvider
			}
			counter := &streamHeaderCounter{calls: map[string]int{}}
			server, opts, serverTeardown := NewMockedSpannerInMemTestServer(t, queryTiming)
			defer serverTeardown()
			opts = append(opts, option.WithGRPCDialOption(grpc.WithChainStreamInterceptor(counter.interceptor)))
			client, err := NewClientWithConfig(ctx, "projects/p/instances/i/databases/d", config, opts...)
			if err != nil {
				t.Fatal(err)
			}
			defer client.Close()

			columns := []string{"SingerId", "AlbumId", "AlbumTitle"}
			if err := client.Single().Query(ctx, NewStatement(SelectSingerIDAlbumIDAlbumTitleFromAlbums)).Do(func(*Row) error { return nil }); err != nil {
				t.Fatalf("Query failed: %v", err)
			}
			if err := client.Single().Read(ctx, "Albums", AllKeys(), columns).Do(func(*Row) error { return nil }); err != nil {
				t.Fatalf("Read failed: %v", err)
			}
			txn, err := client.BatchReadOnlyTransaction(ctx, StrongRead())
			if err != nil {
				t.Fatal(err)
			}
			defer txn.Cleanup(ctx)
			queryPartitions, err := txn.PartitionQuery(ctx, NewStatement(SelectSingerIDAlbumIDAlbumTitleFromAlbums), PartitionOptions{MaxPartitions: 1})
			if err != nil {
				t.Fatal(err)
			}
			readPartitions, err := txn.PartitionRead(ctx, "Albums", AllKeys(), columns, PartitionOptions{MaxPartitions: 1})
			if err != nil {
				t.Fatal(err)
			}
			for _, p := range []*Partition{queryPartitions[0], readPartitions[0]} {
				if err := server.TestSpanner.PutPartitionResult(p.pt, server.CreateSingleRowSingersResult(0)); err != nil {
					t.Fatal(err)
				}
				if err := txn.Execute(ctx, p).Do(func(*Row) error { return nil }); err != nil {
					t.Fatalf("Execute failed: %v", err)
				}
			}

			for _, method := range []string{executeStreamingSQL, streamingRead} {
				if got := counter.get(method); got != tc.wantHeaderCallsPerMethod {
					t.Errorf("%s Header() calls = %d, want %d", method, got, tc.wantHeaderCallsPerMethod)
				}
			}
			// Partitioned queries and reads both record under "Execute".
			sinkLatency := map[string]int64{"query": 456, "ReadWithOptions": 123, "Execute": 456 + 123}
			sinkMethods := slices.Collect(maps.Keys(sinkLatency))
			wantOT := map[string]int64{}
			if tc.openTelemetry {
				wantOT = sinkLatency
			}
			if got := latencySumsByAttr(t, collectTestMetrics(t, otReader), metricsPrefix+"gfe_latency", string(attributeKeyMethod), sinkMethods...); !reflect.DeepEqual(got, wantOT) {
				t.Errorf("OpenTelemetry GFE latency sums = %v, want %v", got, wantOT)
			}
			wantOC := map[string]int64{}
			if tc.openCensus {
				wantOC = sinkLatency
			}
			if got := openCensusLatencySums(t, sinkMethods); !reflect.DeepEqual(got, wantOC) {
				t.Errorf("OpenCensus GFE latency sums = %v, want %v", got, wantOC)
			}
			wantBuiltIn := map[string]int64{}
			if tc.builtIn {
				wantBuiltIn = map[string]int64{"Spanner.ExecuteStreamingSql": 2 * 456, "Spanner.StreamingRead": 2 * 123}
			}
			gotBuiltIn := latencySumsByAttr(t, collectTestMetrics(t, builtInReader), clientMetricsPrefix+metricNameGFELatencies, metricLabelKeyMethod, "Spanner.ExecuteStreamingSql", "Spanner.StreamingRead")
			if !reflect.DeepEqual(gotBuiltIn, wantBuiltIn) {
				t.Errorf("built-in GFE latency sums = %v, want %v", gotBuiltIn, wantBuiltIn)
			}
			var gotSpanLatencies []float64
			for _, span := range spans.GetSpans() {
				if span.Name != "cloud.google.com/go/spanner.RowIterator" {
					continue
				}
				for _, attr := range span.Attributes {
					if attr.Key == "gfe.latency_ms" {
						gotSpanLatencies = append(gotSpanLatencies, attr.Value.AsFloat64())
					}
				}
			}
			var wantSpanLatencies []float64
			if tc.tracing {
				wantSpanLatencies = []float64{123, 123, 456, 456}
			}
			sort.Float64s(gotSpanLatencies)
			if !reflect.DeepEqual(gotSpanLatencies, wantSpanLatencies) {
				t.Errorf("RowIterator span gfe.latency_ms = %v, want %v", gotSpanLatencies, wantSpanLatencies)
			}
		})
	}
}

// latencySumsByAttr returns the sum of the histogram named name for each of the
// given values of the attribute key.
func latencySumsByAttr(t *testing.T, rm metricdata.ResourceMetrics, name, key string, values ...string) map[string]int64 {
	t.Helper()
	sums := map[string]int64{}
	m, ok := findTestMetric(rm, name)
	if !ok {
		return sums
	}
	add := func(attrs attribute.Set, sum int64) {
		v, _ := attrs.Value(attribute.Key(key))
		if slices.Contains(values, v.AsString()) {
			sums[v.AsString()] += sum
		}
	}
	switch data := m.Data.(type) {
	case metricdata.Histogram[int64]:
		for _, dp := range data.DataPoints {
			add(dp.Attributes, dp.Sum)
		}
	case metricdata.Histogram[float64]:
		for _, dp := range data.DataPoints {
			add(dp.Attributes, int64(dp.Sum))
		}
	default:
		t.Fatalf("metric %q data type = %T, want a histogram", name, m.Data)
	}
	return sums
}

func TestGFELatencyHeaderOptions(t *testing.T) {
	origFlag := getGFELatencyMetricsFlag()
	t.Cleanup(func() { setGFELatencyMetricsFlag(origFlag) })

	for _, tc := range []struct {
		name                 string
		ocFlag               bool
		hasOpenCensusContext bool
		otConfig             *openTelemetryConfig
		wantHeader           bool
	}{
		{name: "no sinks"},
		{name: "OpenCensus flag without context", ocFlag: true},
		{name: "OpenCensus flag and context", ocFlag: true, hasOpenCensusContext: true, wantHeader: true},
		{name: "OpenTelemetry disabled", otConfig: &openTelemetryConfig{}},
		{name: "OpenTelemetry enabled", otConfig: &openTelemetryConfig{enabled: true}, wantHeader: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			setGFELatencyMetricsFlag(tc.ocFlag)
			var md metadata.MD
			var settings gax.CallSettings
			for _, opt := range gfeLatencyHeaderOptions(&md, tc.hasOpenCensusContext, tc.otConfig) {
				opt.Resolve(&settings)
			}
			var gotHeader bool
			for _, opt := range settings.GRPC {
				if h, ok := opt.(grpc.HeaderCallOption); ok && h.HeaderAddr == &md {
					gotHeader = true
				}
			}
			if gotHeader != tc.wantHeader || len(settings.GRPC) > 1 {
				t.Fatalf("gfeLatencyHeaderOptions() gRPC options = %v, want header capture %v", settings.GRPC, tc.wantHeader)
			}
		})
	}
}

// headerRequestCounter counts, per method, the calls that request the response
// headers with grpc.Header. Installed with grpc.WithUnaryInterceptor and
// grpc.WithStreamInterceptor, it runs before every chained interceptor, so it
// only sees the requests made by the Spanner client call sites.
type headerRequestCounter struct {
	mu    sync.Mutex
	calls map[string]int
}

func (c *headerRequestCounter) count(method string, opts []grpc.CallOption) {
	c.mu.Lock()
	defer c.mu.Unlock()
	for _, opt := range opts {
		// Request ID options use a grpc.Header option that already holds the
		// outgoing request ID; they do not capture response headers for a sink.
		if h, ok := opt.(grpc.HeaderCallOption); ok && len(*h.HeaderAddr) == 0 {
			c.calls[method]++
		}
	}
}

func (c *headerRequestCounter) unary(ctx context.Context, method string, req, reply any, cc *grpc.ClientConn, invoker grpc.UnaryInvoker, opts ...grpc.CallOption) error {
	c.count(method, opts)
	return invoker(ctx, method, req, reply, cc, opts...)
}

func (c *headerRequestCounter) stream(ctx context.Context, desc *grpc.StreamDesc, cc *grpc.ClientConn, method string, streamer grpc.Streamer, opts ...grpc.CallOption) (grpc.ClientStream, error) {
	c.count(method, opts)
	return streamer(ctx, desc, cc, method, opts...)
}

func (c *headerRequestCounter) get() map[string]int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return maps.Clone(c.calls)
}

// TestResponseHeadersOnlyRequestedWhenUsed runs the unary calls that feed the
// GFE latency sinks, and a BatchWrite, through the public client. It verifies
// that the response headers are only requested when a sink uses them, that
// every enabled sink still records the latency of each call, and that server
// timing span attributes are unchanged.
func TestResponseHeadersOnlyRequestedWhenUsed(t *testing.T) {
	const (
		beginTransaction = "/google.spanner.v1.Spanner/BeginTransaction"
		executeSQL       = "/google.spanner.v1.Spanner/ExecuteSql"
		executeBatchDml  = "/google.spanner.v1.Spanner/ExecuteBatchDml"
		commit           = "/google.spanner.v1.Spanner/Commit"
		partitionQuery   = "/google.spanner.v1.Spanner/PartitionQuery"
		partitionRead    = "/google.spanner.v1.Spanner/PartitionRead"
		createSession    = "/google.spanner.v1.Spanner/CreateSession"
		batchWrite       = "/google.spanner.v1.Spanner/BatchWrite"
	)
	// The mock server sends "gfet4t7; dur=123" for CreateSession. Send a
	// different value for every other method to tell them apart.
	gfeLatency := map[string]int64{
		beginTransaction: 11,
		executeSQL:       22,
		executeBatchDml:  33,
		commit:           44,
		partitionQuery:   55,
		partitionRead:    66,
		batchWrite:       77,
	}
	serverTiming := func(method string) metadata.MD {
		return metadata.Pairs("server-timing", fmt.Sprintf("gfet4t7; dur=%d", gfeLatency[method]))
	}
	unaryTiming := grpc.UnaryInterceptor(func(ctx context.Context, req any, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (any, error) {
		if _, ok := gfeLatency[info.FullMethod]; ok {
			if err := grpc.SetHeader(ctx, serverTiming(info.FullMethod)); err != nil {
				return nil, err
			}
		}
		return handler(ctx, req)
	})
	batchWriteTiming := grpc.StreamInterceptor(func(srv any, stream grpc.ServerStream, info *grpc.StreamServerInfo, handler grpc.StreamHandler) error {
		if info.FullMethod == batchWrite {
			if err := stream.SetHeader(serverTiming(batchWrite)); err != nil {
				return err
			}
		}
		return handler(srv, stream)
	})
	// Other tests in this package enable OpenCensus GFE latency metrics and
	// install a recording tracer provider, so set every sink explicitly.
	origOTFlag := IsOpenTelemetryMetricsEnabled()
	origOCFlag := getGFELatencyMetricsFlag()
	origTracerProvider := otel.GetTracerProvider()
	t.Cleanup(func() {
		setOpenTelemetryMetricsFlag(origOTFlag)
		setGFELatencyMetricsFlag(origOCFlag)
		otel.SetTracerProvider(origTracerProvider)
	})

	// The sink method names of the calls below, and the GFE latency each
	// records once.
	sinkLatency := map[string]int64{
		"begin_BeginTransaction":             11,
		"update":                             22,
		"batchUpdateWithOptions":             33,
		"commit":                             44,
		"partitionQuery":                     55,
		"PartitionReadUsingIndexWithOptions": 66,
		"executePdml_ExecuteSql":             22,
		"executeCreateSession":               123,
		"createSession":                      123,
	}
	sinkMethods := append(slices.Collect(maps.Keys(sinkLatency)), "BatchWrite")
	// The calls below request the headers of these methods once each, except
	// ExecuteSql, which runs for both an update and a partitioned update, and
	// CreateSession, which runs for the multiplexed session and createSession.
	sinkHeaderRequests := map[string]int{
		createSession:    2,
		beginTransaction: 1,
		executeSQL:       2,
		executeBatchDml:  1,
		commit:           1,
		partitionQuery:   1,
		partitionRead:    1,
	}

	for _, tc := range []struct {
		name                               string
		openCensus, openTelemetry, builtIn bool
		tracing                            bool
		wantHeaderRequests                 map[string]int
	}{
		{name: "no sinks", wantHeaderRequests: map[string]int{}},
		{name: "tracing", tracing: true, wantHeaderRequests: map[string]int{}},
		{name: "built-in metrics", builtIn: true, wantHeaderRequests: map[string]int{}},
		{name: "OpenCensus", openCensus: true, wantHeaderRequests: sinkHeaderRequests},
		{name: "OpenTelemetry", openTelemetry: true, wantHeaderRequests: sinkHeaderRequests},
		{name: "all sinks", openCensus: true, openTelemetry: true, builtIn: true, tracing: true, wantHeaderRequests: sinkHeaderRequests},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ctx := context.Background()
			setGFELatencyMetricsFlag(tc.openCensus)
			setOpenTelemetryMetricsFlag(tc.openTelemetry)
			if err := view.Register(GFELatencyView); err != nil {
				t.Fatal(err)
			}
			defer view.Unregister(GFELatencyView)
			spans := tracetest.NewInMemoryExporter()
			if tc.tracing {
				tp := sdktrace.NewTracerProvider(sdktrace.WithSyncer(spans))
				defer tp.Shutdown(ctx)
				otel.SetTracerProvider(tp)
			} else {
				otel.SetTracerProvider(noop.NewTracerProvider())
			}
			otReader, otProvider := newTestMeterProvider()
			config := ClientConfig{DisableNativeMetrics: true, OpenTelemetryMeterProvider: otProvider}
			builtInReader, builtInProvider := newTestMeterProvider()
			if tc.builtIn {
				config.ClientMetricsProvider = builtInProvider
			}
			headerRequests := &headerRequestCounter{calls: map[string]int{}}
			streamHeaders := &streamHeaderCounter{calls: map[string]int{}}
			_, opts, serverTeardown := NewMockedSpannerInMemTestServer(t, unaryTiming, batchWriteTiming)
			defer serverTeardown()
			opts = append(opts,
				option.WithGRPCDialOption(grpc.WithUnaryInterceptor(headerRequests.unary)),
				option.WithGRPCDialOption(grpc.WithStreamInterceptor(headerRequests.stream)),
				option.WithGRPCDialOption(grpc.WithChainStreamInterceptor(streamHeaders.interceptor)),
			)
			client, err := NewClientWithConfig(ctx, "projects/p/instances/i/databases/d", config, opts...)
			if err != nil {
				t.Fatal(err)
			}
			defer client.Close()

			// BeginTransaction.
			ro := client.ReadOnlyTransaction()
			if err := ro.Query(ctx, NewStatement(SelectSingerIDAlbumIDAlbumTitleFromAlbums)).Do(func(*Row) error { return nil }); err != nil {
				t.Fatalf("Query failed: %v", err)
			}
			ro.Close()
			// ExecuteSql, ExecuteBatchDml and Commit.
			if _, err := client.ReadWriteTransaction(ctx, func(ctx context.Context, tx *ReadWriteTransaction) error {
				if _, err := tx.Update(ctx, NewStatement(UpdateBarSetFoo)); err != nil {
					return err
				}
				_, err := tx.BatchUpdate(ctx, []Statement{NewStatement(UpdateBarSetFoo)})
				return err
			}); err != nil {
				t.Fatalf("ReadWriteTransaction failed: %v", err)
			}
			// PartitionQuery and PartitionRead.
			txn, err := client.BatchReadOnlyTransaction(ctx, StrongRead())
			if err != nil {
				t.Fatal(err)
			}
			if _, err := txn.PartitionQuery(ctx, NewStatement(SelectSingerIDAlbumIDAlbumTitleFromAlbums), PartitionOptions{MaxPartitions: 1}); err != nil {
				t.Fatal(err)
			}
			if _, err := txn.PartitionRead(ctx, "Albums", AllKeys(), []string{"SingerId"}, PartitionOptions{MaxPartitions: 1}); err != nil {
				t.Fatal(err)
			}
			txn.Cleanup(ctx)
			// ExecuteSql of a partitioned update.
			if _, err := client.PartitionedUpdate(ctx, NewStatement(UpdateBarSetFoo)); err != nil {
				t.Fatalf("PartitionedUpdate failed: %v", err)
			}
			// CreateSession of a regular session.
			if _, err := client.sc.createSession(ctx); err != nil {
				t.Fatalf("createSession failed: %v", err)
			}
			iter := client.BatchWrite(ctx, []*MutationGroup{{Mutations: []*Mutation{Insert("Albums", []string{"SingerId"}, []any{1})}}})
			if err := iter.Do(func(*spannerpb.BatchWriteResponse) error { return nil }); err != nil {
				t.Fatalf("BatchWrite failed: %v", err)
			}

			if got := headerRequests.get(); !reflect.DeepEqual(got, tc.wantHeaderRequests) {
				t.Errorf("header requests = %v, want %v", got, tc.wantHeaderRequests)
			}
			// BatchWrite does not record GFE latency, so it never reads headers.
			if got := streamHeaders.get(batchWrite); got != 0 {
				t.Errorf("BatchWrite Header() calls = %d, want 0", got)
			}

			wantOT := map[string]int64{}
			if tc.openTelemetry {
				wantOT = sinkLatency
			}
			if got := latencySumsByAttr(t, collectTestMetrics(t, otReader), metricsPrefix+"gfe_latency", string(attributeKeyMethod), sinkMethods...); !reflect.DeepEqual(got, wantOT) {
				t.Errorf("OpenTelemetry GFE latency sums = %v, want %v", got, wantOT)
			}
			wantOC := map[string]int64{}
			if tc.openCensus {
				wantOC = sinkLatency
			}
			if got := openCensusLatencySums(t, sinkMethods); !reflect.DeepEqual(got, wantOC) {
				t.Errorf("OpenCensus GFE latency sums = %v, want %v", got, wantOC)
			}
			builtInMetrics := collectTestMetrics(t, builtInReader)
			// The built-in metrics interceptor records every unary call,
			// including the BeginTransaction of the batch transaction and of
			// the partitioned update.
			builtInLatency := map[string]int64{
				"Spanner.CreateSession":    2 * 123,
				"Spanner.BeginTransaction": 3 * 11,
				"Spanner.ExecuteSql":       2 * 22,
				"Spanner.ExecuteBatchDml":  33,
				"Spanner.Commit":           44,
				"Spanner.PartitionQuery":   55,
				"Spanner.PartitionRead":    66,
			}
			wantBuiltIn := map[string]int64{}
			if tc.builtIn {
				wantBuiltIn = builtInLatency
			}
			if got := latencySumsByAttr(t, builtInMetrics, clientMetricsPrefix+metricNameGFELatencies, metricLabelKeyMethod, slices.Collect(maps.Keys(builtInLatency))...); !reflect.DeepEqual(got, wantBuiltIn) {
				t.Errorf("built-in GFE latency sums = %v, want %v", got, wantBuiltIn)
			}
			// The built-in metrics tracer is not in the BatchWrite context.
			if got := builtInMetricsOfMethod(builtInMetrics, "Spanner.BatchWrite"); len(got) != 0 {
				t.Errorf("built-in metrics of Spanner.BatchWrite = %v, want none", got)
			}

			// The built-in metrics interceptor sets the server timing span
			// attributes of unary calls independently of the sinks. The
			// partition calls do not start a span, and BatchWrite sets none.
			gotSpanLatencies := map[float64]bool{}
			for _, span := range spans.GetSpans() {
				for _, attr := range span.Attributes {
					if attr.Key == "gfe.latency_ms" {
						gotSpanLatencies[attr.Value.AsFloat64()] = true
					}
				}
			}
			wantSpanLatencies := map[float64]bool{}
			if tc.tracing {
				wantSpanLatencies = map[float64]bool{11: true, 22: true, 33: true, 44: true, 123: true}
			}
			if !reflect.DeepEqual(gotSpanLatencies, wantSpanLatencies) {
				t.Errorf("span gfe.latency_ms values = %v, want %v", gotSpanLatencies, wantSpanLatencies)
			}
		})
	}
}

// openCensusLatencySums returns the sum of the OpenCensus GFE latency view
// for each of the given methods.
func openCensusLatencySums(t *testing.T, methods []string) map[string]int64 {
	t.Helper()
	rows, err := view.RetrieveData(GFELatencyView.Name)
	if err != nil {
		t.Fatal(err)
	}
	sums := map[string]int64{}
	for _, row := range rows {
		for _, tag := range row.Tags {
			if tag.Key == tagKeyMethod && slices.Contains(methods, tag.Value) {
				sums[tag.Value] += int64(row.Data.(*view.DistributionData).Sum())
			}
		}
	}
	return sums
}

// TestStreamCreationFailureRecordsNoBuiltInMetrics verifies that a query or
// read whose stream is rejected before it reaches Spanner records no built-in
// metrics for the streaming method.
func TestStreamCreationFailureRecordsNoBuiltInMetrics(t *testing.T) {
	reject := grpc.WithChainStreamInterceptor(func(ctx context.Context, desc *grpc.StreamDesc, cc *grpc.ClientConn, method string, streamer grpc.Streamer, opts ...grpc.CallOption) (grpc.ClientStream, error) {
		return nil, status.Error(codes.InvalidArgument, "rejected before sending")
	})
	for _, tc := range []struct {
		name   string
		method string
		run    func(context.Context, *Client) error
	}{
		{
			name:   "query",
			method: "Spanner.ExecuteStreamingSql",
			run: func(ctx context.Context, client *Client) error {
				return client.Single().Query(ctx, NewStatement(SelectSingerIDAlbumIDAlbumTitleFromAlbums)).Do(func(*Row) error { return nil })
			},
		},
		{
			name:   "read",
			method: "Spanner.StreamingRead",
			run: func(ctx context.Context, client *Client) error {
				return client.Single().Read(ctx, "Albums", AllKeys(), []string{"SingerId"}).Do(func(*Row) error { return nil })
			},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ctx := context.Background()
			reader, provider := newTestMeterProvider()
			_, opts, serverTeardown := NewMockedSpannerInMemTestServer(t)
			defer serverTeardown()
			opts = append(opts, option.WithGRPCDialOption(reject))
			client, err := NewClientWithConfig(ctx, "projects/p/instances/i/databases/d", ClientConfig{DisableNativeMetrics: true, ClientMetricsProvider: provider}, opts...)
			if err != nil {
				t.Fatal(err)
			}
			defer client.Close()

			if err := tc.run(ctx, client); ErrCode(err) != codes.InvalidArgument {
				t.Fatalf("error = %v, want code %v", err, codes.InvalidArgument)
			}
			if got := builtInMetricsOfMethod(collectTestMetrics(t, reader), tc.method); len(got) != 0 {
				t.Errorf("built-in metrics of %s = %v, want none", tc.method, got)
			}
		})
	}
}

// builtInMetricsOfMethod returns the names of the metrics that have a data
// point for the given built-in metrics method label.
func builtInMetricsOfMethod(rm metricdata.ResourceMetrics, method string) []string {
	var names []string
	for _, sm := range rm.ScopeMetrics {
		for _, m := range sm.Metrics {
			var attrs []attribute.Set
			switch data := m.Data.(type) {
			case metricdata.Sum[int64]:
				for _, dp := range data.DataPoints {
					attrs = append(attrs, dp.Attributes)
				}
			case metricdata.Histogram[float64]:
				for _, dp := range data.DataPoints {
					attrs = append(attrs, dp.Attributes)
				}
			case metricdata.Histogram[int64]:
				for _, dp := range data.DataPoints {
					attrs = append(attrs, dp.Attributes)
				}
			}
			for _, a := range attrs {
				if v, ok := a.Value(attribute.Key(metricLabelKeyMethod)); ok && v.AsString() == method {
					names = append(names, m.Name)
					break
				}
			}
		}
	}
	return names
}

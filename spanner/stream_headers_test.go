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
	"reflect"
	"slices"
	"sort"
	"sync"
	"testing"
	"time"

	"cloud.google.com/go/spanner/apiv1/spannerpb"
	. "cloud.google.com/go/spanner/internal/testutil"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/sdk/metric/metricdata"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
	oteltrace "go.opentelemetry.io/otel/trace"
	"go.opentelemetry.io/otel/trace/noop"
	"google.golang.org/api/option"
	"google.golang.org/grpc"
	"google.golang.org/grpc/metadata"
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
		{name: "nil tracer", noTracer: true, recording: true},
		{name: "no current attempt", builtInEnabled: true, recording: true, noAttempt: true},
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
			if tc.wantHeaderCalls > 0 {
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
			wantOT := map[string]int64{}
			if tc.openTelemetry {
				wantOT = map[string]int64{"query": 456, "ReadWithOptions": 123, "Execute": 456 + 123}
			}
			if got := latencySumsByAttr(t, collectTestMetrics(t, otReader), metricsPrefix+"gfe_latency", string(attributeKeyMethod), "query", "ReadWithOptions", "Execute"); !reflect.DeepEqual(got, wantOT) {
				t.Errorf("OpenTelemetry GFE latency sums = %v, want %v", got, wantOT)
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

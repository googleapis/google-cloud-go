// Copyright 2025 Google LLC
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

package spanner

import (
	"context"
	"sort"
	"testing"
	"time"

	"cloud.google.com/go/internal/testutil"
	"cloud.google.com/go/spanner/apiv1/spannerpb"
	"cloud.google.com/go/spanner/internal"
	"github.com/google/go-cmp/cmp"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/sdk/instrumentation"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
	"go.opentelemetry.io/otel/trace"
)

func TestTraceSpannerTraceStartEndSpan(t *testing.T) {
	ctx := context.Background()
	e := tracetest.NewInMemoryExporter()
	tp := sdktrace.NewTracerProvider(sdktrace.WithSyncer(e))
	defer tp.Shutdown(ctx)
	otel.SetTracerProvider(tp)

	spanName := "spanner.TestTrace.TestSpannerTraceStartEndSpan"
	addAttrs := attribute.String("foo", "bar")
	spanStartOpts := []trace.SpanStartOption{
		trace.WithAttributes(
			attribute.String("gcp.client.version", internal.Version),
			attribute.String("gcp.client.repo", gcpClientRepo),
			attribute.String("gcp.client.artifact", gcpClientArtifact),
		),
		trace.WithAttributes(addAttrs),
	}
	newAttrs := attribute.Int("fakeKey", 800)

	ctx, span := startSpan(ctx, spanName, spanStartOpts...)
	span.SetAttributes(newAttrs)
	endSpan(ctx, nil)

	spans := e.GetSpans()
	if len(spans) != 1 {
		t.Errorf("expected one span, got %d", len(spans))
	}

	// Test StartSpanOption and Cloud Trace adoption common attributes are appended.
	// Test startSpan returns the span and additional attributes can be set.
	wantSpan := createWantSpanStub(spanName)
	wantSpan.Attributes = append(wantSpan.Attributes, addAttrs)
	wantSpan.Attributes = append(wantSpan.Attributes, newAttrs)
	opts := []cmp.Option{
		cmp.Comparer(spanAttributesComparer),
	}
	for _, span := range spans {
		if diff := testutil.Diff(span, wantSpan, opts...); diff != "" {
			t.Errorf("diff: -got, +want:\n%s\n", diff)
		}
	}
	e.Reset()
}

func TestLocationAwareRouteSelectionTrace(t *testing.T) {
	ctx := context.Background()
	exporter := tracetest.NewInMemoryExporter()
	tp := sdktrace.NewTracerProvider(sdktrace.WithSyncer(exporter))
	defer tp.Shutdown(ctx)
	otel.SetTracerProvider(tp)

	ctx, _ = startSpan(ctx, "RouteSelectionTrace")
	lac := &locationAwareSpannerClient{
		defaultEndpointAddress: "default:443",
	}
	lac.recordRouteSelectionTrace(
		ctx,
		"google.spanner.v1.Spanner/ExecuteSql",
		&passthroughChannelEndpoint{address: "replica:443"},
		false,
		routeSelectionDetails{
			defaultReasonCode: routeReasonRangeCacheMiss,
			selectedEndpoint:  "replica:443",
		},
	)
	endSpan(ctx, nil)

	spans := exporter.GetSpans()
	if len(spans) != 1 {
		t.Fatalf("expected one span, got %d", len(spans))
	}
	gotSpan := spans[0]
	if !hasKeyValueAttribute(gotSpan.Attributes, "spanner.target", "replica:443") {
		t.Fatal("expected spanner.target attribute")
	}
	if !hasKeyValueAttribute(gotSpan.Attributes, "spanner.route.used_default_endpoint", false) {
		t.Fatal("expected spanner.route.used_default_endpoint attribute")
	}
	if !hasKeyValueAttribute(gotSpan.Attributes, "spanner.route.has_channel_finder", true) {
		t.Fatal("expected spanner.route.has_channel_finder attribute")
	}
	if !hasKeyValueAttribute(gotSpan.Attributes, "spanner.route.method", "google.spanner.v1.Spanner/ExecuteSql") {
		t.Fatal("expected spanner.route.method attribute")
	}
	if !hasKeyValueAttribute(gotSpan.Attributes, "spanner.route.default_reason_code", routeReasonRangeCacheMiss) {
		t.Fatal("expected spanner.route.default_reason_code attribute")
	}
	if len(gotSpan.Events) != 1 {
		t.Fatalf("expected one route-selection event, got %d", len(gotSpan.Events))
	}
	if gotSpan.Events[0].Name != "spanner.route.selected" {
		t.Fatalf("event name = %q, want %q", gotSpan.Events[0].Name, "spanner.route.selected")
	}
	if !hasKeyValueAttribute(gotSpan.Events[0].Attributes, "spanner.target", "replica:443") {
		t.Fatal("expected spanner.target event attribute")
	}
	if !hasKeyValueAttribute(gotSpan.Events[0].Attributes, "spanner.route.default_reason_code", routeReasonRangeCacheMiss) {
		t.Fatal("expected spanner.route.default_reason_code event attribute")
	}
}

func hasKeyValueAttribute(attrs []attribute.KeyValue, key string, want any) bool {
	for _, attr := range attrs {
		if string(attr.Key) != key {
			continue
		}
		switch typedWant := want.(type) {
		case string:
			return attr.Value.AsString() == typedWant
		case bool:
			return attr.Value.AsBool() == typedWant
		}
	}
	return false
}

func createWantSpanStub(spanName string) tracetest.SpanStub {
	return tracetest.SpanStub{
		Name: prependPackageName(spanName),
		Attributes: []attribute.KeyValue{
			attribute.String("gcp.client.version", internal.Version),
			attribute.String("gcp.client.repo", gcpClientRepo),
			attribute.String("gcp.client.artifact", gcpClientArtifact),
		},
		InstrumentationLibrary: instrumentation.Scope{
			Name:    "cloud.google.com/go/spanner",
			Version: internal.Version,
		},
	}
}

func spanAttributesComparer(a, b tracetest.SpanStub) bool {
	if a.Name != b.Name {
		return false
	}
	if len(a.Attributes) != len(b.Attributes) {
		return false
	}
	if a.InstrumentationLibrary != b.InstrumentationLibrary {
		return false
	}
	return true
}

func BenchmarkSetSpanAttributes(b *testing.B) {
	testCases := []struct {
		name string
		req  any
	}{
		{
			name: "ExecuteSqlRequest",
			req: &spannerpb.ExecuteSqlRequest{
				RequestOptions: &spannerpb.RequestOptions{
					TransactionTag: "tx-tag-1",
					RequestTag:     "req-tag-1",
				},
				Sql: "SELECT 1",
			},
		},
		{
			name: "ExecuteBatchDmlRequest",
			req: &spannerpb.ExecuteBatchDmlRequest{
				RequestOptions: &spannerpb.RequestOptions{
					TransactionTag: "tx-tag-2",
					RequestTag:     "req-tag-2",
				},
				Statements: []*spannerpb.ExecuteBatchDmlRequest_Statement{
					{Sql: "UPDATE t SET c = 1"},
					{Sql: "UPDATE t2 SET c = 2"},
				},
			},
		},
		{
			name: "ReadRequest",
			req: &spannerpb.ReadRequest{
				RequestOptions: &spannerpb.RequestOptions{
					TransactionTag: "tx-tag-3",
					RequestTag:     "req-tag-3",
				},
				Table: "MyTable",
			},
		},
		{
			name: "CommitRequest",
			req: &spannerpb.CommitRequest{
				RequestOptions: &spannerpb.RequestOptions{
					TransactionTag: "tx-tag-4",
					RequestTag:     "req-tag-4",
				},
			},
		},
		{
			name: "PartitionQueryRequest",
			req: &spannerpb.PartitionQueryRequest{
				Sql: "SELECT * FROM Table",
			},
		},
		{
			name: "PartitionReadRequest",
			req: &spannerpb.PartitionReadRequest{
				Table: "AnotherTable",
			},
		},
		{
			name: "BatchWriteRequest - no attributes",
			req:  &spannerpb.BatchWriteRequest{},
		},
		{
			name: "Empty struct - no attributes",
			req:  struct{}{},
		},
	}

	for _, tc := range testCases {
		b.Run(tc.name, func(b *testing.B) {
			// Benchmark with a recording span.
			b.Run("recording", func(b *testing.B) {
				recorder := tracetest.NewSpanRecorder()
				provider := sdktrace.NewTracerProvider(sdktrace.WithSpanProcessor(recorder))
				tracer := provider.Tracer("test-tracer")
				_, span := tracer.Start(context.Background(), "test-span")

				b.ReportAllocs()
				b.ResetTimer()

				timings := make([]time.Duration, b.N)
				for i := 0; i < b.N; i++ {
					start := time.Now()
					setSpanAttributes(span, tc.req)
					timings[i] = time.Since(start)
				}

				b.StopTimer()

				if len(timings) > 0 {
					sort.Slice(timings, func(i, j int) bool { return timings[i] < timings[j] })
					p50 := timings[int(float64(len(timings)-1)*0.50)]
					p90 := timings[int(float64(len(timings)-1)*0.90)]
					p99 := timings[int(float64(len(timings)-1)*0.99)]
					b.ReportMetric(float64(p50.Nanoseconds()), "p50-ns/op")
					b.ReportMetric(float64(p90.Nanoseconds()), "p90-ns/op")
					b.ReportMetric(float64(p99.Nanoseconds()), "p99-ns/op")
				}
			})

			// Benchmark with a non-recording span.
			b.Run("not-recording", func(b *testing.B) {
				noopTracerProvider := trace.NewNoopTracerProvider()
				tracer := noopTracerProvider.Tracer("test-tracer")
				_, span := tracer.Start(context.Background(), "test-span")

				b.ReportAllocs()
				b.ResetTimer()
				timings := make([]time.Duration, b.N)
				for i := 0; i < b.N; i++ {
					start := time.Now()
					setSpanAttributes(span, tc.req)
					timings[i] = time.Since(start)
				}
				b.StopTimer()

				if len(timings) > 0 {
					sort.Slice(timings, func(i, j int) bool { return timings[i] < timings[j] })
					p50 := timings[int(float64(len(timings)-1)*0.50)]
					p90 := timings[int(float64(len(timings)-1)*0.90)]
					p99 := timings[int(float64(len(timings)-1)*0.99)]
					b.ReportMetric(float64(p50.Nanoseconds()), "p50-ns/op")
					b.ReportMetric(float64(p90.Nanoseconds()), "p90-ns/op")
					b.ReportMetric(float64(p99.Nanoseconds()), "p99-ns/op")
				}
			})
		})
	}
}

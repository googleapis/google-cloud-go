// Copyright 2024 Google LLC
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
	"fmt"
	"io"
	"io/ioutil"
	"net/http"
	"os"
	"testing"

	"cloud.google.com/go/internal/testutil"
	"cloud.google.com/go/storage/internal"
	"github.com/google/go-cmp/cmp"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
	"go.opentelemetry.io/otel/trace"
	"google.golang.org/api/option"
)

func TestTraceStorageTraceStartEndSpan(t *testing.T) {
	originalOtelTracingBool := os.Getenv("GO_STORAGE_DEV_OTEL_TRACING")
	defer os.Setenv("GO_STORAGE_DEV_OTEL_TRACING", originalOtelTracingBool)

	os.Setenv("GO_STORAGE_DEV_OTEL_TRACING", "true")
	ctx := context.Background()
	e := tracetest.NewInMemoryExporter()
	tp := sdktrace.NewTracerProvider(sdktrace.WithSyncer(e))
	defer tp.Shutdown(ctx)
	otel.SetTracerProvider(tp)

	spanName := "storage.TestTrace.TestStorageTraceStartEndSpan"
	spanStartopts := []trace.SpanStartOption{
		trace.WithAttributes(
			attribute.String("foo", "bar"),
		),
	}
	addAttrs := attribute.String("fakeKey", "fakeVal")

	ctx, span := startSpan(ctx, spanName, spanStartopts...)
	span.SetAttributes(addAttrs)
	endSpan(ctx, nil)

	spans := e.GetSpans()
	if len(spans) != 1 {
		t.Errorf("expected one span, got %d", len(spans))
	}

	// Test StartSpanOption and Cloud Trace Adoption common attributes are appended.
	wantAttributes := tracetest.SpanStub{
		Name: spanName,
		Attributes: []attribute.KeyValue{
			attribute.String("foo", "bar"),
			attribute.String("gcp.client.version", internal.Version),
			attribute.String("gcp.client.repo", gcpClientRepo),
			attribute.String("gcp.client.artifact", gcpClientArtifact),
		},
	}
	// Test startSpan returns the span and additional attributes can be set.
	wantAttributes.Attributes = append(wantAttributes.Attributes, addAttrs)
	opts := []cmp.Option{
		cmp.Comparer(spanAttributesComparer),
	}
	for _, span := range spans {
		if diff := testutil.Diff(span, wantAttributes, opts...); diff != "" {
			t.Errorf("diff: -got, +want:\n%s\n", diff)
		}
	}
}

func TestTraceOtelTraceDevFlagEnabled(t *testing.T) {
	originalOtelTracingBool := os.Getenv("GO_STORAGE_DEV_OTEL_TRACING")
	defer os.Setenv("GO_STORAGE_DEV_OTEL_TRACING", originalOtelTracingBool)

	os.Setenv("GO_STORAGE_DEV_OTEL_TRACING", "true")
	ctx := context.Background()
	e := tracetest.NewInMemoryExporter()
	tp := sdktrace.NewTracerProvider(sdktrace.WithSyncer(e))
	defer tp.Shutdown(ctx)
	otel.SetTracerProvider(tp)

	// This utilizes newTestServer to make RPC calls and export traces to
	// the tracetest.InMemoryExporter.
	// TBD: What RPC calls do we want to test?
	hClient, close := newTestServer(func(w http.ResponseWriter, r *http.Request) {
		io.Copy(ioutil.Discard, r.Body)
		fmt.Fprintf(w, "{}")
	})
	defer close()

	client, err := NewClient(ctx, option.WithHTTPClient(hClient))
	if err != nil {
		t.Fatal(err)
	}
	_, err = client.Bucket("b").Attrs(ctx)
	if err != nil {
		t.Errorf("got %v", err)
	}

	// Test Cloud Trace Adoption common attributes are appended.
	wantAttributes := tracetest.SpanStub{
		Name: "storage.Bucket.Attrs",
		Attributes: []attribute.KeyValue{
			attribute.String("gcp.client.version", internal.Version),
			attribute.String("gcp.client.repo", gcpClientRepo),
			attribute.String("gcp.client.artifact", gcpClientArtifact),
		},
	}

	spans := e.GetSpans()
	opts := []cmp.Option{
		cmp.Comparer(spanAttributesComparer),
	}
	for _, span := range spans {
		if diff := testutil.Diff(span, wantAttributes, opts...); diff != "" {
			t.Errorf("diff: -got, +want:\n%s\n", diff)
		}
	}
}

func spanAttributesComparer(a, b tracetest.SpanStub) bool {
	if a.Name != b.Name {
		fmt.Printf("name mismatch: a.Name: %v, b.Name: %v\n", a.Name, b.Name)
		return false
	}
	if len(a.Attributes) != len(b.Attributes) {
		fmt.Printf("len mismatch: a.Attributes: %d, b.Attributes: %d\n", len(a.Attributes), len(b.Attributes))
		return false
	}
	return true
}

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

package storage

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"testing"
	"time"

	"cloud.google.com/go/internal/testutil"
	"cloud.google.com/go/storage/internal"
	"github.com/google/go-cmp/cmp"
	"go.opentelemetry.io/otel/attribute"
	otcodes "go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/sdk/instrumentation"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
	"go.opentelemetry.io/otel/trace"
	"google.golang.org/api/googleapi"
	"google.golang.org/genproto/googleapis/rpc/errdetails"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func TestStorageTraceStartEndSpan(t *testing.T) {
	ctx := context.Background()
	te := testutil.NewOpenTelemetryTestExporter()
	t.Cleanup(func() {
		te.Unregister(ctx)
	})

	// TODO: Remove setting development env var upon launch.
	t.Setenv("GO_STORAGE_DEV_OTEL_TRACING", "true")

	spanName := "storage.TestTrace.TestStartEndSpan"
	ctx, span := startSpan(ctx, spanName)
	newAttrs := attribute.Int("fakeKey", 800)
	span.SetAttributes(newAttrs)
	endSpan(ctx, nil)

	spans := te.Spans()
	gotSpan := spans[0]
	if len(spans) != 1 {
		t.Errorf("expected one span, got %d", len(spans))
	}
	if got, want := gotSpan.Name, appendPackageName(spanName); got != want {
		t.Fatalf("got %s, want %s", got, want)
	}

	wantSpan := createWantSpanStub(spanName, getCommonAttributes())
	wantSpan.Attributes = append(wantSpan.Attributes, newAttrs)
	opts := []cmp.Option{
		cmp.Comparer(spanAttributesComparer),
	}
	if diff := testutil.Diff(gotSpan, wantSpan, opts...); diff != "" {
		t.Errorf("diff: -got, +want:\n%s\n", diff)
	}
}
func TestStorageTraceStartSpanOption(t *testing.T) {
	ctx := context.Background()
	te := testutil.NewOpenTelemetryTestExporter()
	t.Cleanup(func() {
		te.Unregister(ctx)
	})

	// TODO: Remove setting development env var upon launch.
	t.Setenv("GO_STORAGE_DEV_OTEL_TRACING", "true")

	spanName := "storage.TestTrace.TestStartSpanOption"
	attrMap := make(map[string]interface{})
	attrMap["my_string"] = "my string"
	attrMap["my_bool"] = true
	attrMap["my_int"] = 123
	attrMap["my_int64"] = int64(456)
	attrMap["my_float"] = 0.9
	spanStartOpts := makeSpanStartOptAttrs(attrMap)

	ctx, _ = startSpan(ctx, spanName, spanStartOpts...)
	endSpan(ctx, nil)

	spans := te.Spans()
	gotSpan := spans[0]
	if len(spans) != 1 {
		t.Errorf("expected one span, got %d", len(spans))
	}
	if got, want := gotSpan.Name, appendPackageName(spanName); got != want {
		t.Fatalf("got %s, want %s", got, want)
	}

	wantSpan := createWantSpanStub(spanName, getCommonAttributes())
	wantSpan.Attributes = append(wantSpan.Attributes, otAttrs(attrMap)...)
	opts := []cmp.Option{
		cmp.Comparer(spanAttributesComparer),
	}
	if diff := testutil.Diff(gotSpan, wantSpan, opts...); diff != "" {
		t.Errorf("diff: -got, +want:\n%s\n", diff)
	}
}

func TestStorageTraceEndSpanRecordError(t *testing.T) {
	ctx := context.Background()
	te := testutil.NewOpenTelemetryTestExporter()
	t.Cleanup(func() {
		te.Unregister(ctx)
	})

	// TODO: Remove setting development env var upon launch.
	t.Setenv("GO_STORAGE_DEV_OTEL_TRACING", "true")

	spanName := "storage.TestTrace.TestRecordError"
	ctx, _ = startSpan(ctx, spanName)
	err := &googleapi.Error{Code: http.StatusBadRequest, Message: "INVALID ARGUMENT"}
	endSpan(ctx, err)

	spans := te.Spans()
	gotSpan := spans[0]
	if len(spans) != 1 {
		t.Errorf("expected one span, got %d", len(spans))
	}
	if got, want := gotSpan.Name, appendPackageName(spanName); got != want {
		t.Fatalf("got %s, want %s", got, want)
	}
	if want := otcodes.Error; gotSpan.Status.Code != want {
		t.Errorf("got %v, want %v", gotSpan.Status.Code, want)
	}
}

func createWantSpanStub(spanName string, attrs []attribute.KeyValue) tracetest.SpanStub {
	return tracetest.SpanStub{
		Name:       appendPackageName(spanName),
		Attributes: attrs,
		InstrumentationScope: instrumentation.Scope{
			Name:    "cloud.google.com/go/storage",
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
	if a.InstrumentationScope != b.InstrumentationScope {
		return false
	}
	return true
}

// makeSpanStartOptAttrs makes a SpanStartOption and converts a generic map to OpenTelemetry attributes.
func makeSpanStartOptAttrs(attrMap map[string]interface{}) []trace.SpanStartOption {
	attrs := otAttrs(attrMap)
	return []trace.SpanStartOption{
		trace.WithAttributes(attrs...),
	}
}

// otAttrs converts a generic map to OpenTelemetry attributes.
func otAttrs(attrMap map[string]interface{}) []attribute.KeyValue {
	var attrs []attribute.KeyValue
	for k, v := range attrMap {
		var a attribute.KeyValue
		switch v := v.(type) {
		case string:
			a = attribute.Key(k).String(v)
		case bool:
			a = attribute.Key(k).Bool(v)
		case int:
			a = attribute.Key(k).Int(v)
		case int64:
			a = attribute.Key(k).Int64(v)
		default:
			a = attribute.Key(k).String(fmt.Sprintf("%#v", v))
		}
		attrs = append(attrs, a)
	}
	return attrs
}

func TestStartSpanWithBucket(t *testing.T) {
	ctx := context.Background()
	te := testutil.NewOpenTelemetryTestExporter()
	t.Cleanup(func() {
		te.Unregister(ctx)
	})

	t.Setenv("GO_STORAGE_DEV_OTEL_TRACING", "true")

	fetcher := &mockMetadataFetcher{
		fetchFunc: func(ctx context.Context, bucket string) (resource string, location string, err error) {
			return "projects/p1/buckets/" + bucket, "us-west1", nil
		},
	}

	tests := []struct {
		name         string
		bucket       string
		setupCache   func(*bucketMetadataCache)
		wantResource string
		wantLocation string
		verifyCache  bool
	}{
		{
			name:   "Cache Miss (Placeholder)",
			bucket: "bucket-miss",
			setupCache: func(c *bucketMetadataCache) {
				// empty cache
			},
			wantResource: storageResourceNamePrefix + "projects/_/buckets/bucket-miss",
			wantLocation: "global",
			verifyCache:  true,
		},
		{
			name:   "Cache Hit (Resolved)",
			bucket: "bucket-hit",
			setupCache: func(c *bucketMetadataCache) {
				c.put("bucket-hit", bucketMetadata{resource: "projects/p1/buckets/bucket-hit", location: "us-west1"})
			},
			wantResource: storageResourceNamePrefix + "projects/p1/buckets/bucket-hit",
			wantLocation: "us-west1",
			verifyCache:  false,
		},
	}

	for i, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			cache := newBucketMetadataCache(10, fetcher)
			tc.setupCache(cache)
			doneChan := make(chan struct{}, 1)
			if tc.verifyCache {
				cache.fetchDone = doneChan
			}
			client := &Client{bucketMetadataCache: cache}

			ctx1, _ := startSpanWithBucket(ctx, client, tc.bucket, "TestSpan")
			endSpan(ctx1, nil)

			spans := te.Spans()
			if len(spans) != i+1 {
				t.Fatalf("expected %d spans, got %d", i+1, len(spans))
			}
			gotSpan := spans[i]

			verifySpanAttributes(t, gotSpan, tc.wantResource, tc.wantLocation)

			if tc.verifyCache {
				// Wait for background fetch to complete and populate cache.
				select {
				case <-doneChan:
				case <-time.After(fetchBackgroundTimeout):
					t.Fatalf("timeout waiting for fetchBackground completion")
				}
				_, found := cache.get(tc.bucket)
				if !found {
					t.Fatalf("expected entry to be populated in cache")
				}
			}
		})
	}
}

func verifySpanAttributes(t *testing.T, span tracetest.SpanStub, wantResource, wantLocation string) {
	t.Helper()
	var gotResource, gotLocation string
	for _, attr := range span.Attributes {
		if attr.Key == "gcp.resource.destination.id" {
			gotResource = attr.Value.AsString()
		}
		if attr.Key == "gcp.resource.destination.location" {
			gotLocation = attr.Value.AsString()
		}
	}

	if gotResource != wantResource {
		t.Errorf("got resource %q, want %q", gotResource, wantResource)
	}

	if gotLocation != wantLocation {
		t.Errorf("got location %q, want %q", gotLocation, wantLocation)
	}
}

func TestEndSpanEviction(t *testing.T) {
	t.Setenv("GO_STORAGE_DEV_OTEL_TRACING", "true")

	bucketName := "evict-bucket"
	tests := []struct {
		name      string
		spanName  string
		err       error
		wantEvict bool
	}{
		{
			name:      "Evict on ErrBucketNotExist",
			spanName:  "Bucket.Attrs",
			err:       ErrBucketNotExist,
			wantEvict: true,
		},
		{
			name:      "Evict on googleapi.Error 404",
			spanName:  "Bucket.Attrs",
			err:       &googleapi.Error{Code: http.StatusNotFound},
			wantEvict: true,
		},
		{
			name:      "No Evict on 500",
			spanName:  "Bucket.Attrs",
			err:       &googleapi.Error{Code: http.StatusInternalServerError},
			wantEvict: false,
		},
		{
			name:      "No Evict on Object 404",
			spanName:  "Object.Attrs",
			err:       ErrObjectNotExist,
			wantEvict: false,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			fetcher := &mockMetadataFetcher{}
			cache := newBucketMetadataCache(10, fetcher)
			client := &Client{bucketMetadataCache: cache}

			// Populate cache.
			cache.put(bucketName, bucketMetadata{resource: "res", location: "loc"})

			ctx, _ := startSpanWithBucket(context.Background(), client, bucketName, tc.spanName)
			endSpan(ctx, tc.err)

			_, found := cache.get(bucketName)
			if tc.wantEvict && found {
				t.Errorf("expected bucket to be evicted")
			}
			if !tc.wantEvict && !found {
				t.Errorf("expected bucket to remain in cache")
			}
		})
	}
}

func TestErrorAttributes(t *testing.T) {
	ctx := context.Background()

	grpcWithErrorInfo, err := status.New(codes.Unimplemented, "Appendable objects are not supported for this request.").WithDetails(&errdetails.ErrorInfo{
		Reason: "GRPC_APPENDABLE_OBJECT_NOT_SUPPORTED",
		Domain: "storage.googleapis.com",
	})
	if err != nil {
		t.Fatalf("WithDetails: %v", err)
	}
	httpWithErrorInfo := &googleapi.Error{
		Code:    http.StatusForbidden,
		Message: "Permission denied.",
		Body:    `{"error":{"code":403,"message":"Permission denied.","status":"PERMISSION_DENIED","details":[{"@type":"type.googleapis.com/google.rpc.ErrorInfo","reason":"SERVICE_DISABLED","domain":"googleapis.com"}]}}`,
	}

	tests := []struct {
		name       string
		err        error
		wantType   string // "" means the attribute must not be set.
		wantDomain string // "" means the attribute must not be set.
	}{
		{
			name:     "HTTP not found",
			err:      fmt.Errorf("%w: %w", ErrBucketNotExist, &googleapi.Error{Code: http.StatusNotFound, Message: "The specified bucket does not exist."}),
			wantType: "NOT_FOUND",
		},
		{
			name:     "HTTP precondition failed",
			err:      &googleapi.Error{Code: http.StatusPreconditionFailed, Message: "At least one of the pre-conditions you specified did not hold."},
			wantType: "412",
		},
		{
			name: "HTTP XML error",
			err: &googleapi.Error{
				Code: http.StatusRequestedRangeNotSatisfiable,
				Body: "<?xml version='1.0' encoding='UTF-8'?><Error><Code>InvalidRange</Code><Message>The requested range cannot be satisfied.</Message></Error>",
			},
			wantType: "OUT_OF_RANGE",
		},
		{
			name:       "HTTP error with ErrorInfo",
			err:        httpWithErrorInfo,
			wantType:   "PERMISSION_DENIED",
			wantDomain: "googleapis.com",
		},
		{
			name:     "gRPC not found",
			err:      fmt.Errorf("%w: %w", ErrObjectNotExist, status.Error(codes.NotFound, "No such object: bucket/obj")),
			wantType: "NOT_FOUND",
		},
		{
			name:       "gRPC error with ErrorInfo",
			err:        grpcWithErrorInfo.Err(),
			wantType:   "UNIMPLEMENTED",
			wantDomain: "storage.googleapis.com",
		},
		{
			name:       "wrapped gRPC error with ErrorInfo",
			err:        fmt.Errorf("storage: write failed: %w", grpcWithErrorInfo.Err()),
			wantType:   "UNIMPLEMENTED",
			wantDomain: "storage.googleapis.com",
		},
		{
			name:     "context canceled",
			err:      context.Canceled,
			wantType: "CANCELLED",
		},
		{
			name:     "deadline exceeded",
			err:      context.DeadlineExceeded,
			wantType: "TIMEOUT",
		},
		{
			name:     "client-side error",
			err:      errors.New("storage: bucket name is empty"),
			wantType: "UNKNOWN",
		},
		{
			name:     "nil error sets no attributes",
			err:      nil,
			wantType: "",
		},
		{
			name:     "io.EOF does not set error.type",
			err:      io.EOF,
			wantType: "",
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := map[attribute.Key]string{}
			for _, kv := range errorAttributes(ctx, tc.err) {
				got[kv.Key] = kv.Value.AsString()
			}
			want := map[attribute.Key]string{}
			if tc.wantType != "" {
				want["error.type"] = tc.wantType
			}
			if tc.wantDomain != "" {
				want["gcp.errors.domain"] = tc.wantDomain
			}
			if diff := cmp.Diff(want, got); diff != "" {
				t.Errorf("errorAttributes(%v) mismatch (-want +got):\n%s", tc.err, diff)
			}
		})
	}
}

func TestStorageTraceEndSpanErrorAttributes(t *testing.T) {
	ctx := context.Background()
	grpcWithErrorInfo, err := status.New(codes.PermissionDenied, "Permission denied.").WithDetails(&errdetails.ErrorInfo{
		Reason: "SERVICE_DISABLED",
		Domain: "storage.googleapis.com",
	})
	if err != nil {
		t.Fatalf("WithDetails: %v", err)
	}

	tests := []struct {
		name      string
		otelOn    bool
		err       error
		wantAttrs map[attribute.Key]string
	}{
		{
			name:      "error sets error.type and gcp.errors.domain",
			otelOn:    true,
			err:       grpcWithErrorInfo.Err(),
			wantAttrs: map[attribute.Key]string{"error.type": "PERMISSION_DENIED", "gcp.errors.domain": "storage.googleapis.com"},
		},
		{
			name:      "gRPC not found error sets error.type",
			otelOn:    true,
			err:       status.Error(codes.NotFound, "No such object: bucket/obj"),
			wantAttrs: map[attribute.Key]string{"error.type": "NOT_FOUND"},
		},
		{
			name:      "HTTP not found error sets error.type",
			otelOn:    true,
			err:       fmt.Errorf("%w: %w", ErrBucketNotExist, &googleapi.Error{Code: http.StatusNotFound, Message: "The specified bucket does not exist."}),
			wantAttrs: map[attribute.Key]string{"error.type": "NOT_FOUND"},
		},
		{
			name:      "nil error sets no error attributes",
			otelOn:    true,
			err:       nil,
			wantAttrs: map[attribute.Key]string{},
		},
		{
			// The legacy internalTrace path still ends a span through the
			// global provider; it must not get the new attributes.
			name:      "legacy internalTrace path unchanged",
			otelOn:    false,
			err:       grpcWithErrorInfo.Err(),
			wantAttrs: map[attribute.Key]string{},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if tc.otelOn {
				t.Setenv("GO_STORAGE_DEV_OTEL_TRACING", "true")
			} else {
				t.Setenv("GO_STORAGE_DEV_OTEL_TRACING", "false")
			}
			te := testutil.NewOpenTelemetryTestExporter()
			t.Cleanup(func() {
				te.Unregister(ctx)
			})

			spanCtx, _ := startSpan(ctx, "Object.Attrs")
			endSpan(spanCtx, tc.err)

			spans := te.Spans()
			if len(spans) != 1 {
				t.Fatalf("expected 1 span, got %d", len(spans))
			}
			got := map[attribute.Key]string{}
			for _, kv := range spans[0].Attributes {
				if kv.Key == "error.type" || kv.Key == "gcp.errors.domain" {
					got[kv.Key] = kv.Value.AsString()
				}
			}
			if diff := cmp.Diff(tc.wantAttrs, got); diff != "" {
				t.Errorf("error attributes mismatch (-want +got):\n%s", diff)
			}
		})
	}
}

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
	"net/http"
	"testing"
	"time"

	"cloud.google.com/go/internal/testutil"
	"cloud.google.com/go/storage/internal"
	"github.com/google/go-cmp/cmp"
	"github.com/googleapis/gax-go/v2/apierror"
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
	var gotErrMsg string
	for _, attr := range gotSpan.Attributes {
		if attr.Key == "error.message" {
			gotErrMsg = attr.Value.AsString()
		}
	}
	if want := "INVALID ARGUMENT"; gotErrMsg != want {
		t.Errorf("got error.message %q, want %q", gotErrMsg, want)
	}
}

func TestStorageTraceErrorMessage(t *testing.T) {
	ctx := context.Background()
	t.Setenv("GO_STORAGE_DEV_OTEL_TRACING", "true")

	apiErrGRPC, _ := apierror.FromError(status.Error(codes.FailedPrecondition, "This bucket requires appendable objects."))
	apiErrHTTP, _ := apierror.FromError(&googleapi.Error{Code: http.StatusPreconditionFailed, Message: "Precondition Failed"})

	tests := []struct {
		name           string
		err            error
		wantErrMsg     string
		wantHasAttr    bool
		wantStatusCode otcodes.Code
	}{
		{
			name:           "nil error does not set error.message",
			err:            nil,
			wantErrMsg:     "",
			wantHasAttr:    false,
			wantStatusCode: otcodes.Unset,
		},
		{
			name:           "googleapi.Error extracts Message",
			err:            &googleapi.Error{Code: http.StatusBadRequest, Message: "This bucket requires appendable objects."},
			wantErrMsg:     "This bucket requires appendable objects.",
			wantHasAttr:    true,
			wantStatusCode: otcodes.Error,
		},
		{
			name:           "gRPC status.Error extracts Message",
			err:            status.Error(codes.FailedPrecondition, "This bucket requires appendable objects."),
			wantErrMsg:     "This bucket requires appendable objects.",
			wantHasAttr:    true,
			wantStatusCode: otcodes.Error,
		},
		{
			name:           "apierror.APIError wrapping gRPC status",
			err:            apiErrGRPC,
			wantErrMsg:     "This bucket requires appendable objects.",
			wantHasAttr:    true,
			wantStatusCode: otcodes.Error,
		},
		{
			name:           "apierror.APIError wrapping googleapi.Error",
			err:            apiErrHTTP,
			wantErrMsg:     "Precondition Failed",
			wantHasAttr:    true,
			wantStatusCode: otcodes.Error,
		},
		{
			name:           "wrapped gRPC status.Error preserves inner message",
			err:            fmt.Errorf("%w: %w", ErrObjectNotExist, status.Error(codes.NotFound, "No such object: bucket/obj")),
			wantErrMsg:     "No such object: bucket/obj",
			wantHasAttr:    true,
			wantStatusCode: otcodes.Error,
		},
		{
			name:           "sentinel error falls back to Error()",
			err:            ErrBucketNotExist,
			wantErrMsg:     ErrBucketNotExist.Error(),
			wantHasAttr:    true,
			wantStatusCode: otcodes.Error,
		},
		{
			name:           "plain error falls back to Error()",
			err:            errors.New("storage: bucket name is empty"),
			wantErrMsg:     "storage: bucket name is empty",
			wantHasAttr:    true,
			wantStatusCode: otcodes.Error,
		},
		{
			name: "XML API error body extracts Message element",
			err: &googleapi.Error{
				Code: http.StatusRequestedRangeNotSatisfiable,
				Body: "<?xml version='1.0' encoding='UTF-8'?><Error><Code>InvalidRange</Code><Message>The requested range cannot be satisfied.</Message><Details>bytes=104857600-104857727</Details></Error>",
			},
			wantErrMsg:     "The requested range cannot be satisfied.",
			wantHasAttr:    true,
			wantStatusCode: otcodes.Error,
		},
		{
			name: "wrapped XML API error body extracts Message element",
			err: fmt.Errorf("storage: read failed: %w", &googleapi.Error{
				Code: http.StatusPreconditionFailed,
				Body: "<?xml version='1.0' encoding='UTF-8'?><Error><Code>ConditionNotMet</Code><Message>At least one of the pre-conditions you specified did not hold.</Message></Error>",
			}),
			wantErrMsg:     "At least one of the pre-conditions you specified did not hold.",
			wantHasAttr:    true,
			wantStatusCode: otcodes.Error,
		},
		{
			name:           "googleapi.Error with non-XML body falls back to status text",
			err:            &googleapi.Error{Code: http.StatusServiceUnavailable, Body: "upstream connect error"},
			wantErrMsg:     "Service Unavailable",
			wantHasAttr:    true,
			wantStatusCode: otcodes.Error,
		},
		{
			name:           "googleapi.Error with empty body falls back to status text",
			err:            &googleapi.Error{Code: http.StatusNotFound},
			wantErrMsg:     "Not Found",
			wantHasAttr:    true,
			wantStatusCode: otcodes.Error,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
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
			gotSpan := spans[0]
			if gotSpan.Status.Code != tc.wantStatusCode {
				t.Errorf("got status code %v, want %v", gotSpan.Status.Code, tc.wantStatusCode)
			}

			var gotErrMsg string
			var hasAttr bool
			for _, attr := range gotSpan.Attributes {
				if attr.Key == "error.message" {
					hasAttr = true
					gotErrMsg = attr.Value.AsString()
				}
			}
			if hasAttr != tc.wantHasAttr {
				t.Fatalf("got hasAttr=%v, want %v (attrs=%v)", hasAttr, tc.wantHasAttr, gotSpan.Attributes)
			}
			if tc.wantHasAttr && gotErrMsg != tc.wantErrMsg {
				t.Errorf("got error.message %q, want %q", gotErrMsg, tc.wantErrMsg)
			}
		})
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

func TestStorageTraceErrorDomain(t *testing.T) {
	ctx := context.Background()
	t.Setenv("GO_STORAGE_DEV_OTEL_TRACING", "true")

	grpcWithErrorInfo, err := status.New(codes.PermissionDenied, "Permission denied.").WithDetails(&errdetails.ErrorInfo{
		Reason: "SERVICE_DISABLED",
		Domain: "storage.googleapis.com",
	})
	if err != nil {
		t.Fatalf("WithDetails: %v", err)
	}
	apiErrGRPC, _ := apierror.FromError(grpcWithErrorInfo.Err())

	tests := []struct {
		name       string
		err        error
		wantDomain string // "" means the attribute must not be set.
	}{
		{
			name:       "nil error",
			err:        nil,
			wantDomain: "",
		},
		{
			name:       "gRPC status with ErrorInfo",
			err:        grpcWithErrorInfo.Err(),
			wantDomain: "storage.googleapis.com",
		},
		{
			name:       "wrapped gRPC status with ErrorInfo",
			err:        fmt.Errorf("storage: retry failed: %w", grpcWithErrorInfo.Err()),
			wantDomain: "storage.googleapis.com",
		},
		{
			name:       "apierror.APIError with ErrorInfo",
			err:        apiErrGRPC,
			wantDomain: "storage.googleapis.com",
		},
		{
			name: "HTTP JSON error with ErrorInfo details",
			err: &googleapi.Error{
				Code:    http.StatusForbidden,
				Message: "Permission denied.",
				Body:    `{"error":{"code":403,"message":"Permission denied.","status":"PERMISSION_DENIED","details":[{"@type":"type.googleapis.com/google.rpc.ErrorInfo","reason":"SERVICE_DISABLED","domain":"storage.googleapis.com"}]}}`,
			},
			wantDomain: "storage.googleapis.com",
		},
		{
			name: "HTTP JSON error with legacy errors[].domain is not used",
			err: &googleapi.Error{
				Code:    http.StatusNotFound,
				Message: "The specified bucket does not exist.",
				Body:    `{"error":{"code":404,"message":"The specified bucket does not exist.","errors":[{"message":"The specified bucket does not exist.","domain":"global","reason":"notFound"}]}}`,
			},
			wantDomain: "",
		},
		{
			name:       "gRPC status without ErrorInfo",
			err:        status.Error(codes.NotFound, "No such object: bucket/obj"),
			wantDomain: "",
		},
		{
			name: "HTTP XML error",
			err: &googleapi.Error{
				Code: http.StatusRequestedRangeNotSatisfiable,
				Body: "<?xml version='1.0' encoding='UTF-8'?><Error><Code>InvalidRange</Code><Message>The requested range cannot be satisfied.</Message></Error>",
			},
			wantDomain: "",
		},
		{
			name:       "client-side context error",
			err:        context.Canceled,
			wantDomain: "",
		},
		{
			name:       "sentinel error",
			err:        ErrObjectNotExist,
			wantDomain: "",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
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
			var gotDomain string
			var hasAttr bool
			for _, attr := range spans[0].Attributes {
				if attr.Key == "gcp.errors.domain" {
					hasAttr = true
					gotDomain = attr.Value.AsString()
				}
			}
			if wantHasAttr := tc.wantDomain != ""; hasAttr != wantHasAttr {
				t.Fatalf("got hasAttr=%v, want %v (attrs=%v)", hasAttr, wantHasAttr, spans[0].Attributes)
			}
			if gotDomain != tc.wantDomain {
				t.Errorf("got gcp.errors.domain %q, want %q", gotDomain, tc.wantDomain)
			}
		})
	}
}

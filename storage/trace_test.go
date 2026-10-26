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
	"net"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"cloud.google.com/go/iam"
	"cloud.google.com/go/iam/apiv1/iampb"
	"cloud.google.com/go/internal/testutil"
	"cloud.google.com/go/storage/internal"
	"cloud.google.com/go/storage/internal/apiv2/storagepb"
	"github.com/google/go-cmp/cmp"
	gax "github.com/googleapis/gax-go/v2"
	"github.com/googleapis/gax-go/v2/apierror"
	"go.opentelemetry.io/otel/attribute"
	otcodes "go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/sdk/instrumentation"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
	"go.opentelemetry.io/otel/trace"
	"google.golang.org/api/googleapi"
	"google.golang.org/api/option"
	"google.golang.org/genproto/googleapis/rpc/errdetails"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/stats"
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

	grpcWithoutErrorInfo, err := status.New(codes.ResourceExhausted, "Rate limit exceeded.").WithDetails(&errdetails.RetryInfo{})
	if err != nil {
		t.Fatalf("WithDetails: %v", err)
	}
	// The generated JSON client returns a googleapi.Error wrapping an APIError.
	generatedHTTPErr := &googleapi.Error{
		Code:    http.StatusForbidden,
		Message: "Permission denied.",
		Body:    `{"error":{"code":403,"message":"Permission denied.","status":"PERMISSION_DENIED","details":[{"@type":"type.googleapis.com/google.rpc.ErrorInfo","reason":"SERVICE_DISABLED","domain":"storage.googleapis.com"}]}}`,
	}
	apiErr, ok := apierror.ParseError(generatedHTTPErr, false)
	if !ok {
		t.Fatal("apierror.ParseError failed")
	}
	generatedHTTPErr.Wrap(apiErr)

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
			wantType: "FAILED_PRECONDITION",
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
			name:       "HTTP error with ErrorInfo from the generated client",
			err:        generatedHTTPErr,
			wantType:   "PERMISSION_DENIED",
			wantDomain: "storage.googleapis.com",
		},
		{
			name:       "wrapped HTTP error with ErrorInfo from the generated client",
			err:        fmt.Errorf("%w: %w", ErrBucketNotExist, generatedHTTPErr),
			wantType:   "NOT_FOUND",
			wantDomain: "storage.googleapis.com",
		},
		{
			name:     "bare ErrObjectNotExist",
			err:      ErrObjectNotExist,
			wantType: "NOT_FOUND",
		},
		{
			name:     "bare ErrBucketNotExist",
			err:      ErrBucketNotExist,
			wantType: "NOT_FOUND",
		},
		{
			name:     "gRPC not found with key in object name",
			err:      fmt.Errorf("%w: %w", ErrObjectNotExist, status.Error(codes.NotFound, "No such object: bucket/my-key")),
			wantType: "NOT_FOUND",
		},
		{
			name:     "gRPC not found with timeout in object name",
			err:      status.Error(codes.NotFound, "No such object: bucket/timeout.log"),
			wantType: "NOT_FOUND",
		},
		{
			name:     "HTTP not found with eof in object name",
			err:      &googleapi.Error{Code: http.StatusNotFound, Message: "No such object: bucket/geofence.json"},
			wantType: "NOT_FOUND",
		},
		{
			name:     "gRPC invalid page token",
			err:      status.Error(codes.InvalidArgument, "Invalid page token."),
			wantType: "INVALID_ARGUMENT",
		},
		{
			name:     "gRPC unavailable with network error message",
			err:      status.Error(codes.Unavailable, "connection error: dial tcp: connection refused"),
			wantType: "CONNECTION_ERROR",
		},
		{
			name:     "gRPC unavailable from the service",
			err:      status.Error(codes.Unavailable, "The service is currently unavailable."),
			wantType: "UNAVAILABLE",
		},
		{
			name:     "googleapi.Error without an HTTP code",
			err:      &googleapi.Error{Message: "something went wrong"},
			wantType: "UNKNOWN",
		},
		{
			name:     "wrapped deadline exceeded",
			err:      fmt.Errorf("storage: read failed: %w", context.DeadlineExceeded),
			wantType: "TIMEOUT",
		},
		{
			name:     "gRPC deadline exceeded from the service",
			err:      status.Error(codes.DeadlineExceeded, "Deadline exceeded."),
			wantType: "DEADLINE_EXCEEDED",
		},
		{
			name:     "gRPC client-side deadline",
			err:      status.Error(codes.DeadlineExceeded, "context deadline exceeded"),
			wantType: "TIMEOUT",
		},
		{
			name:     "gRPC error with details but no ErrorInfo",
			err:      grpcWithoutErrorInfo.Err(),
			wantType: "RESOURCE_EXHAUSTED",
		},
		{
			name:     "gRPC crc32c mismatch on upload",
			err:      status.Error(codes.InvalidArgument, "Object crc32c mismatch: server computed 1234, client provided 5678."),
			wantType: "CHECKSUM_MISMATCH",
		},
		{
			name:     "HTTP crc32c mismatch on upload",
			err:      &googleapi.Error{Code: http.StatusBadRequest, Message: `Provided CRC32C "AAAAAA==" doesn't match calculated CRC32C "BBBBBB==".`},
			wantType: "CHECKSUM_MISMATCH",
		},
		{
			name:     "CRC mismatch on read",
			err:      fmt.Errorf("%w: bad CRC on read: got %d, want %d", errChecksumMismatch, 1, 2),
			wantType: "CHECKSUM_MISMATCH",
		},
		{
			name:     "wrapped CRC mismatch on read",
			err:      fmt.Errorf("storage: read failed: %w", fmt.Errorf("%w: bad CRC on chunk read: got %d, want %d", errChecksumMismatch, 1, 2)),
			wantType: "CHECKSUM_MISMATCH",
		},
		{
			name:     "invalid argument mentioning crc32c is not a checksum mismatch",
			err:      status.Error(codes.InvalidArgument, "crc32c must be set when finishing a write."),
			wantType: "INVALID_ARGUMENT",
		},
		{
			name:     "wrapped io.EOF sets no attributes",
			err:      fmt.Errorf("storage: read: %w", io.EOF),
			wantType: "",
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
			for _, kv := range errorAttributes(tc.err) {
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

// storageURIAttr returns the gcp.storage.uri value on the span named spanName,
// and whether it was set.
func storageURIAttr(t *testing.T, spans tracetest.SpanStubs, spanName string) (string, bool) {
	t.Helper()
	for _, s := range spans {
		if s.Name != appendPackageName(spanName) {
			continue
		}
		for _, a := range s.Attributes {
			if a.Key == storageURIAttrKey {
				return a.Value.AsString(), true
			}
		}
		return "", false
	}
	t.Fatalf("span %q not found", spanName)
	return "", false
}

func TestStorageURIAttribute(t *testing.T) {
	const (
		bucket    = "my-bucket"
		object    = "dir/my object.txt"
		dstBucket = "dst-bucket"
		dstObject = "dst/object.txt"
	)
	wantObjURI := "gs://" + bucket + "/" + object
	wantDstURI := "gs://" + dstBucket + "/" + dstObject
	wantBucketURI := "gs://" + bucket + "/"

	tests := []struct {
		name     string // Test name; defaults to spanName.
		spanName string
		op       func(ctx context.Context, c *Client)
		wantURI  string
	}{
		// Object-scoped operations.
		{
			spanName: "Object.Attrs",
			op:       func(ctx context.Context, c *Client) { c.Bucket(bucket).Object(object).Attrs(ctx) },
			wantURI:  wantObjURI,
		},
		{
			spanName: "Object.Update",
			op: func(ctx context.Context, c *Client) {
				c.Bucket(bucket).Object(object).Update(ctx, ObjectAttrsToUpdate{ContentType: "text/plain"})
			},
			wantURI: wantObjURI,
		},
		{
			spanName: "Object.Delete",
			op:       func(ctx context.Context, c *Client) { c.Bucket(bucket).Object(object).Delete(ctx) },
			wantURI:  wantObjURI,
		},
		{
			spanName: "Object.Reader",
			op:       func(ctx context.Context, c *Client) { c.Bucket(bucket).Object(object).NewReader(ctx) },
			wantURI:  wantObjURI,
		},
		{
			spanName: "Object.Writer",
			op: func(ctx context.Context, c *Client) {
				w := c.Bucket(bucket).Object(object).NewWriter(ctx)
				w.Write([]byte("data"))
				w.Close()
			},
			wantURI: wantObjURI,
		},
		{
			spanName: "Object.MultiRangeDownloader",
			op:       func(ctx context.Context, c *Client) { c.Bucket(bucket).Object(object).NewMultiRangeDownloader(ctx) },
			wantURI:  wantObjURI,
		},
		{
			spanName: "Object.WriterFromAppendableObject",
			op: func(ctx context.Context, c *Client) {
				c.Bucket(bucket).Object(object).Generation(1).NewWriterFromAppendableObject(ctx, nil)
			},
			wantURI: wantObjURI,
		},
		{
			name:     "Copier.Run uses destination",
			spanName: "Copier.Run",
			op: func(ctx context.Context, c *Client) {
				c.Bucket(dstBucket).Object(dstObject).CopierFrom(c.Bucket(bucket).Object(object)).Run(ctx)
			},
			wantURI: wantDstURI,
		},
		{
			name:     "Composer.Run uses destination",
			spanName: "Composer.Run",
			op: func(ctx context.Context, c *Client) {
				c.Bucket(dstBucket).Object(dstObject).ComposerFrom(c.Bucket(dstBucket).Object("src1")).Run(ctx)
			},
			wantURI: wantDstURI,
		},
		{
			name:     "object ACL.List",
			spanName: "ACL.List",
			op:       func(ctx context.Context, c *Client) { c.Bucket(bucket).Object(object).ACL().List(ctx) },
			wantURI:  wantObjURI,
		},
		{
			name:     "object ACL.Set",
			spanName: "ACL.Set",
			op: func(ctx context.Context, c *Client) {
				c.Bucket(bucket).Object(object).ACL().Set(ctx, AllUsers, RoleReader)
			},
			wantURI: wantObjURI,
		},
		{
			name:     "object ACL.Delete",
			spanName: "ACL.Delete",
			op:       func(ctx context.Context, c *Client) { c.Bucket(bucket).Object(object).ACL().Delete(ctx, AllUsers) },
			wantURI:  wantObjURI,
		},
		// Bucket-scoped operations.
		{
			name:     "bucket ACL.List",
			spanName: "ACL.List",
			op:       func(ctx context.Context, c *Client) { c.Bucket(bucket).ACL().List(ctx) },
			wantURI:  wantBucketURI,
		},
		{
			spanName: "Bucket.Create",
			op:       func(ctx context.Context, c *Client) { c.Bucket(bucket).Create(ctx, "my-project", nil) },
			wantURI:  wantBucketURI,
		},
		{
			spanName: "Bucket.Delete",
			op:       func(ctx context.Context, c *Client) { c.Bucket(bucket).Delete(ctx) },
			wantURI:  wantBucketURI,
		},
		{
			spanName: "Bucket.Attrs",
			op:       func(ctx context.Context, c *Client) { c.Bucket(bucket).Attrs(ctx) },
			wantURI:  wantBucketURI,
		},
		{
			spanName: "Bucket.Update",
			op: func(ctx context.Context, c *Client) {
				c.Bucket(bucket).Update(ctx, BucketAttrsToUpdate{StorageClass: "STANDARD"})
			},
			wantURI: wantBucketURI,
		},
		{
			spanName: "Bucket.AddNotification",
			op: func(ctx context.Context, c *Client) {
				c.Bucket(bucket).AddNotification(ctx, &Notification{TopicProjectID: "p", TopicID: "t"})
			},
			wantURI: wantBucketURI,
		},
		{
			spanName: "Bucket.Notifications",
			op:       func(ctx context.Context, c *Client) { c.Bucket(bucket).Notifications(ctx) },
			wantURI:  wantBucketURI,
		},
		{
			spanName: "Bucket.DeleteNotification",
			op:       func(ctx context.Context, c *Client) { c.Bucket(bucket).DeleteNotification(ctx, "n") },
			wantURI:  wantBucketURI,
		},
		{
			spanName: "storage.IAM.Get",
			op:       func(ctx context.Context, c *Client) { c.Bucket(bucket).IAM().Policy(ctx) },
			wantURI:  wantBucketURI,
		},
		{
			spanName: "storage.IAM.Set",
			op: func(ctx context.Context, c *Client) {
				c.Bucket(bucket).IAM().SetPolicy(ctx, &iam.Policy{InternalProto: &iampb.Policy{}})
			},
			wantURI: wantBucketURI,
		},
		{
			spanName: "storage.IAM.Test",
			op: func(ctx context.Context, c *Client) {
				c.Bucket(bucket).IAM().TestPermissions(ctx, []string{"storage.buckets.get"})
			},
			wantURI: wantBucketURI,
		},
		{
			spanName: "httpStorageClient.ObjectsListCall",
			op:       func(ctx context.Context, c *Client) { c.Bucket(bucket).Objects(ctx, nil).Next() },
			wantURI:  wantBucketURI,
		},
		{
			spanName: "grpcStorageClient.ObjectsListCall",
			op: func(ctx context.Context, _ *Client) {
				gc, err := NewGRPCClient(ctx,
					option.WithEndpoint("localhost:1"),
					option.WithoutAuthentication(),
					option.WithGRPCDialOption(grpc.WithTransportCredentials(insecure.NewCredentials())),
				)
				if err != nil {
					t.Fatalf("NewGRPCClient: %v", err)
				}
				defer gc.Close()
				gc.Bucket(bucket).Retryer(WithPolicy(RetryNever)).Objects(ctx, nil).Next()
			},
			wantURI: wantBucketURI,
		},
		{
			name:     "empty bucket omits attribute",
			spanName: "Bucket.Attrs",
			op:       func(ctx context.Context, c *Client) { c.Bucket("").Attrs(ctx) },
			wantURI:  "",
		},
	}

	for _, tc := range tests {
		name := tc.name
		if name == "" {
			name = tc.spanName
		}
		t.Run(name, func(t *testing.T) {
			t.Setenv(storageOtelTracingDevVar, "true")
			// Disable the bucket metadata cache so no background fetch runs.
			t.Setenv(storageBucketMetadataDisabledVar, "true")
			ctx := context.Background()
			te := testutil.NewOpenTelemetryTestExporter()
			t.Cleanup(func() { te.Unregister(ctx) })

			tc.op(ctx, mockClient(t, &mockTransport{}))

			got, ok := storageURIAttr(t, te.Spans(), tc.spanName)
			if tc.wantURI == "" {
				if ok {
					t.Errorf("%s = %q, want it absent when bucket is empty", storageURIAttrKey, got)
				}
				return
			}
			if !ok {
				t.Fatalf("%s not found on span %q", storageURIAttrKey, tc.spanName)
			}
			if got != tc.wantURI {
				t.Errorf("%s = %q, want %q", storageURIAttrKey, got, tc.wantURI)
			}
		})
	}
}

// retryAttemptNumber returns the value of gcp.client.retry.attempt_number in
// attrs and whether it is present.
func retryAttemptNumber(attrs []attribute.KeyValue) (int64, bool) {
	for _, a := range attrs {
		if a.Key == "gcp.client.retry.attempt_number" {
			return a.Value.AsInt64(), true
		}
	}
	return 0, false
}

func TestRetryAttemptStatsHandlerTagRPC(t *testing.T) {
	t.Setenv("GO_STORAGE_DEV_OTEL_TRACING", "true")
	for _, tc := range []struct {
		name string
		// start returns the ctx handed to TagRPC, mimicking the ctx the
		// transport's stats handler produces, and ends any spans it started.
		start func(ctx context.Context) (tagCtx context.Context, end func())
		// wantAttempts maps span names to the attempt number they must carry.
		// Spans not listed must not carry one.
		wantAttempts map[string]int64
	}{
		{
			name: "attempt span under operation span",
			start: func(ctx context.Context) (context.Context, func()) {
				opCtx, opSpan := tracer().Start(ctx, "op")
				attemptCtx, attemptSpan := tracer().Start(withRetryAttempt(opCtx, 2), "attempt")
				return attemptCtx, func() { attemptSpan.End(); opSpan.End() }
			},
			wantAttempts: map[string]int64{"attempt": 2},
		},
		{
			name: "attempt span without operation span",
			start: func(ctx context.Context) (context.Context, func()) {
				attemptCtx, attemptSpan := tracer().Start(withRetryAttempt(ctx, 1), "attempt")
				return attemptCtx, func() { attemptSpan.End() }
			},
			wantAttempts: map[string]int64{"attempt": 1},
		},
		{
			name: "no attempt in context",
			start: func(ctx context.Context) (context.Context, func()) {
				opCtx, opSpan := tracer().Start(ctx, "op")
				attemptCtx, attemptSpan := tracer().Start(opCtx, "attempt")
				return attemptCtx, func() { attemptSpan.End(); opSpan.End() }
			},
		},
		{
			// With no transport span the span in ctx is the operation span,
			// which must be left alone.
			name: "no attempt span",
			start: func(ctx context.Context) (context.Context, func()) {
				opCtx, opSpan := tracer().Start(ctx, "op")
				return withRetryAttempt(opCtx, 2), func() { opSpan.End() }
			},
		},
		{
			name: "no span at all",
			start: func(ctx context.Context) (context.Context, func()) {
				return withRetryAttempt(ctx, 2), func() {}
			},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ctx := context.Background()
			te := testutil.NewOpenTelemetryTestExporter()
			t.Cleanup(func() {
				te.Unregister(ctx)
			})

			tagCtx, end := tc.start(ctx)
			retryAttemptStatsHandler{}.TagRPC(tagCtx, &stats.RPCTagInfo{})
			end()

			// Left nil when no span carries the attribute so that it compares
			// equal to an unset wantAttempts.
			var gotAttempts map[string]int64
			for _, s := range te.Spans() {
				if n, ok := retryAttemptNumber(s.Attributes); ok {
					if gotAttempts == nil {
						gotAttempts = map[string]int64{}
					}
					gotAttempts[s.Name] = n
				}
			}
			if diff := cmp.Diff(tc.wantAttempts, gotAttempts); diff != "" {
				t.Errorf("attempt numbers mismatch (-want +got):\n%s", diff)
			}
		})
	}
}

func TestStorageURIAttributeDevTracingDisabled(t *testing.T) {
	t.Setenv(storageOtelTracingDevVar, "false")
	t.Setenv(storageBucketMetadataDisabledVar, "true")
	ctx := context.Background()
	te := testutil.NewOpenTelemetryTestExporter()
	t.Cleanup(func() { te.Unregister(ctx) })

	mockClient(t, &mockTransport{}).Bucket("b").Object("o").Attrs(ctx)

	if got, ok := storageURIAttr(t, te.Spans(), "Object.Attrs"); ok {
		t.Errorf("%s = %q, want it absent when dev tracing is disabled", storageURIAttrKey, got)
	}
}

// startFakeStorageServer starts an in-process gRPC server for srv, points
// STORAGE_EMULATOR_HOST_GRPC at it for the duration of t, and returns the
// listener address.
func startFakeStorageServer(t *testing.T, srv storagepb.StorageServer) string {
	t.Helper()
	lis, err := net.Listen("tcp", "localhost:0")
	if err != nil {
		t.Fatalf("net.Listen: %v", err)
	}
	grpcSrv := grpc.NewServer()
	storagepb.RegisterStorageServer(grpcSrv, srv)
	go grpcSrv.Serve(lis)
	t.Cleanup(grpcSrv.Stop)
	addr := lis.Addr().String()
	t.Setenv("STORAGE_EMULATOR_HOST_GRPC", addr)
	return addr
}

// retryAttemptTestServer is a fake storage server whose GetBucket and
// BidiReadObject fail with UNAVAILABLE for the first failures calls and
// succeed afterwards.
type retryAttemptTestServer struct {
	storagepb.UnimplementedStorageServer
	mu       sync.Mutex
	failures int
	calls    int
}

func (s *retryAttemptTestServer) nextShouldFail() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.calls++
	return s.calls <= s.failures
}

func (s *retryAttemptTestServer) GetBucket(_ context.Context, req *storagepb.GetBucketRequest) (*storagepb.Bucket, error) {
	if s.nextShouldFail() {
		return nil, status.Error(codes.Unavailable, "retry me")
	}
	return &storagepb.Bucket{Name: req.GetName()}, nil
}

func (s *retryAttemptTestServer) BidiReadObject(stream storagepb.Storage_BidiReadObjectServer) error {
	if _, err := stream.Recv(); err != nil {
		return err
	}
	if s.nextShouldFail() {
		return status.Error(codes.Unavailable, "retry me")
	}
	if err := stream.Send(&storagepb.BidiReadObjectResponse{
		Metadata: &storagepb.Object{Size: 1},
	}); err != nil {
		return err
	}
	_, err := stream.Recv()
	if errors.Is(err, io.EOF) {
		return nil
	}
	return err
}

func TestGRPCRetryAttemptNumberTracing(t *testing.T) {
	const (
		getBucketSpan      = "google.storage.v2.Storage/GetBucket"
		bidiReadObjectSpan = "google.storage.v2.Storage/BidiReadObject"
	)

	getBucketCall := func(t *testing.T, ctx context.Context, client *Client) {
		t.Helper()
		if _, err := client.Bucket("bucket").Attrs(ctx); err != nil {
			t.Fatalf("Bucket.Attrs: %v", err)
		}
	}
	mrdCall := func(t *testing.T, ctx context.Context, client *Client) {
		t.Helper()
		mrd, err := client.Bucket("bucket").Object("object").NewMultiRangeDownloader(ctx)
		if err != nil {
			t.Fatalf("NewMultiRangeDownloader: %v", err)
		}
		if err := mrd.Close(); err != nil {
			t.Fatalf("mrd.Close: %v", err)
		}
	}

	for _, tc := range []struct {
		name        string
		devTracing  bool
		failures    int
		opSpan      string
		attemptSpan string
		call        func(t *testing.T, ctx context.Context, client *Client)
		// wantAttempts are the attempt numbers expected on the attempt spans,
		// in order. nil means the spans carry none.
		wantAttempts []int64
	}{
		{
			name:         "single attempt",
			devTracing:   true,
			opSpan:       "Bucket.Attrs",
			attemptSpan:  getBucketSpan,
			call:         getBucketCall,
			wantAttempts: []int64{1},
		},
		{
			name:         "retried",
			devTracing:   true,
			failures:     2,
			opSpan:       "Bucket.Attrs",
			attemptSpan:  getBucketSpan,
			call:         getBucketCall,
			wantAttempts: []int64{1, 2, 3},
		},
		{
			name:         "retried MRD BidiReadObject",
			devTracing:   true,
			failures:     2,
			opSpan:       "Object.MultiRangeDownloader",
			attemptSpan:  bidiReadObjectSpan,
			call:         mrdCall,
			wantAttempts: []int64{1, 2, 3},
		},
		{
			name:        "dev tracing disabled",
			failures:    1,
			opSpan:      "Bucket.Attrs",
			attemptSpan: getBucketSpan,
			call:        getBucketCall,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ctx := context.Background()
			te := testutil.NewOpenTelemetryTestExporter()
			t.Cleanup(func() {
				te.Unregister(ctx)
			})
			t.Setenv("GO_STORAGE_DEV_OTEL_TRACING", strconv.FormatBool(tc.devTracing))
			// The background bucket metadata fetch would issue an extra,
			// untraced GetBucket that also consumes the server's failures.
			t.Setenv("GO_OTEL_BUCKETMETADATA_DISABLED", "true")

			startFakeStorageServer(t, &retryAttemptTestServer{failures: tc.failures})

			client, err := NewGRPCClient(ctx, WithGRPCBidiReads())
			if err != nil {
				t.Fatalf("NewGRPCClient: %v", err)
			}
			t.Cleanup(func() { client.Close() })
			client.SetRetry(WithBackoff(gax.Backoff{Initial: time.Millisecond, Max: time.Millisecond}))

			tc.call(t, ctx, client)

			spans := te.Spans()
			var opSpan *tracetest.SpanStub
			for i := range spans {
				if spans[i].Name == appendPackageName(tc.opSpan) {
					if opSpan != nil {
						t.Fatalf("got more than one %s span", tc.opSpan)
					}
					opSpan = &spans[i]
				}
			}
			if opSpan == nil {
				t.Fatalf("%s span not found", tc.opSpan)
			}

			var gotAttempts []int64
			attemptSpans := 0
			for _, s := range spans {
				if s.Name != tc.attemptSpan {
					continue
				}
				attemptSpans++
				if got, want := s.Parent.SpanID(), opSpan.SpanContext.SpanID(); got != want {
					t.Errorf("%s span parent ID = %v, want %v", tc.attemptSpan, got, want)
				}
				if n, ok := retryAttemptNumber(s.Attributes); ok {
					gotAttempts = append(gotAttempts, n)
				}
			}
			if want := tc.failures + 1; attemptSpans != want {
				t.Errorf("got %d %s spans, want %d", attemptSpans, tc.attemptSpan, want)
			}
			if diff := cmp.Diff(tc.wantAttempts, gotAttempts); diff != "" {
				t.Errorf("attempt numbers on %s spans mismatch (-want +got):\n%s", tc.attemptSpan, diff)
			}
		})
	}
}

func TestRecordRetryBackoffNoParentSpan(t *testing.T) {
	ctx := context.Background()
	te := testutil.NewOpenTelemetryTestExporter()
	t.Cleanup(func() {
		te.Unregister(ctx)
	})
	t.Setenv(storageOtelTracingDevVar, "true")

	recordRetryBackoff(ctx, time.Now())

	if got := len(te.Spans()); got != 0 {
		t.Errorf("recordRetryBackoff() without a parent span exported %d spans, want 0", got)
	}
}

func TestRunRetryBackoffTracing(t *testing.T) {
	fastBackoff := &gax.Backoff{Initial: time.Millisecond, Max: time.Millisecond}
	maxAttempts := 2
	retryableErr := &googleapi.Error{Code: http.StatusServiceUnavailable}
	nonRetryableErr := &googleapi.Error{Code: http.StatusBadRequest}

	for _, tc := range []struct {
		name  string
		retry *retryConfig
		// errs[i] is returned by attempt i+1. Later attempts succeed.
		errs []error
		// timeout, if set, is applied to the context passed to run.
		timeout time.Duration
		wantErr bool
		// wantBackoffs is the number of RetryBackoff spans expected.
		wantBackoffs int
	}{
		{
			name:  "success on first attempt",
			retry: &retryConfig{backoff: fastBackoff},
		},
		{
			name:         "one retry",
			retry:        &retryConfig{backoff: fastBackoff},
			errs:         []error{retryableErr},
			wantBackoffs: 1,
		},
		{
			name:         "two retries",
			retry:        &retryConfig{backoff: fastBackoff},
			errs:         []error{retryableErr, retryableErr},
			wantBackoffs: 2,
		},
		{
			name:    "non-retryable error",
			retry:   &retryConfig{backoff: fastBackoff},
			errs:    []error{nonRetryableErr},
			wantErr: true,
		},
		{
			name:         "max attempts reached",
			retry:        &retryConfig{backoff: fastBackoff, maxAttempts: &maxAttempts},
			errs:         []error{retryableErr, retryableErr},
			wantErr:      true,
			wantBackoffs: 1,
		},
		{
			name: "context done during backoff",
			// gax picks a random pause in [1ns, Initial], so a huge Initial makes
			// a pause shorter than the timeout practically impossible.
			retry:        &retryConfig{backoff: &gax.Backoff{Initial: 1000 * time.Hour, Max: 1000 * time.Hour}},
			errs:         []error{retryableErr},
			timeout:      50 * time.Millisecond,
			wantErr:      true,
			wantBackoffs: 1,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ctx := context.Background()
			te := testutil.NewOpenTelemetryTestExporter()
			t.Cleanup(func() {
				te.Unregister(ctx)
			})
			t.Setenv(storageOtelTracingDevVar, "true")

			spanCtx, _ := startSpan(ctx, "Bucket.Attrs")
			runCtx := spanCtx
			if tc.timeout > 0 {
				var cancel context.CancelFunc
				runCtx, cancel = context.WithTimeout(spanCtx, tc.timeout)
				defer cancel()
			}
			attempt := 0
			err := run(runCtx, func(context.Context) error {
				attempt++
				if attempt <= len(tc.errs) {
					return tc.errs[attempt-1]
				}
				return nil
			}, tc.retry, true)
			endSpan(spanCtx, err)

			if gotErr := err != nil; gotErr != tc.wantErr {
				t.Fatalf("run() = %v, want error: %t", err, tc.wantErr)
			}
			checkRetryBackoffs(t, te.Spans(), tc.wantBackoffs, tc.timeout)
		})
	}
}

func TestRunRetryBackoffTracingDisabled(t *testing.T) {
	ctx := context.Background()
	te := testutil.NewOpenTelemetryTestExporter()
	t.Cleanup(func() {
		te.Unregister(ctx)
	})
	t.Setenv(storageOtelTracingDevVar, "false")

	// The span is started directly so that it is recording even though dev
	// tracing is disabled, proving that the environment gate alone prevents
	// the backoff from being recorded.
	spanCtx, span := tracer().Start(ctx, "Bucket.Attrs")
	attempt := 0
	err := run(spanCtx, func(context.Context) error {
		attempt++
		if attempt == 1 {
			return &googleapi.Error{Code: http.StatusServiceUnavailable}
		}
		return nil
	}, &retryConfig{backoff: &gax.Backoff{Initial: time.Millisecond, Max: time.Millisecond}}, true)
	span.End()

	if err != nil {
		t.Fatalf("run() = %v, want nil", err)
	}
	spans := te.Spans()
	if len(spans) != 1 {
		t.Errorf("run() produced %d spans, want 1", len(spans))
	}
}

// findSpans returns the spans in spans named with the package-qualified form
// of name.
func findSpans(spans tracetest.SpanStubs, name string) tracetest.SpanStubs {
	want := appendPackageName(name)
	var found tracetest.SpanStubs
	for _, s := range spans {
		if s.Name == want {
			found = append(found, s)
		}
	}
	return found
}

// checkRetryBackoffs checks that the single Bucket.Attrs span in spans has
// wantBackoffs RetryBackoff child spans, each lying within the parent span's
// time range and lasting at least minDuration.
func checkRetryBackoffs(t *testing.T, spans tracetest.SpanStubs, wantBackoffs int, minDuration time.Duration) {
	t.Helper()
	parents := findSpans(spans, "Bucket.Attrs")
	if len(parents) != 1 {
		t.Fatalf("got %d Bucket.Attrs spans, want 1", len(parents))
	}
	parent := parents[0]
	backoffs := findSpans(spans, "RetryBackoff")
	if len(backoffs) != wantBackoffs {
		t.Errorf("got %d RetryBackoff spans, want %d", len(backoffs), wantBackoffs)
	}
	for _, s := range backoffs {
		if got, want := s.Parent.SpanID(), parent.SpanContext.SpanID(); got != want {
			t.Errorf("RetryBackoff span parent ID = %v, want %v", got, want)
		}
		if s.StartTime.After(s.EndTime) {
			t.Errorf("RetryBackoff span start time %v is after end time %v", s.StartTime, s.EndTime)
		}
		if got := s.EndTime.Sub(s.StartTime); got < minDuration {
			t.Errorf("RetryBackoff span duration = %v, want >= %v", got, minDuration)
		}
		if s.StartTime.Before(parent.StartTime) || s.EndTime.After(parent.EndTime) {
			t.Errorf("RetryBackoff span [%v, %v] is outside parent span [%v, %v]", s.StartTime, s.EndTime, parent.StartTime, parent.EndTime)
		}
	}
}

func TestRecordWriterTraceAttributes(t *testing.T) {
	newMockObjectHandle := func() *ObjectHandle {
		c := &Client{tc: &mockStorageClient{
			openWriterFn: func(params *openWriterParams, _ ...storageOption) (internalWriter, error) {
				close(params.donec)
				return &mockInternalWriter{internalWriter: &httpInternalWriter{}}, nil
			},
		}}
		return c.Bucket("bucket").Object("object")
	}
	// Spans started via NewWriter also carry the object's storage URI.
	uriAttr := attribute.String(storageURIAttrKey, "gs://bucket/object")

	testCases := []struct {
		name      string
		run       func(*testing.T, context.Context)
		wantAttrs []attribute.KeyValue
	}{
		{
			name: "resumable_multi_chunk",
			run: func(t *testing.T, ctx context.Context) {
				w := newMockObjectHandle().NewWriter(ctx)
				w.ChunkSize = 256 * 1024
				if _, err := w.Write(make([]byte, 256*1024)); err != nil {
					t.Fatalf("Write failed: %v", err)
				}
				if err := w.Close(); err != nil {
					t.Fatalf("Close failed: %v", err)
				}
			},
			wantAttrs: []attribute.KeyValue{
				attribute.String("gcp.storage.write.mode", "resumable"),
				uriAttr,
			},
		},
		{
			name: "oneshot_zero_chunk_size",
			run: func(t *testing.T, ctx context.Context) {
				w := newMockObjectHandle().NewWriter(ctx)
				w.ChunkSize = 0
				if err := w.Close(); err != nil {
					t.Fatalf("Close failed: %v", err)
				}
			},
			wantAttrs: []attribute.KeyValue{
				attribute.String("gcp.storage.write.mode", "oneshot"),
				uriAttr,
			},
		},
		{
			name: "oneshot_small_payload_default_chunk_size",
			run: func(t *testing.T, ctx context.Context) {
				w := newMockObjectHandle().NewWriter(ctx)
				if _, err := w.Write([]byte("hello")); err != nil {
					t.Fatalf("Write failed: %v", err)
				}
				if err := w.Close(); err != nil {
					t.Fatalf("Close failed: %v", err)
				}
			},
			wantAttrs: []attribute.KeyValue{
				attribute.String("gcp.storage.write.mode", "oneshot"),
				uriAttr,
			},
		},
		{
			name: "oneshot_non_aligned_chunk_size",
			run: func(t *testing.T, ctx context.Context) {
				w := newMockObjectHandle().NewWriter(ctx)
				// 300 KiB rounds up to 512 KiB via gRPCChunkSize / googleapi.ChunkSize,
				// so a 400 KiB write fits in the first chunk and runs as oneshot.
				w.ChunkSize = 300 * 1024
				if _, err := w.Write(make([]byte, 400*1024)); err != nil {
					t.Fatalf("Write failed: %v", err)
				}
				if err := w.Close(); err != nil {
					t.Fatalf("Close failed: %v", err)
				}
			},
			wantAttrs: []attribute.KeyValue{
				attribute.String("gcp.storage.write.mode", "oneshot"),
				uriAttr,
			},
		},
		{
			name: "oneshot_grpc_force_oneshot",
			run: func(_ *testing.T, ctx context.Context) {
				ctx, _ = startSpan(ctx, "Object.Writer")
				recordWriterTraceAttributes(ctx, &Writer{
					ChunkSize: googleapi.DefaultUploadChunkSize,
					iw:        &gRPCWriter{forceOneShot: true},
				})
				endSpan(ctx, nil)
			},
			wantAttrs: []attribute.KeyValue{
				attribute.String("gcp.storage.write.mode", "oneshot"),
			},
		},
		{
			name: "resumable_grpc",
			run: func(_ *testing.T, ctx context.Context) {
				ctx, _ = startSpan(ctx, "Object.Writer")
				recordWriterTraceAttributes(ctx, &Writer{
					ChunkSize: googleapi.DefaultUploadChunkSize,
					iw:        &gRPCWriter{forceOneShot: false},
				})
				endSpan(ctx, nil)
			},
			wantAttrs: []attribute.KeyValue{
				attribute.String("gcp.storage.write.mode", "resumable"),
			},
		},
		{
			name: "appendable",
			run: func(t *testing.T, ctx context.Context) {
				w := newMockObjectHandle().NewWriter(ctx)
				w.Append = true
				if err := w.Close(); err != nil {
					t.Fatalf("Close failed: %v", err)
				}
			},
			wantAttrs: []attribute.KeyValue{
				attribute.String("gcp.storage.write.mode", "appendable"),
				uriAttr,
			},
		},
		{
			name: "open_writer_validation_failure",
			run: func(t *testing.T, ctx context.Context) {
				w := newMockObjectHandle().NewWriter(ctx)
				w.Name = "mismatched-name"
				if err := w.Close(); err == nil {
					t.Fatal("Close succeeded, want validation error")
				}
			},
			wantAttrs: []attribute.KeyValue{
				attribute.String("gcp.storage.write.mode", "resumable"),
				uriAttr,
			},
		},
		{
			name: "parallel",
			run: func(t *testing.T, ctx context.Context) {
				c := &Client{tc: &grpcStorageClient{}}
				w := c.Bucket("bucket").Object("object").NewWriter(ctx)
				w.EnableParallelUpload = true
				w.ParallelUploadConfig = ParallelUploadConfig{
					MaxConcurrency: 4,
				}
				w.MD5 = []byte{1, 2, 3}
				if err := w.Close(); err == nil {
					t.Fatal("Close succeeded, want validation error")
				}
			},
			wantAttrs: []attribute.KeyValue{
				attribute.String("gcp.storage.write.mode", "parallel"),
				attribute.Int("gcp.storage.write.parallel.part_size", 16*1024*1024),
				attribute.Int("gcp.storage.write.parallel.concurrency", 4),
				uriAttr,
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			ctx := context.Background()
			te := testutil.NewOpenTelemetryTestExporter()
			t.Cleanup(func() {
				te.Unregister(ctx)
			})
			// TODO: Remove setting development env var upon launch.
			t.Setenv(storageOtelTracingDevVar, "true")

			tc.run(t, ctx)

			verifySingleSpanAttributes(t, te.Spans(), tc.wantAttrs)
		})
	}
}

func TestWriterTraceWriteModeEmulated(t *testing.T) {
	checkEmulatorEnvironment(t)

	ctx := context.Background()
	c := &Client{tc: emulatorClients["grpc"]}
	bucket := fmt.Sprintf("grpc-trace-bucket-%d", time.Now().UnixNano())
	if err := c.Bucket(bucket).Create(ctx, "project", nil); err != nil {
		t.Fatalf("Bucket.Create: %v", err)
	}

	testCases := []struct {
		name      string
		configure func(*Writer)
		payload   []byte
		wantAttrs []attribute.KeyValue
	}{
		{
			name:      "oneshot",
			configure: func(_ *Writer) {},
			payload:   make([]byte, 1024),
			wantAttrs: []attribute.KeyValue{
				attribute.String("gcp.storage.write.mode", "oneshot"),
			},
		},
		{
			name: "resumable",
			configure: func(w *Writer) {
				w.ChunkSize = 256 * 1024
			},
			payload: make([]byte, 1024*1024),
			wantAttrs: []attribute.KeyValue{
				attribute.String("gcp.storage.write.mode", "resumable"),
			},
		},
		{
			name: "appendable",
			configure: func(w *Writer) {
				w.Append = true
				w.FinalizeOnClose = true
			},
			payload: make([]byte, 1024),
			wantAttrs: []attribute.KeyValue{
				attribute.String("gcp.storage.write.mode", "appendable"),
			},
		},
		{
			name: "parallel",
			configure: func(w *Writer) {
				w.EnableParallelUpload = true
				w.ParallelUploadConfig = ParallelUploadConfig{
					PartSize:       8 * 1024 * 1024,
					MaxConcurrency: 4,
				}
			},
			payload: make([]byte, 1024),
			wantAttrs: []attribute.KeyValue{
				attribute.String("gcp.storage.write.mode", "parallel"),
				attribute.Int("gcp.storage.write.parallel.part_size", 8*1024*1024),
				attribute.Int("gcp.storage.write.parallel.concurrency", 4),
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			te := testutil.NewOpenTelemetryTestExporter()
			t.Cleanup(func() {
				te.Unregister(ctx)
			})
			// TODO: Remove setting development env var upon launch.
			t.Setenv(storageOtelTracingDevVar, "true")

			objName := fmt.Sprintf("object-%s-%d", tc.name, time.Now().UnixNano())
			w := c.Bucket(bucket).Object(objName).NewWriter(ctx)
			tc.configure(w)
			if _, err := w.Write(tc.payload); err != nil {
				t.Fatalf("Write: %v", err)
			}
			if err := w.Close(); err != nil {
				t.Fatalf("Close: %v", err)
			}

			var writerSpans tracetest.SpanStubs
			for _, s := range te.Spans() {
				if s.Name == appendPackageName("Object.Writer") && !s.Parent.IsValid() {
					writerSpans = append(writerSpans, s)
				}
			}
			verifySingleSpanAttributes(t, writerSpans, append(tc.wantAttrs,
				attribute.String(storageURIAttrKey, "gs://"+bucket+"/"+objName)))
		})
	}
}

// TestReaderTraceReadMode verifies the read mode recorded by NewRangeReader and
// NewMultiRangeDownloader. The ObjectHandle has no object name, so each call
// fails validation after the span has been started and the attribute recorded,
// before any RPC is made.
func TestReaderTraceReadMode(t *testing.T) {
	testCases := []struct {
		name     string
		read     func(context.Context, *ObjectHandle) error
		wantMode string
	}{
		{
			name: "full",
			read: func(ctx context.Context, o *ObjectHandle) error {
				_, err := o.NewReader(ctx)
				return err
			},
			wantMode: "full",
		},
		{
			name: "range",
			read: func(ctx context.Context, o *ObjectHandle) error {
				_, err := o.NewRangeReader(ctx, 100, 500)
				return err
			},
			wantMode: "range",
		},
		{
			name: "range_to_end",
			read: func(ctx context.Context, o *ObjectHandle) error {
				_, err := o.NewRangeReader(ctx, 100, -1)
				return err
			},
			wantMode: "range",
		},
		{
			name: "range_suffix",
			read: func(ctx context.Context, o *ObjectHandle) error {
				_, err := o.NewRangeReader(ctx, -100, -1)
				return err
			},
			wantMode: "range",
		},
		{
			name: "multi_range",
			read: func(ctx context.Context, o *ObjectHandle) error {
				_, err := o.NewMultiRangeDownloader(ctx)
				return err
			},
			wantMode: "multi_range",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			ctx := context.Background()
			te := testutil.NewOpenTelemetryTestExporter()
			t.Cleanup(func() {
				te.Unregister(ctx)
			})
			// TODO: Remove setting development env var upon launch.
			t.Setenv(storageOtelTracingDevVar, "true")

			o := &ObjectHandle{c: &Client{}, bucket: "bucket"}
			if err := tc.read(ctx, o); err == nil {
				t.Fatal("read succeeded, want validation error")
			}

			verifySingleSpanAttributes(t, te.Spans(), []attribute.KeyValue{
				attribute.String("gcp.storage.read.mode", tc.wantMode),
				attribute.String(storageURIAttrKey, "gs://bucket/"),
			})
		})
	}
}

func TestRecordTraceAttributes_DevTracingDisabled(t *testing.T) {
	ctx := context.Background()
	te := testutil.NewOpenTelemetryTestExporter()
	t.Cleanup(func() {
		te.Unregister(ctx)
	})
	t.Setenv(storageOtelTracingDevVar, "false")

	wCtx, _ := startSpan(ctx, "Object.Writer")
	recordWriterTraceAttributes(wCtx, &Writer{EnableParallelUpload: true})
	endSpan(wCtx, nil)

	for _, s := range te.Spans() {
		for _, a := range s.Attributes {
			switch a.Key {
			case "gcp.storage.write.mode",
				"gcp.storage.write.parallel.part_size",
				"gcp.storage.write.parallel.concurrency":
				t.Errorf("span %s has unexpected attribute %s when dev tracing is disabled", s.Name, a.Key)
			}
		}
	}
}

// verifySingleSpanAttributes checks that exactly one span was exported and that
// its attributes are exactly the common attributes plus wantAttrs.
func verifySingleSpanAttributes(t *testing.T, spans tracetest.SpanStubs, wantAttrs []attribute.KeyValue) {
	t.Helper()
	if len(spans) != 1 {
		t.Fatalf("expected one span, got %d", len(spans))
	}
	// Error attributes depend on the error that ended the span and are covered
	// by TestErrorAttributes, so they are not compared here.
	got, _ := attribute.NewSetWithFiltered(spans[0].Attributes, func(kv attribute.KeyValue) bool {
		return kv.Key != "error.type" && kv.Key != "gcp.errors.domain"
	})
	want := attribute.NewSet(append(getCommonAttributes(), wantAttrs...)...)
	if !got.Equals(&want) {
		t.Errorf("span %s attributes mismatch:\n got: %s\nwant: %s", spans[0].Name,
			got.Encoded(attribute.DefaultEncoder()), want.Encoded(attribute.DefaultEncoder()))
	}
}

// chunkTraceTestServer is a fake storage server for resumable and oneshot
// BidiWriteObject uploads. If failAt is positive, the first stream to receive
// data past offset failAt fails with UNAVAILABLE before persisting it.
type chunkTraceTestServer struct {
	storagepb.UnimplementedStorageServer
	failAt int64

	mu        sync.Mutex
	persisted int64
	failed    bool
}

func (s *chunkTraceTestServer) StartResumableWrite(context.Context, *storagepb.StartResumableWriteRequest) (*storagepb.StartResumableWriteResponse, error) {
	return &storagepb.StartResumableWriteResponse{UploadId: "upload-id"}, nil
}

func (s *chunkTraceTestServer) QueryWriteStatus(context.Context, *storagepb.QueryWriteStatusRequest) (*storagepb.QueryWriteStatusResponse, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return &storagepb.QueryWriteStatusResponse{
		WriteStatus: &storagepb.QueryWriteStatusResponse_PersistedSize{PersistedSize: s.persisted},
	}, nil
}

func (s *chunkTraceTestServer) BidiWriteObject(stream storagepb.Storage_BidiWriteObjectServer) error {
	for {
		req, err := stream.Recv()
		if err == io.EOF {
			return nil
		}
		if err != nil {
			return err
		}
		n := int64(len(req.GetChecksummedData().GetContent()))
		size := req.GetWriteOffset() + n

		s.mu.Lock()
		if s.failAt > 0 && !s.failed && size > s.failAt {
			s.failed = true
			s.mu.Unlock()
			return status.Error(codes.Unavailable, "injected stream failure")
		}
		if req.GetFlush() || req.GetFinishWrite() {
			if size > s.persisted {
				s.persisted = size
			}
		}
		persisted := s.persisted
		s.mu.Unlock()

		if req.GetFinishWrite() {
			return stream.Send(&storagepb.BidiWriteObjectResponse{
				WriteStatus: &storagepb.BidiWriteObjectResponse_Resource{
					Resource: &storagepb.Object{Bucket: "projects/_/buckets/bucket", Name: "object", Size: persisted},
				},
			})
		}
		if req.GetStateLookup() {
			if err := stream.Send(&storagepb.BidiWriteObjectResponse{
				WriteStatus: &storagepb.BidiWriteObjectResponse_PersistedSize{PersistedSize: persisted},
			}); err != nil {
				return err
			}
		}
	}
}

// uploadChunkSpanInfo describes an exported Storage.UploadChunk span.
type uploadChunkSpanInfo struct {
	Number int64
	Offset int64
	Size   int64
	Failed bool
}

// uploadChunkSpans returns the Storage.UploadChunk spans in spans in the order
// they ended. It fails the test if one is not a child of the Object.Writer
// span.
func uploadChunkSpans(t *testing.T, spans tracetest.SpanStubs) []uploadChunkSpanInfo {
	t.Helper()
	var writer *tracetest.SpanStub
	for i := range spans {
		if spans[i].Name == appendPackageName("Object.Writer") {
			writer = &spans[i]
		}
	}
	var got []uploadChunkSpanInfo
	for _, s := range spans {
		if s.Name != appendPackageName(uploadChunkSpanName) {
			continue
		}
		if writer == nil {
			t.Fatalf("got %s span without an Object.Writer span", uploadChunkSpanName)
		}
		if gotID, wantID := s.Parent.SpanID(), writer.SpanContext.SpanID(); gotID != wantID {
			t.Errorf("%s span parent = %v, want Object.Writer span %v", uploadChunkSpanName, gotID, wantID)
		}
		info := uploadChunkSpanInfo{Number: -1, Offset: -1, Size: -1, Failed: s.Status.Code == otcodes.Error}
		for _, a := range s.Attributes {
			switch a.Key {
			case uploadChunkNumberAttr:
				info.Number = a.Value.AsInt64()
			case payloadOffsetAttr:
				info.Offset = a.Value.AsInt64()
			case payloadSizeBytesAttr:
				info.Size = a.Value.AsInt64()
			}
		}
		got = append(got, info)
	}
	// No other span, such as the gRPC attempt span, carries the attributes.
	for _, s := range spans {
		if s.Name == appendPackageName(uploadChunkSpanName) {
			continue
		}
		for _, a := range s.Attributes {
			if a.Key == uploadChunkNumberAttr || a.Key == payloadOffsetAttr || a.Key == payloadSizeBytesAttr {
				t.Errorf("span %q has attribute %s, want it only on %s spans", s.Name, a.Key, uploadChunkSpanName)
			}
		}
	}
	return got
}

func TestGRPCWriterUploadChunkSpans(t *testing.T) {
	const kib = 1024
	const mib = 1024 * kib

	for _, tc := range []struct {
		name       string
		devTracing bool
		chunkSize  int
		// write uploads the object through w. Write errors fail the test.
		write func(t *testing.T, w *Writer)
		want  []uploadChunkSpanInfo
	}{
		{
			name:       "multiple chunks",
			devTracing: true,
			chunkSize:  256 * kib,
			write:      writeBytes(mib),
			want: []uploadChunkSpanInfo{
				{Number: 1, Offset: 0, Size: 256 * kib},
				{Number: 2, Offset: 256 * kib, Size: 256 * kib},
				{Number: 3, Offset: 512 * kib, Size: 256 * kib},
				{Number: 4, Offset: 768 * kib, Size: 256 * kib},
			},
		},
		{
			name:       "final partial chunk",
			devTracing: true,
			chunkSize:  256 * kib,
			write:      writeBytes(mib + 100*kib),
			want: []uploadChunkSpanInfo{
				{Number: 1, Offset: 0, Size: 256 * kib},
				{Number: 2, Offset: 256 * kib, Size: 256 * kib},
				{Number: 3, Offset: 512 * kib, Size: 256 * kib},
				{Number: 4, Offset: 768 * kib, Size: 256 * kib},
				{Number: 5, Offset: mib, Size: 100 * kib},
			},
		},
		{
			name:       "chunk sent as several messages",
			devTracing: true,
			chunkSize:  4 * mib, // Two 2 MiB messages per chunk; one span each.
			write:      writeBytes(10 * mib),
			want: []uploadChunkSpanInfo{
				{Number: 1, Offset: 0, Size: 4 * mib},
				{Number: 2, Offset: 4 * mib, Size: 4 * mib},
				{Number: 3, Offset: 8 * mib, Size: 2 * mib},
			},
		},
		{
			name:       "dev tracing disabled",
			devTracing: false,
			chunkSize:  256 * kib,
			write:      writeBytes(mib),
		},
		{
			name:       "oneshot",
			devTracing: true,
			chunkSize:  0,
			write:      writeBytes(mib),
		},
		{
			// An object that fits in one chunk is sent as a oneshot upload
			// (see gatherFirstBuffer), so no chunk spans are recorded.
			name:       "payload smaller than one chunk",
			devTracing: true,
			chunkSize:  256 * kib,
			write:      writeBytes(100 * kib),
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			srv := &chunkTraceTestServer{}
			te := runChunkTraceUpload(t, srv, tc.devTracing, tc.chunkSize, tc.write)

			got := uploadChunkSpans(t, te.Spans())
			if diff := cmp.Diff(tc.want, got); diff != "" {
				t.Errorf("%s spans mismatch (-want +got):\n%s", uploadChunkSpanName, diff)
			}
		})
	}
}

func TestGRPCWriterUploadChunkSpansRetry(t *testing.T) {
	const kib = 1024
	// Chunk 3 covers [512 KiB, 768 KiB). Its stream fails mid-chunk, before
	// GCS persists it, and the upload resumes from 512 KiB.
	srv := &chunkTraceTestServer{failAt: 600 * kib}
	te := runChunkTraceUpload(t, srv, true, 256*kib, writeBytes(1024*kib))

	got := uploadChunkSpans(t, te.Spans())
	var okSpans []uploadChunkSpanInfo
	failedSpans := map[int64]uploadChunkSpanInfo{}
	for _, s := range got {
		if s.Failed {
			failedSpans[s.Number] = s
		} else {
			okSpans = append(okSpans, s)
		}
	}
	wantOK := []uploadChunkSpanInfo{
		{Number: 1, Offset: 0, Size: 256 * kib},
		{Number: 2, Offset: 256 * kib, Size: 256 * kib},
		{Number: 3, Offset: 512 * kib, Size: 256 * kib},
		{Number: 4, Offset: 768 * kib, Size: 256 * kib},
	}
	if diff := cmp.Diff(wantOK, okSpans); diff != "" {
		t.Errorf("successful %s spans mismatch (-want +got):\n%s", uploadChunkSpanName, diff)
	}
	wantFailed3 := uploadChunkSpanInfo{Number: 3, Offset: 512 * kib, Size: 256 * kib, Failed: true}
	if diff := cmp.Diff(wantFailed3, failedSpans[3]); diff != "" {
		t.Errorf("failed chunk 3 span mismatch (-want +got):\n%s", diff)
	}
	for n := range failedSpans {
		// Chunk 4 can be in flight when chunk 3 fails, so it may also fail.
		if n != 3 && n != 4 {
			t.Errorf("got failed span for chunk %d, want only chunks 3 and 4 to fail; all spans: %+v", n, got)
		}
	}
	srv.mu.Lock()
	defer srv.mu.Unlock()
	if !srv.failed {
		t.Error("server did not inject the stream failure")
	}
}

// writeBytes returns a write func that writes n bytes in one call.
func writeBytes(n int) func(*testing.T, *Writer) {
	return func(t *testing.T, w *Writer) {
		t.Helper()
		if _, err := w.Write(make([]byte, n)); err != nil {
			t.Fatalf("Write: %v", err)
		}
	}
}

// runChunkTraceUpload uploads an object to srv over gRPC with the given chunk
// size and returns the exporter holding the recorded spans.
func runChunkTraceUpload(t *testing.T, srv *chunkTraceTestServer, devTracing bool, chunkSize int, write func(*testing.T, *Writer)) *testutil.OpenTelemetryTestExporter {
	t.Helper()
	ctx := context.Background()
	te := testutil.NewOpenTelemetryTestExporter()
	t.Cleanup(func() { te.Unregister(ctx) })
	// TODO: Remove setting development env var upon launch.
	t.Setenv(storageOtelTracingDevVar, strconv.FormatBool(devTracing))
	t.Setenv(storageBucketMetadataDisabledVar, "true")
	startFakeStorageServer(t, srv)

	client, err := NewGRPCClient(ctx)
	if err != nil {
		t.Fatalf("NewGRPCClient: %v", err)
	}
	t.Cleanup(func() { client.Close() })
	client.SetRetry(WithBackoff(gax.Backoff{Initial: time.Millisecond, Max: time.Millisecond}))

	w := client.Bucket("bucket").Object("object").NewWriter(ctx)
	w.ChunkSize = chunkSize
	w.DisableAutoChecksum = true
	write(t, w)
	if err := w.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
	return te
}

func TestHTTPWriterUploadChunkSpans(t *testing.T) {
	const kib = 1024
	const mib = 1024 * kib

	for _, tc := range []struct {
		name       string
		devTracing bool
		chunkSize  int
		write      func(t *testing.T, w *Writer)
		want       []uploadChunkSpanInfo
	}{
		{
			name:       "multiple chunks",
			devTracing: true,
			chunkSize:  256 * kib,
			write:      writeBytes(mib),
			want: []uploadChunkSpanInfo{
				{Number: 1, Offset: 0, Size: 256 * kib},
				{Number: 2, Offset: 256 * kib, Size: 256 * kib},
				{Number: 3, Offset: 512 * kib, Size: 256 * kib},
				{Number: 4, Offset: 768 * kib, Size: 256 * kib},
			},
		},
		{
			name:       "final partial chunk",
			devTracing: true,
			chunkSize:  256 * kib,
			write:      writeBytes(mib + 100*kib),
			want: []uploadChunkSpanInfo{
				{Number: 1, Offset: 0, Size: 256 * kib},
				{Number: 2, Offset: 256 * kib, Size: 256 * kib},
				{Number: 3, Offset: 512 * kib, Size: 256 * kib},
				{Number: 4, Offset: 768 * kib, Size: 256 * kib},
				{Number: 5, Offset: mib, Size: 100 * kib},
			},
		},
		{
			name:       "dev tracing disabled",
			devTracing: false,
			chunkSize:  256 * kib,
			write:      writeBytes(mib),
		},
		{
			name:       "oneshot",
			devTracing: true,
			chunkSize:  0,
			write:      writeBytes(mib),
		},
		{
			name:       "payload smaller than one chunk",
			devTracing: true,
			chunkSize:  256 * kib,
			write:      writeBytes(100 * kib),
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			te := runHTTPChunkTraceUpload(t, 0, tc.devTracing, tc.chunkSize, tc.write)

			got := uploadChunkSpans(t, te.Spans())
			if diff := cmp.Diff(tc.want, got); diff != "" {
				t.Errorf("%s spans mismatch (-want +got):\n%s", uploadChunkSpanName, diff)
			}
		})
	}
}

func TestHTTPWriterUploadChunkSpansRetry(t *testing.T) {
	const kib = 1024
	// Fail the first attempt of chunk 3 (offset 512 KiB).
	te := runHTTPChunkTraceUpload(t, 512*kib, true, 256*kib, writeBytes(1024*kib))

	got := uploadChunkSpans(t, te.Spans())
	want := []uploadChunkSpanInfo{
		{Number: 1, Offset: 0, Size: 256 * kib},
		{Number: 2, Offset: 256 * kib, Size: 256 * kib},
		{Number: 3, Offset: 512 * kib, Size: 256 * kib, Failed: true},
		{Number: 3, Offset: 512 * kib, Size: 256 * kib},
		{Number: 4, Offset: 768 * kib, Size: 256 * kib},
	}
	if diff := cmp.Diff(want, got); diff != "" {
		t.Errorf("%s spans mismatch (-want +got):\n%s", uploadChunkSpanName, diff)
	}
}

func runHTTPChunkTraceUpload(t *testing.T, failOffset int64, devTracing bool, chunkSize int, write func(*testing.T, *Writer)) *testutil.OpenTelemetryTestExporter {
	t.Helper()
	ctx := context.Background()
	te := testutil.NewOpenTelemetryTestExporter()
	t.Cleanup(func() { te.Unregister(ctx) })
	t.Setenv(storageOtelTracingDevVar, strconv.FormatBool(devTracing))
	t.Setenv(storageBucketMetadataDisabledVar, "true")

	var mu sync.Mutex
	var failedOnce bool
	var srv *httptest.Server
	srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		io.Copy(io.Discard, r.Body)
		if r.URL.Query().Get("uploadType") == "resumable" && r.Header.Get("Content-Range") == "" {
			w.Header().Set("Location", srv.URL+"/upload?upload_id=test-session")
			w.WriteHeader(http.StatusOK)
			return
		}
		cr := r.Header.Get("Content-Range")
		if cr != "" {
			mu.Lock()
			if failOffset > 0 && !failedOnce && strings.HasPrefix(cr, fmt.Sprintf("bytes %d-", failOffset)) {
				failedOnce = true
				mu.Unlock()
				w.WriteHeader(http.StatusServiceUnavailable)
				return
			}
			mu.Unlock()
			if strings.HasSuffix(cr, "/*") {
				w.Header().Set("X-Http-Status-Code-Override", "308")
				w.WriteHeader(http.StatusOK)
				return
			}
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"name":"object","bucket":"bucket"}`))
	}))
	t.Cleanup(srv.Close)

	client, err := NewClient(ctx, option.WithEndpoint(srv.URL), option.WithoutAuthentication())
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}
	t.Cleanup(func() { client.Close() })
	client.SetRetry(WithBackoff(gax.Backoff{Initial: time.Millisecond, Max: time.Millisecond}), WithPolicy(RetryAlways))

	w := client.Bucket("bucket").Object("object").NewWriter(ctx)
	w.ChunkSize = chunkSize
	w.DisableAutoChecksum = true
	write(t, w)
	if err := w.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
	return te
}

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
	"bytes"
	"context"
	"errors"
	"fmt"
	"hash/crc32"
	"io"
	"net"
	"net/http"
	"slices"
	"strconv"
	"strings"
	"sync"
	"testing"
	"testing/iotest"
	"time"

	"cloud.google.com/go/auth"
	"cloud.google.com/go/iam"
	"cloud.google.com/go/iam/apiv1/iampb"
	"cloud.google.com/go/internal/testutil"
	"cloud.google.com/go/storage/internal"
	gapic "cloud.google.com/go/storage/internal/apiv2"
	"cloud.google.com/go/storage/internal/apiv2/storagepb"
	"github.com/google/go-cmp/cmp"
	gax "github.com/googleapis/gax-go/v2"
	"github.com/googleapis/gax-go/v2/apierror"
	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"
	"go.opentelemetry.io/otel/attribute"
	otcodes "go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/sdk/instrumentation"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
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
	"google.golang.org/grpc/test/bufconn"
	"google.golang.org/protobuf/proto"
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

func TestFinishSpanNoEviction(t *testing.T) {
	t.Setenv("GO_STORAGE_DEV_OTEL_TRACING", "true")

	bucketName := "evict-bucket"
	cache := newBucketMetadataCache(10, &mockMetadataFetcher{})
	client := &Client{bucketMetadataCache: cache}
	cache.put(bucketName, bucketMetadata{resource: "res", location: "loc"})

	ctx, _ := startSpanWithBucket(context.Background(), client, bucketName, "Bucket.Attrs")
	childCtx, _ := startSpan(ctx, "Auth.RefreshAccessToken")
	finishSpan(childCtx, ErrBucketNotExist)

	if _, found := cache.get(bucketName); !found {
		t.Errorf("finishSpan evicted the operation's bucket; want it to remain in cache")
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

func TestAttemptStatsHandlerTagRPC(t *testing.T) {
	t.Setenv(storageOtelTracingDevVar, "true")
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
			attemptStatsHandler{}.TagRPC(tagCtx, &stats.RPCTagInfo{})
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
	req, err := stream.Recv()
	if err != nil {
		return err
	}
	if s.nextShouldFail() {
		return status.Error(codes.Unavailable, "retry me")
	}
	// Answer each request with the ranges it asks for, each split across two
	// responses; the first response also carries the object metadata.
	first := &storagepb.BidiReadObjectResponse{Metadata: &storagepb.Object{Size: 100}}
	for {
		second := &storagepb.BidiReadObjectResponse{}
		for _, r := range req.GetReadRanges() {
			half := r.GetReadLength() / 2
			first.ObjectDataRanges = append(first.ObjectDataRanges, &storagepb.ObjectRangeData{
				ChecksummedData: &storagepb.ChecksummedData{Content: make([]byte, half)},
				ReadRange:       &storagepb.ReadRange{ReadOffset: r.GetReadOffset(), ReadLength: half, ReadId: r.GetReadId()},
			})
			second.ObjectDataRanges = append(second.ObjectDataRanges, &storagepb.ObjectRangeData{
				ChecksummedData: &storagepb.ChecksummedData{Content: make([]byte, r.GetReadLength()-half)},
				ReadRange:       &storagepb.ReadRange{ReadOffset: r.GetReadOffset() + half, ReadLength: r.GetReadLength() - half, ReadId: r.GetReadId()},
				RangeEnd:        true,
			})
		}
		for _, resp := range []*storagepb.BidiReadObjectResponse{first, second} {
			if proto.Size(resp) == 0 {
				continue
			}
			if err := stream.Send(resp); err != nil {
				return err
			}
		}
		first = &storagepb.BidiReadObjectResponse{}
		if req, err = stream.Recv(); errors.Is(err, io.EOF) {
			return nil
		} else if err != nil {
			return err
		}
	}
}

// ReadObject returns the requested range of a 100-byte object in two
// responses, the first of which also carries the object metadata.
func (s *retryAttemptTestServer) ReadObject(req *storagepb.ReadObjectRequest, stream storagepb.Storage_ReadObjectServer) error {
	if s.nextShouldFail() {
		return status.Error(codes.Unavailable, "retry me")
	}
	const size = 100
	n := req.GetReadLimit()
	if n <= 0 {
		n = size - req.GetReadOffset()
	}
	// Like GCS, send a CRC32C with each chunk of data.
	chunk := func(n int64) *storagepb.ChecksummedData {
		data := make([]byte, n)
		crc := crc32.Checksum(data, crc32cTable)
		return &storagepb.ChecksummedData{Content: data, Crc32C: &crc}
	}
	half := n / 2
	if err := stream.Send(&storagepb.ReadObjectResponse{
		ChecksummedData: chunk(half),
		ContentRange:    &storagepb.ContentRange{Start: req.GetReadOffset(), End: req.GetReadOffset() + n, CompleteLength: size},
		Metadata:        &storagepb.Object{Bucket: "projects/_/buckets/bucket", Name: "object", Size: size, Generation: 1},
	}); err != nil {
		return err
	}
	return stream.Send(&storagepb.ReadObjectResponse{ChecksummedData: chunk(n - half)})
}

// StartResumableWrite starts a resumable upload.
func (s *retryAttemptTestServer) StartResumableWrite(context.Context, *storagepb.StartResumableWriteRequest) (*storagepb.StartResumableWriteResponse, error) {
	return &storagepb.StartResumableWriteResponse{UploadId: "upload"}, nil
}

// BidiWriteObject receives messages, acknowledging flushes, until the write is
// finished and returns the resulting object.
func (s *retryAttemptTestServer) BidiWriteObject(stream storagepb.Storage_BidiWriteObjectServer) error {
	var size int64
	for {
		req, err := stream.Recv()
		if err != nil {
			return err
		}
		size = req.GetWriteOffset() + int64(len(req.GetChecksummedData().GetContent()))
		if req.GetFinishWrite() {
			break
		}
		if req.GetFlush() {
			if err := stream.Send(&storagepb.BidiWriteObjectResponse{
				WriteStatus: &storagepb.BidiWriteObjectResponse_PersistedSize{PersistedSize: size},
			}); err != nil {
				return err
			}
		}
	}
	return stream.Send(&storagepb.BidiWriteObjectResponse{
		WriteStatus: &storagepb.BidiWriteObjectResponse_Resource{
			Resource: &storagepb.Object{Bucket: "projects/_/buckets/bucket", Name: "object", Size: size},
		},
	})
}

// payloadAttrs returns the (offset, size) payload attributes in attrs, with
// -1 for an absent attribute.
func payloadAttrs(attrs []attribute.KeyValue) [2]int64 {
	got := [2]int64{-1, -1}
	for _, a := range attrs {
		switch a.Key {
		case "gcp.storage.payload.offset":
			got[0] = a.Value.AsInt64()
		case "gcp.storage.payload.size_bytes":
			got[1] = a.Value.AsInt64()
		}
	}
	return got
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

	testCases := []struct {
		name      string
		configure func(*Writer)
		payload   []byte
		wantAttrs []attribute.KeyValue
		// wantUploadID is set when the upload creates a resumable session, whose
		// server-generated ID must be recorded as gcp.storage.upload.id.
		wantUploadID bool
		// grpcOnly is set for upload modes that are not supported over HTTP.
		grpcOnly bool
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
			wantUploadID: true,
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
			grpcOnly: true,
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
			grpcOnly: true,
		},
	}

	for _, transport := range []string{"grpc", "http"} {
		t.Run(transport, func(t *testing.T) {
			c := &Client{tc: emulatorClients[transport]}
			bucket := fmt.Sprintf("%s-trace-bucket-%d", transport, time.Now().UnixNano())
			if err := c.Bucket(bucket).Create(ctx, "project", nil); err != nil {
				t.Fatalf("Bucket.Create: %v", err)
			}

			for _, tc := range testCases {
				t.Run(tc.name, func(t *testing.T) {
					if tc.grpcOnly && transport != "grpc" {
						t.Skipf("%s uploads are gRPC-only", tc.name)
					}
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
					wantAttrs := append(slices.Clone(tc.wantAttrs),
						attribute.String(storageURIAttrKey, "gs://"+bucket+"/"+objName))
					if tc.wantUploadID && len(writerSpans) == 1 {
						// The upload ID is server-generated, so only check that it is present.
						var uploadID string
						for _, a := range writerSpans[0].Attributes {
							if a.Key == "gcp.storage.upload.id" {
								uploadID = a.Value.AsString()
							}
						}
						if uploadID == "" {
							t.Errorf("Object.Writer span missing non-empty gcp.storage.upload.id")
						}
						wantAttrs = append(wantAttrs, attribute.String("gcp.storage.upload.id", uploadID))
					}
					verifySingleSpanAttributes(t, writerSpans, wantAttrs)
				})
			}
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

type staticAuthTokenProvider struct {
	token *auth.Token
	err   error
}

func (s *staticAuthTokenProvider) Token(ctx context.Context) (*auth.Token, error) {
	return s.token, s.err
}

func findSpansByName(spans tracetest.SpanStubs, name string) tracetest.SpanStubs {
	var matched tracetest.SpanStubs
	for _, s := range spans {
		if s.Name == name {
			matched = append(matched, s)
		}
	}
	return matched
}

func TestWrapAuthCredentialsForTracing(t *testing.T) {
	if wrapAuthCredentialsForTracing(nil) != nil {
		t.Errorf("expected nil when c is nil")
	}
	c := &auth.Credentials{}
	if wrapAuthCredentialsForTracing(c) != c {
		t.Errorf("expected original credentials when TokenProvider is nil")
	}
	c.TokenProvider = &staticAuthTokenProvider{token: &auth.Token{Value: "fake-token"}}
	wrapped := wrapAuthCredentialsForTracing(c)
	if wrapped == c {
		t.Errorf("expected a new copy of credentials, got original")
	}
	if _, ok := wrapped.TokenProvider.(*tracedTokenProvider); !ok {
		t.Errorf("expected TokenProvider to be *tracedTokenProvider")
	}
	wrapped2 := wrapAuthCredentialsForTracing(wrapped)
	if wrapped2 != wrapped {
		t.Errorf("expected original wrapped credentials on double wrap")
	}
	metricsWrapped := wrapAuthCredentials(wrapped, &clientMetrics{})
	if wrapAuthCredentialsForTracing(metricsWrapped) != metricsWrapped {
		t.Errorf("expected original credentials when already wrapped inside metricsTokenProvider")
	}
}

func TestTracedTokenProviderSpan(t *testing.T) {
	ctx := context.Background()
	te := testutil.NewOpenTelemetryTestExporter()
	t.Cleanup(func() {
		te.Unregister(ctx)
	})
	t.Setenv("GO_STORAGE_DEV_OTEL_TRACING", "true")

	creds := wrapAuthCredentialsForTracing(&auth.Credentials{
		TokenProvider: &staticAuthTokenProvider{token: &auth.Token{Value: "fake-token"}},
	})

	parentCtx, parentSpan := startSpan(ctx, "Object.Writer")
	tok, err := creds.Token(parentCtx)
	endSpan(parentCtx, nil)
	if err != nil {
		t.Fatalf("creds.Token: %v", err)
	}
	if tok.Value != "fake-token" {
		t.Fatalf("got access token %q, want %q", tok.Value, "fake-token")
	}

	const wantSpanName = "cloud.google.com/go/storage.Auth.RefreshAccessToken"
	authSpans := findSpansByName(te.Spans(), wantSpanName)
	if len(authSpans) != 1 {
		t.Fatalf("expected 1 %q span, got %d", wantSpanName, len(authSpans))
	}
	if got, want := authSpans[0].Parent.SpanID(), parentSpan.SpanContext().SpanID(); got != want {
		t.Errorf("got parent span ID %v, want %v", got, want)
	}
}

func TestTracedTokenProviderError(t *testing.T) {
	ctx := context.Background()
	te := testutil.NewOpenTelemetryTestExporter()
	t.Cleanup(func() {
		te.Unregister(ctx)
	})
	t.Setenv("GO_STORAGE_DEV_OTEL_TRACING", "true")

	wantErr := errors.New("token refresh failed")
	creds := wrapAuthCredentialsForTracing(&auth.Credentials{
		TokenProvider: &staticAuthTokenProvider{err: wantErr},
	})

	parentCtx, _ := startSpan(ctx, "Object.Writer")
	_, err := creds.Token(parentCtx)
	endSpan(parentCtx, nil)
	if !errors.Is(err, wantErr) {
		t.Fatalf("got err %v, want %v", err, wantErr)
	}

	const wantSpanName = "cloud.google.com/go/storage.Auth.RefreshAccessToken"
	authSpans := findSpansByName(te.Spans(), wantSpanName)
	if len(authSpans) != 1 {
		t.Fatalf("expected 1 %q span, got %d", wantSpanName, len(authSpans))
	}
	span := authSpans[0]
	if span.Status.Code != otcodes.Error {
		t.Errorf("got span status code %v, want %v", span.Status.Code, otcodes.Error)
	}
	if span.Status.Description != wantErr.Error() {
		t.Errorf("got span status description %q, want %q", span.Status.Description, wantErr.Error())
	}
	hasExceptionEvent := false
	for _, ev := range span.Events {
		if ev.Name == "exception" {
			hasExceptionEvent = true
			break
		}
	}
	if !hasExceptionEvent {
		t.Errorf("expected exception event on error span, got events: %+v", span.Events)
	}
}

func TestTracedTokenProviderCacheHit(t *testing.T) {
	ctx := context.Background()
	te := testutil.NewOpenTelemetryTestExporter()
	t.Cleanup(func() {
		te.Unregister(ctx)
	})
	t.Setenv("GO_STORAGE_DEV_OTEL_TRACING", "true")

	cachedTok := &auth.Token{Value: "cached-token"}
	provider := &staticAuthTokenProvider{token: cachedTok}
	creds := wrapAuthCredentialsForTracing(&auth.Credentials{
		TokenProvider: provider,
	})
	parentCtx, _ := startSpan(ctx, "Object.Writer")
	t.Cleanup(func() { endSpan(parentCtx, nil) })

	// First fetch obtains a new token -> emits 1 span.
	if _, err := creds.Token(parentCtx); err != nil {
		t.Fatalf("first creds.Token: %v", err)
	}
	const wantSpanName = "cloud.google.com/go/storage.Auth.RefreshAccessToken"
	if got := len(findSpansByName(te.Spans(), wantSpanName)); got != 1 {
		t.Fatalf("after first fetch: expected 1 %q span, got %d", wantSpanName, got)
	}

	// Second fetch returns the same *auth.Token pointer (cache hit) -> no new span.
	if _, err := creds.Token(parentCtx); err != nil {
		t.Fatalf("second creds.Token: %v", err)
	}
	if got := len(findSpansByName(te.Spans(), wantSpanName)); got != 1 {
		t.Fatalf("after cache hit: expected 1 %q span, got %d", wantSpanName, got)
	}

	// Third fetch returns a newly refreshed *auth.Token pointer -> emits a 2nd span.
	provider.token = &auth.Token{Value: "refreshed-token"}
	if _, err := creds.Token(parentCtx); err != nil {
		t.Fatalf("third creds.Token: %v", err)
	}
	if got := len(findSpansByName(te.Spans(), wantSpanName)); got != 2 {
		t.Fatalf("after refresh: expected 2 %q spans, got %d", wantSpanName, got)
	}
}

func TestHTTPClientAuthTracingWithMetrics(t *testing.T) {
	ctx := context.Background()
	te := testutil.NewOpenTelemetryTestExporter()
	t.Cleanup(func() {
		te.Unregister(ctx)
	})
	t.Setenv("GO_STORAGE_DEV_OTEL_TRACING", "true")
	t.Setenv("STORAGE_EMULATOR_HOST", "")

	mr := sdkmetric.NewManualReader()
	meterProvider := sdkmetric.NewMeterProvider(sdkmetric.WithReader(mr))
	t.Cleanup(func() {
		_ = meterProvider.Shutdown(ctx)
	})

	baseProvider := &staticAuthTokenProvider{token: &auth.Token{Value: "fake-token"}}
	baseCreds := &auth.Credentials{TokenProvider: baseProvider}
	client, err := NewClient(ctx,
		option.WithAuthCredentials(baseCreds),
		withOtelMetrics(),
		withOtelDebugMetrics(),
		withMeterProvider(meterProvider),
	)
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}
	t.Cleanup(func() {
		_ = client.Close()
	})

	// Caller-owned credentials must not be mutated by client construction.
	if baseCreds.TokenProvider != auth.TokenProvider(baseProvider) {
		t.Errorf("caller-owned credentials were mutated: got TokenProvider %T, want %T", baseCreds.TokenProvider, baseProvider)
	}

	var hc *httpStorageClient
	switch tc := client.tc.(type) {
	case *metricsStorageClient:
		hc = tc.storageClient.(*httpStorageClient)
	case *httpStorageClient:
		hc = tc
	default:
		t.Fatalf("unexpected storageClient type: %T", client.tc)
	}
	if hc.creds == baseCreds {
		t.Errorf("httpStorageClient.creds aliases caller-owned credentials; want a clone")
	}
	// The HTTP transport built in NewClient must observe the wrapped provider.
	if client.creds != hc.creds {
		t.Errorf("NewClient creds (%p) and httpStorageClient creds (%p) must be the same clone", client.creds, hc.creds)
	}
	parentCtx, _ := startSpan(ctx, "Bucket.Attrs")
	_, err = hc.creds.Token(parentCtx)
	endSpan(parentCtx, nil)
	if err != nil {
		t.Fatalf("hc.creds.Token: %v", err)
	}

	const wantSpanName = "cloud.google.com/go/storage.Auth.RefreshAccessToken"
	authSpans := findSpansByName(te.Spans(), wantSpanName)
	if len(authSpans) != 1 {
		t.Fatalf("expected exactly 1 %q span per fetch with metrics+tracing enabled, got %d", wantSpanName, len(authSpans))
	}
}

func TestTracedTokenProviderDevTracingDisabled(t *testing.T) {
	ctx := context.Background()
	te := testutil.NewOpenTelemetryTestExporter()
	t.Cleanup(func() {
		te.Unregister(ctx)
	})
	t.Setenv("GO_STORAGE_DEV_OTEL_TRACING", "false")
	t.Setenv("STORAGE_EMULATOR_HOST", "")

	baseCreds := &auth.Credentials{
		TokenProvider: &staticAuthTokenProvider{token: &auth.Token{Value: "fake-token"}},
	}
	sc, err := newHTTPStorageClient(ctx, withClientOptions(
		option.WithAuthCredentials(baseCreds),
	))
	if err != nil {
		t.Fatalf("newHTTPStorageClient: %v", err)
	}
	t.Cleanup(func() {
		_ = sc.Close()
	})

	hc := sc.(*httpStorageClient)
	tok, err := hc.creds.Token(ctx)
	if err != nil {
		t.Fatalf("hc.creds.Token: %v", err)
	}
	if tok.Value != "fake-token" {
		t.Fatalf("got access token %q, want %q", tok.Value, "fake-token")
	}

	const wantSpanName = "cloud.google.com/go/storage.Auth.RefreshAccessToken"
	if got := len(findSpansByName(te.Spans(), wantSpanName)); got > 0 {
		t.Fatalf("expected 0 %q spans when dev tracing is disabled, got %d", wantSpanName, got)
	}
}

func TestTracedTokenProviderRequiresRecordingSpan(t *testing.T) {
	ctx := context.Background()
	te := testutil.NewOpenTelemetryTestExporter()
	t.Cleanup(func() {
		te.Unregister(ctx)
	})
	t.Setenv("GO_STORAGE_DEV_OTEL_TRACING", "true")

	provider := &staticAuthTokenProvider{token: &auth.Token{Value: "first-token"}}
	creds := wrapAuthCredentialsForTracing(&auth.Credentials{
		TokenProvider: provider,
	})
	const wantSpanName = "cloud.google.com/go/storage.Auth.RefreshAccessToken"

	// A fetch without a recording span in ctx, such as the auth library's
	// background refresh of a stale token, emits no span.
	if _, err := creds.Token(ctx); err != nil {
		t.Fatalf("creds.Token without span: %v", err)
	}
	if got := len(findSpansByName(te.Spans(), wantSpanName)); got != 0 {
		t.Fatalf("without a recording span: expected 0 %q spans, got %d", wantSpanName, got)
	}

	parentCtx, parentSpan := startSpan(ctx, "Object.Reader")
	t.Cleanup(func() { endSpan(parentCtx, nil) })

	// The token obtained above was still tracked, so returning it again
	// under a recording span is a cache hit and emits no span.
	if _, err := creds.Token(parentCtx); err != nil {
		t.Fatalf("creds.Token cache hit: %v", err)
	}
	if got := len(findSpansByName(te.Spans(), wantSpanName)); got != 0 {
		t.Fatalf("after cache hit: expected 0 %q spans, got %d", wantSpanName, got)
	}

	// A new token under a recording span emits a child span.
	provider.token = &auth.Token{Value: "second-token"}
	if _, err := creds.Token(parentCtx); err != nil {
		t.Fatalf("creds.Token with span: %v", err)
	}
	authSpans := findSpansByName(te.Spans(), wantSpanName)
	if len(authSpans) != 1 {
		t.Fatalf("with a recording span: expected 1 %q span, got %d", wantSpanName, len(authSpans))
	}
	if got, want := authSpans[0].Parent.SpanID(), parentSpan.SpanContext().SpanID(); got != want {
		t.Errorf("got parent span ID %v, want %v", got, want)
	}
}

func TestStorageTraceRPCSystemNameAttribute(t *testing.T) {
	t.Setenv(storageOtelTracingDevVar, "true")
	// Disable the bucket metadata cache so no background fetch shares the
	// mock transport or emits extra spans.
	t.Setenv(storageBucketMetadataDisabledVar, "true")

	const bucket = "my-bucket"
	rangeParams := &newRangeReaderParams{
		bucket: bucket,
		object: "my-object",
	}

	// newHTTPClient returns a client built by NewClient whose requests fail
	// immediately via the mock transport.
	newHTTPClient := func(t *testing.T) *Client {
		return mockClient(t, &mockTransport{})
	}
	// newGRPCClient returns a client built by NewGRPCClient pointing at an
	// unreachable endpoint, so requests fail fast without network access.
	newGRPCClient := func(t *testing.T, ctx context.Context) *Client {
		c, err := NewGRPCClient(ctx,
			option.WithEndpoint("localhost:1"),
			option.WithoutAuthentication(),
			option.WithGRPCDialOption(grpc.WithTransportCredentials(insecure.NewCredentials())),
		)
		if err != nil {
			t.Fatalf("NewGRPCClient: %v", err)
		}
		t.Cleanup(func() { c.Close() })
		return c
	}

	tests := []struct {
		name     string // Test name; defaults to spanName.
		spanName string
		op       func(t *testing.T, ctx context.Context)
		want     string // "" means the attribute must not be set.
	}{
		{
			name:     "HTTP operation span",
			spanName: "Bucket.Attrs",
			op: func(t *testing.T, ctx context.Context) {
				newHTTPClient(t).Bucket(bucket).Retryer(WithPolicy(RetryNever)).Attrs(ctx)
			},
			want: "http",
		},
		{
			name:     "gRPC operation span",
			spanName: "Bucket.Attrs",
			op: func(t *testing.T, ctx context.Context) {
				newGRPCClient(t, ctx).Bucket(bucket).Retryer(WithPolicy(RetryNever)).Attrs(ctx)
			},
			want: "grpc",
		},
		{
			spanName: "httpStorageClient.ObjectsListCall",
			op: func(t *testing.T, ctx context.Context) {
				newHTTPClient(t).Bucket(bucket).Retryer(WithPolicy(RetryNever)).Objects(ctx, nil).Next()
			},
			want: "http",
		},
		{
			spanName: "grpcStorageClient.ObjectsListCall",
			op: func(t *testing.T, ctx context.Context) {
				newGRPCClient(t, ctx).Bucket(bucket).Retryer(WithPolicy(RetryNever)).Objects(ctx, nil).Next()
			},
			want: "grpc",
		},
		{
			spanName: "httpStorageClient.NewRangeReader",
			op: func(t *testing.T, ctx context.Context) {
				hc := &httpStorageClient{
					settings: &settings{},
					config:   &storageConfig{},
				}
				canceledCtx, cancel := context.WithCancel(ctx)
				cancel()
				hc.NewRangeReader(canceledCtx, rangeParams)
			},
			want: "http",
		},
		{
			spanName: "grpcStorageClient.NewRangeReader",
			op: func(t *testing.T, ctx context.Context) {
				gc := &grpcStorageClient{
					settings: &settings{},
					config:   &storageConfig{grpcBidiReads: true},
				}
				canceledCtx, cancel := context.WithCancel(ctx)
				cancel()
				gc.NewRangeReader(canceledCtx, rangeParams)
			},
			want: "grpc",
		},
		{
			spanName: "grpcStorageClient.NewRangeReaderReadObject",
			op: func(t *testing.T, ctx context.Context) {
				gc := &grpcStorageClient{
					settings: &settings{},
					config:   &storageConfig{grpcBidiReads: false},
				}
				canceledCtx, cancel := context.WithCancel(ctx)
				cancel()
				gc.NewRangeReader(canceledCtx, rangeParams)
			},
			want: "grpc",
		},
		{
			name:     "dev tracing disabled omits attribute",
			spanName: "Bucket.Attrs",
			op: func(t *testing.T, ctx context.Context) {
				t.Setenv(storageOtelTracingDevVar, "false")
				newHTTPClient(t).Bucket(bucket).Retryer(WithPolicy(RetryNever)).Attrs(ctx)
			},
			want: "",
		},
		{
			name:     "zero-value rpcSystem omits attribute",
			spanName: "Bucket.Attrs",
			op: func(t *testing.T, ctx context.Context) {
				sCtx, _ := startSpanWithBucket(ctx, &Client{}, bucket, "Bucket.Attrs")
				endSpan(sCtx, nil)
			},
			want: "",
		},
	}

	for _, tc := range tests {
		name := tc.name
		if name == "" {
			name = tc.spanName
		}
		t.Run(name, func(t *testing.T) {
			ctx := context.Background()
			te := testutil.NewOpenTelemetryTestExporter()
			t.Cleanup(func() {
				te.Unregister(ctx)
			})

			tc.op(t, ctx)

			got, ok := spanStringAttribute(t, te.Spans(), tc.spanName, rpcSystemNameKey)
			switch {
			case tc.want == "" && ok:
				t.Errorf("%s: %s = %q, want attribute absent", tc.spanName, rpcSystemNameKey, got)
			case tc.want != "" && got != tc.want:
				t.Errorf("%s: %s = %q (present: %v), want %q", tc.spanName, rpcSystemNameKey, got, ok, tc.want)
			}
		})
	}
}

// spanStringAttribute returns the string value of attribute key on the span
// named spanName and whether the attribute is set. It fails the test if no
// such span was recorded.
func spanStringAttribute(t *testing.T, spans tracetest.SpanStubs, spanName string, key attribute.Key) (string, bool) {
	t.Helper()
	for _, s := range spans {
		if s.Name != appendPackageName(spanName) {
			continue
		}
		for _, attr := range s.Attributes {
			if attr.Key == key {
				return attr.Value.AsString(), true
			}
		}
		return "", false
	}
	t.Fatalf("span %q not found", spanName)
	return "", false
}

func TestRecordUploadID(t *testing.T) {
	const uploadID = "AJjja9aXX4R20FIZ2ddLnYddhJmD9Xm_Y2B_NEu5G2a6PzJiGRgPKLp6hEQpM"
	location := func(query string) http.Header {
		return http.Header{"Location": {"https://storage.googleapis.com/upload/storage/v1/b/my-bucket/o?" + query}}
	}

	tests := []struct {
		name          string
		devTracingOff bool
		record        func(ctx context.Context)
		wantUploadID  string
	}{
		{
			name:         "direct ID",
			record:       func(ctx context.Context) { recordUploadID(ctx, uploadID) },
			wantUploadID: uploadID,
		},
		{
			name:   "empty ID",
			record: func(ctx context.Context) { recordUploadID(ctx, "") },
		},
		{
			name: "header with upload_id",
			record: func(ctx context.Context) {
				recordUploadIDFromHeader(ctx, location("uploadType=resumable&upload_id="+uploadID))
			},
			wantUploadID: uploadID,
		},
		{
			name:   "header without upload_id",
			record: func(ctx context.Context) { recordUploadIDFromHeader(ctx, location("uploadType=resumable")) },
		},
		{
			name:   "unparseable Location",
			record: func(ctx context.Context) { recordUploadIDFromHeader(ctx, http.Header{"Location": {"://bad-url"}}) },
		},
		{
			name:   "nil header",
			record: func(ctx context.Context) { recordUploadIDFromHeader(ctx, nil) },
		},
		{
			name: "trackingTransport records ID from response",
			record: func(ctx context.Context) {
				mock := &mockTransport{}
				mock.addResult(&http.Response{Status: "200 OK", Header: location("upload_id=" + uploadID)}, nil)
				req, err := http.NewRequestWithContext(ctx, "POST", "https://storage.googleapis.com/upload/storage/v1/b/my-bucket/o?uploadType=resumable", nil)
				if err != nil {
					t.Fatalf("NewRequestWithContext: %v", err)
				}
				if _, err := (&trackingTransport{base: mock}).RoundTrip(req); err != nil {
					t.Fatalf("RoundTrip: %v", err)
				}
			},
			wantUploadID: uploadID,
		},
		{
			name:          "dev tracing off",
			devTracingOff: true,
			record:        func(ctx context.Context) { recordUploadID(ctx, uploadID) },
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			ctx := context.Background()
			te := testutil.NewOpenTelemetryTestExporter()
			t.Cleanup(func() {
				te.Unregister(ctx)
			})
			// TODO: Remove setting development env var upon launch.
			t.Setenv(storageOtelTracingDevVar, strconv.FormatBool(!tc.devTracingOff))

			ctx, _ = startSpan(ctx, "Object.Writer")
			tc.record(ctx)
			endSpan(ctx, nil)

			if tc.devTracingOff {
				for _, s := range te.Spans() {
					for _, a := range s.Attributes {
						if a.Key == "gcp.storage.upload.id" {
							t.Errorf("span %s has gcp.storage.upload.id when dev tracing is disabled", s.Name)
						}
					}
				}
				return
			}
			var wantAttrs []attribute.KeyValue
			if tc.wantUploadID != "" {
				wantAttrs = append(wantAttrs, attribute.String("gcp.storage.upload.id", tc.wantUploadID))
			}
			verifySingleSpanAttributes(t, te.Spans(), wantAttrs)
		})
	}
}

// fakeResumableUploadServer is a minimal in-memory Storage gRPC server used to
// exercise gRPCResumableBidiWriteBufferSender.connect.
type fakeResumableUploadServer struct {
	storagepb.UnimplementedStorageServer
	uploadID string
}

func (s *fakeResumableUploadServer) StartResumableWrite(context.Context, *storagepb.StartResumableWriteRequest) (*storagepb.StartResumableWriteResponse, error) {
	return &storagepb.StartResumableWriteResponse{UploadId: s.uploadID}, nil
}

func (s *fakeResumableUploadServer) QueryWriteStatus(context.Context, *storagepb.QueryWriteStatusRequest) (*storagepb.QueryWriteStatusResponse, error) {
	return &storagepb.QueryWriteStatusResponse{
		WriteStatus: &storagepb.QueryWriteStatusResponse_PersistedSize{PersistedSize: 0},
	}, nil
}

func (s *fakeResumableUploadServer) BidiWriteObject(stream storagepb.Storage_BidiWriteObjectServer) error {
	for {
		if _, err := stream.Recv(); err != nil {
			if err == io.EOF {
				return nil
			}
			return err
		}
	}
}

// newFakeResumableUploadGAPICClient starts fake on an in-memory listener and
// returns a GAPIC client connected to it.
func newFakeResumableUploadGAPICClient(ctx context.Context, t *testing.T, fake *fakeResumableUploadServer) *gapic.Client {
	t.Helper()
	lis := bufconn.Listen(1 << 20)
	srv := grpc.NewServer()
	storagepb.RegisterStorageServer(srv, fake)
	go srv.Serve(lis)
	t.Cleanup(srv.Stop)

	conn, err := grpc.NewClient("passthrough:///bufnet",
		grpc.WithContextDialer(func(ctx context.Context, _ string) (net.Conn, error) {
			return lis.DialContext(ctx)
		}),
		grpc.WithTransportCredentials(insecure.NewCredentials()),
	)
	if err != nil {
		t.Fatalf("grpc.NewClient: %v", err)
	}
	raw, err := gapic.NewClient(ctx, option.WithGRPCConn(conn))
	if err != nil {
		t.Fatalf("gapic.NewClient: %v", err)
	}
	t.Cleanup(func() { raw.Close() })
	return raw
}

func TestGRPCResumableBidiWriteBufferSenderConnectUploadID(t *testing.T) {
	const sampleUploadID = "AJjja9aXX4R20FIZ2ddLnYddhJmD9Xm_Y2B_NEu5G2a6PzJiGRgPKLp6hEQpM"
	spanName := "Object.Writer"

	tests := []struct {
		name   string
		sender func(raw *gapic.Client) *gRPCResumableBidiWriteBufferSender
	}{
		{
			name: "New upload records upload.id from StartResumableWrite",
			sender: func(raw *gapic.Client) *gRPCResumableBidiWriteBufferSender {
				return &gRPCResumableBidiWriteBufferSender{
					raw:               raw,
					bucket:            "my-bucket",
					startWriteRequest: &storagepb.StartResumableWriteRequest{},
				}
			},
		},
		{
			name: "Resumed upload records existing upload.id",
			sender: func(raw *gapic.Client) *gRPCResumableBidiWriteBufferSender {
				return &gRPCResumableBidiWriteBufferSender{
					raw:    raw,
					bucket: "my-bucket",
					upid:   sampleUploadID,
				}
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			ctx := context.Background()
			te := testutil.NewOpenTelemetryTestExporter()
			t.Cleanup(func() {
				te.Unregister(ctx)
			})

			// TODO: Remove setting development env var upon launch.
			t.Setenv(storageOtelTracingDevVar, "true")

			raw := newFakeResumableUploadGAPICClient(ctx, t, &fakeResumableUploadServer{uploadID: sampleUploadID})
			s := tc.sender(raw)

			requests := make(chan gRPCBidiWriteRequest)
			completions := make(chan gRPCBidiWriteCompletion, 1)
			cs := gRPCBufSenderChans{
				requests:    requests,
				requestAcks: make(chan struct{}, 1),
				completions: completions,
			}

			ctx, _ = startSpan(ctx, spanName)
			s.connect(ctx, cs)
			// Close the request channel so the stream shuts down, then drain
			// completions until connect's goroutines close the channel.
			close(requests)
			for range completions {
			}
			if err := s.err(); err != nil {
				t.Fatalf("connect: %v", err)
			}
			endSpan(ctx, nil)

			spans := te.Spans()
			if len(spans) != 1 {
				t.Fatalf("expected one span, got %d", len(spans))
			}
			wantSpan := createWantSpanStub(spanName, append(getCommonAttributes(), attribute.String("gcp.storage.upload.id", sampleUploadID)))
			opts := []cmp.Option{
				cmp.Comparer(spanAttributesComparer),
			}
			if diff := testutil.Diff(spans[0], wantSpan, opts...); diff != "" {
				t.Errorf("diff: -got, +want:\n%s\n", diff)
			}
		})
	}
}

// contextStream is a grpc.ClientStream that only provides a context, which is
// all recordReadPayload uses.
type contextStream struct {
	grpc.ClientStream
	ctx context.Context
}

func (s contextStream) Context() context.Context { return s.ctx }

// TestAttemptStatsHandlerPayload feeds the messages of one gRPC attempt to
// attemptStatsHandler and recordReadPayload and checks the attempt span.
func TestAttemptStatsHandlerPayload(t *testing.T) {
	type wreq = storagepb.BidiWriteObjectRequest
	data := func(n int) *storagepb.BidiWriteObjectRequest_ChecksummedData {
		return &storagepb.BidiWriteObjectRequest_ChecksummedData{ChecksummedData: &storagepb.ChecksummedData{Content: make([]byte, n)}}
	}
	bidi := func(offsets ...int64) *storagepb.BidiReadObjectRequest {
		req := &storagepb.BidiReadObjectRequest{}
		for _, o := range offsets {
			req.ReadRanges = append(req.ReadRanges, &storagepb.ReadRange{ReadOffset: o})
		}
		return req
	}
	for _, tc := range []struct {
		name string
		msgs []any    // Requests sent, or int64 object bytes decoded from a response.
		want [2]int64 // (offset, size), -1 if absent.
	}{
		{"write", []any{&wreq{Data: data(100)}, &wreq{WriteOffset: 100, Data: data(50), FinishWrite: true}}, [2]int64{0, 150}},
		{"write starting mid-object", []any{&wreq{WriteOffset: 262144, Data: data(262144), Flush: true}}, [2]int64{262144, 262144}},
		{"write message without data is ignored", []any{&wreq{}, &wreq{WriteOffset: 10, Data: data(5)}}, [2]int64{10, 5}},
		{"write flush without data", []any{&wreq{WriteOffset: 7, Flush: true}}, [2]int64{7, 0}},
		{"write empty object", []any{&wreq{FinishWrite: true}}, [2]int64{0, 0}},
		{"write without data", []any{&wreq{}}, [2]int64{-1, -1}},
		{"read", []any{&storagepb.ReadObjectRequest{ReadOffset: 10}, int64(100), int64(50)}, [2]int64{10, 150}},
		{"bidi read uses lowest range offset", []any{bidi(40, 20), int64(30)}, [2]int64{20, 30}},
		{"bidi request without ranges is ignored", []any{bidi(), bidi(5), int64(10)}, [2]int64{5, 10}},
		{"response without object data is ignored", []any{int64(0), bidi(5)}, [2]int64{5, 0}},
		// A multi-range response can be decoded before HandleRPC sees its
		// request, which is sent on another goroutine.
		{"bytes decoded before the request", []any{int64(20), bidi(100)}, [2]int64{100, 20}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ctx := context.Background()
			te := testutil.NewOpenTelemetryTestExporter()
			t.Cleanup(func() { te.Unregister(ctx) })
			ctx, span := tracer().Start(ctx, "attempt")
			ctx = context.WithValue(ctx, payloadAttemptKey{}, &payloadAttempt{span: span})
			for _, m := range tc.msgs {
				if n, ok := m.(int64); ok {
					recordReadPayload(contextStream{ctx: ctx}, n)
				} else {
					attemptStatsHandler{}.HandleRPC(ctx, &stats.OutPayload{Client: true, Payload: m})
				}
			}
			span.End()
			if got := payloadAttrs(te.Spans()[0].Attributes); got != tc.want {
				t.Errorf("got (offset, size) %v, want %v", got, tc.want)
			}
		})
	}
}

func TestRecordHTTPReadPayload(t *testing.T) {
	t.Setenv(storageOtelTracingDevVar, "true")
	for _, tc := range []struct {
		name           string
		offset, length int64
		status         int
		contentLength  int64
		noAttemptSpan  bool     // The response carries the operation span.
		want           [2]int64 // Attempt span (offset, size), -1 if absent.
	}{
		{"range", 10, 20, 206, 20, false, [2]int64{10, 20}},
		{"suffix range", -15, -1, 206, 15, false, [2]int64{-15, 15}},
		// The response describes the whole object, which is not read.
		{"zero-length read", 10, 0, 200, 100, false, [2]int64{10, 0}},
		// Failed attempts serve no object bytes, as for gRPC.
		{"error response", 10, 20, 503, 52, false, [2]int64{10, 0}},
		{"unknown length", 0, -1, 200, -1, false, [2]int64{0, -1}},
		{"operation span is not tagged", 10, 20, 206, 20, true, [2]int64{-1, -1}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ctx := context.Background()
			te := testutil.NewOpenTelemetryTestExporter()
			t.Cleanup(func() { te.Unregister(ctx) })
			opCtx, opSpan := tracer().Start(ctx, "operation")
			respCtx, attSpan := tracer().Start(opCtx, "attempt")
			if tc.noAttemptSpan {
				respCtx = opCtx
			}
			req, _ := http.NewRequestWithContext(respCtx, "GET", "https://example.com/bucket/object", nil)
			recordHTTPReadPayload(opCtx, &http.Response{StatusCode: tc.status, ContentLength: tc.contentLength, Request: req}, tc.offset, tc.length)
			attSpan.End()
			opSpan.End()
			for _, s := range te.Spans() {
				want := [2]int64{-1, -1}
				if s.Name == "attempt" {
					want = tc.want
				}
				if got := payloadAttrs(s.Attributes); got != want {
					t.Errorf("%s span: got (offset, size) %v, want %v", s.Name, got, want)
				}
			}
		})
	}
}

func TestParseUploadContentRangeOffset(t *testing.T) {
	for _, tc := range []struct {
		cr     string
		offset int64
		ok     bool
	}{
		{"bytes 0-9/100", 0, true},
		{"bytes 10-19/*", 10, true},
		{"bytes */100", 100, true},
		{"bytes */*", 0, false},
		{"bytes 100/200", 0, false},
		{"bytes */-5", 0, false},
		{"garbage", 0, false},
	} {
		if offset, ok := parseUploadContentRangeOffset(tc.cr); offset != tc.offset || ok != tc.ok {
			t.Errorf("parseUploadContentRangeOffset(%q) = (%d, %v), want (%d, %v)", tc.cr, offset, ok, tc.offset, tc.ok)
		}
	}
}

// TestPayloadTracing checks the payload attributes of the transport attempt
// spans of uploads and reads, over gRPC against a fake server and over HTTP
// with canned responses through a real otelhttp transport.
func TestPayloadTracing(t *testing.T) {
	const chunk = googleapi.MinUploadChunkSize // 256 KiB.
	ctx := context.Background()

	grpcClient := func(srv *retryAttemptTestServer, opts ...option.ClientOption) func(*testing.T) *Client {
		return func(t *testing.T) *Client {
			startFakeStorageServer(t, srv)
			c, err := NewGRPCClient(ctx, opts...)
			if err != nil {
				t.Fatalf("NewGRPCClient: %v", err)
			}
			if srv.failures > 0 {
				// Fast backoff for retried cases. SetRetry replaces the whole
				// retry config, resetting the policy to the default.
				c.SetRetry(WithBackoff(gax.Backoff{Initial: time.Millisecond, Max: time.Millisecond}))
			}
			return c
		}
	}
	httpClient := func(opts []option.ClientOption, responses ...*http.Response) func(*testing.T) *Client {
		return func(t *testing.T) *Client {
			mock := &mockTransport{}
			for _, r := range responses {
				mock.addResult(r, nil)
			}
			c, err := NewClient(ctx, append(opts, option.WithHTTPClient(&http.Client{Transport: otelhttp.NewTransport(mock)}))...)
			if err != nil {
				t.Fatalf("NewClient: %v", err)
			}
			return c
		}
	}
	object := func(size int) *http.Response {
		return &http.Response{StatusCode: 200, Header: http.Header{"Content-Type": {"application/json"}}, Body: bodyReader(fmt.Sprintf(`{"bucket":"bucket","name":"object","size":"%d"}`, size))}
	}
	session := func() *http.Response {
		return &http.Response{StatusCode: 200, Header: http.Header{"Location": {"https://example.com/upload?uploadType=resumable&upload_id=abc"}}, Body: bodyReader("")}
	}
	incomplete := func() *http.Response {
		return &http.Response{StatusCode: 200, Header: http.Header{"X-Http-Status-Code-Override": {"308"}}, Body: bodyReader("")}
	}
	rangeResp := func(start, n int64, body io.ReadCloser) *http.Response {
		return &http.Response{StatusCode: 206, Header: http.Header{"Content-Range": {fmt.Sprintf("bytes %d-%d/100", start, start+n-1)}}, ContentLength: n, Body: body}
	}

	write := func(chunkSize, size int) func(*testing.T, *Client) {
		return func(t *testing.T, c *Client) {
			w := c.Bucket("bucket").Object("object").NewWriter(ctx)
			w.ChunkSize = chunkSize
			w.DisableAutoChecksum = true // The HTTP responses carry no checksum.
			if _, err := w.Write(make([]byte, size)); err != nil {
				t.Fatalf("Write: %v", err)
			}
			if err := w.Close(); err != nil {
				t.Fatalf("Close: %v", err)
			}
		}
	}
	read := func(t *testing.T, c *Client) {
		r, err := c.Bucket("bucket").Object("object").NewRangeReader(ctx, 10, 20)
		if err != nil {
			t.Fatalf("NewRangeReader: %v", err)
		}
		defer r.Close()
		if n, err := io.Copy(io.Discard, r); err != nil || n != 20 {
			t.Fatalf("read %d bytes, err %v; want 20 bytes", n, err)
		}
	}
	multiRangeRead := func(t *testing.T, c *Client) {
		mrd, err := c.Bucket("bucket").Object("object").NewMultiRangeDownloader(ctx)
		if err != nil {
			t.Fatalf("NewMultiRangeDownloader: %v", err)
		}
		var buf1, buf2 bytes.Buffer
		mrd.Add(&buf1, 0, 10, func(int64, int64, error) {})
		mrd.Add(&buf2, 20, 10, func(int64, int64, error) {})
		mrd.Wait()
		if err := mrd.Close(); err != nil {
			t.Fatalf("mrd.Close: %v", err)
		}
	}

	for _, tc := range []struct {
		name   string
		client func(*testing.T) *Client
		run    func(*testing.T, *Client)
		want   [][2]int64 // (offset, size) of each attempt span, sorted, -1 if absent.
	}{
		{"grpc/oneshot write", grpcClient(&retryAttemptTestServer{}), write(0, 1000), [][2]int64{{0, 1000}}},
		// The StartResumableWrite attempt is a unary RPC and carries no payload
		// attributes; both chunks and the final request are sent on one stream.
		{"grpc/resumable write", grpcClient(&retryAttemptTestServer{}), write(chunk, 2*chunk), [][2]int64{{-1, -1}, {0, 2 * chunk}}},
		// The response also carries the object metadata, which is not counted.
		{"grpc/range read", grpcClient(&retryAttemptTestServer{}), read, [][2]int64{{10, 20}}},
		{"grpc/bidi range read", grpcClient(&retryAttemptTestServer{}, WithGRPCBidiReads()), read, [][2]int64{{10, 20}}},
		// The failed attempt sent its request but received no object bytes.
		// The server's failure count is shared with GetBucket, which the
		// disabled bucket metadata fetch would otherwise call first.
		{"grpc/range read retried", grpcClient(&retryAttemptTestServer{failures: 1}), read, [][2]int64{{10, 0}, {10, 20}}},
		// Only the object data of the two ranges is counted.
		{"grpc/multi-range read", grpcClient(&retryAttemptTestServer{}, WithGRPCBidiReads()), multiRangeRead, [][2]int64{{0, 20}}},
		{"http/oneshot write", httpClient(nil, object(1000)), write(0, 1000), [][2]int64{{0, 1000}}},
		// An object smaller than ChunkSize is buffered and sent in one request.
		{"http/buffered single-request write", httpClient(nil, object(100)), write(chunk, 100), [][2]int64{{0, 100}}},
		// The session start carries no object bytes, and the empty final
		// request ("bytes */N") starts at the object size.
		{"http/resumable write", httpClient(nil, session(), incomplete(), incomplete(), object(2*chunk)), write(chunk, 2*chunk),
			[][2]int64{{-1, -1}, {0, chunk}, {chunk, chunk}, {2 * chunk, 0}}},
		{"http/XML range read", httpClient(nil, rangeResp(10, 20, bodyReader(strings.Repeat("a", 20)))), read, [][2]int64{{10, 20}}},
		{"http/JSON range read", httpClient([]option.ClientOption{WithJSONReads()}, rangeResp(10, 20, bodyReader(strings.Repeat("a", 20)))), read, [][2]int64{{10, 20}}},
		// The read is reopened where the broken body stopped, after the
		// NewRangeReader span ended; the first response still reports its
		// full Content-Length.
		{"http/range read reopened mid-body", httpClient(nil,
			rangeResp(10, 20, io.NopCloser(io.MultiReader(bytes.NewReader(make([]byte, 5)), iotest.ErrReader(io.ErrUnexpectedEOF)))),
			rangeResp(15, 15, bodyReader(strings.Repeat("a", 15)))), read, [][2]int64{{10, 20}, {15, 15}}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			te := testutil.NewOpenTelemetryTestExporter()
			t.Cleanup(func() { te.Unregister(ctx) })
			t.Setenv(storageOtelTracingDevVar, "true")
			t.Setenv(storageBucketMetadataDisabledVar, "true")
			c := tc.client(t)
			t.Cleanup(func() { c.Close() })
			tc.run(t, c)

			var got [][2]int64
			for _, s := range te.Spans() {
				// Attempt spans come from otelgrpc or otelhttp; the storage
				// library's own spans never carry payload attributes.
				if s.InstrumentationScope.Name != defaultTracerName {
					got = append(got, payloadAttrs(s.Attributes))
				} else if attrs := payloadAttrs(s.Attributes); attrs != [2]int64{-1, -1} {
					t.Errorf("storage span %s has payload attributes %v", s.Name, attrs)
				}
			}
			// Spans are exported as they end, not in request order.
			slices.SortFunc(got, func(a, b [2]int64) int { return slices.Compare(a[:], b[:]) })
			if diff := cmp.Diff(tc.want, got); diff != "" {
				t.Errorf("attempt span (offset, size) mismatch (-want +got):\n%s", diff)
			}
		})
	}
}

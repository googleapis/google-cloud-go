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
	"fmt"
	"net/http"
	"testing"
	"time"

	"cloud.google.com/go/internal/testutil"
	"cloud.google.com/go/storage/internal"
	"github.com/google/go-cmp/cmp"
	gax "github.com/googleapis/gax-go/v2"
	"go.opentelemetry.io/otel/attribute"
	otcodes "go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/sdk/instrumentation"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
	"go.opentelemetry.io/otel/trace"
	"google.golang.org/api/googleapi"
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

// retryAttemptNumber returns the value of the retry attempt number attribute
// in attrs, or 0 if it is not present.
func retryAttemptNumber(attrs []attribute.KeyValue) int64 {
	for _, a := range attrs {
		if a.Key == "gcp.client.retry.attempt_number" {
			return a.Value.AsInt64()
		}
	}
	return 0
}

// checkRetryBackoffs checks that the single Bucket.Attrs span in spans has
// one RetryBackoff child span per entry in wantAttempts, carrying that retry
// attempt number and lying within the parent span's time range.
func checkRetryBackoffs(t *testing.T, spans tracetest.SpanStubs, wantAttempts []int64) {
	t.Helper()
	parents := findSpans(spans, "Bucket.Attrs")
	if len(parents) != 1 {
		t.Fatalf("got %d Bucket.Attrs spans, want 1", len(parents))
	}
	parent := parents[0]
	var spanAttempts []int64
	for _, s := range findSpans(spans, "RetryBackoff") {
		if got, want := s.Parent.SpanID(), parent.SpanContext.SpanID(); got != want {
			t.Errorf("RetryBackoff span parent ID = %v, want %v", got, want)
		}
		if s.StartTime.After(s.EndTime) {
			t.Errorf("RetryBackoff span start time %v is after end time %v", s.StartTime, s.EndTime)
		}
		if s.StartTime.Before(parent.StartTime) || s.EndTime.After(parent.EndTime) {
			t.Errorf("RetryBackoff span [%v, %v] is outside parent span [%v, %v]", s.StartTime, s.EndTime, parent.StartTime, parent.EndTime)
		}
		spanAttempts = append(spanAttempts, retryAttemptNumber(s.Attributes))
	}
	if diff := cmp.Diff(wantAttempts, spanAttempts); diff != "" {
		t.Errorf("RetryBackoff span attempt numbers mismatch (-want +got):\n%s", diff)
	}
}

func TestRecordRetryBackoffNoParentSpan(t *testing.T) {
	ctx := context.Background()
	te := testutil.NewOpenTelemetryTestExporter()
	t.Cleanup(func() {
		te.Unregister(ctx)
	})
	t.Setenv("GO_STORAGE_DEV_OTEL_TRACING", "true")

	recordRetryBackoff(ctx, 1, time.Now())

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
		// wantAttempts are the attempt numbers of the recorded backoffs.
		wantAttempts []int64
	}{
		{
			name:  "success on first attempt",
			retry: &retryConfig{backoff: fastBackoff},
		},
		{
			name:         "one retry",
			retry:        &retryConfig{backoff: fastBackoff},
			errs:         []error{retryableErr},
			wantAttempts: []int64{1},
		},
		{
			name:         "two retries",
			retry:        &retryConfig{backoff: fastBackoff},
			errs:         []error{retryableErr, retryableErr},
			wantAttempts: []int64{1, 2},
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
			wantAttempts: []int64{1},
		},
		{
			name: "context done during backoff",
			// gax picks a random pause in [1ns, Initial), so a huge Initial makes
			// a pause shorter than the timeout practically impossible.
			retry:        &retryConfig{backoff: &gax.Backoff{Initial: 1000 * time.Hour, Max: 1000 * time.Hour}},
			errs:         []error{retryableErr},
			timeout:      50 * time.Millisecond,
			wantErr:      true,
			wantAttempts: []int64{1},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ctx := context.Background()
			te := testutil.NewOpenTelemetryTestExporter()
			t.Cleanup(func() {
				te.Unregister(ctx)
			})
			t.Setenv("GO_STORAGE_DEV_OTEL_TRACING", "true")

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
			checkRetryBackoffs(t, te.Spans(), tc.wantAttempts)
		})
	}
}

func TestRunRetryBackoffTracingDisabled(t *testing.T) {
	ctx := context.Background()
	te := testutil.NewOpenTelemetryTestExporter()
	t.Cleanup(func() {
		te.Unregister(ctx)
	})
	t.Setenv("GO_STORAGE_DEV_OTEL_TRACING", "false")

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

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

	"cloud.google.com/go/auth"
	"cloud.google.com/go/internal/testutil"
	"cloud.google.com/go/storage/internal"
	"github.com/google/go-cmp/cmp"
	"go.opentelemetry.io/otel/attribute"
	otcodes "go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/sdk/instrumentation"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
	"go.opentelemetry.io/otel/trace"
	"google.golang.org/api/googleapi"
	"google.golang.org/api/option"
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

	_, err := creds.Token(ctx)
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

	// First fetch obtains a new token -> emits 1 span.
	if _, err := creds.Token(ctx); err != nil {
		t.Fatalf("first creds.Token: %v", err)
	}
	const wantSpanName = "cloud.google.com/go/storage.Auth.RefreshAccessToken"
	if got := len(findSpansByName(te.Spans(), wantSpanName)); got != 1 {
		t.Fatalf("after first fetch: expected 1 %q span, got %d", wantSpanName, got)
	}

	// Second fetch returns the same *auth.Token pointer (cache hit) -> no new span.
	if _, err := creds.Token(ctx); err != nil {
		t.Fatalf("second creds.Token: %v", err)
	}
	if got := len(findSpansByName(te.Spans(), wantSpanName)); got != 1 {
		t.Fatalf("after cache hit: expected 1 %q span, got %d", wantSpanName, got)
	}

	// Third fetch returns a newly refreshed *auth.Token pointer -> emits a 2nd span.
	provider.token = &auth.Token{Value: "refreshed-token"}
	if _, err := creds.Token(ctx); err != nil {
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

	mr := sdkmetric.NewManualReader()
	meterProvider := sdkmetric.NewMeterProvider(sdkmetric.WithReader(mr))
	t.Cleanup(func() {
		_ = meterProvider.Shutdown(ctx)
	})

	baseCreds := &auth.Credentials{
		TokenProvider: &staticAuthTokenProvider{token: &auth.Token{Value: "fake-token"}},
	}
	sc, err := newHTTPStorageClient(ctx, withClientOptions(
		option.WithAuthCredentials(baseCreds),
		withOtelMetrics(),
		withOtelDebugMetrics(),
		withMeterProvider(meterProvider),
	))
	if err != nil {
		t.Fatalf("newHTTPStorageClient: %v", err)
	}
	t.Cleanup(func() {
		_ = sc.Close()
	})

	hc := sc.(*httpStorageClient)
	if _, err := hc.creds.Token(ctx); err != nil {
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

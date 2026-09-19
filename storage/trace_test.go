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
	"strings"
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
	"golang.org/x/oauth2"
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
			wantResource: "projects/_/buckets/bucket-miss",
			wantLocation: "global",
			verifyCache:  true,
		},
		{
			name:   "Cache Hit (Resolved)",
			bucket: "bucket-hit",
			setupCache: func(c *bucketMetadataCache) {
				c.put("bucket-hit", bucketMetadata{resource: "projects/p1/buckets/bucket-hit", location: "us-west1"})
			},
			wantResource: "projects/p1/buckets/bucket-hit",
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

func TestRecordWriterTraceAttributes(t *testing.T) {
	testCases := []struct {
		name      string
		writer    *Writer
		wantAttrs map[string]interface{}
	}{
		{
			name: "resumable",
			writer: &Writer{
				ChunkSize:            256 * 1024,
				Append:               false,
				EnableParallelUpload: false,
				ObjectAttrs:          ObjectAttrs{Name: "test-file.txt"},
			},
			wantAttrs: map[string]interface{}{
				"storage.write.mode":       "resumable",
				"storage.write.chunk_size": int64(256 * 1024),
				"storage.write.append":     false,
				"storage.write.parallel":   false,
				"storage.object.name":      "test-file.txt",
			},
		},
		{
			name: "oneshot",
			writer: &Writer{
				ChunkSize:            0,
				Append:               false,
				EnableParallelUpload: false,
				ObjectAttrs:          ObjectAttrs{Name: "test-oneshot.txt"},
			},
			wantAttrs: map[string]interface{}{
				"storage.write.mode":       "oneshot",
				"storage.write.chunk_size": int64(0),
				"storage.write.append":     false,
				"storage.write.parallel":   false,
				"storage.object.name":      "test-oneshot.txt",
			},
		},
		{
			name: "appendable",
			writer: &Writer{
				ChunkSize:            256 * 1024,
				Append:               true,
				EnableParallelUpload: false,
				ObjectAttrs:          ObjectAttrs{Name: "test-append.txt"},
			},
			wantAttrs: map[string]interface{}{
				"storage.write.mode":       "appendable",
				"storage.write.chunk_size": int64(256 * 1024),
				"storage.write.append":     true,
				"storage.write.parallel":   false,
				"storage.object.name":      "test-append.txt",
			},
		},
		{
			name: "parallel_composite",
			writer: &Writer{
				ChunkSize:            256 * 1024,
				Append:               false,
				EnableParallelUpload: true,
				ParallelUploadConfig: ParallelUploadConfig{
					PartSize:       16 * 1024 * 1024,
					MaxConcurrency: 4,
				},
				ObjectAttrs: ObjectAttrs{Name: "test-parallel.txt"},
			},
			wantAttrs: map[string]interface{}{
				"storage.write.mode":            "parallel_composite",
				"storage.write.chunk_size":      int64(256 * 1024),
				"storage.write.append":          false,
				"storage.write.parallel":        true,
				"storage.object.name":           "test-parallel.txt",
				"storage.write.pcu_part_size":   int64(16 * 1024 * 1024),
				"storage.write.pcu_concurrency": int64(4),
			},
		},
		{
			name: "pcu_part_writer",
			writer: &Writer{
				ChunkSize:            256 * 1024,
				Append:               false,
				EnableParallelUpload: false,
				ObjectAttrs:          ObjectAttrs{Name: "gcs-go-sdk-pu-tmp/part-1"},
			},
			wantAttrs: map[string]interface{}{
				"storage.write.mode":       "pcu_part_writer",
				"storage.write.chunk_size": int64(256 * 1024),
				"storage.write.append":     false,
				"storage.write.parallel":   false,
				"storage.object.name":      "gcs-go-sdk-pu-tmp/part-1",
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
			t.Setenv("GO_STORAGE_DEV_OTEL_TRACING", "true")

			spanName := "Object.Writer"
			ctx, _ = startSpan(ctx, spanName)
			recordWriterTraceAttributes(ctx, tc.writer)
			endSpan(ctx, nil)

			spans := te.Spans()
			if len(spans) != 1 {
				t.Fatalf("expected 1 span, got %d", len(spans))
			}
			gotSpan := spans[0]

			for k, wantVal := range tc.wantAttrs {
				found := false
				for _, a := range gotSpan.Attributes {
					if string(a.Key) == k {
						found = true
						switch v := wantVal.(type) {
						case string:
							if got := a.Value.AsString(); got != v {
								t.Errorf("key %q: got %v, want %v", k, got, v)
							}
						case int64:
							if got := a.Value.AsInt64(); got != v {
								t.Errorf("key %q: got %v, want %v", k, got, v)
							}
						case bool:
							if got := a.Value.AsBool(); got != v {
								t.Errorf("key %q: got %v, want %v", k, got, v)
							}
						}
					}
				}
				if !found {
					t.Errorf("attribute %q not found on span", k)
				}
			}
		})
	}
}

func TestRecordReaderTraceAttributes(t *testing.T) {
	testCases := []struct {
		name       string
		readMode   string
		offset     int64
		length     int64
		objectName string
		wantAttrs  map[string]interface{}
	}{
		{
			name:       "range",
			readMode:   "range",
			offset:     100,
			length:     500,
			objectName: "read-obj.txt",
			wantAttrs: map[string]interface{}{
				"storage.read.mode":   "range",
				"storage.read.offset": int64(100),
				"storage.read.length": int64(500),
				"storage.object.name": "read-obj.txt",
			},
		},
		{
			name:       "full",
			readMode:   "full",
			offset:     0,
			length:     -1,
			objectName: "read-full.txt",
			wantAttrs: map[string]interface{}{
				"storage.read.mode":   "full",
				"storage.object.name": "read-full.txt",
			},
		},
		{
			name:       "multi_range",
			readMode:   "multi_range",
			offset:     0,
			length:     0,
			objectName: "read-mrd.txt",
			wantAttrs: map[string]interface{}{
				"storage.read.mode":   "multi_range",
				"storage.object.name": "read-mrd.txt",
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
			t.Setenv("GO_STORAGE_DEV_OTEL_TRACING", "true")

			spanName := "Object.Reader"
			ctx, _ = startSpan(ctx, spanName)
			recordReaderTraceAttributes(ctx, tc.readMode, tc.offset, tc.length, tc.objectName)
			endSpan(ctx, nil)

			spans := te.Spans()
			if len(spans) != 1 {
				t.Fatalf("expected 1 span, got %d", len(spans))
			}
			gotSpan := spans[0]

			for k, wantVal := range tc.wantAttrs {
				found := false
				for _, a := range gotSpan.Attributes {
					if string(a.Key) == k {
						found = true
						switch v := wantVal.(type) {
						case string:
							if got := a.Value.AsString(); got != v {
								t.Errorf("key %q: got %v, want %v", k, got, v)
							}
						case int64:
							if got := a.Value.AsInt64(); got != v {
								t.Errorf("key %q: got %v, want %v", k, got, v)
							}
						}
					}
				}
				if !found {
					t.Errorf("attribute %q not found on span", k)
				}
			}
		})
	}
}

func TestStartChunkSpanWithChunkNumber(t *testing.T) {
	ctx := context.Background()
	te := testutil.NewOpenTelemetryTestExporter()
	t.Cleanup(func() {
		te.Unregister(ctx)
	})
	t.Setenv("GO_STORAGE_DEV_OTEL_TRACING", "true")

	chunkCtx, _ := startChunkSpan(ctx, "Storage.UploadChunk", 262144, 262144, withChunkNumber(2))
	endSpan(chunkCtx, nil)

	spans := te.Spans()
	if len(spans) != 1 {
		t.Fatalf("expected 1 span, got %d", len(spans))
	}
	gotSpan := spans[0]
	if got, want := gotSpan.Name, "cloud.google.com/go/storage.Storage.UploadChunk"; got != want {
		t.Errorf("got span name %q, want %q", got, want)
	}

	wantAttrs := map[string]interface{}{
		"upload.chunk_number":      int64(2),
		"gcp.storage.chunk.offset": int64(262144),
		"gcp.storage.chunk.size":   int64(262144),
	}
	for k, wantVal := range wantAttrs {
		found := false
		for _, a := range gotSpan.Attributes {
			if string(a.Key) == k {
				found = true
				if got := a.Value.AsInt64(); got != wantVal.(int64) {
					t.Errorf("key %q: got %v, want %v", k, got, wantVal)
				}
			}
		}
		if !found {
			t.Errorf("attribute %q not found on span", k)
		}
	}
}

func TestWriterMarkClosedEndsChunkSpan(t *testing.T) {
	ctx := context.Background()
	te := testutil.NewOpenTelemetryTestExporter()
	t.Cleanup(func() {
		te.Unregister(ctx)
	})
	t.Setenv("GO_STORAGE_DEV_OTEL_TRACING", "true")

	w := &Writer{
		ctx:       ctx,
		ChunkSize: 256 * 1024,
	}
	_, span := startSpan(ctx, "Storage.UploadChunk")
	w.curChunkSpan = span

	if err := w.markClosed(nil); err != nil {
		t.Fatalf("w.markClosed: %v", err)
	}
	if w.curChunkSpan != nil {
		t.Errorf("expected w.curChunkSpan to be nil after markClosed, got %v", w.curChunkSpan)
	}
}

func TestStartChecksumSpan(t *testing.T) {
	ctx := context.Background()
	te := testutil.NewOpenTelemetryTestExporter()
	t.Cleanup(func() {
		te.Unregister(ctx)
	})
	t.Setenv("GO_STORAGE_DEV_OTEL_TRACING", "true")

	chkCtx, _ := startChecksumSpan(ctx, "CRC32C")
	endSpan(chkCtx, nil)

	spans := te.Spans()
	if len(spans) != 1 {
		t.Fatalf("expected 1 span, got %d", len(spans))
	}
	gotSpan := spans[0]
	if got, want := gotSpan.Name, "cloud.google.com/go/storage.Storage.CalculateChecksum"; got != want {
		t.Errorf("got span name %q, want %q", got, want)
	}
	foundChecksumType := false
	for _, a := range gotSpan.Attributes {
		if string(a.Key) == "gcp.storage.checksum.type" {
			foundChecksumType = true
			if got, want := a.Value.AsString(), "CRC32C"; got != want {
				t.Errorf("checksum type = %q, want %q", got, want)
			}
		}
	}
	if !foundChecksumType {
		t.Errorf("gcp.storage.checksum.type attribute not found on span")
	}
}

func TestChecksumSpanDevTracingDisabled(t *testing.T) {
	ctx := context.Background()
	te := testutil.NewOpenTelemetryTestExporter()
	t.Cleanup(func() {
		te.Unregister(ctx)
	})
	t.Setenv("GO_STORAGE_DEV_OTEL_TRACING", "false")

	// Start a parent span
	ctx, _ = tracer().Start(ctx, "ParentSpan")

	// Call startChecksumSpan and endSpan with the returned context
	chkCtx, _ := startChecksumSpan(ctx, "CRC32C")
	span := trace.SpanFromContext(chkCtx)
	span.End()

	spans := te.Spans()
	if len(spans) > 0 {
		t.Fatalf("expected 0 ended spans because ParentSpan should still be active, but got %d ended spans: %v", len(spans), spans[0].Name)
	}
}

func TestMetadataRetryBackoffTracing(t *testing.T) {
	ctx := context.Background()
	te := testutil.NewOpenTelemetryTestExporter()
	t.Cleanup(func() {
		te.Unregister(ctx)
	})
	t.Setenv("GO_STORAGE_DEV_OTEL_TRACING", "true")

	// Test metadata operation (e.g. Bucket.Attrs or ObjectsListCall) experiencing 2 retries.
	spanName := "Bucket.Attrs"
	ctx, _ = startSpan(ctx, spanName)
	recordRetryBackoffEvent(ctx, 1, time.Now().Add(-100*time.Millisecond))
	recordRetryBackoffEvent(ctx, 2, time.Now().Add(-200*time.Millisecond))
	endSpan(ctx, nil)

	spans := te.Spans()
	if len(spans) != 3 {
		t.Fatalf("expected 3 spans (1 parent metadata span + 2 RetryBackoff child spans), got %d", len(spans))
	}

	var parentSpan tracetest.SpanStub
	var backoffCount int
	for _, s := range spans {
		if strings.HasSuffix(s.Name, "RetryBackoff") {
			backoffCount++
		} else if strings.HasSuffix(s.Name, spanName) {
			parentSpan = s
		}
	}
	if backoffCount != 2 {
		t.Errorf("expected 2 RetryBackoff child spans, got %d", backoffCount)
	}
	if parentSpan.Name == "" {
		t.Fatalf("failed to find parent metadata span %q", spanName)
	}
	if len(parentSpan.Events) != 2 {
		t.Fatalf("expected 2 storage.retry.backoff events on metadata span, got %d", len(parentSpan.Events))
	}
	for i, ev := range parentSpan.Events {
		if ev.Name != "storage.retry.backoff" {
			t.Errorf("event %d: got name %q, want %q", i, ev.Name, "storage.retry.backoff")
		}
	}
}

type staticTokenSource struct {
	token *oauth2.Token
}

func (s *staticTokenSource) Token() (*oauth2.Token, error) {
	return s.token, nil
}

func TestTracedTokenSourceSpan(t *testing.T) {
	ctx := context.Background()
	te := testutil.NewOpenTelemetryTestExporter()
	t.Cleanup(func() {
		te.Unregister(ctx)
	})
	t.Setenv("GO_STORAGE_DEV_OTEL_TRACING", "true")

	rawTS := &staticTokenSource{token: &oauth2.Token{AccessToken: "fake-token"}}
	tts := NewTracedTokenSource(rawTS)

	tok, err := tts.Token()
	if err != nil {
		t.Fatalf("tts.Token: %v", err)
	}
	if tok.AccessToken != "fake-token" {
		t.Fatalf("got access token %q, want %q", tok.AccessToken, "fake-token")
	}

	spans := te.Spans()
	if len(spans) != 1 {
		t.Fatalf("expected 1 span, got %d", len(spans))
	}
	if got, want := spans[0].Name, "cloud.google.com/go/storage.Auth.RefreshAccessToken"; got != want {
		t.Errorf("got span name %q, want %q", got, want)
	}
}

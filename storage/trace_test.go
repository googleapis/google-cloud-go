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
				"gcp.storage.write.mode":   "resumable",
				"gcp.storage.payload.size": int64(256 * 1024),
				"gcp.storage.object.name":  "test-file.txt",
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
				"gcp.storage.write.mode":   "oneshot",
				"gcp.storage.payload.size": int64(0),
				"gcp.storage.object.name":  "test-oneshot.txt",
			},
		},
		{
			name: "oneshot_negative_chunk",
			writer: &Writer{
				ChunkSize:            -1,
				Append:               false,
				EnableParallelUpload: false,
				ObjectAttrs:          ObjectAttrs{Name: "test-oneshot-neg.txt"},
			},
			wantAttrs: map[string]interface{}{
				"gcp.storage.write.mode":   "oneshot",
				"gcp.storage.payload.size": int64(-1),
				"gcp.storage.object.name":  "test-oneshot-neg.txt",
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
				"gcp.storage.write.mode":   "appendable",
				"gcp.storage.payload.size": int64(256 * 1024),
				"gcp.storage.object.name":  "test-append.txt",
			},
		},
		{
			name: "parallel",
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
				"gcp.storage.write.mode":            "parallel",
				"gcp.storage.payload.size":          int64(256 * 1024),
				"gcp.storage.object.name":           "test-parallel.txt",
				"gcp.storage.parallel.part_size":   int64(16 * 1024 * 1024),
				"gcp.storage.parallel.concurrency": int64(4),
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
				"gcp.storage.read.mode":      "range",
				"gcp.storage.payload.offset": int64(100),
				"gcp.storage.payload.size":   int64(500),
				"gcp.storage.object.name":    "read-obj.txt",
			},
		},
		{
			name:       "full",
			readMode:   "full",
			offset:     0,
			length:     -1,
			objectName: "read-full.txt",
			wantAttrs: map[string]interface{}{
				"gcp.storage.read.mode":   "full",
				"gcp.storage.object.name": "read-full.txt",
			},
		},
		{
			name:       "multi_range",
			readMode:   "multi_range",
			offset:     0,
			length:     0,
			objectName: "read-mrd.txt",
			wantAttrs: map[string]interface{}{
				"gcp.storage.read.mode":   "multi_range",
				"gcp.storage.object.name": "read-mrd.txt",
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

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

	rCtx, _ := startSpan(ctx, "Object.Reader")
	recordReaderTraceAttributes(rCtx, "full")
	endSpan(rCtx, nil)

	for _, s := range te.Spans() {
		for _, a := range s.Attributes {
			switch a.Key {
			case "gcp.storage.write.mode",
				"gcp.storage.write.parallel.part_size",
				"gcp.storage.write.parallel.concurrency",
				"gcp.storage.read.mode":
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
	got := attribute.NewSet(spans[0].Attributes...)
	want := attribute.NewSet(append(getCommonAttributes(), wantAttrs...)...)
	if !got.Equals(&want) {
		t.Errorf("span %s attributes mismatch:\n got: %s\nwant: %s", spans[0].Name,
			got.Encoded(attribute.DefaultEncoder()), want.Encoded(attribute.DefaultEncoder()))
	}
}

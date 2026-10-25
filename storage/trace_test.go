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
	"io"
	"net"
	"net/http"
	"testing"
	"time"

	"cloud.google.com/go/internal/testutil"
	"cloud.google.com/go/storage/internal"
	gapic "cloud.google.com/go/storage/internal/apiv2"
	"cloud.google.com/go/storage/internal/apiv2/storagepb"
	"github.com/google/go-cmp/cmp"
	"go.opentelemetry.io/otel/attribute"
	otcodes "go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/sdk/instrumentation"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
	"go.opentelemetry.io/otel/trace"
	"google.golang.org/api/googleapi"
	"google.golang.org/api/option"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/test/bufconn"
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

func TestSetUploadIDAttribute(t *testing.T) {
	const sampleUploadID = "AJjja9aXX4R20FIZ2ddLnYddhJmD9Xm_Y2B_NEu5G2a6PzJiGRgPKLp6hEQpM"
	spanName := "Object.Writer"

	tests := []struct {
		name       string
		devTracing string
		uploadID   string
		wantAttrs  []attribute.KeyValue
	}{
		{
			name:       "Records upload.id when dev tracing enabled",
			devTracing: "true",
			uploadID:   sampleUploadID,
			wantAttrs:  append(getCommonAttributes(), attribute.String("gcp.storage.upload.id", sampleUploadID)),
		},
		{
			name:       "Omits upload.id when upload_id is empty",
			devTracing: "true",
			uploadID:   "",
			wantAttrs:  getCommonAttributes(),
		},
		{
			name:       "Omits upload.id when dev tracing disabled",
			devTracing: "false",
			uploadID:   sampleUploadID,
			wantAttrs:  nil,
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
			t.Setenv(storageOtelTracingDevVar, tc.devTracing)

			ctx, _ = startSpan(ctx, spanName)
			setUploadIDAttribute(ctx, tc.uploadID)
			endSpan(ctx, nil)

			spans := te.Spans()
			if len(spans) != 1 {
				t.Fatalf("expected one span, got %d", len(spans))
			}
			gotSpan := spans[0]
			if got, want := gotSpan.Name, appendPackageName(spanName); got != want {
				t.Fatalf("got %s, want %s", got, want)
			}

			if tc.devTracing == "true" {
				wantSpan := createWantSpanStub(spanName, tc.wantAttrs)
				opts := []cmp.Option{
					cmp.Comparer(spanAttributesComparer),
				}
				if diff := testutil.Diff(gotSpan, wantSpan, opts...); diff != "" {
					t.Errorf("diff: -got, +want:\n%s\n", diff)
				}
			} else if len(gotSpan.Attributes) != 0 {
				t.Errorf("expected no attributes when dev tracing is disabled, got %v", gotSpan.Attributes)
			}
		})
	}
}

func TestTrackingTransportUploadID(t *testing.T) {
	const sampleUploadID = "AJjja9aXX4R20FIZ2ddLnYddhJmD9Xm_Y2B_NEu5G2a6PzJiGRgPKLp6hEQpM"
	spanName := "Object.Writer"

	tests := []struct {
		name       string
		reqURL     string
		respHeader http.Header
		wantAttrs  []attribute.KeyValue
	}{
		{
			name:   "Extracts upload_id from Location response header",
			reqURL: "http://example.com/upload/storage/v1/b/my-bucket/o?uploadType=resumable",
			respHeader: http.Header{
				"Location": {"http://example.com/upload/storage/v1/b/my-bucket/o?uploadType=resumable&upload_id=" + sampleUploadID},
			},
			wantAttrs: append(getCommonAttributes(), attribute.String("gcp.storage.upload.id", sampleUploadID)),
		},
		{
			name:      "Omits upload.id when Location header not present",
			reqURL:    "http://example.com/storage/v1/b/my-bucket/o",
			wantAttrs: getCommonAttributes(),
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

			mock := &mockTransport{}
			mock.addResult(&http.Response{Status: "200 OK", Header: tc.respHeader}, nil)
			tt := &trackingTransport{base: mock}

			ctx, _ = startSpan(ctx, spanName)
			req, err := http.NewRequestWithContext(ctx, "POST", tc.reqURL, nil)
			if err != nil {
				t.Fatalf("NewRequestWithContext failed: %v", err)
			}
			if _, err := tt.RoundTrip(req); err != nil {
				t.Fatalf("RoundTrip failed: %v", err)
			}
			endSpan(ctx, nil)

			spans := te.Spans()
			if len(spans) != 1 {
				t.Fatalf("expected one span, got %d", len(spans))
			}
			gotSpan := spans[0]
			wantSpan := createWantSpanStub(spanName, tc.wantAttrs)
			opts := []cmp.Option{
				cmp.Comparer(spanAttributesComparer),
			}
			if diff := testutil.Diff(gotSpan, wantSpan, opts...); diff != "" {
				t.Errorf("diff: -got, +want:\n%s\n", diff)
			}
		})
	}
}

func TestSetUploadIDFromHeader(t *testing.T) {
	const sampleUploadID = "AJjja9aXX4R20FIZ2ddLnYddhJmD9Xm_Y2B_NEu5G2a6PzJiGRgPKLp6hEQpM"
	spanName := "Object.Writer"

	tests := []struct {
		name       string
		devTracing string
		header     http.Header
		wantAttrs  []attribute.KeyValue
	}{
		{
			name:       "Records upload.id from Location header",
			devTracing: "true",
			header: http.Header{
				"Location": {"https://storage.googleapis.com/upload/storage/v1/b/my-bucket/o?uploadType=resumable&upload_id=" + sampleUploadID},
			},
			wantAttrs: append(getCommonAttributes(), attribute.String("gcp.storage.upload.id", sampleUploadID)),
		},
		{
			name:       "Omits upload.id when Location has no upload_id",
			devTracing: "true",
			header: http.Header{
				"Location": {"https://storage.googleapis.com/upload/storage/v1/b/my-bucket/o?uploadType=resumable"},
			},
			wantAttrs: getCommonAttributes(),
		},
		{
			name:       "Omits upload.id when Location is unparseable",
			devTracing: "true",
			header: http.Header{
				"Location": {"://bad-url"},
			},
			wantAttrs: getCommonAttributes(),
		},
		{
			name:       "Omits upload.id when header is nil",
			devTracing: "true",
			wantAttrs:  getCommonAttributes(),
		},
		{
			name:       "Omits upload.id when dev tracing disabled",
			devTracing: "false",
			header: http.Header{
				"Location": {"https://storage.googleapis.com/upload/storage/v1/b/my-bucket/o?upload_id=" + sampleUploadID},
			},
			wantAttrs: nil,
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
			t.Setenv(storageOtelTracingDevVar, tc.devTracing)

			ctx, _ = startSpan(ctx, spanName)
			setUploadIDFromHeader(ctx, tc.header)
			endSpan(ctx, nil)

			spans := te.Spans()
			if len(spans) != 1 {
				t.Fatalf("expected one span, got %d", len(spans))
			}
			gotSpan := spans[0]
			if tc.devTracing != "true" {
				if len(gotSpan.Attributes) != 0 {
					t.Errorf("expected no attributes when dev tracing is disabled, got %v", gotSpan.Attributes)
				}
				return
			}
			wantSpan := createWantSpanStub(spanName, tc.wantAttrs)
			opts := []cmp.Option{
				cmp.Comparer(spanAttributesComparer),
			}
			if diff := testutil.Diff(gotSpan, wantSpan, opts...); diff != "" {
				t.Errorf("diff: -got, +want:\n%s\n", diff)
			}
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

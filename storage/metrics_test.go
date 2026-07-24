// Copyright 2026 Google LLC
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
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"

	"cloud.google.com/go/storage/internal/apiv2/storagepb"

	"go.opentelemetry.io/otel/attribute"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/metric/metricdata"
	"go.opentelemetry.io/otel/sdk/resource"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
)

func TestFormatMetricWithPrefix(t *testing.T) {
	s := metricdata.Metrics{Name: "metric.name"}
	for _, tc := range []struct {
		prefix string
		want   string
	}{
		{prefix: metricPrefix, want: "storage.googleapis.com/client/metric/name"},
		{prefix: customMetricPrefix, want: "custom.googleapis.com/metric/name"},
	} {
		got := formatMetricWithPrefix(s, tc.prefix)
		if got != tc.want {
			t.Errorf("formatMetricWithPrefix(s, %q) = %q, want %q", tc.prefix, got, tc.want)
		}
	}
}

func TestIsOtelMetricsEnabled(t *testing.T) {
	// Test config option only (env var not set).
	cfg := storageConfig{enableOtelMetrics: true}
	os.Unsetenv("GCP_STORAGE_GO_ENABLE_OTEL_METRICS")
	if !isOtelMetricsEnabled(&cfg) {
		t.Errorf("expected Otel metrics to be enabled via config option")
	}

	cfg = storageConfig{enableOtelMetrics: false}
	if isOtelMetricsEnabled(&cfg) {
		t.Errorf("expected Otel metrics to be disabled when config option is false")
	}

	// Test env var override (option is false, env var is true).
	cfg = storageConfig{enableOtelMetrics: false}
	os.Setenv("GCP_STORAGE_GO_ENABLE_OTEL_METRICS", "true")
	if !isOtelMetricsEnabled(&cfg) {
		t.Errorf("expected Otel metrics to be enabled via env var override (option=false)")
	}

	// Test env var override (option is true, env var is false).
	cfg = storageConfig{enableOtelMetrics: true}
	os.Setenv("GCP_STORAGE_GO_ENABLE_OTEL_METRICS", "false")
	if isOtelMetricsEnabled(&cfg) {
		t.Errorf("expected Otel metrics to be disabled via env var override (option=true)")
	}

	// Test env var override with truthy "1".
	cfg = storageConfig{enableOtelMetrics: false}
	os.Setenv("GCP_STORAGE_GO_ENABLE_OTEL_METRICS", "1")
	if !isOtelMetricsEnabled(&cfg) {
		t.Errorf("expected Otel metrics to be enabled via env var override set to 1")
	}

	// Test env var override with falsy "0".
	cfg = storageConfig{enableOtelMetrics: true}
	os.Setenv("GCP_STORAGE_GO_ENABLE_OTEL_METRICS", "0")
	if isOtelMetricsEnabled(&cfg) {
		t.Errorf("expected Otel metrics to be disabled via env var override set to 0")
	}

	os.Unsetenv("GCP_STORAGE_GO_ENABLE_OTEL_METRICS")
}

func TestIsOtelDebugMetricsEnabled(t *testing.T) {
	// Test config option only (env var not set).
	cfg := storageConfig{enableOtelDebugMetrics: true}
	os.Unsetenv("GCP_STORAGE_GO_ENABLE_OTEL_DEBUG_METRICS")
	if !isOtelDebugMetricsEnabled(&cfg) {
		t.Errorf("expected Otel debug metrics to be enabled via config option")
	}

	cfg = storageConfig{enableOtelDebugMetrics: false}
	if isOtelDebugMetricsEnabled(&cfg) {
		t.Errorf("expected Otel debug metrics to be disabled when config option is false")
	}

	// Test env var override (option is false, env var is true).
	cfg = storageConfig{enableOtelDebugMetrics: false}
	os.Setenv("GCP_STORAGE_GO_ENABLE_OTEL_DEBUG_METRICS", "true")
	if !isOtelDebugMetricsEnabled(&cfg) {
		t.Errorf("expected Otel debug metrics to be enabled via env var override (option=false)")
	}

	// Test env var override (option is true, env var is false).
	cfg = storageConfig{enableOtelDebugMetrics: true}
	os.Setenv("GCP_STORAGE_GO_ENABLE_OTEL_DEBUG_METRICS", "false")
	if isOtelDebugMetricsEnabled(&cfg) {
		t.Errorf("expected Otel debug metrics to be disabled via env var override (option=true)")
	}

	// Test env var override with truthy "1".
	cfg = storageConfig{enableOtelDebugMetrics: false}
	os.Setenv("GCP_STORAGE_GO_ENABLE_OTEL_DEBUG_METRICS", "1")
	if !isOtelDebugMetricsEnabled(&cfg) {
		t.Errorf("expected Otel debug metrics to be enabled via env var override set to 1")
	}

	// Test env var override with falsy "0".
	cfg = storageConfig{enableOtelDebugMetrics: true}
	os.Setenv("GCP_STORAGE_GO_ENABLE_OTEL_DEBUG_METRICS", "0")
	if isOtelDebugMetricsEnabled(&cfg) {
		t.Errorf("expected Otel debug metrics to be disabled via env var override set to 0")
	}

	os.Unsetenv("GCP_STORAGE_GO_ENABLE_OTEL_DEBUG_METRICS")
}

func TestComputeURLTemplate(t *testing.T) {
	tests := []struct {
		desc string
		path string
		host string
		want string
	}{
		{
			desc: "JSON API bucket list",
			path: "/storage/v1/b",
			host: "storage.googleapis.com",
			want: "/storage/v1/b",
		},
		{
			desc: "JSON API bucket attrs",
			path: "/storage/v1/b/my-bucket",
			host: "storage.googleapis.com",
			want: "/storage/v1/b/{bucket}",
		},
		{
			desc: "JSON API object list",
			path: "/storage/v1/b/my-bucket/o",
			host: "storage.googleapis.com",
			want: "/storage/v1/b/{bucket}/o",
		},
		{
			desc: "JSON API object attrs / download",
			path: "/storage/v1/b/my-bucket/o/my/object/path",
			host: "storage.googleapis.com",
			want: "/storage/v1/b/{bucket}/o/{object}",
		},
		{
			desc: "JSON API upload",
			path: "/upload/storage/v1/b/my-bucket/o",
			host: "storage.googleapis.com",
			want: "/upload/storage/v1/b/{bucket}/o",
		},
		{
			desc: "XML API host-style root",
			path: "/",
			host: "my-bucket.storage.googleapis.com",
			want: "/",
		},
		{
			desc: "XML API host-style object",
			path: "/my/object/path",
			host: "my-bucket.storage.googleapis.com",
			want: "/{object}",
		},
		{
			desc: "XML API path-style root",
			path: "/",
			host: "storage.googleapis.com",
			want: "/",
		},
		{
			desc: "XML API path-style bucket only",
			path: "/my-bucket",
			host: "storage.googleapis.com",
			want: "/{bucket}",
		},
		{
			desc: "XML API path-style object",
			path: "/my-bucket/my/object/path",
			host: "storage.googleapis.com",
			want: "/{bucket}/{object}",
		},
	}

	for _, tc := range tests {
		t.Run(tc.desc, func(t *testing.T) {
			got := computeURLTemplate(tc.path, tc.host)
			if got != tc.want {
				t.Errorf("computeURLTemplate(%q, %q) = %q, want %q", tc.path, tc.host, got, tc.want)
			}
		})
	}
}

func TestHTTPMetricsRecording(t *testing.T) {
	ctx := context.Background()
	mr := sdkmetric.NewManualReader()

	// Create a resource with static attributes so that we can test resource propagation.
	res, err := resource.New(ctx,
		resource.WithAttributes(
			attribute.String("gcp.client.service", "storage"),
			attribute.String("gcp.client.repo", "googleapis/google-cloud-go"),
		),
	)
	if err != nil {
		t.Fatalf("resource.New: %v", err)
	}

	provider := sdkmetric.NewMeterProvider(
		sdkmetric.WithReader(mr),
		sdkmetric.WithResource(res),
	)
	defer provider.Shutdown(ctx)

	cfg := storageConfig{
		enableOtelMetrics:      true,
		enableOtelDebugMetrics: true,
		meterProvider:          provider,
	}

	cm, _, err := initMetrics(ctx, "project-id", &cfg)
	if err != nil {
		t.Fatalf("initMetrics: %v", err)
	}

	// Create a mock HTTP server to respond to requests.
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Goog-Gfe-Service-Time", "150")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("hello world"))
	}))
	defer server.Close()

	client := &http.Client{
		Transport: &metricsRoundTripper{
			base:    http.DefaultTransport,
			metrics: cm,
		},
	}

	// Make a request.
	req, err := http.NewRequestWithContext(ctx, "GET", server.URL+"/storage/v1/b/my-bucket/o/my-object", nil)
	if err != nil {
		t.Fatalf("NewRequest: %v", err)
	}

	resp, err := client.Do(req)
	if err != nil {
		t.Fatalf("client.Do: %v", err)
	}

	// Read and close the body (draining).
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("ReadAll: %v", err)
	}
	if string(body) != "hello world" {
		t.Errorf("unexpected body: %q", string(body))
	}
	resp.Body.Close()

	// Collect metrics.
	var rm metricdata.ResourceMetrics
	if err := mr.Collect(ctx, &rm); err != nil {
		t.Fatalf("Collect: %v", err)
	}

	found := false
	for _, sm := range rm.ScopeMetrics {
		for _, m := range sm.Metrics {
			if m.Name == "http.client.request.duration" {
				found = true
				hist, ok := m.Data.(metricdata.Histogram[float64])
				if !ok {
					t.Fatalf("expected Histogram data, got %T", m.Data)
				}
				if len(hist.DataPoints) != 1 {
					t.Fatalf("expected 1 datapoint, got %d", len(hist.DataPoints))
				}
				dp := hist.DataPoints[0]

				// Verify dynamic attributes are present on the data point.
				attrMap := make(map[string]string)
				for _, kv := range dp.Attributes.ToSlice() {
					attrMap[string(kv.Key)] = kv.Value.Emit()
				}

				if attrMap["rpc.system.name"] != "http" {
					t.Errorf("expected rpc.system.name http, got %q", attrMap["rpc.system.name"])
				}
				if attrMap["http.request.method"] != "GET" {
					t.Errorf("expected GET, got %q", attrMap["http.request.method"])
				}
				if attrMap["url.template"] != "/storage/v1/b/{bucket}/o/{object}" {
					t.Errorf("expected template /storage/v1/b/{bucket}/o/{object}, got %q", attrMap["url.template"])
				}
				if attrMap["http.response.status_code"] != "200" {
					t.Errorf("expected status_code 200, got %q", attrMap["http.response.status_code"])
				}
				if attrMap["error.type"] != "OK" {
					t.Errorf("expected error.type OK, got %q", attrMap["error.type"])
				}
			}

			if m.Name == "gcp.storage.client.gfe.duration" {
				hist, ok := m.Data.(metricdata.Histogram[float64])
				if ok && len(hist.DataPoints) > 0 {
					if hist.DataPoints[0].Sum != 0.15 {
						t.Errorf("expected gfe.duration 0.15s, got %v", hist.DataPoints[0].Sum)
					}
				} else {
					t.Errorf("expected gfe.duration datapoints")
				}
			}
		}
	}

	if !found {
		t.Errorf("metric http.client.request.duration not found")
	}
}

type mockClientStream struct {
	grpc.ClientStream
	recvErr error
}

func (m *mockClientStream) RecvMsg(msg interface{}) error {
	return m.recvErr
}

func (m *mockClientStream) Header() (metadata.MD, error) {
	return metadata.Pairs("x-goog-gfe-service-time", "120"), nil
}

func (m *mockClientStream) Trailer() metadata.MD {
	return nil
}

func TestGRPCMetricsRecording(t *testing.T) {
	ctx := context.Background()
	mr := sdkmetric.NewManualReader()
	provider := sdkmetric.NewMeterProvider(sdkmetric.WithReader(mr))
	defer provider.Shutdown(ctx)

	cfg := storageConfig{
		enableOtelMetrics:      true,
		enableOtelDebugMetrics: true,
		meterProvider:          provider,
	}

	cm, _, err := initMetrics(ctx, "project-id", &cfg)
	if err != nil {
		t.Fatalf("initMetrics: %v", err)
	}

	unaryInt, streamInt := metricsInterceptors(cm)

	invoker := func(ctx context.Context, method string, req, reply interface{}, cc *grpc.ClientConn, opts ...grpc.CallOption) error {
		return status.Error(codes.NotFound, "not found")
	}

	err = unaryInt(ctx, "/google.storage.v2.Storage/GetObject", nil, nil, nil, invoker)
	if err == nil || status.Code(err) != codes.NotFound {
		t.Errorf("unexpected unary error: %v", err)
	}

	// Test Server-Streaming call (ReadObject).
	streamer := func(ctx context.Context, desc *grpc.StreamDesc, cc *grpc.ClientConn, method string, opts ...grpc.CallOption) (grpc.ClientStream, error) {
		return &mockClientStream{recvErr: io.EOF}, nil
	}

	descRead := &grpc.StreamDesc{
		ServerStreams: true,
		ClientStreams: false,
	}
	clientStream, err := streamInt(ctx, descRead, nil, "/google.storage.v2.Storage/ReadObject", streamer)
	if err != nil {
		t.Fatalf("streamInt: %v", err)
	}

	// Trigger stream termination by receiving EOF.
	if err := clientStream.RecvMsg(nil); err != io.EOF {
		t.Errorf("expected io.EOF, got %v", err)
	}

	// Test Client-Streaming call (WriteObject).
	streamerWrite := func(ctx context.Context, desc *grpc.StreamDesc, cc *grpc.ClientConn, method string, opts ...grpc.CallOption) (grpc.ClientStream, error) {
		return &mockClientStream{recvErr: nil}, nil
	}

	descWrite := &grpc.StreamDesc{
		ServerStreams: false,
		ClientStreams: true,
	}
	clientStreamWrite, err := streamInt(ctx, descWrite, nil, "/google.storage.v2.Storage/WriteObject", streamerWrite)
	if err != nil {
		t.Fatalf("streamInt: %v", err)
	}

	// Trigger stream termination by successfully receiving response (returns nil).
	if err := clientStreamWrite.RecvMsg(nil); err != nil {
		t.Errorf("expected nil error, got %v", err)
	}

	// Collect metrics.
	var rm metricdata.ResourceMetrics
	if err := mr.Collect(ctx, &rm); err != nil {
		t.Fatalf("Collect: %v", err)
	}

	var unaryDp, streamDp, writeDp *metricdata.HistogramDataPoint[float64]

	for _, sm := range rm.ScopeMetrics {
		for _, m := range sm.Metrics {
			if m.Name == "rpc.client.call.duration" {
				hist, ok := m.Data.(metricdata.Histogram[float64])
				if !ok {
					t.Fatalf("expected Histogram data, got %T", m.Data)
				}
				for _, dp := range hist.DataPoints {
					dpCopy := dp // avoid reference capture of loop variable
					attrs := make(map[string]string)
					for _, kv := range dp.Attributes.ToSlice() {
						attrs[string(kv.Key)] = kv.Value.Emit()
					}
					if attrs["rpc.method"] == "GetObject" {
						unaryDp = &dpCopy
					} else if attrs["rpc.method"] == "ReadObject" {
						streamDp = &dpCopy
					} else if attrs["rpc.method"] == "WriteObject" {
						writeDp = &dpCopy
					}
				}
			}

			if m.Name == "gcp.storage.client.gfe.duration" {
				hist, ok := m.Data.(metricdata.Histogram[float64])
				if ok && len(hist.DataPoints) > 0 {
					foundGfeDuration := false
					for _, dp := range hist.DataPoints {
						if dp.Sum == 0.12 {
							foundGfeDuration = true
						}
					}
					if !foundGfeDuration {
						t.Errorf("expected gfe.duration 0.12s from stream")
					}
				} else {
					t.Errorf("expected gfe.duration datapoints")
				}
			}
		}
	}

	if unaryDp == nil {
		t.Errorf("unary metric not recorded")
	} else {
		attrs := make(map[string]string)
		for _, kv := range unaryDp.Attributes.ToSlice() {
			attrs[string(kv.Key)] = kv.Value.Emit()
		}
		if attrs["rpc.system.name"] != "grpc" {
			t.Errorf("expected rpc.system.name grpc, got %q", attrs["rpc.system.name"])
		}
		if attrs["rpc.service"] != "google.storage.v2.Storage" {
			t.Errorf("expected rpc.service, got %q", attrs["rpc.service"])
		}
		if attrs["rpc.grpc.status_code"] != "5" { // codes.NotFound is 5.
			t.Errorf("expected status_code 5, got %q", attrs["rpc.grpc.status_code"])
		}
		if attrs["error.type"] != "NOT_FOUND" {
			t.Errorf("expected error.type NOT_FOUND, got %q", attrs["error.type"])
		}
	}

	if streamDp == nil {
		t.Errorf("streaming metric (ReadObject) not recorded")
	} else {
		attrs := make(map[string]string)
		for _, kv := range streamDp.Attributes.ToSlice() {
			attrs[string(kv.Key)] = kv.Value.Emit()
		}
		if attrs["rpc.system.name"] != "grpc" {
			t.Errorf("expected rpc.system.name grpc, got %q", attrs["rpc.system.name"])
		}
		if attrs["rpc.service"] != "google.storage.v2.Storage" {
			t.Errorf("expected rpc.service, got %q", attrs["rpc.service"])
		}
		if attrs["rpc.grpc.status_code"] != "0" { // codes.OK is 0 (io.EOF maps to OK).
			t.Errorf("expected status_code 0, got %q", attrs["rpc.grpc.status_code"])
		}
		if attrs["error.type"] != "OK" {
			t.Errorf("expected error.type OK, got %q", attrs["error.type"])
		}
	}

	if writeDp == nil {
		t.Errorf("streaming metric (WriteObject) not recorded")
	} else {
		attrs := make(map[string]string)
		for _, kv := range writeDp.Attributes.ToSlice() {
			attrs[string(kv.Key)] = kv.Value.Emit()
		}
		if attrs["rpc.system.name"] != "grpc" {
			t.Errorf("expected rpc.system.name grpc, got %q", attrs["rpc.system.name"])
		}
		if attrs["rpc.service"] != "google.storage.v2.Storage" {
			t.Errorf("expected rpc.service, got %q", attrs["rpc.service"])
		}
		if attrs["rpc.grpc.status_code"] != "0" { // codes.OK is 0.
			t.Errorf("expected status_code 0, got %q", attrs["rpc.grpc.status_code"])
		}
		if attrs["error.type"] != "OK" {
			t.Errorf("expected error.type OK, got %q", attrs["error.type"])
		}
	}
}

type mockStorageClient struct {
	storageClient
	getObjectFn  func(ctx context.Context, params *getObjectParams, opts ...storageOption) (*ObjectAttrs, error)
	newReaderFn  func(ctx context.Context, params *newRangeReaderParams, opts ...storageOption) (*Reader, error)
	openWriterFn func(params *openWriterParams, opts ...storageOption) (internalWriter, error)
}

func (m *mockStorageClient) GetObject(ctx context.Context, params *getObjectParams, opts ...storageOption) (*ObjectAttrs, error) {
	if m.getObjectFn != nil {
		return m.getObjectFn(ctx, params, opts...)
	}
	return nil, nil
}

func (m *mockStorageClient) NewRangeReader(ctx context.Context, params *newRangeReaderParams, opts ...storageOption) (*Reader, error) {
	if m.newReaderFn != nil {
		return m.newReaderFn(ctx, params, opts...)
	}
	return nil, nil
}

func (m *mockStorageClient) OpenWriter(params *openWriterParams, opts ...storageOption) (internalWriter, error) {
	if m.openWriterFn != nil {
		return m.openWriterFn(params, opts...)
	}
	return nil, nil
}

type mockInternalWriter struct {
	internalWriter
	writeFn func([]byte) (int, error)
	closeFn func() error
}

func (m *mockInternalWriter) Write(p []byte) (n int, err error) {
	if m.writeFn != nil {
		return m.writeFn(p)
	}
	return len(p), nil
}

func (m *mockInternalWriter) Close() error {
	if m.closeFn != nil {
		return m.closeFn()
	}
	return nil
}

func TestStandardMetricsRecording(t *testing.T) {
	ctx := context.Background()
	mr := sdkmetric.NewManualReader()
	provider := sdkmetric.NewMeterProvider(sdkmetric.WithReader(mr))
	defer provider.Shutdown(ctx)

	cfg := storageConfig{
		enableOtelMetrics: true,
		meterProvider:     provider,
	}

	cm, _, err := initMetrics(ctx, "project-id", &cfg)
	if err != nil {
		t.Fatalf("initMetrics: %v", err)
	}

	// Create mock storageClient.
	mock := &mockStorageClient{}
	wrapped := &metricsStorageClient{
		storageClient: mock,
		metrics:       cm,
		isHTTP:        false,
	}

	client := &Client{
		tc: wrapped,
	}

	// Test GetObject (unary).
	mock.getObjectFn = func(ctx context.Context, params *getObjectParams, opts ...storageOption) (*ObjectAttrs, error) {
		return &ObjectAttrs{Name: params.object}, nil
	}
	_, err = client.Bucket("my-bucket").Object("my-object").Attrs(ctx)
	if err != nil {
		t.Fatalf("Attrs: %v", err)
	}

	// Test Reader (ReadObject).
	mock.newReaderFn = func(ctx context.Context, params *newRangeReaderParams, opts ...storageOption) (*Reader, error) {
		return &Reader{
			reader: io.NopCloser(bytes.NewReader([]byte("hello"))),
			ctx:    ctx,
		}, nil
	}
	r, err := client.Bucket("my-bucket").Object("my-object").NewReader(ctx)
	if err != nil {
		t.Fatalf("NewReader: %v", err)
	}
	buf := make([]byte, 5)
	n, err := r.Read(buf)
	if err != nil && err != io.EOF {
		t.Fatalf("Read: %v", err)
	}
	if n != 5 || string(buf) != "hello" {
		t.Errorf("got %q, want %q", string(buf), "hello")
	}
	r.Close()

	// Test Writer (WriteObject).
	donec := make(chan struct{})
	close(donec) // pre-close it so Close doesn't block
	mock.openWriterFn = func(params *openWriterParams, opts ...storageOption) (internalWriter, error) {
		params.setObj(&ObjectAttrs{Name: "my-object", Size: 11})
		return &mockInternalWriter{
			writeFn: func(p []byte) (int, error) {
				return len(p), nil
			},
			closeFn: func() error {
				return nil
			},
		}, nil
	}
	w := client.Bucket("my-bucket").Object("my-object").NewWriter(ctx)
	w.ChunkSize = 0
	w.donec = donec
	n, err = w.Write([]byte("hello world"))
	if err != nil {
		t.Fatalf("Write: %v", err)
	}
	if n != 11 {
		t.Errorf("got write size %d, want 11", n)
	}
	if err := w.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}

	// Collect metrics.
	var rm metricdata.ResourceMetrics
	if err := mr.Collect(ctx, &rm); err != nil {
		t.Fatalf("Collect: %v", err)
	}

	// Verify the metrics.
	metricsMap := make(map[string]metricdata.Metrics)
	for _, sm := range rm.ScopeMetrics {
		for _, m := range sm.Metrics {
			metricsMap[m.Name] = m
		}
	}

	// Check gcp.client.request.duration.
	if m, ok := metricsMap["gcp.client.request.duration"]; !ok {
		t.Errorf("metric gcp.client.request.duration not found")
	} else {
		hist := m.Data.(metricdata.Histogram[float64])
		if len(hist.DataPoints) != 3 {
			t.Errorf("expected 3 datapoints for gcp.client.request.duration, got %d", len(hist.DataPoints))
		}
		methods := make(map[string]bool)
		for _, dp := range hist.DataPoints {
			for _, kv := range dp.Attributes.ToSlice() {
				if kv.Key == "rpc.method" {
					methods[kv.Value.AsString()] = true
				}
			}
		}
		if !methods["GetObject"] || !methods["ReadObject"] || !methods["WriteObject"] {
			t.Errorf("expected GetObject, ReadObject, WriteObject, got %v", methods)
		}
	}

	// Check gcp.storage.client.operations.
	if m, ok := metricsMap["gcp.storage.client.operations"]; !ok {
		t.Errorf("metric gcp.storage.client.operations not found")
	} else {
		sum := m.Data.(metricdata.Sum[int64])
		if len(sum.DataPoints) != 3 {
			t.Errorf("expected 3 datapoints for gcp.storage.client.operations, got %d", len(sum.DataPoints))
		}
	}

	// Check gcp.storage.client.response.body.size.
	if m, ok := metricsMap["gcp.storage.client.response.body.size"]; !ok {
		t.Errorf("metric gcp.storage.client.response.body.size not found")
	} else {
		hist := m.Data.(metricdata.Histogram[int64])
		if len(hist.DataPoints) != 1 {
			t.Fatalf("expected 1 datapoint for response body size, got %d", len(hist.DataPoints))
		}
		dp := hist.DataPoints[0]
		if dp.Sum != 5 {
			t.Errorf("expected sum 5, got %d", dp.Sum)
		}
	}

	// Check gcp.storage.client.request.body.size.
	if m, ok := metricsMap["gcp.storage.client.request.body.size"]; !ok {
		t.Errorf("metric gcp.storage.client.request.body.size not found")
	} else {
		hist := m.Data.(metricdata.Histogram[int64])
		if len(hist.DataPoints) != 1 {
			t.Fatalf("expected 1 datapoint for request body size, got %d", len(hist.DataPoints))
		}
		dp := hist.DataPoints[0]
		if dp.Sum != 11 {
			t.Errorf("expected sum 11, got %d", dp.Sum)
		}
	}
}

func TestRecordTTFB_MetadataOnly(t *testing.T) {
	ctx := context.Background()
	mr := sdkmetric.NewManualReader()
	provider := sdkmetric.NewMeterProvider(sdkmetric.WithReader(mr))
	defer provider.Shutdown(ctx)

	cfg := storageConfig{
		enableOtelMetrics: true,
		meterProvider:     provider,
	}

	cm, _, err := initMetrics(ctx, "project-id", &cfg)
	if err != nil {
		t.Fatalf("initMetrics: %v", err)
	}

	w := &wrappedClientStream{
		metrics:   cm,
		ctx:       ctx,
		method:    "/google.storage.v2.Storage/ReadObject",
		startTime: time.Now(),
	}

	// First response with only metadata should trigger TTFB.
	resp := &storagepb.ReadObjectResponse{
		Metadata: &storagepb.Object{Name: "test-object"},
	}
	w.recordTTFB(resp)

	if !w.recordedTTFB.Load() {
		t.Errorf("recordTTFB did not trigger TTFB for metadata-only ReadObjectResponse")
	}
}

func TestComputeErrorType(t *testing.T) {
	tests := []struct {
		err        error
		isHTTP     bool
		statusCode int64
		want       string
	}{
		{err: errors.New("dial tcp: no such host"), want: "DNS_FAILURE"},
		{err: errors.New("connection refused"), want: "CONNECTION_ERROR"},
		{err: errors.New("connection reset by peer"), want: "CONNECTION_ERROR"},
		{err: errors.New("tls: bad certificate"), want: "TLS_FAILURE"},
		{err: errors.New("unexpected eof"), want: "CONNECTION_ERROR"},
		{err: context.DeadlineExceeded, want: "TIMEOUT"},
		{err: context.Canceled, want: "CANCELLED"},
		{err: status.Error(codes.NotFound, "not found"), want: "NOT_FOUND"},
	}

	for _, tc := range tests {
		got := computeErrorType(tc.err, tc.isHTTP, tc.statusCode)
		if got != tc.want {
			t.Errorf("computeErrorType(%v, %v, %v) = %v, want %v", tc.err, tc.isHTTP, tc.statusCode, got, tc.want)
		}
	}
}

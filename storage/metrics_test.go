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
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	"go.opentelemetry.io/otel/attribute"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/metric/metricdata"
	"go.opentelemetry.io/otel/sdk/resource"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

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
		enableOtelMetrics: true,
		meterProvider:     provider,
	}

	sm, _, err := initMetrics(ctx, "project-id", &cfg)
	if err != nil {
		t.Fatalf("initMetrics: %v", err)
	}

	// Create a mock HTTP server to respond to requests.
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("hello world"))
	}))
	defer server.Close()

	client := &http.Client{
		Transport: &metricsRoundTripper{
			underlying: http.DefaultTransport,
			metrics:    sm,
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

				if attrMap["rpc.system"] != "http" {
					t.Errorf("expected rpc.system http, got %q", attrMap["rpc.system"])
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

func TestGRPCMetricsRecording(t *testing.T) {
	ctx := context.Background()
	mr := sdkmetric.NewManualReader()
	provider := sdkmetric.NewMeterProvider(sdkmetric.WithReader(mr))
	defer provider.Shutdown(ctx)

	cfg := storageConfig{
		enableOtelMetrics: true,
		meterProvider:     provider,
	}

	sm, _, err := initMetrics(ctx, "project-id", &cfg)
	if err != nil {
		t.Fatalf("initMetrics: %v", err)
	}

	// 1. Test Unary call.
	unaryInt, streamInt := metricsInterceptors(sm)

	invoker := func(ctx context.Context, method string, req, reply interface{}, cc *grpc.ClientConn, opts ...grpc.CallOption) error {
		return status.Error(codes.NotFound, "not found")
	}

	err = unaryInt(ctx, "/google.storage.v2.Storage/GetObject", nil, nil, nil, invoker)
	if err == nil || status.Code(err) != codes.NotFound {
		t.Errorf("unexpected unary error: %v", err)
	}

	// 2. Test Stream call.
	streamer := func(ctx context.Context, desc *grpc.StreamDesc, cc *grpc.ClientConn, method string, opts ...grpc.CallOption) (grpc.ClientStream, error) {
		return &mockClientStream{recvErr: io.EOF}, nil
	}

	clientStream, err := streamInt(ctx, nil, nil, "/google.storage.v2.Storage/ReadObject", streamer)
	if err != nil {
		t.Fatalf("streamInt: %v", err)
	}

	// Trigger stream termination by receiving EOF.
	if err := clientStream.RecvMsg(nil); err != io.EOF {
		t.Errorf("expected io.EOF, got %v", err)
	}

	// Collect metrics.
	var rm metricdata.ResourceMetrics
	if err := mr.Collect(ctx, &rm); err != nil {
		t.Fatalf("Collect: %v", err)
	}

	var unaryDp, streamDp *metricdata.HistogramDataPoint[float64]

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
					}
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
		if attrs["rpc.system"] != "grpc" {
			t.Errorf("expected rpc.system grpc, got %q", attrs["rpc.system"])
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
		t.Errorf("streaming metric not recorded")
	} else {
		attrs := make(map[string]string)
		for _, kv := range streamDp.Attributes.ToSlice() {
			attrs[string(kv.Key)] = kv.Value.Emit()
		}
		if attrs["rpc.system"] != "grpc" {
			t.Errorf("expected rpc.system grpc, got %q", attrs["rpc.system"])
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
}

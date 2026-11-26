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
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"net/http/httptrace"
	"net/url"
	"os"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"cloud.google.com/go/auth"
	gcemetadata "cloud.google.com/go/compute/metadata"
	"cloud.google.com/go/storage/internal"
	"cloud.google.com/go/storage/internal/apiv2/storagepb"
	"github.com/google/uuid"

	"go.opentelemetry.io/otel/attribute"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/metric/metricdata"
	"go.opentelemetry.io/otel/sdk/resource"
	"golang.org/x/oauth2"
	"golang.org/x/oauth2/google"
	"google.golang.org/api/googleapi"
	"google.golang.org/api/option"
	"google.golang.org/api/option/internaloption"
	"google.golang.org/api/transport"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/connectivity"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/mem"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/peer"
	"google.golang.org/grpc/stats"
	"google.golang.org/grpc/status"
	"google.golang.org/grpc/test/bufconn"
	"google.golang.org/protobuf/proto"
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
	ctx = context.WithValue(ctx, apiMethodKey{}, "storage.Object.Read")
	mr := sdkmetric.NewManualReader()

	// Create a resource with static attributes so that we can test resource propagation.
	res, err := resource.New(ctx,
		resource.WithAttributes(
			attribute.String("gcp.client.service", "storage"),
			attribute.String("gcp.client.artifact", "cloud.google.com/go/storage"),
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
	foundAttempts := false
	foundServerDuration := false
	foundTTFB := false
	foundActive := false
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

				assertExactAttrKeys(t, m.Name, dp.Attributes,
					"error.type", "http.request.method", "http.response.status_code", "network.path", "rpc.system.name", "server.address", "url.template")
				if got := getHistAttr(dp, "rpc.system.name"); got != "http" {
					t.Errorf("expected rpc.system.name http, got %q", got)
				}
				if got := getHistAttr(dp, "http.request.method"); got != "GET" {
					t.Errorf("expected GET, got %q", got)
				}
				if got := getHistAttr(dp, "url.template"); got != "/storage/v1/b/{bucket}/o/{object}" {
					t.Errorf("expected template /storage/v1/b/{bucket}/o/{object}, got %q", got)
				}
				if got := getHistAttr(dp, "http.response.status_code"); got != "200" {
					t.Errorf("expected status_code 200, got %q", got)
				}
				if got := getHistAttr(dp, "error.type"); got != "OK" {
					t.Errorf("expected error.type OK, got %q", got)
				}
				if got := getHistAttr(dp, "network.path"); got != networkPathCloud {
					t.Errorf("http.client.request.duration network.path = %q, want %q", got, networkPathCloud)
				}
			}

			if m.Name == "gcp.storage.client.attempts" {
				foundAttempts = true
				sum, ok := m.Data.(metricdata.Sum[int64])
				if !ok {
					t.Fatalf("expected Sum data for attempts, got %T", m.Data)
				}
				if len(sum.DataPoints) != 1 {
					t.Fatalf("expected 1 attempts datapoint, got %d", len(sum.DataPoints))
				}
				assertExactAttrKeys(t, m.Name, sum.DataPoints[0].Attributes,
					"error.type", "gcp.storage.operation", "http.response.status_code", "network.path", "rpc.system.name", "server.address")
				if got := getAttr(sum.DataPoints[0], "gcp.storage.operation"); got != "Unknown" {
					t.Errorf("attempts gcp.storage.operation = %q, want Unknown", got)
				}
				if got := getAttr(sum.DataPoints[0], "network.path"); got != networkPathCloud {
					t.Errorf("attempts network.path = %q, want %q", got, networkPathCloud)
				}
			}

			if m.Name == "gcp.storage.client.server.duration" {
				foundServerDuration = true
				hist, ok := m.Data.(metricdata.Histogram[float64])
				if ok && len(hist.DataPoints) > 0 {
					assertExactAttrKeys(t, m.Name, hist.DataPoints[0].Attributes,
						"gcp.storage.operation", "network.path", "rpc.system.name", "server.address")
					if hist.DataPoints[0].Sum != 0.15 {
						t.Errorf("expected server.duration 0.15s, got %v", hist.DataPoints[0].Sum)
					}
					if got := getHistAttr(hist.DataPoints[0], "gcp.storage.operation"); got != "Unknown" {
						t.Errorf("server.duration gcp.storage.operation = %q, want Unknown", got)
					}
					if got := getHistAttr(hist.DataPoints[0], "network.path"); got != networkPathCloud {
						t.Errorf("server.duration network.path = %q, want %q", got, networkPathCloud)
					}
				} else {
					t.Errorf("expected server.duration datapoints")
				}
			}

			if m.Name == "gcp.storage.client.ttfb" {
				foundTTFB = true
				hist, ok := m.Data.(metricdata.Histogram[float64])
				if !ok || len(hist.DataPoints) != 1 {
					t.Fatalf("ttfb: got %T (len=%d), want 1 Histogram datapoint", m.Data, len(hist.DataPoints))
				}
				assertExactAttrKeys(t, m.Name, hist.DataPoints[0].Attributes,
					"gcp.storage.operation", "network.path", "rpc.system.name", "server.address")
				if got := getHistAttr(hist.DataPoints[0], "network.path"); got != networkPathCloud {
					t.Errorf("ttfb network.path = %q, want %q", got, networkPathCloud)
				}
			}

			if m.Name == "gcp.storage.client.network.tcp.connect.duration" {
				hist, ok := m.Data.(metricdata.Histogram[float64])
				if !ok || len(hist.DataPoints) == 0 {
					t.Fatalf("tcp.connect.duration: got %T (len=%d), want Histogram datapoints", m.Data, len(hist.DataPoints))
				}
				assertExactAttrKeys(t, m.Name, hist.DataPoints[0].Attributes,
					"network.path", "rpc.system.name", "server.address")
				if got := getHistAttr(hist.DataPoints[0], "network.path"); got != networkPathCloud {
					t.Errorf("tcp.connect.duration network.path = %q, want %q", got, networkPathCloud)
				}
			}

			if m.Name == "gcp.storage.client.network.dns.lookup.duration" {
				hist, ok := m.Data.(metricdata.Histogram[float64])
				if ok && len(hist.DataPoints) > 0 {
					assertExactAttrKeys(t, m.Name, hist.DataPoints[0].Attributes,
						"rpc.system.name", "server.address")
				}
			}

			if m.Name == "gcp.storage.client.request.active" {
				foundActive = true
				sum, ok := m.Data.(metricdata.Sum[int64])
				if !ok || len(sum.DataPoints) != 1 {
					t.Fatalf("request.active: got %T (len=%d), want 1 Sum datapoint", m.Data, len(sum.DataPoints))
				}
				assertExactAttrKeys(t, m.Name, sum.DataPoints[0].Attributes,
					"gcp.storage.operation", "rpc.system.name", "server.address")
				if sum.DataPoints[0].Value != 0 {
					t.Errorf("request.active final value = %d, want 0", sum.DataPoints[0].Value)
				}
			}
		}
	}

	if !found {
		t.Errorf("metric http.client.request.duration not found")
	}
	if !foundAttempts {
		t.Errorf("metric gcp.storage.client.attempts not found")
	}
	if !foundServerDuration {
		t.Errorf("metric gcp.storage.client.server.duration not found")
	}
	if !foundTTFB {
		t.Errorf("metric gcp.storage.client.ttfb not found")
	}
	if !foundActive {
		t.Errorf("metric gcp.storage.client.request.active not found")
	}
}

type mockClientStream struct {
	grpc.ClientStream
	ctx     context.Context
	sendErr error
	recvErr error
}

func (m *mockClientStream) RecvMsg(msg interface{}) error {
	return m.recvErr
}

func (m *mockClientStream) SendMsg(msg interface{}) error {
	return m.sendErr
}

func (m *mockClientStream) Header() (metadata.MD, error) {
	return metadata.Pairs("x-goog-gfe-service-time", "120"), nil
}

func (m *mockClientStream) Trailer() metadata.MD {
	return nil
}

func (m *mockClientStream) Context() context.Context {
	if m.ctx != nil {
		return m.ctx
	}
	return context.Background()
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

	dpPeer := &peer.Peer{Addr: &net.TCPAddr{IP: net.ParseIP("34.126.0.10"), Port: 443}}
	cpPeerCtx := peer.NewContext(ctx, &peer.Peer{Addr: &net.TCPAddr{IP: net.ParseIP("142.250.1.1"), Port: 443}})

	invoker := func(ctx context.Context, method string, req, reply interface{}, cc *grpc.ClientConn, opts ...grpc.CallOption) error {
		for _, o := range opts {
			if p, ok := o.(grpc.PeerCallOption); ok {
				*p.PeerAddr = *dpPeer
			}
		}
		return status.Error(codes.NotFound, "not found")
	}

	err = unaryInt(ctx, "/google.storage.v2.Storage/GetObject", nil, nil, nil, invoker)
	if err == nil || status.Code(err) != codes.NotFound {
		t.Errorf("unexpected unary error: %v", err)
	}

	// Test Server-Streaming call (ReadObject).
	streamer := func(ctx context.Context, desc *grpc.StreamDesc, cc *grpc.ClientConn, method string, opts ...grpc.CallOption) (grpc.ClientStream, error) {
		return &mockClientStream{ctx: cpPeerCtx, recvErr: io.EOF}, nil
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
		return &mockClientStream{ctx: cpPeerCtx, recvErr: nil}, nil
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

	// Test Bidi-Streaming call (BidiWriteObject).
	descBidi := &grpc.StreamDesc{
		ServerStreams: true,
		ClientStreams: true,
	}
	streamerBidi := func(ctx context.Context, desc *grpc.StreamDesc, cc *grpc.ClientConn, method string, opts ...grpc.CallOption) (grpc.ClientStream, error) {
		return &mockClientStream{ctx: cpPeerCtx, recvErr: nil}, nil
	}
	clientStreamBidi, err := streamInt(ctx, descBidi, nil, "/google.storage.v2.Storage/BidiWriteObject", streamerBidi)
	if err != nil {
		t.Fatalf("streamInt: %v", err)
	}

	// 1st Bidi stream.
	clientStreamBidi.SendMsg(&storagepb.BidiWriteObjectRequest{FinishWrite: true})
	clientStreamBidi.RecvMsg(nil)

	// Terminate stream with a wrapped io.EOF to verify errors.Is(err, io.EOF) handling.
	clientStreamBidi.(*wrappedClientStream).record(fmt.Errorf("bidi stream done: %w", io.EOF))

	// Collect metrics.
	var rm metricdata.ResourceMetrics
	if err := mr.Collect(ctx, &rm); err != nil {
		t.Fatalf("Collect: %v", err)
	}

	rpcDps := make(map[string]metricdata.HistogramDataPoint[float64])
	var foundAttempts, foundErrors, foundServerDuration, foundTTFB, foundActive bool

	for _, sm := range rm.ScopeMetrics {
		for _, m := range sm.Metrics {
			switch m.Name {
			case "rpc.client.call.duration":
				hist, ok := m.Data.(metricdata.Histogram[float64])
				if !ok {
					t.Fatalf("expected Histogram data, got %T", m.Data)
				}
				for _, dp := range hist.DataPoints {
					assertExactAttrKeys(t, m.Name, dp.Attributes,
						"error.type", "network.path", "rpc.method", "rpc.response.status_code", "rpc.system.name", "server.address")
					rpcDps[getHistAttr(dp, "rpc.method")] = dp
				}

			case "gcp.storage.client.attempts":
				foundAttempts = true
				sum, ok := m.Data.(metricdata.Sum[int64])
				if !ok {
					t.Fatalf("attempts: got %T, want Sum[int64]", m.Data)
				}
				if len(sum.DataPoints) != 3 {
					t.Fatalf("expected 3 attempts datapoints, got %d", len(sum.DataPoints))
				}
				ops := make(map[string]int64)
				statusByOp := make(map[string]string)
				for _, dp := range sum.DataPoints {
					assertExactAttrKeys(t, m.Name, dp.Attributes,
						"error.type", "gcp.storage.operation", "network.path", "rpc.response.status_code", "rpc.system.name", "server.address")
					op := getAttr(dp, "gcp.storage.operation")
					ops[op] += dp.Value
					statusByOp[op] = getAttr(dp, "rpc.response.status_code")
				}
				if ops["GetObject"] != 1 || ops["ReadObject"] != 1 || ops["WriteObject"] != 2 {
					t.Errorf("unexpected attempts by gcp.storage.operation: %v", ops)
				}
				if statusByOp["GetObject"] != "NOT_FOUND" || statusByOp["ReadObject"] != "OK" || statusByOp["WriteObject"] != "OK" {
					t.Errorf("unexpected rpc.response.status_code on attempts: %v", statusByOp)
				}

			case "gcp.storage.client.errors":
				foundErrors = true
				sum, ok := m.Data.(metricdata.Sum[int64])
				if !ok {
					t.Fatalf("errors: got %T, want Sum[int64]", m.Data)
				}
				if len(sum.DataPoints) != 1 {
					t.Fatalf("expected 1 errors datapoint, got %d", len(sum.DataPoints))
				}
				assertExactAttrKeys(t, m.Name, sum.DataPoints[0].Attributes,
					"error.type", "gcp.storage.operation", "network.path", "rpc.system.name", "server.address")
				if got := getAttr(sum.DataPoints[0], "gcp.storage.operation"); got != "GetObject" {
					t.Errorf("errors gcp.storage.operation = %q, want GetObject", got)
				}

			case "gcp.storage.client.server.duration":
				foundServerDuration = true
				hist, ok := m.Data.(metricdata.Histogram[float64])
				if ok && len(hist.DataPoints) > 0 {
					foundVal := false
					for _, dp := range hist.DataPoints {
						assertExactAttrKeys(t, m.Name, dp.Attributes,
							"gcp.storage.operation", "network.path", "rpc.system.name", "server.address")
						if dp.Sum == 0.12 {
							foundVal = true
						}
						if got := getHistAttr(dp, "gcp.storage.operation"); got == "" {
							t.Errorf("expected gcp.storage.operation on server.duration")
						}
					}
					if !foundVal {
						t.Errorf("expected server.duration 0.12s from stream")
					}
				} else {
					t.Errorf("expected server.duration datapoints")
				}

			case "gcp.storage.client.ttfb":
				foundTTFB = true
				hist, ok := m.Data.(metricdata.Histogram[float64])
				if !ok || len(hist.DataPoints) == 0 {
					t.Fatalf("expected ttfb Histogram datapoints")
				}
				for _, dp := range hist.DataPoints {
					assertExactAttrKeys(t, m.Name, dp.Attributes,
						"gcp.storage.operation", "network.path", "rpc.system.name", "server.address")
					if got := getHistAttr(dp, "network.path"); got != networkPathCloud {
						t.Errorf("ttfb network.path = %q, want %q", got, networkPathCloud)
					}
				}

			case "gcp.storage.client.request.active":
				foundActive = true
				sum, ok := m.Data.(metricdata.Sum[int64])
				if !ok || len(sum.DataPoints) == 0 {
					t.Fatalf("expected request.active Sum datapoints")
				}
				for _, dp := range sum.DataPoints {
					assertExactAttrKeys(t, m.Name, dp.Attributes,
						"gcp.storage.operation", "rpc.system.name", "server.address")
					if dp.Value != 0 {
						t.Errorf("request.active (%s) = %d, want 0", getAttr(dp, "gcp.storage.operation"), dp.Value)
					}
				}
			}
		}
	}

	if !foundAttempts {
		t.Errorf("metric gcp.storage.client.attempts not found")
	}
	if !foundErrors {
		t.Errorf("metric gcp.storage.client.errors not found")
	}
	if !foundServerDuration {
		t.Errorf("metric gcp.storage.client.server.duration not found")
	}
	if !foundTTFB {
		t.Errorf("metric gcp.storage.client.ttfb not found")
	}
	if !foundActive {
		t.Errorf("metric gcp.storage.client.request.active not found")
	}

	for _, tc := range []struct {
		method     string
		wantStatus string
		wantPath   string
	}{
		{method: "google.storage.v2.Storage/GetObject", wantStatus: "NOT_FOUND", wantPath: networkPathDirect},
		{method: "google.storage.v2.Storage/ReadObject", wantStatus: "OK", wantPath: networkPathCloud},
		{method: "google.storage.v2.Storage/WriteObject", wantStatus: "OK", wantPath: networkPathCloud},
		{method: "google.storage.v2.Storage/BidiWriteObject", wantStatus: "OK", wantPath: networkPathCloud},
	} {
		dp, ok := rpcDps[tc.method]
		if !ok {
			t.Errorf("rpc.client.call.duration not recorded for %s", tc.method)
			continue
		}
		if dp.Count != 1 {
			t.Errorf("%s count = %d, want 1", tc.method, dp.Count)
		}
		if got := getHistAttr(dp, "rpc.system.name"); got != "grpc" {
			t.Errorf("%s rpc.system.name = %q, want grpc", tc.method, got)
		}
		if got := getHistAttr(dp, "rpc.response.status_code"); got != tc.wantStatus {
			t.Errorf("%s rpc.response.status_code = %q, want %q", tc.method, got, tc.wantStatus)
		}
		if got := getHistAttr(dp, "error.type"); got != tc.wantStatus {
			t.Errorf("%s error.type = %q, want %q", tc.method, got, tc.wantStatus)
		}
		if got := getHistAttr(dp, "network.path"); got != tc.wantPath {
			t.Errorf("%s network.path = %q, want %q", tc.method, got, tc.wantPath)
		}
	}
}

type mockStorageClient struct {
	storageClient
	getObjectFn  func(ctx context.Context, params *getObjectParams, opts ...storageOption) (*ObjectAttrs, error)
	newReaderFn  func(ctx context.Context, params *newRangeReaderParams, opts ...storageOption) (*Reader, error)
	newMRDFn     func(ctx context.Context, params *newMultiRangeDownloaderParams, opts ...storageOption) (*MultiRangeDownloader, error)
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

type mockInternalMRD struct {
	bytesRead int64
	ctx       context.Context
}

func (m *mockInternalMRD) add(output io.Writer, offset, length int64, callback func(int64, int64, error)) {
}
func (m *mockInternalMRD) close(err error) error       { return nil }
func (m *mockInternalMRD) wait()                       {}
func (m *mockInternalMRD) getHandle() []byte           { return nil }
func (m *mockInternalMRD) getPermanentError() error    { return nil }
func (m *mockInternalMRD) getSpanCtx() context.Context { return m.ctx }
func (m *mockInternalMRD) getBytesRead() int64         { return m.bytesRead }

func (m *mockStorageClient) NewMultiRangeDownloader(ctx context.Context, params *newMultiRangeDownloaderParams, opts ...storageOption) (*MultiRangeDownloader, error) {
	if m.newMRDFn != nil {
		return m.newMRDFn(ctx, params, opts...)
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

	// Test NewRangeReader directly (distinguishing Object.NewRangeReader from Object.NewReader).
	rr, err := client.Bucket("my-bucket").Object("my-object").NewRangeReader(ctx, 0, 5)
	if err != nil {
		t.Fatalf("NewRangeReader: %v", err)
	}
	if _, err := rr.Read(buf); err != nil && err != io.EOF {
		t.Fatalf("Range Read: %v", err)
	}
	rr.Close()

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

	// Test MultiRangeDownloader (exercising the real stream interceptor for BidiReadObject
	// when closed before RecvMsg completes).
	_, streamInterceptor := metricsInterceptors(cm)
	mock.newMRDFn = func(ctx context.Context, params *newMultiRangeDownloaderParams, opts ...storageOption) (*MultiRangeDownloader, error) {
		_, err := streamInterceptor(ctx, &grpc.StreamDesc{ServerStreams: true, ClientStreams: true}, nil, "/google.storage.v2.Storage/BidiReadObject", func(ctx context.Context, desc *grpc.StreamDesc, cc *grpc.ClientConn, method string, opts ...grpc.CallOption) (grpc.ClientStream, error) {
			return &mockClientStream{}, nil
		})
		if err != nil {
			return nil, err
		}
		return &MultiRangeDownloader{
			impl: &mockInternalMRD{bytesRead: 42, ctx: ctx},
		}, nil
	}
	mrd, err := client.Bucket("my-bucket").Object("my-object").NewMultiRangeDownloader(ctx)
	if err != nil {
		t.Fatalf("NewMultiRangeDownloader: %v", err)
	}
	if err := mrd.Close(); err != nil {
		t.Fatalf("mrd.Close: %v", err)
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
		hist, ok := m.Data.(metricdata.Histogram[float64])
		if !ok {
			t.Fatalf("gcp.client.request.duration: got %T, want Histogram[float64]", m.Data)
		}
		if len(hist.DataPoints) != 5 {
			t.Errorf("expected 5 datapoints for gcp.client.request.duration, got %d", len(hist.DataPoints))
		}
		ops := make(map[string]bool)
		methods := make(map[string]bool)
		clientMethods := make(map[string]bool)
		for _, dp := range hist.DataPoints {
			assertExactAttrKeys(t, m.Name, dp.Attributes,
				"error.type", "gcp.client.method", "gcp.storage.operation", "rpc.method", "rpc.system.name", "server.address")
			ops[getHistAttr(dp, "gcp.storage.operation")] = true
			methods[getHistAttr(dp, "rpc.method")] = true
			clientMethods[getHistAttr(dp, "gcp.client.method")] = true
		}
		if !ops["GetObject"] || !ops["ReadObject"] || !ops["WriteObject"] {
			t.Errorf("expected gcp.storage.operation GetObject, ReadObject, WriteObject, got %v", ops)
		}
		if !methods["google.storage.v2.Storage/GetObject"] || !methods["google.storage.v2.Storage/ReadObject"] || !methods["google.storage.v2.Storage/BidiReadObject"] || !methods["google.storage.v2.Storage/WriteObject"] {
			t.Errorf("expected fully-qualified rpc.method (including BidiReadObject) on gcp.client.request.duration, got %v", methods)
		}
		if !clientMethods["Object.Attrs"] || !clientMethods["Object.NewReader"] || !clientMethods["Object.NewRangeReader"] || !clientMethods["Object.NewWriter"] || !clientMethods["Object.NewMultiRangeDownloader"] {
			t.Errorf("expected gcp.client.method on gcp.client.request.duration, got %v", clientMethods)
		}
	}

	// Check gcp.storage.client.operations.
	if m, ok := metricsMap["gcp.storage.client.operations"]; !ok {
		t.Errorf("metric gcp.storage.client.operations not found")
	} else {
		sum, ok := m.Data.(metricdata.Sum[int64])
		if !ok {
			t.Fatalf("gcp.storage.client.operations: got %T, want Sum[int64]", m.Data)
		}
		if len(sum.DataPoints) != 5 {
			t.Errorf("expected 5 datapoints for gcp.storage.client.operations, got %d", len(sum.DataPoints))
		}
		for _, dp := range sum.DataPoints {
			assertExactAttrKeys(t, m.Name, dp.Attributes,
				"error.type", "gcp.client.method", "gcp.storage.operation", "rpc.system.name", "server.address")
			if got := getAttr(dp, "gcp.storage.operation"); got == "" {
				t.Errorf("expected gcp.storage.operation on gcp.storage.client.operations")
			}
			if got := getAttr(dp, "gcp.client.method"); got == "" {
				t.Errorf("expected gcp.client.method on gcp.storage.client.operations")
			}
		}
	}

	// Check gcp.storage.client.response.body.size.
	if m, ok := metricsMap["gcp.storage.client.response.body.size"]; !ok {
		t.Errorf("metric gcp.storage.client.response.body.size not found")
	} else {
		hist, ok := m.Data.(metricdata.Histogram[int64])
		if !ok {
			t.Fatalf("response.body.size: got %T, want Histogram[int64]", m.Data)
		}
		if len(hist.DataPoints) != 3 {
			t.Fatalf("expected 3 datapoints for response body size (NewReader, NewRangeReader, and MRD), got %d", len(hist.DataPoints))
		}
		var totalSum int64
		for _, dp := range hist.DataPoints {
			totalSum += dp.Sum
			assertExactAttrKeys(t, m.Name, dp.Attributes,
				"gcp.client.method", "gcp.storage.operation", "rpc.system.name", "server.address")
		}
		if totalSum != 52 {
			t.Errorf("expected total sum 52, got %d", totalSum)
		}
	}

	// Check gcp.storage.client.request.body.size.
	if m, ok := metricsMap["gcp.storage.client.request.body.size"]; !ok {
		t.Errorf("metric gcp.storage.client.request.body.size not found")
	} else {
		hist, ok := m.Data.(metricdata.Histogram[int64])
		if !ok {
			t.Fatalf("request.body.size: got %T, want Histogram[int64]", m.Data)
		}
		if len(hist.DataPoints) != 1 {
			t.Fatalf("expected 1 datapoint for request body size, got %d", len(hist.DataPoints))
		}
		dp := hist.DataPoints[0]
		if dp.Sum != 11 {
			t.Errorf("expected sum 11, got %d", dp.Sum)
		}
		assertExactAttrKeys(t, m.Name, dp.Attributes,
			"gcp.client.method", "gcp.storage.operation", "rpc.system.name", "server.address")
		if v, _ := dp.Attributes.Value("gcp.storage.operation"); v.AsString() != "WriteObject" {
			t.Errorf("expected gcp.storage.operation WriteObject, got %q", v.AsString())
		}
		if v, _ := dp.Attributes.Value("gcp.client.method"); v.AsString() != "Object.NewWriter" {
			t.Errorf("expected gcp.client.method Object.NewWriter, got %q", v.AsString())
		}
	}
}

func TestGetLogicalMethod(t *testing.T) {
	var nilState *metricsState
	nilState.setRPCMethod("google.storage.v2.Storage/GetObject") // nil receiver must be a no-op

	for _, tc := range []struct {
		in   string
		want string
	}{
		{in: "GetObject", want: "GetObject"},
		{in: "/google.storage.v2.Storage/GetObject", want: "GetObject"},
		{in: "/google.storage.v2.Storage/BidiReadObject", want: "ReadObject"},
		{in: "/google.storage.v2.Storage/BidiWriteObject", want: "WriteObject"},
		{in: "/google.storage.v2.Storage/StartResumableWrite", want: "WriteObject"},
		{in: "/google.storage.v2.Storage/QueryWriteStatus", want: "WriteObject"},
		{in: "/google.storage.v2.Storage/CancelResumableWrite", want: "WriteObject"},
	} {
		if got := getLogicalMethod(tc.in); got != tc.want {
			t.Errorf("getLogicalMethod(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}

func TestOperationDuration_HTTPRPCMethodFallback(t *testing.T) {
	ctx := context.Background()
	mr := sdkmetric.NewManualReader()
	provider := sdkmetric.NewMeterProvider(sdkmetric.WithReader(mr))
	defer provider.Shutdown(ctx)

	cm, _, err := initMetrics(ctx, "project-id", &storageConfig{
		enableOtelMetrics: true,
		meterProvider:     provider,
	})
	if err != nil {
		t.Fatalf("initMetrics: %v", err)
	}

	// An HTTP operation without an operation span falls back to the v2-qualified
	// rpc.method and defaultClientMethods for gcp.client.method.
	_, record := cm.startOperation(ctx, "GetHMACKey", true)
	record(nil)

	var rm metricdata.ResourceMetrics
	if err := mr.Collect(ctx, &rm); err != nil {
		t.Fatalf("Collect: %v", err)
	}
	found := false
	for _, sm := range rm.ScopeMetrics {
		for _, m := range sm.Metrics {
			if m.Name != "gcp.client.request.duration" {
				continue
			}
			found = true
			hist, ok := m.Data.(metricdata.Histogram[float64])
			if !ok {
				t.Fatalf("expected Histogram data, got %T", m.Data)
			}
			if len(hist.DataPoints) != 1 {
				t.Fatalf("expected 1 datapoint, got %d", len(hist.DataPoints))
			}
			dp := hist.DataPoints[0]
			assertExactAttrKeys(t, m.Name, dp.Attributes,
				"error.type", "gcp.client.method", "gcp.storage.operation", "rpc.method", "rpc.system.name", "server.address")
			for key, want := range map[string]string{
				"rpc.system.name":       "http",
				"gcp.storage.operation": "GetHMACKey",
				"gcp.client.method":     "HMACKey.Get",
				"rpc.method":            "google.storage.v2.Storage/GetHMACKey",
			} {
				if got := getHistAttr(dp, key); got != want {
					t.Errorf("%s = %q, want %q", key, got, want)
				}
			}
		}
	}
	if !found {
		t.Errorf("metric gcp.client.request.duration not found")
	}
}

func TestRecordTTFB_MetadataOnly(t *testing.T) {
	for _, method := range []string{
		"/google.storage.v2.Storage/ReadObject",
		"/google.storage.v2.Storage/BidiReadObject",
	} {
		t.Run(method, func(t *testing.T) {
			ctx := context.Background()
			mr := sdkmetric.NewManualReader()
			provider := sdkmetric.NewMeterProvider(sdkmetric.WithReader(mr))
			defer provider.Shutdown(ctx)

			cm, _, err := initMetrics(ctx, "project-id", &storageConfig{
				enableOtelMetrics: true,
				meterProvider:     provider,
			})
			if err != nil {
				t.Fatalf("initMetrics: %v", err)
			}

			w := &wrappedClientStream{
				metrics:   cm,
				ctx:       ctx,
				method:    method,
				startTime: time.Now(),
			}

			// First response with only metadata should trigger TTFB.
			w.maybeRecordTTFB()

			if !w.recordedTTFB.Load() {
				t.Errorf("maybeRecordTTFB did not trigger TTFB for metadata-only ReadObjectResponse")
			}

			var rm metricdata.ResourceMetrics
			if err := mr.Collect(ctx, &rm); err != nil {
				t.Fatalf("Collect: %v", err)
			}
			foundTTFB := false
			for _, sm := range rm.ScopeMetrics {
				for _, m := range sm.Metrics {
					if m.Name == "gcp.storage.client.ttfb" {
						hist, ok := m.Data.(metricdata.Histogram[float64])
						if !ok {
							t.Fatalf("ttfb: got %T, want Histogram[float64]", m.Data)
						}
						if len(hist.DataPoints) == 1 {
							foundTTFB = true
							assertExactAttrKeys(t, m.Name, hist.DataPoints[0].Attributes,
								"gcp.storage.operation", "network.path", "rpc.system.name", "server.address")
							if got := getHistAttr(hist.DataPoints[0], "gcp.storage.operation"); got != "ReadObject" {
								t.Errorf("ttfb gcp.storage.operation = %q, want ReadObject", got)
							}
						}
					}
				}
			}
			if !foundTTFB {
				t.Errorf("expected gcp.storage.client.ttfb datapoint")
			}
		})
	}
}

type concurrentFlushMockStream struct {
	mockClientStream
	onFlushSend func()
	sendErr     error
}

func (c *concurrentFlushMockStream) SendMsg(msg interface{}) error {
	if req, ok := msg.(*storagepb.BidiWriteObjectRequest); ok && (req.GetFinishWrite() || req.GetStateLookup()) && c.onFlushSend != nil {
		fn := c.onFlushSend
		c.onFlushSend = nil
		fn()
	}
	return c.sendErr
}

func (c *concurrentFlushMockStream) CloseSend() error {
	return nil
}

func TestTTFB_ClientObservedReadsAndWrites(t *testing.T) {
	setup := func(t *testing.T) (context.Context, *sdkmetric.ManualReader, *clientMetrics) {
		t.Helper()
		ctx := context.Background()
		mr := sdkmetric.NewManualReader()
		provider := sdkmetric.NewMeterProvider(sdkmetric.WithReader(mr))
		t.Cleanup(func() { _ = provider.Shutdown(ctx) })

		cm, _, err := initMetrics(ctx, "project-id", &storageConfig{
			enableOtelMetrics: true,
			meterProvider:     provider,
		})
		if err != nil {
			t.Fatalf("initMetrics: %v", err)
		}
		return ctx, mr, cm
	}

	collectTTFB := func(t *testing.T, ctx context.Context, mr *sdkmetric.ManualReader) []metricdata.HistogramDataPoint[float64] {
		t.Helper()
		var rm metricdata.ResourceMetrics
		if err := mr.Collect(ctx, &rm); err != nil {
			t.Fatalf("Collect: %v", err)
		}
		for _, sm := range rm.ScopeMetrics {
			for _, m := range sm.Metrics {
				if m.Name == "gcp.storage.client.ttfb" {
					return m.Data.(metricdata.Histogram[float64]).DataPoints
				}
			}
		}
		return nil
	}

	newWriteStream := func(ctx context.Context, cm *clientMetrics, method string, cs grpc.ClientStream, start time.Time) *wrappedClientStream {
		return &wrappedClientStream{
			ClientStream:  cs,
			metrics:       cm,
			ctx:           ctx,
			method:        method,
			isWrite:       isWriteStreamMethod(method),
			target:        "storage.googleapis.com",
			startTime:     start,
			serverStreams: method == "/google.storage.v2.Storage/BidiWriteObject",
			clientStreams: true,
		}
	}

	t.Run("HTTP download records TTFB before body Read", func(t *testing.T) {
		ctx, mr, cm := setup(t)

		releaseBody := make(chan struct{})
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
			if f, ok := w.(http.Flusher); ok {
				f.Flush()
			}
			<-releaseBody
			_, _ = w.Write([]byte("hello"))
		}))
		defer server.Close()

		hc := &http.Client{Transport: &metricsRoundTripper{base: http.DefaultTransport, metrics: cm}}
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, server.URL+"/storage/v1/b/bkt/o/obj?alt=media", nil)
		if err != nil {
			t.Fatalf("NewRequest: %v", err)
		}
		resp, err := hc.Do(req)
		if err != nil {
			close(releaseBody)
			t.Fatalf("Do: %v", err)
		}

		// TTFB must already be recorded upon receiving response headers, before Read() is called.
		dps := collectTTFB(t, ctx, mr)
		if len(dps) != 1 || dps[0].Count != 1 {
			close(releaseBody)
			_ = resp.Body.Close()
			t.Fatalf("expected 1 ttfb sample before resp.Body.Read(), got %+v", dps)
		}

		close(releaseBody)
		if _, err := io.ReadAll(resp.Body); err != nil {
			t.Fatalf("ReadAll: %v", err)
		}
		_ = resp.Body.Close()

		if dpsAfter := collectTTFB(t, ctx, mr); len(dpsAfter) != 1 || dpsAfter[0].Count != 1 {
			t.Fatalf("expected ttfb count to remain 1 after body Close(), got %+v", dpsAfter)
		}
	})

	t.Run("HTTP upload measures from WroteRequest to GotFirstResponseByte", func(t *testing.T) {
		ctx, mr, cm := setup(t)

		gotFirstChunk := make(chan struct{})
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			buf := make([]byte, 7)
			if _, err := io.ReadFull(r.Body, buf); err == nil {
				close(gotFirstChunk)
			}
			_, _ = io.ReadAll(r.Body)
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{"name":"obj"}`))
		}))
		defer server.Close()

		pr, pw := io.Pipe()
		const uploadInterval = 50 * time.Millisecond
		go func() {
			_, _ = pw.Write([]byte("chunk-1"))
			<-gotFirstChunk
			<-time.After(uploadInterval)
			_, _ = pw.Write([]byte("chunk-2"))
			_ = pw.Close()
		}()

		hc := &http.Client{Transport: &metricsRoundTripper{base: http.DefaultTransport, metrics: cm}}
		req, err := http.NewRequestWithContext(ctx, http.MethodPut, server.URL+"/upload/storage/v1/b/bkt/o?uploadType=resumable&upload_id=123", pr)
		if err != nil {
			t.Fatalf("NewRequest: %v", err)
		}
		beforeDo := time.Now()
		resp, err := hc.Do(req)
		totalDo := time.Since(beforeDo).Seconds()
		if err != nil {
			t.Fatalf("Do: %v", err)
		}
		_, _ = io.ReadAll(resp.Body)
		_ = resp.Body.Close()

		dps := collectTTFB(t, ctx, mr)
		if len(dps) != 1 || dps[0].Count != 1 {
			t.Fatalf("expected 1 ttfb datapoint, got %+v", dps)
		}
		// Since WroteRequest occurs after uploadInterval elapses past startTime,
		// totalDo - ttfb >= uploadInterval regardless of scheduler jitter.
		if totalDo-dps[0].Sum < 0.045 {
			t.Errorf("HTTP upload ttfb = %vs out of totalDo = %vs; expected upload streaming interval (>= 45ms) to be excluded", dps[0].Sum, totalDo)
		}
	})

	t.Run("HTTP error response records TTFB while failed round trip omits TTFB", func(t *testing.T) {
		ctx, mr, cm := setup(t)

		hcFail := &http.Client{Transport: &metricsRoundTripper{base: roundTripFunc(func(r *http.Request) (*http.Response, error) {
			return nil, &url.Error{Op: "Get", URL: r.URL.String(), Err: errors.New("connection refused")}
		}), metrics: cm}}
		reqFail, _ := http.NewRequestWithContext(ctx, http.MethodGet, "https://storage.googleapis.com/storage/v1/b/bkt/o/obj", nil)
		_, _ = hcFail.Do(reqFail)

		if dps := collectTTFB(t, ctx, mr); len(dps) != 0 {
			t.Fatalf("expected 0 ttfb datapoints on failed HTTP round trip, got %+v", dps)
		}

		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusNotFound)
			_, _ = w.Write([]byte(`{"error":{"code":404}}`))
		}))
		defer server.Close()

		hc404 := &http.Client{Transport: &metricsRoundTripper{base: http.DefaultTransport, metrics: cm}}
		req404, _ := http.NewRequestWithContext(ctx, http.MethodGet, server.URL+"/storage/v1/b/bkt/o/missing", nil)
		resp, err := hc404.Do(req404)
		if err != nil {
			t.Fatalf("Do(404): %v", err)
		}
		_ = resp.Body.Close()

		if dps := collectTTFB(t, ctx, mr); len(dps) != 1 || dps[0].Count != 1 {
			t.Fatalf("expected 1 ttfb datapoint on HTTP 404 response, got %+v", dps)
		}
	})

	t.Run("HTTP trace hook edge cases", func(t *testing.T) {
		const (
			uploadURL   = "https://storage.googleapis.com/upload/storage/v1/b/bkt/o?uploadType=resumable&upload_id=123"
			downloadURL = "https://storage.googleapis.com/storage/v1/b/bkt/o/obj?alt=media"
			hookDelay   = 50 * time.Millisecond
		)
		for _, tc := range []struct {
			name        string
			method      string
			url         string
			body        []byte
			statusCode  int
			fire        func(*httptrace.ClientTrace)
			wantCount   uint64
			wantZeroSum bool
			wantMinSum  float64
			wantMaxGap  float64 // require totalElapsed - ttfb >= wantMaxGap when > 0
		}{
			{
				name:       "WroteRequest defer trails GotFirstResponseByte clamps to 0",
				method:     http.MethodPut,
				url:        uploadURL,
				body:       []byte("payload"),
				statusCode: http.StatusOK,
				fire: func(tr *httptrace.ClientTrace) {
					tr.GotFirstResponseByte()
					tr.WroteRequest(httptrace.WroteRequestInfo{})
				},
				wantCount:   1,
				wantZeroSum: true,
			},
			{
				name:       "first byte before body was written skips sample",
				method:     http.MethodPut,
				url:        uploadURL,
				body:       []byte("payload"),
				statusCode: http.StatusOK,
				fire: func(tr *httptrace.ClientTrace) {
					tr.GotFirstResponseByte()
				},
				wantCount: 0,
			},
			{
				name:       "failed body write clears earlier WroteRequest anchor",
				method:     http.MethodPut,
				url:        uploadURL,
				body:       []byte("payload"),
				statusCode: http.StatusOK,
				fire: func(tr *httptrace.ClientTrace) {
					tr.WroteRequest(httptrace.WroteRequestInfo{})
					tr.GotFirstResponseByte()
					tr.WroteRequest(httptrace.WroteRequestInfo{Err: errors.New("write: broken pipe")})
				},
				wantCount: 0,
			},
			{
				name:       "transport without trace hooks skips upload sample",
				method:     http.MethodPut,
				url:        uploadURL,
				body:       []byte("payload"),
				statusCode: http.StatusOK,
				wantCount:  0,
			},
			{
				name:       "GetConn clears prior hop trace timestamps before early upload rejection",
				method:     http.MethodPut,
				url:        uploadURL,
				body:       []byte("payload"),
				statusCode: http.StatusPreconditionFailed,
				fire: func(tr *httptrace.ClientTrace) {
					tr.GetConn("oauth2.googleapis.com:443")
					tr.WroteRequest(httptrace.WroteRequestInfo{})
					tr.GotFirstResponseByte()
					tr.GetConn("storage.googleapis.com:443")
					tr.GotFirstResponseByte()
				},
				wantCount: 0,
			},
			{
				name:       "GetConn anchors read TTFB after auth token refresh",
				method:     http.MethodGet,
				url:        downloadURL,
				statusCode: http.StatusOK,
				fire: func(tr *httptrace.ClientTrace) {
					time.Sleep(hookDelay)
					tr.GetConn("storage.googleapis.com:443")
					tr.GotFirstResponseByte()
				},
				wantCount:  1,
				wantMaxGap: 0.045,
			},
			{
				name:       "GetConn clears prior hop firstByteTime on read",
				method:     http.MethodGet,
				url:        downloadURL,
				statusCode: http.StatusOK,
				fire: func(tr *httptrace.ClientTrace) {
					tr.GetConn("oauth2.googleapis.com:443")
					tr.GotFirstResponseByte()
					time.Sleep(hookDelay)
					tr.GetConn("storage.googleapis.com:443")
					time.Sleep(hookDelay)
				},
				wantCount:  1,
				wantMinSum: 0.045,
			},
		} {
			t.Run(tc.name, func(t *testing.T) {
				ctx, mr, cm := setup(t)
				hc := &http.Client{Transport: &metricsRoundTripper{base: roundTripFunc(func(r *http.Request) (*http.Response, error) {
					if trace := httptrace.ContextClientTrace(r.Context()); trace != nil && tc.fire != nil {
						tc.fire(trace)
					}
					return &http.Response{StatusCode: tc.statusCode, Body: io.NopCloser(bytes.NewReader(nil)), Header: make(http.Header)}, nil
				}), metrics: cm}}
				var body io.Reader
				if tc.body != nil {
					body = bytes.NewReader(tc.body)
				}
				req, err := http.NewRequestWithContext(ctx, tc.method, tc.url, body)
				if err != nil {
					t.Fatalf("NewRequest: %v", err)
				}
				beforeDo := time.Now()
				resp, err := hc.Do(req)
				totalElapsed := time.Since(beforeDo).Seconds()
				if err != nil {
					t.Fatalf("Do: %v", err)
				}
				_ = resp.Body.Close()

				dps := collectTTFB(t, ctx, mr)
				var got uint64
				for _, dp := range dps {
					got += dp.Count
				}
				if got != tc.wantCount {
					t.Fatalf("ttfb sample count = %d, want %d", got, tc.wantCount)
				}
				if tc.wantZeroSum && dps[0].Sum != 0 {
					t.Errorf("expected clamped ttfb = 0s, got %vs", dps[0].Sum)
				}
				if tc.wantMinSum > 0 && dps[0].Sum < tc.wantMinSum {
					t.Errorf("ttfb = %vs, want >= %vs", dps[0].Sum, tc.wantMinSum)
				}
				if tc.wantMaxGap > 0 && totalElapsed-dps[0].Sum < tc.wantMaxGap {
					t.Errorf("ttfb = %vs with totalElapsed = %vs; expected delay >= %vs to be excluded", dps[0].Sum, totalElapsed, tc.wantMaxGap)
				}
			})
		}
	})

	t.Run("gRPC BidiWriteObject pipelined flush does not overwrite the first flush anchor", func(t *testing.T) {
		ctx, mr, cm := setup(t)
		w := newWriteStream(ctx, cm, "/google.storage.v2.Storage/BidiWriteObject", &concurrentFlushMockStream{}, time.Now())

		if err := w.SendMsg(&storagepb.BidiWriteObjectRequest{WriteOffset: 0, StateLookup: true}); err != nil {
			t.Fatalf("SendMsg first flush: %v", err)
		}
		firstAnchor := w.firstFlushSentAt.Load()
		if firstAnchor == nil {
			t.Fatalf("expected firstFlushSentAt to be set on first flush")
		}
		// Both flushes are sent before the first response is received.
		if err := w.SendMsg(&storagepb.BidiWriteObjectRequest{WriteOffset: 1024, FinishWrite: true}); err != nil {
			t.Fatalf("SendMsg second flush: %v", err)
		}
		if err := w.CloseSend(); err != nil {
			t.Fatalf("CloseSend: %v", err)
		}
		if got := w.firstFlushSentAt.Load(); got != firstAnchor {
			t.Errorf("pipelined FinishWrite or CloseSend overwrote the first flush anchor")
		}
		if err := w.RecvMsg(&storagepb.BidiWriteObjectResponse{}); err != nil {
			t.Fatalf("RecvMsg: %v", err)
		}
		if dps := collectTTFB(t, ctx, mr); len(dps) != 1 || dps[0].Count != 1 {
			t.Fatalf("expected 1 ttfb datapoint, got %+v", dps)
		}
	})

	t.Run("gRPC BidiWriteObject anchors first flush when RecvMsg runs during SendMsg", func(t *testing.T) {
		ctx, mr, cm := setup(t)

		recvReady := make(chan struct{})
		recvDone := make(chan error, 1)
		mockStream := &concurrentFlushMockStream{
			onFlushSend: func() {
				// Unblock RecvMsg inside ClientStream.SendMsg before SendMsg returns
				// so the anchor must already be stamped.
				close(recvReady)
				<-recvDone
			},
		}
		w := newWriteStream(ctx, cm, "/google.storage.v2.Storage/BidiWriteObject", mockStream, time.Now().Add(-2*time.Second))

		if err := w.SendMsg(&storagepb.BidiWriteObjectRequest{WriteOffset: 0}); err != nil {
			t.Fatalf("SendMsg non-flush: %v", err)
		}
		if w.firstFlushSentAt.Load() != nil {
			t.Fatalf("expected firstFlushSentAt to remain nil after non-flush frame")
		}

		go func() {
			<-recvReady
			recvDone <- w.RecvMsg(&storagepb.BidiWriteObjectResponse{})
		}()

		beforeFlush := time.Now()
		if err := w.SendMsg(&storagepb.BidiWriteObjectRequest{WriteOffset: 1024, StateLookup: true}); err != nil {
			t.Fatalf("SendMsg first flush: %v", err)
		}
		dps := collectTTFB(t, ctx, mr)
		if len(dps) != 1 || dps[0].Count != 1 {
			t.Fatalf("expected 1 ttfb datapoint, got %+v", dps)
		}
		if maxExpected := time.Since(beforeFlush).Seconds(); dps[0].Sum > maxExpected || dps[0].Sum >= 1.0 {
			t.Errorf("BidiWriteObject ttfb = %vs, want <= %vs (and < 1.0s)", dps[0].Sum, maxExpected)
		}
	})

	t.Run("gRPC BidiWriteObject re-stamps flush anchor after slow SendMsg", func(t *testing.T) {
		ctx, mr, cm := setup(t)
		const sendDelay = 50 * time.Millisecond
		mockStream := &concurrentFlushMockStream{
			onFlushSend: func() {
				time.Sleep(sendDelay)
			},
		}
		w := newWriteStream(ctx, cm, "/google.storage.v2.Storage/BidiWriteObject", mockStream, time.Now().Add(-2*time.Second))

		beforeSend := time.Now()
		if err := w.SendMsg(&storagepb.BidiWriteObjectRequest{WriteOffset: 1024, StateLookup: true}); err != nil {
			t.Fatalf("SendMsg: %v", err)
		}
		if err := w.RecvMsg(&storagepb.BidiWriteObjectResponse{}); err != nil {
			t.Fatalf("RecvMsg: %v", err)
		}
		totalElapsed := time.Since(beforeSend).Seconds()

		dps := collectTTFB(t, ctx, mr)
		if len(dps) != 1 || dps[0].Count != 1 {
			t.Fatalf("expected 1 ttfb datapoint, got %+v", dps)
		}
		if totalElapsed-dps[0].Sum < 0.045 {
			t.Errorf("BidiWriteObject ttfb = %vs with totalElapsed = %vs; expected SendMsg delay >= 0.045s to be excluded", dps[0].Sum, totalElapsed)
		}
	})

	t.Run("gRPC BidiWriteObject clears flush anchor when SendMsg fails", func(t *testing.T) {
		ctx, _, cm := setup(t)
		sendErr := errors.New("send failed")
		w := newWriteStream(ctx, cm, "/google.storage.v2.Storage/BidiWriteObject", &concurrentFlushMockStream{sendErr: sendErr}, time.Now())

		if err := w.SendMsg(&storagepb.BidiWriteObjectRequest{WriteOffset: 0, StateLookup: true}); !errors.Is(err, sendErr) {
			t.Fatalf("SendMsg err = %v, want %v", err, sendErr)
		}
		if got := w.firstFlushSentAt.Load(); got != nil {
			t.Errorf("expected firstFlushSentAt to be cleared on send error, got %v", *got)
		}
	})

	t.Run("gRPC BidiWriteObject append takeover with typed-nil Data anchors TTFB", func(t *testing.T) {
		ctx, mr, cm := setup(t)

		// Construct the takeover probe via bidiWriteObjectRequest with nil buf so
		// req.Data holds a typed-nil *BidiWriteObjectRequest_ChecksummedData.
		probe := bidiWriteObjectRequest(gRPCBidiWriteRequest{}, nil, nil)
		proto.Merge(probe, &storagepb.BidiWriteObjectRequest{
			FirstMessage: &storagepb.BidiWriteObjectRequest_AppendObjectSpec{
				AppendObjectSpec: &storagepb.AppendObjectSpec{Bucket: "projects/_/buckets/bkt", Object: "obj", Generation: 42},
			},
		})
		if !isWriteFlushMsg(probe) {
			t.Fatalf("expected isWriteFlushMsg(probe) = true for append-takeover probe with typed-nil Data")
		}

		w := newWriteStream(ctx, cm, "/google.storage.v2.Storage/BidiWriteObject", &concurrentFlushMockStream{}, time.Now().Add(-2*time.Second))
		beforeProbe := time.Now()
		if err := w.SendMsg(probe); err != nil {
			t.Fatalf("SendMsg(probe): %v", err)
		}
		if err := w.RecvMsg(&storagepb.BidiWriteObjectResponse{}); err != nil {
			t.Fatalf("RecvMsg: %v", err)
		}

		dps := collectTTFB(t, ctx, mr)
		if len(dps) != 1 || dps[0].Count != 1 {
			t.Fatalf("expected 1 ttfb datapoint, got %+v", dps)
		}
		if maxExpected := time.Since(beforeProbe).Seconds(); dps[0].Sum > maxExpected || dps[0].Sum >= 1.0 {
			t.Errorf("append takeover ttfb = %vs, want <= %vs (and < 1.0s)", dps[0].Sum, maxExpected)
		}
	})

	t.Run("gRPC WriteObject CloseSend anchors TTFB when no prior FinishWrite was sent", func(t *testing.T) {
		ctx, mr, cm := setup(t)
		w := newWriteStream(ctx, cm, "/google.storage.v2.Storage/WriteObject", &concurrentFlushMockStream{}, time.Now().Add(-2*time.Second))

		if err := w.SendMsg(&storagepb.WriteObjectRequest{WriteOffset: 0}); err != nil {
			t.Fatalf("SendMsg: %v", err)
		}
		beforeClose := time.Now()
		if err := w.CloseSend(); err != nil {
			t.Fatalf("CloseSend: %v", err)
		}
		if err := w.RecvMsg(&storagepb.WriteObjectResponse{}); err != nil {
			t.Fatalf("RecvMsg: %v", err)
		}

		dps := collectTTFB(t, ctx, mr)
		if len(dps) != 1 || dps[0].Count != 1 {
			t.Fatalf("expected 1 ttfb datapoint, got %+v", dps)
		}
		if maxExpected := time.Since(beforeClose).Seconds(); dps[0].Sum > maxExpected || dps[0].Sum >= 1.0 {
			t.Errorf("WriteObject CloseSend ttfb = %vs, want <= %vs (and < 1.0s)", dps[0].Sum, maxExpected)
		}
	})

	t.Run("gRPC write stream skips TTFB when server responds before any flush", func(t *testing.T) {
		ctx, mr, cm := setup(t)
		w := newWriteStream(ctx, cm, "/google.storage.v2.Storage/BidiWriteObject", &concurrentFlushMockStream{}, time.Now().Add(-2*time.Second))

		if err := w.SendMsg(&storagepb.BidiWriteObjectRequest{WriteOffset: 0}); err != nil {
			t.Fatalf("SendMsg non-flush: %v", err)
		}
		if err := w.RecvMsg(&storagepb.BidiWriteObjectResponse{}); err != nil {
			t.Fatalf("RecvMsg: %v", err)
		}
		if dps := collectTTFB(t, ctx, mr); len(dps) != 0 {
			t.Fatalf("expected 0 ttfb datapoints on unanchored write response, got %+v", dps)
		}
	})

	t.Run("gRPC unary records TTFB only on genuine server responses", func(t *testing.T) {
		t.Run("real server response over bufconn records TTFB", func(t *testing.T) {
			ctx, mr, cm := setup(t)
			unaryInt, _ := metricsInterceptors(cm)

			lis := bufconn.Listen(1024 * 1024)
			srv := grpc.NewServer()
			storagepb.RegisterStorageServer(srv, &storagepb.UnimplementedStorageServer{})
			go func() { _ = srv.Serve(lis) }()
			t.Cleanup(srv.Stop)

			conn, err := grpc.NewClient("passthrough:///bufnet",
				grpc.WithContextDialer(func(context.Context, string) (net.Conn, error) { return lis.Dial() }),
				grpc.WithTransportCredentials(insecure.NewCredentials()),
				grpc.WithUnaryInterceptor(unaryInt),
			)
			if err != nil {
				t.Fatalf("grpc.NewClient: %v", err)
			}
			t.Cleanup(func() { _ = conn.Close() })

			_, err = storagepb.NewStorageClient(conn).GetObject(ctx, &storagepb.GetObjectRequest{Bucket: "projects/_/buckets/bkt", Object: "obj"})
			if status.Code(err) != codes.Unimplemented {
				t.Fatalf("GetObject err = %v, want Unimplemented", err)
			}
			if dps := collectTTFB(t, ctx, mr); len(dps) != 1 || dps[0].Count != 1 {
				t.Errorf("expected 1 ttfb datapoint from server Unimplemented response, got %+v", dps)
			}
		})

		for _, tc := range []struct {
			name        string
			withHeader  bool
			withTrailer bool
			err         error
			wantCount   uint64
		}{
			{name: "successful call without metadata", withHeader: false, err: nil, wantCount: 1},
			{name: "server NotFound with headers", withHeader: true, err: status.Error(codes.NotFound, "object not found"), wantCount: 1},
			{name: "server NotFound with trailers", withTrailer: true, err: status.Error(codes.NotFound, "object not found"), wantCount: 1},
			{name: "local transport closing without metadata", withHeader: false, err: status.Error(codes.Unavailable, "transport is closing"), wantCount: 0},
			{name: "local dial connection refused", withHeader: false, err: status.Error(codes.Unavailable, "connection error: desc = \"transport: Error while dialing: dial tcp: connection refused\""), wantCount: 0},
			{name: "connection reset after server headers", withHeader: true, err: status.Error(codes.Unavailable, "connection error: desc = \"transport: read tcp: connection reset by peer\""), wantCount: 0},
			{name: "client cancel after server headers", withHeader: true, err: status.Error(codes.Canceled, "context canceled"), wantCount: 0},
			{name: "client deadline after server headers", withHeader: true, err: status.Error(codes.DeadlineExceeded, "context deadline exceeded"), wantCount: 0},
		} {
			t.Run(tc.name, func(t *testing.T) {
				ctx, mr, cm := setup(t)
				unaryInt, _ := metricsInterceptors(cm)
				_ = unaryInt(ctx, "/google.storage.v2.Storage/GetObject", nil, nil, nil, func(_ context.Context, _ string, _, _ interface{}, _ *grpc.ClientConn, opts ...grpc.CallOption) error {
					for _, o := range opts {
						if tc.withHeader {
							if h, ok := o.(grpc.HeaderCallOption); ok {
								*h.HeaderAddr = metadata.Pairs(uploadIDHeader, "u-123")
							}
						}
						if tc.withTrailer {
							if tr, ok := o.(grpc.TrailerCallOption); ok {
								*tr.TrailerAddr = metadata.Pairs(uploadIDHeader, "u-123")
							}
						}
					}
					return tc.err
				})

				var got uint64
				for _, dp := range collectTTFB(t, ctx, mr) {
					got += dp.Count
				}
				if got != tc.wantCount {
					t.Errorf("ttfb sample count = %d, want %d", got, tc.wantCount)
				}
			})
		}
	})
}

func TestIsHTTPPayloadUpload(t *testing.T) {
	for _, tc := range []struct {
		name          string
		method        string
		url           string
		body          io.Reader
		logicalMethod string
		want          bool
	}{
		{name: "resumable chunk PUT", method: http.MethodPut, url: "https://storage.googleapis.com/upload/storage/v1/b/bkt/o?uploadType=resumable&upload_id=1", body: bytes.NewReader([]byte("x")), logicalMethod: "WriteObject", want: true},
		{name: "resumable chunk POST", method: http.MethodPost, url: "https://storage.googleapis.com/upload/storage/v1/b/bkt/o?uploadType=resumable&upload_id=1", body: bytes.NewReader([]byte("x")), logicalMethod: "WriteObject", want: true},
		{name: "multipart upload POST", method: http.MethodPost, url: "https://storage.googleapis.com/upload/storage/v1/b/bkt/o?uploadType=multipart", body: bytes.NewReader([]byte("x")), logicalMethod: "WriteObject", want: true},
		{name: "write operation PUT outside the upload path", method: http.MethodPut, url: "https://bkt.storage.googleapis.com/obj?partNumber=1&uploadId=1", body: bytes.NewReader([]byte("x")), logicalMethod: "WriteObject", want: true},
		{name: "resumable session initiation POST", method: http.MethodPost, url: "https://storage.googleapis.com/upload/storage/v1/b/bkt/o?uploadType=resumable", body: bytes.NewReader([]byte("{}")), logicalMethod: "WriteObject", want: false},
		{name: "compose POST with JSON body", method: http.MethodPost, url: "https://storage.googleapis.com/storage/v1/b/bkt/o/obj/compose", body: bytes.NewReader([]byte("{}")), logicalMethod: "ComposeObject", want: false},
		{name: "bucket insert POST with JSON body", method: http.MethodPost, url: "https://storage.googleapis.com/storage/v1/b", body: bytes.NewReader([]byte("{}")), logicalMethod: "CreateBucket", want: false},
		{name: "object PATCH with JSON body", method: http.MethodPatch, url: "https://storage.googleapis.com/storage/v1/b/bkt/o/obj", body: bytes.NewReader([]byte("{}")), logicalMethod: "UpdateObject", want: false},
		{name: "upload PUT without body", method: http.MethodPut, url: "https://storage.googleapis.com/upload/storage/v1/b/bkt/o?uploadType=resumable&upload_id=1", logicalMethod: "WriteObject", want: false},
		{name: "upload PUT with http.NoBody", method: http.MethodPut, url: "https://storage.googleapis.com/upload/storage/v1/b/bkt/o?uploadType=resumable&upload_id=1", body: http.NoBody, logicalMethod: "WriteObject", want: false},
		{name: "download GET", method: http.MethodGet, url: "https://storage.googleapis.com/storage/v1/b/bkt/o/obj?alt=media", logicalMethod: "ReadObject", want: false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			req, err := http.NewRequest(tc.method, tc.url, tc.body)
			if err != nil {
				t.Fatalf("NewRequest: %v", err)
			}
			if got := isHTTPPayloadUpload(req, tc.logicalMethod); got != tc.want {
				t.Errorf("isHTTPPayloadUpload = %v, want %v", got, tc.want)
			}
		})
	}
}

func TestIsWriteFlushMsg(t *testing.T) {
	appendSpec := &storagepb.BidiWriteObjectRequest_AppendObjectSpec{AppendObjectSpec: &storagepb.AppendObjectSpec{Object: "obj"}}
	appendableNewSpec := &storagepb.BidiWriteObjectRequest_WriteObjectSpec{WriteObjectSpec: &storagepb.WriteObjectSpec{Appendable: proto.Bool(true)}}
	nonAppendableNewSpec := &storagepb.BidiWriteObjectRequest_WriteObjectSpec{WriteObjectSpec: &storagepb.WriteObjectSpec{Appendable: proto.Bool(false)}}
	data := &storagepb.BidiWriteObjectRequest_ChecksummedData{ChecksummedData: &storagepb.ChecksummedData{Content: []byte("x")}}
	for _, tc := range []struct {
		name string
		msg  interface{}
		want bool
	}{
		{name: "bidi data only", msg: &storagepb.BidiWriteObjectRequest{Data: data}, want: false},
		{name: "bidi flush without finish or lookup", msg: &storagepb.BidiWriteObjectRequest{Data: data, Flush: true}, want: false},
		{name: "bidi finish_write", msg: &storagepb.BidiWriteObjectRequest{Data: data, FinishWrite: true}, want: true},
		{name: "bidi state_lookup", msg: &storagepb.BidiWriteObjectRequest{StateLookup: true}, want: true},
		{name: "bidi append spec without data", msg: &storagepb.BidiWriteObjectRequest{FirstMessage: appendSpec}, want: true},
		{name: "bidi append spec with inline data", msg: &storagepb.BidiWriteObjectRequest{FirstMessage: appendSpec, Data: data}, want: true},
		{name: "bidi new appendable write spec with inline data", msg: &storagepb.BidiWriteObjectRequest{FirstMessage: appendableNewSpec, Data: data}, want: true},
		{name: "bidi non-appendable write spec without flush", msg: &storagepb.BidiWriteObjectRequest{FirstMessage: nonAppendableNewSpec, Data: data}, want: false},
		{name: "write data only", msg: &storagepb.WriteObjectRequest{WriteOffset: 1}, want: false},
		{name: "write finish_write", msg: &storagepb.WriteObjectRequest{FinishWrite: true}, want: true},
		{name: "read request", msg: &storagepb.BidiReadObjectRequest{}, want: false},
		{name: "nil message", msg: nil, want: false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := isWriteFlushMsg(tc.msg); got != tc.want {
				t.Errorf("isWriteFlushMsg = %v, want %v", got, tc.want)
			}
		})
	}
}

func TestComputeErrorType(t *testing.T) {
	dnsErr := &net.DNSError{Err: "no such host", Name: "storage.googleapis.com", IsTimeout: true}

	tests := []struct {
		name       string
		err        error
		isHTTP     bool
		statusCode int64
		want       string
	}{
		{name: "dns no such host", err: errors.New("dial tcp: no such host"), want: "DNS_FAILURE"},
		{name: "connection refused", err: errors.New("connection refused"), want: "CONNECTION_ERROR"},
		{name: "connection reset", err: errors.New("connection reset by peer"), want: "CONNECTION_ERROR"},
		{name: "tls bad certificate", err: errors.New("tls: bad certificate"), want: "TLS_FAILURE"},
		{name: "unexpected eof", err: errors.New("unexpected eof"), want: "CONNECTION_ERROR"},
		{name: "context deadline exceeded", err: context.DeadlineExceeded, want: "TIMEOUT"},
		{name: "context canceled", err: context.Canceled, want: "CANCELLED"},
		{name: "grpc not found", err: status.Error(codes.NotFound, "not found"), want: "NOT_FOUND"},
		{name: "ErrObjectNotExist", err: ErrObjectNotExist, isHTTP: true, want: "NOT_FOUND"},
		{name: "ErrBucketNotExist", err: ErrBucketNotExist, want: "NOT_FOUND"},
		{name: "grpc not found with key in object name", err: status.Error(codes.NotFound, "No such object: bucket/my-key"), want: "NOT_FOUND"},
		{name: "http 404 with author in object name", err: &googleapi.Error{Code: 404, Message: "No such object: bucket/author.txt"}, isHTTP: true, want: "NOT_FOUND"},
		{name: "grpc invalid page token", err: status.Error(codes.InvalidArgument, "Invalid page token."), want: "INVALID_ARGUMENT"},
		{name: "grpc unavailable connection reset", err: status.Error(codes.Unavailable, "connection error: connection reset by peer"), want: "CONNECTION_ERROR"},
		{name: "grpc unavailable", err: status.Error(codes.Unavailable, "unavailable"), want: "UNAVAILABLE"},
		{name: "grpc crc32c mismatch", err: status.Error(codes.InvalidArgument, "Object crc32c mismatch: server computed 1234, client provided 5678."), want: "CHECKSUM_MISMATCH"},
		{name: "http 400 crc32c mismatch", err: &googleapi.Error{Code: 400, Message: `Provided CRC32C "AAAAAA==" doesn't match calculated CRC32C "BBBBBB==".`}, isHTTP: true, want: "CHECKSUM_MISMATCH"},
		{name: "client checksum mismatch sentinel", err: fmt.Errorf("%w: bad CRC on read: got %d, want %d", errChecksumMismatch, 1, 2), want: "CHECKSUM_MISMATCH"},
		{name: "http 412 failed precondition", err: &googleapi.Error{Code: 412, Message: "At least one of the pre-conditions you specified did not hold."}, isHTTP: true, want: "FAILED_PRECONDITION"},
		{name: "http 408 deadline exceeded", err: &googleapi.Error{Code: 408}, isHTTP: true, want: "DEADLINE_EXCEEDED"},
		{name: "grpc server deadline exceeded", err: status.Error(codes.DeadlineExceeded, "Deadline exceeded."), want: "DEADLINE_EXCEEDED"},
		{name: "wrapped io.EOF on grpc read", err: fmt.Errorf("read: %w", io.EOF), want: "OK"},

		// HTTP status, url.Error & EOF edge cases.
		{name: "http 502 bad gateway", err: &googleapi.Error{Code: 502}, isHTTP: true, want: "UNAVAILABLE"},
		{name: "http body io.EOF with 200", err: io.EOF, isHTTP: true, statusCode: 200, want: "OK"},
		{name: "http body io.EOF with 404", err: io.EOF, isHTTP: true, statusCode: 404, want: "NOT_FOUND"},
		{name: "http round-trip io.EOF before headers", err: fmt.Errorf("Get \"https://storage.googleapis.com\": %w", io.EOF), isHTTP: true, statusCode: 0, want: "CONNECTION_ERROR"},
		{name: "http ErrUnexpectedEOF", err: io.ErrUnexpectedEOF, isHTTP: true, want: "CONNECTION_ERROR"},
		{name: "url.Error with author bucket and deadline", err: &url.Error{Op: "Get", URL: "https://storage.googleapis.com/b/author-bkt/o/tls-certs", Err: context.DeadlineExceeded}, isHTTP: true, want: "TIMEOUT"},
		{name: "url.Error with author bucket and unknown error", err: &url.Error{Op: "Get", URL: "https://storage.googleapis.com/b/author-bkt/o/tls-certs", Err: errors.New("unexpected failure")}, isHTTP: true, want: "UNKNOWN"},

		// Timeouts & stall watchdogs.
		{name: "grpc client context deadline exceeded", err: status.Error(codes.DeadlineExceeded, "context deadline exceeded"), want: "TIMEOUT"},
		{name: "write stall timeout error", err: &stallTimeoutError{timeout: time.Second, stage: "BidiWriteObject"}, want: "TIMEOUT"},
		// wrappedCallErr.Is matches both context.Canceled and errStallTimeout when the
		// caller cancels ctx during retry backoff after a stall; user cancel wins.
		{name: "user cancel after write stall attempt", err: wrappedCallErr{ctxErr: context.Canceled, wrappedErr: &stallTimeoutError{timeout: time.Second, stage: "BidiWriteObject"}}, want: "CANCELLED"},

		// Authentication failures.
		{name: "credentialError wrapping dial timeout", err: &credentialError{err: errors.New("dial tcp 169.254.169.254:80: i/o timeout")}, isHTTP: true, want: "AUTHENTICATION_ERROR"},
		{name: "grpc per-RPC creds timeout", err: status.Error(codes.Unavailable, "transport: per-RPC creds failed due to error: context deadline exceeded"), want: "AUTHENTICATION_ERROR"},

		// Structured DNS, TCP, and TLS transport errors.
		{name: "dial OpError wrapping DNSError", err: &net.OpError{Op: "dial", Net: "tcp", Err: dnsErr}, isHTTP: true, want: "DNS_FAILURE"},
		{name: "dial OpError wrapping os.ErrDeadlineExceeded", err: &net.OpError{Op: "dial", Net: "tcp", Err: os.ErrDeadlineExceeded}, isHTTP: true, want: "CONNECTION_ERROR"},
		{name: "net.ErrClosed", err: net.ErrClosed, isHTTP: true, want: "CONNECTION_ERROR"},
		{name: "tls unknown authority", err: errors.New("tls: failed to verify certificate: x509: certificate signed by unknown authority"), isHTTP: true, want: "TLS_FAILURE"},
		{name: "grpc authentication handshake failed without tls keyword", err: status.Error(codes.Unavailable, `connection error: desc = "transport: authentication handshake failed: remote error: bad MAC"`), want: "TLS_FAILURE"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := computeErrorType(tc.err, tc.isHTTP, tc.statusCode)
			if got != tc.want {
				t.Errorf("computeErrorType(%v, %v, %v) = %v, want %v", tc.err, tc.isHTTP, tc.statusCode, got, tc.want)
			}
		})
	}

	uerr := &url.Error{Op: "Get", URL: "https://storage.googleapis.com", Err: &credentialError{err: os.ErrDeadlineExceeded}}
	if !uerr.Timeout() || !uerr.Temporary() {
		t.Errorf("url.Error wrapping credentialError(os.ErrDeadlineExceeded): Timeout()=%v, Temporary()=%v; want both true", uerr.Timeout(), uerr.Temporary())
	}
}

func TestExecuteWithReadStallTimeout_RecordsTimeoutMetric(t *testing.T) {
	ctx := context.Background()
	mr := sdkmetric.NewManualReader()
	provider := sdkmetric.NewMeterProvider(sdkmetric.WithReader(mr))
	defer provider.Shutdown(ctx)

	cm, _, err := initMetrics(ctx, "project-id", &storageConfig{
		enableOtelMetrics: true,
		meterProvider:     provider,
	})
	if err != nil {
		t.Fatalf("initMetrics: %v", err)
	}

	unblock := make(chan struct{})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		<-unblock
	}))
	defer func() {
		close(unblock)
		server.Close()
	}()

	dm, err := newBucketDelayManager(0.99, 15, 0, time.Millisecond, time.Second)
	if err != nil {
		t.Fatalf("newBucketDelayManager: %v", err)
	}
	client := &http.Client{
		Transport: &metricsRoundTripper{base: http.DefaultTransport, metrics: cm},
	}

	err = executeWithReadStallTimeout(ctx, dm, "my-bucket", func(reqCtx context.Context) error {
		req, err := http.NewRequestWithContext(reqCtx, http.MethodGet, server.URL+"/storage/v1/b/my-bucket/o/my-object?alt=media", nil)
		if err != nil {
			return err
		}
		resp, err := client.Do(req)
		if resp != nil {
			resp.Body.Close()
		}
		return err
	}, nil)
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("executeWithReadStallTimeout err = %v, want context.DeadlineExceeded", err)
	}

	var rm metricdata.ResourceMetrics
	if err := mr.Collect(ctx, &rm); err != nil {
		t.Fatalf("Collect: %v", err)
	}
	seen := make(map[string]bool)
	for _, sm := range rm.ScopeMetrics {
		for _, m := range sm.Metrics {
			switch m.Name {
			case "gcp.storage.client.attempts", "gcp.storage.client.errors":
				seen[m.Name] = true
				dps := m.Data.(metricdata.Sum[int64]).DataPoints
				if len(dps) != 1 {
					t.Fatalf("%s: got %d data points, want 1", m.Name, len(dps))
				}
				if got := getAttr(dps[0], "error.type"); got != "TIMEOUT" {
					t.Errorf("%s error.type = %q, want TIMEOUT", m.Name, got)
				}
				if got := getAttr(dps[0], "network.path"); got != networkPathCloud {
					t.Errorf("%s network.path = %q, want %q", m.Name, got, networkPathCloud)
				}
			case "http.client.request.duration":
				seen[m.Name] = true
				dps := m.Data.(metricdata.Histogram[float64]).DataPoints
				if len(dps) != 1 {
					t.Fatalf("%s: got %d data points, want 1", m.Name, len(dps))
				}
				if got := getHistAttr(dps[0], "error.type"); got != "TIMEOUT" {
					t.Errorf("%s error.type = %q, want TIMEOUT", m.Name, got)
				}
			}
		}
	}
	for _, name := range []string{"gcp.storage.client.attempts", "gcp.storage.client.errors", "http.client.request.duration"} {
		if !seen[name] {
			t.Errorf("metric %s not recorded", name)
		}
	}
}

func TestRefineCancelled(t *testing.T) {
	readCtx, readCancel := context.WithCancelCause(context.Background())
	readCancel(errReadStallTimeout)

	writeCtx, writeCancel := context.WithCancelCause(context.Background())
	writeCancel(errStallTimeout)

	userCtx, userCancel := context.WithCancel(context.Background())
	userCancel()

	for _, tc := range []struct {
		name      string
		ctx       context.Context
		errorType string
		want      string
	}{
		{name: "read stall timeout cause", ctx: readCtx, errorType: "CANCELLED", want: "TIMEOUT"},
		{name: "write stall timeout cause", ctx: writeCtx, errorType: "CANCELLED", want: "TIMEOUT"},
		{name: "user context cancel", ctx: userCtx, errorType: "CANCELLED", want: "CANCELLED"},
		{name: "non-cancelled error type", ctx: readCtx, errorType: "CONNECTION_ERROR", want: "CONNECTION_ERROR"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := refineCancelled(tc.ctx, tc.errorType); got != tc.want {
				t.Errorf("refineCancelled(ctx, %q) = %q, want %q", tc.errorType, got, tc.want)
			}
		})
	}
}

func TestStripPort(t *testing.T) {
	for _, tc := range []struct {
		in, want string
	}{
		{"storage.googleapis.com:443", "storage.googleapis.com"},
		{"dns:///storage.googleapis.com:443", "storage.googleapis.com"},
		{"google-c2p:///storage.googleapis.com", "storage.googleapis.com"},
		{"dns://authority/storage.googleapis.com:443", "storage.googleapis.com"},
		{"[::1]:8080", "::1"},
		{"[::1]", "::1"},
	} {
		t.Run(tc.in, func(t *testing.T) {
			if got := stripPort(tc.in); got != tc.want {
				t.Errorf("stripPort(%q) = %q, want %q", tc.in, got, tc.want)
			}
		})
	}
}

func TestGRPCMetricsStatsHandler(t *testing.T) {
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

	var dialTimes sync.Map
	localAddr := &net.TCPAddr{IP: net.IPv4(127, 0, 0, 1), Port: 50000}
	remoteAddr := &net.TCPAddr{IP: net.IPv4(34, 126, 10, 53), Port: 443}
	dialTimes.Store(localAddr.String(), dialInfo{
		doneTime:   time.Now().Add(-5 * time.Millisecond),
		host:       "storage.googleapis.com",
		remoteAddr: remoteAddr,
	})

	h := &grpcMetricsStatsHandler{metrics: cm, dialTimes: &dialTimes}
	connCtx := h.TagConn(ctx, &stats.ConnTagInfo{LocalAddr: localAddr, RemoteAddr: remoteAddr})
	if _, ok := dialTimes.Load(localAddr.String()); ok {
		t.Errorf("expected TagConn to delete dialTimes entry")
	}
	h.HandleConn(connCtx, &stats.ConnBegin{})

	var rm metricdata.ResourceMetrics
	if err := mr.Collect(ctx, &rm); err != nil {
		t.Fatalf("Collect: %v", err)
	}
	var foundTLS bool
	for _, sm := range rm.ScopeMetrics {
		for _, m := range sm.Metrics {
			if m.Name == "gcp.storage.client.network.tls.handshake.duration" {
				foundTLS = true
				dps := m.Data.(metricdata.Histogram[float64]).DataPoints
				if len(dps) != 1 {
					t.Fatalf("tls.handshake.duration datapoints = %d, want 1", len(dps))
				}
				assertExactAttrKeys(t, m.Name, dps[0].Attributes,
					"network.path", "rpc.system.name", "server.address")
				if got := getHistAttr(dps[0], "network.path"); got != networkPathDirect {
					t.Errorf("tls.handshake.duration network.path = %q, want %q", got, networkPathDirect)
				}
			}
		}
	}
	if !foundTLS {
		t.Errorf("expected gcp.storage.client.network.tls.handshake.duration to be recorded")
	}
}

func TestCountingConn(t *testing.T) {
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

	// Test countingConn records socket-level gcp.storage.client.network.io with
	// network.path and invokes onClose once to clean up dialTimes.
	c1, c2 := net.Pipe()
	defer c2.Close()

	var dialTimes sync.Map
	dialTimes.Store("local-1", dialInfo{doneTime: time.Now(), host: "storage.googleapis.com"})
	var closeCalls int
	cc := newCountingConn(c1, cm.networkIO, "storage.googleapis.com", networkPathDirect, func() {
		closeCalls++
		dialTimes.Delete("local-1")
	})

	done := make(chan error, 1)
	go func() {
		buf := make([]byte, 16)
		if _, err := cc.Read(buf); err != nil {
			done <- err
			return
		}
		_, err := cc.Write([]byte("ping"))
		done <- err
	}()
	if _, err := c2.Write([]byte("0123456789abcdef")); err != nil {
		t.Fatalf("pipe write: %v", err)
	}
	reply := make([]byte, 4)
	if _, err := io.ReadFull(c2, reply); err != nil {
		t.Fatalf("pipe read: %v", err)
	}
	if err := <-done; err != nil {
		t.Fatalf("countingConn I/O: %v", err)
	}
	if err := cc.Close(); err != nil {
		t.Fatalf("countingConn.Close: %v", err)
	}
	_ = cc.Close()
	if closeCalls != 1 {
		t.Errorf("onClose called %d times, want 1", closeCalls)
	}
	if _, ok := dialTimes.Load("local-1"); ok {
		t.Errorf("expected countingConn.Close to delete dialTimes entry")
	}

	var rm metricdata.ResourceMetrics
	if err := mr.Collect(ctx, &rm); err != nil {
		t.Fatalf("Collect: %v", err)
	}

	var ingress, egress int64
	for _, sm := range rm.ScopeMetrics {
		for _, m := range sm.Metrics {
			if m.Name == "gcp.storage.client.network.io" {
				sum := m.Data.(metricdata.Sum[int64])
				for _, dp := range sum.DataPoints {
					assertExactAttrKeys(t, m.Name, dp.Attributes,
						"network.io.direction", "network.path", "rpc.system.name", "server.address")
					if got := getAttr(dp, "network.path"); got != networkPathDirect {
						t.Errorf("network.path = %q, want %q", got, networkPathDirect)
					}
					switch getAttr(dp, "network.io.direction") {
					case networkIOReceive:
						ingress += dp.Value
					case networkIOTransmit:
						egress += dp.Value
					}
				}
			}
		}
	}

	if ingress != 16 {
		t.Errorf("expected ingress 16, got %v", ingress)
	}
	if egress != 4 {
		t.Errorf("expected egress 4, got %v", egress)
	}
}

func attrSetValueString(s attribute.Set, key string) string {
	v, ok := s.Value(attribute.Key(key))
	if !ok {
		return ""
	}
	return v.String()
}

func assertExactAttrKeys(t *testing.T, metricName string, s attribute.Set, wantKeys ...string) {
	t.Helper()
	got := make([]string, 0, s.Len())
	iter := s.Iter()
	for iter.Next() {
		got = append(got, string(iter.Attribute().Key))
	}
	slices.Sort(got)
	want := slices.Clone(wantKeys)
	slices.Sort(want)
	if !slices.Equal(got, want) {
		t.Errorf("%s attribute keys = %v, want %v", metricName, got, want)
	}
}

func getAttr(dp metricdata.DataPoint[int64], key string) string {
	return attrSetValueString(dp.Attributes, key)
}

func getHistAttr(dp metricdata.HistogramDataPoint[float64], key string) string {
	return attrSetValueString(dp.Attributes, key)
}

func TestRecordCredentialRefreshDuration(t *testing.T) {
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

	tok1 := &auth.Token{Value: "tok-1", Expiry: time.Unix(1000, 0)}
	tok2 := &auth.Token{Value: "tok-2", Expiry: time.Unix(2000, 0)}
	seq := 0
	creds := wrapAuthCredentials(&auth.Credentials{
		TokenProvider: &mockTokenProvider{
			tokenFn: func() (*auth.Token, error) {
				seq++
				switch seq {
				case 1, 2:
					return tok1, nil // 2nd call is a cache hit and should not be recorded
				case 3:
					return tok2, nil // rotated token should be recorded
				case 4:
					return nil, errors.New("refresh failed")
				case 5:
					return nil, context.DeadlineExceeded
				default:
					return nil, context.Canceled
				}
			},
		},
	}, cm)

	for i := 0; i < 6; i++ {
		_, _ = creds.TokenProvider.Token(ctx)
	}

	var rm metricdata.ResourceMetrics
	if err := mr.Collect(ctx, &rm); err != nil {
		t.Fatalf("Collect: %v", err)
	}

	found := false
	for _, sm := range rm.ScopeMetrics {
		for _, m := range sm.Metrics {
			if m.Name == "gcp.storage.client.auth.credential_refresh.duration" {
				found = true
				hist := m.Data.(metricdata.Histogram[float64])
				byType := make(map[string]metricdata.HistogramDataPoint[float64])
				for _, dp := range hist.DataPoints {
					byType[getHistAttr(dp, "error.type")] = dp
				}
				if dp, ok := byType["OK"]; !ok || dp.Count != 2 {
					t.Errorf("expected 2 OK datapoints (initial + rotated token, excluding cache hit), got %+v", dp)
				}
				for _, wantType := range []string{"AUTHENTICATION_ERROR", "TIMEOUT", "CANCELLED"} {
					if dp, ok := byType[wantType]; !ok || dp.Count != 1 {
						t.Errorf("missing or unexpected %s datapoint: %+v", wantType, dp)
					}
				}
			}
		}
	}
	if !found {
		t.Errorf("metric not found")
	}
}

func TestWrapAuthCredentials(t *testing.T) {
	if wrapAuthCredentials(nil, nil) != nil {
		t.Errorf("expected nil when c is nil")
	}
	c := &auth.Credentials{}
	if wrapAuthCredentials(c, nil) != c {
		t.Errorf("expected original credentials when TokenProvider is nil")
	}
	c.TokenProvider = &mockTokenProvider{}
	m := &clientMetrics{}
	wrapped := wrapAuthCredentials(c, m)
	if wrapped == c {
		t.Errorf("expected a new copy of credentials, got original")
	}
	if _, ok := wrapped.TokenProvider.(*metricsTokenProvider); !ok {
		t.Errorf("expected TokenProvider to be *metricsTokenProvider")
	}
	wrapped2 := wrapAuthCredentials(wrapped, m)
	if wrapped2 != wrapped {
		t.Errorf("expected original wrapped credentials on double wrap")
	}

	rootErr := &net.OpError{Op: "dial", Net: "tcp", Err: context.DeadlineExceeded}
	errCreds := wrapAuthCredentials(&auth.Credentials{
		TokenProvider: &mockTokenProvider{err: rootErr},
	}, m)
	_, err := errCreds.TokenProvider.Token(context.Background())
	if !errors.Is(err, rootErr) {
		t.Errorf("expected errors.Is(err, rootErr) to be true, got %v", err)
	}
	if got := computeErrorType(err, true, 0); got != "AUTHENTICATION_ERROR" {
		t.Errorf("computeErrorType(err, true, 0) = %q, want AUTHENTICATION_ERROR", got)
	}
}

type debugOnlyGRPCServer struct {
	storagepb.UnimplementedStorageServer
}

func (s *debugOnlyGRPCServer) GetObject(ctx context.Context, req *storagepb.GetObjectRequest) (*storagepb.Object, error) {
	_ = grpc.SetHeader(ctx, metadata.Pairs("x-goog-gfe-service-time", "12"))
	return &storagepb.Object{Name: req.GetObject(), Bucket: req.GetBucket()}, nil
}

func TestNewClient_DebugMetricsOnly(t *testing.T) {
	standard := map[string]bool{
		"rpc.client.call.duration":              true,
		"http.client.request.duration":          true,
		"gcp.client.request.duration":           true,
		"gcp.storage.client.operations":         true,
		"gcp.storage.client.attempts":           true,
		"gcp.storage.client.errors":             true,
		"gcp.storage.client.ttfb":               true,
		"gcp.storage.client.request.body.size":  true,
		"gcp.storage.client.response.body.size": true,
		"gcp.storage.client.server.unreached":   true,
	}

	for _, tc := range []struct {
		name      string
		newClient func(t *testing.T, ctx context.Context, provider *sdkmetric.MeterProvider) *Client
		wantDebug []string
	}{
		{
			name: "HTTP",
			newClient: func(t *testing.T, ctx context.Context, provider *sdkmetric.MeterProvider) *Client {
				srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					w.Header().Set("X-Goog-Gfe-Service-Time", "15")
					w.WriteHeader(http.StatusOK)
					_, _ = w.Write([]byte(`{"name":"my-object"}`))
				}))
				t.Cleanup(srv.Close)
				c, err := NewClient(ctx,
					option.WithEndpoint(srv.URL+"/storage/v1/"),
					option.WithAuthCredentials(auth.NewCredentials(&auth.CredentialsOptions{
						TokenProvider: &mockTokenProvider{},
					})),
					withMeterProvider(provider),
				)
				if err != nil {
					t.Fatalf("NewClient: %v", err)
				}
				return c
			},
			wantDebug: []string{
				"gcp.storage.client.auth.credential_refresh.duration",
				"gcp.storage.client.network.tcp.connect.duration",
				"gcp.storage.client.server.duration",
			},
		},
		{
			name: "gRPC",
			newClient: func(t *testing.T, ctx context.Context, provider *sdkmetric.MeterProvider) *Client {
				lis, err := net.Listen("tcp", "127.0.0.1:0")
				if err != nil {
					t.Fatalf("net.Listen: %v", err)
				}
				srv := grpc.NewServer()
				storagepb.RegisterStorageServer(srv, &debugOnlyGRPCServer{})
				go func() { _ = srv.Serve(lis) }()
				t.Cleanup(srv.Stop)
				c, err := NewGRPCClient(ctx,
					option.WithEndpoint(lis.Addr().String()),
					option.WithoutAuthentication(),
					option.WithGRPCDialOption(grpc.WithTransportCredentials(insecure.NewCredentials())),
					internaloption.EnableDirectPath(false),
					withMeterProvider(provider),
				)
				if err != nil {
					t.Fatalf("NewGRPCClient: %v", err)
				}
				return c
			},
			wantDebug: []string{
				"gcp.storage.client.network.tcp.connect.duration",
				"gcp.storage.client.server.duration",
			},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ctx := context.Background()
			mr := sdkmetric.NewManualReader()
			provider := sdkmetric.NewMeterProvider(sdkmetric.WithReader(mr))
			defer provider.Shutdown(ctx)

			t.Setenv(envOtelDebugMetrics, "true")
			t.Setenv(envOtelMetrics, "false")
			client := tc.newClient(t, ctx, provider)
			defer client.Close()

			if _, err := client.Bucket("my-bucket").Object("my-object").Attrs(ctx); err != nil {
				t.Fatalf("Attrs: %v", err)
			}

			var rm metricdata.ResourceMetrics
			if err := mr.Collect(ctx, &rm); err != nil {
				t.Fatalf("Collect: %v", err)
			}
			seen := make(map[string]uint64)
			for _, sm := range rm.ScopeMetrics {
				for _, m := range sm.Metrics {
					if standard[m.Name] {
						t.Errorf("standard metric %s emitted with only debug metrics enabled", m.Name)
					}
					if hist, ok := m.Data.(metricdata.Histogram[float64]); ok && len(hist.DataPoints) > 0 {
						seen[m.Name] = hist.DataPoints[0].Count
					}
				}
			}
			for _, name := range tc.wantDebug {
				if seen[name] == 0 {
					t.Errorf("expected debug metric %s to record at least 1 sample, seen=%v", name, seen)
				}
			}
		})
	}
}

type mockTokenProvider struct {
	err     error
	tokenFn func() (*auth.Token, error)
}

func (m *mockTokenProvider) Token(ctx context.Context) (*auth.Token, error) {
	if m.tokenFn != nil {
		return m.tokenFn()
	}
	if m.err != nil {
		return nil, m.err
	}
	return &auth.Token{Value: "test-token", Expiry: time.Now().Add(time.Hour)}, nil
}

func TestRecordStallDuration_BidiWriteObject(t *testing.T) {
	dpAddr, _ := net.ResolveTCPAddr("tcp", "34.126.0.1:443")
	cpAddr, _ := net.ResolveTCPAddr("tcp", "142.250.80.10:443")

	for _, tc := range []struct {
		name          string
		run           func(ctx context.Context, cm *clientMetrics)
		wantSystem    string
		wantOperation string
		wantMethod    string
		wantPath      string
		wantSum       float64
	}{
		{
			name: "gRPC BidiWriteObject without peer defaults to unknown",
			run: func(ctx context.Context, cm *clientMetrics) {
				cm.recordStallDuration(ctx, 2*time.Second, rpcBidiWriteObject, "grpc", "storage.googleapis.com")
			},
			wantSystem: "grpc", wantOperation: "WriteObject", wantMethod: "google.storage.v2.Storage/BidiWriteObject", wantPath: "unknown", wantSum: 2.0,
		},
		{
			name: "gRPC BidiWriteObject with DirectPath OutHeader",
			run: func(ctx context.Context, cm *clientMetrics) {
				dpCtx, _ := cm.startOperation(ctx, "WriteObject", false)
				(&grpcMetricsStatsHandler{metrics: cm}).HandleRPC(dpCtx, &stats.OutHeader{RemoteAddr: dpAddr})
				cm.recordStallDuration(dpCtx, 2*time.Second, rpcBidiWriteObject, "grpc", "storage.googleapis.com")
			},
			wantSystem: "grpc", wantOperation: "WriteObject", wantMethod: "google.storage.v2.Storage/BidiWriteObject", wantPath: "directpath", wantSum: 2.0,
		},
		{
			name: "gRPC StartResumableWrite with CloudPath OutHeader",
			run: func(ctx context.Context, cm *clientMetrics) {
				cpCtx, _ := cm.startOperation(ctx, "WriteObject", false)
				(&grpcMetricsStatsHandler{metrics: cm}).HandleRPC(cpCtx, &stats.OutHeader{RemoteAddr: cpAddr})
				cm.recordStallDuration(cpCtx, 1500*time.Millisecond, rpcStartResumableWrite, "grpc", "storage.googleapis.com")
			},
			wantSystem: "grpc", wantOperation: "WriteObject", wantMethod: "google.storage.v2.Storage/StartResumableWrite", wantPath: "cloudpath", wantSum: 1.5,
		},
		{
			name: "gRPC BidiReadObject with DirectPath peer",
			run: func(ctx context.Context, cm *clientMetrics) {
				dpCtx, _ := cm.startOperation(ctx, "ReadObject", false)
				metricsStateFromContext(dpCtx).setNetworkPath(networkPathOfPeer(&peer.Peer{Addr: dpAddr}))
				cm.recordStallDuration(dpCtx, time.Second, rpcBidiReadObject, "grpc", "storage.googleapis.com")
			},
			wantSystem: "grpc", wantOperation: "ReadObject", wantMethod: "google.storage.v2.Storage/BidiReadObject", wantPath: "directpath", wantSum: 1.0,
		},
		{
			name: "HTTP ReadObject defaults to cloudpath",
			run: func(ctx context.Context, cm *clientMetrics) {
				cm.recordStallDuration(ctx, 3*time.Second, "ReadObject", "http", "storage.googleapis.com")
			},
			wantSystem: "http", wantOperation: "ReadObject", wantMethod: "google.storage.v2.Storage/ReadObject", wantPath: "cloudpath", wantSum: 3.0,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ctx := context.Background()
			mr := sdkmetric.NewManualReader()
			provider := sdkmetric.NewMeterProvider(sdkmetric.WithReader(mr))
			defer provider.Shutdown(ctx)

			cm, _, err := initMetrics(ctx, "project-id", &storageConfig{
				enableOtelMetrics:      true,
				enableOtelDebugMetrics: true,
				meterProvider:          provider,
			})
			if err != nil {
				t.Fatalf("initMetrics: %v", err)
			}
			tc.run(ctx, cm)

			var rm metricdata.ResourceMetrics
			if err := mr.Collect(ctx, &rm); err != nil {
				t.Fatalf("Collect: %v", err)
			}
			var found bool
			for _, sm := range rm.ScopeMetrics {
				for _, m := range sm.Metrics {
					if m.Name != "gcp.storage.client.stall.duration" {
						continue
					}
					found = true
					dps := m.Data.(metricdata.Histogram[float64]).DataPoints
					if len(dps) != 1 {
						t.Fatalf("got %d data points, want 1", len(dps))
					}
					dp := dps[0]
					if dp.Sum != tc.wantSum {
						t.Errorf("sum = %v, want %v", dp.Sum, tc.wantSum)
					}
					for k, want := range map[string]string{
						"rpc.system.name":       tc.wantSystem,
						"gcp.storage.operation": tc.wantOperation,
						"rpc.method":            tc.wantMethod,
						"network.path":          tc.wantPath,
						"server.address":        "storage.googleapis.com",
					} {
						if got := getHistAttr(dp, k); got != want {
							t.Errorf("%s = %q, want %q", k, got, want)
						}
					}
				}
			}
			if !found {
				t.Errorf("metric gcp.storage.client.stall.duration not found")
			}
		})
	}
}

func TestGRPCNetworkMetricsDialOptions_DNSLookup(t *testing.T) {
	ctx := context.Background()
	mr := sdkmetric.NewManualReader()
	provider := sdkmetric.NewMeterProvider(sdkmetric.WithReader(mr))
	defer provider.Shutdown(ctx)

	cm, _, err := initMetrics(ctx, "project-id", &storageConfig{
		enableOtelMetrics:      true,
		enableOtelDebugMetrics: true,
		meterProvider:          provider,
	})
	if err != nil {
		t.Fatalf("initMetrics: %v", err)
	}

	lis, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("Listen: %v", err)
	}
	defer lis.Close()
	srv := grpc.NewServer()
	defer srv.Stop()
	go srv.Serve(lis)

	origDefault := isDefaultDNSLookupHost
	origLookup := dnsLookupHost
	origNow := dnsNow
	t.Cleanup(func() {
		isDefaultDNSLookupHost = origDefault
		dnsLookupHost = origLookup
		dnsNow = origNow
	})

	isDefaultDNSLookupHost = false
	dnsLookupHost = func(ctx context.Context, host string) ([]string, error) {
		return []string{"127.0.0.1"}, nil
	}
	currentTime := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	dnsNow = func() time.Time { return currentTime }

	dialOpts := append(grpcNetworkMetricsRawDialOptions("storage.us-central1.rep.googleapis.com:443", cm), grpc.WithTransportCredentials(insecure.NewCredentials()))

	dialOnce := func() {
		conn, err := grpc.NewClient("passthrough:///"+lis.Addr().String(), dialOpts...)
		if err != nil {
			t.Fatalf("NewClient: %v", err)
		}
		defer conn.Close()
		conn.Connect()
		waitCtx, cancel := context.WithTimeout(ctx, 2*time.Second)
		defer cancel()
		for st := conn.GetState(); st != connectivity.Ready; st = conn.GetState() {
			if !conn.WaitForStateChange(waitCtx, st) {
				t.Fatalf("timed out waiting for connection Ready; state=%v", st)
			}
		}
	}

	waitDNSCount := func(want uint64) metricdata.HistogramDataPoint[float64] {
		deadline := time.Now().Add(2 * time.Second)
		for {
			var rm metricdata.ResourceMetrics
			if err := mr.Collect(ctx, &rm); err != nil {
				t.Fatalf("Collect: %v", err)
			}
			for _, sm := range rm.ScopeMetrics {
				for _, m := range sm.Metrics {
					if m.Name == "gcp.storage.client.network.dns.lookup.duration" {
						dps := m.Data.(metricdata.Histogram[float64]).DataPoints
						if len(dps) == 1 && dps[0].Count == want {
							return dps[0]
						}
					}
				}
			}
			if time.Now().After(deadline) {
				t.Fatalf("timed out waiting for DNS lookup count == %d", want)
			}
			time.Sleep(5 * time.Millisecond)
		}
	}

	// 1st dial triggers async DNS probe; 2nd immediate dial hits the 60s cooldown.
	dialOnce()
	waitDNSCount(1)
	dialOnce()

	// Advance clock past the 60s cooldown; 3rd dial triggers a second DNS probe.
	currentTime = currentTime.Add(61 * time.Second)
	dialOnce()
	dp := waitDNSCount(2)

	for k, want := range map[string]string{
		"rpc.system.name": "grpc",
		"server.address":  "storage.us-central1.rep.googleapis.com",
		"network.path":    "",
	} {
		if got := getHistAttr(dp, k); got != want {
			t.Errorf("%s = %q, want %q", k, got, want)
		}
	}

	var rm metricdata.ResourceMetrics
	if err := mr.Collect(ctx, &rm); err != nil {
		t.Fatalf("Collect: %v", err)
	}
	var foundTCP, foundIO bool
	for _, sm := range rm.ScopeMetrics {
		for _, m := range sm.Metrics {
			switch m.Name {
			case "gcp.storage.client.network.tcp.connect.duration":
				foundTCP = true
				for _, tdp := range m.Data.(metricdata.Histogram[float64]).DataPoints {
					if got := getHistAttr(tdp, "network.path"); got != networkPathCloud {
						t.Errorf("tcp.connect.duration network.path = %q, want %q", got, networkPathCloud)
					}
				}
			case "gcp.storage.client.network.io":
				foundIO = true
				for _, idp := range m.Data.(metricdata.Sum[int64]).DataPoints {
					if got := getAttr(idp, "network.path"); got != networkPathCloud {
						t.Errorf("network.io network.path = %q, want %q", got, networkPathCloud)
					}
				}
			}
		}
	}
	if !foundTCP {
		t.Errorf("expected gcp.storage.client.network.tcp.connect.duration to be recorded")
	}
	if !foundIO {
		t.Errorf("expected gcp.storage.client.network.io to be recorded through dialer")
	}
}

func TestKeepFilteredResourceAttribute(t *testing.T) {
	for _, tc := range []struct {
		key  string
		want bool
	}{
		{"cloud.platform", true},
		{"cloud.region", true},
		{"host.id", true},
		{"gcp.client.instance_id", true},
		{"gcp.client.version", true},
		{"cloud.availability_zone", false},
		{"service.name", false},
		{"", false},
		{"gcp.clientX", false},
	} {
		if got := keepFilteredResourceAttribute(tc.key); got != tc.want {
			t.Errorf("keepFilteredResourceAttribute(%q) = %v, want %v", tc.key, got, tc.want)
		}
	}
}

func TestMetricBaseLabels(t *testing.T) {
	toMap := func(kvs []attribute.KeyValue) map[string]string {
		m := make(map[string]string, len(kvs))
		for _, kv := range kvs {
			m[string(kv.Key)] = kv.Value.AsString()
		}
		return m
	}
	first := toMap(metricBaseLabels())
	second := toMap(metricBaseLabels())

	if len(first) != 4 {
		t.Fatalf("expected 4 base labels, got %d", len(first))
	}
	if first["gcp.client.version"] != internal.Version {
		t.Errorf("got version %q, want %q", first["gcp.client.version"], internal.Version)
	}
	if first["gcp.client.service"] != "storage" {
		t.Errorf("got service %q, want storage", first["gcp.client.service"])
	}
	if first["gcp.client.artifact"] != "cloud.google.com/go/storage" {
		t.Errorf("got artifact %q, want cloud.google.com/go/storage", first["gcp.client.artifact"])
	}
	if _, err := uuid.Parse(first["gcp.client.instance_id"]); err != nil {
		t.Errorf("expected valid UUID for gcp.client.instance_id, got %q: %v", first["gcp.client.instance_id"], err)
	}
	if first["gcp.client.instance_id"] == second["gcp.client.instance_id"] {
		t.Errorf("expected distinct instance_id values across calls, got %q", first["gcp.client.instance_id"])
	}
}

func TestInitMetrics_StandardTier(t *testing.T) {
	ctx := context.Background()
	mr := sdkmetric.NewManualReader()
	provider := sdkmetric.NewMeterProvider(sdkmetric.WithReader(mr))
	defer provider.Shutdown(ctx)

	cm, _, err := initMetrics(ctx, "project-id", &storageConfig{
		enableOtelMetrics:      true,
		enableOtelDebugMetrics: false,
		meterProvider:          provider,
	})
	if err != nil {
		t.Fatalf("initMetrics: %v", err)
	}
	if cm.serverUnreached == nil {
		t.Fatalf("expected server.unreached instrument to be initialized under standard metrics")
	}
	if cm.serverDuration != nil {
		t.Errorf("expected server.duration to remain disabled when debug metrics are off")
	}

	cmDebug, _, err := initMetrics(ctx, "project-id", &storageConfig{
		enableOtelMetrics:      true,
		enableOtelDebugMetrics: true,
		meterProvider:          provider,
	})
	if err != nil {
		t.Fatalf("initMetrics(debug): %v", err)
	}
	if cmDebug.serverUnreached == nil || cmDebug.serverDuration == nil {
		t.Fatalf("expected both server.unreached and server.duration to be initialized when debug metrics are on")
	}
}

func TestRecordServerMetrics(t *testing.T) {
	var statsBuf [10]byte
	binary.LittleEndian.PutUint64(statsBuf[2:], uint64(42*time.Millisecond))
	trailerWithStats := metadata.Pairs("grpc-server-stats-bin", string(statsBuf[:]))
	dpPeer := &peer.Peer{Addr: &net.TCPAddr{IP: net.ParseIP("34.126.0.10"), Port: 443}}
	cpPeer := &peer.Peer{Addr: &net.TCPAddr{IP: net.ParseIP("142.250.1.1"), Port: 443}}

	httpRespWithUploadID := &http.Response{Header: make(http.Header)}
	httpRespWithUploadID.Header.Set(uploadIDHeader, "upload-123")
	httpRespWithUploadID.Header.Set("X-Goog-Gfe-Service-Time", "bad-float")

	for _, tc := range []struct {
		name            string
		debug           bool
		run             func(ctx context.Context, cm *clientMetrics)
		wantUnreached   int64
		wantDuration    float64
		wantNetworkPath string
	}{
		{
			name:  "DirectPath with grpc-server-stats-bin",
			debug: true,
			run: func(ctx context.Context, cm *clientMetrics) {
				cm.recordServerMetrics(ctx, nil, trailerWithStats, nil, "GetObject", "storage.googleapis.com", dpPeer)
			},
			wantUnreached:   0,
			wantDuration:    0.042,
			wantNetworkPath: networkPathDirect,
		},
		{
			name:  "DirectPath success without headers",
			debug: true,
			run: func(ctx context.Context, cm *clientMetrics) {
				cm.recordServerMetrics(ctx, nil, nil, nil, "GetObject", "storage.googleapis.com", dpPeer)
			},
			wantUnreached: 0,
		},
		{
			name:  "DirectPath pre-response connection error",
			debug: true,
			run: func(ctx context.Context, cm *clientMetrics) {
				cm.recordServerMetrics(ctx, nil, nil, errors.New("connection refused"), "GetObject", "storage.googleapis.com", dpPeer)
			},
			wantUnreached:   1,
			wantNetworkPath: networkPathDirect,
		},
		{
			name:  "DirectPath ambiguous client timeout",
			debug: true,
			run: func(ctx context.Context, cm *clientMetrics) {
				cm.recordServerMetrics(ctx, nil, nil, status.Error(codes.DeadlineExceeded, "context deadline exceeded"), "GetObject", "storage.googleapis.com", dpPeer)
			},
			wantUnreached: 0,
		},
		{
			name:  "DirectPath unavailable without metadata skipped as ambiguous",
			debug: true,
			run: func(ctx context.Context, cm *clientMetrics) {
				cm.recordServerMetrics(ctx, nil, nil, status.Error(codes.Unavailable, "transport is closing"), "GetObject", "storage.googleapis.com", dpPeer)
			},
			wantUnreached: 0,
		},
		{
			name:  "CloudPath ambiguous client timeout",
			debug: true,
			run: func(ctx context.Context, cm *clientMetrics) {
				cm.recordServerMetrics(ctx, nil, nil, status.Error(codes.DeadlineExceeded, "context deadline exceeded"), "GetObject", "storage.googleapis.com", cpPeer)
			},
			wantUnreached: 0,
		},
		{
			name:  "CloudPath service deadline exceeded",
			debug: true,
			run: func(ctx context.Context, cm *clientMetrics) {
				cm.recordServerMetrics(ctx, nil, nil, status.Error(codes.DeadlineExceeded, "Deadline exceeded."), "GetObject", "storage.googleapis.com", cpPeer)
			},
			wantUnreached: 0,
		},
		{
			name:  "CloudPath with upload ID and malformed GFE/stats headers",
			debug: true,
			run: func(ctx context.Context, cm *clientMetrics) {
				cm.recordServerMetrics(ctx, metadata.Pairs(uploadIDHeader, "abc", "x-goog-gfe-service-time", "not-a-number"), metadata.Pairs("grpc-server-stats-bin", "short"), nil, "GetObject", "storage.googleapis.com", cpPeer)
			},
			wantUnreached: 0,
		},
		{
			name:  "CloudPath failure without headers",
			debug: true,
			run: func(ctx context.Context, cm *clientMetrics) {
				cm.recordServerMetrics(ctx, nil, nil, errors.New("connection refused"), "GetObject", "storage.googleapis.com", cpPeer)
			},
			wantUnreached:   1,
			wantNetworkPath: networkPathCloud,
		},
		{
			name:  "Nil peer failure without headers",
			debug: true,
			run: func(ctx context.Context, cm *clientMetrics) {
				cm.recordServerMetrics(ctx, nil, nil, errors.New("connection refused"), "GetObject", "storage.googleapis.com", nil)
			},
			wantUnreached:   1,
			wantNetworkPath: networkPathUnknown,
		},
		{
			name:  "HTTP with upload ID and malformed GFE header",
			debug: true,
			run: func(ctx context.Context, cm *clientMetrics) {
				cm.recordHTTPServerMetrics(ctx, httpRespWithUploadID, "GetObject", "storage.googleapis.com", "OK")
			},
			wantUnreached: 0,
		},
		{
			name:  "HTTP response without headers and debug off",
			debug: false,
			run: func(ctx context.Context, cm *clientMetrics) {
				cm.recordHTTPServerMetrics(ctx, &http.Response{Header: make(http.Header)}, "GetObject", "storage.googleapis.com", "UNAVAILABLE")
			},
			wantUnreached:   1,
			wantNetworkPath: networkPathCloud,
		},
		{
			name:  "HTTP 504 response without GFE headers",
			debug: true,
			run: func(ctx context.Context, cm *clientMetrics) {
				cm.recordHTTPServerMetrics(ctx, &http.Response{StatusCode: http.StatusGatewayTimeout, Header: make(http.Header)}, "GetObject", "storage.googleapis.com", "DEADLINE_EXCEEDED")
			},
			wantUnreached:   1,
			wantNetworkPath: networkPathCloud,
		},
		{
			name:  "HTTP nil response connection error",
			debug: true,
			run: func(ctx context.Context, cm *clientMetrics) {
				cm.recordHTTPServerMetrics(ctx, nil, "GetObject", "storage.googleapis.com", "CONNECTION_ERROR")
			},
			wantUnreached:   1,
			wantNetworkPath: networkPathCloud,
		},
		{
			name:  "HTTP nil response ambiguous client cancel",
			debug: true,
			run: func(ctx context.Context, cm *clientMetrics) {
				cm.recordHTTPServerMetrics(ctx, nil, "GetObject", "storage.googleapis.com", "CANCELLED")
			},
			wantUnreached: 0,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ctx := context.Background()
			mr := sdkmetric.NewManualReader()
			provider := sdkmetric.NewMeterProvider(sdkmetric.WithReader(mr))
			defer provider.Shutdown(ctx)

			cm, _, err := initMetrics(ctx, "project-id", &storageConfig{
				enableOtelMetrics:      true,
				enableOtelDebugMetrics: tc.debug,
				meterProvider:          provider,
			})
			if err != nil {
				t.Fatalf("initMetrics: %v", err)
			}
			tc.run(ctx, cm)

			var rm metricdata.ResourceMetrics
			if err := mr.Collect(ctx, &rm); err != nil {
				t.Fatalf("Collect: %v", err)
			}
			var gotUnreached int64
			var gotDuration float64
			var gotDurationCount uint64
			for _, sm := range rm.ScopeMetrics {
				for _, m := range sm.Metrics {
					if m.Name == "gcp.storage.client.server.unreached" {
						for _, dp := range m.Data.(metricdata.Sum[int64]).DataPoints {
							gotUnreached += dp.Value
							assertExactAttrKeys(t, m.Name, dp.Attributes,
								"error.type", "gcp.storage.operation", "network.path", "rpc.system.name", "server.address")
							if tc.wantNetworkPath != "" {
								if got := getAttr(dp, "network.path"); got != tc.wantNetworkPath {
									t.Errorf("server.unreached network.path = %q, want %q", got, tc.wantNetworkPath)
								}
							}
						}
					}
					if m.Name == "gcp.storage.client.server.duration" {
						for _, dp := range m.Data.(metricdata.Histogram[float64]).DataPoints {
							gotDurationCount += dp.Count
							gotDuration += dp.Sum
							assertExactAttrKeys(t, m.Name, dp.Attributes,
								"gcp.storage.operation", "network.path", "rpc.system.name", "server.address")
							if tc.wantNetworkPath != "" {
								if got := getHistAttr(dp, "network.path"); got != tc.wantNetworkPath {
									t.Errorf("server.duration network.path = %q, want %q", got, tc.wantNetworkPath)
								}
							}
						}
					}
				}
			}
			if gotUnreached != tc.wantUnreached {
				t.Errorf("server.unreached = %d, want %d", gotUnreached, tc.wantUnreached)
			}
			if tc.wantDuration == 0 && gotDurationCount != 0 {
				t.Errorf("server.duration count = %d, want 0", gotDurationCount)
			}
			if tc.wantDuration > 0 && (gotDurationCount != 1 || gotDuration < tc.wantDuration-0.001 || gotDuration > tc.wantDuration+0.001) {
				t.Errorf("server.duration = %v (count=%d), want %v", gotDuration, gotDurationCount, tc.wantDuration)
			}
		})
	}
}

func TestMetricUnits(t *testing.T) {
	ctx := context.Background()
	mr := sdkmetric.NewManualReader()
	provider := sdkmetric.NewMeterProvider(sdkmetric.WithReader(mr))
	defer provider.Shutdown(ctx)

	cm, _, err := initMetrics(ctx, "project-id", &storageConfig{
		enableOtelMetrics:      true,
		enableOtelDebugMetrics: true,
		meterProvider:          provider,
	})
	if err != nil {
		t.Fatalf("initMetrics: %v", err)
	}
	cm.operations.Add(ctx, 1)
	cm.attempts.Add(ctx, 1)
	cm.errors.Add(ctx, 1)
	cm.serverUnreached.Add(ctx, 1)
	cm.activeRequests.Add(ctx, 1)

	var rm metricdata.ResourceMetrics
	if err := mr.Collect(ctx, &rm); err != nil {
		t.Fatalf("Collect: %v", err)
	}
	units := make(map[string]string)
	for _, sm := range rm.ScopeMetrics {
		for _, m := range sm.Metrics {
			units[m.Name] = m.Unit
		}
	}
	for name, wantUnit := range map[string]string{
		"gcp.storage.client.operations":       "{operation}",
		"gcp.storage.client.attempts":         "{attempt}",
		"gcp.storage.client.errors":           "{error}",
		"gcp.storage.client.server.unreached": "{request}",
		"gcp.storage.client.request.active":   "{request}",
	} {
		if units[name] != wantUnit {
			t.Errorf("metric %s unit = %q, want %q", name, units[name], wantUnit)
		}
	}
}

func TestMonitoringClientOptions(t *testing.T) {
	t.Setenv("GOOGLE_CLOUD_UNIVERSE_DOMAIN", "")
	t.Setenv("GOOGLE_API_USE_MTLS_ENDPOINT", "")

	t.Run("disabled_without_authentication", func(t *testing.T) {
		if _, ok := monitoringClientOptions([]option.ClientOption{option.WithoutAuthentication()}, nil, nil); ok {
			t.Fatal("monitoringClientOptions() ok = true, want false with WithoutAuthentication")
		}
	})

	t.Run("disabled_on_custom_transport_or_api_key_or_nil_creds", func(t *testing.T) {
		ambientCreds := &auth.Credentials{TokenProvider: &staticAuthTokenProvider{token: &auth.Token{Value: "ambient"}}}
		if _, ok := monitoringClientOptions([]option.ClientOption{option.WithHTTPClient(http.DefaultClient)}, ambientCreds, nil); ok {
			t.Fatal("monitoringClientOptions() ok = true, want false with custom HTTP client")
		}
		if _, ok := monitoringClientOptions([]option.ClientOption{option.WithGRPCConn(&grpc.ClientConn{})}, ambientCreds, nil); ok {
			t.Fatal("monitoringClientOptions() ok = true, want false with custom gRPC conn")
		}
		if _, ok := monitoringClientOptions([]option.ClientOption{option.WithAPIKey("test-api-key")}, ambientCreds, nil); ok {
			t.Fatal("monitoringClientOptions() ok = true, want false with API key")
		}
		if _, ok := monitoringClientOptions([]option.ClientOption{option.WithUniverseDomain("custom.example.com")}, nil, nil); ok {
			t.Fatal("monitoringClientOptions() ok = true, want false when resolved credentials are nil")
		}
	})

	t.Run("forwards_universe_domain_and_cloned_auth_creds", func(t *testing.T) {
		parentCreds := &auth.Credentials{TokenProvider: &staticAuthTokenProvider{token: &auth.Token{Value: "tok"}}}
		clientOpts := []option.ClientOption{
			option.WithUniverseDomain("custom.example.com"),
			option.WithEndpoint("https://storage.custom.example.com/storage/v1/"),
		}

		mOpts, ok := monitoringClientOptions(clientOpts, parentCreds, nil)
		if !ok {
			t.Fatal("monitoringClientOptions() ok = false, want true")
		}

		res, err := internaloption.NewUnsafeResolver(mOpts...)
		if err != nil {
			t.Fatalf("NewUnsafeResolver: %v", err)
		}
		if got := res.ResolvedUniverseDomain(); got != "custom.example.com" {
			t.Errorf("ResolvedUniverseDomain() = %q, want %q", got, "custom.example.com")
		}

		// Mutate parentCreds in place (as http_client.go does when wrapping) and ensure
		// the forwarded Monitoring credentials remain untouched.
		parentCreds.TokenProvider = nil
		gotCreds, err := internaloption.AuthCreds(context.Background(), mOpts)
		if err != nil {
			t.Fatalf("internaloption.AuthCreds: %v", err)
		}
		if gotCreds == nil || gotCreds.TokenProvider == nil {
			t.Fatal("forwarded auth credentials were nil or mutated by parent in-place update")
		}
	})

	t.Run("forwards_legacy_google_creds", func(t *testing.T) {
		gCreds := &google.Credentials{
			ProjectID:   "test-proj",
			TokenSource: oauth2.StaticTokenSource(&oauth2.Token{AccessToken: "legacy-tok"}),
		}
		mOpts, ok := monitoringClientOptions([]option.ClientOption{option.WithUniverseDomain("custom.example.com")}, nil, gCreds)
		if !ok {
			t.Fatal("monitoringClientOptions() ok = false, want true")
		}
		gotCreds, err := transport.Creds(context.Background(), mOpts...)
		if err != nil {
			t.Fatalf("transport.Creds: %v", err)
		}
		if gotCreds != gCreds {
			t.Errorf("resolved credentials = %v, want caller's legacy credentials", gotCreds)
		}
	})

	t.Run("disabled_on_endpoint_only_non_googleapis_without_universe", func(t *testing.T) {
		ambientCreds := &auth.Credentials{TokenProvider: &staticAuthTokenProvider{token: &auth.Token{Value: "ambient"}}}
		if _, ok := monitoringClientOptions([]option.ClientOption{option.WithEndpoint("storage.custom-tpc.goog:443")}, ambientCreds, nil); ok {
			t.Fatal("monitoringClientOptions() ok = true, want false for endpoint-only non-googleapis.com host")
		}
		if _, ok := monitoringClientOptions([]option.ClientOption{option.WithEndpoint("https://storage.custom-tpc.goog/storage/v1/")}, ambientCreds, nil); ok {
			t.Fatal("monitoringClientOptions() ok = true, want false for HTTPS endpoint-only non-googleapis.com host")
		}
	})

	t.Run("enabled_on_regional_googleapis_endpoint", func(t *testing.T) {
		ambientCreds := &auth.Credentials{TokenProvider: &staticAuthTokenProvider{token: &auth.Token{Value: "ambient"}}}
		if _, ok := monitoringClientOptions([]option.ClientOption{option.WithEndpoint("https://us-central1-storage.googleapis.com/storage/v1/")}, ambientCreds, nil); !ok {
			t.Fatal("monitoringClientOptions() ok = false, want true for regional googleapis.com endpoint")
		}
	})
}

func TestInitMetrics_EmptyProjectIDReturnsError(t *testing.T) {
	if gcemetadata.OnGCE() {
		t.Skip("skipping on GCE where metadata detector populates project ID")
	}
	// Ensure initMetrics fails fast instead of passing an empty project ID to
	// mexporter.New (which would fall back to google.FindDefaultCredentials).
	_, _, err := initMetrics(context.Background(), "", &storageConfig{})
	if err == nil {
		t.Fatal("initMetrics() with empty projectID = nil, want error")
	}
	if !strings.Contains(err.Error(), "project is required") {
		t.Errorf("initMetrics() error = %q, want substring %q", err.Error(), "project is required")
	}
}

func TestNewClient_PreservesDefaultHTTPMonitoringOptions(t *testing.T) {
	t.Setenv("STORAGE_EMULATOR_HOST", "")
	t.Setenv("GOOGLE_CLOUD_UNIVERSE_DOMAIN", "")
	ctx := context.Background()

	creds := auth.NewCredentials(&auth.CredentialsOptions{
		TokenProvider:     &staticAuthTokenProvider{token: &auth.Token{Value: "tok"}},
		ProjectIDProvider: auth.CredentialsPropertyFunc(func(context.Context) (string, error) { return "test-proj", nil }),
	})

	// Standard NewClient (without caller-supplied WithHTTPClient) must NOT treat
	// its own internal htransport.NewClient as a custom HTTP client: exportOK
	// must be true so default OTel metrics initialize with monitoringOpts.
	c, err := NewClient(ctx,
		option.WithAuthCredentials(creds),
		option.WithUniverseDomain("custom.example.com"),
		withOtelMetrics(),
	)
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}
	defer func() { _ = c.Close() }()

	msc, ok := c.tc.(*metricsStorageClient)
	if !ok {
		t.Fatalf("c.tc = %T, want *metricsStorageClient", c.tc)
	}
	hsc, ok := msc.storageClient.(*httpStorageClient)
	if !ok {
		t.Fatalf("msc.storageClient = %T, want *httpStorageClient", msc.storageClient)
	}
	if len(hsc.config.monitoringOpts) == 0 {
		t.Fatal("expected NewClient to populate monitoringOpts on default HTTP transport, got empty")
	}
	res, err := internaloption.NewUnsafeResolver(hsc.config.monitoringOpts...)
	if err != nil {
		t.Fatalf("NewUnsafeResolver: %v", err)
	}
	if got := res.ResolvedUniverseDomain(); got != "custom.example.com" {
		t.Errorf("ResolvedUniverseDomain() = %q, want %q", got, "custom.example.com")
	}

	// Conversely, when the caller explicitly passes option.WithHTTPClient,
	// default Cloud Monitoring export must be skipped (c.tc stays unwrapped).
	customHC, err := NewClient(ctx,
		option.WithHTTPClient(&http.Client{}),
		withOtelMetrics(),
	)
	if err != nil {
		t.Fatalf("NewClient(WithHTTPClient): %v", err)
	}
	defer func() { _ = customHC.Close() }()
	if _, wrapped := customHC.tc.(*metricsStorageClient); wrapped {
		t.Error("expected default OTel metrics to be skipped when caller supplies option.WithHTTPClient")
	}
}

func TestMetricsState_RecordBodySizeZeroAndOnce(t *testing.T) {
	mr := sdkmetric.NewManualReader()
	provider := sdkmetric.NewMeterProvider(sdkmetric.WithReader(mr))
	defer provider.Shutdown(context.Background())
	cfg := storageConfig{enableOtelMetrics: true, meterProvider: provider}
	cm, _, err := initMetrics(context.Background(), "test-proj", &cfg)
	if err != nil {
		t.Fatalf("initMetrics: %v", err)
	}

	ctx := withAPIMethod(context.Background(), "Object.NewWriter")
	ctx, _ = cm.startOperation(ctx, "WriteObject", false)
	state := metricsStateFromContext(ctx)

	// Errors must not record body size, leaving the once-guard open for success.
	state.recordRequestBodySize(ctx, "WriteObject", 0, errors.New("boom"))
	// 0 B success records once; duplicate calls are ignored.
	state.recordRequestBodySize(ctx, "WriteObject", 0, nil)
	state.recordRequestBodySize(ctx, "WriteObject", 0, nil)

	// Exercise real 0 B Reader.Close() and double-Close() wiring.
	rCtx := withAPIMethod(context.Background(), "Object.NewReader")
	rCtx, _ = cm.startOperation(rCtx, "ReadObject", false)
	rdr := &Reader{
		ctx:          rCtx,
		reader:       io.NopCloser(bytes.NewReader(nil)),
		metricsState: metricsStateFromContext(rCtx),
	}
	if err := rdr.Close(); err != nil {
		t.Fatalf("Reader.Close: %v", err)
	}
	_ = rdr.Close()

	// Exercise real 0 B Writer.markClosed() and double-close wiring.
	wCtx := withAPIMethod(context.Background(), "Object.NewWriter")
	wCtx, _ = cm.startOperation(wCtx, "WriteObject", false)
	wr := &Writer{ctx: wCtx}
	if err := wr.markClosed(nil); err != nil {
		t.Fatalf("Writer.markClosed: %v", err)
	}
	_ = wr.markClosed(nil)

	var rm metricdata.ResourceMetrics
	if err := mr.Collect(ctx, &rm); err != nil {
		t.Fatalf("Collect: %v", err)
	}
	var reqCount, respCount uint64
	for _, sm := range rm.ScopeMetrics {
		for _, m := range sm.Metrics {
			switch m.Name {
			case "gcp.storage.client.request.body.size":
				for _, dp := range m.Data.(metricdata.Histogram[int64]).DataPoints {
					reqCount += dp.Count
					if dp.Sum != 0 {
						t.Errorf("request.body.size sum = %d, want 0", dp.Sum)
					}
				}
			case "gcp.storage.client.response.body.size":
				for _, dp := range m.Data.(metricdata.Histogram[int64]).DataPoints {
					respCount += dp.Count
					if dp.Sum != 0 {
						t.Errorf("response.body.size sum = %d, want 0", dp.Sum)
					}
				}
			}
		}
	}
	if reqCount != 2 {
		t.Errorf("request.body.size count = %d, want 2", reqCount)
	}
	if respCount != 1 {
		t.Errorf("response.body.size count = %d, want 1", respCount)
	}
}

func TestWrappedClientStream_SendMsgEOFDoesNotRecordOK(t *testing.T) {
	mr := sdkmetric.NewManualReader()
	provider := sdkmetric.NewMeterProvider(sdkmetric.WithReader(mr))
	defer provider.Shutdown(context.Background())
	cfg := storageConfig{enableOtelMetrics: true, enableOtelDebugMetrics: true, meterProvider: provider}
	cm, _, err := initMetrics(context.Background(), "test-proj", &cfg)
	if err != nil {
		t.Fatalf("initMetrics: %v", err)
	}

	_, streamInt := metricsInterceptors(cm)
	mock := &mockClientStream{
		sendErr: io.EOF,
		recvErr: status.Error(codes.NotFound, "object not found"),
	}
	cs, err := streamInt(context.Background(), &grpc.StreamDesc{ClientStreams: true, ServerStreams: true}, nil, "/google.storage.v2.Storage/BidiReadObject",
		func(ctx context.Context, desc *grpc.StreamDesc, cc *grpc.ClientConn, method string, opts ...grpc.CallOption) (grpc.ClientStream, error) {
			return mock, nil
		})
	if err != nil {
		t.Fatalf("streamInt: %v", err)
	}
	if err := cs.SendMsg(&storagepb.BidiReadObjectRequest{}); !errors.Is(err, io.EOF) {
		t.Fatalf("SendMsg err = %v, want io.EOF", err)
	}
	if err := cs.RecvMsg(&storagepb.BidiReadObjectResponse{}); status.Code(err) != codes.NotFound {
		t.Fatalf("RecvMsg err = %v, want NotFound", err)
	}

	// Server-streaming RPC (ClientStreams: false) returns immediately on SendMsg
	// io.EOF without calling RecvMsg, so SendMsg must record to avoid leaking activeRequests.
	ss, err := streamInt(context.Background(), &grpc.StreamDesc{ClientStreams: false, ServerStreams: true}, nil, "/google.storage.v2.Storage/ReadObject",
		func(ctx context.Context, desc *grpc.StreamDesc, cc *grpc.ClientConn, method string, opts ...grpc.CallOption) (grpc.ClientStream, error) {
			return &mockClientStream{sendErr: io.EOF}, nil
		})
	if err != nil {
		t.Fatalf("streamInt (server-streaming): %v", err)
	}
	if err := ss.SendMsg(&storagepb.ReadObjectRequest{}); !errors.Is(err, io.EOF) {
		t.Fatalf("server-streaming SendMsg err = %v, want io.EOF", err)
	}

	var rm metricdata.ResourceMetrics
	if err := mr.Collect(context.Background(), &rm); err != nil {
		t.Fatalf("Collect: %v", err)
	}
	var errCount, activeSum, notFoundAttempts int64
	for _, sm := range rm.ScopeMetrics {
		for _, m := range sm.Metrics {
			switch m.Name {
			case "gcp.storage.client.attempts":
				for _, dp := range m.Data.(metricdata.Sum[int64]).DataPoints {
					if getAttr(dp, "error.type") == "NOT_FOUND" {
						notFoundAttempts += dp.Value
					}
				}
			case "gcp.storage.client.errors":
				for _, dp := range m.Data.(metricdata.Sum[int64]).DataPoints {
					errCount += dp.Value
					if got := getAttr(dp, "error.type"); got != "NOT_FOUND" {
						t.Errorf("errors error.type = %q, want NOT_FOUND", got)
					}
				}
			case "gcp.storage.client.request.active":
				for _, dp := range m.Data.(metricdata.Sum[int64]).DataPoints {
					activeSum += dp.Value
				}
			}
		}
	}
	if notFoundAttempts != 1 {
		t.Errorf("NOT_FOUND attempts = %d, want 1", notFoundAttempts)
	}
	if errCount != 1 {
		t.Errorf("errors count = %d, want 1", errCount)
	}
	if activeSum != 0 {
		t.Errorf("request.active net = %d, want 0", activeSum)
	}
}

type stringAddr string

func (a stringAddr) Network() string { return "tcp" }
func (a stringAddr) String() string  { return string(a) }

func TestNetworkPathOfAddr(t *testing.T) {
	for _, tc := range []struct {
		name string
		addr net.Addr
		want string
	}{
		{name: "nil addr", addr: nil, want: networkPathUnknown},
		{name: "tcp ipv4 directpath", addr: &net.TCPAddr{IP: net.IPv4(34, 126, 10, 53), Port: 443}, want: networkPathDirect},
		{name: "tcp ipv4-mapped ipv6 directpath", addr: &net.TCPAddr{IP: net.ParseIP("::ffff:34.126.1.1"), Port: 443}, want: networkPathDirect},
		{name: "tcp ipv6 directpath", addr: &net.TCPAddr{IP: net.ParseIP("2001:4860:8040::1"), Port: 443}, want: networkPathDirect},
		{name: "tcp cloudpath", addr: &net.TCPAddr{IP: net.IPv4(74, 125, 126, 207), Port: 443}, want: networkPathCloud},
		{name: "non-tcp directpath", addr: stringAddr("34.126.0.1:443"), want: networkPathDirect},
		{name: "non-tcp cloudpath", addr: stringAddr("storage.googleapis.com:443"), want: networkPathCloud},
	} {
		if got := networkPathOfAddr(tc.addr); got != tc.want {
			t.Errorf("%s: networkPathOfAddr(%v) = %q, want %q", tc.name, tc.addr, got, tc.want)
		}
	}
}

type multiRecvMockStream struct {
	mockClientStream
	calls   int
	wantErr error
}

func (m *multiRecvMockStream) RecvMsg(msg interface{}) error {
	m.calls++
	if m.calls <= 2 {
		if bufs, ok := msg.(*mem.BufferSlice); ok {
			b := []byte("chunk")
			*bufs = append(*bufs, mem.NewBuffer(&b, nil))
		}
		return nil
	}
	return m.wantErr
}

func TestDrainStreamSendEOF(t *testing.T) {
	wantErr := status.Error(codes.FailedPrecondition, "generation mismatch")
	if err := drainStreamSendEOF(&mockClientStream{recvErr: wantErr}, io.EOF, &storagepb.BidiReadObjectResponse{}); !errors.Is(err, wantErr) {
		t.Errorf("drainStreamSendEOF(io.EOF) = %v, want %v", err, wantErr)
	}
	otherErr := errors.New("broken pipe")
	if err := drainStreamSendEOF(&mockClientStream{recvErr: wantErr}, otherErr, &storagepb.BidiReadObjectResponse{}); !errors.Is(err, otherErr) {
		t.Errorf("drainStreamSendEOF(non-EOF) = %v, want %v", err, otherErr)
	}
	if err := drainStreamSendEOF(nil, io.EOF, &storagepb.BidiReadObjectResponse{}); !errors.Is(err, io.EOF) {
		t.Errorf("drainStreamSendEOF(nil stream) = %v, want io.EOF", err)
	}
	var bufs mem.BufferSlice
	ms := &multiRecvMockStream{wantErr: wantErr}
	if err := drainStreamSendEOF(ms, io.EOF, &bufs); !errors.Is(err, wantErr) {
		t.Errorf("drainStreamSendEOF(multi-msg) = %v, want %v", err, wantErr)
	}
	if ms.calls != 3 {
		t.Errorf("RecvMsg calls = %d, want 3", ms.calls)
	}
	if len(bufs) != 0 {
		t.Errorf("expected BufferSlice to be freed/cleared, got len=%d", len(bufs))
	}
}

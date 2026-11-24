// Copyright 2026 Google LLC
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     https://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package observability

import (
	"context"
	"log/slog"
	"net"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	secretmanager "cloud.google.com/go/secretmanager/apiv1"
	secretmanagerpb "cloud.google.com/go/secretmanager/apiv1/secretmanagerpb"
	gax "github.com/googleapis/gax-go/v2"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/propagation"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
	"go.opentelemetry.io/otel/trace"
	"google.golang.org/api/option"
	"google.golang.org/grpc"
	grpccodes "google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
)

type fakeSecretManagerServer struct {
	secretmanagerpb.UnimplementedSecretManagerServiceServer
	addr         string
	mu           sync.Mutex
	traceparents []string
}

func startFakeServer(t *testing.T) *fakeSecretManagerServer {
	t.Helper()
	l, err := net.Listen("tcp", "localhost:0")
	if err != nil {
		t.Fatal(err)
	}
	srv := &fakeSecretManagerServer{addr: l.Addr().String()}
	gsrv := grpc.NewServer()
	secretmanagerpb.RegisterSecretManagerServiceServer(gsrv, srv)
	t.Cleanup(gsrv.Stop)
	go func() { _ = gsrv.Serve(l) }()
	return srv
}

func (f *fakeSecretManagerServer) GetSecret(ctx context.Context, req *secretmanagerpb.GetSecretRequest) (*secretmanagerpb.Secret, error) {
	var tp string
	if md, ok := metadata.FromIncomingContext(ctx); ok {
		tp = strings.Join(md.Get("traceparent"), ",")
	}
	f.mu.Lock()
	f.traceparents = append(f.traceparents, tp)
	f.mu.Unlock()

	if strings.HasSuffix(req.GetName(), "/ok") {
		return &secretmanagerpb.Secret{Name: req.GetName()}, nil
	}
	return nil, status.Error(grpccodes.NotFound, "secret not found")
}

func (f *fakeSecretManagerServer) takeTraceparents() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	out := slices.Clone(f.traceparents)
	f.traceparents = nil
	return out
}

type capturedLog struct {
	Level       slog.Level
	Message     string
	SpanContext trace.SpanContext
	Attrs       map[string]any
}

type memoryLogHandler struct {
	minLevel slog.Level
	preAttrs []slog.Attr
	mu       *sync.Mutex
	entries  *[]capturedLog
}

func newMemoryLogHandler(minLevel slog.Level) *memoryLogHandler {
	var mu sync.Mutex
	var entries []capturedLog
	return &memoryLogHandler{
		minLevel: minLevel,
		mu:       &mu,
		entries:  &entries,
	}
}

func (h *memoryLogHandler) Enabled(_ context.Context, level slog.Level) bool {
	return level >= h.minLevel
}

func (h *memoryLogHandler) Handle(ctx context.Context, r slog.Record) error {
	attrs := make(map[string]any, len(h.preAttrs)+r.NumAttrs())
	for _, a := range h.preAttrs {
		attrs[a.Key] = a.Value.Any()
	}
	r.Attrs(func(a slog.Attr) bool {
		attrs[a.Key] = a.Value.Any()
		return true
	})

	h.mu.Lock()
	defer h.mu.Unlock()
	*h.entries = append(*h.entries, capturedLog{
		Level:       r.Level,
		Message:     r.Message,
		SpanContext: trace.SpanContextFromContext(ctx),
		Attrs:       attrs,
	})
	return nil
}

func (h *memoryLogHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	return &memoryLogHandler{
		minLevel: h.minLevel,
		preAttrs: append(slices.Clone(h.preAttrs), attrs...),
		mu:       h.mu,
		entries:  h.entries,
	}
}

func (h *memoryLogHandler) WithGroup(_ string) slog.Handler { return h }

func (h *memoryLogHandler) Entries() []capturedLog {
	h.mu.Lock()
	defer h.mu.Unlock()
	return slices.Clone(*h.entries)
}

type testEnv struct {
	client       *secretmanager.Client
	fakeSrv      *fakeSecretManagerServer
	traceExp     *tracetest.InMemoryExporter
	metricReader *sdkmetric.ManualReader
	logSink      *memoryLogHandler
}

func newTestEnv(t *testing.T, fakeSrv *fakeSecretManagerServer, minLogLevel slog.Level) *testEnv {
	t.Helper()
	_ = fakeSrv.takeTraceparents()

	traceExp := setupTestTracing(t)
	metricReader := setupTestMetrics(t)
	logSink := newMemoryLogHandler(minLogLevel)
	client := newFakeClient(t, fakeSrv.addr, slog.New(logSink))

	return &testEnv{
		client:       client,
		fakeSrv:      fakeSrv,
		traceExp:     traceExp,
		metricReader: metricReader,
		logSink:      logSink,
	}
}

func setupTestTracing(t *testing.T) *tracetest.InMemoryExporter {
	t.Helper()
	prevTP := otel.GetTracerProvider()
	prevProp := otel.GetTextMapPropagator()
	t.Cleanup(func() {
		otel.SetTracerProvider(prevTP)
		otel.SetTextMapPropagator(prevProp)
	})

	exp := tracetest.NewInMemoryExporter()
	tp := sdktrace.NewTracerProvider(sdktrace.WithSyncer(exp))
	otel.SetTracerProvider(tp)
	otel.SetTextMapPropagator(propagation.TraceContext{})
	t.Cleanup(func() { _ = tp.Shutdown(context.Background()) })
	return exp
}

func setupTestMetrics(t *testing.T) *sdkmetric.ManualReader {
	t.Helper()
	prevMP := otel.GetMeterProvider()
	t.Cleanup(func() { otel.SetMeterProvider(prevMP) })

	reader := sdkmetric.NewManualReader()
	mp := sdkmetric.NewMeterProvider(sdkmetric.WithReader(reader))
	otel.SetMeterProvider(mp)
	t.Cleanup(func() { _ = mp.Shutdown(context.Background()) })
	return reader
}

func newFakeClient(t *testing.T, addr string, logger *slog.Logger) *secretmanager.Client {
	t.Helper()
	client, err := secretmanager.NewClient(context.Background(),
		option.WithEndpoint(addr),
		option.WithoutAuthentication(),
		option.WithGRPCDialOption(grpc.WithTransportCredentials(insecure.NewCredentials())),
		option.WithLogger(logger),
	)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = client.Close() })
	return client
}

func (e *testEnv) callGetSecret(secretName string, retries int) error {
	var callOpts []gax.CallOption
	if retries > 0 {
		attempts := 0
		callOpts = append(callOpts, gax.WithRetry(func() gax.Retryer {
			return gax.OnErrorFunc(gax.Backoff{Initial: time.Millisecond, Max: time.Millisecond}, func(error) bool {
				attempts++
				return attempts <= retries
			})
		}))
	}
	_, err := e.client.GetSecret(context.Background(), &secretmanagerpb.GetSecretRequest{
		Name: secretName,
	}, callOpts...)
	return err
}

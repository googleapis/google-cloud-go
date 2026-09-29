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
	"fmt"
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
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/propagation"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/metric/metricdata"
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
	mu           sync.Mutex
	traceparents []string
}

func (f *fakeSecretManagerServer) GetSecret(ctx context.Context, req *secretmanagerpb.GetSecretRequest) (*secretmanagerpb.Secret, error) {
	if md, ok := metadata.FromIncomingContext(ctx); ok {
		if vals := md.Get("traceparent"); len(vals) > 0 {
			f.mu.Lock()
			f.traceparents = append(f.traceparents, vals[0])
			f.mu.Unlock()
		}
	}
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

type logStore struct {
	mu      sync.Mutex
	entries []capturedLog
}

type memoryLogHandler struct {
	minLevel slog.Level
	preAttrs []slog.Attr
	store    *logStore
}

func newMemoryLogHandler(minLevel slog.Level) *memoryLogHandler {
	return &memoryLogHandler{
		minLevel: minLevel,
		store:    &logStore{},
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

	h.store.mu.Lock()
	defer h.store.mu.Unlock()
	h.store.entries = append(h.store.entries, capturedLog{
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
		store:    h.store,
	}
}

func (h *memoryLogHandler) WithGroup(_ string) slog.Handler { return h }

func (h *memoryLogHandler) Entries() []capturedLog {
	h.store.mu.Lock()
	defer h.store.mu.Unlock()
	return slices.Clone(h.store.entries)
}

func spanAttrs(attrs []attribute.KeyValue) map[string]any {
	m := make(map[string]any, len(attrs))
	for _, kv := range attrs {
		m[string(kv.Key)] = kv.Value.AsInterface()
	}
	return m
}

type testEnv struct {
	client       *secretmanager.Client
	fakeSrv      *fakeSecretManagerServer
	traceExp     *tracetest.InMemoryExporter
	metricReader *sdkmetric.ManualReader
	logSink      *memoryLogHandler
}

func newTestEnv(t *testing.T, addr string, fakeSrv *fakeSecretManagerServer, minLogLevel slog.Level) *testEnv {
	t.Helper()
	_ = fakeSrv.takeTraceparents()

	prevTP := otel.GetTracerProvider()
	prevMP := otel.GetMeterProvider()
	prevProp := otel.GetTextMapPropagator()
	t.Cleanup(func() {
		otel.SetTracerProvider(prevTP)
		otel.SetMeterProvider(prevMP)
		otel.SetTextMapPropagator(prevProp)
	})

	traceExp := tracetest.NewInMemoryExporter()
	tp := sdktrace.NewTracerProvider(sdktrace.WithSyncer(traceExp))
	otel.SetTracerProvider(tp)
	otel.SetTextMapPropagator(propagation.TraceContext{})
	t.Cleanup(func() { _ = tp.Shutdown(context.Background()) })

	metricReader := sdkmetric.NewManualReader()
	mp := sdkmetric.NewMeterProvider(sdkmetric.WithReader(metricReader))
	otel.SetMeterProvider(mp)
	t.Cleanup(func() { _ = mp.Shutdown(context.Background()) })

	logSink := newMemoryLogHandler(minLogLevel)
	client, err := secretmanager.NewClient(context.Background(),
		option.WithEndpoint(addr),
		option.WithoutAuthentication(),
		option.WithGRPCDialOption(grpc.WithTransportCredentials(insecure.NewCredentials())),
		option.WithLogger(slog.New(logSink)),
	)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = client.Close() })

	return &testEnv{
		client:       client,
		fakeSrv:      fakeSrv,
		traceExp:     traceExp,
		metricReader: metricReader,
		logSink:      logSink,
	}
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

func (e *testEnv) assertSpans(t *testing.T, wantAttempts int, wantStatus codes.Code, wantErrorType any) (t3 *tracetest.SpanStub, t4s []*tracetest.SpanStub) {
	t.Helper()
	for _, s := range e.traceExp.GetSpans() {
		switch s.Name {
		case "SecretManager.GetSecret":
			t3 = &s
		case "google.cloud.secretmanager.v1.SecretManagerService/GetSecret":
			t4s = append(t4s, &s)
		}
	}
	if t3 == nil || len(t4s) != wantAttempts {
		t.Fatalf("got t3=%v, len(t4s)=%d; want non-nil t3 and %d t4 spans", t3, len(t4s), wantAttempts)
	}
	if t3.SpanKind != trace.SpanKindClient || t3.Status.Code != wantStatus {
		t.Errorf("t3 kind=%v status=%v, want Client/%v", t3.SpanKind, t3.Status.Code, wantStatus)
	}
	if gotErrType := spanAttrs(t3.Attributes)["error.type"]; gotErrType != wantErrorType {
		t.Errorf("t3 error.type=%v, want %v", gotErrType, wantErrorType)
	}

	traceparents := e.fakeSrv.takeTraceparents()
	if len(traceparents) != wantAttempts {
		t.Fatalf("got %d incoming traceparents, want %d", len(traceparents), wantAttempts)
	}
	for i, t4 := range t4s {
		if t4.Parent.SpanID() != t3.SpanContext.SpanID() {
			t.Errorf("t4[%d] parent=%v, want t3 spanID=%v", i, t4.Parent.SpanID(), t3.SpanContext.SpanID())
		}
		wantTP := fmt.Sprintf("00-%s-%s-01", t4.SpanContext.TraceID(), t4.SpanContext.SpanID())
		if traceparents[i] != wantTP {
			t.Errorf("traceparents[%d]=%q, want %q", i, traceparents[i], wantTP)
		}
	}
	return t3, t4s
}

func (e *testEnv) assertLogs(t *testing.T, t3 *tracetest.SpanStub, t4s []*tracetest.SpanStub, wantWarn, wantDebug int, wantErrorType any, retries int) {
	t.Helper()
	var l3WarnLogs, l4DebugLogs []capturedLog
	for _, entry := range e.logSink.Entries() {
		if entry.Level == slog.LevelWarn {
			l3WarnLogs = append(l3WarnLogs, entry)
		} else if entry.Level == slog.LevelDebug && entry.Attrs["error.type"] != nil {
			l4DebugLogs = append(l4DebugLogs, entry)
		}
	}
	if len(l3WarnLogs) != wantWarn || len(l4DebugLogs) != wantDebug {
		t.Fatalf("got %d WARN and %d DEBUG error logs, want %d and %d", len(l3WarnLogs), len(l4DebugLogs), wantWarn, wantDebug)
	}

	for _, l3 := range l3WarnLogs {
		if l3.Message != "gcp.client.request" || l3.Attrs["error.type"] != wantErrorType || l3.Attrs["resend_count"] != int64(retries) {
			t.Errorf("unexpected L3 WARN log: msg=%q attrs=%v", l3.Message, l3.Attrs)
		}
		if l3.SpanContext.TraceID() != t3.SpanContext.TraceID() || l3.SpanContext.SpanID() != t3.SpanContext.SpanID() {
			t.Errorf("L3 log trace/span=%s/%s, want %s/%s", l3.SpanContext.TraceID(), l3.SpanContext.SpanID(), t3.SpanContext.TraceID(), t3.SpanContext.SpanID())
		}
	}

	for i, l4 := range l4DebugLogs {
		if l4.SpanContext.TraceID() != t3.SpanContext.TraceID() || l4.SpanContext.SpanID() != t4s[i].SpanContext.SpanID() {
			t.Errorf("L4 log[%d] trace/span=%s/%s, want %s/%s", i, l4.SpanContext.TraceID(), l4.SpanContext.SpanID(), t3.SpanContext.TraceID(), t4s[i].SpanContext.SpanID())
		}
		if l4.Attrs["gcp.client.service"] != "secretmanager" {
			t.Errorf("L4 log[%d] gcp.client.service=%v, want secretmanager", i, l4.Attrs["gcp.client.service"])
		}
	}
}

func (e *testEnv) assertDurationMetric(t *testing.T) {
	t.Helper()
	var rm metricdata.ResourceMetrics
	if err := e.metricReader.Collect(context.Background(), &rm); err != nil {
		t.Fatalf("metricReader.Collect: %v", err)
	}
	for _, sm := range rm.ScopeMetrics {
		for _, m := range sm.Metrics {
			if m.Name == "gcp.client.request.duration" {
				return
			}
		}
	}
	t.Fatal("expected gcp.client.request.duration metric")
}

func TestObservabilitySignals(t *testing.T) {
	t.Setenv("GOOGLE_SDK_GO_METRICS", "true")
	t.Setenv("GOOGLE_SDK_GO_TRACING", "true")
	t.Setenv("GOOGLE_SDK_GO_LOGGING", "true")
	gax.TestOnlyResetIsFeatureEnabled()
	t.Cleanup(gax.TestOnlyResetIsFeatureEnabled)

	l, err := net.Listen("tcp", "localhost:0")
	if err != nil {
		t.Fatal(err)
	}
	fakeSrv := &fakeSecretManagerServer{}
	gsrv := grpc.NewServer()
	secretmanagerpb.RegisterSecretManagerServiceServer(gsrv, fakeSrv)
	t.Cleanup(gsrv.Stop)
	go func() { _ = gsrv.Serve(l) }()

	tests := []struct {
		name           string
		secretName     string
		minLogLevel    slog.Level
		retries        int
		wantRPCCode    grpccodes.Code
		wantSpanStatus codes.Code
		wantErrorType  any
		wantAttempts   int
		wantWarnLogs   int
		wantDebugLogs  int
	}{
		{
			name:           "success",
			secretName:     "projects/test-project/secrets/ok",
			minLogLevel:    slog.LevelWarn,
			retries:        0,
			wantRPCCode:    grpccodes.OK,
			wantSpanStatus: codes.Ok,
			wantErrorType:  nil,
			wantAttempts:   1,
			wantWarnLogs:   0,
			wantDebugLogs:  0,
		},
		{
			name:           "retry_failure_warn_level",
			secretName:     "projects/test-project/secrets/missing",
			minLogLevel:    slog.LevelWarn,
			retries:        2,
			wantRPCCode:    grpccodes.NotFound,
			wantSpanStatus: codes.Error,
			wantErrorType:  "NOT_FOUND",
			wantAttempts:   3,
			wantWarnLogs:   1,
			wantDebugLogs:  0,
		},
		{
			name:           "retry_failure_debug_level",
			secretName:     "projects/test-project/secrets/missing",
			minLogLevel:    slog.LevelDebug,
			retries:        2,
			wantRPCCode:    grpccodes.NotFound,
			wantSpanStatus: codes.Error,
			wantErrorType:  "NOT_FOUND",
			wantAttempts:   3,
			wantWarnLogs:   1,
			wantDebugLogs:  3,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			env := newTestEnv(t, l.Addr().String(), fakeSrv, tc.minLogLevel)

			err := env.callGetSecret(tc.secretName, tc.retries)
			if got := status.Code(err); got != tc.wantRPCCode {
				t.Fatalf("GetSecret status=%v, want %v (err=%v)", got, tc.wantRPCCode, err)
			}

			t3, t4s := env.assertSpans(t, tc.wantAttempts, tc.wantSpanStatus, tc.wantErrorType)
			env.assertLogs(t, t3, t4s, tc.wantWarnLogs, tc.wantDebugLogs, tc.wantErrorType, tc.retries)
			env.assertDurationMetric(t)
		})
	}
}

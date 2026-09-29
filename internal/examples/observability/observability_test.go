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
	"testing"

	gax "github.com/googleapis/gax-go/v2"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/sdk/metric/metricdata"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
	"go.opentelemetry.io/otel/trace"
	grpccodes "google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

type signalTestCase struct {
	name           string
	secretName     string
	minLogLevel    slog.Level
	retries        int
	wantRPCCode    grpccodes.Code
	wantSpanStatus codes.Code
	wantErrorType  any
	wantAttempts   int
	wantWarnLog    bool
	wantDebugLogs  int
}

func TestObservabilitySignals(t *testing.T) {
	t.Setenv("GOOGLE_SDK_GO_METRICS", "true")
	t.Setenv("GOOGLE_SDK_GO_TRACING", "true")
	t.Setenv("GOOGLE_SDK_GO_LOGGING", "true")
	gax.TestOnlyResetIsFeatureEnabled()
	t.Cleanup(gax.TestOnlyResetIsFeatureEnabled)

	fakeSrv := startFakeServer(t)

	tests := []signalTestCase{
		{
			name:           "success",
			secretName:     "projects/test-project/secrets/ok",
			minLogLevel:    slog.LevelWarn,
			retries:        0,
			wantRPCCode:    grpccodes.OK,
			wantSpanStatus: codes.Ok,
			wantErrorType:  nil,
			wantAttempts:   1,
			wantWarnLog:    false,
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
			wantWarnLog:    true,
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
			wantWarnLog:    true,
			wantDebugLogs:  3,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			env := newTestEnv(t, fakeSrv, tc.minLogLevel)

			err := env.callGetSecret(tc.secretName, tc.retries)
			if got := status.Code(err); got != tc.wantRPCCode {
				t.Fatalf("GetSecret status=%v, want %v (err=%v)", got, tc.wantRPCCode, err)
			}

			t3, t4s := env.assertSpans(t, tc)
			env.assertLogs(t, tc, t3, t4s)
			env.assertDurationMetric(t, tc)
		})
	}
}

func (e *testEnv) assertSpans(t *testing.T, tc signalTestCase) (t3 *tracetest.SpanStub, t4s []*tracetest.SpanStub) {
	t.Helper()
	var t3s []*tracetest.SpanStub
	for _, s := range e.traceExp.GetSpans() {
		switch s.Name {
		case "SecretManager.GetSecret":
			t3s = append(t3s, &s)
		case "google.cloud.secretmanager.v1.SecretManagerService/GetSecret":
			t4s = append(t4s, &s)
		}
	}
	if len(t3s) != 1 || len(t4s) != tc.wantAttempts {
		t.Fatalf("got len(t3s)=%d, len(t4s)=%d; want 1 t3 and %d t4 spans", len(t3s), len(t4s), tc.wantAttempts)
	}
	t3 = t3s[0]
	if t3.SpanKind != trace.SpanKindClient || t3.Status.Code != tc.wantSpanStatus {
		t.Errorf("t3 kind=%v status=%v, want Client/%v", t3.SpanKind, t3.Status.Code, tc.wantSpanStatus)
	}
	if gotErrType := spanAttrs(t3.Attributes)["error.type"]; gotErrType != tc.wantErrorType {
		t.Errorf("t3 error.type=%v, want %v", gotErrType, tc.wantErrorType)
	}

	traceparents := e.fakeSrv.takeTraceparents()
	if len(traceparents) != tc.wantAttempts {
		t.Fatalf("got %d incoming traceparents, want %d", len(traceparents), tc.wantAttempts)
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

func (e *testEnv) assertLogs(t *testing.T, tc signalTestCase, t3 *tracetest.SpanStub, t4s []*tracetest.SpanStub) {
	t.Helper()
	var l3WarnLogs, l4DebugLogs []capturedLog
	for _, entry := range e.logSink.Entries() {
		switch {
		case entry.Level == slog.LevelWarn:
			l3WarnLogs = append(l3WarnLogs, entry)
		case entry.Level == slog.LevelDebug && entry.Attrs["error.type"] != nil:
			l4DebugLogs = append(l4DebugLogs, entry)
		case entry.Level > slog.LevelDebug:
			t.Errorf("unexpected log entry: %+v", entry)
		}
	}
	wantWarn := 0
	if tc.wantWarnLog {
		wantWarn = 1
	}
	if len(l3WarnLogs) != wantWarn || len(l4DebugLogs) != tc.wantDebugLogs {
		t.Fatalf("got %d WARN and %d DEBUG error logs, want %d and %d", len(l3WarnLogs), len(l4DebugLogs), wantWarn, tc.wantDebugLogs)
	}

	for _, l3 := range l3WarnLogs {
		if l3.Message != "gcp.client.request" || l3.Attrs["error.type"] != tc.wantErrorType || l3.Attrs["resend_count"] != int64(tc.retries) {
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

func (e *testEnv) assertDurationMetric(t *testing.T, tc signalTestCase) {
	t.Helper()
	var rm metricdata.ResourceMetrics
	if err := e.metricReader.Collect(context.Background(), &rm); err != nil {
		t.Fatalf("metricReader.Collect: %v", err)
	}
	for _, sm := range rm.ScopeMetrics {
		for _, m := range sm.Metrics {
			if m.Name != "gcp.client.request.duration" {
				continue
			}
			hist, ok := m.Data.(metricdata.Histogram[float64])
			if !ok || len(hist.DataPoints) != 1 || hist.DataPoints[0].Count != 1 {
				t.Fatalf("unexpected duration metric data: %+v", m.Data)
			}
			if gotErrType := spanAttrs(hist.DataPoints[0].Attributes.ToSlice())["error.type"]; gotErrType != tc.wantErrorType {
				t.Errorf("duration metric error.type=%v, want %v", gotErrType, tc.wantErrorType)
			}
			return
		}
	}
	t.Fatal("expected gcp.client.request.duration metric")
}

func spanAttrs(attrs []attribute.KeyValue) map[string]any {
	m := make(map[string]any, len(attrs))
	for _, kv := range attrs {
		m[string(kv.Key)] = kv.Value.AsInterface()
	}
	return m
}

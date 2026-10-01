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
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"testing"
	"time"

	secretmanager "cloud.google.com/go/secretmanager/apiv1"
	secretmanagerpb "cloud.google.com/go/secretmanager/apiv1/secretmanagerpb"
	gax "github.com/googleapis/gax-go/v2"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/exporters/otlp/otlpmetric/otlpmetricgrpc"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracegrpc"
	"go.opentelemetry.io/otel/propagation"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/trace"
	"golang.org/x/oauth2"
	"golang.org/x/oauth2/google"
	"google.golang.org/api/option"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials/oauth"
	"google.golang.org/grpc/status"
)

type cloudTraceResponse struct {
	Spans []struct {
		SpanID       string            `json:"spanId"`
		ParentSpanID string            `json:"parentSpanId"`
		Name         string            `json:"name"`
		Labels       map[string]string `json:"labels"`
	} `json:"spans"`
}

func TestIntegration_Signals(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}
	projectID := os.Getenv("GOOGLE_CLOUD_PROJECT")
	if projectID == "" {
		projectID = os.Getenv("GCLOUD_TESTS_GOLANG_PROJECT_ID")
	}
	if projectID == "" {
		t.Skip("GOOGLE_CLOUD_PROJECT and GCLOUD_TESTS_GOLANG_PROJECT_ID not set")
	}

	ctx := context.Background()
	creds, err := google.FindDefaultCredentials(ctx, "https://www.googleapis.com/auth/cloud-platform")
	if err != nil {
		t.Skipf("ADC unavailable: %v", err)
	}

	t.Setenv("GOOGLE_SDK_GO_TRACING", "true")
	t.Setenv("GOOGLE_SDK_GO_METRICS", "true")
	t.Setenv("GOOGLE_SDK_GO_LOGGING", "true")
	gax.TestOnlyResetIsFeatureEnabled()
	t.Cleanup(gax.TestOnlyResetIsFeatureEnabled)
	saveGlobalOtelState(t)

	tp, err := setupTracing(ctx, projectID, creds)
	if err != nil {
		t.Fatalf("setupTracing: %v", err)
	}
	t.Cleanup(func() {
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = tp.Shutdown(shutdownCtx)
	})

	mp, err := setupMetrics(ctx, projectID, creds)
	if err != nil {
		t.Fatalf("setupMetrics: %v", err)
	}
	t.Cleanup(func() {
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = mp.Shutdown(shutdownCtx)
	})

	var logBuf bytes.Buffer
	logger := setupLogging(&logBuf, slog.LevelDebug)

	client, err := secretmanager.NewClient(ctx, option.WithLogger(logger))
	if err != nil {
		t.Fatal(err)
	}
	defer client.Close()

	appCtx, appSpan := otel.Tracer("integration").Start(ctx, "integration-showcase-test", trace.WithSpanKind(trace.SpanKindInternal))
	traceID := appSpan.SpanContext().TraceID().String()

	retries := 0
	retryOpt := gax.WithRetry(func() gax.Retryer {
		return gax.OnErrorFunc(gax.Backoff{Initial: 50 * time.Millisecond, Max: 100 * time.Millisecond}, func(error) bool {
			retries++
			return retries <= 2
		})
	})

	_, err = client.GetSecret(appCtx, &secretmanagerpb.GetSecretRequest{
		Name: fmt.Sprintf("projects/%s/secrets/nonexistent-signals-secret", projectID),
	}, retryOpt)
	appSpan.End()
	if status.Code(err) != codes.NotFound {
		t.Fatalf("GetSecret err = %v, want status %v", err, codes.NotFound)
	}

	t.Run("Logging", func(t *testing.T) {
		verifyLogging(t, logBuf.Bytes())
	})

	t.Run("Metrics", func(t *testing.T) {
		flushCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
		defer cancel()
		if err := mp.ForceFlush(flushCtx); err != nil {
			t.Fatalf("mp.ForceFlush: %v", err)
		}
	})

	t.Run("Tracing", func(t *testing.T) {
		flushCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
		defer cancel()
		if err := tp.ForceFlush(flushCtx); err != nil {
			t.Fatalf("tp.ForceFlush: %v", err)
		}
		httpClient := oauth2.NewClient(ctx, creds.TokenSource)
		httpClient.Timeout = 10 * time.Second
		verifyCloudTrace(ctx, t, httpClient, projectID, traceID)
	})
}

func setupTracing(ctx context.Context, projectID string, creds *google.Credentials) (*sdktrace.TracerProvider, error) {
	res, err := resource.New(ctx, resource.WithAttributes(
		attribute.String("gcp.project_id", projectID),
		attribute.String("service.name", "observability-integration-test"),
	))
	if err != nil {
		return nil, err
	}
	perRPCCreds := oauth.TokenSource{TokenSource: creds.TokenSource}
	traceExp, err := otlptracegrpc.New(ctx,
		otlptracegrpc.WithEndpoint("telemetry.googleapis.com:443"),
		otlptracegrpc.WithDialOption(grpc.WithPerRPCCredentials(perRPCCreds)),
	)
	if err != nil {
		return nil, err
	}
	tp := sdktrace.NewTracerProvider(sdktrace.WithResource(res), sdktrace.WithBatcher(traceExp))
	otel.SetTracerProvider(tp)
	otel.SetTextMapPropagator(propagation.TraceContext{})
	return tp, nil
}

func setupMetrics(ctx context.Context, projectID string, creds *google.Credentials) (*sdkmetric.MeterProvider, error) {
	res, err := resource.New(ctx, resource.WithAttributes(
		attribute.String("gcp.project_id", projectID),
		attribute.String("cloud.region", "us-central1"),
		attribute.String("service.name", "observability-integration-test"),
		attribute.String("service.instance.id", "integration-test-instance"),
	))
	if err != nil {
		return nil, err
	}
	perRPCCreds := oauth.TokenSource{TokenSource: creds.TokenSource}
	metricExp, err := otlpmetricgrpc.New(ctx,
		otlpmetricgrpc.WithEndpoint("telemetry.googleapis.com:443"),
		otlpmetricgrpc.WithDialOption(grpc.WithPerRPCCredentials(perRPCCreds)),
	)
	if err != nil {
		return nil, err
	}
	mp := sdkmetric.NewMeterProvider(
		sdkmetric.WithResource(res),
		sdkmetric.WithReader(sdkmetric.NewPeriodicReader(metricExp)),
	)
	otel.SetMeterProvider(mp)
	return mp, nil
}

func setupLogging(w io.Writer, level slog.Level) *slog.Logger {
	return slog.New(slog.NewJSONHandler(w, &slog.HandlerOptions{Level: level}))
}

func saveGlobalOtelState(t *testing.T) {
	t.Helper()
	prevTP := otel.GetTracerProvider()
	prevMP := otel.GetMeterProvider()
	prevProp := otel.GetTextMapPropagator()
	t.Cleanup(func() {
		otel.SetTracerProvider(prevTP)
		otel.SetMeterProvider(prevMP)
		otel.SetTextMapPropagator(prevProp)
	})
}

func verifyLogging(t *testing.T, raw []byte) {
	t.Helper()
	trimmed := bytes.TrimSpace(raw)
	if len(trimmed) == 0 {
		t.Fatal("expected structured error logs, got none")
	}
	var warnLogs, debugAttemptLogs []map[string]any
	for _, line := range bytes.Split(trimmed, []byte("\n")) {
		var entry map[string]any
		if err := json.Unmarshal(line, &entry); err != nil {
			t.Fatalf("failed to unmarshal log line %q: %v", line, err)
		}
		switch {
		case entry["level"] == "WARN" && entry["msg"] == "gcp.client.request":
			warnLogs = append(warnLogs, entry)
		case entry["level"] == "DEBUG" && entry["error.type"] != nil:
			debugAttemptLogs = append(debugAttemptLogs, entry)
		}
	}
	if len(warnLogs) != 1 || len(debugAttemptLogs) != 3 {
		t.Fatalf("got %d WARN and %d DEBUG error logs, want 1 and 3 (raw=%s)", len(warnLogs), len(debugAttemptLogs), string(raw))
	}
	warn := warnLogs[0]
	if warn["error.type"] != "NOT_FOUND" || warn["resend_count"] != float64(2) || warn["gcp.client.service"] != "secretmanager" {
		t.Errorf("unexpected WARN log attributes: %v", warn)
	}
	for i, dbg := range debugAttemptLogs {
		if dbg["error.type"] != "NOT_FOUND" || dbg["gcp.client.service"] != "secretmanager" {
			t.Errorf("DEBUG log[%d] unexpected attributes: %v", i, dbg)
		}
	}
}

func verifyCloudTrace(ctx context.Context, t *testing.T, httpClient *http.Client, projectID, traceID string) {
	t.Helper()
	traceURL := fmt.Sprintf("https://cloudtrace.googleapis.com/v1/projects/%s/traces/%s", projectID, traceID)
	pollCtx, cancel := context.WithTimeout(ctx, 45*time.Second)
	defer cancel()

	var (
		foundT3, foundT4 bool
		lastStatus       int
		lastBody         string
	)
	for first := true; pollCtx.Err() == nil; first = false {
		if !first {
			select {
			case <-pollCtx.Done():
				continue
			case <-time.After(3 * time.Second):
			}
		}
		req, err := http.NewRequestWithContext(pollCtx, http.MethodGet, traceURL, nil)
		if err != nil {
			t.Fatal(err)
		}
		resp, err := httpClient.Do(req)
		if err != nil {
			continue
		}
		body, err := io.ReadAll(resp.Body)
		resp.Body.Close()
		if err != nil {
			continue
		}
		lastStatus = resp.StatusCode
		lastBody = string(body)
		if resp.StatusCode != http.StatusOK {
			if resp.StatusCode != http.StatusNotFound {
				t.Logf("Cloud Trace API returned unexpected status %d: %s", resp.StatusCode, lastBody)
			}
			continue
		}

		var traceData cloudTraceResponse
		if json.Unmarshal(body, &traceData) != nil {
			continue
		}
		var appSpanID, t3SpanID, t3ParentID string
		for _, s := range traceData.Spans {
			if s.Name == "integration-showcase-test" {
				appSpanID = s.SpanID
			}
			if s.Name == "SecretManager.GetSecret" && s.Labels["error.type"] == "NOT_FOUND" && s.Labels["gcp.client.service"] == "secretmanager" {
				t3SpanID = s.SpanID
				t3ParentID = s.ParentSpanID
			}
		}
		foundT3 = appSpanID != "" && t3SpanID != "" && t3ParentID == appSpanID
		foundT4 = false
		for _, s := range traceData.Spans {
			if s.Name == "google.cloud.secretmanager.v1.SecretManagerService/GetSecret" &&
				s.ParentSpanID == t3SpanID &&
				s.Labels["gcp.grpc.resend_count"] != "" {
				foundT4 = true
			}
		}
		if foundT3 && foundT4 {
			return
		}
	}
	t.Fatalf("trace %s missing expected spans in Cloud Trace within 45s (foundT3=%v, foundT4=%v, lastStatus=%d, lastBody=%s)", traceID, foundT3, foundT4, lastStatus, lastBody)
}

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
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/credentials/oauth"
)

type cloudTraceResponse struct {
	Spans []struct {
		Name   string            `json:"name"`
		Labels map[string]string `json:"labels"`
	} `json:"spans"`
}

func TestE2E_Signals(t *testing.T) {
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

	t.Setenv("GOOGLE_SDK_GO_METRICS", "true")
	t.Setenv("GOOGLE_SDK_GO_TRACING", "true")
	t.Setenv("GOOGLE_SDK_GO_LOGGING", "true")
	gax.TestOnlyResetIsFeatureEnabled()
	t.Cleanup(gax.TestOnlyResetIsFeatureEnabled)

	tp, mp := setupCloudTelemetry(ctx, t, projectID, oauth.TokenSource{TokenSource: creds.TokenSource})

	var logBuf bytes.Buffer
	logger := slog.New(slog.NewJSONHandler(&logBuf, &slog.HandlerOptions{Level: slog.LevelWarn}))
	client, err := secretmanager.NewClient(ctx, option.WithLogger(logger))
	if err != nil {
		t.Fatal(err)
	}
	defer client.Close()

	appCtx, appSpan := tp.Tracer("e2e").Start(ctx, "e2e-showcase-test", trace.WithSpanKind(trace.SpanKindInternal))
	traceID := appSpan.SpanContext().TraceID().String()

	retries := 0
	retryOpt := gax.WithRetry(func() gax.Retryer {
		return gax.OnErrorFunc(gax.Backoff{Initial: 50 * time.Millisecond, Max: 100 * time.Millisecond}, func(error) bool {
			retries++
			return retries <= 2
		})
	})

	_, _ = client.GetSecret(appCtx, &secretmanagerpb.GetSecretRequest{
		Name: fmt.Sprintf("projects/%s/secrets/nonexistent-signals-secret", projectID),
	}, retryOpt)
	appSpan.End()

	flushCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	if err := tp.Shutdown(flushCtx); err != nil {
		t.Fatalf("tp.Shutdown: %v", err)
	}
	if err := mp.Shutdown(flushCtx); err != nil {
		t.Fatalf("mp.Shutdown: %v", err)
	}

	assertWarnErrorLog(t, logBuf.Bytes())

	httpClient := oauth2.NewClient(ctx, creds.TokenSource)
	httpClient.Timeout = 10 * time.Second
	verifyCloudTrace(ctx, t, httpClient, projectID, traceID)
}

func setupCloudTelemetry(ctx context.Context, t *testing.T, projectID string, perRPCCreds credentials.PerRPCCredentials) (*sdktrace.TracerProvider, *sdkmetric.MeterProvider) {
	t.Helper()
	res, err := resource.New(ctx, resource.WithAttributes(attribute.String("gcp.project_id", projectID)))
	if err != nil {
		t.Fatal(err)
	}

	prevTP := otel.GetTracerProvider()
	prevMP := otel.GetMeterProvider()
	prevProp := otel.GetTextMapPropagator()
	t.Cleanup(func() {
		otel.SetTracerProvider(prevTP)
		otel.SetMeterProvider(prevMP)
		otel.SetTextMapPropagator(prevProp)
	})

	traceExp, err := otlptracegrpc.New(ctx,
		otlptracegrpc.WithEndpoint("telemetry.googleapis.com:443"),
		otlptracegrpc.WithDialOption(grpc.WithPerRPCCredentials(perRPCCreds)),
	)
	if err != nil {
		t.Fatal(err)
	}
	tp := sdktrace.NewTracerProvider(sdktrace.WithResource(res), sdktrace.WithBatcher(traceExp))
	t.Cleanup(func() { _ = tp.Shutdown(context.Background()) })
	otel.SetTracerProvider(tp)
	otel.SetTextMapPropagator(propagation.TraceContext{})

	metricExp, err := otlpmetricgrpc.New(ctx,
		otlpmetricgrpc.WithEndpoint("telemetry.googleapis.com:443"),
		otlpmetricgrpc.WithDialOption(grpc.WithPerRPCCredentials(perRPCCreds)),
	)
	if err != nil {
		t.Fatal(err)
	}
	mp := sdkmetric.NewMeterProvider(sdkmetric.WithResource(res), sdkmetric.WithReader(sdkmetric.NewPeriodicReader(metricExp)))
	t.Cleanup(func() { _ = mp.Shutdown(context.Background()) })
	otel.SetMeterProvider(mp)

	return tp, mp
}

func assertWarnErrorLog(t *testing.T, raw []byte) {
	t.Helper()
	trimmed := bytes.TrimSpace(raw)
	if len(trimmed) == 0 {
		t.Fatal("expected 1 structured error log, got 0")
	}
	lines := bytes.Split(trimmed, []byte("\n"))
	if len(lines) != 1 {
		t.Fatalf("expected 1 structured error log line, got %d: %s", len(lines), string(raw))
	}
	var entry map[string]any
	if err := json.Unmarshal(lines[0], &entry); err != nil {
		t.Fatalf("failed to unmarshal structured error log %q: %v", lines[0], err)
	}
	if entry["level"] != "WARN" || entry["msg"] != "gcp.client.request" || entry["error.type"] != "NOT_FOUND" || entry["resend_count"] != float64(2) {
		t.Errorf("unexpected structured error log: %v", entry)
	}
}

func verifyCloudTrace(ctx context.Context, t *testing.T, httpClient *http.Client, projectID, traceID string) {
	t.Helper()
	traceURL := fmt.Sprintf("https://cloudtrace.googleapis.com/v1/projects/%s/traces/%s", projectID, traceID)
	var foundT3, foundT4 bool

	for deadline := time.Now().Add(45 * time.Second); time.Now().Before(deadline); time.Sleep(3 * time.Second) {
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, traceURL, nil)
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

		var traceData cloudTraceResponse
		if resp.StatusCode != http.StatusOK || json.Unmarshal(body, &traceData) != nil {
			continue
		}
		foundT3, foundT4 = false, false
		for _, s := range traceData.Spans {
			if s.Name == "SecretManager.GetSecret" && s.Labels["error.type"] == "NOT_FOUND" {
				foundT3 = true
			}
			if s.Name == "google.cloud.secretmanager.v1.SecretManagerService/GetSecret" && s.Labels["gcp.grpc.resend_count"] != "" {
				foundT4 = true
			}
		}
		if foundT3 && foundT4 {
			return
		}
	}
	t.Fatalf("trace %s missing expected spans in Cloud Trace within 45s (foundT3=%v, foundT4=%v)", traceID, foundT3, foundT4)
}

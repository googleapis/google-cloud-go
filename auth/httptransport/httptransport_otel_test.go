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

package httptransport

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"
	"github.com/googleapis/gax-go/v2"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"

	"github.com/googleapis/gax-go/v2/callctx"
	oteltrace "go.opentelemetry.io/otel/trace"
)

const (
	keyHTTPRequestMetod   = attribute.Key("http.request.method")
	keyHTTPResponseStatus = attribute.Key("http.response.status_code")
	keyNetProtoVersion    = attribute.Key("network.protocol.version")
	keyServerAddr         = attribute.Key("server.address")
	keyServerPort         = attribute.Key("server.port")
	keyURLFull            = attribute.Key("url.full")
	keyErrorType          = attribute.Key("error.type")

	valHTTPGet   = "GET"
	valHTTP11    = "1.1"
	valLocalhost = "127.0.0.1"
)

func TestNewClient_OpenTelemetry_Enabled(t *testing.T) {
	gax.TestOnlyResetIsFeatureEnabled()
	defer gax.TestOnlyResetIsFeatureEnabled()
	t.Setenv("GOOGLE_SDK_GO_EXPERIMENTAL_TRACING", "true")

	defer http.DefaultTransport.(*http.Transport).CloseIdleConnections()

	exporter := tracetest.NewInMemoryExporter()
	tp := sdktrace.NewTracerProvider(sdktrace.WithSyncer(exporter))
	defer tp.Shutdown(context.Background())

	// Restore the global tracer provider after the test to avoid side effects.
	defer func(prev oteltrace.TracerProvider) { otel.SetTracerProvider(prev) }(otel.GetTracerProvider())
	otel.SetTracerProvider(tp)

	tests := []struct {
		name               string
		opts               *Options
		telemetryCtxValues map[string]string
		statusCode         int
		errorType          string // "timeout", "cancel", "connection"
		wantErr            bool
		wantSpans          int
		wantSpan           sdktrace.ReadOnlySpan
		wantAttrKeys       []attribute.Key
	}{
		{
			name:       "telemetry enabled success",
			opts:       &Options{DisableAuthentication: true},
			statusCode: http.StatusOK,
			wantSpans:  1,
			wantSpan: tracetest.SpanStub{
				Name:     "HTTP GET",
				SpanKind: oteltrace.SpanKindClient,
				Status: sdktrace.Status{
					Code: codes.Unset,
				},
				Attributes: []attribute.KeyValue{
					keyHTTPRequestMetod.String(valHTTPGet),
					keyHTTPResponseStatus.Int(200),
					keyNetProtoVersion.String(valHTTP11),
					keyServerAddr.String(valLocalhost),
					attribute.String("rpc.system.name", "http"),
				},
			}.Snapshot(),
			wantAttrKeys: []attribute.Key{keyServerPort, keyURLFull},
		},
		{
			name:       "telemetry enabled error",
			opts:       &Options{DisableAuthentication: true},
			statusCode: http.StatusInternalServerError,
			wantErr:    false, // The RoundTrip itself doesn't return an error for 500, it returns a response.
			wantSpans:  1,
			wantSpan: tracetest.SpanStub{
				Name:     "HTTP GET",
				SpanKind: oteltrace.SpanKindClient,
				Status: sdktrace.Status{
					Code:        codes.Error,
					Description: "",
				},
				Attributes: []attribute.KeyValue{
					keyHTTPRequestMetod.String(valHTTPGet),
					keyHTTPResponseStatus.Int(500),
					keyNetProtoVersion.String(valHTTP11),
					keyServerAddr.String(valLocalhost),
					keyErrorType.String("500"),
					attribute.String("status.message", "500 Internal Server Error"),
					attribute.String("rpc.system.name", "http"),
				},
			}.Snapshot(),
			wantAttrKeys: []attribute.Key{keyServerPort, keyURLFull},
		},
		{
			name:      "telemetry enabled client timeout",
			opts:      &Options{DisableAuthentication: true},
			errorType: "timeout",
			wantErr:   true,
			wantSpans: 1,
			wantSpan: tracetest.SpanStub{
				Name:     "HTTP GET",
				SpanKind: oteltrace.SpanKindClient,
				Status: sdktrace.Status{
					Code:        codes.Error,
					Description: "context deadline exceeded",
				},
				Attributes: []attribute.KeyValue{
					keyHTTPRequestMetod.String(valHTTPGet),
					keyErrorType.String("context.deadlineExceededError"),
					attribute.String("rpc.system.name", "http"),
				},
			}.Snapshot(),
			wantAttrKeys: []attribute.Key{keyServerAddr, attribute.Key("status.message"), attribute.Key("exception.type")},
		},
		{
			name:      "telemetry enabled client cancelled",
			opts:      &Options{DisableAuthentication: true},
			errorType: "cancel",
			wantErr:   true,
			wantSpans: 1,
			wantSpan: tracetest.SpanStub{
				Name:     "HTTP GET",
				SpanKind: oteltrace.SpanKindClient,
				Status: sdktrace.Status{
					Code:        codes.Error,
					Description: "context canceled",
				},
				Attributes: []attribute.KeyValue{
					keyHTTPRequestMetod.String(valHTTPGet),
					keyErrorType.String("*errors.errorString"),
					attribute.String("rpc.system.name", "http"),
				},
			}.Snapshot(),
			wantAttrKeys: []attribute.Key{keyServerAddr, attribute.Key("status.message"), attribute.Key("exception.type")},
		},
		{
			name:      "telemetry enabled client connection error",
			opts:      &Options{DisableAuthentication: true},
			errorType: "connection",
			wantErr:   true,
			wantSpans: 1,
			wantSpan: tracetest.SpanStub{
				Name:     "HTTP GET",
				SpanKind: oteltrace.SpanKindClient,
				Status: sdktrace.Status{
					Code: codes.Error,
				},
				Attributes: []attribute.KeyValue{
					keyHTTPRequestMetod.String(valHTTPGet),
					keyErrorType.String("*net.OpError"),
					attribute.String("rpc.system.name", "http"),
				},
			}.Snapshot(),
			wantAttrKeys: []attribute.Key{keyServerAddr, attribute.Key("status.message"), attribute.Key("exception.type")},
		},
		{
			name: "telemetry disabled",
			opts: &Options{
				DisableAuthentication: true,
				DisableTelemetry:      true,
			},
			statusCode: http.StatusOK,
			wantSpans:  0,
		},
		{
			name: "telemetry enabled metadata enrichment",
			opts: &Options{
				DisableAuthentication: true,
				InternalOptions: &InternalOptions{
					TelemetryAttributes: map[string]string{
						"gcp.client.service":  "myservice",
						"gcp.client.version":  "1.0.0",
						"gcp.client.repo":     "googleapis/google-cloud-go",
						"gcp.client.artifact": "c.g/auth/httptransport",
						"gcp.client.language": "go",
						"url.domain":          "myservice.googleapis.com",
						"ignored.key":         "should not be included",
					},
				},
			},
			telemetryCtxValues: map[string]string{"resource_name": "my-resource"},
			statusCode:         http.StatusOK,
			wantSpans:          1,
			wantSpan: tracetest.SpanStub{
				Name:     "HTTP GET",
				SpanKind: oteltrace.SpanKindClient,
				Attributes: []attribute.KeyValue{
					keyHTTPRequestMetod.String(valHTTPGet),
					keyHTTPResponseStatus.Int(200),
					attribute.String("gcp.resource.destination.id", "my-resource"),
					attribute.String("gcp.client.service", "myservice"),
					attribute.String("gcp.client.version", "1.0.0"),
					attribute.String("gcp.client.repo", "googleapis/google-cloud-go"),
					attribute.String("gcp.client.artifact", "c.g/auth/httptransport"),
					attribute.String("gcp.client.language", "go"),
					attribute.String("rpc.system.name", "http"),
					attribute.String("url.domain", "myservice.googleapis.com"),
				},
			}.Snapshot(),
			wantAttrKeys: []attribute.Key{keyServerAddr},
		},
		{
			name: "telemetry enabled url template",
			opts: &Options{
				DisableAuthentication: true,
			},
			telemetryCtxValues: map[string]string{"url_template": "/my/template"},
			statusCode:         http.StatusOK,
			wantSpans:          1,
			wantSpan: tracetest.SpanStub{
				Name:     "GET /my/template",
				SpanKind: oteltrace.SpanKindClient,
				Attributes: []attribute.KeyValue{
					keyHTTPRequestMetod.String(valHTTPGet),
					keyHTTPResponseStatus.Int(200),
					attribute.String("url.template", "/my/template"),
					attribute.String("rpc.system.name", "http"),
				},
			}.Snapshot(),
			wantAttrKeys: []attribute.Key{keyServerAddr},
		},
		{
			name: "telemetry enabled resend count",
			opts: &Options{
				DisableAuthentication: true,
			},
			telemetryCtxValues: map[string]string{"resend_count": "2"},
			statusCode:         http.StatusOK,
			wantSpans:          1,
			wantSpan: tracetest.SpanStub{
				Name:     "HTTP GET",
				SpanKind: oteltrace.SpanKindClient,
				Attributes: []attribute.KeyValue{
					keyHTTPRequestMetod.String(valHTTPGet),
					keyHTTPResponseStatus.Int(200),
					attribute.Int("http.request.resend_count", 2),
					attribute.String("rpc.system.name", "http"),
				},
			}.Snapshot(),
			wantAttrKeys: []attribute.Key{keyServerAddr},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			exporter.Reset()

			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if tt.errorType == "timeout" {
					time.Sleep(100 * time.Millisecond)
				}
				if tt.statusCode != 0 {
					w.WriteHeader(tt.statusCode)
				}
			}))
			defer server.Close()

			if tt.errorType == "connection" {
				server.Close()
			}

			tt.opts.Endpoint = server.URL
			client, err := NewClient(tt.opts)
			if err != nil {
				t.Fatalf("NewClient() = %v, want nil", err)
			}

			ctx := context.Background()
			var cancel context.CancelFunc
			if tt.errorType == "timeout" {
				ctx, cancel = context.WithTimeout(ctx, 10*time.Millisecond)
				defer cancel()
			} else if tt.errorType == "cancel" {
				ctx, cancel = context.WithCancel(ctx)
				cancel()
			}

			for k, v := range tt.telemetryCtxValues {
				ctx = callctx.WithTelemetryContext(ctx, k, v)
			}

			req, err := http.NewRequestWithContext(ctx, "GET", server.URL, nil)
			if err != nil {
				t.Fatalf("http.NewRequest() = %v, want nil", err)
			}

			resp, err := client.Do(req)
			if (err != nil) != tt.wantErr {
				t.Errorf("client.Do() error = %v, wantErr %v", err, tt.wantErr)
			}
			if resp != nil && resp.Body != nil {
				resp.Body.Close()
			}

			spans := exporter.GetSpans()
			if len(spans) != tt.wantSpans {
				t.Fatalf("len(spans) = %d, want %d", len(spans), tt.wantSpans)
			}

			if tt.wantSpans > 0 {
				span := exporter.GetSpans()[0]
				if diff := cmp.Diff(tt.wantSpan.Name(), span.Name); diff != "" {
					t.Errorf("span.Name mismatch (-want +got):\n%s", diff)
				}
				if diff := cmp.Diff(tt.wantSpan.SpanKind(), span.SpanKind); diff != "" {
					t.Errorf("span.SpanKind mismatch (-want +got):\n%s", diff)
				}
				if diff := cmp.Diff(tt.wantSpan.Status(), span.Status, cmpopts.IgnoreFields(sdktrace.Status{}, "Description")); diff != "" {
					t.Errorf("span.Status mismatch (-want +got):\n%s", diff)
				}

				// Note: Real-world spans in Cloud Trace will contain additional attributes
				// that are not present in this unit test.
				//
				// 1. Resource Attributes:
				//    - "g.co/r/generic_node/location" (e.g., "global")
				//    - "g.co/r/generic_node/namespace"
				//    - "g.co/r/generic_node/node_id"
				//    - "service.name" (e.g., "my-application")
				//    - "telemetry.sdk.language" (e.g., "go")
				//    - "telemetry.sdk.name" (e.g., "opentelemetry")
				//    - "telemetry.sdk.version" (e.g., "1.20.0")
				//    These are defined by the TracerProvider's Resource configuration. This test uses
				//    a basic TracerProvider, so these attributes contain default values rather than production values.
				//
				// 2. Instrumentation Scope:
				//    - "otel.scope.name" (e.g., "go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp")
				//    - "otel.scope.version" (e.g., "0.49.0")
				//    These identify the instrumentation library itself and are part of the
				//    OpenTelemetry data model, separate from Span attributes.
				//
				// 3. Exporter Attributes:
				//    - "g.co/agent" (e.g., "opentelemetry-go 1.37.0; google-cloud-trace-exporter 1.20.0")
				//    These are injected by specific exporters (like the Google Cloud Trace exporter)
				//    and are not present when using the InMemoryExporter.
				//
				// This test focuses on verifying the "http.*", "net.*" and "url.*" attributes generated
				// by the otelhttp instrumentation library.

				gotAttrs := map[attribute.Key]attribute.Value{}
				for _, attr := range span.Attributes {
					gotAttrs[attr.Key] = attr.Value
				}
				for _, wantAttr := range tt.wantSpan.Attributes() {
					if gotVal, ok := gotAttrs[wantAttr.Key]; !ok {
						t.Errorf("missing attribute: %s", wantAttr.Key)
					} else {
						// Use simple value comparison for non-dynamic fields
						if diff := cmp.Diff(wantAttr.Value, gotVal, cmp.AllowUnexported(attribute.Value{})); diff != "" {
							t.Errorf("attribute %s mismatch (-want +got):\n%s", wantAttr.Key, diff)
						}
					}
				}
				for _, wantKey := range tt.wantAttrKeys {
					if _, ok := gotAttrs[wantKey]; !ok {
						t.Errorf("missing attribute key: %s", wantKey)
					}
				}
				if _, ok := gotAttrs[attribute.Key("ignored.key")]; ok {
					t.Errorf("found unexpected attribute key: ignored.key")
				}
			}
		})
	}
}

func TestNewClient_OpenTelemetry_Disabled(t *testing.T) {
	gax.TestOnlyResetIsFeatureEnabled()
	defer gax.TestOnlyResetIsFeatureEnabled()
	t.Setenv("GOOGLE_SDK_GO_EXPERIMENTAL_TRACING", "false")

	defer http.DefaultTransport.(*http.Transport).CloseIdleConnections()

	exporter := tracetest.NewInMemoryExporter()
	tp := sdktrace.NewTracerProvider(sdktrace.WithSyncer(exporter))
	defer tp.Shutdown(context.Background())

	// Restore the global tracer provider after the test to avoid side effects.
	defer func(prev oteltrace.TracerProvider) { otel.SetTracerProvider(prev) }(otel.GetTracerProvider())
	otel.SetTracerProvider(tp)

	tests := []struct {
		name               string
		opts               *Options
		telemetryCtxValues map[string]string
		statusCode         int
		errorType          string // "timeout", "cancel", "connection"
		wantErr            bool
		wantSpans          int
		wantSpan           sdktrace.ReadOnlySpan
		wantAttrKeys       []attribute.Key
	}{
		{
			name:       "telemetry enabled success (but gated off)",
			opts:       &Options{DisableAuthentication: true},
			statusCode: http.StatusOK,
			wantSpans:  1,
			wantSpan: tracetest.SpanStub{
				Name:     "HTTP GET",
				SpanKind: oteltrace.SpanKindClient,
				Status: sdktrace.Status{
					Code: codes.Unset,
				},
				Attributes: []attribute.KeyValue{
					keyHTTPRequestMetod.String(valHTTPGet),
					keyHTTPResponseStatus.Int(200),
					keyNetProtoVersion.String(valHTTP11),
					keyServerAddr.String(valLocalhost),
					// NO rpc.system
				},
			}.Snapshot(),
			wantAttrKeys: []attribute.Key{keyServerPort, keyURLFull}, // NO url.domain
		},
		{
			name:       "telemetry enabled error (but gated off)",
			opts:       &Options{DisableAuthentication: true},
			statusCode: http.StatusInternalServerError,
			wantErr:    false,
			wantSpans:  1,
			wantSpan: tracetest.SpanStub{
				Name:     "HTTP GET",
				SpanKind: oteltrace.SpanKindClient,
				Status: sdktrace.Status{
					Code:        codes.Error,
					Description: "",
				},
				Attributes: []attribute.KeyValue{
					keyHTTPRequestMetod.String(valHTTPGet),
					keyHTTPResponseStatus.Int(500),
					keyNetProtoVersion.String(valHTTP11),
					keyServerAddr.String(valLocalhost),
					// NO rpc.system, status.message, error.type
				},
			}.Snapshot(),
			wantAttrKeys: []attribute.Key{keyServerPort, keyURLFull}, // NO url.domain
		},
		{
			name:      "telemetry enabled client timeout (but gated off)",
			opts:      &Options{DisableAuthentication: true},
			errorType: "timeout",
			wantErr:   true,
			wantSpans: 1,
			wantSpan: tracetest.SpanStub{
				Name:     "HTTP GET",
				SpanKind: oteltrace.SpanKindClient,
				Status: sdktrace.Status{
					Code: codes.Error,
				},
				Attributes: []attribute.KeyValue{
					keyHTTPRequestMetod.String(valHTTPGet),
					// NO rpc.system, exception.type, error.type
				},
			}.Snapshot(),
			wantAttrKeys: []attribute.Key{keyServerAddr},
		},
		{
			name:      "telemetry enabled client cancelled (but gated off)",
			opts:      &Options{DisableAuthentication: true},
			errorType: "cancel",
			wantErr:   true,
			wantSpans: 1,
			wantSpan: tracetest.SpanStub{
				Name:     "HTTP GET",
				SpanKind: oteltrace.SpanKindClient,
				Status: sdktrace.Status{
					Code: codes.Error,
				},
				Attributes: []attribute.KeyValue{
					keyHTTPRequestMetod.String(valHTTPGet),
					// NO rpc.system, exception.type, error.type
				},
			}.Snapshot(),
			wantAttrKeys: []attribute.Key{keyServerAddr},
		},
		{
			name:      "telemetry enabled client connection error (but gated off)",
			opts:      &Options{DisableAuthentication: true},
			errorType: "connection",
			wantErr:   true,
			wantSpans: 1,
			wantSpan: tracetest.SpanStub{
				Name:     "HTTP GET",
				SpanKind: oteltrace.SpanKindClient,
				Status: sdktrace.Status{
					Code: codes.Error,
				},
				Attributes: []attribute.KeyValue{
					keyHTTPRequestMetod.String(valHTTPGet),
					// NO rpc.system, exception.type, error.type
				},
			}.Snapshot(),
			wantAttrKeys: []attribute.Key{keyServerAddr},
		},
		{
			name: "telemetry disabled",
			opts: &Options{
				DisableAuthentication: true,
				DisableTelemetry:      true,
			},
			statusCode: http.StatusOK,
			wantSpans:  0,
		},
		{
			name: "telemetry enabled metadata enrichment (but gated off)",
			opts: &Options{
				DisableAuthentication: true,
				InternalOptions: &InternalOptions{
					TelemetryAttributes: map[string]string{
						"gcp.client.version": "1.0.0",
						"ignored.key":        "should not be included",
					},
				},
			},
			telemetryCtxValues: map[string]string{"resource_name": "my-resource"},
			statusCode:         http.StatusOK,
			wantSpans:          1,
			wantSpan: tracetest.SpanStub{
				Name:     "HTTP GET",
				SpanKind: oteltrace.SpanKindClient,
				Attributes: []attribute.KeyValue{
					keyHTTPRequestMetod.String(valHTTPGet),
					keyHTTPResponseStatus.Int(200),
					// NO gcp.* attributes, NO rpc.system
				},
			}.Snapshot(),
			wantAttrKeys: []attribute.Key{keyServerAddr}, // NO url.domain
		},
		{
			name: "telemetry enabled resend count (but gated off)",
			opts: &Options{
				DisableAuthentication: true,
			},
			telemetryCtxValues: map[string]string{"resend_count": "2"},
			statusCode:         http.StatusOK,
			wantSpans:          1,
			wantSpan: tracetest.SpanStub{
				Name:     "HTTP GET",
				SpanKind: oteltrace.SpanKindClient,
				Attributes: []attribute.KeyValue{
					keyHTTPRequestMetod.String(valHTTPGet),
					keyHTTPResponseStatus.Int(200),
					// NO http.request.resend_count
				},
			}.Snapshot(),
			wantAttrKeys: []attribute.Key{keyServerAddr}, // NO url.domain
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			exporter.Reset()

			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if tt.errorType == "timeout" {
					time.Sleep(100 * time.Millisecond)
				}
				if tt.statusCode != 0 {
					w.WriteHeader(tt.statusCode)
				}
			}))
			defer server.Close()

			if tt.errorType == "connection" {
				server.Close()
			}

			tt.opts.Endpoint = server.URL
			client, err := NewClient(tt.opts)
			if err != nil {
				t.Fatalf("NewClient() = %v, want nil", err)
			}

			ctx := context.Background()
			var cancel context.CancelFunc
			if tt.errorType == "timeout" {
				ctx, cancel = context.WithTimeout(ctx, 10*time.Millisecond)
				defer cancel()
			} else if tt.errorType == "cancel" {
				ctx, cancel = context.WithCancel(ctx)
				cancel()
			}

			for k, v := range tt.telemetryCtxValues {
				ctx = callctx.WithTelemetryContext(ctx, k, v)
			}

			req, err := http.NewRequestWithContext(ctx, "GET", server.URL, nil)
			if err != nil {
				t.Fatalf("http.NewRequest() = %v, want nil", err)
			}

			resp, err := client.Do(req)
			if (err != nil) != tt.wantErr {
				t.Errorf("client.Do() error = %v, wantErr %v", err, tt.wantErr)
			}
			if resp != nil && resp.Body != nil {
				resp.Body.Close()
			}

			spans := exporter.GetSpans()
			if len(spans) != tt.wantSpans {
				t.Fatalf("len(spans) = %d, want %d", len(spans), tt.wantSpans)
			}

			if tt.wantSpans > 0 {
				span := exporter.GetSpans()[0]
				if diff := cmp.Diff(tt.wantSpan.Name(), span.Name); diff != "" {
					t.Errorf("span.Name mismatch (-want +got):\n%s", diff)
				}
				if diff := cmp.Diff(tt.wantSpan.SpanKind(), span.SpanKind); diff != "" {
					t.Errorf("span.SpanKind mismatch (-want +got):\n%s", diff)
				}
				if diff := cmp.Diff(tt.wantSpan.Status(), span.Status, cmpopts.IgnoreFields(sdktrace.Status{}, "Description")); diff != "" {
					t.Errorf("span.Status mismatch (-want +got):\n%s", diff)
				}

				gotAttrs := map[attribute.Key]attribute.Value{}
				for _, attr := range span.Attributes {
					gotAttrs[attr.Key] = attr.Value
				}
				for _, wantAttr := range tt.wantSpan.Attributes() {
					if gotVal, ok := gotAttrs[wantAttr.Key]; !ok {
						t.Errorf("missing attribute: %s", wantAttr.Key)
					} else {
						// Use simple value comparison for non-dynamic fields
						if diff := cmp.Diff(wantAttr.Value, gotVal, cmp.AllowUnexported(attribute.Value{})); diff != "" {
							t.Errorf("attribute %s mismatch (-want +got):\n%s", wantAttr.Key, diff)
						}
					}
				}
				for _, wantKey := range tt.wantAttrKeys {
					if _, ok := gotAttrs[wantKey]; !ok {
						t.Errorf("missing attribute key: %s", wantKey)
					}
				}
				if _, ok := gotAttrs[attribute.Key("ignored.key")]; ok {
					t.Errorf("found unexpected attribute key: ignored.key")
				}
			}
		})
	}
}

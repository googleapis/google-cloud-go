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
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/google/go-cmp/cmp"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
	oteltrace "go.opentelemetry.io/otel/trace"

	sdktrace "go.opentelemetry.io/otel/sdk/trace"
)

func TestNewClient_OpenTelemetry(t *testing.T) {
	exporter := tracetest.NewInMemoryExporter()
	tp := sdktrace.NewTracerProvider(sdktrace.WithSyncer(exporter))
	otel.SetTracerProvider(tp)

	tests := []struct {
		name       string
		opts       *Options
		statusCode int
		wantSpans  int
		wantSpan   sdktrace.ReadOnlySpan
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
					// In Cloud Trace, this often forms part of the Span Name.
					attribute.Key("http.request.method").String("GET"),
					// In Cloud Trace, this status code maps to the visual "Status" field
					// (e.g., a green checkmark for 200, or an error icon for 5xx).
					attribute.Key("http.response.status_code").Int(200),
					attribute.Key("network.protocol.version").String("1.1"),
					// "server.address", "server.port", and "url.full" are displayed as
					// standard attribute keys in the "Attributes" tab.
					attribute.Key("server.address").String("127.0.0.1"),
					attribute.Key("server.port").Int(0),  // Dynamic
					attribute.Key("url.full").String(""), // Dynamic
				},
			}.Snapshot(),
		},
		{
			name:       "telemetry enabled error",
			opts:       &Options{DisableAuthentication: true},
			statusCode: http.StatusInternalServerError,
			wantSpans:  1,
			wantSpan: tracetest.SpanStub{
				Name:     "HTTP GET",
				SpanKind: oteltrace.SpanKindClient,
				Status: sdktrace.Status{
					Code:        codes.Error,
					Description: "",
				},
				Attributes: []attribute.KeyValue{
					attribute.Key("http.request.method").String("GET"),
					// In Cloud Trace, 5xx status codes are displayed as errors in the "Status" field.
					attribute.Key("http.response.status_code").Int(500),
					attribute.Key("network.protocol.version").String("1.1"),
					attribute.Key("server.address").String("127.0.0.1"),
					attribute.Key("server.port").Int(0),       // Dynamic
					attribute.Key("url.full").String(""),      // Dynamic
					attribute.Key("error.type").String("500"), // otelhttp adds this on error
				},
			}.Snapshot(),
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
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			exporter.Reset()

			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(tt.statusCode)
			}))
			defer server.Close()

			tt.opts.Endpoint = server.URL
			client, err := NewClient(tt.opts)
			if err != nil {
				t.Fatalf("NewClient() = %v, want nil", err)
			}

			req, err := http.NewRequest("GET", server.URL, nil)
			if err != nil {
				t.Fatalf("http.NewRequest() = %v, want nil", err)
			}

			resp, err := client.Do(req)
			if err != nil {
				t.Fatalf("client.Do() = %v, want nil", err)
			}
			resp.Body.Close()

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
				if diff := cmp.Diff(tt.wantSpan.Status(), span.Status); diff != "" {
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
						// Ignore value comparison for dynamic fields
						if wantAttr.Key == "server.port" || wantAttr.Key == "url.full" {
							continue
						}
						// Use simple value comparison for non-dynamic fields
						if diff := cmp.Diff(wantAttr.Value, gotVal, cmp.AllowUnexported(attribute.Value{})); diff != "" {
							t.Errorf("attribute %s mismatch (-want +got):\n%s", wantAttr.Key, diff)
						}
					}
				}
			}
		})
	}
}

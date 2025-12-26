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

package grpctransport

import (
	"context"
	"net"
	"net/http"
	"testing"

	"github.com/google/go-cmp/cmp"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
	oteltrace "go.opentelemetry.io/otel/trace"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/status"

	echo "cloud.google.com/go/auth/grpctransport/testdata"

	grpccodes "google.golang.org/grpc/codes"
)

const (
	keyRPCMethod     = attribute.Key("rpc.method")
	keyRPCService    = attribute.Key("rpc.service")
	keyRPCSystem     = attribute.Key("rpc.system")
	keyRPCStatusCode = attribute.Key("rpc.grpc.status_code")
	keyServerAddr    = attribute.Key("server.address")
	keyServerPort    = attribute.Key("server.port")

	valRPCSystemGRPC = "grpc"
	valLocalhost     = "127.0.0.1"
)

func TestDial_OpenTelemetry(t *testing.T) {
	// Ensure any lingering HTTP/2 connections are closed to avoid goroutine leaks.
	defer http.DefaultTransport.(*http.Transport).CloseIdleConnections()

	exporter := tracetest.NewInMemoryExporter()
	tp := sdktrace.NewTracerProvider(sdktrace.WithSyncer(exporter))
	defer tp.Shutdown(context.Background())

	// Restore the global tracer provider after the test to avoid side effects.
	defer func(prev oteltrace.TracerProvider) { otel.SetTracerProvider(prev) }(otel.GetTracerProvider())
	otel.SetTracerProvider(tp)

	successfulEchoer := &fakeEchoService{
		Fn: func(ctx context.Context, req *echo.EchoRequest) (*echo.EchoReply, error) {
			return &echo.EchoReply{Message: req.Message}, nil
		},
	}
	errorEchoer := &fakeEchoService{
		Fn: func(ctx context.Context, req *echo.EchoRequest) (*echo.EchoReply, error) {
			return nil, status.Error(grpccodes.Internal, "test error")
		},
	}

	tests := []struct {
		name         string
		echoer       echo.EchoerServer
		opts         *Options
		wantErr      bool
		wantSpans    int
		wantSpan     sdktrace.ReadOnlySpan
		wantAttrKeys []attribute.Key
	}{
		{
			name:      "telemetry enabled success",
			echoer:    successfulEchoer,
			opts:      &Options{DisableAuthentication: true},
			wantSpans: 1,
			wantSpan: tracetest.SpanStub{
				Name:     "echo.Echoer/Echo",
				SpanKind: oteltrace.SpanKindClient,
				Status: sdktrace.Status{
					Code: codes.Unset,
				},
				Attributes: []attribute.KeyValue{
					// Note on Events (Logs):
					// The otelgrpc instrumentation also records "message" events (Sent/Received)
					// containing message sizes (compressed/uncompressed). These appear in the
					// "Logs" or "Events" tab in Cloud Trace. This test does not explicitly verify
					// them, but they are present in the generated span.

					// In Cloud Trace, this status code maps to the visual "Status" field
					// (e.g., a green checkmark for 0/OK, or an error icon for other codes).
					keyRPCStatusCode.Int64(0),
					// In Cloud Trace, "rpc.service" and "rpc.method" are combined to form
					// the Span Name (e.g., "echo.Echoer/Echo").
					keyRPCMethod.String("Echo"),
					keyRPCService.String("echo.Echoer"),
					// "rpc.system" is displayed as a standard attribute key in the "Attributes" tab.
					keyRPCSystem.String(valRPCSystemGRPC),
					// "server.address" and "server.port" are displayed as standard attribute keys.
					keyServerAddr.String(valLocalhost),
				},
			}.Snapshot(),
			wantAttrKeys: []attribute.Key{keyServerPort},
		},
		{
			name:      "telemetry enabled error",
			echoer:    errorEchoer,
			opts:      &Options{DisableAuthentication: true},
			wantErr:   true,
			wantSpans: 1,
			wantSpan: tracetest.SpanStub{
				Name:     "echo.Echoer/Echo",
				SpanKind: oteltrace.SpanKindClient,
				Status: sdktrace.Status{
					Code:        codes.Error,
					Description: "test error",
				},
				Attributes: []attribute.KeyValue{
					// Note on Events (Logs):
					// The otelgrpc instrumentation also records "message" events (Sent/Received)
					// containing message sizes (compressed/uncompressed). These appear in the
					// "Logs" or "Events" tab in Cloud Trace. This test does not explicitly verify
					// them, but they are present in the generated span.

					// In Cloud Trace, non-zero status codes (like 13 for INTERNAL) are displayed
					// as errors in the "Status" field.
					keyRPCStatusCode.Int64(13),
					keyRPCMethod.String("Echo"),
					keyRPCService.String("echo.Echoer"),
					keyRPCSystem.String(valRPCSystemGRPC),
					keyServerAddr.String(valLocalhost),
				},
			}.Snapshot(),
			wantAttrKeys: []attribute.Key{keyServerPort},
		},
		{
			name:   "telemetry disabled",
			echoer: successfulEchoer,
			opts: &Options{
				DisableAuthentication: true,
				DisableTelemetry:      true,
			},
			wantSpans: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			exporter.Reset()

			l, err := net.Listen("tcp", "127.0.0.1:0")
			if err != nil {
				t.Fatalf("Failed to listen: %v", err)
			}
			s := grpc.NewServer()
			echo.RegisterEchoerServer(s, tt.echoer)
			go s.Serve(l)
			defer s.Stop()

			tt.opts.Endpoint = l.Addr().String()
			tt.opts.GRPCDialOpts = []grpc.DialOption{grpc.WithTransportCredentials(insecure.NewCredentials())}
			pool, err := Dial(context.Background(), false, tt.opts)
			if err != nil {
				t.Fatalf("Dial() = %v, want nil", err)
			}
			defer pool.Close()

			client := echo.NewEchoerClient(pool)
			_, err = client.Echo(context.Background(), &echo.EchoRequest{Message: "hello"})
			if (err != nil) != tt.wantErr {
				t.Errorf("client.Echo() error = %v, wantErr %v", err, tt.wantErr)
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
				// In Cloud Trace, SpanKind "Client" identifies this as an outgoing request,
				// often affecting the icon used in the trace visualization.
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
				//    a basic TracerProvider, so these attributes contain default values (e.g.,
				//    service.name="unknown_service:grpctransport.test") rather than production values.
				//
				// 2. Instrumentation Scope:
				//    - "otel.scope.name" (e.g., "go.opentelemetry.io/contrib/instrumentation/google.golang.org/grpc/otelgrpc")
				//    - "otel.scope.version" (e.g., "0.46.0")
				//    These identify the instrumentation library itself and are part of the
				//    OpenTelemetry data model, separate from Span attributes.
				//
				// 3. Exporter Attributes:
				//    - "g.co/agent" (e.g., "opentelemetry-go 1.20.0; google-cloud-trace-exporter 1.20.0")
				//    These are injected by specific exporters (like the Google Cloud Trace exporter)
				//    and are not present when using the InMemoryExporter.
				//
				// This test focuses on verifying the "rpc.*" and "server.*" attributes, which are
				// generated by the otelgrpc instrumentation library itself.

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
			}
		})
	}
}

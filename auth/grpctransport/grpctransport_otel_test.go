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

func TestDial_OpenTelemetry_Enabled(t *testing.T) {
	gax.TestOnlyResetIsFeatureEnabled()
	defer gax.TestOnlyResetIsFeatureEnabled()
	t.Setenv("GOOGLE_SDK_GO_EXPERIMENTAL_TRACING", "true")

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
	timeoutEchoer := &fakeEchoService{
		Fn: func(ctx context.Context, req *echo.EchoRequest) (*echo.EchoReply, error) {
			time.Sleep(100 * time.Millisecond)
			return &echo.EchoReply{Message: req.Message}, nil
		},
	}
	cancelEchoer := &fakeEchoService{
		Fn: func(ctx context.Context, req *echo.EchoRequest) (*echo.EchoReply, error) {
			time.Sleep(100 * time.Millisecond)
			return &echo.EchoReply{Message: req.Message}, nil
		},
	}

	tests := []struct {
		name               string
		echoer             echo.EchoerServer
		opts               *Options
		telemetryCtxValues map[string]string
		errorType          string // "timeout", "cancel"
		wantErr            bool
		wantSpans          int
		wantSpan           sdktrace.ReadOnlySpan
		wantAttrKeys       []attribute.Key
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
					keyRPCStatusCode.Int64(0),
					keyRPCMethod.String("Echo"),
					keyRPCService.String("echo.Echoer"),
					keyRPCSystem.String(valRPCSystemGRPC),
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
					keyRPCStatusCode.Int64(13),
					keyRPCMethod.String("Echo"),
					keyRPCService.String("echo.Echoer"),
					keyRPCSystem.String(valRPCSystemGRPC),
					keyServerAddr.String(valLocalhost),
					attribute.String("error.type", "*status.Error"),
					attribute.String("status.message", "test error"),
					attribute.String("rpc.response.status_code", "INTERNAL"),
				},
			}.Snapshot(),
			wantAttrKeys: []attribute.Key{keyServerPort},
		},
		{
			name:      "telemetry enabled client timeout",
			echoer:    timeoutEchoer,
			opts:      &Options{DisableAuthentication: true},
			errorType: "timeout",
			wantErr:   true,
			wantSpans: 1,
			wantSpan: tracetest.SpanStub{
				Name:     "echo.Echoer/Echo",
				SpanKind: oteltrace.SpanKindClient,
				Status: sdktrace.Status{
					Code:        codes.Error,
					Description: "context deadline exceeded",
				},
				Attributes: []attribute.KeyValue{
					keyRPCStatusCode.Int64(4),
					keyRPCMethod.String("Echo"),
					keyRPCService.String("echo.Echoer"),
					keyRPCSystem.String(valRPCSystemGRPC),
					keyServerAddr.String(valLocalhost),
					attribute.String("error.type", "CLIENT_TIMEOUT"),
					attribute.String("rpc.response.status_code", "DEADLINE_EXCEEDED"),
				},
			}.Snapshot(),
			wantAttrKeys: []attribute.Key{keyServerPort, attribute.Key("status.message"), attribute.Key("exception.type")},
		},
		{
			name:      "telemetry enabled client cancelled",
			echoer:    cancelEchoer,
			opts:      &Options{DisableAuthentication: true},
			errorType: "cancel",
			wantErr:   true,
			wantSpans: 1,
			wantSpan: tracetest.SpanStub{
				Name:     "echo.Echoer/Echo",
				SpanKind: oteltrace.SpanKindClient,
				Status: sdktrace.Status{
					Code:        codes.Error,
					Description: "context canceled",
				},
				Attributes: []attribute.KeyValue{
					keyRPCStatusCode.Int64(1),
					keyRPCMethod.String("Echo"),
					keyRPCService.String("echo.Echoer"),
					keyRPCSystem.String(valRPCSystemGRPC),
					keyServerAddr.String(valLocalhost),
					attribute.String("error.type", "CLIENT_CANCELLED"),
					attribute.String("rpc.response.status_code", "CANCELED"),
				},
			}.Snapshot(),
			wantAttrKeys: []attribute.Key{keyServerPort, attribute.Key("status.message"), attribute.Key("exception.type")},
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
		{
			name:   "telemetry enabled metadata enrichment",
			echoer: successfulEchoer,
			opts: &Options{
				DisableAuthentication: true,
				InternalOptions: &InternalOptions{
					TelemetryAttributes: map[string]string{
						"gcp.client.service":  "echo",
						"gcp.client.version":  "1.0.0",
						"gcp.client.repo":     "googleapis/google-cloud-go",
						"gcp.client.artifact": "c.g/auth/grpctransport",
						"gcp.client.language": "go",
						"url.domain":          "echo.googleapis.com",
						"ignored.key":         "should not be included",
					},
				},
			},
			telemetryCtxValues: map[string]string{"resource_name": "my-resource"},
			wantSpans:          1,
			wantSpan: tracetest.SpanStub{
				Name:     "echo.Echoer/Echo",
				SpanKind: oteltrace.SpanKindClient,
				Status: sdktrace.Status{
					Code: codes.Unset,
				},
				Attributes: []attribute.KeyValue{
					keyRPCStatusCode.Int64(0),
					keyRPCMethod.String("Echo"),
					keyRPCService.String("echo.Echoer"),
					keyRPCSystem.String(valRPCSystemGRPC),
					keyServerAddr.String(valLocalhost),
					attribute.String("gcp.resource.destination.id", "my-resource"),
					attribute.String("gcp.client.service", "echo"),
					attribute.String("gcp.client.version", "1.0.0"),
					attribute.String("gcp.client.repo", "googleapis/google-cloud-go"),
					attribute.String("gcp.client.artifact", "c.g/auth/grpctransport"),
					attribute.String("gcp.client.language", "go"),
					attribute.String("url.domain", "echo.googleapis.com"),
					attribute.String("rpc.response.status_code", "OK"),
				},
			}.Snapshot(),
			wantAttrKeys: []attribute.Key{keyServerPort},
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

			ctx := context.Background()
			var cancel context.CancelFunc
			if tt.errorType == "timeout" {
				ctx, cancel = context.WithTimeout(ctx, 10*time.Millisecond)
				defer cancel()
			} else if tt.errorType == "cancel" {
				ctx, cancel = context.WithCancel(ctx)
				time.AfterFunc(10*time.Millisecond, cancel)
			}

			for k, v := range tt.telemetryCtxValues {
				ctx = callctx.WithTelemetryContext(ctx, k, v)
			}

			client := echo.NewEchoerClient(pool)
			_, err = client.Echo(ctx, &echo.EchoRequest{Message: "hello"})
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

func TestDial_OpenTelemetry_Disabled(t *testing.T) {
	gax.TestOnlyResetIsFeatureEnabled()
	defer gax.TestOnlyResetIsFeatureEnabled()
	t.Setenv("GOOGLE_SDK_GO_EXPERIMENTAL_TRACING", "false")

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
	timeoutEchoer := &fakeEchoService{
		Fn: func(ctx context.Context, req *echo.EchoRequest) (*echo.EchoReply, error) {
			time.Sleep(100 * time.Millisecond)
			return &echo.EchoReply{Message: req.Message}, nil
		},
	}
	cancelEchoer := &fakeEchoService{
		Fn: func(ctx context.Context, req *echo.EchoRequest) (*echo.EchoReply, error) {
			time.Sleep(100 * time.Millisecond)
			return &echo.EchoReply{Message: req.Message}, nil
		},
	}

	tests := []struct {
		name               string
		echoer             echo.EchoerServer
		opts               *Options
		telemetryCtxValues map[string]string
		errorType          string // "timeout", "cancel"
		wantErr            bool
		wantSpans          int
		wantSpan           sdktrace.ReadOnlySpan
		wantAttrKeys       []attribute.Key
	}{
		{
			name:      "telemetry enabled success (but gated off)",
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
					keyRPCStatusCode.Int64(0),
					keyRPCMethod.String("Echo"),
					keyRPCService.String("echo.Echoer"),
					keyRPCSystem.String(valRPCSystemGRPC),
					keyServerAddr.String(valLocalhost),
				},
			}.Snapshot(),
			wantAttrKeys: []attribute.Key{keyServerPort},
		},
		{
			name:      "telemetry enabled error (but gated off)",
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
					keyRPCStatusCode.Int64(13),
					keyRPCMethod.String("Echo"),
					keyRPCService.String("echo.Echoer"),
					keyRPCSystem.String(valRPCSystemGRPC),
					keyServerAddr.String(valLocalhost),
					// Standard OTel attributes only, NO strict error.type/status.message/grpc.status
				},
			}.Snapshot(),
			wantAttrKeys: []attribute.Key{keyServerPort},
		},
		{
			name:      "telemetry enabled client timeout (but gated off)",
			echoer:    timeoutEchoer,
			opts:      &Options{DisableAuthentication: true},
			errorType: "timeout",
			wantErr:   true,
			wantSpans: 1,
			wantSpan: tracetest.SpanStub{
				Name:     "echo.Echoer/Echo",
				SpanKind: oteltrace.SpanKindClient,
				Status: sdktrace.Status{
					Code:        codes.Error,
					Description: "context deadline exceeded",
				},
				Attributes: []attribute.KeyValue{
					keyRPCStatusCode.Int64(4),
					keyRPCMethod.String("Echo"),
					keyRPCService.String("echo.Echoer"),
					keyRPCSystem.String(valRPCSystemGRPC),
					keyServerAddr.String(valLocalhost),
				},
			}.Snapshot(),
			wantAttrKeys: []attribute.Key{keyServerPort},
		},
		{
			name:      "telemetry enabled client cancelled (but gated off)",
			echoer:    cancelEchoer,
			opts:      &Options{DisableAuthentication: true},
			errorType: "cancel",
			wantErr:   true,
			wantSpans: 1,
			wantSpan: tracetest.SpanStub{
				Name:     "echo.Echoer/Echo",
				SpanKind: oteltrace.SpanKindClient,
				Status: sdktrace.Status{
					Code:        codes.Error,
					Description: "context canceled",
				},
				Attributes: []attribute.KeyValue{
					keyRPCStatusCode.Int64(1),
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
		{
			name:   "telemetry enabled metadata enrichment (but gated off)",
			echoer: successfulEchoer,
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
			wantSpans:          1,
			wantSpan: tracetest.SpanStub{
				Name:     "echo.Echoer/Echo",
				SpanKind: oteltrace.SpanKindClient,
				Status: sdktrace.Status{
					Code: codes.Unset,
				},
				Attributes: []attribute.KeyValue{
					keyRPCStatusCode.Int64(0),
					keyRPCMethod.String("Echo"),
					keyRPCService.String("echo.Echoer"),
					keyRPCSystem.String(valRPCSystemGRPC),
					keyServerAddr.String(valLocalhost),
					// NO gcp.* attributes
				},
			}.Snapshot(),
			wantAttrKeys: []attribute.Key{keyServerPort},
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

			ctx := context.Background()
			var cancel context.CancelFunc
			if tt.errorType == "timeout" {
				ctx, cancel = context.WithTimeout(ctx, 10*time.Millisecond)
				defer cancel()
			} else if tt.errorType == "cancel" {
				ctx, cancel = context.WithCancel(ctx)
				time.AfterFunc(10*time.Millisecond, cancel)
			}

			for k, v := range tt.telemetryCtxValues {
				ctx = callctx.WithTelemetryContext(ctx, k, v)
			}

			client := echo.NewEchoerClient(pool)
			_, err = client.Echo(ctx, &echo.EchoRequest{Message: "hello"})
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

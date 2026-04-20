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
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"
	"github.com/googleapis/gax-go/v2"
	"github.com/googleapis/gax-go/v2/apierror"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"

	"github.com/googleapis/gax-go/v2/callctx"
	oteltrace "go.opentelemetry.io/otel/trace"
	"google.golang.org/genproto/googleapis/rpc/errdetails"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/stats"
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
					keyRPCMethod.String("echo.Echoer/Echo"),
					attribute.String("rpc.system.name", "grpc"),
					keyServerAddr.String(valLocalhost),
					attribute.String("rpc.response.status_code", "OK"),
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
					keyRPCMethod.String("echo.Echoer/Echo"),
					attribute.String("rpc.system.name", "grpc"),
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
					keyRPCMethod.String("echo.Echoer/Echo"),
					attribute.String("rpc.system.name", "grpc"),
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
					keyRPCMethod.String("echo.Echoer/Echo"),
					attribute.String("rpc.system.name", "grpc"),
					keyServerAddr.String(valLocalhost),
					attribute.String("error.type", "CLIENT_CANCELLED"),
					attribute.String("rpc.response.status_code", "CANCELLED"),
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
					keyRPCMethod.String("echo.Echoer/Echo"),
					attribute.String("rpc.system.name", "grpc"),
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
					keyRPCMethod.String("echo.Echoer/Echo"),
					attribute.String("rpc.system.name", "grpc"),
					keyServerAddr.String(valLocalhost),
					attribute.String("rpc.response.status_code", "OK"),
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
					keyRPCMethod.String("echo.Echoer/Echo"),
					attribute.String("rpc.system.name", "grpc"),
					keyServerAddr.String(valLocalhost),
					attribute.String("rpc.response.status_code", "INTERNAL"),
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
					keyRPCMethod.String("echo.Echoer/Echo"),
					attribute.String("rpc.system.name", "grpc"),
					keyServerAddr.String(valLocalhost),
					attribute.String("rpc.response.status_code", "DEADLINE_EXCEEDED"),
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
					keyRPCMethod.String("echo.Echoer/Echo"),
					attribute.String("rpc.system.name", "grpc"),
					keyServerAddr.String(valLocalhost),
					attribute.String("rpc.response.status_code", "CANCELLED"),
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
					keyRPCMethod.String("echo.Echoer/Echo"),
					attribute.String("rpc.system.name", "grpc"),
					keyServerAddr.String(valLocalhost),
					attribute.String("rpc.response.status_code", "OK"),
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

func TestHandleRPC_ActionableErrors(t *testing.T) {
	// Do not add t.Parallel() to these tests. The global resetting will cause
	// flaky tests because they will stomp over each other's feature flag state.
	gax.TestOnlyResetIsFeatureEnabled()
	defer gax.TestOnlyResetIsFeatureEnabled()
	t.Setenv("GOOGLE_SDK_GO_EXPERIMENTAL_LOGGING", "true")

	stWithReason := status.New(grpccodes.Unavailable, "network timeout")
	ei := &errdetails.ErrorInfo{
		Reason: "RATE_LIMIT_EXCEEDED",
		Domain: "googleapis.com",
		Metadata: map[string]string{
			"quota_limit": "100",
		},
	}
	stWithReason, _ = stWithReason.WithDetails(ei)

	stDifferentReason := status.New(grpccodes.PermissionDenied, "not allowed")
	eiDiff := &errdetails.ErrorInfo{
		Reason: "IAM_PERMISSION_DENIED",
		Domain: "iam.googleapis.com",
	}
	stDifferentReason, _ = stDifferentReason.WithDetails(eiDiff)

	stMatchingMsgReason := status.New(grpccodes.PermissionDenied, "IAM_PERMISSION_DENIED: User does not have permission")
	eiMatching := &errdetails.ErrorInfo{
		Reason: "IAM_PERMISSION_DENIED",
		Domain: "iam.googleapis.com",
	}
	stMatchingMsgReason, _ = stMatchingMsgReason.WithDetails(eiMatching)

	stEmptyMsg := status.New(grpccodes.Internal, "")

	tests := []struct {
		name     string
		err      error
		setupCtx func(context.Context) (context.Context, context.CancelFunc)
		want     map[string]any
	}{
		{
			name: "ErrorInfo Actionable Error",
			err:  stWithReason.Err(),
			want: map[string]any{
				"level":                           "DEBUG",
				"msg":                             "network timeout",
				"rpc.system.name":                 "grpc",
				"rpc.response.status_code":        "UNAVAILABLE",
				"error.type":                      "RATE_LIMIT_EXCEEDED",
				"gcp.errors.domain":               "googleapis.com",
				"gcp.errors.metadata.quota_limit": "100",
				"gcp.client.version":              "1.2.3",
			},
		},
		{
			name: "Different ErrorInfo Reason and GRPC Status",
			err:  stDifferentReason.Err(),
			want: map[string]any{
				"level":                    "DEBUG",
				"msg":                      "not allowed",
				"rpc.system.name":          "grpc",
				"rpc.response.status_code": "PERMISSION_DENIED",
				"error.type":               "IAM_PERMISSION_DENIED",
				"gcp.errors.domain":        "iam.googleapis.com",
				"gcp.client.version":       "1.2.3",
			},
		},
		{
			name: "Matching Message and Reason",
			err:  stMatchingMsgReason.Err(),
			want: map[string]any{
				"level":                    "DEBUG",
				"msg":                      "IAM_PERMISSION_DENIED: User does not have permission",
				"rpc.system.name":          "grpc",
				"rpc.response.status_code": "PERMISSION_DENIED",
				"error.type":               "IAM_PERMISSION_DENIED",
				"gcp.errors.domain":        "iam.googleapis.com",
				"gcp.client.version":       "1.2.3",
			},
		},
		{
			name: "APIError Wrapped",
			err:  func() error { err, _ := apierror.FromError(stWithReason.Err()); return err }(),
			setupCtx: func(ctx context.Context) (context.Context, context.CancelFunc) {
				ctx = callctx.WithTelemetryContext(ctx, "resend_count", "3")
				ctx = callctx.WithTelemetryContext(ctx, "resource_name", "my-resource")
				return ctx, nil
			},
			want: map[string]any{
				"level":                           "DEBUG",
				"msg":                             "network timeout",
				"rpc.system.name":                 "grpc",
				"rpc.response.status_code":        "UNAVAILABLE",
				"error.type":                      "RATE_LIMIT_EXCEEDED",
				"gcp.errors.domain":               "googleapis.com",
				"gcp.errors.metadata.quota_limit": "100",
				"gcp.grpc.resend_count":           float64(3),
				"gcp.resource.destination.id":     "my-resource",
				"gcp.client.version":              "1.2.3",
			},
		},
		{
			name: "CLIENT_TIMEOUT",
			err:  context.DeadlineExceeded,
			setupCtx: func(ctx context.Context) (context.Context, context.CancelFunc) {
				return context.WithDeadline(ctx, time.Now().Add(-1*time.Second))
			},
			want: map[string]any{
				"level":                    "DEBUG",
				"msg":                      "context deadline exceeded",
				"rpc.system.name":          "grpc",
				"rpc.response.status_code": "DEADLINE_EXCEEDED",
				"error.type":               "CLIENT_TIMEOUT",
				"gcp.client.version":       "1.2.3",
			},
		},
		{
			name: "CLIENT_CANCELLED",
			err:  context.Canceled,
			setupCtx: func(ctx context.Context) (context.Context, context.CancelFunc) {
				return context.WithCancel(ctx)
			},
			want: map[string]any{
				"level":                    "DEBUG",
				"msg":                      "context canceled",
				"rpc.system.name":          "grpc",
				"rpc.response.status_code": "CANCELED",
				"error.type":               "CLIENT_CANCELLED",
				"gcp.client.version":       "1.2.3",
			},
		},
		{
			name: "Fallback error type",
			err:  errors.New("custom error"),
			want: map[string]any{
				"level":                    "DEBUG",
				"msg":                      "custom error",
				"rpc.system.name":          "grpc",
				"rpc.response.status_code": "UNKNOWN",
				"error.type":               "*errors.errorString",
				"gcp.client.version":       "1.2.3",
			},
		},
		{
			name: "Empty Message Status",
			err:  stEmptyMsg.Err(),
			want: map[string]any{
				"level":                    "DEBUG",
				"msg":                      "API call failed",
				"rpc.system.name":          "grpc",
				"rpc.response.status_code": "INTERNAL",
				"error.type":               "*status.Error",
				"gcp.client.version":       "1.2.3",
			},
		},
		{
			name: "Fast Exit No Logging No Tracing",
			err:  errors.New("should not log"),
			setupCtx: func(ctx context.Context) (context.Context, context.CancelFunc) {
				t.Setenv("GOOGLE_SDK_GO_EXPERIMENTAL_LOGGING", "false")
				gax.TestOnlyResetIsFeatureEnabled()
				return ctx, nil
			},
			want: nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv("GOOGLE_SDK_GO_EXPERIMENTAL_LOGGING", "true")
			gax.TestOnlyResetIsFeatureEnabled()

			var logBuf bytes.Buffer
			logger := slog.New(slog.NewJSONHandler(&logBuf, &slog.HandlerOptions{Level: slog.LevelDebug}))
			ctx := callctx.WithLoggerContext(context.Background(), logger)

			if tt.setupCtx != nil {
				var cancel context.CancelFunc
				ctx, cancel = tt.setupCtx(ctx)
				if cancel != nil {
					cancel()
					defer cancel()
				}
			}

			staticLogAttrs := []any{
				slog.String("gcp.client.version", "1.2.3"),
			}

			h := &otelHandler{
				Handler: &mockStatsHandler{},
				logger:  logger.With(staticLogAttrs...),
			}

			h.HandleRPC(ctx, &stats.End{Error: tt.err})

			logOutput := logBuf.String()

			if tt.want == nil {
				if strings.TrimSpace(logOutput) != "" {
					t.Fatalf("Expected no log output, got: %s", logOutput)
				}
				return
			}

			if strings.Count(strings.TrimSpace(logOutput), "\n") > 0 {
				t.Fatalf("Expected exactly 1 log record, got multiple: %s", logOutput)
			}

			var got map[string]any
			if err := json.Unmarshal(logBuf.Bytes(), &got); err != nil {
				t.Fatalf("failed to unmarshal log JSON: %v", err)
			}

			if _, ok := got["time"].(string); !ok {
				t.Errorf("Expected time attribute of type string, got: %v", got["time"])
			}
			delete(got, "time")

			if diff := cmp.Diff(tt.want, got); diff != "" {
				t.Errorf("Log attributes mismatch (-want +got):\n%s", diff)
			}
		})
	}
}

func TestDial_Telemetry_Combinations(t *testing.T) {
	// Ensure any lingering HTTP/2 connections are closed to avoid goroutine leaks.
	defer http.DefaultTransport.(*http.Transport).CloseIdleConnections()

	exporter := tracetest.NewInMemoryExporter()
	tp := sdktrace.NewTracerProvider(sdktrace.WithSyncer(exporter))
	defer tp.Shutdown(context.Background())

	// Restore the global tracer provider after the test to avoid side effects.
	defer func(prev oteltrace.TracerProvider) { otel.SetTracerProvider(prev) }(otel.GetTracerProvider())
	otel.SetTracerProvider(tp)

	errorEchoer := &fakeEchoService{
		Fn: func(ctx context.Context, req *echo.EchoRequest) (*echo.EchoReply, error) {
			return nil, status.Error(grpccodes.Internal, "test error")
		},
	}

	tests := []struct {
		name             string
		logging          bool
		tracing          bool
		metrics          bool
		wantLog          bool
		wantTracingAttrs bool
		wantMetricsAttrs bool
	}{
		{
			name:             "all disabled",
			logging:          false,
			tracing:          false,
			metrics:          false,
			wantLog:          false,
			wantTracingAttrs: false,
			wantMetricsAttrs: false,
		},
		{
			name:             "tracing enabled",
			logging:          false,
			tracing:          true,
			metrics:          false,
			wantLog:          false,
			wantTracingAttrs: true,
			wantMetricsAttrs: false,
		},
		{
			name:             "logging enabled",
			logging:          true,
			tracing:          false,
			metrics:          false,
			wantLog:          true,
			wantTracingAttrs: false,
			wantMetricsAttrs: false,
		},
		{
			name:             "metrics enabled",
			logging:          false,
			tracing:          false,
			metrics:          true,
			wantLog:          false,
			wantTracingAttrs: false,
			wantMetricsAttrs: true,
		},
		{
			name:             "tracing and logging enabled",
			logging:          true,
			tracing:          true,
			metrics:          false,
			wantLog:          true,
			wantTracingAttrs: true,
			wantMetricsAttrs: false,
		},
		{
			name:             "tracing and metrics enabled",
			logging:          false,
			tracing:          true,
			metrics:          true,
			wantLog:          false,
			wantTracingAttrs: true,
			wantMetricsAttrs: true,
		},
		{
			name:             "logging and metrics enabled",
			logging:          true,
			tracing:          false,
			metrics:          true,
			wantLog:          true,
			wantTracingAttrs: false,
			wantMetricsAttrs: true,
		},
		{
			name:             "all enabled",
			logging:          true,
			tracing:          true,
			metrics:          true,
			wantLog:          true,
			wantTracingAttrs: true,
			wantMetricsAttrs: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			exporter.Reset()
			gax.TestOnlyResetIsFeatureEnabled()
			defer gax.TestOnlyResetIsFeatureEnabled()

			if tt.logging {
				t.Setenv("GOOGLE_SDK_GO_EXPERIMENTAL_LOGGING", "true")
			} else {
				t.Setenv("GOOGLE_SDK_GO_EXPERIMENTAL_LOGGING", "false")
			}
			if tt.tracing {
				t.Setenv("GOOGLE_SDK_GO_EXPERIMENTAL_TRACING", "true")
			} else {
				t.Setenv("GOOGLE_SDK_GO_EXPERIMENTAL_TRACING", "false")
			}
			if tt.metrics {
				t.Setenv("GOOGLE_SDK_GO_EXPERIMENTAL_METRICS", "true")
			} else {
				t.Setenv("GOOGLE_SDK_GO_EXPERIMENTAL_METRICS", "false")
			}

			l, err := net.Listen("tcp", "localhost:0")
			if err != nil {
				t.Fatal(err)
			}
			gsrv := grpc.NewServer()
			echo.RegisterEchoerServer(gsrv, errorEchoer)
			go func() {
				if err := gsrv.Serve(l); err != nil {
					panic(err)
				}
			}()
			defer gsrv.Stop()

			var logBuf bytes.Buffer
			logger := slog.New(slog.NewJSONHandler(&logBuf, &slog.HandlerOptions{Level: slog.LevelDebug}))

			opts := &Options{
				Endpoint:              l.Addr().String(),
				DisableAuthentication: true,
				Logger:                logger,
				InternalOptions: &InternalOptions{
					TelemetryAttributes: map[string]string{
						"gcp.client.version": "1.2.3",
					},
				},
				GRPCDialOpts: []grpc.DialOption{grpc.WithTransportCredentials(insecure.NewCredentials())},
			}

			pool, err := Dial(context.Background(), false, opts)
			if err != nil {
				t.Fatalf("Dial() = %v, want nil", err)
			}
			defer pool.Close()

			data := &gax.TransportTelemetryData{}
			ctx := gax.InjectTransportTelemetry(context.Background(), data)

			client := echo.NewEchoerClient(pool)
			_, _ = client.Echo(ctx, &echo.EchoRequest{Message: "hello"})

			logOutput := logBuf.String()
			hasLog := strings.TrimSpace(logOutput) != ""

			if hasLog != tt.wantLog {
				t.Errorf("got log: %v, want: %v\noutput: %s", hasLog, tt.wantLog, logOutput)
			}

			hasMetricsAttrs := data.ServerAddress() != ""
			if hasMetricsAttrs != tt.wantMetricsAttrs {
				t.Errorf("got metrics attrs: %v, want: %v", hasMetricsAttrs, tt.wantMetricsAttrs)
			}

			spans := exporter.GetSpans()
			hasTracingAttrs := false
			for _, span := range spans {
				for _, attr := range span.Attributes {
					if attr.Key == "gcp.client.version" && attr.Value.AsString() == "1.2.3" {
						hasTracingAttrs = true
						break
					}
				}
			}

			if hasTracingAttrs != tt.wantTracingAttrs {
				t.Errorf("got tracing attrs: %v, want: %v", hasTracingAttrs, tt.wantTracingAttrs)
			}
		})
	}
}

type mockStatsHandler struct{}

func (m *mockStatsHandler) TagRPC(ctx context.Context, info *stats.RPCTagInfo) context.Context {
	return ctx
}

func (m *mockStatsHandler) HandleRPC(ctx context.Context, rs stats.RPCStats) {}

func (m *mockStatsHandler) TagConn(ctx context.Context, info *stats.ConnTagInfo) context.Context {
	return ctx
}

func (m *mockStatsHandler) HandleConn(ctx context.Context, cs stats.ConnStats) {}

func TestExtractHostPort(t *testing.T) {
	tests := []struct {
		target   string
		wantHost string
		wantPort int
	}{
		{"localhost:8080", "localhost", 8080},
		{"[::1]:443", "::1", 443},
		{"google.com", "google.com", 0},
		{"dns:///localhost:8080", "localhost", 8080},
		{"dns:///google.com:443", "google.com", 443},
		{"xds:///my-service:80", "my-service", 80},
		{"dns:///[::1]:8080", "::1", 8080},
		{"google.com:foo", "google.com", 0},
		{"dns://8.8.8.8/lb.example.com:443", "lb.example.com", 443},
	}

	for _, tt := range tests {
		t.Run(tt.target, func(t *testing.T) {
			gotHost, gotPort := extractHostPort(tt.target)
			if gotHost != tt.wantHost {
				t.Errorf("extractHostPort(%q) host = %q, want %q", tt.target, gotHost, tt.wantHost)
			}
			if gotPort != tt.wantPort {
				t.Errorf("extractHostPort(%q) port = %v, want %v", tt.target, gotPort, tt.wantPort)
			}
		})
	}
}

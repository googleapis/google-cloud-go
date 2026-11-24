/*
Copyright 2026 Google LLC

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

	http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package internal

import (
	"context"
	"fmt"
	"net"
	"sync"
	"testing"
	"time"

	"cloud.google.com/go/bigtable/internal/metricstest"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/sdk/resource"
	"google.golang.org/api/option"
	"google.golang.org/api/option/internaloption"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/resolver"
)

type staticDNSResolverBuilder struct{}

func (staticDNSResolverBuilder) Build(target resolver.Target, cc resolver.ClientConn, _ resolver.BuildOptions) (resolver.Resolver, error) {
	if err := cc.UpdateState(resolver.State{
		Addresses: []resolver.Address{{Addr: target.Endpoint()}},
	}); err != nil {
		return nil, err
	}
	return staticDNSResolver{}, nil
}

func (staticDNSResolverBuilder) Scheme() string { return "dns" }

type staticDNSResolver struct{}

func (staticDNSResolver) ResolveNow(resolver.ResolveNowOptions) {}
func (staticDNSResolver) Close()                                {}

func TestExporters_RespectTPCOptions(t *testing.T) {
	tests := []struct {
		name       string
		clientOpts []option.ClientOption
		wantTarget string
	}{
		{
			name: "universe domain routes both exporters to TPC monitoring endpoint",
			clientOpts: []option.ClientOption{
				internaloption.WithDefaultEndpointTemplate("bigtable.UNIVERSE_DOMAIN:443"),
				internaloption.WithDefaultMTLSEndpoint("bigtable.mtls.googleapis.com:443"),
				internaloption.WithDefaultUniverseDomain("googleapis.com"),
				option.WithScopes("https://www.googleapis.com/auth/bigtable.data"),
				option.WithUniverseDomain("tpc.example.test"),
			},
			wantTarget: "monitoring.tpc.example.test:443",
		},
		{
			name: "explicit endpoint routes both exporters to configured endpoint",
			clientOpts: []option.ClientOption{
				internaloption.WithDefaultEndpointTemplate("bigtable.UNIVERSE_DOMAIN:443"),
				option.WithEndpoint("custom-tpc.example.test:443"),
			},
			wantTarget: "custom-tpc.example.test:443",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv("GOOGLE_CLOUD_UNIVERSE_DOMAIN", "")
			t.Setenv("GOOGLE_API_USE_CLIENT_CERTIFICATE", "false")
			t.Setenv("GOOGLE_API_USE_MTLS_ENDPOINT", "auto")

			srv, err := metricstest.NewServer()
			if err != nil {
				t.Fatalf("metricstest.NewServer: %v", err)
			}
			go srv.Serve()
			defer srv.Shutdown()

			var mu sync.Mutex
			var dialed []string
			var unexpected []string
			dialer := func(ctx context.Context, addr string) (net.Conn, error) {
				mu.Lock()
				dialed = append(dialed, addr)
				if addr != tc.wantTarget {
					unexpected = append(unexpected, addr)
				}
				mu.Unlock()
				if addr != tc.wantTarget {
					return nil, fmt.Errorf("unexpected dial to %q, want %q", addr, tc.wantTarget)
				}
				return (&net.Dialer{}).DialContext(ctx, "tcp", srv.Endpoint)
			}

			origSamplePeriod := DefaultSamplePeriod
			DefaultSamplePeriod = 50 * time.Millisecond
			t.Cleanup(func() { DefaultSamplePeriod = origSamplePeriod })

			opts := append([]option.ClientOption{}, tc.clientOpts...)
			opts = append(opts,
				option.WithoutAuthentication(),
				option.WithGRPCDialOption(grpc.WithTransportCredentials(insecure.NewCredentials())),
				option.WithGRPCDialOption(grpc.WithResolvers(staticDNSResolverBuilder{})),
				option.WithGRPCDialOption(grpc.WithContextDialer(dialer)),
			)

			ctx := context.Background()

			// 1. Verify built-in monitoringExporter via NewFactory.
			factory, err := NewFactory(ctx, "test-project", "test-instance", "test-app", "", nil, opts...)
			if err != nil {
				t.Fatalf("NewFactory: %v", err)
			}
			if !factory.Enabled {
				t.Fatalf("NewFactory returned disabled factory")
			}
			factory.NewAsyncRefreshErrHandler()()
			factory.Shutdown()

			// 2. Verify OTel GCM exporter via newOtelMetricsContext with CreateExporterOptions.
			otelCtx, err := newOtelMetricsContext(ctx, metricsConfig{
				project:              "test-project",
				instance:             "test-instance",
				appProfile:           "test-app",
				clientName:           "test-client",
				clientUID:            "test-uid",
				interval:             DefaultSamplePeriod,
				monitoringClientOpts: CreateExporterOptions(opts...),
				resourceOpts: []resource.Option{resource.WithAttributes(
					attribute.String("cloud.account.id", "client-project"),
					attribute.String("cloud.region", "us-central1"),
					attribute.String("cloud.platform", "gcp"),
					attribute.String("host.id", "host-id"),
					attribute.String("host.name", "host-name"),
				)},
			})
			if err != nil {
				t.Fatalf("newOtelMetricsContext: %v", err)
			}
			c, err := otelCtx.OtelMeterProvider.Meter("test").Int64Counter("test.counter")
			if err != nil {
				otelCtx.close()
				t.Fatalf("Int64Counter: %v", err)
			}
			c.Add(ctx, 1)
			otelCtx.close()

			mu.Lock()
			gotDialed := append([]string(nil), dialed...)
			gotUnexpected := append([]string(nil), unexpected...)
			mu.Unlock()

			if len(gotUnexpected) > 0 {
				t.Errorf("exporters dialed unexpected endpoints %v, want only %q", gotUnexpected, tc.wantTarget)
			}
			if len(gotDialed) < 2 {
				t.Errorf("expected both exporters to dial %q, got dialed=%v", tc.wantTarget, gotDialed)
			}
		})
	}
}

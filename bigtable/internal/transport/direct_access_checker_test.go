// Copyright 2026 Google LLC
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

package internal

import (
	"context"
	"errors"
	"net"
	"testing"
	"time"

	btopt "cloud.google.com/go/bigtable/internal/option"
	"go.opentelemetry.io/otel/attribute"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/metric/metricdata"
	"google.golang.org/api/option"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/status"
	"google.golang.org/grpc/test/bufconn"

	btpb "cloud.google.com/go/bigtable/apiv2/bigtablepb"
)

func defaultBigtableOpts(t *testing.T) []option.ClientOption {
	t.Helper()
	t.Setenv("BIGTABLE_EMULATOR_HOST", "")
	opts, err := btopt.DefaultClientOptions(
		"bigtable.UNIVERSE_DOMAIN:443",
		"bigtable.mtls.googleapis.com:443",
		"https://www.googleapis.com/auth/bigtable.data",
		"test-ua",
	)
	if err != nil {
		t.Fatalf("DefaultClientOptions() failed: %v", err)
	}
	return opts
}

func TestIsGCUEndpoint(t *testing.T) {
	baseOpts := defaultBigtableOpts(t)

	tests := []struct {
		name      string
		envUD     string
		extraOpts []option.ClientOption
		useBase   bool
		want      bool
	}{
		{
			name:    "default_gcu",
			useBase: true,
			want:    true,
		},
		{
			name:      "explicit_gcu_universe_domain",
			useBase:   true,
			extraOpts: []option.ClientOption{option.WithUniverseDomain("googleapis.com")},
			want:      true,
		},
		{
			name:      "sandbox_googleapis_endpoint",
			useBase:   true,
			extraOpts: []option.ClientOption{option.WithEndpoint("test-bigtable.sandbox.googleapis.com:443")},
			want:      true,
		},
		{
			name:      "dns_scheme_googleapis_endpoint",
			useBase:   true,
			extraOpts: []option.ClientOption{option.WithEndpoint("dns:///bigtable.googleapis.com:443")},
			want:      true,
		},
		{
			name:      "google_c2p_scheme_googleapis_endpoint",
			useBase:   true,
			extraOpts: []option.ClientOption{option.WithEndpoint("google-c2p:///bigtable.googleapis.com")},
			want:      true,
		},
		{
			name:      "uppercase_with_trailing_dot",
			useBase:   true,
			extraOpts: []option.ClientOption{option.WithEndpoint("BIGTABLE.GOOGLEAPIS.COM.:443")},
			want:      true,
		},
		{
			name:      "tpc_with_universe_domain",
			useBase:   true,
			extraOpts: []option.ClientOption{option.WithUniverseDomain("apis-tpclp.goog")},
			want:      false,
		},
		{
			name:    "tpc_env_universe_domain",
			useBase: true,
			envUD:   "apis-tpclp.goog",
			want:    false,
		},
		{
			name:      "tpc_with_endpoint_only",
			useBase:   true,
			extraOpts: []option.ClientOption{option.WithEndpoint("bigtable.apis-tpclp.goog:443")},
			want:      false,
		},
		{
			name:      "tpc_universe_domain_with_gcu_endpoint_override",
			useBase:   true,
			extraOpts: []option.ClientOption{option.WithUniverseDomain("apis-tpclp.goog"), option.WithEndpoint("bigtable.googleapis.com:443")},
			want:      false,
		},
		{
			name:      "localhost_endpoint",
			useBase:   true,
			extraOpts: []option.ClientOption{option.WithEndpoint("127.0.0.1:8080")},
			want:      false,
		},
		{
			name:      "ipv6_loopback_endpoint",
			useBase:   true,
			extraOpts: []option.ClientOption{option.WithEndpoint("[::1]:8080")},
			want:      false,
		},
		{
			name:      "passthrough_scheme_rejected",
			useBase:   true,
			extraOpts: []option.ClientOption{option.WithEndpoint("passthrough:///bigtable.googleapis.com:443")},
			want:      false,
		},
		{
			name:      "unix_scheme_rejected",
			useBase:   true,
			extraOpts: []option.ClientOption{option.WithEndpoint("unix:///tmp/bigtable.sock")},
			want:      false,
		},
		{
			name:    "empty_options",
			useBase: false,
			want:    false,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv("GOOGLE_CLOUD_UNIVERSE_DOMAIN", tc.envUD)
			var opts []option.ClientOption
			if tc.useBase {
				opts = append(opts, baseOpts...)
			}
			opts = append(opts, tc.extraOpts...)

			if got := IsGCUEndpoint(opts...); got != tc.want {
				t.Errorf("IsGCUEndpoint(%s) = %v, want %v", tc.name, got, tc.want)
			}
		})
	}
}

func TestNoopDirectAccessChecker(t *testing.T) {
	c := NewNoopDirectAccessChecker()
	conn, compatible := c.CheckCompatibility(t.Context())
	if compatible {
		t.Errorf("NewNoopDirectAccessChecker().CheckCompatibility() compatible = true, want false")
	}
	if conn != nil {
		t.Errorf("NewNoopDirectAccessChecker().CheckCompatibility() conn = %v, want nil", conn)
	}
	if got := c.Dialer(); got != nil {
		t.Errorf("NewNoopDirectAccessChecker().Dialer() = %p, want nil", got)
	}
	if got := c.LastProbeConfig(); got != nil {
		t.Errorf("NewNoopDirectAccessChecker().LastProbeConfig() = %v, want nil", got)
	}
}

func TestInvestigateDirectAccessFailureWithEnv(t *testing.T) {
	v4 := net.ParseIP("10.128.0.2")
	v6 := net.ParseIP("2600:1900:4000::2")
	sentinelErr := errors.New("check failed")

	tests := []struct {
		name        string
		notOnGCP    bool
		metaDown    bool
		hasIPv4     bool
		hasIPv6     bool
		loDown      bool
		loV4Missing bool
		loV6Missing bool
		v4Plumbed   bool
		v6Plumbed   bool
		wantReason  string
	}{
		{
			name:       "not_in_gcp",
			notOnGCP:   true,
			wantReason: "not_in_gcp",
		},
		{
			name:       "metadata_unreachable",
			metaDown:   true,
			wantReason: "metadata_unreachable",
		},
		{
			name:       "no_ip_assigned",
			wantReason: "no_ip_assigned",
		},
		{
			name:       "loopback_misconfigured",
			hasIPv4:    true,
			loDown:     true,
			wantReason: "loopback_misconfigured",
		},
		{
			name:        "loopback_misconfigured_ipv4",
			hasIPv4:     true,
			loV4Missing: true,
			wantReason:  "loopback_misconfigured_ipv4",
		},
		{
			name:        "loopback_misconfigured_ipv6",
			hasIPv6:     true,
			loV6Missing: true,
			wantReason:  "loopback_misconfigured_ipv6",
		},
		{
			name:       "all_local_checks_pass_reports_unknown",
			hasIPv4:    true,
			hasIPv6:    true,
			v4Plumbed:  true,
			v6Plumbed:  true,
			wantReason: "unknown",
		},
		{
			name:       "containerized_unplumbed_ips_still_reports_unknown",
			hasIPv4:    true,
			hasIPv6:    true,
			wantReason: "unknown",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			env := directAccessEnv{
				isRunningOnGCP: func() error {
					if tc.notOnGCP {
						return sentinelErr
					}
					return nil
				},
				checkMetadataReachability: func() error {
					if tc.metaDown {
						return sentinelErr
					}
					return nil
				},
				fetchIPFromMetadata: func(addrFamily string) (*net.IP, error) {
					if addrFamily == "IPv4" && tc.hasIPv4 {
						ip := v4
						return &ip, nil
					}
					if addrFamily == "IPv6" && tc.hasIPv6 {
						ip := v6
						return &ip, nil
					}
					return nil, sentinelErr
				},
				checkLoopbackInterfaceUp: func() error {
					if tc.loDown {
						return sentinelErr
					}
					return nil
				},
				checkLocalIPv4Loopback: func() error {
					if tc.loV4Missing {
						return sentinelErr
					}
					return nil
				},
				checkLocalIPv6Loopback: func() error {
					if tc.loV6Missing {
						return sentinelErr
					}
					return nil
				},
				checkLocalIPv4Addresses: func(*net.IP) (*net.Interface, error) {
					if tc.v4Plumbed {
						return &net.Interface{}, nil
					}
					return nil, sentinelErr
				},
				checkLocalIPv6Addresses: func(*net.IP) (*net.Interface, error) {
					if tc.v6Plumbed {
						return &net.Interface{}, nil
					}
					return nil, sentinelErr
				},
			}

			var gotReason string
			investigateDirectAccessFailureWithEnv(nil, func(reason string) {
				gotReason = reason
			}, errors.New("prime failed"), env)

			if gotReason != tc.wantReason {
				t.Errorf("investigateDirectAccessFailureWithEnv(%s) reason = %q, want %q", tc.name, gotReason, tc.wantReason)
			}
		})
	}
}

type failingConfigServer struct {
	btpb.UnimplementedBigtableServer
}

func (*failingConfigServer) PingAndWarm(context.Context, *btpb.PingAndWarmRequest) (*btpb.PingAndWarmResponse, error) {
	return nil, status.Error(codes.Unavailable, "unavailable")
}

func (*failingConfigServer) GetClientConfiguration(context.Context, *btpb.GetClientConfigurationRequest) (*btpb.ClientConfiguration, error) {
	return nil, status.Error(codes.Unavailable, "unavailable")
}

func newBufconnDialer(t *testing.T) func() (*BigtableConn, error) {
	t.Helper()
	lis := bufconn.Listen(1024 * 1024)
	t.Cleanup(func() { _ = lis.Close() })

	srv := grpc.NewServer()
	btpb.RegisterBigtableServer(srv, &failingConfigServer{})
	t.Cleanup(srv.Stop)
	go func() { _ = srv.Serve(lis) }()

	return func() (*BigtableConn, error) {
		conn, err := grpc.NewClient(
			"passthrough:///bufnet",
			grpc.WithContextDialer(func(ctx context.Context, _ string) (net.Conn, error) {
				return lis.DialContext(ctx)
			}),
			grpc.WithTransportCredentials(insecure.NewCredentials()),
		)
		if err != nil {
			return nil, err
		}
		return NewBigtableConn(conn), nil
	}
}

func waitForRecordedReason(ctx context.Context, reader *sdkmetric.ManualReader) (string, bool) {
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		var rm metricdata.ResourceMetrics
		if err := reader.Collect(ctx, &rm); err == nil {
			for _, sm := range rm.ScopeMetrics {
				for _, m := range sm.Metrics {
					if m.Name != "direct_access/compatible" {
						continue
					}
					gauge, ok := m.Data.(metricdata.Gauge[int64])
					if !ok || len(gauge.DataPoints) == 0 {
						continue
					}
					val, found := gauge.DataPoints[0].Attributes.Value(attribute.Key("reason"))
					if found {
						return val.AsString(), true
					}
				}
			}
		}
		time.Sleep(10 * time.Millisecond)
	}
	return "", false
}

func TestCheckers_InvestigateFailureRecordsMetricReason(t *testing.T) {
	dialer := newBufconnDialer(t)

	t.Run("classic_checker", func(t *testing.T) {
		reader := sdkmetric.NewManualReader()
		mp := sdkmetric.NewMeterProvider(sdkmetric.WithReader(reader))
		t.Cleanup(func() { _ = mp.Shutdown(context.Background()) })

		primer := newPingAndWarmChannelPrimer("projects/p/instances/i", "ap", nil)
		checker := NewPingAndWarmDirectAccessChecker(dialer, primer, mp, nil)
		conn, ok := checker.CheckCompatibility(t.Context())
		if ok || conn != nil {
			t.Fatalf("CheckCompatibility() = (%v, %v), want (nil, false)", conn, ok)
		}

		reason, recorded := waitForRecordedReason(t.Context(), reader)
		if !recorded || reason == "" {
			t.Errorf("CheckCompatibility() failure metric recorded = %v, reason = %q, want non-empty reason", recorded, reason)
		}
	})

	t.Run("session_checker", func(t *testing.T) {
		reader := sdkmetric.NewManualReader()
		mp := sdkmetric.NewMeterProvider(sdkmetric.WithReader(reader))
		t.Cleanup(func() { _ = mp.Shutdown(context.Background()) })

		checker := NewSessionClientDirectAccessChecker(dialer, "projects/p/instances/i", "ap", nil, mp, nil)
		conn, ok := checker.CheckCompatibility(t.Context())
		if ok || conn != nil {
			t.Fatalf("CheckCompatibility() = (%v, %v), want (nil, false)", conn, ok)
		}

		reason, recorded := waitForRecordedReason(t.Context(), reader)
		if !recorded || reason == "" {
			t.Errorf("CheckCompatibility() failure metric recorded = %v, reason = %q, want non-empty reason", recorded, reason)
		}
	})
}

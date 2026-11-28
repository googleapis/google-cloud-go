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
	"net"
	"sync/atomic"
	"testing"
	"time"

	btpb "cloud.google.com/go/bigtable/apiv2/bigtablepb"
	btopt "cloud.google.com/go/bigtable/internal/option"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/metric/metricdata"
	"google.golang.org/api/option"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/resolver"
	"google.golang.org/grpc/resolver/manual"
	"google.golang.org/grpc/test/bufconn"
)

type dummyBigtableServer struct {
	btpb.UnimplementedBigtableServer
	pingCount atomic.Int32
}

func (s *dummyBigtableServer) PingAndWarm(ctx context.Context, req *btpb.PingAndWarmRequest) (*btpb.PingAndWarmResponse, error) {
	s.pingCount.Add(1)
	return &btpb.PingAndWarmResponse{}, nil
}

func TestCreateAndStartManagedChannelPool_Classic(t *testing.T) {
	ctx := context.Background()

	lis := bufconn.Listen(1024 * 1024)
	defer lis.Close()

	srv := grpc.NewServer()
	btpb.RegisterBigtableServer(srv, &dummyBigtableServer{})
	go srv.Serve(lis)
	defer srv.Stop()

	o := []option.ClientOption{
		option.WithEndpoint("passthrough:///bufnet"),
		option.WithoutAuthentication(),
		option.WithGRPCDialOption(grpc.WithContextDialer(func(context.Context, string) (net.Conn, error) {
			return lis.Dial()
		})),
		option.WithGRPCDialOption(grpc.WithTransportCredentials(insecure.NewCredentials())),
	}

	mPool, err := CreateAndStartManagedChannelPool(
		ctx,
		"my-project",
		"my-instance",
		ChannelPoolConfig{
			DisableDynamicChannelPool: true,
			DisableConnectionRecycler: true,
		},
		nil,
		o,
		nil,
		metadata.MD{},
		time.Now(),
		false, // enableBigtableConnPool
	)
	if err != nil {
		t.Fatalf("CreateAndStartManagedChannelPool failed: %v", err)
	}
	defer mPool.Close()

	if mPool.Pool == nil {
		t.Error("Expected pool to be non-nil")
	}
	if mPool.Dsm != nil {
		t.Error("Expected DSM to be nil for classic pool")
	}
	if mPool.ConnRecycler != nil {
		t.Error("Expected connection recycler to be nil for classic pool")
	}
}

func TestCreateAndStartManagedChannelPool_BigtableConnPool(t *testing.T) {
	ctx := context.Background()

	lis := bufconn.Listen(1024 * 1024)
	defer lis.Close()

	srv := grpc.NewServer()
	btpb.RegisterBigtableServer(srv, &dummyBigtableServer{})
	go srv.Serve(lis)
	defer srv.Stop()

	o := []option.ClientOption{
		option.WithEndpoint("passthrough:///bufnet"),
		option.WithoutAuthentication(),
		option.WithGRPCDialOption(grpc.WithContextDialer(func(context.Context, string) (net.Conn, error) {
			return lis.Dial()
		})),
		option.WithGRPCDialOption(grpc.WithTransportCredentials(insecure.NewCredentials())),
		option.WithGRPCConnectionPool(2),
	}

	mPool, err := CreateAndStartManagedChannelPool(
		ctx,
		"my-project",
		"my-instance",
		ChannelPoolConfig{
			DisableDynamicChannelPool: false,
			DisableConnectionRecycler: false,
		},
		nil,
		o,
		nil,
		metadata.MD{},
		time.Now(),
		true, // enableBigtableConnPool
	)
	if err != nil {
		t.Fatalf("CreateAndStartManagedChannelPool failed: %v", err)
	}
	defer mPool.Close()

	if mPool.Pool == nil {
		t.Error("Expected pool to be non-nil")
	}
	if mPool.Dsm == nil {
		t.Error("Expected DSM to be started for Bigtable pool")
	}
	if mPool.ConnRecycler == nil {
		t.Error("Expected connection recycler to be started for Bigtable pool")
	}
}

func TestManagedChannelPool_Close(t *testing.T) {
	ctx := context.Background()

	lis := bufconn.Listen(1024 * 1024)
	defer lis.Close()

	srv := grpc.NewServer()
	btpb.RegisterBigtableServer(srv, &dummyBigtableServer{})
	go srv.Serve(lis)
	defer srv.Stop()

	o := []option.ClientOption{
		option.WithEndpoint("passthrough:///bufnet"),
		option.WithoutAuthentication(),
		option.WithGRPCDialOption(grpc.WithContextDialer(func(context.Context, string) (net.Conn, error) {
			return lis.Dial()
		})),
		option.WithGRPCDialOption(grpc.WithTransportCredentials(insecure.NewCredentials())),
		option.WithGRPCConnectionPool(2),
	}

	mPool, err := CreateAndStartManagedChannelPool(
		ctx,
		"my-project",
		"my-instance",
		ChannelPoolConfig{
			DisableDynamicChannelPool: false,
			DisableConnectionRecycler: false,
		},
		nil,
		o,
		nil,
		metadata.MD{},
		time.Now(),
		true, // enableBigtableConnPool
	)
	if err != nil {
		t.Fatalf("CreateAndStartManagedChannelPool failed: %v", err)
	}

	// Close it and verify no panic
	if err := mPool.Close(); err != nil {
		t.Errorf("mPool.Close() returned error: %v", err)
	}
}

func newChannelPoolTestEnv(t *testing.T, extraOpts ...option.ClientOption) (*dummyBigtableServer, *sdkmetric.ManualReader, *sdkmetric.MeterProvider, []option.ClientOption) {
	t.Helper()
	t.Setenv("BIGTABLE_EMULATOR_HOST", "")
	t.Setenv("CBT_ENABLE_DIRECTPATH", "")

	lis, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("net.Listen() failed: %v", err)
	}
	t.Cleanup(func() { _ = lis.Close() })

	fakeSrv := &dummyBigtableServer{}
	srv := grpc.NewServer()
	btpb.RegisterBigtableServer(srv, fakeSrv)
	t.Cleanup(srv.Stop)
	go func() { _ = srv.Serve(lis) }()

	dnsResolver := manual.NewBuilderWithScheme("dns")
	dnsResolver.InitialState(resolver.State{
		Addresses: []resolver.Address{{Addr: lis.Addr().String()}},
	})

	baseOpts, err := btopt.DefaultClientOptions(
		"dns:///bigtable.UNIVERSE_DOMAIN:443",
		"bigtable.mtls.googleapis.com:443",
		"https://www.googleapis.com/auth/bigtable.data",
		"test-ua",
	)
	if err != nil {
		t.Fatalf("DefaultClientOptions() failed: %v", err)
	}
	opts := append(baseOpts,
		option.WithoutAuthentication(),
		option.WithGRPCConnectionPool(2),
		option.WithGRPCDialOption(grpc.WithResolvers(dnsResolver)),
		option.WithGRPCDialOption(grpc.WithTransportCredentials(insecure.NewCredentials())),
	)
	opts = append(opts, extraOpts...)

	reader := sdkmetric.NewManualReader()
	mp := sdkmetric.NewMeterProvider(sdkmetric.WithReader(reader))
	t.Cleanup(func() { _ = mp.Shutdown(context.Background()) })

	return fakeSrv, reader, mp, opts
}

func TestCreateBigtableChannelPool_GCUDirectAccessChecker(t *testing.T) {
	t.Run("enabled_runs_probe", func(t *testing.T) {
		fakeSrv, reader, mp, opts := newChannelPoolTestEnv(t)

		pool, err := CreateBigtableChannelPool(t.Context(), "my-project", "my-instance", ChannelPoolConfig{}, mp, opts, btopt.DirectAccessOptions(), metadata.MD{}, time.Now())
		if err != nil {
			t.Fatalf("CreateBigtableChannelPool() failed: %v", err)
		}
		t.Cleanup(func() { _ = pool.Close() })

		if got, want := fakeSrv.pingCount.Load(), int32(3); got != want {
			t.Errorf("CreateBigtableChannelPool() PingAndWarm count = %d, want %d (1 probe + 2 pool conns)", got, want)
		}
		reason, recorded := waitForRecordedReason(t.Context(), reader)
		if !recorded || reason == "" || reason == "manually_disabled" {
			t.Errorf("CreateBigtableChannelPool() recorded = %v, reason = %q, want diagnostic failure reason", recorded, reason)
		}
	})

	t.Run("disabled_records_manually_disabled", func(t *testing.T) {
		fakeSrv, reader, mp, opts := newChannelPoolTestEnv(t)

		pool, err := CreateBigtableChannelPool(t.Context(), "my-project", "my-instance", ChannelPoolConfig{DisableDirectAccess: true}, mp, opts, btopt.DirectAccessOptions(), metadata.MD{}, time.Now())
		if err != nil {
			t.Fatalf("CreateBigtableChannelPool(DisableDirectAccess=true) failed: %v", err)
		}
		t.Cleanup(func() { _ = pool.Close() })

		if got, want := fakeSrv.pingCount.Load(), int32(2); got != want {
			t.Errorf("CreateBigtableChannelPool(DisableDirectAccess=true) PingAndWarm count = %d, want %d", got, want)
		}
		reason, recorded := waitForRecordedReason(t.Context(), reader)
		if !recorded || reason != "manually_disabled" {
			t.Errorf("CreateBigtableChannelPool(DisableDirectAccess=true) recorded = %v, reason = %q, want %q", recorded, reason, "manually_disabled")
		}
	})
}

func TestCreateBigtableChannelPool_NonGCUSkipsDirectAccessChecker(t *testing.T) {
	tests := []struct {
		name      string
		extraOpts []option.ClientOption
	}{
		{
			name:      "tpc_universe_domain",
			extraOpts: []option.ClientOption{option.WithUniverseDomain("apis-tpclp.goog")},
		},
		{
			name:      "tpc_endpoint",
			extraOpts: []option.ClientOption{option.WithEndpoint("dns:///bigtable.apis-tpclp.goog:443")},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			fakeSrv, reader, mp, opts := newChannelPoolTestEnv(t, tc.extraOpts...)

			pool, err := CreateBigtableChannelPool(t.Context(), "my-project", "my-instance", ChannelPoolConfig{}, mp, opts, btopt.DirectAccessOptions(), metadata.MD{}, time.Now())
			if err != nil {
				t.Fatalf("CreateBigtableChannelPool(%s) failed: %v", tc.name, err)
			}
			t.Cleanup(func() { _ = pool.Close() })

			if got, want := fakeSrv.pingCount.Load(), int32(2); got != want {
				t.Errorf("CreateBigtableChannelPool(%s) PingAndWarm count = %d, want %d", tc.name, got, want)
			}

			var rm metricdata.ResourceMetrics
			if err := reader.Collect(t.Context(), &rm); err != nil {
				t.Fatalf("reader.Collect() failed: %v", err)
			}
			for _, sm := range rm.ScopeMetrics {
				for _, m := range sm.Metrics {
					if m.Name == "direct_access/compatible" {
						t.Errorf("CreateBigtableChannelPool(%s) unexpectedly recorded direct_access/compatible metric", tc.name)
					}
				}
			}
		})
	}
}

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

package bigtable

import (
	"context"
	"net"
	"testing"
	"time"

	btpb "cloud.google.com/go/bigtable/apiv2/bigtablepb"
	"google.golang.org/api/option"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/test/bufconn"
)

type dummyBigtableServer struct {
	btpb.UnimplementedBigtableServer
}

func (s *dummyBigtableServer) PingAndWarm(ctx context.Context, req *btpb.PingAndWarmRequest) (*btpb.PingAndWarmResponse, error) {
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

	metricsTracerFactory := &builtinMetricsTracerFactory{}

	mPool, err := createAndStartManagedChannelPool(
		ctx,
		"my-project",
		"my-instance",
		ClientConfig{
			DisableDynamicChannelPool: true,
			DisableConnectionRecycler: true,
		},
		metricsTracerFactory,
		o,
		nil,
		metadata.MD{},
		time.Now(),
		false, // enableBigtableConnPool
	)
	if err != nil {
		t.Fatalf("createAndStartManagedChannelPool failed: %v", err)
	}
	defer mPool.Close()

	if mPool.pool == nil {
		t.Error("Expected pool to be non-nil")
	}
	if mPool.dsm != nil {
		t.Error("Expected DSM to be nil for classic pool")
	}
	if mPool.connRecycler != nil {
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

	metricsTracerFactory := &builtinMetricsTracerFactory{}

	mPool, err := createAndStartManagedChannelPool(
		ctx,
		"my-project",
		"my-instance",
		ClientConfig{
			DisableDynamicChannelPool: false,
			DisableConnectionRecycler: false,
		},
		metricsTracerFactory,
		o,
		nil,
		metadata.MD{},
		time.Now(),
		true, // enableBigtableConnPool
	)
	if err != nil {
		t.Fatalf("createAndStartManagedChannelPool failed: %v", err)
	}
	defer mPool.Close()

	if mPool.pool == nil {
		t.Error("Expected pool to be non-nil")
	}
	if mPool.dsm == nil {
		t.Error("Expected DSM to be started for Bigtable pool")
	}
	if mPool.connRecycler == nil {
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

	metricsTracerFactory := &builtinMetricsTracerFactory{}

	mPool, err := createAndStartManagedChannelPool(
		ctx,
		"my-project",
		"my-instance",
		ClientConfig{
			DisableDynamicChannelPool: false,
			DisableConnectionRecycler: false,
		},
		metricsTracerFactory,
		o,
		nil,
		metadata.MD{},
		time.Now(),
		true, // enableBigtableConnPool
	)
	if err != nil {
		t.Fatalf("createAndStartManagedChannelPool failed: %v", err)
	}

	// Close it and verify no panic
	if err := mPool.Close(); err != nil {
		t.Errorf("mPool.Close() returned error: %v", err)
	}
}

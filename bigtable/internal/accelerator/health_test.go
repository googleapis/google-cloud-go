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

package accelerator

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	healthpb "google.golang.org/grpc/health/grpc_health_v1"
)

// startHealthTestServer boots a daemon on a temp UDS and returns a client for
// its health service. The zero-value Channel is deliberate: a health RPC must
// never reach the Channel, so leaving it unusable means a regression in the
// isProxied gate shows up as a failure here rather than as a passing test that
// quietly forwarded the probe to Bigtable.
func startHealthTestServer(t *testing.T) (healthpb.HealthClient, *Server) {
	t.Helper()
	udsPath := filepath.Join(t.TempDir(), "bt_proxy.sock")

	server := NewServer(udsPath, &Channel{}, WithStdinReader(nil))
	if err := server.Start(); err != nil {
		t.Fatalf("Start: %v", err)
	}
	t.Cleanup(server.Stop)

	conn, err := grpc.NewClient("unix://"+udsPath, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		t.Fatalf("grpc.NewClient: %v", err)
	}
	t.Cleanup(func() { conn.Close() })

	return healthpb.NewHealthClient(conn), server
}

// A unary RPC on a locally-served service reaches its real handler. Without the
// isProxied gate the proxy interceptor would swallow this and try to forward it
// to Bigtable, since it never calls the next handler.
func TestServer_HealthCheck(t *testing.T) {
	client, _ := startHealthTestServer(t)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	resp, err := client.Check(ctx, &healthpb.HealthCheckRequest{})
	if err != nil {
		t.Fatalf("Check: %v", err)
	}
	if resp.GetStatus() != healthpb.HealthCheckResponse_SERVING {
		t.Errorf("status = %v, want SERVING", resp.GetStatus())
	}
}

// The same, for the streaming half of the gate: Health/Watch is a locally
// served server-streaming method, so proxyStreamInterceptor must hand it off
// rather than opening a Channel stream for it.
func TestServer_HealthWatch(t *testing.T) {
	client, _ := startHealthTestServer(t)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	stream, err := client.Watch(ctx, &healthpb.HealthCheckRequest{})
	if err != nil {
		t.Fatalf("Watch: %v", err)
	}
	resp, err := stream.Recv()
	if err != nil {
		t.Fatalf("Recv: %v", err)
	}
	if resp.GetStatus() != healthpb.HealthCheckResponse_SERVING {
		t.Errorf("first status = %v, want SERVING", resp.GetStatus())
	}
}

// A caller watching the daemon is told it is going away, rather than being left
// to infer it from the connection dropping.
func TestServer_HealthNotServingOnStop(t *testing.T) {
	client, server := startHealthTestServer(t)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	stream, err := client.Watch(ctx, &healthpb.HealthCheckRequest{})
	if err != nil {
		t.Fatalf("Watch: %v", err)
	}
	if resp, err := stream.Recv(); err != nil {
		t.Fatalf("Recv (initial): %v", err)
	} else if resp.GetStatus() != healthpb.HealthCheckResponse_SERVING {
		t.Fatalf("first status = %v, want SERVING", resp.GetStatus())
	}

	go server.Stop()

	resp, err := stream.Recv()
	if err != nil {
		t.Fatalf("Recv (after Stop): %v", err)
	}
	if resp.GetStatus() != healthpb.HealthCheckResponse_NOT_SERVING {
		t.Errorf("status after Stop = %v, want NOT_SERVING", resp.GetStatus())
	}
}

func TestIsProxied(t *testing.T) {
	for _, tc := range []struct {
		method string
		want   bool
	}{
		{"/google.bigtable.v2.Bigtable/ReadRows", true},
		{"/google.bigtable.v2.Bigtable/MutateRow", true},
		{"/grpc.health.v1.Health/Check", false},
		{"/grpc.health.v1.Health/Watch", false},
		{"/grpc.reflection.v1.ServerReflection/ServerReflectionInfo", false},
		// The trailing slash in the prefix is load-bearing: a different service
		// whose name merely starts with "Bigtable" is not ours to forward.
		{"/google.bigtable.v2.BigtableAdmin/CreateTable", false},
		// Admin lives under a different package entirely and is not proxied.
		{"/google.bigtable.admin.v2.BigtableTableAdmin/CreateTable", false},
		{"", false},
		{"/", false},
		{"/google.bigtable.v2.Bigtable", false},
	} {
		if got := isProxied(tc.method); got != tc.want {
			t.Errorf("isProxied(%q) = %v, want %v", tc.method, got, tc.want)
		}
	}
}

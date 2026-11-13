// Copyright 2026 Google LLC
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package transport

import (
	"context"
	"net"
	"testing"

	"google.golang.org/api/option"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func TestStripEndpointScheme(t *testing.T) {
	cases := []struct {
		in   string
		want string
	}{
		{"localhost:9010", "localhost:9010"},
		{"http://localhost:9010", "localhost:9010"},
		{"https://spanner.googleapis.com:443", "spanner.googleapis.com:443"},
		{"passthrough:///localhost:9010", "localhost:9010"},
		{"passthrough:///http://localhost:9010", "localhost:9010"},
		{"passthrough:///https://spanner.googleapis.com:443", "spanner.googleapis.com:443"},
		{"http://passthrough:///localhost:9010", "localhost:9010"},
	}
	for _, tc := range cases {
		if got := StripEndpointScheme(tc.in); got != tc.want {
			t.Errorf("StripEndpointScheme(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}

func TestResolveDirectPathAndFallbackEnv(t *testing.T) {
	t.Setenv("GOOGLE_SPANNER_ENABLE_DIRECT_ACCESS", "")
	if ResolveDirectPathEnabled(false) {
		t.Fatal("expected DirectPath=false when env unset and config=false")
	}
	if !ResolveDirectPathEnabled(true) {
		t.Fatal("expected DirectPath=true when env unset and config=true")
	}
	t.Setenv("GOOGLE_SPANNER_ENABLE_DIRECT_ACCESS", "true")
	if !ResolveDirectPathEnabled(false) {
		t.Fatal("expected GOOGLE_SPANNER_ENABLE_DIRECT_ACCESS=true to override config=false")
	}
	t.Setenv("GOOGLE_SPANNER_ENABLE_DIRECT_ACCESS", "invalid-bool")
	if ResolveDirectPathEnabled(true) {
		t.Fatal("expected invalid GOOGLE_SPANNER_ENABLE_DIRECT_ACCESS to evaluate to false via strconv.ParseBool")
	}

	t.Setenv("GOOGLE_SPANNER_ENABLE_GCP_FALLBACK", "")
	if ResolveGCPFallbackEnabled() {
		t.Fatal("expected ResolveGCPFallbackEnabled=false by default")
	}
	t.Setenv("GOOGLE_SPANNER_ENABLE_GCP_FALLBACK", "true")
	if !ResolveGCPFallbackEnabled() {
		t.Fatal("expected ResolveGCPFallbackEnabled=true when env=true")
	}
}

func TestDialStaticPoolAndNewGAPICClient(t *testing.T) {
	lis, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("failed to listen: %v", err)
	}
	srv := grpc.NewServer()
	go func() { _ = srv.Serve(lis) }()
	defer func() {
		srv.Stop()
		_ = lis.Close()
	}()

	emuOpts := EmulatorClientOptions(lis.Addr().String())
	if _, _, err := DialStaticPool(
		context.Background(),
		PoolConfig{
			NumChannels:          4,
			HasNumChannelsConfig: true,
		},
		append(emuOpts, option.WithGRPCConnectionPool(2))...,
	); status.Code(err) != codes.InvalidArgument {
		t.Fatalf("expected InvalidArgument on conflicting pool size, got %v", err)
	}

	pool, _, err := DialStaticPool(
		context.Background(),
		PoolConfig{
			NumChannels:          2,
			HasNumChannelsConfig: true,
			Compression:          "gzip",
		},
		emuOpts...,
	)
	if err != nil {
		t.Fatalf("DialStaticPool failed: %v", err)
	}
	defer pool.Close()
	if pool.Num() != 2 {
		t.Fatalf("pool.Num() = %d, want 2", pool.Num())
	}

	raw, err := NewGAPICClient(context.Background(), "custom-agent/1.0", nil, option.WithGRPCConn(pool.Conn()))
	if err != nil {
		t.Fatalf("NewGAPICClient failed: %v", err)
	}
	_ = raw.Close()
}

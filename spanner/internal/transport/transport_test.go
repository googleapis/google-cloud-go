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
	"errors"
	"net"
	"testing"

	vkit "cloud.google.com/go/spanner/apiv1"
	"github.com/googleapis/gax-go/v2"
	"google.golang.org/api/option"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func TestStripEndpointSchemeAndEmulatorClientOptions(t *testing.T) {
	cases := []struct {
		in   string
		want string
	}{
		{"localhost:9010", "localhost:9010"},
		{"http://localhost:9010", "localhost:9010"},
		{"https://spanner.googleapis.com:443", "spanner.googleapis.com:443"},
		{"passthrough:///localhost:9010", "localhost:9010"},
		// Single-prefix behavior matching historical regexp ^(http://|https://|passthrough:///).
		{"passthrough:///http://localhost:9010", "http://localhost:9010"},
		{"http://https://spanner.googleapis.com:443", "https://spanner.googleapis.com:443"},
		{"http://", ""},
		{"", ""},
	}
	for _, tc := range cases {
		if got := stripEndpointScheme(tc.in); got != tc.want {
			t.Errorf("stripEndpointScheme(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}

	// Even when scheme stripping leaves an empty address (e.g. SPANNER_EMULATOR_HOST="http://"),
	// EmulatorClientOptions must still include WithEndpoint("passthrough:///") so NewClient
	// never falls back to dialing production Spanner in plaintext without auth.
	for _, raw := range []string{"http://", ""} {
		if got := len(EmulatorClientOptions(raw)); got != 4 {
			t.Fatalf("EmulatorClientOptions(%q) len = %d, want 4 (including WithEndpoint)", raw, got)
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

func TestDefaultClientOptionsDoesNotMutateCallerSlice(t *testing.T) {
	caller := make([]option.ClientOption, 0, 8)
	caller = append(caller, EmulatorClientOptions("127.0.0.1:9010")...)
	sentinel := option.WithUserAgent("sentinel")
	callerFull := caller[:cap(caller)]
	for i := len(caller); i < len(callerFull); i++ {
		callerFull[i] = sentinel
	}

	_ = DefaultClientOptions(4, "gzip", false, nil, nil, caller...)
	for i := len(caller); i < len(callerFull); i++ {
		if callerFull[i] != sentinel {
			t.Fatalf("DefaultClientOptions mutated caller backing array at spare index %d", i)
		}
	}
}

func TestDialStaticPoolFallbackPoolAndNewGAPICClient(t *testing.T) {
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
	} else {
		var mismatch *PoolSizeMismatchError
		if !errors.As(err, &mismatch) {
			t.Fatalf("expected *PoolSizeMismatchError, got %T (%v)", err, err)
		}
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

	customGRPCOpt := gax.WithGRPCOptions(grpc.MaxCallRecvMsgSize(8192))
	raw, err := NewGAPICClient(
		context.Background(),
		"custom-agent/1.0",
		&vkit.CallOptions{ExecuteSql: []gax.CallOption{customGRPCOpt}},
		option.WithGRPCConn(pool.Conn()),
	)
	if err != nil {
		t.Fatalf("NewGAPICClient failed: %v", err)
	}
	if raw.Connection() == nil {
		t.Fatal("expected non-nil raw.Connection()")
	}
	var cs gax.CallSettings
	for _, opt := range raw.CallOptions.ExecuteSql {
		opt.Resolve(&cs)
	}
	if len(cs.GRPC) != 1 {
		t.Fatalf("expected merged ExecuteSql GRPC options len=1, got %d", len(cs.GRPC))
	}
	_ = raw.Close()

	// CreateFallbackConnPool: mismatch error and happy path.
	if _, _, err := CreateFallbackConnPool(
		context.Background(),
		PoolConfig{
			NumChannels:          4,
			HasNumChannelsConfig: true,
		},
		append(emuOpts, option.WithGRPCConnectionPool(2))...,
	); status.Code(err) != codes.InvalidArgument {
		t.Fatalf("expected InvalidArgument on fallback pool mismatch, got %v", err)
	} else {
		var mismatch *PoolSizeMismatchError
		if !errors.As(err, &mismatch) {
			t.Fatalf("expected *PoolSizeMismatchError from CreateFallbackConnPool, got %T (%v)", err, err)
		}
	}

	fbPool, _, err := CreateFallbackConnPool(
		context.Background(),
		PoolConfig{
			NumChannels:          2,
			HasNumChannelsConfig: true,
		},
		emuOpts...,
	)
	if err != nil {
		t.Fatalf("CreateFallbackConnPool failed: %v", err)
	}
	if fbPool.Conn() != nil {
		t.Fatal("expected FallbackPool.Conn() == nil")
	}
	if fbPool.Num() != 2 {
		t.Fatalf("FallbackPool.Num() = %d, want 2", fbPool.Num())
	}
	if err := fbPool.Close(); err != nil {
		t.Fatalf("FallbackPool.Close() failed: %v", err)
	}

	// MergeCallOptions keeps unset fields nil.
	emptyMerged := MergeCallOptions(&vkit.CallOptions{}, &vkit.CallOptions{})
	if emptyMerged.ExecuteSql != nil {
		t.Fatalf("expected MergeCallOptions on nil slices to stay nil, got %#v", emptyMerged.ExecuteSql)
	}
}

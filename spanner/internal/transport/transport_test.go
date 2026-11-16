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
	"strings"
	"sync"
	"sync/atomic"
	"testing"

	vkit "cloud.google.com/go/spanner/apiv1"
	"cloud.google.com/go/spanner/apiv1/spannerpb"
	"cloud.google.com/go/spanner/internal/requestid"
	"github.com/googleapis/gax-go/v2"
	"google.golang.org/api/option"
	gtransport "google.golang.org/api/transport/grpc"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/encoding/gzip"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/stats"
	"google.golang.org/grpc/status"
)

type headerCapturingSpannerServer struct {
	spannerpb.UnimplementedSpannerServer
	mu           sync.Mutex
	lastMD       metadata.MD
	lastUA       string
	lastCompress string
}

func (s *headerCapturingSpannerServer) TagRPC(ctx context.Context, _ *stats.RPCTagInfo) context.Context {
	return ctx
}
func (s *headerCapturingSpannerServer) HandleRPC(_ context.Context, rs stats.RPCStats) {
	if inHdr, ok := rs.(*stats.InHeader); ok {
		s.mu.Lock()
		s.lastCompress = inHdr.Compression
		s.mu.Unlock()
	}
}
func (s *headerCapturingSpannerServer) TagConn(ctx context.Context, _ *stats.ConnTagInfo) context.Context {
	return ctx
}
func (s *headerCapturingSpannerServer) HandleConn(context.Context, stats.ConnStats) {}

func (s *headerCapturingSpannerServer) ExecuteSql(ctx context.Context, _ *spannerpb.ExecuteSqlRequest) (*spannerpb.ResultSet, error) {
	s.mu.Lock()
	if md, ok := metadata.FromIncomingContext(ctx); ok {
		s.lastMD = md.Copy()
		if uas := md.Get("user-agent"); len(uas) > 0 {
			s.lastUA = uas[0]
		}
	}
	s.mu.Unlock()
	return &spannerpb.ResultSet{}, nil
}

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

	// Prepending DefaultClientOptions supplies GAPIC's default spanner.googleapis.com:443
	// endpoint; EmulatorClientOptions("http://") and ("") must still override it with
	// "passthrough:///" and fail locally in the passthrough resolver.
	for _, raw := range []string{"http://", ""} {
		opts := DefaultClientOptions(1, "", false, nil, nil, EmulatorClientOptions(raw)...)
		pool, err := gtransport.DialPool(context.Background(), opts...)
		if err != nil {
			if !strings.Contains(err.Error(), "passthrough") {
				t.Fatalf("DialPool(%q) failed with non-passthrough error: %v", raw, err)
			}
			continue
		}
		conn := pool.Conn()
		target := ""
		if conn != nil {
			target = conn.Target()
		}
		_ = pool.Close()
		if conn == nil || target != "passthrough:///" {
			t.Fatalf("DialPool(%q) bound target %q, want passthrough:///", raw, target)
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

func TestDefaultClientOptionsPrecedenceAndNonMutation(t *testing.T) {
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

	// Verify runtime precedence: userOpts override DefaultClientOptions' numChannels and
	// user-agent, while compression=="gzip" is appended after userOpts and overrides them.
	lis, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("failed to listen: %v", err)
	}
	capturer := &headerCapturingSpannerServer{}
	srv := grpc.NewServer(grpc.StatsHandler(capturer))
	spannerpb.RegisterSpannerServer(srv, capturer)
	go func() { _ = srv.Serve(lis) }()
	defer func() {
		srv.Stop()
		_ = lis.Close()
	}()

	userOpts := append(
		EmulatorClientOptions(lis.Addr().String()),
		option.WithGRPCConnectionPool(3),
		option.WithUserAgent("caller-override-ua/9.9"),
		option.WithGRPCDialOption(grpc.WithDefaultCallOptions(grpc.UseCompressor("identity"))),
	)
	opts := DefaultClientOptions(1, gzip.Name, false, nil, nil, userOpts...)
	pool, err := gtransport.DialPool(context.Background(), opts...)
	if err != nil {
		t.Fatalf("DialPool failed: %v", err)
	}
	defer pool.Close()
	if pool.Num() != 3 {
		t.Fatalf("expected userOpts WithGRPCConnectionPool(3) to override default 1, got %d", pool.Num())
	}
	raw, err := NewGAPICClient(context.Background(), "", nil, gtransport.WithConnPool(pool))
	if err != nil {
		t.Fatalf("NewGAPICClient failed: %v", err)
	}
	defer raw.Close()
	if _, err := raw.ExecuteSql(context.Background(), &spannerpb.ExecuteSqlRequest{Sql: "SELECT 1"}); err != nil {
		t.Fatalf("ExecuteSql failed: %v", err)
	}
	capturer.mu.Lock()
	gotUA := capturer.lastUA
	gotCompress := capturer.lastCompress
	capturer.mu.Unlock()
	if !strings.Contains(gotUA, "caller-override-ua/9.9") {
		t.Fatalf("expected userOpts user-agent override in %q", gotUA)
	}
	if gotCompress != gzip.Name {
		t.Fatalf("expected gzip compressor to override userOpts identity compressor, got %q", gotCompress)
	}
}

func TestDialStaticPoolFallbackPoolAndRequestIDInjection(t *testing.T) {
	lis, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("failed to listen: %v", err)
	}
	capturer := &headerCapturingSpannerServer{}
	srv := grpc.NewServer(grpc.StatsHandler(capturer))
	spannerpb.RegisterSpannerServer(srv, capturer)
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
		gtransport.WithConnPool(pool),
	)
	if err != nil {
		t.Fatalf("NewGAPICClient failed: %v", err)
	}
	var cs gax.CallSettings
	for _, opt := range raw.CallOptions.ExecuteSql {
		opt.Resolve(&cs)
	}
	if len(cs.GRPC) != 1 {
		t.Fatalf("expected merged ExecuteSql GRPC options len=1, got %d", len(cs.GRPC))
	}

	// Verify DialStaticPool's interceptor injects x-goog-spanner-request-id onto outgoing RPCs.
	var nth atomic.Uint32
	tr := requestid.NewTracker(5, 1, &nth)
	if _, err := raw.ExecuteSql(context.Background(), &spannerpb.ExecuteSqlRequest{Sql: "SELECT 1"}, tr.OptsWithNextRequestID(nil)...); err != nil {
		t.Fatalf("static pool ExecuteSql failed: %v", err)
	}
	capturer.mu.Lock()
	staticIDs := capturer.lastMD.Get(requestid.Header)
	capturer.lastMD = nil
	capturer.mu.Unlock()
	if len(staticIDs) != 1 || !strings.HasSuffix(staticIDs[0], ".5.1.1.1") {
		t.Fatalf("expected injected static-pool request ID ending in .5.1.1.1, got %v", staticIDs)
	}
	_ = raw.Close()

	// CreateFallbackConnPool: mismatch error, happy path, and request-ID injection.
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
	fbRaw, err := NewGAPICClient(context.Background(), "", nil, gtransport.WithConnPool(fbPool))
	if err != nil {
		t.Fatalf("NewGAPICClient(fbPool) failed: %v", err)
	}
	if _, err := fbRaw.ExecuteSql(context.Background(), &spannerpb.ExecuteSqlRequest{Sql: "SELECT 1"}, tr.OptsWithNextRequestID(nil)...); err != nil {
		t.Fatalf("fallback pool ExecuteSql failed: %v", err)
	}
	capturer.mu.Lock()
	fbIDs := capturer.lastMD.Get(requestid.Header)
	capturer.mu.Unlock()
	if len(fbIDs) != 1 || !strings.HasSuffix(fbIDs[0], ".5.1.2.1") {
		t.Fatalf("expected injected fallback-pool request ID ending in .5.1.2.1, got %v", fbIDs)
	}
	// fbRaw wraps fbPool via gtransport.WithConnPool(fbPool), so fbRaw.Close()
	// delegates to FallbackPool.Close().
	if err := fbRaw.Close(); err != nil {
		t.Fatalf("fbRaw.Close() (delegating to FallbackPool.Close()) failed: %v", err)
	}

	// MergeCallOptions keeps unset fields nil and gives b precedence over a.
	aOpts := &vkit.CallOptions{CreateSession: []gax.CallOption{gax.WithGRPCOptions(grpc.MaxCallRecvMsgSize(1024))}}
	bOpts := &vkit.CallOptions{CreateSession: []gax.CallOption{gax.WithGRPCOptions(grpc.MaxCallRecvMsgSize(2048))}}
	merged := MergeCallOptions(aOpts, bOpts)
	if merged.ExecuteSql != nil {
		t.Fatalf("expected MergeCallOptions on nil slices to stay nil, got %#v", merged.ExecuteSql)
	}
	var mergedCS gax.CallSettings
	for _, o := range merged.CreateSession {
		o.Resolve(&mergedCS)
	}
	if len(mergedCS.GRPC) != 1 || mergedCS.GRPC[0] != grpc.MaxCallRecvMsgSize(2048) {
		t.Fatalf("expected b to override a in MergeCallOptions, got %#v", mergedCS.GRPC)
	}
}

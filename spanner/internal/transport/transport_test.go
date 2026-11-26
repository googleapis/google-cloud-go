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
	"fmt"
	"io"
	"net"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"cloud.google.com/go/spanner/apiv1/spannerpb"
	"cloud.google.com/go/spanner/internal/requestid"
	"go.opentelemetry.io/otel/metric"
	"go.opentelemetry.io/otel/metric/noop"
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

func startTestSpannerServer(t *testing.T, impl spannerpb.SpannerServer, opts ...grpc.ServerOption) string {
	t.Helper()
	lis, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("net.Listen failed: %v", err)
	}
	srv := grpc.NewServer(opts...)
	spannerpb.RegisterSpannerServer(srv, impl)
	go func() { _ = srv.Serve(lis) }()
	t.Cleanup(func() {
		srv.Stop()
		_ = lis.Close()
	})
	return lis.Addr().String()
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
		if got := StripEndpointScheme(tc.in); got != tc.want {
			t.Errorf("StripEndpointScheme(%q) = %q, want %q", tc.in, got, tc.want)
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

func directPathOptionNames(opts []option.ClientOption) []string {
	var names []string
	for _, o := range opts {
		switch name := fmt.Sprintf("%T", o); name {
		case "internaloption.enableDirectPath",
			"internaloption.enableDirectPathXds",
			"internaloption.allowHardBoundTokens",
			"internaloption.allowNonDefaultServiceAccount":
			names = append(names, name)
		}
	}
	return names
}

func TestCloudPathClientOptionsHaveNoDirectPathOptions(t *testing.T) {
	t.Setenv("GOOGLE_SPANNER_DISABLE_DIRECT_ACCESS_BOUND_TOKEN", "")
	for _, tc := range []struct {
		env        string
		configured bool
	}{
		{env: "", configured: true},
		{env: "true", configured: false},
		{env: "true", configured: true},
	} {
		t.Setenv("GOOGLE_SPANNER_ENABLE_DIRECT_ACCESS", tc.env)
		cfg := PoolConfig{NumChannels: 1, Compression: gzip.Name, DirectPathMode: ResolveDirectPathMode(tc.configured)}
		userOpts := []option.ClientOption{option.WithUserAgent("caller-ua/1.0")}
		direct := cfg.ClientOptions(userOpts...)
		if got := directPathOptionNames(direct); len(got) != 4 {
			t.Fatalf("env %q, configured %v: DirectPath options of ClientOptions = %v, want 4", tc.env, tc.configured, got)
		}
		cloud := cfg.CloudPathClientOptions(userOpts...)
		if got := directPathOptionNames(cloud); len(got) != 0 {
			t.Fatalf("env %q, configured %v: DirectPath options of CloudPathClientOptions = %v, want none", tc.env, tc.configured, got)
		}
		if got, want := len(cloud), len(direct)-4; got != want {
			t.Fatalf("env %q, configured %v: CloudPathClientOptions has %d options, want the %d other options of ClientOptions", tc.env, tc.configured, got, want)
		}
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
	capturer := &headerCapturingSpannerServer{}
	addr := startTestSpannerServer(t, capturer, grpc.StatsHandler(capturer))

	userOpts := append(
		EmulatorClientOptions(addr),
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
	raw, err := NewGAPICClient(context.Background(), "", gtransport.WithConnPool(pool))
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
	capturer := &headerCapturingSpannerServer{}
	emuOpts := EmulatorClientOptions(startTestSpannerServer(t, capturer, grpc.StatsHandler(capturer)))
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

	raw, err := NewGAPICClient(
		context.Background(),
		"custom-agent/1.0",
		gtransport.WithConnPool(pool),
	)
	if err != nil {
		t.Fatalf("NewGAPICClient failed: %v", err)
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
	fbRaw, err := NewGAPICClient(context.Background(), "", gtransport.WithConnPool(fbPool))
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
}

func TestPlainTextClientOptions(t *testing.T) {
	t.Parallel()
	addr := startTestSpannerServer(t, &headerCapturingSpannerServer{})

	combined := append([]option.ClientOption{option.WithEndpoint("passthrough:///" + addr)}, PlainTextClientOptions()...)
	pool, err := gtransport.DialPool(context.Background(), DefaultClientOptions(1, "", false, nil, nil, combined...)...)
	if err != nil {
		t.Fatalf("DialPool failed: %v", err)
	}
	defer pool.Close()
	if got := pool.Conn().Target(); got != "passthrough:///"+addr {
		t.Fatalf("conn target = %q, want %q", got, "passthrough:///"+addr)
	}
	raw, err := NewGAPICClient(context.Background(), "", gtransport.WithConnPool(pool))
	if err != nil {
		t.Fatalf("NewGAPICClient failed: %v", err)
	}
	defer raw.Close()
	if _, err := raw.ExecuteSql(context.Background(), &spannerpb.ExecuteSqlRequest{Sql: "SELECT 1"}); err != nil {
		t.Fatalf("ExecuteSql failed: %v", err)
	}
}

func TestEmulatorClientOptions(t *testing.T) {
	t.Parallel()
	addr := startTestSpannerServer(t, &headerCapturingSpannerServer{})

	opts := EmulatorClientOptions("http://" + addr)
	pool, err := gtransport.DialPool(context.Background(), DefaultClientOptions(1, "", false, nil, nil, opts...)...)
	if err != nil {
		t.Fatalf("DialPool failed: %v", err)
	}
	defer pool.Close()
	if got := pool.Conn().Target(); got != "passthrough:///"+addr {
		t.Fatalf("conn target = %q, want %q", got, "passthrough:///"+addr)
	}
}

func TestPoolConfig_DirectPathMode(t *testing.T) {
	for _, tc := range []struct {
		mode DirectPathMode
		want string
	}{
		{DirectPathDefault, "DirectPathDefault"},
		{DirectPathEnabled, "DirectPathEnabled"},
		{DirectPathDisabled, "DirectPathDisabled"},
		{DirectPathMode(99), "DirectPathMode(99)"},
	} {
		if got := tc.mode.String(); got != tc.want {
			t.Fatalf("DirectPathMode(%d).String() = %q, want %q", int(tc.mode), got, tc.want)
		}
	}

	hasDirectPathOpt := func(opts []option.ClientOption) bool {
		for _, o := range opts {
			desc := fmt.Sprintf("%T", o)
			if strings.Contains(desc, "DirectPath") || strings.Contains(desc, "HardBound") || strings.Contains(desc, "NonDefaultServiceAccount") {
				return true
			}
		}
		return false
	}

	cases := []struct {
		name   string
		mode   DirectPathMode
		envVal string
		want   bool
	}{
		{"Default/env=unset", DirectPathDefault, "", false},
		{"Default/env=true", DirectPathDefault, "true", true},
		{"Enabled/env=false", DirectPathEnabled, "false", true},
		{"Enabled/env=true", DirectPathEnabled, "true", true},
		{"Disabled/env=false", DirectPathDisabled, "false", false},
		{"Disabled/env=true", DirectPathDisabled, "true", false},
		{"Unknown/env=true", DirectPathMode(99), "true", false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv("GOOGLE_SPANNER_ENABLE_DIRECT_ACCESS", tc.envVal)
			opts := (PoolConfig{NumChannels: 1, DirectPathMode: tc.mode}).ClientOptions()
			if got := hasDirectPathOpt(opts); got != tc.want {
				t.Fatalf("DirectPathMode=%v env=%q: hasDirectPathOpt=%v, want %v", tc.mode, tc.envVal, got, tc.want)
			}
		})
	}

	resolveCases := []struct {
		name   string
		cfg    bool
		envVal string
		want   DirectPathMode
	}{
		{"cfg=false/env=unset", false, "", DirectPathDisabled},
		{"cfg=true/env=unset", true, "", DirectPathEnabled},
		{"cfg=false/env=true", false, "true", DirectPathEnabled},
		{"cfg=true/env=false", true, "false", DirectPathDisabled},
	}
	for _, tc := range resolveCases {
		t.Run("ResolveDirectPathMode/"+tc.name, func(t *testing.T) {
			t.Setenv("GOOGLE_SPANNER_ENABLE_DIRECT_ACCESS", tc.envVal)
			if got := ResolveDirectPathMode(tc.cfg); got != tc.want {
				t.Fatalf("ResolveDirectPathMode(%v) env=%q = %v, want %v", tc.cfg, tc.envVal, got, tc.want)
			}
		})
	}
}

type streamingHeaderServer struct {
	spannerpb.UnimplementedSpannerServer
}

func (s *streamingHeaderServer) ExecuteSql(context.Context, *spannerpb.ExecuteSqlRequest) (*spannerpb.ResultSet, error) {
	return &spannerpb.ResultSet{}, nil
}

func (s *streamingHeaderServer) ExecuteStreamingSql(req *spannerpb.ExecuteSqlRequest, stream spannerpb.Spanner_ExecuteStreamingSqlServer) error {
	if err := grpc.SendHeader(stream.Context(), metadata.Pairs("server-timing", "gfet4t7; dur=1.5")); err != nil {
		return err
	}
	return stream.Send(&spannerpb.PartialResultSet{Last: true})
}

func TestExecuteStreamingSQL_CachesHeaderAndPropagatesError(t *testing.T) {
	t.Parallel()
	addr := startTestSpannerServer(t, &streamingHeaderServer{})

	pool, err := gtransport.DialPool(context.Background(), DefaultClientOptions(1, "", false, nil, nil, EmulatorClientOptions(addr)...)...)
	if err != nil {
		t.Fatalf("DialPool failed: %v", err)
	}
	defer pool.Close()

	raw, err := NewGAPICClient(context.Background(), "", gtransport.WithConnPool(pool))
	if err != nil {
		t.Fatalf("NewGAPICClient failed: %v", err)
	}
	defer raw.Close()

	stream, err := ExecuteStreamingSQL(context.Background(), raw, &spannerpb.ExecuteSqlRequest{Sql: "SELECT 1"})
	if err != nil {
		t.Fatalf("ExecuteStreamingSQL failed: %v", err)
	}

	md1, err := stream.Header()
	if err != nil {
		t.Fatalf("first Header() failed: %v", err)
	}
	md2, err := stream.Header()
	if err != nil {
		t.Fatalf("second Header() failed: %v", err)
	}
	if w, g := []string{"gfet4t7; dur=1.5"}, md1.Get("server-timing"); len(g) != 1 || g[0] != w[0] || len(md2.Get("server-timing")) != 1 || md2.Get("server-timing")[0] != w[0] {
		t.Fatalf("server-timing header mismatch:\n Want: %v\n  Got: md1=%v md2=%v", w, md1.Get("server-timing"), md2.Get("server-timing"))
	}
	if _, err := stream.Recv(); err != nil {
		t.Fatalf("Recv() failed: %v", err)
	}
	if _, err := stream.Recv(); err != io.EOF {
		t.Fatalf("drain Recv() mismatch:\n Want: %v\n  Got: %v", io.EOF, err)
	}

	canceledCtx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := ExecuteStreamingSQL(canceledCtx, raw, &spannerpb.ExecuteSqlRequest{Sql: "SELECT 1"}); err == nil {
		t.Fatal("expected error from ExecuteStreamingSQL with canceled context")
	}
}

func TestDefaultClientOptions_InterceptorsAndPoolSizeMismatchError(t *testing.T) {
	t.Parallel()
	addr := startTestSpannerServer(t, &streamingHeaderServer{})

	var unaryCalls, streamCalls atomic.Int32
	unaryInterceptor := func(ctx context.Context, method string, req, reply any, cc *grpc.ClientConn, invoker grpc.UnaryInvoker, opts ...grpc.CallOption) error {
		unaryCalls.Add(1)
		return invoker(ctx, method, req, reply, cc, opts...)
	}
	streamInterceptor := func(ctx context.Context, desc *grpc.StreamDesc, cc *grpc.ClientConn, method string, streamer grpc.Streamer, opts ...grpc.CallOption) (grpc.ClientStream, error) {
		streamCalls.Add(1)
		return streamer(ctx, desc, cc, method, opts...)
	}

	opts := DefaultClientOptions(
		1,
		"",
		false,
		[]grpc.UnaryClientInterceptor{unaryInterceptor},
		[]grpc.StreamClientInterceptor{streamInterceptor},
		EmulatorClientOptions(addr)...,
	)
	pool, err := gtransport.DialPool(context.Background(), opts...)
	if err != nil {
		t.Fatalf("DialPool failed: %v", err)
	}
	defer pool.Close()

	raw, err := NewGAPICClient(context.Background(), "", gtransport.WithConnPool(pool))
	if err != nil {
		t.Fatalf("NewGAPICClient failed: %v", err)
	}
	defer raw.Close()

	if _, err := raw.ExecuteSql(context.Background(), &spannerpb.ExecuteSqlRequest{Sql: "SELECT 1"}); err != nil {
		t.Fatalf("ExecuteSql failed: %v", err)
	}
	st, err := raw.ExecuteStreamingSql(context.Background(), &spannerpb.ExecuteSqlRequest{Sql: "SELECT 1"})
	if err != nil {
		t.Fatalf("ExecuteStreamingSql failed: %v", err)
	}
	if _, err := st.Recv(); err != nil {
		t.Fatalf("Recv failed: %v", err)
	}
	if _, err := st.Recv(); err != io.EOF {
		t.Fatalf("drain Recv mismatch:\n Want: %v\n  Got: %v", io.EOF, err)
	}

	if w, g := int32(1), unaryCalls.Load(); w != g {
		t.Fatalf("unary interceptor count mismatch:\n Want: %d\n  Got: %d", w, g)
	}
	if w, g := int32(1), streamCalls.Load(); w != g {
		t.Fatalf("stream interceptor count mismatch:\n Want: %d\n  Got: %d", w, g)
	}

	mismatchErr := &PoolSizeMismatchError{Msg: "pool size mismatch"}
	if w, g := "pool size mismatch", mismatchErr.Error(); w != g {
		t.Fatalf("PoolSizeMismatchError.Error mismatch:\n Want: %q\n  Got: %q", w, g)
	}
}

type fallbackRateCheckSpy struct {
	noop.MeterProvider
	afterCancel      atomic.Bool
	postCancelCtxErr chan error
}

type fallbackSpyMeter struct {
	noop.Meter
	spy *fallbackRateCheckSpy
}

type fallbackSpyErrorRatioGauge struct {
	noop.Float64Gauge
	spy *fallbackRateCheckSpy
}

func (p *fallbackRateCheckSpy) Meter(string, ...metric.MeterOption) metric.Meter {
	return &fallbackSpyMeter{Meter: noop.Meter{}, spy: p}
}

func (m *fallbackSpyMeter) Float64Gauge(name string, opts ...metric.Float64GaugeOption) (metric.Float64Gauge, error) {
	if name != "eef.error_ratio" {
		return m.Meter.Float64Gauge(name, opts...)
	}
	return &fallbackSpyErrorRatioGauge{Float64Gauge: noop.Float64Gauge{}, spy: m.spy}, nil
}

func (g *fallbackSpyErrorRatioGauge) Record(ctx context.Context, val float64, opts ...metric.RecordOption) {
	if g.spy.afterCancel.Load() {
		select {
		case g.spy.postCancelCtxErr <- ctx.Err():
		default:
		}
	}
}

func TestCreateFallbackConnPool_StartupContextOnly(t *testing.T) {
	t.Parallel()
	emuOpts := EmulatorClientOptions(startTestSpannerServer(t, &headerCapturingSpannerServer{}))

	// Non-blocking dial on a pre-canceled context preserves DialStaticPool / spanner.NewClient parity.
	preCanceled, cancelPre := context.WithCancel(context.Background())
	cancelPre()
	prePool, _, err := CreateFallbackConnPool(preCanceled, PoolConfig{NumChannels: 1, DirectPathMode: DirectPathDisabled}, emuOpts...)
	if err != nil {
		t.Fatalf("non-blocking CreateFallbackConnPool(preCanceled) failed: %v", err)
	}
	_ = prePool.Close()

	// Canceling startup context after construction must NOT stop GCPFallback's background rateCheck loop.
	spy := &fallbackRateCheckSpy{postCancelCtxErr: make(chan error, 8)}
	startupCtx, cancelStartup := context.WithCancel(context.Background())
	pool, _, err := createFallbackConnPool(
		startupCtx,
		PoolConfig{
			NumChannels:          2,
			HasNumChannelsConfig: true,
			DirectPathMode:       DirectPathDisabled,
			MeterProvider:        spy,
		},
		2*time.Millisecond,
		emuOpts...,
	)
	if err != nil {
		cancelStartup()
		t.Fatalf("createFallbackConnPool failed: %v", err)
	}
	cancelStartup()
	spy.afterCancel.Store(true)
	defer func() {
		spy.afterCancel.Store(false)
		_ = pool.Close()
	}()

	// Each rateCheck() tick records eef.error_ratio twice (primary and fallback) using gcpFallback.ctx.
	// Observing 4 post-cancel records (>= 2 full ticks) with ctx.Err() == nil proves rateCheck()
	// remains alive after startupCtx is canceled.
	for i := 0; i < 4; i++ {
		select {
		case recCtxErr := <-spy.postCancelCtxErr:
			if recCtxErr != nil {
				t.Fatalf("post-cancel rateCheck record %d observed canceled fallback ctx: %v", i, recCtxErr)
			}
		case <-time.After(2 * time.Second):
			t.Fatalf("timed out waiting for post-cancel GCPFallback rateCheck tick (record %d/4)", i)
		}
	}
	spy.afterCancel.Store(false)

	fbPool, ok := pool.(*FallbackPool)
	if !ok {
		t.Fatalf("expected *FallbackPool, got %T", pool)
	}
	if fbPool.PrimaryConn() == nil || fbPool.FallbackConn() == nil {
		t.Fatalf("expected non-nil primary and fallback pools")
	}

	raw, err := NewGAPICClient(context.Background(), "", gtransport.WithConnPool(pool))
	if err != nil {
		t.Fatalf("NewGAPICClient failed: %v", err)
	}
	defer raw.Close()
	if _, err := raw.ExecuteSql(context.Background(), &spannerpb.ExecuteSqlRequest{Sql: "SELECT 1"}); err != nil {
		t.Fatalf("ExecuteSql after startup ctx cancel failed: %v", err)
	}
}

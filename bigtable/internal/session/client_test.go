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

package session

import (
	"context"
	"encoding/base64"
	"errors"
	"sync/atomic"
	"testing"

	btpb "cloud.google.com/go/bigtable/apiv2/bigtablepb"
	metrics "cloud.google.com/go/bigtable/internal/metrics"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	"google.golang.org/api/option"
	"google.golang.org/grpc/metadata"
	"google.golang.org/protobuf/proto"
)

// fakeChannelPool satisfies the ChannelPool interface for unit tests:
// counts Close calls and can be configured to return a Close error.
// Kept unexported and out of the shared _test.go pool because the only
// consumer is client_test.go.
type fakeChannelPool struct {
	closed atomic.Int32
	err    error
}

func (f *fakeChannelPool) Close() error {
	f.closed.Add(1)
	return f.err
}

// newTestFactory builds a *metrics.Factory backed by a ManualReader.
// Callers that don't care about the reader can discard it. Kept
// separate from newTestTable's helper (in table_test.go) so this file
// doesn't take a hidden dependency on that fixture's shape.
func newTestFactory(t *testing.T) *metrics.Factory {
	t.Helper()
	reader := sdkmetric.NewManualReader()
	mp := sdkmetric.NewMeterProvider(sdkmetric.WithReader(reader))
	factory, err := metrics.NewFactoryForTest("test-project", "test-instance", "test-profile", mp)
	if err != nil {
		t.Fatalf("metrics.NewFactoryForTest: %v", err)
	}
	return factory
}

// newTestClient wires a *sessionClient with the given ChannelPool +
// Config. Passes a nil stub so no ClientConfigurationManager gets
// started (Start would poll GetClientConfiguration on a real gRPC
// stub — not what unit tests want). A test-only metrics factory is
// injected so MeterProvider / MetricsFactory return non-nil.
func newTestClient(t *testing.T, pool ChannelPool, cfg Config) *sessionClient {
	t.Helper()
	if cfg.Project == "" {
		cfg.Project = "test-project"
	}
	if cfg.Instance == "" {
		cfg.Instance = "test-instance"
	}
	if cfg.AppProfile == "" {
		cfg.AppProfile = "test-profile"
	}
	return newSessionClientFromParts(pool, nil, newTestFactory(t), cfg)
}

// decodeFeatureFlags decodes the value of the bigtable-features
// metadata header (base64(proto.Marshal(FeatureFlags))) back into a
// FeatureFlags struct — mirrors what the server does on the wire.
func decodeFeatureFlags(t *testing.T, md metadata.MD) *btpb.FeatureFlags {
	t.Helper()
	vals := md.Get(featureFlagsHeaderKey)
	if len(vals) != 1 {
		t.Fatalf("md[%q] = %v, want exactly one value", featureFlagsHeaderKey, vals)
	}
	raw, err := base64.URLEncoding.DecodeString(vals[0])
	if err != nil {
		t.Fatalf("base64 decode: %v", err)
	}
	ff := &btpb.FeatureFlags{}
	if err := proto.Unmarshal(raw, ff); err != nil {
		t.Fatalf("proto.Unmarshal FeatureFlags: %v", err)
	}
	return ff
}

// TestBuildFeatureFlagsMD_AlwaysOnBits pins the invariant flags that
// every session client (mixed-mode or standalone) MUST advertise. If a
// server-visible flag needs to move, break this test on purpose so
// reviewers notice.
func TestBuildFeatureFlagsMD_AlwaysOnBits(t *testing.T) {
	md := buildFeatureFlagsMD(false, false, false)
	ff := decodeFeatureFlags(t, md)
	if !ff.RoutingCookie || !ff.ReverseScans || !ff.LastScannedRowResponses || !ff.SessionsCompatible || !ff.PeerInfo {
		t.Errorf("always-on flags missing: %+v", ff)
	}
}

// TestBuildFeatureFlagsMD_ReflectsToggles walks the three toggle inputs
// and asserts each maps to the right proto field, including RetryInfo's
// inversion (input=disableRetryInfo, wire=RetryInfo).
func TestBuildFeatureFlagsMD_ReflectsToggles(t *testing.T) {
	tests := []struct {
		name             string
		metricsEnabled   bool
		disableRetryInfo bool
		directAccess     bool
		wantMetrics      bool
		wantRetryInfo    bool
		wantDirect       bool
	}{
		{"all-off", false, false, false, false, true /* !disable */, false},
		{"all-on", true, true, true, true, false /* !disable */, true},
		{"metrics-only", true, false, false, true, true, false},
		{"direct-only", false, false, true, false, true, true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			ff := decodeFeatureFlags(t, buildFeatureFlagsMD(tc.metricsEnabled, tc.disableRetryInfo, tc.directAccess))
			if ff.ClientSideMetricsEnabled != tc.wantMetrics {
				t.Errorf("ClientSideMetricsEnabled = %v, want %v", ff.ClientSideMetricsEnabled, tc.wantMetrics)
			}
			if ff.RetryInfo != tc.wantRetryInfo {
				t.Errorf("RetryInfo = %v, want %v (input disableRetryInfo=%v)", ff.RetryInfo, tc.wantRetryInfo, tc.disableRetryInfo)
			}
			if ff.DirectAccessRequested != tc.wantDirect {
				t.Errorf("DirectAccessRequested = %v, want %v", ff.DirectAccessRequested, tc.wantDirect)
			}
			if ff.TrafficDirectorEnabled != tc.wantDirect {
				t.Errorf("TrafficDirectorEnabled = %v, want %v (must track DirectAccessRequested)", ff.TrafficDirectorEnabled, tc.wantDirect)
			}
		})
	}
}

// TestResolveConnPoolSize_FallbackWhenNoOption confirms the fallback
// applies when the caller supplies no gRPC pool sizing hint.
func TestResolveConnPoolSize_FallbackWhenNoOption(t *testing.T) {
	if got := resolveConnPoolSize(nil, 7); got != 7 {
		t.Errorf("resolveConnPoolSize(nil, 7) = %d, want 7", got)
	}
}

// TestResolveConnPoolSize_UsesCallerSize confirms an explicit
// option.WithGRPCConnectionPool overrides the fallback.
func TestResolveConnPoolSize_UsesCallerSize(t *testing.T) {
	opts := []option.ClientOption{option.WithGRPCConnectionPool(4)}
	if got := resolveConnPoolSize(opts, 10); got != 4 {
		t.Errorf("resolveConnPoolSize(WithGRPCConnectionPool(4), 10) = %d, want 4", got)
	}
}

// TestSessionClient_NameFormatters covers the four resource-name
// helpers. Failures here mean routing headers get built wrong and the
// server would 5xx / mis-route.
func TestSessionClient_NameFormatters(t *testing.T) {
	sc := newTestClient(t, nil, Config{Project: "p", Instance: "i", AppProfile: "ap"})
	defer sc.Close()

	if got, want := sc.fullTableName("t"), "projects/p/instances/i/tables/t"; got != want {
		t.Errorf("fullTableName = %q, want %q", got, want)
	}
	if got, want := sc.fullAuthorizedViewName("t", "v"), "projects/p/instances/i/tables/t/authorizedViews/v"; got != want {
		t.Errorf("fullAuthorizedViewName = %q, want %q", got, want)
	}
	if got, want := sc.fullMaterializedViewName("v"), "projects/p/instances/i/materializedViews/v"; got != want {
		t.Errorf("fullMaterializedViewName = %q, want %q", got, want)
	}
	if got, want := sc.fullInstanceName(), "projects/p/instances/i"; got != want {
		t.Errorf("fullInstanceName = %q, want %q", got, want)
	}
}

// TestSessionClient_PerResourceMetadata verifies the per-vRPC routing
// metadata: resource-prefix + request-params (with URL-escaped values +
// app_profile_id) + merged FeatureFlagsMD.
func TestSessionClient_PerResourceMetadata(t *testing.T) {
	ffMD := buildFeatureFlagsMD(true, false, true)
	sc := newTestClient(t, nil, Config{
		Project: "p", Instance: "i", AppProfile: "profile with spaces",
		FeatureFlagsMD: ffMD,
	})
	defer sc.Close()

	md := sc.perResourceMetadata("projects/p/instances/i/tables/t", "table_name", "projects/p/instances/i/tables/t")

	if got := md.Get(resourcePrefixHeader); len(got) != 1 || got[0] != "projects/p/instances/i/tables/t" {
		t.Errorf("%s = %v, want single %q", resourcePrefixHeader, got, "projects/p/instances/i/tables/t")
	}
	params := md.Get(requestParamsHeader)
	if len(params) != 1 {
		t.Fatalf("%s = %v, want exactly one value", requestParamsHeader, params)
	}
	// URL escaping — "/" -> %2F, " " -> "+", so app_profile_id encodes
	// the injected space as "+".
	wantParam := "table_name=projects%2Fp%2Finstances%2Fi%2Ftables%2Ft&app_profile_id=profile+with+spaces"
	if params[0] != wantParam {
		t.Errorf("%s = %q, want %q", requestParamsHeader, params[0], wantParam)
	}
	if len(md.Get(featureFlagsHeaderKey)) != 1 {
		t.Errorf("feature-flags header missing from perResourceMetadata output: %v", md)
	}
}

// TestSessionClient_FeatureFlags asserts featureFlags() (used to build
// each OpenSessionRequest.Flags) reflects the config booleans, with
// RetryInfo inverted from DisableRetryInfo.
func TestSessionClient_FeatureFlags(t *testing.T) {
	sc := newTestClient(t, nil, Config{MetricsEnabled: true, DisableRetryInfo: true})
	defer sc.Close()

	ff := sc.featureFlags()
	if !ff.ClientSideMetricsEnabled {
		t.Error("ClientSideMetricsEnabled = false, want true (MetricsEnabled=true)")
	}
	if ff.RetryInfo {
		t.Error("RetryInfo = true, want false (DisableRetryInfo=true)")
	}
	// Always-on bits — same invariant list as buildFeatureFlagsMD.
	if !ff.RoutingCookie || !ff.ReverseScans || !ff.LastScannedRowResponses ||
		!ff.SessionsCompatible || !ff.PeerInfo || !ff.TrafficDirectorEnabled || !ff.DirectAccessRequested {
		t.Errorf("always-on bits missing: %+v", ff)
	}
}

// TestSessionClient_BuildLazyOpener_NilPayloadReturnsNil pins the
// materialized-view write path: buildLazyOpener with a nil payload
// MUST return a nil opener so newSessionTable receives openWrite=nil
// and MutateRow surfaces ErrWriteNotSupported. If this ever returns a
// non-nil closure, the MV read-only invariant (SESSION_SPEC.md #11)
// silently breaks.
func TestSessionClient_BuildLazyOpener_NilPayloadReturnsNil(t *testing.T) {
	sc := newTestClient(t, nil, Config{})
	defer sc.Close()

	got := sc.buildLazyOpener("projects/p/instances/i/materializedViews/v", nil, nil, nil, poolKey{"mv:v", permissionWrite})
	if got != nil {
		t.Errorf("buildLazyOpener(payload=nil) = non-nil, want nil (MV write side)")
	}
}

// TestSessionClient_MeterProvider_NilFactorySafe covers construction
// with a nil factory: MeterProvider must return nil (not panic).
func TestSessionClient_MeterProvider_NilFactorySafe(t *testing.T) {
	sc := newSessionClientFromParts(nil, nil, nil, Config{})
	if mp := sc.MeterProvider(); mp != nil {
		t.Errorf("MeterProvider() with nil factory = %v, want nil", mp)
	}
	if f := sc.MetricsFactory(); f != nil {
		t.Errorf("MetricsFactory() with nil factory = %v, want nil", f)
	}
}

// TestSessionClient_MetricsFactory_ReturnsInjected verifies the
// accessor returns the exact *metrics.Factory the constructor received.
// Consumers (sessionTable.ensureTracer) rely on identity so tracers
// share a meter provider with the SessionClient.
func TestSessionClient_MetricsFactory_ReturnsInjected(t *testing.T) {
	f := newTestFactory(t)
	sc := newSessionClientFromParts(nil, nil, f, Config{})
	if got := sc.MetricsFactory(); got != f {
		t.Errorf("MetricsFactory identity mismatch: got %p, want %p", got, f)
	}
	if mp := sc.MeterProvider(); mp != f.OtelMeterProvider {
		t.Errorf("MeterProvider identity mismatch: got %v, want %v", mp, f.OtelMeterProvider)
	}
}

// TestSessionClient_AddSessionLoadListener_NilManagerNoop confirms the
// documented fallback: when no ClientConfigurationManager is wired
// (stub nil at construction), the returned unregister thunk is a
// non-nil no-op so callers can call it unconditionally.
func TestSessionClient_AddSessionLoadListener_NilManagerNoop(t *testing.T) {
	sc := newTestClient(t, nil, Config{})
	defer sc.Close()

	unreg := sc.AddSessionLoadListener(func(float64) {})
	if unreg == nil {
		t.Fatal("AddSessionLoadListener returned nil unregister, want non-nil no-op")
	}
	unreg() // must not panic
}

// TestSessionClient_Close_NilSafe pins the "everything nil" happy path
// — channelPool nil, configManager nil, backgroundCancel nil. This is
// the constructor shape newSessionClientFromParts leaves behind when
// callers pass nil channelPool/stub, and Close must tolerate it so
// half-constructed clients can be discarded without panicking.
func TestSessionClient_Close_NilSafe(t *testing.T) {
	sc := newSessionClientFromParts(nil, nil, nil, Config{})
	if err := sc.Close(); err != nil {
		t.Errorf("Close on nil-everything client = %v, want nil", err)
	}
}

// TestSessionClient_Close_CallsChannelPoolAndCancelsCtx verifies the
// documented tear-down order:
//   - the underlying ChannelPool.Close is invoked (exactly once);
//   - backgroundCancel fires so per-pool background loops unwind.
func TestSessionClient_Close_CallsChannelPoolAndCancelsCtx(t *testing.T) {
	pool := &fakeChannelPool{}
	bgCtx, cancel := context.WithCancel(context.Background())
	sc := newTestClient(t, pool, Config{BackgroundCtx: bgCtx})
	sc.backgroundCancel = cancel

	if err := sc.Close(); err != nil {
		t.Errorf("Close = %v, want nil", err)
	}
	if got := pool.closed.Load(); got != 1 {
		t.Errorf("channelPool.Close call count = %d, want 1", got)
	}
	select {
	case <-bgCtx.Done():
	default:
		t.Error("backgroundCtx not cancelled after Close")
	}
}

// TestSessionClient_Close_PropagatesChannelPoolError verifies the
// documented return: Close returns the ChannelPool.Close error
// verbatim so callers can log / metric it. Other tear-down steps must
// still run (they're deferred inside Close via ordering, not defer).
func TestSessionClient_Close_PropagatesChannelPoolError(t *testing.T) {
	sentinel := errors.New("channel pool closed with error")
	pool := &fakeChannelPool{err: sentinel}
	sc := newTestClient(t, pool, Config{})

	if err := sc.Close(); !errors.Is(err, sentinel) {
		t.Errorf("Close = %v, want %v", err, sentinel)
	}
	if got := pool.closed.Load(); got != 1 {
		t.Errorf("channelPool.Close call count = %d, want 1 (must run even when returning error)", got)
	}
}

// TestSessionClient_ChannelPool_ReturnsNilForFake verifies the debug
// accessor's type-assert-or-nil behavior: a non-*BigtableChannelPool
// implementation (our fake, tests) returns nil so channelz doesn't
// panic when the SessionClient wasn't constructed with a real gRPC
// pool.
func TestSessionClient_ChannelPool_ReturnsNilForFake(t *testing.T) {
	pool := &fakeChannelPool{}
	sc := newTestClient(t, pool, Config{})
	defer sc.Close()

	if bp := sc.ChannelPool(); bp != nil {
		t.Errorf("ChannelPool() with fake pool = %v, want nil", bp)
	}
}

// TestSessionClient_OpenMaterializedView_MutateRowNotSupported drives
// OpenMaterializedView end-to-end and asserts the returned
// TableAPI's MutateRow returns ErrWriteNotSupported. This
// complements TestSessionTable_MatView_MutateRowReturnsErrWriteNotSupported
// (table_test.go) — that test constructs sessionTable directly; this
// one exercises client.go's OpenMaterializedView wiring (openWrite=nil
// literal at the call site).
func TestSessionClient_OpenMaterializedView_MutateRowNotSupported(t *testing.T) {
	sc := newTestClient(t, nil, Config{})
	defer sc.Close()

	api := sc.OpenMaterializedView("mv")
	if api == nil {
		t.Fatal("OpenMaterializedView returned nil TableAPI")
	}
	_, err := api.MutateRow(context.Background(), &btpb.SessionMutateRowRequest{
		Key: []byte("k"),
		Mutations: []*btpb.Mutation{{
			Mutation: &btpb.Mutation_SetCell_{SetCell: &btpb.Mutation_SetCell{
				FamilyName: "cf", ColumnQualifier: []byte("q"), Value: []byte("v"),
			}},
		}},
	})
	if !errors.Is(err, ErrWriteNotSupported) {
		t.Errorf("MutateRow on OpenMaterializedView-returned api = %v, want ErrWriteNotSupported", err)
	}
}

// TestSessionClient_OpenTableAndAuthorizedView_ReturnTableAPI
// asserts the two writable opens return a non-nil TableAPI.
// Only identity is checked — driving the pools open would require a
// real bidi stream stub; those paths are covered under
// bigtable/internal/session/table_test.go with fakeInvoker.
func TestSessionClient_OpenTableAndAuthorizedView_ReturnTableAPI(t *testing.T) {
	sc := newTestClient(t, nil, Config{})
	defer sc.Close()

	if api := sc.OpenSessionTable("t"); api == nil {
		t.Error("OpenSessionTable returned nil TableAPI")
	}
	if api := sc.OpenAuthorizedView("t", "v"); api == nil {
		t.Error("OpenAuthorizedView returned nil TableAPI")
	}
}

// TestSessionClient_DebugAccessors verifies the DebugAccess surface
// returns non-nil providers when a factory is wired, and that
// PoolSnapshots / LoadBalancingSnapshots return empty (not nil-panic)
// on a freshly-constructed client that has never opened a pool.
func TestSessionClient_DebugAccessors(t *testing.T) {
	sc := newTestClient(t, &fakeChannelPool{}, Config{})
	defer sc.Close()

	if p := sc.SessionDebug(); p == nil {
		t.Error("SessionDebug() = nil, want non-nil provider")
	}
	if p := sc.ChannelDebug(); p == nil {
		t.Error("ChannelDebug() = nil, want non-nil provider")
	}
	// ConfigDebug returns nil when configManager is nil (nil-stub path).
	if p := sc.ConfigDebug(); p != nil {
		t.Errorf("ConfigDebug() with nil stub = %v, want nil", p)
	}
	if snaps := sc.PoolSnapshots(); len(snaps) != 0 {
		t.Errorf("PoolSnapshots() on fresh client = %v, want empty", snaps)
	}
	if snaps := sc.LoadBalancingSnapshots(); len(snaps) != 0 {
		t.Errorf("LoadBalancingSnapshots() on fresh client = %v, want empty", snaps)
	}
}

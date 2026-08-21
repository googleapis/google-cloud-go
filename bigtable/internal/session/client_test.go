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
	btransport "cloud.google.com/go/bigtable/internal/transport"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
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
	if cfg.FeatureFlagsProto == nil {
		cfg.FeatureFlagsProto = btransport.NewFeatureFlagsProto(btransport.FeatureFlagsInput{
			ClientSideMetricsEnabled: cfg.MetricsEnabled,
			EnableDirectAccess:       true,
		})
	}
	return newSessionClientFromParts(pool, nil, newTestFactory(t), cfg)
}

// decodeFeatureFlags decodes the value of the bigtable-features
// metadata header (base64(proto.Marshal(FeatureFlags))) back into a
// FeatureFlags struct — mirrors what the server does on the wire.
func decodeFeatureFlags(t *testing.T, md metadata.MD) *btpb.FeatureFlags {
	t.Helper()
	vals := md.Get(btransport.FeatureFlagsHeader)
	if len(vals) != 1 {
		t.Fatalf("md[%q] = %v, want exactly one value", btransport.FeatureFlagsHeader, vals)
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
	md := btransport.MarshalFeatureFlagsMD(btransport.NewFeatureFlagsProto(btransport.FeatureFlagsInput{}))
	ff := decodeFeatureFlags(t, md)
	if !ff.RoutingCookie || !ff.ReverseScans || !ff.LastScannedRowResponses || !ff.SessionsCompatible || !ff.PeerInfo {
		t.Errorf("always-on flags missing: %+v", ff)
	}
}

// TestBuildFeatureFlagsMD_ReflectsToggles walks the caller-driven
// inputs and asserts each maps to the right proto field. RetryInfo
// is not in the input set — it's unconditionally true on the wire.
func TestBuildFeatureFlagsMD_ReflectsToggles(t *testing.T) {
	tests := []struct {
		name           string
		metricsEnabled bool
		directAccess   bool
		wantMetrics    bool
		wantDirect     bool
	}{
		{"all-off", false, false, false, false},
		{"all-on", true, true, true, true},
		{"metrics-only", true, false, true, false},
		{"direct-only", false, true, false, true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			ff := decodeFeatureFlags(t, btransport.MarshalFeatureFlagsMD(btransport.NewFeatureFlagsProto(btransport.FeatureFlagsInput{
				ClientSideMetricsEnabled: tc.metricsEnabled,
				EnableDirectAccess:       tc.directAccess,
			})))
			if ff.ClientSideMetricsEnabled != tc.wantMetrics {
				t.Errorf("ClientSideMetricsEnabled = %v, want %v", ff.ClientSideMetricsEnabled, tc.wantMetrics)
			}
			// RetryInfo is unconditionally true — this client always
			// honors server-attached retry hints.
			if !ff.RetryInfo {
				t.Errorf("RetryInfo = false, want true (unconditional)")
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
	ffMD := btransport.MarshalFeatureFlagsMD(btransport.NewFeatureFlagsProto(btransport.FeatureFlagsInput{
		ClientSideMetricsEnabled: true,
		EnableDirectAccess:       true,
	}))
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
	if len(md.Get(btransport.FeatureFlagsHeader)) != 1 {
		t.Errorf("feature-flags header missing from perResourceMetadata output: %v", md)
	}
}

// TestSessionClient_FeatureFlags asserts featureFlags() (used to build
// each OpenSessionRequest.Flags) reflects the config booleans and
// always ships RetryInfo=true (single source of truth in
// NewFeatureFlagsProto).
func TestSessionClient_FeatureFlags(t *testing.T) {
	sc := newTestClient(t, nil, Config{MetricsEnabled: true})
	defer sc.Close()

	ff := sc.featureFlags()
	if !ff.ClientSideMetricsEnabled {
		t.Error("ClientSideMetricsEnabled = false, want true (MetricsEnabled=true)")
	}
	if !ff.RetryInfo {
		t.Error("RetryInfo = false, want true (unconditional)")
	}
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

	got := sc.buildLazyOpener("projects/p/instances/i/materializedViews/v", nil, nil, nil, poolKey{resourceName: "mv:v", perm: permissionWrite}, &poolCloser{})
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
// share a meter provider with the Client.
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
// panic when the Client wasn't constructed with a real gRPC
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

	if api := sc.OpenTable("t"); api == nil {
		t.Error("OpenTable returned nil TableAPI")
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
	// EnableDebug: true — otherwise SessionDebug() returns nil by design.
	// The "not enabled" nil path is exercised by the sessionClient
	// unit tests that construct a client with default (zero-value) Config.
	sc := newTestClient(t, &fakeChannelPool{}, Config{EnableDebug: true})
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

// TestPoolKey_DisplayName pins the "<resource-id>-<PERM>" contract for
// the session_name OTel metric label + sessionz UI. If this test
// changes, coordinate with dashboard owners — session_name is a public
// metric label.
func TestPoolKey_DisplayName(t *testing.T) {
	tests := []struct {
		name string
		key  poolKey
		want string
	}{
		{"table read", poolKey{resourceName: "table:my-table", perm: permissionRead}, "my-table-READ"},
		{"table write", poolKey{resourceName: "table:my-table", perm: permissionWrite}, "my-table-WRITE"},
		{"authorized view read", poolKey{resourceName: "av:my-table:my-view", perm: permissionRead}, "my-table/my-view-READ"},
		{"authorized view write", poolKey{resourceName: "av:my-table:my-view", perm: permissionWrite}, "my-table/my-view-WRITE"},
		{"authorized view — same view id, different tables must not collide",
			poolKey{resourceName: "av:other-table:my-view", perm: permissionRead}, "other-table/my-view-READ"},
		{"materialized view read", poolKey{resourceName: "mv:my-mat-view", perm: permissionRead}, "my-mat-view-READ"},
		{"table id with dashes and dots", poolKey{resourceName: "table:tbl-1.foo", perm: permissionRead}, "tbl-1.foo-READ"},
		// Unknown prefix falls through to the raw resource string so
		// operators still get a legible label even if a future resource
		// kind hasn't been wired into displayResource yet.
		{"unknown prefix falls through", poolKey{resourceName: "future-kind:xyz", perm: permissionRead}, "future-kind:xyz-READ"},
		// Malformed inputs: pin current fallback behavior so nobody
		// silently changes to a panic. In practice these can't be
		// produced by any of the Open* call sites; the assertions are
		// defensive-programming contracts on displayResource.
		{"empty resource", poolKey{resourceName: "", perm: permissionRead}, "-READ"},
		{"empty table id after prefix", poolKey{resourceName: "table:", perm: permissionRead}, "-READ"},
		{"empty view id after av prefix", poolKey{resourceName: "av:t:", perm: permissionRead}, "t/-READ"},
		{"empty view id after mv prefix", poolKey{resourceName: "mv:", perm: permissionRead}, "-READ"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := tc.key.displayName(); got != tc.want {
				t.Errorf("displayName() = %q, want %q", got, tc.want)
			}
		})
	}
}

// --- releasePoolByID ---------------------------------------------------------

// TestReleasePoolByID_AfterClientClose_NoOp: once Close nils out
// sessionPools, any late release from a poolCloser draining on the way
// out must be a benign no-op rather than a nil-map panic.
func TestReleasePoolByID_AfterClientClose_NoOp(t *testing.T) {
	sc := newTestClient(t, &fakeChannelPool{}, Config{})
	release := sc.releasePoolByID(1)
	sc.sessionPools = nil // simulate Close having snapshotted+nil'd
	if err := release(); err != nil {
		t.Errorf("releasePoolByID on nil map = %v, want nil (post-Close no-op)", err)
	}
}

// TestReleasePoolByID_MissingIDNoOp: releasing an id that isn't in the
// map returns nil and leaves other entries untouched. This is what makes
// double-release safe (winner deletes; loser sees absent).
func TestReleasePoolByID_MissingIDNoOp(t *testing.T) {
	sc := newTestClient(t, &fakeChannelPool{}, Config{})
	const presentID = 7
	sc.sessionPools[presentID] = &managedSessionPool{id: presentID} // sentinel, not touched
	if err := sc.releasePoolByID(99)(); err != nil {
		t.Errorf("releasePoolByID on absent id = %v, want nil", err)
	}
	if _, still := sc.sessionPools[presentID]; !still {
		t.Error("releasePoolByID on absent id deleted a different entry")
	}
}

// TestReleasePoolByID_RemovesEntryAndInvokesUnregister covers the happy
// path with a real (unstarted) SessionPoolImpl. The pool never dialed a
// stream so Close returns cleanly; we assert the map entry is gone AND
// the config-listener unregister thunk fired, and that the returned
// closer is idempotent.
func TestReleasePoolByID_RemovesEntryAndInvokesUnregister(t *testing.T) {
	sc := newTestClient(t, &fakeChannelPool{}, Config{})
	const id = 3
	pool := btransport.NewSessionPoolImpl(
		id, "test-pool",
		func(ctx context.Context) (btransport.Stream, error) { return nil, errors.New("test never dials") },
		&btpb.OpenSessionRequest{}, nil,
		btransport.SessionTypeTable, false,
	)
	var unregistered int
	sc.sessionPools[id] = &managedSessionPool{
		id:         id,
		key:        poolKey{resourceName: "table:foo", perm: permissionRead},
		pool:       pool,
		unregister: func() { unregistered++ },
	}
	release := sc.releasePoolByID(id)
	if err := release(); err != nil {
		t.Fatalf("releasePoolByID = %v, want nil", err)
	}
	if _, still := sc.sessionPools[id]; still {
		t.Error("sessionPools still holds the released id")
	}
	if unregistered != 1 {
		t.Errorf("unregister fired %d times, want 1", unregistered)
	}
	// Second release is a clean no-op (winner deleted, loser sees absent).
	if err := release(); err != nil {
		t.Errorf("second releasePoolByID = %v, want nil", err)
	}
	if unregistered != 1 {
		t.Errorf("unregister fired %d times after second release, want still 1", unregistered)
	}
}

// --- single ownership --------------------------------------------------------

// TestCreateSessionPool_SingleOwnership_SameKeyDistinctPools pins the
// heart of the fix: two Open* calls that resolve the SAME poolKey each
// get their OWN pool, registered under a distinct id — createSessionPool
// NEVER dedups on key. Releasing one pool's id-scoped closer tears down
// only that pool and leaves the sibling registered and open, so a
// discarded-loser or evicted-then-reopened handle can no longer close a
// pool a live sibling is still using.
func TestCreateSessionPool_SingleOwnership_SameKeyDistinctPools(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel) // stops the pools' background loops on test exit
	sc := newTestClient(t, &fakeChannelPool{}, Config{BackgroundCtx: ctx})

	key := poolKey{resourceName: "table:foo", perm: permissionRead}
	neverDial := func(context.Context) (btransport.Stream, error) {
		return nil, errors.New("test never dials")
	}
	newPool := func() (*btransport.SessionPoolImpl, func() error) {
		return sc.createSessionPool(key, neverDial, &btpb.OpenSessionRequest{}, nil, btransport.SessionTypeTable)
	}

	poolA, releaseA := newPool()
	poolB, releaseB := newPool()

	if poolA == nil || poolB == nil {
		t.Fatalf("createSessionPool returned nil pool: A=%v B=%v", poolA, poolB)
	}
	if poolA == poolB {
		t.Fatal("same poolKey returned the SAME pool instance — createSessionPool deduped on key (regressed to shared-pool ownership)")
	}
	if got := len(sc.sessionPools); got != 2 {
		t.Fatalf("sessionPools has %d entries, want 2 (both siblings registered under distinct ids)", got)
	}

	// Releasing A removes ONLY A; B stays registered and points at the
	// same live pool it created. This is the assertion the old shared-key
	// releaseSessionPool could not satisfy.
	if err := releaseA(); err != nil {
		t.Fatalf("releaseA: %v", err)
	}
	if got := len(sc.sessionPools); got != 1 {
		t.Fatalf("after releaseA, sessionPools has %d entries, want 1 (sibling survives)", got)
	}
	var survivor *managedSessionPool
	for _, mp := range sc.sessionPools {
		survivor = mp
	}
	if survivor == nil || survivor.pool != poolB {
		t.Fatalf("releaseA tore down the sibling: survivor=%v, want B's pool", survivor)
	}

	// B's closer is independent and still fires cleanly.
	if err := releaseB(); err != nil {
		t.Fatalf("releaseB: %v", err)
	}
	if got := len(sc.sessionPools); got != 0 {
		t.Errorf("after releaseB, sessionPools has %d entries, want 0", got)
	}
}

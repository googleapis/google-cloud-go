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
	"fmt"
	"net/url"
	"os"
	"sort"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	btpb "cloud.google.com/go/bigtable/apiv2/bigtablepb"
	metrics "cloud.google.com/go/bigtable/internal/metrics"
	btopt "cloud.google.com/go/bigtable/internal/option"
	btransport "cloud.google.com/go/bigtable/internal/transport"
	otelmetric "go.opentelemetry.io/otel/metric"
	"google.golang.org/api/option"
	"google.golang.org/api/option/internaloption"
	gtransport "google.golang.org/api/transport/grpc"
	"google.golang.org/grpc/metadata"
	"google.golang.org/protobuf/proto"
)

// defaultSessionChannelPoolSize seeds the session channel pool with a
// small footprint. The server-driven ClientConfigurationManager
// reshapes this at runtime via SessionClientConfiguration polls, so
// the initial size only has to carry traffic long enough for the
// first poll to return. Session pools intentionally IGNORE
// option.WithGRPCConnectionPool(N) — pool shape is server-driven
// end-to-end.
const defaultSessionChannelPoolSize = 10

// Standard gRPC routing headers — aliased to the shared exports in
// internal/transport so the session package and the top-level bigtable
// package (which keeps its own copies for back-compat) don't drift.
const (
	resourcePrefixHeader = btransport.ResourcePrefixHeader
	requestParamsHeader  = btransport.RequestParamsHeader
)

// sessionProtocolVersion is the wire-protocol version stamped on every
// OpenSessionRequest envelope. Bump when we introduce a
// non-backwards-compatible change to the request/response shape or the
// vRPC descriptor set.
const sessionProtocolVersion = 1

// featureFlagsHeaderKey mirrors the bigtable package constant of the
// same name — duplicated because internal/session can't import
// bigtable (import cycle).
const featureFlagsHeaderKey = "bigtable-features"

// ChannelPool is the narrow surface sessionClient needs from the
// managed channel pool it owns. Satisfied by
// *btransport.BigtableChannelPool. Interfaced so tests can swap in a
// fake pool without wiring a real gRPC transport.
type ChannelPool interface {
	Close() error
}

// Config bundles the settings sessionClient needs at construction
// time. Kept as a struct rather than a long positional constructor.
type Config struct {
	// Project / Instance / AppProfile identify the target resource
	// and get baked into resource-name composition + request-params.
	Project    string
	Instance   string
	AppProfile string

	// FeatureFlagsMD is merged into per-pool routing metadata.
	FeatureFlagsMD metadata.MD

	// ConfigMD is the metadata attached to the ClientConfigurationManager's
	// GetClientConfiguration polls — instance-scoped headers.
	ConfigMD metadata.MD

	// MetricsEnabled / DisableRetryInfo mirror the SessionManager
	// booleans of the same name; both propagate into FeatureFlags on
	// every OpenSessionRequest.
	MetricsEnabled   bool
	DisableRetryInfo bool

	// SessionLoadListener is invoked whenever the server-driven
	// ClientConfigurationManager reports a new session-load ratio. The
	// bigtable Client wires this to Diverter.SetSessionLoad so the
	// classic/session traffic split follows the server's directive.
	SessionLoadListener func(load float64)

	// BackgroundCtx is the parent context passed to each per-pool
	// Start(ctx) call. Cancelled by Client teardown so every pool's
	// Tick + AFE prune loops unwind together.
	BackgroundCtx context.Context

	// EnableDebug controls whether the pools this Client mints will
	// collect per-pool snapshot state (sessionz / afez / flightz /
	// loadz). Default false. When false, every allocating debug
	// recorder in the pool is skipped for zero hot-path overhead —
	// no per-session events ring, no latency-sample buffers, no
	// per-pick candidate slices, no pool-wide histogram inserts, no
	// slow-vRPC log entries. SessionDebug() also returns nil so the
	// debugview handler renders a "not enabled" panel.
	//
	// Callers that plan to serve /debug/ from bigtable/debugview or
	// scrape session snapshots programmatically should set this true;
	// production workloads that only care about the OTel metrics
	// (attempt_latencies / operation_latencies / etc.) can leave it
	// off. The debug surface is otherwise unchanged; flipping the
	// flag on or off requires rebuilding the client.
	EnableDebug bool

	// DisableSessionAwareChannelPool reverts to the classic RPC-load-
	// driven DynamicScaleMonitor for channel-pool sizing. Default
	// (false) is the recommended production behavior: session-count-
	// driven sizing keeps per-channel session load under the server-
	// specified soft cap (ChannelPoolConfiguration.PerServerSessionCount).
	//
	// Escape hatch — flip on if a deployment sees pathological churn
	// under the session-aware policy and needs the classic behavior
	// while root-cause is investigated.
	DisableSessionAwareChannelPool bool
}

// managedSessionPool bundles a pool with its config-listener unregister
// thunk so the listener can be detached before pool teardown.
type managedSessionPool struct {
	pool       *btransport.SessionPoolImpl
	unregister func()
}

// permission is the read/write axis of a pool's identity. Kept as a
// typed enum (not a string suffix on the map key) so getOrCreateSessionPool
// doesn't have to reverse-parse the key to label the pool.
type permission int

const (
	permissionRead permission = iota
	permissionWrite
)

// display returns the label used in pool display names (e.g. "READ").
func (p permission) display() string {
	switch p {
	case permissionRead:
		return "READ"
	case permissionWrite:
		return "WRITE"
	}
	return ""
}

// poolKey identifies one pool inside sessionClient.pools. Resource is
// the caller-supplied short name ("table:foo", "av:t:v", "mv:v");
// permission separates the read pool from the write pool for the same
// resource. Struct keys let the map dedup on both axes without string
// concatenation.
type poolKey struct {
	resourceName string
	perm         permission
}

// less orders poolKeys for stable snapshot rendering (resource first,
// then permission).
func (k poolKey) less(other poolKey) bool {
	if k.resourceName != other.resourceName {
		return k.resourceName < other.resourceName
	}
	return k.perm < other.perm
}

// displayName renders the human-readable pool identity that is stamped
// as the OTel `session_name` metric label (via WithSessionPoolName)
// and rendered in sessionz. Format: "<resource-id>-<PERM>".
//
// Resource-id is the caller-supplied short name with the internal
// type-prefix stripped:
//
//	table:<id>    → "<id>-<PERM>"            e.g. "my-table-READ"
//	av:<t>:<v>    → "<t>/<v>-<PERM>"         e.g. "my-table/my-view-READ"
//	mv:<v>        → "<v>-<PERM>"             e.g. "my-view-READ"
//
// For authorized views the table qualifier is preserved as "<table>/<view>"
// (converted from the "av:" pool-key encoding "av:<table>:<view>") so
// two AVs with the same view id on different tables produce distinct
// `session_name` timeseries and distinct sessionz rows — otherwise they
// would silently aggregate on the label.
//
// (Resource, permission) is already unique per pool, so the numeric
// pool id is intentionally left out of the display name. The pool id
// remains on SessionPoolImpl (as poolID) and gets baked into per-session
// log names via createSession — `session_name` label cardinality stays
// bounded by (resource × permission).
func (k poolKey) displayName() string {
	r := k.resourceName
	switch {
	case strings.HasPrefix(r, "table:"):
		r = strings.TrimPrefix(r, "table:")
	case strings.HasPrefix(r, "mv:"):
		r = strings.TrimPrefix(r, "mv:")
	case strings.HasPrefix(r, "av:"):
		// "av:<table>:<view>" → "<table>/<view>" so the label
		// disambiguates AVs with the same view id on different tables.
		r = strings.Replace(strings.TrimPrefix(r, "av:"), ":", "/", 1)
	}
	return r + "-" + k.perm.display()
}

// sessionClient is the internal implementation of the Client
// interface. Owns the channel pool + gRPC stub + configuration
// manager, and vends per-resource TableAPI instances.
//
// The channel pool + stub + metrics factory are OWNED — Close() closes
// all three. backgroundCancel unwinds every per-pool goroutine parented
// on the internally-created background ctx.
type sessionClient struct {
	cfg            Config
	channelPool    ChannelPool
	stub           btpb.BigtableClient
	metricsFactory *metrics.Factory
	// configManager runs the server-driven GetClientConfiguration poll
	// loop for the lifetime of this Client. Production runtime state
	// (not debug/test) — its output drives Diverter.SetSessionLoad,
	// per-pool UpdateConfig reshapes (bounds, picker, load-balancing
	// strategy), and future eager-scale directives. Also surfaced on
	// /debug/configz for operator visibility.
	configManager    *btransport.ClientConfigurationManager
	backgroundCancel context.CancelFunc // release when Close() runs
	// managed carries the DynamicScaleMonitor + ConnectionRecycler that
	// the shared btransport.CreateAndStartManagedChannelPool wires up.
	// Close() unwinds both by calling managed.Close(). Zero-value for
	// the test factory (newSessionClientFromParts with a fake pool):
	// managed.Pool == nil, so Close falls back to sc.channelPool.Close().
	managedChannelPool btransport.ManagedChannelPool

	// scaleMonitor sizes the shared channel pool from live session count
	// instead of RPC load. Populated by NewClient when
	// Config.DisableSessionAwareChannelPool is false (default). Nil on
	// the test-only newSessionClientFromParts path and when the opt-out
	// flag is set. Close() stops it before managed.Close so no scale
	// tick races against pool teardown.
	scaleMonitor       *btransport.SessionAwareScaleMonitor
	scaleMonitorUnreg  func()

	// enableDebug mirrors Config.EnableDebug. When false, pools this
	// client mints short-circuit every allocating debug recorder and
	// SessionDebug() returns nil so /debug/ renders a "not enabled"
	// panel. Immutable after construction — no atomic needed.
	enableDebug bool

	sessionPoolsMu sync.Mutex
	sessionPools   map[poolKey]*managedSessionPool
	nextPoolID     atomic.Uint64
}

// NewClient constructs a standalone session.Client. It owns the
// underlying channel pool, gRPC stub, metrics factory, and background
// goroutines end-to-end — Close() unwinds all four.
//
// The metricsProvider argument mirrors bigtable.ClientConfig.MetricsProvider
// (nil = built-in metrics enabled, NoopMetricsProvider{} = disabled).
// opts are the standard google.api option.ClientOption values passed to
// gtransport.Dial — endpoint, credentials, gRPC connection pool size,
// etc.
//
// Pool sizing bootstraps from btransport.defaultPoolConfig() and is
// overridden at runtime by the server-driven SessionClientConfiguration
// polls, which reshape live pools via SessionPoolImpl.UpdateConfig.
//
// The load-balancing hook for a mixed-mode setup lives at
// AddSessionLoadListener — call it after construction if you're
// composing this Client with a bigtable.Client Diverter.
func NewClient(
	ctx context.Context,
	project, instance, appProfile string,
	metricsProvider metrics.MetricsProvider,
	enableDebug bool,
	opts ...option.ClientOption,
) (Client, error) {
	factory, err := metrics.NewFactory(ctx, project, instance, appProfile, metricsProvider)
	if err != nil {
		return nil, fmt.Errorf("session.NewClient: metrics.NewFactory: %w", err)
	}

	// Feature-flag metadata carried on every Prime + GetClientConfiguration
	// invocation. Mirrors bigtable.createFeatureFlagsMD(true, false, true) —
	// duplicated to avoid an import cycle back into the bigtable package.
	directAccessMD := buildFeatureFlagsMD(factory.Enabled, false /* disableRetryInfo */, true /* enableDirectAccess */)

	fullInstance := fmt.Sprintf("projects/%s/instances/%s", project, instance)

	// Instance-scoped headers for GetClientConfiguration — shared by the
	// DirectAccessChecker's compat probe and ClientConfigurationManager's
	// steady-state polls so both hit the same-shaped RPC.
	configMD := metadata.Join(metadata.Pairs(
		resourcePrefixHeader, fullInstance,
		requestParamsHeader, fmt.Sprintf("name=%s", url.QueryEscape(fullInstance)),
	), directAccessMD)

	// Session pool bypasses CreateAndStartManagedChannelPool so we can
	// plug in the session-flavored primer + direct-access checker
	// directly instead of adding config knobs to the classic helper.
	// NoOpChannelPrimer skips PingAndWarm on every sub-channel —
	// session channels warm on-demand via OpenSession bidi streams.
	// NewSessionClientDirectAccessChecker probes with
	// GetClientConfiguration (the same RPC ClientConfigurationManager
	// polls) instead of PingAndWarm, matching the actual on-wire RPC
	// mix a session pool serves.
	poolSize := defaultSessionChannelPoolSize
	dial := func() (*btransport.BigtableConn, error) {
		grpcConn, dialErr := gtransport.Dial(ctx, opts...)
		if dialErr != nil {
			return nil, dialErr
		}
		return btransport.NewBigtableConn(grpcConn), nil
	}
	daDialOpts := append(append([]option.ClientOption{}, opts...),
		internaloption.EnableDirectPath(true),
		internaloption.EnableDirectPathXds(),
		internaloption.AllowHardBoundTokens("ALTS"))
	daDial := func() (*btransport.BigtableConn, error) {
		grpcConn, dialErr := gtransport.Dial(ctx, daDialOpts...)
		if dialErr != nil {
			return nil, dialErr
		}
		return btransport.NewBigtableConn(grpcConn), nil
	}

	if err := btransport.ValidateDynamicConfig(btopt.DefaultDynamicChannelPoolConfig(), poolSize); err != nil {
		return nil, fmt.Errorf("session.NewClient: invalid DynamicChannelPoolConfig: %w", err)
	}
	pool, err := btransport.NewBigtableChannelPool(
		ctx,
		poolSize,
		btopt.BigtableLoadBalancingStrategy(),
		dial,
		time.Now(),
		btransport.WithMetricsReporterConfig(btopt.DefaultMetricsReporterConfig()),
		btransport.WithMeterProvider(factory.OtelMeterProvider),
		btransport.WithChannelPrimer(btransport.NoOpChannelPrimer{}),
		btransport.WithDirectAccessChecker(btransport.NewSessionClientDirectAccessChecker(
			daDial, fullInstance, appProfile, directAccessMD,
			factory.OtelMeterProvider, nil,
		)),
	)
	if err != nil {
		return nil, fmt.Errorf("session.NewClient: NewBigtableChannelPool: %w", err)
	}
	// DynamicScaleMonitor is skipped in favor of SessionAwareScaleMonitor
	// (registered after sc.configManager exists — see below). The two
	// policies can't share a pool: both would fight to add/remove
	// connections on independent signals. Session-aware is the more
	// informed signal for the session data plane; RPC-load stays
	// available on the classic path (bigtable/internal/transport/
	// channel_pool_factory.go). ManagedChannelPool.Close is nil-safe on
	// Dsm so passing nil here doesn't break the teardown chain.
	connRecycler := btransport.NewConnectionRecycler(btopt.DefaultConnectionRecycleConfig(), pool)
	connRecycler.Start(ctx)
	managed := btransport.NewManagedChannelPool(pool, nil, connRecycler)
	stub := btpb.NewBigtableClient(pool)

	backgroundCtx, cancel := context.WithCancel(context.Background())

	sc := newSessionClientFromParts(pool, stub, factory, Config{
		Project:          project,
		Instance:         instance,
		AppProfile:       appProfile,
		FeatureFlagsMD:   directAccessMD,
		ConfigMD:         configMD,
		MetricsEnabled:   factory.Enabled,
		DisableRetryInfo: false,
		BackgroundCtx:    backgroundCtx,
		EnableDebug:      enableDebug,
	})
	sc.backgroundCancel = cancel
	sc.managedChannelPool = managed
	// SessionAwareScaleMonitor is only meaningful once sc.configManager
	// exists — newSessionClientFromParts constructs it inline. Register
	// after that call so the config listener has something to hook.
	sc.registerScaleMonitor(pool)
	return sc, nil
}

// newSessionClientFromParts wires a sessionClient from already-built
// pool + stub + factory + Config. Extracted from the old public
// constructor so the new NewClient can share the assembly path.
// Unexported — no consumer outside this package should reach for it.
func newSessionClientFromParts(channelPool ChannelPool, stub btpb.BigtableClient, metricsFactory *metrics.Factory, cfg Config) *sessionClient {
	if cfg.MetricsEnabled && metricsFactory != nil {
		_ = btransport.InitializeSessionMetrics(metricsFactory.OtelMeterProvider)
	}
	sc := &sessionClient{
		cfg:            cfg,
		channelPool:    channelPool,
		stub:           stub,
		metricsFactory: metricsFactory,
		enableDebug:    cfg.EnableDebug,
		sessionPools:   make(map[poolKey]*managedSessionPool),
	}
	// stub == nil only happens on the test-only newSessionClientFromParts
	// path (unit tests wiring a fake ChannelPool without a real gRPC stub).
	// In that mode we do NOT construct a ClientConfigurationManager, do
	// NOT poll GetClientConfiguration, and do NOT fall back to any
	// default config — pools open with whatever seed the test supplies,
	// and SessionLoadListener never fires. Production NewClient always
	// supplies a stub.
	if stub != nil {
		sc.configManager = btransport.NewClientConfigurationManager(
			stub, sc.fullInstanceName(), cfg.AppProfile, cfg.ConfigMD, nil,
		)
		sc.configManager.Start(cfg.BackgroundCtx)
		if cfg.SessionLoadListener != nil {
			sc.configManager.AddSessionLoadListener(cfg.SessionLoadListener)
		}
	}
	return sc
}

// buildFeatureFlagsMD mirrors bigtable.createFeatureFlagsMD. Duplicated
// (rather than exported+imported) to keep the internal/session package
// free of a back-reference to the bigtable package.
func buildFeatureFlagsMD(clientSideMetricsEnabled, disableRetryInfo, enableDirectAccess bool) metadata.MD {
	// CBT_FORCE_SESSION tri-state — Google-internal only, gated
	// server-side. See bigtable.createFeatureFlagsMD for the semantic
	// (kept in sync intentionally; internal/session can't import
	// bigtable due to the import-cycle boundary).
	sessionsCompatible, sessionsRequired := true, false
	if v, ok := os.LookupEnv("CBT_FORCE_SESSION"); ok {
		if b, err := strconv.ParseBool(v); err == nil {
			sessionsCompatible, sessionsRequired = b, b
		}
	}
	ff := btpb.FeatureFlags{
		RoutingCookie:            true,
		ReverseScans:             true,
		LastScannedRowResponses:  true,
		ClientSideMetricsEnabled: clientSideMetricsEnabled,
		RetryInfo:                !disableRetryInfo,
		TrafficDirectorEnabled:   enableDirectAccess,
		DirectAccessRequested:    enableDirectAccess,
		SessionsCompatible:       sessionsCompatible,
		SessionsRequired:         sessionsRequired,
		PeerInfo:                 true,
	}
	val := ""
	if b, err := proto.Marshal(&ff); err == nil {
		val = base64.URLEncoding.EncodeToString(b)
	}
	return metadata.Pairs(featureFlagsHeaderKey, val)
}

func (sc *sessionClient) MeterProvider() otelmetric.MeterProvider {
	if sc.metricsFactory == nil {
		return nil
	}
	return sc.metricsFactory.OtelMeterProvider
}

// AddSessionLoadListener forwards to the internal
// ClientConfigurationManager. Returns a no-op unregister when no
// manager is wired (stub == nil at construction).
func (sc *sessionClient) AddSessionLoadListener(fn func(load float64)) func() {
	if sc.configManager == nil {
		return func() {}
	}
	return sc.configManager.AddSessionLoadListener(fn)
}

// MetricsFactory returns the *metrics.Factory the Client was
// constructed with. Exposed to internal callers (sessionTable) that
// need to construct tracers lazily when ctx doesn't already carry one.
func (sc *sessionClient) MetricsFactory() *metrics.Factory {
	return sc.metricsFactory
}

// OpenTable returns a TableAPI for a standard table.
func (sc *sessionClient) OpenTable(tableID string) TableAPI {
	fullName := sc.fullTableName(tableID)
	streamFactory := func(ctx context.Context) (btransport.Stream, error) { return sc.stub.OpenTable(ctx) }
	resource := "table:" + tableID
	readKey := poolKey{resource, permissionRead}
	writeKey := poolKey{resource, permissionWrite}
	openRead := sc.buildLazyOpener(fullName, btransport.TABLE_SESSION, streamFactory,
		&btpb.OpenTableRequest{TableName: fullName, AppProfileId: sc.cfg.AppProfile, Permission: btpb.OpenTableRequest_PERMISSION_READ},
		readKey)
	openWrite := sc.buildLazyOpener(fullName, btransport.TABLE_SESSION, streamFactory,
		&btpb.OpenTableRequest{TableName: fullName, AppProfileId: sc.cfg.AppProfile, Permission: btpb.OpenTableRequest_PERMISSION_WRITE},
		writeKey)
	closeRead := sc.buildLazyReleaser(readKey)
	closeWrite := sc.buildLazyReleaser(writeKey)
	return newSessionTable(tableID, openRead, openWrite, closeRead, closeWrite, btransport.READ_ROW, btransport.MUTATE_ROW, sc.perResourceMetadata(fullName, "table_name", fullName), sc.metricsFactory)
}

// OpenAuthorizedView returns a TableAPI for an authorized view.
func (sc *sessionClient) OpenAuthorizedView(table, view string) TableAPI {
	fullName := sc.fullAuthorizedViewName(table, view)
	streamFactory := func(ctx context.Context) (btransport.Stream, error) { return sc.stub.OpenAuthorizedView(ctx) }
	resource := fmt.Sprintf("av:%s:%s", table, view)
	readKey := poolKey{resource, permissionRead}
	writeKey := poolKey{resource, permissionWrite}
	openRead := sc.buildLazyOpener(fullName, btransport.AUTHORIZED_VIEW_SESSION, streamFactory,
		&btpb.OpenAuthorizedViewRequest{AuthorizedViewName: fullName, AppProfileId: sc.cfg.AppProfile, Permission: btpb.OpenAuthorizedViewRequest_PERMISSION_READ},
		readKey)
	openWrite := sc.buildLazyOpener(fullName, btransport.AUTHORIZED_VIEW_SESSION, streamFactory,
		&btpb.OpenAuthorizedViewRequest{AuthorizedViewName: fullName, AppProfileId: sc.cfg.AppProfile, Permission: btpb.OpenAuthorizedViewRequest_PERMISSION_WRITE},
		writeKey)
	closeRead := sc.buildLazyReleaser(readKey)
	closeWrite := sc.buildLazyReleaser(writeKey)
	return newSessionTable(table, openRead, openWrite, closeRead, closeWrite, btransport.READ_ROW_AUTH_VIEW, btransport.MUTATE_ROW_AUTH_VIEW, sc.perResourceMetadata(fullName, "authorized_view_name", fullName), sc.metricsFactory)
}

// OpenMaterializedView returns a read-only TableAPI for a
// materialized view. Only a read pool is opened; MutateRow errors
// cleanly via the nil openWrite passed to newSessionTable.
func (sc *sessionClient) OpenMaterializedView(view string) TableAPI {
	fullName := sc.fullMaterializedViewName(view)
	readKey := poolKey{"mv:" + view, permissionRead}
	openRead := sc.buildLazyOpener(fullName, btransport.MATERIALIZED_VIEW_SESSION,
		func(ctx context.Context) (btransport.Stream, error) { return sc.stub.OpenMaterializedView(ctx) },
		&btpb.OpenMaterializedViewRequest{
			MaterializedViewName: fullName,
			AppProfileId:         sc.cfg.AppProfile,
			Permission:           btpb.OpenMaterializedViewRequest_PERMISSION_READ,
		},
		readKey)
	closeRead := sc.buildLazyReleaser(readKey)
	return newSessionTable("", openRead, nil, closeRead, nil, btransport.READ_ROW_MAT_VIEW, nil, sc.perResourceMetadata(fullName, "materialized_view_name", fullName), sc.metricsFactory)
}

// registerScaleMonitor wires SessionAwareScaleMonitor to the shared
// channel pool, hooking its OnConfig into ClientConfigurationManager
// so the sizer reacts to server-driven min/max/perServerSessionCount
// updates. Skipped when the opt-out flag is set or when configManager
// is nil (test-fake path).
//
// The monitor reads its session-count signal directly from
// BigtableChannelPool.TotalStreamCount — sessions are bidi streams
// (one streamingLoad increment on the picked connEntry when they
// open), so the pool already tracks the number without any
// cross-package plumbing.
func (sc *sessionClient) registerScaleMonitor(pool *btransport.BigtableChannelPool) {
	if sc.cfg.DisableSessionAwareChannelPool || sc.configManager == nil || pool == nil {
		return
	}
	sc.scaleMonitor = btransport.NewSessionAwareScaleMonitor(pool)
	sc.scaleMonitorUnreg = sc.configManager.AddChannelPoolConfigListener(sc.scaleMonitor.OnConfig)
	sc.scaleMonitor.Start(sc.cfg.BackgroundCtx)
}

// Close tears down in a phased order that keeps late callbacks from
// firing against half-dead pools:
//  1. Stop config polling — no more UpdateConfig can fire after.
//  2. Close every session pool (per-pool listeners already detached).
//  3. Cancel the background ctx we constructed (unwinds heartbeat /
//     AFE-prune / scaling loops parented on it).
//  4. managed.Close() stops the DynamicScaleMonitor + ConnectionRecycler
//     wired by btransport.CreateAndStartManagedChannelPool, then closes
//     the underlying pool — in that order, so no scale-up / recycle tick
//     races against pool teardown. Test-fake path (managed.Pool == nil)
//     falls back to sc.channelPool.Close().
//  5. Shut down the metrics factory (final flush).
func (sc *sessionClient) Close() error {
	// Snapshot everything owned under the lock, then release before
	// running the actual Close/Shutdown/Cancel calls. Any of those can
	// block on a graceful drain or a final metrics flush, and any
	// concurrent caller reaching for a snapshot method (SessionDebug,
	// PoolSnapshots, ChannelPool) would deadlock if we held sessionPoolsMu
	// through the teardown.
	sc.sessionPoolsMu.Lock()
	pools := sc.sessionPools
	sc.sessionPools = nil // refuse subsequent Opens; getOrCreateSessionPool nil-checks
	mgr := sc.configManager
	chp := sc.channelPool
	managed := sc.managedChannelPool
	factory := sc.metricsFactory
	cancel := sc.backgroundCancel
	sam := sc.scaleMonitor
	samUnreg := sc.scaleMonitorUnreg
	sc.sessionPoolsMu.Unlock()

	// Stop the scale monitor BEFORE mgr.Close so a mid-tick reconcile
	// can't fire an addConnections into a channel pool that Phase 4
	// is about to tear down. Order: unregister listener → Stop the
	// monitor's goroutine → then unwind everything else.
	if samUnreg != nil {
		samUnreg()
	}
	if sam != nil {
		sam.Stop()
	}
	if mgr != nil {
		mgr.Close()
	}
	for _, mp := range pools {
		if mp.unregister != nil {
			mp.unregister()
		}
		mp.pool.Close()
	}
	if cancel != nil {
		cancel()
	}
	// Prefer managed.Close (stops DSM + Recycler in order then closes
	// the pool). Test fakes bypass CreateAndStartManagedChannelPool so
	// managed.Pool is nil — fall back to the fake's own Close.
	var err error
	if managed.Pool != nil {
		err = managed.Close()
	} else if chp != nil {
		err = chp.Close()
	}
	if factory != nil && factory.Shutdown != nil {
		factory.Shutdown()
	}
	return err
}

// buildLazyOpener returns a closure that, on first invocation,
// creates (or reuses via the keyed cache) the pool for the given
// payload/key. Returns nil when payload is nil (materialized-view
// write side).
func (sc *sessionClient) buildLazyOpener(
	resourceName string,
	sessionDesc *btransport.SessionDescriptor,
	streamFactory func(ctx context.Context) (btransport.Stream, error),
	payload proto.Message,
	key poolKey,
) func() (Invoker, error) {
	if payload == nil {
		return nil
	}
	return func() (Invoker, error) {
		pool, err := sc.createSessionPoolForPayload(resourceName, sessionDesc, streamFactory, payload, key)
		if err != nil {
			return nil, err
		}
		if pool == nil {
			// getOrCreateSessionPool returns nil ONLY when Close() has already
			// snapshotted the pool set. Surface a distinct sentinel so
			// callers can tell "client closed" apart from "resource has
			// no write pool" (ErrWriteNotSupported) or "bookkeeping
			// drift" (errReadPoolNil).
			return nil, ErrClientClosed
		}
		return pool, nil
	}
}

// buildLazyReleaser returns a closure that releases the pool entry
// for the given key from sessionPools + closes the underlying pool.
// Returned closure is idempotent (second call finds entry absent and
// no-ops) and nil-safe when key is the zero value (the caller can
// pass nil for resources without a write side — e.g. materialized
// views — mirroring buildLazyOpener's payload==nil convention).
//
// Symmetric with buildLazyOpener so sessionTable can drive real
// per-resource teardown from its Close() without needing back-refs
// or knowing about poolKey / sessionPools internals.
func (sc *sessionClient) buildLazyReleaser(key poolKey) func() error {
	return func() error {
		return sc.releaseSessionPool(key)
	}
}

// releaseSessionPool removes key from sessionPools and closes the
// underlying pool. No-op when:
//   - the client has already Closed (sessionPools == nil), so any late
//     release from a cache handle draining on the way out doesn't
//     racily double-close a pool that Client.Close is already
//     tearing down.
//   - the entry is absent (already released — makes second calls safe).
//
// Assumes the caller (currently bigtable.Client via sessionTableCache)
// guarantees at-most-one sessionTable per resource at any moment. That
// invariant means no co-owner can be relying on the pool we're closing.
// If session.Client is ever exposed to callers that bypass that cache,
// this must gain a refcount.
//
// Teardown runs OUTSIDE sessionPoolsMu so a graceful pool drain (which
// can block on in-flight vRPCs) doesn't deadlock any snapshotter
// waiting on the lock. Matches the snapshot-under-lock / teardown-
// outside pattern in Client.Close.
func (sc *sessionClient) releaseSessionPool(key poolKey) error {
	sc.sessionPoolsMu.Lock()
	if sc.sessionPools == nil {
		sc.sessionPoolsMu.Unlock()
		return nil
	}
	mp, ok := sc.sessionPools[key]
	if !ok {
		sc.sessionPoolsMu.Unlock()
		return nil
	}
	delete(sc.sessionPools, key)
	sc.sessionPoolsMu.Unlock()

	if mp.unregister != nil {
		mp.unregister()
	}
	return mp.pool.Close()
}

// createSessionPoolForPayload marshals the resource-typed OpenXxxRequest
// into the transport-level OpenSessionRequest envelope, builds routing
// metadata via the descriptor's MetadataFn, and delegates to
// getOrCreateSessionPool for cache-hit-or-construct.
func (sc *sessionClient) createSessionPoolForPayload(
	resourceName string,
	sessionDesc *btransport.SessionDescriptor,
	streamFactory func(ctx context.Context) (btransport.Stream, error),
	payload proto.Message,
	key poolKey,
) (*btransport.SessionPoolImpl, error) {
	if payload == nil {
		return nil, nil
	}
	payloadBytes, err := proto.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("proto.Marshal session payload: %w", err)
	}
	handshake := &btpb.OpenSessionRequest{
		ProtocolVersion: sessionProtocolVersion,
		Payload:         payloadBytes,
		Flags:           sc.featureFlags(),
	}

	metaMap := sessionDesc.MetadataFn(payload)
	keys := make([]string, 0, len(metaMap))
	for k := range metaMap {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	sessionMetadata := make([]string, 0, len(keys))
	for _, k := range keys {
		sessionMetadata = append(sessionMetadata, fmt.Sprintf("%s=%s", k, url.QueryEscape(metaMap[k])))
	}
	paramsVal := strings.Join(sessionMetadata, "&")

	md := metadata.Join(metadata.Pairs(
		resourcePrefixHeader, resourceName,
		requestParamsHeader, paramsVal,
	), sc.cfg.FeatureFlagsMD)

	return sc.getOrCreateSessionPool(key, streamFactory, handshake, md, sessionDesc.Type), nil
}

// getOrCreateSessionPool ports SessionManager.GetOrCreateSessionPool:
// dedups on key, mints a display name, constructs the pool, wires
// the config listener + background loops.
func (sc *sessionClient) getOrCreateSessionPool(
	key poolKey,
	streamFactory func(ctx context.Context) (btransport.Stream, error),
	openSessionRequest *btpb.OpenSessionRequest,
	md metadata.MD,
	sessionType btransport.SessionType,
) *btransport.SessionPoolImpl {
	sc.sessionPoolsMu.Lock()
	// Close() sets pools=nil to refuse subsequent Opens; a nil map here
	// means teardown has already snapshotted the pool set.
	if sc.sessionPools == nil {
		sc.sessionPoolsMu.Unlock()
		return nil
	}
	if mp, ok := sc.sessionPools[key]; ok {
		sc.sessionPoolsMu.Unlock()
		return mp.pool
	}
	id := sc.nextPoolID.Add(1)
	// poolName is stamped as the `session_name` OTel metric label and
	// surfaces in sessionz — "<resource-id>-<PERM>" (see poolKey.displayName).
	// Numeric pool id lives on SessionPoolImpl.poolID for the sessionz
	// ↔ channelz reverse link and per-session log names; we don't need
	// it in the human-readable label because (resource, permission)
	// already uniquely identifies the pool.
	poolName := key.displayName()
	// Pool bounds start at 0; NewSessionPoolImpl falls back to
	// defaultPoolConfig() and ClientConfigurationManager overrides on
	// first UpdateConfig with server-driven values.
	pool := btransport.NewSessionPoolImpl(
		id,
		poolName, 0, 0, streamFactory, openSessionRequest, md, sessionType,
		sc.enableDebug,
	)
	mp := &managedSessionPool{pool: pool}
	sc.sessionPools[key] = mp
	configManager := sc.configManager
	backgroundCtx := sc.cfg.BackgroundCtx
	sc.sessionPoolsMu.Unlock()
	// TODO(sushanb): publish-before-Start race — a concurrent second Open
	// on the same key sees mp in the map before pool.Start(backgroundCtx)
	// runs below and can Invoke on an unstarted pool. Fix in follow-up by
	// gating each managedSessionPool with a sync.Once + <-ready channel.

	if configManager != nil {
		unregister := configManager.AddSessionPoolListener(func(config *btpb.SessionClientConfiguration_SessionPoolConfiguration) {
			pool.UpdateConfig(config)
		})
		sc.sessionPoolsMu.Lock()
		if cur, stillThere := sc.sessionPools[key]; stillThere && cur == mp {
			mp.unregister = unregister
			sc.sessionPoolsMu.Unlock()
		} else {
			sc.sessionPoolsMu.Unlock()
			unregister()
		}
	}

	pool.Start(backgroundCtx)
	return pool
}

// featureFlags builds the OpenSessionRequest.Flags proto from
// sessionClient config. SessionsCompatible / SessionsRequired MUST
// match the values in bigtable-features metadata header (built by
// buildFeatureFlagsMD) — the server rejects OpenSession with
// INVALID_ARGUMENT when the proto and header disagree on session-mode
// flags. See bigtable.createFeatureFlagsMD for the tri-state semantic.
//
// TODO(sushanb): CBT_FORCE_SESSION is re-read here on every pool open
// but buildFeatureFlagsMD reads it once at NewClient. If the
// env flips mid-process the header + proto disagree and the server
// rejects. Resolve once at construction and stash on Config.
func (sc *sessionClient) featureFlags() *btpb.FeatureFlags {
	sessionsCompatible, sessionsRequired := true, false
	if v, ok := os.LookupEnv("CBT_FORCE_SESSION"); ok {
		if b, err := strconv.ParseBool(v); err == nil {
			sessionsCompatible, sessionsRequired = b, b
		}
	}
	return &btpb.FeatureFlags{
		RoutingCookie:            true,
		ReverseScans:             true,
		LastScannedRowResponses:  true,
		ClientSideMetricsEnabled: sc.cfg.MetricsEnabled,
		RetryInfo:                !sc.cfg.DisableRetryInfo,
		TrafficDirectorEnabled:   true,
		DirectAccessRequested:    true,
		SessionsCompatible:       sessionsCompatible,
		SessionsRequired:         sessionsRequired,
		PeerInfo:                 true,
	}
}

// perResourceMetadata builds the per-vRPC context metadata: the pair
// carried on the outgoing gRPC call for the underlying bidi stream.
// Header shape matches classic Table.md (bigtable/open.go:32-35).
func (sc *sessionClient) perResourceMetadata(fullResourceName, paramKey, paramVal string) metadata.MD {
	return metadata.Join(metadata.Pairs(
		resourcePrefixHeader, fullResourceName,
		requestParamsHeader, fmt.Sprintf("%s=%s&app_profile_id=%s", paramKey, url.QueryEscape(paramVal), url.QueryEscape(sc.cfg.AppProfile)),
	), sc.cfg.FeatureFlagsMD)
}

// Resource-name composition — duplicated from bigtable.Client to
// avoid an import cycle. Keep in sync with client.go's
// fullTableName / fullAuthorizedViewName / fullMaterializedViewName /
// fullInstanceName helpers.

func (sc *sessionClient) fullTableName(table string) string {
	return fmt.Sprintf("projects/%s/instances/%s/tables/%s", sc.cfg.Project, sc.cfg.Instance, table)
}

func (sc *sessionClient) fullAuthorizedViewName(table, view string) string {
	return fmt.Sprintf("projects/%s/instances/%s/tables/%s/authorizedViews/%s", sc.cfg.Project, sc.cfg.Instance, table, view)
}

func (sc *sessionClient) fullMaterializedViewName(view string) string {
	return fmt.Sprintf("projects/%s/instances/%s/materializedViews/%s", sc.cfg.Project, sc.cfg.Instance, view)
}

func (sc *sessionClient) fullInstanceName() string {
	return fmt.Sprintf("projects/%s/instances/%s", sc.cfg.Project, sc.cfg.Instance)
}

// Debug accessors exposed for the bigtable/session_debug.go providers.
// Kept off the Client interface — consumers who need them
// type-assert to *sessionClient.

// ConfigManager returns the internal ClientConfigurationManager for
// configz. Nil when no stub was provided.
func (sc *sessionClient) ConfigManager() *btransport.ClientConfigurationManager {
	return sc.configManager
}

// PoolSnapshots returns one PoolSnapshot per owned pool, ordered by
// pool key for stable rendering. Same lock discipline as
// SessionManager.ManagerSnapshot.
func (sc *sessionClient) PoolSnapshots() []btransport.PoolSnapshot {
	entries := sc.orderedPoolEntries()
	out := make([]btransport.PoolSnapshot, 0, len(entries))
	for _, e := range entries {
		out = append(out, e.pool.PoolSnapshot())
	}
	return out
}

// LoadBalancingSnapshots returns per-pool picker + pick-history
// snapshots for loadz. Ordered by pool key.
func (sc *sessionClient) LoadBalancingSnapshots() []btransport.LoadBalancingSnapshot {
	entries := sc.orderedPoolEntries()
	out := make([]btransport.LoadBalancingSnapshot, 0, len(entries))
	for _, e := range entries {
		out = append(out, e.pool.LoadBalancingSnapshot())
	}
	return out
}

// poolEntry is the internal (key, pool) tuple used by snapshot methods
// so they can sort by poolKey without duplicating the collection loop.
type poolEntry struct {
	key  poolKey
	pool *btransport.SessionPoolImpl
}

// orderedPoolEntries snapshots the pools map under lock and returns
// its non-nil entries sorted by poolKey.
func (sc *sessionClient) orderedPoolEntries() []poolEntry {
	sc.sessionPoolsMu.Lock()
	entries := make([]poolEntry, 0, len(sc.sessionPools))
	for k, mp := range sc.sessionPools {
		if mp == nil || mp.pool == nil {
			continue
		}
		entries = append(entries, poolEntry{key: k, pool: mp.pool})
	}
	sc.sessionPoolsMu.Unlock()
	sort.Slice(entries, func(i, j int) bool { return entries[i].key.less(entries[j].key) })
	return entries
}

// ChannelPool returns the *btransport.BigtableChannelPool the
// sessionClient was constructed with, if any. Used by channelz to
// surface session-pool channel stats without leaking the interface
// through the public Client API.
func (sc *sessionClient) ChannelPool() *btransport.BigtableChannelPool {
	if sc.channelPool == nil {
		return nil
	}
	bp, _ := sc.channelPool.(*btransport.BigtableChannelPool)
	return bp
}

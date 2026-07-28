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

// Standard gRPC routing headers — duplicated from the bigtable package
// constants (package boundary means we can't import them). Keep the
// values in sync with bigtable/doc.go.
const (
	resourcePrefixHeader = "google-cloud-resource-prefix"
	requestParamsHeader  = "x-goog-request-params"
)

// Default pool sizing — same as SessionManager's fallback (10/100).
const (
	defaultMinSessions = 10
	defaultMaxSessions = 100
)

// sessionProtocolVersion is the wire-protocol version stamped on every
// OpenSessionRequest envelope. Bump when we introduce a
// non-backwards-compatible change to the request/response shape or the
// vRPC descriptor set.
const sessionProtocolVersion = 1

// defaultChannelPoolSize matches bigtable.defaultBigtableConnPoolSize
// (10) — the fallback used when neither the caller-supplied
// option.WithGRPCConnectionPool nor an internaloption resolver produces
// a size.
const defaultChannelPoolSize = 10

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

	// MinSessions / MaxSessions are per-pool bounds. Zero uses
	// defaults (10/100).
	MinSessions int
	MaxSessions int

	// SessionLoadListener is invoked whenever the server-driven
	// ClientConfigurationManager reports a new session-load ratio. The
	// bigtable Client wires this to Diverter.SetSessionLoad so the
	// classic/session traffic split follows the server's directive.
	SessionLoadListener func(load float64)

	// BackgroundCtx is the parent context passed to each per-pool
	// Start(ctx) call. Cancelled by Client teardown so every pool's
	// Tick + AFE prune loops unwind together.
	BackgroundCtx context.Context
}

// managedPool bundles a pool with its config-listener unregister
// thunk so the listener can be detached before pool teardown.
type managedPool struct {
	pool       SessionPool
	unregister func()
}

// permission is the read/write axis of a pool's identity. Kept as a
// typed enum (not a string suffix on the map key) so getOrCreatePool
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
	resource string
	perm     permission
}

// less orders poolKeys for stable snapshot rendering (resource first,
// then permission).
func (k poolKey) less(other poolKey) bool {
	if k.resource != other.resource {
		return k.resource < other.resource
	}
	return k.perm < other.perm
}

// sessionClient is the internal implementation of the SessionClient
// interface. Owns the channel pool + gRPC stub + configuration
// manager, and vends per-resource TableAPI instances.
//
// The channel pool + stub + metrics factory are OWNED — Close() closes
// all three. backgroundCancel unwinds every per-pool goroutine parented
// on the internally-created background ctx.
type sessionClient struct {
	cfg              Config
	channelPool      ChannelPool
	stub             btpb.BigtableClient
	metricsFactory   *metrics.Factory
	configManager    *btransport.ClientConfigurationManager
	backgroundCancel context.CancelFunc // release when Close() runs
	// dsm + connRecycler are the lifecycle monitors classic clients get
	// from createAndStartManagedChannelPool. Session client wires them
	// itself so operators see the same connection_pool/outstanding_rpcs,
	// per-connection error histograms, dynamic scaling, and periodic
	// connection replacement they get on the classic path. Nil for the
	// test factory (newSessionClientFromParts with a fake pool).
	dsm          *btransport.DynamicScaleMonitor
	connRecycler *btransport.ConnectionRecycler

	poolsMu    sync.Mutex
	pools      map[poolKey]*managedPool
	nextPoolID atomic.Uint64
}

// NewSessionClient constructs a standalone SessionClient. It owns the
// underlying channel pool, gRPC stub, metrics factory, and background
// goroutines end-to-end — Close() unwinds all four.
//
// The metricsProvider argument mirrors bigtable.ClientConfig.MetricsProvider
// (nil = built-in metrics enabled, NoopMetricsProvider{} = disabled).
// opts are the standard google.api option.ClientOption values passed to
// gtransport.Dial — endpoint, credentials, gRPC connection pool size,
// etc.
//
// Pool sizing (MinSessions/MaxSessions) uses internal defaults (10/100);
// override at runtime by responding to the server-driven
// SessionClientConfiguration polls, which reshape live pools via
// SessionPoolImpl.UpdateConfig.
//
// The load-balancing hook for a mixed-mode setup lives at
// AddSessionLoadListener — call it after construction if you're
// composing this SessionClient with a bigtable.Client Diverter.
func NewSessionClient(
	ctx context.Context,
	project, instance, appProfile string,
	metricsProvider metrics.MetricsProvider,
	opts ...option.ClientOption,
) (SessionClient, error) {
	factory, err := metrics.NewFactory(ctx, project, instance, appProfile, metricsProvider)
	if err != nil {
		return nil, fmt.Errorf("session.NewSessionClient: metrics.NewFactory: %w", err)
	}

	// Feature-flag metadata carried on every Prime + GetClientConfiguration
	// invocation. Mirrors bigtable.createFeatureFlagsMD(true, false, true) —
	// duplicated to avoid an import cycle back into the bigtable package.
	directAccessMD := buildFeatureFlagsMD(factory.Enabled, false /* disableRetryInfo */, true /* enableDirectAccess */)

	// Resolve pool size from opts. Falls back to the default when the
	// caller neither set option.WithGRPCConnectionPool nor provided an
	// internaloption-aware resolver.
	poolSize := resolveConnPoolSize(opts, defaultChannelPoolSize)

	fullInstance := fmt.Sprintf("projects/%s/instances/%s", project, instance)

	dial := func() (*btransport.BigtableConn, error) {
		grpcConn, dialErr := gtransport.Dial(ctx, opts...)
		if dialErr != nil {
			return nil, dialErr
		}
		return btransport.NewBigtableConn(grpcConn), nil
	}

	// Direct-access dialer for the compatibility checker only — layers
	// DirectPath enablement + ALTS hard-bound tokens on top of the
	// caller's opts. The pool itself still uses the plain `dial` above
	// (standard path); only the DAC's GetClientConfiguration probe uses
	// this.
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

	// Instance-scoped headers for GetClientConfiguration — shared by the
	// DirectAccessChecker's compat probe and ClientConfigurationManager's
	// steady-state polls so both hit the same-shaped RPC.
	configMD := metadata.Join(metadata.Pairs(
		resourcePrefixHeader, fullInstance,
		requestParamsHeader, fmt.Sprintf("name=%s", url.QueryEscape(fullInstance)),
	), directAccessMD)

	// No ChannelPrimer on the pool — session-based clients warm channels
	// via their own OpenSession bidi streams. The DAC still needs a
	// primer for its startup probe.
	//
	// TODO(sushanb): switch to NewGetClientConfigDirectAccessChecker
	// once we've validated it end-to-end in the sandbox. The session
	// path should probe with GetClientConfiguration (the same RPC
	// ConfigurationManager polls) rather than PingAndWarm — see
	// project_bigtable_direct_access_checker memory.
	// Validate before dialing so a bad config fails fast without pool churn.
	if err := btransport.ValidateDynamicConfig(btopt.DefaultDynamicChannelPoolConfig(), poolSize); err != nil {
		return nil, fmt.Errorf("session.NewSessionClient: invalid DynamicChannelPoolConfig: %w", err)
	}

	pool, err := btransport.NewBigtableChannelPool(
		ctx,
		poolSize,
		btopt.BigtableLoadBalancingStrategy(),
		dial,
		time.Now(),
		btransport.WithInstanceName(fullInstance),
		btransport.WithAppProfile(appProfile),
		btransport.WithMetricsReporterConfig(btopt.DefaultMetricsReporterConfig()),
		btransport.WithMeterProvider(factory.OtelMeterProvider),
		btransport.WithDirectAccessChecker(btransport.NewPingAndWarmDirectAccessChecker(
			daDial,
			// Primer=nil: session-based clients warm channels on-demand
			// via OpenSession, not eagerly at pool-init. The DAC skips
			// its Prime step when the primer is nil.
			nil,
			factory.OtelMeterProvider,
			nil,
		)),
	)
	if err != nil {
		return nil, fmt.Errorf("session.NewSessionClient: NewBigtableChannelPool: %w", err)
	}

	// Lifecycle monitors that the classic path gets from
	// bigtable.createAndStartManagedChannelPool. Duplicated here (rather
	// than sharing the helper) because internal/session can't import
	// bigtable. Both are opt-out on the classic side via ClientConfig
	// flags; session client always enables them for now — a future
	// SessionClientConfig can expose the same DisableDynamicChannelPool /
	// DisableConnectionRecycler knobs if operators need them.
	//
	// Started with the same ctx classic uses, and for the same reason:
	// every action DSM/ConnectionRecycler take goes through pool methods
	// (addConnections, replaceConnection, factory.newEntry) that already
	// observe pool.poolCtx (derived from this ctx). Passing an unrelated
	// background ctx to Start would create zombie tickers that keep
	// firing after the pool's own operations have shut down.
	dsm := btransport.NewDynamicScaleMonitor(btopt.DefaultDynamicChannelPoolConfig(), pool)
	dsm.Start(ctx)
	connRecycler := btransport.NewConnectionRecycler(btopt.DefaultConnectionRecycleConfig(), pool)
	connRecycler.Start(ctx)

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
		MinSessions:      defaultMinSessions,
		MaxSessions:      defaultMaxSessions,
		BackgroundCtx:    backgroundCtx,
	})
	sc.backgroundCancel = cancel
	sc.dsm = dsm
	sc.connRecycler = connRecycler
	return sc, nil
}

// newSessionClientFromParts wires a sessionClient from already-built
// pool + stub + factory + Config. Extracted from the old public
// constructor so the new NewSessionClient can share the assembly path.
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
		pools:          make(map[poolKey]*managedPool),
	}
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
	// CBT_FORCE_SESSION tri-state — see bigtable.createFeatureFlagsMD for
	// the semantic (kept in sync intentionally; internal/session can't
	// import bigtable due to the import-cycle boundary).
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

// resolveConnPoolSize walks opts for a caller-supplied gRPC connection
// pool size, falling back to defaultChannelPoolSize when unavailable.
// Mirrors the same-shaped logic in bigtable/channel_pool_factory.go.
func resolveConnPoolSize(opts []option.ClientOption, fallback int) int {
	uResolver, err := internaloption.NewUnsafeResolver(opts...)
	if err != nil {
		return fallback
	}
	if n := uResolver.ResolvedGRPCConnPoolSize(); n > 0 {
		return n
	}
	return fallback
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

// MetricsFactory returns the *metrics.Factory the SessionClient was
// constructed with. Exposed to internal callers (sessionTable) that
// need to construct tracers lazily when ctx doesn't already carry one.
func (sc *sessionClient) MetricsFactory() *metrics.Factory {
	return sc.metricsFactory
}

// OpenSessionTable returns a TableAPI for a standard table.
func (sc *sessionClient) OpenSessionTable(tableID string) TableAPI {
	fullName := sc.fullTableName(tableID)
	streamFactory := func(ctx context.Context) (btransport.Stream, error) { return sc.stub.OpenTable(ctx) }
	resource := "table:" + tableID
	openRead := sc.buildLazyOpener(fullName, btransport.TABLE_SESSION, streamFactory,
		&btpb.OpenTableRequest{TableName: fullName, AppProfileId: sc.cfg.AppProfile, Permission: btpb.OpenTableRequest_PERMISSION_READ},
		poolKey{resource, permissionRead})
	openWrite := sc.buildLazyOpener(fullName, btransport.TABLE_SESSION, streamFactory,
		&btpb.OpenTableRequest{TableName: fullName, AppProfileId: sc.cfg.AppProfile, Permission: btpb.OpenTableRequest_PERMISSION_WRITE},
		poolKey{resource, permissionWrite})
	return newSessionTable(tableID, openRead, openWrite, btransport.READ_ROW, btransport.MUTATE_ROW, sc.perResourceMetadata(fullName, "table_name", fullName), sc.metricsFactory)
}

// OpenAuthorizedView returns a TableAPI for an authorized view.
func (sc *sessionClient) OpenAuthorizedView(table, view string) TableAPI {
	fullName := sc.fullAuthorizedViewName(table, view)
	streamFactory := func(ctx context.Context) (btransport.Stream, error) { return sc.stub.OpenAuthorizedView(ctx) }
	resource := fmt.Sprintf("av:%s:%s", table, view)
	openRead := sc.buildLazyOpener(fullName, btransport.AUTHORIZED_VIEW_SESSION, streamFactory,
		&btpb.OpenAuthorizedViewRequest{AuthorizedViewName: fullName, AppProfileId: sc.cfg.AppProfile, Permission: btpb.OpenAuthorizedViewRequest_PERMISSION_READ},
		poolKey{resource, permissionRead})
	openWrite := sc.buildLazyOpener(fullName, btransport.AUTHORIZED_VIEW_SESSION, streamFactory,
		&btpb.OpenAuthorizedViewRequest{AuthorizedViewName: fullName, AppProfileId: sc.cfg.AppProfile, Permission: btpb.OpenAuthorizedViewRequest_PERMISSION_WRITE},
		poolKey{resource, permissionWrite})
	return newSessionTable(table, openRead, openWrite, btransport.READ_ROW_AUTH_VIEW, btransport.MUTATE_ROW_AUTH_VIEW, sc.perResourceMetadata(fullName, "authorized_view_name", fullName), sc.metricsFactory)
}

// OpenMaterializedView returns a read-only TableAPI for a
// materialized view. Only a read pool is opened; MutateRow errors
// cleanly via the nil openWrite passed to newSessionTable.
func (sc *sessionClient) OpenMaterializedView(view string) TableAPI {
	fullName := sc.fullMaterializedViewName(view)
	openRead := sc.buildLazyOpener(fullName, btransport.MATERIALIZED_VIEW_SESSION,
		func(ctx context.Context) (btransport.Stream, error) { return sc.stub.OpenMaterializedView(ctx) },
		&btpb.OpenMaterializedViewRequest{
			MaterializedViewName: fullName,
			AppProfileId:         sc.cfg.AppProfile,
			Permission:           btpb.OpenMaterializedViewRequest_PERMISSION_READ,
		},
		poolKey{"mv:" + view, permissionRead})
	return newSessionTable("", openRead, nil, btransport.READ_ROW_MAT_VIEW, nil, sc.perResourceMetadata(fullName, "materialized_view_name", fullName), sc.metricsFactory)
}

// Close tears down in a phased order that keeps late callbacks from
// firing against half-dead pools:
//  1. Stop config polling — no more UpdateConfig can fire after.
//  2. Close every session pool (per-pool listeners already detached).
//  3. Cancel the background ctx we constructed (unwinds heartbeat /
//     AFE-prune / scaling loops parented on it).
//  4. Stop DSM + ConnectionRecycler explicitly so no scale-up / recycle
//     tick races against the pool.Close in the next step. Their internal
//     Start-ctx is the caller's ctx, not backgroundCtx, so Stop() is the
//     only mechanism that guarantees teardown independent of caller ctx.
//  5. Close the underlying channel pool.
//  6. Shut down the metrics factory (final flush).
func (sc *sessionClient) Close() error {
	// Snapshot everything owned under the lock, then release before
	// running the actual Close/Shutdown/Cancel calls. Any of those can
	// block on a graceful drain or a final metrics flush, and any
	// concurrent caller reaching for a snapshot method (SessionDebug,
	// PoolSnapshots, ChannelPool) would deadlock if we held poolsMu
	// through the teardown.
	sc.poolsMu.Lock()
	pools := sc.pools
	sc.pools = nil // refuse subsequent Opens; getOrCreatePool nil-checks
	mgr := sc.configManager
	chp := sc.channelPool
	factory := sc.metricsFactory
	cancel := sc.backgroundCancel
	sc.poolsMu.Unlock()

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
	// Stop the lifecycle monitors before closing the pool so neither
	// tries to dial/replace/scale a pool that's mid-teardown. Mirrors
	// managedChannelPool.Close in bigtable/channel_pool_factory.go.
	// Safe to call under sc.poolsMu: neither Stop callback reaches back
	// into sessionClient. If that changes, hoist Stop calls above the
	// mutex — otherwise a callback that re-acquires poolsMu deadlocks.
	if sc.dsm != nil {
		sc.dsm.Stop()
	}
	if sc.connRecycler != nil {
		sc.connRecycler.Stop()
	}
	var err error
	if chp != nil {
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
		pool, err := sc.createPoolForPayload(resourceName, sessionDesc, streamFactory, payload, key)
		if err != nil {
			return nil, err
		}
		if pool == nil {
			// getOrCreatePool returns nil ONLY when Close() has already
			// snapshotted the pool set. Surface a distinct sentinel so
			// callers can tell "client closed" apart from "resource has
			// no write pool" (ErrWriteNotSupported) or "bookkeeping
			// drift" (errReadPoolNil).
			return nil, ErrSessionClientClosed
		}
		return pool, nil
	}
}

// createPoolForPayload marshals the resource-typed OpenXxxRequest
// into the transport-level OpenSessionRequest envelope, builds routing
// metadata via the descriptor's MetadataFn, and delegates to
// getOrCreatePool for cache-hit-or-construct.
func (sc *sessionClient) createPoolForPayload(
	resourceName string,
	sessionDesc *btransport.SessionDescriptor,
	streamFactory func(ctx context.Context) (btransport.Stream, error),
	payload proto.Message,
	key poolKey,
) (SessionPool, error) {
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

	min := sc.cfg.MinSessions
	if min <= 0 {
		min = defaultMinSessions
	}
	max := sc.cfg.MaxSessions
	if max <= 0 {
		max = defaultMaxSessions
	}
	return sc.getOrCreatePool(key, min, max, streamFactory, handshake, md, sessionDesc.Type), nil
}

// getOrCreatePool ports SessionManager.GetOrCreateSessionPool:
// dedups on key, mints a display name, constructs the pool, wires
// the config listener + background loops.
func (sc *sessionClient) getOrCreatePool(
	key poolKey,
	min, max int,
	streamFactory func(ctx context.Context) (btransport.Stream, error),
	openSessionRequest *btpb.OpenSessionRequest,
	md metadata.MD,
	sessionType btransport.SessionType,
) SessionPool {
	sc.poolsMu.Lock()
	// Close() sets pools=nil to refuse subsequent Opens; a nil map here
	// means teardown has already snapshotted the pool set.
	if sc.pools == nil {
		sc.poolsMu.Unlock()
		return nil
	}
	if mp, ok := sc.pools[key]; ok {
		sc.poolsMu.Unlock()
		return mp.pool
	}
	id := sc.nextPoolID.Add(1)
	poolName := fmt.Sprintf("%sPool-%d", sessionType.ProtoName(), id)
	if label := key.perm.display(); label != "" {
		poolName += " [" + label + "]"
	}
	pool := btransport.NewSessionPoolImpl(
		id,
		poolName, min, max, streamFactory, openSessionRequest, md, sessionType,
	)
	mp := &managedPool{pool: pool}
	sc.pools[key] = mp
	configManager := sc.configManager
	backgroundCtx := sc.cfg.BackgroundCtx
	sc.poolsMu.Unlock()
	// TODO(sushanb): publish-before-Start race — a concurrent second Open
	// on the same key sees mp in the map before pool.Start(backgroundCtx)
	// runs below and can Invoke on an unstarted pool. Fix in follow-up by
	// gating each managedPool with a sync.Once + <-ready channel.

	if configManager != nil {
		unregister := configManager.AddSessionPoolListener(func(config *btpb.SessionClientConfiguration_SessionPoolConfiguration) {
			pool.UpdateConfig(config)
		})
		sc.poolsMu.Lock()
		if cur, stillThere := sc.pools[key]; stillThere && cur == mp {
			mp.unregister = unregister
			sc.poolsMu.Unlock()
		} else {
			sc.poolsMu.Unlock()
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
// but buildFeatureFlagsMD reads it once at NewSessionClient. If the
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
// Kept off the SessionClient interface — consumers who need them
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
	pool SessionPool
}

// orderedPoolEntries snapshots the pools map under lock and returns
// its non-nil entries sorted by poolKey.
func (sc *sessionClient) orderedPoolEntries() []poolEntry {
	sc.poolsMu.Lock()
	entries := make([]poolEntry, 0, len(sc.pools))
	for k, mp := range sc.pools {
		if mp == nil || mp.pool == nil {
			continue
		}
		entries = append(entries, poolEntry{key: k, pool: mp.pool})
	}
	sc.poolsMu.Unlock()
	sort.Slice(entries, func(i, j int) bool { return entries[i].key.less(entries[j].key) })
	return entries
}

// ChannelPool returns the *btransport.BigtableChannelPool the
// sessionClient was constructed with, if any. Used by channelz to
// surface session-pool channel stats without leaking the interface
// through the public SessionClient API.
func (sc *sessionClient) ChannelPool() *btransport.BigtableChannelPool {
	if sc.channelPool == nil {
		return nil
	}
	bp, _ := sc.channelPool.(*btransport.BigtableChannelPool)
	return bp
}

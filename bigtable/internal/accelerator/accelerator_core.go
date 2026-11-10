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

// Package accelerator exposes an in-process grpc.ClientConnInterface that
// translates Bigtable V2 RPCs into proto-native session vRPCs handled by
// internal/session.
//
// Channel is scoped to a GCP project (and optionally a pinned instance and
// appProfile). It owns one or more session.Client instances (keyed by
// (instance, appProfile)) and a session.TableCache, and dispatches each RPC by
// resolving a per-resource session.TableAPI through the cache. Handles and
// lazily created clients are kept warm for reuse and evicted on idle TTL, so a
// long-lived daemon reclaims idle resources instead of accumulating them
// forever. Close tears down the cache (releasing cached handles' pools) and
// then all session.Client instances.
package accelerator

import (
	"bytes"
	"context"
	"errors"
	"io"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	v2pb "cloud.google.com/go/bigtable/apiv2/bigtablepb"
	"cloud.google.com/go/bigtable/internal"
	"cloud.google.com/go/bigtable/internal/accelerator/adapters"
	metrics "cloud.google.com/go/bigtable/internal/metrics"
	btopt "cloud.google.com/go/bigtable/internal/option"
	"cloud.google.com/go/bigtable/internal/session"
	btransport "cloud.google.com/go/bigtable/internal/transport"
	"google.golang.org/api/option"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	gmetadata "google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/proto"
)

// Default data-plane dial parameters. These mirror the unexported
// bigtable.prodAddr / mtlsProdAddr / Scope / clientUserAgent constants; they
// are duplicated here (rather than imported) to keep the daemon from pulling
// in the full bigtable client package, matching how internal/session already
// duplicates its feature-flag metadata to avoid the same import cycle.
const (
	prodAddr     = "bigtable.UNIVERSE_DOMAIN:443"
	mtlsProdAddr = "bigtable.mtls.googleapis.com:443"
)

// DataScope is the default OAuth scope the channel dials with when the caller
// supplies no scope override. Exported so the daemon binary can resolve ADC
// once with the same effective scope the channel would, then hand the resolved
// credentials back via option.WithCredentials (avoiding a second ADC lookup).
const DataScope = "https://www.googleapis.com/auth/bigtable.data"

var userAgent = "go-acc/v" + internal.Version

// ComposeUserAgent joins a caller-supplied user-agent prefix with the daemon's
// own token. Cross-language clients (e.g. the Python SDK) pass their own client
// user agent so the service sees both the caller and the accelerator build, e.g.
// "python-bigtable/2.1.0 go-acc/v1.38.0". Tokens are space-separated
// per the HTTP/gRPC user-agent product-token convention. An empty or blank
// prefix yields the daemon token alone.
func ComposeUserAgent(prefix string) string {
	if prefix = strings.TrimSpace(prefix); prefix == "" {
		return userAgent
	}
	return prefix + " " + userAgent
}

// Ensure Channel implements grpc.ClientConnInterface.
var _ grpc.ClientConnInterface = (*Channel)(nil)

// newSessionClient constructs the session.Client a Channel dials on
// construction. It is a package-level seam that tests override to inject a
// mock; package-level state mutation is not parallel-safe.
var newSessionClient = func(
	ctx context.Context,
	project, instance, appProfile, clientName string,
	opts ...option.ClientOption,
) (session.Client, error) {
	// Phase 1 keeps built-in client-side metrics on: a nil MetricsProvider is
	// what metrics.NewFactory treats as the default Cloud Monitoring exporter.
	var metricsProvider metrics.MetricsProvider

	// Mirror bigtable.NewClientWithConfig: derive the on-wire feature flags
	// from the REAL metrics state rather than assuming it. metrics.NewFactory
	// reports Enabled=false when the provider disables metrics or built-in
	// setup fails (client-UID / exporter init), so the advertised
	// ClientSideMetricsEnabled matches what this client can actually emit.
	//
	// session.NewClient builds the live factory from the same provider below;
	// this instance exists only to read .Enabled, so shut it down immediately.
	// The accelerator has no separate classic data path that would use a second
	// factory, and leaving it running would double-export session's metrics.
	factory, err := metrics.NewFactory(ctx, project, instance, appProfile, clientName, metricsProvider, opts...)
	if err != nil {
		return nil, err
	}
	metricsEnabled := factory.Enabled
	factory.Shutdown()

	// One FeatureFlags proto, from the same btransport source of truth the
	// classic client uses. session.NewClient reuses this single reference for
	// both the bigtable-features header and OpenSessionRequest.Flags, so the two
	// stay byte-identical (the server rejects OpenSession with INVALID_ARGUMENT
	// when they disagree).
	featureFlags := btransport.NewFeatureFlagsProto(btransport.FeatureFlagsInput{
		ClientSideMetricsEnabled: metricsEnabled,
		// Advertise direct access, matching the classic client's default
		// (isDirectAccessEnabled → !DisableDirectAccess, i.e. true).
		EnableDirectAccess: true,
	})

	return session.NewClient(ctx, project, instance, appProfile, clientName, metricsProvider, featureFlags, opts...)
}

// clientKey identifies a distinct (instance, appProfile) session.Client within
// a project-scoped Channel.
type clientKey struct {
	instance   string
	appProfile string
}

// clientLoadState coordinates single-flight lazy client creation per clientKey.
type clientLoadState struct {
	ready chan struct{}
}

// sessionClientEntry wraps a cached session.Client with its last-access
// timestamp and associated sessionTables handles so idle unpinned clients can
// evict their table handles before closing.
type sessionClientEntry struct {
	sc             session.Client
	lastAccessNano atomic.Int64
	tables         map[string]session.TableAPI
}

// Channel is an in-process grpc.ClientConnInterface backed by
// internal/session. It is scoped to a GCP project (and optionally a pinned
// instance/appProfile). When instance is omitted at construction, Channel
// lazily creates and caches one session.Client per (instance, appProfile)
// observed in incoming RPC requests.
//
// scopePrefix is "projects/<project>/instances/<instance>/" when instance is
// pinned, or "projects/<project>/instances/" when project-scoped, precomputed
// once at construction. The dispatch path validates that an incoming V2
// resource name targets this daemon's scope against it, then strips it to the
// instance ID and leaf ID(s) session.Client expects (see resourcename.go).
//
// sessionTables caches per-resource session.TableAPI handles keyed by the
// fully-qualified V2 resource name and effective app profile, so repeated RPCs
// to the same resource share one warm handle and idle resources' pools are
// reclaimed by its TTL sweeper.
type Channel struct {
	ctx        context.Context
	project    string
	instance   string
	appProfile string
	clientName string
	opts       []option.ClientOption

	scopePrefix   string
	sessionTables *session.TableCache

	clientTTL time.Duration
	nowFn     func() time.Time

	mu      sync.Mutex
	clients map[clientKey]*sessionClientEntry
	loading map[clientKey]*clientLoadState
	closed  bool
}

// NewChannel constructs a Channel scoped to project (and optionally instance
// and appProfile when non-empty). When instance is provided at construction,
// the underlying session.Client is dialed eagerly; when instance is empty,
// session.Client instances are dialed lazily per (instance, appProfile) on
// incoming RPCs and closed via Close (below).
//
// clientName is stamped as the client_name attribute on exported client-side
// metrics. The daemon passes its composed user agent here (see ComposeUserAgent),
// e.g. "python-bigtable/2.1.0 go-acc/v1.38.0", so CSM attributes the metrics to
// both the calling client library and the daemon build; an empty string keeps
// the default "go-bigtable/<version>" token.
func NewChannel(
	ctx context.Context,
	project, instance, appProfile, clientName string,
	opts ...option.ClientOption,
) (*Channel, error) {
	// session.NewClient forwards opts straight to gtransport.Dial
	// without supplying a default endpoint, so without these the dial target
	// is empty ("received empty target in Build()"). Establish the standard
	// Bigtable data-plane endpoint, scope, and user agent first, then let the
	// caller's opts override. Mirrors bigtable.NewClient's use of
	// btopt.DefaultClientOptions.
	defaultOpts, err := btopt.DefaultClientOptions(prodAddr, mtlsProdAddr, DataScope, userAgent)
	if err != nil {
		return nil, err
	}
	// DefaultClientOptions does not touch message sizes, and this dial does not
	// go through bigtable.NewClient, so without this the daemon would keep
	// gRPC's 4 MiB default receive limit — making the accelerated path reject
	// ReadRows responses the client it stands in for accepts.
	defaultOpts = append(defaultOpts, btopt.MessageSizeOptions()...)
	opts = append(defaultOpts, opts...)

	ch := &Channel{
		ctx:           ctx,
		project:       project,
		instance:      instance,
		appProfile:    appProfile,
		clientName:    clientName,
		opts:          opts,
		scopePrefix:   scopePrefixFor(project, instance),
		sessionTables: session.NewTableCache(session.DefaultTableCacheTTL, session.DefaultTableCacheSweepInterval, nil /* time.Now */),
		clients:       make(map[clientKey]*sessionClientEntry),
		loading:       make(map[clientKey]*clientLoadState),
	}
	if instance != "" {
		sc, err := newSessionClient(ctx, project, instance, appProfile, clientName, opts...)
		if err != nil {
			ch.sessionTables.Close()
			return nil, err
		}
		entry := &sessionClientEntry{
			sc:     sc,
			tables: make(map[string]session.TableAPI),
		}
		entry.lastAccessNano.Store(ch.now().UnixNano())
		ch.clients[clientKey{instance: instance, appProfile: appProfile}] = entry
	}
	return ch, nil
}

func (c *Channel) now() time.Time {
	if c.nowFn != nil {
		return c.nowFn()
	}
	return time.Now()
}

func (c *Channel) effectiveClientTTL() time.Duration {
	if c.clientTTL > 0 {
		return c.clientTTL
	}
	return session.DefaultTableCacheTTL
}

// getOrCreateSessionClient returns the session.Client for (instanceID, appProfile),
// dialing and caching a new one if needed using per-key single-flight
// coordination so warm clients return immediately without waiting on cold dials.
func (c *Channel) getOrCreateSessionClient(instanceID, appProfile string) (session.Client, error) {
	key := clientKey{instance: instanceID, appProfile: appProfile}
	for {
		c.mu.Lock()
		if c.closed {
			c.mu.Unlock()
			return nil, status.Error(codes.Unavailable, "accelerator: channel is closed")
		}
		if entry, ok := c.clients[key]; ok {
			entry.lastAccessNano.Store(c.now().UnixNano())
			sc := entry.sc
			c.mu.Unlock()
			return sc, nil
		}
		if c.loading == nil {
			c.loading = make(map[clientKey]*clientLoadState)
		}
		if ls, ok := c.loading[key]; ok {
			c.mu.Unlock()
			<-ls.ready
			continue
		}
		ls := &clientLoadState{ready: make(chan struct{})}
		c.loading[key] = ls
		c.mu.Unlock()

		var (
			sc            session.Client
			err           error
			evictedTables []session.TableAPI
			evictedScs    []session.Client
		)
		func() {
			defer func() {
				c.mu.Lock()
				delete(c.loading, key)
				c.mu.Unlock()
				close(ls.ready)
			}()

			ctx := c.ctx
			if ctx == nil {
				ctx = context.Background()
			}
			sc, err = newSessionClient(ctx, c.project, instanceID, appProfile, c.clientName, c.opts...)
			if err != nil {
				return
			}

			nowNano := c.now().UnixNano()
			cutoff := c.now().Add(-c.effectiveClientTTL()).UnixNano()

			c.mu.Lock()
			if c.closed {
				c.mu.Unlock()
				_ = sc.Close()
				sc = nil
				err = status.Error(codes.Unavailable, "accelerator: channel is closed")
				return
			}
			if c.clients == nil {
				c.clients = make(map[clientKey]*sessionClientEntry)
			}
			evictedTables, evictedScs = c.collectIdleClientsLocked(cutoff, key)
			entry := &sessionClientEntry{
				sc:     sc,
				tables: make(map[string]session.TableAPI),
			}
			entry.lastAccessNano.Store(nowNano)
			c.clients[key] = entry
			c.mu.Unlock()
		}()

		for _, tbl := range evictedTables {
			_ = tbl.Close()
		}
		for _, oldSc := range evictedScs {
			_ = oldSc.Close()
		}
		return sc, err
	}
}

func (c *Channel) collectIdleClientsLocked(cutoff int64, skipKey clientKey) ([]session.TableAPI, []session.Client) {
	pinnedKey := clientKey{instance: c.instance, appProfile: c.appProfile}
	var (
		evictedTables []session.TableAPI
		evictedScs    []session.Client
	)
	for k, entry := range c.clients {
		if k == skipKey || (c.instance != "" && k == pinnedKey) {
			continue
		}
		if entry.lastAccessNano.Load() < cutoff {
			for _, tbl := range entry.tables {
				evictedTables = append(evictedTables, tbl)
			}
			if entry.sc != nil {
				evictedScs = append(evictedScs, entry.sc)
			}
			delete(c.clients, k)
		}
	}
	return evictedTables, evictedScs
}

func (c *Channel) recordClientTable(key clientKey, cacheKey string, tbl session.TableAPI) {
	c.mu.Lock()
	if entry, ok := c.clients[key]; ok {
		if entry.tables == nil {
			entry.tables = make(map[string]session.TableAPI)
		}
		entry.tables[cacheKey] = tbl
	}
	c.mu.Unlock()
}

// resolveAppProfile returns the effective app profile for an RPC, checking
// against the channel's pinned app profile if one was configured at startup.
func (c *Channel) resolveAppProfile(reqAppProfile string) (string, error) {
	if c.appProfile != "" {
		if reqAppProfile != "" && reqAppProfile != c.appProfile {
			return "", status.Errorf(codes.InvalidArgument,
				"accelerator: request app_profile_id %q does not match this daemon's app profile (%s)",
				reqAppProfile, c.appProfile)
		}
		return c.appProfile, nil
	}
	return reqAppProfile, nil
}

// openHandle resolves an extracted resource to a per-resource, cached
// session.TableAPI based on its kind. The adapter already determined the kind
// from the populated V2 name field; here the full name is validated against
// this daemon's scope and reduced to the instance ID and leaf ID(s)
// session.Client expects, which re-prefixes them with its own project/instance.
// A name targeting a different project (or a different instance when pinned) is
// rejected rather than silently served from this daemon's scope.
//
// Validation runs before the cache lookup, so an out-of-scope name is rejected
// without ever creating or caching a handle. In-scope names resolve through the
// TableCache keyed by the full V2 name and effective app profile, so repeated
// RPCs to the same resource share one warm handle; the cache's sweeper evicts
// idle handles (releasing their pools) on TTL.
func (c *Channel) openHandle(res adapters.Resource) (session.TableAPI, error) {
	appProfile, err := c.resolveAppProfile(res.AppProfile)
	if err != nil {
		return nil, err
	}

	var (
		instanceID string
		openWith   func(session.Client) session.TableAPI
	)
	switch res.Kind {
	case adapters.ResourceTable:
		var tableID string
		instanceID, tableID, err = c.parseTableName(res.Name)
		if err != nil {
			return nil, err
		}
		openWith = func(sc session.Client) session.TableAPI { return sc.OpenTable(tableID) }
	case adapters.ResourceAuthorizedView:
		var tableID, viewID string
		instanceID, tableID, viewID, err = c.parseAuthorizedViewName(res.Name)
		if err != nil {
			return nil, err
		}
		openWith = func(sc session.Client) session.TableAPI { return sc.OpenAuthorizedView(tableID, viewID) }
	case adapters.ResourceMaterializedView:
		var viewID string
		instanceID, viewID, err = c.parseMaterializedViewName(res.Name)
		if err != nil {
			return nil, err
		}
		openWith = func(sc session.Client) session.TableAPI { return sc.OpenMaterializedView(viewID) }
	default:
		// Kind is set by the adapter from the populated V2 name field, so an
		// unrecognized kind is an internal invariant violation, not bad input.
		return nil, status.Errorf(codes.Internal, "accelerator: unknown resource kind %v", res.Kind)
	}

	sc, err := c.getOrCreateSessionClient(instanceID, appProfile)
	if err != nil {
		return nil, err
	}

	// Key on the full V2 name plus effective app profile, so table /
	// authorized-view / materialized-view keys and distinct app profiles never
	// collide. A nil handle means the cache has been Closed (Channel.Close), so
	// the channel is no longer usable.
	cacheKey := res.Name
	if appProfile != "" {
		cacheKey = res.Name + "\x00" + appProfile
	}
	tbl := c.sessionTables.GetOrOpen(cacheKey, func() session.TableAPI {
		return openWith(sc)
	})
	if tbl == nil {
		return nil, status.Error(codes.Unavailable, "accelerator: channel is closed")
	}
	c.recordClientTable(clientKey{instance: instanceID, appProfile: appProfile}, cacheKey, tbl)
	return tbl, nil
}

// Invoke implements grpc.ClientConnInterface for unary V2 RPCs.
func (c *Channel) Invoke(ctx context.Context, method string, args, reply interface{}, _ ...grpc.CallOption) error {
	switch method {
	case v2pb.Bigtable_MutateRow_FullMethodName:
		return c.mutateRowImpl(ctx, args, reply)
	default:
		// Unimplemented is the fallback signal: the method is a valid Bigtable
		// RPC this sidecar does not serve, and the caller is expected to retry
		// it over a direct connection. See validateSingleRowReadRequest.
		return status.Errorf(codes.Unimplemented, "accelerator: method %s not implemented", method)
	}
}

func (c *Channel) mutateRowImpl(ctx context.Context, args, reply interface{}) error {
	reqV2, ok := args.(*v2pb.MutateRowRequest)
	if !ok {
		return status.Errorf(codes.Internal, "accelerator: unexpected request type %T for MutateRow", args)
	}
	respV2, ok := reply.(*v2pb.MutateRowResponse)
	if !ok {
		return status.Errorf(codes.Internal, "accelerator: unexpected reply type %T for MutateRow", reply)
	}

	reqAdapter := adapters.DefaultMutateRowRequestAdapter
	resource, err := reqAdapter.ExtractResource(reqV2)
	if err != nil {
		return err
	}
	sessionReq, err := reqAdapter.Adapt(reqV2)
	if err != nil {
		return err
	}

	// Resolve a cached handle for the resource, routed to a table or authorized
	// view by the resource kind; repeated calls to the same resource reuse one
	// warm handle from the TableCache.
	tbl, err := c.openHandle(resource)
	if err != nil {
		return err
	}

	sessionResp, err := tbl.MutateRow(ctx, sessionReq)
	if err != nil {
		return err
	}

	respAdapter := adapters.DefaultMutateRowResponseAdapter
	adapted, err := respAdapter.Adapt(sessionResp)
	if err != nil {
		return err
	}
	proto.Reset(respV2)
	if adapted != nil {
		proto.Merge(respV2, adapted)
	}
	return nil
}

// NewStream implements grpc.ClientConnInterface for streaming V2 RPCs.
// ReadRows is dispatched through session.TableAPI.ReadRow lazily: the
// session call is deferred until the first RecvMsg so the consumer's pull
// rate controls when work happens.
func (c *Channel) NewStream(ctx context.Context, _ *grpc.StreamDesc, method string, _ ...grpc.CallOption) (grpc.ClientStream, error) {
	switch method {
	case v2pb.Bigtable_ReadRows_FullMethodName:
		return &readRowsClientStream{ctx: ctx, c: c}, nil
	default:
		// Unimplemented is the fallback signal: the method is a valid Bigtable
		// RPC this sidecar does not serve, and the caller is expected to retry
		// it over a direct connection. See validateSingleRowReadRequest.
		return nil, status.Errorf(codes.Unimplemented, "accelerator: streaming method %s not implemented", method)
	}
}

// Close releases resources held by the channel. It closes the per-resource
// TableCache first — stopping its sweeper and Close()ing every cached handle so
// each releases its session pools — then tears down all session.Client
// instances that own those pools.
func (c *Channel) Close() error {
	if c == nil {
		return nil
	}
	c.mu.Lock()
	if c.closed {
		c.mu.Unlock()
		return nil
	}
	c.closed = true
	clients := make([]session.Client, 0, len(c.clients))
	for _, entry := range c.clients {
		if entry != nil && entry.sc != nil {
			clients = append(clients, entry.sc)
		}
	}
	c.clients = nil
	c.mu.Unlock()

	if c.sessionTables != nil {
		c.sessionTables.Close()
	}
	var errs []error
	for _, sc := range clients {
		if err := sc.Close(); err != nil {
			errs = append(errs, err)
		}
	}
	return errors.Join(errs...)
}

// readRowsClientStream implements grpc.ClientStream for the V2 ReadRows RPC,
// dispatching to a single SessionTableApi.ReadRow call lazily on the first
// RecvMsg. Backpressure flows naturally: no session work happens until the
// consumer pulls.
//
// Concurrency contract: unlike a general google.golang.org/grpc.ClientStream,
// this stream must be driven sequentially by a single goroutine. It does NOT
// support concurrent SendMsg/RecvMsg (nor SendMsg concurrent with CloseSend):
// the send and receive paths share unsynchronized state (req, sent, done,
// terminalErr) with no locking, so concurrent access races. This narrower
// contract is deliberate — the stream is effectively unary (one SendMsg, then
// RecvMsg until EOF), and the daemon's stream interceptor (interceptors.go),
// the only production caller, always drives one stream sequentially from a
// single per-RPC goroutine, so no synchronization is warranted.
//
// State machine:
//   - awaitingSend: SendMsg has not been called.
//   - awaitingRecv: request captured; session call has not run.
//   - done: session call has completed (or errored) and the single response
//     has been delivered (or never will be). Subsequent RecvMsg returns the
//     terminal status recorded in terminalErr — io.EOF after a successful read,
//     or the error that aborted the stream — matching grpc.ClientStream, which
//     keeps reporting the terminal status on repeated RecvMsg calls.
type readRowsClientStream struct {
	ctx context.Context
	c   *Channel

	req  *v2pb.ReadRowsRequest
	sent bool
	done bool
	// terminalErr is the status a post-completion RecvMsg replays: io.EOF once
	// the single response was delivered, or the error that aborted the stream.
	// Set together with done.
	terminalErr error
}

func (s *readRowsClientStream) Header() (gmetadata.MD, error) { return nil, nil }
func (s *readRowsClientStream) Trailer() gmetadata.MD         { return nil }
func (s *readRowsClientStream) CloseSend() error              { return nil }
func (s *readRowsClientStream) Context() context.Context      { return s.ctx }

func (s *readRowsClientStream) SendMsg(m any) error {
	if s.sent {
		return status.Error(codes.Internal, "accelerator: ReadRows SendMsg called more than once")
	}
	req, ok := m.(*v2pb.ReadRowsRequest)
	if !ok {
		return status.Errorf(codes.Internal, "accelerator: unexpected request type %T for ReadRows", m)
	}
	if err := validateSingleRowReadRequest(req); err != nil {
		return err
	}
	s.req = req
	s.sent = true
	return nil
}

func (s *readRowsClientStream) RecvMsg(m any) error {
	if s.done {
		// Replay the terminal status: io.EOF after a successful read, or the
		// error that aborted the stream. Returning io.EOF here after an error
		// would falsely signal clean completion to a caller that keeps pulling.
		return s.terminalErr
	}
	if !s.sent {
		return status.Error(codes.Internal, "accelerator: ReadRows RecvMsg called before SendMsg")
	}
	resp, ok := m.(*v2pb.ReadRowsResponse)
	if !ok {
		return status.Errorf(codes.Internal, "accelerator: unexpected reply type %T for ReadRows", m)
	}

	reqAdapter := adapters.DefaultReadRowRequestAdapter
	resource, err := reqAdapter.ExtractResource(s.req)
	if err != nil {
		return s.terminate(err)
	}
	sessionReq, err := reqAdapter.Adapt(s.req)
	if err != nil {
		return s.terminate(err)
	}

	// Routed to a table, authorized view, or materialized view by the
	// resource kind the adapter extracted.
	tbl, err := s.c.openHandle(resource)
	if err != nil {
		return s.terminate(err)
	}

	sessionResp, err := tbl.ReadRow(s.ctx, sessionReq)
	if err != nil {
		return s.terminate(err)
	}

	adapted, err := adapters.DefaultReadRowResponseAdapter.Adapt(sessionResp)
	if err != nil {
		return s.terminate(err)
	}
	proto.Reset(resp)
	if adapted != nil {
		proto.Merge(resp, adapted)
	}
	// The single response was delivered; the next RecvMsg reports EOF.
	s.done = true
	s.terminalErr = io.EOF
	return nil
}

// terminate marks the stream done, records err as the terminal status so
// subsequent RecvMsg calls replay it, and returns err for this call.
func (s *readRowsClientStream) terminate(err error) error {
	s.done = true
	s.terminalErr = err
	return err
}

// validateSingleRowReadRequest rejects request shapes the session transport
// cannot express. SessionReadRow targets exactly one row by key; the only
// accepted shapes are (a) exactly one RowKey and no RowRanges, or (b) no
// RowKeys and one closed-closed RowRange whose start and end are equal
// (which pins the range to a single row). Anything broader must fail fast
// rather than silently drop rows.
//
// Fallback contract: the accelerator is a fast-path sidecar for point reads and
// writes, not a full Bigtable implementation. The status code encodes what the
// caller should do next:
//
//   - codes.Unimplemented — the request is well-formed but outside the
//     fast path (e.g. a range scan, reversed read, or multi-row read). Callers
//     are expected to treat this as a signal to transparently retry the same
//     request over a direct Bigtable connection; the operation is valid, this
//     sidecar just does not serve it.
//   - codes.InvalidArgument — the request is genuinely malformed (e.g. nil).
//     A direct backend would reject it too, so callers must not fall back or
//     retry.
//
// Preserving this distinction is why unsupported-but-valid shapes below return
// Unimplemented rather than InvalidArgument.
func validateSingleRowReadRequest(req *v2pb.ReadRowsRequest) error {
	if req == nil {
		return status.Error(codes.InvalidArgument, "accelerator: nil ReadRowsRequest")
	}
	if req.Reversed {
		return status.Error(codes.Unimplemented, "accelerator: reversed ReadRows not supported")
	}
	if req.Rows == nil {
		return status.Error(codes.Unimplemented, "accelerator: ReadRows without a single row key not supported")
	}
	nKeys, nRanges := len(req.Rows.RowKeys), len(req.Rows.RowRanges)
	switch {
	case nKeys == 1 && nRanges == 0:
		return nil
	case nKeys == 0 && nRanges == 1:
		return validateSingleRowRange(req.Rows.RowRanges[0])
	default:
		return status.Errorf(codes.Unimplemented, "accelerator: ReadRows must specify exactly one row (got %d row keys, %d row ranges)", nKeys, nRanges)
	}
}

// validateSingleRowRange accepts only a closed-closed range whose start and
// end bounds are byte-equal — the only range shape that pins to exactly one
// row.
func validateSingleRowRange(r *v2pb.RowRange) error {
	if r == nil {
		return status.Error(codes.Unimplemented, "accelerator: nil RowRange")
	}
	start, ok := r.StartKey.(*v2pb.RowRange_StartKeyClosed)
	if !ok {
		return status.Error(codes.Unimplemented, "accelerator: ReadRows RowRange must use start_key_closed")
	}
	end, ok := r.EndKey.(*v2pb.RowRange_EndKeyClosed)
	if !ok {
		return status.Error(codes.Unimplemented, "accelerator: ReadRows RowRange must use end_key_closed")
	}
	if !bytes.Equal(start.StartKeyClosed, end.EndKeyClosed) {
		return status.Error(codes.Unimplemented, "accelerator: ReadRows RowRange must have equal start and end bounds")
	}
	return nil
}

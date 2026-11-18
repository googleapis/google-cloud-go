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

package sharedcore

import (
	"bytes"
	"context"
	"io"
	"log"
	"os"
	"strings"
	"sync"
	"sync/atomic"
	"unicode"

	vkit "cloud.google.com/go/spanner/apiv1"
	sppb "cloud.google.com/go/spanner/apiv1/spannerpb"
	"cloud.google.com/go/spanner/internal/requestid"
	"cloud.google.com/go/spanner/internal/transport"
	"github.com/googleapis/gax-go/v2"
	"google.golang.org/api/option"
	gtransport "google.golang.org/api/transport/grpc"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/encoding/gzip"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
)

var sharedCoreClientIDCounter atomic.Uint32

type logWriter struct {
	logFn func(level, msg string)
}

func (w *logWriter) Write(p []byte) (int, error) {
	msg := string(bytes.TrimRight(p, "\r\n"))
	if msg != "" && w.logFn != nil {
		w.logFn("WARN", msg)
	}
	return len(p), nil
}

func newLogger(logFn func(level, msg string)) *log.Logger {
	if logFn != nil {
		return log.New(&logWriter{logFn: logFn}, "", 0)
	}
	return log.New(io.Discard, "", 0)
}

type transportSettings struct {
	rawEmulator          string
	emulatorAddr         string
	isEmulator           bool
	isPlain              bool
	isDirectPathEnabled  bool
	isFallbackEnabled    bool
	useDCP               bool
	dcpDisabledForNoAuth bool
	hasNumChannelsConfig bool
	numChannels          int
}

func resolveSettings(cfg ClientConfig) transportSettings {
	rawEmulator := os.Getenv("SPANNER_EMULATOR_HOST")
	emulatorAddr := transport.StripEndpointScheme(rawEmulator)
	endpointAddr := transport.StripEndpointScheme(cfg.Endpoint)
	isEmulator := rawEmulator != "" && (cfg.Endpoint == "" || cfg.Endpoint == rawEmulator || (emulatorAddr != "" && endpointAddr == emulatorAddr))
	isPlain := cfg.PlainText || isEmulator

	dcpRequested := cfg.DynamicChannelPoolConfig.DCPEnabled
	dcpDisabledForNoAuth := dcpRequested && !isEmulator && !isPlain && cfg.DisableBuiltInAuth
	useDCP := dcpRequested && !isEmulator && (isPlain || !cfg.DisableBuiltInAuth)

	isDirectPathEnabled := !isPlain && !cfg.DisableBuiltInAuth && transport.ResolveDirectPathEnabled(cfg.EnableDirectAccess)
	isFallbackEnabled := isDirectPathEnabled && !useDCP && transport.ResolveGCPFallbackEnabled()

	hasNumChannelsConfig := cfg.NumChannels > 0
	numCh := cfg.NumChannels
	if numCh <= 0 {
		numCh = transport.DefaultNumChannels
	}

	return transportSettings{
		rawEmulator:          rawEmulator,
		emulatorAddr:         emulatorAddr,
		isEmulator:           isEmulator,
		isPlain:              isPlain,
		isDirectPathEnabled:  isDirectPathEnabled,
		isFallbackEnabled:    isFallbackEnabled,
		useDCP:               useDCP,
		dcpDisabledForNoAuth: dcpDisabledForNoAuth,
		hasNumChannelsConfig: hasNumChannelsConfig,
		numChannels:          numCh,
	}
}

func buildClientOptions(cfg ClientConfig, settings transportSettings, extraOpts ...option.ClientOption) []option.ClientOption {
	var opts []option.ClientOption
	if settings.isEmulator {
		opts = append(opts, transport.EmulatorClientOptions(settings.rawEmulator)...)
	} else if settings.isPlain {
		if cfg.Endpoint != "" {
			addr := cfg.Endpoint
			if after, ok := strings.CutPrefix(addr, "http://"); ok {
				addr = after
			} else if after, ok := strings.CutPrefix(addr, "https://"); ok {
				addr = after
			}
			if strings.Contains(addr, "://") {
				opts = append(opts, option.WithEndpoint(addr))
			} else {
				opts = append(opts, option.WithEndpoint("passthrough:///"+addr))
			}
		}
		opts = append(opts, transport.PlainTextClientOptions()...)
	} else {
		if cfg.Endpoint != "" {
			opts = append(opts, option.WithEndpoint(cfg.Endpoint))
		}
		if cfg.DisableBuiltInAuth {
			opts = append(opts, option.WithoutAuthentication())
		}
		if cfg.UniverseDomain != "" {
			opts = append(opts, option.WithUniverseDomain(cfg.UniverseDomain))
		}
	}

	return append(opts, extraOpts...)
}

func isValidResourceSegment(s string) bool {
	if len(s) == 0 {
		return false
	}
	for _, r := range s {
		if unicode.IsSpace(r) {
			return false
		}
	}
	return true
}

// parseSessionDatabasePrefix validates the strict 8-segment Spanner session name
// "projects/<p>/instances/<i>/databases/<d>/sessions/<s>" with non-empty,
// non-whitespace segments and returns "projects/<p>/instances/<i>/databases/<d>"
// without allocating.
func parseSessionDatabasePrefix(sessionID string) (string, bool) {
	rest, ok := strings.CutPrefix(sessionID, "projects/")
	if !ok {
		return "", false
	}
	i := strings.IndexByte(rest, '/')
	if i <= 0 || !isValidResourceSegment(rest[:i]) {
		return "", false
	}
	rest, ok = strings.CutPrefix(rest[i+1:], "instances/")
	if !ok {
		return "", false
	}
	i = strings.IndexByte(rest, '/')
	if i <= 0 || !isValidResourceSegment(rest[:i]) {
		return "", false
	}
	rest, ok = strings.CutPrefix(rest[i+1:], "databases/")
	if !ok {
		return "", false
	}
	i = strings.IndexByte(rest, '/')
	if i <= 0 || !isValidResourceSegment(rest[:i]) {
		return "", false
	}
	afterDB := rest[i+1:]
	sess, ok := strings.CutPrefix(afterDB, "sessions/")
	if !ok || strings.IndexByte(sess, '/') >= 0 || !isValidResourceSegment(sess) {
		return "", false
	}
	dbLen := len(sessionID) - len(afterDB) - 1
	return sessionID[:dbLen], true
}

// applyDCPPrimeMetadata attaches the validated database resource-prefix header,
// route-to-leader header, and sessionMD (e.g. x-response-encoding: gzip) without
// mutating any existing outgoing metadata or sessionMD.
func applyDCPPrimeMetadata(ctx context.Context, sessionMD metadata.MD, dbPrefix string) context.Context {
	outMD := metadata.Pairs(
		transport.ResourcePrefixHeader, dbPrefix,
		transport.RouteToLeaderHeader, "true",
	)
	for k, vals := range sessionMD {
		lk := strings.ToLower(k)
		if lk != transport.ResourcePrefixHeader && lk != transport.RouteToLeaderHeader {
			outMD.Append(lk, vals...)
		}
	}
	if existing, ok := metadata.FromOutgoingContext(ctx); ok {
		merged := make(metadata.MD, len(existing)+len(outMD))
		for k, vals := range existing {
			merged.Append(strings.ToLower(k), vals...)
		}
		for k, vals := range outMD {
			merged[k] = vals
		}
		return metadata.NewOutgoingContext(ctx, merged)
	}
	return metadata.NewOutgoingContext(ctx, outMD)
}

// ChannelClient is the per-channel Spanner RPC client interface returned by Transport.PickClient.
// Underlying connections are owned by Transport and released via Transport.Close.
//
// Request-ID contract:
//   - ExecuteSql automatically injects x-goog-spanner-request-id unless opts already carry one.
//   - ExecuteStreamingSql does not inject a request ID; callers must obtain a *requestid.Wrap
//     from RequestIDHeaderInjector (or supply requestid.WithRequestID) and pass per-attempt
//     CallOptions so retries reuse the same logical request key.
type ChannelClient interface {
	RequestIDHeaderInjector(context.Context) (*requestid.Wrap, error)
	ExecuteSql(ctx context.Context, req *sppb.ExecuteSqlRequest, opts ...gax.CallOption) (*sppb.ResultSet, error)
	ExecuteStreamingSql(ctx context.Context, req *sppb.ExecuteSqlRequest, opts ...gax.CallOption) (sppb.Spanner_ExecuteStreamingSqlClient, error)
}

// managedChannelClient extends ChannelClient with Close for internal DCP entry lifecycle management.
type managedChannelClient interface {
	ChannelClient
	Close() error
}

type rawChannelClient struct {
	raw     *vkit.Client
	tracker requestid.Tracker
}

func newRawChannelClient(ctx context.Context, userAgent string, clientID int, channelID uint64, nthRequest *atomic.Uint32, opts ...option.ClientOption) (*rawChannelClient, error) {
	raw, err := transport.NewGAPICClient(ctx, userAgent, opts...)
	if err != nil {
		return nil, err
	}
	return &rawChannelClient{
		raw:     raw,
		tracker: requestid.NewTracker(clientID, channelID, nthRequest),
	}, nil
}

func (c *rawChannelClient) Close() error {
	return c.raw.Close()
}

func (c *rawChannelClient) RequestIDHeaderInjector(context.Context) (*requestid.Wrap, error) {
	return c.tracker.NewWrap(), nil
}

func (c *rawChannelClient) ExecuteSql(ctx context.Context, req *sppb.ExecuteSqlRequest, opts ...gax.CallOption) (*sppb.ResultSet, error) {
	return c.raw.ExecuteSql(ctx, req, c.tracker.OptsWithNextRequestID(opts)...)
}

func (c *rawChannelClient) ExecuteStreamingSql(ctx context.Context, req *sppb.ExecuteSqlRequest, opts ...gax.CallOption) (sppb.Spanner_ExecuteStreamingSqlClient, error) {
	return transport.ExecuteStreamingSQL(ctx, c.raw, req, opts...)
}

type dcpChannelClient struct {
	entry    *transport.DCPEntry[managedChannelClient]
	delegate managedChannelClient
}

func (c *dcpChannelClient) Close() error { return c.delegate.Close() }

func (c *dcpChannelClient) RequestIDHeaderInjector(ctx context.Context) (*requestid.Wrap, error) {
	return c.delegate.RequestIDHeaderInjector(ctx)
}

func (c *dcpChannelClient) ExecuteSql(ctx context.Context, req *sppb.ExecuteSqlRequest, opts ...gax.CallOption) (resp *sppb.ResultSet, err error) {
	done := c.entry.StartUnary(ctx)
	defer func() { done(err) }()
	return c.delegate.ExecuteSql(ctx, req, opts...)
}

func (c *dcpChannelClient) ExecuteStreamingSql(ctx context.Context, req *sppb.ExecuteSqlRequest, opts ...gax.CallOption) (stream sppb.Spanner_ExecuteStreamingSqlClient, err error) {
	ref := c.entry.StartStream(ctx)
	var ok bool
	defer func() {
		if !ok {
			ref.Done(err)
		}
	}()
	stream, err = c.delegate.ExecuteStreamingSql(ctx, req, opts...)
	if err != nil {
		return nil, err
	}
	ok = true
	return &transport.DCPExecuteStreamingSQLClient{Spanner_ExecuteStreamingSqlClient: stream, Ref: ref}, nil
}

type dcpResolvingChannelClient struct {
	transport.DCPResolver[managedChannelClient]
}

func newDCPResolvingChannelClient(pool *transport.DynamicChannelPool[managedChannelClient], entry *transport.DCPEntry[managedChannelClient]) *dcpResolvingChannelClient {
	c := &dcpResolvingChannelClient{
		DCPResolver: transport.DCPResolver[managedChannelClient]{Pool: pool},
	}
	c.Entry.Store(entry)
	return c
}

func (c *dcpResolvingChannelClient) Acquire(ctx context.Context) (*transport.DCPEntry[managedChannelClient], error) {
	if c == nil || c.Pool == nil {
		return nil, transport.ErrDCPNoEntries
	}
	return c.DCPResolver.Acquire(ctx)
}

func (c *dcpResolvingChannelClient) RequestIDHeaderInjector(ctx context.Context) (*requestid.Wrap, error) {
	e, err := c.Acquire(ctx)
	if err != nil {
		return nil, err
	}
	defer e.Release()
	return e.Client.RequestIDHeaderInjector(ctx)
}

func (c *dcpResolvingChannelClient) ExecuteSql(ctx context.Context, req *sppb.ExecuteSqlRequest, opts ...gax.CallOption) (*sppb.ResultSet, error) {
	e, err := c.Acquire(ctx)
	if err != nil {
		return nil, err
	}
	defer e.Release()
	return e.Client.ExecuteSql(ctx, req, opts...)
}

func (c *dcpResolvingChannelClient) ExecuteStreamingSql(ctx context.Context, req *sppb.ExecuteSqlRequest, opts ...gax.CallOption) (sppb.Spanner_ExecuteStreamingSqlClient, error) {
	e, err := c.Acquire(ctx)
	if err != nil {
		return nil, err
	}
	defer e.Release()
	return e.Client.ExecuteStreamingSql(ctx, req, opts...)
}

type primeSessionState struct {
	sessionID string
	dbPrefix  string
	stale     bool
}

// Transport manages the gRPC connection pool (fixed-mode pool, DirectPath fallback,
// or DynamicChannelPool) and per-channel GAPIC clients for the shared native core.
type Transport struct {
	closed         atomic.Bool
	pool           gtransport.ConnPool
	dcp            *transport.DynamicChannelPool[managedChannelClient]
	fallbackClient ChannelClient
	sessionMD      metadata.MD
	logger         *log.Logger
	primeMu        sync.Mutex
	primeState     atomic.Pointer[primeSessionState]
}

// NewTransport initializes a new Transport using the shared transport and requestid packages.
// ctx is used for initial connection dialing and as the parent context for DCP scale-up/drain
// operations (and GCPFallback monitoring when enabled); callers must call Close to release
// connections and stop background DCP workers.
func NewTransport(ctx context.Context, cfg ClientConfig) (*Transport, error) {
	return newTransport(ctx, cfg)
}

func newTransport(ctx context.Context, cfg ClientConfig, extraOpts ...option.ClientOption) (*Transport, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	settings := resolveSettings(cfg)
	if settings.isPlain && !settings.isEmulator && cfg.Endpoint == "" && len(extraOpts) == 0 {
		return nil, status.Error(codes.InvalidArgument, "sharedcore: PlainText requires a non-empty Endpoint or SPANNER_EMULATOR_HOST")
	}

	logger := newLogger(cfg.LogFunc)
	if settings.dcpDisabledForNoAuth {
		logger.Printf("spanner sharedcore: DynamicChannelPoolConfig is disabled when DisableBuiltInAuth is true on a non-plaintext endpoint; falling back to fixed channel pool")
	}

	opts := buildClientOptions(cfg, settings, extraOpts...)

	sessionMD := metadata.MD{}
	if cfg.Compression == gzip.Name {
		sessionMD.Append(transport.RequestsCompressionHeader, gzip.Name)
	}

	clientID := int(sharedCoreClientIDCounter.Add(1))
	nthRequest := new(atomic.Uint32)

	t := &Transport{
		sessionMD: sessionMD,
		logger:    logger,
	}

	if settings.useDCP {
		if err := t.initDCPPool(ctx, cfg, settings, clientID, nthRequest, opts); err != nil {
			return nil, err
		}
		return t, nil
	}

	if err := t.initStaticOrFallbackPool(ctx, cfg, settings, clientID, nthRequest, opts); err != nil {
		return nil, err
	}
	return t, nil
}

func (t *Transport) initDCPPool(
	ctx context.Context,
	cfg ClientConfig,
	settings transportSettings,
	clientID int,
	nthRequest *atomic.Uint32,
	opts []option.ClientOption,
) error {
	dcpPoolCfg := transport.PoolConfig{
		NumChannels:          1,
		HasNumChannelsConfig: true,
		Compression:          cfg.Compression,
		EnableDirectAccess:   settings.isDirectPathEnabled,
		DisableDirectPath:    !settings.isDirectPathEnabled,
	}
	dial := func(dialCtx context.Context) (gtransport.ConnPool, error) {
		pool, _, err := transport.DialStaticPool(dialCtx, dcpPoolCfg, opts...)
		return pool, err
	}
	dcp, err := transport.NewDynamicChannelPool[managedChannelClient](
		ctx,
		cfg.DynamicChannelPoolConfig,
		transport.DCPHooks[managedChannelClient]{
			Logger: t.logger,
			Dial:   dial,
			NewEntryClient: func(entryCtx context.Context, e *transport.DCPEntry[managedChannelClient]) error {
				rawClient, err := newRawChannelClient(entryCtx, cfg.UserAgent, clientID, e.ID, nthRequest, gtransport.WithConnPool(e))
				if err != nil {
					return err
				}
				e.Delegate = rawClient
				e.Client = &dcpChannelClient{entry: e, delegate: rawClient}
				return nil
			},
			PrimeRPC: func(primeCtx context.Context, e *transport.DCPEntry[managedChannelClient], sessionID string) error {
				dbPrefix, ok := parseSessionDatabasePrefix(sessionID)
				if !ok {
					return status.Errorf(codes.InvalidArgument, "sharedcore: invalid DCP prime session %q", sessionID)
				}
				stmt := &sppb.ExecuteSqlRequest{Session: sessionID, Sql: "SELECT 1"}
				_, err := e.Delegate.ExecuteSql(applyDCPPrimeMetadata(primeCtx, t.sessionMD, dbPrefix), stmt)
				if err != nil {
					switch status.Code(err) {
					case codes.NotFound, codes.PermissionDenied, codes.InvalidArgument, codes.FailedPrecondition:
						t.markPrimeSessionStale(sessionID)
					}
				}
				return err
			},
			NoEntriesErr: transport.ErrDCPNoEntries,
		},
	)
	if err != nil {
		return err
	}
	t.pool = dcp
	t.dcp = dcp

	if !settings.isPlain {
		for _, e := range dcp.GetEntries() {
			if e.Pool != nil {
				if conn := e.Pool.Conn(); conn != nil {
					conn.Connect()
				}
			}
		}
	}
	return nil
}

func (t *Transport) initStaticOrFallbackPool(
	ctx context.Context,
	cfg ClientConfig,
	settings transportSettings,
	clientID int,
	nthRequest *atomic.Uint32,
	opts []option.ClientOption,
) error {
	poolCfg := transport.PoolConfig{
		NumChannels:          settings.numChannels,
		HasNumChannelsConfig: settings.hasNumChannelsConfig,
		Compression:          cfg.Compression,
		EnableDirectAccess:   settings.isDirectPathEnabled,
		DisableDirectPath:    !settings.isDirectPathEnabled,
	}

	if settings.isFallbackEnabled {
		pool, _, err := transport.CreateFallbackConnPool(ctx, poolCfg, opts...)
		if err != nil {
			return err
		}
		if pool.Num() <= 0 {
			_ = pool.Close()
			return status.Error(codes.InvalidArgument, "gRPC connection pool has no connections")
		}
		fbClient, err := newRawChannelClient(ctx, cfg.UserAgent, clientID, 0, nthRequest, gtransport.WithConnPool(pool))
		if err != nil {
			_ = pool.Close()
			return err
		}
		if !settings.isPlain {
			if fbPool, ok := pool.(*transport.FallbackPool); ok && fbPool.PrimaryConn() != nil {
				primary := fbPool.PrimaryConn()
				seen := make(map[*grpc.ClientConn]struct{}, primary.Num())
				for i := 0; i < primary.Num(); i++ {
					if conn := primary.Conn(); conn != nil {
						if _, dup := seen[conn]; !dup {
							seen[conn] = struct{}{}
							conn.Connect()
						}
					}
				}
			}
		}
		t.pool = pool
		t.fallbackClient = fbClient
		return nil
	}

	pool, _, err := transport.DialStaticPool(ctx, poolCfg, opts...)
	if err != nil {
		return err
	}
	if pool.Num() <= 0 {
		_ = pool.Close()
		return status.Error(codes.InvalidArgument, "gRPC connection pool has no connections")
	}

	fixedPool, err := transport.NewFixedChannelPool[managedChannelClient](
		ctx,
		pool,
		transport.DCPHooks[managedChannelClient]{
			Logger: t.logger,
			NewEntryClient: func(entryCtx context.Context, e *transport.DCPEntry[managedChannelClient]) error {
				rawClient, err := newRawChannelClient(entryCtx, cfg.UserAgent, clientID, e.ID, nthRequest, gtransport.WithConnPool(e))
				if err != nil {
					return err
				}
				e.Delegate = rawClient
				e.Client = rawClient
				return nil
			},
			NoEntriesErr: transport.ErrDCPNoEntries,
		},
	)
	if err != nil {
		_ = pool.Close()
		return err
	}

	if !settings.isPlain {
		for _, e := range fixedPool.GetEntries() {
			if e.Pool != nil {
				if conn := e.Pool.Conn(); conn != nil {
					conn.Connect()
				}
			}
		}
	}

	t.pool = fixedPool
	t.dcp = fixedPool
	return nil
}

func (t *Transport) markPrimeSessionStale(sessionID string) {
	t.primeMu.Lock()
	defer t.primeMu.Unlock()
	cur := t.primeState.Load()
	if cur != nil && cur.sessionID == sessionID && !cur.stale {
		t.primeState.Store(&primeSessionState{
			sessionID: cur.sessionID,
			dbPrefix:  cur.dbPrefix,
			stale:     true,
		})
	}
}

// UpdateDCPPrimeSession records a multiplexed session ID used to prime newly scaled-up
// DCP channels. Callers must pass a valid multiplexed session name
// ("projects/<p>/instances/<i>/databases/<d>/sessions/<s>").
//
// Once a healthy prime session is recorded for a database on a shared multi-database
// Transport, sessions from other databases do not overwrite it (avoiding PrimeSession
// thrashing and repeated scale-up wakeups). Same-database session rotations still update
// the recorded session, and if the pinned session encounters a terminal session/database
// error during priming it is marked stale so another live database can take over.
func (t *Transport) UpdateDCPPrimeSession(sessionStr string) {
	if t == nil || t.closed.Load() || t.dcp == nil || t.dcp.IsFixed() {
		return
	}
	if cur := t.primeState.Load(); cur != nil && cur.sessionID == sessionStr {
		return
	}
	dbPrefix, ok := parseSessionDatabasePrefix(sessionStr)
	if !ok {
		return
	}
	if cur := t.primeState.Load(); cur != nil && !cur.stale && cur.dbPrefix != dbPrefix {
		return
	}

	t.primeMu.Lock()
	defer t.primeMu.Unlock()
	if t.closed.Load() {
		return
	}
	cur := t.primeState.Load()
	if cur != nil && cur.sessionID == sessionStr {
		return
	}
	if cur != nil && !cur.stale && cur.dbPrefix != dbPrefix {
		return
	}
	t.primeState.Store(&primeSessionState{
		sessionID: sessionStr,
		dbPrefix:  dbPrefix,
	})
	t.dcp.SetPrimeSession(sessionStr)
}

// PickClient selects a ChannelClient for an RPC.
//   - When dynamic channel pooling is active, it delegates to the pool's configured
//     selection strategy and returns a resolving client with drain reference counting.
//   - On a fixed pool, it uses lock-free round-robin across pre-initialized channel clients.
//   - On a DirectPath fallback pool, it returns the shared fallback channel client.
func (t *Transport) PickClient(ctx context.Context) (ChannelClient, error) {
	if t == nil || t.closed.Load() || t.pool == nil {
		return nil, status.Error(codes.FailedPrecondition, "sharedcore.Transport is closed or uninitialized")
	}
	if t.dcp != nil {
		entry, err := t.dcp.Pick(ctx)
		if err != nil {
			return nil, err
		}
		if t.dcp.IsFixed() {
			return entry.Client, nil
		}
		return newDCPResolvingChannelClient(t.dcp, entry), nil
	}
	if t.fallbackClient != nil {
		return t.fallbackClient, nil
	}
	return nil, status.Error(codes.FailedPrecondition, "sharedcore.Transport is closed or uninitialized")
}

// Close shuts down the underlying connection pool and any background DCP workers.
func (t *Transport) Close() error {
	if t == nil || !t.closed.CompareAndSwap(false, true) {
		return nil
	}
	// Drain any in-flight UpdateDCPPrimeSession before closing the pool.
	t.primeMu.Lock()
	t.primeMu.Unlock()
	if t.pool != nil {
		return t.pool.Close()
	}
	return nil
}

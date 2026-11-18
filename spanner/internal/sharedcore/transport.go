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
	"sync/atomic"

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

// PlaceholderDB is the database name used when initializing a database-agnostic
// shared core transport pool.
const PlaceholderDB = "projects/shared-core/instances/shared-core/databases/shared-core"

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

// NewLogger constructs a *log.Logger that routes messages to logFn when non-nil,
// or discards log output when logFn is nil.
func NewLogger(logFn func(level, msg string)) *log.Logger {
	if logFn != nil {
		return log.New(&logWriter{logFn: logFn}, "", 0)
	}
	return log.New(io.Discard, "", 0)
}

// stripEndpointSchemes removes leading passthrough:///, https://, and http://
// schemes (including chained prefixes) in an order-independent loop.
func stripEndpointSchemes(endpoint string) string {
	s := endpoint
	for {
		advanced := false
		for _, prefix := range []string{"passthrough:///", "https://", "http://"} {
			if after, ok := strings.CutPrefix(s, prefix); ok {
				s = after
				advanced = true
				break
			}
		}
		if !advanced {
			return s
		}
	}
}

// TransportSettings holds the resolved connection pool, endpoint, and DirectPath/DCP settings.
type TransportSettings struct {
	// EmulatorAddr is the scheme-stripped SPANNER_EMULATOR_HOST target.
	EmulatorAddr string
	// EndpointAddr is the scheme-stripped cfg.Endpoint target.
	EndpointAddr string
	// IsEmulator reports whether the resolved target is the Spanner emulator.
	IsEmulator bool
	// IsPlain reports whether the transport uses plaintext credentials (PlainText or emulator).
	IsPlain bool
	// IsDirectPathEnabled reports whether DirectPath is enabled (always false for plaintext/no-auth).
	IsDirectPathEnabled bool
	// IsFallbackEnabled reports whether DirectPath-to-CloudPath fallback is enabled.
	IsFallbackEnabled bool
	// UseDCP reports whether DynamicChannelPool is enabled for this transport.
	UseDCP bool
	// HasNumChannelsConfig reports whether cfg.NumChannels was explicitly set (> 0).
	HasNumChannelsConfig bool
	// NumChannels is the resolved default channel count (cfg.NumChannels when > 0, else transport.DefaultNumChannels).
	NumChannels int
}

// ResolveSettings resolves TransportSettings from ClientConfig and standard Spanner environment variables.
func ResolveSettings(cfg ClientConfig) TransportSettings {
	rawEmulator := os.Getenv("SPANNER_EMULATOR_HOST")
	emulatorAddr := stripEndpointSchemes(rawEmulator)
	endpointAddr := stripEndpointSchemes(cfg.Endpoint)
	isEmulator := rawEmulator != "" && (cfg.Endpoint == "" || (emulatorAddr != "" && endpointAddr == emulatorAddr))
	isPlain := cfg.PlainText || isEmulator

	var isDirectPathEnabled, isFallbackEnabled bool
	if !isPlain && !cfg.DisableBuiltInAuth {
		isDirectPathEnabled = transport.ResolveDirectPathEnabled(cfg.EnableDirectAccess)
		isFallbackEnabled = transport.ResolveGCPFallbackEnabled()
	}
	useDCP := cfg.DynamicChannelPoolConfig.Enabled && !isEmulator

	hasNumChannelsConfig := cfg.NumChannels > 0
	numCh := cfg.NumChannels
	if numCh <= 0 {
		numCh = transport.DefaultNumChannels
	}

	return TransportSettings{
		EmulatorAddr:         emulatorAddr,
		EndpointAddr:         endpointAddr,
		IsEmulator:           isEmulator,
		IsPlain:              isPlain,
		IsDirectPathEnabled:  isDirectPathEnabled,
		IsFallbackEnabled:    isFallbackEnabled,
		UseDCP:               useDCP,
		HasNumChannelsConfig: hasNumChannelsConfig,
		NumChannels:          numCh,
	}
}

// BuildClientOptions builds the base option.ClientOption slice from ClientConfig and TransportSettings.
func BuildClientOptions(cfg ClientConfig, settings TransportSettings) []option.ClientOption {
	var opts []option.ClientOption
	if settings.IsEmulator {
		opts = append(opts, transport.PlainTextClientOptions(settings.EmulatorAddr, true)...)
	} else if settings.IsPlain {
		opts = append(opts, transport.PlainTextClientOptions(settings.EndpointAddr, cfg.Endpoint != "")...)
	} else {
		if cfg.Endpoint != "" {
			opts = append(opts, option.WithEndpoint(cfg.Endpoint))
		}
		if cfg.DisableBuiltInAuth {
			opts = append(opts, option.WithoutAuthentication())
		}
	}

	if cfg.UniverseDomain != "" && !settings.IsPlain {
		opts = append(opts, option.WithUniverseDomain(cfg.UniverseDomain))
	}

	return append(opts, cfg.ClientOptions...)
}

func parseSessionDatabasePrefix(sessionID string) (string, bool) {
	const marker = "/sessions/"
	idx := strings.Index(sessionID, marker)
	if idx <= 0 || idx+len(marker) >= len(sessionID) {
		return "", false
	}
	return sessionID[:idx], true
}

// ApplyDCPPrimeMetadata attaches the database resource-prefix header (extracted from sessionID),
// route-to-leader header, and sessionMD (e.g. x-response-encoding: gzip) without mutating
// any existing outgoing metadata or sessionMD.
func ApplyDCPPrimeMetadata(ctx context.Context, sessionMD metadata.MD, sessionID string) context.Context {
	dbPrefix := PlaceholderDB
	if prefix, ok := parseSessionDatabasePrefix(sessionID); ok {
		dbPrefix = prefix
	}
	outMD := metadata.Pairs(
		transport.ResourcePrefixHeader, dbPrefix,
		transport.RouteToLeaderHeader, "true",
	)
	if len(sessionMD) > 0 {
		for k, vals := range sessionMD {
			lk := strings.ToLower(k)
			if lk != transport.ResourcePrefixHeader && lk != transport.RouteToLeaderHeader {
				outMD.Append(lk, vals...)
			}
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

func toTransportDCPConfig(cfg DynamicChannelPoolConfig) transport.DCPConfig {
	return transport.DCPConfig{
		DCPEnabled:                           cfg.Enabled,
		DCPInitialChannels:                   cfg.InitialChannels,
		DCPMinChannels:                       cfg.MinChannels,
		DCPMaxChannels:                       cfg.MaxChannels,
		DCPMaxRPCPerChannel:                  cfg.MaxRPCPerChannel,
		DCPMinRPCPerChannel:                  cfg.MinRPCPerChannel,
		DCPErrorPenaltyStep:                  cfg.ErrorPenaltyStep,
		DCPErrorPenaltyDuration:              cfg.ErrorPenaltyDuration,
		DCPScaleDownCheckInterval:            cfg.ScaleDownCheckInterval,
		DCPScaleUpCooldown:                   cfg.ScaleUpCooldown,
		DCPDownscaleConsecutiveLowLoadChecks: cfg.DownscaleConsecutiveLowLoadChecks,
		DCPMaxScaleUpPercent:                 cfg.MaxScaleUpPercent,
		DCPMaxRemoveChannels:                 cfg.MaxRemoveChannels,
		DCPDrainIdleGrace:                    cfg.DrainIdleGrace,
		DCPPrimeTimeout:                      cfg.PrimeTimeout,
		DCPPrimeMaxAttempts:                  cfg.PrimeMaxAttempts,
		DCPSelectionStrategy:                 transport.DCPSelectionStrategy(cfg.SelectionStrategy),
	}
}

// ChannelClient is the per-channel Spanner RPC client interface returned by Transport.PickClient.
// Underlying connections are owned by Transport and released via Transport.Close.
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
	if c == nil {
		return nil, status.Error(codes.Internal, "request-id header provider is unavailable")
	}
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

func (c *dcpChannelClient) ExecuteSql(ctx context.Context, req *sppb.ExecuteSqlRequest, opts ...gax.CallOption) (*sppb.ResultSet, error) {
	done := c.entry.StartUnary(ctx)
	resp, err := c.delegate.ExecuteSql(ctx, req, opts...)
	done(err)
	return resp, err
}

func (c *dcpChannelClient) ExecuteStreamingSql(ctx context.Context, req *sppb.ExecuteSqlRequest, opts ...gax.CallOption) (sppb.Spanner_ExecuteStreamingSqlClient, error) {
	ref := c.entry.StartStream(ctx)
	stream, err := c.delegate.ExecuteStreamingSql(ctx, req, opts...)
	if err != nil {
		ref.Done(err)
		return nil, err
	}
	return &transport.DCPExecuteStreamingSQLClient{Spanner_ExecuteStreamingSqlClient: stream, Ref: ref}, nil
}

type dcpResolvingChannelClient struct {
	transport.DCPResolver[managedChannelClient]
}

var errSharedCoreDCPNoEntries = status.Error(codes.Unavailable, "spanner_dcp: no available channels")

func newDCPResolvingChannelClient(pool *transport.DynamicChannelPool[managedChannelClient], entry *transport.DCPEntry[managedChannelClient]) *dcpResolvingChannelClient {
	c := &dcpResolvingChannelClient{
		DCPResolver: transport.DCPResolver[managedChannelClient]{Pool: pool},
	}
	c.Entry.Store(entry)
	return c
}

func (c *dcpResolvingChannelClient) Acquire(ctx context.Context) (*transport.DCPEntry[managedChannelClient], error) {
	if c == nil || c.Pool == nil {
		return nil, errSharedCoreDCPNoEntries
	}
	return c.DCPResolver.Acquire(ctx)
}

func (c *dcpResolvingChannelClient) RequestIDHeaderInjector(ctx context.Context) (*requestid.Wrap, error) {
	e, err := c.Acquire(ctx)
	if err != nil {
		return nil, err
	}
	defer e.Release()
	if e.Client == nil {
		return nil, errSharedCoreDCPNoEntries
	}
	return e.Client.RequestIDHeaderInjector(ctx)
}

func (c *dcpResolvingChannelClient) ExecuteSql(ctx context.Context, req *sppb.ExecuteSqlRequest, opts ...gax.CallOption) (*sppb.ResultSet, error) {
	e, err := c.Acquire(ctx)
	if err != nil {
		return nil, err
	}
	defer e.Release()
	if e.Client == nil {
		return nil, errSharedCoreDCPNoEntries
	}
	return e.Client.ExecuteSql(ctx, req, opts...)
}

func (c *dcpResolvingChannelClient) ExecuteStreamingSql(ctx context.Context, req *sppb.ExecuteSqlRequest, opts ...gax.CallOption) (sppb.Spanner_ExecuteStreamingSqlClient, error) {
	e, err := c.Acquire(ctx)
	if err != nil {
		return nil, err
	}
	defer e.Release()
	if e.Client == nil {
		return nil, errSharedCoreDCPNoEntries
	}
	return e.Client.ExecuteStreamingSql(ctx, req, opts...)
}

// Transport manages the gRPC connection pool (static, DirectPath fallback, or DynamicChannelPool)
// and per-channel GAPIC clients for the shared native core.
type Transport struct {
	closed    atomic.Bool
	pool      gtransport.ConnPool
	dcp       *transport.DynamicChannelPool[managedChannelClient]
	clients   []ChannelClient
	sessionMD metadata.MD
	logger    *log.Logger
	rrCounter atomic.Uint32
}

// NewTransport initializes a new Transport using the shared transport and requestid packages.
// ctx is used for initial connection dialing and as the parent context for DCP scale-up/drain
// operations (and GCPFallback monitoring when enabled); callers must call Close to release
// connections and stop background DCP workers.
func NewTransport(ctx context.Context, cfg ClientConfig) (*Transport, error) {
	logger := NewLogger(cfg.LogFunc)
	settings := ResolveSettings(cfg)
	opts := BuildClientOptions(cfg, settings)

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

	if settings.UseDCP {
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
	settings TransportSettings,
	clientID int,
	nthRequest *atomic.Uint32,
	opts []option.ClientOption,
) error {
	userOpts := append([]option.ClientOption(nil), opts...)
	dcpPoolCfg := transport.PoolConfig{
		NumChannels:          1,
		HasNumChannelsConfig: true,
		Compression:          cfg.Compression,
		EnableDirectAccess:   settings.IsDirectPathEnabled,
	}
	dial := func(dialCtx context.Context) (gtransport.ConnPool, error) {
		pool, _, err := transport.DialStaticPool(dialCtx, dcpPoolCfg, userOpts...)
		return pool, err
	}
	dcp, err := transport.NewDynamicChannelPool[managedChannelClient](
		ctx,
		toTransportDCPConfig(cfg.DynamicChannelPoolConfig),
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
				stmt := &sppb.ExecuteSqlRequest{Session: sessionID, Sql: "SELECT 1"}
				_, err := e.Delegate.ExecuteSql(ApplyDCPPrimeMetadata(primeCtx, t.sessionMD, sessionID), stmt)
				return err
			},
			NoEntriesErr: errSharedCoreDCPNoEntries,
		},
	)
	if err != nil {
		return err
	}
	t.pool = dcp
	t.dcp = dcp

	if !settings.IsPlain {
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
	settings TransportSettings,
	clientID int,
	nthRequest *atomic.Uint32,
	opts []option.ClientOption,
) error {
	poolCfg := transport.PoolConfig{
		NumChannels:          settings.NumChannels,
		HasNumChannelsConfig: settings.HasNumChannelsConfig,
		Compression:          cfg.Compression,
		EnableDirectAccess:   settings.IsDirectPathEnabled,
	}
	var (
		pool gtransport.ConnPool
		err  error
	)
	if settings.IsFallbackEnabled && settings.IsDirectPathEnabled {
		pool, _, err = transport.CreateFallbackConnPool(ctx, poolCfg, opts...)
	} else {
		pool, _, err = transport.DialStaticPool(ctx, poolCfg, opts...)
	}
	if err != nil {
		return err
	}
	actualPoolSize := pool.Num()
	if actualPoolSize <= 0 {
		_ = pool.Close()
		return status.Error(codes.InvalidArgument, "gRPC connection pool has no connections")
	}

	channelIDMap := make(map[*grpc.ClientConn]uint64, actualPoolSize)
	clients := make([]ChannelClient, actualPoolSize)
	for i := 0; i < actualPoolSize; i++ {
		var (
			clientOpt option.ClientOption
			channelID uint64
		)
		if conn := pool.Conn(); conn == nil {
			clientOpt = gtransport.WithConnPool(pool)
			channelID = uint64(i + 1)
		} else {
			var ok bool
			channelID, ok = channelIDMap[conn]
			if !ok {
				channelID = uint64(len(channelIDMap)) + 1
				channelIDMap[conn] = channelID
			}
			clientOpt = option.WithGRPCConn(conn)
		}
		c, err := newRawChannelClient(ctx, cfg.UserAgent, clientID, channelID, nthRequest, clientOpt)
		if err != nil {
			_ = pool.Close()
			return err
		}
		clients[i] = c
	}

	if !settings.IsPlain {
		for conn := range channelIDMap {
			if conn != nil {
				conn.Connect()
			}
		}
	}

	t.pool = pool
	t.clients = clients
	return nil
}

// UpdateDCPPrimeSession records the multiplexed session ID used to prime newly scaled-up DCP channels.
// Once a valid prime session is recorded for a database on a shared multi-database Transport,
// sessions from other databases do not overwrite it (avoiding PrimeSession thrashing and
// repeated scaleUpWorker wakeups), while session rotations for the same database still update it.
func (t *Transport) UpdateDCPPrimeSession(sessionStr string) {
	if t == nil || t.dcp == nil {
		return
	}
	newDB, ok := parseSessionDatabasePrefix(sessionStr)
	if !ok {
		return
	}
	prev := t.dcp.PrimeSessionID()
	if prev == sessionStr {
		return
	}
	if prev != "" {
		if prevDB, ok := parseSessionDatabasePrefix(prev); ok && prevDB != newDB {
			return
		}
	}
	t.dcp.SetPrimeSession(sessionStr)
}

// PickClient selects a ChannelClient for an RPC.
//   - When DynamicChannelPool is active (t.dcp != nil), it delegates to t.dcp.Pick(ctx)
//     and returns a resolving client that uses the configured DCP selection strategy
//     and drain reference counting.
//   - On a static pool (t.dcp == nil), it uses atomic round-robin across the
//     pre-initialized t.clients slice.
func (t *Transport) PickClient(ctx context.Context) (ChannelClient, error) {
	if t == nil || t.closed.Load() || t.pool == nil {
		return nil, status.Error(codes.FailedPrecondition, "sharedcore.Transport is closed")
	}
	if t.dcp != nil {
		entry, err := t.dcp.Pick(ctx)
		if err != nil {
			return nil, err
		}
		return newDCPResolvingChannelClient(t.dcp, entry), nil
	}
	if len(t.clients) == 0 {
		return nil, status.Error(codes.FailedPrecondition, "sharedcore.Transport is closed")
	}
	if len(t.clients) == 1 {
		return t.clients[0], nil
	}
	idx := (t.rrCounter.Add(1) - 1) % uint32(len(t.clients))
	return t.clients[idx], nil
}

// Close shuts down the underlying connection pool and any background DCP workers.
func (t *Transport) Close() error {
	if t == nil || !t.closed.CompareAndSwap(false, true) {
		return nil
	}
	if t.pool != nil {
		return t.pool.Close()
	}
	return nil
}

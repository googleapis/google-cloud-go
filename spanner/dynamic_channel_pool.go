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

package spanner

import (
	"context"
	"time"

	vkit "cloud.google.com/go/spanner/apiv1"
	"cloud.google.com/go/spanner/apiv1/spannerpb"
	"cloud.google.com/go/spanner/internal/transport"
	"github.com/googleapis/gax-go/v2"
	"go.opentelemetry.io/otel/metric"
	gtransport "google.golang.org/api/transport/grpc"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
)

// DynamicChannelSelectionStrategy controls how DCP chooses an active channel.
type DynamicChannelSelectionStrategy int

const (
	// DCPPowerOfTwoLeastBusy compares two random active channels and returns the
	// channel with lower picker load, including any active error penalty. It falls
	// back to a full scan if random picks only find draining entries.
	DCPPowerOfTwoLeastBusy DynamicChannelSelectionStrategy = iota
	// DCPRoundRobin cycles through active channels and skips draining entries.
	DCPRoundRobin
)

// DynamicChannelPoolConfig holds the knobs for Spanner dynamic channel pool.
// Zero values use DefaultDynamicChannelPoolConfig unless noted otherwise.
type DynamicChannelPoolConfig struct {
	DCPEnabled         bool // DCPEnabled opts the client into dynamic channel pool.
	DCPInitialChannels int  // DCPInitialChannels is the number of channels created at client startup.
	DCPMinChannels     int  // DCPMinChannels is the lower bound retained during scale-down.
	DCPMaxChannels     int  // DCPMaxChannels is the upper bound created during scale-up.

	// DCPMaxRPCPerChannel triggers event-driven scale-up when per-channel load or
	// average load exceeds this value.
	DCPMaxRPCPerChannel float64
	// DCPMinRPCPerChannel is the low-load threshold used by scale-down checks.
	DCPMinRPCPerChannel float64
	// DCPErrorPenaltyStep is the picker load added for each qualifying error. Zero
	// defaults to 5, or to ceil(DCPMaxRPCPerChannel) when that is smaller. An
	// explicitly set step must not exceed ceil(DCPMaxRPCPerChannel). One transient
	// error therefore biases selection only slightly; repeated errors within the
	// penalty window accumulate up to ceil(DCPMaxRPCPerChannel), so a persistently
	// failing channel looks exactly as busy as a full one.
	DCPErrorPenaltyStep int32
	// DCPErrorPenaltyDuration controls the penalty window. Every qualifying error
	// restarts the window, and the accumulated penalty expires to zero when it
	// lapses. Zero defaults to 5 seconds; negative disables the penalty. Active
	// penalty counts toward scale-up so lost capacity can be replaced, while
	// scale-down and draining deliberately ignore it.
	DCPErrorPenaltyDuration time.Duration

	DCPScaleDownCheckInterval            time.Duration // DCPScaleDownCheckInterval controls periodic downscale evaluation.
	DCPScaleUpCooldown                   time.Duration // DCPScaleUpCooldown prevents repeated scale-up bursts.
	DCPDownscaleConsecutiveLowLoadChecks int           // DCPDownscaleConsecutiveLowLoadChecks debounces scale-down.
	DCPMaxScaleUpPercent                 int           // DCPMaxScaleUpPercent caps channels added per scale-up event.
	DCPMaxRemoveChannels                 int           // DCPMaxRemoveChannels caps channels marked draining per scale-down.
	// DCPDrainIdleGrace keeps an idle drained entry briefly before close. The
	// effective idle time is never below 15 seconds: Spanner aborts a read-write
	// transaction that is idle for about 10 seconds, so the floor keeps a live
	// transaction's channel open between its statements.
	DCPDrainIdleGrace    time.Duration
	DCPPrimeTimeout      time.Duration // DCPPrimeTimeout bounds the SELECT 1 priming attempt for scaled-up channels.
	DCPPrimeMaxAttempts  int           // DCPPrimeMaxAttempts bounds scaled-up channel priming retries.
	DCPSelectionStrategy DynamicChannelSelectionStrategy
}

func toTransportDCPConfig(cfg DynamicChannelPoolConfig) transport.DCPConfig {
	return transport.DCPConfig{
		DCPEnabled:                           cfg.DCPEnabled,
		DCPInitialChannels:                   cfg.DCPInitialChannels,
		DCPMinChannels:                       cfg.DCPMinChannels,
		DCPMaxChannels:                       cfg.DCPMaxChannels,
		DCPMaxRPCPerChannel:                  cfg.DCPMaxRPCPerChannel,
		DCPMinRPCPerChannel:                  cfg.DCPMinRPCPerChannel,
		DCPErrorPenaltyStep:                  cfg.DCPErrorPenaltyStep,
		DCPErrorPenaltyDuration:              cfg.DCPErrorPenaltyDuration,
		DCPScaleDownCheckInterval:            cfg.DCPScaleDownCheckInterval,
		DCPScaleUpCooldown:                   cfg.DCPScaleUpCooldown,
		DCPDownscaleConsecutiveLowLoadChecks: cfg.DCPDownscaleConsecutiveLowLoadChecks,
		DCPMaxScaleUpPercent:                 cfg.DCPMaxScaleUpPercent,
		DCPMaxRemoveChannels:                 cfg.DCPMaxRemoveChannels,
		DCPDrainIdleGrace:                    cfg.DCPDrainIdleGrace,
		DCPPrimeTimeout:                      cfg.DCPPrimeTimeout,
		DCPPrimeMaxAttempts:                  cfg.DCPPrimeMaxAttempts,
		DCPSelectionStrategy:                 transport.DCPSelectionStrategy(cfg.DCPSelectionStrategy),
	}
}

func fromTransportDCPConfig(cfg transport.DCPConfig) DynamicChannelPoolConfig {
	return DynamicChannelPoolConfig{
		DCPEnabled:                           cfg.DCPEnabled,
		DCPInitialChannels:                   cfg.DCPInitialChannels,
		DCPMinChannels:                       cfg.DCPMinChannels,
		DCPMaxChannels:                       cfg.DCPMaxChannels,
		DCPMaxRPCPerChannel:                  cfg.DCPMaxRPCPerChannel,
		DCPMinRPCPerChannel:                  cfg.DCPMinRPCPerChannel,
		DCPErrorPenaltyStep:                  cfg.DCPErrorPenaltyStep,
		DCPErrorPenaltyDuration:              cfg.DCPErrorPenaltyDuration,
		DCPScaleDownCheckInterval:            cfg.DCPScaleDownCheckInterval,
		DCPScaleUpCooldown:                   cfg.DCPScaleUpCooldown,
		DCPDownscaleConsecutiveLowLoadChecks: cfg.DCPDownscaleConsecutiveLowLoadChecks,
		DCPMaxScaleUpPercent:                 cfg.DCPMaxScaleUpPercent,
		DCPMaxRemoveChannels:                 cfg.DCPMaxRemoveChannels,
		DCPDrainIdleGrace:                    cfg.DCPDrainIdleGrace,
		DCPPrimeTimeout:                      cfg.DCPPrimeTimeout,
		DCPPrimeMaxAttempts:                  cfg.DCPPrimeMaxAttempts,
		DCPSelectionStrategy:                 DynamicChannelSelectionStrategy(cfg.DCPSelectionStrategy),
	}
}

// DefaultDynamicChannelPoolConfig returns the default DCP settings.
func DefaultDynamicChannelPoolConfig() DynamicChannelPoolConfig {
	return fromTransportDCPConfig(transport.DefaultDCPConfig())
}

type dynamicChannelPool = transport.DynamicChannelPool[spannerClient]
type dcpEntry = transport.DCPEntry[spannerClient]
type dcpStreamRef = transport.DCPStreamRef
type dcpExecuteStreamingSqlClient = transport.DCPExecuteStreamingSQLClient

var errDCPNoEntries = spannerErrorf(codes.Unavailable, "spanner_dcp: no available channels")
var errDCPPrimeNotReady = spannerErrorf(codes.FailedPrecondition, "spanner_dcp: cannot prime channel before multiplexed session is available")

// newDynamicChannelPool returns a dynamic channel pool that dials via dial.
// When fallback is non-nil, entries fall back from DirectPath to CloudPath and
// the pool closes fallback when it closes.
func newDynamicChannelPool(
	ctx context.Context,
	sc *sessionClient,
	cfg DynamicChannelPoolConfig,
	mp metric.MeterProvider,
	builtinMetricsCfg channelPoolBuiltinMetricsConfig,
	dial func(context.Context) (gtransport.ConnPool, error),
	fallback *directPathFallback,
) (*dynamicChannelPool, error) {
	tcfg, err := transport.NormalizeDCPConfig(toTransportDCPConfig(cfg))
	if err != nil {
		return nil, err
	}
	hooks := transport.DCPHooks[spannerClient]{
		Logger: sc.logger,
		Dial: func(dialCtx context.Context) (gtransport.ConnPool, error) {
			slot, err := dialChannelSlot(dialCtx, dial, fallback, tcfg.DCPPrimeTimeout)
			if err != nil {
				return nil, err
			}
			return slot, nil
		},
		NewEntryClient: func(entryCtx context.Context, e *dcpEntry) error {
			client, err := newGRPCSpannerClient(entryCtx, sc, e.ID, gtransport.WithConnPool(e))
			if err != nil {
				return err
			}
			e.Delegate = client
			e.Client = &dcpSpannerClient{entry: e, delegate: client}
			return nil
		},
		// Verify scaled-up channels via SELECT 1 through the new entry's delegate
		// so failed channels are never published to normal request picking.
		PrimeRPC: func(primeCtx context.Context, e *dcpEntry, sessionID string) error {
			stmt := &spannerpb.ExecuteSqlRequest{Session: sessionID, Sql: "SELECT 1"}
			_, err := e.Delegate.ExecuteSql(contextWithOutgoingMetadata(withoutFallbackAccounting(primeCtx), sc.md, sc.disableRouteToLeader), stmt)
			return err
		},
		InitMetrics: func(p *dynamicChannelPool) transport.DCPMetricsRecorder {
			return newDCPMetrics(p, sc, mp)
		},
		NoEntriesErr:     errDCPNoEntries,
		PrimeNotReadyErr: errDCPPrimeNotReady,
	}
	builtinMetrics := newChannelPoolBuiltinMetrics(sc.logger, builtinMetricsCfg, fallback)
	if builtinMetrics != nil && builtinMetrics.channelLoad != nil {
		hooks.OnCallStart = builtinMetrics.recordChannelLoad
	}
	if fallback != nil {
		hooks.PenaltyEpoch = func() uint64 { return fallback.current().generation }
	}
	if builtinMetrics != nil || fallback != nil {
		hooks.OnClose = func() {
			builtinMetrics.close()
			fallback.close()
		}
	}
	p, err := transport.NewDynamicChannelPool[spannerClient](ctx, tcfg, hooks)
	if err != nil {
		return nil, err
	}
	builtinMetrics.registerChannelCount(p)
	if fallback != nil {
		// Penalties earned on DirectPath say nothing about CloudPath.
		fallback.addSwitchHook(p.ClearStaleErrorPenalties)
	}
	return p, nil
}

// newFixedChannelPool returns a fixed pool with one entry per connection of
// underlying. The pool closes underlying and, if non-nil, fallback when
// closed.
func newFixedChannelPool(
	ctx context.Context,
	sc *sessionClient,
	underlying gtransport.ConnPool,
	fallback *directPathFallback,
) (*dynamicChannelPool, error) {
	hooks := transport.DCPHooks[spannerClient]{
		Logger: sc.logger,
		NewEntryClient: func(entryCtx context.Context, e *dcpEntry) error {
			client, err := newGRPCSpannerClient(entryCtx, sc, e.ID, gtransport.WithConnPool(e))
			if err != nil {
				return err
			}
			// Wrap e.Pool only after client creation succeeds so an error leaves
			// the underlying pool untouched.
			e.Pool = newChannelSlot(e.Pool, fallback)
			e.Delegate = client
			e.Client = client
			return nil
		},
		NoEntriesErr: errDCPNoEntries,
	}
	if fallback != nil {
		hooks.OnClose = fallback.close
	}
	return transport.NewFixedChannelPool[spannerClient](ctx, underlying, hooks)
}

// TODO: Investigate replacing dcpSpannerClient and transport.DCPConnPoolTrackedStream with
// per-entry gRPC unary/stream client interceptors injected when dialing each DCP
// entry. The interceptors could track load and trigger scale-up for both the
// ConnPool path and spannerClient path, avoiding per-RPC wrapper methods.
type dcpSpannerClient struct {
	entry    *dcpEntry
	delegate spannerClient
}

func (c *dcpSpannerClient) CallOptions() *vkit.CallOptions { return c.delegate.CallOptions() }
func (c *dcpSpannerClient) Close() error                   { return c.delegate.Close() }
func (c *dcpSpannerClient) Connection() *grpc.ClientConn   { return c.delegate.Connection() }

func (c *dcpSpannerClient) startUnary(ctx context.Context) func(error) {
	return c.entry.StartUnary(ctx)
}

func (c *dcpSpannerClient) startStream(ctx context.Context) *dcpStreamRef {
	return c.entry.StartStream(ctx)
}

func (c *dcpSpannerClient) CreateSession(ctx context.Context, req *spannerpb.CreateSessionRequest, opts ...gax.CallOption) (*spannerpb.Session, error) {
	done := c.startUnary(ctx)
	resp, err := c.delegate.CreateSession(ctx, req, opts...)
	done(err)
	return resp, err
}

func (c *dcpSpannerClient) BatchCreateSessions(ctx context.Context, req *spannerpb.BatchCreateSessionsRequest, opts ...gax.CallOption) (*spannerpb.BatchCreateSessionsResponse, error) {
	done := c.startUnary(ctx)
	resp, err := c.delegate.BatchCreateSessions(ctx, req, opts...)
	done(err)
	return resp, err
}

func (c *dcpSpannerClient) GetSession(ctx context.Context, req *spannerpb.GetSessionRequest, opts ...gax.CallOption) (*spannerpb.Session, error) {
	done := c.startUnary(ctx)
	resp, err := c.delegate.GetSession(ctx, req, opts...)
	done(err)
	return resp, err
}

func (c *dcpSpannerClient) ListSessions(ctx context.Context, req *spannerpb.ListSessionsRequest, opts ...gax.CallOption) *vkit.SessionIterator {
	iter := c.delegate.ListSessions(ctx, req, opts...)
	if iter != nil && iter.InternalFetch != nil {
		fetch := iter.InternalFetch
		iter.InternalFetch = func(pageSize int, pageToken string) ([]*spannerpb.Session, string, error) {
			// Pages are fetched after the call that built the iterator returned.
			if !c.entry.Acquire() {
				return nil, "", errDCPNoEntries
			}
			defer c.entry.Release()
			done := c.startUnary(ctx)
			results, nextPageToken, err := fetch(pageSize, pageToken)
			done(err)
			return results, nextPageToken, err
		}
	}
	return iter
}

func (c *dcpSpannerClient) DeleteSession(ctx context.Context, req *spannerpb.DeleteSessionRequest, opts ...gax.CallOption) error {
	done := c.startUnary(ctx)
	err := c.delegate.DeleteSession(ctx, req, opts...)
	done(err)
	return err
}

func (c *dcpSpannerClient) ExecuteSql(ctx context.Context, req *spannerpb.ExecuteSqlRequest, opts ...gax.CallOption) (*spannerpb.ResultSet, error) {
	done := c.startUnary(ctx)
	resp, err := c.delegate.ExecuteSql(ctx, req, opts...)
	done(err)
	return resp, err
}

func (c *dcpSpannerClient) ExecuteStreamingSql(ctx context.Context, req *spannerpb.ExecuteSqlRequest, opts ...gax.CallOption) (spannerpb.Spanner_ExecuteStreamingSqlClient, error) {
	ref := c.startStream(ctx)
	stream, err := c.delegate.ExecuteStreamingSql(ctx, req, opts...)
	if err != nil {
		ref.Done(err)
		return nil, err
	}
	return &dcpExecuteStreamingSqlClient{Spanner_ExecuteStreamingSqlClient: stream, Ref: ref}, nil
}

func (c *dcpSpannerClient) ExecuteBatchDml(ctx context.Context, req *spannerpb.ExecuteBatchDmlRequest, opts ...gax.CallOption) (*spannerpb.ExecuteBatchDmlResponse, error) {
	done := c.startUnary(ctx)
	resp, err := c.delegate.ExecuteBatchDml(ctx, req, opts...)
	done(err)
	return resp, err
}

func (c *dcpSpannerClient) Read(ctx context.Context, req *spannerpb.ReadRequest, opts ...gax.CallOption) (*spannerpb.ResultSet, error) {
	done := c.startUnary(ctx)
	resp, err := c.delegate.Read(ctx, req, opts...)
	done(err)
	return resp, err
}

func (c *dcpSpannerClient) StreamingRead(ctx context.Context, req *spannerpb.ReadRequest, opts ...gax.CallOption) (spannerpb.Spanner_StreamingReadClient, error) {
	ref := c.startStream(ctx)
	stream, err := c.delegate.StreamingRead(ctx, req, opts...)
	if err != nil {
		ref.Done(err)
		return nil, err
	}
	return &dcpStreamingReadClient{Spanner_StreamingReadClient: stream, ref: ref}, nil
}

func (c *dcpSpannerClient) BeginTransaction(ctx context.Context, req *spannerpb.BeginTransactionRequest, opts ...gax.CallOption) (*spannerpb.Transaction, error) {
	done := c.startUnary(ctx)
	resp, err := c.delegate.BeginTransaction(ctx, req, opts...)
	done(err)
	return resp, err
}

func (c *dcpSpannerClient) Commit(ctx context.Context, req *spannerpb.CommitRequest, opts ...gax.CallOption) (*spannerpb.CommitResponse, error) {
	done := c.startUnary(ctx)
	resp, err := c.delegate.Commit(ctx, req, opts...)
	done(err)
	return resp, err
}

func (c *dcpSpannerClient) Rollback(ctx context.Context, req *spannerpb.RollbackRequest, opts ...gax.CallOption) error {
	done := c.startUnary(ctx)
	err := c.delegate.Rollback(ctx, req, opts...)
	done(err)
	return err
}

func (c *dcpSpannerClient) PartitionQuery(ctx context.Context, req *spannerpb.PartitionQueryRequest, opts ...gax.CallOption) (*spannerpb.PartitionResponse, error) {
	done := c.startUnary(ctx)
	resp, err := c.delegate.PartitionQuery(ctx, req, opts...)
	done(err)
	return resp, err
}

func (c *dcpSpannerClient) PartitionRead(ctx context.Context, req *spannerpb.PartitionReadRequest, opts ...gax.CallOption) (*spannerpb.PartitionResponse, error) {
	done := c.startUnary(ctx)
	resp, err := c.delegate.PartitionRead(ctx, req, opts...)
	done(err)
	return resp, err
}

func (c *dcpSpannerClient) BatchWrite(ctx context.Context, req *spannerpb.BatchWriteRequest, opts ...gax.CallOption) (spannerpb.Spanner_BatchWriteClient, error) {
	ref := c.startStream(ctx)
	stream, err := c.delegate.BatchWrite(ctx, req, opts...)
	if err != nil {
		ref.Done(err)
		return nil, err
	}
	return &dcpBatchWriteClient{Spanner_BatchWriteClient: stream, ref: ref}, nil
}

type dcpStreamingReadClient struct {
	spannerpb.Spanner_StreamingReadClient
	ref *dcpStreamRef
}

func (c *dcpStreamingReadClient) Recv() (*spannerpb.PartialResultSet, error) {
	resp, err := c.Spanner_StreamingReadClient.Recv()
	if err != nil {
		c.ref.Done(err)
	}
	return resp, err
}

// RecvMsg is Recv for callers that receive the PartialResultSet into a message
// of another type. See vtproto_stream.go.
func (c *dcpStreamingReadClient) RecvMsg(m any) error {
	err := c.Spanner_StreamingReadClient.RecvMsg(m)
	if err != nil {
		c.ref.Done(err)
	}
	return err
}

func (c *dcpStreamingReadClient) CloseSend() error {
	err := c.Spanner_StreamingReadClient.CloseSend()
	if err != nil {
		c.ref.Done(err)
	}
	// Successful CloseSend only half-closes the client send side. The stream is
	// still active until Recv returns a terminal error/EOF or the context is
	// canceled, so do not release stream load here.
	return err
}

type dcpBatchWriteClient struct {
	spannerpb.Spanner_BatchWriteClient
	ref *dcpStreamRef
}

func (c *dcpBatchWriteClient) Recv() (*spannerpb.BatchWriteResponse, error) {
	resp, err := c.Spanner_BatchWriteClient.Recv()
	if err != nil {
		c.ref.Done(err)
	}
	return resp, err
}

func (c *dcpBatchWriteClient) CloseSend() error {
	err := c.Spanner_BatchWriteClient.CloseSend()
	if err != nil {
		c.ref.Done(err)
	}
	// Successful CloseSend only half-closes the client send side. The stream is
	// still active until Recv returns a terminal error/EOF or the context is
	// canceled, so do not release stream load here.
	return err
}

// dcpResolvingSpannerClient keeps using its bound entry while it is active or
// draining, so all operations of a read-write transaction stay on one channel
// through a scale-down. It re-picks only once the bound entry is closed.
//
// TODO: Investigate replacing this per-RPC resolving wrapper with a generic
// interceptor-based approach after the DCP load-tracking interceptor follow-up
// is designed.
type dcpResolvingSpannerClient struct {
	transport.DCPResolver[spannerClient]
}

func newDCPResolvingSpannerClient(pool *dynamicChannelPool, entry *dcpEntry) *dcpResolvingSpannerClient {
	c := &dcpResolvingSpannerClient{
		DCPResolver: transport.DCPResolver[spannerClient]{Pool: pool},
	}
	c.Entry.Store(entry)
	return c
}

// Acquire guards a nil outer pointer or nil Pool (returning spanner's
// errDCPNoEntries) before delegating to the embedded DCPResolver.Acquire.
func (c *dcpResolvingSpannerClient) Acquire(ctx context.Context) (*dcpEntry, error) {
	if c == nil || c.Pool == nil {
		return nil, errDCPNoEntries
	}
	return c.DCPResolver.Acquire(ctx)
}

// Resolve returns the bound entry's client for callers that start no RPC,
// preserving Acquire's nil-receiver/nil-pool errDCPNoEntries handling.
func (c *dcpResolvingSpannerClient) Resolve(ctx context.Context) (spannerClient, error) {
	e, err := c.Acquire(ctx)
	if err != nil {
		return nil, err
	}
	e.Release()
	return e.Client, nil
}

func (c *dcpResolvingSpannerClient) CallOptions() *vkit.CallOptions {
	client, err := c.Resolve(context.Background())
	if err != nil || client == nil {
		return &vkit.CallOptions{}
	}
	return client.CallOptions()
}

// Close is intentionally a no-op. The resolver is a session-handle view over a
// DCP entry; dynamicChannelPool owns and closes the underlying clients.
func (c *dcpResolvingSpannerClient) Close() error { return nil }

func (c *dcpResolvingSpannerClient) Connection() *grpc.ClientConn {
	client, err := c.Resolve(context.Background())
	if err != nil || client == nil {
		return nil
	}
	return client.Connection()
}

func (c *dcpResolvingSpannerClient) requestIDHeaderInjector(ctx context.Context) (*requestIDWrap, error) {
	client, err := c.Resolve(ctx)
	if err != nil {
		return nil, err
	}
	if client == nil {
		return nil, errDCPNoEntries
	}
	gsc := asGRPCSpannerClient(client)
	if gsc == nil {
		return nil, spannerErrorf(codes.Internal, "request-id header provider is unavailable for %T", client)
	}
	return gsc.generateRequestIDHeaderInjector(), nil
}

func (c *dcpResolvingSpannerClient) CreateSession(ctx context.Context, req *spannerpb.CreateSessionRequest, opts ...gax.CallOption) (*spannerpb.Session, error) {
	e, err := c.Acquire(ctx)
	if err != nil {
		return nil, err
	}
	defer e.Release()
	return e.Client.CreateSession(ctx, req, opts...)
}

func (c *dcpResolvingSpannerClient) BatchCreateSessions(ctx context.Context, req *spannerpb.BatchCreateSessionsRequest, opts ...gax.CallOption) (*spannerpb.BatchCreateSessionsResponse, error) {
	e, err := c.Acquire(ctx)
	if err != nil {
		return nil, err
	}
	defer e.Release()
	return e.Client.BatchCreateSessions(ctx, req, opts...)
}

func (c *dcpResolvingSpannerClient) GetSession(ctx context.Context, req *spannerpb.GetSessionRequest, opts ...gax.CallOption) (*spannerpb.Session, error) {
	e, err := c.Acquire(ctx)
	if err != nil {
		return nil, err
	}
	defer e.Release()
	return e.Client.GetSession(ctx, req, opts...)
}

func (c *dcpResolvingSpannerClient) ListSessions(ctx context.Context, req *spannerpb.ListSessionsRequest, opts ...gax.CallOption) *vkit.SessionIterator {
	e, err := c.Acquire(ctx)
	if err != nil {
		return &vkit.SessionIterator{}
	}
	defer e.Release()
	return e.Client.ListSessions(ctx, req, opts...)
}

func (c *dcpResolvingSpannerClient) DeleteSession(ctx context.Context, req *spannerpb.DeleteSessionRequest, opts ...gax.CallOption) error {
	e, err := c.Acquire(ctx)
	if err != nil {
		return err
	}
	defer e.Release()
	return e.Client.DeleteSession(ctx, req, opts...)
}

func (c *dcpResolvingSpannerClient) ExecuteSql(ctx context.Context, req *spannerpb.ExecuteSqlRequest, opts ...gax.CallOption) (*spannerpb.ResultSet, error) {
	e, err := c.Acquire(ctx)
	if err != nil {
		return nil, err
	}
	defer e.Release()
	return e.Client.ExecuteSql(ctx, req, opts...)
}

func (c *dcpResolvingSpannerClient) ExecuteStreamingSql(ctx context.Context, req *spannerpb.ExecuteSqlRequest, opts ...gax.CallOption) (spannerpb.Spanner_ExecuteStreamingSqlClient, error) {
	e, err := c.Acquire(ctx)
	if err != nil {
		return nil, err
	}
	defer e.Release()
	return e.Client.ExecuteStreamingSql(ctx, req, opts...)
}

func (c *dcpResolvingSpannerClient) ExecuteBatchDml(ctx context.Context, req *spannerpb.ExecuteBatchDmlRequest, opts ...gax.CallOption) (*spannerpb.ExecuteBatchDmlResponse, error) {
	e, err := c.Acquire(ctx)
	if err != nil {
		return nil, err
	}
	defer e.Release()
	return e.Client.ExecuteBatchDml(ctx, req, opts...)
}

func (c *dcpResolvingSpannerClient) Read(ctx context.Context, req *spannerpb.ReadRequest, opts ...gax.CallOption) (*spannerpb.ResultSet, error) {
	e, err := c.Acquire(ctx)
	if err != nil {
		return nil, err
	}
	defer e.Release()
	return e.Client.Read(ctx, req, opts...)
}

func (c *dcpResolvingSpannerClient) StreamingRead(ctx context.Context, req *spannerpb.ReadRequest, opts ...gax.CallOption) (spannerpb.Spanner_StreamingReadClient, error) {
	e, err := c.Acquire(ctx)
	if err != nil {
		return nil, err
	}
	defer e.Release()
	return e.Client.StreamingRead(ctx, req, opts...)
}

func (c *dcpResolvingSpannerClient) BeginTransaction(ctx context.Context, req *spannerpb.BeginTransactionRequest, opts ...gax.CallOption) (*spannerpb.Transaction, error) {
	e, err := c.Acquire(ctx)
	if err != nil {
		return nil, err
	}
	defer e.Release()
	return e.Client.BeginTransaction(ctx, req, opts...)
}

func (c *dcpResolvingSpannerClient) Commit(ctx context.Context, req *spannerpb.CommitRequest, opts ...gax.CallOption) (*spannerpb.CommitResponse, error) {
	e, err := c.Acquire(ctx)
	if err != nil {
		return nil, err
	}
	defer e.Release()
	return e.Client.Commit(ctx, req, opts...)
}

func (c *dcpResolvingSpannerClient) Rollback(ctx context.Context, req *spannerpb.RollbackRequest, opts ...gax.CallOption) error {
	e, err := c.Acquire(ctx)
	if err != nil {
		return err
	}
	defer e.Release()
	return e.Client.Rollback(ctx, req, opts...)
}

func (c *dcpResolvingSpannerClient) PartitionQuery(ctx context.Context, req *spannerpb.PartitionQueryRequest, opts ...gax.CallOption) (*spannerpb.PartitionResponse, error) {
	e, err := c.Acquire(ctx)
	if err != nil {
		return nil, err
	}
	defer e.Release()
	return e.Client.PartitionQuery(ctx, req, opts...)
}

func (c *dcpResolvingSpannerClient) PartitionRead(ctx context.Context, req *spannerpb.PartitionReadRequest, opts ...gax.CallOption) (*spannerpb.PartitionResponse, error) {
	e, err := c.Acquire(ctx)
	if err != nil {
		return nil, err
	}
	defer e.Release()
	return e.Client.PartitionRead(ctx, req, opts...)
}

func (c *dcpResolvingSpannerClient) BatchWrite(ctx context.Context, req *spannerpb.BatchWriteRequest, opts ...gax.CallOption) (spannerpb.Spanner_BatchWriteClient, error) {
	e, err := c.Acquire(ctx)
	if err != nil {
		return nil, err
	}
	defer e.Release()
	return e.Client.BatchWrite(ctx, req, opts...)
}

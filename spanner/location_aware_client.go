/*
Copyright 2026 Google LLC

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package spanner

import (
	"context"
	"sync"
	"time"

	vkit "cloud.google.com/go/spanner/apiv1"
	"cloud.google.com/go/spanner/apiv1/spannerpb"
	"github.com/googleapis/gax-go/v2"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

var suppressEndpointRetryOptions = newSuppressRetryCodesOption(codes.ResourceExhausted, codes.Unavailable)

type locationAwareState struct {
	clientPool              []spannerClient
	router                  *locationRouter
	endpointCache           channelEndpointCache
	defaultAffinityEndpoint channelEndpoint
	defaultEndpointAddress  string
}

func newLocationAwareState(
	clientPool []spannerClient,
	router *locationRouter,
	endpointCache channelEndpointCache,
) *locationAwareState {
	var defaultAffinityEndpoint channelEndpoint = &passthroughChannelEndpoint{address: ""}
	if endpointCache != nil && endpointCache.DefaultChannel() != nil {
		defaultAffinityEndpoint = endpointCache.DefaultChannel()
	}
	return &locationAwareState{
		clientPool:              clientPool,
		router:                  router,
		endpointCache:           endpointCache,
		defaultAffinityEndpoint: defaultAffinityEndpoint,
		defaultEndpointAddress:  defaultAffinityEndpoint.Address(),
	}
}

func (s *locationAwareState) defaultClient(idx int) spannerClient {
	if s == nil || idx < 0 || idx >= len(s.clientPool) {
		return nil
	}
	return s.clientPool[idx]
}

// locationAwareSpannerClient is a thin spannerClient adapter that routes RPCs
// using shared client-level location-aware state while preserving the chosen
// default pooled client for the current request.
type locationAwareSpannerClient struct {
	state                   *locationAwareState
	defaultClientIndex      int
	defaultClient           spannerClient
	router                  *locationRouter
	endpointCache           channelEndpointCache
	defaultAffinityEndpoint channelEndpoint
	defaultEndpointAddress  string
}

var _ spannerClient = (*locationAwareSpannerClient)(nil)

// asGRPCSpannerClient extracts the underlying *grpcSpannerClient from a
// spannerClient, handling the locationAwareSpannerClient wrapper.
func asGRPCSpannerClient(c spannerClient) *grpcSpannerClient {
	if gsc, ok := c.(*grpcSpannerClient); ok {
		return gsc
	}
	if lac, ok := c.(*locationAwareSpannerClient); ok {
		return asGRPCSpannerClient(lac.defaultClient)
	}
	if dcp, ok := c.(*dcpSpannerClient); ok {
		return asGRPCSpannerClient(dcp.delegate)
	}
	return nil
}

func requestIDHeaderProviderFromSpannerClient(c spannerClient) requestIDHeaderProvider {
	if dcp, ok := c.(*dcpResolvingSpannerClient); ok {
		return dcp
	}
	return asGRPCSpannerClient(c)
}

func newLocationAwareSpannerClient(defaultClient spannerClient, router *locationRouter, endpointCache channelEndpointCache) *locationAwareSpannerClient {
	return newIndexedLocationAwareSpannerClient(
		newLocationAwareState([]spannerClient{defaultClient}, router, endpointCache),
		0,
	)
}

func newIndexedLocationAwareSpannerClient(state *locationAwareState, defaultClientIndex int) *locationAwareSpannerClient {
	if state == nil {
		return &locationAwareSpannerClient{defaultClientIndex: defaultClientIndex}
	}
	defaultClient := state.defaultClient(defaultClientIndex)
	return &locationAwareSpannerClient{
		state:                   state,
		defaultClientIndex:      defaultClientIndex,
		defaultClient:           defaultClient,
		router:                  state.router,
		endpointCache:           state.endpointCache,
		defaultAffinityEndpoint: state.defaultAffinityEndpoint,
		defaultEndpointAddress:  state.defaultEndpointAddress,
	}
}

func (c *locationAwareSpannerClient) affinityTrackingEndpoint(ep channelEndpoint) channelEndpoint {
	if ep != nil {
		return ep
	}
	return c.defaultAffinityEndpoint
}

func (c *locationAwareSpannerClient) onRequestRouted(ep channelEndpoint) {
	if c == nil || c.router == nil || c.router.lifecycleManager == nil || ep == nil {
		return
	}
	c.router.lifecycleManager.recordRealTraffic(ep.Address())
}

func (c *locationAwareSpannerClient) maybeMarkEndpointCoolingDown(ep channelEndpoint, err error) {
	if c == nil || ep == nil {
		return
	}
	if !shouldCooldownEndpointOnRetry(status.Code(err)) {
		return
	}
	if ep.Address() == c.defaultEndpointAddress {
		return
	}
	if c.endpointCache != nil {
		c.endpointCache.recordFailure(ep.Address())
	}
}

func shouldCooldownEndpointOnRetry(code codes.Code) bool {
	return code == codes.ResourceExhausted || code == codes.Unavailable
}

func (c *locationAwareSpannerClient) maybeRecordEndpointErrorPenalty(ep channelEndpoint, operationUID uint64, preferLeader bool, err error) {
	if c == nil || ep == nil || operationUID == 0 {
		return
	}
	if !shouldCooldownEndpointOnRetry(status.Code(err)) || ep.Address() == c.defaultEndpointAddress {
		return
	}
	if c.endpointCache != nil {
		c.endpointCache.recordError(operationUID, preferLeader, ep.Address())
	}
}

func (c *locationAwareSpannerClient) recordEndpointLatency(ep channelEndpoint, operationUID uint64, preferLeader bool, startedAt time.Time) {
	if c == nil || ep == nil || operationUID == 0 || ep.Address() == c.defaultEndpointAddress {
		return
	}
	if c.endpointCache != nil {
		c.endpointCache.recordLatency(operationUID, preferLeader, ep.Address(), time.Since(startedAt))
	}
}

func (c *locationAwareSpannerClient) rerouteErrorMarker(ep channelEndpoint, operationUID uint64, preferLeader bool) func(error) {
	var once sync.Once
	return func(err error) {
		if !shouldCooldownEndpointOnRetry(status.Code(err)) {
			return
		}
		once.Do(func() {
			c.maybeMarkEndpointCoolingDown(ep, err)
			c.maybeRecordEndpointErrorPenalty(ep, operationUID, preferLeader, err)
		})
	}
}

func locationAwareUnaryRetryOptions() []gax.CallOption {
	return []gax.CallOption{
		gax.WithRetry(func() gax.Retryer {
			return onCodesWithResourceExhaustedRetryOption(
				DefaultRetryBackoff,
				true,
				codes.Unavailable,
				codes.ResourceExhausted,
			)
		}),
	}
}

func singleAttemptRoutedUnaryCallOptions(base []gax.CallOption) []gax.CallOption {
	opts := make([]gax.CallOption, 0, len(base)+1)
	opts = append(opts, base...)
	opts = append(opts, suppressEndpointRetryOptions)
	return opts
}

func singleAttemptRoutedStreamCallOptions(base []gax.CallOption) []gax.CallOption {
	opts := make([]gax.CallOption, 0, len(base)+1)
	opts = append(opts, base...)
	opts = append(opts, suppressEndpointRetryOptions)
	return opts
}

func (c *locationAwareSpannerClient) observeExecuteSQLResponse(req *spannerpb.ExecuteSqlRequest, resp *spannerpb.ResultSet, ep channelEndpoint) {
	c.router.observeResultSet(resp)
	if txMeta := resp.GetMetadata().GetTransaction(); txMeta != nil && len(txMeta.GetId()) > 0 {
		if isReadOnlyBegin, readOnlyStrong := readOnlyBeginFromSelector(req.GetTransaction()); isReadOnlyBegin {
			c.router.trackReadOnlyTransaction(string(txMeta.GetId()), readOnlyStrong)
		} else if isReadWriteBeginFromSelector(req.GetTransaction()) {
			c.router.setTransactionAffinity(string(txMeta.GetId()), c.affinityTrackingEndpoint(ep))
		}
	}
}

func (c *locationAwareSpannerClient) observeReadResponse(req *spannerpb.ReadRequest, resp *spannerpb.ResultSet, ep channelEndpoint) {
	c.router.observeResultSet(resp)
	if txMeta := resp.GetMetadata().GetTransaction(); txMeta != nil && len(txMeta.GetId()) > 0 {
		if isReadOnlyBegin, readOnlyStrong := readOnlyBeginFromSelector(req.GetTransaction()); isReadOnlyBegin {
			c.router.trackReadOnlyTransaction(string(txMeta.GetId()), readOnlyStrong)
		} else if isReadWriteBeginFromSelector(req.GetTransaction()) {
			c.router.setTransactionAffinity(string(txMeta.GetId()), c.affinityTrackingEndpoint(ep))
		}
	}
}

func (c *locationAwareSpannerClient) observeBeginTransactionResponse(req *spannerpb.BeginTransactionRequest, resp *spannerpb.Transaction, ep channelEndpoint) {
	c.router.observeTransaction(resp)
	if len(resp.GetId()) > 0 {
		if isReadOnly, readOnlyStrong := readOnlyBeginFromTransactionOptions(req.GetOptions()); isReadOnly {
			c.router.trackReadOnlyTransaction(string(resp.GetId()), readOnlyStrong)
		} else {
			c.router.setTransactionAffinity(string(resp.GetId()), c.affinityTrackingEndpoint(ep))
		}
	}
}

// clientForEndpoint resolves a channelEndpoint to a spannerClient, falling
// back to the default client only when no selected endpoint/client exists.
func (c *locationAwareSpannerClient) clientForEndpoint(ep channelEndpoint) spannerClient {
	if ep == nil {
		return c.defaultClient
	}
	client := c.endpointCache.ClientFor(ep)
	if client == nil {
		return c.defaultClient
	}
	c.onRequestRouted(ep)
	return client
}

// affinityClient returns the spannerClient for a given transaction ID based on
// affinity, falling back to the default client.
func (c *locationAwareSpannerClient) affinityClient(txID []byte) spannerClient {
	return c.clientForTransactionAffinity(txID)
}

func (c *locationAwareSpannerClient) affinityEndpoint(txID []byte) channelEndpoint {
	if len(txID) == 0 {
		return nil
	}
	ep := c.router.getTransactionAffinity(string(txID))
	if ep != nil && c.endpointCache != nil && c.endpointCache.isCoolingDown(ep.Address()) {
		return nil
	}
	if ep != nil && !ep.IsHealthy() && c.router != nil && c.router.lifecycleManager != nil {
		c.router.lifecycleManager.requestEndpointRecreation(ep.Address())
	}
	if ep != nil && !ep.IsHealthy() {
		return nil
	}
	return ep
}

func (c *locationAwareSpannerClient) clientForTransactionAffinity(txID []byte) spannerClient {
	ep := c.affinityEndpoint(txID)
	return c.clientForEndpoint(ep)
}

type routeDecision struct {
	endpoint     channelEndpoint
	operationUID uint64
	preferLeader bool
}

func executeRoutedUnary[T any](
	ctx context.Context,
	c *locationAwareSpannerClient,
	opts []gax.CallOption,
	decide func() routeDecision,
	invoke func(spannerClient, []gax.CallOption) (T, error),
	observe func(T, channelEndpoint),
) (T, error) {
	var zero T
	var resp T
	err := gax.Invoke(ctx, func(invokeCtx context.Context, callSettings gax.CallSettings) error {
		decision := decide()
		ep := decision.endpoint
		client := c.clientForEndpoint(ep)
		markRetryableError := c.rerouteErrorMarker(ep, decision.operationUID, decision.preferLeader)
		currentOpts := singleAttemptRoutedUnaryCallOptions(opts)
		if ep != nil {
			ep.IncrementActiveRequests()
		}
		startedAt := time.Now()
		var err error
		resp, err = invoke(client, currentOpts)
		if ep != nil {
			ep.DecrementActiveRequests()
		}
		if err == nil {
			c.recordEndpointLatency(ep, decision.operationUID, decision.preferLeader, startedAt)
			if observe != nil {
				observe(resp, ep)
			}
			return nil
		}
		markRetryableError(err)
		return err
	}, locationAwareUnaryRetryOptions()...)
	if err != nil {
		return zero, err
	}
	return resp, nil
}

func executeRoutedStream[T streamingClient](
	c *locationAwareSpannerClient,
	opts []gax.CallOption,
	decide func() routeDecision,
	open func(spannerClient, []gax.CallOption) (T, error),
	wrap func(T, channelEndpoint, routeDecision, func(error), time.Time, func()) T,
) (T, error) {
	var zero T
	decision := decide()
	ep := decision.endpoint
	client := c.clientForEndpoint(ep)
	markRetryableError := c.rerouteErrorMarker(ep, decision.operationUID, decision.preferLeader)
	currentOpts := singleAttemptRoutedStreamCallOptions(opts)
	if ep != nil {
		ep.IncrementActiveRequests()
	}
	startedAt := time.Now()
	stream, err := open(client, currentOpts)
	if err != nil {
		if ep != nil {
			ep.DecrementActiveRequests()
		}
		markRetryableError(err)
		return zero, err
	}
	return wrap(stream, ep, decision, markRetryableError, startedAt, func() {
		if ep != nil {
			ep.DecrementActiveRequests()
		}
	}), nil
}

// --- Pass-through methods ---

func (c *locationAwareSpannerClient) CallOptions() *vkit.CallOptions {
	return c.defaultClient.CallOptions()
}

func (c *locationAwareSpannerClient) Close() error {
	return nil
}

func (c *locationAwareSpannerClient) Connection() *grpc.ClientConn {
	return c.defaultClient.Connection()
}

func (c *locationAwareSpannerClient) CreateSession(ctx context.Context, req *spannerpb.CreateSessionRequest, opts ...gax.CallOption) (*spannerpb.Session, error) {
	return c.defaultClient.CreateSession(ctx, req, opts...)
}

func (c *locationAwareSpannerClient) BatchCreateSessions(ctx context.Context, req *spannerpb.BatchCreateSessionsRequest, opts ...gax.CallOption) (*spannerpb.BatchCreateSessionsResponse, error) {
	return c.defaultClient.BatchCreateSessions(ctx, req, opts...)
}

func (c *locationAwareSpannerClient) GetSession(ctx context.Context, req *spannerpb.GetSessionRequest, opts ...gax.CallOption) (*spannerpb.Session, error) {
	return c.defaultClient.GetSession(ctx, req, opts...)
}

func (c *locationAwareSpannerClient) ListSessions(ctx context.Context, req *spannerpb.ListSessionsRequest, opts ...gax.CallOption) *vkit.SessionIterator {
	return c.defaultClient.ListSessions(ctx, req, opts...)
}

func (c *locationAwareSpannerClient) DeleteSession(ctx context.Context, req *spannerpb.DeleteSessionRequest, opts ...gax.CallOption) error {
	return c.defaultClient.DeleteSession(ctx, req, opts...)
}

func (c *locationAwareSpannerClient) ExecuteBatchDml(ctx context.Context, req *spannerpb.ExecuteBatchDmlRequest, opts ...gax.CallOption) (*spannerpb.ExecuteBatchDmlResponse, error) {
	return c.defaultClient.ExecuteBatchDml(ctx, req, opts...)
}

func (c *locationAwareSpannerClient) PartitionQuery(ctx context.Context, req *spannerpb.PartitionQueryRequest, opts ...gax.CallOption) (*spannerpb.PartitionResponse, error) {
	return c.defaultClient.PartitionQuery(ctx, req, opts...)
}

func (c *locationAwareSpannerClient) PartitionRead(ctx context.Context, req *spannerpb.PartitionReadRequest, opts ...gax.CallOption) (*spannerpb.PartitionResponse, error) {
	return c.defaultClient.PartitionRead(ctx, req, opts...)
}

func (c *locationAwareSpannerClient) BatchWrite(ctx context.Context, req *spannerpb.BatchWriteRequest, opts ...gax.CallOption) (spannerpb.Spanner_BatchWriteClient, error) {
	return c.defaultClient.BatchWrite(ctx, req, opts...)
}

// --- Routed RPCs ---

func (c *locationAwareSpannerClient) StreamingRead(ctx context.Context, req *spannerpb.ReadRequest, opts ...gax.CallOption) (spannerpb.Spanner_StreamingReadClient, error) {
	return executeRoutedStream(
		c,
		opts,
		func() routeDecision {
			return routeDecision{
				endpoint:     c.router.prepareReadRequest(ctx, req),
				operationUID: req.GetRoutingHint().GetOperationUid(),
				preferLeader: preferLeaderFromSelector(req.GetTransaction()),
			}
		},
		func(client spannerClient, currentOpts []gax.CallOption) (spannerpb.Spanner_StreamingReadClient, error) {
			return client.StreamingRead(ctx, req, currentOpts...)
		},
		func(stream spannerpb.Spanner_StreamingReadClient, ep channelEndpoint, decision routeDecision, markRetryableError func(error), startedAt time.Time, onDone func()) spannerpb.Spanner_StreamingReadClient {
			isReadOnlyBegin, readOnlyStrong := readOnlyBeginFromSelector(req.GetTransaction())
			return newAffinityTrackingStream(
				stream,
				c.router,
				c.affinityTrackingEndpoint(ep),
				isReadOnlyBegin,
				readOnlyStrong,
				isReadWriteBeginFromSelector(req.GetTransaction()),
				func() { c.recordEndpointLatency(ep, decision.operationUID, decision.preferLeader, startedAt) },
				markRetryableError,
				onDone,
			)
		},
	)
}

func (c *locationAwareSpannerClient) Read(ctx context.Context, req *spannerpb.ReadRequest, opts ...gax.CallOption) (*spannerpb.ResultSet, error) {
	return executeRoutedUnary(
		ctx,
		c,
		opts,
		func() routeDecision {
			return routeDecision{
				endpoint:     c.router.prepareReadRequest(ctx, req),
				operationUID: req.GetRoutingHint().GetOperationUid(),
				preferLeader: preferLeaderFromSelector(req.GetTransaction()),
			}
		},
		func(client spannerClient, currentOpts []gax.CallOption) (*spannerpb.ResultSet, error) {
			return client.Read(ctx, req, currentOpts...)
		},
		func(resp *spannerpb.ResultSet, ep channelEndpoint) {
			c.observeReadResponse(req, resp, ep)
		},
	)
}

func (c *locationAwareSpannerClient) ExecuteStreamingSql(ctx context.Context, req *spannerpb.ExecuteSqlRequest, opts ...gax.CallOption) (spannerpb.Spanner_ExecuteStreamingSqlClient, error) {
	return executeRoutedStream(
		c,
		opts,
		func() routeDecision {
			return routeDecision{
				endpoint:     c.router.prepareExecuteSQLRequest(ctx, req),
				operationUID: req.GetRoutingHint().GetOperationUid(),
				preferLeader: preferLeaderFromSelector(req.GetTransaction()),
			}
		},
		func(client spannerClient, currentOpts []gax.CallOption) (spannerpb.Spanner_ExecuteStreamingSqlClient, error) {
			return client.ExecuteStreamingSql(ctx, req, currentOpts...)
		},
		func(stream spannerpb.Spanner_ExecuteStreamingSqlClient, ep channelEndpoint, decision routeDecision, markRetryableError func(error), startedAt time.Time, onDone func()) spannerpb.Spanner_ExecuteStreamingSqlClient {
			isReadOnlyBegin, readOnlyStrong := readOnlyBeginFromSelector(req.GetTransaction())
			return newAffinityTrackingStream(
				stream,
				c.router,
				c.affinityTrackingEndpoint(ep),
				isReadOnlyBegin,
				readOnlyStrong,
				isReadWriteBeginFromSelector(req.GetTransaction()),
				func() { c.recordEndpointLatency(ep, decision.operationUID, decision.preferLeader, startedAt) },
				markRetryableError,
				onDone,
			)
		},
	)
}

func (c *locationAwareSpannerClient) ExecuteSql(ctx context.Context, req *spannerpb.ExecuteSqlRequest, opts ...gax.CallOption) (*spannerpb.ResultSet, error) {
	return executeRoutedUnary(
		ctx,
		c,
		opts,
		func() routeDecision {
			return routeDecision{
				endpoint:     c.router.prepareExecuteSQLRequest(ctx, req),
				operationUID: req.GetRoutingHint().GetOperationUid(),
				preferLeader: preferLeaderFromSelector(req.GetTransaction()),
			}
		},
		func(client spannerClient, currentOpts []gax.CallOption) (*spannerpb.ResultSet, error) {
			return client.ExecuteSql(ctx, req, currentOpts...)
		},
		func(resp *spannerpb.ResultSet, ep channelEndpoint) {
			c.observeExecuteSQLResponse(req, resp, ep)
		},
	)
}

func (c *locationAwareSpannerClient) BeginTransaction(ctx context.Context, req *spannerpb.BeginTransactionRequest, opts ...gax.CallOption) (*spannerpb.Transaction, error) {
	return executeRoutedUnary(
		ctx,
		c,
		opts,
		func() routeDecision {
			return routeDecision{
				endpoint:     c.router.prepareBeginTransactionRequest(ctx, req),
				operationUID: req.GetRoutingHint().GetOperationUid(),
				preferLeader: preferLeaderFromTransactionOptions(req.GetOptions()),
			}
		},
		func(client spannerClient, currentOpts []gax.CallOption) (*spannerpb.Transaction, error) {
			return client.BeginTransaction(ctx, req, currentOpts...)
		},
		func(resp *spannerpb.Transaction, ep channelEndpoint) {
			c.observeBeginTransactionResponse(req, resp, ep)
		},
	)
}

// --- Affinity RPCs ---

func (c *locationAwareSpannerClient) Commit(ctx context.Context, req *spannerpb.CommitRequest, opts ...gax.CallOption) (*spannerpb.CommitResponse, error) {
	ep := c.router.prepareCommitRequest(ctx, req)
	if txID := req.GetTransactionId(); len(txID) > 0 {
		if affinityEndpoint := c.affinityEndpoint(txID); affinityEndpoint != nil {
			ep = affinityEndpoint
		}
	}
	markRetryableError := c.rerouteErrorMarker(ep, 0, false)
	client := c.clientForEndpoint(ep)
	resp, err := client.Commit(ctx, req, appendResourceExhaustedMarkerOptions(opts, markRetryableError, true)...)
	markRetryableError(err)
	c.router.observeCommitResponse(resp)
	c.router.clearTransactionAffinity(string(req.GetTransactionId()))
	return resp, err
}

func (c *locationAwareSpannerClient) Rollback(ctx context.Context, req *spannerpb.RollbackRequest, opts ...gax.CallOption) error {
	ep := c.affinityEndpoint(req.GetTransactionId())
	markRetryableError := c.rerouteErrorMarker(ep, 0, false)
	client := c.clientForEndpoint(ep)
	err := client.Rollback(ctx, req, appendResourceExhaustedMarkerOptions(opts, markRetryableError, true)...)
	markRetryableError(err)
	c.router.clearTransactionAffinity(string(req.GetTransactionId()))
	return err
}

// affinityTrackingStream wraps a streaming RPC client to intercept Recv()
// calls and record transaction affinity from the first PartialResultSet that
// contains a transaction ID.
type affinityTrackingStream struct {
	grpc.ClientStream
	router             *locationRouter
	affinityEndpoint   channelEndpoint
	trackReadOnlyBegin bool
	readOnlyStrong     bool
	trackAffinity      bool
	once               sync.Once
	errorOnce          sync.Once
	latencyOnce        sync.Once
	doneOnce           sync.Once
	inner              streamingClient
	onFirstResponse    func()
	onError            func(error)
	onDone             func()
}

func (s *affinityTrackingStream) finish() {
	s.doneOnce.Do(func() {
		if s.onDone != nil {
			s.onDone()
		}
	})
}

// streamingClient is the shared interface implemented by both
// StreamingRead and ExecuteStreamingSql response streams.
type streamingClient interface {
	Recv() (*spannerpb.PartialResultSet, error)
	grpc.ClientStream
}

func newAffinityTrackingStream(
	inner streamingClient,
	router *locationRouter,
	affinityEndpoint channelEndpoint,
	trackReadOnlyBegin bool,
	readOnlyStrong bool,
	trackAffinity bool,
	onFirstResponse func(),
	onError func(error),
	onDone func(),
) *affinityTrackingStream {
	return &affinityTrackingStream{
		ClientStream:       inner,
		router:             router,
		affinityEndpoint:   affinityEndpoint,
		trackReadOnlyBegin: trackReadOnlyBegin,
		readOnlyStrong:     readOnlyStrong,
		trackAffinity:      trackAffinity,
		inner:              inner,
		onFirstResponse:    onFirstResponse,
		onError:            onError,
		onDone:             onDone,
	}
}

func (s *affinityTrackingStream) Recv() (*spannerpb.PartialResultSet, error) {
	prs, err := s.inner.Recv()
	if err != nil {
		s.finish()
		s.errorOnce.Do(func() {
			if s.onError != nil {
				s.onError(err)
			}
		})
		return nil, err
	}
	s.latencyOnce.Do(func() {
		if s.onFirstResponse != nil {
			s.onFirstResponse()
		}
	})
	// Record transaction metadata from the first PartialResultSet that contains
	// a transaction ID.
	if txMeta := prs.GetMetadata().GetTransaction(); txMeta != nil && len(txMeta.GetId()) > 0 {
		txID := string(txMeta.GetId())
		s.once.Do(func() {
			if s.trackReadOnlyBegin {
				s.router.trackReadOnlyTransaction(txID, s.readOnlyStrong)
				return
			}
			if s.trackAffinity {
				s.router.setTransactionAffinity(txID, s.affinityEndpoint)
			}
		})
	}
	// Observe cache updates from every PartialResultSet.
	s.router.observePartialResultSet(prs)
	return prs, nil
}

func readOnlyBeginFromSelector(selector *spannerpb.TransactionSelector) (bool, bool) {
	if selector == nil {
		return false, false
	}
	begin := selector.GetBegin()
	if begin == nil || begin.GetReadOnly() == nil {
		return false, false
	}
	return true, begin.GetReadOnly().GetStrong()
}

func isReadWriteBeginFromSelector(selector *spannerpb.TransactionSelector) bool {
	if selector == nil {
		return false
	}
	begin := selector.GetBegin()
	return begin != nil && begin.GetReadOnly() == nil
}

func readOnlyBeginFromTransactionOptions(options *spannerpb.TransactionOptions) (bool, bool) {
	if options == nil || options.GetReadOnly() == nil {
		return false, false
	}
	return true, options.GetReadOnly().GetStrong()
}

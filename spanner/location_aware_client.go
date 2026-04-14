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

	vkit "cloud.google.com/go/spanner/apiv1"
	"cloud.google.com/go/spanner/apiv1/spannerpb"
	"github.com/googleapis/gax-go/v2"
	"go.opentelemetry.io/otel/attribute"
	otrace "go.opentelemetry.io/otel/trace"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// locationAwareSpannerClient is a spannerClient wrapper that routes RPCs to
// specific server endpoints based on location-aware routing hints.
//
// Routed RPCs (StreamingRead, Read, ExecuteStreamingSql, ExecuteSql,
// BeginTransaction) first ask the locationRouter for a routing hint and
// endpoint, then dispatch to the endpoint's spannerClient if available.
//
// Affinity RPCs (Commit, Rollback) look up the transaction affinity set by
// prior RPCs and route to the same server.
//
// All other RPCs are passed through to the default client.
type locationAwareSpannerClient struct {
	defaultClient           spannerClient
	router                  *locationRouter
	endpointCache           channelEndpointCache
	defaultAffinityEndpoint channelEndpoint
	defaultEndpointAddress  string
	excludedEndpoints       *logicalRequestEndpointExclusionCache
}

var _ spannerClient = (*locationAwareSpannerClient)(nil)

type requestIDAttemptOptioner interface {
	withNextRetryAttempt(uint32) gax.CallOption
}

// asGRPCSpannerClient extracts the underlying *grpcSpannerClient from a
// spannerClient, handling the locationAwareSpannerClient wrapper.
func asGRPCSpannerClient(c spannerClient) *grpcSpannerClient {
	if gsc, ok := c.(*grpcSpannerClient); ok {
		return gsc
	}
	if lac, ok := c.(*locationAwareSpannerClient); ok {
		return asGRPCSpannerClient(lac.defaultClient)
	}
	return nil
}

func newLocationAwareSpannerClient(defaultClient spannerClient, router *locationRouter, endpointCache channelEndpointCache) *locationAwareSpannerClient {
	var defaultAffinityEndpoint channelEndpoint = &passthroughChannelEndpoint{address: ""}
	if endpointCache != nil && endpointCache.DefaultChannel() != nil {
		defaultAffinityEndpoint = endpointCache.DefaultChannel()
	}
	return &locationAwareSpannerClient{
		defaultClient:           defaultClient,
		router:                  router,
		endpointCache:           endpointCache,
		defaultAffinityEndpoint: defaultAffinityEndpoint,
		defaultEndpointAddress:  defaultAffinityEndpoint.Address(),
		excludedEndpoints:       newLogicalRequestEndpointExclusionCache(),
	}
}

func (c *locationAwareSpannerClient) affinityTrackingEndpoint(ep channelEndpoint) channelEndpoint {
	if ep != nil {
		return ep
	}
	return c.defaultAffinityEndpoint
}

func (c *locationAwareSpannerClient) onRequestRouted(ep channelEndpoint) {
	if c == nil || c.router == nil || c.router.lifecycleManager == nil || ep == nil || !ep.IsHealthy() {
		return
	}
	c.router.lifecycleManager.recordRealTraffic(ep.Address())
}

func (c *locationAwareSpannerClient) excludedEndpointsForCall(opts []gax.CallOption) (string, endpointExcluder) {
	if c == nil || c.excludedEndpoints == nil {
		return "", noExcludedEndpoints
	}
	logicalRequestKey := logicalRequestKeyFromCallOptions(opts)
	return logicalRequestKey, c.excludedEndpoints.consume(logicalRequestKey)
}

func (c *locationAwareSpannerClient) maybeExcludeEndpointOnNextCall(ep channelEndpoint, logicalRequestKey string, err error) {
	if c == nil || c.excludedEndpoints == nil || ep == nil || logicalRequestKey == "" {
		return
	}
	if status.Code(err) != codes.ResourceExhausted {
		return
	}
	if ep.Address() == c.defaultEndpointAddress {
		return
	}
	c.excludedEndpoints.record(logicalRequestKey, ep.Address())
}

func (c *locationAwareSpannerClient) recordRouteSelectionTrace(ctx context.Context, method string, ep channelEndpoint, usedDefaultEndpoint bool, details routeSelectionDetails) {
	endpointAddr := details.selectedEndpoint
	if endpointAddr == "" && ep != nil {
		endpointAddr = ep.Address()
	}

	if c == nil {
		return
	}

	span := otrace.SpanFromContext(ctx)
	if !span.IsRecording() {
		return
	}

	target := c.defaultEndpointAddress
	if !usedDefaultEndpoint && ep != nil {
		target = ep.Address()
	}

	attrs := []attribute.KeyValue{
		attribute.String("spanner.target", target),
		attribute.Bool("spanner.route.used_default_endpoint", usedDefaultEndpoint),
		attribute.Bool("spanner.route.has_channel_finder", endpointAddr != ""),
		attribute.String("spanner.route.method", method),
	}
	if details.defaultReasonCode != "" {
		attrs = append(attrs, attribute.String("spanner.route.default_reason_code", details.defaultReasonCode))
	}
	span.SetAttributes(attrs...)
	span.AddEvent("spanner.route.selected", otrace.WithAttributes(attrs...))
}

func requestIDAttemptOptionerFromCallOptions(client spannerClient, opts []gax.CallOption) requestIDAttemptOptioner {
	if len(opts) != 0 {
		var settings gax.CallSettings
		for _, opt := range opts {
			if opt == nil {
				continue
			}
			opt.Resolve(&settings)
		}
		_, reqID, found := gRPCCallOptionsToRequestID(settings.GRPC)
		if found {
			return logicalRequestIDWrap{logicalKey: reqID.logicalRequestKey()}
		}
	}

	gsc := asGRPCSpannerClient(client)
	if gsc == nil {
		return nil
	}
	return gsc.generateRequestIDHeaderInjector()
}

func appendUnaryRetryOverrideOptions(base []gax.CallOption, requestID requestIDAttemptOptioner, attempt uint32) []gax.CallOption {
	opts := append([]gax.CallOption{}, base...)
	if requestID != nil {
		opts = append(opts, requestID.withNextRetryAttempt(attempt))
	}
	opts = append(opts, newSuppressRetryCodesOption(codes.ResourceExhausted))
	return opts
}

func combineEndpointExcluders(base endpointExcluder, excludedAddress string) endpointExcluder {
	if excludedAddress == "" {
		return base
	}
	return func(address string) bool {
		if address == excludedAddress {
			return true
		}
		return isEndpointExcluded(base, address)
	}
}

func (c *locationAwareSpannerClient) shouldManualRoutedUnaryRetry(ep channelEndpoint, client spannerClient, err error) bool {
	if c == nil || ep == nil || client == nil || client == c.defaultClient {
		return false
	}
	if ep.Address() == "" || ep.Address() == c.defaultEndpointAddress {
		return false
	}
	return status.Code(err) == codes.ResourceExhausted
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
// back to the default client if the endpoint is nil, unhealthy, or has no
// associated client.
func (c *locationAwareSpannerClient) clientForEndpoint(ep channelEndpoint) spannerClient {
	if ep == nil || !ep.IsHealthy() {
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
	return c.affinityClientWithExclusions(txID, nil)
}

func (c *locationAwareSpannerClient) affinityEndpoint(txID []byte, excludedEndpoints endpointExcluder) channelEndpoint {
	if len(txID) == 0 {
		return nil
	}
	ep := c.router.getTransactionAffinity(string(txID))
	if ep != nil && isEndpointExcluded(excludedEndpoints, ep.Address()) {
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

func (c *locationAwareSpannerClient) affinityClientWithExclusions(txID []byte, excludedEndpoints endpointExcluder) spannerClient {
	ep := c.affinityEndpoint(txID, excludedEndpoints)
	return c.clientForEndpoint(ep)
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
	logicalRequestKey, excludedEndpoints := c.excludedEndpointsForCall(opts)
	ep, details := c.router.prepareReadRequestWithExclusionsAndDetails(ctx, req, excludedEndpoints)
	client := c.clientForEndpoint(ep)
	c.recordRouteSelectionTrace(ctx, "google.spanner.v1.Spanner/StreamingRead", ep, client == c.defaultClient, details)
	stream, err := client.StreamingRead(ctx, req, opts...)
	if err != nil {
		c.maybeExcludeEndpointOnNextCall(ep, logicalRequestKey, err)
		return nil, err
	}
	isReadOnlyBegin, readOnlyStrong := readOnlyBeginFromSelector(req.GetTransaction())
	return newAffinityTrackingStream(
		stream,
		c.router,
		c.affinityTrackingEndpoint(ep),
		isReadOnlyBegin,
		readOnlyStrong,
		isReadWriteBeginFromSelector(req.GetTransaction()),
		nil,
		func(err error) {
			c.maybeExcludeEndpointOnNextCall(ep, logicalRequestKey, err)
		},
	), nil
}

func (c *locationAwareSpannerClient) Read(ctx context.Context, req *spannerpb.ReadRequest, opts ...gax.CallOption) (*spannerpb.ResultSet, error) {
	logicalRequestKey, excludedEndpoints := c.excludedEndpointsForCall(opts)
	requestID := requestIDAttemptOptionerFromCallOptions(c.defaultClient, opts)
	ep, details := c.router.prepareReadRequestWithExclusionsAndDetails(ctx, req, excludedEndpoints)
	client := c.clientForEndpoint(ep)
	c.recordRouteSelectionTrace(ctx, "google.spanner.v1.Spanner/Read", ep, client == c.defaultClient, details)
	resp, err := client.Read(ctx, req, appendUnaryRetryOverrideOptions(opts, requestID, 1)...)
	if err != nil {
		if !c.shouldManualRoutedUnaryRetry(ep, client, err) {
			c.maybeExcludeEndpointOnNextCall(ep, logicalRequestKey, err)
			return nil, err
		}

		retryExcludedEndpoints := combineEndpointExcluders(excludedEndpoints, ep.Address())
		retryEndpoint, retryDetails := c.router.prepareReadRequestWithExclusionsAndDetails(ctx, req, retryExcludedEndpoints)
		retryClient := c.clientForEndpoint(retryEndpoint)
		c.recordRouteSelectionTrace(ctx, "google.spanner.v1.Spanner/Read", retryEndpoint, retryClient == c.defaultClient, retryDetails)
		resp, err = retryClient.Read(ctx, req, appendUnaryRetryOverrideOptions(opts, requestID, 2)...)
		if err == nil {
			c.observeReadResponse(req, resp, retryEndpoint)
			return resp, nil
		}
		c.maybeExcludeEndpointOnNextCall(retryEndpoint, logicalRequestKey, err)
		return nil, err
	}
	c.observeReadResponse(req, resp, ep)
	return resp, nil
}

func (c *locationAwareSpannerClient) ExecuteStreamingSql(ctx context.Context, req *spannerpb.ExecuteSqlRequest, opts ...gax.CallOption) (spannerpb.Spanner_ExecuteStreamingSqlClient, error) {
	logicalRequestKey, excludedEndpoints := c.excludedEndpointsForCall(opts)
	ep, details := c.router.prepareExecuteSQLRequestWithExclusionsAndDetails(ctx, req, excludedEndpoints)
	client := c.clientForEndpoint(ep)
	c.recordRouteSelectionTrace(ctx, "google.spanner.v1.Spanner/ExecuteStreamingSql", ep, client == c.defaultClient, details)
	stream, err := client.ExecuteStreamingSql(ctx, req, opts...)
	if err != nil {
		c.maybeExcludeEndpointOnNextCall(ep, logicalRequestKey, err)
		return nil, err
	}
	isReadOnlyBegin, readOnlyStrong := readOnlyBeginFromSelector(req.GetTransaction())
	return newAffinityTrackingStream(
		stream,
		c.router,
		c.affinityTrackingEndpoint(ep),
		isReadOnlyBegin,
		readOnlyStrong,
		isReadWriteBeginFromSelector(req.GetTransaction()),
		nil,
		func(err error) {
			c.maybeExcludeEndpointOnNextCall(ep, logicalRequestKey, err)
		},
	), nil
}

func (c *locationAwareSpannerClient) ExecuteSql(ctx context.Context, req *spannerpb.ExecuteSqlRequest, opts ...gax.CallOption) (*spannerpb.ResultSet, error) {
	logicalRequestKey, excludedEndpoints := c.excludedEndpointsForCall(opts)
	requestID := requestIDAttemptOptionerFromCallOptions(c.defaultClient, opts)
	ep, details := c.router.prepareExecuteSQLRequestWithExclusionsAndDetails(ctx, req, excludedEndpoints)
	client := c.clientForEndpoint(ep)
	c.recordRouteSelectionTrace(ctx, "google.spanner.v1.Spanner/ExecuteSql", ep, client == c.defaultClient, details)
	resp, err := client.ExecuteSql(ctx, req, appendUnaryRetryOverrideOptions(opts, requestID, 1)...)
	if err != nil {
		if !c.shouldManualRoutedUnaryRetry(ep, client, err) {
			c.maybeExcludeEndpointOnNextCall(ep, logicalRequestKey, err)
			return nil, err
		}

		retryExcludedEndpoints := combineEndpointExcluders(excludedEndpoints, ep.Address())
		retryEndpoint, retryDetails := c.router.prepareExecuteSQLRequestWithExclusionsAndDetails(ctx, req, retryExcludedEndpoints)
		retryClient := c.clientForEndpoint(retryEndpoint)
		c.recordRouteSelectionTrace(ctx, "google.spanner.v1.Spanner/ExecuteSql", retryEndpoint, retryClient == c.defaultClient, retryDetails)
		resp, err = retryClient.ExecuteSql(ctx, req, appendUnaryRetryOverrideOptions(opts, requestID, 2)...)
		if err == nil {
			c.observeExecuteSQLResponse(req, resp, retryEndpoint)
			return resp, nil
		}
		c.maybeExcludeEndpointOnNextCall(retryEndpoint, logicalRequestKey, err)
		return nil, err
	}
	c.observeExecuteSQLResponse(req, resp, ep)
	return resp, nil
}

func (c *locationAwareSpannerClient) BeginTransaction(ctx context.Context, req *spannerpb.BeginTransactionRequest, opts ...gax.CallOption) (*spannerpb.Transaction, error) {
	logicalRequestKey, excludedEndpoints := c.excludedEndpointsForCall(opts)
	requestID := requestIDAttemptOptionerFromCallOptions(c.defaultClient, opts)
	ep, details := c.router.prepareBeginTransactionRequestWithExclusionsAndDetails(ctx, req, excludedEndpoints)
	client := c.clientForEndpoint(ep)
	c.recordRouteSelectionTrace(ctx, "google.spanner.v1.Spanner/BeginTransaction", ep, client == c.defaultClient, details)
	resp, err := client.BeginTransaction(ctx, req, appendUnaryRetryOverrideOptions(opts, requestID, 1)...)
	if err != nil {
		if !c.shouldManualRoutedUnaryRetry(ep, client, err) {
			c.maybeExcludeEndpointOnNextCall(ep, logicalRequestKey, err)
			return nil, err
		}

		retryExcludedEndpoints := combineEndpointExcluders(excludedEndpoints, ep.Address())
		retryEndpoint, retryDetails := c.router.prepareBeginTransactionRequestWithExclusionsAndDetails(ctx, req, retryExcludedEndpoints)
		retryClient := c.clientForEndpoint(retryEndpoint)
		c.recordRouteSelectionTrace(ctx, "google.spanner.v1.Spanner/BeginTransaction", retryEndpoint, retryClient == c.defaultClient, retryDetails)
		resp, err = retryClient.BeginTransaction(ctx, req, appendUnaryRetryOverrideOptions(opts, requestID, 2)...)
		if err == nil {
			c.observeBeginTransactionResponse(req, resp, retryEndpoint)
			return resp, nil
		}
		c.maybeExcludeEndpointOnNextCall(retryEndpoint, logicalRequestKey, err)
		return nil, err
	}
	c.observeBeginTransactionResponse(req, resp, ep)
	return resp, nil
}

// --- Affinity RPCs ---

func (c *locationAwareSpannerClient) Commit(ctx context.Context, req *spannerpb.CommitRequest, opts ...gax.CallOption) (*spannerpb.CommitResponse, error) {
	logicalRequestKey, excludedEndpoints := c.excludedEndpointsForCall(opts)
	ep, details := c.router.prepareCommitRequestWithExclusionsAndDetails(ctx, req, excludedEndpoints)
	if txID := req.GetTransactionId(); len(txID) > 0 {
		if affinityEndpoint := c.affinityEndpoint(txID, excludedEndpoints); affinityEndpoint != nil {
			ep = affinityEndpoint
			details.setSelectedTablet(ep.Address(), false, false)
		}
	}
	client := c.clientForEndpoint(ep)
	c.recordRouteSelectionTrace(ctx, "google.spanner.v1.Spanner/Commit", ep, client == c.defaultClient, details)
	resp, err := client.Commit(ctx, req, opts...)
	c.maybeExcludeEndpointOnNextCall(ep, logicalRequestKey, err)
	c.router.observeCommitResponse(resp)
	c.router.clearTransactionAffinity(string(req.GetTransactionId()))
	return resp, err
}

func (c *locationAwareSpannerClient) Rollback(ctx context.Context, req *spannerpb.RollbackRequest, opts ...gax.CallOption) error {
	logicalRequestKey, excludedEndpoints := c.excludedEndpointsForCall(opts)
	ep := c.affinityEndpoint(req.GetTransactionId(), excludedEndpoints)
	details := newRouteSelectionDetails()
	if ep != nil {
		details.setSelectedTablet(ep.Address(), false, false)
	}
	client := c.clientForEndpoint(ep)
	c.recordRouteSelectionTrace(ctx, "google.spanner.v1.Spanner/Rollback", ep, client == c.defaultClient, details)
	err := client.Rollback(ctx, req, opts...)
	c.maybeExcludeEndpointOnNextCall(ep, logicalRequestKey, err)
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
	inner              streamingClient
	onFirstResponse    func()
	onError            func(error)
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
	}
}

func (s *affinityTrackingStream) Recv() (*spannerpb.PartialResultSet, error) {
	prs, err := s.inner.Recv()
	if err != nil {
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

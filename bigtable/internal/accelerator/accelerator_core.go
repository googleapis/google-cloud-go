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
// Channel is scoped to one (project, instance, appProfile). It owns a
// session.Client and dispatches each RPC by opening a per-resource
// session.TableAPI on it. session.Client already dedupes the expensive
// resource — the per-(resource, permission) read/write session pools — so
// opening a handle per call is cheap and nothing is cached at this layer.
// Close tears down the session.Client and all its pools.
package accelerator

import (
	"bytes"
	"context"
	"io"

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
	dataScope    = "https://www.googleapis.com/auth/bigtable.data"
)

var userAgent = "cbt-go-accelerator/v" + internal.Version

// Ensure Channel implements grpc.ClientConnInterface.
var _ grpc.ClientConnInterface = (*Channel)(nil)

// newSessionClient constructs the session.Client a Channel dials on
// construction. It is a package-level seam that tests override to inject a
// mock; package-level state mutation is not parallel-safe.
var newSessionClient = func(
	ctx context.Context,
	project, instance, appProfile string,
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
	factory, err := metrics.NewFactory(ctx, project, instance, appProfile, metricsProvider, opts...)
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

	return session.NewClient(ctx, project, instance, appProfile, metricsProvider, featureFlags, opts...)
}

// Channel is an in-process grpc.ClientConnInterface backed by
// internal/session. It owns a session.Client and opens a per-resource
// session.TableAPI on it for each RPC. One channel per
// (project, instance, appProfile).
//
// scopePrefix is "projects/<project>/instances/<instance>/", precomputed once
// at construction. The dispatch path validates that an incoming V2 resource
// name targets this daemon's scope against it, then strips it to the leaf ID
// session.Client expects (see resourcename.go). It is the only retained form of
// the (project, instance) the channel is scoped to.
type Channel struct {
	sc          session.Client
	scopePrefix string
}

// NewChannel constructs an Channel scoped to
// (project, instance, appProfile). It dials the underlying session.Client,
// which the Channel owns and closes via Close (below).
func NewChannel(
	ctx context.Context,
	project, instance, appProfile string,
	opts ...option.ClientOption,
) (*Channel, error) {
	// session.NewClient forwards opts straight to gtransport.Dial
	// without supplying a default endpoint, so without these the dial target
	// is empty ("received empty target in Build()"). Establish the standard
	// Bigtable data-plane endpoint, scope, and user agent first, then let the
	// caller's opts override. Mirrors bigtable.NewClient's use of
	// btopt.DefaultClientOptions.
	defaultOpts, err := btopt.DefaultClientOptions(prodAddr, mtlsProdAddr, dataScope, userAgent)
	if err != nil {
		return nil, err
	}
	opts = append(defaultOpts, opts...)

	sc, err := newSessionClient(ctx, project, instance, appProfile, opts...)
	if err != nil {
		return nil, err
	}
	return &Channel{
		sc:          sc,
		scopePrefix: scopePrefixFor(project, instance),
	}, nil
}

// openHandle routes an extracted resource to a session.TableAPI based on its
// kind. The adapter already determined the kind from the populated V2 name
// field; here the full name is validated against this daemon's scope and
// reduced to the leaf ID(s) session.Client expects, which re-prefixes them with
// its own project/instance. A name targeting a different project/instance is
// rejected rather than silently served from this daemon's scope.
//
// The underlying read/write session pools are deduped inside session.Client,
// so opening a handle per call is cheap and nothing is cached here.
func (c *Channel) openHandle(res adapters.Resource) (session.TableAPI, error) {
	switch res.Kind {
	case adapters.ResourceTable:
		tableID, err := c.parseTableName(res.Name)
		if err != nil {
			return nil, err
		}
		return c.sc.OpenTable(tableID), nil
	case adapters.ResourceAuthorizedView:
		tableID, viewID, err := c.parseAuthorizedViewName(res.Name)
		if err != nil {
			return nil, err
		}
		return c.sc.OpenAuthorizedView(tableID, viewID), nil
	case adapters.ResourceMaterializedView:
		viewID, err := c.parseMaterializedViewName(res.Name)
		if err != nil {
			return nil, err
		}
		return c.sc.OpenMaterializedView(viewID), nil
	default:
		// Kind is set by the adapter from the populated V2 name field, so an
		// unrecognized kind is an internal invariant violation, not bad input.
		return nil, status.Errorf(codes.Internal, "accelerator: unknown resource kind %v", res.Kind)
	}
}

// Invoke implements grpc.ClientConnInterface for unary V2 RPCs.
func (c *Channel) Invoke(ctx context.Context, method string, args, reply interface{}, _ ...grpc.CallOption) error {
	switch method {
	case v2pb.Bigtable_MutateRow_FullMethodName:
		return c.mutateRowImpl(ctx, args, reply)
	default:
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

	// Open a fresh handle per call, routed to a table or authorized view by the
	// resource kind; the underlying read/write session pools are deduped inside
	// session.Client, so this is cheap.
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
		return nil, status.Errorf(codes.Unimplemented, "accelerator: streaming method %s not implemented", method)
	}
}

// Close releases resources held by the channel by closing the underlying
// session.Client and all its pools.
func (c *Channel) Close() error {
	if c.sc == nil {
		return nil
	}
	return c.sc.Close()
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

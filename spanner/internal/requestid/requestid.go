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

// Package requestid implements x-goog-spanner-request-id generation, CallOption
// propagation, and gRPC client interceptors shared across Spanner packages.
package requestid

import (
	"context"
	"crypto/rand"
	"fmt"
	"math"
	"math/big"
	"strings"
	"sync/atomic"
	"time"

	"github.com/googleapis/gax-go/v2"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"
	"google.golang.org/grpc"
	"google.golang.org/grpc/metadata"
)

// randIDForProcess is a strongly randomly generated 16-hex-char value derived
// from a uint64 in the range [0, maxUint64].
var randIDForProcess string

func init() {
	bigMaxInt64, _ := new(big.Int).SetString(fmt.Sprintf("%d", uint64(math.MaxUint64)), 10)
	if g, w := bigMaxInt64.Uint64(), uint64(math.MaxUint64); g != w {
		panic(fmt.Sprintf("mismatch in randIDForProcess.maxUint64:\n\tGot:  %d\n\tWant: %d", g, w))
	}
	r64, err := rand.Int(rand.Reader, bigMaxInt64)
	if err != nil {
		panic(err)
	}
	randIDForProcess = fmt.Sprintf("%016x", r64.Uint64())
}

// RandIDForProcess returns the process-wide 16-hex-character random identifier
// used in x-goog-spanner-request-id headers.
func RandIDForProcess() string {
	return randIDForProcess
}

// Version is the x-goog-spanner-request-id specification version.
// Please bump this version whenever this implementation
// executes on the plans of a new specification.
const Version uint8 = 1

// Header is the outgoing gRPC metadata key for Spanner request IDs.
const Header = "x-goog-spanner-request-id"

// spanAttr is the OpenTelemetry span attribute key for Spanner request IDs.
const spanAttr = "x_goog_spanner_request_id"

// RequestID represents a formatted x-goog-spanner-request-id value.
type RequestID string

// LogicalRequestKey returns the request ID prefix without the trailing ".attempt" segment.
func (r RequestID) LogicalRequestKey() string {
	requestIDValue := string(r)
	lastSeparator := strings.LastIndexByte(requestIDValue, '.')
	if lastSeparator < 0 {
		return requestIDValue
	}
	return requestIDValue[:lastSeparator]
}

type logicalRequestKeyOption interface {
	logicalRequestKey() string
}

// GRPCCallOptionsToRequestID extracts the x-goog-spanner-request-id header from gRPC CallOptions.
func GRPCCallOptionsToRequestID(opts []grpc.CallOption) (md metadata.MD, reqID RequestID, found bool) {
	for _, opt := range opts {
		hdrOpt, ok := opt.(grpc.HeaderCallOption)
		if !ok {
			continue
		}
		mdPtr := hdrOpt.HeaderAddr
		if mdPtr == nil {
			continue
		}
		reqIDs := mdPtr.Get(Header)
		if len(reqIDs) != 0 && len(reqIDs[0]) != 0 {
			md = *mdPtr
			reqID = RequestID(reqIDs[0])
			found = true
			break
		}
	}
	return
}

func resolveCallSettings(opts []gax.CallOption) gax.CallSettings {
	var settings gax.CallSettings
	for _, opt := range opts {
		if opt == nil {
			continue
		}
		opt.Resolve(&settings)
	}
	return settings
}

// LogicalRequestKeyFromCallOptions extracts the logical request key (without attempt suffix)
// from a slice of gax.CallOptions.
func LogicalRequestKeyFromCallOptions(opts []gax.CallOption) string {
	if len(opts) == 0 {
		return ""
	}
	for _, opt := range opts {
		if opt == nil {
			continue
		}
		reqIDOpt, ok := opt.(logicalRequestKeyOption)
		if !ok {
			continue
		}
		if key := reqIDOpt.logicalRequestKey(); key != "" {
			return key
		}
	}

	settings := resolveCallSettings(opts)
	_, reqID, found := GRPCCallOptionsToRequestID(settings.GRPC)
	if !found {
		return ""
	}
	return reqID.LogicalRequestKey()
}

func callOptionsHaveRequestID(opts []gax.CallOption) bool {
	if len(opts) == 0 {
		return false
	}
	settings := resolveCallSettings(opts)
	_, _, found := GRPCCallOptionsToRequestID(settings.GRPC)
	return found
}

// stripRequestIDGRPCOptions returns a slice without any grpc.HeaderCallOption
// carrying x-goog-spanner-request-id, without mutating opts's backing array.
func stripRequestIDGRPCOptions(opts []grpc.CallOption) []grpc.CallOption {
	if len(opts) == 0 {
		return nil
	}
	hasReqID := false
	for _, opt := range opts {
		if hdrOpt, ok := opt.(grpc.HeaderCallOption); ok && hdrOpt.HeaderAddr != nil && len(hdrOpt.HeaderAddr.Get(Header)) > 0 {
			hasReqID = true
			break
		}
	}
	if !hasReqID {
		return opts[:len(opts):len(opts)]
	}
	filtered := make([]grpc.CallOption, 0, len(opts))
	for _, opt := range opts {
		hdrOpt, ok := opt.(grpc.HeaderCallOption)
		if !ok || hdrOpt.HeaderAddr == nil || len(hdrOpt.HeaderAddr.Get(Header)) == 0 {
			filtered = append(filtered, opt)
		}
	}
	return filtered
}

// ErrorAugmenter attaches a requestID to a non-nil RPC error when appropriate.
type ErrorAugmenter func(err error, reqID string) error

// HeaderInjector is a gRPC unary and stream client interceptor that propagates
// x-goog-spanner-request-id from CallOptions into outgoing metadata, OpenTelemetry
// span attributes, and returned errors.
type HeaderInjector struct {
	augment ErrorAugmenter
}

// NewHeaderInjector creates a HeaderInjector using augment to attach request IDs to errors.
func NewHeaderInjector(augment ErrorAugmenter) *HeaderInjector {
	return &HeaderInjector{augment: augment}
}

func (wr *HeaderInjector) augmentErr(err error, reqID RequestID) error {
	if err == nil || wr == nil || wr.augment == nil {
		return err
	}
	return wr.augment(err, string(reqID))
}

func injectRequestIDIntoContext(ctx context.Context, opts []grpc.CallOption) (context.Context, RequestID, bool) {
	// It is imperative to search for the requestID before the call
	// because gRPC's internals will consume the headers.
	_, reqID, found := GRPCCallOptionsToRequestID(opts)
	if found {
		ctx = metadata.AppendToOutgoingContext(ctx, Header, string(reqID))
		// Associate the requestId as an attribute on the span in the current context.
		span := trace.SpanFromContext(ctx)
		span.SetAttributes(attribute.KeyValue{
			Key:   spanAttr,
			Value: attribute.StringValue(string(reqID)),
		})
	}
	return ctx, reqID, found
}

// InterceptUnary is a grpc.UnaryClientInterceptor that injects x-goog-spanner-request-id.
func (wr *HeaderInjector) InterceptUnary(ctx context.Context, method string, req, reply any, cc *grpc.ClientConn, invoker grpc.UnaryInvoker, opts ...grpc.CallOption) error {
	ctx, reqID, foundRequestID := injectRequestIDIntoContext(ctx, opts)

	err := invoker(ctx, method, req, reply, cc, opts...)
	if !foundRequestID {
		return err
	}
	return wr.augmentErr(err, reqID)
}

type errWrappingClientStream struct {
	grpc.ClientStream
	reqID   RequestID
	augment ErrorAugmenter
}

func (rew *errWrappingClientStream) processErr(err error) error {
	if err == nil || rew.augment == nil {
		return err
	}
	return rew.augment(err, string(rew.reqID))
}

func (rew *errWrappingClientStream) SendMsg(msg any) error {
	err := rew.ClientStream.SendMsg(msg)
	return rew.processErr(err)
}

func (rew *errWrappingClientStream) RecvMsg(msg any) error {
	err := rew.ClientStream.RecvMsg(msg)
	return rew.processErr(err)
}

var _ grpc.ClientStream = (*errWrappingClientStream)(nil)

// InterceptStream is a grpc.StreamClientInterceptor that injects x-goog-spanner-request-id.
func (wr *HeaderInjector) InterceptStream(ctx context.Context, desc *grpc.StreamDesc, cc *grpc.ClientConn, method string, streamer grpc.Streamer, opts ...grpc.CallOption) (grpc.ClientStream, error) {
	ctx, reqID, foundRequestID := injectRequestIDIntoContext(ctx, opts)

	cs, err := streamer(ctx, desc, cc, method, opts...)
	if !foundRequestID || wr == nil || wr.augment == nil {
		return cs, err
	}
	wcs := &errWrappingClientStream{ClientStream: cs, reqID: reqID, augment: wr.augment}
	if err == nil {
		return wcs, nil
	}
	return wcs, wr.augmentErr(err, reqID)
}

// Tracker tracks client ordinal, channel ID, and request sequence numbers for a gRPC channel.
type Tracker struct {
	clientID   int
	channelID  uint64
	nthRequest *atomic.Uint32
}

// NewTracker constructs a Tracker for the given clientID, channelID, and shared request counter.
// nthRequest must not be nil.
func NewTracker(clientID int, channelID uint64, nthRequest *atomic.Uint32) Tracker {
	if nthRequest == nil {
		panic("requestid: NewTracker requires non-nil nthRequest")
	}
	return Tracker{
		clientID:   clientID,
		channelID:  channelID,
		nthRequest: nthRequest,
	}
}

func (t *Tracker) nextNthRequest() uint32 {
	return t.nthRequest.Add(1)
}

func (t *Tracker) formatRequestID(nthRequest, attempt uint32) string {
	// Google Engineering has requested that each value be added in Decimal unpadded.
	return fmt.Sprintf("%d.%s.%d.%d.%d.%d", Version, randIDForProcess, t.clientID, t.channelID, nthRequest, attempt)
}

func (t *Tracker) generateAndInsertRequestID(md *metadata.MD, nthRequest, attempt uint32) {
	reqID := t.formatRequestID(nthRequest, attempt)
	if *md == nil {
		*md = metadata.MD{}
	}
	md.Set(Header, reqID)
}

// OptsWithNextRequestID appends a per-attempt request-ID CallOption unless priors already have one.
func (t *Tracker) OptsWithNextRequestID(priors []gax.CallOption) []gax.CallOption {
	if callOptionsHaveRequestID(priors) {
		return priors
	}
	return append(priors[:len(priors):len(priors)], &retryerWithRequestID{tracker: t})
}

// NewWrap allocates a new Wrap for streaming RPCs that advances nthRequest once and
// increments the attempt number on each retry.
func (t *Tracker) NewWrap() *Wrap {
	return &Wrap{
		md:         new(metadata.MD),
		nthRequest: t.nextNthRequest(),
		tracker:    t,
	}
}

// retryerWithRequestID is a gax.CallOption that injects "x-goog-spanner-request-id"
// into every RPC, and appropriately increments the RPC's attempt number per retry.
type retryerWithRequestID struct {
	tracker *Tracker
}

var _ gax.CallOption = (*retryerWithRequestID)(nil)
var _ logicalRequestKeyOption = (*retryerWithRequestID)(nil)

func (*retryerWithRequestID) logicalRequestKey() string {
	return ""
}

func (wr *retryerWithRequestID) Resolve(cs *gax.CallSettings) {
	nthRequest := wr.tracker.nextNthRequest()
	attempt := uint32(1)
	// Inject the first request-id header.
	// Note: after every gax.Invoke call, all the gRPC option headers are cleared out
	// and nullified, but yet cs.GRPC still contains a reference to the inserted *metadata.MD
	// just that it got cleared out and nullified. However, for retries we need to retain control
	// of the entry to re-insert the updated request-id on every call, hence why we are creating
	// and retaining a pointer reference to the metadata and shall be re-inserting the header value
	// on every retry.
	md := new(metadata.MD)
	wr.tracker.generateAndInsertRequestID(md, nthRequest, attempt)
	// Insert our grpc.CallOption that'll be updated by reference on every retry attempt.
	cs.GRPC = append(cs.GRPC[:len(cs.GRPC):len(cs.GRPC)], grpc.Header(md))

	if cs.Retry == nil {
		// If there was no retry manager, our journey has ended.
		return
	}

	originalRetryer := cs.Retry()
	cs.Retry = func() gax.Retryer {
		return (WrapRetryFn)(func(err error) (pause time.Duration, shouldRetry bool) {
			attempt++
			wr.tracker.generateAndInsertRequestID(md, nthRequest, attempt)
			return originalRetryer.Retry(err)
		})
	}
}

// WrapRetryFn adapts a retry function to implement gax.Retryer.
type WrapRetryFn func(err error) (time.Duration, bool)

// Retry implements gax.Retryer.
func (fn WrapRetryFn) Retry(err error) (time.Duration, bool) {
	return fn(err)
}

// Wrap holds the mutable metadata and logical request ordinal for a streaming RPC across retries.
type Wrap struct {
	md         *metadata.MD
	nthRequest uint32
	tracker    *Tracker
}

// WithNextRetryAttempt updates the wrapped metadata with the given attempt number and returns a CallOption.
func (riw *Wrap) WithNextRetryAttempt(attempt uint32) gax.CallOption {
	riw.tracker.generateAndInsertRequestID(riw.md, riw.nthRequest, attempt)
	return callOption{md: riw.md}
}

// LogicalWrap builds retry CallOptions from a pre-existing logical request key (e.g. "1.proc.1.1.5").
type LogicalWrap struct {
	LogicalKey string
}

// WithNextRetryAttempt returns a CallOption setting x-goog-spanner-request-id to "<LogicalKey>.<attempt>".
func (riw LogicalWrap) WithNextRetryAttempt(attempt uint32) gax.CallOption {
	md := metadata.MD{Header: []string{fmt.Sprintf("%s.%d", riw.LogicalKey, attempt)}}
	return callOption{md: &md}
}

type callOption struct {
	md *metadata.MD
}

var _ logicalRequestKeyOption = callOption{}

func (opt callOption) Resolve(cs *gax.CallSettings) {
	if opt.md == nil {
		return
	}
	cs.GRPC = stripRequestIDGRPCOptions(cs.GRPC)
	cs.GRPC = append(cs.GRPC, grpc.Header(opt.md))
}

func (opt callOption) logicalRequestKey() string {
	if opt.md == nil {
		return ""
	}
	reqIDs := opt.md.Get(Header)
	if len(reqIDs) == 0 || reqIDs[0] == "" {
		return ""
	}
	return RequestID(reqIDs[0]).LogicalRequestKey()
}

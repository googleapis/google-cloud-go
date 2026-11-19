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
	"io"
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

// literalRequestIDGRPCOption carries an immutable x-goog-spanner-request-id across
// gRPC calls and gax retries without allocating metadata.MD or wrapping cs.Retry.
type literalRequestIDGRPCOption struct {
	grpc.EmptyCallOption
	reqID RequestID
}

func extractRequestID(opts []grpc.CallOption) (RequestID, bool) {
	for _, opt := range opts {
		switch o := opt.(type) {
		case literalRequestIDGRPCOption:
			if o.reqID != "" {
				return o.reqID, true
			}
		case grpc.HeaderCallOption:
			if o.HeaderAddr == nil {
				continue
			}
			reqIDs := o.HeaderAddr.Get(Header)
			if len(reqIDs) != 0 && len(reqIDs[0]) != 0 {
				return RequestID(reqIDs[0]), true
			}
		}
	}
	return "", false
}

// GRPCCallOptionsToRequestID extracts the x-goog-spanner-request-id header from gRPC CallOptions.
func GRPCCallOptionsToRequestID(opts []grpc.CallOption) (md metadata.MD, reqID RequestID, found bool) {
	for _, opt := range opts {
		switch o := opt.(type) {
		case literalRequestIDGRPCOption:
			if o.reqID != "" {
				md = metadata.MD{Header: []string{string(o.reqID)}}
				reqID = o.reqID
				found = true
				return
			}
		case grpc.HeaderCallOption:
			mdPtr := o.HeaderAddr
			if mdPtr == nil {
				continue
			}
			reqIDs := mdPtr.Get(Header)
			if len(reqIDs) != 0 && len(reqIDs[0]) != 0 {
				md = *mdPtr
				reqID = RequestID(reqIDs[0])
				found = true
				return
			}
		}
	}
	return
}

func resolveCallSettings(opts []gax.CallOption, includeRetryerPlaceholder bool) gax.CallSettings {
	var settings gax.CallSettings
	for _, opt := range opts {
		if opt == nil {
			continue
		}
		if _, isRetryer := opt.(*retryerWithRequestID); isRetryer {
			// Avoid calling (*retryerWithRequestID).Resolve during inspection so
			// Tracker.nthRequest is never incremented as a side effect.
			if includeRetryerPlaceholder {
				md := metadata.MD{Header: []string{"1"}}
				settings.GRPC = append(settings.GRPC[:len(settings.GRPC):len(settings.GRPC)], grpc.Header(&md))
			}
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
	// Fast path for a single request-ID option; equivalent to resolveCallSettings(opts, false).
	if len(opts) == 1 && opts[0] != nil {
		if reqIDOpt, ok := opts[0].(logicalRequestKeyOption); ok {
			return reqIDOpt.logicalRequestKey()
		}
	}

	settings := resolveCallSettings(opts, false)
	reqID, found := extractRequestID(settings.GRPC)
	if !found {
		return ""
	}
	return reqID.LogicalRequestKey()
}

func callOptionsHaveRequestID(opts []gax.CallOption) bool {
	if len(opts) == 0 {
		return false
	}
	settings := resolveCallSettings(opts, true)
	_, found := extractRequestID(settings.GRPC)
	return found
}

func isRequestIDGRPCOption(opt grpc.CallOption) bool {
	switch o := opt.(type) {
	case literalRequestIDGRPCOption:
		return true
	case grpc.HeaderCallOption:
		return o.HeaderAddr != nil && len(o.HeaderAddr.Get(Header)) > 0
	default:
		return false
	}
}

// stripRequestIDGRPCOptions returns a slice without any gRPC CallOption carrying
// x-goog-spanner-request-id (either grpc.HeaderCallOption or literalRequestIDGRPCOption).
// It never mutates opts's backing array and never returns a slice sharing spare capacity
// with opts (which subsequent Resolve appends rely on).
func stripRequestIDGRPCOptions(opts []grpc.CallOption) []grpc.CallOption {
	if len(opts) == 0 {
		return nil
	}
	hasReqID := false
	for _, opt := range opts {
		if isRequestIDGRPCOption(opt) {
			hasReqID = true
			break
		}
	}
	if !hasReqID {
		return opts[:len(opts):len(opts)]
	}
	filtered := make([]grpc.CallOption, 0, len(opts))
	for _, opt := range opts {
		if !isRequestIDGRPCOption(opt) {
			filtered = append(filtered, opt)
		}
	}
	return filtered
}

// ErrorAugmenter attaches reqID to a non-nil RPC error when appropriate.
//
// HeaderInjector passes non-nil errors other than io.EOF to ErrorAugmenter; if an
// implementation returns nil, HeaderInjector preserves the original non-nil error.
// Implementations should also return sentinel errors (such as context.Canceled or
// iterator.Done) unchanged when callers check them by identity.
type ErrorAugmenter func(err error, reqID string) error

func applyErrorAugmenter(augment ErrorAugmenter, err error, reqID RequestID) error {
	if err == nil || err == io.EOF || augment == nil {
		return err
	}
	if augmented := augment(err, string(reqID)); augmented != nil {
		return augmented
	}
	return err
}

// HeaderInjector is a gRPC unary and stream client interceptor that propagates
// x-goog-spanner-request-id from CallOptions into outgoing metadata, OpenTelemetry
// span attributes, and returned errors.
type HeaderInjector struct {
	augment ErrorAugmenter
}

// NewHeaderInjector creates a HeaderInjector using augment to attach request IDs to errors.
// When augment is nil, request-ID headers and span attributes are still injected,
// returned errors are left unmodified, and client streams are not wrapped.
func NewHeaderInjector(augment ErrorAugmenter) *HeaderInjector {
	return &HeaderInjector{augment: augment}
}

func injectRequestIDIntoContext(ctx context.Context, opts []grpc.CallOption) (context.Context, RequestID, bool) {
	// It is imperative to search for the requestID before the call
	// because gRPC's internals will consume the headers.
	reqID, found := extractRequestID(opts)
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
	if !foundRequestID || wr == nil {
		return err
	}
	return applyErrorAugmenter(wr.augment, err, reqID)
}

type errWrappingClientStream struct {
	grpc.ClientStream
	reqID   RequestID
	augment ErrorAugmenter
}

func (rew *errWrappingClientStream) SendMsg(msg any) error {
	err := rew.ClientStream.SendMsg(msg)
	return applyErrorAugmenter(rew.augment, err, rew.reqID)
}

func (rew *errWrappingClientStream) RecvMsg(msg any) error {
	err := rew.ClientStream.RecvMsg(msg)
	return applyErrorAugmenter(rew.augment, err, rew.reqID)
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
	return wcs, applyErrorAugmenter(wr.augment, err, reqID)
}

// Tracker is a lightweight value descriptor that tracks client ordinal, channel ID,
// and logical request sequence numbers for a gRPC channel.
// nthRequest is incremented once per new logical request (and is not advanced on retries).
type Tracker struct {
	clientID   int
	channelID  uint64
	nthRequest *atomic.Uint32
}

// NewTracker constructs a Tracker value sharing the client-wide nthRequest counter.
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

func (t Tracker) nextNthRequest() uint32 {
	return t.nthRequest.Add(1)
}

func (t Tracker) formatLogicalRequestKey(nthRequest uint32) string {
	return fmt.Sprintf("%d.%s.%d.%d.%d", Version, randIDForProcess, t.clientID, t.channelID, nthRequest)
}

func (t Tracker) formatRequestID(nthRequest, attempt uint32) string {
	// Google Engineering has requested that each value be added in Decimal unpadded.
	return fmt.Sprintf("%d.%s.%d.%d.%d.%d", Version, randIDForProcess, t.clientID, t.channelID, nthRequest, attempt)
}

func (t Tracker) generateAndInsertRequestID(md *metadata.MD, nthRequest, attempt uint32) {
	reqID := t.formatRequestID(nthRequest, attempt)
	if *md == nil {
		*md = metadata.MD{}
	}
	md.Set(Header, reqID)
}

// OptsWithNextRequestID appends a request-ID CallOption (which allocates one
// nthRequest ordinal per Resolve and increments only the attempt number across
// retries) unless priors already resolve an x-goog-spanner-request-id header.
func (t Tracker) OptsWithNextRequestID(priors []gax.CallOption) []gax.CallOption {
	if callOptionsHaveRequestID(priors) {
		return priors
	}
	return append(priors[:len(priors):len(priors)], &retryerWithRequestID{tracker: t})
}

// NewWrap allocates a new Wrap for streaming RPCs that advances nthRequest once and
// increments the attempt number on each retry.
func (t Tracker) NewWrap() *Wrap {
	return &Wrap{
		md:         new(metadata.MD),
		nthRequest: t.nextNthRequest(),
		tracker:    t,
	}
}

// retryerWithRequestID is a gax.CallOption that injects "x-goog-spanner-request-id"
// into every RPC, and appropriately increments the RPC's attempt number per retry.
type retryerWithRequestID struct {
	tracker Tracker
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
	tracker    Tracker
}

// LogicalRequestKey returns the logical request key (without attempt suffix) for riw,
// or "" if riw or its tracker is zero/nil.
func (riw *Wrap) LogicalRequestKey() string {
	if riw == nil || riw.tracker.nthRequest == nil {
		return ""
	}
	return riw.tracker.formatLogicalRequestKey(riw.nthRequest)
}

// WithNextRetryAttempt updates the wrapped metadata with the given attempt number and returns a CallOption.
func (riw *Wrap) WithNextRetryAttempt(attempt uint32) gax.CallOption {
	riw.tracker.generateAndInsertRequestID(riw.md, riw.nthRequest, attempt)
	return callOption{md: riw.md}
}

// WithRequestID returns an immutable CallOption that sets x-goog-spanner-request-id
// to the exact literal value reqID, replacing any prior request-ID gRPC CallOption
// on Resolve. When reqID is empty, the returned CallOption is a no-op.
//
// Because it carries reqID via a private grpc.EmptyCallOption rather than grpc.Header,
// it allocates no metadata.MD, does not touch cs.Retry, and is safe for repeated and
// concurrent reuse across RPCs and gax retries (including when trailing gax.WithRetry
// options are applied or retry factories return nil).
func WithRequestID(reqID string) gax.CallOption {
	return literalRequestIDOption(reqID)
}

type literalRequestIDOption string

var _ gax.CallOption = literalRequestIDOption("")
var _ logicalRequestKeyOption = literalRequestIDOption("")

func (opt literalRequestIDOption) Resolve(cs *gax.CallSettings) {
	if opt == "" {
		return
	}
	cs.GRPC = stripRequestIDGRPCOptions(cs.GRPC)
	cs.GRPC = append(cs.GRPC, literalRequestIDGRPCOption{reqID: RequestID(opt)})
}

func (opt literalRequestIDOption) logicalRequestKey() string {
	if opt == "" {
		return ""
	}
	return RequestID(opt).LogicalRequestKey()
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

// callOption replaces any prior x-goog-spanner-request-id gRPC header option with md.
// Its zero value (md == nil) is an intentional no-op.
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

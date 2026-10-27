// Copyright 2025 Google LLC
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

package storage

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"slices"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"cloud.google.com/go/auth"
	internalTrace "cloud.google.com/go/internal/trace"
	"cloud.google.com/go/storage/internal"
	"cloud.google.com/go/storage/internal/apiv2/storagepb"
	"github.com/googleapis/gax-go/v2/apierror"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	otelcodes "go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"
	"google.golang.org/api/googleapi"
	"google.golang.org/genproto/googleapis/rpc/errdetails"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/stats"
	"google.golang.org/grpc/status"
)

type traceContextKey string

const (
	cacheContextKey    traceContextKey = "bucketMetadataCache"
	bucketContextKey   traceContextKey = "bucketName"
	isBucketContextKey traceContextKey = "isBucketOp"
	traceAttributesKey traceContextKey = "traceAttributes"
)

func contextWithTraceAttributes(ctx context.Context, attrs []attribute.KeyValue) context.Context {
	return context.WithValue(ctx, traceAttributesKey, attrs)
}

func traceAttributesFromContext(ctx context.Context) ([]attribute.KeyValue, bool) {
	attrs, ok := ctx.Value(traceAttributesKey).([]attribute.KeyValue)
	return attrs, ok
}

const (
	defaultTracerName = "cloud.google.com/go/storage"
	gcpClientArtifact = "cloud.google.com/go/storage"
	gcpClientService  = "storage"
	storageURIAttrKey = "gcp.storage.uri"
)

// rpcSystemNameKey is the OTel semantic convention attribute key for the RPC
// system used by a span.
const rpcSystemNameKey = attribute.Key("rpc.system.name")

var (
	rpcSystemHTTP = rpcSystemNameKey.String("http")
	rpcSystemGRPC = rpcSystemNameKey.String("grpc")

	// Pre-built span start options for transport-level spans, so starting
	// those spans does not allocate a new option each time.
	rpcSystemHTTPOpt = trace.WithAttributes(rpcSystemHTTP)
	rpcSystemGRPCOpt = trace.WithAttributes(rpcSystemGRPC)
)

// isOTelTracingDevEnabled checks the development flag until experimental feature is launched.
// TODO: Remove development flag upon experimental launch.
func isOTelTracingDevEnabled() bool {
	return os.Getenv(storageOtelTracingDevVar) == "true"
}

func isACOEnabled() bool {
	return os.Getenv(storageBucketMetadataDisabledVar) != "true"
}

func tracer() trace.Tracer {
	return otel.Tracer(defaultTracerName, trace.WithInstrumentationVersion(internal.Version))
}

func withStorageURI(bucket, object string) trace.SpanStartOption {
	if !isOTelTracingDevEnabled() || bucket == "" {
		return trace.WithAttributes()
	}
	return trace.WithAttributes(attribute.String(storageURIAttrKey, "gs://"+bucket+"/"+object))
}

func startSpanWithBucket(ctx context.Context, client *Client, bucket, name string, opts ...trace.SpanStartOption) (context.Context, trace.Span) {
	return startSpanWithObject(ctx, client, bucket, "", name, opts...)
}

func startSpanWithObject(ctx context.Context, client *Client, bucket, object, name string, opts ...trace.SpanStartOption) (context.Context, trace.Span) {
	if !isOTelTracingDevEnabled() {
		return startSpan(ctx, name, opts...)
	}
	opts = append(opts, withStorageURI(bucket, object))
	if client != nil && client.rpcSystem.Valid() {
		opts = append(opts, trace.WithAttributes(client.rpcSystem))
	}
	if client != nil && client.bucketMetadataCache != nil && bucket != "" {
		ctx = context.WithValue(ctx, cacheContextKey, client.bucketMetadataCache)
		ctx = context.WithValue(ctx, bucketContextKey, bucket)
		isBucket := strings.HasPrefix(name, "Bucket.") || strings.HasPrefix(name, "ACL.") || strings.HasPrefix(name, "storage.IAM.")
		ctx = context.WithValue(ctx, isBucketContextKey, isBucket)

		cache := client.bucketMetadataCache
		meta, hit := cache.get(bucket)
		if !hit {
			placeholder := bucketMetadata{
				resource:    fmt.Sprintf("projects/_/buckets/%s", bucket),
				location:    "global",
				placeholder: true,
			}
			cache.put(bucket, placeholder)
			cache.fetchBackground(ctx, bucket)
			meta = placeholder
		}
		attrs := []attribute.KeyValue{
			attribute.String("gcp.resource.destination.id", destinationResourceName(meta.resource)),
			attribute.String("gcp.resource.destination.location", meta.location),
		}
		ctx = contextWithTraceAttributes(ctx, attrs)
	}
	return startSpan(ctx, name, opts...)
}

// storageResourceNamePrefix is prepended to the bucket resource name only when
// it is emitted in the gcp.resource.destination.id span attribute. Cloud
// Trace's App Hub extractor only accepts full resource names of the form
// "//{service}/{path}"; a bare "projects/.../buckets/..." path is rejected as
// malformed and the span is silently dropped from App Hub enrichment. The
// bucket metadata cache and fetchBucketMetadata keep the bare
// "projects/{p}/buckets/{b}" form.
const storageResourceNamePrefix = "//storage.googleapis.com/"

// destinationResourceName converts a bare bucket resource name
// ("projects/{p}/buckets/{b}") into the full resource name
// ("//storage.googleapis.com/projects/{p}/buckets/{b}") required by Cloud
// Trace's App Hub extractor for the gcp.resource.destination.id attribute.
func destinationResourceName(resource string) string {
	return storageResourceNamePrefix + resource
}

// startSpan creates a span and a context.Context containing the newly-created span.
// If the context.Context provided in `ctx` contains a span then the newly-created
// span will be a child of that span, otherwise it will be a root span.
func startSpan(ctx context.Context, name string, opts ...trace.SpanStartOption) (context.Context, trace.Span) {
	if ctx.Value(apiMethodKey{}) == nil {
		ctx = context.WithValue(ctx, apiMethodKey{}, name)
	}
	name = appendPackageName(name)
	// TODO: Remove internalTrace upon experimental launch.
	if !isOTelTracingDevEnabled() {
		ctx = internalTrace.StartSpan(ctx, name)
		return ctx, nil
	}
	opts = append(opts, getCommonTraceOptions()...)
	if attrs, ok := traceAttributesFromContext(ctx); ok {
		opts = append(opts, trace.WithAttributes(attrs...))
	}
	ctx, span := tracer().Start(ctx, name, opts...)
	return ctx, span
}

func isNotFoundError(err error) bool {
	if errors.Is(err, ErrBucketNotExist) {
		return true
	}
	var e *googleapi.Error
	if s, ok := status.FromError(err); (ok && s.Code() == codes.NotFound) ||
		(errors.As(err, &e) && e.Code == http.StatusNotFound) {
		return true
	}
	return false
}

// errorAttributes returns the error.type and, when known, gcp.errors.domain
// span attributes for err. It returns no attributes for a nil error or io.EOF.
//
// error.type reuses computeErrorType, the classifier behind the error.type
// metric attribute, so spans and metrics report the same value for a failure.
// gcp.errors.domain is the google.rpc.ErrorInfo domain of the error response.
func errorAttributes(err error) []attribute.KeyValue {
	if err == nil || errors.Is(err, io.EOF) {
		return nil
	}
	// Infer the transport from the error, since endSpan does not know it:
	// HTTP errors always carry a *googleapi.Error.
	var httpErr *googleapi.Error
	isHTTP := errors.As(err, &httpErr)
	attrs := []attribute.KeyValue{attribute.String("error.type", computeErrorType(err, isHTTP, 0))}
	if domain := errorDomain(err); domain != "" {
		attrs = append(attrs, attribute.String("gcp.errors.domain", domain))
	}
	return attrs
}

// errorDomain returns the google.rpc.ErrorInfo domain of err, or "" if the
// error response carried no ErrorInfo.
func errorDomain(err error) string {
	// Check for an existing APIError first: the generated JSON client returns
	// a googleapi.Error wrapping an APIError, and ParseError on that prefers
	// the APIError's mapped gRPC status, which carries no ErrorInfo.
	var apiErr *apierror.APIError
	if errors.As(err, &apiErr) {
		return apiErr.Domain()
	}
	var httpErr *googleapi.Error
	if errors.As(err, &httpErr) {
		if apiErr, ok := apierror.ParseError(httpErr, false); ok {
			return apiErr.Domain()
		}
		return ""
	}
	// For gRPC, read the ErrorInfo directly instead of parsing every detail
	// with apierror.ParseError.
	if st, ok := status.FromError(err); ok {
		for _, d := range st.Details() {
			if info, ok := d.(*errdetails.ErrorInfo); ok {
				return info.GetDomain()
			}
		}
	}
	return ""
}

// endSpan ends the operation span in ctx. On a not-found error it also evicts
// the operation's bucket from the bucket metadata cache, then calls finishSpan.
// Non-operation spans should call finishSpan directly.
func endSpan(ctx context.Context, err error) {
	if err != nil && isNotFoundError(err) {
		isBucket, _ := ctx.Value(isBucketContextKey).(bool)
		cache, _ := ctx.Value(cacheContextKey).(*bucketMetadataCache)
		bucket, _ := ctx.Value(bucketContextKey).(string)

		if isBucket && cache != nil && bucket != "" {
			cache.evict(bucket)
		}
	}
	finishSpan(ctx, err)
}

// finishSpan retrieves the current span from ctx and completes the span.
// If an error occurs, the error.type and, when known, gcp.errors.domain
// attributes are set, the error is recorded as an exception span event for
// this span, and the span status is set in the form of a code and a
// description.
func finishSpan(ctx context.Context, err error) {
	// TODO: Remove internalTrace upon experimental launch.
	if !isOTelTracingDevEnabled() {
		internalTrace.EndSpan(ctx, err)
	} else {
		span := trace.SpanFromContext(ctx)
		if err != nil {
			span.SetAttributes(errorAttributes(err)...)
			span.SetStatus(otelcodes.Error, err.Error())
			span.RecordError(err)
		}
		span.End()
	}
}

// getCommonTraceOptions makes a SpanStartOption with common attributes.
func getCommonTraceOptions() []trace.SpanStartOption {
	opts := []trace.SpanStartOption{
		trace.WithAttributes(getCommonAttributes()...),
	}
	return opts
}

// getCommonAttributes includes the common attributes used for Cloud Trace adoption tracking.
func getCommonAttributes() []attribute.KeyValue {
	return []attribute.KeyValue{
		attribute.String("gcp.client.service", gcpClientService),
		attribute.String("gcp.client.version", internal.Version),
		attribute.String("gcp.client.artifact", gcpClientArtifact),
	}
}

func appendPackageName(spanName string) string {
	return fmt.Sprintf("%s.%s", gcpClientArtifact, spanName)
}

// recordRetryBackoff records the backoff that started at start as a
// RetryBackoff child span of the span in ctx. It is a no-op unless dev tracing
// is enabled and the span in ctx is recording.
func recordRetryBackoff(ctx context.Context, start time.Time) {
	if !isOTelTracingDevEnabled() {
		return
	}
	span := trace.SpanFromContext(ctx)
	if !span.IsRecording() {
		return
	}
	backoffCtx, _ := startSpan(ctx, "RetryBackoff", trace.WithTimestamp(start))
	finishSpan(backoffCtx, nil)
}

// retryAttemptKey is the context key under which run records the attempt it
// is about to make.
type retryAttemptKey struct{}

// retryAttempt describes an attempt made by run.
type retryAttempt struct {
	// number is the attempt number, matching the gccl-attempt-count value sent
	// to the service. It restarts at 1 when run is given withProgressReset.
	number int
	// parentCtx is the context passed to run, whose active span is the
	// operation span.
	parentCtx context.Context
	// parent is the span context of the span in run's ctx, usually the
	// operation span. It lets retryAttemptStatsHandler tell the transport's
	// attempt span apart from it.
	parent trace.SpanContext
}

// withRetryAttempt returns ctx annotated with the given attempt number so that
// retryAttemptStatsHandler can record it on the gRPC attempt span.
func withRetryAttempt(ctx context.Context, attempt int) context.Context {
	if !isOTelTracingDevEnabled() {
		return ctx
	}
	return context.WithValue(ctx, retryAttemptKey{}, retryAttempt{
		number:    attempt,
		parentCtx: ctx,
		parent:    trace.SpanContextFromContext(ctx),
	})
}

// retryAttemptStatsHandler records gcp.client.retry.attempt_number on the
// transport's gRPC attempt span and Storage.UploadChunk spans for resumable
// BidiWriteObject streams.
type retryAttemptStatsHandler struct{}

// TagRPC implements stats.Handler.
func (retryAttemptStatsHandler) TagRPC(ctx context.Context, info *stats.RPCTagInfo) context.Context {
	attempt, ok := ctx.Value(retryAttemptKey{}).(retryAttempt)
	if !ok {
		return ctx
	}
	span := trace.SpanFromContext(ctx)
	// If the transport did not start an attempt span (for example because
	// telemetry is disabled), the span in ctx is still the operation span,
	// which must not be tagged.
	if span.IsRecording() && span.SpanContext().SpanID() != attempt.parent.SpanID() {
		span.SetAttributes(attribute.Int("gcp.client.retry.attempt_number", attempt.number))
	}
	if chunkSize, ok := ctx.Value(uploadChunkSizeKey{}).(int64); ok && chunkSize > 0 &&
		info != nil && strings.HasSuffix(info.FullMethodName, "/BidiWriteObject") {
		ctx = context.WithValue(ctx, grpcUploadChunkKey{}, &grpcUploadChunkTracker{
			parentCtx: attempt.parentCtx,
			chunkSize: chunkSize,
		})
	}
	return ctx
}

// HandleRPC implements stats.Handler.
func (retryAttemptStatsHandler) HandleRPC(ctx context.Context, rs stats.RPCStats) {
	ct, ok := ctx.Value(grpcUploadChunkKey{}).(*grpcUploadChunkTracker)
	if !ok {
		return
	}
	switch s := rs.(type) {
	case *stats.OutPayload:
		if req, ok := s.Payload.(*storagepb.BidiWriteObjectRequest); ok && s.Client {
			ct.onRequest(req)
		}
	case *stats.InPayload:
		if resp, ok := s.Payload.(*storagepb.BidiWriteObjectResponse); ok && s.Client {
			ct.onResponse(resp)
		}
	case *stats.End:
		if s.Client {
			ct.onEnd(s.Error)
		}
	}
}

// TagConn implements stats.Handler.
func (retryAttemptStatsHandler) TagConn(ctx context.Context, _ *stats.ConnTagInfo) context.Context {
	return ctx
}

// HandleConn implements stats.Handler.
func (retryAttemptStatsHandler) HandleConn(context.Context, stats.ConnStats) {}

func recordingSpan(ctx context.Context) (trace.Span, bool) {
	if !isOTelTracingDevEnabled() {
		return nil, false
	}
	span := trace.SpanFromContext(ctx)
	return span, span.IsRecording()
}

// recordWriterTraceAttributes attaches the upload strategy to the Object.Writer
// span in ctx as gcp.storage.write.mode (resumable, oneshot, appendable or
// parallel). For parallel uploads, the configured part size and concurrency are
// also recorded.
func recordWriterTraceAttributes(ctx context.Context, w *Writer) {
	span, ok := recordingSpan(ctx)
	if !ok {
		return
	}
	if w.EnableParallelUpload {
		span.SetAttributes(
			attribute.String("gcp.storage.write.mode", "parallel"),
			attribute.Int("gcp.storage.write.parallel.part_size", w.ParallelUploadConfig.PartSize),
			attribute.Int("gcp.storage.write.parallel.concurrency", w.ParallelUploadConfig.MaxConcurrency),
		)
		return
	}
	writeMode := "resumable"
	if w.Append {
		writeMode = "appendable"
	} else if isOneshotUpload(w) {
		writeMode = "oneshot"
	}
	span.SetAttributes(attribute.String("gcp.storage.write.mode", writeMode))
}

// isOneshotUpload reports whether the Writer performs a single-request upload.
// For HTTP, payloads smaller than the 256 KiB-aligned ChunkSize are sent in a
// single request (matching googleapi.ChunkSize).
func isOneshotUpload(w *Writer) bool {
	if gw, ok := w.iw.(*gRPCWriter); ok {
		return gw.usesOneshotSender()
	}
	if w.ChunkSize <= 0 {
		return true
	}
	return w.opened && w.closed && atomic.LoadInt64(&w.bytesWritten) < int64(gRPCChunkSize(w.ChunkSize))
}

const (
	uploadChunkSpanName   = "Storage.UploadChunk"
	uploadChunkNumberAttr = "gcp.storage.upload.chunk.number"
	payloadOffsetAttr     = "gcp.storage.payload.offset"
	payloadSizeBytesAttr  = "gcp.storage.payload.size_bytes"
)

type uploadChunkSizeKey struct{}
type grpcUploadChunkKey struct{}

// withUploadChunkSize attaches the 256 KiB-aligned chunk size to ctx for
// resumable uploads when dev tracing is enabled.
func withUploadChunkSize(ctx context.Context, chunkSize int) context.Context {
	if chunkSize <= 0 {
		return ctx
	}
	if _, ok := recordingSpan(ctx); !ok {
		return ctx
	}
	return context.WithValue(ctx, uploadChunkSizeKey{}, int64(gRPCChunkSize(chunkSize)))
}

// uploadChunkSpan is the Storage.UploadChunk span of one logical chunk of a
// gRPC resumable upload, which may be sent across several messages.
type uploadChunkSpan struct {
	ctx       context.Context
	span      trace.Span
	endOffset int64
	sizeBytes int64
	flushed   bool
}

func (s uploadChunkSpan) finish(err error) {
	s.span.SetAttributes(attribute.Int64(payloadSizeBytesAttr, s.sizeBytes))
	finishSpan(s.ctx, err)
}

// grpcUploadChunkTracker tracks Storage.UploadChunk spans for one gRPC
// BidiWriteObject stream.
type grpcUploadChunkTracker struct {
	parentCtx context.Context
	chunkSize int64

	mu              sync.Mutex
	spans           []uploadChunkSpan
	lastEndOffset   int64
	confirmedOffset int64
	finalized       bool
	ended           bool
	endErr          error
}

func (ct *grpcUploadChunkTracker) onRequest(req *storagepb.BidiWriteObjectRequest) {
	n := int64(len(req.GetChecksummedData().GetContent()))
	if n == 0 {
		return
	}
	offset := req.GetWriteOffset()
	flushed := req.GetFlush() || req.GetFinishWrite()

	ct.mu.Lock()
	defer ct.mu.Unlock()
	if ct.ended && offset < ct.lastEndOffset {
		return
	}
	if offset+n > ct.lastEndOffset {
		ct.lastEndOffset = offset + n
	}
	if m := len(ct.spans); m > 0 && !ct.spans[m-1].flushed {
		ct.spans[m-1].endOffset = offset + n
		ct.spans[m-1].sizeBytes += n
		ct.spans[m-1].flushed = flushed
	} else {
		number := offset/ct.chunkSize + 1
		ctx, span := startSpan(ct.parentCtx, uploadChunkSpanName,
			trace.WithAttributes(
				attribute.Int64(uploadChunkNumberAttr, number),
				attribute.Int64(payloadOffsetAttr, offset),
			))
		ct.spans = append(ct.spans, uploadChunkSpan{
			ctx:       ctx,
			span:      span,
			endOffset: offset + n,
			sizeBytes: n,
			flushed:   flushed,
		})
	}
	if ct.ended {
		ct.finishAllLocked(ct.endErr)
		return
	}
	ct.finishConfirmedLocked()
}

func (ct *grpcUploadChunkTracker) onResponse(resp *storagepb.BidiWriteObjectResponse) {
	var flushOffset int64
	var finalized bool
	switch ws := resp.GetWriteStatus().(type) {
	case *storagepb.BidiWriteObjectResponse_PersistedSize:
		flushOffset = ws.PersistedSize
	case *storagepb.BidiWriteObjectResponse_Resource:
		flushOffset = ws.Resource.GetSize()
		finalized = ws.Resource != nil
	default:
		return
	}
	ct.mu.Lock()
	defer ct.mu.Unlock()
	if flushOffset > ct.confirmedOffset {
		ct.confirmedOffset = flushOffset
	}
	if finalized {
		ct.finalized = true
	}
	ct.finishConfirmedLocked()
}

func (ct *grpcUploadChunkTracker) onEnd(err error) {
	ct.mu.Lock()
	defer ct.mu.Unlock()
	ct.ended = true
	ct.endErr = err
	ct.finishAllLocked(err)
}

func (ct *grpcUploadChunkTracker) finishConfirmedLocked() {
	i := 0
	for i < len(ct.spans) && ct.spans[i].flushed && (ct.finalized || ct.confirmedOffset >= ct.spans[i].endOffset) {
		ct.spans[i].finish(nil)
		i++
	}
	ct.spans = slices.Delete(ct.spans, 0, i)
}

func (ct *grpcUploadChunkTracker) finishAllLocked(err error) {
	for _, s := range ct.spans {
		s.finish(err)
	}
	clear(ct.spans)
	ct.spans = ct.spans[:0]
}

// startHTTPUploadChunkSpan starts a Storage.UploadChunk span for an HTTP
// resumable upload chunk request when dev tracing is enabled.
func startHTTPUploadChunkSpan(ctx context.Context, h http.Header) (context.Context, bool) {
	chunkNum, offset, sizeBytes, ok := httpUploadChunkInfo(ctx, h)
	if !ok {
		return ctx, false
	}
	ctx, _ = startSpan(ctx, uploadChunkSpanName,
		trace.WithAttributes(
			attribute.Int64(uploadChunkNumberAttr, chunkNum),
			attribute.Int64(payloadOffsetAttr, offset),
			attribute.Int64(payloadSizeBytesAttr, sizeBytes),
		))
	return ctx, true
}

// httpUploadChunkInfo parses the Content-Range header of an HTTP resumable
// upload chunk request and returns its 1-based chunk number, byte offset, and
// payload size in bytes.
// A zero-byte finalization request ("bytes */<size>") does not upload a chunk
// and returns false.
func httpUploadChunkInfo(ctx context.Context, h http.Header) (chunkNum, offset, sizeBytes int64, ok bool) {
	rest, found := strings.CutPrefix(h.Get("Content-Range"), "bytes ")
	if !found {
		return 0, 0, 0, false
	}
	chunkSize, valid := ctx.Value(uploadChunkSizeKey{}).(int64)
	if !valid || chunkSize <= 0 {
		return 0, 0, 0, false
	}
	rangePart, _, found := strings.Cut(rest, "/")
	if !found {
		return 0, 0, 0, false
	}
	startStr, endStr, found := strings.Cut(rangePart, "-")
	if !found {
		return 0, 0, 0, false
	}
	offset, err := strconv.ParseInt(startStr, 10, 64)
	if err != nil || offset < 0 {
		return 0, 0, 0, false
	}
	end, err := strconv.ParseInt(endStr, 10, 64)
	if err != nil || end < offset {
		return 0, 0, 0, false
	}
	return offset/chunkSize + 1, offset, end - offset + 1, true
}

// httpUploadChunkError returns the error to record on an HTTP
// Storage.UploadChunk span for a RoundTrip result.
func httpUploadChunkError(resp *http.Response, err error) error {
	if err != nil {
		return err
	}
	if resp != nil && (resp.StatusCode < 200 || resp.StatusCode > 299) && resp.StatusCode != 308 {
		return &googleapi.Error{Code: resp.StatusCode, Message: http.StatusText(resp.StatusCode)}
	}
	return nil
}

// tracedTokenProvider records an Auth.RefreshAccessToken span for token fetches
// that block a request. Cache hits and background refreshes (no recording span
// in ctx) are skipped.
type tracedTokenProvider struct {
	base auth.TokenProvider
	last atomic.Pointer[auth.Token] // cache hits return the same *auth.Token
}

func (p *tracedTokenProvider) Token(ctx context.Context) (*auth.Token, error) {
	start := time.Now()
	tok, err := p.base.Token(ctx)
	// Update the last-seen token regardless of ctx so cache-hit detection
	// stays correct for later calls.
	if err == nil && p.last.Swap(tok) == tok {
		return tok, nil // cache hit, no span
	}
	if !trace.SpanFromContext(ctx).IsRecording() {
		return tok, err
	}
	ctx, _ = startSpan(ctx, "Auth.RefreshAccessToken", trace.WithTimestamp(start))
	finishSpan(ctx, err)
	return tok, err
}

// wrapAuthCredentialsForTracing wraps an auth.Credentials object to record
// Auth.RefreshAccessToken internal operation spans on blocking token fetches
// and refreshes made under a recording span (cache hits and background
// refreshes do not emit spans).
func wrapAuthCredentialsForTracing(c *auth.Credentials) *auth.Credentials {
	if c == nil || c.TokenProvider == nil {
		return c
	}
	if hasTracedTokenProvider(c.TokenProvider) {
		return c
	}
	clone := *c
	clone.TokenProvider = &tracedTokenProvider{base: c.TokenProvider}
	return &clone
}

func hasTracedTokenProvider(tp auth.TokenProvider) bool {
	for tp != nil {
		switch p := tp.(type) {
		case *tracedTokenProvider:
			return true
		case *metricsTokenProvider:
			tp = p.base
		default:
			return false
		}
	}
	return false
}

// recordUploadID sets gcp.storage.upload.id on the span in ctx.
func recordUploadID(ctx context.Context, uploadID string) {
	if span, ok := recordingSpan(ctx); ok && uploadID != "" {
		span.SetAttributes(attribute.String("gcp.storage.upload.id", uploadID))
	}
}

// recordUploadIDFromHeader records the upload_id query parameter of a
// resumable-session Location header, if present.
func recordUploadIDFromHeader(ctx context.Context, h http.Header) {
	if loc := h.Get("Location"); loc != "" {
		if u, err := url.Parse(loc); err == nil {
			recordUploadID(ctx, u.Query().Get("upload_id"))
		}
	}
}

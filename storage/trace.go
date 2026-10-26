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
	"strings"
	"sync/atomic"
	"time"

	internalTrace "cloud.google.com/go/internal/trace"
	"cloud.google.com/go/storage/internal"
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
	ctx = context.WithValue(ctx, apiMethodKey{}, name)
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

// endSpan retrieves the current span from ctx and completes the span.
// If an error occurs, the error.type and, when known, gcp.errors.domain
// attributes are set, the error is recorded as an exception span event for
// this span, and the span status is set in the form of a code and a
// description.
func endSpan(ctx context.Context, err error) {
	if err != nil && isNotFoundError(err) {
		isBucket, _ := ctx.Value(isBucketContextKey).(bool)
		cache, _ := ctx.Value(cacheContextKey).(*bucketMetadataCache)
		bucket, _ := ctx.Value(bucketContextKey).(string)

		if isBucket && cache != nil && bucket != "" {
			cache.evict(bucket)
		}
	}

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
	_, backoffSpan := startSpan(ctx, "RetryBackoff", trace.WithTimestamp(start))
	backoffSpan.End()
}

// retryAttemptKey is the context key under which run records the attempt it
// is about to make.
type retryAttemptKey struct{}

// retryAttempt describes an attempt made by run.
type retryAttempt struct {
	// number is the attempt number, matching the gccl-attempt-count value sent
	// to the service. It restarts at 1 when run is given withProgressReset.
	number int
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
		number: attempt,
		parent: trace.SpanContextFromContext(ctx),
	})
}

// retryAttemptStatsHandler records gcp.client.retry.attempt_number on the
// transport's gRPC attempt span.
type retryAttemptStatsHandler struct{}

// TagRPC implements stats.Handler.
func (retryAttemptStatsHandler) TagRPC(ctx context.Context, _ *stats.RPCTagInfo) context.Context {
	attempt, ok := ctx.Value(retryAttemptKey{}).(retryAttempt)
	if !ok {
		return ctx
	}
	span := trace.SpanFromContext(ctx)
	// If the transport did not start an attempt span (for example because
	// telemetry is disabled), the span in ctx is still the operation span,
	// which must not be tagged.
	if !span.IsRecording() || span.SpanContext().SpanID() == attempt.parent.SpanID() {
		return ctx
	}
	span.SetAttributes(attribute.Int("gcp.client.retry.attempt_number", attempt.number))
	return ctx
}

// HandleRPC implements stats.Handler.
func (retryAttemptStatsHandler) HandleRPC(context.Context, stats.RPCStats) {}

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

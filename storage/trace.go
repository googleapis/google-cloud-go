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
	"net/http"
	"os"
	"strings"

	internalTrace "cloud.google.com/go/internal/trace"
	"cloud.google.com/go/storage/internal"
	gax "github.com/googleapis/gax-go/v2"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	otelcodes "go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"
	"google.golang.org/api/googleapi"
	"google.golang.org/grpc/codes"
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

func startSpanWithBucket(ctx context.Context, client *Client, bucket string, name string, opts ...trace.SpanStartOption) (context.Context, trace.Span) {
	if !isOTelTracingDevEnabled() {
		return startSpan(ctx, name, opts...)
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
// span attributes for err. It returns no attributes for a nil error.
//
// error.type reuses computeErrorType, the classifier behind the error.type
// metric attribute, so spans and metrics report the same value for a failure.
// It intentionally does not use gax's TelemetryErrorInfo.ErrorType, so values
// can differ from the spans gax creates for the underlying RPCs (for example,
// TIMEOUT instead of CLIENT_TIMEOUT, and the gRPC or HTTP code instead of the
// google.rpc.ErrorInfo reason).
// gcp.errors.domain is the google.rpc.ErrorInfo domain, extracted with
// gax.ExtractTelemetryErrorInfo.
func errorAttributes(ctx context.Context, err error) []attribute.KeyValue {
	var attrs []attribute.KeyValue
	// Infer the transport from the error, since endSpan does not know it:
	// HTTP errors always carry a *googleapi.Error.
	var httpErr *googleapi.Error
	isHTTP := errors.As(err, &httpErr)
	// computeErrorType reports nil and io.EOF as "OK"; error.type must not be
	// set then.
	if errType := computeErrorType(err, isHTTP, 0); errType != "OK" {
		attrs = append(attrs, attribute.String("error.type", errType))
	}
	if domain := gax.ExtractTelemetryErrorInfo(ctx, err).Domain; domain != "" {
		attrs = append(attrs, attribute.String("gcp.errors.domain", domain))
	}
	return attrs
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
			span.SetAttributes(errorAttributes(ctx, err)...)
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

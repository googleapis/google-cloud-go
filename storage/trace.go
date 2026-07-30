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
	"time"

	internalTrace "cloud.google.com/go/internal/trace"
	"cloud.google.com/go/storage/internal"
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
	storageOtelTracingDevVar         = "GO_STORAGE_DEV_OTEL_TRACING"
	defaultTracerName                = "cloud.google.com/go/storage"
	gcpClientRepo                    = "googleapis/google-cloud-go"
	gcpClientArtifact                = "cloud.google.com/go/storage"
	storageBucketMetadataDisabledVar = "GO_OTEL_BUCKETMETADATA_DISABLED"
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
			cache.fetchBackground(bucket)
			meta = placeholder
		}
		attrs := []attribute.KeyValue{
			attribute.String("gcp.resource.destination.id", meta.resource),
			attribute.String("gcp.resource.destination.location", meta.location),
		}
		ctx = contextWithTraceAttributes(ctx, attrs)
	}
	return startSpan(ctx, name, opts...)
}

// startSpan creates a span and a context.Context containing the newly-created span.
// If the context.Context provided in `ctx` contains a span then the newly-created
// span will be a child of that span, otherwise it will be a root span.
func startSpan(ctx context.Context, name string, opts ...trace.SpanStartOption) (context.Context, trace.Span) {
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

// endSpan retrieves the current span from ctx and completes the span.
// If an error occurs, the error is recorded as an exception span event for this span,
// and the span status is set in the form of a code and a description.
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
		attribute.String("gcp.client.version", internal.Version),
		attribute.String("gcp.client.repo", gcpClientRepo),
		attribute.String("gcp.client.artifact", gcpClientArtifact),
	}
}

func appendPackageName(spanName string) string {
	return fmt.Sprintf("%s.%s", gcpClientArtifact, spanName)
}

// recordRetryBackoffEvent adds an event and a child span to the current span in ctx recording a retry backoff.
func recordRetryBackoffEvent(ctx context.Context, backoff time.Duration, attempt int, startTime time.Time) {
	if !isOTelTracingDevEnabled() {
		return
	}
	span := trace.SpanFromContext(ctx)
	if !span.IsRecording() {
		return
	}
	attrs := []attribute.KeyValue{
		attribute.Int("attempt", attempt),
		attribute.String("backoff", backoff.String()),
	}
	span.AddEvent("storage.retry.backoff", trace.WithAttributes(attrs...))

	// Create a child span for the backoff duration so it appears as a visual
	// block in Trace Explorer waterfall charts.
	if startTime.IsZero() {
		startTime = time.Now().Add(-backoff)
	}
	_, backoffSpan := startSpan(ctx, "RetryBackoff", trace.WithTimestamp(startTime))
	backoffSpan.SetAttributes(attrs...)
	backoffSpan.End(trace.WithTimestamp(startTime.Add(backoff)))
}

// recordWriterTraceAttributes attaches descriptive upload mode and configuration attributes
// to the Object.Writer span in ctx so observers can inspect the write type in Trace Explorer.
func recordWriterTraceAttributes(ctx context.Context, w *Writer) {
	if !isOTelTracingDevEnabled() || w == nil {
		return
	}
	span := trace.SpanFromContext(ctx)
	if !span.IsRecording() {
		return
	}

	writeMode := "resumable"
	if w.EnableParallelUpload {
		writeMode = "parallel_composite"
	} else if strings.HasPrefix(w.ObjectAttrs.Name, "gcs-go-sdk-pu-tmp/") {
		writeMode = "pcu_part_writer"
	} else if w.Append {
		writeMode = "appendable"
	} else if w.ChunkSize == 0 {
		writeMode = "oneshot"
	}

	attrs := []attribute.KeyValue{
		attribute.String("storage.write.mode", writeMode),
		attribute.Int("storage.write.chunk_size", w.ChunkSize),
		attribute.Bool("storage.write.append", w.Append),
		attribute.Bool("storage.write.parallel", w.EnableParallelUpload),
		attribute.String("storage.object.name", w.ObjectAttrs.Name),
	}
	if w.EnableParallelUpload {
		attrs = append(attrs,
			attribute.Int("storage.write.pcu_part_size", w.ParallelUploadConfig.PartSize),
			attribute.Int("storage.write.pcu_concurrency", w.ParallelUploadConfig.MaxConcurrency),
		)
	}
	span.SetAttributes(attrs...)
}

// recordReaderTraceAttributes attaches descriptive read mode and range attributes
// to the Object.Reader or Object.MultiRangeDownloader span in ctx.
func recordReaderTraceAttributes(ctx context.Context, readMode string, offset, length int64, objectName string) {
	if !isOTelTracingDevEnabled() {
		return
	}
	span := trace.SpanFromContext(ctx)
	if !span.IsRecording() {
		return
	}

	attrs := []attribute.KeyValue{
		attribute.String("storage.read.mode", readMode),
		attribute.String("storage.object.name", objectName),
	}
	if readMode == "range" || readMode == "full" {
		attrs = append(attrs,
			attribute.Int64("storage.read.offset", offset),
			attribute.Int64("storage.read.length", length),
		)
	}
	span.SetAttributes(attrs...)
}

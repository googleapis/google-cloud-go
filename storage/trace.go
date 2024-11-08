// Copyright 2024 Google LLC
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
	"os"

	"cloud.google.com/go/storage/internal"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	otelcodes "go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"
)

const (
	storageOtelTracingDevVar = "GO_STORAGE_DEV_OTEL_TRACING"
	defaultTracerName        = "cloud.google.com/go/storage"
	gcpClientRepo            = "googleapis/google-cloud-go"
	gcpClientArtifact        = "storage"
)

// isOTelTracingDevEnabled checks the development flag until experimental feature is launched.
func isOTelTracingDevEnabled() bool {
	return os.Getenv(storageOtelTracingDevVar) == "true"
}

func tracer() trace.Tracer {
	return otel.Tracer(defaultTracerName, trace.WithInstrumentationVersion(internal.Version))
}

// startSpan creates a span and a context.Context containing the newly-created span.
// If the context.Context provided in `ctx` contains a span then the newly-created
// span will be a child of that span, otherwise it will be a root span.
func startSpan(ctx context.Context, name string, opts ...trace.SpanStartOption) (context.Context, trace.Span) {
	opts = append(opts, getCommonTraceOptions()...)
	return tracer().Start(ctx, name, opts...)
}

// endSpan retrieves the current span from ctx and completes the span.
// If an error occurs, the error is recorded as an exception span event for this span,
// and the span status is set in the form of a code and a description.
func endSpan(ctx context.Context, err error) {
	span := trace.SpanFromContext(ctx)
	if err != nil {
		span.SetStatus(otelcodes.Error, err.Error())
		span.RecordError(err)
	}
	span.End()
}

// getCommonTraceOptions includes the common attributes used for Cloud Trace adoption tracking.
func getCommonTraceOptions() []trace.SpanStartOption {
	opts := []trace.SpanStartOption{
		trace.WithAttributes(
			attribute.String("gcp.client.version", internal.Version),
			attribute.String("gcp.client.repo", gcpClientRepo),
			attribute.String("gcp.client.artifact", gcpClientArtifact),
		),
	}
	return opts
}

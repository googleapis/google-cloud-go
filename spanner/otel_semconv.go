package spanner

import (
	"context"

	semconv "go.opentelemetry.io/otel/semconv/v1.34.0"
	ottrace "go.opentelemetry.io/otel/trace"
)

func addDbSemanticConventionAttributes(ctx context.Context, statement Statement) {
	span := ottrace.SpanFromContext(ctx)
	if span == nil {
		return
	}

	span.SetAttributes(semconv.DBSystemNameGCPSpanner)

	if statement.SQL != "" {
		span.SetAttributes(semconv.DBQueryText(statement.SQL))
	}
}

package test

import (
	"context"

	"go.opentelemetry.io/otel"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/metric/metricdata"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
)

// openTelemetryTestExporter is a test utility exporter. It should be created
// with newOpenTelemetryTestExporter.
type openTelemetryTestExporter struct {
	exporter *tracetest.InMemoryExporter
	tp       *sdktrace.TracerProvider

	metricReader *sdkmetric.ManualReader
	mp           *sdkmetric.MeterProvider
}

// newOpenTelemetryTestExporter creates a OpenTelemetryTestExporter with
// underlying InMemoryExporter and TracerProvider from OpenTelemetry.
func newOpenTelemetryTestExporter(globalMeter bool, globalTracer bool) *openTelemetryTestExporter {
	exporter := tracetest.NewInMemoryExporter()
	tp := sdktrace.NewTracerProvider(
		sdktrace.WithSyncer(exporter),
		sdktrace.WithSampler(sdktrace.AlwaysSample()),
	)
	if globalTracer {
		otel.SetTracerProvider(tp)
	}

	metricReader := sdkmetric.NewManualReader()
	mp := sdkmetric.NewMeterProvider(
		sdkmetric.WithReader(metricReader),
	)
	if globalMeter {
		otel.SetMeterProvider(mp)
	}
	return &openTelemetryTestExporter{
		exporter:     exporter,
		tp:           tp,
		metricReader: metricReader,
		mp:           mp,
	}
}

// spans returns the current in-memory stored spans.
func (te *openTelemetryTestExporter) spans() tracetest.SpanStubs {
	return te.exporter.GetSpans()
}

// metrics returns the current in-memory stored metrics.
func (te *openTelemetryTestExporter) metrics(ctx context.Context) (*metricdata.ResourceMetrics, error) {
	rm := metricdata.ResourceMetrics{}
	err := te.metricReader.Collect(ctx, &rm)
	return &rm, err
}

// Unregister shuts down the underlying OpenTelemetry TracerProvider.
func (te *openTelemetryTestExporter) Unregister(ctx context.Context) {
	te.tp.Shutdown(ctx)
	te.mp.Shutdown(ctx)
}

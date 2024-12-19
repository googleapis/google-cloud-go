//go:build go1.20
// +build go1.20

/*
Copyright 2024 Google LLC

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

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

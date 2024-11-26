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
	"errors"
	"fmt"
	"strings"
	"time"

	mexporter "github.com/GoogleCloudPlatform/opentelemetry-operations-go/exporter/metric"
	"github.com/google/uuid"
	"go.opentelemetry.io/contrib/detectors/gcp"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/metric/metricdata"
	"go.opentelemetry.io/otel/sdk/resource"
	"google.golang.org/api/option"
	"google.golang.org/api/transport"
	"google.golang.org/grpc"
	"google.golang.org/grpc/stats/opentelemetry"
)

const (
	monitoredResourceName = "storage.googleapis.com/Client"
	metricPrefix          = "storage.googleapis.com/client/"
)

func latencyHistogramBoundaries() []float64 {
	boundaries := []float64{}
	boundary := 0.0
	increment := 0.002
	// 2ms buckets for first 100ms, so we can have higher resolution for uploads and downloads in the 100 KiB range
	for i := 0; i < 50; i++ {
		boundaries = append(boundaries, boundary)
		// increment by 2ms
		boundary += increment
	}
	// For the remaining buckets do 10 10ms, 10 20ms, and so on, up until 5 minutes
	for i := 0; i < 150 && boundary < 300; i++ {
		boundaries = append(boundaries, boundary)
		if i != 0 && i%10 == 0 {
			increment *= 2
		}
		boundary += increment
	}
	return boundaries
}

func sizeHistogramBoundaries() []float64 {
	kb := 1024.0
	mb := 1024.0 * kb
	gb := 1024.0 * mb
	boundaries := []float64{}
	boundary := 0.0
	increment := 128 * kb
	// 128 KiB increments up to 4MiB, then exponential growth
	for len(boundaries) < 200 && boundary <= 16*gb {
		boundaries = append(boundaries, boundary)
		boundary += increment
		if boundary >= 4*mb {
			increment *= 2
		}
	}
	return boundaries
}

func metricFormatter(m metricdata.Metrics) string {
	return metricPrefix + strings.ReplaceAll(string(m.Name), ".", "/")
}

// Added to help with tests
type storageMonitoredResource struct {
	project  string
	resource *resource.Resource
}

func (smr *storageMonitoredResource) name() string {
	return monitoredResourceName
}

func (smr *storageMonitoredResource) attributes() []string {
	return []string{"project_id", "location", "cloud_platform", "host_id", "instance_id", "api"}
}

func (smr *storageMonitoredResource) detectFromGCP(ctx context.Context, opts ...resource.Option) error {
	aopts := append([]resource.Option{resource.WithDetectors(gcp.NewDetector())}, opts...)
	detectedAttrs, err := resource.New(ctx, aopts...)
	if err != nil {
		return err
	}
	s := detectedAttrs.Set()
	if p, present := s.Value("cloud.account.id"); present && smr.project == "" {
		smr.project = p.AsString()
	} else if !present && smr.project == "" {
		return errors.New("google cloud project is required to start client-side metrics")
	}
	mrAttrs := []attribute.KeyValue{
		{Key: "gcp.resource_type", Value: attribute.StringValue(monitoredResourceName)},
		{Key: "project_id", Value: attribute.StringValue(smr.project)},
		{Key: "api", Value: attribute.StringValue("grpc")},
		{Key: "instance_id", Value: attribute.StringValue(uuid.New().String())},
	}
	if v, ok := s.Value("location"); ok {
		mrAttrs = append(mrAttrs, attribute.KeyValue{Key: "location", Value: v})
	} else {
		mrAttrs = append(mrAttrs, attribute.KeyValue{Key: "location", Value: attribute.StringValue("global")})
	}
	if v, ok := s.Value("cloud.platform"); ok {
		mrAttrs = append(mrAttrs, attribute.KeyValue{Key: "cloud_platform", Value: v})
	} else {
		mrAttrs = append(mrAttrs, attribute.KeyValue{Key: "cloud_platform", Value: attribute.StringValue("unknown")})
	}
	if v, ok := s.Value("host.id"); ok {
		mrAttrs = append(mrAttrs, attribute.KeyValue{Key: "host_id", Value: v})
	} else {
		mrAttrs = append(mrAttrs, attribute.KeyValue{Key: "host_id", Value: attribute.StringValue("unknown")})
	}
	r, err := resource.New(ctx, resource.WithAttributes(mrAttrs...))
	if err != nil {
		return err
	}
	smr.resource = r
	return nil
}

type metricsContext struct {
	// client options passed to gRPC channels
	clientOpts []option.ClientOption
	// instance of metric reader used by gRPC client-side metrics
	provider *metric.MeterProvider
	// clean func to call when closing gRPC client
	close func()
}

func createHistogramView(name string, boundaries []float64) metric.View {
	return metric.NewView(metric.Instrument{
		Name: name,
		Kind: metric.InstrumentKindHistogram,
	}, metric.Stream{
		Name:        name,
		Aggregation: metric.AggregationExplicitBucketHistogram{Boundaries: boundaries},
	})
}

func newGRPCMetricContext(ctx context.Context, project string, config storageConfig) (*metricsContext, error) {
	var exporter metric.Exporter
	meterOpts := []metric.Option{}
	if config.metricExporter != nil {
		exporter = *config.metricExporter
	} else {
		smr := &storageMonitoredResource{
			project: project,
		}
		if err := smr.detectFromGCP(ctx); err != nil {
			return nil, err
		}
		meterOpts = append(meterOpts, metric.WithResource(smr.resource))
		meOpts := []mexporter.Option{
			mexporter.WithProjectID(smr.project),
			mexporter.WithMetricDescriptorTypeFormatter(metricFormatter),
			mexporter.WithCreateServiceTimeSeries(),
			mexporter.WithMonitoredResourceDescription(smr.name(), smr.attributes()),
		}
		ex, err := mexporter.New(meOpts...)
		if err != nil {
			return nil, err
		}
		exporter = ex
	}
	// Metric views update histogram boundaries to be relevant to GCS
	// otherwise default OTel histogram boundaries are used.
	metricViews := []metric.View{
		createHistogramView("grpc.client.attempt.duration", latencyHistogramBoundaries()),
		createHistogramView("grpc.client.attempt.rcvd_total_compressed_message_size", sizeHistogramBoundaries()),
		createHistogramView("grpc.client.attempt.sent_total_compressed_message_size", sizeHistogramBoundaries()),
	}
	interval := time.Minute
	if config.metricInterval > 0 {
		interval = config.metricInterval
	}
	meterOpts = append(meterOpts, metric.WithReader(metric.NewPeriodicReader(&exporterLogSuppressor{exporter: exporter}, metric.WithInterval(interval))),
		metric.WithView(metricViews...))
	if config.testReader != nil {
		meterOpts = append(meterOpts, metric.WithReader(config.testReader))
	}
	provider := metric.NewMeterProvider(meterOpts...)
	mo := opentelemetry.MetricsOptions{
		MeterProvider: provider,
		Metrics: opentelemetry.DefaultMetrics().Add(
			"grpc.lb.wrr.rr_fallback",
			"grpc.lb.wrr.endpoint_weight_not_yet_usable",
			"grpc.lb.wrr.endpoint_weight_stale",
			"grpc.lb.wrr.endpoint_weights",
			"grpc.lb.rls.cache_entries",
			"grpc.lb.rls.cache_size",
			"grpc.lb.rls.default_target_picks",
			"grpc.lb.rls.target_picks",
			"grpc.lb.rls.failed_picks"),
		OptionalLabels: []string{"grpc.lb.locality"},
	}
	opts := []option.ClientOption{
		option.WithGRPCDialOption(opentelemetry.DialOption(opentelemetry.Options{MetricsOptions: mo})),
		option.WithGRPCDialOption(grpc.WithDefaultCallOptions(grpc.StaticMethodCallOption{})),
	}
	context := &metricsContext{
		clientOpts: opts,
		provider:   provider,
		close:      createShutdown(ctx, provider),
	}
	return context, nil
}

func enableClientMetrics(ctx context.Context, s *settings, config storageConfig) (*metricsContext, error) {
	var project string
	c, err := transport.Creds(ctx, s.clientOption...)
	if err == nil {
		project = c.ProjectID
	}
	// Enable client-side metrics for gRPC
	metricsContext, err := newGRPCMetricContext(ctx, project, config)
	if err != nil {
		return nil, fmt.Errorf("gRPC Metrics: %w", err)
	}
	return metricsContext, nil
}

func createShutdown(ctx context.Context, provider *metric.MeterProvider) func() {
	return func() {
		provider.Shutdown(ctx)
	}
}

// Silences permission errors after initial error is emitted to prevent
// chatty logs.
type exporterLogSuppressor struct {
	exporter       metric.Exporter
	emittedFailure bool
}

// Implements OTel SDK metric.Exporter interface to prevent noisy logs from
// lack of credentials after initial failure.
// https://pkg.go.dev/go.opentelemetry.io/otel/sdk/metric@v1.28.0#Exporter
func (e *exporterLogSuppressor) Export(ctx context.Context, rm *metricdata.ResourceMetrics) error {
	if err := e.exporter.Export(ctx, rm); err != nil && !e.emittedFailure {
		if strings.Contains(err.Error(), "PermissionDenied") {
			e.emittedFailure = true
			return fmt.Errorf("gRPC metrics failed due permission issue: %w", err)
		}
		return err
	}
	return nil
}

func (e *exporterLogSuppressor) Temporality(k metric.InstrumentKind) metricdata.Temporality {
	return e.exporter.Temporality(k)
}

func (e *exporterLogSuppressor) Aggregation(k metric.InstrumentKind) metric.Aggregation {
	return e.exporter.Aggregation(k)
}

func (e *exporterLogSuppressor) ForceFlush(ctx context.Context) error {
	return e.exporter.ForceFlush(ctx)
}

func (e *exporterLogSuppressor) Shutdown(ctx context.Context) error {
	return e.exporter.Shutdown(ctx)
}

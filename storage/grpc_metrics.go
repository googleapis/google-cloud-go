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
	"fmt"
	"log"
	"strings"
	"time"

	mexporter "github.com/GoogleCloudPlatform/opentelemetry-operations-go/exporter/metric"
	"github.com/google/uuid"
	"go.opentelemetry.io/contrib/detectors/gcp"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/sdk/metric"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/metric/metricdata"
	"go.opentelemetry.io/otel/sdk/resource"
	"google.golang.org/api/option"
	"google.golang.org/api/transport"
	htransport "google.golang.org/api/transport/http"
	"google.golang.org/grpc/stats/opentelemetry"
)

const (
	monitored_resource_name = "storage.googleapis.com/Client"
)

func latencyHistogramBoundaries() []float64 {
	boundaries := []float64{}
	boundary := 0.0
	increment := 0.002
	// 2ms buckets for first 100ms, so we can have higher resolution for uploads and downloads in the 100 KiB range
	for i := 0; i < 50; i += 1 {
		boundaries = append(boundaries, boundary)
		// increment by 2ms
		boundary += increment
	}
	// For the remaining buckets do 10 10ms, 10 20ms, and so on, up until 5 minutes
	for i := 0; i < 150 && boundary < 300; i += 1 {
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
	return "storage.googleapis.com/client/" + strings.ReplaceAll(string(m.Name), ".", "/")
}

func gcpAttributeExpectedDefaults() []attribute.KeyValue {
	return []attribute.KeyValue{
		{Key: "location", Value: attribute.StringValue("global")},
		{Key: "cloud_platform", Value: attribute.StringValue("unknown")},
		{Key: "host_id", Value: attribute.StringValue("unknown")}}
}

// Added to help with tests
type internalPreparedResource struct {
	projectToUse string
	resource     *resource.Resource
}

func createPreparedResource(ctx context.Context, project string, resourceOptions []resource.Option) (*internalPreparedResource, error) {
	detectedAttrs, err := resource.New(ctx, resourceOptions...)
	if err != nil {
		return nil, err
	}
	preparedResource := &internalPreparedResource{}

	s := detectedAttrs.Set()
	p, present := s.Value("cloud.account.id")
	if present {
		preparedResource.projectToUse = p.AsString()
	} else {
		preparedResource.projectToUse = project
	}
	updates := []attribute.KeyValue{}
	for _, kv := range gcpAttributeExpectedDefaults() {
		if val, present := s.Value(kv.Key); !present || val.AsString() == "" {
			updates = append(updates, attribute.KeyValue{Key: kv.Key, Value: kv.Value})
		}
	}
	r, err := resource.New(
		ctx,
		resource.WithAttributes(
			attribute.KeyValue{Key: "gcp.resource_type", Value: attribute.StringValue(monitored_resource_name)},
			attribute.KeyValue{Key: "instance_id", Value: attribute.StringValue(uuid.New().String())},
			attribute.KeyValue{Key: "project_id", Value: attribute.StringValue(project)},
			attribute.KeyValue{Key: "api", Value: attribute.StringValue("grpc")},
		),
		resource.WithAttributes(detectedAttrs.Attributes()...),
		// Last duplicate key / value wins
		resource.WithAttributes(updates...),
	)
	if err != nil {
		return nil, err
	}
	preparedResource.resource = r
	return preparedResource, nil
}

type internalMetricsConfig struct {
	project  string
	endpoint string
}

type internalMetricsContext struct {
	// monitoring API endpoint used
	endpoint string
	// project used by exporter
	project string
	// client options passed to gRPC channels
	clientOpts []option.ClientOption
	// instance of metric reader used by gRPC client-side metrics
	provider *metric.MeterProvider
	// clean func to call when closing gRPC client
	close func()
}

func (mc *internalMetricsContext) String() string {
	return fmt.Sprintf("endpoint: %v\nproject: %v\n", mc.endpoint, mc.project)
}

func determineMonitoringEndpoint(endpoint string) string {
	// Check storage endpoint in case its using VPC then we use that endpoint instead.
	if strings.Contains(endpoint, "private.googleapis.com") || strings.Contains(endpoint, "restricted.googleapis.com") {
		return endpoint
	}
	// Default monitoring endpoint is used.
	return ""
}

func createHistogramView(name, desc, unit string, boundaries []float64) sdkmetric.View {
	return sdkmetric.NewView(sdkmetric.Instrument{
		Name:        name,
		Description: name,
		Kind:        sdkmetric.InstrumentKindHistogram,
		Unit:        unit,
	}, sdkmetric.Stream{
		Name:        name,
		Description: desc,
		Unit:        unit,
		Aggregation: sdkmetric.AggregationExplicitBucketHistogram{Boundaries: boundaries},
	})
}

func newGRPCMetricContext(ctx context.Context, config internalMetricsConfig) (*internalMetricsContext, error) {
	preparedResource, err := createPreparedResource(ctx, config.project, []resource.Option{resource.WithDetectors(gcp.NewDetector())})
	if err != nil {
		return nil, err
	}
	if config.project != preparedResource.projectToUse {
		log.Printf("The Project ID configured for metrics is %s, but the Project ID of the storage client is %s. Make sure that the service account in use has the required metric writing role (roles/monitoring.metricWriter) in the project projectIdToUse or metrics will not be written.", preparedResource.projectToUse, config.project)
	}
	meOpts := []mexporter.Option{
		mexporter.WithProjectID(preparedResource.projectToUse),
		mexporter.WithMetricDescriptorTypeFormatter(metricFormatter),
		mexporter.WithCreateServiceTimeSeries(),
		mexporter.WithMonitoredResourceDescription(monitored_resource_name, []string{"project_id", "location", "cloud_platform", "host_id", "instance_id", "api"})}
	endpointToUse := determineMonitoringEndpoint(config.endpoint)
	if endpointToUse != "" {
		meOpts = append(meOpts, mexporter.WithMonitoringClientOptions(option.WithEndpoint(endpointToUse)))
	}
	exporter, err := mexporter.New(meOpts...)
	if err != nil {
		return nil, err
	}
	metricViews := []sdkmetric.View{
		createHistogramView("grpc.client.attempt.duration", "A view of grpc.client.attempt.duration with histogram boundaries more appropriate for Google Cloud Storage RPCs", "s", latencyHistogramBoundaries()),
		createHistogramView("grpc.client.attempt.rcvd_total_compressed_message_size", "A view of grpc.client.attempt.rcvd_total_compressed_message_size with histogram boundaries more appropriate for Google Cloud Storage RPCs", "By", sizeHistogramBoundaries()),
		createHistogramView("grpc.client.attempt.sent_total_compressed_message_size", "A view of grpc.client.attempt.sent_total_compressed_message_size with histogram boundaries more appropriate for Google Cloud Storage RPCs", "By", sizeHistogramBoundaries()),
	}
	provider := sdkmetric.NewMeterProvider(
		sdkmetric.WithReader(sdkmetric.NewPeriodicReader(exporter, sdkmetric.WithInterval(time.Minute))),
		sdkmetric.WithResource(preparedResource.resource),
		sdkmetric.WithView(metricViews...),
	)
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
	do := option.WithGRPCDialOption(opentelemetry.DialOption(opentelemetry.Options{MetricsOptions: mo}))
	context := &internalMetricsContext{
		project:    preparedResource.projectToUse,
		endpoint:   endpointToUse,
		clientOpts: []option.ClientOption{do},
		provider:   provider,
		close:      createShutdown(ctx, provider),
	}
	return context, nil
}

func enableClientMetrics(ctx context.Context, s *settings) (*internalMetricsContext, error) {
	_, ep, err := htransport.NewClient(ctx, s.clientOption...)
	if err != nil {
		return nil, fmt.Errorf("gRPC Metrics: %w", err)
	}
	project := ""
	c, err := transport.Creds(ctx, s.clientOption...)
	if err == nil {
		project = c.ProjectID
	}
	// Enable client-side metrics for gRPC
	metricsContext, err := newGRPCMetricContext(ctx, internalMetricsConfig{
		project:  project,
		endpoint: ep,
	})
	if err != nil {
		return nil, fmt.Errorf("gRPC Metrics: %w", err)
	}
	return metricsContext, nil
}

func createShutdown(ctx context.Context, provider *sdkmetric.MeterProvider) func() {
	return func() {
		provider.Shutdown(ctx)
	}
}

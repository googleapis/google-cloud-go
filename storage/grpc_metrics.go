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
	estats "google.golang.org/grpc/experimental/stats"
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

	for len(boundaries) < 200 && boundary <= 16*gb {
		boundaries = append(boundaries, boundary)
		boundary += increment
		if boundary >= 4*mb {
			increment *= 2
		}
	}
	return boundaries
}

func getViewMasks(defaultMetrics map[estats.Metric]bool, additionalMetrics []estats.Metric) []sdkmetric.View {
	views := []sdkmetric.View{}
	for k, include := range defaultMetrics {
		if !include {
			continue
		}
		views = append(views, sdkmetric.NewView(sdkmetric.Instrument{
			Name: string(k),
		}, sdkmetric.Stream{Name: strings.ReplaceAll(string(k), ".", "/")}))
	}
	for _, m := range additionalMetrics {
		views = append(views, sdkmetric.NewView(sdkmetric.Instrument{
			Name: string(m),
		}, sdkmetric.Stream{Name: strings.ReplaceAll(string(m), ".", "/")}))
	}
	return views
}

func metricFormatter(m metricdata.Metrics) string {
	return "storage.googleapis.com/client/" + m.Name
}

func gcpAttributeExpectedDefaults() []attribute.KeyValue {
	return []attribute.KeyValue{
		{Key: "location", Value: attribute.StringValue("global")},
		{Key: "cloud_platform", Value: attribute.StringValue("unknown")},
		{Key: "host_id", Value: attribute.StringValue("unknown")}}
}

// Added to help with tests
type preparedResource struct {
	projectToUse string
	resource     *resource.Resource
}

func createPreparedResource(ctx context.Context, project string, resourceOptions []resource.Option) (*preparedResource, error) {
	detectedAttrs, err := resource.New(ctx, resourceOptions...)
	if err != nil {
		return nil, err
	}
	preparedResource := &preparedResource{}

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
	project string
	host    string
}

type internalMetricsContext struct {
	// client options passed to gRPC channels
	clientOpts []option.ClientOption
	// instance of metric reader used by gRPC client-side metrics
	provider *metric.MeterProvider
	// clean func to call when closing gRPC client
	close func()
}

// TODO: format errors emitted here.
func gRPCMetricProvider(ctx context.Context, config internalMetricsConfig) (*internalMetricsContext, error) {
	preparedResource, err := createPreparedResource(ctx, config.project, []resource.Option{resource.WithDetectors(gcp.NewDetector())})
	if err != nil {
		return nil, err
	}
	if config.project != preparedResource.projectToUse {
		log.Printf("The Project ID configured for metrics is %s, but the Project ID of the storage client is %s. Make sure that the service account in use has the required metric writing role (roles/monitoring.metricWriter) in the project projectIdToUse or metrics will not be written.", preparedResource.projectToUse, config.project)
	}
	metricsToEnable := []estats.Metric{
		"grpc.lb.wrr.rr_fallback",
		"grpc.lb.wrr.endpoint_weight_not_yet_usable",
		"grpc.lb.wrr.endpoint_weight_stale",
		"grpc.lb.wrr.endpoint_weights",
		"grpc.lb.rls.cache_entries",
		"grpc.lb.rls.cache_size",
		"grpc.lb.rls.default_target_picks",
		"grpc.lb.rls.target_picks",
		"grpc.lb.rls.failed_picks",
	}
	defaultMetrics := opentelemetry.DefaultMetrics()
	metricViews := getViewMasks(defaultMetrics.Metrics(), metricsToEnable)
	metricViews = append(metricViews, []sdkmetric.View{
		sdkmetric.NewView(sdkmetric.Instrument{
			Name: "grpc/client/attempt/duration",
			Kind: sdkmetric.InstrumentKindHistogram,
		}, sdkmetric.Stream{
			Name:        "grpc/client/attempt/duration",
			Description: "A view of grpc/client/attempt/duration with histogram boundaries more appropriate for Google Cloud Storage RPCs",
			Unit:        "s",
			Aggregation: sdkmetric.AggregationExplicitBucketHistogram{Boundaries: latencyHistogramBoundaries()},
		}),
		sdkmetric.NewView(sdkmetric.Instrument{
			Name: "grpc/client/attempt/rcvd_total_compressed_message_size",
			Kind: sdkmetric.InstrumentKindHistogram,
		}, sdkmetric.Stream{
			Name:        "grpc/client/attempt/rcvd_total_compressed_message_size",
			Description: "A view of grpc/client/attempt/rcvd_total_compressed_message_size with histogram boundaries more appropriate for Google Cloud Storage RPCs",
			Unit:        "By",
			Aggregation: sdkmetric.AggregationExplicitBucketHistogram{Boundaries: sizeHistogramBoundaries()},
		}),
		sdkmetric.NewView(sdkmetric.Instrument{
			Name: "grpc/client/attempt/rcvd_total_compressed_message_size",
			Kind: sdkmetric.InstrumentKindHistogram,
		}, sdkmetric.Stream{
			Name:        "grpc/client/attempt/sent_total_compressed_message_size",
			Description: "A view of grpc/client/attempt/sent_total_compressed_message_size with histogram boundaries more appropriate for Google Cloud Storage RPCs",
			Unit:        "By",
			Aggregation: sdkmetric.AggregationExplicitBucketHistogram{Boundaries: sizeHistogramBoundaries()},
		}),
	}...)
	exporter, err := mexporter.New(
		// mexporter.WithMonitoringClientOptions(option.WithEndpoint(config.host)),
		mexporter.WithProjectID(preparedResource.projectToUse),
		mexporter.WithMetricDescriptorTypeFormatter(metricFormatter),
		mexporter.WithCreateServiceTimeSeries(),
		mexporter.WithMonitoredResourceDescription(monitored_resource_name, []string{"project_id", "location", "cloud_platform", "host_id", "instance_id", "api"}))
	if err != nil {
		return nil, err
	}
	provider := sdkmetric.NewMeterProvider(
		sdkmetric.WithReader(sdkmetric.NewPeriodicReader(exporter, sdkmetric.WithInterval(time.Second*60))),
		sdkmetric.WithResource(preparedResource.resource),
		sdkmetric.WithView(metricViews...),
	)
	mo := opentelemetry.MetricsOptions{
		MeterProvider:  provider,
		Metrics:        defaultMetrics.Add(metricsToEnable...),
		OptionalLabels: []string{"grpc.lb.locality"},
	}
	do := option.WithGRPCDialOption(opentelemetry.DialOption(opentelemetry.Options{MetricsOptions: mo}))
	context := &internalMetricsContext{[]option.ClientOption{do}, provider, createShutdown(ctx, provider)}
	return context, nil
}

func createShutdown(ctx context.Context, provider *sdkmetric.MeterProvider) func() {
	return func() {
		provider.Shutdown(ctx)
	}
}

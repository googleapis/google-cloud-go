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
	"strings"

	mexporter "github.com/GoogleCloudPlatform/opentelemetry-operations-go/exporter/metric"
	"github.com/google/uuid"
	"go.opentelemetry.io/contrib/detectors/gcp"
	"go.opentelemetry.io/otel/attribute"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/metric/metricdata"
	"go.opentelemetry.io/otel/sdk/resource"
	"google.golang.org/api/option"
	estats "google.golang.org/grpc/experimental/stats"
	"google.golang.org/grpc/stats/opentelemetry"
)

const (
	monitored_resource_name         = "storage.googleapis.com/Client"
	mr_location_label_default       = "global"
	mr_cloud_platform_label_default = "unknown"
	mr_host_id_label_default        = "unknown"
	metric_format_prefix            = "storage.googleapis.com/client/"
	metric_lb_locality_label        = "grpc.lb.locality"
)

func getMetricsToEnable() []estats.Metric {
	return []estats.Metric{"grpc.lb.wrr.rr_fallback",
		"grpc.lb.wrr.endpoint_weight_not_yet_usable",
		"grpc.lb.wrr.endpoint_weight_stale",
		"grpc.lb.wrr.endpoint_weights",
		"grpc.lb.rls.cache_entries",
		"grpc.lb.rls.cache_size",
		"grpc.lb.rls.default_target_picks",
		"grpc.lb.rls.target_picks",
		"grpc.lb.rls.failed_picks",
		"grpc.xds_client.connected",
		"grpc.xds_client.server_failure",
		"grpc.xds_client.resource_updates_valid",
		"grpc.xds_client.resource_updates_invalid",
		"grpc.xds_client.resources",
	}
}

func getMetricsEnabledByDefault() []estats.Metric {
	return []estats.Metric{
		"grpc.client.attempt.sent_total_compressed_message_size",
		"grpc.client.attempt.rcvd_total_compressed_message_size",
		"grpc.client.attempt.started",
		"grpc.client.attempt.duration",
		"grpc.client.call.duration",
	}
}

func getViewMasks(metrics []estats.Metric) []sdkmetric.View {
	views := []sdkmetric.View{}
	for _, m := range metrics {
		views = append(views, sdkmetric.NewView(sdkmetric.Instrument{
			Name: string(m),
		}, sdkmetric.Stream{Name: strings.ReplaceAll(string(m), ".", "/")}))
	}
	return views
}

func metricFormatter(m metricdata.Metrics) string {
	return metric_format_prefix + m.Name
}

func gcpAttributeExpectedDefaults() []attribute.KeyValue {
	return []attribute.KeyValue{
		{Key: "location", Value: attribute.StringValue(mr_location_label_default)},
		{Key: "cloud_platform", Value: attribute.StringValue(mr_cloud_platform_label_default)},
		{Key: "host_id", Value: attribute.StringValue(mr_host_id_label_default)}}
}

func getPreparedResourceUsingGCPDetector(ctx context.Context, project string) (*resource.Resource, error) {
	gcpDetector := []resource.Option{resource.WithDetectors(gcp.NewDetector())}
	return getPreparedResource(ctx, project, gcpDetector)
}

// Added to help with tests
func getPreparedResource(ctx context.Context, project string, resourceOptions []resource.Option) (*resource.Resource, error) {
	detectedAttrs, err := resource.New(ctx, resourceOptions...)
	if err != nil {
		return nil, err
	}
	s := detectedAttrs.Set()
	updates := []attribute.KeyValue{}
	for _, kv := range gcpAttributeExpectedDefaults() {
		if val, present := s.Value(kv.Key); !present || val.AsString() == "" {
			updates = append(updates, attribute.KeyValue{Key: kv.Key, Value: kv.Value})
		}
	}
	return resource.New(
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
}

// TODO: format errors emitted here.
func gRPCMetricProvider(ctx context.Context, project string) (*sdkmetric.MeterProvider, error) {
	exporter, err := mexporter.New(
		mexporter.WithProjectID(project),
		mexporter.WithMetricDescriptorTypeFormatter(metricFormatter),
		mexporter.WithCreateServiceTimeSeries(),
		mexporter.WithMonitoredResourceDescription(monitored_resource_name, []string{"project_id", "location", "cloud_platform", "host_id", "instance_id", "api"}))
	if err != nil {
		return nil, err
	}
	preparedResource, err := getPreparedResourceUsingGCPDetector(ctx, project)
	if err != nil {
		return nil, err
	}
	allMetrics := append(getMetricsEnabledByDefault(), getMetricsToEnable()...)
	metricViews := getViewMasks(allMetrics)
	provider := sdkmetric.NewMeterProvider(
		sdkmetric.WithReader(sdkmetric.NewPeriodicReader(exporter)),
		sdkmetric.WithResource(preparedResource),
		sdkmetric.WithView(metricViews...),
	)
	return provider, nil
}

func togRPCDialOption(provider *sdkmetric.MeterProvider) option.ClientOption {
	mo := opentelemetry.MetricsOptions{
		MeterProvider:  provider,
		Metrics:        opentelemetry.DefaultMetrics().Add(getMetricsToEnable()...),
		OptionalLabels: []string{metric_lb_locality_label},
	}
	do := option.WithGRPCDialOption(opentelemetry.DialOption(opentelemetry.Options{MetricsOptions: mo}))
	return do
}

func metricCleanup(ctx context.Context, provider *sdkmetric.MeterProvider) func() {
	return func() {
		provider.Shutdown(ctx)
	}
}

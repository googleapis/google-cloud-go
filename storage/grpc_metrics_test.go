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
	"testing"

	mexporter "github.com/GoogleCloudPlatform/opentelemetry-operations-go/exporter/metric"

	"go.opentelemetry.io/contrib/detectors/gcp"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/sdk/metric"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/metric/metricdata"
	"go.opentelemetry.io/otel/sdk/resource"
	"google.golang.org/api/iterator"
	"google.golang.org/api/option"
	"google.golang.org/grpc/stats/opentelemetry"
)

func getMetricsToEnable() []string {
	return []string{"grpc.lb.wrr.rr_fallback",
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

func getMetricsEnabledByDefault() []string {
	return []string{
		"grpc.client.attempt.sent_total_compressed_message_size",
		"grpc.client.attempt.rcvd_total_compressed_message_size",
		"grpc.client.attempt.started",
		"grpc.client.attempt.duration",
		"grpc.client.call.duration",
	}
}

func getCorrectMetricName() []metric.View {
	views := []metric.View{}
	for _, m := range getMetricsEnabledByDefault() {
		// element is the element from someSlice for where we are
		views = append(views, metric.NewView(metric.Instrument{
			Name: m,
		}, metric.Stream{Name: strings.ReplaceAll(m, ".", "/")}))
	}
	for _, m := range getMetricsToEnable() {
		// element is the element from someSlice for where we are
		views = append(views, metric.NewView(metric.Instrument{
			Name: m,
		}, metric.Stream{Name: strings.ReplaceAll(m, ".", "/")}))
	}
	return views
}

func TestMetrics(t *testing.T) {
	// extend timeout
	ctx := context.Background()
	// reader := metric.NewManualReader()
	// // provider := metric.NewMeterProvider(
	// // 	metric.WithReader(metric.NewPeriodicReader(exp)))
	// provider := metric.NewMeterProvider(metric.WithReader(reader))
	// Test if using local impl.
	// opentelemetry.Frank()
	exporter, err := mexporter.New(mexporter.WithProjectID("spec-test-ruby-samples"), mexporter.WithMetricDescriptorTypeFormatter(func(m metricdata.Metrics) string {
		return "storage.googleapis.com/client/" + m.Name
	}), mexporter.WithCreateServiceTimeSeries(), mexporter.WithMonitoredResourceDescription("storage.googleapis.com/Client", []string{"project_id", "location", "cloud_platform", "host_id", "instance_id", "api"}))
	if err != nil {
		log.Fatalf("Failed to create exporter: %v", err)
	}

	res, err := resource.New(
		ctx,
		// Use the GCP resource detector to detect information about the GCP platform
		resource.WithDetectors(gcp.NewDetector()),
		// Keep the default detectors
		resource.WithTelemetrySDK(),
		// Add attributes from environment variables
		resource.WithFromEnv(),
		// Add your own custom attributes to identify your application
		resource.WithAttributes(
			attribute.KeyValue{Key: "gcp.resource_type", Value: attribute.StringValue("storage.googleapis.com/Client")},
			attribute.KeyValue{Key: "location", Value: attribute.StringValue("us-central1")},
			attribute.KeyValue{Key: "cloud_platform", Value: attribute.StringValue("platform")},
			attribute.KeyValue{Key: "host_id", Value: attribute.StringValue("host")},
			attribute.KeyValue{Key: "project_id", Value: attribute.StringValue("spec-test-ruby-samples")},
			attribute.KeyValue{Key: "api", Value: attribute.StringValue("grpc")},
			attribute.KeyValue{Key: "instance_id", Value: attribute.StringValue(("UUID"))},
		),
	)
	if err != nil {
		log.Fatalf("Failed to create resource: %v", err)
	}
	views := getCorrectMetricName()
	// Construct the exporter using the above config
	provider := sdkmetric.NewMeterProvider(
		sdkmetric.WithReader(sdkmetric.NewPeriodicReader(exporter)),
		sdkmetric.WithResource(res),
		sdkmetric.WithView(views...),
	)

	mo := opentelemetry.MetricsOptions{
		MeterProvider:  provider,
		Metrics:        opentelemetry.DefaultMetrics().Add("grpc.lb.wrr.rr_fallback", "grpc.lb.wrr.endpoint_weight_not_yet_usable", "grpc.lb.wrr.endpoint_weight_stale", "grpc.lb.wrr.endpoint_weights"),
		OptionalLabels: []string{"grpc.lb.locality"},
	}
	do := opentelemetry.DialOption(opentelemetry.Options{MetricsOptions: mo})

	grpcClient, err := NewGRPCClient(ctx, option.WithGRPCDialOption(do))
	if err != nil {
		log.Fatalf("Error setting up gRPC client for emulator tests: %v", err)
	}
	it := grpcClient.Buckets(ctx, "spec-test-ruby-samples")
	for {
		for {
			_, err := it.Next()
			if err == iterator.Done {
				break
			}
			if err != nil {
				log.Fatalf("Failed: %v", err)
			}
			// log.Printf("Buckets: %v\n", battrs.Name)
			// rm := &metricdata.ResourceMetrics{}
			// reader.Collect(ctx, rm)
			// log.Printf("metric: %v\n", rm)

		}
	}
}

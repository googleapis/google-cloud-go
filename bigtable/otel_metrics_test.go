/*
Copyright 2025 Google LLC

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

package bigtable

import (
	"context"
	"fmt"
	"testing"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/metric/metricdata"
	"go.opentelemetry.io/otel/sdk/resource"
)

func TestMetricFormatter(t *testing.T) {
	want := "bigtable.googleapis.com/internal/client/metric/name"
	s := metricdata.Metrics{Name: "metric.name"}
	got := metricFormatter(s)
	if want != got {
		t.Errorf("got: %v, want %v", got, want)
	}
}

func TestNewExporterLogSuppressor(t *testing.T) {
	ctx := context.Background()
	s := &exporterLogSuppressor{Exporter: &failingExporter{}}
	if err := s.Export(ctx, nil); err == nil {
		t.Errorf("exporterLogSuppressor: did not emit an error when one was expected")
	}
	if err := s.Export(ctx, nil); err != nil {
		t.Errorf("exporterLogSuppressor: emitted an error when it should have suppressed")
	}
}

type failingExporter struct {
	metric.Exporter
}

func (f *failingExporter) Export(ctx context.Context, rm *metricdata.ResourceMetrics) error {
	return fmt.Errorf("PermissionDenied")
}

func TestOtelMetricsContext(t *testing.T) {
	ctx := context.Background()
	mr := metric.NewManualReader()
	attrs := []attribute.KeyValue{
		{Key: "cloud.account.id",
			Value: attribute.StringValue("client-project-id")},
		{Key: "cloud.region",
			Value: attribute.StringValue("us-central1")},
		{Key: "cloud.platform",
			Value: attribute.StringValue("gcp")},
		{Key: "host.id",
			Value: attribute.StringValue("gce-instance-id")},
		{Key: "host.name",
			Value: attribute.StringValue("gce-instance-name")},
	}
	cfg := metricsConfig{
		project:         "project-id",
		instance:        "instance-id",
		appProfile:      "app-profile",
		clientName:      "client-name",
		clientUID:       "client-uid",
		manualReader:    mr,
		disableExporter: true, // disable since this is a unit test
		resourceOpts:    []resource.Option{resource.WithAttributes(attrs...)},
	}
	mc, err := newOtelMetricsContext(ctx, cfg)
	if err != nil {
		t.Errorf("newGRPCMetricContext: %v", err)
	}
	defer mc.close()
	rm := metricdata.ResourceMetrics{}
	if err := mr.Collect(ctx, &rm); err != nil {
		t.Errorf("ManualReader.Collect: %v", err)
	}
	monitoredResourceWant := map[string]string{
		"gcp.resource_type": bigtableClientMonitoredResourceName,
		"app_profile":       "app-profile",
		"client_name":       "client-name",
		"uuid":              "client-uid",
		"client_project":    "client-project-id",
		"cloud_platform":    "gcp",
		"host_id":           "gce-instance-id",
		"host_name":         "gce-instance-name",
		"location":          "us-central1",
		"project_id":        "project-id",
		"instance":          "instance-id",
		"region":            "us-central1",
	}
	for _, attr := range rm.Resource.Attributes() {
		attrKey := string(attr.Key)
		want := monitoredResourceWant[attrKey]
		got := attr.Value.AsString()
		if want != got {
			t.Errorf("attr.key: %v, got: %v want: %v", attrKey, got, want)
		}
	}
}

func TestOtelMetricsSchema(t *testing.T) {
	tests := []struct {
		name         string
		inputAttrs   []attribute.KeyValue
		wantRegion   string
		wantHostName string
	}{
		{
			name: "zone",
			inputAttrs: []attribute.KeyValue{
				{Key: "cloud.availability_zone", Value: attribute.StringValue("us-central1-a")},
				{Key: "cloud.platform", Value: attribute.StringValue("gcp_compute_engine")},
				{Key: "host.name", Value: attribute.StringValue("explicit-host")},
			},
			wantRegion:   "us-central1",
			wantHostName: "explicit-host",
		},
		{
			name: "Pod Name",
			inputAttrs: []attribute.KeyValue{
				{Key: "k8s.pod.name", Value: attribute.StringValue("pod-123")},
				{Key: "cloud.region", Value: attribute.StringValue("us-west1")},
			},
			wantRegion:   "us-west1",
			wantHostName: "pod-123",
		},
		{
			name: "K8s Node Name",
			inputAttrs: []attribute.KeyValue{
				{Key: "k8s.node.name", Value: attribute.StringValue("node-abc")},
				{Key: "cloud.region", Value: attribute.StringValue("us-west1")},
			},
			wantRegion:   "us-west1",
			wantHostName: "node-abc",
		},
		{
			name: "aws zone",
			inputAttrs: []attribute.KeyValue{
				{Key: "k8s.node.name", Value: attribute.StringValue("node-abc")},
				{Key: "cloud.availability_zone", Value: attribute.StringValue("us-west-1a")},
			},
			wantRegion:   "us-west",
			wantHostName: "node-abc",
		},
		{
			name: "Global Default",
			inputAttrs: []attribute.KeyValue{
				{Key: "cloud.platform", Value: attribute.StringValue("unknown")},
			},
			wantRegion:   "global",
			wantHostName: "unknown",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			ctx := context.Background()
			mr := metric.NewManualReader()

			// Basic required attributes for the test setup
			baseAttrs := []attribute.KeyValue{
				{Key: "cloud.account.id", Value: attribute.StringValue("test-account")},
				{Key: "host.id", Value: attribute.StringValue("test-host-id")},
			}
			allAttrs := append(baseAttrs, tc.inputAttrs...)

			cfg := metricsConfig{
				project:         "test-project",
				instance:        "test-instance",
				appProfile:      "default",
				clientName:      "test-client",
				clientUID:       "test-uid",
				manualReader:    mr,
				disableExporter: true,
				resourceOpts:    []resource.Option{resource.WithAttributes(allAttrs...)},
			}

			mc, err := newOtelMetricsContext(ctx, cfg)
			if err != nil {
				t.Fatalf("newOtelMetricsContext failed: %v", err)
			}
			defer mc.close()

			rm := metricdata.ResourceMetrics{}
			if err := mr.Collect(ctx, &rm); err != nil {
				t.Fatalf("Collect failed: %v", err)
			}

			// Extract attributes into a map for easy lookup
			gotAttrs := make(map[string]string)
			for _, attr := range rm.Resource.Attributes() {
				gotAttrs[string(attr.Key)] = attr.Value.AsString()
			}

			if gotAttrs["region"] != tc.wantRegion {
				t.Errorf("region: got %q, want %q", gotAttrs["region"], tc.wantRegion)
			}
			if gotAttrs["host_name"] != tc.wantHostName {
				t.Errorf("host_name: got %q, want %q", gotAttrs["host_name"], tc.wantHostName)
			}
		})
	}
}

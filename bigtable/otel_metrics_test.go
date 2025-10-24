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

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
	"testing"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/metric/metricdata"
	"go.opentelemetry.io/otel/sdk/resource"
)

func TestMetricFormatter(t *testing.T) {
	want := "storage.googleapis.com/client/metric/name"
	s := metricdata.Metrics{Name: "metric.name"}
	got := metricFormatter(s)
	if want != got {
		t.Errorf("got: %v, want %v", got, want)
	}
}

func TestStorageMonitoredResource(t *testing.T) {
	ctx := context.Background()
	for _, test := range []struct {
		desc               string
		project            string
		api                string
		detectedAttributes []attribute.KeyValue
		wantAttributes     attribute.Set
	}{
		{
			desc:    "default values set when GCP attributes are not detected",
			project: "project-id",
			api:     "grpc",
			wantAttributes: attribute.NewSet(attribute.KeyValue{
				Key:   "location",
				Value: attribute.StringValue("global"),
			}, attribute.KeyValue{
				Key:   "cloud_platform",
				Value: attribute.StringValue("unknown"),
			}, attribute.KeyValue{
				Key:   "host_id",
				Value: attribute.StringValue("unknown"),
			}, attribute.KeyValue{
				Key:   "project_id",
				Value: attribute.StringValue("project-id"),
			}, attribute.KeyValue{
				Key:   "api",
				Value: attribute.StringValue("grpc"),
			}),
		},
		{
			desc:    "use detected values when GCE attributes are detected",
			project: "project-id",
			api:     "grpc",
			detectedAttributes: []attribute.KeyValue{
				{Key: "cloud.region",
					Value: attribute.StringValue("us-central1")},
				{Key: "cloud.platform",
					Value: attribute.StringValue("gce")},
				{Key: "host.id",
					Value: attribute.StringValue("gce-instance-id")},
			},
			wantAttributes: attribute.NewSet(attribute.KeyValue{
				Key:   "location",
				Value: attribute.StringValue("us-central1"),
			}, attribute.KeyValue{
				Key:   "cloud_platform",
				Value: attribute.StringValue("gce"),
			}, attribute.KeyValue{
				Key:   "host_id",
				Value: attribute.StringValue("gce-instance-id"),
			}, attribute.KeyValue{
				Key:   "project_id",
				Value: attribute.StringValue("project-id"),
			}, attribute.KeyValue{
				Key:   "api",
				Value: attribute.StringValue("grpc"),
			}),
		},
		{
			desc:    "use detected values when FAAS attributes are detected",
			project: "project-id",
			api:     "grpc",
			detectedAttributes: []attribute.KeyValue{
				{Key: "cloud.region",
					Value: attribute.StringValue("us-central1")},
				{Key: "cloud.platform",
					Value: attribute.StringValue("cloud-run")},
				{Key: "faas.id",
					Value: attribute.StringValue("run-instance-id")},
			},
			wantAttributes: attribute.NewSet(attribute.KeyValue{
				Key:   "location",
				Value: attribute.StringValue("us-central1"),
			}, attribute.KeyValue{
				Key:   "cloud_platform",
				Value: attribute.StringValue("cloud-run"),
			}, attribute.KeyValue{
				Key:   "host_id",
				Value: attribute.StringValue("run-instance-id"),
			}, attribute.KeyValue{
				Key:   "project_id",
				Value: attribute.StringValue("project-id"),
			}, attribute.KeyValue{
				Key:   "api",
				Value: attribute.StringValue("grpc"),
			}),
		},
	} {
		t.Run(test.desc, func(t *testing.T) {
			smr, err := newStorageMonitoredResource(ctx, test.project, test.api, resource.WithAttributes(test.detectedAttributes...))
			if err != nil {
				t.Errorf("newStorageMonitoredResource: %v", err)
			}
			resultSet := smr.resource.Set()
			for _, want := range test.wantAttributes.ToSlice() {
				got, exists := resultSet.Value(want.Key)
				if !exists {
					t.Errorf("resultSet[%v] not set", want.Key)
					continue
				}
				if got != want.Value {
					t.Errorf("want[%v] = %v, got: %v", want.Key, want.Value.AsString(), got.AsString())
					continue
				}
			}
		})
	}
}

func TestNewGRPCMetricContext(t *testing.T) {
	ctx := context.Background()
	mr := metric.NewManualReader()
	attrs := []attribute.KeyValue{
		{Key: "cloud.region",
			Value: attribute.StringValue("us-central1")},
		{Key: "cloud.platform",
			Value: attribute.StringValue("gcp")},
		{Key: "host.id",
			Value: attribute.StringValue("gce-instance-id")},
	}
	cfg := metricsConfig{
		project:      "project-id",
		manualReader: mr,
		resourceOpts: []resource.Option{resource.WithAttributes(attrs...)},
	}
	mc, err := newGRPCMetricContext(ctx, cfg)
	if err != nil {
		t.Errorf("newGRPCMetricContext: %v", err)
	}
	defer mc.close()
	rm := metricdata.ResourceMetrics{}
	if err := mr.Collect(ctx, &rm); err != nil {
		t.Errorf("ManualReader.Collect: %v", err)
	}
	monitoredResourceWant := map[string]string{
		"gcp.resource_type": "storage.googleapis.com/Client",
		"api":               "grpc",
		"cloud_platform":    "gcp",
		"host_id":           "gce-instance-id",
		"location":          "us-central1",
		"project_id":        "project-id",
		"instance_id":       "ignore",
	}
	for _, attr := range rm.Resource.Attributes() {
		want := monitoredResourceWant[string(attr.Key)]
		if want == "ignore" {
			continue
		}
		got := attr.Value.AsString()
		if want != got {
			t.Errorf("got: %v want: %v", got, want)
		}
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

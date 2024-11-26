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
		detectedAttributes []attribute.KeyValue
		wantAttributes     attribute.Set
	}{
		{
			desc: "default values set when GCP attributes are not detected",
			wantAttributes: attribute.NewSet(attribute.KeyValue{
				Key:   "location",
				Value: attribute.StringValue("global"),
			}, attribute.KeyValue{
				Key:   "cloud_platform",
				Value: attribute.StringValue("unknown"),
			}, attribute.KeyValue{
				Key:   "host_id",
				Value: attribute.StringValue("unknown"),
			}),
		},
		{
			desc: "use detected values when GCP attributes are detected",
			detectedAttributes: []attribute.KeyValue{
				{Key: "location",
					Value: attribute.StringValue("us-central1")},
				{Key: "cloud.platform",
					Value: attribute.StringValue("gcp")},
				{Key: "host.id",
					Value: attribute.StringValue("gce-instance-id")},
			},
			wantAttributes: attribute.NewSet(attribute.KeyValue{
				Key:   "location",
				Value: attribute.StringValue("us-central1"),
			}, attribute.KeyValue{
				Key:   "cloud_platform",
				Value: attribute.StringValue("gcp"),
			}, attribute.KeyValue{
				Key:   "host_id",
				Value: attribute.StringValue("gce-instance-id"),
			}),
		},
	} {
		t.Run(test.desc, func(t *testing.T) {
			smr := &storageMonitoredResource{
				project: "project",
			}
			if err := smr.detectFromGCP(ctx, resource.WithAttributes(test.detectedAttributes...)); err != nil {
				t.Errorf("detectFromGCP: %v", err)
			}
			resultSet := smr.resource.Set()
			for _, want := range test.wantAttributes.ToSlice() {
				got, exists := resultSet.Value(want.Key)
				if !exists {
					t.Errorf("detectFromGCP: %v not set", want.Key)
					continue
				}
				if got != want.Value {
					t.Errorf("detectFromGCP: want[%v] = %v, got: %v", want.Key, want.Value, got)
					continue
				}
			}
		})
	}
}

func TestNewExporterLogSuppressor(t *testing.T) {
	ctx := context.Background()
	s := &exporterLogSuppressor{exporter: &failingExporter{}}
	if err := s.Export(ctx, nil); err == nil {
		t.Errorf("exporterLogSuppressor: did not emit an error when one was expected")
	}
	if err := s.Export(ctx, nil); err != nil {
		t.Errorf("exporterLogSuppressor: emitted an error when it should have suppressed")
	}
}

type failingExporter struct{}

func (f *failingExporter) Export(ctx context.Context, rm *metricdata.ResourceMetrics) error {
	return fmt.Errorf("PermissionDenied")
}

func (f *failingExporter) Temporality(m metric.InstrumentKind) metricdata.Temporality {
	return metricdata.CumulativeTemporality
}

func (f *failingExporter) Aggregation(ik metric.InstrumentKind) metric.Aggregation {
	return metric.AggregationDefault{}
}

func (f *failingExporter) ForceFlush(ctx context.Context) error {
	return nil
}

func (f *failingExporter) Shutdown(ctx context.Context) error {
	return nil
}

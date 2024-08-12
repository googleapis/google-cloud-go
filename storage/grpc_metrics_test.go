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
	"testing"

	"go.opentelemetry.io/otel/attribute"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/metric/metricdata"
	"go.opentelemetry.io/otel/sdk/resource"
	"google.golang.org/api/iterator"
	estats "google.golang.org/grpc/experimental/stats"
)

func TestMetrics(t *testing.T) {
	ctx := context.Background()
	grpcClient, err := NewGRPCClient(ctx)
	if err != nil {
		log.Fatalf("Error setting up gRPC client: %v", err)
	}
	defer grpcClient.Close()
	bucket := grpcClient.Bucket("-frank")
	it := bucket.Objects(ctx, nil)
	for {
		_, err := it.Next()
		if err == iterator.Done {
			break
		}
		if err != nil {
			log.Fatalf("Failed: %v", err)
		}
	}
}

func TestGetViewMasks(t *testing.T) {
	testMetricName := estats.Metric("test.metric.name")
	views := getViewMasks([]estats.Metric{testMetricName})
	for _, mask := range views {
		got, ok := mask(sdkmetric.Instrument{Name: string(testMetricName)})
		if !ok {
			t.Errorf("getViewMasks: did not find %v", testMetricName)
		}
		want := "test/metric/name"
		if got.Name != want {
			t.Errorf("got: %v, want: %v\n", got, want)
		}
	}
}

func TestMetricFormatter(t *testing.T) {
	want := "storage.googleapis.com/client/t"
	s := metricdata.Metrics{Name: "t", Description: "", Unit: "", Data: nil}
	got := metricFormatter(s)
	if want != got {
		t.Errorf("got: %v, want %v", got, want)
	}
}

func TestGetPreparedResource(t *testing.T) {
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
				{Key: "cloud_platform",
					Value: attribute.StringValue("gcp")},
				{Key: "host_id",
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
		}, {
			desc: "use default when value is empty string",
			detectedAttributes: []attribute.KeyValue{
				{Key: "location",
					Value: attribute.StringValue("us-central1")},
				{Key: "cloud_platform",
					Value: attribute.StringValue("")},
				{Key: "host_id",
					Value: attribute.StringValue("")},
			},
			wantAttributes: attribute.NewSet(attribute.KeyValue{
				Key:   "location",
				Value: attribute.StringValue("us-central1"),
			}, attribute.KeyValue{
				Key:   "cloud_platform",
				Value: attribute.StringValue("unknown"),
			}, attribute.KeyValue{
				Key:   "host_id",
				Value: attribute.StringValue("unknown"),
			}),
		},
	} {
		t.Run(test.desc, func(t *testing.T) {
			resourceOptions := []resource.Option{resource.WithAttributes(test.detectedAttributes...)}
			result, err := getPreparedResource(ctx, "project", resourceOptions)
			if err != nil {
				t.Errorf("getPreparedResource: %v", err)
			}
			resultSet := result.Set()
			for _, want := range test.wantAttributes.ToSlice() {
				got, exists := resultSet.Value(want.Key)
				if !exists {
					t.Errorf("getPreparedResource: %v not set", want.Key)
					continue
				}
				if got != want.Value {
					t.Errorf("getPreparedResource: want[%v] = %v, got: %v", want.Key, want.Value, got)
					continue
				}
			}
		})
	}
}

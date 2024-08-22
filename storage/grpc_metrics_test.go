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
	"go.opentelemetry.io/otel/sdk/metric/metricdata"
	"go.opentelemetry.io/otel/sdk/resource"
	"google.golang.org/api/iterator"
)

func TestMetrics(t *testing.T) {
	if testing.Short() && !replaying {
		t.Skip("Integration tests skipped in short mode")
	}
	ctx := context.Background()
	grpcClient, err := NewGRPCClient(ctx)
	if err != nil {
		log.Fatalf("Error setting up gRPC client: %v", err)
	}
	defer grpcClient.Close()
	bucket := grpcClient.Bucket("anima-frank-gcs-grpc-team-test-central1")
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

func TestNonDefaultEndpoint(t *testing.T) {
	for _, test := range []struct {
		desc          string
		inputEndpoint string
		wantEndpoint  string
	}{
		{
			desc:          "use exporter default endpoint for monitoring API",
			inputEndpoint: "storage.googelapis.com",
			wantEndpoint:  "",
		},
		{
			desc:          "use private endpoint if provided to storage",
			inputEndpoint: "private.googleapis.com",
			wantEndpoint:  "private.googleapis.com",
		},
		{
			desc:          "use restricted endpoint if provided to storage",
			inputEndpoint: "restricted.googleapis.com",
			wantEndpoint:  "restricted.googleapis.com",
		},
	} {
		t.Run(test.desc, func(t *testing.T) {
			got := determineMonitoringEndpoint(test.inputEndpoint)
			if got != test.wantEndpoint {
				t.Errorf("determineMonitoringEndpoint: got=%v, want=%v", got, test.wantEndpoint)
			}
		})
	}
}

func TestMetricFormatter(t *testing.T) {
	want := "storage.googleapis.com/client/metric/name"
	s := metricdata.Metrics{Name: "metric.name"}
	got := metricFormatter(s)
	if want != got {
		t.Errorf("got: %v, want %v", got, want)
	}
}

func TestNewPreparedResource(t *testing.T) {
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
			result, err := newPreparedResource(ctx, "project", resourceOptions)
			if err != nil {
				t.Errorf("newPreparedResource: %v", err)
			}
			resultSet := result.resource.Set()
			for _, want := range test.wantAttributes.ToSlice() {
				got, exists := resultSet.Value(want.Key)
				if !exists {
					t.Errorf("newPreparedResource: %v not set", want.Key)
					continue
				}
				if got != want.Value {
					t.Errorf("newPreparedResource: want[%v] = %v, got: %v", want.Key, want.Value, got)
					continue
				}
			}
		})
	}
}

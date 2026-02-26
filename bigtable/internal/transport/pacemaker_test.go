// Copyright 2026 Google LLC
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//	http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package internal

import (
	"context"
	"io"
	"log"
	"testing"
	"time"

	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/metric/metricdata"
)

// helper to find a metric in the collected resource metrics
func findPacemakerMetric(rm metricdata.ResourceMetrics, name string) (metricdata.Metrics, bool) {
	for _, sm := range rm.ScopeMetrics {
		for _, m := range sm.Metrics {
			if m.Name == name {
				return m, true
			}
		}
	}
	return metricdata.Metrics{}, false
}

func TestPacemakerExporting(t *testing.T) {
	reader := sdkmetric.NewManualReader()
	provider := sdkmetric.NewMeterProvider(sdkmetric.WithReader(reader))

	logger := log.New(io.Discard, "", 0)
	pm := NewPacemaker(provider, logger)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	pm.Start(ctx)

	// 4. Wait for ticks
	// The pacemaker ticks every 100ms. Waiting 250ms ensures we capture at least 2 ticks.
	time.Sleep(250 * time.Millisecond)

	var rm metricdata.ResourceMetrics
	if err := reader.Collect(ctx, &rm); err != nil {
		t.Fatalf("Failed to collect metrics: %v", err)
	}

	metric, ok := findPacemakerMetric(rm, "pacemaker_delays")
	if !ok {
		t.Fatalf("Metric 'pacemaker_delays' not found in exported metrics")
	}

	if metric.Unit != "us" {
		t.Errorf("Metric unit mismatch: got %q, want 'us'", metric.Unit)
	}

	hist, ok := metric.Data.(metricdata.Histogram[float64])
	if !ok {
		t.Fatalf("Metric data type mismatch: expected Histogram[float64], got %T", metric.Data)
	}

	// 9. Verify Data Points
	// We expect the total count of recorded values to be at least 1
	var totalCount uint64
	for _, dp := range hist.DataPoints {
		totalCount += dp.Count
		// Check for the "executor" attribute
		foundExecutor := false
		for _, attr := range dp.Attributes.ToSlice() {
			if attr.Key == "executor" {
				if attr.Value.AsString() == "goroutine" {
					foundExecutor = true
				} else {
					t.Errorf("Unexpected attribute value for 'executor': got %q, want 'goroutine'", attr.Value.AsString())
				}
			}
		}
		if !foundExecutor {
			t.Errorf("Data point missing 'executor' attribute")
		}
	}

	if totalCount < 1 {
		t.Errorf("Expected at least 1 recorded data points, got %d", totalCount)
	}

	// 10. Cleanup
	pm.Stop()
}

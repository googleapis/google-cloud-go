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

// sumPacemakerDelays returns the total of every recorded pacemaker_delays
// value, in microseconds, along with the number of recordings.
func sumPacemakerDelays(t *testing.T, reader sdkmetric.Reader) (sum float64, count uint64) {
	t.Helper()
	var rm metricdata.ResourceMetrics
	if err := reader.Collect(context.Background(), &rm); err != nil {
		t.Fatalf("Collect: %v", err)
	}
	m, ok := findPacemakerMetric(rm, "pacemaker_delays")
	if !ok {
		t.Fatal("Metric 'pacemaker_delays' not found in exported metrics")
	}
	hist, ok := m.Data.(metricdata.Histogram[float64])
	if !ok {
		t.Fatalf("Metric data type mismatch: expected Histogram[float64], got %T", m.Data)
	}
	for _, dp := range hist.DataPoints {
		sum += dp.Sum
		count += dp.Count
	}
	return sum, count
}

// The delay reported for a tick is how late the goroutine was to handle it,
// measured against the time the tick was due. In particular it does not depend
// on the previous tick, so starvation shorter than one full interval -- which
// drops no ticks at all -- is still reported.
func TestPacemakerRecordTick(t *testing.T) {
	for _, tc := range []struct {
		name   string
		late   time.Duration
		wantUs float64
	}{
		{name: "on time", late: 0, wantUs: 0},
		{name: "later than a tick interval", late: 250 * time.Millisecond, wantUs: 250000},
		{name: "shorter than a tick interval", late: 20 * time.Millisecond, wantUs: 20000},
		{name: "sub-millisecond", late: 300 * time.Microsecond, wantUs: 300},
	} {
		t.Run(tc.name, func(t *testing.T) {
			reader := sdkmetric.NewManualReader()
			pm := NewPacemaker(sdkmetric.NewMeterProvider(sdkmetric.WithReader(reader)), log.New(io.Discard, "", 0))

			now := time.Now()
			pm.recordTick(context.Background(), now.Add(-tc.late), now)

			got, count := sumPacemakerDelays(t, reader)
			if count != 1 {
				t.Fatalf("recorded %d values, want 1", count)
			}
			if got != tc.wantUs {
				t.Errorf("delay = %vus, want %vus", got, tc.wantUs)
			}
		})
	}
}

// A clock that runs backwards between the due time and the read must not
// record a negative delay into the histogram.
func TestPacemakerRecordTickNeverNegative(t *testing.T) {
	reader := sdkmetric.NewManualReader()
	pm := NewPacemaker(sdkmetric.NewMeterProvider(sdkmetric.WithReader(reader)), log.New(io.Discard, "", 0))

	now := time.Now()
	pm.recordTick(context.Background(), now.Add(time.Second), now)

	if got, _ := sumPacemakerDelays(t, reader); got != 0 {
		t.Errorf("delay = %vus, want 0", got)
	}
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

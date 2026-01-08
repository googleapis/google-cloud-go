// Copyright 2025 Google LLC
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

//go:build synctest

package internal

import (
	"context"
	"testing"
	"testing/synctest"
	"time"

	btopt "cloud.google.com/go/bigtable/internal/option"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/metric/metricdata"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
)

func TestRecordClientStartUp(t *testing.T) {
	fake := &fakeService{}
	addr := setupTestServer(t, fake)
	dialFunc := func() (*BigtableConn, error) { return dialBigtableserver(addr) }

	synctest.Test(t, func(t *testing.T) {
		ctx := context.Background()
		reader := metric.NewManualReader()
		provider := metric.NewMeterProvider(metric.WithReader(reader))

		poolSize := 1
		startTime := time.Now()
		sleepTimer := 500
		time.Sleep(time.Duration(sleepTimer) * time.Millisecond)

		channelPoolOptions := append(poolOpts(), WithMeterProvider(provider))
		pool, err := NewBigtableChannelPool(ctx, poolSize, btopt.RoundRobin, dialFunc, startTime, channelPoolOptions...)

		if err != nil {
			t.Fatalf("NewBigtableChannelPool failed: %v", err)
		}

		defer pool.Close()

		// Collect metrics
		rm := metricdata.ResourceMetrics{}
		if err := reader.Collect(ctx, &rm); err != nil {
			t.Fatalf("Failed to collect metrics: %v", err)
		}

		if len(rm.ScopeMetrics) == 0 {
			t.Fatalf("No scope metrics found")
		}
		sm := rm.ScopeMetrics[0]
		if sm.Scope.Name != clientMeterName {
			t.Errorf("Scope name got %q, want %q", sm.Scope.Name, clientMeterName)
		}

		if len(sm.Metrics) == 0 {
			t.Fatalf("No metrics found")
		}
		m := sm.Metrics[0]

		if m.Name != "startup_time" {
			t.Errorf("Metric name got %q, want %q", m.Name, "startup_time")
		}
		if m.Unit != "ms" {
			t.Errorf("Metric unit got %q, want %q", m.Unit, "ms")
		}

		hist, ok := m.Data.(metricdata.Histogram[float64])
		if !ok {
			t.Fatalf("Metric data is not a Histogram: %T", m.Data)
		}

		if len(hist.DataPoints) != 1 {
			t.Fatalf("Expected 1 data point, got %d", len(hist.DataPoints))
		}
		dp := hist.DataPoints[0]
		expectedAttrs := attribute.NewSet(
			attribute.String("transport_type", "unknown"),
			attribute.String("status", "OK"),
		)
		if !dp.Attributes.Equals(&expectedAttrs) {
			t.Errorf("Attributes got %v, want %v", dp.Attributes, expectedAttrs)
		}
		if dp.Count != 1 {
			t.Errorf("Data point count got %d, want 1", dp.Count)
		}
		if dp.Sum != float64(sleepTimer) {
			t.Errorf("Expected %f, got %f", float64(sleepTimer), dp.Sum)
		}
	})
}

func TestConnectionFactoryWithSyncTest(t *testing.T) {
	fake := &fakeService{}
	addr := setupTestServer(t, fake)
	goodDialFunc := func() (*BigtableConn, error) { return dialBigtableserver(addr) }

	someErr := status.Error(codes.Unavailable, "error1")

	tests := []struct {
		name           string
		dialFunc       func() (*BigtableConn, error)
		primeErrors    []error
		ctxTimeout     time.Duration
		wantErr        bool
		wantPrimeCalls int
	}{
		{
			name:        "Timeout During Backoff",
			dialFunc:    goodDialFunc,
			primeErrors: []error{someErr}, // Fails 1st attempt
			// 100ms, first backoff
			ctxTimeout:     50 * time.Millisecond,
			wantErr:        true,
			wantPrimeCalls: 1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			synctest.Run(func() {
				fake.reset()
				fake.setPingErr(tt.primeErrors...)
				factory := &connectionFactory{
					dial:           tt.dialFunc,
					instanceName:   testInstanceName,
					appProfile:     testAppProfile,
					featureFlagsMD: metadata.MD{},
				}

				ctx := context.Background()
				if tt.ctxTimeout != 0 {
					var cancel context.CancelFunc
					ctx, cancel = context.WithTimeout(context.Background(), tt.ctxTimeout)
					defer cancel()
				}

				entry, err := factory.newEntry(ctx)

				if (err != nil) != tt.wantErr {
					t.Fatalf("newEntry() error = %v, wantErr %v", err, tt.wantErr)
				}

				if !tt.wantErr && entry == nil {
					t.Errorf("newEntry() returned nil entry on success")
				}
				if !tt.wantErr && entry != nil {
					defer entry.conn.Close()
				}

				if tt.wantErr {
					if entry != nil {
						t.Errorf("newEntry() returned non-nil entry on error: %v", entry)
					}
				}

				gotPrimeCalls := fake.getPingCallCount()
				if gotPrimeCalls != tt.wantPrimeCalls {
					t.Errorf("PingAndWarm was called %d times, want %d", gotPrimeCalls, tt.wantPrimeCalls)
				}
			})
		})
	}
}

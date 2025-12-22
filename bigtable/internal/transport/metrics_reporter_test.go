// Copyright 2025 Google LLC
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
	"errors"
	"testing"
	"time"

	btopt "cloud.google.com/go/bigtable/internal/option"
	"go.opentelemetry.io/otel/attribute"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/metric/metricdata"
	"google.golang.org/grpc"
	testpb "google.golang.org/grpc/interop/grpc_testing"
)

func findMetric(rm metricdata.ResourceMetrics, name string) (metricdata.Metrics, bool) {
	for _, sm := range rm.ScopeMetrics {
		for _, m := range sm.Metrics {
			if m.Name == name {
				return m, true
			}
		}
	}
	return metricdata.Metrics{}, false
}

func TestMetricsExporting(t *testing.T) {
	ctx := context.Background()
	fake := &fakeService{}
	addr := setupTestServer(t, fake)
	dialFunc := func() (*BigtableConn, error) { return dialBigtableserver(addr) }

	reader := sdkmetric.NewManualReader()
	provider := sdkmetric.NewMeterProvider(sdkmetric.WithReader(reader))

	poolSize := 2
	strategy := btopt.RoundRobin
	pool, err := NewBigtableChannelPool(ctx, poolSize, strategy, dialFunc, WithMeterProvider(provider), WithMetricsReporterConfig(btopt.DefaultMetricsReporterConfig()))
	if err != nil {
		t.Fatalf("Failed to create pool: %v", err)
	}
	defer pool.Close()

	// Wait for initial priming to settle
	time.Sleep(100 * time.Millisecond)

	// Action 1: Successful Invoke on conn 0
	req := &testpb.SimpleRequest{Payload: &testpb.Payload{Body: []byte("hello")}}
	res := &testpb.SimpleResponse{}
	if err := pool.Invoke(ctx, "/grpc.testing.BenchmarkService/UnaryCall", req, res); err != nil {
		t.Errorf("Invoke failed: %v", err)
	}

	// Action 2: Failed Invoke on conn 1
	fake.serverErr = errors.New("simulated error")
	pool.Invoke(ctx, "/grpc.testing.BenchmarkService/UnaryCall", req, res) // Error expected
	fake.serverErr = nil

	// Action 3: Successful Stream on conn 0
	stream, err := pool.NewStream(ctx, &grpc.StreamDesc{StreamName: "StreamingCall"}, "/grpc.testing.BenchmarkService/StreamingCall")
	if err != nil {
		t.Fatalf("NewStream failed: %v", err)
	}
	stream.SendMsg(req)
	stream.RecvMsg(res)
	stream.CloseSend()
	for {
		if err := stream.RecvMsg(res); err != nil {
			break
		}
	}

	// Action 4: Stream with SendMsg error on conn 1
	fake.streamSendErr = errors.New("simulated send error")
	stream2, err := pool.NewStream(ctx, &grpc.StreamDesc{StreamName: "StreamingCall"}, "/grpc.testing.BenchmarkService/StreamingCall")
	if err != nil {
		t.Fatalf("NewStream 2 failed: %v", err)
	}
	stream2.SendMsg(req) // Expect error, triggers load decrement and error count
	stream2.CloseSend()
	for {
		if err := stream2.RecvMsg(res); err != nil {
			break
		}
	}
	fake.streamSendErr = nil

	time.Sleep(20 * time.Millisecond) // Allow stream error handling to complete

	// Find the MetricsReporter and force metrics collection
	var metricsReporter *MetricsReporter
	for _, monitor := range pool.monitors {
		if mr, ok := monitor.(*MetricsReporter); ok {
			metricsReporter = mr
			break
		}
	}

	// Force metrics collection
	metricsReporter.snapshotAndRecordMetrics(ctx)

	rm := metricdata.ResourceMetrics{}
	if err := reader.Collect(ctx, &rm); err != nil {
		t.Fatalf("Failed to collect metrics: %v", err)
	}

	// --- Check outstanding_rpcs ---
	outstandingRPCs, ok := findMetric(rm, "connection_pool/outstanding_rpcs")
	if !ok {
		t.Fatalf("Metric connection_pool/outstanding_rpcs not found")
	}

	hist, ok := outstandingRPCs.Data.(metricdata.Histogram[float64])
	if !ok {
		t.Fatalf("Metric connection_pool/outstanding_rpcs is not a Histogram[float64]: %T", outstandingRPCs.Data)
	}

	// Expected: poolSize * 2 data points (unary and streaming for each connection)
	if len(hist.DataPoints) != 2 {
		t.Errorf("Outstanding RPCs histogram has %d data points, want %d", len(hist.DataPoints), poolSize*2)
	}

	var totalUnary, totalStreaming float64
	for _, dp := range hist.DataPoints {
		attrMap := make(map[string]attribute.Value)
		for _, kv := range dp.Attributes.ToSlice() {
			attrMap[string(kv.Key)] = kv.Value
		}

		if val, ok := attrMap["lb_policy"]; !ok || val.AsString() != strategy.String() {
			t.Errorf("Missing or incorrect lb_policy attribute: want %v, got %v", strategy.String(), val)
		}
		if val, ok := attrMap["transport_type"]; !ok || val.AsString() != "cloudpath" {
			t.Errorf("Missing or incorrect transport_type attribute: want 'cloudpath', got %v", val)
		}

		streamingVal, ok := attrMap["streaming"]
		if !ok {
			t.Errorf("Missing streaming attribute")
			continue
		}
		if streamingVal.Type() != attribute.BOOL {
			t.Errorf("streaming attribute is not a BOOL: got %v", streamingVal.Type())
			continue
		}
		isStreaming := streamingVal.AsBool()

		if isStreaming {
			totalStreaming += dp.Sum
		} else {
			totalUnary += dp.Sum
		}
	}
	if totalUnary != 0 {
		t.Errorf("Total Unary load sum is %f, want 0", totalUnary)
	}
	if totalStreaming != 0 {
		t.Errorf("Total Streaming load sum is %f, want 0", totalStreaming)
	}

	// --- Check per_connection_error_count ---
	errorCount, ok := findMetric(rm, "per_connection_error_count")
	if !ok {
		t.Fatalf("Metric per_connection_error_count not found")
	}
	errorHist, ok := errorCount.Data.(metricdata.Histogram[float64])
	if !ok {
		t.Fatalf("Metric per_connection_error_count is not a Histogram[float64]: %T", errorCount.Data)
	}

	if len(errorHist.DataPoints) != 1 {
		t.Errorf("Error Count histogram has %d data points, want %d", len(errorHist.DataPoints), poolSize)
	}

	var totalErrorSum float64
	for _, dp := range errorHist.DataPoints {
		totalErrorSum += dp.Sum
	}
	// Expected errors: 1 from Invoke, 1 from Stream SendMsg
	if totalErrorSum != 2 {
		t.Errorf("Total Error Count sum is %f, want 2", totalErrorSum)
	}

	// Check if error counts on entries are reset
	for _, entry := range pool.getConns() {
		if entry.errorCount.Load() != 0 {
			t.Errorf("entry.errorCount is %d after metric collection, want 0", entry.errorCount.Load())
		}
	}
}

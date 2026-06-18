// Copyright 2026 Google LLC
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

package bigtable

import (
	"context"
	"sort"
	"strings"
	"sync"
	"testing"
	"time"

	btpb "cloud.google.com/go/bigtable/apiv2/bigtablepb"
	"google.golang.org/api/option"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
)

// TestBuiltInMetricsSmoke is a regression gate for the set of built-in
// metrics the bigtable client exports under the
// "bigtable.googleapis.com/internal/client/" prefix.
//
// After a representative set of operations against the in-process fake, it
// asserts every metric name in wantInternalNames shows up in at least one
// CreateTimeSeries call to the mock monitoring server, fully qualified with
// the expected prefix.
//
// Add a new built-in metric instrument in metrics.go? Append its short name
// to wantInternalNames here so the smoke test continues to cover it.
//
// Removed or renamed a metric? Update wantInternalNames and explain why in
// the PR — silently dropping it from this list is the regression this test
// exists to prevent.
//
// Out of scope: the gRPC pass-through metrics declared in otel_metrics.go
// (grpc.client.attempt.duration, grpc.lb.rls.*, grpc.xds_client.*,
// grpc.subchannel.*). Those are emitted by grpc-go's stats handler under
// conditions (xDS / RLS / subchannel events) that aren't reproducible
// in-process against bttest, so asserting their presence here would be
// flaky-by-design. They have their own regression coverage upstream in
// grpc-go.
func TestBuiltInMetricsSmoke(t *testing.T) {
	const wantPrefix = "bigtable.googleapis.com/internal/client/"

	// One entry per metric instrument created in metrics.go's
	// createInstruments. Keep in sync with the constants there.
	wantInternalNames := []string{
		metricNameOperationLatencies,
		metricNameAttemptLatencies,
		metricNameServerLatencies,
		metricNameFirstRespLatencies,
		metricNameAppBlockingLatencies,
		metricNameClientBlockingLatencies,
		metricNameRetryCount,
		metricNameConnErrCount,
	}

	wantFullNames := make([]string, len(wantInternalNames))
	for i, n := range wantInternalNames {
		wantFullNames[i] = wantPrefix + n
	}

	// Reduce sample period so the periodic exporter fires within the test
	// timeout.
	origSamplePeriod := defaultSamplePeriod
	defaultSamplePeriod = 500 * time.Millisecond
	defer func() { defaultSamplePeriod = origSamplePeriod }()

	// Mock Cloud Monitoring server. The periodic exporter sends
	// CreateTimeSeries to it; we inspect those requests for metric names.
	monitoringServer, err := NewMetricTestServer()
	if err != nil {
		t.Fatalf("NewMetricTestServer: %v", err)
	}
	go monitoringServer.Serve()
	defer monitoringServer.Shutdown()

	origCreateExporterOptions := createExporterOptions
	createExporterOptions = func(opts ...option.ClientOption) []option.ClientOption {
		return []option.ClientOption{
			option.WithEndpoint(monitoringServer.Endpoint),
			option.WithoutAuthentication(),
			option.WithGRPCDialOption(grpc.WithTransportCredentials(insecure.NewCredentials())),
		}
	}
	defer func() { createExporterOptions = origCreateExporterOptions }()

	// Custom ReadRows handler: fail the first attempt with Unavailable so
	// the client retries (populates retry_count and connectivity_error_count),
	// then send response headers + an empty stream on subsequent attempts
	// (populates first_response_latencies + server_latencies).
	var attemptCount int
	var mu sync.Mutex
	customReadRows := func(_ any, stream btpb.Bigtable_ReadRowsServer) error {
		mu.Lock()
		attemptCount++
		n := attemptCount
		mu.Unlock()
		if n == 1 {
			return status.Error(codes.Unavailable, "smoke-test forced retry")
		}
		_ = stream.SendHeader(metadata.MD{
			locationMDKey:     []string{string(testHeaders)},
			serverTimingMDKey: []string{"gfet4t7; dur=42"},
		})
		return nil
	}

	tbl, cleanup, err := setupFakeServerWithCustomHandler(
		"smoke-project", "smoke-instance",
		ClientConfig{AppProfile: "smoke-profile"},
		customReadRows,
	)
	if err != nil {
		t.Fatalf("setupFakeServerWithCustomHandler: %v", err)
	}
	defer cleanup()

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// Drive the read path twice — first call triggers the forced retry,
	// second call exercises the steady-state path.
	if _, err := tbl.ReadRow(ctx, "row-1"); err != nil {
		t.Logf("ReadRow: %v (smoke test continues — metrics emit on the error path too)", err)
	}
	if err := tbl.ReadRows(ctx, InfiniteRange(""), func(_ Row) bool { return true }); err != nil {
		t.Logf("ReadRows: %v", err)
	}

	// Wait at least three sample periods so the periodic reader is
	// guaranteed to have flushed all observed instruments.
	time.Sleep(3 * defaultSamplePeriod)

	requests := monitoringServer.CreateServiceTimeSeriesRequests()
	if len(requests) == 0 {
		t.Fatalf("no CreateTimeSeries calls received — periodic exporter never fired (sample period %v)", defaultSamplePeriod)
	}

	gotByName := map[string]bool{}
	for _, req := range requests {
		for _, ts := range req.TimeSeries {
			gotByName[ts.Metric.Type] = true
		}
	}

	var missing []string
	for _, want := range wantFullNames {
		if !gotByName[want] {
			missing = append(missing, want)
		}
	}
	sort.Strings(missing)
	if len(missing) > 0 {
		gotList := make([]string, 0, len(gotByName))
		for name := range gotByName {
			gotList = append(gotList, name)
		}
		sort.Strings(gotList)
		t.Fatalf("missing %d metric(s) under %q:\n  missing:\n    %s\n  observed metric types (%d):\n    %s",
			len(missing), wantPrefix,
			strings.Join(missing, "\n    "),
			len(gotList),
			strings.Join(gotList, "\n    "))
	}

	// Also assert no observed metric drifts off the expected prefix —
	// catches accidental renames of the prefix constant itself.
	for name := range gotByName {
		if !strings.HasPrefix(name, wantPrefix) {
			t.Errorf("metric %q does not have the expected prefix %q", name, wantPrefix)
		}
	}
}

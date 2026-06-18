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
	"cloud.google.com/go/bigtable/bttest"
	"go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/metric/metricdata"
	"google.golang.org/api/option"
	gtransport "google.golang.org/api/transport/grpc"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
)

// TestBuiltInMetricsSmoke is the regression gate for the built-in metric
// instruments registered in metrics.go's createInstruments. After a
// representative read+write path against the in-process fake, it asserts
// every short name in wantInternalNames shows up in at least one
// CreateTimeSeries call to the mock monitoring server, fully qualified with
// the bigtable.googleapis.com/internal/client/ prefix.
//
// Add a new built-in metric instrument in metrics.go? Append its short name
// here. Removed or renamed? Update the list and explain why in the PR — the
// list is the regression this test exists to prevent silently dropping.
func TestBuiltInMetricsSmoke(t *testing.T) {
	const wantPrefix = "bigtable.googleapis.com/internal/client/"

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

	// Reduce sample period so the periodic exporter fires within the
	// test timeout.
	origSamplePeriod := defaultSamplePeriod
	defaultSamplePeriod = 500 * time.Millisecond
	defer func() { defaultSamplePeriod = origSamplePeriod }()

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

	tbl, cleanup, err := setupSmokeBigtable(t, "smoke-project", "smoke-instance", "smoke-profile", forceFirstReadRowsRetry())
	if err != nil {
		t.Fatalf("setupSmokeBigtable: %v", err)
	}
	defer cleanup()

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	mut := NewMutation()
	mut.Set("cf", "col", Timestamp(time.Now().UnixMicro()), []byte("v"))
	if err := tbl.Apply(ctx, "row-1", mut); err != nil {
		t.Logf("Apply: %v (smoke test continues — metrics emit on the error path too)", err)
	}
	if _, err := tbl.ReadRow(ctx, "row-1"); err != nil {
		t.Logf("ReadRow: %v", err)
	}
	if err := tbl.ReadRows(ctx, InfiniteRange(""), func(_ Row) bool { return true }); err != nil {
		t.Logf("ReadRows: %v", err)
	}

	// Wait at least three sample periods so the periodic reader flushes.
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

	if missing := missingNames(wantFullNames, gotByName); len(missing) > 0 {
		t.Fatalf("missing %d metric(s) under %q:\n  missing:\n    %s\n  observed (%d):\n    %s",
			len(missing), wantPrefix,
			strings.Join(missing, "\n    "),
			len(gotByName),
			strings.Join(sortedKeys(gotByName), "\n    "))
	}
	for name := range gotByName {
		if !strings.HasPrefix(name, wantPrefix) {
			t.Errorf("metric %q does not have the expected prefix %q", name, wantPrefix)
		}
	}
}

// TestGrpcOtelMetricsSmoke is the regression gate for the gRPC pass-through
// metrics the bigtable client registers via the gRPC OTel stats handler in
// newOtelMetricsContext (see otel_metrics.go). It exercises the path with a
// real RPC against bttest and asserts the expected metric short names show
// up in the OTel meter provider's ManualReader.
//
// In scope: grpc.client.attempt.duration, grpc.subchannel.open_connections,
// grpc.subchannel.connection_attempts_succeeded — these fire deterministically
// on a single successful unary RPC against the in-process fake.
//
// Out of scope: grpc.lb.rls.* and grpc.xds_client.* — these only fire under
// RLS / xDS conditions that aren't reproducible in-process against bttest.
// grpc.subchannel.disconnections and grpc.subchannel.connection_attempts_failed
// also skipped: the first needs a subchannel teardown, the second needs a
// failed dial — neither happens on a happy-path run.
//
// This test doesn't go through the bigtable Client API because the OTel
// exporter inside newBuiltinMetricsTracerFactory is hardwired to Cloud
// Monitoring; instead we drive newOtelMetricsContext directly with the
// existing test-only manualReader field. That's enough to verify the
// stats.NewMetricSet registration in otel_metrics.go still names every
// metric this list pins.
func TestGrpcOtelMetricsSmoke(t *testing.T) {
	const wantPrefix = "grpc."

	wantNames := []string{
		"grpc.client.attempt.duration",
		"grpc.subchannel.open_connections",
		"grpc.subchannel.connection_attempts_succeeded",
	}

	srv, err := bttest.NewServer("localhost:0")
	if err != nil {
		t.Fatalf("bttest.NewServer: %v", err)
	}
	defer srv.Close()

	mr := metric.NewManualReader()
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	otelCtx, err := newOtelMetricsContext(ctx, metricsConfig{
		project:         "smoke-project",
		instance:        "smoke-instance",
		appProfile:      "smoke-profile",
		clientName:      "smoke-client",
		clientUID:       "smoke-uid",
		manualReader:    mr,
		disableExporter: true, // don't try to push to real Cloud Monitoring
	})
	if err != nil {
		t.Fatalf("newOtelMetricsContext: %v", err)
	}
	defer otelCtx.close()

	// Dial bttest with the same dial options the bigtable client wires up
	// for its gRPC connections (otelCtx.clientOpts contains the OTel stats
	// handler that records the grpc.* metrics under test).
	dialOpts := append([]option.ClientOption{
		option.WithEndpoint(srv.Addr),
		option.WithoutAuthentication(),
		option.WithGRPCDialOption(grpc.WithTransportCredentials(insecure.NewCredentials())),
	}, otelCtx.clientOpts...)

	conn, err := gtransport.Dial(ctx, dialOpts...)
	if err != nil {
		t.Fatalf("gtransport.Dial: %v", err)
	}
	defer conn.Close()

	// Trigger one unary RPC. PingAndWarm against bttest may return
	// Unimplemented; either way grpc.client.attempt.duration fires on the
	// attempt, and the subchannel metrics fire on the dial.
	client := btpb.NewBigtableClient(conn)
	_, _ = client.PingAndWarm(ctx, &btpb.PingAndWarmRequest{
		Name: "projects/smoke-project/instances/smoke-instance",
	})

	// Give the OTel periodic reader / gRPC stats handler a moment to
	// publish into the ManualReader.
	time.Sleep(200 * time.Millisecond)

	var rm metricdata.ResourceMetrics
	if err := mr.Collect(ctx, &rm); err != nil {
		t.Fatalf("ManualReader.Collect: %v", err)
	}

	got := map[string]bool{}
	for _, sm := range rm.ScopeMetrics {
		for _, m := range sm.Metrics {
			got[m.Name] = true
		}
	}

	if missing := missingNames(wantNames, got); len(missing) > 0 {
		t.Fatalf("missing %d gRPC metric(s):\n  missing:\n    %s\n  observed (%d):\n    %s",
			len(missing),
			strings.Join(missing, "\n    "),
			len(got),
			strings.Join(sortedKeys(got), "\n    "))
	}
	// Sanity: every grpc.* metric we got is also in the production
	// stats.NewMetricSet list (catches "we silently turned on something
	// new and unexpected"). Non-grpc metrics from this provider are fine.
	for name := range got {
		if !strings.HasPrefix(name, wantPrefix) {
			continue
		}
		if !containsString(otelGrpcMetricSet(), name) {
			t.Errorf("got grpc metric %q not in stats.NewMetricSet allow-list — production set may have drifted from the test pin", name)
		}
	}
}

// otelGrpcMetricSet mirrors the stats.NewMetricSet(...) call in
// newOtelMetricsContext. Keep in sync; this is the cross-check that any
// production drift gets called out by the smoke test.
func otelGrpcMetricSet() []string {
	return []string{
		"grpc.client.attempt.duration",
		"grpc.lb.rls.default_target_picks",
		"grpc.lb.rls.target_picks",
		"grpc.lb.rls.failed_picks",
		"grpc.xds_client.server_failure",
		"grpc.xds_client.resource_updates_invalid",
		"grpc.subchannel.open_connections",
		"grpc.subchannel.disconnections",
		"grpc.subchannel.connection_attempts_succeeded",
		"grpc.subchannel.connection_attempts_failed",
	}
}

// setupSmokeBigtable spins up bttest with the given stream interceptor,
// dials it via option.WithEndpoint (NOT option.WithGRPCConn — that path
// bypasses the bigtable client's own dial options and the OTel stats
// handler never gets attached), and returns a Table ready for traffic.
func setupSmokeBigtable(t *testing.T, project, instance, appProfile string, opts ...grpc.ServerOption) (*Table, func(), error) {
	t.Helper()
	srv, err := bttest.NewServer("localhost:0", opts...)
	if err != nil {
		return nil, nil, err
	}

	clientOpts := []option.ClientOption{
		option.WithEndpoint(srv.Addr),
		option.WithoutAuthentication(),
		option.WithGRPCDialOption(grpc.WithTransportCredentials(insecure.NewCredentials())),
		option.WithGRPCConnectionPool(1),
	}

	ctx := context.Background()
	client, err := NewClientWithConfig(ctx, project, instance, ClientConfig{AppProfile: appProfile}, clientOpts...)
	if err != nil {
		srv.Close()
		return nil, nil, err
	}
	adminClient, err := NewAdminClient(ctx, project, instance, clientOpts...)
	if err != nil {
		client.Close()
		srv.Close()
		return nil, nil, err
	}
	if err := adminClient.CreateTable(ctx, "smoke-table"); err != nil {
		adminClient.Close()
		client.Close()
		srv.Close()
		return nil, nil, err
	}
	if err := adminClient.CreateColumnFamily(ctx, "smoke-table", "cf"); err != nil {
		adminClient.Close()
		client.Close()
		srv.Close()
		return nil, nil, err
	}

	tbl := client.Open("smoke-table")
	cleanup := func() {
		adminClient.Close()
		client.Close()
		srv.Close()
	}
	return tbl, cleanup, nil
}

// forceFirstReadRowsRetry returns a stream interceptor that fails the very
// first ReadRows attempt with Unavailable so the client retries — required
// to populate retry_count and connectivity_error_count. Subsequent calls
// send a server-timing + location header (populates server_latencies and
// first_response_latencies) and let bttest serve normally.
func forceFirstReadRowsRetry() grpc.ServerOption {
	var (
		mu    sync.Mutex
		count int
	)
	return grpc.StreamInterceptor(func(srv interface{}, ss grpc.ServerStream, info *grpc.StreamServerInfo, handler grpc.StreamHandler) error {
		if strings.HasSuffix(info.FullMethod, "ReadRows") {
			mu.Lock()
			count++
			n := count
			mu.Unlock()
			if n == 1 {
				return status.Error(codes.Unavailable, "smoke-test forced retry")
			}
		}
		_ = ss.SendHeader(metadata.New(map[string]string{
			serverTimingMDKey: "gfet4t7; dur=42",
			locationMDKey:     string(testHeaders),
		}))
		return handler(srv, ss)
	})
}

func missingNames(want []string, got map[string]bool) []string {
	var miss []string
	for _, w := range want {
		if !got[w] {
			miss = append(miss, w)
		}
	}
	sort.Strings(miss)
	return miss
}

func sortedKeys(m map[string]bool) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

func containsString(haystack []string, needle string) bool {
	for _, s := range haystack {
		if s == needle {
			return true
		}
	}
	return false
}

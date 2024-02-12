//go:build go1.20
// +build go1.20

/*
Copyright 2024 Google LLC

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

	http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/
package test

import (
	"context"
	"errors"
	"testing"
	"time"

	"cloud.google.com/go/spanner"
	"cloud.google.com/go/spanner/apiv1/spannerpb"
	"cloud.google.com/go/spanner/internal"
	stestutil "cloud.google.com/go/spanner/internal/testutil"
	structpb "github.com/golang/protobuf/ptypes/struct"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/sdk/metric/metricdata"
	"go.opentelemetry.io/otel/sdk/metric/metricdata/metricdatatest"
	"google.golang.org/api/iterator"
)

func TestOTMetrics_InstrumentationScope(t *testing.T) {
	ctx := context.Background()
	te := newOpenTelemetryTestExporter(false, false)
	t.Cleanup(func() {
		te.Unregister(ctx)
	})
	spanner.EnableOpenTelemetryMetrics()
	_, c, teardown := setupMockedTestServerWithConfig(t, spanner.ClientConfig{OpenTelemetryMeterProvider: te.mp})
	defer teardown()

	c.Single().ReadRow(context.Background(), "Users", spanner.Key{"alice"}, []string{"email"})
	rm, err := te.metrics(ctx)
	if err != nil {
		t.Error(err)
	}
	if len(rm.ScopeMetrics) != 1 {
		t.Fatalf("Error in number of instrumentation scope, got: %d, want: %d", len(rm.ScopeMetrics), 1)
	}
	if rm.ScopeMetrics[0].Scope.Name != spanner.OtInstrumentationScope {
		t.Fatalf("Error in instrumentation scope name, got: %s, want: %s", rm.ScopeMetrics[0].Scope.Name, spanner.OtInstrumentationScope)
	}
	if rm.ScopeMetrics[0].Scope.Version != internal.Version {
		t.Fatalf("Error in instrumentation scope version, got: %s, want: %s", rm.ScopeMetrics[0].Scope.Version, internal.Version)
	}
}

func TestOTMetrics_SessionPool(t *testing.T) {
	ctx := context.Background()
	te := newOpenTelemetryTestExporter(false, false)
	t.Cleanup(func() {
		te.Unregister(ctx)
	})
	spanner.EnableOpenTelemetryMetrics()

	_, client, teardown := setupMockedTestServerWithConfig(t, spanner.ClientConfig{OpenTelemetryMeterProvider: te.mp})
	defer teardown()
	client.Single().ReadRow(context.Background(), "Users", spanner.Key{"alice"}, []string{"email"})

	for _, test := range []struct {
		name           string
		expectedMetric metricdata.Metrics
	}{
		{
			"OpenSessionCount",
			metricdata.Metrics{
				Name:        "spanner/open_session_count",
				Description: "Number of sessions currently opened",
				Unit:        "1",
				Data: metricdata.Gauge[int64]{
					DataPoints: []metricdata.DataPoint[int64]{
						{
							Attributes: attribute.NewSet(getAttributes(client.ClientID())...),
							Value:      25,
						},
					},
				},
			},
		},
		{
			"MaxAllowedSessionsCount",
			metricdata.Metrics{
				Name:        "spanner/max_allowed_sessions",
				Description: "The maximum number of sessions allowed. Configurable by the user.",
				Unit:        "1",
				Data: metricdata.Gauge[int64]{
					DataPoints: []metricdata.DataPoint[int64]{
						{
							Attributes: attribute.NewSet(getAttributes(client.ClientID())...),
							Value:      400,
						},
					},
				},
			},
		},
		{
			"MaxInUseSessionsCount",
			metricdata.Metrics{
				Name:        "spanner/max_in_use_sessions",
				Description: "The maximum number of sessions in use during the last 10 minute interval.",
				Unit:        "1",
				Data: metricdata.Gauge[int64]{
					DataPoints: []metricdata.DataPoint[int64]{
						{
							Attributes: attribute.NewSet(getAttributes(client.ClientID())...),
							Value:      1,
						},
					},
				},
			},
		},
		{
			"AcquiredSessionsCount",
			metricdata.Metrics{
				Name:        "spanner/num_acquired_sessions",
				Description: "The number of sessions acquired from the session pool.",
				Unit:        "1",
				Data: metricdata.Sum[int64]{
					DataPoints: []metricdata.DataPoint[int64]{
						{
							Attributes: attribute.NewSet(getAttributes(client.ClientID())...),
							Value:      1,
						},
					},
					Temporality: metricdata.CumulativeTemporality,
					IsMonotonic: true,
				},
			},
		},
		{
			"ReleasedSessionsCount",
			metricdata.Metrics{
				Name:        "spanner/num_released_sessions",
				Description: "The number of sessions released by the user and pool maintainer.",
				Unit:        "1",
				Data: metricdata.Sum[int64]{
					DataPoints: []metricdata.DataPoint[int64]{
						{
							Attributes: attribute.NewSet(getAttributes(client.ClientID())...),
							Value:      1,
						},
					},
					Temporality: metricdata.CumulativeTemporality,
					IsMonotonic: true,
				},
			},
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			metricName := test.expectedMetric.Name
			expectedMetric := test.expectedMetric
			validateOTMetric(ctx, t, te, metricName, expectedMetric)
		})
	}
}

func TestOTMetrics_SessionPool_SessionsCount(t *testing.T) {
	ctx := context.Background()
	te := newOpenTelemetryTestExporter(false, false)
	t.Cleanup(func() {
		te.Unregister(ctx)
	})

	spanner.EnableOpenTelemetryMetrics()
	server, client, teardown := setupMockedTestServerWithConfig(t, spanner.ClientConfig{SessionPoolConfig: spanner.DefaultSessionPoolConfig, OpenTelemetryMeterProvider: te.mp})
	client.DatabaseName()
	defer teardown()
	// Wait for the session pool initialization to finish.
	expectedReads := spanner.DefaultSessionPoolConfig.MinOpened
	waitFor(t, func() error {
		if uint64(server.TestSpanner.TotalSessionsCreated()) == expectedReads {
			return nil
		}
		return errors.New("Not yet initialized")
	})

	client.Single().ReadRow(context.Background(), "Users", spanner.Key{"alice"}, []string{"email"})

	attributesNumInUseSessions := append(getAttributes(client.ClientID()), attribute.Key("type").String("num_in_use_sessions"))
	attributesNumSessions := append(getAttributes(client.ClientID()), attribute.Key("type").String("num_sessions"))

	expectedMetricData := metricdata.Metrics{
		Name:        "spanner/num_sessions_in_pool",
		Description: "The number of sessions currently in use.",
		Unit:        "1",
		Data: metricdata.Gauge[int64]{
			DataPoints: []metricdata.DataPoint[int64]{
				{
					Attributes: attribute.NewSet(attributesNumInUseSessions...),
					Value:      0,
				},
				{
					Attributes: attribute.NewSet(attributesNumSessions...),
					Value:      100,
				},
			},
		},
	}

	validateOTMetric(ctx, t, te, expectedMetricData.Name, expectedMetricData)
}

func TestOTMetrics_SessionPool_GetSessionTimeoutsCount(t *testing.T) {
	ctx1 := context.Background()
	te := newOpenTelemetryTestExporter(false, false)
	t.Cleanup(func() {
		te.Unregister(ctx1)
	})
	spanner.EnableOpenTelemetryMetrics()
	server, client, teardown := setupMockedTestServerWithConfig(t, spanner.ClientConfig{OpenTelemetryMeterProvider: te.mp})
	defer teardown()

	server.TestSpanner.PutExecutionTime(stestutil.MethodBatchCreateSession,
		stestutil.SimulatedExecutionTime{
			MinimumExecutionTime: 2 * time.Millisecond,
		})

	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Millisecond)
	defer cancel()
	client.Single().ReadRow(ctx, "Users", spanner.Key{"alice"}, []string{"email"})

	expectedMetricData := metricdata.Metrics{
		Name:        "spanner/get_session_timeouts",
		Description: "The number of get sessions timeouts due to pool exhaustion.",
		Unit:        "1",
		Data: metricdata.Sum[int64]{
			DataPoints: []metricdata.DataPoint[int64]{
				{
					Attributes: attribute.NewSet(getAttributes(client.ClientID())...),
					Value:      1,
				},
			},
			Temporality: metricdata.CumulativeTemporality,
			IsMonotonic: true,
		},
	}
	validateOTMetric(ctx1, t, te, expectedMetricData.Name, expectedMetricData)
}

func TestOTMetrics_GFELatency(t *testing.T) {
	ctx := context.Background()
	te := newOpenTelemetryTestExporter(false, false)
	t.Cleanup(func() {
		te.Unregister(ctx)
	})
	spanner.EnableOpenTelemetryMetrics()
	server, client, teardown := setupMockedTestServerWithConfig(t, spanner.ClientConfig{OpenTelemetryMeterProvider: te.mp})
	defer teardown()

	if err := server.TestSpanner.PutStatementResult("SELECT email FROM Users", &stestutil.StatementResult{
		Type: stestutil.StatementResultResultSet,
		ResultSet: &spannerpb.ResultSet{
			Metadata: &spannerpb.ResultSetMetadata{
				RowType: &spannerpb.StructType{
					Fields: []*spannerpb.StructType_Field{
						{
							Name: "email",
							Type: &spannerpb.Type{Code: spannerpb.TypeCode_STRING},
						},
					},
				},
			},
			Rows: []*structpb.ListValue{
				{Values: []*structpb.Value{{
					Kind: &structpb.Value_StringValue{StringValue: "test@test.com"},
				}}},
			},
		},
	}); err != nil {
		t.Fatalf("could not add result: %v", err)
	}
	iter := client.Single().Read(context.Background(), "Users", spanner.AllKeys(), []string{"email"})
	for {
		_, err := iter.Next()
		if err == iterator.Done {
			break
		}
		if err != nil {
			t.Fatal(err.Error())
		}
	}

	attributeGFELatency := append(getAttributes(client.ClientID()), attribute.Key("grpc_client_method").String("executeBatchCreateSessions"))

	resourceMetrics, err := te.metrics(context.Background())
	if err != nil {
		t.Error(err)
	}
	if resourceMetrics == nil {
		t.Fatal("Resource Metrics is nil")
	}
	if got, want := len(resourceMetrics.ScopeMetrics), 1; got != want {
		t.Fatalf("ScopeMetrics length mismatch, got %v, want %v", got, want)
	}

	gfeLatencyMetricName := "spanner/gfe_latency"
	idx := getMetricIndex(resourceMetrics.ScopeMetrics[0].Metrics, gfeLatencyMetricName)
	if idx == -1 {
		t.Fatalf("Metric Name %s not found", gfeLatencyMetricName)
	}
	gfeLatencyRecordedMetric := resourceMetrics.ScopeMetrics[0].Metrics[idx]
	if gfeLatencyRecordedMetric.Name != gfeLatencyMetricName {
		t.Fatalf("Got metric name: %s, want: %s", gfeLatencyRecordedMetric.Name, gfeLatencyMetricName)
	}
	if _, ok := gfeLatencyRecordedMetric.Data.(metricdata.Histogram[int64]); !ok {
		t.Fatal("gfe latency metric data not of type metricdata.Histogram[int64]")
	}
	gfeLatencyRecordedMetricData := gfeLatencyRecordedMetric.Data.(metricdata.Histogram[int64])
	count := gfeLatencyRecordedMetricData.DataPoints[0].Count
	if got, want := count, uint64(0); got <= want {
		t.Fatalf("Incorrect data: got %d, wanted more than %d for metric %v", got, want, gfeLatencyRecordedMetric.Name)
	}
	metricdatatest.AssertHasAttributes[metricdata.HistogramDataPoint[int64]](t, gfeLatencyRecordedMetricData.DataPoints[0], attributeGFELatency...)

	gfeHeaderMissingMetric := "spanner/gfe_header_missing_count"
	idx1 := getMetricIndex(resourceMetrics.ScopeMetrics[0].Metrics, gfeHeaderMissingMetric)
	if idx1 == -1 {
		t.Fatalf("Metric Name %s not found", gfeHeaderMissingMetric)
	}

	expectedMetricData := metricdata.Metrics{
		Name:        gfeHeaderMissingMetric,
		Description: "Number of RPC responses received without the server-timing header, most likely means that the RPC never reached Google's network",
		Unit:        "1",
		Data: metricdata.Sum[int64]{
			DataPoints: []metricdata.DataPoint[int64]{
				{
					Attributes: attribute.NewSet(getAttributes(client.ClientID())...),
					Value:      1,
				},
			},
			Temporality: metricdata.CumulativeTemporality,
			IsMonotonic: true,
		},
	}
	metricdatatest.AssertEqual(t, expectedMetricData, resourceMetrics.ScopeMetrics[0].Metrics[idx1], metricdatatest.IgnoreTimestamp(), metricdatatest.IgnoreExemplars())
}

func getMetricIndex(metrics []metricdata.Metrics, metricName string) int64 {
	for i, metric := range metrics {
		if metric.Name == metricName {
			return int64(i)
		}
	}
	return -1
}

func getAttributes(clientID string) []attribute.KeyValue {
	return []attribute.KeyValue{
		attribute.Key("client_id").String(clientID),
		attribute.Key("database").String("[DATABASE]"),
		attribute.Key("instance_id").String("[INSTANCE]"),
		attribute.Key("library_version").String(internal.Version),
	}
}

func validateOTMetric(ctx context.Context, t *testing.T, te *openTelemetryTestExporter, metricName string, expectedMetric metricdata.Metrics) {
	resourceMetrics, err := te.metrics(ctx)
	if err != nil {
		t.Error(err)
	}
	if resourceMetrics == nil {
		t.Fatal("Resource Metrics is nil")
	}
	if got, want := len(resourceMetrics.ScopeMetrics), 1; got != want {
		t.Fatalf("ScopeMetrics length mismatch, got %v, want %v", got, want)
	}

	idx := getMetricIndex(resourceMetrics.ScopeMetrics[0].Metrics, metricName)
	if idx == -1 {
		t.Fatalf("Metric Name %s not found", metricName)
	}
	metricdatatest.AssertEqual(t, expectedMetric, resourceMetrics.ScopeMetrics[0].Metrics[idx], metricdatatest.IgnoreTimestamp(), metricdatatest.IgnoreExemplars())
}

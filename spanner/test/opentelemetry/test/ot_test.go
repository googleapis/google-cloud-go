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
	"testing"

	"cloud.google.com/go/spanner"
	"cloud.google.com/go/spanner/internal"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/sdk/metric/metricdata"
	"go.opentelemetry.io/otel/sdk/metric/metricdata/metricdatatest"
)

/*func TestOTStats(t *testing.T) {
	t.Logf("restarting the test with different parameter")
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
	t.Logf("Length of scope metrics %d", len(rm.ScopeMetrics))
	t.Logf("Length of scope metrics %q", rm.ScopeMetrics)
	t.Logf("Length of scope metrics %q", rm.ScopeMetrics[0].Metrics)
	t.Logf("Length of scope metrics %q", rm.ScopeMetrics[0].Scope)
	t.Logf("Length of scope metrics %q", rm.ScopeMetrics[1].Metrics)
	t.Logf("Length of scope metrics %d", len(rm.ScopeMetrics[1].Metrics))
	t.Logf("Length of scope metrics %q", rm.ScopeMetrics[1].Scope)

	for _, m := range rm.ScopeMetrics {
		t.Log(m.Scope)
		for _, metric := range m.Metrics {
			t.Log(metric.Data)
			switch metric.Data.(type) {
			case metricdata.Gauge[int64]:
				a := metric.Data.(metricdata.Gauge[int64]).DataPoints
				t.Log(a)
			case metricdata.Gauge[float64]:
				a := metric.Data.(metricdata.Gauge[float64]).DataPoints
				t.Log(a)
			case metricdata.Sum[int64]:
				a := metric.Data.(metricdata.Sum[int64]).DataPoints
				t.Log(a)
			case metricdata.Sum[float64]:
				a := metric.Data.(metricdata.Sum[float64]).DataPoints
				t.Log(a)
			}
		}
	}
	//metricdatatest.AssertAggregationsEqual()
}*/

func getMetricIndex(metrics []metricdata.Metrics, metricName string) int64 {
	for i, metric := range metrics {
		if metric.Name == metricName {
			return int64(i)
		}
	}
	return -1
}

func TestOTStats_SessionPool(t *testing.T) {
	for _, test := range []struct {
		name           string
		expectedMetric metricdata.Metrics
	}{
		{
			"OpenSessionCount",
			metricdata.Metrics{
				Name:        "open_session_count_test_ot_local",
				Description: "Number of sessions currently opened",
				Unit:        "1",
				Data: metricdata.Gauge[int64]{
					DataPoints: []metricdata.DataPoint[int64]{
						{
							Attributes: getAttributes("client-1"),
							Value:      25,
						},
					},
				},
			},
		},
		{
			"MaxAllowedSessionsCount",
			metricdata.Metrics{
				Name:        "max_allowed_sessions_test_ot_local",
				Description: "The maximum number of sessions allowed. Configurable by the user.",
				Unit:        "1",
				Data: metricdata.Gauge[int64]{
					DataPoints: []metricdata.DataPoint[int64]{
						{
							Attributes: getAttributes("client-2"),
							Value:      400,
						},
					},
				},
			},
		},
		{
			"MaxInUseSessionsCount",
			metricdata.Metrics{
				Name:        "max_in_use_sessions_test_ot_local",
				Description: "The maximum number of sessions in use during the last 10 minute interval.",
				Unit:        "1",
				Data: metricdata.Gauge[int64]{
					DataPoints: []metricdata.DataPoint[int64]{
						{
							Attributes: getAttributes("client-3"),
							Value:      1,
						},
					},
				},
			},
		},
		{
			"AcquiredSessionsCount",
			metricdata.Metrics{
				Name:        "num_acquired_sessions_test_ot_ctr_local",
				Description: "The number of sessions acquired from the session pool.",
				Unit:        "1",
				Data: metricdata.Sum[int64]{
					DataPoints: []metricdata.DataPoint[int64]{
						{
							Attributes: getAttributes("client-4"),
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
				Name:        "num_released_sessions_test_ot_ctr_local",
				Description: "The number of sessions released by the user and pool maintainer.",
				Unit:        "1",
				Data: metricdata.Sum[int64]{
					DataPoints: []metricdata.DataPoint[int64]{
						{
							Attributes: getAttributes("client-5"),
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
			testSimpleOTMetric(t, test.expectedMetric.Name, test.expectedMetric)
		})
	}
}

func getAttributes(clientId string) attribute.Set {
	return attribute.NewSet(
		attribute.Key("client_id").String(clientId),
		attribute.Key("database").String("[DATABASE]"),
		attribute.Key("instance_id").String("[INSTANCE]"),
		attribute.Key("library_version").String(internal.Version),
	)
}

func testSimpleOTMetric(t *testing.T, metricName string, expectedMetric metricdata.Metrics) {
	ctx := context.Background()
	te := newOpenTelemetryTestExporter(false, false)
	t.Cleanup(func() {
		te.Unregister(ctx)
	})
	spanner.EnableOpenTelemetryMetrics()

	_, client, teardown := setupMockedTestServerWithConfig(t, spanner.ClientConfig{OpenTelemetryMeterProvider: te.mp})
	defer teardown()

	client.Single().ReadRow(context.Background(), "Users", spanner.Key{"alice"}, []string{"email"})

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

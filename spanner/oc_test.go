// Copyright 2018 Google LLC
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package spanner

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	"cloud.google.com/go/internal/testutil"
	"cloud.google.com/go/spanner/apiv1/spannerpb"
	"cloud.google.com/go/spanner/internal"
	stestutil "cloud.google.com/go/spanner/internal/testutil"
	"go.opencensus.io/stats/view"
	"go.opencensus.io/tag"
	"google.golang.org/api/iterator"
	structpb "google.golang.org/protobuf/types/known/structpb"
)

// Check that stats are being exported.
func TestOCStats(t *testing.T) {
	DisableGfeLatencyAndHeaderMissingCountViews()
	te := testutil.NewTestExporter()
	defer te.Unregister()

	_, c, teardown := setupMockedTestServer(t)
	defer teardown()

	c.Single().ReadRow(context.Background(), "Users", Key{"alice"}, []string{"email"})
	// Wait until we see data from the view.
	select {
	case <-te.Stats:
	case <-time.After(1 * time.Second):
		t.Fatal("no stats were exported before timeout")
	}
}

func TestOCStats_SessionPool(t *testing.T) {
	skipUnsupportedPGTest(t)
	DisableGfeLatencyAndHeaderMissingCountViews()
	// expectedValues is a map of expected values for different configurations of
	// multiplexed session env="GOOGLE_CLOUD_SPANNER_MULTIPLEXED_SESSIONS".
	expectedValues := map[string]map[bool]string{
		"open_session_count": {
			false: "25",
			// since we are doing only R/O operations and MinOpened=0, we should have only one session.
			true: "1",
		},
		"max_in_use_sessions": {
			false: "1",
			true:  "0",
		},
	}
	for _, test := range []struct {
		name    string
		view    *view.View
		measure string
		value   string
	}{
		{
			"OpenSessionCount",
			OpenSessionCountView,
			"open_session_count",
			expectedValues["open_session_count"][isMultiplexEnabled],
		},
		{
			"MaxAllowedSessionsCount",
			MaxAllowedSessionsCountView,
			"max_allowed_sessions",
			"400",
		},
		{
			"MaxInUseSessionsCount",
			MaxInUseSessionsCountView,
			"max_in_use_sessions",
			expectedValues["max_in_use_sessions"][isMultiplexEnabled],
		},
		{
			"AcquiredSessionsCount",
			AcquiredSessionsCountView,
			"num_acquired_sessions",
			"1",
		},
		{
			"ReleasedSessionsCount",
			ReleasedSessionsCountView,
			"num_released_sessions",
			"1",
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			testSimpleMetric(t, test.view, test.measure, test.value)
		})
	}
}

func testSimpleMetric(t *testing.T, v *view.View, measure, value string) {
	DisableGfeLatencyAndHeaderMissingCountViews()
	te := testutil.NewTestExporter(v)
	defer te.Unregister()

	_, client, teardown := setupMockedTestServer(t)
	defer teardown()

	client.Single().ReadRow(context.Background(), "Users", Key{"alice"}, []string{"email"})

	// Wait for a while to see all exported metrics.
	waitErr := &Error{}
	waitFor(t, func() error {
		select {
		case stat := <-te.Stats:
			if len(stat.Rows) > 0 {
				return nil
			}
		}
		return waitErr
	})

	// Wait until we see data from the view.
	select {
	case stat := <-te.Stats:
		if len(stat.Rows) == 0 {
			t.Fatal("No metrics are exported")
		}
		if got, want := stat.View.Measure.Name(), statsPrefix+measure; got != want {
			t.Fatalf("Incorrect measure: got %v, want %v", got, want)
		}
		row := stat.Rows[0]
		m := getTagMap(row.Tags)
		checkCommonTags(t, m)
		var data string
		switch row.Data.(type) {
		default:
			data = fmt.Sprintf("%v", row.Data)
		case *view.CountData:
			data = fmt.Sprintf("%v", row.Data.(*view.CountData).Value)
		case *view.LastValueData:
			data = fmt.Sprintf("%v", row.Data.(*view.LastValueData).Value)
		}
		if got, want := data, value; got != want {
			t.Fatalf("Incorrect data: got %v, want %v", got, want)
		}
	case <-time.After(1 * time.Second):
		t.Fatal("no stats were exported before timeout")
	}
}

func TestOCStats_SessionPool_SessionsCount(t *testing.T) {
	DisableGfeLatencyAndHeaderMissingCountViews()
	te := testutil.NewTestExporter(SessionsCountView)
	defer te.Unregister()

	waitErr := &Error{}
	_, client, teardown := setupMockedTestServerWithConfig(t, ClientConfig{DisableNativeMetrics: true, SessionPoolConfig: DefaultSessionPoolConfig})
	defer teardown()
	// Wait for the session pool initialization to finish.
	expectedWrites := uint64(0)
	expectedReads := DefaultSessionPoolConfig.MinOpened - expectedWrites
	waitFor(t, func() error {
		client.idleSessions.mu.Lock()
		defer client.idleSessions.mu.Unlock()
		if client.idleSessions.numSessions == expectedReads {
			return nil
		}
		return waitErr
	})
	client.Single().ReadRow(context.Background(), "Users", Key{"alice"}, []string{"email"})

	expectedStats := 2
	if isMultiplexEnabled {
		// num_in_use_sessions is not exported when multiplexed sessions are enabled and only ReadOnly transactions are performed.
		expectedStats = 1
	}
	// Wait for a while to see all exported metrics.
	waitFor(t, func() error {
		select {
		case stat := <-te.Stats:
			if len(stat.Rows) >= expectedStats {
				return nil
			}
		}
		return waitErr
	})

	// Wait until we see data from the view.
	select {
	case stat := <-te.Stats:
		// There are 4 types for this metric, so we should see at least four
		// rows.
		if len(stat.Rows) < expectedStats {
			t.Fatal("No enough metrics are exported")
		}
		if got, want := stat.View.Measure.Name(), statsPrefix+"num_sessions_in_pool"; got != want {
			t.Fatalf("Incorrect measure: got %v, want %v", got, want)
		}
		for _, row := range stat.Rows {
			m := getTagMap(row.Tags)
			checkCommonTags(t, m)
			// view.AggregationData does not have a way to extract the value. So
			// we have to convert it to a string and then compare with expected
			// values.
			data := row.Data.(*view.LastValueData)
			got := fmt.Sprintf("%v", data.Value)
			var want string
			switch m[tagKeyType] {
			case "num_sessions":
				want = "100"
			case "num_in_use_sessions":
				want = "0"
			default:
				t.Fatalf("Incorrect type: %v", m[tagKeyType])
			}
			if got != want {
				t.Fatalf("Incorrect data: got %v, want %v", got, want)
			}
		}
	case <-time.After(1 * time.Second):
		t.Fatal("no stats were exported before timeout")
	}
}

func TestOCStats_SessionPool_GetSessionTimeoutsCount(t *testing.T) {
	DisableGfeLatencyAndHeaderMissingCountViews()
	te := testutil.NewTestExporter(GetSessionTimeoutsCountView)
	defer te.Unregister()

	server, client, teardown := setupMockedTestServerWithoutWaitingForMultiplexedSessionInit(t)
	defer teardown()

	server.TestSpanner.PutExecutionTime(stestutil.MethodBatchCreateSession,
		stestutil.SimulatedExecutionTime{
			MinimumExecutionTime: 2 * time.Millisecond,
		})
	server.TestSpanner.PutExecutionTime(stestutil.MethodCreateSession,
		stestutil.SimulatedExecutionTime{
			MinimumExecutionTime: 2 * time.Millisecond,
		})
	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Millisecond)
	defer cancel()
	client.Single().ReadRow(ctx, "Users", Key{"alice"}, []string{"email"})

	// Wait for a while to see all exported metrics.
	waitErr := &Error{}
	waitFor(t, func() error {
		select {
		case stat := <-te.Stats:
			if len(stat.Rows) > 0 {
				return nil
			}
		}
		return waitErr
	})

	// Wait until we see data from the view.
	select {
	case stat := <-te.Stats:
		if len(stat.Rows) == 0 {
			t.Fatal("No metrics are exported")
		}
		if got, want := stat.View.Measure.Name(), statsPrefix+"get_session_timeouts"; got != want {
			t.Fatalf("Incorrect measure: got %v, want %v", got, want)
		}
		row := stat.Rows[0]
		m := getTagMap(row.Tags)
		checkCommonTags(t, m)
		data := row.Data.(*view.CountData).Value
		if got, want := fmt.Sprintf("%v", data), "1"; got != want {
			t.Fatalf("Incorrect data: got %v, want %v", got, want)
		}
	case <-time.After(1 * time.Second):
		t.Fatal("no stats were exported before timeout")
	}
}

func TestOCStats_GFE_Latency(t *testing.T) {
	te := testutil.NewTestExporter([]*view.View{GFELatencyView, GFEHeaderMissingCountView}...)
	defer te.Unregister()

	setGFELatencyMetricsFlag(true)

	server, client, teardown := setupMockedTestServer(t)
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
	iter := client.Single().Read(context.Background(), "Users", AllKeys(), []string{"email"})
	for {
		_, err := iter.Next()
		if err == iterator.Done {
			break
		}
		if err != nil {
			t.Fatal(err.Error())
		}
	}

	waitErr := &Error{}
	waitFor(t, func() error {
		select {
		case stat := <-te.Stats:
			if len(stat.Rows) > 0 {
				return nil
			}
		}
		return waitErr
	})

	// Wait until we see data from the view.
	select {
	case stat := <-te.Stats:
		if len(stat.Rows) == 0 {
			t.Fatal("No metrics are exported")
		}
		if stat.View.Measure.Name() != statsPrefix+"gfe_latency" && stat.View.Measure.Name() != statsPrefix+"gfe_header_missing_count" {
			t.Fatalf("Incorrect measure: got %v, want %v", stat.View.Measure.Name(), statsPrefix+"gfe_header_missing_count or "+statsPrefix+"gfe_latency")
		}
		row := stat.Rows[0]
		m := getTagMap(row.Tags)
		checkCommonTags(t, m)
		var data string
		switch row.Data.(type) {
		default:
			data = fmt.Sprintf("%v", row.Data)
		case *view.CountData:
			data = fmt.Sprintf("%v", row.Data.(*view.CountData).Value)
		case *view.LastValueData:
			data = fmt.Sprintf("%v", row.Data.(*view.LastValueData).Value)
		case *view.DistributionData:
			data = fmt.Sprintf("%v", row.Data.(*view.DistributionData).Count)
		}
		if got, want := fmt.Sprintf("%v", data), "0"; got <= want {
			t.Fatalf("Incorrect data: got %v, wanted more than %v for metric %v", got, want, stat.View.Measure.Name())
		}
	case <-time.After(1 * time.Second):
		t.Fatal("no stats were exported before timeout")
	}

}
func getTagMap(tags []tag.Tag) map[tag.Key]string {
	m := make(map[tag.Key]string)
	for _, t := range tags {
		m[t.Key] = t.Value
	}
	return m
}

func checkCommonTags(t *testing.T, m map[tag.Key]string) {
	// We only check prefix because client ID increases if we create
	// multiple clients for the same database.
	if !strings.HasPrefix(m[tagKeyClientID], "client-") {
		t.Fatalf("Incorrect client ID: %v", m[tagKeyClientID])
	}
	if m[tagKeyInstance] != "[INSTANCE]" {
		t.Fatalf("Incorrect instance ID: %v", m[tagKeyInstance])
	}
	if m[tagKeyDatabase] != "[DATABASE]" {
		t.Fatalf("Incorrect database ID: %v", m[tagKeyDatabase])
	}
	if m[tagKeyLibVersion] != internal.Version {
		t.Fatalf("Incorrect library version: %v", m[tagKeyLibVersion])
	}
}

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

	"cloud.google.com/go/spanner/apiv1/spannerpb"
	"cloud.google.com/go/spanner/internal"
	stestutil "cloud.google.com/go/spanner/internal/testutil"
	"go.opencensus.io/stats/view"
	"go.opencensus.io/tag"
	"google.golang.org/api/iterator"
	structpb "google.golang.org/protobuf/types/known/structpb"
)

func TestOCStats_SessionPool_GetSessionTimeoutsCount(t *testing.T) {
	DisableGfeLatencyAndHeaderMissingCountViews()
	te := stestutil.NewTestExporter(GetSessionTimeoutsCountView)
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
	te := stestutil.NewTestExporter([]*view.View{GFELatencyView, GFEHeaderMissingCountView}...)
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

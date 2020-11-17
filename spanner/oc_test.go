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
	"math"
	"strings"
	"testing"
	"time"

	"cloud.google.com/go/internal/testutil"
	"cloud.google.com/go/internal/version"
	stestutil "cloud.google.com/go/spanner/internal/testutil"
	"go.opencensus.io/stats/view"
	"go.opencensus.io/tag"
)

// Check that stats are being exported.
func TestOCStats(t *testing.T) {
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
			"25",
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
			"1",
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
	te := testutil.NewTestExporter(SessionsCountView)
	defer te.Unregister()

	waitErr := &Error{}
	_, client, teardown := setupMockedTestServerWithConfig(t, ClientConfig{SessionPoolConfig: DefaultSessionPoolConfig})
	defer teardown()
	// Wait for the session pool initialization to finish.
	expectedWrites := uint64(math.Floor(float64(DefaultSessionPoolConfig.MinOpened) * DefaultSessionPoolConfig.WriteSessions))
	expectedReads := DefaultSessionPoolConfig.MinOpened - expectedWrites
	waitFor(t, func() error {
		client.idleSessions.mu.Lock()
		defer client.idleSessions.mu.Unlock()
		if client.idleSessions.numReads == expectedReads && client.idleSessions.numWrites == expectedWrites {
			return nil
		}
		return waitErr
	})
	client.Single().ReadRow(context.Background(), "Users", Key{"alice"}, []string{"email"})

	// Wait for a while to see all exported metrics.
	waitFor(t, func() error {
		select {
		case stat := <-te.Stats:
			if len(stat.Rows) >= 4 {
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
		if len(stat.Rows) < 4 {
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
			case "num_write_prepared_sessions":
				want = "20"
			case "num_read_sessions":
				want = "80"
			case "num_sessions_being_prepared":
				want = "0"
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
	te := testutil.NewTestExporter(GetSessionTimeoutsCountView)
	defer te.Unregister()

	server, client, teardown := setupMockedTestServer(t)
	defer teardown()

	server.TestSpanner.PutExecutionTime(stestutil.MethodBatchCreateSession,
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
	if m[tagKeyLibVersion] != version.Repo {
		t.Fatalf("Incorrect library version: %v", m[tagKeyLibVersion])
	}
}

// Copyright 2016 Google LLC
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

// TODO(jba): test that OnError is getting called appropriately.

package logadmin

import (
	"context"
	"flag"
	"log"
	"net/http"
	"net/url"
	"os"
	"strings"
	"testing"
	"time"

	"cloud.google.com/go/internal/testutil"
	"cloud.google.com/go/logging"
	logpb "cloud.google.com/go/logging/apiv2/loggingpb"
	ltesting "cloud.google.com/go/logging/internal/testing"
	"github.com/google/go-cmp/cmp/cmpopts"
	"google.golang.org/api/option"
	mrpb "google.golang.org/genproto/googleapis/api/monitoredres"
	audit "google.golang.org/genproto/googleapis/cloud/audit"
	logtypepb "google.golang.org/genproto/googleapis/logging/type"
	"google.golang.org/grpc"
	"google.golang.org/protobuf/types/known/anypb"
	durpb "google.golang.org/protobuf/types/known/durationpb"
	structpb "google.golang.org/protobuf/types/known/structpb"
	"google.golang.org/protobuf/types/known/timestamppb"
)

var (
	client        *Client
	testProjectID string
)

var (
	// If true, this test is using the production service, not a fake.
	integrationTest bool

	newClient func(ctx context.Context, projectID string) *Client
)

func TestMain(m *testing.M) {
	flag.Parse() // needed for testing.Short()
	ctx := context.Background()
	testProjectID = testutil.ProjID()
	if testProjectID == "" || testing.Short() {
		integrationTest = false
		if testProjectID != "" {
			log.Print("Integration tests skipped in short mode (using fake instead)")
		}
		testProjectID = "PROJECT_ID"
		addr, err := ltesting.NewServer()
		if err != nil {
			log.Fatalf("creating fake server: %v", err)
		}
		newClient = func(ctx context.Context, projectID string) *Client {
			conn, err := grpc.Dial(addr, grpc.WithInsecure(), grpc.WithBlock())
			if err != nil {
				log.Fatalf("dialing %q: %v", addr, err)
			}
			c, err := NewClient(ctx, projectID, option.WithGRPCConn(conn))
			if err != nil {
				log.Fatalf("creating client for fake at %q: %v", addr, err)
			}
			return c
		}
	} else {
		integrationTest = true
		ts := testutil.TokenSource(ctx, logging.AdminScope)
		if ts == nil {
			log.Fatal("The project key must be set. See CONTRIBUTING.md for details")
		}
		log.Printf("running integration tests with project %s", testProjectID)
		newClient = func(ctx context.Context, projectID string) *Client {
			c, err := NewClient(ctx, projectID, option.WithTokenSource(ts),
				option.WithGRPCDialOption(grpc.WithBlock()))
			if err != nil {
				log.Fatalf("creating prod client: %v", err)
			}
			return c
		}
	}
	client = newClient(ctx, testProjectID)
	initMetrics(ctx)
	cleanup := initSinks(ctx)
	exit := m.Run()
	cleanup()
	client.Close()
	os.Exit(exit)
}

// EntryIterator and DeleteLog are tested in the logging package.

func TestClientClose(t *testing.T) {
	c := newClient(context.Background(), testProjectID)
	if err := c.Close(); err != nil {
		t.Errorf("want got %v, want nil", err)
	}
}

func TestFromLogEntry(t *testing.T) {
	now := time.Now()
	res := &mrpb.MonitoredResource{Type: "global"}
	ts := timestamppb.New(now)
	logEntry := logpb.LogEntry{
		LogName:   "projects/PROJECT_ID/logs/LOG_ID",
		Resource:  res,
		Payload:   &logpb.LogEntry_TextPayload{TextPayload: "hello"},
		Timestamp: ts,
		Severity:  logtypepb.LogSeverity_INFO,
		InsertId:  "123",
		HttpRequest: &logtypepb.HttpRequest{
			RequestMethod:                  "GET",
			RequestUrl:                     "http:://example.com/path?q=1",
			RequestSize:                    100,
			Status:                         200,
			ResponseSize:                   25,
			Latency:                        &durpb.Duration{Seconds: 100},
			UserAgent:                      "user-agent",
			RemoteIp:                       "127.0.0.1",
			ServerIp:                       "127.0.0.1",
			Referer:                        "referer",
			CacheLookup:                    true,
			CacheHit:                       true,
			CacheValidatedWithOriginServer: true,
			CacheFillBytes:                 2048,
		},
		Labels: map[string]string{
			"a": "1",
			"b": "two",
			"c": "true",
		},
		SourceLocation: &logpb.LogEntrySourceLocation{
			File:     "some_file.go",
			Line:     1,
			Function: "someFunction",
		},
	}
	u, err := url.Parse("http:://example.com/path?q=1")
	if err != nil {
		t.Fatal(err)
	}
	want := &logging.Entry{
		LogName:   "projects/PROJECT_ID/logs/LOG_ID",
		Resource:  res,
		Timestamp: now.In(time.UTC),
		Severity:  logging.Info,
		Payload:   "hello",
		Labels: map[string]string{
			"a": "1",
			"b": "two",
			"c": "true",
		},
		InsertID: "123",
		HTTPRequest: &logging.HTTPRequest{
			Request: &http.Request{
				Method: "GET",
				URL:    u,
				Header: map[string][]string{
					"User-Agent": {"user-agent"},
					"Referer":    {"referer"},
				},
			},
			RequestSize:                    100,
			Status:                         200,
			ResponseSize:                   25,
			Latency:                        100 * time.Second,
			LocalIP:                        "127.0.0.1",
			RemoteIP:                       "127.0.0.1",
			CacheLookup:                    true,
			CacheHit:                       true,
			CacheValidatedWithOriginServer: true,
			CacheFillBytes:                 2048,
		},
		SourceLocation: &logpb.LogEntrySourceLocation{
			File:     "some_file.go",
			Line:     1,
			Function: "someFunction",
		},
	}
	got, err := fromLogEntry(&logEntry)
	if err != nil {
		t.Fatal(err)
	}
	if diff := testutil.Diff(got, want, cmpopts.IgnoreUnexported(http.Request{})); diff != "" {
		t.Errorf("FullEntry:\n%s", diff)
	}

	// Proto payload.
	alog := &audit.AuditLog{
		ServiceName:  "svc",
		MethodName:   "method",
		ResourceName: "shelves/S/books/B",
	}
	any, err := anypb.New(alog)
	if err != nil {
		t.Fatal(err)
	}
	logEntry = logpb.LogEntry{
		LogName:   "projects/PROJECT_ID/logs/LOG_ID",
		Resource:  res,
		Timestamp: ts,
		Payload:   &logpb.LogEntry_ProtoPayload{ProtoPayload: any},
	}
	got, err = fromLogEntry(&logEntry)
	if err != nil {
		t.Fatal(err)
	}
	if !ltesting.PayloadEqual(got.Payload, alog) {
		t.Errorf("got %+v, want %+v", got.Payload, alog)
	}

	// JSON payload.
	jstruct := &structpb.Struct{Fields: map[string]*structpb.Value{
		"f": {Kind: &structpb.Value_NumberValue{NumberValue: 3.1}},
	}}
	logEntry = logpb.LogEntry{
		LogName:   "projects/PROJECT_ID/logs/LOG_ID",
		Resource:  res,
		Timestamp: ts,
		Payload:   &logpb.LogEntry_JsonPayload{JsonPayload: jstruct},
	}
	got, err = fromLogEntry(&logEntry)
	if err != nil {
		t.Fatal(err)
	}
	if !ltesting.PayloadEqual(got.Payload, jstruct) {
		t.Errorf("got %+v, want %+v", got.Payload, jstruct)
	}

	// No payload.
	logEntry = logpb.LogEntry{
		LogName:   "projects/PROJECT_ID/logs/LOG_ID",
		Resource:  res,
		Timestamp: ts,
	}
	got, err = fromLogEntry(&logEntry)
	if err != nil {
		t.Fatal(err)
	}
	if !ltesting.PayloadEqual(got.Payload, nil) {
		t.Errorf("got %+v, want %+v", got.Payload, nil)
	}
}

func TestListLogEntriesRequestDefaults(t *testing.T) {
	const timeFilterPrefix = "timestamp >= "

	got := listLogEntriesRequest("projects/PROJECT_ID", nil)

	// parse time from filter
	if len(got.Filter) < len(timeFilterPrefix) {
		t.Errorf("got %v; want len(%v) start with '%v'", got, got.Filter, timeFilterPrefix)
	}
	filterTime, err := time.Parse(time.RFC3339, strings.Trim(got.Filter[len(timeFilterPrefix):], "\""))
	if err != nil {
		t.Errorf("got %v; want %v in RFC3339", err, got.Filter)
	}
	timeDiff := time.Now().UTC().Sub(filterTime)

	// Default is client's project ID, 24 hour lookback, and no orderBy.
	if !testutil.Equal(got.ResourceNames, []string{"projects/PROJECT_ID"}) || got.OrderBy != "" || timeDiff.Hours() < 24 {
		t.Errorf("got %v; want resource_names:\"projects/PROJECT_ID\" filter: %v - 24 hours order_by:\"\"", got, filterTime)
	}
}

func TestListLogEntriesRequest(t *testing.T) {
	for _, test := range []struct {
		opts          []EntriesOption
		resourceNames []string
		filterPrefix  string
		orderBy       string
		pageSize      int32
	}{
		// Timestamp default does not override user's filter
		{
			// default resource name and timestamp filter
			opts: []EntriesOption{
				NewestFirst(),
				Filter(`timestamp > "2020-10-30T15:39:09Z"`),
			},
			resourceNames: []string{"projects/PROJECT_ID"},
			filterPrefix:  `timestamp > "2020-10-30T15:39:09Z"`,
			orderBy:       "timestamp desc",
		},
		{
			// default resource name and user's filter
			opts: []EntriesOption{
				NewestFirst(),
				Filter("f"),
			},
			resourceNames: []string{"projects/PROJECT_ID"},
			filterPrefix:  "f AND timestamp >= \"",
			orderBy:       "timestamp desc",
		},
		{
			// user's project id and default timestamp filter
			opts: []EntriesOption{
				ProjectIDs([]string{"foo"}),
			},
			resourceNames: []string{"projects/foo"},
			filterPrefix:  "timestamp >= \"",
			orderBy:       "",
		},
		{
			// user's resource name and default timestamp filter
			opts: []EntriesOption{
				ResourceNames([]string{"folders/F", "organizations/O"}),
			},
			resourceNames: []string{"folders/F", "organizations/O"},
			filterPrefix:  "timestamp >= \"",
			orderBy:       "",
		},
		{
			// user's project id and user's options
			opts: []EntriesOption{
				NewestFirst(),
				Filter("f"),
				ProjectIDs([]string{"foo"}),
			},
			resourceNames: []string{"projects/foo"},
			filterPrefix:  "f AND timestamp >= \"",
			orderBy:       "timestamp desc",
		},
		{
			// user's project id with multiple filter options
			opts: []EntriesOption{
				NewestFirst(),
				Filter("no"),
				ProjectIDs([]string{"foo"}),
				Filter("f"),
			},
			resourceNames: []string{"projects/foo"},
			filterPrefix:  "f AND timestamp >= \"",
			orderBy:       "timestamp desc",
		},
		{
			// user's project id and custom page size
			opts: []EntriesOption{
				ProjectIDs([]string{"foo"}),
				PageSize(100),
			},
			resourceNames: []string{"projects/foo"},
			filterPrefix:  "timestamp >= \"",
			pageSize:      100,
		},
	} {
		got := listLogEntriesRequest("projects/PROJECT_ID", test.opts)
		want := &logpb.ListLogEntriesRequest{
			ResourceNames: test.resourceNames,
			Filter:        test.filterPrefix,
			OrderBy:       test.orderBy,
			PageSize:      test.pageSize,
		}
		if !testutil.Equal(got.ResourceNames, want.ResourceNames) || !strings.HasPrefix(got.Filter, want.Filter) || got.OrderBy != want.OrderBy || got.PageSize != want.PageSize {
			t.Errorf("got: %v; want %v (mind wanted Filter is prefix)", got, want)
		}
	}
}

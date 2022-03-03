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
	ltesting "cloud.google.com/go/logging/internal/testing"
	"github.com/golang/protobuf/ptypes"
	durpb "github.com/golang/protobuf/ptypes/duration"
	structpb "github.com/golang/protobuf/ptypes/struct"
	"github.com/google/go-cmp/cmp/cmpopts"
	"google.golang.org/api/option"
	mrpb "google.golang.org/genproto/googleapis/api/monitoredres"
	audit "google.golang.org/genproto/googleapis/cloud/audit"
	logtypepb "google.golang.org/genproto/googleapis/logging/type"
	logpb "google.golang.org/genproto/googleapis/logging/v2"
	"google.golang.org/grpc"
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
	ts, err := ptypes.TimestampProto(now)
	if err != nil {
		t.Fatal(err)
	}
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
	any, err := ptypes.MarshalAny(alog)
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
	}{
		// Timestamp default does not override user's filter
		{[]EntriesOption{NewestFirst(), Filter(`timestamp > "2020-10-30T15:39:09Z"`)},
			[]string{"projects/PROJECT_ID"}, `timestamp > "2020-10-30T15:39:09Z"`, "timestamp desc"},
		{[]EntriesOption{NewestFirst(), Filter("f")},
			[]string{"projects/PROJECT_ID"}, "f AND timestamp >= \"", "timestamp desc"},
		{[]EntriesOption{ProjectIDs([]string{"foo"})},
			[]string{"projects/foo"}, "timestamp >= \"", ""},
		{[]EntriesOption{ResourceNames([]string{"folders/F", "organizations/O"})},
			[]string{"folders/F", "organizations/O"}, "timestamp >= \"", ""},
		{[]EntriesOption{NewestFirst(), Filter("f"), ProjectIDs([]string{"foo"})},
			[]string{"projects/foo"}, "f AND timestamp >= \"", "timestamp desc"},
		{[]EntriesOption{NewestFirst(), Filter("f"), ProjectIDs([]string{"foo"})},
			[]string{"projects/foo"}, "f AND timestamp >= \"", "timestamp desc"},
		// If there are repeats, last one wins.
		{[]EntriesOption{NewestFirst(), Filter("no"), ProjectIDs([]string{"foo"}), Filter("f")},
			[]string{"projects/foo"}, "f AND timestamp >= \"", "timestamp desc"},
	} {
		got := listLogEntriesRequest("projects/PROJECT_ID", test.opts)
		want := &logpb.ListLogEntriesRequest{
			ResourceNames: test.resourceNames,
			Filter:        test.filterPrefix,
			OrderBy:       test.orderBy,
		}
		if !testutil.Equal(got.ResourceNames, want.ResourceNames) || !strings.HasPrefix(got.Filter, want.Filter) || got.OrderBy != want.OrderBy {
			t.Errorf("got: %v; want %v (mind wanted Filter is prefix)", got, want)
		}
	}
}

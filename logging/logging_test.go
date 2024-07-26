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

package logging_test

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"log"
	"math/rand"
	"net"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	cinternal "cloud.google.com/go/internal"
	"cloud.google.com/go/internal/testutil"
	"cloud.google.com/go/internal/uid"
	"cloud.google.com/go/logging"
	logpb "cloud.google.com/go/logging/apiv2/loggingpb"
	"cloud.google.com/go/logging/internal"
	ltesting "cloud.google.com/go/logging/internal/testing"
	"cloud.google.com/go/logging/logadmin"
	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"
	gax "github.com/googleapis/gax-go/v2"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/trace"
	"go.opentelemetry.io/otel/trace/noop"
	"golang.org/x/oauth2"
	"google.golang.org/api/iterator"
	"google.golang.org/api/option"
	mrpb "google.golang.org/genproto/googleapis/api/monitoredres"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/anypb"
)

const testLogIDPrefix = "GO-LOGGING-CLIENT/TEST-LOG"

var (
	client        *logging.Client
	aclient       *logadmin.Client
	testProjectID string
	testLogID     string
	testFilter    string
	errorc        chan error
	ctx           context.Context

	// Adjust the fields of a FullEntry received from the production service
	// before comparing it with the expected result. We can't correctly
	// compare certain fields, like times or server-generated IDs.
	clean func(*logging.Entry)

	// Create a new client with the given project ID.
	newClients func(ctx context.Context, projectID string) (*logging.Client, *logadmin.Client)

	uids = uid.NewSpace(testLogIDPrefix, nil)

	// If true, this test is using the production service, not a fake.
	integrationTest bool
)

func testNow() time.Time {
	return time.Unix(1000, 0)
}

func TestMain(m *testing.M) {
	flag.Parse() // needed for testing.Short()

	// disable ingesting instrumentation log entry
	internal.InstrumentOnce.Do(func() {})

	ctx = context.Background()
	testProjectID = testutil.ProjID()
	errorc = make(chan error, 100)
	if testProjectID == "" || testing.Short() {
		integrationTest = false
		if testProjectID != "" {
			log.Print("Integration tests skipped in short mode (using fake instead)")
		}
		testProjectID = ltesting.ValidProjectID
		clean = func(e *logging.Entry) {
			// Remove the insert ID for consistency with the integration test.
			e.InsertID = ""
		}

		addr, err := ltesting.NewServer()
		if err != nil {
			log.Fatalf("creating fake server: %v", err)
		}
		logging.SetNow(testNow)

		newClients = func(ctx context.Context, parent string) (*logging.Client, *logadmin.Client) {
			conn, err := grpc.Dial(addr, grpc.WithInsecure())
			if err != nil {
				log.Fatalf("dialing %q: %v", addr, err)
			}
			c, err := logging.NewClient(ctx, parent, option.WithGRPCConn(conn))
			if err != nil {
				log.Fatalf("creating client for fake at %q: %v", addr, err)
			}
			ac, err := logadmin.NewClient(ctx, parent, option.WithGRPCConn(conn))
			if err != nil {
				log.Fatalf("creating client for fake at %q: %v", addr, err)
			}
			return c, ac
		}

	} else {
		integrationTest = true
		clean = func(e *logging.Entry) {
			// We cannot compare timestamps, so set them to the test time.
			// Also, remove the insert ID added by the service.
			e.Timestamp = testNow().UTC()
			e.InsertID = ""
		}
		ts := testutil.TokenSource(ctx, logging.AdminScope)
		if ts == nil {
			log.Fatal("The project key must be set. See CONTRIBUTING.md for details")
		}
		log.Printf("running integration tests with project %s", testProjectID)
		newClients = func(ctx context.Context, parent string) (*logging.Client, *logadmin.Client) {
			c, err := logging.NewClient(ctx, parent, option.WithTokenSource(ts))
			if err != nil {
				log.Fatalf("creating prod client: %v", err)
			}
			ac, err := logadmin.NewClient(ctx, parent, option.WithTokenSource(ts))
			if err != nil {
				log.Fatalf("creating prod client: %v", err)
			}
			return c, ac
		}

	}
	client, aclient = newClients(ctx, testProjectID)
	client.OnError = func(e error) { errorc <- e }

	exit := m.Run()
	os.Exit(exit)
}

func initLogs() {
	testLogID = uids.New()
	hourAgo := time.Now().Add(-1 * time.Hour).UTC()
	testFilter = fmt.Sprintf(`logName = "projects/%s/logs/%s" AND
timestamp >= "%s"`,
		testProjectID, strings.Replace(testLogID, "/", "%2F", -1), hourAgo.Format(time.RFC3339))
}

func TestLogSync(t *testing.T) {
	initLogs() // Generate new testLogID
	ctx := context.Background()
	lg := client.Logger(testLogID)
	err := lg.LogSync(ctx, logging.Entry{Payload: "hello"})
	if err != nil {
		t.Fatal(err)
	}
	err = lg.LogSync(ctx, logging.Entry{Payload: "goodbye"})
	if err != nil {
		t.Fatal(err)
	}
	// Allow overriding the MonitoredResource.
	err = lg.LogSync(ctx, logging.Entry{Payload: "mr", Resource: &mrpb.MonitoredResource{Type: "global"}})
	if err != nil {
		t.Fatal(err)
	}

	want := []*logging.Entry{
		entryForTesting("hello"),
		entryForTesting("goodbye"),
		entryForTesting("mr"),
	}
	var got []*logging.Entry
	ok := waitFor(func() bool {
		got, err = allTestLogEntries(ctx)
		if err != nil {
			t.Log("fetching log entries: ", err)
			return false
		}
		return len(got) == len(want)
	})
	if !ok {
		t.Fatalf("timed out; got: %d, want: %d\n", len(got), len(want))
	}
	if msg, ok := compareEntries(got, want); !ok {
		t.Error(msg)
	}
}

func TestLogAndEntries(t *testing.T) {
	initLogs() // Generate new testLogID
	ctx := context.Background()
	payloads := []string{"p1", "p2", "p3", "p4", "p5"}
	lg := client.Logger(testLogID)
	for _, p := range payloads {
		// Use the insert ID to guarantee iteration order.
		lg.Log(logging.Entry{Payload: p, InsertID: p})
	}
	if err := lg.Flush(); err != nil {
		t.Fatal(err)
	}
	var want []*logging.Entry
	for _, p := range payloads {
		want = append(want, entryForTesting(p))
	}
	var got []*logging.Entry
	ok := waitFor(func() bool {
		var err error
		got, err = allTestLogEntries(ctx)
		if err != nil {
			t.Log("fetching log entries: ", err)
			return false
		}
		return len(got) == len(want)
	})
	if !ok {
		t.Fatalf("timed out; got: %d, want: %d\n", len(got), len(want))
	}
	if msg, ok := compareEntries(got, want); !ok {
		t.Error(msg)
	}
}

func TestLogInvalidUtf8(t *testing.T) {
	lg := client.Logger(testLogID)
	msg := fmt.Sprintf("\x6c\x6f\x67\xe5")
	lg.Log(logging.Entry{
		Payload:   msg,
		Timestamp: time.Now(),
	})
	err := lg.Flush()
	s, _ := status.FromError(err)
	if !strings.Contains(s.Message(), "string field contains invalid UTF-8") {
		t.Fatalf("got an incorrect error: %v", err)
	}
}

func TestContextFunc(t *testing.T) {
	initLogs()
	var contextFuncCalls, cleanupCalls int32 //atomic

	lg := client.Logger(testLogID, logging.ContextFunc(func() (context.Context, func()) {
		atomic.AddInt32(&contextFuncCalls, 1)
		return context.Background(), func() { atomic.AddInt32(&cleanupCalls, 1) }
	}))
	lg.Log(logging.Entry{Payload: "p"})
	if err := lg.Flush(); err != nil {
		t.Fatal(err)
	}
	got1 := atomic.LoadInt32(&contextFuncCalls)
	got2 := atomic.LoadInt32(&cleanupCalls)
	if got1 != 1 || got1 != got2 {
		t.Errorf("got %d calls to context func, %d calls to cleanup func; want 1, 1", got1, got2)
	}
}

func TestToLogEntry(t *testing.T) {
	u := &url.URL{Scheme: "http"}
	tests := []struct {
		name      string
		in        logging.Entry
		want      *logpb.LogEntry
		wantError error
	}{
		{
			name: "BlankLogEntry",
			in:   logging.Entry{},
			want: &logpb.LogEntry{},
		}, {
			name: "Already set Trace",
			in:   logging.Entry{Trace: "t1"},
			want: &logpb.LogEntry{Trace: "t1"},
		}, {
			name: "No X-Trace-Context header",
			in: logging.Entry{
				HTTPRequest: &logging.HTTPRequest{
					Request: &http.Request{URL: u, Header: http.Header{"foo": {"bar"}}},
				},
			},
			want: &logpb.LogEntry{},
		}, {
			name: "X-Trace-Context header with all fields",
			in: logging.Entry{
				TraceSampled: false,
				HTTPRequest: &logging.HTTPRequest{
					Request: &http.Request{
						URL:    u,
						Header: http.Header{"X-Cloud-Trace-Context": {"105445aa7843bc8bf206b120001000/000000000000004a;o=1"}},
					},
				},
			},
			want: &logpb.LogEntry{
				Trace:        "projects/P/traces/105445aa7843bc8bf206b120001000",
				SpanId:       "000000000000004a",
				TraceSampled: true,
			},
		}, {
			name: "X-Trace-Context header with all fields; TraceSampled explicitly set",
			in: logging.Entry{
				TraceSampled: true,
				HTTPRequest: &logging.HTTPRequest{
					Request: &http.Request{
						URL:    u,
						Header: http.Header{"X-Cloud-Trace-Context": {"105445aa7843bc8bf206b120001000/000000000000004a;o=0"}},
					},
				},
			},
			want: &logpb.LogEntry{
				Trace:        "projects/P/traces/105445aa7843bc8bf206b120001000",
				SpanId:       "000000000000004a",
				TraceSampled: true,
			},
		}, {
			name: "X-Trace-Context header with all fields; TraceSampled from Header",
			in: logging.Entry{
				HTTPRequest: &logging.HTTPRequest{
					Request: &http.Request{
						URL:    u,
						Header: http.Header{"X-Cloud-Trace-Context": {"105445aa7843bc8bf206b120001000/000000000000004a;o=1"}},
					},
				},
			},
			want: &logpb.LogEntry{
				Trace:        "projects/P/traces/105445aa7843bc8bf206b120001000",
				SpanId:       "000000000000004a",
				TraceSampled: true,
			},
		}, {
			name: "X-Trace-Context header with blank trace",
			in: logging.Entry{
				HTTPRequest: &logging.HTTPRequest{
					Request: &http.Request{
						URL:    u,
						Header: http.Header{"X-Cloud-Trace-Context": {"/0;o=1"}},
					},
				},
			},
			want: &logpb.LogEntry{},
		}, {
			name: "X-Trace-Context header with blank span",
			in: logging.Entry{
				HTTPRequest: &logging.HTTPRequest{
					Request: &http.Request{
						URL:    u,
						Header: http.Header{"X-Cloud-Trace-Context": {"105445aa7843bc8bf206b120001000/;o=0"}},
					},
				},
			},
			want: &logpb.LogEntry{
				Trace: "projects/P/traces/105445aa7843bc8bf206b120001000",
			},
		}, {
			name: "X-Trace-Context header with missing traceSampled aka ?o=*",
			in: logging.Entry{
				HTTPRequest: &logging.HTTPRequest{
					Request: &http.Request{
						URL:    u,
						Header: http.Header{"X-Cloud-Trace-Context": {"105445aa7843bc8bf206b120001000/0"}},
					},
				},
			},
			want: &logpb.LogEntry{
				Trace: "projects/P/traces/105445aa7843bc8bf206b120001000",
			},
		}, {
			name: "X-Trace-Context header with all blank fields",
			in: logging.Entry{
				HTTPRequest: &logging.HTTPRequest{
					Request: &http.Request{
						URL:    u,
						Header: http.Header{"X-Cloud-Trace-Context": {""}},
					},
				},
			},
			want: &logpb.LogEntry{},
		}, {
			name: "Invalid X-Trace-Context header but already set TraceID",
			in: logging.Entry{
				HTTPRequest: &logging.HTTPRequest{
					Request: &http.Request{
						URL:    u,
						Header: http.Header{"X-Cloud-Trace-Context": {"t3"}},
					},
				},
				Trace: "t4",
			},
			want: &logpb.LogEntry{
				Trace: "t4",
			},
		}, {
			name: "Already set TraceID and SpanID",
			in:   logging.Entry{Trace: "t1", SpanID: "007"},
			want: &logpb.LogEntry{
				Trace:  "t1",
				SpanId: "007",
			},
		}, {
			name: "Empty request produces an error",
			in: logging.Entry{
				HTTPRequest: &logging.HTTPRequest{
					RequestSize: 128,
				},
			},
			wantError: errors.New("logging: HTTPRequest must have a non-nil Request"),
		},
		{
			name: "Traceparent header with entry fields unset",
			in: logging.Entry{
				HTTPRequest: &logging.HTTPRequest{
					Request: &http.Request{
						URL:    u,
						Header: http.Header{"Traceparent": {"00-105445aa7843bc8bf206b12000100012-000000000000004a-01"}},
					},
				},
			},
			want: &logpb.LogEntry{
				Trace:  "projects/P/traces/105445aa7843bc8bf206b12000100012",
				SpanId: "000000000000004a",
			},
		},
		{
			name: "traceparent header with preset sampled field",
			in: logging.Entry{
				TraceSampled: true,
				HTTPRequest: &logging.HTTPRequest{
					Request: &http.Request{
						URL:    u,
						Header: http.Header{"Traceparent": {"00-105445aa7843bc8bf206b12000100012-000000000000004a-00"}},
					},
				},
			},
			want: &logpb.LogEntry{
				Trace:        "projects/P/traces/105445aa7843bc8bf206b12000100012",
				SpanId:       "000000000000004a",
				TraceSampled: true,
			},
		},
		{
			name: "Traceparent header together with x-trace-context header",
			in: logging.Entry{
				HTTPRequest: &logging.HTTPRequest{
					Request: &http.Request{
						URL: u,
						Header: http.Header{
							"X-Cloud-Trace-Context": {"105445aa7843bc8bf206b120000000/0000000000000bbb;o=1"},
							"Traceparent":           {"00-105445aa7843bc8bf206b1200010aaaa-0000000000000aaa-00"}},
					},
				},
			},
			want: &logpb.LogEntry{
				Trace:  "projects/P/traces/105445aa7843bc8bf206b1200010aaaa",
				SpanId: "0000000000000aaa",
			},
		},
		{
			name: "Traceparent header invalid protocol",
			in: logging.Entry{
				HTTPRequest: &logging.HTTPRequest{
					Request: &http.Request{
						URL:    u,
						Header: http.Header{"Traceparent": {"01-105445aa7843bc8bf206b12000100012-000000000000004a-00"}},
					},
				},
			},
			want: &logpb.LogEntry{},
		},
		{
			name: "Traceparent header short trace field",
			in: logging.Entry{
				HTTPRequest: &logging.HTTPRequest{
					Request: &http.Request{
						URL:    u,
						Header: http.Header{"Traceparent": {"00-12345678901234567890-000000000000004a-00"}},
					},
				},
			},
			want: &logpb.LogEntry{},
		},
		{
			name: "Traceparent header long trace field",
			in: logging.Entry{
				HTTPRequest: &logging.HTTPRequest{
					Request: &http.Request{
						URL:    u,
						Header: http.Header{"Traceparent": {"00-1234567890123456789012345678901234567890-000000000000004a-00"}},
					},
				},
			},
			want: &logpb.LogEntry{},
		},
		{
			name: "Traceparent header invalid trace field",
			in: logging.Entry{
				HTTPRequest: &logging.HTTPRequest{
					Request: &http.Request{
						URL:    u,
						Header: http.Header{"Traceparent": {"00-123456789012345678901234567890xx-000000000000004a-00"}},
					},
				},
			},
			want: &logpb.LogEntry{},
		},
		{
			name: "Traceparent header trace field all 0s",
			in: logging.Entry{
				HTTPRequest: &logging.HTTPRequest{
					Request: &http.Request{
						URL:    u,
						Header: http.Header{"Traceparent": {"00-00000000000000000000000000000000-000000000000004a-00"}},
					},
				},
			},
			want: &logpb.LogEntry{},
		},
		{
			name: "Traceparent header short span field",
			in: logging.Entry{
				HTTPRequest: &logging.HTTPRequest{
					Request: &http.Request{
						URL:    u,
						Header: http.Header{"Traceparent": {"00-12345678901234567890123456789012-123456789012345-00"}},
					},
				},
			},
			want: &logpb.LogEntry{},
		},
		{
			name: "Traceparent header long span field",
			in: logging.Entry{
				HTTPRequest: &logging.HTTPRequest{
					Request: &http.Request{
						URL:    u,
						Header: http.Header{"Traceparent": {"00-12345678901234567890123456789012-12345678901234567890-00"}},
					},
				},
			},
			want: &logpb.LogEntry{},
		},
		{
			name: "Traceparent header invalid span field",
			in: logging.Entry{
				HTTPRequest: &logging.HTTPRequest{
					Request: &http.Request{
						URL:    u,
						Header: http.Header{"Traceparent": {"00-12345678901234567890123456789012-abcdefghijklmnop-00"}},
					},
				},
			},
			want: &logpb.LogEntry{},
		},
		{
			name: "Traceparent header span field all 0s",
			in: logging.Entry{
				HTTPRequest: &logging.HTTPRequest{
					Request: &http.Request{
						URL:    u,
						Header: http.Header{"Traceparent": {"00-12345678901234567890123456789012-0000000000000000-00"}},
					},
				},
			},
			want: &logpb.LogEntry{},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			e, err := logging.ToLogEntry(test.in, "projects/P")
			if err != nil && test.wantError == nil {
				t.Fatalf("Unexpected error: %+v: %v", test.in, err)
			}
			if err == nil && test.wantError != nil {
				t.Fatalf("Error is expected: %+v: %v", test.in, test.wantError)
			}
			if test.wantError != nil {
				return
			}
			if got := e.Trace; got != test.want.Trace {
				t.Errorf("TraceId: %+v: got %q, want %q", test.in, got, test.want.Trace)
			}
			if got := e.SpanId; got != test.want.SpanId {
				t.Errorf("SpanId: %+v: got %q, want %q", test.in, got, test.want.SpanId)
			}
			if got := e.TraceSampled; got != test.want.TraceSampled {
				t.Errorf("TraceSampled: %+v: got %t, want %t", test.in, got, test.want.TraceSampled)
			}
		})
	}
}

func TestToLogEntryOTelIntegration(t *testing.T) {
	// Some slight modifications need to be done for testing ToLogEntry
	// for the OpenTelemetry integration, so they are in a separate function.
	u := &url.URL{Scheme: "http"}
	tests := []struct {
		name string
		in   logging.Entry
		want *logpb.LogEntry // if want is nil, pull wants from spanContext
	}{
		{
			name: "Using OpenTelemetry with a valid span",
			in: logging.Entry{
				HTTPRequest: &logging.HTTPRequest{
					Request: &http.Request{
						URL: u,
					},
				},
			},
		},
		{
			name: "Using OpenTelemetry only with a valid span + valid traceparent headers (precedence test)",
			in: logging.Entry{
				HTTPRequest: &logging.HTTPRequest{
					Request: &http.Request{
						URL: u,
						Header: http.Header{
							"Traceparent": {"00-105445aa7843bc8bf206b12000100012-000000000000004a-01"},
						},
					},
				},
			},
		},
		{
			name: "Using OpenTelemetry only with a valid span + valid XCTC headers (precedence test)",
			in: logging.Entry{
				HTTPRequest: &logging.HTTPRequest{
					Request: &http.Request{
						URL: u,
						Header: http.Header{
							"X-Cloud-Trace-Context": {"105445aa7843bc8bf206b120000000/0000000000000bbb;o=1"},
						},
					},
				},
			},
		},
		{
			name: "Using OpenTelemetry with a valid span + trace info set in Entry object",
			in: logging.Entry{
				HTTPRequest: &logging.HTTPRequest{
					Request: &http.Request{
						URL: u,
					},
				},
				Trace:        "abc",
				SpanID:       "def",
				TraceSampled: false,
			},
			want: &logpb.LogEntry{
				Trace:        "abc",
				SpanId:       "def",
				TraceSampled: false,
			},
		},
		{
			name: "Using OpenTelemetry without a request",
			in:   logging.Entry{},
			want: &logpb.LogEntry{},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			var span trace.Span
			ctx := context.Background()

			// Set up an OTel SDK tracer if integration test, mock noop tracer if not.
			if integrationTest {
				tracerProvider := sdktrace.NewTracerProvider()
				defer tracerProvider.Shutdown(ctx)

				ctx, span = tracerProvider.Tracer("integration-test-tracer").Start(ctx, "test span")
				defer span.End()
			} else {
				otelTraceID, _ := trace.TraceIDFromHex(strings.Repeat("a", 32))
				otelSpanID, _ := trace.SpanIDFromHex(strings.Repeat("f", 16))
				otelTraceFlags := trace.FlagsSampled // tracesampled = true
				mockSpanContext := trace.NewSpanContext(trace.SpanContextConfig{
					TraceID:    otelTraceID,
					SpanID:     otelSpanID,
					TraceFlags: otelTraceFlags,
				})
				ctx = trace.ContextWithSpanContext(ctx, mockSpanContext)
				ctx, span = noop.NewTracerProvider().Tracer("test tracer").Start(ctx, "test span")
				defer span.End()
			}

			if test.in.HTTPRequest != nil && test.in.HTTPRequest.Request != nil {
				test.in.HTTPRequest.Request = test.in.HTTPRequest.Request.WithContext(ctx)
			}
			spanContext := trace.SpanContextFromContext(ctx)

			// if want is nil, pull wants from spanContext
			if test.want == nil {
				test.want = &logpb.LogEntry{
					Trace:        "projects/P/traces/" + spanContext.TraceID().String(),
					SpanId:       spanContext.SpanID().String(),
					TraceSampled: spanContext.TraceFlags().IsSampled(),
				}
			}

			e, err := logging.ToLogEntry(test.in, "projects/P")
			if err != nil {
				t.Fatalf("Unexpected error: %+v: %v", test.in, err)
			}
			if got := e.Trace; got != test.want.Trace {
				t.Errorf("TraceId: %+v: SpanContext: %+v: got %q, want %q", test.in, spanContext, got, test.want.Trace)
			}
			if got := e.SpanId; got != test.want.SpanId {
				t.Errorf("SpanId: %+v: SpanContext: %+v: got %q, want %q", test.in, spanContext, got, test.want.SpanId)
			}
			if got := e.TraceSampled; got != test.want.TraceSampled {
				t.Errorf("TraceSampled: %+v: SpanContext: %+v: got %t, want %t", test.in, spanContext, got, test.want.TraceSampled)
			}
		})
	}
}

// compareEntries compares most fields list of Entries against expected. compareEntries does not compare:
//   - HTTPRequest
//   - Operation
//   - Resource
//   - SourceLocation
func compareEntries(got, want []*logging.Entry) (string, bool) {
	if len(got) != len(want) {
		return fmt.Sprintf("got %d entries, want %d", len(got), len(want)), false
	}
	for i := range got {
		if !compareEntry(got[i], want[i]) {
			return fmt.Sprintf("#%d:\ngot  %+v\nwant %+v", i, got[i], want[i]), false
		}
	}
	return "", true
}

func compareEntry(got, want *logging.Entry) bool {
	if got.Timestamp.Unix() != want.Timestamp.Unix() {
		return false
	}

	if got.Severity != want.Severity {
		return false
	}

	if !ltesting.PayloadEqual(got.Payload, want.Payload) {
		return false
	}
	if !testutil.Equal(got.Labels, want.Labels) {
		return false
	}

	if got.InsertID != want.InsertID {
		return false
	}

	if got.LogName != want.LogName {
		return false
	}

	return true
}

func entryForTesting(payload interface{}) *logging.Entry {
	return &logging.Entry{
		Timestamp: testNow().UTC(),
		Payload:   payload,
		LogName:   "projects/" + testProjectID + "/logs/" + testLogID,
		Resource:  &mrpb.MonitoredResource{Type: "global", Labels: map[string]string{"project_id": testProjectID}},
	}
}

// allTestLogEntries should be called sparingly. It takes ~10s to get logs, even with indexed filters.
func allTestLogEntries(ctx context.Context) ([]*logging.Entry, error) {
	return allEntries(ctx, aclient, testFilter)
}

func allEntries(ctx context.Context, aclient *logadmin.Client, filter string) ([]*logging.Entry, error) {
	var es []*logging.Entry
	it := aclient.Entries(ctx, logadmin.Filter(filter))
	for {
		e, err := cleanNext(it)
		switch err {
		case nil:
			es = append(es, e)
		case iterator.Done:
			return es, nil
		default:
			return nil, err
		}
	}
}

func cleanNext(it *logadmin.EntryIterator) (*logging.Entry, error) {
	e, err := it.Next()
	if err != nil {
		return nil, err
	}
	clean(e)
	return e, nil
}

func TestStandardLogger(t *testing.T) {
	initLogs() // Generate new testLogID
	ctx := context.Background()
	lg := client.Logger(testLogID)
	slg := lg.StandardLogger(logging.Info)

	if slg != lg.StandardLogger(logging.Info) {
		t.Error("There should be only one standard logger at each severity.")
	}
	if slg == lg.StandardLogger(logging.Debug) {
		t.Error("There should be a different standard logger for each severity.")
	}

	slg.Print("info")
	if err := lg.Flush(); err != nil {
		t.Fatal(err)
	}
	var got []*logging.Entry
	ok := waitFor(func() bool {
		var err error
		got, err = allTestLogEntries(ctx)
		if err != nil {
			t.Log("fetching log entries: ", err)
			return false
		}
		return len(got) == 1
	})
	if !ok {
		t.Fatalf("timed out; got: %d, want: %d\n", len(got), 1)
	}
	if len(got) != 1 {
		t.Fatalf("expected non-nil request with one entry; got:\n%+v", got)
	}
	if got, want := got[0].Payload.(string), "info\n"; got != want {
		t.Errorf("payload: got %q, want %q", got, want)
	}
	if got, want := logging.Severity(got[0].Severity), logging.Info; got != want {
		t.Errorf("severity: got %s, want %s", got, want)
	}
}

func TestStandardLoggerPopulateSourceLocation(t *testing.T) {
	initLogs() // Generate new testLogID
	ctx := context.Background()
	lg := client.Logger(testLogID, logging.SourceLocationPopulation(logging.AlwaysPopulateSourceLocation))
	slg := lg.StandardLogger(logging.Info)

	_, _, line, lineOk := runtime.Caller(0)
	if !lineOk {
		t.Fatal("Cannot determine line number")
	}
	wantLine := int64(line + 5)
	slg.Print("info")
	if err := lg.Flush(); err != nil {
		t.Fatal(err)
	}
	var got []*logging.Entry
	ok := waitFor(func() bool {
		var err error
		got, err = allTestLogEntries(ctx)
		if err != nil {
			t.Log("fetching log entries: ", err)
			return false
		}
		return len(got) == 1
	})
	if !ok {
		t.Fatalf("timed out; got: %d, want: %d\n", len(got), 1)
	}
	if len(got) != 1 {
		t.Fatalf("expected non-nil request with one entry; got:\n%+v", got)
	}
	if got, want := filepath.Base(got[0].SourceLocation.GetFile()), "logging_test.go"; got != want {
		t.Errorf("sourcelocation file: got %s, want %s", got, want)
	}
	if got, want := got[0].SourceLocation.GetFunction(), "cloud.google.com/go/logging_test.TestStandardLoggerPopulateSourceLocation"; got != want {
		t.Errorf("sourcelocation function: got %s, want %s", got, want)
	}
	if got := got[0].SourceLocation.Line; got != wantLine {
		t.Errorf("source location line: got %d, want %d", got, wantLine)
	}
}

func TestStandardLoggerFromTemplate(t *testing.T) {
	tests := []struct {
		name     string
		template logging.Entry
		message  string
		want     logging.Entry
	}{
		{
			name: "severity only",
			template: logging.Entry{
				Severity: logging.Error,
			},
			message: "log message",
			want: logging.Entry{
				Severity: logging.Error,
				Payload:  "log message\n",
			},
		},
		{
			name: "severity and trace",
			template: logging.Entry{
				Severity: logging.Info,
				Trace:    "projects/P/traces/105445aa7843bc8bf206b120001000",
			},
			message: "log message",
			want: logging.Entry{
				Severity: logging.Info,
				Payload:  "log message\n",
				Trace:    "projects/P/traces/105445aa7843bc8bf206b120001000",
			},
		},
		{
			name: "severity and http request",
			template: logging.Entry{
				Severity: logging.Info,
				HTTPRequest: &logging.HTTPRequest{
					Request: &http.Request{
						Method: "GET",
						Host:   "example.com",
					},
					Status: 200,
				},
			},
			message: "log message",
			want: logging.Entry{
				Severity: logging.Info,
				Payload:  "log message\n",
				HTTPRequest: &logging.HTTPRequest{
					Request: &http.Request{
						Method: "GET",
						Host:   "example.com",
					},
					Status: 200,
				},
			},
		},
		{
			name: "payload in template is ignored",
			template: logging.Entry{
				Severity: logging.Info,
				Payload:  "this should not be set in the template",
				Trace:    "projects/P/traces/105445aa7843bc8bf206b120001000",
			},
			message: "log message",
			want: logging.Entry{
				Severity: logging.Info,
				Payload:  "log message\n",
				Trace:    "projects/P/traces/105445aa7843bc8bf206b120001000",
			},
		},
	}
	lg := client.Logger(testLogID)
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			mock := func(got logging.Entry, l *logging.Logger, parent string, skipLevels int) (*logpb.LogEntry, error) {
				if !reflect.DeepEqual(got, tc.want) {
					t.Errorf("Emitted Entry incorrect. Expected %v got %v", tc.want, got)
				}
				// Return value is not interesting
				return &logpb.LogEntry{}, nil
			}

			f := logging.SetToLogEntryInternal(mock)
			defer func() { logging.SetToLogEntryInternal(f) }()

			slg := lg.StandardLoggerFromTemplate(&tc.template)
			slg.Print(tc.message)
			if err := lg.Flush(); err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestSeverity(t *testing.T) {
	if got, want := logging.Info.String(), "Info"; got != want {
		t.Errorf("got %q, want %q", got, want)
	}
	if got, want := logging.Severity(-99).String(), "-99"; got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestParseSeverity(t *testing.T) {
	for _, test := range []struct {
		in   string
		want logging.Severity
	}{
		{"", logging.Default},
		{"whatever", logging.Default},
		{"Default", logging.Default},
		{"ERROR", logging.Error},
		{"Error", logging.Error},
		{"error", logging.Error},
	} {
		got := logging.ParseSeverity(test.in)
		if got != test.want {
			t.Errorf("%q: got %s, want %s\n", test.in, got, test.want)
		}
	}
}

func TestErrors(t *testing.T) {
	initLogs() // Generate new testLogID
	// Drain errors already seen.
loop:
	for {
		select {
		case <-errorc:
		default:
			break loop
		}
	}
	// Try to log something that can't be JSON-marshalled.
	lg := client.Logger(testLogID)
	lg.Log(logging.Entry{Payload: func() {}})
	// Expect an error from Flush.
	err := lg.Flush()
	if err == nil {
		t.Fatal("expected error, got nil")
	}
}

type badTokenSource struct{}

func (badTokenSource) Token() (*oauth2.Token, error) {
	return &oauth2.Token{}, nil
}

func TestPing(t *testing.T) {
	// Ping twice, in case the service's InsertID logic messes with the error code.
	ctx := context.Background()
	// The global client should be valid.
	if err := client.Ping(ctx); err != nil {
		t.Errorf("project %s: got %v, expected nil", testProjectID, err)
	}
	if err := client.Ping(ctx); err != nil {
		t.Errorf("project %s, #2: got %v, expected nil", testProjectID, err)
	}
	// nonexistent project
	c, a := newClients(ctx, testProjectID+"-BAD")
	defer c.Close()
	defer a.Close()
	if err := c.Ping(ctx); err == nil {
		t.Errorf("nonexistent project: want error pinging logging api, got nil")
	}
	if err := c.Ping(ctx); err == nil {
		t.Errorf("nonexistent project, #2: want error pinging logging api, got nil")
	}

	// Bad creds. We cannot test this with the fake, since it doesn't do auth.
	if integrationTest {
		c, err := logging.NewClient(ctx, testProjectID, option.WithTokenSource(badTokenSource{}))
		if err != nil {
			t.Fatal(err)
		}
		if err := c.Ping(ctx); err == nil {
			t.Errorf("bad creds: want error pinging logging api, got nil")
		}
		if err := c.Ping(ctx); err == nil {
			t.Errorf("bad creds, #2: want error pinging logging api, got nil")
		}
		if err := c.Close(); err != nil {
			t.Fatalf("error closing client: %v", err)
		}
	}
}

func TestDeleteLog(t *testing.T) {
	ctx := context.Background()
	initLogs()
	c, a := newClients(ctx, testProjectID)
	defer c.Close()
	defer a.Close()
	lg := c.Logger(testLogID)

	if err := lg.LogSync(ctx, logging.Entry{Payload: "hello"}); err != nil {
		t.Fatal(err)
	}

	if err := aclient.DeleteLog(ctx, testLogID); err != nil {
		// Ignore NotFound. Sometimes, amazingly, DeleteLog cannot find
		// a log that is returned by Logs.
		if status.Code(err) != codes.NotFound {
			t.Fatalf("deleting %q: %v", testLogID, err)
		}
	} else {
		t.Logf("deleted log_id: %q", testLogID)
	}
}

func TestNonProjectParent(t *testing.T) {
	ctx := context.Background()
	initLogs()
	parent := "organizations/" + ltesting.ValidOrgID
	c, a := newClients(ctx, parent)
	defer c.Close()
	defer a.Close()
	lg := c.Logger(testLogID)
	err := lg.LogSync(ctx, logging.Entry{Payload: "hello"})
	if integrationTest {
		// We don't have permission to log to the organization.
		if got, want := status.Code(err), codes.PermissionDenied; got != want {
			t.Errorf("got code %s, want %s", got, want)
		}
		return
	}
	// Continue test against fake.
	if err != nil {
		t.Fatal(err)
	}
	want := []*logging.Entry{{
		Timestamp: testNow().UTC(),
		Payload:   "hello",
		LogName:   parent + "/logs/" + testLogID,
		Resource: &mrpb.MonitoredResource{
			Type:   "organization",
			Labels: map[string]string{"organization_id": ltesting.ValidOrgID},
		},
	}}
	var got []*logging.Entry
	ok := waitFor(func() bool {
		got, err = allEntries(ctx, a, fmt.Sprintf(`logName = "%s/logs/%s"`, parent,
			strings.Replace(testLogID, "/", "%2F", -1)))
		if err != nil {
			t.Log("fetching log entries: ", err)
			return false
		}
		return len(got) == len(want)
	})
	if !ok {
		t.Fatalf("timed out; got: %d, want: %d\n", len(got), len(want))
	}
	if msg, ok := compareEntries(got, want); !ok {
		t.Error(msg)
	}
}

func TestDetectProjectIdParent(t *testing.T) {
	ctx := context.Background()
	initLogs()
	addr, err := ltesting.NewServer()
	if err != nil {
		t.Fatalf("creating fake server: %v", err)
	}
	conn, err := grpc.Dial(addr, grpc.WithInsecure())
	if err != nil {
		t.Fatalf("dialing %q: %v", addr, err)
	}

	tests := []struct {
		name      string
		resource  *mrpb.MonitoredResource
		want      string
		wantError error
	}{
		{
			name: "Test DetectProjectId parent properly set up resource detection",
			resource: &mrpb.MonitoredResource{
				Labels: map[string]string{"project_id": testProjectID},
			},
			want: "projects/" + testProjectID,
		},
		{
			name:      "Test DetectProjectId parent no resource detected",
			resource:  nil,
			wantError: errors.New("could not determine project ID from environment"),
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			// Check if toLogEntryInternal was called with the right parent
			toLogEntryInternalMock := func(got logging.Entry, l *logging.Logger, parent string, skipLevels int) (*logpb.LogEntry, error) {
				if parent != test.want {
					t.Errorf("toLogEntryInternal called with wrong parent. got: %s want: %s", parent, test.want)
				}
				return &logpb.LogEntry{}, nil
			}

			detectResourceMock := func() *mrpb.MonitoredResource {
				return test.resource
			}

			realToLogEntryInternal := logging.SetToLogEntryInternal(toLogEntryInternalMock)
			defer func() { logging.SetToLogEntryInternal(realToLogEntryInternal) }()

			realDetectResourceInternal := logging.SetDetectResourceInternal(detectResourceMock)
			defer func() { logging.SetDetectResourceInternal(realDetectResourceInternal) }()

			cli, err := logging.NewClient(ctx, logging.DetectProjectID, option.WithGRPCConn(conn))
			if err != nil && test.wantError == nil {
				t.Fatalf("Unexpected error: %+v: %v", test.resource, err)
			}
			if err == nil && test.wantError != nil {
				t.Fatalf("Error is expected: %+v: %v", test.resource, test.wantError)
			}
			if test.wantError != nil {
				return
			}

			cli.Logger(testLogID).LogSync(ctx, logging.Entry{Payload: "hello"})
		})
	}
}

// waitFor calls f repeatedly with exponential backoff, blocking until it returns true.
// It returns false after a while (if it times out).
func waitFor(f func() bool) bool {
	// TODO(shadams): Find a better way to deflake these tests.
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
	defer cancel()
	err := cinternal.Retry(ctx,
		gax.Backoff{Initial: time.Second, Multiplier: 2, Max: 30 * time.Second},
		func() (bool, error) { return f(), nil })
	return err == nil
}

// Interleave a lot of Log and Flush calls, to induce race conditions.
// Run this test with:
//
//	go test -run LogFlushRace -race -count 100
func TestLogFlushRace(t *testing.T) {
	initLogs() // Generate new testLogID
	lg := client.Logger(testLogID,
		logging.ConcurrentWriteLimit(5),  // up to 5 concurrent log writes
		logging.EntryCountThreshold(100)) // small bundle size to increase interleaving
	var wgf, wgl sync.WaitGroup
	donec := make(chan struct{})
	for i := 0; i < 10; i++ {
		wgl.Add(1)
		go func() {
			defer wgl.Done()
			for j := 0; j < 1e4; j++ {
				lg.Log(logging.Entry{Payload: "the payload"})
			}
		}()
	}
	for i := 0; i < 5; i++ {
		wgf.Add(1)
		go func() {
			defer wgf.Done()
			for {
				select {
				case <-donec:
					return
				case <-time.After(time.Duration(rand.Intn(5)) * time.Millisecond):
					if err := lg.Flush(); err != nil {
						t.Error(err)
					}
				}
			}
		}()
	}
	wgl.Wait()
	close(donec)
	wgf.Wait()
}

// Test the throughput of concurrent writers.
func BenchmarkConcurrentWrites(b *testing.B) {
	if !integrationTest {
		b.Skip("only makes sense when running against production service")
	}
	for n := 1; n <= 32; n *= 2 {
		b.Run(fmt.Sprint(n), func(b *testing.B) {
			b.StopTimer()
			lg := client.Logger(testLogID, logging.ConcurrentWriteLimit(n), logging.EntryCountThreshold(1000))
			const (
				nEntries = 1e5
				payload  = "the quick brown fox jumps over the lazy dog"
			)
			b.SetBytes(int64(nEntries * len(payload)))
			b.StartTimer()
			for i := 0; i < b.N; i++ {
				for j := 0; j < nEntries; j++ {
					lg.Log(logging.Entry{Payload: payload})
				}
				if err := lg.Flush(); err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}

func TestSeverityUnmarshal(t *testing.T) {
	j := []byte(`{"logName": "test-log","severity": "ERROR","payload": "test"}`)
	var entry logging.Entry
	err := json.Unmarshal(j, &entry)
	if err != nil {
		t.Fatalf("en.Unmarshal: %v", err)
	}
	if entry.Severity != logging.Error {
		t.Fatalf("Severity: got %v, want %v", entry.Severity, logging.Error)
	}
}

func TestSeverityAsNumberUnmarshal(t *testing.T) {
	j := []byte(fmt.Sprintf(`{"logName": "test-log","severity": %d, "payload": "test"}`, logging.Info))
	var entry logging.Entry
	err := json.Unmarshal(j, &entry)
	if err != nil {
		t.Fatalf("en.Unmarshal: %v", err)
	}
	if entry.Severity != logging.Info {
		t.Fatalf("Severity: got %v, want %v", entry.Severity, logging.Info)
	}
}

func TestSeverityMarshalThenUnmarshal(t *testing.T) {
	entry := logging.Entry{Severity: logging.Warning, Payload: "test"}
	j, err := json.Marshal(entry)
	if err != nil {
		t.Fatalf("en.Marshal: %v", err)
	}

	var entryU logging.Entry

	err = json.Unmarshal(j, &entryU)
	if err != nil {
		t.Fatalf("en.Unmarshal: %v", err)
	}

	if entryU.Severity != logging.Warning {
		t.Fatalf("Severity: got %v, want %v", entryU.Severity, logging.Warning)
	}
}

func TestSourceLocationPopulation(t *testing.T) {
	tests := []struct {
		name   string
		logger *logging.Logger
		in     logging.Entry
		want   *logpb.LogEntrySourceLocation
	}{
		{
			name:   "populate source location for debug entry when allowed",
			logger: client.Logger("test-source-location", logging.SourceLocationPopulation(logging.PopulateSourceLocationForDebugEntries)),
			in: logging.Entry{
				Severity: logging.Severity(logging.Debug),
			},
			// want field will be patched to setup actual code line and function name
			want: nil,
		}, {
			name:   "populate source location for non-debug entry when allowed",
			logger: client.Logger("test-source-location", logging.SourceLocationPopulation(logging.AlwaysPopulateSourceLocation)),
			in: logging.Entry{
				Severity: logging.Severity(logging.Default),
			},
			// want field will be patched to setup actual code line and function name
			want: nil,
		}, {
			name:   "do not populate source location for debug entry with source location",
			logger: client.Logger("test-source-location", logging.SourceLocationPopulation(logging.PopulateSourceLocationForDebugEntries)),
			in: logging.Entry{
				Severity: logging.Severity(logging.Debug),
				SourceLocation: &logpb.LogEntrySourceLocation{
					File:     "test_source_file.go",
					Function: "testFunction",
					Line:     65536,
				},
			},
			want: &logpb.LogEntrySourceLocation{
				File:     "test_source_file.go",
				Function: "testFunction",
				Line:     65536,
			},
		}, {
			name:   "do not populate source location for non-debug entry when only allowed for debug",
			logger: client.Logger("test-source-location", logging.SourceLocationPopulation(logging.PopulateSourceLocationForDebugEntries)),
			in: logging.Entry{
				Severity: logging.Severity(logging.Info),
			},
			want: nil,
		}, {
			name:   "do not populate source location when not allowed for any",
			logger: client.Logger("test-source-location", logging.SourceLocationPopulation(logging.DoNotPopulateSourceLocation)),
			in: logging.Entry{
				Severity: logging.Severity(logging.Debug),
			},
			want: nil,
		}, {
			name:   "do not populate source location by default",
			logger: client.Logger("test-source-location"),
			in: logging.Entry{
				Severity: logging.Severity(logging.Debug),
			},
			want: nil,
		},
	}

	for index, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			// patch first two want results to produce correct source info
			if index < 2 {
				pc, file, line, ok := runtime.Caller(0)
				if !ok {
					t.Fatalf("Unexpected error: %+v: failed to call runtime.Caller()", tc.in)
				}
				details := runtime.FuncForPC(pc)
				tc.want = &logpb.LogEntrySourceLocation{
					File:     file,
					Function: details.Name(),
					Line:     int64(line + 11), // 11 code lines between runtime.Caller() and logging.ToLogEntry()
				}
			}
			e, err := tc.logger.ToLogEntry(tc.in, "projects/P")
			if err != nil {
				t.Fatalf("Unexpected error: %+v: %v", tc.in, err)
			}

			if e.SourceLocation != tc.want {
				if diff := cmp.Diff(e.SourceLocation, tc.want, cmpopts.IgnoreUnexported(logpb.LogEntrySourceLocation{})); diff != "" {
					t.Errorf("got(-),want(+):\n%s", diff)
				}
			}
		})
	}
}

func BenchmarkSourceLocationPopulation(b *testing.B) {
	logger := *client.Logger("test-source-location", logging.SourceLocationPopulation(logging.PopulateSourceLocationForDebugEntries))
	tests := []struct {
		name string
		in   logging.Entry
	}{
		{
			name: "with source location population",
			in: logging.Entry{
				Severity: logging.Severity(logging.Debug),
			},
		}, {
			name: "without source location population",
			in: logging.Entry{
				Severity: logging.Severity(logging.Info),
			},
		},
	}
	var err error
	for _, tc := range tests {
		b.Run(tc.name, func(b *testing.B) {
			for n := 0; n < b.N; n++ {
				_, err = logger.ToLogEntry(tc.in, "projects/P")
				if err != nil {
					b.Fatalf("Unexpected error: %+v: %v", tc.in, err)
				}
			}
		})
	}
}

// writeLogEntriesTestHandler is a fake Logging backend handler used to test partialSuccess option logic
type writeLogEntriesTestHandler struct {
	logpb.UnimplementedLoggingServiceV2Server
	hook func(*logpb.WriteLogEntriesRequest)
}

func (f *writeLogEntriesTestHandler) WriteLogEntries(_ context.Context, e *logpb.WriteLogEntriesRequest) (*logpb.WriteLogEntriesResponse, error) {
	if f.hook != nil {
		f.hook(e)
	}
	return &logpb.WriteLogEntriesResponse{}, nil
}

func fakeClient(parent string, writeLogEntryHandler func(e *logpb.WriteLogEntriesRequest), serverOptions ...grpc.ServerOption) (*logging.Client, error) {
	// setup fake server
	fakeBackend := &writeLogEntriesTestHandler{}
	l, err := net.Listen("tcp", "localhost:0")
	if err != nil {
		return nil, err
	}
	gsrv := grpc.NewServer(serverOptions...)
	logpb.RegisterLoggingServiceV2Server(gsrv, fakeBackend)
	fakeServerAddr := l.Addr().String()
	go func() {
		if err := gsrv.Serve(l); err != nil {
			panic(err)
		}
	}()
	fakeBackend.hook = writeLogEntryHandler
	ctx := context.Background()
	client, _ := logging.NewClient(ctx, parent, option.WithEndpoint(fakeServerAddr),
		option.WithoutAuthentication(),
		option.WithGRPCDialOption(grpc.WithInsecure()))
	return client, nil
}

func TestPartialSuccessOption(t *testing.T) {
	var logger *logging.Logger
	var partialSuccess bool

	entry := logging.Entry{Payload: "payload string"}
	tests := []struct {
		name string
		do   func()
	}{
		{
			name: "use PartialSuccess with LogSync",
			do: func() {
				logger.LogSync(context.Background(), entry)
			},
		},
		{
			name: "use PartialSuccess with Log",
			do: func() {
				logger.Log(entry)
				logger.Flush()
			},
		},
	}

	// setup fake client
	client, err := fakeClient("projects/test", func(e *logpb.WriteLogEntriesRequest) {
		partialSuccess = e.PartialSuccess
	})
	if err != nil {
		t.Fatal(err)
	}
	defer client.Close()
	logger = client.Logger("abc", logging.PartialSuccess())

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			partialSuccess = false
			tc.do()
			if !partialSuccess {
				t.Fatal("e.PartialSuccess = false, want true")
			}
		})
	}
}

func TestWriteLogEntriesSizeLimit(t *testing.T) {
	// Test that logging too many large requests at once doesn't bump up
	// against WriteLogEntriesRequest size limit
	sizeLimit := 10485760 // 10MiB size limit

	// Create a fake client whose server can only handle messages of at most sizeLimit
	client, err := fakeClient("projects/test", func(e *logpb.WriteLogEntriesRequest) {}, grpc.MaxRecvMsgSize(sizeLimit))
	if err != nil {
		t.Fatal(err)
	}

	client.OnError = func(e error) {
		t.Fatalf(e.Error())
	}

	defer client.Close()
	logger := client.Logger("test")
	entry := logging.Entry{Payload: strings.Repeat("1", 250000)}

	for i := 0; i < 200; i++ {
		logger.Log(entry)
	}
}

func TestRedirectOutputIngestion(t *testing.T) {
	var hookCalled bool

	// setup fake client
	client, err := fakeClient("projects/test", func(e *logpb.WriteLogEntriesRequest) {
		hookCalled = true
	})
	if err != nil {
		t.Fatal(err)
	}
	defer client.Close()

	entry := logging.Entry{Payload: "testing payload string"}
	tests := []struct {
		name   string
		logger *logging.Logger
		want   bool
	}{
		{
			name:   "redirect output does not ingest",
			logger: client.Logger("stdout-redirection-log", logging.RedirectAsJSON(os.Stdout)),
			want:   false,
		},
		{
			name:   "log without Redirect flags ingest",
			logger: client.Logger("default-ingestion-log"),
			want:   true,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			hookCalled = false
			tc.logger.LogSync(context.Background(), entry)
			if hookCalled != tc.want {
				t.Errorf("Log ingestion works unexpected: got %v want %v\n", hookCalled, tc.want)
			}
		})
	}
}

func TestRedirectOutputFormats(t *testing.T) {
	testURL, _ := url.Parse("https://example.com/test")
	tests := []struct {
		name      string
		in        *logging.Entry
		want      string
		wantError error
	}{
		{
			name: "full data redirect with text payload",
			in: &logging.Entry{
				Labels:       map[string]string{"key1": "value1", "key2": "value2"},
				Timestamp:    testNow().UTC(),
				Severity:     logging.Debug,
				InsertID:     "0000AAA01",
				Trace:        "projects/P/ABCD12345678AB12345678",
				SpanID:       "000000000001",
				TraceSampled: true,
				SourceLocation: &logpb.LogEntrySourceLocation{
					File:     "acme.go",
					Function: "main",
					Line:     100,
				},
				Operation: &logpb.LogEntryOperation{
					Id:       "0123456789",
					Producer: "test",
				},
				HTTPRequest: &logging.HTTPRequest{
					Request: &http.Request{
						URL:    testURL,
						Method: "POST",
					},
				},

				Payload: "this is text payload",
			},
			want: `{"httpRequest":{"requestMethod":"POST","requestUrl":"https://example.com/test"},"logging.googleapis.com/insertId":"0000AAA01",` +
				`"logging.googleapis.com/labels":{"key1":"value1","key2":"value2"},"logging.googleapis.com/operation":{"id":"0123456789","producer":"test"},` +
				`"logging.googleapis.com/sourceLocation":{"file":"acme.go","function":"main","line":"100"},"logging.googleapis.com/spanId":"000000000001",` +
				`"logging.googleapis.com/trace":"projects/P/ABCD12345678AB12345678","logging.googleapis.com/trace_sampled":true,` +
				`"message":"this is text payload","severity":"DEBUG","timestamp":"seconds:1000"}`,
		},
		{
			name: "full data redirect with json payload",
			in: &logging.Entry{
				Labels:       map[string]string{"key1": "value1", "key2": "value2"},
				Timestamp:    testNow().UTC(),
				Severity:     logging.Debug,
				InsertID:     "0000AAA01",
				Trace:        "projects/P/ABCD12345678AB12345678",
				SpanID:       "000000000001",
				TraceSampled: true,
				SourceLocation: &logpb.LogEntrySourceLocation{
					File:     "acme.go",
					Function: "main",
					Line:     100,
				},
				Operation: &logpb.LogEntryOperation{
					Id:       "0123456789",
					Producer: "test",
				},
				HTTPRequest: &logging.HTTPRequest{
					Request: &http.Request{
						URL:    testURL,
						Method: "POST",
					},
				},
				Payload: map[string]interface{}{
					"Message": "message part of the payload",
					"Latency": 321,
				},
			},
			want: `{"httpRequest":{"requestMethod":"POST","requestUrl":"https://example.com/test"},"logging.googleapis.com/insertId":"0000AAA01",` +
				`"logging.googleapis.com/labels":{"key1":"value1","key2":"value2"},"logging.googleapis.com/operation":{"id":"0123456789","producer":"test"},` +
				`"logging.googleapis.com/sourceLocation":{"file":"acme.go","function":"main","line":"100"},"logging.googleapis.com/spanId":"000000000001",` +
				`"logging.googleapis.com/trace":"projects/P/ABCD12345678AB12345678","logging.googleapis.com/trace_sampled":true,` +
				`"message":{"Latency":321,"Message":"message part of the payload"},"severity":"DEBUG","timestamp":"seconds:1000"}`,
		},
		{
			name: "error on redirect with proto payload",
			in: &logging.Entry{
				Timestamp: testNow().UTC(),
				Severity:  logging.Debug,
				Payload:   &anypb.Any{},
			},
			wantError: logging.ErrRedirectProtoPayloadNotSupported,
		},
	}
	buffer := &strings.Builder{}
	logger := client.Logger("test-redirect-output", logging.RedirectAsJSON(buffer))
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			buffer.Reset()
			err := logger.LogSync(context.Background(), *tc.in)
			if err != nil {
				if tc.wantError == nil {
					t.Fatalf("Unexpected error: %+v: %v", tc.in, err)
				}
				if tc.wantError != err {
					t.Errorf("Expected error: %+v, got: %v want: %v\n", tc.in, err, tc.wantError)
				}
			} else {
				if tc.wantError != nil {
					t.Errorf("Expected error: %+v, want: %v\n", tc.in, tc.wantError)
				}
				got := strings.TrimSpace(buffer.String())

				// Compare structure equivalence of the outputs, not string equivalence, as order doesn't matter.
				var gotJson, wantJson interface{}

				err = json.Unmarshal([]byte(got), &gotJson)
				if err != nil {
					t.Errorf("Error when serializing JSON output: %v", err)
				}

				err = json.Unmarshal([]byte(tc.want), &wantJson)
				if err != nil {
					t.Fatalf("Error unmarshalling JSON input for want: %v", err)
				}

				if !reflect.DeepEqual(gotJson, wantJson) {
					t.Errorf("TestRedirectOutputFormats: %+v: got %v, want %v", tc.in, got, tc.want)
				}
			}
		})
	}
}

func TestInstrumentationIngestion(t *testing.T) {
	var got []*logpb.LogEntry

	// setup fake client
	client, err := fakeClient("projects/test", func(e *logpb.WriteLogEntriesRequest) {
		got = e.GetEntries()
	})
	if err != nil {
		t.Fatal(err)
	}
	defer client.Close()

	entry := &logging.Entry{Severity: logging.Info, Payload: "test string"}
	logger := client.Logger("test-instrumentation")
	tests := []struct {
		entryLen      int
		hasDiagnostic bool
	}{
		{
			entryLen:      2,
			hasDiagnostic: true,
		},
		{
			entryLen:      1,
			hasDiagnostic: false,
		},
	}
	onceBackup := internal.InstrumentOnce
	internal.InstrumentOnce = new(sync.Once)
	for _, test := range tests {
		got = nil
		err := logger.LogSync(context.Background(), *entry)
		if err != nil {
			t.Fatal(err)
		}
		if len(got) != test.entryLen {
			t.Errorf("got(%v), want(%v)", got, test.entryLen)
		}
		diagnosticEntry := false
		for _, ent := range got {
			if internal.LogIDFromPath("projects/test", ent.LogName) == "diagnostic-log" {
				diagnosticEntry = true
				break
			}
		}
		if diagnosticEntry != test.hasDiagnostic {
			t.Errorf("instrumentation entry misplaced: got(%v), want(%v)", diagnosticEntry, test.hasDiagnostic)
		}
	}
	internal.InstrumentOnce = onceBackup
}

func TestInstrumentationWithRedirect(t *testing.T) {
	want := []string{
		// do not format the string to preserve expected new-line between messages
		`{"message":"test string","severity":"INFO","timestamp":"seconds:1000"}
{"message":{"logging.googleapis.com/diagnostic":{"instrumentation_source":[{"name":"go","version":"` + internal.Version + `"}],"runtime":"` + internal.VersionGo() + `"}},"severity":"DEFAULT","timestamp":"seconds:1000"}`,
		`{"message":"test string","severity":"INFO","timestamp":"seconds:1000"}`,
	}
	entry := &logging.Entry{Severity: logging.Info, Payload: "test string"}
	buffer := &strings.Builder{}
	logger := client.Logger("test-redirect-output", logging.RedirectAsJSON(buffer))
	onceBackup, timeBackup := internal.InstrumentOnce, logging.SetNow(testNow)
	internal.InstrumentOnce = new(sync.Once)
	for i := range want {
		buffer.Reset()
		err := logger.LogSync(context.Background(), *entry)
		if err != nil {
			t.Fatal(err)
		}
		got := strings.TrimSpace(buffer.String())
		if got != want[i] {
			t.Errorf("got(%v), want(%v)", got, want[i])
		}
	}
	logging.SetNow(timeBackup)
	internal.InstrumentOnce = onceBackup
}

func ExampleRedirectAsJSON_withStdout() {
	logger := client.Logger("redirect-to-stdout", logging.RedirectAsJSON(os.Stdout))
	logger.Log(logging.Entry{Severity: logging.Debug, Payload: "redirected log"})
}

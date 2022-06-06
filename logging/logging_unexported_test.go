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

// Tests that require access to unexported names of the logging package.

package logging

import (
	"encoding/json"
	"net/http"
	"net/url"
	"strings"
	"testing"
	"time"

	"cloud.google.com/go/internal/testutil"
	"github.com/golang/protobuf/proto"
	durpb "github.com/golang/protobuf/ptypes/duration"
	structpb "github.com/golang/protobuf/ptypes/struct"
	"google.golang.org/api/support/bundler"
	mrpb "google.golang.org/genproto/googleapis/api/monitoredres"
	logtypepb "google.golang.org/genproto/googleapis/logging/type"
	logpb "google.golang.org/genproto/googleapis/logging/v2"
	"google.golang.org/protobuf/types/known/anypb"
	"google.golang.org/protobuf/types/known/timestamppb"
)

func TestLoggerCreation(t *testing.T) {
	const logID = "testing"
	c := &Client{parent: "projects/PROJECT_ID"}
	customResource := &mrpb.MonitoredResource{
		Type: "global",
		Labels: map[string]string{
			"project_id": "ANOTHER_PROJECT",
		},
	}
	defaultBundler := &bundler.Bundler{
		DelayThreshold:       DefaultDelayThreshold,
		BundleCountThreshold: DefaultEntryCountThreshold,
		BundleByteThreshold:  DefaultEntryByteThreshold,
		BundleByteLimit:      0,
		BufferedByteLimit:    DefaultBufferedByteLimit,
	}
	for _, test := range []struct {
		options         []LoggerOption
		wantLogger      *Logger
		defaultResource bool
		wantBundler     *bundler.Bundler
	}{
		{
			options:         nil,
			wantLogger:      &Logger{},
			defaultResource: true,
			wantBundler:     defaultBundler,
		},
		{
			options: []LoggerOption{
				CommonResource(nil),
				CommonLabels(map[string]string{"a": "1"}),
			},
			wantLogger: &Logger{
				commonResource: nil,
				commonLabels:   map[string]string{"a": "1"},
			},
			wantBundler: defaultBundler,
		},
		{
			options:     []LoggerOption{CommonResource(customResource)},
			wantLogger:  &Logger{commonResource: customResource},
			wantBundler: defaultBundler,
		},
		{
			options: []LoggerOption{
				DelayThreshold(time.Minute),
				EntryCountThreshold(99),
				EntryByteThreshold(17),
				EntryByteLimit(18),
				BufferedByteLimit(19),
			},
			wantLogger:      &Logger{},
			defaultResource: true,
			wantBundler: &bundler.Bundler{
				DelayThreshold:       time.Minute,
				BundleCountThreshold: 99,
				BundleByteThreshold:  17,
				BundleByteLimit:      18,
				BufferedByteLimit:    19,
			},
		},
	} {
		gotLogger := c.Logger(logID, test.options...)
		if got, want := gotLogger.commonResource, test.wantLogger.commonResource; !test.defaultResource && !proto.Equal(got, want) {
			t.Errorf("%v: resource: got %v, want %v", test.options, got, want)
		}
		if got, want := gotLogger.commonLabels, test.wantLogger.commonLabels; !testutil.Equal(got, want) {
			t.Errorf("%v: commonLabels: got %v, want %v", test.options, got, want)
		}
		if got, want := gotLogger.bundler.DelayThreshold, test.wantBundler.DelayThreshold; got != want {
			t.Errorf("%v: DelayThreshold: got %v, want %v", test.options, got, want)
		}
		if got, want := gotLogger.bundler.BundleCountThreshold, test.wantBundler.BundleCountThreshold; got != want {
			t.Errorf("%v: BundleCountThreshold: got %v, want %v", test.options, got, want)
		}
		if got, want := gotLogger.bundler.BundleByteThreshold, test.wantBundler.BundleByteThreshold; got != want {
			t.Errorf("%v: BundleByteThreshold: got %v, want %v", test.options, got, want)
		}
		if got, want := gotLogger.bundler.BundleByteLimit, test.wantBundler.BundleByteLimit; got != want {
			t.Errorf("%v: BundleByteLimit: got %v, want %v", test.options, got, want)
		}
		if got, want := gotLogger.bundler.BufferedByteLimit, test.wantBundler.BufferedByteLimit; got != want {
			t.Errorf("%v: BufferedByteLimit: got %v, want %v", test.options, got, want)
		}
	}
}

func TestToProtoStruct(t *testing.T) {
	v := struct {
		Foo string                 `json:"foo"`
		Bar int                    `json:"bar,omitempty"`
		Baz []float64              `json:"baz"`
		Moo map[string]interface{} `json:"moo"`
	}{
		Foo: "foovalue",
		Baz: []float64{1.1},
		Moo: map[string]interface{}{
			"a": 1,
			"b": "two",
			"c": true,
		},
	}

	got, err := toProtoStruct(v)
	if err != nil {
		t.Fatal(err)
	}
	want := &structpb.Struct{
		Fields: map[string]*structpb.Value{
			"foo": {Kind: &structpb.Value_StringValue{StringValue: v.Foo}},
			"baz": {Kind: &structpb.Value_ListValue{ListValue: &structpb.ListValue{Values: []*structpb.Value{
				{Kind: &structpb.Value_NumberValue{NumberValue: 1.1}},
			}}}},
			"moo": {Kind: &structpb.Value_StructValue{
				StructValue: &structpb.Struct{
					Fields: map[string]*structpb.Value{
						"a": {Kind: &structpb.Value_NumberValue{NumberValue: 1}},
						"b": {Kind: &structpb.Value_StringValue{StringValue: "two"}},
						"c": {Kind: &structpb.Value_BoolValue{BoolValue: true}},
					},
				},
			}},
		},
	}
	if !proto.Equal(got, want) {
		t.Errorf("got  %+v\nwant %+v", got, want)
	}

	// Non-structs should fail to convert.
	for v := range []interface{}{3, "foo", []int{1, 2, 3}} {
		_, err := toProtoStruct(v)
		if err == nil {
			t.Errorf("%v: got nil, want error", v)
		}
	}

	// Test fast path.
	got, err = toProtoStruct(want)
	if err != nil {
		t.Fatal(err)
	}
	if got != want {
		t.Error("got and want should be identical, but are not")
	}
}

func TestToLogEntryPayload(t *testing.T) {
	for _, test := range []struct {
		in         interface{}
		wantText   string
		wantStruct *structpb.Struct
	}{
		{
			in:       "string",
			wantText: "string",
		},
		{
			in: map[string]interface{}{"a": 1, "b": true},
			wantStruct: &structpb.Struct{
				Fields: map[string]*structpb.Value{
					"a": {Kind: &structpb.Value_NumberValue{NumberValue: 1}},
					"b": {Kind: &structpb.Value_BoolValue{BoolValue: true}},
				},
			},
		},
		{
			in: json.RawMessage([]byte(`{"a": 1, "b": true}`)),
			wantStruct: &structpb.Struct{
				Fields: map[string]*structpb.Value{
					"a": {Kind: &structpb.Value_NumberValue{NumberValue: 1}},
					"b": {Kind: &structpb.Value_BoolValue{BoolValue: true}},
				},
			},
		},
	} {
		e, err := toLogEntryInternal(Entry{Payload: test.in}, nil, "", 0)
		if err != nil {
			t.Fatalf("%+v: %v", test.in, err)
		}
		if test.wantStruct != nil {
			got := e.GetJsonPayload()
			if !proto.Equal(got, test.wantStruct) {
				t.Errorf("%+v: got %s, want %s", test.in, got, test.wantStruct)
			}
		} else {
			got := e.GetTextPayload()
			if got != test.wantText {
				t.Errorf("%+v: got %s, want %s", test.in, got, test.wantText)
			}
		}
	}
}

func TestFromHTTPRequest(t *testing.T) {
	// The test URL has invalid UTF-8 runes.
	const testURL = "http://example.com/path?q=1&name=\xfe\xff"
	u, err := url.Parse(testURL)
	if err != nil {
		t.Fatal(err)
	}
	req := &HTTPRequest{
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
		RemoteIP:                       "10.0.1.1",
		CacheHit:                       true,
		CacheValidatedWithOriginServer: true,
	}
	got, err := fromHTTPRequest(req)
	if err != nil {
		t.Errorf("got %v", err)
	}
	want := &logtypepb.HttpRequest{
		RequestMethod: "GET",

		// RequestUrl should have its invalid utf-8 runes replaced by the Unicode replacement character U+FFFD.
		// See Issue https://github.com/googleapis/google-cloud-go/issues/1383
		RequestUrl: "http://example.com/path?q=1&name=" + string('\ufffd') + string('\ufffd'),

		RequestSize:                    100,
		Status:                         200,
		ResponseSize:                   25,
		Latency:                        &durpb.Duration{Seconds: 100},
		UserAgent:                      "user-agent",
		ServerIp:                       "127.0.0.1",
		RemoteIp:                       "10.0.1.1",
		Referer:                        "referer",
		CacheHit:                       true,
		CacheValidatedWithOriginServer: true,
	}
	if !proto.Equal(got, want) {
		t.Errorf("got  %+v\nwant %+v", got, want)
	}

	// And finally checks directly that the error that was
	// in https://github.com/googleapis/google-cloud-go/issues/1383
	// doesn't not regress.
	if _, err := proto.Marshal(got); err != nil {
		t.Fatalf("Unexpected proto.Marshal error: %v", err)
	}

	// fromHTTPRequest returns nil if there is no Request property (but does not panic)
	reqNil := &HTTPRequest{
		RequestSize: 100,
	}
	got, err = fromHTTPRequest(reqNil)
	if got != nil && err == nil {
		t.Errorf("got  %+v\nwant %+v", got, want)
	}
}

func TestMonitoredResource(t *testing.T) {
	for _, test := range []struct {
		parent string
		want   *mrpb.MonitoredResource
	}{
		{
			"projects/P",
			&mrpb.MonitoredResource{
				Type:   "project",
				Labels: map[string]string{"project_id": "P"},
			},
		},

		{
			"folders/F",
			&mrpb.MonitoredResource{
				Type:   "folder",
				Labels: map[string]string{"folder_id": "F"},
			},
		},
		{
			"billingAccounts/B",
			&mrpb.MonitoredResource{
				Type:   "billing_account",
				Labels: map[string]string{"account_id": "B"},
			},
		},
		{
			"organizations/123",
			&mrpb.MonitoredResource{
				Type:   "organization",
				Labels: map[string]string{"organization_id": "123"},
			},
		},
		{
			"unknown/X",
			&mrpb.MonitoredResource{
				Type:   "global",
				Labels: map[string]string{"project_id": "X"},
			},
		},
		{
			"whatever",
			&mrpb.MonitoredResource{
				Type:   "global",
				Labels: map[string]string{"project_id": "whatever"},
			},
		},
	} {
		got := monitoredResource(test.parent)
		if !testutil.Equal(got, test.want) {
			t.Errorf("%q: got %+v, want %+v", test.parent, got, test.want)
		}
	}
}

// Used by the tests in logging_test.
func SetNow(f func() time.Time) {
	now = f
}

func TestRedirectOutputFormats(t *testing.T) {
	tests := []struct {
		name      string
		in        *logpb.LogEntry
		want      string
		wantError error
	}{
		{
			name: "full data redirect with text payload",
			in: &logpb.LogEntry{
				Labels: map[string]string{"key1": "value1", "key2": "value2"},
				Timestamp: &timestamppb.Timestamp{
					Seconds: 1641024000,
				},
				Severity:     logtypepb.LogSeverity_DEBUG,
				InsertId:     "0000AAA01",
				Trace:        "projects/P/ABCD12345678AB12345678",
				SpanId:       "000000000001",
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
				HttpRequest: &logtypepb.HttpRequest{
					RequestUrl:    "https://example.com/test",
					RequestMethod: "POST",
				},
				Payload: &logpb.LogEntry_TextPayload{
					TextPayload: "this is text payload",
				},
			},
			want: `{"message":"this is text payload","severity":"DEBUG","httpRequest":{"request_method":"POST","request_url":"https://example.com/test"},` +
				`"timestamp":"seconds:1641024000","logging.googleapis.com/labels":{"key1":"value1","key2":"value2"},"logging.googleapis.com/insertId":"0000AAA01",` +
				`"logging.googleapis.com/operation":{"id":"0123456789","producer":"test"},"logging.googleapis.com/sourceLocation":{"file":"acme.go","line":100,"function":"main"},` +
				`"logging.googleapis.com/spanId":"000000000001","logging.googleapis.com/trace":"projects/P/ABCD12345678AB12345678","logging.googleapis.com/trace_sampled":true}`,
		},
		{
			name: "full data redirect with json payload",
			in: &logpb.LogEntry{
				Labels: map[string]string{"key1": "value1", "key2": "value2"},
				Timestamp: &timestamppb.Timestamp{
					Seconds: 1641024000,
				},
				Severity:     logtypepb.LogSeverity_DEBUG,
				InsertId:     "0000AAA01",
				Trace:        "projects/P/ABCD12345678AB12345678",
				SpanId:       "000000000001",
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
				HttpRequest: &logtypepb.HttpRequest{
					RequestUrl:    "https://example.com/test",
					RequestMethod: "POST",
				},
				Payload: &logpb.LogEntry_JsonPayload{
					JsonPayload: &structpb.Struct{
						Fields: map[string]*structpb.Value{
							"Message": {
								Kind: &structpb.Value_StringValue{
									StringValue: "message part of the payload",
								},
							},
							"Latency": {
								Kind: &structpb.Value_NumberValue{
									NumberValue: 321,
								},
							},
						},
					},
				},
			},
			want: `{"message":{"Latency":321,"Message":"message part of the payload"},"severity":"DEBUG","httpRequest":{"request_method":"POST","request_url":"https://example.com/test"},` +
				`"timestamp":"seconds:1641024000","logging.googleapis.com/labels":{"key1":"value1","key2":"value2"},"logging.googleapis.com/insertId":"0000AAA01",` +
				`"logging.googleapis.com/operation":{"id":"0123456789","producer":"test"},"logging.googleapis.com/sourceLocation":{"file":"acme.go","line":100,"function":"main"},` +
				`"logging.googleapis.com/spanId":"000000000001","logging.googleapis.com/trace":"projects/P/ABCD12345678AB12345678","logging.googleapis.com/trace_sampled":true}`,
		},
		{
			name: "error on redirect with proto payload",
			in: &logpb.LogEntry{
				Timestamp: &timestamppb.Timestamp{
					Seconds: 1641024000,
				},
				Severity: logtypepb.LogSeverity_DEBUG,
				Payload: &logpb.LogEntry_ProtoPayload{
					ProtoPayload: &anypb.Any{},
				},
			},
			wantError: ErrRedirectProtoPayloadNotSupported,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			buffer := &strings.Builder{}
			err := printEntryToWriter(tc.in, buffer)
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
				if got != tc.want {
					t.Errorf("printEntryToWriter: %+v: got %v, want %v", tc.in, got, tc.want)
				}
			}
		})
	}
}

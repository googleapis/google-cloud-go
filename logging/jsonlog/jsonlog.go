// Copyright 2021 Google LLC
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

// Package jsonlog provides a Logger that logs structured JSON to Stderr by
// default. When used on the various Cloud Compute environments (Cloud Run,
// Cloud Functions, GKE, etc.) these JSON messages will be parsed by the Cloud
// Logging agent and transformed into a message format that mirrors that of the
// Cloud Logging API.
package jsonlog

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"

	"cloud.google.com/go/logging"
	"cloud.google.com/go/logging/internal"
	logtypepb "google.golang.org/genproto/googleapis/logging/type"
	logpb "google.golang.org/genproto/googleapis/logging/v2"
	"google.golang.org/protobuf/encoding/protojson"
)

// NewLogger creates a Logger that logs structured JSON to Stderr. The value of
// parent must be in the format of:
//    projects/PROJECT_ID
//    folders/FOLDER_ID
//    billingAccounts/ACCOUNT_ID
//    organizations/ORG_ID
func NewLogger(parent string, opts ...LoggerOption) (*Logger, error) {
	return NewLoggerFromRequest(parent, nil, opts...)
}

// NewLoggerFromRequest creates a Logger similar to NewLogger but with the
// additional context that the current request can provide.
// The value of parent must be in the format of:
//    projects/PROJECT_ID
//    folders/FOLDER_ID
//    billingAccounts/ACCOUNT_ID
//    organizations/ORG_ID
func NewLoggerFromRequest(parent string, r *http.Request, opts ...LoggerOption) (*Logger, error) {
	err := validateParent(parent)
	if err != nil {
		return nil, err
	}
	var req *logtypepb.HttpRequest
	if r != nil {
		u := *r.URL
		req = &logtypepb.HttpRequest{
			RequestMethod: r.Method,
			RequestUrl:    internal.FixUTF8(u.String()),
			UserAgent:     r.UserAgent(),
			Referer:       r.Referer(),
			Protocol:      r.Proto,
		}
		if r.Response != nil {
			req.Status = int32(r.Response.StatusCode)
		}
	}
	l := &Logger{
		w:   os.Stderr,
		req: req,
	}

	var traceHeader string
	if r != nil && r.Header != nil {
		traceHeader = r.Header.Get("X-Cloud-Trace-Context")
	}
	if traceHeader != "" {
		traceID, spanID, traceSampled := internal.DeconstructXCloudTraceContext(traceHeader)
		l.traceID = fmt.Sprintf("%s/traces/%s", parent, traceID)
		l.spanID = spanID
		l.sampled = traceSampled
	}
	for _, opt := range opts {
		opt.set(l)
	}
	return l, nil
}

// Logger is used for logging JSON entries.
type Logger struct {
	w       io.Writer
	now     func() time.Time
	errhook func(error)

	// read-only fields
	labels  map[string]string
	req     *logtypepb.HttpRequest
	traceID string
	sampled bool
	spanID  string
}

// Debugf is a convenience method for writing an Entry with a Debug Severity
// and the provided formatted message.
func (l *Logger) Debugf(format string, a ...interface{}) {
	e := entry{
		Message:  fmt.Sprintf(format, a...),
		Severity: "DEBUG",
	}
	l.log(e)
}

// Infof is a convenience method for writing an Entry with a Debug Severity
// and the provided formatted message.
func (l *Logger) Infof(format string, a ...interface{}) {
	e := entry{
		Message:  fmt.Sprintf(format, a...),
		Severity: "INFO",
	}
	l.log(e)
}

// Noticef is a convenience method for writing an Entry with a Debug Severity
// and the provided formatted message.
func (l *Logger) Noticef(format string, a ...interface{}) {
	e := entry{
		Message:  fmt.Sprintf(format, a...),
		Severity: "NOTICE",
	}
	l.log(e)
}

// Warnf is a convenience method for writing an Entry with a Debug Severity
// and the provided formatted message.
func (l *Logger) Warnf(format string, a ...interface{}) {
	e := entry{
		Message:  fmt.Sprintf(format, a...),
		Severity: "WARNING",
	}
	l.log(e)
}

// Errorf is a convenience method for writing an Entry with a Debug Severity
// and the provided formatted message.
func (l *Logger) Errorf(format string, a ...interface{}) {
	e := entry{
		Message:  fmt.Sprintf(format, a...),
		Severity: "ERROR",
	}
	l.log(e)
}

// Criticalf is a convenience method for writing an Entry with a Debug Severity
// and the provided formatted message.
func (l *Logger) Criticalf(format string, a ...interface{}) {
	e := entry{
		Message:  fmt.Sprintf(format, a...),
		Severity: "CRITICAL",
	}
	l.log(e)
}

// Alertf is a convenience method for writing an Entry with a Debug Severity
// and the provided formatted message.
func (l *Logger) Alertf(format string, a ...interface{}) {
	e := entry{
		Message:  fmt.Sprintf(format, a...),
		Severity: "ALERT",
	}
	l.log(e)
}

// Emergencyf is a convenience method for writing an Entry with a Debug Severity
// and the provided formatted message.
func (l *Logger) Emergencyf(format string, a ...interface{}) {
	e := entry{
		Message:  fmt.Sprintf(format, a...),
		Severity: "EMERGENCY",
	}
	l.log(e)
}

// Log an Entry. Note that not all of the fields in entry will used when
// writting the log message, only those that are mentioned
// https://cloud.google.com/logging/docs/structured-logging will be logged.
func (l *Logger) Log(e logging.Entry) {
	le := entry{
		Severity:       e.Severity.String(),
		Labels:         e.Labels,
		InsertID:       e.InsertID,
		Operation:      e.Operation,
		SourceLocation: e.SourceLocation,
		SpanID:         e.SpanID,
		Trace:          e.Trace,
		TraceSampled:   e.TraceSampled,
	}
	if e.HTTPRequest != nil {
		le.HTTPRequest = toLogpbHTTPRequest(e.HTTPRequest.Request)
	}
	if !e.Timestamp.IsZero() {
		le.Timestamp = e.Timestamp.Format(time.RFC3339)
	}
	switch p := e.Payload.(type) {
	case string:
		le.Message = p
	default:
		s, err := internal.ToProtoStruct(p)
		if err != nil {
			if l.errhook != nil {
				l.errhook(err)
			}
			return
		}
		b, err := protojson.Marshal(s)
		if err != nil {
			if l.errhook != nil {
				l.errhook(err)
			}
			return
		}
		le.Message = string(b)
	}
	l.log(le)
}

func (l *Logger) log(e entry) {
	if e.Timestamp == "" && l.now != nil {
		e.Timestamp = l.now().Format(time.RFC3339)
	}
	if e.Trace == "" {
		e.Trace = l.traceID
	}
	if e.SpanID == "" {
		e.SpanID = l.spanID
	}
	if !e.TraceSampled {
		e.TraceSampled = l.sampled
	}
	if e.HTTPRequest == nil && l.req != nil {
		e.HTTPRequest = l.req
	}
	if l.labels != nil {
		if e.Labels == nil {
			e.Labels = l.labels
		} else {
			for k, v := range l.labels {
				if _, ok := e.Labels[k]; !ok {
					e.Labels[k] = v
				}
			}
		}
	}
	if err := json.NewEncoder(l.w).Encode(e); err != nil && l.errhook != nil {
		l.errhook(err)
	}
}

// WithLabels creates a new JSONLogger based off an existing one. The labels
// provided will be added to the loggers existing labels, replacing any
// overlapping keys with the new values.
func (l *Logger) WithLabels(labels map[string]string) *Logger {
	new := &Logger{
		w:       l.w,
		req:     l.req,
		traceID: l.traceID,
		sampled: l.sampled,
		spanID:  l.spanID,
	}
	newLabels := make(map[string]string, len(new.labels))
	for k, v := range new.labels {
		newLabels[k] = v
	}
	for k, v := range labels {
		newLabels[k] = v
	}
	new.labels = newLabels
	return new
}

// validateParent checks to make sure name is in the format.
func validateParent(parent string) error {
	if !strings.HasPrefix(parent, "projects/") &&
		!strings.HasPrefix(parent, "folders/") &&
		!strings.HasPrefix(parent, "billingAccounts/") &&
		!strings.HasPrefix(parent, "organizations/") {
		return fmt.Errorf("jsonlog: name formatting incorrect")
	}
	return nil
}

// entry represents the fields of a logging.Entry that can be parsed by Logging
// agent. To see a list of these mappings see
// https://cloud.google.com/logging/docs/structured-logging.
type entry struct {
	Message        string                        `json:"message"`
	Severity       string                        `json:"severity,omitempty"`
	HTTPRequest    *logtypepb.HttpRequest        `json:"httpRequest,omitempty"`
	Timestamp      string                        `json:"timestamp,omitempty"`
	Labels         map[string]string             `json:"logging.googleapis.com/labels,omitempty"`
	InsertID       string                        `json:"logging.googleapis.com/insertId,omitempty"`
	Operation      *logpb.LogEntryOperation      `json:"logging.googleapis.com/operation,omitempty"`
	SourceLocation *logpb.LogEntrySourceLocation `json:"logging.googleapis.com/sourceLocation,omitempty"`
	SpanID         string                        `json:"logging.googleapis.com/spanId,omitempty"`
	Trace          string                        `json:"logging.googleapis.com/trace,omitempty"`
	TraceSampled   bool                          `json:"logging.googleapis.com/trace_sampled,omitempty"`
}

func toLogpbHTTPRequest(r *http.Request) *logtypepb.HttpRequest {
	if r == nil {
		return nil
	}
	u := *r.URL
	return &logtypepb.HttpRequest{
		RequestMethod: r.Method,
		RequestUrl:    internal.FixUTF8(u.String()),
		Status:        int32(r.Response.StatusCode),
		UserAgent:     r.UserAgent(),
		Referer:       r.Referer(),
		Protocol:      r.Proto,
	}
}

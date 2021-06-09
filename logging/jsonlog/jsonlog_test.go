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

package jsonlog

import (
	"bytes"
	"encoding/json"
	"net/http"
	"strings"
	"testing"
	"time"

	"cloud.google.com/go/logging"
	"cloud.google.com/go/logging/internal"
	"github.com/google/go-cmp/cmp"
	ltype "google.golang.org/genproto/googleapis/logging/type"
)

func TestLoggerLevel(t *testing.T) {
	now := time.Date(1977, time.May, 25, 0, 0, 0, 0, time.UTC)
	fnow := now.Format(time.RFC3339)
	tests := []struct {
		severity string
		lFn      func(*Logger)
	}{
		{severity: debugSeverity, lFn: func(l *Logger) { l.Debugf("Hello, World!") }},
		{severity: infoSeverity, lFn: func(l *Logger) { l.Infof("Hello, World!") }},
		{severity: noticeSeverity, lFn: func(l *Logger) { l.Noticef("Hello, World!") }},
		{severity: warnSeverity, lFn: func(l *Logger) { l.Warnf("Hello, World!") }},
		{severity: errorSeverity, lFn: func(l *Logger) { l.Errorf("Hello, World!") }},
		{severity: criticalSeverity, lFn: func(l *Logger) { l.Criticalf("Hello, World!") }},
		{severity: alertSeverity, lFn: func(l *Logger) { l.Alertf("Hello, World!") }},
		{severity: emergencySeverity, lFn: func(l *Logger) { l.Emergencyf("Hello, World!") }},
	}
	for _, tt := range tests {
		t.Run(tt.severity, func(t *testing.T) {
			buf := bytes.NewBuffer(nil)
			l, err := NewLogger("projects/test")
			if err != nil {
				t.Fatalf("unable to create logger: %v", err)
			}
			l.w = buf
			l.now = func() time.Time {
				return now
			}

			e := &entry{
				Message:   "Hello, World!",
				Timestamp: fnow,
			}
			e.Severity = tt.severity
			b, err := json.Marshal(e)
			if err != nil {
				t.Fatalf("unable to marshal: %v", err)
			}
			tt.lFn(l)
			if diff := cmp.Diff(string(b), strings.TrimSpace(buf.String())); diff != "" {
				t.Fatalf("Logger.Debugf() mismatch (-want +got):\n%s", diff)
			}
		})
	}

}

func TestLoggerWithLabels(t *testing.T) {
	l, err := NewLogger("projects/test/log123")
	if err != nil {
		t.Fatalf("unable to create logger: %v", err)
	}

	tests := []struct {
		name           string
		commonLabels   map[string]string
		entryLabels    map[string]string
		outputLabelLen int
	}{
		{
			name:           "empty labels",
			outputLabelLen: 0,
		},
		{
			name:           "two common labels",
			commonLabels:   map[string]string{"one": "foo", "two": "bar"},
			outputLabelLen: 2,
		},
		{
			name:           "two common labels and one entry",
			commonLabels:   map[string]string{"one": "foo", "two": "bar"},
			entryLabels:    map[string]string{"three": "baz"},
			outputLabelLen: 3,
		},
		{
			name:           "three unique labels with overlap",
			commonLabels:   map[string]string{"one": "foo", "two": "bar", "three": "baz"},
			entryLabels:    map[string]string{"three": "baz"},
			outputLabelLen: 3,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			buf := bytes.NewBuffer(nil)
			l.w = buf
			e := entry{}

			ll := l.WithLabels(tt.commonLabels)
			ll.Log(logging.Entry{
				Severity: logging.Info,
				Payload:  "test",
				Labels:   tt.entryLabels,
			})
			if err := json.Unmarshal(buf.Bytes(), &e); err != nil {
				t.Fatalf("unable to unmarshal data: %v", err)
			}
			if len(e.Labels) != tt.outputLabelLen {
				t.Fatalf("len(LogEntry.Labels) = %v, want %v", len(e.Labels), tt.outputLabelLen)
			}
		})
	}
}

func TestLoggerWithLabels_WithOverlap(t *testing.T) {
	l, err := NewLogger("projects/test")
	if err != nil {
		t.Fatalf("unable to create logger: %v", err)
	}
	buf := bytes.NewBuffer(nil)
	l.w = buf
	e := entry{}

	l = l.WithLabels(map[string]string{"one": "foo", "two": "bar", "three": "baz"})
	l.Log(logging.Entry{
		Severity: logging.Info,
		Payload:  "test",
		Labels:   map[string]string{"three": "newbaz"},
	})
	if err := json.Unmarshal(buf.Bytes(), &e); err != nil {
		t.Fatalf("unable to unmarshal data: %v", err)
	}
	if len(e.Labels) != 3 {
		t.Fatalf("len(LogEntry.Labels) = %v, want %v", len(e.Labels), 3)
	}
	if e.Labels["three"] != "newbaz" {
		t.Fatalf("le.Labels[\"three\"] = %q, want \"newbaz\"", e.Labels["three"])
	}
}

func TestLoggerWithLabels_Twice(t *testing.T) {
	l, err := NewLogger("projects/test/log123")
	if err != nil {
		t.Fatalf("unable to create logger: %v", err)
	}
	buf := bytes.NewBuffer(nil)
	l.w = buf
	e := entry{}

	l = l.WithLabels(map[string]string{"one": "foo", "two": "bar"})
	l = l.WithLabels(map[string]string{"one": "foo", "two": "newbar", "three": "baz"})
	l.Log(logging.Entry{
		Severity: logging.Info,
		Payload:  "test",
		Labels:   map[string]string{"three": "newbaz"},
	})
	if err := json.Unmarshal(buf.Bytes(), &e); err != nil {
		t.Fatalf("unable to unmarshal data: %v", err)
	}
	if len(e.Labels) != 3 {
		t.Fatalf("len(LogEntry.Labels) = %v, want %v", len(e.Labels), 3)
	}
	if e.Labels["three"] != "newbaz" {
		t.Fatalf("le.Labels[\"three\"] = %q, want \"newbaz\"", e.Labels["three"])
	}
	if e.Labels["two"] != "newbar" {
		t.Fatalf("le.Labels[\"two\"] = %q, want \"newbar\"", e.Labels["two"])
	}
}

func TestParseName(t *testing.T) {
	tests := []struct {
		name        string
		input       string
		wantParent  string
		wantLogName string
		wantErr     bool
	}{
		{
			name:    "valid project style",
			input:   "projects/test",
			wantErr: false,
		},
		{
			name:    "valid folder style",
			input:   "folders/tes",
			wantErr: false,
		},
		{
			name:    "valid billing style",
			input:   "billingAccounts/tes",
			wantErr: false,
		},
		{
			name:    "valid org style",
			input:   "organizations/tes",
			wantErr: false,
		},
		{
			name:    "invalid parent",
			input:   "blah/blah",
			wantErr: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotErr := validateParent(tt.input)
			if (tt.wantErr && gotErr == nil) || (!tt.wantErr && gotErr != nil) {
				t.Errorf("err: got %v, want %v", gotErr, tt.wantErr)
			}
		})
	}
}

func TestLogger_WithRequest(t *testing.T) {
	now := time.Date(1977, time.May, 25, 0, 0, 0, 0, time.UTC)
	fnow := now.Format(time.RFC3339)

	t.Run("basic, without trace info", func(t *testing.T) {
		r, err := http.NewRequest("Get", "http://example.com", nil)
		if err != nil {
			t.Fatalf("unable to create request: %v", err)
		}
		l, err := NewLogger("projects/test/log123")
		if err != nil {
			t.Fatalf("unable to create logger: %v", err)
		}
		l = l.WithRequest(r)
		buf := bytes.NewBuffer(nil)
		l.w = buf
		l.now = func() time.Time {
			return now
		}
		e := entry{
			Message:   "Hello, World!",
			Severity:  "INFO",
			Timestamp: fnow,
			HTTPRequest: &ltype.HttpRequest{
				RequestMethod: "Get",
				RequestUrl:    "http://example.com",
				Protocol:      "HTTP/1.1",
			},
		}

		l.Infof("Hello, World!")
		b, err := json.Marshal(e)
		if err != nil {
			t.Fatalf("unable to marshal: %v", err)
		}
		if diff := cmp.Diff(string(b), strings.TrimSpace(buf.String())); diff != "" {
			t.Fatalf("Logger.Warnf() mismatch (-want +got):\n%s", diff)
		}
	})

	t.Run("with trace header", func(t *testing.T) {
		r, err := http.NewRequest("Get", "http://example.com", nil)
		if err != nil {
			t.Fatalf("unable to create request: %v", err)
		}
		r.Header.Set(internal.TraceHeader, "105445aa7843bc8bf206b120001000/1;o=1")
		l, err := NewLogger("projects/test")
		if err != nil {
			t.Fatalf("unable to create logger: %v", err)
		}
		l = l.WithRequest(r)
		buf := bytes.NewBuffer(nil)
		l.w = buf
		l.now = func() time.Time {
			return now
		}
		e := entry{
			Message:   "Hello, World!",
			Severity:  "INFO",
			Timestamp: fnow,
			HTTPRequest: &ltype.HttpRequest{
				RequestMethod: "Get",
				RequestUrl:    "http://example.com",
				Protocol:      "HTTP/1.1",
			},
			Trace:        "projects/test/traces/105445aa7843bc8bf206b120001000",
			SpanID:       "1",
			TraceSampled: true,
		}

		l.Infof("Hello, World!")
		b, err := json.Marshal(e)
		if err != nil {
			t.Fatalf("unable to marshal: %v", err)
		}
		if diff := cmp.Diff(string(b), strings.TrimSpace(buf.String())); diff != "" {
			t.Fatalf("Logger.Warnf() mismatch (-want +got):\n%s", diff)
		}
	})
}

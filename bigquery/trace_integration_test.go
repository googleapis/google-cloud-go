// Copyright 2022 Google LLC
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

package bigquery

import (
	"context"
	"strings"
	"testing"
	"time"

	"go.opencensus.io/trace"
)

// testExporter is a testing exporter for validating captured spans.
type testExporter struct {
	spans []*trace.SpanData
}

func (te *testExporter) ExportSpan(s *trace.SpanData) {
	te.spans = append(te.spans, s)
}

// hasSpans checks that the exporter has all the span names
// specified in the slice.  It returns the unmatched names.
func (te *testExporter) hasSpans(names []string) []string {
	matches := make(map[string]struct{})
	for _, n := range names {
		matches[n] = struct{}{}
	}
	for _, s := range te.spans {
		delete(matches, s.Name)
	}
	var unmatched []string
	for k := range matches {
		unmatched = append(unmatched, k)
	}
	return unmatched
}

func TestIntegration_Tracing(t *testing.T) {
	if client == nil {
		t.Skip("Integration tests skipped")
	}

	ctx := context.Background()

	for _, tc := range []struct {
		description string
		callF       func(ctx context.Context)
		wantSpans   []string
	}{
		{
			description: "fast path query",
			callF: func(ctx context.Context) {
				client.Query("SELECT SESSION_USER()").Read(ctx)
			},
			wantSpans: []string{"bigquery.jobs.query", "cloud.google.com/go/bigquery.Query.Run"},
		},
		{
			description: "slow path query",
			callF: func(ctx context.Context) {
				q := client.Query("SELECT SESSION_USER()")
				q.JobTimeout = time.Hour
				q.Read(ctx)
			},
			wantSpans: []string{"bigquery.jobs.insert", "bigquery.jobs.getQueryResults", "cloud.google.com/go/bigquery.Job.Read", "cloud.google.com/go/bigquery.Query.Run"},
		},
		{
			description: "table metadata",
			callF: func(ctx context.Context) {
				client.DatasetInProject("bigquery-public-data", "samples").Table("shakespeare").Metadata(ctx)
			},
			wantSpans: []string{"bigquery.tables.get", "cloud.google.com/go/bigquery.Table.Metadata"},
		},
	} {
		exporter := &testExporter{}
		trace.RegisterExporter(exporter)
		traceCtx, span := trace.StartSpan(ctx, "testspan", trace.WithSampler(trace.AlwaysSample()))
		tc.callF(traceCtx)
		span.End()
		trace.UnregisterExporter(exporter)

		if unmatched := exporter.hasSpans(tc.wantSpans); len(unmatched) > 0 {
			t.Errorf("case (%s): unmatched spans: %s", tc.description, strings.Join(unmatched, ","))
		}
	}
}

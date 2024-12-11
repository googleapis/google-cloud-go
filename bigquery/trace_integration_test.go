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
	"sync"
	"testing"
	"time"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/sdk/trace"
)

// testExporter is a testing exporter for validating captured spans.
type testExporter struct {
	mu    sync.Mutex
	spans []trace.ReadOnlySpan
}

func (te *testExporter) ExportSpans(ctx context.Context, spans []trace.ReadOnlySpan) error {
	te.mu.Lock()
	defer te.mu.Unlock()
	te.spans = append(te.spans, spans...)
	return nil
}

// Satisfy the exporter contract.  This method does nothing.
func (te *testExporter) Shutdown(ctx context.Context) error {
	return nil
}

// hasSpans checks that the exporter has all the span names
// specified in the slice.  It returns the unmatched names.
func (te *testExporter) hasSpans(names []string) []string {
	matches := make(map[string]struct{})
	for _, n := range names {
		matches[n] = struct{}{}
	}
	for _, s := range te.spans {
		delete(matches, s.Name())
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
		{
			description: "dataset and table insert/update/delete",
			callF: func(ctx context.Context) {
				ds := client.Dataset(datasetIDs.New())
				if err := ds.Create(ctx, nil); err != nil {
					t.Fatalf("creating dataset %s: %v", ds.DatasetID, err)
				}
				defer ds.DeleteWithContents(ctx)
				tbl := ds.Table(tableIDs.New())
				tm := &TableMetadata{
					Schema: schema,
				}
				if err := tbl.Create(ctx, tm); err != nil {
					t.Fatalf("creating table %s: %v", tbl.TableID, err)
				}
				defer tbl.Delete(ctx)
				md := TableMetadataToUpdate{
					Description: "new description",
				}
				if _, err := tbl.Update(ctx, md, ""); err != nil {
					t.Fatalf("updating table %s: %v", tbl.TableID, err)
				}
			},
			wantSpans: []string{
				"cloud.google.com/go/bigquery.Dataset.Create",
				"bigquery.tables.insert", "cloud.google.com/go/bigquery.Table.Create",
				"bigquery.tables.patch", "cloud.google.com/go/bigquery.Table.Update",
				"bigquery.datasets.delete", "cloud.google.com/go/bigquery.Dataset.Delete",
				"bigquery.tables.delete", "cloud.google.com/go/bigquery.Table.Delete",
			},
		},
	} {
		t.Run(tc.description, func(t *testing.T) {
			exporter := &testExporter{}
			bsp := trace.NewBatchSpanProcessor(exporter)
			tp := trace.NewTracerProvider(
				trace.WithSampler(trace.AlwaysSample()),
				trace.WithSpanProcessor(bsp),
			)
			otel.SetTracerProvider(tp)

			tracer := tp.Tracer("test-trace")
			traceCtx, span := tracer.Start(ctx, "startspan")
			// Invoke the func to be traced.
			tc.callF(traceCtx)
			span.End()
			tp.Shutdown(traceCtx)

			if unmatched := exporter.hasSpans(tc.wantSpans); len(unmatched) > 0 {
				t.Errorf("case (%s): unmatched spans: %s", tc.description, strings.Join(unmatched, ","))
			}
		})
	}
}

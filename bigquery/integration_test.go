// Copyright 2015 Google Inc. All Rights Reserved.
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
	"fmt"
	"net/http"
	"reflect"
	"sort"
	"strings"
	"testing"
	"time"

	"cloud.google.com/go/internal/pretty"
	"cloud.google.com/go/internal/testutil"
	"golang.org/x/net/context"
	"google.golang.org/api/googleapi"
	"google.golang.org/api/iterator"
	"google.golang.org/api/option"
)

func TestIntegration(t *testing.T) {
	if testing.Short() {
		t.Skip("Integration tests skipped in short mode")
	}

	ctx := context.Background()
	ts := testutil.TokenSource(ctx, Scope)
	if ts == nil {
		t.Skip("Integration tests skipped. See CONTRIBUTING.md for details")
	}

	projID := testutil.ProjID()
	c, err := NewClient(ctx, projID, option.WithTokenSource(ts))
	if err != nil {
		t.Fatal(err)
	}
	ds := c.Dataset("bigquery_integration_test")
	if err := ds.Create(ctx); err != nil && !hasStatusCode(err, http.StatusConflict) { // AlreadyExists is 409
		t.Fatal(err)
	}
	schema := Schema([]*FieldSchema{
		{Name: "name", Type: StringFieldType},
		{Name: "num", Type: IntegerFieldType},
	})
	table := ds.Table("t1")
	// Delete the table in case it already exists. (Ignore errors.)
	table.Delete(ctx)
	// Create the table.
	err = table.Create(ctx, schema, TableExpiration(time.Now().Add(5*time.Minute)))
	if err != nil {
		t.Fatal(err)
	}
	// Check table metadata.
	md, err := table.Metadata(ctx)
	if err != nil {
		t.Fatal(err)
	}
	// TODO(jba): check md more thorougly.
	if got, want := md.ID, fmt.Sprintf("%s:%s.%s", projID, ds.DatasetID, table.TableID); got != want {
		t.Errorf("metadata.ID: got %q, want %q", got, want)
	}
	if got, want := md.Type, RegularTable; got != want {
		t.Errorf("metadata.Type: got %v, want %v", got, want)
	}

	// Iterate over tables in the dataset.
	it := ds.Tables(ctx)
	var tables []*Table
	for {
		tbl, err := it.Next()
		if err == iterator.Done {
			break
		}
		if err != nil {
			t.Fatal(err)
		}
		tables = append(tables, tbl)
	}
	if got, want := tables, []*Table{table}; !reflect.DeepEqual(got, want) {
		t.Errorf("Tables: got %v, want %v", pretty.Value(got), pretty.Value(want))
	}

	// Populate the table.
	upl := table.Uploader()
	var rows []*ValuesSaver
	for i, name := range []string{"a", "b", "c"} {
		rows = append(rows, &ValuesSaver{
			Schema:   schema,
			InsertID: name,
			Row:      []Value{name, i},
		})
	}
	if err := upl.Put(ctx, rows); err != nil {
		t.Fatal(err)
	}

	checkRead := func(msg string, it *RowIterator) {
		gotRows, err := readAll(it)
		if err != nil {
			t.Fatal(err)
		}
		if len(gotRows) != len(rows) {
			t.Errorf("%s: got %d rows, want %d", msg, len(gotRows), len(rows))
		}
		sort.Sort(byCol0(gotRows))
		for i, got := range gotRows {
			got := []Value(got)
			want := rows[i].Row
			if !reflect.DeepEqual(got, want) {
				t.Errorf("%s #%d: got %v, want %v", msg, i, got, want)
			}
		}
	}

	// Wait until the data has been uploaded. This can take a few seconds, according
	// to https://cloud.google.com/bigquery/streaming-data-into-bigquery.
	for {
		it := table.Read(ctx)
		var v ValueList
		err := it.Next(&v)
		if err == nil {
			break
		}
		if err != iterator.Done {
			t.Fatal(err)
		}
		time.Sleep(1 * time.Second)
	}

	// Read the table.
	checkRead("upload", table.Read(ctx))

	// Query the table.
	q := c.Query("select name, num from t1")
	q.DefaultProjectID = projID
	q.DefaultDatasetID = ds.DatasetID

	rit, err := q.Read(ctx)
	if err != nil {
		t.Fatal(err)
	}
	checkRead("query", rit)

	// Query the long way.
	job1, err := q.Run(ctx)
	if err != nil {
		t.Fatal(err)
	}
	job2, err := c.JobFromID(ctx, job1.ID())
	if err != nil {
		t.Fatal(err)
	}
	// TODO(jba): poll status until job is done
	_, err = job2.Status(ctx)
	if err != nil {
		t.Fatal(err)
	}

	rit, err = job2.Read(ctx)
	if err != nil {
		t.Fatal(err)
	}
	checkRead("job.Read", rit)

	// Test Update.
	tm, err := table.Metadata(ctx)
	if err != nil {
		t.Fatal(err)
	}
	wantDescription := tm.Description + "more"
	wantName := tm.Name + "more"
	got, err := table.Update(ctx, TableMetadataToUpdate{
		Description: wantDescription,
		Name:        wantName,
	})
	if err != nil {
		t.Fatal(err)
	}
	if got.Description != wantDescription {
		t.Errorf("Description: got %q, want %q", got.Description, wantDescription)
	}
	if got.Name != wantName {
		t.Errorf("Name: got %q, want %q", got.Name, wantName)
	}

	// Load the table from a reader.
	r := strings.NewReader("a,0\nb,1\nc,2\n")
	rs := NewReaderSource(r)
	loader := table.LoaderFrom(rs)
	loader.WriteDisposition = WriteTruncate
	job, err := loader.Run(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if err := wait(ctx, job); err != nil {
		t.Fatal(err)
	}
	checkRead("reader load", table.Read(ctx))

	// Use DML to insert.
	// We can't use WriteDisposition to truncate, and CreateIfNeeded doesn't work.
	// So delete and re-create the table.
	if err := table.Delete(ctx); err != nil {
		t.Fatal(err)
	}
	if err := table.Create(ctx, schema, TableExpiration(time.Now().Add(5*time.Minute))); err != nil {
		t.Fatal(err)
	}
	q = c.Query("INSERT bigquery_integration_test.t1 (name, num) VALUES ('a', 0), ('b', 1), ('c', 2)")
	q.UseStandardSQL = true // necessary for DML
	job, err = q.Run(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if err := wait(ctx, job); err != nil {
		t.Fatal(err)
	}
	checkRead("INSERT", table.Read(ctx))
}

func hasStatusCode(err error, code int) bool {
	if e, ok := err.(*googleapi.Error); ok && e.Code == code {
		return true
	}
	return false
}

// wait polls the job until it is complete or an error is returned.
func wait(ctx context.Context, job *Job) error {
	for {
		status, err := job.Status(ctx)
		if err != nil {
			return fmt.Errorf("getting job status: %v", err)
		}
		if status.Done() {
			if status.Err() != nil {
				return fmt.Errorf("job status: %#v", status.Err())
			}
			return nil
		}
		time.Sleep(1 * time.Second)
	}
}

func readAll(it *RowIterator) ([]ValueList, error) {
	var rows []ValueList
	for {
		var vals ValueList
		err := it.Next(&vals)
		if err == iterator.Done {
			return rows, nil
		}
		if err != nil {
			return nil, err
		}
		rows = append(rows, vals)
	}
}

type byCol0 []ValueList

func (b byCol0) Len() int           { return len(b) }
func (b byCol0) Swap(i, j int)      { b[i], b[j] = b[j], b[i] }
func (b byCol0) Less(i, j int) bool { return b[i][0].(string) < b[j][0].(string) }

// Copyright 2023 Google LLC
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
	"errors"
	"fmt"
	"testing"
	"time"

	"cloud.google.com/go/internal/testutil"
	"google.golang.org/api/iterator"
)

func TestIntegration_StorageReadBasicTypes(t *testing.T) {
	if client == nil {
		t.Skip("Integration tests skipped")
	}
	ctx := context.Background()

	initQueryParameterTestCases()

	for _, c := range queryParameterTestCases {
		q := storageOptimizedClient.Query(c.query)
		q.Parameters = c.parameters
		q.forceStorageAPI = true
		it, err := q.Read(ctx)
		if err != nil {
			t.Fatal(err)
		}
		err = checkIteratorRead(it, c.wantRow)
		if err != nil {
			t.Fatalf("%s: error on query `%s`[%v]: %v", it.SourceJob().ID(), c.query, c.parameters, err)
		}
		if !it.IsAccelerated() {
			t.Fatalf("%s: expected storage api to be used", it.SourceJob().ID())
		}
	}
}

func TestIntegration_StorageReadEmptyResultSet(t *testing.T) {
	if client == nil {
		t.Skip("Integration tests skipped")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	table := storageOptimizedClient.Dataset(dataset.DatasetID).Table(tableIDs.New())
	err := table.Create(ctx, &TableMetadata{
		Schema: Schema{
			{Name: "name", Type: StringFieldType, Required: true},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	defer table.Delete(ctx)

	it := table.Read(ctx)
	err = checkIteratorRead(it, []Value{})
	if err != nil {
		t.Fatalf("failed to read empty table: %v", err)
	}
	if !it.IsAccelerated() {
		t.Fatal("expected storage api to be used")
	}
}

func TestIntegration_StorageReadFromSources(t *testing.T) {
	if client == nil {
		t.Skip("Integration tests skipped")
	}
	ctx := context.Background()

	dstTable := dataset.Table(tableIDs.New())
	dstTable.c = storageOptimizedClient

	sql := `SELECT 1 as num, 'one' as str 
UNION ALL 
SELECT 2 as num, 'two' as str 
UNION ALL 
SELECT 3 as num, 'three' as str 
ORDER BY num`
	q := storageOptimizedClient.Query(sql)
	q.Dst = dstTable
	job, err := q.Run(ctx)
	if err != nil {
		t.Fatal(err)
	}
	status, err := job.Wait(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if err := status.Err(); err != nil {
		t.Fatal(err)
	}
	expectedRows := [][]Value{
		{int64(1), "one"},
		{int64(2), "two"},
		{int64(3), "three"},
	}
	tableRowIt := dstTable.Read(ctx)
	if err = checkRowsRead(tableRowIt, expectedRows); err != nil {
		t.Fatalf("checkRowsRead(table): %v", err)
	}
	if !tableRowIt.IsAccelerated() {
		t.Fatalf("reading from table should use Storage API")
	}
	jobRowIt, err := job.Read(ctx)
	if err != nil {
		t.Fatalf("ReadJobResults(job): %v", err)
	}
	if err = checkRowsRead(jobRowIt, expectedRows); err != nil {
		t.Fatalf("checkRowsRead(job): %v", err)
	}
	if !jobRowIt.IsAccelerated() {
		t.Fatalf("reading job should use Storage API")
	}
	q.Dst = nil
	q.forceStorageAPI = true
	qRowIt, err := q.Read(ctx)
	if err != nil {
		t.Fatalf("ReadQuery(query): %v", err)
	}
	if !qRowIt.IsAccelerated() {
		t.Fatalf("reading query should use Storage API")
	}
	if err = checkRowsRead(qRowIt, expectedRows); err != nil {
		t.Fatalf("checkRowsRead(query): %v", err)
	}
}

func TestIntegration_StorageReadScriptJob(t *testing.T) {
	if client == nil {
		t.Skip("Integration tests skipped")
	}
	tableID := tableIDs.New()
	ctx := context.Background()

	sql := fmt.Sprintf(`
-- Statement 0
DECLARE x INT64;
SET x = 4;
-- Statement 1
SELECT 1 as foo;
-- Statement 2
SELECT 1 as num, 'one' as str 
UNION ALL 
SELECT 2 as num, 'two' as str;
-- Statement 3
SELECT 1 as num, 'one' as str 
UNION ALL 
SELECT 2 as num, 'two' as str 
UNION ALL 
SELECT 3 as num, 'three' as str 
UNION ALL 
SELECT x as num, 'four' as str 
ORDER BY num;
-- Statement 4
CREATE TABLE %s.%s ( num INT64, str STRING );
-- Statement 5
DROP TABLE %s.%s;
`, dataset.DatasetID, tableID, dataset.DatasetID, tableID)
	q := storageOptimizedClient.Query(sql)
	q.forceStorageAPI = true
	it, err := q.Read(ctx)
	if err != nil {
		t.Fatal(err)
	}
	expectedRows := [][]Value{
		{int64(1), "one"},
		{int64(2), "two"},
		{int64(3), "three"},
		{int64(4), "four"},
	}
	if err = checkRowsRead(it, expectedRows); err != nil {
		t.Fatalf("checkRowsRead(it): %v", err)
	}
	if !it.IsAccelerated() {
		t.Fatalf("reading job should use Storage API")
	}
}

func TestIntegration_StorageReadQueryOrdering(t *testing.T) {
	if client == nil {
		t.Skip("Integration tests skipped")
	}
	ctx := context.Background()

	table := "`bigquery-public-data.usa_names.usa_1910_current`"
	testCases := []struct {
		name               string
		query              string
		maxExpectedStreams int
	}{
		{
			name:               "Non_Ordered_Query",
			query:              fmt.Sprintf(`SELECT name, number, state FROM %s`, table),
			maxExpectedStreams: -1, // No limit
		},
		{
			name:               "Ordered_Query",
			query:              fmt.Sprintf(`SELECT name, number, state FROM %s order by name`, table),
			maxExpectedStreams: 1,
		},
	}

	type S struct {
		Name   string
		Number int
		State  string
	}

	for _, tc := range testCases {
		q := storageOptimizedClient.Query(tc.query)
		q.forceStorageAPI = true

		it, err := q.Read(ctx)
		if err != nil {
			t.Fatal(err)
		}

		total, err := countIteratorRows(it)
		if err != nil {
			t.Fatal(err)
		}
		bqSession := it.arrowIterator.session.bqSession
		if len(bqSession.Streams) == 0 {
			t.Fatalf("%s: expected to use at least one stream but found %d", tc.name, len(bqSession.Streams))
		}
		streamSettings := it.arrowIterator.session.settings.maxStreamCount
		if tc.maxExpectedStreams > 0 {
			if streamSettings > tc.maxExpectedStreams {
				t.Fatalf("%s: expected stream settings to be at most %d streams but found %d", tc.name, tc.maxExpectedStreams, streamSettings)
			}
			if len(bqSession.Streams) > tc.maxExpectedStreams {
				t.Fatalf("%s: expected server to set up at most %d streams but found %d", tc.name, tc.maxExpectedStreams, len(bqSession.Streams))
			}
		} else {
			if streamSettings != 0 {
				t.Fatalf("%s: expected stream settings to be 0 (server defines amount of stream) but found %d", tc.name, streamSettings)
			}
		}
		if total != it.TotalRows {
			t.Fatalf("%s: should have read %d rows, but read %d", tc.name, it.TotalRows, total)
		}
		if !it.IsAccelerated() {
			t.Fatalf("%s: expected query to be accelerated by Storage API", tc.name)
		}
	}
}

func TestIntegration_StorageReadQueryMorePages(t *testing.T) {
	if client == nil {
		t.Skip("Integration tests skipped")
	}
	ctx := context.Background()
	table := "`bigquery-public-data.samples.github_timeline`"
	sql := fmt.Sprintf(`SELECT repository_url as url, repository_owner as owner, repository_forks as forks FROM %s`, table)
	// Don't forceStorageAPI usage and still see internally Storage API is selected
	q := storageOptimizedClient.Query(sql)
	q.DisableQueryCache = true
	it, err := q.Read(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if !it.IsAccelerated() {
		t.Fatal("expected query to use Storage API")
	}

	type S struct {
		URL   NullString
		Owner NullString
		Forks NullInt64
	}

	total, err := countIteratorRows(it)
	if err != nil {
		t.Fatal(err)
	}
	bqSession := it.arrowIterator.session.bqSession
	if len(bqSession.Streams) == 0 {
		t.Fatalf("should use more than one stream but found %d", len(bqSession.Streams))
	}
	if total != it.TotalRows {
		t.Fatalf("should have read %d rows, but read %d", it.TotalRows, total)
	}
}

func TestIntegration_StorageReadCancel(t *testing.T) {
	if client == nil {
		t.Skip("Integration tests skipped")
	}
	ctx := context.Background()
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	table := "`bigquery-public-data.samples.github_timeline`"
	sql := fmt.Sprintf(`SELECT repository_url as url, repository_owner as owner, repository_forks as forks FROM %s`, table)
	storageOptimizedClient.rc.settings.maxWorkerCount = 1
	q := storageOptimizedClient.Query(sql)
	q.DisableQueryCache = true
	q.forceStorageAPI = true
	it, err := q.Read(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if !it.IsAccelerated() {
		t.Fatal("expected query to use Storage API")
	}

	// Cancel read after readings 1000 rows
	rowsRead := 0
	for {
		var dst []Value
		err := it.Next(&dst)
		if err == iterator.Done {
			break
		}
		if err != nil {
			if errors.Is(err, context.DeadlineExceeded) ||
				errors.Is(err, context.Canceled) {
				break
			}
			t.Fatalf("failed to fetch via storage API: %v", err)
		}
		rowsRead++
		if rowsRead > 1000 {
			cancel()
		}
	}
	// resources are cleaned asynchronously
	time.Sleep(time.Second)
	if !it.arrowIterator.isDone() {
		t.Fatal("expected stream to be done")
	}
}

func countIteratorRows(it *RowIterator) (total uint64, err error) {
	for {
		var dst []Value
		err := it.Next(&dst)
		if err == iterator.Done {
			break
		}
		if err != nil {
			return total, fmt.Errorf("failed to fetch via storage API: %w", err)
		}
		total++
	}
	return total, err
}

func checkRowsRead(it *RowIterator, expectedRows [][]Value) error {
	if int(it.TotalRows) != len(expectedRows) {
		return fmt.Errorf("expected %d rows, found %d", len(expectedRows), it.TotalRows)
	}
	for _, row := range expectedRows {
		err := checkIteratorRead(it, row)
		if err != nil {
			return err
		}
	}
	return nil
}

func checkIteratorRead(it *RowIterator, expectedRow []Value) error {
	var outRow []Value
	err := it.Next(&outRow)
	if err == iterator.Done {
		return nil
	}
	if err != nil {
		return fmt.Errorf("failed to fetch via storage API: %v", err)
	}
	if len(outRow) != len(expectedRow) {
		return fmt.Errorf("expected %d columns, but got %d", len(expectedRow), len(outRow))
	}
	if !testutil.Equal(outRow, expectedRow) {
		return fmt.Errorf("got %v, want %v", outRow, expectedRow)
	}
	return nil
}

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
	"github.com/apache/arrow/go/v14/arrow"
	"github.com/apache/arrow/go/v14/arrow/array"
	"github.com/apache/arrow/go/v14/arrow/ipc"
	"github.com/apache/arrow/go/v14/arrow/math"
	"github.com/apache/arrow/go/v14/arrow/memory"
	"github.com/google/go-cmp/cmp"
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

		var firstValue S
		err = it.Next(&firstValue)
		if err != nil {
			t.Fatal(err)
		}

		if cmp.Equal(firstValue, S{}) {
			t.Fatalf("user defined struct was not filled with data")
		}

		total, err := countIteratorRows(it)
		if err != nil {
			t.Fatal(err)
		}
		total++ // as we read the first value separately

		session := it.arrowIterator.(*storageArrowIterator).session
		bqSession := session.bqSession
		if len(bqSession.Streams) == 0 {
			t.Fatalf("%s: expected to use at least one stream but found %d", tc.name, len(bqSession.Streams))
		}
		streamSettings := session.settings.maxStreamCount
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

func TestIntegration_StorageReadQueryStruct(t *testing.T) {
	if client == nil {
		t.Skip("Integration tests skipped")
	}
	ctx := context.Background()
	table := "`bigquery-public-data.samples.wikipedia`"
	sql := fmt.Sprintf(`SELECT id, title, timestamp, comment FROM %s LIMIT 1000`, table)
	q := storageOptimizedClient.Query(sql)
	q.forceStorageAPI = true
	q.DisableQueryCache = true
	it, err := q.Read(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if !it.IsAccelerated() {
		t.Fatal("expected query to use Storage API")
	}

	type S struct {
		ID        int64
		Title     string
		Timestamp int64
		Comment   NullString
	}

	total := uint64(0)
	for {
		var dst S
		err := it.Next(&dst)
		if err == iterator.Done {
			break
		}
		if err != nil {
			t.Fatalf("failed to fetch via storage API: %v", err)
		}
		if cmp.Equal(dst, S{}) {
			t.Fatalf("user defined struct was not filled with data")
		}
		total++
	}

	bqSession := it.arrowIterator.(*storageArrowIterator).session.bqSession
	if len(bqSession.Streams) == 0 {
		t.Fatalf("should use more than one stream but found %d", len(bqSession.Streams))
	}
	if total != it.TotalRows {
		t.Fatalf("should have read %d rows, but read %d", it.TotalRows, total)
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

	var firstValue S
	err = it.Next(&firstValue)
	if err != nil {
		t.Fatal(err)
	}

	if cmp.Equal(firstValue, S{}) {
		t.Fatalf("user defined struct was not filled with data")
	}

	total, err := countIteratorRows(it)
	if err != nil {
		t.Fatal(err)
	}
	total++ // as we read the first value separately

	bqSession := it.arrowIterator.(*storageArrowIterator).session.bqSession
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
	arrowIt := it.arrowIterator.(*storageArrowIterator)
	if !arrowIt.isDone() {
		t.Fatal("expected stream to be done")
	}
}

func TestIntegration_StorageReadArrow(t *testing.T) {
	if client == nil {
		t.Skip("Integration tests skipped")
	}
	ctx := context.Background()
	table := "`bigquery-public-data.usa_names.usa_1910_current`"
	sql := fmt.Sprintf(`SELECT name, number, state FROM %s where state = "CA"`, table)

	q := storageOptimizedClient.Query(sql)
	job, err := q.Run(ctx) // force usage of Storage API by skipping fast paths
	if err != nil {
		t.Fatal(err)
	}
	it, err := job.Read(ctx)
	if err != nil {
		t.Fatal(err)
	}

	checkedAllocator := memory.NewCheckedAllocator(memory.DefaultAllocator)
	it.arrowDecoder.allocator = checkedAllocator
	defer checkedAllocator.AssertSize(t, 0)

	arrowIt, err := it.ArrowIterator()
	if err != nil {
		t.Fatalf("expected iterator to be accelerated: %v", err)
	}
	arrowItReader := NewArrowIteratorReader(arrowIt)

	records := []arrow.Record{}
	r, err := ipc.NewReader(arrowItReader, ipc.WithAllocator(checkedAllocator))
	numrec := 0
	for r.Next() {
		rec := r.Record()
		rec.Retain()
		defer rec.Release()
		records = append(records, rec)
		numrec += int(rec.NumRows())
	}
	r.Release()

	arrowSchema := r.Schema()
	arrowTable := array.NewTableFromRecords(arrowSchema, records)
	defer arrowTable.Release()
	if arrowTable.NumRows() != int64(it.TotalRows) {
		t.Fatalf("should have a table with %d rows, but found %d", it.TotalRows, arrowTable.NumRows())
	}
	if arrowTable.NumCols() != 3 {
		t.Fatalf("should have a table with 3 columns, but found %d", arrowTable.NumCols())
	}

	sumSQL := fmt.Sprintf(`SELECT sum(number) as total FROM %s where state = "CA"`, table)
	sumQuery := client.Query(sumSQL)
	sumIt, err := sumQuery.Read(ctx)
	if err != nil {
		t.Fatal(err)
	}
	sumValues := []Value{}
	err = sumIt.Next(&sumValues)
	if err != nil {
		t.Fatal(err)
	}
	totalFromSQL := sumValues[0].(int64)

	tr := array.NewTableReader(arrowTable, arrowTable.NumRows())
	defer tr.Release()
	var totalFromArrow int64
	for tr.Next() {
		rec := tr.Record()
		vec := rec.Column(1).(*array.Int64)
		totalFromArrow += math.Int64.Sum(vec)
	}
	if totalFromArrow != totalFromSQL {
		t.Fatalf("expected total to be %d, but with arrow we got %d", totalFromSQL, totalFromArrow)
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

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
	"fmt"
	"sync"
	"testing"
	"time"

	"cloud.google.com/go/internal/testutil"
	"github.com/apache/arrow/go/v10/arrow"
	"github.com/apache/arrow/go/v10/arrow/array"
	"github.com/apache/arrow/go/v10/arrow/math"
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
		q.ForceStorageAPI = true
		it, err := q.Read(ctx)
		if err != nil {
			t.Fatal(err)
		}
		err = checkIteratorRead(it, c.wantRow)
		if err != nil {
			t.Fatalf("error on query `%s`[%v]: %v", c.query, c.parameters, err)
		}
		if it.arrowIterator == nil {
			t.Fatal("expected storage api to be used")
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
	if it.arrowIterator == nil {
		t.Fatal("expected storage api to be used")
	}
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
	if tableRowIt.arrowIterator == nil {
		t.Fatalf("reading from table should use Storage API")
	}
	jobRowIt, err := job.Read(ctx)
	if err != nil {
		t.Fatalf("ReadJobResults(job): %v", err)
	}
	if err = checkRowsRead(jobRowIt, expectedRows); err != nil {
		t.Fatalf("checkRowsRead(job): %v", err)
	}
	if jobRowIt.arrowIterator == nil {
		t.Fatalf("reading job should use Storage API")
	}
	q.Dst = nil
	q.ForceStorageAPI = true
	qRowIt, err := q.Read(ctx)
	if err != nil {
		t.Fatalf("ReadQuery(query): %v", err)
	}
	if qRowIt.arrowIterator == nil {
		t.Fatalf("reading query should use Storage API")
	}
	if err = checkRowsRead(qRowIt, expectedRows); err != nil {
		t.Fatalf("checkRowsRead(query): %v", err)
	}
}

func TestIntegration_StorageRawReadQuery(t *testing.T) {
	if client == nil {
		t.Skip("Integration tests skipped")
	}
	ctx := context.Background()
	table := "`bigquery-public-data.usa_names.usa_1910_current`"
	sql := fmt.Sprintf(`SELECT name, number, state FROM %s where state = "CA"`, table)
	q := storageOptimizedClient.Query(sql)

	s, err := storageOptimizedClient.rc.SessionForQuery(ctx, q, WithMaxStreamCount(0))
	if err != nil {
		t.Fatal(err)
	}
	err = s.Run()
	if err != nil {
		t.Fatal(err)
	}

	rows := []*RowStream{}
	wg := sync.WaitGroup{}
	info := s.Info()
	for _, readStream := range info.ReadStreams {
		wg.Add(1)
		go func(stream string) {
			rrows, err := consumeStream(s, stream)
			if err != nil {
				t.Logf("error consuming stream: %v", err)
				wg.Done()
				return
			}
			wg.Done()
			rows = append(rows, rrows...)
		}(readStream)
	}
	wg.Wait()

	meta, err := s.table.Metadata(ctx)
	if err != nil {
		t.Fatal(err)
	}
	schema := meta.Schema

	decoder, err := newArrowDecoderFromSession(s, schema)
	if err != nil {
		t.Fatal(err)
	}

	records := []arrow.Record{}
	for _, row := range rows {
		recs, err := decoder.decodeRetainedArrowRecords(row.SerializedArrowRecordBatch)
		if err != nil {
			t.Fatal(err)
		}
		records = append(records, recs...)
	}
	arrowSchema := decoder.arrowSchema
	if arrowSchema == nil {
		t.Fatal("should have Arrow table available, but nil found")
	}
	var arrowTable arrow.Table
	arrowTable = array.NewTableFromRecords(arrowSchema, records)
	defer arrowTable.Release()

	sumSQL := fmt.Sprintf(`SELECT sum(number) as total, count(*) as numRows FROM %s where state = "CA"`, table)
	sumQuery := storageOptimizedClient.Query(sumSQL)
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
	numRowsFromSQL := sumValues[1].(int64)

	if arrowTable.NumRows() != int64(numRowsFromSQL) {
		t.Fatalf("should have a table with %d rows, but found %d", numRowsFromSQL, arrowTable.NumRows())
	}
	if arrowTable.NumCols() != 3 {
		t.Fatalf("should have a table with 3 columns, but found %d", arrowTable.NumCols())
	}

	tr := array.NewTableReader(arrowTable, arrowTable.NumRows())
	var totalFromArrow int64
	for tr.Next() {
		rec := tr.Record()
		vec := array.NewInt64Data(rec.Column(1).Data())
		totalFromArrow += math.Int64.Sum(vec)
	}
	if totalFromArrow != totalFromSQL {
		t.Fatalf("expected total to be %d, but with arrow we got %d", totalFromSQL, totalFromArrow)
	}
}

func consumeRowStream(it *RowStreamIterator) ([]*RowStream, int64, error) {
	var rows []*RowStream
	var total int64
	for {
		row, err := it.Next()
		if err == iterator.Done {
			break
		}
		if err != nil {
			return nil, 0, err
		}
		if row.RowCount > 0 {
			total += row.RowCount
			rows = append(rows, row)
		}
	}
	if total == 0 {
		return nil, 0, iterator.Done
	}
	return rows, total, nil
}

func consumeStream(session *ReadSession, readStream string) ([]*RowStream, error) {
	var rows []*RowStream
	var offset int64
	for {
		it, err := session.ReadRows(ReadRowsRequest{
			ReadStream: readStream,
			Offset:     offset,
		})
		if err != nil {
			return nil, err
		}
		rrows, count, err := consumeRowStream(it)
		if err == iterator.Done {
			break
		}
		offset += count
		rows = append(rows, rrows...)
	}
	return rows, nil
}

func TestIntegration_StorageReadQueryOrdering(t *testing.T) {
	if client == nil {
		t.Skip("Integration tests skipped")
	}
	ctx := context.Background()
	table := "`bigquery-public-data.usa_names.usa_1910_current`"
	sql := fmt.Sprintf(`SELECT name, number, state FROM %s`, table)
	q := storageOptimizedClient.Query(sql)

	it, err := q.Read(ctx)
	if err != nil {
		t.Fatal(err)
	}
	type S struct {
		Name   string
		Number int
		State  string
	}

	var i uint64
	for {
		var s S
		err := it.Next(&s)
		if err == iterator.Done {
			break
		}
		if err != nil {
			t.Fatalf("failed to fetch via storage API: %v", err)
		}
		i++
	}
	t.Logf("%d lines read", i)
	info := it.arrowIterator.Session.Info()
	if len(info.ReadStreams) == 0 {
		t.Fatalf("should use more than one stream but found %d", len(info.ReadStreams))
	}
	if i != it.TotalRows {
		t.Fatalf("should have read %d rows, but read %d", it.TotalRows, i)
	}
	t.Logf("number of parallel streams for query `%s`: %d", q.Q, len(info.ReadStreams))
	t.Logf("bytes scanned for query `%s`: %d", q.Q, len(info.ReadStreams))

	orderedQ := storageOptimizedClient.Query(sql + " order by name")
	it, err = orderedQ.Read(ctx)
	if err != nil {
		t.Fatal(err)
	}
	i = 0
	for {
		var s S
		err := it.Next(&s)
		if err == iterator.Done {
			break
		}
		if err != nil {
			t.Fatalf("failed to fetch via storage API: %v", err)
		}
		i++
	}
	t.Logf("%d lines read", i)
	info = it.arrowIterator.Session.Info()
	if len(info.ReadStreams) > 1 {
		t.Fatalf("should use just one stream as is ordered, but found %d", len(info.ReadStreams))
	}
	if i != it.TotalRows {
		t.Fatalf("should have read %d rows, but read %d", it.TotalRows, i)
	}
	t.Logf("number of parallel streams for query `%s`: %d", orderedQ.Q, len(info.ReadStreams))
}

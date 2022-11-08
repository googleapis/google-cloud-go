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

package reader

import (
	"context"
	"flag"
	"fmt"
	"log"
	"math/big"
	"os"
	"testing"
	"time"

	"cloud.google.com/go/bigquery"
	"cloud.google.com/go/civil"
	"cloud.google.com/go/internal/testutil"
	"cloud.google.com/go/internal/uid"
	"github.com/apache/arrow/go/v10/arrow"
	"github.com/apache/arrow/go/v10/arrow/array"
	"github.com/apache/arrow/go/v10/arrow/math"
	"google.golang.org/api/iterator"
	"google.golang.org/api/option"
)

var (
	client               *bigquery.Client
	storageReadClient    *Client
	dataset              *bigquery.Dataset
	testTableExpiration  time.Time
	datasetIDs, tableIDs *uid.Space
)

// Note: integration tests cannot be run in parallel, because TestIntegration_Location
// modifies the client.

func TestMain(m *testing.M) {
	cleanup := initIntegrationTest()
	r := m.Run()
	cleanup()
	os.Exit(r)
}

func getClient(t *testing.T) *bigquery.Client {
	if client == nil {
		t.Skip("Integration tests skipped")
	}
	return client
}

var grpcHeadersChecker = testutil.DefaultHeadersEnforcer()

// If integration tests will be run, create a unique dataset for them.
// Return a cleanup function.
func initIntegrationTest() func() {
	ctx := context.Background()
	flag.Parse() // needed for testing.Short()
	projID := testutil.ProjID()
	switch {
	case testing.Short():
		client = nil
		return func() {}

	default: // Run integration tests against a real backend.
		ts := testutil.TokenSource(ctx, bigquery.Scope)
		if ts == nil {
			log.Println("Integration tests skipped. See CONTRIBUTING.md for details")
			return func() {}
		}
		bqOpts := []option.ClientOption{option.WithTokenSource(ts)}
		bqOpts = append(bqOpts, grpcHeadersChecker.CallOptions()...)
		cleanup := func() {}
		now := time.Now().UTC()
		var err error
		client, err = bigquery.NewClient(ctx, projID, bqOpts...)
		if err != nil {
			log.Fatalf("bigquery.NewClient: %v", err)
		}
		storageReadClient, err = NewClient(ctx, projID, bqOpts...)
		if err != nil {
			log.Fatalf("NewClient: %v", err)
		}
		c := initTestState(client, now)
		return func() { c(); cleanup() }
	}
}

func initTestState(client *bigquery.Client, t time.Time) func() {
	// BigQuery does not accept hyphens in dataset or table IDs, so we create IDs
	// with underscores.
	ctx := context.Background()
	opts := &uid.Options{Sep: '_', Time: t}
	datasetIDs = uid.NewSpace("dataset", opts)
	tableIDs = uid.NewSpace("table", opts)
	testTableExpiration = t.Add(2 * time.Hour).Round(time.Second)
	// For replayability, seed the random source with t.
	bigquery.Seed(t.UnixNano())

	dataset = client.Dataset(datasetIDs.New())

	if err := dataset.Create(ctx, nil); err != nil {
		log.Fatalf("creating dataset %s: %v", dataset.DatasetID, err)
	}

	return func() {
		if err := dataset.DeleteWithContents(ctx); err != nil {
			log.Printf("could not delete %s", dataset.DatasetID)
		}
	}
}

func TestIntegration_StorageReadBasicTypes(t *testing.T) {
	if client == nil {
		t.Skip("Integration tests skipped")
	}
	ctx := context.Background()

	d := civil.Date{Year: 2016, Month: 3, Day: 20}
	tm := civil.Time{Hour: 15, Minute: 04, Second: 05, Nanosecond: 3008}
	rtm := tm
	rtm.Nanosecond = 3000 // round to microseconds
	dtm := civil.DateTime{Date: d, Time: tm}
	ts := time.Date(2016, 3, 20, 15, 04, 05, 0, time.UTC)
	rat := big.NewRat(13, 10)
	bigRat := big.NewRat(12345, 10e10)
	type ss struct {
		String string
	}

	type s struct {
		Timestamp      time.Time
		StringArray    []string
		SubStruct      ss
		SubStructArray []ss
	}
	testCases := []struct {
		query      string
		parameters []bigquery.QueryParameter
		wantRow    []bigquery.Value
		wantConfig interface{}
	}{
		{
			"SELECT @val",
			[]bigquery.QueryParameter{{Name: "val", Value: 1}},
			[]bigquery.Value{int64(1)},
			int64(1),
		},
		{
			"SELECT @val",
			[]bigquery.QueryParameter{{Name: "val", Value: 1.3}},
			[]bigquery.Value{1.3},
			1.3,
		},
		{
			"SELECT @val",
			[]bigquery.QueryParameter{{Name: "val", Value: rat}},
			[]bigquery.Value{rat},
			rat,
		},
		{
			"SELECT @val",
			[]bigquery.QueryParameter{{Name: "val", Value: &bigquery.QueryParameterValue{
				Type: bigquery.StandardSQLDataType{
					TypeKind: "BIGNUMERIC",
				},
				Value: bigquery.BigNumericString(bigRat),
			}}},
			[]bigquery.Value{bigRat},
			bigRat,
		},
		{
			"SELECT @val",
			[]bigquery.QueryParameter{{Name: "val", Value: true}},
			[]bigquery.Value{true},
			true,
		},
		{
			"SELECT @val",
			[]bigquery.QueryParameter{{Name: "val", Value: "ABC"}},
			[]bigquery.Value{"ABC"},
			"ABC",
		},
		{
			"SELECT @val",
			[]bigquery.QueryParameter{{Name: "val", Value: []byte("foo")}},
			[]bigquery.Value{[]byte("foo")},
			[]byte("foo"),
		},
		{
			"SELECT @val",
			[]bigquery.QueryParameter{{Name: "val", Value: ts}},
			[]bigquery.Value{ts},
			ts,
		},
		{
			"SELECT @val",
			[]bigquery.QueryParameter{{Name: "val", Value: []time.Time{ts, ts}}},
			[]bigquery.Value{[]bigquery.Value{ts, ts}},
			[]interface{}{ts, ts},
		},
		{
			"SELECT @val",
			[]bigquery.QueryParameter{{Name: "val", Value: dtm}},
			[]bigquery.Value{civil.DateTime{Date: d, Time: rtm}},
			civil.DateTime{Date: d, Time: rtm},
		},
		{
			"SELECT @val",
			[]bigquery.QueryParameter{{Name: "val", Value: d}},
			[]bigquery.Value{d},
			d,
		},
		{
			"SELECT @val",
			[]bigquery.QueryParameter{{Name: "val", Value: tm}},
			[]bigquery.Value{rtm},
			rtm,
		},
		{
			"SELECT @val",
			[]bigquery.QueryParameter{{Name: "val", Value: s{ts, []string{"a", "b"}, ss{"c"}, []ss{{"d"}, {"e"}}}}},
			[]bigquery.Value{[]bigquery.Value{ts, []bigquery.Value{"a", "b"}, []bigquery.Value{"c"}, []bigquery.Value{[]bigquery.Value{"d"}, []bigquery.Value{"e"}}}},
			map[string]interface{}{
				"Timestamp":   ts,
				"StringArray": []interface{}{"a", "b"},
				"SubStruct":   map[string]interface{}{"String": "c"},
				"SubStructArray": []interface{}{
					map[string]interface{}{"String": "d"},
					map[string]interface{}{"String": "e"},
				},
			},
		},
		{
			"SELECT @val.Timestamp, @val.SubStruct.String",
			[]bigquery.QueryParameter{{Name: "val", Value: s{Timestamp: ts, SubStruct: ss{"a"}}}},
			[]bigquery.Value{ts, "a"},
			map[string]interface{}{
				"Timestamp":      ts,
				"SubStruct":      map[string]interface{}{"String": "a"},
				"StringArray":    nil,
				"SubStructArray": nil,
			},
		},
	}
	for _, c := range testCases {
		q := client.Query(c.query)
		q.Parameters = c.parameters
		it, err := storageReadClient.ReadQuery(ctx, q)
		if err != nil {
			t.Fatal(err)
		}
		err = checkRead(it, c.wantRow)
		if err != nil {
			t.Fatalf("error on query `%s`[%v]: %v", c.query, c.parameters, err)
		}
	}
}

func checkRowsRead(it *RowIterator, expectedRows [][]bigquery.Value) error {
	for _, row := range expectedRows {
		err := checkRead(it, row)
		if err != nil {
			return err
		}
	}
	return nil
}

func checkRead(it *RowIterator, expectedRow []bigquery.Value) error {
	var outRow []bigquery.Value
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

func TestIntegration_ReadFromSources(t *testing.T) {
	if client == nil {
		t.Skip("Integration tests skipped")
	}
	ctx := context.Background()

	dstTable := dataset.Table(tableIDs.New())
	sql := `SELECT 1 as num, 'one' as str 
UNION ALL 
SELECT 2 as num, 'two' as str 
UNION ALL 
SELECT 3 as num, 'three' as str 
ORDER BY num`
	q := client.Query(sql)
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
	expectedRows := [][]bigquery.Value{
		{int64(1), "one"},
		{int64(2), "two"},
		{int64(3), "three"},
	}
	tableRowIt, err := storageReadClient.ReadTable(ctx, dstTable, WithMaxStreamCount(1))
	if err != nil {
		t.Fatalf("ReadTable(table): %v", err)
	}
	if err = checkRowsRead(tableRowIt, expectedRows); err != nil {
		t.Fatalf("checkRowsRead(table): %v", err)
	}
	jobRowIt, err := storageReadClient.ReadJobResults(ctx, job, WithMaxStreamCount(1))
	if err != nil {
		t.Fatalf("ReadJobResults(job): %v", err)
	}
	if err = checkRowsRead(jobRowIt, expectedRows); err != nil {
		t.Fatalf("checkRowsRead(job): %v", err)
	}
	q.Dst = nil
	qRowIt, err := storageReadClient.ReadQuery(ctx, q, WithMaxStreamCount(1))
	if err != nil {
		t.Fatalf("ReadQuery(query): %v", err)
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
	q := client.Query(sql)

	s, err := storageReadClient.SessionForQuery(ctx, q, WithMaxStreamCount(0))
	if err != nil {
		t.Fatal(err)
	}
	it, err := s.ReadArrow()
	if err != nil {
		t.Fatal(err)
	}
	records := []arrow.Record{}
	for {
		record, err := it.Next()
		if err == iterator.Done {
			break
		}
		if err != nil {
			t.Fatal(err)
		}
		records = append(records, record)
	}
	arrowSchema := it.Schema()
	if arrowSchema == nil {
		t.Fatal("should have Arrow table available, but nil found")
	}
	var arrowTable arrow.Table
	arrowTable = array.NewTableFromRecords(arrowSchema, records)
	defer arrowTable.Release()
	if arrowTable.NumRows() != int64(it.TotalRows) {
		t.Fatalf("should have a table with %d rows, but found %d", it.TotalRows, arrowTable.NumRows())
	}
	if arrowTable.NumCols() != 3 {
		t.Fatalf("should have a table with 3 columns, but found %d", arrowTable.NumCols())
	}

	// Re run query
	s, err = storageReadClient.SessionForQuery(ctx, q, WithMaxStreamCount(0))
	if err != nil {
		t.Fatal(err)
	}
	it, err = s.ReadArrow()
	if err != nil {
		t.Fatal(err)
	}
	arrowTable, err = it.Table()
	if err != nil {
		t.Fatal(err)
	}
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
	sumValues := []bigquery.Value{}
	err = sumIt.Next(&sumValues)
	if err != nil {
		t.Fatal(err)
	}
	totalFromSQL := sumValues[0].(int64)

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

func TestIntegration_StorageReadQueryOrdering(t *testing.T) {
	if client == nil {
		t.Skip("Integration tests skipped")
	}
	ctx := context.Background()
	table := "`bigquery-public-data.usa_names.usa_1910_current`"
	sql := fmt.Sprintf(`SELECT name, number, state FROM %s`, table)
	q := client.Query(sql)

	it, err := storageReadClient.ReadQuery(ctx, q, WithMaxStreamCount(0))
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
	if it.Session.StreamCount == 0 {
		t.Fatalf("should use more than one stream but found %d", it.Session.StreamCount)
	}
	if i != it.TotalRows {
		t.Fatalf("should have read %d rows, but read %d", it.TotalRows, i)
	}
	t.Logf("number of parallel streams for query `%s`: %d", q.Q, it.Session.StreamCount)
	t.Logf("bytes scanned for query `%s`: %d", q.Q, it.Session.EstimatedTotalBytesScanned)

	orderedQ := client.Query(sql + " order by name")
	it, err = storageReadClient.ReadQuery(ctx, orderedQ)
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
	if it.Session.StreamCount > 1 {
		t.Fatalf("should use just one stream as is ordered, but found %d", it.Session.StreamCount)
	}
	if i != it.TotalRows {
		t.Fatalf("should have read %d rows, but read %d", it.TotalRows, i)
	}
	t.Logf("number of parallel streams for query `%s`: %d", orderedQ.Q, it.Session.StreamCount)
}

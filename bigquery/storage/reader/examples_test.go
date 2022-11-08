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

package reader_test

import (
	"context"
	"fmt"

	"cloud.google.com/go/bigquery"
	"cloud.google.com/go/bigquery/storage/reader"
	"github.com/apache/arrow/go/v10/arrow"
	"github.com/apache/arrow/go/v10/arrow/array"
	"github.com/apache/arrow/go/v10/arrow/math"
	"google.golang.org/api/iterator"
)

func ExampleReadFromSources() {
	ctx := context.Background()
	projectID := "project-id"
	client, err := bigquery.NewClient(ctx, projectID)
	if err != nil {
		// TODO: Handle error.
	}
	storageReadClient, err := reader.NewClient(ctx, projectID)
	if err != nil {
		// TODO: Handle error.
	}

	sql := fmt.Sprintf(`SELECT name, number, state FROM %s WHERE state = "CA"`, `bigquery-public-data.usa_names.usa_1910_current`)
	q := client.Query(sql)
	job, err := q.Run(ctx)
	if err != nil {
		// TODO: Handle error.
	}
	status, err := job.Wait(ctx)
	if err != nil {
		// TODO: Handle error.
	}
	if err := status.Err(); err != nil {
		// TODO: Handle error.
	}

	it, err := storageReadClient.ReadQuery(ctx, q)
	if err != nil {
		// TODO: Handle error.
	}
	type S struct {
		Name   string
		Number int
		State  string
	}
	for {
		var s S
		err := it.Next(&s)
		if err == iterator.Done {
			break
		}
		if err != nil {
			// TODO: Handle error.
		}
	}
}

func ExampleReadRawArrow() {
	ctx := context.Background()
	projectID := "project-id"
	client, err := bigquery.NewClient(ctx, projectID)
	if err != nil {
		// TODO: Handle error.
	}
	storageReadClient, err := reader.NewClient(ctx, projectID)
	if err != nil {
		// TODO: Handle error.
	}

	table := "`bigquery-public-data.usa_names.usa_1910_current`"
	sql := fmt.Sprintf(`SELECT name, number, state FROM %s where state = "CA"`, table)
	q := client.Query(sql)

	s, err := storageReadClient.SessionForQuery(ctx, q)
	if err != nil {
		// TODO: Handle error.
	}
	it, err := s.ReadArrow()
	if err != nil {
		// TODO: Handle error.
	}
	records := []arrow.Record{}
	for {
		record, err := it.Next()
		if err == iterator.Done {
			break
		}
		if err != nil {
			// TODO: Handle error.
		}
		records = append(records, record)
	}
	arrowSchema := it.Schema()
	var arrowTable arrow.Table
	arrowTable = array.NewTableFromRecords(arrowSchema, records)
	defer arrowTable.Release()

	// Re run query
	it, err = storageReadClient.RawReadQuery(ctx, q)
	if err != nil {
		// TODO: Handle error.
	}
	arrowTable, err = it.Table()
	if err != nil {
		// TODO: Handle error.
	}
	defer arrowTable.Release()

	sumSQL := fmt.Sprintf(`SELECT sum(number) as total FROM %s where state = "CA"`, table)
	sumQuery := client.Query(sumSQL)
	sumIt, err := sumQuery.Read(ctx)
	if err != nil {
		// TODO: Handle error.
	}
	sumValues := []bigquery.Value{}
	err = sumIt.Next(&sumValues)
	if err != nil {
		// TODO: Handle error.
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
		// TODO: Handle error.
	}
}

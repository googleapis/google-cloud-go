// Copyright 2025 Google LLC
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     https://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law_assets/v2_query_iterator.go
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package query

import (
	"context"

	"google.golang.org/api/iterator"
	"google.golang.org/protobuf/types/known/structpb"
)

// RowIterator is an iterator over the results of a query.
type RowIterator struct {
	c         *QueryClient
	rows      []*Row
	query     *QueryJob
	totalRows uint64
	schema    *schema
	pageToken string
}

// Next returns the next row from the results.
func (it *RowIterator) Next(ctx context.Context) (*Row, error) {
	if len(it.rows) > 0 {
		return it.dequeueRow(), nil
	}
	if it.pageToken == "" {
		return nil, iterator.Done
	}

	err := it.fetchRows(ctx)
	if err != nil {
		return nil, err
	}

	return it.dequeueRow(), nil
}

func (it *RowIterator) fetchRows(ctx context.Context) error {
	res, err := it.query.getRows(ctx, it.pageToken)
	if err != nil {
		return err
	}

	if res.TotalRows != nil {
		it.totalRows = res.TotalRows.Value
	}
	if it.schema == nil {
		it.schema = newSchema(res.Schema)
	}

	rows := res.GetRows()
	if len(rows) == 0 {
		return iterator.Done
	}

	it.rows, err = fieldValueRowsToRowList(rows, it.schema)
	if err != nil {
		return err
	}
	it.pageToken = res.GetPageToken()

	return nil
}

func (it *RowIterator) dequeueRow() *Row {
	if len(it.rows) == 0 {
		panic("no rows to dequeue")
	}
	row := it.rows[0]
	it.rows = it.rows[1:]
	return row
}

func fieldValueRowsToRowList(rows []*structpb.Struct, schema *schema) ([]*Row, error) {
	values, err := convertRows(rows, schema)
	if err != nil {
		return nil, err
	}
	nrows := make([]*Row, len(rows))
	for i := range rows {
		nrows[i] = newRowFromValues(values[i])
	}
	return nrows, nil
}

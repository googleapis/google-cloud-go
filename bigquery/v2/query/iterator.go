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
	"iter"

	"github.com/googleapis/gax-go/v2"
	gaxIterator "github.com/googleapis/gax-go/v2/iterator"
	"google.golang.org/api/iterator"
)

// RowIterator is an iterator over the results of a query.
type RowIterator struct {
	ctx       context.Context
	r         sourceReader
	rows      []*Row
	totalRows uint64
	pageToken string
	opts      []gax.CallOption
}

// All returns an iterator. If an error is returned by the iterator, the
// iterator will stop after that iteration.
func (it *RowIterator) All() iter.Seq2[*Row, error] {
	return gaxIterator.RangeAdapter(it.Next)
}

// Next returns the next row from the results.
func (it *RowIterator) Next() (*Row, error) {
	if len(it.rows) > 0 {
		return it.dequeueRow(), nil
	}

	err := it.fetchRows(it.ctx, it.opts)
	if err != nil {
		return nil, err
	}

	return it.dequeueRow(), nil
}

func (it *RowIterator) fetchRows(ctx context.Context, opts []gax.CallOption) error {
	res, err := it.r.nextPage(ctx, it.pageToken, opts)
	if err != nil {
		return err
	}

	if res.totalRows != nil {
		it.totalRows = res.totalRows.GetValue()
	}

	rows := res.rows
	if len(rows) == 0 {
		return iterator.Done
	}

	it.rows = res.rows
	it.pageToken = res.pageToken

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

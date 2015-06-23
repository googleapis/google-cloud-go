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
	"errors"

	"golang.org/x/net/context"
)

// A pageFetcher returns the next page of rows.
type pageFetcher func(ctx context.Context, token string) (*readDataResult, error)

// Iterator provides access to the result of a BigQuery lookup.
// Next must be called before the first call to Get.
type Iterator struct {
	// pf fetches a page of data.
	pf        pageFetcher
	nextToken string
	done      bool // Set to true when there is no  more data to be fetched from the server.

	rs  [][]Value // contains prefetched rows. The first element is returned by Get.
	err error     // contains any error encountered during calls to Next.
}

// Next advances the Iterator to the next row, making that row available
// via the Get method.
// Next must be called before the first call to Get, and blocks until data is available.
// Next returns false when there are no more rows available, either because
// the end of the output was reached, or because there was an error (consult
// the Err method to determine which).
func (it *Iterator) Next(ctx context.Context) bool {
	if it.err != nil {
		return false
	}

	if len(it.rs) > 0 {
		it.rs = it.rs[1:]
	}

	if len(it.rs) == 0 {
		it.fetchRows(ctx)
	}

	return it.hasCurrentRow()
}

func (it *Iterator) hasCurrentRow() bool {
	return it.err == nil && len(it.rs) != 0
}

// fetchRows fetches a list of rows from the BigQuery service.
// The fetched rows will be returned via subsequent calls to Get.
func (it *Iterator) fetchRows(ctx context.Context) {
	if it.done {
		return
	}

	res, err := it.pf(ctx, it.nextToken)
	for err == incompleteJobError {
		res, err = it.pf(ctx, it.nextToken)
	}
	if err != nil {
		it.err = err
		return
	}

	it.done = (res.pageToken == "")
	it.nextToken = res.pageToken
	it.rs = res.rows
}

// Err returns the last error encountered by Next, or nil for no error.
func (it *Iterator) Err() error {
	return it.err
}

// Get loads the current row into dst, which must implement ValueLoader.
func (it *Iterator) Get(dst interface{}) error {
	if !it.hasCurrentRow() {
		return errors.New("Get called on iterator with no remaining values")
	}

	if dst, ok := dst.(ValueLoader); ok {
		return dst.Load(it.rs[0])
	}
	return errors.New("Get called with unsupported argument type")
}

// TODO(mcgreevy): Add a method to *Iterator that returns a schema which describes the data.

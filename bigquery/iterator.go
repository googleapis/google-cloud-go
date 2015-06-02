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

	"golang.org/x/net/context"
)

// Iterator provides access to the result of a BigQuery lookup.
// Next must be called before the first call to Get.
type Iterator struct {
	s service

	// conf contains the information necessary to make the next readTabledata call.
	// conf is set to nil when there is no more data to be fetched from the server.
	conf *readTabledataConf
	rs   [][]Value // contains prefetched rows. The first element is returned by Get.
	err  error     // contains any error encountered during calls to Next.
}

// Next advances the Iterator to the next row, making that row available
// via the Get method.
// Next must be called before the first call to Get.
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

// fetchRows fetches a series of rows from the BigQuery service.
// The fetched rows will be returned via subsequent calls to Get.
func (it *Iterator) fetchRows(ctx context.Context) {
	if it.conf == nil {
		return
	}
	// TODO(mcgreevy): refactor to support reads of query results.
	res, err := it.s.readTabledata(ctx, it.conf)
	if err != nil {
		it.err = err
		return
	}
	if res.pageToken == "" {
		// No more data.
		it.conf = nil
	} else {
		it.conf.paging.pageToken = res.pageToken
	}
	it.rs = res.rows
}

// Err returns the last error encountered by Next, or nil for no error.
func (it *Iterator) Err() error {
	return it.err
}

// Get loads the current row into dst, which must implement ValueLoader.
func (it *Iterator) Get(dst interface{}) error {
	if !it.hasCurrentRow() {
		return fmt.Errorf("Get called on iterator with no remaining values")
	}

	if dst, ok := dst.(ValueLoader); ok {
		return dst.Load(it.rs[0])
	}
	return fmt.Errorf("Get called with unsupported argument type")
}

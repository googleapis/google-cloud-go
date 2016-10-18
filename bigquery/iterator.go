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
	"golang.org/x/net/context"
	"google.golang.org/api/iterator"
)

// A pageFetcher returns a page of rows, starting from the row specified by token.
type pageFetcher interface {
	fetch(ctx context.Context, s service, token string) (*readDataResult, error)
	setPaging(*pagingConf)
}

func newRowIterator(ctx context.Context, s service, pf pageFetcher) *RowIterator {
	it := &RowIterator{
		ctx:     ctx,
		service: s,
		pf:      pf,
	}
	it.pageInfo, it.nextFunc = iterator.NewPageInfo(
		it.fetch,
		func() int { return len(it.rows) },
		func() interface{} { r := it.rows; it.rows = nil; return r })
	return it
}

// A RowIterator provides access to the result of a BigQuery lookup.
type RowIterator struct {
	ctx      context.Context
	service  service
	pf       pageFetcher
	pageInfo *iterator.PageInfo
	nextFunc func() error

	// StartIndex can be set before the first call to Next. If PageInfo().PageToken
	// is also set, StartIndex is ignored.
	StartIndex uint64

	rows [][]Value

	schema Schema // populated on first call to fetch
}

// Next loads the next row into dst. Its return value is iterator.Done if there
// are no more results. Once Next returns iterator.Done, all subsequent calls
// will return iterator.Done.
func (it *RowIterator) Next(dst ValueLoader) error {
	if err := it.nextFunc(); err != nil {
		return err
	}
	row := it.rows[0]
	it.rows = it.rows[1:]
	return dst.Load(row, it.schema)
}

// PageInfo supports pagination. See the google.golang.org/api/iterator package for details.
func (it *RowIterator) PageInfo() *iterator.PageInfo { return it.pageInfo }

func (it *RowIterator) fetch(pageSize int, pageToken string) (string, error) {
	pc := &pagingConf{}
	if pageSize > 0 {
		pc.recordsPerRequest = int64(pageSize)
		pc.setRecordsPerRequest = true
	}
	if pageToken == "" {
		pc.startIndex = it.StartIndex
	}
	it.pf.setPaging(pc)
	var res *readDataResult
	var err error
	for {
		res, err = it.pf.fetch(it.ctx, it.service, pageToken)
		if err != errIncompleteJob {
			break
		}
	}
	if err != nil {
		return "", err
	}
	it.rows = append(it.rows, res.rows...)
	it.schema = res.schema
	return res.pageToken, nil
}

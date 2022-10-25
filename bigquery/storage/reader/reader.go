// Copyright 2022 Google LLC
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     https://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package reader

import (
	"context"

	"cloud.google.com/go/bigquery"
)

// Reader is the abstraction over a storage API read session.
type Reader struct {
	settings *settings
	c        *Client
}
type settings struct {
	// MaxStreamCount governs how many parallel streams
	// can be opened.
	MaxStreamCount int
}

func defaultSettings() *settings {
	return &settings{
		MaxStreamCount: 0,
	}
}

// ReadQuery creates a read stream for a given query.
func (r *Reader) ReadQuery(ctx context.Context, query *bigquery.Query) (RowIterator, error) {
	return newQueryRowIterator(ctx, r, query)
}

// ReadJobResults creates a read stream for a given job.
func (r *Reader) ReadJobResults(ctx context.Context, job *bigquery.Job) (RowIterator, error) {
	return newJobRowIterator(ctx, r, job)
}

// ReadTable creates a read stream for a given table.
func (r *Reader) ReadTable(ctx context.Context, table *bigquery.Table) (RowIterator, error) {
	return newTableRowIterator(ctx, r, table)
}

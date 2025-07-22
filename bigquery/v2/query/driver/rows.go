// Copyright 2025 Google LLC
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

package driver

import (
	"context"
	"database/sql/driver"
	"io"

	"cloud.google.com/go/bigquery/v2/apiv2/bigquerypb"
	"cloud.google.com/go/bigquery/v2/query"
	"google.golang.org/api/iterator"
)

// rows is a database/sql/driver.Rows for BigQuery.
type rows struct {
	ctx    context.Context
	it     *query.RowIterator
	schema *bigquerypb.TableSchema
}

// Columns returns the names of the columns.
func (r *rows) Columns() []string {
	names := make([]string, len(r.schema.Fields))
	for i, f := range r.schema.Fields {
		names[i] = f.Name
	}
	return names
}

// Close closes the rows iterator.
func (r *rows) Close() error {
	// The iterator is closed automatically when all rows are read.
	return nil
}

// Next is called to populate the next row of data into
// the provided slice. The provided slice will be the same
// size as the number of columns.
func (r *rows) Next(dest []driver.Value) error {
	row, err := r.it.Next(r.ctx)
	if err == iterator.Done {
		return io.EOF
	}
	if err != nil {
		return err
	}
	values := row.AsMap()
	for i, f := range r.schema.Fields {
		v := values[f.Name]
		dest[i] = v
	}
	return nil
}

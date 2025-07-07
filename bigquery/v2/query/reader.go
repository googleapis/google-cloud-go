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

package query

import (
	"context"

	"cloud.google.com/go/bigquery/storage/apiv1/storagepb"
	"cloud.google.com/go/bigquery/v2/apiv2/bigquerypb"
)

// Reader is used to read the results of a query.
type Reader struct {
	c          *Client
	readClient *storagepb.BigQueryReadClient
}

// Read reads the results of a query job.
func (r *Reader) Read(ctx context.Context, jobRef *bigquerypb.JobReference, schema *bigquerypb.TableSchema, opts ...ReadOption) (*RowIterator, error) {
	// TODO: use storage read API
	query, err := newQueryJobFromJobReference(r.c, schema, jobRef)
	if err != nil {
		return nil, err
	}

	return r.readQuery(ctx, query, opts...)
}

func (r *Reader) readQuery(ctx context.Context, q *Query, opts ...ReadOption) (*RowIterator, error) {
	initState := &readState{
		pageToken: q.cachedPageToken,
	}
	for _, opt := range opts {
		opt(initState)
	}

	it := &RowIterator{
		c:         r.c,
		query:     q,
		pageToken: initState.pageToken,
		rows:      q.cachedRows,
		totalRows: q.cachedTotalRows,
		schema:    q.cachedSchema,
	}

	if len(it.query.cachedRows) > 0 {
		return it, nil
	}

	err := it.fetchRows(ctx)
	if err != nil {
		return nil, err
	}
	return it, nil
}

// ReadOption is an option for reading query results.
type ReadOption func(*readState)

type readState struct {
	pageToken string
}

// WithPageToken sets the page token for reading query results.
func WithPageToken(t string) ReadOption {
	return func(s *readState) {
		s.pageToken = t
	}
}

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

// reader is used to read the results of a query.
type reader struct {
	c          *Client
	readClient *storagepb.BigQueryReadClient
	q          *Query
}

func newReaderFromQuery(c *Client, q *Query) *reader {
	r := &reader{
		c:          c,
		readClient: c.rc,
		q:          q,
	}
	return r
}

func (r *reader) read(ctx context.Context, opts ...ReadOption) (*RowIterator, error) {
	initState := &readState{
		pageToken: r.q.cachedPageToken,
	}
	for _, opt := range opts {
		opt(initState)
	}

	it := &RowIterator{
		r:         r,
		pageToken: initState.pageToken,
		rows:      r.q.cachedRows,
		totalRows: r.q.cachedTotalRows,
		schema:    r.q.cachedSchema,
	}

	if len(r.q.cachedRows) > 0 {
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

func (r *reader) getRows(ctx context.Context, pageToken string) (*bigquerypb.GetQueryResultsResponse, error) {
	return r.c.c.GetQueryResults(ctx, &bigquerypb.GetQueryResultsRequest{
		FormatOptions: &bigquerypb.DataFormatOptions{
			UseInt64Timestamp: true,
		},
		JobId:     r.q.jobID,
		ProjectId: r.q.projectID,
		Location:  r.q.location,
		PageToken: pageToken,
	})
}

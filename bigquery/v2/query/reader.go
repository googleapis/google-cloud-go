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

	"cloud.google.com/go/bigquery/v2/apiv2/bigquerypb"
	"github.com/googleapis/gax-go/v2"
	"google.golang.org/api/iterator"
)

type sourceReader interface {
	// start do all set up and make sure data can be read.
	start(ctx context.Context, state *readState, opts []gax.CallOption) (*RowIterator, error)
	// nextPage fetchs new page of results. Can return iterator.Done if there is
	// no more pages to be fetched.
	nextPage(ctx context.Context, pageToken string, opts []gax.CallOption) (*resultSet, error)
}

type resultSet struct {
	totalRows uint64
	pageToken string
	rows      []*Row
}

// jobsReader is used to read the results of a query using jobs.getQueryResults API.
type jobsReader struct {
	c            *Client
	q            *Query
	gotFirstPage bool
}

var _ sourceReader = &jobsReader{}

func newReaderFromQuery(ctx context.Context, c *Client, q *Query, state *readState) sourceReader {
	if c.rc != nil || state.readClient != nil {
		rc := c.rc
		if rc == nil {
			rc = state.readClient
		}
		r, err := newStorageReader(ctx, c, rc, q)
		if err == nil {
			return r
		}
	}
	return newJobsReader(c, q)
}

func newJobsReader(c *Client, q *Query) *jobsReader {
	r := &jobsReader{
		c: c,
		q: q,
	}
	r.gotFirstPage = len(q.cachedRows) > 0

	return r
}

func (r *jobsReader) start(ctx context.Context, state *readState, opts []gax.CallOption) (*RowIterator, error) {
	it := &RowIterator{
		ctx:       ctx,
		opts:      opts,
		r:         r,
		rows:      r.q.cachedRows,
		pageToken: state.pageToken,
		totalRows: r.q.cachedTotalRows,
	}

	if len(it.rows) > 0 {
		return it, nil
	}

	err := it.fetchRows(ctx, opts)
	if err != nil {
		return nil, err
	}
	return it, nil
}

func (r *jobsReader) nextPage(ctx context.Context, pageToken string, opts []gax.CallOption) (*resultSet, error) {
	if pageToken == "" && r.gotFirstPage {
		return nil, iterator.Done
	}

	if !hasRetry(opts) {
		opts = append(opts, gax.WithRetry(defaultRetryerFunc))
	}

	res, err := r.c.c.GetQueryResults(ctx, &bigquerypb.GetQueryResultsRequest{
		FormatOptions: &bigquerypb.DataFormatOptions{
			UseInt64Timestamp: true,
		},
		JobId:     r.q.jobID,
		ProjectId: r.q.projectID,
		Location:  r.q.location,
		PageToken: pageToken,
	}, opts...)
	if err != nil {
		return nil, err
	}
	r.gotFirstPage = true

	schema := r.q.cachedSchema
	if schema == nil {
		schema = newSchema(res.Schema)
		r.q.cachedSchema = schema
	}

	rows, err := fieldValueRowsToRowList(res.Rows, schema)
	if err != nil {
		return nil, err
	}
	rs := &resultSet{
		rows:      rows,
		pageToken: res.PageToken,
		totalRows: 0,
	}
	if res.TotalRows != nil {
		rs.totalRows = res.TotalRows.Value
	}
	return rs, nil
}

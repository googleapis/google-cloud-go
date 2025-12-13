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
	"fmt"

	"cloud.google.com/go/bigquery/v2/apiv2/bigquerypb"
	"github.com/googleapis/gax-go/v2"
	"google.golang.org/api/iterator"
	"google.golang.org/protobuf/types/known/wrapperspb"
)

type sourceReader interface {
	// start do all set up and make sure data can be read.
	start(ctx context.Context, state *readState, opts []gax.CallOption) (*RowIterator, error)
	// nextPage fetchs new page of results. Can return iterator.Done if there is
	// no more pages to be fetched.
	nextPage(ctx context.Context, pageToken string, opts []gax.CallOption) (*resultSet, error)
}

type resultSet struct {
	totalRows *wrapperspb.UInt64Value
	pageToken string
	rows      []*Row
}

// jobsReader is used to read the results of a query using jobs.getQueryResults API.
type jobsReader struct {
	h            *Helper
	q            *Query
	gotFirstPage bool
}

var _ sourceReader = &jobsReader{}

func newReaderFromQuery(h *Helper, q *Query) sourceReader {
	// TODO(#12877): support Storage Read API and branch off here
	return newJobsReader(h, q)
}

func newJobsReader(h *Helper, q *Query) *jobsReader {
	r := &jobsReader{
		h: h,
		q: q,
	}
	r.gotFirstPage = len(q.cachedRows) > 0

	return r
}

func (r *jobsReader) start(ctx context.Context, state *readState, opts []gax.CallOption) (*RowIterator, error) {
	it := &RowIterator{
		ctx:       ctx,
		r:         r,
		opts:      opts,
		rows:      r.q.cachedRows,
		pageToken: state.pageToken,
		totalRows: r.q.cachedTotalRows,
	}

	if len(it.rows) > 0 {
		return it, nil
	}

	err := it.fetchRows(ctx, it.opts)
	if err != nil {
		return nil, err
	}
	return it, nil
}

func (r *jobsReader) nextPage(ctx context.Context, pageToken string, opts []gax.CallOption) (*resultSet, error) {
	if pageToken == "" && r.gotFirstPage {
		return nil, iterator.Done
	}

	jobRef := r.q.JobReference()
	if jobRef == nil {
		return nil, fmt.Errorf("missing job reference to read more pages")
	}

	location := ""
	if jobRef.GetLocation() != nil {
		location = jobRef.GetLocation().GetValue()
	}
	res, err := r.h.c.GetQueryResults(ctx, &bigquerypb.GetQueryResultsRequest{
		FormatOptions: &bigquerypb.DataFormatOptions{
			UseInt64Timestamp: true,
		},
		JobId:     jobRef.GetJobId(),
		ProjectId: jobRef.GetProjectId(),
		Location:  location,
		PageToken: pageToken,
	}, opts...)
	if err != nil {
		return nil, err
	}
	r.gotFirstPage = true

	r.q.updateCachedSchema(res.GetSchema())

	rows := parseRows(res.GetRows())
	rs := &resultSet{
		rows:      rows,
		pageToken: res.GetPageToken(),
		totalRows: res.GetTotalRows(),
	}
	return rs, nil
}

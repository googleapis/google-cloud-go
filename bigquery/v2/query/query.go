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
	"time"

	"cloud.google.com/go/bigquery/v2/apiv2/bigquerypb"
	"google.golang.org/protobuf/types/known/wrapperspb"
)

// Query represents a query job.
type Query struct {
	c         *Client
	complete  bool
	projectID string
	jobID     string
	location  string

	cachedTotalRows uint64
	cachedPageToken string
	cachedRows      []*Row
	cachedSchema    *schema
}

func newQueryJobFromQueryResponse(c *Client, res *bigquerypb.QueryResponse) (*Query, error) {
	schema := newSchema(res.Schema)
	q := &Query{
		c:               c,
		cachedSchema:    schema,
		cachedPageToken: res.PageToken,
	}
	if res.TotalRows != nil {
		q.cachedTotalRows = res.TotalRows.Value
	}
	if res.JobComplete != nil {
		q.complete = res.JobComplete.Value
	}
	if res.Rows != nil {
		var err error
		q.cachedRows, err = fieldValueRowsToRowList(res.Rows, schema)
		if err != nil {
			return nil, err
		}
	}
	if res.JobReference != nil {
		jobRef := res.JobReference
		q.projectID = jobRef.ProjectId
		q.jobID = jobRef.JobId
		if jobRef.Location != nil {
			q.location = jobRef.Location.GetValue()
		}
	}
	return q, nil
}

func newQueryJobFromJob(c *Client, job *bigquerypb.Job) (*Query, error) {
	return newQueryJobFromJobReference(c, nil, job.JobReference)
}

func newQueryJobFromJobReference(c *Client, schema *bigquerypb.TableSchema, jobRef *bigquerypb.JobReference) (*Query, error) {
	res := &bigquerypb.QueryResponse{
		Schema:       schema,
		JobReference: jobRef,
	}
	return newQueryJobFromQueryResponse(c, res)
}

// Read returns a RowIterator for the query results.
func (q *Query) Read(ctx context.Context, opts ...ReadOption) (*RowIterator, error) {
	state := &readState{
		pageToken: q.cachedPageToken,
	}
	for _, opt := range opts {
		opt(state)
	}
	r := newReaderFromQuery(ctx, q.c, q, state)
	return r.start(ctx, state)
}

func (q *Query) checkStatus(ctx context.Context) error {
	res, err := q.c.c.GetQueryResults(ctx, &bigquerypb.GetQueryResultsRequest{
		ProjectId:  q.projectID,
		JobId:      q.jobID,
		Location:   q.location,
		MaxResults: wrapperspb.UInt32(0),
		FormatOptions: &bigquerypb.DataFormatOptions{
			UseInt64Timestamp: true,
		},
	})
	if err != nil {
		return err
	}

	err = q.consumeQueryResponse(res)
	if err != nil {
		return err
	}

	return nil
}

func (q *Query) consumeQueryResponse(res *bigquerypb.GetQueryResultsResponse) error {
	q.cachedPageToken = res.PageToken
	schema := newSchema(res.Schema)
	var err error
	q.cachedRows, err = fieldValueRowsToRowList(res.Rows, schema)
	if err != nil {
		return err
	}
	q.cachedSchema = schema
	q.cachedTotalRows = res.TotalRows.Value
	if res.JobComplete != nil {
		q.complete = res.JobComplete.Value
	}
	return nil
}

// Wait waits for the query to complete.
func (q *Query) Wait(ctx context.Context) error {
	for !q.complete {
		err := q.checkStatus(ctx)
		if err != nil {
			return err
		}
		select {
		case <-time.After(1 * time.Second): // TODO: exponetial backoff
		case <-ctx.Done():
			return ctx.Err()
		}
	}
	return nil
}

// JobReference returns the job reference.
func (q *Query) JobReference() *bigquerypb.JobReference {
	return &bigquerypb.JobReference{
		ProjectId: q.projectID,
		JobId:     q.jobID,
		Location:  wrapperspb.String(q.location),
	}
}

// Schema returns the schema of the query results.
func (q *Query) Schema() *bigquerypb.TableSchema {
	return q.cachedSchema.pb
}

// Complete to check if job finished execution
func (q *Query) Complete() bool {
	return q.complete
}

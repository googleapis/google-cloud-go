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
	"sync"
	"time"

	"cloud.google.com/go/bigquery/v2/apiv2/bigquerypb"
	"github.com/googleapis/gax-go/v2"
	"google.golang.org/protobuf/types/known/wrapperspb"
)

// Query represents a query job.
type Query struct {
	c         *Client
	projectID string
	jobID     string
	location  string
	queryID   string

	// context for background pooling
	ctx      context.Context
	mu       sync.RWMutex
	complete bool
	ready    chan struct{}
	err      error

	cachedTotalRows uint64
}

// Create Query handler from jobs.query response and start background pooling job
func newQueryJobFromQueryResponse(ctx context.Context, c *Client, res *bigquerypb.QueryResponse, opts ...gax.CallOption) (*Query, error) {
	q := &Query{
		c:       c,
		ctx:     ctx,
		queryID: res.QueryId,
		ready:   make(chan struct{}),
	}
	err := q.consumeQueryResponse(&bigquerypb.GetQueryResultsResponse{
		Schema:              res.Schema,
		PageToken:           res.PageToken,
		TotalRows:           res.TotalRows,
		JobReference:        res.JobReference,
		Rows:                res.Rows,
		JobComplete:         res.JobComplete,
		Errors:              res.Errors,
		CacheHit:            res.CacheHit,
		TotalBytesProcessed: res.TotalBytesProcessed,
		NumDmlAffectedRows:  res.NumDmlAffectedRows,
	})
	if err != nil {
		return nil, err
	}

	go q.waitForQueryBackground(opts)

	return q, nil
}

// Create Query handler from JobReference response and start background pooling job
func newQueryJobFromJobReference(ctx context.Context, c *Client, jobRef *bigquerypb.JobReference, opts ...gax.CallOption) (*Query, error) {
	res := &bigquerypb.QueryResponse{
		JobReference: jobRef,
	}
	return newQueryJobFromQueryResponse(ctx, c, res, opts...)
}

// Read returns a RowIterator for the query results.
func (q *Query) Read(ctx context.Context, opts ...ReadOption) (*RowIterator, error) {
	// TODO: proper setup iterator
	return &RowIterator{}, nil
}

// Wait waits for the query to complete.
func (q *Query) Wait(opts ...gax.CallOption) error {
	select {
	case <-q.ready:
		return q.err
	case <-q.ctx.Done():
		return q.ctx.Err()
	}
}

func (q *Query) waitForQueryBackground(opts []gax.CallOption) {
	for !q.complete {
		err := q.waitForQuery(q.ctx, opts)
		if err != nil {
			q.markDone(err)
			return
		}
		select {
		case <-time.After(1 * time.Second): // TODO: exponetial backoff
		case <-q.ctx.Done():
			q.markDone(q.ctx.Err())
			return
		}
	}
	q.markDone(nil)
}

func (q *Query) markDone(err error) {
	q.mu.Lock()
	q.err = err
	close(q.ready)
	q.mu.Unlock()
}

func (q *Query) waitForQuery(ctx context.Context, opts []gax.CallOption) error {
	res, err := q.c.c.GetQueryResults(ctx, &bigquerypb.GetQueryResultsRequest{
		ProjectId:  q.projectID,
		JobId:      q.jobID,
		Location:   q.location,
		MaxResults: wrapperspb.UInt32(0),
		FormatOptions: &bigquerypb.DataFormatOptions{
			UseInt64Timestamp: true,
		},
	}, opts...)
	if err != nil {
		return err
	}

	return q.consumeQueryResponse(res)
}

func (q *Query) consumeQueryResponse(res *bigquerypb.GetQueryResultsResponse) error {
	q.mu.Lock()
	defer q.mu.Unlock()

	if res.JobComplete != nil {
		q.complete = res.JobComplete.Value
	}

	if res.JobReference != nil {
		jobRef := res.JobReference
		q.projectID = jobRef.ProjectId
		q.jobID = jobRef.JobId
		if jobRef.Location != nil {
			q.location = jobRef.Location.GetValue()
		}
	}

	if res.TotalRows != nil {
		q.cachedTotalRows = res.TotalRows.Value
	}

	// TODO: save schema, page token, total rows and parse rows

	return nil
}

// QueryID returns the auto-generated ID for the query.
// Only filled for stateless queries.
func (q *Query) QueryID() string {
	return q.queryID
}

// JobReference returns the job reference.
func (q *Query) JobReference() *bigquerypb.JobReference {
	if q.jobID == "" {
		return nil
	}
	return &bigquerypb.JobReference{
		ProjectId: q.projectID,
		JobId:     q.jobID,
		Location:  wrapperspb.String(q.location),
	}
}

// Schema returns the schema of the query results.
func (q *Query) Schema() *bigquerypb.TableSchema {
	return nil // TODO: fill schema
}

// Complete to check if job finished execution
func (q *Query) Complete() bool {
	return q.complete
}

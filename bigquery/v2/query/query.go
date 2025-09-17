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

// Create Query handler using jobs.query request and start background pooling job
func newQueryJobFromQueryRequest(ctx context.Context, c *Client, req *bigquerypb.PostQueryRequest, opts ...gax.CallOption) *Query {
	q := &Query{
		c:     c,
		ctx:   ctx,
		ready: make(chan struct{}),
	}
	go q.runQuery(req, opts)

	return q
}

// Create Query handler using jobs.insert request and start background pooling job
func newQueryJobFromJob(ctx context.Context, c *Client, projectID string, job *bigquerypb.Job, opts ...gax.CallOption) *Query {
	q := &Query{
		c:         c,
		ctx:       ctx,
		ready:     make(chan struct{}),
		projectID: projectID,
	}
	go q.insertQuery(job, opts)

	return q
}

// Create Query handler from JobReference response and start background pooling job
func newQueryJobFromJobReference(ctx context.Context, c *Client, jobRef *bigquerypb.JobReference, opts ...gax.CallOption) *Query {
	q := &Query{
		c:     c,
		ctx:   ctx,
		ready: make(chan struct{}),
	}
	q.consumeQueryResponse(&bigquerypb.GetQueryResultsResponse{
		JobReference: jobRef,
	})

	go q.waitForQueryBackground(opts)

	return q
}

// Read returns a RowIterator for the query results.
func (q *Query) Read(ctx context.Context, opts ...ReadOption) (*RowIterator, error) {
	// TODO: proper setup iterator
	return &RowIterator{}, nil
}

// Wait waits for the query to complete.
// The context.Context parameter is used for cancelling just this particular call to Wait.
// It's basically a shortcut for using the exposed channel and wait for the query to be done.
func (q *Query) Wait(ctx context.Context) error {
	select {
	case <-q.Done():
		return q.Err()
	case <-ctx.Done():
		return q.Err()
	}
}

// Done exposes the internal channel to notify for when the query is ready.
// See Wait method for shortcut for waiting for a query to be executed and get
// the last error.
func (q *Query) Done(opts ...gax.CallOption) <-chan struct{} {
	return q.ready
}

// Err holds last error that happened with the given query execution.
// See Wait method for shortcut for waiting for a query to be executed and get
// the last error.
func (q *Query) Err() error {
	if q.ctx.Err() != nil {
		return q.ctx.Err()
	}
	return q.err
}

func (q *Query) insertQuery(job *bigquerypb.Job, opts []gax.CallOption) {
	res, err := q.c.c.InsertJob(q.ctx, &bigquerypb.InsertJobRequest{
		ProjectId: q.projectID,
		Job:       job,
	}, opts...)

	if err != nil {
		q.markDone(err)
		return
	}

	q.consumeQueryResponse(&bigquerypb.GetQueryResultsResponse{
		JobReference: res.JobReference,
	})

	go q.waitForQueryBackground(opts)
}

func (q *Query) runQuery(req *bigquerypb.PostQueryRequest, opts []gax.CallOption) {
	res, err := q.c.c.Query(q.ctx, req, opts...)
	if err != nil {
		q.markDone(err)
		return
	}
	q.queryID = res.GetQueryId()

	q.consumeQueryResponse(&bigquerypb.GetQueryResultsResponse{
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

	go q.waitForQueryBackground(opts)
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

	q.consumeQueryResponse(res)
	return nil
}

func (q *Query) consumeQueryResponse(res *bigquerypb.GetQueryResultsResponse) {
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

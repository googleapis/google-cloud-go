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

// Query represents a handle to a query job. Its methods can be used to wait for
// the job to complete and to iterate over the results.
type Query struct {
	h *Helper

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
func newQueryJobFromQueryRequest(ctx context.Context, h *Helper, req *bigquerypb.PostQueryRequest, opts ...gax.CallOption) *Query {
	q := &Query{
		h:     h,
		ctx:   ctx,
		ready: make(chan struct{}),
	}
	go q.runQuery(req, opts)

	return q
}

// Create Query handler using jobs.insert request and start background pooling job
func newQueryJobFromJob(ctx context.Context, h *Helper, projectID string, job *bigquerypb.Job, opts ...gax.CallOption) *Query {
	q := &Query{
		h:         h,
		ctx:       ctx,
		ready:     make(chan struct{}),
		projectID: projectID,
	}
	go q.insertQuery(job, opts)

	return q
}

// Create Query handler from JobReference response and start background pooling job
func newQueryJobFromJobReference(ctx context.Context, h *Helper, jobRef *bigquerypb.JobReference, opts ...gax.CallOption) *Query {
	q := &Query{
		h:     h,
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

// Wait blocks until the query has completed. The provided context can be used to
// cancel the wait. If the query completes successfully, Wait returns nil.
// Otherwise, it returns the error that caused the query to fail.
//
// Wait is a convenience wrapper around Done and Err.
func (q *Query) Wait(ctx context.Context) error {
	select {
	case <-q.Done():
		return q.Err()
	case <-ctx.Done():
		return ctx.Err()
	}
}

// Done returns a channel that is closed when the query has completed.
// It can be used in a select statement to perform non-blocking waits.
//
// Example:
//
//		select {
//		case <-q.Done():
//			if err := q.Err(); err != nil {
//				// Handle error.
//			}
//			// Query is complete.
//	 case <-time.After(30*time.Second):
//		    // Timeout logic
//		default:
//			// Query is still running.
//		}
func (q *Query) Done(opts ...gax.CallOption) <-chan struct{} {
	return q.ready
}

// Err returns the final error state of the query. It is only valid to call Err
// after the channel returned by Done has been closed. If the query completed
// successfully, Err returns nil.
func (q *Query) Err() error {
	q.mu.RLock()
	defer q.mu.RUnlock()
	err := q.ctx.Err()
	if err != nil {
		return err
	}
	return q.err
}

func (q *Query) insertQuery(job *bigquerypb.Job, opts []gax.CallOption) {
	res, err := q.h.c.InsertJob(q.ctx, &bigquerypb.InsertJobRequest{
		ProjectId: q.projectID,
		Job:       job,
	}, opts...)

	if err != nil {
		q.markDone(err)
		return
	}

	q.consumeQueryResponse(&bigquerypb.GetQueryResultsResponse{
		JobReference: res.GetJobReference(),
	})

	go q.waitForQueryBackground(opts)
}

func (q *Query) runQuery(req *bigquerypb.PostQueryRequest, opts []gax.CallOption) {
	res, err := q.h.c.Query(q.ctx, req, opts...)
	if err != nil {
		q.markDone(err)
		return
	}
	q.queryID = res.GetQueryId()

	q.consumeQueryResponse(res)

	go q.waitForQueryBackground(opts)
}

func (q *Query) waitForQueryBackground(opts []gax.CallOption) {
	backoff := gax.Backoff{
		Initial:    50 * time.Millisecond,
		Multiplier: 1.3,
		Max:        60 * time.Second,
	}
	for !q.complete {
		err := q.waitForQuery(q.ctx, opts)
		if err != nil {
			q.markDone(err)
			return
		}
		select {
		case <-time.After(backoff.Pause()):
		case <-q.ctx.Done():
			q.markDone(q.ctx.Err())
			return
		}
	}
	q.markDone(nil)
}

func (q *Query) markDone(err error) {
	q.mu.Lock()
	defer q.mu.Unlock()

	// Check if already done to prevent panic on closing closed channel.
	select {
	case <-q.ready:
		// Already closed
		return
	default:
		// Not closed yet
		q.err = err
		close(q.ready)
	}
}

func (q *Query) waitForQuery(ctx context.Context, opts []gax.CallOption) error {
	res, err := q.h.c.GetQueryResults(ctx, &bigquerypb.GetQueryResultsRequest{
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

// Common fields from jobs.query and jobs.getQueryResults
// Needs to be updated as new fields are consumed
type queryResponse interface {
	GetJobComplete() *wrapperspb.BoolValue
	GetJobReference() *bigquerypb.JobReference
	GetTotalRows() *wrapperspb.UInt64Value
}

func (q *Query) consumeQueryResponse(res queryResponse) {
	q.mu.Lock()
	defer q.mu.Unlock()

	if res.GetJobComplete() != nil {
		q.complete = res.GetJobComplete().GetValue()
	}

	jobRef := res.GetJobReference()
	if jobRef != nil {
		q.projectID = jobRef.GetProjectId()
		q.jobID = jobRef.GetJobId()
		if jobRef.GetLocation() != nil {
			q.location = jobRef.GetLocation().GetValue()
		}
	}

	if res.GetTotalRows() != nil {
		q.cachedTotalRows = res.GetTotalRows().GetValue()
	}

	// TODO: save schema, page token, total rows and parse rows
}

// QueryID returns the auto-generated ID for the query.
// This is only populated for stateless queries (i.e. those started via jobs.query)
// after the query has been submitted.
func (q *Query) QueryID() string {
	q.mu.RLock()
	defer q.mu.RUnlock()
	return q.queryID
}

// JobReference returns a reference to the query job.
// This will be nil until the query job has been successfully submitted.
func (q *Query) JobReference() *bigquerypb.JobReference {
	q.mu.RLock()
	defer q.mu.RUnlock()
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
// This will be nil until the query has completed and the schema is available.
func (q *Query) Schema() *bigquerypb.TableSchema {
	return nil // TODO: fill schema
}

// Complete returns true if the query job has finished execution.
func (q *Query) Complete() bool {
	q.mu.RLock()
	defer q.mu.RUnlock()
	return q.complete
}

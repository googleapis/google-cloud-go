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
	"sync"

	"cloud.google.com/go/bigquery/v2/apiv2/bigquerypb"
	internaljob "cloud.google.com/go/bigquery/v2/internal/job"
	"github.com/googleapis/gax-go/v2"
	"google.golang.org/protobuf/reflect/protoreflect"
	"google.golang.org/protobuf/types/known/wrapperspb"
)

// Query represents a handle to a query job. Its methods can be used to wait for
// the job to complete and to iterate over the results.
type Query struct {
	h     *Helper
	inner *internaljob.Handler

	queryID string

	// context for background pooling
	ctx context.Context
	mu  sync.RWMutex

	cachedTotalRows uint64
}

// Create Query handler using jobs.query request and start background pooling job
func newQueryJobFromQueryRequest(ctx context.Context, h *Helper, req *bigquerypb.PostQueryRequest, opts ...gax.CallOption) *Query {
	q := &Query{
		h:   h,
		ctx: ctx,
	}
	q.inner = internaljob.NewHandler(ctx, q.runQueryFunc(req), q.waitFunc, nil)
	q.inner.Start(opts)

	return q
}

// Create Query handler using jobs.insert request and start background pooling job
func newQueryJobFromJob(ctx context.Context, h *Helper, projectID string, j *bigquerypb.Job, opts ...gax.CallOption) *Query {
	q := &Query{
		h:   h,
		ctx: ctx,
	}
	q.inner = internaljob.NewHandler(ctx, q.insertQueryFunc(j, projectID, opts), q.waitFunc, nil)
	q.inner.Start(opts)

	return q
}

// Create Query handler from JobReference response and start background pooling job
func newQueryJobFromJobReference(ctx context.Context, h *Helper, jobRef *bigquerypb.JobReference, opts ...gax.CallOption) *Query {
	q := &Query{
		h:   h,
		ctx: ctx,
	}
	q.inner = internaljob.NewHandler(ctx, nil, q.waitFunc, jobRef)
	q.inner.Start(opts)

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
	return q.inner.Wait(ctx)
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
	return q.inner.Done()
}

// Err returns the final error state of the query. It is only valid to call Err
// after the channel returned by Done has been closed. If the query completed
// successfully, Err returns nil.
func (q *Query) Err() error {
	return q.inner.Err()
}

func (q *Query) insertQueryFunc(job *bigquerypb.Job, projectID string, opts []gax.CallOption) internaljob.CreateFunc {
	return func(ctx context.Context, opts []gax.CallOption) (protoreflect.Message, error) {
		res, err := q.h.c.InsertJob(q.ctx, &bigquerypb.InsertJobRequest{
			ProjectId: projectID,
			Job:       job,
		}, opts...)

		if err != nil {
			return nil, err
		}

		return res.ProtoReflect(), nil
	}
}

func (q *Query) runQueryFunc(req *bigquerypb.PostQueryRequest) internaljob.CreateFunc {
	return func(ctx context.Context, opts []gax.CallOption) (protoreflect.Message, error) {
		res, err := q.h.c.Query(q.ctx, req, opts...)
		if err != nil {
			return nil, err
		}
		q.queryID = res.GetQueryId()

		q.consumeQueryResponse(res)
		return res.ProtoReflect(), nil
	}
}

func (q *Query) waitFunc(ctx context.Context, opts []gax.CallOption) (protoreflect.Message, error) {
	jobRef := q.inner.JobReference()
	if jobRef == nil {
		return nil, fmt.Errorf("bigquery: job reference is missing, can't wait for query to complete")
	}
	location := ""
	if jobRef.GetLocation() != nil {
		location = jobRef.GetLocation().GetValue()
	}
	res, err := q.h.c.GetQueryResults(ctx, &bigquerypb.GetQueryResultsRequest{
		ProjectId:  jobRef.GetProjectId(),
		JobId:      jobRef.GetJobId(),
		Location:   location,
		MaxResults: wrapperspb.UInt32(0),
		FormatOptions: &bigquerypb.DataFormatOptions{
			UseInt64Timestamp: true,
		},
	}, opts...)
	if err != nil {
		return nil, err
	}

	q.consumeQueryResponse(res)

	return res.ProtoReflect(), nil
}

// Common fields from jobs.query and jobs.getQueryResults
// Needs to be updated as new fields are consumed
type queryResponse interface {
	GetTotalRows() *wrapperspb.UInt64Value
}

func (q *Query) consumeQueryResponse(res queryResponse) {
	q.mu.Lock()
	defer q.mu.Unlock()

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
	return q.inner.JobReference()
}

// Schema returns the schema of the query results.
// This will be nil until the query has completed and the schema is available.
func (q *Query) Schema() *bigquerypb.TableSchema {
	return nil // TODO: fill schema
}

// Complete returns true if the query job has finished execution.
func (q *Query) Complete() bool {
	return q.inner.Complete()
}

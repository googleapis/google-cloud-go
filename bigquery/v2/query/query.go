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
type QueryJob struct {
	c         *QueryClient
	complete  bool
	projectID string
	jobID     string
	location  string

	cachedTotalRows uint64
	cachedPageToken string
	cachedRows      []*Row
	cachedSchema    *bigquerypb.TableSchema
}

func newQueryJobFromQueryResponse(c *QueryClient, res *bigquerypb.QueryResponse) *QueryJob {
	j := &QueryJob{
		c:               c,
		cachedSchema:    res.Schema,
		cachedPageToken: res.PageToken,
	}
	if res.TotalRows != nil {
		j.cachedTotalRows = res.TotalRows.Value
	}
	if res.JobComplete != nil {
		j.complete = res.JobComplete.Value
	}
	if res.Rows != nil {
		j.cachedRows = fieldValueRowsToRowList(res.Rows, res.Schema)
	}
	if res.JobReference != nil {
		jobRef := res.JobReference
		j.projectID = jobRef.ProjectId
		j.jobID = jobRef.JobId
		if jobRef.Location != nil {
			j.location = jobRef.Location.GetValue()
		}
	}
	return j
}

func newQueryJobFromJob(c *QueryClient, job *bigquerypb.Job) *QueryJob {
	return newQueryJobFromJobReference(c, nil, job.JobReference)
}

func newQueryJobFromJobReference(c *QueryClient, schema *bigquerypb.TableSchema, jobRef *bigquerypb.JobReference) *QueryJob {
	res := &bigquerypb.QueryResponse{
		Schema:       schema,
		JobReference: jobRef,
	}
	return newQueryJobFromQueryResponse(c, res)
}

// state is one of a sequence of states that a Job progresses through as it is processed.
type state = string

const (
	// Pending is a state that describes that the job is pending.
	Pending state = "PENDING"
	// Running is a state that describes that the job is running.
	Running state = "RUNNING"
	// Done is a state that describes that the job is done.
	Done state = "DONE"
)

func (j *QueryJob) checkStatus(ctx context.Context) error {
	res, err := j.c.c.GetQueryResults(ctx, &bigquerypb.GetQueryResultsRequest{
		ProjectId:  j.projectID,
		JobId:      j.jobID,
		Location:   j.location,
		MaxResults: wrapperspb.UInt32(0),
	})
	if err != nil {
		return err
	}

	j.consumeQueryResponse(res)

	return nil
}

func (j *QueryJob) consumeQueryResponse(res *bigquerypb.GetQueryResultsResponse) {
	j.cachedPageToken = res.PageToken
	j.cachedRows = fieldValueRowsToRowList(res.Rows, res.Schema)
	j.cachedSchema = res.Schema
	j.cachedTotalRows = res.TotalRows.Value
	if res.JobComplete != nil {
		j.complete = res.JobComplete.Value
	}
}

// Wait waits for the query to complete.
func (j *QueryJob) Wait(ctx context.Context) error {
	for !j.complete {
		err := j.checkStatus(ctx)
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

// GetJobReference returns the job reference.
func (j *QueryJob) JobReference() *bigquerypb.JobReference {
	return &bigquerypb.JobReference{
		ProjectId: j.projectID,
		JobId:     j.jobID,
		Location:  wrapperspb.String(j.location),
	}
}

// GetSchema returns the schema of the query results.
func (j *QueryJob) Schema() *bigquerypb.TableSchema {
	return j.cachedSchema
}

func (j *QueryJob) Complete() bool {
	return j.complete
}

// Read returns a RowIterator for the query results.
func (j *QueryJob) Read(ctx context.Context) (*RowIterator, error) {
	return &RowIterator{
		query:     j,
		c:         j.c,
		rows:      j.cachedRows,
		totalRows: j.cachedTotalRows,
		schema:    j.cachedSchema,
		pageToken: j.cachedPageToken,
	}, nil
}

func (j *QueryJob) getRows(ctx context.Context, pageToken string) (*bigquerypb.GetQueryResultsResponse, error) {
	return j.c.c.GetQueryResults(ctx, &bigquerypb.GetQueryResultsRequest{
		FormatOptions: &bigquerypb.DataFormatOptions{
			UseInt64Timestamp: true,
		},
		JobId:     j.jobID,
		ProjectId: j.projectID,
		Location:  j.location,
		PageToken: pageToken,
	})
}

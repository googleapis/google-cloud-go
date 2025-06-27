// Copyright 2024 Google LLC
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
)

// QueryRunner is used to run a query.
type QueryRunner struct {
	c *QueryClient
}

//TODO: how to setup job timeout

// StartQuery runs a query and returns a QueryJob handle.
func (qr *QueryRunner) StartQuery(ctx context.Context, req *bigquerypb.PostQueryRequest, opts ...gax.CallOption) (*QueryJob, error) {
	res, err := qr.c.c.Query(ctx, req)
	if err != nil {
		return nil, fmt.Errorf("failed to run query: %w", err)
	}

	return newQueryJobFromQueryResponse(qr.c, res), nil
}

// StartQueryRequest runs a query and returns a QueryJob handle.
func (qr *QueryRunner) StartQueryRequest(ctx context.Context, req *bigquerypb.QueryRequest, opts ...gax.CallOption) (*QueryJob, error) {
	return qr.StartQuery(ctx, &bigquerypb.PostQueryRequest{
		QueryRequest: req,
		ProjectId:    qr.c.billingProjectID,
	})
}

// StartJob from a bigquerypb.Job definition. Should have job.Configuration.Query filled out.
func (qr *QueryRunner) StartQueryJob(ctx context.Context, job *bigquerypb.Job, opts ...gax.CallOption) (*QueryJob, error) {
	qconfig := job.Configuration.Query
	if qconfig == nil {
		return nil, fmt.Errorf("job is not a query")
	}
	job, err := qr.c.c.InsertJob(ctx, &bigquerypb.InsertJobRequest{
		ProjectId: qr.c.billingProjectID,
		Job:       job,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to insert query: %w", err)
	}
	return newQueryJobFromJob(qr.c, job), nil
}

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
	"cloud.google.com/go/bigquery/v2/apiv2_client"
	"cloud.google.com/go/internal/uid"
	"github.com/googleapis/gax-go/v2"
	"google.golang.org/api/option"
)

// Client is a client for running queries in BigQuery. It is a lightweight wrapper
// around the auto-generated BigQuery v2 client, focused on query operations.
type Client struct {
	c         *apiv2_client.Client
	projectID string
}

// NewClient creates a new query client. A client should be reused instead of
// created per-request. The client must be closed when it is no longer needed.
func NewClient(ctx context.Context, projectID string, opts ...option.ClientOption) (*Client, error) {
	qc := &Client{
		projectID: projectID,
	}
	for _, opt := range opts {
		if cOpt, ok := opt.(*customClientOption); ok {
			cOpt.ApplyCustomClientOpt(qc)
		}
	}
	if qc.c == nil {
		c, err := apiv2_client.NewClient(ctx, opts...)
		if err != nil {
			return nil, fmt.Errorf("failed to setup bigquery client: %w", err)
		}
		qc.c = c
	}
	return qc, nil
}

// StartQuery executes a query using the stateless jobs.query RPC. It returns a
// handle to the running query. The returned Query object can be used to wait for
// completion and retrieve results.
func (c *Client) StartQuery(ctx context.Context, req *bigquerypb.PostQueryRequest, opts ...gax.CallOption) (*Query, error) {
	if req.QueryRequest.RequestId == "" {
		req.QueryRequest.RequestId = uid.NewSpace("request", nil).New()
	}

	return newQueryJobFromQueryRequest(ctx, c, req, opts...), nil
}

// StartQueryJob from a bigquerypb.Job definition. Should have job.Configuration.Query filled out.
func (c *Client) StartQueryJob(ctx context.Context, job *bigquerypb.Job, opts ...gax.CallOption) (*Query, error) {
	config := job.GetConfiguration()
	if config == nil {
		return nil, fmt.Errorf("bigquery: job is missing configuration")
	}
	qconfig := config.Query
	if qconfig == nil {
		return nil, fmt.Errorf("bigquery: job is not a query")
	}

	jobRef := job.GetJobReference()
	if jobRef == nil {
		jobRef = &bigquerypb.JobReference{}
	}
	if jobRef.JobId == "" {
		jobRef.JobId = uid.NewSpace("job", nil).New()
	}
	job.JobReference = jobRef

	return newQueryJobFromJob(ctx, c, c.projectID, job, opts...), nil
}

// AttachJob attaches to an existing query job. The returned Query object can be
// used to monitor the job's status, wait for its completion, and retrieve its
// results.
func (c *Client) AttachJob(ctx context.Context, jobRef *bigquerypb.JobReference, opts ...gax.CallOption) (*Query, error) {
	if jobRef == nil {
		return nil, fmt.Errorf("bigquery: AttachJob requires a non-nil JobReference")
	}
	if jobRef.GetJobId() == "" {
		return nil, fmt.Errorf("bigquery: AttachJob requires a non-empty JobReference.JobId")
	}
	if jobRef.GetProjectId() == "" {
		jobRef.ProjectId = c.projectID
	}
	return newQueryJobFromJobReference(ctx, c, jobRef, opts...), nil
}

// Close closes the connection to the API service. The user should invoke this when
// the client is no longer required.
func (c *Client) Close() error {
	return c.c.Close()
}

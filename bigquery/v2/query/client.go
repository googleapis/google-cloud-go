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

	storage "cloud.google.com/go/bigquery/storage/apiv1"
	"cloud.google.com/go/bigquery/v2/apiv2/bigquerypb"
	"cloud.google.com/go/bigquery/v2/apiv2_client"
	"github.com/googleapis/gax-go/v2"
	"google.golang.org/api/option"
)

// Client is a client for running queries in BigQuery.
type Client struct {
	c                      *apiv2_client.Client
	rc                     *storage.BigQueryReadClient
	projectID              string
	billingProjectID       string
	defaultJobCreationMode bigquerypb.QueryRequest_JobCreationMode
}

// NewClient creates a new query client.
func NewClient(ctx context.Context, projectID string, opts ...option.ClientOption) (*Client, error) {
	qc := &Client{
		projectID:        projectID,
		billingProjectID: projectID,
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

// StartQuery runs a query and returns a QueryJob handle.
func (c *Client) StartQuery(ctx context.Context, req *bigquerypb.PostQueryRequest, opts ...gax.CallOption) (*Query, error) {
	if req.QueryRequest.JobCreationMode == bigquerypb.QueryRequest_JOB_CREATION_MODE_UNSPECIFIED {
		req.QueryRequest.JobCreationMode = c.defaultJobCreationMode
	}
	res, err := c.c.Query(ctx, req, opts...)
	if err != nil {
		return nil, fmt.Errorf("failed to run query: %w", err)
	}

	return newQueryJobFromQueryResponse(c, res)
}

// StartQueryRequest runs a query and returns a QueryJob handle.
func (c *Client) StartQueryRequest(ctx context.Context, req *bigquerypb.QueryRequest, opts ...gax.CallOption) (*Query, error) {
	return c.StartQuery(ctx, &bigquerypb.PostQueryRequest{
		QueryRequest: req,
		ProjectId:    c.billingProjectID,
	}, opts...)
}

// StartQueryJob from a bigquerypb.Job definition. Should have job.Configuration.Query filled out.
func (c *Client) StartQueryJob(ctx context.Context, job *bigquerypb.Job, opts ...gax.CallOption) (*Query, error) {
	qconfig := job.Configuration.Query
	if qconfig == nil {
		return nil, fmt.Errorf("job is not a query")
	}
	job, err := c.c.InsertJob(ctx, &bigquerypb.InsertJobRequest{
		ProjectId: c.billingProjectID,
		Job:       job,
	}, opts...)
	if err != nil {
		return nil, fmt.Errorf("failed to insert query: %w", err)
	}
	return newQueryJobFromJobReference(c, job.JobReference)
}

// AttachJob set up a query job to be read from an existing one.
func (c *Client) AttachJob(ctx context.Context, jobRef *bigquerypb.JobReference, opts ...ReadOption) (*Query, error) {
	return newQueryJobFromJobReference(c, jobRef)
}

// Close closes the connection to the API service. The user should invoke this when
// the client is no longer required.
func (c *Client) Close() error {
	return c.c.Close()
}

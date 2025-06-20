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

package apiv2_client

import (
	"context"

	bigquery "cloud.google.com/go/bigquery/v2/apiv2"
	"cloud.google.com/go/bigquery/v2/apiv2/bigquerypb"

	gax "github.com/googleapis/gax-go/v2"
)

// CancelJob requests that a job be cancelled. This call will return immediately, and
// the client will need to poll for the job status to see if the cancel
// completed successfully. Cancelled jobs may still incur costs.
func (mc *Client) CancelJob(ctx context.Context, req *bigquerypb.CancelJobRequest, opts ...gax.CallOption) (*bigquerypb.JobCancelResponse, error) {
	return mc.jobClient.CancelJob(ctx, req, opts...)
}

// GetJob returns information about a specific job. Job information is available for
// a six month period after creation. Requires that you’re the person who ran
// the job, or have the Is Owner project role.
func (mc *Client) GetJob(ctx context.Context, req *bigquerypb.GetJobRequest, opts ...gax.CallOption) (*bigquerypb.Job, error) {
	return mc.jobClient.GetJob(ctx, req, opts...)
}

// InsertJob starts a new asynchronous job.
//
// This API has two different kinds of endpoint URIs, as this method supports
// a variety of use cases.
//
//	The Metadata URI is used for most interactions, as it accepts the job
//	configuration directly.
//
//	The Upload URI is ONLY for the case when you’re sending both a load job
//	configuration and a data stream together.  In this case, the Upload URI
//	accepts the job configuration and the data as two distinct multipart MIME
//	parts.
func (mc *Client) InsertJob(ctx context.Context, req *bigquerypb.InsertJobRequest, opts ...gax.CallOption) (*bigquerypb.Job, error) {
	return mc.jobClient.InsertJob(ctx, req, opts...)
}

// DeleteJob requests the deletion of the metadata of a job. This call returns when the
// job’s metadata is deleted.
func (mc *Client) DeleteJob(ctx context.Context, req *bigquerypb.DeleteJobRequest, opts ...gax.CallOption) error {
	return mc.jobClient.DeleteJob(ctx, req, opts...)
}

// ListJobs lists all jobs that you started in the specified project. Job information
// is available for a six month period after creation. The job list is sorted
// in reverse chronological order, by job creation time. Requires the Can View
// project role, or the Is Owner project role if you set the allUsers
// property.
func (mc *Client) ListJobs(ctx context.Context, req *bigquerypb.ListJobsRequest, opts ...gax.CallOption) *bigquery.ListFormatJobIterator {
	return mc.jobClient.ListJobs(ctx, req, opts...)
}

// GetQueryResults rPC to get the results of a query job.
func (mc *Client) GetQueryResults(ctx context.Context, req *bigquerypb.GetQueryResultsRequest, opts ...gax.CallOption) (*bigquerypb.GetQueryResultsResponse, error) {
	return mc.jobClient.GetQueryResults(ctx, req, opts...)
}

// Query runs a BigQuery SQL query synchronously and returns query results if the
// query completes within a specified timeout.
func (mc *Client) Query(ctx context.Context, req *bigquerypb.PostQueryRequest, opts ...gax.CallOption) (*bigquerypb.QueryResponse, error) {
	return mc.jobClient.Query(ctx, req, opts...)
}

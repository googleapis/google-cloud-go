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
	"errors"
	"fmt"

	"cloud.google.com/go/bigquery/v2/apiv2/bigquerypb"
	"cloud.google.com/go/bigquery/v2/apiv2_client"
	"cloud.google.com/go/internal/uid"
	"github.com/googleapis/gax-go/v2"
	"google.golang.org/api/option"
	"google.golang.org/protobuf/proto"
)

// Helper for running queries in BigQuery. It is a lightweight wrapper
// around the auto-generated BigQuery v2 client, focused on query operations.
type Helper struct {
	c         *apiv2_client.Client
	projectID string
}

// NewHelper creates a new query helper. This helper should be reused instead of
// created per-request.
func NewHelper(c *apiv2_client.Client, projectID string, opts ...option.ClientOption) (*Helper, error) {
	qc := &Helper{
		c:         c,
		projectID: projectID,
	}
	if qc.c == nil {
		return nil, errors.New("missing bigquery client")
	}
	return qc, nil
}

// StartQuery executes a query using the stateless jobs.query RPC. It returns a
// handle to the running query. The returned Query object can be used to wait for
// completion and retrieve results.
func (h *Helper) StartQuery(ctx context.Context, req *bigquerypb.PostQueryRequest, opts ...gax.CallOption) (*Query, error) {
	req = proto.Clone(req).(*bigquerypb.PostQueryRequest)
	qr := req.GetQueryRequest()
	if qr == nil {
		return nil, fmt.Errorf("bigquery: request is missing QueryRequest")
	}
	if qr.GetRequestId() == "" {
		qr.RequestId = uid.NewSpace("request", nil).New()
	}

	return newQueryJobFromQueryRequest(ctx, h, req, opts...), nil
}

// StartQueryJob from a bigquerypb.Job definition. Should have job.Configuration.Query filled out.
func (h *Helper) StartQueryJob(ctx context.Context, job *bigquerypb.Job, opts ...gax.CallOption) (*Query, error) {
	job = proto.Clone(job).(*bigquerypb.Job)
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
		job.JobReference = jobRef
	}
	if jobRef.GetJobId() == "" {
		jobRef.JobId = uid.NewSpace("job", nil).New()
	}
	if jobRef.GetProjectId() == "" {
		jobRef.ProjectId = h.projectID
	}

	return newQueryJobFromJob(ctx, h, h.projectID, job, opts...), nil
}

// AttachJob attaches to an existing query job. The returned Query object can be
// used to monitor the job's status, wait for its completion, and retrieve its
// results.
func (h *Helper) AttachJob(ctx context.Context, jobRef *bigquerypb.JobReference, opts ...gax.CallOption) (*Query, error) {
	jobRef = proto.Clone(jobRef).(*bigquerypb.JobReference)
	if jobRef == nil {
		return nil, fmt.Errorf("bigquery: AttachJob requires a non-nil JobReference")
	}
	if jobRef.GetJobId() == "" {
		return nil, fmt.Errorf("bigquery: AttachJob requires a non-empty JobReference.JobId")
	}
	if jobRef.GetProjectId() == "" {
		jobRef.ProjectId = h.projectID
	}
	return newQueryJobFromJobReference(ctx, h, jobRef, opts...), nil
}

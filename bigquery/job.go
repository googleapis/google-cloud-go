// Copyright 2015 Google Inc. All Rights Reserved.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//      http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package bigquery

import (
	"golang.org/x/net/context"
	bq "google.golang.org/api/bigquery/v2"
)

// A Job represents an operation which has been submitted to BigQuery for processing.
type Job struct {
	service   service
	projectID string
	jobID     string

	isQuery bool
}

// JobFromID creates a Job which refers to an existing BigQuery job. The job
// need not have been created by this package. For example, the job may have
// been created in the BigQuery console.
func (c *Client) JobFromID(ctx context.Context, id string) (*Job, error) {
	jobType, err := c.service.getJobType(ctx, c.projectID, id)
	if err != nil {
		return nil, err
	}

	return &Job{
		service:   c.service,
		projectID: c.projectID,
		jobID:     id,
		isQuery:   jobType == queryJobType,
	}, nil
}

func (j *Job) ID() string {
	return j.jobID
}

// State is one of a sequence of states that a Job progresses through as it is processed.
type State int

const (
	Pending State = iota
	Running
	Done
)

// JobStatus contains the current State of a job, and errors encountered while processing that job.
type JobStatus struct {
	State State

	err error

	// All errors encountered during the running of the job.
	// Not all Errors are fatal, so errors here do not necessarily mean that the job has completed or was unsuccessful.
	Errors []*Error
}

// setJobRef initializes job's JobReference if given a non-empty jobID.
// projectID must be non-empty.
func setJobRef(job *bq.Job, jobID, projectID string) {
	if jobID == "" {
		return
	}
	// We don't check whether projectID is empty; the server will return an
	// error when it encounters the resulting JobReference.

	job.JobReference = &bq.JobReference{
		JobId:     jobID,
		ProjectId: projectID,
	}
}

// Done reports whether the job has completed.
// After Done returns true, the Err method will return an error if the job completed unsuccesfully.
func (s *JobStatus) Done() bool {
	return s.State == Done
}

// Err returns the error that caused the job to complete unsuccesfully (if any).
func (s *JobStatus) Err() error {
	return s.err
}

// Status returns the current status of the job. It fails if the Status could not be determined.
func (j *Job) Status(ctx context.Context) (*JobStatus, error) {
	return j.service.jobStatus(ctx, j.projectID, j.jobID)
}

// Cancel requests that a job be cancelled. This method returns without waiting for
// cancellation to take effect. To check whether the job has terminated, use Job.Status.
// Cancelled jobs may still incur costs.
func (j *Job) Cancel(ctx context.Context) error {
	return j.service.jobCancel(ctx, j.projectID, j.jobID)
}

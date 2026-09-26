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

package job

import (
	"context"
	"sync"
	"time"

	"cloud.google.com/go/bigquery/v2/apiv2/bigquerypb"
	"github.com/googleapis/gax-go/v2"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/reflect/protoreflect"
	"google.golang.org/protobuf/types/known/wrapperspb"
)

// Handler is responsible for managing the lifecycle of a BigQuery job.
// It provides mechanisms for starting, waiting for, and checking the status of a job.
type Handler struct {
	create CreateFunc
	wait   WaitFunc

	jobRef *bigquerypb.JobReference

	// context for background pooling
	ctx      context.Context
	mu       sync.RWMutex
	complete bool
	ready    chan struct{}
	err      error
}

// CreateFunc is a function that start/insert a new job in BigQuery. Usually jobs.insert or jobs.query.
type CreateFunc = func(ctx context.Context, opts []gax.CallOption) (protoreflect.Message, error)

// WaitFunc is a function that check if a job is complete. Usually jobs.getQueryResults or jobs.get.
type WaitFunc = func(ctx context.Context, opts []gax.CallOption) (protoreflect.Message, error)

// NewHandler creates a new job handler.
func NewHandler(ctx context.Context, create CreateFunc, wait WaitFunc, jobRef *bigquerypb.JobReference) *Handler {
	jh := &Handler{
		ctx:    ctx,
		ready:  make(chan struct{}),
		create: create,
		wait:   wait,
		jobRef: jobRef,
	}

	return jh
}

// Start begins the job creation and background polling process.
func (jh *Handler) Start(opts []gax.CallOption) {
	go jh.start(opts)
}

// start initiates the job creation and polling process.
func (jh *Handler) start(opts []gax.CallOption) {
	if jh.create != nil {
		res, err := jh.create(jh.ctx, opts)
		if err != nil {
			jh.markDone(err)
			return
		}
		jh.consumeResponse(res)
	}
	jh.waitInBackground(opts)
}

// Wait blocks until the query has completed. The provided context can be used to
// cancel the wait. If the query completes successfully, Wait returns nil.
// Otherwise, it returns the error that caused the query to fail.
//
// Wait is a convenience wrapper around Done and Err.
func (jh *Handler) Wait(ctx context.Context) error {
	select {
	case <-jh.Done():
		return jh.Err()
	case <-ctx.Done():
		return ctx.Err()
	}
}

// Done returns a channel that is closed when the job has completed.
// It can be used in a select statement to perform non-blocking waits.
//
// Example:
//
//	select {
//	case <-jh.Done():
//		if err := jh.Err(); err != nil {
//			// Handle error.
//		}
//		// Job is complete.
//	 case <-time.After(30*time.Second):
//		    // Timeout logic
//	default:
//		// Job is still running.
//	}
func (jh *Handler) Done(opts ...gax.CallOption) <-chan struct{} {
	return jh.ready
}

// Err returns the final error state of the job. It is only valid to call Err
// after the channel returned by Done has been closed. If the job completed
// successfully, Err returns nil.
func (jh *Handler) Err() error {
	jh.mu.RLock()
	defer jh.mu.RUnlock()
	if jh.ctx.Err() != nil {
		return jh.ctx.Err()
	}
	return jh.err
}

// waitInBackground polls the job status with exponential backoff until the job is complete.
func (jh *Handler) waitInBackground(opts []gax.CallOption) {
	backoff := gax.Backoff{
		Initial:    50 * time.Millisecond,
		Multiplier: 1.3,
		Max:        60 * time.Second,
	}
	for !jh.complete {
		m, err := jh.wait(jh.ctx, opts)
		if err != nil {
			jh.markDone(err)
			return
		}
		jh.consumeResponse(m)
		select {
		case <-time.After(backoff.Pause()):
		case <-jh.ctx.Done():
			jh.markDone(jh.ctx.Err())
			return
		}
	}
	jh.markDone(nil)
}

// markDone marks the job as complete and records the final error state.
func (jh *Handler) markDone(err error) {
	jh.mu.Lock()
	defer jh.mu.Unlock()

	// Check if already done to prevent panic on closing closed channel.
	select {
	case <-jh.ready:
		// Already closed
		return
	default:
		// Not closed yet
		jh.err = err
		close(jh.ready)
	}
}

// consumeResponse processes a response from the API, updating the job's state.
func (jh *Handler) consumeResponse(m protoreflect.Message) {
	jh.mu.Lock()
	defer jh.mu.Unlock()

	status := getJobStatus(m)
	if status != nil {
		jh.complete = status.GetState() == "DONE"
	}

	jobComplete := getJobComplete(m)
	if jobComplete != nil {
		jh.complete = jobComplete.GetValue()
	}

	jobRef := getJobReference(m)
	if jobRef != nil {
		jh.jobRef = jobRef
	}
}

// getJobStatus extracts the job status from a protobuf message.
func getJobStatus(m protoreflect.Message) *bigquerypb.JobStatus {
	statusField := m.Descriptor().Fields().ByName("status")
	if statusField == nil {
		return nil
	}
	statusSrc := m.Get(statusField)

	if statusSrc.IsValid() {
		status := &bigquerypb.JobStatus{}
		proto.Merge(status, statusSrc.Message().Interface())
		return status
	}

	return nil
}

// getJobComplete extracts the job completion status from a protobuf message.
func getJobComplete(m protoreflect.Message) *wrapperspb.BoolValue {
	jobCompleteField := m.Descriptor().Fields().ByName("job_complete")
	if jobCompleteField == nil {
		return nil
	}
	jobCompleteSrc := m.Get(jobCompleteField)

	if jobCompleteSrc.IsValid() {
		jobComplete := &wrapperspb.BoolValue{}
		proto.Merge(jobComplete, jobCompleteSrc.Message().Interface())
		return jobComplete
	}

	return nil
}

// getJobReference extracts the job reference from a protobuf message.
func getJobReference(m protoreflect.Message) *bigquerypb.JobReference {
	jobRefField := m.Descriptor().Fields().ByName("job_reference")
	if jobRefField == nil {
		return nil
	}
	jobRefSrc := m.Get(jobRefField)

	if jobRefSrc.IsValid() {
		jobRef := &bigquerypb.JobReference{}
		proto.Merge(jobRef, jobRefSrc.Message().Interface())
		return jobRef
	}

	return nil
}

// JobReference returns a reference to the job.
// This will be nil until the job has been successfully submitted.
func (jh *Handler) JobReference() *bigquerypb.JobReference {
	jh.mu.RLock()
	defer jh.mu.RUnlock()
	return jh.jobRef
}

// Complete returns true if the job has finished execution.
func (jh *Handler) Complete() bool {
	jh.mu.RLock()
	defer jh.mu.RUnlock()
	return jh.complete
}

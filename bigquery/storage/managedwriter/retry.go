// Copyright 2021 Google LLC
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

package managedwriter

import (
	"context"
	"errors"
	"io"
	"strings"
	"time"

	"github.com/googleapis/gax-go/v2"
	"github.com/googleapis/gax-go/v2/apierror"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

var (
	defaultAppendRetries = 3
	knownReconnectErrors = []error{
		io.EOF,
		status.Error(codes.Unavailable, "the connection is draining"), // errStreamDrain in gRPC transport
	}
)

func newDefaultRetryer() *defaultRetryer {
	return &defaultRetryer{
		bigBo: gax.Backoff{
			Initial:    2 * time.Second,
			Multiplier: 5,
			Max:        5 * time.Minute,
		},
	}
}

type defaultRetryer struct {
	bo    gax.Backoff
	bigBo gax.Backoff // for more aggressive backoff, such as throughput quota
}

func (r *defaultRetryer) Retry(err error) (pause time.Duration, shouldRetry bool) {
	// This predicate evaluates enqueuing.
	s, ok := status.FromError(err)
	if !ok {
		// Treat context errors as non-retriable.
		if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
			return r.bo.Pause(), false
		}
		// Any other non-status based errors treated as retryable.
		return r.bo.Pause(), true
	}
	switch s.Code() {
	case codes.Unavailable:
		return r.bo.Pause(), true
	default:
		return r.bo.Pause(), false
	}
}

func (r *defaultRetryer) RetryAppend(err error, attemptCount int) (pause time.Duration, shouldRetry bool) {
	if err == nil {
		return 0, false // This shouldn't need to be here, and is only provided defensively.
	}
	if attemptCount > defaultAppendRetries {
		return 0, false // exceeded maximum retries.
	}
	// This predicate evaluates the received response to determine if we should re-enqueue.
	apiErr, ok := apierror.FromError(err)
	if !ok {
		// These are non status-based errors.
		// Context errors are non-retriable.
		if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
			return 0, false
		}
		// EOF can happen in the case of connection close before response received.
		if errors.Is(err, io.EOF) {
			return r.bo.Pause(), true
		}
		// Any other non-status based errors are not retried.
		return 0, false
	}
	// Evaluate based on the more generic grpc error status.
	// TODO: Revisit whether we want to include some user-induced
	// race conditions that map into FailedPrecondition once it's clearer whether that's
	// safe to retry by default.
	code := apiErr.GRPCStatus().Code()
	switch code {
	case codes.Aborted,
		codes.Canceled,
		codes.DeadlineExceeded,
		codes.Internal,
		codes.Unavailable:
		return r.bo.Pause(), true
	case codes.ResourceExhausted:
		if strings.HasPrefix(apiErr.GRPCStatus().Message(), "Exceeds 'AppendRows throughput' quota") {
			// Note: internal b/246031522 opened to give this a structured error
			// and avoid string parsing.  Should be a QuotaFailure or similar.
			return r.bigBo.Pause(), true // more aggressive backoff
		}
	}
	// We treat all other failures as non-retriable.
	return 0, false
}

// shouldReconnect is akin to a retry predicate, in that it evaluates whether we should force
// our bidi stream to close/reopen based on the responses error.  Errors here signal that no
// further appends will succeed.
func shouldReconnect(err error) bool {
	for _, ke := range knownReconnectErrors {
		if errors.Is(err, ke) {
			return true
		}
	}
	return false
}

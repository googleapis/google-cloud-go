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
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

var (
	defaultAppendRetries = 3
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
	bigBo gax.Backoff // For more aggressive backoff, such as throughput quota
}

func (r *defaultRetryer) Retry(err error) (pause time.Duration, shouldRetry bool) {
	// This predicate evaluates errors for both enqueuing and reconnection.
	// See RetryAppend for retry that bounds attempts to a fixed number.
	s, ok := status.FromError(err)
	if !ok {
		// Treat context errors as non-retriable.
		if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
			return r.bo.Pause(), false
		}
		// EOF can happen in the case of connection close.
		if errors.Is(err, io.EOF) {
			return r.bo.Pause(), true
		}
		// Any other non-status based errors treated as retryable.
		return r.bo.Pause(), true
	}
	switch s.Code() {
	case codes.Aborted,
		codes.Canceled,
		codes.DeadlineExceeded,
		codes.Internal,
		codes.Unavailable:
		return r.bo.Pause(), true
	case codes.ResourceExhausted:
		if strings.HasPrefix(s.Message(), "Exceeds 'AppendRows throughput' quota") {
			// Note: internal b/246031522 opened to give this a structured error
			// and avoid string parsing.  Should be a QuotaFailure or similar.
			return r.bigBo.Pause(), true // more aggressive backoff
		}
	}
	return 0, false
}

// RetryAppend is a variation of the retry predicate that also bounds retries to a finite number of attempts.
func (r *defaultRetryer) RetryAppend(err error, attemptCount int) (pause time.Duration, shouldRetry bool) {

	if attemptCount > defaultAppendRetries {
		return 0, false // exceeded maximum retries.
	}
	return r.Retry(err)
}

// shouldReconnect is akin to a retry predicate, in that it evaluates whether we should force
// our bidi stream to close/reopen based on the responses error.  Errors here signal that no
// further appends will succeed.
func shouldReconnect(err error) bool {
	var knownErrors = []error{
		io.EOF,
		status.Error(codes.Unavailable, "the connection is draining"), // errStreamDrain in gRPC transport
	}
	for _, ke := range knownErrors {
		if errors.Is(err, ke) {
			return true
		}
	}
	return false
}

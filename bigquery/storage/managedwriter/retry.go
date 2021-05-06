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
	"time"

	"github.com/googleapis/gax-go"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

type defaultRetryer struct {
	bo gax.Backoff
}

// Retry governs whether an error is considered retriable, and an appropriate backoff.
//
// ALREADY_EXISTS: happens when offset is specified, it means the entire
// request is already appended, it is safe to ignore this error.
//
// OUT_OF_RANGE: Occurs when specified offset is beyond the end of the
// stream.  Expected when we have a transient failure and a subsequent
// append gets processed with the old offset.
//
// RESOURCE_EXHAUSTED: normal throttling.
//
// ABORTED: failed due to prior failures.
//
// INTERNAL: backend errors that aren't classified further.
//
func (r defaultRetryer) Retry(err error) (pause time.Duration, shouldRetry bool) {
	s, ok := status.FromError(err)
	if !ok {
		// things which aren't gRPC status
		return r.bo.Pause(), true
	}
	switch s.Code() {
	case codes.AlreadyExists,
		codes.Internal, codes.ResourceExhausted, codes.Aborted, codes.OutOfRange:
		return r.bo.Pause(), true
	default:
		return 0, false
	}
}

type streamRetryer struct {
	defaultRetryer gax.Retryer
}

func (r *streamRetryer) Retry(err error) (pause time.Duration, shouldRetry bool) {
	s, ok := status.FromError(err)
	if !ok {
		return r.defaultRetryer.Retry(err)
	}
	switch s.Code() {
	case codes.ResourceExhausted:
		return 0, false
	default:
		return r.defaultRetryer.Retry(err)
	}
}

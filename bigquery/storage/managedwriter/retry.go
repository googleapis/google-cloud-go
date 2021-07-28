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

	"github.com/googleapis/gax-go/v2"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

type defaultRetryer struct {
	bo gax.Backoff
}

func (r *defaultRetryer) Retry(err error) (pause time.Duration, shouldRetry bool) {
	// TODO: refine this logic in a subsequent PR, there's some service-specific
	// retry predicates in addition to statuscode-based.
	s, ok := status.FromError(err)
	if !ok {
		// non-status based errors as retryable
		return r.bo.Pause(), true
	}
	switch s.Code() {
	case codes.Unavailable:
		return r.bo.Pause(), true
	default:
		return r.bo.Pause(), false
	}
}

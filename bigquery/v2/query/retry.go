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
	"errors"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/googleapis/gax-go/v2"
	"google.golang.org/api/googleapi"
)

var (
	defaultRetryReasons = []string{"backendError", "rateLimitExceeded"}

	// These reasons are used exclusively for enqueuing jobs (jobs.insert and jobs.query).
	// Using them for polling may cause unwanted retries until context deadline/cancellation/etc.
	jobRetryReasons = []string{"backendError", "rateLimitExceeded", "jobRateLimitExceeded", "internalError"}

	retry5xxCodes = []int{
		http.StatusInternalServerError,
		http.StatusBadGateway,
		http.StatusServiceUnavailable,
		http.StatusGatewayTimeout,
	}
)

type queryRetryer struct {
	bo      gax.Backoff
	reasons []string
}

func defaultRetryerFunc() gax.Retryer {
	return retryerWithReasons(defaultRetryReasons)
}

func defaultJobRetryerFunc() gax.Retryer {
	return retryerWithReasons(jobRetryReasons)
}

func retryerWithReasons(reasons []string) gax.Retryer {
	return &queryRetryer{
		// These parameters match the suggestions in https://cloud.google.com/bigquery/sla.
		bo: gax.Backoff{
			Initial:    1 * time.Second,
			Max:        32 * time.Second,
			Multiplier: 2,
		},
		reasons: reasons,
	}
}

var _ gax.Retryer = &queryRetryer{}

func (r *queryRetryer) Retry(err error) (pause time.Duration, shouldRetry bool) {
	return r.bo.Pause(), retryableError(err, r.reasons)
}

// retryableError is the unary retry predicate for this library.  In addition to structured error
// reasons, it specifies some HTTP codes (500, 502, 503, 504) and network/transport reasons.
func retryableError(err error, allowedReasons []string) bool {
	if err == nil {
		return false
	}
	if err == io.ErrUnexpectedEOF {
		return true
	}
	// Special case due to http2: https://github.com/googleapis/google-cloud-go/issues/1793
	// Due to Go's default being higher for streams-per-connection than is accepted by the
	// BQ backend, it's possible to get streams refused immediately after a connection is
	// started but before we receive SETTINGS frame from the backend.  This generally only
	// happens when we try to enqueue > 100 requests onto a newly initiated connection.
	if err.Error() == "http2: stream closed" {
		return true
	}

	switch e := err.(type) {
	case *googleapi.Error:
		// We received a structured error from backend.
		var reason string
		if len(e.Errors) > 0 {
			reason = e.Errors[0].Reason
			for _, r := range allowedReasons {
				if reason == r {
					return true
				}
			}
		}
		for _, code := range retry5xxCodes {
			if e.Code == code {
				return true
			}
		}
	case *url.Error:
		retryable := []string{"connection refused", "connection reset"}
		for _, s := range retryable {
			if strings.Contains(e.Error(), s) {
				return true
			}
		}
	case interface{ Temporary() bool }:
		if e.Temporary() {
			return true
		}
	}
	// Check wrapped error.
	return retryableError(errors.Unwrap(err), allowedReasons)
}

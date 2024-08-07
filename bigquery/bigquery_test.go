// Copyright 2021 Google LLC
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
	"errors"
	"io"
	"net/http"
	"net/url"
	"testing"

	"golang.org/x/xerrors"
	"google.golang.org/api/googleapi"
)

func TestRetryableErrors(t *testing.T) {
	testCases := []struct {
		description        string
		in                 error
		useJobRetryReasons bool
		wantRetry          bool
	}{
		{
			description: "nil error",
			in:          nil,
			wantRetry:   false,
		},
		{
			description: "http stream closed",
			in:          errors.New("http2: stream closed"),
			wantRetry:   true,
		},
		{
			description: "io ErrUnexpectedEOF",
			in:          io.ErrUnexpectedEOF,
			wantRetry:   true,
		},
		{
			description: "unavailable",
			in: &googleapi.Error{
				Code:    http.StatusServiceUnavailable,
				Message: "foo",
			},
			wantRetry: true,
		},
		{
			description: "url connection error",
			in:          &url.Error{Op: "blah", URL: "blah", Err: errors.New("connection refused")},
			wantRetry:   true,
		},
		{
			description: "url other error",
			in:          &url.Error{Op: "blah", URL: "blah", Err: errors.New("blah")},
			wantRetry:   false,
		},
		{
			description: "wrapped retryable",
			in: xerrors.Errorf("test of wrapped retryable: %w", &googleapi.Error{
				Code:    http.StatusServiceUnavailable,
				Message: "foo",
				Errors: []googleapi.ErrorItem{
					{Reason: "backendError", Message: "foo"},
				},
			}),
			wantRetry: true,
		},
		{
			description: "wrapped non-retryable",
			in:          xerrors.Errorf("test of wrapped retryable: %w", errors.New("blah")),
			wantRetry:   false,
		},
		{
			// not retried per https://google.aip.dev/194
			description: "internal error",
			in: &googleapi.Error{
				Code: http.StatusInternalServerError,
			},
			wantRetry: true,
		},
		{
			description: "internal w/backend reason",
			in: &googleapi.Error{
				Code:    http.StatusServiceUnavailable,
				Message: "foo",
				Errors: []googleapi.ErrorItem{
					{Reason: "backendError", Message: "foo"},
				},
			},
			wantRetry: true,
		},
		{
			description: "internal w/rateLimitExceeded reason",
			in: &googleapi.Error{
				Code:    http.StatusServiceUnavailable,
				Message: "foo",
				Errors: []googleapi.ErrorItem{
					{Reason: "rateLimitExceeded", Message: "foo"},
				},
			},
			wantRetry: true,
		},
		{
			description: "bad gateway error",
			in: &googleapi.Error{
				Code:    http.StatusBadGateway,
				Message: "foo",
			},
			wantRetry: true,
		},
		{
			description: "jobRateLimitExceeded default",
			in: &googleapi.Error{
				Code:    http.StatusOK, // ensure we're testing the reason
				Message: "foo",
				Errors: []googleapi.ErrorItem{
					{Reason: "jobRateLimitExceeded", Message: "foo"},
				},
			},
			wantRetry: false,
		},
		{
			description: "jobRateLimitExceeded job",
			in: &googleapi.Error{
				Code:    http.StatusOK, // ensure we're testing the reason
				Message: "foo",
				Errors: []googleapi.ErrorItem{
					{Reason: "jobRateLimitExceeded", Message: "foo"},
				},
			},
			useJobRetryReasons: true,
			wantRetry:          true,
		},
		{
			description: "structured internal error default",
			in: &googleapi.Error{
				Code:    http.StatusOK, // ensure we're testing the reason
				Message: "foo",
				Errors: []googleapi.ErrorItem{
					{Reason: "internalError", Message: "foo"},
				},
			},
			wantRetry: false,
		},
		{
			description: "structured internal error default",
			in: &googleapi.Error{
				Code:    http.StatusOK, // ensure we're testing the reason
				Message: "foo",
				Errors: []googleapi.ErrorItem{
					{Reason: "internalError", Message: "foo"},
				},
			},
			useJobRetryReasons: true,
			wantRetry:          true,
		},
	}

	for _, testcase := range testCases {
		tc := testcase
		t.Run(tc.description, func(t *testing.T) {
			t.Parallel()
			reasons := defaultRetryReasons
			if tc.useJobRetryReasons {
				reasons = jobRetryReasons
			}
			got := retryableError(tc.in, reasons)
			if got != tc.wantRetry {
				t.Errorf("case (%s) mismatch:  got %t wantRetry %t", tc.description, got, tc.wantRetry)
			}
		})
	}
}

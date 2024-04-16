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
	for _, tc := range []struct {
		description string
		in          error
		want        bool
	}{
		{
			"nil error",
			nil,
			false,
		},
		{
			"http stream closed",
			errors.New("http2: stream closed"),
			true,
		},
		{
			"io ErrUnexpectedEOF",
			io.ErrUnexpectedEOF,
			true,
		},
		{
			"unavailable",
			&googleapi.Error{
				Code:    http.StatusServiceUnavailable,
				Message: "foo",
			},
			true,
		},
		{
			"url connection error",
			&url.Error{Op: "blah", URL: "blah", Err: errors.New("connection refused")},
			true,
		},
		{
			"url other error",
			&url.Error{Op: "blah", URL: "blah", Err: errors.New("blah")},
			false,
		},
		{
			"wrapped retryable",
			xerrors.Errorf("test of wrapped retryable: %w", &googleapi.Error{
				Code:    http.StatusServiceUnavailable,
				Message: "foo",
				Errors: []googleapi.ErrorItem{
					{Reason: "backendError", Message: "foo"},
				},
			}),
			true,
		},
		{
			"wrapped non-retryable",
			xerrors.Errorf("test of wrapped retryable: %w", errors.New("blah")),
			false,
		},
		{
			// not retried per https://google.aip.dev/194
			"internal error",
			&googleapi.Error{
				Code: http.StatusInternalServerError,
			},
			true,
		},
		{
			"internal w/backend reason",
			&googleapi.Error{
				Code:    http.StatusServiceUnavailable,
				Message: "foo",
				Errors: []googleapi.ErrorItem{
					{Reason: "backendError", Message: "foo"},
				},
			},
			true,
		},
		{
			"internal w/rateLimitExceeded reason",
			&googleapi.Error{
				Code:    http.StatusServiceUnavailable,
				Message: "foo",
				Errors: []googleapi.ErrorItem{
					{Reason: "rateLimitExceeded", Message: "foo"},
				},
			},
			true,
		},
		{
			"bad gateway error",
			&googleapi.Error{
				Code:    http.StatusBadGateway,
				Message: "foo",
			},
			true,
		},
	} {
		got := retryableError(tc.in, defaultRetryReasons)
		if got != tc.want {
			t.Errorf("case (%s) mismatch:  got %t want %t", tc.description, got, tc.want)
		}
	}
}

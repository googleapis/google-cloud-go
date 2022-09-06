// Copyright 2020 Google LLC
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

package storage

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"testing"

	"golang.org/x/xerrors"

	"google.golang.org/api/googleapi"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func TestInvoke(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	// Time-based tests are flaky. We just make sure that invoke eventually
	// returns with the right error.

	for _, test := range []struct {
		desc              string
		count             int   // Number of times to return retryable error.
		initialErr        error // Error to return initially.
		finalErr          error // Error to return after count returns of retryCode.
		retry             *retryConfig
		isIdempotentValue bool
		expectFinalErr    bool
	}{
		{
			desc:              "test fn never returns initial error with count=0",
			count:             0,
			initialErr:        &googleapi.Error{Code: 0}, //non-retryable
			finalErr:          nil,
			isIdempotentValue: true,
			expectFinalErr:    true,
		},
		{
			desc:              "non-retryable error is returned without retrying",
			count:             1,
			initialErr:        &googleapi.Error{Code: 0},
			finalErr:          nil,
			isIdempotentValue: true,
			expectFinalErr:    false,
		},
		{
			desc:              "retryable error is retried",
			count:             1,
			initialErr:        &googleapi.Error{Code: 429},
			finalErr:          nil,
			isIdempotentValue: true,
			expectFinalErr:    true,
		},
		{
			desc:              "returns non-retryable error after retryable error",
			count:             1,
			initialErr:        &googleapi.Error{Code: 429},
			finalErr:          errors.New("bar"),
			isIdempotentValue: true,
			expectFinalErr:    true,
		},
		{
			desc:              "retryable 5xx error is retried",
			count:             2,
			initialErr:        &googleapi.Error{Code: 518},
			finalErr:          nil,
			isIdempotentValue: true,
			expectFinalErr:    true,
		},
		{
			desc:              "retriable error not retried when non-idempotent",
			count:             2,
			initialErr:        &googleapi.Error{Code: 599},
			finalErr:          nil,
			isIdempotentValue: false,
			expectFinalErr:    false,
		},
		{

			desc:              "non-idempotent retriable error retried when policy is RetryAlways",
			count:             2,
			initialErr:        &googleapi.Error{Code: 500},
			finalErr:          nil,
			isIdempotentValue: false,
			retry:             &retryConfig{policy: RetryAlways},
			expectFinalErr:    true,
		},
		{
			desc:              "retriable error not retried when policy is RetryNever",
			count:             2,
			initialErr:        &url.Error{Op: "blah", URL: "blah", Err: errors.New("connection refused")},
			finalErr:          nil,
			isIdempotentValue: true,
			retry:             &retryConfig{policy: RetryNever},
			expectFinalErr:    false,
		},
		{
			desc:              "non-retriable error not retried when policy is RetryAlways",
			count:             2,
			initialErr:        xerrors.Errorf("non-retriable error: %w", &googleapi.Error{Code: 400}),
			finalErr:          nil,
			isIdempotentValue: true,
			retry:             &retryConfig{policy: RetryAlways},
			expectFinalErr:    false,
		},

		{
			desc:              "non-retriable error retried with custom fn",
			count:             2,
			initialErr:        io.ErrNoProgress,
			finalErr:          nil,
			isIdempotentValue: true,
			retry: &retryConfig{
				shouldRetry: func(err error) bool {
					return err == io.ErrNoProgress
				},
			},
			expectFinalErr: true,
		},
		{
			desc:              "retriable error not retried with custom fn",
			count:             2,
			initialErr:        io.ErrUnexpectedEOF,
			finalErr:          nil,
			isIdempotentValue: true,
			retry: &retryConfig{
				shouldRetry: func(err error) bool {
					return err == io.ErrNoProgress
				},
			},
			expectFinalErr: false,
		},
		{
			desc:              "error not retried when policy is RetryNever despite custom fn",
			count:             2,
			initialErr:        io.ErrUnexpectedEOF,
			finalErr:          nil,
			isIdempotentValue: true,
			retry: &retryConfig{
				shouldRetry: func(err error) bool {
					return err == io.ErrUnexpectedEOF
				},
				policy: RetryNever,
			},
			expectFinalErr: false,
		},
	} {
		t.Run(test.desc, func(s *testing.T) {
			counter := 0
			req := &fakeApiaryRequest{header: http.Header{}}
			var initialHeader string
			call := func() error {
				if counter == 0 {
					initialHeader = req.Header()["X-Goog-Api-Client"][0]
				}
				counter++
				if counter <= test.count {
					return test.initialErr
				}
				return test.finalErr
			}
			got := run(ctx, call, test.retry, test.isIdempotentValue, setRetryHeaderHTTP(req))
			if test.expectFinalErr && got != test.finalErr {
				s.Errorf("got %v, want %v", got, test.finalErr)
			} else if !test.expectFinalErr && got != test.initialErr {
				s.Errorf("got %v, want %v", got, test.initialErr)
			}
			gotHeader := req.Header()["X-Goog-Api-Client"][0]
			wantAttempts := 1 + test.count
			if !test.expectFinalErr {
				wantAttempts = 1
			}
			wantHeader := strings.ReplaceAll(initialHeader, "gccl-attempt-count/1", fmt.Sprintf("gccl-attempt-count/%v", wantAttempts))
			if gotHeader != wantHeader {
				t.Errorf("case %q, retry header:\ngot %v\nwant %v", test.desc, gotHeader, wantHeader)
			}
			wantHeaderFormat := "gccl-invocation-id/.{36} gccl-attempt-count/[0-9]+ gl-go/.* gccl/"
			match, err := regexp.MatchString(wantHeaderFormat, gotHeader)
			if err != nil {
				s.Fatalf("compiling regexp: %v", err)
			}
			if !match {
				s.Errorf("X-Goog-Api-Client header has wrong format\ngot %v\nwant regex matching %v", gotHeader, wantHeaderFormat)
			}
		})
	}
}

type fakeApiaryRequest struct {
	header http.Header
}

func (f *fakeApiaryRequest) Header() http.Header {
	return f.header
}

func TestShouldRetry(t *testing.T) {
	t.Parallel()

	for _, test := range []struct {
		desc        string
		inputErr    error
		shouldRetry bool
	}{
		{
			desc:        "googleapi.Error{Code: 0}",
			inputErr:    &googleapi.Error{Code: 0},
			shouldRetry: false,
		},
		{
			desc:        "googleapi.Error{Code: 429}",
			inputErr:    &googleapi.Error{Code: 429},
			shouldRetry: true,
		},
		{
			desc:        "errors.New(foo)",
			inputErr:    errors.New("foo"),
			shouldRetry: false,
		},
		{
			desc:        "googleapi.Error{Code: 518}",
			inputErr:    &googleapi.Error{Code: 518},
			shouldRetry: true,
		},
		{
			desc:        "googleapi.Error{Code: 599}",
			inputErr:    &googleapi.Error{Code: 599},
			shouldRetry: true,
		},
		{
			desc:        "googleapi.Error{Code: 428}",
			inputErr:    &googleapi.Error{Code: 428},
			shouldRetry: false,
		},
		{
			desc:        "googleapi.Error{Code: 518}",
			inputErr:    &googleapi.Error{Code: 518},
			shouldRetry: true,
		},
		{
			desc:        "url.Error{Err: errors.New(\"connection refused\")}",
			inputErr:    &url.Error{Op: "blah", URL: "blah", Err: errors.New("connection refused")},
			shouldRetry: true,
		},
		{
			desc:        "io.ErrUnexpectedEOF",
			inputErr:    io.ErrUnexpectedEOF,
			shouldRetry: true,
		},
		{
			desc:        "wrapped retryable error",
			inputErr:    xerrors.Errorf("Test unwrapping of a temporary error: %w", &googleapi.Error{Code: 500}),
			shouldRetry: true,
		},
		{
			desc:        "wrapped non-retryable error",
			inputErr:    xerrors.Errorf("Test unwrapping of a non-retriable error: %w", &googleapi.Error{Code: 400}),
			shouldRetry: false,
		},
		{
			desc:        "googleapi.Error{Code: 400}",
			inputErr:    &googleapi.Error{Code: 400},
			shouldRetry: false,
		},
		{
			desc:        "googleapi.Error{Code: 408}",
			inputErr:    &googleapi.Error{Code: 408},
			shouldRetry: true,
		},
		{
			desc:        "retryable gRPC error",
			inputErr:    status.Error(codes.Unavailable, "retryable gRPC error"),
			shouldRetry: true,
		},
		{
			desc:        "non-retryable gRPC error",
			inputErr:    status.Error(codes.PermissionDenied, "non-retryable gRPC error"),
			shouldRetry: false,
		},
		{
			desc: "wrapped ErrClosed text",
			// TODO: check directly against wrapped net.ErrClosed (go 1.16+)
			inputErr:    &net.OpError{Op: "write", Err: errors.New("use of closed network connection")},
			shouldRetry: true,
		},
	} {
		t.Run(test.desc, func(s *testing.T) {
			got := ShouldRetry(test.inputErr)

			if got != test.shouldRetry {
				s.Errorf("got %v, want %v", got, test.shouldRetry)
			}
		})
	}
}

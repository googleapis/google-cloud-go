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
	"time"

	"github.com/googleapis/gax-go/v2"
	"github.com/googleapis/gax-go/v2/callctx"
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
		wantAttempts      *int
	}{
		{
			desc:              "test fn never returns initial error with count=0",
			count:             0,
			initialErr:        &googleapi.Error{Code: 0}, //non-retryable
			finalErr:          nil,
			isIdempotentValue: true,
			expectFinalErr:    true,
			wantAttempts:      intPointer(1),
		},
		{
			desc:              "non-retryable error is returned without retrying",
			count:             1,
			initialErr:        &googleapi.Error{Code: 0},
			finalErr:          nil,
			isIdempotentValue: true,
			expectFinalErr:    false,
			wantAttempts:      intPointer(1),
		},
		{
			desc:              "retryable error is retried",
			count:             1,
			initialErr:        &googleapi.Error{Code: 429},
			finalErr:          nil,
			isIdempotentValue: true,
			expectFinalErr:    true,
			wantAttempts:      intPointer(2),
		},
		{
			desc:              "retryable gRPC error is retried",
			count:             1,
			initialErr:        status.Error(codes.ResourceExhausted, "rate limit"),
			finalErr:          nil,
			isIdempotentValue: true,
			expectFinalErr:    true,
			wantAttempts:      intPointer(2),
		},
		{
			desc:              "returns non-retryable error after retryable error",
			count:             1,
			initialErr:        &googleapi.Error{Code: 429},
			finalErr:          errors.New("bar"),
			isIdempotentValue: true,
			expectFinalErr:    true,
			wantAttempts:      intPointer(2),
		},
		{
			desc:              "retryable 5xx error is retried",
			count:             2,
			initialErr:        &googleapi.Error{Code: 518},
			finalErr:          nil,
			isIdempotentValue: true,
			expectFinalErr:    true,
			wantAttempts:      intPointer(3),
		},
		{
			desc:              "retriable error not retried when non-idempotent",
			count:             2,
			initialErr:        &googleapi.Error{Code: 599},
			finalErr:          nil,
			isIdempotentValue: false,
			expectFinalErr:    false,
			wantAttempts:      intPointer(1),
		},
		{
			desc:              "non-idempotent retriable error retried when policy is RetryAlways",
			count:             2,
			initialErr:        &googleapi.Error{Code: 500},
			finalErr:          nil,
			isIdempotentValue: false,
			retry:             &retryConfig{policy: RetryAlways},
			expectFinalErr:    true,
			wantAttempts:      intPointer(3),
		},
		{
			desc:              "retriable error not retried when policy is RetryNever",
			count:             2,
			initialErr:        &url.Error{Op: "blah", URL: "blah", Err: errors.New("connection refused")},
			finalErr:          nil,
			isIdempotentValue: true,
			retry:             &retryConfig{policy: RetryNever},
			expectFinalErr:    false,
			wantAttempts:      intPointer(1),
		},
		{
			desc:              "non-retriable error not retried when policy is RetryAlways",
			count:             2,
			initialErr:        fmt.Errorf("non-retriable error: %w", &googleapi.Error{Code: 400}),
			finalErr:          nil,
			isIdempotentValue: true,
			retry:             &retryConfig{policy: RetryAlways},
			expectFinalErr:    false,
			wantAttempts:      intPointer(1),
		},
		{
			desc:              "non-retriable error retried with custom fn",
			count:             2,
			initialErr:        io.ErrNoProgress,
			finalErr:          nil,
			isIdempotentValue: true,
			retry: &retryConfig{
				shouldRetry: func(err error, retryCtx *RetryContext) bool {
					return err == io.ErrNoProgress
				},
			},
			expectFinalErr: true,
			wantAttempts:   intPointer(3),
		},
		{
			desc:              "retriable error not retried with custom fn",
			count:             2,
			initialErr:        io.ErrUnexpectedEOF,
			finalErr:          nil,
			isIdempotentValue: true,
			retry: &retryConfig{
				shouldRetry: func(err error, retryCtx *RetryContext) bool {
					return err == io.ErrNoProgress
				},
			},
			expectFinalErr: false,
			wantAttempts:   intPointer(1),
		},
		{
			desc:              "error not retried when policy is RetryNever despite custom fn",
			count:             2,
			initialErr:        io.ErrUnexpectedEOF,
			finalErr:          nil,
			isIdempotentValue: true,
			retry: &retryConfig{
				shouldRetry: func(err error, retryCtx *RetryContext) bool {
					return err == io.ErrUnexpectedEOF
				},
				policy: RetryNever,
			},
			expectFinalErr: false,
			wantAttempts:   intPointer(1),
		},
		{
			desc:              "non-idempotent retriable error retried when policy is RetryAlways till maxAttempts",
			count:             4,
			initialErr:        &googleapi.Error{Code: 500},
			finalErr:          nil,
			isIdempotentValue: false,
			retry:             &retryConfig{policy: RetryAlways, maxAttempts: intPointer(2)},
			expectFinalErr:    false,
			wantAttempts:      intPointer(2),
		},
		{
			desc:              "non-idempotent retriable error not retried when policy is RetryNever with maxAttempts set",
			count:             4,
			initialErr:        &googleapi.Error{Code: 500},
			finalErr:          nil,
			isIdempotentValue: false,
			retry:             &retryConfig{policy: RetryNever, maxAttempts: intPointer(2)},
			expectFinalErr:    false,
			wantAttempts:      intPointer(1),
		},
		{
			desc:              "non-retriable error retried with custom fn till maxAttempts",
			count:             4,
			initialErr:        io.ErrNoProgress,
			finalErr:          nil,
			isIdempotentValue: true,
			retry: &retryConfig{
				shouldRetry: func(err error, retryCtx *RetryContext) bool {
					return err == io.ErrNoProgress
				},
				maxAttempts: intPointer(2),
			},
			expectFinalErr: false,
			wantAttempts:   intPointer(2),
		},
		{
			desc:              "non-idempotent retriable error retried when policy is RetryAlways till maxAttempts where count equals to maxAttempts-1",
			count:             3,
			initialErr:        &googleapi.Error{Code: 500},
			finalErr:          nil,
			isIdempotentValue: false,
			retry:             &retryConfig{policy: RetryAlways, maxAttempts: intPointer(4)},
			expectFinalErr:    true,
			wantAttempts:      intPointer(4),
		},
		{
			desc:              "non-idempotent retriable error retried when policy is RetryAlways till maxAttempts where count equals to maxAttempts",
			count:             4,
			initialErr:        &googleapi.Error{Code: 500},
			finalErr:          nil,
			isIdempotentValue: true,
			retry:             &retryConfig{policy: RetryAlways, maxAttempts: intPointer(4)},
			expectFinalErr:    false,
			wantAttempts:      intPointer(4),
		},
		{
			desc:              "non-idempotent retriable error not retried when policy is RetryAlways with maxAttempts equals to zero",
			count:             4,
			initialErr:        &googleapi.Error{Code: 500},
			finalErr:          nil,
			isIdempotentValue: true,
			retry:             &retryConfig{maxAttempts: intPointer(0), policy: RetryAlways},
			expectFinalErr:    false,
			wantAttempts:      intPointer(1),
		},
		{
			desc:              "retry deadline stops retries",
			count:             20,
			initialErr:        &googleapi.Error{Code: 500},
			finalErr:          nil,
			isIdempotentValue: true,
			retry:             &retryConfig{policy: RetryAlways, maxRetryDuration: time.Second / 2},
			expectFinalErr:    false,
		},
		{
			desc:              "retry deadline not reached",
			count:             4,
			initialErr:        &googleapi.Error{Code: 500},
			finalErr:          nil,
			isIdempotentValue: true,
			retry:             &retryConfig{policy: RetryAlways, maxRetryDuration: time.Second / 2},
			expectFinalErr:    true,
			wantAttempts:      intPointer(5),
		},
		{
			desc:              "maxAttempts reached before maxRetryDuration",
			count:             10,
			initialErr:        &googleapi.Error{Code: 500},
			finalErr:          nil,
			isIdempotentValue: true,
			retry:             &retryConfig{policy: RetryAlways, maxAttempts: intPointer(3), maxRetryDuration: time.Second * 10},
			expectFinalErr:    false,
			wantAttempts:      intPointer(3),
		},
		{
			desc:              "maxRetryDuration reached before maxAttempts",
			count:             1000,
			initialErr:        &googleapi.Error{Code: 500},
			finalErr:          nil,
			isIdempotentValue: true,
			retry:             &retryConfig{policy: RetryAlways, maxAttempts: intPointer(1005), maxRetryDuration: time.Millisecond * 10},
			expectFinalErr:    false,
		},
		{
			desc:              "maxRetryDuration set to 0 allows infinite retries",
			count:             5,
			initialErr:        &googleapi.Error{Code: 500},
			finalErr:          nil,
			isIdempotentValue: true,
			retry:             &retryConfig{policy: RetryAlways, maxRetryDuration: 0},
			expectFinalErr:    true,
			wantAttempts:      intPointer(6),
		},
	} {
		t.Run(test.desc, func(s *testing.T) {
			counter := 0
			var initialClientHeader, initialIdempotencyHeader string
			var gotClientHeader, gotIdempotencyHeader string
			call := func(ctx context.Context) error {
				if counter == 0 {
					headers := callctx.HeadersFromContext(ctx)
					initialClientHeader = headers["x-goog-api-client"][0]
					initialIdempotencyHeader = headers["x-goog-gcs-idempotency-token"][0]
				}
				counter++
				headers := callctx.HeadersFromContext(ctx)
				gotClientHeader = headers["x-goog-api-client"][0]
				gotIdempotencyHeader = headers["x-goog-gcs-idempotency-token"][0]
				if counter <= test.count {
					return test.initialErr
				}
				return test.finalErr
			}
			// Use a short backoff to speed up the test.
			if test.retry == nil {
				test.retry = defaultRetry.clone()
			}
			test.retry.backoff = &gax.Backoff{Initial: time.Millisecond}
			got := run(ctx, call, test.retry, test.isIdempotentValue)
			if test.expectFinalErr && !errors.Is(got, test.finalErr) {
				s.Errorf("got %v, want %v", got, test.finalErr)
			} else if !test.expectFinalErr && !errors.Is(got, test.initialErr) {
				s.Errorf("got %v, want %v", got, test.initialErr)
			}

			if test.wantAttempts != nil {
				wantClientHeader := strings.ReplaceAll(initialClientHeader, "gccl-attempt-count/1", fmt.Sprintf("gccl-attempt-count/%v", *test.wantAttempts))
				if gotClientHeader != wantClientHeader {
					t.Errorf("case %q, retry header:\ngot %v\nwant %v", test.desc, gotClientHeader, wantClientHeader)
				}
			}

			wantClientHeaderFormat := "gccl-invocation-id/.{36} gccl-attempt-count/[0-9]+ gl-go/.* gccl/"
			match, err := regexp.MatchString(wantClientHeaderFormat, gotClientHeader)
			if err != nil {
				s.Fatalf("compiling regexp: %v", err)
			}
			if !match {
				s.Errorf("X-Goog-Api-Client header has wrong format\ngot %v\nwant regex matching %v", gotClientHeader, wantClientHeaderFormat)
			}
			if gotIdempotencyHeader != initialIdempotencyHeader {
				t.Errorf("case %q, idempotency header:\ngot %v\nwant %v", test.desc, gotIdempotencyHeader, initialIdempotencyHeader)
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
			desc:        "net.OpError{Err: errors.New(\"connection reset by peer\")}",
			inputErr:    &net.OpError{Op: "blah", Net: "tcp", Err: errors.New("connection reset by peer")},
			shouldRetry: true,
		},
		{
			desc:        "io.ErrUnexpectedEOF",
			inputErr:    io.ErrUnexpectedEOF,
			shouldRetry: true,
		},
		{
			desc:        "wrapped retryable error",
			inputErr:    fmt.Errorf("Test unwrapping of a temporary error: %w", &googleapi.Error{Code: 500}),
			shouldRetry: true,
		},
		{
			desc:        "wrapped non-retryable error",
			inputErr:    fmt.Errorf("Test unwrapping of a non-retriable error: %w", &googleapi.Error{Code: 400}),
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
			desc:        "wrapped net.ErrClosed",
			inputErr:    &net.OpError{Err: net.ErrClosed},
			shouldRetry: true,
		},
		{
			desc:        "nil error",
			inputErr:    nil,
			shouldRetry: false,
		},
		{
			desc:        "http2: client connection lost",
			inputErr:    &url.Error{Op: "blah", URL: "blah", Err: errors.New("http2: client connection lost")},
			shouldRetry: true,
		},
		{
			desc:        "wrapped http2: client connection lost",
			inputErr:    fmt.Errorf("wrapped error: %w", &url.Error{Op: "blah", URL: "blah", Err: errors.New("http2: client connection lost")}),
			shouldRetry: true,
		},
		{
			desc:        "server closed idle connection",
			inputErr:    &url.Error{Op: "blah", URL: "blah", Err: errors.New("http: server closed idle connection")},
			shouldRetry: true,
		},
		{
			desc:        "wrapped server closed idle connection",
			inputErr:    fmt.Errorf("wrapped error: %w", &url.Error{Op: "blah", URL: "blah", Err: errors.New("http: server closed idle connection")}),
			shouldRetry: true,
		},
		{
			desc:        "net.OpError with server closed idle connection",
			inputErr:    &net.OpError{Op: "read", Net: "tcp", Err: errors.New("server closed idle connection")},
			shouldRetry: true,
		},
		{
			desc:        "wrapped net.OpError with server closed idle connection",
			inputErr:    fmt.Errorf("wrapped error: %w", &net.OpError{Op: "read", Net: "tcp", Err: errors.New("server closed idle connection")}),
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

func TestInvokeWithCookie(t *testing.T) {
	expectedCookie := "C=a_test_cookie"
	oldCookieHeader := cookieHeader
	cookieHeader = func() string { return expectedCookie }
	defer func() {
		cookieHeader = oldCookieHeader
	}()

	ctx := context.Background()
	var gotCookie, gotDirectpathCookie string
	if err := run(ctx, func(ctx context.Context) error {
		headers := callctx.HeadersFromContext(ctx)
		gotCookie = headers["cookie"][0]
		gotDirectpathCookie = headers["x-directpath-tracing-cookie"][0]
		return nil
	}, nil, false); err != nil {
		t.Errorf("error during run; got %v, want nil", err)
	}

	if gotCookie != expectedCookie {
		t.Errorf("incorrect value for cookie header; got %v, want %v", gotCookie, expectedCookie)
	}

	if gotDirectpathCookie != expectedCookie {
		t.Errorf("incorrect value for x-directpath-tracing-cookie header; got %v, want %v", gotDirectpathCookie, expectedCookie)
	}
}

func TestInvokeWithRetryContext(t *testing.T) {
	t.Parallel()
	ctx := context.Background()

	tests := []struct {
		desc          string
		operation     string
		bucket        string
		object        string
		retryAttempts int
		errorToReturn error
	}{
		{
			desc:          "RetryContext populated for object operations",
			operation:     "GetObject",
			bucket:        "test-bucket",
			object:        "test-object.txt",
			retryAttempts: 3,
			errorToReturn: &googleapi.Error{Code: 429},
		},
		{
			desc:          "RetryContext populated with empty object",
			operation:     "ListObjects",
			bucket:        "test-bucket",
			object:        "",
			retryAttempts: 2,
			errorToReturn: status.Error(codes.ResourceExhausted, "rate limit"),
		},
		{
			desc:          "RetryContext populated for write operations",
			operation:     "WriteObject",
			bucket:        "my-bucket",
			object:        "path/to/file.dat",
			retryAttempts: 4,
			errorToReturn: &googleapi.Error{Code: 503},
		},
	}

	for _, test := range tests {
		t.Run(test.desc, func(s *testing.T) {
			var capturedContexts []*RetryContext
			var lastInvocationID string
			callCounter := 0

			// Custom shouldRetry function that captures RetryContext.
			customRetry := &retryConfig{
				policy: RetryAlways,
				shouldRetry: func(err error, retryCtx *RetryContext) bool {
					// Don't retry on success
					if err == nil {
						return false
					}

					// Capture the context for verification.
					capturedContexts = append(capturedContexts, &RetryContext{
						Attempt:      retryCtx.Attempt,
						InvocationID: retryCtx.InvocationID,
						Operation:    retryCtx.Operation,
						Bucket:       retryCtx.Bucket,
						Object:       retryCtx.Object,
					})

					// Remember the invocation ID from the first call.
					if len(capturedContexts) == 1 {
						lastInvocationID = retryCtx.InvocationID
					}

					// Retry if we haven't reached the retry limit.
					// retryCtx.Attempt is the current attempt that just failed.
					return retryCtx.Attempt <= test.retryAttempts
				},
				backoff: &gax.Backoff{Initial: time.Millisecond},
			}

			call := func(ctx context.Context) error {
				callCounter++
				// Fail for the first retryAttempts calls, succeed on call retryAttempts+1.
				if callCounter <= test.retryAttempts {
					return test.errorToReturn
				}
				return nil
			}

			// Run with the operation metadata.
			err := run(ctx, call, customRetry, true,
				withOperation(test.operation),
				withBucket(test.bucket),
				withObject(test.object))

			if err != nil {
				s.Fatalf("expected nil error after retries, got: %v", err)
			}

			// Verify we got the expected number of retry contexts.
			// shouldRetry is called once per failed attempt.
			expectedCalls := test.retryAttempts
			if len(capturedContexts) != expectedCalls {
				s.Errorf("expected %d retry contexts, got %d", expectedCalls, len(capturedContexts))
			}

			// Verify each captured context
			for i, retryCtx := range capturedContexts {
				expectedAttempt := i + 1

				// Check attempt number.
				if retryCtx.Attempt != expectedAttempt {
					s.Errorf("attempt %d: expected Attempt=%d, got %d", i, expectedAttempt, retryCtx.Attempt)
				}

				// Check invocation ID is consistent.
				if retryCtx.InvocationID == "" {
					s.Errorf("attempt %d: InvocationID should not be empty", i)
				}
				if retryCtx.InvocationID != lastInvocationID {
					s.Errorf("attempt %d: InvocationID changed, expected %s, got %s", i, lastInvocationID, retryCtx.InvocationID)
				}

				// Check operation name.
				if retryCtx.Operation != test.operation {
					s.Errorf("attempt %d: expected Operation=%q, got %q", i, test.operation, retryCtx.Operation)
				}

				// Check bucket name.
				if retryCtx.Bucket != test.bucket {
					s.Errorf("attempt %d: expected Bucket=%q, got %q", i, test.bucket, retryCtx.Bucket)
				}

				// Check object name.
				if retryCtx.Object != test.object {
					s.Errorf("attempt %d: expected Object=%q, got %q", i, test.object, retryCtx.Object)
				}
			}
		})
	}
}

// Test that RetryContext fields are empty when run is called without
// any runOption. This verifies that the default values for
// these fields are empty or zero.
func TestInvokeWithRetryContextWithoutRunOptions(t *testing.T) {
	t.Parallel()
	ctx := context.Background()

	var capturedContext *RetryContext

	customRetry := &retryConfig{
		shouldRetry: func(err error, retryCtx *RetryContext) bool {
			capturedContext = &RetryContext{
				Attempt:      retryCtx.Attempt,
				InvocationID: retryCtx.InvocationID,
				Operation:    retryCtx.Operation,
				Bucket:       retryCtx.Bucket,
				Object:       retryCtx.Object,
			}
			return false // Don't retry.
		},
		backoff: &gax.Backoff{Initial: time.Millisecond},
	}

	call := func(ctx context.Context) error {
		return &googleapi.Error{Code: 429}
	}

	// Run without any metadata options.
	_ = run(ctx, call, customRetry, true)

	// Verify context was captured.
	if capturedContext == nil {
		t.Fatal("RetryContext was not passed to shouldRetry function")
	}

	// Verify basic fields are set.
	if capturedContext.Attempt != 1 {
		t.Errorf("expected Attempt=1, got %d", capturedContext.Attempt)
	}

	if capturedContext.InvocationID == "" {
		t.Error("InvocationID should not be empty")
	}

	// Verify metadata fields are empty when not provided.
	if capturedContext.Operation != "" {
		t.Errorf("expected empty Operation, got %q", capturedContext.Operation)
	}

	if capturedContext.Bucket != "" {
		t.Errorf("expected empty Bucket, got %q", capturedContext.Bucket)
	}

	if capturedContext.Object != "" {
		t.Errorf("expected empty Object, got %q", capturedContext.Object)
	}
}

func TestIsError(t *testing.T) {
	tests := []struct {
		name          string
		err           error
		httpErrorCode int
		grpcErrorCode codes.Code
		want          bool
	}{
		{
			name:          "matching HTTP error",
			err:           &googleapi.Error{Code: 404},
			httpErrorCode: 404,
			grpcErrorCode: codes.NotFound,
			want:          true,
		},
		{
			name:          "matching gRPC error",
			err:           status.Error(codes.NotFound, "not found"),
			httpErrorCode: 404,
			grpcErrorCode: codes.NotFound,
			want:          true,
		},
		{
			name:          "wrapped matching HTTP error",
			err:           fmt.Errorf("wrapped: %w", &googleapi.Error{Code: 404}),
			httpErrorCode: 404,
			grpcErrorCode: codes.NotFound,
			want:          true,
		},
		{
			name:          "wrapped matching gRPC error",
			err:           fmt.Errorf("wrapped: %w", status.Error(codes.NotFound, "not found")),
			httpErrorCode: 404,
			grpcErrorCode: codes.NotFound,
			want:          true,
		},
		{
			name:          "non-matching HTTP error",
			err:           &googleapi.Error{Code: 403},
			httpErrorCode: 404,
			grpcErrorCode: codes.NotFound,
			want:          false,
		},
		{
			name:          "non-matching gRPC error",
			err:           status.Error(codes.PermissionDenied, "permission denied"),
			httpErrorCode: 404,
			grpcErrorCode: codes.NotFound,
			want:          false,
		},
		{
			name:          "nil error",
			err:           nil,
			httpErrorCode: 404,
			grpcErrorCode: codes.NotFound,
			want:          false,
		},
		{
			name:          "unrelated error",
			err:           errors.New("some other error"),
			httpErrorCode: 404,
			grpcErrorCode: codes.NotFound,
			want:          false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := isError(tt.err, tt.httpErrorCode, tt.grpcErrorCode); got != tt.want {
				t.Errorf("isError() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestInvokeWithProgressReset(t *testing.T) {
	t.Parallel()
	ctx := context.Background()

	t.Run("resets maxAttempts and attempt header on progress", func(t *testing.T) {
		// maxAttempts is 2 per chunk.
		// Call 1: chunk 1 fails (attempt 1 of chunk 1, no progress).
		// Call 2: chunk 1 succeeds (progress = true) and chunk 2 fails (attempt 1 of chunk 2).
		// Call 3: chunk 2 succeeds.
		// Without progress reset, Call 2 would hit maxAttempts=2 and fail.
		retry := &retryConfig{
			policy:      RetryAlways,
			maxAttempts: intPointer(2),
			backoff:     &gax.Backoff{Initial: time.Millisecond},
		}

		callCount := 0
		progressMade := false
		var gotHeaders []string

		err := run(ctx, func(ctx context.Context) error {
			callCount++
			var header string
			if v := callctx.HeadersFromContext(ctx)["x-goog-api-client"]; len(v) > 0 {
				header = v[0]
			}
			gotHeaders = append(gotHeaders, header)
			switch callCount {
			case 1:
				return status.Error(codes.Unavailable, "chunk 1 transient failure")
			case 2:
				progressMade = true
				return status.Error(codes.Unavailable, "chunk 1 succeeded, chunk 2 transient failure")
			default:
				return nil
			}
		}, retry, true, withProgressReset(func() bool {
			if progressMade {
				progressMade = false
				return true
			}
			return false
		}))

		if err != nil {
			t.Fatalf("expected nil error when progress resets attempts, got: %v", err)
		}
		if callCount != 3 {
			t.Fatalf("expected 3 calls, got %d", callCount)
		}
		// Call 1: attempt 1; Call 2: attempt 2; Call 3 (after reset on Call 2): attempt 2 of chunk 2.
		wantCounts := []string{"gccl-attempt-count/1", "gccl-attempt-count/2", "gccl-attempt-count/2"}
		if len(gotHeaders) != len(wantCounts) {
			t.Fatalf("got %d x-goog-api-client headers, want %d", len(gotHeaders), len(wantCounts))
		}
		for i, want := range wantCounts {
			if !strings.Contains(gotHeaders[i], want) {
				t.Errorf("call %d header = %q, want substring %q", i+1, gotHeaders[i], want)
			}
		}
	})

	t.Run("enforces maxAttempts when no progress is made", func(t *testing.T) {
		retry := &retryConfig{
			policy:      RetryAlways,
			maxAttempts: intPointer(2),
			backoff:     &gax.Backoff{Initial: time.Millisecond},
		}

		callCount := 0
		progressMade := false

		err := run(ctx, func(ctx context.Context) error {
			callCount++
			if callCount == 2 {
				// Progress on call 2 resets attempts to 1, then call 3 fails without progress (attempts=2 -> hits maxAttempts=2).
				progressMade = true
			}
			return status.Error(codes.Unavailable, "transient failure")
		}, retry, true, withProgressReset(func() bool {
			if progressMade {
				progressMade = false
				return true
			}
			return false
		}))

		if err == nil || !strings.Contains(err.Error(), "retry failed after 2 attempts") {
			t.Fatalf("expected retry failed after 2 attempts error, got: %v", err)
		}
		if callCount != 3 {
			t.Fatalf("expected 3 total calls (2 for chunk 1, 1 retry for chunk 2 before hitting maxAttempts=2), got %d", callCount)
		}
	})

	t.Run("resets backoff on progress", func(t *testing.T) {
		// After call 1 the backoff's current pause is 1h. If progress on call
		// 2 resets it, the pause before call 3 is at most 1ms; otherwise it
		// is uniform in (0, 1h]. Assert on the measured pause; the context
		// deadline only keeps a regression from hanging the suite.
		const maxPause = time.Second
		retry := &retryConfig{
			policy:  RetryAlways,
			backoff: &gax.Backoff{Initial: time.Millisecond, Multiplier: 1e7, Max: time.Hour},
		}
		ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
		defer cancel()

		callCount := 0
		progressMade := false
		var call2Returned time.Time
		var pauseBeforeCall3 time.Duration
		err := run(ctx, func(context.Context) error {
			callCount++
			switch callCount {
			case 1:
				return status.Error(codes.Unavailable, "chunk 1 transient failure")
			case 2:
				progressMade = true
				call2Returned = time.Now()
				return status.Error(codes.Unavailable, "chunk 1 succeeded, chunk 2 transient failure")
			default:
				pauseBeforeCall3 = time.Since(call2Returned)
				return nil
			}
		}, retry, true, withProgressReset(func() bool {
			if progressMade {
				progressMade = false
				return true
			}
			return false
		}))

		if err != nil {
			t.Fatalf("expected nil error when progress resets backoff, got: %v", err)
		}
		if callCount != 3 {
			t.Fatalf("expected 3 calls, got %d", callCount)
		}
		if pauseBeforeCall3 > maxPause {
			t.Fatalf("pause between call 2 and call 3 was %v, want <= %v (backoff not reset on progress)", pauseBeforeCall3, maxPause)
		}
	})

	t.Run("returns when context is cancelled during backoff", func(t *testing.T) {
		retry := &retryConfig{
			policy:  RetryAlways,
			backoff: &gax.Backoff{Initial: time.Hour, Max: time.Hour},
		}
		ctx, cancel := context.WithCancel(ctx)
		defer cancel()

		callErr := status.Error(codes.Unavailable, "transient failure")
		callCount := 0
		start := time.Now()
		err := run(ctx, func(context.Context) error {
			callCount++
			// Cancel once run() is sleeping in the backoff.
			time.AfterFunc(10*time.Millisecond, cancel)
			return callErr
		}, retry, true, withProgressReset(func() bool { return false }))

		if elapsed := time.Since(start); elapsed > 5*time.Second {
			t.Errorf("run() returned after %v; want prompt return on cancellation", elapsed)
		}
		if callCount != 1 {
			t.Errorf("expected 1 call, got %d", callCount)
		}
		var wrapped wrappedCallErr
		if !errors.As(err, &wrapped) {
			t.Fatalf("expected wrappedCallErr, got %T: %v", err, err)
		}
		if !errors.Is(err, context.Canceled) {
			t.Errorf("errors.Is(err, context.Canceled) = false; err: %v", err)
		}
		if !errors.Is(err, callErr) {
			t.Errorf("errors.Is(err, callErr) = false; err: %v", err)
		}
	})
}

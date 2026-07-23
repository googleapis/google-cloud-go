// Copyright 2016 Google LLC
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

package internal

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	gax "github.com/googleapis/gax-go/v2"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func TestRetry(t *testing.T) {
	ctx := context.Background()
	// Without a context deadline, retry will run until the function
	// says not to retry any more.
	n := 0
	endRetry := errors.New("end retry")
	err := retryN(ctx, gax.Backoff{}, 0, // 0 means infinite retries
		func() (bool, error) {
			n++
			if n < 10 {
				return false, nil
			}
			return true, endRetry
		},
		func(context.Context, time.Duration) error { return nil })
	if got, want := err, endRetry; got != want {
		t.Errorf("got %v, want %v", err, endRetry)
	}
	if n != 10 {
		t.Errorf("n: got %d, want %d", n, 10)
	}

	// If the context has a deadline, sleep will return an error
	// and end the function.
	n = 0
	err = retryN(ctx, gax.Backoff{}, 0,
		func() (bool, error) { return false, nil },
		func(context.Context, time.Duration) error {
			n++
			if n < 10 {
				return nil
			}
			return context.DeadlineExceeded
		})
	if err == nil {
		t.Error("got nil, want error")
	}
}

func TestRetryPreserveError(t *testing.T) {
	// Retry tries to preserve the type and other information from
	// the last error returned by the function.
	err := retryN(context.Background(), gax.Backoff{}, 0,
		func() (bool, error) {
			return false, status.Error(codes.NotFound, "not found")
		},
		func(context.Context, time.Duration) error {
			return context.DeadlineExceeded
		})
	if err == nil {
		t.Fatalf("unexpectedly got nil error")
	}
	wantError := "retry failed with context deadline exceeded; last error: rpc error: code = NotFound desc = not found"
	if g, w := err.Error(), wantError; g != w {
		t.Errorf("got error %q, want %q", g, w)
	}
	got, ok := status.FromError(err)
	if !ok {
		t.Fatalf("got %T, wanted a status", got)
	}
	if g, w := got.Code(), codes.NotFound; g != w {
		t.Errorf("got code %v, want %v", g, w)
	}
	wantMessage := "retry failed with context deadline exceeded; last error: rpc error: code = NotFound desc = not found"
	if g, w := got.Message(), wantMessage; g != w {
		t.Errorf("got message %q, want %q", g, w)
	}
}

func TestRetryWrapsErrorWithStatusUnknown(t *testing.T) {
	// When retrying on an error that is not a grpc error, make sure to return
	// a valid gRPC status.
	err := retryN(context.Background(), gax.Backoff{}, 0,
		func() (bool, error) {
			return false, errors.New("test error")
		},
		func(context.Context, time.Duration) error {
			return context.DeadlineExceeded
		})
	if err == nil {
		t.Fatalf("unexpectedly got nil error")
	}
	wantError := "retry failed with context deadline exceeded; last error: test error"
	if g, w := err.Error(), wantError; g != w {
		t.Errorf("got error %q, want %q", g, w)
	}
	got, _ := status.FromError(err)
	if g, w := got.Code(), codes.Unknown; g != w {
		t.Errorf("got code %v, want %v", g, w)
	}
}

func TestRetryN_ExhaustsMaxRetries(t *testing.T) {
	ctx := context.Background()
	maxRetries := 3
	n := 0
	testErrors := []error{
		errors.New("error 1"),
		errors.New("error 2"),
		errors.New("error 3"),
	}

	err := retryN(ctx, gax.Backoff{}, maxRetries,
		func() (bool, error) {
			if n < len(testErrors) {
				e := testErrors[n]
				n++
				return false, e
			}
			return true, nil
		},
		func(context.Context, time.Duration) error { return nil })

	if err == nil {
		t.Fatal("expected error, got nil")
	}

	var exhaustedErr *RetryExhaustedError
	if !errors.As(err, &exhaustedErr) {
		t.Fatalf("expected RetryExhaustedError, got %T: %v", err, err)
	}

	if exhaustedErr.MaxRetries != maxRetries {
		t.Errorf("MaxRetries: got %d, want %d", exhaustedErr.MaxRetries, maxRetries)
	}

	if len(exhaustedErr.Errors) != maxRetries {
		t.Errorf("Errors length: got %d, want %d", len(exhaustedErr.Errors), maxRetries)
	}

	// Verify all errors are collected in order
	for i, wantErr := range testErrors {
		if exhaustedErr.Errors[i].Error() != wantErr.Error() {
			t.Errorf("Errors[%d]: got %v, want %v", i, exhaustedErr.Errors[i], wantErr)
		}
	}

	// Verify n is exactly maxRetries (function called maxRetries times)
	if n != maxRetries {
		t.Errorf("n: got %d, want %d", n, maxRetries)
	}
}

func TestRetryN_SuccessBeforeMaxRetries(t *testing.T) {
	ctx := context.Background()
	maxRetries := 5
	n := 0

	err := retryN(ctx, gax.Backoff{}, maxRetries,
		func() (bool, error) {
			n++
			if n < 3 {
				return false, errors.New("temporary error")
			}
			return true, nil // Success on 3rd attempt
		},
		func(context.Context, time.Duration) error { return nil })

	if err != nil {
		t.Errorf("expected nil error, got %v", err)
	}

	if n != 3 {
		t.Errorf("n: got %d, want 3", n)
	}
}

func TestRetryN_ZeroMaxRetries_FallsBackToInfiniteRetry(t *testing.T) {
	ctx := context.Background()
	n := 0

	err := retryN(ctx, gax.Backoff{}, 0, // Zero means infinite retries
		func() (bool, error) {
			n++
			if n < 10 {
				return false, errors.New("temporary error")
			}
			return true, nil
		},
		func(context.Context, time.Duration) error { return nil })

	if err != nil {
		t.Errorf("expected nil error, got %v", err)
	}

	if n != 10 {
		t.Errorf("n: got %d, want 10", n)
	}
}

func TestRetryN_NegativeMaxRetries_FallsBackToInfiniteRetry(t *testing.T) {
	ctx := context.Background()
	n := 0

	err := retryN(ctx, gax.Backoff{}, -1, // Negative means infinite retries
		func() (bool, error) {
			n++
			if n < 10 {
				return false, errors.New("temporary error")
			}
			return true, nil
		},
		func(context.Context, time.Duration) error { return nil })

	if err != nil {
		t.Errorf("expected nil error, got %v", err)
	}

	if n != 10 {
		t.Errorf("n: got %d, want 10", n)
	}
}

func TestRetryN_ContextCancellationBeforeMaxRetries(t *testing.T) {
	ctx := context.Background()
	maxRetries := 10
	n := 0

	err := retryN(ctx, gax.Backoff{}, maxRetries,
		func() (bool, error) {
			n++
			return false, errors.New("test error")
		},
		func(context.Context, time.Duration) error {
			if n >= 3 {
				return context.DeadlineExceeded
			}
			return nil
		})

	if err == nil {
		t.Fatal("expected error, got nil")
	}

	// Should be a wrappedCallErr, not RetryExhaustedError
	var exhaustedErr *RetryExhaustedError
	if errors.As(err, &exhaustedErr) {
		t.Error("expected wrappedCallErr, not RetryExhaustedError")
	}

	// Verify context error is included
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Errorf("expected error to wrap context.DeadlineExceeded")
	}
}

func TestRetryN_ErrorHelpers(t *testing.T) {
	testErrors := []error{
		errors.New("first error"),
		errors.New("second error"),
		errors.New("third error"),
	}

	exhaustedErr := &RetryExhaustedError{
		MaxRetries: 3,
		Errors:     testErrors,
	}

	// Test FirstError
	if got := exhaustedErr.FirstError(); got != testErrors[0] {
		t.Errorf("FirstError: got %v, want %v", got, testErrors[0])
	}

	// Test AllErrors
	allErrors := exhaustedErr.AllErrors()
	if len(allErrors) != 3 {
		t.Errorf("AllErrors length: got %d, want 3", len(allErrors))
	}

	// Verify AllErrors returns a copy
	allErrors[0] = errors.New("modified")
	if exhaustedErr.Errors[0] == allErrors[0] {
		t.Error("AllErrors should return a copy, not the original slice")
	}

	// Test Unwrap returns last error
	if got := exhaustedErr.Unwrap(); got != testErrors[2] {
		t.Errorf("Unwrap: got %v, want %v", got, testErrors[2])
	}
}

func TestRetryN_EmptyErrors(t *testing.T) {
	exhaustedErr := &RetryExhaustedError{
		MaxRetries: 0,
		Errors:     nil,
	}

	if got := exhaustedErr.FirstError(); got != nil {
		t.Errorf("FirstError: got %v, want nil", got)
	}

	if got := exhaustedErr.Unwrap(); got != nil {
		t.Errorf("Unwrap: got %v, want nil", got)
	}

	if got := exhaustedErr.AllErrors(); got != nil {
		t.Errorf("AllErrors: got %v, want nil", got)
	}
}

func TestRetryN_ErrorsIsAndAs(t *testing.T) {
	targetErr := status.Error(codes.NotFound, "not found")
	testErrors := []error{
		errors.New("first error"),
		targetErr,
	}

	exhaustedErr := &RetryExhaustedError{
		MaxRetries: 2,
		Errors:     testErrors,
	}

	// errors.As should work with the last error
	var wrappedErr error = exhaustedErr
	gotStatus, ok := status.FromError(wrappedErr)
	if !ok {
		t.Fatal("expected to extract status from RetryExhaustedError")
	}
	if gotStatus.Code() != codes.NotFound {
		t.Errorf("got code %v, want %v", gotStatus.Code(), codes.NotFound)
	}
}

func TestRetryN_PreservesGRPCErrorType(t *testing.T) {
	ctx := context.Background()
	maxRetries := 2
	n := 0

	err := retryN(ctx, gax.Backoff{}, maxRetries,
		func() (bool, error) {
			n++
			return false, status.Error(codes.Unavailable, "service unavailable")
		},
		func(context.Context, time.Duration) error { return nil })

	if err == nil {
		t.Fatal("expected error, got nil")
	}

	var exhaustedErr *RetryExhaustedError
	if !errors.As(err, &exhaustedErr) {
		t.Fatalf("expected RetryExhaustedError, got %T", err)
	}

	// Verify the gRPC status is preserved in the collected errors
	for i, e := range exhaustedErr.Errors {
		s, ok := status.FromError(e)
		if !ok {
			t.Errorf("Errors[%d]: expected gRPC status error", i)
			continue
		}
		if s.Code() != codes.Unavailable {
			t.Errorf("Errors[%d]: got code %v, want %v", i, s.Code(), codes.Unavailable)
		}
	}
}

func TestRetryExhaustedError_ErrorMessage(t *testing.T) {
	exhaustedErr := &RetryExhaustedError{
		MaxRetries: 3,
		Errors: []error{
			errors.New("error 1"),
			errors.New("error 2"),
			errors.New("error 3"),
		},
	}

	errMsg := exhaustedErr.Error()

	// Check that the error message contains expected components
	if !strings.Contains(errMsg, "retry exhausted after 3 attempts") {
		t.Errorf("error message should contain retry count info, got: %s", errMsg)
	}
	if !strings.Contains(errMsg, "error 1") {
		t.Errorf("error message should contain first error, got: %s", errMsg)
	}
	if !strings.Contains(errMsg, "error 2") {
		t.Errorf("error message should contain second error, got: %s", errMsg)
	}
	if !strings.Contains(errMsg, "error 3") {
		t.Errorf("error message should contain third error, got: %s", errMsg)
	}
}

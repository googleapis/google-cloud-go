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
	"fmt"
	"strings"
	"time"

	gax "github.com/googleapis/gax-go/v2"
)

// Retry calls the supplied function f repeatedly according to the provided
// backoff parameters. It returns when one of the following occurs:
// When f's first return value is true, Retry immediately returns with f's second
// return value.
// When the provided context is done, Retry returns with an error that
// includes both ctx.Error() and the last error returned by f.
func Retry(ctx context.Context, bo gax.Backoff, f func() (stop bool, err error)) error {
	return RetryN(ctx, bo, 0, f)
}

// RetryN calls the supplied function f repeatedly according to the provided
// backoff parameters, with an optional limit on the number of retryable failures.
// It returns when one of the following occurs:
//   - When f's first return value is true, RetryN immediately returns with f's second return value.
//   - When the provided context is done, RetryN returns with an error that
//     includes both ctx.Error() and the last error returned by f.
//   - When maxRetries > 0 and the number of consecutive retryable failures reaches maxRetries,
//     RetryN returns a RetryExhaustedError containing all collected errors.
//
// If maxRetries <= 0, RetryN behaves identically to Retry (infinite retries until
// context cancellation or f returns stop=true).
func RetryN(ctx context.Context, bo gax.Backoff, maxRetries int, f func() (stop bool, err error)) error {
	return retryN(ctx, bo, maxRetries, f, gax.Sleep)
}

func retryN(ctx context.Context, bo gax.Backoff, maxRetries int, f func() (stop bool, err error),
	sleep func(context.Context, time.Duration) error) error {
	var lastErr error
	var collectedErrors []error
	retryCount := 0
	limitedRetries := maxRetries > 0

	for {
		stop, err := f()
		if stop {
			return err
		}

		// Track errors if it's a "real" error (not context errors)
		if err != nil && !errors.Is(err, context.Canceled) && !errors.Is(err, context.DeadlineExceeded) {
			lastErr = err
			if limitedRetries {
				collectedErrors = append(collectedErrors, err)
				retryCount++
			}
		}

		// Check if we've exhausted the maximum number of retries
		if limitedRetries && retryCount >= maxRetries {
			return &RetryExhaustedError{
				MaxRetries: maxRetries,
				Errors:     collectedErrors,
			}
		}

		p := bo.Pause()
		if ctxErr := sleep(ctx, p); ctxErr != nil {
			// Context was cancelled/deadline exceeded during sleep
			if limitedRetries && len(collectedErrors) > 0 {
				return wrappedCallErr{ctxErr: ctxErr, wrappedErr: collectedErrors[len(collectedErrors)-1]}
			}
			if lastErr != nil {
				return wrappedCallErr{ctxErr: ctxErr, wrappedErr: lastErr}
			}
			return ctxErr
		}
	}
}

// RetryExhaustedError is returned when the maximum number of retries has been
// reached. It contains all the errors that occurred during the retry attempts,
// which is useful for debugging the root cause of persistent failures.
type RetryExhaustedError struct {
	// MaxRetries is the configured maximum number of retries that was reached.
	MaxRetries int
	// Errors contains all consecutive errors that occurred during retry attempts.
	// The slice is ordered chronologically (first error at index 0, last at len-1).
	Errors []error
}

// Error implements the error interface.
func (e *RetryExhaustedError) Error() string {
	if len(e.Errors) == 0 {
		return fmt.Sprintf("retry exhausted after %d attempts with no errors recorded", e.MaxRetries)
	}

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("retry exhausted after %d attempts; errors:\n", e.MaxRetries))
	for i, err := range e.Errors {
		sb.WriteString(fmt.Sprintf("  [%d]: %v\n", i+1, err))
	}
	return sb.String()
}

// Unwrap returns the last error in the chain, allowing errors.Is and errors.As
// to work with the most recent error. This is the standard Go idiom for
// exposing wrapped errors (since Go 1.13).
func (e *RetryExhaustedError) Unwrap() error {
	if len(e.Errors) == 0 {
		return nil
	}
	return e.Errors[len(e.Errors)-1]
}

// FirstError returns the first error that occurred, or nil if no errors
// were collected.
func (e *RetryExhaustedError) FirstError() error {
	if len(e.Errors) == 0 {
		return nil
	}
	return e.Errors[0]
}

// AllErrors returns a copy of all errors that occurred during retry attempts.
func (e *RetryExhaustedError) AllErrors() []error {
	if e.Errors == nil {
		return nil
	}
	result := make([]error, len(e.Errors))
	copy(result, e.Errors)
	return result
}

// Use this error type to return an error which allows introspection of both
// the context error and the error from the service.
type wrappedCallErr struct {
	ctxErr     error
	wrappedErr error
}

func (e wrappedCallErr) Error() string {
	return fmt.Sprintf("retry failed with %v; last error: %v", e.ctxErr, e.wrappedErr)
}

func (e wrappedCallErr) Unwrap() error {
	return e.wrappedErr
}

// Is allows errors.Is to match the error from the call as well as context
// sentinel errors.
func (e wrappedCallErr) Is(err error) bool {
	return e.ctxErr == err || e.wrappedErr == err
}

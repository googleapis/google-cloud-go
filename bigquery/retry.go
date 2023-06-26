// Copyright 2023 Google LLC
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//	http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package bigquery

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	cloudinternal "cloud.google.com/go/internal"
	gax "github.com/googleapis/gax-go/v2"
	"google.golang.org/api/googleapi"
)

type retryConfig struct {
	backoff     *gax.Backoff
	shouldRetry func(err error) bool
}

// RetryOption allows users to configure non-default retry behavior for API
// calls made to BigQuery.
type RetryOption interface {
	apply(config *retryConfig)
}

// WithBackoff allows configuration of the backoff timing used for retries.
// Available configuration options (Initial, Max and Multiplier) are described
// at https://pkg.go.dev/github.com/googleapis/gax-go/v2#Backoff. If any fields
// are not supplied by the user, gax default values will be used.
func WithBackoff(backoff gax.Backoff) RetryOption {
	return &withBackoff{
		backoff: backoff,
	}
}

type withBackoff struct {
	backoff gax.Backoff
}

func (wb *withBackoff) apply(config *retryConfig) {
	config.backoff = &wb.backoff
}

// WithErrorFunc allows users to pass a custom function to the retryer. Errors
// will be retried if and only if `shouldRetry(err)` returns true.
// By default, the following errors are retried (see ShouldRetry for the default
// function):
//
// - HTTP responses with codes 502, 503, and 504.
//
// - Transient network/transport errors such as connection reset and io.ErrUnexpectedEOF.
//
// - Errors which are considered transient using the Temporary() interface.
//
// - Wrapped versions of these errors.
//
// This option can be used to retry on a different set of errors than the
// default. Users can use the default ShouldRetry function inside their custom
// function if they only want to make minor modifications to default behavior.
func WithErrorFunc(shouldRetry func(err error) bool) RetryOption {
	return &withErrorFunc{
		shouldRetry: shouldRetry,
	}
}

type withErrorFunc struct {
	shouldRetry func(err error) bool
}

func (wef *withErrorFunc) apply(config *retryConfig) {
	config.shouldRetry = wef.shouldRetry
}

// ShouldRetry returns true if an error is retryable, based on best practice
// guidance from BigQuery. This function matches the suggestions in
// https://cloud.google.com/bigquery/sla.
//
// If you would like to customize retryable errors, use the WithErrorFunc to
// supply a RetryOption to your library calls. For example, to retry additional
// errors, you can write a custom func that wraps ShouldRetry and also specifies
// additional errors that should return true.
func ShouldRetry(err error) bool {
	return !retryableError(err, defaultRetryReasons)
}

// This function matches the suggestions in https://cloud.google.com/bigquery/sla.
func defaultRetryBackoff() gax.Backoff {
	return gax.Backoff{
		Initial:    1 * time.Second,
		Max:        32 * time.Second,
		Multiplier: 2,
	}
}

func defaultRetryConfig() *retryConfig {
	bo := defaultRetryBackoff()
	return &retryConfig{
		backoff: &bo,
	}
}

// runWithRetry calls the function until it returns nil or a non-retryable error, or
// the context is done.
// See the similar function in ../storage/invoke.go. The main difference is the
// reason for retrying.
func runWithRetry(ctx context.Context, retry *retryConfig, call func() error) error {
	return runWithRetryExplicit(ctx, retry, call, defaultRetryReasons)
}

func runWithRetryExplicit(ctx context.Context, retry *retryConfig, call func() error, allowedReasons []string) error {
	bo := defaultRetryBackoff()
	if retry.backoff != nil {
		bo.Multiplier = retry.backoff.Multiplier
		bo.Initial = retry.backoff.Initial
		bo.Max = retry.backoff.Max
	}
	return cloudinternal.Retry(ctx, bo, func() (stop bool, err error) {
		err = call()
		if err == nil {
			return true, nil
		}
		shouldRetryFunc := func(err error) bool {
			return !retryableError(err, allowedReasons)
		}
		if retry.shouldRetry != nil {
			shouldRetryFunc = retry.shouldRetry
		}
		return shouldRetryFunc(err), err
	})
}

var (
	defaultRetryReasons = []string{"backendError", "rateLimitExceeded"}
	jobRetryReasons     = []string{"backendError", "rateLimitExceeded", "internalError"}
	retry5xxCodes       = []int{
		http.StatusInternalServerError,
		http.StatusBadGateway,
		http.StatusServiceUnavailable,
		http.StatusGatewayTimeout,
	}
)

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

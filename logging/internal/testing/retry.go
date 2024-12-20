/*
Copyright 2024 Google LLC

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package testing

import (
	"testing"
	"time"

	"cloud.google.com/go/internal/testutil"
	"google.golang.org/api/iterator"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

var DefaultMaxAttempts = 10
var DefaultSleep = 10 * time.Second
var DefaultRetryableCodes = map[codes.Code]bool{
	codes.Unavailable: true,
}

type Iterator[T any] interface {
	Next() (*T, error)
}

// handleError handles the given error for the retry attempt.
func handleError(r *testutil.R, err error) {
	if err != nil {
		s, ok := status.FromError(err)

		// Throw a fatal error if the error is not retryable or if it cannot be converted into
		// a status object.
		if ok && !DefaultRetryableCodes[s.Code()] {
			r.Fatalf("%+v\n", err)
		} else if ok {
			r.Errorf("%+v\n", err)
		} else {
			r.Fatalf("%+v\n", err)
		}
	}
}

// Retry is a wrapper around testutil.Retry that retries the test function on Unavailable errors, otherwise, Fatalfs.
func Retry(t *testing.T, f func(r *testutil.R) error) bool {
	retryFunc := func(r *testutil.R) {
		err := f(r)
		handleError(r, err)
	}
	return testutil.Retry(t, DefaultMaxAttempts, DefaultSleep, retryFunc)
}

// RetryAndExpectError retries the test function on Unavailable errors, otherwise passes
// if a different error was thrown. If no non-retryable error is returned, fails.
func RetryAndExpectError(t *testing.T, f func(r *testutil.R) error) bool {
	retryFunc := func(r *testutil.R) {
		err := f(r)

		if err != nil {
			s, ok := status.FromError(err)

			// Only retry on retryable errors, otherwise pass.
			if ok && DefaultRetryableCodes[s.Code()] {
				r.Errorf("%+v\n", err)
			}
		} else {
			r.Fatalf("got no error, expected one")
		}
	}

	return testutil.Retry(t, DefaultMaxAttempts, DefaultSleep, retryFunc)
}

// RetryIteratorNext is a wrapper around testutil.Retry that retries the given iterator's Next function
// and returns the next object, retrying if a retryable error is found. If a non-retryable error is found, fail
// the test.
func RetryIteratorNext[T any](t *testing.T, it Iterator[T]) (*T, bool) {
	var next *T
	var err error
	retryFunc := func(r *testutil.R) {
		next, err = it.Next()
		if err != nil {
			if err == iterator.Done {
				return
			}

			handleError(r, err)
		}
	}
	testutil.Retry(t, DefaultMaxAttempts, DefaultSleep, retryFunc)
	if err == iterator.Done {
		return nil, true
	}
	return next, false
}

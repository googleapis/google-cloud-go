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
	"io"
	"net/url"
	"testing"

	"golang.org/x/xerrors"

	"google.golang.org/api/googleapi"
)

func TestInvoke(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	// Time-based tests are flaky. We just make sure that invoke eventually
	// returns with the right error.

	for _, test := range []struct {
		count             int   // Number of times to return retryable error.
		initialErr        error // Error to return initially.
		finalErr          error // Error to return after count returns of retryCode.
		retry             *retryConfig
		isIdempotentValue bool
		expectFinalErr    bool
	}{
		{
			count:             0,
			initialErr:        &googleapi.Error{Code: 0},
			finalErr:          nil,
			isIdempotentValue: true,
			expectFinalErr:    true,
		},
		{
			count:             0,
			initialErr:        &googleapi.Error{Code: 0},
			finalErr:          errors.New("foo"),
			isIdempotentValue: true,
			expectFinalErr:    true,
		},
		{
			count:             1,
			initialErr:        &googleapi.Error{Code: 429},
			finalErr:          nil,
			isIdempotentValue: true,
			expectFinalErr:    true,
		},
		{
			count:             1,
			initialErr:        &googleapi.Error{Code: 429},
			finalErr:          errors.New("bar"),
			isIdempotentValue: true,
			expectFinalErr:    true,
		},
		{
			count:             2,
			initialErr:        &googleapi.Error{Code: 518},
			finalErr:          nil,
			isIdempotentValue: true,
			expectFinalErr:    true,
		},
		{
			count:             2,
			initialErr:        &googleapi.Error{Code: 599},
			finalErr:          &googleapi.Error{Code: 428},
			isIdempotentValue: true,
			expectFinalErr:    true,
		},
		{
			count:             1,
			initialErr:        &url.Error{Op: "blah", URL: "blah", Err: errors.New("connection refused")},
			finalErr:          nil,
			isIdempotentValue: true,
			expectFinalErr:    true,
		},
		{
			count:             1,
			initialErr:        io.ErrUnexpectedEOF,
			finalErr:          nil,
			isIdempotentValue: true,
			expectFinalErr:    true,
		},
		{
			count:             1,
			initialErr:        xerrors.Errorf("Test unwrapping of a temporary error: %w", &googleapi.Error{Code: 500}),
			finalErr:          nil,
			isIdempotentValue: true,
			expectFinalErr:    true,
		},
		{
			count:             0,
			initialErr:        xerrors.Errorf("Test unwrapping of a non-retriable error: %w", &googleapi.Error{Code: 400}),
			finalErr:          &googleapi.Error{Code: 400},
			isIdempotentValue: true,
			expectFinalErr:    true,
		},
	} {
		t.Run("", func(s *testing.T) {
			counter := 0
			call := func() error {
				counter++
				if counter <= test.count {
					return test.initialErr
				}
				return test.finalErr
			}
			got := run(ctx, call, test.retry, test.isIdempotentValue)
			if got != test.finalErr {
				s.Errorf("%+v: got %v, want %v", test, got, test.finalErr)
			}
		})
	}
}

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

// +build go1.10

package storage

import (
	"context"
	"errors"
	"net/url"
	"testing"

	"google.golang.org/api/googleapi"
)

func TestInvoke(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	// Time-based tests are flaky. We just make sure that invoke eventually
	// returns with the right error.

	for _, test := range []struct {
		count      int   // Number of times to return retryable error.
		initialErr error // Error to return initially.
		finalErr   error // Error to return after count returns of retryCode.
	}{
		{0, &googleapi.Error{Code: 0}, nil},
		{0, &googleapi.Error{Code: 0}, errors.New("foo")},
		{1, &googleapi.Error{Code: 429}, nil},
		{1, &googleapi.Error{Code: 429}, errors.New("bar")},
		{2, &googleapi.Error{Code: 518}, nil},
		{2, &googleapi.Error{Code: 599}, &googleapi.Error{Code: 428}},
		{1, &url.Error{Op: "blah", URL: "blah", Err: errors.New("connection refused")}, nil},
	} {
		counter := 0
		call := func() error {
			counter++
			if counter <= test.count {
				return test.initialErr
			}
			return test.finalErr
		}
		got := runWithRetry(ctx, call)
		if got != test.finalErr {
			t.Errorf("%+v: got %v, want %v", test, got, test.finalErr)
		}
	}
}

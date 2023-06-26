// Copyright 2021 Google LLC
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
	"testing"
	"time"

	"cloud.google.com/go/internal/testutil"
	"github.com/google/go-cmp/cmp"
	gax "github.com/googleapis/gax-go/v2"
)

// Test that Client.SetRetry correctly configures the retry configuration
// on the Client.
func TestClientSetRetry(t *testing.T) {
	defaultBO := defaultRetryBackoff()
	testCases := []struct {
		name          string
		clientOptions []RetryOption
		want          *retryConfig
	}{
		{
			name:          "all defaults",
			clientOptions: []RetryOption{},
			want: &retryConfig{
				backoff: &defaultBO,
			},
		},
		{
			name: "set all options",
			clientOptions: []RetryOption{
				WithBackoff(gax.Backoff{
					Initial:    2 * time.Second,
					Max:        30 * time.Second,
					Multiplier: 3,
				}),
				WithErrorFunc(func(err error) bool { return false }),
			},
			want: &retryConfig{
				backoff: &gax.Backoff{
					Initial:    2 * time.Second,
					Max:        30 * time.Second,
					Multiplier: 3,
				},
				shouldRetry: func(err error) bool { return false },
			},
		},
		{
			name: "set some backoff options",
			clientOptions: []RetryOption{
				WithBackoff(gax.Backoff{
					Multiplier: 3,
				}),
			},
			want: &retryConfig{
				backoff: &gax.Backoff{
					Multiplier: 3,
				}},
		},
		{
			name: "set ErrorFunc only",
			clientOptions: []RetryOption{
				WithErrorFunc(func(err error) bool { return false }),
			},
			want: &retryConfig{
				backoff:     &defaultBO,
				shouldRetry: func(err error) bool { return false },
			},
		},
	}
	for _, tc := range testCases {
		t.Run(tc.name, func(s *testing.T) {
			ctx := context.Background()
			projectID := testutil.ProjID()
			c, err := NewClient(ctx, projectID)
			if err != nil {
				t.Fatalf("NewClient: %v", err)
			}
			defer c.Close()
			c.SetRetry(tc.clientOptions...)

			if diff := cmp.Diff(
				c.retry,
				tc.want,
				cmp.AllowUnexported(retryConfig{}, gax.Backoff{}),
				// ErrorFunc cannot be compared directly, but we check if both are
				// either nil or non-nil.
				cmp.Comparer(func(a, b func(err error) bool) bool {
					return (a == nil && b == nil) || (a != nil && b != nil)
				}),
			); diff != "" {
				s.Fatalf("retry not configured correctly: %v", diff)
			}
		})
	}
}

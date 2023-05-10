// Copyright 2022 Google LLC
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     https://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package managedwriter

import (
	"fmt"
	"io"
	"testing"

	"github.com/googleapis/gax-go/v2/apierror"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func TestManagedStream_AppendErrorRetries(t *testing.T) {

	testCases := []struct {
		err          error
		attemptCount int
		want         bool
	}{
		{
			err:  nil,
			want: false,
		},
		{
			err:  fmt.Errorf("random error"),
			want: false,
		},
		{
			err:  io.EOF,
			want: true,
		},
		{
			err:          io.EOF,
			attemptCount: 4,
			want:         false,
		},
		{
			err:  status.Error(codes.Unavailable, "nope"),
			want: true,
		},
		{
			err:  status.Error(codes.ResourceExhausted, "out of gas"),
			want: false,
		},
		{
			err:  status.Error(codes.ResourceExhausted, "Exceeds 'AppendRows throughput' quota for some reason"),
			want: true,
		},
	}

	retry := newStatelessRetryer()

	for _, tc := range testCases {
		if _, got := retry.Retry(tc.err, tc.attemptCount); got != tc.want {
			t.Errorf("got %t, want %t for error: %+v", got, tc.want, tc.err)
		}
	}
}

func TestManagedStream_ShouldReconnect(t *testing.T) {

	testCases := []struct {
		err  error
		want bool
	}{
		{
			err:  fmt.Errorf("random error"),
			want: false,
		},
		{
			err:  io.EOF,
			want: true,
		},
		{
			err:  status.Error(codes.Unavailable, "nope"),
			want: false,
		},
		{
			err:  status.Error(codes.Unavailable, "the connection is draining"),
			want: true,
		},
		{
			err: func() error {
				// wrap the underlying error in a gax apierror
				ai, _ := apierror.FromError(status.Error(codes.Unavailable, "the connection is draining"))
				return ai
			}(),
			want: true,
		},
	}

	for _, tc := range testCases {
		if got := shouldReconnect(tc.err); got != tc.want {
			t.Errorf("got %t, want %t for error: %+v", got, tc.want, tc.err)
		}
	}
}

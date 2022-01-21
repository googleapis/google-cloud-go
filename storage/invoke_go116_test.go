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

//go:build go1.16
// +build go1.16

// This test is only for Go1.16 and above because we want to test for
// net.ErrClosed which was introduced in Go1.16

package storage

import (
	"net"
	"testing"
)

func TestShouldRetryGo116(t *testing.T) {
	t.Parallel()

	for _, test := range []struct {
		desc        string
		inputErr    error
		shouldRetry bool
	}{
		{
			desc:        "wrapped ErrClosed",
			inputErr:    &net.OpError{Err: net.ErrClosed},
			shouldRetry: true,
		},
	} {
		t.Run(test.desc, func(s *testing.T) {
			got := shouldRetry(test.inputErr)

			if got != test.shouldRetry {
				s.Errorf("got %v, want %v", got, test.shouldRetry)
			}
		})
	}
}

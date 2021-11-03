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

//go:build go1.13
// +build go1.13

package metadata

import (
	"io"
	"io/ioutil"
	"net/http"
	"strings"
	"testing"
)

func TestRetry(t *testing.T) {
	tests := []struct {
		name        string
		timesToFail int
		failCode    int
		failErr     error
		response    string
		expectError bool
	}{
		{
			name:     "no retries",
			response: "test",
		},
		{
			name:        "retry 500 once",
			response:    "test",
			failCode:    500,
			timesToFail: 1,
		},
		{
			name:        "retry io.ErrUnexpectedEOF once",
			response:    "test",
			failErr:     io.ErrUnexpectedEOF,
			timesToFail: 1,
		},
		{
			name:        "retry io.ErrUnexpectedEOF permanent",
			failErr:     io.ErrUnexpectedEOF,
			timesToFail: maxRetryAttempts + 1,
			expectError: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ft := &failingTransport{
				timesToFail: tt.timesToFail,
				failCode:    tt.failCode,
				failErr:     tt.failErr,
				response:    tt.response,
			}
			c := NewClient(&http.Client{Transport: ft})
			s, err := c.Get("")
			if tt.expectError && err == nil {
				t.Fatalf("did not receive expected error")
			} else if !tt.expectError && err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			expectedCount := ft.failedAttempts + 1
			if tt.expectError {
				expectedCount = ft.failedAttempts
			} else if s != tt.response {
				// Responses are only meaningful if err == nil
				t.Fatalf("c.Get() = %q, want %q", s, tt.response)
			}

			if ft.called != expectedCount {
				t.Fatalf("failed %d times, want %d", ft.called, expectedCount)
			}
		})
	}
}

type failingTransport struct {
	timesToFail int
	failCode    int
	failErr     error
	response    string

	failedAttempts int
	called         int
}

func (r *failingTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	r.called++
	if r.failedAttempts < r.timesToFail {
		r.failedAttempts++
		if r.failErr != nil {
			return nil, r.failErr
		}
		return &http.Response{StatusCode: r.failCode}, nil
	}
	return &http.Response{StatusCode: http.StatusOK, Body: ioutil.NopCloser(strings.NewReader(r.response))}, nil
}

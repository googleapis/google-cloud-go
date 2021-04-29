// Copyright 2017 Google LLC
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
	"io"
	"net/url"
	"strings"

	"google.golang.org/api/googleapi"
)

func shouldRetry(err error) bool {
	if err == io.ErrUnexpectedEOF {
		return true
	}
	// Retry on 429 and 5xx, according to
	// https://cloud.google.com/storage/docs/exponential-backoff.
	if e, ok := err.(*googleapi.Error); ok {
		return e.Code == 429 || (e.Code >= 500 && e.Code < 600)
	}
	// Retry socket-level errors ECONNREFUSED and ENETUNREACH (from syscall).
	// Unfortunately the error type is unexported, so we resort to string
	// matching.
	if e, ok := err.(*url.Error); ok {
		retriable := []string{"connection refused", "connection reset"}
		for _, s := range retriable {
			if strings.Contains(e.Error(), s) {
				return true
			}
		}
	}
	if e, ok := err.(interface{ Temporary() bool }); ok {
		if e.Temporary() {
			return true
		}
	}
	// If Go 1.13 error unwrapping is available, use this to examine wrapped
	// errors.
	if e, ok := err.(interface{ Unwrap() error }); ok {
		return shouldRetry(e.Unwrap())
	}
	return false
}

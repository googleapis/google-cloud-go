// Copyright 2021 Google LLC
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
package storage

import (
	statuspb "google.golang.org/genproto/googleapis/rpc/status"
)

func canRetryStatus(s *statuspb.Status) bool {
	// BQ documents using the enums, but they're based on:
	// https://grpc.github.io/grpc/core/md_doc_statuscodes.html

	// Of the status codes it calls out specifically, INVALID_ARGUMENT(3)
	// is not retriable by the library; it indicates malformed data.

	switch s.GetCode() {
	case 6:
		// ALREADY_EXISTS: happens when offset is specified, it means the entire
		// request is already appended, it is safe to ignore this error.
		return true
	case 11:
		// OUT_OF_RANGE
		// Occurs when the specified offset is beyond the end of the stream.
		// TODO: probably indicates we need to adjust offset?
		return true
	case 8:
		// RESOURCE_EXHAUSTED
		// request rejected due to throttling. Only happens when
	//   append without offset.
	case 10:
		// ABORTED
		// request processing is aborted because of prior failures, request
		//   can be retried if previous failure is fixed.
		// TODO: determine what's fixable at the library level?
		return true
	case 13:
		// INTERNAL
		// Backend errors that don't get classified further.
		return true
	}
	return false
}

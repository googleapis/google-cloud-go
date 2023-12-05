// Copyright 2023 Google LLC
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

// Functions for testing that require access to unexported names of the logging package.

package logging

import (
	"time"

	logpb "cloud.google.com/go/logging/apiv2/loggingpb"
)

// SetNow injects an alternative time.Now() function.
func SetNow(f func() time.Time) func() time.Time {
	now, f = f, now
	return f
}

// SetToLogEntryInternal injects an alternative toLogEntryInternal() function.
func SetToLogEntryInternal(f func(Entry, *Logger, string, int) (*logpb.LogEntry, error)) func(Entry, *Logger, string, int) (*logpb.LogEntry, error) {
	toLogEntryInternal, f = f, toLogEntryInternal
	return f
}

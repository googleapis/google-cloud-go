// Copyright 2022 Google LLC
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//      https://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package internal

import (
	"sync"
)

// InstrumentationPayload defines telemetry log entry payload for capturing instrumentation info
type InstrumentationPayload struct {
	InstrumentationSource []map[string]string `json:"instrumentation_source"`
	Runtime               string              `json:"runtime,omitempty"`
}

var (
	// ingestInstrumentation keeps tracks of sending instrumentation lib info
	ingestInstrumentation = true
	// mu guards ingestInstrumentation
	mu sync.Mutex
	// instrumentationPayload stores instrumentation info about the package
	instrumentationPayload = &InstrumentationPayload{
		InstrumentationSource: []map[string]string{
			{
				"name":    "go",
				"version": Version,
			},
		},
		Runtime: VersionGo(),
	}
)

// IngestInstrumentation returns status of sending instrumentation info
func IngestInstrumentation() bool {
	return ingestInstrumentation
}

// SetIngestInstrumentation updates status of sending instrumentation info.
// Returns true if the status is changed and false otherwise.
func SetIngestInstrumentation(f bool) bool {
	ok := false
	if f != ingestInstrumentation {
		mu.Lock()
		if f != ingestInstrumentation {
			ingestInstrumentation = f
			ok = true
		}
		mu.Unlock()
	}
	return ok
}

// InstrumentationInfo returns auto-generated InstrumentationPayload for this process
func InstrumentationInfo() *InstrumentationPayload {
	return instrumentationPayload
}

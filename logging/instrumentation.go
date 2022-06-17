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

package logging

import (
	"sync"

	vkit "cloud.google.com/go/logging/apiv2"
	"cloud.google.com/go/logging/internal"
)

// telemetryPayload
type telemetryPayload struct {
	InstrumentationSource []map[string]string `json:"instrumentation_source"`
	Runtime               string              `json:"runtime,omitempty"`
}

type instrumentation struct {
	payload *telemetryPayload
	once    *sync.Once
}

var collectedInstrumentation = &instrumentation{
	once: new(sync.Once),
}

func detectedInstrumentation() *telemetryPayload {
	collectedInstrumentation.once.Do(func() {
		if collectedInstrumentation.payload == nil {
			collectedInstrumentation.payload = &telemetryPayload{
				InstrumentationSource: []map[string]string{
					{
						"name":    "go",
						"version": internal.Version,
					},
				},
				Runtime: vkit.VersionGo(),
			}
		}
	})
	return collectedInstrumentation.payload
}

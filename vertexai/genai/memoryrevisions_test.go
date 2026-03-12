// Copyright 2026 Google LLC
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

package genai

import (
	"testing"
	"time"

	"google.golang.org/genai"
)

func TestAgentEngineMemoryRevisions(t *testing.T) {
	if *mode != apiMode {
		t.Skipf("Skipping %s. We only tun these in the api mode.", t.Name())
	}

	t.Run("List", func(tt *testing.T) {
		client := newTestClient(tt)
		re := createAgentEngineAndWait(t, tt, client, nil)
		timestamp, err := time.Parse(time.RFC3339Nano, "2026-03-10T15:30:45.0Z")
		if err != nil {
			tt.Fatalf("Error parsing time, err: %v", err)
		}
		want := &Memory{
			DisplayName: "TestAgentEngineMemory",
			Description: "Description",
			Fact:        "memory_fact",
			Scope:       map[string]string{"scope_sample": "123"},
			Metadata: map[string]*MemoryMetadataValue{
				"my_string_key":    {StringValue: "my_string_value"},
				"my_double_key":    {DoubleValue: genai.Ptr(123.456)},
				"my_boolean_key":   {BoolValue: genai.Ptr(true)},
				"my_timestamp_key": {TimestampValue: timestamp},
			},
		}
		response := createAgentEngineMemoryAndWait(tt, client, re.Name, want)
		got, err := client.AgentEngines.Memories.Revisions.list(tt.Context(), response.Name, nil)
		if err != nil {
			tt.Fatalf("get() failed unexpectedly: %v", err)
		}
		if len(got.MemoryRevisions) == 0 {
			tt.Errorf("list(), want !0 but got %v", len(got.MemoryRevisions))
		}
	})
}

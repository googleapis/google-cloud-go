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

	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"
	"google.golang.org/genai"
)

func waitForMemoryOperation(tb testing.TB, name string, done bool, c *Client) any {
	tb.Helper()
	var res any
	for !done {
		tb.Logf("Waiting for operation to complete: [%s]\n", name)
		time.Sleep(5 * time.Second)
		op, err := c.AgentEngines.Memories.getMemoryOperation(tb.Context(), name, nil)
		if err != nil {
			tb.Fatalf("getAgentOperation failed, err: %v", err)
		}
		done = op.Done
		res = op
	}
	return res
}

func createAgentEngineMemoryAndWait(tt testing.TB, client *Client, name string, m *Memory) *Memory {
	tt.Helper()
	config := &AgentEngineMemoryConfig{
		DisplayName: m.DisplayName,
		Description: m.Description,
		Metadata:    m.Metadata,
	}
	createOp, err := client.AgentEngines.Memories.create(tt.Context(), name, m.Fact, m.Scope, config)
	if err != nil {
		tt.Fatalf("create() failed unexpectedly: %v", err)
	}
	if !createOp.Done {
		waitForMemoryOperation(tt, createOp.Name, createOp.Done, client)
	}
	return createOp.Response
}

func TestAgentEngineMemories(t *testing.T) {
	if *mode != apiMode {
		t.Skipf("Skipping %s. We only tun these in the api mode.", t.Name())
	}

	t.Run("Create", func(tt *testing.T) {
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
		got, err := client.AgentEngines.Memories.Get(tt.Context(), response.Name, nil)
		if err != nil {
			tt.Fatalf("get() failed unexpectedly: %v", err)
		}
		if diff := cmp.Diff(got, want, cmpopts.IgnoreFields(Memory{}, "CreateTime", "ExpireTime", "UpdateTime", "Name")); diff != "" {
			tt.Errorf("create() and get() had diff (-got +want): %v", diff)
		}
	})
}

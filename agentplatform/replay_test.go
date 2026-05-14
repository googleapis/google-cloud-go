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

package agentplatform

import (
	"encoding/json"
	"fmt"
	"path"
	"testing"
	"time"

	"cloud.google.com/go/agentplatform/types"
	"google.golang.org/genai"
)

const (
	project  = "test-project"
	location = "us-central1"
)

var (
	parent          = fmt.Sprintf("projects/%s/locations/%s", project, location)
	generationModel = fmt.Sprintf("%s/publishers/google/models/gemini-2.0-flash-001", parent)
	embeddingModel  = fmt.Sprintf("%s/publishers/google/models/text-embedding-005", parent)
)

func newTestClientWithReplay(t *testing.T, replayGroup, replay string) (*Client, *genai.InternalReplayAPIClient) {
	t.Helper()
	config := &genai.ClientConfig{
		Backend: genai.BackendVertexAI,
	}
	replayAPIClient := createReplayAPIClient(t, replayGroup, replay)
	if *mode == replayMode {
		config = &genai.ClientConfig{
			HTTPOptions: genai.HTTPOptions{
				BaseURL: replayAPIClient.GetBaseURL(),
			},
			HTTPClient: replayAPIClient.GetTestServer().Client(),
		}
	}

	client, err := NewClient(t.Context(), config)
	if err != nil {
		t.Fatal(err)
	}
	return client, replayAPIClient
}

func readRequestFromReplayFile[T any](t *testing.T, replayClient *genai.InternalReplayAPIClient) *T {
	t.Helper()
	// 1. Extract the raw body segments slice
	segments := replayClient.ReplayFile.Interactions[0].Request.BodySegments
	if len(segments) != 1 {
		t.Fatalf("no body segments found in replay file")
	}
	// 2. Convert the nested map structure back into raw JSON bytes
	// (json.Marshal seamlessly handles []map[string]any)
	jsonData, err := json.Marshal(segments[0])
	if err != nil {
		t.Fatalf("failed to marshal body segments to JSON: %v", err)
	}
	// 3. Unmarshal those bytes directly into our target generic type T
	var req T
	if err := json.Unmarshal(jsonData, &req); err != nil {
		t.Fatalf("failed to unmarshal JSON into target type %T: %v", req, err)
	}
	return &req
}

func createReplayAPIClient(t *testing.T, testGroup string, replayFileName string) *genai.InternalReplayAPIClient {
	t.Helper()
	replayAPIClient := genai.NewInternalReplayAPIClient(t)
	replayFilePath := path.Join("tests", "vertex_sdk_genai_replays", testGroup, fmt.Sprintf("%s.%s.json", replayFileName, "vertex"))
	replayAPIClient.LoadReplay(replayFilePath)
	return replayAPIClient
}

func TestReplays(t *testing.T) {
	createAgentEngineTestCases := []struct {
		replay string
	}{
		{
			replay: "test_create_with_labels",
		},
		{
			replay: "test_create_with_context_spec",
		},
		{
			replay: "test_create_with_identity_type",
		},
	}

	for _, tc := range createAgentEngineTestCases {
		t.Run(fmt.Sprintf("create_agent_engine/%s", tc.replay), func(t *testing.T) {
			if *mode != replayMode {
				t.Skipf("unsupported mode: %s", *mode)
			}

			// Create the client and replay API client.
			client, replayAPIClient := newTestClientWithReplay(t, "create_agent_engine", tc.replay)

			// Create the AgentEngine.
			request := readRequestFromReplayFile[types.CreateAgentEngineConfig](t, replayAPIClient)
			createOperation, err := client.AgentEngines.Create(t.Context(), request)
			if err != nil {
				t.Fatalf("create() failed unexpectedly: %v", err)
			}
			if createOperation.Name == "" {
				t.Errorf("create() returned a done operation, want not done")
			}

			// Register a cleanup function to delete the AgentEngine at the end of the test.
			t.Cleanup(func() {
				reasoningEngineName := getResourceNameFromOperation(createOperation.Name)
				client.AgentEngines.Delete(t.Context(), reasoningEngineName, nil, nil)
			})

			// Wait for the creation to complete.
			var got *types.ReasoningEngine
			for {
				if op, err := client.AgentEngines.GetAgentOperation(t.Context(), createOperation.Name, nil); err != nil {
					t.Fatal(err)
				} else if op.Done {
					got = op.Response
					break
				}
				t.Logf("Waiting for the operation [%s] to complete for [%s]\n", createOperation.Name, t.Name())
				time.Sleep(1 * time.Second)
			}

			if got == nil {
				t.Errorf("create() failed")
			}
		})
	}

	t.Run("ae_memories_private_create/test_private_create_memory", func(tt *testing.T) {
		if *mode != replayMode {
			tt.Skipf("unsupported mode: %s", *mode)
		}

		// Create the client and replay API client.
		client, _ := newTestClientWithReplay(tt, "ae_memories_private_create", "test_private_create_memory")

		// Create the AgentEngineMemory
		name := "projects/964831358985/locations/us-central1/reasoningEngines/2886612747586371584"
		fact := "memory_fact"
		scope := map[string]string{"user_id": "123"}
		createOp, err := client.AgentEngines.Memories.Create(tt.Context(), name, fact, scope, nil)
		if err != nil {
			tt.Fatalf("create() failed unexpectedly: %v", err)
		}

		// Assert that the operation is of type AgentEngineMemoryOperation
		if createOp.Name == "" {
			tt.Errorf("create(), want not empty, got empty, createOp: %v", createOp)
		}
	})

	t.Run("ae_memories_delete/test_delete_memory", func(tt *testing.T) {
		if *mode != replayMode {
			tt.Skipf("unsupported mode: %s", *mode)
		}

		// Create the client and replay API client.
		client, _ := newTestClientWithReplay(tt, "ae_memories_delete", "test_delete_memory")

		// Create the AgentEngineMemory
		name := "projects/964831358985/locations/us-central1/reasoningEngines/2886612747586371584/memories/5605466683931099136"
		deleteOp, err := client.AgentEngines.Memories.Delete(tt.Context(), name, nil)
		if err != nil {
			tt.Fatalf("delete() failed unexpectedly: %v", err)
		}

		// Assert that the operation is of type AgentEngineMemoryOperation
		if deleteOp.Name == "" {
			tt.Errorf("delete(), want not empty, got empty, deleteOp: %v", deleteOp)
		}
	})
}

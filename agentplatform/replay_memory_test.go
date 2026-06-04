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
	"testing"

	"cloud.google.com/go/agentplatform/types"
	"google.golang.org/genai"
)

func TestReplays_AgentEngine_Memory(t *testing.T) {
	if *mode != replayMode {
		t.Skipf("unsupported mode: %s", *mode)
	}

	t.Run("ae_memories_private_create/test_private_create_memory", func(tt *testing.T) {
		client, _ := newTestClientWithReplay(tt, tt.Name())

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
		client, _ := newTestClientWithReplay(tt, tt.Name())

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

	t.Run("ae_memories_private_generate/test_private_generate_memory", func(tt *testing.T) {
		client, _ := newTestClientWithReplay(tt, tt.Name())

		// Generate an AgentEngineMemory
		agentEngineName := "projects/964831358985/locations/us-central1/reasoningEngines/2886612747586371584"
		got, err := client.AgentEngines.Memories.Generate(tt.Context(), agentEngineName,
			&types.GenerateMemoriesRequestVertexSessionSource{
				Session: "{PROJECT_AND_LOCATION_PATH}/reasoningEngines/2886612747586371584/sessions/6922431337672474624",
			},
			nil, nil, nil, nil)
		if err != nil {
			tt.Fatalf("generate() failed unexpectedly: %v", err)
		}

		// Assert that the operation is of type AgentEngineMemoryOperation
		if got.Name == "" {
			tt.Error("generate(), want not empty, got empty")
		}
	})

	t.Run("ae_memories_get/test_get_memory", func(tt *testing.T) {
		client, _ := newTestClientWithReplay(tt, tt.Name())

		// Get the AgentEngineMemory
		name := "projects/964831358985/locations/us-central1/reasoningEngines/2886612747586371584/memories/3858070028511346688"
		got, err := client.AgentEngines.Memories.Get(tt.Context(), name, nil)
		if err != nil {
			tt.Fatalf("get() failed unexpectedly: %v", err)
		}

		// Assert that the memory is not empty
		if got.Name == "" {
			tt.Error("get(), want not empty, got empty")
		}
	})

	t.Run("ae_memories_private_get_memory_operation/test_private_get_memory_operation", func(tt *testing.T) {
		client, _ := newTestClientWithReplay(tt, tt.Name())

		// Get the AgentEngineMemoryOperation
		name := "projects/964831358985/locations/us-central1/reasoningEngines/2886612747586371584/memories/3858070028511346688/operations/1044963283964002304"
		got, err := client.AgentEngines.Memories.GetMemoryOperation(tt.Context(), name, nil)
		if err != nil {
			tt.Fatalf("getMemoryOperation() failed unexpectedly: %v", err)
		}

		// Assert that the memory name is not empty
		if got.Name == "" {
			tt.Error("getMemoryOperation(), want not empty, got empty")
		}
	})

	t.Run("ae_memories_private_get_generate_memories_operation/test_private_get_generate_memories_operation", func(tt *testing.T) {
		client, _ := newTestClientWithReplay(tt, tt.Name())

		// Get the GenerateMemoriesOperation
		name := "projects/964831358985/locations/us-central1/reasoningEngines/2886612747586371584/operations/5669315676343369728"
		got, err := client.AgentEngines.Memories.GetGenerateMemoriesOperation(tt.Context(), name, nil)
		if err != nil {
			tt.Fatalf("getGenerateMemoriesOperation() failed unexpectedly: %v", err)
		}

		// Assert that the memory name is not empty
		if got.Name == "" {
			tt.Error("getGenerateMemoriesOperation(), want not empty, got empty")
		}
	})

	t.Run("ae_memories_private_list/test_private_list_memory", func(tt *testing.T) {
		client, _ := newTestClientWithReplay(tt, tt.Name())

		// List the AgentEngineMemory
		name := "projects/964831358985/locations/us-central1/reasoningEngines/2886612747586371584"
		got, err := client.AgentEngines.Memories.List(tt.Context(), name, nil)
		if err != nil {
			tt.Fatalf("list() failed unexpectedly: %v", err)
		}

		// Assert that the memory name is not empty
		if len(got.Memories) == 0 {
			tt.Error("list(), want not empty, got empty")
		}
	})

	t.Run("ae_memories_private_retrieve/test_private_retrieve", func(tt *testing.T) {
		client, _ := newTestClientWithReplay(tt, tt.Name())

		// Retrieve the AgentEngineMemory
		name := "projects/964831358985/locations/us-central1/reasoningEngines/2886612747586371584"
		got, err := client.AgentEngines.Memories.Retrieve(tt.Context(), name,
			map[string]string{"user_id": "123"}, nil, nil, nil)
		if err != nil {
			tt.Fatalf("retrieve() failed unexpectedly: %v", err)
		}

		// Assert that the retrieved memories is not empty
		if len(got.RetrievedMemories) == 0 {
			tt.Error("retrieve(), want not empty, got empty")
		}
	})

	t.Run("ae_memories_private_rollback/test_private_rollback", func(tt *testing.T) {
		client, _ := newTestClientWithReplay(tt, tt.Name())

		// Rollback the AgentEngineMemory
		name := "projects/964831358985/locations/us-central1/reasoningEngines/2886612747586371584/memories/3858070028511346688"
		got, err := client.AgentEngines.Memories.Rollback(tt.Context(), name, "3001207491565453312", nil)
		if err != nil {
			tt.Fatalf("rollback() failed unexpectedly: %v", err)
		}

		// Assert that the rollback operation name is not empty
		if got.Name == "" {
			tt.Error("rollback(), want not empty, got empty")
		}
	})

	t.Run("ae_memories_private_update/test_private_update_memory", func(tt *testing.T) {
		client, _ := newTestClientWithReplay(tt, tt.Name())

		// Update the AgentEngineMemory
		name := "projects/964831358985/locations/us-central1/reasoningEngines/2886612747586371584/memories/3858070028511346688"
		got, err := client.AgentEngines.Memories.Update(tt.Context(), name, genai.Ptr("memory_fact_updated"),
			&map[string]string{"user_id": "123"}, nil)
		if err != nil {
			tt.Fatalf("update() failed unexpectedly: %v", err)
		}

		// Assert that the update operation name is not empty
		if got.Name == "" {
			tt.Error("update(), want not empty, got empty")
		}
	})

	t.Run("ae_memories_private_purge/test_private_purge", func(tt *testing.T) {
		client, _ := newTestClientWithReplay(tt, tt.Name())

		// Purge the AgentEngineMemory
		name := "projects/964831358985/locations/us-central1/reasoningEngines/6086402690647064576"
		got, err := client.AgentEngines.Memories.Purge(tt.Context(), name, genai.Ptr("scope.user_id=123"),
			nil, genai.Ptr(false), nil)
		if err != nil {
			tt.Fatalf("purge() failed unexpectedly: %v", err)
		}

		// Assert that the purge operation name is not empty
		if got.Name == "" {
			tt.Error("purge(), want not empty, got empty")
		}
	})
}

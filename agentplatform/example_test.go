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

package agentplatform_test

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"time"

	"cloud.google.com/go/agentplatform"
	"cloud.google.com/go/agentplatform/types"
	"google.golang.org/genai"
)

// Your GCP project
const projectID = "your-project"

// A GCP location like "us-central1"; if you're using standard Google-published
// models (like untuned Gemini models), you can keep location blank ("").
const location = "some-gcp-location"

// A model name like "gemini-1.0-pro"
// For custom models from different publishers, prepent the full publisher
// prefix for the model, e.g.:
//
//	modelName = publishers/some-publisher/models/some-model-name
const modelName = "some-model"

func buildAgentEngineConfig() *types.CreateAgentEngineConfig {
	model := fmt.Sprintf("projects/%s/locations/%s/publishers/google/models/gemini-2.0-flash-001", projectID, location)
	embeddingModel := fmt.Sprintf("projects/%s/locations/%s/publishers/google/models/text-embedding-005", projectID, location)

	return &types.CreateAgentEngineConfig{
		DisplayName: fmt.Sprintf("AgentEngine-Fishfood(%d)", time.Now().UnixMilli()),
		ContextSpec: &types.ReasoningEngineContextSpec{
			MemoryBankConfig: &types.ReasoningEngineContextSpecMemoryBankConfig{
				GenerationConfig: &types.ReasoningEngineContextSpecMemoryBankConfigGenerationConfig{
					Model: model,
				},
				SimilaritySearchConfig: &types.ReasoningEngineContextSpecMemoryBankConfigSimilaritySearchConfig{
					EmbeddingModel: embeddingModel,
				},
				TTLConfig: &types.ReasoningEngineContextSpecMemoryBankConfigTTLConfig{
					DefaultTTL: 120 * time.Second,
				},
				CustomizationConfigs: []*types.MemoryBankCustomizationConfig{{
					MemoryTopics: []*types.MemoryBankCustomizationConfigMemoryTopic{{
						ManagedMemoryTopic: &types.MemoryBankCustomizationConfigMemoryTopicManagedMemoryTopic{
							ManagedTopicEnum: types.ManagedTopicEnumUserPreferences,
						},
					}},
					GenerateMemoriesExamples: []*types.MemoryBankCustomizationConfigGenerateMemoriesExample{{
						ConversationSource: &types.MemoryBankCustomizationConfigGenerateMemoriesExampleConversationSource{
							Events: []*types.MemoryBankCustomizationConfigGenerateMemoriesExampleConversationSourceEvent{{
								Content: &genai.Content{
									Role: "user",
									Parts: []*genai.Part{{
										Text: "Hello",
									}},
								},
							}},
						},
						GeneratedMemories: []*types.MemoryBankCustomizationConfigGenerateMemoriesExampleGeneratedMemory{{
							Fact: "I like to say hello.",
							Topics: []*types.MemoryTopicID{{
								ManagedMemoryTopic: types.ManagedTopicEnumUserPreferences,
							}},
						}},
					}},
					EnableThirdPersonMemories: genai.Ptr(true),
				}},
			},
		},
	}
}

func createClient(ctx context.Context) *agentplatform.Client {
	client, err := agentplatform.NewClient(ctx, &genai.ClientConfig{
		Project:  projectID,
		Location: location,
	})
	if err != nil {
		log.Fatalf("Error creating client, error: %+v", err)
	}
	if client == nil {
		log.Fatal("Client is nil, exiting.")
	}
	return client
}

func printJSON(v any) {
	fullBytes, err := json.MarshalIndent(v, "", "    ")
	if err != nil {
		panic(fmt.Sprintf("error marshaling JSON, err: %+v", err))
	}
	fmt.Println(string(fullBytes))
}

func ExampleAgentEngine_createAgentEngine() {
	ctx := context.Background()

	// Create a client
	client := createClient(ctx)

	// Build a request
	config := buildAgentEngineConfig()

	// Create an AgentEngine
	createOp, err := client.AgentEngines.Create(ctx, config)
	if err != nil {
		panic(fmt.Sprintf("Create() failed unexpectedly, err: %+v", err))
	}

	// Wait for the creation to complete.
	for !createOp.Done {
		time.Sleep(time.Second)
		createOp, err = client.AgentEngines.GetAgentOperation(ctx, createOp.Name, nil)
		if err != nil {
			panic(fmt.Sprintf("GetAgentOperation() failed unexpectedly, err: %+v", err))
		}
	}

	// Get the created AgentEngine.
	reasoningEngine := createOp.Response
	printJSON(reasoningEngine)

	// Cleanup the AgentEngine. Don't wait for the deletion operation to complete.
	deleteAllResources := true
	client.AgentEngines.Delete(ctx, reasoningEngine.Name, &deleteAllResources, nil)
}

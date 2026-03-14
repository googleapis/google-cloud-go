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
	"fmt"
	"testing"
	"time"

	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"
	"google.golang.org/genai"
)

func TestAgentEngines(t *testing.T) {
	if *mode != apiMode {
		t.Skipf("Skipping %s. We only tun these in the api mode.", t.Name())
	}

	t.Run("Create", func(tt *testing.T) {
		client := newTestClient(tt)
		l := client.AgentEngines.apiClient.ClientConfig().Location
		p := client.AgentEngines.apiClient.ClientConfig().Project
		model := fmt.Sprintf("projects/%s/locations/%s/publishers/google/models/gemini-2.0-flash-001", p, l)
		embeddingModel := fmt.Sprintf("projects/%s/locations/%s/publishers/google/models/text-embedding-005", p, l)
		request := &CreateAgentEngineConfig{
			ContextSpec: &ReasoningEngineContextSpec{
				MemoryBankConfig: &ReasoningEngineContextSpecMemoryBankConfig{
					GenerationConfig: &ReasoningEngineContextSpecMemoryBankConfigGenerationConfig{
						Model: model,
					},
					SimilaritySearchConfig: &ReasoningEngineContextSpecMemoryBankConfigSimilaritySearchConfig{
						EmbeddingModel: embeddingModel,
					},
					TTLConfig: &ReasoningEngineContextSpecMemoryBankConfigTTLConfig{
						DefaultTTL: 120 * time.Second,
					},
					CustomizationConfigs: []*MemoryBankCustomizationConfig{{
						MemoryTopics: []*MemoryBankCustomizationConfigMemoryTopic{{
							ManagedMemoryTopic: &MemoryBankCustomizationConfigMemoryTopicManagedMemoryTopic{
								ManagedTopicEnum: ManagedTopicEnumUserPreferences,
							},
						}},
						GenerateMemoriesExamples: []*MemoryBankCustomizationConfigGenerateMemoriesExample{{
							ConversationSource: &MemoryBankCustomizationConfigGenerateMemoriesExampleConversationSource{
								Events: []*MemoryBankCustomizationConfigGenerateMemoriesExampleConversationSourceEvent{{
									Content: &genai.Content{
										Role: "user",
										Parts: []*genai.Part{{
											Text: "Hello",
										}},
									},
								}},
							},
							GeneratedMemories: []*MemoryBankCustomizationConfigGenerateMemoriesExampleGeneratedMemory{{
								Fact: "I like to say hello.",
								Topics: []*MemoryTopicID{{
									ManagedMemoryTopic: ManagedTopicEnumUserPreferences,
								}},
							}},
						}},
						EnableThirdPersonMemories: genai.Ptr(true),
					}},
				},
			},
		}
		re := createAgentEngineAndWait(t, tt, client, request)
		if got, want := re.DisplayName, request.DisplayName; got != want {
			tt.Errorf("create() returned DisplayName %v, want %v", got, want)
		}
		if got, want := re.Description, request.Description; got != want {
			tt.Errorf("create() returned Description %v, want %v", got, want)
		}
	})

	t.Run("Delete", func(tt *testing.T) {
		ctx := tt.Context()
		client := newTestClient(t)
		re := createAgentEngineAndWait(t, tt, client, nil)
		deleteOp, err := client.AgentEngines.delete(t.Context(), re.Name, nil, nil)
		if err != nil {
			tt.Fatalf("delete() failed unexpectedly: %v", err)
		}
		operation := func() (*AgentEngineOperation, error) {
			return client.AgentEngines.getAgentOperation(tt.Context(), deleteOp.Name, nil)
		}
		waitForOperation(t, operation)
		got, err := client.AgentEngines.get(ctx, re.Name, nil)
		if err == nil {
			t.Errorf("delete() didn't remove the reasoning engine, want error(NOT_FOUND), got: %v", got)
		}
	})

	t.Run("Get", func(tt *testing.T) {
		ctx := tt.Context()
		client := newTestClient(tt)
		want := &ReasoningEngine{
			DisplayName: tt.Name(),
			Description: "You can remove this agent engine if it is older than 10 minutes. It must be an orphan AE.",
		}
		config := &CreateAgentEngineConfig{
			DisplayName: want.DisplayName,
			Description: want.Description,
		}
		re := createAgentEngineAndWait(t, tt, client, config)
		got, err := client.AgentEngines.get(ctx, re.Name, nil)
		if err != nil {
			tt.Errorf("get() failed unexpectedly: %v", err)
		}
		if diff := cmp.Diff(got, want, cmpopts.IgnoreFields(ReasoningEngine{}, "CreateTime", "Spec", "UpdateTime", "Name")); diff != "" {
			tt.Errorf("create() and get() had diff (-got +want): %v", diff)
		}

	})

	t.Run("List", func(tt *testing.T) {
		ctx := tt.Context()
		client := newTestClient(tt)
		createAgentEngineAndWait(t, tt, client, nil)
		list, err := client.AgentEngines.list(ctx, &ListAgentEngineConfig{PageSize: 2})
		if err != nil {
			tt.Fatalf("list() failed unexpectedly: %v", err)
		}
		if len(list.ReasoningEngines) == 0 {
			tt.Errorf("list(), want !0 but got %v", len(list.ReasoningEngines))
		}
	})

	t.Run("Update", func(tt *testing.T) {
		ctx := tt.Context()
		client := newTestClient(tt)
		re := createAgentEngineAndWait(t, tt, client, nil)
		want := fmt.Sprintf("Updated(%s)", re.DisplayName)
		op, err := client.AgentEngines.update(ctx, re.Name, &UpdateAgentEngineConfig{
			DisplayName: want,
		})
		if err != nil {
			tt.Fatalf("update() failed unexpectedly: %v", err)
		}
		operation := func() (*AgentEngineOperation, error) {
			return client.AgentEngines.getAgentOperation(tt.Context(), op.Name, nil)
		}
		updated := waitForOperation(tt, operation).Response
		if got := updated.DisplayName; got != want {
			tt.Errorf("update() returned DisplayName %v, want %v", got, want)
		}

	})
}

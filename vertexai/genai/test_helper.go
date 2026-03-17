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
	"flag"
	"reflect"
	"strings"
	"testing"
	"time"

	"cloud.google.com/go/vertexai/genai/types"
	"google.golang.org/genai"
)

const (
	apiMode  = "api"  // API mode runs the tests in any environment where the tests can hit the actual service.
	unitMode = "unit" // Unit mode runs the test in the github actions using the mocked service (this is the default).
)

var mode = flag.String("mode", unitMode, "Test mode")

func newTestClient(t testing.TB) *Client {
	t.Helper()
	client, err := NewGenAIClient(t.Context(), &genai.ClientConfig{
		Backend: genai.BackendVertexAI,
	})
	if err != nil {
		t.Fatal(err)
	}
	return client
}

func getResourceNameFromOperation(operationName string) string {
	i := strings.Index(operationName, "/operations/")
	return operationName[:i]
}

func createAgentEngineAndWait(t testing.TB, tt testing.TB, client *Client, config *types.CreateAgentEngineConfig) *types.ReasoningEngine {
	tt.Helper()
	if config == nil {
		config = &types.CreateAgentEngineConfig{}
	}
	if config.DisplayName == "" {
		config.DisplayName = tt.Name()
	}
	if config.Description == "" {
		config.Description = "You can remove this agent engine if it is older than 10 minutes. It must be an orphan AE."
	}
	createOp, err := client.AgentEngines.create(tt.Context(), config)
	if err != nil {
		tt.Fatalf("create() failed unexpectedly: %v", err)
	}
	reasoningEngineName := getResourceNameFromOperation(createOp.Name)
	tt.Cleanup(cleanupAgentEngine(t, client, reasoningEngineName))
	operation := func() (*types.AgentEngineOperation, error) {
		return client.AgentEngines.getAgentOperation(tt.Context(), createOp.Name, nil)
	}
	createOp = waitForOperation(t, operation)
	return createOp.Response
}

func cleanupAgentEngine(t testing.TB, client *Client, name string) func() {
	return func() {
		t.Logf("Cleaning up AgentEngine: %s", name)
		deleteOp, err := client.AgentEngines.delete(t.Context(), name, genai.Ptr(true), nil)
		if err != nil {
			t.Logf("cleanup() failed, err: %v", err)
		} else {
			operation := func() (*types.AgentEngineOperation, error) {
				return client.AgentEngines.getAgentOperation(t.Context(), deleteOp.Name, nil)
			}
			waitForOperation(t, operation)
		}
	}
}

func createAgentEngineMemoryAndWait(tt testing.TB, client *Client, re *types.ReasoningEngine, m *types.Memory) *types.Memory {
	tt.Helper()
	config := &types.AgentEngineMemoryConfig{
		DisplayName: m.DisplayName,
		Description: m.Description,
		Metadata:    m.Metadata,
	}
	createOp, err := client.AgentEngines.Memories.create(tt.Context(), re.Name, m.Fact, m.Scope, config)
	if err != nil {
		tt.Fatalf("create() failed unexpectedly: %v", err)
	}
	if !createOp.Done {
		operation := func() (*types.AgentEngineMemoryOperation, error) {
			return client.AgentEngines.Memories.getMemoryOperation(tt.Context(), createOp.Name, nil)
		}
		waitForOperation(tt, operation)
	}
	return createOp.Response
}

func createAgentEngineSandboxesAndWait(
	tt testing.TB,
	client *Client,
	re *types.ReasoningEngine,
	spec *types.SandboxEnvironmentSpec,
	config *types.CreateAgentEngineSandboxConfig,
) *types.SandboxEnvironment {
	tt.Helper()
	createOp, err := client.AgentEngines.Sandboxes.create(tt.Context(), re.Name, spec, config)
	if err != nil {
		tt.Fatalf("create() failed unexpectedly: %v", err)
	}
	if !createOp.Done {
		operation := func() (*types.AgentEngineSandboxOperation, error) {
			return client.AgentEngines.Sandboxes.getSandboxOperation(tt.Context(), createOp.Name, nil)
		}
		createOp = waitForOperation(tt, operation)
	}
	return createOp.Response
}

func createAgentEngineSessionEvent(tt testing.TB, client *Client, s *types.Session, se *types.SessionEvent) {
	tt.Helper()
	config := &types.AppendAgentEngineSessionEventConfig{
		Content: se.Content,
	}
	_, err := client.AgentEngines.Sessions.Events.Append(tt.Context(), s.Name, se.Author, se.InvocationID, se.Timestamp, config)
	if err != nil {
		tt.Fatalf("append() failed unexpectedly: %v", err)
	}
}

func createAgentEngineSessionAndWait(tt testing.TB, client *Client, re *types.ReasoningEngine, s *types.Session) *types.Session {
	tt.Helper()
	config := &types.CreateAgentEngineSessionConfig{
		DisplayName:  s.DisplayName,
		SessionState: s.SessionState,
		TTL:          s.TTL,
		Labels:       s.Labels,
	}
	createOp, err := client.AgentEngines.Sessions.create(tt.Context(), re.Name, s.UserID, config)
	if err != nil {
		tt.Fatalf("create() failed unexpectedly: %v", err)
	}
	if !createOp.Done {
		operation := func() (*types.AgentEngineSessionOperation, error) {
			return client.AgentEngines.Sessions.getSessionOperation(tt.Context(), createOp.Name, nil)
		}
		createOp = waitForOperation(tt, operation)
	}
	return createOp.Response
}

func waitForOperation[T any](tb testing.TB, wait func() (T, error)) T {
	tb.Helper()
	for {
		tb.Logf("Waiting for operation to complete for [%s]\n", tb.Name())
		time.Sleep(5 * time.Second)
		op, err := wait()
		if err != nil {
			tb.Fatal(err)
		}
		if isDone(op) {
			return op
		}
	}
}

func isDone(o any) bool {
	v := reflect.Indirect(reflect.ValueOf(o))
	return v.FieldByName("Done").Bool()
}

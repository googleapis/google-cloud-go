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
	"time"

	"cloud.google.com/go/agentplatform/types"
	"google.golang.org/genai"
)

func TestReplays_AgentEngine_SandboxTemplates(t *testing.T) {
	if *mode != replayMode {
		t.Skipf("unsupported mode: %s", *mode)
	}

	t.Run("ae_sandbox_templates_default_create/test_sandbox_templates_default_create", func(tt *testing.T) {
		client, _ := newTestClientWithReplay(tt, tt.Name())

		name := "projects/802583348448/locations/us-central1/reasoningEngines/6130241318758121472"
		config := &types.CreateSandboxEnvironmentTemplateConfig{
			DefaultContainerEnvironment: &types.SandboxEnvironmentTemplateDefaultContainerEnvironment{
				DefaultContainerCategory: types.DefaultContainerCategoryComputerUse,
			},
			EgressControlConfig: &types.SandboxEnvironmentTemplateEgressControlConfig{
				InternetAccess: genai.Ptr(true),
			},
		}
		createOp, err := client.AgentEngines.Sandboxes.Templates.create(
			tt.Context(), name, "Test Sandbox Template 1", config)
		if err != nil {
			tt.Fatalf("create() failed unexpectedly: %v", err)
		}
		if createOp.Name == "" {
			tt.Errorf("create(), want non-empty name, got empty: %v", createOp)
		}
	})

	t.Run("ae_sandbox_templates_get/test_sandbox_templates_get", func(tt *testing.T) {
		client, _ := newTestClientWithReplay(tt, tt.Name())

		name := "projects/254005681254/locations/us-central1/reasoningEngines/208148546254274560/sandboxEnvironmentTemplates/4632233691727265792"
		template, err := client.AgentEngines.Sandboxes.Templates.get(tt.Context(), name, nil)
		if err != nil {
			tt.Fatalf("get() failed unexpectedly: %v", err)
		}
		if template.Name != name {
			tt.Errorf("get() name = %q, want %q", template.Name, name)
		}
	})

	t.Run("ae_sandbox_templates_list/test_sandbox_templates_list", func(tt *testing.T) {
		client, _ := newTestClientWithReplay(tt, tt.Name())

		name := "projects/254005681254/locations/us-central1/reasoningEngines/208148546254274560"
		response, err := client.AgentEngines.Sandboxes.Templates.list(tt.Context(), name, nil)
		if err != nil {
			tt.Fatalf("list() failed unexpectedly: %v", err)
		}
		if len(response.SandboxEnvironmentTemplates) == 0 {
			tt.Errorf("list(), want non-empty templates, got empty")
		}
	})

	t.Run("ae_sandbox_templates_delete/test_sandbox_templates_delete", func(tt *testing.T) {
		client, _ := newTestClientWithReplay(tt, tt.Name())

		name := "projects/254005681254/locations/us-central1/reasoningEngines/208148546254274560/sandboxEnvironmentTemplates/4632233691727265792"
		deleteOp, err := client.AgentEngines.Sandboxes.Templates.delete(tt.Context(), name, nil)
		if err != nil {
			tt.Fatalf("delete() failed unexpectedly: %v", err)
		}
		if deleteOp == nil {
			tt.Errorf("delete(), want non-nil operation, got nil")
		}
	})

	t.Run("ae_sandbox_templates_get_sandbox_template_operation/test_get_sandbox_template_operation", func(tt *testing.T) {
		client, _ := newTestClientWithReplay(tt, tt.Name())

		operationName := "projects/254005681254/locations/us-central1/operations/7252775414349692928"
		op, err := client.AgentEngines.Sandboxes.Templates.GetSandboxEnvironmentTemplateOperation(
			tt.Context(), operationName, nil)
		if err != nil {
			tt.Fatalf("GetSandboxEnvironmentTemplateOperation() failed unexpectedly: %v", err)
		}
		if op.Name != operationName {
			tt.Errorf("GetSandboxEnvironmentTemplateOperation() name = %q, want %q", op.Name, operationName)
		}
	})
}

func TestReplays_AgentEngine_SandboxSnapshots(t *testing.T) {
	if *mode != replayMode {
		t.Skipf("unsupported mode: %s", *mode)
	}

	t.Run("ae_sandbox_snapshots_create/test_create_sandbox_snapshot", func(tt *testing.T) {
		client, _ := newTestClientWithReplay(tt, tt.Name())

		source := "projects/802583348448/locations/us-central1/reasoningEngines/6130241318758121472/sandboxEnvironments/525190525100228608"
		config := &types.CreateAgentEngineSandboxSnapshotConfig{
			DisplayName: "test_snapshot",
			Owner:       "test_owner",
			TTL:         3600 * time.Second,
		}
		createOp, err := client.AgentEngines.Sandboxes.Snapshots.create(tt.Context(), source, config)
		if err != nil {
			tt.Fatalf("create() failed unexpectedly: %v", err)
		}
		if createOp.Name == "" {
			tt.Errorf("create(), want non-empty name, got empty: %v", createOp)
		}
	})

	t.Run("ae_sandbox_snapshots_get/test_get_sandbox_snapshot", func(tt *testing.T) {
		client, _ := newTestClientWithReplay(tt, tt.Name())

		name := "projects/802583348448/locations/us-central1/reasoningEngines/6130241318758121472/sandboxEnvironmentSnapshots/2433069698686910464"
		snapshot, err := client.AgentEngines.Sandboxes.Snapshots.get(tt.Context(), name, nil)
		if err != nil {
			tt.Fatalf("get() failed unexpectedly: %v", err)
		}
		if snapshot.Name != name {
			tt.Errorf("get() name = %q, want %q", snapshot.Name, name)
		}
	})

	t.Run("ae_sandbox_snapshots_list/test_list_sandbox_snapshots", func(tt *testing.T) {
		client, _ := newTestClientWithReplay(tt, tt.Name())

		name := "projects/802583348448/locations/us-central1/reasoningEngines/6130241318758121472"
		response, err := client.AgentEngines.Sandboxes.Snapshots.list(tt.Context(), name, nil)
		if err != nil {
			tt.Fatalf("list() failed unexpectedly: %v", err)
		}
		if len(response.SandboxEnvironmentSnapshots) == 0 {
			tt.Errorf("list(), want non-empty snapshots, got empty")
		}
	})

	t.Run("ae_sandbox_snapshots_delete/test_delete_sandbox_snapshot", func(tt *testing.T) {
		client, _ := newTestClientWithReplay(tt, tt.Name())

		name := "projects/802583348448/locations/us-central1/reasoningEngines/6130241318758121472/sandboxEnvironmentSnapshots/421086565159141376"
		deleteOp, err := client.AgentEngines.Sandboxes.Snapshots.delete(tt.Context(), name, nil)
		if err != nil {
			tt.Fatalf("delete() failed unexpectedly: %v", err)
		}
		if deleteOp == nil {
			tt.Errorf("delete(), want non-nil operation, got nil")
		}
	})
}

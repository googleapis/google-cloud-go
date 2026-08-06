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
	"fmt"
	"time"

	"cloud.google.com/go/agentplatform"
	"cloud.google.com/go/agentplatform/types"
	"google.golang.org/genai"
)

// Example_agentEngineSandboxTemplatesAndSnapshots demonstrates using the Go SDK
// to perform operations on Agent Engine sandbox environment templates and
// snapshots.
//
// It reuses the createClient helper, projectID, and location declared in
// example_test.go.
func Example_agentEngineSandboxTemplatesAndSnapshots() {
	ctx := context.Background()
	client := createClient(ctx)

	// Create a temporary Agent Engine to own the sandboxes.
	createOp, err := client.AgentEngines.Create(
		ctx, &types.CreateAgentEngineConfig{DisplayName: "sandbox-example"})
	if err != nil {
		panic(fmt.Sprintf("AgentEngines.Create() failed: %+v", err))
	}
	for !createOp.Done {
		time.Sleep(10 * time.Second)
		createOp, err = client.AgentEngines.GetAgentOperation(ctx, createOp.Name, nil)
		if err != nil {
			panic(fmt.Sprintf("GetAgentOperation() failed: %+v", err))
		}
	}
	agentEngineName := createOp.Response.Name
	fmt.Println("Created Agent Engine:", agentEngineName)

	// Clean up the Agent Engine (and all child resources) when finished.
	deleteAllResources := true
	defer client.AgentEngines.Delete(ctx, agentEngineName, &deleteAllResources, nil)

	sandboxTemplatesExample(ctx, client, agentEngineName)
	sandboxSnapshotsExample(ctx, client, agentEngineName)
}

// sandboxTemplatesExample demonstrates the sandbox templates submodule.
func sandboxTemplatesExample(ctx context.Context, client *agentplatform.Client, agentEngineName string) {
	// 1. Create a sandbox environment template.
	templateOp, err := client.AgentEngines.Sandboxes.Templates.Create(
		ctx, agentEngineName, "Example Sandbox Template", computerUseTemplateConfig())
	if err != nil {
		panic(fmt.Sprintf("Templates.Create() failed: %+v", err))
	}
	for !templateOp.Done {
		time.Sleep(10 * time.Second)
		templateOp, err = client.AgentEngines.Sandboxes.Templates.GetSandboxEnvironmentTemplateOperation(
			ctx, templateOp.Name, nil)
		if err != nil {
			panic(fmt.Sprintf("GetSandboxEnvironmentTemplateOperation() failed: %+v", err))
		}
	}
	templateName := templateOp.Response.Name
	fmt.Println("Created Sandbox Template:", templateName)

	// 2. Get the template.
	template, err := client.AgentEngines.Sandboxes.Templates.Get(ctx, templateName, nil)
	if err != nil {
		panic(fmt.Sprintf("Templates.Get() failed: %+v", err))
	}
	fmt.Println("Template display name:", template.DisplayName)

	// 3. List templates.
	listResp, err := client.AgentEngines.Sandboxes.Templates.List(ctx, agentEngineName, nil)
	if err != nil {
		panic(fmt.Sprintf("Templates.List() failed: %+v", err))
	}
	fmt.Println("Templates found:", len(listResp.SandboxEnvironmentTemplates))

	// 4. Delete the template.
	if _, err := client.AgentEngines.Sandboxes.Templates.Delete(ctx, templateName, nil); err != nil {
		panic(fmt.Sprintf("Templates.Delete() failed: %+v", err))
	}
	fmt.Println("Deleted Sandbox Template.")
}

// sandboxSnapshotsExample demonstrates the sandbox snapshots submodule.
//
// Snapshots can only be taken of template-based (e.g. computer-use) sandboxes,
// so it first creates a computer-use template, then a sandbox from that
// template, snapshots the running sandbox, and finally restores a new sandbox
// from the snapshot.
func sandboxSnapshotsExample(ctx context.Context, client *agentplatform.Client, agentEngineName string) {
	// Create a computer-use template to base the source sandbox on.
	templateOp, err := client.AgentEngines.Sandboxes.Templates.Create(
		ctx, agentEngineName, "Snapshot Source Template", computerUseTemplateConfig())
	if err != nil {
		panic(fmt.Sprintf("Templates.Create() failed: %+v", err))
	}
	for !templateOp.Done {
		time.Sleep(10 * time.Second)
		templateOp, err = client.AgentEngines.Sandboxes.Templates.GetSandboxEnvironmentTemplateOperation(
			ctx, templateOp.Name, nil)
		if err != nil {
			panic(fmt.Sprintf("GetSandboxEnvironmentTemplateOperation() failed: %+v", err))
		}
	}
	templateName := templateOp.Response.Name
	fmt.Println("Created Sandbox Template:", templateName)
	defer client.AgentEngines.Sandboxes.Templates.Delete(ctx, templateName, nil)

	// Create a sandbox from the template so it has a snapshottable state.
	sandboxOp, err := client.AgentEngines.Sandboxes.Create(
		ctx,
		agentEngineName,
		nil, // No spec: the sandbox is created from the template.
		&types.CreateAgentEngineSandboxConfig{
			DisplayName:                "Snapshot Source Sandbox",
			SandboxEnvironmentTemplate: templateName,
		},
	)
	if err != nil {
		panic(fmt.Sprintf("Sandboxes.Create() failed: %+v", err))
	}
	for !sandboxOp.Done {
		time.Sleep(10 * time.Second)
		sandboxOp, err = client.AgentEngines.Sandboxes.GetSandboxOperation(ctx, sandboxOp.Name, nil)
		if err != nil {
			panic(fmt.Sprintf("GetSandboxOperation() failed: %+v", err))
		}
	}
	sandboxName := sandboxOp.Response.Name
	fmt.Println("Created source Sandbox:", sandboxName)
	defer client.AgentEngines.Sandboxes.Delete(ctx, sandboxName, nil)

	// 1. Create a snapshot of the running sandbox.
	snapshotOp, err := client.AgentEngines.Sandboxes.Snapshots.Create(
		ctx,
		sandboxName,
		&types.CreateAgentEngineSandboxSnapshotConfig{
			DisplayName: "Example Sandbox Snapshot",
			TTL:         3600 * time.Second,
		},
	)
	if err != nil {
		panic(fmt.Sprintf("Snapshots.Create() failed: %+v", err))
	}
	for !snapshotOp.Done {
		time.Sleep(10 * time.Second)
		snapshotOp, err = client.AgentEngines.Sandboxes.Snapshots.GetSandboxSnapshotOperation(
			ctx, snapshotOp.Name, nil)
		if err != nil {
			panic(fmt.Sprintf("GetSandboxSnapshotOperation() failed: %+v", err))
		}
	}
	snapshotName := snapshotOp.Response.Name
	fmt.Println("Created Sandbox Snapshot:", snapshotName)

	// 2. Get the snapshot.
	snapshot, err := client.AgentEngines.Sandboxes.Snapshots.Get(ctx, snapshotName, nil)
	if err != nil {
		panic(fmt.Sprintf("Snapshots.Get() failed: %+v", err))
	}
	fmt.Println("Snapshot Name:", snapshot.Name)

	// 3. List snapshots.
	listResp, err := client.AgentEngines.Sandboxes.Snapshots.List(ctx, agentEngineName, nil)
	if err != nil {
		panic(fmt.Sprintf("Snapshots.List() failed: %+v", err))
	}
	fmt.Println("Snapshots found:", len(listResp.SandboxEnvironmentSnapshots))

	// 4. Restore a new sandbox from the snapshot.
	restoreOp, err := client.AgentEngines.Sandboxes.Create(
		ctx,
		agentEngineName,
		nil, // No spec: the sandbox is restored from the snapshot.
		&types.CreateAgentEngineSandboxConfig{
			DisplayName:                "Restored Sandbox",
			SandboxEnvironmentSnapshot: snapshotName,
		},
	)
	if err != nil {
		panic(fmt.Sprintf("Sandboxes.Create() from snapshot failed: %+v", err))
	}
	for !restoreOp.Done {
		time.Sleep(10 * time.Second)
		restoreOp, err = client.AgentEngines.Sandboxes.GetSandboxOperation(ctx, restoreOp.Name, nil)
		if err != nil {
			panic(fmt.Sprintf("GetSandboxOperation() failed: %+v", err))
		}
	}
	restoredSandboxName := restoreOp.Response.Name
	fmt.Println("Restored Sandbox:", restoredSandboxName)
	defer client.AgentEngines.Sandboxes.Delete(ctx, restoredSandboxName, nil)

	// 5. Delete the snapshot.
	if _, err := client.AgentEngines.Sandboxes.Snapshots.Delete(ctx, snapshotName, nil); err != nil {
		panic(fmt.Sprintf("Snapshots.Delete() failed: %+v", err))
	}
	fmt.Println("Deleted Sandbox Snapshot.")
}

// computerUseTemplateConfig returns the config for a computer-use sandbox
// template with internet access.
func computerUseTemplateConfig() *types.CreateSandboxEnvironmentTemplateConfig {
	return &types.CreateSandboxEnvironmentTemplateConfig{
		DefaultContainerEnvironment: &types.SandboxEnvironmentTemplateDefaultContainerEnvironment{
			DefaultContainerCategory: types.DefaultContainerCategoryComputerUse,
		},
		EgressControlConfig: &types.SandboxEnvironmentTemplateEgressControlConfig{
			InternetAccess: genai.Ptr(true),
		},
	}
}

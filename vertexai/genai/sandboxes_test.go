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
)

func waitForSandboxesOperation(tt testing.TB, name string, done bool, c *Client) *AgentEngineSandboxOperation {
	tt.Helper()
	var res *AgentEngineSandboxOperation
	for !done {
		tt.Logf("Waiting for operation to complete: [%s]\n", name)
		time.Sleep(5 * time.Second)
		op, err := c.AgentEngines.Sandboxes.getSandboxOperation(tt.Context(), name, nil)
		if err != nil {
			tt.Fatalf("getAgentOperation failed, err: %v", err)
		}
		done = op.Done
		res = op
	}
	return res
}

func createAgentEngineSandboxesAndWait(tt testing.TB, client *Client, name string, spec *SandboxEnvironmentSpec, config *CreateAgentEngineSandboxConfig) *SandboxEnvironment {
	tt.Helper()
	createOp, err := client.AgentEngines.Sandboxes.create(tt.Context(), name, spec, config)
	if err != nil {
		tt.Fatalf("create() failed unexpectedly: %v", err)
	}
	if !createOp.Done {
		createOp = waitForSandboxesOperation(tt, createOp.Name, createOp.Done, client)
	}
	return createOp.Response
}

func TestAgentEngineSandboxes(t *testing.T) {
	if *mode != apiMode {
		t.Skipf("Skipping %s. We only tun these in the api mode.", t.Name())
	}

	t.Run("Create", func(tt *testing.T) {
		client := newTestClient(tt)
		re := createAgentEngineAndWait(t, tt, client, nil)

		want := &SandboxEnvironment{
			Spec: &SandboxEnvironmentSpec{
				CodeExecutionEnvironment: &SandboxEnvironmentSpecCodeExecutionEnvironment{
					MachineConfig: MachineConfigVcpu4Ram4gib,
				},
			},
			DisplayName: tt.Name(),
			TTL:         time.Hour,
		}
		config := &CreateAgentEngineSandboxConfig{
			DisplayName: want.DisplayName,
			TTL:         want.TTL,
		}
		got := createAgentEngineSandboxesAndWait(tt, client, re.Name, want.Spec, config)
		if diff := cmp.Diff(got, want, cmpopts.IgnoreFields(SandboxEnvironment{}, "Name", "ExpireTime", "TTL")); diff != "" {
			tt.Errorf("create() had diff (-got +want): %v", diff)
		}
	})
}

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

	"cloud.google.com/go/vertexai/genai/types"
	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"
)

func TestAgentEngineSandboxes(t *testing.T) {
	if *mode != apiMode {
		t.Skipf("Skipping %s. We only tun these in the api mode.", t.Name())
	}

	t.Run("Create", func(tt *testing.T) {
		client := newTestClient(tt)
		re := createAgentEngineAndWait(t, tt, client, nil)

		want := &types.SandboxEnvironment{
			Spec: &types.SandboxEnvironmentSpec{
				CodeExecutionEnvironment: &types.SandboxEnvironmentSpecCodeExecutionEnvironment{
					MachineConfig: types.MachineConfigVcpu4Ram4gib,
				},
			},
			DisplayName: tt.Name(),
			TTL:         time.Hour,
		}
		config := &types.CreateAgentEngineSandboxConfig{
			DisplayName: want.DisplayName,
			TTL:         want.TTL,
		}
		got := createAgentEngineSandboxesAndWait(tt, client, re, want.Spec, config)
		if diff := cmp.Diff(got, want, cmpopts.IgnoreFields(types.SandboxEnvironment{}, "Name", "ExpireTime", "TTL")); diff != "" {
			tt.Errorf("create() had diff (-got +want): %v", diff)
		}
	})
}

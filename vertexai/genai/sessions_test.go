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

func waitForSessionsOperation(tt testing.TB, name string, done bool, c *Client) any {
	tt.Helper()
	var res any
	for !done {
		tt.Logf("Waiting for operation to complete: [%s]\n", name)
		time.Sleep(5 * time.Second)
		op, err := c.AgentEngines.Sessions.getSessionOperation(tt.Context(), name, nil)
		if err != nil {
			tt.Fatalf("getAgentOperation failed, err: %v", err)
		}
		done = op.Done
		res = op
	}
	return res
}

func createAgentEngineSessionAndWait(tt testing.TB, client *Client, name string, s *Session) *Session {
	tt.Helper()
	config := &CreateAgentEngineSessionConfig{
		DisplayName:  s.DisplayName,
		SessionState: s.SessionState,
		TTL:          s.TTL,
		Labels:       s.Labels,
	}
	createOp, err := client.AgentEngines.Sessions.create(tt.Context(), name, s.UserID, config)
	if err != nil {
		tt.Fatalf("create() failed unexpectedly: %v", err)
	}
	if !createOp.Done {
		createOp = waitForSessionsOperation(tt, createOp.Name, createOp.Done, client).(*AgentEngineSessionOperation)
	}
	return createOp.Response
}

func TestAgentEngineSessions(t *testing.T) {
	if *mode != apiMode {
		t.Skipf("Skipping %s. We only tun these in the api mode.", t.Name())
	}

	t.Run("Create", func(tt *testing.T) {
		client := newTestClient(tt)
		re := createAgentEngineAndWait(t, tt, client, nil)
		want := &Session{
			DisplayName:  tt.Name(),
			SessionState: map[string]any{"foo": "bar"},
			Labels:       map[string]string{"label_key": "label_value"},
			TTL:          24 * time.Hour,
			UserID:       "test-user-123",
		}
		got := createAgentEngineSessionAndWait(tt, client, re.Name, want)
		if diff := cmp.Diff(got, want, cmpopts.IgnoreFields(Session{}, "Name")); diff != "" {
			tt.Errorf("create() had diff (-got +want): %v", diff)
		}
	})
}

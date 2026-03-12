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
	"google.golang.org/genai"
)

func createAgentEngineSessionEventAndWait(tt testing.TB, client *Client, name string, se *SessionEvent) {
	tt.Helper()
	config := &AppendAgentEngineSessionEventConfig{
		Content: se.Content,
	}
	_, err := client.AgentEngines.Sessions.Events.Append(tt.Context(), name, se.Author, se.InvocationID, se.Timestamp, config)
	if err != nil {
		tt.Fatalf("append() failed unexpectedly: %v", err)
	}
}

func TestAgentEngineSessionEvents(t *testing.T) {
	if *mode != apiMode {
		t.Skipf("Skipping %s. We only tun these in the api mode.", t.Name())
	}

	t.Run("AppendAndList", func(tt *testing.T) {
		client := newTestClient(tt)
		re := createAgentEngineAndWait(t, tt, client, nil)

		session := &Session{
			DisplayName:  tt.Name(),
			SessionState: map[string]any{"foo": "bar"},
			Labels:       map[string]string{"label_key": "label_value"},
			TTL:          24 * time.Hour,
			UserID:       "test-user-123",
		}
		session = createAgentEngineSessionAndWait(tt, client, re.Name, session)
		timestamp, err := time.Parse(time.RFC3339Nano, "2026-03-10T15:30:45.0Z")
		if err != nil {
			tt.Fatalf("Error parsing time, err: %v", err)
		}
		want := &SessionEvent{
			Author:       tt.Name(),
			InvocationID: "test-invocation-id",
			Timestamp:    timestamp,
			Actions:      &EventActions{},
			Content: &genai.Content{
				Parts: []*genai.Part{{
					Text: "Hello World",
				}},
			},
		}
		createAgentEngineSessionEventAndWait(tt, client, session.Name, want)

		got, err := client.AgentEngines.Sessions.Events.list(tt.Context(), session.Name, nil)
		if err != nil {
			tt.Fatalf("list() failed unexpectedly: %v", err)
		}
		if diff := cmp.Diff(got.SessionEvents, []*SessionEvent{want}, cmpopts.IgnoreFields(SessionEvent{}, "Name")); diff != "" {
			tt.Errorf("list() had diff (-got +want): %v", diff)
		}
	})
}

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
		got := createAgentEngineSessionAndWait(tt, client, re, want)
		if diff := cmp.Diff(got, want, cmpopts.IgnoreFields(Session{}, "Name")); diff != "" {
			tt.Errorf("create() had diff (-got +want): %v", diff)
		}
	})
}

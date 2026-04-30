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

// To get the protoveneer tool:
//    go install golang.org/x/exp/protoveneer/cmd/protoveneer@latest

//go:generate protoveneer -license license.txt config.yaml ../../aiplatform/apiv1beta1/aiplatformpb

package agentplatform

import (
	"context"
	"fmt"

	"google.golang.org/genai"
)

type clientAgentEngines struct {
	AgentEngines
	Memories  *clientMemories
	Sessions  *clientSessions
	Sandboxes *Sandboxes
}

type clientMemories struct {
	Memories
	Revisions *MemoryRevisions
}

type clientSessions struct {
	Sessions
	Events *SessionEvents
}

// A Client is a Google Vertex AI client.
type Client struct {
	AgentEngines *clientAgentEngines
}

// NewClient creates a new Google Vertex AI client and configures the the GenAI components.
func NewClient(ctx context.Context, cc *genai.ClientConfig) (*Client, error) {
	config := genai.ClientConfig{Backend: genai.BackendVertexAI}
	if cc != nil {
		config = *cc
	}
	if config.Backend == genai.BackendUnspecified {
		config.Backend = genai.BackendVertexAI
	}
	ac, err := genai.NewInternalAPIClient(ctx, &config)
	if err != nil {
		return nil, err
	}
	if ac.ClientConfig().Backend != genai.BackendVertexAI {
		return nil, fmt.Errorf("only Vertex AI backend is supported")
	}
	return &Client{
		AgentEngines: &clientAgentEngines{
			AgentEngines: AgentEngines{apiClient: ac},
			Memories: &clientMemories{
				Memories:  Memories{apiClient: ac},
				Revisions: &MemoryRevisions{apiClient: ac},
			},
			Sessions: &clientSessions{
				Sessions: Sessions{apiClient: ac},
				Events:   &SessionEvents{apiClient: ac},
			},
			Sandboxes: &Sandboxes{apiClient: ac},
		},
	}, nil
}

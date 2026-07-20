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
	"context"
	"os"
	"testing"

	"google.golang.org/genai"
)

func TestNewClient(t *testing.T) {
	ctx := context.Background()
	projectKey := "GOOGLE_CLOUD_PROJECT"
	locationKey := "GOOGLE_CLOUD_LOCATION"
	originalProjectValue, _ := os.LookupEnv(projectKey)
	originalLocationValue, _ := os.LookupEnv(locationKey)
	os.Setenv(projectKey, "test-gcp-project")
	os.Setenv(locationKey, "us-central1")

	t.Cleanup(func() {
		os.Setenv(locationKey, originalLocationValue)
		os.Setenv(projectKey, originalProjectValue)
	})

	for _, test := range []struct {
		name string
		cc   *genai.ClientConfig
	}{
		{name: "nil config", cc: nil},
		{name: "empty config", cc: &genai.ClientConfig{}},
	} {
		t.Run(test.name, func(t *testing.T) {
			client, err := NewClient(ctx, test.cc)
			if err != nil {
				t.Fatalf("NewClient() failed unexpectedly, err: %v", err)
			}
			if client == nil {
				t.Error("client must not be nil")
			}
		})
	}
}

func TestNewClientErrors(t *testing.T) {
	ctx := context.Background()
	for _, test := range []struct {
		name string
		cc   *genai.ClientConfig
	}{
		{name: "gemini backend", cc: &genai.ClientConfig{Backend: genai.BackendGeminiAPI}},
	} {
		t.Run(test.name, func(t *testing.T) {
			if _, err := NewClient(ctx, test.cc); err == nil {
				t.Error("wants error, but got nil")
			}
		})
	}
}

// Copyright 2026 Google LLC
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//	http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package bidi

import (
	"testing"

	btpb "cloud.google.com/go/bigtable/apiv2/bigtablepb"
)

func TestShouldUseSession(t *testing.T) {
	cm := &ConfigManager{}

	t.Run("Always false on nil config", func(t *testing.T) {
		if cm.ShouldUseSession(nil) {
			t.Error("Expected false on nil config")
		}
	})

	t.Run("Always false on nil SessionConfiguration", func(t *testing.T) {
		config := &btpb.ClientConfiguration{}
		if cm.ShouldUseSession(config) {
			t.Error("Expected false on nil SessionConfiguration")
		}
	})

	t.Run("Probabilistic routing", func(t *testing.T) {
		config := &btpb.ClientConfiguration{
			SessionConfiguration: &btpb.SessionClientConfiguration{
				SessionLoad: 0.1, // 10%
			},
		}

		iterations := 10000
		sessionCount := 0
		for i := 0; i < iterations; i++ {
			if cm.ShouldUseSession(config) {
				sessionCount++
			}
		}

		expected := int(float32(iterations) * config.SessionConfiguration.SessionLoad)
		tolerance := 200 // Allow some variance

		if sessionCount < expected-tolerance || sessionCount > expected+tolerance {
			t.Errorf("Expected approximately %d session calls, got %d", expected, sessionCount)
		}
	})
}

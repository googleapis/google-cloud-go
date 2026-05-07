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

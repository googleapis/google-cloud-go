package bidi

import (
	"context"
	"math/rand"
	"sync"
	"time"

	btpb "cloud.google.com/go/bigtable/apiv2/bigtablepb"
)

// ConfigManager handles fetching and analyzing client configuration.
type ConfigManager struct {
	client        btpb.BigtableClient
	mu            sync.RWMutex
	currentConfig *btpb.ClientConfiguration
	cancel        context.CancelFunc
}

// NewConfigManager creates a new ConfigManager.
func NewConfigManager(client btpb.BigtableClient) *ConfigManager {
	return &ConfigManager{client: client}
}

// GetClientConfiguration fetches the client configuration from the server.
func (m *ConfigManager) GetClientConfiguration(ctx context.Context, instanceName string, appProfileId string) (*btpb.ClientConfiguration, error) {
	req := &btpb.GetClientConfigurationRequest{
		InstanceName: instanceName,
		AppProfileId: appProfileId,
	}
	return m.client.GetClientConfiguration(ctx, req)
}

// ShouldUseSession returns true if the request should be routed to session protocol based on configuration.
func (m *ConfigManager) ShouldUseSession(config *btpb.ClientConfiguration) bool {
	if config == nil || config.SessionConfiguration == nil {
		return false
	}
	// Implement routing logic based on SessionLoad fraction.
	// Returns true with probability equal to SessionLoad.
	return rand.Float32() < config.SessionConfiguration.SessionLoad
}
func (m *ConfigManager) GetConfig() *btpb.ClientConfiguration {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.currentConfig
}

func (m *ConfigManager) StartPolling(ctx context.Context, instanceName, appProfileId string) {
	pollCtx, cancel := context.WithCancel(ctx)
	m.mu.Lock()
	m.cancel = cancel
	m.mu.Unlock()

	go func() {
		ticker := time.NewTicker(1 * time.Minute) // Default interval
		defer ticker.Stop()

		for {
			config, err := m.GetClientConfiguration(pollCtx, instanceName, appProfileId)
			if err == nil && config != nil {
				m.mu.Lock()
				m.currentConfig = config
				m.mu.Unlock()

				// Extract PollingInterval if available
				var interval time.Duration
				if config.Polling != nil {
					switch p := config.Polling.(type) {
					case *btpb.ClientConfiguration_StopPolling:
						if p.StopPolling {
							return
						}
					case *btpb.ClientConfiguration_PollingInterval:
						interval = time.Duration(p.PollingInterval.Seconds) * time.Second
					case *btpb.ClientConfiguration_PollingConfiguration_:
						if p.PollingConfiguration != nil && p.PollingConfiguration.PollingInterval != nil {
							interval = time.Duration(p.PollingConfiguration.PollingInterval.Seconds) * time.Second
						}
					}
				}
				if interval > 0 {
					ticker.Reset(interval)
				}
			}

			select {
			case <-ticker.C:
			case <-pollCtx.Done():
				return
			}
		}
	}()
}

func (m *ConfigManager) Close() {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.cancel != nil {
		m.cancel()
	}
}

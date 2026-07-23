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
	"context"
	"fmt"
	"math/rand"
	"net/url"
	"sync"
	"time"

	btpb "cloud.google.com/go/bigtable/apiv2/bigtablepb"
	"google.golang.org/grpc/metadata"
)

// ConfigManager handles fetching and applying client configuration.
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

	requestParamsMD := metadata.Pairs(requestParamsHeader,
		fmt.Sprintf("name=%s&app_profile_id=%s", url.QueryEscape(instanceName), url.QueryEscape(appProfileId)))

	originalContextMd, _ := metadata.FromOutgoingContext(ctx)
	ctx = metadata.NewOutgoingContext(ctx, metadata.Join(originalContextMd, requestParamsMD))

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

		ticker := time.NewTicker(1 * time.Hour) // Default interval to poll at beginning
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

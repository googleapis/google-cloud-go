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

package internal

import (
	"context"
	"sync"
	"testing"
	"time"

	bigtablepb "cloud.google.com/go/bigtable/apiv2/bigtablepb"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/durationpb"
)

type mockBigtableClient struct {
	bigtablepb.BigtableClient
	getConfigFunc func(ctx context.Context, req *bigtablepb.GetClientConfigurationRequest) (*bigtablepb.ClientConfiguration, error)
}

func (m *mockBigtableClient) GetClientConfiguration(ctx context.Context, req *bigtablepb.GetClientConfigurationRequest, opts ...grpc.CallOption) (*bigtablepb.ClientConfiguration, error) {
	if m.getConfigFunc != nil {
		return m.getConfigFunc(ctx, req)
	}
	return nil, status.Error(codes.Unimplemented, "not implemented")
}

func TestNewClientConfigurationManager(t *testing.T) {
	client := &mockBigtableClient{}
	manager := NewClientConfigurationManager(client, "instance", "profile", nil, nil)

	if manager == nil {
		t.Fatal("Expected manager to be non-nil")
	}

	cfg := manager.getConfig()
	if cfg.Polling.MaxRpcRetryCount != 5 {
		t.Errorf("Expected MaxRpcRetryCount to be 5, got %d", cfg.Polling.MaxRpcRetryCount)
	}
}

func TestManagerPoll_Success(t *testing.T) {
	client := &mockBigtableClient{}
	expectedCfg := &bigtablepb.ClientConfiguration{
		Polling: &bigtablepb.ClientConfiguration_PollingConfiguration_{
			PollingConfiguration: &bigtablepb.ClientConfiguration_PollingConfiguration{
				PollingInterval: durationpb.New(600 * time.Second),
			},
		},
	}

	client.getConfigFunc = func(ctx context.Context, req *bigtablepb.GetClientConfigurationRequest) (*bigtablepb.ClientConfiguration, error) {
		return expectedCfg, nil
	}

	manager := NewClientConfigurationManager(client, "instance", "profile", nil, nil)
	manager.poll(context.Background())

	cfg := manager.getConfig()
	if cfg.Polling.PollingInterval != 600*time.Second {
		t.Errorf("Expected polling interval to be 600s, got %v", cfg.Polling.PollingInterval)
	}
}

func TestManagerPoll_FailureKeepsOldConfig(t *testing.T) {
	client := &mockBigtableClient{}
	manager := NewClientConfigurationManager(client, "instance", "profile", nil, nil)

	// Set a valid until time in the future
	manager.validUntil = time.Now().Add(time.Hour)

	client.getConfigFunc = func(ctx context.Context, req *bigtablepb.GetClientConfigurationRequest) (*bigtablepb.ClientConfiguration, error) {
		return nil, status.Error(codes.Unavailable, "service unavailable")
	}

	manager.poll(context.Background())

	cfg := manager.getConfig()
	if cfg != manager.defaultConfig {
		t.Error("Expected config to be equivalent to default config on failure before expiration")
	}
}

func TestManagerPoll_FailureFallbackToDefault(t *testing.T) {
	client := &mockBigtableClient{}
	manager := NewClientConfigurationManager(client, "instance", "profile", nil, nil)

	// Set a valid until time in the past
	manager.validUntil = time.Now().Add(-time.Hour)

	// Change current config to something non-default
	manager.currentConfig = clientConfig{}

	client.getConfigFunc = func(ctx context.Context, req *bigtablepb.GetClientConfigurationRequest) (*bigtablepb.ClientConfiguration, error) {
		return nil, status.Error(codes.Unavailable, "service unavailable")
	}

	manager.poll(context.Background())

	cfg := manager.getConfig()
	if cfg != manager.defaultConfig {
		t.Error("Expected config to fallback to default config on failure after expiration")
	}
}

func TestManagerNotifyListeners(t *testing.T) {
	client := &mockBigtableClient{}
	manager := NewClientConfigurationManager(client, "instance", "profile", nil, nil)

	var wg sync.WaitGroup
	wg.Add(2) // Expect two notifications: immediate, and after poll
	var receivedConfigs []clientConfig
	var receivedSeqs []int64

	manager.addListener(func(cfg clientConfig, seq int64) {
		receivedConfigs = append(receivedConfigs, cfg)
		receivedSeqs = append(receivedSeqs, seq)
		wg.Done()
	})

	expectedCfg := &bigtablepb.ClientConfiguration{
		Polling: &bigtablepb.ClientConfiguration_PollingConfiguration_{
			PollingConfiguration: &bigtablepb.ClientConfiguration_PollingConfiguration{
				PollingInterval: durationpb.New(600 * time.Second),
			},
		},
	}

	client.getConfigFunc = func(ctx context.Context, req *bigtablepb.GetClientConfigurationRequest) (*bigtablepb.ClientConfiguration, error) {
		return expectedCfg, nil
	}

	manager.poll(context.Background())
	wg.Wait()

	if len(receivedConfigs) != 2 {
		t.Fatalf("Expected 2 notifications, got %d", len(receivedConfigs))
	}

	// First config should be equivalent to default config
	if receivedConfigs[0] != manager.defaultConfig {
		t.Error("Expected first notification to have default config")
	}

	// Second config should have the updated polling interval
	if receivedConfigs[1].Polling.PollingInterval != 600*time.Second {
		t.Errorf("Expected second notification to have polling interval 600s, got %v", receivedConfigs[1].Polling.PollingInterval)
	}

	if len(receivedSeqs) != 2 {
		t.Fatalf("Expected 2 sequences, got %d", len(receivedSeqs))
	}
	if receivedSeqs[0] != 0 {
		t.Errorf("Expected first sequence to be 0, got %d", receivedSeqs[0])
	}
	if receivedSeqs[1] != 1 {
		t.Errorf("Expected second sequence to be 1, got %d", receivedSeqs[1])
	}
}

func TestGetConfig_ReturnsCopy(t *testing.T) {
	client := &mockBigtableClient{}
	manager := NewClientConfigurationManager(client, "instance", "profile", nil, nil)

	cfg1 := manager.getConfig()
	cfg1.Polling.MaxRpcRetryCount = 999

	cfg2 := manager.getConfig()
	if cfg2.Polling.MaxRpcRetryCount == 999 {
		t.Error("Expected modifications to returned config to not affect manager state")
	}
}

func TestManagerNotifyListeners_Race(t *testing.T) {
	client := &mockBigtableClient{}
	manager := NewClientConfigurationManager(client, "instance", "profile", nil, nil)

	// Initial config is default (seq 0)

	config1 := &bigtablepb.ClientConfiguration{
		SessionConfiguration: &bigtablepb.SessionClientConfiguration{
			SessionLoad: 1.0,
			SessionPoolConfiguration: &bigtablepb.SessionClientConfiguration_SessionPoolConfiguration{
				MinSessionCount: 10, // Distinct value
			},
		},
	}
	config2 := &bigtablepb.ClientConfiguration{
		SessionConfiguration: &bigtablepb.SessionClientConfiguration{
			SessionLoad: 1.0,
			SessionPoolConfiguration: &bigtablepb.SessionClientConfiguration_SessionPoolConfiguration{
				MinSessionCount: 20, // Distinct value
			},
		},
	}

	// We will use a mock client that returns config1 then config2
	var configIndex int
	var configMu sync.Mutex
	client.getConfigFunc = func(ctx context.Context, req *bigtablepb.GetClientConfigurationRequest) (*bigtablepb.ClientConfiguration, error) {
		configMu.Lock()
		defer configMu.Unlock()
		if configIndex == 0 {
			configIndex++
			return config1, nil
		}
		return config2, nil
	}

	// 1. First poll to get config1 (seq becomes 1)
	manager.poll(context.Background())
	// Now manager.currentConfig is config1, configSeq is 1

	var listenerMu sync.Mutex
	var lastSeq int64
	var activeConfig clientConfig
	var activeConfigSet bool

	// Channels to control the race
	blockImmediate := make(chan struct{})
	immediateCalled := make(chan struct{})
	newerCompleted := make(chan struct{})

	// This listener mimics SessionPoolImpl
	listener := func(cfg clientConfig, seq int64) {
		listenerMu.Lock()
		isImmediate := seq == 1 && cfg.Session.SessionPool.MinSessionCount == 10
		listenerMu.Unlock()

		if isImmediate {
			close(immediateCalled)
			<-blockImmediate // Block the immediate notification
		}

		listenerMu.Lock()
		defer listenerMu.Unlock()

		// Sequence check
		if seq <= lastSeq {
			return
		}
		lastSeq = seq
		activeConfig = cfg
		activeConfigSet = true

		if seq == 2 {
			close(newerCompleted)
		}
	}

	// Start AddListener in a goroutine.
	// It will immediately call listener(config1, 1) which will block.
	go func() {
		manager.addListener(listener)
	}()

	// Wait until immediate notification is called and blocked
	<-immediateCalled

	// Now trigger second poll.
	// This will update config to config2 (seq 2).
	// It will notify listeners (including our registered listener) with (config2, 2).
	// This call to listener should NOT block because seq != 1.
	manager.poll(context.Background())

	// Wait for the newer notification to complete
	<-newerCompleted

	// Now unblock the immediate notification (config1, 1)
	close(blockImmediate)

	// Give some time for immediate notification to finish (it should be discarded)
	time.Sleep(50 * time.Millisecond)

	listenerMu.Lock()
	defer listenerMu.Unlock()

	// Assert that activeConfig is config2 (seq 2) and NOT config1 (seq 1)
	if !activeConfigSet {
		t.Fatal("Expected activeConfig to be non-nil")
	}
	minSessions := activeConfig.Session.SessionPool.MinSessionCount
	if minSessions != 20 {
		t.Errorf("Expected activeConfig to have MinSessionCount 20 (config2), got %d", minSessions)
	}
}

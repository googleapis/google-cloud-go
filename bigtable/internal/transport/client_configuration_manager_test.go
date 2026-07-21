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
	"google.golang.org/protobuf/proto"
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

	if got := maxRPCRetryCount(manager.getConfig()); got != 5 {
		t.Errorf("Expected MaxRpcRetryCount to be 5, got %d", got)
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

	if got := pollingInterval(manager.getConfig()); got != 600*time.Second {
		t.Errorf("Expected polling interval to be 600s, got %v", got)
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
	if !proto.Equal(cfg, manager.defaultConfig) {
		t.Error("Expected config to be equivalent to default config on failure before expiration")
	}
}

func TestManagerPoll_FailureFallbackToDefault(t *testing.T) {
	client := &mockBigtableClient{}
	manager := NewClientConfigurationManager(client, "instance", "profile", nil, nil)

	// Set a valid until time in the past
	manager.validUntil = time.Now().Add(-time.Hour)

	// Change current config to something non-default.
	manager.currentConfig = &bigtablepb.ClientConfiguration{}

	client.getConfigFunc = func(ctx context.Context, req *bigtablepb.GetClientConfigurationRequest) (*bigtablepb.ClientConfiguration, error) {
		return nil, status.Error(codes.Unavailable, "service unavailable")
	}

	manager.poll(context.Background())

	cfg := manager.getConfig()
	if !proto.Equal(cfg, manager.defaultConfig) {
		t.Error("Expected config to fallback to default config on failure after expiration")
	}
}

func TestManagerNotifyListeners(t *testing.T) {
	client := &mockBigtableClient{}
	manager := NewClientConfigurationManager(client, "instance", "profile", nil, nil)

	var wg sync.WaitGroup
	wg.Add(2) // Expect two notifications: immediate, and after poll
	var receivedConfigs []*bigtablepb.ClientConfiguration
	var receivedSeqs []int64

	manager.addListener(func(cfg *bigtablepb.ClientConfiguration, seq int64) {
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
	if !proto.Equal(receivedConfigs[0], manager.defaultConfig) {
		t.Error("Expected first notification to have default config")
	}

	// Second config should have the updated polling interval
	if got := pollingInterval(receivedConfigs[1]); got != 600*time.Second {
		t.Errorf("Expected second notification to have polling interval 600s, got %v", got)
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

func TestManagerPoll_UnchangedConfigSkipsListenerFire(t *testing.T) {
	client := &mockBigtableClient{}
	sameCfg := &bigtablepb.ClientConfiguration{
		Polling: &bigtablepb.ClientConfiguration_PollingConfiguration_{
			PollingConfiguration: &bigtablepb.ClientConfiguration_PollingConfiguration{
				PollingInterval: durationpb.New(600 * time.Second),
			},
		},
	}
	client.getConfigFunc = func(ctx context.Context, req *bigtablepb.GetClientConfigurationRequest) (*bigtablepb.ClientConfiguration, error) {
		return sameCfg, nil
	}

	manager := NewClientConfigurationManager(client, "instance", "profile", nil, nil)

	var mu sync.Mutex
	var seqs []int64
	manager.addListener(func(cfg *bigtablepb.ClientConfiguration, seq int64) {
		mu.Lock()
		seqs = append(seqs, seq)
		mu.Unlock()
	})

	// Three back-to-back polls, all returning sameCfg. After the first poll
	// transitions default -> sameCfg, the next two polls return identical
	// configs and must not fire the listener again.
	manager.poll(context.Background())
	manager.poll(context.Background())
	manager.poll(context.Background())

	mu.Lock()
	defer mu.Unlock()
	// Expect seqs == [0, 1]: bootstrap fire (seq=0, default) + first poll
	// (default -> sameCfg, seq=1). Polls 2 and 3 are no-ops.
	if len(seqs) != 2 {
		t.Fatalf("Expected 2 listener fires (bootstrap + first poll), got %d (seqs=%v)", len(seqs), seqs)
	}
	if seqs[0] != 0 || seqs[1] != 1 {
		t.Errorf("Expected seqs [0 1], got %v", seqs)
	}
	manager.mu.RLock()
	gotSeq := manager.configSeq
	manager.mu.RUnlock()
	if gotSeq != 1 {
		t.Errorf("Expected configSeq=1 after unchanged polls, got %d", gotSeq)
	}
}

func TestManagerPoll_UnchangedConfigRefreshesValidity(t *testing.T) {
	client := &mockBigtableClient{}
	cfg := &bigtablepb.ClientConfiguration{
		Polling: &bigtablepb.ClientConfiguration_PollingConfiguration_{
			PollingConfiguration: &bigtablepb.ClientConfiguration_PollingConfiguration{
				PollingInterval:  durationpb.New(600 * time.Second),
				ValidityDuration: durationpb.New(2 * time.Hour),
			},
		},
	}
	client.getConfigFunc = func(ctx context.Context, req *bigtablepb.GetClientConfigurationRequest) (*bigtablepb.ClientConfiguration, error) {
		return cfg, nil
	}

	manager := NewClientConfigurationManager(client, "instance", "profile", nil, nil)

	// First poll: transitions default -> cfg, sets validUntil ~= now + 2h.
	manager.poll(context.Background())

	// Backdate validUntil so we can verify the next no-op poll refreshes it.
	manager.mu.Lock()
	manager.validUntil = time.Now().Add(-time.Minute)
	manager.mu.Unlock()

	manager.poll(context.Background())

	manager.mu.RLock()
	validUntil := manager.validUntil
	manager.mu.RUnlock()
	// A refreshed validity window must be at least ~1h59m into the future
	// (2h ValidityDuration minus generous slop for slow CI).
	if min := time.Now().Add(time.Hour + 50*time.Minute); validUntil.Before(min) {
		t.Errorf("Expected unchanged poll to refresh validUntil >= %v, got %v", min, validUntil)
	}
}

func TestGetConfig_ReturnsCopy(t *testing.T) {
	client := &mockBigtableClient{}
	manager := NewClientConfigurationManager(client, "instance", "profile", nil, nil)

	// getConfig returns a proto.Clone, so mutations on the returned message
	// must not reach the manager's stored currentConfig.
	cfg1 := manager.getConfig()
	cfg1.GetPollingConfiguration().MaxRpcRetryCount = 999

	cfg2 := manager.getConfig()
	if got := cfg2.GetPollingConfiguration().GetMaxRpcRetryCount(); got == 999 {
		t.Error("Expected modifications to returned config to not affect manager state")
	}
}

func TestClose_DoubleCloseDoesNotPanic(t *testing.T) {
	client := &mockBigtableClient{}
	manager := NewClientConfigurationManager(client, "instance", "profile", nil, nil)

	// First Close performs the actual teardown; second Close must be a no-op
	// rather than panicking with "close of closed channel".
	manager.Close()
	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("Second Close() panicked: %v", r)
		}
	}()
	manager.Close()

	if !manager.isClosed() {
		t.Error("Expected manager to report closed after Close()")
	}
}

func TestClose_SuppressesListenerCallbacks(t *testing.T) {
	client := &mockBigtableClient{}
	manager := NewClientConfigurationManager(client, "instance", "profile", nil, nil)

	// Listener captures invocations made AFTER Close().
	var mu sync.Mutex
	var callsAfterClose int
	manager.addListener(func(cfg *bigtablepb.ClientConfiguration, seq int64) {
		mu.Lock()
		defer mu.Unlock()
		if manager.isClosed() {
			callsAfterClose++
		}
	})

	// Drive a poll that, on the happy path, would fire listeners. Close
	// flips the gate first; poll() must observe it and skip the fire.
	client.getConfigFunc = func(ctx context.Context, req *bigtablepb.GetClientConfigurationRequest) (*bigtablepb.ClientConfiguration, error) {
		return &bigtablepb.ClientConfiguration{
			Polling: &bigtablepb.ClientConfiguration_PollingConfiguration_{
				PollingConfiguration: &bigtablepb.ClientConfiguration_PollingConfiguration{
					PollingInterval: durationpb.New(600 * time.Second),
				},
			},
		}, nil
	}

	manager.Close()
	manager.poll(context.Background())

	mu.Lock()
	defer mu.Unlock()
	if callsAfterClose != 0 {
		t.Errorf("Expected no listener calls after Close(), got %d", callsAfterClose)
	}
}

func TestClose_WaitsForInFlightPolls(t *testing.T) {
	client := &mockBigtableClient{}
	manager := NewClientConfigurationManager(client, "instance", "profile", nil, nil)

	// Block the RPC so the poll() launched by Start() is in-flight when
	// Close() is called. Close() must wait for it before returning.
	rpcStarted := make(chan struct{})
	releaseRPC := make(chan struct{})
	client.getConfigFunc = func(ctx context.Context, req *bigtablepb.GetClientConfigurationRequest) (*bigtablepb.ClientConfiguration, error) {
		close(rpcStarted)
		<-releaseRPC
		return nil, status.Error(codes.Unavailable, "unavailable")
	}

	// Pin currentConfig to a proto that explicitly declares 0 retries so the
	// poll terminates after a single attempt (matches the pre-rewrite test).
	// The PollingConfiguration case being set at all is what tells the
	// helper to honor the 0 verbatim instead of falling back to the default.
	manager.currentConfig = &bigtablepb.ClientConfiguration{
		Polling: &bigtablepb.ClientConfiguration_PollingConfiguration_{
			PollingConfiguration: &bigtablepb.ClientConfiguration_PollingConfiguration{
				MaxRpcRetryCount: 0,
			},
		},
	}

	manager.Start(context.Background())
	<-rpcStarted

	closeReturned := make(chan struct{})
	go func() {
		manager.Close()
		close(closeReturned)
	}()

	// Close() must not return while the poll() is in flight.
	select {
	case <-closeReturned:
		t.Fatal("Close() returned before in-flight poll completed")
	case <-time.After(50 * time.Millisecond):
	}

	close(releaseRPC)

	select {
	case <-closeReturned:
	case <-time.After(time.Second):
		t.Fatal("Close() did not return after in-flight poll completed")
	}
}

func TestAddSessionLoadListener_SkipsImmediateDefaultFire(t *testing.T) {
	// AddSessionLoadListener intentionally suppresses the seq=0 registration-
	// time fire so the Diverter's bootstrap value (e.g. NewDiverter(1.0)) is
	// not silently overwritten by the default config's SessionLoad=0.
	client := &mockBigtableClient{}
	manager := NewClientConfigurationManager(client, "instance", "profile", nil, nil)

	got := make(chan float64, 1)
	unregister := manager.AddSessionLoadListener(func(load float64) {
		select {
		case got <- load:
		default:
		}
	})
	defer unregister()

	select {
	case load := <-got:
		t.Fatalf("listener fired with load=%v at registration time; want no fire (seq=0 suppression)", load)
	case <-time.After(100 * time.Millisecond):
		// Expected: no fire.
	}
}

func TestAddSessionLoadListener_FiresOnFirstPoll(t *testing.T) {
	// After the first successful poll, AddSessionLoadListener fires with the
	// server-reported SessionLoad.
	client := &mockBigtableClient{
		getConfigFunc: func(ctx context.Context, req *bigtablepb.GetClientConfigurationRequest) (*bigtablepb.ClientConfiguration, error) {
			return &bigtablepb.ClientConfiguration{
				SessionConfiguration: &bigtablepb.SessionClientConfiguration{
					SessionLoad: 0.75,
				},
			}, nil
		},
	}
	manager := NewClientConfigurationManager(client, "instance", "profile", nil, nil)

	got := make(chan float64, 1)
	unregister := manager.AddSessionLoadListener(func(load float64) {
		select {
		case got <- load:
		default:
		}
	})
	defer unregister()

	manager.poll(context.Background())

	select {
	case load := <-got:
		if load != 0.75 {
			t.Errorf("post-poll SessionLoad fire = %v, want 0.75", load)
		}
	case <-time.After(time.Second):
		t.Fatal("AddSessionLoadListener did not fire after first poll")
	}
}

func TestAddSessionLoadListener_UnregisterStopsCallbacks(t *testing.T) {
	client := &mockBigtableClient{}
	manager := NewClientConfigurationManager(client, "instance", "profile", nil, nil)

	var mu sync.Mutex
	var totalCalls int
	unregister := manager.AddSessionLoadListener(func(load float64) {
		mu.Lock()
		defer mu.Unlock()
		totalCalls++
	})

	// No immediate fire (seq=0 suppression); confirm baseline is 0.
	mu.Lock()
	if totalCalls != 0 {
		mu.Unlock()
		t.Fatalf("listener fired %d times at registration; want 0 (seq=0 suppression)", totalCalls)
	}
	mu.Unlock()

	unregister()

	// Now drive a successful poll. The listener was removed, so its counter
	// must stay at 0 even though the manager re-notifies all currently
	// registered listeners.
	client.getConfigFunc = func(ctx context.Context, req *bigtablepb.GetClientConfigurationRequest) (*bigtablepb.ClientConfiguration, error) {
		return &bigtablepb.ClientConfiguration{
			SessionConfiguration: &bigtablepb.SessionClientConfiguration{
				SessionLoad: 0.75,
			},
		}, nil
	}
	manager.poll(context.Background())

	mu.Lock()
	defer mu.Unlock()
	if totalCalls != 0 {
		t.Errorf("listener fired %d times after unregister; want 0", totalCalls)
	}
}

func TestAddSessionPoolListener_SkipsOnUnchangedSlice(t *testing.T) {
	// Two distinct configs that differ on PollingInterval (so the whole-
	// config diff in poll() lets the fan-out through) but carry the same
	// SessionPool slice. The per-listener extractor diff must suppress the
	// second SP delivery.
	const (
		minCount = 10
		maxCount = 200
	)
	configs := []*bigtablepb.ClientConfiguration{
		{
			Polling: &bigtablepb.ClientConfiguration_PollingConfiguration_{
				PollingConfiguration: &bigtablepb.ClientConfiguration_PollingConfiguration{
					PollingInterval: durationpb.New(600 * time.Second),
				},
			},
			SessionConfiguration: &bigtablepb.SessionClientConfiguration{
				SessionLoad: 0.5,
				SessionPoolConfiguration: &bigtablepb.SessionClientConfiguration_SessionPoolConfiguration{
					MinSessionCount: minCount,
					MaxSessionCount: maxCount,
				},
			},
		},
		{
			Polling: &bigtablepb.ClientConfiguration_PollingConfiguration_{
				PollingConfiguration: &bigtablepb.ClientConfiguration_PollingConfiguration{
					PollingInterval: durationpb.New(900 * time.Second),
				},
			},
			SessionConfiguration: &bigtablepb.SessionClientConfiguration{
				SessionLoad: 0.7, // changed — but SP slice still identical
				SessionPoolConfiguration: &bigtablepb.SessionClientConfiguration_SessionPoolConfiguration{
					MinSessionCount: minCount,
					MaxSessionCount: maxCount,
				},
			},
		},
	}
	var idx int
	var idxMu sync.Mutex
	client := &mockBigtableClient{
		getConfigFunc: func(ctx context.Context, req *bigtablepb.GetClientConfigurationRequest) (*bigtablepb.ClientConfiguration, error) {
			idxMu.Lock()
			defer idxMu.Unlock()
			cfg := configs[idx]
			if idx+1 < len(configs) {
				idx++
			}
			return cfg, nil
		},
	}
	manager := NewClientConfigurationManager(client, "instance", "profile", nil, nil)

	var mu sync.Mutex
	var deliveries []*bigtablepb.SessionClientConfiguration_SessionPoolConfiguration
	manager.AddSessionPoolListener(func(sp *bigtablepb.SessionClientConfiguration_SessionPoolConfiguration) {
		mu.Lock()
		deliveries = append(deliveries, sp)
		mu.Unlock()
	})

	manager.poll(context.Background()) // default SP -> {10, 200}: fires
	manager.poll(context.Background()) // {10, 200} -> {10, 200}: must skip

	mu.Lock()
	defer mu.Unlock()
	// Expect 2 deliveries: bootstrap (default SP) + first poll (default ->
	// {10, 200}). The second poll changes the whole config but not the SP
	// slice, so the per-listener diff must suppress that fire.
	if len(deliveries) != 2 {
		t.Fatalf("AddSessionPoolListener fired %d times, want 2 (bootstrap + first poll only)", len(deliveries))
	}
	if got := deliveries[1].GetMinSessionCount(); got != minCount {
		t.Errorf("post-first-poll MinSessionCount = %d, want %d", got, minCount)
	}
}

func TestAddSessionLoadListener_SkipsOnUnchangedLoad(t *testing.T) {
	// Same shape as the SessionPool test: two polls whose only difference
	// is the PollingInterval. SessionLoad is held constant, so the per-
	// listener load diff must suppress the second fire even though the
	// whole-config diff doesn't.
	configs := []*bigtablepb.ClientConfiguration{
		{
			Polling: &bigtablepb.ClientConfiguration_PollingConfiguration_{
				PollingConfiguration: &bigtablepb.ClientConfiguration_PollingConfiguration{
					PollingInterval: durationpb.New(600 * time.Second),
				},
			},
			SessionConfiguration: &bigtablepb.SessionClientConfiguration{SessionLoad: 0.5},
		},
		{
			Polling: &bigtablepb.ClientConfiguration_PollingConfiguration_{
				PollingConfiguration: &bigtablepb.ClientConfiguration_PollingConfiguration{
					PollingInterval: durationpb.New(900 * time.Second),
				},
			},
			SessionConfiguration: &bigtablepb.SessionClientConfiguration{SessionLoad: 0.5},
		},
	}
	var idx int
	var idxMu sync.Mutex
	client := &mockBigtableClient{
		getConfigFunc: func(ctx context.Context, req *bigtablepb.GetClientConfigurationRequest) (*bigtablepb.ClientConfiguration, error) {
			idxMu.Lock()
			defer idxMu.Unlock()
			cfg := configs[idx]
			if idx+1 < len(configs) {
				idx++
			}
			return cfg, nil
		},
	}
	manager := NewClientConfigurationManager(client, "instance", "profile", nil, nil)

	var mu sync.Mutex
	var loads []float64
	manager.AddSessionLoadListener(func(load float64) {
		mu.Lock()
		loads = append(loads, load)
		mu.Unlock()
	})

	manager.poll(context.Background()) // default load 0 -> 0.5: fires (seq>0)
	manager.poll(context.Background()) // 0.5 -> 0.5: must skip

	mu.Lock()
	defer mu.Unlock()
	// Expect 1 delivery: bootstrap is suppressed by AddSessionLoadListener's
	// seq=0 guard, the first poll fires with 0.5, and the second poll's
	// per-listener diff suppresses the redundant 0.5 delivery.
	if len(loads) != 1 || loads[0] != 0.5 {
		t.Fatalf("AddSessionLoadListener deliveries = %v, want [0.5]", loads)
	}
}

func TestManagerPoll_StopPollingFlagsCurrentConfig(t *testing.T) {
	client := &mockBigtableClient{
		getConfigFunc: func(ctx context.Context, req *bigtablepb.GetClientConfigurationRequest) (*bigtablepb.ClientConfiguration, error) {
			return &bigtablepb.ClientConfiguration{
				Polling: &bigtablepb.ClientConfiguration_StopPolling{StopPolling: true},
			}, nil
		},
	}
	manager := NewClientConfigurationManager(client, "instance", "profile", nil, nil)
	manager.poll(context.Background())

	if !manager.getConfig().GetStopPolling() {
		t.Fatalf("expected currentConfig StopPolling=true after StopPolling response, got false")
	}
}

// TestManagerNotifyListeners_Race was removed when addListener moved to
// holding m.mu across the registration-time fire. The race it guarded
// against — poll() fan-out interleaving with addListener's deferred fire
// and delivering seq=N+1 before seq=N — is structurally impossible now:
// poll() cannot snapshot the listener map while addListener holds the
// write lock, so the registration fire always lands before any poll
// fire and listeners observe seq monotonically by construction.

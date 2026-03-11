/*
Copyright 2026 Google LLC

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package spanner

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
)

// closableSpannerClient is a minimal spannerClient that tracks Close calls.
// It uses mockSpannerClient from location_aware_client_test.go as the base.
type closableSpannerClient struct {
	mockSpannerClient
	closed atomic.Bool
}

func (c *closableSpannerClient) Close() error {
	c.closed.Store(true)
	return nil
}

func TestEndpointClientCache_GetOrCreate(t *testing.T) {
	createCount := 0
	factory := func(ctx context.Context, address string) (spannerClient, error) {
		createCount++
		return &closableSpannerClient{}, nil
	}

	cache := newEndpointClientCache(factory)

	// First Get should create.
	ep1 := cache.Get(context.Background(), "addr1")
	if ep1 == nil {
		t.Fatal("expected non-nil endpoint")
	}
	if ep1.Address() != "addr1" {
		t.Fatalf("expected address addr1, got %s", ep1.Address())
	}
	if !ep1.IsHealthy() {
		t.Fatal("expected endpoint to be healthy")
	}
	if createCount != 1 {
		t.Fatalf("expected 1 creation, got %d", createCount)
	}

	// Second Get for same address should hit cache.
	ep2 := cache.Get(context.Background(), "addr1")
	if ep2 != ep1 {
		t.Fatal("expected same endpoint from cache")
	}
	if createCount != 1 {
		t.Fatalf("expected still 1 creation, got %d", createCount)
	}

	// Different address should create new.
	ep3 := cache.Get(context.Background(), "addr2")
	if ep3 == nil {
		t.Fatal("expected non-nil endpoint for addr2")
	}
	if createCount != 2 {
		t.Fatalf("expected 2 creations, got %d", createCount)
	}
}

func TestEndpointClientCache_ClientFor(t *testing.T) {
	client := &closableSpannerClient{}
	factory := func(ctx context.Context, address string) (spannerClient, error) {
		return client, nil
	}

	cache := newEndpointClientCache(factory)
	ep := cache.Get(context.Background(), "addr1")

	resolved := cache.ClientFor(ep)
	if resolved == nil {
		t.Fatal("expected non-nil client from ClientFor")
	}

	// ClientFor with nil should return nil.
	if cache.ClientFor(nil) != nil {
		t.Fatal("expected nil from ClientFor(nil)")
	}

	// ClientFor with non-grpcChannelEndpoint should return nil.
	passthrough := &passthroughChannelEndpoint{address: "foo"}
	if cache.ClientFor(passthrough) != nil {
		t.Fatal("expected nil from ClientFor(passthroughEndpoint)")
	}
}

func TestEndpointClientCache_Close(t *testing.T) {
	clients := make([]*closableSpannerClient, 0)
	factory := func(ctx context.Context, address string) (spannerClient, error) {
		c := &closableSpannerClient{}
		clients = append(clients, c)
		return c, nil
	}

	cache := newEndpointClientCache(factory)
	cache.Get(context.Background(), "addr1")
	cache.Get(context.Background(), "addr2")

	if len(clients) != 2 {
		t.Fatalf("expected 2 clients, got %d", len(clients))
	}

	if err := cache.Close(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	for i, c := range clients {
		if !c.closed.Load() {
			t.Fatalf("client %d was not closed", i)
		}
	}

	// Cache should be empty after close.
	cache.mu.RLock()
	remaining := len(cache.endpoints)
	cache.mu.RUnlock()
	if remaining != 0 {
		t.Fatalf("expected 0 endpoints after close, got %d", remaining)
	}
}

func TestEndpointClientCache_FactoryError(t *testing.T) {
	factory := func(ctx context.Context, address string) (spannerClient, error) {
		return nil, fmt.Errorf("connection failed")
	}

	cache := newEndpointClientCache(factory)
	ep := cache.Get(context.Background(), "addr1")
	if ep != nil {
		t.Fatal("expected nil endpoint when factory fails")
	}
}

func TestEndpointClientCache_UnhealthyEndpoint(t *testing.T) {
	factory := func(ctx context.Context, address string) (spannerClient, error) {
		return &closableSpannerClient{}, nil
	}

	cache := newEndpointClientCache(factory)
	ep := cache.Get(context.Background(), "addr1")
	if !ep.IsHealthy() {
		t.Fatal("expected healthy initially")
	}

	// Mark unhealthy.
	gep := ep.(*grpcChannelEndpoint)
	gep.healthy.Store(false)
	if ep.IsHealthy() {
		t.Fatal("expected unhealthy after marking")
	}
}

func TestEndpointClientCache_GetPassesContextToFactory(t *testing.T) {
	type contextKey string

	const key contextKey = "request"
	ctx := context.WithValue(context.Background(), key, "ctx-value")
	factory := func(factoryCtx context.Context, address string) (spannerClient, error) {
		if got := factoryCtx.Value(key); got != "ctx-value" {
			t.Fatalf("expected context value to propagate, got %v", got)
		}
		return &closableSpannerClient{}, nil
	}

	cache := newEndpointClientCache(factory)
	if ep := cache.Get(ctx, "addr1"); ep == nil {
		t.Fatal("expected endpoint")
	}
}

func TestEndpointClientCache_GetSingleFlightPerAddress(t *testing.T) {
	var createCount atomic.Int32
	started := make(chan struct{})
	release := make(chan struct{})
	factory := func(ctx context.Context, address string) (spannerClient, error) {
		if createCount.Add(1) == 1 {
			close(started)
		}
		<-release
		return &closableSpannerClient{}, nil
	}

	cache := newEndpointClientCache(factory)
	results := make(chan channelEndpoint, 2)
	var wg sync.WaitGroup
	for i := 0; i < 2; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			results <- cache.Get(context.Background(), "addr1")
		}()
	}

	<-started
	close(release)
	wg.Wait()
	close(results)

	if got := createCount.Load(); got != 1 {
		t.Fatalf("expected one factory call, got %d", got)
	}

	var first channelEndpoint
	for ep := range results {
		if ep == nil {
			t.Fatal("expected non-nil endpoint")
		}
		if first == nil {
			first = ep
			continue
		}
		if ep != first {
			t.Fatal("expected both callers to observe the same endpoint")
		}
	}
}

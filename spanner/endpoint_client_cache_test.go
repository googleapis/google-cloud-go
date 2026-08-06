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
	"errors"
	"fmt"
	"net"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/connectivity"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/test/bufconn"
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

func newReadyTestConn(t *testing.T) (*grpc.ClientConn, func()) {
	t.Helper()

	listener := bufconn.Listen(1024 * 1024)
	server := grpc.NewServer()
	done := make(chan struct{})
	go func() {
		defer close(done)
		_ = server.Serve(listener)
	}()

	dialer := func(context.Context, string) (net.Conn, error) {
		return listener.Dial()
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	conn, err := grpc.DialContext(
		ctx,
		"passthrough:///bufconn",
		grpc.WithContextDialer(dialer),
		grpc.WithTransportCredentials(insecure.NewCredentials()),
	)
	if err != nil {
		t.Fatalf("DialContext() failed: %v", err)
	}
	conn.Connect()
	waitForConnState(t, conn, connectivity.Ready)

	cleanup := func() {
		_ = conn.Close()
		server.Stop()
		<-done
	}
	return conn, cleanup
}

func newTransientFailureTestConn(t *testing.T) (*grpc.ClientConn, func()) {
	t.Helper()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	conn, err := grpc.DialContext(
		ctx,
		"passthrough:///transient-failure",
		grpc.WithContextDialer(func(context.Context, string) (net.Conn, error) {
			return nil, errors.New("dial failed")
		}),
		grpc.WithTransportCredentials(insecure.NewCredentials()),
	)
	if err != nil {
		t.Fatalf("DialContext() failed: %v", err)
	}
	conn.Connect()
	waitForConnState(t, conn, connectivity.TransientFailure)

	return conn, func() { _ = conn.Close() }
}

func waitForConnState(t *testing.T, conn *grpc.ClientConn, target connectivity.State) {
	t.Helper()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	for {
		state := conn.GetState()
		if state == target {
			return
		}
		if !conn.WaitForStateChange(ctx, state) {
			t.Fatalf("timed out waiting for state %v, last state %v", target, state)
		}
	}
}

func TestEndpointClientCache_GetOrCreate(t *testing.T) {
	conn, cleanup := newReadyTestConn(t)
	defer cleanup()

	createCount := 0
	factory := func(ctx context.Context, address string) (spannerClient, error) {
		createCount++
		return &closableSpannerClient{mockSpannerClient: mockSpannerClient{conn: conn}}, nil
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
	conn, cleanup := newTransientFailureTestConn(t)
	defer cleanup()

	factory := func(ctx context.Context, address string) (spannerClient, error) {
		return &closableSpannerClient{mockSpannerClient: mockSpannerClient{conn: conn}}, nil
	}

	cache := newEndpointClientCache(factory)
	ep := cache.Get(context.Background(), "addr1")
	if ep.IsHealthy() {
		t.Fatal("expected endpoint to be unhealthy in transient failure")
	}
	if !ep.IsTransientFailure() {
		t.Fatal("expected endpoint to report transient failure")
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

func TestEndpointClientCache_GetIfPresent(t *testing.T) {
	createCount := 0
	factory := func(ctx context.Context, address string) (spannerClient, error) {
		createCount++
		return &closableSpannerClient{}, nil
	}

	cache := newEndpointClientCache(factory)
	if got := cache.GetIfPresent("addr1"); got != nil {
		t.Fatalf("GetIfPresent(addr1) = %#v, want nil", got)
	}
	if createCount != 0 {
		t.Fatalf("expected GetIfPresent not to create clients, got %d creations", createCount)
	}

	created := cache.Get(context.Background(), "addr1")
	if created == nil {
		t.Fatal("expected endpoint creation")
	}
	if got := cache.GetIfPresent("addr1"); got != created {
		t.Fatalf("GetIfPresent(addr1) = %#v, want %#v", got, created)
	}
}

func TestEndpointClientCache_Evict(t *testing.T) {
	client := &closableSpannerClient{}
	cache := newEndpointClientCache(func(context.Context, string) (spannerClient, error) {
		return client, nil
	})

	created := cache.Get(context.Background(), "addr1")
	if created == nil {
		t.Fatal("expected endpoint creation")
	}

	cache.Evict("addr1")

	if !client.closed.Load() {
		t.Fatal("expected evicted endpoint client to be closed")
	}
	if got := cache.GetIfPresent("addr1"); got != nil {
		t.Fatalf("GetIfPresent(addr1) after Evict = %#v, want nil", got)
	}
}

func TestEndpointClientCache_EvictInFlightCreation(t *testing.T) {
	started := make(chan struct{})
	release := make(chan struct{})
	client := &closableSpannerClient{}
	cache := newEndpointClientCache(func(context.Context, string) (spannerClient, error) {
		close(started)
		<-release
		return client, nil
	})

	result := make(chan channelEndpoint, 1)
	go func() {
		result <- cache.Get(context.Background(), "addr1")
	}()

	<-started
	cache.Evict("addr1")
	close(release)

	if got := <-result; got != nil {
		t.Fatalf("Get(addr1) after inflight Evict = %#v, want nil", got)
	}
	if !client.closed.Load() {
		t.Fatal("expected evicted in-flight client to be closed")
	}
}

func TestEndpointClientCache_DefaultChannel(t *testing.T) {
	cache := newEndpointClientCache(func(context.Context, string) (spannerClient, error) {
		return &closableSpannerClient{}, nil
	})

	defaultChannel := cache.DefaultChannel()
	if defaultChannel == nil {
		t.Fatal("expected non-nil default channel")
	}
	if defaultChannel.Address() != "" {
		t.Fatalf("default channel address = %q, want empty sentinel", defaultChannel.Address())
	}

	cache.Evict(defaultChannel.Address())
	if got := cache.DefaultChannel(); got != defaultChannel {
		t.Fatalf("default channel changed after Evict: got %#v, want %#v", got, defaultChannel)
	}
}

func TestEndpointClientCache_DefaultChannelUsesConfiguredAddress(t *testing.T) {
	cache := newEndpointClientCacheWithDefaultAddress(
		func(context.Context, string) (spannerClient, error) {
			return &closableSpannerClient{}, nil
		},
		"spanner.googleapis.com:443",
	)

	defaultChannel := cache.DefaultChannel()
	if defaultChannel == nil {
		t.Fatal("expected non-nil default channel")
	}
	if got := defaultChannel.Address(); got != "spanner.googleapis.com:443" {
		t.Fatalf("default channel address = %q, want %q", got, "spanner.googleapis.com:443")
	}
}

func TestEndpointClientCache_GetReturnsDefaultChannelForConfiguredAddress(t *testing.T) {
	createCount := 0
	cache := newEndpointClientCacheWithDefaultAddress(
		func(context.Context, string) (spannerClient, error) {
			createCount++
			t.Fatal("factory should not be called for the configured default endpoint")
			return nil, nil
		},
		"spanner.googleapis.com:443",
	)

	defaultChannel := cache.DefaultChannel()
	if defaultChannel == nil {
		t.Fatal("expected non-nil default channel")
	}

	if got := cache.Get(context.Background(), "spanner.googleapis.com:443"); got != defaultChannel {
		t.Fatalf("Get(default endpoint) = %#v, want %#v", got, defaultChannel)
	}
	if createCount != 0 {
		t.Fatalf("factory createCount = %d, want 0", createCount)
	}
}

func TestGRPCChannelEndpoint_GetConn(t *testing.T) {
	conn, cleanup := newReadyTestConn(t)
	defer cleanup()

	endpoint := &grpcChannelEndpoint{
		address: "addr1",
		conn:    conn,
	}
	if got := endpoint.GetConn(); got != conn {
		t.Fatalf("GetConn() = %#v, want %#v", got, conn)
	}
}

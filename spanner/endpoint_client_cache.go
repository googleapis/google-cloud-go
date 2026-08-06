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
	"sync"
	"sync/atomic"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/connectivity"
)

// grpcChannelEndpoint is a channelEndpoint backed by a real gRPC connection.
type grpcChannelEndpoint struct {
	address string
	client  spannerClient
	conn    *grpc.ClientConn
	active  atomic.Int64

	stateMu             sync.Mutex
	consecutiveFailures int
	cooldownUntil       time.Time
	lastFailureAt       time.Time
	scores              map[endpointScoreKey]*endpointScoreState

	lifecycleMu       sync.Mutex
	lastRealTrafficAt time.Time
	transientFailures int
}

var (
	_                     channelEndpoint      = (*grpcChannelEndpoint)(nil)
	_                     channelEndpointCache = (*endpointClientCache)(nil)
	endpointCacheRegistry sync.Map
)

func (e *grpcChannelEndpoint) Address() string {
	return e.address
}

func (e *grpcChannelEndpoint) IsHealthy() bool {
	if e == nil || e.conn == nil {
		return false
	}
	return e.conn.GetState() == connectivity.Ready
}

func (e *grpcChannelEndpoint) IsTransientFailure() bool {
	if e == nil || e.conn == nil {
		return false
	}
	return e.conn.GetState() == connectivity.TransientFailure
}

func (e *grpcChannelEndpoint) GetConn() *grpc.ClientConn {
	if e == nil {
		return nil
	}
	return e.conn
}

func (e *grpcChannelEndpoint) IncrementActiveRequests() {
	if e == nil {
		return
	}
	e.active.Add(1)
}

func (e *grpcChannelEndpoint) DecrementActiveRequests() {
	if e == nil {
		return
	}
	e.active.Add(-1)
}

func (e *grpcChannelEndpoint) ActiveRequestCount() int {
	if e == nil {
		return 0
	}
	return int(e.active.Load())
}

func (e *grpcChannelEndpoint) recordRealTrafficAt(now time.Time) {
	if e == nil {
		return
	}
	e.lifecycleMu.Lock()
	defer e.lifecycleMu.Unlock()
	e.lastRealTrafficAt = now
}

func (e *grpcChannelEndpoint) resetLifecycleTransientFailures() {
	if e == nil {
		return
	}
	e.lifecycleMu.Lock()
	defer e.lifecycleMu.Unlock()
	e.transientFailures = 0
}

func (e *grpcChannelEndpoint) incrementLifecycleTransientFailures() int {
	if e == nil {
		return 0
	}
	e.lifecycleMu.Lock()
	defer e.lifecycleMu.Unlock()
	e.transientFailures++
	return e.transientFailures
}

func (e *grpcChannelEndpoint) lifecycleStateSnapshot() endpointLifecycleState {
	if e == nil {
		return endpointLifecycleState{}
	}
	e.lifecycleMu.Lock()
	defer e.lifecycleMu.Unlock()
	return endpointLifecycleState{
		lastRealTrafficAt:            e.lastRealTrafficAt,
		consecutiveTransientFailures: e.transientFailures,
	}
}

// endpointClientCache implements channelEndpointCache with actual gRPC
// connections to specific server addresses.
type endpointClientCache struct {
	mu              sync.RWMutex
	endpoints       map[string]*grpcChannelEndpoint
	inflight        map[string]*endpointClientCreation
	evicted         map[string]struct{}
	defaultEndpoint channelEndpoint
	clientFactory   func(ctx context.Context, address string) (spannerClient, error)
	closed          bool
	routingConfig   endpointRoutingConfig
}

type endpointClientCreation struct {
	done chan struct{}
	ep   channelEndpoint
}

func newEndpointClientCache(clientFactory func(ctx context.Context, address string) (spannerClient, error)) *endpointClientCache {
	return newEndpointClientCacheWithDefaultAddress(clientFactory, "")
}

func newEndpointClientCacheWithDefaultAddress(clientFactory func(ctx context.Context, address string) (spannerClient, error), defaultAddress string) *endpointClientCache {
	cache := &endpointClientCache{
		endpoints:       make(map[string]*grpcChannelEndpoint),
		inflight:        make(map[string]*endpointClientCreation),
		evicted:         make(map[string]struct{}),
		defaultEndpoint: &passthroughChannelEndpoint{address: defaultAddress},
		clientFactory:   clientFactory,
		routingConfig:   defaultEndpointRoutingConfig(),
	}
	endpointCacheRegistry.Store(cache, struct{}{})
	return cache
}

func (c *endpointClientCache) setRoutingConfigForTest(cfg endpointRoutingConfig) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.routingConfig = cfg.normalize()
}

// Get returns a channelEndpoint for the given address, creating a new gRPC
// connection if one does not already exist. Channel creation is coordinated
// per address so slow dials do not block unrelated cache access.
func (c *endpointClientCache) Get(ctx context.Context, address string) channelEndpoint {
	if ctx == nil {
		ctx = context.Background()
	}
	if address == c.defaultEndpoint.Address() {
		return c.defaultEndpoint
	}
	// Fast path: read lock.
	c.mu.RLock()
	if ep, ok := c.endpoints[address]; ok {
		c.mu.RUnlock()
		return ep
	}
	c.mu.RUnlock()

	c.mu.Lock()
	if ep, ok := c.endpoints[address]; ok {
		c.mu.Unlock()
		return ep
	}
	if c.closed {
		c.mu.Unlock()
		return nil
	}
	if creation, ok := c.inflight[address]; ok {
		c.mu.Unlock()
		select {
		case <-creation.done:
			return creation.ep
		case <-ctx.Done():
			return nil
		}
	}
	creation := &endpointClientCreation{done: make(chan struct{})}
	c.inflight[address] = creation
	c.mu.Unlock()

	client, err := c.clientFactory(ctx, address)

	c.mu.Lock()
	delete(c.inflight, address)
	_, wasEvicted := c.evicted[address]
	delete(c.evicted, address)
	if err == nil && !c.closed && !wasEvicted {
		ep := &grpcChannelEndpoint{
			address: address,
			client:  client,
			conn:    client.Connection(),
		}
		c.endpoints[address] = ep
		creation.ep = ep
	}
	shouldCloseClient := (c.closed || wasEvicted) && client != nil
	close(creation.done)
	c.mu.Unlock()

	if shouldCloseClient {
		_ = client.Close()
	}
	return creation.ep
}

// GetIfPresent returns a cached endpoint without creating a new gRPC connection.
func (c *endpointClientCache) GetIfPresent(address string) channelEndpoint {
	if address == c.defaultEndpoint.Address() {
		return c.defaultEndpoint
	}
	c.mu.RLock()
	defer c.mu.RUnlock()
	ep, ok := c.endpoints[address]
	if !ok {
		return nil
	}
	return ep
}

func (c *endpointClientCache) addresses() []string {
	c.mu.RLock()
	defer c.mu.RUnlock()
	addresses := make([]string, 0, len(c.endpoints))
	for address := range c.endpoints {
		addresses = append(addresses, address)
	}
	return addresses
}

// Evict removes a cached endpoint and closes its underlying client.
func (c *endpointClientCache) Evict(address string) {
	if address == c.defaultEndpoint.Address() {
		return
	}

	c.mu.Lock()
	ep := c.endpoints[address]
	if ep != nil {
		delete(c.endpoints, address)
	} else if _, ok := c.inflight[address]; ok {
		c.evicted[address] = struct{}{}
	}
	c.mu.Unlock()

	if ep != nil && ep.client != nil {
		_ = ep.client.Close()
	}
}

func (c *endpointClientCache) DefaultChannel() channelEndpoint {
	return c.defaultEndpoint
}

func (c *endpointClientCache) isCoolingDown(address string) bool {
	if address == "" {
		return false
	}
	c.mu.RLock()
	ep := c.endpoints[address]
	c.mu.RUnlock()
	if ep == nil {
		return false
	}
	return ep.isCoolingDown(c.routingConfig)
}

func (c *endpointClientCache) remainingCooldown(address string) time.Duration {
	if address == "" {
		return 0
	}
	c.mu.RLock()
	ep := c.endpoints[address]
	c.mu.RUnlock()
	if ep == nil {
		return 0
	}
	return ep.remainingCooldown(c.routingConfig)
}

func (c *endpointClientCache) recordFailure(address string) {
	if address == "" {
		return
	}
	c.mu.RLock()
	ep := c.endpoints[address]
	c.mu.RUnlock()
	if ep == nil {
		return
	}
	ep.recordFailure(c.routingConfig)
}

func (c *endpointClientCache) selectionCost(operationUID uint64, preferLeader bool, endpoint channelEndpoint, address string) float64 {
	if gep, ok := endpoint.(*grpcChannelEndpoint); ok && gep != nil {
		return gep.selectionCost(c.routingConfig, operationUID, preferLeader)
	}
	c.mu.RLock()
	ep := c.endpoints[address]
	c.mu.RUnlock()
	if ep == nil {
		return endpointLatencyDefaultPenaltyValue
	}
	return ep.selectionCost(c.routingConfig, operationUID, preferLeader)
}

func (c *endpointClientCache) recordLatency(operationUID uint64, preferLeader bool, address string, latency time.Duration) {
	if operationUID == 0 || address == "" {
		return
	}
	c.mu.RLock()
	ep := c.endpoints[address]
	c.mu.RUnlock()
	if ep == nil {
		return
	}
	ep.recordLatency(c.routingConfig, operationUID, preferLeader, latency)
}

func (c *endpointClientCache) recordError(operationUID uint64, preferLeader bool, address string) {
	if operationUID == 0 || address == "" {
		return
	}
	c.mu.RLock()
	ep := c.endpoints[address]
	c.mu.RUnlock()
	if ep == nil {
		return
	}
	ep.recordError(c.routingConfig, operationUID, preferLeader)
}

func (c *endpointClientCache) hasScore(operationUID uint64, preferLeader bool, address string) bool {
	if operationUID == 0 || address == "" {
		return false
	}
	c.mu.RLock()
	ep := c.endpoints[address]
	c.mu.RUnlock()
	if ep == nil {
		return false
	}
	return ep.hasScore(c.routingConfig, operationUID, preferLeader)
}

func (c *endpointClientCache) pruneStaleRoutingState(maxAge time.Duration) {
	if maxAge <= 0 {
		return
	}
	c.mu.RLock()
	endpoints := make([]*grpcChannelEndpoint, 0, len(c.endpoints))
	for _, ep := range c.endpoints {
		endpoints = append(endpoints, ep)
	}
	c.mu.RUnlock()
	for _, ep := range endpoints {
		ep.pruneRoutingState(c.routingConfig, maxAge)
	}
}

func (c *endpointClientCache) clearLatencyScores() {
	c.mu.RLock()
	endpoints := make([]*grpcChannelEndpoint, 0, len(c.endpoints))
	for _, ep := range c.endpoints {
		endpoints = append(endpoints, ep)
	}
	c.mu.RUnlock()
	for _, ep := range endpoints {
		ep.clearScores()
	}
}

func (c *endpointClientCache) routingStateSnapshot(address string) (endpointRoutingStateSnapshot, bool) {
	if address == "" {
		return endpointRoutingStateSnapshot{}, false
	}
	c.mu.RLock()
	ep := c.endpoints[address]
	cfg := c.routingConfig
	c.mu.RUnlock()
	if ep == nil {
		return endpointRoutingStateSnapshot{}, false
	}
	cfg = cfg.normalize()
	now := cfg.now()

	ep.stateMu.Lock()
	defer ep.stateMu.Unlock()
	if !ep.lastFailureAt.IsZero() && !ep.cooldownUntil.After(now) && now.Sub(ep.lastFailureAt) >= cfg.resetAfter {
		ep.resetCooldownLocked()
		return endpointRoutingStateSnapshot{}, false
	}
	if ep.lastFailureAt.IsZero() && ep.cooldownUntil.IsZero() && ep.consecutiveFailures == 0 {
		return endpointRoutingStateSnapshot{}, false
	}
	return endpointRoutingStateSnapshot{
		consecutiveFailures: ep.consecutiveFailures,
		cooldownUntil:       ep.cooldownUntil,
		lastFailureAt:       ep.lastFailureAt,
	}, true
}

// ClientFor resolves a channelEndpoint to the underlying spannerClient.
func (c *endpointClientCache) ClientFor(ep channelEndpoint) spannerClient {
	if ep == nil {
		return nil
	}
	gep, ok := ep.(*grpcChannelEndpoint)
	if !ok {
		return nil
	}
	return gep.client
}

// Close shuts down all cached gRPC connections.
func (c *endpointClientCache) Close() error {
	c.mu.Lock()
	c.closed = true
	defer c.mu.Unlock()
	endpointCacheRegistry.Delete(c)
	var firstErr error
	for addr, ep := range c.endpoints {
		if ep.client != nil {
			if err := ep.client.Close(); err != nil && firstErr == nil {
				firstErr = err
			}
		}
		delete(c.endpoints, addr)
	}
	return firstErr
}

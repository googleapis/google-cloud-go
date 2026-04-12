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
	"time"

	"google.golang.org/grpc/connectivity"
)

const (
	defaultLifecycleProbeInterval  = time.Minute
	defaultIdleEvictionDuration    = 30 * time.Minute
	lifecycleEvictionCheckInterval = 5 * time.Minute
	maxTransientFailureProbeCount  = 3
)

type lifecycleEvictionReason int

const (
	lifecycleEvictionReasonTransientFailure lifecycleEvictionReason = iota
	lifecycleEvictionReasonShutdown
	lifecycleEvictionReasonIdle
	lifecycleEvictionReasonStale
)

type endpointLifecycleState struct {
	address                      string
	lastProbeAt                  time.Time
	lastRealTrafficAt            time.Time
	lastReadyAt                  time.Time
	consecutiveTransientFailures int
	needsCreate                  bool
}

type endpointLifecycleManager struct {
	endpointCache          channelEndpointCache
	defaultEndpointAddress string
	probeInterval          time.Duration
	idleEvictionDuration   time.Duration
	now                    func() time.Time

	mu                               sync.Mutex
	endpoints                        map[string]*endpointLifecycleState
	transientFailureEvictedAddresses map[string]struct{}
	shutdownOnce                     sync.Once
	stopped                          bool

	workCh chan struct{}
	stopCh chan struct{}
	doneCh chan struct{}
}

func newEndpointLifecycleManager(endpointCache channelEndpointCache) *endpointLifecycleManager {
	return newEndpointLifecycleManagerWithOptions(
		endpointCache,
		defaultLifecycleProbeInterval,
		defaultIdleEvictionDuration,
		time.Now,
	)
}

func newEndpointLifecycleManagerWithOptions(
	endpointCache channelEndpointCache,
	probeInterval time.Duration,
	idleEvictionDuration time.Duration,
	now func() time.Time,
) *endpointLifecycleManager {
	if endpointCache == nil {
		endpointCache = newPassthroughChannelEndpointCache()
	}
	if probeInterval <= 0 {
		probeInterval = defaultLifecycleProbeInterval
	}
	if idleEvictionDuration <= 0 {
		idleEvictionDuration = defaultIdleEvictionDuration
	}
	if now == nil {
		now = time.Now
	}

	manager := &endpointLifecycleManager{
		endpointCache:                    endpointCache,
		defaultEndpointAddress:           endpointCache.DefaultChannel().Address(),
		probeInterval:                    probeInterval,
		idleEvictionDuration:             idleEvictionDuration,
		now:                              now,
		endpoints:                        make(map[string]*endpointLifecycleState),
		transientFailureEvictedAddresses: make(map[string]struct{}),
		workCh:                           make(chan struct{}, 1),
		stopCh:                           make(chan struct{}),
		doneCh:                           make(chan struct{}),
	}
	go manager.run()
	return manager
}

func (m *endpointLifecycleManager) run() {
	defer close(m.doneCh)

	probeTicker := time.NewTicker(m.probeInterval)
	defer probeTicker.Stop()

	evictionTicker := time.NewTicker(lifecycleEvictionCheckInterval)
	defer evictionTicker.Stop()

	for {
		select {
		case <-m.workCh:
			m.processPendingCreations()
		case <-probeTicker.C:
			m.processPendingCreations()
			m.probeManagedEndpoints()
		case <-evictionTicker.C:
			m.checkIdleEviction()
		case <-m.stopCh:
			return
		}
	}
}

func (m *endpointLifecycleManager) signalWork() {
	select {
	case m.workCh <- struct{}{}:
	default:
	}
}

func (m *endpointLifecycleManager) recordRealTraffic(address string) {
	if m == nil || address == "" || address == m.defaultEndpointAddress {
		return
	}

	now := m.now()

	m.mu.Lock()
	if m.stopped {
		m.mu.Unlock()
		return
	}
	state, ok := m.endpoints[address]
	if !ok {
		state = &endpointLifecycleState{
			address:           address,
			lastRealTrafficAt: now,
			needsCreate:       true,
		}
		m.endpoints[address] = state
		m.mu.Unlock()
		m.signalWork()
		return
	}
	state.lastRealTrafficAt = now
	m.mu.Unlock()

	if m.endpointCache.GetIfPresent(address) == nil {
		m.mu.Lock()
		if state = m.endpoints[address]; state != nil && !m.stopped {
			state.needsCreate = true
		}
		m.mu.Unlock()
		m.signalWork()
	}
}

func (m *endpointLifecycleManager) requestEndpointRecreation(address string) {
	if m == nil || address == "" || address == m.defaultEndpointAddress {
		return
	}

	now := m.now()

	m.mu.Lock()
	if m.stopped {
		m.mu.Unlock()
		return
	}
	state, ok := m.endpoints[address]
	if !ok {
		state = &endpointLifecycleState{
			address:           address,
			lastRealTrafficAt: now,
		}
		m.endpoints[address] = state
	}
	state.needsCreate = true
	m.mu.Unlock()

	m.signalWork()
}

func (m *endpointLifecycleManager) processPendingCreations() {
	if m == nil {
		return
	}

	addresses := m.pendingCreationAddresses()
	for _, address := range addresses {
		endpoint := m.endpointCache.Get(context.Background(), address)
		if endpoint == nil {
			m.mu.Lock()
			if state := m.endpoints[address]; state != nil && !m.stopped {
				state.needsCreate = true
			}
			m.mu.Unlock()
			continue
		}

		m.mu.Lock()
		_, stillManaged := m.endpoints[address]
		m.mu.Unlock()
		if !stillManaged {
			m.endpointCache.Evict(address)
		}
	}
}

func (m *endpointLifecycleManager) pendingCreationAddresses() []string {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.stopped {
		return nil
	}

	addresses := make([]string, 0, len(m.endpoints))
	for address, state := range m.endpoints {
		if !state.needsCreate {
			continue
		}
		state.needsCreate = false
		addresses = append(addresses, address)
	}
	return addresses
}

func (m *endpointLifecycleManager) probeManagedEndpoints() {
	if m == nil {
		return
	}

	for _, address := range m.managedAddresses() {
		m.probe(address)
	}
}

func (m *endpointLifecycleManager) managedAddresses() []string {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.stopped {
		return nil
	}

	addresses := make([]string, 0, len(m.endpoints))
	for address := range m.endpoints {
		addresses = append(addresses, address)
	}
	return addresses
}

func (m *endpointLifecycleManager) probe(address string) {
	endpoint := m.endpointCache.GetIfPresent(address)
	if endpoint == nil {
		return
	}

	conn := endpoint.GetConn()
	if conn == nil {
		return
	}

	now := m.now()
	state := conn.GetState()

	m.mu.Lock()
	lifecycleState, ok := m.endpoints[address]
	if !ok || m.stopped {
		m.mu.Unlock()
		return
	}
	lifecycleState.lastProbeAt = now

	switch state {
	case connectivity.Ready:
		lifecycleState.lastReadyAt = now
		lifecycleState.consecutiveTransientFailures = 0
		delete(m.transientFailureEvictedAddresses, address)
		m.mu.Unlock()
		return
	case connectivity.Idle:
		lifecycleState.consecutiveTransientFailures = 0
		m.mu.Unlock()
		conn.Connect()
		return
	case connectivity.Connecting:
		lifecycleState.consecutiveTransientFailures = 0
		m.mu.Unlock()
		return
	case connectivity.TransientFailure:
		lifecycleState.consecutiveTransientFailures++
		evict := lifecycleState.consecutiveTransientFailures >= maxTransientFailureProbeCount
		m.mu.Unlock()
		if evict {
			m.evictEndpoint(address, lifecycleEvictionReasonTransientFailure)
		}
		return
	case connectivity.Shutdown:
		m.mu.Unlock()
		m.evictEndpoint(address, lifecycleEvictionReasonShutdown)
		return
	default:
		m.mu.Unlock()
		return
	}
}

func (m *endpointLifecycleManager) checkIdleEviction() {
	if m == nil {
		return
	}

	now := m.now()
	var toEvict []string

	m.mu.Lock()
	if m.stopped {
		m.mu.Unlock()
		return
	}
	for address, state := range m.endpoints {
		if address == m.defaultEndpointAddress {
			continue
		}
		if now.Sub(state.lastRealTrafficAt) > m.idleEvictionDuration {
			toEvict = append(toEvict, address)
		}
	}
	m.mu.Unlock()

	for _, address := range toEvict {
		m.evictEndpoint(address, lifecycleEvictionReasonIdle)
	}
}

func (m *endpointLifecycleManager) evictEndpoint(address string, reason lifecycleEvictionReason) {
	if m == nil || address == "" || address == m.defaultEndpointAddress {
		return
	}

	m.mu.Lock()
	if m.stopped {
		m.mu.Unlock()
		return
	}
	if _, ok := m.endpoints[address]; !ok {
		m.mu.Unlock()
		return
	}
	delete(m.endpoints, address)
	if reason == lifecycleEvictionReasonTransientFailure {
		m.transientFailureEvictedAddresses[address] = struct{}{}
	} else {
		delete(m.transientFailureEvictedAddresses, address)
	}
	m.mu.Unlock()

	m.endpointCache.Evict(address)
}

func (m *endpointLifecycleManager) isManaged(address string) bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	_, ok := m.endpoints[address]
	return ok
}

func (m *endpointLifecycleManager) wasRecentlyEvictedTransientFailure(address string) bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	_, ok := m.transientFailureEvictedAddresses[address]
	return ok
}

func (m *endpointLifecycleManager) getEndpointState(address string) (endpointLifecycleState, bool) {
	m.mu.Lock()
	defer m.mu.Unlock()

	state, ok := m.endpoints[address]
	if !ok {
		return endpointLifecycleState{}, false
	}
	return *state, true
}

func (m *endpointLifecycleManager) managedEndpointCount() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return len(m.endpoints)
}

func (m *endpointLifecycleManager) shutdown() {
	if m == nil {
		return
	}

	m.shutdownOnce.Do(func() {
		m.mu.Lock()
		m.stopped = true
		m.endpoints = make(map[string]*endpointLifecycleState)
		m.transientFailureEvictedAddresses = make(map[string]struct{})
		m.mu.Unlock()

		close(m.stopCh)
		<-m.doneCh
	})
}

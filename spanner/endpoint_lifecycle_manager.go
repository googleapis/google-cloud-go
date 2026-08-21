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
	defaultLifecycleCreateTimeout  = 30 * time.Second
)

type lifecycleEvictionReason int

const (
	lifecycleEvictionReasonTransientFailure lifecycleEvictionReason = iota
	lifecycleEvictionReasonShutdown
	lifecycleEvictionReasonIdle
)

type endpointLifecycleState struct {
	lastRealTrafficAt            time.Time
	consecutiveTransientFailures int
}

type lifecycleManagedEndpoint interface {
	channelEndpoint
	recordRealTrafficAt(time.Time)
	resetLifecycleTransientFailures()
	incrementLifecycleTransientFailures() int
	lifecycleStateSnapshot() endpointLifecycleState
}

type endpointAddressProvider interface {
	addresses() []string
}

type endpointLifecycleManager struct {
	endpointCache          channelEndpointCache
	defaultEndpointAddress string
	probeInterval          time.Duration
	idleEvictionDuration   time.Duration
	now                    func() time.Time

	mu                               sync.Mutex
	pendingRecreations               map[string]struct{}
	transientFailureEvictedAddresses map[string]struct{}
	shutdownOnce                     sync.Once
	stopped                          bool

	stopCh chan struct{}
	doneCh chan struct{}

	createCtx    context.Context
	cancelCreate context.CancelFunc
	createWakeCh chan struct{}
	createDoneCh chan struct{}
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
		return nil
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
		pendingRecreations:               make(map[string]struct{}),
		transientFailureEvictedAddresses: make(map[string]struct{}),
		stopCh:                           make(chan struct{}),
		doneCh:                           make(chan struct{}),
		createWakeCh:                     make(chan struct{}, 1),
		createDoneCh:                     make(chan struct{}),
	}
	manager.createCtx, manager.cancelCreate = context.WithCancel(context.Background())
	go manager.run()
	go manager.runCreator()
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
		case <-probeTicker.C:
			m.signalCreator()
			m.probeCachedEndpoints()
		case <-evictionTicker.C:
			m.checkIdleEviction()
		case <-m.stopCh:
			return
		}
	}
}

func (m *endpointLifecycleManager) recordRealTraffic(address string) {
	if m == nil || address == "" || address == m.defaultEndpointAddress {
		return
	}

	endpoint := m.endpointCache.GetIfPresent(address)
	if managed := m.managedEndpoint(endpoint); managed != nil {
		managed.recordRealTrafficAt(m.now())
		return
	}

	m.requestEndpointRecreation(address)
}

func (m *endpointLifecycleManager) requestEndpointRecreation(address string) {
	if m == nil || address == "" || address == m.defaultEndpointAddress {
		return
	}

	m.mu.Lock()
	if m.stopped {
		m.mu.Unlock()
		return
	}
	m.pendingRecreations[address] = struct{}{}
	m.mu.Unlock()

	m.signalCreator()
}

func (m *endpointLifecycleManager) runCreator() {
	defer close(m.createDoneCh)

	for {
		select {
		case <-m.createWakeCh:
		case <-m.stopCh:
			return
		}

		for {
			addresses := m.pendingCreationAddresses()
			if len(addresses) == 0 {
				break
			}
			for _, address := range addresses {
				if !m.createEndpoint(address) {
					select {
					case <-m.stopCh:
						return
					default:
					}
				}
			}
		}
	}
}

func (m *endpointLifecycleManager) signalCreator() {
	if m == nil {
		return
	}
	select {
	case m.createWakeCh <- struct{}{}:
	default:
	}
}

func (m *endpointLifecycleManager) createEndpoint(address string) bool {
	if m == nil || address == "" {
		return true
	}

	ctx, cancel := context.WithTimeout(m.createCtx, defaultLifecycleCreateTimeout)
	defer cancel()

	endpoint := m.endpointCache.Get(ctx, address)
	select {
	case <-m.createCtx.Done():
		return false
	default:
	}
	if endpoint == nil {
		m.requestEndpointRecreation(address)
		return true
	}

	managed := m.managedEndpoint(endpoint)
	m.mu.Lock()
	stopped := m.stopped
	m.mu.Unlock()
	if stopped {
		m.endpointCache.Evict(address)
		return true
	}
	if managed != nil {
		managed.recordRealTrafficAt(m.now())
	}
	return true
}

func (m *endpointLifecycleManager) pendingCreationAddresses() []string {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.stopped {
		return nil
	}

	addresses := make([]string, 0, len(m.pendingRecreations))
	for address := range m.pendingRecreations {
		delete(m.pendingRecreations, address)
		addresses = append(addresses, address)
	}
	return addresses
}

func (m *endpointLifecycleManager) probeCachedEndpoints() {
	if m == nil {
		return
	}

	for _, address := range m.cachedAddresses() {
		m.probe(address)
	}
}

func (m *endpointLifecycleManager) cachedAddresses() []string {
	if m == nil || m.endpointCache == nil {
		return nil
	}
	provider, ok := m.endpointCache.(endpointAddressProvider)
	if !ok {
		return nil
	}
	return provider.addresses()
}

func (m *endpointLifecycleManager) managedEndpoint(endpoint channelEndpoint) lifecycleManagedEndpoint {
	if endpoint == nil {
		return nil
	}
	managed, ok := endpoint.(lifecycleManagedEndpoint)
	if !ok {
		return nil
	}
	return managed
}

func (m *endpointLifecycleManager) probe(address string) {
	if address == "" || address == m.defaultEndpointAddress {
		return
	}

	endpoint := m.endpointCache.GetIfPresent(address)
	managed := m.managedEndpoint(endpoint)
	if managed == nil {
		return
	}

	conn := managed.GetConn()
	if conn == nil {
		return
	}

	// GetState reports grpc-go's current connectivity state; it is not an
	// active liveness probe. A connection that is silently broken can remain
	// Ready until real traffic or keepalive detects the failure. Since the
	// client configures a 2 minute keepalive and this lifecycle probe runs once
	// per minute, an otherwise idle broken connection can take on the order of
	// a few minutes to transition out of Ready.
	state := conn.GetState()

	switch state {
	case connectivity.Ready:
		managed.resetLifecycleTransientFailures()
		m.mu.Lock()
		delete(m.transientFailureEvictedAddresses, address)
		m.mu.Unlock()
		return
	case connectivity.Idle:
		managed.resetLifecycleTransientFailures()
		conn.Connect()
		return
	case connectivity.Connecting:
		return
	case connectivity.TransientFailure:
		if managed.incrementLifecycleTransientFailures() >= maxTransientFailureProbeCount {
			m.evictEndpoint(address, lifecycleEvictionReasonTransientFailure)
		}
		return
	case connectivity.Shutdown:
		m.evictEndpoint(address, lifecycleEvictionReasonShutdown)
		return
	default:
		return
	}
}

func (m *endpointLifecycleManager) checkIdleEviction() {
	if m == nil {
		return
	}

	now := m.now()
	var toEvict []string
	for _, address := range m.cachedAddresses() {
		if address == "" || address == m.defaultEndpointAddress {
			continue
		}
		state, ok := m.getEndpointState(address)
		if !ok || state.lastRealTrafficAt.IsZero() {
			continue
		}
		if now.Sub(state.lastRealTrafficAt) > m.idleEvictionDuration {
			toEvict = append(toEvict, address)
		}
	}

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
	delete(m.pendingRecreations, address)
	if reason == lifecycleEvictionReasonTransientFailure {
		m.transientFailureEvictedAddresses[address] = struct{}{}
	} else {
		delete(m.transientFailureEvictedAddresses, address)
	}
	m.mu.Unlock()

	m.endpointCache.Evict(address)
}

func (m *endpointLifecycleManager) isManaged(address string) bool {
	if m == nil || address == "" || address == m.defaultEndpointAddress {
		return false
	}
	return m.endpointCache.GetIfPresent(address) != nil
}

func (m *endpointLifecycleManager) wasRecentlyEvictedTransientFailure(address string) bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	_, ok := m.transientFailureEvictedAddresses[address]
	return ok
}

func (m *endpointLifecycleManager) getEndpointState(address string) (endpointLifecycleState, bool) {
	if m == nil || address == "" || address == m.defaultEndpointAddress {
		return endpointLifecycleState{}, false
	}
	managed := m.managedEndpoint(m.endpointCache.GetIfPresent(address))
	if managed == nil {
		return endpointLifecycleState{}, false
	}
	return managed.lifecycleStateSnapshot(), true
}

func (m *endpointLifecycleManager) managedEndpointCount() int {
	if m == nil {
		return 0
	}
	count := 0
	for _, address := range m.cachedAddresses() {
		if address == "" || address == m.defaultEndpointAddress {
			continue
		}
		count++
	}
	return count
}

func (m *endpointLifecycleManager) shutdown() {
	if m == nil {
		return
	}

	m.shutdownOnce.Do(func() {
		m.mu.Lock()
		m.stopped = true
		m.mu.Unlock()

		m.cancelCreate()
		close(m.stopCh)
		<-m.doneCh
		<-m.createDoneCh

		m.mu.Lock()
		m.pendingRecreations = make(map[string]struct{})
		m.transientFailureEvictedAddresses = make(map[string]struct{})
		m.mu.Unlock()
	})
}

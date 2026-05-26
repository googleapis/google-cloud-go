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
	"errors"
	"fmt"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	spb "cloud.google.com/go/bigtable/apiv2/bigtablepb"
	"google.golang.org/grpc/metadata"
)

// SessionPoolImpl implements a thread-safe session pool.
type SessionPoolImpl struct {
	mu                 sync.Mutex
	sizer              *PoolSizer
	picker             Picker
	budget             SessionThrottler
	sessions           []*SessionHandle
	startingSessions   map[*Session]bool
	closed             bool
	scalingInProgress  bool
	minSessions        int
	maxSessions        int
	streamFactory      func(ctx context.Context) (Stream, error)
	openSessionRequest *spb.OpenSessionRequest // Target specific stream handshake template
	metadata           metadata.MD             // Pre-computed call metadata headers
	nextSessionID      uint64                  // Monotonically increasing counter for unique session naming
	sessionType        SessionType
	poolName           string
}

// NewSessionPoolImpl creates a new SessionPoolImpl.
func NewSessionPoolImpl(poolName string, min, max int, streamFactory func(ctx context.Context) (Stream, error), openSessionRequest *spb.OpenSessionRequest, md metadata.MD, sessionType SessionType) *SessionPoolImpl {
	pool := &SessionPoolImpl{
		poolName:           poolName,
		minSessions:        min,
		maxSessions:        max,
		streamFactory:      streamFactory,
		openSessionRequest: openSessionRequest,
		metadata:           md,
		startingSessions:   make(map[*Session]bool),
		sessionType:        sessionType,
	}

	fetcher := func() *PoolStats {
		return pool.Stats()
	}
	pool.sizer = NewPoolSizer(fetcher, min, max, 0.10)
	pool.picker = NewRandomPicker(pool.sessions)
	pool.budget = NewAdaptiveSessionThrottler(10, 10*time.Second)

	return pool
}

// CheckoutSession retrieves a session from the pool for routing requests.
func (p *SessionPoolImpl) CheckoutSession(ctx context.Context) (*SessionHandle, error) {
	// Triggers scaling immediately if we might be short of sessions
	p.mu.Lock()
	if !p.closed && len(p.sessions) == 0 {
		fmt.Printf(">>> POOL %s: all sessions busy, trying to create new session <<<\n", p.poolName)
		go p.PerformScaling(ctx)
	}
	p.mu.Unlock()

	ticker := time.NewTicker(15 * time.Millisecond)
	defer ticker.Stop()

	for {
		p.mu.Lock()
		if p.closed {
			p.mu.Unlock()
			return nil, errors.New("session pool is closed")
		}

		sh := p.picker.PickSession()
		if sh != nil {
			if sh.session.State() == StateActive {
				sh.IncOutstanding()
				p.mu.Unlock()
				return sh, nil
			}
			// Session is not active anymore. Remove it immediately from pool sessions
			idx := -1
			for i, sHandle := range p.sessions {
				if sHandle == sh {
					idx = i
					break
				}
			}
			if idx != -1 {
				p.sessions = append(p.sessions[:idx], p.sessions[idx+1:]...)
				p.picker = NewRandomPicker(p.sessions)
			}
			// Trigger scale up immediately to replace the dead session
			go p.PerformScaling(ctx)
		}

		p.mu.Unlock()

		select {
		case <-ctx.Done():
			return nil, fmt.Errorf("no active sessions available: %w", ctx.Err())
		case <-ticker.C:
		}
	}
}

// Stats returns the current operational statistics of the session pool.
func (p *SessionPoolImpl) Stats() *PoolStats {
	p.mu.Lock()
	defer p.mu.Unlock()

	ready := 0
	inUse := 0
	totalOutstanding := 0
	for _, sh := range p.sessions {
		if sh.session.State() == StateActive {
			ready++
		}
		outstanding := atomic.LoadInt64(&sh.outstanding)
		if outstanding > 0 {
			inUse++
			totalOutstanding += int(outstanding)
		}
	}

	return &PoolStats{
		ReadyCount:    ready,
		InUseCount:    inUse,
		StartingCount: len(p.startingSessions),
		PendingCount:  totalOutstanding,
	}
}

// Close gracefully closes all active sessions in the pool.
func (p *SessionPoolImpl) Close() error {
	p.mu.Lock()
	defer p.mu.Unlock()

	if p.closed {
		return nil
	}
	p.closed = true

	for _, sh := range p.sessions {
		if sh.session != nil {
			go sh.session.Close(&spb.CloseSessionRequest{
				Reason:      spb.CloseSessionRequest_CLOSE_SESSION_REASON_USER,
				Description: "graceful pool teardown",
			})
		}
	}
	p.sessions = nil
	return nil
}

// OnStart is a no-op callback for session start.
func (p *SessionPoolImpl) OnStart(ctx context.Context) {}

// OnActive is triggered when a background session finishes its open session req and becomes active.
// The session is wrapped inside a SessionHandle and registered into the ready sessions list!
func (p *SessionPoolImpl) OnActive(s *Session) {
	p.mu.Lock()
	defer p.mu.Unlock()

	delete(p.startingSessions, s)

	if p.closed {
		s.ForceClose(&spb.CloseSessionRequest{
			Reason:      spb.CloseSessionRequest_CLOSE_SESSION_REASON_ERROR,
			Description: "pool closed before session became active",
		})
		return
	}

	// Ensure we do not duplicate register the same session!
	for _, sh := range p.sessions {
		if sh.session == s {
			return
		}
	}

	sh := NewSessionHandle(s)
	p.sessions = append(p.sessions, sh)

	// Re-initialize picker with updated sessions list
	p.picker = NewRandomPicker(p.sessions)
}

// OnClose removes the closed session from the active sessions list and updates the picker.
func (p *SessionPoolImpl) OnClose(s *Session, err error) {
	p.mu.Lock()
	defer p.mu.Unlock()

	if _, starting := p.startingSessions[s]; starting {
		delete(p.startingSessions, s)
		return
	}

	idx := -1
	for i, sh := range p.sessions {
		if sh.session == s {
			idx = i
			break
		}
	}

	if idx != -1 {
		// Remove session handle from slice
		p.sessions = append(p.sessions[:idx], p.sessions[idx+1:]...)
		// Re-initialize picker with updated active sessions
		p.picker = NewRandomPicker(p.sessions)
		// Trigger scale up evaluation asynchronously immediately!
		go p.PerformScaling(context.Background())
	}
}

// UpdateConfig dynamically adjusts the pool size constraints and budget governor limits at runtime.
func (p *SessionPoolImpl) UpdateConfig(config *spb.SessionClientConfiguration_SessionPoolConfiguration) {
	p.mu.Lock()
	p.minSessions = int(config.MinSessionCount)
	p.maxSessions = int(config.MaxSessionCount)
	fmt.Printf(">>> SessionPool %p UpdateConfig: minSessions=%d, maxSessions=%d <<<\n", p, p.minSessions, p.maxSessions)

	if config.LoadBalancingOptions != nil {
		lbo := config.LoadBalancingOptions
		switch opt := lbo.LoadBalancingStrategy.(type) {
		case *spb.LoadBalancingOptions_Random_:
			p.picker = NewRandomPicker(p.sessions)
		case *spb.LoadBalancingOptions_LeastInFlight_:
			p.picker = NewRoundRobinPicker(p.sessions)
		case *spb.LoadBalancingOptions_PeakEwma_:
			subsetSize := 2
			if opt.PeakEwma != nil {
				subsetSize = int(opt.PeakEwma.RandomSubsetSize)
			}
			p.picker = NewPeakEwmaPicker(p.sessions, subsetSize)
		}
	}
	p.mu.Unlock()

	// Dynamically update sizer thresholds E2E!
	p.sizer.UpdateConfig(config)
}

// StartHeartbeat begins the background scaling watchdog evaluation loop.
func (p *SessionPoolImpl) StartHeartbeat(ctx context.Context, interval time.Duration) {
	go func() {
		ticker := time.NewTicker(interval)
		defer ticker.Stop()

		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				p.PerformScaling(ctx)
			}
		}
	}()
}

func (p *SessionPoolImpl) PerformScaling(ctx context.Context) {
	p.mu.Lock()
	if p.closed || p.scalingInProgress {
		p.mu.Unlock()
		return
	}
	p.scalingInProgress = true
	p.mu.Unlock()

	defer func() {
		p.mu.Lock()
		p.scalingInProgress = false
		p.mu.Unlock()
	}()

	stats := p.Stats()
	fmt.Printf(">>> POOL %s STATS: Ready=%d, Starting=%d, InUse=%d, PendingOutstanding=%d <<<\n",
		p.poolName, stats.ReadyCount, stats.StartingCount, stats.InUseCount, stats.PendingCount)

	delta := p.sizer.GetScaleDelta()
	if delta == 0 {
		return
	}

	p.mu.Lock()
	currentSessions := len(p.sessions)
	startingSessions := len(p.startingSessions)
	p.mu.Unlock()

	fmt.Printf(">>> POOL %s PerformScaling starting evaluation: delta=%d, current_sessions=%d, starting_sessions=%d <<<\n", p.poolName, delta, currentSessions, startingSessions)

	if delta > 0 {
		// Scale up: provision new sessions asynchronously and wait for completion to release the gate
		var wg sync.WaitGroup
		for i := 0; i < delta; i++ {
			wg.Add(1)
			go func() {
				defer wg.Done()
				if err := p.createSession(ctx); err != nil {
					fmt.Printf(">>> POOL %s PerformScaling createSession failed: %v <<<\n", p.poolName, err)
				} else {
					fmt.Printf(">>> POOL %s PerformScaling successfully provisioned a new session <<<\n", p.poolName)
				}
			}()
		}
		wg.Wait()
	} else {
		// Scale down: prune idle sessions gracefully
		fmt.Printf(">>> POOL %s PerformScaling pruning %d idle sessions <<<\n", p.poolName, -delta)
		p.pruneSessions(-delta)
	}
}

func (p *SessionPoolImpl) createSession(ctx context.Context) error {
	// Strip client deadline to prevent gRPC Bidi stream from having a user-set timeout
	dialCtx := noDeadlineContext{Context: ctx}

	// Acquire a token from the concurrency governor budget before dialing!
	if err := p.budget.Acquire(dialCtx); err != nil {
		return fmt.Errorf("failed to acquire session creation budget: %w", err)
	}

	success := false
	defer func() {
		p.budget.Release(success) // Release budget registering success/failure penalty token!
	}()

	// Inject the pre-computed target metadata headers context-safely E2E!
	dialCtxOut := metadata.NewOutgoingContext(dialCtx, p.metadata)
	stream, err := p.streamFactory(dialCtxOut)
	if err != nil {
		return err
	}

	// Determine session name and check limits briefly under lock
	p.mu.Lock()
	if p.closed {
		p.mu.Unlock()
		return fmt.Errorf("session pool is closed")
	}
	if len(p.sessions) >= p.maxSessions {
		p.mu.Unlock()
		return fmt.Errorf("session pool limit reached")
	}
	id := atomic.AddUint64(&p.nextSessionID, 1)
	role := "session"
	if strings.HasSuffix(p.poolName, ":read") {
		role = "read"
	} else if strings.HasSuffix(p.poolName, ":write") {
		role = "write"
	}
	sessionName := fmt.Sprintf("session-%s-%d", role, id)
	p.mu.Unlock()

	// Create and start new session wrapper passing pool pointer as the lifecycle listener!
	s := NewSession(sessionName, stream, p, p.sessionType)

	p.mu.Lock()
	p.startingSessions[s] = true
	p.mu.Unlock()

	if err := s.Start(dialCtx, p.openSessionRequest, nil); err != nil {
		p.mu.Lock()
		delete(p.startingSessions, s)
		p.mu.Unlock()
		fmt.Printf(">>> POOL %p createSession Start failed for %s: %v <<<\n", p, sessionName, err)
		return fmt.Errorf("failed to start session: %w", err)
	}

	success = true
	return nil
}

func (p *SessionPoolImpl) pruneSessions(count int) {
	p.mu.Lock()
	defer p.mu.Unlock()

	if p.closed {
		return
	}

	pruned := 0
	var active []*SessionHandle
	for _, sh := range p.sessions {
		if pruned < count && atomic.LoadInt64(&sh.outstanding) == 0 {
			// Prune this session by triggering full graceful close asynchronously
			if sh.session != nil {
				go sh.session.Close(&spb.CloseSessionRequest{
					Reason:      spb.CloseSessionRequest_CLOSE_SESSION_REASON_DOWNSIZE,
					Description: "prune session downsize",
				})
			}
			pruned++
		} else {
			active = append(active, sh)
		}
	}

	p.sessions = active
	p.picker = NewRandomPicker(p.sessions)
}

type noDeadlineContext struct {
	context.Context
}

func (noDeadlineContext) Deadline() (deadline time.Time, ok bool) {
	return time.Time{}, false
}

func (noDeadlineContext) Done() <-chan struct{} {
	return nil
}

func (noDeadlineContext) Err() error {
	return nil
}

// ExecuteVRpc checks out a session, executes a virtual RPC request, and manages session outstanding counts.
func (p *SessionPoolImpl) ExecuteVRpc(ctx context.Context, desc VRpcDescriptor, req interface{}) (resp interface{}, clInfo *spb.ClusterInformation, err error) {
	sh, err := p.CheckoutSession(ctx)
	if err != nil {
		return nil, nil, err
	}
	start := time.Now()
	defer func() {
		duration := time.Since(start)
		sh.DecOutstanding(duration)
	}()

	return sh.session.ExecuteVRpc(ctx, desc, req)
}

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
	"math/rand"
	"sync"
	"sync/atomic"
	"time"

	spb "cloud.google.com/go/bigtable/apiv2/bigtablepb"
)

// RoundRobinPicker picks sessions in a round-robin sequence.
type RoundRobinPicker struct {
	mu       sync.Mutex
	sessions []*SessionHandle
	next     uint32
}

// NewRoundRobinPicker creates a new RoundRobinPicker.
func NewRoundRobinPicker(sessions []*SessionHandle) *RoundRobinPicker {
	return &RoundRobinPicker{
		sessions: sessions,
	}
}

// PickSession selects the next session sequentially.
func (p *RoundRobinPicker) PickSession() *SessionHandle {
	p.mu.Lock()
	defer p.mu.Unlock()
	if len(p.sessions) == 0 {
		return nil
	}
	sh := p.sessions[p.next%uint32(len(p.sessions))]
	p.next++
	return sh
}

// PeakEwmaPicker picks sessions based on outstanding request count and EWMA latency.
type PeakEwmaPicker struct {
	mu               sync.Mutex
	sessions         []*SessionHandle
	randomSubsetSize int
	rng              *rand.Rand
}

// NewPeakEwmaPicker creates a new PeakEwmaPicker.
func NewPeakEwmaPicker(sessions []*SessionHandle, randomSubsetSize int) *PeakEwmaPicker {
	if randomSubsetSize <= 0 {
		randomSubsetSize = 2 // Default to 2-choice randomized selection
	}
	return &PeakEwmaPicker{
		sessions:         sessions,
		randomSubsetSize: randomSubsetSize,
		rng:              rand.New(rand.NewSource(time.Now().UnixNano())),
	}
}

// PickSession selects a session using the Peak EWMA least-cost algorithm.
func (p *PeakEwmaPicker) PickSession() *SessionHandle {
	p.mu.Lock()
	defer p.mu.Unlock()
	if len(p.sessions) == 0 {
		return nil
	}

	// If the subset size is larger than the number of sessions, scan all sessions
	subsetSize := p.randomSubsetSize
	if subsetSize >= len(p.sessions) {
		return p.pickMinCost(p.sessions)
	}

	// Randomized K-choice selection
	choices := make([]*SessionHandle, subsetSize)
	for i := 0; i < subsetSize; i++ {
		idx := p.rng.Intn(len(p.sessions))
		choices[i] = p.sessions[idx]
	}

	return p.pickMinCost(choices)
}

func (p *PeakEwmaPicker) pickMinCost(choices []*SessionHandle) *SessionHandle {
	var minSH *SessionHandle
	minCost := -1.0

	for _, sh := range choices {
		cost := p.getSessionCost(sh)
		if minCost < 0 || cost < minCost {
			minCost = cost
			minSH = sh
		}
	}
	return minSH
}

func (p *PeakEwmaPicker) getSessionCost(sh *SessionHandle) float64 {
	outstanding := atomic.LoadInt64(&sh.outstanding)
	val := 1.0
	if sh.ewma != nil {
		ewmaVal := sh.ewma.Value()
		if ewmaVal > 0 {
			val = ewmaVal
		}
	}
	return float64(outstanding+1) * val
}

// SessionHandle wraps a Session.
type SessionHandle struct {
	session      *Session
	outstanding  int64
	ewma         *PeakEwma
	lastActivity int64 // UnixNano timestamp of the last completed call
}

// NewSessionHandle creates a new SessionHandle wrapping a Session.
func NewSessionHandle(session *Session) *SessionHandle {
	return &SessionHandle{
		session: session,
		ewma:    NewPeakEwma(10 * time.Second),
	}
}

// IncOutstanding increments outstanding calls.
func (h *SessionHandle) IncOutstanding() {
	atomic.AddInt64(&h.outstanding, 1)
}

// DecOutstanding decrements outstanding calls and updates EWMA latency + lastActivity timestamp.
func (h *SessionHandle) DecOutstanding(latency time.Duration) {
	atomic.AddInt64(&h.outstanding, -1)
	if h.ewma != nil && latency > 0 {
		h.ewma.Update(latency)
	}
	atomic.StoreInt64(&h.lastActivity, time.Now().UnixNano())
}

// GetLastActivity returns the time of the last activity.
func (h *SessionHandle) GetLastActivity() time.Time {
	nano := atomic.LoadInt64(&h.lastActivity)
	if nano == 0 {
		return time.Time{}
	}
	return time.Unix(0, nano)
}

// ExecuteVRpc delegates the virtual RPC execution to the underlying session.
func (h *SessionHandle) ExecuteVRpc(ctx context.Context, desc VRpcDescriptor, req interface{}) (interface{}, *spb.ClusterInformation, error) {
	return h.session.ExecuteVRpc(ctx, desc, req)
}

// Picker defines the interface for picking a session from a pool.
type Picker interface {
	PickSession() *SessionHandle
}

// RandomPicker picks a session randomly from a list of sessions.
type RandomPicker struct {
	mu       sync.Mutex
	sessions []*SessionHandle
	rng      *rand.Rand
}

// NewRandomPicker creates a new RandomPicker.
func NewRandomPicker(sessions []*SessionHandle) *RandomPicker {
	return &RandomPicker{
		sessions: sessions,
		rng:      rand.New(rand.NewSource(time.Now().UnixNano())),
	}
}

// PickSession selects a session randomly.
func (p *RandomPicker) PickSession() *SessionHandle {
	p.mu.Lock()
	defer p.mu.Unlock()
	if len(p.sessions) == 0 {
		return nil
	}
	idx := p.rng.Intn(len(p.sessions))
	return p.sessions[idx]
}

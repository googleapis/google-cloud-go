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

package session

import (
	"context"
	"sync"
	"sync/atomic"

	btransport "cloud.google.com/go/bigtable/internal/transport"
)

// Invoker is the narrow surface sessionTable needs from a session
// pool: dispatch a single virtual RPC and surface the full
// InvokeResult (response, cluster info, server-side Stats, and the
// local SentAt timestamp). Satisfied by *btransport.SessionPoolImpl;
// the interface exists so tests can substitute a fake without
// standing up a real pool.
type Invoker interface {
	Invoke(ctx context.Context, desc btransport.VRpcDescriptor, req interface{}) (btransport.InvokeResult, error)
}

// lazyPool wraps an Invoker (typically *btransport.SessionPoolImpl)
// that is opened on first use. Callers invoke get(); the first
// winner runs the open closure (synchronously — dial + handshake
// happens here) and stores the result; subsequent callers see the
// stored pool with no work.
//
// Failed opens are NOT cached: the next caller retries. This
// matters because pool creation can fail transiently (proto.Marshal
// is the only obvious source today, but future descriptor variants
// may add more). A permanent error-cache would leave the sessionTable
// stuck for the process lifetime.
//
// A nil *lazyPool or one with a nil open closure returns (nil, nil)
// — "no session support, use fallback." Used for the write side of
// materialized views (read-only).
type lazyPool struct {
	mu   sync.Mutex
	pool Invoker
	open func() (Invoker, error)
	// openedFlag mirrors "pool != nil" but is readable WITHOUT mu.
	// get() holds mu for the whole open() (a dial that can block for
	// seconds), so a mu-guarded opened() would make any concurrent
	// reader — notably sessionTable.Close's ownership gate — stall
	// behind, or deadlock against, an in-flight open. Set to true only
	// on a successful open, under mu, so the lock-free load is
	// monotonic false->true and never observes a half-open pool.
	openedFlag atomic.Bool
}

// get returns the underlying pool, opening it on first call.
// Concurrent callers block until the open completes.
func (l *lazyPool) get() (Invoker, error) {
	if l == nil || l.open == nil {
		return nil, nil
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.pool != nil {
		return l.pool, nil
	}
	p, err := l.open()
	if err != nil {
		return nil, err
	}
	l.pool = p
	l.openedFlag.Store(true)
	return p, nil
}

// opened reports whether the pool has been opened yet — for tests, for
// the sessionz debug UI ("read pool: not yet opened" vs a live pool),
// and for sessionTable.Close's ownership gate. Reads openedFlag
// lock-free so a caller racing an in-flight open() (which holds mu for
// the whole dial) neither stalls nor deadlocks on mu.
func (l *lazyPool) opened() bool {
	if l == nil {
		return false
	}
	return l.openedFlag.Load()
}

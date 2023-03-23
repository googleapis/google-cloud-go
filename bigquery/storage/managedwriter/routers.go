// Copyright 2023 Google LLC
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     https://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package managedwriter

import (
	"fmt"
	"sort"
	"sync"
	"time"
)

type poolRouter interface {

	// poolAttach is called once to signal a router that it is responsible for a given pool.
	poolAttach(pool *connectionPool) error

	// poolDetach is called as part of clean connectionPool shutdown.
	// It provides an opportunity for the router to shut down internal state.
	poolDetach() error

	// writerAttach is a hook to notify the router that a new writer is being attached to the pool.
	// It provides an opportunity for the router to allocate resources and update internal state.
	writerAttach(writer *ManagedStream) error

	// writerAttach signals the router that a given writer is being removed from the pool.  The router
	// does not have responsibility for closing the writer, but this is called as part of writer close.
	writerDetach(writer *ManagedStream) error

	// pickConnection is used to select a connection for a given pending write.
	pickConnection(pw *pendingWrite) (*connection, error)
}

// simpleRouter is a primitive traffic router that routes all traffic to its single connection instance.
//
// This router is designed for our migration case, where an single ManagedStream writer has as 1:1 relationship
// with a connectionPool.  You can multiplex with this router, but it will never scale beyond a single connection.
type simpleRouter struct {
	mode string
	pool *connectionPool

	mu      sync.RWMutex
	conn    *connection
	writers map[string]struct{}
}

func (rtr *simpleRouter) poolAttach(pool *connectionPool) error {
	if rtr.pool == nil {
		rtr.pool = pool
		return nil
	}
	return fmt.Errorf("router already attached to pool %q", rtr.pool.id)
}

func (rtr *simpleRouter) poolDetach() error {
	rtr.mu.Lock()
	defer rtr.mu.Unlock()
	if rtr.conn != nil {
		rtr.conn.close()
		rtr.conn = nil
	}
	return nil
}

func (rtr *simpleRouter) writerAttach(writer *ManagedStream) error {
	if writer.id == "" {
		return fmt.Errorf("writer has no ID")
	}
	rtr.mu.Lock()
	defer rtr.mu.Unlock()
	rtr.writers[writer.id] = struct{}{}
	if rtr.conn == nil {
		rtr.conn = newConnection(rtr.pool, rtr.mode)
	}
	return nil
}

func (rtr *simpleRouter) writerDetach(writer *ManagedStream) error {
	if writer.id == "" {
		return fmt.Errorf("writer has no ID")
	}
	rtr.mu.Lock()
	defer rtr.mu.Unlock()
	delete(rtr.writers, writer.id)
	if len(rtr.writers) == 0 && rtr.conn != nil {
		// no attached writers, cleanup and remove connection.
		defer rtr.conn.close()
		rtr.conn = nil
	}
	return nil
}

// Picking a connection is easy; there's only one.
func (rtr *simpleRouter) pickConnection(pw *pendingWrite) (*connection, error) {
	rtr.mu.RLock()
	defer rtr.mu.RUnlock()
	if rtr.conn != nil {
		return rtr.conn, nil
	}
	return nil, fmt.Errorf("no connection available")
}

func newSimpleRouter(mode string) *simpleRouter {
	return &simpleRouter{
		// We don't add a connection until writers attach.
		mode:    mode,
		writers: make(map[string]struct{}),
	}
}

// sharedRouter is a more comprehensive router for a connection pool.
//
// It maintains state for both exclusive and shared connections, but doesn't commingle the
// two.  If the router is configured to allow multiplex, it also runs a watchdog goroutine
// that allows is to curate traffic there by reassigning writers to different connections.
//
// Multiplexing routing here is designed for connection sharing among more idle writers,
// and does NOT yet handle the use case where a single writer produces enough traffic to
// warrant fanout across multiple connections.
type sharedRouter struct {
	pool      *connectionPool
	multiplex bool
	maxConns  int           // multiplex limit.
	close     chan struct{} // for shutting down watchdog

	// mu guards access to exclusive connections
	mu sync.RWMutex
	// keyed by writer ID
	exclusiveConns map[string]*connection

	// multiMu guards access to multiplex mappings.
	multiMu sync.RWMutex
	// keyed by writer ID
	multiMap   map[string]*connection
	multiConns []*connection
}

type connPair struct {
	writer *ManagedStream
	conn   *connection
}

func (sr *sharedRouter) poolAttach(pool *connectionPool) error {
	if sr.pool == nil {
		sr.pool = pool
		sr.close = make(chan struct{})
		if sr.multiplex {
			go sr.watchdog()
		}
		return nil
	}
	return fmt.Errorf("router already attached to pool %q", sr.pool.id)
}

func (sr *sharedRouter) poolDetach() error {
	sr.mu.Lock()
	// cleanup explicit connections
	for writerID, conn := range sr.exclusiveConns {
		conn.close()
		delete(sr.exclusiveConns, writerID)
	}
	sr.mu.Unlock()
	// cleanup multiplex resources
	sr.multiMu.Lock()
	for _, co := range sr.multiConns {
		co.close()
	}
	sr.multiMap = make(map[string]*connection)
	sr.multiConns = nil
	close(sr.close) // trigger watchdog shutdown
	sr.multiMu.Unlock()
	return nil
}

func (sr *sharedRouter) writerAttach(writer *ManagedStream) error {
	if writer == nil {
		return fmt.Errorf("invalid writer")
	}
	if writer.id == "" {
		return fmt.Errorf("writer has empty ID")
	}
	if sr.multiplex && canMultiplex(writer.StreamName()) {
		return sr.writerAttachMulti(writer)
	}
	// Handle non-multiplex writer.
	sr.mu.Lock()
	defer sr.mu.Unlock()
	if pair := sr.exclusiveConns[writer.id]; pair != nil {
		return fmt.Errorf("writer %q already attached", writer.id)
	}
	sr.exclusiveConns[writer.id] = newConnection(sr.pool, "SIMPLEX")
	return nil
}

// multiAttach is the multiplex-specific logic for writerAttach.
// It should only be called from writerAttach.
func (sr *sharedRouter) writerAttachMulti(writer *ManagedStream) error {
	sr.multiMu.Lock()
	defer sr.multiMu.Unlock()
	// order any existing connections
	sr.orderAndGrowMultiConns()
	conn := sr.multiConns[0]
	sr.multiMap[writer.id] = conn
	return nil
}

// orderMultiConns orders the connection slice by current load, and will grow
// the connections if necessary.
//
// Should only be called with R/W lock.
func (sr *sharedRouter) orderAndGrowMultiConns() {
	sort.SliceStable(sr.multiConns,
		func(i, j int) bool {
			return sr.multiConns[i].curLoad() < sr.multiConns[j].curLoad()
		})
	if len(sr.multiConns) == 0 {
		sr.multiConns = []*connection{newConnection(sr.pool, "MULTIPLEX")}
	} else if sr.multiConns[0].isLoaded() && len(sr.multiConns) < sr.maxConns {
		sr.multiConns = append([]*connection{newConnection(sr.pool, "MULTIPLEX")}, sr.multiConns...)
	}
}

// rebalanceWriters looks for opportunities to redistribute traffic load.
//
// Should only be called with R/W lock.
func (sr *sharedRouter) rebalanceWriters() {
	mostIdleIdx := 0
	leastIdleIdx := len(sr.multiConns) - 1

	mostIdleConn := sr.multiConns[0]
	mostIdleLoad := mostIdleConn.curLoad()
	if mostIdleConn.isLoaded() {
		// Don't rebalance if all connections are loaded.
		return
	}
	// only look for rebalance opportunies between different connections.
	for mostIdleIdx != leastIdleIdx {
		targetConn := sr.multiConns[leastIdleIdx]
		if targetConn.curLoad() < mostIdleLoad*1.2 {
			// the load delta isn't significant enough between connections to rebalance.
			return
		}
		numWriters := 0
		candidateID := ""
		// Walk the writers to find who all shares the multimap, and pick a writer.
		// TODO: Revisit if we want to maintain an inverted mapping to make this cheaper.
		for writerID, conn := range sr.multiMap {
			if conn == targetConn {
				numWriters++
				if candidateID == "" {
					candidateID = writerID
				}
			}
		}
		if numWriters == 0 {
			// TODO: should we do anything here?
			// The likely cause of this would be where a writer is removed while there are still writes
			// in flight.  Eventually this should become the most idle connection, so premature pruning
			// seems unwarranted.
			leastIdleIdx = leastIdleIdx - 1
			continue
		}
		if numWriters == 1 {
			// the target only has a single writer, check the next busiest connection
			leastIdleIdx = leastIdleIdx - 1
			continue
		}
		// Rebalance candidate writer to the most idle conn.
		if candidateID != "" {
			sr.multiMap[candidateID] = mostIdleConn
		}
		return
	}

}

func (sr *sharedRouter) writerDetach(writer *ManagedStream) error {
	if writer == nil {
		return fmt.Errorf("invalid writer")
	}
	if sr.multiplex && canMultiplex(writer.StreamName()) {
		return sr.writerDetachMulti(writer)
	}
	// Handle non-multiplex writer.
	sr.mu.Lock()
	defer sr.mu.Unlock()
	conn := sr.exclusiveConns[writer.id]
	if conn == nil {
		return fmt.Errorf("writer not currently attached")
	}
	conn.close()
	delete(sr.exclusiveConns, writer.id)
	return nil
}

// writerDetachMulti is the multiplex-specific logic for writerDetach.
// It should only be called from writerDetach.
func (sr *sharedRouter) writerDetachMulti(writer *ManagedStream) error {
	sr.multiMu.Lock()
	defer sr.multiMu.Unlock()
	delete(sr.multiMap, writer.id)
	// If the number of writers drops to zero, close all open connections.
	if len(sr.multiMap) == 0 {
		for _, co := range sr.multiConns {
			co.close()
		}
		sr.multiConns = nil
	}
	return nil
}

func (sr *sharedRouter) pickConnection(pw *pendingWrite) (*connection, error) {
	if pw.writer == nil {
		return nil, fmt.Errorf("no writer present pending write")
	}
	if sr.multiplex && canMultiplex(pw.writer.StreamName()) {
		return sr.pickMultiplexConnection(pw)
	}
	sr.mu.RLock()
	defer sr.mu.RUnlock()
	conn := sr.exclusiveConns[pw.writer.id]
	if conn == nil {
		return nil, fmt.Errorf("writer %q unknown", pw.writer.id)
	}
	return conn, nil
}

func (sr *sharedRouter) pickMultiplexConnection(pw *pendingWrite) (*connection, error) {
	sr.multiMu.RLock()
	defer sr.multiMu.RUnlock()
	conn := sr.multiMap[pw.writer.id]
	if conn == nil {
		// TODO: update map
		return nil, fmt.Errorf("no multiplex connection assigned")
	}
	return conn, nil
}

func (sr *sharedRouter) watchdog() {
	//threshold := 1e-9
	//idleInterval := 5 * time.Second
	for {
		select {
		case <-sr.close:
			return
		case <-time.After(2 * time.Second):
			sr.multiMu.Lock()
			sr.orderAndGrowMultiConns()
			sr.rebalanceWriters()
			sr.multiMu.Unlock()
			/*
				mostIdle := sr.multiConns[0]
				if math.Abs(mostIdle.curLoad()) < threshold {
					lastWritten := mostIdle.lastWrite
					if time.Since(lastWritten) > cleanupInterval {
						// TODO: remove the connection.
					}
				}
			*/
		}
	}
}

func newSharedRouter(multiplex bool, maxConns int) *sharedRouter {
	return &sharedRouter{
		multiplex:      multiplex,
		maxConns:       maxConns,
		exclusiveConns: make(map[string]*connection),
		multiMap:       make(map[string]*connection),
	}
}

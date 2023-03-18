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
	"sync"
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

type sharedRouter struct {
	pool      *connectionPool
	multiplex bool

	mu sync.RWMutex
	// keyed by writer ID
	exclusivePairs map[string]*connPair
}

type connPair struct {
	writer *ManagedStream
	conn   *connection
}

func (sr *sharedRouter) poolAttach(pool *connectionPool) error {
	if sr.pool == nil {
		sr.pool = pool
		sr.exclusivePairs = make(map[string]*connPair)
		return nil
	}
	return fmt.Errorf("router already attached to pool %q", sr.pool.id)
}

func (sr *sharedRouter) poolDetach() error {
	sr.mu.Lock()
	defer sr.mu.Unlock()
	for writerID, pair := range sr.exclusivePairs {
		if conn := pair.conn; conn != nil {
			conn.close()
		}
		if writer := pair.writer; writer != nil {
			writer.Close()
		}
		delete(sr.exclusivePairs, writerID)
	}
	return nil
}

func (sr *sharedRouter) writerAttach(writer *ManagedStream) error {
	if writer == nil {
		return fmt.Errorf("invalid writer")
	}
	if sr.multiplex && canMultiplex(writer.StreamName()) {
		// TODO: wire up multiplexing
		return fmt.Errorf("multiplex routing not implemented")
	}
	if pair := sr.exclusivePairs[writer.id]; pair != nil {
		return fmt.Errorf("writer %q already attached", writer.id)
	}
	sr.exclusivePairs[writer.id] = &connPair{
		writer: writer,
		conn:   newConnection(sr.pool, "SIMPLEX"),
	}
	return nil
}

func (sr *sharedRouter) writerDetach(writer *ManagedStream) error {
	if writer == nil {
		return fmt.Errorf("invalid writer")
	}
	if sr.multiplex && canMultiplex(writer.StreamName()) {
		// TODO: multiplex detach
		return fmt.Errorf("multiplex routing not implemented")
	}
	sr.mu.Lock()
	defer sr.mu.Unlock()
	pair := sr.exclusivePairs[writer.id]
	if pair == nil {
		return fmt.Errorf("writer not currently attached")
	}
	pair.conn.close()
	delete(sr.exclusivePairs, writer.id)
	return nil
}

func (sr *sharedRouter) pickConnection(pw *pendingWrite) (*connection, error) {
	if pw.writer == nil {
		return nil, fmt.Errorf("no writer present pending write")
	}
	if sr.multiplex && canMultiplex(pw.writer.StreamName()) {
		return nil, fmt.Errorf("multiplex routing not yet implemented")
	}
	sr.mu.RLock()
	defer sr.mu.RUnlock()
	pair := sr.exclusivePairs[pw.writer.id]
	if pair == nil {
		return nil, fmt.Errorf("writer %q unknown", pw.writer.id)
	}
	return pair.conn, nil
}

func newSharedRouter(multiplex bool) *sharedRouter {
	return &sharedRouter{
		multiplex: multiplex,
	}
}

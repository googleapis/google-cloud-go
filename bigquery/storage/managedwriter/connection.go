// Copyright 2022 Google LLC
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
	"context"
	"fmt"
	"io"
	"sync"

	"cloud.google.com/go/bigquery/storage/apiv1/storagepb"
	"github.com/googleapis/gax-go/v2"
	"go.opencensus.io/tag"
	"google.golang.org/grpc/codes"
	grpcstatus "google.golang.org/grpc/status"
	"google.golang.org/protobuf/proto"
)

const (
	poolIDPrefix   string = "connectionpool"
	connIDPrefix   string = "connection"
	writerIDPrefix string = "writer"
)

// connectionPool represents a pooled set of connections.
//
// The pool retains references to connections, and maintains the mapping between writers
// and connections.
//
// TODO: connection and writer mappings will be added in a subsequent PR.
type connectionPool struct {
	id string

	// the pool retains the long-lived context responsible for opening/maintaining bidi connections.
	ctx    context.Context
	cancel context.CancelFunc

	baseFlowController *flowController // template flow controller used for building connections.

	// We centralize the open function on the pool, rather than having an instance of the open func on every
	// connection.  Opening the connection is a stateless operation.
	open func(opts ...gax.CallOption) (storagepb.BigQueryWrite_AppendRowsClient, error)

	// We specify one set of calloptions for the pool.
	// All connections in the pool open with the same call options.
	callOptions []gax.CallOption

	router poolRouter // poolManager makes the decisions about connections and routing.

	retry *statelessRetryer // default retryer for the pool.
}

// processWrite is responsible for routing a write request to an appropriate connection.  It's used by ManagedStream instances
// to send writes without awareness of individual connections.
func (pool *connectionPool) processWrite(pw *pendingWrite) error {
	conn, err := pool.router.pickConnection(pw)
	if err != nil {
		return err
	}
	return conn.appendWithRetry(pw)
}

func (pool *connectionPool) addWriter(writer *ManagedStream) error {
	if pool.router != nil {
		return pool.router.writerAttach(writer)
	}
	return fmt.Errorf("no router for pool")
}

func (pool *connectionPool) removeWriter(writer *ManagedStream) error {
	if pool.router != nil {
		return pool.router.writerDetach(writer)
	}
	return fmt.Errorf("no router for pool")
}

// openWithRetry establishes a new bidi stream and channel pair.  It is used by connection objects
// when (re)opening the network connection to the backend.
//
// The connection.getStream() func should be the only consumer of this.
func (cp *connectionPool) openWithRetry(co *connection) (storagepb.BigQueryWrite_AppendRowsClient, chan *pendingWrite, error) {
	r := &unaryRetryer{}
	for {
		recordStat(cp.ctx, AppendClientOpenCount, 1)
		arc, err := cp.open(cp.callOptions...)
		bo, shouldRetry := r.Retry(err)
		if err != nil && shouldRetry {
			recordStat(cp.ctx, AppendClientOpenRetryCount, 1)
			if err := gax.Sleep(cp.ctx, bo); err != nil {
				return nil, nil, err
			}
			continue
		}
		if err == nil {
			// The channel relationship with its ARC is 1:1.  If we get a new ARC, create a new pending
			// write channel and fire up the associated receive processor.  The channel ensures that
			// responses for a connection are processed in the same order that appends were sent.
			depth := 1000 // default backend queue limit
			if d := co.fc.maxInsertCount; d > 0 {
				depth = d
			}
			ch := make(chan *pendingWrite, depth)
			go connRecvProcessor(co, arc, ch)
			return arc, ch, nil
		}
		return arc, nil, err
	}
}

// returns the stateless default retryer for the pool.  If one's not set (re-enqueue retries disabled),
// it returns a retryer that only permits single attempts.
func (cp *connectionPool) defaultRetryer() *statelessRetryer {
	if cp.retry != nil {
		return cp.retry
	}
	return &statelessRetryer{
		maxAttempts: 1,
	}
}

// connection models the underlying AppendRows grpc bidi connection used for writing
// data and receiving acknowledgements.  It is responsible for enqueing writes and processing
// responses from the backend.
type connection struct {
	id   string
	pool *connectionPool // each connection retains a reference to its owning pool.

	fc     *flowController // each connection has it's own flow controller.
	ctx    context.Context // retained context for maintaining the connection, derived from the owning pool.
	cancel context.CancelFunc

	retry *statelessRetryer

	mu        sync.Mutex
	arc       *storagepb.BigQueryWrite_AppendRowsClient // reference to the grpc connection (send, recv, close)
	reconnect bool                                      //
	err       error                                     // terminal connection error
	pending   chan *pendingWrite
}

func newConnection(ctx context.Context, pool *connectionPool) *connection {
	// create and retain a cancellable context.
	connCtx, cancel := context.WithCancel(ctx)
	fc := newFlowController(0, 0)
	if pool != nil {
		fc = copyFlowController(pool.baseFlowController)
	}
	return &connection{
		id:     newUUID(connIDPrefix),
		pool:   pool,
		fc:     fc,
		ctx:    connCtx,
		cancel: cancel,
	}
}

// close closes a connection.
func (co *connection) close() {
	co.mu.Lock()
	defer co.mu.Unlock()
	// first, cancel the retained context.
	if co.cancel != nil {
		co.cancel()
		co.cancel = nil
	}
	// close sending if we have an ARC.
	if co.arc != nil {
		(*co.arc).CloseSend()
		co.arc = nil
	}
	// mark terminal error if not already set.
	if co.err != nil {
		co.err = io.EOF
	}
	// signal pending channel close.
	if co.pending != nil {
		close(co.pending)
	}
}

// lockingAppend handles a single append request on a given connection.
func (co *connection) lockingAppend(pw *pendingWrite) error {
	// Don't both calling/retrying if this append's context is already expired.
	if err := pw.reqCtx.Err(); err != nil {
		return err
	}

	var statsOnExit func()

	// critical section:  Things that need to happen inside the critical section:
	//
	// * get/open conenction
	// * issue the append
	// * add the pending write to the channel for the connection (ordering for the response)
	co.mu.Lock()
	defer func() {
		co.mu.Unlock()
		if statsOnExit != nil {
			statsOnExit()
		}
	}()

	var arc *storagepb.BigQueryWrite_AppendRowsClient
	var ch chan *pendingWrite
	var err error

	arc, ch, err = co.getStream(arc, false)
	if err != nil {
		return err
	}

	// TODO: optimization logic here
	// Here, we need to compare values for the previous append and compare them to the pending write.
	// If they are matches, we can clear fields in the request that aren't needed (e.g. not resending descriptor,
	// stream ID, etc.)
	//
	// Current implementation just clones the request.
	req := proto.Clone(pw.request).(*storagepb.AppendRowsRequest)

	pw.attemptCount = pw.attemptCount + 1
	if err = (*arc).Send(req); err != nil {
		if shouldReconnect(err) {
			// if we think this connection is unhealthy, force a reconnect on the next send.
			co.reconnect = true
		}
		return err
	}

	// Compute numRows, once we pass ownership to the channel the request may be
	// cleared.
	numRows := int64(len(pw.request.GetProtoRows().Rows.GetSerializedRows()))
	statsOnExit = func() {
		// these will get recorded once we exit the critical section.
		// TODO: resolve open questions around what labels should be attached (connection, streamID, etc)
		recordStat(co.ctx, AppendRequestRows, numRows)
		recordStat(co.ctx, AppendRequests, 1)
		recordStat(co.ctx, AppendRequestBytes, int64(pw.reqSize))
	}
	ch <- pw
	return nil
}

// getStream returns either a valid ARC client stream or permanent error.
//
// Any calls to getStream should do so in possesion of the critical section lock.
func (co *connection) getStream(arc *storagepb.BigQueryWrite_AppendRowsClient, forceReconnect bool) (*storagepb.BigQueryWrite_AppendRowsClient, chan *pendingWrite, error) {
	if co.err != nil {
		return nil, nil, co.err
	}
	co.err = co.ctx.Err()
	if co.err != nil {
		return nil, nil, co.err
	}

	// Previous activity on the stream indicated it is not healthy, so propagate that as a reconnect.
	if co.reconnect {
		forceReconnect = true
		co.reconnect = false
	}
	// Always return the retained ARC if the arg differs.
	if arc != co.arc && !forceReconnect {
		return co.arc, co.pending, nil
	}
	// We need to (re)open a connection.  Cleanup previous connection and channel if they are present.
	if co.arc != nil {
		(*co.arc).CloseSend()
	}
	if co.pending != nil {
		close(co.pending)
	}

	co.arc = new(storagepb.BigQueryWrite_AppendRowsClient)
	*co.arc, co.pending, co.err = co.pool.openWithRetry(co)
	return co.arc, co.pending, co.err
}

// appendWithRetry handles the details of adding sending an append request on a connection.  Retries here address
// problems with sending the request.  The processor on the connection is responsible for retrying based on the
// response received from the service.
func (co *connection) appendWithRetry(pw *pendingWrite) error {
	appendRetryer := resolveRetry(pw, co.pool)
	for {
		appendErr := co.lockingAppend(pw)
		if appendErr != nil {
			// Append yielded an error.  Retry by continuing or return.
			status := grpcstatus.Convert(appendErr)
			if status != nil {
				ctx, _ := tag.New(co.ctx, tag.Insert(keyError, status.Code().String()))
				recordStat(ctx, AppendRequestErrors, 1)
			}
			bo, shouldRetry := appendRetryer.Retry(appendErr, pw.attemptCount)
			if shouldRetry {
				if err := gax.Sleep(co.ctx, bo); err != nil {
					return err
				}
				continue
			}
			// Mark the pending write done.  This will not be returned to the user, they'll receive the returned error.
			pw.markDone(nil, appendErr, co.fc)
			return appendErr
		}
		return nil
	}
}

// connRecvProcessor is used to propagate append responses back up with the originating write requests.  It
// It runs as a goroutine.  A connection object allows for reconnection, and each reconnection establishes a new
// processing gorouting and backing channel.
func connRecvProcessor(co *connection, arc storagepb.BigQueryWrite_AppendRowsClient, ch <-chan *pendingWrite) {
	for {
		select {
		case <-co.ctx.Done():
			// Context is done, so we're not going to get further updates.  Mark all work left in the channel
			// with the context error.  We don't attempt to re-enqueue in this case.
			for {
				pw, ok := <-ch
				if !ok {
					return
				}
				pw.markDone(nil, co.ctx.Err(), co.fc)
			}
		case nextWrite, ok := <-ch:
			if !ok {
				// Channel closed, all elements processed.
				return
			}
			// block until we get a corresponding response or err from stream.
			resp, err := arc.Recv()
			if err != nil {
				// TODO: wire in retryer in a later refactor.
				// For now, we go back to old behavior of simply marking with error.
				nextWrite.markDone(resp, err, co.fc)
				continue
			}
			// Record that we did in fact get a response from the backend.
			recordStat(co.ctx, AppendResponses, 1)

			if status := resp.GetError(); status != nil {
				// The response from the backend embedded a status error.  We record that the error
				// occurred, and tag it based on the response code of the status.
				if tagCtx, tagErr := tag.New(co.ctx, tag.Insert(keyError, codes.Code(status.GetCode()).String())); tagErr == nil {
					recordStat(tagCtx, AppendResponseErrors, 1)
				}
				respErr := grpcstatus.ErrorProto(status)
				// TODO: wire in retryer for backend response here in a future refactor.
				// For now, go back to old behavior of marking with terminal error.
				nextWrite.markDone(resp, respErr, co.fc)
				continue
			}
			// We had no error in the receive or in the response.  Mark the write done.
			nextWrite.markDone(resp, nil, co.fc)
		}
	}
}

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
	pool *connectionPool

	mu      sync.RWMutex
	conn    *connection
	writers map[string]struct{}
}

// TODO: This will be implemented in a future PR where we hook up the new connection and pool to the writer.
func (rtr *simpleRouter) poolAttach(pool *connectionPool) error {
	return fmt.Errorf("unimplemented")
}

// TODO: This will be implemented in a future PR where we hook up the new connection and pool to the writer.
func (rtr *simpleRouter) poolDetach() error {
	return fmt.Errorf("unimplemented")
}

func (rtr *simpleRouter) writerAttach(writer *ManagedStream) error {
	if writer.id == "" {
		return fmt.Errorf("writer has no ID")
	}
	rtr.mu.Lock()
	defer rtr.mu.Unlock()
	rtr.writers[writer.id] = struct{}{}
	if rtr.conn == nil {
		rtr.conn = newConnection(rtr.pool.ctx, rtr.pool)
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

func newSimpleRouter(pool *connectionPool) *simpleRouter {
	return &simpleRouter{
		pool:    pool,
		writers: make(map[string]struct{}),
	}
}

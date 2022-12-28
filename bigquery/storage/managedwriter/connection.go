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

// connectionPool represents a pooled set of connections.
//
// The pool retains references to connections, and maintains the mapping between writers
// and connections.
//
// TODO: connection and writer mappings will be added in a subsequent PR.
type connectionPool struct {
	id string

	ctx    context.Context
	cancel context.CancelFunc
	// baseFlowController isn't used directly, but is the prototype used for each connection instance.
	baseFlowController *flowController

	// We centralize the open function on the pool, rather than having an instance of the open func on every
	// connection.  Opening the connection is a stateless operation.
	open func(opts ...gax.CallOption) (storagepb.BigQueryWrite_AppendRowsClient, error)

	// We specify one set of calloptions for the pool.
	// All connections in the pool open with the same call options.
	callOptions []gax.CallOption

	retry *statelessRetryer // default retryer for all members of the pool.

	// mutex guards all modifications to the connection/writer mappings.
	// We use a RWMutex as we expect many lookups for routing, but few modifications.
	mapMu sync.RWMutex
	// connectionMap is keyed by connectionID
	connectionMap map[string]*connection
	// writerMap is keyed by writer ID
	writerMap map[string]*ManagedStream
	// forward lookups from writer to connection.
	// TODO: This will be a map[string][]string in the future, as we want to be able to fan out writes to multiple connections
	// for high traffic streams.
	writerToConnMap map[string]string
	// backward lookups from connection to writers.
	connToWriterMap map[string][]string
}

// newConnectionPool configures a new connection pool instance and initializes core functionality.
func newConnectionPool(ctx context.Context, fc *flowController, openF func(opts ...gax.CallOption) (storagepb.BigQueryWrite_AppendRowsClient, error), opts ...gax.CallOption) *connectionPool {
	cpCtx, cancel := context.WithCancel(ctx)
	if fc == nil {
		fc = newFlowController(0, 0)
	}
	return &connectionPool{
		id:                 newUUID("connectionpool"),
		ctx:                cpCtx,
		cancel:             cancel,
		baseFlowController: fc,
		open:               openF,
		callOptions:        opts,
		connectionMap:      make(map[string]*connection),
		writerMap:          make(map[string]*ManagedStream),
		writerToConnMap:    make(map[string]string),
		connToWriterMap:    make(map[string][]string),
	}
}

// addConnection creates an additional connection associated to the connection pool.
//
// It does not add the connection to the connection map.  Code in control of the map
// write lock is responsible for this.
func (cp *connectionPool) addConnection() (*connection, error) {

	coCtx, cancel := context.WithCancel(cp.ctx)
	conn := &connection{
		id:     newUUID("connection"),
		pool:   cp,
		fc:     copyFlowController(cp.baseFlowController),
		ctx:    coCtx,
		cancel: cancel,
	}
	return conn, nil
}

// evictConnection is used to remove an existing connection from the pool.
//
// TODO: in a true multiplex scenario, this should rebalance traffic by redistributing
// writers associated with the connection to be evicted.  In this initial implementation
// we simply remove references.  The connectionForWriter resolver handles this by adding
// a new connection when the reference isn't found.
func (cp *connectionPool) evictConnection(connID string) error {
	if connID == "" {
		return fmt.Errorf("empty connection ID")
	}
	cp.mapMu.Lock()
	defer cp.mapMu.Unlock()
	delete(cp.connectionMap, connID)
	delete(cp.connToWriterMap, connID)
	for w, c := range cp.writerToConnMap {
		if c == connID {
			delete(cp.writerToConnMap, w)
		}
	}
	return nil
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

// processRetry resolves retry behaviors for a given write.  Callers of processRetry no longer need worry about the state of the write, it
// will either mark the write complete or re-enqueue it for another write attempt.
func (cp *connectionPool) processRetry(pw *pendingWrite, srcConn *connection, appendResp *storagepb.AppendRowsResponse, initialErr error) {
	err := initialErr
	for {
		pause, shouldRetry := pw.writer.statelessRetryer().Retry(err, pw.attemptCount)
		if !shouldRetry {
			// Should not attempt to re-append.
			pw.markDone(appendResp, err, srcConn.fc)
			return
		}
		gax.Sleep(pw.reqCtx, pause)
		err = pw.writer.appendWithRetry(pw)
		if err != nil {
			// Re-enqueue failed, send it through the loop again.
			continue
		}
		// Break out of the loop, we were successful and the write has been
		// re-inserted.
		recordStat(cp.ctx, AppendRetryCount, 1)
		break
	}
}

// connectionForWriter returns the associated connection for a given writer.
//
// TODO: this implementation is expressely primitive, and is used to refactor away from
// existing logic where writer and connection are coupled conceptually.  Further refactors
// will augment this logic.
func (cp *connectionPool) connectionForWriter(writerID string) (*connection, error) {
	// take a full write lock for the simple implementation.
	cp.mapMu.Lock()
	defer cp.mapMu.Unlock()
	if len(cp.connectionMap) > 0 {
		for _, conn := range cp.connectionMap {
			return conn, nil
		}
	}
	// For safety, ensure the writer is known before allocating a new connection.
	if _, ok := cp.writerMap[writerID]; !ok {
		return nil, fmt.Errorf("writer not owned by the pool: %s", writerID)
	}
	conn, err := cp.addConnection()
	if err != nil {
		return nil, err
	}
	cp.connectionMap[conn.id] = conn
	cp.connToWriterMap[conn.id] = []string{writerID}
	cp.writerToConnMap[writerID] = conn.id
	return conn, nil
}

// connection models the underlying AppendRows grpc bidi connection used for writing
// data and receiving acknowledgements.  It is responsible for enqueing writes and processing
// responses from the backend.
type connection struct {
	id   string
	pool *connectionPool // each connection retains a reference to its owning pool.

	fc     *flowController // each connection has it's own flow controller.
	ctx    context.Context // retained context for maintaining the connection.
	cancel context.CancelFunc

	mu        sync.Mutex
	arc       *storagepb.BigQueryWrite_AppendRowsClient // reference to the grpc connection (send, recv, close)
	reconnect bool                                      //
	err       error                                     // terminal connection error
	pending   chan *pendingWrite
}

// close can be used to close a connection and it's backing channel.  Fully terminating a connection
// marks it with a terminal error (io.EOF) if it's still viable, and removes references to the connection
// from the pool.
func (co *connection) close(terminate bool) {
	// Close the connection so it can't be used for subsequent writes.
	co.mu.Lock()
	defer co.mu.Unlock()
	if co.arc != nil {
		(*co.arc).CloseSend()
	}
	if co.pending != nil {
		close(co.pending)
	}
	if terminate {
		if co.err == nil {
			co.err = io.EOF
		}
		if co.pool != nil {
			co.pool.evictConnection(co.id)
		}
	}
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

// lockingAppend handles a single append request on a connection.
func (c *connection) lockingAppend(pw *pendingWrite) error {
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
	c.mu.Lock()
	defer func() {
		c.mu.Unlock()
		if statsOnExit != nil {
			statsOnExit()
		}
	}()

	var arc *storagepb.BigQueryWrite_AppendRowsClient
	var ch chan *pendingWrite
	var err error

	arc, ch, err = c.getStream(arc, false)
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
			c.reconnect = true
		}
		return err
	}

	// Compute numRows, once we pass ownership to the channel the request may be
	// cleared.
	numRows := int64(len(pw.request.GetProtoRows().Rows.GetSerializedRows()))
	statsOnExit = func() {
		// these will get recorded once we exit the critical section.
		// TODO: resolve open questions around what labels should be attached (connection, streamID, etc)
		recordStat(c.ctx, AppendRequestRows, numRows)
		recordStat(c.ctx, AppendRequests, 1)
		recordStat(c.ctx, AppendRequestBytes, int64(pw.reqSize))
	}
	ch <- pw
	return nil
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
				// Evaluate the error from the receive and possibly retry.
				co.pool.processRetry(nextWrite, co, nil, err)
				// We're done with the write regardless of outcome, continue onto the
				// next element.
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
				// We use the status error to evaluate and possible re-enqueue the write.
				co.pool.processRetry(nextWrite, co, resp, respErr)
				// We're done with the write regardless of outcome, continue on to the next
				// element.
				continue
			}
			// We had no error in the receive or in the response.  Mark the write done.
			nextWrite.markDone(resp, nil, co.fc)
		}
	}
}

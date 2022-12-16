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
	"sync"

	"cloud.google.com/go/bigquery/storage/apiv1/storagepb"
	"github.com/googleapis/gax-go/v2"
	"go.opencensus.io/tag"
	"google.golang.org/grpc/codes"
	grpcstatus "google.golang.org/grpc/status"
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
}

// addConnection creates an additional connection associated to the connection pool.
func (cp *connectionPool) addConnection() (*connection, error) {

	coCtx, cancel := context.WithCancel(cp.ctx)
	conn := &connection{
		id:     newUUID("connection"),
		pool:   cp,
		fc:     copyFlowController(cp.baseFlowController),
		ctx:    coCtx,
		cancel: cancel,
	}
	// TODO: retain a reference to the connection in the pool registry.
	return conn, nil
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

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
	"errors"
	"fmt"
	"io"
	"sync"

	"cloud.google.com/go/bigquery/storage/apiv1/storagepb"
	"github.com/googleapis/gax-go/v2"
	"go.opencensus.io/tag"
	"google.golang.org/grpc/codes"
	grpcstatus "google.golang.org/grpc/status"
)

const (
	poolIDPrefix   string = "connectionpool"
	connIDPrefix   string = "connection"
	writerIDPrefix string = "writer"
)

var (
	errNoRouterForPool = errors.New("no router for connection pool")
)

// connectionPool represents a pooled set of connections.
//
// The pool retains references to connections, and maintains the mapping between writers
// and connections.
type connectionPool struct {
	id       string
	location string // BQ region associated with this pool.

	// the pool retains the long-lived context responsible for opening/maintaining bidi connections.
	ctx    context.Context
	cancel context.CancelFunc

	baseFlowController *flowController // template flow controller used for building connections.

	// We centralize the open function on the pool, rather than having an instance of the open func on every
	// connection.  Opening the connection is a stateless operation.
	open func(ctx context.Context, opts ...gax.CallOption) (storagepb.BigQueryWrite_AppendRowsClient, error)

	// We specify default calloptions for the pool.
	// Explicit connections may have their own calloptions as well.
	callOptions []gax.CallOption

	router poolRouter // poolManager makes the decisions about connections and routing.

	retry *statelessRetryer // default retryer for the pool.
}

// activateRouter handles wiring up a connection pool and it's router.
func (pool *connectionPool) activateRouter(rtr poolRouter) error {
	if pool.router != nil {
		return fmt.Errorf("router already activated")
	}
	if err := rtr.poolAttach(pool); err != nil {
		return fmt.Errorf("router rejected attach: %w", err)
	}
	pool.router = rtr
	return nil
}

func (pool *connectionPool) Close() error {
	// Signal router and cancel context, which should propagate to all writers.
	var err error
	if pool.router != nil {
		err = pool.router.poolDetach()
	}
	if cancel := pool.cancel; cancel != nil {
		cancel()
	}
	return err
}

// pickConnection is used by writers to select a connection.
func (pool *connectionPool) selectConn(pw *pendingWrite) (*connection, error) {
	if pool.router == nil {
		return nil, errNoRouterForPool
	}
	return pool.router.pickConnection(pw)
}

func (pool *connectionPool) addWriter(writer *ManagedStream) error {
	if p := writer.pool; p != nil {
		return fmt.Errorf("writer already attached to pool %q", p.id)
	}
	if pool.router == nil {
		return errNoRouterForPool
	}
	if err := pool.router.writerAttach(writer); err != nil {
		return err
	}
	writer.pool = pool
	return nil
}

func (pool *connectionPool) removeWriter(writer *ManagedStream) error {
	if pool.router == nil {
		return errNoRouterForPool
	}
	detachErr := pool.router.writerDetach(writer)
	return detachErr
}

func (cp *connectionPool) mergeCallOptions(co *connection) []gax.CallOption {
	if co == nil {
		return cp.callOptions
	}
	var mergedOpts []gax.CallOption
	mergedOpts = append(mergedOpts, cp.callOptions...)
	mergedOpts = append(mergedOpts, co.callOptions...)
	return mergedOpts
}

// openWithRetry establishes a new bidi stream and channel pair.  It is used by connection objects
// when (re)opening the network connection to the backend.
//
// The connection.getStream() func should be the only consumer of this.
func (cp *connectionPool) openWithRetry(co *connection) (storagepb.BigQueryWrite_AppendRowsClient, chan *pendingWrite, error) {
	r := &unaryRetryer{}
	for {
		arc, err := cp.open(co.ctx, cp.mergeCallOptions(co)...)
		metricCtx := cp.ctx
		if err == nil {
			// accumulate AppendClientOpenCount for the success case.
			recordStat(metricCtx, AppendClientOpenCount, 1)
		}
		if err != nil {
			if tagCtx, tagErr := tag.New(cp.ctx, tag.Insert(keyError, grpcstatus.Code(err).String())); tagErr == nil {
				metricCtx = tagCtx
			}
			// accumulate AppendClientOpenCount for the error case.
			recordStat(metricCtx, AppendClientOpenCount, 1)
			bo, shouldRetry := r.Retry(err)
			if shouldRetry {
				recordStat(cp.ctx, AppendClientOpenRetryCount, 1)
				if err := gax.Sleep(cp.ctx, bo); err != nil {
					return nil, nil, err
				}
				continue
			} else {
				// non-retriable error while opening
				return nil, nil, err
			}
		}

		// The channel relationship with its ARC is 1:1.  If we get a new ARC, create a new pending
		// write channel and fire up the associated receive processor.  The channel ensures that
		// responses for a connection are processed in the same order that appends were sent.
		depth := 1000 // default backend queue limit
		if d := co.fc.maxInsertCount; d > 0 {
			depth = d
		}
		ch := make(chan *pendingWrite, depth)
		go connRecvProcessor(co.ctx, co, arc, ch)
		return arc, ch, nil
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

	fc          *flowController  // each connection has it's own flow controller.
	callOptions []gax.CallOption // custom calloptions for this connection.
	ctx         context.Context  // retained context for maintaining the connection, derived from the owning pool.
	cancel      context.CancelFunc

	retry     *statelessRetryer
	optimizer sendOptimizer

	mu        sync.Mutex
	arc       *storagepb.BigQueryWrite_AppendRowsClient // reference to the grpc connection (send, recv, close)
	reconnect bool                                      //
	err       error                                     // terminal connection error
	pending   chan *pendingWrite

	loadBytesThreshold int
	loadCountThreshold int
}

type connectionMode string

const (
	multiplexConnectionMode connectionMode = "MULTIPLEX"
	simplexConnectionMode   connectionMode = "SIMPLEX"
	verboseConnectionMode   connectionMode = "VERBOSE"
)

func newConnection(pool *connectionPool, mode connectionMode, settings *streamSettings) *connection {
	if pool == nil {
		return nil
	}
	// create and retain a cancellable context.
	connCtx, cancel := context.WithCancel(pool.ctx)

	// Resolve local overrides for flow control and call options
	fcRequests := 0
	fcBytes := 0
	var opts []gax.CallOption

	if pool.baseFlowController != nil {
		fcRequests = pool.baseFlowController.maxInsertCount
		fcBytes = pool.baseFlowController.maxInsertBytes
	}
	if settings != nil {
		if settings.MaxInflightRequests > 0 {
			fcRequests = settings.MaxInflightRequests
		}
		if settings.MaxInflightBytes > 0 {
			fcBytes = settings.MaxInflightBytes
		}
		opts = settings.appendCallOptions
	}
	fc := newFlowController(fcRequests, fcBytes)
	countLimit, byteLimit := computeLoadThresholds(fc)

	return &connection{
		id:                 newUUID(connIDPrefix),
		pool:               pool,
		fc:                 fc,
		ctx:                connCtx,
		cancel:             cancel,
		optimizer:          optimizer(mode),
		loadBytesThreshold: byteLimit,
		loadCountThreshold: countLimit,
		callOptions:        opts,
	}
}

func computeLoadThresholds(fc *flowController) (countLimit, byteLimit int) {
	countLimit = 1000
	byteLimit = 0
	if fc != nil {
		if fc.maxInsertBytes > 0 {
			// 20% of byte limit
			byteLimit = int(float64(fc.maxInsertBytes) * 0.2)
		}
		if fc.maxInsertCount > 0 {
			// MIN(1, 20% of insert limit)
			countLimit = int(float64(fc.maxInsertCount) * 0.2)
			if countLimit < 1 {
				countLimit = 1
			}
		}
	}
	return
}

func optimizer(mode connectionMode) sendOptimizer {
	switch mode {
	case multiplexConnectionMode:
		return &multiplexOptimizer{}
	case verboseConnectionMode:
		return &verboseOptimizer{}
	case simplexConnectionMode:
		return &simplexOptimizer{}
	}
	return nil
}

// release is used to signal flow control release when a write is no longer in flight.
func (co *connection) release(pw *pendingWrite) {
	co.fc.release(pw.reqSize)
}

// signal indicating that multiplex traffic level is high enough to warrant adding more connections.
func (co *connection) isLoaded() bool {
	if co.loadCountThreshold > 0 && co.fc.count() > co.loadCountThreshold {
		return true
	}
	if co.loadBytesThreshold > 0 && co.fc.bytes() > co.loadBytesThreshold {
		return true
	}
	return false
}

// curLoad is a representation of connection load.
// Its primary purpose is comparing the load of different connections.
func (co *connection) curLoad() float64 {
	load := float64(co.fc.count()) / float64(co.loadCountThreshold+1)
	if co.fc.maxInsertBytes > 0 {
		load += (float64(co.fc.bytes()) / float64(co.loadBytesThreshold+1))
		load = load / 2
	}
	return load
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
	// close sending if we have a real ARC.
	if co.arc != nil && (*co.arc) != (storagepb.BigQueryWrite_AppendRowsClient)(nil) {
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

	if err := co.fc.acquire(pw.reqCtx, pw.reqSize); err != nil {
		// We've failed to acquire.  This may get retried on a different connection, so marking the write done is incorrect.
		return err
	}

	var statsOnExit func(ctx context.Context)

	// critical section:  Things that need to happen inside the critical section:
	//
	// * get/open conenction
	// * issue the append
	// * add the pending write to the channel for the connection (ordering for the response)
	co.mu.Lock()
	defer func() {
		sCtx := co.ctx
		co.mu.Unlock()
		if statsOnExit != nil && sCtx != nil {
			statsOnExit(sCtx)
		}
	}()

	var arc *storagepb.BigQueryWrite_AppendRowsClient
	var ch chan *pendingWrite
	var err error

	// Handle promotion of per-request schema to default schema in the case of updates.
	// Additionally, we check multiplex status as schema changes for explicit streams
	// require reconnect, whereas multiplex does not.
	forceReconnect := false
	promoted := false
	if pw.writer != nil && pw.reqTmpl != nil {
		if !pw.reqTmpl.Compatible(pw.writer.curTemplate) {
			if pw.writer.curTemplate == nil {
				// promote because there's no current template
				pw.writer.curTemplate = pw.reqTmpl
				promoted = true
			} else {
				if pw.writer.curTemplate.versionTime.Before(pw.reqTmpl.versionTime) {
					pw.writer.curTemplate = pw.reqTmpl
					promoted = true
				}
			}
		}
	}
	if promoted {
		if co.optimizer == nil {
			forceReconnect = true
		} else {
			if !co.optimizer.isMultiplexing() {
				forceReconnect = true
			}
		}
	}

	arc, ch, err = co.getStream(arc, forceReconnect)
	if err != nil {
		return err
	}

	pw.attemptCount = pw.attemptCount + 1
	if co.optimizer != nil {
		err = co.optimizer.optimizeSend((*arc), pw)
		if err != nil {
			// Reset optimizer state on error.
			co.optimizer.signalReset()
		}
	} else {
		// No optimizer present, send a fully populated request.
		err = (*arc).Send(pw.constructFullRequest(true))
	}
	if err != nil {
		if shouldReconnect(err) {
			metricCtx := co.ctx // start with the ctx that must be present
			if pw.writer != nil {
				metricCtx = pw.writer.ctx // the writer ctx bears the stream/origin tagging, so prefer it.
			}
			if tagCtx, tagErr := tag.New(metricCtx, tag.Insert(keyError, grpcstatus.Code(err).String())); tagErr == nil {
				metricCtx = tagCtx
			}
			recordStat(metricCtx, AppendRequestReconnects, 1)
			// if we think this connection is unhealthy, force a reconnect on the next send.
			co.reconnect = true
		}
		return err
	}

	// Compute numRows, once we pass ownership to the channel the request may be
	// cleared.
	var numRows int64
	if r := pw.req.GetProtoRows(); r != nil {
		if pr := r.GetRows(); pr != nil {
			numRows = int64(len(pr.GetSerializedRows()))
		}
	}
	statsOnExit = func(ctx context.Context) {
		// these will get recorded once we exit the critical section.
		// TODO: resolve open questions around what labels should be attached (connection, streamID, etc)
		recordStat(ctx, AppendRequestRows, numRows)
		recordStat(ctx, AppendRequests, 1)
		recordStat(ctx, AppendRequestBytes, int64(pw.reqSize))
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
	// We need to (re)open a connection.  Cleanup previous connection, channel, and context if they are present.
	if co.arc != nil && (*co.arc) != (storagepb.BigQueryWrite_AppendRowsClient)(nil) {
		(*co.arc).CloseSend()
	}
	if co.pending != nil {
		close(co.pending)
	}
	if co.cancel != nil {
		co.cancel()
		co.ctx, co.cancel = context.WithCancel(co.pool.ctx)
	}

	co.arc = new(storagepb.BigQueryWrite_AppendRowsClient)
	// We're going to (re)open the connection, so clear any optimizer state.
	if co.optimizer != nil {
		co.optimizer.signalReset()
	}
	*co.arc, co.pending, co.err = co.pool.openWithRetry(co)
	return co.arc, co.pending, co.err
}

// enables testing
type streamClientFunc func(context.Context, ...gax.CallOption) (storagepb.BigQueryWrite_AppendRowsClient, error)

// connRecvProcessor is used to propagate append responses back up with the originating write requests.  It
// It runs as a goroutine.  A connection object allows for reconnection, and each reconnection establishes a new
// processing gorouting and backing channel.
func connRecvProcessor(ctx context.Context, co *connection, arc storagepb.BigQueryWrite_AppendRowsClient, ch <-chan *pendingWrite) {
	for {
		select {
		case <-ctx.Done():
			// Context is done, so we're not going to get further updates.  Mark all work left in the channel
			// with the context error.  We don't attempt to re-enqueue in this case.
			for {
				pw, ok := <-ch
				if !ok {
					return
				}
				// It's unlikely this connection will recover here, but for correctness keep the flow controller
				// state correct by releasing.
				co.release(pw)
				pw.markDone(nil, ctx.Err())
			}
		case nextWrite, ok := <-ch:
			if !ok {
				// Channel closed, all elements processed.
				return
			}
			// block until we get a corresponding response or err from stream.
			resp, err := arc.Recv()
			co.release(nextWrite)
			if err != nil {
				// The Recv() itself yielded an error.  We increment AppendResponseErrors by one, tagged by the status
				// code.
				status := grpcstatus.Convert(err)
				metricCtx := ctx
				if tagCtx, tagErr := tag.New(ctx, tag.Insert(keyError, codes.Code(status.Code()).String())); tagErr == nil {
					metricCtx = tagCtx
				}
				recordStat(metricCtx, AppendResponseErrors, 1)

				nextWrite.writer.processRetry(nextWrite, co, nil, err)
				continue
			}
			// Record that we did in fact get a response from the backend.
			recordStat(ctx, AppendResponses, 1)

			if status := resp.GetError(); status != nil {
				// The response was received successfully, but the response embeds a status error in the payload.
				// Increment AppendResponseErrors, tagged by status code.
				metricCtx := ctx
				if tagCtx, tagErr := tag.New(ctx, tag.Insert(keyError, codes.Code(status.GetCode()).String())); tagErr == nil {
					metricCtx = tagCtx
				}
				recordStat(metricCtx, AppendResponseErrors, 1)
				respErr := grpcstatus.ErrorProto(status)

				nextWrite.writer.processRetry(nextWrite, co, resp, respErr)

				continue
			}
			// We had no error in the receive or in the response.  Mark the write done.
			nextWrite.markDone(resp, nil)
		}
	}
}

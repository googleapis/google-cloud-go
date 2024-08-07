// Copyright 2024 Google LLC
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
	"container/list"
	"context"
	"fmt"
	"sync"

	"cloud.google.com/go/bigquery/storage/apiv1/storagepb"
	"go.opencensus.io/tag"
	"google.golang.org/grpc/codes"
	grpcstatus "google.golang.org/grpc/status"
)

var globalQueueId string = "GLOBAL_FIFO_QUEUE"
var errDrainDisallowed = fmt.Errorf("cannot drain queue when still open, please close")
var errDestinationEmpty = fmt.Errorf("specific queue destination is empty or not present")

// pendingQueue is responsible for maintaining the queue of pendingWrites that have been sent
// and are awaiting acknowledgement.  The default behavior of an AppendRows connection is to
// respect global FIFO ordering, but for multiplex scenarios where writes are being interleaved
// the backend can respect per-destination ordering.
type pendingQueue struct {
	multiQueue bool
	mu         sync.Mutex
	dests      map[string]*list.List
	// waitingCh is used to signal messages are present in the queue.
	waitingCh chan struct{}
	onceClose *sync.Once
	closed    bool
}

func newPendingQueue(enableMultipleQueue bool, maxDepth int) *pendingQueue {
	return &pendingQueue{
		multiQueue: enableMultipleQueue,
		dests:      make(map[string]*list.List),
		waitingCh:  make(chan struct{}, maxDepth),
		onceClose:  &sync.Once{},
	}
}

// close signals the queue is closed for additions, but can still be drained.
func (pq *pendingQueue) closeAdd() {
	pq.onceClose.Do(func() {
		pq.mu.Lock()
		defer pq.mu.Unlock()
		pq.closed = true
		close(pq.waitingCh)
	})
}

func (pq *pendingQueue) addPending(pw *pendingWrite) error {
	if pw == nil {
		return fmt.Errorf("won't enqueue nil writes")
	}
	pq.mu.Lock()
	defer pq.mu.Unlock()
	if pq.closed {
		return fmt.Errorf("queue already closed")
	}
	dest := pw.writeStreamID
	if !pq.multiQueue {
		dest = globalQueueId
	}
	l, ok := pq.dests[dest]
	if !ok {
		// subqueue not yet present, create it.
		l = list.New()
		pq.dests[dest] = l
	}
	l.PushBack(pw)
	pq.waitingCh <- struct{}{}
	return nil
}

// listDests returns the currently available queues and the number of elements assigned to each.
func (pq *pendingQueue) listDests() map[string]int {
	pq.mu.Lock()
	defer pq.mu.Unlock()
	m := make(map[string]int)
	for k, l := range pq.dests {
		m[k] = l.Len()
	}
	if len(m) == 0 {
		return nil
	}
	return m
}

// dequeue provides the next element in the given destination.
//
// if the pendingQueue is not configured to support multiple destinations, the next message is grabbed from the
// global queue regardless of the provided destination.
//
// It does not consume waitingCh.
func (pq *pendingQueue) dequeue(destId string) (*pendingWrite, error) {
	pq.mu.Lock()
	defer pq.mu.Unlock()
	if !pq.multiQueue {
		destId = globalQueueId
	}
	l, ok := pq.dests[destId]
	if !ok {
		return nil, errDestinationEmpty
	}
	e := l.Front()
	l.Remove(e)
	if l.Len() == 0 {
		delete(pq.dests, destId)
	}
	return e.Value.(*pendingWrite), nil
}

// drain returns a message from one of the non-empty streams in the queue.
// If the queue is fully empty it will return nil, otherwise it will choose
// from a random destination.
func (pq *pendingQueue) drain() (*pendingWrite, error) {
	pq.mu.Lock()
	defer pq.mu.Unlock()
	if !pq.closed {
		return nil, errDrainDisallowed
	}
	if len(pq.dests) == 0 {
		// all queues empty
		return nil, nil
	}
	// leverage unpredicatable go ordering by ranging the map.
	for dest, l := range pq.dests {
		e := l.Front()
		l.Remove(e)
		if l.Len() == 0 {
			delete(pq.dests, dest)
		}
		return e.Value.(*pendingWrite), nil
	}
	// we should only ever get here if something is violating locking expectations.
	return nil, fmt.Errorf("pending queue in an inconsistent state")
}

// This function is a successor to connRecvProcessor, but uses a pendingQueue rather than a single buffered channel.
func connRecvQueueProcessor(ctx context.Context, co *connection, arc storagepb.BigQueryWrite_AppendRowsClient, pq *pendingQueue) {
	for {
		select {
		case <-ctx.Done():
			// Channel context is done, which means we're not getting further updates on in flight appends and should
			// process everything left in the existing channel/connection.
			doneErr := ctx.Err()
			if doneErr == context.Canceled {
				// This is a special case.  Connection recovery ends up cancelling a context as part of a reconnection, and with
				// request retrying enabled we can possibly re-enqueue writes.  To allow graceful retry for this behavior, we
				// we translate this to an rpc status error to avoid doing things like introducing context errors as part of the retry predicate.
				//
				// The tradeoff here is that write retries may roundtrip multiple times for something like a pool shutdown, even though the final
				// outcome would result in an error.
				doneErr = errConnectionCanceled
			}
			for {
				// we cannot proceed, so let's close the queue for additions.
				pq.closeAdd()
				// process the remaining elements in the queue.
				_, ok := <-pq.waitingCh
				if !ok {
					return
				}
				pw, err := pq.drain()
				if err != nil {
					// Something terribly wrong has occurred, and we're unable to drain
					// and don't know if something is stuck in the queue.
					panic(fmt.Sprintf("connection %q queueing cannot be drained: %v", co.id, err))
				}
				// This connection will not recover, but still attempt to keep flow controller state consistent.
				co.release(pw)

				// TODO:  Determine if/how we should report this case, as we have no viable context for propagating.

				// Because we can't tell locally if this write is done, we pass it back to the retrier for possible re-enqueue.
				pw.writer.processRetry(pw, co, nil, doneErr)
			}
		case _, ok := <-pq.waitingCh:
			if !ok {
				// Channel closed, all elements processed.
				return
			}
			// retrieve the next response, so we can lookup the pending write.
			resp, err := arc.Recv()
			if err != nil {
				// Our recv has become unhealthy, and so we invoke emptyQueue which
				// handles draining/retrying any elements remaining in the queue.
				emptyQueue(pq, co, err)
			}
			// Record that we did in fact get a response from the backend.
			recordStat(ctx, AppendResponses, 1)

			// get the destination from the response, and dequeue the next write
			// from that destination.
			dest := resp.GetWriteStream()
			nextWrite, err := pq.dequeue(dest)
			if err != nil {
				panic(fmt.Sprintf("attempted to dequeue from %q and failed: %v", dest, err))
			}
			// release the flow controller
			co.release(nextWrite)

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

// utility mechanism for draining a pending queue of all remaining pending writes.
// it ensures the queue is closed for additions to avoid further use of the connection/queue.
func emptyQueue(pq *pendingQueue, srcConn *connection, srcErr error) {
	// ensure the queue is no longer accepting writes.
	pq.closeAdd()

	for {
		next, err := pq.drain()
		if err != nil {
			panic(fmt.Sprintf("emptyQueue errored on drain(): %v", err))
		}
		if next == nil {
			// No elements left.  Done.
			break
		}

		next.writer.processRetry(next, srcConn, nil, srcErr)
	}

}

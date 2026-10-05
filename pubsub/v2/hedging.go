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

package pubsub

import (
	"context"
	"errors"
	"fmt"
	"math"
	"sync"
	"time"

	pb "cloud.google.com/go/pubsub/v2/apiv1/pubsubpb"
	gax "github.com/googleapis/gax-go/v2"
	semconv "go.opentelemetry.io/otel/semconv/v1.26.0"
	"go.opentelemetry.io/otel/trace"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
)

const (
	defaultHedgingDelay     time.Duration = 1 * time.Second
	defaultHedgingRatio     float64       = 0.1
	defaultMaxHedgingTokens int64         = 50

	minHedgingDelay       time.Duration = 100 * time.Millisecond
	maxHedgingDelay       time.Duration = 10 * time.Second
	maxHedgingTokensLimit int64         = 250
	minHedgingRatio       float64       = 0.001
	maxHedgingRatio       float64       = 0.2

	// tokenScaleFactor scales token bucket calculations to fixed-point integers
	// (1 token = 1,000 milli-tokens) to avoid floating-point accumulation drift.
	tokenScaleFactor int64 = 1000
)

var errPublisherHedgingAndOrderingEnabled = errors.New("pubsub: Hedging and MessageOrdering cannot both be enabled on the Publisher")

// HedgingSettings enables the publisher to issue hedged requests.
type HedgingSettings struct {
	// Delay configures the delay of when the hedged RPC should be attempted.
	// Default is 1s.
	// Must be between >= 0.1s and <= 10s.
	Delay time.Duration

	// MaxTokens configures the upper bound of the internal token bucket limiter.
	//
	// Every time an RPC exceeds the hedging delay, it consumes 1 token to fire
	// a hedged request. Therefore, MaxTokens bounds the number of
	// hedged requests the client can issue in a period of time if it is not
	// refilled by successful requests, see RefillRatio.
	//
	// Default is 50.
	// Must be between > 0 and <= 250.
	MaxTokens int64

	// RefillRatio is the amount of tokens added to the bucket per successful publish.
	// Represents the % of requests that can be hedged.
	// Default is 0.1.
	// Must be between >= 0.001 and <= 0.2.
	RefillRatio float64
}

func validateHedgingSettings(hs *HedgingSettings) error {
	if hs == nil {
		return nil
	}
	if hs.Delay != 0 && (hs.Delay < minHedgingDelay || hs.Delay > maxHedgingDelay) {
		return fmt.Errorf("pubsub: HedgingSettings.Delay (%v) must be between %v and %v", hs.Delay, minHedgingDelay, maxHedgingDelay)
	}
	if hs.MaxTokens != 0 && (hs.MaxTokens <= 0 || hs.MaxTokens > maxHedgingTokensLimit) {
		return fmt.Errorf("pubsub: HedgingSettings.MaxTokens (%d) must be > 0 and <= %d", hs.MaxTokens, maxHedgingTokensLimit)
	}
	if hs.RefillRatio != 0 && (hs.RefillRatio < minHedgingRatio || hs.RefillRatio > maxHedgingRatio) {
		return fmt.Errorf("pubsub: HedgingSettings.RefillRatio (%v) must be between %v and %v", hs.RefillRatio, minHedgingRatio, maxHedgingRatio)
	}
	return nil
}

type attemptResult struct {
	res *pb.PublishResponse
	err error
	id  int
}

// hedgedRequest is a scheduled hedged attempt waiting in the hedging queue.
//
// It intentionally holds no reference to the batch payload. Every batch that
// is eligible for hedging enqueues one of these, and it stays in the queue
// until sendAfter even if the publish resolved long before. The payload lives
// on the cancellationSharer instead, which drops it as soon as the publish
// resolves.
type hedgedRequest struct {
	attemptID int
	sendAfter time.Time
	cs        *cancellationSharer
}

func (req *hedgedRequest) isDone() bool {
	return req.cs.isDone()
}

// hedgeBatch is the per-batch state shared by all hedged attempts of a single
// publish.
type hedgeBatch struct {
	ctx       context.Context
	startTime time.Time
	resCh     chan attemptResult
	pbMsgs    []*pb.PubsubMessage
	gaxOpts   []gax.CallOption
	bmsgs     []*bundledMessage
}

// cancellationSharer coordinates cancellation between all publish attempts.
// When one attempt completes, it cancels all other attempts to minimize
// duplicate messages on the server.
//
// It also owns the batch payload used by hedged attempts. The payload is
// released when the publish resolves (win or cancelAll), and inflight tracks
// hedged attempts that are still using it so the publisher can wait for them
// to exit before handing results (and ownership of message data) back to the
// user.
type cancellationSharer struct {
	mu       sync.Mutex
	cancels  map[int]context.CancelFunc
	done     bool
	nextID   int
	batch    *hedgeBatch
	inflight sync.WaitGroup
}

func newCancellationSharer(batch *hedgeBatch) *cancellationSharer {
	return &cancellationSharer{
		cancels: make(map[int]context.CancelFunc),
		batch:   batch,
	}
}

func (cs *cancellationSharer) isDone() bool {
	cs.mu.Lock()
	defer cs.mu.Unlock()
	return cs.done
}

// add registers a cancel function and returns its ID. Returns -1 if already done.
func (cs *cancellationSharer) add(cancel context.CancelFunc) int {
	cs.mu.Lock()
	defer cs.mu.Unlock()
	if cs.done {
		cancel()
		return -1
	}
	id := cs.nextID
	cs.nextID++
	cs.cancels[id] = cancel
	return id
}

// acquireHedge registers a new hedged attempt. It returns the attempt ID, a
// cancellable context derived from the batch context, and the batch payload.
// ok is false if the publish has already resolved, in which case the caller
// must not send the attempt. On success the caller must call releaseHedge once
// it no longer uses the payload.
func (cs *cancellationSharer) acquireHedge() (id int, ctx context.Context, b *hedgeBatch, ok bool) {
	cs.mu.Lock()
	defer cs.mu.Unlock()
	if cs.done || cs.batch == nil {
		return 0, nil, nil, false
	}
	ctx, cancel := context.WithCancel(cs.batch.ctx)
	id = cs.nextID
	cs.nextID++
	cs.cancels[id] = cancel
	// Add is done under mu while !done, so it always happens before the Wait in
	// wait(), which is only called after cancelAll has set done.
	cs.inflight.Add(1)
	return id, ctx, cs.batch, true
}

// releaseHedge marks a hedged attempt acquired via acquireHedge as finished.
func (cs *cancellationSharer) releaseHedge() {
	cs.inflight.Done()
}

// win marks the coordinator as resolved by winnerID and cancels all other attempts.
func (cs *cancellationSharer) win(winnerID int) {
	cs.mu.Lock()
	defer cs.mu.Unlock()
	if cs.done {
		return
	}
	cs.done = true
	cs.batch = nil
	for id, cancel := range cs.cancels {
		if id != winnerID {
			cancel()
		}
	}
}

// cancelAll cancels all registered attempt contexts and releases the batch
// payload. It is safe to call more than once.
func (cs *cancellationSharer) cancelAll() {
	cs.mu.Lock()
	defer cs.mu.Unlock()
	cs.done = true
	cs.batch = nil
	for _, cancel := range cs.cancels {
		cancel()
	}
	cs.cancels = nil
}

// wait blocks until every hedged attempt acquired via acquireHedge has called
// releaseHedge. It must be called after cancelAll.
func (cs *cancellationSharer) wait() {
	cs.inflight.Wait()
}

// publishHedged publishes pbMsgs with hedging enabled. The original attempt
// runs synchronously on the calling goroutine; hedged attempts are scheduled
// through the hedging queue and fire every hedgingDelay while tokens are
// available. The first successful response wins and cancels the others.
//
// It returns only after every hedged attempt for this batch has exited, so the
// caller may release flow control and hand results back to the user.
func (t *Publisher) publishHedged(ctx context.Context, start time.Time, pbMsgs []*pb.PubsubMessage, bms []*bundledMessage, gaxOpts []gax.CallOption) (*pb.PublishResponse, error) {
	resCh := make(chan attemptResult, 1)
	cs := newCancellationSharer(&hedgeBatch{
		ctx:       ctx,
		startTime: start,
		resCh:     resCh,
		pbMsgs:    pbMsgs,
		gaxOpts:   gaxOpts,
		bmsgs:     bms,
	})

	t.enqueueHedgedRequest(&hedgedRequest{
		attemptID: 1,
		sendAfter: start.Add(t.hedgingDelay),
		cs:        cs,
	})

	mainCtx, mainCancel := context.WithCancel(ctx)
	mainID := cs.add(mainCancel)
	mainCtx = metadata.AppendToOutgoingContext(
		mainCtx,
		pubsubClientTelemetryHeader,
		encodePubsubClientTelemetry(0, start),
	)
	r, e := t.c.TopicAdminClient.Publish(mainCtx, &pb.PublishRequest{
		Topic:    t.name,
		Messages: pbMsgs,
	}, gaxOpts...)

	if e == nil {
		select {
		case resCh <- attemptResult{res: r, err: nil, id: mainID}:
			cs.win(mainID)
		default:
		}
	} else {
		// Terminal error on original attempt cancels any pending hedged attempts
		// unless a hedged attempt has already succeeded.
		cs.cancelAll()
	}

	var res *pb.PublishResponse
	var err error
	select {
	case winner := <-resCh:
		res = winner.res
		err = winner.err
		if err == nil {
			t.replenishHedgingTokens()
		}
	default:
		res = r
		err = e
	}

	// Resolve the publish before releasing flow control and setting
	// results: cancel outstanding hedged attempts, drop the shared
	// payload so queued hedgedRequests don't keep it alive, and wait for
	// in-flight hedges to exit. After this, no hedge goroutine can still
	// be reading message Data/Attributes, which the user may reuse as
	// soon as PublishResult.Get returns.
	cs.cancelAll()
	cs.wait()
	return res, err
}

func (t *Publisher) stopHedging() {
	t.hedgingMu.Lock()
	t.hedgingStopped = true
	if t.hedgingTimer != nil {
		t.hedgingTimer.Stop()
		t.hedgingTimer = nil
	}
	t.hedgingQueue = nil
	t.hedgingMu.Unlock()
}

func (t *Publisher) enqueueHedgedRequest(req *hedgedRequest) {
	t.hedgingMu.Lock()
	defer t.hedgingMu.Unlock()
	if t.hedgingStopped {
		return
	}
	t.hedgingQueue = append(t.hedgingQueue, req)
	if len(t.hedgingQueue) == 1 {
		delay := time.Until(req.sendAfter)
		if delay < 0 {
			delay = 0
		}
		t.hedgingTimer = time.AfterFunc(delay, t.processHedgingQueue)
	}
}

func (t *Publisher) processHedgingQueue() {
	t.hedgingMu.Lock()
	if t.hedgingStopped {
		t.hedgingMu.Unlock()
		return
	}

	now := time.Now()
	var ready []*hedgedRequest
	for len(t.hedgingQueue) > 0 {
		head := t.hedgingQueue[0]
		if head.sendAfter.After(now) {
			break
		}
		ready = append(ready, head)
		t.hedgingQueue[0] = nil // don't retain the popped request in the backing array
		t.hedgingQueue = t.hedgingQueue[1:]
	}

	if len(t.hedgingQueue) > 0 {
		nextDelay := time.Until(t.hedgingQueue[0].sendAfter)
		if nextDelay < 0 {
			nextDelay = 0
		}
		t.hedgingTimer = time.AfterFunc(nextDelay, t.processHedgingQueue)
	} else {
		t.hedgingTimer = nil
	}
	t.hedgingMu.Unlock()

	for _, req := range ready {
		if req.isDone() {
			continue
		}
		t.hedgingMu.Lock()
		hasToken := t.hedgingTokenBucket >= tokenScaleFactor
		if hasToken {
			t.hedgingTokenBucket -= tokenScaleFactor
		}
		t.hedgingMu.Unlock()

		// If the token bucket is empty (< 1 token), the scheduled hedged attempt is
		// discarded and NOT returned to the queue.
		if hasToken {
			go t.fireHedgedAttempt(req)
		}
	}
}

func (t *Publisher) fireHedgedAttempt(req *hedgedRequest) {
	id, hedgedCtx, b, ok := req.cs.acquireHedge()
	if !ok {
		return
	}
	defer req.cs.releaseHedge()
	if b.ctx.Err() != nil {
		return
	}

	t.enqueueHedgedRequest(&hedgedRequest{
		attemptID: req.attemptID + 1,
		sendAfter: time.Now().Add(t.hedgingDelay),
		cs:        req.cs,
	})

	var timeout time.Duration
	if deadline, ok := b.ctx.Deadline(); ok {
		timeout = time.Until(deadline)
	} else {
		timeout = t.PublishSettings.Timeout - time.Since(b.startTime)
	}
	if timeout <= 0 {
		return
	}
	if timeout > 10*time.Second {
		timeout = 10 * time.Second
	}

	// Hedged attempts should not be retried. Any errors from hedged attempts
	// are discarded so they do not prematurely fail the overall publish request.
	opts := append([]gax.CallOption(nil), b.gaxOpts...)
	opts = append(opts,
		gax.WithRetry(func() gax.Retryer { return gax.OnCodes([]codes.Code{}, gax.Backoff{}) }),
		gax.WithTimeout(timeout),
	)

	if t.enableTracing {
		for _, m := range b.bmsgs {
			m.createSpan.AddEvent(eventHedgedPublishStart, trace.WithAttributes(semconv.MessagingBatchMessageCount(len(b.bmsgs))))
		}
	}

	hedgedCtx = metadata.AppendToOutgoingContext(
		hedgedCtx,
		pubsubClientTelemetryHeader,
		encodePubsubClientTelemetry(req.attemptID, b.startTime),
	)

	r, e := t.c.TopicAdminClient.Publish(hedgedCtx, &pb.PublishRequest{
		Topic:    t.name,
		Messages: b.pbMsgs,
	}, opts...)

	if t.enableTracing {
		for _, m := range b.bmsgs {
			m.createSpan.AddEvent(eventHedgedPublishEnd)
		}
	}

	if e == nil {
		select {
		case b.resCh <- attemptResult{res: r, err: nil, id: id}:
			req.cs.win(id)
		default:
		}
	}
}

func (t *Publisher) replenishHedgingTokens() {
	t.hedgingMu.Lock()
	defer t.hedgingMu.Unlock()

	ratio := defaultHedgingRatio
	maxTokens := defaultMaxHedgingTokens
	if t.PublishSettings.HedgingSettings != nil {
		if t.PublishSettings.HedgingSettings.RefillRatio > 0 {
			ratio = t.PublishSettings.HedgingSettings.RefillRatio
		}
		if t.PublishSettings.HedgingSettings.MaxTokens > 0 {
			maxTokens = t.PublishSettings.HedgingSettings.MaxTokens
		}
	}

	refillMilliTokens := int64(math.Round(ratio * float64(tokenScaleFactor)))
	maxMilliTokens := maxTokens * tokenScaleFactor

	if t.hedgingTokenBucket < maxMilliTokens {
		t.hedgingTokenBucket += refillMilliTokens
		if t.hedgingTokenBucket > maxMilliTokens {
			t.hedgingTokenBucket = maxMilliTokens
		}
	}
}

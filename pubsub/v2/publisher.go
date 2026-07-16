// Copyright 2025 Google LLC
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
	"log"
	"math"
	"runtime"
	"strings"
	"sync"
	"time"

	ipubsub "cloud.google.com/go/internal/pubsub"
	pb "cloud.google.com/go/pubsub/v2/apiv1/pubsubpb"
	"cloud.google.com/go/pubsub/v2/internal/scheduler"
	gax "github.com/googleapis/gax-go/v2"
	"go.opencensus.io/stats"
	"go.opencensus.io/tag"
	"go.opentelemetry.io/otel/attribute"
	semconv "go.opentelemetry.io/otel/semconv/v1.26.0"
	"go.opentelemetry.io/otel/trace"
	"google.golang.org/api/support/bundler"
	"google.golang.org/grpc"
	"google.golang.org/grpc/encoding/gzip"
	"google.golang.org/protobuf/proto"
)

const (
	// MaxPublishRequestCount is the maximum number of messages that can be in
	// a single publish request, as defined by the PubSub service.
	MaxPublishRequestCount = 1000

	// MaxPublishRequestBytes is the maximum size of a single publish request
	// in bytes, as defined by the PubSub service.
	MaxPublishRequestBytes = 1e7

	// hedging is disabled by default but if HedgingSettings is set to a zero value,
	// we will use a default value of 50ms.
	defaultHedgingDelay time.Duration = 50 * time.Millisecond
)

// Default Token Bucket configurations
const (
	defaultHedgingRatio     float64 = 0.1
	defaultMaxHedgingTokens float64 = 100.0
)

// ErrOversizedMessage indicates that a message's size exceeds MaxPublishRequestBytes.
var ErrOversizedMessage = bundler.ErrOversizedItem

// Publisher is a reference to a PubSub publisher, associated with a single topic.
//
// The methods of Publisher are safe for use by multiple goroutines.
type Publisher struct {
	c *Client
	// The fully qualified identifier for the topic, in the format "projects/<projid>/topics/<name>"
	name string

	// Settings for publishing messages. All changes must be made before the
	// first call to Publish. The default is DefaultPublishSettings.
	PublishSettings PublishSettings

	mu        sync.RWMutex
	stopped   bool
	scheduler *scheduler.PublishScheduler

	flowController

	// EnableMessageOrdering enables delivery of ordered keys.
	EnableMessageOrdering bool

	// enableTracing enables OTel tracing of Pub/Sub messages on this topic.
	// This is configured at client instantiation, and allows
	// disabling tracing even when a tracer provider is detectd.
	enableTracing bool

	// if non-zero, publish requests will be hedged after this delay.
	// The first request will be sent immediately, and if it has not
	// completed after the hedging delay, a second request will be sent.
	// The first response to arrive will be used, and the other request
	// will be cancelled.
	hedgingDelay time.Duration

	// the current number of tokens which limits the number of hedged requests
	// that can be sent concurrently.
	hedgingTokenBucket float64

	hedgingMu      sync.Mutex
	hedgingQueue   []*hedgedRequest
	hedgingTimer   *time.Timer
	hedgingStopped bool
}

type attemptResult struct {
	res *pb.PublishResponse
	err error
	id  int
}

type hedgedRequest struct {
	attemptID int
	sendAfter time.Time
	resCh     chan attemptResult
	cs        *cancellationSharer
	ctx       context.Context
	pbMsgs    []*pb.PubsubMessage
	gaxOpts   []gax.CallOption
}

func (req *hedgedRequest) isDone() bool {
	return req.cs.isDone()
}

// cancellationSharer coordinates cancellation between all publish attempts.
// When one attempt completes, it cancels all other attempts to minimize
// duplicate messages on the server.
type cancellationSharer struct {
	mu      sync.Mutex
	cancels map[int]context.CancelFunc
	done    bool
	nextID  int
}

func newCancellationSharer() *cancellationSharer {
	return &cancellationSharer{
		cancels: make(map[int]context.CancelFunc),
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

// win marks the coordinator as resolved by winnerID and cancels all other attempts.
func (cs *cancellationSharer) win(winnerID int) {
	cs.mu.Lock()
	defer cs.mu.Unlock()
	if cs.done {
		return
	}
	cs.done = true
	for id, cancel := range cs.cancels {
		if id != winnerID {
			cancel()
		}
	}
}

// cancelAll cancels all registered attempt contexts.
func (cs *cancellationSharer) cancelAll() {
	cs.mu.Lock()
	defer cs.mu.Unlock()
	cs.done = true
	for _, cancel := range cs.cancels {
		cancel()
	}
}

// PublishSettings control the bundling of published messages.
type PublishSettings struct {
	// Publish a non-empty batch after this delay has passed.
	DelayThreshold time.Duration

	// Publish a batch when it has this many messages. The maximum is
	// MaxPublishRequestCount.
	CountThreshold int

	// Publish a batch when its size in bytes reaches this value.
	ByteThreshold int

	// The number of goroutines used in each of the data structures that are
	// involved along the the Publish path. Adjusting this value adjusts
	// concurrency along the publish path.
	//
	// Defaults to a multiple of GOMAXPROCS.
	NumGoroutines int

	// The maximum time that the client will attempt to publish a bundle of messages.
	Timeout time.Duration

	// FlowControlSettings defines publisher flow control settings.
	FlowControlSettings FlowControlSettings

	// EnableCompression enables transport compression for Publish operations
	EnableCompression bool

	// CompressionBytesThreshold defines the threshold (in bytes) above which messages
	// are compressed for transport. Only takes effect if EnableCompression is true.
	CompressionBytesThreshold int

	HedgingSettings *HedgingSettings
}

type HedgingSettings struct {
	Delay time.Duration

	// MaxHedgedAttempts is the maximum number of hedged requests to send.
	// If 0, there is no limit on hedged attempts (dynamic multi-hedging).
	// If set to 1, at most 1 hedged request is sent per bundle (single hedging).
	MaxHedgedAttempts int

	// MaxTokens is the maximum number of tokens for the hedging token bucket.
	MaxTokens float64

	// TokenRatio is the amount of tokens added to the bucket per successful publish.
	TokenRatio float64
}

func (ps *PublishSettings) shouldCompress(batchSize int) bool {
	return ps.EnableCompression && batchSize > ps.CompressionBytesThreshold
}

// DefaultPublishSettings holds the default values for topics' PublishSettings.
var DefaultPublishSettings = PublishSettings{
	DelayThreshold: 10 * time.Millisecond,
	CountThreshold: 100,
	ByteThreshold:  1e6,
	Timeout:        60 * time.Second,
	FlowControlSettings: FlowControlSettings{
		MaxOutstandingMessages: 1000,
		MaxOutstandingBytes:    -1,
		LimitExceededBehavior:  FlowControlIgnore,
	},
	// Publisher compression defaults matches Java's defaults
	// https://github.com/googleapis/java-pubsub/blob/7d33e7891db1b2e32fd523d7655b6c11ea140a8b/google-cloud-pubsub/src/main/java/com/google/cloud/pubsub/v1/Publisher.java#L717-L718
	EnableCompression:         false,
	CompressionBytesThreshold: 240,
}

// Publisher constructs a publisher client from either a topicID or a topic name, otherwise known as a full path.
//
// The client created is a reference and does not return any errors if the topic does not exist.
// Errors will be returned when attempting to Publish instead.
// If a Publisher's Publish method is called, it has background goroutines
// associated with it. Clean them up by calling Publisher.Stop.
//
// It is best practice to reuse the Publisher when publishing to the same topic.
// Avoid creating many Publisher instances if you use them to publish.
func (c *Client) Publisher(topicNameOrID string) *Publisher {
	s := strings.Split(topicNameOrID, "/")
	// The string looks like a properly formatted topic name, use it directly.
	if len(s) == 4 {
		return newPublisher(c, topicNameOrID)
	}
	// In all other cases, treat the string as the topicID, even if misformatted.
	return newPublisher(c, fmt.Sprintf("projects/%s/topics/%s", c.projectID, topicNameOrID))
}

func newPublisher(c *Client, name string) *Publisher {
	var maxTokens float64 = defaultMaxHedgingTokens
	if DefaultPublishSettings.HedgingSettings != nil && DefaultPublishSettings.HedgingSettings.MaxTokens > 0 {
		maxTokens = DefaultPublishSettings.HedgingSettings.MaxTokens
	}

	return &Publisher{
		c:                  c,
		name:               name,
		PublishSettings:    DefaultPublishSettings,
		enableTracing:      c.enableTracing,
		hedgingTokenBucket: maxTokens,
	}
}

// ID returns the unique identifier of the topic within its project.
func (t *Publisher) ID() string {
	slash := strings.LastIndex(t.name, "/")
	if slash == -1 {
		// name is not a fully-qualified name.
		panic("bad topic name")
	}
	return t.name[slash+1:]
}

// String returns the printable globally unique name for the topic.
func (t *Publisher) String() string {
	return t.name
}

// ErrPublisherStopped indicates that topic has been stopped and further publishing will fail.
var ErrPublisherStopped = errors.New("pubsub: Stop has been called for this publisher")

// A PublishResult holds the result from a call to Publish.
//
// Call Get to obtain the result of the Publish call. Example:
//
//	// Get blocks until Publish completes or ctx is done.
//	id, err := r.Get(ctx)
//	if err != nil {
//	    // TODO: Handle error.
//	}
type PublishResult = ipubsub.PublishResult

var errPublisherOrderingNotEnabled = errors.New("Publisher.EnableMessageOrdering=false, but an OrderingKey was set in Message. Please remove the OrderingKey or turn on Publisher.EnableMessageOrdering")
var errPublisherHedgingAndOrderingEnabled = errors.New("pubsub: Hedging and MessageOrdering cannot both be enabled on the Publisher")

// Publish publishes msg to the topic asynchronously. Messages are batched and
// sent according to the topic's PublishSettings. Publish never blocks.
//
// Publish returns a non-nil PublishResult which will be ready when the
// message has been sent (or has failed to be sent) to the server.
//
// Publish creates goroutines for batching and sending messages. These goroutines
// need to be stopped by calling t.Stop(). Once stopped, future calls to Publish
// will immediately return a PublishResult with an error.
func (t *Publisher) Publish(ctx context.Context, msg *Message) *PublishResult {
	var createSpan trace.Span
	if t.enableTracing {
		opts := getPublishSpanAttributes(t.c.projectID, t.name, msg)
		opts = append(opts, trace.WithAttributes(semconv.CodeFunction("Publish")))
		ctx, createSpan = startSpan(ctx, createSpanName, t.ID(), opts...)
	}
	ctx, err := tag.New(ctx, tag.Insert(keyStatus, "OK"), tag.Upsert(keyTopic, t.name))
	if err != nil {
		log.Printf("pubsub: cannot create context with tag in Publish: %v", err)
	}

	r := ipubsub.NewPublishResult()
	if !t.EnableMessageOrdering && msg.OrderingKey != "" {
		ipubsub.SetPublishResult(r, "", errPublisherOrderingNotEnabled)
		spanRecordError(createSpan, errPublisherOrderingNotEnabled)
		return r
	}
	if t.EnableMessageOrdering && t.PublishSettings.HedgingSettings != nil {
		ipubsub.SetPublishResult(r, "", errPublisherHedgingAndOrderingEnabled)
		spanRecordError(createSpan, errPublisherHedgingAndOrderingEnabled)
		return r
	}

	// Calculate the size of the encoded proto message by accounting
	// for the length of an individual PubSubMessage and Data/Attributes field.
	msgSize := proto.Size(&pb.PubsubMessage{
		Data:        msg.Data,
		Attributes:  msg.Attributes,
		OrderingKey: msg.OrderingKey,
	})
	if t.enableTracing {
		createSpan.SetAttributes(semconv.MessagingMessageBodySize(len(msg.Data)))
	}

	t.initBundler()
	t.mu.RLock()
	defer t.mu.RUnlock()
	if t.stopped {
		ipubsub.SetPublishResult(r, "", ErrPublisherStopped)
		spanRecordError(createSpan, ErrPublisherStopped)
		return r
	}

	var batcherSpan trace.Span
	var fcSpan trace.Span

	if t.enableTracing {
		_, fcSpan = startSpan(ctx, publishFCSpanName, "")
	}
	if err := t.flowController.acquire(ctx, msgSize); err != nil {
		t.scheduler.Pause(msg.OrderingKey)
		ipubsub.SetPublishResult(r, "", err)
		spanRecordError(fcSpan, err)
		return r
	}
	if t.enableTracing {
		fcSpan.End()
	}

	bmsg := &bundledMessage{
		msg:        msg,
		res:        r,
		size:       msgSize,
		createSpan: createSpan,
	}

	if t.enableTracing {
		_, batcherSpan = startSpan(ctx, batcherSpanName, "")
		bmsg.batcherSpan = batcherSpan

		// Inject the context from the first publish span rather than from flow control / batching.
		injectPropagation(ctx, msg)
	}

	if err := t.scheduler.Add(msg.OrderingKey, bmsg, msgSize); err != nil {
		t.flowController.release(ctx, msgSize)
		t.scheduler.Pause(msg.OrderingKey)
		ipubsub.SetPublishResult(r, "", err)
		spanRecordError(createSpan, err)
	}

	return r
}

// Stop sends all remaining published messages and stop goroutines created for handling
// publishing. Returns once all outstanding messages have been sent or have
// failed to be sent.
func (t *Publisher) Stop() {
	t.mu.Lock()
	noop := t.stopped || t.scheduler == nil
	t.stopped = true
	t.mu.Unlock()
	if noop {
		return
	}
	t.stopHedging()
	t.scheduler.FlushAndStop()
}

// Flush blocks until all remaining messages are sent.
func (t *Publisher) Flush() {
	if t.stopped || t.scheduler == nil {
		return
	}
	t.scheduler.Flush()
}

type bundledMessage struct {
	msg  *Message
	res  *PublishResult
	size int
	// createSpan is the entire publish createSpan (from user calling Publish to the publish RPC resolving).
	createSpan trace.Span
	// batcherSpan traces the message batching operation in publish scheduler.
	batcherSpan trace.Span
}

func (t *Publisher) initBundler() {
	t.mu.RLock()
	noop := t.stopped || t.scheduler != nil
	t.mu.RUnlock()
	if noop {
		return
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	// Must re-check, since we released the lock.
	if t.stopped || t.scheduler != nil {
		return
	}

	timeout := t.PublishSettings.Timeout

	workers := t.PublishSettings.NumGoroutines
	// Unless overridden, allow many goroutines per CPU to call the Publish RPC
	// concurrently. The default value was determined via extensive load
	// testing (see the loadtest subdirectory).
	if t.PublishSettings.NumGoroutines == 0 {
		workers = 25 * runtime.GOMAXPROCS(0)
	}

	t.scheduler = scheduler.NewPublishScheduler(workers, func(bundle interface{}) {
		// Use a context detached from the one passed to NewClient.
		ctx := context.Background()
		if timeout != 0 {
			var cancel func()
			ctx, cancel = context.WithTimeout(ctx, timeout)
			defer cancel()
		}
		bmsgs := bundle.([]*bundledMessage)
		if t.enableTracing {
			for _, m := range bmsgs {
				m.batcherSpan.End()
				m.createSpan.AddEvent(eventPublishStart, trace.WithAttributes(semconv.MessagingBatchMessageCount(len(bmsgs))))
			}
		}
		t.publishMessageBundle(ctx, bmsgs)
		if t.enableTracing {
			for _, m := range bmsgs {
				m.createSpan.AddEvent(eventPublishEnd)
				m.createSpan.End()
			}
		}
	})
	t.scheduler.DelayThreshold = t.PublishSettings.DelayThreshold
	t.scheduler.BundleCountThreshold = t.PublishSettings.CountThreshold
	if t.scheduler.BundleCountThreshold > MaxPublishRequestCount {
		t.scheduler.BundleCountThreshold = MaxPublishRequestCount
	}
	t.scheduler.BundleByteThreshold = t.PublishSettings.ByteThreshold

	fcs := DefaultPublishSettings.FlowControlSettings
	fcs.LimitExceededBehavior = t.PublishSettings.FlowControlSettings.LimitExceededBehavior
	if t.PublishSettings.FlowControlSettings.MaxOutstandingBytes > 0 {
		b := t.PublishSettings.FlowControlSettings.MaxOutstandingBytes
		fcs.MaxOutstandingBytes = b
	}
	if t.PublishSettings.FlowControlSettings.MaxOutstandingMessages > 0 {
		fcs.MaxOutstandingMessages = t.PublishSettings.FlowControlSettings.MaxOutstandingMessages
	}

	t.flowController = newPublisherFlowController(fcs)

	// Calculate the max limit of a single bundle. 5 comes from the number of bytes
	// needed to be reserved for encoding the PubsubMessage repeated field.
	t.scheduler.BundleByteLimit = MaxPublishRequestBytes - calcFieldSizeString(t.name) - 5

	// The max size of publish messages in a system should be handled by the flow controller,
	// not the scheduler or bundler. Disable this by setting to MaxInt.
	t.scheduler.BufferedByteLimit = math.MaxInt

	if t.PublishSettings.HedgingSettings != nil && !t.EnableMessageOrdering {
		t.hedgingDelay = t.PublishSettings.HedgingSettings.Delay
		if t.hedgingDelay == 0 {
			t.hedgingDelay = defaultHedgingDelay
		}
	}
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
		hasToken := t.hedgingTokenBucket >= 1.0
		if hasToken {
			t.hedgingTokenBucket -= 1.0
		}
		t.hedgingMu.Unlock()

		if hasToken {
			go t.fireHedgedAttempt(req)
		}
	}
}

func (t *Publisher) fireHedgedAttempt(req *hedgedRequest) {
	if req.isDone() || req.ctx.Err() != nil {
		return
	}

	maxHedged := t.PublishSettings.HedgingSettings.MaxHedgedAttempts
	if maxHedged == 0 || req.attemptID < maxHedged {
		t.enqueueHedgedRequest(&hedgedRequest{
			attemptID: req.attemptID + 1,
			sendAfter: time.Now().Add(t.hedgingDelay),
			resCh:     req.resCh,
			cs:        req.cs,
			ctx:       req.ctx,
			pbMsgs:    req.pbMsgs,
			gaxOpts:   req.gaxOpts,
		})
	}

	hedgedCtx, hedgedCancel := context.WithCancel(req.ctx)
	id := req.cs.add(hedgedCancel)
	if id == -1 {
		return
	}

	r, e := t.c.TopicAdminClient.Publish(hedgedCtx, &pb.PublishRequest{
		Topic:    t.name,
		Messages: req.pbMsgs,
	}, req.gaxOpts...)

	if e == nil {
		select {
		case req.resCh <- attemptResult{res: r, err: e, id: id}:
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
		if t.PublishSettings.HedgingSettings.TokenRatio > 0 {
			ratio = t.PublishSettings.HedgingSettings.TokenRatio
		}
		if t.PublishSettings.HedgingSettings.MaxTokens > 0 {
			maxTokens = t.PublishSettings.HedgingSettings.MaxTokens
		}
	}

	if t.hedgingTokenBucket < maxTokens {
		t.hedgingTokenBucket += ratio
		if t.hedgingTokenBucket > maxTokens {
			t.hedgingTokenBucket = maxTokens
		}
	}
}

// ErrPublishingPaused is a custom error indicating that the publish paused for the specified ordering key.
type ErrPublishingPaused struct {
	OrderingKey string
}

func (e ErrPublishingPaused) Error() string {
	return fmt.Sprintf("pubsub: Publishing for ordering key, %s, paused due to previous error. Call topic.ResumePublish(orderingKey) before resuming publishing", e.OrderingKey)

}

func (t *Publisher) publishMessageBundle(ctx context.Context, bms []*bundledMessage) {
	ctx, err := tag.New(ctx, tag.Insert(keyStatus, "OK"), tag.Upsert(keyTopic, t.name))
	if err != nil {
		log.Printf("pubsub: cannot create context with tag in publishMessageBundle: %v", err)
	}
	numMsgs := len(bms)
	pbMsgs := make([]*pb.PubsubMessage, numMsgs)
	var orderingKey string
	if numMsgs != 0 {
		// extract the ordering key for this batch. since
		// messages in the same batch share the same ordering
		// key, it doesn't matter which we read from.
		orderingKey = bms[0].msg.OrderingKey
	}

	if t.enableTracing {
		links := make([]trace.Link, 0, numMsgs)
		for _, bm := range bms {
			if bm.createSpan.SpanContext().IsSampled() {
				links = append(links, trace.Link{SpanContext: bm.createSpan.SpanContext()})
			}
		}

		projectID, topicID := parseResourceName(t.name)
		var pSpan trace.Span
		opts := getCommonOptions(projectID, t.name)
		// Add link to publish RPC span of createSpan(s).
		opts = append(opts, trace.WithLinks(links...))
		opts = append(
			opts,
			trace.WithAttributes(
				semconv.MessagingBatchMessageCount(numMsgs),
				semconv.CodeFunction("publishMessageBundle"),
			),
		)
		ctx, pSpan = startSpan(ctx, publishRPCSpanName, topicID, opts...)
		defer pSpan.End()

		// Add the reverse link to createSpan(s) of publish RPC span.
		if pSpan.SpanContext().IsSampled() {
			for _, bm := range bms {
				bm.createSpan.AddLink(trace.Link{
					SpanContext: pSpan.SpanContext(),
					Attributes: []attribute.KeyValue{
						semconv.MessagingOperationName(publishRPCSpanName),
					},
				})
			}
		}
	}
	var batchSize int
	for i, bm := range bms {
		pbMsgs[i] = &pb.PubsubMessage{
			Data:        bm.msg.Data,
			Attributes:  bm.msg.Attributes,
			OrderingKey: bm.msg.OrderingKey,
		}
		batchSize = batchSize + proto.Size(pbMsgs[i])
		bm.msg = nil // release bm.msg for GC
	}

	var res *pb.PublishResponse
	start := time.Now()
	if orderingKey != "" && t.scheduler.IsPaused(orderingKey) {
		err = ErrPublishingPaused{OrderingKey: orderingKey}
	} else {
		// Apply custom publish retryer on top of user specified retryer and
		// default retryer.
		opts := t.c.TopicAdminClient.CallOptions.Publish
		var settings gax.CallSettings
		for _, opt := range opts {
			opt.Resolve(&settings)
		}
		r := &publishRetryer{defaultRetryer: settings.Retry()}
		gaxOpts := []gax.CallOption{
			gax.WithGRPCOptions(grpc.MaxCallSendMsgSize(maxSendRecvBytes)),
			gax.WithRetry(func() gax.Retryer { return r }),
		}
		if t.PublishSettings.shouldCompress(batchSize) {
			gaxOpts = append(gaxOpts, gax.WithGRPCOptions(grpc.UseCompressor(gzip.Name)))
		}

		if t.hedgingDelay > 0 && orderingKey == "" {
			cs := newCancellationSharer()
			defer cs.cancelAll()

			resCh := make(chan attemptResult, 1)

			initialHedge := &hedgedRequest{
				attemptID: 1,
				sendAfter: time.Now().Add(t.hedgingDelay),
				resCh:     resCh,
				cs:        cs,
				ctx:       ctx,
				pbMsgs:    pbMsgs,
				gaxOpts:   gaxOpts,
			}
			t.enqueueHedgedRequest(initialHedge)

			mainCtx, mainCancel := context.WithCancel(ctx)
			mainID := cs.add(mainCancel)
			r, e := t.c.TopicAdminClient.Publish(mainCtx, &pb.PublishRequest{
				Topic:    t.name,
				Messages: pbMsgs,
			}, gaxOpts...)

			if e == nil {
				select {
				case resCh <- attemptResult{res: r, err: e, id: mainID}:
					cs.win(mainID)
				default:
				}
			}

			select {
			case winner := <-resCh:
				res = winner.res
				err = winner.err
				if err == nil {
					t.replenishHedgingTokens()
				}
			default:
				// Main attempt failed or delayed; wait for a winning hedged attempt if one succeeds
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
			}
		} else {
			// regular publish without hedging
			res, err = t.c.TopicAdminClient.Publish(ctx, &pb.PublishRequest{
				Topic:    t.name,
				Messages: pbMsgs,
			}, gaxOpts...)
			if err == nil && t.hedgingDelay > 0 {
				t.replenishHedgingTokens()
			}
		}
	}
	end := time.Now()
	if err != nil {
		t.scheduler.Pause(orderingKey)
		// Update context with error tag for OpenCensus,
		// using same stats.Record() call as success case.
		ctx, _ = tag.New(ctx, tag.Upsert(keyStatus, "ERROR"),
			tag.Upsert(keyError, err.Error()))
	}
	stats.Record(ctx,
		PublishLatency.M(float64(end.Sub(start)/time.Millisecond)),
		PublishedMessages.M(int64(len(bms))))
	for i, bm := range bms {
		t.flowController.release(ctx, bm.size)
		if err != nil {
			ipubsub.SetPublishResult(bm.res, "", err)
			spanRecordError(bm.createSpan, err)
		} else {
			ipubsub.SetPublishResult(bm.res, res.MessageIds[i], nil)
			if t.enableTracing {
				bm.createSpan.SetAttributes(semconv.MessagingMessageIDKey.String(res.MessageIds[i]))
			}
		}
	}
}

// ResumePublish resumes accepting messages for the provided ordering key.
// Publishing using an ordering key might be paused if an error is
// encountered while publishing, to prevent messages from being published
// out of order.
func (t *Publisher) ResumePublish(orderingKey string) {
	t.mu.RLock()
	noop := t.scheduler == nil
	t.mu.RUnlock()
	if noop {
		return
	}

	t.scheduler.Resume(orderingKey)
}

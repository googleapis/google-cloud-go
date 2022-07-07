// Copyright 2016 Google LLC
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
	"io"
	"strings"
	"sync"
	"time"

	ipubsub "cloud.google.com/go/internal/pubsub"
	vkit "cloud.google.com/go/pubsub/apiv1"
	"cloud.google.com/go/pubsub/internal/distribution"
	gax "github.com/googleapis/gax-go/v2"
	pb "google.golang.org/genproto/googleapis/pubsub/v1"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/encoding/protowire"
)

// Between message receipt and ack (that is, the time spent processing a message) we want to extend the message
// deadline by way of modack. However, we don't want to extend the deadline right as soon as the deadline expires;
// instead, we'd want to extend the deadline a little bit of time ahead. gracePeriod is that amount of time ahead
// of the actual deadline.
const gracePeriod = 5 * time.Second

// These are vars so tests can change them.
var (
	maxDurationPerLeaseExtension            = 10 * time.Minute
	minDurationPerLeaseExtension            = 10 * time.Second
	minDurationPerLeaseExtensionExactlyOnce = 1 * time.Minute
)

type messageIterator struct {
	ctx        context.Context
	cancel     func() // the function that will cancel ctx; called in stop
	po         *pullOptions
	ps         *pullStream
	subc       *vkit.SubscriberClient
	subName    string
	kaTick     <-chan time.Time // keep-alive (deadline extensions)
	ackTicker  *time.Ticker     // message acks
	nackTicker *time.Ticker     // message nacks
	pingTicker *time.Ticker     //  sends to the stream to keep it open
	failed     chan struct{}    // closed on stream error
	drained    chan struct{}    // closed when stopped && no more pending messages
	wg         sync.WaitGroup

	mu          sync.Mutex
	ackTimeDist *distribution.D // dist uses seconds

	// keepAliveDeadlines is a map of id to expiration time. This map is used in conjunction with
	// subscription.ReceiveSettings.MaxExtension to record the maximum amount of time (the
	// deadline, more specifically) we're willing to extend a message's ack deadline. As each
	// message arrives, we'll record now+MaxExtension in this table; whenever we have a chance
	// to update ack deadlines (via modack), we'll consult this table and only include IDs
	// that are not beyond their deadline.
	keepAliveDeadlines map[string]time.Time
	pendingAcks        map[string]*AckResult
	pendingNacks       map[string]*AckResult
	// ack IDs whose ack deadline is to be modified
	// This technically does not need to be an AckResult, since it is just a set,
	// but allows reuse of iterator.sendAckIDRPC.
	pendingModAcks map[string]*AckResult
	// This stores pending AckResults for cleaner shutdown when sub.Receive's ctx is cancelled.
	// If exactly once delivery is not enabled, this map should not be populated.
	pendingAckResults map[string]*AckResult
	err               error // error from stream failure

	eoMu                      sync.RWMutex
	enableExactlyOnceDelivery bool
	sendNewAckDeadline        bool
}

// newMessageIterator starts and returns a new messageIterator.
// subName is the full name of the subscription to pull messages from.
// Stop must be called on the messageIterator when it is no longer needed.
// The iterator always uses the background context for acking messages and extending message deadlines.
func newMessageIterator(subc *vkit.SubscriberClient, subName string, po *pullOptions) *messageIterator {
	var ps *pullStream
	if !po.synchronous {
		maxMessages := po.maxOutstandingMessages
		maxBytes := po.maxOutstandingBytes
		if po.useLegacyFlowControl {
			maxMessages = 0
			maxBytes = 0
		}
		ps = newPullStream(context.Background(), subc.StreamingPull, subName, maxMessages, maxBytes, po.maxExtensionPeriod)
	}
	// The period will update each tick based on the distribution of acks. We'll start by arbitrarily sending
	// the first keepAlive halfway towards the minimum ack deadline.
	keepAlivePeriod := minDurationPerLeaseExtension / 2

	// Ack promptly so users don't lose work if client crashes.
	ackTicker := time.NewTicker(100 * time.Millisecond)
	nackTicker := time.NewTicker(100 * time.Millisecond)
	pingTicker := time.NewTicker(30 * time.Second)
	cctx, cancel := context.WithCancel(context.Background())
	cctx = withSubscriptionKey(cctx, subName)
	it := &messageIterator{
		ctx:                cctx,
		cancel:             cancel,
		ps:                 ps,
		po:                 po,
		subc:               subc,
		subName:            subName,
		kaTick:             time.After(keepAlivePeriod),
		ackTicker:          ackTicker,
		nackTicker:         nackTicker,
		pingTicker:         pingTicker,
		failed:             make(chan struct{}),
		drained:            make(chan struct{}),
		ackTimeDist:        distribution.New(int(maxDurationPerLeaseExtension/time.Second) + 1),
		keepAliveDeadlines: map[string]time.Time{},
		pendingAcks:        map[string]*AckResult{},
		pendingNacks:       map[string]*AckResult{},
		pendingModAcks:     map[string]*AckResult{},
	}
	it.wg.Add(1)
	go it.sender()
	return it
}

// Subscription.receive will call stop on its messageIterator when finished with it.
// Stop will block until Done has been called on all Messages that have been
// returned by Next, or until the context with which the messageIterator was created
// is cancelled or exceeds its deadline.
func (it *messageIterator) stop() {
	it.cancel()
	it.mu.Lock()
	it.checkDrained()
	it.mu.Unlock()
	it.wg.Wait()
}

// checkDrained closes the drained channel if the iterator has been stopped and all
// pending messages have either been n/acked or expired.
//
// Called with the lock held.
func (it *messageIterator) checkDrained() {
	select {
	case <-it.drained:
		return
	default:
	}
	select {
	case <-it.ctx.Done():
		if len(it.keepAliveDeadlines) == 0 {
			close(it.drained)
		}
	default:
	}
}

// Given a receiveTime, add the elapsed time to the iterator's ack distribution.
// These values are bounded by the ModifyAckDeadline limits, which are
// min/maxDurationPerLeaseExtension.
func (it *messageIterator) addToDistribution(receiveTime time.Time) {
	d := time.Since(receiveTime)
	d = maxDuration(d, minDurationPerLeaseExtension)
	d = minDuration(d, maxDurationPerLeaseExtension)
	it.ackTimeDist.Record(int(d / time.Second))
}

// Called when a message is acked/nacked.
func (it *messageIterator) done(ackID string, ack bool, r *AckResult, receiveTime time.Time) {
	it.addToDistribution(receiveTime)
	it.mu.Lock()
	defer it.mu.Unlock()
	delete(it.keepAliveDeadlines, ackID)
	delete(it.pendingAckResults, ackID)
	if ack {
		it.pendingAcks[ackID] = r
	} else {
		it.pendingNacks[ackID] = r
	}
	it.checkDrained()
}

// fail is called when a stream method returns a permanent error.
// fail returns it.err. This may be err, or it may be the error
// set by an earlier call to fail.
func (it *messageIterator) fail(err error) error {
	it.mu.Lock()
	defer it.mu.Unlock()
	if it.err == nil {
		it.err = err
		close(it.failed)
	}
	return it.err
}

// receive makes a call to the stream's Recv method, or the Pull RPC, and returns
// its messages.
// maxToPull is the maximum number of messages for the Pull RPC.
func (it *messageIterator) receive(maxToPull int32) ([]*Message, error) {
	it.mu.Lock()
	ierr := it.err
	it.mu.Unlock()
	if ierr != nil {
		return nil, ierr
	}

	// Stop retrieving messages if the iterator's Stop method was called.
	select {
	case <-it.ctx.Done():
		it.wg.Wait()
		return nil, io.EOF
	default:
	}

	var rmsgs []*pb.ReceivedMessage
	var err error
	if it.po.synchronous {
		rmsgs, err = it.pullMessages(maxToPull)
	} else {
		rmsgs, err = it.recvMessages()
	}
	// Any error here is fatal.
	if err != nil {
		return nil, it.fail(err)
	}
	recordStat(it.ctx, PullCount, int64(len(rmsgs)))
	now := time.Now()
	msgs, err := convertMessages(rmsgs, now, it.done)
	if err != nil {
		return nil, it.fail(err)
	}
	// We received some messages. Remember them so we can keep them alive. Also,
	// do a receipt mod-ack when streaming.
	maxExt := time.Now().Add(it.po.maxExtension)
	ackIDs := map[string]*AckResult{}
	it.mu.Lock()
	for _, m := range msgs {
		ackID := msgAckID(m)
		addRecv(m.ID, ackID, now)
		it.keepAliveDeadlines[ackID] = maxExt
		// Don't change the mod-ack if the message is going to be nacked. This is
		// possible if there are retries.
		if _, ok := it.pendingNacks[ackID]; !ok {
			// Don't use the message's AckResult here.
			// These ids are used for modacks which require an empty AckResult.
			// Calling m.AckWithResult() (or NackWithResult) prematurely locks
			// the message's ack/nack status.
			ackIDs[ackID] = &ipubsub.AckResult{}
		}
		// If exactly once is enabled, keep track of all pending AckResults
		// so we can cleanly close them all at shutdown.
		if it.enableExactlyOnceDelivery {
			ackh, ok := ipubsub.MessageAckHandler(m).(*psAckHandler)
			if !ok {
				it.fail(errors.New("failed to assert type as psAckHandler"))
			}
			it.pendingAckResults[ackID] = ackh.ackResult
		}
	}
	deadline := it.ackDeadline()
	it.mu.Unlock()
	if len(ackIDs) > 0 {
		if !it.sendModAck(ackIDs, deadline) {
			return nil, it.err
		}
	}
	return msgs, nil
}

// Get messages using the Pull RPC.
// This may block indefinitely. It may also return zero messages, after some time waiting.
func (it *messageIterator) pullMessages(maxToPull int32) ([]*pb.ReceivedMessage, error) {
	// Use it.ctx as the RPC context, so that if the iterator is stopped, the call
	// will return immediately.
	res, err := it.subc.Pull(it.ctx, &pb.PullRequest{
		Subscription: it.subName,
		MaxMessages:  maxToPull,
	}, gax.WithGRPCOptions(grpc.MaxCallRecvMsgSize(maxSendRecvBytes)))
	switch {
	case err == context.Canceled:
		return nil, nil
	case status.Code(err) == codes.Canceled:
		return nil, nil
	case err != nil:
		return nil, err
	default:
		return res.ReceivedMessages, nil
	}
}

func (it *messageIterator) recvMessages() ([]*pb.ReceivedMessage, error) {
	res, err := it.ps.Recv()
	if err != nil {
		return nil, err
	}
	it.eoMu.Lock()
	if got := res.GetSubscriptionProperties().GetExactlyOnceDeliveryEnabled(); got != it.enableExactlyOnceDelivery {
		it.sendNewAckDeadline = true
		it.enableExactlyOnceDelivery = got
	}
	it.eoMu.Unlock()
	return res.ReceivedMessages, nil
}

// sender runs in a goroutine and handles all sends to the stream.
func (it *messageIterator) sender() {
	defer it.wg.Done()
	defer it.ackTicker.Stop()
	defer it.nackTicker.Stop()
	defer it.pingTicker.Stop()
	defer func() {
		if it.ps != nil {
			it.ps.CloseSend()
		}
	}()

	done := false
	for !done {
		sendAcks := false
		sendNacks := false
		sendModAcks := false
		sendPing := false

		dl := it.ackDeadline()

		select {
		case <-it.failed:
			// Stream failed: nothing to do, so stop immediately.
			return

		case <-it.drained:
			// All outstanding messages have been marked done:
			// nothing left to do except make the final calls.
			it.mu.Lock()
			sendAcks = (len(it.pendingAcks) > 0)
			sendNacks = (len(it.pendingNacks) > 0)
			// No point in sending modacks.
			done = true

		case <-it.kaTick:
			it.mu.Lock()
			it.handleKeepAlives()
			sendModAcks = (len(it.pendingModAcks) > 0)

			nextTick := dl - gracePeriod
			if nextTick <= 0 {
				// If the deadline is <= gracePeriod, let's tick again halfway to
				// the deadline.
				nextTick = dl / 2
			}
			it.kaTick = time.After(nextTick)

		case <-it.nackTicker.C:
			it.mu.Lock()
			sendNacks = (len(it.pendingNacks) > 0)

		case <-it.ackTicker.C:
			it.mu.Lock()
			sendAcks = (len(it.pendingAcks) > 0)

		case <-it.pingTicker.C:
			it.mu.Lock()
			// Ping only if we are processing messages via streaming.
			sendPing = !it.po.synchronous
		}
		// Lock is held here.
		var acks, nacks, modAcks map[string]*AckResult
		if sendAcks {
			acks = it.pendingAcks
			it.pendingAcks = map[string]*AckResult{}
		}
		if sendNacks {
			nacks = it.pendingNacks
			it.pendingNacks = map[string]*AckResult{}
		}
		if sendModAcks {
			modAcks = it.pendingModAcks
			it.pendingModAcks = map[string]*AckResult{}
		}
		it.mu.Unlock()
		// Make Ack and ModAck RPCs.
		if sendAcks {
			if !it.sendAck(acks) {
				return
			}
		}
		if sendNacks {
			// Nack indicated by modifying the deadline to zero.
			if !it.sendModAck(nacks, 0) {
				return
			}
		}
		if sendModAcks {
			if !it.sendModAck(modAcks, dl) {
				return
			}
		}
		if sendPing {
			it.pingStream()
		}
	}
}

// handleKeepAlives modifies the pending request to include deadline extensions
// for live messages. It also purges expired messages.
//
// Called with the lock held.
func (it *messageIterator) handleKeepAlives() {
	now := time.Now()
	for id, expiry := range it.keepAliveDeadlines {
		if expiry.Before(now) {
			// This delete will not result in skipping any map items, as implied by
			// the spec at https://golang.org/ref/spec#For_statements, "For
			// statements with range clause", note 3, and stated explicitly at
			// https://groups.google.com/forum/#!msg/golang-nuts/UciASUb03Js/pzSq5iVFAQAJ.
			delete(it.keepAliveDeadlines, id)
		} else {
			// This will not conflict with a nack, because nacking removes the ID from keepAliveDeadlines.
			// Use an empty AckResult here since we don't propagate ModAcks back to the user.
			it.pendingModAcks[id] = &ipubsub.AckResult{}
		}
	}
	it.checkDrained()
}

func (it *messageIterator) sendAck(m map[string]*AckResult) bool {
	// Account for the Subscription field.
	overhead := calcFieldSizeString(it.subName)
	return it.sendAckIDRPC(m, maxPayload-overhead, func(ids []string) error {
		recordStat(it.ctx, AckCount, int64(len(ids)))
		addAcks(ids)
		bo := gax.Backoff{
			Initial:    100 * time.Millisecond,
			Max:        time.Second,
			Multiplier: 2,
		}
		cctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
		defer cancel()
		for {
			// Use context.Background() as the call's context, not it.ctx. We don't
			// want to cancel this RPC when the iterator is stopped.
			cctx2, cancel2 := context.WithTimeout(context.Background(), 60*time.Second)
			defer cancel2()
			err := it.subc.Acknowledge(cctx2, &pb.AcknowledgeRequest{
				Subscription: it.subName,
				AckIds:       ids,
			})
			// Retry DeadlineExceeded errors a few times before giving up and
			// allowing the message to expire and be redelivered.
			// The underlying library handles other retries, currently only
			// codes.Unavailable.
			switch status.Code(err) {
			case codes.DeadlineExceeded:
				// Use the outer context with timeout here. Errors from gax, including
				// context deadline exceeded should be transparent, as unacked messages
				// will be redelivered.
				if err := gax.Sleep(cctx, bo.Pause()); err != nil {
					return nil
				}
			default:
				// TODO(b/226593754): by default, errors should not be fatal unless exactly once is enabled
				// since acks are "fire and forget". Once EOS feature is out, retry these errors
				// if exactly-once is enabled, which can be determined from StreamingPull response.
				return nil
			}
		}
	})
}

// The receipt mod-ack amount is derived from a percentile distribution based
// on the time it takes to process messages. The percentile chosen is the 99%th
// percentile in order to capture the highest amount of time necessary without
// considering 1% outliers.
func (it *messageIterator) sendModAck(m map[string]*AckResult, deadline time.Duration) bool {
	deadlineSec := int32(deadline / time.Second)
	// Account for the Subscription and AckDeadlineSeconds fields.
	overhead := calcFieldSizeString(it.subName) + calcFieldSizeInt(int(deadlineSec))
	return it.sendAckIDRPC(m, maxPayload-overhead, func(ids []string) error {
		if deadline == 0 {
			recordStat(it.ctx, NackCount, int64(len(ids)))
		} else {
			recordStat(it.ctx, ModAckCount, int64(len(ids)))
		}
		addModAcks(ids, deadlineSec)
		// Retry this RPC on Unavailable for a short amount of time, then give up
		// without returning a fatal error. The utility of this RPC is by nature
		// transient (since the deadline is relative to the current time) and it
		// isn't crucial for correctness (since expired messages will just be
		// resent).
		cctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		bo := gax.Backoff{
			Initial:    100 * time.Millisecond,
			Max:        time.Second,
			Multiplier: 2,
		}
		for {
			err := it.subc.ModifyAckDeadline(cctx, &pb.ModifyAckDeadlineRequest{
				Subscription:       it.subName,
				AckDeadlineSeconds: deadlineSec,
				AckIds:             ids,
			})
			switch status.Code(err) {
			case codes.Unavailable:
				if err := gax.Sleep(cctx, bo.Pause()); err == nil {
					continue
				}
				// Treat sleep timeout like RPC timeout.
				fallthrough
			case codes.DeadlineExceeded:
				// Timeout. Not a fatal error, but note that it happened.
				recordStat(it.ctx, ModAckTimeoutCount, 1)
				return nil
			default:
				// This addresses an error where `context deadline exceeded` errors
				// not captured by the previous case causes fatal errors.
				// See https://github.com/googleapis/google-cloud-go/issues/3060
				if err != nil && strings.Contains(err.Error(), "context deadline exceeded") {
					recordStat(it.ctx, ModAckTimeoutCount, 1)
					return nil
				}
				// TODO(b/226593754): by default, errors should not be fatal unless exactly once is enabled
				// since modacks are "fire and forget". Once EOS feature is out, retry these errors
				// if exactly-once is enabled, which can be determined from StreamingPull response.
				return nil
			}
		}
	})
}

func (it *messageIterator) sendAckIDRPC(ackIDSet map[string]*AckResult, maxSize int, call func([]string) error) bool {
	ackIDs := make([]string, 0, len(ackIDSet))
	for k := range ackIDSet {
		ackIDs = append(ackIDs, k)
	}
	var toSend []string
	for len(ackIDs) > 0 {
		toSend, ackIDs = splitRequestIDs(ackIDs, maxSize)
		if err := call(toSend); err != nil {
			// The underlying client handles retries, so any error is fatal to the
			// iterator.
			it.fail(err)
			return false
		}
	}
	return true
}

// Send a message to the stream to keep it open. The stream will close if there's no
// traffic on it for a while. By keeping it open, we delay the start of the
// expiration timer on messages that are buffered by gRPC or elsewhere in the
// network. This matters if it takes a long time to process messages relative to the
// default ack deadline, and if the messages are small enough so that many can fit
// into the buffer.
func (it *messageIterator) pingStream() {
	spr := &pb.StreamingPullRequest{}
	it.eoMu.RLock()
	if it.sendNewAckDeadline {
		spr.StreamAckDeadlineSeconds = int32(it.ackDeadline())
		it.sendNewAckDeadline = false
	}
	it.eoMu.RUnlock()
	it.ps.Send(spr)
}

// calcFieldSizeString returns the number of bytes string fields
// will take up in an encoded proto message.
func calcFieldSizeString(fields ...string) int {
	overhead := 0
	for _, field := range fields {
		overhead += 1 + len(field) + protowire.SizeVarint(uint64(len(field)))
	}
	return overhead
}

// calcFieldSizeInt returns the number of bytes int fields
// will take up in an encoded proto message.
func calcFieldSizeInt(fields ...int) int {
	overhead := 0
	for _, field := range fields {
		overhead += 1 + protowire.SizeVarint(uint64(field))
	}
	return overhead
}

// splitRequestIDs takes a slice of ackIDs and returns two slices such that the first
// ackID slice can be used in a request where the payload does not exceed maxSize.
func splitRequestIDs(ids []string, maxSize int) (prefix, remainder []string) {
	size := 0
	i := 0
	// TODO(hongalex): Use binary search to find split index, since ackIDs are
	// fairly constant.
	for size < maxSize && i < len(ids) {
		size += calcFieldSizeString(ids[i])
		i++
	}
	if size > maxSize {
		i--
	}
	return ids[:i], ids[i:]
}

// The deadline to ack is derived from a percentile distribution based
// on the time it takes to process messages. The percentile chosen is the 99%th
// percentile - that is, processing times up to the 99%th longest processing
// times should be safe. The highest 1% may expire. This number was chosen
// as a way to cover most users' usecases without losing the value of
// expiration.
func (it *messageIterator) ackDeadline() time.Duration {
	pt := time.Duration(it.ackTimeDist.Percentile(.99)) * time.Second
	it.eoMu.RLock()
	enableExactlyOnce := it.enableExactlyOnceDelivery
	it.eoMu.RUnlock()
	return boundedDuration(pt, it.po.minExtensionPeriod, it.po.maxExtensionPeriod, enableExactlyOnce)
}

func boundedDuration(ackDeadline, minExtension, maxExtension time.Duration, exactlyOnce bool) time.Duration {
	// If the user explicitly sets a maxExtensionPeriod, respect it.
	if maxExtension > 0 {
		ackDeadline = minDuration(ackDeadline, maxExtension)
	}

	// If the user explicitly sets a minExtensionPeriod, respect it.
	if minExtension > 0 {
		ackDeadline = maxDuration(ackDeadline, minExtension)
	} else if exactlyOnce {
		// Higher minimum ack_deadline for subscriptions with
		// exactly-once delivery enabled.
		ackDeadline = maxDuration(ackDeadline, minDurationPerLeaseExtensionExactlyOnce)
	} else if ackDeadline < minDurationPerLeaseExtension {
		// Otherwise, lower bound is min ack extension. This is normally bounded
		// when adding datapoints to the distribution, but this is needed for
		// the initial few calls to ackDeadline.
		ackDeadline = minDurationPerLeaseExtension
	}

	return ackDeadline
}

func minDuration(x, y time.Duration) time.Duration {
	if x < y {
		return x
	}
	return y
}

func maxDuration(x, y time.Duration) time.Duration {
	if x > y {
		return x
	}
	return y
}

const (
	transientErrStringPrefix     = "TRANSIENT_"
	transientInvalidAckErrString = transientErrStringPrefix + "FAILURE_INVALID_ACK_ID"
	permanentInvalidAckErrString = "PERMANENT_FAILURE_INVALID_ACK_ID"
)

// processResults processes AckResults by referring to errorStatus and errorsMap.
// The errors returned by the server in `errorStatus` or in `errorsByAckID`
// are used to complete the AckResults in `ackResMap` (with a success
// or error) or to return requests for further retries.
// Logic is derived from python-pubsub: https://github.com/googleapis/python-pubsub/blob/main/google/cloud/pubsub_v1/subscriber/_protocol/streaming_pull_manager.py#L161-L220
func processResults(errorStatus *status.Status, ackResMap map[string]*AckResult, errorsByAckID map[string]error) ([]*AckResult, []*AckResult) {
	var completedResults, retryResults []*AckResult
	for ackID, res := range ackResMap {
		// Handle special errors returned for ack/modack RPCs via the ErrorInfo
		// sidecar metadata when exactly-once delivery is enabled.
		if errAckID, ok := errorsByAckID[ackID]; ok {
			errAckIDStr := errAckID.Error()
			if strings.HasPrefix(errAckIDStr, transientErrStringPrefix) {
				retryResults = append(retryResults, res)
			} else {
				if errAckIDStr == permanentInvalidAckErrString {
					ipubsub.SetAckResult(res, AcknowledgeStatusInvalidAckID, errAckID)
				} else {
					ipubsub.SetAckResult(res, AcknowledgeStatusOther, errAckID)
				}
				completedResults = append(completedResults, res)
			}
		} else if errorStatus != nil && contains(errorStatus.Code(), exactlyOnceDeliveryTemporaryRetryErrors) {
			retryResults = append(retryResults, ackResMap[ackID])
		} else if errorStatus != nil {
			// Other gRPC errors are not retried.
			switch errorStatus.Code() {
			case codes.PermissionDenied:
				ipubsub.SetAckResult(res, AcknowledgeStatusPermissionDenied, errorStatus.Err())
			case codes.FailedPrecondition:
				ipubsub.SetAckResult(res, AcknowledgeStatusFailedPrecondition, errorStatus.Err())
			default:
				ipubsub.SetAckResult(res, AcknowledgeStatusOther, errorStatus.Err())
			}
			completedResults = append(completedResults, res)
		} else if res != nil {
			// Since no error occurred, requests with AckResults are completed successfully.
			ipubsub.SetAckResult(res, AcknowledgeStatusSuccess, nil)
			completedResults = append(completedResults, res)
		} else {
			// All other requests are considered completed.
			completedResults = append(completedResults, res)
		}
	}
	return completedResults, retryResults
}

// Copyright 2020 Google LLC
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

package wire

import (
	"bytes"
	"context"
	"errors"
	"sort"
	"sync"
	"testing"
	"time"

	"cloud.google.com/go/internal/testutil"
	"cloud.google.com/go/pubsublite/internal/test"
	"github.com/google/go-cmp/cmp/cmpopts"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/proto"

	pb "cloud.google.com/go/pubsublite/apiv1/pubsublitepb"
)

const (
	maxMessages int = 10
	maxBytes    int = 1000
)

func testSubscriberSettings() ReceiveSettings {
	settings := testReceiveSettings()
	settings.MaxOutstandingMessages = maxMessages
	settings.MaxOutstandingBytes = maxBytes
	return settings
}

// initFlowControlReq returns the first expected flow control request when
// testSubscriberSettings are used.
func initFlowControlReq() *pb.SubscribeRequest {
	return flowControlSubReq(flowControlTokens{Bytes: int64(maxBytes), Messages: int64(maxMessages)})
}

func partitionMsgs(partition int, msgs ...*pb.SequencedMessage) []*ReceivedMessage {
	var received []*ReceivedMessage
	for _, msg := range msgs {
		received = append(received, &ReceivedMessage{Msg: msg, Partition: partition})
	}
	return received
}

func join(args ...[]*ReceivedMessage) []*ReceivedMessage {
	var received []*ReceivedMessage
	for _, msgs := range args {
		received = append(received, msgs...)
	}
	return received
}

type testMessageReceiver struct {
	t        *testing.T
	received chan *ReceivedMessage
}

func newTestMessageReceiver(t *testing.T) *testMessageReceiver {
	return &testMessageReceiver{
		t:        t,
		received: make(chan *ReceivedMessage, 5),
	}
}

func (tr *testMessageReceiver) onMessage(msg *ReceivedMessage) {
	tr.received <- msg
}

func (tr *testMessageReceiver) ValidateMsg(want *pb.SequencedMessage) AckConsumer {
	select {
	case <-time.After(serviceTestWaitTimeout):
		tr.t.Errorf("Message (%v) not received within %v", want, serviceTestWaitTimeout)
		return nil
	case got := <-tr.received:
		if !proto.Equal(got.Msg, want) {
			tr.t.Errorf("Received message: got (%v), want (%v)", got.Msg, want)
		}
		return got.Ack
	}
}

type ByMsgOffset []*ReceivedMessage

func (m ByMsgOffset) Len() int      { return len(m) }
func (m ByMsgOffset) Swap(i, j int) { m[i], m[j] = m[j], m[i] }
func (m ByMsgOffset) Less(i, j int) bool {
	return m[i].Msg.GetCursor().GetOffset() < m[j].Msg.GetCursor().GetOffset()
}

func (tr *testMessageReceiver) ValidateMsgs(want []*ReceivedMessage) {
	var got []*ReceivedMessage
	for count := 0; count < len(want); count++ {
		select {
		case <-time.After(serviceTestWaitTimeout):
			tr.t.Errorf("Received messages count: got %d, want %d", count, len(want))
		case received := <-tr.received:
			received.Ack.Ack()
			got = append(got, received)
		}
	}

	sort.Sort(ByMsgOffset(want))
	sort.Sort(ByMsgOffset(got))
	if !testutil.Equal(got, want, cmpopts.IgnoreFields(ReceivedMessage{}, "Ack")) {
		tr.t.Errorf("Received messages: got: %v\nwant: %v", got, want)
	}
}

func (tr *testMessageReceiver) VerifyNoMsgs() {
	select {
	case got := <-tr.received:
		tr.t.Errorf("Got unexpected message: %v", got.Msg)
	case <-time.After(20 * time.Millisecond):
		// Wait to ensure no messages received.
	}
}

// testBlockingMessageReceiver can be used to simulate a client message receiver
// func that is blocking due to slow message processing.
type testBlockingMessageReceiver struct {
	blockReceive chan struct{}

	testMessageReceiver
}

func newTestBlockingMessageReceiver(t *testing.T) *testBlockingMessageReceiver {
	return &testBlockingMessageReceiver{
		testMessageReceiver: testMessageReceiver{
			t:        t,
			received: make(chan *ReceivedMessage, 5),
		},
		blockReceive: make(chan struct{}),
	}
}

// onMessage is the message receiver func and blocks until there is a call to
// Return().
func (tr *testBlockingMessageReceiver) onMessage(msg *ReceivedMessage) {
	tr.testMessageReceiver.onMessage(msg)
	<-tr.blockReceive
}

// Return signals onMessage to return.
func (tr *testBlockingMessageReceiver) Return() {
	var void struct{}
	tr.blockReceive <- void
}

func TestMessageDeliveryQueueStartStop(t *testing.T) {
	acks := newAckTracker()
	receiver := newTestMessageReceiver(t)
	messageQueue := newMessageDeliveryQueue(acks, receiver.onMessage, 10)

	t.Run("Add before start", func(t *testing.T) {
		msg1 := seqMsgWithOffset(1)
		ack1 := newAckConsumer(1, 0, nil)
		messageQueue.Add(&ReceivedMessage{Msg: msg1, Ack: ack1})

		receiver.VerifyNoMsgs()
	})

	t.Run("Add after start", func(t *testing.T) {
		msg2 := seqMsgWithOffset(2)
		ack2 := newAckConsumer(2, 0, nil)
		msg3 := seqMsgWithOffset(3)
		ack3 := newAckConsumer(3, 0, nil)

		messageQueue.Start()
		messageQueue.Start() // Check duplicate starts
		messageQueue.Add(&ReceivedMessage{Msg: msg2, Ack: ack2})
		messageQueue.Add(&ReceivedMessage{Msg: msg3, Ack: ack3})

		receiver.ValidateMsg(msg2)
		receiver.ValidateMsg(msg3)
	})

	t.Run("Add after stop", func(t *testing.T) {
		msg4 := seqMsgWithOffset(4)
		ack4 := newAckConsumer(4, 0, nil)

		messageQueue.Stop()
		messageQueue.Stop() // Check duplicate stop
		messageQueue.Add(&ReceivedMessage{Msg: msg4, Ack: ack4})
		messageQueue.Wait()

		receiver.VerifyNoMsgs()
	})

	t.Run("Restart", func(t *testing.T) {
		msg5 := seqMsgWithOffset(5)
		ack5 := newAckConsumer(5, 0, nil)

		messageQueue.Start()
		messageQueue.Add(&ReceivedMessage{Msg: msg5, Ack: ack5})

		receiver.ValidateMsg(msg5)
	})

	t.Run("Stop", func(t *testing.T) {
		messageQueue.Stop()
		messageQueue.Wait()

		receiver.VerifyNoMsgs()
	})
}

func TestMessageDeliveryQueueDiscardMessages(t *testing.T) {
	acks := newAckTracker()
	blockingReceiver := newTestBlockingMessageReceiver(t)
	messageQueue := newMessageDeliveryQueue(acks, blockingReceiver.onMessage, 10)

	msg1 := seqMsgWithOffset(1)
	ack1 := newAckConsumer(1, 0, nil)
	msg2 := seqMsgWithOffset(2)
	ack2 := newAckConsumer(2, 0, nil)

	messageQueue.Start()
	messageQueue.Add(&ReceivedMessage{Msg: msg1, Ack: ack1})
	messageQueue.Add(&ReceivedMessage{Msg: msg2, Ack: ack2})

	// The blocking receiver suspends after receiving msg1.
	blockingReceiver.ValidateMsg(msg1)
	// Stopping the message queue should discard undelivered msg2.
	messageQueue.Stop()

	// Unsuspend the blocking receiver and verify msg2 is not received.
	blockingReceiver.Return()
	messageQueue.Wait()
	blockingReceiver.VerifyNoMsgs()
	if got, want := acks.outstandingAcks.Len(), 1; got != want {
		t.Errorf("ackTracker.outstandingAcks.Len() got %v, want %v", got, want)
	}
}

// testSubscribeStream wraps a subscribeStream for ease of testing.
type testSubscribeStream struct {
	Receiver *testMessageReceiver
	t        *testing.T
	acks     *ackTracker
	sub      *subscribeStream
	mu       sync.Mutex
	resetErr error
	serviceTestProxy
}

func newTestSubscribeStream(t *testing.T, subscription subscriptionPartition, settings ReceiveSettings) *testSubscribeStream {
	ctx := context.Background()
	subClient, err := newSubscriberClient(ctx, "ignored", testServer.ClientConn())
	if err != nil {
		t.Fatal(err)
	}

	ts := &testSubscribeStream{
		Receiver: newTestMessageReceiver(t),
		t:        t,
		acks:     newAckTracker(),
	}
	ts.sub = newSubscribeStream(ctx, subClient, settings, ts.Receiver.onMessage, subscription, ts.acks, ts.handleReset, true)
	ts.initAndStart(t, ts.sub, "Subscriber", subClient)
	return ts
}

// SendBatchFlowControl invokes the periodic background batch flow control. Note
// that the periodic task is disabled in tests.
func (ts *testSubscribeStream) SendBatchFlowControl() {
	ts.sub.sendBatchFlowControl()
}

func (ts *testSubscribeStream) PendingFlowControlRequest() *pb.FlowControlRequest {
	ts.sub.mu.Lock()
	defer ts.sub.mu.Unlock()
	return ts.sub.flowControl.pendingTokens.ToFlowControlRequest()
}

func (ts *testSubscribeStream) SetResetErr(err error) {
	ts.mu.Lock()
	defer ts.mu.Unlock()
	ts.resetErr = err
}

func (ts *testSubscribeStream) handleReset() error {
	ts.mu.Lock()
	defer ts.mu.Unlock()
	return ts.resetErr
}

func TestSubscribeStreamReconnect(t *testing.T) {
	subscription := subscriptionPartition{"projects/123456/locations/us-central1-b/subscriptions/my-sub", 0}
	msg1 := seqMsgWithOffsetAndSize(67, 200)
	msg2 := seqMsgWithOffsetAndSize(68, 100)
	permanentErr := status.Error(codes.FailedPrecondition, "permanent failure")

	verifiers := test.NewVerifiers(t)

	stream1 := test.NewRPCVerifier(t)
	stream1.Push(initSubReqCommit(subscription), initSubResp(), nil)
	stream1.Push(initFlowControlReq(), msgSubResp(msg1), nil)
	stream1.Push(nil, nil, status.Error(codes.Unavailable, "server unavailable"))
	verifiers.AddSubscribeStream(subscription.Path, subscription.Partition, stream1)

	// When reconnected, the subscribeStream should set initial cursor to msg2 and
	// have subtracted flow control tokens.
	stream2 := test.NewRPCVerifier(t)
	stream2.Push(initSubReqCursor(subscription, 68), initSubResp(), nil)
	stream2.Push(flowControlSubReq(flowControlTokens{Bytes: 800, Messages: 9}), msgSubResp(msg2), nil)
	// Subscriber should terminate on permanent error.
	stream2.Push(nil, nil, permanentErr)
	verifiers.AddSubscribeStream(subscription.Path, subscription.Partition, stream2)

	mockServer.OnTestStart(verifiers)
	defer mockServer.OnTestEnd()

	sub := newTestSubscribeStream(t, subscription, testSubscriberSettings())
	if gotErr := sub.StartError(); gotErr != nil {
		t.Errorf("Start() got err: (%v)", gotErr)
	}
	sub.Receiver.ValidateMsg(msg1)
	sub.Receiver.ValidateMsg(msg2)
	if gotErr := sub.FinalError(); !test.ErrorEqual(gotErr, permanentErr) {
		t.Errorf("Final err: (%v), want: (%v)", gotErr, permanentErr)
	}
}

func TestSubscribeStreamFlowControlBatching(t *testing.T) {
	subscription := subscriptionPartition{"projects/123456/locations/us-central1-b/subscriptions/my-sub", 0}
	msg1 := seqMsgWithOffsetAndSize(67, 200)
	msg2 := seqMsgWithOffsetAndSize(68, 100)
	serverErr := status.Error(codes.InvalidArgument, "verifies flow control received")

	verifiers := test.NewVerifiers(t)
	stream := test.NewRPCVerifier(t)
	stream.Push(initSubReqCommit(subscription), initSubResp(), nil)
	stream.Push(initFlowControlReq(), msgSubResp(msg1, msg2), nil)
	// Batch flow control request expected.
	stream.Push(flowControlSubReq(flowControlTokens{Bytes: 300, Messages: 2}), nil, serverErr)
	verifiers.AddSubscribeStream(subscription.Path, subscription.Partition, stream)

	mockServer.OnTestStart(verifiers)
	defer mockServer.OnTestEnd()

	sub := newTestSubscribeStream(t, subscription, testSubscriberSettings())
	if gotErr := sub.StartError(); gotErr != nil {
		t.Errorf("Start() got err: (%v)", gotErr)
	}
	sub.Receiver.ValidateMsg(msg1)
	sub.Receiver.ValidateMsg(msg2)
	sub.sub.onAck(&ackConsumer{MsgBytes: msg1.SizeBytes})
	sub.sub.onAck(&ackConsumer{MsgBytes: msg2.SizeBytes})
	sub.sub.sendBatchFlowControl()
	if gotErr := sub.FinalError(); !test.ErrorEqual(gotErr, serverErr) {
		t.Errorf("Final err: (%v), want: (%v)", gotErr, serverErr)
	}
}

func TestSubscribeStreamExpediteFlowControl(t *testing.T) {
	subscription := subscriptionPartition{"projects/123456/locations/us-central1-b/subscriptions/my-sub", 0}
	msg1 := seqMsgWithOffsetAndSize(67, 250)
	// MaxOutstandingBytes = 1000, so msg2 pushes the pending flow control bytes
	// over the expediteBatchRequestRatio=50% threshold in flowControlBatcher.
	msg2 := seqMsgWithOffsetAndSize(68, 251)
	serverErr := status.Error(codes.InvalidArgument, "verifies flow control received")

	verifiers := test.NewVerifiers(t)
	stream := test.NewRPCVerifier(t)
	stream.Push(initSubReqCommit(subscription), initSubResp(), nil)
	stream.Push(initFlowControlReq(), msgSubResp(msg1, msg2), nil)
	// Batch flow control request expected.
	stream.Push(flowControlSubReq(flowControlTokens{Bytes: 501, Messages: 2}), nil, serverErr)
	verifiers.AddSubscribeStream(subscription.Path, subscription.Partition, stream)

	mockServer.OnTestStart(verifiers)
	defer mockServer.OnTestEnd()

	sub := newTestSubscribeStream(t, subscription, testSubscriberSettings())
	if gotErr := sub.StartError(); gotErr != nil {
		t.Errorf("Start() got err: (%v)", gotErr)
	}
	sub.Receiver.ValidateMsg(msg1)
	sub.Receiver.ValidateMsg(msg2)
	sub.sub.onAck(&ackConsumer{MsgBytes: msg1.SizeBytes})
	sub.sub.onAck(&ackConsumer{MsgBytes: msg2.SizeBytes})
	// Note: the ack for msg2 automatically triggers sending the flow control.
	if gotErr := sub.FinalError(); !test.ErrorEqual(gotErr, serverErr) {
		t.Errorf("Final err: (%v), want: (%v)", gotErr, serverErr)
	}
}

func TestSubscribeStreamDisableBatchFlowControl(t *testing.T) {
	subscription := subscriptionPartition{"projects/123456/locations/us-central1-b/subscriptions/my-sub", 0}
	// MaxOutstandingBytes = 1000, so this pushes the pending flow control bytes
	// over the expediteBatchRequestRatio=50% threshold in flowControlBatcher.
	msg := seqMsgWithOffsetAndSize(67, 800)
	retryableErr := status.Error(codes.Unavailable, "unavailable")
	serverErr := status.Error(codes.InvalidArgument, "verifies flow control received")

	verifiers := test.NewVerifiers(t)

	stream1 := test.NewRPCVerifier(t)
	stream1.Push(initSubReqCommit(subscription), initSubResp(), nil)
	stream1.Push(initFlowControlReq(), msgSubResp(msg), nil)
	// Break the stream immediately after sending the message.
	stream1.Push(nil, nil, retryableErr)
	verifiers.AddSubscribeStream(subscription.Path, subscription.Partition, stream1)

	stream2 := test.NewRPCVerifier(t)
	// The barrier is used to pause in the middle of stream reconnection.
	barrier := stream2.PushWithBarrier(initSubReqCursor(subscription, 68), initSubResp(), nil)
	// Full flow control tokens should be sent after stream has connected.
	stream2.Push(initFlowControlReq(), nil, serverErr)
	verifiers.AddSubscribeStream(subscription.Path, subscription.Partition, stream2)

	mockServer.OnTestStart(verifiers)
	defer mockServer.OnTestEnd()

	sub := newTestSubscribeStream(t, subscription, testSubscriberSettings())
	if gotErr := sub.StartError(); gotErr != nil {
		t.Errorf("Start() got err: (%v)", gotErr)
	}

	sub.Receiver.ValidateMsg(msg)
	barrier.ReleaseAfter(func() {
		// While the stream is not connected, the pending flow control request
		// should not be released and sent to the stream.
		sub.sub.onAck(&ackConsumer{MsgBytes: msg.SizeBytes})
		if sub.PendingFlowControlRequest() == nil {
			t.Errorf("Pending flow control request should not be cleared")
		}
	})

	if gotErr := sub.FinalError(); !test.ErrorEqual(gotErr, serverErr) {
		t.Errorf("Final err: (%v), want: (%v)", gotErr, serverErr)
	}
}

func TestSubscribeStreamInvalidInitialResponse(t *testing.T) {
	subscription := subscriptionPartition{"projects/123456/locations/us-central1-b/subscriptions/my-sub", 0}

	verifiers := test.NewVerifiers(t)
	stream := test.NewRPCVerifier(t)
	stream.Push(initSubReqCommit(subscription), seekResp(0), nil) // Seek instead of init response
	verifiers.AddSubscribeStream(subscription.Path, subscription.Partition, stream)

	mockServer.OnTestStart(verifiers)
	defer mockServer.OnTestEnd()

	sub := newTestSubscribeStream(t, subscription, testSubscriberSettings())
	if gotErr, wantErr := sub.StartError(), errInvalidInitialSubscribeResponse; !test.ErrorEqual(gotErr, wantErr) {
		t.Errorf("Start got err: (%v), want: (%v)", gotErr, wantErr)
	}
}

func TestSubscribeStreamDuplicateInitialResponse(t *testing.T) {
	subscription := subscriptionPartition{"projects/123456/locations/us-central1-b/subscriptions/my-sub", 0}

	verifiers := test.NewVerifiers(t)
	stream := test.NewRPCVerifier(t)
	stream.Push(initSubReqCommit(subscription), initSubResp(), nil)
	stream.Push(initFlowControlReq(), initSubResp(), nil) // Second initial response
	verifiers.AddSubscribeStream(subscription.Path, subscription.Partition, stream)

	mockServer.OnTestStart(verifiers)
	defer mockServer.OnTestEnd()

	sub := newTestSubscribeStream(t, subscription, testSubscriberSettings())
	if gotErr, wantErr := sub.FinalError(), errInvalidSubscribeResponse; !test.ErrorEqual(gotErr, wantErr) {
		t.Errorf("Final err: (%v), want: (%v)", gotErr, wantErr)
	}
}

func TestSubscribeStreamSpuriousSeekResponse(t *testing.T) {
	subscription := subscriptionPartition{"projects/123456/locations/us-central1-b/subscriptions/my-sub", 0}

	verifiers := test.NewVerifiers(t)
	stream := test.NewRPCVerifier(t)
	stream.Push(initSubReqCommit(subscription), initSubResp(), nil)
	stream.Push(initFlowControlReq(), seekResp(1), nil) // Seek response with no seek request
	verifiers.AddSubscribeStream(subscription.Path, subscription.Partition, stream)

	mockServer.OnTestStart(verifiers)
	defer mockServer.OnTestEnd()

	sub := newTestSubscribeStream(t, subscription, testSubscriberSettings())
	if gotErr, wantErr := sub.FinalError(), errInvalidSubscribeResponse; !test.ErrorEqual(gotErr, wantErr) {
		t.Errorf("Final err: (%v), want: (%v)", gotErr, wantErr)
	}
}

func TestSubscribeStreamNoMessages(t *testing.T) {
	subscription := subscriptionPartition{"projects/123456/locations/us-central1-b/subscriptions/my-sub", 0}

	verifiers := test.NewVerifiers(t)
	stream := test.NewRPCVerifier(t)
	stream.Push(initSubReqCommit(subscription), initSubResp(), nil)
	stream.Push(initFlowControlReq(), msgSubResp(), nil) // No messages in response
	verifiers.AddSubscribeStream(subscription.Path, subscription.Partition, stream)

	mockServer.OnTestStart(verifiers)
	defer mockServer.OnTestEnd()

	sub := newTestSubscribeStream(t, subscription, testSubscriberSettings())
	if gotErr, wantErr := sub.FinalError(), errServerNoMessages; !test.ErrorEqual(gotErr, wantErr) {
		t.Errorf("Final err: (%v), want: (%v)", gotErr, wantErr)
	}
}

func TestSubscribeStreamMessagesOutOfOrder(t *testing.T) {
	subscription := subscriptionPartition{"projects/123456/locations/us-central1-b/subscriptions/my-sub", 0}
	msg1 := seqMsgWithOffsetAndSize(56, 100)
	msg2 := seqMsgWithOffsetAndSize(55, 100) // Offset before msg1

	verifiers := test.NewVerifiers(t)
	stream := test.NewRPCVerifier(t)
	stream.Push(initSubReqCommit(subscription), initSubResp(), nil)
	stream.Push(initFlowControlReq(), msgSubResp(msg1), nil)
	stream.Push(nil, msgSubResp(msg2), nil)
	verifiers.AddSubscribeStream(subscription.Path, subscription.Partition, stream)

	mockServer.OnTestStart(verifiers)
	defer mockServer.OnTestEnd()

	sub := newTestSubscribeStream(t, subscription, testSubscriberSettings())
	sub.Receiver.ValidateMsg(msg1)
	if gotErr, msg := sub.FinalError(), "start offset = 55, expected >= 57"; !test.ErrorHasMsg(gotErr, msg) {
		t.Errorf("Final err: (%v), want msg: %q", gotErr, msg)
	}
}

func TestSubscribeStreamFlowControlOverflow(t *testing.T) {
	subscription := subscriptionPartition{"projects/123456/locations/us-central1-b/subscriptions/my-sub", 0}
	msg1 := seqMsgWithOffsetAndSize(56, 900)
	msg2 := seqMsgWithOffsetAndSize(57, 101) // Overflows ReceiveSettings.MaxOutstandingBytes = 1000

	verifiers := test.NewVerifiers(t)
	stream := test.NewRPCVerifier(t)
	stream.Push(initSubReqCommit(subscription), initSubResp(), nil)
	stream.Push(initFlowControlReq(), msgSubResp(msg1), nil)
	stream.Push(nil, msgSubResp(msg2), nil)
	verifiers.AddSubscribeStream(subscription.Path, subscription.Partition, stream)

	mockServer.OnTestStart(verifiers)
	defer mockServer.OnTestEnd()

	sub := newTestSubscribeStream(t, subscription, testSubscriberSettings())
	sub.Receiver.ValidateMsg(msg1)
	if gotErr, wantErr := sub.FinalError(), errTokenCounterBytesNegative; !test.ErrorEqual(gotErr, wantErr) {
		t.Errorf("Final err: (%v), want: (%v)", gotErr, wantErr)
	}
}

func TestSubscribeStreamHandleResetError(t *testing.T) {
	subscription := subscriptionPartition{"projects/123456/locations/us-central1-b/subscriptions/my-sub", 0}
	msg := seqMsgWithOffsetAndSize(67, 100)

	verifiers := test.NewVerifiers(t)
	stream := test.NewRPCVerifier(t)
	stream.Push(initSubReqCommit(subscription), initSubResp(), nil)
	stream.Push(initFlowControlReq(), msgSubResp(msg), nil)
	barrier := stream.PushWithBarrier(nil, nil, makeStreamResetSignal())
	verifiers.AddSubscribeStream(subscription.Path, subscription.Partition, stream)
	// No reconnect expected because the reset handler func will fail.

	mockServer.OnTestStart(verifiers)
	defer mockServer.OnTestEnd()

	sub := newTestSubscribeStream(t, subscription, testSubscriberSettings())
	sub.SetResetErr(status.Error(codes.FailedPrecondition, "reset handler failed"))
	if gotErr := sub.StartError(); gotErr != nil {
		t.Errorf("Start() got err: (%v)", gotErr)
	}
	sub.Receiver.ValidateMsg(msg)
	barrier.Release()
	if gotErr := sub.FinalError(); gotErr != nil {
		t.Errorf("Final err: (%v), want: <nil>", gotErr)
	}
}

func TestSubscribeStreamReceiveLargeMessage(t *testing.T) {
	subscription := subscriptionPartition{"projects/123456/locations/us-central1-b/subscriptions/my-sub", 0}
	const msgSize = 10 * 1024 * 1024 // 10 MiB
	msg := &pb.SequencedMessage{
		Cursor:    &pb.Cursor{Offset: 1},
		SizeBytes: msgSize,
		Message:   &pb.PubSubMessage{Data: bytes.Repeat([]byte{'0'}, msgSize)},
	}

	settings := testSubscriberSettings()
	settings.MaxOutstandingBytes = msgSize
	expectedFlowControlReq := flowControlSubReq(flowControlTokens{
		Bytes:    int64(settings.MaxOutstandingBytes),
		Messages: int64(settings.MaxOutstandingMessages),
	})

	verifiers := test.NewVerifiers(t)
	stream := test.NewRPCVerifier(t)
	stream.Push(initSubReqCommit(subscription), initSubResp(), nil)
	stream.Push(expectedFlowControlReq, msgSubResp(msg), nil)
	verifiers.AddSubscribeStream(subscription.Path, subscription.Partition, stream)

	mockServer.OnTestStart(verifiers)
	defer mockServer.OnTestEnd()

	sub := newTestSubscribeStream(t, subscription, settings)
	sub.Receiver.ValidateMsg(msg)
	sub.StopVerifyNoError()
}

type testSinglePartitionSubscriber singlePartitionSubscriber

func (t *testSinglePartitionSubscriber) WaitStopped() error {
	err := t.compositeService.WaitStopped()
	// Close connections.
	t.committer.cursorClient.Close()
	t.subscriber.subClient.Close()
	return err
}

func newTestSinglePartitionSubscriber(t *testing.T, receiverFunc MessageReceiverFunc, subscription subscriptionPartition) *testSinglePartitionSubscriber {
	ctx := context.Background()
	subClient, err := newSubscriberClient(ctx, "ignored", testServer.ClientConn())
	if err != nil {
		t.Fatal(err)
	}
	cursorClient, err := newCursorClient(ctx, "ignored", testServer.ClientConn())
	if err != nil {
		t.Fatal(err)
	}

	f := &singlePartitionSubscriberFactory{
		ctx:              ctx,
		subClient:        subClient,
		cursorClient:     cursorClient,
		settings:         testSubscriberSettings(),
		subscriptionPath: subscription.Path,
		receiver:         receiverFunc,
		disableTasks:     true, // Background tasks disabled to control event order
	}
	sub := f.New(subscription.Partition)
	sub.Start()
	return (*testSinglePartitionSubscriber)(sub)
}

func TestSinglePartitionSubscriberStartStop(t *testing.T) {
	subscription := subscriptionPartition{"projects/123456/locations/us-central1-b/subscriptions/my-sub", 0}
	receiver := newTestMessageReceiver(t)

	verifiers := test.NewVerifiers(t)

	// Verifies the behavior of the subscribeStream and committer when they are
	// stopped before any messages are received.
	subStream := test.NewRPCVerifier(t)
	subStream.Push(initSubReqCommit(subscription), initSubResp(), nil)
	barrier := subStream.PushWithBarrier(initFlowControlReq(), nil, nil)
	verifiers.AddSubscribeStream(subscription.Path, subscription.Partition, subStream)

	cmtStream := test.NewRPCVerifier(t)
	cmtStream.Push(initCommitReq(subscription), initCommitResp(), nil)
	verifiers.AddCommitStream(subscription.Path, subscription.Partition, cmtStream)

	mockServer.OnTestStart(verifiers)
	defer mockServer.OnTestEnd()

	sub := newTestSinglePartitionSubscriber(t, receiver.onMessage, subscription)
	if gotErr := sub.WaitStarted(); gotErr != nil {
		t.Errorf("Start() got err: (%v)", gotErr)
	}
	barrier.Release() // To ensure the test is deterministic (i.e. flow control req always received)
	sub.Stop()
	if gotErr := sub.WaitStopped(); gotErr != nil {
		t.Errorf("Stop() got err: (%v)", gotErr)
	}
}

func TestSinglePartitionSubscriberSimpleMsgAck(t *testing.T) {
	subscription := subscriptionPartition{"projects/123456/locations/us-central1-b/subscriptions/my-sub", 0}
	receiver := newTestMessageReceiver(t)
	msg1 := seqMsgWithOffsetAndSize(22, 100)
	msg2 := seqMsgWithOffsetAndSize(23, 200)

	verifiers := test.NewVerifiers(t)

	subStream := test.NewRPCVerifier(t)
	subStream.Push(initSubReqCommit(subscription), initSubResp(), nil)
	subStream.Push(initFlowControlReq(), msgSubResp(msg1, msg2), nil)
	verifiers.AddSubscribeStream(subscription.Path, subscription.Partition, subStream)

	cmtStream := test.NewRPCVerifier(t)
	cmtStream.Push(initCommitReq(subscription), initCommitResp(), nil)
	cmtStream.Push(commitReq(24), commitResp(1), nil)
	verifiers.AddCommitStream(subscription.Path, subscription.Partition, cmtStream)

	mockServer.OnTestStart(verifiers)
	defer mockServer.OnTestEnd()

	sub := newTestSinglePartitionSubscriber(t, receiver.onMessage, subscription)
	if gotErr := sub.WaitStarted(); gotErr != nil {
		t.Errorf("Start() got err: (%v)", gotErr)
	}
	receiver.ValidateMsg(msg1).Ack()
	receiver.ValidateMsg(msg2).Ack()
	sub.Stop()
	if gotErr := sub.WaitStopped(); gotErr != nil {
		t.Errorf("Stop() got err: (%v)", gotErr)
	}
}

func TestSinglePartitionSubscriberMessageQueue(t *testing.T) {
	subscription := subscriptionPartition{"projects/123456/locations/us-central1-b/subscriptions/my-sub", 0}
	receiver := newTestBlockingMessageReceiver(t)
	msg1 := seqMsgWithOffsetAndSize(1, 100)
	msg2 := seqMsgWithOffsetAndSize(2, 100)
	msg3 := seqMsgWithOffsetAndSize(3, 100)
	retryableErr := status.Error(codes.Unavailable, "should retry")

	verifiers := test.NewVerifiers(t)

	subStream1 := test.NewRPCVerifier(t)
	subStream1.Push(initSubReqCommit(subscription), initSubResp(), nil)
	subStream1.Push(initFlowControlReq(), msgSubResp(msg1), nil)
	subStream1.Push(nil, msgSubResp(msg2), nil)
	subStream1.Push(nil, nil, retryableErr)
	verifiers.AddSubscribeStream(subscription.Path, subscription.Partition, subStream1)

	// When reconnected, the subscribeStream should set initial cursor to msg3 and
	// have subtracted flow control tokens for msg1 and msg2.
	subStream2 := test.NewRPCVerifier(t)
	subStream2.Push(initSubReqCursor(subscription, 3), initSubResp(), nil)
	subStream2.Push(flowControlSubReq(flowControlTokens{Bytes: 800, Messages: 8}), msgSubResp(msg3), nil)
	verifiers.AddSubscribeStream(subscription.Path, subscription.Partition, subStream2)

	cmtStream := test.NewRPCVerifier(t)
	cmtStream.Push(initCommitReq(subscription), initCommitResp(), nil)
	cmtStream.Push(commitReq(4), commitResp(1), nil)
	verifiers.AddCommitStream(subscription.Path, subscription.Partition, cmtStream)

	mockServer.OnTestStart(verifiers)
	defer mockServer.OnTestEnd()

	sub := newTestSinglePartitionSubscriber(t, receiver.onMessage, subscription)
	if gotErr := sub.WaitStarted(); gotErr != nil {
		t.Errorf("Start() got err: (%v)", gotErr)
	}

	// Verifies that messageDeliveryQueue delivers messages sequentially and waits
	// for the client message receiver func to return before delivering the next
	// message.
	var acks []AckConsumer
	for _, msg := range []*pb.SequencedMessage{msg1, msg2, msg3} {
		ack := receiver.ValidateMsg(msg)
		acks = append(acks, ack)
		receiver.VerifyNoMsgs()
		receiver.Return()
	}

	// Ack all messages so that the committer terminates.
	for _, ack := range acks {
		ack.Ack()
	}

	sub.Stop()
	if gotErr := sub.WaitStopped(); gotErr != nil {
		t.Errorf("Stop() got err: (%v)", gotErr)
	}
}

func TestSinglePartitionSubscriberStopDuringReceive(t *testing.T) {
	subscription := subscriptionPartition{"projects/123456/locations/us-central1-b/subscriptions/my-sub", 0}
	receiver := newTestBlockingMessageReceiver(t)
	msg1 := seqMsgWithOffsetAndSize(1, 100)
	msg2 := seqMsgWithOffsetAndSize(2, 100)

	verifiers := test.NewVerifiers(t)

	subStream := test.NewRPCVerifier(t)
	subStream.Push(initSubReqCommit(subscription), initSubResp(), nil)
	subStream.Push(initFlowControlReq(), msgSubResp(msg1, msg2), nil)
	verifiers.AddSubscribeStream(subscription.Path, subscription.Partition, subStream)

	cmtStream := test.NewRPCVerifier(t)
	cmtStream.Push(initCommitReq(subscription), initCommitResp(), nil)
	cmtStream.Push(commitReq(2), commitResp(1), nil)
	verifiers.AddCommitStream(subscription.Path, subscription.Partition, cmtStream)

	mockServer.OnTestStart(verifiers)
	defer mockServer.OnTestEnd()

	sub := newTestSinglePartitionSubscriber(t, receiver.onMessage, subscription)
	if gotErr := sub.WaitStarted(); gotErr != nil {
		t.Errorf("Start() got err: (%v)", gotErr)
	}

	receiver.ValidateMsg(msg1).Ack()

	// Stop the subscriber before returning from the message receiver func.
	sub.Stop()
	receiver.Return()

	if gotErr := sub.WaitStopped(); gotErr != nil {
		t.Errorf("Stop() got err: (%v)", gotErr)
	}
	receiver.VerifyNoMsgs() // msg2 should not be received
}

func TestSinglePartitionSubscriberAdminSeekWhileConnected(t *testing.T) {
	subscription := subscriptionPartition{"projects/123456/locations/us-central1-b/subscriptions/my-sub", 0}
	receiver := newTestMessageReceiver(t)
	msg1 := seqMsgWithOffsetAndSize(1, 100)
	msg2 := seqMsgWithOffsetAndSize(2, 100)
	msg3 := seqMsgWithOffsetAndSize(3, 100)

	verifiers := test.NewVerifiers(t)

	subStream1 := test.NewRPCVerifier(t)
	subStream1.Push(initSubReqCommit(subscription), initSubResp(), nil)
	subStream1.Push(initFlowControlReq(), msgSubResp(msg1, msg2, msg3), nil)
	// Server disconnects the stream with the RESET signal.
	barrier := subStream1.PushWithBarrier(nil, nil, makeStreamResetSignal())
	verifiers.AddSubscribeStream(subscription.Path, subscription.Partition, subStream1)

	subStream2 := test.NewRPCVerifier(t)
	// Reconnected stream reads from commit cursor.
	subStream2.Push(initSubReqCommit(subscription), initSubResp(), nil)
	// Ensure that the subscriber resets state and can handle seeking back to
	// msg1.
	subStream2.Push(initFlowControlReq(), msgSubResp(msg1), nil)
	verifiers.AddSubscribeStream(subscription.Path, subscription.Partition, subStream2)

	cmtStream := test.NewRPCVerifier(t)
	cmtStream.Push(initCommitReq(subscription), initCommitResp(), nil)
	cmtStream.Push(commitReq(4), commitResp(1), nil)
	cmtStream.Push(commitReq(2), commitResp(1), nil)
	verifiers.AddCommitStream(subscription.Path, subscription.Partition, cmtStream)

	mockServer.OnTestStart(verifiers)
	defer mockServer.OnTestEnd()

	sub := newTestSinglePartitionSubscriber(t, receiver.onMessage, subscription)
	if gotErr := sub.WaitStarted(); gotErr != nil {
		t.Errorf("Start() got err: (%v)", gotErr)
	}

	receiver.ValidateMsg(msg1).Ack()
	receiver.ValidateMsg(msg2).Ack()
	receiver.ValidateMsg(msg3).Ack()
	barrier.Release()
	receiver.ValidateMsg(msg1).Ack()

	sub.Stop()
	if gotErr := sub.WaitStopped(); gotErr != nil {
		t.Errorf("Stop() got err: (%v)", gotErr)
	}
}

func TestSinglePartitionSubscriberAdminSeekWhileReconnecting(t *testing.T) {
	subscription := subscriptionPartition{"projects/123456/locations/us-central1-b/subscriptions/my-sub", 0}
	receiver := newTestMessageReceiver(t)
	msg1 := seqMsgWithOffsetAndSize(1, 100)
	msg2 := seqMsgWithOffsetAndSize(2, 100)
	msg3 := seqMsgWithOffsetAndSize(3, 100)

	verifiers := test.NewVerifiers(t)

	subStream1 := test.NewRPCVerifier(t)
	subStream1.Push(initSubReqCommit(subscription), initSubResp(), nil)
	subStream1.Push(initFlowControlReq(), msgSubResp(msg1, msg2, msg3), nil)
	// Normal stream breakage.
	barrier := subStream1.PushWithBarrier(nil, nil, status.Error(codes.DeadlineExceeded, ""))
	verifiers.AddSubscribeStream(subscription.Path, subscription.Partition, subStream1)

	subStream2 := test.NewRPCVerifier(t)
	// The server sends the RESET signal during stream initialization.
	subStream2.Push(initSubReqCursor(subscription, 4), nil, makeStreamResetSignal())
	verifiers.AddSubscribeStream(subscription.Path, subscription.Partition, subStream2)

	subStream3 := test.NewRPCVerifier(t)
	// Reconnected stream reads from commit cursor.
	subStream3.Push(initSubReqCommit(subscription), initSubResp(), nil)
	// Ensure that the subscriber resets state and can handle seeking back to
	// msg1.
	subStream3.Push(initFlowControlReq(), msgSubResp(msg1), nil)
	verifiers.AddSubscribeStream(subscription.Path, subscription.Partition, subStream3)

	cmtStream := test.NewRPCVerifier(t)
	cmtStream.Push(initCommitReq(subscription), initCommitResp(), nil)
	cmtStream.Push(commitReq(3), commitResp(1), nil)
	cmtStream.Push(commitReq(2), commitResp(1), nil)
	verifiers.AddCommitStream(subscription.Path, subscription.Partition, cmtStream)

	mockServer.OnTestStart(verifiers)
	defer mockServer.OnTestEnd()

	sub := newTestSinglePartitionSubscriber(t, receiver.onMessage, subscription)
	if gotErr := sub.WaitStarted(); gotErr != nil {
		t.Errorf("Start() got err: (%v)", gotErr)
	}

	receiver.ValidateMsg(msg1).Ack()
	receiver.ValidateMsg(msg2).Ack()
	ack := receiver.ValidateMsg(msg3) // Unacked message discarded
	barrier.Release()
	receiver.ValidateMsg(msg1).Ack()
	ack.Ack() // Should be ignored

	sub.Stop()
	if gotErr := sub.WaitStopped(); gotErr != nil {
		t.Errorf("Stop() got err: (%v)", gotErr)
	}
}

func TestSinglePartitionSubscriberStopDuringAdminSeek(t *testing.T) {
	subscription := subscriptionPartition{"projects/123456/locations/us-central1-b/subscriptions/my-sub", 0}
	receiver := newTestMessageReceiver(t)
	msg1 := seqMsgWithOffsetAndSize(1, 100)
	msg2 := seqMsgWithOffsetAndSize(2, 100)

	verifiers := test.NewVerifiers(t)

	subStream := test.NewRPCVerifier(t)
	subStream.Push(initSubReqCommit(subscription), initSubResp(), nil)
	subStream.Push(initFlowControlReq(), msgSubResp(msg1, msg2), nil)
	// Server disconnects the stream with the RESET signal.
	subBarrier := subStream.PushWithBarrier(nil, nil, makeStreamResetSignal())
	verifiers.AddSubscribeStream(subscription.Path, subscription.Partition, subStream)

	cmtStream := test.NewRPCVerifier(t)
	cmtStream.Push(initCommitReq(subscription), initCommitResp(), nil)
	cmtBarrier := cmtStream.PushWithBarrier(commitReq(3), commitResp(1), nil)
	verifiers.AddCommitStream(subscription.Path, subscription.Partition, cmtStream)

	mockServer.OnTestStart(verifiers)
	defer mockServer.OnTestEnd()

	sub := newTestSinglePartitionSubscriber(t, receiver.onMessage, subscription)
	if gotErr := sub.WaitStarted(); gotErr != nil {
		t.Errorf("Start() got err: (%v)", gotErr)
	}

	receiver.ValidateMsg(msg1).Ack()
	receiver.ValidateMsg(msg2).Ack()
	subBarrier.Release()

	// Ensure that the user is able to call Stop while a reset is in progress.
	// Verifies that the subscribeStream is not holding mutexes while waiting and
	// that the subscribe stream is not reconnected.
	cmtBarrier.ReleaseAfter(func() {
		sub.Stop()
	})

	if gotErr := sub.WaitStopped(); gotErr != nil {
		t.Errorf("Stop() got err: (%v)", gotErr)
	}
}

func verifyPartitionsActive(t *testing.T, sub Subscriber, want bool, partitions ...int) {
	t.Helper()
	for _, p := range partitions {
		if got := sub.PartitionActive(p); got != want {
			t.Errorf("PartitionActive(%d) got %v, want %v", p, got, want)
		}
	}
}

func newTestMultiPartitionSubscriber(t *testing.T, receiverFunc MessageReceiverFunc, subscriptionPath string, partitions []int) *multiPartitionSubscriber {
	ctx := context.Background()
	subClient, err := newSubscriberClient(ctx, "ignored", testServer.ClientConn())
	if err != nil {
		t.Fatal(err)
	}
	cursorClient, err := newCursorClient(ctx, "ignored", testServer.ClientConn())
	if err != nil {
		t.Fatal(err)
	}
	allClients := apiClients{subClient, cursorClient}

	f := &singlePartitionSubscriberFactory{
		ctx:              ctx,
		subClient:        subClient,
		cursorClient:     cursorClient,
		settings:         testSubscriberSettings(),
		subscriptionPath: subscriptionPath,
		receiver:         receiverFunc,
		disableTasks:     true, // Background tasks disabled to control event order
	}
	f.settings.Partitions = partitions
	sub := newMultiPartitionSubscriber(allClients, f)
	sub.Start()
	return sub
}

func TestMultiPartitionSubscriberMultipleMessages(t *testing.T) {
	const subscription = "projects/123456/locations/us-central1-b/subscriptions/my-sub"
	receiver := newTestMessageReceiver(t)
	msg1 := seqMsgWithOffsetAndSize(22, 100)
	msg2 := seqMsgWithOffsetAndSize(23, 200)
	msg3 := seqMsgWithOffsetAndSize(44, 100)
	msg4 := seqMsgWithOffsetAndSize(45, 200)

	verifiers := test.NewVerifiers(t)

	// Partition 1
	subStream1 := test.NewRPCVerifier(t)
	subStream1.Push(initSubReqCommit(subscriptionPartition{Path: subscription, Partition: 1}), initSubResp(), nil)
	subStream1.Push(initFlowControlReq(), msgSubResp(msg1), nil)
	subStream1.Push(nil, msgSubResp(msg2), nil)
	verifiers.AddSubscribeStream(subscription, 1, subStream1)

	cmtStream1 := test.NewRPCVerifier(t)
	cmtStream1.Push(initCommitReq(subscriptionPartition{Path: subscription, Partition: 1}), initCommitResp(), nil)
	cmtStream1.Push(commitReq(24), commitResp(1), nil)
	verifiers.AddCommitStream(subscription, 1, cmtStream1)

	// Partition 2
	subStream2 := test.NewRPCVerifier(t)
	subStream2.Push(initSubReqCommit(subscriptionPartition{Path: subscription, Partition: 2}), initSubResp(), nil)
	subStream2.Push(initFlowControlReq(), msgSubResp(msg3), nil)
	subStream2.Push(nil, msgSubResp(msg4), nil)
	verifiers.AddSubscribeStream(subscription, 2, subStream2)

	cmtStream2 := test.NewRPCVerifier(t)
	cmtStream2.Push(initCommitReq(subscriptionPartition{Path: subscription, Partition: 2}), initCommitResp(), nil)
	cmtStream2.Push(commitReq(46), commitResp(1), nil)
	verifiers.AddCommitStream(subscription, 2, cmtStream2)

	mockServer.OnTestStart(verifiers)
	defer mockServer.OnTestEnd()

	sub := newTestMultiPartitionSubscriber(t, receiver.onMessage, subscription, []int{1, 2})
	verifyPartitionsActive(t, sub, true, 1, 2)
	verifyPartitionsActive(t, sub, false, 0, 3)

	if gotErr := sub.WaitStarted(); gotErr != nil {
		t.Errorf("Start() got err: (%v)", gotErr)
	}
	receiver.ValidateMsgs(join(partitionMsgs(1, msg1, msg2), partitionMsgs(2, msg3, msg4)))
	sub.Stop()
	if gotErr := sub.WaitStopped(); gotErr != nil {
		t.Errorf("Stop() got err: (%v)", gotErr)
	}
}

func TestMultiPartitionSubscriberPermanentError(t *testing.T) {
	const subscription = "projects/123456/locations/us-central1-b/subscriptions/my-sub"
	receiver := newTestMessageReceiver(t)
	msg1 := seqMsgWithOffsetAndSize(22, 100)
	msg2 := seqMsgWithOffsetAndSize(23, 200)
	msg3 := seqMsgWithOffsetAndSize(44, 100)
	serverErr := status.Error(codes.FailedPrecondition, "failed")

	verifiers := test.NewVerifiers(t)

	// Partition 1
	subStream1 := test.NewRPCVerifier(t)
	subStream1.Push(initSubReqCommit(subscriptionPartition{Path: subscription, Partition: 1}), initSubResp(), nil)
	subStream1.Push(initFlowControlReq(), msgSubResp(msg1), nil)
	msg2Barrier := subStream1.PushWithBarrier(nil, msgSubResp(msg2), nil)
	verifiers.AddSubscribeStream(subscription, 1, subStream1)

	cmtStream1 := test.NewRPCVerifier(t)
	cmtStream1.Push(initCommitReq(subscriptionPartition{Path: subscription, Partition: 1}), initCommitResp(), nil)
	cmtStream1.Push(commitReq(23), commitResp(1), nil)
	verifiers.AddCommitStream(subscription, 1, cmtStream1)

	// Partition 2
	subStream2 := test.NewRPCVerifier(t)
	subStream2.Push(initSubReqCommit(subscriptionPartition{Path: subscription, Partition: 2}), initSubResp(), nil)
	subStream2.Push(initFlowControlReq(), msgSubResp(msg3), nil)
	errorBarrier := subStream2.PushWithBarrier(nil, nil, serverErr)
	verifiers.AddSubscribeStream(subscription, 2, subStream2)

	cmtStream2 := test.NewRPCVerifier(t)
	cmtStream2.Push(initCommitReq(subscriptionPartition{Path: subscription, Partition: 2}), initCommitResp(), nil)
	cmtStream2.Push(commitReq(45), commitResp(1), nil)
	verifiers.AddCommitStream(subscription, 2, cmtStream2)

	mockServer.OnTestStart(verifiers)
	defer mockServer.OnTestEnd()

	sub := newTestMultiPartitionSubscriber(t, receiver.onMessage, subscription, []int{1, 2})
	if gotErr := sub.WaitStarted(); gotErr != nil {
		t.Errorf("Start() got err: (%v)", gotErr)
	}
	receiver.ValidateMsgs(join(partitionMsgs(1, msg1), partitionMsgs(2, msg3)))
	errorBarrier.Release() // Release server error now to ensure test is deterministic
	if gotErr := sub.WaitStopped(); !test.ErrorEqual(gotErr, serverErr) {
		t.Errorf("Final error got: (%v), want: (%v)", gotErr, serverErr)
	}

	// Verify msg2 never received as subscriber has terminated.
	msg2Barrier.Release()
	receiver.VerifyNoMsgs()
}

func (as *assigningSubscriber) Subscribers() []*singlePartitionSubscriber {
	as.mu.Lock()
	defer as.mu.Unlock()

	var subscribers []*singlePartitionSubscriber
	for _, s := range as.subscribers {
		subscribers = append(subscribers, s)
	}
	return subscribers
}

func (as *assigningSubscriber) FlushCommits() {
	as.mu.Lock()
	defer as.mu.Unlock()

	for _, sub := range as.subscribers {
		sub.committer.commitOffsetToStream()
	}
}

func noopReassignmentHandler(_, _ PartitionSet) error {
	return nil
}

func newTestAssigningSubscriber(t *testing.T, receiverFunc MessageReceiverFunc, reassignmentHandler ReassignmentHandlerFunc, subscriptionPath string) *assigningSubscriber {
	ctx := context.Background()
	subClient, err := newSubscriberClient(ctx, "ignored", testServer.ClientConn())
	if err != nil {
		t.Fatal(err)
	}
	cursorClient, err := newCursorClient(ctx, "ignored", testServer.ClientConn())
	if err != nil {
		t.Fatal(err)
	}
	assignmentClient, err := newPartitionAssignmentClient(ctx, "ignored", testServer.ClientConn())
	if err != nil {
		t.Fatal(err)
	}
	allClients := apiClients{subClient, cursorClient, assignmentClient}

	f := &singlePartitionSubscriberFactory{
		ctx:              ctx,
		subClient:        subClient,
		cursorClient:     cursorClient,
		settings:         testSubscriberSettings(),
		subscriptionPath: subscriptionPath,
		receiver:         receiverFunc,
		disableTasks:     true, // Background tasks disabled to control event order
	}
	sub, err := newAssigningSubscriber(allClients, assignmentClient, reassignmentHandler, fakeGenerateUUID, f)
	if err != nil {
		t.Fatal(err)
	}
	sub.Start()
	return sub
}

func TestAssigningSubscriberAddRemovePartitions(t *testing.T) {
	const subscription = "projects/123456/locations/us-central1-b/subscriptions/my-sub"
	receiver := newTestMessageReceiver(t)
	msg1 := seqMsgWithOffsetAndSize(33, 100)
	msg2 := seqMsgWithOffsetAndSize(34, 200)
	msg3 := seqMsgWithOffsetAndSize(66, 100)
	msg4 := seqMsgWithOffsetAndSize(67, 100)
	msg5 := seqMsgWithOffsetAndSize(88, 100)

	verifiers := test.NewVerifiers(t)

	// Assignment stream
	asnStream := test.NewRPCVerifier(t)
	asnStream.Push(initAssignmentReq(subscription, fakeUUID[:]), assignmentResp([]int64{3, 6}), nil)
	assignmentBarrier1 := asnStream.PushWithBarrier(assignmentAckReq(), assignmentResp([]int64{3, 8}), nil)
	assignmentBarrier2 := asnStream.PushWithBarrier(assignmentAckReq(), nil, nil)
	verifiers.AddAssignmentStream(subscription, asnStream)

	// Partition 3
	subStream3 := test.NewRPCVerifier(t)
	subStream3.Push(initSubReqCommit(subscriptionPartition{Path: subscription, Partition: 3}), initSubResp(), nil)
	subStream3.Push(initFlowControlReq(), msgSubResp(msg1), nil)
	msg2Barrier := subStream3.PushWithBarrier(nil, msgSubResp(msg2), nil)
	verifiers.AddSubscribeStream(subscription, 3, subStream3)

	cmtStream3 := test.NewRPCVerifier(t)
	cmtStream3.Push(initCommitReq(subscriptionPartition{Path: subscription, Partition: 3}), initCommitResp(), nil)
	cmtStream3.Push(commitReq(34), commitResp(1), nil)
	cmtStream3.Push(commitReq(35), commitResp(1), nil)
	verifiers.AddCommitStream(subscription, 3, cmtStream3)

	// Partition 6
	subStream6 := test.NewRPCVerifier(t)
	subStream6.Push(initSubReqCommit(subscriptionPartition{Path: subscription, Partition: 6}), initSubResp(), nil)
	subStream6.Push(initFlowControlReq(), msgSubResp(msg3), nil)
	// msg4 should not be received.
	msg4Barrier := subStream6.PushWithBarrier(nil, msgSubResp(msg4), nil)
	verifiers.AddSubscribeStream(subscription, 6, subStream6)

	cmtStream6 := test.NewRPCVerifier(t)
	cmtStream6.Push(initCommitReq(subscriptionPartition{Path: subscription, Partition: 6}), initCommitResp(), nil)
	cmtStream6.Push(commitReq(67), commitResp(1), nil)
	verifiers.AddCommitStream(subscription, 6, cmtStream6)

	// Partition 8
	subStream8 := test.NewRPCVerifier(t)
	subStream8.Push(initSubReqCommit(subscriptionPartition{Path: subscription, Partition: 8}), initSubResp(), nil)
	subStream8.Push(initFlowControlReq(), msgSubResp(msg5), nil)
	verifiers.AddSubscribeStream(subscription, 8, subStream8)

	cmtStream8 := test.NewRPCVerifier(t)
	cmtStream8.Push(initCommitReq(subscriptionPartition{Path: subscription, Partition: 8}), initCommitResp(), nil)
	cmtStream8.Push(commitReq(89), commitResp(1), nil)
	verifiers.AddCommitStream(subscription, 8, cmtStream8)

	mockServer.OnTestStart(verifiers)
	defer mockServer.OnTestEnd()

	sub := newTestAssigningSubscriber(t, receiver.onMessage, noopReassignmentHandler, subscription)
	if gotErr := sub.WaitStarted(); gotErr != nil {
		t.Errorf("Start() got err: (%v)", gotErr)
	}

	// Partition assignments are initially {3, 6}.
	receiver.ValidateMsgs(join(partitionMsgs(3, msg1), partitionMsgs(6, msg3)))
	verifyPartitionsActive(t, sub, true, 3, 6)
	verifyPartitionsActive(t, sub, false, 1, 8)

	// Partition assignments will now be {3, 8}.
	assignmentBarrier1.Release()
	receiver.ValidateMsgs(partitionMsgs(8, msg5))
	verifyPartitionsActive(t, sub, true, 3, 8)
	verifyPartitionsActive(t, sub, false, 2, 6)

	// msg2 is from partition 3 and should be received. msg4 is from partition 6
	// (removed) and should be discarded.
	sub.FlushCommits()
	msg2Barrier.Release()
	msg4Barrier.Release()
	receiver.ValidateMsgs(partitionMsgs(3, msg2))

	// Ensure the second assignment ack is received by the server to avoid test
	// flakiness.
	assignmentBarrier2.Release()

	// Stop should flush all commit cursors.
	sub.Stop()
	if gotErr := sub.WaitStopped(); gotErr != nil {
		t.Errorf("Stop() got err: (%v)", gotErr)
	}
}

func TestAssigningSubscriberPermanentError(t *testing.T) {
	const subscription = "projects/123456/locations/us-central1-b/subscriptions/my-sub"
	receiver := newTestMessageReceiver(t)
	msg1 := seqMsgWithOffsetAndSize(11, 100)
	msg2 := seqMsgWithOffsetAndSize(22, 200)
	serverErr := status.Error(codes.FailedPrecondition, "failed")

	verifiers := test.NewVerifiers(t)

	// Assignment stream
	asnStream := test.NewRPCVerifier(t)
	asnStream.Push(initAssignmentReq(subscription, fakeUUID[:]), assignmentResp([]int64{1, 2}), nil)
	errBarrier := asnStream.PushWithBarrier(assignmentAckReq(), nil, serverErr)
	verifiers.AddAssignmentStream(subscription, asnStream)

	// Partition 1
	subStream1 := test.NewRPCVerifier(t)
	subStream1.Push(initSubReqCommit(subscriptionPartition{Path: subscription, Partition: 1}), initSubResp(), nil)
	subStream1.Push(initFlowControlReq(), msgSubResp(msg1), nil)
	verifiers.AddSubscribeStream(subscription, 1, subStream1)

	cmtStream1 := test.NewRPCVerifier(t)
	cmtStream1.Push(initCommitReq(subscriptionPartition{Path: subscription, Partition: 1}), initCommitResp(), nil)
	cmtStream1.Push(commitReq(12), commitResp(1), nil)
	verifiers.AddCommitStream(subscription, 1, cmtStream1)

	// Partition 2
	subStream2 := test.NewRPCVerifier(t)
	subStream2.Push(initSubReqCommit(subscriptionPartition{Path: subscription, Partition: 2}), initSubResp(), nil)
	subStream2.Push(initFlowControlReq(), msgSubResp(msg2), nil)
	verifiers.AddSubscribeStream(subscription, 2, subStream2)

	cmtStream2 := test.NewRPCVerifier(t)
	cmtStream2.Push(initCommitReq(subscriptionPartition{Path: subscription, Partition: 2}), initCommitResp(), nil)
	cmtStream2.Push(commitReq(23), commitResp(1), nil)
	verifiers.AddCommitStream(subscription, 2, cmtStream2)

	mockServer.OnTestStart(verifiers)
	defer mockServer.OnTestEnd()

	sub := newTestAssigningSubscriber(t, receiver.onMessage, noopReassignmentHandler, subscription)
	if gotErr := sub.WaitStarted(); gotErr != nil {
		t.Errorf("Start() got err: (%v)", gotErr)
	}
	receiver.ValidateMsgs(join(partitionMsgs(1, msg1), partitionMsgs(2, msg2)))

	// Permanent assignment stream error should terminate subscriber. Commits are
	// still flushed.
	errBarrier.Release()
	if gotErr := sub.WaitStopped(); !test.ErrorEqual(gotErr, serverErr) {
		t.Errorf("Final error got: (%v), want: (%v)", gotErr, serverErr)
	}
}

func TestAssigningSubscriberIgnoreOutstandingAcks(t *testing.T) {
	const subscription = "projects/123456/locations/us-central1-b/subscriptions/my-sub"
	receiver := newTestMessageReceiver(t)
	msg1 := seqMsgWithOffsetAndSize(11, 100)
	msg2 := seqMsgWithOffsetAndSize(22, 200)

	verifiers := test.NewVerifiers(t)

	// Assignment stream
	asnStream := test.NewRPCVerifier(t)
	asnStream.Push(initAssignmentReq(subscription, fakeUUID[:]), assignmentResp([]int64{1}), nil)
	assignmentBarrier1 := asnStream.PushWithBarrier(assignmentAckReq(), assignmentResp([]int64{}), nil)
	assignmentBarrier2 := asnStream.PushWithBarrier(assignmentAckReq(), nil, nil)
	verifiers.AddAssignmentStream(subscription, asnStream)

	// Partition 1
	subStream := test.NewRPCVerifier(t)
	subStream.Push(initSubReqCommit(subscriptionPartition{Path: subscription, Partition: 1}), initSubResp(), nil)
	subStream.Push(initFlowControlReq(), msgSubResp(msg1, msg2), nil)
	verifiers.AddSubscribeStream(subscription, 1, subStream)

	cmtStream := test.NewRPCVerifier(t)
	cmtStream.Push(initCommitReq(subscriptionPartition{Path: subscription, Partition: 1}), initCommitResp(), nil)
	cmtStream.Push(commitReq(12), commitResp(1), nil)
	verifiers.AddCommitStream(subscription, 1, cmtStream)

	mockServer.OnTestStart(verifiers)
	defer mockServer.OnTestEnd()

	sub := newTestAssigningSubscriber(t, receiver.onMessage, noopReassignmentHandler, subscription)
	if gotErr := sub.WaitStarted(); gotErr != nil {
		t.Errorf("Start() got err: (%v)", gotErr)
	}

	// Partition assignments are initially {1}.
	receiver.ValidateMsg(msg1).Ack()
	ack2 := receiver.ValidateMsg(msg2)
	subscribers := sub.Subscribers()

	// Partition assignments will now be {}.
	assignmentBarrier1.Release()
	assignmentBarrier2.ReleaseAfter(func() {
		// Verify that the assignment is acked after the subscriber has terminated.
		if got, want := len(subscribers), 1; got != want {
			t.Errorf("singlePartitionSubcriber count: got %d, want %d", got, want)
			return
		}
		if got, want := subscribers[0].Status(), serviceTerminated; got != want {
			t.Errorf("singlePartitionSubcriber status: got %v, want %v", got, want)
		}
	})

	// Partition 1 has already been unassigned, so this ack is discarded.
	ack2.Ack()

	sub.Stop()
	if gotErr := sub.WaitStopped(); gotErr != nil {
		t.Errorf("Stop() got err: (%v)", gotErr)
	}
}

func TestAssigningSubscriberStoppedWhileReassignmentHandlerActive(t *testing.T) {
	const subscription = "projects/123456/locations/us-central1-b/subscriptions/my-sub"
	receiver := newTestMessageReceiver(t)

	verifiers := test.NewVerifiers(t)

	// Assignment stream
	asnStream := test.NewRPCVerifier(t)
	asnStream.Push(initAssignmentReq(subscription, fakeUUID[:]), assignmentResp([]int64{1}), nil)
	verifiers.AddAssignmentStream(subscription, asnStream)

	// Partition 1
	subStream := test.NewRPCVerifier(t)
	subStream.Push(initSubReqCommit(subscriptionPartition{Path: subscription, Partition: 1}), initSubResp(), nil)
	subBarrier := subStream.PushWithBarrier(initFlowControlReq(), nil, nil)
	verifiers.AddSubscribeStream(subscription, 1, subStream)

	cmtStream := test.NewRPCVerifier(t)
	cmtBarrier := cmtStream.PushWithBarrier(initCommitReq(subscriptionPartition{Path: subscription, Partition: 1}), initCommitResp(), nil)
	verifiers.AddCommitStream(subscription, 1, cmtStream)

	mockServer.OnTestStart(verifiers)
	defer mockServer.OnTestEnd()

	reassignmentHandlerCalled := test.NewCondition("reassignment handler called")
	returnReassignmentHandler := test.NewCondition("return reassignment handler")
	onReassignment := func(before, after PartitionSet) error {
		if got, want := len(before.SortedInts()), 0; got != want {
			t.Errorf("len(before): got %v, want %v", got, want)
		}
		if got, want := after.SortedInts(), []int{1}; !testutil.Equal(got, want) {
			t.Errorf("after: got %v, want %v", got, want)
		}
		reassignmentHandlerCalled.SetDone()
		returnReassignmentHandler.WaitUntilDone(t, serviceTestWaitTimeout)
		return nil
	}

	sub := newTestAssigningSubscriber(t, receiver.onMessage, onReassignment, subscription)
	if gotErr := sub.WaitStarted(); gotErr != nil {
		t.Errorf("Start() got err: (%v)", gotErr)
	}

	// Used to control order of execution to ensure the test is deterministic.
	subBarrier.Release()
	cmtBarrier.Release()

	// Ensure there are no deadlocks if the reassignment handler blocks and the
	// subscriber is stopped.
	reassignmentHandlerCalled.WaitUntilDone(t, serviceTestWaitTimeout)
	sub.Stop()
	returnReassignmentHandler.SetDone()

	if gotErr := sub.WaitStopped(); gotErr != nil {
		t.Errorf("WaitStopped() got err: (%v)", gotErr)
	}
}

func TestAssigningSubscriberReassignmentHandlerReturnsError(t *testing.T) {
	const subscription = "projects/123456/locations/us-central1-b/subscriptions/my-sub"
	receiver := newTestMessageReceiver(t)

	verifiers := test.NewVerifiers(t)

	// Assignment stream
	asnStream := test.NewRPCVerifier(t)
	asnStream.Push(initAssignmentReq(subscription, fakeUUID[:]), assignmentResp([]int64{1}), nil)
	verifiers.AddAssignmentStream(subscription, asnStream)

	// Partition 1
	subStream := test.NewRPCVerifier(t)
	subStream.Push(initSubReqCommit(subscriptionPartition{Path: subscription, Partition: 1}), initSubResp(), nil)
	subBarrier := subStream.PushWithBarrier(initFlowControlReq(), nil, nil)
	verifiers.AddSubscribeStream(subscription, 1, subStream)

	cmtStream := test.NewRPCVerifier(t)
	cmtBarrier := cmtStream.PushWithBarrier(initCommitReq(subscriptionPartition{Path: subscription, Partition: 1}), initCommitResp(), nil)
	verifiers.AddCommitStream(subscription, 1, cmtStream)

	mockServer.OnTestStart(verifiers)
	defer mockServer.OnTestEnd()

	reassignmentErr := errors.New("reassignment handler error")
	returnReassignmentErr := test.NewCondition("return reassignment error")
	onAssignment := func(before, after PartitionSet) error {
		if got, want := len(before.SortedInts()), 0; got != want {
			t.Errorf("len(before): got %v, want %v", got, want)
		}
		if got, want := after.SortedInts(), []int{1}; !testutil.Equal(got, want) {
			t.Errorf("after: got %v, want %v", got, want)
		}
		returnReassignmentErr.WaitUntilDone(t, serviceTestWaitTimeout)
		return reassignmentErr
	}

	sub := newTestAssigningSubscriber(t, receiver.onMessage, onAssignment, subscription)
	if gotErr := sub.WaitStarted(); gotErr != nil {
		t.Errorf("Start() got err: (%v)", gotErr)
	}

	// Used to control order of execution to ensure the test is deterministic.
	subBarrier.Release()
	cmtBarrier.Release()
	returnReassignmentErr.SetDone()

	if gotErr := sub.WaitStopped(); !test.ErrorEqual(gotErr, reassignmentErr) {
		t.Errorf("WaitStopped() got err: (%v), want err: (%v)", gotErr, reassignmentErr)
	}
}

func TestNewSubscriberValidatesSettings(t *testing.T) {
	const subscription = "projects/123456/locations/us-central1-b/subscriptions/my-sub"
	const region = "us-central1"
	receiver := newTestMessageReceiver(t)

	settings := DefaultReceiveSettings
	settings.MaxOutstandingMessages = 0
	if _, err := NewSubscriber(context.Background(), settings, receiver.onMessage, noopReassignmentHandler, region, subscription); err == nil {
		t.Error("NewSubscriber() did not return error")
	}
}

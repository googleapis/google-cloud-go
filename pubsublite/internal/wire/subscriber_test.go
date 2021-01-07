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
	"context"
	"sort"
	"testing"
	"time"

	"cloud.google.com/go/internal/testutil"
	"cloud.google.com/go/pubsublite/internal/test"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/proto"

	pb "google.golang.org/genproto/googleapis/cloud/pubsublite/v1"
)

func testSubscriberSettings() ReceiveSettings {
	settings := testReceiveSettings()
	settings.MaxOutstandingMessages = 10
	settings.MaxOutstandingBytes = 1000
	return settings
}

// initFlowControlReq returns the first expected flow control request when
// testSubscriberSettings are used.
func initFlowControlReq() *pb.SubscribeRequest {
	return flowControlSubReq(flowControlTokens{Bytes: 1000, Messages: 10})
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

type ByMsgOffset []*pb.SequencedMessage

func (m ByMsgOffset) Len() int      { return len(m) }
func (m ByMsgOffset) Swap(i, j int) { m[i], m[j] = m[j], m[i] }
func (m ByMsgOffset) Less(i, j int) bool {
	return m[i].GetCursor().GetOffset() < m[j].GetCursor().GetOffset()
}

func (tr *testMessageReceiver) ValidateMsgs(want []*pb.SequencedMessage) {
	var got []*pb.SequencedMessage
	for count := 0; count < len(want); count++ {
		select {
		case <-time.After(serviceTestWaitTimeout):
			tr.t.Errorf("Received messages count: got %d, want %d", count, len(want))
		case received := <-tr.received:
			received.Ack.Ack()
			got = append(got, received.Msg)
		}
	}

	sort.Sort(ByMsgOffset(want))
	sort.Sort(ByMsgOffset(got))
	if !testutil.Equal(got, want) {
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

func TestMessageDeliveryQueue(t *testing.T) {
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

		receiver.VerifyNoMsgs()
	})
}

// testSubscribeStream wraps a subscribeStream for ease of testing.
type testSubscribeStream struct {
	Receiver *testMessageReceiver
	t        *testing.T
	sub      *subscribeStream
	serviceTestProxy
}

func newTestSubscribeStream(t *testing.T, subscription subscriptionPartition, settings ReceiveSettings, acks *ackTracker) *testSubscribeStream {
	ctx := context.Background()
	subClient, err := newSubscriberClient(ctx, "ignored", testClientOpts...)
	if err != nil {
		t.Fatal(err)
	}

	ts := &testSubscribeStream{
		Receiver: newTestMessageReceiver(t),
		t:        t,
	}
	ts.sub = newSubscribeStream(ctx, subClient, settings, ts.Receiver.onMessage, subscription, acks, true)
	ts.initAndStart(t, ts.sub, "Subscriber")
	return ts
}

// SendBatchFlowControl invokes the periodic background batch flow control. Note
// that the periodic task is disabled in tests.
func (ts *testSubscribeStream) SendBatchFlowControl() {
	ts.sub.sendBatchFlowControl()
}

func TestSubscribeStreamReconnect(t *testing.T) {
	subscription := subscriptionPartition{"projects/123456/locations/us-central1-b/subscriptions/my-sub", 0}
	acks := newAckTracker()
	msg1 := seqMsgWithOffsetAndSize(67, 200)
	msg2 := seqMsgWithOffsetAndSize(68, 100)
	permanentErr := status.Error(codes.FailedPrecondition, "permanent failure")

	verifiers := test.NewVerifiers(t)

	stream1 := test.NewRPCVerifier(t)
	stream1.Push(initSubReq(subscription), initSubResp(), nil)
	stream1.Push(initFlowControlReq(), msgSubResp(msg1), nil)
	stream1.Push(nil, nil, status.Error(codes.Unavailable, "server unavailable"))
	verifiers.AddSubscribeStream(subscription.Path, subscription.Partition, stream1)

	// When reconnected, the subscribeStream should seek to msg2 and have
	// subtracted flow control tokens.
	stream2 := test.NewRPCVerifier(t)
	stream2.Push(initSubReq(subscription), initSubResp(), nil)
	stream2.Push(seekReq(68), seekResp(68), nil)
	stream2.Push(flowControlSubReq(flowControlTokens{Bytes: 800, Messages: 9}), msgSubResp(msg2), nil)
	// Subscriber should terminate on permanent error.
	stream2.Push(nil, nil, permanentErr)
	verifiers.AddSubscribeStream(subscription.Path, subscription.Partition, stream2)

	mockServer.OnTestStart(verifiers)
	defer mockServer.OnTestEnd()

	sub := newTestSubscribeStream(t, subscription, testSubscriberSettings(), acks)
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
	acks := newAckTracker()
	msg1 := seqMsgWithOffsetAndSize(67, 200)
	msg2 := seqMsgWithOffsetAndSize(68, 100)
	serverErr := status.Error(codes.InvalidArgument, "verifies flow control received")

	verifiers := test.NewVerifiers(t)
	stream := test.NewRPCVerifier(t)
	stream.Push(initSubReq(subscription), initSubResp(), nil)
	stream.Push(initFlowControlReq(), msgSubResp(msg1, msg2), nil)
	// Batch flow control request expected.
	stream.Push(flowControlSubReq(flowControlTokens{Bytes: 300, Messages: 2}), nil, serverErr)
	verifiers.AddSubscribeStream(subscription.Path, subscription.Partition, stream)

	mockServer.OnTestStart(verifiers)
	defer mockServer.OnTestEnd()

	sub := newTestSubscribeStream(t, subscription, testSubscriberSettings(), acks)
	if gotErr := sub.StartError(); gotErr != nil {
		t.Errorf("Start() got err: (%v)", gotErr)
	}
	sub.Receiver.ValidateMsg(msg1)
	sub.Receiver.ValidateMsg(msg2)
	sub.sub.onAckAsync(msg1.SizeBytes)
	sub.sub.onAckAsync(msg2.SizeBytes)
	sub.sub.sendBatchFlowControl()
	if gotErr := sub.FinalError(); !test.ErrorEqual(gotErr, serverErr) {
		t.Errorf("Final err: (%v), want: (%v)", gotErr, serverErr)
	}
}

func TestSubscribeStreamExpediteFlowControl(t *testing.T) {
	subscription := subscriptionPartition{"projects/123456/locations/us-central1-b/subscriptions/my-sub", 0}
	acks := newAckTracker()
	msg1 := seqMsgWithOffsetAndSize(67, 250)
	// MaxOutstandingBytes = 1000, so msg2 pushes the pending flow control bytes
	// over the expediteBatchRequestRatio=50% threshold in flowControlBatcher.
	msg2 := seqMsgWithOffsetAndSize(68, 251)
	serverErr := status.Error(codes.InvalidArgument, "verifies flow control received")

	verifiers := test.NewVerifiers(t)
	stream := test.NewRPCVerifier(t)
	stream.Push(initSubReq(subscription), initSubResp(), nil)
	stream.Push(initFlowControlReq(), msgSubResp(msg1, msg2), nil)
	// Batch flow control request expected.
	stream.Push(flowControlSubReq(flowControlTokens{Bytes: 501, Messages: 2}), nil, serverErr)
	verifiers.AddSubscribeStream(subscription.Path, subscription.Partition, stream)

	mockServer.OnTestStart(verifiers)
	defer mockServer.OnTestEnd()

	sub := newTestSubscribeStream(t, subscription, testSubscriberSettings(), acks)
	if gotErr := sub.StartError(); gotErr != nil {
		t.Errorf("Start() got err: (%v)", gotErr)
	}
	sub.Receiver.ValidateMsg(msg1)
	sub.Receiver.ValidateMsg(msg2)
	sub.sub.onAckAsync(msg1.SizeBytes)
	sub.sub.onAckAsync(msg2.SizeBytes)
	// Note: the ack for msg2 automatically triggers sending the flow control.
	if gotErr := sub.FinalError(); !test.ErrorEqual(gotErr, serverErr) {
		t.Errorf("Final err: (%v), want: (%v)", gotErr, serverErr)
	}
}

func TestSubscribeStreamInvalidInitialResponse(t *testing.T) {
	subscription := subscriptionPartition{"projects/123456/locations/us-central1-b/subscriptions/my-sub", 0}
	acks := newAckTracker()

	verifiers := test.NewVerifiers(t)
	stream := test.NewRPCVerifier(t)
	stream.Push(initSubReq(subscription), seekResp(0), nil) // Seek instead of init response
	verifiers.AddSubscribeStream(subscription.Path, subscription.Partition, stream)

	mockServer.OnTestStart(verifiers)
	defer mockServer.OnTestEnd()

	sub := newTestSubscribeStream(t, subscription, testSubscriberSettings(), acks)
	if gotErr, wantErr := sub.StartError(), errInvalidInitialSubscribeResponse; !test.ErrorEqual(gotErr, wantErr) {
		t.Errorf("Start got err: (%v), want: (%v)", gotErr, wantErr)
	}
}

func TestSubscribeStreamDuplicateInitialResponse(t *testing.T) {
	subscription := subscriptionPartition{"projects/123456/locations/us-central1-b/subscriptions/my-sub", 0}
	acks := newAckTracker()

	verifiers := test.NewVerifiers(t)
	stream := test.NewRPCVerifier(t)
	stream.Push(initSubReq(subscription), initSubResp(), nil)
	stream.Push(initFlowControlReq(), initSubResp(), nil) // Second initial response
	verifiers.AddSubscribeStream(subscription.Path, subscription.Partition, stream)

	mockServer.OnTestStart(verifiers)
	defer mockServer.OnTestEnd()

	sub := newTestSubscribeStream(t, subscription, testSubscriberSettings(), acks)
	if gotErr, wantErr := sub.FinalError(), errInvalidSubscribeResponse; !test.ErrorEqual(gotErr, wantErr) {
		t.Errorf("Final err: (%v), want: (%v)", gotErr, wantErr)
	}
}

func TestSubscribeStreamSpuriousSeekResponse(t *testing.T) {
	subscription := subscriptionPartition{"projects/123456/locations/us-central1-b/subscriptions/my-sub", 0}
	acks := newAckTracker()

	verifiers := test.NewVerifiers(t)
	stream := test.NewRPCVerifier(t)
	stream.Push(initSubReq(subscription), initSubResp(), nil)
	stream.Push(initFlowControlReq(), seekResp(1), nil) // Seek response with no seek request
	verifiers.AddSubscribeStream(subscription.Path, subscription.Partition, stream)

	mockServer.OnTestStart(verifiers)
	defer mockServer.OnTestEnd()

	sub := newTestSubscribeStream(t, subscription, testSubscriberSettings(), acks)
	if gotErr, wantErr := sub.FinalError(), errNoInFlightSeek; !test.ErrorEqual(gotErr, wantErr) {
		t.Errorf("Final err: (%v), want: (%v)", gotErr, wantErr)
	}
}

func TestSubscribeStreamNoMessages(t *testing.T) {
	subscription := subscriptionPartition{"projects/123456/locations/us-central1-b/subscriptions/my-sub", 0}
	acks := newAckTracker()

	verifiers := test.NewVerifiers(t)
	stream := test.NewRPCVerifier(t)
	stream.Push(initSubReq(subscription), initSubResp(), nil)
	stream.Push(initFlowControlReq(), msgSubResp(), nil) // No messages in response
	verifiers.AddSubscribeStream(subscription.Path, subscription.Partition, stream)

	mockServer.OnTestStart(verifiers)
	defer mockServer.OnTestEnd()

	sub := newTestSubscribeStream(t, subscription, testSubscriberSettings(), acks)
	if gotErr, wantErr := sub.FinalError(), errServerNoMessages; !test.ErrorEqual(gotErr, wantErr) {
		t.Errorf("Final err: (%v), want: (%v)", gotErr, wantErr)
	}
}

func TestSubscribeStreamMessagesOutOfOrder(t *testing.T) {
	subscription := subscriptionPartition{"projects/123456/locations/us-central1-b/subscriptions/my-sub", 0}
	acks := newAckTracker()
	msg1 := seqMsgWithOffsetAndSize(56, 100)
	msg2 := seqMsgWithOffsetAndSize(55, 100) // Offset before msg1

	verifiers := test.NewVerifiers(t)
	stream := test.NewRPCVerifier(t)
	stream.Push(initSubReq(subscription), initSubResp(), nil)
	stream.Push(initFlowControlReq(), msgSubResp(msg1), nil)
	stream.Push(nil, msgSubResp(msg2), nil)
	verifiers.AddSubscribeStream(subscription.Path, subscription.Partition, stream)

	mockServer.OnTestStart(verifiers)
	defer mockServer.OnTestEnd()

	sub := newTestSubscribeStream(t, subscription, testSubscriberSettings(), acks)
	sub.Receiver.ValidateMsg(msg1)
	if gotErr, msg := sub.FinalError(), "start offset = 55, expected >= 57"; !test.ErrorHasMsg(gotErr, msg) {
		t.Errorf("Final err: (%v), want msg: %q", gotErr, msg)
	}
}

func TestSubscribeStreamFlowControlOverflow(t *testing.T) {
	subscription := subscriptionPartition{"projects/123456/locations/us-central1-b/subscriptions/my-sub", 0}
	acks := newAckTracker()
	msg1 := seqMsgWithOffsetAndSize(56, 900)
	msg2 := seqMsgWithOffsetAndSize(57, 101) // Overflows ReceiveSettings.MaxOutstandingBytes = 1000

	verifiers := test.NewVerifiers(t)
	stream := test.NewRPCVerifier(t)
	stream.Push(initSubReq(subscription), initSubResp(), nil)
	stream.Push(initFlowControlReq(), msgSubResp(msg1), nil)
	stream.Push(nil, msgSubResp(msg2), nil)
	verifiers.AddSubscribeStream(subscription.Path, subscription.Partition, stream)

	mockServer.OnTestStart(verifiers)
	defer mockServer.OnTestEnd()

	sub := newTestSubscribeStream(t, subscription, testSubscriberSettings(), acks)
	sub.Receiver.ValidateMsg(msg1)
	if gotErr, wantErr := sub.FinalError(), errTokenCounterBytesNegative; !test.ErrorEqual(gotErr, wantErr) {
		t.Errorf("Final err: (%v), want: (%v)", gotErr, wantErr)
	}
}

func newTestSinglePartitionSubscriber(t *testing.T, receiverFunc MessageReceiverFunc, subscription subscriptionPartition) *singlePartitionSubscriber {
	ctx := context.Background()
	subClient, err := newSubscriberClient(ctx, "ignored", testClientOpts...)
	if err != nil {
		t.Fatal(err)
	}
	cursorClient, err := newCursorClient(ctx, "ignored", testClientOpts...)
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
	return sub
}

func TestSinglePartitionSubscriberStartStop(t *testing.T) {
	subscription := subscriptionPartition{"projects/123456/locations/us-central1-b/subscriptions/my-sub", 0}
	receiver := newTestMessageReceiver(t)

	verifiers := test.NewVerifiers(t)

	// Verifies the behavior of the subscribeStream and committer when they are
	// stopped before any messages are received.
	subStream := test.NewRPCVerifier(t)
	subStream.Push(initSubReq(subscription), initSubResp(), nil)
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
	subStream.Push(initSubReq(subscription), initSubResp(), nil)
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
	subStream1.Push(initSubReq(subscription), initSubResp(), nil)
	subStream1.Push(initFlowControlReq(), msgSubResp(msg1), nil)
	subStream1.Push(nil, msgSubResp(msg2), nil)
	subStream1.Push(nil, nil, retryableErr)
	verifiers.AddSubscribeStream(subscription.Path, subscription.Partition, subStream1)

	// When reconnected, the subscribeStream should seek to msg3 and have
	// subtracted flow control tokens for msg1 and msg2.
	subStream2 := test.NewRPCVerifier(t)
	subStream2.Push(initSubReq(subscription), initSubResp(), nil)
	subStream2.Push(seekReq(3), nil, nil)
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
	subStream.Push(initSubReq(subscription), initSubResp(), nil)
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

func newTestMultiPartitionSubscriber(t *testing.T, receiverFunc MessageReceiverFunc, subscriptionPath string, partitions []int) *multiPartitionSubscriber {
	ctx := context.Background()
	subClient, err := newSubscriberClient(ctx, "ignored", testClientOpts...)
	if err != nil {
		t.Fatal(err)
	}
	cursorClient, err := newCursorClient(ctx, "ignored", testClientOpts...)
	if err != nil {
		t.Fatal(err)
	}

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
	sub := newMultiPartitionSubscriber(f)
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
	subStream1.Push(initSubReq(subscriptionPartition{Path: subscription, Partition: 1}), initSubResp(), nil)
	subStream1.Push(initFlowControlReq(), msgSubResp(msg1), nil)
	subStream1.Push(nil, msgSubResp(msg2), nil)
	verifiers.AddSubscribeStream(subscription, 1, subStream1)

	cmtStream1 := test.NewRPCVerifier(t)
	cmtStream1.Push(initCommitReq(subscriptionPartition{Path: subscription, Partition: 1}), initCommitResp(), nil)
	cmtStream1.Push(commitReq(24), commitResp(1), nil)
	verifiers.AddCommitStream(subscription, 1, cmtStream1)

	// Partition 2
	subStream2 := test.NewRPCVerifier(t)
	subStream2.Push(initSubReq(subscriptionPartition{Path: subscription, Partition: 2}), initSubResp(), nil)
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
	if gotErr := sub.WaitStarted(); gotErr != nil {
		t.Errorf("Start() got err: (%v)", gotErr)
	}
	receiver.ValidateMsgs([]*pb.SequencedMessage{msg1, msg2, msg3, msg4})
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
	subStream1.Push(initSubReq(subscriptionPartition{Path: subscription, Partition: 1}), initSubResp(), nil)
	subStream1.Push(initFlowControlReq(), msgSubResp(msg1), nil)
	msg2Barrier := subStream1.PushWithBarrier(nil, msgSubResp(msg2), nil)
	verifiers.AddSubscribeStream(subscription, 1, subStream1)

	cmtStream1 := test.NewRPCVerifier(t)
	cmtStream1.Push(initCommitReq(subscriptionPartition{Path: subscription, Partition: 1}), initCommitResp(), nil)
	cmtStream1.Push(commitReq(23), commitResp(1), nil)
	verifiers.AddCommitStream(subscription, 1, cmtStream1)

	// Partition 2
	subStream2 := test.NewRPCVerifier(t)
	subStream2.Push(initSubReq(subscriptionPartition{Path: subscription, Partition: 2}), initSubResp(), nil)
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
	receiver.ValidateMsgs([]*pb.SequencedMessage{msg1, msg3})
	errorBarrier.Release() // Release server error now to ensure test is deterministic
	if gotErr := sub.WaitStopped(); !test.ErrorEqual(gotErr, serverErr) {
		t.Errorf("Final error got: (%v), want: (%v)", gotErr, serverErr)
	}

	// Verify msg2 never received as subscriber has terminated.
	msg2Barrier.Release()
	receiver.VerifyNoMsgs()
}

func (as *assigningSubscriber) Partitions() []int {
	as.mu.Lock()
	defer as.mu.Unlock()

	var partitions []int
	for p := range as.subscribers {
		partitions = append(partitions, p)
	}
	sort.Ints(partitions)
	return partitions
}

func (as *assigningSubscriber) FlushCommits() {
	as.mu.Lock()
	defer as.mu.Unlock()

	for _, sub := range as.subscribers {
		sub.committer.commitOffsetToStream()
	}
}

func newTestAssigningSubscriber(t *testing.T, receiverFunc MessageReceiverFunc, subscriptionPath string) *assigningSubscriber {
	ctx := context.Background()
	subClient, err := newSubscriberClient(ctx, "ignored", testClientOpts...)
	if err != nil {
		t.Fatal(err)
	}
	cursorClient, err := newCursorClient(ctx, "ignored", testClientOpts...)
	if err != nil {
		t.Fatal(err)
	}
	assignmentClient, err := newPartitionAssignmentClient(ctx, "ignored", testClientOpts...)
	if err != nil {
		t.Fatal(err)
	}

	f := &singlePartitionSubscriberFactory{
		ctx:              ctx,
		subClient:        subClient,
		cursorClient:     cursorClient,
		settings:         testSubscriberSettings(),
		subscriptionPath: subscriptionPath,
		receiver:         receiverFunc,
		disableTasks:     true, // Background tasks disabled to control event order
	}
	sub, err := newAssigningSubscriber(assignmentClient, fakeGenerateUUID, f)
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
	assignmentBarrier := asnStream.PushWithBarrier(assignmentAckReq(), assignmentResp([]int64{3, 8}), nil)
	asnStream.Push(assignmentAckReq(), nil, nil)
	verifiers.AddAssignmentStream(subscription, asnStream)

	// Partition 3
	subStream3 := test.NewRPCVerifier(t)
	subStream3.Push(initSubReq(subscriptionPartition{Path: subscription, Partition: 3}), initSubResp(), nil)
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
	subStream6.Push(initSubReq(subscriptionPartition{Path: subscription, Partition: 6}), initSubResp(), nil)
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
	subStream8.Push(initSubReq(subscriptionPartition{Path: subscription, Partition: 8}), initSubResp(), nil)
	subStream8.Push(initFlowControlReq(), msgSubResp(msg5), nil)
	verifiers.AddSubscribeStream(subscription, 8, subStream8)

	cmtStream8 := test.NewRPCVerifier(t)
	cmtStream8.Push(initCommitReq(subscriptionPartition{Path: subscription, Partition: 8}), initCommitResp(), nil)
	cmtStream8.Push(commitReq(89), commitResp(1), nil)
	verifiers.AddCommitStream(subscription, 8, cmtStream8)

	mockServer.OnTestStart(verifiers)
	defer mockServer.OnTestEnd()

	sub := newTestAssigningSubscriber(t, receiver.onMessage, subscription)
	if gotErr := sub.WaitStarted(); gotErr != nil {
		t.Errorf("Start() got err: (%v)", gotErr)
	}

	// Partition assignments are initially {3, 6}.
	receiver.ValidateMsgs([]*pb.SequencedMessage{msg1, msg3})
	if got, want := sub.Partitions(), []int{3, 6}; !testutil.Equal(got, want) {
		t.Errorf("subscriber partitions: got %d, want %d", got, want)
	}

	// Partition assignments will now be {3, 8}.
	assignmentBarrier.Release()
	receiver.ValidateMsgs([]*pb.SequencedMessage{msg5})
	if got, want := sub.Partitions(), []int{3, 8}; !testutil.Equal(got, want) {
		t.Errorf("subscriber partitions: got %d, want %d", got, want)
	}

	// msg2 is from partition 3 and should be received. msg4 is from partition 6
	// (removed) and should be discarded.
	sub.FlushCommits()
	msg2Barrier.Release()
	msg4Barrier.Release()
	receiver.ValidateMsgs([]*pb.SequencedMessage{msg2})

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
	subStream1.Push(initSubReq(subscriptionPartition{Path: subscription, Partition: 1}), initSubResp(), nil)
	subStream1.Push(initFlowControlReq(), msgSubResp(msg1), nil)
	verifiers.AddSubscribeStream(subscription, 1, subStream1)

	cmtStream1 := test.NewRPCVerifier(t)
	cmtStream1.Push(initCommitReq(subscriptionPartition{Path: subscription, Partition: 1}), initCommitResp(), nil)
	cmtStream1.Push(commitReq(12), commitResp(1), nil)
	verifiers.AddCommitStream(subscription, 1, cmtStream1)

	// Partition 2
	subStream2 := test.NewRPCVerifier(t)
	subStream2.Push(initSubReq(subscriptionPartition{Path: subscription, Partition: 2}), initSubResp(), nil)
	subStream2.Push(initFlowControlReq(), msgSubResp(msg2), nil)
	verifiers.AddSubscribeStream(subscription, 2, subStream2)

	cmtStream2 := test.NewRPCVerifier(t)
	cmtStream2.Push(initCommitReq(subscriptionPartition{Path: subscription, Partition: 2}), initCommitResp(), nil)
	cmtStream2.Push(commitReq(23), commitResp(1), nil)
	verifiers.AddCommitStream(subscription, 2, cmtStream2)

	mockServer.OnTestStart(verifiers)
	defer mockServer.OnTestEnd()

	sub := newTestAssigningSubscriber(t, receiver.onMessage, subscription)
	if gotErr := sub.WaitStarted(); gotErr != nil {
		t.Errorf("Start() got err: (%v)", gotErr)
	}
	receiver.ValidateMsgs([]*pb.SequencedMessage{msg1, msg2})

	// Permanent assignment stream error should terminate subscriber. Commits are
	// still flushed.
	errBarrier.Release()
	if gotErr := sub.WaitStopped(); !test.ErrorEqual(gotErr, serverErr) {
		t.Errorf("Final error got: (%v), want: (%v)", gotErr, serverErr)
	}
}

func TestNewSubscriberCreatesCorrectImpl(t *testing.T) {
	const subscription = "projects/123456/locations/us-central1-b/subscriptions/my-sub"
	const region = "us-central1"
	receiver := newTestMessageReceiver(t)

	sub, err := NewSubscriber(context.Background(), DefaultReceiveSettings, receiver.onMessage, region, subscription)
	if err != nil {
		t.Errorf("NewSubscriber() got error: %v", err)
	} else if _, ok := sub.(*assigningSubscriber); !ok {
		t.Error("NewSubscriber() did not return a assigningSubscriber")
	}

	settings := DefaultReceiveSettings
	settings.Partitions = []int{1, 2, 3}
	sub, err = NewSubscriber(context.Background(), settings, receiver.onMessage, region, subscription)
	if err != nil {
		t.Errorf("NewSubscriber() got error: %v", err)
	} else if _, ok := sub.(*multiPartitionSubscriber); !ok {
		t.Error("NewSubscriber() did not return a multiPartitionSubscriber")
	}
}

func TestNewSubscriberValidatesSettings(t *testing.T) {
	const subscription = "projects/123456/locations/us-central1-b/subscriptions/my-sub"
	const region = "us-central1"
	receiver := newTestMessageReceiver(t)

	settings := DefaultReceiveSettings
	settings.MaxOutstandingMessages = 0
	if _, err := NewSubscriber(context.Background(), settings, receiver.onMessage, region, subscription); err == nil {
		t.Error("NewSubscriber() did not return error")
	}
}

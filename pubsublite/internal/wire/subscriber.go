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
	"errors"
	"reflect"
	"sync"
	"time"

	"github.com/google/uuid"
	"google.golang.org/api/option"
	"google.golang.org/grpc"

	vkit "cloud.google.com/go/pubsublite/apiv1"
	pb "cloud.google.com/go/pubsublite/apiv1/pubsublitepb"
)

var (
	errServerNoMessages                = errors.New("pubsublite: server delivered no messages")
	errInvalidInitialSubscribeResponse = errors.New("pubsublite: first response from server was not an initial response for subscribe")
	errInvalidSubscribeResponse        = errors.New("pubsublite: received unexpected subscribe response from server")
)

// ReceivedMessage stores a received Pub/Sub message and AckConsumer for
// acknowledging the message.
type ReceivedMessage struct {
	Msg       *pb.SequencedMessage
	Ack       AckConsumer
	Partition int
}

// MessageReceiverFunc receives a Pub/Sub message from a topic partition.
type MessageReceiverFunc func(*ReceivedMessage)

// messageDeliveryQueue delivers received messages to the client-provided
// MessageReceiverFunc sequentially. It is only accessed by the subscribeStream.
type messageDeliveryQueue struct {
	bufferSize int
	acks       *ackTracker
	receiver   MessageReceiverFunc
	messagesC  chan *ReceivedMessage
	stopC      chan struct{}
	active     sync.WaitGroup
}

func newMessageDeliveryQueue(acks *ackTracker, receiver MessageReceiverFunc, bufferSize int) *messageDeliveryQueue {
	return &messageDeliveryQueue{
		bufferSize: bufferSize,
		acks:       acks,
		receiver:   receiver,
	}
}

// Start the message delivery, if not already started.
func (mq *messageDeliveryQueue) Start() {
	if mq.stopC != nil {
		return
	}

	mq.stopC = make(chan struct{})
	mq.messagesC = make(chan *ReceivedMessage, mq.bufferSize)
	mq.active.Add(1)
	go mq.deliverMessages(mq.messagesC, mq.stopC)
}

// Stop message delivery and discard undelivered messages.
func (mq *messageDeliveryQueue) Stop() {
	if mq.stopC == nil {
		return
	}

	close(mq.stopC)
	mq.stopC = nil
	mq.messagesC = nil
}

// Wait until the message delivery goroutine has terminated.
func (mq *messageDeliveryQueue) Wait() {
	mq.active.Wait()
}

func (mq *messageDeliveryQueue) Add(msg *ReceivedMessage) {
	if mq.messagesC != nil {
		mq.messagesC <- msg
	}
}

func (mq *messageDeliveryQueue) deliverMessages(messagesC chan *ReceivedMessage, stopC chan struct{}) {
	// Notify the wait group that the goroutine has terminated upon exit.
	defer mq.active.Done()

	for {
		// stopC has higher priority.
		select {
		case <-stopC:
			return // Ends the goroutine.
		default:
		}

		select {
		case <-stopC:
			return // Ends the goroutine.
		case msg := <-messagesC:
			// Register outstanding acks, which are primarily handled by the
			// `committer`.
			mq.acks.Push(msg.Ack.(*ackConsumer))
			mq.receiver(msg)
		}
	}
}

// The frequency of sending batch flow control requests.
const batchFlowControlPeriod = 100 * time.Millisecond

// Handles subscriber reset actions that are external to the subscribeStream
// (e.g. wait for the committer to flush commits).
type subscriberResetHandler func() error

// subscribeStream directly wraps the subscribe client stream. It passes
// messages to the message receiver and manages flow control. Flow control
// tokens are batched and sent to the stream via a periodic background task,
// although it can be expedited if the user is rapidly acking messages.
//
// Client-initiated seek unsupported.
type subscribeStream struct {
	// Immutable after creation.
	subClient    *vkit.SubscriberClient
	settings     ReceiveSettings
	subscription subscriptionPartition
	handleReset  subscriberResetHandler
	metadata     pubsubMetadata

	// Fields below must be guarded with mu.
	messageQueue           *messageDeliveryQueue
	stream                 *retryableStream
	offsetTracker          subscriberOffsetTracker
	flowControl            flowControlBatcher
	pollFlowControl        *periodicTask
	enableBatchFlowControl bool

	abstractService
}

func newSubscribeStream(ctx context.Context, subClient *vkit.SubscriberClient, settings ReceiveSettings,
	receiver MessageReceiverFunc, subscription subscriptionPartition, acks *ackTracker,
	handleReset subscriberResetHandler, disableTasks bool) *subscribeStream {

	s := &subscribeStream{
		subClient:    subClient,
		settings:     settings,
		subscription: subscription,
		handleReset:  handleReset,
		messageQueue: newMessageDeliveryQueue(acks, receiver, settings.MaxOutstandingMessages),
		metadata:     newPubsubMetadata(),
	}
	s.stream = newRetryableStream(ctx, s, settings.Timeout, streamIdleTimeout(settings.Timeout), reflect.TypeOf(pb.SubscribeResponse{}))
	s.metadata.AddSubscriptionRoutingMetadata(s.subscription)
	s.metadata.AddClientInfo(settings.Framework)

	backgroundTask := s.sendBatchFlowControl
	if disableTasks {
		backgroundTask = func() {}
	}
	s.pollFlowControl = newPeriodicTask(batchFlowControlPeriod, backgroundTask)
	return s
}

// Start establishes a subscribe stream connection and initializes flow control
// tokens from ReceiveSettings.
func (s *subscribeStream) Start() {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.unsafeUpdateStatus(serviceStarting, nil) {
		s.stream.Start()
		s.pollFlowControl.Start()
		s.messageQueue.Start()

		s.flowControl.Reset(flowControlTokens{
			Bytes:    int64(s.settings.MaxOutstandingBytes),
			Messages: int64(s.settings.MaxOutstandingMessages),
		})
	}
}

// Stop immediately terminates the subscribe stream.
func (s *subscribeStream) Stop() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.unsafeInitiateShutdown(serviceTerminating, nil)
}

func (s *subscribeStream) newStream(ctx context.Context) (grpc.ClientStream, error) {
	return s.subClient.Subscribe(s.metadata.AddToContext(ctx))
}

func (s *subscribeStream) initialRequest() (interface{}, initialResponseRequired) {
	s.mu.Lock()
	defer s.mu.Unlock()
	initReq := &pb.SubscribeRequest{
		Request: &pb.SubscribeRequest_Initial{
			Initial: &pb.InitialSubscribeRequest{
				Subscription:    s.subscription.Path,
				Partition:       int64(s.subscription.Partition),
				InitialLocation: s.offsetTracker.RequestForRestart(),
			},
		},
	}
	return initReq, initialResponseRequired(true)
}

func (s *subscribeStream) validateInitialResponse(response interface{}) error {
	subscribeResponse, _ := response.(*pb.SubscribeResponse)
	if subscribeResponse.GetInitial() == nil {
		return errInvalidInitialSubscribeResponse
	}
	return nil
}

func (s *subscribeStream) onStreamStatusChange(status streamStatus) {
	s.mu.Lock()
	defer s.mu.Unlock()

	switch status {
	case streamConnected:
		s.unsafeUpdateStatus(serviceActive, nil)

		// Reinitialize the flow control tokens when a new subscribe stream instance
		// is connected.
		s.unsafeSendFlowControl(s.flowControl.RequestForRestart())
		s.enableBatchFlowControl = true
		s.pollFlowControl.Start()

	case streamReconnecting:
		// Ensure no batch flow control tokens are sent until the RequestForRestart
		// is sent above when a new subscribe stream is initialized.
		s.enableBatchFlowControl = false
		s.pollFlowControl.Stop()

	case streamResetState:
		// Handle out-of-band seek notifications from the server. Committer and
		// subscriber state are reset.

		s.messageQueue.Stop()

		// Wait for all message receiver callbacks to finish and the committer to
		// flush pending commits and reset its state. Release the mutex while
		// waiting.
		s.mu.Unlock()
		s.messageQueue.Wait()
		err := s.handleReset()
		s.mu.Lock()

		if err != nil {
			s.unsafeInitiateShutdown(serviceTerminating, nil)
			return
		}
		s.messageQueue.Start()
		s.offsetTracker.Reset()
		s.flowControl.Reset(flowControlTokens{
			Bytes:    int64(s.settings.MaxOutstandingBytes),
			Messages: int64(s.settings.MaxOutstandingMessages),
		})

	case streamTerminated:
		s.unsafeInitiateShutdown(serviceTerminated, s.stream.Error())
	}
}

func (s *subscribeStream) onResponse(response interface{}) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.status >= serviceTerminating {
		return
	}

	var err error
	subscribeResponse, _ := response.(*pb.SubscribeResponse)
	switch {
	case subscribeResponse.GetMessages() != nil:
		err = s.unsafeOnMessageResponse(subscribeResponse.GetMessages())
	default:
		err = errInvalidSubscribeResponse
	}
	if err != nil {
		s.unsafeInitiateShutdown(serviceTerminated, err)
	}
}

func (s *subscribeStream) unsafeOnMessageResponse(response *pb.MessageResponse) error {
	if len(response.Messages) == 0 {
		return errServerNoMessages
	}
	if err := s.offsetTracker.OnMessages(response.Messages); err != nil {
		return err
	}
	if err := s.flowControl.OnMessages(response.Messages); err != nil {
		return err
	}

	for _, msg := range response.Messages {
		ack := newAckConsumer(msg.GetCursor().GetOffset(), msg.GetSizeBytes(), s.onAck)
		s.messageQueue.Add(&ReceivedMessage{Msg: msg, Ack: ack, Partition: s.subscription.Partition})
	}
	return nil
}

func (s *subscribeStream) onAck(ac *ackConsumer) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.status == serviceActive {
		s.unsafeAllowFlow(flowControlTokens{Bytes: ac.MsgBytes, Messages: 1})
	}
}

// sendBatchFlowControl is called by the periodic background task.
func (s *subscribeStream) sendBatchFlowControl() {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.enableBatchFlowControl {
		s.unsafeSendFlowControl(s.flowControl.ReleasePendingRequest())
	}
}

func (s *subscribeStream) unsafeAllowFlow(allow flowControlTokens) {
	s.flowControl.OnClientFlow(allow)
	if s.flowControl.ShouldExpediteBatchRequest() && s.enableBatchFlowControl {
		s.unsafeSendFlowControl(s.flowControl.ReleasePendingRequest())
	}
}

func (s *subscribeStream) unsafeSendFlowControl(req *pb.FlowControlRequest) {
	if req == nil {
		return
	}

	// Note: If Send() returns false, the stream will be reconnected and
	// flowControlBatcher.RequestForRestart() will be sent when the stream
	// reconnects. So its return value is ignored.
	s.stream.Send(&pb.SubscribeRequest{
		Request: &pb.SubscribeRequest_FlowControl{FlowControl: req},
	})
}

func (s *subscribeStream) unsafeInitiateShutdown(targetStatus serviceStatus, err error) {
	if !s.unsafeUpdateStatus(targetStatus, wrapError("subscriber", s.subscription.String(), err)) {
		return
	}

	// No data to send. Immediately terminate the stream.
	s.messageQueue.Stop()
	s.pollFlowControl.Stop()
	s.stream.Stop()
}

// singlePartitionSubscriber receives messages from a single topic partition.
// It requires 2 child services:
// - subscribeStream to receive messages from the subscribe stream.
// - committer to commit cursor offsets to the streaming commit cursor stream.
type singlePartitionSubscriber struct {
	subscriber *subscribeStream
	committer  *committer

	compositeService
}

// Terminate shuts down the singlePartitionSubscriber without waiting for
// outstanding acks. Alternatively, Stop() will wait for outstanding acks.
func (s *singlePartitionSubscriber) Terminate() {
	s.subscriber.Stop()
	s.committer.Terminate()
}

type singlePartitionSubscriberFactory struct {
	ctx              context.Context
	subClient        *vkit.SubscriberClient
	cursorClient     *vkit.CursorClient
	settings         ReceiveSettings
	subscriptionPath string
	receiver         MessageReceiverFunc
	disableTasks     bool
}

func (f *singlePartitionSubscriberFactory) New(partition int) *singlePartitionSubscriber {
	subscription := subscriptionPartition{Path: f.subscriptionPath, Partition: partition}
	acks := newAckTracker()
	commit := newCommitter(f.ctx, f.cursorClient, f.settings, subscription, acks, f.disableTasks)
	sub := newSubscribeStream(f.ctx, f.subClient, f.settings, f.receiver, subscription, acks, commit.BlockingReset, f.disableTasks)
	ps := &singlePartitionSubscriber{
		subscriber: sub,
		committer:  commit,
	}
	ps.init()
	ps.unsafeAddServices(sub, commit)
	return ps
}

// multiPartitionSubscriber receives messages from a fixed set of topic
// partitions.
type multiPartitionSubscriber struct {
	// Immutable after creation.
	subscribers map[int]*singlePartitionSubscriber

	compositeService
}

func newMultiPartitionSubscriber(allClients apiClients, subFactory *singlePartitionSubscriberFactory) *multiPartitionSubscriber {
	ms := &multiPartitionSubscriber{
		subscribers: make(map[int]*singlePartitionSubscriber),
	}
	ms.init()
	ms.toClose = allClients

	for _, partition := range subFactory.settings.Partitions {
		subscriber := subFactory.New(partition)
		ms.unsafeAddServices(subscriber)
		ms.subscribers[partition] = subscriber
	}
	return ms
}

// Terminate shuts down all singlePartitionSubscribers without waiting for
// outstanding acks. Alternatively, Stop() will wait for outstanding acks.
func (ms *multiPartitionSubscriber) Terminate() {
	ms.mu.Lock()
	defer ms.mu.Unlock()

	for _, sub := range ms.subscribers {
		sub.Terminate()
	}
}

// PartitionActive returns whether the partition is active.
func (ms *multiPartitionSubscriber) PartitionActive(partition int) bool {
	_, exists := ms.subscribers[partition]
	return exists
}

// ReassignmentHandlerFunc receives a partition assignment change.
type ReassignmentHandlerFunc func(before, after PartitionSet) error

// assigningSubscriber uses the Pub/Sub Lite partition assignment service to
// listen to its assigned partition numbers and dynamically add/remove
// singlePartitionSubscribers.
type assigningSubscriber struct {
	// Immutable after creation.
	reassignmentHandler ReassignmentHandlerFunc
	subFactory          *singlePartitionSubscriberFactory
	assigner            *assigner

	// Fields below must be guarded with mu.
	// Subscribers keyed by partition number. Updated as assignments change.
	subscribers map[int]*singlePartitionSubscriber

	compositeService
}

func newAssigningSubscriber(allClients apiClients, assignmentClient *vkit.PartitionAssignmentClient, reassignmentHandler ReassignmentHandlerFunc,
	genUUID generateUUIDFunc, subFactory *singlePartitionSubscriberFactory) (*assigningSubscriber, error) {
	as := &assigningSubscriber{
		reassignmentHandler: reassignmentHandler,
		subFactory:          subFactory,
		subscribers:         make(map[int]*singlePartitionSubscriber),
	}
	as.init()
	as.toClose = allClients

	assigner, err := newAssigner(subFactory.ctx, assignmentClient, genUUID, subFactory.settings, subFactory.subscriptionPath, as.handleAssignment)
	if err != nil {
		return nil, err
	}
	as.assigner = assigner
	as.unsafeAddServices(assigner)
	return as, nil
}

func (as *assigningSubscriber) handleAssignment(nextPartitions PartitionSet) error {
	previousPartitions, removedSubscribers, err := as.doHandleAssignment(nextPartitions)
	if err != nil {
		return err
	}

	// Notify the user reassignment handler.
	if err := as.reassignmentHandler(previousPartitions, nextPartitions); err != nil {
		return err
	}

	// Wait for removed subscribers to completely stop (which waits for commit
	// acknowledgments from the server) before acking the assignment. This avoids
	// commits racing with the new assigned client.
	for _, subscriber := range removedSubscribers {
		subscriber.WaitStopped()
	}
	return nil
}

// Returns the previous set of partitions and removed subscribers.
func (as *assigningSubscriber) doHandleAssignment(nextPartitions PartitionSet) (PartitionSet, []*singlePartitionSubscriber, error) {
	as.mu.Lock()
	defer as.mu.Unlock()

	var previousPartitions []int
	for partition := range as.subscribers {
		previousPartitions = append(previousPartitions, partition)
	}

	// Handle new partitions.
	for _, partition := range nextPartitions.Ints() {
		if _, exists := as.subscribers[partition]; !exists {
			subscriber := as.subFactory.New(partition)
			if err := as.unsafeAddServices(subscriber); err != nil {
				// Occurs when the assigningSubscriber is stopping/stopped.
				return nil, nil, err
			}
			as.subscribers[partition] = subscriber
		}
	}

	// Handle removed partitions.
	var removedSubscribers []*singlePartitionSubscriber
	for partition, subscriber := range as.subscribers {
		if !nextPartitions.Contains(partition) {
			// Ignore unacked messages from this point on to avoid conflicting with
			// the commits of the new subscriber that will be assigned this partition.
			subscriber.Terminate()
			removedSubscribers = append(removedSubscribers, subscriber)

			as.unsafeRemoveService(subscriber)
			// Safe to delete map entry during range loop:
			// https://golang.org/ref/spec#For_statements
			delete(as.subscribers, partition)
		}
	}
	return NewPartitionSet(previousPartitions), removedSubscribers, nil
}

// Terminate shuts down all singlePartitionSubscribers without waiting for
// outstanding acks. Alternatively, Stop() will wait for outstanding acks.
func (as *assigningSubscriber) Terminate() {
	as.mu.Lock()
	defer as.mu.Unlock()

	for _, sub := range as.subscribers {
		sub.Terminate()
	}
}

// PartitionActive returns whether the partition is still active.
func (as *assigningSubscriber) PartitionActive(partition int) bool {
	as.mu.Lock()
	defer as.mu.Unlock()

	_, exists := as.subscribers[partition]
	return exists
}

// Subscriber is the client interface exported from this package for receiving
// messages.
type Subscriber interface {
	Start()
	WaitStarted() error
	Stop()
	WaitStopped() error
	Terminate()
	PartitionActive(int) bool
}

// NewSubscriber creates a new client for receiving messages.
func NewSubscriber(ctx context.Context, settings ReceiveSettings, receiver MessageReceiverFunc, reassignmentHandler ReassignmentHandlerFunc,
	region, subscriptionPath string, opts ...option.ClientOption) (Subscriber, error) {
	if err := ValidateRegion(region); err != nil {
		return nil, err
	}
	if err := validateReceiveSettings(settings); err != nil {
		return nil, err
	}

	var allClients apiClients
	subClient, err := newSubscriberClient(ctx, region, opts...)
	if err != nil {
		return nil, err
	}
	allClients = append(allClients, subClient)

	cursorClient, err := newCursorClient(ctx, region, opts...)
	if err != nil {
		allClients.Close()
		return nil, err
	}
	allClients = append(allClients, cursorClient)

	subFactory := &singlePartitionSubscriberFactory{
		ctx:              ctx,
		subClient:        subClient,
		cursorClient:     cursorClient,
		settings:         settings,
		subscriptionPath: subscriptionPath,
		receiver:         receiver,
	}

	if len(settings.Partitions) > 0 {
		return newMultiPartitionSubscriber(allClients, subFactory), nil
	}
	partitionClient, err := newPartitionAssignmentClient(ctx, region, opts...)
	if err != nil {
		allClients.Close()
		return nil, err
	}
	allClients = append(allClients, partitionClient)
	return newAssigningSubscriber(allClients, partitionClient, reassignmentHandler, uuid.NewRandom, subFactory)
}

// Copyright 2026 Google LLC
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

package wire

import (
	"context"
	"fmt"
	"strconv"
	"sync"
	"time"

	"github.com/IBM/sarama"
	"google.golang.org/protobuf/types/known/timestamppb"

	pb "cloud.google.com/go/pubsublite/apiv1/pubsublitepb"
)

// kafkaEventTimeHeader is the Kafka record header name used to carry the
// pubsublite event time through a GMK topic.
const kafkaEventTimeHeader = "pubsublite.event_time"

// kafkaSubscriberStatus tracks lifecycle state.
type kafkaSubscriberStatus int

const (
	kafkaSubscriberUninitialized kafkaSubscriberStatus = iota
	kafkaSubscriberActive
	kafkaSubscriberTerminating
	kafkaSubscriberTerminated
)

// KafkaSubscriber implements the Subscriber interface backed by a
// sarama.ConsumerGroup. Each Kafka record is converted into a synthetic
// *pb.SequencedMessage and delivered via the receiver callback. Ack() on a
// received message triggers a synchronous offset commit on the active session.
// Nacks are not supported natively; redelivery happens only after session
// timeout triggers a rebalance (matches the documented degraded behavior).
type KafkaSubscriber struct {
	topicName string
	groupID   string
	group     sarama.ConsumerGroup
	receiver  MessageReceiverFunc

	ctx    context.Context
	cancel context.CancelFunc

	waitStarted    chan struct{}
	waitTerminated chan struct{}

	mu        sync.Mutex
	status    kafkaSubscriberStatus
	err       error
	activeSet map[int32]struct{} // currently claimed partitions
}

// NewKafkaSubscriber creates a KafkaSubscriber wrapping the given
// ConsumerGroup. The group must be configured with auto-commit disabled.
func NewKafkaSubscriber(group sarama.ConsumerGroup, topicName, groupID string, receiver MessageReceiverFunc) *KafkaSubscriber {
	return &KafkaSubscriber{
		topicName:      topicName,
		groupID:        groupID,
		group:          group,
		receiver:       receiver,
		waitStarted:    make(chan struct{}),
		waitTerminated: make(chan struct{}),
		status:         kafkaSubscriberUninitialized,
		activeSet:      make(map[int32]struct{}),
	}
}

// Start begins consuming messages in a background goroutine.
func (ks *KafkaSubscriber) Start() {
	ks.mu.Lock()
	if ks.status != kafkaSubscriberUninitialized {
		ks.mu.Unlock()
		return
	}
	ks.status = kafkaSubscriberActive
	ks.ctx, ks.cancel = context.WithCancel(context.Background())
	ks.mu.Unlock()

	close(ks.waitStarted)

	go ks.runConsumeLoop()
	go ks.runErrorDrain()
}

// runConsumeLoop repeatedly calls Consume until the context is cancelled.
// Each Consume call covers one session; rebalances trigger a fresh iteration.
func (ks *KafkaSubscriber) runConsumeLoop() {
	handler := &kafkaConsumerHandler{sub: ks}

	for {
		if err := ks.group.Consume(ks.ctx, []string{ks.topicName}, handler); err != nil {
			if ks.ctx.Err() != nil {
				break
			}
			// Surface the first error and terminate.
			ks.setError(fmt.Errorf("gmk: consumer group error: %w", err))
			break
		}
		if ks.ctx.Err() != nil {
			break
		}
	}

	_ = ks.group.Close()
	ks.mu.Lock()
	ks.status = kafkaSubscriberTerminated
	ks.mu.Unlock()
	close(ks.waitTerminated)
}

// runErrorDrain reads the consumer group's error channel so it doesn't block.
func (ks *KafkaSubscriber) runErrorDrain() {
	for err := range ks.group.Errors() {
		ks.setError(fmt.Errorf("gmk: consumer error: %w", err))
	}
}

// WaitStarted blocks until the subscriber is started.
func (ks *KafkaSubscriber) WaitStarted() error {
	<-ks.waitStarted
	return ks.Error()
}

// Stop initiates a graceful shutdown. Pending messages already delivered to
// the receiver callback are not revoked; unacked messages are not committed.
func (ks *KafkaSubscriber) Stop() {
	ks.mu.Lock()
	if ks.status >= kafkaSubscriberTerminating {
		ks.mu.Unlock()
		return
	}
	ks.status = kafkaSubscriberTerminating
	cancel := ks.cancel
	ks.mu.Unlock()

	if cancel != nil {
		cancel()
	}
}

// Terminate immediately shuts down the subscriber.
func (ks *KafkaSubscriber) Terminate() {
	ks.Stop()
}

// WaitStopped blocks until the subscriber has fully stopped.
func (ks *KafkaSubscriber) WaitStopped() error {
	<-ks.waitTerminated
	return ks.Error()
}

// PartitionActive reports whether the given partition is currently claimed by
// this consumer. Used by pscompat to decide whether to honor a nack.
func (ks *KafkaSubscriber) PartitionActive(partition int) bool {
	ks.mu.Lock()
	defer ks.mu.Unlock()
	_, ok := ks.activeSet[int32(partition)]
	return ok
}

// Error returns the first error that caused the subscriber to fail, or nil.
func (ks *KafkaSubscriber) Error() error {
	ks.mu.Lock()
	defer ks.mu.Unlock()
	return ks.err
}

func (ks *KafkaSubscriber) setError(err error) {
	ks.mu.Lock()
	defer ks.mu.Unlock()
	if ks.err == nil {
		ks.err = err
	}
}

// kafkaConsumerHandler implements sarama.ConsumerGroupHandler, translating
// Kafka records into ReceivedMessage and delivering them via the subscriber's
// receiver callback.
type kafkaConsumerHandler struct {
	sub *KafkaSubscriber
}

func (h *kafkaConsumerHandler) Setup(session sarama.ConsumerGroupSession) error {
	h.sub.mu.Lock()
	defer h.sub.mu.Unlock()
	h.sub.activeSet = make(map[int32]struct{})
	for _, p := range session.Claims()[h.sub.topicName] {
		h.sub.activeSet[p] = struct{}{}
	}
	return nil
}

func (h *kafkaConsumerHandler) Cleanup(session sarama.ConsumerGroupSession) error {
	h.sub.mu.Lock()
	defer h.sub.mu.Unlock()
	h.sub.activeSet = make(map[int32]struct{})
	return nil
}

func (h *kafkaConsumerHandler) ConsumeClaim(session sarama.ConsumerGroupSession, claim sarama.ConsumerGroupClaim) error {
	for {
		select {
		case msg, ok := <-claim.Messages():
			if !ok {
				return nil
			}
			seqMsg := kafkaMessageToSequencedMessage(msg)
			ack := &kafkaAckConsumer{session: session, msg: msg}
			h.sub.receiver(&ReceivedMessage{
				Msg:       seqMsg,
				Ack:       ack,
				Partition: int(msg.Partition),
			})
		case <-session.Context().Done():
			return nil
		}
	}
}

// kafkaAckConsumer implements AckConsumer. Calling Ack() synchronously commits
// the message's offset using the active consumer group session. If the session
// has already been released (e.g. due to rebalance), MarkMessage/Commit become
// no-ops internally and the outcome is benign.
type kafkaAckConsumer struct {
	mu      sync.Mutex
	session sarama.ConsumerGroupSession
	msg     *sarama.ConsumerMessage
	acked   bool
}

func (k *kafkaAckConsumer) Ack() {
	k.mu.Lock()
	defer k.mu.Unlock()
	if k.acked {
		return
	}
	k.acked = true
	k.session.MarkMessage(k.msg, "")
	k.session.Commit()
}

// kafkaMessageToSequencedMessage builds a synthetic *pb.SequencedMessage from a
// Kafka ConsumerMessage so the pscompat layer can process it unchanged.
//
// Field mapping (inverse of pubsubMessageToProducerMessage):
//
//	Kafka Key              -> PubSubMessage.Key
//	Kafka Value            -> PubSubMessage.Data
//	Kafka Headers          -> PubSubMessage.Attributes (multi-valued)
//	Header pubsublite.event_time -> PubSubMessage.EventTime
//	Kafka Timestamp        -> SequencedMessage.PublishTime
//	Kafka Offset           -> SequencedMessage.Cursor.Offset
func kafkaMessageToSequencedMessage(msg *sarama.ConsumerMessage) *pb.SequencedMessage {
	pubsubMsg := &pb.PubSubMessage{
		Data:       msg.Value,
		Attributes: make(map[string]*pb.AttributeValues),
	}
	if len(msg.Key) > 0 {
		pubsubMsg.Key = msg.Key
	}

	for _, hdr := range msg.Headers {
		key := string(hdr.Key)
		if key == kafkaEventTimeHeader {
			if t, ok := parseEventTimeHeader(hdr.Value); ok {
				pubsubMsg.EventTime = t
			}
			continue
		}
		pubsubMsg.Attributes[key] = &pb.AttributeValues{
			Values: append(pubsubMsg.GetAttributes()[key].GetValues(), hdr.Value),
		}
	}

	return &pb.SequencedMessage{
		Cursor:      &pb.Cursor{Offset: msg.Offset},
		PublishTime: timestamppb.New(msg.Timestamp),
		Message:     pubsubMsg,
		SizeBytes:   int64(len(msg.Value) + len(msg.Key)),
	}
}

// parseEventTimeHeader accepts both the seconds-only format ("123") emitted by
// earlier versions of the publisher and the seconds.nanos format ("123.456789012").
func parseEventTimeHeader(raw []byte) (*timestamppb.Timestamp, bool) {
	s := string(raw)
	// seconds.nanos form
	for i := 0; i < len(s); i++ {
		if s[i] == '.' {
			secs, err := strconv.ParseInt(s[:i], 10, 64)
			if err != nil {
				return nil, false
			}
			nanos, err := strconv.ParseInt(s[i+1:], 10, 64)
			if err != nil {
				return nil, false
			}
			return timestamppb.New(time.Unix(secs, nanos)), true
		}
	}
	// seconds-only
	secs, err := strconv.ParseInt(s, 10, 64)
	if err != nil {
		return nil, false
	}
	return timestamppb.New(time.Unix(secs, 0)), true
}

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
	"fmt"
	"sync"
	"sync/atomic"

	"github.com/IBM/sarama"

	pb "cloud.google.com/go/pubsublite/apiv1/pubsublitepb"
)

// kafkaPublisherStatus tracks lifecycle state.
type kafkaPublisherStatus int

const (
	kafkaPublisherUninitialized kafkaPublisherStatus = iota
	kafkaPublisherActive
	kafkaPublisherTerminating
	kafkaPublisherTerminated
)

// KafkaPublisher implements the Publisher interface backed by a Sarama
// AsyncProducer. It converts PubSubMessage protos to Kafka ProducerMessages and
// dispatches results via PublishResultFunc callbacks.
//
// Concurrency model:
//   - status is an atomic so Publish/Start/Stop need no mutex to inspect or
//     transition lifecycle state.
//   - inFlight counts Publish calls that have passed the status gate but not
//     yet completed their producer.Input() send. Stop waits on this WaitGroup
//     before calling AsyncClose so we never send on a closed channel.
//   - errMu protects err only; it is written from the error dispatcher
//     goroutine and read via Error().
type KafkaPublisher struct {
	// Immutable after creation.
	topicName string
	producer  sarama.AsyncProducer

	// Lifecycle channels.
	waitStarted    chan struct{}
	waitTerminated chan struct{}

	// status holds a kafkaPublisherStatus value. Stored as int32 for atomic
	// access — see CompareAndSwap calls in Start and Stop.
	status atomic.Int32

	wg       sync.WaitGroup // dispatcher goroutines
	inFlight sync.WaitGroup // Publish calls between status check and channel send

	errMu sync.Mutex
	err   error
}

// NewKafkaPublisher creates a new KafkaPublisher wrapping the given
// AsyncProducer. The producer must have Return.Successes and Return.Errors
// enabled — pscompat verifies this on the *sarama.Config before constructing
// the producer (see validateKafkaPublisherConfig); the AsyncProducer interface
// itself does not expose its config.
func NewKafkaPublisher(producer sarama.AsyncProducer, topicName string) *KafkaPublisher {
	// status defaults to 0 == kafkaPublisherUninitialized via atomic.Int32 zero value.
	return &KafkaPublisher{
		topicName:      topicName,
		producer:       producer,
		waitStarted:    make(chan struct{}),
		waitTerminated: make(chan struct{}),
	}
}

// Publish converts the PubSubMessage to a Kafka ProducerMessage and sends it
// asynchronously. The onResult callback is invoked when the message is
// acknowledged by Kafka or fails.
func (kp *KafkaPublisher) Publish(msg *pb.PubSubMessage, onResult PublishResultFunc) {
	// Add to inFlight before checking status so Stop's Wait() observes us.
	// Without this guard, Stop could call AsyncClose() (which closes the
	// producer's Input channel) between our status check and our channel send,
	// causing a "send on closed channel" panic.
	kp.inFlight.Add(1)
	defer kp.inFlight.Done()

	if kp.status.Load() != int32(kafkaPublisherActive) {
		onResult(nil, ErrServiceStopped)
		return
	}

	pm := pubsubMessageToProducerMessage(kp.topicName, msg)
	pm.Metadata = onResult // Store callback for correlation in result dispatchers.

	kp.producer.Input() <- pm
}

// Start initializes the publisher and starts result dispatcher goroutines.
func (kp *KafkaPublisher) Start() {
	if !kp.status.CompareAndSwap(int32(kafkaPublisherUninitialized), int32(kafkaPublisherActive)) {
		return // already started (or beyond)
	}
	close(kp.waitStarted)

	// Dispatcher for successful publishes.
	kp.wg.Add(1)
	go func() {
		defer kp.wg.Done()
		for msg := range kp.producer.Successes() {
			onResult, ok := msg.Metadata.(PublishResultFunc)
			if !ok {
				continue
			}
			onResult(&MessageMetadata{
				Partition: int(msg.Partition),
				Offset:    msg.Offset,
			}, nil)
		}
	}()

	// Dispatcher for publish errors.
	kp.wg.Add(1)
	go func() {
		defer kp.wg.Done()
		for prodErr := range kp.producer.Errors() {
			if prodErr.Msg == nil {
				continue
			}
			onResult, ok := prodErr.Msg.Metadata.(PublishResultFunc)
			if !ok {
				continue
			}
			err := fmt.Errorf("gmk: kafka publish error: %w", prodErr.Err)
			kp.setError(err)
			onResult(nil, err)
		}
	}()

	// Background goroutine that waits for dispatchers to finish after
	// AsyncClose, then marks the publisher as terminated.
	go func() {
		kp.wg.Wait()
		kp.status.Store(int32(kafkaPublisherTerminated))
		close(kp.waitTerminated)
	}()
}

// WaitStarted blocks until the publisher is started. Returns any startup error.
func (kp *KafkaPublisher) WaitStarted() error {
	<-kp.waitStarted
	return kp.Error()
}

// Stop initiates a graceful shutdown. Pending messages are flushed before the
// producer channels are closed.
func (kp *KafkaPublisher) Stop() {
	// Only the first transition to terminating proceeds. Active → terminating
	// is the only valid edge here; if we're uninitialized or already past
	// terminating, return.
	if !kp.status.CompareAndSwap(int32(kafkaPublisherActive), int32(kafkaPublisherTerminating)) {
		return
	}

	// Wait for any Publish that already passed the status gate to finish its
	// send to producer.Input(); only then is it safe to AsyncClose, which
	// closes the Input channel.
	kp.inFlight.Wait()
	kp.producer.AsyncClose()
}

// WaitStopped blocks until the publisher has fully stopped.
func (kp *KafkaPublisher) WaitStopped() error {
	<-kp.waitTerminated
	return kp.Error()
}

// Error returns the first error that caused the publisher to fail, or nil.
func (kp *KafkaPublisher) Error() error {
	kp.errMu.Lock()
	defer kp.errMu.Unlock()
	return kp.err
}

func (kp *KafkaPublisher) setError(err error) {
	kp.errMu.Lock()
	defer kp.errMu.Unlock()
	if kp.err == nil {
		kp.err = err
	}
}

// pubsubMessageToProducerMessage converts a PubSubMessage proto to a Sarama
// ProducerMessage.
//
// Mapping:
//
//	PubSubMessage.Key        → ProducerMessage.Key (bytes)
//	PubSubMessage.Data       → ProducerMessage.Value (bytes)
//	PubSubMessage.Attributes → ProducerMessage.Headers (one header per value)
//	PubSubMessage.EventTime  → Header "pubsublite.event_time" (seconds as string)
func pubsubMessageToProducerMessage(topic string, msg *pb.PubSubMessage) *sarama.ProducerMessage {
	pm := &sarama.ProducerMessage{
		Topic: topic,
		Value: sarama.ByteEncoder(msg.GetData()),
	}

	if len(msg.GetKey()) > 0 {
		pm.Key = sarama.ByteEncoder(msg.GetKey())
	}

	// Convert multi-valued attributes to Kafka headers.
	for key, attrVals := range msg.GetAttributes() {
		for _, val := range attrVals.GetValues() {
			pm.Headers = append(pm.Headers, sarama.RecordHeader{
				Key:   []byte(key),
				Value: val,
			})
		}
	}

	// Encode event time as a special header.
	if msg.GetEventTime() != nil {
		t := msg.GetEventTime()
		pm.Headers = append(pm.Headers, sarama.RecordHeader{
			Key:   []byte("pubsublite.event_time"),
			Value: []byte(fmt.Sprintf("%d.%09d", t.GetSeconds(), t.GetNanos())),
		})
	}

	return pm
}

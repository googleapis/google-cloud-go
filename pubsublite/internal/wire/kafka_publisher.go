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
type KafkaPublisher struct {
	// Immutable after creation.
	topicName string
	producer  sarama.AsyncProducer

	// Lifecycle channels.
	waitStarted    chan struct{}
	waitTerminated chan struct{}

	// WaitGroup for dispatcher goroutines.
	wg sync.WaitGroup

	mu     sync.Mutex
	status kafkaPublisherStatus
	err    error
}

// NewKafkaPublisher creates a new KafkaPublisher wrapping the given
// AsyncProducer. The producer must have Return.Successes and Return.Errors
// enabled.
func NewKafkaPublisher(producer sarama.AsyncProducer, topicName string) *KafkaPublisher {
	return &KafkaPublisher{
		topicName:      topicName,
		producer:       producer,
		waitStarted:    make(chan struct{}),
		waitTerminated: make(chan struct{}),
		status:         kafkaPublisherUninitialized,
	}
}

// Publish converts the PubSubMessage to a Kafka ProducerMessage and sends it
// asynchronously. The onResult callback is invoked when the message is
// acknowledged by Kafka or fails.
func (kp *KafkaPublisher) Publish(msg *pb.PubSubMessage, onResult PublishResultFunc) {
	kp.mu.Lock()
	if kp.status != kafkaPublisherActive {
		kp.mu.Unlock()
		onResult(nil, ErrServiceStopped)
		return
	}
	kp.mu.Unlock()

	pm := pubsubMessageToProducerMessage(kp.topicName, msg)
	pm.Metadata = onResult // Store callback for correlation in result dispatchers.

	kp.producer.Input() <- pm
}

// Start initializes the publisher and starts result dispatcher goroutines.
func (kp *KafkaPublisher) Start() {
	kp.mu.Lock()
	defer kp.mu.Unlock()

	if kp.status != kafkaPublisherUninitialized {
		return
	}
	kp.status = kafkaPublisherActive
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
		kp.mu.Lock()
		kp.status = kafkaPublisherTerminated
		kp.mu.Unlock()
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
	kp.mu.Lock()
	if kp.status >= kafkaPublisherTerminating {
		kp.mu.Unlock()
		return
	}
	kp.status = kafkaPublisherTerminating
	kp.mu.Unlock()

	kp.producer.AsyncClose()
}

// WaitStopped blocks until the publisher has fully stopped.
func (kp *KafkaPublisher) WaitStopped() error {
	<-kp.waitTerminated
	return kp.Error()
}

// Error returns the first error that caused the publisher to fail, or nil.
func (kp *KafkaPublisher) Error() error {
	kp.mu.Lock()
	defer kp.mu.Unlock()
	return kp.err
}

func (kp *KafkaPublisher) setError(err error) {
	kp.mu.Lock()
	defer kp.mu.Unlock()
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

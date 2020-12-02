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

package ps

import (
	"time"

	"cloud.google.com/go/pubsub"
	"cloud.google.com/go/pubsublite/internal/wire"

	pb "google.golang.org/genproto/googleapis/cloud/pubsublite/v1"
)

const (
	// MaxPublishRequestCount is the maximum number of messages that can be
	// batched in a single publish request.
	MaxPublishRequestCount = wire.MaxPublishRequestCount

	// MaxPublishMessageBytes is the maximum allowed serialized size of a single
	// Pub/Sub message in bytes.
	MaxPublishMessageBytes = wire.MaxPublishMessageBytes

	// MaxPublishRequestBytes is the maximum allowed serialized size of a single
	// publish request (containing a batch of messages) in bytes.
	MaxPublishRequestBytes = wire.MaxPublishRequestBytes
)

// KeyExtractorFunc is a function that extracts an ordering key from a Message.
type KeyExtractorFunc func(*pubsub.Message) []byte

// PublishMessageTransformerFunc transforms a pubsub.Message to a Pub/Sub Lite
// PubSubMessage. If this returns an error, the pubsub.PublishResult will be
// errored and the PublisherClient will consider this a fatal error and
// terminate.
type PublishMessageTransformerFunc func(*pubsub.Message, *pb.PubSubMessage) error

// PublishSettings control the batching of published messages. These settings
// apply per partition.
//
// Use DefaultPublishSettings for defaults, as an empty PublishSettings will
// fail validation.
type PublishSettings struct {
	// Publish a non-empty batch after this delay has passed. Must be > 0.
	DelayThreshold time.Duration

	// Publish a batch when it has this many messages. Must be > 0. The maximum is
	// MaxPublishRequestCount.
	CountThreshold int

	// Publish a batch when its size in bytes reaches this value. Must be > 0. The
	// maximum is MaxPublishRequestBytes.
	ByteThreshold int

	// The maximum time that the client will attempt to establish a publish stream
	// connection to the server. Must be > 0.
	//
	// The timeout is exceeded, the publisher will terminate with the last error
	// that occurred while trying to reconnect. Note that if the timeout duration
	// is long, ErrOverflow may occur first.
	Timeout time.Duration

	// The maximum number of bytes that the publisher will keep in memory before
	// returning ErrOverflow. Must be > 0.
	//
	// Note that Pub/Sub Lite topics are provisioned a publishing throughput
	// capacity, per partition, shared by all publisher clients. Setting a large
	// buffer size can mitigate transient publish spikes. However, consistently
	// attempting to publish messages at a much higher rate than the publishing
	// throughput capacity can cause the buffers to overflow. For more
	// information, see https://cloud.google.com/pubsub/lite/docs/topics.
	BufferedByteLimit int

	// Optional custom function that extracts an ordering key from a Message. The
	// default implementation extracts the key from Message.OrderingKey.
	KeyExtractor KeyExtractorFunc

	// Optional custom function that transforms a pubsub.Message to a
	// PubSubMessage API proto.
	MessageTransformer PublishMessageTransformerFunc
}

// DefaultPublishSettings holds the default values for PublishSettings.
var DefaultPublishSettings = PublishSettings{
	DelayThreshold:    wire.DefaultPublishSettings.DelayThreshold,
	CountThreshold:    wire.DefaultPublishSettings.CountThreshold,
	ByteThreshold:     wire.DefaultPublishSettings.ByteThreshold,
	Timeout:           wire.DefaultPublishSettings.Timeout,
	BufferedByteLimit: wire.DefaultPublishSettings.BufferedByteLimit,
}

func (s *PublishSettings) toWireSettings() wire.PublishSettings {
	return wire.PublishSettings{
		DelayThreshold:    s.DelayThreshold,
		CountThreshold:    s.CountThreshold,
		ByteThreshold:     s.ByteThreshold,
		Timeout:           s.Timeout,
		BufferedByteLimit: s.BufferedByteLimit,
		ConfigPollPeriod:  wire.DefaultPublishSettings.ConfigPollPeriod,
		Framework:         wire.FrameworkCloudPubSubShim,
	}
}

// NackHandler is invoked when pubsub.Message.Nack() is called. Cloud Pub/Sub
// Lite does not have a concept of 'nack'. If the nack handler implementation
// returns nil, the message is acknowledged. If an error is returned, the
// SubscriberClient will consider this a fatal error and terminate once all
// outstanding message receivers have finished.
//
// In Cloud Pub/Sub Lite, only a single subscriber for a given subscription is
// connected to any partition at a time, and there is no other client that may
// be able to handle messages.
type NackHandler func(*pubsub.Message) error

// ReceiveMessageTransformerFunc transforms a Pub/Sub Lite SequencedMessage to a
// pubsub.Message. If this returns an error, the SubscriberClient will consider
// this a fatal error and terminate.
type ReceiveMessageTransformerFunc func(*pb.SequencedMessage, *pubsub.Message) error

// ReceiveSettings configure the Receive method. These settings apply per
// partition. If MaxOutstandingBytes is being used to bound memory usage, keep
// in mind the number of partitions in the associated topic.
//
// Use DefaultReceiveSettings for defaults, as an empty ReceiveSettings will
// fail validation.
type ReceiveSettings struct {
	// MaxOutstandingMessages is the maximum number of unacknowledged messages.
	// Must be > 0.
	MaxOutstandingMessages int

	// MaxOutstandingBytes is the maximum size (in quota bytes) of unacknowledged
	// messages. Must be > 0.
	MaxOutstandingBytes int

	// The maximum time that the client will attempt to establish a subscribe
	// stream connection to the server. Must be > 0.
	//
	// The timeout is exceeded, the SubscriberClient will terminate with the last
	// error that occurred while trying to reconnect.
	Timeout time.Duration

	// The topic partition numbers (zero-indexed) to receive messages from.
	// Values must be less than the number of partitions for the topic. If not
	// specified, the SubscriberClient will use the partition assignment service
	// to determine which partitions it should connect to.
	Partitions []int

	// Optional custom function to handle pubsub.Message.Nack() calls. If not set,
	// the default behavior is to terminate the SubscriberClient.
	NackHandler NackHandler

	// Optional custom function that transforms a PubSubMessage API proto to a
	// pubsub.Message.
	MessageTransformer ReceiveMessageTransformerFunc
}

// DefaultReceiveSettings holds the default values for ReceiveSettings.
var DefaultReceiveSettings = ReceiveSettings{
	MaxOutstandingMessages: wire.DefaultReceiveSettings.MaxOutstandingMessages,
	MaxOutstandingBytes:    wire.DefaultReceiveSettings.MaxOutstandingBytes,
	Timeout:                wire.DefaultReceiveSettings.Timeout,
}

func (s *ReceiveSettings) toWireSettings() wire.ReceiveSettings {
	return wire.ReceiveSettings{
		MaxOutstandingMessages: s.MaxOutstandingMessages,
		MaxOutstandingBytes:    s.MaxOutstandingBytes,
		Timeout:                s.Timeout,
		Partitions:             s.Partitions,
		Framework:              wire.FrameworkCloudPubSubShim,
	}
}

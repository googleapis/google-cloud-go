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

package pscompat

import (
	"time"

	"cloud.google.com/go/pubsub"
	"cloud.google.com/go/pubsublite/internal/wire"

	pb "cloud.google.com/go/pubsublite/apiv1/pubsublitepb"
)

const (
	// MaxPublishRequestCount is the maximum number of messages that can be
	// batched in a single publish request.
	MaxPublishRequestCount = wire.MaxPublishRequestCount

	// MaxPublishRequestBytes is the maximum allowed serialized size of a single
	// publish request (containing a batch of messages) in bytes.
	MaxPublishRequestBytes = wire.MaxPublishRequestBytes
)

// KeyExtractorFunc is a function that extracts an ordering key from a Message.
type KeyExtractorFunc func(*pubsub.Message) []byte

// PublishMessageTransformerFunc transforms a pubsub.Message to a Pub/Sub Lite
// PubSubMessage API proto. If this returns an error, the pubsub.PublishResult
// will be errored and the PublisherClient will consider this a fatal error and
// terminate.
type PublishMessageTransformerFunc func(*pubsub.Message, *pb.PubSubMessage) error

// PublishSettings configure the PublisherClient. Batching settings
// (DelayThreshold, CountThreshold, ByteThreshold, BufferedByteLimit) apply per
// partition.
//
// A zero PublishSettings will result in values equivalent to
// DefaultPublishSettings.
type PublishSettings struct {
	// Publish a non-empty batch after this delay has passed. If DelayThreshold is
	// 0, it will be treated as DefaultPublishSettings.DelayThreshold. Otherwise
	// must be > 0.
	DelayThreshold time.Duration

	// Publish a batch when it has this many messages. The maximum is
	// MaxPublishRequestCount. If CountThreshold is 0, it will be treated as
	// DefaultPublishSettings.CountThreshold. Otherwise must be > 0.
	CountThreshold int

	// Publish a batch when its size in bytes reaches this value. The maximum is
	// MaxPublishRequestBytes. If ByteThreshold is 0, it will be treated as
	// DefaultPublishSettings.ByteThreshold. Otherwise must be > 0.
	ByteThreshold int

	// The maximum time that the client will attempt to open a publish stream
	// to the server. If Timeout is 0, it will be treated as
	// DefaultPublishSettings.Timeout. Otherwise must be > 0.
	//
	// If your application has a low tolerance to backend unavailability, set
	// Timeout to a lower duration to detect and handle. When the timeout is
	// exceeded, the PublisherClient will terminate with ErrBackendUnavailable and
	// details of the last error that occurred while trying to reconnect to
	// backends. Note that if the timeout duration is long, ErrOverflow may occur
	// first.
	//
	// It is not recommended to set Timeout below 2 minutes. If no failover
	// operations need to be performed by the application, it is recommended to
	// just use the default timeout value to avoid the PublisherClient terminating
	// during short periods of backend unavailability.
	Timeout time.Duration

	// The maximum number of bytes that the publisher will keep in memory before
	// returning ErrOverflow. If BufferedByteLimit is 0, it will be treated as
	// DefaultPublishSettings.BufferedByteLimit. Otherwise must be > 0.
	//
	// Note that this setting applies per partition. If BufferedByteLimit is being
	// used to bound memory usage, keep in mind the number of partitions in the
	// topic.
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

	// The polling interval to watch for topic partition count updates.
	// Currently internal only and overridden in tests.
	configPollPeriod time.Duration
}

// DefaultPublishSettings holds the default values for PublishSettings.
var DefaultPublishSettings = PublishSettings{
	DelayThreshold:    10 * time.Millisecond,
	CountThreshold:    100,
	ByteThreshold:     1e6,
	Timeout:           7 * 24 * time.Hour,
	BufferedByteLimit: 1e10,
}

func (s *PublishSettings) toWireSettings() wire.PublishSettings {
	wireSettings := wire.PublishSettings{
		DelayThreshold:    DefaultPublishSettings.DelayThreshold,
		CountThreshold:    DefaultPublishSettings.CountThreshold,
		ByteThreshold:     DefaultPublishSettings.ByteThreshold,
		Timeout:           DefaultPublishSettings.Timeout,
		BufferedByteLimit: DefaultPublishSettings.BufferedByteLimit,
		ConfigPollPeriod:  wire.DefaultPublishSettings.ConfigPollPeriod,
		Framework:         wire.FrameworkCloudPubSubShim,
	}
	// Negative values preserved, but will fail validation in wire package.
	if s.DelayThreshold != 0 {
		wireSettings.DelayThreshold = s.DelayThreshold
	}
	if s.CountThreshold != 0 {
		wireSettings.CountThreshold = s.CountThreshold
	}
	if s.ByteThreshold != 0 {
		wireSettings.ByteThreshold = s.ByteThreshold
	}
	if s.Timeout != 0 {
		wireSettings.Timeout = s.Timeout
	}
	if s.BufferedByteLimit != 0 {
		wireSettings.BufferedByteLimit = s.BufferedByteLimit
	}
	if s.configPollPeriod != 0 {
		wireSettings.ConfigPollPeriod = s.configPollPeriod
	}
	return wireSettings
}

// NackHandler is invoked when pubsub.Message.Nack() is called. Pub/Sub Lite
// does not have a concept of 'nack'. If the nack handler implementation returns
// nil, the message is acknowledged. If an error is returned, the
// SubscriberClient will consider this a fatal error and terminate.
//
// In Pub/Sub Lite, only a single subscriber for a given subscription is
// connected to any partition at a time, and there is no other client that may
// be able to handle messages.
type NackHandler func(*pubsub.Message) error

// ReceiveMessageTransformerFunc transforms a Pub/Sub Lite SequencedMessage API
// proto to a pubsub.Message. The implementation must not set pubsub.Message.ID.
//
// If this returns an error, the SubscriberClient will consider this a fatal
// error and terminate.
type ReceiveMessageTransformerFunc func(*pb.SequencedMessage, *pubsub.Message) error

// ReassignmentHandlerFunc is called any time a new partition assignment is
// received from the server. It will be called with both the previous and new
// partition numbers as decided by the server. Both slices of partition numbers
// are sorted in ascending order.
//
// When this handler is called, partitions that are being assigned away are
// stopping and new partitions are starting. Acks and nacks for messages from
// partitions that are being assigned away will have no effect, but message
// deliveries may still be in flight.
//
// The client library will not acknowledge the assignment until this handler
// returns. The server will not assign any of the partitions in
// `previousPartitions` to another client unless the assignment is acknowledged,
// or a client takes too long to acknowledge (currently 30 seconds from the time
// the assignment is sent from server's point of view).
//
// Because of the above, as long as reassignment handling is processed quickly,
// it can be used to abort outstanding operations on partitions which are being
// assigned away from this client.
//
// If this handler returns an error, the SubscriberClient will consider this a
// fatal error and terminate.
type ReassignmentHandlerFunc func(previousPartitions, nextPartitions []int) error

// ReceiveSettings configure the SubscriberClient. Flow control settings
// (MaxOutstandingMessages, MaxOutstandingBytes) apply per partition.
//
// A zero ReceiveSettings will result in values equivalent to
// DefaultReceiveSettings.
type ReceiveSettings struct {
	// MaxOutstandingMessages is the maximum number of unacknowledged messages.
	// If MaxOutstandingMessages is 0, it will be treated as
	// DefaultReceiveSettings.MaxOutstandingMessages. Otherwise must be > 0.
	MaxOutstandingMessages int

	// MaxOutstandingBytes is the maximum size (in quota bytes) of unacknowledged
	// messages. If MaxOutstandingBytes is 0, it will be treated as
	// DefaultReceiveSettings.MaxOutstandingBytes. Otherwise must be > 0.
	//
	// Note that this setting applies per partition. If MaxOutstandingBytes is
	// being used to bound memory usage, keep in mind the number of partitions in
	// the associated topic.
	MaxOutstandingBytes int

	// The maximum time that the client will attempt to open a subscribe stream
	// to the server. If Timeout is 0, it will be treated as
	// DefaultReceiveSettings.Timeout. Otherwise must be > 0.
	//
	// If your application has a low tolerance to backend unavailability, set
	// Timeout to a lower duration to detect and handle. When the timeout is
	// exceeded, the SubscriberClient will terminate with ErrBackendUnavailable
	// and details of the last error that occurred while trying to reconnect to
	// backends.
	//
	// It is not recommended to set Timeout below 2 minutes. If no failover
	// operations need to be performed by the application, it is recommended to
	// just use the default timeout value to avoid the SubscriberClient
	// terminating during short periods of backend unavailability.
	Timeout time.Duration

	// The topic partition numbers (zero-indexed) to receive messages from.
	// Values must be less than the number of partitions for the topic. If not
	// specified, the SubscriberClient will use the partition assignment service
	// to determine which partitions it should connect to.
	Partitions []int

	// Optional custom function to handle pubsub.Message.Nack() calls. If not set,
	// the default behavior is to terminate the SubscriberClient.
	NackHandler NackHandler

	// Optional custom function that transforms a SequencedMessage API proto to a
	// pubsub.Message.
	MessageTransformer ReceiveMessageTransformerFunc

	// Optional custom function that is called when a new partition assignment has
	// been delivered to the client.
	ReassignmentHandler ReassignmentHandlerFunc
}

// DefaultReceiveSettings holds the default values for ReceiveSettings.
var DefaultReceiveSettings = ReceiveSettings{
	MaxOutstandingMessages: 1000,
	MaxOutstandingBytes:    1e9,
	Timeout:                7 * 24 * time.Hour,
}

func (s *ReceiveSettings) toWireSettings() wire.ReceiveSettings {
	wireSettings := wire.ReceiveSettings{
		MaxOutstandingMessages: DefaultReceiveSettings.MaxOutstandingMessages,
		MaxOutstandingBytes:    DefaultReceiveSettings.MaxOutstandingBytes,
		Timeout:                DefaultReceiveSettings.Timeout,
		Partitions:             s.Partitions,
		Framework:              wire.FrameworkCloudPubSubShim,
	}
	// Negative values preserved, but will fail validation in wire package.
	if s.MaxOutstandingMessages != 0 {
		wireSettings.MaxOutstandingMessages = s.MaxOutstandingMessages
	}
	if s.MaxOutstandingBytes != 0 {
		wireSettings.MaxOutstandingBytes = s.MaxOutstandingBytes
	}
	if s.Timeout != 0 {
		wireSettings.Timeout = s.Timeout
	}
	return wireSettings
}

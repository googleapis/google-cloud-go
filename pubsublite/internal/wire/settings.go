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
	"errors"
	"fmt"
	"time"
)

const (
	// MaxPublishRequestCount is the maximum number of messages that can be
	// batched in a single publish request.
	MaxPublishRequestCount = 1000

	// MaxPublishRequestBytes is the maximum allowed serialized size of a single
	// publish request (containing a batch of messages) in bytes. Must be lower
	// than the gRPC limit of 4 MiB.
	MaxPublishRequestBytes int = 3.5 * 1024 * 1024
)

// FrameworkType is the user-facing API for Cloud Pub/Sub Lite.
type FrameworkType string

// FrameworkCloudPubSubShim is the API that emulates Cloud Pub/Sub.
const FrameworkCloudPubSubShim FrameworkType = "CLOUD_PUBSUB_SHIM"

// PublishSettings control the batching of published messages. These settings
// apply per partition.
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

	// The polling interval to watch for topic partition count updates. Set to 0
	// to disable polling if the number of partitions will never update.
	ConfigPollPeriod time.Duration

	// The user-facing API type.
	Framework FrameworkType
}

// DefaultPublishSettings holds the default values for PublishSettings.
var DefaultPublishSettings = PublishSettings{
	DelayThreshold: 10 * time.Millisecond,
	CountThreshold: 100,
	ByteThreshold:  1e6,
	Timeout:        10 * time.Minute,
	// By default set to a high limit that is not likely to occur, but prevents
	// OOM errors in clients.
	BufferedByteLimit: 1 << 30, // 1 GiB
	ConfigPollPeriod:  10 * time.Minute,
}

func validatePublishSettings(settings PublishSettings) error {
	if settings.DelayThreshold <= 0 {
		return errors.New("pubsublite: invalid publish settings. DelayThreshold duration must be > 0")
	}
	if settings.Timeout <= 0 {
		return errors.New("pubsublite: invalid publish settings. Timeout duration must be > 0")
	}
	if settings.CountThreshold <= 0 {
		return errors.New("pubsublite: invalid publish settings. CountThreshold must be > 0")
	}
	if settings.CountThreshold > MaxPublishRequestCount {
		return fmt.Errorf("pubsublite: invalid publish settings. Maximum CountThreshold is MaxPublishRequestCount (%d)", MaxPublishRequestCount)
	}
	if settings.ByteThreshold <= 0 {
		return errors.New("pubsublite: invalid publish settings. ByteThreshold must be > 0")
	}
	if settings.ByteThreshold > MaxPublishRequestBytes {
		return fmt.Errorf("pubsublite: invalid publish settings. Maximum ByteThreshold is MaxPublishRequestBytes (%d)", MaxPublishRequestBytes)
	}
	if settings.BufferedByteLimit <= 0 {
		return errors.New("pubsublite: invalid publish settings. BufferedByteLimit must be > 0")
	}
	return nil
}

// ReceiveSettings control the receiving of messages. These settings apply
// per partition.
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
	// The timeout is exceeded, the subscriber will terminate with the last error
	// that occurred while trying to reconnect.
	Timeout time.Duration

	// The topic partition numbers (zero-indexed) to receive messages from.
	// Values must be less than the number of partitions for the topic. If not
	// specified, the client will use the partition assignment service to
	// determine which partitions it should connect to.
	Partitions []int

	// The user-facing API type.
	Framework FrameworkType
}

// DefaultReceiveSettings holds the default values for ReceiveSettings.
var DefaultReceiveSettings = ReceiveSettings{
	MaxOutstandingMessages: 1000,
	MaxOutstandingBytes:    1e9,
	Timeout:                10 * time.Minute,
}

func validateReceiveSettings(settings ReceiveSettings) error {
	if settings.MaxOutstandingMessages <= 0 {
		return errors.New("pubsublite: invalid receive settings. MaxOutstandingMessages must be > 0")
	}
	if settings.MaxOutstandingBytes <= 0 {
		return errors.New("pubsublite: invalid receive settings. MaxOutstandingBytes must be > 0")
	}
	if settings.Timeout <= 0 {
		return errors.New("pubsublite: invalid receive settings. Timeout duration must be > 0")
	}
	if len(settings.Partitions) > 0 {
		var void struct{}
		partitionMap := make(map[int]struct{})
		for _, p := range settings.Partitions {
			if p < 0 {
				return fmt.Errorf("pubsublite: invalid partition number %d in receive settings. Partition numbers are zero-indexed", p)
			}
			if _, exists := partitionMap[p]; exists {
				return fmt.Errorf("pubsublite: invalid receive settings. Duplicate partition number %d", p)
			}
			partitionMap[p] = void
		}
	}
	return nil
}

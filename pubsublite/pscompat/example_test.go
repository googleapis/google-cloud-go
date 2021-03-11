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

package pscompat_test

import (
	"context"
	"fmt"
	"time"

	"cloud.google.com/go/pubsub"
	"cloud.google.com/go/pubsublite/pscompat"
)

func ExamplePublisherClient_Publish() {
	ctx := context.Background()
	const topic = "projects/my-project/locations/zone/topics/my-topic"
	publisher, err := pscompat.NewPublisherClient(ctx, topic)
	if err != nil {
		// TODO: Handle error.
	}
	defer publisher.Stop()

	var results []*pubsub.PublishResult
	r := publisher.Publish(ctx, &pubsub.Message{
		Data: []byte("hello world"),
	})
	results = append(results, r)
	// Publish more messages ...
	for _, r := range results {
		id, err := r.Get(ctx)
		if err != nil {
			// TODO: Handle error.
			// NOTE: The publisher will terminate upon first error. Create a new
			// publisher to republish failed messages.
		}
		fmt.Printf("Published a message with a message ID: %s\n", id)
	}
}

// This example illustrates how to set batching settings for publishing. Note
// that batching settings apply per partition. If BufferedByteLimit is being
// used to bound memory usage, keep in mind the number of partitions in the
// topic.
func ExamplePublisherClient_Publish_batchingSettings() {
	ctx := context.Background()
	const topic = "projects/my-project/locations/zone/topics/my-topic"
	settings := pscompat.DefaultPublishSettings
	settings.DelayThreshold = 50 * time.Millisecond
	settings.CountThreshold = 200
	settings.BufferedByteLimit = 5e8
	publisher, err := pscompat.NewPublisherClientWithSettings(ctx, topic, settings)
	if err != nil {
		// TODO: Handle error.
	}
	defer publisher.Stop()

	var results []*pubsub.PublishResult
	r := publisher.Publish(ctx, &pubsub.Message{
		Data: []byte("hello world"),
	})
	results = append(results, r)
	// Publish more messages ...
	for _, r := range results {
		id, err := r.Get(ctx)
		if err != nil {
			// TODO: Handle error.
			// NOTE: The publisher will terminate upon first error. Create a new
			// publisher to republish failed messages.
		}
		fmt.Printf("Published a message with a message ID: %s\n", id)
	}
}

func ExamplePublisherClient_Error() {
	ctx := context.Background()
	const topic = "projects/my-project/locations/zone/topics/my-topic"
	publisher, err := pscompat.NewPublisherClient(ctx, topic)
	if err != nil {
		// TODO: Handle error.
	}
	defer publisher.Stop()

	var results []*pubsub.PublishResult
	r := publisher.Publish(ctx, &pubsub.Message{
		Data: []byte("hello world"),
	})
	results = append(results, r)
	// Publish more messages ...
	for _, r := range results {
		id, err := r.Get(ctx)
		if err != nil {
			// Prints the fatal error that caused the publisher to terminate.
			fmt.Printf("Publisher client stopped due to error: %v\n", publisher.Error())

			// TODO: Handle error.
			// NOTE: The publisher will terminate upon first error. Create a new
			// publisher to republish failed messages.
		}
		fmt.Printf("Published a message with a message ID: %s\n", id)
	}
}

func ExampleSubscriberClient_Receive() {
	ctx := context.Background()
	const subscription = "projects/my-project/locations/zone/subscriptions/my-subscription"
	subscriber, err := pscompat.NewSubscriberClient(ctx, subscription)
	if err != nil {
		// TODO: Handle error.
	}
	cctx, cancel := context.WithCancel(ctx)
	err = subscriber.Receive(cctx, func(ctx context.Context, m *pubsub.Message) {
		// TODO: Handle message.
		// NOTE: May be called concurrently; synchronize access to shared memory.
		m.Ack()
	})
	if err != nil {
		// TODO: Handle error.
	}

	// Call cancel from callback, or another goroutine.
	cancel()
}

// This example shows how to throttle SubscriberClient.Receive, which aims for
// high throughput by default. By limiting the number of messages and/or bytes
// being processed at once, you can bound your program's resource consumption.
// Note that ReceiveSettings apply per partition, so keep in mind the number of
// partitions in the associated topic.
func ExampleSubscriberClient_Receive_maxOutstanding() {
	ctx := context.Background()
	const subscription = "projects/my-project/locations/zone/subscriptions/my-subscription"
	settings := pscompat.DefaultReceiveSettings
	settings.MaxOutstandingMessages = 5
	settings.MaxOutstandingBytes = 10e6
	subscriber, err := pscompat.NewSubscriberClientWithSettings(ctx, subscription, settings)
	if err != nil {
		// TODO: Handle error.
	}
	cctx, cancel := context.WithCancel(ctx)
	err = subscriber.Receive(cctx, func(ctx context.Context, m *pubsub.Message) {
		// TODO: Handle message.
		// NOTE: May be called concurrently; synchronize access to shared memory.
		m.Ack()
	})
	if err != nil {
		// TODO: Handle error.
	}

	// Call cancel from the receiver callback or another goroutine to stop
	// receiving.
	cancel()
}

// This example shows how to manually assign which topic partitions a
// SubscriberClient should connect to. If not specified, the SubscriberClient
// will use Pub/Sub Lite's partition assignment service to automatically
// determine which partitions it should connect to.
func ExampleSubscriberClient_Receive_manualPartitionAssignment() {
	ctx := context.Background()
	const subscription = "projects/my-project/locations/zone/subscriptions/my-subscription"
	settings := pscompat.DefaultReceiveSettings
	// NOTE: The corresponding topic must have 2 or more partitions.
	settings.Partitions = []int{0, 1}
	subscriber, err := pscompat.NewSubscriberClientWithSettings(ctx, subscription, settings)
	if err != nil {
		// TODO: Handle error.
	}
	cctx, cancel := context.WithCancel(ctx)
	err = subscriber.Receive(cctx, func(ctx context.Context, m *pubsub.Message) {
		// TODO: Handle message.
		// NOTE: May be called concurrently; synchronize access to shared memory.
		m.Ack()
	})
	if err != nil {
		// TODO: Handle error.
	}

	// Call cancel from the receiver callback or another goroutine to stop
	// receiving.
	cancel()
}

func ExampleParseMessageMetadata_publisher() {
	ctx := context.Background()
	const topic = "projects/my-project/locations/zone/topics/my-topic"
	publisher, err := pscompat.NewPublisherClient(ctx, topic)
	if err != nil {
		// TODO: Handle error.
	}
	defer publisher.Stop()

	result := publisher.Publish(ctx, &pubsub.Message{Data: []byte("payload")})
	id, err := result.Get(ctx)
	if err != nil {
		// TODO: Handle error.
	}
	metadata, err := pscompat.ParseMessageMetadata(id)
	if err != nil {
		// TODO: Handle error.
	}
	fmt.Printf("Published message to partition %d with offset %d\n", metadata.Partition, metadata.Offset)
}

func ExampleParseMessageMetadata_subscriber() {
	ctx := context.Background()
	const subscription = "projects/my-project/locations/zone/subscriptions/my-subscription"
	subscriber, err := pscompat.NewSubscriberClient(ctx, subscription)
	if err != nil {
		// TODO: Handle error.
	}
	err = subscriber.Receive(ctx, func(ctx context.Context, m *pubsub.Message) {
		// TODO: Handle message.
		m.Ack()
		metadata, err := pscompat.ParseMessageMetadata(m.ID)
		if err != nil {
			// TODO: Handle error.
		}
		fmt.Printf("Received message from partition %d with offset %d\n", metadata.Partition, metadata.Offset)
	})
	if err != nil {
		// TODO: Handle error.
	}
}

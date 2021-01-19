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

	"cloud.google.com/go/pubsub"
	"cloud.google.com/go/pubsublite/pscompat"
)

func ExamplePublisherClient_Publish() {
	ctx := context.Background()
	const topic = "projects/my-project/locations/zone/topics/my-topic"
	// NOTE: DefaultPublishSettings and empty PublishSettings{} are equivalent.
	publisher, err := pscompat.NewPublisherClient(ctx, pscompat.DefaultPublishSettings, topic)
	if err != nil {
		// TODO: Handle error.
	}
	defer publisher.Stop()

	var results []*pubsub.PublishResult
	r := publisher.Publish(ctx, &pubsub.Message{
		Data: []byte("hello world"),
	})
	results = append(results, r)
	// Do other work ...
	for _, r := range results {
		id, err := r.Get(ctx)
		if err != nil {
			// TODO: Handle error.
		}
		fmt.Printf("Published a message with a message ID: %s\n", id)
	}
}

func ExamplePublisherClient_Error() {
	ctx := context.Background()
	const topic = "projects/my-project/locations/zone/topics/my-topic"
	publisher, err := pscompat.NewPublisherClient(ctx, pscompat.DefaultPublishSettings, topic)
	if err != nil {
		// TODO: Handle error.
	}
	defer publisher.Stop()

	var results []*pubsub.PublishResult
	r := publisher.Publish(ctx, &pubsub.Message{
		Data: []byte("hello world"),
	})
	results = append(results, r)
	// Do other work ...
	for _, r := range results {
		id, err := r.Get(ctx)
		if err != nil {
			// TODO: Handle error.
			if err == pscompat.ErrPublisherStopped {
				fmt.Printf("Publisher client stopped due to error: %v\n", publisher.Error())
			}
		}
		fmt.Printf("Published a message with a message ID: %s\n", id)
	}
}

func ExampleSubscriberClient_Receive() {
	ctx := context.Background()
	const subscription = "projects/my-project/locations/zone/subscriptions/my-subscription"
	// NOTE: DefaultReceiveSettings and empty ReceiveSettings{} are equivalent.
	subscriber, err := pscompat.NewSubscriberClient(ctx, pscompat.DefaultReceiveSettings, subscription)
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
	subscriber, err := pscompat.NewSubscriberClient(ctx, settings, subscription)
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

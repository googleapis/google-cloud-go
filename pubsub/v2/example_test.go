// Copyright 2014 Google LLC
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//      http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package pubsub_test

import (
	"context"
	"errors"
	"fmt"
	"time"

	"cloud.google.com/go/pubsub/v2"
)

func ExampleNewClient() {
	ctx := context.Background()
	_, err := pubsub.NewClient(ctx, "project-id")
	if err != nil {
		// TODO: Handle error.
	}

	// See the other examples to learn how to use the Client.
}

// Use Publisher to refer to a topic that is not in the client's project, such
// as a public topic.
func ExampleClient_Publisher() {
	ctx := context.Background()
	client, err := pubsub.NewClient(ctx, "project-id")
	if err != nil {
		// TODO: Handle error.
	}
	otherProjectID := "another-project-id"
	publisher := client.Publisher(fmt.Sprintf("projects/%s/topics/%s", otherProjectID, "my-topic"))
	_ = publisher // TODO: use the publisher client.
}

func ExamplePublisher_Publish() {
	ctx := context.Background()
	client, err := pubsub.NewClient(ctx, "project-id")
	if err != nil {
		// TODO: Handle error.
	}

	publisher := client.Publisher("topicName")
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

func ExampleSubscriber_Receive() {
	ctx := context.Background()
	client, err := pubsub.NewClient(ctx, "project-id")
	if err != nil {
		// TODO: Handle error.
	}
	// Can use either "projects/project-id/subscriptions/sub-id" or just "sub-id" here
	sub := client.Subscriber("sub-id")
	err = sub.Receive(ctx, func(ctx context.Context, m *pubsub.Message) {
		// TODO: Handle message.
		// NOTE: May be called concurrently; synchronize access to shared memory.
		m.Ack()
	})
	if err != nil && !errors.Is(err, context.Canceled) {
		// TODO: Handle error.
	}
}

// This example shows how to configure keepalive so that unacknowledged messages
// expire quickly, allowing other subscribers to take them.
func ExampleSubscriber_Receive_maxExtension() {
	ctx := context.Background()
	client, err := pubsub.NewClient(ctx, "project-id")
	if err != nil {
		// TODO: Handle error.
	}
	sub := client.Subscriber("subNameOrID")
	// This program is expected to process and acknowledge messages in 30 seconds. If
	// not, the Pub/Sub API will assume the message is not acknowledged.
	sub.ReceiveSettings.MaxExtension = 30 * time.Second
	err = sub.Receive(ctx, func(ctx context.Context, m *pubsub.Message) {
		// TODO: Handle message.
		m.Ack()
	})
	if err != nil && !errors.Is(err, context.Canceled) {
		// TODO: Handle error.
	}
}

// This example shows how to throttle Subscription.Receive, which aims for high
// throughput by default. By limiting the number of messages and/or bytes being
// processed at once, you can bound your program's resource consumption.
func ExampleSubscriber_Receive_maxOutstanding() {
	ctx := context.Background()
	client, err := pubsub.NewClient(ctx, "project-id")
	if err != nil {
		// TODO: Handle error.
	}
	sub := client.Subscriber("subNameOrID")
	sub.ReceiveSettings.MaxOutstandingMessages = 5
	sub.ReceiveSettings.MaxOutstandingBytes = 10e6
	err = sub.Receive(ctx, func(ctx context.Context, m *pubsub.Message) {
		// TODO: Handle message.
		m.Ack()
	})
	if err != nil && !errors.Is(err, context.Canceled) {
		// TODO: Handle error.
	}
}

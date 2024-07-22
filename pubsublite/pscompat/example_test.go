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
	"errors"
	"fmt"
	"log"
	"sync"
	"time"

	"cloud.google.com/go/pubsub"
	api "cloud.google.com/go/pubsublite/apiv1"
	"cloud.google.com/go/pubsublite/pscompat"
	"golang.org/x/oauth2/google"
	"golang.org/x/sync/errgroup"
	"google.golang.org/api/option"
	"google.golang.org/protobuf/types/known/timestamppb"
)

func ExamplePublisherClient_Publish() {
	ctx := context.Background()
	const topic = "projects/my-project/locations/region-or-zone/topics/my-topic"
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

	var publishFailed bool
	for _, r := range results {
		id, err := r.Get(ctx)
		if err != nil {
			// TODO: Handle error.
			publishFailed = true
			continue
		}
		fmt.Printf("Published a message with a message ID: %s\n", id)
	}

	// NOTE: A failed PublishResult indicates that the publisher client
	// encountered a fatal error and has permanently terminated. After the fatal
	// error has been resolved, a new publisher client instance must be created to
	// republish failed messages.
	if publishFailed {
		fmt.Printf("Publisher client terminated due to error: %v\n", publisher.Error())
	}
}

// This example illustrates how to configure OAuth tokens to be refreshed 5
// minutes before they expire, in order to mitigate delays which may occur
// during refresh.
func ExampleNewPublisherClient_earlyTokenRefresh() {
	ctx := context.Background()
	const topic = "projects/my-project/locations/region-or-zone/topics/my-topic"
	params := google.CredentialsParams{
		Scopes:            api.DefaultAuthScopes(),
		EarlyTokenRefresh: 5 * time.Minute,
	}
	creds, err := google.FindDefaultCredentialsWithParams(ctx, params)
	if err != nil {
		log.Fatalf("No 'Application Default Credentials' found: %v.", err)
	}
	publisher, err := pscompat.NewPublisherClient(ctx, topic, option.WithTokenSource(creds.TokenSource))
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

	var publishFailed bool
	for _, r := range results {
		id, err := r.Get(ctx)
		if err != nil {
			// TODO: Handle error.
			publishFailed = true
			continue
		}
		fmt.Printf("Published a message with a message ID: %s\n", id)
	}

	// NOTE: A failed PublishResult indicates that the publisher client
	// encountered a fatal error and has permanently terminated. After the fatal
	// error has been resolved, a new publisher client instance must be created to
	// republish failed messages.
	if publishFailed {
		fmt.Printf("Publisher client terminated due to error: %v\n", publisher.Error())
	}
}

// This example illustrates how to set batching settings for publishing. Note
// that batching settings apply per partition. If BufferedByteLimit is being
// used to bound memory usage, keep in mind the number of partitions in the
// topic.
func ExampleNewPublisherClientWithSettings_batchingSettings() {
	ctx := context.Background()
	const topic = "projects/my-project/locations/region-or-zone/topics/my-topic"
	settings := pscompat.PublishSettings{
		DelayThreshold:    50 * time.Millisecond,
		CountThreshold:    200,
		BufferedByteLimit: 5e8,
	}
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

	var publishFailed bool
	for _, r := range results {
		id, err := r.Get(ctx)
		if err != nil {
			// TODO: Handle error.
			publishFailed = true
			continue
		}
		fmt.Printf("Published a message with a message ID: %s\n", id)
	}

	// NOTE: A failed PublishResult indicates that the publisher client
	// encountered a fatal error and has permanently terminated. After the fatal
	// error has been resolved, a new publisher client instance must be created to
	// republish failed messages.
	if publishFailed {
		fmt.Printf("Publisher client terminated due to error: %v\n", publisher.Error())
	}
}

// This example illustrates how to handle various publishing errors. Some errors
// can be automatically handled (e.g. backend unavailable and buffer overflow),
// while others are fatal errors that should be inspected.
// If the application has a low tolerance to backend unavailability, set a lower
// PublishSettings.Timeout value to detect and alert.
func ExamplePublisherClient_Publish_errorHandling() {
	ctx := context.Background()
	const topic = "projects/my-project/locations/region-or-zone/topics/my-topic"
	settings := pscompat.PublishSettings{
		// The PublisherClient will terminate when it cannot connect to backends for
		// more than 10 minutes.
		Timeout: 10 * time.Minute,
		// Sets a conservative publish buffer byte limit, per partition.
		BufferedByteLimit: 1e8,
	}
	publisher, err := pscompat.NewPublisherClientWithSettings(ctx, topic, settings)
	if err != nil {
		// TODO: Handle error.
	}
	defer publisher.Stop()

	var toRepublish []*pubsub.Message
	var mu sync.Mutex
	g := new(errgroup.Group)

	for i := 0; i < 10; i++ {
		msg := &pubsub.Message{
			Data: []byte(fmt.Sprintf("message-%d", i)),
		}
		result := publisher.Publish(ctx, msg)

		g.Go(func() error {
			id, err := result.Get(ctx)
			if err != nil {
				// NOTE: A failed PublishResult indicates that the publisher client has
				// permanently terminated. A new publisher client instance must be
				// created to republish failed messages.
				fmt.Printf("Publish error: %v\n", err)
				// Oversized messages cannot be published.
				if !errors.Is(err, pscompat.ErrOversizedMessage) {
					mu.Lock()
					toRepublish = append(toRepublish, msg)
					mu.Unlock()
				}
				return err
			}
			fmt.Printf("Published a message with a message ID: %s\n", id)
			return nil
		})
	}
	if err := g.Wait(); err != nil {
		fmt.Printf("Publisher client terminated due to error: %v\n", publisher.Error())
		switch {
		case errors.Is(publisher.Error(), pscompat.ErrBackendUnavailable):
			// TODO: Create a new publisher client to republish failed messages.
		case errors.Is(publisher.Error(), pscompat.ErrOverflow):
			// TODO: Create a new publisher client to republish failed messages.
			// Throttle publishing. Note that backend unavailability can also cause
			// buffer overflow before the ErrBackendUnavailable error.
		default:
			// TODO: Inspect and handle fatal error.
		}
	}
}

func ExampleEncodeEventTimeAttribute() {
	ctx := context.Background()
	const topic = "projects/my-project/locations/region-or-zone/topics/my-topic"
	publisher, err := pscompat.NewPublisherClient(ctx, topic)
	if err != nil {
		// TODO: Handle error.
	}
	defer publisher.Stop()

	v, err := pscompat.EncodeEventTimeAttribute(&timestamppb.Timestamp{
		Seconds: 1672531200,
		Nanos:   500000000,
	})
	if err != nil {
		// TODO: Handle error.
	}

	r := publisher.Publish(ctx, &pubsub.Message{
		Data: []byte("hello world"),
		Attributes: map[string]string{
			pscompat.EventTimeAttributeKey: v,
		},
	})
	_, err = r.Get(ctx)
	if err != nil {
		// TODO: Handle error.
	}
}

func ExampleDecodeEventTimeAttribute() {
	ctx := context.Background()
	const subscription = "projects/my-project/locations/region-or-zone/subscriptions/my-subscription"
	subscriber, err := pscompat.NewSubscriberClient(ctx, subscription)
	if err != nil {
		// TODO: Handle error.
	}
	cctx, cancel := context.WithCancel(ctx)
	err = subscriber.Receive(cctx, func(ctx context.Context, m *pubsub.Message) {
		m.Ack()
		if v, ok := m.Attributes[pscompat.EventTimeAttributeKey]; ok {
			eventTime, err := pscompat.DecodeEventTimeAttribute(v)
			if err != nil {
				// TODO: Handle error.
			}
			fmt.Printf("Received message with event time: %v\n", eventTime)
		}
	})
	if err != nil {
		// TODO: Handle error.
	}

	// Call cancel from the receiver callback or another goroutine to stop
	// receiving.
	cancel()
}

func ExampleSubscriberClient_Receive() {
	ctx := context.Background()
	const subscription = "projects/my-project/locations/region-or-zone/subscriptions/my-subscription"
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

	// Call cancel from the receiver callback or another goroutine to stop
	// receiving.
	cancel()
}

// If the application has a low tolerance to backend unavailability, set a lower
// ReceiveSettings.Timeout value to detect and alert.
func ExampleSubscriberClient_Receive_errorHandling() {
	ctx := context.Background()
	const subscription = "projects/my-project/locations/region-or-zone/subscriptions/my-subscription"
	settings := pscompat.ReceiveSettings{
		// The SubscriberClient will terminate when it cannot connect to backends
		// for more than 5 minutes.
		Timeout: 5 * time.Minute,
	}
	subscriber, err := pscompat.NewSubscriberClientWithSettings(ctx, subscription, settings)
	if err != nil {
		// TODO: Handle error.
	}

	for {
		cctx, cancel := context.WithCancel(ctx)
		err = subscriber.Receive(cctx, func(ctx context.Context, m *pubsub.Message) {
			// TODO: Handle message.
			// NOTE: May be called concurrently; synchronize access to shared memory.
			m.Ack()
		})
		if err != nil {
			cancel()
			fmt.Printf("Subscriber client stopped receiving due to error: %v\n", err)
			if errors.Is(err, pscompat.ErrBackendUnavailable) {
				// TODO: Alert if necessary. Receive can be retried.
			} else {
				// TODO: Handle fatal error.
				break
			}
		}

		// Call cancel from the receiver callback or another goroutine to stop
		// receiving.
		cancel()
	}
}

// This example shows how to throttle SubscriberClient.Receive, which aims for
// high throughput by default. By limiting the number of messages and/or bytes
// being processed at once, you can bound your program's resource consumption.
// Note that ReceiveSettings apply per partition, so keep in mind the number of
// partitions in the associated topic.
func ExampleNewSubscriberClientWithSettings_maxOutstanding() {
	ctx := context.Background()
	const subscription = "projects/my-project/locations/region-or-zone/subscriptions/my-subscription"
	settings := pscompat.ReceiveSettings{
		MaxOutstandingMessages: 5,
		MaxOutstandingBytes:    10e6,
	}
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
func ExampleNewSubscriberClientWithSettings_manualPartitionAssignment() {
	ctx := context.Background()
	const subscription = "projects/my-project/locations/region-or-zone/subscriptions/my-subscription"
	settings := pscompat.ReceiveSettings{
		// NOTE: The corresponding topic must have 2 or more partitions.
		Partitions: []int{0, 1},
	}
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

// This example illustrates how to declare a common interface for publisher
// clients from Cloud Pub/Sub (cloud.google.com/go/pubsub) and Pub/Sub Lite
// (cloud.google.com/go/pubsublite/pscompat).
func ExampleNewPublisherClient_interface() {
	// publisherInterface is implemented by both pscompat.PublisherClient and
	// pubsub.Topic.
	type publisherInterface interface {
		Publish(context.Context, *pubsub.Message) *pubsub.PublishResult
		Stop()
	}

	publish := func(publisher publisherInterface) {
		defer publisher.Stop()
		// TODO: Publish messages.
	}

	// Create a Pub/Sub Lite publisher client.
	ctx := context.Background()
	publisher, err := pscompat.NewPublisherClient(ctx, "projects/my-project/locations/region-or-zone/topics/my-topic")
	if err != nil {
		// TODO: Handle error.
	}
	publish(publisher)

	// Create a Cloud Pub/Sub topic to publish.
	client, err := pubsub.NewClient(ctx, "my-project")
	if err != nil {
		// TODO: Handle error.
	}
	topic := client.Topic("my-topic")
	publish(topic)
}

// This example illustrates how to declare a common interface for subscriber
// clients from Cloud Pub/Sub (cloud.google.com/go/pubsub) and Pub/Sub Lite
// (cloud.google.com/go/pubsublite/pscompat).
func ExampleNewSubscriberClient_interface() {
	// subscriberInterface is implemented by both pscompat.SubscriberClient and
	// pubsub.Subscription.
	type subscriberInterface interface {
		Receive(context.Context, func(context.Context, *pubsub.Message)) error
	}

	receive := func(subscriber subscriberInterface) {
		// TODO: Receive messages.
	}

	// Create a Pub/Sub Lite subscriber client.
	ctx := context.Background()
	subscriber, err := pscompat.NewSubscriberClient(ctx, "projects/my-project/locations/region-or-zone/subscriptions/my-subscription")
	if err != nil {
		// TODO: Handle error.
	}
	receive(subscriber)

	// Create a Cloud Pub/Sub subscription to receive.
	client, err := pubsub.NewClient(ctx, "my-project")
	if err != nil {
		// TODO: Handle error.
	}
	subscription := client.Subscription("my-subscription")
	receive(subscription)
}

func ExampleParseMessageMetadata_publisher() {
	ctx := context.Background()
	const topic = "projects/my-project/locations/region-or-zone/topics/my-topic"
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
	const subscription = "projects/my-project/locations/region-or-zone/subscriptions/my-subscription"
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

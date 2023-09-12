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

/*
Package pubsublite provides an easy way to publish and receive messages using
the Pub/Sub Lite service.

Google Pub/Sub services are designed to provide reliable, many-to-many,
asynchronous messaging between applications. Publisher applications can send
messages to a topic and other applications can subscribe to that topic to
receive the messages. By decoupling senders and receivers, Google Pub/Sub allows
developers to communicate between independently written applications.

Compared to Cloud Pub/Sub, Pub/Sub Lite provides partitioned data storage with
predefined throughput and storage capacity. Guidance on how to choose between
Cloud Pub/Sub and Pub/Sub Lite is available at
https://cloud.google.com/pubsub/docs/choosing-pubsub-or-lite.

More information about Pub/Sub Lite is available at
https://cloud.google.com/pubsub/lite.

See https://pkg.go.dev/cloud.google.com/go for authentication, timeouts,
connection pooling and similar aspects of this package.

# Introduction

Examples can be found at
https://pkg.go.dev/cloud.google.com/go/pubsublite#pkg-examples
and
https://pkg.go.dev/cloud.google.com/go/pubsublite/pscompat#pkg-examples.

Complete sample programs can be found at
https://github.com/GoogleCloudPlatform/golang-samples/tree/master/pubsublite.

The cloud.google.com/go/pubsublite/pscompat subpackage contains clients for
publishing and receiving messages, which have similar interfaces to their
pubsub.Topic and pubsub.Subscription counterparts in cloud.google.com/go/pubsub.
The following examples demonstrate how to declare common interfaces:
https://pkg.go.dev/cloud.google.com/go/pubsublite/pscompat#example-NewPublisherClient-Interface
and
https://pkg.go.dev/cloud.google.com/go/pubsublite/pscompat#example-NewSubscriberClient-Interface.

The following imports are required for code snippets below:

	import (
	  "cloud.google.com/go/pubsub"
	  "cloud.google.com/go/pubsublite"
	  "cloud.google.com/go/pubsublite/pscompat"
	)

# Creating Topics

Messages are published to topics. Pub/Sub Lite topics may be created like so:

	ctx := context.Background()
	const topicPath = "projects/my-project/locations/us-central1-c/topics/my-topic"
	topicConfig := pubsublite.TopicConfig{
	  Name:                       topicPath,
	  PartitionCount:             1,
	  PublishCapacityMiBPerSec:   4,
	  SubscribeCapacityMiBPerSec: 4,
	  PerPartitionBytes:          30 * 1024 * 1024 * 1024,  // 30 GiB
	  RetentionDuration:          pubsublite.InfiniteRetention,
	}
	adminClient, err := pubsublite.NewAdminClient(ctx, "us-central1")
	if err != nil {
	  // TODO: Handle error.
	}
	if _, err = adminClient.CreateTopic(ctx, topicConfig); err != nil {
	  // TODO: Handle error.
	}

Close must be called to release resources when an AdminClient is no longer
required.

See https://cloud.google.com/pubsub/lite/docs/topics for more information about
how Pub/Sub Lite topics are configured.

See https://cloud.google.com/pubsub/lite/docs/locations for the list of
locations where Pub/Sub Lite is available.

# Publishing

Pub/Sub Lite uses gRPC streams extensively for high throughput. For more
differences, see https://pkg.go.dev/cloud.google.com/go/pubsublite/pscompat.

To publish messages to a topic, first create a PublisherClient:

	publisher, err := pscompat.NewPublisherClient(ctx, topicPath)
	if err != nil {
	  // TODO: Handle error.
	}

Then call Publish:

	result := publisher.Publish(ctx, &pubsub.Message{Data: []byte("payload")})

Publish queues the message for publishing and returns immediately. When enough
messages have accumulated, or enough time has elapsed, the batch of messages is
sent to the Pub/Sub Lite service. Thresholds for batching can be configured in
PublishSettings.

Publish returns a PublishResult, which behaves like a future; its Get method
blocks until the message has been sent (or has failed to be sent) to the
service:

	id, err := result.Get(ctx)
	if err != nil {
	  // TODO: Handle error.
	}

Once you've finishing publishing all messages, call Stop to flush all messages
to the service and close gRPC streams. The PublisherClient can no longer be used
after it has been stopped or has terminated due to a permanent error.

	publisher.Stop()

PublisherClients are expected to be long-lived and used for the duration of the
application, rather than for publishing small batches of messages. Stop must be
called to release resources when a PublisherClient is no longer required.

See https://cloud.google.com/pubsub/lite/docs/publishing for more information
about publishing.

# Creating Subscriptions

To receive messages published to a topic, create a subscription to the topic.
There may be more than one subscription per topic; each message that is
published to the topic will be delivered to all of its subscriptions.

Pub/Sub Lite subscriptions may be created like so:

	const subscriptionPath = "projects/my-project/locations/us-central1-c/subscriptions/my-subscription"
	subscriptionConfig := pubsublite.SubscriptionConfig{
	  Name:                subscriptionPath,
	  Topic:               topicPath,
	  DeliveryRequirement: pubsublite.DeliverImmediately,
	}
	if _, err = adminClient.CreateSubscription(ctx, subscriptionConfig); err != nil {
	  // TODO: Handle error.
	}

See https://cloud.google.com/pubsub/lite/docs/subscriptions for more information
about how subscriptions are configured.

# Receiving

To receive messages for a subscription, first create a SubscriberClient:

	subscriber, err := pscompat.NewSubscriberClient(ctx, subscriptionPath)

Messages are then consumed from a subscription via callback. The callback may be
invoked concurrently by multiple goroutines (one per partition that the
subscriber client is connected to).

	cctx, cancel := context.WithCancel(ctx)
	err = subscriber.Receive(cctx, func(ctx context.Context, m *pubsub.Message) {
	  log.Printf("Got message: %s", m.Data)
	  m.Ack()
	})
	if err != nil {
	  // TODO: Handle error.
	}

Receive blocks until either the context is canceled or a permanent error occurs.
To terminate a call to Receive, cancel its context:

	cancel()

Clients must call pubsub.Message.Ack() or pubsub.Message.Nack() for every
message received. Pub/Sub Lite does not have ACK deadlines. Pub/Sub Lite also
does not actually have the concept of NACK. The default behavior terminates the
SubscriberClient. In Pub/Sub Lite, only a single subscriber for a given
subscription is connected to any partition at a time, and there is no other
client that may be able to handle messages.

See https://cloud.google.com/pubsub/lite/docs/subscribing for more information
about receiving messages.

# gRPC Connection Pools

Pub/Sub Lite utilizes gRPC streams extensively. gRPC allows a maximum of 100
streams per connection. Internally, the library uses a default connection pool
size of 8, which supports up to 800 topic partitions. To alter the connection
pool size, pass a ClientOption to pscompat.NewPublisherClient and
pscompat.NewSubscriberClient:

	pub, err := pscompat.NewPublisherClient(ctx, topicPath, option.WithGRPCConnectionPool(10))
*/
package pubsublite // import "cloud.google.com/go/pubsublite"

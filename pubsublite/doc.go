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
Google Cloud Pub/Sub Lite.

Google Cloud Pub/Sub is designed to provide reliable, many-to-many, asynchronous
messaging between applications. Publisher applications can send messages to a
topic and other applications can subscribe to that topic to receive the
messages. By decoupling senders and receivers, Google Cloud Pub/Sub allows
developers to communicate between independently written applications.

Compared to Google Cloud Pub/Sub, Pub/Sub Lite provides partitioned zonal data
storage with predefined throughput and storage capacity. Guidance on how to
choose between Google Cloud Pub/Sub and Pub/Sub Lite is available at
https://cloud.google.com/pubsub/docs/choosing-pubsub-or-lite.

More information about Google Cloud Pub/Sub Lite is available at
https://cloud.google.com/pubsub/lite.

See https://godoc.org/cloud.google.com/go for authentication, timeouts,
connection pooling and similar aspects of this package.

Note: This library is in ALPHA. Backwards-incompatible changes may be made
before stable v1.0.0 is released.


Creating Topics

Messages are published to topics. Cloud Pub/Sub Lite topics may be created like
so (error handling omitted for brevity):

  topicPath := pubsublite.TopicPath{
    Project: "project-id",
    Zone:    "zone",
    TopicID: "topic-id",
  }
  topicConfig := pubsublite.TopicConfig{
    Name:                       topicPath,
    PartitionCount:             1,
    PublishCapacityMiBPerSec:   4,
    SubscribeCapacityMiBPerSec: 4,
    PerPartitionBytes:          30 * 1024 * 1024 * 1024,  // 30 GiB
    RetentionDuration:          pubsublite.InfiniteRetention,
  }

  region, err := pubsublite.ZoneToRegion(topicPath.Zone)
  adminClient, err := pubsublite.NewAdminClient(ctx, region)
  _, err = adminClient.CreateTopic(ctx, topicConfig)

See https://cloud.google.com/pubsub/lite/docs/topics for more information about
how Cloud Pub/Sub Lite topics are configured.


Publishing

The pubsublite/ps subpackage contains clients for publishing and receiving
messages, which have similar interfaces to their Topic and Subscription
counterparts in the pubsub package. Cloud Pub/Sub Lite uses gRPC streams
extensively for high throughput. For more differences, see
https://godoc.org/cloud.google.com/go/pubsublite/ps.

To publish messages to a topic, first create a PublisherClient:

  publisher, err := ps.NewPublisherClient(ctx, ps.DefaultPublishSettings, topicPath)

Then call Publish:

  // Note: result is a pubsub.PublishResult
  result := publisher.Publish(ctx, &pubsub.Message{Data: []byte("payload")})

Publish queues the message for publishing and returns immediately. When enough
messages have accumulated, or enough time has elapsed, the batch of messages is
sent to the Cloud Pub/Sub Lite service. Thresholds for batching can be
configured in PublishSettings.

Publish returns a PublishResult, which behaves like a future: its Get method
blocks until the message has been sent (or has failed to be sent) to the
service. Once you've finishing publishing, call Stop to flush all messages to
the service and close gRPC streams:

  publisher.Stop()

See https://cloud.google.com/pubsub/lite/docs/publishing for more information
about publishing.


Creating Subscriptions

To receive messages published to a topic, clients create subscriptions to the
topic. There may be more than one subscription per topic; each message that is
published to the topic will be delivered to all of its subscriptions.

Cloud Pub/Sub Lite subscriptions may be created like so:

  subscriptionPath := pubsublite.SubscriptionPath{
    Project:        "project-id",
    Zone:           "zone",
    SubscriptionID: "subscription-id",
  }
  subscriptionConfig := pubsublite.SubscriptionConfig{
    Name:                subscriptionPath,
    Topic:               topicPath,
    DeliveryRequirement: pubsublite.DeliverImmediately,
  }
  _, err = admin.CreateSubscription(ctx, subscriptionConfig)

See https://cloud.google.com/pubsub/lite/docs/subscriptions for more information
about how subscriptions are configured.


Receiving

To receive messages for a subscription, first create a SubscriberClient:

  subscriber, err := ps.NewSubscriberClient(ctx, ps.DefaultReceiveSettings, subscriptionPath)

Messages are then consumed from a subscription via callback.

  cctx, cancel := context.WithCancel(ctx)
  err = subscriber.Receive(cctx, func(ctx context.Context, m *pubsub.Message) {
    log.Printf("Got message: %s", m.Data)
    m.Ack()
  })
  if err != nil {
    // Handle error.
  }

The callback may be invoked concurrently by multiple goroutines (one per
partition that the subscriber client is connected to). To terminate a call to
Receive, cancel its context:

  cancel()

Clients must call pubsub.Message.Ack() or pubsub.Message.Nack() for every
message received. Cloud Pub/Sub Lite does not have ACK deadlines. Cloud Pub/Sub
Lite also does not actually have the concept of NACK. The default behavior
terminates the SubscriberClient. In Cloud Pub/Sub Lite, only a single subscriber
for a given subscription is connected to any partition at a time, and there is
no other client that may be able to handle messages.

See https://cloud.google.com/pubsub/lite/docs/subscribing for more information
about receiving messages.
*/
package pubsublite // import "cloud.google.com/go/pubsublite"

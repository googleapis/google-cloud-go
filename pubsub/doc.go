// Copyright 2016 Google LLC
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

/*
Package pubsub provides an easy way to publish and receive Google Cloud Pub/Sub
messages, hiding the details of the underlying server RPCs.
Pub/Sub is a many-to-many, asynchronous messaging system that decouples senders
and receivers.

More information about Pub/Sub is available at
https://cloud.google.com/pubsub/docs

See https://godoc.org/cloud.google.com/go for authentication, timeouts,
connection pooling and similar aspects of this package.

# Publishing

Pub/Sub messages are published to topics. A [Topic] may be created
using [Client.CreateTopic] like so:

	topic, err := pubsubClient.CreateTopic(context.Background(), "topic-name")

Messages may then be published to a [Topic]:

	res := topic.Publish(ctx, &pubsub.Message{Data: []byte("payload")})

[Topic.Publish] queues the message for publishing and returns immediately. When enough
messages have accumulated, or enough time has elapsed, the batch of messages is
sent to the Pub/Sub service.

[Topic.Publish] returns a [PublishResult], which behaves like a future: its Get method
blocks until the message has been sent to the service.

The first time you call [Topic.Publish] on a [Topic], goroutines are started in the
background. To clean up these goroutines, call [Topic.Stop]:

	topic.Stop()

# Receiving

To receive messages published to a [Topic], clients create a [Subscription]
for the topic. There may be more than one subscription per topic ; each message
that is published to the topic will be delivered to all associated subscriptions.

A [Subscription] may be created like so:

	 sub, err := pubsubClient.CreateSubscription(context.Background(), "sub-name",
		pubsub.SubscriptionConfig{Topic: topic})

Messages are then consumed from a [Subscription] via callback.

	 err := sub.Receive(context.Background(), func(ctx context.Context, m *Message) {
		log.Printf("Got message: %s", m.Data)
		m.Ack()
	 })
	 if err != nil && !errors.Is(err, context.Canceled) {
		// Handle error.
	 }

The callback is invoked concurrently by multiple goroutines, maximizing
throughput. To terminate a call to [Subscription.Receive], cancel its context.

Once client code has processed the [Message], it must call Message.Ack or
Message.Nack; otherwise the Message will eventually be redelivered. Ack/Nack
MUST be called within the [Subscription.Receive] handler function, and not from a goroutine.
Otherwise, flow control (e.g. ReceiveSettings.MaxOutstandingMessages) will
not be respected, and messages can get orphaned when cancelling Receive.

If the client cannot or doesn't want to process the message, it can call Message.Nack
to speed redelivery. For more information and configuration options, see
Ack Deadlines below.

Note: It is possible for a [Message] to be redelivered even if Message.Ack has
been called. Client code must be robust to multiple deliveries of messages.

Note: This uses pubsub's streaming pull feature. This feature has properties that
may be surprising. Please take a look at https://cloud.google.com/pubsub/docs/pull#streamingpull
for more details on how streaming pull behaves compared to the synchronous
pull method.

# Streams Management

The number of StreamingPull connections can be configured by setting NumGoroutines in [ReceiveSettings].
The default value of 10 means the client library will maintain 10 StreamingPull connections.
This is more than sufficient for most use cases, as StreamingPull connections can handle up to
10 MB/s https://cloud.google.com/pubsub/quotas#resource_limits. In some cases, using too many streams
can lead to client library behaving poorly as the application becomes I/O bound.

By default, the number of connections in the gRPC conn pool is min(4,GOMAXPROCS). Each connection supports
up to 100 streams. Thus, if you have 4 or more CPU cores, the default setting allows a maximum of 400 streams
which is already excessive for most use cases.
If you want to change the limits on the number of streams, you can change the number of connections
in the gRPC connection pool as shown below:

	 opts := []option.ClientOption{
		option.WithGRPCConnectionPool(2),
	 }
	 client, err := pubsub.NewClient(ctx, projID, opts...)

# Ack Deadlines

The default pubsub deadlines are suitable for most use cases, but may be
overridden. This section describes the tradeoffs that should be considered
when overriding the defaults.

Behind the scenes, each message returned by the Pub/Sub server has an
associated lease, known as an "ack deadline". Unless a message is
acknowledged within the ack deadline, or the client requests that
the ack deadline be extended, the message will become eligible for redelivery.

As a convenience, the pubsub client will automatically extend deadlines until
either:
  - Message.Ack or Message.Nack is called, or
  - The "MaxExtension" duration elapses from the time the message is fetched from
    the server. This defaults to 60m.

Ack deadlines are extended periodically by the client. The period between extensions,
as well as the length of the extension, automatically adjusts based on the time it takes the
subscriber application to ack messages (based on the 99th percentile of ack latency).
By default, this extension period is capped at 10m, but this limit can be configured
by the "MaxExtensionPeriod" setting. This has the effect that subscribers that process
messages quickly have their message ack deadlines extended for a short amount, whereas
subscribers that process message slowly have their message ack deadlines extended
for a large amount. The net effect is fewer RPCs sent from the client library.

For example, consider a subscriber that takes 3 minutes to process each message.
Since the library has already recorded several 3-minute "ack latencies"s in a
percentile distribution, future message extensions are sent with a value of 3
minutes, every 3 minutes. Suppose the application crashes 5 seconds after the
library sends such an extension: the Pub/Sub server would wait the remaining
2m55s before re-sending the messages out to other subscribers.

Please note that by default, the client library does not use the subscription's
AckDeadline for the MaxExtension value.

# Slow Message Processing

For use cases where message processing exceeds 30 minutes, we recommend using
the base client in a pull model, since long-lived streams are periodically killed
by firewalls. See the example at https://godoc.org/cloud.google.com/go/pubsub/apiv1#example-SubscriberClient-Pull-LengthyClientProcessing

# Emulator

To use an emulator with this library, you can set the PUBSUB_EMULATOR_HOST
environment variable to the address at which your emulator is running. This will
send requests to that address instead of to Pub/Sub. You can then create
and use a client as usual:

	// Set PUBSUB_EMULATOR_HOST environment variable.
	err := os.Setenv("PUBSUB_EMULATOR_HOST", "localhost:9000")
	if err != nil {
		// TODO: Handle error.
	}
	// Create client as usual.
	client, err := pubsub.NewClient(ctx, "my-project-id")
	if err != nil {
		// TODO: Handle error.
	}
	defer client.Close()

Deprecated: Please use cloud.google.com/go/pubsub/v2.
*/
package pubsub // import "cloud.google.com/go/pubsub"

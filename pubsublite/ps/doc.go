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
Package ps contains clients for publishing and subscribing using the Google
Cloud Pub/Sub Lite service.

If interfaces are defined, PublisherClient and SubscriberClient can be used as
substitutions for pubsub.Topic.Publish() and pubsub.Subscription.Receive(),
respectively, from the pubsub package.

As noted in comments, the two services have some differences:
  - Pub/Sub Lite does not support NACK for messages. By default, this will
    terminate the SubscriberClient. A custom function can be provided for
    ReceiveSettings.NackHandler to handle NACKed messages.
  - Pub/Sub Lite has no concept of ack deadlines. Subscribers must ACK or NACK
    every message received and can take as much time as they need to process the
    message.
  - Pub/Sub Lite PublisherClients and SubscriberClients can terminate when an
    unretryable error occurs.
  - Publishers and subscribers will be throttled if Pub/Sub Lite publish or
    subscribe throughput limits are exceeded. Thus publishing can be more
    sensitive to buffer overflow than Cloud Pub/Sub.
  - Pub/Sub Lite utilizes bidirectional gRPC streams extensively to maximize
    publish and subscribe throughput.

More information about Google Cloud Pub/Sub Lite is available at
https://cloud.google.com/pubsub/lite.

Information about choosing between Google Cloud Pub/Sub vs Pub/Sub Lite is
available at https://cloud.google.com/pubsub/docs/choosing-pubsub-or-lite.

See https://godoc.org/cloud.google.com/go for authentication, timeouts,
connection pooling and similar aspects of this package.
*/
package ps // import "cloud.google.com/go/pubsublite/ps"

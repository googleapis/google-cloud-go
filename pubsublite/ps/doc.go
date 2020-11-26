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
Package ps contains a publisher and subscriber client for the Cloud Pub/Sub Lite
service that emulates the Cloud Pub/Sub API.

If interfaces are defined for pubsub.Topic.Publish() and
pubsub.Subscription.Receive(), the clients in this package can be used as
drop-in replacements. As noted in comments, the two services have some
differences:

- Pub/Sub Lite does not support nack for messages.
- Pub/Sub Lite has publish and subscribe throughput limits. Thus publishing can
  be more sensitive to buffer overflow.
- Pub/Sub Lite publisher clients can terminate when an unretryable error occurs.
- DefaultPublishSettings and DefaultReceiveSettings should be used for default
  settings rather than their empty types.

For more information about Cloud Pub/Sub Lite, see
https://cloud.google.com/pubsub/lite/docs.
*/
package ps // import "cloud.google.com/go/pubsublite/ps"

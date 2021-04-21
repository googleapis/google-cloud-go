// Copyright 2021 Google LLC
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

// [START pubsub_generated_pubsublite_pscompat_ParseMessageMetadata_subscriber]

package main

import (
	"context"
	"fmt"

	"cloud.google.com/go/pubsub"
	"cloud.google.com/go/pubsublite/pscompat"
)

func main() {
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

// [END pubsub_generated_pubsublite_pscompat_ParseMessageMetadata_subscriber]

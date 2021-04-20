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

// [START pubsub_generated_pubsublite_pscompat_ParseMessageMetadata_publisher]

package main

import (
	"context"
	"fmt"

	"cloud.google.com/go/pubsub"
	"cloud.google.com/go/pubsublite/pscompat"
)

func main() {
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

// [END pubsub_generated_pubsublite_pscompat_ParseMessageMetadata_publisher]

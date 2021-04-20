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

// [START pubsub_generated_pubsublite_pscompat_PublisherClient_Publish]

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

// [END pubsub_generated_pubsublite_pscompat_PublisherClient_Publish]

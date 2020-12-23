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

package ps_test

import (
	"context"
	"fmt"

	"cloud.google.com/go/pubsub"
	"cloud.google.com/go/pubsublite"
	"cloud.google.com/go/pubsublite/ps"
)

func ExamplePublisherClient_Publish() {
	ctx := context.Background()
	topic := pubsublite.TopicPath{Project: "project-id", Zone: "zone", TopicID: "topic-id"}
	publisher, err := ps.NewPublisherClient(ctx, ps.DefaultPublishSettings, topic)
	if err != nil {
		// TODO: Handle error.
	}
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

func ExamplePublisherClient_Error() {
	ctx := context.Background()
	topic := pubsublite.TopicPath{Project: "project-id", Zone: "zone", TopicID: "topic-id"}
	publisher, err := ps.NewPublisherClient(ctx, ps.DefaultPublishSettings, topic)
	if err != nil {
		// TODO: Handle error.
	}
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
			if err == ps.ErrPublisherStopped {
				fmt.Printf("Publisher client stopped due to error: %v\n", publisher.Error())
			}
		}
		fmt.Printf("Published a message with a message ID: %s\n", id)
	}
}

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

// [START pubsub_generated_pubsub_Client_CreateSubscription_neverExpire]

package main

import (
	"context"
	"time"

	"cloud.google.com/go/pubsub"
)

func main() {
	ctx := context.Background()
	client, err := pubsub.NewClient(ctx, "project-id")
	if err != nil {
		// TODO: Handle error.
	}

	// Create a new topic with the given name.
	topic, err := client.CreateTopic(ctx, "topicName")
	if err != nil {
		// TODO: Handle error.
	}

	// Create a new subscription to the previously
	// created topic and ensure it never expires.
	sub, err := client.CreateSubscription(ctx, "subName", pubsub.SubscriptionConfig{
		Topic:            topic,
		AckDeadline:      10 * time.Second,
		ExpirationPolicy: time.Duration(0),
	})
	if err != nil {
		// TODO: Handle error.
	}
	_ = sub // TODO: Use the subscription
}

// [END pubsub_generated_pubsub_Client_CreateSubscription_neverExpire]

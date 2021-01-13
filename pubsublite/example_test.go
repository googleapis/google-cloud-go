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

package pubsublite_test

import (
	"context"
	"fmt"
	"time"

	"cloud.google.com/go/pubsublite"
	"google.golang.org/api/iterator"
)

func ExampleAdminClient_CreateTopic() {
	ctx := context.Background()
	admin, err := pubsublite.NewAdminClient(ctx, "region")
	if err != nil {
		// TODO: Handle error.
	}

	const gib = 1 << 30
	topicConfig := pubsublite.TopicConfig{
		Name: pubsublite.TopicPath{
			Project: "project-id",
			Zone:    "zone",
			TopicID: "topic-id",
		},
		PartitionCount:             2,        // Must be at least 1.
		PublishCapacityMiBPerSec:   4,        // Must be 4-16 MiB/s.
		SubscribeCapacityMiBPerSec: 8,        // Must be 4-32 MiB/s.
		PerPartitionBytes:          30 * gib, // Must be 30 GiB-10 TiB.
		// Retain messages indefinitely as long as there is available storage.
		RetentionDuration: pubsublite.InfiniteRetention,
	}
	_, err = admin.CreateTopic(ctx, topicConfig)
	if err != nil {
		// TODO: Handle error.
	}
}

func ExampleAdminClient_UpdateTopic() {
	ctx := context.Background()
	admin, err := pubsublite.NewAdminClient(ctx, "region")
	if err != nil {
		// TODO: Handle error.
	}

	updateConfig := pubsublite.TopicConfigToUpdate{
		Name: pubsublite.TopicPath{
			Project: "project-id",
			Zone:    "zone",
			TopicID: "topic-id",
		},
		PublishCapacityMiBPerSec:   8,
		SubscribeCapacityMiBPerSec: 16,
		// Garbage collect messages older than 24 hours.
		RetentionDuration: 24 * time.Hour,
	}
	_, err = admin.UpdateTopic(ctx, updateConfig)
	if err != nil {
		// TODO: Handle error.
	}
}

func ExampleAdminClient_DeleteTopic() {
	ctx := context.Background()
	admin, err := pubsublite.NewAdminClient(ctx, "region")
	if err != nil {
		// TODO: Handle error.
	}

	topic := pubsublite.TopicPath{
		Project: "project-id",
		Zone:    "zone",
		TopicID: "topic-id",
	}
	if err := admin.DeleteTopic(ctx, topic); err != nil {
		// TODO: Handle error.
	}
}

func ExampleAdminClient_Topics() {
	ctx := context.Background()
	admin, err := pubsublite.NewAdminClient(ctx, "region")
	if err != nil {
		// TODO: Handle error.
	}

	// List the configs of all topics in the given zone for the project.
	location := pubsublite.LocationPath{Project: "project-id", Zone: "zone"}
	it := admin.Topics(ctx, location)
	for {
		topicConfig, err := it.Next()
		if err == iterator.Done {
			break
		}
		if err != nil {
			// TODO: Handle error.
		}
		fmt.Println(topicConfig)
	}
}

func ExampleAdminClient_TopicSubscriptions() {
	ctx := context.Background()
	admin, err := pubsublite.NewAdminClient(ctx, "region")
	if err != nil {
		// TODO: Handle error.
	}

	// List the paths of all subscriptions of a topic.
	topic := pubsublite.TopicPath{
		Project: "project-id",
		Zone:    "zone",
		TopicID: "topic-id",
	}
	it := admin.TopicSubscriptions(ctx, topic)
	for {
		subscriptionPath, err := it.Next()
		if err == iterator.Done {
			break
		}
		if err != nil {
			// TODO: Handle error.
		}
		fmt.Println(subscriptionPath)
	}
}

func ExampleAdminClient_CreateSubscription() {
	ctx := context.Background()
	admin, err := pubsublite.NewAdminClient(ctx, "region")
	if err != nil {
		// TODO: Handle error.
	}

	subscriptionConfig := pubsublite.SubscriptionConfig{
		Name: pubsublite.SubscriptionPath{
			Project:        "project-id",
			Zone:           "zone",
			SubscriptionID: "subscription-id",
		},
		Topic: pubsublite.TopicPath{
			Project: "project-id",
			Zone:    "zone",
			TopicID: "topic-id",
		},
		// Do not wait for a published message to be successfully written to storage
		// before delivering it to subscribers.
		DeliveryRequirement: pubsublite.DeliverImmediately,
	}
	_, err = admin.CreateSubscription(ctx, subscriptionConfig)
	if err != nil {
		// TODO: Handle error.
	}
}

func ExampleAdminClient_UpdateSubscription() {
	ctx := context.Background()
	admin, err := pubsublite.NewAdminClient(ctx, "region")
	if err != nil {
		// TODO: Handle error.
	}

	updateConfig := pubsublite.SubscriptionConfigToUpdate{
		Name: pubsublite.SubscriptionPath{
			Project:        "project-id",
			Zone:           "zone",
			SubscriptionID: "subscription-id",
		},
		// Deliver a published message to subscribers after it has been successfully
		// written to storage.
		DeliveryRequirement: pubsublite.DeliverAfterStored,
	}
	_, err = admin.UpdateSubscription(ctx, updateConfig)
	if err != nil {
		// TODO: Handle error.
	}
}

func ExampleAdminClient_DeleteSubscription() {
	ctx := context.Background()
	admin, err := pubsublite.NewAdminClient(ctx, "region")
	if err != nil {
		// TODO: Handle error.
	}

	subscription := pubsublite.SubscriptionPath{
		Project:        "project-id",
		Zone:           "zone",
		SubscriptionID: "subscription-id",
	}
	if err := admin.DeleteSubscription(ctx, subscription); err != nil {
		// TODO: Handle error.
	}
}

func ExampleAdminClient_Subscriptions() {
	ctx := context.Background()
	admin, err := pubsublite.NewAdminClient(ctx, "region")
	if err != nil {
		// TODO: Handle error.
	}

	// List the configs of all subscriptions in the given zone for the project.
	location := pubsublite.LocationPath{Project: "project-id", Zone: "zone"}
	it := admin.Subscriptions(ctx, location)
	for {
		subscriptionConfig, err := it.Next()
		if err == iterator.Done {
			break
		}
		if err != nil {
			// TODO: Handle error.
		}
		fmt.Println(subscriptionConfig)
	}
}

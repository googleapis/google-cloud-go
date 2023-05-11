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

// This example demonstrates how to create a new topic. Topics may be regional
// or zonal. See https://cloud.google.com/pubsub/lite/docs/topics for more
// information about how Pub/Sub Lite topics are configured.
// See https://cloud.google.com/pubsub/lite/docs/locations for the list of
// regions and zones where Pub/Sub Lite is available.
func ExampleAdminClient_CreateTopic() {
	ctx := context.Background()
	// NOTE: resources must be located within this region.
	admin, err := pubsublite.NewAdminClient(ctx, "region")
	if err != nil {
		// TODO: Handle error.
	}
	defer admin.Close()

	const gib = 1 << 30
	topicConfig := pubsublite.TopicConfig{
		Name:                       "projects/my-project/locations/region-or-zone/topics/my-topic",
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
	// NOTE: resources must be located within this region.
	admin, err := pubsublite.NewAdminClient(ctx, "region")
	if err != nil {
		// TODO: Handle error.
	}
	defer admin.Close()

	updateConfig := pubsublite.TopicConfigToUpdate{
		Name:                       "projects/my-project/locations/region-or-zone/topics/my-topic",
		PartitionCount:             3, // Only increases currently supported.
		PublishCapacityMiBPerSec:   8,
		SubscribeCapacityMiBPerSec: 16,
		RetentionDuration:          24 * time.Hour, // Garbage collect messages older than 24 hours.
	}
	_, err = admin.UpdateTopic(ctx, updateConfig)
	if err != nil {
		// TODO: Handle error.
	}
}

func ExampleAdminClient_DeleteTopic() {
	ctx := context.Background()
	// NOTE: resources must be located within this region.
	admin, err := pubsublite.NewAdminClient(ctx, "region")
	if err != nil {
		// TODO: Handle error.
	}
	defer admin.Close()

	const topic = "projects/my-project/locations/region-or-zone/topics/my-topic"
	if err := admin.DeleteTopic(ctx, topic); err != nil {
		// TODO: Handle error.
	}
}

func ExampleAdminClient_Topics() {
	ctx := context.Background()
	// NOTE: resources must be located within this region.
	admin, err := pubsublite.NewAdminClient(ctx, "region")
	if err != nil {
		// TODO: Handle error.
	}
	defer admin.Close()

	// List the configs of all topics in the given region or zone for the project.
	it := admin.Topics(ctx, "projects/my-project/locations/region-or-zone")
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
	// NOTE: resources must be located within this region.
	admin, err := pubsublite.NewAdminClient(ctx, "region")
	if err != nil {
		// TODO: Handle error.
	}
	defer admin.Close()

	// List the paths of all subscriptions of a topic.
	const topic = "projects/my-project/locations/region-or-zone/topics/my-topic"
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

// This example demonstrates how to create a new subscription for a topic.
// See https://cloud.google.com/pubsub/lite/docs/subscriptions for more
// information about how subscriptions are configured.
func ExampleAdminClient_CreateSubscription() {
	ctx := context.Background()
	// NOTE: resources must be located within this region.
	admin, err := pubsublite.NewAdminClient(ctx, "region")
	if err != nil {
		// TODO: Handle error.
	}
	defer admin.Close()

	subscriptionConfig := pubsublite.SubscriptionConfig{
		Name:  "projects/my-project/locations/region-or-zone/subscriptions/my-subscription",
		Topic: "projects/my-project/locations/region-or-zone/topics/my-topic",
		// Do not wait for a published message to be successfully written to storage
		// before delivering it to subscribers.
		DeliveryRequirement: pubsublite.DeliverImmediately,
	}
	_, err = admin.CreateSubscription(ctx, subscriptionConfig)
	if err != nil {
		// TODO: Handle error.
	}
}

// This example demonstrates how to create a new subscription initialized to a
// specified target location within the message backlog. The target location can
// be a BacklogLocation, PublishTime or EventTime.
func ExampleAdminClient_CreateSubscription_atTargetLocation() {
	ctx := context.Background()
	// NOTE: resources must be located within this region.
	admin, err := pubsublite.NewAdminClient(ctx, "region")
	if err != nil {
		// TODO: Handle error.
	}
	defer admin.Close()

	subscriptionConfig := pubsublite.SubscriptionConfig{
		Name:  "projects/my-project/locations/region-or-zone/subscriptions/my-subscription",
		Topic: "projects/my-project/locations/region-or-zone/topics/my-topic",
		// Do not wait for a published message to be successfully written to storage
		// before delivering it to subscribers.
		DeliveryRequirement: pubsublite.DeliverImmediately,
	}
	// Initialize the subscription to the oldest retained messages for each
	// partition.
	targetLocation := pubsublite.AtTargetLocation(pubsublite.Beginning)
	_, err = admin.CreateSubscription(ctx, subscriptionConfig, targetLocation)
	if err != nil {
		// TODO: Handle error.
	}
}

// This example demonstrates how to create a new subscription that exports
// messages to a Pub/Sub topic.
// See https://cloud.google.com/pubsub/lite/docs/export-pubsub for more
// information about how export subscriptions are configured.
func ExampleAdminClient_CreateSubscription_exportToPubSub() {
	ctx := context.Background()
	// NOTE: resources must be located within this region.
	admin, err := pubsublite.NewAdminClient(ctx, "region")
	if err != nil {
		// TODO: Handle error.
	}
	defer admin.Close()

	subscriptionConfig := pubsublite.SubscriptionConfig{
		Name:  "projects/my-project/locations/region-or-zone/subscriptions/my-subscription",
		Topic: "projects/my-project/locations/region-or-zone/topics/my-topic",
		// Deliver a published message to subscribers after it has been successfully
		// written to storage.
		DeliveryRequirement: pubsublite.DeliverAfterStored,
		ExportConfig: &pubsublite.ExportConfig{
			DesiredState: pubsublite.ExportActive,
			// Configure an export subscription to a Pub/Sub topic.
			Destination: &pubsublite.PubSubDestinationConfig{
				Topic: "projects/my-project/topics/destination-pubsub-topic",
			},
			// Optional Lite topic to receive messages that cannot be exported to the
			// destination.
			DeadLetterTopic: "projects/my-project/locations/region-or-zone/topics/dead-letter-topic",
		},
	}
	_, err = admin.CreateSubscription(ctx, subscriptionConfig)
	if err != nil {
		// TODO: Handle error.
	}
}

func ExampleAdminClient_UpdateSubscription() {
	ctx := context.Background()
	// NOTE: resources must be located within this region.
	admin, err := pubsublite.NewAdminClient(ctx, "region")
	if err != nil {
		// TODO: Handle error.
	}
	defer admin.Close()

	updateConfig := pubsublite.SubscriptionConfigToUpdate{
		Name: "projects/my-project/locations/region-or-zone/subscriptions/my-subscription",
		// Deliver a published message to subscribers after it has been successfully
		// written to storage.
		DeliveryRequirement: pubsublite.DeliverAfterStored,
	}
	_, err = admin.UpdateSubscription(ctx, updateConfig)
	if err != nil {
		// TODO: Handle error.
	}
}

func ExampleAdminClient_SeekSubscription() {
	ctx := context.Background()
	// NOTE: resources must be located within this region.
	admin, err := pubsublite.NewAdminClient(ctx, "region")
	if err != nil {
		// TODO: Handle error.
	}
	defer admin.Close()

	const subscription = "projects/my-project/locations/region-or-zone/subscriptions/my-subscription"
	seekOp, err := admin.SeekSubscription(ctx, subscription, pubsublite.Beginning)
	if err != nil {
		// TODO: Handle error.
	}

	// Optional: Wait for the seek operation to complete, which indicates when
	// subscribers for all partitions are receiving messages from the seek target.
	_, err = seekOp.Wait(ctx)
	if err != nil {
		// TODO: Handle error.
	}
	metadata, err := seekOp.Metadata()
	if err != nil {
		// TODO: Handle error.
	}
	fmt.Println(metadata)
}

func ExampleAdminClient_DeleteSubscription() {
	ctx := context.Background()
	// NOTE: resources must be located within this region.
	admin, err := pubsublite.NewAdminClient(ctx, "region")
	if err != nil {
		// TODO: Handle error.
	}
	defer admin.Close()

	const subscription = "projects/my-project/locations/region-or-zone/subscriptions/my-subscription"
	if err := admin.DeleteSubscription(ctx, subscription); err != nil {
		// TODO: Handle error.
	}
}

func ExampleAdminClient_Subscriptions() {
	ctx := context.Background()
	// NOTE: resources must be located within this region.
	admin, err := pubsublite.NewAdminClient(ctx, "region")
	if err != nil {
		// TODO: Handle error.
	}
	defer admin.Close()

	// List the configs of all subscriptions in the given region or zone for the project.
	it := admin.Subscriptions(ctx, "projects/my-project/locations/region-or-zone")
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

// This example demonstrates how to create a new reservation.
// See https://cloud.google.com/pubsub/lite/docs/locations for the list of
// regions where Pub/Sub Lite is available.
func ExampleAdminClient_CreateReservation() {
	ctx := context.Background()
	admin, err := pubsublite.NewAdminClient(ctx, "region")
	if err != nil {
		// TODO: Handle error.
	}
	defer admin.Close()

	reservationConfig := pubsublite.ReservationConfig{
		Name:               "projects/my-project/locations/region/reservations/my-reservation",
		ThroughputCapacity: 10,
	}
	_, err = admin.CreateReservation(ctx, reservationConfig)
	if err != nil {
		// TODO: Handle error.
	}
}

func ExampleAdminClient_UpdateReservation() {
	ctx := context.Background()
	admin, err := pubsublite.NewAdminClient(ctx, "region")
	if err != nil {
		// TODO: Handle error.
	}
	defer admin.Close()

	updateConfig := pubsublite.ReservationConfigToUpdate{
		Name:               "projects/my-project/locations/region/reservations/my-reservation",
		ThroughputCapacity: 20,
	}
	_, err = admin.UpdateReservation(ctx, updateConfig)
	if err != nil {
		// TODO: Handle error.
	}
}

func ExampleAdminClient_DeleteReservation() {
	ctx := context.Background()
	admin, err := pubsublite.NewAdminClient(ctx, "region")
	if err != nil {
		// TODO: Handle error.
	}
	defer admin.Close()

	const reservation = "projects/my-project/locations/region/reservations/my-reservation"
	if err := admin.DeleteReservation(ctx, reservation); err != nil {
		// TODO: Handle error.
	}
}

func ExampleAdminClient_Reservations() {
	ctx := context.Background()
	admin, err := pubsublite.NewAdminClient(ctx, "region")
	if err != nil {
		// TODO: Handle error.
	}
	defer admin.Close()

	// List the configs of all reservations in the given region for the project.
	it := admin.Reservations(ctx, "projects/my-project/locations/region")
	for {
		reservation, err := it.Next()
		if err == iterator.Done {
			break
		}
		if err != nil {
			// TODO: Handle error.
		}
		fmt.Println(reservation)
	}
}

func ExampleAdminClient_ReservationTopics() {
	ctx := context.Background()
	admin, err := pubsublite.NewAdminClient(ctx, "region")
	if err != nil {
		// TODO: Handle error.
	}
	defer admin.Close()

	// List the paths of all topics using a reservation.
	const reservation = "projects/my-project/locations/region/reservations/my-reservation"
	it := admin.ReservationTopics(ctx, reservation)
	for {
		topicPath, err := it.Next()
		if err == iterator.Done {
			break
		}
		if err != nil {
			// TODO: Handle error.
		}
		fmt.Println(topicPath)
	}
}

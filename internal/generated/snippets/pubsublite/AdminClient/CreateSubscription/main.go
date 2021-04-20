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

// [START pubsub_generated_pubsublite_AdminClient_CreateSubscription]

package main

import (
	"context"

	"cloud.google.com/go/pubsublite"
)

func main() {
	ctx := context.Background()
	// NOTE: region must correspond to the zone of the topic and subscription.
	admin, err := pubsublite.NewAdminClient(ctx, "region")
	if err != nil {
		// TODO: Handle error.
	}

	subscriptionConfig := pubsublite.SubscriptionConfig{
		Name:  "projects/my-project/locations/zone/subscriptions/my-subscription",
		Topic: "projects/my-project/locations/zone/topics/my-topic",
		// Do not wait for a published message to be successfully written to storage
		// before delivering it to subscribers.
		DeliveryRequirement: pubsublite.DeliverImmediately,
	}
	_, err = admin.CreateSubscription(ctx, subscriptionConfig)
	if err != nil {
		// TODO: Handle error.
	}
}

// [END pubsub_generated_pubsublite_AdminClient_CreateSubscription]

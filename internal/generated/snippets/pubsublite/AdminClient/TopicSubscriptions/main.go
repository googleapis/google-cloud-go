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

// [START pubsub_generated_pubsublite_AdminClient_TopicSubscriptions]

package main

import (
	"context"
	"fmt"

	"cloud.google.com/go/pubsublite"
	"google.golang.org/api/iterator"
)

func main() {
	ctx := context.Background()
	// NOTE: region must correspond to the zone of the topic.
	admin, err := pubsublite.NewAdminClient(ctx, "region")
	if err != nil {
		// TODO: Handle error.
	}

	// List the paths of all subscriptions of a topic.
	const topic = "projects/my-project/locations/zone/topics/my-topic"
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

// [END pubsub_generated_pubsublite_AdminClient_TopicSubscriptions]

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

// [START pubsub_generated_pubsublite_AdminClient_Subscriptions]

package main

import (
	"context"
	"fmt"

	"cloud.google.com/go/pubsublite"
	"google.golang.org/api/iterator"
)

func main() {
	ctx := context.Background()
	// NOTE: region must correspond to the zone below.
	admin, err := pubsublite.NewAdminClient(ctx, "region")
	if err != nil {
		// TODO: Handle error.
	}

	// List the configs of all subscriptions in the given zone for the project.
	it := admin.Subscriptions(ctx, "projects/my-project/locations/zone")
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

// [END pubsub_generated_pubsublite_AdminClient_Subscriptions]

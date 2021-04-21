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

// [START pubsub_generated_pubsub_Client_CreateTopicWithConfig]

package main

import (
	"context"

	"cloud.google.com/go/pubsub"
)

func main() {
	ctx := context.Background()
	client, err := pubsub.NewClient(ctx, "project-id")
	if err != nil {
		// TODO: Handle error.
	}

	// Create a new topic with the given name and config.
	topicConfig := &pubsub.TopicConfig{
		KMSKeyName: "projects/project-id/locations/global/keyRings/my-key-ring/cryptoKeys/my-key",
		MessageStoragePolicy: pubsub.MessageStoragePolicy{
			AllowedPersistenceRegions: []string{"us-east1"},
		},
	}
	topic, err := client.CreateTopicWithConfig(ctx, "topicName", topicConfig)
	if err != nil {
		// TODO: Handle error.
	}
	_ = topic // TODO: use the topic.
}

// [END pubsub_generated_pubsub_Client_CreateTopicWithConfig]

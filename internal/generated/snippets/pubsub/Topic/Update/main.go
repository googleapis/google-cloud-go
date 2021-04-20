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

// [START pubsub_generated_pubsub_Topic_Update]

package main

import (
	"context"

	"cloud.google.com/go/pubsub"
)

func main() {
	ctx := context.Background()
	client, err := pubsub.NewClient(ctx, "project-id")
	if err != nil {
		// TODO: Handle error.V
	}
	topic := client.Topic("topic-name")
	topicConfig, err := topic.Update(ctx, pubsub.TopicConfigToUpdate{
		MessageStoragePolicy: &pubsub.MessageStoragePolicy{
			AllowedPersistenceRegions: []string{
				"asia-east1", "asia-northeast1", "asia-southeast1", "australia-southeast1",
				"europe-north1", "europe-west1", "europe-west2", "europe-west3", "europe-west4",
				"us-central1", "us-central2", "us-east1", "us-east4", "us-west1", "us-west2"},
		},
	})
	if err != nil {
		// TODO: Handle error.
	}
	_ = topicConfig // TODO: Use TopicConfig
}

// [END pubsub_generated_pubsub_Topic_Update]

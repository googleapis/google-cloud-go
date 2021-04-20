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

// [START pubsub_generated_pubsublite_pscompat_PublisherClient_Publish_errorHandling]

package main

import (
	"context"
	"fmt"
	"sync"
	"time"

	"cloud.google.com/go/pubsub"
	"cloud.google.com/go/pubsublite/pscompat"
	"golang.org/x/sync/errgroup"
	"golang.org/x/xerrors"
)

func main() {
	ctx := context.Background()
	const topic = "projects/my-project/locations/zone/topics/my-topic"
	settings := pscompat.PublishSettings{
		// The PublisherClient will terminate when it cannot connect to backends for
		// more than 10 minutes.
		Timeout: 10 * time.Minute,
		// Sets a conservative publish buffer byte limit, per partition.
		BufferedByteLimit: 1e8,
	}
	publisher, err := pscompat.NewPublisherClientWithSettings(ctx, topic, settings)
	if err != nil {
		// TODO: Handle error.
	}
	defer publisher.Stop()

	var toRepublish []*pubsub.Message
	var mu sync.Mutex
	g := new(errgroup.Group)

	for i := 0; i < 10; i++ {
		msg := &pubsub.Message{
			Data: []byte(fmt.Sprintf("message-%d", i)),
		}
		result := publisher.Publish(ctx, msg)

		g.Go(func() error {
			id, err := result.Get(ctx)
			if err != nil {
				// NOTE: A failed PublishResult indicates that the publisher client has
				// permanently terminated. A new publisher client instance must be
				// created to republish failed messages.
				fmt.Printf("Publish error: %v\n", err)
				// Oversized messages cannot be published.
				if !xerrors.Is(err, pscompat.ErrOversizedMessage) {
					mu.Lock()
					toRepublish = append(toRepublish, msg)
					mu.Unlock()
				}
				return err
			}
			fmt.Printf("Published a message with a message ID: %s\n", id)
			return nil
		})
	}
	if err := g.Wait(); err != nil {
		fmt.Printf("Publisher client terminated due to error: %v\n", publisher.Error())
		switch {
		case xerrors.Is(publisher.Error(), pscompat.ErrBackendUnavailable):
			// TODO: Create a new publisher client to republish failed messages.
		case xerrors.Is(publisher.Error(), pscompat.ErrOverflow):
			// TODO: Create a new publisher client to republish failed messages.
			// Throttle publishing. Note that backend unavailability can also cause
			// buffer overflow before the ErrBackendUnavailable error.
		default:
			// TODO: Inspect and handle fatal error.
		}
	}
}

// [END pubsub_generated_pubsublite_pscompat_PublisherClient_Publish_errorHandling]

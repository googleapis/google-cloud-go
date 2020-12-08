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

	"cloud.google.com/go/pubsub"
	"cloud.google.com/go/pubsublite"
	"cloud.google.com/go/pubsublite/ps"
)

func ExampleSubscriberClient_Receive() {
	ctx := context.Background()
	subscription := pubsublite.SubscriptionPath{Project: "project-id", Zone: "zone", SubscriptionID: "subscription-id"}
	sub, err := ps.NewSubscriberClient(ctx, ps.DefaultReceiveSettings, subscription)
	if err != nil {
		// TODO: Handle error.
	}
	cctx, cancel := context.WithCancel(ctx)
	err = sub.Receive(cctx, func(ctx context.Context, m *pubsub.Message) {
		// TODO: Handle message.
		// NOTE: May be called concurrently; synchronize access to shared memory.
		m.Ack()
	})
	if err != nil {
		// TODO: Handle error.
	}

	// Call cancel from callback, or another goroutine.
	cancel()
}

// This example shows how to throttle SubscriberClient.Receive, which aims for
// high throughput by default. By limiting the number of messages and/or bytes
// being processed at once, you can bound your program's resource consumption.
// Note that ReceiveSettings apply per partition, so keep in mind the number of
// partitions in the associated topic.
func ExampleSubscriberClient_Receive_maxOutstanding() {
	ctx := context.Background()
	subscription := pubsublite.SubscriptionPath{Project: "project-id", Zone: "zone", SubscriptionID: "subscription-id"}
	settings := ps.DefaultReceiveSettings
	settings.MaxOutstandingMessages = 5
	settings.MaxOutstandingBytes = 10e6
	sub, err := ps.NewSubscriberClient(ctx, settings, subscription)
	if err != nil {
		// TODO: Handle error.
	}
	cctx, cancel := context.WithCancel(ctx)
	err = sub.Receive(cctx, func(ctx context.Context, m *pubsub.Message) {
		// TODO: Handle message.
		// NOTE: May be called concurrently; synchronize access to shared memory.
		m.Ack()
	})
	if err != nil {
		// TODO: Handle error.
	}

	// Call cancel from callback, or another goroutine.
	cancel()
}

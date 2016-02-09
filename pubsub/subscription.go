// Copyright 2016 Google Inc. All Rights Reserved.
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

package pubsub

import (
	"fmt"

	"golang.org/x/net/context"
)

// SubscriptionHandle is a reference to a PubSub subscription.
type SubscriptionHandle struct {
	c *Client

	// The fully qualified identifier for the subscription, in the format "projects/<projid>/subscriptions/<name>"
	name string
}

// Subscription creates a reference to a subscription.
func (c *Client) Subscription(name string) *SubscriptionHandle {
	return &SubscriptionHandle{
		c:    c,
		name: fmt.Sprintf("projects/%s/subscriptions/%s", c.projectID, name),
	}
}

// Name returns the globally unique name for the subscription.
func (s *SubscriptionHandle) Name() string {
	return s.name
}

// Subscriptions lists all of the subscriptions for the client's project.
func (c *Client) Subscriptions(ctx context.Context) ([]*SubscriptionHandle, error) {
	subNames, err := c.s.listProjectSubscriptions(ctx, c.fullyQualifiedProjectName())
	if err != nil {
		return nil, err
	}

	subs := []*SubscriptionHandle{}
	for _, s := range subNames {
		subs = append(subs, &SubscriptionHandle{c: c, name: s})
	}
	return subs, nil
}

// TODO(mcgreevy): Allow configuring a PushConfig (endpoint and attributes) and default ack deadline.
type SubscriptionConfig struct {
}

// Delete deletes the subscription.
func (s *SubscriptionHandle) Delete(ctx context.Context) error {
	return s.c.s.deleteSubscription(ctx, s.name)
}

// Exists reports whether the subscription exists on the server.
func (s *SubscriptionHandle) Exists(ctx context.Context) (bool, error) {
	return s.c.s.subscriptionExists(ctx, s.name)
}

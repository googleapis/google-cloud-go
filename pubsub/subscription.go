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
	"net/http"

	"google.golang.org/api/googleapi"

	"golang.org/x/net/context"
	raw "google.golang.org/api/pubsub/v1"
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
	subs := []*SubscriptionHandle{}
	err := c.s.Projects.Subscriptions.List(c.fullyQualifiedProjectName()).
		Pages(ctx, func(res *raw.ListSubscriptionsResponse) error {
			for _, s := range res.Subscriptions {
				subs = append(subs, &SubscriptionHandle{c: c, name: s.Name})
			}
			return nil
		})
	if err != nil {
		return nil, err
	}
	return subs, nil
}

// TODO(mcgreevy): Allow configuring a PushConfig (endpoint and attributes) and default ack deadline.
type SubscriptionConfig struct {
}

// Delete deletes the subscription.
func (s *SubscriptionHandle) Delete(ctx context.Context) error {
	_, err := s.c.s.Projects.Subscriptions.Delete(s.name).Context(ctx).Do()
	return err
}

// Exists reports whether the subscription exists on the server.
func (s *SubscriptionHandle) Exists(ctx context.Context) (bool, error) {
	_, err := s.c.s.Projects.Subscriptions.Get(s.name).Context(ctx).Do()
	if err == nil {
		return true, nil
	}
	if e, ok := err.(*googleapi.Error); ok && e.Code == http.StatusNotFound {
		return false, nil
	}
	return false, err
}

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

	"golang.org/x/net/context"
	"google.golang.org/api/googleapi"
	raw "google.golang.org/api/pubsub/v1"
)

// TopicHandle is a reference to a PubSub topic.
type TopicHandle struct {
	c *Client

	// The fully qualified identifier for the topic, in the format "projects/<projid>/topics/<name>"
	name string
}

// NewTopic creates a new topic.
// The specified topic name must start with a letter, and contain only letters
// ([A-Za-z]), numbers ([0-9]), dashes (-), underscores (_), periods (.),
// tildes (~), plus (+) or percent signs (%).  It must be between 3 and 255
// characters in length, and must not start with "goog".
// If the topic already exists an error will be returned.
func (c *Client) NewTopic(ctx context.Context, name string) (*TopicHandle, error) {
	t := c.Topic(name)
	// Note: The raw API expects a Topic body, but ignores it.
	_, err := c.s.Projects.Topics.Create(t.Name(), &raw.Topic{}).
		Context(ctx).
		Do()
	return t, err
}

// Topic creates a reference to a topic.
func (c *Client) Topic(name string) *TopicHandle {
	return &TopicHandle{c: c, name: fmt.Sprintf("projects/%s/topics/%s", c.projectID, name)}
}

// Topics lists all of the topics for the client's project.
func (c *Client) Topics(ctx context.Context) ([]*TopicHandle, error) {
	topics := []*TopicHandle{}
	err := c.s.Projects.Topics.List(c.fullyQualifiedProjectName()).
		Pages(ctx, func(res *raw.ListTopicsResponse) error {
			for _, t := range res.Topics {
				topics = append(topics, &TopicHandle{c: c, name: t.Name})
			}
			return nil
		})
	if err != nil {
		return nil, err
	}
	return topics, nil
}

// Name returns the globally unique name for the topic.
func (t *TopicHandle) Name() string {
	return t.name
}

// Delete deletes the topic.
func (t *TopicHandle) Delete(ctx context.Context) error {
	_, err := t.c.s.Projects.Topics.Delete(t.name).Context(ctx).Do()
	return err
}

// Exists reports whether the topic exists on the server.
func (t *TopicHandle) Exists(ctx context.Context) (bool, error) {
	_, err := t.c.s.Projects.Topics.Get(t.name).Context(ctx).Do()
	if err == nil {
		return true, nil
	}
	if e, ok := err.(*googleapi.Error); ok && e.Code == http.StatusNotFound {
		return false, nil
	}
	return false, err
}

// Subscriptions lists the subscriptions for this topic.
func (t *TopicHandle) Subscriptions(ctx context.Context) ([]*SubscriptionHandle, error) {
	subs := []*SubscriptionHandle{}
	err := t.c.s.Projects.Topics.Subscriptions.List(t.name).
		Pages(ctx, func(res *raw.ListTopicSubscriptionsResponse) error {
			for _, s := range res.Subscriptions {
				subs = append(subs, &SubscriptionHandle{c: t.c, name: s})
			}
			return nil
		})
	if err != nil {
		return nil, err
	}
	return subs, nil
}

// Subscribe creates a new subscription to the topic.
// The specified subscription name must start with a letter, and contain only
// letters ([A-Za-z]), numbers ([0-9]), dashes (-), underscores (_), periods
// (.), tildes (~), plus (+) or percent signs (%). It must be between 3 and 255
// characters in length, and must not start with "goog".
// If the subscription already exists an error will be returned.
func (t *TopicHandle) Subscribe(ctx context.Context, name string, config *SubscriptionConfig) (*SubscriptionHandle, error) {
	rawSub := &raw.Subscription{
		Topic: t.name,
	}
	sub := t.c.Subscription(name)
	_, err := t.c.s.Projects.Subscriptions.Create(sub.Name(), rawSub).Context(ctx).Do()
	return sub, err
}

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
	raw "google.golang.org/api/pubsub/v1"
)

// TopicHandle is a reference to a PubSub topic.
type TopicHandle struct {
	c    *Client
	name string
}

// NewTopic creates a new topic with the specified name.
// It returns an error if a topic already exists with that name.
func (c *Client) NewTopic(ctx context.Context, name string) (*TopicHandle, error) {
	t := c.Topic(name)
	// Note: The raw API expects a Topic body, but ignores it.
	_, err := c.s.Projects.Topics.Create(t.fullyQualifiedName(), &raw.Topic{}).
		Context(ctx).
		Do()
	return t, err
}

// Topic creates a reference to a topic.
func (c *Client) Topic(name string) *TopicHandle {
	return &TopicHandle{c: c, name: name}
}

// Name returns the name which uniquely identifies this topic within a project.
func (t *TopicHandle) Name() string {
	return t.name
}

func (t *TopicHandle) fullyQualifiedName() string {
	return fmt.Sprintf("projects/%s/topics/%s", t.c.projectID, t.name)
}

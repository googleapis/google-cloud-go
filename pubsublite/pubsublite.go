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

package pubsublite

import (
	"context"

	"google.golang.org/api/option"

	vkit "cloud.google.com/go/pubsublite/apiv1"
	pb "google.golang.org/genproto/googleapis/cloud/pubsublite/v1"
)

// Client provides admin operations for Google Pub/Sub Lite resources within a
// Google Cloud region. A Client may be shared by multiple goroutines.
type Client struct {
	// The Google Cloud region that this Client manages resources for.
	Region string
	admin  *vkit.AdminClient
}

// NewAdminClient creates a new Cloud Pub/Sub Lite client to perform admin
// operations for resources within a given region.
// See https://cloud.google.com/pubsub/lite/docs/locations for the list of
// regions and zones where Google Pub/Sub Lite is available.
func NewAdminClient(ctx context.Context, region string, opts ...option.ClientOption) (c *Client, err error) {
	if err := validateRegion(region); err != nil {
		return nil, err
	}
	options := []option.ClientOption{option.WithEndpoint(region + "-pubsublite.googleapis.com:443")}
	admin, err := vkit.NewAdminClient(ctx, options...)
	if err != nil {
		return nil, err
	}
	return &Client{Region: region, admin: admin}, nil
}

// CreateTopic creates a new topic from the given config.
func (c *Client) CreateTopic(ctx context.Context, config *TopicConfig) (*TopicConfig, error) {
	req := &pb.CreateTopicRequest{
		Parent:  config.Name.location().String(),
		Topic:   config.toProto(),
		TopicId: config.Name.TopicID,
	}
	topicpb, err := c.admin.CreateTopic(ctx, req)
	if err != nil {
		return nil, err
	}
	return protoToTopicConfig(topicpb)
}

// UpdateTopic updates an existing topic from the given config and returns the
// new topic config.
func (c *Client) UpdateTopic(ctx context.Context, config *TopicConfigToUpdate) (*TopicConfig, error) {
	topicpb, err := c.admin.UpdateTopic(ctx, config.toUpdateRequest())
	if err != nil {
		return nil, err
	}
	return protoToTopicConfig(topicpb)
}

// DeleteTopic deletes a topic.
func (c *Client) DeleteTopic(ctx context.Context, topic TopicPath) error {
	return c.admin.DeleteTopic(ctx, &pb.DeleteTopicRequest{Name: topic.String()})
}

// Topic retrieves the configuration of a topic.
func (c *Client) Topic(ctx context.Context, topic TopicPath) (*TopicConfig, error) {
	topicpb, err := c.admin.GetTopic(ctx, &pb.GetTopicRequest{Name: topic.String()})
	if err != nil {
		return nil, err
	}
	return protoToTopicConfig(topicpb)
}

// TopicPartitions returns the number of partitions for a topic.
func (c *Client) TopicPartitions(ctx context.Context, topic TopicPath) (int, error) {
	partitions, err := c.admin.GetTopicPartitions(ctx, &pb.GetTopicPartitionsRequest{Name: topic.String()})
	if err != nil {
		return 0, err
	}
	return int(partitions.GetPartitionCount()), nil
}

// TopicSubscriptions retrieves the list of subscription paths for a topic.
func (c *Client) TopicSubscriptions(ctx context.Context, topic TopicPath) (*SubscriptionPathIterator, error) {
	subsPathIt := c.admin.ListTopicSubscriptions(ctx, &pb.ListTopicSubscriptionsRequest{Name: topic.String()})
	return &SubscriptionPathIterator{it: subsPathIt}, nil
}

// Topics retrieves the list of topic configs for a given project and zone.
func (c *Client) Topics(ctx context.Context, location LocationPath) *TopicIterator {
	return &TopicIterator{
		it: c.admin.ListTopics(ctx, &pb.ListTopicsRequest{Parent: location.String()}),
	}
}

// CreateSubscription creates a new subscription from the given config.
func (c *Client) CreateSubscription(ctx context.Context, config *SubscriptionConfig) (*SubscriptionConfig, error) {
	req := &pb.CreateSubscriptionRequest{
		Parent:         config.Name.location().String(),
		Subscription:   config.toProto(),
		SubscriptionId: config.Name.SubscriptionID,
	}
	subspb, err := c.admin.CreateSubscription(ctx, req)
	if err != nil {
		return nil, err
	}
	return protoToSubscriptionConfig(subspb)
}

// UpdateSubscription updates an existing subscription from the given config and
// returns the new subscription config.
func (c *Client) UpdateSubscription(ctx context.Context, config *SubscriptionConfigToUpdate) (*SubscriptionConfig, error) {
	subspb, err := c.admin.UpdateSubscription(ctx, config.toUpdateRequest())
	if err != nil {
		return nil, err
	}
	return protoToSubscriptionConfig(subspb)
}

// DeleteSubscription deletes a subscription.
func (c *Client) DeleteSubscription(ctx context.Context, subscription SubscriptionPath) error {
	return c.admin.DeleteSubscription(ctx, &pb.DeleteSubscriptionRequest{Name: subscription.String()})
}

// Subscription retrieves the configuration of a subscription.
func (c *Client) Subscription(ctx context.Context, subscription SubscriptionPath) (*SubscriptionConfig, error) {
	subspb, err := c.admin.GetSubscription(ctx, &pb.GetSubscriptionRequest{Name: subscription.String()})
	if err != nil {
		return nil, err
	}
	return protoToSubscriptionConfig(subspb)
}

// Subscriptions retrieves the list of subscription configs for a given project
// and zone.
func (c *Client) Subscriptions(ctx context.Context, location LocationPath) *SubscriptionIterator {
	return &SubscriptionIterator{
		it: c.admin.ListSubscriptions(ctx, &pb.ListSubscriptionsRequest{Parent: location.String()}),
	}
}

// Close releases any resources held by the client when it is no longer
// required. If the client is available for the lifetime of the program, then
// Close need not be called at exit.
func (c *Client) Close() error {
	return c.admin.Close()
}

// TopicIterator is an iterator that returns a list of topic configs.
type TopicIterator struct {
	it *vkit.TopicIterator
}

// Next returns the next topic config. The second return value will be
// iterator.Done if there are no more topic configs.
func (t *TopicIterator) Next() (*TopicConfig, error) {
	topicpb, err := t.it.Next()
	if err != nil {
		return nil, err
	}
	return protoToTopicConfig(topicpb)
}

// SubscriptionIterator is an iterator that returns a list of subscription
// configs.
type SubscriptionIterator struct {
	it *vkit.SubscriptionIterator
}

// Next returns the next subscription config. The second return value will be
// iterator.Done if there are no more subscription configs.
func (s *SubscriptionIterator) Next() (*SubscriptionConfig, error) {
	subspb, err := s.it.Next()
	if err != nil {
		return nil, err
	}
	return protoToSubscriptionConfig(subspb)
}

// SubscriptionPathIterator is an iterator that returns a list of subscription
// paths.
type SubscriptionPathIterator struct {
	it *vkit.StringIterator
}

// Next returns the next subscription path. The second return value will be
// iterator.Done if there are no more subscription paths.
func (sp *SubscriptionPathIterator) Next() (SubscriptionPath, error) {
	subsPath, err := sp.it.Next()
	if err != nil {
		return SubscriptionPath{}, err
	}
	return parseSubscriptionPath(subsPath)
}

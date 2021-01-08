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
	"errors"

	"cloud.google.com/go/pubsublite/internal/wire"
	"google.golang.org/api/option"

	vkit "cloud.google.com/go/pubsublite/apiv1"
	pb "google.golang.org/genproto/googleapis/cloud/pubsublite/v1"
)

var (
	errNoTopicFieldsUpdated        = errors.New("pubsublite: no fields updated for topic")
	errNoSubscriptionFieldsUpdated = errors.New("pubsublite: no fields updated for subscription")
)

// AdminClient provides admin operations for Cloud Pub/Sub Lite resources
// within a Google Cloud region. An AdminClient may be shared by multiple
// goroutines.
type AdminClient struct {
	admin *vkit.AdminClient
}

// NewAdminClient creates a new Cloud Pub/Sub Lite client to perform admin
// operations for resources within a given region.
// See https://cloud.google.com/pubsub/lite/docs/locations for the list of
// regions and zones where Cloud Pub/Sub Lite is available.
func NewAdminClient(ctx context.Context, region string, opts ...option.ClientOption) (*AdminClient, error) {
	if err := wire.ValidateRegion(region); err != nil {
		return nil, err
	}
	admin, err := wire.NewAdminClient(ctx, region, opts...)
	if err != nil {
		return nil, err
	}
	return &AdminClient{admin: admin}, nil
}

// CreateTopic creates a new topic from the given config. If the topic already
// exists an error will be returned.
func (ac *AdminClient) CreateTopic(ctx context.Context, config TopicConfig) (*TopicConfig, error) {
	req := &pb.CreateTopicRequest{
		Parent:  config.Name.location().String(),
		Topic:   config.toProto(),
		TopicId: config.Name.TopicID,
	}
	topicpb, err := ac.admin.CreateTopic(ctx, req)
	if err != nil {
		return nil, err
	}
	return protoToTopicConfig(topicpb)
}

// UpdateTopic updates an existing topic from the given config and returns the
// new topic config. UpdateTopic returns an error if no fields were modified.
func (ac *AdminClient) UpdateTopic(ctx context.Context, config TopicConfigToUpdate) (*TopicConfig, error) {
	req := config.toUpdateRequest()
	if len(req.GetUpdateMask().GetPaths()) == 0 {
		return nil, errNoTopicFieldsUpdated
	}
	topicpb, err := ac.admin.UpdateTopic(ctx, req)
	if err != nil {
		return nil, err
	}
	return protoToTopicConfig(topicpb)
}

// DeleteTopic deletes a topic.
func (ac *AdminClient) DeleteTopic(ctx context.Context, topic TopicPath) error {
	return ac.admin.DeleteTopic(ctx, &pb.DeleteTopicRequest{Name: topic.String()})
}

// Topic retrieves the configuration of a topic.
func (ac *AdminClient) Topic(ctx context.Context, topic TopicPath) (*TopicConfig, error) {
	topicpb, err := ac.admin.GetTopic(ctx, &pb.GetTopicRequest{Name: topic.String()})
	if err != nil {
		return nil, err
	}
	return protoToTopicConfig(topicpb)
}

// TopicPartitions returns the number of partitions for a topic.
func (ac *AdminClient) TopicPartitions(ctx context.Context, topic TopicPath) (int, error) {
	partitions, err := ac.admin.GetTopicPartitions(ctx, &pb.GetTopicPartitionsRequest{Name: topic.String()})
	if err != nil {
		return 0, err
	}
	return int(partitions.GetPartitionCount()), nil
}

// TopicSubscriptions retrieves the list of subscription paths for a topic.
func (ac *AdminClient) TopicSubscriptions(ctx context.Context, topic TopicPath) *SubscriptionPathIterator {
	return &SubscriptionPathIterator{
		it: ac.admin.ListTopicSubscriptions(ctx, &pb.ListTopicSubscriptionsRequest{Name: topic.String()}),
	}
}

// Topics retrieves the list of topic configs for a given project and zone.
func (ac *AdminClient) Topics(ctx context.Context, location LocationPath) *TopicIterator {
	return &TopicIterator{
		it: ac.admin.ListTopics(ctx, &pb.ListTopicsRequest{Parent: location.String()}),
	}
}

// CreateSubscription creates a new subscription from the given config. If the
// subscription already exists an error will be returned.
func (ac *AdminClient) CreateSubscription(ctx context.Context, config SubscriptionConfig) (*SubscriptionConfig, error) {
	req := &pb.CreateSubscriptionRequest{
		Parent:         config.Name.location().String(),
		Subscription:   config.toProto(),
		SubscriptionId: config.Name.SubscriptionID,
	}
	subspb, err := ac.admin.CreateSubscription(ctx, req)
	if err != nil {
		return nil, err
	}
	return protoToSubscriptionConfig(subspb)
}

// UpdateSubscription updates an existing subscription from the given config and
// returns the new subscription config. UpdateSubscription returns an error if
// no fields were modified.
func (ac *AdminClient) UpdateSubscription(ctx context.Context, config SubscriptionConfigToUpdate) (*SubscriptionConfig, error) {
	req := config.toUpdateRequest()
	if len(req.GetUpdateMask().GetPaths()) == 0 {
		return nil, errNoSubscriptionFieldsUpdated
	}
	subspb, err := ac.admin.UpdateSubscription(ctx, req)
	if err != nil {
		return nil, err
	}
	return protoToSubscriptionConfig(subspb)
}

// DeleteSubscription deletes a subscription.
func (ac *AdminClient) DeleteSubscription(ctx context.Context, subscription SubscriptionPath) error {
	return ac.admin.DeleteSubscription(ctx, &pb.DeleteSubscriptionRequest{Name: subscription.String()})
}

// Subscription retrieves the configuration of a subscription.
func (ac *AdminClient) Subscription(ctx context.Context, subscription SubscriptionPath) (*SubscriptionConfig, error) {
	subspb, err := ac.admin.GetSubscription(ctx, &pb.GetSubscriptionRequest{Name: subscription.String()})
	if err != nil {
		return nil, err
	}
	return protoToSubscriptionConfig(subspb)
}

// Subscriptions retrieves the list of subscription configs for a given project
// and zone.
func (ac *AdminClient) Subscriptions(ctx context.Context, location LocationPath) *SubscriptionIterator {
	return &SubscriptionIterator{
		it: ac.admin.ListSubscriptions(ctx, &pb.ListSubscriptionsRequest{Parent: location.String()}),
	}
}

// Close releases any resources held by the client when it is no longer
// required. If the client is available for the lifetime of the program, then
// Close need not be called at exit.
func (ac *AdminClient) Close() error {
	return ac.admin.Close()
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

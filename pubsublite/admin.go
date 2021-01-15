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

// AdminClient provides admin operations for Pub/Sub Lite resources within a
// Google Cloud region. The zone component of resource paths must be within this
// region. See https://cloud.google.com/pubsub/lite/docs/locations for the list
// of zones where Pub/Sub Lite is available.
//
// An AdminClient may be shared by multiple goroutines.
type AdminClient struct {
	admin *vkit.AdminClient
}

// NewAdminClient creates a new Pub/Sub Lite client to perform admin operations
// for resources within a given region.
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
	topicPath, err := wire.ParseTopicPath(config.Name)
	if err != nil {
		return nil, err
	}
	req := &pb.CreateTopicRequest{
		Parent:  topicPath.Location().String(),
		Topic:   config.toProto(),
		TopicId: topicPath.TopicID,
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
	if _, err := wire.ParseTopicPath(config.Name); err != nil {
		return nil, err
	}
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

// DeleteTopic deletes a topic. A valid topic path has the format:
// "projects/PROJECT_ID/locations/ZONE/topics/TOPIC_ID".
func (ac *AdminClient) DeleteTopic(ctx context.Context, topic string) error {
	if _, err := wire.ParseTopicPath(topic); err != nil {
		return err
	}
	return ac.admin.DeleteTopic(ctx, &pb.DeleteTopicRequest{Name: topic})
}

// Topic retrieves the configuration of a topic. A valid topic path has the
// format: "projects/PROJECT_ID/locations/ZONE/topics/TOPIC_ID".
func (ac *AdminClient) Topic(ctx context.Context, topic string) (*TopicConfig, error) {
	if _, err := wire.ParseTopicPath(topic); err != nil {
		return nil, err
	}
	topicpb, err := ac.admin.GetTopic(ctx, &pb.GetTopicRequest{Name: topic})
	if err != nil {
		return nil, err
	}
	return protoToTopicConfig(topicpb)
}

// TopicPartitionCount returns the number of partitions for a topic. A valid
// topic path has the format:
// "projects/PROJECT_ID/locations/ZONE/topics/TOPIC_ID".
func (ac *AdminClient) TopicPartitionCount(ctx context.Context, topic string) (int, error) {
	if _, err := wire.ParseTopicPath(topic); err != nil {
		return 0, err
	}
	partitions, err := ac.admin.GetTopicPartitions(ctx, &pb.GetTopicPartitionsRequest{Name: topic})
	if err != nil {
		return 0, err
	}
	return int(partitions.GetPartitionCount()), nil
}

// TopicSubscriptions retrieves the list of subscription paths for a topic.
// A valid topic path has the format:
// "projects/PROJECT_ID/locations/ZONE/topics/TOPIC_ID".
func (ac *AdminClient) TopicSubscriptions(ctx context.Context, topic string) *SubscriptionPathIterator {
	if _, err := wire.ParseTopicPath(topic); err != nil {
		return &SubscriptionPathIterator{err: err}
	}
	return &SubscriptionPathIterator{
		it: ac.admin.ListTopicSubscriptions(ctx, &pb.ListTopicSubscriptionsRequest{Name: topic}),
	}
}

// Topics retrieves the list of topic configs for a given project and zone.
// A valid parent path has the format: "projects/PROJECT_ID/locations/ZONE".
func (ac *AdminClient) Topics(ctx context.Context, parent string) *TopicIterator {
	if _, err := wire.ParseLocationPath(parent); err != nil {
		return &TopicIterator{err: err}
	}
	return &TopicIterator{
		it: ac.admin.ListTopics(ctx, &pb.ListTopicsRequest{Parent: parent}),
	}
}

// CreateSubscription creates a new subscription from the given config. If the
// subscription already exists an error will be returned.
func (ac *AdminClient) CreateSubscription(ctx context.Context, config SubscriptionConfig) (*SubscriptionConfig, error) {
	subsPath, err := wire.ParseSubscriptionPath(config.Name)
	if err != nil {
		return nil, err
	}
	if _, err := wire.ParseTopicPath(config.Topic); err != nil {
		return nil, err
	}
	req := &pb.CreateSubscriptionRequest{
		Parent:         subsPath.Location().String(),
		Subscription:   config.toProto(),
		SubscriptionId: subsPath.SubscriptionID,
	}
	subspb, err := ac.admin.CreateSubscription(ctx, req)
	if err != nil {
		return nil, err
	}
	return protoToSubscriptionConfig(subspb), nil
}

// UpdateSubscription updates an existing subscription from the given config and
// returns the new subscription config. UpdateSubscription returns an error if
// no fields were modified.
func (ac *AdminClient) UpdateSubscription(ctx context.Context, config SubscriptionConfigToUpdate) (*SubscriptionConfig, error) {
	if _, err := wire.ParseSubscriptionPath(config.Name); err != nil {
		return nil, err
	}
	req := config.toUpdateRequest()
	if len(req.GetUpdateMask().GetPaths()) == 0 {
		return nil, errNoSubscriptionFieldsUpdated
	}
	subspb, err := ac.admin.UpdateSubscription(ctx, req)
	if err != nil {
		return nil, err
	}
	return protoToSubscriptionConfig(subspb), nil
}

// DeleteSubscription deletes a subscription. A valid subscription path has the
// format: "projects/PROJECT_ID/locations/ZONE/subscriptions/SUBSCRIPTION_ID".
func (ac *AdminClient) DeleteSubscription(ctx context.Context, subscription string) error {
	if _, err := wire.ParseSubscriptionPath(subscription); err != nil {
		return err
	}
	return ac.admin.DeleteSubscription(ctx, &pb.DeleteSubscriptionRequest{Name: subscription})
}

// Subscription retrieves the configuration of a subscription. A valid
// subscription name has the format:
// "projects/PROJECT_ID/locations/ZONE/subscriptions/SUBSCRIPTION_ID".
func (ac *AdminClient) Subscription(ctx context.Context, subscription string) (*SubscriptionConfig, error) {
	if _, err := wire.ParseSubscriptionPath(subscription); err != nil {
		return nil, err
	}
	subspb, err := ac.admin.GetSubscription(ctx, &pb.GetSubscriptionRequest{Name: subscription})
	if err != nil {
		return nil, err
	}
	return protoToSubscriptionConfig(subspb), nil
}

// Subscriptions retrieves the list of subscription configs for a given project
// and zone. A valid parent path has the format:
// "projects/PROJECT_ID/locations/ZONE".
func (ac *AdminClient) Subscriptions(ctx context.Context, parent string) *SubscriptionIterator {
	if _, err := wire.ParseLocationPath(parent); err != nil {
		return &SubscriptionIterator{err: err}
	}
	return &SubscriptionIterator{
		it: ac.admin.ListSubscriptions(ctx, &pb.ListSubscriptionsRequest{Parent: parent}),
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
	it  *vkit.TopicIterator
	err error
}

// Next returns the next topic config. The second return value will be
// iterator.Done if there are no more topic configs.
func (t *TopicIterator) Next() (*TopicConfig, error) {
	if t.err != nil {
		return nil, t.err
	}
	topicpb, err := t.it.Next()
	if err != nil {
		return nil, err
	}
	return protoToTopicConfig(topicpb)
}

// SubscriptionIterator is an iterator that returns a list of subscription
// configs.
type SubscriptionIterator struct {
	it  *vkit.SubscriptionIterator
	err error
}

// Next returns the next subscription config. The second return value will be
// iterator.Done if there are no more subscription configs.
func (s *SubscriptionIterator) Next() (*SubscriptionConfig, error) {
	if s.err != nil {
		return nil, s.err
	}
	subspb, err := s.it.Next()
	if err != nil {
		return nil, err
	}
	return protoToSubscriptionConfig(subspb), nil
}

// SubscriptionPathIterator is an iterator that returns a list of subscription
// paths.
type SubscriptionPathIterator struct {
	it  *vkit.StringIterator
	err error
}

// Next returns the next subscription path, which has format:
// "projects/PROJECT_ID/locations/ZONE/subscriptions/SUBSCRIPTION_ID". The
// second return value will be iterator.Done if there are no more subscription
// paths.
func (sp *SubscriptionPathIterator) Next() (string, error) {
	if sp.err != nil {
		return "", sp.err
	}
	subsPath, err := sp.it.Next()
	if err != nil {
		return "", err
	}
	return subsPath, nil
}

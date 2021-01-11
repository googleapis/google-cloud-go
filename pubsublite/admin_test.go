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
	"testing"
	"time"

	"cloud.google.com/go/internal/testutil"
	"cloud.google.com/go/pubsublite/internal/test"
	"google.golang.org/api/iterator"

	emptypb "github.com/golang/protobuf/ptypes/empty"
	pb "google.golang.org/genproto/googleapis/cloud/pubsublite/v1"
)

func newTestAdminClient(t *testing.T) *AdminClient {
	admin, err := NewAdminClient(context.Background(), "us-central1", testClientOpts...)
	if err != nil {
		t.Fatal(err)
	}
	return admin
}

func TestAdminTopicCRUD(t *testing.T) {
	ctx := context.Background()

	// Inputs
	topicPath := TopicPath{Project: "my-proj", Zone: "us-central1-a", TopicID: "my-topic"}
	topicConfig := TopicConfig{
		Name:                       topicPath,
		PartitionCount:             2,
		PublishCapacityMiBPerSec:   4,
		SubscribeCapacityMiBPerSec: 4,
		PerPartitionBytes:          30 * gibi,
		RetentionDuration:          time.Duration(24 * time.Hour),
	}
	updateConfig := TopicConfigToUpdate{
		Name:                       topicPath,
		PublishCapacityMiBPerSec:   6,
		SubscribeCapacityMiBPerSec: 8,
		PerPartitionBytes:          40 * gibi,
		RetentionDuration:          InfiniteRetention,
	}

	// Expected requests and fake responses
	wantCreateReq := &pb.CreateTopicRequest{
		Parent:  "projects/my-proj/locations/us-central1-a",
		TopicId: "my-topic",
		Topic:   topicConfig.toProto(),
	}
	wantUpdateReq := updateConfig.toUpdateRequest()
	wantGetReq := &pb.GetTopicRequest{
		Name: "projects/my-proj/locations/us-central1-a/topics/my-topic",
	}
	wantPartitionsReq := &pb.GetTopicPartitionsRequest{
		Name: "projects/my-proj/locations/us-central1-a/topics/my-topic",
	}
	wantDeleteReq := &pb.DeleteTopicRequest{
		Name: "projects/my-proj/locations/us-central1-a/topics/my-topic",
	}

	verifiers := test.NewVerifiers(t)
	verifiers.GlobalVerifier.Push(wantCreateReq, topicConfig.toProto(), nil)
	verifiers.GlobalVerifier.Push(wantUpdateReq, topicConfig.toProto(), nil)
	verifiers.GlobalVerifier.Push(wantGetReq, topicConfig.toProto(), nil)
	verifiers.GlobalVerifier.Push(wantPartitionsReq, &pb.TopicPartitions{PartitionCount: 3}, nil)
	verifiers.GlobalVerifier.Push(wantDeleteReq, &emptypb.Empty{}, nil)
	mockServer.OnTestStart(verifiers)
	defer mockServer.OnTestEnd()

	admin := newTestAdminClient(t)

	if gotConfig, err := admin.CreateTopic(ctx, topicConfig); err != nil {
		t.Errorf("CreateTopic() got err: %v", err)
	} else if !testutil.Equal(gotConfig, &topicConfig) {
		t.Errorf("CreateTopic() got: %v\nwant: %v", gotConfig, topicConfig)
	}

	if gotConfig, err := admin.UpdateTopic(ctx, updateConfig); err != nil {
		t.Errorf("UpdateTopic() got err: %v", err)
	} else if !testutil.Equal(gotConfig, &topicConfig) {
		t.Errorf("UpdateTopic() got: %v\nwant: %v", gotConfig, topicConfig)
	}

	if _, err := admin.UpdateTopic(ctx, TopicConfigToUpdate{}); !test.ErrorEqual(err, errNoTopicFieldsUpdated) {
		t.Errorf("UpdateTopic() got err: (%v), want err: (%v)", err, errNoTopicFieldsUpdated)
	}

	if gotConfig, err := admin.Topic(ctx, topicPath); err != nil {
		t.Errorf("Topic() got err: %v", err)
	} else if !testutil.Equal(gotConfig, &topicConfig) {
		t.Errorf("Topic() got: %v\nwant: %v", gotConfig, topicConfig)
	}

	if gotPartitions, err := admin.TopicPartitions(ctx, topicPath); err != nil {
		t.Errorf("TopicPartitions() got err: %v", err)
	} else if wantPartitions := 3; gotPartitions != wantPartitions {
		t.Errorf("TopicPartitions() got: %v\nwant: %v", gotPartitions, wantPartitions)
	}

	if err := admin.DeleteTopic(ctx, topicPath); err != nil {
		t.Errorf("DeleteTopic() got err: %v", err)
	}
}

func TestAdminListTopics(t *testing.T) {
	ctx := context.Background()

	// Inputs
	locationPath := LocationPath{Project: "my-proj", Zone: "us-central1-a"}
	topicConfig1 := TopicConfig{
		Name:                       TopicPath{Project: "my-proj", Zone: "us-central1-a", TopicID: "topic1"},
		PartitionCount:             2,
		PublishCapacityMiBPerSec:   4,
		SubscribeCapacityMiBPerSec: 4,
		PerPartitionBytes:          30 * gibi,
		RetentionDuration:          24 * time.Hour,
	}
	topicConfig2 := TopicConfig{
		Name:                       TopicPath{Project: "my-proj", Zone: "us-central1-a", TopicID: "topic2"},
		PartitionCount:             4,
		PublishCapacityMiBPerSec:   6,
		SubscribeCapacityMiBPerSec: 8,
		PerPartitionBytes:          50 * gibi,
		RetentionDuration:          InfiniteRetention,
	}
	topicConfig3 := TopicConfig{
		Name:                       TopicPath{Project: "my-proj", Zone: "us-central1-a", TopicID: "topic3"},
		PartitionCount:             3,
		PublishCapacityMiBPerSec:   8,
		SubscribeCapacityMiBPerSec: 12,
		PerPartitionBytes:          60 * gibi,
		RetentionDuration:          12 * time.Hour,
	}

	// Expected requests and fake responses
	wantListReq1 := &pb.ListTopicsRequest{
		Parent: "projects/my-proj/locations/us-central1-a",
	}
	listResp1 := &pb.ListTopicsResponse{
		Topics:        []*pb.Topic{topicConfig1.toProto(), topicConfig2.toProto()},
		NextPageToken: "next_token",
	}
	wantListReq2 := &pb.ListTopicsRequest{
		Parent:    "projects/my-proj/locations/us-central1-a",
		PageToken: "next_token",
	}
	listResp2 := &pb.ListTopicsResponse{
		Topics: []*pb.Topic{topicConfig3.toProto()},
	}

	verifiers := test.NewVerifiers(t)
	verifiers.GlobalVerifier.Push(wantListReq1, listResp1, nil)
	verifiers.GlobalVerifier.Push(wantListReq2, listResp2, nil)
	mockServer.OnTestStart(verifiers)
	defer mockServer.OnTestEnd()

	admin := newTestAdminClient(t)

	var gotTopicConfigs []*TopicConfig
	topicIt := admin.Topics(ctx, locationPath)
	for {
		topic, err := topicIt.Next()
		if err == iterator.Done {
			break
		}
		if err != nil {
			t.Errorf("TopicIterator.Next() got err: %v", err)
		} else {
			gotTopicConfigs = append(gotTopicConfigs, topic)
		}
	}

	wantTopicConfigs := []*TopicConfig{&topicConfig1, &topicConfig2, &topicConfig3}
	if diff := testutil.Diff(gotTopicConfigs, wantTopicConfigs); diff != "" {
		t.Errorf("Topics() got: -, want: +\n%s", diff)
	}
}

func TestAdminListTopicSubscriptions(t *testing.T) {
	ctx := context.Background()

	// Expected requests and fake responses
	wantListReq1 := &pb.ListTopicSubscriptionsRequest{
		Name: "projects/my-proj/locations/us-central1-a/topics/my-topic",
	}
	listResp1 := &pb.ListTopicSubscriptionsResponse{
		Subscriptions: []string{
			"projects/my-proj/locations/us-central1-a/subscriptions/subscription1",
			"projects/my-proj/locations/us-central1-a/subscriptions/subscription2",
		},
		NextPageToken: "next_token",
	}
	wantListReq2 := &pb.ListTopicSubscriptionsRequest{
		Name:      "projects/my-proj/locations/us-central1-a/topics/my-topic",
		PageToken: "next_token",
	}
	listResp2 := &pb.ListTopicSubscriptionsResponse{
		Subscriptions: []string{
			"projects/my-proj/locations/us-central1-a/subscriptions/subscription3",
		},
	}

	verifiers := test.NewVerifiers(t)
	verifiers.GlobalVerifier.Push(wantListReq1, listResp1, nil)
	verifiers.GlobalVerifier.Push(wantListReq2, listResp2, nil)
	mockServer.OnTestStart(verifiers)
	defer mockServer.OnTestEnd()

	admin := newTestAdminClient(t)

	// Inputs
	topicPath := TopicPath{Project: "my-proj", Zone: "us-central1-a", TopicID: "my-topic"}
	subscription1 := SubscriptionPath{Project: "my-proj", Zone: "us-central1-a", SubscriptionID: "subscription1"}
	subscription2 := SubscriptionPath{Project: "my-proj", Zone: "us-central1-a", SubscriptionID: "subscription2"}
	subscription3 := SubscriptionPath{Project: "my-proj", Zone: "us-central1-a", SubscriptionID: "subscription3"}

	var gotSubscriptions []SubscriptionPath
	subsPathIt := admin.TopicSubscriptions(ctx, topicPath)
	for {
		subsPath, err := subsPathIt.Next()
		if err == iterator.Done {
			break
		}
		if err != nil {
			t.Errorf("SubscriptionPathIterator.Next() got err: %v", err)
		} else {
			gotSubscriptions = append(gotSubscriptions, subsPath)
		}
	}

	wantSubscriptions := []SubscriptionPath{subscription1, subscription2, subscription3}
	if !testutil.Equal(gotSubscriptions, wantSubscriptions) {
		t.Errorf("TopicSubscriptions() got: %v\nwant: %v", gotSubscriptions, wantSubscriptions)
	}
}

func TestAdminSubscriptionCRUD(t *testing.T) {
	ctx := context.Background()

	// Inputs
	topicPath := TopicPath{Project: "my-proj", Zone: "us-central1-a", TopicID: "my-subscription"}
	subscriptionPath := SubscriptionPath{Project: "my-proj", Zone: "us-central1-a", SubscriptionID: "my-subscription"}
	subscriptionConfig := SubscriptionConfig{
		Name:                subscriptionPath,
		Topic:               topicPath,
		DeliveryRequirement: DeliverImmediately,
	}
	updateConfig := SubscriptionConfigToUpdate{
		Name:                subscriptionPath,
		DeliveryRequirement: DeliverAfterStored,
	}

	// Expected requests and fake responses
	wantCreateReq := &pb.CreateSubscriptionRequest{
		Parent:         "projects/my-proj/locations/us-central1-a",
		SubscriptionId: "my-subscription",
		Subscription:   subscriptionConfig.toProto(),
	}
	wantUpdateReq := updateConfig.toUpdateRequest()
	wantGetReq := &pb.GetSubscriptionRequest{
		Name: "projects/my-proj/locations/us-central1-a/subscriptions/my-subscription",
	}
	wantDeleteReq := &pb.DeleteSubscriptionRequest{
		Name: "projects/my-proj/locations/us-central1-a/subscriptions/my-subscription",
	}

	verifiers := test.NewVerifiers(t)
	verifiers.GlobalVerifier.Push(wantCreateReq, subscriptionConfig.toProto(), nil)
	verifiers.GlobalVerifier.Push(wantUpdateReq, subscriptionConfig.toProto(), nil)
	verifiers.GlobalVerifier.Push(wantGetReq, subscriptionConfig.toProto(), nil)
	verifiers.GlobalVerifier.Push(wantDeleteReq, &emptypb.Empty{}, nil)
	mockServer.OnTestStart(verifiers)
	defer mockServer.OnTestEnd()

	admin := newTestAdminClient(t)

	if gotConfig, err := admin.CreateSubscription(ctx, subscriptionConfig); err != nil {
		t.Errorf("CreateSubscription() got err: %v", err)
	} else if !testutil.Equal(gotConfig, &subscriptionConfig) {
		t.Errorf("CreateSubscription() got: %v\nwant: %v", gotConfig, subscriptionConfig)
	}

	if gotConfig, err := admin.UpdateSubscription(ctx, updateConfig); err != nil {
		t.Errorf("UpdateSubscription() got err: %v", err)
	} else if !testutil.Equal(gotConfig, &subscriptionConfig) {
		t.Errorf("UpdateSubscription() got: %v\nwant: %v", gotConfig, subscriptionConfig)
	}

	if _, err := admin.UpdateSubscription(ctx, SubscriptionConfigToUpdate{}); !test.ErrorEqual(err, errNoSubscriptionFieldsUpdated) {
		t.Errorf("UpdateSubscription() got err: (%v), want err: (%v)", err, errNoSubscriptionFieldsUpdated)
	}

	if gotConfig, err := admin.Subscription(ctx, subscriptionPath); err != nil {
		t.Errorf("Subscription() got err: %v", err)
	} else if !testutil.Equal(gotConfig, &subscriptionConfig) {
		t.Errorf("Subscription() got: %v\nwant: %v", gotConfig, subscriptionConfig)
	}

	if err := admin.DeleteSubscription(ctx, subscriptionPath); err != nil {
		t.Errorf("DeleteSubscription() got err: %v", err)
	}
}

func TestAdminListSubscriptions(t *testing.T) {
	ctx := context.Background()

	// Inputs
	locationPath := LocationPath{Project: "my-proj", Zone: "us-central1-a"}
	subscriptionConfig1 := SubscriptionConfig{
		Name:                SubscriptionPath{Project: "my-proj", Zone: "us-central1-a", SubscriptionID: "subscription1"},
		Topic:               TopicPath{Project: "my-proj", Zone: "us-central1-a", TopicID: "topic1"},
		DeliveryRequirement: DeliverImmediately,
	}
	subscriptionConfig2 := SubscriptionConfig{
		Name:                SubscriptionPath{Project: "my-proj", Zone: "us-central1-a", SubscriptionID: "subscription2"},
		Topic:               TopicPath{Project: "my-proj", Zone: "us-central1-a", TopicID: "topic2"},
		DeliveryRequirement: DeliverAfterStored,
	}
	subscriptionConfig3 := SubscriptionConfig{
		Name:                SubscriptionPath{Project: "my-proj", Zone: "us-central1-a", SubscriptionID: "subscription3"},
		Topic:               TopicPath{Project: "my-proj", Zone: "us-central1-a", TopicID: "topic3"},
		DeliveryRequirement: DeliverImmediately,
	}

	// Expected requests and fake responses
	wantListReq1 := &pb.ListSubscriptionsRequest{
		Parent: "projects/my-proj/locations/us-central1-a",
	}
	listResp1 := &pb.ListSubscriptionsResponse{
		Subscriptions: []*pb.Subscription{subscriptionConfig1.toProto(), subscriptionConfig2.toProto()},
		NextPageToken: "next_token",
	}
	wantListReq2 := &pb.ListSubscriptionsRequest{
		Parent:    "projects/my-proj/locations/us-central1-a",
		PageToken: "next_token",
	}
	listResp2 := &pb.ListSubscriptionsResponse{
		Subscriptions: []*pb.Subscription{subscriptionConfig3.toProto()},
	}

	verifiers := test.NewVerifiers(t)
	verifiers.GlobalVerifier.Push(wantListReq1, listResp1, nil)
	verifiers.GlobalVerifier.Push(wantListReq2, listResp2, nil)
	mockServer.OnTestStart(verifiers)
	defer mockServer.OnTestEnd()

	admin := newTestAdminClient(t)

	var gotSubscriptionConfigs []*SubscriptionConfig
	subscriptionIt := admin.Subscriptions(ctx, locationPath)
	for {
		subscription, err := subscriptionIt.Next()
		if err == iterator.Done {
			break
		}
		if err != nil {
			t.Errorf("SubscriptionIterator.Next() got err: %v", err)
		} else {
			gotSubscriptionConfigs = append(gotSubscriptionConfigs, subscription)
		}
	}

	wantSubscriptionConfigs := []*SubscriptionConfig{&subscriptionConfig1, &subscriptionConfig2, &subscriptionConfig3}
	if diff := testutil.Diff(gotSubscriptionConfigs, wantSubscriptionConfigs); diff != "" {
		t.Errorf("Subscriptions() got: -, want: +\n%s", diff)
	}
}

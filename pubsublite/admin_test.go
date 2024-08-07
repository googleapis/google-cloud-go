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
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	lrpb "cloud.google.com/go/longrunning/autogen/longrunningpb"
	pb "cloud.google.com/go/pubsublite/apiv1/pubsublitepb"
	statuspb "google.golang.org/genproto/googleapis/rpc/status"
	emptypb "google.golang.org/protobuf/types/known/emptypb"
	fmpb "google.golang.org/protobuf/types/known/fieldmaskpb"
	tspb "google.golang.org/protobuf/types/known/timestamppb"
)

func newTestAdminClient(t *testing.T) *AdminClient {
	admin, err := NewAdminClient(context.Background(), "us-central1", testServer.ClientConn())
	if err != nil {
		t.Fatal(err)
	}
	return admin
}

func TestAdminTopicCRUD(t *testing.T) {
	ctx := context.Background()

	// Inputs
	const topicPath = "projects/my-proj/locations/us-central1-a/topics/my-topic"
	topicConfig := TopicConfig{
		Name:                       topicPath,
		PartitionCount:             2,
		PublishCapacityMiBPerSec:   4,
		SubscribeCapacityMiBPerSec: 4,
		PerPartitionBytes:          30 * gibi,
		RetentionDuration:          24 * time.Hour,
		ThroughputReservation:      "projects/my-proj/locations/us-central1/reservations/my-reservation",
	}
	updateConfig := TopicConfigToUpdate{
		Name:                       topicPath,
		PublishCapacityMiBPerSec:   6,
		SubscribeCapacityMiBPerSec: 8,
		PerPartitionBytes:          40 * gibi,
		RetentionDuration:          InfiniteRetention,
		ThroughputReservation:      "",
	}
	emptyUpdateConfig := TopicConfigToUpdate{
		Name: topicPath,
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
	defer admin.Close()

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

	if _, err := admin.UpdateTopic(ctx, emptyUpdateConfig); !test.ErrorEqual(err, errNoTopicFieldsUpdated) {
		t.Errorf("UpdateTopic() got err: (%v), want err: (%v)", err, errNoTopicFieldsUpdated)
	}

	if gotConfig, err := admin.Topic(ctx, topicPath); err != nil {
		t.Errorf("Topic() got err: %v", err)
	} else if !testutil.Equal(gotConfig, &topicConfig) {
		t.Errorf("Topic() got: %v\nwant: %v", gotConfig, topicConfig)
	}

	if gotPartitions, err := admin.TopicPartitionCount(ctx, topicPath); err != nil {
		t.Errorf("TopicPartitionCount() got err: %v", err)
	} else if wantPartitions := 3; gotPartitions != wantPartitions {
		t.Errorf("TopicPartitionCount() got: %v\nwant: %v", gotPartitions, wantPartitions)
	}

	if err := admin.DeleteTopic(ctx, topicPath); err != nil {
		t.Errorf("DeleteTopic() got err: %v", err)
	}
}

func TestAdminListTopics(t *testing.T) {
	ctx := context.Background()

	// Inputs
	const locationPath = "projects/my-proj/locations/us-central1-a"
	topicConfig1 := TopicConfig{
		Name:                       "projects/my-proj/locations/us-central1-a/topics/topic1",
		PartitionCount:             2,
		PublishCapacityMiBPerSec:   4,
		SubscribeCapacityMiBPerSec: 4,
		PerPartitionBytes:          30 * gibi,
		RetentionDuration:          24 * time.Hour,
	}
	topicConfig2 := TopicConfig{
		Name:                       "projects/my-proj/locations/us-central1-a/topics/topic2",
		PartitionCount:             4,
		PublishCapacityMiBPerSec:   6,
		SubscribeCapacityMiBPerSec: 8,
		PerPartitionBytes:          50 * gibi,
		RetentionDuration:          InfiniteRetention,
	}
	topicConfig3 := TopicConfig{
		Name:                       "projects/my-proj/locations/us-central1-a/topics/topic3",
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
	defer admin.Close()

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

	// Inputs
	const (
		topicPath     = "projects/my-proj/locations/us-central1-a/topics/my-topic"
		subscription1 = "projects/my-proj/locations/us-central1-a/subscriptions/subscription1"
		subscription2 = "projects/my-proj/locations/us-central1-a/subscriptions/subscription2"
		subscription3 = "projects/my-proj/locations/us-central1-a/subscriptions/subscription3"
	)

	// Expected requests and fake responses
	wantListReq1 := &pb.ListTopicSubscriptionsRequest{
		Name: "projects/my-proj/locations/us-central1-a/topics/my-topic",
	}
	listResp1 := &pb.ListTopicSubscriptionsResponse{
		Subscriptions: []string{subscription1, subscription2},
		NextPageToken: "next_token",
	}
	wantListReq2 := &pb.ListTopicSubscriptionsRequest{
		Name:      "projects/my-proj/locations/us-central1-a/topics/my-topic",
		PageToken: "next_token",
	}
	listResp2 := &pb.ListTopicSubscriptionsResponse{
		Subscriptions: []string{subscription3},
	}

	verifiers := test.NewVerifiers(t)
	verifiers.GlobalVerifier.Push(wantListReq1, listResp1, nil)
	verifiers.GlobalVerifier.Push(wantListReq2, listResp2, nil)
	mockServer.OnTestStart(verifiers)
	defer mockServer.OnTestEnd()

	admin := newTestAdminClient(t)
	defer admin.Close()

	var gotSubscriptions []string
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

	wantSubscriptions := []string{subscription1, subscription2, subscription3}
	if !testutil.Equal(gotSubscriptions, wantSubscriptions) {
		t.Errorf("TopicSubscriptions() got: %v\nwant: %v", gotSubscriptions, wantSubscriptions)
	}
}

func TestAdminSubscriptionCRUD(t *testing.T) {
	ctx := context.Background()

	// Inputs
	const topicPath = "projects/my-proj/locations/us-central1-a/topics/my-topic"
	const subscriptionPath = "projects/my-proj/locations/us-central1-a/subscriptions/my-subscription"
	subscriptionConfig := SubscriptionConfig{
		Name:                subscriptionPath,
		Topic:               topicPath,
		DeliveryRequirement: DeliverImmediately,
	}
	updateConfig := SubscriptionConfigToUpdate{
		Name:                subscriptionPath,
		DeliveryRequirement: DeliverAfterStored,
	}
	emptyUpdateConfig := SubscriptionConfigToUpdate{
		Name: subscriptionPath,
	}

	// Expected requests and fake responses
	wantCreateReq := &pb.CreateSubscriptionRequest{
		Parent:         "projects/my-proj/locations/us-central1-a",
		SubscriptionId: "my-subscription",
		Subscription:   subscriptionConfig.toProto(),
		SkipBacklog:    true,
	}
	wantCreateAtBacklogReq := &pb.CreateSubscriptionRequest{
		Parent:         "projects/my-proj/locations/us-central1-a",
		SubscriptionId: "my-subscription",
		Subscription:   subscriptionConfig.toProto(),
		SkipBacklog:    false,
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
	verifiers.GlobalVerifier.Push(wantCreateReq, subscriptionConfig.toProto(), nil)
	verifiers.GlobalVerifier.Push(wantCreateAtBacklogReq, subscriptionConfig.toProto(), nil)
	verifiers.GlobalVerifier.Push(wantUpdateReq, subscriptionConfig.toProto(), nil)
	verifiers.GlobalVerifier.Push(wantGetReq, subscriptionConfig.toProto(), nil)
	verifiers.GlobalVerifier.Push(wantDeleteReq, &emptypb.Empty{}, nil)
	mockServer.OnTestStart(verifiers)
	defer mockServer.OnTestEnd()

	admin := newTestAdminClient(t)
	defer admin.Close()

	if gotConfig, err := admin.CreateSubscription(ctx, subscriptionConfig); err != nil {
		t.Errorf("CreateSubscription() got err: %v", err)
	} else if !testutil.Equal(gotConfig, &subscriptionConfig) {
		t.Errorf("CreateSubscription() got: %v\nwant: %v", gotConfig, subscriptionConfig)
	}

	if gotConfig, err := admin.CreateSubscription(ctx, subscriptionConfig, StartingOffset(End)); err != nil {
		t.Errorf("CreateSubscription() got err: %v", err)
	} else if !testutil.Equal(gotConfig, &subscriptionConfig) {
		t.Errorf("CreateSubscription() got: %v\nwant: %v", gotConfig, subscriptionConfig)
	}

	if gotConfig, err := admin.CreateSubscription(ctx, subscriptionConfig, StartingOffset(Beginning)); err != nil {
		t.Errorf("CreateSubscription() got err: %v", err)
	} else if !testutil.Equal(gotConfig, &subscriptionConfig) {
		t.Errorf("CreateSubscription() got: %v\nwant: %v", gotConfig, subscriptionConfig)
	}

	if gotConfig, err := admin.UpdateSubscription(ctx, updateConfig); err != nil {
		t.Errorf("UpdateSubscription() got err: %v", err)
	} else if !testutil.Equal(gotConfig, &subscriptionConfig) {
		t.Errorf("UpdateSubscription() got: %v\nwant: %v", gotConfig, subscriptionConfig)
	}

	if _, err := admin.UpdateSubscription(ctx, emptyUpdateConfig); !test.ErrorEqual(err, errNoSubscriptionFieldsUpdated) {
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

func TestAdminCreateSubscriptionAtTargetLocation(t *testing.T) {
	const locationPath = "projects/my-proj/locations/us-central1-a"
	const subscription = "my-subscription"
	const topicPath = "projects/my-proj/locations/us-central1-a/topics/my-topic"
	const subscriptionPath = "projects/my-proj/locations/us-central1-a/subscriptions/my-subscription"
	const exportDestinationPath = "projects/my-proj/topics/destination-topic"
	standardSubscription := SubscriptionConfig{
		Name:                subscriptionPath,
		Topic:               topicPath,
		DeliveryRequirement: DeliverImmediately,
	}
	activeExportSubscription := SubscriptionConfig{
		Name:                subscriptionPath,
		Topic:               topicPath,
		DeliveryRequirement: DeliverImmediately,
		ExportConfig: &ExportConfig{
			DesiredState: ExportActive,
			Destination:  &PubSubDestinationConfig{Topic: exportDestinationPath},
		},
	}
	pausedExportSubscription := SubscriptionConfig{
		Name:                subscriptionPath,
		Topic:               topicPath,
		DeliveryRequirement: DeliverImmediately,
		ExportConfig: &ExportConfig{
			DesiredState: ExportPaused,
			Destination:  &PubSubDestinationConfig{Topic: exportDestinationPath},
		},
	}

	timestamp := time.Unix(1234, 0)
	wantSeekToPublishTimeReq := &pb.SeekSubscriptionRequest{
		Name: subscriptionPath,
		Target: &pb.SeekSubscriptionRequest_TimeTarget{
			TimeTarget: &pb.TimeTarget{
				Time: &pb.TimeTarget_PublishTime{
					PublishTime: &tspb.Timestamp{Seconds: 1234},
				},
			},
		},
	}
	wantSeekToEventTimeReq := &pb.SeekSubscriptionRequest{
		Name: subscriptionPath,
		Target: &pb.SeekSubscriptionRequest_TimeTarget{
			TimeTarget: &pb.TimeTarget{
				Time: &pb.TimeTarget_EventTime{
					EventTime: &tspb.Timestamp{Seconds: 1234},
				},
			},
		},
	}
	wantUpdateReq := &pb.UpdateSubscriptionRequest{
		Subscription: &pb.Subscription{
			Name:         subscriptionPath,
			ExportConfig: &pb.ExportConfig{DesiredState: pb.ExportConfig_ACTIVE},
		},
		UpdateMask: &fmpb.FieldMask{
			Paths: []string{"export_config.desired_state"},
		},
	}

	createErr := status.Error(codes.InvalidArgument, "invalid")
	seekErr := status.Error(codes.FailedPrecondition, "failed")
	updateErr := status.Error(codes.PermissionDenied, "permission")

	ctx := context.Background()
	admin := newTestAdminClient(t)
	defer admin.Close()

	for _, tc := range []struct {
		desc                string
		target              SeekTarget
		inputConfig         SubscriptionConfig
		addExpectedRequests func(*test.RPCVerifier)
		wantConfig          *SubscriptionConfig
		wantErr             error
	}{
		{
			desc:        "Standard subscription at beginning success",
			target:      Beginning,
			inputConfig: standardSubscription,
			addExpectedRequests: func(verifier *test.RPCVerifier) {
				verifier.Push(&pb.CreateSubscriptionRequest{
					Parent:         locationPath,
					SubscriptionId: subscription,
					Subscription:   standardSubscription.toProto(),
					SkipBacklog:    false,
				}, standardSubscription.toProto(), nil)
			},
			wantConfig: &standardSubscription,
		},
		{
			desc:        "Standard subscription at end error",
			target:      End,
			inputConfig: standardSubscription,
			addExpectedRequests: func(verifier *test.RPCVerifier) {
				verifier.Push(&pb.CreateSubscriptionRequest{
					Parent:         locationPath,
					SubscriptionId: subscription,
					Subscription:   standardSubscription.toProto(),
					SkipBacklog:    true,
				}, nil, createErr)
			},
			wantErr: createErr,
		},
		{
			desc:        "Standard subscription at publish time success",
			target:      PublishTime(timestamp),
			inputConfig: standardSubscription,
			addExpectedRequests: func(verifier *test.RPCVerifier) {
				verifier.Push(&pb.CreateSubscriptionRequest{
					Parent:         locationPath,
					SubscriptionId: subscription,
					Subscription:   standardSubscription.toProto(),
					SkipBacklog:    false,
				}, standardSubscription.toProto(), nil)
				verifier.Push(wantSeekToPublishTimeReq, &lrpb.Operation{}, nil)
			},
			wantConfig: &standardSubscription,
		},
		{
			desc:        "Standard subscription at event time create error",
			target:      EventTime(timestamp),
			inputConfig: standardSubscription,
			addExpectedRequests: func(verifier *test.RPCVerifier) {
				verifier.Push(&pb.CreateSubscriptionRequest{
					Parent:         locationPath,
					SubscriptionId: subscription,
					Subscription:   standardSubscription.toProto(),
					SkipBacklog:    false,
				}, nil, createErr)
			},
			wantErr: createErr,
		},
		{
			desc:        "Standard subscription at event time seek error",
			target:      EventTime(timestamp),
			inputConfig: standardSubscription,
			addExpectedRequests: func(verifier *test.RPCVerifier) {
				verifier.Push(&pb.CreateSubscriptionRequest{
					Parent:         locationPath,
					SubscriptionId: subscription,
					Subscription:   standardSubscription.toProto(),
					SkipBacklog:    false,
				}, standardSubscription.toProto(), nil)
				verifier.Push(wantSeekToEventTimeReq, nil, seekErr)
			},
			wantErr: seekErr,
		},
		{
			desc:        "Active export subscription at beginning success",
			target:      Beginning,
			inputConfig: activeExportSubscription,
			addExpectedRequests: func(verifier *test.RPCVerifier) {
				verifier.Push(&pb.CreateSubscriptionRequest{
					Parent:         locationPath,
					SubscriptionId: subscription,
					Subscription:   activeExportSubscription.toProto(),
					SkipBacklog:    false,
				}, activeExportSubscription.toProto(), nil)
			},
			wantConfig: &activeExportSubscription,
		},
		{
			desc:        "Paused export subscription at end success",
			target:      End,
			inputConfig: pausedExportSubscription,
			addExpectedRequests: func(verifier *test.RPCVerifier) {
				verifier.Push(&pb.CreateSubscriptionRequest{
					Parent:         locationPath,
					SubscriptionId: subscription,
					Subscription:   pausedExportSubscription.toProto(),
					SkipBacklog:    true,
				}, pausedExportSubscription.toProto(), nil)
			},
			wantConfig: &pausedExportSubscription,
		},
		{
			desc:        "Paused export subscription at publish time success",
			target:      PublishTime(timestamp),
			inputConfig: pausedExportSubscription,
			addExpectedRequests: func(verifier *test.RPCVerifier) {
				verifier.Push(&pb.CreateSubscriptionRequest{
					Parent:         locationPath,
					SubscriptionId: subscription,
					Subscription:   pausedExportSubscription.toProto(),
					SkipBacklog:    false,
				}, pausedExportSubscription.toProto(), nil)
				verifier.Push(wantSeekToPublishTimeReq, &lrpb.Operation{}, nil)
			},
			wantConfig: &pausedExportSubscription,
		},
		{
			desc:        "Active export subscription at event time success",
			target:      EventTime(timestamp),
			inputConfig: activeExportSubscription,
			addExpectedRequests: func(verifier *test.RPCVerifier) {
				// Created in paused state.
				verifier.Push(&pb.CreateSubscriptionRequest{
					Parent:         locationPath,
					SubscriptionId: subscription,
					Subscription:   pausedExportSubscription.toProto(),
					SkipBacklog:    false,
				}, pausedExportSubscription.toProto(), nil)
				verifier.Push(wantSeekToEventTimeReq, &lrpb.Operation{}, nil)
				verifier.Push(wantUpdateReq, activeExportSubscription.toProto(), nil)
			},
			wantConfig: &activeExportSubscription,
		},
		{
			desc:        "Active export subscription at event time create error",
			target:      EventTime(timestamp),
			inputConfig: activeExportSubscription,
			addExpectedRequests: func(verifier *test.RPCVerifier) {
				// Created in paused state.
				verifier.Push(&pb.CreateSubscriptionRequest{
					Parent:         locationPath,
					SubscriptionId: subscription,
					Subscription:   pausedExportSubscription.toProto(),
					SkipBacklog:    false,
				}, nil, createErr)
			},
			wantErr: createErr,
		},
		{
			desc:        "Active export subscription at event time seek error",
			target:      EventTime(timestamp),
			inputConfig: activeExportSubscription,
			addExpectedRequests: func(verifier *test.RPCVerifier) {
				// Created in paused state.
				verifier.Push(&pb.CreateSubscriptionRequest{
					Parent:         locationPath,
					SubscriptionId: subscription,
					Subscription:   pausedExportSubscription.toProto(),
					SkipBacklog:    false,
				}, pausedExportSubscription.toProto(), nil)
				verifier.Push(wantSeekToEventTimeReq, nil, seekErr)
			},
			wantErr: seekErr,
		},
		{
			desc:        "Active export subscription at event time update error",
			target:      EventTime(timestamp),
			inputConfig: activeExportSubscription,
			addExpectedRequests: func(verifier *test.RPCVerifier) {
				// Created in paused state.
				verifier.Push(&pb.CreateSubscriptionRequest{
					Parent:         locationPath,
					SubscriptionId: subscription,
					Subscription:   pausedExportSubscription.toProto(),
					SkipBacklog:    false,
				}, pausedExportSubscription.toProto(), nil)
				verifier.Push(wantSeekToEventTimeReq, &lrpb.Operation{}, nil)
				verifier.Push(wantUpdateReq, nil, updateErr)
			},
			wantErr: updateErr,
		},
	} {
		t.Run(tc.desc, func(t *testing.T) {
			verifiers := test.NewVerifiers(t)
			tc.addExpectedRequests(verifiers.GlobalVerifier)
			mockServer.OnTestStart(verifiers)
			defer mockServer.OnTestEnd()

			gotConfig, err := admin.CreateSubscription(ctx, tc.inputConfig, AtTargetLocation(tc.target))
			if diff := testutil.Diff(gotConfig, tc.wantConfig); diff != "" {
				t.Errorf("CreateSubscription() got: -, want: +\n%s", diff)
			}
			if !test.ErrorEqual(err, tc.wantErr) {
				t.Errorf("CreateSubscription() got err: (%v), want err: (%v)", err, tc.wantErr)
			}
		})
	}
}

func TestAdminListSubscriptions(t *testing.T) {
	ctx := context.Background()

	// Inputs
	const locationPath = "projects/my-proj/locations/us-central1-a"
	subscriptionConfig1 := SubscriptionConfig{
		Name:                "projects/my-proj/locations/us-central1-a/subscriptions/subscription1",
		Topic:               "projects/my-proj/locations/us-central1-a/topics/topic1",
		DeliveryRequirement: DeliverImmediately,
	}
	subscriptionConfig2 := SubscriptionConfig{
		Name:                "projects/my-proj/locations/us-central1-a/subscriptions/subscription2",
		Topic:               "projects/my-proj/locations/us-central1-a/topics/topic2",
		DeliveryRequirement: DeliverAfterStored,
	}
	subscriptionConfig3 := SubscriptionConfig{
		Name:                "projects/my-proj/locations/us-central1-a/subscriptions/subscription3",
		Topic:               "projects/my-proj/locations/us-central1-a/topics/topic3",
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
	defer admin.Close()

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

func TestAdminReservationCRUD(t *testing.T) {
	ctx := context.Background()

	// Inputs
	const reservationPath = "projects/my-proj/locations/us-central1/reservations/my-reservation"
	reservationConfig := ReservationConfig{
		Name:               reservationPath,
		ThroughputCapacity: 4,
	}
	updateConfig := ReservationConfigToUpdate{
		Name:               reservationPath,
		ThroughputCapacity: 5,
	}
	emptyUpdateConfig := ReservationConfigToUpdate{
		Name: reservationPath,
	}

	// Expected requests and fake responses
	wantCreateReq := &pb.CreateReservationRequest{
		Parent:        "projects/my-proj/locations/us-central1",
		ReservationId: "my-reservation",
		Reservation:   reservationConfig.toProto(),
	}
	wantUpdateReq := updateConfig.toUpdateRequest()
	wantGetReq := &pb.GetReservationRequest{
		Name: "projects/my-proj/locations/us-central1/reservations/my-reservation",
	}
	wantDeleteReq := &pb.DeleteReservationRequest{
		Name: "projects/my-proj/locations/us-central1/reservations/my-reservation",
	}

	verifiers := test.NewVerifiers(t)
	verifiers.GlobalVerifier.Push(wantCreateReq, reservationConfig.toProto(), nil)
	verifiers.GlobalVerifier.Push(wantUpdateReq, reservationConfig.toProto(), nil)
	verifiers.GlobalVerifier.Push(wantGetReq, reservationConfig.toProto(), nil)
	verifiers.GlobalVerifier.Push(wantDeleteReq, &emptypb.Empty{}, nil)
	mockServer.OnTestStart(verifiers)
	defer mockServer.OnTestEnd()

	admin := newTestAdminClient(t)
	defer admin.Close()

	if gotConfig, err := admin.CreateReservation(ctx, reservationConfig); err != nil {
		t.Errorf("CreateReservation() got err: %v", err)
	} else if !testutil.Equal(gotConfig, &reservationConfig) {
		t.Errorf("CreateReservation() got: %v\nwant: %v", gotConfig, reservationConfig)
	}

	if gotConfig, err := admin.UpdateReservation(ctx, updateConfig); err != nil {
		t.Errorf("UpdateReservation() got err: %v", err)
	} else if !testutil.Equal(gotConfig, &reservationConfig) {
		t.Errorf("UpdateReservation() got: %v\nwant: %v", gotConfig, reservationConfig)
	}

	if _, err := admin.UpdateReservation(ctx, emptyUpdateConfig); !test.ErrorEqual(err, errNoReservationFieldsUpdated) {
		t.Errorf("UpdateReservation() got err: (%v), want err: (%v)", err, errNoReservationFieldsUpdated)
	}

	if gotConfig, err := admin.Reservation(ctx, reservationPath); err != nil {
		t.Errorf("Reservation() got err: %v", err)
	} else if !testutil.Equal(gotConfig, &reservationConfig) {
		t.Errorf("Reservation() got: %v\nwant: %v", gotConfig, reservationConfig)
	}

	if err := admin.DeleteReservation(ctx, reservationPath); err != nil {
		t.Errorf("DeleteReservation() got err: %v", err)
	}
}

func TestAdminListReservations(t *testing.T) {
	ctx := context.Background()

	// Inputs
	const locationPath = "projects/my-proj/locations/us-central1"
	reservationConfig1 := ReservationConfig{
		Name:               "projects/my-proj/locations/us-central1/reservations/reservation1",
		ThroughputCapacity: 1,
	}
	reservationConfig2 := ReservationConfig{
		Name:               "projects/my-proj/locations/us-central1/reservations/reservation2",
		ThroughputCapacity: 2,
	}
	reservationConfig3 := ReservationConfig{
		Name:               "projects/my-proj/locations/us-central1/reservations/reservation3",
		ThroughputCapacity: 2,
	}

	// Expected requests and fake responses
	wantListReq1 := &pb.ListReservationsRequest{
		Parent: "projects/my-proj/locations/us-central1",
	}
	listResp1 := &pb.ListReservationsResponse{
		Reservations:  []*pb.Reservation{reservationConfig1.toProto(), reservationConfig2.toProto()},
		NextPageToken: "next_token",
	}
	wantListReq2 := &pb.ListReservationsRequest{
		Parent:    "projects/my-proj/locations/us-central1",
		PageToken: "next_token",
	}
	listResp2 := &pb.ListReservationsResponse{
		Reservations: []*pb.Reservation{reservationConfig3.toProto()},
	}

	verifiers := test.NewVerifiers(t)
	verifiers.GlobalVerifier.Push(wantListReq1, listResp1, nil)
	verifiers.GlobalVerifier.Push(wantListReq2, listResp2, nil)
	mockServer.OnTestStart(verifiers)
	defer mockServer.OnTestEnd()

	admin := newTestAdminClient(t)
	defer admin.Close()

	var gotReservationConfigs []*ReservationConfig
	reservationIt := admin.Reservations(ctx, locationPath)
	for {
		reservation, err := reservationIt.Next()
		if err == iterator.Done {
			break
		}
		if err != nil {
			t.Errorf("ReservationIterator.Next() got err: %v", err)
		} else {
			gotReservationConfigs = append(gotReservationConfigs, reservation)
		}
	}

	wantReservationConfigs := []*ReservationConfig{&reservationConfig1, &reservationConfig2, &reservationConfig3}
	if diff := testutil.Diff(gotReservationConfigs, wantReservationConfigs); diff != "" {
		t.Errorf("Reservations() got: -, want: +\n%s", diff)
	}
}

func TestAdminListReservationTopics(t *testing.T) {
	ctx := context.Background()

	// Inputs
	const (
		reservationPath = "projects/my-proj/locations/us-central1/reservations/my-reservation"
		topic1          = "projects/my-proj/locations/us-central1-a/topics/topic1"
		topic2          = "projects/my-proj/locations/us-central1-a/topics/topic2"
		topic3          = "projects/my-proj/locations/us-central1-a/topics/topic3"
	)

	// Expected requests and fake responses
	wantListReq1 := &pb.ListReservationTopicsRequest{
		Name: "projects/my-proj/locations/us-central1/reservations/my-reservation",
	}
	listResp1 := &pb.ListReservationTopicsResponse{
		Topics:        []string{topic1, topic2},
		NextPageToken: "next_token",
	}
	wantListReq2 := &pb.ListReservationTopicsRequest{
		Name:      "projects/my-proj/locations/us-central1/reservations/my-reservation",
		PageToken: "next_token",
	}
	listResp2 := &pb.ListReservationTopicsResponse{
		Topics: []string{topic3},
	}

	verifiers := test.NewVerifiers(t)
	verifiers.GlobalVerifier.Push(wantListReq1, listResp1, nil)
	verifiers.GlobalVerifier.Push(wantListReq2, listResp2, nil)
	mockServer.OnTestStart(verifiers)
	defer mockServer.OnTestEnd()

	admin := newTestAdminClient(t)
	defer admin.Close()

	var gotTopics []string
	topicPathIt := admin.ReservationTopics(ctx, reservationPath)
	for {
		topicPath, err := topicPathIt.Next()
		if err == iterator.Done {
			break
		}
		if err != nil {
			t.Errorf("TopicPathIterator.Next() got err: %v", err)
		} else {
			gotTopics = append(gotTopics, topicPath)
		}
	}

	wantTopics := []string{topic1, topic2, topic3}
	if !testutil.Equal(gotTopics, wantTopics) {
		t.Errorf("ReservationTopics() got: %v\nwant: %v", gotTopics, wantTopics)
	}
}

func TestAdminValidateResourcePaths(t *testing.T) {
	ctx := context.Background()

	// Note: no server requests expected.
	verifiers := test.NewVerifiers(t)
	mockServer.OnTestStart(verifiers)
	defer mockServer.OnTestEnd()

	admin := newTestAdminClient(t)
	defer admin.Close()

	if _, err := admin.Topic(ctx, "INVALID"); err == nil {
		t.Errorf("Topic() should fail")
	}
	if _, err := admin.TopicPartitionCount(ctx, "INVALID"); err == nil {
		t.Errorf("TopicPartitionCount() should fail")
	}
	if err := admin.DeleteTopic(ctx, "INVALID"); err == nil {
		t.Errorf("DeleteTopic() should fail")
	}
	if _, err := admin.Subscription(ctx, "INVALID"); err == nil {
		t.Errorf("Subscription() should fail")
	}
	if err := admin.DeleteSubscription(ctx, "INVALID"); err == nil {
		t.Errorf("DeleteSubscription() should fail")
	}
	if _, err := admin.Reservation(ctx, "INVALID"); err == nil {
		t.Errorf("Reservation() should fail")
	}
	if err := admin.DeleteReservation(ctx, "INVALID"); err == nil {
		t.Errorf("DeleteReservation() should fail")
	}

	topicIt := admin.Topics(ctx, "INVALID")
	if _, err := topicIt.Next(); err == nil {
		t.Errorf("TopicIterator.Next() should fail")
	}
	subsPathIt := admin.TopicSubscriptions(ctx, "INVALID")
	if _, err := subsPathIt.Next(); err == nil {
		t.Errorf("SubscriptionPathIterator.Next() should fail")
	}
	subsIt := admin.Subscriptions(ctx, "INVALID")
	if _, err := subsIt.Next(); err == nil {
		t.Errorf("SubscriptionIterator.Next() should fail")
	}
	resIt := admin.Reservations(ctx, "INVALID")
	if _, err := resIt.Next(); err == nil {
		t.Errorf("ReservationIterator.Next() should fail")
	}
	topicPathIt := admin.ReservationTopics(ctx, "INVALID")
	if _, err := topicPathIt.Next(); err == nil {
		t.Errorf("TopicPathIterator.Next() should fail")
	}
}

func TestAdminSeekSubscription(t *testing.T) {
	const subscriptionPath = "projects/my-proj/locations/us-central1-a/subscriptions/my-subscription"
	const operationPath = "projects/my-proj/locations/us-central1-a/operations/seek-op"
	ctx := context.Background()

	for _, tc := range []struct {
		desc    string
		target  SeekTarget
		wantReq *pb.SeekSubscriptionRequest
	}{
		{
			desc:   "Beginning",
			target: Beginning,
			wantReq: &pb.SeekSubscriptionRequest{
				Name: subscriptionPath,
				Target: &pb.SeekSubscriptionRequest_NamedTarget_{
					NamedTarget: pb.SeekSubscriptionRequest_TAIL,
				},
			},
		},
		{
			desc:   "End",
			target: End,
			wantReq: &pb.SeekSubscriptionRequest{
				Name: subscriptionPath,
				Target: &pb.SeekSubscriptionRequest_NamedTarget_{
					NamedTarget: pb.SeekSubscriptionRequest_HEAD,
				},
			},
		},
		{
			desc:   "PublishTime",
			target: PublishTime(time.Unix(1234, 0)),
			wantReq: &pb.SeekSubscriptionRequest{
				Name: subscriptionPath,
				Target: &pb.SeekSubscriptionRequest_TimeTarget{
					TimeTarget: &pb.TimeTarget{
						Time: &pb.TimeTarget_PublishTime{
							PublishTime: &tspb.Timestamp{Seconds: 1234},
						},
					},
				},
			},
		},
		{
			desc:   "EventTime",
			target: EventTime(time.Unix(2345, 0)),
			wantReq: &pb.SeekSubscriptionRequest{
				Name: subscriptionPath,
				Target: &pb.SeekSubscriptionRequest_TimeTarget{
					TimeTarget: &pb.TimeTarget{
						Time: &pb.TimeTarget_EventTime{
							EventTime: &tspb.Timestamp{Seconds: 2345},
						},
					},
				},
			},
		},
	} {
		t.Run(tc.desc, func(t *testing.T) {
			initialOpResponse := &lrpb.Operation{
				Name: operationPath,
				Done: false,
				Metadata: test.MakeAny(&pb.OperationMetadata{
					Target:     subscriptionPath,
					Verb:       "seek",
					CreateTime: &tspb.Timestamp{Seconds: 123456, Nanos: 700},
				}),
			}
			wantInitialMetadata := &OperationMetadata{
				Target:     subscriptionPath,
				Verb:       "seek",
				CreateTime: time.Unix(123456, 700),
			}

			wantGetOpReq := &lrpb.GetOperationRequest{
				Name: operationPath,
			}
			successOpResponse := &lrpb.Operation{
				Name: operationPath,
				Done: true,
				Metadata: test.MakeAny(&pb.OperationMetadata{
					Target:     subscriptionPath,
					Verb:       "seek",
					CreateTime: &tspb.Timestamp{Seconds: 123456, Nanos: 700},
					EndTime:    &tspb.Timestamp{Seconds: 234567, Nanos: 800},
				}),
				Result: &lrpb.Operation_Response{
					Response: test.MakeAny(&pb.SeekSubscriptionResponse{}),
				},
			}
			failedOpResponse := &lrpb.Operation{
				Name: operationPath,
				Done: true,
				Metadata: test.MakeAny(&pb.OperationMetadata{
					Target:     subscriptionPath,
					Verb:       "seek",
					CreateTime: &tspb.Timestamp{Seconds: 123456, Nanos: 700},
					EndTime:    &tspb.Timestamp{Seconds: 234567, Nanos: 800},
				}),
				Result: &lrpb.Operation_Error{
					Error: &statuspb.Status{Code: 10},
				},
			}
			wantCompleteMetadata := &OperationMetadata{
				Target:     subscriptionPath,
				Verb:       "seek",
				CreateTime: time.Unix(123456, 700),
				EndTime:    time.Unix(234567, 800),
			}

			seekErr := status.Error(codes.FailedPrecondition, "")

			verifiers := test.NewVerifiers(t)
			// Seek 1
			verifiers.GlobalVerifier.Push(tc.wantReq, initialOpResponse, nil)
			verifiers.GlobalVerifier.Push(wantGetOpReq, successOpResponse, nil)
			// Seek 2
			verifiers.GlobalVerifier.Push(tc.wantReq, initialOpResponse, nil)
			verifiers.GlobalVerifier.Push(wantGetOpReq, failedOpResponse, nil)
			// Seek 3
			verifiers.GlobalVerifier.Push(tc.wantReq, nil, seekErr)
			mockServer.OnTestStart(verifiers)
			defer mockServer.OnTestEnd()

			admin := newTestAdminClient(t)
			defer admin.Close()

			// Seek 1 - Successful operation.
			op, err := admin.SeekSubscription(ctx, subscriptionPath, tc.target)
			if err != nil {
				t.Fatalf("SeekSubscription() got err: %v", err)
			}
			if got, want := op.Done(), false; got != want {
				t.Errorf("Done() got %v, want %v", got, want)
			}
			if got, want := op.Name(), operationPath; got != want {
				t.Errorf("Name() got %v, want %v", got, want)
			}
			gotMetadata, err := op.Metadata()
			if err != nil {
				t.Errorf("Metadata() got err: %v", err)
			} else if diff := testutil.Diff(gotMetadata, wantInitialMetadata); diff != "" {
				t.Errorf("Metadata() got: -, want: +\n%s", diff)
			}

			result, err := op.Wait(ctx)
			if err != nil {
				t.Fatalf("Wait() got err: %v", err)
			}
			if result == nil {
				t.Error("SeekSubscriptionResult was nil")
			}
			if got, want := op.Done(), true; got != want {
				t.Errorf("Done() got %v, want %v", got, want)
			}
			gotMetadata, err = op.Metadata()
			if err != nil {
				t.Errorf("Metadata() got err: %v", err)
			} else if diff := testutil.Diff(gotMetadata, wantCompleteMetadata); diff != "" {
				t.Errorf("Metadata() got: -, want: +\n%s", diff)
			}

			// Seek 2 - Failed operation.
			op, err = admin.SeekSubscription(ctx, subscriptionPath, tc.target)
			if err != nil {
				t.Fatalf("SeekSubscription() got err: %v", err)
			}
			if got, want := op.Done(), false; got != want {
				t.Errorf("Done() got %v, want %v", got, want)
			}
			if got, want := op.Name(), operationPath; got != want {
				t.Errorf("Name() got %v, want %v", got, want)
			}
			gotMetadata, err = op.Metadata()
			if err != nil {
				t.Errorf("Metadata() got err: %v", err)
			} else if diff := testutil.Diff(gotMetadata, wantInitialMetadata); diff != "" {
				t.Errorf("Metadata() got: -, want: +\n%s", diff)
			}

			_, gotErr := op.Wait(ctx)
			if wantErr := status.Error(codes.Aborted, ""); !test.ErrorEqual(gotErr, wantErr) {
				t.Fatalf("Wait() got err: %v, want err: %v", gotErr, wantErr)
			}
			if got, want := op.Done(), true; got != want {
				t.Errorf("Done() got %v, want %v", got, want)
			}
			gotMetadata, err = op.Metadata()
			if err != nil {
				t.Errorf("Metadata() got err: %v", err)
			} else if diff := testutil.Diff(gotMetadata, wantCompleteMetadata); diff != "" {
				t.Errorf("Metadata() got: -, want: +\n%s", diff)
			}

			// Seek 3 - Failed seek.
			if _, gotErr := admin.SeekSubscription(ctx, subscriptionPath, tc.target); !test.ErrorEqual(gotErr, seekErr) {
				t.Errorf("SeekSubscription() got err: %v, want err: %v", gotErr, seekErr)
			}
		})
	}
}

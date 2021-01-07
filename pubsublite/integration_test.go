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
	"cloud.google.com/go/internal/uid"
	"cloud.google.com/go/pubsublite/internal/test"
	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"
	"google.golang.org/api/iterator"
	"google.golang.org/api/option"

	vkit "cloud.google.com/go/pubsublite/apiv1"
)

const gibi = 1 << 30

var (
	resourceIDs = uid.NewSpace("go-admin-test", nil)

	// The server returns topic and subscription configs with project numbers in
	// resource paths. These will not match a project id specified for integration
	// tests.
	testCmpOptions = []cmp.Option{
		cmpopts.IgnoreFields(SubscriptionPath{}, "Project"),
		cmpopts.IgnoreFields(TopicPath{}, "Project"),
	}
)

func initIntegrationTest(t *testing.T) {
	if testing.Short() {
		t.Skip("Integration tests skipped in short mode")
	}
	if testutil.ProjID() == "" {
		t.Skip("Integration tests skipped. See CONTRIBUTING.md for details")
	}
}

func withGRPCHeadersAssertion(t *testing.T, opts ...option.ClientOption) []option.ClientOption {
	grpcHeadersEnforcer := &testutil.HeadersEnforcer{
		OnFailure: t.Errorf,
		Checkers: []*testutil.HeaderChecker{
			testutil.XGoogClientHeaderChecker,
		},
	}
	return append(grpcHeadersEnforcer.CallOptions(), opts...)
}

func adminClient(ctx context.Context, t *testing.T, region string, opts ...option.ClientOption) *AdminClient {
	ts := testutil.TokenSource(ctx, vkit.DefaultAuthScopes()...)
	if ts == nil {
		t.Skip("Integration tests skipped. See CONTRIBUTING.md for details")
	}
	opts = append(withGRPCHeadersAssertion(t, option.WithTokenSource(ts)), opts...)
	admin, err := NewAdminClient(ctx, region, opts...)
	if err != nil {
		t.Fatalf("Failed to create admin client: %v", err)
	}
	return admin
}

func cleanUpTopic(ctx context.Context, t *testing.T, admin *AdminClient, name TopicPath) {
	if err := admin.DeleteTopic(ctx, name); err != nil {
		t.Errorf("Failed to delete topic %s: %v", name, err)
	}
}

func cleanUpSubscription(ctx context.Context, t *testing.T, admin *AdminClient, name SubscriptionPath) {
	if err := admin.DeleteSubscription(ctx, name); err != nil {
		t.Errorf("Failed to delete subscription %s: %v", name, err)
	}
}

func TestIntegration_ResourceAdminOperations(t *testing.T) {
	initIntegrationTest(t)

	ctx := context.Background()
	proj := testutil.ProjID()
	zone := test.RandomLiteZone()
	region, _ := ZoneToRegion(zone)
	resourceID := resourceIDs.New()

	locationPath := LocationPath{Project: proj, Zone: zone}
	topicPath := TopicPath{Project: proj, Zone: zone, TopicID: resourceID}
	subscriptionPath := SubscriptionPath{Project: proj, Zone: zone, SubscriptionID: resourceID}
	t.Logf("Topic path: %s", topicPath)

	admin := adminClient(ctx, t, region)
	defer admin.Close()

	// Topic admin operations.
	newTopicConfig := &TopicConfig{
		Name:                       topicPath,
		PartitionCount:             2,
		PublishCapacityMiBPerSec:   4,
		SubscribeCapacityMiBPerSec: 4,
		PerPartitionBytes:          30 * gibi,
		RetentionDuration:          time.Duration(24 * time.Hour),
	}

	gotTopicConfig, err := admin.CreateTopic(ctx, *newTopicConfig)
	if err != nil {
		t.Fatalf("Failed to create topic: %v", err)
	}
	defer cleanUpTopic(ctx, t, admin, topicPath)
	if diff := testutil.Diff(gotTopicConfig, newTopicConfig, testCmpOptions...); diff != "" {
		t.Errorf("CreateTopic() got: -, want: +\n%s", diff)
	}

	if gotTopicConfig, err := admin.Topic(ctx, topicPath); err != nil {
		t.Errorf("Failed to get topic: %v", err)
	} else if diff := testutil.Diff(gotTopicConfig, newTopicConfig, testCmpOptions...); diff != "" {
		t.Errorf("Topic() got: -, want: +\n%s", diff)
	}

	if gotTopicPartitions, err := admin.TopicPartitions(ctx, topicPath); err != nil {
		t.Errorf("Failed to get topic partitions: %v", err)
	} else if gotTopicPartitions != newTopicConfig.PartitionCount {
		t.Errorf("TopicPartitions() got: %v, want: %v", gotTopicPartitions, newTopicConfig.PartitionCount)
	}

	topicIt := admin.Topics(ctx, locationPath)
	var foundTopic *TopicConfig
	for {
		topic, err := topicIt.Next()
		if err == iterator.Done {
			break
		}
		if testutil.Equal(topic.Name, topicPath, testCmpOptions...) {
			foundTopic = topic
			break
		}
	}
	if foundTopic == nil {
		t.Error("Topics() did not return topic config")
	} else if diff := testutil.Diff(foundTopic, newTopicConfig, testCmpOptions...); diff != "" {
		t.Errorf("Topics() found config: -, want: +\n%s", diff)
	}

	topicUpdate1 := TopicConfigToUpdate{
		Name:                       topicPath,
		PublishCapacityMiBPerSec:   6,
		SubscribeCapacityMiBPerSec: 8,
	}
	wantUpdatedTopicConfig1 := &TopicConfig{
		Name:                       topicPath,
		PartitionCount:             2,
		PublishCapacityMiBPerSec:   6,
		SubscribeCapacityMiBPerSec: 8,
		PerPartitionBytes:          30 * gibi,
		RetentionDuration:          time.Duration(24 * time.Hour),
	}
	if gotTopicConfig, err := admin.UpdateTopic(ctx, topicUpdate1); err != nil {
		t.Errorf("Failed to update topic: %v", err)
	} else if diff := testutil.Diff(gotTopicConfig, wantUpdatedTopicConfig1, testCmpOptions...); diff != "" {
		t.Errorf("UpdateTopic() got: -, want: +\n%s", diff)
	}

	topicUpdate2 := TopicConfigToUpdate{
		Name:              topicPath,
		PerPartitionBytes: 35 * gibi,
		RetentionDuration: InfiniteRetention,
	}
	wantUpdatedTopicConfig2 := &TopicConfig{
		Name:                       topicPath,
		PartitionCount:             2,
		PublishCapacityMiBPerSec:   6,
		SubscribeCapacityMiBPerSec: 8,
		PerPartitionBytes:          35 * gibi,
		RetentionDuration:          InfiniteRetention,
	}
	if gotTopicConfig, err := admin.UpdateTopic(ctx, topicUpdate2); err != nil {
		t.Errorf("Failed to update topic: %v", err)
	} else if diff := testutil.Diff(gotTopicConfig, wantUpdatedTopicConfig2, testCmpOptions...); diff != "" {
		t.Errorf("UpdateTopic() got: -, want: +\n%s", diff)
	}

	// Subscription admin operations.
	newSubsConfig := &SubscriptionConfig{
		Name:                subscriptionPath,
		Topic:               topicPath,
		DeliveryRequirement: DeliverImmediately,
	}

	gotSubsConfig, err := admin.CreateSubscription(ctx, *newSubsConfig)
	if err != nil {
		t.Fatalf("Failed to create subscription: %v", err)
	}
	defer cleanUpSubscription(ctx, t, admin, subscriptionPath)
	if diff := testutil.Diff(gotSubsConfig, newSubsConfig, testCmpOptions...); diff != "" {
		t.Errorf("CreateSubscription() got: -, want: +\n%s", diff)
	}

	if gotSubsConfig, err := admin.Subscription(ctx, subscriptionPath); err != nil {
		t.Errorf("Failed to get subscription: %v", err)
	} else if diff := testutil.Diff(gotSubsConfig, newSubsConfig, testCmpOptions...); diff != "" {
		t.Errorf("Subscription() got: -, want: +\n%s", diff)
	}

	subsIt := admin.Subscriptions(ctx, locationPath)
	var foundSubs *SubscriptionConfig
	for {
		subs, err := subsIt.Next()
		if err == iterator.Done {
			break
		}
		if testutil.Equal(subs.Name, subscriptionPath, testCmpOptions...) {
			foundSubs = subs
			break
		}
	}
	if foundSubs == nil {
		t.Error("Subscriptions() did not return subscription config")
	} else if diff := testutil.Diff(foundSubs, gotSubsConfig, testCmpOptions...); diff != "" {
		t.Errorf("Subscriptions() found config: -, want: +\n%s", diff)
	}

	subsPathIt := admin.TopicSubscriptions(ctx, topicPath)
	foundSubsPath := false
	for {
		subsPath, err := subsPathIt.Next()
		if err == iterator.Done {
			break
		}
		if testutil.Equal(subsPath, subscriptionPath, testCmpOptions...) {
			foundSubsPath = true
			break
		}
	}
	if !foundSubsPath {
		t.Error("TopicSubscriptions() did not return subscription path")
	}

	subsUpdate := SubscriptionConfigToUpdate{
		Name:                subscriptionPath,
		DeliveryRequirement: DeliverAfterStored,
	}
	wantUpdatedSubsConfig := &SubscriptionConfig{
		Name:                subscriptionPath,
		Topic:               topicPath,
		DeliveryRequirement: DeliverAfterStored,
	}
	if gotSubsConfig, err := admin.UpdateSubscription(ctx, subsUpdate); err != nil {
		t.Errorf("Failed to update subscription: %v", err)
	} else if diff := testutil.Diff(gotSubsConfig, wantUpdatedSubsConfig, testCmpOptions...); diff != "" {
		t.Errorf("UpdateSubscription() got: -, want: +\n%s", diff)
	}
}

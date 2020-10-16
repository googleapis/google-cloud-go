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
	"fmt"
	"math/rand"
	"testing"
	"time"

	"cloud.google.com/go/internal/testutil"
	"google.golang.org/api/iterator"
	"google.golang.org/api/option"

	vkit "cloud.google.com/go/pubsublite/apiv1"
)

const gibi = 1 << 30

var (
	rng *rand.Rand

	// A random zone is selected for each integration test run.
	supportedZones = []string{
		"us-central1-a",
		"us-central1-b",
		"us-central1-c",
		"europe-west1-b",
		"europe-west1-d",
	}
)

func initIntegrationTest(t *testing.T) {
	if testing.Short() {
		t.Skip("Integration tests skipped in short mode")
	}
	if testutil.ProjID() == "" {
		t.Skip("Integration tests skipped. See CONTRIBUTING.md for details")
	}
	rng = testutil.NewRand(time.Now())
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

func adminClient(t *testing.T, ctx context.Context, region string, opts ...option.ClientOption) *Client {
	ts := testutil.TokenSource(ctx, vkit.DefaultAuthScopes()...)
	if ts == nil {
		t.Skip("Integration tests skipped. See CONTRIBUTING.md for details")
	}
	opts = append(withGRPCHeadersAssertion(t, option.WithTokenSource(ts)), opts...)
	client, err := NewAdminClient(ctx, region, opts...)
	if err != nil {
		t.Fatalf("Failed to create admin client: %v", err)
	}
	return client
}

func cleanUpTopic(ctx context.Context, t *testing.T, client *Client, name TopicPath) {
	if err := client.DeleteTopic(ctx, name); err != nil {
		t.Errorf("Failed to delete topic %s: %v", name, err)
	}
}

func cleanUpSubscription(ctx context.Context, t *testing.T, client *Client, name SubscriptionPath) {
	if err := client.DeleteSubscription(ctx, name); err != nil {
		t.Errorf("Failed to delete subscription %s: %v", name, err)
	}
}

func randomLiteZone() string {
	i := rng.Int63() % int64(len(supportedZones))
	return supportedZones[i]
}

func TestResourceAdminOperations(t *testing.T) {
	initIntegrationTest(t)

	ctx := context.Background()
	proj := testutil.ProjID()
	zone := randomLiteZone()
	region, _ := ZoneToRegion(zone)
	resourceID := fmt.Sprintf("go-test-admin-%d", rng.Int63())

	locationPath := LocationPath{Project: proj, Zone: zone}
	topicPath := TopicPath{Project: proj, Zone: zone, TopicID: resourceID}
	subscriptionPath := SubscriptionPath{Project: proj, Zone: zone, SubscriptionID: resourceID}
	t.Logf("Topic path: %s", topicPath)

	client := adminClient(t, ctx, region)
	defer client.Close()

	// Topic admin operations.
	newTopicConfig := &TopicConfig{
		Name:                       topicPath,
		PartitionCount:             2,
		PublishCapacityMiBPerSec:   4,
		SubscribeCapacityMiBPerSec: 4,
		PerPartitionBytes:          30 * gibi,
		RetentionDuration:          time.Duration(24 * time.Hour),
	}

	gotTopicConfig, err := client.CreateTopic(ctx, newTopicConfig)
	if err != nil {
		t.Fatalf("Failed to create topic: %v", err)
	}
	defer cleanUpTopic(ctx, t, client, topicPath)
	if !testutil.Equal(gotTopicConfig, newTopicConfig) {
		t.Errorf("CreateTopic() got: %v, want: %v", gotTopicConfig, newTopicConfig)
	}

	if gotTopicConfig, err := client.Topic(ctx, topicPath); err != nil {
		t.Errorf("Failed to get topic: %v", err)
	} else if !testutil.Equal(gotTopicConfig, newTopicConfig) {
		t.Errorf("Topic() got: %v, want: %v", gotTopicConfig, newTopicConfig)
	}

	if gotTopicPartitions, err := client.TopicPartitions(ctx, topicPath); err != nil {
		t.Errorf("Failed to get topic partitions: %v", err)
	} else if gotTopicPartitions != newTopicConfig.PartitionCount {
		t.Errorf("TopicPartitions() got: %v, want: %v", gotTopicPartitions, newTopicConfig.PartitionCount)
	}

	topicIt := client.Topics(ctx, locationPath)
	var foundTopic *TopicConfig
	for {
		topic, err := topicIt.Next()
		if err == iterator.Done {
			break
		}
		if testutil.Equal(topic.Name, topicPath) {
			foundTopic = topic
			break
		}
	}
	if foundTopic == nil {
		t.Error("Topics() did not return topic config")
	} else if !testutil.Equal(foundTopic, newTopicConfig) {
		t.Errorf("Topics() found config: %v, want: %v", foundTopic, newTopicConfig)
	}

	topicUpdate1 := &TopicConfigToUpdate{
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
	if gotTopicConfig, err := client.UpdateTopic(ctx, topicUpdate1); err != nil {
		t.Errorf("Failed to update topic: %v", err)
	} else if !testutil.Equal(gotTopicConfig, wantUpdatedTopicConfig1) {
		t.Errorf("UpdateTopic() got: %v, want: %v", gotTopicConfig, wantUpdatedTopicConfig1)
	}

	topicUpdate2 := &TopicConfigToUpdate{
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
	}
	if gotTopicConfig, err := client.UpdateTopic(ctx, topicUpdate2); err != nil {
		t.Errorf("Failed to update topic: %v", err)
	} else if !testutil.Equal(gotTopicConfig, wantUpdatedTopicConfig2) {
		t.Errorf("UpdateTopic() got: %v, want: %v", gotTopicConfig, wantUpdatedTopicConfig2)
	}

	// Subscription admin operations.
	newSubsConfig := &SubscriptionConfig{
		Name:                subscriptionPath,
		Topic:               topicPath,
		DeliveryRequirement: DeliverImmediately,
	}

	gotSubsConfig, err := client.CreateSubscription(ctx, newSubsConfig)
	if err != nil {
		t.Fatalf("Failed to create subscription: %v", err)
	}
	defer cleanUpSubscription(ctx, t, client, subscriptionPath)
	if !testutil.Equal(gotSubsConfig, newSubsConfig) {
		t.Errorf("CreateSubscription() got: %v, want: %v", gotSubsConfig, newSubsConfig)
	}

	if gotSubsConfig, err := client.Subscription(ctx, subscriptionPath); err != nil {
		t.Errorf("Failed to get subscription: %v", err)
	} else if !testutil.Equal(gotSubsConfig, newSubsConfig) {
		t.Errorf("Subscription() got: %v, want: %v", gotSubsConfig, newSubsConfig)
	}

	subsIt := client.Subscriptions(ctx, locationPath)
	var foundSubs *SubscriptionConfig
	for {
		subs, err := subsIt.Next()
		if err == iterator.Done {
			break
		}
		if testutil.Equal(subs.Name, subscriptionPath) {
			foundSubs = subs
			break
		}
	}
	if foundSubs == nil {
		t.Error("Subscriptions() did not return subscription config")
	} else if !testutil.Equal(foundSubs, gotSubsConfig) {
		t.Errorf("Subscriptions() found config: %v, want: %v", foundSubs, gotSubsConfig)
	}

	if subsPathIt, err := client.TopicSubscriptions(ctx, topicPath); err != nil {
		t.Errorf("Failed to list topic subscriptions: %v", err)
	} else {
		foundSubsPath := false
		for {
			subsPath, err := subsPathIt.Next()
			if err == iterator.Done {
				break
			}
			if testutil.Equal(subsPath, subscriptionPath) {
				foundSubsPath = true
				break
			}
		}
		if !foundSubsPath {
			t.Error("TopicSubscriptions() did not return subscription path")
		}
	}

	subsUpdate := &SubscriptionConfigToUpdate{
		Name:                subscriptionPath,
		DeliveryRequirement: DeliverAfterStored,
	}
	wantUpdatedSubsConfig := &SubscriptionConfig{
		Name:                subscriptionPath,
		Topic:               topicPath,
		DeliveryRequirement: DeliverAfterStored,
	}
	if gotSubsConfig, err := client.UpdateSubscription(ctx, subsUpdate); err != nil {
		t.Errorf("Failed to update subscription: %v", err)
	} else if !testutil.Equal(gotSubsConfig, wantUpdatedSubsConfig) {
		t.Errorf("UpdateSubscription() got: %v, want: %v", gotSubsConfig, wantUpdatedSubsConfig)
	}
}

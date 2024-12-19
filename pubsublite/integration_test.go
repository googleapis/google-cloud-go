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
	"testing"
	"time"

	"cloud.google.com/go/internal/testutil"
	"cloud.google.com/go/internal/uid"
	"cloud.google.com/go/pubsub"
	"cloud.google.com/go/pubsublite/internal/test"
	"cloud.google.com/go/pubsublite/internal/wire"
	"google.golang.org/api/iterator"
	"google.golang.org/api/option"

	vkit "cloud.google.com/go/pubsublite/apiv1"
	cloudresourcemanager "google.golang.org/api/cloudresourcemanager/v1"
)

const gibi = 1 << 30

var (
	resourceIDs = uid.NewSpace("go-admin-test", nil)
)

func initIntegrationTest(t *testing.T) {
	if testing.Short() {
		t.Skip("Integration tests skipped in short mode")
	}
	if testutil.ProjID() == "" {
		t.Skip("Integration tests skipped. See CONTRIBUTING.md for details")
	}
}

func projectNumber(t *testing.T) string {
	projID := testutil.ProjID()
	if projID == "" {
		return ""
	}
	// Pub/Sub Lite returns project numbers in resource paths, so we need to
	// convert from project id to numbers for simpler comparisons in tests.
	crm, err := cloudresourcemanager.NewService(context.Background())
	if err != nil {
		t.Fatalf("Failed to create cloudresourcemanager: %v", err)
	}
	project, err := crm.Projects.Get(projID).Do()
	if err != nil {
		t.Fatalf("Failed to retrieve project %q: %v", projID, err)
	}
	return fmt.Sprintf("%d", project.ProjectNumber)
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

func pubsubClient(ctx context.Context, t *testing.T, opts ...option.ClientOption) *pubsub.Client {
	ts := testutil.TokenSource(ctx, vkit.DefaultAuthScopes()...)
	if ts == nil {
		t.Skip("Integration tests skipped. See CONTRIBUTING.md for details")
	}
	opts = append(withGRPCHeadersAssertion(t, option.WithTokenSource(ts)), opts...)
	client, err := pubsub.NewClient(ctx, testutil.ProjID())
	if err != nil {
		t.Fatalf("Failed to create pubsub client: %v", err)
	}
	return client
}

func cleanUpReservation(ctx context.Context, t *testing.T, admin *AdminClient, name string) {
	if err := admin.DeleteReservation(ctx, name); err != nil {
		t.Errorf("Failed to delete reservation %s: %v", name, err)
	}
}

func cleanUpTopic(ctx context.Context, t *testing.T, admin *AdminClient, name string) {
	if err := admin.DeleteTopic(ctx, name); err != nil {
		t.Errorf("Failed to delete topic %s: %v", name, err)
	}
}

func cleanUpPubsubTopic(ctx context.Context, t *testing.T, topic *pubsub.Topic) {
	if err := topic.Delete(ctx); err != nil {
		t.Errorf("Failed to delete pubsub topic %s: %v", topic, err)
	}
}

func cleanUpSubscription(ctx context.Context, t *testing.T, admin *AdminClient, name string) {
	if err := admin.DeleteSubscription(ctx, name); err != nil {
		t.Errorf("Failed to delete subscription %s: %v", name, err)
	}
}

func validateNewSeekOperation(t *testing.T, subscription string, seekOp *SeekSubscriptionOperation) {
	t.Helper()

	if len(seekOp.Name()) == 0 {
		t.Error("Seek operation path missing")
	}
	if got, want := seekOp.Done(), false; got != want {
		t.Errorf("Operation.Done() got: %v, want: %v", got, want)
	}

	m, err := seekOp.Metadata()
	if err != nil {
		t.Errorf("Operation.Metadata() got err: %v", err)
		return
	}
	if got, want := m.Target, subscription; got != want {
		t.Errorf("Metadata.Target got: %v, want: %v", got, want)
	}
	if len(m.Verb) == 0 {
		t.Error("Metadata.Verb missing")
	}
	if m.CreateTime.IsZero() {
		t.Error("Metadata.CreateTime missing")
	}
}

func TestIntegration_ResourceAdminOperations(t *testing.T) {
	initIntegrationTest(t)

	ctx := context.Background()
	proj := projectNumber(t)
	zone := test.RandomLiteZone()
	region, _ := wire.LocationToRegion(zone)
	resourceID := resourceIDs.New()

	locationPath := wire.LocationPath{Project: proj, Location: zone}.String()
	topicPath := wire.TopicPath{Project: proj, Location: zone, TopicID: resourceID}.String()
	pubsubTopicPath := fmt.Sprintf("projects/%s/topics/%s", proj, resourceID)
	subscriptionPath := wire.SubscriptionPath{Project: proj, Location: zone, SubscriptionID: resourceID}.String()
	exportSubscriptionPath := wire.SubscriptionPath{Project: proj, Location: zone, SubscriptionID: resourceID + "export"}.String()
	reservationPath := wire.ReservationPath{Project: proj, Region: region, ReservationID: resourceID}.String()
	t.Logf("Topic path: %s", topicPath)

	admin := adminClient(ctx, t, region)
	defer admin.Close()

	// Reservation admin operations.
	newResConfig := &ReservationConfig{
		Name:               reservationPath,
		ThroughputCapacity: 3,
	}

	gotResConfig, err := admin.CreateReservation(ctx, *newResConfig)
	if err != nil {
		t.Fatalf("Failed to create reservation: %v", err)
	}
	defer cleanUpReservation(ctx, t, admin, reservationPath)
	if diff := testutil.Diff(gotResConfig, newResConfig); diff != "" {
		t.Errorf("CreateReservation() got: -, want: +\n%s", diff)
	}

	if gotResConfig, err := admin.Reservation(ctx, reservationPath); err != nil {
		t.Errorf("Failed to get reservation: %v", err)
	} else if diff := testutil.Diff(gotResConfig, newResConfig); diff != "" {
		t.Errorf("Reservation() got: -, want: +\n%s", diff)
	}

	testutil.Retry(t, 4, 30*time.Second, func(r *testutil.R) {
		resIt := admin.Reservations(ctx, wire.LocationPath{Project: proj, Location: region}.String())
		var foundRes *ReservationConfig
		for {
			res, err := resIt.Next()
			if err == iterator.Done {
				break
			}
			if res.Name == reservationPath {
				foundRes = res
				break
			}
		}
		if foundRes == nil {
			r.Errorf("Reservations() did not return reservation config")
		} else if diff := testutil.Diff(foundRes, newResConfig); diff != "" {
			r.Errorf("Reservations() found config: -, want: +\n%s", diff)
		}
	})

	resUpdate := ReservationConfigToUpdate{
		Name:               reservationPath,
		ThroughputCapacity: 4,
	}
	wantUpdatedResConfig := &ReservationConfig{
		Name:               reservationPath,
		ThroughputCapacity: 4,
	}
	if gotResConfig, err := admin.UpdateReservation(ctx, resUpdate); err != nil {
		t.Errorf("Failed to update reservation: %v", err)
	} else if diff := testutil.Diff(gotResConfig, wantUpdatedResConfig); diff != "" {
		t.Errorf("UpdateReservation() got: -, want: +\n%s", diff)
	}

	// Topic admin operations.
	newTopicConfig := &TopicConfig{
		Name:                       topicPath,
		PartitionCount:             1,
		PublishCapacityMiBPerSec:   4,
		SubscribeCapacityMiBPerSec: 4,
		PerPartitionBytes:          30 * gibi,
		RetentionDuration:          24 * time.Hour,
		ThroughputReservation:      reservationPath,
	}

	gotTopicConfig, err := admin.CreateTopic(ctx, *newTopicConfig)
	if err != nil {
		t.Fatalf("Failed to create topic: %v", err)
	}
	defer cleanUpTopic(ctx, t, admin, topicPath)
	if diff := testutil.Diff(gotTopicConfig, newTopicConfig); diff != "" {
		t.Errorf("CreateTopic() got: -, want: +\n%s", diff)
	}

	if gotTopicConfig, err := admin.Topic(ctx, topicPath); err != nil {
		t.Errorf("Failed to get topic: %v", err)
	} else if diff := testutil.Diff(gotTopicConfig, newTopicConfig); diff != "" {
		t.Errorf("Topic() got: -, want: +\n%s", diff)
	}

	if gotTopicPartitions, err := admin.TopicPartitionCount(ctx, topicPath); err != nil {
		t.Errorf("Failed to get topic partitions: %v", err)
	} else if gotTopicPartitions != newTopicConfig.PartitionCount {
		t.Errorf("TopicPartitionCount() got: %v, want: %v", gotTopicPartitions, newTopicConfig.PartitionCount)
	}

	testutil.Retry(t, 4, 30*time.Second, func(r *testutil.R) {
		topicIt := admin.Topics(ctx, locationPath)
		var foundTopic *TopicConfig
		for {
			topic, err := topicIt.Next()
			if err == iterator.Done {
				break
			}
			if topic.Name == topicPath {
				foundTopic = topic
				break
			}
		}
		if foundTopic == nil {
			r.Errorf("Topics() did not return topic config")
		} else if diff := testutil.Diff(foundTopic, newTopicConfig); diff != "" {
			r.Errorf("Topics() found config: -, want: +\n%s", diff)
		}
	})

	testutil.Retry(t, 4, 30*time.Second, func(r *testutil.R) {
		topicPathIt := admin.ReservationTopics(ctx, reservationPath)
		foundTopicPath := false
		for {
			path, err := topicPathIt.Next()
			if err == iterator.Done {
				break
			}
			if topicPath == path {
				foundTopicPath = true
				break
			}
		}
		if !foundTopicPath {
			r.Errorf("ReservationTopics() did not return topic path")
		}
	})

	topicUpdate1 := TopicConfigToUpdate{
		Name:                       topicPath,
		PartitionCount:             2,
		PublishCapacityMiBPerSec:   6,
		SubscribeCapacityMiBPerSec: 8,
		ThroughputReservation:      "",
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
	} else if diff := testutil.Diff(gotTopicConfig, wantUpdatedTopicConfig1); diff != "" {
		t.Errorf("UpdateTopic() got: -, want: +\n%s", diff)
	}

	topicUpdate2 := TopicConfigToUpdate{
		Name:                  topicPath,
		PerPartitionBytes:     35 * gibi,
		RetentionDuration:     InfiniteRetention,
		ThroughputReservation: reservationPath,
	}
	wantUpdatedTopicConfig2 := &TopicConfig{
		Name:                       topicPath,
		PartitionCount:             2,
		PublishCapacityMiBPerSec:   6,
		SubscribeCapacityMiBPerSec: 8,
		PerPartitionBytes:          35 * gibi,
		RetentionDuration:          InfiniteRetention,
		ThroughputReservation:      reservationPath,
	}
	if gotTopicConfig, err := admin.UpdateTopic(ctx, topicUpdate2); err != nil {
		t.Errorf("Failed to update topic: %v", err)
	} else if diff := testutil.Diff(gotTopicConfig, wantUpdatedTopicConfig2); diff != "" {
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
	if diff := testutil.Diff(gotSubsConfig, newSubsConfig); diff != "" {
		t.Errorf("CreateSubscription() got: -, want: +\n%s", diff)
	}

	if gotSubsConfig, err := admin.Subscription(ctx, subscriptionPath); err != nil {
		t.Errorf("Failed to get subscription: %v", err)
	} else if diff := testutil.Diff(gotSubsConfig, newSubsConfig); diff != "" {
		t.Errorf("Subscription() got: -, want: +\n%s", diff)
	}

	testutil.Retry(t, 4, 30*time.Second, func(r *testutil.R) {
		subsIt := admin.Subscriptions(ctx, locationPath)
		var foundSubs *SubscriptionConfig
		for {
			subs, err := subsIt.Next()
			if err == iterator.Done {
				break
			}
			if subs.Name == subscriptionPath {
				foundSubs = subs
				break
			}
		}
		if foundSubs == nil {
			r.Errorf("Subscriptions() did not return subscription config")
		} else if diff := testutil.Diff(foundSubs, gotSubsConfig); diff != "" {
			r.Errorf("Subscriptions() found config: -, want: +\n%s", diff)
		}
	})

	testutil.Retry(t, 4, 30*time.Second, func(r *testutil.R) {
		subsPathIt := admin.TopicSubscriptions(ctx, topicPath)
		foundSubsPath := false
		for {
			subsPath, err := subsPathIt.Next()
			if err == iterator.Done {
				break
			}
			if subsPath == subscriptionPath {
				foundSubsPath = true
				break
			}
		}
		if !foundSubsPath {
			r.Errorf("TopicSubscriptions() did not return subscription path")
		}
	})

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
	} else if diff := testutil.Diff(gotSubsConfig, wantUpdatedSubsConfig); diff != "" {
		t.Errorf("UpdateSubscription() got: -, want: +\n%s", diff)
	}

	// Seek subscription.
	if seekOp, err := admin.SeekSubscription(ctx, subscriptionPath, Beginning); err != nil {
		t.Errorf("SeekSubscription() got err: %v", err)
	} else {
		validateNewSeekOperation(t, subscriptionPath, seekOp)
	}

	// Create an export subscription to a Pub/Sub topic.
	client := pubsubClient(ctx, t)
	defer client.Close()
	pubsubTopic, err := client.CreateTopic(ctx, resourceID)
	if err != nil {
		t.Fatalf("Failed to create pubsub topic: %v", err)
	}
	defer cleanUpPubsubTopic(ctx, t, pubsubTopic)
	t.Logf("Pub/Sub topic: %s", pubsubTopic)

	newExportSubsConfig := &SubscriptionConfig{
		Name:                exportSubscriptionPath,
		Topic:               topicPath,
		DeliveryRequirement: DeliverImmediately,
		ExportConfig: &ExportConfig{
			DesiredState: ExportActive,
			CurrentState: ExportActive,
			Destination:  &PubSubDestinationConfig{Topic: pubsubTopicPath},
		},
	}

	gotExportSubsConfig, err := admin.CreateSubscription(ctx, *newExportSubsConfig, AtTargetLocation(PublishTime(time.Now())))
	if err != nil {
		t.Fatalf("Failed to create export subscription: %v", err)
	}
	defer cleanUpSubscription(ctx, t, admin, exportSubscriptionPath)
	if diff := testutil.Diff(gotExportSubsConfig, newExportSubsConfig); diff != "" {
		t.Errorf("CreateSubscription() got: -, want: +\n%s", diff)
	}

	if gotExportSubsConfig, err := admin.Subscription(ctx, exportSubscriptionPath); err != nil {
		t.Errorf("Failed to get export subscription: %v", err)
	} else if diff := testutil.Diff(gotExportSubsConfig, newExportSubsConfig); diff != "" {
		t.Errorf("Subscription() got: -, want: +\n%s", diff)
	}

	exportSubsUpdate := SubscriptionConfigToUpdate{
		Name: exportSubscriptionPath,
		ExportConfig: &ExportConfigToUpdate{
			DesiredState: ExportPaused,
		},
	}
	wantUpdatedExportSubsConfig := &SubscriptionConfig{
		Name:                exportSubscriptionPath,
		Topic:               topicPath,
		DeliveryRequirement: DeliverImmediately,
		ExportConfig: &ExportConfig{
			DesiredState: ExportPaused,
			CurrentState: ExportPaused,
			Destination:  &PubSubDestinationConfig{Topic: pubsubTopicPath},
		},
	}
	if gotExportSubsConfig, err := admin.UpdateSubscription(ctx, exportSubsUpdate); err != nil {
		t.Errorf("Failed to update export subscription: %v", err)
	} else if diff := testutil.Diff(gotExportSubsConfig, wantUpdatedExportSubsConfig); diff != "" {
		t.Errorf("UpdateSubscription() got: -, want: +\n%s", diff)
	}
}

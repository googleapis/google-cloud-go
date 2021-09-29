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
	"cloud.google.com/go/pubsublite/internal/wire"
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
	pathCmpOptions = []cmp.Option{
		cmpopts.IgnoreFields(wire.TopicPath{}, "Project"),
		cmpopts.IgnoreFields(wire.SubscriptionPath{}, "Project"),
		cmpopts.IgnoreFields(wire.ReservationPath{}, "Project"),
	}
	configCmpOptions = []cmp.Option{
		cmp.Comparer(func(t1, t2 *TopicConfig) bool {
			return cmp.Equal(t1, t2, cmpopts.IgnoreFields(TopicConfig{}, "Name", "ThroughputReservation")) &&
				TopicPathsEqual(t1.Name, t2.Name) && ReservationPathsEqual(t1.ThroughputReservation, t2.ThroughputReservation, true)
		}),
		cmp.Comparer(func(s1, s2 *SubscriptionConfig) bool {
			return cmp.Equal(s1, s2, cmpopts.IgnoreFields(SubscriptionConfig{}, "Name", "Topic")) &&
				TopicPathsEqual(s1.Topic, s2.Topic) && SubscriptionPathsEqual(s1.Name, s2.Name)
		}),
		cmp.Comparer(func(r1, r2 *ReservationConfig) bool {
			return cmp.Equal(r1, r2, cmpopts.IgnoreFields(ReservationConfig{}, "Name")) &&
				ReservationPathsEqual(r1.Name, r2.Name, false)
		}),
	}
)

func TopicPathsEqual(topic1, topic2 string) bool {
	tp1, err := wire.ParseTopicPath(topic1)
	if err != nil {
		return false
	}
	tp2, err := wire.ParseTopicPath(topic2)
	if err != nil {
		return false
	}
	return cmp.Equal(tp1, tp2, pathCmpOptions...)
}

func SubscriptionPathsEqual(subscription1, subscription2 string) bool {
	sp1, err := wire.ParseSubscriptionPath(subscription1)
	if err != nil {
		return false
	}
	sp2, err := wire.ParseSubscriptionPath(subscription2)
	if err != nil {
		return false
	}
	return cmp.Equal(sp1, sp2, pathCmpOptions...)
}

func ReservationPathsEqual(reservation1, reservation2 string, allowEmpty bool) bool {
	if allowEmpty && len(reservation1)+len(reservation2) == 0 {
		return true
	}
	rp1, err := wire.ParseReservationPath(reservation1)
	if err != nil {
		return false
	}
	rp2, err := wire.ParseReservationPath(reservation2)
	if err != nil {
		return false
	}
	return cmp.Equal(rp1, rp2, pathCmpOptions...)
}

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
	if got, want := m.Target, subscription; !SubscriptionPathsEqual(got, want) {
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
	proj := testutil.ProjID()
	zone := test.RandomLiteZone()
	region, _ := wire.LocationToRegion(zone)
	resourceID := resourceIDs.New()

	locationPath := wire.LocationPath{Project: proj, Location: zone}.String()
	topicPath := wire.TopicPath{Project: proj, Location: zone, TopicID: resourceID}.String()
	subscriptionPath := wire.SubscriptionPath{Project: proj, Location: zone, SubscriptionID: resourceID}.String()
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
	if diff := testutil.Diff(gotResConfig, newResConfig, configCmpOptions...); diff != "" {
		t.Errorf("CreateReservation() got: -, want: +\n%s", diff)
	}

	if gotResConfig, err := admin.Reservation(ctx, reservationPath); err != nil {
		t.Errorf("Failed to get reservation: %v", err)
	} else if diff := testutil.Diff(gotResConfig, newResConfig, configCmpOptions...); diff != "" {
		t.Errorf("Reservation() got: -, want: +\n%s", diff)
	}

	resIt := admin.Reservations(ctx, wire.LocationPath{proj, region}.String())
	var foundRes *ReservationConfig
	for {
		res, err := resIt.Next()
		if err == iterator.Done {
			break
		}
		if ReservationPathsEqual(res.Name, reservationPath, false) {
			foundRes = res
			break
		}
	}
	if foundRes == nil {
		t.Error("Reservations() did not return reservation config")
	} else if diff := testutil.Diff(foundRes, newResConfig, configCmpOptions...); diff != "" {
		t.Errorf("Reservations() found config: -, want: +\n%s", diff)
	}

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
	} else if diff := testutil.Diff(gotResConfig, wantUpdatedResConfig, configCmpOptions...); diff != "" {
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
	if diff := testutil.Diff(gotTopicConfig, newTopicConfig, configCmpOptions...); diff != "" {
		t.Errorf("CreateTopic() got: -, want: +\n%s", diff)
	}

	if gotTopicConfig, err := admin.Topic(ctx, topicPath); err != nil {
		t.Errorf("Failed to get topic: %v", err)
	} else if diff := testutil.Diff(gotTopicConfig, newTopicConfig, configCmpOptions...); diff != "" {
		t.Errorf("Topic() got: -, want: +\n%s", diff)
	}

	if gotTopicPartitions, err := admin.TopicPartitionCount(ctx, topicPath); err != nil {
		t.Errorf("Failed to get topic partitions: %v", err)
	} else if gotTopicPartitions != newTopicConfig.PartitionCount {
		t.Errorf("TopicPartitionCount() got: %v, want: %v", gotTopicPartitions, newTopicConfig.PartitionCount)
	}

	topicIt := admin.Topics(ctx, locationPath)
	var foundTopic *TopicConfig
	for {
		topic, err := topicIt.Next()
		if err == iterator.Done {
			break
		}
		if TopicPathsEqual(topic.Name, topicPath) {
			foundTopic = topic
			break
		}
	}
	if foundTopic == nil {
		t.Error("Topics() did not return topic config")
	} else if diff := testutil.Diff(foundTopic, newTopicConfig, configCmpOptions...); diff != "" {
		t.Errorf("Topics() found config: -, want: +\n%s", diff)
	}

	topicPathIt := admin.ReservationTopics(ctx, reservationPath)
	foundTopicPath := false
	for {
		path, err := topicPathIt.Next()
		if err == iterator.Done {
			break
		}
		if TopicPathsEqual(topicPath, path) {
			foundTopicPath = true
			break
		}
	}
	if !foundTopicPath {
		t.Error("ReservationTopics() did not return topic path")
	}

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
	} else if diff := testutil.Diff(gotTopicConfig, wantUpdatedTopicConfig1, configCmpOptions...); diff != "" {
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
	} else if diff := testutil.Diff(gotTopicConfig, wantUpdatedTopicConfig2, configCmpOptions...); diff != "" {
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
	if diff := testutil.Diff(gotSubsConfig, newSubsConfig, configCmpOptions...); diff != "" {
		t.Errorf("CreateSubscription() got: -, want: +\n%s", diff)
	}

	if gotSubsConfig, err := admin.Subscription(ctx, subscriptionPath); err != nil {
		t.Errorf("Failed to get subscription: %v", err)
	} else if diff := testutil.Diff(gotSubsConfig, newSubsConfig, configCmpOptions...); diff != "" {
		t.Errorf("Subscription() got: -, want: +\n%s", diff)
	}

	subsIt := admin.Subscriptions(ctx, locationPath)
	var foundSubs *SubscriptionConfig
	for {
		subs, err := subsIt.Next()
		if err == iterator.Done {
			break
		}
		if SubscriptionPathsEqual(subs.Name, subscriptionPath) {
			foundSubs = subs
			break
		}
	}
	if foundSubs == nil {
		t.Error("Subscriptions() did not return subscription config")
	} else if diff := testutil.Diff(foundSubs, gotSubsConfig, configCmpOptions...); diff != "" {
		t.Errorf("Subscriptions() found config: -, want: +\n%s", diff)
	}

	subsPathIt := admin.TopicSubscriptions(ctx, topicPath)
	foundSubsPath := false
	for {
		subsPath, err := subsPathIt.Next()
		if err == iterator.Done {
			break
		}
		if SubscriptionPathsEqual(subsPath, subscriptionPath) {
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
	} else if diff := testutil.Diff(gotSubsConfig, wantUpdatedSubsConfig, configCmpOptions...); diff != "" {
		t.Errorf("UpdateSubscription() got: -, want: +\n%s", diff)
	}

	// Seek subscription.
	if seekOp, err := admin.SeekSubscription(ctx, subscriptionPath, Beginning); err != nil {
		t.Errorf("SeekSubscription() got err: %v", err)
	} else {
		validateNewSeekOperation(t, subscriptionPath, seekOp)
	}
}

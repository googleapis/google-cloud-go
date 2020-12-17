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

package ps

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"cloud.google.com/go/internal/testutil"
	"cloud.google.com/go/internal/uid"
	"cloud.google.com/go/pubsub"
	"cloud.google.com/go/pubsublite"
	"cloud.google.com/go/pubsublite/internal/test"
	"github.com/google/go-cmp/cmp/cmpopts"
	"google.golang.org/api/option"

	vkit "cloud.google.com/go/pubsublite/apiv1"
	pb "google.golang.org/genproto/googleapis/cloud/pubsublite/v1"
)

const (
	gibi               = 1 << 30
	defaultTestTimeout = 5 * time.Minute
)

var resourceIDs = uid.NewSpace("go-ps-test", nil)

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

func testOptions(ctx context.Context, t *testing.T, opts ...option.ClientOption) []option.ClientOption {
	ts := testutil.TokenSource(ctx, vkit.DefaultAuthScopes()...)
	if ts == nil {
		t.Skip("Integration tests skipped. See CONTRIBUTING.md for details")
	}
	return append(withGRPCHeadersAssertion(t, option.WithTokenSource(ts)), opts...)
}

func adminClient(ctx context.Context, t *testing.T, region string, opts ...option.ClientOption) *pubsublite.AdminClient {
	opts = testOptions(ctx, t, opts...)
	admin, err := pubsublite.NewAdminClient(ctx, region, opts...)
	if err != nil {
		t.Fatalf("Failed to create admin client: %v", err)
	}
	return admin
}

func publisherClient(ctx context.Context, t *testing.T, settings PublishSettings, topic pubsublite.TopicPath, opts ...option.ClientOption) *PublisherClient {
	opts = testOptions(ctx, t, opts...)
	pub, err := NewPublisherClient(ctx, settings, topic, opts...)
	if err != nil {
		t.Fatalf("Failed to create publisher client: %v", err)
	}
	return pub
}

func subscriberClient(ctx context.Context, t *testing.T, settings ReceiveSettings, subscription pubsublite.SubscriptionPath, opts ...option.ClientOption) *SubscriberClient {
	opts = testOptions(ctx, t, opts...)
	sub, err := NewSubscriberClient(ctx, settings, subscription, opts...)
	if err != nil {
		t.Fatalf("Failed to create publisher client: %v", err)
	}
	return sub
}

func initResourcePaths(t *testing.T) (string, pubsublite.TopicPath, pubsublite.SubscriptionPath) {
	initIntegrationTest(t)

	proj := testutil.ProjID()
	zone := test.RandomLiteZone()
	region, _ := pubsublite.ZoneToRegion(zone)
	resourceID := resourceIDs.New()

	topicPath := pubsublite.TopicPath{Project: proj, Zone: zone, TopicID: resourceID}
	subscriptionPath := pubsublite.SubscriptionPath{Project: proj, Zone: zone, SubscriptionID: resourceID}
	return region, topicPath, subscriptionPath
}

func createTopic(ctx context.Context, t *testing.T, admin *pubsublite.AdminClient, topic pubsublite.TopicPath, partitionCount int) {
	topicConfig := pubsublite.TopicConfig{
		Name:                       topic,
		PartitionCount:             partitionCount,
		PublishCapacityMiBPerSec:   4,
		SubscribeCapacityMiBPerSec: 8,
		PerPartitionBytes:          30 * gibi,
		RetentionDuration:          24 * time.Hour,
	}
	_, err := admin.CreateTopic(ctx, topicConfig)
	if err != nil {
		t.Fatalf("Failed to create topic %s: %v", topic, err)
	} else {
		t.Logf("Created topic %s", topic)
	}
}

func cleanUpTopic(ctx context.Context, t *testing.T, admin *pubsublite.AdminClient, topic pubsublite.TopicPath) {
	if err := admin.DeleteTopic(ctx, topic); err != nil {
		t.Errorf("Failed to delete topic %s: %v", topic, err)
	} else {
		t.Logf("Deleted topic %s", topic)
	}
}

func createSubscription(ctx context.Context, t *testing.T, admin *pubsublite.AdminClient, subscription pubsublite.SubscriptionPath, topic pubsublite.TopicPath) {
	subConfig := &pubsublite.SubscriptionConfig{
		Name:                subscription,
		Topic:               topic,
		DeliveryRequirement: pubsublite.DeliverImmediately,
	}
	_, err := admin.CreateSubscription(ctx, *subConfig)
	if err != nil {
		t.Fatalf("Failed to create subscription %s: %v", subscription, err)
	} else {
		t.Logf("Created subscription %s", subscription)
	}
}

func cleanUpSubscription(ctx context.Context, t *testing.T, admin *pubsublite.AdminClient, subscription pubsublite.SubscriptionPath) {
	if err := admin.DeleteSubscription(ctx, subscription); err != nil {
		t.Errorf("Failed to delete subscription %s: %v", subscription, err)
	} else {
		t.Logf("Deleted subscription %s", subscription)
	}
}

func partitionNumbers(partitionCount int) []int {
	var partitions []int
	for i := 0; i < partitionCount; i++ {
		partitions = append(partitions, i)
	}
	return partitions
}

func publishMessages(t *testing.T, settings PublishSettings, topic pubsublite.TopicPath, msgs ...*pubsub.Message) {
	ctx := context.Background()
	publisher := publisherClient(ctx, t, settings, topic)
	defer publisher.Stop()

	var pubResults []*pubsub.PublishResult
	for _, msg := range msgs {
		pubResults = append(pubResults, publisher.Publish(ctx, msg))
	}
	waitForPublishResults(t, pubResults)
}

func publishPrefixedMessages(t *testing.T, settings PublishSettings, topic pubsublite.TopicPath, msgPrefix string, messageCount int) []string {
	ctx := context.Background()
	publisher := publisherClient(ctx, t, settings, topic)
	defer publisher.Stop()

	orderingSender := test.NewOrderingSender()
	var pubResults []*pubsub.PublishResult
	var msgs []string
	for i := 0; i < messageCount; i++ {
		data := orderingSender.Next(msgPrefix)
		msgs = append(msgs, data)
		pubResults = append(pubResults, publisher.Publish(ctx, &pubsub.Message{Data: []byte(data)}))
	}
	waitForPublishResults(t, pubResults)
	return msgs
}

func waitForPublishResults(t *testing.T, pubResults []*pubsub.PublishResult) {
	cctx, cancel := context.WithTimeout(context.Background(), defaultTestTimeout)
	for i, result := range pubResults {
		_, err := result.Get(cctx)
		if err != nil {
			t.Errorf("Publish(%d) got err: %v", i, err)
		}
	}
	t.Logf("Published %d messages", len(pubResults))
	cancel()
}

const maxPrintMsgLen = 70

func truncateMsg(msg string) string {
	if len(msg) > maxPrintMsgLen {
		return fmt.Sprintf("%s...", msg[0:maxPrintMsgLen])
	}
	return msg
}

func messageDiff(got, want *pubsub.Message) string {
	return testutil.Diff(got, want, cmpopts.IgnoreUnexported(pubsub.Message{}), cmpopts.IgnoreFields(pubsub.Message{}, "ID", "PublishTime"), cmpopts.EquateEmpty())
}

type checkOrdering bool

func receiveAllMessages(t *testing.T, msgTracker *test.MsgTracker, settings ReceiveSettings, subscription pubsublite.SubscriptionPath, checkOrder checkOrdering) {
	cctx, stopSubscriber := context.WithTimeout(context.Background(), defaultTestTimeout)
	orderingValidator := test.NewOrderingReceiver()

	messageReceiver := func(ctx context.Context, msg *pubsub.Message) {
		msg.Ack()
		data := string(msg.Data)
		if !msgTracker.Remove(data) {
			// Prevent a flood of errors if a message for a previous test was found.
			t.Fatalf("Received unexpected message: %q", truncateMsg(data))
			return
		}
		if checkOrder {
			if err := orderingValidator.Receive(data, msg.OrderingKey); err != nil {
				t.Errorf("Received unordered message: %q", truncateMsg(data))
			}
		}
		if msgTracker.Empty() {
			stopSubscriber() // Stop the subscriber when all messages have been received
		}
	}

	subscriber := subscriberClient(cctx, t, settings, subscription)
	if err := subscriber.Receive(cctx, messageReceiver); err != nil {
		t.Errorf("Receive() got err: %v", err)
	}
	if err := msgTracker.Status(); err != nil {
		t.Error(err)
	}
}

func receiveAndVerifyMessage(t *testing.T, want *pubsub.Message, settings ReceiveSettings, subscription pubsublite.SubscriptionPath) {
	cctx, stopSubscriber := context.WithTimeout(context.Background(), defaultTestTimeout)

	messageReceiver := func(ctx context.Context, got *pubsub.Message) {
		got.Ack()
		stopSubscriber()

		if diff := messageDiff(got, want); diff != "" {
			t.Errorf("Received message got: -, want: +\n%s", diff)
		}
		if len(got.ID) == 0 {
			t.Error("Received message missing ID")
		}
		if got.PublishTime.IsZero() {
			t.Error("Received message missing PublishTime")
		}
	}

	subscriber := subscriberClient(cctx, t, settings, subscription)
	if err := subscriber.Receive(cctx, messageReceiver); err != nil {
		t.Errorf("Receive() got err: %v", err)
	}
}

func TestIntegration_PublishSubscribeSinglePartition(t *testing.T) {
	region, topicPath, subscriptionPath := initResourcePaths(t)
	ctx := context.Background()
	const partitionCount = 1
	recvSettings := DefaultReceiveSettings
	recvSettings.Partitions = partitionNumbers(partitionCount)

	admin := adminClient(ctx, t, region)
	defer admin.Close()
	createTopic(ctx, t, admin, topicPath, partitionCount)
	defer cleanUpTopic(ctx, t, admin, topicPath)
	createSubscription(ctx, t, admin, subscriptionPath, topicPath)
	defer cleanUpSubscription(ctx, t, admin, subscriptionPath)

	// The same topic and subscription resources are used for each subtest. This
	// implicitly verifies commits. If cursors are not successfully committed at
	// the end of each test, the next test will receive an incorrect message and
	// fail. The subtests can also be run independently.

	// Sets all fields for a message and ensures it is correctly received.
	t.Run("AllFieldsRoundTrip", func(t *testing.T) {
		msg := &pubsub.Message{
			Data:        []byte("round_trip"),
			OrderingKey: "ordering_key",
			Attributes: map[string]string{
				"attr1": "value1",
				"attr2": "value2",
			},
		}
		publishMessages(t, DefaultPublishSettings, topicPath, msg)
		receiveAndVerifyMessage(t, msg, recvSettings, subscriptionPath)
	})

	// Verifies custom message transformers.
	t.Run("CustomMessageTransformers", func(t *testing.T) {
		customPubSettings := DefaultPublishSettings
		customPubSettings.MessageTransformer = func(from *pubsub.Message, to *pb.PubSubMessage) error {
			to.Data = []byte(string(from.Data) + "_foo")
			to.Key = []byte(from.OrderingKey + "_foo")
			return nil
		}
		msg := &pubsub.Message{
			Data:        []byte("msg_transformers"),
			OrderingKey: "ordering_key",
			Attributes: map[string]string{
				"attr1": "value1",
			},
		}
		publishMessages(t, customPubSettings, topicPath, msg)

		customRecvSettings := recvSettings
		customRecvSettings.MessageTransformer = func(wireMsg *pb.SequencedMessage, msg *pubsub.Message) error {
			// Swaps data and key.
			msg.Data = wireMsg.GetMessage().GetKey()
			msg.OrderingKey = string(wireMsg.GetMessage().GetData())
			msg.ID = "FAKE_ID"
			msg.PublishTime = time.Now()
			return nil
		}
		want := &pubsub.Message{
			Data:        []byte("ordering_key_foo"),
			OrderingKey: "msg_transformers_foo",
		}
		receiveAndVerifyMessage(t, want, customRecvSettings, subscriptionPath)
	})

	// Verifies that nacks are correctly handled.
	t.Run("Nack", func(t *testing.T) {
		msg1 := &pubsub.Message{Data: []byte("nack_msg1")}
		msg2 := &pubsub.Message{Data: []byte("nack_msg2")}
		publishMessages(t, DefaultPublishSettings, topicPath, msg1, msg2)

		// Case A: Default nack handler. Terminates subscriber.
		cctx, _ := context.WithTimeout(context.Background(), defaultTestTimeout)
		messageReceiver1 := func(ctx context.Context, got *pubsub.Message) {
			if diff := messageDiff(got, msg1); diff != "" {
				t.Errorf("Received message got: -, want: +\n%s", diff)
			}
			got.Nack()
		}
		subscriber := subscriberClient(cctx, t, recvSettings, subscriptionPath)
		if gotErr := subscriber.Receive(cctx, messageReceiver1); !test.ErrorEqual(gotErr, errNackCalled) {
			t.Errorf("Receive() got err: (%v), want err: (%v)", gotErr, errNackCalled)
		}

		// Case B: Custom nack handler.
		errCustomNack := errors.New("message nacked")
		customSettings := recvSettings
		customSettings.NackHandler = func(msg *pubsub.Message) error {
			if string(msg.Data) == "nack_msg1" {
				return nil // Causes msg1 to be acked
			}
			if string(msg.Data) == "nack_msg2" {
				return errCustomNack // Terminates subscriber
			}
			return fmt.Errorf("Received unexpected message: %q", truncateMsg(string(msg.Data)))
		}
		subscriber = subscriberClient(cctx, t, customSettings, subscriptionPath)

		messageReceiver2 := func(ctx context.Context, got *pubsub.Message) {
			got.Nack()
		}
		if gotErr := subscriber.Receive(cctx, messageReceiver2); !test.ErrorEqual(gotErr, errCustomNack) {
			t.Errorf("Receive() got err: (%v), want err: (%v)", gotErr, errCustomNack)
		}

		// Finally: receive and ack msg2.
		receiveAndVerifyMessage(t, msg2, recvSettings, subscriptionPath)
	})

	// Verifies that SubscriberClient.Receive() can be invoked multiple times
	// serially (note: parallel would error).
	t.Run("SubscriberMultipleReceive", func(t *testing.T) {
		msgs := []*pubsub.Message{
			{Data: []byte("multiple_receive1")},
			{Data: []byte("multiple_receive2")},
			{Data: []byte("multiple_receive3")},
		}
		publishMessages(t, DefaultPublishSettings, topicPath, msgs...)

		var cctx context.Context
		var stopSubscriber context.CancelFunc
		var gotReceivedCount int32
		messageReceiver := func(ctx context.Context, got *pubsub.Message) {
			currentIdx := atomic.AddInt32(&gotReceivedCount, 1) - 1
			if diff := messageDiff(got, msgs[currentIdx]); diff != "" {
				t.Errorf("Received message got: -, want: +\n%s", diff)
			}
			got.Ack()
			stopSubscriber()
		}
		subscriber := subscriberClient(cctx, t, recvSettings, subscriptionPath)

		// The message receiver stops the subscriber after receiving the first
		// message. However, the subscriber isn't guaranteed to immediately stop, so
		// allow up to `len(msgs)` iterations.
		wantReceivedCount := len(msgs)
		for i := 0; i < wantReceivedCount; i++ {
			// New cctx must be created for each iteration as it is cancelled each
			// time stopSubscriber is called.
			cctx, stopSubscriber = context.WithTimeout(context.Background(), defaultTestTimeout)
			if err := subscriber.Receive(cctx, messageReceiver); err != nil {
				t.Errorf("Receive() got err: %v", err)
			}
			if int(gotReceivedCount) == wantReceivedCount {
				t.Logf("Received %d messages in %d iterations", gotReceivedCount, i+1)
				break
			}
		}
		if int(gotReceivedCount) != wantReceivedCount {
			t.Errorf("Received message count: got %d, want %d", gotReceivedCount, wantReceivedCount)
		}
	})

	// Verifies that a blocking message receiver is notified of shutdown.
	t.Run("BlockingMessageReceiver", func(t *testing.T) {
		msg := &pubsub.Message{
			Data: []byte("blocking_message_receiver"),
		}
		publishMessages(t, DefaultPublishSettings, topicPath, msg)

		cctx, stopSubscriber := context.WithTimeout(context.Background(), defaultSubscriberTestTimeout)
		messageReceiver := func(ctx context.Context, got *pubsub.Message) {
			if diff := messageDiff(got, msg); diff != "" {
				t.Errorf("Received message got: -, want: +\n%s", diff)
			}

			// Ensure the test is deterministic. Wait until the message is received,
			// then stop the subscriber, which would cause `ctx` to be done below.
			stopSubscriber()

			select {
			case <-time.After(defaultSubscriberTestTimeout):
				t.Errorf("MessageReceiverFunc context not closed within %v", defaultSubscriberTestTimeout)
			case <-ctx.Done():
			}

			// The commit offset for this ack should be processed since the subscriber
			// is not shut down due to fatal error. Not actually detected until the
			// next test, which would receive an incorrect message.
			got.Ack()
		}
		subscriber := subscriberClient(cctx, t, recvSettings, subscriptionPath)

		if err := subscriber.Receive(cctx, messageReceiver); err != nil {
			t.Errorf("Receive() got err: %v", err)
		}
	})

	// Checks that messages are published and received in order.
	t.Run("Ordering", func(t *testing.T) {
		const messageCount = 500
		const publishBatchSize = 10

		// Publish messages.
		pubSettings := DefaultPublishSettings
		pubSettings.CountThreshold = publishBatchSize
		pubSettings.DelayThreshold = 100 * time.Millisecond
		msgs := publishPrefixedMessages(t, pubSettings, topicPath, "ordering", messageCount)

		// Receive messages.
		msgTracker := test.NewMsgTracker()
		msgTracker.Add(msgs...)
		receiveAllMessages(t, msgTracker, recvSettings, subscriptionPath, checkOrdering(true))
	})

	// Checks that subscriber flow control works.
	t.Run("SubscriberFlowControl", func(t *testing.T) {
		const messageCount = 20
		const maxOutstandingMessages = 2 // Receive small batches of messages

		// Publish messages.
		msgs := publishPrefixedMessages(t, DefaultPublishSettings, topicPath, "subscriber_flow_control", messageCount)

		// Receive messages.
		msgTracker := test.NewMsgTracker()
		msgTracker.Add(msgs...)
		customSettings := recvSettings
		customSettings.MaxOutstandingMessages = maxOutstandingMessages
		receiveAllMessages(t, msgTracker, customSettings, subscriptionPath, checkOrdering(true))
	})

	// Verifies that large messages can be sent and received.
	t.Run("LargeMessages", func(t *testing.T) {
		const messageCount = 3
		const messageLen = MaxPublishRequestBytes - 50

		// Publish messages.
		msgTracker := test.NewMsgTracker()
		var msgs []*pubsub.Message
		for i := 0; i < messageCount; i++ {
			data := strings.Repeat(fmt.Sprintf("%d", i), messageLen)
			msgTracker.Add(data)
			msgs = append(msgs, &pubsub.Message{Data: []byte(data)})
		}
		publishMessages(t, DefaultPublishSettings, topicPath, msgs...)

		// Receive messages.
		receiveAllMessages(t, msgTracker, recvSettings, subscriptionPath, checkOrdering(false))
	})
}

func TestIntegration_PublishSubscribeMultiPartition(t *testing.T) {
	const partitionCount = 3
	region, topicPath, subscriptionPath := initResourcePaths(t)
	ctx := context.Background()
	recvSettings := DefaultReceiveSettings
	recvSettings.Partitions = partitionNumbers(partitionCount)

	admin := adminClient(ctx, t, region)
	defer admin.Close()
	createTopic(ctx, t, admin, topicPath, partitionCount)
	defer cleanUpTopic(ctx, t, admin, topicPath)
	createSubscription(ctx, t, admin, subscriptionPath, topicPath)
	defer cleanUpSubscription(ctx, t, admin, subscriptionPath)

	// The same topic and subscription resources are used for each subtest. This
	// implicitly verifies commits. If cursors are not successfully committed at
	// the end of each test, the next test will receive an incorrect message and
	// fail. The subtests can also be run independently.

	// Tests messages published without ordering key.
	t.Run("PublishRoutingNoKey", func(t *testing.T) {
		const messageCount = 50 * partitionCount

		// Publish messages.
		msgs := publishPrefixedMessages(t, DefaultPublishSettings, topicPath, "routing_no_key", messageCount)

		// Receive messages, not checking for ordering since they do not have a key.
		// However, they would still be ordered within their partition.
		msgTracker := test.NewMsgTracker()
		msgTracker.Add(msgs...)
		receiveAllMessages(t, msgTracker, recvSettings, subscriptionPath, checkOrdering(false))
	})

	// Tests messages published with ordering key.
	t.Run("PublishRoutingWithKey", func(t *testing.T) {
		const messageCountPerPartition = 100
		const publishBatchSize = 5 // Verifies ordering of batches

		// Publish messages.
		orderingSender := test.NewOrderingSender()
		msgTracker := test.NewMsgTracker()
		var msgs []*pubsub.Message
		for partition := 0; partition < partitionCount; partition++ {
			for i := 0; i < messageCountPerPartition; i++ {
				data := orderingSender.Next("routing_with_key")
				msgTracker.Add(data)
				msg := &pubsub.Message{
					Data:        []byte(data),
					OrderingKey: fmt.Sprintf("p%d", partition),
				}
				msgs = append(msgs, msg)
			}
		}

		pubSettings := DefaultPublishSettings
		pubSettings.CountThreshold = publishBatchSize
		publishMessages(t, pubSettings, topicPath, msgs...)

		// Receive messages.
		receiveAllMessages(t, msgTracker, recvSettings, subscriptionPath, checkOrdering(true))
	})

	// Verifies usage of the partition assignment service.
	t.Run("PartitionAssignment", func(t *testing.T) {
		const messageCount = 100
		const subscriberCount = 2 // Should be between [2, partitionCount]

		// Publish messages.
		msgs := publishPrefixedMessages(t, DefaultPublishSettings, topicPath, "partition_assignment", messageCount)

		// Start multiple subscribers that use partition assignment.
		msgTracker := test.NewMsgTracker()
		msgTracker.Add(msgs...)

		messageReceiver := func(ctx context.Context, msg *pubsub.Message) {
			msg.Ack()
			data := string(msg.Data)
			if !msgTracker.Remove(data) {
				t.Errorf("Received unexpected message: %q", truncateMsg(data))
			}
		}

		cctx, stopSubscribers := context.WithTimeout(context.Background(), defaultTestTimeout)
		for i := 0; i < subscriberCount; i++ {
			// Subscribers must be started in a goroutine as Receive() blocks.
			go func() {
				subscriber := subscriberClient(cctx, t, DefaultReceiveSettings, subscriptionPath)
				if err := subscriber.Receive(cctx, messageReceiver); err != nil {
					t.Errorf("Receive() got err: %v", err)
				}
			}()
		}

		// Wait until all messages have been received.
		msgTracker.Wait(defaultTestTimeout)
		stopSubscribers()
	})
}

func TestIntegration_SubscribeFanOut(t *testing.T) {
	// Creates multiple subscriptions for the same topic and ensures that each
	// subscription receives the published messages. This must be a standalone
	// test as the subscribers should not read from backlog.

	const subscriberCount = 3
	const partitionCount = 1
	const messageCount = 100
	region, topicPath, baseSubscriptionPath := initResourcePaths(t)
	ctx := context.Background()

	admin := adminClient(ctx, t, region)
	defer admin.Close()
	createTopic(ctx, t, admin, topicPath, partitionCount)
	defer cleanUpTopic(ctx, t, admin, topicPath)

	var subscriptionPaths []pubsublite.SubscriptionPath
	for i := 0; i < subscriberCount; i++ {
		subscription := baseSubscriptionPath
		subscription.SubscriptionID += fmt.Sprintf("%s-%d", baseSubscriptionPath.SubscriptionID, i)
		subscriptionPaths = append(subscriptionPaths, subscription)

		createSubscription(ctx, t, admin, subscription, topicPath)
		defer cleanUpSubscription(ctx, t, admin, subscription)
	}

	// Publish messages.
	msgs := publishPrefixedMessages(t, DefaultPublishSettings, topicPath, "fan_out", messageCount)

	// Receive messages from multiple subscriptions.
	recvSettings := DefaultReceiveSettings
	recvSettings.Partitions = partitionNumbers(partitionCount)

	for _, subscription := range subscriptionPaths {
		msgTracker := test.NewMsgTracker()
		msgTracker.Add(msgs...)
		receiveAllMessages(t, msgTracker, recvSettings, subscription, checkOrdering(partitionCount == 1))
	}
}

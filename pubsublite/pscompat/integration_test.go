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

package pscompat

import (
	"context"
	"errors"
	"fmt"
	"runtime"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"cloud.google.com/go/internal/testutil"
	"cloud.google.com/go/internal/uid"
	"cloud.google.com/go/pubsub"
	"cloud.google.com/go/pubsublite"
	"cloud.google.com/go/pubsublite/internal/test"
	"cloud.google.com/go/pubsublite/internal/wire"
	"github.com/google/go-cmp/cmp/cmpopts"
	"golang.org/x/sync/errgroup"
	"google.golang.org/api/option"

	vkit "cloud.google.com/go/pubsublite/apiv1"
	pb "cloud.google.com/go/pubsublite/apiv1/pubsublitepb"
	tspb "google.golang.org/protobuf/types/known/timestamppb"
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

func publisherClient(ctx context.Context, t *testing.T, settings PublishSettings, topic wire.TopicPath, opts ...option.ClientOption) *PublisherClient {
	opts = testOptions(ctx, t, opts...)
	pub, err := NewPublisherClientWithSettings(ctx, topic.String(), settings, opts...)
	if err != nil {
		t.Fatalf("Failed to create publisher client: %v", err)
	}
	return pub
}

func subscriberClient(ctx context.Context, t *testing.T, settings ReceiveSettings, subscription wire.SubscriptionPath, opts ...option.ClientOption) *SubscriberClient {
	opts = testOptions(ctx, t, opts...)
	sub, err := NewSubscriberClientWithSettings(ctx, subscription.String(), settings, opts...)
	if err != nil {
		t.Fatalf("Failed to create publisher client: %v", err)
	}
	return sub
}

func initResourcePaths(t *testing.T) (string, wire.TopicPath, wire.SubscriptionPath) {
	initIntegrationTest(t)

	proj := testutil.ProjID()
	zone := test.RandomLiteZone()
	region, _ := wire.LocationToRegion(zone)
	resourceID := resourceIDs.New()

	topicPath := wire.TopicPath{Project: proj, Location: zone, TopicID: resourceID}
	subscriptionPath := wire.SubscriptionPath{Project: proj, Location: zone, SubscriptionID: resourceID}
	return region, topicPath, subscriptionPath
}

func createTopic(ctx context.Context, t *testing.T, admin *pubsublite.AdminClient, topic wire.TopicPath, partitionCount int) {
	topicConfig := pubsublite.TopicConfig{
		Name:                       topic.String(),
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

func cleanUpTopic(ctx context.Context, t *testing.T, admin *pubsublite.AdminClient, topic wire.TopicPath) {
	if err := admin.DeleteTopic(ctx, topic.String()); err != nil {
		t.Errorf("Failed to delete topic %s: %v", topic, err)
	} else {
		t.Logf("Deleted topic %s", topic)
	}
}

func createSubscription(ctx context.Context, t *testing.T, admin *pubsublite.AdminClient, subscription wire.SubscriptionPath, topic wire.TopicPath) {
	subConfig := &pubsublite.SubscriptionConfig{
		Name:                subscription.String(),
		Topic:               topic.String(),
		DeliveryRequirement: pubsublite.DeliverImmediately,
	}
	_, err := admin.CreateSubscription(ctx, *subConfig)
	if err != nil {
		t.Fatalf("Failed to create subscription %s: %v", subscription, err)
	} else {
		t.Logf("Created subscription %s", subscription)
	}
}

func cleanUpSubscription(ctx context.Context, t *testing.T, admin *pubsublite.AdminClient, subscription wire.SubscriptionPath) {
	if err := admin.DeleteSubscription(ctx, subscription.String()); err != nil {
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

func publishMessages(t *testing.T, settings PublishSettings, topic wire.TopicPath, msgs ...*pubsub.Message) {
	ctx := context.Background()
	publisher := publisherClient(context.Background(), t, settings, topic)
	defer publisher.Stop()

	var pubResults []*pubsub.PublishResult
	for _, msg := range msgs {
		pubResults = append(pubResults, publisher.Publish(ctx, msg))
	}
	waitForPublishResults(t, pubResults)
}

func publishPrefixedMessages(t *testing.T, settings PublishSettings, topic wire.TopicPath, msgPrefix string, msgCount, msgSize int) []string {
	ctx := context.Background()
	publisher := publisherClient(context.Background(), t, settings, topic)
	defer publisher.Stop()

	orderingSender := test.NewOrderingSender()
	var pubResults []*pubsub.PublishResult
	var msgData []string
	for i := 0; i < msgCount; i++ {
		data := orderingSender.Next(msgPrefix)
		msgData = append(msgData, data)
		msg := &pubsub.Message{Data: []byte(data)}
		if msgSize > 0 {
			// Add padding to achieve desired message size.
			msg.Attributes = map[string]string{"attr": strings.Repeat("*", msgSize-len(data))}
		}
		pubResults = append(pubResults, publisher.Publish(ctx, msg))
	}
	waitForPublishResults(t, pubResults)
	return msgData
}

func waitForPublishResults(t *testing.T, pubResults []*pubsub.PublishResult) {
	cctx, cancel := context.WithTimeout(context.Background(), defaultTestTimeout)
	for i, result := range pubResults {
		id, err := result.Get(cctx)
		if err != nil {
			t.Errorf("Publish(%d) got err: %v", i, err)
		}
		if _, err := ParseMessageMetadata(id); err != nil {
			t.Error(err)
		}
	}
	t.Logf("Published %d messages", len(pubResults))
	cancel()
}

func parseMessageMetadata(ctx context.Context, t *testing.T, result *pubsub.PublishResult) (*MessageMetadata, error) {
	id, err := result.Get(ctx)
	if err != nil {
		return nil, err
	}
	metadata, err := ParseMessageMetadata(id)
	if err != nil {
		t.Fatalf("Failed to parse message metadata: %v", err)
	}
	return metadata, err
}

func makeMsgTracker(msgs []string) *test.MsgTracker {
	msgTracker := test.NewMsgTracker()
	msgTracker.Add(msgs...)
	return msgTracker
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

type publishTimeRange struct {
	mu  sync.Mutex
	min time.Time
	max time.Time
}

func (p *publishTimeRange) Min() time.Time {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.min
}

func (p *publishTimeRange) Max() time.Time {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.max
}

func (p *publishTimeRange) AddTime(t time.Time) {
	p.mu.Lock()
	defer p.mu.Unlock()

	if p.min.IsZero() || t.Before(p.min) {
		p.min = t
	}
	if p.max.IsZero() || t.After(p.max) {
		p.max = t
	}
}

func receiveAllMessages(t *testing.T, msgTracker *test.MsgTracker, settings ReceiveSettings, subscription wire.SubscriptionPath) *publishTimeRange {
	cctx, stopSubscriber := context.WithTimeout(context.Background(), defaultTestTimeout)
	orderingValidator := test.NewOrderingReceiver()
	publishTimes := new(publishTimeRange)

	messageReceiver := func(ctx context.Context, msg *pubsub.Message) {
		msg.Ack()
		data := string(msg.Data)
		if !msgTracker.Remove(data) {
			// Prevent a flood of errors if a message for a previous test was found.
			t.Fatalf("Received unexpected message: %q", truncateMsg(data))
			return
		}

		// Check message ordering.
		metadata, err := ParseMessageMetadata(msg.ID)
		if err != nil {
			t.Error(err)
		} else {
			orderingKey := fmt.Sprintf("%d", metadata.Partition)
			if err := orderingValidator.Receive(data, orderingKey); err != nil {
				t.Errorf("Received unordered message with id %s: %q", msg.ID, truncateMsg(data))
			}
		}

		// Record publish times.
		publishTimes.AddTime(msg.PublishTime)

		// Stop the subscriber when all messages have been received.
		if msgTracker.Empty() {
			stopSubscriber()
		}
	}

	subscriber := subscriberClient(context.Background(), t, settings, subscription)
	if err := subscriber.Receive(cctx, messageReceiver); err != nil {
		t.Errorf("Receive() got err: %v", err)
	}
	if err := msgTracker.Status(); err != nil {
		t.Error(err)
	}
	return publishTimes
}

func receiveAndVerifyMessage(t *testing.T, want *pubsub.Message, settings ReceiveSettings, subscription wire.SubscriptionPath) {
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

	subscriber := subscriberClient(context.Background(), t, settings, subscription)
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
		eventTime, err := EncodeEventTimeAttribute(&tspb.Timestamp{
			Seconds: 1672531200,
			Nanos:   500000000,
		})
		if err != nil {
			t.Errorf("EncodeEventTimeAttribute() got err: %v", err)
		}
		msg := &pubsub.Message{
			Data:        []byte("round_trip"),
			OrderingKey: "ordering_key",
			Attributes: map[string]string{
				"attr1":               "value1",
				"attr2":               "value2",
				EventTimeAttributeKey: eventTime,
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
		cctx, cancel := context.WithTimeout(context.Background(), defaultTestTimeout)
		defer cancel()
		messageReceiver1 := func(ctx context.Context, got *pubsub.Message) {
			if diff := messageDiff(got, msg1); diff != "" {
				t.Errorf("Received message got: -, want: +\n%s", diff)
			}
			got.Nack()
		}
		subscriber := subscriberClient(context.Background(), t, recvSettings, subscriptionPath)
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
		subscriber = subscriberClient(context.Background(), t, customSettings, subscriptionPath)

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
		subscriber := subscriberClient(context.Background(), t, recvSettings, subscriptionPath)

		// The message receiver stops the subscriber after receiving the first
		// message. However, the subscriber isn't guaranteed to immediately stop, so
		// allow up to `len(msgs)` iterations.
		wantReceivedCount := len(msgs)
		for i := 0; i < wantReceivedCount; i++ {
			// New cctx must be created for each iteration as it is cancelled each
			// time stopSubscriber is called.
			cctx, stopSubscriber = context.WithTimeout(context.Background(), defaultTestTimeout)
			defer stopSubscriber()
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
		subscriber := subscriberClient(context.Background(), t, recvSettings, subscriptionPath)

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
		msgs := publishPrefixedMessages(t, pubSettings, topicPath, "ordering", messageCount, 0)

		// Receive messages.
		receiveAllMessages(t, makeMsgTracker(msgs), recvSettings, subscriptionPath)
	})

	// Checks that subscriber flow control works.
	t.Run("SubscriberFlowControl", func(t *testing.T) {
		const messageCount = 20
		const maxOutstandingMessages = 2 // Receive small batches of messages

		// Publish messages.
		msgs := publishPrefixedMessages(t, DefaultPublishSettings, topicPath, "subscriber_flow_control", messageCount, 0)

		// Receive messages.
		customSettings := recvSettings
		customSettings.MaxOutstandingMessages = maxOutstandingMessages
		receiveAllMessages(t, makeMsgTracker(msgs), customSettings, subscriptionPath)
	})

	// Verifies that large messages can be sent and received.
	t.Run("LargeMessages", func(t *testing.T) {
		const messageCount = 5
		const messageSize = MaxPublishRequestBytes - 50

		// Publish messages.
		msgs := publishPrefixedMessages(t, DefaultPublishSettings, topicPath, "large_messages", messageCount, messageSize)

		// Receive messages.
		receiveAllMessages(t, makeMsgTracker(msgs), recvSettings, subscriptionPath)
	})

	// Verifies that cancelling the context passed to NewPublisherClient can shut
	// down the publisher.
	t.Run("CancelPublisherContext", func(t *testing.T) {
		cctx, cancel := context.WithCancel(context.Background())
		publisher := publisherClient(cctx, t, DefaultPublishSettings, topicPath)
		defer publisher.Stop()

		cancel()

		wantErr := context.Canceled
		result := publisher.Publish(ctx, &pubsub.Message{Data: []byte("cancel_publisher_context")})
		if _, gotErr := result.Get(ctx); !test.ErrorEqual(gotErr, wantErr) {
			t.Errorf("Publish() got err: %v, want err: %v", gotErr, wantErr)
		}
	})

	// Verifies that publisher clients are not stopped while still in use.
	t.Run("Finalizer", func(t *testing.T) {
		publisher := publisherClient(context.Background(), t, DefaultPublishSettings, topicPath)
		runtime.GC() // Publisher should not be stopped

		result := publisher.Publish(ctx, &pubsub.Message{Data: []byte("finalizer1")})
		runtime.GC() // Publisher should not be stopped
		if _, err := result.Get(ctx); err != nil {
			t.Errorf("Publish() got err: %v", err)
		}

		result = publisher.Publish(ctx, &pubsub.Message{Data: []byte("finalizer2")})
		// The finalizer runs during the next GC. Publish should still succeed
		// because Stop flushes outstanding messages and waits for publish responses
		// before closing connections.
		runtime.GC()
		if _, err := result.Get(ctx); err != nil {
			t.Errorf("Publish() got err: %v", err)
		}

		// Explicitly clear the publisher reference, but the finalizer should have
		// already been triggered.
		publisher = nil
		runtime.GC()
	})

	// Verifies that cancelling the context passed to NewSubscriberClient can shut
	// down the subscriber.
	t.Run("CancelSubscriberContext", func(t *testing.T) {
		msg := &pubsub.Message{Data: []byte("cancel_subscriber_context")}
		publishMessages(t, DefaultPublishSettings, topicPath, msg)

		cctx, cancel := context.WithCancel(context.Background())
		subscriber := subscriberClient(cctx, t, recvSettings, subscriptionPath)

		gotErr := subscriber.Receive(context.Background(), func(ctx context.Context, got *pubsub.Message) {
			got.Ack()
			cancel()
		})

		if wantErr := context.Canceled; !test.ErrorEqual(gotErr, wantErr) {
			t.Errorf("Receive() got err: %v, want err: %v", gotErr, wantErr)
		}
	})

	// NOTE: This should be the last test case.
	// Verifies that increasing the number of topic partitions is handled
	// correctly by publishers.
	t.Run("IncreasePartitions", func(t *testing.T) {
		// Create the publisher client with the initial single partition.
		const pollPeriod = 10 * time.Second
		pubSettings := DefaultPublishSettings
		pubSettings.configPollPeriod = pollPeriod // Poll updates more frequently
		publisher := publisherClient(context.Background(), t, pubSettings, topicPath)
		defer publisher.Stop()

		// Update the number of partitions.
		update := pubsublite.TopicConfigToUpdate{
			Name:           topicPath.String(),
			PartitionCount: 2,
		}
		if _, err := admin.UpdateTopic(ctx, update); err != nil {
			t.Errorf("Failed to increase partitions: %v", err)
		}

		// Wait for the new config to propagate.
		time.Sleep(3 * pollPeriod)

		// Publish 2 messages, which should be routed to different partitions
		// (round robin).
		result1 := publisher.Publish(ctx, &pubsub.Message{Data: []byte("increase-partitions-1")})
		result2 := publisher.Publish(ctx, &pubsub.Message{Data: []byte("increase-partitions-2")})
		// Tolerate test flakiness, as the new config may not have propagated on the
		// server.
		metadata1, err := parseMessageMetadata(ctx, t, result1)
		if err != nil {
			t.Logf("Warning: failed to publish message: %v. Publisher error: %v", err, publisher.Error())
			return
		}
		metadata2, err := parseMessageMetadata(ctx, t, result2)
		if err != nil {
			t.Logf("Warning: failed to publish message: %v. Publisher error: %v", err, publisher.Error())
			return
		}
		if metadata1.Partition == metadata2.Partition {
			t.Errorf("Messages were published to the same partition = %d. Expected different partitions", metadata1.Partition)
		}
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
		msgs := publishPrefixedMessages(t, DefaultPublishSettings, topicPath, "routing_no_key", messageCount, 0)

		// Receive messages.
		receiveAllMessages(t, makeMsgTracker(msgs), recvSettings, subscriptionPath)
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
		receiveAllMessages(t, msgTracker, recvSettings, subscriptionPath)
	})

	// Verifies usage of the partition assignment service.
	t.Run("PartitionAssignment", func(t *testing.T) {
		const messageCount = 100
		const subscriberCount = 2 // Should be between [2, partitionCount]

		// Publish messages.
		msgs := publishPrefixedMessages(t, DefaultPublishSettings, topicPath, "partition_assignment", messageCount, 0)

		// Start multiple subscribers that use partition assignment.
		msgTracker := makeMsgTracker(msgs)

		messageReceiver := func(ctx context.Context, msg *pubsub.Message) {
			msg.Ack()
			data := string(msg.Data)
			if !msgTracker.Remove(data) {
				t.Errorf("Received unexpected message: %q", truncateMsg(data))
			}
		}

		// Verify partition reassignment notifications.
		var allPartitions []int
		var mu sync.Mutex
		reassignmentHandler := func(before, after []int) error {
			t.Logf("Partition assignments: before %v, after %v", before, after)
			if got, want := len(before), 0; got != want {
				t.Errorf("Partition assignments len(before): got %d, want %d", got, want)
			}
			mu.Lock()
			allPartitions = append(allPartitions, after...)
			mu.Unlock()
			return nil
		}

		receiveSettings := DefaultReceiveSettings
		receiveSettings.ReassignmentHandler = reassignmentHandler

		cctx, stopSubscribers := context.WithTimeout(context.Background(), defaultTestTimeout)
		g, _ := errgroup.WithContext(ctx)
		for i := 0; i < subscriberCount; i++ {
			// Subscribers must be started in a goroutine as Receive() blocks.
			g.Go(func() error {
				subscriber := subscriberClient(context.Background(), t, receiveSettings, subscriptionPath)
				err := subscriber.Receive(cctx, messageReceiver)
				if err != nil {
					t.Errorf("Receive() got err: %v", err)
				}
				return err
			})
		}

		// Wait until all messages have been received.
		msgTracker.Wait(defaultTestTimeout)
		stopSubscribers()
		// Wait until all subscribers have terminated.
		g.Wait()

		mu.Lock()
		sort.Ints(allPartitions)
		if got, want := allPartitions, partitionNumbers(partitionCount); !testutil.Equal(got, want) {
			t.Errorf("Assigned partition numbers: got %v, want %v", got, want)
		}
		mu.Unlock()
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

	var subscriptionPaths []wire.SubscriptionPath
	for i := 0; i < subscriberCount; i++ {
		subscription := baseSubscriptionPath
		subscription.SubscriptionID += fmt.Sprintf("%s-%d", baseSubscriptionPath.SubscriptionID, i)
		subscriptionPaths = append(subscriptionPaths, subscription)

		createSubscription(ctx, t, admin, subscription, topicPath)
		defer cleanUpSubscription(ctx, t, admin, subscription)
	}

	// Publish messages.
	msgs := publishPrefixedMessages(t, DefaultPublishSettings, topicPath, "fan_out", messageCount, 0)

	// Receive messages from multiple subscriptions.
	recvSettings := DefaultReceiveSettings
	recvSettings.Partitions = partitionNumbers(partitionCount)

	for _, subscription := range subscriptionPaths {
		receiveAllMessages(t, makeMsgTracker(msgs), recvSettings, subscription)
	}
}

func validateNewSeekOperation(t *testing.T, _ wire.SubscriptionPath, seekOp *pubsublite.SeekSubscriptionOperation) {
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
	t.Logf("Seek operation initiated: %s, metadata: %v", seekOp.Name(), m)
}

func validateCompleteSeekOperation(ctx context.Context, t *testing.T, _ wire.SubscriptionPath, seekOp *pubsublite.SeekSubscriptionOperation) {
	t.Helper()

	_, err := seekOp.Wait(ctx)
	if err != nil {
		t.Errorf("Operation.Wait() got err: %v", err)
		return
	}
	if got, want := seekOp.Done(), true; got != want {
		t.Errorf("Operation.Done() got: %v, want: %v", got, want)
	}

	m, err := seekOp.Metadata()
	if err != nil {
		t.Errorf("Operation.Metadata() got err: %v", err)
		return
	}
	if len(m.Target) == 0 {
		t.Error("Metadata.Target missing")
	}
	if len(m.Verb) == 0 {
		t.Error("Metadata.Verb missing")
	}
	if m.CreateTime.IsZero() {
		t.Error("Metadata.CreateTime missing")
	}
	if m.EndTime.IsZero() {
		t.Error("Metadata.EndTime missing")
	}
	t.Logf("Seek operation complete: %s, metadata: %v", seekOp.Name(), m)
}

func TestIntegration_SeekSubscription(t *testing.T) {
	const partitionCount = 4
	const messageCount = 50 * partitionCount
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

	// Note: Subtests need to be run sequentially.

	t.Run("SeekToBeginning", func(t *testing.T) {
		// Publish the first batch of messages.
		msgBatch1 := publishPrefixedMessages(t, DefaultPublishSettings, topicPath, "seek-batch1", messageCount, 0)
		msgTracker := makeMsgTracker(msgBatch1)

		// Keep the subscriber alive to test the subscriber client's handling of
		// out-of-band seek notifications from the server.
		cctx, stopSubscriber := context.WithTimeout(context.Background(), defaultTestTimeout)
		messageReceiver := func(ctx context.Context, msg *pubsub.Message) {
			msg.Ack()
			data := string(msg.Data)
			if !msgTracker.Remove(data) {
				stopSubscriber()
				t.Errorf("Received unexpected message: %q", data)
			}
		}
		subscriber := subscriberClient(context.Background(), t, recvSettings, subscriptionPath)
		receiveDone := test.NewCondition("subscriber-stopped")
		go func() {
			// Receive messages in a goroutine, as Receive is blocking.
			if err := subscriber.Receive(cctx, messageReceiver); err != nil {
				t.Errorf("Receive() got err: %v", err)
			}
			receiveDone.SetDone()
		}()

		// Receive batch 1 once.
		if err := msgTracker.Wait(defaultTestTimeout); err != nil {
			t.Fatal(err)
		}
		msgTracker.Add(msgBatch1...)

		// Seek to beginning.
		seekOp, err := admin.SeekSubscription(ctx, subscriptionPath.String(), pubsublite.Beginning)
		if err != nil {
			t.Errorf("SeekSubscription() got err: %v", err)
		} else {
			validateNewSeekOperation(t, subscriptionPath, seekOp)
		}

		// Verify that messages are received from the beginning of batch 1.
		if err := msgTracker.Wait(defaultTestTimeout); err != nil {
			t.Fatal(err)
		}
		stopSubscriber()
		receiveDone.WaitUntilDone(t, defaultTestTimeout)

		if seekOp != nil {
			validateCompleteSeekOperation(ctx, t, subscriptionPath, seekOp)
		}
	})

	t.Run("VerifyCommits", func(t *testing.T) {
		// Verifies that cursors are committed correctly post-seek in the previous
		// test. Only newly published messages should be received.
		msgs := publishPrefixedMessages(t, DefaultPublishSettings, topicPath, "verify-commits", 5*partitionCount, 0)
		receiveAllMessages(t, makeMsgTracker(msgs), recvSettings, subscriptionPath)
	})

	t.Run("SeekToEnd", func(t *testing.T) {
		// Publish batch 2, but do not receive messages in order to test seeking to
		// head/end.
		publishPrefixedMessages(t, DefaultPublishSettings, topicPath, "seek-batch2", messageCount, 0)

		// Seek to end.
		seekOp, err := admin.SeekSubscription(ctx, subscriptionPath.String(), pubsublite.End)
		if err != nil {
			t.Errorf("SeekSubscription() got err: %v", err)
		} else {
			validateNewSeekOperation(t, subscriptionPath, seekOp)
		}

		// Publish batch 3 and verify that messages are only received from batch 3
		// (batch 2 skipped).
		msgBatch := publishPrefixedMessages(t, DefaultPublishSettings, topicPath, "seek-batch3", messageCount, 0)
		receiveAllMessages(t, makeMsgTracker(msgBatch), recvSettings, subscriptionPath)

		if seekOp != nil {
			validateCompleteSeekOperation(ctx, t, subscriptionPath, seekOp)
		}
	})
}

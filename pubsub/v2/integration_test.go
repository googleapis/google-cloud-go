// Copyright 2014 Google LLC
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//      http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package pubsub

import (
	"bufio"
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"cloud.google.com/go/internal/testutil"
	"cloud.google.com/go/internal/uid"
	pb "cloud.google.com/go/pubsub/v2/apiv1/pubsubpb"
	testutil2 "cloud.google.com/go/pubsub/v2/internal/testutil"
	"google.golang.org/api/option"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/encoding/protowire"
	"google.golang.org/protobuf/proto"
)

var (
	topicIDs = uid.NewSpace("topic", nil)
	subIDs   = uid.NewSpace("sub", nil)
)

// messageData is used to hold the contents of a message so that it can be compared against the contents
// of another message without regard to irrelevant fields.
type messageData struct {
	ID         string
	Data       string
	Attributes map[string]string
}

func extractMessageData(m *Message) messageData {
	return messageData{
		ID:         m.ID,
		Data:       string(m.Data),
		Attributes: m.Attributes,
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

func integrationTestClient(ctx context.Context, t *testing.T, opts ...option.ClientOption) *Client {
	if testing.Short() {
		t.Skip("Integration tests skipped in short mode")
	}
	projID := testutil.ProjID()
	if projID == "" {
		t.Skip("Integration tests skipped. See CONTRIBUTING.md for details")
	}
	ts := testutil.TokenSource(ctx, ScopePubSub, ScopeCloudPlatform)
	if ts == nil {
		t.Skip("Integration tests skipped. See CONTRIBUTING.md for details")
	}
	opts = append(withGRPCHeadersAssertion(t, option.WithTokenSource(ts)), opts...)
	client, err := NewClient(ctx, projID, opts...)
	if err != nil {
		t.Fatalf("Creating client error: %v", err)
	}
	return client
}

func TestIntegration_PublishReceive(t *testing.T) {
	ctx := context.Background()
	client := integrationTestClient(ctx, t)

	for _, sync := range []bool{false, true} {
		for _, maxMsgs := range []int{0, 3, -1} { // MaxOutstandingMessages = default, 3, unlimited
			testPublishAndReceive(t, client, maxMsgs, sync, false, 10, 0)
		}

		// Tests for large messages (larger than the 4MB gRPC limit).
		testPublishAndReceive(t, client, 0, sync, false, 1, 5*1024*1024)
	}
}

func testPublishAndReceive(t *testing.T, client *Client, maxMsgs int, synchronous, exactlyOnceDelivery bool, numMsgs, extraBytes int) {
	t.Run(fmt.Sprintf("maxMsgs:%d,synchronous:%t,exactlyOnceDelivery:%t,numMsgs:%d", maxMsgs, synchronous, exactlyOnceDelivery, numMsgs), func(t *testing.T) {
		t.Parallel()
		testutil.Retry(t, 3, 10*time.Second, func(r *testutil.R) {
			ctx := context.Background()
			topic, err := createTopicWithRetry(ctx, t, client, &pb.Topic{Name: newTopicName()})
			if err != nil {
				r.Errorf("CreateTopic error: %v", err)
			}
			defer client.TopicAdminClient.DeleteTopic(ctx, &pb.DeleteTopicRequest{Topic: topic.Name})
			publisher := client.Publisher(topic.Name)
			defer publisher.Stop()

			pbs, err := createSubWithRetry(ctx, t, client, &pb.Subscription{
				Name:                      newSubName(),
				Topic:                     topic.Name,
				EnableExactlyOnceDelivery: exactlyOnceDelivery,
			})
			if err != nil {
				r.Errorf("CreateSub error: %v", err)
			}
			defer client.SubscriptionAdminClient.DeleteSubscription(ctx, &pb.DeleteSubscriptionRequest{Subscription: pbs.Name})
			sub := client.Subscriber(pbs.Name)

			var msgs []*Message
			for i := 0; i < numMsgs; i++ {
				text := fmt.Sprintf("a message with an index %d - %s", i, strings.Repeat(".", extraBytes))
				attrs := make(map[string]string)
				attrs["foo"] = "bar"
				msgs = append(msgs, &Message{
					Data:       []byte(text),
					Attributes: attrs,
				})
			}

			// Publish some messages.
			type pubResult struct {
				m *Message
				r *PublishResult
			}
			var rs []pubResult
			for _, m := range msgs {
				r := publisher.Publish(ctx, m)
				rs = append(rs, pubResult{m, r})
			}
			want := make(map[string]messageData)
			for _, res := range rs {
				id, err := res.r.Get(ctx)
				if err != nil {
					r.Errorf("r.Get: %v", err)
				}
				md := extractMessageData(res.m)
				md.ID = id
				want[md.ID] = md
			}

			sub.ReceiveSettings.MaxOutstandingMessages = maxMsgs
			sub.ReceiveSettings.Synchronous = synchronous

			// Use a timeout to ensure that Pull does not block indefinitely if there are
			// unexpectedly few messages available.
			now := time.Now()
			timeout := 3 * time.Minute
			timeoutCtx, cancel := context.WithTimeout(ctx, timeout)
			defer cancel()
			gotMsgs, err := pullN(timeoutCtx, sub, len(want), 0, func(ctx context.Context, m *Message) {
				m.Ack()
			})
			if err != nil {
				if c := status.Convert(err); c.Code() == codes.Canceled {
					if time.Since(now) >= timeout {
						r.Errorf("pullN took longer than %v", timeout)
					}
				} else {
					r.Errorf("Pull: %v", err)
				}
			}
			got := make(map[string]messageData)
			for _, m := range gotMsgs {
				md := extractMessageData(m)
				got[md.ID] = md
			}
			if !testutil.Equal(got, want) {
				r.Errorf("MaxOutstandingMessages=%d, Synchronous=%t, messages got: %+v, messages want: %+v",
					maxMsgs, synchronous, got, want)
			}
		})
	})
}

func TestIntegration_LargePublishSize(t *testing.T) {
	ctx := context.Background()
	client := integrationTestClient(ctx, t)
	defer client.Close()

	topic, err := createTopicWithRetry(ctx, t, client, &pb.Topic{Name: newTopicName()})
	if err != nil {
		t.Fatalf("CreateTopic error: %v", err)
	}
	defer client.TopicAdminClient.DeleteTopic(ctx, &pb.DeleteTopicRequest{Topic: topic.Name})
	publisher := client.Publisher(topic.Name)
	defer publisher.Stop()

	// Calculate the largest possible message length that is still valid.
	// First, calculate the max length of the encoded message accounting for the topic name.
	length := MaxPublishRequestBytes - calcFieldSizeString(topic.Name)
	// Next, account for the overhead from encoding an individual PubsubMessage,
	// and the inner PubsubMessage.Data field.
	pbMsgOverhead := 1 + protowire.SizeVarint(uint64(length))
	dataOverhead := 1 + protowire.SizeVarint(uint64(length-pbMsgOverhead))
	maxLengthSingleMessage := length - pbMsgOverhead - dataOverhead

	publishReq := &pb.PublishRequest{
		Topic: topic.Name,
		Messages: []*pb.PubsubMessage{
			{
				Data: bytes.Repeat([]byte{'A'}, maxLengthSingleMessage),
			},
		},
	}

	if got := proto.Size(publishReq); got != MaxPublishRequestBytes {
		t.Fatalf("Created request size of %d bytes,\nwant %f bytes", got, MaxPublishRequestBytes)
	}

	// Publishing the max length message by itself should succeed.
	msg := &Message{
		Data: bytes.Repeat([]byte{'A'}, maxLengthSingleMessage),
	}
	publisher.PublishSettings.FlowControlSettings.LimitExceededBehavior = FlowControlSignalError
	r := publisher.Publish(ctx, msg)
	if _, err := r.Get(ctx); err != nil {
		t.Fatalf("Failed to publish max length message: %v", err)
	}

	// Publish a small message first and make sure the max length message
	// is added to its own bundle.
	smallMsg := &Message{
		Data: []byte{'A'},
	}
	publisher.Publish(ctx, smallMsg)
	r = publisher.Publish(ctx, msg)
	if _, err := r.Get(ctx); err != nil {
		t.Fatalf("Failed to publish max length message after a small message: %v", err)
	}

	// Increase the data byte string by 1 byte, which should cause the request to fail,
	// specifically due to exceeding the bundle byte limit.
	msg.Data = append(msg.Data, 'A')
	r = publisher.Publish(ctx, msg)
	if _, err := r.Get(ctx); err != ErrOversizedMessage {
		t.Fatalf("Should throw item size too large error, got %v", err)
	}
}

func TestIntegration_CancelReceive(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	client := integrationTestClient(ctx, t)
	defer client.Close()

	topic, err := createTopicWithRetry(ctx, t, client, &pb.Topic{Name: newTopicName()})
	if err != nil {
		t.Errorf("failed to create topic: %v", err)
	}
	defer client.TopicAdminClient.DeleteTopic(ctx, &pb.DeleteTopicRequest{Topic: topic.Name})
	publisher := client.Publisher(topic.Name)
	defer publisher.Stop()

	s, err := createSubWithRetry(ctx, t, client, &pb.Subscription{
		Name:  newSubName(),
		Topic: topic.Name,
	})
	if err != nil {
		t.Fatalf("failed to create subscription: %v", err)
	}
	defer client.SubscriptionAdminClient.DeleteSubscription(ctx, &pb.DeleteSubscriptionRequest{Subscription: s.Name})

	ctx, cancel := context.WithCancel(context.Background())
	sub := client.Subscriber(s.Name)
	sub.ReceiveSettings.MaxOutstandingMessages = -1
	sub.ReceiveSettings.MaxOutstandingBytes = -1
	sub.ReceiveSettings.NumGoroutines = 1

	doneReceiving := make(chan struct{})

	// Publish the messages.
	go func() {
		for {
			select {
			case <-doneReceiving:
				return
			default:
				publisher.Publish(ctx, &Message{Data: []byte("some msg")})
				time.Sleep(time.Second)
			}
		}
	}()

	go func() {
		err = sub.Receive(ctx, func(_ context.Context, msg *Message) {
			cancel()
			time.AfterFunc(5*time.Second, msg.Ack)
		})
		close(doneReceiving)
	}()

	select {
	case <-time.After(60 * time.Second):
		t.Fatalf("Waited 60 seconds for Receive to finish, should have finished sooner")
	case <-doneReceiving:
	}
}

// publishSync is a utility function for publishing a message and
// blocking until the message has been confirmed.
func publishSync(ctx context.Context, t *testing.T, publisher *Publisher, msg *Message) {
	res := publisher.Publish(ctx, msg)
	_, err := res.Get(ctx)
	if err != nil {
		t.Fatalf("publishSync err: %v", err)
	}
}

func TestIntegration_PublicTopic(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	client := integrationTestClient(ctx, t)
	defer client.Close()

	subID := subIDs.New()

	sub, err := createSubWithRetry(ctx, t, client, &pb.Subscription{
		Name:  fmt.Sprintf("projects/%s/subscriptions/%s", testutil.ProjID(), subID),
		Topic: fmt.Sprintf("projects/%s/topics/%s", "pubsub-public-data", "taxirides-realtime"),
	})

	if err != nil {
		t.Fatal(err)
	}
	client.SubscriptionAdminClient.DeleteSubscription(ctx, &pb.DeleteSubscriptionRequest{
		Subscription: sub.GetName(),
	})
}

func TestIntegration_OrderedKeys_Basic(t *testing.T) {
	ctx := context.Background()
	client := integrationTestClient(ctx, t, option.WithEndpoint("us-west1-pubsub.googleapis.com:443"))
	defer client.Close()

	topic, err := createTopicWithRetry(ctx, t, client, &pb.Topic{Name: newTopicName()})
	if err != nil {
		t.Fatal(err)
	}
	defer client.TopicAdminClient.DeleteTopic(ctx, &pb.DeleteTopicRequest{Topic: topic.Name})
	publisher := client.Publisher(topic.Name)
	defer publisher.Stop()

	pbs, err := createSubWithRetry(ctx, t, client, &pb.Subscription{
		Name:                  newSubName(),
		Topic:                 topic.Name,
		EnableMessageOrdering: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	defer client.SubscriptionAdminClient.DeleteSubscription(ctx, &pb.DeleteSubscriptionRequest{Subscription: pbs.Name})
	sub := client.Subscriber(pbs.Name)

	publisher.PublishSettings.DelayThreshold = time.Second
	publisher.EnableMessageOrdering = true

	orderingKey := "some-ordering-key"
	numItems := 1000
	for i := 0; i < numItems; i++ {
		r := publisher.Publish(ctx, &Message{
			ID:          fmt.Sprintf("id-%d", i),
			Data:        []byte(fmt.Sprintf("item-%d", i)),
			OrderingKey: orderingKey,
		})
		go func() {
			if _, err := r.Get(ctx); err != nil {
				t.Error(err)
			}
		}()
	}

	received := make(chan string, numItems)
	ctx2, cancel := context.WithCancel(ctx)
	go func() {
		for i := 0; i < numItems; i++ {
			select {
			case r := <-received:
				if got, want := r, fmt.Sprintf("item-%d", i); got != want {
					t.Errorf("%d: got %s, want %s", i, got, want)
				}
			case <-time.After(30 * time.Second):
				t.Errorf("timed out after 30s waiting for item %d", i)
				cancel()
			}
		}
		cancel()
	}()

	if err := sub.Receive(ctx2, func(ctx context.Context, msg *Message) {
		defer msg.Ack()
		if msg.OrderingKey != orderingKey {
			t.Errorf("got ordering key %s, expected %s", msg.OrderingKey, orderingKey)
		}

		received <- string(msg.Data)
	}); err != nil && !errors.Is(err, context.Canceled) {
		t.Error(err)
	}
}

func TestIntegration_OrderedKeys_JSON(t *testing.T) {
	ctx := context.Background()
	client := integrationTestClient(ctx, t, option.WithEndpoint("us-west1-pubsub.googleapis.com:443"))
	defer client.Close()

	testutil.Retry(t, 2, 1*time.Second, func(r *testutil.R) {
		topic, err := createTopicWithRetry(ctx, t, client, &pb.Topic{Name: newTopicName()})
		if err != nil {
			r.Errorf("createTopicWithRetry err: %v", err)
		}
		defer client.TopicAdminClient.DeleteTopic(ctx, &pb.DeleteTopicRequest{Topic: topic.Name})
		publisher := client.Publisher(topic.Name)
		defer publisher.Stop()
		pbs, err := createSubWithRetry(ctx, t, client, &pb.Subscription{
			Name:                  newSubName(),
			Topic:                 topic.Name,
			EnableMessageOrdering: true,
		})
		if err != nil {
			t.Fatal(err)
		}
		defer client.SubscriptionAdminClient.DeleteSubscription(ctx, &pb.DeleteSubscriptionRequest{Subscription: pbs.Name})
		sub := client.Subscriber(pbs.Name)

		publisher.PublishSettings.DelayThreshold = time.Second
		publisher.EnableMessageOrdering = true

		inFile, err := os.Open("testdata/publish.csv")
		if err != nil {
			r.Errorf("os.Open err: %v", err)
		}
		defer inFile.Close()

		mu := sync.Mutex{}
		var publishData []testutil2.OrderedKeyMsg
		var receiveData []testutil2.OrderedKeyMsg
		// Keep track of duplicate messages to avoid negative waitgroup counter.
		receiveSet := make(map[string]struct{})

		wg := sync.WaitGroup{}
		scanner := bufio.NewScanner(inFile)
		for scanner.Scan() {
			line := scanner.Text()
			// TODO: use strings.ReplaceAll once we only support 1.11+.
			line = strings.Replace(line, "\"", "", -1)
			parts := strings.Split(line, ",")
			key := parts[0]
			msg := parts[1]
			publishData = append(publishData, testutil2.OrderedKeyMsg{Key: key, Data: msg})
			res := publisher.Publish(ctx, &Message{
				Data:        []byte(msg),
				OrderingKey: key,
			})
			go func() {
				_, err := res.Get(ctx)
				if err != nil {
					// Can't fail inside goroutine, so just log the error.
					r.Logf("publish error for message(%s): %v", msg, err)
				}
			}()
			wg.Add(1)
		}
		if err := scanner.Err(); err != nil {
			r.Errorf("scanner.Err(): %v", err)
		}

		go func() {
			sub.Receive(ctx, func(ctx context.Context, msg *Message) {
				mu.Lock()
				defer mu.Unlock()
				// Messages are deduped using the data field, since in this case all
				// messages are unique.
				if _, ok := receiveSet[string(msg.Data)]; ok {
					r.Logf("received duplicate message: %s", msg.Data)
					return
				}
				receiveSet[string(msg.Data)] = struct{}{}
				receiveData = append(receiveData, testutil2.OrderedKeyMsg{Key: msg.OrderingKey, Data: string(msg.Data)})
				wg.Done()
				msg.Ack()
			})
		}()

		done := make(chan struct{})
		go func() {
			wg.Wait()
			close(done)
		}()

		select {
		case <-done:
		case <-time.After(2 * time.Minute):
			r.Errorf("timed out after 2m waiting for all messages to be received")
		}

		mu.Lock()
		defer mu.Unlock()
		if err := testutil2.VerifyKeyOrdering(publishData, receiveData); err != nil {
			r.Errorf("VerifyKeyOrdering error: %v", err)
		}
	})
}

func TestIntegration_OrderedKeys_ResumePublish(t *testing.T) {
	ctx := context.Background()
	client := integrationTestClient(ctx, t, option.WithEndpoint("us-west1-pubsub.googleapis.com:443"))
	defer client.Close()

	topic, err := createTopicWithRetry(ctx, t, client, &pb.Topic{Name: newTopicName()})
	if err != nil {
		t.Fatal(err)
	}
	defer client.TopicAdminClient.DeleteTopic(ctx, &pb.DeleteTopicRequest{Topic: topic.Name})
	publisher := client.Publisher(topic.Name)
	defer publisher.Stop()

	publisher.PublishSettings.BufferedByteLimit = 100
	publisher.EnableMessageOrdering = true

	orderingKey := "some-ordering-key2"
	// Publish a message that is too large so we'll get an error that
	// pauses publishing for this ordering key.
	r := publisher.Publish(ctx, &Message{
		Data:        bytes.Repeat([]byte("A"), 1000),
		OrderingKey: orderingKey,
	})
	if _, err := r.Get(ctx); err == nil {
		t.Fatalf("expected bundle byte limit error, got nil")
	}
	// Publish a normal sized message now, which should fail
	// since publishing on this ordering key is paused.
	r = publisher.Publish(ctx, &Message{
		Data:        []byte("should fail"),
		OrderingKey: orderingKey,
	})
	if _, err := r.Get(ctx); err == nil || !errors.As(err, &ErrPublishingPaused{}) {
		t.Fatalf("expected ordering keys publish error, got %v", err)
	}

	// Lastly, call ResumePublish and make sure subsequent publishes succeed.
	publisher.ResumePublish(orderingKey)
	r = publisher.Publish(ctx, &Message{
		Data:        []byte("should succeed"),
		OrderingKey: orderingKey,
	})
	if _, err := r.Get(ctx); err != nil {
		t.Fatalf("got error while publishing message: %v", err)
	}
}

// TestIntegration_OrderedKeys_SubscriptionOrdering tests that messages
// with ordering keys are not processed as such if the subscription
// does not have message ordering enabled.
func TestIntegration_OrderedKeys_SubscriptionOrdering(t *testing.T) {
	ctx := context.Background()
	client := integrationTestClient(ctx, t, option.WithEndpoint("us-west1-pubsub.googleapis.com:443"))
	defer client.Close()

	topic, err := createTopicWithRetry(ctx, t, client, &pb.Topic{Name: newTopicName()})
	if err != nil {
		t.Fatal(err)
	}
	defer client.TopicAdminClient.DeleteTopic(ctx, &pb.DeleteTopicRequest{Topic: topic.Name})
	publisher := client.Publisher(topic.Name)
	defer publisher.Stop()
	publisher.EnableMessageOrdering = true

	// Explicitly disable message ordering on the subscription.
	enableMessageOrdering := false
	pbs := &pb.Subscription{
		Name:                  newSubName(),
		Topic:                 topic.Name,
		EnableMessageOrdering: enableMessageOrdering,
	}
	pbs, err = createSubWithRetry(ctx, t, client, pbs)
	if err != nil {
		t.Fatal(err)
	}
	defer client.SubscriptionAdminClient.DeleteSubscription(ctx, &pb.DeleteSubscriptionRequest{Subscription: pbs.Name})
	sub := client.Subscriber(pbs.Name)

	publishSync(ctx, t, publisher, &Message{
		Data:        []byte("message-1"),
		OrderingKey: "ordering-key-1",
	})

	publishSync(ctx, t, publisher, &Message{
		Data:        []byte("message-2"),
		OrderingKey: "ordering-key-1",
	})

	sub.ReceiveSettings.Synchronous = true
	ctx2, cancel := context.WithTimeout(ctx, 12*time.Second)
	defer cancel()

	var numAcked int32
	sub.Receive(ctx2, func(_ context.Context, msg *Message) {
		// Create artificial constraints on message processing time.
		if string(msg.Data) == "message-1" {
			time.Sleep(10 * time.Second)
		} else {
			time.Sleep(5 * time.Second)
		}
		msg.Ack()
		atomic.AddInt32(&numAcked, 1)
	})
	// If the messages were received on a subscription with the EnableMessageOrdering=true,
	// total processing would exceed the timeout and only one message would be processed.
	if numAcked < 2 {
		t.Fatalf("did not process all messages in time, numAcked: %d", numAcked)
	}
}

func TestIntegration_OrderingWithExactlyOnce(t *testing.T) {
	ctx := context.Background()
	client := integrationTestClient(ctx, t, option.WithEndpoint("us-west1-pubsub.googleapis.com:443"))
	defer client.Close()

	topic, err := createTopicWithRetry(ctx, t, client, &pb.Topic{Name: newTopicName()})
	if err != nil {
		t.Fatal(err)
	}
	defer client.TopicAdminClient.DeleteTopic(ctx, &pb.DeleteTopicRequest{Topic: topic.Name})
	publisher := client.Publisher(topic.Name)
	defer publisher.Stop()
	pbs, err := createSubWithRetry(ctx, t, client, &pb.Subscription{
		Name:                  newSubName(),
		Topic:                 topic.Name,
		EnableMessageOrdering: true,
	})
	if err != nil {
		t.Fatal(err)
	}

	defer client.SubscriptionAdminClient.DeleteSubscription(ctx, &pb.DeleteSubscriptionRequest{Subscription: pbs.Name})
	sub := client.Subscriber(pbs.Name)

	publisher.PublishSettings.DelayThreshold = time.Second
	publisher.EnableMessageOrdering = true

	orderingKey := "some-ordering-key"
	numItems := 10
	for i := 0; i < numItems; i++ {
		r := publisher.Publish(ctx, &Message{
			ID:          fmt.Sprintf("id-%d", i),
			Data:        []byte(fmt.Sprintf("item-%d", i)),
			OrderingKey: orderingKey,
		})
		go func() {
			if _, err := r.Get(ctx); err != nil {
				t.Error(err)
			}
		}()
	}

	received := make(chan string, numItems)
	ctx2, cancel := context.WithCancel(ctx)
	go func() {
		for i := 0; i < numItems; i++ {
			select {
			case r := <-received:
				if got, want := r, fmt.Sprintf("item-%d", i); got != want {
					t.Errorf("%d: got %s, want %s", i, got, want)
				}
			case <-time.After(30 * time.Second):
				t.Errorf("timed out after 30s waiting for item %d", i)
				cancel()
			}
		}
		cancel()
	}()

	if err := sub.Receive(ctx2, func(ctx context.Context, msg *Message) {
		defer msg.Ack()
		if msg.OrderingKey != orderingKey {
			t.Errorf("got ordering key %s, expected %s", msg.OrderingKey, orderingKey)
		}

		received <- string(msg.Data)
	}); err != nil {
		if c := status.Code(err); c != codes.Canceled {
			t.Error(err)
		}
	}

}

// Test that the DeliveryAttempt field is nil when dead lettering is not enabled.
func TestIntegration_DeadLetterPolicy_DeliveryAttempt(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	client := integrationTestClient(ctx, t)
	defer client.Close()

	topic, err := createTopicWithRetry(ctx, t, client, &pb.Topic{Name: newTopicName()})
	if err != nil {
		t.Fatalf("CreateTopic error: %v", err)
	}
	defer client.TopicAdminClient.DeleteTopic(ctx, &pb.DeleteTopicRequest{Topic: topic.Name})
	publisher := client.Publisher(topic.Name)
	defer publisher.Stop()

	pbs := &pb.Subscription{
		Name:  newSubName(),
		Topic: topic.Name,
	}
	if pbs, err = createSubWithRetry(ctx, t, client, pbs); err != nil {
		t.Fatalf("CreateSub error: %v", err)
	}
	defer client.SubscriptionAdminClient.DeleteSubscription(ctx, &pb.DeleteSubscriptionRequest{Subscription: pbs.Name})

	res := publisher.Publish(ctx, &Message{
		Data: []byte("failed message"),
	})
	if _, err := res.Get(ctx); err != nil {
		t.Fatalf("Publish message error: %v", err)
	}

	ctx2, cancel := context.WithCancel(ctx)
	sub := client.Subscriber(pbs.Name)
	err = sub.Receive(ctx2, func(_ context.Context, m *Message) {
		defer m.Ack()
		defer cancel()
		if m.DeliveryAttempt != nil {
			t.Fatalf("DeliveryAttempt should be nil when dead lettering is disabled")
		}
	})
	if err != nil {
		t.Fatalf("Streaming pull error: %v\n", err)
	}
}

// TestIntegration_BadEndpoint tests that specifying a bad
// endpoint will cause an error in RPCs.
func TestIntegration_BadEndpoint(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	opts := withGRPCHeadersAssertion(t,
		option.WithEndpoint("example.googleapis.com:443"),
	)
	client := integrationTestClient(ctx, t, opts...)
	defer client.Close()
	if _, err := client.TopicAdminClient.CreateTopic(ctx, &pb.Topic{Name: newTopicName()}); err == nil {
		t.Fatalf("CreateTopic should fail with fake endpoint, got nil err")
	}
}

func TestIntegration_ExactlyOnceDelivery_PublishReceive(t *testing.T) {
	ctx := context.Background()
	client := integrationTestClient(ctx, t)

	for _, maxMsgs := range []int{0, 3, -1} { // MaxOutstandingMessages = default, 3, unlimited
		testPublishAndReceive(t, client, maxMsgs, false, true, 10, 0)
	}
}

func TestIntegration_DetectProjectID(t *testing.T) {
	t.Skip("doesn't pass locally")
	if testing.Short() {
		t.Skip("Integration tests skipped in short mode")
	}
	ctx := context.Background()
	testCreds := testutil.Credentials(ctx)
	if testCreds == nil {
		t.Skip("test credentials not present, skipping")
	}

	goodClient, err := NewClient(ctx, DetectProjectID, option.WithCredentials(testCreds))
	if err != nil {
		t.Errorf("test pubsub.NewClient: %v", err)
	}
	if goodClient.Project() != testutil.ProjID() {
		t.Errorf("client.Project() got %q, want %q", goodClient.Project(), testutil.ProjID())
	}

	badTS := testutil.ErroringTokenSource{}
	if badClient, err := NewClient(ctx, DetectProjectID, option.WithTokenSource(badTS)); err == nil {
		t.Errorf("expected error from bad token source, NewClient succeeded with project: %s", badClient.projectID)
	}
}

func TestIntegration_PublishCompression(t *testing.T) {
	ctx := context.Background()
	client := integrationTestClient(ctx, t)
	defer client.Close()

	topic, err := createTopicWithRetry(ctx, t, client, &pb.Topic{Name: newTopicName()})
	if err != nil {
		t.Fatal(err)
	}
	defer client.TopicAdminClient.DeleteTopic(ctx, &pb.DeleteTopicRequest{Topic: topic.Name})
	publisher := client.Publisher(topic.Name)
	defer publisher.Stop()

	publisher.PublishSettings.EnableCompression = true
	publisher.PublishSettings.CompressionBytesThreshold = 50

	const messageSizeBytes = 1000

	msg := &Message{Data: bytes.Repeat([]byte{'A'}, int(messageSizeBytes))}
	res := publisher.Publish(ctx, msg)

	_, err = res.Get(ctx)
	if err != nil {
		t.Errorf("publish result got err: %v", err)
	}
}

// createTopicWithRetry creates a topic, wrapped with testutil.Retry and returns the created topic or an error.
func createTopicWithRetry(ctx context.Context, t *testing.T, c *Client, topic *pb.Topic) (*pb.Topic, error) {
	var pbt *pb.Topic
	var err error
	testutil.Retry(t, 5, 1*time.Second, func(r *testutil.R) {
		pbt, err = c.TopicAdminClient.CreateTopic(ctx, topic)
		if err != nil {
			r.Errorf("CreateTopic error: %v", err)
		}
	})
	return pbt, err
}

// createSubWithRetry creates a subscription, wrapped with testutil.Retry and returns the created subscription or an error.
func createSubWithRetry(ctx context.Context, t *testing.T, c *Client, sub *pb.Subscription) (*pb.Subscription, error) {
	var err error
	var s *pb.Subscription
	testutil.Retry(t, 5, 1*time.Second, func(r *testutil.R) {
		s, err = c.SubscriptionAdminClient.CreateSubscription(ctx, sub)
		if err != nil {
			r.Errorf("CreateSubcription error: %v", err)
		}
	})
	return s, err
}

func newTopicName() string {
	return fmt.Sprintf("projects/%s/topics/%s", testutil.ProjID(), topicIDs.New())
}

func newSubName() string {
	return fmt.Sprintf("projects/%s/subscriptions/%s", testutil.ProjID(), subIDs.New())
}

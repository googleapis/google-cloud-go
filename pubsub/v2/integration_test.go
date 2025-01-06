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
	"cloud.google.com/go/internal/version"
	testutil2 "cloud.google.com/go/pubsub/internal/testutil"
	pb "cloud.google.com/go/pubsub/v2/apiv1/pubsubpb"
	gax "github.com/googleapis/gax-go/v2"
	"google.golang.org/api/option"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/encoding/protowire"
	"google.golang.org/protobuf/proto"
)

var (
	topicIDs  = uid.NewSpace("topic", nil)
	subIDs    = uid.NewSpace("sub", nil)
	schemaIDs = uid.NewSpace("schema", nil)
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

	for _, maxMsgs := range []int{0, 3, -1} { // MaxOutstandingMessages = default, 3, unlimited
		testPublishAndReceive(t, client, maxMsgs, false, 10, 0)
	}

	// Tests for large messages (larger than the 4MB gRPC limit).
	testPublishAndReceive(t, client, 0, false, 1, 5*1024*1024)
}

// withGoogleClientInfo sets the name and version of the application in
// the `x-goog-api-client` header passed on each request and returns the
// updated context.
func withGoogleClientInfo(ctx context.Context) context.Context {
	ctxMD, _ := metadata.FromOutgoingContext(ctx)
	kv := []string{
		"gl-go",
		version.Go(),
		"gax",
		gax.Version,
		"grpc",
		grpc.Version,
	}

	allMDs := append([]metadata.MD{ctxMD}, metadata.Pairs("x-goog-api-client", gax.XGoogHeader(kv...)))
	return metadata.NewOutgoingContext(ctx, metadata.Join(allMDs...))
}

func testPublishAndReceive(t *testing.T, client *Client, maxMsgs int, exactlyOnceDelivery bool, numMsgs, extraBytes int) {
	t.Run(fmt.Sprintf("maxMsgs:%d,exactlyOnceDelivery:%t,numMsgs:%d", maxMsgs, exactlyOnceDelivery, numMsgs), func(t *testing.T) {
		t.Parallel()
		testutil.Retry(t, 3, 10*time.Second, func(r *testutil.R) {
			ctx := context.Background()
			topic, err := createTopicWithRetry(ctx, t, client, topicIDs.New(), nil)
			if err != nil {
				r.Errorf("CreateTopic error: %v", err)
			}
			defer topic.Delete(ctx)
			defer topic.Stop()
			exists, err := topic.Exists(ctx)
			if err != nil {
				r.Errorf("TopicExists error: %v", err)
			}
			if !exists {
				r.Errorf("topic %v should exist, but it doesn't", topic)
			}

			sub, err := createSubWithRetry(ctx, t, client, subIDs.New(), SubscriptionConfig{
				Topic:                     topic,
				EnableExactlyOnceDelivery: exactlyOnceDelivery,
			})
			if err != nil {
				r.Errorf("CreateSub error: %v", err)
			}
			defer sub.Delete(ctx)
			exists, err = sub.Exists(ctx)
			if err != nil {
				r.Errorf("SubExists error: %v", err)
			}
			if !exists {
				r.Errorf("subscription %s should exist, but it doesn't", sub.ID())
			}
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
				r := topic.Publish(ctx, m)
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
				r.Errorf("MaxOutstandingMessages=%d, messages got: %+v, messages want: %+v",
					maxMsgs, got, want)
			}
		})
	})
}

func TestIntegration_LargePublishSize(t *testing.T) {
	ctx := context.Background()
	client := integrationTestClient(ctx, t)
	defer client.Close()

	topic, err := createTopicWithRetry(ctx, t, client, topicIDs.New(), nil)
	if err != nil {
		t.Fatalf("CreateTopic error: %v", err)
	}
	defer topic.Delete(ctx)
	defer topic.Stop()

	// Calculate the largest possible message length that is still valid.
	// First, calculate the max length of the encoded message accounting for the topic name.
	length := MaxPublishRequestBytes - calcFieldSizeString(topic.String())
	// Next, account for the overhead from encoding an individual PubsubMessage,
	// and the inner PubsubMessage.Data field.
	pbMsgOverhead := 1 + protowire.SizeVarint(uint64(length))
	dataOverhead := 1 + protowire.SizeVarint(uint64(length-pbMsgOverhead))
	maxLengthSingleMessage := length - pbMsgOverhead - dataOverhead

	publishReq := &pb.PublishRequest{
		Topic: topic.String(),
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
	topic.PublishSettings.FlowControlSettings.LimitExceededBehavior = FlowControlSignalError
	r := topic.Publish(ctx, msg)
	if _, err := r.Get(ctx); err != nil {
		t.Fatalf("Failed to publish max length message: %v", err)
	}

	// Publish a small message first and make sure the max length message
	// is added to its own bundle.
	smallMsg := &Message{
		Data: []byte{'A'},
	}
	topic.Publish(ctx, smallMsg)
	r = topic.Publish(ctx, msg)
	if _, err := r.Get(ctx); err != nil {
		t.Fatalf("Failed to publish max length message after a small message: %v", err)
	}

	// Increase the data byte string by 1 byte, which should cause the request to fail,
	// specifically due to exceeding the bundle byte limit.
	msg.Data = append(msg.Data, 'A')
	r = topic.Publish(ctx, msg)
	if _, err := r.Get(ctx); err != ErrOversizedMessage {
		t.Fatalf("Should throw item size too large error, got %v", err)
	}
}

func TestIntegration_CancelReceive(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	client := integrationTestClient(ctx, t)
	defer client.Close()

	topic, err := createTopicWithRetry(ctx, t, client, topicIDs.New(), nil)
	if err != nil {
		t.Errorf("failed to create topic: %v", err)
	}
	defer topic.Delete(ctx)
	defer topic.Stop()

	var sub *Subscription
	if sub, err = createSubWithRetry(ctx, t, client, subIDs.New(), SubscriptionConfig{Topic: topic}); err != nil {
		t.Fatalf("failed to create subscription: %v", err)
	}
	defer sub.Delete(ctx)

	ctx, cancel := context.WithCancel(context.Background())
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
				topic.Publish(ctx, &Message{Data: []byte("some msg")})
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

func TestIntegration_CreateSubscription_NeverExpire(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	client := integrationTestClient(ctx, t)
	defer client.Close()

	topic, err := createTopicWithRetry(ctx, t, client, topicIDs.New(), nil)
	if err != nil {
		t.Fatalf("CreateTopic error: %v", err)
	}
	defer topic.Delete(ctx)
	defer topic.Stop()

	cfg := SubscriptionConfig{
		Topic:            topic,
		ExpirationPolicy: time.Duration(0),
	}
	var sub *Subscription
	if sub, err = createSubWithRetry(ctx, t, client, subIDs.New(), cfg); err != nil {
		t.Fatalf("CreateSub error: %v", err)
	}
	defer sub.Delete(ctx)

	got, err := sub.Config(ctx)
	if err != nil {
		t.Fatal(err)
	}
	want := time.Duration(0)
	if got.ExpirationPolicy != want {
		t.Fatalf("config.ExpirationPolicy mismatch, got: %v, want: %v\n", got.ExpirationPolicy, want)
	}
}

// publishSync is a utility function for publishing a message and
// blocking until the message has been confirmed.
func publishSync(ctx context.Context, t *testing.T, topic *Topic, msg *Message) {
	res := topic.Publish(ctx, msg)
	_, err := res.Get(ctx)
	if err != nil {
		t.Fatalf("publishSync err: %v", err)
	}
}

func TestIntegration_OrderedKeys_Basic(t *testing.T) {
	ctx := context.Background()
	client := integrationTestClient(ctx, t, option.WithEndpoint("us-west1-pubsub.googleapis.com:443"))
	defer client.Close()

	topic, err := createTopicWithRetry(ctx, t, client, topicIDs.New(), nil)
	if err != nil {
		t.Fatal(err)
	}
	defer topic.Delete(ctx)
	defer topic.Stop()
	exists, err := topic.Exists(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if !exists {
		t.Fatalf("topic %v should exist, but it doesn't", topic)
	}
	var sub *Subscription
	if sub, err = createSubWithRetry(ctx, t, client, subIDs.New(), SubscriptionConfig{
		Topic:                 topic,
		EnableMessageOrdering: true,
	}); err != nil {
		t.Fatal(err)
	}
	defer sub.Delete(ctx)
	exists, err = sub.Exists(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if !exists {
		t.Fatalf("subscription %s should exist, but it doesn't", sub.ID())
	}

	topic.PublishSettings.DelayThreshold = time.Second
	topic.EnableMessageOrdering = true

	orderingKey := "some-ordering-key"
	numItems := 1000
	for i := 0; i < numItems; i++ {
		r := topic.Publish(ctx, &Message{
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
		topic, err := createTopicWithRetry(ctx, t, client, topicIDs.New(), nil)
		if err != nil {
			r.Errorf("createTopicWithRetry err: %v", err)
		}
		defer topic.Delete(ctx)
		defer topic.Stop()
		exists, err := topic.Exists(ctx)
		if err != nil {
			r.Errorf("topic.Exists err: %v", err)
		}
		if !exists {
			r.Errorf("topic %v should exist, but it doesn't", topic)
		}
		var sub *Subscription
		if sub, err = createSubWithRetry(ctx, t, client, subIDs.New(), SubscriptionConfig{
			Topic:                 topic,
			EnableMessageOrdering: true,
		}); err != nil {
			r.Errorf("creteSubWithRetry err: %v", err)
		}
		defer sub.Delete(ctx)
		exists, err = sub.Exists(ctx)
		if err != nil {
			r.Errorf("sub.Exists err: %v", err)
		}
		if !exists {
			r.Errorf("subscription %s should exist, but it doesn't", sub.ID())
		}

		topic.PublishSettings.DelayThreshold = time.Second
		topic.EnableMessageOrdering = true

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
			res := topic.Publish(ctx, &Message{
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

	topic, err := createTopicWithRetry(ctx, t, client, topicIDs.New(), nil)
	if err != nil {
		t.Fatal(err)
	}
	defer topic.Delete(ctx)
	defer topic.Stop()
	exists, err := topic.Exists(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if !exists {
		t.Fatalf("topic %v should exist, but it doesn't", topic)
	}

	topic.EnableMessageOrdering = true

	orderingKey := "some-ordering-key2"
	// Publish a message that is too large so we'll get an error that
	// pauses publishing for this ordering key.
	r := topic.Publish(ctx, &Message{
		Data:        bytes.Repeat([]byte("A"), 1000),
		OrderingKey: orderingKey,
	})
	if _, err := r.Get(ctx); err == nil {
		t.Fatalf("expected bundle byte limit error, got nil")
	}
	// Publish a normal sized message now, which should fail
	// since publishing on this ordering key is paused.
	r = topic.Publish(ctx, &Message{
		Data:        []byte("should fail"),
		OrderingKey: orderingKey,
	})
	if _, err := r.Get(ctx); err == nil || !errors.As(err, &ErrPublishingPaused{}) {
		t.Fatalf("expected ordering keys publish error, got %v", err)
	}

	// Lastly, call ResumePublish and make sure subsequent publishes succeed.
	topic.ResumePublish(orderingKey)
	r = topic.Publish(ctx, &Message{
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

	topic, err := createTopicWithRetry(ctx, t, client, topicIDs.New(), nil)
	if err != nil {
		t.Fatal(err)
	}
	defer topic.Delete(ctx)
	defer topic.Stop()
	exists, err := topic.Exists(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if !exists {
		t.Fatalf("topic %v should exist, but it doesn't", topic)
	}
	topic.EnableMessageOrdering = true

	// Explicitly disable message ordering on the subscription.
	enableMessageOrdering := false
	subCfg := SubscriptionConfig{
		Topic:                 topic,
		EnableMessageOrdering: enableMessageOrdering,
	}
	sub, err := createSubWithRetry(ctx, t, client, subIDs.New(), subCfg)
	if err != nil {
		t.Fatal(err)
	}
	defer sub.Delete(ctx)

	publishSync(ctx, t, topic, &Message{
		Data:        []byte("message-1"),
		OrderingKey: "ordering-key-1",
	})

	publishSync(ctx, t, topic, &Message{
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

	topic, err := createTopicWithRetry(ctx, t, client, topicIDs.New(), nil)
	if err != nil {
		t.Fatal(err)
	}
	defer topic.Delete(ctx)
	defer topic.Stop()
	exists, err := topic.Exists(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if !exists {
		t.Fatalf("topic %v should exist, but it doesn't", topic)
	}
	var sub *Subscription
	if sub, err = createSubWithRetry(ctx, t, client, subIDs.New(), SubscriptionConfig{
		Topic:                     topic,
		EnableMessageOrdering:     true,
		EnableExactlyOnceDelivery: true,
	}); err != nil {
		t.Fatal(err)
	}
	defer sub.Delete(ctx)
	exists, err = sub.Exists(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if !exists {
		t.Fatalf("subscription %s should exist, but it doesn't", sub.ID())
	}

	topic.PublishSettings.DelayThreshold = time.Second
	topic.EnableMessageOrdering = true

	orderingKey := "some-ordering-key"
	numItems := 10
	for i := 0; i < numItems; i++ {
		r := topic.Publish(ctx, &Message{
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

func TestIntegration_ExactlyOnceDelivery_PublishReceive(t *testing.T) {
	ctx := context.Background()
	client := integrationTestClient(ctx, t)

	for _, maxMsgs := range []int{0, 3, -1} { // MaxOutstandingMessages = default, 3, unlimited
		testPublishAndReceive(t, client, maxMsgs, true, 10, 0)
	}
}

func TestIntegration_DetectProjectID(t *testing.T) {
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

	topic, err := createTopicWithRetry(ctx, t, client, topicIDs.New(), nil)
	if err != nil {
		t.Fatal(err)
	}
	defer topic.Delete(ctx)
	defer topic.Stop()

	topic.PublishSettings.EnableCompression = true
	topic.PublishSettings.CompressionBytesThreshold = 50

	const messageSizeBytes = 1000

	msg := &Message{Data: bytes.Repeat([]byte{'A'}, int(messageSizeBytes))}
	res := topic.Publish(ctx, msg)

	_, err = res.Get(ctx)
	if err != nil {
		t.Errorf("publish result got err: %v", err)
	}
}

// createTopicWithRetry creates a topic, wrapped with testutil.Retry and returns the created topic or an error.
func createTopicWithRetry(ctx context.Context, t *testing.T, c *Client, topicID string, cfg *TopicConfig) (*Topic, error) {
	var topic *Topic
	var err error
	testutil.Retry(t, 5, 1*time.Second, func(r *testutil.R) {
		if cfg != nil {
			topic, err = c.CreateTopicWithConfig(ctx, topicID, cfg)
			if err != nil {
				r.Errorf("CreateTopic error: %v", err)
			}
		} else {
			topic, err = c.CreateTopic(ctx, topicID)
			if err != nil {
				r.Errorf("CreateTopic error: %v", err)
			}
		}
	})
	return topic, err
}

// createSubWithRetry creates a subscription, wrapped with testutil.Retry and returns the created subscription or an error.
func createSubWithRetry(ctx context.Context, t *testing.T, c *Client, subID string, cfg SubscriptionConfig) (*Subscription, error) {
	var sub *Subscription
	var err error
	testutil.Retry(t, 5, 1*time.Second, func(r *testutil.R) {
		sub, err = c.CreateSubscription(ctx, subID, cfg)
		if err != nil {
			r.Errorf("CreateSub error: %v", err)
		}
	})
	return sub, err
}

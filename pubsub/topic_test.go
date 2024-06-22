// Copyright 2016 Google LLC
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
	"bytes"
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"

	"cloud.google.com/go/internal/testutil"
	"cloud.google.com/go/pubsub/apiv1/pubsubpb"
	pb "cloud.google.com/go/pubsub/apiv1/pubsubpb"
	"cloud.google.com/go/pubsub/pstest"
	"github.com/google/go-cmp/cmp/cmpopts"
	"google.golang.org/api/iterator"
	"google.golang.org/api/option"
	"google.golang.org/api/support/bundler"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func checkTopicListing(t *testing.T, c *Client, want []string) {
	topics, err := slurpTopics(c.Topics(context.Background()))
	if err != nil {
		t.Fatalf("error listing topics: %v", err)
	}
	var got []string
	for _, topic := range topics {
		got = append(got, topic.ID())
	}
	if !testutil.Equal(got, want) {
		t.Errorf("topic list: got: %v, want: %v", got, want)
	}
}

// All returns the remaining topics from this iterator.
func slurpTopics(it *TopicIterator) ([]*Topic, error) {
	var topics []*Topic
	for {
		switch topic, err := it.Next(); err {
		case nil:
			topics = append(topics, topic)
		case iterator.Done:
			return topics, nil
		default:
			return nil, err
		}
	}
}

func TestTopicID(t *testing.T) {
	const id = "id"
	c, srv := newFake(t)
	defer c.Close()
	defer srv.Close()

	s := c.Topic(id)
	if got, want := s.ID(), id; got != want {
		t.Errorf("Topic.ID() = %q; want %q", got, want)
	}
}

func TestCreateTopicWithConfig(t *testing.T) {
	c, srv := newFake(t)
	defer c.Close()
	defer srv.Close()

	id := "test-topic"
	want := TopicConfig{
		Labels: map[string]string{"label": "value"},
		MessageStoragePolicy: MessageStoragePolicy{
			AllowedPersistenceRegions: []string{"us-east1"},
		},
		KMSKeyName: "projects/P/locations/L/keyRings/R/cryptoKeys/K",
		SchemaSettings: &SchemaSettings{
			Schema:   "projects/P/schemas/S",
			Encoding: EncodingJSON,
		},
		RetentionDuration: 5 * time.Hour,
	}

	topic := mustCreateTopicWithConfig(t, c, id, &want)
	got, err := topic.Config(context.Background())
	if err != nil {
		t.Fatalf("error getting topic config: %v", err)
	}
	opt := cmpopts.IgnoreUnexported(TopicConfig{})
	if !testutil.Equal(got, want, opt) {
		t.Errorf("got %v, want %v", got, want)
	}
}

func TestTopic_IngestionKinesis(t *testing.T) {
	c, srv := newFake(t)
	defer c.Close()
	defer srv.Close()

	id := "test-topic-kinesis"
	want := TopicConfig{
		IngestionDataSourceSettings: &IngestionDataSourceSettings{
			Source: &IngestionDataSourceAWSKinesis{
				StreamARN:         "fake-stream-arn",
				ConsumerARN:       "fake-consumer-arn",
				AWSRoleARN:        "fake-aws-role-arn",
				GCPServiceAccount: "fake-gcp-sa",
			},
		},
	}

	topic := mustCreateTopicWithConfig(t, c, id, &want)
	got, err := topic.Config(context.Background())
	if err != nil {
		t.Fatalf("error getting topic config: %v", err)
	}
	want.State = TopicStateActive
	opt := cmpopts.IgnoreUnexported(TopicConfig{})
	if !testutil.Equal(got, want, opt) {
		t.Errorf("got %v, want %v", got, want)
	}

	// Update ingestion settings.
	ctx := context.Background()
	settings := &IngestionDataSourceSettings{
		Source: &IngestionDataSourceAWSKinesis{
			StreamARN:         "fake-stream-arn-2",
			ConsumerARN:       "fake-consumer-arn-2",
			AWSRoleARN:        "aws-role-arn-2",
			GCPServiceAccount: "gcp-service-account-2",
		},
	}
	config2, err := topic.Update(ctx, TopicConfigToUpdate{IngestionDataSourceSettings: settings})
	if err != nil {
		t.Fatal(err)
	}
	if !testutil.Equal(config2.IngestionDataSourceSettings, settings, opt) {
		t.Errorf("\ngot  %+v\nwant %+v", config2.IngestionDataSourceSettings, settings)
	}

	// Clear schema settings.
	settings = &IngestionDataSourceSettings{}
	config3, err := topic.Update(ctx, TopicConfigToUpdate{IngestionDataSourceSettings: settings})
	if err != nil {
		t.Fatal(err)
	}
	if config3.IngestionDataSourceSettings != nil {
		t.Errorf("got: %+v, want nil", config3.IngestionDataSourceSettings)
	}
}

func TestListTopics(t *testing.T) {
	ctx := context.Background()
	c, srv := newFake(t)
	defer c.Close()
	defer srv.Close()

	var ids []string
	numTopics := 4
	for i := 0; i < numTopics; i++ {
		id := fmt.Sprintf("t%d", i)
		ids = append(ids, id)
		mustCreateTopic(t, c, id)
	}
	checkTopicListing(t, c, ids)

	var tt []*TopicConfig
	it := c.Topics(ctx)
	for {
		topic, err := it.NextConfig()
		if err == iterator.Done {
			break
		}
		if err != nil {
			t.Errorf("TopicIterator.NextConfig() got err: %v", err)
		} else {
			tt = append(tt, topic)
		}
	}
	if got := len(tt); got != numTopics {
		t.Errorf("c.Topics(ctx) returned %d topics, expected %d", got, numTopics)
	}
	for i, top := range tt {
		want := fmt.Sprintf("t%d", i)
		if got := top.ID(); got != want {
			t.Errorf("topic.ID() mismatch: got %s, want: %s", got, want)
		}
		want = fmt.Sprintf("projects/P/topics/t%d", i)
		if got := top.String(); got != want {
			t.Errorf("topic.String() mismatch: got %s, want: %s", got, want)
		}
	}
}

func TestListCompletelyEmptyTopics(t *testing.T) {
	c, srv := newFake(t)
	defer c.Close()
	defer srv.Close()

	checkTopicListing(t, c, nil)
}

func TestStopPublishOrder(t *testing.T) {
	// Check that Stop doesn't panic if called before Publish.
	// Also that Publish after Stop returns the right error.
	ctx := context.Background()
	c := &Client{projectID: "projid"}
	topic := c.Topic("t")
	topic.Stop()
	r := topic.Publish(ctx, &Message{})
	_, err := r.Get(ctx)
	if !errors.Is(err, ErrTopicStopped) {
		t.Errorf("got %v, want ErrTopicStopped", err)
	}
}

func TestPublishTimeout(t *testing.T) {
	ctx := context.Background()
	serv, err := testutil.NewServer()
	if err != nil {
		t.Fatal(err)
	}
	pubsubpb.RegisterPublisherServer(serv.Gsrv, &alwaysFailPublish{})
	serv.Start()
	conn, err := grpc.Dial(serv.Addr, grpc.WithInsecure())
	if err != nil {
		t.Fatal(err)
	}
	opts := withGRPCHeadersAssertion(t, option.WithGRPCConn(conn))
	c, err := NewClient(ctx, "projectID", opts...)
	if err != nil {
		t.Fatal(err)
	}
	topic := c.Topic("t")
	topic.PublishSettings.Timeout = 3 * time.Second
	r := topic.Publish(ctx, &Message{})
	defer topic.Stop()
	select {
	case <-r.Ready():
		_, err = r.Get(ctx)
		if !errors.Is(err, context.DeadlineExceeded) {
			t.Fatalf("got %v, want context.DeadlineExceeded", err)
		}
	case <-time.After(2 * topic.PublishSettings.Timeout):
		t.Fatal("timed out")
	}
}

func TestPublishBufferedByteLimit(t *testing.T) {
	ctx := context.Background()
	client, srv := newFake(t)
	defer client.Close()
	defer srv.Close()

	topic := mustCreateTopic(t, client, "topic-small-buffered-byte-limit")
	defer topic.Stop()

	// Test setting BufferedByteLimit to small number of bytes that should fail.
	topic.PublishSettings.BufferedByteLimit = 100

	const messageSizeBytes = 1000

	msg := &Message{Data: bytes.Repeat([]byte{'A'}, int(messageSizeBytes))}
	res := topic.Publish(ctx, msg)

	_, err := res.Get(ctx)
	if err != bundler.ErrOverflow {
		t.Errorf("got %v, want ErrOverflow", err)
	}
}

func TestUpdateTopic_Label(t *testing.T) {
	ctx := context.Background()
	client, srv := newFake(t)
	defer client.Close()
	defer srv.Close()

	topic := mustCreateTopic(t, client, "T")
	config, err := topic.Config(ctx)
	if err != nil {
		t.Fatal(err)
	}
	want := TopicConfig{}
	opt := cmpopts.IgnoreUnexported(TopicConfig{})
	if !testutil.Equal(config, want, opt) {
		t.Errorf("got %+v, want %+v", config, want)
	}

	// replace labels
	labels := map[string]string{"label": "value"}
	config2, err := topic.Update(ctx, TopicConfigToUpdate{
		Labels: labels,
	})
	if err != nil {
		t.Fatal(err)
	}
	want = TopicConfig{
		Labels: labels,
	}
	if !testutil.Equal(config2, want, opt) {
		t.Errorf("got %+v, want %+v", config2, want)
	}

	// delete all labels
	labels = map[string]string{}
	config3, err := topic.Update(ctx, TopicConfigToUpdate{Labels: labels})
	if err != nil {
		t.Fatal(err)
	}
	want.Labels = nil
	if !testutil.Equal(config3, want, opt) {
		t.Errorf("got %+v, want %+v", config3, want)
	}
}

func TestUpdateTopic_MessageStoragePolicy(t *testing.T) {
	ctx := context.Background()
	client, srv := newFake(t)
	defer client.Close()
	defer srv.Close()

	topic := mustCreateTopic(t, client, "T")
	config, err := topic.Config(ctx)
	if err != nil {
		t.Fatal(err)
	}
	want := TopicConfig{}
	opt := cmpopts.IgnoreUnexported(TopicConfig{})
	if !testutil.Equal(config, want, opt) {
		t.Errorf("\ngot  %+v\nwant %+v", config, want)
	}

	// Update message storage policy.
	msp := &MessageStoragePolicy{
		AllowedPersistenceRegions: []string{"us-east1"},
	}
	config2, err := topic.Update(ctx, TopicConfigToUpdate{MessageStoragePolicy: msp})
	if err != nil {
		t.Fatal(err)
	}
	want.MessageStoragePolicy = MessageStoragePolicy{
		AllowedPersistenceRegions: []string{"us-east1"},
	}
	if !testutil.Equal(config2, want, opt) {
		t.Errorf("\ngot  %+v\nwant %+v", config2, want)
	}
}

func TestUpdateTopic_SchemaSettings(t *testing.T) {
	ctx := context.Background()
	client, srv := newFake(t)
	defer client.Close()
	defer srv.Close()

	topic := mustCreateTopic(t, client, "T")
	config, err := topic.Config(ctx)
	if err != nil {
		t.Fatal(err)
	}
	want := TopicConfig{}
	opt := cmpopts.IgnoreUnexported(TopicConfig{})
	if !testutil.Equal(config, want, opt) {
		t.Errorf("\ngot  %+v\nwant %+v", config, want)
	}

	// Update schema settings.
	settings := &SchemaSettings{
		Schema:          "some-schema",
		Encoding:        EncodingJSON,
		FirstRevisionID: "1234",
	}
	config2, err := topic.Update(ctx, TopicConfigToUpdate{SchemaSettings: settings})
	if err != nil {
		t.Fatal(err)
	}
	if !testutil.Equal(config2.SchemaSettings, settings, opt) {
		t.Errorf("\ngot  %+v\nwant %+v", config2.SchemaSettings, settings)
	}

	// Clear schema settings.
	settings = &SchemaSettings{}
	config3, err := topic.Update(ctx, TopicConfigToUpdate{SchemaSettings: settings})
	if err != nil {
		t.Fatal(err)
	}
	if config3.SchemaSettings != nil {
		t.Errorf("got: %+v, want nil", config3.SchemaSettings)
	}
}

type alwaysFailPublish struct {
	pubsubpb.PublisherServer
}

func (s *alwaysFailPublish) Publish(ctx context.Context, req *pubsubpb.PublishRequest) (*pubsubpb.PublishResponse, error) {
	return nil, status.Errorf(codes.Unavailable, "try again")
}

func mustCreateTopic(t *testing.T, c *Client, id string) *Topic {
	topic, err := c.CreateTopic(context.Background(), id)
	if err != nil {
		t.Fatal(err)
	}
	return topic
}

func mustCreateTopicWithConfig(t *testing.T, c *Client, id string, tc *TopicConfig) *Topic {
	if tc == nil {
		return mustCreateTopic(t, c, id)
	}
	topic, err := c.CreateTopicWithConfig(context.Background(), id, tc)
	if err != nil {
		t.Fatal(err)
	}
	return topic
}

func TestDetachSubscription(t *testing.T) {
	ctx := context.Background()
	c, srv := newFake(t)
	defer c.Close()
	defer srv.Close()

	topic, err := c.CreateTopic(ctx, "some-topic")
	if err != nil {
		t.Fatal(err)
	}
	c.CreateSubscription(ctx, "some-sub", SubscriptionConfig{
		Topic: topic,
	})
	if _, err := c.DetachSubscription(ctx, "projects/P/subscriptions/some-sub"); err != nil {
		t.Errorf("DetachSubscription failed: %v", err)
	}
}

func TestFlushStopTopic(t *testing.T) {
	ctx := context.Background()
	c, srv := newFake(t)
	defer c.Close()
	defer srv.Close()

	topic, err := c.CreateTopic(ctx, "flush-topic")
	if err != nil {
		t.Fatal(err)
	}

	// Subsequent publishes after a flush should succeed.
	topic.Flush()
	r1 := topic.Publish(ctx, &Message{
		Data: []byte("hello"),
	})
	_, err = r1.Get(ctx)
	if err != nil {
		t.Errorf("got err: %v", err)
	}

	// Publishing after a flush should succeed.
	topic.Flush()
	r2 := topic.Publish(ctx, &Message{
		Data: []byte("world"),
	})
	_, err = r2.Get(ctx)
	if err != nil {
		t.Errorf("got err: %v", err)
	}

	// Publishing after a temporarily blocked flush should succeed.
	srv.SetAutoPublishResponse(false)

	r3 := topic.Publish(ctx, &Message{
		Data: []byte("blocking message publish"),
	})
	go func() {
		topic.Flush()
	}()

	// Wait a second between publishes to ensure messages are not bundled together.
	time.Sleep(1 * time.Second)
	r4 := topic.Publish(ctx, &Message{
		Data: []byte("message published after flush"),
	})

	// Wait 5 seconds to simulate network delay.
	time.Sleep(5 * time.Second)
	srv.AddPublishResponse(&pubsubpb.PublishResponse{
		MessageIds: []string{"1"},
	}, nil)
	srv.AddPublishResponse(&pubsubpb.PublishResponse{
		MessageIds: []string{"2"},
	}, nil)

	if _, err = r3.Get(ctx); err != nil {
		t.Errorf("got err: %v", err)
	}
	if _, err = r4.Get(ctx); err != nil {
		t.Errorf("got err: %v", err)
	}

	// Publishing after Stop should fail.
	srv.SetAutoPublishResponse(true)
	topic.Stop()
	r5 := topic.Publish(ctx, &Message{
		Data: []byte("this should fail"),
	})
	if _, err := r5.Get(ctx); !errors.Is(err, ErrTopicStopped) {
		t.Errorf("got %v, want ErrTopicStopped", err)
	}
}

func TestPublishFlowControl_SignalError(t *testing.T) {
	ctx := context.Background()
	c, srv := newFake(t)
	defer c.Close()
	defer srv.Close()

	topic, err := c.CreateTopic(ctx, "some-topic")
	if err != nil {
		t.Fatal(err)
	}
	fc := FlowControlSettings{
		MaxOutstandingMessages: 1,
		MaxOutstandingBytes:    10,
		LimitExceededBehavior:  FlowControlSignalError,
	}
	topic.PublishSettings.FlowControlSettings = fc

	srv.SetAutoPublishResponse(false)

	// Sending a message that is too large results in an error in SignalError mode.
	r1 := publishSingleMessage(ctx, topic, "AAAAAAAAAAA")
	if _, err := r1.Get(ctx); err != ErrFlowControllerMaxOutstandingBytes {
		t.Fatalf("publishResult.Get(): got %v, want %v", err, ErrFlowControllerMaxOutstandingBytes)
	}

	// Sending a second message succeeds.
	r2 := publishSingleMessage(ctx, topic, "AAAA")

	// Sending a third message fails because of the outstanding message.
	r3 := publishSingleMessage(ctx, topic, "AA")
	if _, err := r3.Get(ctx); err != ErrFlowControllerMaxOutstandingMessages {
		t.Fatalf("publishResult.Get(): got %v, want %v", err, ErrFlowControllerMaxOutstandingMessages)
	}

	srv.AddPublishResponse(&pb.PublishResponse{
		MessageIds: []string{"1"},
	}, nil)
	got, err := r2.Get(ctx)
	if err != nil {
		t.Fatalf("publishResult.Get(): got %v", err)
	}
	if want := "1"; got != want {
		t.Fatalf("publishResult.Get() got: %s, want %s", got, want)
	}

	// Sending another messages succeeds.
	r4 := publishSingleMessage(ctx, topic, "AAAA")
	srv.AddPublishResponse(&pb.PublishResponse{
		MessageIds: []string{"2"},
	}, nil)
	got, err = r4.Get(ctx)
	if err != nil {
		t.Fatalf("publishResult.Get(): got %v", err)
	}
	if want := "2"; got != want {
		t.Fatalf("publishResult.Get() got: %s, want %s", got, want)
	}

}

func TestPublishFlowControl_SignalErrorOrderingKey(t *testing.T) {
	ctx := context.Background()
	c, srv := newFake(t)
	defer c.Close()
	defer srv.Close()

	topic, err := c.CreateTopic(ctx, "some-topic")
	if err != nil {
		t.Fatal(err)
	}
	fc := FlowControlSettings{
		MaxOutstandingMessages: 1,
		MaxOutstandingBytes:    10,
		LimitExceededBehavior:  FlowControlSignalError,
	}
	topic.PublishSettings.FlowControlSettings = fc
	topic.PublishSettings.DelayThreshold = 5 * time.Second
	topic.PublishSettings.CountThreshold = 1
	topic.EnableMessageOrdering = true

	// Sending a message that is too large reuslts in an error.
	r1 := publishSingleMessageWithKey(ctx, topic, "AAAAAAAAAAA", "a")
	if _, err := r1.Get(ctx); err != ErrFlowControllerMaxOutstandingBytes {
		t.Fatalf("r1.Get() got: %v, want %v", err, ErrFlowControllerMaxOutstandingBytes)
	}

	// Sending a second message for the same ordering key fails because the first one failed.
	r2 := publishSingleMessageWithKey(ctx, topic, "AAAA", "a")
	if _, err := r2.Get(ctx); err == nil {
		t.Fatal("r2.Get() got nil instead of error before calling topic.ResumePublish(key)")
	}
}

func TestPublishFlowControl_Block(t *testing.T) {
	ctx := context.Background()
	c, srv := newFake(t)
	defer c.Close()
	defer srv.Close()

	topic, err := c.CreateTopic(ctx, "some-topic")
	if err != nil {
		t.Fatal(err)
	}
	fc := FlowControlSettings{
		MaxOutstandingMessages: 2,
		MaxOutstandingBytes:    10,
		LimitExceededBehavior:  FlowControlBlock,
	}
	topic.PublishSettings.FlowControlSettings = fc
	topic.PublishSettings.DelayThreshold = 5 * time.Second
	topic.PublishSettings.CountThreshold = 1

	srv.SetAutoPublishResponse(false)

	var sendResponse1, response1Sent, sendResponse2 sync.WaitGroup
	sendResponse1.Add(1)
	response1Sent.Add(1)
	sendResponse2.Add(1)

	go func() {
		sendResponse1.Wait()
		addSingleResponse(srv, "1")
		response1Sent.Done()
		sendResponse2.Wait()
		addSingleResponse(srv, "2")
	}()

	// Sending two messages succeeds.
	publishSingleMessage(ctx, topic, "AA")
	publishSingleMessage(ctx, topic, "AA")

	// Sending a third message blocks because the messages are outstanding.
	var publish3Completed sync.WaitGroup
	publish3Completed.Add(1)
	go func() {
		publishSingleMessage(ctx, topic, "AAAAAA")
		publish3Completed.Done()
	}()

	go func() {
		sendResponse1.Done()
		response1Sent.Wait()
		sendResponse2.Done()
	}()

	// Sending a fourth message blocks because although only one message has been sent,
	// the third message claimed the tokens for outstanding bytes.
	var publish4Completed sync.WaitGroup
	publish4Completed.Add(1)

	go func() {
		publish3Completed.Wait()
		publishSingleMessage(ctx, topic, "A")
		publish4Completed.Done()
	}()

	publish3Completed.Wait()
	addSingleResponse(srv, "3")
	addSingleResponse(srv, "4")

	publish4Completed.Wait()
}

func TestInvalidUTF8(t *testing.T) {
	ctx := context.Background()
	c, srv := newFake(t)
	defer c.Close()
	defer srv.Close()

	topic, err := c.CreateTopic(ctx, "invalid-utf8-topic")
	if err != nil {
		t.Fatal(err)
	}
	res := topic.Publish(ctx, &Message{
		Data: []byte("foo"),
		Attributes: map[string]string{
			"attr": "a\xc5z",
		},
	})
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	_, err = res.Get(ctx)
	if err == nil || !strings.Contains(err.Error(), "string field contains invalid UTF-8") {
		t.Fatalf("expected invalid UTF-8 error, got: %v", err)
	}
}

// publishSingleMessage publishes a single message to a topic.
func publishSingleMessage(ctx context.Context, t *Topic, data string) *PublishResult {
	return t.Publish(ctx, &Message{
		Data: []byte(data),
	})
}

// publishSingleMessageWithKey publishes a single message to a topic with an ordering key.
func publishSingleMessageWithKey(ctx context.Context, t *Topic, data, key string) *PublishResult {
	return t.Publish(ctx, &Message{
		Data:        []byte(data),
		OrderingKey: key,
	})
}

// addSingleResponse adds a publish response to the provided fake.
func addSingleResponse(srv *pstest.Server, id string) {
	srv.AddPublishResponse(&pb.PublishResponse{
		MessageIds: []string{id},
	}, nil)
}

func TestPublishOrderingNotEnabled(t *testing.T) {
	ctx := context.Background()
	c, srv := newFake(t)
	defer c.Close()
	defer srv.Close()

	topic, err := c.CreateTopic(ctx, "test-topic")
	if err != nil {
		t.Fatal(err)
	}
	res := publishSingleMessageWithKey(ctx, topic, "test", "non-existent-key")
	if _, err := res.Get(ctx); !errors.Is(err, errTopicOrderingNotEnabled) {
		t.Errorf("got %v, want errTopicOrderingNotEnabled", err)
	}
}

func TestPublishCompression(t *testing.T) {
	ctx := context.Background()
	client, srv := newFake(t)
	defer client.Close()
	defer srv.Close()

	topic := mustCreateTopic(t, client, "topic-compression")
	defer topic.Stop()

	topic.PublishSettings.EnableCompression = true
	topic.PublishSettings.CompressionBytesThreshold = 50

	const messageSizeBytes = 1000

	msg := &Message{Data: bytes.Repeat([]byte{'A'}, int(messageSizeBytes))}
	res := topic.Publish(ctx, msg)

	_, err := res.Get(ctx)
	if err != nil {
		t.Errorf("publish result got err: %v", err)
	}
}

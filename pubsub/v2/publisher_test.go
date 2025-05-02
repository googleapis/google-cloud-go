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
	"cloud.google.com/go/pubsub/v2/apiv1/pubsubpb"
	pb "cloud.google.com/go/pubsub/v2/apiv1/pubsubpb"
	"cloud.google.com/go/pubsub/v2/pstest"
	"google.golang.org/api/option"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func TestPublisherID(t *testing.T) {
	const id = "id"
	c, srv := newFake(t)
	defer c.Close()
	defer srv.Close()

	p := c.Publisher(id)
	if got, want := p.ID(), id; got != want {
		t.Errorf("Topic.ID() = %q; want %q", got, want)
	}
}

func TestStopPublishOrder(t *testing.T) {
	// Check that Stop doesn't panic if called before Publish.
	// Also that Publish after Stop returns the right error.
	ctx := context.Background()
	c := &Client{projectID: "projid"}
	topic := c.Publisher("t")
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
	publisher := c.Publisher("t")
	publisher.PublishSettings.Timeout = 3 * time.Second
	r := publisher.Publish(ctx, &Message{})
	defer publisher.Stop()
	select {
	case <-r.Ready():
		_, err = r.Get(ctx)
		if !errors.Is(err, context.DeadlineExceeded) {
			t.Fatalf("got %v, want context.DeadlineExceeded", err)
		}
	case <-time.After(2 * publisher.PublishSettings.Timeout):
		t.Fatal("timed out")
	}
}

type alwaysFailPublish struct {
	pubsubpb.PublisherServer
}

func (s *alwaysFailPublish) Publish(ctx context.Context, req *pubsubpb.PublishRequest) (*pubsubpb.PublishResponse, error) {
	return nil, status.Errorf(codes.Unavailable, "try again")
}

func mustCreateTopic(t *testing.T, c *Client, name string) *Publisher {
	_, err := c.TopicAdminClient.CreateTopic(context.Background(), &pb.Topic{Name: name})
	if err != nil {
		t.Fatal(err)
	}
	return c.Publisher(name)
}

func TestFlushStopTopic(t *testing.T) {
	ctx := context.Background()
	c, srv := newFake(t)
	defer c.Close()
	defer srv.Close()
	topicName := fmt.Sprintf("projects/%s/topics/flush-topic", projName)
	publisher := mustCreateTopic(t, c, topicName)

	// Subsequent publishes after a flush should succeed.
	publisher.Flush()
	r1 := publisher.Publish(ctx, &Message{
		Data: []byte("hello"),
	})
	_, err := r1.Get(ctx)
	if err != nil {
		t.Errorf("got err: %v", err)
	}

	// Publishing after a flush should succeed.
	publisher.Flush()
	r2 := publisher.Publish(ctx, &Message{
		Data: []byte("world"),
	})
	_, err = r2.Get(ctx)
	if err != nil {
		t.Errorf("got err: %v", err)
	}

	// Publishing after a temporarily blocked flush should succeed.
	srv.SetAutoPublishResponse(false)

	r3 := publisher.Publish(ctx, &Message{
		Data: []byte("blocking message publish"),
	})
	go func() {
		publisher.Flush()
	}()

	// Wait a second between publishes to ensure messages are not bundled together.
	time.Sleep(1 * time.Second)
	r4 := publisher.Publish(ctx, &Message{
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
	publisher.Stop()
	r5 := publisher.Publish(ctx, &Message{
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

	topicName := "projects/P/topics/fc-error-topic"
	publisher := mustCreateTopic(t, c, topicName)
	fc := FlowControlSettings{
		MaxOutstandingMessages: 1,
		MaxOutstandingBytes:    10,
		LimitExceededBehavior:  FlowControlSignalError,
	}
	publisher.PublishSettings.FlowControlSettings = fc

	srv.SetAutoPublishResponse(false)

	// Sending a message that is too large results in an error in SignalError mode.
	r1 := publishSingleMessage(ctx, publisher, "AAAAAAAAAAA")
	if _, err := r1.Get(ctx); err != ErrFlowControllerMaxOutstandingBytes {
		t.Fatalf("publishResult.Get(): got %v, want %v", err, ErrFlowControllerMaxOutstandingBytes)
	}

	// Sending a second message succeeds.
	r2 := publishSingleMessage(ctx, publisher, "AAAA")

	// Sending a third message fails because of the outstanding message.
	r3 := publishSingleMessage(ctx, publisher, "AA")
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
	r4 := publishSingleMessage(ctx, publisher, "AAAA")
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

	topicName := fmt.Sprintf("projects/p/topics/%s", "fc-error-ordering-topic")
	publisher := mustCreateTopic(t, c, topicName)
	fc := FlowControlSettings{
		MaxOutstandingMessages: 1,
		MaxOutstandingBytes:    10,
		LimitExceededBehavior:  FlowControlSignalError,
	}
	publisher.PublishSettings.FlowControlSettings = fc
	publisher.PublishSettings.DelayThreshold = 5 * time.Second
	publisher.PublishSettings.CountThreshold = 1
	publisher.EnableMessageOrdering = true

	// Sending a message that is too large results in an error.
	r1 := publishSingleMessageWithKey(ctx, publisher, "AAAAAAAAAAA", "a")
	if _, err := r1.Get(ctx); err != ErrFlowControllerMaxOutstandingBytes {
		t.Fatalf("r1.Get() got: %v, want %v", err, ErrFlowControllerMaxOutstandingBytes)
	}

	// Sending a second message for the same ordering key fails because the first one failed.
	r2 := publishSingleMessageWithKey(ctx, publisher, "AAAA", "a")
	if _, err := r2.Get(ctx); err == nil {
		t.Fatal("r2.Get() got nil instead of error before calling publisher.ResumePublish(key)")
	}
}

func TestPublishFlowControl_Block(t *testing.T) {
	ctx := context.Background()
	c, srv := newFake(t)
	defer c.Close()
	defer srv.Close()

	publisher := mustCreateTopic(t, c, "fc-block-topic")
	fc := FlowControlSettings{
		MaxOutstandingMessages: 2,
		MaxOutstandingBytes:    10,
		LimitExceededBehavior:  FlowControlBlock,
	}
	publisher.PublishSettings.FlowControlSettings = fc
	publisher.PublishSettings.DelayThreshold = 5 * time.Second
	publisher.PublishSettings.CountThreshold = 1

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
	publishSingleMessage(ctx, publisher, "AA")
	publishSingleMessage(ctx, publisher, "AA")

	// Sending a third message blocks because the messages are outstanding.
	var publish3Completed sync.WaitGroup
	publish3Completed.Add(1)
	go func() {
		publishSingleMessage(ctx, publisher, "AAAAAA")
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
		publishSingleMessage(ctx, publisher, "A")
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

	publisher := mustCreateTopic(t, c, "invalid-utf8-topic")
	res := publisher.Publish(ctx, &Message{
		Data: []byte("foo"),
		Attributes: map[string]string{
			"attr": "a\xc5z",
		},
	})
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	_, err := res.Get(ctx)
	if err == nil || !strings.Contains(err.Error(), "string field contains invalid UTF-8") {
		t.Fatalf("expected invalid UTF-8 error, got: %v", err)
	}
}

// publishSingleMessage publishes a single message to a topic.
func publishSingleMessage(ctx context.Context, p *Publisher, data string) *PublishResult {
	return p.Publish(ctx, &Message{
		Data: []byte(data),
	})
}

// publishSingleMessageWithKey publishes a single message to a topic with an ordering key.
func publishSingleMessageWithKey(ctx context.Context, p *Publisher, data, key string) *PublishResult {
	return p.Publish(ctx, &Message{
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

	publisher := mustCreateTopic(t, c, "test-topic")
	res := publishSingleMessageWithKey(ctx, publisher, "test", "non-existent-key")
	if _, err := res.Get(ctx); !errors.Is(err, errTopicOrderingNotEnabled) {
		t.Errorf("got %v, want errTopicOrderingNotEnabled", err)
	}
}

func TestPublishCompression(t *testing.T) {
	ctx := context.Background()
	client, srv := newFake(t)
	defer client.Close()
	defer srv.Close()

	topic := fmt.Sprintf("projects/%s/topics/topic-compression", testutil.ProjID())
	publisher := mustCreateTopic(t, client, topic)
	defer publisher.Stop()

	publisher.PublishSettings.EnableCompression = true
	publisher.PublishSettings.CompressionBytesThreshold = 50

	const messageSizeBytes = 1000

	msg := &Message{Data: bytes.Repeat([]byte{'A'}, int(messageSizeBytes))}
	res := publisher.Publish(ctx, msg)

	_, err := res.Get(ctx)
	if err != nil {
		t.Errorf("publish result got err: %v", err)
	}
}

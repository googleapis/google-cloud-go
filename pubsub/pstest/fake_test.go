// Copyright 2017 Google LLC
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

package pstest

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"math/rand"
	"net"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"

	iampb "cloud.google.com/go/iam/apiv1/iampb"
	"cloud.google.com/go/internal/testutil"
	pb "cloud.google.com/go/pubsub/apiv1/pubsubpb"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/durationpb"
	field_mask "google.golang.org/protobuf/types/known/fieldmaskpb"
	"google.golang.org/protobuf/types/known/timestamppb"
)

func TestNewServerWithPort(t *testing.T) {
	// Allocate an available port to use with NewServerWithPort and then close it so it's available.
	// Note: There is no guarantee that the port does not become used between closing
	// the listener and creating the new server with NewServerWithPort, but the chances are
	// very small.
	l, err := net.Listen("tcp", ":0")
	if err != nil {
		t.Fatal(err)
	}
	port := l.Addr().(*net.TCPAddr).Port
	l.Close()

	// Pass a non 0 port to demonstrate we can pass a hardcoded port for the server to listen on
	srv := NewServerWithPort(port)
	if err != nil {
		t.Fatal(err)
	}
	defer srv.Close()
	conn, err := grpc.Dial(srv.Addr, grpc.WithInsecure())
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
}

func TestNewServerWithCallback(t *testing.T) {
	// Allocate an available port to use with NewServerWithPort and then close it so it's available.
	// Note: There is no guarantee that the port does not become used between closing
	// the listener and creating the new server with NewServerWithPort, but the chances are
	// very small.
	l, err := net.Listen("tcp", ":0")
	if err != nil {
		t.Fatal(err)
	}
	port := l.Addr().(*net.TCPAddr).Port
	l.Close()

	additionalFake := struct {
		iampb.UnimplementedIAMPolicyServer
	}{}

	verifyCallback := false
	callback := func(grpc *grpc.Server) {
		// register something
		iampb.RegisterIAMPolicyServer(grpc, &additionalFake)
		verifyCallback = true
	}

	// Pass a non 0 port to demonstrate we can pass a hardcoded port for the server to listen on
	srv := NewServerWithCallback(port, callback)
	if err != nil {
		t.Fatal(err)
	}
	defer srv.Close()

	conn, err := grpc.NewClient(srv.Addr, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()

	if !verifyCallback {
		t.Fatal("callback was not invoked")
	}
}

func TestTopics(t *testing.T) {
	pclient, sclient, server, cleanup := newFake(context.TODO(), t)
	defer cleanup()

	ctx := context.Background()
	var topics []*pb.Topic
	for i := 1; i < 3; i++ {
		topics = append(topics, mustCreateTopic(context.TODO(), t, pclient, &pb.Topic{
			Name:   fmt.Sprintf("projects/P/topics/T%d", i),
			Labels: map[string]string{"num": fmt.Sprintf("%d", i)},
		}))
	}
	if got, want := len(server.GServer.topics), len(topics); got != want {
		t.Fatalf("got %d topics, want %d", got, want)
	}
	for _, top := range topics {
		got, err := pclient.GetTopic(ctx, &pb.GetTopicRequest{Topic: top.Name})
		if err != nil {
			t.Fatal(err)
		}
		if !testutil.Equal(got, top) {
			t.Errorf("\ngot %+v\nwant %+v", got, top)
		}
	}

	res, err := pclient.ListTopics(ctx, &pb.ListTopicsRequest{Project: "projects/P"})
	if err != nil {
		t.Fatal(err)
	}
	if got, want := res.Topics, topics; !testutil.Equal(got, want) {
		t.Errorf("\ngot %+v\nwant %+v", got, want)
	}

	for _, top := range topics {
		if _, err := pclient.DeleteTopic(ctx, &pb.DeleteTopicRequest{Topic: top.Name}); err != nil {
			t.Fatal(err)
		}
	}
	if got, want := len(server.GServer.topics), 0; got != want {
		t.Fatalf("got %d topics, want %d", got, want)
	}

	t.Run(`Given a topic that is used by a subscription as deadLetter,
	When topic deleted,
	Then error raised`, func(t *testing.T) {
		var topics []*pb.Topic
		for i := 1; i < 3; i++ {
			topics = append(topics, mustCreateTopic(context.TODO(), t, pclient, &pb.Topic{
				Name:   fmt.Sprintf("projects/P/topics/T%d", i),
				Labels: map[string]string{"num": fmt.Sprintf("%d", i)},
			}))
		}

		if got, want := len(server.GServer.topics), len(topics); got != want {
			t.Fatalf("got %d topics, want %d", got, want)
		}

		s := mustCreateSubscription(ctx, t, sclient, &pb.Subscription{
			Name:               fmt.Sprintf("project/P/subscriptions/sub_with_deadLetter"),
			Topic:              topics[0].Name,
			AckDeadlineSeconds: 10,
			DeadLetterPolicy: &pb.DeadLetterPolicy{
				DeadLetterTopic: topics[1].Name,
			},
		})

		_, err := pclient.DeleteTopic(ctx, &pb.DeleteTopicRequest{
			Topic: topics[1].Name,
		})
		expectedErr := status.Errorf(codes.FailedPrecondition, "topic %q used as deadLetter for %s", topics[1].Name, s.Name)
		if err == nil || !errors.Is(err, expectedErr) {
			t.Fatalf("returned a different error than the expected one. \nReceived '%s'; \nExpected: '%s'", err, expectedErr)
		}
	})
}

func TestSubscriptions(t *testing.T) {
	pclient, sclient, server, cleanup := newFake(context.TODO(), t)
	defer cleanup()

	ctx := context.Background()
	topic := mustCreateTopic(context.TODO(), t, pclient, &pb.Topic{Name: "projects/P/topics/T"})
	var subs []*pb.Subscription
	for i := 0; i < 3; i++ {
		subs = append(subs, mustCreateSubscription(context.TODO(), t, sclient, &pb.Subscription{
			Name:               fmt.Sprintf("projects/P/subscriptions/S%d", i),
			Topic:              topic.Name,
			AckDeadlineSeconds: int32(10 * (i + 1)),
		}))
	}

	if got, want := len(server.GServer.subs), len(subs); got != want {
		t.Fatalf("got %d subscriptions, want %d", got, want)
	}
	for _, s := range subs {
		got, err := sclient.GetSubscription(ctx, &pb.GetSubscriptionRequest{Subscription: s.Name})
		if err != nil {
			t.Fatal(err)
		}
		if !testutil.Equal(got, s) {
			t.Errorf("\ngot %+v\nwant %+v", got, s)
		}
	}

	res, err := sclient.ListSubscriptions(ctx, &pb.ListSubscriptionsRequest{Project: "projects/P"})
	if err != nil {
		t.Fatal(err)
	}
	if got, want := res.Subscriptions, subs; !testutil.Equal(got, want) {
		t.Errorf("\ngot %+v\nwant %+v", got, want)
	}

	res2, err := pclient.ListTopicSubscriptions(ctx, &pb.ListTopicSubscriptionsRequest{Topic: topic.Name})
	if err != nil {
		t.Fatal(err)
	}
	if got, want := len(res2.Subscriptions), len(subs); got != want {
		t.Fatalf("got %d subs, want %d", got, want)
	}
	for i, got := range res2.Subscriptions {
		want := subs[i].Name
		if !testutil.Equal(got, want) {
			t.Errorf("\ngot %+v\nwant %+v", got, want)
		}
	}

	subToDetach := "projects/P/subscriptions/S0"
	_, err = pclient.DetachSubscription(ctx, &pb.DetachSubscriptionRequest{
		Subscription: subToDetach,
	})
	if err != nil {
		t.Fatalf("attempted to detach sub %s, got error: %v", subToDetach, err)
	}

	for _, s := range subs {
		if _, err := sclient.DeleteSubscription(ctx, &pb.DeleteSubscriptionRequest{Subscription: s.Name}); err != nil {
			t.Fatal(err)
		}
	}
	if got, want := len(server.GServer.subs), 0; got != want {
		t.Fatalf("got %d subscriptions, want %d", got, want)
	}

	t.Run(`Given a subscription creation,
	When called with a deadLetter topic that does not exist,
	Then error returned`, func(t *testing.T) {
		topic := mustCreateTopic(ctx, t, pclient, &pb.Topic{Name: "projects/P/topics/test"})
		_, err := server.GServer.CreateSubscription(ctx, &pb.Subscription{
			Name:               "projects/P/subscriptions/test",
			Topic:              topic.Name,
			AckDeadlineSeconds: 10,
			DeadLetterPolicy: &pb.DeadLetterPolicy{
				DeadLetterTopic: "projects/P/topics/nonexisting",
			},
		})
		expectedErr := status.Errorf(codes.NotFound, "deadLetter topic \"projects/P/topics/nonexisting\"")
		if err == nil || !errors.Is(err, expectedErr) {
			t.Fatalf("expected subscription creation to fail with a specific err but it didn't. \nError: %s \nExpected err: %s", err, expectedErr)
		}
		_, err = server.GServer.DeleteTopic(ctx, &pb.DeleteTopicRequest{
			Topic: topic.Name,
		})
		if err != nil {
			t.Fatalf("unexpected error during deleting topic")
		}
	})
}

func TestSubscriptionErrors(t *testing.T) {
	_, sclient, _, cleanup := newFake(context.TODO(), t)
	defer cleanup()

	ctx := context.Background()

	checkCode := func(err error, want codes.Code) {
		t.Helper()
		if status.Code(err) != want {
			t.Errorf("got %v, want code %s", err, want)
		}
	}

	_, err := sclient.GetSubscription(ctx, &pb.GetSubscriptionRequest{})
	checkCode(err, codes.InvalidArgument)
	_, err = sclient.GetSubscription(ctx, &pb.GetSubscriptionRequest{Subscription: "s"})
	checkCode(err, codes.NotFound)
	_, err = sclient.UpdateSubscription(ctx, &pb.UpdateSubscriptionRequest{})
	checkCode(err, codes.InvalidArgument)
	_, err = sclient.UpdateSubscription(ctx, &pb.UpdateSubscriptionRequest{Subscription: &pb.Subscription{}})
	checkCode(err, codes.InvalidArgument)
	_, err = sclient.UpdateSubscription(ctx, &pb.UpdateSubscriptionRequest{Subscription: &pb.Subscription{Name: "s"}})
	checkCode(err, codes.NotFound)
	_, err = sclient.DeleteSubscription(ctx, &pb.DeleteSubscriptionRequest{})
	checkCode(err, codes.InvalidArgument)
	_, err = sclient.DeleteSubscription(ctx, &pb.DeleteSubscriptionRequest{Subscription: "s"})
	checkCode(err, codes.NotFound)
	_, err = sclient.Acknowledge(ctx, &pb.AcknowledgeRequest{})
	checkCode(err, codes.InvalidArgument)
	_, err = sclient.Acknowledge(ctx, &pb.AcknowledgeRequest{Subscription: "s"})
	checkCode(err, codes.NotFound)
	_, err = sclient.ModifyAckDeadline(ctx, &pb.ModifyAckDeadlineRequest{})
	checkCode(err, codes.InvalidArgument)
	_, err = sclient.ModifyAckDeadline(ctx, &pb.ModifyAckDeadlineRequest{Subscription: "s"})
	checkCode(err, codes.NotFound)
	_, err = sclient.Pull(ctx, &pb.PullRequest{})
	checkCode(err, codes.InvalidArgument)
	_, err = sclient.Pull(ctx, &pb.PullRequest{Subscription: "s"})
	checkCode(err, codes.NotFound)
	_, err = sclient.Seek(ctx, &pb.SeekRequest{})
	checkCode(err, codes.InvalidArgument)
	srt := &pb.SeekRequest_Time{Time: timestamppb.Now()}
	_, err = sclient.Seek(ctx, &pb.SeekRequest{Target: srt})
	checkCode(err, codes.InvalidArgument)
	_, err = sclient.Seek(ctx, &pb.SeekRequest{Target: srt, Subscription: "s"})
	checkCode(err, codes.NotFound)
}

func TestSubscriptionDeadLetter(t *testing.T) {
	_, _, server, cleanup := newFake(context.TODO(), t)
	defer cleanup()

	ctx := context.Background()

	topic, err := server.GServer.CreateTopic(ctx, &pb.Topic{
		Name: "projects/P/topics/in",
	})
	if err != nil {
		t.Fatalf("failed to create in topic")
	}
	deadLetterTopic, err := server.GServer.CreateTopic(ctx, &pb.Topic{
		Name: "projects/P/topics/deadLetter",
	})
	if err != nil {
		t.Fatalf("failed to create deadLetter topic")
	}
	retries := 3
	sub, err := server.GServer.CreateSubscription(ctx, &pb.Subscription{
		Name:               "projects/P/subscriptions/S",
		Topic:              topic.Name,
		AckDeadlineSeconds: 10,
		DeadLetterPolicy: &pb.DeadLetterPolicy{
			DeadLetterTopic:     deadLetterTopic.Name,
			MaxDeliveryAttempts: int32(retries),
		},
	})
	if err != nil {
		t.Fatalf("failed to create subscription")
	}
	dlSub, err := server.GServer.CreateSubscription(ctx, &pb.Subscription{
		Name:               "projects/P/subscriptions/SD",
		Topic:              deadLetterTopic.Name,
		AckDeadlineSeconds: 10,
	})
	if err != nil {
		t.Fatalf("failed to create subscription")
	}

	messageData := []byte("message data")
	_, err = server.GServer.Publish(ctx, &pb.PublishRequest{
		Topic: topic.Name,
		Messages: []*pb.PubsubMessage{
			{
				Data: messageData,
			},
		},
	})
	if err != nil {
		t.Fatalf("failed to publish message")
	}
	rand.Seed(time.Now().UTC().UnixNano())
	maxAttempts := rand.Intn(5) + retries
	for i := 0; i < maxAttempts; i++ {
		pull, err := server.GServer.Pull(ctx, &pb.PullRequest{
			Subscription: sub.Name,
			MaxMessages:  10,
		})
		if err != nil {
			t.Fatalf("failed during pull")
		}
		if i < retries {
			if len(pull.ReceivedMessages) != 1 {
				t.Fatalf("expected 1 message received a different number %d", len(pull.ReceivedMessages))

			}
			for _, m := range pull.ReceivedMessages {
				if int32(i+1) != m.DeliveryAttempt {
					t.Fatalf("message delivery attempt not the expected one. expected: %d, actual: %d", i+1, m.DeliveryAttempt)
				}
				_, err := server.GServer.ModifyAckDeadline(ctx, &pb.ModifyAckDeadlineRequest{
					Subscription:       sub.Name,
					AckIds:             []string{m.AckId},
					AckDeadlineSeconds: 0,
				})
				if err != nil {
					t.Fatalf("failed to modify ack deadline")
				}
			}
		} else {
			if len(pull.ReceivedMessages) > 0 {
				t.Fatalf("received a non empty list of messages %d", len(pull.ReceivedMessages))
			}
		}
	}

	dlPull, err := server.GServer.Pull(ctx, &pb.PullRequest{
		Subscription: dlSub.Name,
		MaxMessages:  10,
	})
	if err != nil {
		t.Fatalf("failed during pulling from deadLetter sub")
	}
	if len(dlPull.ReceivedMessages) != 1 {
		t.Fatalf("expected 1 message received a different number %d", len(dlPull.ReceivedMessages))
	}
	receivedMessage := dlPull.ReceivedMessages[0]
	if bytes.Compare(receivedMessage.Message.Data, messageData) != 0 {
		t.Fatalf("unexpected message received from deadLetter")
	}
	if receivedMessage.DeliveryAttempt > 0 {
		t.Fatalf("message sent to deadLetter should not have the deliveryAttempt value from the original subscription message")
	}
	_, err = server.GServer.Acknowledge(ctx, &pb.AcknowledgeRequest{
		Subscription: dlSub.Name,
		AckIds:       []string{receivedMessage.GetAckId()},
	})
	if err != nil {
		t.Fatalf("failed to acknowledge message from deadLetter")
	}

	for _, s := range []string{"projects/P/subscriptions/S", "projects/P/subscriptions/SD"} {
		_, err = server.GServer.DeleteSubscription(ctx, &pb.DeleteSubscriptionRequest{
			Subscription: s,
		})
		if err != nil {
			t.Fatalf("failed to delete subscription %s; error: %s", s, err)
		}
	}

	for _, delTopic := range []string{"projects/P/topics/in", "projects/P/topics/deadLetter"} {
		_, err = server.GServer.DeleteTopic(ctx, &pb.DeleteTopicRequest{
			Topic: delTopic,
		})
		if err != nil {
			t.Fatalf("failed to delete topic %s; error: %s", delTopic, err)
		}
	}

	if got, want := len(server.GServer.subs), 0; got != want {
		t.Fatalf("got %d subscriptions, want %d", got, want)
	}

	if got, want := len(server.GServer.topics), 0; got != want {
		t.Fatalf("got %d topics, want %d", got, want)
	}
}

func TestPublish(t *testing.T) {
	s := NewServer()
	defer s.Close()

	var ids []string
	for i := 0; i < 3; i++ {
		ids = append(ids, s.Publish("projects/p/topics/t", []byte("hello"), nil))
	}
	s.Wait()
	ms := s.Messages()
	if got, want := len(ms), len(ids); got != want {
		t.Errorf("got %d messages, want %d", got, want)
	}
	for i, id := range ids {
		if got, want := ms[i].ID, id; got != want {
			t.Errorf("got %s, want %s", got, want)
		}
	}

	m := s.Message(ids[1])
	if m == nil {
		t.Error("got nil, want a message")
	}
}

func TestPublishOrdered(t *testing.T) {
	s := NewServer()
	defer s.Close()

	const orderingKey = "ordering-key"
	var ids []string
	for i := 0; i < 3; i++ {
		ids = append(ids, s.PublishOrdered("projects/p/topics/t", []byte("hello"), nil, orderingKey))
	}
	s.Wait()
	ms := s.Messages()
	if got, want := len(ms), len(ids); got != want {
		t.Errorf("got %d messages, want %d", got, want)
	}
	for i, id := range ids {
		if got, want := ms[i].ID, id; got != want {
			t.Errorf("got %s, want %s", got, want)
		}
		if got, want := ms[i].OrderingKey, orderingKey; got != want {
			t.Errorf("got %s, want %s", got, want)
		}
	}

	m := s.Message(ids[1])
	if m == nil {
		t.Error("got nil, want a message")
	}
}

func TestClearMessages(t *testing.T) {
	pclient, sclient, s, cleanup := newFake(context.TODO(), t)
	defer cleanup()

	top := mustCreateTopic(context.TODO(), t, pclient, &pb.Topic{Name: "projects/P/topics/T"})
	sub := mustCreateSubscription(context.TODO(), t, sclient, &pb.Subscription{
		Name:               "projects/P/subscriptions/S",
		Topic:              top.Name,
		AckDeadlineSeconds: 10,
	})

	for i := 0; i < 3; i++ {
		s.Publish(top.Name, []byte("hello"), nil)
	}
	msgs := s.Messages()
	if got, want := len(msgs), 3; got != want {
		t.Errorf("got %d messages, want %d", got, want)
	}
	s.ClearMessages()
	msgs = s.Messages()
	if got, want := len(msgs), 0; got != want {
		t.Errorf("got %d messages, want %d", got, want)
	}

	res, err := sclient.Pull(context.Background(), &pb.PullRequest{Subscription: sub.Name})
	if err != nil {
		t.Fatal(err)
	}
	if len(res.ReceivedMessages) != 0 {
		t.Errorf("got %d messages, want zero", len(res.ReceivedMessages))
	}
}

// Note: this sets the fake's "now" time, so it is sensitive to concurrent changes to "now".
func publish(t *testing.T, srv *Server, pclient pb.PublisherClient, topic *pb.Topic, messages []*pb.PubsubMessage) map[string]*pb.PubsubMessage {
	pubTime := time.Now()
	srv.SetTimeNowFunc(func() time.Time { return pubTime })
	defer srv.SetTimeNowFunc(time.Now)

	res, err := pclient.Publish(context.Background(), &pb.PublishRequest{
		Topic:    topic.Name,
		Messages: messages,
	})
	if err != nil {
		t.Fatal(err)
	}
	tsPubTime := timestamppb.New(pubTime)
	want := map[string]*pb.PubsubMessage{}
	for i, id := range res.MessageIds {
		want[id] = &pb.PubsubMessage{
			Data:        messages[i].Data,
			Attributes:  messages[i].Attributes,
			MessageId:   id,
			PublishTime: tsPubTime,
		}
	}
	return want
}

func TestPull(t *testing.T) {
	pclient, sclient, srv, cleanup := newFake(context.TODO(), t)
	defer cleanup()

	top := mustCreateTopic(context.TODO(), t, pclient, &pb.Topic{Name: "projects/P/topics/T"})
	sub := mustCreateSubscription(context.TODO(), t, sclient, &pb.Subscription{
		Name:               "projects/P/subscriptions/S",
		Topic:              top.Name,
		AckDeadlineSeconds: 10,
	})

	want := publish(t, srv, pclient, top, []*pb.PubsubMessage{
		{Data: []byte("d1")},
		{Data: []byte("d2")},
		{Data: []byte("d3")},
	})
	got := pubsubMessages(pullN(context.TODO(), t, len(want), sclient, sub))
	if diff := testutil.Diff(got, want); diff != "" {
		t.Error(diff)
	}

	res, err := sclient.Pull(context.Background(), &pb.PullRequest{Subscription: sub.Name})
	if err != nil {
		t.Fatal(err)
	}
	if len(res.ReceivedMessages) != 0 {
		t.Errorf("got %d messages, want zero", len(res.ReceivedMessages))
	}
}

func TestStreamingPull(t *testing.T) {
	// A simple test of streaming pull.
	pclient, sclient, srv, cleanup := newFake(context.TODO(), t)
	defer cleanup()

	deadLetterTopic := mustCreateTopic(context.TODO(), t, pclient, &pb.Topic{
		Name: "projects/P/topics/deadLetter",
	})

	top := mustCreateTopic(context.TODO(), t, pclient, &pb.Topic{Name: "projects/P/topics/T"})
	sub := mustCreateSubscription(context.TODO(), t, sclient, &pb.Subscription{
		Name:               "projects/P/subscriptions/S",
		Topic:              top.Name,
		AckDeadlineSeconds: 10,
		DeadLetterPolicy: &pb.DeadLetterPolicy{
			DeadLetterTopic:     deadLetterTopic.Name,
			MaxDeliveryAttempts: 3,
		},
	})

	want := publish(t, srv, pclient, top, []*pb.PubsubMessage{
		{Data: []byte("d1")},
		{Data: []byte("d2")},
		{Data: []byte("d3")},
	})
	received := streamingPullN(context.TODO(), t, len(want), sclient, sub)
	for _, m := range received {
		if m.DeliveryAttempt != 1 {
			t.Errorf("got DeliveryAttempt==%d, want 1", m.DeliveryAttempt)
		}
	}
	got := pubsubMessages(received)
	if diff := testutil.Diff(got, want); diff != "" {
		t.Error(diff)
	}
}

// This test acks each message as it arrives and makes sure we don't see dups.
func TestStreamingPullAck(t *testing.T) {
	minAckDeadlineSecs = 1
	pclient, sclient, srv, cleanup := newFake(context.TODO(), t)
	defer cleanup()

	top := mustCreateTopic(context.TODO(), t, pclient, &pb.Topic{Name: "projects/P/topics/T"})
	sub := mustCreateSubscription(context.TODO(), t, sclient, &pb.Subscription{
		Name:               "projects/P/subscriptions/S",
		Topic:              top.Name,
		AckDeadlineSeconds: 1,
	})

	_ = publish(t, srv, pclient, top, []*pb.PubsubMessage{
		{Data: []byte("d1")},
		{Data: []byte("d2")},
		{Data: []byte("d3")},
	})

	got := map[string]bool{}
	ctx, cancel := context.WithCancel(context.Background())
	spc := mustStartStreamingPull(ctx, t, sclient, sub)
	time.AfterFunc(time.Duration(2*minAckDeadlineSecs)*time.Second, cancel)

	for i := 0; i < 4; i++ {
		res, err := spc.Recv()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			if status.Code(err) == codes.Canceled {
				break
			}
			t.Fatal(err)
		}
		if i == 3 {
			t.Fatal("expected to only see 3 messages, got 4")
		}
		req := &pb.StreamingPullRequest{}
		for _, m := range res.ReceivedMessages {
			if got[m.Message.MessageId] {
				t.Fatal("duplicate message")
			}
			got[m.Message.MessageId] = true
			req.AckIds = append(req.AckIds, m.AckId)
		}
		if err := spc.Send(req); err != nil {
			t.Fatal(err)
		}
	}
}

func TestAcknowledge(t *testing.T) {
	ctx := context.Background()
	pclient, sclient, srv, cleanup := newFake(context.TODO(), t)
	defer cleanup()

	top := mustCreateTopic(context.TODO(), t, pclient, &pb.Topic{Name: "projects/P/topics/T"})
	sub := mustCreateSubscription(context.TODO(), t, sclient, &pb.Subscription{
		Name:               "projects/P/subscriptions/S",
		Topic:              top.Name,
		AckDeadlineSeconds: 10,
	})

	publish(t, srv, pclient, top, []*pb.PubsubMessage{
		{Data: []byte("d1")},
		{Data: []byte("d2")},
		{Data: []byte("d3")},
	})
	msgs := streamingPullN(context.TODO(), t, 3, sclient, sub)
	var ackIDs []string
	for _, m := range msgs {
		ackIDs = append(ackIDs, m.AckId)
	}
	if _, err := sclient.Acknowledge(ctx, &pb.AcknowledgeRequest{
		Subscription: sub.Name,
		AckIds:       ackIDs,
	}); err != nil {
		t.Fatal(err)
	}
	smsgs := srv.Messages()
	if got, want := len(smsgs), 3; got != want {
		t.Fatalf("got %d messages, want %d", got, want)
	}
	for _, sm := range smsgs {
		if sm.Acks != 1 {
			t.Errorf("message %s: got %d acks, want 1", sm.ID, sm.Acks)
		}
	}
}

func TestModAck(t *testing.T) {
	ctx := context.Background()
	pclient, sclient, srv, cleanup := newFake(context.TODO(), t)
	defer cleanup()

	top := mustCreateTopic(context.TODO(), t, pclient, &pb.Topic{Name: "projects/P/topics/T"})
	sub := mustCreateSubscription(context.TODO(), t, sclient, &pb.Subscription{
		Name:               "projects/P/subscriptions/S",
		Topic:              top.Name,
		AckDeadlineSeconds: 10,
	})

	publish(t, srv, pclient, top, []*pb.PubsubMessage{
		{Data: []byte("d1")},
		{Data: []byte("d2")},
		{Data: []byte("d3")},
	})
	msgs := streamingPullN(context.TODO(), t, 3, sclient, sub)
	var ackIDs []string
	for _, m := range msgs {
		ackIDs = append(ackIDs, m.AckId)
	}
	if _, err := sclient.ModifyAckDeadline(ctx, &pb.ModifyAckDeadlineRequest{
		Subscription:       sub.Name,
		AckIds:             ackIDs,
		AckDeadlineSeconds: 0,
	}); err != nil {
		t.Fatal(err)
	}
	// Having nacked all three messages, we should see them again.
	msgs = streamingPullN(context.TODO(), t, 3, sclient, sub)
	if got, want := len(msgs), 3; got != want {
		t.Errorf("got %d messages, want %d", got, want)
	}
}

func TestAckDeadline(t *testing.T) {
	// Messages should be resent after they expire.
	pclient, sclient, srv, cleanup := newFake(context.TODO(), t)
	defer cleanup()

	minAckDeadlineSecs = 2
	top := mustCreateTopic(context.TODO(), t, pclient, &pb.Topic{Name: "projects/P/topics/T"})
	sub := mustCreateSubscription(context.TODO(), t, sclient, &pb.Subscription{
		Name:               "projects/P/subscriptions/S",
		Topic:              top.Name,
		AckDeadlineSeconds: minAckDeadlineSecs,
	})

	_ = publish(t, srv, pclient, top, []*pb.PubsubMessage{
		{Data: []byte("d1")},
		{Data: []byte("d2")},
		{Data: []byte("d3")},
	})

	got := map[string]int{}
	spc := mustStartStreamingPull(context.TODO(), t, sclient, sub)
	// In 5 seconds the ack deadline will expire twice, so we should see each message
	// exactly three times.
	time.AfterFunc(5*time.Second, func() {
		if err := spc.CloseSend(); err != nil {
			t.Errorf("CloseSend: %v", err)
		}
	})
	for {
		res, err := spc.Recv()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			t.Fatal(err)
		}
		for _, m := range res.ReceivedMessages {
			got[m.Message.MessageId]++
		}
	}
	for id, n := range got {
		if n != 3 {
			t.Errorf("message %s: saw %d times, want 3", id, n)
		}
	}
}

func TestMultiSubs(t *testing.T) {
	// Each subscription gets every message.
	pclient, sclient, srv, cleanup := newFake(context.TODO(), t)
	defer cleanup()

	top := mustCreateTopic(context.TODO(), t, pclient, &pb.Topic{Name: "projects/P/topics/T"})
	sub1 := mustCreateSubscription(context.TODO(), t, sclient, &pb.Subscription{
		Name:               "projects/P/subscriptions/S1",
		Topic:              top.Name,
		AckDeadlineSeconds: 10,
	})
	sub2 := mustCreateSubscription(context.TODO(), t, sclient, &pb.Subscription{
		Name:               "projects/P/subscriptions/S2",
		Topic:              top.Name,
		AckDeadlineSeconds: 10,
	})

	want := publish(t, srv, pclient, top, []*pb.PubsubMessage{
		{Data: []byte("d1")},
		{Data: []byte("d2")},
		{Data: []byte("d3")},
	})
	got1 := pubsubMessages(streamingPullN(context.TODO(), t, len(want), sclient, sub1))
	got2 := pubsubMessages(streamingPullN(context.TODO(), t, len(want), sclient, sub2))
	if diff := testutil.Diff(got1, want); diff != "" {
		t.Error(diff)
	}
	if diff := testutil.Diff(got2, want); diff != "" {
		t.Error(diff)
	}
}

// Messages are handed out to all streams of a subscription in a best-effort
// round-robin behavior. The fake server prefers to fail-fast onto another
// stream when one stream is already busy, though, so we're unable to test
// strict round robin behavior.
func TestMultiStreams(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	pclient, sclient, srv, cleanup := newFake(ctx, t)
	defer cleanup()

	top := mustCreateTopic(ctx, t, pclient, &pb.Topic{Name: "projects/P/topics/T"})
	sub := mustCreateSubscription(ctx, t, sclient, &pb.Subscription{
		Name:               "projects/P/subscriptions/S",
		Topic:              top.Name,
		AckDeadlineSeconds: 10,
	})
	st1 := mustStartStreamingPull(ctx, t, sclient, sub)
	defer st1.CloseSend()
	st1Received := make(chan struct{})
	go func() {
		_, err := st1.Recv()
		if err != nil {
			t.Error(err)
		}
		close(st1Received)
	}()

	st2 := mustStartStreamingPull(ctx, t, sclient, sub)
	defer st2.CloseSend()
	st2Received := make(chan struct{})
	go func() {
		_, err := st2.Recv()
		if err != nil {
			t.Error(err)
		}
		close(st2Received)
	}()

	publish(t, srv, pclient, top, []*pb.PubsubMessage{
		{Data: []byte("d1")},
		{Data: []byte("d2")},
	})

	timeout := time.After(5 * time.Second)
	select {
	case <-timeout:
		t.Fatal("timed out waiting for stream 1 to receive any message")
	case <-st1Received:
	}
	select {
	case <-timeout:
		t.Fatal("timed out waiting for stream 1 to receive any message")
	case <-st2Received:
	}
}

func TestStreamingPullTimeout(t *testing.T) {
	pclient, sclient, srv, cleanup := newFake(context.TODO(), t)
	defer cleanup()

	timeout := 200 * time.Millisecond
	srv.SetStreamTimeout(timeout)
	top := mustCreateTopic(context.TODO(), t, pclient, &pb.Topic{Name: "projects/P/topics/T"})
	sub := mustCreateSubscription(context.TODO(), t, sclient, &pb.Subscription{
		Name:               "projects/P/subscriptions/S",
		Topic:              top.Name,
		AckDeadlineSeconds: 10,
	})
	stream := mustStartStreamingPull(context.TODO(), t, sclient, sub)
	time.Sleep(2 * timeout)
	_, err := stream.Recv()
	if err != io.EOF {
		t.Errorf("got %v, want io.EOF", err)
	}
}

func TestSeek(t *testing.T) {
	pclient, sclient, _, cleanup := newFake(context.TODO(), t)
	defer cleanup()

	top := mustCreateTopic(context.TODO(), t, pclient, &pb.Topic{Name: "projects/P/topics/T"})
	sub := mustCreateSubscription(context.TODO(), t, sclient, &pb.Subscription{
		Name:               "projects/P/subscriptions/S",
		Topic:              top.Name,
		AckDeadlineSeconds: 10,
	})
	ts := timestamppb.Now()
	_, err := sclient.Seek(context.Background(), &pb.SeekRequest{
		Subscription: sub.Name,
		Target:       &pb.SeekRequest_Time{Time: ts},
	})
	if err != nil {
		t.Errorf("Seeking: %v", err)
	}
}

func TestTryDeliverMessage(t *testing.T) {
	for _, test := range []struct {
		availStreamIdx int
		expectedOutIdx int
	}{
		{availStreamIdx: 0, expectedOutIdx: 0},
		// Stream 1 will always be marked for deletion.
		{availStreamIdx: 2, expectedOutIdx: 1}, // s0, s1 (deleted), s2, s3 becomes s0, s2, s3. So we expect outIdx=1.
		{availStreamIdx: 3, expectedOutIdx: 2}, // s0, s1 (deleted), s2, s3 becomes s0, s2, s3. So we expect outIdx=2.
	} {
		top := newTopic(&pb.Topic{Name: "some-topic"})
		sub := newSubscription(top, &sync.Mutex{}, time.Now, nil, &pb.Subscription{Name: "some-sub", Topic: "some-topic"})

		done := make(chan struct{}, 1)
		done <- struct{}{}
		sub.streams = []*stream{{}, {done: done}, {}, {}}

		msgc := make(chan *pb.ReceivedMessage, 1)
		sub.streams[test.availStreamIdx].msgc = msgc

		var d int
		idx, ok := sub.tryDeliverMessage(&message{deliveries: &d}, 0, time.Now())
		if !ok {
			t.Fatalf("[avail=%d]: expected msg to be put on stream %d's channel, but it was not", test.availStreamIdx, test.expectedOutIdx)
		}
		if idx != test.expectedOutIdx {
			t.Fatalf("[avail=%d]: expected msg to be put on stream %d, but it was put on %d", test.availStreamIdx, test.expectedOutIdx, idx)
		}
		select {
		case <-msgc:
		default:
			t.Fatalf("[avail=%d]: expected msg to be put on stream %d's channel, but it was not", test.availStreamIdx, idx)
		}
	}
}

func TestTimeNowFunc(t *testing.T) {
	s := NewServer()
	defer s.Close()

	timeFunc := func() time.Time {
		t, _ := time.Parse(time.RFC3339, "2006-01-02T15:04:05Z")
		return t
	}
	s.SetTimeNowFunc(timeFunc)

	id := s.Publish("projects/p/topics/t", []byte("hello"), nil)
	s.Wait()

	m := s.Message(id)
	if m == nil {
		t.Fatalf("got nil, want a message")
	}
	if got, want := m.PublishTime, timeFunc(); got != want {
		t.Fatalf("got %v, want %v", got, want)
	}
}

func TestModAck_Race(t *testing.T) {
	ctx := context.Background()
	pclient, sclient, server, cleanup := newFake(ctx, t)
	defer cleanup()

	top := mustCreateTopic(ctx, t, pclient, &pb.Topic{Name: "projects/P/topics/T"})
	sub := mustCreateSubscription(ctx, t, sclient, &pb.Subscription{
		Name:               "projects/P/subscriptions/S",
		Topic:              top.Name,
		AckDeadlineSeconds: 10,
	})

	publish(t, server, pclient, top, []*pb.PubsubMessage{
		{Data: []byte("d1")},
		{Data: []byte("d2")},
		{Data: []byte("d3")},
	})
	msgs := streamingPullN(ctx, t, 3, sclient, sub)
	var ackIDs []string
	for _, m := range msgs {
		ackIDs = append(ackIDs, m.AckId)
	}

	// Try to access m.Modacks while simultaneously calling ModifyAckDeadline
	// so as to try and create a race condition.
	// Invoke ModifyAckDeadline from the server rather than the client
	// to increase replicability of simultaneous data access.
	go func() {
		req := &pb.ModifyAckDeadlineRequest{
			Subscription:       sub.Name,
			AckIds:             ackIDs,
			AckDeadlineSeconds: 0,
		}
		server.GServer.ModifyAckDeadline(ctx, req)
	}()

	sm := server.Messages()
	for _, m := range sm {
		t.Logf("got modacks: %v\n", m.Modacks)
	}
}

func TestUpdateDeadLetterPolicy(t *testing.T) {
	pclient, sclient, _, cleanup := newFake(context.TODO(), t)
	defer cleanup()

	top := mustCreateTopic(context.TODO(), t, pclient, &pb.Topic{Name: "projects/P/topics/T"})
	deadTop := mustCreateTopic(context.TODO(), t, pclient, &pb.Topic{Name: "projects/P/topics/TD"})
	sub := mustCreateSubscription(context.TODO(), t, sclient, &pb.Subscription{
		AckDeadlineSeconds: minAckDeadlineSecs,
		Name:               "projects/P/subscriptions/S",
		Topic:              top.Name,
		DeadLetterPolicy: &pb.DeadLetterPolicy{
			DeadLetterTopic:     deadTop.Name,
			MaxDeliveryAttempts: 5,
		},
	})

	update := &pb.Subscription{
		AckDeadlineSeconds: sub.AckDeadlineSeconds,
		Name:               sub.Name,
		Topic:              top.Name,
		DeadLetterPolicy: &pb.DeadLetterPolicy{
			DeadLetterTopic: deadTop.Name,
			// update max delivery attempts
			MaxDeliveryAttempts: 10,
		},
	}

	updated := mustUpdateSubscription(context.TODO(), t, sclient, &pb.UpdateSubscriptionRequest{
		Subscription: update,
		UpdateMask:   &field_mask.FieldMask{Paths: []string{"dead_letter_policy"}},
	})

	if got, want := updated.DeadLetterPolicy.MaxDeliveryAttempts, update.DeadLetterPolicy.MaxDeliveryAttempts; got != want {
		t.Fatalf("got %v, want %v", got, want)
	}
}

func TestUpdateRetryPolicy(t *testing.T) {
	ctx := context.Background()
	pclient, sclient, _, cleanup := newFake(ctx, t)
	defer cleanup()

	top := mustCreateTopic(ctx, t, pclient, &pb.Topic{Name: "projects/P/topics/T"})
	sub := mustCreateSubscription(ctx, t, sclient, &pb.Subscription{
		AckDeadlineSeconds: minAckDeadlineSecs,
		Name:               "projects/P/subscriptions/S",
		Topic:              top.Name,
		RetryPolicy: &pb.RetryPolicy{
			MinimumBackoff: durationpb.New(10 * time.Second),
			MaximumBackoff: durationpb.New(60 * time.Second),
		},
	})

	update := &pb.Subscription{
		AckDeadlineSeconds: sub.AckDeadlineSeconds,
		Name:               sub.Name,
		Topic:              top.Name,
		RetryPolicy: &pb.RetryPolicy{
			MinimumBackoff: durationpb.New(20 * time.Second),
			MaximumBackoff: durationpb.New(100 * time.Second),
		},
	}

	updated := mustUpdateSubscription(ctx, t, sclient, &pb.UpdateSubscriptionRequest{
		Subscription: update,
		UpdateMask:   &field_mask.FieldMask{Paths: []string{"retry_policy"}},
	})

	if got, want := updated.RetryPolicy, update.RetryPolicy; testutil.Diff(got, want) != "" {
		t.Fatalf("got %v, want %v", got, want)
	}
}

func TestSubscriptionFilter(t *testing.T) {
	ctx := context.Background()
	pclient, sclient, _, cleanup := newFake(ctx, t)
	defer cleanup()

	top := mustCreateTopic(ctx, t, pclient, &pb.Topic{Name: "projects/P/topics/T"})

	// Creating a subscription with invalid filter should return an error.
	_, err := sclient.CreateSubscription(ctx, &pb.Subscription{
		Name:                  "projects/p/subscriptions/s",
		Topic:                 top.Name,
		AckDeadlineSeconds:    30,
		EnableMessageOrdering: true,
		Filter:                "bad filter",
	})
	if err == nil {
		t.Fatal("expected bad filter error, got nil")
	}
	if st := status.Convert(err); st.Code() != codes.InvalidArgument {
		t.Fatalf("got err status: %v, want: %v", st.Code(), codes.InvalidArgument)
	}

	sub := mustCreateSubscription(ctx, t, sclient, &pb.Subscription{
		AckDeadlineSeconds: minAckDeadlineSecs,
		Name:               "projects/P/subscriptions/S",
		Topic:              top.Name,
		Filter:             "NOT attributes:foo",
	})

	update := &pb.Subscription{
		AckDeadlineSeconds: sub.AckDeadlineSeconds,
		Name:               sub.Name,
		Topic:              top.Name,
		Filter:             "NOT attributes:bar",
	}

	updated := mustUpdateSubscription(ctx, t, sclient, &pb.UpdateSubscriptionRequest{
		Subscription: update,
		UpdateMask:   &field_mask.FieldMask{Paths: []string{"filter"}},
	})

	if got, want := updated.Filter, update.Filter; got != want {
		t.Fatalf("got %v, want %v", got, want)
	}

	// Updating a subscription with bad filter should return an error.
	update.Filter = "bad filter"
	updated, err = sclient.UpdateSubscription(ctx, &pb.UpdateSubscriptionRequest{
		Subscription: update,
		UpdateMask:   &field_mask.FieldMask{Paths: []string{"filter"}},
	})
	if err == nil {
		t.Fatal("expected bad filter error, got nil")
	}
	if st := status.Convert(err); st.Code() != codes.InvalidArgument {
		t.Fatalf("got err status: %v, want: %v", st.Code(), codes.InvalidArgument)
	}
}

func TestUpdateEnableExactlyOnceDelivery(t *testing.T) {
	ctx := context.Background()
	pclient, sclient, _, cleanup := newFake(ctx, t)
	defer cleanup()

	top := mustCreateTopic(ctx, t, pclient, &pb.Topic{Name: "projects/P/topics/T"})
	sub := mustCreateSubscription(ctx, t, sclient, &pb.Subscription{
		AckDeadlineSeconds: minAckDeadlineSecs,
		Name:               "projects/P/subscriptions/S",
		Topic:              top.Name,
	})

	update := &pb.Subscription{
		AckDeadlineSeconds:        sub.AckDeadlineSeconds,
		Name:                      sub.Name,
		Topic:                     top.Name,
		EnableExactlyOnceDelivery: true,
	}

	updated := mustUpdateSubscription(ctx, t, sclient, &pb.UpdateSubscriptionRequest{
		Subscription: update,
		UpdateMask:   &field_mask.FieldMask{Paths: []string{"enable_exactly_once_delivery"}},
	})

	if got, want := updated.EnableExactlyOnceDelivery, update.EnableExactlyOnceDelivery; got != want {
		t.Fatalf("got %v, want %v", got, want)
	}
}

// Test Create, Get, List, and Delete methods for schema client.
// Updating a schema is not available at this moment.
func TestSchemaAdminClient(t *testing.T) {
	ctx := context.Background()
	_, _, srv, cleanup := newFake(ctx, t)
	defer cleanup()

	conn, err := grpc.DialContext(ctx, srv.Addr, grpc.WithInsecure())
	if err != nil {
		t.Fatal(err)
	}
	project := "projects/some-project"
	schemaID := "some-schema"
	sclient := pb.NewSchemaServiceClient(conn)
	pbs, err := sclient.CreateSchema(ctx, &pb.CreateSchemaRequest{
		Parent: project,
		Schema: &pb.Schema{
			Type:       pb.Schema_AVRO,
			Definition: "avro-definition",
		},
		SchemaId: schemaID,
	})
	if err != nil {
		t.Errorf("cannot create schema: %v", err)
	}
	pbs2, err := sclient.GetSchema(ctx, &pb.GetSchemaRequest{
		Name: fmt.Sprintf("%s/schemas/%s", project, schemaID),
		View: pb.SchemaView_FULL,
	})
	if err != nil {
		t.Errorf("cannot get schema: %v", err)
	}
	if diff := testutil.Diff(pbs, pbs2); diff != "" {
		t.Errorf("returned schema different, -want, +got, %s", diff)
	}

	resp, err := sclient.ListSchemas(ctx, &pb.ListSchemasRequest{
		Parent: project,
		View:   pb.SchemaView_FULL,
	})
	if err != nil {
		t.Errorf("cannot list schema: %v", err)
	}
	schemas := resp.Schemas
	if len(schemas) != 1 {
		for _, schema := range schemas {
			fmt.Printf("schema: %v\n", schema)
		}
		t.Errorf("got wrong number of schemas in list: %d", len(schemas))
	}

	_, err = sclient.DeleteSchema(ctx, &pb.DeleteSchemaRequest{
		Name: fmt.Sprintf("%s/schemas/%s", project, schemaID),
	})
	if err != nil {
		t.Errorf("cannot delete schema: %v", err)
	}
	if got, want := len(srv.GServer.schemas), 0; got != want {
		t.Fatalf("got %d topics, want %d", got, want)
	}

}

func mustStartStreamingPull(ctx context.Context, t *testing.T, sc pb.SubscriberClient, sub *pb.Subscription) pb.Subscriber_StreamingPullClient {
	spc, err := sc.StreamingPull(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if err := spc.Send(&pb.StreamingPullRequest{
		Subscription:             sub.Name,
		StreamAckDeadlineSeconds: sub.GetAckDeadlineSeconds(),
	}); err != nil {
		t.Fatal(err)
	}
	return spc
}

func pullN(ctx context.Context, t *testing.T, n int, sc pb.SubscriberClient, sub *pb.Subscription) map[string]*pb.ReceivedMessage {
	got := map[string]*pb.ReceivedMessage{}
	for i := 0; len(got) < n; i++ {
		res, err := sc.Pull(ctx, &pb.PullRequest{Subscription: sub.Name, MaxMessages: int32(n - len(got))})
		if err != nil {
			t.Fatal(err)
		}
		for _, m := range res.ReceivedMessages {
			got[m.Message.MessageId] = m
		}
	}
	return got
}

func streamingPullN(ctx context.Context, t *testing.T, n int, sc pb.SubscriberClient, sub *pb.Subscription) map[string]*pb.ReceivedMessage {
	spc := mustStartStreamingPull(ctx, t, sc, sub)
	got := map[string]*pb.ReceivedMessage{}
	for i := 0; i < n; i++ {
		res, err := spc.Recv()
		if err != nil {
			t.Fatal(err)
		}
		for _, m := range res.ReceivedMessages {
			got[m.Message.MessageId] = m
		}
	}
	if err := spc.CloseSend(); err != nil {
		t.Fatal(err)
	}
	res, err := spc.Recv()
	if err != io.EOF {
		t.Fatalf("Recv returned <%v> instead of EOF; res = %v", err, res)
	}
	return got
}

func pubsubMessages(rms map[string]*pb.ReceivedMessage) map[string]*pb.PubsubMessage {
	ms := map[string]*pb.PubsubMessage{}
	for k, rm := range rms {
		ms[k] = rm.Message
	}
	return ms
}

func mustCreateTopic(ctx context.Context, t *testing.T, pc pb.PublisherClient, topic *pb.Topic) *pb.Topic {
	top, err := pc.CreateTopic(ctx, topic)
	if err != nil {
		t.Fatal(err)
	}
	return top
}

func mustUpdateTopic(ctx context.Context, t *testing.T, pc pb.PublisherClient, req *pb.UpdateTopicRequest) *pb.Topic {
	top, err := pc.UpdateTopic(ctx, req)
	if err != nil {
		t.Fatal(err)
	}
	return top
}

func mustCreateSubscription(ctx context.Context, t *testing.T, sc pb.SubscriberClient, sub *pb.Subscription) *pb.Subscription {
	sub, err := sc.CreateSubscription(ctx, sub)
	if err != nil {
		t.Fatal(err)
	}
	return sub
}

func mustUpdateSubscription(ctx context.Context, t *testing.T, sc pb.SubscriberClient, req *pb.UpdateSubscriptionRequest) *pb.Subscription {
	sub, err := sc.UpdateSubscription(ctx, req)
	if err != nil {
		t.Fatal(err)
	}
	return sub
}

// newFake creates a new fake server along  with a publisher and subscriber
// client. Its final return is a cleanup function.
//
// Note: be sure to call cleanup!
func newFake(ctx context.Context, t *testing.T, opts ...ServerReactorOption) (pb.PublisherClient, pb.SubscriberClient, *Server, func()) {
	srv := NewServer(opts...)
	conn, err := grpc.DialContext(ctx, srv.Addr, grpc.WithInsecure())
	if err != nil {
		t.Fatal(err)
	}
	return pb.NewPublisherClient(conn), pb.NewSubscriberClient(conn), srv, func() {
		srv.Close()
		conn.Close()
	}
}

func TestErrorInjection(t *testing.T) {
	testcases := []struct {
		funcName string
		param    interface{}
		code     codes.Code
	}{
		{
			funcName: "CreateTopic",
			code:     codes.Internal,
		},
		{
			funcName: "GetTopic",
			code:     codes.Aborted,
		},
		{
			funcName: "UpdateTopic",
			code:     codes.DeadlineExceeded,
		},
		{
			funcName: "ListTopics",
		},
		{
			funcName: "ListTopicSubscriptions",
		},
		{
			funcName: "DeleteTopic",
		},
		{
			funcName: "CreateSubscription",
		},
		{
			funcName: "GetSubscription",
		},
		{
			funcName: "UpdateSubscription",
			param:    &pb.UpdateSubscriptionRequest{Subscription: &pb.Subscription{}},
		},
		{
			funcName: "ListSubscriptions",
		},
		{
			funcName: "DeleteSubscription",
		},
		{
			funcName: "DetachSubscription",
		},
		{
			funcName: "Publish",
		},
		{
			funcName: "Acknowledge",
		},
		{
			funcName: "ModifyAckDeadline",
		},
		{
			funcName: "Pull",
		},
		{
			funcName: "Seek",
			param:    &pb.SeekRequest{Target: &pb.SeekRequest_Time{Time: timestamppb.Now()}},
		},
	}

	for _, tc := range testcases {
		ctx := context.TODO()
		errMsg := "error-injection-" + tc.funcName
		// set error code to unknown unless specified
		ec := codes.Unknown
		if tc.code != codes.OK {
			ec = tc.code
		}
		opts := []ServerReactorOption{
			WithErrorInjection(tc.funcName, ec, errMsg),
		}
		_, _, server, cleanup := newFake(ctx, t, opts...)
		defer cleanup()

		// We used reflection here to blindly look up the function by name and pass
		// context and a typed nil, as all the functions under test will have such
		// a function signature.
		f := reflect.ValueOf(&server.GServer).MethodByName(tc.funcName)
		if !f.IsValid() {
			t.Fatalf("Method %v Not Found", tc.funcName)
		}
		// If param is provided, use the param, otherwise create a typed nil that matches the parameter type.
		var req reflect.Value
		if tc.param != nil {
			req = reflect.ValueOf(tc.param)
		} else {
			req = reflect.New(f.Type().In(1).Elem())
		}
		ret := reflect.ValueOf(&server.GServer).MethodByName(tc.funcName).Call([]reflect.Value{reflect.ValueOf(ctx), req})

		got := ret[1].Interface().(error)
		if got == nil || status.Code(got) != ec || !strings.Contains(got.Error(), errMsg) {
			t.Errorf("Got error does not contain the right key %v", got)
		}
	}
}

func TestPublishResponse(t *testing.T) {
	ctx := context.Background()
	_, _, srv, cleanup := newFake(ctx, t)
	defer cleanup()

	// By default, autoPublishResponse is true so this should succeed immediately.
	got := srv.Publish("projects/p/topics/t", []byte("msg1"), nil)
	if want := "m0"; got != want {
		t.Fatalf("srv.Publish(): got %v, want %v", got, want)
	}

	// After disabling autoPublishResponse, publish() operations
	// will read from the channel instead of auto generating messages.
	srv.SetAutoPublishResponse(false)

	srv.AddPublishResponse(&pb.PublishResponse{
		MessageIds: []string{"1"},
	}, nil)
	got = srv.Publish("projects/p/topics/t", []byte("msg2"), nil)
	if want := "1"; got != want {
		t.Fatalf("srv.Publish(): got %v, want %v", got, want)
	}

	srv.AddPublishResponse(&pb.PublishResponse{
		MessageIds: []string{"2"},
	}, nil)
	got = srv.Publish("projects/p/topics/t", []byte("msg3"), nil)
	if want := "2"; got != want {
		t.Fatalf("srv.Publish(): got %v, want %v", got, want)
	}

	go func() {
		got = srv.Publish("projects/p/topics/t", []byte("msg4"), nil)
		if want := "3"; got != want {
			fmt.Printf("srv.Publish(): got %v, want %v", got, want)
		}
	}()
	time.Sleep(5 * time.Second)
	srv.AddPublishResponse(&pb.PublishResponse{
		MessageIds: []string{"3"},
	}, nil)
}

func TestTopicRetentionAdmin(t *testing.T) {
	ctx := context.Background()
	pclient, sclient, _, cleanup := newFake(ctx, t)
	defer cleanup()

	initialDur := durationpb.New(10 * time.Hour)
	top := mustCreateTopic(ctx, t, pclient, &pb.Topic{
		Name:                     "projects/P/topics/T",
		MessageRetentionDuration: initialDur,
	})
	got := top.MessageRetentionDuration
	want := initialDur
	if diff := testutil.Diff(got, want); diff != "" {
		t.Errorf("top.MessageRetentionDuration mismatch: %s", diff)
	}

	updateTopic := &pb.Topic{
		Name:                     "projects/P/topics/T",
		MessageRetentionDuration: durationpb.New(5 * time.Hour),
	}
	top2 := mustUpdateTopic(ctx, t, pclient, &pb.UpdateTopicRequest{
		Topic:      updateTopic,
		UpdateMask: &field_mask.FieldMask{Paths: []string{"message_retention_duration"}},
	})
	got = top2.MessageRetentionDuration
	want = updateTopic.MessageRetentionDuration
	if diff := testutil.Diff(got, want); diff != "" {
		t.Errorf("top2.MessageRetentionDuration mismatch: %s", diff)
	}

	sub := mustCreateSubscription(ctx, t, sclient, &pb.Subscription{
		AckDeadlineSeconds: minAckDeadlineSecs,
		Name:               "projects/P/subscriptions/S",
		Topic:              top2.Name,
	})

	got = sub.TopicMessageRetentionDuration
	want = top2.MessageRetentionDuration
	if diff := testutil.Diff(got, want); diff != "" {
		t.Errorf("sub.TopicMessageRetentionDuration mismatch: %s", diff)
	}
}

func TestStreaming_SubscriptionProperties(t *testing.T) {
	ctx := context.Background()
	pc, sc, s, cleanup := newFake(ctx, t)
	defer cleanup()

	top := mustCreateTopic(ctx, t, pc, &pb.Topic{
		Name: "projects/P/topics/T",
	})

	sub := mustCreateSubscription(ctx, t, sc, &pb.Subscription{
		AckDeadlineSeconds:        10,
		Name:                      "projects/P/subscriptions/S",
		Topic:                     top.Name,
		EnableMessageOrdering:     true,
		EnableExactlyOnceDelivery: true,
	})

	spc := mustStartStreamingPull(ctx, t, sc, sub)

	s.Publish("projects/P/topics/T", []byte("hello"), nil)

	res, err := spc.Recv()
	if err != nil {
		t.Fatalf("spc.Recv() got err: %v", err)
	}
	sp := res.GetSubscriptionProperties()
	if !sp.GetExactlyOnceDeliveryEnabled() {
		t.Fatalf("expected exactly once delivery to be enabled in StreamingPullResponse")
	}
	if !sp.GetMessageOrderingEnabled() {
		t.Fatalf("expected message ordering to be enabled in StreamingPullResponse")
	}

	// Close the stream.
	if err := spc.CloseSend(); err != nil {
		t.Fatal(err)
	}
	res, err = spc.Recv()
	if err != io.EOF {
		t.Fatalf("Recv returned <%v> instead of EOF; res = %v", err, res)
	}
}

// Test switching between the various subscription types: push to endpoint, bigquery, cloud storage, and pull.
func TestSubscriptionPushPull(t *testing.T) {
	ctx := context.Background()
	pclient, sclient, _, cleanup := newFake(ctx, t)
	defer cleanup()

	top := mustCreateTopic(ctx, t, pclient, &pb.Topic{
		Name: "projects/P/topics/T",
	})

	// Create a push subscription.
	pc := &pb.PushConfig{
		PushEndpoint: "some-endpoint",
		Wrapper: &pb.PushConfig_PubsubWrapper_{
			PubsubWrapper: &pb.PushConfig_PubsubWrapper{},
		},
	}
	got := mustCreateSubscription(ctx, t, sclient, &pb.Subscription{
		AckDeadlineSeconds: minAckDeadlineSecs,
		Name:               "projects/P/subscriptions/S",
		Topic:              top.Name,
		PushConfig:         pc,
	})

	if diff := testutil.Diff(got.PushConfig, pc); diff != "" {
		t.Errorf("sub.PushConfig mismatch: %s", diff)
	}

	// Update the subscription to write to BigQuery instead.
	updateSub := got
	updateSub.PushConfig = &pb.PushConfig{}
	bqc := &pb.BigQueryConfig{
		Table: "some-table",
	}
	updateSub.BigqueryConfig = bqc
	got = mustUpdateSubscription(ctx, t, sclient, &pb.UpdateSubscriptionRequest{
		Subscription: updateSub,
		UpdateMask:   &field_mask.FieldMask{Paths: []string{"push_config", "bigquery_config"}},
	})
	if diff := testutil.Diff(got.PushConfig, new(pb.PushConfig)); diff != "" {
		t.Errorf("sub.PushConfig should be zero value\n%s", diff)
	}
	want := bqc
	want.State = pb.BigQueryConfig_ACTIVE
	if diff := testutil.Diff(got.BigqueryConfig, want); diff != "" {
		t.Errorf("sub.BigQueryConfig mismatch: %s", diff)
	}

	// Switch back to a pull subscription.
	updateSub.BigqueryConfig = nil
	got = mustUpdateSubscription(ctx, t, sclient, &pb.UpdateSubscriptionRequest{
		Subscription: updateSub,
		UpdateMask:   &field_mask.FieldMask{Paths: []string{"bigquery_config"}},
	})
	if diff := testutil.Diff(got.PushConfig, new(pb.PushConfig)); diff != "" {
		t.Errorf("sub.PushConfig should be zero value\n%s", diff)
	}
	if got.BigqueryConfig != nil {
		t.Errorf("sub.BigqueryConfig should be nil, got %s", got.BigqueryConfig)
	}

	// Update the subscription to write to Cloud Storage.
	csc := &pb.CloudStorageConfig{
		Bucket: "fake-bucket",
	}
	updateSub.CloudStorageConfig = csc
	got = mustUpdateSubscription(ctx, t, sclient, &pb.UpdateSubscriptionRequest{
		Subscription: updateSub,
		UpdateMask:   &field_mask.FieldMask{Paths: []string{"cloud_storage_config"}},
	})
	want2 := csc
	want2.State = pb.CloudStorageConfig_ACTIVE
	if diff := testutil.Diff(got.CloudStorageConfig, want2); diff != "" {
		t.Errorf("sub.CloudStorageConfig mismatch: %s", diff)
	}
}

func TestSubscriptionMessageOrdering(t *testing.T) {
	ctx := context.Background()

	s := NewServer()
	defer s.Close()

	top, err := s.GServer.CreateTopic(ctx, &pb.Topic{Name: "projects/p/topics/t"})
	if err != nil {
		t.Errorf("Failed to init pubsub topic: %v", err)
	}
	sub, err := s.GServer.CreateSubscription(ctx, &pb.Subscription{
		Name:                  "projects/p/subscriptions/s",
		Topic:                 top.Name,
		AckDeadlineSeconds:    30,
		EnableMessageOrdering: true,
	})
	if err != nil {
		t.Errorf("Failed to init pubsub subscription: %v", err)
	}

	const orderingKey = "ordering-key"
	var ids []string
	for i := 0; i < 1000; i++ {
		ids = append(ids, s.PublishOrdered("projects/p/topics/t", []byte("hello"), nil, orderingKey))
	}
	for len(ids) > 0 {
		pull, err := s.GServer.Pull(ctx, &pb.PullRequest{Subscription: sub.Name})
		if err != nil {
			t.Errorf("Failed to pull from server: %v", err)
		}
		for i, msg := range pull.ReceivedMessages {
			if msg.Message.MessageId != ids[i] {
				t.Errorf("want %s, got %s", ids[i], msg.AckId)
			}
			s.GServer.Acknowledge(ctx, &pb.AcknowledgeRequest{Subscription: sub.Name, AckIds: []string{msg.AckId}})
		}
		ids = ids[len(pull.ReceivedMessages):]
	}
}

func TestSubscriptionRetention(t *testing.T) {
	// Check that subscriptions with undelivered messages past the
	// retention deadline do not trigger a panic.

	ctx := context.Background()
	s := NewServer()
	defer s.Close()

	start := time.Now()
	s.SetTimeNowFunc(func() time.Time { return start })

	const topicName = "projects/p/topics/t"
	top, err := s.GServer.CreateTopic(ctx, &pb.Topic{Name: topicName})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.GServer.CreateSubscription(ctx, &pb.Subscription{
		Name:                  "projects/p/subscriptions/s",
		Topic:                 top.Name,
		AckDeadlineSeconds:    30,
		EnableMessageOrdering: true,
	}); err != nil {
		t.Fatal(err)
	}
	s.Publish(topicName, []byte("payload"), nil)

	s.SetTimeNowFunc(func() time.Time { return start.Add(retentionDuration + 1) })
	time.Sleep(1 * time.Second)
}

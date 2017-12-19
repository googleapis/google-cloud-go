// Copyright 2017 Google Inc. All Rights Reserved.
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
	"fmt"
	"testing"

	"cloud.google.com/go/internal/testutil"
	"golang.org/x/net/context"
	pb "google.golang.org/genproto/googleapis/pubsub/v1"
	"google.golang.org/grpc"
)

func TestTopics(t *testing.T) {
	pclient, _, server := newFake(t)
	ctx := context.Background()
	var topics []*pb.Topic
	for i := 1; i < 3; i++ {
		topics = append(topics, mustCreateTopic(t, pclient, &pb.Topic{
			Name:   fmt.Sprintf("projects/P/topics/T%d", i),
			Labels: map[string]string{"num": fmt.Sprintf("%d", i)},
		}))
	}
	if got, want := len(server.gServer.topics), len(topics); got != want {
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
	if got, want := len(server.gServer.topics), 0; got != want {
		t.Fatalf("got %d topics, want %d", got, want)
	}
}

func TestSubscriptions(t *testing.T) {
	pclient, sclient, server := newFake(t)
	ctx := context.Background()
	topic := mustCreateTopic(t, pclient, &pb.Topic{Name: "projects/P/topics/T"})
	var subs []*pb.Subscription
	for i := 0; i < 3; i++ {
		subs = append(subs, mustCreateSubscription(t, sclient, &pb.Subscription{
			Name:               fmt.Sprintf("projects/P/subscriptions/S%d", i),
			Topic:              topic.Name,
			AckDeadlineSeconds: int32(10 * (i + 1)),
		}))
	}

	if got, want := len(server.gServer.subs), len(subs); got != want {
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

	for _, s := range subs {
		if _, err := sclient.DeleteSubscription(ctx, &pb.DeleteSubscriptionRequest{Subscription: s.Name}); err != nil {
			t.Fatal(err)
		}
	}
	if got, want := len(server.gServer.subs), 0; got != want {
		t.Fatalf("got %d subscriptions, want %d", got, want)
	}
}

func TestPublish(t *testing.T) {
	s, err := NewServer()
	if err != nil {
		t.Fatal(err)
	}
	var ids []string
	for i := 0; i < 3; i++ {
		id, err := s.Publish("t", []byte("hello"), nil)
		if err != nil {
			t.Fatal(err)
		}
		ids = append(ids, id)
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

func mustCreateTopic(t *testing.T, pc pb.PublisherClient, topic *pb.Topic) *pb.Topic {
	top, err := pc.CreateTopic(context.Background(), topic)
	if err != nil {
		t.Fatal(err)
	}
	return top
}

func mustCreateSubscription(t *testing.T, sc pb.SubscriberClient, sub *pb.Subscription) *pb.Subscription {
	sub, err := sc.CreateSubscription(context.Background(), sub)
	if err != nil {
		t.Fatal(err)
	}
	return sub
}

func newFake(t *testing.T) (pb.PublisherClient, pb.SubscriberClient, *Server) {
	srv, err := NewServer()
	if err != nil {
		t.Fatal(err)
	}
	conn, err := grpc.Dial(srv.Addr, grpc.WithInsecure())
	if err != nil {
		t.Fatal(err)
	}
	return pb.NewPublisherClient(conn), pb.NewSubscriberClient(conn), srv
}

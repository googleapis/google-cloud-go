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
	"context"
	"fmt"
	"testing"

	"cloud.google.com/go/internal/testutil"

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

func mustCreateTopic(t *testing.T, pc pb.PublisherClient, topic *pb.Topic) *pb.Topic {
	top, err := pc.CreateTopic(context.Background(), topic)
	if err != nil {
		t.Fatal(err)
	}
	return top
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

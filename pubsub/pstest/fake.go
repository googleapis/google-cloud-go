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

// Package pstest provides a fake PubSub service for testing.
package pstest

import (
	"sort"
	"strings"
	"sync"
	"time"

	"cloud.google.com/go/internal/testutil"
	emptypb "github.com/golang/protobuf/ptypes/empty"
	"golang.org/x/net/context"
	pb "google.golang.org/genproto/googleapis/pubsub/v1"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
)

var now = time.Now

type Server struct {
	Addr string // The address that the server is listening on.
	gServer
}

type gServer struct {
	pb.PublisherServer
	pb.SubscriberServer

	mu       sync.Mutex
	topics   map[string]*topic
	subs     map[string]*subscription
	msgs     []*Message // all messages ever published
	msgsByID map[string]*Message
	wg       sync.WaitGroup
	nextID   int
}

// NewServer creates a new fake server running in the current process.
func NewServer() (*Server, error) {
	srv, err := testutil.NewServer()
	if err != nil {
		return nil, err
	}
	s := &Server{
		Addr: srv.Addr,
		gServer: gServer{
			topics:   map[string]*topic{},
			subs:     map[string]*subscription{},
			msgsByID: map[string]*Message{},
		},
	}
	pb.RegisterPublisherServer(srv.Gsrv, &s.gServer)
	pb.RegisterSubscriberServer(srv.Gsrv, &s.gServer)
	srv.Start()
	return s, nil
}

// A Message is a message that was published to the server.
type Message struct {
	ID          string
	Data        []byte
	Attributes  map[string]string
	PublishTime time.Time
	Deliveries  int
	Acks        int

	// protected by server mutex
	deliveries int
	acks       int
}

func (s *gServer) CreateTopic(_ context.Context, t *pb.Topic) (*pb.Topic, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.topics[t.Name] != nil {
		return nil, grpc.Errorf(codes.AlreadyExists, "topic %q", t.Name)
	}
	top := newTopic(t)
	s.topics[t.Name] = top
	return top.proto, nil
}

func (s *gServer) GetTopic(_ context.Context, req *pb.GetTopicRequest) (*pb.Topic, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if t := s.topics[req.Topic]; t != nil {
		return t.proto, nil
	}
	return nil, grpc.Errorf(codes.NotFound, "topic %q", req.Topic)
}

func (s *gServer) UpdateTopic(_ context.Context, req *pb.UpdateTopicRequest) (*pb.Topic, error) {
	return nil, grpc.Errorf(codes.Unimplemented, "unimplemented")
}

func (s *gServer) ListTopics(_ context.Context, req *pb.ListTopicsRequest) (*pb.ListTopicsResponse, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	var names []string
	for n := range s.topics {
		if strings.HasPrefix(n, req.Project) {
			names = append(names, n)
		}
	}
	sort.Strings(names)
	from, to, nextToken, err := testutil.PageBounds(int(req.PageSize), req.PageToken, len(names))
	if err != nil {
		return nil, err
	}
	res := &pb.ListTopicsResponse{NextPageToken: nextToken}
	for i := from; i < to; i++ {
		res.Topics = append(res.Topics, s.topics[names[i]].proto)
	}
	return res, nil
}

func (s *gServer) ListTopicSubscriptions(_ context.Context, req *pb.ListTopicSubscriptionsRequest) (*pb.ListTopicSubscriptionsResponse, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	var names []string
	for name, sub := range s.subs {
		if sub.topic.proto.Name == req.Topic {
			names = append(names, name)
		}
	}
	sort.Strings(names)
	from, to, nextToken, err := testutil.PageBounds(int(req.PageSize), req.PageToken, len(names))
	if err != nil {
		return nil, err
	}
	return &pb.ListTopicSubscriptionsResponse{
		Subscriptions: names[from:to],
		NextPageToken: nextToken,
	}, nil
}

func (s *gServer) DeleteTopic(_ context.Context, req *pb.DeleteTopicRequest) (*emptypb.Empty, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	t := s.topics[req.Topic]
	if t == nil {
		return nil, grpc.Errorf(codes.NotFound, "topic %q", req.Topic)
	}
	t.stop()
	delete(s.topics, req.Topic)
	return &emptypb.Empty{}, nil
}

type topic struct {
	mu    sync.Mutex
	proto *pb.Topic
	subs  map[string]*subscription
}

func newTopic(pt *pb.Topic) *topic {
	return &topic{
		proto: pt,
		subs:  map[string]*subscription{},
	}
}

func (t *topic) stop() {
	for _, sub := range t.subs {
		sub.proto.Topic = "_deleted-topic_"
		sub.stop()
	}
}

func (t *topic) deleteSub(sub *subscription) {
	delete(t.subs, sub.proto.Name)
}

type subscription struct {
	topic      *topic
	mu         *sync.Mutex
	proto      *pb.Subscription
	ackTimeout time.Duration
	done       chan struct{}
}

func (s *subscription) stop() {
	close(s.done)
}

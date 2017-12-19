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
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"

	"cloud.google.com/go/internal/testutil"
	"github.com/golang/protobuf/ptypes"
	durpb "github.com/golang/protobuf/ptypes/duration"
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

// Publish behaves as if the Publish RPC was called with a message with the given
// data and attrs. It returns the ID of the message.
// The topic will be created if it doesn't exist.
func (s *Server) Publish(topic string, data []byte, attrs map[string]string) (string, error) {
	_, _ = s.gServer.CreateTopic(nil, &pb.Topic{Name: topic})
	req := &pb.PublishRequest{
		Topic:    topic,
		Messages: []*pb.PubsubMessage{{Data: data, Attributes: attrs}},
	}
	res, err := s.gServer.Publish(nil, req)
	if err != nil {
		return "", err
	}
	return res.MessageIds[0], nil
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

// Messages returns information about all messages ever published.
func (s *Server) Messages() []*Message {
	s.gServer.mu.Lock()
	defer s.gServer.mu.Unlock()

	var msgs []*Message
	for _, m := range s.msgs {
		m.Deliveries = m.deliveries
		m.Acks = m.acks
		msgs = append(msgs, m)
	}
	return msgs
}

// Message returns the message with the given ID, or nil if no message
// with that ID was published.
func (s *Server) Message(id string) *Message {
	s.gServer.mu.Lock()
	defer s.gServer.mu.Unlock()

	m := s.msgsByID[id]
	if m != nil {
		m.Deliveries = m.deliveries
		m.Acks = m.acks
	}
	return m
}

func (s *Server) Wait() {
	s.gServer.wg.Wait()
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

func (s *gServer) CreateSubscription(_ context.Context, ps *pb.Subscription) (*pb.Subscription, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if ps.Name == "" {
		return nil, grpc.Errorf(codes.InvalidArgument, "missing name")
	}
	if s.subs[ps.Name] != nil {
		return nil, grpc.Errorf(codes.AlreadyExists, "subscription %q", ps.Name)
	}
	if ps.Topic == "" {
		return nil, grpc.Errorf(codes.InvalidArgument, "missing topic")
	}
	top := s.topics[ps.Topic]
	if top == nil {
		return nil, grpc.Errorf(codes.NotFound, "topic %q", ps.Topic)
	}
	if err := checkAckDeadline(ps.AckDeadlineSeconds); err != nil {
		return nil, err
	}
	if ps.MessageRetentionDuration == nil {
		ps.MessageRetentionDuration = defaultMessageRetentionDuration
	}
	if err := checkMRD(ps.MessageRetentionDuration); err != nil {
		return nil, err
	}
	if ps.PushConfig == nil {
		ps.PushConfig = &pb.PushConfig{}
	}

	sub := newSubscription(top, &s.mu, ps)
	top.subs[ps.Name] = sub
	s.subs[ps.Name] = sub
	sub.start(&s.wg)
	return ps, nil
}

func checkAckDeadline(ads int32) error {
	if ads < 10 || ads > 600 {
		// PubSub service returns Unknown.
		return grpc.Errorf(codes.Unknown, "bad ack_deadline_seconds: %d", ads)
	}
	return nil
}

const (
	minMessageRetentionDuration = 10 * time.Minute
	maxMessageRetentionDuration = 168 * time.Hour
)

var defaultMessageRetentionDuration = ptypes.DurationProto(maxMessageRetentionDuration)

func checkMRD(pmrd *durpb.Duration) error {
	mrd, err := ptypes.Duration(pmrd)
	if err != nil || mrd < minMessageRetentionDuration || mrd > maxMessageRetentionDuration {
		return grpc.Errorf(codes.InvalidArgument, "bad message_retention_duration %+v", pmrd)
	}
	return nil
}

func (s *gServer) GetSubscription(_ context.Context, req *pb.GetSubscriptionRequest) (*pb.Subscription, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if sub := s.subs[req.Subscription]; sub != nil {
		return sub.proto, nil
	}
	return nil, grpc.Errorf(codes.NotFound, "subscription %q", req.Subscription)
}

func (s *gServer) UpdateSubscription(_ context.Context, req *pb.UpdateSubscriptionRequest) (*pb.Subscription, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	sub := s.subs[req.Subscription.Name]
	if sub == nil {
		return nil, grpc.Errorf(codes.NotFound, "subscription %q", req.Subscription.Name)
	}

	sub.topic.mu.Lock()
	defer sub.topic.mu.Unlock()
	for _, path := range req.UpdateMask.Paths {
		switch path {
		case "push_config":
			sub.proto.PushConfig = req.Subscription.PushConfig

		case "ack_deadline_seconds":
			a := req.Subscription.AckDeadlineSeconds
			if err := checkAckDeadline(a); err != nil {
				return nil, err
			}
			sub.proto.AckDeadlineSeconds = a

		case "retain_acked_messages":
			sub.proto.RetainAckedMessages = req.Subscription.RetainAckedMessages

		case "message_retention_duration":
			if err := checkMRD(req.Subscription.MessageRetentionDuration); err != nil {
				return nil, err
			}
			sub.proto.MessageRetentionDuration = req.Subscription.MessageRetentionDuration

			// TODO(jba): labels
		default:
			return nil, grpc.Errorf(codes.InvalidArgument, "unknown field name %q", path)
		}
	}
	return sub.proto, nil
}

func (s *gServer) ListSubscriptions(_ context.Context, req *pb.ListSubscriptionsRequest) (*pb.ListSubscriptionsResponse, error) {
	var names []string
	for name := range s.subs {
		if strings.HasPrefix(name, req.Project) {
			names = append(names, name)
		}
	}
	sort.Strings(names)
	from, to, nextToken, err := testutil.PageBounds(int(req.PageSize), req.PageToken, len(names))
	if err != nil {
		return nil, err
	}
	res := &pb.ListSubscriptionsResponse{NextPageToken: nextToken}
	for i := from; i < to; i++ {
		res.Subscriptions = append(res.Subscriptions, s.subs[names[i]].proto)
	}
	return res, nil
}

func (s *gServer) DeleteSubscription(_ context.Context, req *pb.DeleteSubscriptionRequest) (*emptypb.Empty, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	sub := s.subs[req.Subscription]
	if sub == nil {
		return nil, grpc.Errorf(codes.NotFound, "subscription %q", req.Subscription)
	}
	sub.stop()
	delete(s.subs, req.Subscription)
	sub.topic.deleteSub(sub)
	return &emptypb.Empty{}, nil
}

func (s *gServer) Publish(_ context.Context, req *pb.PublishRequest) (*pb.PublishResponse, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if req.Topic == "" {
		return nil, grpc.Errorf(codes.InvalidArgument, "missing topic")
	}
	top := s.topics[req.Topic]
	if top == nil {
		return nil, grpc.Errorf(codes.NotFound, "topic %q", req.Topic)
	}
	var ids []string
	for _, pm := range req.Messages {
		id := fmt.Sprintf("m%d", s.nextID)
		s.nextID++
		pm.MessageId = id
		pubTime := now()
		tsPubTime, err := ptypes.TimestampProto(pubTime)
		if err != nil {
			return nil, grpc.Errorf(codes.Internal, err.Error())
		}
		pm.PublishTime = tsPubTime
		m := &Message{
			ID:          id,
			Data:        pm.Data,
			Attributes:  pm.Attributes,
			PublishTime: pubTime,
		}
		top.publish(pm, m)
		ids = append(ids, id)
		s.msgs = append(s.msgs, m)
		s.msgsByID[id] = m
	}
	return &pb.PublishResponse{MessageIds: ids}, nil
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

func (t *topic) publish(pm *pb.PubsubMessage, m *Message) {
	for _, s := range t.subs {
		s.msgs[pm.MessageId] = &message{
			publishTime: m.PublishTime,
			proto: &pb.ReceivedMessage{
				AckId:   pm.MessageId,
				Message: pm,
			},
			deliveries:  &m.deliveries,
			acks:        &m.acks,
			streamIndex: -1,
		}
	}
}

type subscription struct {
	topic      *topic
	mu         *sync.Mutex
	proto      *pb.Subscription
	ackTimeout time.Duration
	msgs       map[string]*message // unacked messages by message ID
	done       chan struct{}
}

func newSubscription(t *topic, mu *sync.Mutex, ps *pb.Subscription) *subscription {
	return &subscription{
		topic:      t,
		mu:         mu,
		proto:      ps,
		ackTimeout: 10 * time.Second,
		msgs:       map[string]*message{},
		done:       make(chan struct{}),
	}
}

func (s *subscription) start(wg *sync.WaitGroup) {
	wg.Add(1)
	go func() {
		defer wg.Done()
		for {
			select {
			case <-s.done:
				return
			case <-time.After(1 * time.Second):
				s.deliver()
			}
		}
	}()
}

func (s *subscription) stop() {
	close(s.done)
}

func (s *subscription) deliver() {
}

type message struct {
	proto       *pb.ReceivedMessage
	publishTime time.Time
	ackDeadline time.Time
	deliveries  *int
	acks        *int
	streamIndex int // index of stream that currently owns msg, for round-robin delivery
}

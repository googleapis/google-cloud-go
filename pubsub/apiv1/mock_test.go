// Copyright 2016, Google Inc. All rights reserved.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

// AUTO-GENERATED CODE. DO NOT EDIT.

package pubsub

import (
	google_protobuf "github.com/golang/protobuf/ptypes/empty"
	iampb "google.golang.org/genproto/googleapis/iam/v1"
	pubsubpb "google.golang.org/genproto/googleapis/pubsub/v1"
)

import (
	"flag"
	"io"
	"log"
	"net"
	"os"
	"reflect"
	"testing"

	"golang.org/x/net/context"
	"google.golang.org/api/option"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
)

var _ = io.EOF

type mockPublisherServer struct {
	reqs []interface{}

	// If set, all calls return this error.
	err error

	// responses to return if err == nil
	resps []interface{}
}

func (s *mockPublisherServer) CreateTopic(_ context.Context, req *pubsubpb.Topic) (*pubsubpb.Topic, error) {
	s.reqs = append(s.reqs, req)
	if s.err != nil {
		return nil, s.err
	}
	return s.resps[0].(*pubsubpb.Topic), nil
}

func (s *mockPublisherServer) Publish(_ context.Context, req *pubsubpb.PublishRequest) (*pubsubpb.PublishResponse, error) {
	s.reqs = append(s.reqs, req)
	if s.err != nil {
		return nil, s.err
	}
	return s.resps[0].(*pubsubpb.PublishResponse), nil
}

func (s *mockPublisherServer) GetTopic(_ context.Context, req *pubsubpb.GetTopicRequest) (*pubsubpb.Topic, error) {
	s.reqs = append(s.reqs, req)
	if s.err != nil {
		return nil, s.err
	}
	return s.resps[0].(*pubsubpb.Topic), nil
}

func (s *mockPublisherServer) ListTopics(_ context.Context, req *pubsubpb.ListTopicsRequest) (*pubsubpb.ListTopicsResponse, error) {
	s.reqs = append(s.reqs, req)
	if s.err != nil {
		return nil, s.err
	}
	return s.resps[0].(*pubsubpb.ListTopicsResponse), nil
}

func (s *mockPublisherServer) ListTopicSubscriptions(_ context.Context, req *pubsubpb.ListTopicSubscriptionsRequest) (*pubsubpb.ListTopicSubscriptionsResponse, error) {
	s.reqs = append(s.reqs, req)
	if s.err != nil {
		return nil, s.err
	}
	return s.resps[0].(*pubsubpb.ListTopicSubscriptionsResponse), nil
}

func (s *mockPublisherServer) DeleteTopic(_ context.Context, req *pubsubpb.DeleteTopicRequest) (*google_protobuf.Empty, error) {
	s.reqs = append(s.reqs, req)
	if s.err != nil {
		return nil, s.err
	}
	return s.resps[0].(*google_protobuf.Empty), nil
}

type mockIamPolicyServer struct {
	reqs []interface{}

	// If set, all calls return this error.
	err error

	// responses to return if err == nil
	resps []interface{}
}

func (s *mockIamPolicyServer) SetIamPolicy(_ context.Context, req *iampb.SetIamPolicyRequest) (*iampb.Policy, error) {
	s.reqs = append(s.reqs, req)
	if s.err != nil {
		return nil, s.err
	}
	return s.resps[0].(*iampb.Policy), nil
}

func (s *mockIamPolicyServer) GetIamPolicy(_ context.Context, req *iampb.GetIamPolicyRequest) (*iampb.Policy, error) {
	s.reqs = append(s.reqs, req)
	if s.err != nil {
		return nil, s.err
	}
	return s.resps[0].(*iampb.Policy), nil
}

func (s *mockIamPolicyServer) TestIamPermissions(_ context.Context, req *iampb.TestIamPermissionsRequest) (*iampb.TestIamPermissionsResponse, error) {
	s.reqs = append(s.reqs, req)
	if s.err != nil {
		return nil, s.err
	}
	return s.resps[0].(*iampb.TestIamPermissionsResponse), nil
}

type mockSubscriberServer struct {
	reqs []interface{}

	// If set, all calls return this error.
	err error

	// responses to return if err == nil
	resps []interface{}
}

func (s *mockSubscriberServer) CreateSubscription(_ context.Context, req *pubsubpb.Subscription) (*pubsubpb.Subscription, error) {
	s.reqs = append(s.reqs, req)
	if s.err != nil {
		return nil, s.err
	}
	return s.resps[0].(*pubsubpb.Subscription), nil
}

func (s *mockSubscriberServer) GetSubscription(_ context.Context, req *pubsubpb.GetSubscriptionRequest) (*pubsubpb.Subscription, error) {
	s.reqs = append(s.reqs, req)
	if s.err != nil {
		return nil, s.err
	}
	return s.resps[0].(*pubsubpb.Subscription), nil
}

func (s *mockSubscriberServer) ListSubscriptions(_ context.Context, req *pubsubpb.ListSubscriptionsRequest) (*pubsubpb.ListSubscriptionsResponse, error) {
	s.reqs = append(s.reqs, req)
	if s.err != nil {
		return nil, s.err
	}
	return s.resps[0].(*pubsubpb.ListSubscriptionsResponse), nil
}

func (s *mockSubscriberServer) DeleteSubscription(_ context.Context, req *pubsubpb.DeleteSubscriptionRequest) (*google_protobuf.Empty, error) {
	s.reqs = append(s.reqs, req)
	if s.err != nil {
		return nil, s.err
	}
	return s.resps[0].(*google_protobuf.Empty), nil
}

func (s *mockSubscriberServer) ModifyAckDeadline(_ context.Context, req *pubsubpb.ModifyAckDeadlineRequest) (*google_protobuf.Empty, error) {
	s.reqs = append(s.reqs, req)
	if s.err != nil {
		return nil, s.err
	}
	return s.resps[0].(*google_protobuf.Empty), nil
}

func (s *mockSubscriberServer) Acknowledge(_ context.Context, req *pubsubpb.AcknowledgeRequest) (*google_protobuf.Empty, error) {
	s.reqs = append(s.reqs, req)
	if s.err != nil {
		return nil, s.err
	}
	return s.resps[0].(*google_protobuf.Empty), nil
}

func (s *mockSubscriberServer) Pull(_ context.Context, req *pubsubpb.PullRequest) (*pubsubpb.PullResponse, error) {
	s.reqs = append(s.reqs, req)
	if s.err != nil {
		return nil, s.err
	}
	return s.resps[0].(*pubsubpb.PullResponse), nil
}

func (s *mockSubscriberServer) ModifyPushConfig(_ context.Context, req *pubsubpb.ModifyPushConfigRequest) (*google_protobuf.Empty, error) {
	s.reqs = append(s.reqs, req)
	if s.err != nil {
		return nil, s.err
	}
	return s.resps[0].(*google_protobuf.Empty), nil
}

// clientOpt is the option tests should use to connect to the test server.
// It is initialized by TestMain.
var clientOpt option.ClientOption

var (
	mockPublisher  mockPublisherServer
	mockIamPolicy  mockIamPolicyServer
	mockSubscriber mockSubscriberServer
)

func TestMain(m *testing.M) {
	flag.Parse()

	serv := grpc.NewServer()
	pubsubpb.RegisterPublisherServer(serv, &mockPublisher)
	iampb.RegisterIAMPolicyServer(serv, &mockIamPolicy)
	pubsubpb.RegisterSubscriberServer(serv, &mockSubscriber)

	lis, err := net.Listen("tcp", "localhost:0")
	if err != nil {
		log.Fatal(err)
	}
	go serv.Serve(lis)

	conn, err := grpc.Dial(lis.Addr().String(), grpc.WithInsecure())
	if err != nil {
		log.Fatal(err)
	}
	clientOpt = option.WithGRPCConn(conn)

	os.Exit(m.Run())
}

func TestPublisherCreateTopicError(t *testing.T) {
	errCode := codes.Internal
	mockPublisher.err = grpc.Errorf(errCode, "test error")

	c, err := NewPublisherClient(context.Background(), clientOpt)
	if err != nil {
		t.Fatal(err)
	}

	var req *pubsubpb.Topic

	reflect.ValueOf(&req).Elem().Set(reflect.New(reflect.TypeOf(req).Elem()))

	_, err = c.CreateTopic(context.Background(), req)

	if c := grpc.Code(err); c != errCode {
		t.Errorf("got error code %q, want %q", c, errCode)
	}
}
func TestPublisherPublishError(t *testing.T) {
	errCode := codes.Internal
	mockPublisher.err = grpc.Errorf(errCode, "test error")

	c, err := NewPublisherClient(context.Background(), clientOpt)
	if err != nil {
		t.Fatal(err)
	}

	var req *pubsubpb.PublishRequest

	reflect.ValueOf(&req).Elem().Set(reflect.New(reflect.TypeOf(req).Elem()))

	_, err = c.Publish(context.Background(), req)

	if c := grpc.Code(err); c != errCode {
		t.Errorf("got error code %q, want %q", c, errCode)
	}
}
func TestPublisherGetTopicError(t *testing.T) {
	errCode := codes.Internal
	mockPublisher.err = grpc.Errorf(errCode, "test error")

	c, err := NewPublisherClient(context.Background(), clientOpt)
	if err != nil {
		t.Fatal(err)
	}

	var req *pubsubpb.GetTopicRequest

	reflect.ValueOf(&req).Elem().Set(reflect.New(reflect.TypeOf(req).Elem()))

	_, err = c.GetTopic(context.Background(), req)

	if c := grpc.Code(err); c != errCode {
		t.Errorf("got error code %q, want %q", c, errCode)
	}
}
func TestPublisherListTopicsError(t *testing.T) {
	errCode := codes.Internal
	mockPublisher.err = grpc.Errorf(errCode, "test error")

	c, err := NewPublisherClient(context.Background(), clientOpt)
	if err != nil {
		t.Fatal(err)
	}

	var req *pubsubpb.ListTopicsRequest

	reflect.ValueOf(&req).Elem().Set(reflect.New(reflect.TypeOf(req).Elem()))

	_, err = c.ListTopics(context.Background(), req).Next()

	if c := grpc.Code(err); c != errCode {
		t.Errorf("got error code %q, want %q", c, errCode)
	}
}
func TestPublisherListTopicSubscriptionsError(t *testing.T) {
	errCode := codes.Internal
	mockPublisher.err = grpc.Errorf(errCode, "test error")

	c, err := NewPublisherClient(context.Background(), clientOpt)
	if err != nil {
		t.Fatal(err)
	}

	var req *pubsubpb.ListTopicSubscriptionsRequest

	reflect.ValueOf(&req).Elem().Set(reflect.New(reflect.TypeOf(req).Elem()))

	_, err = c.ListTopicSubscriptions(context.Background(), req).Next()

	if c := grpc.Code(err); c != errCode {
		t.Errorf("got error code %q, want %q", c, errCode)
	}
}
func TestPublisherDeleteTopicError(t *testing.T) {
	errCode := codes.Internal
	mockPublisher.err = grpc.Errorf(errCode, "test error")

	c, err := NewPublisherClient(context.Background(), clientOpt)
	if err != nil {
		t.Fatal(err)
	}

	var req *pubsubpb.DeleteTopicRequest

	reflect.ValueOf(&req).Elem().Set(reflect.New(reflect.TypeOf(req).Elem()))

	err = c.DeleteTopic(context.Background(), req)

	if c := grpc.Code(err); c != errCode {
		t.Errorf("got error code %q, want %q", c, errCode)
	}
}
func TestSubscriberCreateSubscriptionError(t *testing.T) {
	errCode := codes.Internal
	mockSubscriber.err = grpc.Errorf(errCode, "test error")

	c, err := NewSubscriberClient(context.Background(), clientOpt)
	if err != nil {
		t.Fatal(err)
	}

	var req *pubsubpb.Subscription

	reflect.ValueOf(&req).Elem().Set(reflect.New(reflect.TypeOf(req).Elem()))

	_, err = c.CreateSubscription(context.Background(), req)

	if c := grpc.Code(err); c != errCode {
		t.Errorf("got error code %q, want %q", c, errCode)
	}
}
func TestSubscriberGetSubscriptionError(t *testing.T) {
	errCode := codes.Internal
	mockSubscriber.err = grpc.Errorf(errCode, "test error")

	c, err := NewSubscriberClient(context.Background(), clientOpt)
	if err != nil {
		t.Fatal(err)
	}

	var req *pubsubpb.GetSubscriptionRequest

	reflect.ValueOf(&req).Elem().Set(reflect.New(reflect.TypeOf(req).Elem()))

	_, err = c.GetSubscription(context.Background(), req)

	if c := grpc.Code(err); c != errCode {
		t.Errorf("got error code %q, want %q", c, errCode)
	}
}
func TestSubscriberListSubscriptionsError(t *testing.T) {
	errCode := codes.Internal
	mockSubscriber.err = grpc.Errorf(errCode, "test error")

	c, err := NewSubscriberClient(context.Background(), clientOpt)
	if err != nil {
		t.Fatal(err)
	}

	var req *pubsubpb.ListSubscriptionsRequest

	reflect.ValueOf(&req).Elem().Set(reflect.New(reflect.TypeOf(req).Elem()))

	_, err = c.ListSubscriptions(context.Background(), req).Next()

	if c := grpc.Code(err); c != errCode {
		t.Errorf("got error code %q, want %q", c, errCode)
	}
}
func TestSubscriberDeleteSubscriptionError(t *testing.T) {
	errCode := codes.Internal
	mockSubscriber.err = grpc.Errorf(errCode, "test error")

	c, err := NewSubscriberClient(context.Background(), clientOpt)
	if err != nil {
		t.Fatal(err)
	}

	var req *pubsubpb.DeleteSubscriptionRequest

	reflect.ValueOf(&req).Elem().Set(reflect.New(reflect.TypeOf(req).Elem()))

	err = c.DeleteSubscription(context.Background(), req)

	if c := grpc.Code(err); c != errCode {
		t.Errorf("got error code %q, want %q", c, errCode)
	}
}
func TestSubscriberModifyAckDeadlineError(t *testing.T) {
	errCode := codes.Internal
	mockSubscriber.err = grpc.Errorf(errCode, "test error")

	c, err := NewSubscriberClient(context.Background(), clientOpt)
	if err != nil {
		t.Fatal(err)
	}

	var req *pubsubpb.ModifyAckDeadlineRequest

	reflect.ValueOf(&req).Elem().Set(reflect.New(reflect.TypeOf(req).Elem()))

	err = c.ModifyAckDeadline(context.Background(), req)

	if c := grpc.Code(err); c != errCode {
		t.Errorf("got error code %q, want %q", c, errCode)
	}
}
func TestSubscriberAcknowledgeError(t *testing.T) {
	errCode := codes.Internal
	mockSubscriber.err = grpc.Errorf(errCode, "test error")

	c, err := NewSubscriberClient(context.Background(), clientOpt)
	if err != nil {
		t.Fatal(err)
	}

	var req *pubsubpb.AcknowledgeRequest

	reflect.ValueOf(&req).Elem().Set(reflect.New(reflect.TypeOf(req).Elem()))

	err = c.Acknowledge(context.Background(), req)

	if c := grpc.Code(err); c != errCode {
		t.Errorf("got error code %q, want %q", c, errCode)
	}
}
func TestSubscriberPullError(t *testing.T) {
	errCode := codes.Internal
	mockSubscriber.err = grpc.Errorf(errCode, "test error")

	c, err := NewSubscriberClient(context.Background(), clientOpt)
	if err != nil {
		t.Fatal(err)
	}

	var req *pubsubpb.PullRequest

	reflect.ValueOf(&req).Elem().Set(reflect.New(reflect.TypeOf(req).Elem()))

	_, err = c.Pull(context.Background(), req)

	if c := grpc.Code(err); c != errCode {
		t.Errorf("got error code %q, want %q", c, errCode)
	}
}
func TestSubscriberModifyPushConfigError(t *testing.T) {
	errCode := codes.Internal
	mockSubscriber.err = grpc.Errorf(errCode, "test error")

	c, err := NewSubscriberClient(context.Background(), clientOpt)
	if err != nil {
		t.Fatal(err)
	}

	var req *pubsubpb.ModifyPushConfigRequest

	reflect.ValueOf(&req).Elem().Set(reflect.New(reflect.TypeOf(req).Elem()))

	err = c.ModifyPushConfig(context.Background(), req)

	if c := grpc.Code(err); c != errCode {
		t.Errorf("got error code %q, want %q", c, errCode)
	}
}

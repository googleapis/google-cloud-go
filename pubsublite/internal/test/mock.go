// Copyright 2020 Google LLC
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     https://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and

package test

import (
	"context"
	"io"
	"log"
	"reflect"
	"sync"

	"cloud.google.com/go/internal/testutil"
	"cloud.google.com/go/internal/uid"
	"google.golang.org/api/option"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	emptypb "github.com/golang/protobuf/ptypes/empty"
	pb "google.golang.org/genproto/googleapis/cloud/pubsublite/v1"
)

// MockServer is an in-memory mock implementation of a Pub/Sub Lite service,
// which allows unit tests to inspect requests received by the server and send
// fake responses.
// This is the interface that should be used by tests.
type MockServer interface {
	// OnTestStart must be called at the start of each test to clear any existing
	// state and set the test verifiers.
	OnTestStart(*Verifiers)
	// OnTestEnd should be called at the end of each test to flush the verifiers
	// (i.e. check whether any expected requests were not sent to the server).
	OnTestEnd()
}

// Server is a mock Pub/Sub Lite server that can be used for unit testing.
type Server struct {
	LiteServer MockServer
	gRPCServer *testutil.Server
}

// NewServer creates a new mock Pub/Sub Lite server.
func NewServer() (*Server, error) {
	srv, err := testutil.NewServer()
	if err != nil {
		return nil, err
	}
	liteServer := newMockLiteServer()
	pb.RegisterAdminServiceServer(srv.Gsrv, liteServer)
	pb.RegisterPublisherServiceServer(srv.Gsrv, liteServer)
	pb.RegisterSubscriberServiceServer(srv.Gsrv, liteServer)
	pb.RegisterCursorServiceServer(srv.Gsrv, liteServer)
	pb.RegisterPartitionAssignmentServiceServer(srv.Gsrv, liteServer)
	srv.Start()
	return &Server{LiteServer: liteServer, gRPCServer: srv}, nil
}

// NewServerWithConn creates a new mock Pub/Sub Lite server along with client
// options to connect to it.
func NewServerWithConn() (*Server, []option.ClientOption) {
	testServer, err := NewServer()
	if err != nil {
		log.Fatal(err)
	}
	conn, err := grpc.Dial(testServer.Addr(), grpc.WithInsecure())
	if err != nil {
		log.Fatal(err)
	}
	return testServer, []option.ClientOption{option.WithGRPCConn(conn)}
}

// Addr returns the address that the server is listening on.
func (s *Server) Addr() string {
	return s.gRPCServer.Addr
}

// Close shuts down the server and releases all resources.
func (s *Server) Close() {
	s.gRPCServer.Close()
}

// mockLiteServer implements the MockServer interface.
type mockLiteServer struct {
	pb.AdminServiceServer
	pb.PublisherServiceServer
	pb.SubscriberServiceServer
	pb.CursorServiceServer
	pb.PartitionAssignmentServiceServer

	mu sync.Mutex

	testVerifiers *Verifiers
	testIDs       *uid.Space
	currentTestID string
}

func newMockLiteServer() *mockLiteServer {
	return &mockLiteServer{
		testIDs: uid.NewSpace("mockLiteServer", nil),
	}
}

func (s *mockLiteServer) popGlobalVerifiers(request interface{}) (interface{}, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.testVerifiers == nil {
		return nil, status.Errorf(codes.FailedPrecondition, "mockserver: previous test has ended")
	}
	return s.testVerifiers.GlobalVerifier.Pop(request)
}

func (s *mockLiteServer) popStreamVerifier(key string) (*RPCVerifier, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.testVerifiers == nil {
		return nil, status.Errorf(codes.FailedPrecondition, "mockserver: previous test has ended")
	}
	return s.testVerifiers.streamVerifiers.Pop(key)
}

func (s *mockLiteServer) handleStream(stream grpc.ServerStream, req interface{}, requestType reflect.Type, key string) (err error) {
	testID := s.currentTest()
	if testID == "" {
		return status.Errorf(codes.FailedPrecondition, "mockserver: previous test has ended")
	}
	verifier, err := s.popStreamVerifier(key)
	if err != nil {
		return err
	}

	// Verify initial request.
	retResponse, retErr := verifier.Pop(req)
	var ok bool

	for {
		// See comments for RPCVerifier.Push for valid stream request/response
		// combinations.
		if retErr != nil {
			err = retErr
			break
		}
		if retResponse != nil {
			if err = stream.SendMsg(retResponse); err != nil {
				err = status.Errorf(codes.FailedPrecondition, "mockserver: stream send error: %v", err)
				break
			}
		}

		// Check whether the next response isn't blocked on a request.
		ok, retResponse, retErr = verifier.TryPop()
		if ok {
			continue
		}

		req = reflect.New(requestType).Interface()
		if err = stream.RecvMsg(req); err == io.EOF {
			break
		} else if err != nil {
			err = status.Errorf(codes.FailedPrecondition, "mockserver: stream recv error: %v", err)
			break
		}
		if testID != s.currentTest() {
			err = status.Errorf(codes.FailedPrecondition, "mockserver: previous test has ended")
			break
		}
		retResponse, retErr = verifier.Pop(req)
	}

	// Check whether the stream ended prematurely.
	if testID == s.currentTest() {
		verifier.Flush()
	}
	return
}

// MockServer implementation.

func (s *mockLiteServer) OnTestStart(verifiers *Verifiers) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.currentTestID != "" {
		panic("mockserver is already in use by another test")
	}
	s.currentTestID = s.testIDs.New()
	s.testVerifiers = verifiers
}

func (s *mockLiteServer) OnTestEnd() {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.currentTestID = ""
	if s.testVerifiers != nil {
		s.testVerifiers.flush()
	}
}

func (s *mockLiteServer) currentTest() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.currentTestID
}

// PublisherService implementation.

func (s *mockLiteServer) Publish(stream pb.PublisherService_PublishServer) error {
	req, err := stream.Recv()
	if err != nil {
		return status.Errorf(codes.FailedPrecondition, "mockserver: stream recv error before initial request: %v", err)
	}
	if len(req.GetInitialRequest().GetTopic()) == 0 {
		return status.Errorf(codes.InvalidArgument, "mockserver: received invalid initial publish request: %v", req)
	}

	initReq := req.GetInitialRequest()
	k := keyPartition(publishStreamType, initReq.GetTopic(), int(initReq.GetPartition()))
	return s.handleStream(stream, req, reflect.TypeOf(pb.PublishRequest{}), k)
}

// SubscriberService implementation.

func (s *mockLiteServer) Subscribe(stream pb.SubscriberService_SubscribeServer) error {
	req, err := stream.Recv()
	if err != nil {
		return status.Errorf(codes.FailedPrecondition, "mockserver: stream recv error before initial request: %v", err)
	}
	if len(req.GetInitial().GetSubscription()) == 0 {
		return status.Errorf(codes.InvalidArgument, "mockserver: received invalid initial subscribe request: %v", req)
	}

	initReq := req.GetInitial()
	k := keyPartition(subscribeStreamType, initReq.GetSubscription(), int(initReq.GetPartition()))
	return s.handleStream(stream, req, reflect.TypeOf(pb.SubscribeRequest{}), k)
}

// CursorService implementation.

func (s *mockLiteServer) StreamingCommitCursor(stream pb.CursorService_StreamingCommitCursorServer) error {
	req, err := stream.Recv()
	if err != nil {
		return status.Errorf(codes.FailedPrecondition, "mockserver: stream recv error before initial request: %v", err)
	}
	if len(req.GetInitial().GetSubscription()) == 0 {
		return status.Errorf(codes.InvalidArgument, "mockserver: received invalid initial streaming commit cursor request: %v", req)
	}

	initReq := req.GetInitial()
	k := keyPartition(commitStreamType, initReq.GetSubscription(), int(initReq.GetPartition()))
	return s.handleStream(stream, req, reflect.TypeOf(pb.StreamingCommitCursorRequest{}), k)
}

// PartitionAssignmentService implementation.

func (s *mockLiteServer) AssignPartitions(stream pb.PartitionAssignmentService_AssignPartitionsServer) error {
	req, err := stream.Recv()
	if err != nil {
		return status.Errorf(codes.FailedPrecondition, "mockserver: stream recv error before initial request: %v", err)
	}
	if len(req.GetInitial().GetSubscription()) == 0 {
		return status.Errorf(codes.InvalidArgument, "mockserver: received invalid initial partition assignment request: %v", req)
	}

	k := key(assignmentStreamType, req.GetInitial().GetSubscription())
	return s.handleStream(stream, req, reflect.TypeOf(pb.PartitionAssignmentRequest{}), k)
}

// AdminService implementation.

func (s *mockLiteServer) doTopicResponse(ctx context.Context, req interface{}) (*pb.Topic, error) {
	retResponse, retErr := s.popGlobalVerifiers(req)
	if retErr != nil {
		return nil, retErr
	}
	resp, ok := retResponse.(*pb.Topic)
	if !ok {
		return nil, status.Errorf(codes.FailedPrecondition, "mockserver: invalid response type %v", reflect.TypeOf(retResponse))
	}
	return resp, nil
}

func (s *mockLiteServer) doSubscriptionResponse(ctx context.Context, req interface{}) (*pb.Subscription, error) {
	retResponse, retErr := s.popGlobalVerifiers(req)
	if retErr != nil {
		return nil, retErr
	}
	resp, ok := retResponse.(*pb.Subscription)
	if !ok {
		return nil, status.Errorf(codes.FailedPrecondition, "mockserver: invalid response type %v", reflect.TypeOf(retResponse))
	}
	return resp, nil
}

func (s *mockLiteServer) doEmptyResponse(ctx context.Context, req interface{}) (*emptypb.Empty, error) {
	retResponse, retErr := s.popGlobalVerifiers(req)
	if retErr != nil {
		return nil, retErr
	}
	resp, ok := retResponse.(*emptypb.Empty)
	if !ok {
		return nil, status.Errorf(codes.FailedPrecondition, "mockserver: invalid response type %v", reflect.TypeOf(retResponse))
	}
	return resp, nil
}

func (s *mockLiteServer) CreateTopic(ctx context.Context, req *pb.CreateTopicRequest) (*pb.Topic, error) {
	return s.doTopicResponse(ctx, req)
}

func (s *mockLiteServer) UpdateTopic(ctx context.Context, req *pb.UpdateTopicRequest) (*pb.Topic, error) {
	return s.doTopicResponse(ctx, req)
}

func (s *mockLiteServer) GetTopic(ctx context.Context, req *pb.GetTopicRequest) (*pb.Topic, error) {
	return s.doTopicResponse(ctx, req)
}

func (s *mockLiteServer) GetTopicPartitions(ctx context.Context, req *pb.GetTopicPartitionsRequest) (*pb.TopicPartitions, error) {
	retResponse, retErr := s.popGlobalVerifiers(req)
	if retErr != nil {
		return nil, retErr
	}
	resp, ok := retResponse.(*pb.TopicPartitions)
	if !ok {
		return nil, status.Errorf(codes.FailedPrecondition, "mockserver: invalid response type %v", reflect.TypeOf(retResponse))
	}
	return resp, nil
}

func (s *mockLiteServer) DeleteTopic(ctx context.Context, req *pb.DeleteTopicRequest) (*emptypb.Empty, error) {
	return s.doEmptyResponse(ctx, req)
}

func (s *mockLiteServer) CreateSubscription(ctx context.Context, req *pb.CreateSubscriptionRequest) (*pb.Subscription, error) {
	return s.doSubscriptionResponse(ctx, req)
}

func (s *mockLiteServer) GetSubscription(ctx context.Context, req *pb.GetSubscriptionRequest) (*pb.Subscription, error) {
	return s.doSubscriptionResponse(ctx, req)
}

func (s *mockLiteServer) UpdateSubscription(ctx context.Context, req *pb.UpdateSubscriptionRequest) (*pb.Subscription, error) {
	return s.doSubscriptionResponse(ctx, req)
}

func (s *mockLiteServer) DeleteSubscription(ctx context.Context, req *pb.DeleteSubscriptionRequest) (*emptypb.Empty, error) {
	return s.doEmptyResponse(ctx, req)
}

func (s *mockLiteServer) ListTopics(ctx context.Context, req *pb.ListTopicsRequest) (*pb.ListTopicsResponse, error) {
	retResponse, retErr := s.popGlobalVerifiers(req)
	if retErr != nil {
		return nil, retErr
	}
	resp, ok := retResponse.(*pb.ListTopicsResponse)
	if !ok {
		return nil, status.Errorf(codes.FailedPrecondition, "mockserver: invalid response type %v", reflect.TypeOf(retResponse))
	}
	return resp, nil
}

func (s *mockLiteServer) ListTopicSubscriptions(ctx context.Context, req *pb.ListTopicSubscriptionsRequest) (*pb.ListTopicSubscriptionsResponse, error) {
	retResponse, retErr := s.popGlobalVerifiers(req)
	if retErr != nil {
		return nil, retErr
	}
	resp, ok := retResponse.(*pb.ListTopicSubscriptionsResponse)
	if !ok {
		return nil, status.Errorf(codes.FailedPrecondition, "mockserver: invalid response type %v", reflect.TypeOf(retResponse))
	}
	return resp, nil
}

func (s *mockLiteServer) ListSubscriptions(ctx context.Context, req *pb.ListSubscriptionsRequest) (*pb.ListSubscriptionsResponse, error) {
	retResponse, retErr := s.popGlobalVerifiers(req)
	if retErr != nil {
		return nil, retErr
	}
	resp, ok := retResponse.(*pb.ListSubscriptionsResponse)
	if !ok {
		return nil, status.Errorf(codes.FailedPrecondition, "mockserver: invalid response type %v", reflect.TypeOf(retResponse))
	}
	return resp, nil
}

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
	"fmt"
	"io"
	"reflect"
	"sync"

	"cloud.google.com/go/internal/testutil"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	pb "google.golang.org/genproto/googleapis/cloud/pubsublite/v1"
)

// Server is a mock Pub/Sub Lite server that can be used for unit testing.
type Server struct {
	LiteServer MockServer
	gRPCServer *testutil.Server
}

// MockServer is an in-memory mock implementation of a Pub/Sub Lite service,
// which allows unit tests to inspect requests received by the server and send
// fake responses.
// This is the interface that should be used by tests.
type MockServer interface {
	// OnTestStart must be called at the start of each test to clear any existing
	// state and set the verifier for unary RPCs.
	OnTestStart(globalVerifier *RPCVerifier)
	// OnTestEnd should be called at the end of each test to flush the verifiers
	// (i.e. check whether any expected requests were not sent to the server).
	OnTestEnd()
	// AddPublishStream adds a verifier for a publish stream of a topic partition.
	AddPublishStream(topic string, partition int, streamVerifier *RPCVerifier)
	// AddSubscribeStream adds a verifier for a subscribe stream of a partition.
	AddSubscribeStream(subscription string, partition int, streamVerifier *RPCVerifier)
	// AddCommitStream adds a verifier for a commit stream of a partition.
	AddCommitStream(subscription string, partition int, streamVerifier *RPCVerifier)
	// AddAssignmentStream adds a verifier for a partition assignment stream for a
	// subscription.
	AddAssignmentStream(subscription string, streamVerifier *RPCVerifier)
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

// Addr returns the address that the server is listening on.
func (s *Server) Addr() string {
	return s.gRPCServer.Addr
}

// Close shuts down the server and releases all resources.
func (s *Server) Close() {
	s.gRPCServer.Close()
}

type streamHolder struct {
	stream   grpc.ServerStream
	verifier *RPCVerifier
}

// mockLiteServer implements the MockServer interface.
type mockLiteServer struct {
	pb.AdminServiceServer
	pb.PublisherServiceServer
	pb.SubscriberServiceServer
	pb.CursorServiceServer
	pb.PartitionAssignmentServiceServer

	mu sync.Mutex

	// Global list of verifiers for all unary RPCs. This should be set before the
	// test begins.
	globalVerifier *RPCVerifier

	// Stream verifiers by key.
	publishVerifiers    *keyedStreamVerifiers
	subscribeVerifiers  *keyedStreamVerifiers
	commitVerifiers     *keyedStreamVerifiers
	assignmentVerifiers *keyedStreamVerifiers

	nextStreamID  int
	activeStreams map[int]*streamHolder
	testActive    bool
}

func key(path string, partition int) string {
	return fmt.Sprintf("%s:%d", path, partition)
}

func newMockLiteServer() *mockLiteServer {
	return &mockLiteServer{
		publishVerifiers:    newKeyedStreamVerifiers(),
		subscribeVerifiers:  newKeyedStreamVerifiers(),
		commitVerifiers:     newKeyedStreamVerifiers(),
		assignmentVerifiers: newKeyedStreamVerifiers(),
		activeStreams:       make(map[int]*streamHolder),
	}
}

func (s *mockLiteServer) startStream(stream grpc.ServerStream, verifier *RPCVerifier) (id int) {
	s.mu.Lock()
	defer s.mu.Unlock()

	id = s.nextStreamID
	s.nextStreamID++
	s.activeStreams[id] = &streamHolder{stream: stream, verifier: verifier}
	return
}

func (s *mockLiteServer) endStream(id int) {
	s.mu.Lock()
	defer s.mu.Unlock()

	delete(s.activeStreams, id)
}

func (s *mockLiteServer) popStreamVerifier(key string, keyedVerifiers *keyedStreamVerifiers) (*RPCVerifier, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	return keyedVerifiers.Pop(key)
}

func (s *mockLiteServer) handleStream(stream grpc.ServerStream, req interface{}, requestType reflect.Type, key string, keyedVerifiers *keyedStreamVerifiers) (err error) {
	verifier, err := s.popStreamVerifier(key, keyedVerifiers)
	if err != nil {
		return err
	}

	id := s.startStream(stream, verifier)

	// Verify initial request.
	retResponse, retErr := verifier.Pop(req)
	var ok bool

	for {
		if retErr != nil {
			err = retErr
			break
		}
		if err = stream.SendMsg(retResponse); err != nil {
			err = status.Errorf(codes.FailedPrecondition, "mockserver: stream send error: %v", err)
			break
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
		retResponse, retErr = verifier.Pop(req)
	}

	// Check whether the stream ended prematurely.
	verifier.Flush()
	s.endStream(id)
	return
}

// MockServer implementation.

func (s *mockLiteServer) OnTestStart(globalVerifier *RPCVerifier) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.testActive {
		panic("mockserver is already in use by another test")
	}

	s.testActive = true
	s.globalVerifier = globalVerifier
	s.publishVerifiers.Reset()
	s.subscribeVerifiers.Reset()
	s.commitVerifiers.Reset()
	s.assignmentVerifiers.Reset()
	s.activeStreams = make(map[int]*streamHolder)
}

func (s *mockLiteServer) OnTestEnd() {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.testActive = false
	if s.globalVerifier != nil {
		s.globalVerifier.Flush()
	}

	for _, as := range s.activeStreams {
		as.verifier.Flush()
	}
}

func (s *mockLiteServer) AddPublishStream(topic string, partition int, streamVerifier *RPCVerifier) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.publishVerifiers.Push(key(topic, partition), streamVerifier)
}

func (s *mockLiteServer) AddSubscribeStream(subscription string, partition int, streamVerifier *RPCVerifier) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.subscribeVerifiers.Push(key(subscription, partition), streamVerifier)
}

func (s *mockLiteServer) AddCommitStream(subscription string, partition int, streamVerifier *RPCVerifier) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.commitVerifiers.Push(key(subscription, partition), streamVerifier)
}

func (s *mockLiteServer) AddAssignmentStream(subscription string, streamVerifier *RPCVerifier) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.assignmentVerifiers.Push(subscription, streamVerifier)
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
	k := key(initReq.GetTopic(), int(initReq.GetPartition()))
	return s.handleStream(stream, req, reflect.TypeOf(pb.PublishRequest{}), k, s.publishVerifiers)
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
	k := key(initReq.GetSubscription(), int(initReq.GetPartition()))
	return s.handleStream(stream, req, reflect.TypeOf(pb.SubscribeRequest{}), k, s.subscribeVerifiers)
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
	k := key(initReq.GetSubscription(), int(initReq.GetPartition()))
	return s.handleStream(stream, req, reflect.TypeOf(pb.StreamingCommitCursorRequest{}), k, s.commitVerifiers)
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

	k := req.GetInitial().GetSubscription()
	return s.handleStream(stream, req, reflect.TypeOf(pb.PartitionAssignmentRequest{}), k, s.assignmentVerifiers)
}

// AdminService implementation.

func (s *mockLiteServer) GetTopicPartitions(ctx context.Context, req *pb.GetTopicPartitionsRequest) (*pb.TopicPartitions, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	retResponse, retErr := s.globalVerifier.Pop(req)
	if retErr != nil {
		return nil, retErr
	}
	resp, ok := retResponse.(*pb.TopicPartitions)
	if !ok {
		return nil, status.Errorf(codes.FailedPrecondition, "mockserver: invalid response type %v", reflect.TypeOf(retResponse))
	}
	return resp, nil
}

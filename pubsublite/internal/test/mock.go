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
	LiteServer *MockLiteServer
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

// MockLiteServer is an in-memory mock implementation of a Pub/Sub Lite service,
// which allows unit tests to inspect requests received by the server and send
// fake responses.
type MockLiteServer struct {
	pb.AdminServiceServer
	pb.PublisherServiceServer

	mu sync.Mutex

	// Global list of verifiers for all unary RPCs. This should be set before the
	// test begins.
	globalVerifier *RPCVerifier

	// Publish stream verifiers by topic & partition.
	publishVerifiers map[string]*streamVerifiers

	nextStreamID  int
	activeStreams map[int]*streamHolder
}

func key(path string, partition int) string {
	return fmt.Sprintf("%s:%d", path, partition)
}

func newMockLiteServer() *MockLiteServer {
	return &MockLiteServer{
		publishVerifiers: make(map[string]*streamVerifiers),
		activeStreams:    make(map[int]*streamHolder),
	}
}

// OnTestStart must be called at the start of each test to clear any existing
// state and set the verifier for unary RPCs.
func (s *MockLiteServer) OnTestStart(globalVerifier *RPCVerifier) {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.globalVerifier = globalVerifier
	s.publishVerifiers = make(map[string]*streamVerifiers)
	s.activeStreams = make(map[int]*streamHolder)
}

// OnTestEnd should be called at the end of each test to flush the verifiers
// (i.e. check whether any expected requests were not sent to the server).
func (s *MockLiteServer) OnTestEnd() {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.globalVerifier != nil {
		s.globalVerifier.Flush()
	}

	for _, as := range s.activeStreams {
		as.verifier.Flush()
	}
}

func (s *MockLiteServer) pushStreamVerifier(key string, v *RPCVerifier, verifiers map[string]*streamVerifiers) {
	s.mu.Lock()
	defer s.mu.Unlock()

	sv, ok := verifiers[key]
	if !ok {
		sv = newStreamVerifiers(v.t)
		verifiers[key] = sv
	}
	sv.push(v)
}

func (s *MockLiteServer) popStreamVerifier(key string, verifiers map[string]*streamVerifiers) (*RPCVerifier, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	sv, ok := verifiers[key]
	if !ok {
		return nil, status.Error(codes.FailedPrecondition, "mockserver: unexpected connection with no configured responses")
	}
	return sv.pop()
}

func (s *MockLiteServer) startStream(stream grpc.ServerStream, verifier *RPCVerifier) (id int) {
	s.mu.Lock()
	defer s.mu.Unlock()

	id = s.nextStreamID
	s.nextStreamID++
	s.activeStreams[id] = &streamHolder{stream: stream, verifier: verifier}
	return
}

func (s *MockLiteServer) endStream(id int) {
	s.mu.Lock()
	defer s.mu.Unlock()

	delete(s.activeStreams, id)
}

func (s *MockLiteServer) handleStream(stream grpc.ServerStream, req interface{}, requestType reflect.Type, verifier *RPCVerifier) (err error) {
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
		retResponse, retErr, ok = verifier.TryPop()
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

// AddPublishStream adds a verifier for a publish stream.
func (s *MockLiteServer) AddPublishStream(topic string, partition int, streamVerifier *RPCVerifier) {
	s.pushStreamVerifier(key(topic, partition), streamVerifier, s.publishVerifiers)
}

// PublisherService implementation.

func (s *MockLiteServer) Publish(stream pb.PublisherService_PublishServer) error {
	req, err := stream.Recv()
	if err != nil {
		return status.Errorf(codes.FailedPrecondition, "mockserver: stream recv error before initial request: %v", err)
	}
	if len(req.GetInitialRequest().GetTopic()) == 0 {
		return status.Errorf(codes.InvalidArgument, "mockserver: received invalid initial publish request: %v", req)
	}

	initReq := req.GetInitialRequest()
	verifier, err := s.popStreamVerifier(
		key(initReq.GetTopic(), int(initReq.GetPartition())),
		s.publishVerifiers)
	if err != nil {
		return err
	}
	return s.handleStream(stream, req, reflect.TypeOf(pb.PublishRequest{}), verifier)
}

// AdminService implementation.

func (s *MockLiteServer) GetTopicPartitions(ctx context.Context, req *pb.GetTopicPartitionsRequest) (*pb.TopicPartitions, error) {
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

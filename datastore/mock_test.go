// Copyright 2022 Google LLC
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

package datastore

// Simple mock server for validating service requests.
//
// This mockServer follows the paradigm set here:
// https://github.com/googleapis/google-cloud-go/blob/main/firestore/mock_test.go
//
// You must add new methods to this server when testing additional

import (
	"context"
	"fmt"
	"reflect"
	"testing"

	"cloud.google.com/go/internal/testutil"
	"google.golang.org/api/option"
	pb "google.golang.org/genproto/googleapis/datastore/v1"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/protobuf/encoding/prototext"
	"google.golang.org/protobuf/proto"
)

type mockServer struct {
	pb.DatastoreServer

	Addr     string
	reqItems []reqItem
	resps    []interface{}
}

type reqItem struct {
	wantReq proto.Message
	adjust  func(gotReq proto.Message)
}

const (
	mockProjectID = "projectID"
)

func newMock(t *testing.T) (_ *Client, _ *mockServer, _ func()) {
	srv, cleanup, err := newMockServer()
	if err != nil {
		t.Fatal(err)
	}
	conn, err := grpc.Dial(srv.Addr, grpc.WithTransportCredentials(insecure.NewCredentials()),
		grpc.WithBlock())
	if err != nil {
		t.Fatal(err)
	}
	client, err := NewClient(context.Background(), mockProjectID, option.WithGRPCConn(conn))
	if err != nil {
		t.Fatal(err)
	}
	return client, srv, func() {
		client.Close()
		conn.Close()
		cleanup()
	}
}

func newMockServer() (_ *mockServer, cleanup func(), _ error) {
	srv, err := testutil.NewServer()
	if err != nil {
		return nil, func() {}, err
	}

	mock := &mockServer{Addr: srv.Addr}
	pb.RegisterDatastoreServer(srv.Gsrv, mock)
	srv.Start()

	return mock, func() {
		srv.Close()
	}, nil
}

// addRPC adds a (request, response) pair to the server's list of expected
// interactions. The server will compare the incoming request with wantReq
// using proto.Equal. The response can be a message or an error.
//
// For the Listen RPC, resp should be a []interface{}, where each element
// is either ListenResponse or an error.
//
// Passing nil for wantReq disables the request check.
func (s *mockServer) addRPC(wantReq proto.Message, resp interface{}) {
	s.addRPCAdjust(wantReq, resp, nil)
}

// addRPCAdjust is like addRPC, but accepts a function that can be used
// to tweak the requests before comparison, for example to adjust for
// randomness.
func (s *mockServer) addRPCAdjust(wantReq proto.Message, resp interface{}, adjust func(proto.Message)) {
	s.reqItems = append(s.reqItems, reqItem{wantReq, adjust})
	s.resps = append(s.resps, resp)
}

// popRPC compares the request with the next expected (request, response) pair.
// It returns the response, or an error if the request doesn't match what
// was expected or there are no expected rpcs.
func (s *mockServer) popRPC(gotReq proto.Message) (interface{}, error) {
	if len(s.reqItems) == 0 {
		return nil, fmt.Errorf("out of RPCs, saw %v", reflect.TypeOf(gotReq))
	}
	ri := s.reqItems[0]
	s.reqItems = s.reqItems[1:]
	if ri.wantReq != nil {
		if ri.adjust != nil {
			ri.adjust(gotReq)
		}

		if !proto.Equal(gotReq, ri.wantReq) {
			return nil, fmt.Errorf("mockServer: bad request\ngot:\n%T\n%s\nwant:\n%T\n%s",
				prototext.Format(ri.wantReq), prototext.Format(gotReq),
				ri.wantReq, ri.wantReq)
		}
	}
	resp := s.resps[0]
	s.resps = s.resps[1:]
	if err, ok := resp.(error); ok {
		return nil, err
	}
	return resp, nil
}

func (s *mockServer) reset() {
	s.reqItems = nil
	s.resps = nil
}

func (s *mockServer) Lookup(ctx context.Context, in *pb.LookupRequest) (*pb.LookupResponse, error) {
	res, err := s.popRPC(in)
	if err != nil {
		return nil, err
	}
	return res.(*pb.LookupResponse), nil
}

func (s *mockServer) Rollback(_ context.Context, in *pb.RollbackRequest) (*pb.RollbackResponse, error) {
	res, err := s.popRPC(in)
	if err != nil {
		return nil, err
	}
	return res.(*pb.RollbackResponse), nil
}

func (s *mockServer) Commit(_ context.Context, in *pb.CommitRequest) (*pb.CommitResponse, error) {
	res, err := s.popRPC(in)
	if err != nil {
		return nil, err
	}
	return res.(*pb.CommitResponse), nil
}

func (s *mockServer) BeginTransaction(ctx context.Context, in *pb.BeginTransactionRequest) (*pb.BeginTransactionResponse, error) {
	res, err := s.popRPC(in)
	if err != nil {
		return nil, err
	}
	return res.(*pb.BeginTransactionResponse), nil
}

func (s *mockServer) RunQuery(ctx context.Context, in *pb.RunQueryRequest) (*pb.RunQueryResponse, error) {
	res, err := s.popRPC(in)
	if err != nil {
		return nil, err
	}
	return res.(*pb.RunQueryResponse), nil
}

func (s *mockServer) RunAggregationQuery(ctx context.Context, in *pb.RunAggregationQueryRequest) (*pb.RunAggregationQueryResponse, error) {
	res, err := s.popRPC(in)
	if err != nil {
		return nil, err
	}
	return res.(*pb.RunAggregationQueryResponse), nil
}

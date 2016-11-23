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

package errorreporting

import (
	clouderrorreportingpb "google.golang.org/genproto/googleapis/devtools/clouderrorreporting/v1beta1"
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

type mockErrorGroupServer struct {
	reqs []interface{}

	// If set, all calls return this error.
	err error

	// responses to return if err == nil
	resps []interface{}
}

func (s *mockErrorGroupServer) GetGroup(_ context.Context, req *clouderrorreportingpb.GetGroupRequest) (*clouderrorreportingpb.ErrorGroup, error) {
	s.reqs = append(s.reqs, req)
	if s.err != nil {
		return nil, s.err
	}
	return s.resps[0].(*clouderrorreportingpb.ErrorGroup), nil
}

func (s *mockErrorGroupServer) UpdateGroup(_ context.Context, req *clouderrorreportingpb.UpdateGroupRequest) (*clouderrorreportingpb.ErrorGroup, error) {
	s.reqs = append(s.reqs, req)
	if s.err != nil {
		return nil, s.err
	}
	return s.resps[0].(*clouderrorreportingpb.ErrorGroup), nil
}

type mockErrorStatsServer struct {
	reqs []interface{}

	// If set, all calls return this error.
	err error

	// responses to return if err == nil
	resps []interface{}
}

func (s *mockErrorStatsServer) ListGroupStats(_ context.Context, req *clouderrorreportingpb.ListGroupStatsRequest) (*clouderrorreportingpb.ListGroupStatsResponse, error) {
	s.reqs = append(s.reqs, req)
	if s.err != nil {
		return nil, s.err
	}
	return s.resps[0].(*clouderrorreportingpb.ListGroupStatsResponse), nil
}

func (s *mockErrorStatsServer) ListEvents(_ context.Context, req *clouderrorreportingpb.ListEventsRequest) (*clouderrorreportingpb.ListEventsResponse, error) {
	s.reqs = append(s.reqs, req)
	if s.err != nil {
		return nil, s.err
	}
	return s.resps[0].(*clouderrorreportingpb.ListEventsResponse), nil
}

func (s *mockErrorStatsServer) DeleteEvents(_ context.Context, req *clouderrorreportingpb.DeleteEventsRequest) (*clouderrorreportingpb.DeleteEventsResponse, error) {
	s.reqs = append(s.reqs, req)
	if s.err != nil {
		return nil, s.err
	}
	return s.resps[0].(*clouderrorreportingpb.DeleteEventsResponse), nil
}

type mockReportErrorsServer struct {
	reqs []interface{}

	// If set, all calls return this error.
	err error

	// responses to return if err == nil
	resps []interface{}
}

func (s *mockReportErrorsServer) ReportErrorEvent(_ context.Context, req *clouderrorreportingpb.ReportErrorEventRequest) (*clouderrorreportingpb.ReportErrorEventResponse, error) {
	s.reqs = append(s.reqs, req)
	if s.err != nil {
		return nil, s.err
	}
	return s.resps[0].(*clouderrorreportingpb.ReportErrorEventResponse), nil
}

// clientOpt is the option tests should use to connect to the test server.
// It is initialized by TestMain.
var clientOpt option.ClientOption

var (
	mockErrorGroup   mockErrorGroupServer
	mockErrorStats   mockErrorStatsServer
	mockReportErrors mockReportErrorsServer
)

func TestMain(m *testing.M) {
	flag.Parse()

	serv := grpc.NewServer()
	clouderrorreportingpb.RegisterErrorGroupServiceServer(serv, &mockErrorGroup)
	clouderrorreportingpb.RegisterErrorStatsServiceServer(serv, &mockErrorStats)
	clouderrorreportingpb.RegisterReportErrorsServiceServer(serv, &mockReportErrors)

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

func TestErrorGroupServiceGetGroupError(t *testing.T) {
	errCode := codes.Internal
	mockErrorGroup.err = grpc.Errorf(errCode, "test error")

	c, err := NewErrorGroupClient(context.Background(), clientOpt)
	if err != nil {
		t.Fatal(err)
	}

	var req *clouderrorreportingpb.GetGroupRequest

	reflect.ValueOf(&req).Elem().Set(reflect.New(reflect.TypeOf(req).Elem()))

	_, err = c.GetGroup(context.Background(), req)

	if c := grpc.Code(err); c != errCode {
		t.Errorf("got error code %q, want %q", c, errCode)
	}
}
func TestErrorGroupServiceUpdateGroupError(t *testing.T) {
	errCode := codes.Internal
	mockErrorGroup.err = grpc.Errorf(errCode, "test error")

	c, err := NewErrorGroupClient(context.Background(), clientOpt)
	if err != nil {
		t.Fatal(err)
	}

	var req *clouderrorreportingpb.UpdateGroupRequest

	reflect.ValueOf(&req).Elem().Set(reflect.New(reflect.TypeOf(req).Elem()))

	_, err = c.UpdateGroup(context.Background(), req)

	if c := grpc.Code(err); c != errCode {
		t.Errorf("got error code %q, want %q", c, errCode)
	}
}
func TestErrorStatsServiceListGroupStatsError(t *testing.T) {
	errCode := codes.Internal
	mockErrorStats.err = grpc.Errorf(errCode, "test error")

	c, err := NewErrorStatsClient(context.Background(), clientOpt)
	if err != nil {
		t.Fatal(err)
	}

	var req *clouderrorreportingpb.ListGroupStatsRequest

	reflect.ValueOf(&req).Elem().Set(reflect.New(reflect.TypeOf(req).Elem()))

	_, err = c.ListGroupStats(context.Background(), req).Next()

	if c := grpc.Code(err); c != errCode {
		t.Errorf("got error code %q, want %q", c, errCode)
	}
}
func TestErrorStatsServiceListEventsError(t *testing.T) {
	errCode := codes.Internal
	mockErrorStats.err = grpc.Errorf(errCode, "test error")

	c, err := NewErrorStatsClient(context.Background(), clientOpt)
	if err != nil {
		t.Fatal(err)
	}

	var req *clouderrorreportingpb.ListEventsRequest

	reflect.ValueOf(&req).Elem().Set(reflect.New(reflect.TypeOf(req).Elem()))

	_, err = c.ListEvents(context.Background(), req).Next()

	if c := grpc.Code(err); c != errCode {
		t.Errorf("got error code %q, want %q", c, errCode)
	}
}
func TestErrorStatsServiceDeleteEventsError(t *testing.T) {
	errCode := codes.Internal
	mockErrorStats.err = grpc.Errorf(errCode, "test error")

	c, err := NewErrorStatsClient(context.Background(), clientOpt)
	if err != nil {
		t.Fatal(err)
	}

	var req *clouderrorreportingpb.DeleteEventsRequest

	reflect.ValueOf(&req).Elem().Set(reflect.New(reflect.TypeOf(req).Elem()))

	_, err = c.DeleteEvents(context.Background(), req)

	if c := grpc.Code(err); c != errCode {
		t.Errorf("got error code %q, want %q", c, errCode)
	}
}
func TestReportErrorsServiceReportErrorEventError(t *testing.T) {
	errCode := codes.Internal
	mockReportErrors.err = grpc.Errorf(errCode, "test error")

	c, err := NewReportErrorsClient(context.Background(), clientOpt)
	if err != nil {
		t.Fatal(err)
	}

	var req *clouderrorreportingpb.ReportErrorEventRequest

	reflect.ValueOf(&req).Elem().Set(reflect.New(reflect.TypeOf(req).Elem()))

	_, err = c.ReportErrorEvent(context.Background(), req)

	if c := grpc.Code(err); c != errCode {
		t.Errorf("got error code %q, want %q", c, errCode)
	}
}

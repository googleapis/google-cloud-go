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

package debugger

import (
	google_protobuf "github.com/golang/protobuf/ptypes/empty"
	clouddebuggerpb "google.golang.org/genproto/googleapis/devtools/clouddebugger/v2"
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

type mockDebugger2Server struct {
	reqs []interface{}

	// If set, all calls return this error.
	err error

	// responses to return if err == nil
	resps []interface{}
}

func (s *mockDebugger2Server) SetBreakpoint(_ context.Context, req *clouddebuggerpb.SetBreakpointRequest) (*clouddebuggerpb.SetBreakpointResponse, error) {
	s.reqs = append(s.reqs, req)
	if s.err != nil {
		return nil, s.err
	}
	return s.resps[0].(*clouddebuggerpb.SetBreakpointResponse), nil
}

func (s *mockDebugger2Server) GetBreakpoint(_ context.Context, req *clouddebuggerpb.GetBreakpointRequest) (*clouddebuggerpb.GetBreakpointResponse, error) {
	s.reqs = append(s.reqs, req)
	if s.err != nil {
		return nil, s.err
	}
	return s.resps[0].(*clouddebuggerpb.GetBreakpointResponse), nil
}

func (s *mockDebugger2Server) DeleteBreakpoint(_ context.Context, req *clouddebuggerpb.DeleteBreakpointRequest) (*google_protobuf.Empty, error) {
	s.reqs = append(s.reqs, req)
	if s.err != nil {
		return nil, s.err
	}
	return s.resps[0].(*google_protobuf.Empty), nil
}

func (s *mockDebugger2Server) ListBreakpoints(_ context.Context, req *clouddebuggerpb.ListBreakpointsRequest) (*clouddebuggerpb.ListBreakpointsResponse, error) {
	s.reqs = append(s.reqs, req)
	if s.err != nil {
		return nil, s.err
	}
	return s.resps[0].(*clouddebuggerpb.ListBreakpointsResponse), nil
}

func (s *mockDebugger2Server) ListDebuggees(_ context.Context, req *clouddebuggerpb.ListDebuggeesRequest) (*clouddebuggerpb.ListDebuggeesResponse, error) {
	s.reqs = append(s.reqs, req)
	if s.err != nil {
		return nil, s.err
	}
	return s.resps[0].(*clouddebuggerpb.ListDebuggeesResponse), nil
}

type mockController2Server struct {
	reqs []interface{}

	// If set, all calls return this error.
	err error

	// responses to return if err == nil
	resps []interface{}
}

func (s *mockController2Server) RegisterDebuggee(_ context.Context, req *clouddebuggerpb.RegisterDebuggeeRequest) (*clouddebuggerpb.RegisterDebuggeeResponse, error) {
	s.reqs = append(s.reqs, req)
	if s.err != nil {
		return nil, s.err
	}
	return s.resps[0].(*clouddebuggerpb.RegisterDebuggeeResponse), nil
}

func (s *mockController2Server) ListActiveBreakpoints(_ context.Context, req *clouddebuggerpb.ListActiveBreakpointsRequest) (*clouddebuggerpb.ListActiveBreakpointsResponse, error) {
	s.reqs = append(s.reqs, req)
	if s.err != nil {
		return nil, s.err
	}
	return s.resps[0].(*clouddebuggerpb.ListActiveBreakpointsResponse), nil
}

func (s *mockController2Server) UpdateActiveBreakpoint(_ context.Context, req *clouddebuggerpb.UpdateActiveBreakpointRequest) (*clouddebuggerpb.UpdateActiveBreakpointResponse, error) {
	s.reqs = append(s.reqs, req)
	if s.err != nil {
		return nil, s.err
	}
	return s.resps[0].(*clouddebuggerpb.UpdateActiveBreakpointResponse), nil
}

// clientOpt is the option tests should use to connect to the test server.
// It is initialized by TestMain.
var clientOpt option.ClientOption

var (
	mockDebugger2   mockDebugger2Server
	mockController2 mockController2Server
)

func TestMain(m *testing.M) {
	flag.Parse()

	serv := grpc.NewServer()
	clouddebuggerpb.RegisterDebugger2Server(serv, &mockDebugger2)
	clouddebuggerpb.RegisterController2Server(serv, &mockController2)

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

func TestDebugger2SetBreakpointError(t *testing.T) {
	errCode := codes.Internal
	mockDebugger2.err = grpc.Errorf(errCode, "test error")

	c, err := NewDebugger2Client(context.Background(), clientOpt)
	if err != nil {
		t.Fatal(err)
	}

	var req *clouddebuggerpb.SetBreakpointRequest

	reflect.ValueOf(&req).Elem().Set(reflect.New(reflect.TypeOf(req).Elem()))

	_, err = c.SetBreakpoint(context.Background(), req)

	if c := grpc.Code(err); c != errCode {
		t.Errorf("got error code %q, want %q", c, errCode)
	}
}
func TestDebugger2GetBreakpointError(t *testing.T) {
	errCode := codes.Internal
	mockDebugger2.err = grpc.Errorf(errCode, "test error")

	c, err := NewDebugger2Client(context.Background(), clientOpt)
	if err != nil {
		t.Fatal(err)
	}

	var req *clouddebuggerpb.GetBreakpointRequest

	reflect.ValueOf(&req).Elem().Set(reflect.New(reflect.TypeOf(req).Elem()))

	_, err = c.GetBreakpoint(context.Background(), req)

	if c := grpc.Code(err); c != errCode {
		t.Errorf("got error code %q, want %q", c, errCode)
	}
}
func TestDebugger2DeleteBreakpointError(t *testing.T) {
	errCode := codes.Internal
	mockDebugger2.err = grpc.Errorf(errCode, "test error")

	c, err := NewDebugger2Client(context.Background(), clientOpt)
	if err != nil {
		t.Fatal(err)
	}

	var req *clouddebuggerpb.DeleteBreakpointRequest

	reflect.ValueOf(&req).Elem().Set(reflect.New(reflect.TypeOf(req).Elem()))

	err = c.DeleteBreakpoint(context.Background(), req)

	if c := grpc.Code(err); c != errCode {
		t.Errorf("got error code %q, want %q", c, errCode)
	}
}
func TestDebugger2ListBreakpointsError(t *testing.T) {
	errCode := codes.Internal
	mockDebugger2.err = grpc.Errorf(errCode, "test error")

	c, err := NewDebugger2Client(context.Background(), clientOpt)
	if err != nil {
		t.Fatal(err)
	}

	var req *clouddebuggerpb.ListBreakpointsRequest

	reflect.ValueOf(&req).Elem().Set(reflect.New(reflect.TypeOf(req).Elem()))

	_, err = c.ListBreakpoints(context.Background(), req)

	if c := grpc.Code(err); c != errCode {
		t.Errorf("got error code %q, want %q", c, errCode)
	}
}
func TestDebugger2ListDebuggeesError(t *testing.T) {
	errCode := codes.Internal
	mockDebugger2.err = grpc.Errorf(errCode, "test error")

	c, err := NewDebugger2Client(context.Background(), clientOpt)
	if err != nil {
		t.Fatal(err)
	}

	var req *clouddebuggerpb.ListDebuggeesRequest

	reflect.ValueOf(&req).Elem().Set(reflect.New(reflect.TypeOf(req).Elem()))

	_, err = c.ListDebuggees(context.Background(), req)

	if c := grpc.Code(err); c != errCode {
		t.Errorf("got error code %q, want %q", c, errCode)
	}
}
func TestController2RegisterDebuggeeError(t *testing.T) {
	errCode := codes.Internal
	mockController2.err = grpc.Errorf(errCode, "test error")

	c, err := NewController2Client(context.Background(), clientOpt)
	if err != nil {
		t.Fatal(err)
	}

	var req *clouddebuggerpb.RegisterDebuggeeRequest

	reflect.ValueOf(&req).Elem().Set(reflect.New(reflect.TypeOf(req).Elem()))

	_, err = c.RegisterDebuggee(context.Background(), req)

	if c := grpc.Code(err); c != errCode {
		t.Errorf("got error code %q, want %q", c, errCode)
	}
}
func TestController2ListActiveBreakpointsError(t *testing.T) {
	errCode := codes.Internal
	mockController2.err = grpc.Errorf(errCode, "test error")

	c, err := NewController2Client(context.Background(), clientOpt)
	if err != nil {
		t.Fatal(err)
	}

	var req *clouddebuggerpb.ListActiveBreakpointsRequest

	reflect.ValueOf(&req).Elem().Set(reflect.New(reflect.TypeOf(req).Elem()))

	_, err = c.ListActiveBreakpoints(context.Background(), req)

	if c := grpc.Code(err); c != errCode {
		t.Errorf("got error code %q, want %q", c, errCode)
	}
}
func TestController2UpdateActiveBreakpointError(t *testing.T) {
	errCode := codes.Internal
	mockController2.err = grpc.Errorf(errCode, "test error")

	c, err := NewController2Client(context.Background(), clientOpt)
	if err != nil {
		t.Fatal(err)
	}

	var req *clouddebuggerpb.UpdateActiveBreakpointRequest

	reflect.ValueOf(&req).Elem().Set(reflect.New(reflect.TypeOf(req).Elem()))

	_, err = c.UpdateActiveBreakpoint(context.Background(), req)

	if c := grpc.Code(err); c != errCode {
		t.Errorf("got error code %q, want %q", c, errCode)
	}
}

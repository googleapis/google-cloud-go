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
	"io"

	"golang.org/x/net/context"
)

var _ = io.EOF

type mockDebugger2 struct {
	reqs []interface{}

	// If set, all calls return this error.
	err error

	// responses to return if err == nil
	resps []interface{}
}

var _ clouddebuggerpb.Debugger2Server = &mockDebugger2{}

func (s *mockDebugger2) SetBreakpoint(_ context.Context, req *clouddebuggerpb.SetBreakpointRequest) (*clouddebuggerpb.SetBreakpointResponse, error) {
	s.reqs = append(s.reqs, req)
	if s.err != nil {
		return nil, s.err
	}
	return s.resps[0].(*clouddebuggerpb.SetBreakpointResponse), nil
}

func (s *mockDebugger2) GetBreakpoint(_ context.Context, req *clouddebuggerpb.GetBreakpointRequest) (*clouddebuggerpb.GetBreakpointResponse, error) {
	s.reqs = append(s.reqs, req)
	if s.err != nil {
		return nil, s.err
	}
	return s.resps[0].(*clouddebuggerpb.GetBreakpointResponse), nil
}

func (s *mockDebugger2) DeleteBreakpoint(_ context.Context, req *clouddebuggerpb.DeleteBreakpointRequest) (*google_protobuf.Empty, error) {
	s.reqs = append(s.reqs, req)
	if s.err != nil {
		return nil, s.err
	}
	return s.resps[0].(*google_protobuf.Empty), nil
}

func (s *mockDebugger2) ListBreakpoints(_ context.Context, req *clouddebuggerpb.ListBreakpointsRequest) (*clouddebuggerpb.ListBreakpointsResponse, error) {
	s.reqs = append(s.reqs, req)
	if s.err != nil {
		return nil, s.err
	}
	return s.resps[0].(*clouddebuggerpb.ListBreakpointsResponse), nil
}

func (s *mockDebugger2) ListDebuggees(_ context.Context, req *clouddebuggerpb.ListDebuggeesRequest) (*clouddebuggerpb.ListDebuggeesResponse, error) {
	s.reqs = append(s.reqs, req)
	if s.err != nil {
		return nil, s.err
	}
	return s.resps[0].(*clouddebuggerpb.ListDebuggeesResponse), nil
}

type mockController2 struct {
	reqs []interface{}

	// If set, all calls return this error.
	err error

	// responses to return if err == nil
	resps []interface{}
}

var _ clouddebuggerpb.Controller2Server = &mockController2{}

func (s *mockController2) RegisterDebuggee(_ context.Context, req *clouddebuggerpb.RegisterDebuggeeRequest) (*clouddebuggerpb.RegisterDebuggeeResponse, error) {
	s.reqs = append(s.reqs, req)
	if s.err != nil {
		return nil, s.err
	}
	return s.resps[0].(*clouddebuggerpb.RegisterDebuggeeResponse), nil
}

func (s *mockController2) ListActiveBreakpoints(_ context.Context, req *clouddebuggerpb.ListActiveBreakpointsRequest) (*clouddebuggerpb.ListActiveBreakpointsResponse, error) {
	s.reqs = append(s.reqs, req)
	if s.err != nil {
		return nil, s.err
	}
	return s.resps[0].(*clouddebuggerpb.ListActiveBreakpointsResponse), nil
}

func (s *mockController2) UpdateActiveBreakpoint(_ context.Context, req *clouddebuggerpb.UpdateActiveBreakpointRequest) (*clouddebuggerpb.UpdateActiveBreakpointResponse, error) {
	s.reqs = append(s.reqs, req)
	if s.err != nil {
		return nil, s.err
	}
	return s.resps[0].(*clouddebuggerpb.UpdateActiveBreakpointResponse), nil
}

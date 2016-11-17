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
	"io"

	"golang.org/x/net/context"
)

var _ = io.EOF

type mockErrorGroupService struct {
	reqs []interface{}

	// If set, all calls return this error.
	err error

	// responses to return if err == nil
	resps []interface{}
}

var _ clouderrorreportingpb.ErrorGroupServiceServer = &mockErrorGroupService{}

func (s *mockErrorGroupService) GetGroup(_ context.Context, req *clouderrorreportingpb.GetGroupRequest) (*clouderrorreportingpb.ErrorGroup, error) {
	s.reqs = append(s.reqs, req)
	if s.err != nil {
		return nil, s.err
	}
	return s.resps[0].(*clouderrorreportingpb.ErrorGroup), nil
}

func (s *mockErrorGroupService) UpdateGroup(_ context.Context, req *clouderrorreportingpb.UpdateGroupRequest) (*clouderrorreportingpb.ErrorGroup, error) {
	s.reqs = append(s.reqs, req)
	if s.err != nil {
		return nil, s.err
	}
	return s.resps[0].(*clouderrorreportingpb.ErrorGroup), nil
}

type mockErrorStatsService struct {
	reqs []interface{}

	// If set, all calls return this error.
	err error

	// responses to return if err == nil
	resps []interface{}
}

var _ clouderrorreportingpb.ErrorStatsServiceServer = &mockErrorStatsService{}

func (s *mockErrorStatsService) ListGroupStats(_ context.Context, req *clouderrorreportingpb.ListGroupStatsRequest) (*clouderrorreportingpb.ListGroupStatsResponse, error) {
	s.reqs = append(s.reqs, req)
	if s.err != nil {
		return nil, s.err
	}
	return s.resps[0].(*clouderrorreportingpb.ListGroupStatsResponse), nil
}

func (s *mockErrorStatsService) ListEvents(_ context.Context, req *clouderrorreportingpb.ListEventsRequest) (*clouderrorreportingpb.ListEventsResponse, error) {
	s.reqs = append(s.reqs, req)
	if s.err != nil {
		return nil, s.err
	}
	return s.resps[0].(*clouderrorreportingpb.ListEventsResponse), nil
}

func (s *mockErrorStatsService) DeleteEvents(_ context.Context, req *clouderrorreportingpb.DeleteEventsRequest) (*clouderrorreportingpb.DeleteEventsResponse, error) {
	s.reqs = append(s.reqs, req)
	if s.err != nil {
		return nil, s.err
	}
	return s.resps[0].(*clouderrorreportingpb.DeleteEventsResponse), nil
}

type mockReportErrorsService struct {
	reqs []interface{}

	// If set, all calls return this error.
	err error

	// responses to return if err == nil
	resps []interface{}
}

var _ clouderrorreportingpb.ReportErrorsServiceServer = &mockReportErrorsService{}

func (s *mockReportErrorsService) ReportErrorEvent(_ context.Context, req *clouderrorreportingpb.ReportErrorEventRequest) (*clouderrorreportingpb.ReportErrorEventResponse, error) {
	s.reqs = append(s.reqs, req)
	if s.err != nil {
		return nil, s.err
	}
	return s.resps[0].(*clouderrorreportingpb.ReportErrorEventResponse), nil
}

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

package monitoring

import (
	google_protobuf "github.com/golang/protobuf/ptypes/empty"
	metricpb "google.golang.org/genproto/googleapis/api/metric"
	monitoredrespb "google.golang.org/genproto/googleapis/api/monitoredres"
	monitoringpb "google.golang.org/genproto/googleapis/monitoring/v3"
)

import (
	"io"

	"golang.org/x/net/context"
)

var _ = io.EOF

type mockGroupService struct {
	reqs []interface{}

	// If set, all calls return this error.
	err error

	// responses to return if err == nil
	resps []interface{}
}

var _ monitoringpb.GroupServiceServer = &mockGroupService{}

func (s *mockGroupService) ListGroups(_ context.Context, req *monitoringpb.ListGroupsRequest) (*monitoringpb.ListGroupsResponse, error) {
	s.reqs = append(s.reqs, req)
	if s.err != nil {
		return nil, s.err
	}
	return s.resps[0].(*monitoringpb.ListGroupsResponse), nil
}

func (s *mockGroupService) GetGroup(_ context.Context, req *monitoringpb.GetGroupRequest) (*monitoringpb.Group, error) {
	s.reqs = append(s.reqs, req)
	if s.err != nil {
		return nil, s.err
	}
	return s.resps[0].(*monitoringpb.Group), nil
}

func (s *mockGroupService) CreateGroup(_ context.Context, req *monitoringpb.CreateGroupRequest) (*monitoringpb.Group, error) {
	s.reqs = append(s.reqs, req)
	if s.err != nil {
		return nil, s.err
	}
	return s.resps[0].(*monitoringpb.Group), nil
}

func (s *mockGroupService) UpdateGroup(_ context.Context, req *monitoringpb.UpdateGroupRequest) (*monitoringpb.Group, error) {
	s.reqs = append(s.reqs, req)
	if s.err != nil {
		return nil, s.err
	}
	return s.resps[0].(*monitoringpb.Group), nil
}

func (s *mockGroupService) DeleteGroup(_ context.Context, req *monitoringpb.DeleteGroupRequest) (*google_protobuf.Empty, error) {
	s.reqs = append(s.reqs, req)
	if s.err != nil {
		return nil, s.err
	}
	return s.resps[0].(*google_protobuf.Empty), nil
}

func (s *mockGroupService) ListGroupMembers(_ context.Context, req *monitoringpb.ListGroupMembersRequest) (*monitoringpb.ListGroupMembersResponse, error) {
	s.reqs = append(s.reqs, req)
	if s.err != nil {
		return nil, s.err
	}
	return s.resps[0].(*monitoringpb.ListGroupMembersResponse), nil
}

type mockMetricService struct {
	reqs []interface{}

	// If set, all calls return this error.
	err error

	// responses to return if err == nil
	resps []interface{}
}

var _ monitoringpb.MetricServiceServer = &mockMetricService{}

func (s *mockMetricService) ListMonitoredResourceDescriptors(_ context.Context, req *monitoringpb.ListMonitoredResourceDescriptorsRequest) (*monitoringpb.ListMonitoredResourceDescriptorsResponse, error) {
	s.reqs = append(s.reqs, req)
	if s.err != nil {
		return nil, s.err
	}
	return s.resps[0].(*monitoringpb.ListMonitoredResourceDescriptorsResponse), nil
}

func (s *mockMetricService) GetMonitoredResourceDescriptor(_ context.Context, req *monitoringpb.GetMonitoredResourceDescriptorRequest) (*monitoredrespb.MonitoredResourceDescriptor, error) {
	s.reqs = append(s.reqs, req)
	if s.err != nil {
		return nil, s.err
	}
	return s.resps[0].(*monitoredrespb.MonitoredResourceDescriptor), nil
}

func (s *mockMetricService) ListMetricDescriptors(_ context.Context, req *monitoringpb.ListMetricDescriptorsRequest) (*monitoringpb.ListMetricDescriptorsResponse, error) {
	s.reqs = append(s.reqs, req)
	if s.err != nil {
		return nil, s.err
	}
	return s.resps[0].(*monitoringpb.ListMetricDescriptorsResponse), nil
}

func (s *mockMetricService) GetMetricDescriptor(_ context.Context, req *monitoringpb.GetMetricDescriptorRequest) (*metricpb.MetricDescriptor, error) {
	s.reqs = append(s.reqs, req)
	if s.err != nil {
		return nil, s.err
	}
	return s.resps[0].(*metricpb.MetricDescriptor), nil
}

func (s *mockMetricService) CreateMetricDescriptor(_ context.Context, req *monitoringpb.CreateMetricDescriptorRequest) (*metricpb.MetricDescriptor, error) {
	s.reqs = append(s.reqs, req)
	if s.err != nil {
		return nil, s.err
	}
	return s.resps[0].(*metricpb.MetricDescriptor), nil
}

func (s *mockMetricService) DeleteMetricDescriptor(_ context.Context, req *monitoringpb.DeleteMetricDescriptorRequest) (*google_protobuf.Empty, error) {
	s.reqs = append(s.reqs, req)
	if s.err != nil {
		return nil, s.err
	}
	return s.resps[0].(*google_protobuf.Empty), nil
}

func (s *mockMetricService) ListTimeSeries(_ context.Context, req *monitoringpb.ListTimeSeriesRequest) (*monitoringpb.ListTimeSeriesResponse, error) {
	s.reqs = append(s.reqs, req)
	if s.err != nil {
		return nil, s.err
	}
	return s.resps[0].(*monitoringpb.ListTimeSeriesResponse), nil
}

func (s *mockMetricService) CreateTimeSeries(_ context.Context, req *monitoringpb.CreateTimeSeriesRequest) (*google_protobuf.Empty, error) {
	s.reqs = append(s.reqs, req)
	if s.err != nil {
		return nil, s.err
	}
	return s.resps[0].(*google_protobuf.Empty), nil
}

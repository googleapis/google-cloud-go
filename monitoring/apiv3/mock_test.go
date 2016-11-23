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

type mockGroupServer struct {
	reqs []interface{}

	// If set, all calls return this error.
	err error

	// responses to return if err == nil
	resps []interface{}
}

func (s *mockGroupServer) ListGroups(_ context.Context, req *monitoringpb.ListGroupsRequest) (*monitoringpb.ListGroupsResponse, error) {
	s.reqs = append(s.reqs, req)
	if s.err != nil {
		return nil, s.err
	}
	return s.resps[0].(*monitoringpb.ListGroupsResponse), nil
}

func (s *mockGroupServer) GetGroup(_ context.Context, req *monitoringpb.GetGroupRequest) (*monitoringpb.Group, error) {
	s.reqs = append(s.reqs, req)
	if s.err != nil {
		return nil, s.err
	}
	return s.resps[0].(*monitoringpb.Group), nil
}

func (s *mockGroupServer) CreateGroup(_ context.Context, req *monitoringpb.CreateGroupRequest) (*monitoringpb.Group, error) {
	s.reqs = append(s.reqs, req)
	if s.err != nil {
		return nil, s.err
	}
	return s.resps[0].(*monitoringpb.Group), nil
}

func (s *mockGroupServer) UpdateGroup(_ context.Context, req *monitoringpb.UpdateGroupRequest) (*monitoringpb.Group, error) {
	s.reqs = append(s.reqs, req)
	if s.err != nil {
		return nil, s.err
	}
	return s.resps[0].(*monitoringpb.Group), nil
}

func (s *mockGroupServer) DeleteGroup(_ context.Context, req *monitoringpb.DeleteGroupRequest) (*google_protobuf.Empty, error) {
	s.reqs = append(s.reqs, req)
	if s.err != nil {
		return nil, s.err
	}
	return s.resps[0].(*google_protobuf.Empty), nil
}

func (s *mockGroupServer) ListGroupMembers(_ context.Context, req *monitoringpb.ListGroupMembersRequest) (*monitoringpb.ListGroupMembersResponse, error) {
	s.reqs = append(s.reqs, req)
	if s.err != nil {
		return nil, s.err
	}
	return s.resps[0].(*monitoringpb.ListGroupMembersResponse), nil
}

type mockMetricServer struct {
	reqs []interface{}

	// If set, all calls return this error.
	err error

	// responses to return if err == nil
	resps []interface{}
}

func (s *mockMetricServer) ListMonitoredResourceDescriptors(_ context.Context, req *monitoringpb.ListMonitoredResourceDescriptorsRequest) (*monitoringpb.ListMonitoredResourceDescriptorsResponse, error) {
	s.reqs = append(s.reqs, req)
	if s.err != nil {
		return nil, s.err
	}
	return s.resps[0].(*monitoringpb.ListMonitoredResourceDescriptorsResponse), nil
}

func (s *mockMetricServer) GetMonitoredResourceDescriptor(_ context.Context, req *monitoringpb.GetMonitoredResourceDescriptorRequest) (*monitoredrespb.MonitoredResourceDescriptor, error) {
	s.reqs = append(s.reqs, req)
	if s.err != nil {
		return nil, s.err
	}
	return s.resps[0].(*monitoredrespb.MonitoredResourceDescriptor), nil
}

func (s *mockMetricServer) ListMetricDescriptors(_ context.Context, req *monitoringpb.ListMetricDescriptorsRequest) (*monitoringpb.ListMetricDescriptorsResponse, error) {
	s.reqs = append(s.reqs, req)
	if s.err != nil {
		return nil, s.err
	}
	return s.resps[0].(*monitoringpb.ListMetricDescriptorsResponse), nil
}

func (s *mockMetricServer) GetMetricDescriptor(_ context.Context, req *monitoringpb.GetMetricDescriptorRequest) (*metricpb.MetricDescriptor, error) {
	s.reqs = append(s.reqs, req)
	if s.err != nil {
		return nil, s.err
	}
	return s.resps[0].(*metricpb.MetricDescriptor), nil
}

func (s *mockMetricServer) CreateMetricDescriptor(_ context.Context, req *monitoringpb.CreateMetricDescriptorRequest) (*metricpb.MetricDescriptor, error) {
	s.reqs = append(s.reqs, req)
	if s.err != nil {
		return nil, s.err
	}
	return s.resps[0].(*metricpb.MetricDescriptor), nil
}

func (s *mockMetricServer) DeleteMetricDescriptor(_ context.Context, req *monitoringpb.DeleteMetricDescriptorRequest) (*google_protobuf.Empty, error) {
	s.reqs = append(s.reqs, req)
	if s.err != nil {
		return nil, s.err
	}
	return s.resps[0].(*google_protobuf.Empty), nil
}

func (s *mockMetricServer) ListTimeSeries(_ context.Context, req *monitoringpb.ListTimeSeriesRequest) (*monitoringpb.ListTimeSeriesResponse, error) {
	s.reqs = append(s.reqs, req)
	if s.err != nil {
		return nil, s.err
	}
	return s.resps[0].(*monitoringpb.ListTimeSeriesResponse), nil
}

func (s *mockMetricServer) CreateTimeSeries(_ context.Context, req *monitoringpb.CreateTimeSeriesRequest) (*google_protobuf.Empty, error) {
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
	mockGroup  mockGroupServer
	mockMetric mockMetricServer
)

func TestMain(m *testing.M) {
	flag.Parse()

	serv := grpc.NewServer()
	monitoringpb.RegisterGroupServiceServer(serv, &mockGroup)
	monitoringpb.RegisterMetricServiceServer(serv, &mockMetric)

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

func TestGroupServiceListGroupsError(t *testing.T) {
	errCode := codes.Internal
	mockGroup.err = grpc.Errorf(errCode, "test error")

	c, err := NewGroupClient(context.Background(), clientOpt)
	if err != nil {
		t.Fatal(err)
	}

	var req *monitoringpb.ListGroupsRequest

	reflect.ValueOf(&req).Elem().Set(reflect.New(reflect.TypeOf(req).Elem()))

	_, err = c.ListGroups(context.Background(), req).Next()

	if c := grpc.Code(err); c != errCode {
		t.Errorf("got error code %q, want %q", c, errCode)
	}
}
func TestGroupServiceGetGroupError(t *testing.T) {
	errCode := codes.Internal
	mockGroup.err = grpc.Errorf(errCode, "test error")

	c, err := NewGroupClient(context.Background(), clientOpt)
	if err != nil {
		t.Fatal(err)
	}

	var req *monitoringpb.GetGroupRequest

	reflect.ValueOf(&req).Elem().Set(reflect.New(reflect.TypeOf(req).Elem()))

	_, err = c.GetGroup(context.Background(), req)

	if c := grpc.Code(err); c != errCode {
		t.Errorf("got error code %q, want %q", c, errCode)
	}
}
func TestGroupServiceCreateGroupError(t *testing.T) {
	errCode := codes.Internal
	mockGroup.err = grpc.Errorf(errCode, "test error")

	c, err := NewGroupClient(context.Background(), clientOpt)
	if err != nil {
		t.Fatal(err)
	}

	var req *monitoringpb.CreateGroupRequest

	reflect.ValueOf(&req).Elem().Set(reflect.New(reflect.TypeOf(req).Elem()))

	_, err = c.CreateGroup(context.Background(), req)

	if c := grpc.Code(err); c != errCode {
		t.Errorf("got error code %q, want %q", c, errCode)
	}
}
func TestGroupServiceUpdateGroupError(t *testing.T) {
	errCode := codes.Internal
	mockGroup.err = grpc.Errorf(errCode, "test error")

	c, err := NewGroupClient(context.Background(), clientOpt)
	if err != nil {
		t.Fatal(err)
	}

	var req *monitoringpb.UpdateGroupRequest

	reflect.ValueOf(&req).Elem().Set(reflect.New(reflect.TypeOf(req).Elem()))

	_, err = c.UpdateGroup(context.Background(), req)

	if c := grpc.Code(err); c != errCode {
		t.Errorf("got error code %q, want %q", c, errCode)
	}
}
func TestGroupServiceDeleteGroupError(t *testing.T) {
	errCode := codes.Internal
	mockGroup.err = grpc.Errorf(errCode, "test error")

	c, err := NewGroupClient(context.Background(), clientOpt)
	if err != nil {
		t.Fatal(err)
	}

	var req *monitoringpb.DeleteGroupRequest

	reflect.ValueOf(&req).Elem().Set(reflect.New(reflect.TypeOf(req).Elem()))

	err = c.DeleteGroup(context.Background(), req)

	if c := grpc.Code(err); c != errCode {
		t.Errorf("got error code %q, want %q", c, errCode)
	}
}
func TestGroupServiceListGroupMembersError(t *testing.T) {
	errCode := codes.Internal
	mockGroup.err = grpc.Errorf(errCode, "test error")

	c, err := NewGroupClient(context.Background(), clientOpt)
	if err != nil {
		t.Fatal(err)
	}

	var req *monitoringpb.ListGroupMembersRequest

	reflect.ValueOf(&req).Elem().Set(reflect.New(reflect.TypeOf(req).Elem()))

	_, err = c.ListGroupMembers(context.Background(), req).Next()

	if c := grpc.Code(err); c != errCode {
		t.Errorf("got error code %q, want %q", c, errCode)
	}
}
func TestMetricServiceListMonitoredResourceDescriptorsError(t *testing.T) {
	errCode := codes.Internal
	mockMetric.err = grpc.Errorf(errCode, "test error")

	c, err := NewMetricClient(context.Background(), clientOpt)
	if err != nil {
		t.Fatal(err)
	}

	var req *monitoringpb.ListMonitoredResourceDescriptorsRequest

	reflect.ValueOf(&req).Elem().Set(reflect.New(reflect.TypeOf(req).Elem()))

	_, err = c.ListMonitoredResourceDescriptors(context.Background(), req).Next()

	if c := grpc.Code(err); c != errCode {
		t.Errorf("got error code %q, want %q", c, errCode)
	}
}
func TestMetricServiceGetMonitoredResourceDescriptorError(t *testing.T) {
	errCode := codes.Internal
	mockMetric.err = grpc.Errorf(errCode, "test error")

	c, err := NewMetricClient(context.Background(), clientOpt)
	if err != nil {
		t.Fatal(err)
	}

	var req *monitoringpb.GetMonitoredResourceDescriptorRequest

	reflect.ValueOf(&req).Elem().Set(reflect.New(reflect.TypeOf(req).Elem()))

	_, err = c.GetMonitoredResourceDescriptor(context.Background(), req)

	if c := grpc.Code(err); c != errCode {
		t.Errorf("got error code %q, want %q", c, errCode)
	}
}
func TestMetricServiceListMetricDescriptorsError(t *testing.T) {
	errCode := codes.Internal
	mockMetric.err = grpc.Errorf(errCode, "test error")

	c, err := NewMetricClient(context.Background(), clientOpt)
	if err != nil {
		t.Fatal(err)
	}

	var req *monitoringpb.ListMetricDescriptorsRequest

	reflect.ValueOf(&req).Elem().Set(reflect.New(reflect.TypeOf(req).Elem()))

	_, err = c.ListMetricDescriptors(context.Background(), req).Next()

	if c := grpc.Code(err); c != errCode {
		t.Errorf("got error code %q, want %q", c, errCode)
	}
}
func TestMetricServiceGetMetricDescriptorError(t *testing.T) {
	errCode := codes.Internal
	mockMetric.err = grpc.Errorf(errCode, "test error")

	c, err := NewMetricClient(context.Background(), clientOpt)
	if err != nil {
		t.Fatal(err)
	}

	var req *monitoringpb.GetMetricDescriptorRequest

	reflect.ValueOf(&req).Elem().Set(reflect.New(reflect.TypeOf(req).Elem()))

	_, err = c.GetMetricDescriptor(context.Background(), req)

	if c := grpc.Code(err); c != errCode {
		t.Errorf("got error code %q, want %q", c, errCode)
	}
}
func TestMetricServiceCreateMetricDescriptorError(t *testing.T) {
	errCode := codes.Internal
	mockMetric.err = grpc.Errorf(errCode, "test error")

	c, err := NewMetricClient(context.Background(), clientOpt)
	if err != nil {
		t.Fatal(err)
	}

	var req *monitoringpb.CreateMetricDescriptorRequest

	reflect.ValueOf(&req).Elem().Set(reflect.New(reflect.TypeOf(req).Elem()))

	_, err = c.CreateMetricDescriptor(context.Background(), req)

	if c := grpc.Code(err); c != errCode {
		t.Errorf("got error code %q, want %q", c, errCode)
	}
}
func TestMetricServiceDeleteMetricDescriptorError(t *testing.T) {
	errCode := codes.Internal
	mockMetric.err = grpc.Errorf(errCode, "test error")

	c, err := NewMetricClient(context.Background(), clientOpt)
	if err != nil {
		t.Fatal(err)
	}

	var req *monitoringpb.DeleteMetricDescriptorRequest

	reflect.ValueOf(&req).Elem().Set(reflect.New(reflect.TypeOf(req).Elem()))

	err = c.DeleteMetricDescriptor(context.Background(), req)

	if c := grpc.Code(err); c != errCode {
		t.Errorf("got error code %q, want %q", c, errCode)
	}
}
func TestMetricServiceListTimeSeriesError(t *testing.T) {
	errCode := codes.Internal
	mockMetric.err = grpc.Errorf(errCode, "test error")

	c, err := NewMetricClient(context.Background(), clientOpt)
	if err != nil {
		t.Fatal(err)
	}

	var req *monitoringpb.ListTimeSeriesRequest

	reflect.ValueOf(&req).Elem().Set(reflect.New(reflect.TypeOf(req).Elem()))

	_, err = c.ListTimeSeries(context.Background(), req).Next()

	if c := grpc.Code(err); c != errCode {
		t.Errorf("got error code %q, want %q", c, errCode)
	}
}
func TestMetricServiceCreateTimeSeriesError(t *testing.T) {
	errCode := codes.Internal
	mockMetric.err = grpc.Errorf(errCode, "test error")

	c, err := NewMetricClient(context.Background(), clientOpt)
	if err != nil {
		t.Fatal(err)
	}

	var req *monitoringpb.CreateTimeSeriesRequest

	reflect.ValueOf(&req).Elem().Set(reflect.New(reflect.TypeOf(req).Elem()))

	err = c.CreateTimeSeries(context.Background(), req)

	if c := grpc.Code(err); c != errCode {
		t.Errorf("got error code %q, want %q", c, errCode)
	}
}

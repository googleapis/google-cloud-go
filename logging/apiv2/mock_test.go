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

package logging

import (
	google_protobuf "github.com/golang/protobuf/ptypes/empty"
	loggingpb "google.golang.org/genproto/googleapis/logging/v2"
)

import (
	"io"

	"golang.org/x/net/context"
)

var _ = io.EOF

type mockLoggingServiceV2 struct {
	reqs []interface{}

	// If set, all calls return this error.
	err error

	// responses to return if err == nil
	resps []interface{}
}

var _ loggingpb.LoggingServiceV2Server = &mockLoggingServiceV2{}

func (s *mockLoggingServiceV2) DeleteLog(_ context.Context, req *loggingpb.DeleteLogRequest) (*google_protobuf.Empty, error) {
	s.reqs = append(s.reqs, req)
	if s.err != nil {
		return nil, s.err
	}
	return s.resps[0].(*google_protobuf.Empty), nil
}

func (s *mockLoggingServiceV2) WriteLogEntries(_ context.Context, req *loggingpb.WriteLogEntriesRequest) (*loggingpb.WriteLogEntriesResponse, error) {
	s.reqs = append(s.reqs, req)
	if s.err != nil {
		return nil, s.err
	}
	return s.resps[0].(*loggingpb.WriteLogEntriesResponse), nil
}

func (s *mockLoggingServiceV2) ListLogEntries(_ context.Context, req *loggingpb.ListLogEntriesRequest) (*loggingpb.ListLogEntriesResponse, error) {
	s.reqs = append(s.reqs, req)
	if s.err != nil {
		return nil, s.err
	}
	return s.resps[0].(*loggingpb.ListLogEntriesResponse), nil
}

func (s *mockLoggingServiceV2) ListMonitoredResourceDescriptors(_ context.Context, req *loggingpb.ListMonitoredResourceDescriptorsRequest) (*loggingpb.ListMonitoredResourceDescriptorsResponse, error) {
	s.reqs = append(s.reqs, req)
	if s.err != nil {
		return nil, s.err
	}
	return s.resps[0].(*loggingpb.ListMonitoredResourceDescriptorsResponse), nil
}

type mockConfigServiceV2 struct {
	reqs []interface{}

	// If set, all calls return this error.
	err error

	// responses to return if err == nil
	resps []interface{}
}

var _ loggingpb.ConfigServiceV2Server = &mockConfigServiceV2{}

func (s *mockConfigServiceV2) ListSinks(_ context.Context, req *loggingpb.ListSinksRequest) (*loggingpb.ListSinksResponse, error) {
	s.reqs = append(s.reqs, req)
	if s.err != nil {
		return nil, s.err
	}
	return s.resps[0].(*loggingpb.ListSinksResponse), nil
}

func (s *mockConfigServiceV2) GetSink(_ context.Context, req *loggingpb.GetSinkRequest) (*loggingpb.LogSink, error) {
	s.reqs = append(s.reqs, req)
	if s.err != nil {
		return nil, s.err
	}
	return s.resps[0].(*loggingpb.LogSink), nil
}

func (s *mockConfigServiceV2) CreateSink(_ context.Context, req *loggingpb.CreateSinkRequest) (*loggingpb.LogSink, error) {
	s.reqs = append(s.reqs, req)
	if s.err != nil {
		return nil, s.err
	}
	return s.resps[0].(*loggingpb.LogSink), nil
}

func (s *mockConfigServiceV2) UpdateSink(_ context.Context, req *loggingpb.UpdateSinkRequest) (*loggingpb.LogSink, error) {
	s.reqs = append(s.reqs, req)
	if s.err != nil {
		return nil, s.err
	}
	return s.resps[0].(*loggingpb.LogSink), nil
}

func (s *mockConfigServiceV2) DeleteSink(_ context.Context, req *loggingpb.DeleteSinkRequest) (*google_protobuf.Empty, error) {
	s.reqs = append(s.reqs, req)
	if s.err != nil {
		return nil, s.err
	}
	return s.resps[0].(*google_protobuf.Empty), nil
}

type mockMetricsServiceV2 struct {
	reqs []interface{}

	// If set, all calls return this error.
	err error

	// responses to return if err == nil
	resps []interface{}
}

var _ loggingpb.MetricsServiceV2Server = &mockMetricsServiceV2{}

func (s *mockMetricsServiceV2) ListLogMetrics(_ context.Context, req *loggingpb.ListLogMetricsRequest) (*loggingpb.ListLogMetricsResponse, error) {
	s.reqs = append(s.reqs, req)
	if s.err != nil {
		return nil, s.err
	}
	return s.resps[0].(*loggingpb.ListLogMetricsResponse), nil
}

func (s *mockMetricsServiceV2) GetLogMetric(_ context.Context, req *loggingpb.GetLogMetricRequest) (*loggingpb.LogMetric, error) {
	s.reqs = append(s.reqs, req)
	if s.err != nil {
		return nil, s.err
	}
	return s.resps[0].(*loggingpb.LogMetric), nil
}

func (s *mockMetricsServiceV2) CreateLogMetric(_ context.Context, req *loggingpb.CreateLogMetricRequest) (*loggingpb.LogMetric, error) {
	s.reqs = append(s.reqs, req)
	if s.err != nil {
		return nil, s.err
	}
	return s.resps[0].(*loggingpb.LogMetric), nil
}

func (s *mockMetricsServiceV2) UpdateLogMetric(_ context.Context, req *loggingpb.UpdateLogMetricRequest) (*loggingpb.LogMetric, error) {
	s.reqs = append(s.reqs, req)
	if s.err != nil {
		return nil, s.err
	}
	return s.resps[0].(*loggingpb.LogMetric), nil
}

func (s *mockMetricsServiceV2) DeleteLogMetric(_ context.Context, req *loggingpb.DeleteLogMetricRequest) (*google_protobuf.Empty, error) {
	s.reqs = append(s.reqs, req)
	if s.err != nil {
		return nil, s.err
	}
	return s.resps[0].(*google_protobuf.Empty), nil
}

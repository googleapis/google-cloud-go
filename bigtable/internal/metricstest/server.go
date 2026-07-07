// Copyright 2024 Google LLC
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

// Package metricstest hosts an in-process fake Cloud Monitoring server
// used by both bigtable/internal/metrics/*_test.go and the top-level
// bigtable/metrics_test.go integration tests. Kept in a dedicated
// package so both callers share one implementation instead of
// duplicating ~200 lines of gRPC test scaffolding.
package metricstest

import (
	"context"
	"fmt"
	"net"
	"strings"
	"sync"
	"time"

	"cloud.google.com/go/monitoring/apiv3/v2/monitoringpb"
	metricpb "google.golang.org/genproto/googleapis/api/metric"
	"google.golang.org/grpc"
	"google.golang.org/grpc/keepalive"
	"google.golang.org/grpc/metadata"
	"google.golang.org/protobuf/types/known/emptypb"
)

// MetricsTestServer is an in-process fake implementation of
// google.monitoring.v3.MetricService. Tests use it to capture the
// CreateServiceTimeSeries / CreateMetricDescriptor calls the metrics
// exporter would otherwise send to Cloud Monitoring.
type MetricsTestServer struct {
	lis                         net.Listener
	srv                         *grpc.Server
	Endpoint                    string
	userAgent                   string
	createMetricDescriptorReqs  []*monitoringpb.CreateMetricDescriptorRequest
	createServiceTimeSeriesReqs []*monitoringpb.CreateTimeSeriesRequest
	RetryCount                  int
	mu                          sync.Mutex

	createServiceTimeSeriesReqCount     int
	expectedCreateServiceTimeSeriesReqs int
	timeSeriesReqCh                     chan struct{}
}

// Shutdown gracefully stops the fake server.
func (m *MetricsTestServer) Shutdown() {
	m.srv.GracefulStop()
}

// UserAgent returns (and clears) the most recently observed user-agent
// header from a CreateServiceTimeSeries call.
func (m *MetricsTestServer) UserAgent() string {
	m.mu.Lock()
	defer m.mu.Unlock()
	ua := m.userAgent
	m.userAgent = ""
	return ua
}

// CreateServiceTimeSeriesRequests pops the CreateTimeSeriesRequest
// batches captured so far.
func (m *MetricsTestServer) CreateServiceTimeSeriesRequests() []*monitoringpb.CreateTimeSeriesRequest {
	m.mu.Lock()
	defer m.mu.Unlock()
	reqs := m.createServiceTimeSeriesReqs
	m.createServiceTimeSeriesReqs = nil
	return reqs
}

func (m *MetricsTestServer) appendCreateMetricDescriptorReq(_ context.Context, req *monitoringpb.CreateMetricDescriptorRequest) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.createMetricDescriptorReqs = append(m.createMetricDescriptorReqs, req)
}

func (m *MetricsTestServer) appendCreateServiceTimeSeriesReq(ctx context.Context, req *monitoringpb.CreateTimeSeriesRequest) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.createServiceTimeSeriesReqs = append(m.createServiceTimeSeriesReqs, req)
	if md, ok := metadata.FromIncomingContext(ctx); ok {
		m.userAgent = strings.Join(md.Get("User-Agent"), ";")
	}

	m.createServiceTimeSeriesReqCount++
	if m.expectedCreateServiceTimeSeriesReqs > 0 && m.createServiceTimeSeriesReqCount >= m.expectedCreateServiceTimeSeriesReqs {
		select {
		case m.timeSeriesReqCh <- struct{}{}:
		default:
		}
	}
}

// WaitForRequests blocks until `count` CreateServiceTimeSeries calls
// have arrived, or ctx cancels, or `timeout` elapses.
func (m *MetricsTestServer) WaitForRequests(ctx context.Context, count int, timeout time.Duration) error {
	m.mu.Lock()
	m.expectedCreateServiceTimeSeriesReqs = count
	m.createServiceTimeSeriesReqCount = 0
	m.timeSeriesReqCh = make(chan struct{}, 1)
	currentReqCount := len(m.createServiceTimeSeriesReqs)
	if currentReqCount >= count {
		m.mu.Unlock()
		select {
		case m.timeSeriesReqCh <- struct{}{}:
		default:
		}
	} else {
		m.mu.Unlock()
	}

	select {
	case <-m.timeSeriesReqCh:
		m.mu.Lock()
		m.expectedCreateServiceTimeSeriesReqs = 0
		m.mu.Unlock()
		return nil
	case <-ctx.Done():
		m.mu.Lock()
		m.expectedCreateServiceTimeSeriesReqs = 0
		m.mu.Unlock()
		return ctx.Err()
	case <-time.After(timeout):
		m.mu.Lock()
		m.expectedCreateServiceTimeSeriesReqs = 0
		numReceived := m.createServiceTimeSeriesReqCount
		m.mu.Unlock()
		return fmt.Errorf("timed out waiting for %d requests, received %d", count, numReceived)
	}
}

// Serve blocks serving the gRPC listener; call from a goroutine.
func (m *MetricsTestServer) Serve() error {
	return m.srv.Serve(m.lis)
}

type fakeMetricServiceServer struct {
	monitoringpb.UnimplementedMetricServiceServer
	metricsTestServer *MetricsTestServer
}

func (f *fakeMetricServiceServer) CreateServiceTimeSeries(
	ctx context.Context,
	req *monitoringpb.CreateTimeSeriesRequest,
) (*emptypb.Empty, error) {
	f.metricsTestServer.appendCreateServiceTimeSeriesReq(ctx, req)
	return &emptypb.Empty{}, nil
}

func (f *fakeMetricServiceServer) CreateMetricDescriptor(
	ctx context.Context,
	req *monitoringpb.CreateMetricDescriptorRequest,
) (*metricpb.MetricDescriptor, error) {
	f.metricsTestServer.appendCreateMetricDescriptorReq(ctx, req)
	return &metricpb.MetricDescriptor{}, nil
}

// NewMetricTestServer stands up a fresh in-process fake Cloud Monitoring
// server on 127.0.0.1:<random-free-port>. Call Serve() from a goroutine
// and Shutdown() from a defer.
func NewMetricTestServer() (*MetricsTestServer, error) {
	srv := grpc.NewServer(grpc.KeepaliveParams(keepalive.ServerParameters{Time: 5 * time.Minute}))
	lis, err := net.Listen("tcp", "localhost:0")
	if err != nil {
		return nil, err
	}
	testServer := &MetricsTestServer{
		Endpoint:        lis.Addr().String(),
		lis:             lis,
		srv:             srv,
		timeSeriesReqCh: make(chan struct{}, 1),
	}

	monitoringpb.RegisterMetricServiceServer(
		srv,
		&fakeMetricServiceServer{metricsTestServer: testServer},
	)

	return testServer, nil
}

package spanner

import (
	"context"
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

type MetricsTestServer struct {
	lis                         net.Listener
	srv                         *grpc.Server
	Endpoint                    string
	userAgent                   string
	createMetricDescriptorReqs  []*monitoringpb.CreateMetricDescriptorRequest
	createServiceTimeSeriesReqs []*monitoringpb.CreateTimeSeriesRequest
	RetryCount                  int
	mu                          sync.Mutex
}

func (m *MetricsTestServer) Shutdown() {
	// this will close mts.lis
	m.srv.GracefulStop()
}

// Pops out the UserAgent from the most recent CreateTimeSeriesRequests or CreateServiceTimeSeriesRequests.
func (m *MetricsTestServer) UserAgent() string {
	m.mu.Lock()
	defer m.mu.Unlock()
	ua := m.userAgent
	m.userAgent = ""
	return ua
}

// Pops out the CreateServiceTimeSeriesRequests which the test server has received so far.
func (m *MetricsTestServer) CreateServiceTimeSeriesRequests() []*monitoringpb.CreateTimeSeriesRequest {
	m.mu.Lock()
	defer m.mu.Unlock()
	reqs := m.createServiceTimeSeriesReqs
	m.createServiceTimeSeriesReqs = nil
	return reqs
}

func (m *MetricsTestServer) appendCreateMetricDescriptorReq(ctx context.Context, req *monitoringpb.CreateMetricDescriptorRequest) {
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
}

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

func NewMetricTestServer() (*MetricsTestServer, error) {
	srv := grpc.NewServer(grpc.KeepaliveParams(keepalive.ServerParameters{Time: 5 * time.Minute}))
	lis, err := net.Listen("tcp", "localhost:0")
	if err != nil {
		return nil, err
	}
	testServer := &MetricsTestServer{
		Endpoint: lis.Addr().String(),
		lis:      lis,
		srv:      srv,
	}

	monitoringpb.RegisterMetricServiceServer(
		srv,
		&fakeMetricServiceServer{metricsTestServer: testServer},
	)

	return testServer, nil
}

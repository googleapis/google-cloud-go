// Copyright 2025 Google LLC
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//	http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package internal

import (
	"context"
	"io"
	"net"
	"sync"
	"testing"
	"time"

	btpb "cloud.google.com/go/bigtable/apiv2/bigtablepb"
	"google.golang.org/grpc"
	"google.golang.org/grpc/connectivity"
	"google.golang.org/grpc/credentials/insecure"
	testpb "google.golang.org/grpc/interop/grpc_testing"
	"google.golang.org/grpc/metadata"
)

// fakeService is a mock gRPC server that implements both BenchmarkService and BigtableServer.
type fakeService struct {
	testpb.UnimplementedBenchmarkServiceServer
	btpb.UnimplementedBigtableServer
	mu                      sync.Mutex
	pingCount               int
	callCount               int
	streamSema              chan struct{} // To control stream lifetime
	delay                   time.Duration // To simulate work
	serverErr               error         // Error to return from server
	lastPingAndWarmMetadata metadata.MD   // Stores metadata from the last PingAndWarm call
	pingErr                 error         // Error to return from PingAndWarm
	pingErrMu               sync.Mutex    // Protects pingErr
	streamRecvErr           error         // Error to return from stream.Recv()
	streamSendErr           error         // Error to return from stream.Send()
}

func (s *fakeService) setPingErr(err error) {
	s.pingErrMu.Lock()
	defer s.pingErrMu.Unlock()
	s.pingErr = err
}

func (s *fakeService) setDelay(duration time.Duration) {
	s.pingErrMu.Lock()
	defer s.pingErrMu.Unlock()
	s.delay = duration
}

func (s *fakeService) getDelay() time.Duration {
	s.pingErrMu.Lock()
	defer s.pingErrMu.Unlock()
	return s.delay
}

func (s *fakeService) getPingErr() error {
	s.pingErrMu.Lock()
	defer s.pingErrMu.Unlock()
	return s.pingErr
}

func (s *fakeService) UnaryCall(ctx context.Context, req *testpb.SimpleRequest) (*testpb.SimpleResponse, error) {
	s.mu.Lock()
	s.callCount++
	s.mu.Unlock()
	if s.delay > 0 {
		time.Sleep(s.delay)
	}
	if s.serverErr != nil {
		return nil, s.serverErr
	}
	return &testpb.SimpleResponse{Payload: req.GetPayload()}, nil
}

func (s *fakeService) StreamingCall(stream testpb.BenchmarkService_StreamingCallServer) error {
	s.mu.Lock()
	s.callCount++
	s.mu.Unlock()

	if s.serverErr != nil {
		return s.serverErr
	}

	if s.streamSema != nil {
		<-s.streamSema
	}

	for {
		req, err := stream.Recv()
		if err == io.EOF {
			return nil
		}
		if err != nil {
			return err
		}
		if s.streamRecvErr != nil {
			return s.streamRecvErr
		}

		if err := stream.Send(&testpb.SimpleResponse{Payload: req.GetPayload()}); err != nil {
			return err
		}
		if s.streamSendErr != nil {
			return s.streamSendErr
		}
	}
}

func (s *fakeService) PingAndWarm(ctx context.Context, req *btpb.PingAndWarmRequest) (*btpb.PingAndWarmResponse, error) {
	s.mu.Lock()
	s.pingCount++
	defer s.mu.Unlock()

	// Capture metadata
	if md, ok := metadata.FromIncomingContext(ctx); ok {
		s.lastPingAndWarmMetadata = md.Copy()
	}

	delay := s.getDelay()

	if delay > 0 {
		select {
		case <-time.After(delay):
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}

	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if err := s.getPingErr(); err != nil {
		return nil, err
	}
	return &btpb.PingAndWarmResponse{}, nil
}

func (s *fakeService) getCallCount() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.callCount
}

func (s *fakeService) getPrimeMetadata() metadata.MD {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.lastPingAndWarmMetadata
}

func (s *fakeService) getPingCount() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.pingCount
}

func (s *fakeService) setPingCount(count int) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.pingCount = count
}

func setupTestServer(t testing.TB, service *fakeService) string {
	t.Helper()
	lis, err := net.Listen("tcp", "localhost:0")
	if err != nil {
		t.Fatalf("Failed to listen: %v", err)
	}
	srv := grpc.NewServer()
	testpb.RegisterBenchmarkServiceServer(srv, service)
	btpb.RegisterBigtableServer(srv, service)
	go func() {
		srv.Serve(lis)
	}()

	t.Cleanup(func() {
		srv.Stop()
		lis.Close()
	})

	return lis.Addr().String()
}

func dialBigtableserver(addr string) (*BigtableConn, error) {
	conn, err := grpc.Dial(addr, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		return nil, err
	}
	return NewBigtableConn(conn), nil
}

// isConnClosed checks if a grpc.ClientConn has been closed.
func isConnClosed(conn *grpc.ClientConn) bool {
	return conn.GetState() == connectivity.Shutdown
}

// setConnLoads is a test helper to set the load on all connections in a slice.
func setConnLoads(conns []*connEntry, unary, stream int32) {
	for _, entry := range conns {
		entry.unaryLoad.Store(unary)
		entry.streamingLoad.Store(stream)
	}
}

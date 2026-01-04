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
	pingErrs                []error       // Errors to return from PingAndWarm
	pingErrMu               sync.Mutex    // Protects pingErr
	streamRecvErr           error         // Error to return from stream.Recv()
	streamSendErr           error         // Error to return from stream.Send()
}

func (s *fakeService) setPingErr(errs ...error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.pingErrs = errs
}

func (s *fakeService) setDelay(duration time.Duration) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.delay = duration
}

func (s *fakeService) getDelay() time.Duration {
	s.pingErrMu.Lock()
	defer s.pingErrMu.Unlock()
	return s.delay
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

func (f *fakeService) getPingCallCount() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.pingCount
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

func (f *fakeService) reset() {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.callCount = 0
	f.pingCount = 0
	f.serverErr = nil
	f.pingErrs = nil
	f.delay = 0
	f.lastPingAndWarmMetadata = nil
	if f.streamSema != nil {
		select {
		case <-f.streamSema: // Drain if not closed
		default:
		}
	}
	f.streamSema = nil
}

func (s *fakeService) PingAndWarm(ctx context.Context, req *btpb.PingAndWarmRequest) (*btpb.PingAndWarmResponse, error) {
	s.mu.Lock()
	callNum := s.pingCount
	s.pingCount++

	var err error
	if len(s.pingErrs) > 0 {
		if callNum < len(s.pingErrs) {
			err = s.pingErrs[callNum]
		} else {
			// If callCount exceeds provided errors, use the last one for subsequent calls
			err = s.pingErrs[len(s.pingErrs)-1]
		}
	}

	delay := s.delay
	// Capture metadata on the first call, assuming headers are constant
	if callNum == 0 {
		s.lastPingAndWarmMetadata, _ = metadata.FromIncomingContext(ctx)
	}
	s.mu.Unlock()
	if delay > 0 {
		select {
		case <-time.After(delay):
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}
	if err != nil {
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

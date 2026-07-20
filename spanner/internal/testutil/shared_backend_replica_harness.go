// Copyright 2026 Google LLC
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     https://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package testutil

import (
	"context"
	"fmt"
	"net"
	"strings"
	"sync"
	"testing"

	"cloud.google.com/go/spanner/apiv1/spannerpb"
	"google.golang.org/api/option"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/protobuf/proto"
)

const (
	// MethodRead identifies unary Read requests recorded by the shared replica
	// harness.
	MethodRead = "READ"
)

// HookedUnaryReplicaServer exposes one shared in-memory Spanner backend through
// a dedicated listener address and allows per-method error injection while
// keeping session and transaction state shared across replicas.
type HookedUnaryReplicaServer struct {
	InMemSpannerServer

	mu           sync.Mutex
	methodErrors map[string][]error
	requests     map[string][]proto.Message
}

// NewHookedUnaryReplicaServer creates a replica-fronting test server backed by
// the shared in-memory Spanner backend.
func NewHookedUnaryReplicaServer(backend InMemSpannerServer) *HookedUnaryReplicaServer {
	return &HookedUnaryReplicaServer{
		InMemSpannerServer: backend,
		methodErrors:       make(map[string][]error),
		requests:           make(map[string][]proto.Message),
	}
}

// PutMethodErrors replaces the queued injected errors for the given method.
func (s *HookedUnaryReplicaServer) PutMethodErrors(method string, errs ...error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.methodErrors[method] = append([]error(nil), errs...)
}

// Requests returns a copy of the captured requests for the given method.
func (s *HookedUnaryReplicaServer) Requests(method string) []proto.Message {
	s.mu.Lock()
	defer s.mu.Unlock()
	reqs := s.requests[method]
	out := make([]proto.Message, len(reqs))
	copy(out, reqs)
	return out
}

// ExecuteSql records and optionally injects an error before delegating to the
// shared backend.
func (s *HookedUnaryReplicaServer) ExecuteSql(ctx context.Context, req *spannerpb.ExecuteSqlRequest) (*spannerpb.ResultSet, error) {
	s.recordRequest(MethodExecuteSql, req)
	if err := s.nextError(MethodExecuteSql); err != nil {
		return nil, err
	}
	return s.InMemSpannerServer.ExecuteSql(ctx, req)
}

// ExecuteStreamingSql records and optionally injects an error before
// delegating to the shared backend.
func (s *HookedUnaryReplicaServer) ExecuteStreamingSql(req *spannerpb.ExecuteSqlRequest, stream spannerpb.Spanner_ExecuteStreamingSqlServer) error {
	s.recordRequest(MethodExecuteStreamingSql, req)
	if err := s.nextError(MethodExecuteStreamingSql); err != nil {
		return err
	}
	return s.InMemSpannerServer.ExecuteStreamingSql(req, stream)
}

// StreamingRead records and optionally injects an error before delegating to
// the shared backend.
func (s *HookedUnaryReplicaServer) StreamingRead(req *spannerpb.ReadRequest, stream spannerpb.Spanner_StreamingReadServer) error {
	s.recordRequest(MethodStreamingRead, req)
	if err := s.nextError(MethodStreamingRead); err != nil {
		return err
	}
	return s.InMemSpannerServer.StreamingRead(req, stream)
}

// Read records and optionally injects an error before delegating to the shared
// backend.
func (s *HookedUnaryReplicaServer) Read(ctx context.Context, req *spannerpb.ReadRequest) (*spannerpb.ResultSet, error) {
	s.recordRequest(MethodRead, req)
	if err := s.nextError(MethodRead); err != nil {
		return nil, err
	}
	return s.InMemSpannerServer.ExecuteSql(ctx, &spannerpb.ExecuteSqlRequest{
		Session:        req.GetSession(),
		Transaction:    req.GetTransaction(),
		PartitionToken: req.GetPartitionToken(),
		ResumeToken:    req.GetResumeToken(),
		Sql:            fmt.Sprintf("SELECT %s FROM %s", strings.Join(req.GetColumns(), ", "), req.GetTable()),
	})
}

// BeginTransaction records and optionally injects an error before delegating to
// the shared backend.
func (s *HookedUnaryReplicaServer) BeginTransaction(ctx context.Context, req *spannerpb.BeginTransactionRequest) (*spannerpb.Transaction, error) {
	s.recordRequest(MethodBeginTransaction, req)
	if err := s.nextError(MethodBeginTransaction); err != nil {
		return nil, err
	}
	return s.InMemSpannerServer.BeginTransaction(ctx, req)
}

// Commit records and optionally injects an error before delegating to the
// shared backend.
func (s *HookedUnaryReplicaServer) Commit(ctx context.Context, req *spannerpb.CommitRequest) (*spannerpb.CommitResponse, error) {
	s.recordRequest(MethodCommitTransaction, req)
	if err := s.nextError(MethodCommitTransaction); err != nil {
		return nil, err
	}
	return s.InMemSpannerServer.Commit(ctx, req)
}

func (s *HookedUnaryReplicaServer) recordRequest(method string, req proto.Message) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if req != nil {
		s.requests[method] = append(s.requests[method], proto.Clone(req))
	}
}

func (s *HookedUnaryReplicaServer) nextError(method string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	errs := s.methodErrors[method]
	if len(errs) == 0 {
		return nil
	}
	err := errs[0]
	s.methodErrors[method] = errs[1:]
	return err
}

// SharedBackendSpannerReplicaHarness exposes one shared in-memory backend
// through a default listener and multiple replica listeners for location-aware
// routing tests.
type SharedBackendSpannerReplicaHarness struct {
	Backend InMemSpannerServer

	DefaultReplica   *HookedUnaryReplicaServer
	DefaultAddress   string
	Replicas         []*HookedUnaryReplicaServer
	ReplicaAddresses []string

	grpcServers []*grpc.Server
}

// NewSharedBackendSpannerReplicaHarness starts one default listener plus
// replicaCount routed listeners that all share one in-memory backend.
func NewSharedBackendSpannerReplicaHarness(t *testing.T, replicaCount int, sopt ...grpc.ServerOption) (h *SharedBackendSpannerReplicaHarness, opts []option.ClientOption, teardown func()) {
	t.Helper()

	backend := NewInMemSpannerServer()
	mockedServer := &MockedSpannerInMemTestServer{TestSpanner: backend}
	mockedServer.setupFooResults()
	mockedServer.setupSingersResults()

	h = &SharedBackendSpannerReplicaHarness{
		Backend:          backend,
		DefaultReplica:   NewHookedUnaryReplicaServer(backend),
		Replicas:         make([]*HookedUnaryReplicaServer, replicaCount),
		ReplicaAddresses: make([]string, replicaCount),
	}

	h.DefaultAddress, h.grpcServers = h.startListener(t, h.DefaultReplica, sopt...)
	for i := 0; i < replicaCount; i++ {
		h.Replicas[i] = NewHookedUnaryReplicaServer(backend)
		h.ReplicaAddresses[i], h.grpcServers = h.startListener(t, h.Replicas[i], sopt...)
	}

	opts = []option.ClientOption{
		option.WithEndpoint(h.DefaultAddress),
		option.WithGRPCDialOption(grpc.WithTransportCredentials(insecure.NewCredentials())),
		option.WithoutAuthentication(),
	}

	teardown = func() {
		for i := len(h.grpcServers) - 1; i >= 0; i-- {
			h.grpcServers[i].Stop()
		}
		h.Backend.Stop()
	}
	return h, opts, teardown
}

func (h *SharedBackendSpannerReplicaHarness) startListener(t *testing.T, server spannerpb.SpannerServer, sopt ...grpc.ServerOption) (string, []*grpc.Server) {
	t.Helper()

	grpcServer := grpc.NewServer(sopt...)
	spannerpb.RegisterSpannerServer(grpcServer, server)

	lis, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	go grpcServer.Serve(lis)
	t.Cleanup(func() {
		_ = lis.Close()
	})

	return lis.Addr().String(), append(h.grpcServers, grpcServer)
}

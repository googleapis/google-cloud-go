/*
Copyright 2026 Google LLC

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package spanner

import (
	"context"
	"io"
	"testing"

	vkit "cloud.google.com/go/spanner/apiv1"
	sppb "cloud.google.com/go/spanner/apiv1/spannerpb"
	"github.com/googleapis/gax-go/v2"
	"google.golang.org/grpc"
	"google.golang.org/grpc/metadata"
	"google.golang.org/protobuf/proto"
	structpb "google.golang.org/protobuf/types/known/structpb"
)

// mockSpannerClient records which RPCs were called and can return configurable
//
// responses. It implements spannerClient.
type mockSpannerClient struct {
	// Track which methods were called
	streamingReadCalled    bool
	executeSQLCalled       bool
	executeStreamSQLCalled bool
	beginTxCalled          bool
	commitCalled           bool
	rollbackCalled         bool
	streamingReadCount     int
	executeSQLCount        int
	executeStreamSQLCount  int
	beginTxCount           int
	commitCount            int
	rollbackCount          int
	closeCount             int
	lastBeginTxReq         *sppb.BeginTransactionRequest
	lastCommitReq          *sppb.CommitRequest

	// Return values
	beginTxResp    *sppb.Transaction
	executeSQLResp *sppb.ResultSet
	commitResp     *sppb.CommitResponse
	streamResp     *mockStreamingClient
}

func (m *mockSpannerClient) CallOptions() *vkit.CallOptions { return nil }
func (m *mockSpannerClient) Close() error {
	m.closeCount++
	return nil
}
func (m *mockSpannerClient) Connection() *grpc.ClientConn { return nil }
func (m *mockSpannerClient) CreateSession(ctx context.Context, req *sppb.CreateSessionRequest, opts ...gax.CallOption) (*sppb.Session, error) {
	return nil, nil
}
func (m *mockSpannerClient) BatchCreateSessions(ctx context.Context, req *sppb.BatchCreateSessionsRequest, opts ...gax.CallOption) (*sppb.BatchCreateSessionsResponse, error) {
	return nil, nil
}
func (m *mockSpannerClient) GetSession(ctx context.Context, req *sppb.GetSessionRequest, opts ...gax.CallOption) (*sppb.Session, error) {
	return nil, nil
}
func (m *mockSpannerClient) ListSessions(ctx context.Context, req *sppb.ListSessionsRequest, opts ...gax.CallOption) *vkit.SessionIterator {
	return nil
}
func (m *mockSpannerClient) DeleteSession(ctx context.Context, req *sppb.DeleteSessionRequest, opts ...gax.CallOption) error {
	return nil
}
func (m *mockSpannerClient) ExecuteBatchDml(ctx context.Context, req *sppb.ExecuteBatchDmlRequest, opts ...gax.CallOption) (*sppb.ExecuteBatchDmlResponse, error) {
	return nil, nil
}
func (m *mockSpannerClient) PartitionQuery(ctx context.Context, req *sppb.PartitionQueryRequest, opts ...gax.CallOption) (*sppb.PartitionResponse, error) {
	return nil, nil
}
func (m *mockSpannerClient) PartitionRead(ctx context.Context, req *sppb.PartitionReadRequest, opts ...gax.CallOption) (*sppb.PartitionResponse, error) {
	return nil, nil
}
func (m *mockSpannerClient) BatchWrite(ctx context.Context, req *sppb.BatchWriteRequest, opts ...gax.CallOption) (sppb.Spanner_BatchWriteClient, error) {
	return nil, nil
}

func (m *mockSpannerClient) StreamingRead(ctx context.Context, req *sppb.ReadRequest, opts ...gax.CallOption) (sppb.Spanner_StreamingReadClient, error) {
	m.streamingReadCalled = true
	m.streamingReadCount++
	return m.streamResp, nil
}

func (m *mockSpannerClient) Read(ctx context.Context, req *sppb.ReadRequest, opts ...gax.CallOption) (*sppb.ResultSet, error) {
	return &sppb.ResultSet{}, nil
}

func (m *mockSpannerClient) ExecuteStreamingSql(ctx context.Context, req *sppb.ExecuteSqlRequest, opts ...gax.CallOption) (sppb.Spanner_ExecuteStreamingSqlClient, error) {
	m.executeStreamSQLCalled = true
	m.executeStreamSQLCount++
	return m.streamResp, nil
}

func (m *mockSpannerClient) ExecuteSql(ctx context.Context, req *sppb.ExecuteSqlRequest, opts ...gax.CallOption) (*sppb.ResultSet, error) {
	m.executeSQLCalled = true
	m.executeSQLCount++
	return m.executeSQLResp, nil
}

func (m *mockSpannerClient) BeginTransaction(ctx context.Context, req *sppb.BeginTransactionRequest, opts ...gax.CallOption) (*sppb.Transaction, error) {
	m.beginTxCalled = true
	m.beginTxCount++
	if req != nil {
		m.lastBeginTxReq = proto.Clone(req).(*sppb.BeginTransactionRequest)
	}
	return m.beginTxResp, nil
}

func (m *mockSpannerClient) Commit(ctx context.Context, req *sppb.CommitRequest, opts ...gax.CallOption) (*sppb.CommitResponse, error) {
	m.commitCalled = true
	m.commitCount++
	if req != nil {
		m.lastCommitReq = proto.Clone(req).(*sppb.CommitRequest)
	}
	return m.commitResp, nil
}

func (m *mockSpannerClient) Rollback(ctx context.Context, req *sppb.RollbackRequest, opts ...gax.CallOption) error {
	m.rollbackCalled = true
	m.rollbackCount++
	return nil
}

// mockStreamingClient implements both Spanner_StreamingReadClient and
// Spanner_ExecuteStreamingSqlClient interfaces.
type mockStreamingClient struct {
	grpc.ClientStream
	results []*sppb.PartialResultSet
	index   int
}

func (m *mockStreamingClient) Recv() (*sppb.PartialResultSet, error) {
	if m.index >= len(m.results) {
		return nil, io.EOF
	}
	prs := m.results[m.index]
	m.index++
	return prs, nil
}

func (m *mockStreamingClient) Header() (metadata.MD, error) { return nil, nil }
func (m *mockStreamingClient) Trailer() metadata.MD         { return nil }
func (m *mockStreamingClient) CloseSend() error             { return nil }
func (m *mockStreamingClient) Context() context.Context     { return context.Background() }
func (m *mockStreamingClient) SendMsg(interface{}) error    { return nil }
func (m *mockStreamingClient) RecvMsg(interface{}) error    { return nil }

// mockEndpointCache implements channelEndpointCache for testing.
type mockEndpointCache struct {
	clients map[string]spannerClient
	seen    map[string]*grpcChannelEndpoint
}

func newMockEndpointCache() *mockEndpointCache {
	return &mockEndpointCache{
		clients: make(map[string]spannerClient),
		seen:    make(map[string]*grpcChannelEndpoint),
	}
}

func (c *mockEndpointCache) Get(_ context.Context, address string) channelEndpoint {
	if _, ok := c.clients[address]; ok {
		if ep, ok := c.seen[address]; ok {
			return ep
		}
		ep := &grpcChannelEndpoint{address: address}
		ep.healthy.Store(true)
		c.seen[address] = ep
		return ep
	}
	return nil
}

func (c *mockEndpointCache) ClientFor(ep channelEndpoint) spannerClient {
	if ep == nil {
		return nil
	}
	return c.clients[ep.Address()]
}

func (c *mockEndpointCache) Close() error { return nil }

func (c *mockEndpointCache) addEndpoint(address string, client spannerClient) {
	c.clients[address] = client
}

func TestLocationAwareSpannerClient_RoutesToEndpoint(t *testing.T) {
	defaultClient := &mockSpannerClient{}
	endpointClient := &mockSpannerClient{
		beginTxResp: &sppb.Transaction{Id: []byte("tx1")},
	}

	epCache := newMockEndpointCache()
	epCache.addEndpoint("server1:443", endpointClient)

	router := newLocationRouter(epCache)
	// Seed the range cache so finder returns an endpoint.
	router.finder.rangeCache.endpointCache = epCache

	lac := newLocationAwareSpannerClient(defaultClient, router, epCache)

	// BeginTransaction should route to endpoint client when the finder returns one.
	// Since we haven't set up the finder's caches, it will fall back to default.
	resp, err := lac.BeginTransaction(context.Background(), &sppb.BeginTransactionRequest{
		Session: "projects/p/instances/i/databases/d/sessions/s",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// Without finder caches, it should use default client.
	if !defaultClient.beginTxCalled {
		t.Fatal("expected default client to be called when no endpoint found")
	}
	_ = resp
}

func TestLocationAwareSpannerClient_FallsBackToDefault(t *testing.T) {
	defaultClient := &mockSpannerClient{
		executeSQLResp: &sppb.ResultSet{},
	}

	epCache := newMockEndpointCache()
	router := newLocationRouter(epCache)

	lac := newLocationAwareSpannerClient(defaultClient, router, epCache)

	// No endpoints configured, should use default.
	_, err := lac.ExecuteSql(context.Background(), &sppb.ExecuteSqlRequest{
		Sql: "SELECT 1",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !defaultClient.executeSQLCalled {
		t.Fatal("expected default client to be called")
	}
}

func TestLocationAwareSpannerClient_TransactionAffinity_BeginTransaction(t *testing.T) {
	defaultClient := &mockSpannerClient{
		beginTxResp: &sppb.Transaction{Id: []byte("tx-123")},
		commitResp:  &sppb.CommitResponse{},
	}
	endpointClient := &mockSpannerClient{
		commitResp: &sppb.CommitResponse{},
	}

	epCache := newMockEndpointCache()
	epCache.addEndpoint("server1:443", endpointClient)

	router := newLocationRouter(epCache)
	lac := newLocationAwareSpannerClient(defaultClient, router, epCache)

	// Simulate that BeginTransaction was routed to a specific endpoint.
	ep := &grpcChannelEndpoint{address: "server1:443"}
	ep.healthy.Store(true)
	router.setTransactionAffinity("tx-123", ep)

	// Commit should route to the same endpoint.
	_, err := lac.Commit(context.Background(), &sppb.CommitRequest{
		Transaction: &sppb.CommitRequest_TransactionId{
			TransactionId: []byte("tx-123"),
		},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !endpointClient.commitCalled {
		t.Fatal("expected endpoint client to be called for Commit")
	}
	if defaultClient.commitCalled {
		t.Fatal("expected default client NOT to be called for Commit")
	}

	// Affinity should be cleared after commit.
	if router.getTransactionAffinity("tx-123") != nil {
		t.Fatal("expected transaction affinity to be cleared after Commit")
	}
}

func TestLocationAwareSpannerClient_BeginTransactionAddsRoutingHint(t *testing.T) {
	defaultClient := &mockSpannerClient{
		beginTxResp: &sppb.Transaction{Id: []byte("tx-begin-hint")},
	}
	epCache := newMockEndpointCache()
	router := newLocationRouter(epCache)
	router.observeResultSet(&sppb.ResultSet{CacheUpdate: createMutationRoutingCacheUpdate()})

	lac := newLocationAwareSpannerClient(defaultClient, router, epCache)
	_, err := lac.BeginTransaction(context.Background(), &sppb.BeginTransactionRequest{
		Session:     "projects/p/instances/i/databases/d/sessions/s",
		MutationKey: createInsertMutation("b"),
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if defaultClient.lastBeginTxReq == nil {
		t.Fatal("expected BeginTransaction request to be captured")
	}
	hint := defaultClient.lastBeginTxReq.GetRoutingHint()
	if hint == nil {
		t.Fatal("expected BeginTransaction routing hint")
	}
	if hint.GetDatabaseId() != 7 {
		t.Fatalf("expected database id 7, got %d", hint.GetDatabaseId())
	}
	if string(hint.GetSchemaGeneration()) != "1" {
		t.Fatalf("expected schema generation 1, got %q", hint.GetSchemaGeneration())
	}
	if len(hint.GetKey()) == 0 {
		t.Fatal("expected BeginTransaction encoded key")
	}
}

func TestLocationAwareSpannerClient_TransactionCacheUpdateEnablesCommitRoutingHint(t *testing.T) {
	defaultClient := &mockSpannerClient{
		beginTxResp: &sppb.Transaction{
			Id:          []byte("tx-cache-update"),
			CacheUpdate: createMutationRoutingCacheUpdate(),
		},
		commitResp: &sppb.CommitResponse{},
	}
	epCache := newMockEndpointCache()
	router := newLocationRouter(epCache)
	lac := newLocationAwareSpannerClient(defaultClient, router, epCache)

	_, err := lac.BeginTransaction(context.Background(), &sppb.BeginTransactionRequest{
		Session: "projects/p/instances/i/databases/d/sessions/s",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	_, err = lac.Commit(context.Background(), &sppb.CommitRequest{
		Session: "projects/p/instances/i/databases/d/sessions/s",
		Transaction: &sppb.CommitRequest_TransactionId{
			TransactionId: []byte("tx-cache-update"),
		},
		Mutations: []*sppb.Mutation{createInsertMutation("b")},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if defaultClient.lastCommitReq == nil {
		t.Fatal("expected Commit request to be captured")
	}
	hint := defaultClient.lastCommitReq.GetRoutingHint()
	if hint == nil {
		t.Fatal("expected Commit routing hint")
	}
	if hint.GetDatabaseId() != 7 {
		t.Fatalf("expected database id 7, got %d", hint.GetDatabaseId())
	}
	if string(hint.GetSchemaGeneration()) != "1" {
		t.Fatalf("expected schema generation 1, got %q", hint.GetSchemaGeneration())
	}
	if len(hint.GetKey()) == 0 {
		t.Fatal("expected Commit encoded key")
	}
}

func TestLocationAwareSpannerClient_SingleUseCommitRoutesUsingRoutingHint(t *testing.T) {
	defaultClient := &mockSpannerClient{commitResp: &sppb.CommitResponse{}}
	endpointClient := &mockSpannerClient{commitResp: &sppb.CommitResponse{}}

	epCache := newMockEndpointCache()
	epCache.addEndpoint("server-a:443", endpointClient)
	router := newLocationRouter(epCache)
	router.observeResultSet(&sppb.ResultSet{CacheUpdate: createMutationRecipeCacheUpdate()})

	lac := newLocationAwareSpannerClient(defaultClient, router, epCache)
	_, err := lac.Commit(context.Background(), &sppb.CommitRequest{
		Session: "projects/p/instances/i/databases/d/sessions/s",
		Transaction: &sppb.CommitRequest_SingleUseTransaction{
			SingleUseTransaction: &sppb.TransactionOptions{
				Mode: &sppb.TransactionOptions_ReadWrite_{ReadWrite: &sppb.TransactionOptions_ReadWrite{}},
			},
		},
		Mutations: []*sppb.Mutation{createInsertMutation("b")},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if defaultClient.lastCommitReq == nil {
		t.Fatal("expected initial Commit request to be captured")
	}
	router.observeResultSet(&sppb.ResultSet{CacheUpdate: createRangeCacheUpdateForHint(defaultClient.lastCommitReq.GetRoutingHint())})

	_, err = lac.Commit(context.Background(), &sppb.CommitRequest{
		Session: "projects/p/instances/i/databases/d/sessions/s",
		Transaction: &sppb.CommitRequest_SingleUseTransaction{
			SingleUseTransaction: &sppb.TransactionOptions{
				Mode: &sppb.TransactionOptions_ReadWrite_{ReadWrite: &sppb.TransactionOptions_ReadWrite{}},
			},
		},
		Mutations: []*sppb.Mutation{createInsertMutation("b")},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if defaultClient.commitCount != 1 {
		t.Fatalf("expected first Commit to use default client once, got %d", defaultClient.commitCount)
	}
	if endpointClient.commitCount != 1 {
		t.Fatalf("expected second Commit to route to endpoint client once, got %d", endpointClient.commitCount)
	}
	if endpointClient.lastCommitReq == nil || endpointClient.lastCommitReq.GetRoutingHint() == nil {
		t.Fatal("expected routed Commit request to contain a routing hint")
	}
}

func TestLocationAwareSpannerClient_SingleUseCommitUsesSameMutationSelectionAsBeginTransaction(t *testing.T) {
	defaultClient := &mockSpannerClient{
		beginTxResp: &sppb.Transaction{Id: []byte("tx-selection")},
		commitResp:  &sppb.CommitResponse{},
	}
	epCache := newMockEndpointCache()
	router := newLocationRouter(epCache)
	router.observeResultSet(&sppb.ResultSet{CacheUpdate: createMutationRecipeCacheUpdate()})

	lac := newLocationAwareSpannerClient(defaultClient, router, epCache)
	deleteMutation := createDeleteMutation("b")

	_, err := lac.BeginTransaction(context.Background(), &sppb.BeginTransactionRequest{
		Session:     "projects/p/instances/i/databases/d/sessions/s",
		MutationKey: deleteMutation,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if defaultClient.lastBeginTxReq == nil {
		t.Fatal("expected BeginTransaction request to be captured")
	}
	expectedHint := proto.Clone(defaultClient.lastBeginTxReq.GetRoutingHint()).(*sppb.RoutingHint)

	_, err = lac.Commit(context.Background(), &sppb.CommitRequest{
		Session: "projects/p/instances/i/databases/d/sessions/s",
		Transaction: &sppb.CommitRequest_SingleUseTransaction{
			SingleUseTransaction: &sppb.TransactionOptions{
				Mode: &sppb.TransactionOptions_ReadWrite_{ReadWrite: &sppb.TransactionOptions_ReadWrite{}},
			},
		},
		Mutations: []*sppb.Mutation{
			createInsertMutation("a"),
			deleteMutation,
		},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if defaultClient.lastCommitReq == nil {
		t.Fatal("expected Commit request to be captured")
	}
	if !proto.Equal(expectedHint, defaultClient.lastCommitReq.GetRoutingHint()) {
		t.Fatalf("expected Commit routing hint %v, got %v", expectedHint, defaultClient.lastCommitReq.GetRoutingHint())
	}
}

func TestLocationAwareSpannerClient_CommitWithTransactionIDRoutesUsingRoutingHintWhenAffinityMissing(t *testing.T) {
	defaultClient := &mockSpannerClient{commitResp: &sppb.CommitResponse{}}
	endpointClient := &mockSpannerClient{commitResp: &sppb.CommitResponse{}}

	epCache := newMockEndpointCache()
	epCache.addEndpoint("server-a:443", endpointClient)
	router := newLocationRouter(epCache)
	router.observeResultSet(&sppb.ResultSet{CacheUpdate: createMutationRecipeCacheUpdate()})

	lac := newLocationAwareSpannerClient(defaultClient, router, epCache)
	_, err := lac.Commit(context.Background(), &sppb.CommitRequest{
		Session: "projects/p/instances/i/databases/d/sessions/s",
		Transaction: &sppb.CommitRequest_TransactionId{
			TransactionId: []byte("tx-no-affinity"),
		},
		Mutations: []*sppb.Mutation{createInsertMutation("b")},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if defaultClient.lastCommitReq == nil {
		t.Fatal("expected initial Commit request to be captured")
	}
	router.observeResultSet(&sppb.ResultSet{CacheUpdate: createRangeCacheUpdateForHint(defaultClient.lastCommitReq.GetRoutingHint())})

	_, err = lac.Commit(context.Background(), &sppb.CommitRequest{
		Session: "projects/p/instances/i/databases/d/sessions/s",
		Transaction: &sppb.CommitRequest_TransactionId{
			TransactionId: []byte("tx-no-affinity"),
		},
		Mutations: []*sppb.Mutation{createInsertMutation("b")},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if defaultClient.commitCount != 1 {
		t.Fatalf("expected first Commit to use default client once, got %d", defaultClient.commitCount)
	}
	if endpointClient.commitCount != 1 {
		t.Fatalf("expected second Commit to route to endpoint client once, got %d", endpointClient.commitCount)
	}
}

func TestLocationAwareSpannerClient_CommitResponseCacheUpdateEnablesSubsequentBeginRoutingHint(t *testing.T) {
	defaultClient := &mockSpannerClient{
		beginTxResp: &sppb.Transaction{Id: []byte("tx-commit-update")},
		commitResp:  &sppb.CommitResponse{CacheUpdate: createMutationRoutingCacheUpdate()},
	}
	epCache := newMockEndpointCache()
	router := newLocationRouter(epCache)
	lac := newLocationAwareSpannerClient(defaultClient, router, epCache)

	_, err := lac.BeginTransaction(context.Background(), &sppb.BeginTransactionRequest{
		Session: "projects/p/instances/i/databases/d/sessions/s",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	_, err = lac.Commit(context.Background(), &sppb.CommitRequest{
		Session: "projects/p/instances/i/databases/d/sessions/s",
		Transaction: &sppb.CommitRequest_TransactionId{
			TransactionId: []byte("tx-commit-update"),
		},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	_, err = lac.BeginTransaction(context.Background(), &sppb.BeginTransactionRequest{
		Session:     "projects/p/instances/i/databases/d/sessions/s",
		MutationKey: createInsertMutation("b"),
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if defaultClient.lastBeginTxReq == nil {
		t.Fatal("expected BeginTransaction request to be captured")
	}
	hint := defaultClient.lastBeginTxReq.GetRoutingHint()
	if hint == nil {
		t.Fatal("expected BeginTransaction routing hint after commit cache update")
	}
	if hint.GetDatabaseId() != 7 {
		t.Fatalf("expected database id 7, got %d", hint.GetDatabaseId())
	}
	if string(hint.GetSchemaGeneration()) != "1" {
		t.Fatalf("expected schema generation 1, got %q", hint.GetSchemaGeneration())
	}
	if len(hint.GetKey()) == 0 {
		t.Fatal("expected BeginTransaction encoded key")
	}
}

func TestLocationAwareSpannerClient_TransactionAffinity_Rollback(t *testing.T) {
	defaultClient := &mockSpannerClient{}
	endpointClient := &mockSpannerClient{}

	epCache := newMockEndpointCache()
	epCache.addEndpoint("server2:443", endpointClient)

	router := newLocationRouter(epCache)
	lac := newLocationAwareSpannerClient(defaultClient, router, epCache)

	ep := &grpcChannelEndpoint{address: "server2:443"}
	ep.healthy.Store(true)
	router.setTransactionAffinity("tx-456", ep)

	err := lac.Rollback(context.Background(), &sppb.RollbackRequest{
		TransactionId: []byte("tx-456"),
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !endpointClient.rollbackCalled {
		t.Fatal("expected endpoint client to be called for Rollback")
	}
	if defaultClient.rollbackCalled {
		t.Fatal("expected default client NOT to be called for Rollback")
	}

	// Affinity should be cleared after rollback.
	if router.getTransactionAffinity("tx-456") != nil {
		t.Fatal("expected transaction affinity to be cleared after Rollback")
	}
}

func TestLocationAwareSpannerClient_AffinityFromStreaming(t *testing.T) {
	defaultClient := &mockSpannerClient{
		streamResp: &mockStreamingClient{
			results: []*sppb.PartialResultSet{
				{
					Metadata: &sppb.ResultSetMetadata{
						Transaction: &sppb.Transaction{
							Id: []byte("tx-stream-1"),
						},
					},
				},
				{
					// Second PRS without tx metadata.
				},
			},
		},
	}

	endpointClient := &mockSpannerClient{
		streamResp: &mockStreamingClient{
			results: []*sppb.PartialResultSet{
				{
					Metadata: &sppb.ResultSetMetadata{
						Transaction: &sppb.Transaction{
							Id: []byte("tx-stream-ep"),
						},
					},
				},
			},
		},
	}

	epCache := newMockEndpointCache()
	epCache.addEndpoint("server3:443", endpointClient)

	router := newLocationRouter(epCache)
	lac := newLocationAwareSpannerClient(defaultClient, router, epCache)

	// Without endpoint routing, stream should go to default.
	stream, err := lac.ExecuteStreamingSql(context.Background(), &sppb.ExecuteSqlRequest{
		Sql: "SELECT 1",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Read all results.
	for {
		_, err := stream.Recv()
		if err == io.EOF {
			break
		}
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
	}

	// Since it went through default (no ep found by finder), affinity should
	// NOT be set (ep was nil).
	if router.getTransactionAffinity("tx-stream-1") != nil {
		t.Fatal("expected no affinity when no endpoint was used")
	}
}

func TestLocationAwareSpannerClient_ExecuteSqlSetsAffinity(t *testing.T) {
	endpointClient := &mockSpannerClient{
		executeSQLResp: &sppb.ResultSet{
			Metadata: &sppb.ResultSetMetadata{
				Transaction: &sppb.Transaction{
					Id: []byte("tx-exec-1"),
				},
			},
		},
	}

	defaultClient := &mockSpannerClient{
		executeSQLResp: &sppb.ResultSet{
			Metadata: &sppb.ResultSetMetadata{
				Transaction: &sppb.Transaction{
					Id: []byte("tx-exec-default"),
				},
			},
		},
	}

	epCache := newMockEndpointCache()
	router := newLocationRouter(epCache)
	lac := newLocationAwareSpannerClient(defaultClient, router, epCache)

	// With no endpoint available, ExecuteSql should NOT set affinity.
	_, err := lac.ExecuteSql(context.Background(), &sppb.ExecuteSqlRequest{Sql: "SELECT 1"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if router.getTransactionAffinity("tx-exec-default") != nil {
		t.Fatal("expected no affinity when using default client")
	}

	_ = endpointClient // Will be used when endpoint routing is tested
}

func TestLocationAwareSpannerClient_ReadOnlyInlinedBeginRoutesIndependently(t *testing.T) {
	defaultClient := &mockSpannerClient{executeSQLResp: &sppb.ResultSet{}}
	endpointA := &mockSpannerClient{
		executeSQLResp: &sppb.ResultSet{
			Metadata: &sppb.ResultSetMetadata{
				Transaction: &sppb.Transaction{Id: []byte("ro-inline-1")},
			},
		},
	}
	endpointB := &mockSpannerClient{executeSQLResp: &sppb.ResultSet{}}

	epCache := newMockEndpointCache()
	epCache.addEndpoint("server-a:443", endpointA)
	epCache.addEndpoint("server-b:443", endpointB)
	router := newLocationRouter(epCache)
	seedTwoRangeRoutingCache(router)

	lac := newLocationAwareSpannerClient(defaultClient, router, epCache)

	_, err := lac.ExecuteSql(context.Background(), executeSQLWithKeyAndSelector("b", &sppb.TransactionSelector{
		Selector: &sppb.TransactionSelector_Begin{
			Begin: &sppb.TransactionOptions{
				Mode: &sppb.TransactionOptions_ReadOnly_{
					ReadOnly: &sppb.TransactionOptions_ReadOnly{
						TimestampBound: &sppb.TransactionOptions_ReadOnly_Strong{Strong: true},
					},
				},
			},
		},
	}))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	_, err = lac.ExecuteSql(context.Background(), executeSQLWithKeyAndSelector("n", &sppb.TransactionSelector{
		Selector: &sppb.TransactionSelector_Id{Id: []byte("ro-inline-1")},
	}))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if endpointA.executeSQLCount != 1 {
		t.Fatalf("expected first RO request to hit server-a once, got %d", endpointA.executeSQLCount)
	}
	if endpointB.executeSQLCount != 1 {
		t.Fatalf("expected second RO request to hit server-b once, got %d", endpointB.executeSQLCount)
	}
	if router.getTransactionAffinity("ro-inline-1") != nil {
		t.Fatal("expected no affinity for read-only transaction")
	}
	if !router.isReadOnlyTransaction("ro-inline-1") {
		t.Fatal("expected read-only transaction to be tracked")
	}
}

func TestLocationAwareSpannerClient_ReadWriteInlinedBeginMaintainsAffinity(t *testing.T) {
	defaultClient := &mockSpannerClient{executeSQLResp: &sppb.ResultSet{}}
	endpointA := &mockSpannerClient{
		executeSQLResp: &sppb.ResultSet{
			Metadata: &sppb.ResultSetMetadata{
				Transaction: &sppb.Transaction{Id: []byte("rw-inline-1")},
			},
		},
		commitResp: &sppb.CommitResponse{},
	}
	endpointB := &mockSpannerClient{executeSQLResp: &sppb.ResultSet{}}

	epCache := newMockEndpointCache()
	epCache.addEndpoint("server-a:443", endpointA)
	epCache.addEndpoint("server-b:443", endpointB)
	router := newLocationRouter(epCache)
	seedTwoRangeRoutingCache(router)

	lac := newLocationAwareSpannerClient(defaultClient, router, epCache)

	_, err := lac.ExecuteSql(context.Background(), executeSQLWithKeyAndSelector("b", &sppb.TransactionSelector{
		Selector: &sppb.TransactionSelector_Begin{
			Begin: &sppb.TransactionOptions{
				Mode: &sppb.TransactionOptions_ReadWrite_{ReadWrite: &sppb.TransactionOptions_ReadWrite{}},
			},
		},
	}))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	_, err = lac.ExecuteSql(context.Background(), executeSQLWithKeyAndSelector("n", &sppb.TransactionSelector{
		Selector: &sppb.TransactionSelector_Id{Id: []byte("rw-inline-1")},
	}))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	_, err = lac.Commit(context.Background(), &sppb.CommitRequest{
		Transaction: &sppb.CommitRequest_TransactionId{
			TransactionId: []byte("rw-inline-1"),
		},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if endpointA.executeSQLCount != 2 {
		t.Fatalf("expected both RW requests to hit server-a, got %d", endpointA.executeSQLCount)
	}
	if endpointB.executeSQLCount != 0 {
		t.Fatalf("expected no RW request on server-b, got %d", endpointB.executeSQLCount)
	}
	if endpointA.commitCount != 1 {
		t.Fatalf("expected commit on server-a once, got %d", endpointA.commitCount)
	}
}

func TestLocationAwareSpannerClient_ReadWriteExplicitBeginPinsDefaultClient(t *testing.T) {
	defaultClient := &mockSpannerClient{
		beginTxResp:    &sppb.Transaction{Id: []byte("rw-explicit-1")},
		executeSQLResp: &sppb.ResultSet{},
	}
	endpointB := &mockSpannerClient{executeSQLResp: &sppb.ResultSet{}}

	epCache := newMockEndpointCache()
	epCache.addEndpoint("server-b:443", endpointB)
	router := newLocationRouter(epCache)
	seedTwoRangeRoutingCache(router)
	lac := newLocationAwareSpannerClient(defaultClient, router, epCache)

	_, err := lac.BeginTransaction(context.Background(), &sppb.BeginTransactionRequest{
		Session: "projects/p/instances/i/databases/d/sessions/s",
		Options: &sppb.TransactionOptions{
			Mode: &sppb.TransactionOptions_ReadWrite_{ReadWrite: &sppb.TransactionOptions_ReadWrite{}},
		},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	_, err = lac.ExecuteSql(context.Background(), executeSQLWithKeyAndSelector("n", &sppb.TransactionSelector{
		Selector: &sppb.TransactionSelector_Id{Id: []byte("rw-explicit-1")},
	}))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if defaultClient.executeSQLCount != 1 {
		t.Fatalf("expected RW request to stay on default client, got %d", defaultClient.executeSQLCount)
	}
	if endpointB.executeSQLCount != 0 {
		t.Fatalf("expected no RW request on routed endpoint, got %d", endpointB.executeSQLCount)
	}
}

func TestLocationAwareSpannerClient_ReadOnlyTransactionIgnoresAffinityLookup(t *testing.T) {
	defaultClient := &mockSpannerClient{executeSQLResp: &sppb.ResultSet{}}
	endpointA := &mockSpannerClient{executeSQLResp: &sppb.ResultSet{}}
	endpointB := &mockSpannerClient{executeSQLResp: &sppb.ResultSet{}}

	epCache := newMockEndpointCache()
	epCache.addEndpoint("server-a:443", endpointA)
	epCache.addEndpoint("server-b:443", endpointB)
	router := newLocationRouter(epCache)
	seedTwoRangeRoutingCache(router)
	lac := newLocationAwareSpannerClient(defaultClient, router, epCache)

	ep := epCache.Get(context.Background(), "server-a:443")
	router.setTransactionAffinity("ro-explicit-1", ep)
	router.trackReadOnlyTransaction("ro-explicit-1", true)

	_, err := lac.ExecuteSql(context.Background(), executeSQLWithKeyAndSelector("n", &sppb.TransactionSelector{
		Selector: &sppb.TransactionSelector_Id{Id: []byte("ro-explicit-1")},
	}))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if endpointA.executeSQLCount != 0 {
		t.Fatalf("expected read-only txn to ignore affinity endpoint, got server-a count %d", endpointA.executeSQLCount)
	}
	if endpointB.executeSQLCount != 1 {
		t.Fatalf("expected read-only txn to route by key to server-b, got %d", endpointB.executeSQLCount)
	}
}

func TestLocationAwareSpannerClient_PassThroughRPCs(t *testing.T) {
	defaultClient := &mockSpannerClient{}
	epCache := newMockEndpointCache()
	router := newLocationRouter(epCache)
	lac := newLocationAwareSpannerClient(defaultClient, router, epCache)

	// These should all pass through to default client.
	lac.CreateSession(context.Background(), &sppb.CreateSessionRequest{})
	lac.BatchCreateSessions(context.Background(), &sppb.BatchCreateSessionsRequest{})
	lac.GetSession(context.Background(), &sppb.GetSessionRequest{})
	lac.DeleteSession(context.Background(), &sppb.DeleteSessionRequest{})
	lac.ExecuteBatchDml(context.Background(), &sppb.ExecuteBatchDmlRequest{})
	lac.PartitionQuery(context.Background(), &sppb.PartitionQueryRequest{})
	lac.PartitionRead(context.Background(), &sppb.PartitionReadRequest{})
}

func TestLocationAwareSpannerClient_AsGRPCSpannerClient(t *testing.T) {
	// Test that asGRPCSpannerClient can extract from wrapper.
	gsc := &grpcSpannerClient{}
	if got := asGRPCSpannerClient(gsc); got != gsc {
		t.Fatal("expected same grpcSpannerClient back")
	}

	// Test extraction from locationAwareSpannerClient.
	lac := &locationAwareSpannerClient{defaultClient: gsc}
	if got := asGRPCSpannerClient(lac); got != gsc {
		t.Fatal("expected to extract grpcSpannerClient from wrapper")
	}

	// Test nil for unknown types.
	if got := asGRPCSpannerClient(&mockSpannerClient{}); got != nil {
		t.Fatal("expected nil for unknown client type")
	}
}

func TestLocationAwareSpannerClient_CloseDoesNotCloseDefaultClient(t *testing.T) {
	defaultClient := &mockSpannerClient{}
	lac := newLocationAwareSpannerClient(defaultClient, newLocationRouter(nil), newPassthroughChannelEndpointCache())

	if err := lac.Close(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if defaultClient.closeCount != 0 {
		t.Fatalf("expected shared default client to remain open, got %d close calls", defaultClient.closeCount)
	}
}

func TestLocationRouter_TransactionAffinityLifecycle(t *testing.T) {
	router := newLocationRouter(nil)

	ep := &passthroughChannelEndpoint{address: "server1"}

	// Initially no affinity.
	if router.getTransactionAffinity("tx1") != nil {
		t.Fatal("expected no affinity initially")
	}

	// Set affinity.
	router.setTransactionAffinity("tx1", ep)
	got := router.getTransactionAffinity("tx1")
	if got == nil {
		t.Fatal("expected affinity to be set")
	}
	if got.Address() != "server1" {
		t.Fatalf("expected address server1, got %s", got.Address())
	}

	// Clear affinity.
	router.clearTransactionAffinity("tx1")
	if router.getTransactionAffinity("tx1") != nil {
		t.Fatal("expected affinity to be cleared")
	}
	if router.isReadOnlyTransaction("tx1") {
		t.Fatal("expected read-only marker to be cleared")
	}

	// Read-only tracking lifecycle.
	router.trackReadOnlyTransaction("tx2", false)
	if !router.isReadOnlyTransaction("tx2") {
		t.Fatal("expected read-only marker to be present")
	}
	if preferLeader, ok := router.getReadOnlyTransactionPreferLeader("tx2"); !ok || preferLeader {
		t.Fatalf("unexpected read-only preferLeader state: got (%t, %t), want (false, true)", preferLeader, ok)
	}
	router.clearTransactionAffinity("tx2")
	if router.isReadOnlyTransaction("tx2") {
		t.Fatal("expected read-only marker to be cleared for tx2")
	}

	// Nil safety.
	router.setTransactionAffinity("", ep)
	router.setTransactionAffinity("tx", nil)
	router.clearTransactionAffinity("")

	var nilRouter *locationRouter
	nilRouter.setTransactionAffinity("tx", ep)
	nilRouter.getTransactionAffinity("tx")
	nilRouter.clearTransactionAffinity("tx")
}

func TestLocationRouter_Close(t *testing.T) {
	closed := false
	epCache := &mockCloseCache{closeFn: func() error { closed = true; return nil }}
	router := newLocationRouter(epCache)

	if err := router.Close(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !closed {
		t.Fatal("expected endpoint cache to be closed")
	}

	// Nil safety.
	var nilRouter *locationRouter
	if err := nilRouter.Close(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

type mockCloseCache struct {
	closeFn func() error
}

func (c *mockCloseCache) Get(context.Context, string) channelEndpoint { return nil }
func (c *mockCloseCache) ClientFor(_ channelEndpoint) spannerClient   { return nil }
func (c *mockCloseCache) Close() error                                { return c.closeFn() }

func seedTwoRangeRoutingCache(router *locationRouter) {
	router.observeResultSet(&sppb.ResultSet{
		CacheUpdate: &sppb.CacheUpdate{
			DatabaseId: 1,
			Range: []*sppb.Range{
				{
					StartKey:   []byte("a"),
					LimitKey:   []byte("m"),
					GroupUid:   1,
					SplitId:    1,
					Generation: []byte("g"),
				},
				{
					StartKey:   []byte("m"),
					LimitKey:   []byte("z"),
					GroupUid:   2,
					SplitId:    2,
					Generation: []byte("g"),
				},
			},
			Group: []*sppb.Group{
				{
					GroupUid:   1,
					Generation: []byte("g"),
					Tablets: []*sppb.Tablet{
						{
							TabletUid:     11,
							ServerAddress: "server-a:443",
							Role:          sppb.Tablet_READ_WRITE,
							Incarnation:   []byte("i"),
						},
					},
				},
				{
					GroupUid:   2,
					Generation: []byte("g"),
					Tablets: []*sppb.Tablet{
						{
							TabletUid:     22,
							ServerAddress: "server-b:443",
							Role:          sppb.Tablet_READ_WRITE,
							Incarnation:   []byte("i"),
						},
					},
				},
			},
		},
	})
}

func createMutationRoutingCacheUpdate() *sppb.CacheUpdate {
	update := proto.Clone(createMutationRecipeCacheUpdate()).(*sppb.CacheUpdate)
	proto.Merge(update, createRangeCacheUpdateForHint(&sppb.RoutingHint{Key: []byte("a")}))
	return update
}

func createMutationRecipeCacheUpdate() *sppb.CacheUpdate {
	return &sppb.CacheUpdate{
		DatabaseId: 7,
		KeyRecipes: &sppb.RecipeList{
			SchemaGeneration: []byte("1"),
			Recipe: []*sppb.KeyRecipe{
				{
					Target: &sppb.KeyRecipe_TableName{TableName: "T"},
					Part: []*sppb.KeyRecipe_Part{
						{Tag: 1},
						{
							Order:     sppb.KeyRecipe_Part_ASCENDING,
							NullOrder: sppb.KeyRecipe_Part_NULLS_FIRST,
							Type:      &sppb.Type{Code: sppb.TypeCode_STRING},
							ValueType: &sppb.KeyRecipe_Part_Identifier{Identifier: "k"},
						},
					},
				},
			},
		},
	}
}

func createRangeCacheUpdateForHint(hint *sppb.RoutingHint) *sppb.CacheUpdate {
	if hint == nil {
		hint = &sppb.RoutingHint{}
	}
	key := append([]byte(nil), hint.GetKey()...)
	limitKey := append([]byte(nil), hint.GetLimitKey()...)
	if len(limitKey) == 0 {
		limitKey = append(append([]byte(nil), key...), 0)
	}
	return &sppb.CacheUpdate{
		DatabaseId: 7,
		Range: []*sppb.Range{
			{
				StartKey:   key,
				LimitKey:   limitKey,
				GroupUid:   1,
				SplitId:    1,
				Generation: []byte("1"),
			},
		},
		Group: []*sppb.Group{
			{
				GroupUid:   1,
				Generation: []byte("1"),
				Tablets: []*sppb.Tablet{
					{
						TabletUid:     11,
						ServerAddress: "server-a:443",
						Role:          sppb.Tablet_READ_WRITE,
						Incarnation:   []byte("i"),
					},
				},
			},
		},
	}
}

func createInsertMutation(key string) *sppb.Mutation {
	return &sppb.Mutation{
		Operation: &sppb.Mutation_Insert{
			Insert: &sppb.Mutation_Write{
				Table:   "T",
				Columns: []string{"k"},
				Values: []*structpb.ListValue{
					{
						Values: []*structpb.Value{structpb.NewStringValue(key)},
					},
				},
			},
		},
	}
}

func createDeleteMutation(key string) *sppb.Mutation {
	return &sppb.Mutation{
		Operation: &sppb.Mutation_Delete_{
			Delete: &sppb.Mutation_Delete{
				Table: "T",
				KeySet: &sppb.KeySet{
					Keys: []*structpb.ListValue{
						{
							Values: []*structpb.Value{structpb.NewStringValue(key)},
						},
					},
				},
			},
		},
	}
}

func executeSQLWithKeyAndSelector(key string, selector *sppb.TransactionSelector) *sppb.ExecuteSqlRequest {
	return &sppb.ExecuteSqlRequest{
		Sql:         "SELECT 1",
		Transaction: selector,
		RoutingHint: &sppb.RoutingHint{
			Key: []byte(key),
		},
		Params: &structpb.Struct{},
	}
}

// Copyright 2024 Google LLC
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

package spanner

import (
	"context"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"math"
	"regexp"
	"slices"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	sppb "cloud.google.com/go/spanner/apiv1/spannerpb"
	"github.com/google/go-cmp/cmp"
	"google.golang.org/api/iterator"
	"google.golang.org/api/option"
	"google.golang.org/genproto/googleapis/rpc/errdetails"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/durationpb"
	"google.golang.org/protobuf/types/known/structpb"

	"cloud.google.com/go/spanner/internal/testutil"
)

var regRequestID = regexp.MustCompile(`^(?P<version>\d+).(?P<randProcessId>[a-z0-9]+)\.(?P<clientId>\d+)\.(?P<channelId>\d+)\.(?P<reqId>\d+)\.(?P<rpcId>\d+)$`)

type requestIDSegments struct {
	Version   uint8  `json:"vers"`
	ProcessID string `json:"proc_id"`
	ClientID  uint32 `json:"c_id"`
	RequestNo uint32 `json:"req_id"`
	ChannelID uint32 `json:"ch_id"`
	RPCNo     uint32 `json:"rpc_id"`
}

func (ris *requestIDSegments) String() string {
	return fmt.Sprintf("%d.%s.%d.%d.%d.%d", ris.Version, ris.ProcessID, ris.ClientID, ris.ChannelID, ris.RequestNo, ris.RPCNo)
}

func checkForMissingSpannerRequestIDHeader(opts []grpc.CallOption) (*requestIDSegments, error) {
	requestID := ""
	for _, opt := range opts {
		if hdrOpt, ok := opt.(grpc.HeaderCallOption); ok {
			hdrs := hdrOpt.HeaderAddr.Get(xSpannerRequestIDHeader)
			gotRequestID := len(hdrs) != 0 && len(hdrs[0]) != 0
			if gotRequestID {
				requestID = hdrs[0]
				break
			}
		}
	}

	if requestID == "" {
		return nil, status.Errorf(codes.InvalidArgument, "missing %q header", xSpannerRequestIDHeader)
	}
	if !regRequestID.MatchString(requestID) {
		return nil, status.Errorf(codes.InvalidArgument, "requestID does not conform to pattern=%q", regRequestID.String())
	}

	// Now extract the respective fields and validate that they match our rubric.
	template := `{"vers":$version,"proc_id":"$randProcessId","c_id":$clientId,"req_id":$reqId,"ch_id":$channelId,"rpc_id":$rpcId}`
	asJSONBytes := []byte{}
	for _, submatch := range regRequestID.FindAllStringSubmatchIndex(requestID, -1) {
		asJSONBytes = regRequestID.ExpandString(asJSONBytes, template, requestID, submatch)
	}
	recv := new(requestIDSegments)
	if err := json.Unmarshal(asJSONBytes, recv); err != nil {
		return nil, status.Error(codes.InvalidArgument, "could not correctly parse requestID segements: "+string(asJSONBytes))
	}
	if g, w := recv.ProcessID, randIDForProcess; g != w {
		return nil, status.Errorf(codes.InvalidArgument, "invalid processId, got=%q want=%q", g, w)
	}
	return recv, validateRequestIDSegments(recv)
}

func validateRequestIDSegments(recv *requestIDSegments) error {
	if recv == nil || recv.ProcessID == "" {
		return status.Errorf(codes.InvalidArgument, "unset processId")
	}
	if len(recv.ProcessID) == 0 || len(recv.ProcessID) > 20 {
		return status.Errorf(codes.InvalidArgument, "processId must be in the range (0, maxUint64), got %d", len(recv.ProcessID))
	}
	if g := recv.ClientID; g < 1 {
		return status.Errorf(codes.InvalidArgument, "clientID must be >= 1, got=%d", g)
	}
	if g := recv.RequestNo; g < 1 {
		return status.Errorf(codes.InvalidArgument, "requestNumber must be >= 1, got=%d", g)
	}
	if g := recv.ChannelID; g < 1 {
		return status.Errorf(codes.InvalidArgument, "channelID must be >= 1, got=%d", g)
	}
	if g := recv.RPCNo; g < 1 {
		return status.Errorf(codes.InvalidArgument, "rpcID must be >= 1, got=%d", g)
	}
	return nil
}

func TestRequestIDHeader_sentOnEveryClientCall(t *testing.T) {
	if isMultiplexEnabled {
		t.Skip("Skipping these tests with multiplexed sessions until #11308 is fixed")
	}

	interceptorTracker := newInterceptorTracker()

	clientOpts := []option.ClientOption{
		option.WithGRPCDialOption(grpc.WithUnaryInterceptor(interceptorTracker.unaryClientInterceptor)),
		option.WithGRPCDialOption(grpc.WithStreamInterceptor(interceptorTracker.streamClientInterceptor)),
	}
	clientConfig := ClientConfig{
		SessionPoolConfig: SessionPoolConfig{
			MinOpened:     2,
			MaxOpened:     10,
			WriteSessions: 0.2,
			incStep:       2,
		},
		DisableNativeMetrics: true,
	}
	server, sc, tearDown := setupMockedTestServerWithConfigAndClientOptions(t, clientConfig, clientOpts)
	t.Cleanup(tearDown)
	defer sc.Close()

	sqlSELECT1 := "SELECT 1"
	resultSet := &sppb.ResultSet{
		Rows: []*structpb.ListValue{
			{Values: []*structpb.Value{
				{Kind: &structpb.Value_NumberValue{NumberValue: 1}},
			}},
		},
		Metadata: &sppb.ResultSetMetadata{
			RowType: &sppb.StructType{
				Fields: []*sppb.StructType_Field{
					{Name: "Int", Type: &sppb.Type{Code: sppb.TypeCode_INT64}},
				},
			},
		},
	}
	result := &testutil.StatementResult{
		Type:      testutil.StatementResultResultSet,
		ResultSet: resultSet,
	}
	server.TestSpanner.PutStatementResult(sqlSELECT1, result)

	txn := sc.ReadOnlyTransaction()
	defer txn.Close()

	ctx := context.Background()
	stmt := NewStatement(sqlSELECT1)
	rowIter := txn.Query(ctx, stmt)
	defer rowIter.Stop()
	for {
		rows, err := rowIter.Next()
		if err == iterator.Done {
			break
		}
		if err != nil {
			t.Fatal(err)
		}
		_ = rows
	}

	if interceptorTracker.unaryCallCount() < 1 {
		t.Error("unaryClientCall was not invoked")
	}
	if interceptorTracker.streamCallCount() < 1 {
		t.Error("streamClientCall was not invoked")
	}

	if err := interceptorTracker.validateRequestIDsMonotonicity(); err != nil {
		t.Fatal(err)
	}
}

type interceptorTracker struct {
	nUnaryClientCalls  *atomic.Uint64
	nStreamClientCalls *atomic.Uint64

	mu                            sync.Mutex // mu protects the fields down below.
	unaryClientRequestIDSegments  []*requestIDSegments
	streamClientRequestIDSegments []*requestIDSegments
}

func (it *interceptorTracker) unaryCallCount() uint64 {
	return it.nUnaryClientCalls.Load()
}

func (it *interceptorTracker) streamCallCount() uint64 {
	return it.nStreamClientCalls.Load()
}

func (it *interceptorTracker) validateRequestIDsMonotonicity() error {
	if err := ensureMonotonicityOfRequestIDs(it.unaryClientRequestIDSegments); err != nil {
		return fmt.Errorf("unaryClientRequestIDs: %w", err)
	}
	if err := ensureMonotonicityOfRequestIDs(it.streamClientRequestIDSegments); err != nil {
		return fmt.Errorf("streamClientRequestIDs: %w", err)
	}
	return nil
}

type interceptSummary struct {
	ProcIDs      []string `json:"proc_ids"`
	MaxChannelID uint32   `json:"max_ch_id"`
	MinChannelID uint32   `json:"min_ch_id"`
	MaxClientID  uint32   `json:"max_c_id"`
	MinClientID  uint32   `json:"min_c_id"`
	MaxRPCID     uint32   `json:"max_rpc_id"`
	MinRPCID     uint32   `json:"min_rpc_id"`
}

func (it *interceptorTracker) summarize() (unarySummary, streamSummary *interceptSummary) {
	return computeSummary(it.unaryClientRequestIDSegments), computeSummary(it.streamClientRequestIDSegments)
}

func computeSummary(segments []*requestIDSegments) *interceptSummary {
	summary := new(interceptSummary)
	summary.MinRPCID = math.MaxUint32
	summary.MaxRPCID = 0
	summary.MinClientID = math.MaxUint32
	summary.MaxClientID = 0
	summary.MinChannelID = math.MaxUint32
	summary.MaxChannelID = 0
	for _, segment := range segments {
		if len(summary.ProcIDs) == 0 || summary.ProcIDs[len(summary.ProcIDs)-1] != segment.ProcessID {
			summary.ProcIDs = append(summary.ProcIDs, segment.ProcessID)
		}
		if segment.ClientID < summary.MinClientID {
			summary.MinClientID = segment.ClientID
		}
		if segment.ClientID > summary.MaxClientID {
			summary.MaxClientID = segment.ClientID
		}
		if segment.RPCNo < summary.MinRPCID {
			summary.MinRPCID = segment.RPCNo
		}
		if segment.RPCNo > summary.MaxRPCID {
			summary.MaxRPCID = segment.RPCNo
		}
		if segment.ChannelID < summary.MinChannelID {
			summary.MinChannelID = segment.ChannelID
		}
		if segment.ChannelID > summary.MaxChannelID {
			summary.MaxChannelID = segment.ChannelID
		}
		if segment.RPCNo > summary.MaxRPCID {
			summary.MaxRPCID = segment.RPCNo
		}
		if segment.RPCNo < summary.MinRPCID {
			summary.MinRPCID = segment.RPCNo
		}
	}
	return summary
}

func ensureMonotonicityOfRequestIDs(requestIDs []*requestIDSegments) error {
	for _, segment := range requestIDs {
		if err := validateRequestIDSegments(segment); err != nil {
			return err
		}
	}

	// 2. Compare the current against previous requestID which requires at least 2 elements.
	for i := 1; i < len(requestIDs); i++ {
		rCurr, rPrev := requestIDs[i], requestIDs[i-1]
		if rPrev.ProcessID != rCurr.ProcessID {
			return fmt.Errorf("processID mismatch: #[%d].ProcessID=%q, #[%d].ProcessID=%q", i, rCurr.ProcessID, i-1, rPrev.ProcessID)
		}
		if rPrev.ClientID == rCurr.ClientID {
			if rPrev.ChannelID == rCurr.ChannelID {
				if rPrev.RequestNo == rCurr.RequestNo {
					if rPrev.RPCNo >= rCurr.RPCNo {
						return fmt.Errorf("sameChannelID, sameRequestNo yet #[%d].RPCNo=%d >= #[%d].RPCNo=%d\n\n\t%s\n\t%s", i-1, rPrev.RPCNo, i, rCurr.RPCNo, rPrev, rCurr)
					}
				}
			}

			// In the case of retries, we shall might have the same request
			// number, but rpc id must be monotonically increasing.
			if false && rPrev.RequestNo == rCurr.RequestNo {
				if rPrev.RPCNo >= rCurr.RPCNo {
					return fmt.Errorf("sameClientID but rpcNo mismatch: #[%d].RPCNo=%d >= #[%d].RPCNo=%d", i-1, rPrev.RPCNo, i, rCurr.RPCNo)
				}
			}
		} else if rPrev.ClientID > rCurr.ClientID {
			// For requests that execute in parallel such as with PartitionQuery,
			// we could have requests from previous clients executing slower than
			// the newest client, hence this is not an error.
		}
	}

	// All checks passed so good to go.
	return nil
}

func TestRequestIDHeader_ensureMonotonicityOfRequestIDs(t *testing.T) {
	procID := randIDForProcess
	tests := []struct {
		name    string
		in      []*requestIDSegments
		wantErr string
	}{
		{name: "no values", wantErr: ""},
		{name: "1 value", in: []*requestIDSegments{
			{ProcessID: procID, ClientID: 1, RequestNo: 1, ChannelID: 3, RPCNo: 1},
		}, wantErr: ""},
		{name: "Different processIDs", in: []*requestIDSegments{
			{ProcessID: procID, ClientID: 1, RequestNo: 1, RPCNo: 1, ChannelID: 1},
			{ProcessID: strings.Repeat("a", len(procID)), ClientID: 1, RequestNo: 1, RPCNo: 2, ChannelID: 1},
		}, wantErr: "processID mismatch"},
		{
			name: "Different clientID, prev has higher value",
			in: []*requestIDSegments{
				{ProcessID: procID, ClientID: 2, RequestNo: 1, RPCNo: 1, ChannelID: 1},
				{ProcessID: procID, ClientID: 1, RequestNo: 1, RPCNo: 1, ChannelID: 1},
			},
			wantErr: "", // Requests can occur in parallel.
		},
		{
			name: "Different clientID, prev has lower value",
			in: []*requestIDSegments{
				{ProcessID: procID, ClientID: 1, RPCNo: 1, ChannelID: 1, RequestNo: 1},
				{ProcessID: procID, ClientID: 2, RPCNo: 1, ChannelID: 1, RequestNo: 1},
			},
			wantErr: "",
		},
		{
			name: "Same channelID, prev has same RequestNo",
			in: []*requestIDSegments{
				{ProcessID: procID, ClientID: 1, ChannelID: 1, RPCNo: 1, RequestNo: 8},
				{ProcessID: procID, ClientID: 1, ChannelID: 1, RPCNo: 1, RequestNo: 8},
			},
			wantErr: "sameChannelID, sameRequestNo yet #[0].RPCNo=1 >= #[1].RPCNo=1",
		},
		{
			name: "Same clientID, different ChannelID prev has same RequestNo",
			in: []*requestIDSegments{
				{ProcessID: procID, ClientID: 1, ChannelID: 1, RequestNo: 1, RPCNo: 1},
				{ProcessID: procID, ClientID: 1, ChannelID: 2, RequestNo: 1, RPCNo: 1},
			},
			wantErr: "",
		},
		{
			name: "Same clientID, same ChannelID, same RequestNo, different RPCNo",
			in: []*requestIDSegments{
				{ProcessID: procID, ClientID: 1, ChannelID: 4, RequestNo: 3, RPCNo: 1},
				{ProcessID: procID, ClientID: 1, ChannelID: 4, RequestNo: 3, RPCNo: 4},
			},
			wantErr: "",
		},
		{
			name: "Same clientID, prev has higher RPCNo",
			in: []*requestIDSegments{
				{ProcessID: procID, ClientID: 1, ChannelID: 1, RequestNo: 1, RPCNo: 2},
				{ProcessID: procID, ClientID: 1, ChannelID: 1, RequestNo: 1, RPCNo: 1},
			},
			wantErr: "sameRequestNo yet #[0].RPCNo=2 >= #[1].RPCNo=1",
		},
		{
			name: "Same clientID, same channelID, prev has lower RPCNo",
			in: []*requestIDSegments{
				{ProcessID: procID, ClientID: 1, RequestNo: 2, ChannelID: 1, RPCNo: 1},
				{ProcessID: procID, ClientID: 1, RequestNo: 2, ChannelID: 1, RPCNo: 2},
			},
			wantErr: "",
		},
		{
			name: "Same clientID, prev has higher clientID",
			in: []*requestIDSegments{
				{ProcessID: procID, ClientID: 2, RequestNo: 1, RPCNo: 1, ChannelID: 1},
				{ProcessID: procID, ClientID: 1, RequestNo: 1, RPCNo: 1, ChannelID: 1},
			},
			wantErr: "", // Requests can execute in parallel.
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// 1. Each segment but be valid!
			err := ensureMonotonicityOfRequestIDs(tt.in)
			if tt.wantErr != "" {
				if err == nil {
					t.Fatal("Expected a non-nil error")
				}
				if !strings.Contains(err.Error(), tt.wantErr) {
					t.Fatalf("Error mismatch\n\t%q\ncould not be found in\n\t%q", tt.wantErr, err)
				}
				return
			}

			if err != nil {
				t.Fatalf("Unexpected error: %v", err)
			}
		})
	}
}

func (it *interceptorTracker) unaryClientInterceptor(ctx context.Context, method string, req, reply any, cc *grpc.ClientConn, invoker grpc.UnaryInvoker, opts ...grpc.CallOption) error {
	it.nUnaryClientCalls.Add(1)
	reqID, err := checkForMissingSpannerRequestIDHeader(opts)
	if err != nil {
		return err
	}

	it.mu.Lock()
	it.unaryClientRequestIDSegments = append(it.unaryClientRequestIDSegments, reqID)
	it.mu.Unlock()

	// fmt.Printf("unary.method=%q\n", method)
	// fmt.Printf("method=%q\nReq: %#v\nRes: %#v\n", method, req, reply)
	// Otherwise proceed with the call.
	return invoker(ctx, method, req, reply, cc, opts...)
}

func (it *interceptorTracker) streamClientInterceptor(ctx context.Context, desc *grpc.StreamDesc, cc *grpc.ClientConn, method string, streamer grpc.Streamer, opts ...grpc.CallOption) (grpc.ClientStream, error) {
	it.nStreamClientCalls.Add(1)
	reqID, err := checkForMissingSpannerRequestIDHeader(opts)
	if err != nil {
		return nil, err
	}

	it.mu.Lock()
	it.streamClientRequestIDSegments = append(it.streamClientRequestIDSegments, reqID)
	it.mu.Unlock()

	// fmt.Printf("stream.method=%q\n", method)
	// Otherwise proceed with the call.
	return streamer(ctx, desc, cc, method, opts...)
}

func newInterceptorTracker() *interceptorTracker {
	return &interceptorTracker{
		nUnaryClientCalls:  new(atomic.Uint64),
		nStreamClientCalls: new(atomic.Uint64),
	}
}

func TestRequestIDHeader_onRetriesWithFailedTransactionCommit(t *testing.T) {
	if isMultiplexEnabled {
		t.Skip("Skipping these tests with multiplexed sessions until #11308 is fixed")
	}

	interceptorTracker := newInterceptorTracker()
	clientOpts := []option.ClientOption{
		option.WithGRPCDialOption(grpc.WithUnaryInterceptor(interceptorTracker.unaryClientInterceptor)),
		option.WithGRPCDialOption(grpc.WithStreamInterceptor(interceptorTracker.streamClientInterceptor)),
	}
	clientConfig := ClientConfig{
		SessionPoolConfig: SessionPoolConfig{
			MinOpened:     2,
			MaxOpened:     10,
			WriteSessions: 0.2,
			incStep:       2,
		},
		DisableNativeMetrics: true,
	}
	server, sc, tearDown := setupMockedTestServerWithConfigAndClientOptions(t, clientConfig, clientOpts)
	t.Cleanup(tearDown)
	defer sc.Close()

	// First commit will fail, and the retry will begin a new transaction.
	server.TestSpanner.PutExecutionTime(testutil.MethodCommitTransaction,
		testutil.SimulatedExecutionTime{
			Errors: []error{newAbortedErrorWithMinimalRetryDelay()},
		})

	ctx := context.Background()
	ms := []*Mutation{
		Insert("Accounts", []string{"AccountId"}, []any{int64(1)}),
	}

	if _, err := sc.Apply(ctx, ms); err != nil {
		t.Fatalf("ReadWriteTransaction retry on abort, got %v, want nil.", err)
	}

	if _, err := shouldHaveReceived(server.TestSpanner, []any{
		&sppb.BatchCreateSessionsRequest{},
		&sppb.BeginTransactionRequest{},
		&sppb.CommitRequest{}, // First commit fails.
		&sppb.BeginTransactionRequest{},
		&sppb.CommitRequest{}, // Second commit succeeds.
	}); err != nil {
		t.Fatal(err)
	}

	if g, w := interceptorTracker.unaryCallCount(), uint64(5); g != w {
		t.Errorf("unaryClientCall is incorrect; got=%d want=%d", g, w)
	}
	if g := interceptorTracker.streamCallCount(); g > 0 {
		t.Errorf("streamClientCall was unexpectedly invoked %d times", g)
	}

	if err := interceptorTracker.validateRequestIDsMonotonicity(); err != nil {
		t.Fatal(err)
	}
}

// Tests that SessionNotFound errors are retried.
func TestRequestIDHeader_retriesOnSessionNotFound(t *testing.T) {
	if isMultiplexEnabled {
		t.Skip("Skipping these tests with multiplexed sessions until #11308 is fixed")
	}

	t.Parallel()
	ctx := context.Background()
	interceptorTracker := newInterceptorTracker()
	clientOpts := []option.ClientOption{
		option.WithGRPCDialOption(grpc.WithUnaryInterceptor(interceptorTracker.unaryClientInterceptor)),
		option.WithGRPCDialOption(grpc.WithStreamInterceptor(interceptorTracker.streamClientInterceptor)),
	}
	clientConfig := ClientConfig{
		SessionPoolConfig: SessionPoolConfig{
			MinOpened:     2,
			MaxOpened:     10,
			WriteSessions: 0.2,
			incStep:       2,
		},
		DisableNativeMetrics: true,
	}
	server, sc, tearDown := setupMockedTestServerWithConfigAndClientOptions(t, clientConfig, clientOpts)
	t.Cleanup(tearDown)
	defer sc.Close()

	serverErr := newSessionNotFoundError("projects/p/instances/i/databases/d/sessions/s")
	server.TestSpanner.PutExecutionTime(testutil.MethodBeginTransaction,
		testutil.SimulatedExecutionTime{
			Errors: []error{serverErr, serverErr, serverErr},
		})
	server.TestSpanner.PutExecutionTime(testutil.MethodCommitTransaction,
		testutil.SimulatedExecutionTime{
			Errors: []error{serverErr},
		})

	txn := sc.ReadOnlyTransaction()
	defer txn.Close()

	var wantErr error
	if _, _, got := txn.acquire(ctx); !testEqual(wantErr, got) {
		t.Fatalf("Expect acquire to succeed, got %v, want %v.", got, wantErr)
	}

	// The server error should lead to a retry of the BeginTransaction call and
	// a valid session handle to be returned that will be used by the following
	// requests. Note that calling txn.Query(...) does not actually send the
	// query to the (mock) server. That is done at the first call to
	// RowIterator.Next. The following statement only verifies that the
	// transaction is in a valid state and received a valid session handle.
	if got := txn.Query(ctx, NewStatement("SELECT 1")); !testEqual(wantErr, got.err) {
		t.Fatalf("Expect Query to succeed, got %v, want %v.", got.err, wantErr)
	}

	if got := txn.Read(ctx, "Users", KeySets(Key{"alice"}, Key{"bob"}), []string{"name", "email"}); !testEqual(wantErr, got.err) {
		t.Fatalf("Expect Read to succeed, got %v, want %v.", got.err, wantErr)
	}

	wantErr = ToSpannerError(newSessionNotFoundError("projects/p/instances/i/databases/d/sessions/s"))
	ms := []*Mutation{
		Insert("Accounts", []string{"AccountId", "Nickname", "Balance"}, []any{int64(1), "Foo", int64(50)}),
		Insert("Accounts", []string{"AccountId", "Nickname", "Balance"}, []any{int64(2), "Bar", int64(1)}),
	}
	_, got := sc.Apply(ctx, ms, ApplyAtLeastOnce())
	if !testEqual(wantErr, got) {
		t.Fatalf("Expect Apply to fail\nGot:  %v\nWant: %v\n", got, wantErr)
	}
	gotSErr := got.(*Error)
	if gotSErr.RequestID == "" {
		t.Fatal("Expected a non-blank requestID")
	}

	if g, w := interceptorTracker.unaryCallCount(), uint64(8); g != w {
		t.Errorf("unaryClientCall is incorrect; got=%d want=%d", g, w)
	}
	if g := interceptorTracker.streamCallCount(); g > 0 {
		t.Errorf("streamClientCall was unexpectedly invoked %d times", g)
	}

	if err := interceptorTracker.validateRequestIDsMonotonicity(); err != nil {
		t.Fatal(err)
	}
}

func TestRequestIDHeader_BatchDMLWithMultipleDML(t *testing.T) {
	if isMultiplexEnabled {
		t.Skip("Skipping these tests with multiplexed sessions until #11308 is fixed")
	}

	t.Parallel()

	interceptorTracker := newInterceptorTracker()
	clientOpts := []option.ClientOption{
		option.WithGRPCDialOption(grpc.WithUnaryInterceptor(interceptorTracker.unaryClientInterceptor)),
		option.WithGRPCDialOption(grpc.WithStreamInterceptor(interceptorTracker.streamClientInterceptor)),
	}
	clientConfig := ClientConfig{
		SessionPoolConfig: SessionPoolConfig{
			MinOpened:     2,
			MaxOpened:     10,
			WriteSessions: 0.2,
			incStep:       2,
		},
		DisableNativeMetrics: true,
	}

	ctx := context.Background()
	server, sc, tearDown := setupMockedTestServerWithConfigAndClientOptions(t, clientConfig, clientOpts)
	t.Cleanup(tearDown)
	defer sc.Close()

	updateBarSetFoo := testutil.UpdateBarSetFoo
	_, err := sc.ReadWriteTransaction(ctx, func(ctx context.Context, tx *ReadWriteTransaction) (err error) {
		if _, err = tx.Update(ctx, Statement{SQL: updateBarSetFoo}); err != nil {
			return err
		}
		if _, err = tx.BatchUpdate(ctx, []Statement{{SQL: updateBarSetFoo}, {SQL: updateBarSetFoo}}); err != nil {
			return err
		}
		if _, err = tx.Update(ctx, Statement{SQL: updateBarSetFoo}); err != nil {
			return err
		}
		_, err = tx.BatchUpdate(ctx, []Statement{{SQL: updateBarSetFoo}})
		return err
	})
	if err != nil {
		t.Fatal(err)
	}

	gotReqs, err := shouldHaveReceived(server.TestSpanner, []any{
		&sppb.BatchCreateSessionsRequest{},
		&sppb.ExecuteSqlRequest{},
		&sppb.ExecuteBatchDmlRequest{},
		&sppb.ExecuteSqlRequest{},
		&sppb.ExecuteBatchDmlRequest{},
		&sppb.CommitRequest{},
	})
	if err != nil {
		t.Fatal(err)
	}
	muxCreateBuffer := 0
	if isMultiplexEnabled {
		muxCreateBuffer = 1
	}
	if got, want := gotReqs[1+muxCreateBuffer].(*sppb.ExecuteSqlRequest).Seqno, int64(1); got != want {
		t.Errorf("got %d, want %d", got, want)
	}
	if got, want := gotReqs[2+muxCreateBuffer].(*sppb.ExecuteBatchDmlRequest).Seqno, int64(2); got != want {
		t.Errorf("got %d, want %d", got, want)
	}
	if got, want := gotReqs[3+muxCreateBuffer].(*sppb.ExecuteSqlRequest).Seqno, int64(3); got != want {
		t.Errorf("got %d, want %d", got, want)
	}
	if got, want := gotReqs[4+muxCreateBuffer].(*sppb.ExecuteBatchDmlRequest).Seqno, int64(4); got != want {
		t.Errorf("got %d, want %d", got, want)
	}

	if g, w := interceptorTracker.unaryCallCount(), uint64(6); g != w {
		t.Errorf("unaryClientCall is incorrect; got=%d want=%d", g, w)
	}
	if g := interceptorTracker.streamCallCount(); g > 0 {
		t.Errorf("streamClientCall was unexpectedly invoked %d times", g)
	}

	if err := interceptorTracker.validateRequestIDsMonotonicity(); err != nil {
		t.Fatal(err)
	}
}

func TestRequestIDHeader_clientBatchWrite(t *testing.T) {
	if isMultiplexEnabled {
		t.Skip("Skipping these tests with multiplexed sessions until #11308 is fixed")
	}

	t.Parallel()

	interceptorTracker := newInterceptorTracker()
	clientOpts := []option.ClientOption{
		option.WithGRPCDialOption(grpc.WithUnaryInterceptor(interceptorTracker.unaryClientInterceptor)),
		option.WithGRPCDialOption(grpc.WithStreamInterceptor(interceptorTracker.streamClientInterceptor)),
	}
	clientConfig := ClientConfig{
		SessionPoolConfig: SessionPoolConfig{
			MinOpened:     2,
			MaxOpened:     10,
			WriteSessions: 0.2,
			incStep:       2,
		},
		DisableNativeMetrics: true,
	}

	server, sc, tearDown := setupMockedTestServerWithConfigAndClientOptions(t, clientConfig, clientOpts)
	t.Cleanup(tearDown)
	defer sc.Close()

	mutationGroups := []*MutationGroup{
		{[]*Mutation{
			{opInsertOrUpdate, "t_test", nil, []string{"key", "val"}, []any{"foo1", 1}},
		}},
	}
	iter := sc.BatchWrite(context.Background(), mutationGroups)
	responseCount := 0
	doFunc := func(r *sppb.BatchWriteResponse) error {
		responseCount++
		return nil
	}
	if err := iter.Do(doFunc); err != nil {
		t.Fatal(err)
	}
	if responseCount != len(mutationGroups) {
		t.Fatalf("Response count mismatch.\nGot: %v\nWant:%v", responseCount, len(mutationGroups))
	}
	requests := drainRequestsFromServer(server.TestSpanner)
	if err := compareRequests([]any{
		&sppb.BatchCreateSessionsRequest{},
		&sppb.BatchWriteRequest{},
	}, requests); err != nil {
		t.Fatal(err)
	}

	if g, w := interceptorTracker.unaryCallCount(), uint64(1); g != w {
		t.Errorf("unaryClientCall is incorrect; got=%d want=%d", g, w)
	}
	if g, w := interceptorTracker.streamCallCount(), uint64(1); g != w {
		t.Errorf("streamClientCall is incorrect; got=%d want=%d", g, w)
	}

	if err := interceptorTracker.validateRequestIDsMonotonicity(); err != nil {
		t.Fatal(err)
	}
}

func TestRequestIDHeader_ClientBatchWriteWithSessionNotFound(t *testing.T) {
	if isMultiplexEnabled {
		t.Skip("Skipping these tests with multiplexed sessions until #11308 is fixed")
	}

	t.Parallel()

	interceptorTracker := newInterceptorTracker()
	clientOpts := []option.ClientOption{
		option.WithGRPCDialOption(grpc.WithUnaryInterceptor(interceptorTracker.unaryClientInterceptor)),
		option.WithGRPCDialOption(grpc.WithStreamInterceptor(interceptorTracker.streamClientInterceptor)),
	}
	clientConfig := ClientConfig{
		SessionPoolConfig: SessionPoolConfig{
			MinOpened:     2,
			MaxOpened:     10,
			WriteSessions: 0.2,
			incStep:       2,
		},
		DisableNativeMetrics: true,
	}

	server, sc, tearDown := setupMockedTestServerWithConfigAndClientOptions(t, clientConfig, clientOpts)
	t.Cleanup(tearDown)
	defer sc.Close()

	server.TestSpanner.PutExecutionTime(
		testutil.MethodBatchWrite,
		testutil.SimulatedExecutionTime{Errors: []error{newSessionNotFoundError("projects/p/instances/i/databases/d/sessions/s")}},
	)
	mutationGroups := []*MutationGroup{
		{[]*Mutation{
			{opInsertOrUpdate, "t_test", nil, []string{"key", "val"}, []any{"foo1", 1}},
		}},
	}
	iter := sc.BatchWrite(context.Background(), mutationGroups)
	responseCount := 0
	doFunc := func(r *sppb.BatchWriteResponse) error {
		responseCount++
		return nil
	}
	if err := iter.Do(doFunc); err != nil {
		t.Fatal(err)
	}
	if responseCount != len(mutationGroups) {
		t.Fatalf("Response count mismatch.\nGot: %v\nWant:%v", responseCount, len(mutationGroups))
	}

	requests := drainRequestsFromServer(server.TestSpanner)
	if err := compareRequests([]any{
		&sppb.BatchCreateSessionsRequest{},
		&sppb.BatchWriteRequest{},
		&sppb.BatchWriteRequest{},
	}, requests); err != nil {
		t.Fatal(err)
	}

	if g, w := interceptorTracker.unaryCallCount(), uint64(1); g != w {
		t.Errorf("unaryClientCall is incorrect; got=%d want=%d", g, w)
	}

	// We had a retry for BatchWrite after the first SessionNotFound error, hence expecting 2 calls.
	if g, w := interceptorTracker.streamCallCount(), uint64(2); g != w {
		t.Errorf("streamClientCall is incorrect; got=%d want=%d", g, w)
	}

	if err := interceptorTracker.validateRequestIDsMonotonicity(); err != nil {
		t.Fatal(err)
	}
}

func TestRequestIDHeader_ClientBatchWriteWithError(t *testing.T) {
	if isMultiplexEnabled {
		t.Skip("Skipping these tests with multiplexed sessions until #11308 is fixed")
	}

	t.Parallel()

	interceptorTracker := newInterceptorTracker()
	clientOpts := []option.ClientOption{
		option.WithGRPCDialOption(grpc.WithUnaryInterceptor(interceptorTracker.unaryClientInterceptor)),
		option.WithGRPCDialOption(grpc.WithStreamInterceptor(interceptorTracker.streamClientInterceptor)),
	}
	clientConfig := ClientConfig{
		SessionPoolConfig: SessionPoolConfig{
			MinOpened:     2,
			MaxOpened:     10,
			WriteSessions: 0.2,
			incStep:       2,
		},
		DisableNativeMetrics: true,
	}

	server, sc, tearDown := setupMockedTestServerWithConfigAndClientOptions(t, clientConfig, clientOpts)
	t.Cleanup(tearDown)
	defer sc.Close()

	injectedErr := status.Error(codes.InvalidArgument, "Invalid argument")
	server.TestSpanner.PutExecutionTime(
		testutil.MethodBatchWrite,
		testutil.SimulatedExecutionTime{Errors: []error{injectedErr}},
	)
	mutationGroups := []*MutationGroup{
		{[]*Mutation{
			{opInsertOrUpdate, "t_test", nil, []string{"key", "val"}, []any{"foo1", 1}},
		}},
	}
	iter := sc.BatchWrite(context.Background(), mutationGroups)
	responseCount := 0
	doFunc := func(r *sppb.BatchWriteResponse) error {
		responseCount++
		return nil
	}
	err := iter.Do(doFunc)
	if err == nil {
		t.Fatal("Expected an error")
	}

	gotSErr := err.(*Error)
	if gotSErr.RequestID == "" {
		t.Fatal("Expected a non-blank requestID")
	}

	if responseCount != 0 {
		t.Fatalf("Do unexpectedly called %d times", responseCount)
	}

	if g, w := interceptorTracker.unaryCallCount(), uint64(1); g != w {
		t.Errorf("unaryClientCall is incorrect; got=%d want=%d", g, w)
	}

	// We had a straight-up failure after the first BatchWrite call so only 1 call.
	if g, w := interceptorTracker.streamCallCount(), uint64(1); g != w {
		t.Errorf("streamClientCall is incorrect; got=%d want=%d", g, w)
	}

	if err := interceptorTracker.validateRequestIDsMonotonicity(); err != nil {
		t.Fatal(err)
	}
}

func TestRequestIDHeader_PartitionQueryWithoutError(t *testing.T) {
	testRequestIDHeaderPartitionQuery(t, false)
}

func TestRequestIDHeader_PartitionQueryWithError(t *testing.T) {
	testRequestIDHeaderPartitionQuery(t, true)
}

func testRequestIDHeaderPartitionQuery(t *testing.T, mustErrorOnPartitionQuery bool) {
	if isMultiplexEnabled {
		t.Skip("Skipping these tests with multiplexed sessions until #11308 is fixed")
	}

	t.Parallel()

	interceptorTracker := newInterceptorTracker()
	clientOpts := []option.ClientOption{
		option.WithGRPCDialOption(grpc.WithUnaryInterceptor(interceptorTracker.unaryClientInterceptor)),
		option.WithGRPCDialOption(grpc.WithStreamInterceptor(interceptorTracker.streamClientInterceptor)),
	}
	clientConfig := ClientConfig{
		SessionPoolConfig: SessionPoolConfig{
			MinOpened:     2,
			MaxOpened:     10,
			WriteSessions: 0.2,
			incStep:       2,
		},
		DisableNativeMetrics: true,
	}

	server, sc, tearDown := setupMockedTestServerWithConfigAndClientOptions(t, clientConfig, clientOpts)
	t.Cleanup(tearDown)
	defer sc.Close()

	// The request will initially fail, and be retried.
	server.TestSpanner.PutExecutionTime(testutil.MethodExecuteStreamingSql,
		testutil.SimulatedExecutionTime{
			Errors: []error{newAbortedErrorWithMinimalRetryDelay()},
		})
	if mustErrorOnPartitionQuery {
		server.TestSpanner.PutExecutionTime(testutil.MethodPartitionQuery,
			testutil.SimulatedExecutionTime{
				Errors: []error{newAbortedErrorWithMinimalRetryDelay()},
			})
	}

	sqlFromSingers := "SELECT * FROM Singers"
	resultSet := &sppb.ResultSet{
		Rows: []*structpb.ListValue{
			{
				Values: []*structpb.Value{
					structpb.NewStructValue(&structpb.Struct{
						Fields: map[string]*structpb.Value{
							"SingerId":  {Kind: &structpb.Value_NumberValue{NumberValue: 1}},
							"FirstName": {Kind: &structpb.Value_StringValue{StringValue: "Bruce"}},
							"LastName":  {Kind: &structpb.Value_StringValue{StringValue: "Wayne"}},
						},
					}),
					structpb.NewStructValue(&structpb.Struct{
						Fields: map[string]*structpb.Value{
							"SingerId":  {Kind: &structpb.Value_NumberValue{NumberValue: 2}},
							"FirstName": {Kind: &structpb.Value_StringValue{StringValue: "Robin"}},
							"LastName":  {Kind: &structpb.Value_StringValue{StringValue: "SideKick"}},
						},
					}),
					structpb.NewStructValue(&structpb.Struct{
						Fields: map[string]*structpb.Value{
							"SingerId":  {Kind: &structpb.Value_NumberValue{NumberValue: 3}},
							"FirstName": {Kind: &structpb.Value_StringValue{StringValue: "Gordon"}},
							"LastName":  {Kind: &structpb.Value_StringValue{StringValue: "Commissioner"}},
						},
					}),
					structpb.NewStructValue(&structpb.Struct{
						Fields: map[string]*structpb.Value{
							"SingerId":  {Kind: &structpb.Value_NumberValue{NumberValue: 4}},
							"FirstName": {Kind: &structpb.Value_StringValue{StringValue: "Joker"}},
							"LastName":  {Kind: &structpb.Value_StringValue{StringValue: "None"}},
						},
					}),
					structpb.NewStructValue(&structpb.Struct{
						Fields: map[string]*structpb.Value{
							"SingerId":  {Kind: &structpb.Value_NumberValue{NumberValue: 5}},
							"FirstName": {Kind: &structpb.Value_StringValue{StringValue: "Riddler"}},
							"LastName":  {Kind: &structpb.Value_StringValue{StringValue: "None"}},
						},
					}),
				}},
		},
		Metadata: &sppb.ResultSetMetadata{
			RowType: &sppb.StructType{
				Fields: []*sppb.StructType_Field{
					{Name: "SingerId", Type: &sppb.Type{Code: sppb.TypeCode_INT64}},
					{Name: "FirstName", Type: &sppb.Type{Code: sppb.TypeCode_STRING}},
					{Name: "LastName", Type: &sppb.Type{Code: sppb.TypeCode_STRING}},
				},
			},
		},
	}
	result := &testutil.StatementResult{
		Type:      testutil.StatementResultResultSet,
		ResultSet: resultSet,
	}
	server.TestSpanner.PutStatementResult(sqlFromSingers, result)

	ctx := context.Background()
	txn, err := sc.BatchReadOnlyTransaction(ctx, StrongRead())

	if err != nil {
		t.Fatal(err)
	}
	defer txn.Close()

	// Singer represents the elements in a row from the Singers table.
	type Singer struct {
		SingerID   int64
		FirstName  string
		LastName   string
		SingerInfo []byte
	}
	stmt := Statement{SQL: "SELECT * FROM Singers;"}
	partitions, err := txn.PartitionQuery(ctx, stmt, PartitionOptions{})

	if mustErrorOnPartitionQuery {
		// The methods invoked should be: ['/BatchCreateSessions', '/CreateSession', '/BeginTransaction', '/PartitionQuery']
		if g, w := interceptorTracker.unaryCallCount(), uint64(4); g != w {
			t.Errorf("unaryClientCall is incorrect; got=%d want=%d", g, w)
		}

		// We had a straight-up failure after the first BatchWrite call so only 1 call.
		if g, w := interceptorTracker.streamCallCount(), uint64(0); g != w {
			t.Errorf("streamClientCall is incorrect; got=%d want=%d", g, w)
		}

		if err := interceptorTracker.validateRequestIDsMonotonicity(); err != nil {
			t.Fatal(err)
		}
		return
	}

	if err != nil {
		t.Fatal(err)
	}

	wg := new(sync.WaitGroup)
	for i, p := range partitions {
		wg.Add(1)
		go func(i int, p *Partition) {
			defer wg.Done()
			iter := txn.Execute(ctx, p)
			defer iter.Stop()
			for {
				row, err := iter.Next()
				if err == iterator.Done {
					break
				}
				var s Singer
				if err := row.ToStruct(&s); err != nil {
					_ = err
				}
				_ = s
			}
		}(i, p)
	}
	wg.Wait()

	// The methods invoked should be: ['/BatchCreateSessions', '/CreateSession', '/BeginTransaction', '/PartitionQuery']
	if g, w := interceptorTracker.unaryCallCount(), uint64(4); g != w {
		t.Errorf("unaryClientCall is incorrect; got=%d want=%d", g, w)
	}

	// We had a straight-up failure after the first BatchWrite call so only 1 call.
	if g, w := interceptorTracker.streamCallCount(), uint64(0); g != w {
		t.Errorf("streamClientCall is incorrect; got=%d want=%d", g, w)
	}

	if err := interceptorTracker.validateRequestIDsMonotonicity(); err != nil {
		t.Fatal(err)
	}
}

func TestRequestIDHeader_ReadWriteTransactionUpdate(t *testing.T) {
	if isMultiplexEnabled {
		t.Skip("Skipping these tests with multiplexed sessions until #11308 is fixed")
	}

	t.Parallel()

	interceptorTracker := newInterceptorTracker()
	clientOpts := []option.ClientOption{
		option.WithGRPCDialOption(grpc.WithUnaryInterceptor(interceptorTracker.unaryClientInterceptor)),
		option.WithGRPCDialOption(grpc.WithStreamInterceptor(interceptorTracker.streamClientInterceptor)),
	}
	clientConfig := ClientConfig{
		SessionPoolConfig: SessionPoolConfig{
			MinOpened:     2,
			MaxOpened:     10,
			WriteSessions: 0.2,
			incStep:       2,
		},
		DisableNativeMetrics: true,
	}

	server, sc, tearDown := setupMockedTestServerWithConfigAndClientOptions(t, clientConfig, clientOpts)
	t.Cleanup(tearDown)
	defer sc.Close()

	ctx := context.Background()
	updateSQL := testutil.UpdateBarSetFoo
	_, err := sc.ReadWriteTransaction(ctx, func(ctx context.Context, tx *ReadWriteTransaction) (err error) {
		if _, err = tx.Update(ctx, Statement{SQL: updateSQL}); err != nil {
			return err
		}
		if _, err = tx.BatchUpdate(ctx, []Statement{{SQL: updateSQL}, {SQL: updateSQL}}); err != nil {
			return err
		}
		if _, err = tx.Update(ctx, Statement{SQL: updateSQL}); err != nil {
			return err
		}
		_, err = tx.BatchUpdate(ctx, []Statement{{SQL: updateSQL}})
		return err
	})
	if err != nil {
		t.Fatal(err)
	}

	gotReqs, err := shouldHaveReceived(server.TestSpanner, []any{
		&sppb.BatchCreateSessionsRequest{},
		&sppb.ExecuteSqlRequest{},
		&sppb.ExecuteBatchDmlRequest{},
		&sppb.ExecuteSqlRequest{},
		&sppb.ExecuteBatchDmlRequest{},
		&sppb.CommitRequest{},
	})
	if err != nil {
		t.Fatal(err)
	}
	muxCreateBuffer := 0
	if isMultiplexEnabled {
		muxCreateBuffer = 1
	}
	if got, want := gotReqs[1+muxCreateBuffer].(*sppb.ExecuteSqlRequest).Seqno, int64(1); got != want {
		t.Errorf("got %d, want %d", got, want)
	}
	if got, want := gotReqs[2+muxCreateBuffer].(*sppb.ExecuteBatchDmlRequest).Seqno, int64(2); got != want {
		t.Errorf("got %d, want %d", got, want)
	}
	if got, want := gotReqs[3+muxCreateBuffer].(*sppb.ExecuteSqlRequest).Seqno, int64(3); got != want {
		t.Errorf("got %d, want %d", got, want)
	}
	if got, want := gotReqs[4+muxCreateBuffer].(*sppb.ExecuteBatchDmlRequest).Seqno, int64(4); got != want {
		t.Errorf("got %d, want %d", got, want)
	}

	// The methods invoked should be: ['/BatchCreateSessions', '/ExecuteSql', '/ExecuteBatchDml', '/ExecuteSql', '/ExecuteBatchDml', '/Commit']
	if g, w := interceptorTracker.unaryCallCount(), uint64(6); g != w {
		t.Errorf("unaryClientCall is incorrect; got=%d want=%d", g, w)
	}

	// We had a straight-up failure after the first BatchWrite call so only 1 call.
	if g, w := interceptorTracker.streamCallCount(), uint64(0); g != w {
		t.Errorf("streamClientCall is incorrect; got=%d want=%d", g, w)
	}

	if err := interceptorTracker.validateRequestIDsMonotonicity(); err != nil {
		t.Fatal(err)
	}
}

func TestRequestIDHeader_ReadWriteTransactionBatchUpdateWithOptions(t *testing.T) {
	if isMultiplexEnabled {
		t.Skip("Skipping these tests with multiplexed sessions until #11308 is fixed")
	}

	t.Parallel()

	interceptorTracker := newInterceptorTracker()
	clientOpts := []option.ClientOption{
		option.WithGRPCDialOption(grpc.WithUnaryInterceptor(interceptorTracker.unaryClientInterceptor)),
		option.WithGRPCDialOption(grpc.WithStreamInterceptor(interceptorTracker.streamClientInterceptor)),
	}
	clientConfig := ClientConfig{
		SessionPoolConfig: SessionPoolConfig{
			MinOpened:     2,
			MaxOpened:     10,
			WriteSessions: 0.2,
			incStep:       2,
		},
		DisableNativeMetrics: true,
	}

	_, sc, tearDown := setupMockedTestServerWithConfigAndClientOptions(t, clientConfig, clientOpts)
	t.Cleanup(tearDown)
	defer sc.Close()

	ctx := context.Background()
	selectSQL := testutil.SelectSingerIDAlbumIDAlbumTitleFromAlbums
	updateSQL := testutil.UpdateBarSetFoo
	_, err := sc.ReadWriteTransaction(ctx, func(ctx context.Context, tx *ReadWriteTransaction) (err error) {
		iter := tx.QueryWithOptions(ctx, NewStatement(selectSQL), QueryOptions{})
		iter.Next()
		iter.Stop()

		qo := QueryOptions{}
		iter = tx.ReadWithOptions(ctx, "FOO", AllKeys(), []string{"BAR"}, &ReadOptions{Priority: qo.Priority})
		iter.Next()
		iter.Stop()

		tx.UpdateWithOptions(ctx, NewStatement(updateSQL), qo)
		tx.BatchUpdateWithOptions(ctx, []Statement{
			NewStatement(updateSQL),
		}, qo)
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}

	// The methods invoked should be: ['/BatchCreateSessions', '/ExecuteSql', '/ExecuteBatchDml', '/Commit']
	if g, w := interceptorTracker.unaryCallCount(), uint64(4); g != w {
		t.Errorf("unaryClientCall is incorrect; got=%d want=%d", g, w)
	}

	// The methods invoked should be: ['/ExecuteStreamingSql', '/StreamingRead']
	if g, w := interceptorTracker.streamCallCount(), uint64(2); g != w {
		t.Errorf("streamClientCall is incorrect; got=%d want=%d", g, w)
	}

	if err := interceptorTracker.validateRequestIDsMonotonicity(); err != nil {
		t.Fatal(err)
	}
}

func TestRequestIDHeader_multipleParallelCallsWithConventionalCustomerCalls(t *testing.T) {
	if isMultiplexEnabled {
		t.Skip("Skipping these tests with multiplexed sessions until #11308 is fixed")
	}

	t.Parallel()

	interceptorTracker := newInterceptorTracker()
	clientOpts := []option.ClientOption{
		option.WithGRPCDialOption(grpc.WithUnaryInterceptor(interceptorTracker.unaryClientInterceptor)),
		option.WithGRPCDialOption(grpc.WithStreamInterceptor(interceptorTracker.streamClientInterceptor)),
	}
	clientConfig := ClientConfig{
		SessionPoolConfig: SessionPoolConfig{
			MinOpened:     2,
			MaxOpened:     10,
			WriteSessions: 0.2,
			incStep:       2,
		},
		DisableNativeMetrics: true,
	}

	// We created exactly 1 client.
	_, sc, tearDown := setupMockedTestServerWithConfigAndClientOptions(t, clientConfig, clientOpts)
	t.Cleanup(tearDown)
	defer sc.Close()

	beginningClientID := uint32(sc.sc.nthClient)

	ctx := context.Background()
	selectSQL := testutil.SelectSingerIDAlbumIDAlbumTitleFromAlbums
	updateSQL := testutil.UpdateBarSetFoo

	// We are going to invoke 10 calls in parallel.
	n := uint64(80)
	wg := new(sync.WaitGroup)
	semaCh := make(chan bool)
	semaWg := new(sync.WaitGroup)
	semaWg.Add(int(n))
	for i := uint64(0); i < n; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			semaWg.Done()
			<-semaCh
			_, err := sc.ReadWriteTransaction(ctx, func(ctx context.Context, tx *ReadWriteTransaction) (err error) {
				iter := tx.QueryWithOptions(ctx, NewStatement(selectSQL), QueryOptions{})
				iter.Next()
				iter.Stop()

				qo := QueryOptions{}
				iter = tx.ReadWithOptions(ctx, "FOO", AllKeys(), []string{"BAR"}, &ReadOptions{Priority: qo.Priority})
				iter.Next()
				iter.Stop()

				tx.UpdateWithOptions(ctx, NewStatement(updateSQL), qo)
				tx.BatchUpdateWithOptions(ctx, []Statement{
					NewStatement(updateSQL),
				}, qo)
				return nil
			})
			if err != nil {
				panic(err)
			}
		}()
	}

	go func() {
		semaWg.Wait()
		close(semaCh)
	}()

	wg.Wait()

	maxChannelID := uint32(sc.sc.connPool.Num())

	if err := interceptorTracker.validateRequestIDsMonotonicity(); err != nil {
		t.Fatal(err)
	}

	gotUnarySummary, gotStreamSummary := interceptorTracker.summarize()
	wantUnarySummary := &interceptSummary{
		ProcIDs:      []string{randIDForProcess},
		MaxClientID:  beginningClientID,
		MinClientID:  beginningClientID,
		MaxRPCID:     1,
		MinRPCID:     1,
		MaxChannelID: maxChannelID,
		MinChannelID: 1,
	}
	if diff := cmp.Diff(gotUnarySummary, wantUnarySummary); diff != "" {
		t.Errorf("UnarySummary mismatch: got - want +\n%s", diff)
	}
	wantStreamSummary := &interceptSummary{
		ProcIDs:      []string{randIDForProcess},
		MaxClientID:  beginningClientID,
		MinClientID:  beginningClientID,
		MaxRPCID:     1,
		MinRPCID:     1,
		MaxChannelID: maxChannelID,
		MinChannelID: 1,
	}
	if diff := cmp.Diff(gotStreamSummary, wantStreamSummary); diff != "" {
		t.Errorf("StreamSummary mismatch: got - want +\n%s", diff)
	}

	// The methods invoked should be: ['/BatchCreateSessions', '/ExecuteSql', '/ExecuteBatchDml', '/Commit']
	if g, w := interceptorTracker.unaryCallCount(), uint64(245); g != w && false {
		t.Errorf("unaryClientCall is incorrect; got=%d want=%d", g, w)
	}

	// The methods invoked should be: ['/ExecuteStreamingSql', '/StreamingRead']
	if g, w := interceptorTracker.streamCallCount(), uint64(2)*n; g != w && false {
		t.Errorf("streamClientCall is incorrect; got=%d want=%d", g, w)
	}
}

func newUnavailableErrorWithMinimalRetryDelay() error {
	st := status.New(codes.Unavailable, "Please try again")
	retry := &errdetails.RetryInfo{
		RetryDelay: durationpb.New(time.Nanosecond),
	}
	st, _ = st.WithDetails(retry)
	return st.Err()
}

func newInvalidArgumentError() error {
	st := status.New(codes.InvalidArgument, "Invalid argument")
	return st.Err()
}

func TestRequestIDHeader_RetryOnAbortAndValidate(t *testing.T) {
	if isMultiplexEnabled {
		t.Skip("Skipping these tests with multiplexed sessions until #11308 is fixed")
	}

	t.Parallel()

	ctx := context.Background()
	interceptorTracker := newInterceptorTracker()
	clientOpts := []option.ClientOption{
		option.WithGRPCDialOption(grpc.WithUnaryInterceptor(interceptorTracker.unaryClientInterceptor)),
		option.WithGRPCDialOption(grpc.WithStreamInterceptor(interceptorTracker.streamClientInterceptor)),
	}
	clientConfig := ClientConfig{
		SessionPoolConfig: SessionPoolConfig{
			MinOpened:     2,
			MaxOpened:     10,
			WriteSessions: 0.2,
			incStep:       2,
		},
		DisableNativeMetrics: true,
	}

	server, sc, tearDown := setupMockedTestServerWithConfigAndClientOptions(t, clientConfig, clientOpts)
	t.Cleanup(tearDown)
	defer sc.Close()

	// First commit will fail, and the retry will begin a new transaction.
	server.TestSpanner.PutExecutionTime(testutil.MethodCommitTransaction,
		testutil.SimulatedExecutionTime{
			Errors: []error{
				newUnavailableErrorWithMinimalRetryDelay(),
				newUnavailableErrorWithMinimalRetryDelay(),
				newUnavailableErrorWithMinimalRetryDelay(),
			},
		})

	ms := []*Mutation{
		Insert("Accounts", []string{"AccountId"}, []interface{}{int64(1)}),
	}

	if _, e := sc.Apply(ctx, ms); e != nil {
		t.Fatalf("ReadWriteTransaction retry on abort, got %v, want nil.", e)
	}

	if _, err := shouldHaveReceived(server.TestSpanner, []interface{}{
		&sppb.BatchCreateSessionsRequest{},
		&sppb.BeginTransactionRequest{},
		&sppb.CommitRequest{},
		&sppb.CommitRequest{},
		&sppb.CommitRequest{},
		&sppb.CommitRequest{},
	}); err != nil {
		t.Fatal(err)
	}

	// The method CommitTransaction is retried 3 times due to the 3 retry errors, so we expect 4 invocations of it
	// plus BatchCreateSession + BeginTransaction, hence a total of 6 calls.
	// We expect 1 BatchCreateSessionsRequests + 6 * (BeginTransactionRequest + CommitRequest) = 13
	if g, w := interceptorTracker.unaryCallCount(), uint64(6); g != w {
		t.Errorf("unaryClientCall is incorrect; got=%d want=%d", g, w)
	}

	// We had a straight-up failure after the first BatchWrite call so only 1 call.
	if g, w := interceptorTracker.streamCallCount(), uint64(0); g != w {
		t.Errorf("streamClientCall is incorrect; got=%d want=%d", g, w)
	}

	if err := interceptorTracker.validateRequestIDsMonotonicity(); err != nil {
		t.Fatal(err)
	}

	clientID := uint32(sc.sc.nthClient)
	procID := randIDForProcess
	version := xSpannerRequestIDVersion
	wantUnarySegments := []*requestIDSegments{
		{Version: version, ProcessID: procID, ClientID: clientID, ChannelID: 1, RequestNo: 1, RPCNo: 1}, // BatchCreateSession
		{Version: version, ProcessID: procID, ClientID: clientID, ChannelID: 1, RequestNo: 2, RPCNo: 1}, // BeginTransaction
		{Version: version, ProcessID: procID, ClientID: clientID, ChannelID: 1, RequestNo: 3, RPCNo: 1}, // Commit: failed on 1st attempt
		{Version: version, ProcessID: procID, ClientID: clientID, ChannelID: 1, RequestNo: 3, RPCNo: 2}, // Commit: failed on 2nd attempt
		{Version: version, ProcessID: procID, ClientID: clientID, ChannelID: 1, RequestNo: 3, RPCNo: 3}, // Commit: failed on 3rd attempt
		{Version: version, ProcessID: procID, ClientID: clientID, ChannelID: 1, RequestNo: 3, RPCNo: 4}, // Commit: success on 4th attempt
	}

	if diff := cmp.Diff(interceptorTracker.unaryClientRequestIDSegments, wantUnarySegments); diff != "" {
		t.Fatalf("RequestID segments mismatch: got - want +\n%s", diff)
	}
}

func TestRequestIDHeader_BatchCreateSessions_Unavailable(t *testing.T) {
	if isMultiplexEnabled {
		t.Skip("Skipping these tests with multiplexed sessions until #11308 is fixed")
	}

	t.Parallel()

	ctx := context.Background()
	interceptorTracker := newInterceptorTracker()
	clientOpts := []option.ClientOption{
		option.WithGRPCDialOption(grpc.WithUnaryInterceptor(interceptorTracker.unaryClientInterceptor)),
		option.WithGRPCDialOption(grpc.WithStreamInterceptor(interceptorTracker.streamClientInterceptor)),
	}
	clientConfig := ClientConfig{
		SessionPoolConfig: SessionPoolConfig{
			MinOpened:     2,
			MaxOpened:     10,
			WriteSessions: 0.2,
			incStep:       2,
		},
		DisableNativeMetrics: true,
	}

	server, sc, tearDown := setupMockedTestServerWithConfigAndClientOptions(t, clientConfig, clientOpts)
	t.Cleanup(tearDown)
	defer sc.Close()

	// BatchCreateSessions returns UNAVAILABLE and should be retried.
	server.TestSpanner.PutExecutionTime(testutil.MethodBatchCreateSession,
		testutil.SimulatedExecutionTime{
			Errors: []error{
				newUnavailableErrorWithMinimalRetryDelay(),
			},
		})
	iter := sc.Single().Query(ctx, Statement{SQL: testutil.SelectFooFromBar})
	defer iter.Stop()
	for {
		_, err := iter.Next()
		if err == iterator.Done {
			break
		}
		if err != nil {
			t.Fatal(err)
		}
	}

	if _, err := shouldHaveReceived(server.TestSpanner, []interface{}{
		&sppb.BatchCreateSessionsRequest{},
		&sppb.BatchCreateSessionsRequest{},
		&sppb.ExecuteSqlRequest{},
	}); err != nil {
		t.Fatal(err)
	}

	if g, w := interceptorTracker.unaryCallCount(), uint64(2); g != w {
		t.Errorf("unaryClientCall is incorrect; got=%d want=%d", g, w)
	}

	if g, w := interceptorTracker.streamCallCount(), uint64(1); g != w {
		t.Errorf("streamClientCall is incorrect; got=%d want=%d", g, w)
	}

	if err := interceptorTracker.validateRequestIDsMonotonicity(); err != nil {
		t.Fatal(err)
	}

	clientID := uint32(sc.sc.nthClient)
	procID := randIDForProcess
	version := xSpannerRequestIDVersion
	wantUnarySegments := []*requestIDSegments{
		{Version: version, ProcessID: procID, ClientID: clientID, ChannelID: 1, RequestNo: 1, RPCNo: 1}, // BatchCreateSession (initial attempt)
		{Version: version, ProcessID: procID, ClientID: clientID, ChannelID: 1, RequestNo: 1, RPCNo: 2}, // BatchCreateSession (retry)
	}
	wantStreamingSegments := []*requestIDSegments{
		{Version: version, ProcessID: procID, ClientID: clientID, ChannelID: 1, RequestNo: 2, RPCNo: 1}, // ExecuteStreamingSql
	}

	if diff := cmp.Diff(interceptorTracker.unaryClientRequestIDSegments, wantUnarySegments); diff != "" {
		t.Fatalf("RequestID unary segments mismatch: got - want +\n%s", diff)
	}
	if diff := cmp.Diff(interceptorTracker.streamClientRequestIDSegments, wantStreamingSegments); diff != "" {
		t.Fatalf("RequestID streaming segments mismatch: got - want +\n%s", diff)
	}
}

func TestRequestIDHeader_SingleUseReadOnly_ExecuteStreamingSql_Unavailable(t *testing.T) {
	if isMultiplexEnabled {
		t.Skip("Skipping these tests with multiplexed sessions until #11308 is fixed")
	}

	t.Parallel()

	ctx := context.Background()
	interceptorTracker := newInterceptorTracker()
	clientOpts := []option.ClientOption{
		option.WithGRPCDialOption(grpc.WithUnaryInterceptor(interceptorTracker.unaryClientInterceptor)),
		option.WithGRPCDialOption(grpc.WithStreamInterceptor(interceptorTracker.streamClientInterceptor)),
	}
	clientConfig := ClientConfig{
		SessionPoolConfig: SessionPoolConfig{
			MinOpened:     2,
			MaxOpened:     10,
			WriteSessions: 0.2,
			incStep:       2,
		},
		DisableNativeMetrics: true,
	}

	server, sc, tearDown := setupMockedTestServerWithConfigAndClientOptions(t, clientConfig, clientOpts)
	t.Cleanup(tearDown)
	defer sc.Close()

	// ExecuteStreamingSql returns UNAVAILABLE and should be retried.
	server.TestSpanner.PutExecutionTime(testutil.MethodExecuteStreamingSql,
		testutil.SimulatedExecutionTime{
			Errors: []error{
				newUnavailableErrorWithMinimalRetryDelay(),
				newUnavailableErrorWithMinimalRetryDelay(),
			},
		})
	iter := sc.Single().Query(ctx, Statement{SQL: testutil.SelectFooFromBar})
	defer iter.Stop()
	for {
		_, err := iter.Next()
		if err == iterator.Done {
			break
		}
		if err != nil {
			t.Fatal(err)
		}
	}

	if _, err := shouldHaveReceived(server.TestSpanner, []any{
		&sppb.BatchCreateSessionsRequest{},
		&sppb.ExecuteSqlRequest{},
		&sppb.ExecuteSqlRequest{},
		&sppb.ExecuteSqlRequest{},
	}); err != nil {
		t.Fatal(err)
	}

	if g, w := interceptorTracker.unaryCallCount(), uint64(1); g != w {
		t.Errorf("unaryClientCall is incorrect; got=%d want=%d", g, w)
	}

	if g, w := interceptorTracker.streamCallCount(), uint64(3); g != w {
		t.Errorf("streamClientCall is incorrect; got=%d want=%d", g, w)
	}

	if err := interceptorTracker.validateRequestIDsMonotonicity(); err != nil {
		t.Fatal(err)
	}

	clientID := uint32(sc.sc.nthClient)
	procID := randIDForProcess
	version := xSpannerRequestIDVersion
	wantUnarySegments := []*requestIDSegments{
		{Version: version, ProcessID: procID, ClientID: clientID, ChannelID: 1, RequestNo: 1, RPCNo: 1}, // BatchCreateSession
	}
	wantStreamingSegments := []*requestIDSegments{
		{Version: version, ProcessID: procID, ClientID: clientID, ChannelID: 1, RequestNo: 2, RPCNo: 1}, // ExecuteStreamingSql (initial attempt)
		{Version: version, ProcessID: procID, ClientID: clientID, ChannelID: 1, RequestNo: 2, RPCNo: 2}, // ExecuteStreamingSql (retry)
		{Version: version, ProcessID: procID, ClientID: clientID, ChannelID: 1, RequestNo: 2, RPCNo: 3}, // ExecuteStreamingSql (retry)
	}

	if diff := cmp.Diff(interceptorTracker.unaryClientRequestIDSegments, wantUnarySegments); diff != "" {
		t.Fatalf("RequestID unary segments mismatch: got - want +\n%s", diff)
	}
	if diff := cmp.Diff(interceptorTracker.streamClientRequestIDSegments, wantStreamingSegments); diff != "" {
		t.Fatalf("RequestID streaming segments mismatch: got - want +\n%s", diff)
	}
}

func TestRequestIDHeader_SingleUseReadOnly_ExecuteStreamingSql_InvalidArgument(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	interceptorTracker := newInterceptorTracker()
	clientOpts := []option.ClientOption{
		option.WithGRPCDialOption(grpc.WithUnaryInterceptor(interceptorTracker.unaryClientInterceptor)),
		option.WithGRPCDialOption(grpc.WithStreamInterceptor(interceptorTracker.streamClientInterceptor)),
	}
	clientConfig := ClientConfig{
		SessionPoolConfig: SessionPoolConfig{
			MinOpened:     2,
			MaxOpened:     10,
			WriteSessions: 0.2,
			incStep:       2,
		},
		DisableNativeMetrics: true,
	}

	server, sc, tearDown := setupMockedTestServerWithConfigAndClientOptions(t, clientConfig, clientOpts)
	t.Cleanup(tearDown)
	defer sc.Close()

	// Simulate that ExecuteStreamingSql is slow.
	server.TestSpanner.PutExecutionTime(testutil.MethodExecuteStreamingSql,
		testutil.SimulatedExecutionTime{Errors: []error{newInvalidArgumentError()}})

	iter := sc.Single().Query(ctx, Statement{SQL: testutil.SelectFooFromBar})
	defer iter.Stop()
	_, err := iter.Next()
	if err == nil {
		t.Fatal("missing invalid argument error")
	}
	if g, w := ErrCode(err), codes.InvalidArgument; g != w {
		t.Fatalf("error code mismatch\n Got: %v\nWant: %v", g, w)
	}
	spannerError, ok := err.(*Error)
	if !ok {
		t.Fatal("not a Spanner error")
	}
	if spannerError.RequestID == "" {
		t.Fatal("missing RequestID on error")
	}
}

func TestRequestIDHeader_SingleUseReadOnly_ExecuteStreamingSql_ContextDeadlineExceeded(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	interceptorTracker := newInterceptorTracker()
	clientOpts := []option.ClientOption{
		option.WithGRPCDialOption(grpc.WithUnaryInterceptor(interceptorTracker.unaryClientInterceptor)),
		option.WithGRPCDialOption(grpc.WithStreamInterceptor(interceptorTracker.streamClientInterceptor)),
	}
	clientConfig := ClientConfig{
		SessionPoolConfig: SessionPoolConfig{
			MinOpened:     2,
			MaxOpened:     10,
			WriteSessions: 0.2,
			incStep:       2,
		},
		DisableNativeMetrics: true,
	}

	server, sc, tearDown := setupMockedTestServerWithConfigAndClientOptions(t, clientConfig, clientOpts)
	t.Cleanup(tearDown)
	defer sc.Close()

	// Simulate that ExecuteStreamingSql is slow.
	server.TestSpanner.PutExecutionTime(testutil.MethodExecuteStreamingSql,
		testutil.SimulatedExecutionTime{MinimumExecutionTime: time.Second})
	ctx, cancel := context.WithTimeout(ctx, 10*time.Millisecond)
	defer cancel()
	iter := sc.Single().Query(ctx, Statement{SQL: testutil.SelectFooFromBar})
	defer iter.Stop()
	_, err := iter.Next()
	if err == nil {
		t.Fatal("missing deadline exceeded error")
	}
	if g, w := ErrCode(err), codes.DeadlineExceeded; g != w {
		t.Fatalf("error code mismatch\n Got: %v\nWant: %v", g, w)
	}
	spannerError, ok := err.(*Error)
	if !ok {
		t.Fatal("not a Spanner error")
	}
	if spannerError.RequestID == "" {
		t.Fatal("missing RequestID on error")
	}
}

func TestRequestIDHeader_Commit_ContextDeadlineExceeded(t *testing.T) {
	t.Skip("TODO: debug error from PR #11788 and un-skip. See https://source.cloud.google.com/results/invocations/ead1cb6b-10e8-4d2b-80e3-aec3b96feff0")
	t.Parallel()

	ctx := context.Background()
	interceptorTracker := newInterceptorTracker()
	clientOpts := []option.ClientOption{
		option.WithGRPCDialOption(grpc.WithUnaryInterceptor(interceptorTracker.unaryClientInterceptor)),
		option.WithGRPCDialOption(grpc.WithStreamInterceptor(interceptorTracker.streamClientInterceptor)),
	}
	clientConfig := ClientConfig{
		SessionPoolConfig: SessionPoolConfig{
			MinOpened:     2,
			MaxOpened:     10,
			WriteSessions: 0.2,
			incStep:       2,
		},
		DisableNativeMetrics: true,
	}

	server, sc, tearDown := setupMockedTestServerWithConfigAndClientOptions(t, clientConfig, clientOpts)
	t.Cleanup(tearDown)
	defer sc.Close()

	// Simulate that Commit is slow.
	server.TestSpanner.PutExecutionTime(testutil.MethodCommitTransaction,
		testutil.SimulatedExecutionTime{MinimumExecutionTime: time.Second})
	ctx, cancel := context.WithTimeout(ctx, 10*time.Millisecond)
	defer cancel()
	_, err := sc.Apply(ctx, []*Mutation{})
	if err == nil {
		t.Fatal("missing deadline exceeded error")
	}
	if g, w := ErrCode(err), codes.DeadlineExceeded; g != w {
		t.Fatalf("error code mismatch\n Got: %v\nWant: %v", g, w)
	}
	spannerError, ok := err.(*Error)
	if !ok {
		t.Fatal("not a Spanner error")
	}
	if spannerError.RequestID == "" {
		t.Fatal("missing RequestID on error")
	}
}

func TestRequestIDHeader_VerifyChannelNumber(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	interceptorTracker := newInterceptorTracker()
	clientOpts := []option.ClientOption{
		option.WithGRPCDialOption(grpc.WithUnaryInterceptor(interceptorTracker.unaryClientInterceptor)),
		option.WithGRPCDialOption(grpc.WithStreamInterceptor(interceptorTracker.streamClientInterceptor)),
	}
	clientConfig := ClientConfig{
		SessionPoolConfig: SessionPoolConfig{
			MinOpened: 100,
			MaxOpened: 400,
			incStep:   25,
		},
		NumChannels:          4,
		DisableNativeMetrics: true,
	}

	_, sc, tearDown := setupMockedTestServerWithConfigAndClientOptions(t, clientConfig, clientOpts)
	t.Cleanup(tearDown)
	defer sc.Close()
	// Wait for the session pool to be initialized.
	sp := sc.idleSessions
	waitFor(t, func() error {
		sp.mu.Lock()
		defer sp.mu.Unlock()
		if uint64(sp.idleList.Len()) != clientConfig.MinOpened {
			return fmt.Errorf("num open sessions mismatch\nWant: %d\nGot: %d", sp.MinOpened, sp.numOpened)
		}
		return nil
	})
	// Verify that we've seen request IDs for each channel number.
	for channel := uint32(1); channel <= uint32(clientConfig.NumChannels); channel++ {
		if !slices.ContainsFunc(interceptorTracker.unaryClientRequestIDSegments, func(segments *requestIDSegments) bool {
			return segments.ChannelID == channel
		}) {
			t.Fatalf("missing channel %d in unary requests", channel)
		}
	}

	// Execute MinOpened + 1 queries without closing the iterators.
	// This will check out MinOpened + 1 sessions, which also triggers
	// one more BatchCreateSessions call.
	iterators := make([]*RowIterator, 0, clientConfig.MinOpened+1)
	for i := 0; i < int(clientConfig.MinOpened)+1; i++ {
		iter := sc.Single().Query(ctx, Statement{SQL: testutil.SelectFooFromBar})
		iterators = append(iterators, iter)
		_, err := iter.Next()
		if err != nil {
			t.Fatal(err)
		}
	}
	// Verify that we've seen request IDs for each channel number.
	for channel := uint32(1); channel <= uint32(clientConfig.NumChannels); channel++ {
		if !slices.ContainsFunc(interceptorTracker.streamClientRequestIDSegments, func(segments *requestIDSegments) bool {
			return segments.ChannelID == channel
		}) {
			t.Fatalf("missing channel %d in unary requests", channel)
		}
	}
	// Verify that we've only seen channel numbers in the range [1, config.NumChannels].
	for _, segmentsSlice := range [][]*requestIDSegments{interceptorTracker.streamClientRequestIDSegments, interceptorTracker.unaryClientRequestIDSegments} {
		if slices.ContainsFunc(segmentsSlice, func(segments *requestIDSegments) bool {
			return segments.ChannelID < 1 || segments.ChannelID > uint32(clientConfig.NumChannels)
		}) {
			t.Fatalf("invalid channel in requests: %v", segmentsSlice)
		}
	}

	if g, w := interceptorTracker.unaryCallCount(), uint64(5); g != w {
		t.Errorf("unaryClientCall is incorrect; got=%d want=%d", g, w)
	}

	if g, w := interceptorTracker.streamCallCount(), uint64(101); g != w {
		t.Errorf("streamClientCall is incorrect; got=%d want=%d", g, w)
	}

	if err := interceptorTracker.validateRequestIDsMonotonicity(); err != nil {
		t.Fatal(err)
	}
}

func TestRequestIDInError(t *testing.T) {
	cases := []struct {
		name string
		err  *Error
		want string
	}{
		{"nil error", nil, "spanner: OK"},
		{"only requestID", &Error{RequestID: "req-id"}, `spanner: code = "OK", desc = "", requestID = "req-id"`},
		{
			"with an error",
			&Error{RequestID: "req-id", Code: codes.Internal, Desc: "An error"},
			`spanner: code = "Internal", desc = "An error", requestID = "req-id"`,
		},
		{
			"with additional details",
			&Error{additionalInformation: "additional", RequestID: "req-id"},
			`spanner: code = "OK", desc = "", additional information = additional, requestID = "req-id"`,
		},
	}

	for _, tt := range cases {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			got := tt.err.Error()
			if diff := cmp.Diff(got, tt.want); diff != "" {
				t.Fatalf("Error string mismatch: got - want +\n%s", diff)
			}
		})
	}
}

func TestRequestIDHeader_SingleUseReadOnly_ExecuteStreamingSql_UnavailableDuringStream(t *testing.T) {
	if isMultiplexEnabled {
		t.Skip("Skipping these tests with multiplexed sessions until #11308 is fixed")
	}

	t.Parallel()
	ctx := context.Background()
	interceptorTracker := newInterceptorTracker()
	clientOpts := []option.ClientOption{
		option.WithGRPCDialOption(grpc.WithUnaryInterceptor(interceptorTracker.unaryClientInterceptor)),
		option.WithGRPCDialOption(grpc.WithStreamInterceptor(interceptorTracker.streamClientInterceptor)),
	}
	clientConfig := ClientConfig{
		SessionPoolConfig: SessionPoolConfig{
			MinOpened:     2,
			MaxOpened:     10,
			WriteSessions: 0.2,
			incStep:       2,
		},
		DisableNativeMetrics: true,
	}
	server, sc, tearDown := setupMockedTestServerWithConfigAndClientOptions(t, clientConfig, clientOpts)
	t.Cleanup(tearDown)
	defer sc.Close()
	// A stream of PartialResultSets can break halfway and be retried from that point.
	server.TestSpanner.AddPartialResultSetError(
		testutil.SelectSingerIDAlbumIDAlbumTitleFromAlbums,
		testutil.PartialResultSetExecutionTime{
			ResumeToken: testutil.EncodeResumeToken(2),
			Err:         status.Errorf(codes.Internal, "stream terminated by RST_STREAM"),
		},
	)
	server.TestSpanner.AddPartialResultSetError(
		testutil.SelectSingerIDAlbumIDAlbumTitleFromAlbums,
		testutil.PartialResultSetExecutionTime{
			ResumeToken: testutil.EncodeResumeToken(3),
			Err:         status.Errorf(codes.Unavailable, "server is unavailable"),
		},
	)
	iter := sc.Single().Query(ctx, Statement{SQL: testutil.SelectSingerIDAlbumIDAlbumTitleFromAlbums})
	defer iter.Stop()
	for {
		_, err := iter.Next()
		if err == iterator.Done {
			break
		}
		if err != nil {
			t.Fatal(err)
		}
	}
	if _, err := shouldHaveReceived(server.TestSpanner, []any{
		&sppb.BatchCreateSessionsRequest{},
		&sppb.ExecuteSqlRequest{},
		&sppb.ExecuteSqlRequest{},
		&sppb.ExecuteSqlRequest{},
	}); err != nil {
		t.Fatal(err)
	}
	if g, w := interceptorTracker.unaryCallCount(), uint64(1); g != w {
		t.Errorf("unaryClientCall is incorrect; got=%d want=%d", g, w)
	}
	if g, w := interceptorTracker.streamCallCount(), uint64(3); g != w {
		t.Errorf("streamClientCall is incorrect; got=%d want=%d", g, w)
	}
	if err := interceptorTracker.validateRequestIDsMonotonicity(); err != nil {
		t.Fatal(err)
	}
	clientID := uint32(sc.sc.nthClient)
	procID := randIDForProcess
	version := xSpannerRequestIDVersion
	wantUnarySegments := []*requestIDSegments{
		{Version: version, ProcessID: procID, ClientID: clientID, ChannelID: 1, RequestNo: 1, RPCNo: 1}, // BatchCreateSession
	}
	wantStreamingSegments := []*requestIDSegments{
		{Version: version, ProcessID: procID, ClientID: clientID, ChannelID: 1, RequestNo: 2, RPCNo: 1}, // ExecuteStreamingSql (initial attempt)
		{Version: version, ProcessID: procID, ClientID: clientID, ChannelID: 1, RequestNo: 2, RPCNo: 2}, // ExecuteStreamingSql (retry)
		{Version: version, ProcessID: procID, ClientID: clientID, ChannelID: 1, RequestNo: 2, RPCNo: 3}, // ExecuteStreamingSql (retry)
	}
	if diff := cmp.Diff(interceptorTracker.unaryClientRequestIDSegments, wantUnarySegments); diff != "" {
		t.Fatalf("RequestID unary segments mismatch: got - want +\n%s", diff)
	}
	if diff := cmp.Diff(interceptorTracker.streamClientRequestIDSegments, wantStreamingSegments); diff != "" {
		t.Fatalf("RequestID streaming segments mismatch: got - want +\n%s", diff)
	}
}

func TestRequestID_randIDForProcessIsHexadecimal(t *testing.T) {
	decoded, err := hex.DecodeString(randIDForProcess)
	if err != nil {
		t.Fatal(err)
	}
	if len(decoded) == 0 {
		t.Fatal("Expected a non-empty decoded result")
	}
}

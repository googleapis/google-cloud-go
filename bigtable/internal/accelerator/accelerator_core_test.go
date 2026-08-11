// Copyright 2026 Google LLC
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

package accelerator

import (
	"bytes"
	"context"
	"errors"
	"io"
	"testing"

	v2pb "cloud.google.com/go/bigtable/apiv2/bigtablepb"
	"cloud.google.com/go/bigtable/internal/session"
	btransport "cloud.google.com/go/bigtable/internal/transport"
	"go.opentelemetry.io/otel/metric"
	"google.golang.org/api/option"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// mockSessionTableAPI is a session.TableAPI stub the tests hand back
// from mockSessionClient.OpenTable.
type mockSessionTableAPI struct {
	mutateRowFn func(ctx context.Context, req *v2pb.SessionMutateRowRequest) (*v2pb.SessionMutateRowResponse, error)
	readRowFn   func(ctx context.Context, req *v2pb.SessionReadRowRequest) (*v2pb.SessionReadRowResponse, error)
}

func (m *mockSessionTableAPI) ReadRow(ctx context.Context, req *v2pb.SessionReadRowRequest) (*v2pb.SessionReadRowResponse, error) {
	if m.readRowFn != nil {
		return m.readRowFn(ctx, req)
	}
	return &v2pb.SessionReadRowResponse{}, nil
}

func (m *mockSessionTableAPI) MutateRow(ctx context.Context, req *v2pb.SessionMutateRowRequest) (*v2pb.SessionMutateRowResponse, error) {
	if m.mutateRowFn != nil {
		return m.mutateRowFn(ctx, req)
	}
	return &v2pb.SessionMutateRowResponse{}, nil
}

func (m *mockSessionTableAPI) Close() error { return nil }

// mockSessionClient hands back a fixed SessionTableApi for any resource and
// records the leaf arguments each Open* method was called with, plus a per-kind
// open counter, so tests can assert dispatch routing.
type mockSessionClient struct {
	table          session.TableAPI
	lastTableName  string
	newTableCalled int

	lastAVTable string
	lastAVView  string
	newAVCalled int
	lastMVView  string
	newMVCalled int
}

func (m *mockSessionClient) OpenTable(name string) session.TableAPI {
	m.lastTableName = name
	m.newTableCalled++
	return m.table
}

func (m *mockSessionClient) OpenAuthorizedView(table, view string) session.TableAPI {
	m.lastAVTable, m.lastAVView = table, view
	m.newAVCalled++
	return m.table
}

func (m *mockSessionClient) OpenMaterializedView(view string) session.TableAPI {
	m.lastMVView = view
	m.newMVCalled++
	return m.table
}

func (m *mockSessionClient) MeterProvider() metric.MeterProvider { return nil }

func (m *mockSessionClient) SessionDebug() btransport.SessionDebugProvider { return nil }
func (m *mockSessionClient) ChannelDebug() btransport.ChannelDebugProvider { return nil }
func (m *mockSessionClient) ConfigDebug() btransport.ConfigDebugProvider   { return nil }

func (m *mockSessionClient) AddSessionLoadListener(func(float64)) func() { return func() {} }

func (m *mockSessionClient) Close() error { return nil }

// stubSessionClient swaps the package-level newSessionClient seam so NewChannel
// builds a Channel backed by the given mock. Restores via t.Cleanup. Must NOT be
// used with t.Parallel — the seam is package-level state.
func stubSessionClient(t *testing.T, sc *mockSessionClient) {
	t.Helper()
	orig := newSessionClient
	newSessionClient = func(
		_ context.Context,
		_, _, _ string,
		_ ...option.ClientOption,
	) (session.Client, error) {
		return sc, nil
	}
	t.Cleanup(func() { newSessionClient = orig })
}

// newTestChannel is a convenience for tests: stubs the SessionClient factory
// and builds an Channel against the given mock.
func newTestChannel(t *testing.T, sc *mockSessionClient) *Channel {
	t.Helper()
	stubSessionClient(t, sc)
	channel, err := NewChannel(context.Background(), "p", "i", "ap")
	if err != nil {
		t.Fatalf("NewChannel error: %v", err)
	}
	// Close on cleanup so the TableCache sweeper goroutine doesn't outlive
	// the test.
	t.Cleanup(func() { _ = channel.Close() })
	return channel
}

func TestNewChannel_Constructs(t *testing.T) {
	channel := newTestChannel(t, &mockSessionClient{table: &mockSessionTableAPI{}})
	if channel.sc == nil {
		t.Error("Channel.sc is nil after construction")
	}
}

func TestInvoke_MutateRow_DispatchesThroughSession(t *testing.T) {
	called := false
	tbl := &mockSessionTableAPI{
		mutateRowFn: func(ctx context.Context, req *v2pb.SessionMutateRowRequest) (*v2pb.SessionMutateRowResponse, error) {
			called = true
			if got := string(req.Key); got != "k" {
				t.Errorf("MutateRow req.Key = %q; want %q", got, "k")
			}
			return &v2pb.SessionMutateRowResponse{}, nil
		},
	}
	sc := &mockSessionClient{table: tbl}
	channel := newTestChannel(t, sc)

	reqV2 := &v2pb.MutateRowRequest{
		TableName: "projects/p/instances/i/tables/t",
		RowKey:    []byte("k"),
	}
	if err := channel.Invoke(context.Background(),
		v2pb.Bigtable_MutateRow_FullMethodName,
		reqV2, &v2pb.MutateRowResponse{}); err != nil {
		t.Fatalf("Invoke(MutateRow) error: %v", err)
	}
	if !called {
		t.Error("Expected SessionTableApi.MutateRow to be called")
	}
	if sc.lastTableName != "t" {
		t.Errorf("Expected NewSessionTable called with leaf %q; got %q", "t", sc.lastTableName)
	}
}

func TestInvoke_MutateRow_RoutesToAuthorizedView(t *testing.T) {
	sc := &mockSessionClient{table: &mockSessionTableAPI{}}
	channel := newTestChannel(t, sc)

	reqV2 := &v2pb.MutateRowRequest{
		AuthorizedViewName: "projects/p/instances/i/tables/t/authorizedViews/v",
		RowKey:             []byte("k"),
	}
	if err := channel.Invoke(context.Background(),
		v2pb.Bigtable_MutateRow_FullMethodName,
		reqV2, &v2pb.MutateRowResponse{}); err != nil {
		t.Fatalf("Invoke(MutateRow) error: %v", err)
	}
	if sc.newAVCalled != 1 {
		t.Errorf("OpenAuthorizedView called %d times; want 1", sc.newAVCalled)
	}
	if sc.lastAVTable != "t" || sc.lastAVView != "v" {
		t.Errorf("OpenAuthorizedView(table, view) = (%q, %q); want (t, v)", sc.lastAVTable, sc.lastAVView)
	}
	if sc.newTableCalled != 0 {
		t.Errorf("OpenTable called %d times; want 0 (authorized view request)", sc.newTableCalled)
	}
}

func TestInvoke_MutateRow_RejectsOutOfScope(t *testing.T) {
	cases := []struct {
		name  string
		table string
		av    string
	}{
		{"wrong-project-table", "projects/other/instances/i/tables/t", ""},
		{"wrong-instance-table", "projects/p/instances/other/tables/t", ""},
		{"wrong-project-av", "", "projects/other/instances/i/tables/t/authorizedViews/v"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			sc := &mockSessionClient{table: &mockSessionTableAPI{}}
			channel := newTestChannel(t, sc)

			reqV2 := &v2pb.MutateRowRequest{
				TableName:          tc.table,
				AuthorizedViewName: tc.av,
				RowKey:             []byte("k"),
			}
			err := channel.Invoke(context.Background(),
				v2pb.Bigtable_MutateRow_FullMethodName,
				reqV2, &v2pb.MutateRowResponse{})
			if got := status.Code(err); got != codes.InvalidArgument {
				t.Fatalf("Invoke(MutateRow) code = %v; want InvalidArgument", got)
			}
			// A rejected out-of-scope name must never open a session handle:
			// that is exactly the cross-tenant rebind the scope check prevents.
			if sc.newTableCalled != 0 || sc.newAVCalled != 0 || sc.newMVCalled != 0 {
				t.Errorf("opened a session handle for out-of-scope name (table=%d av=%d mv=%d); want none",
					sc.newTableCalled, sc.newAVCalled, sc.newMVCalled)
			}
		})
	}
}

func TestInvoke_MutateRow_CachesTableHandle(t *testing.T) {
	sc := &mockSessionClient{table: &mockSessionTableAPI{}}
	channel := newTestChannel(t, sc)

	mutate := func(table string) {
		t.Helper()
		if err := channel.Invoke(context.Background(),
			v2pb.Bigtable_MutateRow_FullMethodName,
			&v2pb.MutateRowRequest{TableName: table, RowKey: []byte("k")},
			&v2pb.MutateRowResponse{}); err != nil {
			t.Fatalf("Invoke(MutateRow, %s): %v", table, err)
		}
	}

	// Repeated RPCs to the same table share one cached handle: OpenTable runs
	// once, on the first (cache-miss) call.
	for i := 0; i < 3; i++ {
		mutate("projects/p/instances/i/tables/t")
	}
	if sc.newTableCalled != 1 {
		t.Errorf("OpenTable called %d times for one table across 3 MutateRows; want 1 (cached)", sc.newTableCalled)
	}

	// A distinct resource misses the cache and opens its own handle.
	mutate("projects/p/instances/i/tables/other")
	if sc.newTableCalled != 2 {
		t.Errorf("OpenTable called %d times after a second distinct table; want 2", sc.newTableCalled)
	}
}

func TestInvoke_MutateRow_PropagatesSessionError(t *testing.T) {
	sentinel := errors.New("session boom")
	tbl := &mockSessionTableAPI{
		mutateRowFn: func(ctx context.Context, req *v2pb.SessionMutateRowRequest) (*v2pb.SessionMutateRowResponse, error) {
			return nil, sentinel
		},
	}
	channel := newTestChannel(t, &mockSessionClient{table: tbl})

	reqV2 := &v2pb.MutateRowRequest{
		TableName: "projects/p/instances/i/tables/t",
		RowKey:    []byte("k"),
	}
	err := channel.Invoke(context.Background(),
		v2pb.Bigtable_MutateRow_FullMethodName,
		reqV2, &v2pb.MutateRowResponse{})
	if !errors.Is(err, sentinel) {
		t.Errorf("Invoke(MutateRow) err = %v; want %v", err, sentinel)
	}
}

func TestInvoke_UnknownMethod_ReturnsUnimplemented(t *testing.T) {
	channel := newTestChannel(t, &mockSessionClient{table: &mockSessionTableAPI{}})

	err := channel.Invoke(context.Background(), "/google.bigtable.v2.Bigtable/SampleRowKeys", nil, nil)
	if status.Code(err) != codes.Unimplemented {
		t.Errorf("Invoke(unknown) code = %v; want Unimplemented", status.Code(err))
	}
}

func singleKeyReadRowsRequest(table, key string) *v2pb.ReadRowsRequest {
	return &v2pb.ReadRowsRequest{
		TableName: table,
		Rows: &v2pb.RowSet{
			RowKeys: [][]byte{[]byte(key)},
		},
	}
}

func TestNewStream_ReadRows_DispatchesThroughSessionAndAdaptsResponse(t *testing.T) {
	gotKey := ""
	tbl := &mockSessionTableAPI{
		readRowFn: func(_ context.Context, req *v2pb.SessionReadRowRequest) (*v2pb.SessionReadRowResponse, error) {
			gotKey = string(req.Key)
			return &v2pb.SessionReadRowResponse{
				Row: &v2pb.Row{
					Key: []byte("k"),
					Families: []*v2pb.Family{{
						Name: "fam",
						Columns: []*v2pb.Column{{
							Qualifier: []byte("q"),
							Cells: []*v2pb.Cell{{
								TimestampMicros: 42,
								Value:           []byte("v"),
							}},
						}},
					}},
				},
			}, nil
		},
	}
	sc := &mockSessionClient{table: tbl}
	channel := newTestChannel(t, sc)

	stream, err := channel.NewStream(context.Background(), nil, v2pb.Bigtable_ReadRows_FullMethodName)
	if err != nil {
		t.Fatalf("NewStream: %v", err)
	}
	if err := stream.SendMsg(singleKeyReadRowsRequest("projects/p/instances/i/tables/t", "k")); err != nil {
		t.Fatalf("SendMsg: %v", err)
	}
	if err := stream.CloseSend(); err != nil {
		t.Fatalf("CloseSend: %v", err)
	}

	resp := &v2pb.ReadRowsResponse{}
	if err := stream.RecvMsg(resp); err != nil {
		t.Fatalf("RecvMsg #1: %v", err)
	}
	if gotKey != "k" {
		t.Errorf("session.ReadRow Key = %q; want %q", gotKey, "k")
	}
	if sc.lastTableName != "t" {
		t.Errorf("NewSessionTable called with %q; want %q", sc.lastTableName, "t")
	}
	if len(resp.Chunks) != 1 {
		t.Fatalf("Chunks len = %d; want 1", len(resp.Chunks))
	}
	cc := resp.Chunks[0]
	if !bytes.Equal(cc.RowKey, []byte("k")) {
		t.Errorf("Chunk.RowKey = %q; want k", cc.RowKey)
	}
	if cc.FamilyName == nil || cc.FamilyName.Value != "fam" {
		t.Errorf("Chunk.FamilyName = %v; want fam", cc.FamilyName)
	}
	if cc.GetCommitRow() != true {
		t.Errorf("Chunk.CommitRow = %v; want true", cc.GetCommitRow())
	}

	if err := stream.RecvMsg(&v2pb.ReadRowsResponse{}); err != io.EOF {
		t.Errorf("RecvMsg #2 = %v; want io.EOF", err)
	}
}

func TestNewStream_ReadRows_RoutesToAuthorizedView(t *testing.T) {
	sc := &mockSessionClient{table: &mockSessionTableAPI{}}
	channel := newTestChannel(t, sc)

	stream, err := channel.NewStream(context.Background(), nil, v2pb.Bigtable_ReadRows_FullMethodName)
	if err != nil {
		t.Fatalf("NewStream: %v", err)
	}
	req := &v2pb.ReadRowsRequest{
		AuthorizedViewName: "projects/p/instances/i/tables/t/authorizedViews/v",
		Rows:               &v2pb.RowSet{RowKeys: [][]byte{[]byte("k")}},
	}
	if err := stream.SendMsg(req); err != nil {
		t.Fatalf("SendMsg: %v", err)
	}
	if err := stream.RecvMsg(&v2pb.ReadRowsResponse{}); err != nil {
		t.Fatalf("RecvMsg: %v", err)
	}
	if sc.newAVCalled != 1 {
		t.Errorf("OpenAuthorizedView called %d times; want 1", sc.newAVCalled)
	}
	if sc.lastAVTable != "t" || sc.lastAVView != "v" {
		t.Errorf("OpenAuthorizedView(table, view) = (%q, %q); want (t, v)", sc.lastAVTable, sc.lastAVView)
	}
	if sc.newTableCalled != 0 {
		t.Errorf("OpenTable called %d times; want 0 (authorized view request)", sc.newTableCalled)
	}
}

func TestNewStream_ReadRows_RoutesToMaterializedView(t *testing.T) {
	sc := &mockSessionClient{table: &mockSessionTableAPI{}}
	channel := newTestChannel(t, sc)

	stream, err := channel.NewStream(context.Background(), nil, v2pb.Bigtable_ReadRows_FullMethodName)
	if err != nil {
		t.Fatalf("NewStream: %v", err)
	}
	req := &v2pb.ReadRowsRequest{
		MaterializedViewName: "projects/p/instances/i/materializedViews/mv",
		Rows:                 &v2pb.RowSet{RowKeys: [][]byte{[]byte("k")}},
	}
	if err := stream.SendMsg(req); err != nil {
		t.Fatalf("SendMsg: %v", err)
	}
	if err := stream.RecvMsg(&v2pb.ReadRowsResponse{}); err != nil {
		t.Fatalf("RecvMsg: %v", err)
	}
	if sc.newMVCalled != 1 {
		t.Errorf("OpenMaterializedView called %d times; want 1", sc.newMVCalled)
	}
	if sc.lastMVView != "mv" {
		t.Errorf("OpenMaterializedView view = %q; want %q", sc.lastMVView, "mv")
	}
	if sc.newTableCalled != 0 {
		t.Errorf("OpenTable called %d times; want 0 (materialized view request)", sc.newTableCalled)
	}
}

func TestNewStream_ReadRows_RejectsOutOfScope(t *testing.T) {
	cases := []struct {
		name string
		req  *v2pb.ReadRowsRequest
	}{
		{"wrong-project-table", &v2pb.ReadRowsRequest{
			TableName: "projects/other/instances/i/tables/t",
			Rows:      &v2pb.RowSet{RowKeys: [][]byte{[]byte("k")}},
		}},
		{"wrong-instance-av", &v2pb.ReadRowsRequest{
			AuthorizedViewName: "projects/p/instances/other/tables/t/authorizedViews/v",
			Rows:               &v2pb.RowSet{RowKeys: [][]byte{[]byte("k")}},
		}},
		{"wrong-project-mv", &v2pb.ReadRowsRequest{
			MaterializedViewName: "projects/other/instances/i/materializedViews/mv",
			Rows:                 &v2pb.RowSet{RowKeys: [][]byte{[]byte("k")}},
		}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			sc := &mockSessionClient{table: &mockSessionTableAPI{}}
			channel := newTestChannel(t, sc)

			stream, err := channel.NewStream(context.Background(), nil, v2pb.Bigtable_ReadRows_FullMethodName)
			if err != nil {
				t.Fatalf("NewStream: %v", err)
			}
			if err := stream.SendMsg(tc.req); err != nil {
				t.Fatalf("SendMsg: %v", err)
			}
			// Dispatch is lazy: the scope check fires on the first RecvMsg.
			err = stream.RecvMsg(&v2pb.ReadRowsResponse{})
			if got := status.Code(err); got != codes.InvalidArgument {
				t.Fatalf("RecvMsg code = %v; want InvalidArgument", got)
			}
			if sc.newTableCalled != 0 || sc.newAVCalled != 0 || sc.newMVCalled != 0 {
				t.Errorf("opened a session handle for out-of-scope name (table=%d av=%d mv=%d); want none",
					sc.newTableCalled, sc.newAVCalled, sc.newMVCalled)
			}
		})
	}
}

func TestNewStream_ReadRows_LazyDispatch(t *testing.T) {
	called := false
	tbl := &mockSessionTableAPI{
		readRowFn: func(_ context.Context, _ *v2pb.SessionReadRowRequest) (*v2pb.SessionReadRowResponse, error) {
			called = true
			return &v2pb.SessionReadRowResponse{}, nil
		},
	}
	channel := newTestChannel(t, &mockSessionClient{table: tbl})

	stream, err := channel.NewStream(context.Background(), nil, v2pb.Bigtable_ReadRows_FullMethodName)
	if err != nil {
		t.Fatalf("NewStream: %v", err)
	}
	if err := stream.SendMsg(singleKeyReadRowsRequest("projects/p/instances/i/tables/t", "k")); err != nil {
		t.Fatalf("SendMsg: %v", err)
	}
	if called {
		t.Fatal("session ReadRow invoked before RecvMsg — backpressure violated")
	}
	if err := stream.RecvMsg(&v2pb.ReadRowsResponse{}); err != nil {
		t.Fatalf("RecvMsg: %v", err)
	}
	if !called {
		t.Fatal("session ReadRow not invoked on first RecvMsg")
	}
}

func TestNewStream_ReadRows_MissingRowEmitsEmptyResponseThenEOF(t *testing.T) {
	tbl := &mockSessionTableAPI{
		readRowFn: func(_ context.Context, _ *v2pb.SessionReadRowRequest) (*v2pb.SessionReadRowResponse, error) {
			return &v2pb.SessionReadRowResponse{}, nil // Row == nil
		},
	}
	channel := newTestChannel(t, &mockSessionClient{table: tbl})

	stream, err := channel.NewStream(context.Background(), nil, v2pb.Bigtable_ReadRows_FullMethodName)
	if err != nil {
		t.Fatalf("NewStream: %v", err)
	}
	if err := stream.SendMsg(singleKeyReadRowsRequest("projects/p/instances/i/tables/t", "k")); err != nil {
		t.Fatalf("SendMsg: %v", err)
	}
	resp := &v2pb.ReadRowsResponse{}
	if err := stream.RecvMsg(resp); err != nil {
		t.Fatalf("RecvMsg: %v", err)
	}
	if len(resp.Chunks) != 0 {
		t.Errorf("Chunks len = %d; want 0", len(resp.Chunks))
	}
	if err := stream.RecvMsg(&v2pb.ReadRowsResponse{}); err != io.EOF {
		t.Errorf("RecvMsg #2 = %v; want io.EOF", err)
	}
}

func TestNewStream_ReadRows_MultiKey_ReturnsUnimplemented(t *testing.T) {
	channel := newTestChannel(t, &mockSessionClient{table: &mockSessionTableAPI{}})
	stream, err := channel.NewStream(context.Background(), nil, v2pb.Bigtable_ReadRows_FullMethodName)
	if err != nil {
		t.Fatalf("NewStream: %v", err)
	}
	req := &v2pb.ReadRowsRequest{
		TableName: "projects/p/instances/i/tables/t",
		Rows: &v2pb.RowSet{
			RowKeys: [][]byte{[]byte("k1"), []byte("k2")},
		},
	}
	err = stream.SendMsg(req)
	if got := status.Code(err); got != codes.Unimplemented {
		t.Errorf("SendMsg(multi-key) code = %v; want Unimplemented", got)
	}
}

func TestNewStream_ReadRows_MixedKeysAndRanges_ReturnsUnimplemented(t *testing.T) {
	channel := newTestChannel(t, &mockSessionClient{table: &mockSessionTableAPI{}})
	stream, err := channel.NewStream(context.Background(), nil, v2pb.Bigtable_ReadRows_FullMethodName)
	if err != nil {
		t.Fatalf("NewStream: %v", err)
	}
	req := &v2pb.ReadRowsRequest{
		TableName: "projects/p/instances/i/tables/t",
		Rows: &v2pb.RowSet{
			RowKeys:   [][]byte{[]byte("k")},
			RowRanges: []*v2pb.RowRange{{}},
		},
	}
	err = stream.SendMsg(req)
	if got := status.Code(err); got != codes.Unimplemented {
		t.Errorf("SendMsg(mixed keys+ranges) code = %v; want Unimplemented", got)
	}
}

// closedClosedRange builds a RowRange with equal closed bounds — the only
// shape SessionReadRow can serve.
func closedClosedRange(key string) *v2pb.RowRange {
	return &v2pb.RowRange{
		StartKey: &v2pb.RowRange_StartKeyClosed{StartKeyClosed: []byte(key)},
		EndKey:   &v2pb.RowRange_EndKeyClosed{EndKeyClosed: []byte(key)},
	}
}

func TestNewStream_ReadRows_SingleClosedClosedRange_DispatchesSingleRow(t *testing.T) {
	gotKey := ""
	tbl := &mockSessionTableAPI{
		readRowFn: func(_ context.Context, req *v2pb.SessionReadRowRequest) (*v2pb.SessionReadRowResponse, error) {
			gotKey = string(req.Key)
			return &v2pb.SessionReadRowResponse{}, nil
		},
	}
	channel := newTestChannel(t, &mockSessionClient{table: tbl})

	stream, err := channel.NewStream(context.Background(), nil, v2pb.Bigtable_ReadRows_FullMethodName)
	if err != nil {
		t.Fatalf("NewStream: %v", err)
	}
	req := &v2pb.ReadRowsRequest{
		TableName: "projects/p/instances/i/tables/t",
		Rows:      &v2pb.RowSet{RowRanges: []*v2pb.RowRange{closedClosedRange("k")}},
	}
	if err := stream.SendMsg(req); err != nil {
		t.Fatalf("SendMsg: %v", err)
	}
	if err := stream.RecvMsg(&v2pb.ReadRowsResponse{}); err != nil {
		t.Fatalf("RecvMsg: %v", err)
	}
	if gotKey != "k" {
		t.Errorf("session.ReadRow Key = %q; want %q", gotKey, "k")
	}
}

func TestNewStream_ReadRows_RangeRejects(t *testing.T) {
	cases := []struct {
		name     string
		rowRange *v2pb.RowRange
	}{
		{"unequal-closed-closed", &v2pb.RowRange{
			StartKey: &v2pb.RowRange_StartKeyClosed{StartKeyClosed: []byte("a")},
			EndKey:   &v2pb.RowRange_EndKeyClosed{EndKeyClosed: []byte("b")},
		}},
		{"closed-open", &v2pb.RowRange{
			StartKey: &v2pb.RowRange_StartKeyClosed{StartKeyClosed: []byte("a")},
			EndKey:   &v2pb.RowRange_EndKeyOpen{EndKeyOpen: []byte("a")},
		}},
		{"open-closed", &v2pb.RowRange{
			StartKey: &v2pb.RowRange_StartKeyOpen{StartKeyOpen: []byte("a")},
			EndKey:   &v2pb.RowRange_EndKeyClosed{EndKeyClosed: []byte("a")},
		}},
		{"unbounded", &v2pb.RowRange{}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			channel := newTestChannel(t, &mockSessionClient{table: &mockSessionTableAPI{}})
			stream, err := channel.NewStream(context.Background(), nil, v2pb.Bigtable_ReadRows_FullMethodName)
			if err != nil {
				t.Fatalf("NewStream: %v", err)
			}
			req := &v2pb.ReadRowsRequest{
				TableName: "projects/p/instances/i/tables/t",
				Rows:      &v2pb.RowSet{RowRanges: []*v2pb.RowRange{tc.rowRange}},
			}
			err = stream.SendMsg(req)
			if got := status.Code(err); got != codes.Unimplemented {
				t.Errorf("SendMsg code = %v; want Unimplemented", got)
			}
		})
	}
}

func TestNewStream_ReadRows_MultipleRanges_ReturnsUnimplemented(t *testing.T) {
	channel := newTestChannel(t, &mockSessionClient{table: &mockSessionTableAPI{}})
	stream, err := channel.NewStream(context.Background(), nil, v2pb.Bigtable_ReadRows_FullMethodName)
	if err != nil {
		t.Fatalf("NewStream: %v", err)
	}
	req := &v2pb.ReadRowsRequest{
		TableName: "projects/p/instances/i/tables/t",
		Rows: &v2pb.RowSet{
			RowRanges: []*v2pb.RowRange{closedClosedRange("a"), closedClosedRange("b")},
		},
	}
	err = stream.SendMsg(req)
	if got := status.Code(err); got != codes.Unimplemented {
		t.Errorf("SendMsg(multi-range) code = %v; want Unimplemented", got)
	}
}

func TestNewStream_ReadRows_PropagatesSessionError(t *testing.T) {
	sentinel := errors.New("session boom")
	tbl := &mockSessionTableAPI{
		readRowFn: func(_ context.Context, _ *v2pb.SessionReadRowRequest) (*v2pb.SessionReadRowResponse, error) {
			return nil, sentinel
		},
	}
	channel := newTestChannel(t, &mockSessionClient{table: tbl})

	stream, err := channel.NewStream(context.Background(), nil, v2pb.Bigtable_ReadRows_FullMethodName)
	if err != nil {
		t.Fatalf("NewStream: %v", err)
	}
	if err := stream.SendMsg(singleKeyReadRowsRequest("projects/p/instances/i/tables/t", "k")); err != nil {
		t.Fatalf("SendMsg: %v", err)
	}
	if err := stream.RecvMsg(&v2pb.ReadRowsResponse{}); !errors.Is(err, sentinel) {
		t.Errorf("RecvMsg err = %v; want %v", err, sentinel)
	}
	// After an aborting error the stream replays that terminal error, not
	// io.EOF: returning io.EOF would falsely signal clean completion to a
	// caller that keeps pulling (matches grpc.ClientStream).
	if err := stream.RecvMsg(&v2pb.ReadRowsResponse{}); !errors.Is(err, sentinel) {
		t.Errorf("RecvMsg after error = %v; want %v", err, sentinel)
	}
}

func TestNewStream_ReadRows_CachesTableHandle(t *testing.T) {
	tbl := &mockSessionTableAPI{
		readRowFn: func(_ context.Context, _ *v2pb.SessionReadRowRequest) (*v2pb.SessionReadRowResponse, error) {
			return &v2pb.SessionReadRowResponse{}, nil
		},
	}
	sc := &mockSessionClient{table: tbl}
	channel := newTestChannel(t, sc)

	readRows := func(table string) {
		t.Helper()
		stream, err := channel.NewStream(context.Background(), nil, v2pb.Bigtable_ReadRows_FullMethodName)
		if err != nil {
			t.Fatalf("NewStream(%s): %v", table, err)
		}
		if err := stream.SendMsg(singleKeyReadRowsRequest(table, "k")); err != nil {
			t.Fatalf("SendMsg(%s): %v", table, err)
		}
		if err := stream.RecvMsg(&v2pb.ReadRowsResponse{}); err != nil {
			t.Fatalf("RecvMsg(%s): %v", table, err)
		}
	}

	// Repeated ReadRows to the same table share one cached handle: OpenTable
	// runs once, on the first (cache-miss) call.
	for i := 0; i < 3; i++ {
		readRows("projects/p/instances/i/tables/t")
	}
	if sc.newTableCalled != 1 {
		t.Errorf("OpenTable called %d times across 3 ReadRows; want 1 (cached)", sc.newTableCalled)
	}

	// A distinct resource misses the cache and opens its own handle.
	readRows("projects/p/instances/i/tables/other")
	if sc.newTableCalled != 2 {
		t.Errorf("OpenTable called %d times after a second distinct table; want 2", sc.newTableCalled)
	}
}

func TestNewStream_ReadRows_SharesCachedHandleWithMutateRow(t *testing.T) {
	tbl := &mockSessionTableAPI{
		readRowFn: func(_ context.Context, _ *v2pb.SessionReadRowRequest) (*v2pb.SessionReadRowResponse, error) {
			return &v2pb.SessionReadRowResponse{}, nil
		},
	}
	sc := &mockSessionClient{table: tbl}
	channel := newTestChannel(t, sc)

	stream, err := channel.NewStream(context.Background(), nil, v2pb.Bigtable_ReadRows_FullMethodName)
	if err != nil {
		t.Fatalf("NewStream: %v", err)
	}
	if err := stream.SendMsg(singleKeyReadRowsRequest("projects/p/instances/i/tables/t", "k")); err != nil {
		t.Fatalf("SendMsg: %v", err)
	}
	if err := stream.RecvMsg(&v2pb.ReadRowsResponse{}); err != nil {
		t.Fatalf("RecvMsg: %v", err)
	}

	if err := channel.Invoke(context.Background(),
		v2pb.Bigtable_MutateRow_FullMethodName,
		&v2pb.MutateRowRequest{
			TableName: "projects/p/instances/i/tables/t",
			RowKey:    []byte("k"),
		}, &v2pb.MutateRowResponse{}); err != nil {
		t.Fatalf("Invoke(MutateRow): %v", err)
	}

	// Read and write on the same table key into the same cache entry, so the
	// handle opened by the ReadRows is reused by the MutateRow.
	if sc.newTableCalled != 1 {
		t.Errorf("OpenTable called %d times for read+write on one table; want 1 (shared handle)", sc.newTableCalled)
	}
}

func TestNewStream_UnknownMethod_ReturnsUnimplemented(t *testing.T) {
	channel := newTestChannel(t, &mockSessionClient{table: &mockSessionTableAPI{}})

	_, err := channel.NewStream(context.Background(), nil, "/google.bigtable.v2.Bigtable/SampleRowKeys")
	if status.Code(err) != codes.Unimplemented {
		t.Errorf("NewStream(unknown) code = %v; want Unimplemented", status.Code(err))
	}
}

func TestComposeUserAgent(t *testing.T) {
	for _, tc := range []struct {
		name   string
		prefix string
		want   string
	}{
		{name: "empty prefix returns daemon token", prefix: "", want: userAgent},
		{name: "blank prefix returns daemon token", prefix: "   ", want: userAgent},
		{name: "prefix is prepended and space-separated", prefix: "python-bigtable/2.1.0", want: "python-bigtable/2.1.0 " + userAgent},
		{name: "prefix is trimmed before joining", prefix: "  python-bigtable/2.1.0  ", want: "python-bigtable/2.1.0 " + userAgent},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := ComposeUserAgent(tc.prefix); got != tc.want {
				t.Errorf("ComposeUserAgent(%q) = %q; want %q", tc.prefix, got, tc.want)
			}
		})
	}
}

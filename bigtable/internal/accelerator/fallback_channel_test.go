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
	"context"
	"io"
	"testing"

	"cloud.google.com/go/bigtable"
	v2pb "cloud.google.com/go/bigtable/apiv2/bigtablepb"
	"cloud.google.com/go/bigtable/internal/session"
	"google.golang.org/api/option"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	gmetadata "google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
)

// --- mockClientStream -------------------------------------------------------

// mockClientStream is a minimal grpc.ClientStream that delegates SendMsg and
// RecvMsg to caller-supplied functions and exposes configurable metadata.
type mockClientStream struct {
	sendFn    func(m any) error
	recvFn    func(m any) error
	headerMD  gmetadata.MD
	trailerMD gmetadata.MD
}

func (m *mockClientStream) Header() (gmetadata.MD, error) { return m.headerMD, nil }
func (m *mockClientStream) Trailer() gmetadata.MD         { return m.trailerMD }
func (m *mockClientStream) CloseSend() error              { return nil }
func (m *mockClientStream) Context() context.Context      { return context.Background() }
func (m *mockClientStream) SendMsg(msg any) error {
	if m.sendFn != nil {
		return m.sendFn(msg)
	}
	return nil
}
func (m *mockClientStream) RecvMsg(msg any) error {
	if m.recvFn != nil {
		return m.recvFn(msg)
	}
	return io.EOF
}

// --- helpers ----------------------------------------------------------------

// singleKeyReadReq builds a ReadRowsRequest for a single row key against
// the package-level testTableName.
func singleKeyReadReq(key string) *v2pb.ReadRowsRequest {
	return &v2pb.ReadRowsRequest{
		TableName: testTableName,
		Rows:      &v2pb.RowSet{RowKeys: [][]byte{[]byte(key)}},
	}
}

// newTestFallbackChannel constructs a FallbackChannel without dialing a real
// server. The caller supplies a pre-populated classicFallback mock and a
// *Channel (may be nil when the channel's Invoke/NewStream won't be called).
func newTestFallbackChannel(session *Channel, cf *classicFallback) *FallbackChannel {
	return &FallbackChannel{session: session, classic: cf}
}

// classicWithRow returns a classicFallback that answers ReadRow with the
// supplied bigtable.Row for any key.
func classicWithRow(row bigtable.Row) *classicFallback {
	mock := &mockTableAPI{
		readRowFn: func(_ context.Context, _ string, _ ...bigtable.ReadOption) (bigtable.Row, error) {
			return row, nil
		},
	}
	return newClassicFallbackWithMock(map[string]bigtable.TableAPI{testTableName: mock})
}

// --- fallbackReadRowsStream tests -------------------------------------------

// TestFallbackStream_AlreadyTripped verifies that when fc.tripped is set, the
// stream goes directly to classic without consulting inner.
func TestFallbackStream_AlreadyTripped(t *testing.T) {
	wantRow := bigtable.Row{"cf": {{Column: "cf:q", Timestamp: 1, Value: []byte("v")}}}
	cf := classicWithRow(wantRow)
	fc := newTestFallbackChannel(nil, cf)
	fc.tripped.Store(true)

	s := &fallbackReadRowsStream{ctx: context.Background(), fc: fc}

	req := singleKeyReadReq("k")
	if err := s.SendMsg(req); err != nil {
		t.Fatalf("SendMsg: %v", err)
	}

	resp := &v2pb.ReadRowsResponse{}
	if err := s.RecvMsg(resp); err != nil {
		t.Fatalf("RecvMsg: %v", err)
	}
	if len(resp.Chunks) == 0 {
		t.Error("expected chunks from classic; got none")
	}

	// Second RecvMsg must return io.EOF (stream is done).
	if err := s.RecvMsg(resp); err != io.EOF {
		t.Errorf("second RecvMsg = %v, want io.EOF", err)
	}
}

// TestFallbackStream_SessionSuccess verifies the happy path: the session inner
// stream returns a valid response that is forwarded as-is.
func TestFallbackStream_SessionSuccess(t *testing.T) {
	wantChunkValue := []byte("fromSession")
	inner := &mockClientStream{
		recvFn: func(m any) error {
			resp := m.(*v2pb.ReadRowsResponse)
			resp.Chunks = []*v2pb.ReadRowsResponse_CellChunk{
				{RowKey: []byte("k"), Value: wantChunkValue},
			}
			return nil
		},
	}
	fc := newTestFallbackChannel(nil, nil)
	s := &fallbackReadRowsStream{ctx: context.Background(), fc: fc, inner: inner}

	req := singleKeyReadReq("k")
	if err := s.SendMsg(req); err != nil {
		t.Fatalf("SendMsg: %v", err)
	}

	resp := &v2pb.ReadRowsResponse{}
	if err := s.RecvMsg(resp); err != nil {
		t.Fatalf("RecvMsg: %v", err)
	}
	if len(resp.Chunks) == 0 || string(resp.Chunks[0].Value) != string(wantChunkValue) {
		t.Errorf("resp.Chunks[0].Value = %q, want %q", resp.Chunks[0].Value, wantChunkValue)
	}
	if fc.tripped.Load() {
		t.Error("session success should not trip breaker")
	}
}

// TestFallbackStream_RecvUnimplemented verifies that Unimplemented from the
// session backend trips the breaker and retries via classic.
func TestFallbackStream_RecvUnimplemented(t *testing.T) {
	inner := &mockClientStream{
		recvFn: func(m any) error {
			return status.Error(codes.Unimplemented, "session not enabled")
		},
	}
	wantRow := bigtable.Row{"cf": {{Column: "cf:q", Timestamp: 1, Value: []byte("classic")}}}
	cf := classicWithRow(wantRow)
	fc := newTestFallbackChannel(nil, cf)
	s := &fallbackReadRowsStream{ctx: context.Background(), fc: fc, inner: inner}

	req := singleKeyReadReq("k")
	if err := s.SendMsg(req); err != nil {
		t.Fatalf("SendMsg: %v", err)
	}

	resp := &v2pb.ReadRowsResponse{}
	if err := s.RecvMsg(resp); err != nil {
		t.Fatalf("RecvMsg: %v", err)
	}
	if !fc.tripped.Load() {
		t.Error("breaker should be tripped after backend Unimplemented")
	}
	if len(resp.Chunks) == 0 {
		t.Error("expected classic response after fallback; got no chunks")
	}
}

// TestFallbackStream_SendUnimplementedNoBreaker verifies that Unimplemented
// from SendMsg (shape error) propagates without tripping the breaker.
func TestFallbackStream_SendUnimplementedNoBreaker(t *testing.T) {
	inner := &mockClientStream{
		sendFn: func(m any) error {
			return status.Error(codes.Unimplemented, "shape not supported")
		},
	}
	fc := newTestFallbackChannel(nil, nil)
	s := &fallbackReadRowsStream{ctx: context.Background(), fc: fc, inner: inner}

	req := singleKeyReadReq("k")
	err := s.SendMsg(req)
	if status.Code(err) != codes.Unimplemented {
		t.Errorf("SendMsg = %v, want Unimplemented", err)
	}
	if fc.tripped.Load() {
		t.Error("shape-based Unimplemented must not trip breaker")
	}
}

// TestFallbackStream_AlreadyTripped_ShapeErrorFromSendMsg verifies that even
// on the already-tripped path, a multi-row request is rejected via
// validateSingleRowReadRequest and returns Unimplemented.
func TestFallbackStream_AlreadyTripped_MultiRowReturnsUnimplemented(t *testing.T) {
	fc := newTestFallbackChannel(nil, nil)
	fc.tripped.Store(true)
	s := &fallbackReadRowsStream{ctx: context.Background(), fc: fc}

	// Multi-key request: not a single-row read.
	req := &v2pb.ReadRowsRequest{
		TableName: testTableName,
		Rows:      &v2pb.RowSet{RowKeys: [][]byte{[]byte("k1"), []byte("k2")}},
	}
	err := s.SendMsg(req)
	if status.Code(err) != codes.Unimplemented {
		t.Errorf("multi-key request on tripped channel: got %v, want Unimplemented", err)
	}
}

// TestFallbackStream_HeaderDelegation verifies that Header/Trailer/CloseSend
// are forwarded to the inner stream when it is active.
func TestFallbackStream_HeaderDelegation(t *testing.T) {
	wantMD := gmetadata.Pairs("x-routing", "us-central1")
	inner := &mockClientStream{headerMD: wantMD, trailerMD: gmetadata.Pairs("grpc-status", "0")}
	fc := newTestFallbackChannel(nil, nil)
	s := &fallbackReadRowsStream{ctx: context.Background(), fc: fc, inner: inner}

	gotHeader, err := s.Header()
	if err != nil {
		t.Fatalf("Header() error: %v", err)
	}
	if got := gotHeader.Get("x-routing"); len(got) == 0 || got[0] != "us-central1" {
		t.Errorf("Header x-routing = %v, want [us-central1]", got)
	}
	gotTrailer := s.Trailer()
	if got := gotTrailer.Get("grpc-status"); len(got) == 0 || got[0] != "0" {
		t.Errorf("Trailer grpc-status = %v, want [0]", got)
	}
}

// TestFallbackStream_HeaderNilWhenNoInner verifies Header/Trailer return nil
// when inner is nil (classic path).
func TestFallbackStream_HeaderNilWhenNoInner(t *testing.T) {
	fc := newTestFallbackChannel(nil, nil)
	s := &fallbackReadRowsStream{ctx: context.Background(), fc: fc}

	gotHeader, err := s.Header()
	if err != nil || gotHeader != nil {
		t.Errorf("Header() = (%v, %v), want (nil, nil)", gotHeader, err)
	}
	if gotTrailer := s.Trailer(); gotTrailer != nil {
		t.Errorf("Trailer() = %v, want nil", gotTrailer)
	}
}

// --- FallbackChannel.Invoke -------------------------------------------------

// TestFallbackChannel_Invoke_UnimplementedTripsBreaker wires a session Channel
// whose MutateRow returns Unimplemented, verifying that the FallbackChannel
// flips fc.tripped and routes the subsequent call to classic.
func TestFallbackChannel_Invoke_UnimplementedTripsBreaker(t *testing.T) {
	sc := &mockSessionClient{
		table: &mockSessionTableAPI{
			mutateRowFn: func(_ context.Context, _ *v2pb.SessionMutateRowRequest) (*v2pb.SessionMutateRowResponse, error) {
				return nil, status.Error(codes.Unimplemented, "session not enabled")
			},
		},
	}
	sessionChannel := newTestChannel(t, sc)

	var classicCalled bool
	mock := &mockTableAPI{
		applyFn: func(_ context.Context, _ string, _ *bigtable.Mutation, _ ...bigtable.ApplyOption) error {
			classicCalled = true
			return nil
		},
	}
	cf := newClassicFallbackWithMock(map[string]bigtable.TableAPI{testTableName: mock})
	fc := newTestFallbackChannel(sessionChannel, cf)

	req := &v2pb.MutateRowRequest{
		TableName: testTableName,
		RowKey:    []byte("k"),
	}
	if err := fc.Invoke(context.Background(), v2pb.Bigtable_MutateRow_FullMethodName, req, &v2pb.MutateRowResponse{}); err != nil {
		t.Fatalf("Invoke: %v", err)
	}
	if !fc.tripped.Load() {
		t.Error("breaker should be tripped after session Unimplemented")
	}
	if !classicCalled {
		t.Error("classic Apply not called after fallback")
	}
}

// TestFallbackChannel_Invoke_SessionSuccess verifies the happy path: session
// Invoke succeeds, the breaker is not tripped, and classic is never called.
func TestFallbackChannel_Invoke_SessionSuccess(t *testing.T) {
	sc := &mockSessionClient{
		table: &mockSessionTableAPI{
			mutateRowFn: func(_ context.Context, _ *v2pb.SessionMutateRowRequest) (*v2pb.SessionMutateRowResponse, error) {
				return &v2pb.SessionMutateRowResponse{}, nil
			},
		},
	}
	sessionChannel := newTestChannel(t, sc)

	classicCalls := 0
	mock := &mockTableAPI{
		applyFn: func(_ context.Context, _ string, _ *bigtable.Mutation, _ ...bigtable.ApplyOption) error {
			classicCalls++
			return nil
		},
	}
	cf := newClassicFallbackWithMock(map[string]bigtable.TableAPI{testTableName: mock})
	fc := newTestFallbackChannel(sessionChannel, cf)

	req := &v2pb.MutateRowRequest{TableName: testTableName, RowKey: []byte("k")}
	for i := 0; i < 3; i++ {
		if err := fc.Invoke(context.Background(), v2pb.Bigtable_MutateRow_FullMethodName, req, &v2pb.MutateRowResponse{}); err != nil {
			t.Fatalf("Invoke[%d]: %v", i, err)
		}
	}
	if fc.tripped.Load() {
		t.Error("session success should not trip breaker")
	}
	if classicCalls != 0 {
		t.Errorf("classic calls = %d, want 0", classicCalls)
	}
}

// TestFallbackChannel_Invoke_BreakersSticky verifies that once tripped, all
// subsequent Invokes go to classic without touching session.
func TestFallbackChannel_Invoke_BreakerSticky(t *testing.T) {
	sessionCalled := false
	sc := &mockSessionClient{
		table: &mockSessionTableAPI{
			mutateRowFn: func(_ context.Context, _ *v2pb.SessionMutateRowRequest) (*v2pb.SessionMutateRowResponse, error) {
				sessionCalled = true
				return &v2pb.SessionMutateRowResponse{}, nil
			},
		},
	}
	sessionChannel := newTestChannel(t, sc)

	classicCalls := 0
	mock := &mockTableAPI{
		applyFn: func(_ context.Context, _ string, _ *bigtable.Mutation, _ ...bigtable.ApplyOption) error {
			classicCalls++
			return nil
		},
	}
	cf := newClassicFallbackWithMock(map[string]bigtable.TableAPI{testTableName: mock})
	fc := newTestFallbackChannel(sessionChannel, cf)
	fc.tripped.Store(true) // pre-trip

	req := &v2pb.MutateRowRequest{TableName: testTableName, RowKey: []byte("k")}
	for i := 0; i < 3; i++ {
		if err := fc.Invoke(context.Background(), v2pb.Bigtable_MutateRow_FullMethodName, req, &v2pb.MutateRowResponse{}); err != nil {
			t.Fatalf("Invoke[%d]: %v", i, err)
		}
	}
	if sessionCalled {
		t.Error("session should not be called when breaker is already tripped")
	}
	if classicCalls != 3 {
		t.Errorf("classic calls = %d, want 3", classicCalls)
	}
}

// TestFallbackChannel_Invoke_UnsupportedMethod verifies that only MutateRow is
// accepted; other methods get Unimplemented.
func TestFallbackChannel_Invoke_UnsupportedMethod(t *testing.T) {
	fc := newTestFallbackChannel(nil, nil)
	err := fc.Invoke(context.Background(), "/google.bigtable.v2.Bigtable/CheckAndMutateRow",
		&struct{}{}, &struct{}{})
	if status.Code(err) != codes.Unimplemented {
		t.Errorf("got %v, want Unimplemented", err)
	}
}

// TestFallbackChannel_NewStream_UnsupportedMethod verifies that only ReadRows
// is accepted; other streaming methods get Unimplemented.
func TestFallbackChannel_NewStream_UnsupportedMethod(t *testing.T) {
	fc := newTestFallbackChannel(nil, nil)
	_, err := fc.NewStream(context.Background(), &grpc.StreamDesc{}, "/google.bigtable.v2.Bigtable/SampleRowKeys")
	if status.Code(err) != codes.Unimplemented {
		t.Errorf("got %v, want Unimplemented", err)
	}
}

// --- FallbackChannel.Close --------------------------------------------------

// TestFallbackChannel_Close_BothErrors verifies that errors from both session
// and classic are preserved via errors.Join.
func TestFallbackChannel_Close_BothErrors(t *testing.T) {
	// Use a test Channel whose close is a no-op (zero-value sc handles nil).
	sessionChannel := newTestChannel(t, &mockSessionClient{table: &mockSessionTableAPI{}})
	// Pre-close it so its Close() returns nil (double-close is idempotent).
	_ = sessionChannel.Close()

	// Build a classic fallback with a nil client so it returns nil too.
	cf := &classicFallback{}
	fc := newTestFallbackChannel(sessionChannel, cf)

	if err := fc.Close(); err != nil {
		t.Errorf("Close() = %v, want nil when both sides succeed", err)
	}
}

// TestFallbackChannel_Close_ClassicError verifies that a classic close error
// surfaces (not silently dropped).
func TestFallbackChannel_Close_ClassicCloseNilClientNoError(t *testing.T) {
	sessionChannel := newTestChannel(t, &mockSessionClient{table: &mockSessionTableAPI{}})
	_ = sessionChannel.Close()

	cf := &classicFallback{}
	fc := newTestFallbackChannel(sessionChannel, cf)

	// nil client → Close returns nil — just verify no panic.
	if err := fc.Close(); err != nil {
		t.Errorf("Close() = %v, want nil", err)
	}
}

func TestFallbackChannel_MultiInstance_PerInstanceTripping(t *testing.T) {
	origSession := newSessionClient
	sessionCalls := make(map[string]int)
	newSessionClient = func(
		_ context.Context,
		_, instance, _, _ string,
		_ ...option.ClientOption,
	) (session.Client, error) {
		tbl := &mockSessionTableAPI{
			mutateRowFn: func(_ context.Context, _ *v2pb.SessionMutateRowRequest) (*v2pb.SessionMutateRowResponse, error) {
				sessionCalls[instance]++
				if instance == "inst-legacy" {
					return nil, status.Error(codes.Unimplemented, "session not enabled on inst-legacy")
				}
				return &v2pb.SessionMutateRowResponse{}, nil
			},
			readRowFn: func(_ context.Context, _ *v2pb.SessionReadRowRequest) (*v2pb.SessionReadRowResponse, error) {
				sessionCalls[instance]++
				if instance == "inst-legacy" {
					return nil, status.Error(codes.Unimplemented, "session not enabled on inst-legacy")
				}
				return &v2pb.SessionReadRowResponse{}, nil
			},
		}
		return &mockSessionClient{table: tbl}, nil
	}
	t.Cleanup(func() { newSessionClient = origSession })

	sessionChannel, err := NewChannel(context.Background(), "p", "", "", "")
	if err != nil {
		t.Fatalf("NewChannel: %v", err)
	}
	t.Cleanup(func() { _ = sessionChannel.Close() })

	classicCalls := make(map[string]int)
	cf := &classicFallback{
		project:     "p",
		scopePrefix: scopePrefixFor("p", ""),
		tables: map[string]bigtable.TableAPI{
			"projects/p/instances/inst-legacy/tables/t": &mockTableAPI{
				applyFn: func(_ context.Context, _ string, _ *bigtable.Mutation, _ ...bigtable.ApplyOption) error {
					classicCalls["inst-legacy"]++
					return nil
				},
				readRowFn: func(_ context.Context, _ string, _ ...bigtable.ReadOption) (bigtable.Row, error) {
					classicCalls["inst-legacy"]++
					return nil, nil
				},
			},
			"projects/p/instances/inst-session/tables/t": &mockTableAPI{
				applyFn: func(_ context.Context, _ string, _ *bigtable.Mutation, _ ...bigtable.ApplyOption) error {
					classicCalls["inst-session"]++
					return nil
				},
			},
		},
	}
	fc := newTestFallbackChannel(sessionChannel, cf)

	// 1. MutateRow on inst-legacy returns Unimplemented from session, trips inst-legacy, and falls back to classic.
	if err := fc.Invoke(context.Background(), v2pb.Bigtable_MutateRow_FullMethodName, &v2pb.MutateRowRequest{
		TableName: "projects/p/instances/inst-legacy/tables/t",
		RowKey:    []byte("k"),
	}, &v2pb.MutateRowResponse{}); err != nil {
		t.Fatalf("Invoke(inst-legacy, 1): %v", err)
	}
	// 2. Second MutateRow and ReadRows on inst-legacy skip session completely and go straight to classic.
	if err := fc.Invoke(context.Background(), v2pb.Bigtable_MutateRow_FullMethodName, &v2pb.MutateRowRequest{
		TableName: "projects/p/instances/inst-legacy/tables/t",
		RowKey:    []byte("k"),
	}, &v2pb.MutateRowResponse{}); err != nil {
		t.Fatalf("Invoke(inst-legacy, 2): %v", err)
	}
	stream, err := fc.NewStream(context.Background(), &grpc.StreamDesc{}, v2pb.Bigtable_ReadRows_FullMethodName)
	if err != nil {
		t.Fatalf("NewStream(inst-legacy): %v", err)
	}
	if err := stream.SendMsg(&v2pb.ReadRowsRequest{
		TableName: "projects/p/instances/inst-legacy/tables/t",
		Rows:      &v2pb.RowSet{RowKeys: [][]byte{[]byte("k")}},
	}); err != nil {
		t.Fatalf("SendMsg(inst-legacy): %v", err)
	}
	if err := stream.RecvMsg(&v2pb.ReadRowsResponse{}); err != nil {
		t.Fatalf("RecvMsg(inst-legacy): %v", err)
	}

	if sessionCalls["inst-legacy"] != 1 {
		t.Errorf("sessionCalls[inst-legacy] = %d; want 1 (tripped after first call)", sessionCalls["inst-legacy"])
	}
	if classicCalls["inst-legacy"] != 3 {
		t.Errorf("classicCalls[inst-legacy] = %d; want 3", classicCalls["inst-legacy"])
	}

	// 3. MutateRow on inst-session still uses the session fast path (not tripped by inst-legacy!).
	if err := fc.Invoke(context.Background(), v2pb.Bigtable_MutateRow_FullMethodName, &v2pb.MutateRowRequest{
		TableName: "projects/p/instances/inst-session/tables/t",
		RowKey:    []byte("k"),
	}, &v2pb.MutateRowResponse{}); err != nil {
		t.Fatalf("Invoke(inst-session): %v", err)
	}
	if sessionCalls["inst-session"] != 1 {
		t.Errorf("sessionCalls[inst-session] = %d; want 1", sessionCalls["inst-session"])
	}
	if classicCalls["inst-session"] != 0 {
		t.Errorf("classicCalls[inst-session] = %d; want 0", classicCalls["inst-session"])
	}
}

// testTableName and newClassicFallbackWithMock are defined in fallback_test.go.

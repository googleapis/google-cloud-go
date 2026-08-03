/*
Copyright 2026 Google LLC

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    https://www.apache.org/licenses/LICENSE-2.0

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
	"net"
	"runtime"
	"strings"
	"sync/atomic"
	"testing"

	vkit "cloud.google.com/go/spanner/apiv1"
	sppb "cloud.google.com/go/spanner/apiv1/spannerpb"
	. "cloud.google.com/go/spanner/internal/testutil"
	"google.golang.org/api/iterator"
	"google.golang.org/api/option"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/mem"
	"google.golang.org/grpc/metadata"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/emptypb"
	structpb "google.golang.org/protobuf/types/known/structpb"
)

func TestVTSafeCodecUsesGeneratedMethodsAndSafeStrings(t *testing.T) {
	req := &sppb.ExecuteSqlRequest{Sql: "SELECT 1"}
	if _, ok := any(req).(interface{ MarshalVT() ([]byte, error) }); !ok {
		t.Fatal("ExecuteSqlRequest does not implement MarshalVT")
	}

	codec := vtSafeCodec{}
	want := strings.Repeat("safe-string-", 200)
	wire, err := proto.Marshal(&sppb.PartialResultSet{
		Values: []*structpb.Value{structpb.NewStringValue(want)},
	})
	if err != nil {
		t.Fatal(err)
	}
	data := mem.BufferSlice{mem.SliceBuffer(wire)}
	got := new(sppb.PartialResultSet)
	if err := codec.Unmarshal(data, got); err != nil {
		t.Fatal(err)
	}
	for i := range wire {
		wire[i] = 0xff
	}
	if value := got.GetValues()[0].GetStringValue(); value != want {
		t.Fatalf("decoded string changed with receive buffer: got %q, want %q", value, want)
	}
}

func TestVTSafeCodecReflectionFallback(t *testing.T) {
	codec := vtSafeCodec{}
	want := &emptypb.Empty{}
	encoded, err := codec.Marshal(want)
	if err != nil {
		t.Fatal(err)
	}
	got := new(emptypb.Empty)
	if err := codec.Unmarshal(encoded, got); err != nil {
		t.Fatal(err)
	}
	if !proto.Equal(got, want) {
		t.Fatalf("fallback round trip = %v, want %v", got, want)
	}
	if name := codec.Name(); name != "" {
		t.Fatalf("codec name = %q, want empty standard-proto subtype", name)
	}
}

func TestPartialResultSetSizeVTMatchesProtoSize(t *testing.T) {
	tests := []*sppb.PartialResultSet{
		{},
		{
			Metadata: &sppb.ResultSetMetadata{
				RowType: &sppb.StructType{Fields: []*sppb.StructType_Field{
					{Name: "string_col", Type: &sppb.Type{Code: sppb.TypeCode_STRING}},
					{Name: "array_col", Type: &sppb.Type{Code: sppb.TypeCode_ARRAY, ArrayElementType: &sppb.Type{Code: sppb.TypeCode_INT64}}},
				}},
				Transaction: &sppb.Transaction{Id: []byte("tx")},
			},
			Values: []*structpb.Value{
				structpb.NewStringValue(strings.Repeat("value", 40)),
				structpb.NewNumberValue(12.5),
				structpb.NewBoolValue(true),
				structpb.NewNullValue(),
				structpb.NewListValue(&structpb.ListValue{Values: []*structpb.Value{
					structpb.NewStringValue("nested"),
				}}),
			},
			ChunkedValue: true,
			ResumeToken:  []byte("resume-token"),
			Last:         true,
		},
		{
			Stats: &sppb.ResultSetStats{
				QueryStats: &structpb.Struct{Fields: map[string]*structpb.Value{
					"elapsed": structpb.NewStringValue("1.2s"),
				}},
				RowCount: &sppb.ResultSetStats_RowCountExact{RowCountExact: 42},
			},
			PrecommitToken: &sppb.MultiplexedSessionPrecommitToken{PrecommitToken: []byte("precommit"), SeqNum: 7},
		},
	}
	for i, msg := range tests {
		if got, want := msg.SizeVT(), proto.Size(msg); got != want {
			t.Errorf("case %d: SizeVT() = %d, proto.Size() = %d", i, got, want)
		}
	}
}

func TestVTCoverageForSpannerRPCMessages(t *testing.T) {
	messages := []proto.Message{
		&sppb.CreateSessionRequest{}, &sppb.Session{},
		&sppb.BatchCreateSessionsRequest{}, &sppb.BatchCreateSessionsResponse{},
		&sppb.GetSessionRequest{},
		&sppb.ListSessionsRequest{}, &sppb.ListSessionsResponse{},
		&sppb.DeleteSessionRequest{},
		&sppb.ExecuteSqlRequest{}, &sppb.ResultSet{}, &sppb.PartialResultSet{},
		&sppb.ExecuteBatchDmlRequest{}, &sppb.ExecuteBatchDmlResponse{},
		&sppb.ReadRequest{},
		&sppb.BeginTransactionRequest{}, &sppb.Transaction{},
		&sppb.CommitRequest{}, &sppb.CommitResponse{},
		&sppb.RollbackRequest{},
		&sppb.PartitionQueryRequest{}, &sppb.PartitionResponse{},
		&sppb.PartitionReadRequest{},
		&sppb.BatchWriteRequest{}, &sppb.BatchWriteResponse{},
		&sppb.FetchCacheUpdateRequest{}, &sppb.CacheUpdate{},
	}
	for _, msg := range messages {
		if _, ok := msg.(vtMarshaler); !ok {
			t.Errorf("%T does not implement MarshalVT", msg)
		}
		if _, ok := msg.(vtUnmarshaler); !ok {
			t.Errorf("%T does not implement UnmarshalVT", msg)
		}
	}

	// Empty is the only top-level Spanner RPC response without generated
	// methods. It is intentionally covered by the reflection fallback.
	if _, ok := any(new(emptypb.Empty)).(vtUnmarshaler); ok {
		t.Fatal("emptypb.Empty unexpectedly implements UnmarshalVT; fallback test is stale")
	}
}

type codecCoverageService struct {
	sppb.UnimplementedSpannerServer
	gotSQL chan string
	value  string
}

func (s *codecCoverageService) ExecuteSql(_ context.Context, req *sppb.ExecuteSqlRequest) (*sppb.ResultSet, error) {
	s.gotSQL <- req.Sql
	return &sppb.ResultSet{
		Rows: []*structpb.ListValue{{Values: []*structpb.Value{structpb.NewStringValue(s.value)}}},
	}, nil
}

func TestDefaultConnectionCodecCoversOutboundAndNonStreamingResponse(t *testing.T) {
	listener, err := net.Listen("tcp", "localhost:0")
	if err != nil {
		t.Fatal(err)
	}
	server := grpc.NewServer(grpc.ForceServerCodecV2(vtSafeCodec{}))
	invalidUTF8 := string([]byte{'v', 0xff, 't'})
	service := &codecCoverageService{gotSQL: make(chan string, 1), value: invalidUTF8}
	sppb.RegisterSpannerServer(server, service)
	go func() { _ = server.Serve(listener) }()
	defer server.Stop()

	opts := allClientOpts(1, "", false,
		option.WithEndpoint(listener.Addr().String()),
		option.WithGRPCDialOption(grpc.WithTransportCredentials(insecure.NewCredentials())),
		option.WithoutAuthentication(),
	)
	client, err := vkit.NewClient(context.Background(), opts...)
	if err != nil {
		t.Fatal(err)
	}
	defer client.Close()

	result, err := client.ExecuteSql(context.Background(), &sppb.ExecuteSqlRequest{Sql: invalidUTF8})
	if err != nil {
		t.Fatalf("ExecuteSql with VT-only wire data: %v", err)
	}
	if got := <-service.gotSQL; got != invalidUTF8 {
		t.Fatalf("server SQL = %q, want %q", got, invalidUTF8)
	}
	if got := result.GetRows()[0].GetValues()[0].GetStringValue(); got != invalidUTF8 {
		t.Fatalf("non-streaming response value = %q, want %q", got, invalidUTF8)
	}
}

type wirePartialResultStream struct {
	ctx         context.Context
	codec       *vtUnsafeCodec
	wires       [][]byte
	next        int
	headerCalls atomic.Int32
}

func (s *wirePartialResultStream) Header() (metadata.MD, error) {
	s.headerCalls.Add(1)
	return metadata.Pairs("server-timing", "gfet4t7; dur=1"), nil
}
func (s *wirePartialResultStream) Trailer() metadata.MD     { return nil }
func (s *wirePartialResultStream) CloseSend() error         { return nil }
func (s *wirePartialResultStream) Context() context.Context { return s.ctx }
func (s *wirePartialResultStream) SendMsg(any) error        { return nil }
func (s *wirePartialResultStream) Recv() (*sppb.PartialResultSet, error) {
	msg := new(sppb.PartialResultSet)
	if err := s.RecvMsg(msg); err != nil {
		return nil, err
	}
	return msg, nil
}
func (s *wirePartialResultStream) RecvMsg(dst any) error {
	if s.next == len(s.wires) {
		return io.EOF
	}
	wire := append([]byte(nil), s.wires[s.next]...)
	s.next++
	return s.codec.Unmarshal(mem.BufferSlice{mem.SliceBuffer(wire)}, dst)
}

func TestVTUnsafeReceiverReturnsMessageToPoolAfterLifetime(t *testing.T) {
	source := &sppb.PartialResultSet{Values: []*structpb.Value{
		structpb.NewStringValue(strings.Repeat("pooled-value", 100)),
	}}
	wire, err := proto.Marshal(source)
	if err != nil {
		t.Fatal(err)
	}
	codec := new(vtUnsafeCodec)
	stream := &wirePartialResultStream{ctx: context.Background(), codec: codec, wires: [][]byte{wire, wire}}
	receiver := &vtUnsafeStreamingReceiver{Spanner_ExecuteStreamingSqlClient: stream, codec: codec}

	first, firstBuffer, err := receiver.recvWithBuffer()
	if err != nil {
		t.Fatal(err)
	}
	if len(first.Values) != 1 {
		t.Fatalf("first value count = %d, want 1", len(first.Values))
	}
	firstValue := first.Values[0]
	retainVTUnsafeBuffer(firstBuffer)
	releaseVTUnsafeBuffer(firstBuffer)
	if first.GetValues()[0].GetStringValue() != source.GetValues()[0].GetStringValue() {
		t.Fatal("message reset while retained row lifetime was live")
	}
	releaseVTUnsafeBuffer(firstBuffer)
	if len(first.Values) != 0 {
		t.Fatal("PartialResultSet was not reset when its final owner released it")
	}

	second, secondBuffer, err := receiver.recvWithBuffer()
	if err != nil {
		t.Fatal(err)
	}
	// sync.Pool may discard an item at any time. When it returns this item,
	// verify the generated decoder also recycled the outer Value object.
	if second == first && second.Values[0] != firstValue {
		t.Fatal("structpb.Value object was not reused from VT pool")
	}
	releaseVTUnsafeBuffer(secondBuffer)
	if len(second.Values) != 0 {
		t.Fatal("second PartialResultSet was not reset on release")
	}
}

func TestStreamingHeaderIsReadOnce(t *testing.T) {
	stream := &wirePartialResultStream{ctx: context.Background()}
	cached := &cachedExecuteStreamingSQLClient{Spanner_ExecuteStreamingSqlClient: stream}
	if _, err := cached.Header(); err != nil {
		t.Fatal(err)
	}
	if _, err := cached.Header(); err != nil {
		t.Fatal(err)
	}
	if got := stream.headerCalls.Load(); got != 1 {
		t.Fatalf("underlying Header calls = %d, want 1", got)
	}
}

func TestDisabledBuiltinMetricsTracerAllocatesNothing(t *testing.T) {
	factory := &builtinMetricsTracerFactory{enabled: false}
	if got := factory.newBuiltinMetricsTracer(context.Background()); got != nil {
		t.Fatalf("disabled tracer = %#v, want nil", got)
	}
	if allocs := testing.AllocsPerRun(1000, func() {
		if factory.newBuiltinMetricsTracer(context.Background()) != nil {
			panic("disabled tracer")
		}
	}); allocs != 0 {
		t.Fatalf("disabled tracer allocations = %v, want 0", allocs)
	}
}

func TestDefaultRowsAndDecodedStringsOutliveIterator(t *testing.T) {
	server, client, teardown := setupMockedTestServer(t)
	defer teardown()

	const sql = "SELECT retained_value"
	fields := []*sppb.StructType_Field{{Name: "retained_value", Type: &sppb.Type{Code: sppb.TypeCode_STRING}}}
	rows := make([]*structpb.ListValue, 32)
	wants := make([]string, len(rows))
	for i := range rows {
		wants[i] = strings.Repeat(string(rune('a'+i%26)), 2048) + string(rune(i))
		rows[i] = &structpb.ListValue{Values: []*structpb.Value{structpb.NewStringValue(wants[i])}}
	}
	if err := server.TestSpanner.PutStatementResult(sql, &StatementResult{
		Type: StatementResultResultSet,
		ResultSet: &sppb.ResultSet{
			Metadata: &sppb.ResultSetMetadata{RowType: &sppb.StructType{Fields: fields}},
			Rows:     rows,
		},
		SetLastFlag: true,
	}); err != nil {
		t.Fatal(err)
	}

	iter := client.Single().Query(context.Background(), NewStatement(sql))
	var retainedRows []*Row
	var retainedStrings []string
	for {
		row, err := iter.Next()
		if err == iterator.Done {
			break
		}
		if err != nil {
			t.Fatal(err)
		}
		retainedRows = append(retainedRows, row)
		var value string
		if err := row.Column(0, &value); err != nil {
			t.Fatal(err)
		}
		retainedStrings = append(retainedStrings, value)
	}
	iter.Stop()
	runtime.GC()

	for i, row := range retainedRows {
		var got string
		if err := row.Column(0, &got); err != nil {
			t.Fatalf("retained row %d: %v", i, err)
		}
		if got != wants[i] || retainedStrings[i] != wants[i] {
			t.Fatalf("retained row/string %d changed", i)
		}
	}
}

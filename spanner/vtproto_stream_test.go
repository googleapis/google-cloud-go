//go:build spanner_vtproto

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

package spanner

import (
	"context"
	"fmt"
	"io"
	"math"
	"reflect"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
	"unsafe"

	sppb "cloud.google.com/go/spanner/apiv1/spannerpb"
	. "cloud.google.com/go/spanner/internal/testutil"
	"cloud.google.com/go/spanner/internal/vtpb"
	"github.com/google/go-cmp/cmp"
	"github.com/googleapis/gax-go/v2"
	"google.golang.org/api/iterator"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/encoding"
	"google.golang.org/grpc/mem"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/encoding/protowire"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/testing/protocmp"
	"google.golang.org/protobuf/types/known/structpb"
	"google.golang.org/protobuf/types/known/timestamppb"
)

// countingBufferPool counts the buffers that are not returned yet.
type countingBufferPool struct {
	outstanding atomic.Int64
}

func (p *countingBufferPool) Get(length int) *[]byte {
	p.outstanding.Add(1)
	// gRPC does not return buffers below its pooling threshold to the pool.
	b := make([]byte, length, max(length, 4096))
	return &b
}

func (p *countingBufferPool) Put(*[]byte) { p.outstanding.Add(-1) }

// codecStream is a stream that decodes PartialResultSets with the gRPC codec
// of its RowIterator, like a gRPC stream does.
type codecStream struct {
	ctx         context.Context
	codec       encoding.CodecV2
	resumeToken []byte
	prs         []*sppb.PartialResultSet
	// err is returned after prs. io.EOF is returned if it is nil.
	err error
	// pool, if set, provides the receive buffers. Otherwise every other
	// message is split into two buffers, which the codec copies.
	pool     *countingBufferPool
	received int
	// drained is closed when RecvMsg is called after the last message.
	drained chan struct{}
	// recvCalled is set if Recv, which receives into a public
	// PartialResultSet, is called.
	recvCalled atomic.Bool
}

func (s *codecStream) Recv() (*sppb.PartialResultSet, error) {
	s.recvCalled.Store(true)
	return nil, io.EOF
}

func (s *codecStream) Context() context.Context { return s.ctx }

func (s *codecStream) RecvMsg(m any) error {
	if _, ok := m.(*vtpb.PartialResultSet); !ok {
		return fmt.Errorf("RecvMsg(%T), want *vtpb.PartialResultSet", m)
	}
	if len(s.prs) == 0 {
		if s.drained != nil {
			close(s.drained)
			s.drained = nil
		}
		if s.err != nil {
			return s.err
		}
		return io.EOF
	}
	b, err := proto.Marshal(s.prs[0])
	if err != nil {
		return err
	}
	s.prs = s.prs[1:]
	s.received++
	var data mem.BufferSlice
	switch {
	case s.pool != nil:
		buf := s.pool.Get(len(b))
		copy(*buf, b)
		data = mem.BufferSlice{mem.NewBuffer(buf, s.pool)}
	case s.received%2 == 0 && len(b) > 1:
		data = mem.BufferSlice{mem.SliceBuffer(b[:len(b)/2]), mem.SliceBuffer(b[len(b)/2:])}
	default:
		data = mem.BufferSlice{mem.SliceBuffer(b)}
	}
	// Like gRPC, free the received data after Unmarshal returns.
	defer data.Free()
	return s.codec.Unmarshal(data, m)
}

// codecFromOptions returns the gRPC codec that opts set.
func codecFromOptions(t *testing.T, opts []gax.CallOption) encoding.CodecV2 {
	t.Helper()
	var settings gax.CallSettings
	for _, opt := range opts {
		opt.Resolve(&settings)
	}
	for _, opt := range settings.GRPC {
		if codec, ok := opt.(grpc.ForceCodecV2CallOption); ok {
			return codec.CodecV2
		}
	}
	t.Fatal("no gRPC codec in the call options")
	return nil
}

// codecStreamIterator returns a RowIterator that receives from the given
// streams, one per stream that it starts.
func codecStreamIterator(t *testing.T, streams ...*codecStream) *RowIterator {
	t.Helper()
	return stream(context.Background(), nil, nil,
		func(ctx context.Context, resumeToken []byte, opts ...gax.CallOption) (streamingReceiver, error) {
			if len(streams) == 0 {
				t.Fatal("the iterator started too many streams")
			}
			s := streams[0]
			streams = streams[1:]
			s.ctx = ctx
			s.codec = codecFromOptions(t, opts)
			s.resumeToken = resumeToken
			return s, nil
		},
		nil,
		func(error) {}, &grpcSpannerClient{nthRequest: new(atomic.Uint32)})
}

// readDetachedRows returns copies of the rows of iter and the error that ended
// it.
func readDetachedRows(iter *RowIterator) ([]*Row, error) {
	defer iter.Stop()
	var rows []*Row
	for {
		row, err := iter.Next()
		if err == iterator.Done {
			return rows, nil
		}
		if err != nil {
			return rows, err
		}
		rows = append(rows, detachRow(row))
	}
}

func TestVTRowIteratorAssemblesRows(t *testing.T) {
	restore := setMaxBytesBetweenResumeTokens()
	defer restore()
	for i, test := range partialResultSetDecoderTests() {
		s := &codecStream{prs: test.input}
		var ts time.Time
		iter := codecStreamIterator(t, s)
		iter.setTimestamp = func(t time.Time) { ts = t }
		rows, err := readDetachedRows(iter)
		if test.wantD && err != nil {
			t.Errorf("test %d: reading rows failed: %v", i, err)
		}
		if !test.wantD && ErrCode(err) != codes.FailedPrecondition {
			t.Errorf("test %d: reading rows returned %v, want FailedPrecondition", i, err)
		}
		if !testEqual(rows, test.wantF) {
			t.Errorf("test %d: rows=\n%v\n; want\n%v", i, describeRows(rows), describeRows(test.wantF))
		}
		if !test.wantTs.IsZero() && !ts.Equal(test.wantTs) {
			t.Errorf("test %d: read timestamp %v, want %v", i, ts, test.wantTs)
		}
		if s.received != len(test.input) {
			t.Errorf("test %d: the codec decoded %d PartialResultSets, want %d", i, s.received, len(test.input))
		}
	}
}

// vtTestPartialResultSet returns a PartialResultSet with all fields set.
func vtTestPartialResultSet() *sppb.PartialResultSet {
	listValue := structpb.NewListValue(&structpb.ListValue{Values: []*structpb.Value{
		structpb.NewStringValue("a"), structpb.NewNullValue(), structpb.NewNumberValue(1.5),
	}})
	structValue := structpb.NewStructValue(&structpb.Struct{Fields: map[string]*structpb.Value{
		"x": structpb.NewBoolValue(true), "y": structpb.NewStringValue("z"),
	}})
	return &sppb.PartialResultSet{
		Metadata: &sppb.ResultSetMetadata{
			RowType: &sppb.StructType{Fields: []*sppb.StructType_Field{
				{Name: "S", Type: &sppb.Type{Code: sppb.TypeCode_STRING}},
				{Name: "A", Type: &sppb.Type{Code: sppb.TypeCode_ARRAY, ArrayElementType: &sppb.Type{Code: sppb.TypeCode_STRING}}},
			}},
			Transaction: &sppb.Transaction{Id: []byte("tx"), ReadTimestamp: &timestamppb.Timestamp{Seconds: 100, Nanos: 7}},
		},
		Values: []*structpb.Value{
			structpb.NewStringValue("foo"),
			structpb.NewStringValue(""),
			structpb.NewNullValue(),
			structpb.NewNumberValue(-2.25),
			structpb.NewNumberValue(math.Inf(1)),
			structpb.NewBoolValue(true),
			structpb.NewBoolValue(false),
			listValue,
			structValue,
			{},
		},
		ChunkedValue: true,
		ResumeToken:  []byte("token"),
		Stats: &sppb.ResultSetStats{
			QueryPlan:  &sppb.QueryPlan{PlanNodes: []*sppb.PlanNode{{Index: 1, DisplayName: "scan"}}},
			QueryStats: &structpb.Struct{Fields: map[string]*structpb.Value{"rows": structpb.NewStringValue("3")}},
			RowCount:   &sppb.ResultSetStats_RowCountExact{RowCountExact: 3},
		},
		PrecommitToken: &sppb.MultiplexedSessionPrecommitToken{PrecommitToken: []byte("precommit"), SeqNum: 4},
		Last:           true,
		CacheUpdate:    &sppb.CacheUpdate{DatabaseId: 12},
	}
}

func TestVTDecodeMatchesProtoUnmarshal(t *testing.T) {
	want := vtTestPartialResultSet()
	b, err := proto.Marshal(want)
	if err != nil {
		t.Fatal(err)
	}
	// Fields that proto.Unmarshal keeps as unknown fields are skipped: an
	// unknown field, and a value with the wrong wire type.
	b = protowire.AppendTag(b, 99, protowire.BytesType)
	b = protowire.AppendBytes(b, []byte("unknown"))
	b = protowire.AppendTag(b, 2, protowire.VarintType)
	b = protowire.AppendVarint(b, 1)

	p := &vtPartialResultSet{}
	if err := p.decode(b); err != nil {
		t.Fatalf("decode failed: %v", err)
	}
	if err := p.convert(); err != nil {
		t.Fatalf("convert failed: %v", err)
	}
	if diff := cmpDiff(p.values, want.Values); diff != "" {
		t.Errorf("values mismatch (-got +want):\n%s", diff)
	}
	if diff := cmpDiff(p.metadata, want.Metadata); diff != "" {
		t.Errorf("metadata mismatch (-got +want):\n%s", diff)
	}
	if diff := cmpDiff(p.stats, want.Stats); diff != "" {
		t.Errorf("stats mismatch (-got +want):\n%s", diff)
	}
	if diff := cmpDiff(p.precommitToken, want.PrecommitToken); diff != "" {
		t.Errorf("precommit token mismatch (-got +want):\n%s", diff)
	}
	if string(p.resumeToken) != "token" || !p.chunkedValue || !p.last || p.size != len(b) {
		t.Errorf("got resume token %q, chunked %v, last %v, size %d; want %q, true, true, %d", p.resumeToken, p.chunkedValue, p.last, p.size, "token", len(b))
	}
	txID, cacheUpdate := partialResultSetRouting(&p.msg)
	if string(txID) != "tx" {
		t.Errorf("transaction ID %q, want %q", txID, "tx")
	}
	if diff := cmpDiff(cacheUpdate, want.CacheUpdate); diff != "" {
		t.Errorf("cache update mismatch (-got +want):\n%s", diff)
	}
}

func TestVTDecodeDropsUnknownValueFields(t *testing.T) {
	value, err := proto.Marshal(structpb.NewStringValue("s"))
	if err != nil {
		t.Fatal(err)
	}
	value = protowire.AppendTag(value, 99, protowire.VarintType)
	value = protowire.AppendVarint(value, 1)
	b := protowire.AppendTag(nil, 2, protowire.BytesType)
	b = protowire.AppendBytes(b, value)

	p := &vtPartialResultSet{}
	if err := p.decode(b); err != nil {
		t.Fatalf("decode failed: %v", err)
	}
	if diff := cmpDiff(p.msg.Values, []*structpb.Value{structpb.NewStringValue("s")}); diff != "" {
		t.Errorf("values mismatch (-got +want):\n%s", diff)
	}
}

func TestVTDecodeFallbackKeepsBoundedKinds(t *testing.T) {
	stringValue, err := proto.Marshal(structpb.NewStringValue("s"))
	if err != nil {
		t.Fatal(err)
	}
	nullValue, err := proto.Marshal(structpb.NewNullValue())
	if err != nil {
		t.Fatal(err)
	}
	// Such Values are decoded by the vtprotobuf fallback, not by
	// setScalarKind.
	unknownField := protowire.AppendVarint(protowire.AppendTag(append([]byte(nil), stringValue...), 99, protowire.VarintType), 1)
	duplicateField := append(append([]byte(nil), stringValue...), stringValue...)
	prs := func(value []byte) []byte {
		return protowire.AppendBytes(protowire.AppendTag(nil, 2, protowire.BytesType), value)
	}
	for name, values := range map[string][][]byte{
		"unknown field":            {unknownField},
		"duplicate field":          {duplicateField},
		"unknown field, then NULL": {unknownField, nullValue},
	} {
		t.Run(name, func(t *testing.T) {
			p := &vtPartialResultSet{}
			for i := 0; i < 100; i++ {
				for _, v := range values {
					p.reset()
					if err := p.decode(prs(v)); err != nil {
						t.Fatal(err)
					}
				}
			}
			k := p.kinds
			for kind, n := range map[string]int{"null": len(k.nulls), "number": len(k.numbers), "string": len(k.strings), "bool": len(k.bools)} {
				if n > cap(p.msg.Values) {
					t.Errorf("the PartialResultSet keeps %d unused %s kinds for %d Values, want at most one per Value", n, kind, cap(p.msg.Values))
				}
			}
		})
	}
}

func TestVTDecodeErrors(t *testing.T) {
	b, err := proto.Marshal(vtTestPartialResultSet())
	if err != nil {
		t.Fatal(err)
	}
	for _, n := range []int{1, len(b) / 2, len(b) - 1} {
		if err := (&vtPartialResultSet{}).decode(b[:n]); err == nil {
			t.Errorf("decode of the first %d of %d bytes succeeded, want an error", n, len(b))
		}
	}
}

func TestVTDecodeReusesValuesAndKinds(t *testing.T) {
	first, err := proto.Marshal(&sppb.PartialResultSet{Values: []*structpb.Value{
		structpb.NewStringValue("a"), structpb.NewNumberValue(1), structpb.NewBoolValue(true), structpb.NewNullValue(),
	}})
	if err != nil {
		t.Fatal(err)
	}
	// The same kinds with other values.
	second, err := proto.Marshal(&sppb.PartialResultSet{Values: []*structpb.Value{
		structpb.NewStringValue("b"), structpb.NewNumberValue(2), structpb.NewBoolValue(false), structpb.NewNullValue(),
	}})
	if err != nil {
		t.Fatal(err)
	}
	// The same kinds at other positions.
	third, err := proto.Marshal(&sppb.PartialResultSet{Values: []*structpb.Value{
		structpb.NewNullValue(), structpb.NewBoolValue(false), structpb.NewNumberValue(3), structpb.NewStringValue("c"),
	}})
	if err != nil {
		t.Fatal(err)
	}
	p := &vtPartialResultSet{}
	if err := p.decode(first); err != nil {
		t.Fatal(err)
	}
	values := append([]*structpb.Value(nil), p.msg.Values...)
	var kinds []uintptr
	for _, v := range values {
		kinds = append(kinds, reflect.ValueOf(v.Kind).Pointer())
	}
	p.reset()
	if err := p.decode(second); err != nil {
		t.Fatal(err)
	}
	for i, v := range p.msg.Values {
		if v != values[i] {
			t.Errorf("decode did not reuse Value message %d", i)
		}
	}
	for i, v := range p.msg.Values {
		if reflect.ValueOf(v.Kind).Pointer() != kinds[i] {
			t.Errorf("decode did not reuse the kind of value %d", i)
		}
	}
	if diff := cmpDiff(p.msg.Values, []*structpb.Value{
		structpb.NewStringValue("b"), structpb.NewNumberValue(2), structpb.NewBoolValue(false), structpb.NewNullValue(),
	}); diff != "" {
		t.Errorf("values mismatch (-got +want):\n%s", diff)
	}

	// Once the PartialResultSet has an unused kind of each type, decoding
	// kinds at other positions does not allocate either.
	allocs := testing.AllocsPerRun(100, func() {
		for _, b := range [][]byte{first, third} {
			p.reset()
			if err := p.decode(b); err != nil {
				t.Fatal(err)
			}
		}
	})
	if allocs != 0 {
		t.Errorf("decode allocated %v times, want 0", allocs)
	}
	if diff := cmpDiff(p.msg.Values, []*structpb.Value{
		structpb.NewNullValue(), structpb.NewBoolValue(false), structpb.NewNumberValue(3), structpb.NewStringValue("c"),
	}); diff != "" {
		t.Errorf("values mismatch (-got +want):\n%s", diff)
	}
}

func TestVTDecodeStringsReferenceReceiveBuffer(t *testing.T) {
	b, err := proto.Marshal(&sppb.PartialResultSet{Values: []*structpb.Value{structpb.NewStringValue("value")}})
	if err != nil {
		t.Fatal(err)
	}
	p := &vtPartialResultSet{}
	if err := p.decode(b); err != nil {
		t.Fatal(err)
	}
	s := p.msg.Values[0].GetStringValue()
	copy(b, make([]byte, len(b)))
	if s == "value" {
		t.Error("the decoded string does not reference the receive buffer")
	}
}

func TestVTRowIteratorReusesRow(t *testing.T) {
	s := &codecStream{prs: []*sppb.PartialResultSet{
		{Metadata: kvMeta, Values: []*structpb.Value{
			structpb.NewStringValue(keyStr(0)), structpb.NewStringValue(valStr(0)),
			structpb.NewStringValue(keyStr(1)), structpb.NewStringValue(valStr(1)),
		}, ResumeToken: EncodeResumeToken(2)},
		{Values: []*structpb.Value{
			structpb.NewStringValue(keyStr(2)), structpb.NewStringValue(valStr(2)),
		}, ResumeToken: EncodeResumeToken(3)},
	}}
	iter := codecStreamIterator(t, s)
	defer iter.Stop()
	var first *Row
	var key string
	for i := 0; i < 3; i++ {
		row, err := iter.Next()
		if err != nil {
			t.Fatalf("Next %d failed: %v", i, err)
		}
		if first == nil {
			first = row
		} else if row != first {
			t.Errorf("Next %d returned another *Row", i)
		}
		if i == 2 && key == keyStr(1) {
			// The first PartialResultSet was released, and its receive
			// buffer overwritten.
			t.Error("a string of a released row still has its value")
		}
		if err := row.Column(0, &key); err != nil {
			t.Fatal(err)
		}
		if key != keyStr(i) {
			t.Errorf("row %d has key %q, want %q", i, key, keyStr(i))
		}
	}
	if _, err := iter.Next(); err != iterator.Done {
		t.Errorf("Next returned %v, want iterator.Done", err)
	}
}

func TestVTRowIteratorDrainsWithPrivateMessage(t *testing.T) {
	drained := make(chan struct{})
	s := &codecStream{drained: drained, prs: []*sppb.PartialResultSet{{
		Metadata:    kvMeta,
		Values:      []*structpb.Value{structpb.NewStringValue(keyStr(0)), structpb.NewStringValue(valStr(0))},
		ResumeToken: EncodeResumeToken(1),
		Last:        true,
	}}}
	rows, err := readDetachedRows(codecStreamIterator(t, s))
	if err != nil || len(rows) != 1 {
		t.Fatalf("got %d rows and error %v, want 1 row", len(rows), err)
	}
	// After the last PartialResultSet, the iterator receives the end of the
	// stream in the background.
	select {
	case <-drained:
	case <-time.After(10 * time.Second):
		t.Fatal("the stream was not drained after the last PartialResultSet")
	}
	if s.recvCalled.Load() {
		t.Error("the stream was drained with Recv, which builds a public PartialResultSet")
	}
}

func TestVTRowIteratorNextDoesNotAllocate(t *testing.T) {
	const rows = 200
	values := make([]*structpb.Value, rows)
	for i := range values {
		values[i] = structpb.NewStringValue(fmt.Sprint(i))
	}
	s := &codecStream{prs: []*sppb.PartialResultSet{{
		Metadata: &sppb.ResultSetMetadata{RowType: &sppb.StructType{Fields: []*sppb.StructType_Field{
			{Name: "Value", Type: &sppb.Type{Code: sppb.TypeCode_INT64}},
		}}},
		Values:      values,
		ResumeToken: EncodeResumeToken(rows),
	}}}
	iter := codecStreamIterator(t, s)
	defer iter.Stop()
	if _, err := iter.Next(); err != nil {
		t.Fatalf("Next() failed: %v", err)
	}
	var v int64
	allocs := testing.AllocsPerRun(rows/2, func() {
		row, err := iter.Next()
		if err != nil {
			t.Fatalf("Next() failed: %v", err)
		}
		if err := row.Column(0, &v); err != nil {
			t.Fatal(err)
		}
	})
	if allocs != 0 {
		t.Errorf("Next() and Column allocated %v times per row, want 0", allocs)
	}
}

func TestVTRowIteratorResumes(t *testing.T) {
	prs := func(i int) *sppb.PartialResultSet {
		r := &sppb.PartialResultSet{
			Values:      []*structpb.Value{structpb.NewStringValue(keyStr(i)), structpb.NewStringValue(valStr(i))},
			ResumeToken: EncodeResumeToken(uint64(i + 1)),
		}
		if i == 0 {
			r.Metadata = kvMeta
		}
		return r
	}
	pool := &countingBufferPool{}
	first := &codecStream{prs: []*sppb.PartialResultSet{prs(0), prs(1), prs(2)}, err: status.Error(codes.Unavailable, "retry"), pool: pool}
	second := &codecStream{prs: []*sppb.PartialResultSet{prs(3), prs(4)}, pool: pool}
	iter := codecStreamIterator(t, first, second)
	iter.streamd.backoff = gax.Backoff{Initial: time.Millisecond, Max: time.Millisecond}
	rows, err := readDetachedRows(iter)
	if err != nil {
		t.Fatalf("reading rows failed: %v", err)
	}
	var want []*Row
	for i := 0; i < 5; i++ {
		want = append(want, &Row{fields: kvMeta.RowType.Fields, vals: []*structpb.Value{structpb.NewStringValue(keyStr(i)), structpb.NewStringValue(valStr(i))}})
	}
	if !testEqual(rows, want) {
		t.Errorf("rows=\n%v\n; want\n%v", describeRows(rows), describeRows(want))
	}
	if got, want := string(second.resumeToken), string(EncodeResumeToken(3)); got != want {
		t.Errorf("the second stream resumed at %q, want %q", got, want)
	}
	if n := pool.outstanding.Load(); n != 0 {
		t.Errorf("%d receive buffers were not freed", n)
	}
}

func TestVTRowIteratorStopReleasesBuffers(t *testing.T) {
	pool := &countingBufferPool{}
	s := &codecStream{pool: pool}
	for i := 0; i < 4; i++ {
		r := &sppb.PartialResultSet{Values: []*structpb.Value{structpb.NewStringValue(keyStr(i)), structpb.NewStringValue(valStr(i))}}
		if i == 0 {
			r.Metadata = kvMeta
		}
		// No resume tokens, so the stream decoder queues the PartialResultSets.
		s.prs = append(s.prs, r)
	}
	s.prs[1].ChunkedValue = true
	s.prs[2].Values[0] = structpb.NewStringValue("")
	iter := codecStreamIterator(t, s)
	if _, err := iter.Next(); err != nil {
		t.Fatalf("Next failed: %v", err)
	}
	iter.Stop()
	if n := pool.outstanding.Load(); n != 0 {
		t.Errorf("%d receive buffers were not freed by Stop", n)
	}
}

// vtTypesStatement is a query of a result that has values of several types.
const vtTypesStatement = "SELECT S, I, F, B, A FROM VTTypes"

var vtTypesMetadata = &sppb.ResultSetMetadata{RowType: &sppb.StructType{Fields: []*sppb.StructType_Field{
	{Name: "S", Type: &sppb.Type{Code: sppb.TypeCode_STRING}},
	{Name: "I", Type: &sppb.Type{Code: sppb.TypeCode_INT64}},
	{Name: "F", Type: &sppb.Type{Code: sppb.TypeCode_FLOAT64}},
	{Name: "B", Type: &sppb.Type{Code: sppb.TypeCode_BOOL}},
	{Name: "A", Type: &sppb.Type{Code: sppb.TypeCode_ARRAY, ArrayElementType: &sppb.Type{Code: sppb.TypeCode_STRING}}},
}}}

type vtTypesRow struct {
	S NullString
	I NullInt64
	F NullFloat64
	B NullBool
	A []NullString
}

func vtTypesRows(n int) []vtTypesRow {
	rows := make([]vtTypesRow, n)
	for i := range rows {
		rows[i] = vtTypesRow{
			F: NullFloat64{Float64: float64(i) + 0.5, Valid: true},
			B: NullBool{Bool: i%2 == 0, Valid: true},
			A: []NullString{{StringVal: fmt.Sprintf("a-%d", i), Valid: true}, {}},
		}
		if i%3 != 0 {
			rows[i].S = NullString{StringVal: fmt.Sprintf("string-%d", i), Valid: true}
		}
		if i%4 != 1 {
			rows[i].I = NullInt64{Int64: int64(i) * 1000, Valid: true}
		}
	}
	return rows
}

// putVTTypesResult adds the rows to the mock server as the result of
// vtTypesStatement and of a read of all columns of the table VTTypes.
func putVTTypesResult(t *testing.T, server *MockedSpannerInMemTestServer, rows []vtTypesRow) {
	t.Helper()
	rs := &sppb.ResultSet{Metadata: vtTypesMetadata}
	for _, r := range rows {
		var values []*structpb.Value
		for _, v := range []any{r.S, r.I, r.F, r.B, r.A} {
			pv, _, err := encodeValue(v)
			if err != nil {
				t.Fatal(err)
			}
			values = append(values, pv)
		}
		rs.Rows = append(rs.Rows, &structpb.ListValue{Values: values})
	}
	result := &StatementResult{Type: StatementResultResultSet, ResultSet: rs}
	if err := server.TestSpanner.PutStatementResult(vtTypesStatement, result); err != nil {
		t.Fatal(err)
	}
}

// readVTTypesRows reads the rows of iter, and checks that the iterator reuses
// the row, and that the codec decoded the rows.
func readVTTypesRows(iter *RowIterator) ([]vtTypesRow, error) {
	defer iter.Stop()
	var rows []vtTypesRow
	var first *Row
	for {
		row, err := iter.Next()
		if err == iterator.Done {
			return rows, nil
		}
		if err != nil {
			return rows, err
		}
		if first == nil {
			first = row
		} else if row != first {
			return rows, fmt.Errorf("Next returned another *Row")
		}
		// The rows are received with the vtprotobuf codec, into a receive
		// buffer that the codec keeps.
		for _, p := range iter.streamd.vt.rowSets {
			if !p.decoded || p.buf == nil {
				return rows, fmt.Errorf("the row was not decoded by the codec: decoded %v, buffer %v", p.decoded, p.buf != nil)
			}
		}
		var r vtTypesRow
		if err := row.ToStruct(&r); err != nil {
			return rows, err
		}
		// Copy the strings, which are only valid until the next call to Next.
		r.S.StringVal = strings.Clone(r.S.StringVal)
		r.A = []NullString{{StringVal: strings.Clone(r.A[0].StringVal), Valid: r.A[0].Valid}, r.A[1]}
		rows = append(rows, r)
	}
}

func checkVTTypesRows(t *testing.T, iter *RowIterator, want []vtTypesRow) {
	t.Helper()
	got, err := readVTTypesRows(iter)
	if err != nil {
		t.Fatalf("reading rows failed: %v", err)
	}
	if diff := cmpDiff(got, want); diff != "" {
		t.Errorf("rows mismatch (-got +want):\n%s", diff)
	}
}

func TestVTQueryAndRead(t *testing.T) {
	server, client, teardown := setupMockedTestServer(t)
	defer teardown()
	want := vtTypesRows(20)
	putVTTypesResult(t, server, want)
	ctx := context.Background()

	t.Run("Query", func(t *testing.T) {
		checkVTTypesRows(t, client.Single().Query(ctx, NewStatement(vtTypesStatement)), want)
	})
	t.Run("Read", func(t *testing.T) {
		checkVTTypesRows(t, client.Single().Read(ctx, "VTTypes", AllKeys(), []string{"S", "I", "F", "B", "A"}), want)
	})
	t.Run("ReadWriteTransaction", func(t *testing.T) {
		_, err := client.ReadWriteTransaction(ctx, func(ctx context.Context, tx *ReadWriteTransaction) error {
			checkVTTypesRows(t, tx.Query(ctx, NewStatement(vtTypesStatement)), want)
			return nil
		})
		if err != nil {
			t.Fatal(err)
		}
	})
}

func TestVTQueryResumes(t *testing.T) {
	server, client, teardown := setupMockedTestServer(t)
	defer teardown()
	want := vtTypesRows(10)
	putVTTypesResult(t, server, want)
	server.TestSpanner.AddPartialResultSetError(vtTypesStatement, PartialResultSetExecutionTime{
		ResumeToken: EncodeResumeToken(4),
		Err:         status.Error(codes.Unavailable, "retry"),
	})
	checkVTTypesRows(t, client.Single().Query(context.Background(), NewStatement(vtTypesStatement)), want)

	var resumeTokens [][]byte
	for _, req := range drainRequestsFromServer(server.TestSpanner) {
		if req, ok := req.(*sppb.ExecuteSqlRequest); ok {
			resumeTokens = append(resumeTokens, req.ResumeToken)
		}
	}
	if len(resumeTokens) != 2 || len(resumeTokens[0]) != 0 || len(resumeTokens[1]) == 0 {
		t.Errorf("got requests with resume tokens %q, want a request without and a request with a resume token", resumeTokens)
	}
}

func TestVTReadRowAndSelectAllReturnCopies(t *testing.T) {
	server, client, teardown := setupMockedTestServer(t)
	defer teardown()
	want := vtTypesRows(5)
	putVTTypesResult(t, server, want)
	ctx := context.Background()
	columns := []string{"S", "I", "F", "B", "A"}

	row, err := client.Single().ReadRow(ctx, "VTTypes", Key{"k"}, columns)
	if err != nil {
		t.Fatalf("ReadRow failed: %v", err)
	}
	// Overwrite the receive buffers of later responses.
	if _, err := readDetachedRows(client.Single().Query(ctx, NewStatement(vtTypesStatement))); err != nil {
		t.Fatal(err)
	}
	var got vtTypesRow
	if err := row.ToStruct(&got); err != nil {
		t.Fatal(err)
	}
	if diff := cmpDiff(got, want[0]); diff != "" {
		t.Errorf("ReadRow mismatch (-got +want):\n%s", diff)
	}

	var all []vtTypesRow
	if err := SelectAll(client.Single().Query(ctx, NewStatement(vtTypesStatement)), &all); err != nil {
		t.Fatalf("SelectAll failed: %v", err)
	}
	if diff := cmpDiff(all, want); diff != "" {
		t.Errorf("SelectAll mismatch (-got +want):\n%s", diff)
	}
}

func TestVTReadRowUsingIndexReturnsCopy(t *testing.T) {
	server, client, teardown := setupMockedTestServer(t)
	defer teardown()
	want := vtTypesRows(1)
	putVTTypesResult(t, server, want)
	row, err := client.Single().ReadRowUsingIndex(context.Background(), "VTTypes", "Idx", Key{"k"}, []string{"S", "I", "F", "B", "A"})
	if err != nil {
		t.Fatalf("ReadRowUsingIndex failed: %v", err)
	}
	var got vtTypesRow
	if err := row.ToStruct(&got); err != nil {
		t.Fatal(err)
	}
	if diff := cmpDiff(got, want[0]); diff != "" {
		t.Errorf("ReadRowUsingIndex mismatch (-got +want):\n%s", diff)
	}
}

func TestVTConcurrentQueries(t *testing.T) {
	server, client, teardown := setupMockedTestServer(t)
	defer teardown()
	want := vtTypesRows(30)
	putVTTypesResult(t, server, want)
	var wg sync.WaitGroup
	errs := make(chan error, 8)
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < 5; j++ {
				got, err := readVTTypesRows(client.Single().Query(context.Background(), NewStatement(vtTypesStatement)))
				if err == nil && !reflect.DeepEqual(got, want) {
					err = fmt.Errorf("rows mismatch:\n%s", cmpDiff(got, want))
				}
				if err != nil {
					errs <- err
					return
				}
			}
		}()
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		t.Error(err)
	}
}

func TestVTDetachRow(t *testing.T) {
	value := "detached"
	b := []byte(value)
	row := &Row{
		fields: []*sppb.StructType_Field{{Name: "S"}, {Name: "L"}},
		vals: []*structpb.Value{
			structpb.NewStringValue(unsafe.String(&b[0], len(b))),
			structpb.NewListValue(&structpb.ListValue{Values: []*structpb.Value{
				structpb.NewStructValue(&structpb.Struct{Fields: map[string]*structpb.Value{
					unsafe.String(&b[0], len(b)): structpb.NewStringValue(unsafe.String(&b[0], len(b))),
				}}),
			}}),
		},
	}
	detached := detachRow(row)
	if diff := cmpDiff(detached, row); diff != "" {
		t.Fatalf("detachRow mismatch (-got +want):\n%s", diff)
	}
	copy(b, "XXXXXXXX")
	if got := detached.vals[0].GetStringValue(); got != value {
		t.Errorf("detached string %q, want %q", got, value)
	}
	fields := detached.vals[1].GetListValue().GetValues()[0].GetStructValue().GetFields()
	if got := fields[value].GetStringValue(); got != value {
		t.Errorf("detached struct %v, want field %q with value %q", fields, value, value)
	}
}

// cmpDiff compares values that contain protocol buffer messages and rows.
func cmpDiff(got, want any) string {
	return cmp.Diff(got, want, protocmp.Transform(), cmp.AllowUnexported(Row{}))
}

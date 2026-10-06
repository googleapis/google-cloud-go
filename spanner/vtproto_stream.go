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

// This file is compiled only with the spanner_vtproto build tag. Every
// streaming query and read receives its PartialResultSets into the private
// vtprotobuf copy of google.spanner.v1 in internal/vtpb, with a gRPC codec
// that is installed for the streams of each RowIterator.
//
// Only ExecuteStreamingSql and StreamingRead use internal/vtpb. They are the
// RPCs that the client reads rows with, and their cost grows with the number of
// values that they return. The client builds and reads the messages of the
// other RPCs with the public spannerpb types, so decoding them with
// internal/vtpb would add a conversion of every request and response. For the
// same reason, the requests of the two streaming RPCs are marshaled from the
// public types.
//
// The codec
//
//   - keeps the gRPC receive buffer, so that the strings of the values
//     reference it instead of being copied,
//   - reuses the PartialResultSet, its google.protobuf.Value messages and
//     their scalar kinds once the iterator no longer uses them, and
//   - never builds a public sppb.PartialResultSet. The values of a
//     vtpb.PartialResultSet are google.protobuf.Value messages, so they become
//     the values of the rows as they are. Only the metadata, the stats and the
//     precommit token are converted to the public types, once per
//     PartialResultSet.
//
// The RowIterator returns the same *Row from every call to Next. The row, its
// values and everything decoded from them are only valid until the next call
// to Next or Stop. ReadRow, ReadRowUsingIndex and SelectAll return copies.
//
// The values are decoded like the vtprotobuf UnmarshalVTUnsafe methods, which
// differ from proto.Unmarshal in these accepted ways: they do not check that
// strings are valid UTF-8, they do not enforce a recursion limit, and the
// unknown fields of the values are dropped.
//
// Without the build tag, vtproto_stream_disabled.go is compiled instead, and
// no vtprotobuf code is linked into the binary.

import (
	"bytes"
	"context"
	"math"
	"strings"
	"sync"
	"unsafe"

	sppb "cloud.google.com/go/spanner/apiv1/spannerpb"
	"cloud.google.com/go/spanner/internal/vtpb"
	"github.com/googleapis/gax-go/v2"
	vtstructpb "github.com/planetscale/vtprotobuf/types/known/structpb"
	"google.golang.org/grpc"
	"google.golang.org/grpc/encoding"
	grpcproto "google.golang.org/grpc/encoding/proto"
	"google.golang.org/grpc/mem"
	"google.golang.org/protobuf/encoding/protowire"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/structpb"
)

// vtStream is the gRPC codec of the streams of one RowIterator, and the state
// that the iterator uses to return the rows of the PartialResultSets that the
// codec decodes. The iterator goroutine uses it, as gRPC calls the codec from
// RecvMsg. The only exception is drainStream, which receives the end of a
// stream on another goroutine once the iterator stopped receiving from it.
type vtStream struct {
	// receiving is the PartialResultSet that recvPartialResultSet receives.
	receiving *vtPartialResultSet

	// pending is the PartialResultSet whose values are being added to rows,
	// starting at pendingIndex.
	pending      *vtPartialResultSet
	pendingIndex int
	// partial are the PartialResultSets used by the row that is being
	// assembled.
	partial []*vtPartialResultSet
	// row is the row that the last call to next returned, and rowSets are the
	// PartialResultSets that it uses.
	row     Row
	rowSets []*vtPartialResultSet
}

var _ encoding.CodecV2 = (*vtStream)(nil)

// newVTStream returns the codec and the state of a new RowIterator.
func newVTStream() *vtStream { return &vtStream{} }

// withCodec returns rpc with s as the gRPC codec of the streams it starts.
func (s *vtStream) withCodec(rpc streamRPC) streamRPC {
	codec := codecCallOption{grpc.ForceCodecV2(s)}
	return func(ctx context.Context, resumeToken []byte, opts ...gax.CallOption) (streamingReceiver, error) {
		return rpc(ctx, resumeToken, append(opts[:len(opts):len(opts)], codec)...)
	}
}

// codecCallOption adds a gRPC call option. Unlike gax.WithGRPCOptions, it
// keeps the gRPC call options of the other gax call options.
type codecCallOption struct {
	opt grpc.CallOption
}

func (o codecCallOption) Resolve(s *gax.CallSettings) {
	s.GRPC = append(s.GRPC[:len(s.GRPC):len(s.GRPC)], o.opt)
}

var protoCodec = encoding.GetCodecV2(grpcproto.Name)

// Marshal marshals the requests with the gRPC protobuf codec.
func (s *vtStream) Marshal(v any) (mem.BufferSlice, error) {
	return protoCodec.Marshal(v)
}

// Unmarshal decodes the PartialResultSet that recvPartialResultSet receives.
// It decodes other messages with the gRPC protobuf codec.
func (s *vtStream) Unmarshal(data mem.BufferSlice, v any) error {
	m, ok := v.(*vtpb.PartialResultSet)
	if !ok || s.receiving == nil || m != &s.receiving.msg {
		return protoCodec.Unmarshal(data, v)
	}
	// The strings of the values reference buf, so keep it until the
	// PartialResultSet is released.
	buf := data.MaterializeToBuffer(mem.DefaultBufferPool())
	if err := s.receiving.decode(buf.ReadOnlyData()); err != nil {
		buf.Free()
		return err
	}
	s.receiving.buf = buf
	return nil
}

// Name returns an empty string, so that the default content subtype is kept on
// the wire.
func (*vtStream) Name() string { return "" }

// vtPartialResultSet is a received PartialResultSet. Its values reference buf.
// It is reused once the stream decoder, the iterator and the rows release it.
type vtPartialResultSet struct {
	// msg is the message that the codec decodes into.
	msg vtpb.PartialResultSet
	// buf is the receive buffer that the strings of the values reference.
	buf mem.Buffer
	// kinds are the unused kinds of the Value messages of msg.
	kinds scalarKinds
	// decoded is set if the codec decoded msg, and not another codec.
	decoded bool
	// pooled is set if p is reused once it is released.
	pooled bool
	refs   int

	// The fields of the PartialResultSet that the iterator uses.
	values         []*structpb.Value
	chunkedValue   bool
	resumeToken    []byte
	last           bool
	metadata       *sppb.ResultSetMetadata
	stats          *sppb.ResultSetStats
	precommitToken *sppb.MultiplexedSessionPrecommitToken
	size           int
}

// overwriteReleasedBuffers makes tests overwrite each receive buffer when it is
// released, so that data used after its PartialResultSet was released is
// detected.
var overwriteReleasedBuffers bool

// maxPooledValues bounds the size of the PartialResultSets that are reused.
const maxPooledValues = 1 << 16

var vtPartialResultSetPool = sync.Pool{
	New: func() any { return &vtPartialResultSet{pooled: true} },
}

func (p *vtPartialResultSet) GetResumeToken() []byte { return p.resumeToken }

func (p *vtPartialResultSet) GetLast() bool { return p.last }

// recvPartialResultSet receives the next PartialResultSet of stream. With s,
// it is a *vtPartialResultSet.
func recvPartialResultSet(s *vtStream, stream streamingReceiver) (receivedPartialResultSet, error) {
	if s == nil {
		prs, err := stream.Recv()
		if err != nil {
			return nil, err
		}
		return prs, nil
	}
	receiver, ok := stream.(interface{ RecvMsg(any) error })
	if !ok {
		// Only streams that are not gRPC streams, such as test fakes, do not
		// implement RecvMsg.
		prs, err := stream.Recv()
		if err != nil {
			return nil, err
		}
		return wrapPartialResultSet(prs), nil
	}
	p := vtPartialResultSetPool.Get().(*vtPartialResultSet)
	p.refs = 1
	s.receiving = p
	err := receiver.RecvMsg(&p.msg)
	s.receiving = nil
	if err == nil {
		err = p.convert()
	}
	if err != nil {
		p.release()
		return nil, err
	}
	return p, nil
}

// drainStream receives the end of stream after its last PartialResultSet, on
// another goroutine than the iterator. With s, it receives into a message of
// its own, so that no public PartialResultSet is built.
func drainStream(s *vtStream, stream streamingReceiver) {
	receiver, ok := stream.(interface{ RecvMsg(any) error })
	if s == nil || !ok {
		_, _ = stream.Recv()
		return
	}
	var m vtpb.PartialResultSet
	_ = receiver.RecvMsg(&m)
}

// wrapPartialResultSet returns a vtPartialResultSet with the contents of prs.
func wrapPartialResultSet(prs *sppb.PartialResultSet) *vtPartialResultSet {
	return &vtPartialResultSet{
		refs:           1,
		values:         prs.GetValues(),
		chunkedValue:   prs.GetChunkedValue(),
		resumeToken:    prs.GetResumeToken(),
		last:           prs.GetLast(),
		metadata:       prs.GetMetadata(),
		stats:          prs.GetStats(),
		precommitToken: prs.GetPrecommitToken(),
		size:           proto.Size(prs),
	}
}

// decode decodes the PartialResultSet b into p.msg. The strings of the values
// reference b. The Value messages of p.msg and their scalar kinds are reused.
func (p *vtPartialResultSet) decode(b []byte) error {
	m := &p.msg
	values := m.Values[:0]
	m.Reset()
	m.Values = values
	p.decoded = true
	p.size = len(b)
	for len(b) > 0 {
		num, typ, n := protowire.ConsumeTag(b)
		if n < 0 {
			return protowire.ParseError(n)
		}
		b = b[n:]
		switch typ {
		case protowire.VarintType:
			x, n := protowire.ConsumeVarint(b)
			if n < 0 {
				return protowire.ParseError(n)
			}
			b = b[n:]
			switch num {
			case 3:
				m.ChunkedValue = x != 0
			case 9:
				m.Last = x != 0
			}
		case protowire.BytesType:
			v, n := protowire.ConsumeBytes(b)
			if n < 0 {
				return protowire.ParseError(n)
			}
			b = b[n:]
			var err error
			switch num {
			case 1:
				if m.Metadata == nil {
					m.Metadata = &vtpb.ResultSetMetadata{}
				}
				err = m.Metadata.UnmarshalVT(v)
			case 2:
				i := len(m.Values)
				if i == cap(m.Values) {
					m.Values = append(m.Values, nil)
				} else {
					m.Values = m.Values[:i+1]
				}
				value := m.Values[i]
				if value == nil {
					value = &structpb.Value{}
					m.Values[i] = value
				}
				err = p.decodeValue(v, value)
			case 4:
				// The stream decoder keeps the resume token after p is
				// released.
				m.ResumeToken = bytes.Clone(v)
			case 5:
				if m.Stats == nil {
					m.Stats = &vtpb.ResultSetStats{}
				}
				err = m.Stats.UnmarshalVT(v)
			case 8:
				if m.PrecommitToken == nil {
					m.PrecommitToken = &vtpb.MultiplexedSessionPrecommitToken{}
				}
				err = m.PrecommitToken.UnmarshalVT(v)
			case 10:
				if m.CacheUpdate == nil {
					m.CacheUpdate = &vtpb.CacheUpdate{}
				}
				err = m.CacheUpdate.UnmarshalVT(v)
			}
			if err != nil {
				return err
			}
		default:
			n := protowire.ConsumeFieldValue(num, typ, b)
			if n < 0 {
				return protowire.ParseError(n)
			}
			b = b[n:]
		}
	}
	return nil
}

// decodeValue decodes the google.protobuf.Value b into v like the vtprotobuf
// UnmarshalVTUnsafe method, so that strings reference b. A Value that consists
// of a single scalar field reuses the kind of v, or an unused kind of p, of
// the same type.
func (p *vtPartialResultSet) decodeValue(b []byte, v *structpb.Value) error {
	if num, typ, n := protowire.ConsumeTag(b); n > 0 {
		if m := protowire.ConsumeFieldValue(num, typ, b[n:]); m >= 0 && n+m == len(b) && p.setScalarKind(v, num, typ, b[n:]) {
			return nil
		}
	}
	// The vtprotobuf decoder allocates the kind of v, so the current kind is
	// dropped rather than kept for reuse.
	v.Reset()
	if err := (*vtstructpb.Value)(v).UnmarshalVTUnsafe(b); err != nil {
		return err
	}
	// A reused Value must not keep unknown fields.
	if len(v.ProtoReflect().GetUnknown()) > 0 {
		v.ProtoReflect().SetUnknown(nil)
	}
	return nil
}

// setScalarKind sets the kind of v to the scalar field num with wire type typ
// and value b. It returns false for all other fields.
func (p *vtPartialResultSet) setScalarKind(v *structpb.Value, num protowire.Number, typ protowire.Type, b []byte) bool {
	switch {
	case num == 1 && typ == protowire.VarintType:
		x, _ := protowire.ConsumeVarint(b)
		k, ok := v.Kind.(*structpb.Value_NullValue)
		if !ok {
			p.kinds.put(v.Kind, cap(p.msg.Values))
			k = takeKind(&p.kinds.nulls)
			v.Kind = k
		}
		k.NullValue = structpb.NullValue(x)
	case num == 2 && typ == protowire.Fixed64Type:
		x, _ := protowire.ConsumeFixed64(b)
		k, ok := v.Kind.(*structpb.Value_NumberValue)
		if !ok {
			p.kinds.put(v.Kind, cap(p.msg.Values))
			k = takeKind(&p.kinds.numbers)
			v.Kind = k
		}
		k.NumberValue = math.Float64frombits(x)
	case num == 3 && typ == protowire.BytesType:
		x, _ := protowire.ConsumeBytes(b)
		k, ok := v.Kind.(*structpb.Value_StringValue)
		if !ok {
			p.kinds.put(v.Kind, cap(p.msg.Values))
			k = takeKind(&p.kinds.strings)
			v.Kind = k
		}
		k.StringValue = unsafe.String(unsafe.SliceData(x), len(x))
	case num == 4 && typ == protowire.VarintType:
		x, _ := protowire.ConsumeVarint(b)
		k, ok := v.Kind.(*structpb.Value_BoolValue)
		if !ok {
			p.kinds.put(v.Kind, cap(p.msg.Values))
			k = takeKind(&p.kinds.bools)
			v.Kind = k
		}
		k.BoolValue = x != 0
	default:
		return false
	}
	return true
}

// scalarKinds are kinds of Value messages that are not used.
type scalarKinds struct {
	nulls   []*structpb.Value_NullValue
	numbers []*structpb.Value_NumberValue
	strings []*structpb.Value_StringValue
	bools   []*structpb.Value_BoolValue
}

// put keeps the kind k for reuse if it is a scalar kind, and fewer than limit
// kinds of its type are kept. A PartialResultSet with limit Values never needs
// more.
func (s *scalarKinds) put(k any, limit int) {
	switch k := k.(type) {
	case *structpb.Value_NullValue:
		s.nulls = keepKind(s.nulls, k, limit)
	case *structpb.Value_NumberValue:
		s.numbers = keepKind(s.numbers, k, limit)
	case *structpb.Value_StringValue:
		k.StringValue = ""
		s.strings = keepKind(s.strings, k, limit)
	case *structpb.Value_BoolValue:
		s.bools = keepKind(s.bools, k, limit)
	}
}

func keepKind[K any](unused []*K, k *K, limit int) []*K {
	if len(unused) >= limit {
		return unused
	}
	return append(unused, k)
}

// takeKind returns an unused kind, or a new one.
func takeKind[K any](unused *[]*K) *K {
	n := len(*unused)
	if n == 0 {
		return new(K)
	}
	k := (*unused)[n-1]
	*unused = (*unused)[:n-1]
	return k
}

// convert sets the fields of p that the iterator uses from p.msg.
func (p *vtPartialResultSet) convert() error {
	m := &p.msg
	if !p.decoded {
		// Another codec decoded m, see vtStream.Unmarshal.
		p.size = proto.Size(m)
	}
	p.values = m.Values
	p.chunkedValue = m.ChunkedValue
	p.resumeToken = m.ResumeToken
	p.last = m.Last
	if m.Metadata != nil {
		p.metadata = &sppb.ResultSetMetadata{}
		if err := convertMessage(m.Metadata, p.metadata); err != nil {
			return err
		}
	}
	if m.Stats != nil {
		p.stats = &sppb.ResultSetStats{}
		if err := convertMessage(m.Stats, p.stats); err != nil {
			return err
		}
	}
	if t := m.PrecommitToken; t != nil {
		p.precommitToken = &sppb.MultiplexedSessionPrecommitToken{
			PrecommitToken: t.PrecommitToken,
			SeqNum:         t.SeqNum,
		}
	}
	return nil
}

// convertMessage sets dst to the vtpb message m, which has the same type.
func convertMessage(m interface{ MarshalVT() ([]byte, error) }, dst proto.Message) error {
	b, err := m.MarshalVT()
	if err != nil {
		return err
	}
	return proto.Unmarshal(b, dst)
}

// release releases a reference to p. The last one frees the receive buffer and
// keeps the messages of p for reuse.
func (p *vtPartialResultSet) release() {
	p.refs--
	if p.refs > 0 {
		return
	}
	if p.buf != nil {
		if overwriteReleasedBuffers {
			b := p.buf.ReadOnlyData()
			for i := range b {
				b[i] = 0xAA
			}
		}
		p.buf.Free()
		p.buf = nil
	}
	if !p.pooled {
		return
	}
	p.reset()
	if cap(p.msg.Values) <= maxPooledValues {
		vtPartialResultSetPool.Put(p)
	}
}

// reset prepares p for reuse. It keeps the Value messages of p and their
// scalar kinds, but no references to the data of the PartialResultSet.
func (p *vtPartialResultSet) reset() {
	values := p.msg.Values
	for _, v := range values {
		switch k := v.Kind.(type) {
		case *structpb.Value_StringValue:
			k.StringValue = ""
		case *structpb.Value_NullValue, *structpb.Value_NumberValue, *structpb.Value_BoolValue:
		default:
			v.Kind = nil
		}
	}
	p.msg.Reset()
	p.msg.Values = values
	p.decoded = false
	p.values = nil
	p.chunkedValue = false
	p.resumeToken = nil
	p.last = false
	p.metadata = nil
	p.stats = nil
	p.precommitToken = nil
	p.size = 0
}

// next implements RowIterator.next. It returns the same row from every call.
func (s *vtStream) next(r *RowIterator) (*Row, error) {
	releaseAll(s.rowSets)
	s.rowSets = s.rowSets[:0]
	for {
		if s.pending != nil {
			row, err := s.nextRow(r.rowd)
			if err != nil {
				r.err = err
				return nil, err
			}
			if row != nil {
				return row, nil
			}
			continue
		}
		if !r.streamd.next() {
			return nil, r.endOfResults()
		}
		p := r.streamd.get().(*vtPartialResultSet)
		s.pending, s.pendingIndex = p, 0
		if err := r.handlePartialResultSet(p.metadata, p.stats, p.precommitToken); err != nil {
			return nil, err
		}
		r.rowd.observeMetadata(p.metadata)
		if p.metadata != nil {
			r.Metadata = p.metadata
		}
		if !r.rowd.ts.IsZero() && r.setTimestamp != nil {
			r.setTimestamp(r.rowd.ts)
			r.setTimestamp = nil
		}
	}
}

// nextRow adds the pending values to rows until a row is complete, like
// partialResultSetDecoder.add. It returns a nil row once all pending values
// are added.
func (s *vtStream) nextRow(d *partialResultSetDecoder) (*Row, error) {
	p := s.pending
	for s.pendingIndex < len(p.values) {
		v := p.values[s.pendingIndex]
		last := s.pendingIndex == len(p.values)-1
		s.pendingIndex++
		s.partial = retain(s.partial, p)
		if d.chunked {
			d.chunked = false
			i := len(d.row.vals) - 1
			if i < 0 {
				return nil, errChunkedEmptyRow()
			}
			merged, err := d.merge(d.row.vals[i], v)
			if err != nil {
				return nil, err
			}
			d.row.vals[i] = merged
		} else {
			d.row.vals = append(d.row.vals, v)
		}
		if len(d.row.vals) == len(d.row.fields) && (!p.chunkedValue || !last) {
			s.row.fields = d.row.fields
			s.row.vals, d.row.vals = d.row.vals, s.row.vals[:0]
			s.rowSets, s.partial = s.partial, s.rowSets
			return &s.row, nil
		}
	}
	if len(p.values) > 0 && p.chunkedValue {
		d.chunked = true
	}
	s.pending = nil
	p.release()
	return nil, nil
}

// stop releases the PartialResultSets of the iterator and of the stream
// decoder d.
func (s *vtStream) stop(d *resumableStreamDecoder) {
	releaseAll(s.rowSets)
	releaseAll(s.partial)
	s.rowSets, s.partial = nil, nil
	if s.pending != nil {
		s.pending.release()
		s.pending = nil
	}
	s.row = Row{}
	d.q.clear()
	d.np = nil
}

// retain adds a reference to p to sets, unless p is already the last one.
func retain(sets []*vtPartialResultSet, p *vtPartialResultSet) []*vtPartialResultSet {
	if len(sets) > 0 && sets[len(sets)-1] == p {
		return sets
	}
	p.refs++
	return append(sets, p)
}

// releaseAll releases sets and clears them, so that they do not keep the
// PartialResultSets reachable.
func releaseAll(sets []*vtPartialResultSet) {
	for i, p := range sets {
		p.release()
		sets[i] = nil
	}
}

// partialResultSetSize returns the encoded size of r.
func partialResultSetSize(r receivedPartialResultSet) int {
	if p, ok := r.(*vtPartialResultSet); ok {
		return p.size
	}
	return proto.Size(r.(*sppb.PartialResultSet))
}

// releasePartialResultSet is called for a PartialResultSet that the iterator
// discards.
func releasePartialResultSet(r receivedPartialResultSet) {
	if p, ok := r.(*vtPartialResultSet); ok {
		p.release()
	}
}

// detachRow returns a deep copy of row, which stays valid after the next call
// to Next or Stop on the iterator that returned row.
func detachRow(row *Row) *Row {
	vals := make([]*structpb.Value, len(row.vals))
	for i, v := range row.vals {
		vals[i] = cloneValue(v)
	}
	return &Row{fields: row.fields, vals: vals}
}

// cloneValue returns a deep copy of v that shares no string storage with v.
// proto.Clone shares the storage of strings.
func cloneValue(v *structpb.Value) *structpb.Value {
	if v == nil {
		return nil
	}
	switch k := v.Kind.(type) {
	case *structpb.Value_StringValue:
		return &structpb.Value{Kind: &structpb.Value_StringValue{StringValue: strings.Clone(k.StringValue)}}
	case *structpb.Value_ListValue:
		if k.ListValue == nil {
			return &structpb.Value{Kind: &structpb.Value_ListValue{}}
		}
		values := make([]*structpb.Value, len(k.ListValue.Values))
		for i, e := range k.ListValue.Values {
			values[i] = cloneValue(e)
		}
		return &structpb.Value{Kind: &structpb.Value_ListValue{ListValue: &structpb.ListValue{Values: values}}}
	case *structpb.Value_StructValue:
		if k.StructValue == nil {
			return &structpb.Value{Kind: &structpb.Value_StructValue{}}
		}
		fields := make(map[string]*structpb.Value, len(k.StructValue.Fields))
		for name, f := range k.StructValue.Fields {
			fields[strings.Clone(name)] = cloneValue(f)
		}
		return &structpb.Value{Kind: &structpb.Value_StructValue{StructValue: &structpb.Struct{Fields: fields}}}
	default:
		return proto.Clone(v).(*structpb.Value)
	}
}

// partialResultSetRouting returns the transaction ID and the cache update of
// the PartialResultSet m that was received with RecvMsg.
func partialResultSetRouting(m any) ([]byte, *sppb.CacheUpdate) {
	switch m := m.(type) {
	case *sppb.PartialResultSet:
		return m.GetMetadata().GetTransaction().GetId(), m.GetCacheUpdate()
	case *vtpb.PartialResultSet:
		var cacheUpdate *sppb.CacheUpdate
		if m.CacheUpdate != nil {
			cacheUpdate = &sppb.CacheUpdate{}
			if err := convertMessage(m.CacheUpdate, cacheUpdate); err != nil {
				// A cache update is only a routing hint.
				cacheUpdate = nil
			}
		}
		return m.GetMetadata().GetTransaction().GetId(), cacheUpdate
	}
	return nil, nil
}

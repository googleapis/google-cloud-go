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
	"fmt"
	"sync/atomic"

	sppb "cloud.google.com/go/spanner/apiv1/spannerpb"
	"github.com/googleapis/gax-go/v2"
	"google.golang.org/grpc"
	"google.golang.org/grpc/encoding"
	"google.golang.org/grpc/mem"
	"google.golang.org/protobuf/proto"
)

type vtUnsafeCodec struct {
	retained *vtUnsafeBuffer
}

var _ encoding.CodecV2 = (*vtUnsafeCodec)(nil)

// vtSafeCodec is the connection-wide protobuf codec. Its empty name keeps the
// standard protobuf content subtype on the wire while allowing generated VT
// methods to handle both directions.
type vtSafeCodec struct{}

var _ encoding.CodecV2 = vtSafeCodec{}

type vtMarshaler interface {
	MarshalVT() ([]byte, error)
}

type vtUnmarshaler interface {
	UnmarshalVT([]byte) error
}

type vtUnsafeBuffer struct {
	buf       mem.Buffer
	rawValues [][]byte
	raw       bool
	refs      atomic.Int32
	message   *sppb.PartialResultSet
}

func newVTUnsafeBuffer(buf mem.Buffer, message *sppb.PartialResultSet) *vtUnsafeBuffer {
	retained := &vtUnsafeBuffer{buf: buf, message: message}
	retained.refs.Store(1)
	return retained
}

func marshalVTOrProto(v any) (mem.BufferSlice, error) {
	if msg, ok := v.(vtMarshaler); ok {
		buf, err := msg.MarshalVT()
		if err != nil {
			return nil, err
		}
		return mem.BufferSlice{mem.SliceBuffer(buf)}, nil
	}
	msg, ok := v.(proto.Message)
	if !ok {
		return nil, fmt.Errorf("spanner vt codec: cannot marshal %T", v)
	}
	buf, err := proto.Marshal(msg)
	if err != nil {
		return nil, err
	}
	return mem.BufferSlice{mem.SliceBuffer(buf)}, nil
}

func (vtSafeCodec) Marshal(v any) (mem.BufferSlice, error) {
	return marshalVTOrProto(v)
}

func (*vtUnsafeCodec) Marshal(v any) (mem.BufferSlice, error) {
	return marshalVTOrProto(v)
}

func (vtSafeCodec) Unmarshal(data mem.BufferSlice, v any) error {
	buf := data.MaterializeToBuffer(mem.DefaultBufferPool())
	defer buf.Free()
	if msg, ok := v.(vtUnmarshaler); ok {
		// gRPC's standard proto codec resets a reused destination before
		// unmarshalling. Preserve that behavior for VT-generated messages.
		if pm, ok := v.(proto.Message); ok {
			proto.Reset(pm)
		}
		return msg.UnmarshalVT(buf.ReadOnlyData())
	}
	msg, ok := v.(proto.Message)
	if !ok {
		return fmt.Errorf("spanner vt codec: cannot unmarshal into %T", v)
	}
	return proto.Unmarshal(buf.ReadOnlyData(), msg)
}

func (vtSafeCodec) Name() string { return "" }

func (c *vtUnsafeCodec) Unmarshal(data mem.BufferSlice, v any) error {
	switch msg := v.(type) {
	case *sppb.PartialResultSet:
		buf := retainBufferSlice(data)
		if err := msg.UnmarshalVTUnsafe(buf.ReadOnlyData()); err != nil {
			buf.Free()
			return err
		}
		c.retained = newVTUnsafeBuffer(buf, msg)
		return nil
	case vtUnmarshaler:
		buf := data.MaterializeToBuffer(mem.DefaultBufferPool())
		defer buf.Free()
		if pm, ok := v.(proto.Message); ok {
			proto.Reset(pm)
		}
		return msg.UnmarshalVT(buf.ReadOnlyData())
	case proto.Message:
		buf := data.MaterializeToBuffer(mem.DefaultBufferPool())
		defer buf.Free()
		return proto.Unmarshal(buf.ReadOnlyData(), msg)
	default:
		return fmt.Errorf("spanner vt codec: cannot unmarshal into %T", v)
	}
}

func (*vtUnsafeCodec) Name() string { return "" }

func (c *vtUnsafeCodec) takeRetained() *vtUnsafeBuffer {
	retained := c.retained
	c.retained = nil
	return retained
}

func retainBufferSlice(data mem.BufferSlice) mem.Buffer {
	if len(data) == 1 {
		data[0].Ref()
		return data[0]
	}
	return data.MaterializeToBuffer(mem.DefaultBufferPool())
}

func retainVTUnsafeBuffer(buf *vtUnsafeBuffer) *vtUnsafeBuffer {
	if buf == nil {
		return nil
	}
	buf.refs.Add(1)
	buf.buf.Ref()
	return buf
}

func releaseVTUnsafeBuffer(buf *vtUnsafeBuffer) {
	if buf == nil {
		return
	}
	buf.buf.Free()
	if buf.refs.Add(-1) == 0 && buf.message != nil {
		buf.message.ReturnToVTPool()
		buf.message = nil
	}
}

func releaseVTUnsafeBufferRefs(buffers []*vtUnsafeBuffer) {
	for _, buf := range buffers {
		releaseVTUnsafeBuffer(buf)
	}
}

func retainVTUnsafePartialResultSetForIterator(prs *sppb.PartialResultSet, buf *vtUnsafeBuffer) func() {
	if prs == nil || buf == nil {
		return nil
	}
	if buf.raw {
		// Raw envelope fields use safe unmarshalling. Raw column values have
		// separate row-owned buffer references.
		return nil
	}
	if prs.Metadata == nil && prs.Stats == nil && prs.PrecommitToken == nil && prs.CacheUpdate == nil {
		return nil
	}
	retainVTUnsafeBuffer(buf)
	return func() { releaseVTUnsafeBuffer(buf) }
}

type vtUnsafeStreamingReceiver struct {
	sppb.Spanner_ExecuteStreamingSqlClient
	codec *vtUnsafeCodec
}

// recvWithBuffer returns the receive buffer immediately after RecvMsg. gRPC
// serializes RecvMsg calls on a stream, so codec state needs no synchronization.
func (r *vtUnsafeStreamingReceiver) recvWithBuffer() (*sppb.PartialResultSet, *vtUnsafeBuffer, error) {
	msg := sppb.PartialResultSetFromVTPool()
	err := r.Spanner_ExecuteStreamingSqlClient.RecvMsg(msg)
	buf := r.codec.takeRetained()
	if err != nil {
		releaseVTUnsafeBuffer(buf)
		if buf == nil {
			msg.ReturnToVTPool()
		}
		return nil, nil, err
	}
	return msg, buf, nil
}

func (r *vtUnsafeStreamingReceiver) Recv() (*sppb.PartialResultSet, error) {
	msg, buf, err := r.recvWithBuffer()
	if err != nil {
		return nil, err
	}
	// recvWithBuffer is used by the iterator and transfers ownership through
	// vtUnsafeBuffer. Recv has no ownership return value, so make a safe copy
	// before releasing the pooled message and receive buffer.
	result := proto.Clone(msg).(*sppb.PartialResultSet)
	releaseVTUnsafeBuffer(buf)
	return result, nil
}

func (g *grpcSpannerClient) ExecuteStreamingSqlVTUnsafe(ctx context.Context, req *sppb.ExecuteSqlRequest, opts ...gax.CallOption) (sppb.Spanner_ExecuteStreamingSqlClient, error) {
	codec := &vtUnsafeCodec{}
	opts = append(opts, gax.WithGRPCOptions(grpc.ForceCodecV2(codec)))
	stream, err := g.ExecuteStreamingSql(ctx, req, opts...)
	if err != nil {
		return nil, err
	}
	return &vtUnsafeStreamingReceiver{Spanner_ExecuteStreamingSqlClient: stream, codec: codec}, nil
}

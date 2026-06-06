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
	"sync"

	sppb "cloud.google.com/go/spanner/apiv1/spannerpb"
	"github.com/googleapis/gax-go/v2"
	"google.golang.org/grpc"
	"google.golang.org/grpc/encoding"
	"google.golang.org/grpc/mem"
	"google.golang.org/protobuf/proto"
)

type vtUnsafeCodec struct{}

var _ encoding.CodecV2 = vtUnsafeCodec{}

type vtUnsafeBuffer struct {
	buf mem.Buffer
}

var vtUnsafeBuffers sync.Map // map[*sppb.PartialResultSet]*vtUnsafeBuffer

func (vtUnsafeCodec) Marshal(v any) (mem.BufferSlice, error) {
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

func (vtUnsafeCodec) Unmarshal(data mem.BufferSlice, v any) error {
	switch msg := v.(type) {
	case *sppb.PartialResultSet:
		buf := retainBufferSlice(data)
		if err := msg.UnmarshalVTUnsafe(buf.ReadOnlyData()); err != nil {
			buf.Free()
			return err
		}
		vtUnsafeBuffers.Store(msg, &vtUnsafeBuffer{buf: buf})
		return nil
	case *sppb.ResultSet:
		buf := retainBufferSlice(data)
		if err := msg.UnmarshalVTUnsafe(buf.ReadOnlyData()); err != nil {
			buf.Free()
			return err
		}
		// ResultSet is not used by the streaming path. Keep this branch for
		// targeted experiments only; callers must not let msg outlive the RPC
		// receive buffer unless they add a matching lifetime owner.
		buf.Free()
		return nil
	case proto.Message:
		buf := data.MaterializeToBuffer(mem.DefaultBufferPool())
		defer buf.Free()
		return proto.Unmarshal(buf.ReadOnlyData(), msg)
	default:
		return fmt.Errorf("spanner vt codec: cannot unmarshal into %T", v)
	}
}

func (vtUnsafeCodec) Name() string { return "" }

func retainBufferSlice(data mem.BufferSlice) mem.Buffer {
	if len(data) == 1 {
		data[0].Ref()
		return data[0]
	}
	return data.MaterializeToBuffer(mem.DefaultBufferPool())
}

func retainVTUnsafePartialResultSet(prs *sppb.PartialResultSet) func() {
	buf := retainVTUnsafePartialResultSetBuffer(prs)
	if buf == nil {
		return nil
	}
	return func() { buf.buf.Free() }
}

func retainVTUnsafePartialResultSetBuffer(prs *sppb.PartialResultSet) *vtUnsafeBuffer {
	if prs == nil {
		return nil
	}
	v, ok := vtUnsafeBuffers.Load(prs)
	if !ok {
		return nil
	}
	buf := v.(*vtUnsafeBuffer)
	buf.buf.Ref()
	return buf
}

func releaseVTUnsafeBufferRefs(buffers []*vtUnsafeBuffer) {
	for _, buf := range buffers {
		if buf != nil {
			buf.buf.Free()
		}
	}
}

func releaseVTUnsafePartialResultSet(prs *sppb.PartialResultSet) {
	if prs == nil {
		return
	}
	v, ok := vtUnsafeBuffers.LoadAndDelete(prs)
	if !ok {
		return
	}
	v.(*vtUnsafeBuffer).buf.Free()
}

func retainVTUnsafePartialResultSetForIterator(prs *sppb.PartialResultSet) func() {
	if prs == nil {
		return nil
	}
	if prs.Metadata == nil && prs.Stats == nil && prs.PrecommitToken == nil && prs.CacheUpdate == nil {
		return nil
	}
	return retainVTUnsafePartialResultSet(prs)
}

func (g *grpcSpannerClient) ExecuteStreamingSqlVTUnsafe(ctx context.Context, req *sppb.ExecuteSqlRequest, opts ...gax.CallOption) (sppb.Spanner_ExecuteStreamingSqlClient, error) {
	opts = append(opts, gax.WithGRPCOptions(grpc.ForceCodecV2(vtUnsafeCodec{})))
	return g.raw.ExecuteStreamingSql(ctx, req, opts...)
}

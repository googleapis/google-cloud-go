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
	"io"
	"log"
	"sync"
	"time"

	sppb "cloud.google.com/go/spanner/apiv1/spannerpb"
	"github.com/googleapis/gax-go/v2"
	"google.golang.org/api/iterator"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/encoding"
	"google.golang.org/grpc/mem"
	"google.golang.org/protobuf/encoding/protowire"
	"google.golang.org/protobuf/proto"
)

type rawBytesCodec struct{}

var _ encoding.CodecV2 = rawBytesCodec{}

func (rawBytesCodec) Marshal(v any) (mem.BufferSlice, error) {
	msg, ok := v.(proto.Message)
	if !ok {
		return nil, fmt.Errorf("spanner raw codec: cannot marshal %T", v)
	}
	buf, err := proto.Marshal(msg)
	if err != nil {
		return nil, err
	}
	return mem.BufferSlice{mem.SliceBuffer(buf)}, nil
}

func (rawBytesCodec) Unmarshal(data mem.BufferSlice, v any) error {
	switch v := v.(type) {
	case *mem.BufferSlice:
		*v = data
		data.Ref()
		return nil
	case proto.Message:
		buf := data.MaterializeToBuffer(mem.DefaultBufferPool())
		defer buf.Free()
		return proto.Unmarshal(buf.ReadOnlyData(), v)
	default:
		return fmt.Errorf("spanner raw codec: cannot unmarshal into %T", v)
	}
}

func (rawBytesCodec) Name() string { return "" }

type rawStreamingReceiver struct {
	grpc.ClientStream
}

func (g *grpcSpannerClient) ExecuteStreamingSqlRaw(ctx context.Context, req *sppb.ExecuteSqlRequest, opts ...gax.CallOption) (*rawStreamingReceiver, error) {
	settings := gax.CallSettings{}
	for _, opt := range opts {
		opt.Resolve(&settings)
	}
	settings.GRPC = append(settings.GRPC, grpc.ForceCodecV2(rawBytesCodec{}))
	stream, err := g.raw.Connection().NewStream(ctx, &sppb.Spanner_ServiceDesc.Streams[0], sppb.Spanner_ExecuteStreamingSql_FullMethodName, settings.GRPC...)
	if err != nil {
		return nil, err
	}
	if err := stream.SendMsg(req); err != nil {
		return nil, err
	}
	if err := stream.CloseSend(); err != nil {
		return nil, err
	}
	return &rawStreamingReceiver{ClientStream: stream}, nil
}

func (r *rawStreamingReceiver) RecvRaw(needMetadata bool) (*rawPartialResultSet, error) {
	var data mem.BufferSlice
	if err := r.RecvMsg(&data); err != nil {
		return nil, err
	}
	prs, err := decodeRawPartialResultSet(data, needMetadata)
	data.Free()
	if err != nil {
		return nil, err
	}
	return prs, nil
}

type rawPartialResultSet struct {
	metadata *sppb.ResultSetMetadata
	values   [][]byte
	last     bool
	data     mem.Buffer
}

type rawRowData struct {
	vals    [][]byte
	release func()
}

var rawRows sync.Map // map[*Row]rawRowData

func rawValsForRow(row *Row) ([][]byte, bool) {
	if row == nil {
		return nil, false
	}
	v, ok := rawRows.Load(row)
	if !ok {
		return nil, false
	}
	return v.(rawRowData).vals, true
}

func setRawRow(row *Row, vals [][]byte, release func()) {
	rawRows.Store(row, rawRowData{vals: vals, release: release})
}

func releaseRawRow(row *Row) {
	if row == nil {
		return
	}
	v, ok := rawRows.LoadAndDelete(row)
	if !ok {
		return
	}
	if release := v.(rawRowData).release; release != nil {
		release()
	}
}

func (r *rawPartialResultSet) free() {
	if r.data != nil {
		r.data.Free()
		r.data = nil
	}
}

func (r *rawPartialResultSet) ref() {
	if r.data != nil {
		r.data.Ref()
	}
}

func decodeRawPartialResultSet(data mem.BufferSlice, needMetadata bool) (*rawPartialResultSet, error) {
	var buf mem.Buffer
	if len(data) == 1 {
		data[0].Ref()
		buf = data[0]
	} else {
		buf = data.MaterializeToBuffer(mem.DefaultBufferPool())
	}
	wire := buf.ReadOnlyData()
	prs := &rawPartialResultSet{data: buf}
	for len(wire) > 0 {
		num, typ, n := protowire.ConsumeTag(wire)
		if n < 0 {
			prs.free()
			return nil, protowire.ParseError(n)
		}
		wire = wire[n:]
		switch {
		case num == 1 && typ == protowire.BytesType:
			b, n := protowire.ConsumeBytes(wire)
			if n < 0 {
				prs.free()
				return nil, protowire.ParseError(n)
			}
			if needMetadata {
				prs.metadata = &sppb.ResultSetMetadata{}
				if err := proto.Unmarshal(b, prs.metadata); err != nil {
					prs.free()
					return nil, err
				}
			}
			wire = wire[n:]
		case num == 2 && typ == protowire.BytesType:
			b, n := protowire.ConsumeBytes(wire)
			if n < 0 {
				prs.free()
				return nil, protowire.ParseError(n)
			}
			v, err := decodeRawStringValue(b)
			if err != nil {
				prs.free()
				return nil, err
			}
			prs.values = append(prs.values, v)
			wire = wire[n:]
		case num == 9 && typ == protowire.VarintType:
			v, n := protowire.ConsumeVarint(wire)
			if n < 0 {
				prs.free()
				return nil, protowire.ParseError(n)
			}
			prs.last = v != 0
			wire = wire[n:]
		default:
			n := protowire.ConsumeFieldValue(num, typ, wire)
			if n < 0 {
				prs.free()
				return nil, protowire.ParseError(n)
			}
			wire = wire[n:]
		}
	}
	return prs, nil
}

func decodeRawStringValue(wire []byte) ([]byte, error) {
	for len(wire) > 0 {
		num, typ, n := protowire.ConsumeTag(wire)
		if n < 0 {
			return nil, protowire.ParseError(n)
		}
		wire = wire[n:]
		if num == 3 && typ == protowire.BytesType {
			v, n := protowire.ConsumeBytes(wire)
			if n < 0 {
				return nil, protowire.ParseError(n)
			}
			return v, nil
		}
		n = protowire.ConsumeFieldValue(num, typ, wire)
		if n < 0 {
			return nil, protowire.ParseError(n)
		}
		wire = wire[n:]
	}
	return nil, fmt.Errorf("spanner raw decode: missing string_value")
}

type rawStreamDecoder struct {
	ctx    context.Context
	cancel func()
	rpc    func(context.Context, []byte, ...gax.CallOption) (*rawStreamingReceiver, error)
	stream *rawStreamingReceiver
	err    error
	done   bool
}

type rawPartialResultSetDecoder struct {
	fields []*sppb.StructType_Field
	vals   [][]byte
}

func rawStreamWithTransactionCallbacks(
	ctx context.Context,
	_ *log.Logger,
	meterTracerFactory *builtinMetricsTracerFactory,
	rpc func(context.Context, []byte, ...gax.CallOption) (*rawStreamingReceiver, error),
	setTransactionID func(transactionID),
	updateTxState func(error) error,
	updatePrecommitToken func(*sppb.MultiplexedSessionPrecommitToken),
	setTimestamp func(time.Time),
	release func(error),
	_ bool,
	_ bool,
) *RowIterator {
	ctx, cancel := context.WithCancel(ctx)
	ctx, _ = startSpan(ctx, "RowIterator")
	return &RowIterator{
		ctx:                  ctx,
		meterTracerFactory:   meterTracerFactory,
		rawStreamd:           &rawStreamDecoder{ctx: ctx, cancel: cancel, rpc: rpc},
		rawRowd:              &rawPartialResultSetDecoder{},
		setTransactionID:     setTransactionID,
		updatePrecommitToken: updatePrecommitToken,
		updateTxState:        updateTxState,
		setTimestamp:         setTimestamp,
		release:              release,
		cancel:               cancel,
	}
}

func (r *RowIterator) nextRaw() (*Row, error) {
	if r.lastRawRow != nil {
		releaseRawRow(r.lastRawRow)
		r.lastRawRow = nil
	}
	if r.err != nil {
		return nil, r.err
	}
	for len(r.rows) == 0 {
		prs, err := r.rawStreamd.next(r.rawRowd.fields == nil)
		if err != nil {
			if err == io.EOF {
				r.cancel = nil
				r.err = iterator.Done
			} else {
				r.err = r.updateTxState(ToSpannerError(err))
			}
			return nil, r.err
		}
		if prs.metadata != nil {
			r.Metadata = prs.metadata
			if r.rawRowd.fields == nil && prs.metadata.RowType != nil {
				r.rawRowd.fields = prs.metadata.RowType.Fields
			}
			if r.setTransactionID != nil && prs.metadata.Transaction != nil {
				r.setTransactionID(prs.metadata.Transaction.GetId())
				r.setTransactionID = nil
			}
		}
		r.rows, r.err = r.rawRowd.add(prs)
		prs.free()
		if r.err != nil {
			return nil, r.err
		}
		if prs.last && len(r.rows) == 0 {
			r.err = iterator.Done
			return nil, r.err
		}
	}
	row := r.rows[0]
	r.rows = r.rows[1:]
	r.lastRawRow = row
	return row, nil
}

func (d *rawStreamDecoder) next(needMetadata bool) (*rawPartialResultSet, error) {
	if d.done {
		return nil, io.EOF
	}
	if d.stream == nil {
		d.stream, d.err = d.rpc(d.ctx, nil)
		if d.err != nil {
			d.done = true
			return nil, d.err
		}
	}
	prs, err := d.stream.RecvRaw(needMetadata)
	if err != nil {
		d.done = true
		if err == io.EOF && d.cancel != nil {
			d.cancel()
		}
		return nil, err
	}
	if prs.last {
		d.done = true
	}
	return prs, nil
}

func (p *rawPartialResultSetDecoder) add(prs *rawPartialResultSet) ([]*Row, error) {
	if p.fields == nil {
		return nil, spannerErrorf(codes.FailedPrecondition, "missing metadata for raw decode")
	}
	if len(prs.values) == 0 {
		return nil, nil
	}
	var rows []*Row
	for _, v := range prs.values {
		p.vals = append(p.vals, v)
		if len(p.vals) == len(p.fields) {
			rawVals := make([][]byte, len(p.vals))
			copy(rawVals, p.vals)
			prs.ref()
			buf := prs.data
			row := &Row{fields: p.fields}
			setRawRow(row, rawVals, func() { buf.Free() })
			rows = append(rows, row)
			p.vals = p.vals[:0]
		}
	}
	return rows, nil
}

func (d *rawStreamDecoder) finish() {
	if d.cancel != nil {
		d.cancel()
	}
}

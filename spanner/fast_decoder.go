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
	"math"
	"reflect"
	"unsafe"

	sppb "cloud.google.com/go/spanner/apiv1/spannerpb"
	"google.golang.org/grpc/codes"
	"google.golang.org/protobuf/encoding/protowire"
	"google.golang.org/protobuf/proto"
	proto3 "google.golang.org/protobuf/types/known/structpb"
)

type customSpannerCodec struct {
	rowd *partialResultSetDecoder
}

func newCustomSpannerCodec(rowd *partialResultSetDecoder) customSpannerCodec {
	return customSpannerCodec{rowd: rowd}
}

func (c customSpannerCodec) Name() string {
	return "spanner-streaming-codec"
}

func (c customSpannerCodec) Marshal(v interface{}) ([]byte, error) {
	return proto.Marshal(v.(proto.Message))
}

func (c customSpannerCodec) Unmarshal(data []byte, v interface{}) error {
	if prs, ok := v.(*sppb.PartialResultSet); ok && c.rowd != nil && c.rowd.fastDecoding {
		return c.rowd.decodeFastPartialResultSet(data, prs)
	}
	return proto.Unmarshal(data, v.(proto.Message))
}

type SpannerValue struct {
	valType   int // 1=null, 2=number, 3=string, 4=bool, 5=struct, 6=list
	strVal    string
	numVal    float64
	boolVal   bool
	isNil     bool
	listVal   []*SpannerValue
	structVal map[string]*SpannerValue

	protoVal   *proto3.Value
	strKind    *proto3.Value_StringValue
	numKind    *proto3.Value_NumberValue
	boolKind   *proto3.Value_BoolValue
	nullKind   *proto3.Value_NullValue
	structKind *proto3.Value_StructValue
}

func (v *SpannerValue) toProto() *proto3.Value {
	if v.protoVal == nil {
		v.protoVal = &proto3.Value{}
		v.strKind = &proto3.Value_StringValue{}
		v.numKind = &proto3.Value_NumberValue{}
		v.boolKind = &proto3.Value_BoolValue{}
		v.nullKind = &proto3.Value_NullValue{}
		v.structKind = &proto3.Value_StructValue{}
	}
	switch v.valType {
	case 1:
		v.protoVal.Kind = v.nullKind
	case 2:
		v.numKind.NumberValue = v.numVal
		v.protoVal.Kind = v.numKind
	case 3:
		v.strKind.StringValue = v.strVal
		v.protoVal.Kind = v.strKind
	case 4:
		v.boolKind.BoolValue = v.boolVal
		v.protoVal.Kind = v.boolKind
	case 5:
		fields := make(map[string]*proto3.Value)
		for k, s := range v.structVal {
			fields[k] = s.toProto()
		}
		v.structKind.StructValue = &proto3.Struct{Fields: fields}
		v.protoVal.Kind = v.structKind
	case 6:
		var list []*proto3.Value
		for i := range v.listVal {
			list = append(list, v.listVal[i].toProto())
		}
		v.protoVal.Kind = &proto3.Value_ListValue{ListValue: &proto3.ListValue{Values: list}}
	default:
		v.protoVal.Kind = v.nullKind
	}
	return v.protoVal
}

type fastRowData struct {
	cells []SpannerValue
}

type fastRowKind struct {
	fastRow *fastRowData
}

func (fastRowKind) isValue_Kind() {}

func (p *partialResultSetDecoder) decodeFastPartialResultSet(data []byte, prs *sppb.PartialResultSet) error {
	for len(data) > 0 {
		num, wire, n := protowire.ConsumeTag(data)
		if n < 0 {
			return spannerErrorf(codes.Internal, "corrupt protobuf tag")
		}
		data = data[n:]
		switch num {
		case 1: // metadata
			if wire != protowire.BytesType {
				return spannerErrorf(codes.Internal, "invalid metadata wire type")
			}
			b, n := protowire.ConsumeBytes(data)
			if n < 0 {
				return spannerErrorf(codes.Internal, "corrupt metadata bytes")
			}
			data = data[n:]
			prs.Metadata = &sppb.ResultSetMetadata{}
			if err := proto.Unmarshal(b, prs.Metadata); err != nil {
				return err
			}
			if p.row.fields == nil {
				p.row.fields = prs.Metadata.RowType.Fields
			}
		case 4: // resume_token
			if wire != protowire.BytesType {
				return spannerErrorf(codes.Internal, "invalid resume token wire type")
			}
			b, n := protowire.ConsumeBytes(data)
			if n < 0 {
				return spannerErrorf(codes.Internal, "corrupt resume token bytes")
			}
			data = data[n:]
			prs.ResumeToken = append([]byte(nil), b...)
		case 3: // chunked_value
			if wire != protowire.VarintType {
				return spannerErrorf(codes.Internal, "invalid chunked value wire type")
			}
			b, n := protowire.ConsumeVarint(data)
			if n < 0 {
				return spannerErrorf(codes.Internal, "corrupt chunked value")
			}
			data = data[n:]
			prs.ChunkedValue = b != 0
		case 5: // stats
			if wire != protowire.BytesType {
				return spannerErrorf(codes.Internal, "invalid stats wire type")
			}
			b, n := protowire.ConsumeBytes(data)
			if n < 0 {
				return spannerErrorf(codes.Internal, "corrupt stats bytes")
			}
			data = data[n:]
			prs.Stats = &sppb.ResultSetStats{}
			if err := proto.Unmarshal(b, prs.Stats); err != nil {
				return err
			}
		case 2: // values
			if wire != protowire.BytesType {
				return spannerErrorf(codes.Internal, "invalid values wire type")
			}
			b, n := protowire.ConsumeBytes(data)
			if n < 0 {
				return spannerErrorf(codes.Internal, "corrupt values bytes")
			}
			data = data[n:]

			if p.curFastRow == nil {
				if len(p.fastPool) > 0 {
					p.curFastRow = p.fastPool[len(p.fastPool)-1]
					p.fastPool = p.fastPool[:len(p.fastPool)-1]
					p.curFastRow.cells = p.curFastRow.cells[:0]
				} else {
					p.curFastRow = &fastRowData{cells: make([]SpannerValue, 0, len(p.row.fields))}
				}
			}

			var cell *SpannerValue
			if len(p.curFastRow.cells) < cap(p.curFastRow.cells) {
				p.curFastRow.cells = p.curFastRow.cells[:len(p.curFastRow.cells)+1]
				cell = &p.curFastRow.cells[len(p.curFastRow.cells)-1]
			} else {
				p.curFastRow.cells = append(p.curFastRow.cells, SpannerValue{})
				cell = &p.curFastRow.cells[len(p.curFastRow.cells)-1]
			}
			cell.valType = 0
			cell.strVal = ""
			cell.numVal = 0
			cell.boolVal = false
			cell.isNil = false
			cell.listVal = nil

			if err := decodeFastSpannerValueBytes(b, cell); err != nil {
				return err
			}
			if len(p.row.fields) > 0 && len(p.curFastRow.cells) == len(p.row.fields) {
				p.completedFastRows = append(p.completedFastRows, p.curFastRow)
				p.curFastRow = nil
			}
		case 8: // precommit_token
			if wire != protowire.BytesType {
				return spannerErrorf(codes.Internal, "invalid precommit token wire type")
			}
			b, n := protowire.ConsumeBytes(data)
			if n < 0 {
				return spannerErrorf(codes.Internal, "corrupt precommit token bytes")
			}
			data = data[n:]
			prs.PrecommitToken = &sppb.MultiplexedSessionPrecommitToken{}
			if err := proto.Unmarshal(b, prs.PrecommitToken); err != nil {
				return err
			}
		default:
			vn := protowire.ConsumeFieldValue(num, wire, data)
			if vn < 0 {
				return spannerErrorf(codes.Internal, "corrupt field value")
			}
			data = data[vn:]
		}
	}
	return nil
}

func decodeFastSpannerValueBytes(valData []byte, cell *SpannerValue) error {
	for len(valData) > 0 {
		vnum, vwire, vn := protowire.ConsumeTag(valData)
		if vn < 0 {
			return spannerErrorf(codes.Internal, "corrupt value tag")
		}
		valData = valData[vn:]
		switch vnum {
		case 1: // null_value
			_, vn := protowire.ConsumeVarint(valData)
			valData = valData[vn:]
			cell.valType = 1
			cell.isNil = true
		case 2: // number_value
			num, vn := protowire.ConsumeFixed64(valData)
			valData = valData[vn:]
			cell.valType = 2
			cell.numVal = math.Float64frombits(num)
		case 3: // string_value
			strBytes, vn := protowire.ConsumeBytes(valData)
			valData = valData[vn:]
			cell.valType = 3
			cell.strVal = string(strBytes)
		case 4: // bool_value
			bv, vn := protowire.ConsumeVarint(valData)
			valData = valData[vn:]
			cell.valType = 4
			cell.boolVal = bv != 0
		case 5: // struct_value
			structBytes, vn := protowire.ConsumeBytes(valData)
			valData = valData[vn:]
			cell.valType = 5
			cell.structVal = decodeFastStructValue(structBytes)
		case 6: // list_value
			listBytes, vn := protowire.ConsumeBytes(valData)
			valData = valData[vn:]
			cell.valType = 6
			cell.listVal = decodeFastListValue(listBytes)
		default:
			vn := protowire.ConsumeFieldValue(vnum, vwire, valData)
			valData = valData[vn:]
		}
	}
	return nil
}

func decodeFastStructValue(b []byte) map[string]*SpannerValue {
	m := make(map[string]*SpannerValue)
	for len(b) > 0 {
		num, wire, n := protowire.ConsumeTag(b)
		if n < 0 || num != 1 || wire != protowire.BytesType {
			break
		}
		b = b[n:]
		entryBytes, n := protowire.ConsumeBytes(b)
		if n < 0 {
			break
		}
		b = b[n:]

		var key string
		val := &SpannerValue{}
		entryData := entryBytes
		for len(entryData) > 0 {
			enum, ewire, en := protowire.ConsumeTag(entryData)
			if en < 0 {
				break
			}
			entryData = entryData[en:]
			switch enum {
			case 1:
				kb, en := protowire.ConsumeBytes(entryData)
				entryData = entryData[en:]
				key = string(kb)
			case 2:
				vb, en := protowire.ConsumeBytes(entryData)
				entryData = entryData[en:]
				_ = decodeFastSpannerValueBytes(vb, val)
			default:
				en = protowire.ConsumeFieldValue(enum, ewire, entryData)
				entryData = entryData[en:]
			}
		}
		if key != "" {
			m[key] = val
		}
	}
	return m
}

func decodeFastListValue(b []byte) []*SpannerValue {
	var list []*SpannerValue
	for len(b) > 0 {
		num, wire, n := protowire.ConsumeTag(b)
		if n < 0 || num != 1 || wire != protowire.BytesType {
			break
		}
		b = b[n:]
		valBytes, n := protowire.ConsumeBytes(b)
		if n < 0 {
			break
		}
		b = b[n:]

		item := &SpannerValue{}
		_ = decodeFastSpannerValueBytes(valBytes, item)
		list = append(list, item)
	}
	return list
}

func (p *partialResultSetDecoder) addFast(r *sppb.PartialResultSet) ([]*Row, *sppb.ResultSetMetadata, error) {
	var rows []*Row
	for _, fast := range p.completedFastRows {
		var fresh *Row
		if len(p.rowPool) > 0 {
			fresh = p.rowPool[len(p.rowPool)-1]
			p.rowPool = p.rowPool[:len(p.rowPool)-1]
		} else {
			fresh = &Row{}
		}
		fresh.fields = p.row.fields
		sh := (*reflect.SliceHeader)(unsafe.Pointer(&fresh.vals))
		sh.Data = uintptr(unsafe.Pointer(fast))
		sh.Len = 0
		sh.Cap = 0

		rows = append(rows, fresh)
	}
	p.completedFastRows = p.completedFastRows[:0]
	return rows, r.Metadata, nil
}

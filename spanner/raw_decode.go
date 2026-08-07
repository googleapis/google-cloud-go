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
	"database/sql"
	"encoding/base64"
	"math"
	"math/big"
	"strconv"
	"time"

	"cloud.google.com/go/civil"
	sppb "cloud.google.com/go/spanner/apiv1/spannerpb"
	"github.com/google/uuid"
	"google.golang.org/protobuf/encoding/protowire"
	"google.golang.org/protobuf/proto"
	structpb "google.golang.org/protobuf/types/known/structpb"
)

// unmarshalRawPartialResultSet decodes the PartialResultSet envelope while
// leaving each google.protobuf.Value as a slice of the receive buffer.
func unmarshalRawPartialResultSet(wire []byte, dst *sppb.PartialResultSet, decodeMetadata bool) ([][]byte, error) {
	valueCount, err := countRawPartialResultSetValues(wire)
	if err != nil {
		return nil, err
	}
	*dst = sppb.PartialResultSet{}
	rawValues := make([][]byte, 0, valueCount)
	merge := proto.UnmarshalOptions{Merge: true}
	for len(wire) > 0 {
		num, typ, n := protowire.ConsumeTag(wire)
		if n < 0 {
			return nil, protowire.ParseError(n)
		}
		wire = wire[n:]
		switch {
		case num == 1 && typ == protowire.BytesType:
			b, consumed := protowire.ConsumeBytes(wire)
			if consumed < 0 {
				return nil, protowire.ParseError(consumed)
			}
			if decodeMetadata {
				if dst.Metadata == nil {
					dst.Metadata = new(sppb.ResultSetMetadata)
				}
				if err := merge.Unmarshal(b, dst.Metadata); err != nil {
					return nil, err
				}
			}
			wire = wire[consumed:]
		case num == 2 && typ == protowire.BytesType:
			b, consumed := protowire.ConsumeBytes(wire)
			if consumed < 0 {
				return nil, protowire.ParseError(consumed)
			}
			rawValues = append(rawValues, b)
			wire = wire[consumed:]
		case num == 3 && typ == protowire.VarintType:
			v, consumed := protowire.ConsumeVarint(wire)
			if consumed < 0 {
				return nil, protowire.ParseError(consumed)
			}
			dst.ChunkedValue = v != 0
			wire = wire[consumed:]
		case num == 4 && typ == protowire.BytesType:
			b, consumed := protowire.ConsumeBytes(wire)
			if consumed < 0 {
				return nil, protowire.ParseError(consumed)
			}
			dst.ResumeToken = append(dst.ResumeToken[:0], b...)
			wire = wire[consumed:]
		case num == 5 && typ == protowire.BytesType:
			b, consumed := protowire.ConsumeBytes(wire)
			if consumed < 0 {
				return nil, protowire.ParseError(consumed)
			}
			if dst.Stats == nil {
				dst.Stats = new(sppb.ResultSetStats)
			}
			if err := merge.Unmarshal(b, dst.Stats); err != nil {
				return nil, err
			}
			wire = wire[consumed:]
		case num == 8 && typ == protowire.BytesType:
			b, consumed := protowire.ConsumeBytes(wire)
			if consumed < 0 {
				return nil, protowire.ParseError(consumed)
			}
			if dst.PrecommitToken == nil {
				dst.PrecommitToken = new(sppb.MultiplexedSessionPrecommitToken)
			}
			if err := merge.Unmarshal(b, dst.PrecommitToken); err != nil {
				return nil, err
			}
			wire = wire[consumed:]
		case num == 9 && typ == protowire.VarintType:
			v, consumed := protowire.ConsumeVarint(wire)
			if consumed < 0 {
				return nil, protowire.ParseError(consumed)
			}
			dst.Last = v != 0
			wire = wire[consumed:]
		case num == 10 && typ == protowire.BytesType:
			b, consumed := protowire.ConsumeBytes(wire)
			if consumed < 0 {
				return nil, protowire.ParseError(consumed)
			}
			if dst.CacheUpdate == nil {
				dst.CacheUpdate = new(sppb.CacheUpdate)
			}
			if err := merge.Unmarshal(b, dst.CacheUpdate); err != nil {
				return nil, err
			}
			wire = wire[consumed:]
		default:
			consumed := protowire.ConsumeFieldValue(num, typ, wire)
			if consumed < 0 {
				return nil, protowire.ParseError(consumed)
			}
			wire = wire[consumed:]
		}
	}
	return rawValues, nil
}

func countRawPartialResultSetValues(wire []byte) (int, error) {
	count := 0
	for len(wire) > 0 {
		num, typ, n := protowire.ConsumeTag(wire)
		if n < 0 {
			return 0, protowire.ParseError(n)
		}
		wire = wire[n:]
		if num == 2 && typ == protowire.BytesType {
			count++
		}
		n = protowire.ConsumeFieldValue(num, typ, wire)
		if n < 0 {
			return 0, protowire.ParseError(n)
		}
		wire = wire[n:]
	}
	return count, nil
}

type rawValueKind uint8

const (
	rawValueUnknown rawValueKind = iota
	rawValueNull
	rawValueNumber
	rawValueString
	rawValueBool
	rawValueStruct
	rawValueList
)

type rawValueView struct {
	kind    rawValueKind
	bytes   []byte
	number  float64
	boolean bool
}

func parseRawValue(wire []byte) (rawValueView, error) {
	var value rawValueView
	for len(wire) > 0 {
		num, typ, n := protowire.ConsumeTag(wire)
		if n < 0 {
			return rawValueView{}, protowire.ParseError(n)
		}
		wire = wire[n:]
		switch {
		case num == 1 && typ == protowire.VarintType:
			_, consumed := protowire.ConsumeVarint(wire)
			if consumed < 0 {
				return rawValueView{}, protowire.ParseError(consumed)
			}
			value = rawValueView{kind: rawValueNull}
			wire = wire[consumed:]
		case num == 2 && typ == protowire.Fixed64Type:
			bits, consumed := protowire.ConsumeFixed64(wire)
			if consumed < 0 {
				return rawValueView{}, protowire.ParseError(consumed)
			}
			value = rawValueView{kind: rawValueNumber, number: math.Float64frombits(bits)}
			wire = wire[consumed:]
		case num == 3 && typ == protowire.BytesType:
			b, consumed := protowire.ConsumeBytes(wire)
			if consumed < 0 {
				return rawValueView{}, protowire.ParseError(consumed)
			}
			value = rawValueView{kind: rawValueString, bytes: b}
			wire = wire[consumed:]
		case num == 4 && typ == protowire.VarintType:
			v, consumed := protowire.ConsumeVarint(wire)
			if consumed < 0 {
				return rawValueView{}, protowire.ParseError(consumed)
			}
			value = rawValueView{kind: rawValueBool, boolean: v != 0}
			wire = wire[consumed:]
		case num == 5 && typ == protowire.BytesType:
			b, consumed := protowire.ConsumeBytes(wire)
			if consumed < 0 {
				return rawValueView{}, protowire.ParseError(consumed)
			}
			value = rawValueView{kind: rawValueStruct, bytes: b}
			wire = wire[consumed:]
		case num == 6 && typ == protowire.BytesType:
			b, consumed := protowire.ConsumeBytes(wire)
			if consumed < 0 {
				return rawValueView{}, protowire.ParseError(consumed)
			}
			value = rawValueView{kind: rawValueList, bytes: b}
			wire = wire[consumed:]
		default:
			consumed := protowire.ConsumeFieldValue(num, typ, wire)
			if consumed < 0 {
				return rawValueView{}, protowire.ParseError(consumed)
			}
			wire = wire[consumed:]
		}
	}
	return value, nil
}

// decodeRawValue handles common scalar destinations without constructing a
// structpb.Value. Nested values and uncommon destination types use the normal
// decoder lazily so experimental raw decode never changes their semantics.
func decodeRawValue(raw []byte, typ *sppb.Type, ptr any) error {
	if typ == nil {
		return errNilSpannerType()
	}
	view, err := parseRawValue(raw)
	if err != nil {
		return err
	}
	if view.kind == rawValueNull {
		if handled, err := decodeRawNull(typ, ptr); handled {
			return err
		}
		return decodeRawValueFallback(raw, typ, ptr)
	}

	switch typ.Code {
	case sppb.TypeCode_STRING:
		if view.kind != rawValueString {
			break
		}
		switch p := ptr.(type) {
		case *string:
			if p == nil {
				return errNilDst(p)
			}
			*p = string(view.bytes)
			return nil
		case *NullString:
			if p == nil {
				return errNilDst(p)
			}
			*p = NullString{StringVal: string(view.bytes), Valid: true}
			return nil
		case **string:
			if p == nil {
				return errNilDst(p)
			}
			v := string(view.bytes)
			*p = &v
			return nil
		case *sql.NullString:
			if p == nil {
				return errNilDst(p)
			}
			*p = sql.NullString{String: string(view.bytes), Valid: true}
			return nil
		}
	case sppb.TypeCode_BYTES, sppb.TypeCode_PROTO:
		if view.kind != rawValueString {
			break
		}
		if p, ok := ptr.(*[]byte); ok {
			if p == nil {
				return errNilDst(p)
			}
			decoded := make([]byte, base64.StdEncoding.DecodedLen(len(view.bytes)))
			n, err := base64.StdEncoding.Decode(decoded, view.bytes)
			if err != nil {
				return decodeRawValueFallback(raw, typ, ptr)
			}
			*p = decoded[:n]
			return nil
		}
	case sppb.TypeCode_INT64, sppb.TypeCode_ENUM:
		if view.kind != rawValueString {
			break
		}
		v, err := strconv.ParseInt(string(view.bytes), 10, 64)
		if err != nil {
			return decodeRawValueFallback(raw, typ, ptr)
		}
		switch p := ptr.(type) {
		case *int64:
			if p == nil {
				return errNilDst(p)
			}
			*p = v
			return nil
		case *NullInt64:
			if p == nil {
				return errNilDst(p)
			}
			*p = NullInt64{Int64: v, Valid: true}
			return nil
		case **int64:
			if p == nil {
				return errNilDst(p)
			}
			*p = &v
			return nil
		}
	case sppb.TypeCode_BOOL:
		if view.kind != rawValueBool {
			break
		}
		switch p := ptr.(type) {
		case *bool:
			if p == nil {
				return errNilDst(p)
			}
			*p = view.boolean
			return nil
		case *NullBool:
			if p == nil {
				return errNilDst(p)
			}
			*p = NullBool{Bool: view.boolean, Valid: true}
			return nil
		case **bool:
			if p == nil {
				return errNilDst(p)
			}
			v := view.boolean
			*p = &v
			return nil
		}
	case sppb.TypeCode_FLOAT64:
		if view.kind != rawValueNumber {
			break
		}
		switch p := ptr.(type) {
		case *float64:
			if p == nil {
				return errNilDst(p)
			}
			*p = view.number
			return nil
		case *NullFloat64:
			if p == nil {
				return errNilDst(p)
			}
			*p = NullFloat64{Float64: view.number, Valid: true}
			return nil
		case **float64:
			if p == nil {
				return errNilDst(p)
			}
			v := view.number
			*p = &v
			return nil
		}
	case sppb.TypeCode_FLOAT32:
		if view.kind != rawValueNumber {
			break
		}
		v := float32(view.number)
		switch p := ptr.(type) {
		case *float32:
			if p == nil {
				return errNilDst(p)
			}
			*p = v
			return nil
		case *NullFloat32:
			if p == nil {
				return errNilDst(p)
			}
			*p = NullFloat32{Float32: v, Valid: true}
			return nil
		case **float32:
			if p == nil {
				return errNilDst(p)
			}
			*p = &v
			return nil
		}
	case sppb.TypeCode_NUMERIC:
		if view.kind != rawValueString {
			break
		}
		if typ.TypeAnnotation == sppb.TypeAnnotationCode_PG_NUMERIC {
			if p, ok := ptr.(*PGNumeric); ok {
				if p == nil {
					return errNilDst(p)
				}
				*p = PGNumeric{Numeric: string(view.bytes), Valid: true}
				return nil
			}
		}
		v, ok := new(big.Rat).SetString(string(view.bytes))
		if !ok {
			return decodeRawValueFallback(raw, typ, ptr)
		}
		switch p := ptr.(type) {
		case *big.Rat:
			if p == nil {
				return errNilDst(p)
			}
			*p = *v
			return nil
		case *NullNumeric:
			if p == nil {
				return errNilDst(p)
			}
			*p = NullNumeric{Numeric: *v, Valid: true}
			return nil
		case **big.Rat:
			if p == nil {
				return errNilDst(p)
			}
			*p = v
			return nil
		}
	case sppb.TypeCode_TIMESTAMP:
		if view.kind != rawValueString {
			break
		}
		v, err := time.Parse(time.RFC3339Nano, string(view.bytes))
		if err != nil {
			return decodeRawValueFallback(raw, typ, ptr)
		}
		switch p := ptr.(type) {
		case *time.Time:
			if p == nil {
				return errNilDst(p)
			}
			*p = v
			return nil
		case *NullTime:
			if p == nil {
				return errNilDst(p)
			}
			*p = NullTime{Time: v, Valid: true}
			return nil
		case **time.Time:
			if p == nil {
				return errNilDst(p)
			}
			*p = &v
			return nil
		}
	case sppb.TypeCode_DATE:
		if view.kind != rawValueString {
			break
		}
		v, err := civil.ParseDate(string(view.bytes))
		if err != nil {
			return decodeRawValueFallback(raw, typ, ptr)
		}
		switch p := ptr.(type) {
		case *civil.Date:
			if p == nil {
				return errNilDst(p)
			}
			*p = v
			return nil
		case *NullDate:
			if p == nil {
				return errNilDst(p)
			}
			*p = NullDate{Date: v, Valid: true}
			return nil
		case **civil.Date:
			if p == nil {
				return errNilDst(p)
			}
			*p = &v
			return nil
		}
	case sppb.TypeCode_JSON:
		if view.kind != rawValueString {
			break
		}
		var v any
		if err := jsonUnmarshal(view.bytes, &v); err != nil {
			return decodeRawValueFallback(raw, typ, ptr)
		}
		if typ.TypeAnnotation == sppb.TypeAnnotationCode_PG_JSONB {
			if p, ok := ptr.(*PGJsonB); ok {
				if p == nil {
					return errNilDst(p)
				}
				*p = PGJsonB{Value: v, Valid: true}
				return nil
			}
		}
		if p, ok := ptr.(*NullJSON); ok {
			if p == nil {
				return errNilDst(p)
			}
			*p = NullJSON{Value: v, Valid: true}
			return nil
		}
	case sppb.TypeCode_UUID:
		if view.kind != rawValueString {
			break
		}
		v, err := uuid.ParseBytes(view.bytes)
		if err != nil {
			return decodeRawValueFallback(raw, typ, ptr)
		}
		switch p := ptr.(type) {
		case *uuid.UUID:
			if p == nil {
				return errNilDst(p)
			}
			*p = v
			return nil
		case *NullUUID:
			if p == nil {
				return errNilDst(p)
			}
			*p = NullUUID{UUID: v, Valid: true}
			return nil
		case **uuid.UUID:
			if p == nil {
				return errNilDst(p)
			}
			*p = &v
			return nil
		}
	case sppb.TypeCode_INTERVAL:
		if view.kind != rawValueString {
			break
		}
		_, intervalDst := ptr.(*Interval)
		_, nullIntervalDst := ptr.(*NullInterval)
		if !intervalDst && !nullIntervalDst {
			break
		}
		v, err := ParseInterval(string(view.bytes))
		if err != nil {
			return decodeRawValueFallback(raw, typ, ptr)
		}
		switch p := ptr.(type) {
		case *Interval:
			if p == nil {
				return errNilDst(p)
			}
			*p = v
			return nil
		case *NullInterval:
			if p == nil {
				return errNilDst(p)
			}
			*p = NullInterval{Interval: v, Valid: true}
			return nil
		}
	}
	return decodeRawValueFallback(raw, typ, ptr)
}

func decodeRawNull(typ *sppb.Type, ptr any) (bool, error) {
	switch typ.Code {
	case sppb.TypeCode_STRING:
		switch p := ptr.(type) {
		case *NullString:
			if p == nil {
				return true, errNilDst(p)
			}
			*p = NullString{}
			return true, nil
		case **string:
			if p == nil {
				return true, errNilDst(p)
			}
			*p = nil
			return true, nil
		case *sql.NullString:
			if p == nil {
				return true, errNilDst(p)
			}
			*p = sql.NullString{}
			return true, nil
		}
	case sppb.TypeCode_BYTES, sppb.TypeCode_PROTO:
		if p, ok := ptr.(*[]byte); ok {
			if p == nil {
				return true, errNilDst(p)
			}
			*p = nil
			return true, nil
		}
	case sppb.TypeCode_INT64, sppb.TypeCode_ENUM:
		switch p := ptr.(type) {
		case *NullInt64:
			if p == nil {
				return true, errNilDst(p)
			}
			*p = NullInt64{}
			return true, nil
		case **int64:
			if p == nil {
				return true, errNilDst(p)
			}
			*p = nil
			return true, nil
		}
	case sppb.TypeCode_BOOL:
		switch p := ptr.(type) {
		case *NullBool:
			if p == nil {
				return true, errNilDst(p)
			}
			*p = NullBool{}
			return true, nil
		case **bool:
			if p == nil {
				return true, errNilDst(p)
			}
			*p = nil
			return true, nil
		}
	case sppb.TypeCode_FLOAT64:
		switch p := ptr.(type) {
		case *NullFloat64:
			if p == nil {
				return true, errNilDst(p)
			}
			*p = NullFloat64{}
			return true, nil
		case **float64:
			if p == nil {
				return true, errNilDst(p)
			}
			*p = nil
			return true, nil
		}
	case sppb.TypeCode_FLOAT32:
		switch p := ptr.(type) {
		case *NullFloat32:
			if p == nil {
				return true, errNilDst(p)
			}
			*p = NullFloat32{}
			return true, nil
		case **float32:
			if p == nil {
				return true, errNilDst(p)
			}
			*p = nil
			return true, nil
		}
	case sppb.TypeCode_NUMERIC:
		switch p := ptr.(type) {
		case *NullNumeric:
			if p == nil {
				return true, errNilDst(p)
			}
			*p = NullNumeric{}
			return true, nil
		case **big.Rat:
			if p == nil {
				return true, errNilDst(p)
			}
			*p = nil
			return true, nil
		case *PGNumeric:
			if typ.TypeAnnotation != sppb.TypeAnnotationCode_PG_NUMERIC {
				return false, nil
			}
			if p == nil {
				return true, errNilDst(p)
			}
			*p = PGNumeric{}
			return true, nil
		}
	case sppb.TypeCode_TIMESTAMP:
		switch p := ptr.(type) {
		case *NullTime:
			if p == nil {
				return true, errNilDst(p)
			}
			*p = NullTime{}
			return true, nil
		case **time.Time:
			if p == nil {
				return true, errNilDst(p)
			}
			*p = nil
			return true, nil
		}
	case sppb.TypeCode_DATE:
		switch p := ptr.(type) {
		case *NullDate:
			if p == nil {
				return true, errNilDst(p)
			}
			*p = NullDate{}
			return true, nil
		case **civil.Date:
			if p == nil {
				return true, errNilDst(p)
			}
			*p = nil
			return true, nil
		}
	case sppb.TypeCode_JSON:
		switch p := ptr.(type) {
		case *NullJSON:
			if p == nil {
				return true, errNilDst(p)
			}
			*p = NullJSON{}
			return true, nil
		case *PGJsonB:
			if typ.TypeAnnotation != sppb.TypeAnnotationCode_PG_JSONB {
				return false, nil
			}
			if p == nil {
				return true, errNilDst(p)
			}
			*p = PGJsonB{}
			return true, nil
		}
	case sppb.TypeCode_UUID:
		switch p := ptr.(type) {
		case *NullUUID:
			if p == nil {
				return true, errNilDst(p)
			}
			*p = NullUUID{}
			return true, nil
		case **uuid.UUID:
			if p == nil {
				return true, errNilDst(p)
			}
			*p = nil
			return true, nil
		}
	case sppb.TypeCode_INTERVAL:
		switch p := ptr.(type) {
		case *NullInterval:
			if p == nil {
				return true, errNilDst(p)
			}
			*p = NullInterval{}
			return true, nil
		}
	}
	return false, nil
}

func decodeRawValueFallback(raw []byte, typ *sppb.Type, ptr any) error {
	value, err := materializeRawValue(raw)
	if err != nil {
		return err
	}
	return decodeValue(value, typ, ptr)
}

func materializeRawValue(raw []byte) (*structpb.Value, error) {
	value := new(structpb.Value)
	if err := proto.Unmarshal(raw, value); err != nil {
		return nil, err
	}
	return value, nil
}

func mergeRawValues(a, b []byte) ([]byte, error) {
	av, err := parseRawValue(a)
	if err != nil {
		return nil, err
	}
	bv, err := parseRawValue(b)
	if err != nil {
		return nil, err
	}
	if av.kind == rawValueString && bv.kind == rawValueString {
		length := len(av.bytes) + len(bv.bytes)
		merged := make([]byte, 0, protowire.SizeTag(3)+protowire.SizeBytes(length))
		merged = protowire.AppendTag(merged, 3, protowire.BytesType)
		merged = protowire.AppendVarint(merged, uint64(length))
		merged = append(merged, av.bytes...)
		merged = append(merged, bv.bytes...)
		return merged, nil
	}
	// Nested ListValue/StructValue merging is uncommon and structurally
	// complex. Materialize only the split value, then reuse the proven merger.
	left, err := materializeRawValue(a)
	if err != nil {
		return nil, err
	}
	right, err := materializeRawValue(b)
	if err != nil {
		return nil, err
	}
	merged, err := new(partialResultSetDecoder).merge(left, right)
	if err != nil {
		return nil, err
	}
	return proto.Marshal(merged)
}

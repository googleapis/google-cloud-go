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

package sharedcore

import (
	"encoding/base64"
	"fmt"
	"math"
	"sync"
	"time"
	"unsafe"

	sppb "cloud.google.com/go/spanner/apiv1/spannerpb"
	proto3 "google.golang.org/protobuf/types/known/structpb"
)

const (
	maxPooledCells      = 32768
	maxPooledArenaBytes = 1024 * 1024
)

type arenaScratch struct {
	cells []PackedCell
	bytes []byte
}

var arenaScratchPool = sync.Pool{
	New: func() any {
		return &arenaScratch{
			cells: make([]PackedCell, 0, 256),
			bytes: make([]byte, 0, 4096),
		}
	},
}

func acquireArenaScratch(topCells, estBytes int) *arenaScratch {
	s := arenaScratchPool.Get().(*arenaScratch)
	if cap(s.cells) < topCells {
		s.cells = make([]PackedCell, topCells)
	} else {
		s.cells = s.cells[:topCells]
	}
	if cap(s.bytes) < estBytes {
		s.bytes = make([]byte, 0, estBytes)
	} else {
		s.bytes = s.bytes[:0]
	}
	return s
}

func releaseArenaScratch(s *arenaScratch) {
	if s == nil {
		return
	}
	if cap(s.cells) <= maxPooledCells && cap(s.bytes) <= maxPooledArenaBytes {
		arenaScratchPool.Put(s)
	}
}

// DecodeRows transforms a slice of raw protobuf Value rows (produced by Go Spanner's
// partialResultSetDecoder) into typed SpannerValue rows ready for C-arena serialization.
func DecodeRows(rawRows [][]*proto3.Value, fields []*sppb.StructType_Field) ([][]SpannerValue, error) {
	if len(rawRows) == 0 || len(fields) == 0 {
		return nil, nil
	}
	numCols := len(fields)
	out := make([][]SpannerValue, len(rawRows))
	// Allocate a single flat backing array for all top-level cells in the batch
	// to minimize Go heap allocations per row.
	flatCells := make([]SpannerValue, len(rawRows)*numCols)

	// Pre-extract column Type pointers and collect indices of columns that are (or contain) BYTES/PROTO.
	var colTypesStack [32]*sppb.Type
	var bytesColsStack [32]int
	var colTypes []*sppb.Type
	bytesColIndices := bytesColsStack[:0]
	if numCols <= len(colTypesStack) {
		colTypes = colTypesStack[:numCols]
	} else {
		colTypes = make([]*sppb.Type, numCols)
	}
	for cIdx := 0; cIdx < numCols; cIdx++ {
		if fields[cIdx] != nil {
			ct := fields[cIdx].Type
			colTypes[cIdx] = ct
			if hasBytesOrProto(ct) {
				bytesColIndices = append(bytesColIndices, cIdx)
			}
		}
	}

	var bytePool []byte
	if len(bytesColIndices) > 0 {
		totalB64 := 0
		for _, rawRow := range rawRows {
			for _, cIdx := range bytesColIndices {
				if cIdx >= len(rawRow) {
					break
				}
				totalB64 += estimateDecodedBytes(rawRow[cIdx], colTypes[cIdx])
			}
		}
		if totalB64 > 0 {
			bytePool = make([]byte, 0, totalB64)
		}
	}

	for rIdx, rawRow := range rawRows {
		rowSlice := flatCells[rIdx*numCols : (rIdx+1)*numCols : (rIdx+1)*numCols]
		limit := len(rawRow)
		if limit > numCols {
			limit = numCols
		}
		for cIdx := 0; cIdx < limit; cIdx++ {
			val, err := decodeCellValueWithPool(rawRow[cIdx], colTypes[cIdx], &bytePool)
			if err != nil {
				return nil, fmt.Errorf("sharedcore: row %d, col %d (%q): %w", rIdx, cIdx, fields[cIdx].GetName(), err)
			}
			rowSlice[cIdx] = val
		}
		out[rIdx] = rowSlice
	}
	return out, nil
}

func hasBytesOrProto(t *sppb.Type) bool {
	if t == nil {
		return false
	}
	switch t.Code {
	case sppb.TypeCode_BYTES, sppb.TypeCode_PROTO:
		return true
	case sppb.TypeCode_ARRAY:
		return hasBytesOrProto(t.ArrayElementType)
	case sppb.TypeCode_STRUCT:
		if t.StructType != nil {
			for _, f := range t.StructType.Fields {
				if f != nil && hasBytesOrProto(f.Type) {
					return true
				}
			}
		}
	}
	return false
}

func estimateDecodedBytes(v *proto3.Value, t *sppb.Type) int {
	if v == nil || t == nil {
		return 0
	}
	switch t.Code {
	case sppb.TypeCode_BYTES, sppb.TypeCode_PROTO:
		if sv, ok := v.Kind.(*proto3.Value_StringValue); ok && sv != nil && len(sv.StringValue) > 0 {
			return base64.StdEncoding.DecodedLen(len(sv.StringValue))
		}
	case sppb.TypeCode_ARRAY:
		if lv, ok := v.Kind.(*proto3.Value_ListValue); ok && lv != nil && lv.ListValue != nil {
			total := 0
			for _, elem := range lv.ListValue.Values {
				total += estimateDecodedBytes(elem, t.ArrayElementType)
			}
			return total
		}
	case sppb.TypeCode_STRUCT:
		if lv, ok := v.Kind.(*proto3.Value_ListValue); ok && lv != nil && lv.ListValue != nil && t.StructType != nil {
			total := 0
			structFields := t.StructType.Fields
			for i, elem := range lv.ListValue.Values {
				if i < len(structFields) && structFields[i] != nil {
					total += estimateDecodedBytes(elem, structFields[i].Type)
				}
			}
			return total
		}
	}
	return 0
}

// PackRowsIntoArena decodes raw protobuf Value rows directly into a contiguous
// pointer-free []PackedCell slice and a contiguous []byte arena in a single pass.
// Because PackedCell contains no Go pointers, Go's garbage collector allocates it
// in a noscan span (zero GC mark overhead), and the cgo layer can copy the entire
// batch across the FFI boundary with two memcpy calls.
func PackRowsIntoArena(rawRows [][]*proto3.Value, fields []*sppb.StructType_Field) ([]PackedCell, []byte, int, int, error) {
	cells, arenaBytes, numRows, numCols, _, err := packRowsIntoArenaInternal(rawRows, fields, false)
	return cells, arenaBytes, numRows, numCols, err
}

func packRowsIntoArenaInternal(
	rawRows [][]*proto3.Value,
	fields []*sppb.StructType_Field,
	usePool bool,
) ([]PackedCell, []byte, int, int, *arenaScratch, error) {
	numRows := len(rawRows)
	numCols := len(fields)
	if numRows == 0 || numCols == 0 {
		return nil, nil, numRows, numCols, nil, nil
	}

	var colTypesStack [32]*sppb.Type
	var colTypes []*sppb.Type
	if numCols <= len(colTypesStack) {
		colTypes = colTypesStack[:numCols]
	} else {
		colTypes = make([]*sppb.Type, numCols)
	}
	for cIdx := 0; cIdx < numCols; cIdx++ {
		if fields[cIdx] != nil {
			colTypes[cIdx] = fields[cIdx].Type
		}
	}

	// Estimate byte arena capacity from the first row in O(numCols) time.
	row0Bytes := 0
	for cIdx := 0; cIdx < len(rawRows[0]) && cIdx < numCols; cIdx++ {
		if v := rawRows[0][cIdx]; v != nil {
			if sv, ok := v.Kind.(*proto3.Value_StringValue); ok && sv != nil {
				row0Bytes += len(sv.StringValue)
			}
		}
	}
	estBytes := row0Bytes*numRows*5/4 + 256
	if estBytes < 256 {
		estBytes = 256
	} else if estBytes > maxPooledArenaBytes {
		estBytes = maxPooledArenaBytes
	}

	topCells := numRows * numCols
	var (
		cells      []PackedCell
		arenaBytes []byte
		scratch    *arenaScratch
	)
	if usePool {
		scratch = acquireArenaScratch(topCells, estBytes)
		cells = scratch.cells
		arenaBytes = scratch.bytes
	} else {
		cells = make([]PackedCell, topCells)
		arenaBytes = make([]byte, 0, estBytes)
	}

	for rIdx, rawRow := range rawRows {
		baseIdx := rIdx * numCols
		limit := len(rawRow)
		if limit > numCols {
			limit = numCols
		}
		for cIdx := 0; cIdx < limit; cIdx++ {
			if err := packCellIntoArena(rawRow[cIdx], colTypes[cIdx], baseIdx+cIdx, &cells, &arenaBytes); err != nil {
				if scratch != nil {
					scratch.cells = cells
					scratch.bytes = arenaBytes
					releaseArenaScratch(scratch)
				}
				return nil, nil, 0, 0, nil, fmt.Errorf("sharedcore: row %d, col %d (%q): %w", rIdx, cIdx, fields[cIdx].GetName(), err)
			}
		}
		if limit < numCols {
			clear(cells[baseIdx+limit : baseIdx+numCols])
		}
	}
	if scratch != nil {
		scratch.cells = cells
		scratch.bytes = arenaBytes
	}
	return cells, arenaBytes, numRows, numCols, scratch, nil
}

func isNullProtoValue(v *proto3.Value) bool {
	if v == nil || v.Kind == nil {
		return true
	}
	_, isNull := v.Kind.(*proto3.Value_NullValue)
	return isNull
}

func errUnexpectedValueKind(v *proto3.Value, code sppb.TypeCode) error {
	return fmt.Errorf("unexpected protobuf value kind %T for Spanner type %v", v.Kind, code)
}

func packCellIntoArena(v *proto3.Value, t *sppb.Type, dstIdx int, cells *[]PackedCell, arenaBytes *[]byte) error {
	(*cells)[dstIdx] = PackedCell{}
	if isNullProtoValue(v) {
		return nil
	}

	var code sppb.TypeCode
	if t != nil {
		code = t.Code
	}

	switch code {
	case sppb.TypeCode_BOOL:
		return packBool(v, dstIdx, cells)
	case sppb.TypeCode_FLOAT64:
		return packFloat(v, KindFloat64, code, dstIdx, cells)
	case sppb.TypeCode_FLOAT32:
		return packFloat(v, KindFloat32, code, dstIdx, cells)
	case sppb.TypeCode_BYTES, sppb.TypeCode_PROTO:
		return packBytes(v, code, dstIdx, cells, arenaBytes)
	case sppb.TypeCode_TIMESTAMP:
		return packTimestamp(v, dstIdx, cells)
	case sppb.TypeCode_ARRAY:
		var elemType *sppb.Type
		if t != nil {
			elemType = t.ArrayElementType
		}
		return packListValue(v, KindArray, code, elemType, nil, dstIdx, cells, arenaBytes)
	case sppb.TypeCode_STRUCT:
		var structFields []*sppb.StructType_Field
		if t != nil && t.StructType != nil {
			structFields = t.StructType.Fields
		}
		return packListValue(v, KindStruct, code, nil, structFields, dstIdx, cells, arenaBytes)
	case sppb.TypeCode_STRING,
		sppb.TypeCode_INT64,
		sppb.TypeCode_NUMERIC,
		sppb.TypeCode_DATE,
		sppb.TypeCode_JSON,
		sppb.TypeCode_INTERVAL,
		sppb.TypeCode_UUID,
		sppb.TypeCode_ENUM:
		return packStringValue(v, code, dstIdx, cells, arenaBytes)
	default:
		return packDefaultValue(v, code, dstIdx, cells, arenaBytes)
	}
}

func packStringValue(v *proto3.Value, code sppb.TypeCode, dstIdx int, cells *[]PackedCell, arenaBytes *[]byte) error {
	sv, ok := v.Kind.(*proto3.Value_StringValue)
	if !ok || sv == nil {
		return errUnexpectedValueKind(v, code)
	}
	packString(sv.StringValue, dstIdx, cells, arenaBytes)
	return nil
}

func packBool(v *proto3.Value, dstIdx int, cells *[]PackedCell) error {
	bv, ok := v.Kind.(*proto3.Value_BoolValue)
	if !ok || bv == nil {
		return errUnexpectedValueKind(v, sppb.TypeCode_BOOL)
	}
	packBoolFlag(bv.BoolValue, dstIdx, cells)
	return nil
}

func packBoolFlag(b bool, dstIdx int, cells *[]PackedCell) {
	var flag uint32
	if b {
		flag = 1
	}
	(*cells)[dstIdx] = PackedCell{Kind: int32(KindBool), Flags: flag}
}

func packString(s string, dstIdx int, cells *[]PackedCell, arenaBytes *[]byte) {
	off := len(*arenaBytes)
	*arenaBytes = append(*arenaBytes, s...)
	(*cells)[dstIdx] = PackedCell{Kind: int32(KindString), Offset: uint32(off), Len: uint32(len(s))}
}

// packFloat packs both FLOAT64 and FLOAT32 values into PackedCell.
// Note: Both KindFloat64 and KindFloat32 store 64-bit IEEE 754 float64 bits in NumBits
// because the C ABI union CSpannerCell.num.f64_val is a 64-bit C double read directly
// by N-API / Python C extensions for both cell kinds.
func packFloat(v *proto3.Value, kind ValueKind, code sppb.TypeCode, dstIdx int, cells *[]PackedCell) error {
	switch kv := v.Kind.(type) {
	case *proto3.Value_NumberValue:
		if kv == nil {
			return errUnexpectedValueKind(v, code)
		}
		(*cells)[dstIdx] = PackedCell{Kind: int32(kind), NumBits: math.Float64bits(kv.NumberValue)}
		return nil
	case *proto3.Value_StringValue:
		if kv == nil {
			return errUnexpectedValueKind(v, code)
		}
		f, ok := parseFloatSpecial(kv.StringValue)
		if !ok {
			return fmt.Errorf("unexpected string value %q for Spanner type %v", kv.StringValue, code)
		}
		(*cells)[dstIdx] = PackedCell{Kind: int32(kind), NumBits: math.Float64bits(f)}
		return nil
	default:
		return errUnexpectedValueKind(v, code)
	}
}

func packBytes(v *proto3.Value, code sppb.TypeCode, dstIdx int, cells *[]PackedCell, arenaBytes *[]byte) error {
	sv, ok := v.Kind.(*proto3.Value_StringValue)
	if !ok || sv == nil {
		return errUnexpectedValueKind(v, code)
	}
	s := sv.StringValue
	if len(s) == 0 {
		(*cells)[dstIdx] = PackedCell{Kind: int32(KindBytes), Offset: uint32(len(*arenaBytes)), Len: 0}
		return nil
	}
	maxLen := base64.StdEncoding.DecodedLen(len(s))
	off := len(*arenaBytes)
	if cap(*arenaBytes)-off < maxLen {
		newCap := cap(*arenaBytes)*2 + maxLen
		grown := make([]byte, off, newCap)
		copy(grown, *arenaBytes)
		*arenaBytes = grown
	}
	dst := (*arenaBytes)[off : off+maxLen]
	srcBytes := unsafe.Slice(unsafe.StringData(s), len(s))
	n, err := base64.StdEncoding.Decode(dst, srcBytes)
	if err != nil {
		return fmt.Errorf("invalid base64 value for Spanner type %v: %w", code, err)
	}
	*arenaBytes = (*arenaBytes)[:off+n]
	(*cells)[dstIdx] = PackedCell{Kind: int32(KindBytes), Offset: uint32(off), Len: uint32(n)}
	return nil
}

func packTimestamp(v *proto3.Value, dstIdx int, cells *[]PackedCell) error {
	sv, ok := v.Kind.(*proto3.Value_StringValue)
	if !ok || sv == nil {
		return errUnexpectedValueKind(v, sppb.TypeCode_TIMESTAMP)
	}
	sec, nano, err := parseRFC3339NanoFast(sv.StringValue)
	if err != nil {
		return err
	}
	(*cells)[dstIdx] = PackedCell{
		Kind:    int32(KindTimestamp),
		Flags:   uint32(nano),
		NumBits: uint64(sec),
	}
	return nil
}

func reserveChildCells(cells *[]PackedCell, n int) int {
	childStart := len(*cells)
	if n > 0 {
		if cap(*cells)-childStart < n {
			newCap := cap(*cells)*2 + n
			grown := make([]PackedCell, childStart+n, newCap)
			copy(grown, *cells)
			*cells = grown
		} else {
			*cells = (*cells)[:childStart+n]
		}
	}
	return childStart
}

func listChildType(kind ValueKind, i int, elemType *sppb.Type, structFields []*sppb.StructType_Field) *sppb.Type {
	if kind == KindStruct {
		if i < len(structFields) && structFields[i] != nil {
			return structFields[i].Type
		}
		return nil
	}
	return elemType
}

func packListValue(
	v *proto3.Value,
	kind ValueKind,
	code sppb.TypeCode,
	elemType *sppb.Type,
	structFields []*sppb.StructType_Field,
	dstIdx int,
	cells *[]PackedCell,
	arenaBytes *[]byte,
) error {
	lv, ok := v.Kind.(*proto3.Value_ListValue)
	if !ok || lv == nil || lv.ListValue == nil {
		return errUnexpectedValueKind(v, code)
	}
	vals := lv.ListValue.Values
	n := len(vals)
	childStart := reserveChildCells(cells, n)
	(*cells)[dstIdx] = PackedCell{
		Kind:   int32(kind),
		Offset: uint32(childStart),
		Len:    uint32(n),
	}
	for i, elem := range vals {
		childType := listChildType(kind, i, elemType, structFields)
		if err := packCellIntoArena(elem, childType, childStart+i, cells, arenaBytes); err != nil {
			return fmt.Errorf("element %d: %w", i, err)
		}
	}
	return nil
}

// packDefaultValue handles TYPE_CODE_UNSPECIFIED and any future unrecognized Spanner TypeCodes.
func packDefaultValue(v *proto3.Value, code sppb.TypeCode, dstIdx int, cells *[]PackedCell, arenaBytes *[]byte) error {
	switch kv := v.Kind.(type) {
	case *proto3.Value_StringValue:
		if kv == nil {
			return errUnexpectedValueKind(v, code)
		}
		packString(kv.StringValue, dstIdx, cells, arenaBytes)
		return nil
	case *proto3.Value_BoolValue:
		if kv == nil {
			return errUnexpectedValueKind(v, code)
		}
		packBoolFlag(kv.BoolValue, dstIdx, cells)
		return nil
	case *proto3.Value_NumberValue:
		if kv == nil {
			return errUnexpectedValueKind(v, code)
		}
		(*cells)[dstIdx] = PackedCell{Kind: int32(KindFloat64), NumBits: math.Float64bits(kv.NumberValue)}
		return nil
	case *proto3.Value_ListValue:
		return packListValue(v, KindArray, code, nil, nil, dstIdx, cells, arenaBytes)
	default:
		return errUnexpectedValueKind(v, code)
	}
}

// DecodeCellValue decodes a single protobuf Value into a typed SpannerValue based on its Spanner Type.
func DecodeCellValue(v *proto3.Value, t *sppb.Type) (SpannerValue, error) {
	val, err := decodeCellValueWithPool(v, t, nil)
	if err != nil {
		return SpannerValue{}, fmt.Errorf("sharedcore: %w", err)
	}
	return val, nil
}

func decodeCellValueWithPool(v *proto3.Value, t *sppb.Type, bytePool *[]byte) (SpannerValue, error) {
	if isNullProtoValue(v) {
		return SpannerValue{Kind: KindNull}, nil
	}

	var code sppb.TypeCode
	if t != nil {
		code = t.Code
	}

	switch code {
	case sppb.TypeCode_BOOL:
		return decodeBool(v)
	case sppb.TypeCode_FLOAT64:
		return decodeFloat(v, KindFloat64, code)
	case sppb.TypeCode_FLOAT32:
		return decodeFloat(v, KindFloat32, code)
	case sppb.TypeCode_BYTES, sppb.TypeCode_PROTO:
		return decodeBytes(v, code, bytePool)
	case sppb.TypeCode_TIMESTAMP:
		return decodeTimestamp(v)
	case sppb.TypeCode_ARRAY:
		var elemType *sppb.Type
		if t != nil {
			elemType = t.ArrayElementType
		}
		return decodeListValue(v, KindArray, code, elemType, nil, bytePool)
	case sppb.TypeCode_STRUCT:
		var structFields []*sppb.StructType_Field
		if t != nil && t.StructType != nil {
			structFields = t.StructType.Fields
		}
		return decodeListValue(v, KindStruct, code, nil, structFields, bytePool)
	case sppb.TypeCode_STRING,
		sppb.TypeCode_INT64,
		sppb.TypeCode_NUMERIC,
		sppb.TypeCode_DATE,
		sppb.TypeCode_JSON,
		sppb.TypeCode_INTERVAL,
		sppb.TypeCode_UUID,
		sppb.TypeCode_ENUM:
		return decodeStringValue(v, code)
	default:
		return decodeDefaultValue(v, code, bytePool)
	}
}

func decodeStringValue(v *proto3.Value, code sppb.TypeCode) (SpannerValue, error) {
	sv, ok := v.Kind.(*proto3.Value_StringValue)
	if !ok || sv == nil {
		return SpannerValue{}, errUnexpectedValueKind(v, code)
	}
	return SpannerValue{Kind: KindString, StrVal: sv.StringValue}, nil
}

func decodeBool(v *proto3.Value) (SpannerValue, error) {
	bv, ok := v.Kind.(*proto3.Value_BoolValue)
	if !ok || bv == nil {
		return SpannerValue{}, errUnexpectedValueKind(v, sppb.TypeCode_BOOL)
	}
	return SpannerValue{Kind: KindBool, BoolVal: bv.BoolValue}, nil
}

func decodeFloat(v *proto3.Value, kind ValueKind, code sppb.TypeCode) (SpannerValue, error) {
	switch kv := v.Kind.(type) {
	case *proto3.Value_NumberValue:
		if kv == nil {
			return SpannerValue{}, errUnexpectedValueKind(v, code)
		}
		return SpannerValue{Kind: kind, Float64Val: kv.NumberValue}, nil
	case *proto3.Value_StringValue:
		if kv == nil {
			return SpannerValue{}, errUnexpectedValueKind(v, code)
		}
		f, ok := parseFloatSpecial(kv.StringValue)
		if !ok {
			return SpannerValue{}, fmt.Errorf("unexpected string value %q for Spanner type %v", kv.StringValue, code)
		}
		return SpannerValue{Kind: kind, Float64Val: f}, nil
	default:
		return SpannerValue{}, errUnexpectedValueKind(v, code)
	}
}

func decodeBytes(v *proto3.Value, code sppb.TypeCode, bytePool *[]byte) (SpannerValue, error) {
	sv, ok := v.Kind.(*proto3.Value_StringValue)
	if !ok || sv == nil {
		return SpannerValue{}, errUnexpectedValueKind(v, code)
	}
	s := sv.StringValue
	if len(s) == 0 {
		return SpannerValue{Kind: KindBytes, BytesVal: []byte{}}, nil
	}
	maxLen := base64.StdEncoding.DecodedLen(len(s))
	srcBytes := unsafe.Slice(unsafe.StringData(s), len(s))
	var (
		dst      []byte
		fromPool bool
		poolOff  int
	)
	if bytePool != nil && cap(*bytePool)-len(*bytePool) >= maxLen {
		poolOff = len(*bytePool)
		dst = (*bytePool)[poolOff : poolOff+maxLen]
		fromPool = true
	} else {
		dst = make([]byte, maxLen)
	}
	n, err := base64.StdEncoding.Decode(dst, srcBytes)
	if err != nil {
		return SpannerValue{}, fmt.Errorf("invalid base64 value for Spanner type %v: %w", code, err)
	}
	if fromPool {
		*bytePool = (*bytePool)[:poolOff+n]
	}
	return SpannerValue{Kind: KindBytes, BytesVal: dst[:n:n]}, nil
}

func decodeTimestamp(v *proto3.Value) (SpannerValue, error) {
	sv, ok := v.Kind.(*proto3.Value_StringValue)
	if !ok || sv == nil {
		return SpannerValue{}, errUnexpectedValueKind(v, sppb.TypeCode_TIMESTAMP)
	}
	sec, nano, err := parseRFC3339NanoFast(sv.StringValue)
	if err != nil {
		return SpannerValue{}, err
	}
	return SpannerValue{Kind: KindTimestamp, Int64Val: sec, Int32Val: nano}, nil
}

func decodeListValue(
	v *proto3.Value,
	kind ValueKind,
	code sppb.TypeCode,
	elemType *sppb.Type,
	structFields []*sppb.StructType_Field,
	bytePool *[]byte,
) (SpannerValue, error) {
	lv, ok := v.Kind.(*proto3.Value_ListValue)
	if !ok || lv == nil || lv.ListValue == nil {
		return SpannerValue{}, errUnexpectedValueKind(v, code)
	}
	vals := lv.ListValue.Values
	children := make([]SpannerValue, len(vals))
	for i, elem := range vals {
		childType := listChildType(kind, i, elemType, structFields)
		childVal, err := decodeCellValueWithPool(elem, childType, bytePool)
		if err != nil {
			return SpannerValue{}, fmt.Errorf("element %d: %w", i, err)
		}
		children[i] = childVal
	}
	return SpannerValue{Kind: kind, Children: children}, nil
}

func decodeDefaultValue(v *proto3.Value, code sppb.TypeCode, bytePool *[]byte) (SpannerValue, error) {
	switch kv := v.Kind.(type) {
	case *proto3.Value_StringValue:
		if kv == nil {
			return SpannerValue{}, errUnexpectedValueKind(v, code)
		}
		return SpannerValue{Kind: KindString, StrVal: kv.StringValue}, nil
	case *proto3.Value_BoolValue:
		if kv == nil {
			return SpannerValue{}, errUnexpectedValueKind(v, code)
		}
		return SpannerValue{Kind: KindBool, BoolVal: kv.BoolValue}, nil
	case *proto3.Value_NumberValue:
		if kv == nil {
			return SpannerValue{}, errUnexpectedValueKind(v, code)
		}
		return SpannerValue{Kind: KindFloat64, Float64Val: kv.NumberValue}, nil
	case *proto3.Value_ListValue:
		return decodeListValue(v, KindArray, code, nil, nil, bytePool)
	default:
		return SpannerValue{}, errUnexpectedValueKind(v, code)
	}
}

func parseFloatSpecial(s string) (float64, bool) {
	switch s {
	case "Infinity":
		return math.Inf(1), true
	case "-Infinity":
		return math.Inf(-1), true
	case "NaN":
		return math.NaN(), true
	default:
		return 0, false
	}
}

// parseRFC3339NanoFast parses standard Spanner UTC timestamps of the form
// "YYYY-MM-DDTHH:MM:SSZ" or "YYYY-MM-DDTHH:MM:SS.fffffffffZ" without layout reflection,
// falling back to time.Parse(time.RFC3339Nano, s) for non-standard RFC3339Nano offsets.
func parseRFC3339NanoFast(s string) (int64, int32, error) {
	if sec, nanos, ok := parseStandardUTCTimestamp(s); ok {
		return sec, nanos, nil
	}
	parsed, err := time.Parse(time.RFC3339Nano, s)
	if err != nil {
		return 0, 0, fmt.Errorf("invalid RFC3339Nano timestamp %q: %w", s, err)
	}
	return parsed.Unix(), int32(parsed.Nanosecond()), nil
}

func parseStandardUTCTimestamp(s string) (int64, int32, bool) {
	n := len(s)
	if n < 20 || n > 30 || s[4] != '-' || s[7] != '-' || s[10] != 'T' || s[13] != ':' || s[16] != ':' || s[n-1] != 'Z' {
		return 0, 0, false
	}
	year, ok1 := parseDigits4(s, 0)
	month, ok2 := parseDigits2(s, 5)
	day, ok3 := parseDigits2(s, 8)
	hour, ok4 := parseDigits2(s, 11)
	minute, ok5 := parseDigits2(s, 14)
	sec, ok6 := parseDigits2(s, 17)
	if !ok1 || !ok2 || !ok3 || !ok4 || !ok5 || !ok6 {
		return 0, 0, false
	}
	nanos, okNanos := parseFractionalNanos(s)
	if !okNanos {
		return 0, 0, false
	}
	if month < 1 || month > 12 || day < 1 || day > maxDaysInMonth(year, month) || hour > 23 || minute > 59 || sec > 59 {
		return 0, 0, false
	}
	return civilToUnixSeconds(year, month, day, hour, minute, sec), nanos, true
}

func parseFractionalNanos(s string) (int32, bool) {
	n := len(s)
	if n == 20 {
		return 0, true
	}
	if s[19] != '.' {
		return 0, false
	}
	fracLen := n - 1 - 20
	if fracLen < 1 || fracLen > 9 {
		return 0, false
	}
	var frac int32
	for i := 20; i < n-1; i++ {
		ch := s[i]
		if ch < '0' || ch > '9' {
			return 0, false
		}
		frac = frac*10 + int32(ch-'0')
	}
	for i := fracLen; i < 9; i++ {
		frac *= 10
	}
	return frac, true
}

func civilToUnixSeconds(year, month, day, hour, minute, sec int) int64 {
	y := int64(year)
	m := int64(month)
	d := int64(day)
	if m <= 2 {
		y--
	}
	var era int64
	if y >= 0 {
		era = y / 400
	} else {
		era = (y - 399) / 400
	}
	yoe := y - era*400
	var mp int64
	if m > 2 {
		mp = m - 3
	} else {
		mp = m + 9
	}
	doy := (153*mp+2)/5 + d - 1
	doe := yoe*365 + yoe/4 - yoe/100 + doy
	days := era*146097 + doe - 719468
	return days*86400 + int64(hour)*3600 + int64(minute)*60 + int64(sec)
}

func maxDaysInMonth(year, month int) int {
	switch month {
	case 1, 3, 5, 7, 8, 10, 12:
		return 31
	case 4, 6, 9, 11:
		return 30
	case 2:
		if (year%4 == 0 && year%100 != 0) || year%400 == 0 {
			return 29
		}
		return 28
	default:
		return 0
	}
}

func parseDigits2(s string, offset int) (int, bool) {
	c0, c1 := s[offset], s[offset+1]
	if c0 < '0' || c0 > '9' || c1 < '0' || c1 > '9' {
		return 0, false
	}
	return int(c0-'0')*10 + int(c1-'0'), true
}

func parseDigits4(s string, offset int) (int, bool) {
	c0, c1, c2, c3 := s[offset], s[offset+1], s[offset+2], s[offset+3]
	if c0 < '0' || c0 > '9' || c1 < '0' || c1 > '9' || c2 < '0' || c2 > '9' || c3 < '0' || c3 > '9' {
		return 0, false
	}
	return int(c0-'0')*1000 + int(c1-'0')*100 + int(c2-'0')*10 + int(c3-'0'), true
}

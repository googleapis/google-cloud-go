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
	"math"
	"strconv"
	"sync"
	"time"
	"unsafe"

	sppb "cloud.google.com/go/spanner/apiv1/spannerpb"
	proto3 "google.golang.org/protobuf/types/known/structpb"
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
		clear(s.cells)
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
	if cap(s.cells) <= 32768 && cap(s.bytes) <= 1024*1024 {
		arenaScratchPool.Put(s)
	}
}

// Internal Use Only: DecodeRows transforms a slice of raw protobuf Value rows (produced by Go Spanner's
// partialResultSetDecoder) into typed SpannerValue rows ready for C-arena serialization.
func DecodeRows(rawRows [][]*proto3.Value, fields []*sppb.StructType_Field) [][]SpannerValue {
	if len(rawRows) == 0 {
		return nil
	}
	numCols := len(fields)
	out := make([][]SpannerValue, len(rawRows))
	// Allocate a single flat backing array for all top-level cells in the batch
	// to minimize Go heap allocations per row.
	flatCells := make([]SpannerValue, len(rawRows)*numCols)

	// Pre-extract column Type pointers and estimate base64 byte pool size if any column is BYTES/PROTO.
	colTypes := make([]*sppb.Type, numCols)
	hasBytesCol := false
	for cIdx := 0; cIdx < numCols; cIdx++ {
		if fields[cIdx] != nil {
			colTypes[cIdx] = fields[cIdx].Type
			if colTypes[cIdx] != nil && (colTypes[cIdx].Code == sppb.TypeCode_BYTES || colTypes[cIdx].Code == sppb.TypeCode_PROTO) {
				hasBytesCol = true
			}
		}
	}

	var bytePool []byte
	if hasBytesCol {
		totalB64 := 0
		for _, rawRow := range rawRows {
			limit := len(rawRow)
			if limit > numCols {
				limit = numCols
			}
			for cIdx := 0; cIdx < limit; cIdx++ {
				if ct := colTypes[cIdx]; ct != nil && (ct.Code == sppb.TypeCode_BYTES || ct.Code == sppb.TypeCode_PROTO) {
					if v := rawRow[cIdx]; v != nil {
						if sv, ok := v.Kind.(*proto3.Value_StringValue); ok && len(sv.StringValue) > 0 {
							totalB64 += base64.StdEncoding.DecodedLen(len(sv.StringValue))
						}
					}
				}
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
			rowSlice[cIdx] = decodeCellValueWithPool(rawRow[cIdx], colTypes[cIdx], &bytePool)
		}
		out[rIdx] = rowSlice
	}
	return out
}

// Internal Use Only: PackRowsIntoArena decodes raw protobuf Value rows directly into a contiguous
// pointer-free []PackedCell slice and a contiguous []byte arena in a single pass.
// Because PackedCell contains no Go pointers, Go's garbage collector allocates it
// in a noscan span (zero GC mark overhead), and the cgo layer can copy the entire
// batch across the FFI boundary with two memcpy calls.
func PackRowsIntoArena(rawRows [][]*proto3.Value, fields []*sppb.StructType_Field) ([]PackedCell, []byte, int, int) {
	cells, arenaBytes, numRows, numCols, _ := packRowsIntoArenaInternal(rawRows, fields, false)
	return cells, arenaBytes, numRows, numCols
}

func packRowsIntoArenaInternal(
	rawRows [][]*proto3.Value,
	fields []*sppb.StructType_Field,
	usePool bool,
) ([]PackedCell, []byte, int, int, *arenaScratch) {
	numRows := len(rawRows)
	numCols := len(fields)
	if numRows == 0 || numCols == 0 {
		return nil, nil, numRows, numCols, nil
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
			if sv, ok := v.Kind.(*proto3.Value_StringValue); ok {
				row0Bytes += len(sv.StringValue)
			}
		}
	}
	estBytes := row0Bytes*numRows*5/4 + 256
	if estBytes < 256 {
		estBytes = 256
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
			packCellIntoArena(rawRow[cIdx], colTypes[cIdx], baseIdx+cIdx, &cells, &arenaBytes)
		}
		if limit < numCols {
			clear(cells[baseIdx+limit : baseIdx+numCols])
		}
	}
	if scratch != nil {
		scratch.cells = cells
		scratch.bytes = arenaBytes
	}
	return cells, arenaBytes, numRows, numCols, scratch
}

func packCellIntoArena(v *proto3.Value, t *sppb.Type, dstIdx int, cells *[]PackedCell, arenaBytes *[]byte) {
	(*cells)[dstIdx] = PackedCell{}
	if v == nil || v.Kind == nil {
		return
	}
	if _, isNull := v.Kind.(*proto3.Value_NullValue); isNull {
		return
	}

	var code sppb.TypeCode
	if t != nil {
		code = t.Code
	}

	switch code {
	case sppb.TypeCode_BOOL:
		if bv, ok := v.Kind.(*proto3.Value_BoolValue); ok {
			var flag uint32
			if bv.BoolValue {
				flag = 1
			}
			(*cells)[dstIdx] = PackedCell{Kind: int32(KindBool), Flags: flag}
		}
		return

	case sppb.TypeCode_FLOAT64:
		switch kv := v.Kind.(type) {
		case *proto3.Value_NumberValue:
			(*cells)[dstIdx] = PackedCell{Kind: int32(KindFloat64), NumBits: math.Float64bits(kv.NumberValue)}
		case *proto3.Value_StringValue:
			if f, ok := parseFloatSpecialOrStd(kv.StringValue); ok {
				(*cells)[dstIdx] = PackedCell{Kind: int32(KindFloat64), NumBits: math.Float64bits(f)}
			} else {
				off := len(*arenaBytes)
				*arenaBytes = append(*arenaBytes, kv.StringValue...)
				(*cells)[dstIdx] = PackedCell{Kind: int32(KindString), Offset: uint32(off), Len: uint32(len(kv.StringValue))}
			}
		}
		return

	case sppb.TypeCode_FLOAT32:
		switch kv := v.Kind.(type) {
		case *proto3.Value_NumberValue:
			(*cells)[dstIdx] = PackedCell{Kind: int32(KindFloat32), NumBits: math.Float64bits(kv.NumberValue)}
		case *proto3.Value_StringValue:
			if f, ok := parseFloatSpecialOrStd(kv.StringValue); ok {
				(*cells)[dstIdx] = PackedCell{Kind: int32(KindFloat32), NumBits: math.Float64bits(f)}
			} else {
				off := len(*arenaBytes)
				*arenaBytes = append(*arenaBytes, kv.StringValue...)
				(*cells)[dstIdx] = PackedCell{Kind: int32(KindString), Offset: uint32(off), Len: uint32(len(kv.StringValue))}
			}
		}
		return

	case sppb.TypeCode_BYTES, sppb.TypeCode_PROTO:
		if sv, ok := v.Kind.(*proto3.Value_StringValue); ok {
			s := sv.StringValue
			if len(s) == 0 {
				(*cells)[dstIdx] = PackedCell{Kind: int32(KindBytes), Offset: uint32(len(*arenaBytes)), Len: 0}
				return
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
			if err == nil {
				*arenaBytes = (*arenaBytes)[:off+n]
				(*cells)[dstIdx] = PackedCell{Kind: int32(KindBytes), Offset: uint32(off), Len: uint32(n)}
				return
			}
			// Fallback to raw string if non-standard base64 so host codec can handle it.
			*arenaBytes = append(*arenaBytes, s...)
			(*cells)[dstIdx] = PackedCell{Kind: int32(KindString), Offset: uint32(off), Len: uint32(len(s))}
		}
		return

	case sppb.TypeCode_TIMESTAMP:
		if sv, ok := v.Kind.(*proto3.Value_StringValue); ok {
			if sec, nano, ok := parseRFC3339NanoFast(sv.StringValue); ok {
				(*cells)[dstIdx] = PackedCell{
					Kind:    int32(KindTimestamp),
					Flags:   uint32(nano),
					NumBits: uint64(sec),
				}
				return
			}
			off := len(*arenaBytes)
			*arenaBytes = append(*arenaBytes, sv.StringValue...)
			(*cells)[dstIdx] = PackedCell{Kind: int32(KindString), Offset: uint32(off), Len: uint32(len(sv.StringValue))}
		}
		return

	case sppb.TypeCode_ARRAY:
		if lv, ok := v.Kind.(*proto3.Value_ListValue); ok && lv.ListValue != nil {
			var elemType *sppb.Type
			if t != nil {
				elemType = t.ArrayElementType
			}
			vals := lv.ListValue.Values
			n := len(vals)
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
			(*cells)[dstIdx] = PackedCell{
				Kind:   int32(KindArray),
				Offset: uint32(childStart),
				Len:    uint32(n),
			}
			for i, elem := range vals {
				packCellIntoArena(elem, elemType, childStart+i, cells, arenaBytes)
			}
		}
		return

	case sppb.TypeCode_STRUCT:
		if lv, ok := v.Kind.(*proto3.Value_ListValue); ok && lv.ListValue != nil {
			var structFields []*sppb.StructType_Field
			if t != nil && t.StructType != nil {
				structFields = t.StructType.Fields
			}
			vals := lv.ListValue.Values
			n := len(vals)
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
			(*cells)[dstIdx] = PackedCell{
				Kind:   int32(KindStruct),
				Offset: uint32(childStart),
				Len:    uint32(n),
			}
			for i, elem := range vals {
				var fieldType *sppb.Type
				if i < len(structFields) && structFields[i] != nil {
					fieldType = structFields[i].Type
				}
				packCellIntoArena(elem, fieldType, childStart+i, cells, arenaBytes)
			}
		}
		return
	}

	// Default scalar fallback (STRING, INT64, NUMERIC, DATE, JSON, INTERVAL, UUID, ENUM, etc.)
	switch kv := v.Kind.(type) {
	case *proto3.Value_StringValue:
		off := len(*arenaBytes)
		*arenaBytes = append(*arenaBytes, kv.StringValue...)
		(*cells)[dstIdx] = PackedCell{Kind: int32(KindString), Offset: uint32(off), Len: uint32(len(kv.StringValue))}
	case *proto3.Value_BoolValue:
		var flag uint32
		if kv.BoolValue {
			flag = 1
		}
		(*cells)[dstIdx] = PackedCell{Kind: int32(KindBool), Flags: flag}
	case *proto3.Value_NumberValue:
		(*cells)[dstIdx] = PackedCell{Kind: int32(KindFloat64), NumBits: math.Float64bits(kv.NumberValue)}
	}
}

// Internal Use Only: DecodeCellValue decodes a single protobuf Value into a typed SpannerValue based on its Spanner Type.
func DecodeCellValue(v *proto3.Value, t *sppb.Type) SpannerValue {
	return decodeCellValueWithPool(v, t, nil)
}

func decodeCellValueWithPool(v *proto3.Value, t *sppb.Type, bytePool *[]byte) SpannerValue {
	if v == nil || v.Kind == nil {
		return SpannerValue{Kind: KindNull}
	}
	if _, isNull := v.Kind.(*proto3.Value_NullValue); isNull {
		return SpannerValue{Kind: KindNull}
	}

	var code sppb.TypeCode
	if t != nil {
		code = t.Code
	}

	switch code {
	case sppb.TypeCode_BOOL:
		if bv, ok := v.Kind.(*proto3.Value_BoolValue); ok {
			return SpannerValue{Kind: KindBool, BoolVal: bv.BoolValue}
		}
		return SpannerValue{Kind: KindNull}

	case sppb.TypeCode_FLOAT64:
		switch kv := v.Kind.(type) {
		case *proto3.Value_NumberValue:
			return SpannerValue{Kind: KindFloat64, Float64Val: kv.NumberValue}
		case *proto3.Value_StringValue:
			if f, ok := parseFloatSpecialOrStd(kv.StringValue); ok {
				return SpannerValue{Kind: KindFloat64, Float64Val: f}
			}
			return SpannerValue{Kind: KindString, StrVal: kv.StringValue}
		}

	case sppb.TypeCode_FLOAT32:
		switch kv := v.Kind.(type) {
		case *proto3.Value_NumberValue:
			return SpannerValue{Kind: KindFloat32, Float64Val: kv.NumberValue}
		case *proto3.Value_StringValue:
			if f, ok := parseFloatSpecialOrStd(kv.StringValue); ok {
				return SpannerValue{Kind: KindFloat32, Float64Val: f}
			}
			return SpannerValue{Kind: KindString, StrVal: kv.StringValue}
		}

	case sppb.TypeCode_BYTES, sppb.TypeCode_PROTO:
		if sv, ok := v.Kind.(*proto3.Value_StringValue); ok {
			s := sv.StringValue
			if len(s) == 0 {
				return SpannerValue{Kind: KindBytes, BytesVal: []byte{}}
			}
			maxLen := base64.StdEncoding.DecodedLen(len(s))
			srcBytes := unsafe.Slice(unsafe.StringData(s), len(s))
			if bytePool != nil && cap(*bytePool)-len(*bytePool) >= maxLen {
				start := len(*bytePool)
				dst := (*bytePool)[start : start+maxLen]
				n, err := base64.StdEncoding.Decode(dst, srcBytes)
				if err == nil {
					*bytePool = (*bytePool)[:start+n]
					return SpannerValue{Kind: KindBytes, BytesVal: dst[:n:n]}
				}
			} else {
				dst := make([]byte, maxLen)
				n, err := base64.StdEncoding.Decode(dst, srcBytes)
				if err == nil {
					return SpannerValue{Kind: KindBytes, BytesVal: dst[:n:n]}
				}
			}
			// Fallback to raw string if non-standard base64 so host codec can handle it.
			return SpannerValue{Kind: KindString, StrVal: s}
		}

	case sppb.TypeCode_TIMESTAMP:
		if sv, ok := v.Kind.(*proto3.Value_StringValue); ok {
			if sec, nano, ok := parseRFC3339NanoFast(sv.StringValue); ok {
				return SpannerValue{Kind: KindTimestamp, SecVal: sec, NanoVal: nano}
			}
			return SpannerValue{Kind: KindString, StrVal: sv.StringValue}
		}

	case sppb.TypeCode_ARRAY:
		if lv, ok := v.Kind.(*proto3.Value_ListValue); ok && lv.ListValue != nil {
			var elemType *sppb.Type
			if t != nil {
				elemType = t.ArrayElementType
			}
			vals := lv.ListValue.Values
			children := make([]SpannerValue, len(vals))
			for i, elem := range vals {
				children[i] = DecodeCellValue(elem, elemType)
			}
			return SpannerValue{Kind: KindArray, Children: children}
		}

	case sppb.TypeCode_STRUCT:
		if lv, ok := v.Kind.(*proto3.Value_ListValue); ok && lv.ListValue != nil {
			var structFields []*sppb.StructType_Field
			if t != nil && t.StructType != nil {
				structFields = t.StructType.Fields
			}
			vals := lv.ListValue.Values
			children := make([]SpannerValue, len(vals))
			for i, elem := range vals {
				var fieldType *sppb.Type
				if i < len(structFields) && structFields[i] != nil {
					fieldType = structFields[i].Type
				}
				children[i] = DecodeCellValue(elem, fieldType)
			}
			return SpannerValue{Kind: KindStruct, Children: children}
		}
	}

	// Default scalar fallback (STRING, INT64, NUMERIC, DATE, JSON, INTERVAL, UUID, ENUM, etc.)
	switch kv := v.Kind.(type) {
	case *proto3.Value_StringValue:
		return SpannerValue{Kind: KindString, StrVal: kv.StringValue}
	case *proto3.Value_BoolValue:
		return SpannerValue{Kind: KindBool, BoolVal: kv.BoolValue}
	case *proto3.Value_NumberValue:
		return SpannerValue{Kind: KindFloat64, Float64Val: kv.NumberValue}
	default:
		return SpannerValue{Kind: KindNull}
	}
}

func parseFloatSpecialOrStd(s string) (float64, bool) {
	switch s {
	case "Infinity", "+Infinity", "Inf", "+Inf":
		return math.Inf(1), true
	case "-Infinity", "-Inf":
		return math.Inf(-1), true
	case "NaN":
		return math.NaN(), true
	default:
		f, err := strconv.ParseFloat(s, 64)
		if err != nil {
			return 0, false
		}
		return f, true
	}
}

// parseRFC3339NanoFast parses standard Spanner UTC timestamps of the form
// "YYYY-MM-DDTHH:MM:SSZ" or "YYYY-MM-DDTHH:MM:SS.fffffffffZ" without layout reflection,
// falling back to time.Parse(time.RFC3339Nano, s) for non-standard formats.
func parseRFC3339NanoFast(s string) (int64, int32, bool) {
	n := len(s)
	if n >= 20 && n <= 30 && s[4] == '-' && s[7] == '-' && (s[10] == 'T' || s[10] == 't') && s[13] == ':' && s[16] == ':' && (s[n-1] == 'Z' || s[n-1] == 'z') {
		year, ok1 := parseDigits4(s, 0)
		month, ok2 := parseDigits2(s, 5)
		day, ok3 := parseDigits2(s, 8)
		hour, ok4 := parseDigits2(s, 11)
		min, ok5 := parseDigits2(s, 14)
		sec, ok6 := parseDigits2(s, 17)
		if ok1 && ok2 && ok3 && ok4 && ok5 && ok6 {
			var nanos int32
			validNanos := true
			if n > 20 {
				if s[19] != '.' {
					validNanos = false
				} else {
					fracLen := n - 1 - 20
					if fracLen < 1 || fracLen > 9 {
						validNanos = false
					} else {
						var frac int32
						for i := 20; i < n-1; i++ {
							ch := s[i]
							if ch < '0' || ch > '9' {
								validNanos = false
								break
							}
							frac = frac*10 + int32(ch-'0')
						}
						for i := fracLen; i < 9; i++ {
							frac *= 10
						}
						nanos = frac
					}
				}
			}
			if validNanos && month >= 1 && month <= 12 && day >= 1 && day <= maxDaysInMonth(year, month) && hour <= 23 && min <= 59 && sec <= 59 {
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
				unixSec := days*86400 + int64(hour)*3600 + int64(min)*60 + int64(sec)
				return unixSec, nanos, true
			}
		}
	}
	parsed, err := time.Parse(time.RFC3339Nano, s)
	if err != nil {
		return 0, 0, false
	}
	return parsed.Unix(), int32(parsed.Nanosecond()), true
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

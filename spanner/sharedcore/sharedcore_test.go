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
	"bytes"
	"encoding/base64"
	"math"
	"testing"

	sppb "cloud.google.com/go/spanner/apiv1/spannerpb"
	proto3 "google.golang.org/protobuf/types/known/structpb"
)

func TestDecodeCellValue_AllTypes(t *testing.T) {
	// 1. TIMESTAMP fast parser
	tsVal := DecodeCellValue(
		proto3.NewStringValue("2026-07-01T12:34:56.123456789Z"),
		&sppb.Type{Code: sppb.TypeCode_TIMESTAMP},
	)
	if tsVal.Kind != KindTimestamp || tsVal.SecVal != 1782909296 || tsVal.NanoVal != 123456789 {
		t.Fatalf("unexpected TIMESTAMP decode: %+v", tsVal)
	}

	// 2. BYTES base64 decode
	rawPayload := []byte("hello-spanner-binary-payload")
	b64Payload := base64.StdEncoding.EncodeToString(rawPayload)
	bytesVal := DecodeCellValue(
		proto3.NewStringValue(b64Payload),
		&sppb.Type{Code: sppb.TypeCode_BYTES},
	)
	if bytesVal.Kind != KindBytes || !bytes.Equal(bytesVal.BytesVal, rawPayload) {
		t.Fatalf("unexpected BYTES decode: %+v", bytesVal)
	}

	// 3. FLOAT64 & FLOAT32 special values ("Infinity", "-Infinity", "NaN")
	infVal := DecodeCellValue(
		proto3.NewStringValue("Infinity"),
		&sppb.Type{Code: sppb.TypeCode_FLOAT64},
	)
	if infVal.Kind != KindFloat64 || !math.IsInf(infVal.Float64Val, 1) {
		t.Fatalf("expected +Inf, got %+v", infVal)
	}
	nanVal := DecodeCellValue(
		proto3.NewStringValue("NaN"),
		&sppb.Type{Code: sppb.TypeCode_FLOAT32},
	)
	if nanVal.Kind != KindFloat32 || !math.IsNaN(nanVal.Float64Val) {
		t.Fatalf("expected NaN, got %+v", nanVal)
	}

	// 4. Recursive ARRAY & STRUCT
	structType := &sppb.Type{
		Code: sppb.TypeCode_STRUCT,
		StructType: &sppb.StructType{
			Fields: []*sppb.StructType_Field{
				{Name: "label", Type: &sppb.Type{Code: sppb.TypeCode_STRING}},
				{
					Name: "scores",
					Type: &sppb.Type{
						Code:             sppb.TypeCode_ARRAY,
						ArrayElementType: &sppb.Type{Code: sppb.TypeCode_INT64},
					},
				},
			},
		},
	}
	structProtoVal := &proto3.Value{
		Kind: &proto3.Value_ListValue{
			ListValue: &proto3.ListValue{
				Values: []*proto3.Value{
					proto3.NewStringValue("nested-struct"),
					{
						Kind: &proto3.Value_ListValue{
							ListValue: &proto3.ListValue{
								Values: []*proto3.Value{
									proto3.NewStringValue("10"),
									proto3.NewNullValue(),
									proto3.NewStringValue("20"),
								},
							},
						},
					},
				},
			},
		},
	}
	decodedStruct := DecodeCellValue(structProtoVal, structType)
	if decodedStruct.Kind != KindStruct || len(decodedStruct.Children) != 2 {
		t.Fatalf("unexpected STRUCT decode: %+v", decodedStruct)
	}
	if decodedStruct.Children[0].Kind != KindString || decodedStruct.Children[0].StrVal != "nested-struct" {
		t.Fatalf("unexpected STRUCT field 0: %+v", decodedStruct.Children[0])
	}
	arrField := decodedStruct.Children[1]
	if arrField.Kind != KindArray || len(arrField.Children) != 3 {
		t.Fatalf("unexpected nested ARRAY field: %+v", arrField)
	}
	if arrField.Children[0].StrVal != "10" || arrField.Children[1].Kind != KindNull || arrField.Children[2].StrVal != "20" {
		t.Fatalf("unexpected nested ARRAY elements: %+v", arrField.Children)
	}
}

func TestPackRowsIntoArena_ReusedSliceClearsStaleFields(t *testing.T) {
	// First call: ARRAY<STRING> with 1 child element allocates cells[0] (top) and cells[1] (child).
	arrFields := []*sppb.StructType_Field{
		{
			Name: "arr",
			Type: &sppb.Type{
				Code:             sppb.TypeCode_ARRAY,
				ArrayElementType: &sppb.Type{Code: sppb.TypeCode_STRING},
			},
		},
	}
	cells, _, _, _, scratch := packRowsIntoArenaInternal(
		[][]*proto3.Value{{
			proto3.NewListValue(&proto3.ListValue{
				Values: []*proto3.Value{proto3.NewStringValue("non-empty-child")},
			}),
		}},
		arrFields,
		true,
	)
	if scratch == nil || len(cells) < 2 {
		t.Fatalf("expected non-nil scratch with child cell, got len=%d", len(cells))
	}
	// Dirty the child slot (cells[1]) in spare capacity before returning scratch to pool.
	// In Call 2, topCells = 1 so acquireArenaScratch only clears cells[:1]. The child
	// slot at cells2[1] is allocated from spare capacity and hits the NULL early return
	// in packCellIntoArena, verifying that decode.go zeroes (*cells)[dstIdx] first.
	cells[1] = PackedCell{Kind: int32(KindString), Flags: 0xdeadbeef, NumBits: 0xcafebabe, Offset: 1234, Len: 5678}
	releaseArenaScratch(scratch)

	cells2, _, numRows, numCols, scratch2 := packRowsIntoArenaInternal(
		[][]*proto3.Value{{
			proto3.NewListValue(&proto3.ListValue{
				Values: []*proto3.Value{proto3.NewNullValue()},
			}),
		}},
		arrFields,
		true,
	)
	defer releaseArenaScratch(scratch2)
	if numRows != 1 || numCols != 1 || len(cells2) != 2 {
		t.Fatalf("unexpected dimensions: rows=%d cols=%d len=%d", numRows, numCols, len(cells2))
	}
	if cells2[1] != (PackedCell{}) {
		t.Errorf("expected stale PackedCell fields in child slot cells2[1] to be cleared, got %+v", cells2[1])
	}
}

func TestPackCellIntoArena_ReusedChildSlotsAreZeroed(t *testing.T) {
	stale := PackedCell{
		Kind:    int32(KindString),
		Flags:   0xdeadbeef,
		NumBits: 0xcafebabe,
		Offset:  1234,
		Len:     5678,
	}
	cases := []struct {
		name string
		typ  *sppb.Type
		elem *proto3.Value
	}{
		{
			name: "null STRING element",
			typ: &sppb.Type{
				Code:             sppb.TypeCode_ARRAY,
				ArrayElementType: &sppb.Type{Code: sppb.TypeCode_STRING},
			},
			elem: proto3.NewNullValue(),
		},
		{
			name: "null ARRAY element",
			typ: &sppb.Type{
				Code: sppb.TypeCode_ARRAY,
				ArrayElementType: &sppb.Type{
					Code:             sppb.TypeCode_ARRAY,
					ArrayElementType: &sppb.Type{Code: sppb.TypeCode_STRING},
				},
			},
			elem: proto3.NewNullValue(),
		},
		{
			name: "kind-mismatched BOOL element",
			typ: &sppb.Type{
				Code:             sppb.TypeCode_ARRAY,
				ArrayElementType: &sppb.Type{Code: sppb.TypeCode_BOOL},
			},
			elem: proto3.NewStringValue("not-a-bool"),
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			backing := make([]PackedCell, 2)
			backing[0] = stale
			backing[1] = stale
			cells := backing[:1]
			var arena []byte

			val := proto3.NewListValue(&proto3.ListValue{
				Values: []*proto3.Value{tc.elem},
			})
			packCellIntoArena(val, tc.typ, 0, &cells, &arena)

			if len(cells) != 2 {
				t.Fatalf("expected len(cells)=2, got %d", len(cells))
			}
			if cells[0].Kind != int32(KindArray) || cells[0].Offset != 1 || cells[0].Len != 1 {
				t.Fatalf("unexpected parent array cell descriptor: %+v", cells[0])
			}
			if cells[1] != (PackedCell{}) {
				t.Fatalf("expected child slot cells[1] in reused spare capacity to be zeroed, got %+v", cells[1])
			}
		})
	}
}

func TestKindMismatchParity_DecodeCellValueAndPackRowsIntoArena(t *testing.T) {
	mismatchCases := []struct {
		name string
		typ  *sppb.Type
		val  *proto3.Value
	}{
		{"BOOL with StringValue", &sppb.Type{Code: sppb.TypeCode_BOOL}, proto3.NewStringValue("true")},
		{"FLOAT64 with BoolValue", &sppb.Type{Code: sppb.TypeCode_FLOAT64}, proto3.NewBoolValue(true)},
		{"FLOAT32 with BoolValue", &sppb.Type{Code: sppb.TypeCode_FLOAT32}, proto3.NewBoolValue(false)},
		{"BYTES with NumberValue", &sppb.Type{Code: sppb.TypeCode_BYTES}, proto3.NewNumberValue(42)},
		{"PROTO with BoolValue", &sppb.Type{Code: sppb.TypeCode_PROTO}, proto3.NewBoolValue(true)},
		{"TIMESTAMP with NumberValue", &sppb.Type{Code: sppb.TypeCode_TIMESTAMP}, proto3.NewNumberValue(123)},
		{"ARRAY with StringValue", &sppb.Type{Code: sppb.TypeCode_ARRAY, ArrayElementType: &sppb.Type{Code: sppb.TypeCode_STRING}}, proto3.NewStringValue("not-a-list")},
		{"STRUCT with StringValue", &sppb.Type{Code: sppb.TypeCode_STRUCT}, proto3.NewStringValue("not-a-struct")},
	}

	for _, tc := range mismatchCases {
		t.Run(tc.name, func(t *testing.T) {
			dv := DecodeCellValue(tc.val, tc.typ)
			if dv.Kind != KindNull {
				t.Fatalf("DecodeCellValue(%s): expected KindNull, got %+v", tc.name, dv)
			}
			fields := []*sppb.StructType_Field{{Name: "col", Type: tc.typ}}
			rows := [][]*proto3.Value{{tc.val}}
			decodedRows := DecodeRows(rows, fields)
			if len(decodedRows) != 1 || len(decodedRows[0]) != 1 || decodedRows[0][0].Kind != KindNull {
				t.Fatalf("DecodeRows(%s): expected KindNull, got %+v", tc.name, decodedRows)
			}
			cells, _, numRows, numCols := PackRowsIntoArena(rows, fields)
			if numRows != 1 || numCols != 1 || len(cells) != 1 || cells[0] != (PackedCell{}) {
				t.Fatalf("PackRowsIntoArena(%s): expected zero PackedCell, got %+v", tc.name, cells)
			}
		})
	}
}

func TestFloatAndTimestampStringFallbacks(t *testing.T) {
	// Non-canonical float strings must fall back to KindString so host codecs handle them identically to stock.
	for _, code := range []sppb.TypeCode{sppb.TypeCode_FLOAT64, sppb.TypeCode_FLOAT32} {
		typ := &sppb.Type{Code: code}
		for _, nonCanonical := range []string{"Inf", "+Inf", "-Inf", "+Infinity", "3.14", "1_000.5", "not-a-float"} {
			val := proto3.NewStringValue(nonCanonical)
			dv := DecodeCellValue(val, typ)
			if dv.Kind != KindString || dv.StrVal != nonCanonical {
				t.Fatalf("DecodeCellValue(%v, %q): expected KindString %q, got %+v", code, nonCanonical, nonCanonical, dv)
			}
			cells, arena, _, _ := PackRowsIntoArena([][]*proto3.Value{{val}}, []*sppb.StructType_Field{{Name: "f", Type: typ}})
			if len(cells) != 1 || cells[0].Kind != int32(KindString) || string(arena[cells[0].Offset:cells[0].Offset+cells[0].Len]) != nonCanonical {
				t.Fatalf("PackRowsIntoArena(%v, %q): expected KindString in arena, got cell=%+v", code, nonCanonical, cells[0])
			}
		}
		for _, special := range []string{"Infinity", "-Infinity", "NaN"} {
			val := proto3.NewStringValue(special)
			dv := DecodeCellValue(val, typ)
			wantKind := KindFloat64
			if code == sppb.TypeCode_FLOAT32 {
				wantKind = KindFloat32
			}
			if dv.Kind != wantKind {
				t.Fatalf("DecodeCellValue(%v, %q): expected %v, got %+v", code, special, wantKind, dv)
			}
			cells, _, _, _ := PackRowsIntoArena([][]*proto3.Value{{val}}, []*sppb.StructType_Field{{Name: "f", Type: typ}})
			if len(cells) != 1 || cells[0].Kind != int32(wantKind) {
				t.Fatalf("PackRowsIntoArena(%v, %q): expected %v, got %+v", code, special, wantKind, cells[0])
			}
		}
	}

	// Lowercase 't' or 'z' on TIMESTAMP must fall back to KindString (matching time.RFC3339Nano).
	tsType := &sppb.Type{Code: sppb.TypeCode_TIMESTAMP}
	for _, rawTS := range []string{
		"2026-07-01t12:34:56.123456789Z",
		"2026-07-01T12:34:56.123456789z",
		"invalid-timestamp",
	} {
		val := proto3.NewStringValue(rawTS)
		dv := DecodeCellValue(val, tsType)
		if dv.Kind != KindString || dv.StrVal != rawTS {
			t.Fatalf("DecodeCellValue(TIMESTAMP, %q): expected KindString fallback, got %+v", rawTS, dv)
		}
		cells, arena, _, _ := PackRowsIntoArena([][]*proto3.Value{{val}}, []*sppb.StructType_Field{{Name: "ts", Type: tsType}})
		if len(cells) != 1 || cells[0].Kind != int32(KindString) || string(arena[cells[0].Offset:cells[0].Offset+cells[0].Len]) != rawTS {
			t.Fatalf("PackRowsIntoArena(TIMESTAMP, %q): expected KindString fallback, got %+v", rawTS, cells[0])
		}
	}

	// Valid non-Z RFC3339Nano offset timestamp uses time.Parse fallback and produces KindTimestamp.
	offsetTS := "2026-07-01T14:34:56.123456789+02:00"
	dv := DecodeCellValue(proto3.NewStringValue(offsetTS), tsType)
	if dv.Kind != KindTimestamp || dv.SecVal != 1782909296 || dv.NanoVal != 123456789 {
		t.Fatalf("expected KindTimestamp from RFC3339Nano offset fallback, got %+v", dv)
	}
}

func TestDecodeRowsAndPackRowsIntoArena_AllTypeCodes(t *testing.T) {
	if DecodeRows(nil, nil) != nil {
		t.Fatal("expected nil for empty rawRows")
	}
	if DecodeRows([][]*proto3.Value{{proto3.NewStringValue("x")}}, nil) != nil {
		t.Fatal("expected nil for empty fields")
	}
	if cells, arena, r, c := PackRowsIntoArena(nil, nil); cells != nil || arena != nil || r != 0 || c != 0 {
		t.Fatalf("expected empty PackRowsIntoArena for nil inputs, got r=%d c=%d", r, c)
	}

	largePayload := bytes.Repeat([]byte("abcd"), 128) // 512 bytes to exercise arena growth
	largeB64 := base64.StdEncoding.EncodeToString(largePayload)

	fields := []*sppb.StructType_Field{
		{Name: "c_bool", Type: &sppb.Type{Code: sppb.TypeCode_BOOL}},
		{Name: "c_f64", Type: &sppb.Type{Code: sppb.TypeCode_FLOAT64}},
		{Name: "c_f32", Type: &sppb.Type{Code: sppb.TypeCode_FLOAT32}},
		{Name: "c_bytes", Type: &sppb.Type{Code: sppb.TypeCode_BYTES}},
		{Name: "c_bytes_empty", Type: &sppb.Type{Code: sppb.TypeCode_BYTES}},
		{Name: "c_bytes_invalid", Type: &sppb.Type{Code: sppb.TypeCode_PROTO}},
		{Name: "c_ts", Type: &sppb.Type{Code: sppb.TypeCode_TIMESTAMP}},
		{Name: "c_str", Type: &sppb.Type{Code: sppb.TypeCode_STRING}},
		{Name: "c_untyped_bool", Type: &sppb.Type{Code: sppb.TypeCode_TYPE_CODE_UNSPECIFIED}},
		{Name: "c_untyped_num", Type: &sppb.Type{Code: sppb.TypeCode_TYPE_CODE_UNSPECIFIED}},
		{
			Name: "c_arr",
			Type: &sppb.Type{
				Code:             sppb.TypeCode_ARRAY,
				ArrayElementType: &sppb.Type{Code: sppb.TypeCode_INT64},
			},
		},
		{
			Name: "c_struct",
			Type: &sppb.Type{
				Code: sppb.TypeCode_STRUCT,
				StructType: &sppb.StructType{
					Fields: []*sppb.StructType_Field{
						{Name: "s_str", Type: &sppb.Type{Code: sppb.TypeCode_STRING}},
						{Name: "s_f64", Type: &sppb.Type{Code: sppb.TypeCode_FLOAT64}},
					},
				},
			},
		},
	}

	rawRows := [][]*proto3.Value{
		{
			proto3.NewBoolValue(true),
			proto3.NewNumberValue(3.25),
			proto3.NewNumberValue(1.5),
			proto3.NewStringValue(largeB64),
			proto3.NewStringValue(""),
			proto3.NewStringValue("!!!not-valid-base64!!!"),
			proto3.NewStringValue("2026-07-01T12:34:56Z"),
			proto3.NewStringValue("hello"),
			proto3.NewBoolValue(false),
			proto3.NewNumberValue(99.5),
			proto3.NewListValue(&proto3.ListValue{
				Values: []*proto3.Value{proto3.NewStringValue("100"), proto3.NewNullValue()},
			}),
			proto3.NewListValue(&proto3.ListValue{
				Values: []*proto3.Value{proto3.NewStringValue("inner"), proto3.NewNumberValue(2.5)},
			}),
		},
	}

	decoded := DecodeRows(rawRows, fields)
	if len(decoded) != 1 || len(decoded[0]) != len(fields) {
		t.Fatalf("unexpected DecodeRows dimensions: %d x %d", len(decoded), len(decoded[0]))
	}
	row := decoded[0]
	if row[0].Kind != KindBool || !row[0].BoolVal {
		t.Fatalf("col 0 (BOOL): %+v", row[0])
	}
	if row[1].Kind != KindFloat64 || row[1].Float64Val != 3.25 {
		t.Fatalf("col 1 (FLOAT64): %+v", row[1])
	}
	if row[2].Kind != KindFloat32 || row[2].Float64Val != 1.5 {
		t.Fatalf("col 2 (FLOAT32): %+v", row[2])
	}
	if row[3].Kind != KindBytes || !bytes.Equal(row[3].BytesVal, largePayload) {
		t.Fatalf("col 3 (BYTES): kind=%v len=%d", row[3].Kind, len(row[3].BytesVal))
	}
	if row[4].Kind != KindBytes || len(row[4].BytesVal) != 0 {
		t.Fatalf("col 4 (empty BYTES): %+v", row[4])
	}
	if row[5].Kind != KindString || row[5].StrVal != "!!!not-valid-base64!!!" {
		t.Fatalf("col 5 (invalid base64 PROTO fallback): %+v", row[5])
	}
	if row[6].Kind != KindTimestamp || row[6].SecVal != 1782909296 || row[6].NanoVal != 0 {
		t.Fatalf("col 6 (TIMESTAMP): %+v", row[6])
	}
	if row[7].Kind != KindString || row[7].StrVal != "hello" {
		t.Fatalf("col 7 (STRING): %+v", row[7])
	}
	if row[8].Kind != KindBool || row[8].BoolVal {
		t.Fatalf("col 8 (untyped bool): %+v", row[8])
	}
	if row[9].Kind != KindFloat64 || row[9].Float64Val != 99.5 {
		t.Fatalf("col 9 (untyped num): %+v", row[9])
	}
	if row[10].Kind != KindArray || len(row[10].Children) != 2 || row[10].Children[0].StrVal != "100" || row[10].Children[1].Kind != KindNull {
		t.Fatalf("col 10 (ARRAY): %+v", row[10])
	}
	if row[11].Kind != KindStruct || len(row[11].Children) != 2 || row[11].Children[0].StrVal != "inner" || row[11].Children[1].Float64Val != 2.5 {
		t.Fatalf("col 11 (STRUCT): %+v", row[11])
	}

	cells, arena, numRows, numCols := PackRowsIntoArena(rawRows, fields)
	if numRows != 1 || numCols != len(fields) || len(cells) != len(fields)+4 {
		t.Fatalf("unexpected PackRowsIntoArena dimensions: rows=%d cols=%d len(cells)=%d", numRows, numCols, len(cells))
	}
	if cells[0].Kind != int32(KindBool) || cells[0].Flags != 1 {
		t.Fatalf("arena col 0: %+v", cells[0])
	}
	if cells[1].Kind != int32(KindFloat64) || math.Float64frombits(cells[1].NumBits) != 3.25 {
		t.Fatalf("arena col 1: %+v", cells[1])
	}
	if cells[2].Kind != int32(KindFloat32) || math.Float64frombits(cells[2].NumBits) != 1.5 {
		t.Fatalf("arena col 2: %+v", cells[2])
	}
	if cells[3].Kind != int32(KindBytes) || !bytes.Equal(arena[cells[3].Offset:cells[3].Offset+cells[3].Len], largePayload) {
		t.Fatalf("arena col 3: %+v", cells[3])
	}
	if cells[4].Kind != int32(KindBytes) || cells[4].Len != 0 {
		t.Fatalf("arena col 4: %+v", cells[4])
	}
	if cells[5].Kind != int32(KindString) || string(arena[cells[5].Offset:cells[5].Offset+cells[5].Len]) != "!!!not-valid-base64!!!" {
		t.Fatalf("arena col 5: %+v", cells[5])
	}
	if cells[6].Kind != int32(KindTimestamp) || int64(cells[6].NumBits) != 1782909296 || cells[6].Flags != 0 {
		t.Fatalf("arena col 6: %+v", cells[6])
	}
	if cells[10].Kind != int32(KindArray) || cells[10].Len != 2 {
		t.Fatalf("arena col 10: %+v", cells[10])
	}
	if cells[11].Kind != int32(KindStruct) || cells[11].Len != 2 {
		t.Fatalf("arena col 11: %+v", cells[11])
	}
}

func TestTimestampCalendarBranches_RaggedRows_ManyCols_AndOutlierRow0Cap(t *testing.T) {
	tsType := &sppb.Type{Code: sppb.TypeCode_TIMESTAMP}
	// 1. Jan/Feb (month <= 2), <9 fractional digits (.123Z), 30-day month (April 30),
	//    leap-year Feb 29 (2024 & 2000), non-leap Feb 28 (2025), and pre-0000 negative era branch.
	validTSCases := []struct {
		raw      string
		wantSec  int64
		wantNano int32
	}{
		{"2026-01-15T00:00:00.123Z", 1768435200, 123000000},
		{"2024-02-29T23:59:59.5Z", 1709251199, 500000000},
		{"2000-02-29T00:00:00Z", 951782400, 0},
		{"2025-02-28T12:00:00Z", 1740744000, 0},
		{"2026-04-30T06:30:00.000001Z", 1777530600, 1000},
	}
	for _, tc := range validTSCases {
		dv := DecodeCellValue(proto3.NewStringValue(tc.raw), tsType)
		if dv.Kind != KindTimestamp || dv.SecVal != tc.wantSec || dv.NanoVal != tc.wantNano {
			t.Fatalf("DecodeCellValue(%q): expected (%d, %d), got %+v", tc.raw, tc.wantSec, tc.wantNano, dv)
		}
	}
	// Invalid calendar dates (non-leap Feb 29, month 13, non-digit frac, bad dot) fall back to KindString.
	for _, invalidTS := range []string{
		"2025-02-29T00:00:00Z",
		"2026-13-01T00:00:00Z",
		"2026-04-31T00:00:00Z",
		"2026-07-01T12:34:56.12aZ",
		"2026-07-01T12:34:56X123Z",
		"2026-07-01T12:34:56.Z",
		"202X-07-01T12:34:56Z",
		"2026-0X-01T12:34:56Z",
	} {
		dv := DecodeCellValue(proto3.NewStringValue(invalidTS), tsType)
		if dv.Kind != KindString || dv.StrVal != invalidTS {
			t.Fatalf("DecodeCellValue(%q): expected KindString fallback, got %+v", invalidTS, dv)
		}
	}

	// 2. >32 columns (exercises heap allocation fallback beyond colTypesStack[32]) and ragged rows.
	const numManyCols = 35
	manyFields := make([]*sppb.StructType_Field, numManyCols)
	fullRow := make([]*proto3.Value, numManyCols+2) // extra trailing cells (> numCols) truncated
	for i := 0; i < numManyCols; i++ {
		manyFields[i] = &sppb.StructType_Field{Name: "c", Type: &sppb.Type{Code: sppb.TypeCode_STRING}}
		fullRow[i] = proto3.NewStringValue("v")
	}
	fullRow[numManyCols] = proto3.NewStringValue("ignored-extra-1")
	fullRow[numManyCols+1] = proto3.NewStringValue("ignored-extra-2")
	shortRow := []*proto3.Value{proto3.NewStringValue("only-col-0")} // < numCols padded with KindNull

	raggedRows := [][]*proto3.Value{fullRow, shortRow}
	decRagged := DecodeRows(raggedRows, manyFields)
	if len(decRagged) != 2 || len(decRagged[0]) != numManyCols || len(decRagged[1]) != numManyCols {
		t.Fatalf("unexpected ragged DecodeRows dimensions")
	}
	if decRagged[1][0].StrVal != "only-col-0" || decRagged[1][numManyCols-1].Kind != KindNull {
		t.Fatalf("expected short row trailing column to be KindNull, got %+v", decRagged[1][numManyCols-1])
	}
	cellsRagged, _, rCount, cCount := PackRowsIntoArena(raggedRows, manyFields)
	if rCount != 2 || cCount != numManyCols || len(cellsRagged) != 2*numManyCols {
		t.Fatalf("unexpected ragged PackRowsIntoArena dimensions: r=%d c=%d len=%d", rCount, cCount, len(cellsRagged))
	}
	if cellsRagged[numManyCols].Kind != int32(KindString) || cellsRagged[2*numManyCols-1] != (PackedCell{}) {
		t.Fatalf("expected short row trailing PackedCell to be zeroed, got %+v", cellsRagged[2*numManyCols-1])
	}

	// 3. Outlier Row 0 estBytes cap at maxPooledArenaBytes (1 MiB).
	outlierFields := []*sppb.StructType_Field{{Name: "s", Type: &sppb.Type{Code: sppb.TypeCode_STRING}}}
	outlierRows := make([][]*proto3.Value, 128)
	outlierRows[0] = []*proto3.Value{proto3.NewStringValue(string(bytes.Repeat([]byte("x"), 16*1024)))} // 16 KB * 128 * 1.25 = 2.5 MiB uncapped
	for i := 1; i < len(outlierRows); i++ {
		outlierRows[i] = []*proto3.Value{proto3.NewStringValue("small")}
	}
	_, outlierArena, _, _, outlierScratch := packRowsIntoArenaInternal(outlierRows, outlierFields, true)
	defer releaseArenaScratch(outlierScratch)
	if cap(outlierArena) > maxPooledArenaBytes {
		t.Fatalf("expected outlier Row 0 arena capacity to be capped at %d, got %d", maxPooledArenaBytes, cap(outlierArena))
	}
}

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
	"strings"
	"testing"
	"time"
	"unsafe"

	sppb "cloud.google.com/go/spanner/apiv1/spannerpb"
	proto3 "google.golang.org/protobuf/types/known/structpb"
)

func TestDecodeCellValue_AllTypes(t *testing.T) {
	// 1. TIMESTAMP fast parser
	tsVal, err := DecodeCellValue(
		proto3.NewStringValue("2026-07-01T12:34:56.123456789Z"),
		&sppb.Type{Code: sppb.TypeCode_TIMESTAMP},
	)
	if err != nil || tsVal.Kind != KindTimestamp || tsVal.Int64Val != 1782909296 || tsVal.Int32Val != 123456789 {
		t.Fatalf("unexpected TIMESTAMP decode: val=%+v err=%v", tsVal, err)
	}

	// 2. BYTES base64 decode
	rawPayload := []byte("hello-spanner-binary-payload")
	b64Payload := base64.StdEncoding.EncodeToString(rawPayload)
	bytesVal, err := DecodeCellValue(
		proto3.NewStringValue(b64Payload),
		&sppb.Type{Code: sppb.TypeCode_BYTES},
	)
	if err != nil || bytesVal.Kind != KindBytes || !bytes.Equal(bytesVal.BytesVal, rawPayload) {
		t.Fatalf("unexpected BYTES decode: val=%+v err=%v", bytesVal, err)
	}

	// 3. FLOAT64 & FLOAT32 special values ("Infinity", "-Infinity", "NaN")
	infVal, err := DecodeCellValue(
		proto3.NewStringValue("Infinity"),
		&sppb.Type{Code: sppb.TypeCode_FLOAT64},
	)
	if err != nil || infVal.Kind != KindFloat64 || !math.IsInf(infVal.Float64Val, 1) {
		t.Fatalf("expected +Inf, got val=%+v err=%v", infVal, err)
	}
	nanVal, err := DecodeCellValue(
		proto3.NewStringValue("NaN"),
		&sppb.Type{Code: sppb.TypeCode_FLOAT32},
	)
	if err != nil || nanVal.Kind != KindFloat32 || !math.IsNaN(nanVal.Float64Val) {
		t.Fatalf("expected NaN, got val=%+v err=%v", nanVal, err)
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
	decodedStruct, err := DecodeCellValue(structProtoVal, structType)
	if err != nil || decodedStruct.Kind != KindStruct || len(decodedStruct.Children) != 2 {
		t.Fatalf("unexpected STRUCT decode: val=%+v err=%v", decodedStruct, err)
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
	cells, _, _, _, scratch, err := packRowsIntoArenaInternal(
		[][]*proto3.Value{{
			proto3.NewListValue(&proto3.ListValue{
				Values: []*proto3.Value{proto3.NewStringValue("non-empty-child")},
			}),
		}},
		arrFields,
		true,
	)
	if err != nil || scratch == nil || len(cells) < 2 {
		t.Fatalf("expected non-nil scratch with child cell, got len=%d err=%v", len(cells), err)
	}
	// Dirty the child slot (cells[1]) in spare capacity before returning scratch to pool.
	cells[1] = PackedCell{Kind: int32(KindString), Flags: 0xdeadbeef, NumBits: 0xcafebabe, Offset: 1234, Len: 5678}
	releaseArenaScratch(scratch)

	cells2, _, numRows, numCols, scratch2, err := packRowsIntoArenaInternal(
		[][]*proto3.Value{{
			proto3.NewListValue(&proto3.ListValue{
				Values: []*proto3.Value{proto3.NewNullValue()},
			}),
		}},
		arrFields,
		true,
	)
	defer releaseArenaScratch(scratch2)
	if err != nil || numRows != 1 || numCols != 1 || len(cells2) != 2 {
		t.Fatalf("unexpected dimensions: rows=%d cols=%d len=%d err=%v", numRows, numCols, len(cells2), err)
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
		name    string
		typ     *sppb.Type
		elem    *proto3.Value
		wantErr bool
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
			name: "kind-mismatched BOOL element zeroes child slot and returns error",
			typ: &sppb.Type{
				Code:             sppb.TypeCode_ARRAY,
				ArrayElementType: &sppb.Type{Code: sppb.TypeCode_BOOL},
			},
			elem:    proto3.NewStringValue("not-a-bool"),
			wantErr: true,
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
			err := packCellIntoArena(val, tc.typ, 0, &cells, &arena)
			if (err != nil) != tc.wantErr {
				t.Fatalf("packCellIntoArena(%s): err=%v, wantErr=%v", tc.name, err, tc.wantErr)
			}
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

func TestKindMismatchReturnsError_DecodeAndPack(t *testing.T) {
	mismatchCases := []struct {
		name string
		typ  *sppb.Type
		val  *proto3.Value
	}{
		{"BOOL with StringValue", &sppb.Type{Code: sppb.TypeCode_BOOL}, proto3.NewStringValue("true")},
		{"BOOL with typed-nil BoolValue", &sppb.Type{Code: sppb.TypeCode_BOOL}, &proto3.Value{Kind: (*proto3.Value_BoolValue)(nil)}},
		{"FLOAT64 with BoolValue", &sppb.Type{Code: sppb.TypeCode_FLOAT64}, proto3.NewBoolValue(true)},
		{"FLOAT64 with typed-nil NumberValue", &sppb.Type{Code: sppb.TypeCode_FLOAT64}, &proto3.Value{Kind: (*proto3.Value_NumberValue)(nil)}},
		{"FLOAT64 with typed-nil StringValue", &sppb.Type{Code: sppb.TypeCode_FLOAT64}, &proto3.Value{Kind: (*proto3.Value_StringValue)(nil)}},
		{"FLOAT32 with BoolValue", &sppb.Type{Code: sppb.TypeCode_FLOAT32}, proto3.NewBoolValue(false)},
		{"BYTES with NumberValue", &sppb.Type{Code: sppb.TypeCode_BYTES}, proto3.NewNumberValue(42)},
		{"BYTES with typed-nil StringValue", &sppb.Type{Code: sppb.TypeCode_BYTES}, &proto3.Value{Kind: (*proto3.Value_StringValue)(nil)}},
		{"PROTO with BoolValue", &sppb.Type{Code: sppb.TypeCode_PROTO}, proto3.NewBoolValue(true)},
		{"TIMESTAMP with NumberValue", &sppb.Type{Code: sppb.TypeCode_TIMESTAMP}, proto3.NewNumberValue(123)},
		{"TIMESTAMP with typed-nil StringValue", &sppb.Type{Code: sppb.TypeCode_TIMESTAMP}, &proto3.Value{Kind: (*proto3.Value_StringValue)(nil)}},
		{"ARRAY with StringValue", &sppb.Type{Code: sppb.TypeCode_ARRAY, ArrayElementType: &sppb.Type{Code: sppb.TypeCode_STRING}}, proto3.NewStringValue("not-a-list")},
		{"ARRAY with nil ListValue", &sppb.Type{Code: sppb.TypeCode_ARRAY, ArrayElementType: &sppb.Type{Code: sppb.TypeCode_STRING}}, &proto3.Value{Kind: &proto3.Value_ListValue{ListValue: nil}}},
		{"ARRAY with mismatched child", &sppb.Type{Code: sppb.TypeCode_ARRAY, ArrayElementType: &sppb.Type{Code: sppb.TypeCode_BOOL}}, proto3.NewListValue(&proto3.ListValue{Values: []*proto3.Value{proto3.NewStringValue("bad")}})},
		{"STRUCT with StringValue", &sppb.Type{Code: sppb.TypeCode_STRUCT}, proto3.NewStringValue("not-a-struct")},
		{"STRUCT with mismatched field", &sppb.Type{Code: sppb.TypeCode_STRUCT, StructType: &sppb.StructType{Fields: []*sppb.StructType_Field{{Name: "b", Type: &sppb.Type{Code: sppb.TypeCode_BOOL}}}}}, proto3.NewListValue(&proto3.ListValue{Values: []*proto3.Value{proto3.NewNumberValue(1)}})},
		{"STRING with StructValue", &sppb.Type{Code: sppb.TypeCode_STRING}, &proto3.Value{Kind: &proto3.Value_StructValue{}}},
		{"STRING with NumberValue", &sppb.Type{Code: sppb.TypeCode_STRING}, proto3.NewNumberValue(123)},
		{"STRING with BoolValue", &sppb.Type{Code: sppb.TypeCode_STRING}, proto3.NewBoolValue(true)},
		{"STRING with ListValue", &sppb.Type{Code: sppb.TypeCode_STRING}, proto3.NewListValue(&proto3.ListValue{})},
		{"STRING with typed-nil StringValue", &sppb.Type{Code: sppb.TypeCode_STRING}, &proto3.Value{Kind: (*proto3.Value_StringValue)(nil)}},
		{"INT64 with NumberValue", &sppb.Type{Code: sppb.TypeCode_INT64}, proto3.NewNumberValue(42)},
		{"INT64 with typed-nil StringValue", &sppb.Type{Code: sppb.TypeCode_INT64}, &proto3.Value{Kind: (*proto3.Value_StringValue)(nil)}},
		{"NUMERIC with NumberValue", &sppb.Type{Code: sppb.TypeCode_NUMERIC}, proto3.NewNumberValue(3.14)},
		{"DATE with NumberValue", &sppb.Type{Code: sppb.TypeCode_DATE}, proto3.NewNumberValue(20260701)},
		{"JSON with BoolValue", &sppb.Type{Code: sppb.TypeCode_JSON}, proto3.NewBoolValue(false)},
		{"INTERVAL with NumberValue", &sppb.Type{Code: sppb.TypeCode_INTERVAL}, proto3.NewNumberValue(1)},
		{"UUID with NumberValue", &sppb.Type{Code: sppb.TypeCode_UUID}, proto3.NewNumberValue(1)},
		{"ENUM with NumberValue", &sppb.Type{Code: sppb.TypeCode_ENUM}, proto3.NewNumberValue(1)},
		{"UNSPECIFIED with typed-nil BoolValue", &sppb.Type{Code: sppb.TypeCode_TYPE_CODE_UNSPECIFIED}, &proto3.Value{Kind: (*proto3.Value_BoolValue)(nil)}},
		{"UNSPECIFIED with typed-nil NumberValue", &sppb.Type{Code: sppb.TypeCode_TYPE_CODE_UNSPECIFIED}, &proto3.Value{Kind: (*proto3.Value_NumberValue)(nil)}},
		{"UNSPECIFIED with nil ListValue", &sppb.Type{Code: sppb.TypeCode_TYPE_CODE_UNSPECIFIED}, &proto3.Value{Kind: &proto3.Value_ListValue{ListValue: nil}}},
	}

	for _, tc := range mismatchCases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := DecodeCellValue(tc.val, tc.typ)
			if err == nil {
				t.Fatalf("DecodeCellValue(%s): expected error, got nil", tc.name)
			}
			if gotCount := strings.Count(err.Error(), "sharedcore:"); gotCount != 1 {
				t.Fatalf("DecodeCellValue(%s): expected exactly 1 'sharedcore:' prefix, got %d in %q", tc.name, gotCount, err.Error())
			}
			if strings.Contains(tc.name, "mismatched") && !strings.Contains(err.Error(), "element 0:") {
				t.Fatalf("DecodeCellValue(%s): expected 'element 0:' in nested error, got %q", tc.name, err.Error())
			}
			fields := []*sppb.StructType_Field{{Name: "col", Type: tc.typ}}
			rows := [][]*proto3.Value{{tc.val}}
			decodedRows, err := DecodeRows(rows, fields)
			if err == nil || decodedRows != nil || !strings.Contains(err.Error(), `sharedcore: row 0, col 0 ("col"):`) {
				t.Fatalf("DecodeRows(%s): expected (nil, row/col wrapped error), got rows=%+v err=%v", tc.name, decodedRows, err)
			}
			if gotCount := strings.Count(err.Error(), "sharedcore:"); gotCount != 1 {
				t.Fatalf("DecodeRows(%s): expected exactly 1 'sharedcore:' prefix, got %d in %q", tc.name, gotCount, err.Error())
			}
			if strings.Contains(tc.name, "mismatched") && !strings.Contains(err.Error(), "element 0:") {
				t.Fatalf("DecodeRows(%s): expected 'element 0:' in nested error, got %q", tc.name, err.Error())
			}
			cells, arena, _, _, err := PackRowsIntoArena(rows, fields)
			if err == nil || cells != nil || arena != nil || !strings.Contains(err.Error(), `sharedcore: row 0, col 0 ("col"):`) {
				t.Fatalf("PackRowsIntoArena(%s): expected (nil, nil, row/col wrapped error), got cells=%+v err=%v", tc.name, cells, err)
			}
			if gotCount := strings.Count(err.Error(), "sharedcore:"); gotCount != 1 {
				t.Fatalf("PackRowsIntoArena(%s): expected exactly 1 'sharedcore:' prefix, got %d in %q", tc.name, gotCount, err.Error())
			}
			if strings.Contains(tc.name, "mismatched") && !strings.Contains(err.Error(), "element 0:") {
				t.Fatalf("PackRowsIntoArena(%s): expected 'element 0:' in nested error, got %q", tc.name, err.Error())
			}
			if cells, arena, _, _, scratch, err := packRowsIntoArenaInternal(rows, fields, true); err == nil || cells != nil || arena != nil || scratch != nil || !strings.Contains(err.Error(), `sharedcore: row 0, col 0 ("col"):`) {
				t.Fatalf("packRowsIntoArenaInternal(%s, usePool=true): expected error and released scratch, got err=%v", tc.name, err)
			}
		})
	}
}

func TestDefaultListValueFallback_AndNestedBytePool(t *testing.T) {
	// 1. Unrecognized / unspecified TypeCode with ListValue falls back to KindArray recursively.
	untypedType := &sppb.Type{Code: sppb.TypeCode_TYPE_CODE_UNSPECIFIED}
	listVal := proto3.NewListValue(&proto3.ListValue{
		Values: []*proto3.Value{
			proto3.NewStringValue("alpha"),
			proto3.NewBoolValue(true),
			proto3.NewNumberValue(42.5),
			proto3.NewListValue(&proto3.ListValue{
				Values: []*proto3.Value{proto3.NewStringValue("nested")},
			}),
		},
	})
	dv, err := DecodeCellValue(listVal, untypedType)
	if err != nil || dv.Kind != KindArray || len(dv.Children) != 4 {
		t.Fatalf("DecodeCellValue(untyped ListValue): got %+v, err=%v", dv, err)
	}
	if dv.Children[0].StrVal != "alpha" || !dv.Children[1].BoolVal || dv.Children[2].Float64Val != 42.5 || dv.Children[3].Kind != KindArray || dv.Children[3].Children[0].StrVal != "nested" {
		t.Fatalf("unexpected untyped ListValue children: %+v", dv.Children)
	}

	cells, arena, r, c, err := PackRowsIntoArena([][]*proto3.Value{{listVal}}, []*sppb.StructType_Field{{Name: "u", Type: untypedType}})
	if err != nil || r != 1 || c != 1 || len(cells) != 6 {
		t.Fatalf("PackRowsIntoArena(untyped ListValue): r=%d c=%d len(cells)=%d err=%v", r, c, len(cells), err)
	}
	if cells[0].Kind != int32(KindArray) || cells[0].Offset != 1 || cells[0].Len != 4 {
		t.Fatalf("unexpected parent cell for untyped ListValue: %+v", cells[0])
	}
	if cells[4].Kind != int32(KindArray) || cells[4].Offset != 5 || cells[4].Len != 1 {
		t.Fatalf("unexpected nested list cell: %+v", cells[4])
	}
	if string(arena[cells[5].Offset:cells[5].Offset+cells[5].Len]) != "nested" {
		t.Fatalf("unexpected nested list string in arena")
	}

	// 2. Nested ARRAY<BYTES> and STRUCT<BYTES> in DecodeRows share the pre-sized contiguous bytePool.
	b1 := []byte("nested-bytes-one")
	b2 := []byte("nested-bytes-two")
	b3 := []byte("struct-bytes-three")
	fields := []*sppb.StructType_Field{
		{
			Name: "arr_bytes",
			Type: &sppb.Type{
				Code:             sppb.TypeCode_ARRAY,
				ArrayElementType: &sppb.Type{Code: sppb.TypeCode_BYTES},
			},
		},
		{
			Name: "struct_bytes",
			Type: &sppb.Type{
				Code: sppb.TypeCode_STRUCT,
				StructType: &sppb.StructType{
					Fields: []*sppb.StructType_Field{
						{Name: "b", Type: &sppb.Type{Code: sppb.TypeCode_PROTO}},
					},
				},
			},
		},
	}
	rawRows := [][]*proto3.Value{
		{
			proto3.NewListValue(&proto3.ListValue{
				Values: []*proto3.Value{
					proto3.NewStringValue(base64.StdEncoding.EncodeToString(b1)),
					proto3.NewStringValue(base64.StdEncoding.EncodeToString(b2)),
				},
			}),
			proto3.NewListValue(&proto3.ListValue{
				Values: []*proto3.Value{
					proto3.NewStringValue(base64.StdEncoding.EncodeToString(b3)),
				},
			}),
		},
	}
	decoded, err := DecodeRows(rawRows, fields)
	if err != nil || len(decoded) != 1 {
		t.Fatalf("DecodeRows(nested bytes) failed: %v", err)
	}
	s1 := decoded[0][0].Children[0].BytesVal
	s2 := decoded[0][0].Children[1].BytesVal
	s3 := decoded[0][1].Children[0].BytesVal
	if !bytes.Equal(s1, b1) || !bytes.Equal(s2, b2) || !bytes.Equal(s3, b3) {
		t.Fatalf("unexpected decoded nested bytes: %q, %q, %q", s1, s2, s3)
	}
	// Verify s1, s2, and s3 are contiguous sub-slices of the same pre-sized bytePool backing array.
	p1 := uintptr(unsafe.Pointer(&s1[0]))
	p2 := uintptr(unsafe.Pointer(&s2[0]))
	p3 := uintptr(unsafe.Pointer(&s3[0]))
	if p2 != p1+uintptr(len(b1)) || p3 != p2+uintptr(len(b2)) {
		t.Fatalf("expected nested BYTES/PROTO slices to be allocated contiguously from bytePool: p1=%x p2=%x p3=%x", p1, p2, p3)
	}
}

func TestFloatBytesAndTimestampInvalidStringErrors(t *testing.T) {
	// Non-canonical float strings must return an error on both DecodeCellValue and PackRowsIntoArena.
	for _, code := range []sppb.TypeCode{sppb.TypeCode_FLOAT64, sppb.TypeCode_FLOAT32} {
		typ := &sppb.Type{Code: code}
		for _, nonCanonical := range []string{"Inf", "+Inf", "-Inf", "+Infinity", "3.14", "1_000.5", "not-a-float"} {
			val := proto3.NewStringValue(nonCanonical)
			if _, err := DecodeCellValue(val, typ); err == nil {
				t.Fatalf("DecodeCellValue(%v, %q): expected error, got nil", code, nonCanonical)
			}
			if _, _, _, _, err := PackRowsIntoArena([][]*proto3.Value{{val}}, []*sppb.StructType_Field{{Name: "f", Type: typ}}); err == nil {
				t.Fatalf("PackRowsIntoArena(%v, %q): expected error, got nil", code, nonCanonical)
			}
		}
		for _, special := range []string{"Infinity", "-Infinity", "NaN"} {
			val := proto3.NewStringValue(special)
			dv, err := DecodeCellValue(val, typ)
			wantKind := KindFloat64
			if code == sppb.TypeCode_FLOAT32 {
				wantKind = KindFloat32
			}
			if err != nil || dv.Kind != wantKind {
				t.Fatalf("DecodeCellValue(%v, %q): expected %v, got %+v err=%v", code, special, wantKind, dv, err)
			}
			cells, _, _, _, err := PackRowsIntoArena([][]*proto3.Value{{val}}, []*sppb.StructType_Field{{Name: "f", Type: typ}})
			if err != nil || len(cells) != 1 || cells[0].Kind != int32(wantKind) {
				t.Fatalf("PackRowsIntoArena(%v, %q): expected %v, got cells=%+v err=%v", code, special, wantKind, cells, err)
			}
		}
	}

	// Invalid base64 on BYTES or PROTO must return an error.
	for _, code := range []sppb.TypeCode{sppb.TypeCode_BYTES, sppb.TypeCode_PROTO} {
		typ := &sppb.Type{Code: code}
		badB64 := proto3.NewStringValue("!!!not-valid-base64!!!")
		if _, err := DecodeCellValue(badB64, typ); err == nil {
			t.Fatalf("DecodeCellValue(%v, invalid base64): expected error, got nil", code)
		}
		if _, _, _, _, err := PackRowsIntoArena([][]*proto3.Value{{badB64}}, []*sppb.StructType_Field{{Name: "b", Type: typ}}); err == nil {
			t.Fatalf("PackRowsIntoArena(%v, invalid base64): expected error, got nil", code)
		}
	}

	// Lowercase 't' or 'z' or malformed TIMESTAMP strings must return an error (matching time.RFC3339Nano).
	tsType := &sppb.Type{Code: sppb.TypeCode_TIMESTAMP}
	for _, rawTS := range []string{
		"2026-07-01t12:34:56.123456789Z",
		"2026-07-01T12:34:56.123456789z",
		"invalid-timestamp",
	} {
		val := proto3.NewStringValue(rawTS)
		if _, err := DecodeCellValue(val, tsType); err == nil {
			t.Fatalf("DecodeCellValue(TIMESTAMP, %q): expected error, got nil", rawTS)
		}
		if _, _, _, _, err := PackRowsIntoArena([][]*proto3.Value{{val}}, []*sppb.StructType_Field{{Name: "ts", Type: tsType}}); err == nil {
			t.Fatalf("PackRowsIntoArena(TIMESTAMP, %q): expected error, got nil", rawTS)
		}
	}

	// Valid non-Z RFC3339Nano offset timestamp uses time.Parse fallback and produces KindTimestamp.
	offsetTS := "2026-07-01T14:34:56.123456789+02:00"
	dv, err := DecodeCellValue(proto3.NewStringValue(offsetTS), tsType)
	if err != nil || dv.Kind != KindTimestamp || dv.Int64Val != 1782909296 || dv.Int32Val != 123456789 {
		t.Fatalf("expected KindTimestamp from RFC3339Nano offset fallback, got %+v err=%v", dv, err)
	}
	cells, _, _, _, err := PackRowsIntoArena([][]*proto3.Value{{proto3.NewStringValue(offsetTS)}}, []*sppb.StructType_Field{{Name: "ts", Type: tsType}})
	if err != nil || len(cells) != 1 || cells[0].Kind != int32(KindTimestamp) || int64(cells[0].NumBits) != 1782909296 || int32(cells[0].Flags) != 123456789 {
		t.Fatalf("expected KindTimestamp in arena from RFC3339Nano offset fallback, got cells=%+v err=%v", cells, err)
	}
}

func TestDecodeRowsAndPackRowsIntoArena_AllTypeCodes(t *testing.T) {
	if rows, err := DecodeRows(nil, nil); rows != nil || err != nil {
		t.Fatalf("expected (nil, nil) for empty rawRows, got rows=%v err=%v", rows, err)
	}
	if rows, err := DecodeRows([][]*proto3.Value{{proto3.NewStringValue("x")}}, nil); rows != nil || err != nil {
		t.Fatalf("expected (nil, nil) for empty fields, got rows=%v err=%v", rows, err)
	}
	if cells, arena, r, c, err := PackRowsIntoArena(nil, nil); cells != nil || arena != nil || r != 0 || c != 0 || err != nil {
		t.Fatalf("expected empty PackRowsIntoArena for nil inputs, got r=%d c=%d err=%v", r, c, err)
	}

	largePayload := bytes.Repeat([]byte("abcd"), 128) // 512 bytes to exercise arena growth
	largeB64 := base64.StdEncoding.EncodeToString(largePayload)

	protoPayload := []byte("proto-bytes-payload")
	protoB64 := base64.StdEncoding.EncodeToString(protoPayload)

	fields := []*sppb.StructType_Field{
		{Name: "c_bool", Type: &sppb.Type{Code: sppb.TypeCode_BOOL}},
		{Name: "c_f64", Type: &sppb.Type{Code: sppb.TypeCode_FLOAT64}},
		{Name: "c_f32", Type: &sppb.Type{Code: sppb.TypeCode_FLOAT32}},
		{Name: "c_bytes", Type: &sppb.Type{Code: sppb.TypeCode_BYTES}},
		{Name: "c_bytes_empty", Type: &sppb.Type{Code: sppb.TypeCode_BYTES}},
		{Name: "c_proto", Type: &sppb.Type{Code: sppb.TypeCode_PROTO}},
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
			proto3.NewStringValue(protoB64),
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

	decoded, err := DecodeRows(rawRows, fields)
	if err != nil || len(decoded) != 1 || len(decoded[0]) != len(fields) {
		t.Fatalf("unexpected DecodeRows dimensions: %d x %d err=%v", len(decoded), len(decoded[0]), err)
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
	if row[5].Kind != KindBytes || !bytes.Equal(row[5].BytesVal, protoPayload) {
		t.Fatalf("col 5 (PROTO): %+v", row[5])
	}
	if row[6].Kind != KindTimestamp || row[6].Int64Val != 1782909296 || row[6].Int32Val != 0 {
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

	cells, arena, numRows, numCols, err := PackRowsIntoArena(rawRows, fields)
	if err != nil || numRows != 1 || numCols != len(fields) || len(cells) != len(fields)+4 {
		t.Fatalf("unexpected PackRowsIntoArena dimensions: rows=%d cols=%d len(cells)=%d err=%v", numRows, numCols, len(cells), err)
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
	if cells[5].Kind != int32(KindBytes) || !bytes.Equal(arena[cells[5].Offset:cells[5].Offset+cells[5].Len], protoPayload) {
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

func TestTimestampBoundaries_CalendarBranches_RaggedRows_ManyCols_AndOutlierRow0Cap(t *testing.T) {
	tsType := &sppb.Type{Code: sppb.TypeCode_TIMESTAMP}
	// 1. Spanner TIMESTAMP domain boundaries (0001-01-01T00:00:00Z through 9999-12-31T23:59:59.999999999Z),
	//    Jan/Feb (month <= 2), <9 fractional digits (.123Z), 30-day month (April 30),
	//    leap-year Feb 29 (2024 & 2000), non-leap Feb 28 (2025), and year 0000 negative era branch (y < 0).
	validTSStrings := []string{
		"0001-01-01T00:00:00Z",
		"0001-01-01T00:00:00.123456789Z",
		"0001-03-01T00:00:00Z",
		"9999-12-31T23:59:59Z",
		"9999-12-31T23:59:59.999999999Z",
		"0000-01-01T00:00:00Z",
		"2026-01-15T00:00:00.123Z",
		"2024-02-29T23:59:59.5Z",
		"2000-02-29T00:00:00Z",
		"2025-02-28T12:00:00Z",
		"2026-04-30T06:30:00.000001Z",
	}
	for _, raw := range validTSStrings {
		wantTime, err := time.Parse(time.RFC3339Nano, raw)
		if err != nil {
			t.Fatalf("time.Parse(%q) failed: %v", raw, err)
		}
		wantSec := wantTime.Unix()
		wantNano := int32(wantTime.Nanosecond())

		dv, err := DecodeCellValue(proto3.NewStringValue(raw), tsType)
		if err != nil || dv.Kind != KindTimestamp || dv.Int64Val != wantSec || dv.Int32Val != wantNano {
			t.Fatalf("DecodeCellValue(%q): expected (%d, %d), got %+v err=%v", raw, wantSec, wantNano, dv, err)
		}
		cells, _, _, _, err := PackRowsIntoArena([][]*proto3.Value{{proto3.NewStringValue(raw)}}, []*sppb.StructType_Field{{Name: "ts", Type: tsType}})
		if err != nil || len(cells) != 1 || cells[0].Kind != int32(KindTimestamp) || int64(cells[0].NumBits) != wantSec || int32(cells[0].Flags) != wantNano {
			t.Fatalf("PackRowsIntoArena(%q): expected (%d, %d), got cells=%+v err=%v", raw, wantSec, wantNano, cells, err)
		}
	}

	// Invalid calendar dates (non-leap Feb 29, secular non-leap 1900-02-29, month 13, non-digit frac, bad dot) must return an error.
	for _, invalidTS := range []string{
		"2025-02-29T00:00:00Z",
		"1900-02-29T00:00:00Z",
		"2026-13-01T00:00:00Z",
		"2026-04-31T00:00:00Z",
		"2026-07-01T12:34:56.12aZ",
		"2026-07-01T12:34:56X123Z",
		"2026-07-01T12:34:56.Z",
		"202X-07-01T12:34:56Z",
		"2026-0X-01T12:34:56Z",
	} {
		if _, err := DecodeCellValue(proto3.NewStringValue(invalidTS), tsType); err == nil {
			t.Fatalf("DecodeCellValue(%q): expected error, got nil", invalidTS)
		}
		if _, _, _, _, err := PackRowsIntoArena([][]*proto3.Value{{proto3.NewStringValue(invalidTS)}}, []*sppb.StructType_Field{{Name: "ts", Type: tsType}}); err == nil {
			t.Fatalf("PackRowsIntoArena(%q): expected error, got nil", invalidTS)
		}
	}

	// 2. >32 columns (exercises heap allocation fallback beyond colTypesStack[32]) and ragged rows (including BYTES column beyond shortRow).
	const numManyCols = 35
	manyFields := make([]*sppb.StructType_Field, numManyCols)
	fullRow := make([]*proto3.Value, numManyCols+2) // extra trailing cells (> numCols) truncated
	for i := 0; i < numManyCols; i++ {
		if i == numManyCols-1 {
			manyFields[i] = &sppb.StructType_Field{Name: "b", Type: &sppb.Type{Code: sppb.TypeCode_BYTES}}
			fullRow[i] = proto3.NewStringValue(base64.StdEncoding.EncodeToString([]byte("tail-bytes")))
		} else {
			manyFields[i] = &sppb.StructType_Field{Name: "c", Type: &sppb.Type{Code: sppb.TypeCode_STRING}}
			fullRow[i] = proto3.NewStringValue("v")
		}
	}
	fullRow[numManyCols] = proto3.NewStringValue("ignored-extra-1")
	fullRow[numManyCols+1] = proto3.NewStringValue("ignored-extra-2")
	shortRow := []*proto3.Value{proto3.NewStringValue("only-col-0")} // < numCols padded with KindNull

	raggedRows := [][]*proto3.Value{fullRow, shortRow}
	decRagged, err := DecodeRows(raggedRows, manyFields)
	if err != nil || len(decRagged) != 2 || len(decRagged[0]) != numManyCols || len(decRagged[1]) != numManyCols {
		t.Fatalf("unexpected ragged DecodeRows dimensions: err=%v", err)
	}
	if decRagged[1][0].StrVal != "only-col-0" || decRagged[1][numManyCols-1].Kind != KindNull {
		t.Fatalf("expected short row trailing column to be KindNull, got %+v", decRagged[1][numManyCols-1])
	}
	cellsRagged, _, rCount, cCount, err := PackRowsIntoArena(raggedRows, manyFields)
	if err != nil || rCount != 2 || cCount != numManyCols || len(cellsRagged) != 2*numManyCols {
		t.Fatalf("unexpected ragged PackRowsIntoArena dimensions: r=%d c=%d len=%d err=%v", rCount, cCount, len(cellsRagged), err)
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
	_, outlierArena, _, _, outlierScratch, err := packRowsIntoArenaInternal(outlierRows, outlierFields, true)
	defer releaseArenaScratch(outlierScratch)
	if err != nil || cap(outlierArena) > maxPooledArenaBytes {
		t.Fatalf("expected outlier Row 0 arena capacity to be capped at %d, got %d err=%v", maxPooledArenaBytes, cap(outlierArena), err)
	}
}

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

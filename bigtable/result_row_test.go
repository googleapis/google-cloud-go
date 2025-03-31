/*
Copyright 2025 Google LLC

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
package bigtable

import (
	"math"
	"reflect"
	"testing"
	"time"

	btpb "cloud.google.com/go/bigtable/apiv2/bigtablepb"
	"cloud.google.com/go/civil"
	"cloud.google.com/go/internal/testutil"
	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"
	"google.golang.org/genproto/googleapis/type/date"
	"google.golang.org/protobuf/types/known/timestamppb"
)

func pbBytes(b []byte) *btpb.Value {
	return &btpb.Value{Kind: &btpb.Value_BytesValue{BytesValue: b}}
}
func pbString(s string) *btpb.Value {
	return &btpb.Value{Kind: &btpb.Value_StringValue{StringValue: s}}
}
func pbInt64(i int64) *btpb.Value {
	return &btpb.Value{Kind: &btpb.Value_IntValue{IntValue: i}}
}
func pbFloat32(f float32) *btpb.Value {
	return &btpb.Value{Kind: &btpb.Value_FloatValue{FloatValue: float64(f)}}
}
func pbFloat64(f float64) *btpb.Value {
	return &btpb.Value{Kind: &btpb.Value_FloatValue{FloatValue: f}}
}
func pbBool(b bool) *btpb.Value {
	return &btpb.Value{Kind: &btpb.Value_BoolValue{BoolValue: b}}
}
func pbTimestamp(t time.Time) *btpb.Value {
	return &btpb.Value{Kind: &btpb.Value_TimestampValue{TimestampValue: timestamppb.New(t)}}
}
func pbDate(d civil.Date) *btpb.Value {
	return &btpb.Value{Kind: &btpb.Value_DateValue{DateValue: &date.Date{Year: int32(d.Year), Month: int32(d.Month), Day: int32(d.Day)}}}
}
func pbNull() *btpb.Value {
	return &btpb.Value{Kind: nil} // Explicit Null
}
func pbArray(elements ...*btpb.Value) *btpb.Value {
	return &btpb.Value{Kind: &btpb.Value_ArrayValue{ArrayValue: &btpb.ArrayValue{Values: elements}}}
}
func pbMapEntry(key, val *btpb.Value) *btpb.Value {
	return pbArray(key, val) // Map entry is stored as [key, value] array
}

func createMetadata(cols ...*btpb.ColumnMetadata) *btpb.ResultSetMetadata {
	return &btpb.ResultSetMetadata{
		Schema: &btpb.ResultSetMetadata_ProtoSchema{ProtoSchema: &btpb.ProtoSchema{Columns: cols}},
	}
}
func colMeta(name string, typ *btpb.Type) *btpb.ColumnMetadata {
	return &btpb.ColumnMetadata{Name: name, Type: typ}
}

var (
	typeBytes          = &btpb.Type{Kind: &btpb.Type_BytesType{BytesType: &btpb.Type_Bytes{}}}
	typeString         = &btpb.Type{Kind: &btpb.Type_StringType{StringType: &btpb.Type_String{}}}
	typeInt64          = &btpb.Type{Kind: &btpb.Type_Int64Type{Int64Type: &btpb.Type_Int64{}}}
	typeFloat32        = &btpb.Type{Kind: &btpb.Type_Float32Type{Float32Type: &btpb.Type_Float32{}}}
	typeFloat64        = &btpb.Type{Kind: &btpb.Type_Float64Type{Float64Type: &btpb.Type_Float64{}}}
	typeBool           = &btpb.Type{Kind: &btpb.Type_BoolType{BoolType: &btpb.Type_Bool{}}}
	typeTimestamp      = &btpb.Type{Kind: &btpb.Type_TimestampType{TimestampType: &btpb.Type_Timestamp{}}}
	typeDate           = &btpb.Type{Kind: &btpb.Type_DateType{DateType: &btpb.Type_Date{}}}
	typeArrayString    = &btpb.Type{Kind: &btpb.Type_ArrayType{ArrayType: &btpb.Type_Array{ElementType: typeString}}}
	typeMapStringBytes = &btpb.Type{Kind: &btpb.Type_MapType{MapType: &btpb.Type_Map{KeyType: typeString, ValueType: typeBytes}}}
	typeMapStringInt64 = &btpb.Type{Kind: &btpb.Type_MapType{MapType: &btpb.Type_Map{KeyType: typeString, ValueType: typeInt64}}}
	typeStructSimple   = &btpb.Type{Kind: &btpb.Type_StructType{StructType: &btpb.Type_Struct{Fields: []*btpb.Type_Struct_Field{
		{FieldName: "name", Type: typeString}, {FieldName: "count", Type: typeInt64},
	}}}}
)

var cmpSQLOpts = []cmp.Option{
	cmpopts.EquateApproxTime(time.Millisecond), // For time.Time comparison
	cmp.AllowUnexported(
		BytesSQLType{}, StringSQLType{}, Int64SQLType{}, Float32SQLType{}, Float64SQLType{},
		BoolSQLType{}, TimestampSQLType{}, DateSQLType{}, ArraySQLType{}, MapSQLType{},
		StructSQLType{}, StructSQLField{}, ColumnMetadata{}, ResultRowMetadata{},
		Struct{},
	),
}

func TestNewResultRowMetadata(t *testing.T) {
	tests := []struct {
		name    string
		pbMeta  *btpb.ResultSetMetadata
		wantMd  *ResultRowMetadata
		wantErr bool
	}{
		{
			name: "success",
			pbMeta: createMetadata(
				colMeta("a", typeArrayString),
				colMeta("m", typeMapStringBytes),
				colMeta("s", typeStructSimple),
			),
			wantMd: &ResultRowMetadata{
				Columns: []ColumnMetadata{
					{Name: "a", SQLType: ArraySQLType{ElemType: StringSQLType{}}},
					{Name: "m", SQLType: MapSQLType{KeyType: StringSQLType{}, ValueType: BytesSQLType{}}},
					{Name: "s", SQLType: StructSQLType{Fields: []StructSQLField{
						{Name: "name", Type: StringSQLType{}}, {Name: "count", Type: Int64SQLType{}},
					}}},
				},
				colNameToIndex: &map[string][]int{"a": {0}, "m": {1}, "s": {2}},
			},
		},
		{
			name:    "nil metadata",
			wantErr: true,
		},
		{
			name:    "nil schema",
			pbMeta:  &btpb.ResultSetMetadata{Schema: nil},
			wantErr: true,
		},
		{
			name: "unsupported type",
			pbMeta: createMetadata(
				colMeta("bad", &btpb.Type{Kind: nil}), // Invalid type kind
			),
			wantErr: true,
		},
		{
			name: "nil column type",
			pbMeta: createMetadata(
				colMeta("bad", nil), // Nil type for column
			),
			wantErr: true,
		},
		{
			name: "array with nil element type",
			pbMeta: createMetadata(
				colMeta("badArr", &btpb.Type{Kind: &btpb.Type_ArrayType{ArrayType: &btpb.Type_Array{ElementType: nil}}}),
			),
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Handle panic case for nil metadata gracefully in test
			var gotMd *ResultRowMetadata
			var err error
			gotMd, err = newResultRowMetadata(tt.pbMeta)

			if (err != nil) != tt.wantErr {
				t.Errorf("newResultRowMetadata() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if err != nil {
				return
			}
			if diff := cmp.Diff(tt.wantMd, gotMd, cmpSQLOpts...); diff != "" {
				t.Errorf("newResultRowMetadata() mismatch (-want +got):\n%s", diff)
			}
		})
	}
}

func TestResultRow_GetByIndex(t *testing.T) {
	// Setup
	now := time.Now().UTC().Truncate(time.Microsecond)
	today := civil.DateOf(now)
	testBytes := []byte{1, 2, 3}
	testString := "hello"
	testInt := int64(123)
	testF32 := float32(1.5)
	testF64 := float64(2.5)
	testBool := true
	testArr := []*string{ptr("a"), nil, ptr("b")}                  // Array with nil
	testArrNoNil := []*string{ptr("x"), ptr("y")}                  // Array without nil
	testMap := map[string]*int64{"one": ptr(int64(1)), "two": nil} // Map with nil
	testStruct := Struct{
		fields: []structFieldWithValue{{
			Name:  "name",
			Value: "obj1",
		},
			{
				Name:  "count",
				Value: int64(100),
			},
		},
	} // SQL STRUCT -> bigtable.Struct

	pbMeta := createMetadata(
		colMeta("s", typeString),             // 0
		colMeta("i", typeInt64),              // 1
		colMeta("f32", typeFloat32),          // 2
		colMeta("f64", typeFloat64),          // 3
		colMeta("b", typeBool),               // 4
		colMeta("by", typeBytes),             // 5
		colMeta("ts", typeTimestamp),         // 6
		colMeta("dt", typeDate),              // 7
		colMeta("arr", typeArrayString),      // 8
		colMeta("arrNoNil", typeArrayString), // 9
		colMeta("m", typeMapStringInt64),     // 10
		colMeta("st", typeStructSimple),      // 11
		colMeta("null_i", typeInt64),         // 12
	)
	pbValues := []*btpb.Value{
		pbString(testString), // 0
		pbInt64(testInt),     // 1
		pbFloat32(testF32),   // 2
		pbFloat64(testF64),   // 3
		pbBool(testBool),     // 4
		pbBytes(testBytes),   // 5
		pbTimestamp(now),     // 6
		pbDate(today),        // 7
		pbArray(pbString("a"), pbNull(), pbString("b")),                                         // 8 Array with NULL
		pbArray(pbString("x"), pbString("y")),                                                   // 9 Array with NO NULL
		pbArray(pbMapEntry(pbString("one"), pbInt64(1)), pbMapEntry(pbString("two"), pbNull())), // 10 Map with NULL value
		pbArray(pbString("obj1"), pbInt64(100)),                                                 // 11 Struct
		pbNull(),                                                                                // 12 Int64 NULL
	}

	rrMeta, _ := newResultRowMetadata(pbMeta)
	row, err := newResultRow(pbValues, pbMeta, rrMeta)
	if err != nil {
		t.Fatalf("newResultRow failed: %v", err)
	}

	type CustomStruct struct{ Field string } // For invalid struct pointer test

	for _, tc := range []struct {
		name      string
		index     int
		destFn    func() any // Returns pointer like &var
		wantValue any
		wantErr   bool
	}{
		// Valid Gets
		{"string", 0, func() any { var v string; return &v }, testString, false},
		{"int64", 1, func() any { var v int64; return &v }, testInt, false},
		{"float32", 2, func() any { var v float32; return &v }, testF32, false},
		{"float64", 3, func() any { var v float64; return &v }, testF64, false},
		{"bool", 4, func() any { var v bool; return &v }, testBool, false},
		{"bytes", 5, func() any { var v []byte; return &v }, testBytes, false},
		{"Time", 6, func() any { var v time.Time; return &v }, now, false},
		{"Date", 7, func() any { var v civil.Date; return &v }, today, false},
		{"Time Ptr", 6, func() any { var v *time.Time; return &v }, &now, false},                                      // T -> *T
		{"Date Ptr", 7, func() any { var v *civil.Date; return &v }, &today, false},                                   // T -> *T
		{"array with nil", 8, func() any { var v []*string; return &v }, testArr, false},                              // []*T -> []*T
		{"array with no nils to pointer slice", 9, func() any { var v []*string; return &v }, testArrNoNil, false},    // []*T -> []*T
		{"array with no nils to value slice", 9, func() any { var v []string; return &v }, []string{"x", "y"}, false}, // []*T -> []T
		{"map with nil", 10, func() any { var v map[string]*int64; return &v }, testMap, false},                       // map[K]*V -> map[K]*V
		{"struct into Struct", 11, func() any { var v Struct; return &v }, testStruct, false},                         // map[string]any -> map[string]any
		{"null into int pointer", 12, func() any { var v *int64; return &v }, (*int64)(nil), false},                   // nil -> *T
		{"null to any", 12, func() any { var v any; return &v }, (any)(nil), false},                                   // nil -> any

		// Error Cases
		{"int64 to int", 1, func() any { var v int; return &v }, nil, true}, // Conversion T->T
		{"index negative", -1, func() any { var v any; return &v }, nil, true},
		{"index too large", 13, func() any { var v any; return &v }, nil, true},
		{"ErrNilDest", 0, func() any {
			return nil
		}, nil, true},
		{"nil into non-nillable", 12, func() any { var v int64; return &v }, nil, true},         // nil -> int64 fails
		{"array with nil to val slice", 8, func() any { var v []string; return &v }, nil, true}, // []*T (with nil) -> []T fails
		{"destination not a pointer", 0, func() any { var v int64; return v }, nil, true},
		{"destination is nil pointer", 0, func() any { var v *int64; return v }, nil, true},
		{"destination pointer to struct", 0, func() any { var v CustomStruct; return &v }, nil, true},
		{"struct into map", 11, func() any { var v map[string]any; return &v }, nil, true}, // STRUCT -> map[string]any
	} {
		t.Run(tc.name, func(t *testing.T) {
			destPtr := tc.destFn()
			err := row.GetByIndex(tc.index, destPtr)

			if (err != nil) != tc.wantErr {
				t.Errorf("GetByIndex: index: %v, error got: %v, want: %v", tc.index, err, tc.wantErr)
			}
			if err != nil || tc.wantErr {
				return
			}

			gotValue := reflect.ValueOf(destPtr).Elem().Interface()
			if diff := testutil.Diff(tc.wantValue, gotValue, cmpSQLOpts...); diff != "" {
				t.Errorf("GetByIndex(%d) value mismatch (-want +got):\n%s", tc.index, diff)
			}
		})
	}
}

func TestResultRow_GetByName(t *testing.T) {
	// Setup
	pbMeta := createMetadata(
		colMeta("id", typeInt64), colMeta("name", typeString), colMeta("value", typeFloat64),
		colMeta("name", typeBytes), // Duplicate
	)
	pbValues := []*btpb.Value{pbInt64(1), pbString("first"), pbFloat64(10.1), pbBytes([]byte("second"))}
	rrMeta, _ := newResultRowMetadata(pbMeta)
	row, err := newResultRow(pbValues, pbMeta, rrMeta)
	if err != nil {
		t.Fatalf("newResultRow: %v", err)
	}

	tests := []struct {
		name      string
		colName   string
		destFn    func() any // Returns pointer like &var
		wantErr   bool
		wantValue any
	}{
		{"found unique name", "id", func() any { var v int64; return &v }, false, int64(1)},
		{"name not found", "address", func() any { var v any; return &v }, true, nil},
		{"empty name", "", func() any { var v any; return &v }, true, nil},
		{"case sensitive miss", "ID", func() any { var v any; return &v }, true, nil},
		{"found duplicate name", "name", func() any { var v string; return &v }, true, nil}, // GetByName requires unique name
		{"nil destination", "id", func() any { return nil }, true, nil},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			destPtr := tt.destFn()
			err := row.GetByName(tt.colName, destPtr)

			if (err != nil) != tt.wantErr {
				t.Errorf("GetByName(%q) error = %v, wantErr %v", tt.colName, err, tt.wantErr)
			}
			if err != nil || tt.wantErr {
				return
			}

			gotValue := reflect.ValueOf(destPtr).Elem().Interface()
			if diff := cmp.Diff(tt.wantValue, gotValue, cmpSQLOpts...); diff != "" {
				t.Errorf("GetByName(%q) value mismatch (-want +got):\n%s", tt.colName, diff)
			}
		})
	}
}

func TestAssignValue(t *testing.T) {
	int64Ptr1 := ptr(int64(1))
	int64Ptr2 := ptr(int64(2))
	int64Ptr3 := ptr(int64(3))
	int64Ptr10 := ptr(int64(10))
	tests := []struct {
		name    string
		src     any
		destFn  func() any // returns pointer to destination variable
		wantErr bool
		wantVal any // expected value IN destination variable
	}{
		// Nil handling
		{"nil to ptr", nil, func() any { var v *int64; return &v }, false, (*int64)(nil)},
		{"nil to interface", nil, func() any { var v any; return &v }, false, (any)(nil)},
		{"nil to slice", nil, func() any { var v []int; return &v }, false, ([]int)(nil)},
		{"nil to map", nil, func() any { var v map[int]int; return &v }, false, (map[int]int)(nil)},
		{"nil to non-nillable", nil, func() any { var v int64; return &v }, true, int64(0)},

		// Direct Assign
		{"int to int", int64(10), func() any { var v int64; return &v }, false, int64(10)},
		{"string to string", "a", func() any { var v string; return &v }, false, "a"},
		{"bytes to bytes", []byte("a"), func() any { var v []byte; return &v }, false, []byte("a")},
		{"map to map", map[string]int{"a": 1}, func() any { var v map[string]int; return &v }, false, map[string]int{"a": 1}},

		// Pointer Assigns
		{"int to ptr int", int64(10), func() any { var v *int64; return &v }, false, int64Ptr10},
		{"ptr int to int", int64Ptr10, func() any { var v int64; return &v }, false, int64(10)},
		{"ptr int to ptr int", int64Ptr10, func() any { var v *int64; return &v }, false, int64Ptr10},

		// Scalar Conversions errors
		{"int64 to int", int64(10), func() any { var v int; return &v }, true, int(10)},
		{"int to int64", int(10), func() any { var v int64; return &v }, true, int64(10)},
		{"float64 to float32", float64(math.MaxFloat64), func() any { var v float32; return &v }, true, float32(math.Inf(0))},
		{"float32 to float64", float32(1.5), func() any { var v float64; return &v }, true, float64(1.5)},
		{"bytes to string", []byte("abc"), func() any { var v string; return &v }, true, "abc"},
		{"string to bytes", "abc", func() any { var v []byte; return &v }, true, []byte("abc")},

		// scalar direct assignments
		{"int64 to int64", int64(10), func() any { var v int64; return &v }, false, int64(10)},
		{"int to int", int(10), func() any { var v int; return &v }, false, int(10)},
		{"float64 to float64", float64(math.MaxFloat64), func() any { var v float64; return &v }, false, float64(math.MaxFloat64)},
		{"float32 to float32", float32(1.5), func() any { var v float32; return &v }, false, float32(1.5)},
		{"bytes to bytes", []byte("abc"), func() any { var v []byte; return &v }, false, []byte("abc")},
		{"string to string", "abc", func() any { var v string; return &v }, false, "abc"},

		//  slice Conversions
		{"ptr int slice to int slice ok", []*int64{int64Ptr1, int64Ptr2}, func() any { var v []int64; return &v }, false, []int64{1, 2}},
		{"ptr int slice to int slice fail", []*int64{int64Ptr1, nil}, func() any { var v []int64; return &v }, true, nil},
		{"int slice to ptr int slice", []int64{1, 2}, func() any { var v []*int64; return &v }, false, []*int64{int64Ptr1, int64Ptr2}},
		{"ptr int slice to any slice", []*int64{int64Ptr1, nil}, func() any { var v []any; return &v }, false, []any{int64(1), nil}},
		{"int slice to any slice", []int64{1, 2}, func() any { var v []any; return &v }, false, []any{int64(1), int64(2)}},
		{"any slice to int slice ok", []any{int64(1), int64(2)}, func() any { var v []int64; return &v }, false, []int64{1, 2}},
		{"any slice to int slice fail type", []any{int64(1), "2"}, func() any { var v []int64; return &v }, true, nil},
		{"any slice to int slice fail nil", []any{int64(1), nil}, func() any { var v []int64; return &v }, true, nil},
		{"any slice to ptr int slice ok", []any{int64(1), nil, int64(3)}, func() any { var v []*int64; return &v }, false, []*int64{int64Ptr1, nil, int64Ptr3}},
		{"any slice to ptr int slice fail type", []any{int64(1), "nil"}, func() any { var v []*int64; return &v }, true, nil},

		// map Conversions
		{"map ptr val to map val ok", map[string]*int64{"a": int64Ptr1}, func() any { var v map[string]int64; return &v }, false, map[string]int64{"a": 1}},
		{"map ptr val to map val fail nil", map[string]*int64{"a": nil}, func() any { var v map[string]int64; return &v }, true, nil},
		{"map val to map ptr val", map[string]int64{"a": 1}, func() any { var v map[string]*int64; return &v }, false, map[string]*int64{"a": int64Ptr1}},
		{"map to map stringany", map[string]*int64{"a": int64Ptr1, "b": nil}, func() any { var v map[string]any; return &v }, false, map[string]any{"a": int64Ptr1, "b": (*int64)(nil)}},

		// Unsupported
		{"Unsupported string to Time", "nottime", func() any { var v time.Time; return &v }, true, time.Time{}},
		{"Unsupported string to int", "10", func() any { var v int64; return &v }, true, int64(10)},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			destPtr := tt.destFn()
			destVal := reflect.ValueOf(destPtr).Elem() // Get the value pointed to

			err := assignValue(destVal, tt.src)

			if (err != nil) != tt.wantErr {
				t.Errorf("assignValue() error = %v, wantErr %v", err, tt.wantErr)
			}
			if err != nil || tt.wantErr {
				return
			}

			gotValue := destVal.Interface() // Read the value back from the destination
			if diff := cmp.Diff(tt.wantVal, gotValue, cmpSQLOpts...); diff != "" {
				t.Errorf("assignValue() value mismatch (-want +got):\n%s", diff)
				t.Logf("Source type: %T", tt.src)
				t.Logf("Dest type: %T", tt.wantVal) // Use wantVal type for logging expected dest type
				t.Logf("Got type: %T", gotValue)
			}
		})
	}
}

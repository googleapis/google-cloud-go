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
	"encoding/base64"
	"math"
	"reflect"
	"strings"
	"testing"
	"time"

	btpb "cloud.google.com/go/bigtable/apiv2/bigtablepb"
	"cloud.google.com/go/civil"
	"cloud.google.com/go/internal/testutil"
	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"
	"google.golang.org/genproto/googleapis/type/date"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
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
	typeMapHistoryStruct = &btpb.Type{
		Kind: &btpb.Type_MapType{
			MapType: &btpb.Type_Map{
				KeyType: typeBytes,
				ValueType: &btpb.Type{
					Kind: &btpb.Type_ArrayType{
						ArrayType: &btpb.Type_Array{
							ElementType: &btpb.Type{
								Kind: &btpb.Type_StructType{
									StructType: &btpb.Type_Struct{
										Fields: []*btpb.Type_Struct_Field{
											{FieldName: "timestamp", Type: typeTimestamp}, {FieldName: "value", Type: typeBytes},
										}}}}}}}}}}
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
		{
			name:    "empty columns",
			pbMeta:  createMetadata(),
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
				t.Errorf("newResultRowMetadata() error: got: %v, want: %v", err, tt.wantErr)
				return
			}
			if err != nil {
				if status.Code(err) != codes.Internal {
					t.Errorf("newResultRowMetadata() status.Code(err) = %v, want %v (err: %v)", status.Code(err), codes.Internal, err)
				}
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
	nowMinus10s := now.Add(-10 * time.Second)
	nowMinus20s := now.Add(-20 * time.Second)

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
		fields: []structFieldWithValue{
			{
				Name:  "name",
				Value: "obj1",
			},
			{
				Name:  "count",
				Value: int64(100),
			},
		},
		nameToIndex: map[string][]int{"name": {0}, "count": {1}},
	} // SQL STRUCT -> bigtable.Struct
	// History Map Data
	histKey1Bytes := []byte("city")
	histKey1Base64 := base64.StdEncoding.EncodeToString(histKey1Bytes)
	histKey2Bytes := []byte("state")
	histKey2Base64 := base64.StdEncoding.EncodeToString(histKey2Bytes)
	histValCity1 := []byte("Seattle")
	histValCity2 := []byte("Redmond")
	histValState1 := []byte("WA")

	// Expected Go Value for the history map
	wantHistoryMap := map[string][]Struct{ // Go map uses base64 string key, value is []Struct
		histKey1Base64: { // Base64("city")
			newStruct([]structFieldWithValue{ // Older cell as a Struct
				{Name: "timestamp", Value: nowMinus10s},
				{Name: "value", Value: histValCity1},
			}),
			newStruct([]structFieldWithValue{ // Newer cell as a Struct
				{Name: "timestamp", Value: now},
				{Name: "value", Value: histValCity2},
			}),
		},
		histKey2Base64: { // Base64("state")
			newStruct([]structFieldWithValue{
				{Name: "timestamp", Value: nowMinus20s}, // Different timestamp
				{Name: "value", Value: histValState1},
			}),
		},
	}

	// Corresponding Protobuf Value for the history map
	pbHistoryMapValue := pbArray( // Map is array of entries
		pbMapEntry( // Entry 1: city
			pbBytes(histKey1Bytes), // Key
			pbArray( // Value is Array<Struct>
				pbArray(pbTimestamp(nowMinus10s), pbBytes(histValCity1)), // Struct 1 (ts, val)
				pbArray(pbTimestamp(now), pbBytes(histValCity2)),         // Struct 2 (ts, val)
			),
		),
		pbMapEntry( // Entry 2: state
			pbBytes(histKey2Bytes), // Key
			pbArray( // Value is Array<Struct>
				pbArray(pbTimestamp(nowMinus20s), pbBytes(histValState1)), // Struct 3 (ts, val)
			),
		),
	)
	pbMeta := createMetadata(
		colMeta("s", typeString),                     // 0
		colMeta("i", typeInt64),                      // 1
		colMeta("f32", typeFloat32),                  // 2
		colMeta("f64", typeFloat64),                  // 3
		colMeta("b", typeBool),                       // 4
		colMeta("by", typeBytes),                     // 5
		colMeta("ts", typeTimestamp),                 // 6
		colMeta("dt", typeDate),                      // 7
		colMeta("arr", typeArrayString),              // 8
		colMeta("arrNoNil", typeArrayString),         // 9
		colMeta("m", typeMapStringInt64),             // 10
		colMeta("st", typeStructSimple),              // 11
		colMeta("null_i", typeInt64),                 // 12
		colMeta("history_map", typeMapHistoryStruct), // 13
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
		pbHistoryMapValue,                                                                       // 13
	}

	rrMeta, _ := newResultRowMetadata(pbMeta)
	row, err := newResultRow(pbValues, pbMeta, rrMeta)
	if err != nil {
		t.Fatalf("newResultRow failed: %v", err)
	}

	type CustomStruct struct{ Field string } // For invalid struct pointer test

	for _, tt := range []struct {
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
		{
			"get history map",
			13, // Index of the new column
			func() any { var v map[string][]Struct; return &v }, // Correct dest type
			wantHistoryMap, // Expected Go map value with base64 keys
			false,
		},

		// Error Cases
		{"int64 to int", 1, func() any { var v int; return &v }, nil, true}, // Conversion T->T
		{"index negative", -1, func() any { var v any; return &v }, nil, true},
		{"index too large", 20, func() any { var v any; return &v }, nil, true},
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
		t.Run(tt.name, func(t *testing.T) {
			destPtr := tt.destFn()
			err := row.GetByIndex(tt.index, destPtr)

			if (err != nil) != tt.wantErr {
				t.Errorf("GetByIndex: index: %v, error got: %v, want: %v", tt.index, err, tt.wantErr)
			}
			if err != nil || tt.wantErr {
				return
			}

			gotValue := reflect.ValueOf(destPtr).Elem().Interface()
			if diff := testutil.Diff(tt.wantValue, gotValue, cmpSQLOpts...); diff != "" {
				t.Errorf("GetByIndex(%d) value mismatch (-want +got):\n%s", tt.index, diff)
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
				t.Errorf("GetByName(%q) error: got: %v, want: %v", tt.colName, err, tt.wantErr)
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
	testStructVal := newStruct([]structFieldWithValue{
		{Name: "fieldA", Value: "abc"},
		{Name: "fieldB", Value: ptr(int64(123))},
		{Name: "fieldC", Value: nil},
	})
	field1Val := "hello nested"
	field2Val := []map[string][]*int64{
		{"map1key1": {int64Ptr1, nil, int64Ptr3}}, // Map with slice containing nil
		nil, // nil map element
		{},  // empty map element
		{"map2key1": {int64Ptr2}},
	}
	field3Val := map[string]map[string]any{ // Actual Go type for MAP<STRING, MAP<STRING, ANY>>
		"outerA": {"innerA1": "a string", "innerA2": int64Ptr10},
		"outerB": nil,
		"outerC": {},
	}
	sourceComplexStructVal := newStruct([]structFieldWithValue{
		{Name: "field1", Value: field1Val},
		{Name: "field2", Value: field2Val},
		{Name: "field3", Value: field3Val},
	})
	expectedComplexStructVal := sourceComplexStructVal

	for _, tt := range []struct {
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
		{"map to map string any", map[string]*int64{"a": int64Ptr1, "b": nil}, func() any { var v map[string]any; return &v }, false, map[string]any{"a": int64(1), "b": nil}},

		// struct
		{
			name:    "struct to struct",
			src:     testStructVal,
			destFn:  func() any { var v Struct; return &v }, // Dest is *Struct
			wantErr: false,
			wantVal: testStructVal, // Should be directly assignable
		},
		{
			name: "success assign specific complex struct to struct gets fields overwritten",
			src:  sourceComplexStructVal,
			destFn: func() any {
				field1Val := "hello nested"
				field2Val := []map[string][]int64{} // int64 as compared to *int64 in src
				field3Val := map[string]map[string][][][][][]*int64{}
				destComplexStructVal := newStruct([]structFieldWithValue{
					{Name: "field1", Value: field1Val},
					{Name: "field2", Value: field2Val},
					{Name: "field3", Value: field3Val},
				})

				return &destComplexStructVal
			}, // Dest is *Struct
			wantErr: false,
			wantVal: expectedComplexStructVal, // Expect identical struct value
		},
		{
			name:    "struct to any",
			src:     testStructVal,
			destFn:  func() any { var v any; return &v }, // Dest is *any
			wantErr: false,
			wantVal: testStructVal, // Interface will hold the Struct value
		},
		{
			name:    "error struct to map",
			src:     testStructVal,
			destFn:  func() any { var v map[string]any; return &v }, // Dest is *map[string]any
			wantErr: true,                                           // assignValue should prevent this
			wantVal: nil,
		},

		// Unsupported
		{"Unsupported string to Time", "nottime", func() any { var v time.Time; return &v }, true, time.Time{}},
		{"Unsupported string to int", "10", func() any { var v int64; return &v }, true, int64(10)},
	} {
		t.Run(tt.name, func(t *testing.T) {
			destPtr := tt.destFn()
			destVal := reflect.ValueOf(destPtr).Elem() // Get the value pointed to

			err := assignValue(destVal, tt.src)

			if (err != nil) != tt.wantErr {
				t.Errorf("assignValue() error: got: %v, want: %v", err, tt.wantErr)
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

func TestStruct_GetByIndex(t *testing.T) {
	// Setup
	now := time.Now().UTC().Truncate(time.Microsecond)
	today := civil.DateOf(now)
	nestedStruct := newStruct([]structFieldWithValue{{Name: "inner", Value: int64(1)}})

	// Internal values representing data after pbValueToGoValue
	testStruct := newStruct([]structFieldWithValue{
		{Name: "str_field", Value: "value1"},                                    // 0: string
		{Name: "int_field", Value: int64(123)},                                  // 1: int64
		{Name: "nil_field", Value: nil},                                         // 2: nil
		{Name: "ptr_field_sim", Value: "value2"},                                // 3: string (simulating base type T)
		{Name: "slice_field", Value: []*int64{ptr(int64(1)), nil}},              // 4: []*int64
		{Name: "time_field", Value: now},                                        // 5: time.Time
		{Name: "date_field", Value: today},                                      // 6: civil.Date
		{Name: "struct_field", Value: nestedStruct},                             // 7: Struct
		{Name: "", Value: bool(true)},                                           // 8: bool (unnamed)
		{Name: "slice_no_nil", Value: []*int64{ptr(int64(10)), ptr(int64(20))}}, // 9: []*int64 (no nils)
	})

	type CustomStruct struct{ Field string } // For testing invalid dest

	for _, tt := range []struct {
		name      string
		index     int
		destFn    func() any // Returns pointer like &var
		wantValue any        // Expected value IN the dest variable after call
		wantErr   bool
	}{
		// Valid Gets
		{"get string", 0, func() any { var v string; return &v }, "value1", false},
		{"get int64", 1, func() any { var v int64; return &v }, int64(123), false},
		{"get nil to string pointer", 2, func() any { var v *string; return &v }, (*string)(nil), false},
		{"get nil to any", 2, func() any { var v any; return &v }, (any)(nil), false},
		{"get string to pointer string", 3, func() any { var v *string; return &v }, ptr("value2"), false},                       // T -> *T assignment
		{"get pointer slice to pointer slice", 4, func() any { var v []*int64; return &v }, []*int64{ptr(int64(1)), nil}, false}, // []*T -> []*T
		{"get pointer slice to any slice", 4, func() any { var v []any; return &v }, []any{int64(1), nil}, false},                // []*T -> []any
		{"get time", 5, func() any { var v time.Time; return &v }, now, false},
		{"get date", 6, func() any { var v civil.Date; return &v }, today, false},
		{"get time pointer", 5, func() any { var v *time.Time; return &v }, &now, false},    // T -> *T
		{"get date pointer", 6, func() any { var v *civil.Date; return &v }, &today, false}, // T -> *T
		{"get struct", 7, func() any { var v Struct; return &v }, nestedStruct, false},
		{"get unnamed bool", 8, func() any { var v bool; return &v }, true, false},
		{"get pointer slice no nil to pointer slice", 9, func() any { var v []*int64; return &v }, []*int64{ptr(int64(10)), ptr(int64(20))}, false}, // []*T -> []*T
		{"get pointer slice no nil to value slice", 9, func() any { var v []int64; return &v }, []int64{10, 20}, false},                             // []*T -> []T

		// Error Cases
		{"error index negative", -1, func() any { var v any; return &v }, nil, true},
		{"error index too large", 10, func() any { var v any; return &v }, nil, true}, // Adjusted index
		{"error nil destination", 0, func() any { return nil }, nil, true},
		{"error non pointer destination", 0, func() any { var v string; return v }, nil, true},
		{"error nil pointer destination", 0, func() any { var v *string; return v }, nil, true},
		{"error assign nil to non nillable", 2, func() any { var v string; return &v }, nil, true},                    // nil -> string fails
		{"error assign wrong type", 0, func() any { var v int64; return &v }, nil, true},                              // string -> int64 fails
		{"error assign pointer slice with nil to value slice", 4, func() any { var v []int64; return &v }, nil, true}, // []*T (with nil) -> []T fails
		{"error assign struct to map", 7, func() any { var v map[string]any; return &v }, nil, true},                  // Struct -> map fails
		{"error assign struct to custom struct", 7, func() any { var v CustomStruct; return &v }, nil, true},          // Struct -> CustomStruct fails
	} {
		t.Run(tt.name, func(t *testing.T) {
			destPtr := tt.destFn()
			err := testStruct.GetByIndex(tt.index, destPtr) // Use GetByIndex

			if (err != nil) != tt.wantErr {
				t.Errorf("Struct.GetByIndex(%d) error: got: %v, want: %v", tt.index, err, tt.wantErr)
			}
			if err != nil || tt.wantErr {
				return
			}

			gotValue := reflect.ValueOf(destPtr).Elem().Interface()
			var diff string
			if diff = testutil.Diff(tt.wantValue, gotValue, cmpSQLOpts...); diff != "" {
				t.Errorf("Struct.GetByIndex(%d) value mismatch (-want +got):\n%s", tt.index, diff)
			}
		})
	}
}

func TestStruct_GetByName(t *testing.T) {
	// Setup struct with duplicate and unnamed fields
	testStruct := newStruct([]structFieldWithValue{
		{Name: "id", Value: int64(101)},       // 0
		{Name: "name", Value: "alpha"},        // 1
		{Name: "value", Value: float64(1.23)}, // 2 (Simulate T value)
		{Name: "name", Value: "beta"},         // 3 (Duplicate name)
		{Name: "", Value: int64(999)},         // 4 (Unnamed)
		{Name: "status", Value: nil},          // 5 (Nil value)
	})

	for _, tt := range []struct {
		name      string
		fieldName string
		destFn    func() any
		wantValue any
		wantErr   bool
	}{
		// Valid Gets
		{"found unique name", "id", func() any { var v int64; return &v }, int64(101), false},
		{"found value non pointer", "value", func() any { var v float64; return &v }, float64(1.23), false},
		{"found value pointer", "value", func() any { var v *float64; return &v }, ptr(float64(1.23)), false}, // T -> *T
		{"found nil", "status", func() any { var v *string; return &v }, (*string)(nil), false},

		// Error Cases
		{"not found", "address", func() any { var v string; return &v }, nil, true},
		{"empty name", "", func() any { var v int64; return &v }, nil, true},
		{"case sensitive miss", "ID", func() any { var v any; return &v }, nil, true},
		{"error duplicate name", "name", func() any { var v string; return &v }, nil, true}, // Error on duplicate
		{"error nil destination", "id", func() any { return nil }, nil, true},
		{"error non pointer destination", "id", func() any { var v int64; return v }, nil, true},
		{"error nil pointer destination", "id", func() any { var v *int64; return v }, nil, true},
	} {
		t.Run(tt.name, func(t *testing.T) {
			destPtr := tt.destFn()
			err := testStruct.GetByName(tt.fieldName, destPtr)

			if (err != nil) != tt.wantErr {
				t.Errorf("Struct.GetByName(%q) error got: %v, want: %v", tt.fieldName, err, tt.wantErr)
			}
			// Check specific error messages if needed
			if tt.name == "error duplicate name" && err != nil && !strings.Contains(err.Error(), "expected only one") {
				t.Errorf("Struct.GetByName(%q) got: %v, want: duplicate name error", tt.fieldName, err)
			}
			if tt.name == "not found" && err != nil && !strings.Contains(err.Error(), "not found") {
				t.Errorf("Struct.GetByName(%q) got: %v, want: not found error", tt.fieldName, err)
			}

			if err != nil || tt.wantErr {
				return
			}

			gotValue := reflect.ValueOf(destPtr).Elem().Interface()
			var diff string
			if diff = testutil.Diff(tt.wantValue, gotValue, cmpSQLOpts...); diff != "" {
				t.Errorf("Struct.GetByName(%q) value mismatch (-want +got):\n%s", tt.fieldName, diff)
			}
		})
	}
}

func TestResultRow_GetByIndex_StructWithUnnamedFields(t *testing.T) {
	typeUnnamedStruct := &btpb.Type{
		Kind: &btpb.Type_StructType{
			StructType: &btpb.Type_Struct{
				Fields: []*btpb.Type_Struct_Field{
					{FieldName: "", Type: typeString},
					{FieldName: "", Type: typeInt64},
				},
			},
		},
	}
	pbMeta := createMetadata(colMeta("struct", typeUnnamedStruct))
	pbValues := []*btpb.Value{
		pbArray(pbString("foo"), pbInt64(100)),
	}
	rrMeta, err := newResultRowMetadata(pbMeta)
	if err != nil {
		t.Fatalf("newResultRowMetadata: %v", err)
	}
	row, err := newResultRow(pbValues, pbMeta, rrMeta)
	if err != nil {
		t.Fatalf("newResultRow: %v", err)
	}

	var got Struct
	if err := row.GetByIndex(0, &got); err != nil {
		t.Fatalf("GetByIndex(0) error: %v", err)
	}
	var f0 string
	if err := got.GetByIndex(0, &f0); err != nil || f0 != "foo" {
		t.Fatalf("Struct.GetByIndex(0) = %q, %v; want %q, nil", f0, err, "foo")
	}
	var f1 int64
	if err := got.GetByIndex(1, &f1); err != nil || f1 != 100 {
		t.Fatalf("Struct.GetByIndex(1) = %d, %v; want %d, nil", f1, err, 100)
	}
}

func TestResultRow_GetByIndex_MapWithNullKey(t *testing.T) {
	typeMapBytesString := &btpb.Type{
		Kind: &btpb.Type_MapType{
			MapType: &btpb.Type_Map{
				KeyType:   typeBytes,
				ValueType: typeString,
			},
		},
	}
	pbMeta := createMetadata(colMeta("mapCol", typeMapBytesString))
	pbValues := []*btpb.Value{
		pbArray(
			pbMapEntry(pbNull(), pbString("foo")),
			pbMapEntry(pbBytes([]byte("bar")), pbNull()),
		),
	}
	rrMeta, err := newResultRowMetadata(pbMeta)
	if err != nil {
		t.Fatalf("newResultRowMetadata: %v", err)
	}
	row, err := newResultRow(pbValues, pbMeta, rrMeta)
	if err != nil {
		t.Fatalf("newResultRow: %v", err)
	}

	barBase64 := base64.StdEncoding.EncodeToString([]byte("bar"))
	wantMap := map[any]*string{
		nil:       ptr("foo"),
		barBase64: nil,
	}

	var gotAny any
	if err := row.GetByIndex(0, &gotAny); err != nil {
		t.Fatalf("GetByIndex(0, &gotAny) error: %v", err)
	}
	if diff := testutil.Diff(wantMap, gotAny); diff != "" {
		t.Errorf("GetByIndex(0, &gotAny) mismatch (-want +got):\n%s", diff)
	}

	var gotMapAny map[any]*string
	if err := row.GetByIndex(0, &gotMapAny); err != nil {
		t.Fatalf("GetByIndex(0, &gotMapAny) error: %v", err)
	}
	if diff := testutil.Diff(wantMap, gotMapAny); diff != "" {
		t.Errorf("GetByIndex(0, &gotMapAny) mismatch (-want +got):\n%s", diff)
	}

	var gotMapString map[string]*string
	if err := row.GetByIndex(0, &gotMapString); err == nil {
		t.Fatalf("GetByIndex(0, &gotMapString) succeeded, want error for nil map key")
	}
}

func TestResultRow_GetByIndex_ValidationErrorsReturnInternal(t *testing.T) {
	pbMeta := createMetadata(
		colMeta("strCol", typeString),
		colMeta("mapCol", typeMapStringInt64),
		colMeta("arrCol", typeArrayString),
		colMeta("structCol", typeStructSimple),
		colMeta("shortStructCol", typeStructSimple),
	)
	pbValues := []*btpb.Value{
		pbInt64(42), // type mismatch: int for string
		pbArray(pbMapEntry(pbString("k"), pbString("wrong"))), // type mismatch within map value
		pbArray(pbString("ok"), pbInt64(1)),                   // type mismatch within array
		pbArray(pbString("ok"), pbString("wrong")),            // type mismatch within struct
		pbArray(pbString("only_one_field")),                   // struct missing field
	}
	rrMeta, err := newResultRowMetadata(pbMeta)
	if err != nil {
		t.Fatalf("newResultRowMetadata: %v", err)
	}
	row, err := newResultRow(pbValues, pbMeta, rrMeta)
	if err != nil {
		t.Fatalf("newResultRow: %v", err)
	}

	for _, tc := range []struct {
		name    string
		index   int
		wantSub string
	}{
		{"scalar type mismatch", 0, "type mismatch: expected StringValue for StringType"},
		{"map value type mismatch", 1, "type mismatch: expected IntValue for Int64Type"},
		{"array element type mismatch", 2, "type mismatch: expected StringValue for StringType"},
		{"struct field type mismatch", 3, "type mismatch: expected IntValue for Int64Type"},
		{"struct missing field", 4, "struct data/schema mismatch"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var v any
			err := row.GetByIndex(tc.index, &v)
			if err == nil {
				t.Fatalf("GetByIndex(%d) succeeded, want error", tc.index)
			}
			if status.Code(err) != codes.Internal {
				t.Errorf("GetByIndex(%d) status.Code(err) = %v, want %v (err: %v)", tc.index, status.Code(err), codes.Internal, err)
			}
			if !strings.Contains(err.Error(), tc.wantSub) {
				t.Errorf("GetByIndex(%d) err = %q, want substring %q", tc.index, err.Error(), tc.wantSub)
			}
		})
	}
}

func TestResultRow_MapDestinationReuseAndPointerUnwrapping(t *testing.T) {
	pbMeta := createMetadata(colMeta("m", typeMapStringInt64))
	rrMeta, err := newResultRowMetadata(pbMeta)
	if err != nil {
		t.Fatalf("newResultRowMetadata: %v", err)
	}

	row1, err := newResultRow([]*btpb.Value{
		pbArray(pbMapEntry(pbString("row1_only"), pbInt64(10)), pbMapEntry(pbString("shared"), pbInt64(20))),
	}, pbMeta, rrMeta)
	if err != nil {
		t.Fatalf("newResultRow(row1): %v", err)
	}
	row2, err := newResultRow([]*btpb.Value{
		pbArray(pbMapEntry(pbString("shared"), pbInt64(30)), pbMapEntry(pbString("row2_only"), pbInt64(40))),
	}, pbMeta, rrMeta)
	if err != nil {
		t.Fatalf("newResultRow(row2): %v", err)
	}
	rowWithNull, err := newResultRow([]*btpb.Value{
		pbArray(pbMapEntry(pbString("ok"), pbInt64(50)), pbMapEntry(pbString("nullVal"), pbNull())),
	}, pbMeta, rrMeta)
	if err != nil {
		t.Fatalf("newResultRow(rowWithNull): %v", err)
	}

	t.Run("reused non-nil map[string]int64 drops stale keys and does not alias prior rows", func(t *testing.T) {
		dest := map[string]int64{"stale": 999}
		if err := row1.GetByIndex(0, &dest); err != nil {
			t.Fatalf("row1.GetByIndex: %v", err)
		}
		row1Snapshot := dest
		wantRow1 := map[string]int64{"row1_only": 10, "shared": 20}
		if diff := cmp.Diff(wantRow1, dest); diff != "" {
			t.Fatalf("row1 mismatch (-want +got):\n%s", diff)
		}

		if err := row2.GetByIndex(0, &dest); err != nil {
			t.Fatalf("row2.GetByIndex: %v", err)
		}
		wantRow2 := map[string]int64{"shared": 30, "row2_only": 40}
		if diff := cmp.Diff(wantRow2, dest); diff != "" {
			t.Fatalf("row2 mismatch (-want +got):\n%s", diff)
		}
		if diff := cmp.Diff(wantRow1, row1Snapshot); diff != "" {
			t.Fatalf("row1 snapshot mutated by row2 scan (-want +got):\n%s", diff)
		}

		if err := rowWithNull.GetByIndex(0, &dest); err == nil {
			t.Fatal("rowWithNull.GetByIndex into map[string]int64 succeeded, want error")
		}
		if diff := cmp.Diff(wantRow2, dest); diff != "" {
			t.Fatalf("failed scan mutated dest (-want +got):\n%s", diff)
		}
	})

	t.Run("map[string]any unwraps pointers and uses untyped nil without stale keys", func(t *testing.T) {
		dest := map[string]any{"stale": "old"}
		if err := row1.GetByIndex(0, &dest); err != nil {
			t.Fatalf("row1.GetByIndex: %v", err)
		}
		if diff := cmp.Diff(map[string]any{"row1_only": int64(10), "shared": int64(20)}, dest); diff != "" {
			t.Fatalf("row1 map[string]any mismatch (-want +got):\n%s", diff)
		}

		if err := rowWithNull.GetByIndex(0, &dest); err != nil {
			t.Fatalf("rowWithNull.GetByIndex: %v", err)
		}
		if _, hasStale := dest["row1_only"]; hasStale {
			t.Fatalf("dest retained stale key row1_only: %#v", dest)
		}
		if gotOk, exists := dest["ok"]; !exists || gotOk != int64(50) {
			t.Fatalf("dest[\"ok\"] = %#v (%T), want int64(50)", gotOk, gotOk)
		}
		gotNull, exists := dest["nullVal"]
		if !exists || gotNull != nil {
			t.Fatalf("dest[\"nullVal\"] = %#v (%T, exists=%v), want untyped nil", gotNull, gotNull, exists)
		}
	})

	t.Run("incompatible empty map returns error", func(t *testing.T) {
		var dest map[string]int64
		if err := assignValue(reflect.ValueOf(&dest).Elem(), map[string]string{}); err == nil {
			t.Fatal("assignValue(empty map[string]string -> map[string]int64) succeeded, want error")
		}
	})
}

func TestResultRow_NestedBytesMapKeysAndInvalidMapKeys(t *testing.T) {
	typeMapBytesBytes := &btpb.Type{
		Kind: &btpb.Type_MapType{
			MapType: &btpb.Type_Map{
				KeyType:   typeBytes,
				ValueType: typeBytes,
			},
		},
	}
	typeArrayMapBytesBytes := &btpb.Type{
		Kind: &btpb.Type_ArrayType{
			ArrayType: &btpb.Type_Array{ElementType: typeMapBytesBytes},
		},
	}
	typeMapStringMapBytesInt := &btpb.Type{
		Kind: &btpb.Type_MapType{
			MapType: &btpb.Type_Map{
				KeyType: typeString,
				ValueType: &btpb.Type{
					Kind: &btpb.Type_MapType{
						MapType: &btpb.Type_Map{
							KeyType:   typeBytes,
							ValueType: typeInt64,
						},
					},
				},
			},
		},
	}

	pbMeta := createMetadata(
		colMeta("arrOfMaps", typeArrayMapBytesBytes),
		colMeta("mapOfMaps", typeMapStringMapBytesInt),
	)
	rrMeta, err := newResultRowMetadata(pbMeta)
	if err != nil {
		t.Fatalf("newResultRowMetadata: %v", err)
	}

	k1 := base64.StdEncoding.EncodeToString([]byte("qual1"))
	k2 := base64.StdEncoding.EncodeToString([]byte{0xff, 0x00})
	row, err := newResultRow([]*btpb.Value{
		pbArray(
			pbArray(pbMapEntry(pbBytes([]byte("qual1")), pbBytes([]byte("val1")))),
			pbNull(),
		),
		pbArray(
			pbMapEntry(pbString("fam"), pbArray(pbMapEntry(pbBytes([]byte{0xff, 0x00}), pbInt64(7)))),
		),
	}, pbMeta, rrMeta)
	if err != nil {
		t.Fatalf("newResultRow: %v", err)
	}

	var gotArr []map[string][]byte
	if err := row.GetByIndex(0, &gotArr); err != nil {
		t.Fatalf("GetByIndex(0, &gotArr): %v", err)
	}
	wantArr := []map[string][]byte{{k1: []byte("val1")}, nil}
	if diff := cmp.Diff(wantArr, gotArr); diff != "" {
		t.Errorf("GetByIndex(0) mismatch (-want +got):\n%s", diff)
	}

	var gotNested map[string]map[string]int64
	if err := row.GetByIndex(1, &gotNested); err != nil {
		t.Fatalf("GetByIndex(1, &gotNested): %v", err)
	}
	wantNested := map[string]map[string]int64{"fam": {k2: 7}}
	if diff := cmp.Diff(wantNested, gotNested); diff != "" {
		t.Errorf("GetByIndex(1) mismatch (-want +got):\n%s", diff)
	}

	t.Run("non-comparable map key returns codes.Internal instead of panicking", func(t *testing.T) {
		typeMapNonComparableKey := &btpb.Type{
			Kind: &btpb.Type_MapType{
				MapType: &btpb.Type_Map{
					KeyType:   typeArrayString,
					ValueType: typeInt64,
				},
			},
		}
		badMeta := createMetadata(colMeta("badMap", typeMapNonComparableKey))
		badRRMeta, err := newResultRowMetadata(badMeta)
		if err != nil {
			t.Fatalf("newResultRowMetadata(badMeta): %v", err)
		}
		badRow, err := newResultRow([]*btpb.Value{pbArray()}, badMeta, badRRMeta)
		if err != nil {
			t.Fatalf("newResultRow(badRow): %v", err)
		}
		var got any
		err = badRow.GetByIndex(0, &got)
		if status.Code(err) != codes.Internal {
			t.Fatalf("GetByIndex(badMap) status.Code(err) = %v (%v), want codes.Internal", status.Code(err), err)
		}
	})
}

func TestResultRow_NullStructDecoding(t *testing.T) {
	typeArrayStruct := &btpb.Type{
		Kind: &btpb.Type_ArrayType{
			ArrayType: &btpb.Type_Array{ElementType: typeStructSimple},
		},
	}
	typeMapStringStruct := &btpb.Type{
		Kind: &btpb.Type_MapType{
			MapType: &btpb.Type_Map{
				KeyType:   typeString,
				ValueType: typeStructSimple,
			},
		},
	}

	pbMeta := createMetadata(
		colMeta("nullStruct", typeStructSimple),
		colMeta("nonNullStruct", typeStructSimple),
		colMeta("arrStructWithNull", typeArrayStruct),
		colMeta("arrStructNoNull", typeArrayStruct),
		colMeta("mapStructWithNull", typeMapStringStruct),
	)
	rrMeta, err := newResultRowMetadata(pbMeta)
	if err != nil {
		t.Fatalf("newResultRowMetadata: %v", err)
	}

	wantSt := newStruct([]structFieldWithValue{
		{Name: "name", Value: "hello"},
		{Name: "count", Value: int64(42)},
	})
	pbSt := pbArray(pbString("hello"), pbInt64(42))

	row, err := newResultRow([]*btpb.Value{
		pbNull(),
		pbSt,
		pbArray(pbSt, pbNull()),
		pbArray(pbSt),
		pbArray(pbMapEntry(pbString("present"), pbSt), pbMapEntry(pbString("missing"), pbNull())),
	}, pbMeta, rrMeta)
	if err != nil {
		t.Fatalf("newResultRow: %v", err)
	}

	// 0: NULL top-level STRUCT
	stPtr := &wantSt
	if err := row.GetByIndex(0, &stPtr); err != nil {
		t.Fatalf("GetByIndex(0, **Struct) unexpected error: %v", err)
	}
	if stPtr != nil {
		t.Fatalf("GetByIndex(0, **Struct) = %#v, want nil", stPtr)
	}
	var nullAny any = "non-nil"
	if err := row.GetByIndex(0, &nullAny); err != nil || nullAny != nil {
		t.Fatalf("GetByIndex(0, *any) = (%#v, %v), want (nil, nil)", nullAny, err)
	}
	var nonPtrSt Struct
	if err := row.GetByIndex(0, &nonPtrSt); err == nil {
		t.Fatal("GetByIndex(0, *Struct) for NULL STRUCT succeeded, want error")
	}

	// 1: Non-NULL top-level STRUCT into **Struct
	var gotStPtr *Struct
	if err := row.GetByIndex(1, &gotStPtr); err != nil {
		t.Fatalf("GetByIndex(1, **Struct) unexpected error: %v", err)
	}
	if gotStPtr == nil {
		t.Fatal("GetByIndex(1, **Struct) = nil, want non-nil")
	}
	if diff := testutil.Diff(wantSt, *gotStPtr, cmpSQLOpts...); diff != "" {
		t.Errorf("GetByIndex(1, **Struct) mismatch (-want +got):\n%s", diff)
	}

	// 2: ARRAY<STRUCT> containing NULL
	var gotArrPtr []*Struct
	if err := row.GetByIndex(2, &gotArrPtr); err != nil {
		t.Fatalf("GetByIndex(2, *[]*Struct) unexpected error: %v", err)
	}
	if diff := testutil.Diff([]*Struct{&wantSt, nil}, gotArrPtr, cmpSQLOpts...); diff != "" {
		t.Errorf("GetByIndex(2, *[]*Struct) mismatch (-want +got):\n%s", diff)
	}
	var gotArrAny []any
	if err := row.GetByIndex(2, &gotArrAny); err != nil {
		t.Fatalf("GetByIndex(2, *[]any) unexpected error: %v", err)
	}
	if diff := testutil.Diff([]any{wantSt, nil}, gotArrAny, cmpSQLOpts...); diff != "" {
		t.Errorf("GetByIndex(2, *[]any) mismatch (-want +got):\n%s", diff)
	}
	var gotArrVal []Struct
	if err := row.GetByIndex(2, &gotArrVal); err == nil {
		t.Fatal("GetByIndex(2, *[]Struct) with NULL element succeeded, want error")
	}

	// 3: ARRAY<STRUCT> without NULL into []Struct
	if err := row.GetByIndex(3, &gotArrVal); err != nil {
		t.Fatalf("GetByIndex(3, *[]Struct) unexpected error: %v", err)
	}
	if diff := testutil.Diff([]Struct{wantSt}, gotArrVal, cmpSQLOpts...); diff != "" {
		t.Errorf("GetByIndex(3, *[]Struct) mismatch (-want +got):\n%s", diff)
	}

	// 4: MAP<STRING, STRUCT> containing NULL
	var gotMapPtr map[string]*Struct
	if err := row.GetByIndex(4, &gotMapPtr); err != nil {
		t.Fatalf("GetByIndex(4, *map[string]*Struct) unexpected error: %v", err)
	}
	if diff := testutil.Diff(map[string]*Struct{"present": &wantSt, "missing": nil}, gotMapPtr, cmpSQLOpts...); diff != "" {
		t.Errorf("GetByIndex(4, *map[string]*Struct) mismatch (-want +got):\n%s", diff)
	}
	var gotMapAny map[string]any
	if err := row.GetByIndex(4, &gotMapAny); err != nil {
		t.Fatalf("GetByIndex(4, *map[string]any) unexpected error: %v", err)
	}
	if diff := testutil.Diff(map[string]any{"present": wantSt, "missing": nil}, gotMapAny, cmpSQLOpts...); diff != "" {
		t.Errorf("GetByIndex(4, *map[string]any) mismatch (-want +got):\n%s", diff)
	}
	var gotMapVal map[string]Struct
	if err := row.GetByIndex(4, &gotMapVal); err == nil {
		t.Fatal("GetByIndex(4, *map[string]Struct) with NULL value succeeded, want error")
	}

	// Invalid pointer depth (***Struct) and custom struct destination (*customStruct) on STRUCT column
	type customStruct struct {
		Name  string
		Count int64
	}
	var triplePtr **Struct
	if err := row.GetByIndex(1, &triplePtr); err == nil {
		t.Fatal("GetByIndex(1, ***Struct) succeeded, want error")
	}
	var custom customStruct
	if err := row.GetByIndex(1, &custom); err == nil {
		t.Fatal("GetByIndex(1, *customStruct) succeeded, want error")
	}

	// Rejecting ARRAY<STRUCT> into []map[string]any or []customStruct
	var gotSliceOfMaps []map[string]any
	if err := row.GetByIndex(3, &gotSliceOfMaps); err == nil {
		t.Fatal("GetByIndex(3, *[]map[string]any) succeeded, want error")
	}
	var gotSliceOfCustom []customStruct
	if err := row.GetByIndex(3, &gotSliceOfCustom); err == nil {
		t.Fatal("GetByIndex(3, *[]customStruct) succeeded, want error")
	}
}

func TestResultRow_AdditionalConversionBranches(t *testing.T) {
	t.Run("typed nil slices and maps reset pre-populated destinations to nil", func(t *testing.T) {
		destSlice := []int64{1, 2, 3}
		if err := assignValue(reflect.ValueOf(&destSlice).Elem(), ([]*int64)(nil)); err != nil {
			t.Fatalf("assignValue(([]*int64)(nil) -> []int64): %v", err)
		}
		if destSlice != nil {
			t.Fatalf("destSlice = %#v, want nil", destSlice)
		}

		destPtrSlice := []*int64{new(int64)}
		if err := assignValue(reflect.ValueOf(&destPtrSlice).Elem(), ([]*int64)(nil)); err != nil {
			t.Fatalf("assignValue(([]*int64)(nil) -> []*int64): %v", err)
		}
		if destPtrSlice != nil {
			t.Fatalf("destPtrSlice = %#v, want nil", destPtrSlice)
		}

		destMap := map[string]int64{"stale": 99}
		if err := assignValue(reflect.ValueOf(&destMap).Elem(), (map[string]*int64)(nil)); err != nil {
			t.Fatalf("assignValue((map[string]*int64)(nil) -> map[string]int64): %v", err)
		}
		if destMap != nil {
			t.Fatalf("destMap = %#v, want nil", destMap)
		}

		destSameMap := map[string]int64{"stale": 99}
		if err := assignValue(reflect.ValueOf(&destSameMap).Elem(), (map[string]int64)(nil)); err != nil {
			t.Fatalf("assignValue((map[string]int64)(nil) -> map[string]int64): %v", err)
		}
		if destSameMap != nil {
			t.Fatalf("destSameMap = %#v, want nil", destSameMap)
		}
	})

	t.Run("incompatible empty slices and Struct containers return errors", func(t *testing.T) {
		type customStruct struct {
			Name string
		}
		st := newStruct([]structFieldWithValue{{Name: "Name", Value: "v"}})

		var gotInts []int64
		if err := assignValue(reflect.ValueOf(&gotInts).Elem(), []string{}); err == nil {
			t.Fatal("assignValue([]string{} -> []int64) succeeded, want error")
		}

		for _, src := range []any{[]Struct{}, []Struct{st}, []*Struct{}, []*Struct{&st}} {
			var gotMaps []map[string]any
			if err := assignValue(reflect.ValueOf(&gotMaps).Elem(), src); err == nil {
				t.Fatalf("assignValue(%T -> []map[string]any) succeeded, want error", src)
			}
			var gotCustom []customStruct
			if err := assignValue(reflect.ValueOf(&gotCustom).Elem(), src); err == nil {
				t.Fatalf("assignValue(%T -> []customStruct) succeeded, want error", src)
			}
		}
	})

	t.Run("custom string map keys and incompatible map key types", func(t *testing.T) {
		type myKey string

		pbMeta := createMetadata(colMeta("mStr", typeMapStringInt64))
		rrMeta, err := newResultRowMetadata(pbMeta)
		if err != nil {
			t.Fatalf("newResultRowMetadata: %v", err)
		}
		row, err := newResultRow([]*btpb.Value{
			pbArray(pbMapEntry(pbString("k1"), pbInt64(11)), pbMapEntry(pbString("k2"), pbInt64(22))),
		}, pbMeta, rrMeta)
		if err != nil {
			t.Fatalf("newResultRow: %v", err)
		}

		gotCustom := map[myKey]int64{"stale": 1}
		if err := row.GetByIndex(0, &gotCustom); err != nil {
			t.Fatalf("GetByIndex(map[myKey]int64): %v", err)
		}
		wantCustom := map[myKey]int64{"k1": 11, "k2": 22}
		if diff := cmp.Diff(wantCustom, gotCustom); diff != "" {
			t.Fatalf("GetByIndex(map[myKey]int64) mismatch (-want +got):\n%s", diff)
		}

		for _, src := range []any{map[int64]int64{}, map[int64]int64{1: 2}} {
			var gotStringKeyMap map[string]any
			if err := assignValue(reflect.ValueOf(&gotStringKeyMap).Elem(), src); err == nil {
				t.Fatalf("assignValue(%#v -> map[string]any) succeeded, want error", src)
			}
		}
	})

	t.Run("maps with NULL keys unwrap pointer values into map[any]V and map[any]any", func(t *testing.T) {
		pbMeta := createMetadata(
			colMeta("nonNullVals", typeMapStringInt64),
			colMeta("mixedVals", typeMapStringInt64),
		)
		rrMeta, err := newResultRowMetadata(pbMeta)
		if err != nil {
			t.Fatalf("newResultRowMetadata: %v", err)
		}
		row, err := newResultRow([]*btpb.Value{
			pbArray(pbMapEntry(pbNull(), pbInt64(10)), pbMapEntry(pbString("k"), pbInt64(20))),
			pbArray(
				pbMapEntry(pbNull(), pbInt64(10)),
				pbMapEntry(pbString("k"), pbInt64(20)),
				pbMapEntry(pbString("nullVal"), pbNull()),
			),
		}, pbMeta, rrMeta)
		if err != nil {
			t.Fatalf("newResultRow: %v", err)
		}

		var gotAnyInt map[any]int64
		if err := row.GetByIndex(0, &gotAnyInt); err != nil {
			t.Fatalf("GetByIndex(0, *map[any]int64): %v", err)
		}
		wantAnyInt := map[any]int64{nil: 10, "k": 20}
		if diff := cmp.Diff(wantAnyInt, gotAnyInt); diff != "" {
			t.Fatalf("GetByIndex(0, *map[any]int64) mismatch (-want +got):\n%s", diff)
		}

		var gotAnyAny map[any]any
		if err := row.GetByIndex(1, &gotAnyAny); err != nil {
			t.Fatalf("GetByIndex(1, *map[any]any): %v", err)
		}
		wantAnyAny := map[any]any{nil: int64(10), "k": int64(20), "nullVal": nil}
		if diff := cmp.Diff(wantAnyAny, gotAnyAny); diff != "" {
			t.Fatalf("GetByIndex(1, *map[any]any) mismatch (-want +got):\n%s", diff)
		}
		if gotNullVal, exists := gotAnyAny["nullVal"]; !exists || gotNullVal != nil {
			t.Fatalf("gotAnyAny[\"nullVal\"] = %#v (%T, exists=%v), want untyped nil", gotNullVal, gotNullVal, exists)
		}

		var gotStringInt map[string]int64
		if err := row.GetByIndex(0, &gotStringInt); err == nil {
			t.Fatal("GetByIndex(0, *map[string]int64) with NULL key succeeded, want error")
		}
	})
}

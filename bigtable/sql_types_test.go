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

package bigtable

import (
	"context"
	"errors"
	"math"
	"strings"
	"testing"

	btpb "cloud.google.com/go/bigtable/apiv2/bigtablepb"
	"github.com/google/go-cmp/cmp"
)

func TestSQLType_Int64StrictValidation(t *testing.T) {
	type customInt64 int64
	type customUint64 uint64

	int64PbType := &btpb.Type{
		Kind: &btpb.Type_Int64Type{
			Int64Type: &btpb.Type_Int64{},
		},
	}

	validTests := []struct {
		name    string
		val     any
		wantVal *btpb.Value
	}{
		{
			name:    "untyped nil",
			val:     nil,
			wantVal: &btpb.Value{Type: int64PbType},
		},
		{
			name: "int",
			val:  int(-42),
			wantVal: &btpb.Value{
				Type: int64PbType,
				Kind: &btpb.Value_IntValue{IntValue: -42},
			},
		},
		{
			name: "int8",
			val:  int8(-8),
			wantVal: &btpb.Value{
				Type: int64PbType,
				Kind: &btpb.Value_IntValue{IntValue: -8},
			},
		},
		{
			name: "int16",
			val:  int16(1600),
			wantVal: &btpb.Value{
				Type: int64PbType,
				Kind: &btpb.Value_IntValue{IntValue: 1600},
			},
		},
		{
			name: "int32",
			val:  int32(-32000),
			wantVal: &btpb.Value{
				Type: int64PbType,
				Kind: &btpb.Value_IntValue{IntValue: -32000},
			},
		},
		{
			name: "int64 max",
			val:  int64(math.MaxInt64),
			wantVal: &btpb.Value{
				Type: int64PbType,
				Kind: &btpb.Value_IntValue{IntValue: math.MaxInt64},
			},
		},
		{
			name: "int64 min",
			val:  int64(math.MinInt64),
			wantVal: &btpb.Value{
				Type: int64PbType,
				Kind: &btpb.Value_IntValue{IntValue: math.MinInt64},
			},
		},
		{
			name: "defined signed int64 type",
			val:  customInt64(99),
			wantVal: &btpb.Value{
				Type: int64PbType,
				Kind: &btpb.Value_IntValue{IntValue: 99},
			},
		},
	}

	for _, tc := range validTests {
		t.Run(tc.name, func(t *testing.T) {
			got, err := anySQLTypeToPbVal(tc.val, Int64SQLType{})
			if err != nil {
				t.Fatalf("anySQLTypeToPbVal(%v (%T)) unexpected error: %v", tc.val, tc.val, err)
			}
			if diff := cmp.Diff(tc.wantVal, got, cmpOptionsBtpbValue()...); diff != "" {
				t.Errorf("anySQLTypeToPbVal(%v (%T)) diff (-want +got):\n%s", tc.val, tc.val, diff)
			}
		})
	}

	var nilInt64Ptr *int64
	nonNilInt64 := int64(7)
	invalidTests := []struct {
		name string
		val  any
	}{
		{name: "float32 fractional", val: float32(3.99)},
		{name: "float64 fractional", val: float64(3.99)},
		{name: "float64 NaN", val: math.NaN()},
		{name: "float64 +Inf", val: math.Inf(1)},
		{name: "float64 -Inf", val: math.Inf(-1)},
		{name: "uint", val: uint(1)},
		{name: "uint8", val: uint8(1)},
		{name: "uint16", val: uint16(1)},
		{name: "uint32", val: uint32(1)},
		{name: "uint64 small", val: uint64(1)},
		{name: "uint64 overflow", val: uint64(math.MaxUint64)},
		{name: "uintptr", val: uintptr(1)},
		{name: "defined uint64 type", val: customUint64(1)},
		{name: "bool", val: true},
		{name: "pointer to int64", val: &nonNilInt64},
		{name: "typed nil *int64", val: nilInt64Ptr},
	}

	for _, tc := range invalidTests {
		t.Run("reject_"+tc.name, func(t *testing.T) {
			_, err := anySQLTypeToPbVal(tc.val, Int64SQLType{})
			if err == nil {
				t.Fatalf("anySQLTypeToPbVal(%v (%T)) succeeded, want error", tc.val, tc.val)
			}
			var mismatchErr *errTypeMismatch
			if !errors.As(err, &mismatchErr) {
				t.Errorf("anySQLTypeToPbVal(%v (%T)) error = %v, want *errTypeMismatch", tc.val, tc.val, err)
			}
		})
	}
}

func TestSQLType_ArrayNilAndElementValidation(t *testing.T) {
	int64ArrayPbType := &btpb.Type{
		Kind: &btpb.Type_ArrayType{
			ArrayType: &btpb.Type_Array{
				ElementType: &btpb.Type{
					Kind: &btpb.Type_Int64Type{Int64Type: &btpb.Type_Int64{}},
				},
			},
		},
	}
	bytesArrayPbType := &btpb.Type{
		Kind: &btpb.Type_ArrayType{
			ArrayType: &btpb.Type_Array{
				ElementType: &btpb.Type{
					Kind: &btpb.Type_BytesType{BytesType: &btpb.Type_Bytes{}},
				},
			},
		},
	}

	var nilInt64Slice []int64
	var nilAnySlice []any
	var nilBytesSlice [][]byte

	t.Run("typed nil slices encode as SQL NULL", func(t *testing.T) {
		cases := []struct {
			name    string
			val     any
			sqlType SQLType
			want    *btpb.Value
		}{
			{
				name:    "typed nil []int64",
				val:     nilInt64Slice,
				sqlType: ArraySQLType{ElemType: Int64SQLType{}},
				want:    &btpb.Value{Type: int64ArrayPbType},
			},
			{
				name:    "typed nil []any",
				val:     nilAnySlice,
				sqlType: ArraySQLType{ElemType: Int64SQLType{}},
				want:    &btpb.Value{Type: int64ArrayPbType},
			},
			{
				name:    "typed nil [][]byte",
				val:     nilBytesSlice,
				sqlType: ArraySQLType{ElemType: BytesSQLType{}},
				want:    &btpb.Value{Type: bytesArrayPbType},
			},
		}
		for _, tc := range cases {
			t.Run(tc.name, func(t *testing.T) {
				got, err := anySQLTypeToPbVal(tc.val, tc.sqlType)
				if err != nil {
					t.Fatalf("anySQLTypeToPbVal(%T) unexpected error: %v", tc.val, err)
				}
				if got.GetKind() != nil {
					t.Fatalf("anySQLTypeToPbVal(%T).Kind = %#v, want nil (SQL NULL)", tc.val, got.GetKind())
				}
				if diff := cmp.Diff(tc.want, got, cmpOptionsBtpbValue()...); diff != "" {
					t.Errorf("diff (-want +got):\n%s", diff)
				}
			})
		}
	})

	t.Run("non-nil empty slice and zero-length array encode as empty ARRAY", func(t *testing.T) {
		for _, val := range []any{[]int64{}, [0]int64{}, []any{}} {
			got, err := anySQLTypeToPbVal(val, ArraySQLType{ElemType: Int64SQLType{}})
			if err != nil {
				t.Fatalf("anySQLTypeToPbVal(%T) unexpected error: %v", val, err)
			}
			arrVal, ok := got.GetKind().(*btpb.Value_ArrayValue)
			if !ok || arrVal.ArrayValue == nil || len(arrVal.ArrayValue.Values) != 0 {
				t.Fatalf("anySQLTypeToPbVal(%T) = %#v, want empty ArrayValue", val, got)
			}
		}
	})

	t.Run("mismatched concrete element types fail even when empty or nil", func(t *testing.T) {
		var nilStringSlice []string
		badVals := []any{
			nilStringSlice,
			[]string{},
			[0]string{},
			[]float64{},
			[]float64{1.5},
			[]uint64{},
			[]uint64{math.MaxUint64},
			[]*int64{},
		}
		for _, val := range badVals {
			_, err := anySQLTypeToPbVal(val, ArraySQLType{ElemType: Int64SQLType{}})
			if err == nil {
				t.Errorf("anySQLTypeToPbVal(%T) succeeded, want element type mismatch error", val)
				continue
			}
			var mismatchErr *errTypeMismatch
			if !errors.As(err, &mismatchErr) {
				t.Errorf("anySQLTypeToPbVal(%T) error = %v, want wrapped *errTypeMismatch", val, err)
			}
		}
	})

	t.Run("unbindable ElemType rejected in PrepareStatement and Bind but allowed in typeProto", func(t *testing.T) {
		unbindableArrays := []ArraySQLType{
			{ElemType: MapSQLType{KeyType: StringSQLType{}, ValueType: Int64SQLType{}}},
			{ElemType: StructSQLType{Fields: []StructSQLField{{Name: "f", Type: StringSQLType{}}}}},
		}

		c := &Client{}
		for _, arrType := range unbindableArrays {
			if _, err := arrType.typeProto(); err != nil {
				t.Fatalf("%#v.typeProto() unexpected error: %v", arrType, err)
			}
			if arrType.isValidPrepareParamType() {
				t.Fatalf("%#v.isValidPrepareParamType() = true, want false", arrType)
			}
			_, err := c.prepareStatement(context.Background(), "SELECT @p", map[string]SQLType{"p": arrType})
			if err == nil || !strings.Contains(err.Error(), "cannot be used as parameter type") {
				t.Errorf("prepareStatement(%#v) error = %v, want 'cannot be used as parameter type'", arrType, err)
			}
			ps := PreparedStatement{paramTypes: map[string]SQLType{"p": arrType}}
			for _, bindVal := range []any{nil, []any{}} {
				if _, err := ps.Bind(map[string]any{"p": bindVal}); err == nil {
					t.Errorf("Bind(%T) with %#v succeeded, want error", bindVal, arrType)
				}
			}
		}
	})
}

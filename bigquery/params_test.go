// Copyright 2016 Google LLC
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//      http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package bigquery

import (
	"context"
	"errors"
	"fmt"
	"math"
	"math/big"
	"reflect"
	"testing"
	"time"

	"cloud.google.com/go/civil"
	"cloud.google.com/go/internal/testutil"
	"github.com/google/go-cmp/cmp"
	bq "google.golang.org/api/bigquery/v2"
)

var scalarTests = []struct {
	name     string
	val      interface{}            // input value sent as query param
	wantNil  bool                   // whether the value returned in a query field should be nil.
	wantVal  string                 // the string form of the scalar value in QueryParameterValue.
	wantType *bq.QueryParameterType // paramType's desired output
	wantStat interface{}            // val when roundtripped and represented as part of job statistics.
}{
	{"Int64Default", int64(0), false, "0", int64ParamType, int64(0)},
	{"NullInt64Valued", NullInt64{Int64: 3, Valid: true}, false, "3", int64ParamType, int64(3)},
	{"NullInt64Null", NullInt64{Valid: false}, true, "", int64ParamType, NullInt64{Valid: false}},
	{"FloatLiteral", 3.14, false, "3.14", float64ParamType, 3.14},
	{"FloatLiteralExponent", 3.14159e-87, false, "3.14159e-87", float64ParamType, 3.14159e-87},
	{"NullFloatValued", NullFloat64{Float64: 3.14, Valid: true}, false, "3.14", float64ParamType, 3.14},
	{"NullFloatNull", NullFloat64{Valid: false}, true, "", float64ParamType, NullFloat64{Valid: false}},
	{"FloatNaN", math.NaN(), false, "NaN", float64ParamType, math.NaN()},
	{"Boolean", true, false, "true", boolParamType, true},
	{"NullBoolValued", NullBool{Bool: true, Valid: true}, false, "true", boolParamType, true},
	{"NullBoolNull", NullBool{Valid: false}, true, "", boolParamType, NullBool{Valid: false}},
	{"String", "string", false, "string", stringParamType, "string"},
	{"StringUnicode", "\u65e5\u672c\u8a9e\n", false, "\u65e5\u672c\u8a9e\n", stringParamType, "\u65e5\u672c\u8a9e\n"},
	{"NullStringValued", NullString{StringVal: "string2", Valid: true}, false, "string2", stringParamType, "string2"},
	{"NullStringNull", NullString{Valid: false}, true, "", stringParamType, NullString{Valid: false}},
	{"Bytes", []byte("foo"), false, "Zm9v", bytesParamType, []byte("foo")}, // base64 encoding of "foo"
	{"TimestampFixed", time.Date(2016, 3, 20, 4, 22, 9, 5000, time.FixedZone("neg1-2", -3720)),
		false,
		"2016-03-20 04:22:09.000005-01:02",
		timestampParamType,
		time.Date(2016, 3, 20, 4, 22, 9, 5000, time.FixedZone("neg1-2", -3720))},
	{"NullTimestampValued", NullTimestamp{Timestamp: time.Date(2016, 3, 22, 4, 22, 9, 5000, time.FixedZone("neg1-2", -3720)), Valid: true},
		false,
		"2016-03-22 04:22:09.000005-01:02",
		timestampParamType,
		time.Date(2016, 3, 22, 4, 22, 9, 5000, time.FixedZone("neg1-2", -3720))},
	{"NullTimestampNull", NullTimestamp{Valid: false},
		true,
		"",
		timestampParamType,
		NullTimestamp{Valid: false}},
	{"Date", civil.Date{Year: 2016, Month: 3, Day: 20},
		false,
		"2016-03-20",
		dateParamType,
		civil.Date{Year: 2016, Month: 3, Day: 20}},
	{"NullDateValued", NullDate{
		Date: civil.Date{Year: 2016, Month: 3, Day: 24}, Valid: true},
		false,
		"2016-03-24",
		dateParamType,
		civil.Date{Year: 2016, Month: 3, Day: 24}},
	{"NullDateNull", NullDate{Valid: false},
		true,
		"",
		dateParamType,
		NullDate{Valid: false}},
	{"Time", civil.Time{Hour: 4, Minute: 5, Second: 6, Nanosecond: 789000000},
		false,
		"04:05:06.789000",
		timeParamType,
		civil.Time{Hour: 4, Minute: 5, Second: 6, Nanosecond: 789000000}},
	{"NullTimeValued", NullTime{
		Time: civil.Time{Hour: 6, Minute: 7, Second: 8, Nanosecond: 789000000}, Valid: true},
		false,
		"06:07:08.789000",
		timeParamType,
		civil.Time{Hour: 6, Minute: 7, Second: 8, Nanosecond: 789000000}},
	{"NullTimeNull", NullTime{Valid: false},
		true,
		"",
		timeParamType,
		NullTime{Valid: false}},
	{"Datetime", civil.DateTime{Date: civil.Date{Year: 2016, Month: 3, Day: 20}, Time: civil.Time{Hour: 4, Minute: 5, Second: 6, Nanosecond: 789000000}},
		false,
		"2016-03-20 04:05:06.789000",
		dateTimeParamType,
		civil.DateTime{Date: civil.Date{Year: 2016, Month: 3, Day: 20}, Time: civil.Time{Hour: 4, Minute: 5, Second: 6, Nanosecond: 789000000}}},
	{"NullDateTimeValued", NullDateTime{
		DateTime: civil.DateTime{Date: civil.Date{Year: 2016, Month: 3, Day: 21}, Time: civil.Time{Hour: 4, Minute: 5, Second: 6, Nanosecond: 789000000}}, Valid: true},
		false,
		"2016-03-21 04:05:06.789000",
		dateTimeParamType,
		civil.DateTime{Date: civil.Date{Year: 2016, Month: 3, Day: 21}, Time: civil.Time{Hour: 4, Minute: 5, Second: 6, Nanosecond: 789000000}}},
	{"NullDateTimeNull", NullDateTime{Valid: false},
		true,
		"",
		dateTimeParamType,
		NullDateTime{Valid: false}},
	{"Numeric", big.NewRat(12345, 1000), false, "12.345000000", numericParamType, big.NewRat(12345, 1000)},
	{"BignumericParam", &QueryParameterValue{
		Type: StandardSQLDataType{
			TypeKind: "BIGNUMERIC",
		},
		Value: BigNumericString(big.NewRat(12345, 10e10)),
	}, false, "0.00000012345000000000000000000000000000", bigNumericParamType, big.NewRat(12345, 10e10)},
	{"IntervalValue", &IntervalValue{Years: 1, Months: 2, Days: 3}, false, "1-2 3 0:0:0", intervalParamType, &IntervalValue{Years: 1, Months: 2, Days: 3}},
	{"NullGeographyValued", NullGeography{GeographyVal: "POINT(-122.335503 47.625536)", Valid: true}, false, "POINT(-122.335503 47.625536)", geographyParamType, "POINT(-122.335503 47.625536)"},
	{"NullGeographyNull", NullGeography{Valid: false}, true, "", geographyParamType, NullGeography{Valid: false}},
	{"NullJsonValued", NullJSON{Valid: true, JSONVal: "{\"alpha\":\"beta\"}"}, false, "{\"alpha\":\"beta\"}", jsonParamType, "{\"alpha\":\"beta\"}"},
	{"NullJsonNull", NullJSON{Valid: false}, true, "", jsonParamType, NullJSON{Valid: false}},
}

type (
	S1 struct {
		A int
		B *S2
		C bool
	}
	S2 struct {
		D string
	}
)

var (
	s1 = S1{
		A: 1,
		B: &S2{D: "s"},
		C: true,
	}

	s1ParamType = &bq.QueryParameterType{
		Type: "STRUCT",
		StructTypes: []*bq.QueryParameterTypeStructTypes{
			{Name: "A", Type: int64ParamType},
			{Name: "B", Type: &bq.QueryParameterType{
				Type: "STRUCT",
				StructTypes: []*bq.QueryParameterTypeStructTypes{
					{Name: "D", Type: stringParamType},
				},
			}},
			{Name: "C", Type: boolParamType},
		},
	}

	s1ParamValue = bq.QueryParameterValue{
		StructValues: map[string]bq.QueryParameterValue{
			"A": sval("1"),
			"B": {
				StructValues: map[string]bq.QueryParameterValue{
					"D": sval("s"),
				},
			},
			"C": sval("true"),
		},
	}

	s1ParamReturnValue = map[string]interface{}{
		"A": int64(1),
		"B": map[string]interface{}{"D": "s"},
		"C": true,
	}
)

func sval(s string) bq.QueryParameterValue {
	return bq.QueryParameterValue{Value: s}
}

func TestParamValueScalar(t *testing.T) {
	nilValue := &bq.QueryParameterValue{
		NullFields: []string{"Value"},
	}

	for _, tc := range scalarTests {
		t.Run(tc.name, func(t *testing.T) {
			got, err := paramValue(reflect.ValueOf(tc.val))
			if err != nil {
				t.Errorf("%v: got err %v", tc.val, err)
			}
			if tc.wantNil {
				if !testutil.Equal(got, nilValue) {
					t.Errorf("%#v: wanted empty QueryParameterValue, got %v", tc.val, got)
				}
			} else {
				want := sval(tc.wantVal)
				if !testutil.Equal(got, &want) {
					t.Errorf("%#v:\ngot  %+v\nwant %+v", tc.val, got, want)
				}
			}
		})
	}
}

func TestParamValueRange(t *testing.T) {

	tTimestamp := time.Date(2016, 3, 22, 4, 22, 9, 5000, time.FixedZone("neg1-2", -3720))
	tDate := civil.Date{Year: 2016, Month: 03, Day: 22}
	tDateTime := civil.DateTime{
		Date: civil.Date{Year: 2017, Month: 7, Day: 13},
		Time: civil.Time{Hour: 4, Minute: 5, Second: 6, Nanosecond: 789000000},
	}
	wTimestamp := "2016-03-22 04:22:09.000005-01:02"
	wDate := "2016-03-22"
	wDateTime := "2017-07-13 04:05:06.789000"

	var testCases = []struct {
		desc string
		in   interface{}
		want *bq.QueryParameterValue
	}{
		{
			desc: "RangeValue time.Time both populated",
			in: &RangeValue{
				Start: tTimestamp,
				End:   tTimestamp,
			},
			want: &bq.QueryParameterValue{
				RangeValue: &bq.RangeValue{
					Start: &bq.QueryParameterValue{
						Value: wTimestamp,
					},
					End: &bq.QueryParameterValue{
						Value: wTimestamp,
					},
				},
			},
		},
		{
			desc: "RangeValue time.Time start only",
			in: &RangeValue{
				Start: tTimestamp,
			},
			want: &bq.QueryParameterValue{
				RangeValue: &bq.RangeValue{
					Start: &bq.QueryParameterValue{
						Value: wTimestamp,
					},
				},
			},
		},
		{
			desc: "RangeValue time.Time end only",
			in: &RangeValue{
				End: tTimestamp,
			},
			want: &bq.QueryParameterValue{
				RangeValue: &bq.RangeValue{
					End: &bq.QueryParameterValue{
						Value: wTimestamp,
					},
				},
			},
		},
		{
			desc: "RangeValue NullTimestamp both populated",
			in: &RangeValue{
				Start: NullTimestamp{Valid: true, Timestamp: tTimestamp},
				End:   NullTimestamp{Valid: true, Timestamp: tTimestamp},
			},
			want: &bq.QueryParameterValue{
				RangeValue: &bq.RangeValue{
					Start: &bq.QueryParameterValue{
						Value: wTimestamp,
					},
					End: &bq.QueryParameterValue{
						Value: wTimestamp,
					},
				},
			},
		},
		{
			desc: "RangeValue NullTimestamp start only",
			in: &RangeValue{
				Start: NullTimestamp{Valid: true, Timestamp: tTimestamp},
				End:   NullTimestamp{Valid: false},
			},
			want: &bq.QueryParameterValue{
				RangeValue: &bq.RangeValue{
					Start: &bq.QueryParameterValue{
						Value: wTimestamp,
					},
					End: &bq.QueryParameterValue{NullFields: []string{"Value"}},
				},
			},
		},
		{
			desc: "RangeValue time.Time end only",
			in: &RangeValue{
				Start: NullTimestamp{Valid: false},
				End:   NullTimestamp{Valid: true, Timestamp: tTimestamp},
			},
			want: &bq.QueryParameterValue{
				RangeValue: &bq.RangeValue{
					Start: &bq.QueryParameterValue{NullFields: []string{"Value"}},
					End: &bq.QueryParameterValue{
						Value: wTimestamp,
					},
				},
			},
		},
		{
			desc: "RangeValue civil.Date both populated",
			in: &RangeValue{
				Start: tDate,
				End:   tDate,
			},
			want: &bq.QueryParameterValue{
				RangeValue: &bq.RangeValue{
					Start: &bq.QueryParameterValue{
						Value: wDate,
					},
					End: &bq.QueryParameterValue{
						Value: wDate,
					},
				},
			},
		},
		{
			desc: "RangeValue civil.DateTime both populated",
			in: &RangeValue{
				Start: tDateTime,
				End:   tDateTime,
			},
			want: &bq.QueryParameterValue{
				RangeValue: &bq.RangeValue{
					Start: &bq.QueryParameterValue{
						Value: wDateTime,
					},
					End: &bq.QueryParameterValue{
						Value: wDateTime,
					},
				},
			},
		},
		{
			desc: "Unbounded Range in QueryParameterValue",
			in: &QueryParameterValue{
				Type: StandardSQLDataType{
					TypeKind: "RANGE",
					RangeElementType: &StandardSQLDataType{
						TypeKind: "DATETIME",
					},
				},
				Value: &RangeValue{},
			},
			want: &bq.QueryParameterValue{
				RangeValue: &bq.RangeValue{},
			},
		},
	}

	for _, tc := range testCases {
		got, err := paramValue(reflect.ValueOf(tc.in))
		if err != nil {
			t.Errorf("%q: got error %v", tc.desc, err)
		}
		if d := testutil.Diff(got, tc.want); d != "" {
			t.Errorf("%q: mismatch\n%s", tc.desc, d)
		}
	}
}

func TestParamValueArray(t *testing.T) {
	qpv := &bq.QueryParameterValue{ArrayValues: []*bq.QueryParameterValue{
		{Value: "1"},
		{Value: "2"},
	},
	}
	for _, tc := range []struct {
		name string
		val  interface{}
		want *bq.QueryParameterValue
	}{
		{"nilIntSlice", []int(nil), &bq.QueryParameterValue{}},
		{"emptyIntSlice", []int{}, &bq.QueryParameterValue{}},
		{"slice", []int{1, 2}, qpv},
		{"array", [2]int{1, 2}, qpv},
	} {
		got, err := paramValue(reflect.ValueOf(tc.val))
		if err != nil {
			t.Fatal(err)
		}
		if !testutil.Equal(got, tc.want) {
			t.Errorf("%#v:\ngot  %+v\nwant %+v", tc.val, got, tc.want)
		}
	}
}

func TestParamValueStruct(t *testing.T) {
	got, err := paramValue(reflect.ValueOf(s1))
	if err != nil {
		t.Fatal(err)
	}
	if !testutil.Equal(got, &s1ParamValue) {
		t.Errorf("got  %+v\nwant %+v", got, &s1ParamValue)
	}
}

func TestParamValueErrors(t *testing.T) {
	// paramValue lets a few invalid types through, but paramType catches them.
	// Since we never call one without the other that's fine.
	for _, val := range []interface{}{nil, new([]int)} {
		_, err := paramValue(reflect.ValueOf(val))
		if err == nil {
			t.Errorf("%v (%T): got nil, want error", val, val)
		}
	}
}

func TestParamType(t *testing.T) {
	for _, tc := range scalarTests {
		t.Run(fmt.Sprintf("scalar-%s", tc.name), func(t *testing.T) {
			got, err := paramType(reflect.TypeOf(tc.val), reflect.ValueOf(tc.val))
			if err != nil {
				t.Fatal(err)
			}
			if d := testutil.Diff(got, tc.wantType); d != "" {
				t.Errorf("%v (%T): \n%s", tc.val, tc.val, d)
			}
		})
	}
	for _, tc := range []struct {
		name string
		val  interface{}
		want *bq.QueryParameterType
	}{
		{"uint32", uint32(32767), int64ParamType},
		{"byteSlice", []byte("foo"), bytesParamType},
		{"intArray", []int{}, &bq.QueryParameterType{Type: "ARRAY", ArrayType: int64ParamType}},
		{"boolArray", [3]bool{}, &bq.QueryParameterType{Type: "ARRAY", ArrayType: boolParamType}},
		{"emptyStruct", S1{}, s1ParamType},
		{"RangeTimestampNilEnd", &RangeValue{Start: time.Now()}, &bq.QueryParameterType{Type: "RANGE", RangeElementType: timestampParamType}},
		{"RangeTimestampNilStart", &RangeValue{End: time.Now()}, &bq.QueryParameterType{Type: "RANGE", RangeElementType: timestampParamType}},
		{"RangeTimestampNullValStart", &RangeValue{Start: NullTimestamp{Valid: false}}, &bq.QueryParameterType{Type: "RANGE", RangeElementType: timestampParamType}},
		{"RangeTimestampNullValEnd", &RangeValue{End: NullTimestamp{Valid: false}}, &bq.QueryParameterType{Type: "RANGE", RangeElementType: timestampParamType}},
		{"RangeDateTimeEmptyStart", &RangeValue{Start: civil.DateTime{}}, &bq.QueryParameterType{Type: "RANGE", RangeElementType: dateTimeParamType}},
		{"RangeDateEmptyEnd", &RangeValue{End: civil.Date{}}, &bq.QueryParameterType{Type: "RANGE", RangeElementType: dateParamType}},
		{"RangeDateTimeInQPV",
			&QueryParameterValue{
				Type: StandardSQLDataType{
					TypeKind: "RANGE",
					RangeElementType: &StandardSQLDataType{
						TypeKind: "DATETIME",
					},
				},
				Value: &RangeValue{},
			},
			&bq.QueryParameterType{
				Type: "RANGE",
				RangeElementType: &bq.QueryParameterType{
					Type: "DATETIME",
				},
			},
		},
	} {
		t.Run(fmt.Sprintf("complex-%s", tc.name), func(t *testing.T) {
			got, err := paramType(reflect.TypeOf(tc.val), reflect.ValueOf(tc.val))
			if err != nil {
				t.Fatal(err)
			}
			if d := testutil.Diff(got, tc.want); d != "" {
				t.Errorf("%v (%T): \n%s", tc.val, tc.val, d)
			}
		})
	}
}
func TestParamTypeErrors(t *testing.T) {
	for _, val := range []interface{}{
		nil, uint(0), new([]int), make(chan int), map[int]interface{}{}, &RangeValue{},
	} {
		_, err := paramType(reflect.TypeOf(val), reflect.ValueOf(val))
		if err == nil {
			t.Errorf("%v (%T): got nil, want error", val, val)
		}
	}

	type recArr struct {
		RecArr []recArr
	}
	type recMap struct {
		RecMap map[string]recMap
	}
	queryParam := QueryParameterValue{
		StructValue: map[string]QueryParameterValue{
			"nested": {
				Type: StandardSQLDataType{
					TypeKind: "STRING",
				},
				Value: "TEST",
			},
		},
	}
	standardSQL := StandardSQLDataType{
		ArrayElementType: &StandardSQLDataType{
			TypeKind: "NUMERIC",
		},
	}
	recursiveArr := recArr{
		RecArr: []recArr{},
	}
	recursiveMap := recMap{
		RecMap: map[string]recMap{},
	}
	// Recursive structs
	for _, val := range []interface{}{
		queryParam, standardSQL, recursiveArr, recursiveMap,
	} {
		_, err := paramType(reflect.TypeOf(val), reflect.ValueOf(val))
		if err == nil {
			t.Errorf("%v (%T): got nil, want error", val, val)
		}
	}
}

func TestConvertParamValue(t *testing.T) {
	// Scalars.
	for _, tc := range scalarTests {
		t.Run(fmt.Sprintf("scalar-%s", tc.name), func(t *testing.T) {
			pval, err := paramValue(reflect.ValueOf(tc.val))
			if err != nil {
				t.Fatal(err)
			}
			ptype, err := paramType(reflect.TypeOf(tc.val), reflect.ValueOf(tc.val))
			if err != nil {
				t.Fatal(err)
			}
			got, err := convertParamValue(pval, ptype)
			if err != nil {
				t.Fatalf("convertParamValue(%+v, %+v): %v", pval, ptype, err)
			}
			if !testutil.Equal(got, tc.wantStat) {
				t.Errorf("%#v: wanted stat as %#v, got %#v", tc.val, tc.wantStat, got)
			}
		})
	}
	// Arrays.
	for _, tc := range []struct {
		name string
		pval *bq.QueryParameterValue
		want []interface{}
	}{
		{
			"empty",
			&bq.QueryParameterValue{},
			nil,
		},
		{
			"intArray",
			&bq.QueryParameterValue{
				ArrayValues: []*bq.QueryParameterValue{{Value: "1"}, {Value: "2"}},
			},
			[]interface{}{int64(1), int64(2)},
		},
	} {
		t.Run(fmt.Sprintf("array-%s", tc.name), func(t *testing.T) {
			ptype := &bq.QueryParameterType{Type: "ARRAY", ArrayType: int64ParamType}
			got, err := convertParamValue(tc.pval, ptype)
			if err != nil {
				t.Fatalf("%+v: %v", tc.pval, err)
			}
			if !testutil.Equal(got, tc.want) {
				t.Errorf("%+v: got %+v, want %+v", tc.pval, got, tc.want)
			}
		})
	}
	// Structs.
	t.Run("s1struct", func(t *testing.T) {
		got, err := convertParamValue(&s1ParamValue, s1ParamType)
		if err != nil {
			t.Fatal(err)
		}
		if !testutil.Equal(got, s1ParamReturnValue) {
			t.Errorf("got %+v, want %+v", got, s1ParamReturnValue)
		}
	})
}

func TestIntegration_ScalarParam(t *testing.T) {
	roundToMicros := cmp.Transformer("RoundToMicros",
		func(t time.Time) time.Time { return t.Round(time.Microsecond) })
	c := getClient(t)
	for _, tc := range scalarTests {
		t.Run(tc.name, func(t *testing.T) {
			gotData, gotParam, err := paramRoundTrip(c, tc.val)
			if err != nil {
				t.Errorf("input %#v errored: %v", tc.val, err)
			}
			// first, check the returned query value
			if tc.wantNil {
				if gotData != nil {
					t.Errorf("data value %#v expected nil, got %#v", tc.val, gotData)
				}
			} else {
				if !testutil.Equal(gotData, tc.wantStat, roundToMicros) {
					t.Errorf("\ngot data value %#v (%T)\nwant %#v (%T)", gotData, gotData, tc.wantStat, tc.wantStat)
				}
			}
			// then, check the stat value
			if !testutil.Equal(gotParam, tc.wantStat, roundToMicros) {
				t.Errorf("\ngot param stat %#v (%T)\nwant %#v (%T)", gotParam, gotParam, tc.wantStat, tc.wantStat)
			}
		})
	}
}

func TestIntegration_OtherParam(t *testing.T) {
	c := getClient(t)
	for _, tc := range []struct {
		name      string
		val       interface{}
		wantData  interface{}
		wantParam interface{}
	}{
		{"nil", []int(nil), []Value(nil), []interface{}(nil)},
		{"emptyIntSlice", []int{}, []Value(nil), []interface{}(nil)},
		{
			"intSlice",
			[]int{1, 2},
			[]Value{int64(1), int64(2)},
			[]interface{}{int64(1), int64(2)},
		},
		{
			"intArray",
			[3]int{1, 2, 3},
			[]Value{int64(1), int64(2), int64(3)},
			[]interface{}{int64(1), int64(2), int64(3)},
		},
		{
			"emptyStruct",
			S1{},
			[]Value{int64(0), nil, false},
			map[string]interface{}{
				"A": int64(0),
				"B": nil,
				"C": false,
			},
		},
		{
			"s1struct",
			s1,
			[]Value{int64(1), []Value{"s"}, true},
			s1ParamReturnValue,
		},
		{
			"RangeTimestamp",
			&RangeValue{
				Start: time.Date(2016, 3, 22, 4, 22, 9, 5000, time.FixedZone("neg1-2", -3720)),
				End:   NullTimestamp{},
			},
			&RangeValue{
				Start: time.Date(2016, 3, 22, 4, 22, 9, 5000, time.FixedZone("neg1-2", -3720)),
				End:   nil,
			},
			&RangeValue{
				Start: time.Date(2016, 3, 22, 4, 22, 9, 5000, time.FixedZone("neg1-2", -3720)),
				End:   nil,
			},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			gotData, gotParam, err := paramRoundTrip(c, tc.val)
			if err != nil {
				t.Fatal(err)
			}
			if !testutil.Equal(gotData, tc.wantData) {
				t.Errorf("%#v:\ngot  %#v (%T)\nwant %#v (%T)",
					tc.val, gotData, gotData, tc.wantData, tc.wantData)
			}
			if !testutil.Equal(gotParam, tc.wantParam) {
				t.Errorf("%#v:\ngot  %#v (%T)\nwant %#v (%T)",
					tc.val, gotParam, gotParam, tc.wantParam, tc.wantParam)
			}
		})
	}
}

// paramRoundTrip passes x as a query parameter to BigQuery. It returns
// the resulting data value from running the query and the parameter value from
// the returned job configuration.
func paramRoundTrip(c *Client, x interface{}) (data Value, param interface{}, err error) {
	ctx := context.Background()
	q := c.Query("select ?")
	q.Parameters = []QueryParameter{{Value: x}}
	job, err := q.Run(ctx)
	if err != nil {
		return nil, nil, err
	}
	it, err := job.Read(ctx)
	if err != nil {
		return nil, nil, err
	}
	var val []Value
	err = it.Next(&val)
	if err != nil {
		return nil, nil, err
	}
	if len(val) != 1 {
		return nil, nil, errors.New("wrong number of values")
	}
	conf, err := job.Config()
	if err != nil {
		return nil, nil, err
	}
	return val[0], conf.(*QueryConfig).Parameters[0].Value, nil
}

func TestQueryParameter_toBQ(t *testing.T) {
	in := QueryParameter{Name: "name", Value: ""}
	want := []string{"Value"}
	q, err := in.toBQ()
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	got := q.ParameterValue.ForceSendFields
	if !cmp.Equal(want, got) {
		t.Fatalf("want %v, got %v", want, got)
	}
}

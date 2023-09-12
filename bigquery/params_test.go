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
	val      interface{}            // input value sent as query param
	wantNil  bool                   // whether the value returned in a query field should be nil.
	wantVal  string                 // the string form of the scalar value in QueryParameterValue.
	wantType *bq.QueryParameterType // paramType's desired output
	wantStat interface{}            // val when roundtripped and represented as part of job statistics.
}{
	{int64(0), false, "0", int64ParamType, int64(0)},
	{NullInt64{Int64: 3, Valid: true}, false, "3", int64ParamType, int64(3)},
	{NullInt64{Valid: false}, true, "", int64ParamType, NullInt64{Valid: false}},
	{3.14, false, "3.14", float64ParamType, 3.14},
	{3.14159e-87, false, "3.14159e-87", float64ParamType, 3.14159e-87},
	{NullFloat64{Float64: 3.14, Valid: true}, false, "3.14", float64ParamType, 3.14},
	{NullFloat64{Valid: false}, true, "", float64ParamType, NullFloat64{Valid: false}},
	{math.NaN(), false, "NaN", float64ParamType, math.NaN()},
	{true, false, "true", boolParamType, true},
	{NullBool{Bool: true, Valid: true}, false, "true", boolParamType, true},
	{NullBool{Valid: false}, true, "", boolParamType, NullBool{Valid: false}},
	{"string", false, "string", stringParamType, "string"},
	{"\u65e5\u672c\u8a9e\n", false, "\u65e5\u672c\u8a9e\n", stringParamType, "\u65e5\u672c\u8a9e\n"},
	{NullString{StringVal: "string2", Valid: true}, false, "string2", stringParamType, "string2"},
	{NullString{Valid: false}, true, "", stringParamType, NullString{Valid: false}},
	{[]byte("foo"), false, "Zm9v", bytesParamType, []byte("foo")}, // base64 encoding of "foo"
	{time.Date(2016, 3, 20, 4, 22, 9, 5000, time.FixedZone("neg1-2", -3720)),
		false,
		"2016-03-20 04:22:09.000005-01:02",
		timestampParamType,
		time.Date(2016, 3, 20, 4, 22, 9, 5000, time.FixedZone("neg1-2", -3720))},
	{NullTimestamp{Timestamp: time.Date(2016, 3, 22, 4, 22, 9, 5000, time.FixedZone("neg1-2", -3720)), Valid: true},
		false,
		"2016-03-22 04:22:09.000005-01:02",
		timestampParamType,
		time.Date(2016, 3, 22, 4, 22, 9, 5000, time.FixedZone("neg1-2", -3720))},
	{NullTimestamp{Valid: false},
		true,
		"",
		timestampParamType,
		NullTimestamp{Valid: false}},
	{civil.Date{Year: 2016, Month: 3, Day: 20},
		false,
		"2016-03-20",
		dateParamType,
		civil.Date{Year: 2016, Month: 3, Day: 20}},
	{NullDate{
		Date: civil.Date{Year: 2016, Month: 3, Day: 24}, Valid: true},
		false,
		"2016-03-24",
		dateParamType,
		civil.Date{Year: 2016, Month: 3, Day: 24}},
	{NullDate{Valid: false},
		true,
		"",
		dateParamType,
		NullDate{Valid: false}},
	{civil.Time{Hour: 4, Minute: 5, Second: 6, Nanosecond: 789000000},
		false,
		"04:05:06.789000",
		timeParamType,
		civil.Time{Hour: 4, Minute: 5, Second: 6, Nanosecond: 789000000}},
	{NullTime{
		Time: civil.Time{Hour: 6, Minute: 7, Second: 8, Nanosecond: 789000000}, Valid: true},
		false,
		"06:07:08.789000",
		timeParamType,
		civil.Time{Hour: 6, Minute: 7, Second: 8, Nanosecond: 789000000}},
	{NullTime{Valid: false},
		true,
		"",
		timeParamType,
		NullTime{Valid: false}},
	{civil.DateTime{Date: civil.Date{Year: 2016, Month: 3, Day: 20}, Time: civil.Time{Hour: 4, Minute: 5, Second: 6, Nanosecond: 789000000}},
		false,
		"2016-03-20 04:05:06.789000",
		dateTimeParamType,
		civil.DateTime{Date: civil.Date{Year: 2016, Month: 3, Day: 20}, Time: civil.Time{Hour: 4, Minute: 5, Second: 6, Nanosecond: 789000000}}},
	{NullDateTime{
		DateTime: civil.DateTime{Date: civil.Date{Year: 2016, Month: 3, Day: 21}, Time: civil.Time{Hour: 4, Minute: 5, Second: 6, Nanosecond: 789000000}}, Valid: true},
		false,
		"2016-03-21 04:05:06.789000",
		dateTimeParamType,
		civil.DateTime{Date: civil.Date{Year: 2016, Month: 3, Day: 21}, Time: civil.Time{Hour: 4, Minute: 5, Second: 6, Nanosecond: 789000000}}},
	{NullDateTime{Valid: false},
		true,
		"",
		dateTimeParamType,
		NullDateTime{Valid: false}},
	{big.NewRat(12345, 1000), false, "12.345000000", numericParamType, big.NewRat(12345, 1000)},
	{&QueryParameterValue{
		Type: StandardSQLDataType{
			TypeKind: "BIGNUMERIC",
		},
		Value: BigNumericString(big.NewRat(12345, 10e10)),
	}, false, "0.00000012345000000000000000000000000000", bigNumericParamType, big.NewRat(12345, 10e10)},
	{&IntervalValue{Years: 1, Months: 2, Days: 3}, false, "1-2 3 0:0:0", intervalParamType, &IntervalValue{Years: 1, Months: 2, Days: 3}},
	{NullGeography{GeographyVal: "POINT(-122.335503 47.625536)", Valid: true}, false, "POINT(-122.335503 47.625536)", geographyParamType, "POINT(-122.335503 47.625536)"},
	{NullGeography{Valid: false}, true, "", geographyParamType, NullGeography{Valid: false}},
	{NullJSON{Valid: true, JSONVal: "{\"alpha\":\"beta\"}"}, false, "{\"alpha\":\"beta\"}", jsonParamType, "{\"alpha\":\"beta\"}"},
	{NullJSON{Valid: false}, true, "", jsonParamType, NullJSON{Valid: false}},
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

	for _, test := range scalarTests {
		got, err := paramValue(reflect.ValueOf(test.val))
		if err != nil {
			t.Errorf("%v: got err %v", test.val, err)
		}
		if test.wantNil {
			if !testutil.Equal(got, nilValue) {
				t.Errorf("%#v: wanted empty QueryParameterValue, got %v", test.val, got)
			}
		} else {
			want := sval(test.wantVal)
			if !testutil.Equal(got, &want) {
				t.Errorf("%#v:\ngot  %+v\nwant %+v", test.val, got, want)
			}
		}
	}
}

func TestParamValueArray(t *testing.T) {
	qpv := &bq.QueryParameterValue{ArrayValues: []*bq.QueryParameterValue{
		{Value: "1"},
		{Value: "2"},
	},
	}
	for _, test := range []struct {
		val  interface{}
		want *bq.QueryParameterValue
	}{
		{[]int(nil), &bq.QueryParameterValue{}},
		{[]int{}, &bq.QueryParameterValue{}},
		{[]int{1, 2}, qpv},
		{[2]int{1, 2}, qpv},
	} {
		got, err := paramValue(reflect.ValueOf(test.val))
		if err != nil {
			t.Fatal(err)
		}
		if !testutil.Equal(got, test.want) {
			t.Errorf("%#v:\ngot  %+v\nwant %+v", test.val, got, test.want)
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
	for _, test := range scalarTests {
		got, err := paramType(reflect.TypeOf(test.val), reflect.ValueOf(test.val))
		if err != nil {
			t.Fatal(err)
		}
		if !testutil.Equal(got, test.wantType) {
			t.Errorf("%v (%T): got %v, want %v", test.val, test.val, got, test.wantType)
		}
	}
	for _, test := range []struct {
		val  interface{}
		want *bq.QueryParameterType
	}{
		{uint32(32767), int64ParamType},
		{[]byte("foo"), bytesParamType},
		{[]int{}, &bq.QueryParameterType{Type: "ARRAY", ArrayType: int64ParamType}},
		{[3]bool{}, &bq.QueryParameterType{Type: "ARRAY", ArrayType: boolParamType}},
		{S1{}, s1ParamType},
	} {
		got, err := paramType(reflect.TypeOf(test.val), reflect.ValueOf(test.val))
		if err != nil {
			t.Fatal(err)
		}
		if !testutil.Equal(got, test.want) {
			t.Errorf("%v (%T): got %v, want %v", test.val, test.val, got, test.want)
		}
	}
}
func TestParamTypeErrors(t *testing.T) {
	for _, val := range []interface{}{
		nil, uint(0), new([]int), make(chan int), map[string]interface{}{},
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
	for _, test := range scalarTests {
		pval, err := paramValue(reflect.ValueOf(test.val))
		if err != nil {
			t.Fatal(err)
		}
		ptype, err := paramType(reflect.TypeOf(test.val), reflect.ValueOf(test.val))
		if err != nil {
			t.Fatal(err)
		}
		got, err := convertParamValue(pval, ptype)
		if err != nil {
			t.Fatalf("convertParamValue(%+v, %+v): %v", pval, ptype, err)
		}
		if !testutil.Equal(got, test.wantStat) {
			t.Errorf("%#v: wanted stat as %#v, got %#v", test.val, test.wantStat, got)
		}
	}
	// Arrays.
	for _, test := range []struct {
		pval *bq.QueryParameterValue
		want []interface{}
	}{
		{
			&bq.QueryParameterValue{},
			nil,
		},
		{
			&bq.QueryParameterValue{
				ArrayValues: []*bq.QueryParameterValue{{Value: "1"}, {Value: "2"}},
			},
			[]interface{}{int64(1), int64(2)},
		},
	} {
		ptype := &bq.QueryParameterType{Type: "ARRAY", ArrayType: int64ParamType}
		got, err := convertParamValue(test.pval, ptype)
		if err != nil {
			t.Fatalf("%+v: %v", test.pval, err)
		}
		if !testutil.Equal(got, test.want) {
			t.Errorf("%+v: got %+v, want %+v", test.pval, got, test.want)
		}
	}
	// Structs.
	got, err := convertParamValue(&s1ParamValue, s1ParamType)
	if err != nil {
		t.Fatal(err)
	}
	if !testutil.Equal(got, s1ParamReturnValue) {
		t.Errorf("got %+v, want %+v", got, s1ParamReturnValue)
	}
}

func TestIntegration_ScalarParam(t *testing.T) {
	roundToMicros := cmp.Transformer("RoundToMicros",
		func(t time.Time) time.Time { return t.Round(time.Microsecond) })
	c := getClient(t)
	for _, test := range scalarTests {
		gotData, gotParam, err := paramRoundTrip(c, test.val)
		if err != nil {
			t.Errorf("input %#v errored: %v", test.val, err)
		}
		// first, check the returned query value
		if test.wantNil {
			if gotData != nil {
				t.Errorf("data value %#v expected nil, got %#v", test.val, gotData)
			}
		} else {
			if !testutil.Equal(gotData, test.wantStat, roundToMicros) {
				t.Errorf("\ngot data value %#v (%T)\nwant %#v (%T)", gotData, gotData, test.wantStat, test.wantStat)
			}
		}
		// then, check the stat value
		if !testutil.Equal(gotParam, test.wantStat, roundToMicros) {
			t.Errorf("\ngot param stat %#v (%T)\nwant %#v (%T)", gotParam, gotParam, test.wantStat, test.wantStat)
		}
	}
}

func TestIntegration_OtherParam(t *testing.T) {
	c := getClient(t)
	for _, test := range []struct {
		val       interface{}
		wantData  interface{}
		wantParam interface{}
	}{
		{[]int(nil), []Value(nil), []interface{}(nil)},
		{[]int{}, []Value(nil), []interface{}(nil)},
		{
			[]int{1, 2},
			[]Value{int64(1), int64(2)},
			[]interface{}{int64(1), int64(2)},
		},
		{
			[3]int{1, 2, 3},
			[]Value{int64(1), int64(2), int64(3)},
			[]interface{}{int64(1), int64(2), int64(3)},
		},
		{
			S1{},
			[]Value{int64(0), nil, false},
			map[string]interface{}{
				"A": int64(0),
				"B": nil,
				"C": false,
			},
		},
		{
			s1,
			[]Value{int64(1), []Value{"s"}, true},
			s1ParamReturnValue,
		},
	} {
		gotData, gotParam, err := paramRoundTrip(c, test.val)
		if err != nil {
			t.Fatal(err)
		}
		if !testutil.Equal(gotData, test.wantData) {
			t.Errorf("%#v:\ngot  %#v (%T)\nwant %#v (%T)",
				test.val, gotData, gotData, test.wantData, test.wantData)
		}
		if !testutil.Equal(gotParam, test.wantParam) {
			t.Errorf("%#v:\ngot  %#v (%T)\nwant %#v (%T)",
				test.val, gotParam, gotParam, test.wantParam, test.wantParam)
		}
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

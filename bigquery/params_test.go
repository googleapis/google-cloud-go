// Copyright 2016 Google Inc. All Rights Reserved.
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
	"reflect"
	"testing"
	"time"

	bq "google.golang.org/api/bigquery/v2"
)

var scalarTests = []struct {
	val  interface{}
	want string
}{
	{int64(0), "0"},
	{3.14, "3.14"},
	{3.14159e-87, "3.14159e-87"},
	{true, "true"},
	{"string", "string"},
	{"\u65e5\u672c\u8a9e\n", "\u65e5\u672c\u8a9e\n"},
	{math.NaN(), "NaN"},
	{[]byte("foo"), "Zm9v"}, // base64 encoding of "foo"
	{time.Date(2016, 3, 20, 4, 22, 9, 5000, time.FixedZone("neg1-2", -3720)),
		"2016-03-20 04:22:09.000005-01:02"},
}

func TestParamValueScalar(t *testing.T) {
	for _, test := range scalarTests {
		got, err := paramValue(test.val)
		if err != nil {
			t.Errorf("%v: got %v, want nil", test.val, err)
			continue
		}
		if got.ArrayValues != nil {
			t.Errorf("%v, ArrayValues: got %v, expected nil", test.val, got.ArrayValues)
		}
		if got.StructValues != nil {
			t.Errorf("%v, StructValues: got %v, expected nil", test.val, got.StructValues)
		}
		if got.Value != test.want {
			t.Errorf("%v: got %q, want %q", test.val, got.Value, test.want)
		}
	}
}

func TestParamValueArray(t *testing.T) {
	for _, test := range []struct {
		val  interface{}
		want []string
	}{
		{[]int(nil), []string{}},
		{[]int{}, []string{}},
		{[]int{1, 2}, []string{"1", "2"}},
		{[3]int{1, 2, 3}, []string{"1", "2", "3"}},
	} {
		got, err := paramValue(test.val)
		if err != nil {
			t.Fatal(err)
		}
		var want bq.QueryParameterValue
		for _, s := range test.want {
			want.ArrayValues = append(want.ArrayValues, &bq.QueryParameterValue{Value: s})
		}
		if !reflect.DeepEqual(got, want) {
			t.Errorf("%#v:\ngot  %+v\nwant %+v", test.val, got, want)
		}
	}
}

func TestParamType(t *testing.T) {
	for _, test := range []struct {
		val  interface{}
		want *bq.QueryParameterType
	}{
		{0, int64ParamType},
		{uint32(32767), int64ParamType},
		{3.14, float64ParamType},
		{float32(3.14), float64ParamType},
		{math.NaN(), float64ParamType},
		{true, boolParamType},
		{"", stringParamType},
		{"string", stringParamType},
		{time.Now(), timestampParamType},
		{[]byte("foo"), bytesParamType},
		{[]int{}, &bq.QueryParameterType{Type: "ARRAY", ArrayType: int64ParamType}},
		{[3]bool{}, &bq.QueryParameterType{Type: "ARRAY", ArrayType: boolParamType}},
	} {
		got, err := paramType(reflect.TypeOf(test.val))
		if err != nil {
			t.Fatal(err)
		}
		if !reflect.DeepEqual(got, test.want) {
			t.Errorf("%v (%T): got %v, want %v", test.val, test.val, got, test.want)
		}
	}
}

func TestIntegration_ScalarParam(t *testing.T) {
	c := getClient(t)
	for _, test := range scalarTests {
		got, err := paramRoundTrip(c, test.val)
		if err != nil {
			t.Fatal(err)
		}
		if !equal(got, test.val) {
			t.Errorf("\ngot  %#v (%T)\nwant %#v (%T)", got, got, test.val, test.val)
		}
	}
}

func TestIntegration_ArrayParam(t *testing.T) {
	c := getClient(t)
	for _, test := range []struct {
		val  interface{}
		want interface{}
	}{
		{[]int(nil), []Value(nil)},
		{[]int{}, []Value(nil)},
		{[]int{1, 2}, []Value{int64(1), int64(2)}},
		{[3]int{1, 2, 3}, []Value{int64(1), int64(2), int64(3)}},
	} {
		got, err := paramRoundTrip(c, test.val)
		if err != nil {
			t.Fatal(err)
		}
		if !equal(got, test.want) {
			t.Errorf("\ngot  %#v (%T)\nwant %#v (%T)", got, got, test.want, test.want)
		}
	}
}

func paramRoundTrip(c *Client, x interface{}) (Value, error) {
	q := c.Query("select ?")
	q.Parameters = []QueryParameter{{Value: x}}
	it, err := q.Read(context.Background())
	if err != nil {
		return nil, err
	}
	var val []Value
	err = it.Next(&val)
	if err != nil {
		return nil, err
	}
	if len(val) != 1 {
		return nil, errors.New("wrong number of values")
	}
	return val[0], nil
}

func equal(x1, x2 interface{}) bool {
	if reflect.TypeOf(x1) != reflect.TypeOf(x2) {
		return false
	}
	switch x1 := x1.(type) {
	case float64:
		if math.IsNaN(x1) {
			return math.IsNaN(x2.(float64))
		}
		return x1 == x2
	case time.Time:
		// BigQuery is only accurate to the microsecond.
		return x1.Round(time.Microsecond).Equal(x2.(time.Time).Round(time.Microsecond))
	default:
		return reflect.DeepEqual(x1, x2)
	}
}

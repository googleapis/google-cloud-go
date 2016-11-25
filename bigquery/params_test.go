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
	"bytes"
	"context"
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

func TestParamTypeScalar(t *testing.T) {
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
	} {
		got, err := paramType(test.val)
		if err != nil {
			t.Fatal(err)
		}
		if got != test.want {
			t.Errorf("%v (%T): got %v, want %v", test.val, test.val, got, test.want)
		}
	}
}

func TestIntegration_ScalarParam(t *testing.T) {
	ctx := context.Background()
	c := getClient(t)
	for _, test := range scalarTests {
		q := c.Query("select ?")
		q.Parameters = []QueryParameter{{Value: test.val}}
		it, err := q.Read(ctx)
		if err != nil {
			t.Fatal(err)
		}
		var val []Value
		err = it.Next(&val)
		if err != nil {
			t.Fatal(err)
		}
		if len(val) != 1 {
			t.Fatalf("got %d values, want 1", len(val))
		}
		got := val[0]
		if !equal(got, test.val) {
			t.Errorf("\ngot  %#v (%T)\nwant %#v (%T)", got, got, test.val, test.val)
		}
	}
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
	case []byte:
		return bytes.Equal(x1, x2.([]byte))
	default:
		return x1 == x2
	}
}

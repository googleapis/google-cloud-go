// Copyright 2025 Google LLC
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     https://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package query

import (
	"context"
	"encoding/base64"
	"fmt"
	"math/big"
	"testing"
	"time"

	"cloud.google.com/go/bigquery/v2/apiv2/bigquerypb"
	"cloud.google.com/go/civil"
	"cloud.google.com/go/internal/testutil"
	"google.golang.org/protobuf/types/known/wrapperspb"
)

func TestReadNestedObject(t *testing.T) {
	if len(testClients) == 0 {
		t.Skip("integration tests skipped")
	}
	for k, client := range testClients {
		t.Run(k, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), defaultTestTimeout)
			defer cancel()

			req := client.FromSQL("SELECT 40 as age, [STRUCT(STRUCT('1' as a, '2' as b) as object)] as nested")

			q, err := client.StartQuery(ctx, req)
			if err != nil {
				t.Fatalf("Run() error: %v", err)
			}
			err = q.Wait(ctx)
			if err != nil {
				t.Fatalf("Wait() error: %v", err)
			}

			if !q.Complete() {
				t.Fatalf("expected job to be complete")
			}

			it, err := q.Read(ctx)
			if err != nil {
				t.Fatalf("Read() error: %v", err)
			}

			rows, _ := readRows(ctx, t, it)
			if msg, ok := compareReadMap(rows, []map[string]Value{{
				"age": int64(40),
				"nested": []Value{
					map[string]Value{
						"object": map[string]Value{
							"a": "1",
							"b": "2",
						},
					},
				},
			}}); !ok {
				t.Fatal(msg)
			}

			if rows[0].GetColumnAtIndex(0).String() != "40" {
				t.Fatalf("expected to read `age` column as string")
			}
			if rows[0].GetColumnName("nested").List()[0].Record().GetColumnName("object").Record().GetColumnName("a").String() != "1" {
				t.Fatalf("expected to read `nested.object.a` column as string")
			}
			if rows[0].GetColumnName("nested").List()[0].Record().GetColumnName("object").Record().GetColumnName("b").String() != "2" {
				t.Fatalf("expected to read `nested.object.a` column as string")
			}
		})
	}
}

func TestReadTypes(t *testing.T) {
	if len(testClients) == 0 {
		t.Skip("integration tests skipped")
	}
	for k, client := range testClients {
		t.Run(k, func(t *testing.T) {
			tcs := queryParameterTestCases()
			for _, tc := range tcs {
				t.Run(tc.name, func(t *testing.T) {
					ctx, cancel := context.WithTimeout(context.Background(), defaultTestTimeout)
					defer cancel()

					req := client.FromSQL(tc.query)
					req.QueryRequest.QueryParameters = tc.parameters

					q, err := client.StartQuery(ctx, req)
					if err != nil {
						t.Fatalf("Run() error: %v", err)
					}
					err = q.Wait(ctx)
					if err != nil {
						t.Fatalf("Wait() error: %v", err)
					}

					if !q.Complete() {
						t.Fatalf("expected job to be complete")
					}

					it, err := q.Read(ctx)
					if err != nil {
						t.Fatalf("Read() error: %v", err)
					}

					rows, _ := readRows(ctx, t, it)
					if msg, ok := compareReadMap(rows, []map[string]Value{tc.wantRowMap}); !ok {
						t.Fatal(msg)
					}
					if msg, ok := compareReadValues(rows, [][]Value{tc.wantRowValues}); !ok {
						t.Fatal(msg)
					}
				})
			}
		})
	}
}

type queryParameterTestCase struct {
	name          string
	query         string
	parameters    []*bigquerypb.QueryParameter
	wantRowMap    map[string]Value
	wantRowValues []Value
}

func queryParameterTestCases() []queryParameterTestCase {
	d := civil.Date{Year: 2016, Month: 3, Day: 20}
	tm := civil.Time{Hour: 15, Minute: 04, Second: 05, Nanosecond: 3008}
	rtm := tm
	rtm.Nanosecond = 3000 // round to microseconds
	dtm := civil.DateTime{Date: d, Time: tm}
	dtm.Time.Nanosecond = 3000 // round to microseconds
	ts := time.Date(2016, 3, 20, 15, 04, 05, 0, time.UTC)
	rat := big.NewRat(13, 10)
	nrat := big.NewRat(-13, 10)
	/*rangeTimestamp1 := &RangeValue{
		Start: time.Date(2016, 3, 20, 15, 04, 05, 0, time.UTC),
	}
	rangeTimestamp2 := &RangeValue{
		End: time.Date(2016, 3, 20, 15, 04, 05, 0, time.UTC),
	}

	type ss struct {
		String string
	}

	type s struct {
		Timestamp      time.Time
		StringArray    []string
		SubStruct      ss
		SubStructArray []ss
	}*/

	queryParameterTestCases := []queryParameterTestCase{
		{
			"Int64Param",
			"SELECT @val",
			[]*bigquerypb.QueryParameter{
				{
					Name:          "val",
					ParameterType: &bigquerypb.QueryParameterType{Type: "INT64"},
					ParameterValue: &bigquerypb.QueryParameterValue{
						Value: &wrapperspb.StringValue{
							Value: "1",
						},
					},
				},
			},
			map[string]Value{"f0_": int64(1)},
			[]Value{int64(1)},
		},
		{
			"FloatParam",
			"SELECT @val",
			[]*bigquerypb.QueryParameter{
				{
					Name:          "val",
					ParameterType: &bigquerypb.QueryParameterType{Type: "FLOAT64"},
					ParameterValue: &bigquerypb.QueryParameterValue{
						Value: &wrapperspb.StringValue{
							Value: "1.3",
						},
					},
				},
			},
			map[string]Value{"f0_": 1.3},
			[]Value{1.3},
		},
		{
			"BigRatParam",
			"SELECT @val",
			[]*bigquerypb.QueryParameter{{
				Name:           "val",
				ParameterType:  &bigquerypb.QueryParameterType{Type: "BIGNUMERIC"},
				ParameterValue: &bigquerypb.QueryParameterValue{Value: &wrapperspb.StringValue{Value: bigNumericString(rat)}},
			}},
			map[string]Value{"f0_": rat},
			[]Value{rat},
		},
		{
			"NegativeBigRatParam",
			"SELECT @val",
			[]*bigquerypb.QueryParameter{{Name: "val",
				ParameterType:  &bigquerypb.QueryParameterType{Type: "BIGNUMERIC"},
				ParameterValue: &bigquerypb.QueryParameterValue{Value: &wrapperspb.StringValue{Value: bigNumericString(nrat)}}}},
			map[string]Value{"f0_": nrat},
			[]Value{nrat},
		},
		{
			"BoolParam",
			"SELECT @val",
			[]*bigquerypb.QueryParameter{{Name: "val",
				ParameterType: &bigquerypb.QueryParameterType{
					Type: "BOOL",
				},
				ParameterValue: &bigquerypb.QueryParameterValue{Value: &wrapperspb.StringValue{Value: "true"}}}},
			map[string]Value{"f0_": true},
			[]Value{true},
		},
		{
			"StringParam",
			"SELECT @val",
			[]*bigquerypb.QueryParameter{{Name: "val",
				ParameterType: &bigquerypb.QueryParameterType{
					Type: "STRING",
				},
				ParameterValue: &bigquerypb.QueryParameterValue{Value: &wrapperspb.StringValue{Value: "ABC"}}}},
			map[string]Value{"f0_": "ABC"},
			[]Value{"ABC"},
		},
		{
			"ByteParam",
			"SELECT @val",
			[]*bigquerypb.QueryParameter{
				{
					Name: "val",
					ParameterType: &bigquerypb.QueryParameterType{
						Type: "BYTES",
					},
					ParameterValue: &bigquerypb.QueryParameterValue{
						Value: &wrapperspb.StringValue{
							Value: base64.StdEncoding.EncodeToString([]byte("foo")),
						},
					},
				},
			},
			map[string]Value{"f0_": []byte("foo")},
			[]Value{[]byte("foo")},
		},
		{
			"TimestampParam",
			"SELECT @val",
			[]*bigquerypb.QueryParameter{{Name: "val", ParameterType: &bigquerypb.QueryParameterType{Type: "TIMESTAMP"}, ParameterValue: &bigquerypb.QueryParameterValue{Value: &wrapperspb.StringValue{Value: timestampString(ts)}}}},
			map[string]Value{"f0_": ts},
			[]Value{ts},
		},
		{
			"TimestampArrayParam",
			"SELECT @val",
			[]*bigquerypb.QueryParameter{{
				Name: "val",
				ParameterType: &bigquerypb.QueryParameterType{
					Type: "ARRAY",
					ArrayType: &bigquerypb.QueryParameterType{
						Type: "TIMESTAMP",
					}},
				ParameterValue: &bigquerypb.QueryParameterValue{
					ArrayValues: []*bigquerypb.QueryParameterValue{
						{Value: &wrapperspb.StringValue{Value: timestampString(ts)}},
						{Value: &wrapperspb.StringValue{Value: timestampString(ts)}},
					},
				},
			}},
			map[string]Value{"f0_": []Value{ts, ts}},
			[]Value{[]Value{ts, ts}},
		},
		{
			"DatetimeParam",
			"SELECT @val",
			[]*bigquerypb.QueryParameter{{Name: "val", ParameterType: &bigquerypb.QueryParameterType{Type: "DATETIME"}, ParameterValue: &bigquerypb.QueryParameterValue{Value: &wrapperspb.StringValue{Value: civilDateTimeString(dtm)}}}},
			map[string]Value{"f0_": civil.DateTime{Date: d, Time: rtm}},
			[]Value{civil.DateTime{Date: d, Time: rtm}},
		},
		{
			"DateParam",
			"SELECT @val",
			[]*bigquerypb.QueryParameter{{Name: "val", ParameterType: &bigquerypb.QueryParameterType{Type: "DATE"}, ParameterValue: &bigquerypb.QueryParameterValue{Value: &wrapperspb.StringValue{Value: civilDateString(d)}}}},
			map[string]Value{"f0_": d},
			[]Value{d},
		},
		{
			"TimeParam",
			"SELECT @val",
			[]*bigquerypb.QueryParameter{{Name: "val", ParameterType: &bigquerypb.QueryParameterType{Type: "TIME"}, ParameterValue: &bigquerypb.QueryParameterValue{Value: &wrapperspb.StringValue{Value: civilTimeString(tm)}}}},
			map[string]Value{"f0_": rtm},
			[]Value{rtm},
		},
		{
			"JsonParam",
			"SELECT @val",
			[]*bigquerypb.QueryParameter{
				{
					Name: "val",
					ParameterType: &bigquerypb.QueryParameterType{
						Type: "JSON",
					},
					ParameterValue: &bigquerypb.QueryParameterValue{
						Value: &wrapperspb.StringValue{
							Value: "{\"alpha\":\"beta\"}",
						},
					},
				},
			},
			map[string]Value{"f0_": "{\"alpha\":\"beta\"}"},
			[]Value{"{\"alpha\":\"beta\"}"},
		},
		/*{
			"RangeUnboundedStart",
			"SELECT @val",
			[]*bigquerypb.QueryParameter{
				{
					Name: "val",
					Value: &QueryParameterValue{
						Type: StandardSQLDataType{
							TypeKind: "RANGE",
							RangeElementType: &StandardSQLDataType{
								TypeKind: "TIMESTAMP",
							},
						},
						Value: rangeTimestamp1,
					},
				},
			},
			[]Value{rangeTimestamp1},
			rangeTimestamp1,
		},
		{
			"RangeUnboundedEnd",
			"SELECT @val",
			[]*bigquerypb.QueryParameter{
				{
					Name: "val",
					Value: &QueryParameterValue{
						Type: StandardSQLDataType{
							TypeKind: "RANGE",
							RangeElementType: &StandardSQLDataType{
								TypeKind: "TIMESTAMP",
							},
						},
						Value: rangeTimestamp2,
					},
				},
			},
			[]Value{rangeTimestamp2},
			rangeTimestamp2,
		},
		{
			"RangeArray",
			"SELECT @val",
			[]*bigquerypb.QueryParameter{
				{
					Name: "val",
					Value: &QueryParameterValue{
						Type: StandardSQLDataType{
							ArrayElementType: &StandardSQLDataType{
								TypeKind: "RANGE",
								RangeElementType: &StandardSQLDataType{
									TypeKind: "TIMESTAMP",
								},
							},
						},
						ArrayValue: []*bigquerypb.QueryParameterValue{
							{Value: rangeTimestamp1},
							{Value: rangeTimestamp2},
						},
					},
				},
			},
			[]Value{[]Value{rangeTimestamp1, rangeTimestamp2}},
			[]interface{}{rangeTimestamp1, rangeTimestamp2},
		},*/
		{
			"NestedStructParam",
			"SELECT @val",
			[]*bigquerypb.QueryParameter{
				{
					Name: "val",
					ParameterType: &bigquerypb.QueryParameterType{
						Type: "STRUCT",
						StructTypes: []*bigquerypb.QueryParameterStructType{
							{
								Name: "Datetime",
								Type: &bigquerypb.QueryParameterType{
									Type: "DATETIME",
								},
							},
							{
								Name: "StringArray",
								Type: &bigquerypb.QueryParameterType{
									Type: "ARRAY",
									ArrayType: &bigquerypb.QueryParameterType{
										Type: "STRING",
									},
								},
							},
							{
								Name: "SubStruct",
								Type: &bigquerypb.QueryParameterType{
									Type: "STRUCT",
									StructTypes: []*bigquerypb.QueryParameterStructType{
										{
											Name: "String",
											Type: &bigquerypb.QueryParameterType{
												Type: "STRING",
											},
										},
									},
								},
							},
							{
								Name: "SubStructArray",
								Type: &bigquerypb.QueryParameterType{
									Type: "ARRAY",
									ArrayType: &bigquerypb.QueryParameterType{
										Type: "STRUCT",
										StructTypes: []*bigquerypb.QueryParameterStructType{
											{
												Name: "String",
												Type: &bigquerypb.QueryParameterType{
													Type: "STRING",
												},
											},
										},
									},
								},
							},
						},
					},
					ParameterValue: &bigquerypb.QueryParameterValue{
						StructValues: map[string]*bigquerypb.QueryParameterValue{
							"Datetime": {
								Value: &wrapperspb.StringValue{
									Value: civilDateTimeString(dtm),
								},
							},
							"StringArray": {
								ArrayValues: []*bigquerypb.QueryParameterValue{
									{
										Value: &wrapperspb.StringValue{
											Value: "a",
										},
									},
									{
										Value: &wrapperspb.StringValue{
											Value: "b",
										},
									},
								},
							},
							"SubStruct": {
								StructValues: map[string]*bigquerypb.QueryParameterValue{
									"String": {
										Value: &wrapperspb.StringValue{
											Value: "c",
										},
									},
								},
							},
							"SubStructArray": {
								ArrayValues: []*bigquerypb.QueryParameterValue{
									{
										StructValues: map[string]*bigquerypb.QueryParameterValue{
											"String": {
												Value: &wrapperspb.StringValue{
													Value: "d",
												},
											},
										},
									},
									{
										StructValues: map[string]*bigquerypb.QueryParameterValue{
											"String": {
												Value: &wrapperspb.StringValue{
													Value: "e",
												},
											},
										},
									},
								},
							},
						},
					},
				},
			},
			map[string]Value{
				"f0_": map[string]Value{
					"Datetime":    dtm,
					"StringArray": []Value{"a", "b"},
					"SubStruct": map[string]Value{
						"String": "c",
					},
					"SubStructArray": []Value{
						map[string]Value{
							"String": "d",
						},
						map[string]Value{
							"String": "e",
						},
					},
				},
			},
			[]Value{[]Value{dtm, []Value{"a", "b"}, []Value{"c"}, []Value{[]Value{"d"}, []Value{"e"}}}},
		},
	}

	return queryParameterTestCases
}

func compareReadMap(actual []*Row, want []map[string]Value) (msg string, ok bool) {
	if len(actual) != len(want) {
		return fmt.Sprintf("got %d rows, want %d", len(actual), len(want)), false
	}
	for i, r := range actual {
		gotRow := r.AsMap()
		wantRow := want[i]
		if !testutil.Equal(gotRow, wantRow) {
			return fmt.Sprintf("#%d: got %#v, want %#v", i, gotRow, wantRow), false
		}
	}
	return "", true
}

func compareReadValues(actual []*Row, want [][]Value) (msg string, ok bool) {
	if len(actual) != len(want) {
		return fmt.Sprintf("got %d rows, want %d", len(actual), len(want)), false
	}
	for i, r := range actual {
		gotRow := r.AsValues()
		wantRow := want[i]
		if !testutil.Equal(gotRow, wantRow) {
			return fmt.Sprintf("#%d: got %#v, want %#v", i, gotRow, wantRow), false
		}
	}
	return "", true
}

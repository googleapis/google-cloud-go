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

	storage "cloud.google.com/go/bigquery/storage/apiv1"
	"cloud.google.com/go/bigquery/v2/apiv2/bigquerypb"
	"cloud.google.com/go/civil"
	"cloud.google.com/go/internal/testutil"
	"google.golang.org/protobuf/types/known/wrapperspb"
)

func TestReadNestedObject(t *testing.T) {
	if len(testClients) == 0 {
		t.Skip("integration tests skipped")
	}

	rotcs := readOptionTestCases(t)

	for k, client := range testClients {
		t.Run(k, func(t *testing.T) {
			for _, roc := range rotcs {
				t.Run(roc.name, func(t *testing.T) {
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
					if msg, ok := compareReadMap(rows, []map[string]any{{
						"age": float64(40),
						"nested": []any{
							map[string]any{
								"object": map[string]any{
									"a": "1",
									"b": "2",
								},
							},
						},
					}}); !ok {
						t.Fatal(msg)
					}

					s, err := rows[0].AsStruct()
					if err != nil {
						t.Fatalf("AsStruct() error: %v", err)
					}

					if s.Fields["age"].GetNumberValue() != 40 {
						t.Fatalf("expected to read `age` column as number")
					}
					if s.Fields["nested"].GetListValue().Values[0].GetStructValue().Fields["object"].GetStructValue().Fields["a"].GetStringValue() != "1" {
						t.Fatalf("expected to read `nested.object.a` column as string")
					}
					if s.Fields["nested"].GetListValue().Values[0].GetStructValue().Fields["object"].GetStructValue().Fields["b"].GetStringValue() != "2" {
						t.Fatalf("expected to read `nested.object.b` column as string")
					}

					type MyStruct struct {
						Age    int
						Nested []struct {
							Object struct {
								A string
								B string
							}
						}
					}
					var ms MyStruct
					err = rows[0].Decode(&ms)
					if err != nil {
						t.Fatalf("Decode() error: %v", err)
					}
					if ms.Age != 40 {
						t.Fatalf("expected to read `Age` column as number")
					}
					if ms.Nested[0].Object.A != "1" {
						t.Fatalf("expected to read `nested.object.a` column as string")
					}
					if ms.Nested[0].Object.B != "2" {
						t.Fatalf("expected to read `nested.object.b` column as string")
					}
				})
			}
		})
	}
}

func TestReadTypes(t *testing.T) {
	if len(testClients) == 0 {
		t.Skip("integration tests skipped")
	}

	rotcs := readOptionTestCases(t)
	tcs := queryParameterTestCases()

	for k, client := range testClients {
		t.Run(k, func(t *testing.T) {
			for _, roc := range rotcs {
				t.Run(roc.name, func(t *testing.T) {
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

							it, err := q.Read(ctx, roc.opts...)
							if err != nil {
								t.Fatalf("Read() error: %v", err)
							}

							rows, _ := readRows(ctx, t, it)
							if msg, ok := compareReadMap(rows, []map[string]any{tc.wantRowMap}); !ok {
								t.Fatal(msg)
							}
							if msg, ok := compareReadValues(rows, [][]any{tc.wantRowValues}); !ok {
								t.Fatal(msg)
							}
						})
					}
				})
			}
		})
	}
}

type readOptionTestCase struct {
	name string
	opts []ReadOption
}

func readOptionTestCases(t *testing.T) []readOptionTestCase {
	ctx := context.Background()
	rc, err := storage.NewBigQueryReadClient(ctx)
	if err != nil {
		t.Fatalf("NewBigQueryReadClient() error: %v", err)
	}
	return []readOptionTestCase{
		{
			name: "jobs.query",
			opts: []ReadOption{},
		},
		{
			name: "storage-read-api",
			opts: []ReadOption{WithStorageReadClient(rc)},
		},
	}
}

type queryParameterTestCase struct {
	name          string
	query         string
	parameters    []*bigquerypb.QueryParameter
	wantRowMap    map[string]any
	wantRowValues []any
}

func queryParameterTestCases() []queryParameterTestCase {
	d := civil.Date{Year: 2016, Month: 3, Day: 20}
	ds := civilDateString(d)
	tm := civil.Time{Hour: 15, Minute: 04, Second: 05, Nanosecond: 3008}
	tm.Nanosecond = 3000 // round to microseconds
	tms := civilTimeString(tm)
	dtm := civil.DateTime{Date: d, Time: tm}
	dtm.Time.Nanosecond = 3000 // round to microseconds
	dtms := civilDateTimeString(dtm)
	ts := time.Date(2016, 3, 20, 15, 04, 05, 0, time.UTC)
	tsInt64 := fmt.Sprintf("%v", ts.UnixMicro())
	rat := big.NewRat(13, 10).FloatString(1)
	nrat := big.NewRat(-13, 10).FloatString(1)
	byteValue := base64.StdEncoding.EncodeToString([]byte("foo"))
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
			map[string]any{"f0_": float64(1)},
			[]any{float64(1)},
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
			map[string]any{"f0_": 1.3},
			[]any{1.3},
		},
		{
			"BigRatParam",
			"SELECT @val",
			[]*bigquerypb.QueryParameter{{
				Name:           "val",
				ParameterType:  &bigquerypb.QueryParameterType{Type: "BIGNUMERIC"},
				ParameterValue: &bigquerypb.QueryParameterValue{Value: &wrapperspb.StringValue{Value: rat}},
			}},
			map[string]any{"f0_": rat},
			[]any{rat},
		},
		{
			"NegativeBigRatParam",
			"SELECT @val",
			[]*bigquerypb.QueryParameter{{Name: "val",
				ParameterType:  &bigquerypb.QueryParameterType{Type: "BIGNUMERIC"},
				ParameterValue: &bigquerypb.QueryParameterValue{Value: &wrapperspb.StringValue{Value: nrat}}}},
			map[string]any{"f0_": nrat},
			[]any{nrat},
		},
		{
			"BoolParam",
			"SELECT @val",
			[]*bigquerypb.QueryParameter{{Name: "val",
				ParameterType: &bigquerypb.QueryParameterType{
					Type: "BOOL",
				},
				ParameterValue: &bigquerypb.QueryParameterValue{Value: &wrapperspb.StringValue{Value: "true"}}}},
			map[string]any{"f0_": true},
			[]any{true},
		},
		{
			"StringParam",
			"SELECT @val",
			[]*bigquerypb.QueryParameter{{Name: "val",
				ParameterType: &bigquerypb.QueryParameterType{
					Type: "STRING",
				},
				ParameterValue: &bigquerypb.QueryParameterValue{Value: &wrapperspb.StringValue{Value: "ABC"}}}},
			map[string]any{"f0_": "ABC"},
			[]any{"ABC"},
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
							Value: byteValue,
						},
					},
				},
			},
			map[string]any{"f0_": byteValue},
			[]any{byteValue},
		},
		{
			"TimestampParam",
			"SELECT @val",
			[]*bigquerypb.QueryParameter{{Name: "val", ParameterType: &bigquerypb.QueryParameterType{Type: "TIMESTAMP"}, ParameterValue: &bigquerypb.QueryParameterValue{Value: &wrapperspb.StringValue{Value: timestampString(ts)}}}},
			map[string]any{"f0_": tsInt64},
			[]any{tsInt64},
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
			map[string]any{"f0_": []any{tsInt64, tsInt64}},
			[]any{[]any{tsInt64, tsInt64}},
		},
		{
			"DatetimeParam",
			"SELECT @val",
			[]*bigquerypb.QueryParameter{{Name: "val", ParameterType: &bigquerypb.QueryParameterType{Type: "DATETIME"}, ParameterValue: &bigquerypb.QueryParameterValue{Value: &wrapperspb.StringValue{Value: dtms}}}},
			map[string]any{"f0_": dtms},
			[]any{dtms},
		},
		{
			"DateParam",
			"SELECT @val",
			[]*bigquerypb.QueryParameter{{Name: "val", ParameterType: &bigquerypb.QueryParameterType{Type: "DATE"}, ParameterValue: &bigquerypb.QueryParameterValue{Value: &wrapperspb.StringValue{Value: ds}}}},
			map[string]any{"f0_": ds},
			[]any{ds},
		},
		{
			"TimeParam",
			"SELECT @val",
			[]*bigquerypb.QueryParameter{{Name: "val", ParameterType: &bigquerypb.QueryParameterType{Type: "TIME"}, ParameterValue: &bigquerypb.QueryParameterValue{Value: &wrapperspb.StringValue{Value: tms}}}},
			map[string]any{"f0_": tms},
			[]any{tms},
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
			map[string]any{"f0_": "{\"alpha\":\"beta\"}"},
			[]any{"{\"alpha\":\"beta\"}"},
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
			map[string]any{
				"f0_": map[string]any{
					"Datetime":    dtms,
					"StringArray": []any{"a", "b"},
					"SubStruct": map[string]any{
						"String": "c",
					},
					"SubStructArray": []any{
						map[string]any{
							"String": "d",
						},
						map[string]any{
							"String": "e",
						},
					},
				},
			},
			[]any{[]any{dtms, []any{"a", "b"}, []any{"c"}, []any{[]any{"d"}, []any{"e"}}}},
		},
	}

	return queryParameterTestCases
}

func compareReadMap(actual []*Row, want []map[string]any) (msg string, ok bool) {
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

func compareReadValues(actual []*Row, want [][]any) (msg string, ok bool) {
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

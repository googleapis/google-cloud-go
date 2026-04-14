// Copyright 2026 Google LLC
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

package firestore

import (
	"testing"

	pb "cloud.google.com/go/firestore/apiv1/firestorepb"
	"cloud.google.com/go/internal/testutil"
	"google.golang.org/genproto/googleapis/type/latlng"
)

func TestTruncFunctions(t *testing.T) {
	testcases := []struct {
		desc string
		expr Expression
		want *pb.Value
	}{
		{
			desc: "Trunc",
			expr: Trunc("field"),
			want: &pb.Value{ValueType: &pb.Value_FunctionValue{
				FunctionValue: &pb.Function{
					Name: "trunc",
					Args: []*pb.Value{
						{ValueType: &pb.Value_FieldReferenceValue{FieldReferenceValue: "field"}},
					},
				},
			}},
		},
		{
			desc: "TruncToPrecision",
			expr: TruncToPrecision("field", 2),
			want: &pb.Value{ValueType: &pb.Value_FunctionValue{
				FunctionValue: &pb.Function{
					Name: "trunc",
					Args: []*pb.Value{
						{ValueType: &pb.Value_FieldReferenceValue{FieldReferenceValue: "field"}},
						{ValueType: &pb.Value_IntegerValue{IntegerValue: 2}},
					},
				},
			}},
		},
		{
			desc: "baseExpression Trunc",
			expr: FieldOf("field").Trunc(),
			want: &pb.Value{ValueType: &pb.Value_FunctionValue{
				FunctionValue: &pb.Function{
					Name: "trunc",
					Args: []*pb.Value{
						{ValueType: &pb.Value_FieldReferenceValue{FieldReferenceValue: "field"}},
					},
				},
			}},
		},
		{
			desc: "baseExpression TruncToPrecision",
			expr: FieldOf("field").TruncToPrecision(3),
			want: &pb.Value{ValueType: &pb.Value_FunctionValue{
				FunctionValue: &pb.Function{
					Name: "trunc",
					Args: []*pb.Value{
						{ValueType: &pb.Value_FieldReferenceValue{FieldReferenceValue: "field"}},
						{ValueType: &pb.Value_IntegerValue{IntegerValue: 3}},
					},
				},
			}},
		},
	}

	for _, tc := range testcases {
		t.Run(tc.desc, func(t *testing.T) {
			got, err := tc.expr.toProto()
			if err != nil {
				t.Fatalf("toProto() failed: %v", err)
			}
			if diff := testutil.Diff(got, tc.want); diff != "" {
				t.Errorf("toProto() returned diff (-got +want): %s", diff)
			}
		})
	}
}

func TestLogicalFunctions(t *testing.T) {
	testcases := []struct {
		desc string
		expr Expression
		want *pb.Value
	}{
		{
			desc: "And",
			expr: And(FieldOf("a").Equal(1), FieldOf("b").Equal(2)),
			want: &pb.Value{ValueType: &pb.Value_FunctionValue{
				FunctionValue: &pb.Function{
					Name: "and",
					Args: []*pb.Value{
						{ValueType: &pb.Value_FunctionValue{
							FunctionValue: &pb.Function{
								Name: "equal",
								Args: []*pb.Value{
									{ValueType: &pb.Value_FieldReferenceValue{FieldReferenceValue: "a"}},
									{ValueType: &pb.Value_IntegerValue{IntegerValue: 1}},
								},
							},
						}},
						{ValueType: &pb.Value_FunctionValue{
							FunctionValue: &pb.Function{
								Name: "equal",
								Args: []*pb.Value{
									{ValueType: &pb.Value_FieldReferenceValue{FieldReferenceValue: "b"}},
									{ValueType: &pb.Value_IntegerValue{IntegerValue: 2}},
								},
							},
						}},
					},
				},
			}},
		},
		{
			desc: "Or",
			expr: Or(FieldOf("a").Equal(1), FieldOf("b").Equal(2)),
			want: &pb.Value{ValueType: &pb.Value_FunctionValue{
				FunctionValue: &pb.Function{
					Name: "or",
					Args: []*pb.Value{
						{ValueType: &pb.Value_FunctionValue{
							FunctionValue: &pb.Function{
								Name: "equal",
								Args: []*pb.Value{
									{ValueType: &pb.Value_FieldReferenceValue{FieldReferenceValue: "a"}},
									{ValueType: &pb.Value_IntegerValue{IntegerValue: 1}},
								},
							},
						}},
						{ValueType: &pb.Value_FunctionValue{
							FunctionValue: &pb.Function{
								Name: "equal",
								Args: []*pb.Value{
									{ValueType: &pb.Value_FieldReferenceValue{FieldReferenceValue: "b"}},
									{ValueType: &pb.Value_IntegerValue{IntegerValue: 2}},
								},
							},
						}},
					},
				},
			}},
		},
		{
			desc: "Nor",
			expr: Nor(FieldOf("a").Equal(1), FieldOf("b").Equal(2)),
			want: &pb.Value{ValueType: &pb.Value_FunctionValue{
				FunctionValue: &pb.Function{
					Name: "nor",
					Args: []*pb.Value{
						{ValueType: &pb.Value_FunctionValue{
							FunctionValue: &pb.Function{
								Name: "equal",
								Args: []*pb.Value{
									{ValueType: &pb.Value_FieldReferenceValue{FieldReferenceValue: "a"}},
									{ValueType: &pb.Value_IntegerValue{IntegerValue: 1}},
								},
							},
						}},
						{ValueType: &pb.Value_FunctionValue{
							FunctionValue: &pb.Function{
								Name: "equal",
								Args: []*pb.Value{
									{ValueType: &pb.Value_FieldReferenceValue{FieldReferenceValue: "b"}},
									{ValueType: &pb.Value_IntegerValue{IntegerValue: 2}},
								},
							},
						}},
					},
				},
			}},
		},
		{
			desc: "IfNull",
			expr: IfNull(FieldOf("a"), 1, 2),
			want: &pb.Value{ValueType: &pb.Value_FunctionValue{
				FunctionValue: &pb.Function{
					Name: "if_null",
					Args: []*pb.Value{
						{ValueType: &pb.Value_FieldReferenceValue{FieldReferenceValue: "a"}},
						{ValueType: &pb.Value_IntegerValue{IntegerValue: 1}},
						{ValueType: &pb.Value_IntegerValue{IntegerValue: 2}},
					},
				},
			}},
		},
		{
			desc: "baseExpression IfNull",
			expr: FieldOf("a").IfNull(1),
			want: &pb.Value{ValueType: &pb.Value_FunctionValue{
				FunctionValue: &pb.Function{
					Name: "if_null",
					Args: []*pb.Value{
						{ValueType: &pb.Value_FieldReferenceValue{FieldReferenceValue: "a"}},
						{ValueType: &pb.Value_IntegerValue{IntegerValue: 1}},
					},
				},
			}},
		},
		{
			desc: "Switch",
			expr: SwitchOn(FieldOf("a").Equal(1), "one", FieldOf("a").Equal(2), "two", "other"),
			want: &pb.Value{ValueType: &pb.Value_FunctionValue{
				FunctionValue: &pb.Function{
					Name: "switch_on",
					Args: []*pb.Value{
						{ValueType: &pb.Value_FunctionValue{
							FunctionValue: &pb.Function{
								Name: "equal",
								Args: []*pb.Value{
									{ValueType: &pb.Value_FieldReferenceValue{FieldReferenceValue: "a"}},
									{ValueType: &pb.Value_IntegerValue{IntegerValue: 1}},
								},
							},
						}},
						{ValueType: &pb.Value_StringValue{StringValue: "one"}},
						{ValueType: &pb.Value_FunctionValue{
							FunctionValue: &pb.Function{
								Name: "equal",
								Args: []*pb.Value{
									{ValueType: &pb.Value_FieldReferenceValue{FieldReferenceValue: "a"}},
									{ValueType: &pb.Value_IntegerValue{IntegerValue: 2}},
								},
							},
						}},
						{ValueType: &pb.Value_StringValue{StringValue: "two"}},
						{ValueType: &pb.Value_StringValue{StringValue: "other"}},
					},
				},
			}},
		},
	}

	for _, tc := range testcases {
		t.Run(tc.desc, func(t *testing.T) {
			got, err := tc.expr.toProto()
			if err != nil {
				t.Fatalf("toProto() failed: %v", err)
			}
			if diff := testutil.Diff(got, tc.want); diff != "" {
				t.Errorf("toProto() returned diff (-got +want): %s", diff)
			}
		})
	}
}

func TestMapFunctions(t *testing.T) {
	testcases := []struct {
		desc string
		expr Expression
		want *pb.Value
	}{
		{
			desc: "MapSet",
			expr: MapSet("field", "k1", "v1", "k2", 2),
			want: &pb.Value{ValueType: &pb.Value_FunctionValue{
				FunctionValue: &pb.Function{
					Name: "map_set",
					Args: []*pb.Value{
						{ValueType: &pb.Value_FieldReferenceValue{FieldReferenceValue: "field"}},
						{ValueType: &pb.Value_StringValue{StringValue: "k1"}},
						{ValueType: &pb.Value_StringValue{StringValue: "v1"}},
						{ValueType: &pb.Value_StringValue{StringValue: "k2"}},
						{ValueType: &pb.Value_IntegerValue{IntegerValue: 2}},
					},
				},
			}},
		},
		{
			desc: "baseExpression MapSet",
			expr: FieldOf("field").MapSet("k1", "v1"),
			want: &pb.Value{ValueType: &pb.Value_FunctionValue{
				FunctionValue: &pb.Function{
					Name: "map_set",
					Args: []*pb.Value{
						{ValueType: &pb.Value_FieldReferenceValue{FieldReferenceValue: "field"}},
						{ValueType: &pb.Value_StringValue{StringValue: "k1"}},
						{ValueType: &pb.Value_StringValue{StringValue: "v1"}},
					},
				},
			}},
		},
		{
			desc: "MapKeys",
			expr: MapKeys("field"),
			want: &pb.Value{ValueType: &pb.Value_FunctionValue{
				FunctionValue: &pb.Function{
					Name: "map_keys",
					Args: []*pb.Value{
						{ValueType: &pb.Value_FieldReferenceValue{FieldReferenceValue: "field"}},
					},
				},
			}},
		},
		{
			desc: "baseExpression MapKeys",
			expr: FieldOf("field").MapKeys(),
			want: &pb.Value{ValueType: &pb.Value_FunctionValue{
				FunctionValue: &pb.Function{
					Name: "map_keys",
					Args: []*pb.Value{
						{ValueType: &pb.Value_FieldReferenceValue{FieldReferenceValue: "field"}},
					},
				},
			}},
		},
		{
			desc: "MapValues",
			expr: MapValues("field"),
			want: &pb.Value{ValueType: &pb.Value_FunctionValue{
				FunctionValue: &pb.Function{
					Name: "map_values",
					Args: []*pb.Value{
						{ValueType: &pb.Value_FieldReferenceValue{FieldReferenceValue: "field"}},
					},
				},
			}},
		},
		{
			desc: "baseExpression MapValues",
			expr: FieldOf("field").MapValues(),
			want: &pb.Value{ValueType: &pb.Value_FunctionValue{
				FunctionValue: &pb.Function{
					Name: "map_values",
					Args: []*pb.Value{
						{ValueType: &pb.Value_FieldReferenceValue{FieldReferenceValue: "field"}},
					},
				},
			}},
		},
		{
			desc: "MapEntries",
			expr: MapEntries("field"),
			want: &pb.Value{ValueType: &pb.Value_FunctionValue{
				FunctionValue: &pb.Function{
					Name: "map_entries",
					Args: []*pb.Value{
						{ValueType: &pb.Value_FieldReferenceValue{FieldReferenceValue: "field"}},
					},
				},
			}},
		},
		{
			desc: "baseExpression MapEntries",
			expr: FieldOf("field").MapEntries(),
			want: &pb.Value{ValueType: &pb.Value_FunctionValue{
				FunctionValue: &pb.Function{
					Name: "map_entries",
					Args: []*pb.Value{
						{ValueType: &pb.Value_FieldReferenceValue{FieldReferenceValue: "field"}},
					},
				},
			}},
		},
	}

	for _, tc := range testcases {
		t.Run(tc.desc, func(t *testing.T) {
			got, err := tc.expr.toProto()
			if err != nil {
				t.Fatalf("toProto() failed: %v", err)
			}
			if diff := testutil.Diff(got, tc.want); diff != "" {
				t.Errorf("toProto() returned diff (-got +want): %s", diff)
			}
		})
	}
}

func TestStringFunctions(t *testing.T) {
	testcases := []struct {
		desc string
		expr Expression
		want *pb.Value
	}{
		{
			desc: "RegexFind",
			expr: RegexFind("field", "pattern"),
			want: &pb.Value{ValueType: &pb.Value_FunctionValue{
				FunctionValue: &pb.Function{
					Name: "regex_find",
					Args: []*pb.Value{
						{ValueType: &pb.Value_FieldReferenceValue{FieldReferenceValue: "field"}},
						{ValueType: &pb.Value_StringValue{StringValue: "pattern"}},
					},
				},
			}},
		},
		{
			desc: "baseExpression RegexFind",
			expr: FieldOf("field").RegexFind("pattern"),
			want: &pb.Value{ValueType: &pb.Value_FunctionValue{
				FunctionValue: &pb.Function{
					Name: "regex_find",
					Args: []*pb.Value{
						{ValueType: &pb.Value_FieldReferenceValue{FieldReferenceValue: "field"}},
						{ValueType: &pb.Value_StringValue{StringValue: "pattern"}},
					},
				},
			}},
		},
		{
			desc: "RegexFindAll",
			expr: RegexFindAll("field", "pattern"),
			want: &pb.Value{ValueType: &pb.Value_FunctionValue{
				FunctionValue: &pb.Function{
					Name: "regex_find_all",
					Args: []*pb.Value{
						{ValueType: &pb.Value_FieldReferenceValue{FieldReferenceValue: "field"}},
						{ValueType: &pb.Value_StringValue{StringValue: "pattern"}},
					},
				},
			}},
		},
		{
			desc: "baseExpression RegexFindAll",
			expr: FieldOf("field").RegexFindAll("pattern"),
			want: &pb.Value{ValueType: &pb.Value_FunctionValue{
				FunctionValue: &pb.Function{
					Name: "regex_find_all",
					Args: []*pb.Value{
						{ValueType: &pb.Value_FieldReferenceValue{FieldReferenceValue: "field"}},
						{ValueType: &pb.Value_StringValue{StringValue: "pattern"}},
					},
				},
			}},
		},
		{
			desc: "StringRepeat",
			expr: StringRepeat("field", 3),
			want: &pb.Value{ValueType: &pb.Value_FunctionValue{
				FunctionValue: &pb.Function{
					Name: "string_repeat",
					Args: []*pb.Value{
						{ValueType: &pb.Value_FieldReferenceValue{FieldReferenceValue: "field"}},
						{ValueType: &pb.Value_IntegerValue{IntegerValue: 3}},
					},
				},
			}},
		},
		{
			desc: "baseExpression StringRepeat",
			expr: FieldOf("field").StringRepeat(2),
			want: &pb.Value{ValueType: &pb.Value_FunctionValue{
				FunctionValue: &pb.Function{
					Name: "string_repeat",
					Args: []*pb.Value{
						{ValueType: &pb.Value_FieldReferenceValue{FieldReferenceValue: "field"}},
						{ValueType: &pb.Value_IntegerValue{IntegerValue: 2}},
					},
				},
			}},
		},
		{
			desc: "StringReplaceOne",
			expr: StringReplaceOne("field", "old", "new"),
			want: &pb.Value{ValueType: &pb.Value_FunctionValue{
				FunctionValue: &pb.Function{
					Name: "string_replace_one",
					Args: []*pb.Value{
						{ValueType: &pb.Value_FieldReferenceValue{FieldReferenceValue: "field"}},
						{ValueType: &pb.Value_StringValue{StringValue: "old"}},
						{ValueType: &pb.Value_StringValue{StringValue: "new"}},
					},
				},
			}},
		},
		{
			desc: "baseExpression StringReplaceOne",
			expr: FieldOf("field").StringReplaceOne("old", "new"),
			want: &pb.Value{ValueType: &pb.Value_FunctionValue{
				FunctionValue: &pb.Function{
					Name: "string_replace_one",
					Args: []*pb.Value{
						{ValueType: &pb.Value_FieldReferenceValue{FieldReferenceValue: "field"}},
						{ValueType: &pb.Value_StringValue{StringValue: "old"}},
						{ValueType: &pb.Value_StringValue{StringValue: "new"}},
					},
				},
			}},
		},
		{
			desc: "StringReplaceAll",
			expr: StringReplaceAll("field", "old", "new"),
			want: &pb.Value{ValueType: &pb.Value_FunctionValue{
				FunctionValue: &pb.Function{
					Name: "string_replace_all",
					Args: []*pb.Value{
						{ValueType: &pb.Value_FieldReferenceValue{FieldReferenceValue: "field"}},
						{ValueType: &pb.Value_StringValue{StringValue: "old"}},
						{ValueType: &pb.Value_StringValue{StringValue: "new"}},
					},
				},
			}},
		},
		{
			desc: "baseExpression StringReplaceAll",
			expr: FieldOf("field").StringReplaceAll("old", "new"),
			want: &pb.Value{ValueType: &pb.Value_FunctionValue{
				FunctionValue: &pb.Function{
					Name: "string_replace_all",
					Args: []*pb.Value{
						{ValueType: &pb.Value_FieldReferenceValue{FieldReferenceValue: "field"}},
						{ValueType: &pb.Value_StringValue{StringValue: "old"}},
						{ValueType: &pb.Value_StringValue{StringValue: "new"}},
					},
				},
			}},
		},
		{
			desc: "StringIndexOf",
			expr: StringIndexOf("field", "search"),
			want: &pb.Value{ValueType: &pb.Value_FunctionValue{
				FunctionValue: &pb.Function{
					Name: "string_index_of",
					Args: []*pb.Value{
						{ValueType: &pb.Value_FieldReferenceValue{FieldReferenceValue: "field"}},
						{ValueType: &pb.Value_StringValue{StringValue: "search"}},
					},
				},
			}},
		},
		{
			desc: "baseExpression StringIndexOf",
			expr: FieldOf("field").StringIndexOf("search"),
			want: &pb.Value{ValueType: &pb.Value_FunctionValue{
				FunctionValue: &pb.Function{
					Name: "string_index_of",
					Args: []*pb.Value{
						{ValueType: &pb.Value_FieldReferenceValue{FieldReferenceValue: "field"}},
						{ValueType: &pb.Value_StringValue{StringValue: "search"}},
					},
				},
			}},
		},
		{
			desc: "LTrim",
			expr: LTrim("field"),
			want: &pb.Value{ValueType: &pb.Value_FunctionValue{
				FunctionValue: &pb.Function{
					Name: "ltrim",
					Args: []*pb.Value{
						{ValueType: &pb.Value_FieldReferenceValue{FieldReferenceValue: "field"}},
					},
				},
			}},
		},
		{
			desc: "LTrimValue",
			expr: LTrimValue("field", "abc"),
			want: &pb.Value{ValueType: &pb.Value_FunctionValue{
				FunctionValue: &pb.Function{
					Name: "ltrim",
					Args: []*pb.Value{
						{ValueType: &pb.Value_FieldReferenceValue{FieldReferenceValue: "field"}},
						{ValueType: &pb.Value_StringValue{StringValue: "abc"}},
					},
				},
			}},
		},
		{
			desc: "baseExpression LTrim",
			expr: FieldOf("field").LTrim(),
			want: &pb.Value{ValueType: &pb.Value_FunctionValue{
				FunctionValue: &pb.Function{
					Name: "ltrim",
					Args: []*pb.Value{
						{ValueType: &pb.Value_FieldReferenceValue{FieldReferenceValue: "field"}},
					},
				},
			}},
		},
		{
			desc: "RTrim",
			expr: RTrim("field"),
			want: &pb.Value{ValueType: &pb.Value_FunctionValue{
				FunctionValue: &pb.Function{
					Name: "rtrim",
					Args: []*pb.Value{
						{ValueType: &pb.Value_FieldReferenceValue{FieldReferenceValue: "field"}},
					},
				},
			}},
		},
		{
			desc: "RTrimValue",
			expr: RTrimValue("field", "abc"),
			want: &pb.Value{ValueType: &pb.Value_FunctionValue{
				FunctionValue: &pb.Function{
					Name: "rtrim",
					Args: []*pb.Value{
						{ValueType: &pb.Value_FieldReferenceValue{FieldReferenceValue: "field"}},
						{ValueType: &pb.Value_StringValue{StringValue: "abc"}},
					},
				},
			}},
		},
		{
			desc: "baseExpression RTrim",
			expr: FieldOf("field").RTrim(),
			want: &pb.Value{ValueType: &pb.Value_FunctionValue{
				FunctionValue: &pb.Function{
					Name: "rtrim",
					Args: []*pb.Value{
						{ValueType: &pb.Value_FieldReferenceValue{FieldReferenceValue: "field"}},
					},
				},
			}},
		},
	}

	for _, tc := range testcases {
		t.Run(tc.desc, func(t *testing.T) {
			got, err := tc.expr.toProto()
			if err != nil {
				t.Fatalf("toProto() failed: %v", err)
			}
			if diff := testutil.Diff(got, tc.want); diff != "" {
				t.Errorf("toProto() returned diff (-got +want): %s", diff)
			}
		})
	}
}

func TestCurrentFunctions(t *testing.T) {
	testcases := []struct {
		desc string
		expr Expression
		want *pb.Value
	}{
		{
			desc: "CurrentTimestamp",
			expr: CurrentTimestamp(),
			want: &pb.Value{ValueType: &pb.Value_FunctionValue{
				FunctionValue: &pb.Function{
					Name: "current_timestamp",
					Args: []*pb.Value{},
				},
			}},
		},
		{
			desc: "CurrentDocument",
			expr: CurrentDocument(),
			want: &pb.Value{ValueType: &pb.Value_FunctionValue{
				FunctionValue: &pb.Function{
					Name: "current_document",
					Args: []*pb.Value{},
				},
			}},
		},
	}

	for _, tc := range testcases {
		t.Run(tc.desc, func(t *testing.T) {
			got, err := tc.expr.toProto()
			if err != nil {
				t.Fatalf("toProto() failed: %v", err)
			}
			if diff := testutil.Diff(got, tc.want); diff != "" {
				t.Errorf("toProto() returned diff (-got +want): %s", diff)
			}
		})
	}
}

func TestKeyFunctions(t *testing.T) {
	testcases := []struct {
		desc string
		expr Expression
		want *pb.Value
	}{
		{
			desc: "GetCollectionID",
			expr: GetCollectionID("field"),
			want: &pb.Value{ValueType: &pb.Value_FunctionValue{
				FunctionValue: &pb.Function{
					Name: "collection_id",
					Args: []*pb.Value{
						{ValueType: &pb.Value_FieldReferenceValue{FieldReferenceValue: "field"}},
					},
				},
			}},
		},
		{
			desc: "GetDocumentID",
			expr: GetDocumentID("field"),
			want: &pb.Value{ValueType: &pb.Value_FunctionValue{
				FunctionValue: &pb.Function{
					Name: "document_id",
					Args: []*pb.Value{
						{ValueType: &pb.Value_StringValue{StringValue: "field"}},
					},
				},
			}},
		},
	}

	for _, tc := range testcases {
		t.Run(tc.desc, func(t *testing.T) {
			got, err := tc.expr.toProto()
			if err != nil {
				t.Fatalf("toProto() failed: %v", err)
			}
			if diff := testutil.Diff(got, tc.want); diff != "" {
				t.Errorf("toProto() returned diff (-got +want): %s", diff)
			}
		})
	}
}

func TestMathFunctions(t *testing.T) {
	testcases := []struct {
		desc string
		expr Expression
		want *pb.Value
	}{
		{
			desc: "Sqrt",
			expr: Sqrt("field"),
			want: &pb.Value{ValueType: &pb.Value_FunctionValue{
				FunctionValue: &pb.Function{
					Name: "sqrt",
					Args: []*pb.Value{
						{ValueType: &pb.Value_FieldReferenceValue{FieldReferenceValue: "field"}},
					},
				},
			}},
		},
		{
			desc: "Cmp",
			expr: Cmp("left", "right"),
			want: &pb.Value{ValueType: &pb.Value_FunctionValue{
				FunctionValue: &pb.Function{
					Name: "cmp",
					Args: []*pb.Value{
						{ValueType: &pb.Value_FieldReferenceValue{FieldReferenceValue: "left"}},
						{ValueType: &pb.Value_StringValue{StringValue: "right"}},
					},
				},
			}},
		},
		{
			desc: "baseExpression Cmp",
			expr: FieldOf("left").Cmp(10),
			want: &pb.Value{ValueType: &pb.Value_FunctionValue{
				FunctionValue: &pb.Function{
					Name: "cmp",
					Args: []*pb.Value{
						{ValueType: &pb.Value_FieldReferenceValue{FieldReferenceValue: "left"}},
						{ValueType: &pb.Value_IntegerValue{IntegerValue: 10}},
					},
				},
			}},
		},
	}

	for _, tc := range testcases {
		t.Run(tc.desc, func(t *testing.T) {
			got, err := tc.expr.toProto()
			if err != nil {
				t.Fatalf("toProto() failed: %v", err)
			}
			if diff := testutil.Diff(got, tc.want); diff != "" {
				t.Errorf("toProto() returned diff (-got +want): %s", diff)
			}
		})
	}
}

func TestTimestampFunctions(t *testing.T) {
	testcases := []struct {
		desc string
		expr Expression
		want *pb.Value
	}{
		{
			desc: "TimestampExtract",
			expr: TimestampExtract("field", "year"),
			want: &pb.Value{ValueType: &pb.Value_FunctionValue{
				FunctionValue: &pb.Function{
					Name: "timestamp_extract",
					Args: []*pb.Value{
						{ValueType: &pb.Value_FieldReferenceValue{FieldReferenceValue: "field"}},
						{ValueType: &pb.Value_StringValue{StringValue: "year"}},
					},
				},
			}},
		},
		{
			desc: "TimestampExtractWithTimezone",
			expr: TimestampExtractWithTimezone("field", "year", "America/Los_Angeles"),
			want: &pb.Value{ValueType: &pb.Value_FunctionValue{
				FunctionValue: &pb.Function{
					Name: "timestamp_extract",
					Args: []*pb.Value{
						{ValueType: &pb.Value_FieldReferenceValue{FieldReferenceValue: "field"}},
						{ValueType: &pb.Value_StringValue{StringValue: "year"}},
						{ValueType: &pb.Value_StringValue{StringValue: "America/Los_Angeles"}},
					},
				},
			}},
		},
		{
			desc: "baseExpression TimestampExtract",
			expr: FieldOf("field").TimestampExtract("month"),
			want: &pb.Value{ValueType: &pb.Value_FunctionValue{
				FunctionValue: &pb.Function{
					Name: "timestamp_extract",
					Args: []*pb.Value{
						{ValueType: &pb.Value_FieldReferenceValue{FieldReferenceValue: "field"}},
						{ValueType: &pb.Value_StringValue{StringValue: "month"}},
					},
				},
			}},
		},
		{
			desc: "baseExpression TimestampExtractWithTimezone",
			expr: FieldOf("field").TimestampExtractWithTimezone("month", "UTC"),
			want: &pb.Value{ValueType: &pb.Value_FunctionValue{
				FunctionValue: &pb.Function{
					Name: "timestamp_extract",
					Args: []*pb.Value{
						{ValueType: &pb.Value_FieldReferenceValue{FieldReferenceValue: "field"}},
						{ValueType: &pb.Value_StringValue{StringValue: "month"}},
						{ValueType: &pb.Value_StringValue{StringValue: "UTC"}},
					},
				},
			}},
		},
		{
			desc: "TimestampDiff",
			expr: TimestampDiff("end", "start", "day"),
			want: &pb.Value{ValueType: &pb.Value_FunctionValue{
				FunctionValue: &pb.Function{
					Name: "timestamp_diff",
					Args: []*pb.Value{
						{ValueType: &pb.Value_FieldReferenceValue{FieldReferenceValue: "end"}},
						{ValueType: &pb.Value_FieldReferenceValue{FieldReferenceValue: "start"}},
						{ValueType: &pb.Value_StringValue{StringValue: "day"}},
					},
				},
			}},
		},
		{
			desc: "baseExpression TimestampDiff",
			expr: FieldOf("end").TimestampDiff("start", "hour"),
			want: &pb.Value{ValueType: &pb.Value_FunctionValue{
				FunctionValue: &pb.Function{
					Name: "timestamp_diff",
					Args: []*pb.Value{
						{ValueType: &pb.Value_FieldReferenceValue{FieldReferenceValue: "end"}},
						{ValueType: &pb.Value_FieldReferenceValue{FieldReferenceValue: "start"}},
						{ValueType: &pb.Value_StringValue{StringValue: "hour"}},
					},
				},
			}},
		},
	}

	for _, tc := range testcases {
		t.Run(tc.desc, func(t *testing.T) {
			got, err := tc.expr.toProto()
			if err != nil {
				t.Fatalf("toProto() failed: %v", err)
			}
			if diff := testutil.Diff(got, tc.want); diff != "" {
				t.Errorf("toProto() returned diff (-got +want): %s", diff)
			}
		})
	}
}

func TestArrayFunctions(t *testing.T) {
	testcases := []struct {
		desc string
		expr Expression
		want *pb.Value
	}{
		{
			desc: "ArrayMaximumN",
			expr: ArrayMaximumN("field", 3),
			want: &pb.Value{ValueType: &pb.Value_FunctionValue{
				FunctionValue: &pb.Function{
					Name: "maximum_n",
					Args: []*pb.Value{
						{ValueType: &pb.Value_FieldReferenceValue{FieldReferenceValue: "field"}},
						{ValueType: &pb.Value_IntegerValue{IntegerValue: 3}},
					},
				},
			}},
		},
		{
			desc: "ArrayMinimumN",
			expr: ArrayMinimumN("field", 2),
			want: &pb.Value{ValueType: &pb.Value_FunctionValue{
				FunctionValue: &pb.Function{
					Name: "minimum_n",
					Args: []*pb.Value{
						{ValueType: &pb.Value_FieldReferenceValue{FieldReferenceValue: "field"}},
						{ValueType: &pb.Value_IntegerValue{IntegerValue: 2}},
					},
				},
			}},
		},
		{
			desc: "ArrayFirst",
			expr: ArrayFirst("field"),
			want: &pb.Value{ValueType: &pb.Value_FunctionValue{
				FunctionValue: &pb.Function{
					Name: "array_first",
					Args: []*pb.Value{
						{ValueType: &pb.Value_FieldReferenceValue{FieldReferenceValue: "field"}},
					},
				},
			}},
		},
		{
			desc: "ArrayFirstN",
			expr: ArrayFirstN("field", 5),
			want: &pb.Value{ValueType: &pb.Value_FunctionValue{
				FunctionValue: &pb.Function{
					Name: "array_first_n",
					Args: []*pb.Value{
						{ValueType: &pb.Value_FieldReferenceValue{FieldReferenceValue: "field"}},
						{ValueType: &pb.Value_IntegerValue{IntegerValue: 5}},
					},
				},
			}},
		},
		{
			desc: "ArrayLast",
			expr: ArrayLast("field"),
			want: &pb.Value{ValueType: &pb.Value_FunctionValue{
				FunctionValue: &pb.Function{
					Name: "array_last",
					Args: []*pb.Value{
						{ValueType: &pb.Value_FieldReferenceValue{FieldReferenceValue: "field"}},
					},
				},
			}},
		},
		{
			desc: "ArrayLastN",
			expr: ArrayLastN("field", 10),
			want: &pb.Value{ValueType: &pb.Value_FunctionValue{
				FunctionValue: &pb.Function{
					Name: "array_last_n",
					Args: []*pb.Value{
						{ValueType: &pb.Value_FieldReferenceValue{FieldReferenceValue: "field"}},
						{ValueType: &pb.Value_IntegerValue{IntegerValue: 10}},
					},
				},
			}},
		},
		{
			desc: "ArraySliceToEnd",
			expr: ArraySliceToEnd("field", 1),
			want: &pb.Value{ValueType: &pb.Value_FunctionValue{
				FunctionValue: &pb.Function{
					Name: "array_slice",
					Args: []*pb.Value{
						{ValueType: &pb.Value_FieldReferenceValue{FieldReferenceValue: "field"}},
						{ValueType: &pb.Value_IntegerValue{IntegerValue: 1}},
					},
				},
			}},
		},
		{
			desc: "ArraySliceWithLength",
			expr: ArraySlice("field", 1, 2),
			want: &pb.Value{ValueType: &pb.Value_FunctionValue{
				FunctionValue: &pb.Function{
					Name: "array_slice",
					Args: []*pb.Value{
						{ValueType: &pb.Value_FieldReferenceValue{FieldReferenceValue: "field"}},
						{ValueType: &pb.Value_IntegerValue{IntegerValue: 1}},
						{ValueType: &pb.Value_IntegerValue{IntegerValue: 2}},
					},
				},
			}},
		},
		{
			desc: "ArrayFilter",
			expr: ArrayFilter("field", "item", FieldOf("item").GreaterThan(5)),
			want: &pb.Value{ValueType: &pb.Value_FunctionValue{
				FunctionValue: &pb.Function{
					Name: "array_filter",
					Args: []*pb.Value{
						{ValueType: &pb.Value_FieldReferenceValue{FieldReferenceValue: "field"}},
						{ValueType: &pb.Value_StringValue{StringValue: "item"}},
						{ValueType: &pb.Value_FunctionValue{
							FunctionValue: &pb.Function{
								Name: "greater_than",
								Args: []*pb.Value{
									{ValueType: &pb.Value_FieldReferenceValue{FieldReferenceValue: "item"}},
									{ValueType: &pb.Value_IntegerValue{IntegerValue: 5}},
								},
							},
						}},
					},
				},
			}},
		},
		{
			desc: "baseExpression ArrayFilter",
			expr: FieldOf("field").ArrayFilter("item", FieldOf("item").GreaterThan(5)),
			want: &pb.Value{ValueType: &pb.Value_FunctionValue{
				FunctionValue: &pb.Function{
					Name: "array_filter",
					Args: []*pb.Value{
						{ValueType: &pb.Value_FieldReferenceValue{FieldReferenceValue: "field"}},
						{ValueType: &pb.Value_StringValue{StringValue: "item"}},
						{ValueType: &pb.Value_FunctionValue{
							FunctionValue: &pb.Function{
								Name: "greater_than",
								Args: []*pb.Value{
									{ValueType: &pb.Value_FieldReferenceValue{FieldReferenceValue: "item"}},
									{ValueType: &pb.Value_IntegerValue{IntegerValue: 5}},
								},
							},
						}},
					},
				},
			}},
		},
		{
			desc: "ArrayIndexOf",
			expr: ArrayIndexOf("field", "search"),
			want: &pb.Value{ValueType: &pb.Value_FunctionValue{
				FunctionValue: &pb.Function{
					Name: "array_index_of",
					Args: []*pb.Value{
						{ValueType: &pb.Value_FieldReferenceValue{FieldReferenceValue: "field"}},
						{ValueType: &pb.Value_StringValue{StringValue: "search"}},
						{ValueType: &pb.Value_StringValue{StringValue: "first"}},
					},
				},
			}},
		},
		{
			desc: "ArrayIndexOfAll",
			expr: ArrayIndexOfAll("field", "search"),
			want: &pb.Value{ValueType: &pb.Value_FunctionValue{
				FunctionValue: &pb.Function{
					Name: "array_index_of_all",
					Args: []*pb.Value{
						{ValueType: &pb.Value_FieldReferenceValue{FieldReferenceValue: "field"}},
						{ValueType: &pb.Value_StringValue{StringValue: "search"}},
					},
				},
			}},
		},
	}

	for _, tc := range testcases {
		t.Run(tc.desc, func(t *testing.T) {
			got, err := tc.expr.toProto()
			if err != nil {
				t.Fatalf("toProto() failed: %v", err)
			}
			if diff := testutil.Diff(got, tc.want); diff != "" {
				t.Errorf("toProto() returned diff (-got +want): %s", diff)
			}
		})
	}
}

func TestGetFieldVariations(t *testing.T) {
	// 1. (e Expression) GetField(string)
	expr1 := FieldOf("doc").GetField("title")
	if expr1 == nil {
		t.Fatal("expected expr1 not to be nil")
	}

	// 2. (e Expression) GetField(Expression)
	expr2 := FieldOf("doc").GetField(ConstantOf("title"))
	if expr2 == nil {
		t.Fatal("expected expr2 not to be nil")
	}

	// 3. GetField(string, string)
	expr3 := GetField("doc", "title")
	if expr3 == nil {
		t.Fatal("expected expr3 not to be nil")
	}

	// 4. GetField(Expression, Expression)
	expr4 := GetField(Variable("doc"), ConstantOf("title"))
	if expr4 == nil {
		t.Fatal("expected expr4 not to be nil")
	}

	// 5. GetField(string, Expression)
	expr5 := GetField("doc", ConstantOf("title"))
	if expr5 == nil {
		t.Fatal("expected expr5 not to be nil")
	}

	// 6. GetField(Expression, string)
	expr6 := GetField(Variable("doc"), "title")
	if expr6 == nil {
		t.Fatal("expected expr6 not to be nil")
	}
}

func TestSearchFunctions(t *testing.T) {
	// 1. DocumentMatches
	// 2. GeoDistance
	// 3. Score
	// 4. GeoDistance method
	// 8. Matches method
	testcases := []struct {
		desc string
		expr Expression
		want *pb.Value
	}{
		{
			desc: "DocumentMatches",
			expr: DocumentMatches("waffles"),
			want: &pb.Value{ValueType: &pb.Value_FunctionValue{
				FunctionValue: &pb.Function{
					Name: "document_matches",
					Args: []*pb.Value{
						{ValueType: &pb.Value_StringValue{StringValue: "waffles"}},
					},
				},
			}},
		},
		{
			desc: "GeoDistance",
			expr: GeoDistance("location", &latlng.LatLng{Latitude: 37.0, Longitude: -122.0}),
			want: &pb.Value{ValueType: &pb.Value_FunctionValue{
				FunctionValue: &pb.Function{
					Name: "geo_distance",
					Args: []*pb.Value{
						{ValueType: &pb.Value_FieldReferenceValue{FieldReferenceValue: "location"}},
						{ValueType: &pb.Value_GeoPointValue{GeoPointValue: &latlng.LatLng{Latitude: 37.0, Longitude: -122.0}}},
					},
				},
			}},
		},
		{
			desc: "Score",
			expr: Score(),
			want: &pb.Value{ValueType: &pb.Value_FunctionValue{
				FunctionValue: &pb.Function{
					Name: "score",
				},
			}},
		},
		{
			desc: "GeoDistance method",
			expr: FieldOf("location").GeoDistance(&latlng.LatLng{Latitude: 37.0, Longitude: -122.0}),
			want: &pb.Value{ValueType: &pb.Value_FunctionValue{
				FunctionValue: &pb.Function{
					Name: "geo_distance",
					Args: []*pb.Value{
						{ValueType: &pb.Value_FieldReferenceValue{FieldReferenceValue: "location"}},
						{ValueType: &pb.Value_GeoPointValue{GeoPointValue: &latlng.LatLng{Latitude: 37.0, Longitude: -122.0}}},
					},
				},
			}},
		},
	}

	for _, tc := range testcases {
		t.Run(tc.desc, func(t *testing.T) {
			got, err := tc.expr.toProto()
			if err != nil {
				t.Fatalf("toProto() failed: %v", err)
			}
			if diff := testutil.Diff(got, tc.want); diff != "" {
				t.Errorf("toProto() returned diff (-got +want): %s", diff)
			}
		})
	}
}

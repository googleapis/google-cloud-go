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
	"math"
	"testing"

	pb "cloud.google.com/go/firestore/apiv1/firestorepb"
	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"
	"google.golang.org/protobuf/testing/protocmp"
)

func TestQueryToPipeline(t *testing.T) {
	c := newTestClient()
	coll := c.Collection("C")

	// Some common expected parts
	collStage := func(path string) *pb.Pipeline_Stage {
		return &pb.Pipeline_Stage{
			Name: "collection",
			Args: []*pb.Value{refval(path)},
		}
	}

	sortStage := func(sorts ...map[string]any) *pb.Pipeline_Stage {
		var args []*pb.Value
		for _, s := range sorts {
			args = append(args, mapval(anyMapToValueMap(s)))
		}
		return &pb.Pipeline_Stage{
			Name: "sort",
			Args: args,
		}
	}

	whereStage := func(expr *pb.Value) *pb.Pipeline_Stage {
		return &pb.Pipeline_Stage{
			Name: "where",
			Args: []*pb.Value{expr},
		}
	}

	limitStage := func(n int64) *pb.Pipeline_Stage {
		return &pb.Pipeline_Stage{
			Name: "limit",
			Args: []*pb.Value{int64val(n)},
		}
	}

	ascending := func(field string) map[string]any {
		return map[string]any{
			"direction":  "ascending",
			"expression": fieldReference(field),
		}
	}

	descending := func(field string) map[string]any {
		return map[string]any{
			"direction":  "descending",
			"expression": fieldReference(field),
		}
	}

	exists := func(field string) *pb.Value {
		return functionval("exists", fieldReference(field))
	}

	equal := func(field string, val *pb.Value) *pb.Value {
		return functionval("equal", fieldReference(field), val)
	}

	greaterThanOrEqual := func(field string, val *pb.Value) *pb.Value {
		return functionval("greater_than_or_equal", fieldReference(field), val)
	}

	lessThanOrEqual := func(field string, val *pb.Value) *pb.Value {
		return functionval("less_than_or_equal", fieldReference(field), val)
	}

	greaterThan := func(field string, val *pb.Value) *pb.Value {
		return functionval("greater_than", fieldReference(field), val)
	}

	lessThan := func(field string, val *pb.Value) *pb.Value {
		return functionval("less_than", fieldReference(field), val)
	}

	notEqual := func(field string, val *pb.Value) *pb.Value {
		return functionval("not_equal", fieldReference(field), val)
	}

	or := func(args ...*pb.Value) *pb.Value {
		return functionval("or", args...)
	}

	and := func(args ...*pb.Value) *pb.Value {
		return functionval("and", args...)
	}

	array := func(args ...*pb.Value) *pb.Value {
		return functionval("array", args...)
	}

	fullRef := func(path string) *pb.Value {
		return refval("projects/test-project/databases/test-db/documents" + path)
	}

	testCases := []struct {
		name  string
		query Query
		want  []*pb.Pipeline_Stage
	}{
		{
			name:  "supportsDefaultQuery",
			query: coll.Query,
			want: []*pb.Pipeline_Stage{
				collStage("/C"),
				sortStage(ascending("__name__")),
			},
		},
		{
			name:  "supportsFilteredQuery",
			query: coll.Where("foo", "==", 1),
			want: []*pb.Pipeline_Stage{
				collStage("/C"),
				whereStage(equal("foo", int64val(1))),
				sortStage(ascending("__name__")),
			},
		},
		{
			name:  "supportsFilteredQueryWithFieldPath",
			query: coll.WherePath([]string{"foo"}, "==", 1),
			want: []*pb.Pipeline_Stage{
				collStage("/C"),
				whereStage(equal("foo", int64val(1))),
				sortStage(ascending("__name__")),
			},
		},
		{
			name:  "supportsOrderedQueryWithDefaultOrder",
			query: coll.OrderBy("foo", Asc),
			want: []*pb.Pipeline_Stage{
				collStage("/C"),
				whereStage(exists("foo")),
				sortStage(ascending("foo"), ascending("__name__")),
			},
		},
		{
			name:  "supportsOrderedQueryWithAsc",
			query: coll.OrderBy("foo", Asc),
			want: []*pb.Pipeline_Stage{
				collStage("/C"),
				whereStage(exists("foo")),
				sortStage(ascending("foo"), ascending("__name__")),
			},
		},
		{
			name:  "supportsOrderedQueryWithDesc",
			query: coll.OrderBy("foo", Desc),
			want: []*pb.Pipeline_Stage{
				collStage("/C"),
				whereStage(exists("foo")),
				sortStage(descending("foo"), descending("__name__")),
			},
		},
		{
			name:  "supportsLimitQuery",
			query: coll.OrderBy("foo", Asc).Limit(1),
			want: []*pb.Pipeline_Stage{
				collStage("/C"),
				whereStage(exists("foo")),
				sortStage(ascending("foo"), ascending("__name__")),
				limitStage(1),
			},
		},
		{
			name:  "supportsLimitToLastQuery",
			query: coll.OrderBy("foo", Asc).LimitToLast(2),
			want: []*pb.Pipeline_Stage{
				collStage("/C"),
				whereStage(exists("foo")),
				sortStage(descending("foo"), descending("__name__")),
				limitStage(2),
				sortStage(ascending("foo"), ascending("__name__")),
			},
		},
		{
			name:  "supportsStartAt",
			query: coll.OrderBy("foo", Asc).StartAt(2),
			want: []*pb.Pipeline_Stage{
				collStage("/C"),
				whereStage(exists("foo")),
				whereStage(greaterThanOrEqual("foo", int64val(2))),
				sortStage(ascending("foo"), ascending("__name__")),
			},
		},
		{
			name:  "supportsStartAtWithLimitToLast",
			query: coll.OrderBy("foo", Asc).StartAt(3).LimitToLast(4),
			want: []*pb.Pipeline_Stage{
				collStage("/C"),
				whereStage(exists("foo")),
				whereStage(greaterThanOrEqual("foo", int64val(3))),
				sortStage(descending("foo"), descending("__name__")),
				limitStage(4),
				sortStage(ascending("foo"), ascending("__name__")),
			},
		},
		{
			name:  "supportsEndAtWithLimitToLast",
			query: coll.OrderBy("foo", Asc).EndAt(3).LimitToLast(2),
			want: []*pb.Pipeline_Stage{
				collStage("/C"),
				whereStage(exists("foo")),
				whereStage(lessThanOrEqual("foo", int64val(3))),
				sortStage(descending("foo"), descending("__name__")),
				limitStage(2),
				sortStage(ascending("foo"), ascending("__name__")),
			},
		},
		{
			name:  "supportsStartAfter",
			query: coll.OrderBy("foo", Asc).StartAfter(1),
			want: []*pb.Pipeline_Stage{
				collStage("/C"),
				whereStage(exists("foo")),
				whereStage(greaterThan("foo", int64val(1))),
				sortStage(ascending("foo"), ascending("__name__")),
			},
		},
		{
			name:  "supportsEndAt",
			query: coll.OrderBy("foo", Asc).EndAt(1),
			want: []*pb.Pipeline_Stage{
				collStage("/C"),
				whereStage(exists("foo")),
				whereStage(lessThanOrEqual("foo", int64val(1))),
				sortStage(ascending("foo"), ascending("__name__")),
			},
		},
		{
			name:  "supportsEndBefore",
			query: coll.OrderBy("foo", Asc).EndBefore(2),
			want: []*pb.Pipeline_Stage{
				collStage("/C"),
				whereStage(exists("foo")),
				whereStage(lessThan("foo", int64val(2))),
				sortStage(ascending("foo"), ascending("__name__")),
			},
		},
		{
			name: "supportsStartAfterWithDocumentSnapshot",
			query: coll.OrderBy("foo", Asc).OrderBy("bar", Asc).OrderBy("baz", Asc).StartAfter(&DocumentSnapshot{
				Ref: &DocumentRef{Path: "projects/test-project/databases/test-db/documents/C/2", shortPath: "C/2", Parent: coll, ID: "2"},
				proto: &pb.Document{
					Fields: map[string]*pb.Value{
						"foo": int64val(1),
						"bar": int64val(1),
						"baz": int64val(2),
					},
				},
			}),
			want: []*pb.Pipeline_Stage{
				collStage("/C"),
				whereStage(exists("foo")),
				whereStage(exists("bar")),
				whereStage(exists("baz")),
				whereStage(or(
					greaterThan("foo", int64val(1)),
					and(
						equal("foo", int64val(1)),
						greaterThan("bar", int64val(1)),
					),
					and(
						equal("foo", int64val(1)),
						equal("bar", int64val(1)),
						greaterThan("baz", int64val(2)),
					),
					and(
						equal("foo", int64val(1)),
						equal("bar", int64val(1)),
						equal("baz", int64val(2)),
						greaterThan("__name__", fullRef("/C/2")),
					),
				)),
				sortStage(ascending("foo"), ascending("bar"), ascending("baz"), ascending("__name__")),
			},
		},
		{
			name:  "supportsQueryOverCollectionPathWithSpecialCharacters",
			query: coll.Doc("so!@#$%^&*()_+special").Collection("so!@#$%^&*()_+special").OrderBy("foo", Asc),
			want: []*pb.Pipeline_Stage{
				collStage("/C/so!@#$%^&*()_+special/so!@#$%^&*()_+special"),
				whereStage(exists("foo")),
				sortStage(ascending("foo"), ascending("__name__")),
			},
		},
		{
			name:  "supportsPagination",
			query: coll.OrderBy("foo", Asc).Limit(1).StartAfter(1),
			want: []*pb.Pipeline_Stage{
				collStage("/C"),
				whereStage(exists("foo")),
				whereStage(greaterThan("foo", int64val(1))),
				sortStage(ascending("foo"), ascending("__name__")),
				limitStage(1),
			},
		},
		{
			name:  "supportsPaginationOnDocumentIds",
			query: coll.OrderBy("foo", Asc).OrderBy(DocumentID, Asc).Limit(1).StartAfter(1, "doc1"),
			want: []*pb.Pipeline_Stage{
				collStage("/C"),
				whereStage(exists("foo")),
				whereStage(or(
					greaterThan("foo", int64val(1)),
					and(
						equal("foo", int64val(1)),
						greaterThan("__name__", fullRef("/C/doc1")),
					),
				)),
				sortStage(ascending("foo"), ascending("__name__")),
				limitStage(1),
			},
		},
		{
			name:  "supportsCollectionGroups",
			query: c.CollectionGroup("G").OrderBy(DocumentID, Asc),
			want: []*pb.Pipeline_Stage{
				{
					Name: "collection_group",
					Args: []*pb.Value{refval(""), strval("G")},
				},
				sortStage(ascending("__name__")),
			},
		},
		{
			name: "supportsMultipleInequalityOnSameField",
			query: coll.WhereEntity(AndFilter{[]EntityFilter{
				PropertyFilter{Path: "id", Operator: ">", Value: 2},
				PropertyFilter{Path: "id", Operator: "<=", Value: 10},
			}}).OrderBy("id", Asc),
			want: []*pb.Pipeline_Stage{
				collStage("/C"),
				whereStage(and(
					greaterThan("id", int64val(2)),
					lessThanOrEqual("id", int64val(10)),
				)),
				whereStage(exists("id")),
				sortStage(ascending("id"), ascending("__name__")),
			},
		},
		{
			name: "supportsMultipleInequalityOnDifferentFields",
			query: coll.WhereEntity(AndFilter{[]EntityFilter{
				PropertyFilter{Path: "id", Operator: ">=", Value: 2},
				PropertyFilter{Path: "baz", Operator: "<", Value: 2},
			}}).OrderBy("id", Asc),
			want: []*pb.Pipeline_Stage{
				collStage("/C"),
				whereStage(and(
					greaterThanOrEqual("id", int64val(2)),
					lessThan("baz", int64val(2)),
				)),
				whereStage(exists("id")),
				sortStage(ascending("id"), ascending("__name__")),
			},
		},
		{
			name:  "supportsCollectionGroupQuery",
			query: c.CollectionGroup("C").Query,
			want: []*pb.Pipeline_Stage{
				{
					Name: "collection_group",
					Args: []*pb.Value{refval(""), strval("C")},
				},
				sortStage(ascending("__name__")),
			},
		},
		{
			name:  "supportsEqNan",
			query: coll.Where("bar", "==", math.NaN()),
			want: []*pb.Pipeline_Stage{
				collStage("/C"),
				whereStage(equal("bar", floatval(math.NaN()))),
				sortStage(ascending("__name__")),
			},
		},
		{
			name:  "supportsNeqNan",
			query: coll.Where("bar", "!=", math.NaN()).OrderBy("foo", Asc),
			want: []*pb.Pipeline_Stage{
				collStage("/C"),
				whereStage(notEqual("bar", floatval(math.NaN()))),
				whereStage(exists("foo")),
				sortStage(ascending("foo"), ascending("__name__")),
			},
		},
		{
			name:  "supportsEqNull",
			query: coll.Where("bar", "==", nil),
			want: []*pb.Pipeline_Stage{
				collStage("/C"),
				whereStage(equal("bar", nullValue)),
				sortStage(ascending("__name__")),
			},
		},
		{
			name:  "supportsNeqNull",
			query: coll.Where("bar", "!=", nil),
			want: []*pb.Pipeline_Stage{
				collStage("/C"),
				whereStage(notEqual("bar", nullValue)),
				sortStage(ascending("__name__")),
			},
		},
		{
			name:  "supportsNeq",
			query: coll.Where("bar", "!=", 0),
			want: []*pb.Pipeline_Stage{
				collStage("/C"),
				whereStage(notEqual("bar", int64val(0))),
				sortStage(ascending("bar"), ascending("__name__")),
			},
		},
		{
			name:  "supportsArrayContains",
			query: coll.Where("bar", "array-contains", 4),
			want: []*pb.Pipeline_Stage{
				collStage("/C"),
				whereStage(functionval("array_contains", fieldReference("bar"), int64val(4))),
				sortStage(ascending("__name__")),
			},
		},
		{
			name:  "supportsArrayContainsAny",
			query: coll.Where("bar", "array-contains-any", []int{4, 5}),
			want: []*pb.Pipeline_Stage{
				collStage("/C"),
				whereStage(functionval("array_contains_any", fieldReference("bar"), array(int64val(4), int64val(5)))),
				sortStage(ascending("__name__")),
			},
		},
		{
			name:  "supportsIn",
			query: coll.Where("bar", "in", []int{0, 10, 20}),
			want: []*pb.Pipeline_Stage{
				collStage("/C"),
				whereStage(functionval("equal_any", fieldReference("bar"), array(int64val(0), int64val(10), int64val(20)))),
				sortStage(ascending("__name__")),
			},
		},
		{
			name:  "supportsInWith1",
			query: coll.Where("bar", "in", []int{2}),
			want: []*pb.Pipeline_Stage{
				collStage("/C"),
				whereStage(functionval("equal_any", fieldReference("bar"), array(int64val(2)))),
				sortStage(ascending("__name__")),
			},
		},
		{
			name:  "supportsNotIn",
			query: coll.Where("bar", "not-in", []int{0, 10, 20}).OrderBy("foo", Asc),
			want: []*pb.Pipeline_Stage{
				collStage("/C"),
				whereStage(functionval("not_equal_any", fieldReference("bar"), array(int64val(0), int64val(10), int64val(20)))),
				whereStage(exists("foo")),
				sortStage(ascending("foo"), ascending("__name__")),
			},
		},
		{
			name:  "supportsNotInWith1",
			query: coll.Where("bar", "not-in", []int{2}).OrderBy("foo", Asc),
			want: []*pb.Pipeline_Stage{
				collStage("/C"),
				whereStage(functionval("not_equal_any", fieldReference("bar"), array(int64val(2)))),
				whereStage(exists("foo")),
				sortStage(ascending("foo"), ascending("__name__")),
			},
		},
		{
			name: "supportsOrOperator",
			query: coll.WhereEntity(OrFilter{[]EntityFilter{
				PropertyFilter{Path: "bar", Operator: "==", Value: 2},
				PropertyFilter{Path: "foo", Operator: "==", Value: 3},
			}}).OrderBy("foo", Asc),
			want: []*pb.Pipeline_Stage{
				collStage("/C"),
				whereStage(or(
					equal("bar", int64val(2)),
					equal("foo", int64val(3)),
				)),
				whereStage(exists("foo")),
				sortStage(ascending("foo"), ascending("__name__")),
			},
		},
		{
			name:  "testNotEqualIncludesMissingField",
			query: coll.Where("bar", "!=", 1),
			want: []*pb.Pipeline_Stage{
				collStage("/C"),
				whereStage(notEqual("bar", int64val(1))),
				sortStage(ascending("bar"), ascending("__name__")),
			},
		},
		{
			name:  "testNotInIncludesMissingField",
			query: coll.Where("bar", "not-in", []int{1}),
			want: []*pb.Pipeline_Stage{
				collStage("/C"),
				whereStage(functionval("not_equal_any", fieldReference("bar"), array(int64val(1)))),
				sortStage(ascending("bar"), ascending("__name__")),
			},
		},
		{
			name:  "testInequalityMaintainsExistenceFilter",
			query: coll.Where("bar", "<", 1),
			want: []*pb.Pipeline_Stage{
				collStage("/C"),
				whereStage(lessThan("bar", int64val(1))),
				sortStage(ascending("bar"), ascending("__name__")),
			},
		},
		{
			name:  "testExplicitOrderMaintainsExistenceFilter",
			query: coll.OrderBy("bar", Asc),
			want: []*pb.Pipeline_Stage{
				collStage("/C"),
				whereStage(exists("bar")),
				sortStage(ascending("bar"), ascending("__name__")),
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			p := tc.query.Pipeline()
			if p.err != nil {
				t.Fatalf("Pipeline() error: %v", p.err)
			}
			req, err := p.toExecutePipelineRequest()
			if err != nil {
				t.Fatalf("toExecutePipelineRequest() error: %v", err)
			}
			got := req.GetStructuredPipeline().GetPipeline().GetStages()
			if diff := cmp.Diff(tc.want, got, protocmp.Transform(), cmpopts.EquateNaNs()); diff != "" {
				t.Errorf("Mismatch (-want +got):\n%s", diff)
			}
		})
	}
}

func fieldReference(name string) *pb.Value {
	return &pb.Value{ValueType: &pb.Value_FieldReferenceValue{FieldReferenceValue: name}}
}

func functionval(name string, args ...*pb.Value) *pb.Value {
	return &pb.Value{ValueType: &pb.Value_FunctionValue{FunctionValue: &pb.Function{
		Name: name,
		Args: args,
	}}}
}

func anyMapToValueMap(m map[string]any) map[string]*pb.Value {
	res := make(map[string]*pb.Value)
	for k, v := range m {
		switch x := v.(type) {
		case string:
			res[k] = strval(x)
		case *pb.Value:
			res[k] = x
		default:
			panic("unsupported type in anyMapToValueMap")
		}
	}
	return res
}

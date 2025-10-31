// Copyright 2025 Google LLC
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
	"context"
	"testing"

	pb "cloud.google.com/go/firestore/apiv1/firestorepb"
	"cloud.google.com/go/internal/testutil"
	"github.com/google/go-cmp/cmp"
	"google.golang.org/protobuf/testing/protocmp"
)

func TestPipelineStages(t *testing.T) {
	docRef1 := &DocumentRef{
		Path:      "projects/projectID/databases/(default)/documents/collection/doc1",
		shortPath: "collection/doc1",
	}
	docRef2 := &DocumentRef{
		Path:      "projects/projectID/databases/(default)/documents/collection/doc2",
		shortPath: "collection/doc2",
	}

	testcases := []struct {
		desc  string
		stage pipelineStage
		want  *pb.Pipeline_Stage
	}{
		{
			desc:  "inputStageCollection",
			stage: newInputStageCollection("my-collection", nil),
			want: &pb.Pipeline_Stage{
				Name: "collection",
				Args: []*pb.Value{{ValueType: &pb.Value_ReferenceValue{ReferenceValue: "/my-collection"}}},
			},
		},
		{
			desc:  "inputStageCollectionGroup",
			stage: newInputStageCollectionGroup("ancestor/path", "my-collection-group", nil),
			want: &pb.Pipeline_Stage{
				Name: "collection_group",
				Args: []*pb.Value{
					{ValueType: &pb.Value_ReferenceValue{ReferenceValue: "ancestor/path"}},
					{ValueType: &pb.Value_StringValue{StringValue: "my-collection-group"}},
				},
			},
		},
		{
			desc:  "inputStageDatabase",
			stage: newInputStageDatabase(),
			want:  &pb.Pipeline_Stage{Name: "database"},
		},
		{
			desc:  "inputStageDocuments",
			stage: newInputStageDocuments(docRef1, docRef2),
			want: &pb.Pipeline_Stage{
				Name: "documents",
				Args: []*pb.Value{
					{ValueType: &pb.Value_ReferenceValue{ReferenceValue: "/collection/doc1"}},
					{ValueType: &pb.Value_ReferenceValue{ReferenceValue: "/collection/doc2"}},
				},
			},
		},
		{
			desc:  "limitStage",
			stage: newLimitStage(10),
			want: &pb.Pipeline_Stage{
				Name: "limit",
				Args: []*pb.Value{{ValueType: &pb.Value_IntegerValue{IntegerValue: 10}}},
			},
		},
		{
			desc:  "offsetStage",
			stage: newOffsetStage(5),
			want: &pb.Pipeline_Stage{
				Name: "offset",
				Args: []*pb.Value{{ValueType: &pb.Value_IntegerValue{IntegerValue: 5}}},
			},
		},
		{
			desc:  "sortStage",
			stage: newSortStage(Ascending(FieldOf("name")), Descending(FieldOf("age"))),
			want: &pb.Pipeline_Stage{
				Name: "sort",
				Args: []*pb.Value{
					{ValueType: &pb.Value_MapValue{MapValue: &pb.MapValue{Fields: map[string]*pb.Value{
						"direction":  {ValueType: &pb.Value_StringValue{StringValue: "ascending"}},
						"expression": {ValueType: &pb.Value_FieldReferenceValue{FieldReferenceValue: "name"}},
					}}}},
					{ValueType: &pb.Value_MapValue{MapValue: &pb.MapValue{Fields: map[string]*pb.Value{
						"direction":  {ValueType: &pb.Value_StringValue{StringValue: "descending"}},
						"expression": {ValueType: &pb.Value_FieldReferenceValue{FieldReferenceValue: "age"}},
					}}}},
				},
			},
		},
	}

	for _, tc := range testcases {
		t.Run(tc.desc, func(t *testing.T) {
			got, err := tc.stage.toProto()
			if err != nil {
				t.Fatalf("toProto() failed: %v", err)
			}
			if diff := testutil.Diff(got, tc.want); diff != "" {
				t.Errorf("toProto() returned diff (-got +want): %s", diff)
			}
		})
	}
}

func TestSelectStage(t *testing.T) {
	stage, err := newSelectStage("name", FieldOf("age"), Add(FieldOf("score"), 10).As("new_score"))
	if err != nil {
		t.Fatalf("newSelectStage() failed: %v", err)
	}

	want := &pb.Pipeline_Stage{
		Name: "select",
		Args: []*pb.Value{
			{ValueType: &pb.Value_MapValue{MapValue: &pb.MapValue{Fields: map[string]*pb.Value{
				"name": {ValueType: &pb.Value_FieldReferenceValue{FieldReferenceValue: "name"}},
				"age":  {ValueType: &pb.Value_FieldReferenceValue{FieldReferenceValue: "age"}},
				"new_score": {ValueType: &pb.Value_FunctionValue{FunctionValue: &pb.Function{
					Name: "add",
					Args: []*pb.Value{
						{ValueType: &pb.Value_FieldReferenceValue{FieldReferenceValue: "score"}},
						{ValueType: &pb.Value_IntegerValue{IntegerValue: 10}},
					},
				}}},
			}}}},
		},
	}

	got, err := stage.toProto()
	if err != nil {
		t.Fatalf("toProto() failed: %v", err)
	}
	if diff := testutil.Diff(got, want); diff != "" {
		t.Errorf("toProto() returned diff (-got +want): %s", diff)
	}
}

func TestWhereStage(t *testing.T) {
	condition := Equal(FieldOf("genre"), "Sci-Fi")
	stage, err := newWhereStage(condition)
	if err != nil {
		t.Fatalf("newWhereStage() failed: %v", err)
	}

	want := &pb.Pipeline_Stage{
		Name: "where",
		Args: []*pb.Value{
			{ValueType: &pb.Value_FunctionValue{FunctionValue: &pb.Function{
				Name: "equal",
				Args: []*pb.Value{
					{ValueType: &pb.Value_FieldReferenceValue{FieldReferenceValue: "genre"}},
					{ValueType: &pb.Value_StringValue{StringValue: "Sci-Fi"}},
				},
			}}},
		},
	}

	got, err := stage.toProto()
	if err != nil {
		t.Fatalf("toProto() failed: %v", err)
	}
	if diff := testutil.Diff(got, want); diff != "" {
		t.Errorf("toProto() returned diff (-got +want): %s", diff)
	}
}

func TestAddFieldsStage(t *testing.T) {
	stage, err := newAddFieldsStage(FieldOf("name").As("name"), Add(FieldOf("score"), 10).As("new_score"))
	if err != nil {
		t.Fatalf("newAddFieldsStage() failed: %v", err)
	}

	want := &pb.Pipeline_Stage{
		Name: "add_fields",
		Args: []*pb.Value{
			{ValueType: &pb.Value_MapValue{MapValue: &pb.MapValue{Fields: map[string]*pb.Value{
				"name": {ValueType: &pb.Value_FieldReferenceValue{FieldReferenceValue: "name"}},
				"new_score": {ValueType: &pb.Value_FunctionValue{FunctionValue: &pb.Function{
					Name: "add",
					Args: []*pb.Value{
						{ValueType: &pb.Value_FieldReferenceValue{FieldReferenceValue: "score"}},
						{ValueType: &pb.Value_IntegerValue{IntegerValue: 10}},
					},
				}}},
			}}}},
		},
	}

	got, err := stage.toProto()
	if err != nil {
		t.Fatalf("toProto() failed: %v", err)
	}
	if diff := testutil.Diff(got, want); diff != "" {
		t.Errorf("toProto() returned diff (-got +want): %s", diff)
	}
}

func TestAggregateStage(t *testing.T) {
	spec := NewAggregateSpec(Sum("score").As("total_score")).WithGroups("category")
	stage, err := newAggregateStage(spec)
	if err != nil {
		t.Fatalf("newAggregateStage() failed: %v", err)
	}

	want := &pb.Pipeline_Stage{
		Name: "aggregate",
		Args: []*pb.Value{
			{ValueType: &pb.Value_MapValue{MapValue: &pb.MapValue{Fields: map[string]*pb.Value{
				"total_score": {ValueType: &pb.Value_FunctionValue{FunctionValue: &pb.Function{
					Name: "sum",
					Args: []*pb.Value{
						{ValueType: &pb.Value_FieldReferenceValue{FieldReferenceValue: "score"}},
					},
				}}},
			}}}},
			{ValueType: &pb.Value_MapValue{MapValue: &pb.MapValue{Fields: map[string]*pb.Value{
				"category": {ValueType: &pb.Value_FieldReferenceValue{FieldReferenceValue: "category"}},
			}}}},
		},
	}

	got, err := stage.toProto()
	if err != nil {
		t.Fatalf("toProto() failed: %v", err)
	}
	if diff := testutil.Diff(got, want); diff != "" {
		t.Errorf("toProto() returned diff (-got +want): %s", diff)
	}
}

func TestDistinctStage(t *testing.T) {
	stage, err := newDistinctStage("category", FieldOf("author"))
	if err != nil {
		t.Fatalf("newDistinctStage() failed: %v", err)
	}

	want := &pb.Pipeline_Stage{
		Name: "distinct",
		Args: []*pb.Value{
			{ValueType: &pb.Value_MapValue{MapValue: &pb.MapValue{Fields: map[string]*pb.Value{
				"category": {ValueType: &pb.Value_FieldReferenceValue{FieldReferenceValue: "category"}},
				"author":   {ValueType: &pb.Value_FieldReferenceValue{FieldReferenceValue: "author"}},
			}}}},
		},
	}

	got, err := stage.toProto()
	if err != nil {
		t.Fatalf("toProto() failed: %v", err)
	}
	if diff := testutil.Diff(got, want); diff != "" {
		t.Errorf("toProto() returned diff (-got +want): %s", diff)
	}
}

func TestFindNearestStage(t *testing.T) {
	limit := 10
	distanceField := "distance"
	stage, err := newFindNearestStage("embedding", []float64{1, 2, 3}, PipelineDistanceMeasureEuclidean, &PipelineFindNearestOptions{Limit: &limit, DistanceField: &distanceField})
	if err != nil {
		t.Fatalf("newFindNearestStage() failed: %v", err)
	}

	want := &pb.Pipeline_Stage{
		Name: "find_nearest",
		Args: []*pb.Value{
			{ValueType: &pb.Value_FieldReferenceValue{FieldReferenceValue: "embedding"}},
			vectorToProtoValue([]float64{1, 2, 3}),
			{ValueType: &pb.Value_StringValue{StringValue: "euclidean"}},
		},
		Options: map[string]*pb.Value{
			"limit":          {ValueType: &pb.Value_IntegerValue{IntegerValue: 10}},
			"distance_field": {ValueType: &pb.Value_FieldReferenceValue{FieldReferenceValue: "distance"}},
		},
	}

	got, err := stage.toProto()
	if err != nil {
		t.Fatalf("toProto() failed: %v", err)
	}
	if diff := testutil.Diff(got, want); diff != "" {
		t.Errorf("toProto() returned diff (-got +want): %s", diff)
	}
}

func TestRawStage(t *testing.T) {
	tests := []struct {
		name  string
		stage *RawStage
		want  *pb.Pipeline_Stage
	}{
		{
			name:  "no args or options",
			stage: NewRawStage("test_stage"),
			want: &pb.Pipeline_Stage{
				Name: "test_stage",
			},
		},
		{
			name:  "with args",
			stage: NewRawStage("another_stage").WithArguments("arg1", 123, true),
			want: &pb.Pipeline_Stage{
				Name: "another_stage",
				Args: []*pb.Value{
					{ValueType: &pb.Value_StringValue{StringValue: "arg1"}},
					{ValueType: &pb.Value_IntegerValue{IntegerValue: 123}},
					{ValueType: &pb.Value_BooleanValue{BooleanValue: true}},
				},
			},
		},
		{
			name: "with options",
			stage: NewRawStage("option_stage").WithOptions(RawStageOptions{
				"opt1": "val1",
				"opt2": 456,
			}),
			want: &pb.Pipeline_Stage{
				Name: "option_stage",
				Options: map[string]*pb.Value{
					"opt1": {ValueType: &pb.Value_StringValue{StringValue: "val1"}},
					"opt2": {ValueType: &pb.Value_IntegerValue{IntegerValue: 456}},
				},
			},
		},
		{
			name:  "with args and options",
			stage: NewRawStage("complex_stage").WithArguments(FieldOf("myField")).WithOptions(RawStageOptions{"enabled": true}),
			want: &pb.Pipeline_Stage{
				Name: "complex_stage",
				Args: []*pb.Value{
					{ValueType: &pb.Value_FieldReferenceValue{FieldReferenceValue: "myField"}},
				},
				Options: map[string]*pb.Value{
					"enabled": {ValueType: &pb.Value_BooleanValue{BooleanValue: true}},
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := tt.stage.toProto()
			if err != nil {
				t.Fatalf("toProto() failed: %v", err)
			}
			if diff := cmp.Diff(tt.want, got, protocmp.Transform()); diff != "" {
				t.Errorf("toProto() mismatch (-want +got):\n%s", diff)
			}
		})
	}
}

func TestRemoveFieldsStage(t *testing.T) {
	stage, err := newRemoveFieldsStage("price", FieldPath{"author", "name"})
	if err != nil {
		t.Fatalf("newRemoveFieldsStage() failed: %v", err)
	}

	want := &pb.Pipeline_Stage{
		Name: "remove_fields",
		Args: []*pb.Value{
			{ValueType: &pb.Value_FieldReferenceValue{FieldReferenceValue: "price"}},
			{ValueType: &pb.Value_FieldReferenceValue{FieldReferenceValue: "author.name"}},
		},
	}

	got, err := stage.toProto()
	if err != nil {
		t.Fatalf("toProto() failed: %v", err)
	}
	if diff := testutil.Diff(got, want); diff != "" {
		t.Errorf("toProto() returned diff (-got +want): %s", diff)
	}
}

func TestReplaceStage(t *testing.T) {
	stage, err := newReplaceStage("metadata")
	if err != nil {
		t.Fatalf("newReplaceStage() failed: %v", err)
	}

	want := &pb.Pipeline_Stage{
		Name: "replace_with",
		Args: []*pb.Value{
			{ValueType: &pb.Value_FieldReferenceValue{FieldReferenceValue: "metadata"}},
			{ValueType: &pb.Value_StringValue{StringValue: "full_replace"}},
		},
	}

	got, err := stage.toProto()
	if err != nil {
		t.Fatalf("toProto() returned diff (-got +want): %v", err)
	}
	if diff := testutil.Diff(got, want); diff != "" {
		t.Errorf("toProto() returned diff (-got +want): %s", diff)
	}
}

func TestSampleStage(t *testing.T) {
	spec := SampleByDocuments(100)
	stage, err := newSampleStage(spec)
	if err != nil {
		t.Fatalf("newSampleStage() failed: %v", err)
	}

	want := &pb.Pipeline_Stage{
		Name: "sample",
		Args: []*pb.Value{
			{ValueType: &pb.Value_IntegerValue{IntegerValue: 100}},
			{ValueType: &pb.Value_StringValue{StringValue: "documents"}},
		},
	}

	got, err := stage.toProto()
	if err != nil {
		t.Fatalf("toProto() failed: %v", err)
	}
	if diff := testutil.Diff(got, want); diff != "" {
		t.Errorf("toProto() returned diff (-got +want):%s", diff)
	}
}

func TestUnionStage(t *testing.T) {
	client, err := NewClient(context.Background(), "projectID")
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}
	otherPipeline := newPipeline(client, newInputStageCollection("other_collection", nil))
	stage, err := newUnionStage(otherPipeline)
	if err != nil {
		t.Fatalf("newUnionStage() failed: %v", err)
	}

	want := &pb.Pipeline_Stage{
		Name: "union",
		Args: []*pb.Value{
			{ValueType: &pb.Value_PipelineValue{PipelineValue: &pb.Pipeline{
				Stages: []*pb.Pipeline_Stage{
					{
						Name: "collection",
						Args: []*pb.Value{{ValueType: &pb.Value_ReferenceValue{ReferenceValue: "/other_collection"}}},
					},
				},
			}}},
		},
	}

	got, err := stage.toProto()
	if err != nil {
		t.Fatalf("toProto() failed: %v", err)
	}
	if diff := testutil.Diff(got, want); diff != "" {
		t.Errorf("toProto() returned diff (-got +want): %s", diff)
	}
}

func TestUnnestStage(t *testing.T) {
	stage, err := newUnnestStage(FieldOf("tags"), "tag", &UnnestOptions{IndexField: "index"})
	if err != nil {
		t.Fatalf("newUnnestStage() failed: %v", err)
	}

	want := &pb.Pipeline_Stage{
		Name: "unnest",
		Args: []*pb.Value{
			{ValueType: &pb.Value_FieldReferenceValue{FieldReferenceValue: "tags"}},
			{ValueType: &pb.Value_FieldReferenceValue{FieldReferenceValue: "tag"}},
		},
		Options: map[string]*pb.Value{
			"index_field": {ValueType: &pb.Value_FieldReferenceValue{FieldReferenceValue: "index"}},
		},
	}

	got, err := stage.toProto()
	if err != nil {
		t.Fatalf("toProto() failed: %v", err)
	}
	if diff := testutil.Diff(got, want); diff != "" {
		t.Errorf("toProto() returned diff (-got +want): %s", diff)
	}
}

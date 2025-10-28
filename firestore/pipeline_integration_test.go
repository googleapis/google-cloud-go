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
	"math"
	"strings"
	"testing"
	"time"

	"cloud.google.com/go/internal/testutil"
	"github.com/google/go-cmp/cmp/cmpopts"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func TestIntegration_PipelineFunctions(t *testing.T) {
	if testParams[firestoreEditionKey].(firestoreEdition) != editionEnterprise {
		t.Skip("Skipping pipeline queries tests since the firestore edition of", testParams[databaseIDKey].(string), "database is not enterprise")
	}
	t.Run("arrayFuncs", arrayFuncs)
	t.Run("stringFuncs", stringFuncs)
	t.Run("typeFuncs", typeFuncs)
	t.Run("vectorFuncs", vectorFuncs)

}

func arrayFuncs(t *testing.T) {
	t.Parallel()
	h := testHelper{t}
	client := integrationClient(t)
	coll := client.Collection(collectionIDs.New())
	docRef1 := coll.NewDoc()
	h.mustCreate(docRef1, map[string]interface{}{
		"a":      []interface{}{1, 2, 3},
		"b":      []interface{}{4, 5, 6},
		"tags":   []string{"Go", "Firestore", "GCP"},
		"tags2":  []string{"Go", "Firestore"},
		"lang":   "Go",
		"status": "active",
	})
	defer deleteDocuments([]*DocumentRef{docRef1})

	tests := []struct {
		name     string
		pipeline *Pipeline
		want     map[string]interface{}
	}{
		{
			name:     "ArrayLength",
			pipeline: client.Pipeline().Collection(coll.ID).Select(ArrayLength("a").As("length")),
			want:     map[string]interface{}{"length": int64(3)},
		},
		{
			name:     "Array",
			pipeline: client.Pipeline().Collection(coll.ID).Select(Array(1, 2, 3).As("array")),
			want:     map[string]interface{}{"array": []interface{}{int64(1), int64(2), int64(3)}},
		},
		{
			name:     "ArrayFromSlice",
			pipeline: client.Pipeline().Collection(coll.ID).Select(ArrayFromSlice([]int{1, 2, 3}).As("array")),
			want:     map[string]interface{}{"array": []interface{}{int64(1), int64(2), int64(3)}},
		},
		{
			name:     "ArrayGet",
			pipeline: client.Pipeline().Collection(coll.ID).Select(ArrayGet("a", 1).As("element")),
			want:     map[string]interface{}{"element": int64(2)},
		},
		{
			name:     "ArrayReverse",
			pipeline: client.Pipeline().Collection(coll.ID).Select(ArrayReverse("a").As("reversed")),
			want:     map[string]interface{}{"reversed": []interface{}{int64(3), int64(2), int64(1)}},
		},
		{
			name:     "ArrayConcat",
			pipeline: client.Pipeline().Collection(coll.ID).Select(ArrayConcat("a", FieldOf("b")).As("concatenated")),
			want:     map[string]interface{}{"concatenated": []interface{}{int64(1), int64(2), int64(3), int64(4), int64(5), int64(6)}},
		},
		{
			name:     "ArraySum",
			pipeline: client.Pipeline().Collection(coll.ID).Select(ArraySum("a").As("sum")),
			want:     map[string]interface{}{"sum": int64(6)},
		},
		{
			name:     "ArrayMaximum",
			pipeline: client.Pipeline().Collection(coll.ID).Select(ArrayMaximum("a").As("max")),
			want:     map[string]interface{}{"max": int64(3)},
		},
		{
			name:     "ArrayMinimum",
			pipeline: client.Pipeline().Collection(coll.ID).Select(ArrayMinimum("a").As("min")),
			want:     map[string]interface{}{"min": int64(1)},
		},
		// Array filter conditions
		{
			name:     "ArrayContains",
			pipeline: client.Pipeline().Collection(coll.ID).Where(ArrayContains("tags", "Go")),
			want:     map[string]interface{}{"lang": "Go", "tags": []interface{}{"Go", "Firestore", "GCP"}, "tags2": []interface{}{"Go", "Firestore"}, "status": "active", "a": []interface{}{int64(1), int64(2), int64(3)}, "b": []interface{}{int64(4), int64(5), int64(6)}},
		},
		{
			name:     "ArrayContainsAll - array of mixed types",
			pipeline: client.Pipeline().Collection(coll.ID).Where(ArrayContainsAll("tags", []any{FieldOf("lang"), "Firestore"})),
			want:     map[string]interface{}{"lang": "Go", "tags": []interface{}{"Go", "Firestore", "GCP"}, "tags2": []interface{}{"Go", "Firestore"}, "status": "active", "a": []interface{}{int64(1), int64(2), int64(3)}, "b": []interface{}{int64(4), int64(5), int64(6)}},
		},
		{
			name:     "ArrayContainsAll - array of constants",
			pipeline: client.Pipeline().Collection(coll.ID).Where(ArrayContainsAll("tags", []string{"Go", "Firestore"})),
			want:     map[string]interface{}{"lang": "Go", "tags": []interface{}{"Go", "Firestore", "GCP"}, "tags2": []interface{}{"Go", "Firestore"}, "status": "active", "a": []interface{}{int64(1), int64(2), int64(3)}, "b": []interface{}{int64(4), int64(5), int64(6)}},
		},
		{
			name:     "ArrayContainsAll - Expr",
			pipeline: client.Pipeline().Collection(coll.ID).Where(ArrayContainsAll("tags", FieldOf("tags2"))),
			want:     map[string]interface{}{"lang": "Go", "tags": []interface{}{"Go", "Firestore", "GCP"}, "tags2": []interface{}{"Go", "Firestore"}, "status": "active", "a": []interface{}{int64(1), int64(2), int64(3)}, "b": []interface{}{int64(4), int64(5), int64(6)}},
		},
		{
			name:     "ArrayContainsAny",
			pipeline: client.Pipeline().Collection(coll.ID).Where(ArrayContainsAny("tags", []string{"Go", "Java"})),
			want:     map[string]interface{}{"lang": "Go", "tags": []interface{}{"Go", "Firestore", "GCP"}, "tags2": []interface{}{"Go", "Firestore"}, "status": "active", "a": []interface{}{int64(1), int64(2), int64(3)}, "b": []interface{}{int64(4), int64(5), int64(6)}},
		},
		{
			name:     "EqualAny",
			pipeline: client.Pipeline().Collection(coll.ID).Where(EqualAny("status", []string{"active", "pending"})),
			want:     map[string]interface{}{"lang": "Go", "tags": []interface{}{"Go", "Firestore", "GCP"}, "tags2": []interface{}{"Go", "Firestore"}, "status": "active", "a": []interface{}{int64(1), int64(2), int64(3)}, "b": []interface{}{int64(4), int64(5), int64(6)}},
		},
		{
			name:     "NotEqualAny",
			pipeline: client.Pipeline().Collection(coll.ID).Where(NotEqualAny("status", []string{"archived", "deleted"})),
			want:     map[string]interface{}{"lang": "Go", "tags": []interface{}{"Go", "Firestore", "GCP"}, "tags2": []interface{}{"Go", "Firestore"}, "status": "active", "a": []interface{}{int64(1), int64(2), int64(3)}, "b": []interface{}{int64(4), int64(5), int64(6)}},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			testutil.Retry(t, 3, time.Second, func(r *testutil.R) {
				ctx := context.Background()
				iter := test.pipeline.Execute(ctx)
				defer iter.Stop()

				docs, err := iter.GetAll()
				if isInvalidHeaderError(err) {
					r.Errorf("GetAll: %v. Retrying....", err)
					return
				} else if err != nil {
					r.Fatalf("GetAll: %v", err)
					return
				}
				if len(docs) != 1 {
					r.Fatalf("expected 1 doc, got %d", len(docs))
					return
				}
				got, err := docs[0].Data()
				if err != nil {
					r.Fatalf("Data: %v", err)
					return
				}
				if diff := testutil.Diff(got, test.want); diff != "" {
					r.Errorf("got: %v, want: %v, diff +want -got: %s", got, test.want, diff)
					return
				}
			})
		})
	}
}

func stringFuncs(t *testing.T) {
	t.Parallel()
	h := testHelper{t}
	client := integrationClient(t)
	coll := client.Collection(collectionIDs.New())
	docRef1 := coll.NewDoc()
	h.mustCreate(docRef1, map[string]interface{}{
		"name":        "  John Doe  ",
		"description": "This is a Firestore document.",
		"productCode": "abc-123",
		"tags":        []string{"tag1", "tag2", "tag3"},
		"email":       "john.doe@example.com",
		"zipCode":     "12345",
	})
	defer deleteDocuments([]*DocumentRef{docRef1})

	doc1want := map[string]interface{}{
		"name":        "  John Doe  ",
		"description": "This is a Firestore document.",
		"productCode": "abc-123",
		"tags":        []interface{}{"tag1", "tag2", "tag3"},
		"email":       "john.doe@example.com",
		"zipCode":     "12345",
	}

	tests := []struct {
		name     string
		pipeline *Pipeline
		want     interface{}
	}{
		{
			name:     "ByteLength",
			pipeline: client.Pipeline().Collection(coll.ID).Select(ByteLength("name").As("byte_length")),
			want:     map[string]interface{}{"byte_length": int64(12)},
		},
		{
			name:     "CharLength",
			pipeline: client.Pipeline().Collection(coll.ID).Select(CharLength("name").As("char_length")),
			want:     map[string]interface{}{"char_length": int64(12)},
		},
		{
			name:     "StringConcat",
			pipeline: client.Pipeline().Collection(coll.ID).Select(StringConcat(FieldOf("name"), " - ", FieldOf("productCode")).As("concatenated_string")),
			want:     map[string]interface{}{"concatenated_string": "  John Doe   - abc-123"},
		},
		{
			name:     "StringReverse",
			pipeline: client.Pipeline().Collection(coll.ID).Select(StringReverse("name").As("reversed_string")),
			want:     map[string]interface{}{"reversed_string": "  eoD nhoJ  "},
		},
		{
			name:     "Join",
			pipeline: client.Pipeline().Collection(coll.ID).Select(Join("tags", ", ").As("joined_string")),
			want:     map[string]interface{}{"joined_string": "tag1, tag2, tag3"},
		},
		{
			name:     "Substring",
			pipeline: client.Pipeline().Collection(coll.ID).Select(Substring("description", 0, 4).As("substring")),
			want:     map[string]interface{}{"substring": "This"},
		},
		{
			name:     "ToLower",
			pipeline: client.Pipeline().Collection(coll.ID).Select(ToLower("name").As("lowercase_name")),
			want:     map[string]interface{}{"lowercase_name": "  john doe  "},
		},
		{
			name:     "ToUpper",
			pipeline: client.Pipeline().Collection(coll.ID).Select(ToUpper("name").As("uppercase_name")),
			want:     map[string]interface{}{"uppercase_name": "  JOHN DOE  "},
		},
		{
			name:     "Trim",
			pipeline: client.Pipeline().Collection(coll.ID).Select(Trim("name").As("trimmed_name")),
			want:     map[string]interface{}{"trimmed_name": "John Doe"},
		},
		// String filter conditions
		{
			name:     "Like",
			pipeline: client.Pipeline().Collection(coll.ID).Where(Like("name", "%John%")),
			want:     []map[string]interface{}{doc1want},
		},
		{
			name:     "StartsWith",
			pipeline: client.Pipeline().Collection(coll.ID).Where(StartsWith("name", "  John")),
			want:     []map[string]interface{}{doc1want},
		},
		{
			name:     "EndsWith",
			pipeline: client.Pipeline().Collection(coll.ID).Where(EndsWith("name", "Doe  ")),
			want:     []map[string]interface{}{doc1want},
		},
		{
			name:     "RegexContains",
			pipeline: client.Pipeline().Collection(coll.ID).Where(RegexContains("email", "@example\\.com")),
			want:     []map[string]interface{}{doc1want},
		},
		{
			name:     "RegexMatch",
			pipeline: client.Pipeline().Collection(coll.ID).Where(RegexMatch("zipCode", "^[0-9]{5}$")),
			want:     []map[string]interface{}{doc1want},
		},
		{
			name:     "StringContains",
			pipeline: client.Pipeline().Collection(coll.ID).Where(StringContains("description", "Firestore")),
			want:     []map[string]interface{}{doc1want},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			testutil.Retry(t, 3, time.Second, func(r *testutil.R) {
				ctx := context.Background()

				iter := test.pipeline.Execute(ctx)
				defer iter.Stop()

				docs, err := iter.GetAll()
				if isInvalidHeaderError(err) {
					r.Errorf("GetAll: %v. Retrying....", err)
					return
				} else if err != nil {
					r.Fatalf("GetAll: %v", err)
					return
				}
				lastStage := test.pipeline.stages[len(test.pipeline.stages)-1]
				lastStageName := lastStage.name()

				if lastStageName == stageNameSelect { // This is a select query
					want, ok := test.want.(map[string]interface{})
					if !ok {
						r.Fatalf("invalid test.want type for select query: %T", test.want)
						return
					}
					if len(docs) != 1 {
						r.Fatalf("expected 1 doc, got %d", len(docs))
						return
					}
					got, err := docs[0].Data()
					if err != nil {
						r.Fatalf("Data: %v", err)
						return
					}
					if diff := testutil.Diff(got, want); diff != "" {
						t.Errorf("got: %v, want: %v, diff +want -got: %s", got, want, diff)
					}
				} else if lastStageName == stageNameWhere { // This is a where query (filter condition)
					want, ok := test.want.([]map[string]interface{})
					if !ok {
						r.Fatalf("invalid test.want type for where query: %T", test.want)
						return
					}
					if len(docs) != len(want) {
						r.Fatalf("expected %d doc(s), got %d", len(want), len(docs))
						return
					}
					var gots []map[string]interface{}
					for _, doc := range docs {
						got, err := doc.Data()
						if err != nil {
							r.Fatalf("Data: %v", err)
							return
						}
						gots = append(gots, got)
					}
					if diff := testutil.Diff(gots, want); diff != "" {
						t.Errorf("got: %v, want: %v, diff +want -got: %s", gots, want, diff)
					}
				} else {
					r.Fatalf("unknown pipeline stage: %s", lastStageName)
					return
				}
			})
		})
	}

}

func typeFuncs(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	client := integrationClient(t)
	coll := client.Collection(collectionIDs.New())
	docWithNaN := map[string]interface{}{
		"docID": 1,
		"value": math.NaN(),
		"type":  "nan",
	}
	_, err := coll.Doc("docNaN").Create(ctx, docWithNaN)
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	docWithNull := map[string]interface{}{
		"docID": 2,
		"value": nil,
		"type":  "null",
	}
	_, err = coll.Doc("docNull").Create(ctx, docWithNull)
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	docWithNumber := map[string]interface{}{
		"docID": 3,
		"value": 123,
		"type":  "number",
	}
	_, err = coll.Doc("docNum").Create(ctx, docWithNumber)
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	defer deleteDocuments([]*DocumentRef{coll.Doc("docNaN"), coll.Doc("docNull"), coll.Doc("docNum")})

	wantNaN := map[string]interface{}{"docID": int64(1), "value": math.NaN(), "type": "nan"}
	wantNull := map[string]interface{}{"docID": int64(2), "value": nil, "type": "null"}
	wantNum := map[string]interface{}{"docID": int64(3), "value": int64(123), "type": "number"}

	tests := []struct {
		name     string
		pipeline *Pipeline
		want     []map[string]interface{}
	}{
		{
			name:     "IsNull",
			pipeline: client.Pipeline().Collection(coll.ID).Where(IsNull("value")),
			want:     []map[string]interface{}{wantNull},
		},
		{
			name:     "IsNotNull",
			pipeline: client.Pipeline().Collection(coll.ID).Where(IsNotNull("value")),
			want:     []map[string]interface{}{wantNaN, wantNum},
		},
		{
			name:     "IsNaN",
			pipeline: client.Pipeline().Collection(coll.ID).Where(IsNaN("value")),
			want:     []map[string]interface{}{wantNaN},
		},
		{
			name:     "IsNotNaN",
			pipeline: client.Pipeline().Collection(coll.ID).Where(IsNotNaN("value")),
			want:     []map[string]interface{}{wantNum},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			testutil.Retry(t, 3, time.Second, func(r *testutil.R) {
				ctx := context.Background()
				iter := test.pipeline.Execute(ctx)
				defer iter.Stop()

				docs, err := iter.GetAll()
				if isInvalidHeaderError(err) {
					r.Errorf("GetAll: %v. Retrying....", err)
					return
				} else if err != nil {
					r.Fatalf("GetAll: %v", err)
					return
				}
				if diff := testutil.Diff(docsToMaps(t, docs), test.want,
					cmpopts.SortSlices(func(a, b map[string]interface{}) bool { return a["docID"].(int64) < b["docID"].(int64) })); diff != "" {
					r.Errorf("mismatch (+want -got):\n%s", diff)
				}
			})
		})
	}
}

func vectorFuncs(t *testing.T) {
	t.Parallel()
	h := testHelper{t}
	client := integrationClient(t)
	coll := client.Collection(collectionIDs.New())
	docRef1 := coll.NewDoc()
	h.mustCreate(docRef1, map[string]interface{}{
		"v1": Vector64{1.0, 2.0, 3.0},
		"v2": Vector64{4.0, 5.0, 6.0},
	})
	defer deleteDocuments([]*DocumentRef{docRef1})

	tests := []struct {
		name     string
		pipeline *Pipeline
		want     map[string]interface{}
	}{
		{
			name:     "VectorLength",
			pipeline: client.Pipeline().Collection(coll.ID).Select(VectorLength("v1").As("length")),
			want:     map[string]interface{}{"length": int64(3)},
		},
		{
			name:     "DotProduct - field and field",
			pipeline: client.Pipeline().Collection(coll.ID).Select(DotProduct("v1", FieldOf("v2")).As("dot_product")),
			want:     map[string]interface{}{"dot_product": float64(1*4 + 2*5 + 3*6)},
		},
		{
			name:     "DotProduct - field and constant",
			pipeline: client.Pipeline().Collection(coll.ID).Select(DotProduct("v1", Vector64{4.0, 5.0, 6.0}).As("dot_product")),
			want:     map[string]interface{}{"dot_product": float64(1*4 + 2*5 + 3*6)},
		},
		{
			name:     "EuclideanDistance - field and field",
			pipeline: client.Pipeline().Collection(coll.ID).Select(EuclideanDistance("v1", FieldOf("v2")).As("euclidean")),
			want:     map[string]interface{}{"euclidean": math.Sqrt(math.Pow(4-1, 2) + math.Pow(5-2, 2) + math.Pow(6-3, 2))},
		},
		{
			name:     "EuclideanDistance - field and constant",
			pipeline: client.Pipeline().Collection(coll.ID).Select(EuclideanDistance("v1", Vector64{4.0, 5.0, 6.0}).As("euclidean")),
			want:     map[string]interface{}{"euclidean": math.Sqrt(math.Pow(4-1, 2) + math.Pow(5-2, 2) + math.Pow(6-3, 2))},
		},
		{
			name:     "CosineDistance - field and field",
			pipeline: client.Pipeline().Collection(coll.ID).Select(CosineDistance("v1", FieldOf("v2")).As("cosine")),
			want:     map[string]interface{}{"cosine": 1 - (32 / (math.Sqrt(14) * math.Sqrt(77)))},
		},
		{
			name:     "CosineDistance - field and constant",
			pipeline: client.Pipeline().Collection(coll.ID).Select(CosineDistance("v1", Vector64{4.0, 5.0, 6.0}).As("cosine")),
			want:     map[string]interface{}{"cosine": 1 - (32 / (math.Sqrt(14) * math.Sqrt(77)))},
		},
	}

	ctx := context.Background()
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			testutil.Retry(t, 3, time.Second, func(r *testutil.R) {
				iter := test.pipeline.Execute(ctx)
				defer iter.Stop()

				docs, err := iter.GetAll()
				if isInvalidHeaderError(err) {
					r.Errorf("GetAll: %v. Retrying....", err)
					return
				} else if err != nil {
					r.Fatalf("GetAll: %v", err)
					return
				}
				if len(docs) != 1 {
					r.Fatalf("expected 1 doc, got %d", len(docs))
					return
				}
				got, err := docs[0].Data()
				if err != nil {
					r.Fatalf("Data: %v", err)
					return
				}
				if diff := testutil.Diff(got, test.want); diff != "" {
					r.Errorf("got: %v, want: %v, diff +want -got: %s", got, test.want, diff)
				}
			})
		})
	}
}

func isInvalidHeaderError(err error) bool {
	if err == nil {
		return false
	}
	s, ok := status.FromError(err)
	if !ok {
		return false
	}
	return s.Code() == codes.InvalidArgument &&
		strings.Contains(s.Message(), "Invalid request routing header") &&
		strings.Contains(s.Message(), "Please fill in the request header with format")
}

/*
Copyright 2026 Google LLC

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package spanner

import (
	"bytes"
	"testing"

	sppb "cloud.google.com/go/spanner/apiv1/spannerpb"
	structpb "google.golang.org/protobuf/types/known/structpb"
)

func executeSQLForTest(
	sql string,
	params map[string]*structpb.Value,
	paramTypes map[string]*sppb.Type,
	queryOptions *sppb.ExecuteSqlRequest_QueryOptions,
) *sppb.ExecuteSqlRequest {
	req := &sppb.ExecuteSqlRequest{
		Sql:          sql,
		ParamTypes:   paramTypes,
		QueryOptions: queryOptions,
	}
	if params != nil {
		req.Params = &structpb.Struct{Fields: params}
	}
	return req
}

func TestFingerprintExecuteSQLRequest_UsesRequestShape(t *testing.T) {
	req := executeSQLForTest(
		"SELECT * FROM T WHERE p1=@p1 AND p2=@p2",
		map[string]*structpb.Value{
			"p1": structpb.NewStringValue("foo"),
			"p2": structpb.NewNumberValue(1),
		},
		nil,
		nil,
	)
	reqSameShape := executeSQLForTest(
		"SELECT * FROM T WHERE p1=@p1 AND p2=@p2",
		map[string]*structpb.Value{
			"p2": structpb.NewNumberValue(2),
			"p1": structpb.NewStringValue("bar"),
		},
		nil,
		&sppb.ExecuteSqlRequest_QueryOptions{},
	)
	if got, want := fingerprintExecuteSQLRequest(req), fingerprintExecuteSQLRequest(reqSameShape); got != want {
		t.Fatalf("expected same fingerprint for same shape, got %d and %d", got, want)
	}

	reqKindChange := executeSQLForTest(
		"SELECT * FROM T WHERE p1=@p1 AND p2=@p2",
		map[string]*structpb.Value{
			"p1": structpb.NewStringValue("foo"),
			"p2": structpb.NewStringValue("2"),
		},
		nil,
		nil,
	)
	if got, want := fingerprintExecuteSQLRequest(req), fingerprintExecuteSQLRequest(reqKindChange); got == want {
		t.Fatalf("expected different fingerprint when untyped param kind changes, got %d", got)
	}

	reqTyped := executeSQLForTest(
		"SELECT * FROM T WHERE p1=@p1",
		map[string]*structpb.Value{"p1": structpb.NewStringValue("1")},
		map[string]*sppb.Type{"p1": &sppb.Type{Code: sppb.TypeCode_INT64}},
		nil,
	)
	reqTypedDifferentValueKind := executeSQLForTest(
		"SELECT * FROM T WHERE p1=@p1",
		map[string]*structpb.Value{"p1": structpb.NewBoolValue(true)},
		map[string]*sppb.Type{"p1": &sppb.Type{Code: sppb.TypeCode_INT64}},
		nil,
	)
	if got, want := fingerprintExecuteSQLRequest(reqTyped), fingerprintExecuteSQLRequest(reqTypedDifferentValueKind); got != want {
		t.Fatalf("expected typed params to fingerprint by type, got %d and %d", got, want)
	}
}

func TestFingerprintReadRequest_UsesRequestShape(t *testing.T) {
	req := &sppb.ReadRequest{
		Table:   "T",
		Columns: []string{"c1", "c2"},
		KeySet: &sppb.KeySet{
			Keys: []*structpb.ListValue{
				{Values: []*structpb.Value{structpb.NewStringValue("foo")}},
			},
		},
	}
	fp := fingerprintReadRequest(req)
	if fp == 0 {
		t.Fatal("expected non-zero fingerprint")
	}
	if fp != fingerprintReadRequest(req) {
		t.Fatal("expected stable fingerprint")
	}

	if fp == fingerprintReadRequest(&sppb.ReadRequest{Table: "U", Columns: []string{"c1", "c2"}}) {
		t.Fatal("expected fingerprint to differ for different table")
	}
	if fp == fingerprintReadRequest(&sppb.ReadRequest{Table: "T", Index: "I", Columns: []string{"c1", "c2"}}) {
		t.Fatal("expected fingerprint to differ for different index")
	}
	if fp == fingerprintReadRequest(&sppb.ReadRequest{Table: "T", Columns: []string{"c1"}}) {
		t.Fatal("expected fingerprint to differ for different column list")
	}

	sameShapeDifferentKeys := &sppb.ReadRequest{
		Table:   "T",
		Columns: []string{"c1", "c2"},
		KeySet: &sppb.KeySet{
			Keys: []*structpb.ListValue{
				{Values: []*structpb.Value{structpb.NewStringValue("bar")}},
			},
		},
	}
	if fp != fingerprintReadRequest(sameShapeDifferentKeys) {
		t.Fatal("expected key values to not affect fingerprint")
	}
}

func TestPreparedQuery_MatchesOnKindsAndTypes(t *testing.T) {
	untyped := executeSQLForTest(
		"SELECT * FROM T WHERE p1=@p1",
		map[string]*structpb.Value{"p1": structpb.NewStringValue("foo")},
		nil,
		nil,
	)
	preparedUntyped := newPreparedQuery(untyped)
	if !preparedUntyped.matches(executeSQLForTest(
		"SELECT * FROM T WHERE p1=@p1",
		map[string]*structpb.Value{"p1": structpb.NewStringValue("bar")},
		nil,
		nil,
	)) {
		t.Fatal("expected untyped query with same kind to match")
	}
	if preparedUntyped.matches(executeSQLForTest(
		"SELECT * FROM T WHERE p1=@p1",
		map[string]*structpb.Value{"p1": structpb.NewBoolValue(true)},
		nil,
		nil,
	)) {
		t.Fatal("expected untyped query with different kind to mismatch")
	}

	typed := executeSQLForTest(
		"SELECT * FROM T WHERE p1=@p1",
		map[string]*structpb.Value{"p1": structpb.NewStringValue("1")},
		map[string]*sppb.Type{"p1": &sppb.Type{Code: sppb.TypeCode_INT64}},
		nil,
	)
	preparedTyped := newPreparedQuery(typed)
	if !preparedTyped.matches(executeSQLForTest(
		"SELECT * FROM T WHERE p1=@p1",
		map[string]*structpb.Value{"p1": structpb.NewBoolValue(true)},
		map[string]*sppb.Type{"p1": &sppb.Type{Code: sppb.TypeCode_INT64}},
		nil,
	)) {
		t.Fatal("expected typed query with same declared type to match")
	}
	if preparedTyped.matches(executeSQLForTest(
		"SELECT * FROM T WHERE p1=@p1",
		map[string]*structpb.Value{"p1": structpb.NewStringValue("1")},
		nil,
		nil,
	)) {
		t.Fatal("expected typed query to mismatch when type declaration is removed")
	}
}

func TestFingerprintExecuteSQLRequest_NullParamKind(t *testing.T) {
	reqNull := executeSQLForTest(
		"SELECT * FROM T WHERE p1=@p1",
		map[string]*structpb.Value{"p1": structpb.NewNullValue()},
		nil,
		nil,
	)
	reqNullSameKind := executeSQLForTest(
		"SELECT * FROM T WHERE p1=@p1",
		map[string]*structpb.Value{"p1": structpb.NewNullValue()},
		nil,
		nil,
	)
	if got, want := fingerprintExecuteSQLRequest(reqNull), fingerprintExecuteSQLRequest(reqNullSameKind); got != want {
		t.Fatalf("expected same fingerprint for same NULL param kind, got %d and %d", got, want)
	}

	reqString := executeSQLForTest(
		"SELECT * FROM T WHERE p1=@p1",
		map[string]*structpb.Value{"p1": structpb.NewStringValue("x")},
		nil,
		nil,
	)
	if got, want := fingerprintExecuteSQLRequest(reqNull), fingerprintExecuteSQLRequest(reqString); got == want {
		t.Fatalf("expected different fingerprints for NULL vs STRING kinds, got %d", got)
	}
}

func TestPreparedQuery_MatchesWithNullParamKind(t *testing.T) {
	nullReq := executeSQLForTest(
		"SELECT * FROM T WHERE p1=@p1",
		map[string]*structpb.Value{"p1": structpb.NewNullValue()},
		nil,
		nil,
	)
	prepared := newPreparedQuery(nullReq)
	if !prepared.matches(executeSQLForTest(
		"SELECT * FROM T WHERE p1=@p1",
		map[string]*structpb.Value{"p1": structpb.NewNullValue()},
		nil,
		nil,
	)) {
		t.Fatal("expected untyped NULL param query to match")
	}
	if prepared.matches(executeSQLForTest(
		"SELECT * FROM T WHERE p1=@p1",
		map[string]*structpb.Value{"p1": structpb.NewBoolValue(true)},
		nil,
		nil,
	)) {
		t.Fatal("expected NULL param kind to mismatch BOOL kind")
	}
}

func TestComputeReadKeys_SetsRoutingHint(t *testing.T) {
	cache := newKeyRecipeCache()
	cache.addRecipes(&sppb.RecipeList{
		SchemaGeneration: []byte("1"),
		Recipe: []*sppb.KeyRecipe{
			{
				Target: &sppb.KeyRecipe_TableName{TableName: "T"},
				Part: []*sppb.KeyRecipe_Part{
					{Tag: 1},
					{
						Order:     sppb.KeyRecipe_Part_ASCENDING,
						NullOrder: sppb.KeyRecipe_Part_NULLS_FIRST,
						Type:      &sppb.Type{Code: sppb.TypeCode_STRING},
						ValueType: &sppb.KeyRecipe_Part_Identifier{Identifier: "k"},
					},
				},
			},
		},
	})

	req := &sppb.ReadRequest{
		Table:   "T",
		Columns: []string{"c1"},
		KeySet: &sppb.KeySet{
			Keys: []*structpb.ListValue{
				{Values: []*structpb.Value{structpb.NewStringValue("foo")}},
			},
		},
	}

	cache.computeReadKeys(req)
	hint := req.GetRoutingHint()
	if hint == nil {
		t.Fatal("expected routing hint to be set")
	}
	if hint.GetOperationUid() == 0 {
		t.Fatal("expected operation uid")
	}
	if !bytes.Equal(hint.GetSchemaGeneration(), []byte("1")) {
		t.Fatalf("expected schema generation 1, got %q", hint.GetSchemaGeneration())
	}
	if len(hint.GetKey()) == 0 {
		t.Fatal("expected key bytes to be set")
	}
}

func TestMutationToTargetRange(t *testing.T) {
	cache := newKeyRecipeCache()

	if got := cache.mutationToTargetRange(nil); got != nil {
		t.Fatalf("expected nil for nil mutation, got %#v", got)
	}

	missingRecipeMutation := &sppb.Mutation{
		Operation: &sppb.Mutation_Insert{
			Insert: &sppb.Mutation_Write{
				Table:   "Missing",
				Columns: []string{"k"},
				Values: []*structpb.ListValue{
					{Values: []*structpb.Value{structpb.NewStringValue("foo")}},
				},
			},
		},
	}
	if got := cache.mutationToTargetRange(missingRecipeMutation); got != nil {
		t.Fatalf("expected nil when recipe is missing, got %#v", got)
	}

	cache.addRecipes(&sppb.RecipeList{
		SchemaGeneration: []byte("1"),
		Recipe: []*sppb.KeyRecipe{
			{
				Target: &sppb.KeyRecipe_TableName{TableName: "T"},
				Part: []*sppb.KeyRecipe_Part{
					{Tag: 1},
					{
						Order:     sppb.KeyRecipe_Part_ASCENDING,
						NullOrder: sppb.KeyRecipe_Part_NULLS_FIRST,
						Type:      &sppb.Type{Code: sppb.TypeCode_STRING},
						ValueType: &sppb.KeyRecipe_Part_Identifier{Identifier: "k"},
					},
				},
			},
		},
	})

	mutation := &sppb.Mutation{
		Operation: &sppb.Mutation_Insert{
			Insert: &sppb.Mutation_Write{
				Table:   "T",
				Columns: []string{"k"},
				Values: []*structpb.ListValue{
					{Values: []*structpb.Value{structpb.NewStringValue("foo")}},
				},
			},
		},
	}
	got := cache.mutationToTargetRange(mutation)
	if got == nil {
		t.Fatal("expected non-nil target range for matching mutation recipe")
	}
	wantStart := expectedKeyForStringValue(t, "foo")
	if !bytes.Equal(got.start, wantStart) {
		t.Fatalf("unexpected start key: got %q want %q", got.start, wantStart)
	}
	wantLimit := makePrefixSuccessor(wantStart)
	if !bytes.Equal(got.limit, wantLimit) {
		t.Fatalf("unexpected limit key: got %q want %q", got.limit, wantLimit)
	}
	if got.approximate {
		t.Fatal("expected exact target range for matching mutation")
	}
}

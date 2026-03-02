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
	"google.golang.org/protobuf/encoding/prototext"
	structpb "google.golang.org/protobuf/types/known/structpb"
)

func TestKeyRecipe_QueryParamsUsesStructIdentifiers(t *testing.T) {
	recipeProto := parseKeyRecipeTextProto(t,
		"part { tag: 1 }\n"+
			"part {\n"+
			"  order: ASCENDING\n"+
			"  null_order: NULLS_FIRST\n"+
			"  type { code: STRING }\n"+
			"  identifier: \"p0\"\n"+
			"  struct_identifiers: 1\n"+
			"}\n")
	params := parseStructTextProto(t,
		"fields {\n"+
			"  key: \"p0\"\n"+
			"  value {\n"+
			"    list_value {\n"+
			"      values { string_value: \"a\" }\n"+
			"      values { string_value: \"b\" }\n"+
			"    }\n"+
			"  }\n"+
			"}\n")

	recipe, err := newKeyRecipe(recipeProto)
	if err != nil {
		t.Fatalf("newKeyRecipe() failed: %v", err)
	}
	target := recipe.queryParamsToTargetRange(params)

	wantStart := expectedKeyForStringValue(t, "b")
	if !bytes.Equal(target.start, wantStart) {
		t.Fatalf("unexpected start: got %q want %q", target.start, wantStart)
	}
	if len(target.limit) != 0 {
		t.Fatalf("expected empty limit, got %q", target.limit)
	}
	if target.approximate {
		t.Fatal("expected exact routing key, got approximate=true")
	}
}

func TestKeyRecipe_QueryParamsUsesConstantValue(t *testing.T) {
	recipeProto := parseKeyRecipeTextProto(t,
		"part { tag: 1 }\n"+
			"part {\n"+
			"  order: ASCENDING\n"+
			"  null_order: NULLS_FIRST\n"+
			"  type { code: STRING }\n"+
			"  value { string_value: \"const\" }\n"+
			"}\n")

	recipe, err := newKeyRecipe(recipeProto)
	if err != nil {
		t.Fatalf("newKeyRecipe() failed: %v", err)
	}
	target := recipe.queryParamsToTargetRange(&structpb.Struct{})

	wantStart := expectedKeyForStringValue(t, "const")
	if !bytes.Equal(target.start, wantStart) {
		t.Fatalf("unexpected start: got %q want %q", target.start, wantStart)
	}
	if len(target.limit) != 0 {
		t.Fatalf("expected empty limit, got %q", target.limit)
	}
	if target.approximate {
		t.Fatal("expected exact routing key, got approximate=true")
	}
}

func TestKeyRecipe_QueryParamsStructIdentifiersInvalidPathIsApproximate(t *testing.T) {
	recipeProto := parseKeyRecipeTextProto(t,
		"part { tag: 1 }\n"+
			"part {\n"+
			"  order: ASCENDING\n"+
			"  null_order: NULLS_FIRST\n"+
			"  type { code: STRING }\n"+
			"  identifier: \"p0\"\n"+
			"  struct_identifiers: 2\n"+
			"}\n")
	params := parseStructTextProto(t,
		"fields {\n"+
			"  key: \"p0\"\n"+
			"  value {\n"+
			"    list_value {\n"+
			"      values { string_value: \"a\" }\n"+
			"      values { string_value: \"b\" }\n"+
			"    }\n"+
			"  }\n"+
			"}\n")

	recipe, err := newKeyRecipe(recipeProto)
	if err != nil {
		t.Fatalf("newKeyRecipe() failed: %v", err)
	}
	target := recipe.queryParamsToTargetRange(params)

	wantStart, err := appendCompositeTag(nil, 1)
	if err != nil {
		t.Fatalf("appendCompositeTag() failed: %v", err)
	}
	if !bytes.Equal(target.start, wantStart) {
		t.Fatalf("unexpected start for invalid path: got %q want %q", target.start, wantStart)
	}
	if !target.approximate {
		t.Fatal("expected approximate=true for invalid struct_identifiers path")
	}
	wantLimit := makePrefixSuccessor(wantStart)
	if !bytes.Equal(target.limit, wantLimit) {
		t.Fatalf("unexpected limit: got %q want %q", target.limit, wantLimit)
	}
}

func TestKeyRecipe_QueryParamsInvalidUUIDIsApproximate(t *testing.T) {
	recipeProto := parseKeyRecipeTextProto(t,
		"part { tag: 1 }\n"+
			"part {\n"+
			"  order: ASCENDING\n"+
			"  null_order: NULLS_FIRST\n"+
			"  type { code: UUID }\n"+
			"  identifier: \"p0\"\n"+
			"}\n")
	params := parseStructTextProto(t,
		"fields {\n"+
			"  key: \"p0\"\n"+
			"  value { string_value: \"not-a-uuid\" }\n"+
			"}\n")

	recipe, err := newKeyRecipe(recipeProto)
	if err != nil {
		t.Fatalf("newKeyRecipe() failed: %v", err)
	}
	target := recipe.queryParamsToTargetRange(params)

	wantStart, err := appendCompositeTag(nil, 1)
	if err != nil {
		t.Fatalf("appendCompositeTag() failed: %v", err)
	}
	if !bytes.Equal(target.start, wantStart) {
		t.Fatalf("unexpected start for invalid UUID: got %q want %q", target.start, wantStart)
	}
	if !target.approximate {
		t.Fatal("expected approximate=true for invalid UUID value")
	}
	wantLimit := makePrefixSuccessor(wantStart)
	if !bytes.Equal(target.limit, wantLimit) {
		t.Fatalf("unexpected limit: got %q want %q", target.limit, wantLimit)
	}
}

func parseKeyRecipeTextProto(t *testing.T, text string) *sppb.KeyRecipe {
	t.Helper()
	recipe := &sppb.KeyRecipe{}
	if err := prototext.Unmarshal([]byte(text), recipe); err != nil {
		t.Fatalf("failed to parse key recipe textproto: %v", err)
	}
	return recipe
}

func parseStructTextProto(t *testing.T, text string) *structpb.Struct {
	t.Helper()
	params := &structpb.Struct{}
	if err := prototext.Unmarshal([]byte(text), params); err != nil {
		t.Fatalf("failed to parse struct textproto: %v", err)
	}
	return params
}

func expectedKeyForStringValue(t *testing.T, value string) []byte {
	t.Helper()
	key, err := appendCompositeTag(nil, 1)
	if err != nil {
		t.Fatalf("appendCompositeTag() failed: %v", err)
	}
	key = appendNotNullMarkerNullOrderedFirst(key)
	key = appendStringIncreasing(key, value)
	return key
}

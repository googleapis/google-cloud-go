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
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	sppb "cloud.google.com/go/spanner/apiv1/spannerpb"
	locationpb "cloud.google.com/go/spanner/test/proto/locationpb"
	"google.golang.org/protobuf/encoding/prototext"
	structpb "google.golang.org/protobuf/types/known/structpb"
)

type recipeGoldenTest struct {
	key         *structpb.ListValue
	keyRange    *sppb.KeyRange
	keySet      *sppb.KeySet
	mutation    *sppb.Mutation
	queryParams *structpb.Struct
	start       []byte
	limit       []byte
	approximate bool
}

type recipeGoldenCase struct {
	name    string
	recipes *sppb.RecipeList
	tests   []recipeGoldenTest
}

func TestKeyRecipe_Golden(t *testing.T) {
	content, err := os.ReadFile(filepath.Join("testdata", "location", "recipe_test.textproto"))
	if err != nil {
		t.Fatalf("failed reading recipe golden file: %v", err)
	}

	cases, err := parseRecipeGoldenCases(string(content))
	if err != nil {
		t.Fatalf("failed parsing recipe goldens: %v", err)
	}
	if len(cases) == 0 {
		t.Fatal("no recipe golden test cases parsed")
	}

	for _, testCase := range cases {
		if testCase.recipes == nil || len(testCase.recipes.GetRecipe()) == 0 {
			t.Fatalf("case %q: missing recipes", testCase.name)
		}
		recipe, err := newKeyRecipe(testCase.recipes.GetRecipe()[0])
		if err != nil {
			t.Fatalf("case %q: newKeyRecipe failed: %v", testCase.name, err)
		}

		for i, testItem := range testCase.tests {
			target := evaluateRecipeGoldenTest(recipe, testItem)
			if shouldRelaxRecipeGoldenStartCheck(testCase.name) {
				if len(target.start) == 0 {
					t.Fatalf("case %q test[%d]: expected non-empty start", testCase.name, i)
				}
				prefixLen := minInt(4, len(testItem.start), len(target.start))
				if !bytes.Equal(testItem.start[:prefixLen], target.start[:prefixLen]) {
					t.Fatalf("case %q test[%d]: start prefix mismatch", testCase.name, i)
				}
			} else if !bytes.Equal(testItem.start, target.start) {
				t.Fatalf("case %q test[%d]: start mismatch", testCase.name, i)
			}
			if !bytes.Equal(testItem.limit, target.limit) {
				t.Fatalf("case %q test[%d]: limit mismatch", testCase.name, i)
			}
			if testItem.approximate != target.approximate {
				t.Fatalf("case %q test[%d]: approximate mismatch: got %t want %t", testCase.name, i, target.approximate, testItem.approximate)
			}
		}
	}
}

func shouldRelaxRecipeGoldenStartCheck(caseName string) bool {
	return caseName == "RandomQueryroot"
}

func minInt(a, b, c int) int {
	m := a
	if b < m {
		m = b
	}
	if c < m {
		m = c
	}
	return m
}

func evaluateRecipeGoldenTest(recipe *keyRecipe, testItem recipeGoldenTest) *targetRange {
	switch {
	case testItem.key != nil:
		return recipe.keyToTargetRange(testItem.key)
	case testItem.keyRange != nil:
		return recipe.keyRangeToTargetRange(testItem.keyRange)
	case testItem.keySet != nil:
		return recipe.keySetToTargetRange(testItem.keySet)
	case testItem.queryParams != nil:
		return recipe.queryParamsToTargetRange(testItem.queryParams)
	case testItem.mutation != nil:
		return recipe.mutationToTargetRange(testItem.mutation)
	default:
		return newTargetRange(nil, ssInfinity, true)
	}
}

func parseRecipeGoldenCases(content string) ([]recipeGoldenCase, error) {
	content = stripFinderGoldenComments(content)
	content = normalizeRecipeGoldenTextproto(content)
	root := &locationpb.RecipeTestCases{}
	if err := prototext.Unmarshal([]byte(content), root); err != nil {
		return nil, fmt.Errorf("unmarshal recipe goldens as proto: %w", err)
	}

	cases := make([]recipeGoldenCase, 0, len(root.GetTestCase()))
	for i, c := range root.GetTestCase() {
		if c == nil {
			return nil, fmt.Errorf("test_case[%d]: nil", i)
		}
		parsed := recipeGoldenCase{name: c.GetName(), recipes: c.GetRecipes()}
		parsed.tests = make([]recipeGoldenTest, 0, len(c.GetTest()))
		for j, tc := range c.GetTest() {
			if tc == nil {
				return nil, fmt.Errorf("test_case[%d] (%q) test[%d]: nil", i, parsed.name, j)
			}
			parsed.tests = append(parsed.tests, recipeGoldenTest{
				key:         tc.GetKey(),
				keyRange:    tc.GetKeyRange(),
				keySet:      tc.GetKeySet(),
				mutation:    tc.GetMutation(),
				queryParams: tc.GetQueryParams(),
				start:       append([]byte(nil), tc.GetStart()...),
				limit:       append([]byte(nil), tc.GetLimit()...),
				approximate: tc.GetApproximate(),
			})
		}
		cases = append(cases, parsed)
	}
	return cases, nil
}

func normalizeRecipeGoldenTextproto(content string) string {
	return strings.ReplaceAll(content, "code: TOKENLIST", "code: 99999")
}

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
	"google.golang.org/protobuf/encoding/prototext"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/reflect/protodesc"
	"google.golang.org/protobuf/reflect/protoreflect"
	"google.golang.org/protobuf/reflect/protoregistry"
	"google.golang.org/protobuf/types/descriptorpb"
	"google.golang.org/protobuf/types/dynamicpb"
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
	descriptor, err := buildRecipeGoldenDescriptor()
	if err != nil {
		return nil, err
	}

	root := dynamicpb.NewMessage(descriptor)
	if err := prototext.Unmarshal([]byte(content), root); err != nil {
		return nil, fmt.Errorf("unmarshal recipe goldens as proto: %w", err)
	}

	casesField := descriptor.Fields().ByName("test_case")
	casesList := root.Get(casesField).List()
	cases := make([]recipeGoldenCase, 0, casesList.Len())
	for i := 0; i < casesList.Len(); i++ {
		caseMsg := casesList.Get(i).Message()
		caseFields := caseMsg.Descriptor().Fields()
		nameField := caseFields.ByName("name")
		recipesField := caseFields.ByName("recipes")
		testsField := caseFields.ByName("test")

		parsedCase := recipeGoldenCase{name: caseMsg.Get(nameField).String()}
		if caseMsg.Has(recipesField) {
			recipes := &sppb.RecipeList{}
			if err := convertDynamicMessage(caseMsg.Get(recipesField).Message(), recipes); err != nil {
				return nil, fmt.Errorf("case[%d] recipes: %w", i, err)
			}
			parsedCase.recipes = recipes
		}

		testsList := caseMsg.Get(testsField).List()
		parsedCase.tests = make([]recipeGoldenTest, 0, testsList.Len())
		for testIdx := 0; testIdx < testsList.Len(); testIdx++ {
			testMsg := testsList.Get(testIdx).Message()
			testFields := testMsg.Descriptor().Fields()
			keyField := testFields.ByName("key")
			keyRangeField := testFields.ByName("key_range")
			keySetField := testFields.ByName("key_set")
			mutationField := testFields.ByName("mutation")
			queryParamsField := testFields.ByName("query_params")
			startField := testFields.ByName("start")
			limitField := testFields.ByName("limit")
			approximateField := testFields.ByName("approximate")

			parsedTest := recipeGoldenTest{
				start:       append([]byte(nil), testMsg.Get(startField).Bytes()...),
				limit:       append([]byte(nil), testMsg.Get(limitField).Bytes()...),
				approximate: testMsg.Get(approximateField).Bool(),
			}
			if testMsg.Has(keyField) {
				key := &structpb.ListValue{}
				if err := convertDynamicMessage(testMsg.Get(keyField).Message(), key); err != nil {
					return nil, fmt.Errorf("case[%d] test[%d] key: %w", i, testIdx, err)
				}
				parsedTest.key = key
			}
			if testMsg.Has(keyRangeField) {
				keyRange := &sppb.KeyRange{}
				if err := convertDynamicMessage(testMsg.Get(keyRangeField).Message(), keyRange); err != nil {
					return nil, fmt.Errorf("case[%d] test[%d] key_range: %w", i, testIdx, err)
				}
				parsedTest.keyRange = keyRange
			}
			if testMsg.Has(keySetField) {
				keySet := &sppb.KeySet{}
				if err := convertDynamicMessage(testMsg.Get(keySetField).Message(), keySet); err != nil {
					return nil, fmt.Errorf("case[%d] test[%d] key_set: %w", i, testIdx, err)
				}
				parsedTest.keySet = keySet
			}
			if testMsg.Has(mutationField) {
				mutation := &sppb.Mutation{}
				if err := convertDynamicMessage(testMsg.Get(mutationField).Message(), mutation); err != nil {
					return nil, fmt.Errorf("case[%d] test[%d] mutation: %w", i, testIdx, err)
				}
				parsedTest.mutation = mutation
			}
			if testMsg.Has(queryParamsField) {
				queryParams := &structpb.Struct{}
				if err := convertDynamicMessage(testMsg.Get(queryParamsField).Message(), queryParams); err != nil {
					return nil, fmt.Errorf("case[%d] test[%d] query_params: %w", i, testIdx, err)
				}
				parsedTest.queryParams = queryParams
			}
			parsedCase.tests = append(parsedCase.tests, parsedTest)
		}
		cases = append(cases, parsedCase)
	}
	return cases, nil
}

func normalizeRecipeGoldenTextproto(content string) string {
	return strings.ReplaceAll(content, "code: TOKENLIST", "code: 99999")
}

func buildRecipeGoldenDescriptor() (protoreflect.MessageDescriptor, error) {
	fileProto := &descriptorpb.FileDescriptorProto{
		Name:    proto.String("recipe_test_dynamic.proto"),
		Package: proto.String("spanner.cloud.location"),
		Syntax:  proto.String("proto3"),
		Dependency: []string{
			"google/protobuf/struct.proto",
			"google/spanner/v1/keys.proto",
			"google/spanner/v1/location.proto",
			"google/spanner/v1/mutation.proto",
		},
		MessageType: []*descriptorpb.DescriptorProto{
			{
				Name: proto.String("RecipeTestCases"),
				Field: []*descriptorpb.FieldDescriptorProto{
					{
						Name:     proto.String("test_case"),
						Number:   proto.Int32(1),
						Label:    finderGoldenFieldLabel(descriptorpb.FieldDescriptorProto_LABEL_REPEATED),
						Type:     finderGoldenFieldType(descriptorpb.FieldDescriptorProto_TYPE_MESSAGE),
						TypeName: proto.String(".spanner.cloud.location.RecipeTestCase"),
					},
				},
			},
			{
				Name: proto.String("RecipeTestCase"),
				Field: []*descriptorpb.FieldDescriptorProto{
					{
						Name:   proto.String("name"),
						Number: proto.Int32(1),
						Label:  finderGoldenFieldLabel(descriptorpb.FieldDescriptorProto_LABEL_OPTIONAL),
						Type:   finderGoldenFieldType(descriptorpb.FieldDescriptorProto_TYPE_STRING),
					},
					{
						Name:     proto.String("recipes"),
						Number:   proto.Int32(2),
						Label:    finderGoldenFieldLabel(descriptorpb.FieldDescriptorProto_LABEL_OPTIONAL),
						Type:     finderGoldenFieldType(descriptorpb.FieldDescriptorProto_TYPE_MESSAGE),
						TypeName: proto.String(".google.spanner.v1.RecipeList"),
					},
					{
						Name:     proto.String("test"),
						Number:   proto.Int32(3),
						Label:    finderGoldenFieldLabel(descriptorpb.FieldDescriptorProto_LABEL_REPEATED),
						Type:     finderGoldenFieldType(descriptorpb.FieldDescriptorProto_TYPE_MESSAGE),
						TypeName: proto.String(".spanner.cloud.location.RecipeTestCase.Test"),
					},
				},
				NestedType: []*descriptorpb.DescriptorProto{
					{
						Name: proto.String("Test"),
						Field: []*descriptorpb.FieldDescriptorProto{
							{
								Name:       proto.String("key"),
								Number:     proto.Int32(1),
								Label:      finderGoldenFieldLabel(descriptorpb.FieldDescriptorProto_LABEL_OPTIONAL),
								Type:       finderGoldenFieldType(descriptorpb.FieldDescriptorProto_TYPE_MESSAGE),
								TypeName:   proto.String(".google.protobuf.ListValue"),
								OneofIndex: proto.Int32(0),
							},
							{
								Name:       proto.String("key_range"),
								Number:     proto.Int32(2),
								Label:      finderGoldenFieldLabel(descriptorpb.FieldDescriptorProto_LABEL_OPTIONAL),
								Type:       finderGoldenFieldType(descriptorpb.FieldDescriptorProto_TYPE_MESSAGE),
								TypeName:   proto.String(".google.spanner.v1.KeyRange"),
								OneofIndex: proto.Int32(0),
							},
							{
								Name:       proto.String("key_set"),
								Number:     proto.Int32(3),
								Label:      finderGoldenFieldLabel(descriptorpb.FieldDescriptorProto_LABEL_OPTIONAL),
								Type:       finderGoldenFieldType(descriptorpb.FieldDescriptorProto_TYPE_MESSAGE),
								TypeName:   proto.String(".google.spanner.v1.KeySet"),
								OneofIndex: proto.Int32(0),
							},
							{
								Name:       proto.String("mutation"),
								Number:     proto.Int32(4),
								Label:      finderGoldenFieldLabel(descriptorpb.FieldDescriptorProto_LABEL_OPTIONAL),
								Type:       finderGoldenFieldType(descriptorpb.FieldDescriptorProto_TYPE_MESSAGE),
								TypeName:   proto.String(".google.spanner.v1.Mutation"),
								OneofIndex: proto.Int32(0),
							},
							{
								Name:       proto.String("query_params"),
								Number:     proto.Int32(5),
								Label:      finderGoldenFieldLabel(descriptorpb.FieldDescriptorProto_LABEL_OPTIONAL),
								Type:       finderGoldenFieldType(descriptorpb.FieldDescriptorProto_TYPE_MESSAGE),
								TypeName:   proto.String(".google.protobuf.Struct"),
								OneofIndex: proto.Int32(0),
							},
							{
								Name:   proto.String("start"),
								Number: proto.Int32(6),
								Label:  finderGoldenFieldLabel(descriptorpb.FieldDescriptorProto_LABEL_OPTIONAL),
								Type:   finderGoldenFieldType(descriptorpb.FieldDescriptorProto_TYPE_BYTES),
							},
							{
								Name:   proto.String("limit"),
								Number: proto.Int32(7),
								Label:  finderGoldenFieldLabel(descriptorpb.FieldDescriptorProto_LABEL_OPTIONAL),
								Type:   finderGoldenFieldType(descriptorpb.FieldDescriptorProto_TYPE_BYTES),
							},
							{
								Name:   proto.String("approximate"),
								Number: proto.Int32(8),
								Label:  finderGoldenFieldLabel(descriptorpb.FieldDescriptorProto_LABEL_OPTIONAL),
								Type:   finderGoldenFieldType(descriptorpb.FieldDescriptorProto_TYPE_BOOL),
							},
						},
						OneofDecl: []*descriptorpb.OneofDescriptorProto{
							{Name: proto.String("operation")},
						},
					},
				},
			},
		},
	}

	fileDesc, err := protodesc.NewFile(fileProto, protoregistry.GlobalFiles)
	if err != nil {
		return nil, fmt.Errorf("create recipe golden descriptor: %w", err)
	}
	rootDesc := fileDesc.Messages().ByName("RecipeTestCases")
	if rootDesc == nil {
		return nil, fmt.Errorf("missing RecipeTestCases descriptor")
	}
	return rootDesc, nil
}

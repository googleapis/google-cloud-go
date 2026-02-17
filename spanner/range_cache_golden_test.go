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
	"fmt"
	"os"
	"path/filepath"
	"testing"

	sppb "cloud.google.com/go/spanner/apiv1/spannerpb"
	"github.com/google/go-cmp/cmp"
	"google.golang.org/protobuf/encoding/prototext"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/reflect/protodesc"
	"google.golang.org/protobuf/reflect/protoreflect"
	"google.golang.org/protobuf/reflect/protoregistry"
	"google.golang.org/protobuf/testing/protocmp"
	"google.golang.org/protobuf/types/descriptorpb"
	"google.golang.org/protobuf/types/dynamicpb"
)

type rangeCacheGoldenTest struct {
	leader       bool
	directedRead *sppb.DirectedReadOptions
	key          []byte
	limitKey     []byte
	mode         rangeMode
	minEntries   *int
	result       *sppb.RoutingHint
	server       string
}

type rangeCacheGoldenStep struct {
	update *sppb.CacheUpdate
	tests  []rangeCacheGoldenTest
}

type rangeCacheGoldenCase struct {
	name  string
	steps []rangeCacheGoldenStep
}

func TestKeyRangeCache_Golden(t *testing.T) {
	content, err := os.ReadFile(filepath.Join("testdata", "location", "range_cache_test.textproto"))
	if err != nil {
		t.Fatalf("failed reading range cache golden file: %v", err)
	}

	cases, err := parseRangeCacheGoldenCases(string(content))
	if err != nil {
		t.Fatalf("failed parsing range cache goldens: %v", err)
	}
	if len(cases) == 0 {
		t.Fatal("no range cache golden test cases parsed")
	}

	for _, testCase := range cases {
		cache := newKeyRangeCache(newFinderGoldenEndpointCache())
		cache.useDeterministicRandom()

		for stepIdx, step := range testCase.steps {
			if step.update != nil {
				cache.addRanges(step.update)
			}
			for testIdx, testStep := range step.tests {
				cache.setMinEntriesForRandomPick(defaultMinEntriesForRandomPick)
				if testStep.minEntries != nil {
					cache.setMinEntriesForRandomPick(*testStep.minEntries)
				}
				hint := &sppb.RoutingHint{
					Key:      append([]byte(nil), testStep.key...),
					LimitKey: append([]byte(nil), testStep.limitKey...),
				}
				directedRead := testStep.directedRead
				if directedRead == nil {
					directedRead = &sppb.DirectedReadOptions{}
				}
				endpoint := cache.fillRoutingHint(testStep.leader, testStep.mode, directedRead, hint)
				assertRangeCacheGoldenEndpoint(t, testCase.name, stepIdx, testIdx, testStep.server, endpoint)
				assertRangeCacheGoldenHint(t, testCase.name, stepIdx, testIdx, testStep.result, hint)
			}
		}
	}
}

func assertRangeCacheGoldenEndpoint(t *testing.T, caseName string, stepIdx, testIdx int, expected string, actual channelEndpoint) {
	t.Helper()
	coord := fmt.Sprintf("%s step=%d test=%d", caseName, stepIdx, testIdx)
	if expected == "" {
		if actual != nil {
			t.Fatalf("%s: expected no endpoint, got %q", coord, actual.Address())
		}
		return
	}
	if actual == nil {
		t.Fatalf("%s: expected endpoint %q, got nil", coord, expected)
	}
	if actual.Address() != expected {
		t.Fatalf("%s: expected endpoint %q, got %q", coord, expected, actual.Address())
	}
}

func assertRangeCacheGoldenHint(t *testing.T, caseName string, stepIdx, testIdx int, expected, actual *sppb.RoutingHint) {
	t.Helper()
	coord := fmt.Sprintf("%s step=%d test=%d", caseName, stepIdx, testIdx)
	if expected == nil {
		expected = &sppb.RoutingHint{}
	}
	if actual == nil {
		actual = &sppb.RoutingHint{}
	}
	if diff := cmp.Diff(expected, actual, protocmp.Transform()); diff != "" {
		t.Fatalf("%s: routing hint mismatch (-want +got):\n%s", coord, diff)
	}
}

func parseRangeCacheGoldenCases(content string) ([]rangeCacheGoldenCase, error) {
	content = stripFinderGoldenComments(content)
	descriptor, err := buildRangeCacheGoldenDescriptor()
	if err != nil {
		return nil, err
	}

	root := dynamicpb.NewMessage(descriptor)
	if err := prototext.Unmarshal([]byte(content), root); err != nil {
		return nil, fmt.Errorf("unmarshal range cache goldens as proto: %w", err)
	}

	casesField := descriptor.Fields().ByName("test_case")
	casesList := root.Get(casesField).List()
	cases := make([]rangeCacheGoldenCase, 0, casesList.Len())
	for i := 0; i < casesList.Len(); i++ {
		caseMsg := casesList.Get(i).Message()
		caseFields := caseMsg.Descriptor().Fields()
		nameField := caseFields.ByName("name")
		stepsField := caseFields.ByName("step")

		testCase := rangeCacheGoldenCase{name: caseMsg.Get(nameField).String()}
		stepsList := caseMsg.Get(stepsField).List()
		testCase.steps = make([]rangeCacheGoldenStep, 0, stepsList.Len())

		for stepIdx := 0; stepIdx < stepsList.Len(); stepIdx++ {
			stepMsg := stepsList.Get(stepIdx).Message()
			stepFields := stepMsg.Descriptor().Fields()
			updateField := stepFields.ByName("update")
			testsField := stepFields.ByName("test")

			step := rangeCacheGoldenStep{}
			if stepMsg.Has(updateField) {
				update := &sppb.CacheUpdate{}
				if err := convertDynamicMessage(stepMsg.Get(updateField).Message(), update); err != nil {
					return nil, fmt.Errorf("case[%d] step[%d] update: %w", i, stepIdx, err)
				}
				step.update = update
			}

			testsList := stepMsg.Get(testsField).List()
			step.tests = make([]rangeCacheGoldenTest, 0, testsList.Len())
			for testIdx := 0; testIdx < testsList.Len(); testIdx++ {
				testMsg := testsList.Get(testIdx).Message()
				testFields := testMsg.Descriptor().Fields()
				leaderField := testFields.ByName("leader")
				droField := testFields.ByName("directed_read_options")
				keyField := testFields.ByName("key")
				limitField := testFields.ByName("limit_key")
				rangeModeField := testFields.ByName("range_mode")
				minEntriesField := testFields.ByName("min_cache_entries_for_random_pick")
				resultField := testFields.ByName("result")
				serverField := testFields.ByName("server")

				parsed := rangeCacheGoldenTest{
					leader:   testMsg.Get(leaderField).Bool(),
					key:      append([]byte(nil), testMsg.Get(keyField).Bytes()...),
					limitKey: append([]byte(nil), testMsg.Get(limitField).Bytes()...),
					server:   testMsg.Get(serverField).String(),
				}
				if testMsg.Has(droField) {
					dro := &sppb.DirectedReadOptions{}
					if err := convertDynamicMessage(testMsg.Get(droField).Message(), dro); err != nil {
						return nil, fmt.Errorf("case[%d] step[%d] test[%d] directed_read_options: %w", i, stepIdx, testIdx, err)
					}
					parsed.directedRead = dro
				}
				switch testMsg.Get(rangeModeField).Enum() {
				case 0:
					parsed.mode = rangeModeCoveringSplit
				case 1:
					parsed.mode = rangeModePickRandom
				default:
					return nil, fmt.Errorf("case[%d] step[%d] test[%d]: unknown range_mode %d", i, stepIdx, testIdx, testMsg.Get(rangeModeField).Enum())
				}
				if testMsg.Has(minEntriesField) {
					min := int(testMsg.Get(minEntriesField).Int())
					parsed.minEntries = &min
				}
				if testMsg.Has(resultField) {
					result := &sppb.RoutingHint{}
					if err := convertDynamicMessage(testMsg.Get(resultField).Message(), result); err != nil {
						return nil, fmt.Errorf("case[%d] step[%d] test[%d] result: %w", i, stepIdx, testIdx, err)
					}
					parsed.result = result
				}
				step.tests = append(step.tests, parsed)
			}
			testCase.steps = append(testCase.steps, step)
		}
		cases = append(cases, testCase)
	}
	return cases, nil
}

func buildRangeCacheGoldenDescriptor() (protoreflect.MessageDescriptor, error) {
	fileProto := &descriptorpb.FileDescriptorProto{
		Name:       proto.String("range_cache_test_dynamic.proto"),
		Package:    proto.String("spanner.cloud.location"),
		Syntax:     proto.String("proto3"),
		Dependency: []string{"google/spanner/v1/location.proto", "google/spanner/v1/spanner.proto"},
		MessageType: []*descriptorpb.DescriptorProto{
			{
				Name: proto.String("RangeCacheTestCases"),
				Field: []*descriptorpb.FieldDescriptorProto{
					{
						Name:     proto.String("test_case"),
						Number:   proto.Int32(1),
						Label:    finderGoldenFieldLabel(descriptorpb.FieldDescriptorProto_LABEL_REPEATED),
						Type:     finderGoldenFieldType(descriptorpb.FieldDescriptorProto_TYPE_MESSAGE),
						TypeName: proto.String(".spanner.cloud.location.RangeCacheTestCase"),
					},
				},
			},
			{
				Name: proto.String("RangeCacheTestCase"),
				Field: []*descriptorpb.FieldDescriptorProto{
					{
						Name:   proto.String("name"),
						Number: proto.Int32(1),
						Label:  finderGoldenFieldLabel(descriptorpb.FieldDescriptorProto_LABEL_OPTIONAL),
						Type:   finderGoldenFieldType(descriptorpb.FieldDescriptorProto_TYPE_STRING),
					},
					{
						Name:     proto.String("step"),
						Number:   proto.Int32(2),
						Label:    finderGoldenFieldLabel(descriptorpb.FieldDescriptorProto_LABEL_REPEATED),
						Type:     finderGoldenFieldType(descriptorpb.FieldDescriptorProto_TYPE_MESSAGE),
						TypeName: proto.String(".spanner.cloud.location.RangeCacheTestCase.Step"),
					},
				},
				NestedType: []*descriptorpb.DescriptorProto{
					{
						Name: proto.String("Step"),
						Field: []*descriptorpb.FieldDescriptorProto{
							{
								Name:     proto.String("update"),
								Number:   proto.Int32(1),
								Label:    finderGoldenFieldLabel(descriptorpb.FieldDescriptorProto_LABEL_OPTIONAL),
								Type:     finderGoldenFieldType(descriptorpb.FieldDescriptorProto_TYPE_MESSAGE),
								TypeName: proto.String(".google.spanner.v1.CacheUpdate"),
							},
							{
								Name:     proto.String("test"),
								Number:   proto.Int32(2),
								Label:    finderGoldenFieldLabel(descriptorpb.FieldDescriptorProto_LABEL_REPEATED),
								Type:     finderGoldenFieldType(descriptorpb.FieldDescriptorProto_TYPE_MESSAGE),
								TypeName: proto.String(".spanner.cloud.location.RangeCacheTestCase.Step.Test"),
							},
						},
						NestedType: []*descriptorpb.DescriptorProto{
							{
								Name: proto.String("Test"),
								Field: []*descriptorpb.FieldDescriptorProto{
									{
										Name:   proto.String("leader"),
										Number: proto.Int32(1),
										Label:  finderGoldenFieldLabel(descriptorpb.FieldDescriptorProto_LABEL_OPTIONAL),
										Type:   finderGoldenFieldType(descriptorpb.FieldDescriptorProto_TYPE_BOOL),
									},
									{
										Name:     proto.String("directed_read_options"),
										Number:   proto.Int32(2),
										Label:    finderGoldenFieldLabel(descriptorpb.FieldDescriptorProto_LABEL_OPTIONAL),
										Type:     finderGoldenFieldType(descriptorpb.FieldDescriptorProto_TYPE_MESSAGE),
										TypeName: proto.String(".google.spanner.v1.DirectedReadOptions"),
									},
									{
										Name:   proto.String("key"),
										Number: proto.Int32(3),
										Label:  finderGoldenFieldLabel(descriptorpb.FieldDescriptorProto_LABEL_OPTIONAL),
										Type:   finderGoldenFieldType(descriptorpb.FieldDescriptorProto_TYPE_BYTES),
									},
									{
										Name:   proto.String("limit_key"),
										Number: proto.Int32(4),
										Label:  finderGoldenFieldLabel(descriptorpb.FieldDescriptorProto_LABEL_OPTIONAL),
										Type:   finderGoldenFieldType(descriptorpb.FieldDescriptorProto_TYPE_BYTES),
									},
									{
										Name:     proto.String("range_mode"),
										Number:   proto.Int32(5),
										Label:    finderGoldenFieldLabel(descriptorpb.FieldDescriptorProto_LABEL_OPTIONAL),
										Type:     finderGoldenFieldType(descriptorpb.FieldDescriptorProto_TYPE_ENUM),
										TypeName: proto.String(".spanner.cloud.location.RangeCacheTestCase.Step.Test.RangeMode"),
									},
									{
										Name:   proto.String("min_cache_entries_for_random_pick"),
										Number: proto.Int32(6),
										Label:  finderGoldenFieldLabel(descriptorpb.FieldDescriptorProto_LABEL_OPTIONAL),
										Type:   finderGoldenFieldType(descriptorpb.FieldDescriptorProto_TYPE_INT32),
									},
									{
										Name:     proto.String("result"),
										Number:   proto.Int32(7),
										Label:    finderGoldenFieldLabel(descriptorpb.FieldDescriptorProto_LABEL_OPTIONAL),
										Type:     finderGoldenFieldType(descriptorpb.FieldDescriptorProto_TYPE_MESSAGE),
										TypeName: proto.String(".google.spanner.v1.RoutingHint"),
									},
									{
										Name:   proto.String("server"),
										Number: proto.Int32(8),
										Label:  finderGoldenFieldLabel(descriptorpb.FieldDescriptorProto_LABEL_OPTIONAL),
										Type:   finderGoldenFieldType(descriptorpb.FieldDescriptorProto_TYPE_STRING),
									},
								},
								EnumType: []*descriptorpb.EnumDescriptorProto{
									{
										Name: proto.String("RangeMode"),
										Value: []*descriptorpb.EnumValueDescriptorProto{
											{Name: proto.String("COVERING_SPLIT"), Number: proto.Int32(0)},
											{Name: proto.String("PICK_RANDOM"), Number: proto.Int32(1)},
										},
									},
								},
							},
						},
					},
				},
			},
		},
	}

	fileDesc, err := protodesc.NewFile(fileProto, protoregistry.GlobalFiles)
	if err != nil {
		return nil, fmt.Errorf("create range cache golden descriptor: %w", err)
	}
	rootDesc := fileDesc.Messages().ByName("RangeCacheTestCases")
	if rootDesc == nil {
		return nil, fmt.Errorf("missing RangeCacheTestCases descriptor")
	}
	return rootDesc, nil
}

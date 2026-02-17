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
	locationpb "cloud.google.com/go/spanner/test/proto/locationpb"
	"github.com/google/go-cmp/cmp"
	"google.golang.org/protobuf/encoding/prototext"
	"google.golang.org/protobuf/testing/protocmp"
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
	root := &locationpb.RangeCacheTestCases{}
	if err := prototext.Unmarshal([]byte(content), root); err != nil {
		return nil, fmt.Errorf("unmarshal range cache goldens as proto: %w", err)
	}

	cases := make([]rangeCacheGoldenCase, 0, len(root.GetTestCase()))
	for i, c := range root.GetTestCase() {
		if c == nil {
			return nil, fmt.Errorf("test_case[%d]: nil", i)
		}
		testCase := rangeCacheGoldenCase{name: c.GetName()}
		testCase.steps = make([]rangeCacheGoldenStep, 0, len(c.GetStep()))

		for stepIdx, s := range c.GetStep() {
			if s == nil {
				return nil, fmt.Errorf("case[%d] step[%d]: nil", i, stepIdx)
			}
			step := rangeCacheGoldenStep{update: s.GetUpdate()}
			step.tests = make([]rangeCacheGoldenTest, 0, len(s.GetTest()))
			for testIdx, tc := range s.GetTest() {
				if tc == nil {
					return nil, fmt.Errorf("case[%d] step[%d] test[%d]: nil", i, stepIdx, testIdx)
				}
				parsed := rangeCacheGoldenTest{
					leader:       tc.GetLeader(),
					directedRead: tc.GetDirectedReadOptions(),
					key:          append([]byte(nil), tc.GetKey()...),
					limitKey:     append([]byte(nil), tc.GetLimitKey()...),
					result:       tc.GetResult(),
					server:       tc.GetServer(),
				}
				switch tc.GetRangeMode() {
				case locationpb.RangeCacheTestCase_Step_Test_COVERING_SPLIT:
					parsed.mode = rangeModeCoveringSplit
				case locationpb.RangeCacheTestCase_Step_Test_PICK_RANDOM:
					parsed.mode = rangeModePickRandom
				default:
					return nil, fmt.Errorf("case[%d] step[%d] test[%d]: unknown range_mode %d", i, stepIdx, testIdx, tc.GetRangeMode())
				}
				if tc.GetMinCacheEntriesForRandomPick() != 0 {
					min := int(tc.GetMinCacheEntriesForRandomPick())
					parsed.minEntries = &min
				}
				step.tests = append(step.tests, parsed)
			}
			testCase.steps = append(testCase.steps, step)
		}
		cases = append(cases, testCase)
	}
	return cases, nil
}

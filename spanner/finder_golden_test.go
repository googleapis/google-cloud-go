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
	"strings"
	"sync"
	"testing"

	sppb "cloud.google.com/go/spanner/apiv1/spannerpb"
	locationpb "cloud.google.com/go/spanner/test/proto/locationpb"
	"github.com/google/go-cmp/cmp"
	"google.golang.org/protobuf/encoding/prototext"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/testing/protocmp"
)

type finderGoldenEvent struct {
	name             string
	unhealthyServers []string
	cacheUpdate      *sppb.CacheUpdate
	read             *sppb.ReadRequest
	sql              *sppb.ExecuteSqlRequest
	server           string
	hint             *sppb.RoutingHint
}

type finderGoldenTestCase struct {
	name   string
	events []finderGoldenEvent
}

type finderGoldenEndpoint struct {
	address string
	cache   *finderGoldenEndpointCache
}

func (e *finderGoldenEndpoint) Address() string {
	return e.address
}

func (e *finderGoldenEndpoint) IsHealthy() bool {
	e.cache.mu.Lock()
	defer e.cache.mu.Unlock()
	return !e.cache.unhealthy[e.address]
}

type finderGoldenEndpointCache struct {
	mu        sync.Mutex
	endpoints map[string]*finderGoldenEndpoint
	unhealthy map[string]bool
}

func newFinderGoldenEndpointCache() *finderGoldenEndpointCache {
	return &finderGoldenEndpointCache{
		endpoints: make(map[string]*finderGoldenEndpoint),
		unhealthy: make(map[string]bool),
	}
}

func (c *finderGoldenEndpointCache) Get(address string) channelEndpoint {
	c.mu.Lock()
	defer c.mu.Unlock()
	if endpoint, ok := c.endpoints[address]; ok {
		return endpoint
	}
	endpoint := &finderGoldenEndpoint{address: address, cache: c}
	c.endpoints[address] = endpoint
	return endpoint
}

func (c *finderGoldenEndpointCache) setUnhealthyServers(addresses []string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.unhealthy = make(map[string]bool, len(addresses))
	for _, address := range addresses {
		c.unhealthy[address] = true
	}
}

func TestChannelFinder_Golden(t *testing.T) {
	content, err := os.ReadFile(filepath.Join("testdata", "location", "finder_test.textproto"))
	if err != nil {
		t.Fatalf("failed reading golden file: %v", err)
	}

	testCases, err := parseFinderGoldenTestCases(string(content))
	if err != nil {
		t.Fatalf("failed parsing golden cases: %v", err)
	}
	if len(testCases) == 0 {
		t.Fatal("no golden test cases parsed")
	}

	for _, testCase := range testCases {
		endpointCache := newFinderGoldenEndpointCache()
		finder := newChannelFinder(endpointCache)
		finder.useDeterministicRandom()

		for idx, event := range testCase.events {
			eventName := event.name
			if eventName == "" {
				eventName = fmt.Sprintf("event_%d", idx)
			}
			endpointCache.setUnhealthyServers(event.unhealthyServers)

			switch {
			case event.cacheUpdate != nil:
				finder.update(event.cacheUpdate)
			case event.read != nil:
				req := proto.Clone(event.read).(*sppb.ReadRequest)
				endpoint := finder.findServerReadWithTransaction(req)
				assertFinderGoldenEndpoint(t, testCase.name, eventName, event.server, endpoint)
				assertFinderGoldenHint(t, testCase.name, eventName, event.hint, req.GetRoutingHint())
			case event.sql != nil:
				req := proto.Clone(event.sql).(*sppb.ExecuteSqlRequest)
				endpoint := finder.findServerExecuteSQLWithTransaction(req)
				assertFinderGoldenEndpoint(t, testCase.name, eventName, event.server, endpoint)
				assertFinderGoldenHint(t, testCase.name, eventName, event.hint, req.GetRoutingHint())
			default:
				t.Fatalf("case %q event %q: unsupported event payload", testCase.name, eventName)
			}
		}
	}
}

func assertFinderGoldenEndpoint(t *testing.T, caseName, eventName, expected string, actual channelEndpoint) {
	t.Helper()
	if expected == "" {
		if actual != nil {
			t.Fatalf("case %q event %q: expected no endpoint, got %q", caseName, eventName, actual.Address())
		}
		return
	}
	if actual == nil {
		t.Fatalf("case %q event %q: expected endpoint %q, got nil", caseName, eventName, expected)
	}
	if actual.Address() != expected {
		t.Fatalf("case %q event %q: expected endpoint %q, got %q", caseName, eventName, expected, actual.Address())
	}
}

func assertFinderGoldenHint(t *testing.T, caseName, eventName string, expected, actual *sppb.RoutingHint) {
	t.Helper()
	if expected == nil {
		expected = &sppb.RoutingHint{}
	}
	if actual == nil {
		actual = &sppb.RoutingHint{}
	}
	if diff := cmp.Diff(expected, actual, protocmp.Transform()); diff != "" {
		t.Fatalf("case %q event %q: routing hint mismatch (-want +got):\n%s", caseName, eventName, diff)
	}
}

func parseFinderGoldenTestCases(content string) ([]finderGoldenTestCase, error) {
	content = stripFinderGoldenComments(content)
	root := &locationpb.FinderTestCases{}
	if err := prototext.Unmarshal([]byte(content), root); err != nil {
		return nil, fmt.Errorf("unmarshal finder goldens as proto: %w", err)
	}

	testCases := make([]finderGoldenTestCase, 0, len(root.GetTestCase()))
	for i, c := range root.GetTestCase() {
		if c == nil {
			return nil, fmt.Errorf("test_case[%d]: nil", i)
		}
		testCase := finderGoldenTestCase{name: c.GetName()}
		testCase.events = make([]finderGoldenEvent, 0, len(c.GetEvent()))
		for j, e := range c.GetEvent() {
			if e == nil {
				return nil, fmt.Errorf("test_case[%d] (%q) event[%d]: nil", i, testCase.name, j)
			}
			event := finderGoldenEvent{
				name:             e.GetName(),
				unhealthyServers: append([]string(nil), e.GetUnhealthyServers()...),
				cacheUpdate:      e.GetCacheUpdate(),
				read:             e.GetRead(),
				sql:              e.GetSql(),
				server:           e.GetServer(),
				hint:             e.GetHint(),
			}
			testCase.events = append(testCase.events, event)
		}
		testCases = append(testCases, testCase)
	}
	return testCases, nil
}

func stripFinderGoldenComments(content string) string {
	lines := strings.Split(content, "\n")
	filtered := make([]string, 0, len(lines))
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "#") {
			continue
		}
		filtered = append(filtered, line)
	}
	return strings.Join(filtered, "\n")
}

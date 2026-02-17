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
	descriptor, err := buildFinderGoldenDescriptor()
	if err != nil {
		return nil, err
	}
	casesMessage := dynamicpb.NewMessage(descriptor)
	if err := prototext.Unmarshal([]byte(content), casesMessage); err != nil {
		return nil, fmt.Errorf("unmarshal finder goldens as proto: %w", err)
	}

	casesField := descriptor.Fields().ByName("test_case")
	casesList := casesMessage.Get(casesField).List()

	testCases := make([]finderGoldenTestCase, 0, casesList.Len())
	for i := 0; i < casesList.Len(); i++ {
		caseMsg := casesList.Get(i).Message()
		nameField := caseMsg.Descriptor().Fields().ByName("name")
		eventField := caseMsg.Descriptor().Fields().ByName("event")

		testCase := finderGoldenTestCase{name: caseMsg.Get(nameField).String()}
		events := caseMsg.Get(eventField).List()
		testCase.events = make([]finderGoldenEvent, 0, events.Len())

		for j := 0; j < events.Len(); j++ {
			eventMsg := events.Get(j).Message()
			event, err := parseFinderGoldenEventMessage(eventMsg)
			if err != nil {
				return nil, fmt.Errorf("test_case[%d] (%q) event[%d]: %w", i, testCase.name, j, err)
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

func parseFinderGoldenEventMessage(eventMsg protoreflect.Message) (finderGoldenEvent, error) {
	fields := eventMsg.Descriptor().Fields()
	nameField := fields.ByName("name")
	unhealthyField := fields.ByName("unhealthy_servers")
	cacheUpdateField := fields.ByName("cache_update")
	readField := fields.ByName("read")
	sqlField := fields.ByName("sql")
	serverField := fields.ByName("server")
	hintField := fields.ByName("hint")

	event := finderGoldenEvent{
		name:   eventMsg.Get(nameField).String(),
		server: eventMsg.Get(serverField).String(),
	}

	unhealthy := eventMsg.Get(unhealthyField).List()
	event.unhealthyServers = make([]string, 0, unhealthy.Len())
	for i := 0; i < unhealthy.Len(); i++ {
		event.unhealthyServers = append(event.unhealthyServers, unhealthy.Get(i).String())
	}

	if eventMsg.Has(cacheUpdateField) {
		cacheUpdate := &sppb.CacheUpdate{}
		if err := convertDynamicMessage(eventMsg.Get(cacheUpdateField).Message(), cacheUpdate); err != nil {
			return event, fmt.Errorf("cache_update: %w", err)
		}
		event.cacheUpdate = cacheUpdate
	}
	if eventMsg.Has(readField) {
		read := &sppb.ReadRequest{}
		if err := convertDynamicMessage(eventMsg.Get(readField).Message(), read); err != nil {
			return event, fmt.Errorf("read: %w", err)
		}
		event.read = read
	}
	if eventMsg.Has(sqlField) {
		sql := &sppb.ExecuteSqlRequest{}
		if err := convertDynamicMessage(eventMsg.Get(sqlField).Message(), sql); err != nil {
			return event, fmt.Errorf("sql: %w", err)
		}
		event.sql = sql
	}
	if eventMsg.Has(hintField) {
		hint := &sppb.RoutingHint{}
		if err := convertDynamicMessage(eventMsg.Get(hintField).Message(), hint); err != nil {
			return event, fmt.Errorf("hint: %w", err)
		}
		event.hint = hint
	}

	return event, nil
}

func convertDynamicMessage(src protoreflect.Message, dst proto.Message) error {
	serialized, err := proto.Marshal(src.Interface())
	if err != nil {
		return err
	}
	return proto.Unmarshal(serialized, dst)
}

func buildFinderGoldenDescriptor() (protoreflect.MessageDescriptor, error) {
	fileProto := &descriptorpb.FileDescriptorProto{
		Name:       proto.String("finder_test_dynamic.proto"),
		Package:    proto.String("spanner.cloud.location"),
		Syntax:     proto.String("proto3"),
		Dependency: []string{"google/spanner/v1/location.proto", "google/spanner/v1/spanner.proto"},
		MessageType: []*descriptorpb.DescriptorProto{
			{
				Name: proto.String("FinderTestCase"),
				Field: []*descriptorpb.FieldDescriptorProto{
					{
						Name:   proto.String("name"),
						Number: proto.Int32(1),
						Label:  finderGoldenFieldLabel(descriptorpb.FieldDescriptorProto_LABEL_OPTIONAL),
						Type:   finderGoldenFieldType(descriptorpb.FieldDescriptorProto_TYPE_STRING),
					},
					{
						Name:     proto.String("event"),
						Number:   proto.Int32(2),
						Label:    finderGoldenFieldLabel(descriptorpb.FieldDescriptorProto_LABEL_REPEATED),
						Type:     finderGoldenFieldType(descriptorpb.FieldDescriptorProto_TYPE_MESSAGE),
						TypeName: proto.String(".spanner.cloud.location.FinderTestCase.Event"),
					},
				},
				NestedType: []*descriptorpb.DescriptorProto{
					{
						Name: proto.String("Event"),
						Field: []*descriptorpb.FieldDescriptorProto{
							{
								Name:   proto.String("name"),
								Number: proto.Int32(1),
								Label:  finderGoldenFieldLabel(descriptorpb.FieldDescriptorProto_LABEL_OPTIONAL),
								Type:   finderGoldenFieldType(descriptorpb.FieldDescriptorProto_TYPE_STRING),
							},
							{
								Name:     proto.String("cache_update"),
								Number:   proto.Int32(2),
								Label:    finderGoldenFieldLabel(descriptorpb.FieldDescriptorProto_LABEL_OPTIONAL),
								Type:     finderGoldenFieldType(descriptorpb.FieldDescriptorProto_TYPE_MESSAGE),
								TypeName: proto.String(".google.spanner.v1.CacheUpdate"),
							},
							{
								Name:   proto.String("unhealthy_servers"),
								Number: proto.Int32(3),
								Label:  finderGoldenFieldLabel(descriptorpb.FieldDescriptorProto_LABEL_REPEATED),
								Type:   finderGoldenFieldType(descriptorpb.FieldDescriptorProto_TYPE_STRING),
							},
							{
								Name:       proto.String("read"),
								Number:     proto.Int32(4),
								Label:      finderGoldenFieldLabel(descriptorpb.FieldDescriptorProto_LABEL_OPTIONAL),
								Type:       finderGoldenFieldType(descriptorpb.FieldDescriptorProto_TYPE_MESSAGE),
								TypeName:   proto.String(".google.spanner.v1.ReadRequest"),
								OneofIndex: proto.Int32(0),
							},
							{
								Name:       proto.String("sql"),
								Number:     proto.Int32(5),
								Label:      finderGoldenFieldLabel(descriptorpb.FieldDescriptorProto_LABEL_OPTIONAL),
								Type:       finderGoldenFieldType(descriptorpb.FieldDescriptorProto_TYPE_MESSAGE),
								TypeName:   proto.String(".google.spanner.v1.ExecuteSqlRequest"),
								OneofIndex: proto.Int32(0),
							},
							{
								Name:   proto.String("server"),
								Number: proto.Int32(6),
								Label:  finderGoldenFieldLabel(descriptorpb.FieldDescriptorProto_LABEL_OPTIONAL),
								Type:   finderGoldenFieldType(descriptorpb.FieldDescriptorProto_TYPE_STRING),
							},
							{
								Name:     proto.String("hint"),
								Number:   proto.Int32(7),
								Label:    finderGoldenFieldLabel(descriptorpb.FieldDescriptorProto_LABEL_OPTIONAL),
								Type:     finderGoldenFieldType(descriptorpb.FieldDescriptorProto_TYPE_MESSAGE),
								TypeName: proto.String(".google.spanner.v1.RoutingHint"),
							},
						},
						OneofDecl: []*descriptorpb.OneofDescriptorProto{
							{Name: proto.String("request")},
						},
					},
				},
			},
			{
				Name: proto.String("FinderTestCases"),
				Field: []*descriptorpb.FieldDescriptorProto{
					{
						Name:     proto.String("test_case"),
						Number:   proto.Int32(2),
						Label:    finderGoldenFieldLabel(descriptorpb.FieldDescriptorProto_LABEL_REPEATED),
						Type:     finderGoldenFieldType(descriptorpb.FieldDescriptorProto_TYPE_MESSAGE),
						TypeName: proto.String(".spanner.cloud.location.FinderTestCase"),
					},
				},
			},
		},
	}

	fileDesc, err := protodesc.NewFile(fileProto, protoregistry.GlobalFiles)
	if err != nil {
		return nil, fmt.Errorf("create finder golden descriptor: %w", err)
	}
	casesDesc := fileDesc.Messages().ByName("FinderTestCases")
	if casesDesc == nil {
		return nil, fmt.Errorf("missing FinderTestCases descriptor")
	}
	return casesDesc, nil
}

func finderGoldenFieldLabel(label descriptorpb.FieldDescriptorProto_Label) *descriptorpb.FieldDescriptorProto_Label {
	return &label
}

func finderGoldenFieldType(fieldType descriptorpb.FieldDescriptorProto_Type) *descriptorpb.FieldDescriptorProto_Type {
	return &fieldType
}

// Copyright 2021 Google LLC
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

package pubsub

import (
	"context"
	"fmt"

	"google.golang.org/api/option"

	vkit "cloud.google.com/go/pubsub/apiv1"
	pb "google.golang.org/genproto/googleapis/pubsub/v1"
)

type SchemaClient struct {
	sc        *vkit.SchemaClient
	projectID string
}

func (s *SchemaClient) Close() error {
	return s.sc.Close()
}

func NewSchemaClient(ctx context.Context, projectID string, opts ...option.ClientOption) (*SchemaClient, error) {
	sc, err := vkit.NewSchemaClient(ctx, opts...)
	if err != nil {
		return nil, err
	}
	return &SchemaClient{sc: sc, projectID: projectID}, nil
}

// SchemaConfig is a reference to a PubSub schema.
type SchemaConfig struct {
	Name string

	// The type of the schema definition.
	Type SchemaType

	// The definition of the schema. This should contain a string representing
	// the full definition of the schema that is a valid schema definition of
	// the type specified in `type`.
	Definition string
}

type SchemaType pb.Schema_Type

const (
	// Default value. This value is unused.
	// SchemaTypeUnspecified SchemaType = 0
	// A Protocol Buffer schema definition.
	SchemaProtocolBuffer SchemaType = 1
	// An Avro schema definition.
	SchemaAvro SchemaType = 2
)

type SchemaView pb.SchemaView

const (
	// The default / unset value.
	// The API will default to the BASIC view.
	SchemaViewUnspecified SchemaView = 0
	// Include the name and type of the schema, but not the definition.
	SchemaViewBasic SchemaView = 1
	// Include all Schema object fields.
	SchemaViewFull SchemaView = 2
)

type SchemaSettings struct {
	Schema   string
	Encoding SchemaEncoding
}

type SchemaEncoding pb.Encoding

func (s *SchemaConfig) toProto() *pb.Schema {
	pbs := &pb.Schema{
		Type:       pb.Schema_Type(s.Type),
		Definition: s.Definition,
	}
	return pbs
}

func protoToSchemaConfig(pbs *pb.Schema) *SchemaConfig {
	return &SchemaConfig{
		Name:       pbs.Name,
		Type:       SchemaType(pbs.Type),
		Definition: pbs.Definition,
	}
}

func (c *SchemaClient) CreateSchema(ctx context.Context, schemaID string, s *SchemaConfig) (*SchemaConfig, error) {
	pbs := s.toProto()
	req := &pb.CreateSchemaRequest{
		Parent:   fmt.Sprintf("projects/%s", c.projectID),
		Schema:   pbs,
		SchemaId: schemaID,
	}
	pbs, err := c.sc.CreateSchema(ctx, req)
	if err != nil {
		return nil, err
	}
	return protoToSchemaConfig(pbs), nil
}

// Schema retrieves the configuration of a topic. A valid schema path has the
// format: "projects/PROJECT_ID/schemas/SCHEMA_ID".
func (c *SchemaClient) Schema(ctx context.Context, schema string, view SchemaView) (*SchemaConfig, error) {
	req := &pb.GetSchemaRequest{
		Name: schema,
		View: pb.SchemaView(view),
	}
	s, err := c.sc.GetSchema(ctx, req)
	if err != nil {
		return nil, err
	}
	return protoToSchemaConfig(s), nil
}

// Schemas returns an iterator which returns all of the schemas for the client's project.
func (c *SchemaClient) Schemas(ctx context.Context, view SchemaView) *SchemaIterator {
	return &SchemaIterator{
		it: c.sc.ListSchemas(ctx, &pb.ListSchemasRequest{
			Parent: c.projectID,
			View:   pb.SchemaView(view),
		}),
	}
}

type SchemaIterator struct {
	it  *vkit.SchemaIterator
	err error
}

// Next returns the next schema. If there are no more schemas, iterator.Done will be returned.
func (s *SchemaIterator) Next() (*SchemaConfig, error) {
	if s.err != nil {
		return nil, s.err
	}
	pbs, err := s.it.Next()
	if err != nil {
		return nil, err
	}
	return protoToSchemaConfig(pbs), nil
}

func (s *SchemaClient) DeleteSchema(ctx context.Context, schemaPath string) error {
	return s.sc.DeleteSchema(ctx, &pb.DeleteSchemaRequest{
		Name: schemaPath,
	})
}

// ValidateSchemaResult is the response for the ValidateSchema method.
// Reserved for future use.
type ValidateSchemaResult struct{}

func (s *SchemaClient) ValidateSchema(ctx context.Context, schema *SchemaConfig) (*ValidateSchemaResult, error) {
	req := &pb.ValidateSchemaRequest{
		Parent: fmt.Sprintf("projects/%s", s.projectID),
		Schema: schema.toProto(),
	}
	_, err := s.sc.ValidateSchema(ctx, req)
	if err != nil {
		return nil, err
	}
	return &ValidateSchemaResult{}, nil
}

// ValidateMessageResult is the response for the ValidateMessage method.
// Reserved for future use.
type ValidateMessageResult struct{}

func (s *SchemaClient) ValidateMessage(ctx context.Context, msg []byte, encoding SchemaEncoding) (*ValidateMessageResult, error) {
	req := &pb.ValidateMessageRequest{
		Parent:   fmt.Sprintf("projects/%s", s.projectID),
		Message:  msg,
		Encoding: pb.Encoding(encoding),
	}
	_, err := s.sc.ValidateMessage(ctx, req)
	if err != nil {
		return nil, err
	}
	return &ValidateMessageResult{}, nil
}

// Copyright 2021 Google LLC
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     https://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and

package pubsub

import (
	"context"
	"errors"
	"testing"

	"cloud.google.com/go/internal/testutil"
	"github.com/google/go-cmp/cmp"
	"google.golang.org/api/iterator"
	"google.golang.org/api/option"
	"google.golang.org/grpc"

	"cloud.google.com/go/pubsub/pstest"
)

func newSchemaFake(t *testing.T) (*SchemaClient, *pstest.Server) {
	ctx := context.Background()
	srv := pstest.NewServer()

	schema, err := NewSchemaClient(ctx, "my-proj", option.WithEndpoint(srv.Addr), option.WithoutAuthentication(), option.WithGRPCDialOption(grpc.WithInsecure()))
	if err != nil {
		t.Fatal(err)
	}
	return schema, srv
}

func TestSchemaBasicCreateGetDelete(t *testing.T) {
	ctx := context.Background()

	// Inputs
	const schemaPath = "projects/my-proj/schemas/my-schema"
	schemaConfig := &SchemaConfig{
		Name:       schemaPath,
		Type:       SchemaAvro,
		Definition: "some-definition",
	}

	admin, _ := newSchemaFake(t)
	defer admin.Close()

	if gotConfig, err := admin.CreateSchema(ctx, "my-schema", schemaConfig); err != nil {
		t.Errorf("CreateSchema() got err: %v", err)
	} else if diff := cmp.Diff(gotConfig, schemaConfig); diff != "" {
		t.Errorf("CreateSchema() -want, +got: %v", diff)
	}

	if gotConfig, err := admin.Schema(ctx, schemaPath, SchemaViewFull); err != nil {
		t.Errorf("Schema() got err: %v", err)
	} else if !testutil.Equal(gotConfig, schemaConfig) {
		t.Errorf("Schema() got: %v\nwant: %v", gotConfig, schemaConfig)
	}

	if err := admin.DeleteSchema(ctx, schemaPath); err != nil {
		t.Errorf("DeleteSchema() got err: %v", err)
	}
}

func TestSchemaListSchemas(t *testing.T) {
	ctx := context.Background()
	admin, _ := newSchemaFake(t)
	defer admin.Close()

	// Inputs
	schemaConfig1 := &SchemaConfig{
		Name:       "projects/my-proj/schemas/schema-1",
		Type:       SchemaAvro,
		Definition: "some schema definition",
	}
	schemaConfig2 := &SchemaConfig{
		Name:       "projects/my-proj/schemas/schema-2",
		Type:       SchemaProtocolBuffer,
		Definition: "some other schema definition",
	}

	mustCreateSchema(t, admin, "schema-1", schemaConfig1)
	mustCreateSchema(t, admin, "schema-2", schemaConfig2)

	var gotSchemaConfigs []*SchemaConfig
	it := admin.Schemas(ctx, SchemaViewFull)
	for {
		schema, err := it.Next()
		if err == iterator.Done {
			break
		}
		if err != nil {
			t.Errorf("SchemaIterator.Next() got err: %v", err)
		} else {
			gotSchemaConfigs = append(gotSchemaConfigs, schema)
		}
	}

	wantSchemaConfigs := []*SchemaConfig{schemaConfig1, schemaConfig2}
	if diff := testutil.Diff(gotSchemaConfigs, wantSchemaConfigs); diff != "" {
		t.Errorf("Schemas() want: -, got: %s", diff)
	}
}

func TestSchemaValidateSchema(t *testing.T) {
	ctx := context.Background()
	admin, _ := newSchemaFake(t)
	defer admin.Close()

	// Inputs
	testCases := []struct {
		schema  *SchemaConfig
		wantErr error
	}{
		{
			schema: &SchemaConfig{
				Name:       "schema-1",
				Type:       SchemaAvro,
				Definition: "bad definition",
			},
			wantErr: errors.New("blah"),
		},
		{
			schema: &SchemaConfig{
				Name:       "schema-2",
				Type:       SchemaAvro,
				Definition: "good definition",
			},
			wantErr: nil,
		},
	}
	for _, tc := range testCases {
		admin.ValidateSchema(ctx, tc.schema)
	}

}

func mustCreateSchema(t *testing.T, c *SchemaClient, id string, sc *SchemaConfig) *SchemaConfig {
	schema, err := c.CreateSchema(context.Background(), id, sc)
	if err != nil {
		t.Fatal(err)
	}
	return schema
}

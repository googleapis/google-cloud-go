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
	"fmt"
	"testing"

	"cloud.google.com/go/internal/testutil"
	"github.com/google/go-cmp/cmp"
	"google.golang.org/api/iterator"
	"google.golang.org/api/option"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

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
	schemaID := "my-schema"
	schemaPath := fmt.Sprintf("projects/my-proj/schemas/%s", schemaID)
	schemaConfig := SchemaConfig{
		Name:       schemaPath,
		Type:       SchemaAvro,
		Definition: "some-definition",
	}

	admin, _ := newSchemaFake(t)
	defer admin.Close()

	if gotConfig, err := admin.CreateSchema(ctx, schemaID, schemaConfig); err != nil {
		t.Errorf("CreateSchema() got err: %v", err)
	} else if diff := cmp.Diff(*gotConfig, schemaConfig); diff != "" {
		t.Errorf("CreateSchema() -want, +got: %v", diff)
	}

	gotConfig, err := admin.Schema(ctx, schemaID, SchemaViewFull)
	if err != nil {
		t.Errorf("Schema() got err: %v", err)
	}
	if diff := testutil.Diff(*gotConfig, schemaConfig); diff != "" {
		t.Errorf("Schema() -got, +want:\n%v", diff)
	}

	if err := admin.DeleteSchema(ctx, schemaID); err != nil {
		t.Errorf("DeleteSchema() got err: %v", err)
	}

	if err := admin.DeleteSchema(ctx, "fake-schema"); err == nil {
		t.Error("DeleteSchema() got nil, expected NotFound err")
	}
}

func TestSchemaListSchemas(t *testing.T) {
	ctx := context.Background()
	admin, _ := newSchemaFake(t)
	defer admin.Close()

	// Inputs
	schemaConfig1 := SchemaConfig{
		Name:       "projects/my-proj/schemas/schema-1",
		Type:       SchemaAvro,
		Definition: "some schema definition",
	}
	schemaConfig2 := SchemaConfig{
		Name:       "projects/my-proj/schemas/schema-2",
		Type:       SchemaProtocolBuffer,
		Definition: "some other schema definition",
	}

	mustCreateSchema(t, admin, "schema-1", schemaConfig1)
	mustCreateSchema(t, admin, "schema-2", schemaConfig2)

	var gotSchemaConfigs []SchemaConfig
	it := admin.Schemas(ctx, SchemaViewFull)
	for {
		schema, err := it.Next()
		if err == iterator.Done {
			break
		}
		if err != nil {
			t.Errorf("SchemaIterator.Next() got err: %v", err)
		} else {
			gotSchemaConfigs = append(gotSchemaConfigs, *schema)
		}
	}

	got := len(gotSchemaConfigs)
	want := 2
	if got != want {
		t.Errorf("Schemas() want: %d schemas, got: %d", want, got)
	}
}

func TestSchemaValidateSchema(t *testing.T) {
	ctx := context.Background()
	admin, _ := newSchemaFake(t)
	defer admin.Close()

	for _, tc := range []struct {
		desc    string
		schema  SchemaConfig
		wantErr error
	}{
		{
			desc: "valid avro schema",
			schema: SchemaConfig{
				Name:       "schema-1",
				Type:       SchemaAvro,
				Definition: "{name:some-avro-schema}",
			},
			wantErr: nil,
		},
		{
			desc: "valid proto schema",
			schema: SchemaConfig{
				Name:       "schema-1",
				Type:       SchemaProtocolBuffer,
				Definition: "some proto buf schema definition",
			},
			wantErr: nil,
		},
		{
			desc: "empty invalid schema",
			schema: SchemaConfig{
				Name:       "schema-3",
				Type:       SchemaProtocolBuffer,
				Definition: "",
			},
			wantErr: status.Error(codes.InvalidArgument, "schema definition cannot be empty"),
		},
	} {
		t.Run(tc.desc, func(t *testing.T) {
			_, gotErr := admin.ValidateSchema(ctx, tc.schema)
			if status.Code(gotErr) != status.Code(tc.wantErr) {
				t.Fatalf("got err: %v\nwant err: %v", gotErr, tc.wantErr)
			}
		})
	}
}

func mustCreateSchema(t *testing.T, c *SchemaClient, id string, sc SchemaConfig) *SchemaConfig {
	schema, err := c.CreateSchema(context.Background(), id, sc)
	if err != nil {
		t.Fatal(err)
	}
	return schema
}

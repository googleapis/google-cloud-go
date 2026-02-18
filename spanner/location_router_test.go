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
	structpb "google.golang.org/protobuf/types/known/structpb"
)

func TestIsExperimentalLocationAPIEnabled(t *testing.T) {
	t.Setenv(experimentalLocationAPIEnvVar, "true")
	if !isExperimentalLocationAPIEnabled() {
		t.Fatal("expected feature flag to be enabled for true")
	}

	t.Setenv(experimentalLocationAPIEnvVar, "false")
	if isExperimentalLocationAPIEnabled() {
		t.Fatal("expected feature flag to be disabled for false")
	}

	t.Setenv(experimentalLocationAPIEnvVar, "not-a-bool")
	if isExperimentalLocationAPIEnabled() {
		t.Fatal("expected invalid boolean env var to be treated as disabled")
	}
}

func TestLocationRouter_PrepareReadRequest_FromObservedResultSetUpdate(t *testing.T) {
	router := newLocationRouter()
	if router == nil || router.finder == nil {
		t.Fatal("expected newLocationRouter to initialize finder")
	}

	router.observeResultSet(&sppb.ResultSet{
		CacheUpdate: &sppb.CacheUpdate{
			DatabaseId: 1,
			KeyRecipes: &sppb.RecipeList{
				SchemaGeneration: []byte("1"),
				Recipe: []*sppb.KeyRecipe{
					{
						Target: &sppb.KeyRecipe_TableName{TableName: "T"},
						Part: []*sppb.KeyRecipe_Part{
							{Tag: 1},
							{
								Order:     sppb.KeyRecipe_Part_ASCENDING,
								NullOrder: sppb.KeyRecipe_Part_NULLS_FIRST,
								Type:      &sppb.Type{Code: sppb.TypeCode_STRING},
								ValueType: &sppb.KeyRecipe_Part_Identifier{Identifier: "k"},
							},
						},
					},
				},
			},
		},
	})

	req := &sppb.ReadRequest{
		Table:   "T",
		Columns: []string{"c1"},
		KeySet: &sppb.KeySet{
			Keys: []*structpb.ListValue{
				{Values: []*structpb.Value{structpb.NewStringValue("foo")}},
			},
		},
	}
	router.prepareReadRequest(req)

	hint := req.GetRoutingHint()
	if hint == nil {
		t.Fatal("expected routing hint")
	}
	if hint.GetOperationUid() == 0 {
		t.Fatal("expected operation uid to be set")
	}
	if !bytes.Equal(hint.GetSchemaGeneration(), []byte("1")) {
		t.Fatalf("unexpected schema generation: got %q", hint.GetSchemaGeneration())
	}
	if len(hint.GetKey()) == 0 {
		t.Fatal("expected encoded key bytes")
	}
	if hint.GetDatabaseId() != 1 {
		t.Fatalf("expected database id 1, got %d", hint.GetDatabaseId())
	}
}

func TestLocationRouter_PrepareExecuteSQLRequest_FromObservedPartialResultSetUpdate(t *testing.T) {
	router := newLocationRouter()
	router.observePartialResultSet(&sppb.PartialResultSet{
		CacheUpdate: &sppb.CacheUpdate{
			DatabaseId: 7,
		},
	})

	req := &sppb.ExecuteSqlRequest{
		Sql: "SELECT * FROM T WHERE p1=@p1",
		Params: &structpb.Struct{
			Fields: map[string]*structpb.Value{
				"p1": structpb.NewStringValue("foo"),
			},
		},
	}
	router.prepareExecuteSQLRequest(req)

	hint := req.GetRoutingHint()
	if hint == nil {
		t.Fatal("expected routing hint")
	}
	if hint.GetOperationUid() == 0 {
		t.Fatal("expected operation uid to be set")
	}
	if hint.GetDatabaseId() != 7 {
		t.Fatalf("expected database id 7, got %d", hint.GetDatabaseId())
	}
}

func TestLocationRouter_NilSafety(t *testing.T) {
	var router *locationRouter
	router.prepareReadRequest(nil)
	router.prepareReadRequest(&sppb.ReadRequest{})
	router.prepareExecuteSQLRequest(nil)
	router.prepareExecuteSQLRequest(&sppb.ExecuteSqlRequest{})
	router.prepareBeginTransactionRequest(nil)
	router.prepareBeginTransactionRequest(&sppb.BeginTransactionRequest{})
	router.observePartialResultSet(nil)
	router.observePartialResultSet(&sppb.PartialResultSet{})
	router.observeResultSet(nil)
	router.observeResultSet(&sppb.ResultSet{})
}

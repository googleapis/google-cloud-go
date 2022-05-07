// Copyright 2022 Google LLC
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

package bigquery

import (
	"context"
	"fmt"
	"testing"

	"cloud.google.com/go/internal/testutil"
	"google.golang.org/api/iterator"
)

func TestIntegration_RoutineScalarUDF(t *testing.T) {
	if client == nil {
		t.Skip("Integration tests skipped")
	}
	ctx := context.Background()

	// Create a scalar UDF routine via API.
	routineID := routineIDs.New()
	routine := dataset.Routine(routineID)
	err := routine.Create(ctx, &RoutineMetadata{
		Type:     "SCALAR_FUNCTION",
		Language: "SQL",
		Body:     "x * 3",
		Arguments: []*RoutineArgument{
			{
				Name: "x",
				DataType: &StandardSQLDataType{
					TypeKind: "INT64",
				},
			},
		},
	})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
}

func TestIntegration_RoutineJSUDF(t *testing.T) {
	if client == nil {
		t.Skip("Integration tests skipped")
	}
	ctx := context.Background()

	// Create a scalar UDF routine via API.
	routineID := routineIDs.New()
	routine := dataset.Routine(routineID)
	meta := &RoutineMetadata{
		Language: "JAVASCRIPT", Type: "SCALAR_FUNCTION",
		Description:      "capitalizes using javascript",
		DeterminismLevel: Deterministic,
		Arguments: []*RoutineArgument{
			{Name: "instr", Kind: "FIXED_TYPE", DataType: &StandardSQLDataType{TypeKind: "STRING"}},
		},
		ReturnType: &StandardSQLDataType{TypeKind: "STRING"},
		Body:       "return instr.toUpperCase();",
	}
	if err := routine.Create(ctx, meta); err != nil {
		t.Fatalf("Create: %v", err)
	}

	newMeta := &RoutineMetadataToUpdate{
		Language:    meta.Language,
		Body:        meta.Body,
		Arguments:   meta.Arguments,
		Description: meta.Description,
		ReturnType:  meta.ReturnType,
		Type:        meta.Type,

		DeterminismLevel: NotDeterministic,
	}
	if _, err := routine.Update(ctx, newMeta, ""); err != nil {
		t.Fatalf("Update: %v", err)
	}
}

func TestIntegration_RoutineComplexTypes(t *testing.T) {
	if client == nil {
		t.Skip("Integration tests skipped")
	}
	ctx := context.Background()

	routineID := routineIDs.New()
	routine := dataset.Routine(routineID)
	routineSQLID, _ := routine.Identifier(StandardSQLID)
	sql := fmt.Sprintf(`
		CREATE FUNCTION %s(
			arr ARRAY<STRUCT<name STRING, val INT64>>
		  ) AS (
			  (SELECT SUM(IF(elem.name = "foo",elem.val,null)) FROM UNNEST(arr) AS elem)
		  )`,
		routineSQLID)
	if _, _, err := runQuerySQL(ctx, sql); err != nil {
		t.Fatal(err)
	}
	defer routine.Delete(ctx)

	meta, err := routine.Metadata(ctx)
	if err != nil {
		t.Fatalf("Metadata: %v", err)
	}
	if meta.Type != "SCALAR_FUNCTION" {
		t.Fatalf("routine type mismatch, got %s want SCALAR_FUNCTION", meta.Type)
	}
	if meta.Language != "SQL" {
		t.Fatalf("language type mismatch, got  %s want SQL", meta.Language)
	}
	want := []*RoutineArgument{
		{
			Name: "arr",
			DataType: &StandardSQLDataType{
				TypeKind: "ARRAY",
				ArrayElementType: &StandardSQLDataType{
					TypeKind: "STRUCT",
					StructType: &StandardSQLStructType{
						Fields: []*StandardSQLField{
							{
								Name: "name",
								Type: &StandardSQLDataType{
									TypeKind: "STRING",
								},
							},
							{
								Name: "val",
								Type: &StandardSQLDataType{
									TypeKind: "INT64",
								},
							},
						},
					},
				},
			},
		},
	}
	if diff := testutil.Diff(meta.Arguments, want); diff != "" {
		t.Fatalf("%+v: -got, +want:\n%s", meta.Arguments, diff)
	}
}

func TestIntegration_RoutineLifecycle(t *testing.T) {
	if client == nil {
		t.Skip("Integration tests skipped")
	}
	ctx := context.Background()

	// Create a scalar UDF routine via a CREATE FUNCTION query
	routineID := routineIDs.New()
	routine := dataset.Routine(routineID)
	routineSQLID, _ := routine.Identifier(StandardSQLID)

	sql := fmt.Sprintf(`
		CREATE FUNCTION %s(x INT64) AS (x * 3);`,
		routineSQLID)
	if _, _, err := runQuerySQL(ctx, sql); err != nil {
		t.Fatal(err)
	}
	defer routine.Delete(ctx)

	// Get the routine metadata.
	curMeta, err := routine.Metadata(ctx)
	if err != nil {
		t.Fatalf("couldn't get metadata: %v", err)
	}

	want := "SCALAR_FUNCTION"
	if curMeta.Type != want {
		t.Errorf("Routine type mismatch.  got %s want %s", curMeta.Type, want)
	}

	want = "SQL"
	if curMeta.Language != want {
		t.Errorf("Language mismatch. got %s want %s", curMeta.Language, want)
	}

	// Perform an update to change the routine body and description.
	want = "x * 4"
	wantDescription := "an updated description"
	// during beta, update doesn't allow partial updates.  Provide all fields.
	newMeta, err := routine.Update(ctx, &RoutineMetadataToUpdate{
		Body:        want,
		Arguments:   curMeta.Arguments,
		Description: wantDescription,
		ReturnType:  curMeta.ReturnType,
		Type:        curMeta.Type,
	}, curMeta.ETag)
	if err != nil {
		t.Fatalf("Update: %v", err)
	}
	if newMeta.Body != want {
		t.Fatalf("Update body failed. want %s got %s", want, newMeta.Body)
	}
	if newMeta.Description != wantDescription {
		t.Fatalf("Update description failed. want %s got %s", wantDescription, newMeta.Description)
	}

	// Ensure presence when enumerating the model list.
	it := dataset.Routines(ctx)
	seen := false
	for {
		r, err := it.Next()
		if err == iterator.Done {
			break
		}
		if err != nil {
			t.Fatal(err)
		}
		if r.RoutineID == routineID {
			seen = true
		}
	}
	if !seen {
		t.Fatal("routine not listed in dataset")
	}

	// Delete the model.
	if err := routine.Delete(ctx); err != nil {
		t.Fatalf("failed to delete routine: %v", err)
	}
}

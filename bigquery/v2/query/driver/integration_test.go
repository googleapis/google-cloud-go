// Copyright 2025 Google LLC
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
// limitations under the License.

package driver

import (
	"context"
	"database/sql"
	"fmt"
	"log"
	"os"
	"strings"
	"testing"
	"time"

	"cloud.google.com/go/bigquery/v2/apiv2/bigquerypb"
	"cloud.google.com/go/bigquery/v2/apiv2_client"
	"cloud.google.com/go/bigquery/v2/query/adapt"
	"cloud.google.com/go/internal/testutil"
)

var (
	testProjectID string
	testDatasetID string
	db            *sql.DB
)

func TestMain(m *testing.M) {
	cleanup := setup()
	code := m.Run()
	if cleanup != nil {
		cleanup()
	}
	os.Exit(code)
}

func setup() func() {
	projID := testutil.ProjID()
	if projID == "" {
		log.Printf("project ID undetected, skipping integration tests")
		return nil
	}
	testProjectID = projID

	ctx := context.Background()
	client, err := apiv2_client.NewClient(ctx)
	if err != nil {
		log.Printf("apiv2_client.NewClient: %v", err)
		return nil
	}

	testDatasetID = fmt.Sprintf("testdataset_%d", time.Now().UnixNano())

	ds, err := client.InsertDataset(ctx, &bigquerypb.InsertDatasetRequest{
		ProjectId: testProjectID,
		Dataset: &bigquerypb.Dataset{
			DatasetReference: &bigquerypb.DatasetReference{
				DatasetId: testDatasetID,
				ProjectId: testProjectID,
			},
			Location: "US",
		},
	})
	if err != nil {
		// Ignore error if dataset already exists.
		if !strings.Contains(err.Error(), "Already Exists") {
			log.Printf("Dataset.Create: %v", err)
			return nil
		}
	}

	connStr := fmt.Sprintf("bigquery://%s?useStorageReadAPI", testProjectID)
	db, err = sql.Open("bigquery", connStr)
	if err != nil {
		log.Printf("sql.Open: %v", err)
		return nil
	}

	return func() {
		db.Close()
		client.DeleteDataset(ctx, &bigquerypb.DeleteDatasetRequest{
			ProjectId:      testProjectID,
			DeleteContents: true,
			DatasetId:      ds.DatasetReference.DatasetId,
		})
		client.Close()
	}
}

func TestQuery(t *testing.T) {
	if db == nil {
		t.Skip("db not configured")
	}
	rows, err := db.Query("SELECT CURRENT_TIMESTAMP() as foo, SESSION_USER() as bar")
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	for rows.Next() {
		var ts adapt.Timestamp
		var s string
		if err := rows.Scan(&ts, &s); err != nil {
			t.Fatal(err)
		}

		if ts.IsZero() {
			t.Errorf("got zero timestamp, want current")
		}

		if s == "" {
			t.Errorf("got empty string, want session user")
		}

		t.Logf("%v %s", ts, s)
	}
}

func TestNestedQuery(t *testing.T) {
	if db == nil {
		t.Skip("db not configured")
	}
	rows, err := db.Query(`SELECT 40 as age, [STRUCT(STRUCT('1' as a, '2' as b) as object)] as nested`)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	for rows.Next() {
		var s []any
		var age int64
		if err := rows.Scan(&age, &s); err != nil {
			t.Fatal(err)
		}

		if len(s) == 0 {
			t.Errorf("expected map, got %v", s)
		}

		a := s[0].(map[string]any)["object"].(map[string]any)["a"]
		if a != "1" {
			t.Errorf("expected nested.object.a to be 1, got %v(%T)", a, a)
		}

		b := s[0].(map[string]any)["object"].(map[string]any)["b"]
		if b != "2" {

			t.Errorf("expected nested.object.b to be 2, got %v(%T)", b, b)
		}

		if age != 40 {
			t.Errorf("expected 40, got %d", age)
		}

		t.Logf("%v %s", age, s)
	}
}

func TestDML(t *testing.T) {
	if db == nil {
		t.Skip("db not configured")
	}
	ctx := context.Background()
	_, err := db.ExecContext(ctx, fmt.Sprintf("CREATE OR REPLACE TABLE %s.table_dml (x INT64)", testDatasetID))
	if err != nil {
		t.Fatal(err)
	}
	res, err := db.ExecContext(ctx, fmt.Sprintf("INSERT INTO %s.table_dml (x) VALUES (1), (2)", testDatasetID))
	if err != nil {
		t.Fatal(err)
	}
	affected, err := res.RowsAffected()
	if err != nil {
		t.Fatal(err)
	}
	if affected != 2 {
		t.Errorf("got %d, want 2", affected)
	}
}

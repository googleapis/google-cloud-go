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
	"net/http"
	"strings"
	"testing"
	"time"

	"cloud.google.com/go/iam"
	"cloud.google.com/go/internal"
	"cloud.google.com/go/internal/pretty"
	"cloud.google.com/go/internal/testutil"
	gax "github.com/googleapis/gax-go/v2"
	"google.golang.org/api/iterator"
)

func TestIntegration_TableInvalidSchema(t *testing.T) {
	// Check that creating a record field with an empty schema is an error.
	if client == nil {
		t.Skip("Integration tests skipped")
	}
	table := dataset.Table("t_bad")
	schema := Schema{
		{Name: "rec", Type: RecordFieldType, Schema: Schema{}},
	}
	err := table.Create(context.Background(), &TableMetadata{
		Schema:         schema,
		ExpirationTime: testTableExpiration.Add(5 * time.Minute),
	})
	if err == nil {
		t.Fatal("want error, got nil")
	}
	if !hasStatusCode(err, http.StatusBadRequest) {
		t.Fatalf("want a 400 error, got %v", err)
	}
}

func TestIntegration_TableValidSchema(t *testing.T) {
	if client == nil {
		t.Skip("Integration tests skipped")
	}
	ctx := context.Background()
	table := dataset.Table("t_bad")
	schema := Schema{
		{
			Name: "range_dt",
			Type: RangeFieldType,
			RangeElementType: &RangeElementType{
				Type: DateTimeFieldType,
			},
		},
		{Name: "rec", Type: RecordFieldType, Schema: Schema{
			{Name: "inner", Type: IntegerFieldType},
		}},
	}
	err := table.Create(ctx, &TableMetadata{
		Schema: schema,
	})
	if err != nil {
		t.Fatalf("table.Create: %v", err)
	}

	meta, err := table.Metadata(ctx)
	if err != nil {
		t.Fatalf("table.Metadata: %v", err)
	}
	if diff := testutil.Diff(meta.Schema, schema); diff != "" {
		t.Fatalf("got=-, want=+:\n%s", diff)
	}
}

func TestIntegration_TableCreateWithConstraints(t *testing.T) {
	if client == nil {
		t.Skip("Integration tests skipped")
	}
	table := dataset.Table("constraints")
	schema := Schema{
		{Name: "str_col", Type: StringFieldType, MaxLength: 10},
		{Name: "bytes_col", Type: BytesFieldType, MaxLength: 150},
		{Name: "num_col", Type: NumericFieldType, Precision: 20},
		{Name: "bignumeric_col", Type: BigNumericFieldType, Precision: 30, Scale: 5},
	}
	err := table.Create(context.Background(), &TableMetadata{
		Schema:         schema,
		ExpirationTime: testTableExpiration.Add(5 * time.Minute),
	})
	if err != nil {
		t.Fatalf("table create error: %v", err)
	}

	meta, err := table.Metadata(context.Background())
	if err != nil {
		t.Fatalf("couldn't get metadata: %v", err)
	}

	if diff := testutil.Diff(meta.Schema, schema); diff != "" {
		t.Fatalf("got=-, want=+:\n%s", diff)
	}
}

func TestIntegration_TableCreateWithDefaultValues(t *testing.T) {
	if client == nil {
		t.Skip("Integration tests skipped")
	}
	ctx := context.Background()
	table := dataset.Table("defaultvalues")
	schema := Schema{
		{Name: "str_col", Type: StringFieldType, DefaultValueExpression: "'FOO'"},
		{Name: "timestamp_col", Type: TimestampFieldType, DefaultValueExpression: "CURRENT_TIMESTAMP()"},
	}
	err := table.Create(ctx, &TableMetadata{
		Schema:         schema,
		ExpirationTime: testTableExpiration.Add(5 * time.Minute),
	})
	if err != nil {
		t.Fatalf("table create error: %v", err)
	}

	meta, err := table.Metadata(ctx)
	if err != nil {
		t.Fatalf("couldn't get metadata: %v", err)
	}

	if diff := testutil.Diff(meta.Schema, schema); diff != "" {
		t.Fatalf("got=-, want=+:\n%s", diff)
	}

	// SQL creation
	id, _ := table.Identifier(StandardSQLID)
	sql := fmt.Sprintf(`
	    CREATE OR REPLACE TABLE %s (
			str_col STRING DEFAULT 'FOO',
			timestamp_col TIMESTAMP DEFAULT CURRENT_TIMESTAMP(),
		)`, id)
	_, _, err = runQuerySQL(ctx, sql)
	if err != nil {
		t.Fatal(err)
	}
	meta, err = table.Metadata(ctx)
	if err != nil {
		t.Fatalf("couldn't get metadata after sql: %v", err)
	}

	if diff := testutil.Diff(meta.Schema, schema); diff != "" {
		t.Fatalf("sql create: got=-, want=+:\n%s", diff)
	}
}

func TestIntegration_TableCreateView(t *testing.T) {
	if client == nil {
		t.Skip("Integration tests skipped")
	}
	ctx := context.Background()
	table := newTable(t, schema)
	tableIdentifier, _ := table.Identifier(StandardSQLID)
	defer table.Delete(ctx)

	// Test that standard SQL views work.
	view := dataset.Table("t_view_standardsql")
	query := fmt.Sprintf("SELECT APPROX_COUNT_DISTINCT(name) FROM %s", tableIdentifier)
	err := view.Create(context.Background(), &TableMetadata{
		ViewQuery:      query,
		UseStandardSQL: true,
	})
	if err != nil {
		t.Fatalf("table.create: Did not expect an error, got: %v", err)
	}
	if err := view.Delete(ctx); err != nil {
		t.Fatal(err)
	}
}

func TestIntegration_TableMetadata(t *testing.T) {

	if client == nil {
		t.Skip("Integration tests skipped")
	}
	ctx := context.Background()
	table := newTable(t, schema)
	defer table.Delete(ctx)
	// Check table metadata.
	md, err := table.Metadata(ctx)
	if err != nil {
		t.Fatal(err)
	}
	// TODO(jba): check md more thorougly.
	if got, want := md.FullID, fmt.Sprintf("%s:%s.%s", dataset.ProjectID, dataset.DatasetID, table.TableID); got != want {
		t.Errorf("metadata.FullID: got %q, want %q", got, want)
	}
	if got, want := md.Type, RegularTable; got != want {
		t.Errorf("metadata.Type: got %v, want %v", got, want)
	}
	if got, want := md.ExpirationTime, testTableExpiration; !got.Equal(want) {
		t.Errorf("metadata.Type: got %v, want %v", got, want)
	}

	// Check that timePartitioning is nil by default
	if md.TimePartitioning != nil {
		t.Errorf("metadata.TimePartitioning: got %v, want %v", md.TimePartitioning, nil)
	}

	// Create tables that have time partitioning
	partitionCases := []struct {
		timePartitioning TimePartitioning
		wantExpiration   time.Duration
		wantField        string
		wantPruneFilter  bool
	}{
		{TimePartitioning{}, time.Duration(0), "", false},
		{TimePartitioning{Expiration: time.Second}, time.Second, "", false},
		{TimePartitioning{RequirePartitionFilter: true}, time.Duration(0), "", true},
		{
			TimePartitioning{
				Expiration:             time.Second,
				Field:                  "date",
				RequirePartitionFilter: true,
			}, time.Second, "date", true},
	}

	schema2 := Schema{
		{Name: "name", Type: StringFieldType},
		{Name: "date", Type: DateFieldType},
	}

	clustering := &Clustering{
		Fields: []string{"name"},
	}

	// Currently, clustering depends on partitioning.  Interleave testing of the two features.
	for i, c := range partitionCases {
		table := dataset.Table(fmt.Sprintf("t_metadata_partition_nocluster_%v", i))
		clusterTable := dataset.Table(fmt.Sprintf("t_metadata_partition_cluster_%v", i))

		// Create unclustered, partitioned variant and get metadata.
		err = table.Create(context.Background(), &TableMetadata{
			Schema:           schema2,
			TimePartitioning: &c.timePartitioning,
			ExpirationTime:   testTableExpiration,
		})
		if err != nil {
			t.Fatal(err)
		}
		defer table.Delete(ctx)
		md, err := table.Metadata(ctx)
		if err != nil {
			t.Fatal(err)
		}

		// Created clustered table and get metadata.
		err = clusterTable.Create(context.Background(), &TableMetadata{
			Schema:           schema2,
			TimePartitioning: &c.timePartitioning,
			ExpirationTime:   testTableExpiration,
			Clustering:       clustering,
		})
		if err != nil {
			t.Fatal(err)
		}
		clusterMD, err := clusterTable.Metadata(ctx)
		if err != nil {
			t.Fatal(err)
		}

		for _, v := range []*TableMetadata{md, clusterMD} {
			got := v.TimePartitioning
			want := &TimePartitioning{
				Type:                   DayPartitioningType,
				Expiration:             c.wantExpiration,
				Field:                  c.wantField,
				RequirePartitionFilter: c.wantPruneFilter,
			}
			if !testutil.Equal(got, want) {
				t.Errorf("metadata.TimePartitioning: got %v, want %v", got, want)
			}
			// Manipulate RequirePartitionFilter at the table level.
			mdUpdate := TableMetadataToUpdate{
				RequirePartitionFilter: !want.RequirePartitionFilter,
			}

			newmd, err := table.Update(ctx, mdUpdate, "")
			if err != nil {
				t.Errorf("failed to invert RequirePartitionFilter on %s: %v", table.FullyQualifiedName(), err)
			}
			if newmd.RequirePartitionFilter == want.RequirePartitionFilter {
				t.Errorf("inverting table-level RequirePartitionFilter on %s failed, want %t got %t", table.FullyQualifiedName(), !want.RequirePartitionFilter, newmd.RequirePartitionFilter)
			}
			// Also verify that the clone of RequirePartitionFilter in the TimePartitioning message is consistent.
			if newmd.RequirePartitionFilter != newmd.TimePartitioning.RequirePartitionFilter {
				t.Errorf("inconsistent RequirePartitionFilter.  Table: %t, TimePartitioning: %t", newmd.RequirePartitionFilter, newmd.TimePartitioning.RequirePartitionFilter)
			}

		}

		if md.Clustering != nil {
			t.Errorf("metadata.Clustering was not nil on unclustered table %s", table.TableID)
		}
		got := clusterMD.Clustering
		want := clustering
		if clusterMD.Clustering != clustering {
			if !testutil.Equal(got, want) {
				t.Errorf("metadata.Clustering: got %v, want %v", got, want)
			}
		}
	}
}

func TestIntegration_TableMetadataOptions(t *testing.T) {
	if client == nil {
		t.Skip("Integration tests skipped")
	}
	ctx := context.Background()
	testTable := dataset.Table(tableIDs.New())
	id, _ := testTable.Identifier(StandardSQLID)
	sql := "CREATE TABLE %s AS SELECT num FROM UNNEST(GENERATE_ARRAY(0,5)) as num"
	q := client.Query(fmt.Sprintf(sql, id))
	if _, err := q.Read(ctx); err != nil {
		t.Fatalf("failed to create table: %v", err)
	}

	defaultMeta, err := testTable.Metadata(ctx)
	if err != nil {
		t.Fatalf("failed to get default metadata: %v", err)
	}
	if defaultMeta.NumBytes <= 0 {
		t.Errorf("expected default positive NumBytes, got %d", defaultMeta.NumBytes)
	}
	if defaultMeta.LastModifiedTime.IsZero() {
		t.Error("expected default LastModifiedTime to be populated, is zero value")
	}
	// Specify a subset of metadata.
	basicMeta, err := testTable.Metadata(ctx, WithMetadataView(BasicMetadataView))
	if err != nil {
		t.Fatalf("failed to get basic metadata: %v", err)
	}
	if basicMeta.NumBytes != 0 {
		t.Errorf("expected basic NumBytes to be zero, got %d", defaultMeta.NumBytes)
	}
	if !basicMeta.LastModifiedTime.IsZero() {
		t.Errorf("expected basic LastModifiedTime to be zero, is %v", basicMeta.LastModifiedTime)
	}
}

func TestIntegration_TableUpdateLabels(t *testing.T) {
	if client == nil {
		t.Skip("Integration tests skipped")
	}
	ctx := context.Background()
	table := newTable(t, schema)
	defer table.Delete(ctx)

	var tm TableMetadataToUpdate
	tm.SetLabel("label", "value")
	md, err := table.Update(ctx, tm, "")
	if err != nil {
		t.Fatal(err)
	}
	if got, want := md.Labels["label"], "value"; got != want {
		t.Errorf("got %q, want %q", got, want)
	}
	tm = TableMetadataToUpdate{}
	tm.DeleteLabel("label")
	md, err = table.Update(ctx, tm, "")
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := md.Labels["label"]; ok {
		t.Error("label still present after deletion")
	}
}

func TestIntegration_Tables(t *testing.T) {
	if client == nil {
		t.Skip("Integration tests skipped")
	}
	ctx := context.Background()
	table := newTable(t, schema)
	defer table.Delete(ctx)
	wantName := table.FullyQualifiedName()

	// This test is flaky due to eventual consistency.
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	err := internal.Retry(ctx, gax.Backoff{}, func() (stop bool, err error) {
		// Iterate over tables in the dataset.
		it := dataset.Tables(ctx)
		var tableNames []string
		for {
			tbl, err := it.Next()
			if err == iterator.Done {
				break
			}
			if err != nil {
				return false, err
			}
			tableNames = append(tableNames, tbl.FullyQualifiedName())
		}
		// Other tests may be running with this dataset, so there might be more
		// than just our table in the list. So don't try for an exact match; just
		// make sure that our table is there somewhere.
		for _, tn := range tableNames {
			if tn == wantName {
				return true, nil
			}
		}
		return false, fmt.Errorf("got %v\nwant %s in the list", tableNames, wantName)
	})
	if err != nil {
		t.Fatal(err)
	}
}

func TestIntegration_TableIAM(t *testing.T) {
	if client == nil {
		t.Skip("Integration tests skipped")
	}
	ctx := context.Background()
	table := newTable(t, schema)
	defer table.Delete(ctx)

	// Check to confirm some of our default permissions.
	checkedPerms := []string{"bigquery.tables.get",
		"bigquery.tables.getData", "bigquery.tables.update"}
	perms, err := table.IAM().TestPermissions(ctx, checkedPerms)
	if err != nil {
		t.Fatalf("IAM().TestPermissions: %v", err)
	}
	if len(perms) != len(checkedPerms) {
		t.Errorf("mismatch in permissions, got (%s) wanted (%s)", strings.Join(perms, " "), strings.Join(checkedPerms, " "))
	}

	// Get existing policy, add a binding for all authenticated users.
	policy, err := table.IAM().Policy(ctx)
	if err != nil {
		t.Fatalf("IAM().Policy: %v", err)
	}
	wantedRole := iam.RoleName("roles/bigquery.dataViewer")
	wantedMember := "allAuthenticatedUsers"
	policy.Add(wantedMember, wantedRole)
	if err := table.IAM().SetPolicy(ctx, policy); err != nil {
		t.Fatalf("IAM().SetPolicy: %v", err)
	}

	// Verify policy mutations were persisted by refetching policy.
	updatedPolicy, err := table.IAM().Policy(ctx)
	if err != nil {
		t.Fatalf("IAM.Policy (after update): %v", err)
	}
	foundRole := false
	for _, r := range updatedPolicy.Roles() {
		if r == wantedRole {
			foundRole = true
			break
		}
	}
	if !foundRole {
		t.Errorf("Did not find added role %s in the set of %d roles.",
			wantedRole, len(updatedPolicy.Roles()))
	}
	members := updatedPolicy.Members(wantedRole)
	foundMember := false
	for _, m := range members {
		if m == wantedMember {
			foundMember = true
			break
		}
	}
	if !foundMember {
		t.Errorf("Did not find member %s in role %s", wantedMember, wantedRole)
	}
}

func TestIntegration_TableUpdate(t *testing.T) {
	if client == nil {
		t.Skip("Integration tests skipped")
	}
	ctx := context.Background()
	table := newTable(t, schema)
	defer table.Delete(ctx)

	// Test Update of non-schema fields.
	tm, err := table.Metadata(ctx)
	if err != nil {
		t.Fatal(err)
	}
	wantDescription := tm.Description + "more"
	wantName := tm.Name + "more"
	wantExpiration := tm.ExpirationTime.Add(time.Hour * 24)
	got, err := table.Update(ctx, TableMetadataToUpdate{
		Description:    wantDescription,
		Name:           wantName,
		ExpirationTime: wantExpiration,
	}, tm.ETag)
	if err != nil {
		t.Fatal(err)
	}
	if got.Description != wantDescription {
		t.Errorf("Description: got %q, want %q", got.Description, wantDescription)
	}
	if got.Name != wantName {
		t.Errorf("Name: got %q, want %q", got.Name, wantName)
	}
	if got.ExpirationTime != wantExpiration {
		t.Errorf("ExpirationTime: got %q, want %q", got.ExpirationTime, wantExpiration)
	}
	if !testutil.Equal(got.Schema, schema) {
		t.Errorf("Schema: got %v, want %v", pretty.Value(got.Schema), pretty.Value(schema))
	}

	// Blind write succeeds.
	_, err = table.Update(ctx, TableMetadataToUpdate{Name: "x"}, "")
	if err != nil {
		t.Fatal(err)
	}
	// Write with old etag fails.
	_, err = table.Update(ctx, TableMetadataToUpdate{Name: "y"}, got.ETag)
	if err == nil {
		t.Fatal("Update with old ETag succeeded, wanted failure")
	}

	// Test schema update.
	// Columns can be added. schema2 is the same as schema, except for the
	// added column in the middle.
	nested := Schema{
		{Name: "nested", Type: BooleanFieldType},
		{Name: "other", Type: StringFieldType},
	}
	schema2 := Schema{
		schema[0],
		{Name: "rec2", Type: RecordFieldType, Schema: nested},
		schema[1],
		schema[2],
	}

	got, err = table.Update(ctx, TableMetadataToUpdate{Schema: schema2}, "")
	if err != nil {
		t.Fatal(err)
	}

	// Wherever you add the column, it appears at the end.
	schema3 := Schema{schema2[0], schema2[2], schema2[3], schema2[1]}
	if !testutil.Equal(got.Schema, schema3) {
		t.Errorf("add field:\ngot  %v\nwant %v",
			pretty.Value(got.Schema), pretty.Value(schema3))
	}

	// Updating with the empty schema succeeds, but is a no-op.
	got, err = table.Update(ctx, TableMetadataToUpdate{Schema: Schema{}}, "")
	if err != nil {
		t.Fatal(err)
	}
	if !testutil.Equal(got.Schema, schema3) {
		t.Errorf("empty schema:\ngot  %v\nwant %v",
			pretty.Value(got.Schema), pretty.Value(schema3))
	}

	// Error cases when updating schema.
	for _, test := range []struct {
		desc   string
		fields Schema
	}{
		{"change from optional to required", Schema{
			{Name: "name", Type: StringFieldType, Required: true},
			schema3[1],
			schema3[2],
			schema3[3],
		}},
		{"add a required field", Schema{
			schema3[0], schema3[1], schema3[2], schema3[3],
			{Name: "req", Type: StringFieldType, Required: true},
		}},
		{"remove a field", Schema{schema3[0], schema3[1], schema3[2]}},
		{"remove a nested field", Schema{
			schema3[0], schema3[1], schema3[2],
			{Name: "rec2", Type: RecordFieldType, Schema: Schema{nested[0]}}}},
		{"remove all nested fields", Schema{
			schema3[0], schema3[1], schema3[2],
			{Name: "rec2", Type: RecordFieldType, Schema: Schema{}}}},
	} {
		_, err = table.Update(ctx, TableMetadataToUpdate{Schema: Schema(test.fields)}, "")
		if err == nil {
			t.Errorf("%s: want error, got nil", test.desc)
		} else if !hasStatusCode(err, 400) {
			t.Errorf("%s: want 400, got %v", test.desc, err)
		}
	}
}

func TestIntegration_TableUseLegacySQL(t *testing.T) {
	// Test UseLegacySQL and UseStandardSQL for Table.Create.
	if client == nil {
		t.Skip("Integration tests skipped")
	}
	ctx := context.Background()
	table := newTable(t, schema)
	defer table.Delete(ctx)
	for i, test := range useLegacySQLTests {
		view := dataset.Table(fmt.Sprintf("t_view_%d", i))
		tm := &TableMetadata{
			ViewQuery:      fmt.Sprintf("SELECT word from %s", test.t),
			UseStandardSQL: test.std,
			UseLegacySQL:   test.legacy,
		}
		err := view.Create(ctx, tm)
		gotErr := err != nil
		if gotErr && !test.err {
			t.Errorf("%+v:\nunexpected error: %v", test, err)
		} else if !gotErr && test.err {
			t.Errorf("%+v:\nsucceeded, but want error", test)
		}
		_ = view.Delete(ctx)
	}
}

func TestIntegration_TableDefaultCollation(t *testing.T) {
	// Test DefaultCollation for Table.Create and Table.Update
	if client == nil {
		t.Skip("Integration tests skipped")
	}
	ctx := context.Background()
	table := dataset.Table(tableIDs.New())
	caseInsensitiveCollation := "und:ci"
	caseSensitiveCollation := ""
	err := table.Create(context.Background(), &TableMetadata{
		Schema:           schema,
		DefaultCollation: caseInsensitiveCollation,
		ExpirationTime:   testTableExpiration,
	})
	if err != nil {
		t.Fatal(err)
	}
	defer table.Delete(ctx)
	md, err := table.Metadata(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if md.DefaultCollation != caseInsensitiveCollation {
		t.Fatalf("expected default collation to be %q, but found %q", caseInsensitiveCollation, md.DefaultCollation)
	}
	for _, field := range md.Schema {
		if field.Type == StringFieldType {
			if field.Collation != caseInsensitiveCollation {
				t.Fatalf("expected all columns to have collation %q, but found %q on field :%v", caseInsensitiveCollation, field.Collation, field.Name)
			}
		}
	}

	// Update table DefaultCollation to case-sensitive
	md, err = table.Update(ctx, TableMetadataToUpdate{
		DefaultCollation: caseSensitiveCollation,
	}, "")
	if err != nil {
		t.Fatal(err)
	}
	if md.DefaultCollation != caseSensitiveCollation {
		t.Fatalf("expected default collation to be %q, but found %q", caseSensitiveCollation, md.DefaultCollation)
	}

	// Add a field with different case-insensitive collation
	updatedSchema := md.Schema
	updatedSchema = append(updatedSchema, &FieldSchema{
		Name:      "another_name",
		Type:      StringFieldType,
		Collation: caseInsensitiveCollation,
	})
	md, err = table.Update(ctx, TableMetadataToUpdate{
		Schema: updatedSchema,
	}, "")
	if err != nil {
		t.Fatal(err)
	}
	if md.DefaultCollation != caseSensitiveCollation {
		t.Fatalf("expected default collation to be %q, but found %q", caseSensitiveCollation, md.DefaultCollation)
	}
	for _, field := range md.Schema {
		if field.Type == StringFieldType {
			if field.Collation != caseInsensitiveCollation {
				t.Fatalf("expected all columns to have collation %q, but found %q on field :%v", caseInsensitiveCollation, field.Collation, field.Name)
			}
		}
	}
}

func TestIntegration_TableConstraintsPK(t *testing.T) {
	// Test Primary Keys for Table.Create and Table.Update
	if client == nil {
		t.Skip("Integration tests skipped")
	}
	ctx := context.Background()
	table := dataset.Table(tableIDs.New())
	err := table.Create(context.Background(), &TableMetadata{
		Schema: schema,
		TableConstraints: &TableConstraints{
			PrimaryKey: &PrimaryKey{
				Columns: []string{"name"},
			},
		},
		ExpirationTime: testTableExpiration,
	})
	if err != nil {
		t.Fatal(err)
	}
	defer table.Delete(ctx)
	md, err := table.Metadata(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if md.TableConstraints.PrimaryKey.Columns[0] != "name" {
		t.Fatalf("expected table primary key to contain column `name`, but found %q", md.TableConstraints.PrimaryKey.Columns)
	}

	md, err = table.Update(ctx, TableMetadataToUpdate{
		TableConstraints: &TableConstraints{
			PrimaryKey: &PrimaryKey{}, // clean primary keys
		},
	}, "")
	if err != nil {
		t.Fatal(err)
	}
	if md.TableConstraints != nil {
		t.Fatalf("expected table primary keys to be removed, but found %v", md.TableConstraints.PrimaryKey)
	}

	tableNoPK := dataset.Table(tableIDs.New())
	err = tableNoPK.Create(context.Background(), &TableMetadata{
		Schema:         schema,
		ExpirationTime: testTableExpiration,
	})
	if err != nil {
		t.Fatal(err)
	}
	defer tableNoPK.Delete(ctx)
	md, err = tableNoPK.Metadata(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if md.TableConstraints != nil {
		t.Fatalf("expected table to not have a PK, but found %v", md.TableConstraints.PrimaryKey.Columns)
	}

	md, err = tableNoPK.Update(ctx, TableMetadataToUpdate{
		TableConstraints: &TableConstraints{
			PrimaryKey: &PrimaryKey{
				Columns: []string{"name"},
			},
		},
	}, "")
	if err != nil {
		t.Fatal(err)
	}
	if md.TableConstraints.PrimaryKey == nil || md.TableConstraints.PrimaryKey.Columns[0] != "name" {
		t.Fatalf("expected table primary key to contain column `name`, but found %v", md.TableConstraints.PrimaryKey)
	}
}

func TestIntegration_TableConstraintsFK(t *testing.T) {
	// Test Foreign keys for Table.Create and Table.Update
	if client == nil {
		t.Skip("Integration tests skipped")
	}
	ctx := context.Background()
	tableA := dataset.Table(tableIDs.New())
	schemaA := []*FieldSchema{
		{Name: "id", Type: IntegerFieldType},
		{Name: "name", Type: StringFieldType},
	}
	err := tableA.Create(context.Background(), &TableMetadata{
		Schema: schemaA,
		TableConstraints: &TableConstraints{
			PrimaryKey: &PrimaryKey{
				Columns: []string{"id"},
			},
		},
		ExpirationTime: testTableExpiration,
	})
	if err != nil {
		t.Fatal(err)
	}
	defer tableA.Delete(ctx)

	tableB := dataset.Table(tableIDs.New())
	schemaB := []*FieldSchema{
		{Name: "id", Type: IntegerFieldType},
		{Name: "name", Type: StringFieldType},
		{Name: "parent", Type: IntegerFieldType},
	}
	err = tableB.Create(context.Background(), &TableMetadata{
		Schema: schemaB,
		TableConstraints: &TableConstraints{
			PrimaryKey: &PrimaryKey{
				Columns: []string{"id"},
			},
			ForeignKeys: []*ForeignKey{
				{
					Name:            "table_a_fk",
					ReferencedTable: tableA,
					ColumnReferences: []*ColumnReference{
						{
							ReferencingColumn: "parent",
							ReferencedColumn:  "id",
						},
					},
				},
			},
		},
		ExpirationTime: testTableExpiration,
	})
	if err != nil {
		t.Fatal(err)
	}
	defer tableB.Delete(ctx)
	md, err := tableB.Metadata(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(md.TableConstraints.ForeignKeys) >= 0 && md.TableConstraints.ForeignKeys[0].Name != "table_a_fk" {
		t.Fatalf("expected table to contains fk `table_a_fk`, but found %v", md.TableConstraints.ForeignKeys)
	}

	md, err = tableB.Update(ctx, TableMetadataToUpdate{
		TableConstraints: &TableConstraints{
			ForeignKeys: []*ForeignKey{}, // clean foreign keys
		},
	}, "")
	if err != nil {
		t.Fatal(err)
	}
	if len(md.TableConstraints.ForeignKeys) > 0 {
		t.Fatalf("expected table foreign keys to be removed, but found %v", md.TableConstraints.ForeignKeys)
	}

	tableNoFK := dataset.Table(tableIDs.New())
	err = tableNoFK.Create(context.Background(), &TableMetadata{
		Schema: schemaB,
		TableConstraints: &TableConstraints{
			PrimaryKey: &PrimaryKey{
				Columns: []string{"id"},
			},
		},
		ExpirationTime: testTableExpiration,
	})
	if err != nil {
		t.Fatal(err)
	}
	defer tableNoFK.Delete(ctx)
	md, err = tableNoFK.Update(ctx, TableMetadataToUpdate{
		TableConstraints: &TableConstraints{
			ForeignKeys: []*ForeignKey{
				{
					Name:            "table_a_fk",
					ReferencedTable: tableA,
					ColumnReferences: []*ColumnReference{
						{
							ReferencedColumn:  "id",
							ReferencingColumn: "parent",
						},
					},
				},
			},
		},
	}, "")
	if err != nil {
		t.Fatal(err)
	}
	if len(md.TableConstraints.ForeignKeys) == 0 || md.TableConstraints.ForeignKeys[0].Name != "table_a_fk" {
		t.Fatalf("expected table to contains fk `table_a_fk`, but found %v", md.TableConstraints.ForeignKeys)
	}
}

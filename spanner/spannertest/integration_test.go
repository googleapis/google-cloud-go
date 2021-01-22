/*
Copyright 2019 Google LLC

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

/*
This file holds tests for the in-memory fake for comparing it against a real Cloud Spanner.

By default it uses the Spanner client against the in-memory fake; set the
-test_db flag to "projects/P/instances/I/databases/D" to make it use a real
Cloud Spanner database instead. You may need to provide -timeout=5m too.
*/

package spannertest

import (
	"context"
	"flag"
	"fmt"
	"reflect"
	"sort"
	"testing"
	"time"

	"cloud.google.com/go/civil"
	"cloud.google.com/go/spanner"
	dbadmin "cloud.google.com/go/spanner/admin/database/apiv1"
	"google.golang.org/api/iterator"
	"google.golang.org/api/option"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	dbadminpb "google.golang.org/genproto/googleapis/spanner/admin/database/v1"
	spannerpb "google.golang.org/genproto/googleapis/spanner/v1"
	structpb "google.golang.org/protobuf/types/known/structpb"
)

var testDBFlag = flag.String("test_db", "", "Fully-qualified database name to test against; empty means use an in-memory fake.")

func dbName() string {
	if *testDBFlag != "" {
		return *testDBFlag
	}
	return "projects/fake-proj/instances/fake-instance/databases/fake-db"
}

func makeClient(t *testing.T) (*spanner.Client, *dbadmin.DatabaseAdminClient, func()) {
	// Despite the docs, this context is also used for auth,
	// so it needs to be long-lived.
	ctx := context.Background()

	if *testDBFlag != "" {
		t.Logf("Using real Spanner DB %s", *testDBFlag)
		dialOpt := option.WithGRPCDialOption(grpc.WithTimeout(5 * time.Second))
		client, err := spanner.NewClient(ctx, *testDBFlag, dialOpt)
		if err != nil {
			t.Fatalf("Connecting to %s: %v", *testDBFlag, err)
		}
		adminClient, err := dbadmin.NewDatabaseAdminClient(ctx, dialOpt)
		if err != nil {
			client.Close()
			t.Fatalf("Connecting DB admin client: %v", err)
		}
		return client, adminClient, func() { client.Close(); adminClient.Close() }
	}

	// Don't use SPANNER_EMULATOR_HOST because we need the raw connection for
	// the database admin client anyway.

	t.Logf("Using in-memory fake Spanner DB")
	srv, err := NewServer("localhost:0")
	if err != nil {
		t.Fatalf("Starting in-memory fake: %v", err)
	}
	srv.SetLogger(t.Logf)
	dialCtx, cancel := context.WithTimeout(ctx, 1*time.Second)
	defer cancel()
	conn, err := grpc.DialContext(dialCtx, srv.Addr, grpc.WithInsecure())
	if err != nil {
		srv.Close()
		t.Fatalf("Dialing in-memory fake: %v", err)
	}
	client, err := spanner.NewClient(ctx, dbName(), option.WithGRPCConn(conn))
	if err != nil {
		srv.Close()
		t.Fatalf("Connecting to in-memory fake: %v", err)
	}
	adminClient, err := dbadmin.NewDatabaseAdminClient(ctx, option.WithGRPCConn(conn))
	if err != nil {
		srv.Close()
		t.Fatalf("Connecting to in-memory fake DB admin: %v", err)
	}
	return client, adminClient, func() {
		client.Close()
		adminClient.Close()
		conn.Close()
		srv.Close()
	}
}

func TestIntegration_SpannerBasics(t *testing.T) {
	client, adminClient, cleanup := makeClient(t)
	defer cleanup()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	// Do a trivial query to verify the connection works.
	it := client.Single().Query(ctx, spanner.NewStatement("SELECT 1"))
	row, err := it.Next()
	if err != nil {
		t.Fatalf("Getting first row of trivial query: %v", err)
	}
	var value int64
	if err := row.Column(0, &value); err != nil {
		t.Fatalf("Decoding row data from trivial query: %v", err)
	}
	if value != 1 {
		t.Errorf("Trivial query gave %d, want 1", value)
	}
	// There shouldn't be a next row.
	_, err = it.Next()
	if err != iterator.Done {
		t.Errorf("Reading second row of trivial query gave %v, want iterator.Done", err)
	}
	it.Stop()

	// Drop any previous test table/index, and make a fresh one in a few stages.
	const tableName = "Characters"
	err = updateDDL(t, adminClient, "DROP INDEX AgeIndex")
	// NotFound is an acceptable failure mode here.
	if st, _ := status.FromError(err); st.Code() == codes.NotFound {
		err = nil
	}
	if err != nil {
		t.Fatalf("Dropping old index: %v", err)
	}
	if err := dropTable(t, adminClient, tableName); err != nil {
		t.Fatal(err)
	}
	err = updateDDL(t, adminClient,
		`CREATE TABLE `+tableName+` (
			FirstName STRING(20) NOT NULL,
			LastName STRING(20) NOT NULL,
			Alias STRING(MAX),
		) PRIMARY KEY (FirstName, LastName)`)
	if err != nil {
		t.Fatalf("Setting up fresh table: %v", err)
	}
	err = updateDDL(t, adminClient,
		`ALTER TABLE `+tableName+` ADD COLUMN Age INT64`,
		`CREATE INDEX AgeIndex ON `+tableName+` (Age DESC)`)
	if err != nil {
		t.Fatalf("Adding new column: %v", err)
	}

	// Insert some data.
	_, err = client.Apply(ctx, []*spanner.Mutation{
		spanner.Insert(tableName,
			[]string{"FirstName", "LastName", "Alias", "Age"},
			[]interface{}{"Steve", "Rogers", "Captain America", 101}),
		spanner.Insert(tableName,
			[]string{"LastName", "FirstName", "Age", "Alias"},
			[]interface{}{"Romanoff", "Natasha", 35, "Black Widow"}),
		spanner.Insert(tableName,
			[]string{"Age", "Alias", "FirstName", "LastName"},
			[]interface{}{49, "Iron Man", "Tony", "Stark"}),
		spanner.Insert(tableName,
			[]string{"FirstName", "Alias", "LastName"}, // no Age
			[]interface{}{"Clark", "Superman", "Kent"}),
		// Two rows with the same value in one column,
		// but with distinct primary keys.
		spanner.Insert(tableName,
			[]string{"FirstName", "LastName", "Alias"},
			[]interface{}{"Peter", "Parker", "Spider-Man"}),
		spanner.Insert(tableName,
			[]string{"FirstName", "LastName", "Alias"},
			[]interface{}{"Peter", "Quill", "Star-Lord"}),
	})
	if err != nil {
		t.Fatalf("Applying mutations: %v", err)
	}

	// Delete some data.
	_, err = client.Apply(ctx, []*spanner.Mutation{
		// Whoops. DC, not MCU.
		spanner.Delete(tableName, spanner.Key{"Clark", "Kent"}),
	})
	if err != nil {
		t.Fatalf("Applying mutations: %v", err)
	}

	// Read a single row.
	row, err = client.Single().ReadRow(ctx, tableName, spanner.Key{"Tony", "Stark"}, []string{"Alias", "Age"})
	if err != nil {
		t.Fatalf("Reading single row: %v", err)
	}
	var alias string
	var age int64
	if err := row.Columns(&alias, &age); err != nil {
		t.Fatalf("Decoding single row: %v", err)
	}
	if alias != "Iron Man" || age != 49 {
		t.Errorf(`Single row read gave (%q, %d), want ("Iron Man", 49)`, alias, age)
	}

	// Read all rows, and do a local age sum.
	rows := client.Single().Read(ctx, tableName, spanner.AllKeys(), []string{"Age"})
	var ageSum int64
	err = rows.Do(func(row *spanner.Row) error {
		var age spanner.NullInt64
		if err := row.Columns(&age); err != nil {
			return err
		}
		if age.Valid {
			ageSum += age.Int64
		}
		return nil
	})
	if err != nil {
		t.Fatalf("Iterating over all row read: %v", err)
	}
	if want := int64(101 + 35 + 49); ageSum != want {
		t.Errorf("Age sum after iterating over all rows = %d, want %d", ageSum, want)
	}

	// Do a more complex query to find the aliases of the two oldest non-centenarian characters.
	stmt := spanner.NewStatement(`SELECT Alias FROM ` + tableName + ` WHERE Age < @ageLimit AND Alias IS NOT NULL ORDER BY Age DESC LIMIT @limit`)
	stmt.Params = map[string]interface{}{
		"ageLimit": 100,
		"limit":    2,
	}
	rows = client.Single().Query(ctx, stmt)
	var oldFolk []string
	err = rows.Do(func(row *spanner.Row) error {
		var alias string
		if err := row.Columns(&alias); err != nil {
			return err
		}
		oldFolk = append(oldFolk, alias)
		return nil
	})
	if err != nil {
		t.Fatalf("Iterating over complex query: %v", err)
	}
	if want := []string{"Iron Man", "Black Widow"}; !reflect.DeepEqual(oldFolk, want) {
		t.Errorf("Complex query results = %v, want %v", oldFolk, want)
	}

	// Apply an update.
	_, err = client.Apply(ctx, []*spanner.Mutation{
		spanner.Update(tableName,
			[]string{"FirstName", "LastName", "Age"},
			[]interface{}{"Steve", "Rogers", 102}),
	})
	if err != nil {
		t.Fatalf("Applying mutations: %v", err)
	}
	row, err = client.Single().ReadRow(ctx, tableName, spanner.Key{"Steve", "Rogers"}, []string{"Age"})
	if err != nil {
		t.Fatalf("Reading single row: %v", err)
	}
	if err := row.Columns(&age); err != nil {
		t.Fatalf("Decoding single row: %v", err)
	}
	if age != 102 {
		t.Errorf("After updating Captain America, age = %d, want 102", age)
	}

	// Do a query where the result type isn't deducible from the first row.
	stmt = spanner.NewStatement(`SELECT Age FROM ` + tableName + ` WHERE FirstName = "Peter"`)
	rows = client.Single().Query(ctx, stmt)
	var nullPeters int
	err = rows.Do(func(row *spanner.Row) error {
		var age spanner.NullInt64
		if err := row.Column(0, &age); err != nil {
			return err
		}
		if age.Valid {
			t.Errorf("Got non-NULL Age %d for a Peter", age.Int64)
		} else {
			nullPeters++
		}
		return nil
	})
	if err != nil {
		t.Fatalf("Counting Peters with NULL Ages: %v", err)
	}
	if nullPeters != 2 {
		t.Errorf("Found %d Peters with NULL Ages, want 2", nullPeters)
	}

	// Check handling of array types.
	err = updateDDL(t, adminClient, `ALTER TABLE `+tableName+` ADD COLUMN Allies ARRAY<STRING(20)>`)
	if err != nil {
		t.Fatalf("Adding new array-typed column: %v", err)
	}
	_, err = client.Apply(ctx, []*spanner.Mutation{
		spanner.Update(tableName,
			[]string{"FirstName", "LastName", "Allies"},
			[]interface{}{"Steve", "Rogers", []string{}}),
		spanner.Update(tableName,
			[]string{"FirstName", "LastName", "Allies"},
			[]interface{}{"Tony", "Stark", []string{"Black Widow", "Spider-Man"}}),
	})
	if err != nil {
		t.Fatalf("Applying mutations: %v", err)
	}
	row, err = client.Single().ReadRow(ctx, tableName, spanner.Key{"Tony", "Stark"}, []string{"Allies"})
	if err != nil {
		t.Fatalf("Reading row with array value: %v", err)
	}
	var names []string
	if err := row.Column(0, &names); err != nil {
		t.Fatalf("Unpacking array value: %v", err)
	}
	if want := []string{"Black Widow", "Spider-Man"}; !reflect.DeepEqual(names, want) {
		t.Errorf("Read array value: got %q, want %q", names, want)
	}
	row, err = client.Single().ReadRow(ctx, tableName, spanner.Key{"Steve", "Rogers"}, []string{"Allies"})
	if err != nil {
		t.Fatalf("Reading row with empty array value: %v", err)
	}
	if err := row.Column(0, &names); err != nil {
		t.Fatalf("Unpacking empty array value: %v", err)
	}
	if len(names) > 0 {
		t.Errorf("Read empty array value: got %q", names)
	}

	// Exercise commit timestamp.
	err = updateDDL(t, adminClient, `ALTER TABLE `+tableName+` ADD COLUMN Updated TIMESTAMP OPTIONS (allow_commit_timestamp=true)`)
	if err != nil {
		t.Fatalf("Adding new timestamp column: %v", err)
	}
	cts, err := client.Apply(ctx, []*spanner.Mutation{
		// Update one row in place.
		spanner.Update(tableName,
			[]string{"FirstName", "LastName", "Allies", "Updated"},
			[]interface{}{"Tony", "Stark", []string{"Spider-Man", "Professor Hulk"}, spanner.CommitTimestamp}),
	})
	if err != nil {
		t.Fatalf("Applying mutations: %v", err)
	}
	cts = cts.In(time.UTC)
	if d := time.Since(cts); d < 0 || d > 10*time.Second {
		t.Errorf("Commit timestamp %v not in the last 10s", cts)
	}
	row, err = client.Single().ReadRow(ctx, tableName, spanner.Key{"Tony", "Stark"}, []string{"Allies", "Updated"})
	if err != nil {
		t.Fatalf("Reading single row: %v", err)
	}
	var gotAllies []string
	var gotUpdated time.Time
	if err := row.Columns(&gotAllies, &gotUpdated); err != nil {
		t.Fatalf("Decoding single row: %v", err)
	}
	if want := []string{"Spider-Man", "Professor Hulk"}; !reflect.DeepEqual(gotAllies, want) {
		t.Errorf("After updating Iron Man, allies = %+v, want %+v", gotAllies, want)
	}
	if !gotUpdated.Equal(cts) {
		t.Errorf("After updating Iron Man, updated = %v, want %v", gotUpdated, cts)
	}

	// Check if IN UNNEST works.
	stmt = spanner.NewStatement(`SELECT Age FROM ` + tableName + ` WHERE FirstName IN UNNEST(@list)`)
	stmt.Params = map[string]interface{}{
		"list": []string{"Peter", "Steve"},
	}
	rows = client.Single().Query(ctx, stmt)
	var ages []int64
	err = rows.Do(func(row *spanner.Row) error {
		var age spanner.NullInt64
		if err := row.Column(0, &age); err != nil {
			return err
		}
		ages = append(ages, age.Int64) // zero for NULL
		return nil
	})
	if err != nil {
		t.Fatalf("Getting ages using IN UNNEST: %v", err)
	}
	sort.Slice(ages, func(i, j int) bool { return ages[i] < ages[j] })
	wantAges := []int64{0, 0, 102} // Peter Parker, Peter Quill, Steve Rogers (modified)
	if !reflect.DeepEqual(ages, wantAges) {
		t.Errorf("Query with IN UNNEST gave wrong ages: got %+v, want %+v", ages, wantAges)
	}
}

func TestIntegration_ReadsAndQueries(t *testing.T) {
	client, adminClient, cleanup := makeClient(t)
	defer cleanup()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	// Drop any old tables.
	// Do them all concurrently; this saves a lot of time.
	allTables := []string{
		"Staff",
		"PlayerStats",
		"JoinA", "JoinB", "JoinC", "JoinD", "JoinE", "JoinF",
		"SomeStrings", "Updateable",
	}
	errc := make(chan error)
	for _, table := range allTables {
		go func(table string) {
			errc <- dropTable(t, adminClient, table)
		}(table)
	}
	var bad bool
	for range allTables {
		if err := <-errc; err != nil {
			t.Error(err)
			bad = true
		}
	}
	if bad {
		t.FailNow()
	}

	err := updateDDL(t, adminClient,
		`CREATE TABLE Staff (
			Tenure INT64,
			ID INT64,
			Name STRING(MAX),
			Cool BOOL,
			Height FLOAT64,
		) PRIMARY KEY (Name, ID)`)
	if err != nil {
		t.Fatal(err)
	}

	// Insert a subset of columns.
	_, err = client.Apply(ctx, []*spanner.Mutation{
		spanner.Insert("Staff", []string{"ID", "Name", "Tenure", "Height"}, []interface{}{1, "Jack", 10, 1.85}),
		spanner.Insert("Staff", []string{"ID", "Name", "Tenure", "Height"}, []interface{}{2, "Daniel", 11, 1.83}),
	})
	if err != nil {
		t.Fatalf("Inserting data: %v", err)
	}
	// Insert a different set of columns.
	_, err = client.Apply(ctx, []*spanner.Mutation{
		spanner.Insert("Staff", []string{"Name", "ID", "Cool", "Tenure", "Height"}, []interface{}{"Sam", 3, false, 9, 1.75}),
		spanner.Insert("Staff", []string{"Name", "ID", "Cool", "Tenure", "Height"}, []interface{}{"Teal'c", 4, true, 8, 1.91}),
		spanner.Insert("Staff", []string{"Name", "ID", "Cool", "Tenure", "Height"}, []interface{}{"George", 5, nil, 6, 1.73}),
		spanner.Insert("Staff", []string{"Name", "ID", "Cool", "Tenure", "Height"}, []interface{}{"Harry", 6, true, nil, nil}),
	})
	if err != nil {
		t.Fatalf("Inserting more data: %v", err)
	}
	// Delete that last one.
	_, err = client.Apply(ctx, []*spanner.Mutation{
		spanner.Delete("Staff", spanner.Key{"Harry", 6}),
	})
	if err != nil {
		t.Fatalf("Deleting a row: %v", err)
	}
	// Turns out this guy isn't cool after all.
	_, err = client.Apply(ctx, []*spanner.Mutation{
		// Missing columns should be left alone.
		spanner.Update("Staff", []string{"Name", "ID", "Cool"}, []interface{}{"Daniel", 2, false}),
	})
	if err != nil {
		t.Fatalf("Updating a row: %v", err)
	}

	// Read some specific keys.
	ri := client.Single().Read(ctx, "Staff", spanner.KeySets(
		spanner.Key{"George", 5},
		spanner.Key{"Harry", 6}, // Missing key should be silently ignored.
		spanner.Key{"Sam", 3},
		spanner.Key{"George", 5}, // Duplicate key should be silently ignored.
	), []string{"Name", "Tenure"})
	if err != nil {
		t.Fatalf("Reading keys: %v", err)
	}
	all := mustSlurpRows(t, ri)
	wantAll := [][]interface{}{
		{"George", int64(6)},
		{"Sam", int64(9)},
	}
	if !reflect.DeepEqual(all, wantAll) {
		t.Errorf("Read data by keys wrong.\n got %v\nwant %v", all, wantAll)
	}
	// Read the same, but by key range.
	ri = client.Single().Read(ctx, "Staff", spanner.KeySets(
		spanner.KeyRange{Start: spanner.Key{"Gabriel"}, End: spanner.Key{"Harpo"}, Kind: spanner.OpenOpen},
		spanner.KeyRange{Start: spanner.Key{"Sam", 3}, End: spanner.Key{"Teal'c", 4}, Kind: spanner.ClosedOpen},
	), []string{"Name", "Tenure"})
	all = mustSlurpRows(t, ri)
	if !reflect.DeepEqual(all, wantAll) {
		t.Errorf("Read data by key ranges wrong.\n got %v\nwant %v", all, wantAll)
	}
	// Read a subset of all rows, with a limit.
	ri = client.Single().ReadWithOptions(ctx, "Staff", spanner.AllKeys(), []string{"Tenure", "Name", "Height"},
		&spanner.ReadOptions{Limit: 4})
	all = mustSlurpRows(t, ri)
	wantAll = [][]interface{}{
		// Primary key is (Name, ID), so results should come back sorted by Name then ID.
		{int64(11), "Daniel", 1.83},
		{int64(6), "George", 1.73},
		{int64(10), "Jack", 1.85},
		{int64(9), "Sam", 1.75},
	}
	if !reflect.DeepEqual(all, wantAll) {
		t.Errorf("ReadAll data wrong.\n got %v\nwant %v", all, wantAll)
	}

	// Add DATE and TIMESTAMP columns, and populate them with some data.
	err = updateDDL(t, adminClient,
		`ALTER TABLE Staff ADD COLUMN FirstSeen DATE`,
		"ALTER TABLE Staff ADD COLUMN `To` TIMESTAMP", // "TO" is a keyword; needs quoting
	)
	if err != nil {
		t.Fatalf("Adding columns: %v", err)
	}
	_, err = client.Apply(ctx, []*spanner.Mutation{
		spanner.Update("Staff", []string{"Name", "ID", "FirstSeen", "To"}, []interface{}{"Jack", 1, "1994-10-28", nil}),
		spanner.Update("Staff", []string{"Name", "ID", "FirstSeen", "To"}, []interface{}{"Daniel", 2, "1994-10-28", nil}),
		spanner.Update("Staff", []string{"Name", "ID", "FirstSeen", "To"}, []interface{}{"George", 5, "1997-07-27", "2008-07-29T11:22:43Z"}),
	})
	if err != nil {
		t.Fatalf("Updating rows: %v", err)
	}

	// Add some more data, then delete it with a KeyRange.
	// The queries below ensure that this was all deleted.
	_, err = client.Apply(ctx, []*spanner.Mutation{
		spanner.Insert("Staff", []string{"Name", "ID"}, []interface{}{"01", 1}),
		spanner.Insert("Staff", []string{"Name", "ID"}, []interface{}{"03", 3}),
		spanner.Insert("Staff", []string{"Name", "ID"}, []interface{}{"06", 6}),
	})
	if err != nil {
		t.Fatalf("Inserting data: %v", err)
	}
	_, err = client.Apply(ctx, []*spanner.Mutation{
		spanner.Delete("Staff", spanner.KeyRange{
			/* This should work:
				Start: spanner.Key{"01", 1},
				End:   spanner.Key{"9"},
			  However, that is unimplemented in the production Cloud Spanner, which rejects
			  that: ""For delete ranges, start and limit keys may only differ in the final key part"
			*/
			Start: spanner.Key{"01"},
			End:   spanner.Key{"9"},
			Kind:  spanner.ClosedOpen,
		}),
	})
	if err != nil {
		t.Fatalf("Deleting key range: %v", err)
	}
	// Re-add the data and delete with DML.
	_, err = client.Apply(ctx, []*spanner.Mutation{
		spanner.Insert("Staff", []string{"Name", "ID"}, []interface{}{"01", 1}),
		spanner.Insert("Staff", []string{"Name", "ID"}, []interface{}{"03", 3}),
		spanner.Insert("Staff", []string{"Name", "ID"}, []interface{}{"06", 6}),
	})
	if err != nil {
		t.Fatalf("Inserting data: %v", err)
	}
	var n int64
	_, err = client.ReadWriteTransaction(ctx, func(ctx context.Context, tx *spanner.ReadWriteTransaction) error {
		stmt := spanner.NewStatement("DELETE FROM Staff WHERE Name >= @min AND Name < @max")
		stmt.Params["min"] = "01"
		stmt.Params["max"] = "07"
		n, err = tx.Update(ctx, stmt)
		return err
	})
	if err != nil {
		t.Fatalf("Deleting with DML: %v", err)
	}
	if n != 3 {
		t.Errorf("Deleting with DML affected %d rows, want 3", n)
	}

	// Add a BYTES column, and populate it with some data.
	err = updateDDL(t, adminClient, `ALTER TABLE Staff ADD COLUMN RawBytes BYTES(MAX)`)
	if err != nil {
		t.Fatalf("Adding column: %v", err)
	}
	_, err = client.Apply(ctx, []*spanner.Mutation{
		// bytes {0x01 0x00 0x01} encode as base-64 AQAB.
		spanner.Update("Staff", []string{"Name", "ID", "RawBytes"}, []interface{}{"Jack", 1, []byte{0x01, 0x00, 0x01}}),
	})
	if err != nil {
		t.Fatalf("Updating rows: %v", err)
	}

	// Prepare the sample tables from the Cloud Spanner docs.
	// https://cloud.google.com/spanner/docs/query-syntax#appendix-a-examples-with-sample-data
	err = updateDDL(t, adminClient,
		// TODO: Roster, TeamMascot when we implement JOINs.
		`CREATE TABLE PlayerStats (
			LastName STRING(MAX),
			OpponentID INT64,
			PointsScored INT64,
		) PRIMARY KEY (LastName, OpponentID)`, // TODO: is this right?
		// JoinFoo are from https://cloud.google.com/spanner/docs/query-syntax#join_types.
		// They aren't consistently named in the docs.
		`CREATE TABLE JoinA ( w INT64, x STRING(MAX) ) PRIMARY KEY (w, x)`,
		`CREATE TABLE JoinB ( y INT64, z STRING(MAX) ) PRIMARY KEY (y, z)`,
		`CREATE TABLE JoinC ( x INT64, y STRING(MAX) ) PRIMARY KEY (x, y)`,
		`CREATE TABLE JoinD ( x INT64, z STRING(MAX) ) PRIMARY KEY (x, z)`,
		`CREATE TABLE JoinE ( w INT64, x STRING(MAX) ) PRIMARY KEY (w, x)`,
		`CREATE TABLE JoinF ( y INT64, z STRING(MAX) ) PRIMARY KEY (y, z)`,
		// Some other test tables.
		`CREATE TABLE SomeStrings ( i INT64, str STRING(MAX) ) PRIMARY KEY (i)`,
		`CREATE TABLE Updateable (
			id INT64,
			first STRING(MAX),
			last STRING(MAX),
		) PRIMARY KEY (id)`,
	)
	if err != nil {
		t.Fatalf("Creating sample tables: %v", err)
	}
	_, err = client.Apply(ctx, []*spanner.Mutation{
		spanner.Insert("PlayerStats", []string{"LastName", "OpponentID", "PointsScored"}, []interface{}{"Adams", 51, 3}),
		spanner.Insert("PlayerStats", []string{"LastName", "OpponentID", "PointsScored"}, []interface{}{"Buchanan", 77, 0}),
		spanner.Insert("PlayerStats", []string{"LastName", "OpponentID", "PointsScored"}, []interface{}{"Coolidge", 77, 1}),
		spanner.Insert("PlayerStats", []string{"LastName", "OpponentID", "PointsScored"}, []interface{}{"Adams", 52, 4}),
		spanner.Insert("PlayerStats", []string{"LastName", "OpponentID", "PointsScored"}, []interface{}{"Buchanan", 50, 13}),

		spanner.Insert("JoinA", []string{"w", "x"}, []interface{}{1, "a"}),
		spanner.Insert("JoinA", []string{"w", "x"}, []interface{}{2, "b"}),
		spanner.Insert("JoinA", []string{"w", "x"}, []interface{}{3, "c"}),
		spanner.Insert("JoinA", []string{"w", "x"}, []interface{}{3, "d"}),

		spanner.Insert("JoinB", []string{"y", "z"}, []interface{}{2, "k"}),
		spanner.Insert("JoinB", []string{"y", "z"}, []interface{}{3, "m"}),
		spanner.Insert("JoinB", []string{"y", "z"}, []interface{}{3, "n"}),
		spanner.Insert("JoinB", []string{"y", "z"}, []interface{}{4, "p"}),

		// JoinC and JoinD have the same contents as JoinA and JoinB; they have different column names.
		spanner.Insert("JoinC", []string{"x", "y"}, []interface{}{1, "a"}),
		spanner.Insert("JoinC", []string{"x", "y"}, []interface{}{2, "b"}),
		spanner.Insert("JoinC", []string{"x", "y"}, []interface{}{3, "c"}),
		spanner.Insert("JoinC", []string{"x", "y"}, []interface{}{3, "d"}),

		spanner.Insert("JoinD", []string{"x", "z"}, []interface{}{2, "k"}),
		spanner.Insert("JoinD", []string{"x", "z"}, []interface{}{3, "m"}),
		spanner.Insert("JoinD", []string{"x", "z"}, []interface{}{3, "n"}),
		spanner.Insert("JoinD", []string{"x", "z"}, []interface{}{4, "p"}),

		// JoinE and JoinF are used in the CROSS JOIN test.
		spanner.Insert("JoinE", []string{"w", "x"}, []interface{}{1, "a"}),
		spanner.Insert("JoinE", []string{"w", "x"}, []interface{}{2, "b"}),

		spanner.Insert("JoinF", []string{"y", "z"}, []interface{}{2, "c"}),
		spanner.Insert("JoinF", []string{"y", "z"}, []interface{}{3, "d"}),

		spanner.Insert("SomeStrings", []string{"i", "str"}, []interface{}{0, "afoo"}),
		spanner.Insert("SomeStrings", []string{"i", "str"}, []interface{}{1, "abar"}),
		spanner.Insert("SomeStrings", []string{"i", "str"}, []interface{}{2, nil}),
		spanner.Insert("SomeStrings", []string{"i", "str"}, []interface{}{3, "bbar"}),

		spanner.Insert("Updateable", []string{"id", "first", "last"}, []interface{}{0, "joe", nil}),
		spanner.Insert("Updateable", []string{"id", "first", "last"}, []interface{}{1, "doe", "joan"}),
		spanner.Insert("Updateable", []string{"id", "first", "last"}, []interface{}{2, "wong", "wong"}),
	})
	if err != nil {
		t.Fatalf("Inserting sample data: %v", err)
	}

	// Perform UPDATE DML; the results are checked later on.
	n = 0
	_, err = client.ReadWriteTransaction(ctx, func(ctx context.Context, tx *spanner.ReadWriteTransaction) error {
		for _, u := range []string{
			`UPDATE Updateable SET last = "bloggs" WHERE id = 0`,
			`UPDATE Updateable SET first = last, last = first WHERE id = 1`,
			`UPDATE Updateable SET last = DEFAULT WHERE id = 2`,
			`UPDATE Updateable SET first = "noname" WHERE id = 3`, // no id=3
		} {
			nr, err := tx.Update(ctx, spanner.NewStatement(u))
			if err != nil {
				return err
			}
			n += nr
		}
		return nil
	})
	if err != nil {
		t.Fatalf("Updating with DML: %v", err)
	}
	if n != 3 {
		t.Errorf("Updating with DML affected %d rows, want 3", n)
	}

	// Do some complex queries.
	tests := []struct {
		q      string
		params map[string]interface{}
		want   [][]interface{}
	}{
		{
			`SELECT 17, "sweet", TRUE AND FALSE, NULL, B"hello"`,
			nil,
			[][]interface{}{{int64(17), "sweet", false, nil, []byte("hello")}},
		},
		// Check handling of NULL values for the IS operator.
		// There was a bug that returned errors for some of these cases.
		{
			`SELECT @x IS TRUE, @x IS NOT TRUE, @x IS FALSE, @x IS NOT FALSE, @x IS NULL, @x IS NOT NULL`,
			map[string]interface{}{"x": (*bool)(nil)},
			[][]interface{}{
				{false, true, false, true, true, false},
			},
		},
		// Check handling of bools that might be NULL.
		// There was a bug where logical operators always returned true/false.
		{
			`SELECT @x, NOT @x, @x AND TRUE, @x AND FALSE, @x OR TRUE, @x OR FALSE`,
			map[string]interface{}{"x": (*bool)(nil)},
			[][]interface{}{
				// At the time of writing (9 Oct 2020), the docs are wrong for `NULL AND FALSE`;
				// the production Spanner returns FALSE, which is what we match.
				{nil, nil, nil, false, true, nil},
			},
		},
		{
			`SELECT Name FROM Staff WHERE Cool`,
			nil,
			[][]interface{}{{"Teal'c"}},
		},
		{
			`SELECT ID FROM Staff WHERE Cool IS NOT NULL ORDER BY ID DESC`,
			nil,
			[][]interface{}{{int64(4)}, {int64(3)}, {int64(2)}},
		},
		{
			`SELECT Name, Tenure FROM Staff WHERE Cool IS NULL OR Cool ORDER BY Name LIMIT 2`,
			nil,
			[][]interface{}{
				{"George", int64(6)},
				{"Jack", int64(10)},
			},
		},
		{
			`SELECT Name, ID + 100 FROM Staff WHERE @min <= Tenure AND Tenure < @lim ORDER BY Cool, Name DESC LIMIT @numResults`,
			map[string]interface{}{"min": 9, "lim": 11, "numResults": 100},
			[][]interface{}{
				{"Jack", int64(101)},
				{"Sam", int64(103)},
			},
		},
		{
			// Expression in SELECT list.
			`SELECT Name, Cool IS NOT NULL FROM Staff WHERE Tenure/2 > 4 ORDER BY NOT Cool, Name`,
			nil,
			[][]interface{}{
				{"Jack", false},  // Jack has NULL Cool (NULLs always come first in orderings)
				{"Daniel", true}, // Daniel has Cool==true
				{"Sam", true},    // Sam has Cool==false
			},
		},
		{
			`SELECT Name, Height FROM Staff ORDER BY Height DESC LIMIT 2`,
			nil,
			[][]interface{}{
				{"Teal'c", 1.91},
				{"Jack", 1.85},
			},
		},
		{
			`SELECT Name FROM Staff WHERE Name LIKE "J%k" OR Name LIKE "_am"`,
			nil,
			[][]interface{}{
				{"Jack"},
				{"Sam"},
			},
		},
		{
			`SELECT Name, Height FROM Staff WHERE Height BETWEEN @min AND @max ORDER BY Height DESC`,
			map[string]interface{}{"min": 1.75, "max": 1.85},
			[][]interface{}{
				{"Jack", 1.85},
				{"Daniel", 1.83},
				{"Sam", 1.75},
			},
		},
		{
			`SELECT COUNT(*) FROM Staff WHERE Name < "T"`,
			nil,
			[][]interface{}{
				{int64(4)},
			},
		},
		{
			// Check that aggregation still works for the empty set.
			`SELECT COUNT(*) FROM Staff WHERE Name = "Nobody"`,
			nil,
			[][]interface{}{
				{int64(0)},
			},
		},
		{
			`SELECT * FROM Staff WHERE Name LIKE "S%"`,
			nil,
			[][]interface{}{
				// These are returned in table column order, based on the appearance in the DDL.
				// Our internal implementation sorts the primary key columns first,
				// but that should not become visible via SELECT *.
				{int64(9), int64(3), "Sam", false, 1.75, nil, nil, nil},
			},
		},
		{
			// Exactly the same as the previous, except with a redundant ORDER BY clause.
			`SELECT * FROM Staff WHERE Name LIKE "S%" ORDER BY Name`,
			nil,
			[][]interface{}{
				{int64(9), int64(3), "Sam", false, 1.75, nil, nil, nil},
			},
		},
		{
			`SELECT Name FROM Staff WHERE FirstSeen >= @min`,
			map[string]interface{}{"min": civil.Date{Year: 1996, Month: 1, Day: 1}},
			[][]interface{}{
				{"George"},
			},
		},
		{
			`SELECT RawBytes FROM Staff WHERE RawBytes IS NOT NULL`,
			nil,
			[][]interface{}{
				{[]byte("\x01\x00\x01")},
			},
		},
		{
			// The keyword "To" needs quoting in queries.
			// Check coercion of comparison operator literal args too.
			"SELECT COUNT(*) FROM Staff WHERE `To` > '2000-01-01T00:00:00Z'",
			nil,
			[][]interface{}{
				{int64(1)},
			},
		},
		{
			`SELECT DISTINCT Cool, Tenure > 8 FROM Staff ORDER BY Cool`,
			nil,
			[][]interface{}{
				// The non-distinct results are
				//          [[false true] [<nil> false] [<nil> true] [false true] [true false]]
				{nil, false},
				{nil, true},
				{false, true},
				{true, false},
			},
		},
		{
			`SELECT Name FROM Staff WHERE ID IN UNNEST(@ids)`,
			map[string]interface{}{"ids": []int64{3, 1}},
			[][]interface{}{
				{"Jack"},
				{"Sam"},
			},
		},
		// From https://cloud.google.com/spanner/docs/query-syntax#group-by-clause_1:
		{
			// TODO: Ordering matters? Our implementation sorts by the GROUP BY key,
			// but nothing documented seems to guarantee that.
			`SELECT LastName, SUM(PointsScored) FROM PlayerStats GROUP BY LastName`,
			nil,
			[][]interface{}{
				{"Adams", int64(7)},
				{"Buchanan", int64(13)},
				{"Coolidge", int64(1)},
			},
		},
		{
			// Another GROUP BY, but referring to an alias.
			// Group by ID oddness, SUM over Tenure.
			`SELECT ID&0x01 AS odd, SUM(Tenure) FROM Staff GROUP BY odd`,
			nil,
			[][]interface{}{
				{int64(0), int64(19)}, // Daniel(ID=2, Tenure=11), Teal'c(ID=4, Tenure=8)
				{int64(1), int64(25)}, // Jack(ID=1, Tenure=10), Sam(ID=3, Tenure=9), George(ID=5, Tenure=6)
			},
		},
		{
			// From https://cloud.google.com/spanner/docs/aggregate_functions#avg.
			`SELECT AVG(x) AS avg FROM UNNEST([0, 2, 4, 4, 5]) AS x`,
			nil,
			[][]interface{}{
				{float64(3)},
			},
		},
		{
			`SELECT MAX(Name) FROM Staff WHERE Name < @lim`,
			map[string]interface{}{"lim": "Teal'c"},
			[][]interface{}{
				{"Sam"},
			},
		},
		{
			`SELECT MAX(Name) FROM Staff WHERE Cool = @p1 LIMIT 1`,
			map[string]interface{}{"p1": true},
			[][]interface{}{
				{"Teal'c"},
			},
		},
		{
			`SELECT MIN(Name) FROM Staff`,
			nil,
			[][]interface{}{
				{"Daniel"},
			},
		},
		{
			`SELECT ARRAY_AGG(Cool) FROM Staff`,
			nil,
			[][]interface{}{
				// Daniel, George (NULL), Jack (NULL), Sam, Teal'c
				{[]interface{}{false, nil, nil, false, true}},
			},
		},
		// SELECT with aliases.
		{
			// Aliased table.
			`SELECT s.Name FROM Staff AS s WHERE s.ID = 3 ORDER BY s.Tenure`,
			nil,
			[][]interface{}{
				{"Sam"},
			},
		},
		{
			// Aliased expression.
			`SELECT Name AS nom FROM Staff WHERE ID < 4 ORDER BY nom`,
			nil,
			[][]interface{}{
				{"Daniel"},
				{"Jack"},
				{"Sam"},
			},
		},
		// Joins.
		{
			`SELECT * FROM JoinA INNER JOIN JoinB ON JoinA.w = JoinB.y ORDER BY w, x, y, z`,
			nil,
			[][]interface{}{
				{int64(2), "b", int64(2), "k"},
				{int64(3), "c", int64(3), "m"},
				{int64(3), "c", int64(3), "n"},
				{int64(3), "d", int64(3), "m"},
				{int64(3), "d", int64(3), "n"},
			},
		},
		{
			`SELECT * FROM JoinE CROSS JOIN JoinF ORDER BY w, x, y, z`,
			nil,
			[][]interface{}{
				{int64(1), "a", int64(2), "c"},
				{int64(1), "a", int64(3), "d"},
				{int64(2), "b", int64(2), "c"},
				{int64(2), "b", int64(3), "d"},
			},
		},
		{
			// Same as in docs, but with a weird ORDER BY clause to match the row ordering.
			`SELECT * FROM JoinA FULL OUTER JOIN JoinB ON JoinA.w = JoinB.y ORDER BY w IS NULL, w, x, y, z`,
			nil,
			[][]interface{}{
				{int64(1), "a", nil, nil},
				{int64(2), "b", int64(2), "k"},
				{int64(3), "c", int64(3), "m"},
				{int64(3), "c", int64(3), "n"},
				{int64(3), "d", int64(3), "m"},
				{int64(3), "d", int64(3), "n"},
				{nil, nil, int64(4), "p"},
			},
		},
		{
			// Same as the previous, but using a USING clause instead of an ON clause.
			`SELECT * FROM JoinC FULL OUTER JOIN JoinD USING (x) ORDER BY x, y, z`,
			nil,
			[][]interface{}{
				{int64(1), "a", nil},
				{int64(2), "b", "k"},
				{int64(3), "c", "m"},
				{int64(3), "c", "n"},
				{int64(3), "d", "m"},
				{int64(3), "d", "n"},
				{int64(4), nil, "p"},
			},
		},
		{
			`SELECT * FROM JoinA LEFT OUTER JOIN JoinB AS B ON JoinA.w = B.y ORDER BY w, x, y, z`,
			nil,
			[][]interface{}{
				{int64(1), "a", nil, nil},
				{int64(2), "b", int64(2), "k"},
				{int64(3), "c", int64(3), "m"},
				{int64(3), "c", int64(3), "n"},
				{int64(3), "d", int64(3), "m"},
				{int64(3), "d", int64(3), "n"},
			},
		},
		{
			// Same as the previous, but using a USING clause instead of an ON clause.
			`SELECT * FROM JoinC LEFT OUTER JOIN JoinD USING (x) ORDER BY x, y, z`,
			nil,
			[][]interface{}{
				{int64(1), "a", nil},
				{int64(2), "b", "k"},
				{int64(3), "c", "m"},
				{int64(3), "c", "n"},
				{int64(3), "d", "m"},
				{int64(3), "d", "n"},
			},
		},
		{
			// Same as in docs, but with a weird ORDER BY clause to match the row ordering.
			`SELECT * FROM JoinA RIGHT OUTER JOIN JoinB AS B ON JoinA.w = B.y ORDER BY w IS NULL, w, x, y, z`,
			nil,
			[][]interface{}{
				{int64(2), "b", int64(2), "k"},
				{int64(3), "c", int64(3), "m"},
				{int64(3), "c", int64(3), "n"},
				{int64(3), "d", int64(3), "m"},
				{int64(3), "d", int64(3), "n"},
				{nil, nil, int64(4), "p"},
			},
		},
		{
			`SELECT * FROM JoinC RIGHT OUTER JOIN JoinD USING (x) ORDER BY x, y, z`,
			nil,
			[][]interface{}{
				{int64(2), "b", "k"},
				{int64(3), "c", "m"},
				{int64(3), "c", "n"},
				{int64(3), "d", "m"},
				{int64(3), "d", "n"},
				{int64(4), nil, "p"},
			},
		},
		// Check the output of the UPDATE DML.
		{
			`SELECT id, first, last FROM Updateable ORDER BY id`,
			nil,
			[][]interface{}{
				{int64(0), "joe", "bloggs"},
				{int64(1), "joan", "doe"},
				{int64(2), "wong", nil},
			},
		},
		// Regression test for aggregating no rows; it used to return an empty row.
		// https://github.com/googleapis/google-cloud-go/issues/2793
		{
			`SELECT Cool, ARRAY_AGG(Name) FROM Staff WHERE Name > "zzz" GROUP BY Cool`,
			nil,
			nil,
		},
		// Regression test for evaluating `IN` incorrectly using ==.
		// https://github.com/googleapis/google-cloud-go/issues/2458
		{
			`SELECT COUNT(*) FROM Staff WHERE RawBytes IN UNNEST(@arg)`,
			map[string]interface{}{
				"arg": [][]byte{
					{0x02},
					{0x01, 0x00, 0x01}, // only one present
				},
			},
			[][]interface{}{
				{int64(1)},
			},
		},
		// Regression test for mishandling NULLs with LIKE operator.
		{
			`SELECT i, str FROM SomeStrings WHERE str LIKE "%bar"`,
			nil,
			[][]interface{}{
				// Does not include [0, "afoo"] or [2, nil].
				{int64(1), "abar"},
				{int64(3), "bbar"},
			},
		},
		{
			`SELECT i, str FROM SomeStrings WHERE str NOT LIKE "%bar"`,
			nil,
			[][]interface{}{
				// Does not include [1, "abar"], [2, nil] or [3, "bbar"].
				{int64(0), "afoo"},
			},
		},
		// Regression test for ORDER BY combined with SELECT aliases.
		{
			`SELECT Name AS nom FROM Staff ORDER BY ID LIMIT 2`,
			nil,
			[][]interface{}{
				{"Jack"},
				{"Daniel"},
			},
		},
	}
	var failures int
	for _, test := range tests {
		t.Logf("Testing query: %s", test.q)
		stmt := spanner.NewStatement(test.q)
		stmt.Params = test.params

		ri = client.Single().Query(ctx, stmt)
		all, err := slurpRows(t, ri)
		if err != nil {
			t.Errorf("Query(%q, %v): %v", test.q, test.params, err)
			failures++
			continue
		}
		if !reflect.DeepEqual(all, test.want) {
			t.Errorf("Results from Query(%q, %v) are wrong.\n got %v\nwant %v", test.q, test.params, all, test.want)
			failures++
		}
	}
	if failures > 0 {
		t.Logf("%d queries failed", failures)
	}
}

func dropTable(t *testing.T, adminClient *dbadmin.DatabaseAdminClient, table string) error {
	t.Helper()
	err := updateDDL(t, adminClient, "DROP TABLE "+table)
	// NotFound is an acceptable failure mode here.
	if st, _ := status.FromError(err); st.Code() == codes.NotFound {
		err = nil
	}
	if err != nil {
		return fmt.Errorf("dropping old table %q: %v", table, err)
	}
	return nil
}

func updateDDL(t *testing.T, adminClient *dbadmin.DatabaseAdminClient, statements ...string) error {
	t.Helper()
	ctx := context.Background()
	t.Logf("DDL update: %q", statements)
	op, err := adminClient.UpdateDatabaseDdl(ctx, &dbadminpb.UpdateDatabaseDdlRequest{
		Database:   dbName(),
		Statements: statements,
	})
	if err != nil {
		t.Fatalf("Starting DDL update: %v", err)
	}
	return op.Wait(ctx)
}

func mustSlurpRows(t *testing.T, ri *spanner.RowIterator) [][]interface{} {
	t.Helper()
	all, err := slurpRows(t, ri)
	if err != nil {
		t.Fatalf("Reading rows: %v", err)
	}
	return all
}

func slurpRows(t *testing.T, ri *spanner.RowIterator) (all [][]interface{}, err error) {
	t.Helper()
	err = ri.Do(func(r *spanner.Row) error {
		var data []interface{}
		for i := 0; i < r.Size(); i++ {
			var gcv spanner.GenericColumnValue
			if err := r.Column(i, &gcv); err != nil {
				return err
			}
			data = append(data, genericValue(t, gcv))
		}
		all = append(all, data)
		return nil
	})
	return
}

func genericValue(t *testing.T, gcv spanner.GenericColumnValue) interface{} {
	t.Helper()

	if _, ok := gcv.Value.Kind.(*structpb.Value_NullValue); ok {
		return nil
	}
	if gcv.Type.Code == spannerpb.TypeCode_ARRAY {
		var arr []interface{}
		for _, v := range gcv.Value.GetListValue().Values {
			arr = append(arr, genericValue(t, spanner.GenericColumnValue{
				Type:  &spannerpb.Type{Code: gcv.Type.ArrayElementType.Code},
				Value: v,
			}))
		}
		return arr
	}

	var dst interface{}
	switch gcv.Type.Code {
	case spannerpb.TypeCode_BOOL:
		dst = new(bool)
	case spannerpb.TypeCode_INT64:
		dst = new(int64)
	case spannerpb.TypeCode_FLOAT64:
		dst = new(float64)
	case spannerpb.TypeCode_TIMESTAMP:
		dst = new(time.Time) // TODO: do we need to force to UTC?
	case spannerpb.TypeCode_DATE:
		dst = new(civil.Date)
	case spannerpb.TypeCode_STRING:
		dst = new(string)
	case spannerpb.TypeCode_BYTES:
		dst = new([]byte)
	}
	if dst == nil {
		t.Fatalf("Can't decode Spanner generic column value: %v", gcv.Type)
	}
	if err := gcv.Decode(dst); err != nil {
		t.Fatalf("Decoding %v into %T: %v", gcv, dst, err)
	}
	return reflect.ValueOf(dst).Elem().Interface()
}

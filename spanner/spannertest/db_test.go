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

package spannertest

import (
	"io"
	"reflect"
	"sync"
	"testing"

	"google.golang.org/grpc/codes"

	structpb "github.com/golang/protobuf/ptypes/struct"

	"cloud.google.com/go/spanner/spansql"
)

var stdTestTable = &spansql.CreateTable{
	Name: "Staff",
	Columns: []spansql.ColumnDef{
		{Name: "Tenure", Type: spansql.Type{Base: spansql.Int64}},
		{Name: "ID", Type: spansql.Type{Base: spansql.Int64}},
		{Name: "Name", Type: spansql.Type{Base: spansql.String}},
		{Name: "Cool", Type: spansql.Type{Base: spansql.Bool}},
		{Name: "Height", Type: spansql.Type{Base: spansql.Float64}},
	},
	PrimaryKey: []spansql.KeyPart{{Column: "Name"}, {Column: "ID"}},
}

func TestTableCreation(t *testing.T) {
	var db database
	st := db.ApplyDDL(stdTestTable)
	if st.Code() != codes.OK {
		t.Fatalf("Creating table: %v", st.Err())
	}

	// Snoop inside to check that it was constructed correctly.
	got, ok := db.tables["Staff"]
	if !ok {
		t.Fatal("Table didn't get registered")
	}
	want := table{
		cols: []colInfo{
			{Name: "Name", Type: spansql.Type{Base: spansql.String}},
			{Name: "ID", Type: spansql.Type{Base: spansql.Int64}},
			{Name: "Tenure", Type: spansql.Type{Base: spansql.Int64}},
			{Name: "Cool", Type: spansql.Type{Base: spansql.Bool}},
			{Name: "Height", Type: spansql.Type{Base: spansql.Float64}},
		},
		colIndex: map[string]int{
			"Tenure": 2, "ID": 1, "Cool": 3, "Name": 0, "Height": 4,
		},
		pkCols: 2,
	}
	if !reflect.DeepEqual(got.cols, want.cols) {
		t.Errorf("table.cols incorrect.\n got %v\nwant %v", got.cols, want.cols)
	}
	if !reflect.DeepEqual(got.colIndex, want.colIndex) {
		t.Errorf("table.colIndex incorrect.\n got %v\nwant %v", got.colIndex, want.colIndex)
	}
	if got.pkCols != want.pkCols {
		t.Errorf("table.pkCols incorrect.\n got %d\nwant %d", got.pkCols, want.pkCols)
	}
}

func TestTableData(t *testing.T) {
	var db database
	st := db.ApplyDDL(stdTestTable)
	if st.Code() != codes.OK {
		t.Fatalf("Creating table: %v", st.Err())
	}

	// Insert a subset of columns.
	tx := db.NewTransaction()
	tx.Start()
	err := db.Insert(tx, "Staff", []string{"ID", "Name", "Tenure", "Height"}, []*structpb.ListValue{
		// int64 arrives as a decimal string.
		listV(stringV("1"), stringV("Jack"), stringV("10"), floatV(1.85)),
		listV(stringV("2"), stringV("Daniel"), stringV("11"), floatV(1.83)),
	})
	if err != nil {
		t.Fatalf("Inserting data: %v", err)
	}
	// Insert a different set of columns.
	err = db.Insert(tx, "Staff", []string{"Name", "ID", "Cool", "Tenure", "Height"}, []*structpb.ListValue{
		listV(stringV("Sam"), stringV("3"), boolV(false), stringV("9"), floatV(1.75)),
		listV(stringV("Teal'c"), stringV("4"), boolV(true), stringV("8"), floatV(1.91)),
		listV(stringV("George"), stringV("5"), nullV(), stringV("6"), floatV(1.73)),
		listV(stringV("Harry"), stringV("6"), boolV(true), nullV(), nullV()),
	})
	if err != nil {
		t.Fatalf("Inserting more data: %v", err)
	}
	// Delete that last one.
	err = db.Delete(tx, "Staff", []*structpb.ListValue{listV(stringV("Harry"), stringV("6"))}, nil, false)
	if err != nil {
		t.Fatalf("Deleting a row: %v", err)
	}
	// Turns out this guy isn't cool after all.
	err = db.Update(tx, "Staff", []string{"Name", "ID", "Cool"}, []*structpb.ListValue{
		// Missing columns should be left alone.
		listV(stringV("Daniel"), stringV("2"), boolV(false)),
	})
	if err != nil {
		t.Fatalf("Updating a row: %v", err)
	}
	if _, err := tx.Commit(); err != nil {
		t.Fatalf("Committing changes: %v", err)
	}

	// Read some specific keys.
	ri, err := db.Read("Staff", []string{"Name", "Tenure"}, []*structpb.ListValue{
		listV(stringV("George"), stringV("5")),
		listV(stringV("Harry"), stringV("6")), // Missing key should be silently ignored.
		listV(stringV("Sam"), stringV("3")),
		listV(stringV("George"), stringV("5")), // Duplicate key should be silently ignored.
	}, nil, 0)
	if err != nil {
		t.Fatalf("Reading keys: %v", err)
	}
	all := slurp(t, ri)
	wantAll := [][]interface{}{
		{"George", int64(6)},
		{"Sam", int64(9)},
	}
	if !reflect.DeepEqual(all, wantAll) {
		t.Errorf("Read data by keys wrong.\n got %v\nwant %v", all, wantAll)
	}
	// Read the same, but by key range.
	ri, err = db.Read("Staff", []string{"Name", "Tenure"}, nil, keyRangeList{
		{start: listV(stringV("Gabriel")), end: listV(stringV("Harpo"))}, // open/open
		{
			// closed/open
			start:       listV(stringV("Sam"), stringV("3")),
			startClosed: true,
			end: listV(stringV("Teal'c"),
				stringV("4")),
		},
	}, 0)
	if err != nil {
		t.Fatalf("Reading key ranges: %v", err)
	}
	all = slurp(t, ri)
	if !reflect.DeepEqual(all, wantAll) {
		t.Errorf("Read data by key ranges wrong.\n got %v\nwant %v", all, wantAll)
	}

	// Read a subset of all rows, with a limit.
	ri, err = db.ReadAll("Staff", []string{"Tenure", "Name", "Height"}, 4)
	if err != nil {
		t.Fatalf("ReadAll: %v", err)
	}
	wantCols := []colInfo{
		{Name: "Tenure", Type: spansql.Type{Base: spansql.Int64}},
		{Name: "Name", Type: spansql.Type{Base: spansql.String}},
		{Name: "Height", Type: spansql.Type{Base: spansql.Float64}},
	}
	if !reflect.DeepEqual(ri.Cols(), wantCols) {
		t.Errorf("ReadAll cols wrong.\n got %v\nwant %v", ri.Cols(), wantCols)
	}
	all = slurp(t, ri)
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
	st = db.ApplyDDL(&spansql.AlterTable{
		Name: "Staff",
		Alteration: spansql.AddColumn{Def: spansql.ColumnDef{
			Name: "FirstSeen",
			Type: spansql.Type{Base: spansql.Date},
		}},
	})
	if st.Code() != codes.OK {
		t.Fatalf("Adding column: %v", st.Err())
	}
	st = db.ApplyDDL(&spansql.AlterTable{
		Name: "Staff",
		Alteration: spansql.AddColumn{Def: spansql.ColumnDef{
			Name: "To", // keyword; will need quoting in queries
			Type: spansql.Type{Base: spansql.Timestamp},
		}},
	})
	if st.Code() != codes.OK {
		t.Fatalf("Adding column: %v", st.Err())
	}
	tx = db.NewTransaction()
	tx.Start()
	err = db.Update(tx, "Staff", []string{"Name", "ID", "FirstSeen", "To"}, []*structpb.ListValue{
		listV(stringV("Jack"), stringV("1"), stringV("1994-10-28"), nullV()),
		listV(stringV("Daniel"), stringV("2"), stringV("1994-10-28"), nullV()),
		listV(stringV("George"), stringV("5"), stringV("1997-07-27"), stringV("2008-07-29T11:22:43Z")),
	})
	if err != nil {
		t.Fatalf("Updating rows: %v", err)
	}
	if _, err := tx.Commit(); err != nil {
		t.Fatalf("Committing changes: %v", err)
	}

	// Add some more data, then delete it with a KeyRange.
	// The queries below ensure that this was all deleted.
	tx = db.NewTransaction()
	tx.Start()
	err = db.Insert(tx, "Staff", []string{"Name", "ID"}, []*structpb.ListValue{
		listV(stringV("01"), stringV("1")),
		listV(stringV("03"), stringV("3")),
		listV(stringV("06"), stringV("6")),
	})
	if err != nil {
		t.Fatalf("Inserting data: %v", err)
	}
	err = db.Delete(tx, "Staff", nil, keyRangeList{{
		start:       listV(stringV("01"), stringV("1")),
		startClosed: true,
		end:         listV(stringV("9")),
	}}, false)
	if err != nil {
		t.Fatalf("Deleting key range: %v", err)
	}
	if _, err := tx.Commit(); err != nil {
		t.Fatalf("Committing changes: %v", err)
	}
	// Re-add the data and delete with DML.
	err = db.Insert(tx, "Staff", []string{"Name", "ID"}, []*structpb.ListValue{
		listV(stringV("01"), stringV("1")),
		listV(stringV("03"), stringV("3")),
		listV(stringV("06"), stringV("6")),
	})
	if err != nil {
		t.Fatalf("Inserting data: %v", err)
	}
	n, err := db.Execute(&spansql.Delete{
		Table: "Staff",
		Where: spansql.LogicalOp{
			LHS: spansql.ComparisonOp{
				LHS: spansql.ID("Name"),
				Op:  spansql.Ge,
				RHS: spansql.Param("min"),
			},
			Op: spansql.And,
			RHS: spansql.ComparisonOp{
				LHS: spansql.ID("Name"),
				Op:  spansql.Lt,
				RHS: spansql.Param("max"),
			},
		},
	}, queryParams{
		"min": stringParam("01"),
		"max": stringParam("07"),
	})
	if err != nil {
		t.Fatalf("Deleting with DML: %v", err)
	}
	if n != 3 {
		t.Errorf("Deleting with DML affected %d rows, want 3", n)
	}

	// Add a BYTES column, and populate it with some data.
	st = db.ApplyDDL(&spansql.AlterTable{
		Name: "Staff",
		Alteration: spansql.AddColumn{Def: spansql.ColumnDef{
			Name: "RawBytes",
			Type: spansql.Type{Base: spansql.Bytes, Len: spansql.MaxLen},
		}},
	})
	if st.Code() != codes.OK {
		t.Fatalf("Adding column: %v", st.Err())
	}
	tx = db.NewTransaction()
	tx.Start()
	err = db.Update(tx, "Staff", []string{"Name", "ID", "RawBytes"}, []*structpb.ListValue{
		// bytes {0x01 0x00 0x01} encode as base-64 AQAB.
		listV(stringV("Jack"), stringV("1"), stringV("AQAB")),
	})
	if err != nil {
		t.Fatalf("Updating rows: %v", err)
	}
	if _, err := tx.Commit(); err != nil {
		t.Fatalf("Committing changes: %v", err)
	}

	// Prepare the sample tables from the Cloud Spanner docs.
	// https://cloud.google.com/spanner/docs/query-syntax#appendix-a-examples-with-sample-data
	for _, ct := range []*spansql.CreateTable{
		// TODO: Roster, TeamMascot when we implement JOINs.
		{
			Name: "PlayerStats",
			Columns: []spansql.ColumnDef{
				{Name: "LastName", Type: spansql.Type{Base: spansql.String}},
				{Name: "OpponentID", Type: spansql.Type{Base: spansql.Int64}},
				{Name: "PointsScored", Type: spansql.Type{Base: spansql.Int64}},
			},
			PrimaryKey: []spansql.KeyPart{{Column: "LastName"}, {Column: "OpponentID"}}, // TODO: is this right?
		},
	} {
		st := db.ApplyDDL(ct)
		if st.Code() != codes.OK {
			t.Fatalf("Creating table: %v", st.Err())
		}
	}
	tx = db.NewTransaction()
	tx.Start()
	err = db.Insert(tx, "PlayerStats", []string{"LastName", "OpponentID", "PointsScored"}, []*structpb.ListValue{
		listV(stringV("Adams"), stringV("51"), stringV("3")),
		listV(stringV("Buchanan"), stringV("77"), stringV("0")),
		listV(stringV("Coolidge"), stringV("77"), stringV("1")),
		listV(stringV("Adams"), stringV("52"), stringV("4")),
		listV(stringV("Buchanan"), stringV("50"), stringV("13")),
	})
	if err != nil {
		t.Fatalf("Inserting data: %v", err)
	}
	if _, err := tx.Commit(); err != nil {
		t.Fatalf("Commiting changes: %v", err)
	}

	// Do some complex queries.
	tests := []struct {
		q      string
		params queryParams
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
			queryParams{"x": nullParam()},
			[][]interface{}{
				{false, true, false, true, true, false},
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
			queryParams{"min": intParam(9), "lim": intParam(11), "numResults": intParam(100)},
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
				{"Daniel", true}, // Daniel has Cool==true
				{"Jack", false},  // Jack has NULL Cool
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
			queryParams{"min": floatParam(1.75), "max": floatParam(1.85)},
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
				// These are returned in table column order.
				// Note that the primary key columns get sorted first.
				{"Sam", int64(3), int64(9), false, 1.75, nil, nil, nil},
			},
		},
		{
			// Exactly the same as the previous, except with a redundant ORDER BY clause.
			`SELECT * FROM Staff WHERE Name LIKE "S%" ORDER BY Name`,
			nil,
			[][]interface{}{
				{"Sam", int64(3), int64(9), false, 1.75, nil, nil, nil},
			},
		},
		{
			`SELECT Name FROM Staff WHERE FirstSeen >= @min`,
			queryParams{"min": queryParam{Value: "1996-01-01", Type: spansql.Type{Base: spansql.Date}}},
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
			"SELECT COUNT(*) FROM Staff WHERE `To` IS NOT NULL",
			nil,
			[][]interface{}{
				{int64(1)},
			},
		},
		{
			`SELECT DISTINCT Cool, Tenure > 8 FROM Staff`,
			nil,
			[][]interface{}{
				// The non-distinct results are be
				//          [[false true] [<nil> false] [<nil> true] [false true] [true false]]
				{false, true},
				{nil, false},
				{nil, true},
				{true, false},
			},
		},
		{
			`SELECT Name FROM Staff WHERE ID IN UNNEST(@ids)`,
			queryParams{"ids": queryParam{
				Value: []interface{}{int64(3), int64(1)},
				Type:  spansql.Type{Base: spansql.Int64, Array: true},
			}},
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
			`SELECT ARRAY_AGG(Cool) FROM Staff ORDER BY Name`,
			nil,
			[][]interface{}{
				// Daniel, George (NULL), Jack (NULL), Sam, Teal'c
				{[]interface{}{false, nil, nil, false, true}},
			},
		},
	}
	for _, test := range tests {
		q, err := spansql.ParseQuery(test.q)
		if err != nil {
			t.Errorf("ParseQuery(%q): %v", test.q, err)
			continue
		}
		ri, err := db.Query(q, test.params)
		if err != nil {
			t.Errorf("Query(%q, %v): %v", test.q, test.params, err)
			continue
		}
		all := slurp(t, ri)
		if !reflect.DeepEqual(all, test.want) {
			t.Errorf("Results from Query(%q, %v) are wrong.\n got %v\nwant %v", test.q, test.params, all, test.want)
		}
	}
}

func TestTableDescendingKey(t *testing.T) {
	var descTestTable = &spansql.CreateTable{
		Name: "Timeseries",
		Columns: []spansql.ColumnDef{
			{Name: "Name", Type: spansql.Type{Base: spansql.String}},
			{Name: "Observed", Type: spansql.Type{Base: spansql.Int64}},
			{Name: "Value", Type: spansql.Type{Base: spansql.Float64}},
		},
		PrimaryKey: []spansql.KeyPart{{Column: "Name"}, {Column: "Observed", Desc: true}},
	}

	var db database
	if st := db.ApplyDDL(descTestTable); st.Code() != codes.OK {
		t.Fatalf("Creating table: %v", st.Err())
	}

	tx := db.NewTransaction()
	tx.Start()
	err := db.Insert(tx, "Timeseries", []string{"Name", "Observed", "Value"}, []*structpb.ListValue{
		listV(stringV("box"), stringV("1"), floatV(1.1)),
		listV(stringV("cupcake"), stringV("1"), floatV(6)),
		listV(stringV("box"), stringV("2"), floatV(1.2)),
		listV(stringV("cupcake"), stringV("2"), floatV(7)),
		listV(stringV("box"), stringV("3"), floatV(1.3)),
		listV(stringV("cupcake"), stringV("3"), floatV(8)),
	})
	if err != nil {
		t.Fatalf("Inserting data: %v", err)
	}
	if _, err := tx.Commit(); err != nil {
		t.Fatalf("Committing changes: %v", err)
	}

	// Querying the entire table should return values in key order,
	// noting that the second key part here is in descending order.
	q, err := spansql.ParseQuery(`SELECT * FROM Timeseries`)
	if err != nil {
		t.Fatalf("ParseQuery: %v", err)
	}
	ri, err := db.Query(q, nil)
	if err != nil {
		t.Fatalf("Query: %v", err)
	}
	got := slurp(t, ri)
	want := [][]interface{}{
		{"box", int64(3), 1.3},
		{"box", int64(2), 1.2},
		{"box", int64(1), 1.1},
		{"cupcake", int64(3), 8.0},
		{"cupcake", int64(2), 7.0},
		{"cupcake", int64(1), 6.0},
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("Results from Query are wrong.\n got %v\nwant %v", got, want)
	}

	// TestKeyRange exercises the edge cases for key range reading.
}

func TestTableSchemaConvertNull(t *testing.T) {
	var db database
	st := db.ApplyDDL(&spansql.CreateTable{
		Name: "Songwriters",
		Columns: []spansql.ColumnDef{
			{Name: "ID", Type: spansql.Type{Base: spansql.Int64}, NotNull: true},
			{Name: "Nickname", Type: spansql.Type{Base: spansql.String}},
		},
		PrimaryKey: []spansql.KeyPart{{Column: "ID"}},
	})
	if err := st.Err(); err != nil {
		t.Fatal(err)
	}

	// Populate with data including a NULL for the STRING field.
	tx := db.NewTransaction()
	tx.Start()
	err := db.Insert(tx, "Songwriters", []string{"ID", "Nickname"}, []*structpb.ListValue{
		listV(stringV("6"), stringV("Tiger")),
		listV(stringV("7"), nullV()),
	})
	if err != nil {
		t.Fatalf("Inserting data: %v", err)
	}
	if _, err := tx.Commit(); err != nil {
		t.Fatalf("Committing changes: %v", err)
	}

	// Convert the STRING field to a BYTES and back.
	st = db.ApplyDDL(&spansql.AlterTable{
		Name: "Songwriters",
		Alteration: spansql.AlterColumn{
			Def: spansql.ColumnDef{Name: "Nickname", Type: spansql.Type{Base: spansql.Bytes}},
		},
	})
	if err := st.Err(); err != nil {
		t.Fatalf("Converting STRING -> BYTES: %v", err)
	}
	st = db.ApplyDDL(&spansql.AlterTable{
		Name: "Songwriters",
		Alteration: spansql.AlterColumn{
			Def: spansql.ColumnDef{Name: "Nickname", Type: spansql.Type{Base: spansql.String}},
		},
	})
	if err := st.Err(); err != nil {
		t.Fatalf("Converting BYTES -> STRING: %v", err)
	}

	// Check that the data is maintained.
	q, err := spansql.ParseQuery(`SELECT * FROM Songwriters`)
	if err != nil {
		t.Fatalf("ParseQuery: %v", err)
	}
	ri, err := db.Query(q, nil)
	if err != nil {
		t.Fatalf("Query: %v", err)
	}
	got := slurp(t, ri)
	want := [][]interface{}{
		{int64(6), "Tiger"},
		{int64(7), nil},
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("Results from Query are wrong.\n got %v\nwant %v", got, want)
	}
}

func TestTableSchemaUpdates(t *testing.T) {
	tests := []struct {
		desc     string
		ddl      string
		wantCode codes.Code
	}{
		// TODO: add more cases, including interactions with the primary key and dropping columns.

		{
			"Add new column",
			`CREATE TABLE Songwriters (
				Id INT64 NOT NULL,
			) PRIMARY KEY (Id);
			ALTER TABLE Songwriters ADD COLUMN Nickname STRING(MAX);`,
			codes.OK,
		},
		{
			"Add new column with NOT NULL",
			`CREATE TABLE Songwriters (
				Id INT64 NOT NULL,
			) PRIMARY KEY (Id);
			ALTER TABLE Songwriters ADD COLUMN Nickname STRING(MAX) NOT NULL;`,
			codes.InvalidArgument,
		},

		// Examples from https://cloud.google.com/spanner/docs/schema-updates:

		{
			"Add NOT NULL to a non-key column",
			`CREATE TABLE Songwriters (
				Id INT64 NOT NULL,
				Nickname STRING(MAX),
			) PRIMARY KEY (Id);
			ALTER TABLE Songwriters ALTER COLUMN Nickname STRING(MAX) NOT NULL;`,
			codes.OK,
		},
		{
			"Remove NOT NULL from a non-key column",
			`CREATE TABLE Songwriters (
				Id INT64 NOT NULL,
				Nickname STRING(MAX) NOT NULL,
			) PRIMARY KEY (Id);
			ALTER TABLE Songwriters ALTER COLUMN Nickname STRING(MAX);`,
			codes.OK,
		},
		{
			"Change a STRING column to a BYTES column",
			`CREATE TABLE Songwriters (
				Id INT64 NOT NULL,
				Nickname STRING(MAX),
			) PRIMARY KEY (Id);
			ALTER TABLE Songwriters ALTER COLUMN Nickname BYTES(MAX);`,
			codes.OK,
		},
		// TODO: Increase or decrease the length limit for a STRING or BYTES type (including to MAX)
		// TODO: Enable or disable commit timestamps in value and primary key columns
	}
testLoop:
	for _, test := range tests {
		var db database

		ddl, err := spansql.ParseDDL("filename", test.ddl)
		if err != nil {
			t.Fatalf("%s: Bad DDL: %v", test.desc, err)
		}
		for _, stmt := range ddl.List {
			if st := db.ApplyDDL(stmt); st.Code() != codes.OK {
				if st.Code() != test.wantCode {
					t.Errorf("%s: Applying statement %q: %v", test.desc, stmt.SQL(), st.Err())
				}
				continue testLoop
			}
		}
		if test.wantCode != codes.OK {
			t.Errorf("%s: Finished with OK, want %v", test.desc, test.wantCode)
		}
	}
}

func TestConcurrentReadInsert(t *testing.T) {
	// Check that data is safely copied during a query.
	tbl := &spansql.CreateTable{
		Name: "Tablino",
		Columns: []spansql.ColumnDef{
			{Name: "A", Type: spansql.Type{Base: spansql.Int64}},
		},
		PrimaryKey: []spansql.KeyPart{{Column: "A"}},
	}

	var db database
	if st := db.ApplyDDL(tbl); st.Code() != codes.OK {
		t.Fatalf("Creating table: %v", st.Err())
	}

	// Insert some initial data.
	tx := db.NewTransaction()
	tx.Start()
	err := db.Insert(tx, "Tablino", []string{"A"}, []*structpb.ListValue{
		listV(stringV("1")),
		listV(stringV("2")),
		listV(stringV("4")),
	})
	if err != nil {
		t.Fatalf("Inserting data: %v", err)
	}
	if _, err := tx.Commit(); err != nil {
		t.Fatalf("Committing changes: %v", err)
	}

	// Now insert "3", and query concurrently.
	q, err := spansql.ParseQuery(`SELECT * FROM Tablino WHERE A > 2`)
	if err != nil {
		t.Fatalf("ParseQuery: %v", err)
	}
	var out [][]interface{}

	var wg sync.WaitGroup
	wg.Add(2)
	go func() {
		defer wg.Done()

		ri, err := db.Query(q, nil)
		if err != nil {
			t.Errorf("Query: %v", err)
			return
		}
		out = slurp(t, ri)
	}()
	go func() {
		defer wg.Done()

		tx := db.NewTransaction()
		tx.Start()
		err := db.Insert(tx, "Tablino", []string{"A"}, []*structpb.ListValue{
			listV(stringV("3")),
		})
		if err != nil {
			t.Errorf("Inserting data: %v", err)
			return
		}
		if _, err := tx.Commit(); err != nil {
			t.Errorf("Committing changes: %v", err)
		}
	}()
	wg.Wait()

	// We should get either 1 or 2 rows (value 4 should be included, and value 3 might).
	if n := len(out); n != 1 && n != 2 {
		t.Fatalf("Concurrent read returned %d rows, want 1 or 2", n)
	}
}

func slurp(t *testing.T, ri rowIter) (all [][]interface{}) {
	t.Helper()
	for {
		row, err := ri.Next()
		if err == io.EOF {
			return
		} else if err != nil {
			t.Fatalf("Reading rows: %v", err)
		}
		all = append(all, row)
	}
}

func listV(vs ...*structpb.Value) *structpb.ListValue { return &structpb.ListValue{Values: vs} }
func stringV(s string) *structpb.Value                { return &structpb.Value{Kind: &structpb.Value_StringValue{s}} }
func floatV(f float64) *structpb.Value                { return &structpb.Value{Kind: &structpb.Value_NumberValue{f}} }
func boolV(b bool) *structpb.Value                    { return &structpb.Value{Kind: &structpb.Value_BoolValue{b}} }
func nullV() *structpb.Value                          { return &structpb.Value{Kind: &structpb.Value_NullValue{}} }

func stringParam(s string) queryParam { return queryParam{Value: s, Type: stringType} }
func intParam(i int64) queryParam     { return queryParam{Value: i, Type: int64Type} }
func floatParam(f float64) queryParam { return queryParam{Value: f, Type: float64Type} }
func nullParam() queryParam           { return queryParam{Value: nil} }

func TestRowCmp(t *testing.T) {
	r := func(x ...interface{}) []interface{} { return x }
	tests := []struct {
		a, b []interface{}
		desc []bool
		want int
	}{
		{r(int64(1), "foo", 1.6), r(int64(1), "foo", 1.6), []bool{false, false, false}, 0},
		{r(int64(1), "foo"), r(int64(1), "foo", 1.6), []bool{false, false, false}, 0}, // first is shorter

		{r(int64(1), "bar", 1.8), r(int64(1), "foo", 1.6), []bool{false, false, false}, -1},
		{r(int64(1), "bar", 1.8), r(int64(1), "foo", 1.6), []bool{false, false, true}, -1},
		{r(int64(1), "bar", 1.8), r(int64(1), "foo", 1.6), []bool{false, true, false}, 1},

		{r(int64(1), "foo", 1.6), r(int64(1), "bar", 1.8), []bool{false, false, false}, 1},
		{r(int64(1), "foo", 1.6), r(int64(1), "bar", 1.8), []bool{false, false, true}, 1},
		{r(int64(1), "foo", 1.6), r(int64(1), "bar", 1.8), []bool{false, true, false}, -1},
		{r(int64(1), "foo", 1.6), r(int64(1), "bar", 1.8), []bool{false, true, true}, -1},
	}
	for _, test := range tests {
		if got := rowCmp(test.a, test.b, test.desc); got != test.want {
			t.Errorf("rowCmp(%v, %v, %v) = %d, want %d", test.a, test.b, test.desc, got, test.want)
		}
	}
}

func TestKeyRange(t *testing.T) {
	r := func(x ...interface{}) []interface{} { return x }
	closedClosed := func(start, end []interface{}) *keyRange {
		return &keyRange{
			startKey:    start,
			endKey:      end,
			startClosed: true,
			endClosed:   true,
		}
	}
	halfOpen := func(start, end []interface{}) *keyRange {
		return &keyRange{
			startKey:    start,
			endKey:      end,
			startClosed: true,
		}
	}
	openOpen := func(start, end []interface{}) *keyRange {
		return &keyRange{
			startKey: start,
			endKey:   end,
		}
	}
	tests := []struct {
		kr      *keyRange
		desc    []bool
		include [][]interface{}
		exclude [][]interface{}
	}{
		// Examples from google/spanner/v1/keys.proto.
		{
			kr: closedClosed(r("Bob", "2015-01-01"), r("Bob", "2015-12-31")),
			include: [][]interface{}{
				r("Bob", "2015-01-01"),
				r("Bob", "2015-07-07"),
				r("Bob", "2015-12-31"),
			},
			exclude: [][]interface{}{
				r("Alice", "2015-07-07"),
				r("Bob", "2014-12-31"),
				r("Bob", "2016-01-01"),
			},
		},
		{
			kr: closedClosed(r("Bob", "2000-01-01"), r("Bob")),
			include: [][]interface{}{
				r("Bob", "2000-01-01"),
				r("Bob", "2022-07-07"),
			},
			exclude: [][]interface{}{
				r("Alice", "2015-07-07"),
				r("Bob", "1999-11-07"),
			},
		},
		{
			kr: closedClosed(r("Bob"), r("Bob")),
			include: [][]interface{}{
				r("Bob", "2000-01-01"),
			},
			exclude: [][]interface{}{
				r("Alice", "2015-07-07"),
				r("Charlie", "1999-11-07"),
			},
		},
		{
			kr: halfOpen(r("Bob"), r("Bob", "2000-01-01")),
			include: [][]interface{}{
				r("Bob", "1999-11-07"),
			},
			exclude: [][]interface{}{
				r("Alice", "1999-11-07"),
				r("Bob", "2000-01-01"),
				r("Bob", "2004-07-07"),
				r("Charlie", "1999-11-07"),
			},
		},
		{
			kr: openOpen(r("Bob", "1999-11-06"), r("Bob", "2000-01-01")),
			include: [][]interface{}{
				r("Bob", "1999-11-07"),
			},
			exclude: [][]interface{}{
				r("Alice", "1999-11-07"),
				r("Bob", "1999-11-06"),
				r("Bob", "2000-01-01"),
				r("Bob", "2004-07-07"),
				r("Charlie", "1999-11-07"),
			},
		},
		{
			kr: closedClosed(r(), r()),
			include: [][]interface{}{
				r("Alice", "1999-11-07"),
				r("Bob", "1999-11-07"),
				r("Charlie", "1999-11-07"),
			},
		},
		{
			kr: halfOpen(r("A"), r("D")),
			include: [][]interface{}{
				r("Alice", "1999-11-07"),
				r("Bob", "1999-11-07"),
				r("Charlie", "1999-11-07"),
			},
			exclude: [][]interface{}{
				r("0day", "1999-11-07"),
				r("Doris", "1999-11-07"),
			},
		},
		// Exercise descending primary key ordering.
		{
			kr:   halfOpen(r("Alpha"), r("Charlie")),
			desc: []bool{true, false},
			// Key range is backwards, so nothing should be returned.
			exclude: [][]interface{}{
				r("Alice", "1999-11-07"),
				r("Bob", "1999-11-07"),
				r("Doris", "1999-11-07"),
			},
		},
		{
			kr:   halfOpen(r("Alice", "1999-11-07"), r("Charlie")),
			desc: []bool{false, true},
			// The second primary key column is descending.
			include: [][]interface{}{
				r("Alice", "1999-09-09"),
				r("Alice", "1999-11-07"),
				r("Bob", "2000-01-01"),
			},
			exclude: [][]interface{}{
				r("Alice", "2000-01-01"),
				r("Doris", "1999-11-07"),
			},
		},
	}
	for _, test := range tests {
		desc := test.desc
		if desc == nil {
			desc = []bool{false, false} // default
		}
		tbl := &table{
			pkCols: 2,
			pkDesc: desc,
		}
		for _, pk := range append(test.include, test.exclude...) {
			rowNum, _ := tbl.rowForPK(pk)
			tbl.insertRow(rowNum, pk)
		}
		start, end := tbl.findRange(test.kr)
		has := func(pk []interface{}) bool {
			n, _ := tbl.rowForPK(pk)
			return start <= n && n < end
		}
		for _, pk := range test.include {
			if !has(pk) {
				t.Errorf("keyRange %v does not include %v", test.kr, pk)
			}
		}
		for _, pk := range test.exclude {
			if has(pk) {
				t.Errorf("keyRange %v includes %v", test.kr, pk)
			}
		}
	}
}

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

// TODO: More of this test should be moved into integration_test.go.

import (
	"fmt"
	"io"
	"reflect"
	"strings"
	"sync"
	"testing"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	structpb "google.golang.org/protobuf/types/known/structpb"

	"cloud.google.com/go/civil"
	"cloud.google.com/go/spanner/spansql"
)

func TestTableCreation(t *testing.T) {
	stdTestTable := &spansql.CreateTable{
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
		colIndex: map[spansql.ID]int{
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
	err := db.Insert(tx, "Timeseries", []spansql.ID{"Name", "Observed", "Value"}, []*structpb.ListValue{
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
	err := db.Insert(tx, "Songwriters", []spansql.ID{"ID", "Nickname"}, []*structpb.ListValue{
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
			Name:       "Nickname",
			Alteration: spansql.SetColumnType{Type: spansql.Type{Base: spansql.Bytes}},
		},
	})
	if err := st.Err(); err != nil {
		t.Fatalf("Converting STRING -> BYTES: %v", err)
	}
	st = db.ApplyDDL(&spansql.AlterTable{
		Name: "Songwriters",
		Alteration: spansql.AlterColumn{
			Name:       "Nickname",
			Alteration: spansql.SetColumnType{Type: spansql.Type{Base: spansql.String}},
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
	err := db.Insert(tx, "Tablino", []spansql.ID{"A"}, []*structpb.ListValue{
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
		err := db.Insert(tx, "Tablino", []spansql.ID{"A"}, []*structpb.ListValue{
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

func TestGeneratedColumn(t *testing.T) {
	sql := `CREATE TABLE Songwriters (
		Id INT64 NOT NULL,
		Name STRING(20),
		Age INT64,
		Over18 BOOL AS (Age > 18) STORED,
	) PRIMARY KEY (Id);`
	var db database

	ddl, err := spansql.ParseDDL("filename", sql)
	if err != nil {
		t.Fatalf("%s: Bad DDL", err)
	}
	for _, stmt := range ddl.List {
		if st := db.ApplyDDL(stmt); st.Code() != codes.OK {
			t.Fatalf("ApplyDDL failed: %v", st)
		}
	}

	addColSQL := `ALTER TABLE Songwriters ADD COLUMN CanonicalName STRING(20) AS (LOWER(Name)) STORED;`
	ddl, err = spansql.ParseDDL("filename", addColSQL)
	if err != nil {
		t.Fatalf("%s: Bad DDL", err)
	}
	if st := db.ApplyDDL(ddl.List[0]); st.Code() != codes.OK {
		t.Fatalf("Should have been able to add a generated column to empty table\n status: %v", st)
	}

	tx := db.NewTransaction()
	err = db.Insert(tx, "Songwriters",
		[]spansql.ID{"Id", "Over18"},
		[]*structpb.ListValue{
			listV(stringV("3"), boolV(true)),
		})
	if err == nil || status.Code(err) != codes.InvalidArgument {
		t.Fatal("Should have failed to insert to generated column")
	}

	err = db.Insert(tx, "Songwriters",
		[]spansql.ID{"Id"},
		[]*structpb.ListValue{
			listV(stringV("1")),
		})
	if err != nil {
		t.Fatalf("Should have succeeded to insert to with no dependent columns: %v", err)
	}

	name := "Famous Writer"
	err = db.Insert(tx, "Songwriters",
		[]spansql.ID{"Id", "Name", "Age"},
		[]*structpb.ListValue{
			listV(stringV("3"), stringV(name), stringV("40")),
		})
	if err != nil {
		t.Fatalf("Should have succeeded to insert to without generated column: %v", err)
	}

	var kr keyRangeList
	iter, err := db.Read("Songwriters", []spansql.ID{"Id", "CanonicalName", "Over18"},
		[]*structpb.ListValue{
			listV(stringV("3")),
		}, kr, 0)
	if err != nil {
		t.Fatalf("failed to read: %v", err)
	}
	rows := slurp(t, iter)
	if rows[0][1].(string) != strings.ToLower(name) {
		t.Fatalf("Generated value for CanonicalName mismatch\n Got: %v\n Want: %v", rows[0][1].(string), strings.ToLower(name))
	}
	if !rows[0][2].(bool) {
		t.Fatalf("Generated value for Over18 mismatch\n Got: %v\n Want: true", rows[0][2].(bool))
	}

	addColSQL = `ALTER TABLE Songwriters ADD COLUMN Under18 BOOL AS (Age < 18) STORED;`
	ddl, err = spansql.ParseDDL("filename", addColSQL)
	if err != nil {
		t.Fatalf("%s: Bad DDL", err)
	}
	if st := db.ApplyDDL(ddl.List[0]); st.Code() != codes.OK {
		t.Fatalf("Failed to add a generated column to non-empty table\n status: %v", st)
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
func stringV(s string) *structpb.Value {
	return &structpb.Value{Kind: &structpb.Value_StringValue{StringValue: s}}
}
func floatV(f float64) *structpb.Value {
	return &structpb.Value{Kind: &structpb.Value_NumberValue{NumberValue: f}}
}
func boolV(b bool) *structpb.Value {
	return &structpb.Value{Kind: &structpb.Value_BoolValue{BoolValue: b}}
}
func nullV() *structpb.Value { return &structpb.Value{Kind: &structpb.Value_NullValue{}} }

func boolParam(b bool) queryParam     { return queryParam{Value: b, Type: boolType} }
func stringParam(s string) queryParam { return queryParam{Value: s, Type: stringType} }
func intParam(i int64) queryParam     { return queryParam{Value: i, Type: int64Type} }
func floatParam(f float64) queryParam { return queryParam{Value: f, Type: float64Type} }
func nullParam() queryParam           { return queryParam{Value: nil} }

func dateParam(s string) queryParam {
	d, err := civil.ParseDate(s)
	if err != nil {
		panic(fmt.Sprintf("bad test date %q: %v", s, err))
	}
	return queryParam{Value: d, Type: spansql.Type{Base: spansql.Date}}
}

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

func TestForeignKeyAddAndAlterConstraint(t *testing.T) {
	sql := `CREATE TABLE Orders (
	  OrderID INT64 NOT NULL,
	  CustomerID INT64 NOT NULL,
	  Quantity INT64 NOT NULL,
	  ProductID INT64 NOT NULL,
	  CONSTRAINT FK_CustomerOrder FOREIGN KEY (CustomerID) REFERENCES Customers (CustomerID)
	) PRIMARY KEY (OrderID);
	ALTER TABLE Orders DROP CONSTRAINT FK_CustomerOrder;`
	var db database

	ddl, err := spansql.ParseDDL("filename", sql)
	if err != nil {
		t.Fatalf("%s: Bad DDL", err)
	}
	for _, stmt := range ddl.List {
		if st := db.ApplyDDL(stmt); st.Code() != codes.OK {
			t.Fatalf("ApplyDDL failed: %v", st)
		}
	}

	altersql := `CREATE TABLE Orders (
	  OrderID INT64 NOT NULL,
	  CustomerID INT64 NOT NULL,
	  Quantity INT64 NOT NULL,
	  ProductID INT64 NOT NULL,
		CONSTRAINT FK_ProductOrder FOREIGN KEY (ProductID) REFERENCES Product (ProductID)
	) PRIMARY KEY (OrderID);
	ALTER TABLE Orders ADD CONSTRAINT FK_CustomerOrder FOREIGN KEY (CustomerID) REFERENCES Customers (CustomerID);
	ALTER TABLE Orders DROP CONSTRAINT FK_ProductOrder;`
	var db1 database

	ddl1, err1 := spansql.ParseDDL("filename", altersql)
	if err1 != nil {
		t.Fatalf("%s: Bad DDL", err1)
	}
	for _, stmt := range ddl1.List {
		if st := db1.ApplyDDL(stmt); st.Code() != codes.OK {
			t.Fatalf("ApplyDDL failed: %v", st)
		}
	}
}

func TestAddBackQuoteForHypen(t *testing.T) {
	ddl, err := spansql.ParseDDL("filename", "ALTER DATABASE `test-db` SET OPTIONS (optimizer_version=4, version_retention_period = '7d', enable_key_visualizer=true)")
	if err != nil {
		t.Fatalf("%s: Bad DDL", err)
	}

	got := ddl.List[0].SQL()
	want := "ALTER DATABASE `test-db` SET OPTIONS (optimizer_version=4, version_retention_period='7d', enable_key_visualizer=true)"

	if !reflect.DeepEqual(got, want) {
		t.Errorf("Generated SQL statement incorrect.\n got %v\nwant %v", got, want)
	}
}

func TestCreateAndManageChangeStream(t *testing.T) {
	// Testing Create Change Stream
	ddl, err := spansql.ParseDDL("filename", "CREATE CHANGE STREAM SingerAlbumStream FOR Singers(FirstName, LastName), Albums OPTIONS (retention_period = '36h')")
	if err != nil {
		t.Fatalf("%s: Bad DDL", err)
	}

	got := ddl.List[0].SQL()
	want := "CREATE CHANGE STREAM SingerAlbumStream FOR Singers(FirstName, LastName), Albums OPTIONS (retention_period='36h')"

	if !reflect.DeepEqual(got, want) {
		t.Errorf("Generated SQL statement incorrect.\n got %v\nwant %v", got, want)
	}

	// Testing Alter Change Stream Options
	ddl, err = spansql.ParseDDL("filename", "ALTER CHANGE STREAM SingerAlbumStream SET OPTIONS (retention_period = '20h')")
	if err != nil {
		t.Fatalf("%s: Bad DDL", err)
	}

	got = ddl.List[0].SQL()
	want = "ALTER CHANGE STREAM SingerAlbumStream SET OPTIONS (retention_period='20h')"

	if !reflect.DeepEqual(got, want) {
		t.Errorf("Generated SQL statement incorrect.\n got %v\nwant %v", got, want)
	}

	// Testing Drop Change Stream Options
	ddl, err = spansql.ParseDDL("filename", "DROP CHANGE STREAM SingerAlbumStream")
	if err != nil {
		t.Fatalf("%s: Bad DDL", err)
	}

	got = ddl.List[0].SQL()
	want = "DROP CHANGE STREAM SingerAlbumStream"

	if !reflect.DeepEqual(got, want) {
		t.Errorf("Generated SQL statement incorrect.\n got %v\nwant %v", got, want)
	}
}

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

package bigtable

import (
	"context"
	"fmt"
	"reflect"
	"strings"
	"testing"

	"cloud.google.com/go/bigtable/bttest"
	"cloud.google.com/go/internal/testutil"
	"google.golang.org/api/option"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

func setupReadTest(t *testing.T) (context.Context, *Client, *AdminClient, *Table, string, func()) {
	srv, err := bttest.NewServer("localhost:0")
	if err != nil {
		t.Fatal(err)
	}

	ctx := context.Background()

	conn, err := grpc.NewClient(srv.Addr, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		t.Fatal(err)
	}

	client, err := NewClient(ctx, "project", "instance", option.WithGRPCConn(conn))
	if err != nil {
		t.Fatal(err)
	}

	adminClient, err := NewAdminClient(ctx, "project", "instance", option.WithGRPCConn(conn))
	if err != nil {
		t.Fatal(err)
	}

	tableName := "test-table"
	if err := adminClient.CreateTable(ctx, tableName); err != nil {
		t.Fatal(err)
	}

	table := client.Open(tableName)

	cleanup := func() {
		client.Close()
		adminClient.Close()
		srv.Close()
	}

	return ctx, client, adminClient, table, tableName, cleanup
}

func TestRead(t *testing.T) {
	ctx, _, adminClient, table, tableName, cleanup := setupReadTest(t)
	defer cleanup()

	// Insert some data.
	initialData := map[string][]string{
		"wmckinley":   {"tjefferson"},
		"gwashington": {"j§adams"},
		"tjefferson":  {"gwashington", "j§adams", "wmckinley"},
		"j§adams":     {"gwashington", "tjefferson"},
	}

	if err := adminClient.CreateColumnFamily(ctx, tableName, "follows"); err != nil {
		t.Fatalf("Creating column family: %v", err)
	}

	for row, ss := range initialData {
		mut := NewMutation()
		for _, name := range ss {
			mut.Set("follows", name, 1000, []byte("1"))
		}
		if err := table.Apply(ctx, row, mut); err != nil {
			t.Fatalf("Mutating row %q: %v", row, err)
		}
	}

	for _, test := range []struct {
		desc   string
		rr     RowSet
		filter Filter     // may be nil
		limit  ReadOption // may be nil

		// We do the read, grab all the cells, turn them into "<row>-<col>-<val>",
		// and join with a comma.
		want       string
		wantLabels []string
	}{
		{
			desc: "read all, unfiltered",
			rr:   RowRange{},
			want: "gwashington-j§adams-1,j§adams-gwashington-1,j§adams-tjefferson-1,tjefferson-gwashington-1,tjefferson-j§adams-1,tjefferson-wmckinley-1,wmckinley-tjefferson-1",
		},
		{
			desc: "read with InfiniteRange, unfiltered",
			rr:   InfiniteRange("tjefferson"),
			want: "tjefferson-gwashington-1,tjefferson-j§adams-1,tjefferson-wmckinley-1,wmckinley-tjefferson-1",
		},
		{
			desc: "read with NewRange, unfiltered",
			rr:   NewRange("gargamel", "hubbard"),
			want: "gwashington-j§adams-1",
		},
		{
			desc: "read with PrefixRange, unfiltered",
			rr:   PrefixRange("j§ad"),
			want: "j§adams-gwashington-1,j§adams-tjefferson-1",
		},
		{
			desc: "read with SingleRow, unfiltered",
			rr:   SingleRow("wmckinley"),
			want: "wmckinley-tjefferson-1",
		},
		{
			desc:   "read all, with ColumnFilter",
			rr:     RowRange{},
			filter: ColumnFilter(".*j.*"), // matches "j§adams" and "tjefferson"
			want:   "gwashington-j§adams-1,j§adams-tjefferson-1,tjefferson-j§adams-1,wmckinley-tjefferson-1",
		},
		{
			desc:   "read all, with ColumnFilter, prefix",
			rr:     RowRange{},
			filter: ColumnFilter("j"), // no matches
			want:   "",
		},
		{
			desc:   "read range, with ColumnRangeFilter",
			rr:     RowRange{},
			filter: ColumnRangeFilter("follows", "h", "k"),
			want:   "gwashington-j§adams-1,tjefferson-j§adams-1",
		},
		{
			desc:   "read range from empty, with ColumnRangeFilter",
			rr:     RowRange{},
			filter: ColumnRangeFilter("follows", "", "u"),
			want:   "gwashington-j§adams-1,j§adams-gwashington-1,j§adams-tjefferson-1,tjefferson-gwashington-1,tjefferson-j§adams-1,wmckinley-tjefferson-1",
		},
		{
			desc:   "read range from start to empty, with ColumnRangeFilter",
			rr:     RowRange{},
			filter: ColumnRangeFilter("follows", "h", ""),
			want:   "gwashington-j§adams-1,j§adams-tjefferson-1,tjefferson-j§adams-1,tjefferson-wmckinley-1,wmckinley-tjefferson-1",
		},
		{
			desc:   "read with RowKeyFilter",
			rr:     RowRange{},
			filter: RowKeyFilter(".*wash.*"),
			want:   "gwashington-j§adams-1",
		},
		{
			desc:   "read with RowKeyFilter unicode",
			rr:     RowRange{},
			filter: RowKeyFilter(".*j§.*"),
			want:   "j§adams-gwashington-1,j§adams-tjefferson-1",
		},
		{
			desc:   "read with RowKeyFilter escaped",
			rr:     RowRange{},
			filter: RowKeyFilter(`.*j\xC2\xA7.*`),
			want:   "j§adams-gwashington-1,j§adams-tjefferson-1",
		},
		{
			desc:   "read with RowKeyFilter, prefix",
			rr:     RowRange{},
			filter: RowKeyFilter("gwash"),
			want:   "",
		},
		{
			desc:   "read with RowKeyFilter, no matches",
			rr:     RowRange{},
			filter: RowKeyFilter(".*xxx.*"),
			want:   "",
		},
		{
			desc:   "read with FamilyFilter, no matches",
			rr:     RowRange{},
			filter: FamilyFilter(".*xxx.*"),
			want:   "",
		},
		{
			desc:   "read with ColumnFilter + row end",
			rr:     RowRange{},
			filter: ColumnFilter(".*j.*"), // matches "j§adams" and "tjefferson"
			limit:  LimitRows(2),
			want:   "gwashington-j§adams-1,j§adams-tjefferson-1",
		},
		{
			desc:       "apply labels to the result rows",
			rr:         RowRange{},
			filter:     LabelFilter("test-label"),
			limit:      LimitRows(2),
			want:       "gwashington-j§adams-1,j§adams-gwashington-1,j§adams-tjefferson-1",
			wantLabels: []string{"test-label", "test-label", "test-label"},
		},
		{
			desc:   "read all, strip values",
			rr:     RowRange{},
			filter: StripValueFilter(),
			want:   "gwashington-j§adams-,j§adams-gwashington-,j§adams-tjefferson-,tjefferson-gwashington-,tjefferson-j§adams-,tjefferson-wmckinley-,wmckinley-tjefferson-",
		},
		{
			desc:   "read with ColumnFilter + row end + strip values",
			rr:     RowRange{},
			filter: ChainFilters(ColumnFilter(".*j.*"), StripValueFilter()), // matches "j§adams" and "tjefferson"
			limit:  LimitRows(2),
			want:   "gwashington-j§adams-,j§adams-tjefferson-",
		},
		{
			desc:   "read with condition, strip values on true",
			rr:     RowRange{},
			filter: ConditionFilter(ColumnFilter(".*j.*"), StripValueFilter(), nil),
			want:   "gwashington-j§adams-,j§adams-gwashington-,j§adams-tjefferson-,tjefferson-gwashington-,tjefferson-j§adams-,tjefferson-wmckinley-,wmckinley-tjefferson-",
		},
		{
			desc:   "read with condition, strip values on false",
			rr:     RowRange{},
			filter: ConditionFilter(ColumnFilter(".*xxx.*"), nil, StripValueFilter()),
			want:   "gwashington-j§adams-,j§adams-gwashington-,j§adams-tjefferson-,tjefferson-gwashington-,tjefferson-j§adams-,tjefferson-wmckinley-,wmckinley-tjefferson-",
		},
		{
			desc:   "read with ValueRangeFilter + row end",
			rr:     RowRange{},
			filter: ValueRangeFilter([]byte("1"), []byte("5")), // matches our value of "1"
			limit:  LimitRows(2),
			want:   "gwashington-j§adams-1,j§adams-gwashington-1,j§adams-tjefferson-1",
		},
		{
			desc:   "read with ValueRangeFilter, no match on exclusive end",
			rr:     RowRange{},
			filter: ValueRangeFilter([]byte("0"), []byte("1")), // no match
			want:   "",
		},
		{
			desc:   "read with ValueRangeFilter, no matches",
			rr:     RowRange{},
			filter: ValueRangeFilter([]byte("3"), []byte("5")), // matches nothing
			want:   "",
		},
		{
			desc:   "read with InterleaveFilter, no matches on all filters",
			rr:     RowRange{},
			filter: InterleaveFilters(ColumnFilter(".*x.*"), ColumnFilter(".*z.*")),
			want:   "",
		},
		{
			desc:   "read with InterleaveFilter, no duplicate cells",
			rr:     RowRange{},
			filter: InterleaveFilters(ColumnFilter(".*g.*"), ColumnFilter(".*j.*")),
			want:   "gwashington-j§adams-1,j§adams-gwashington-1,j§adams-tjefferson-1,tjefferson-gwashington-1,tjefferson-j§adams-1,wmckinley-tjefferson-1",
		},
		{
			desc:   "read with InterleaveFilter, with duplicate cells",
			rr:     RowRange{},
			filter: InterleaveFilters(ColumnFilter(".*g.*"), ColumnFilter(".*g.*")),
			want:   "j§adams-gwashington-1,j§adams-gwashington-1,tjefferson-gwashington-1,tjefferson-gwashington-1",
		},
		{
			desc: "read with a RowRangeList and no filter",
			rr:   RowRangeList{NewRange("gargamel", "hubbard"), InfiniteRange("wmckinley")},
			want: "gwashington-j§adams-1,wmckinley-tjefferson-1",
		},
		{
			desc:   "chain that excludes rows and matches nothing, in a condition",
			rr:     RowRange{},
			filter: ConditionFilter(ChainFilters(ColumnFilter(".*j.*"), ColumnFilter(".*mckinley.*")), StripValueFilter(), nil),
			want:   "",
		},
		{
			desc:   "chain that ends with an interleave that has no match. covers #804",
			rr:     RowRange{},
			filter: ConditionFilter(ChainFilters(ColumnFilter(".*j.*"), InterleaveFilters(ColumnFilter(".*x.*"), ColumnFilter(".*z.*"))), StripValueFilter(), nil),
			want:   "",
		},
	} {
		t.Run(test.desc, func(t *testing.T) {
			var opts []ReadOption
			if test.filter != nil {
				opts = append(opts, RowFilter(test.filter))
			}
			if test.limit != nil {
				opts = append(opts, test.limit)
			}
			var elt, labels []string
			err := table.ReadRows(ctx, test.rr, func(r Row) bool {
				for _, ris := range r {
					for _, ri := range ris {
						labels = append(labels, ri.Labels...)
						elt = append(elt, formatReadItem(ri))
					}
				}
				return true
			}, opts...)
			if err != nil {
				t.Fatal(err)
			}
			if got := strings.Join(elt, ","); got != test.want {
				t.Fatalf("got %q\nwant %q", got, test.want)
			}
			if got, want := labels, test.wantLabels; !reflect.DeepEqual(got, want) {
				t.Fatalf("got %q\nwant %q", got, want)
			}
		})
	}
}

func TestArbitraryTimestamps(t *testing.T) {
	ctx, _, adminClient, table, tableName, cleanup := setupReadTest(t)
	defer cleanup()

	// Test arbitrary timestamps more thoroughly.
	if err := adminClient.CreateColumnFamily(ctx, tableName, "ts"); err != nil {
		t.Fatalf("Creating column family: %v", err)
	}
	const numVersions = 4
	mut := NewMutation()
	for i := 1; i < numVersions; i++ {
		// Timestamps are used in thousands because the server
		// only permits that granularity.
		mut.Set("ts", "col", Timestamp(i*1000), []byte(fmt.Sprintf("val-%d", i)))
		mut.Set("ts", "col2", Timestamp(i*1000), []byte(fmt.Sprintf("val-%d", i)))
	}
	if err := table.Apply(ctx, "testrow", mut); err != nil {
		t.Fatalf("Mutating row: %v", err)
	}
	r, err := table.ReadRow(ctx, "testrow")
	if err != nil {
		t.Fatalf("Reading row: %v", err)
	}
	wantRow := Row{"ts": []ReadItem{
		// These should be returned in descending timestamp order.
		{Row: "testrow", Column: "ts:col", Timestamp: 3000, Value: []byte("val-3")},
		{Row: "testrow", Column: "ts:col", Timestamp: 2000, Value: []byte("val-2")},
		{Row: "testrow", Column: "ts:col", Timestamp: 1000, Value: []byte("val-1")},
		{Row: "testrow", Column: "ts:col2", Timestamp: 3000, Value: []byte("val-3")},
		{Row: "testrow", Column: "ts:col2", Timestamp: 2000, Value: []byte("val-2")},
		{Row: "testrow", Column: "ts:col2", Timestamp: 1000, Value: []byte("val-1")},
	}}
	if !testutil.Equal(r, wantRow) {
		t.Fatalf("Cell with multiple versions,\n got %v\nwant %v", r, wantRow)
	}

	// Do the same read, but filter to the latest two versions.
	r, err = table.ReadRow(ctx, "testrow", RowFilter(LatestNFilter(2)))
	if err != nil {
		t.Fatalf("Reading row: %v", err)
	}
	wantRow = Row{"ts": []ReadItem{
		{Row: "testrow", Column: "ts:col", Timestamp: 3000, Value: []byte("val-3")},
		{Row: "testrow", Column: "ts:col", Timestamp: 2000, Value: []byte("val-2")},
		{Row: "testrow", Column: "ts:col2", Timestamp: 3000, Value: []byte("val-3")},
		{Row: "testrow", Column: "ts:col2", Timestamp: 2000, Value: []byte("val-2")},
	}}
	if !testutil.Equal(r, wantRow) {
		t.Fatalf("Cell with multiple versions and LatestNFilter(2),\n got %v\nwant %v", r, wantRow)
	}
	// Check cell offset / end
	r, err = table.ReadRow(ctx, "testrow", RowFilter(CellsPerRowLimitFilter(3)))
	if err != nil {
		t.Fatalf("Reading row: %v", err)
	}
	wantRow = Row{"ts": []ReadItem{
		{Row: "testrow", Column: "ts:col", Timestamp: 3000, Value: []byte("val-3")},
		{Row: "testrow", Column: "ts:col", Timestamp: 2000, Value: []byte("val-2")},
		{Row: "testrow", Column: "ts:col", Timestamp: 1000, Value: []byte("val-1")},
	}}
	if !testutil.Equal(r, wantRow) {
		t.Fatalf("Cell with multiple versions and CellsPerRowLimitFilter(3),\n got %v\nwant %v", r, wantRow)
	}
	r, err = table.ReadRow(ctx, "testrow", RowFilter(CellsPerRowOffsetFilter(3)))
	if err != nil {
		t.Fatalf("Reading row: %v", err)
	}
	wantRow = Row{"ts": []ReadItem{
		{Row: "testrow", Column: "ts:col2", Timestamp: 3000, Value: []byte("val-3")},
		{Row: "testrow", Column: "ts:col2", Timestamp: 2000, Value: []byte("val-2")},
		{Row: "testrow", Column: "ts:col2", Timestamp: 1000, Value: []byte("val-1")},
	}}
	if !testutil.Equal(r, wantRow) {
		t.Fatalf("Cell with multiple versions and CellsPerRowOffsetFilter(3),\n got %v\nwant %v", r, wantRow)
	}
	// Check timestamp range filtering (with truncation)
	r, err = table.ReadRow(ctx, "testrow", RowFilter(TimestampRangeFilterMicros(1001, 3000)))
	if err != nil {
		t.Fatalf("Reading row: %v", err)
	}
	wantRow = Row{"ts": []ReadItem{
		{Row: "testrow", Column: "ts:col", Timestamp: 2000, Value: []byte("val-2")},
		{Row: "testrow", Column: "ts:col", Timestamp: 1000, Value: []byte("val-1")},
		{Row: "testrow", Column: "ts:col2", Timestamp: 2000, Value: []byte("val-2")},
		{Row: "testrow", Column: "ts:col2", Timestamp: 1000, Value: []byte("val-1")},
	}}
	if !testutil.Equal(r, wantRow) {
		t.Fatalf("Cell with multiple versions and TimestampRangeFilter(1000, 3000),\n got %v\nwant %v", r, wantRow)
	}
	r, err = table.ReadRow(ctx, "testrow", RowFilter(TimestampRangeFilterMicros(1000, 0)))
	if err != nil {
		t.Fatalf("Reading row: %v", err)
	}
	wantRow = Row{"ts": []ReadItem{
		{Row: "testrow", Column: "ts:col", Timestamp: 3000, Value: []byte("val-3")},
		{Row: "testrow", Column: "ts:col", Timestamp: 2000, Value: []byte("val-2")},
		{Row: "testrow", Column: "ts:col", Timestamp: 1000, Value: []byte("val-1")},
		{Row: "testrow", Column: "ts:col2", Timestamp: 3000, Value: []byte("val-3")},
		{Row: "testrow", Column: "ts:col2", Timestamp: 2000, Value: []byte("val-2")},
		{Row: "testrow", Column: "ts:col2", Timestamp: 1000, Value: []byte("val-1")},
	}}
	if !testutil.Equal(r, wantRow) {
		t.Fatalf("Cell with multiple versions and TimestampRangeFilter(1000, 0),\n got %v\nwant %v", r, wantRow)
	}
	// Delete non-existing cells, no such column family in this row
	// Should not delete anything

	if err := adminClient.CreateColumnFamily(ctx, tableName, "non-existing"); err != nil {
		t.Fatalf("Creating column family: %v", err)
	}
	mut = NewMutation()
	mut.DeleteTimestampRange("non-existing", "col", 2000, 3000) // half-open interval
	if err := table.Apply(ctx, "testrow", mut); err != nil {
		t.Fatalf("Mutating row: %v", err)
	}
	r, err = table.ReadRow(ctx, "testrow", RowFilter(LatestNFilter(3)))
	if err != nil {
		t.Fatalf("Reading row: %v", err)
	}
	if !testutil.Equal(r, wantRow) {
		t.Fatalf("Cell was deleted unexpectly,\n got %v\nwant %v", r, wantRow)
	}
	// Delete non-existing cells, no such column in this column family
	// Should not delete anything
	mut = NewMutation()
	mut.DeleteTimestampRange("ts", "non-existing", 2000, 3000) // half-open interval
	if err := table.Apply(ctx, "testrow", mut); err != nil {
		t.Fatalf("Mutating row: %v", err)
	}
	r, err = table.ReadRow(ctx, "testrow", RowFilter(LatestNFilter(3)))
	if err != nil {
		t.Fatalf("Reading row: %v", err)
	}
	if !testutil.Equal(r, wantRow) {
		t.Fatalf("Cell was deleted unexpectly,\n got %v\nwant %v", r, wantRow)
	}
	// Delete the cell with timestamp 2000 and repeat the last read,
	// checking that we get ts 3000 and ts 1000.
	mut = NewMutation()
	mut.DeleteTimestampRange("ts", "col", 2001, 3000) // half-open interval
	if err := table.Apply(ctx, "testrow", mut); err != nil {
		t.Fatalf("Mutating row: %v", err)
	}
	r, err = table.ReadRow(ctx, "testrow", RowFilter(LatestNFilter(2)))
	if err != nil {
		t.Fatalf("Reading row: %v", err)
	}
	wantRow = Row{"ts": []ReadItem{
		{Row: "testrow", Column: "ts:col", Timestamp: 3000, Value: []byte("val-3")},
		{Row: "testrow", Column: "ts:col", Timestamp: 1000, Value: []byte("val-1")},
		{Row: "testrow", Column: "ts:col2", Timestamp: 3000, Value: []byte("val-3")},
		{Row: "testrow", Column: "ts:col2", Timestamp: 2000, Value: []byte("val-2")},
	}}
	if !testutil.Equal(r, wantRow) {
		t.Fatalf("Cell with multiple versions and LatestNFilter(2), after deleting timestamp 2000,\n got %v\nwant %v", r, wantRow)
	}

	// Check DeleteCellsInFamily
	if err := adminClient.CreateColumnFamily(ctx, tableName, "status"); err != nil {
		t.Fatalf("Creating column family: %v", err)
	}

	mut = NewMutation()
	mut.Set("status", "start", 2000, []byte("2"))
	mut.Set("status", "end", 3000, []byte("3"))
	mut.Set("ts", "col", 1000, []byte("1"))
	if err := table.Apply(ctx, "row1", mut); err != nil {
		t.Fatalf("Mutating row: %v", err)
	}
	if err := table.Apply(ctx, "row2", mut); err != nil {
		t.Fatalf("Mutating row: %v", err)
	}

	mut = NewMutation()
	mut.DeleteCellsInFamily("status")
	if err := table.Apply(ctx, "row1", mut); err != nil {
		t.Fatalf("Delete cf: %v", err)
	}

	// ColumnFamily removed
	r, err = table.ReadRow(ctx, "row1")
	if err != nil {
		t.Fatalf("Reading row: %v", err)
	}
	wantRow = Row{"ts": []ReadItem{
		{Row: "row1", Column: "ts:col", Timestamp: 1000, Value: []byte("1")},
	}}
	if !testutil.Equal(r, wantRow) {
		t.Fatalf("column family was not deleted.\n got %v\n want %v", r, wantRow)
	}

	// ColumnFamily not removed
	r, err = table.ReadRow(ctx, "row2")
	if err != nil {
		t.Fatalf("Reading row: %v", err)
	}
	wantRow = Row{
		"ts": []ReadItem{
			{Row: "row2", Column: "ts:col", Timestamp: 1000, Value: []byte("1")},
		},
		"status": []ReadItem{
			{Row: "row2", Column: "status:end", Timestamp: 3000, Value: []byte("3")},
			{Row: "row2", Column: "status:start", Timestamp: 2000, Value: []byte("2")},
		},
	}
	if !testutil.Equal(r, wantRow) {
		t.Fatalf("Column family was deleted unexpectedly.\n got %v\n want %v", r, wantRow)
	}

	// Check DeleteCellsInColumn
	mut = NewMutation()
	mut.Set("status", "start", 2000, []byte("2"))
	mut.Set("status", "middle", 3000, []byte("3"))
	mut.Set("status", "end", 1000, []byte("1"))
	if err := table.Apply(ctx, "row3", mut); err != nil {
		t.Fatalf("Mutating row: %v", err)
	}
	mut = NewMutation()
	mut.DeleteCellsInColumn("status", "middle")
	if err := table.Apply(ctx, "row3", mut); err != nil {
		t.Fatalf("Delete column: %v", err)
	}
	r, err = table.ReadRow(ctx, "row3")
	if err != nil {
		t.Fatalf("Reading row: %v", err)
	}
	wantRow = Row{
		"status": []ReadItem{
			{Row: "row3", Column: "status:end", Timestamp: 1000, Value: []byte("1")},
			{Row: "row3", Column: "status:start", Timestamp: 2000, Value: []byte("2")},
		},
	}
	if !testutil.Equal(r, wantRow) {
		t.Fatalf("Column was not deleted.\n got %v\n want %v", r, wantRow)
	}
	mut = NewMutation()
	mut.DeleteCellsInColumn("status", "start")
	if err := table.Apply(ctx, "row3", mut); err != nil {
		t.Fatalf("Delete column: %v", err)
	}
	r, err = table.ReadRow(ctx, "row3")
	if err != nil {
		t.Fatalf("Reading row: %v", err)
	}
	wantRow = Row{
		"status": []ReadItem{
			{Row: "row3", Column: "status:end", Timestamp: 1000, Value: []byte("1")},
		},
	}
	if !testutil.Equal(r, wantRow) {
		t.Fatalf("Column was not deleted.\n got %v\n want %v", r, wantRow)
	}
	mut = NewMutation()
	mut.DeleteCellsInColumn("status", "end")
	if err := table.Apply(ctx, "row3", mut); err != nil {
		t.Fatalf("Delete column: %v", err)
	}
	r, err = table.ReadRow(ctx, "row3")
	if err != nil {
		t.Fatalf("Reading row: %v", err)
	}
	if len(r) != 0 {
		t.Fatalf("Delete column: got %v, want empty row", r)
	}
	// Add same cell after delete
	mut = NewMutation()
	mut.Set("status", "end", 1000, []byte("1"))
	if err := table.Apply(ctx, "row3", mut); err != nil {
		t.Fatalf("Mutating row: %v", err)
	}
	r, err = table.ReadRow(ctx, "row3")
	if err != nil {
		t.Fatalf("Reading row: %v", err)
	}
	if !testutil.Equal(r, wantRow) {
		t.Fatalf("Column was not deleted correctly.\n got %v\n want %v", r, wantRow)
	}
}

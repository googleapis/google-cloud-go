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

package bigtable

import (
	"context"
	"flag"
	"fmt"
	"math"
	"math/rand"
	"reflect"
	"sort"
	"strings"
	"sync"
	"testing"
	"time"

	"cloud.google.com/go/iam"
	"cloud.google.com/go/internal"
	"cloud.google.com/go/internal/testutil"
	"cloud.google.com/go/internal/uid"
	"github.com/golang/protobuf/proto"
	"github.com/google/go-cmp/cmp"
	gax "github.com/googleapis/gax-go/v2"
	"google.golang.org/api/iterator"
	btapb "google.golang.org/genproto/googleapis/bigtable/admin/v2"
)

const (
	directPathIPV6Prefix = "[2001:4860:8040"
	directPathIPV4Prefix = "34.126"
)

var (
	presidentsSocialGraph = map[string][]string{
		"wmckinley":   {"tjefferson"},
		"gwashington": {"jadams"},
		"tjefferson":  {"gwashington", "jadams"},
		"jadams":      {"gwashington", "tjefferson"},
	}

	tableNameSpace = uid.NewSpace("cbt-test", &uid.Options{Short: true})
)

func populatePresidentsGraph(table *Table) error {
	ctx := context.Background()
	for row, ss := range presidentsSocialGraph {
		mut := NewMutation()
		for _, name := range ss {
			mut.Set("follows", name, 1000, []byte("1"))
		}
		if err := table.Apply(ctx, row, mut); err != nil {
			return fmt.Errorf("Mutating row %q: %v", row, err)
		}
	}
	return nil
}

var instanceToCreate string
var instanceToCreateZone string
var instanceToCreateZone2 string

func init() {
	// Don't test instance creation by default, as quota is necessary and aborted tests could strand resources.
	flag.StringVar(&instanceToCreate, "it.instance-to-create", "",
		"The id of an instance to create, update and delete. Requires sufficient Cloud Bigtable quota. Requires that it.use-prod is true.")
	flag.StringVar(&instanceToCreateZone, "it.instance-to-create-zone", "us-central1-b",
		"The zone in which to create the new test instance.")
	flag.StringVar(&instanceToCreateZone2, "it.instance-to-create-zone2", "us-east1-c",
		"The zone in which to create a second cluster in the test instance.")
}

func TestIntegration_ConditionalMutations(t *testing.T) {
	ctx := context.Background()
	testEnv, _, _, table, _, cleanup, err := setupIntegration(ctx, t)
	if err != nil {
		t.Fatal(err)
	}
	defer cleanup()

	if err := populatePresidentsGraph(table); err != nil {
		t.Fatal(err)
	}

	// Do a conditional mutation with a complex filter.
	mutTrue := NewMutation()
	mutTrue.Set("follows", "wmckinley", 1000, []byte("1"))
	filter := ChainFilters(ColumnFilter("gwash[iz].*"), ValueFilter("."))
	mut := NewCondMutation(filter, mutTrue, nil)
	if err := table.Apply(ctx, "tjefferson", mut); err != nil {
		t.Fatalf("Conditionally mutating row: %v", err)
	}
	verifyDirectPathRemoteAddress(testEnv, t)
	// Do a second condition mutation with a filter that does not match,
	// and thus no changes should be made.
	mutTrue = NewMutation()
	mutTrue.DeleteRow()
	filter = ColumnFilter("snoop.dogg")
	mut = NewCondMutation(filter, mutTrue, nil)
	if err := table.Apply(ctx, "tjefferson", mut); err != nil {
		t.Fatalf("Conditionally mutating row: %v", err)
	}
	verifyDirectPathRemoteAddress(testEnv, t)

	// Fetch a row.
	row, err := table.ReadRow(ctx, "jadams")
	if err != nil {
		t.Fatalf("Reading a row: %v", err)
	}
	verifyDirectPathRemoteAddress(testEnv, t)
	wantRow := Row{
		"follows": []ReadItem{
			{Row: "jadams", Column: "follows:gwashington", Timestamp: 1000, Value: []byte("1")},
			{Row: "jadams", Column: "follows:tjefferson", Timestamp: 1000, Value: []byte("1")},
		},
	}
	if !testutil.Equal(row, wantRow) {
		t.Fatalf("Read row mismatch.\n got %#v\nwant %#v", row, wantRow)
	}
}

func TestIntegration_PartialReadRows(t *testing.T) {
	ctx := context.Background()
	_, _, _, table, _, cleanup, err := setupIntegration(ctx, t)
	if err != nil {
		t.Fatal(err)
	}
	defer cleanup()

	if err := populatePresidentsGraph(table); err != nil {
		t.Fatal(err)
	}

	// Do a scan and stop part way through.
	// Verify that the ReadRows callback doesn't keep running.
	stopped := false
	err = table.ReadRows(ctx, InfiniteRange(""), func(r Row) bool {
		if r.Key() < "h" {
			return true
		}
		if !stopped {
			stopped = true
			return false
		}
		t.Fatalf("ReadRows kept scanning to row %q after being told to stop", r.Key())
		return false
	})
	if err != nil {
		t.Fatalf("Partial ReadRows: %v", err)
	}
}

func TestIntegration_ReadRowList(t *testing.T) {
	ctx := context.Background()
	_, _, _, table, _, cleanup, err := setupIntegration(ctx, t)
	if err != nil {
		t.Fatal(err)
	}
	defer cleanup()

	if err := populatePresidentsGraph(table); err != nil {
		t.Fatal(err)
	}

	// Read a RowList
	var elt []string
	keys := RowList{"wmckinley", "gwashington", "jadams"}
	want := "gwashington-jadams-1,jadams-gwashington-1,jadams-tjefferson-1,wmckinley-tjefferson-1"
	err = table.ReadRows(ctx, keys, func(r Row) bool {
		for _, ris := range r {
			for _, ri := range ris {
				elt = append(elt, formatReadItem(ri))
			}
		}
		return true
	})
	if err != nil {
		t.Fatalf("read RowList: %v", err)
	}

	if got := strings.Join(elt, ","); got != want {
		t.Fatalf("bulk read: wrong reads.\n got %q\nwant %q", got, want)
	}
}

func TestIntegration_DeleteRow(t *testing.T) {
	ctx := context.Background()
	_, _, _, table, _, cleanup, err := setupIntegration(ctx, t)
	if err != nil {
		t.Fatal(err)
	}
	defer cleanup()

	if err := populatePresidentsGraph(table); err != nil {
		t.Fatal(err)
	}

	// Delete a row and check it goes away.
	mut := NewMutation()
	mut.DeleteRow()
	if err := table.Apply(ctx, "wmckinley", mut); err != nil {
		t.Fatalf("Apply DeleteRow: %v", err)
	}
	row, err := table.ReadRow(ctx, "wmckinley")
	if err != nil {
		t.Fatalf("Reading a row after DeleteRow: %v", err)
	}
	if len(row) != 0 {
		t.Fatalf("Read non-zero row after DeleteRow: %v", row)
	}
}

func TestIntegration_ReadModifyWrite(t *testing.T) {
	ctx := context.Background()
	testEnv, _, adminClient, table, tableName, cleanup, err := setupIntegration(ctx, t)
	if err != nil {
		t.Fatal(err)
	}
	defer cleanup()

	if err := populatePresidentsGraph(table); err != nil {
		t.Fatal(err)
	}

	if err := adminClient.CreateColumnFamily(ctx, tableName, "counter"); err != nil {
		t.Fatalf("Creating column family: %v", err)
	}

	appendRMW := func(b []byte) *ReadModifyWrite {
		rmw := NewReadModifyWrite()
		rmw.AppendValue("counter", "likes", b)
		return rmw
	}
	incRMW := func(n int64) *ReadModifyWrite {
		rmw := NewReadModifyWrite()
		rmw.Increment("counter", "likes", n)
		return rmw
	}
	rmwSeq := []struct {
		desc string
		rmw  *ReadModifyWrite
		want []byte
	}{
		{
			desc: "append #1",
			rmw:  appendRMW([]byte{0, 0, 0}),
			want: []byte{0, 0, 0},
		},
		{
			desc: "append #2",
			rmw:  appendRMW([]byte{0, 0, 0, 0, 17}), // the remaining 40 bits to make a big-endian 17
			want: []byte{0, 0, 0, 0, 0, 0, 0, 17},
		},
		{
			desc: "increment",
			rmw:  incRMW(8),
			want: []byte{0, 0, 0, 0, 0, 0, 0, 25},
		},
	}
	for _, step := range rmwSeq {
		row, err := table.ApplyReadModifyWrite(ctx, "gwashington", step.rmw)
		if err != nil {
			t.Fatalf("ApplyReadModifyWrite %+v: %v", step.rmw, err)
		}
		verifyDirectPathRemoteAddress(testEnv, t)
		// Make sure the modified cell returned by the RMW operation has a timestamp.
		if row["counter"][0].Timestamp == 0 {
			t.Fatalf("RMW returned cell timestamp: got %v, want > 0", row["counter"][0].Timestamp)
		}
		clearTimestamps(row)
		wantRow := Row{"counter": []ReadItem{{Row: "gwashington", Column: "counter:likes", Value: step.want}}}
		if !testutil.Equal(row, wantRow) {
			t.Fatalf("After %s,\n got %v\nwant %v", step.desc, row, wantRow)
		}
	}

	// Check for google-cloud-go/issues/723. RMWs that insert new rows should keep row order sorted in the emulator.
	_, err = table.ApplyReadModifyWrite(ctx, "issue-723-2", appendRMW([]byte{0}))
	if err != nil {
		t.Fatalf("ApplyReadModifyWrite null string: %v", err)
	}
	verifyDirectPathRemoteAddress(testEnv, t)
	_, err = table.ApplyReadModifyWrite(ctx, "issue-723-1", appendRMW([]byte{0}))
	if err != nil {
		t.Fatalf("ApplyReadModifyWrite null string: %v", err)
	}
	verifyDirectPathRemoteAddress(testEnv, t)
	// Get only the correct row back on read.
	r, err := table.ReadRow(ctx, "issue-723-1")
	if err != nil {
		t.Fatalf("Reading row: %v", err)
	}
	verifyDirectPathRemoteAddress(testEnv, t)
	if r.Key() != "issue-723-1" {
		t.Fatalf("ApplyReadModifyWrite: incorrect read after RMW,\n got %v\nwant %v", r.Key(), "issue-723-1")
	}
}

func TestIntegration_ArbitraryTimestamps(t *testing.T) {
	ctx := context.Background()
	_, _, adminClient, table, tableName, cleanup, err := setupIntegration(ctx, t)
	if err != nil {
		t.Fatal(err)
	}
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
	// Check cell offset / limit
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

func TestIntegration_HighlyConcurrentReadsAndWrites(t *testing.T) {
	ctx := context.Background()
	_, _, adminClient, table, tableName, cleanup, err := setupIntegration(ctx, t)
	if err != nil {
		t.Fatal(err)
	}
	defer cleanup()

	if err := populatePresidentsGraph(table); err != nil {
		t.Fatal(err)
	}

	if err := adminClient.CreateColumnFamily(ctx, tableName, "ts"); err != nil {
		t.Fatalf("Creating column family: %v", err)
	}

	// Do highly concurrent reads/writes.
	const maxConcurrency = 1000
	var wg sync.WaitGroup
	for i := 0; i < maxConcurrency; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			switch r := rand.Intn(100); { // r ∈ [0,100)
			case 0 <= r && r < 30:
				// Do a read.
				_, err := table.ReadRow(ctx, "testrow", RowFilter(LatestNFilter(1)))
				if err != nil {
					t.Errorf("Concurrent read: %v", err)
				}
			case 30 <= r && r < 100:
				// Do a write.
				mut := NewMutation()
				mut.Set("ts", "col", 1000, []byte("data"))
				if err := table.Apply(ctx, "testrow", mut); err != nil {
					t.Errorf("Concurrent write: %v", err)
				}
			}
		}()
	}
	wg.Wait()
}

func TestIntegration_LargeReadsWritesAndScans(t *testing.T) {
	ctx := context.Background()
	testEnv, _, adminClient, table, tableName, cleanup, err := setupIntegration(ctx, t)
	if err != nil {
		t.Fatal(err)
	}
	defer cleanup()

	if err := adminClient.CreateColumnFamily(ctx, tableName, "ts"); err != nil {
		t.Fatalf("Creating column family: %v", err)
	}

	bigBytes := make([]byte, 5<<20) // 5 MB is larger than current default gRPC max of 4 MB, but less than the max we set.
	nonsense := []byte("lorem ipsum dolor sit amet, ")
	fill(bigBytes, nonsense)
	mut := NewMutation()
	mut.Set("ts", "col", 1000, bigBytes)
	if err := table.Apply(ctx, "bigrow", mut); err != nil {
		t.Fatalf("Big write: %v", err)
	}
	verifyDirectPathRemoteAddress(testEnv, t)
	r, err := table.ReadRow(ctx, "bigrow")
	if err != nil {
		t.Fatalf("Big read: %v", err)
	}
	verifyDirectPathRemoteAddress(testEnv, t)
	wantRow := Row{"ts": []ReadItem{
		{Row: "bigrow", Column: "ts:col", Timestamp: 1000, Value: bigBytes},
	}}
	if !testutil.Equal(r, wantRow) {
		t.Fatalf("Big read returned incorrect bytes: %v", r)
	}

	var wg sync.WaitGroup
	// Now write 1000 rows, each with 82 KB values, then scan them all.
	medBytes := make([]byte, 82<<10)
	fill(medBytes, nonsense)
	sem := make(chan int, 50) // do up to 50 mutations at a time.
	for i := 0; i < 1000; i++ {
		mut := NewMutation()
		mut.Set("ts", "big-scan", 1000, medBytes)
		row := fmt.Sprintf("row-%d", i)
		wg.Add(1)
		go func() {
			defer wg.Done()
			defer func() { <-sem }()
			sem <- 1
			if err := table.Apply(ctx, row, mut); err != nil {
				t.Errorf("Preparing large scan: %v", err)
			}
			verifyDirectPathRemoteAddress(testEnv, t)
		}()
	}
	wg.Wait()
	n := 0
	err = table.ReadRows(ctx, PrefixRange("row-"), func(r Row) bool {
		for _, ris := range r {
			for _, ri := range ris {
				n += len(ri.Value)
			}
		}
		return true
	}, RowFilter(ColumnFilter("big-scan")))
	if err != nil {
		t.Fatalf("Doing large scan: %v", err)
	}
	verifyDirectPathRemoteAddress(testEnv, t)
	if want := 1000 * len(medBytes); n != want {
		t.Fatalf("Large scan returned %d bytes, want %d", n, want)
	}
	// Scan a subset of the 1000 rows that we just created, using a LimitRows ReadOption.
	rc := 0
	wantRc := 3
	err = table.ReadRows(ctx, PrefixRange("row-"), func(r Row) bool {
		rc++
		return true
	}, LimitRows(int64(wantRc)))
	if err != nil {
		t.Fatal(err)
	}
	verifyDirectPathRemoteAddress(testEnv, t)
	if rc != wantRc {
		t.Fatalf("Scan with row limit returned %d rows, want %d", rc, wantRc)
	}

	// Test bulk mutations
	if err := adminClient.CreateColumnFamily(ctx, tableName, "bulk"); err != nil {
		t.Fatalf("Creating column family: %v", err)
	}
	bulkData := map[string][]string{
		"red sox":  {"2004", "2007", "2013"},
		"patriots": {"2001", "2003", "2004", "2014"},
		"celtics":  {"1981", "1984", "1986", "2008"},
	}
	var rowKeys []string
	var muts []*Mutation
	for row, ss := range bulkData {
		mut := NewMutation()
		for _, name := range ss {
			mut.Set("bulk", name, 1000, []byte("1"))
		}
		rowKeys = append(rowKeys, row)
		muts = append(muts, mut)
	}
	status, err := table.ApplyBulk(ctx, rowKeys, muts)
	if err != nil {
		t.Fatalf("Bulk mutating rows %q: %v", rowKeys, err)
	}
	verifyDirectPathRemoteAddress(testEnv, t)
	if status != nil {
		t.Fatalf("non-nil errors: %v", err)
	}

	// Read each row back
	for rowKey, ss := range bulkData {
		row, err := table.ReadRow(ctx, rowKey)
		if err != nil {
			t.Fatalf("Reading a bulk row: %v", err)
		}
		verifyDirectPathRemoteAddress(testEnv, t)
		var wantItems []ReadItem
		for _, val := range ss {
			wantItems = append(wantItems, ReadItem{Row: rowKey, Column: "bulk:" + val, Timestamp: 1000, Value: []byte("1")})
		}
		wantRow := Row{"bulk": wantItems}
		if !testutil.Equal(row, wantRow) {
			t.Fatalf("Read row mismatch.\n got %#v\nwant %#v", row, wantRow)
		}
	}

	// Test bulk write errors.
	// Note: Setting timestamps as ServerTime makes sure the mutations are not retried on error.
	badMut := NewMutation()
	badMut.Set("badfamily", "col", ServerTime, nil)
	badMut2 := NewMutation()
	badMut2.Set("badfamily2", "goodcol", ServerTime, []byte("1"))
	status, err = table.ApplyBulk(ctx, []string{"badrow", "badrow2"}, []*Mutation{badMut, badMut2})
	if err != nil {
		t.Fatalf("Bulk mutating rows %q: %v", rowKeys, err)
	}
	verifyDirectPathRemoteAddress(testEnv, t)
	if status == nil {
		t.Fatalf("No errors for bad bulk mutation")
	} else if status[0] == nil || status[1] == nil {
		t.Fatalf("No error for bad bulk mutation")
	}
}

func TestIntegration_Read(t *testing.T) {
	ctx := context.Background()
	testEnv, _, _, table, _, cleanup, err := setupIntegration(ctx, t)
	if err != nil {
		t.Fatal(err)
	}
	defer cleanup()

	// Insert some data.
	initialData := map[string][]string{
		"wmckinley":   {"tjefferson"},
		"gwashington": {"jadams"},
		"tjefferson":  {"gwashington", "jadams", "wmckinley"},
		"jadams":      {"gwashington", "tjefferson"},
	}
	for row, ss := range initialData {
		mut := NewMutation()
		for _, name := range ss {
			mut.Set("follows", name, 1000, []byte("1"))
		}
		if err := table.Apply(ctx, row, mut); err != nil {
			t.Fatalf("Mutating row %q: %v", row, err)
		}
		verifyDirectPathRemoteAddress(testEnv, t)
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
			want: "gwashington-jadams-1,jadams-gwashington-1,jadams-tjefferson-1,tjefferson-gwashington-1,tjefferson-jadams-1,tjefferson-wmckinley-1,wmckinley-tjefferson-1",
		},
		{
			desc: "read with InfiniteRange, unfiltered",
			rr:   InfiniteRange("tjefferson"),
			want: "tjefferson-gwashington-1,tjefferson-jadams-1,tjefferson-wmckinley-1,wmckinley-tjefferson-1",
		},
		{
			desc: "read with NewRange, unfiltered",
			rr:   NewRange("gargamel", "hubbard"),
			want: "gwashington-jadams-1",
		},
		{
			desc: "read with PrefixRange, unfiltered",
			rr:   PrefixRange("jad"),
			want: "jadams-gwashington-1,jadams-tjefferson-1",
		},
		{
			desc: "read with SingleRow, unfiltered",
			rr:   SingleRow("wmckinley"),
			want: "wmckinley-tjefferson-1",
		},
		{
			desc:   "read all, with ColumnFilter",
			rr:     RowRange{},
			filter: ColumnFilter(".*j.*"), // matches "jadams" and "tjefferson"
			want:   "gwashington-jadams-1,jadams-tjefferson-1,tjefferson-jadams-1,wmckinley-tjefferson-1",
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
			want:   "gwashington-jadams-1,tjefferson-jadams-1",
		},
		{
			desc:   "read range from empty, with ColumnRangeFilter",
			rr:     RowRange{},
			filter: ColumnRangeFilter("follows", "", "u"),
			want:   "gwashington-jadams-1,jadams-gwashington-1,jadams-tjefferson-1,tjefferson-gwashington-1,tjefferson-jadams-1,wmckinley-tjefferson-1",
		},
		{
			desc:   "read range from start to empty, with ColumnRangeFilter",
			rr:     RowRange{},
			filter: ColumnRangeFilter("follows", "h", ""),
			want:   "gwashington-jadams-1,jadams-tjefferson-1,tjefferson-jadams-1,tjefferson-wmckinley-1,wmckinley-tjefferson-1",
		},
		{
			desc:   "read with RowKeyFilter",
			rr:     RowRange{},
			filter: RowKeyFilter(".*wash.*"),
			want:   "gwashington-jadams-1",
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
			desc:   "read with ColumnFilter + row limit",
			rr:     RowRange{},
			filter: ColumnFilter(".*j.*"), // matches "jadams" and "tjefferson"
			limit:  LimitRows(2),
			want:   "gwashington-jadams-1,jadams-tjefferson-1",
		},
		{
			desc:       "apply labels to the result rows",
			rr:         RowRange{},
			filter:     LabelFilter("test-label"),
			limit:      LimitRows(2),
			want:       "gwashington-jadams-1,jadams-gwashington-1,jadams-tjefferson-1",
			wantLabels: []string{"test-label", "test-label", "test-label"},
		},
		{
			desc:   "read all, strip values",
			rr:     RowRange{},
			filter: StripValueFilter(),
			want:   "gwashington-jadams-,jadams-gwashington-,jadams-tjefferson-,tjefferson-gwashington-,tjefferson-jadams-,tjefferson-wmckinley-,wmckinley-tjefferson-",
		},
		{
			desc:   "read with ColumnFilter + row limit + strip values",
			rr:     RowRange{},
			filter: ChainFilters(ColumnFilter(".*j.*"), StripValueFilter()), // matches "jadams" and "tjefferson"
			limit:  LimitRows(2),
			want:   "gwashington-jadams-,jadams-tjefferson-",
		},
		{
			desc:   "read with condition, strip values on true",
			rr:     RowRange{},
			filter: ConditionFilter(ColumnFilter(".*j.*"), StripValueFilter(), nil),
			want:   "gwashington-jadams-,jadams-gwashington-,jadams-tjefferson-,tjefferson-gwashington-,tjefferson-jadams-,tjefferson-wmckinley-,wmckinley-tjefferson-",
		},
		{
			desc:   "read with condition, strip values on false",
			rr:     RowRange{},
			filter: ConditionFilter(ColumnFilter(".*xxx.*"), nil, StripValueFilter()),
			want:   "gwashington-jadams-,jadams-gwashington-,jadams-tjefferson-,tjefferson-gwashington-,tjefferson-jadams-,tjefferson-wmckinley-,wmckinley-tjefferson-",
		},
		{
			desc:   "read with ValueRangeFilter + row limit",
			rr:     RowRange{},
			filter: ValueRangeFilter([]byte("1"), []byte("5")), // matches our value of "1"
			limit:  LimitRows(2),
			want:   "gwashington-jadams-1,jadams-gwashington-1,jadams-tjefferson-1",
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
			want:   "gwashington-jadams-1,jadams-gwashington-1,jadams-tjefferson-1,tjefferson-gwashington-1,tjefferson-jadams-1,wmckinley-tjefferson-1",
		},
		{
			desc:   "read with InterleaveFilter, with duplicate cells",
			rr:     RowRange{},
			filter: InterleaveFilters(ColumnFilter(".*g.*"), ColumnFilter(".*g.*")),
			want:   "jadams-gwashington-1,jadams-gwashington-1,tjefferson-gwashington-1,tjefferson-gwashington-1",
		},
		{
			desc: "read with a RowRangeList and no filter",
			rr:   RowRangeList{NewRange("gargamel", "hubbard"), InfiniteRange("wmckinley")},
			want: "gwashington-jadams-1,wmckinley-tjefferson-1",
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
			verifyDirectPathRemoteAddress(testEnv, t)
			if got := strings.Join(elt, ","); got != test.want {
				t.Fatalf("got %q\nwant %q", got, test.want)
			}
			if got, want := labels, test.wantLabels; !reflect.DeepEqual(got, want) {
				t.Fatalf("got %q\nwant %q", got, want)
			}
		})
	}
}

func TestIntegration_SampleRowKeys(t *testing.T) {
	ctx := context.Background()
	testEnv, _, _, table, _, cleanup, err := setupIntegration(ctx, t)
	if err != nil {
		t.Fatal(err)
	}
	defer cleanup()

	// Insert some data.
	initialData := map[string][]string{
		"wmckinley11":   {"tjefferson11"},
		"gwashington77": {"jadams77"},
		"tjefferson0":   {"gwashington0", "jadams0"},
	}

	for row, ss := range initialData {
		mut := NewMutation()
		for _, name := range ss {
			mut.Set("follows", name, 1000, []byte("1"))
		}
		if err := table.Apply(ctx, row, mut); err != nil {
			t.Fatalf("Mutating row %q: %v", row, err)
		}
		verifyDirectPathRemoteAddress(testEnv, t)
	}
	sampleKeys, err := table.SampleRowKeys(context.Background())
	if err != nil {
		t.Fatalf("%s: %v", "SampleRowKeys:", err)
	}
	if len(sampleKeys) == 0 {
		t.Error("SampleRowKeys length 0")
	}
}

func TestIntegration_Admin(t *testing.T) {
	testEnv, err := NewIntegrationEnv()
	if err != nil {
		t.Fatalf("IntegrationEnv: %v", err)
	}
	defer testEnv.Close()

	timeout := 2 * time.Second
	if testEnv.Config().UseProd {
		timeout = 5 * time.Minute
	}
	ctx, _ := context.WithTimeout(context.Background(), timeout)

	adminClient, err := testEnv.NewAdminClient()
	if err != nil {
		t.Fatalf("NewAdminClient: %v", err)
	}
	defer adminClient.Close()

	iAdminClient, err := testEnv.NewInstanceAdminClient()
	if err != nil {
		t.Fatalf("NewInstanceAdminClient: %v", err)
	}
	if iAdminClient != nil {
		defer iAdminClient.Close()

		iInfo, err := iAdminClient.InstanceInfo(ctx, adminClient.instance)
		if err != nil {
			t.Errorf("InstanceInfo: %v", err)
		}
		if iInfo.Name != adminClient.instance {
			t.Errorf("InstanceInfo returned name %#v, want %#v", iInfo.Name, adminClient.instance)
		}
	}

	list := func() []string {
		tbls, err := adminClient.Tables(ctx)
		if err != nil {
			t.Fatalf("Fetching list of tables: %v", err)
		}
		sort.Strings(tbls)
		return tbls
	}
	containsAll := func(got, want []string) bool {
		gotSet := make(map[string]bool)

		for _, s := range got {
			gotSet[s] = true
		}
		for _, s := range want {
			if !gotSet[s] {
				return false
			}
		}
		return true
	}

	defer deleteTable(ctx, t, adminClient, "mytable")

	if err := adminClient.CreateTable(ctx, "mytable"); err != nil {
		t.Fatalf("Creating table: %v", err)
	}

	defer deleteTable(ctx, t, adminClient, "myothertable")

	if err := adminClient.CreateTable(ctx, "myothertable"); err != nil {
		t.Fatalf("Creating table: %v", err)
	}

	if got, want := list(), []string{"myothertable", "mytable"}; !containsAll(got, want) {
		t.Errorf("adminClient.Tables returned %#v, want %#v", got, want)
	}

	must(adminClient.WaitForReplication(ctx, "mytable"))

	if err := adminClient.DeleteTable(ctx, "myothertable"); err != nil {
		t.Fatalf("Deleting table: %v", err)
	}
	tables := list()
	if got, want := tables, []string{"mytable"}; !containsAll(got, want) {
		t.Errorf("adminClient.Tables returned %#v, want %#v", got, want)
	}
	if got, unwanted := tables, []string{"myothertable"}; containsAll(got, unwanted) {
		t.Errorf("adminClient.Tables return %#v. unwanted %#v", got, unwanted)
	}

	tblConf := TableConf{
		TableID: "conftable",
		Families: map[string]GCPolicy{
			"fam1": MaxVersionsPolicy(1),
			"fam2": MaxVersionsPolicy(2),
		},
	}
	if err := adminClient.CreateTableFromConf(ctx, &tblConf); err != nil {
		t.Fatalf("Creating table from TableConf: %v", err)
	}
	defer deleteTable(ctx, t, adminClient, tblConf.TableID)

	tblInfo, err := adminClient.TableInfo(ctx, tblConf.TableID)
	if err != nil {
		t.Fatalf("Getting table info: %v", err)
	}
	sort.Strings(tblInfo.Families)
	wantFams := []string{"fam1", "fam2"}
	if !testutil.Equal(tblInfo.Families, wantFams) {
		t.Errorf("Column family mismatch, got %v, want %v", tblInfo.Families, wantFams)
	}

	// Populate mytable and drop row ranges
	if err = adminClient.CreateColumnFamily(ctx, "mytable", "cf"); err != nil {
		t.Fatalf("Creating column family: %v", err)
	}

	client, err := testEnv.NewClient()
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}
	defer client.Close()

	tbl := client.Open("mytable")

	prefixes := []string{"a", "b", "c"}
	for _, prefix := range prefixes {
		for i := 0; i < 5; i++ {
			mut := NewMutation()
			mut.Set("cf", "col", 1000, []byte("1"))
			if err := tbl.Apply(ctx, fmt.Sprintf("%v-%v", prefix, i), mut); err != nil {
				t.Fatalf("Mutating row: %v", err)
			}
		}
	}

	if err = adminClient.DropRowRange(ctx, "mytable", "a"); err != nil {
		t.Errorf("DropRowRange a: %v", err)
	}
	if err = adminClient.DropRowRange(ctx, "mytable", "c"); err != nil {
		t.Errorf("DropRowRange c: %v", err)
	}
	if err = adminClient.DropRowRange(ctx, "mytable", "x"); err != nil {
		t.Errorf("DropRowRange x: %v", err)
	}

	var gotRowCount int
	must(tbl.ReadRows(ctx, RowRange{}, func(row Row) bool {
		gotRowCount++
		if !strings.HasPrefix(row.Key(), "b") {
			t.Errorf("Invalid row after dropping range: %v", row)
		}
		return true
	}))
	if gotRowCount != 5 {
		t.Errorf("Invalid row count after dropping range: got %v, want %v", gotRowCount, 5)
	}

	if err = adminClient.DropAllRows(ctx, "mytable"); err != nil {
		t.Errorf("DropAllRows mytable: %v", err)
	}

	gotRowCount = 0
	must(tbl.ReadRows(ctx, RowRange{}, func(row Row) bool {
		gotRowCount++
		return true
	}))
	if gotRowCount != 0 {
		t.Errorf("Invalid row count after truncating table: got %v, want %v", gotRowCount, 0)
	}
}

func TestIntegration_TableIam(t *testing.T) {
	testEnv, err := NewIntegrationEnv()
	if err != nil {
		t.Fatalf("IntegrationEnv: %v", err)
	}
	defer testEnv.Close()

	if !testEnv.Config().UseProd {
		t.Skip("emulator doesn't support IAM Policy creation")
	}

	timeout := 5 * time.Minute
	ctx, _ := context.WithTimeout(context.Background(), timeout)

	adminClient, err := testEnv.NewAdminClient()
	if err != nil {
		t.Fatalf("NewAdminClient: %v", err)
	}
	defer adminClient.Close()

	defer deleteTable(ctx, t, adminClient, "mytable")
	if err := adminClient.CreateTable(ctx, "mytable"); err != nil {
		t.Fatalf("Creating table: %v", err)
	}

	// Verify that the IAM Controls work for Tables.
	iamHandle := adminClient.TableIAM("mytable")
	p, err := iamHandle.Policy(ctx)
	if err != nil {
		t.Errorf("Iam GetPolicy mytable: %v", err)
	}
	if err = iamHandle.SetPolicy(ctx, p); err != nil {
		t.Errorf("Iam SetPolicy mytable: %v", err)
	}
	if _, err = iamHandle.TestPermissions(ctx, []string{"bigtable.tables.get"}); err != nil {
		t.Errorf("Iam TestPermissions mytable: %v", err)
	}
}

func TestIntegration_BackupIAM(t *testing.T) {
	testEnv, err := NewIntegrationEnv()
	if err != nil {
		t.Fatalf("IntegrationEnv: %v", err)
	}
	defer testEnv.Close()

	if !testEnv.Config().UseProd {
		t.Skip("emulator doesn't support IAM Policy creation")
	}
	timeout := 5 * time.Minute
	ctx, _ := context.WithTimeout(context.Background(), timeout)

	adminClient, err := testEnv.NewAdminClient()
	if err != nil {
		t.Fatalf("NewAdminClient: %v", err)
	}
	defer adminClient.Close()

	table := testEnv.Config().Table
	cluster := testEnv.Config().Cluster

	defer deleteTable(ctx, t, adminClient, table)
	if err := adminClient.CreateTable(ctx, table); err != nil {
		t.Fatalf("Creating table: %v", err)
	}
	// Create backup.
	backup := "backup"
	defer adminClient.DeleteBackup(ctx, cluster, backup)
	if err = adminClient.CreateBackup(ctx, table, cluster, backup, time.Now().Add(8*time.Hour)); err != nil {
		t.Fatalf("Creating backup: %v", err)
	}
	iamHandle := adminClient.BackupIAM(cluster, backup)
	// Get backup policy.
	p, err := iamHandle.Policy(ctx)
	if err != nil {
		t.Errorf("iamHandle.Policy: %v", err)
	}
	// The resource is new, so the policy should be empty.
	if got := p.Roles(); len(got) > 0 {
		t.Errorf("got roles %v, want none", got)
	}
	// Set backup policy.
	member := "domain:google.com"
	// Add a member, set the policy, then check that the member is present.
	p.Add(member, iam.Viewer)
	if err = iamHandle.SetPolicy(ctx, p); err != nil {
		t.Errorf("iamHandle.SetPolicy: %v", err)
	}
	p, err = iamHandle.Policy(ctx)
	if err != nil {
		t.Errorf("iamHandle.Policy: %v", err)
	}
	if got, want := p.Members(iam.Viewer), []string{member}; !testutil.Equal(got, want) {
		t.Errorf("iamHandle.Policy: got %v, want %v", got, want)
	}
	// Test backup permissions.
	permissions := []string{"bigtable.backups.get", "bigtable.backups.update"}
	_, err = iamHandle.TestPermissions(ctx, permissions)
	if err != nil {
		t.Errorf("iamHandle.TestPermissions: %v", err)
	}
}

func TestIntegration_AdminCreateInstance(t *testing.T) {
	if instanceToCreate == "" {
		t.Skip("instanceToCreate not set, skipping instance creation testing")
	}

	testEnv, err := NewIntegrationEnv()
	if err != nil {
		t.Fatalf("IntegrationEnv: %v", err)
	}
	defer testEnv.Close()

	if !testEnv.Config().UseProd {
		t.Skip("emulator doesn't support instance creation")
	}

	timeout := 5 * time.Minute
	ctx, _ := context.WithTimeout(context.Background(), timeout)

	iAdminClient, err := testEnv.NewInstanceAdminClient()
	if err != nil {
		t.Fatalf("NewInstanceAdminClient: %v", err)
	}
	defer iAdminClient.Close()

	clusterID := instanceToCreate + "-cluster"

	// Create a development instance
	conf := &InstanceConf{
		InstanceId:   instanceToCreate,
		ClusterId:    clusterID,
		DisplayName:  "test instance",
		Zone:         instanceToCreateZone,
		InstanceType: DEVELOPMENT,
		Labels:       map[string]string{"test-label-key": "test-label-value"},
	}
	if err := iAdminClient.CreateInstance(ctx, conf); err != nil {
		t.Fatalf("CreateInstance: %v", err)
	}
	defer iAdminClient.DeleteInstance(ctx, instanceToCreate)

	iInfo, err := iAdminClient.InstanceInfo(ctx, instanceToCreate)
	if err != nil {
		t.Fatalf("InstanceInfo: %v", err)
	}

	// Basic return values are tested elsewhere, check instance type
	if iInfo.InstanceType != DEVELOPMENT {
		t.Fatalf("Instance is not DEVELOPMENT: %v", iInfo.InstanceType)
	}
	if got, want := iInfo.Labels, conf.Labels; !cmp.Equal(got, want) {
		t.Fatalf("Labels: %v, want: %v", got, want)
	}

	// Update everything we can about the instance in one call.
	confWithClusters := &InstanceWithClustersConfig{
		InstanceID:   instanceToCreate,
		DisplayName:  "new display name",
		InstanceType: PRODUCTION,
		Labels:       map[string]string{"new-label-key": "new-label-value"},
		Clusters: []ClusterConfig{
			{ClusterID: clusterID, NumNodes: 5}},
	}

	if err = iAdminClient.UpdateInstanceWithClusters(ctx, confWithClusters); err != nil {
		t.Fatalf("UpdateInstanceWithClusters: %v", err)
	}

	iInfo, err = iAdminClient.InstanceInfo(ctx, instanceToCreate)
	if err != nil {
		t.Fatalf("InstanceInfo: %v", err)
	}

	if iInfo.InstanceType != PRODUCTION {
		t.Fatalf("Instance type is not PRODUCTION: %v", iInfo.InstanceType)
	}
	if got, want := iInfo.Labels, confWithClusters.Labels; !cmp.Equal(got, want) {
		t.Fatalf("Labels: %v, want: %v", got, want)
	}
	if got, want := iInfo.DisplayName, confWithClusters.DisplayName; got != want {
		t.Fatalf("Display name: %q, want: %q", got, want)
	}

	cInfo, err := iAdminClient.GetCluster(ctx, instanceToCreate, clusterID)
	if err != nil {
		t.Fatalf("GetCluster: %v", err)
	}

	if cInfo.ServeNodes != 5 {
		t.Fatalf("NumNodes: %v, want: %v", cInfo.ServeNodes, 5)
	}
}

func TestIntegration_AdminUpdateInstanceLabels(t *testing.T) {

	// Check the environments
	if instanceToCreate == "" {
		t.Skip("instanceToCreate not set, skipping instance creation testing")
	}
	testEnv, err := NewIntegrationEnv()
	if err != nil {
		t.Fatalf("IntegrationEnv: %v", err)
	}
	defer testEnv.Close()
	if !testEnv.Config().UseProd {
		t.Skip("emulator doesn't support instance creation")
	}

	// Create an instance admin client
	timeout := 5 * time.Minute
	ctx, _ := context.WithTimeout(context.Background(), timeout)
	iAdminClient, err := testEnv.NewInstanceAdminClient()
	if err != nil {
		t.Fatalf("NewInstanceAdminClient: %v", err)
	}
	defer iAdminClient.Close()

	// Create a test instance
	conf := &InstanceConf{
		InstanceId:   instanceToCreate,
		ClusterId:    instanceToCreate + "-cluster",
		DisplayName:  "test instance",
		InstanceType: DEVELOPMENT,
		Zone:         instanceToCreateZone,
	}
	if err := iAdminClient.CreateInstance(ctx, conf); err != nil {
		t.Fatalf("CreateInstance: %v", err)
	}
	defer iAdminClient.DeleteInstance(ctx, instanceToCreate)

	// Check the created test instances
	iInfo, err := iAdminClient.InstanceInfo(ctx, instanceToCreate)
	if err != nil {
		t.Fatalf("InstanceInfo: %v", err)
	}
	if got, want := iInfo.Labels, conf.Labels; !cmp.Equal(got, want) {
		t.Fatalf("Labels: %v, want: %v", got, want)
	}

	// Test patterns to update instance labels
	tests := []struct {
		name string
		in   map[string]string
		out  map[string]string
	}{
		{
			name: "update labels",
			in:   map[string]string{"test-label-key": "test-label-value"},
			out:  map[string]string{"test-label-key": "test-label-value"},
		},
		{
			name: "update multiple labels",
			in:   map[string]string{"update-label-key-a": "update-label-value-a", "update-label-key-b": "update-label-value-b"},
			out:  map[string]string{"update-label-key-a": "update-label-value-a", "update-label-key-b": "update-label-value-b"},
		},
		{
			name: "not update existing labels",
			in:   nil, // nil map
			out:  map[string]string{"update-label-key-a": "update-label-value-a", "update-label-key-b": "update-label-value-b"},
		},
		{
			name: "delete labels",
			in:   map[string]string{}, // empty map
			out:  nil,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			confWithClusters := &InstanceWithClustersConfig{
				InstanceID: instanceToCreate,
				Labels:     tt.in,
			}
			if err := iAdminClient.UpdateInstanceWithClusters(ctx, confWithClusters); err != nil {
				t.Fatalf("UpdateInstanceWithClusters: %v", err)
			}
			iInfo, err := iAdminClient.InstanceInfo(ctx, instanceToCreate)
			if err != nil {
				t.Fatalf("InstanceInfo: %v", err)
			}
			if got, want := iInfo.Labels, tt.out; !cmp.Equal(got, want) {
				t.Fatalf("Labels: %v, want: %v", got, want)
			}
		})
	}
}

func TestIntegration_AdminUpdateInstanceAndSyncClusters(t *testing.T) {
	if instanceToCreate == "" {
		t.Skip("instanceToCreate not set, skipping instance update testing")
	}

	testEnv, err := NewIntegrationEnv()
	if err != nil {
		t.Fatalf("IntegrationEnv: %v", err)
	}
	defer testEnv.Close()

	if !testEnv.Config().UseProd {
		t.Skip("emulator doesn't support instance creation")
	}

	timeout := 5 * time.Minute
	ctx, _ := context.WithTimeout(context.Background(), timeout)

	iAdminClient, err := testEnv.NewInstanceAdminClient()
	if err != nil {
		t.Fatalf("NewInstanceAdminClient: %v", err)
	}
	defer iAdminClient.Close()

	clusterID := instanceToCreate + "-cluster"

	// Create a development instance
	conf := &InstanceConf{
		InstanceId:   instanceToCreate,
		ClusterId:    clusterID,
		DisplayName:  "test instance",
		Zone:         instanceToCreateZone,
		InstanceType: DEVELOPMENT,
		Labels:       map[string]string{"test-label-key": "test-label-value"},
	}
	if err := iAdminClient.CreateInstance(ctx, conf); err != nil {
		t.Fatalf("CreateInstance: %v", err)
	}
	defer iAdminClient.DeleteInstance(ctx, instanceToCreate)

	iInfo, err := iAdminClient.InstanceInfo(ctx, instanceToCreate)
	if err != nil {
		t.Fatalf("InstanceInfo: %v", err)
	}

	// Basic return values are tested elsewhere, check instance type
	if iInfo.InstanceType != DEVELOPMENT {
		t.Fatalf("Instance is not DEVELOPMENT: %v", iInfo.InstanceType)
	}
	if got, want := iInfo.Labels, conf.Labels; !cmp.Equal(got, want) {
		t.Fatalf("Labels: %v, want: %v", got, want)
	}

	// Update everything we can about the instance in one call.
	confWithClusters := &InstanceWithClustersConfig{
		InstanceID:   instanceToCreate,
		DisplayName:  "new display name",
		InstanceType: PRODUCTION,
		Labels:       map[string]string{"new-label-key": "new-label-value"},
		Clusters: []ClusterConfig{
			{ClusterID: clusterID, NumNodes: 5}},
	}

	results, err := UpdateInstanceAndSyncClusters(ctx, iAdminClient, confWithClusters)
	if err != nil {
		t.Fatalf("UpdateInstanceAndSyncClusters: %v", err)
	}

	wantResults := UpdateInstanceResults{
		InstanceUpdated: true,
		UpdatedClusters: []string{clusterID},
	}
	if diff := testutil.Diff(*results, wantResults); diff != "" {
		t.Fatalf("UpdateInstanceResults: got - want +\n%s", diff)
	}

	iInfo, err = iAdminClient.InstanceInfo(ctx, instanceToCreate)
	if err != nil {
		t.Fatalf("InstanceInfo: %v", err)
	}

	if iInfo.InstanceType != PRODUCTION {
		t.Fatalf("Instance type is not PRODUCTION: %v", iInfo.InstanceType)
	}
	if got, want := iInfo.Labels, confWithClusters.Labels; !cmp.Equal(got, want) {
		t.Fatalf("Labels: %v, want: %v", got, want)
	}
	if got, want := iInfo.DisplayName, confWithClusters.DisplayName; got != want {
		t.Fatalf("Display name: %q, want: %q", got, want)
	}

	cInfo, err := iAdminClient.GetCluster(ctx, instanceToCreate, clusterID)
	if err != nil {
		t.Fatalf("GetCluster: %v", err)
	}

	if cInfo.ServeNodes != 5 {
		t.Fatalf("NumNodes: %v, want: %v", cInfo.ServeNodes, 5)
	}

	// Now add a second cluster as the only change. The first cluster must also be provided so
	// it is not removed.
	clusterID2 := clusterID + "-2"
	confWithClusters = &InstanceWithClustersConfig{
		InstanceID: instanceToCreate,
		Clusters: []ClusterConfig{
			{ClusterID: clusterID},
			{ClusterID: clusterID2, NumNodes: 3, StorageType: SSD, Zone: instanceToCreateZone2}},
	}

	results, err = UpdateInstanceAndSyncClusters(ctx, iAdminClient, confWithClusters)
	if err != nil {
		t.Fatalf("UpdateInstanceAndSyncClusters: %v %v", confWithClusters, err)
	}

	wantResults = UpdateInstanceResults{
		InstanceUpdated: false,
		CreatedClusters: []string{clusterID2},
	}
	if diff := testutil.Diff(*results, wantResults); diff != "" {
		t.Fatalf("UpdateInstanceResults: got - want +\n%s", diff)
	}

	// Now update one cluster and delete the other
	confWithClusters = &InstanceWithClustersConfig{
		InstanceID: instanceToCreate,
		Clusters: []ClusterConfig{
			{ClusterID: clusterID, NumNodes: 4}},
	}

	results, err = UpdateInstanceAndSyncClusters(ctx, iAdminClient, confWithClusters)
	if err != nil {
		t.Fatalf("UpdateInstanceAndSyncClusters: %v %v", confWithClusters, err)
	}

	wantResults = UpdateInstanceResults{
		InstanceUpdated: false,
		UpdatedClusters: []string{clusterID},
		DeletedClusters: []string{clusterID2},
	}

	if diff := testutil.Diff(*results, wantResults); diff != "" {
		t.Fatalf("UpdateInstanceResults: got - want +\n%s", diff)
	}

	// Make sure the instance looks as we would expect
	clusters, err := iAdminClient.Clusters(ctx, conf.InstanceId)
	if err != nil {
		t.Fatalf("Clusters: %v", err)
	}

	if len(clusters) != 1 {
		t.Fatalf("Clusters length %v, want: 1", len(clusters))
	}

	wantCluster := &ClusterInfo{
		Name:       clusterID,
		Zone:       instanceToCreateZone,
		ServeNodes: 4,
		State:      "READY",
	}
	if diff := testutil.Diff(clusters[0], wantCluster); diff != "" {
		t.Fatalf("InstanceEquality: got - want +\n%s", diff)
	}
}

func TestIntegration_AdminSnapshot(t *testing.T) {
	testEnv, err := NewIntegrationEnv()
	if err != nil {
		t.Fatalf("IntegrationEnv: %v", err)
	}
	defer testEnv.Close()

	if !testEnv.Config().UseProd {
		t.Skip("emulator doesn't support snapshots")
	}

	timeout := 2 * time.Second
	if testEnv.Config().UseProd {
		timeout = 5 * time.Minute
	}
	ctx, _ := context.WithTimeout(context.Background(), timeout)

	adminClient, err := testEnv.NewAdminClient()
	if err != nil {
		t.Fatalf("NewAdminClient: %v", err)
	}
	defer adminClient.Close()

	table := testEnv.Config().Table
	cluster := testEnv.Config().Cluster

	list := func(cluster string) ([]*SnapshotInfo, error) {
		infos := []*SnapshotInfo(nil)

		it := adminClient.Snapshots(ctx, cluster)
		for {
			s, err := it.Next()
			if err == iterator.Done {
				break
			}
			if err != nil {
				return nil, err
			}
			infos = append(infos, s)
		}
		return infos, err
	}

	// Delete the table at the end of the test. Schedule ahead of time
	// in case the client fails
	defer deleteTable(ctx, t, adminClient, table)

	if err := adminClient.CreateTable(ctx, table); err != nil {
		t.Fatalf("Creating table: %v", err)
	}

	// Precondition: no snapshots
	snapshots, err := list(cluster)
	if err != nil {
		t.Fatalf("Initial snapshot list: %v", err)
	}
	if got, want := len(snapshots), 0; got != want {
		t.Fatalf("Initial snapshot list len: %d, want: %d", got, want)
	}

	// Create snapshot
	defer adminClient.DeleteSnapshot(ctx, cluster, "mysnapshot")

	if err = adminClient.SnapshotTable(ctx, table, cluster, "mysnapshot", 5*time.Hour); err != nil {
		t.Fatalf("Creating snaphot: %v", err)
	}

	// List snapshot
	snapshots, err = list(cluster)
	if err != nil {
		t.Fatalf("Listing snapshots: %v", err)
	}
	if got, want := len(snapshots), 1; got != want {
		t.Fatalf("Listing snapshot count: %d, want: %d", got, want)
	}
	if got, want := snapshots[0].Name, "mysnapshot"; got != want {
		t.Fatalf("Snapshot name: %s, want: %s", got, want)
	}
	if got, want := snapshots[0].SourceTable, table; got != want {
		t.Fatalf("Snapshot SourceTable: %s, want: %s", got, want)
	}
	if got, want := snapshots[0].DeleteTime, snapshots[0].CreateTime.Add(5*time.Hour); math.Abs(got.Sub(want).Minutes()) > 1 {
		t.Fatalf("Snapshot DeleteTime: %s, want: %s", got, want)
	}

	// Get snapshot
	snapshot, err := adminClient.SnapshotInfo(ctx, cluster, "mysnapshot")
	if err != nil {
		t.Fatalf("SnapshotInfo: %v", snapshot)
	}
	if got, want := *snapshot, *snapshots[0]; got != want {
		t.Fatalf("SnapshotInfo: %v, want: %v", got, want)
	}

	// Restore
	restoredTable := table + "-restored"
	defer deleteTable(ctx, t, adminClient, restoredTable)
	if err = adminClient.CreateTableFromSnapshot(ctx, restoredTable, cluster, "mysnapshot"); err != nil {
		t.Fatalf("CreateTableFromSnapshot: %v", err)
	}
	if _, err := adminClient.TableInfo(ctx, restoredTable); err != nil {
		t.Fatalf("Restored TableInfo: %v", err)
	}

	// Delete snapshot
	if err = adminClient.DeleteSnapshot(ctx, cluster, "mysnapshot"); err != nil {
		t.Fatalf("DeleteSnapshot: %v", err)
	}
	snapshots, err = list(cluster)
	if err != nil {
		t.Fatalf("List after Delete: %v", err)
	}
	if got, want := len(snapshots), 0; got != want {
		t.Fatalf("List after delete len: %d, want: %d", got, want)
	}
}

func TestIntegration_Granularity(t *testing.T) {
	testEnv, err := NewIntegrationEnv()
	if err != nil {
		t.Fatalf("IntegrationEnv: %v", err)
	}
	defer testEnv.Close()

	timeout := 2 * time.Second
	if testEnv.Config().UseProd {
		timeout = 5 * time.Minute
	}
	ctx, _ := context.WithTimeout(context.Background(), timeout)

	adminClient, err := testEnv.NewAdminClient()
	if err != nil {
		t.Fatalf("NewAdminClient: %v", err)
	}
	defer adminClient.Close()

	list := func() []string {
		tbls, err := adminClient.Tables(ctx)
		if err != nil {
			t.Fatalf("Fetching list of tables: %v", err)
		}
		sort.Strings(tbls)
		return tbls
	}
	containsAll := func(got, want []string) bool {
		gotSet := make(map[string]bool)

		for _, s := range got {
			gotSet[s] = true
		}
		for _, s := range want {
			if !gotSet[s] {
				return false
			}
		}
		return true
	}

	defer deleteTable(ctx, t, adminClient, "mytable")

	if err := adminClient.CreateTable(ctx, "mytable"); err != nil {
		t.Fatalf("Creating table: %v", err)
	}

	tables := list()
	if got, want := tables, []string{"mytable"}; !containsAll(got, want) {
		t.Errorf("adminClient.Tables returned %#v, want %#v", got, want)
	}

	// calling ModifyColumnFamilies to check the granularity of table
	prefix := adminClient.instancePrefix()
	req := &btapb.ModifyColumnFamiliesRequest{
		Name: prefix + "/tables/" + "mytable",
		Modifications: []*btapb.ModifyColumnFamiliesRequest_Modification{{
			Id:  "cf",
			Mod: &btapb.ModifyColumnFamiliesRequest_Modification_Create{&btapb.ColumnFamily{}},
		}},
	}
	table, err := adminClient.tClient.ModifyColumnFamilies(ctx, req)
	if err != nil {
		t.Fatalf("Creating column family: %v", err)
	}
	if table.Granularity != btapb.Table_TimestampGranularity(btapb.Table_MILLIS) {
		t.Errorf("ModifyColumnFamilies returned granularity %#v, want %#v", table.Granularity, btapb.Table_TimestampGranularity(btapb.Table_MILLIS))
	}
}

func TestIntegration_InstanceAdminClient_AppProfile(t *testing.T) {
	testEnv, err := NewIntegrationEnv()
	if err != nil {
		t.Fatalf("IntegrationEnv: %v", err)
	}
	defer testEnv.Close()

	timeout := 2 * time.Second
	if testEnv.Config().UseProd {
		timeout = 5 * time.Minute
	}
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	adminClient, err := testEnv.NewAdminClient()
	if err != nil {
		t.Fatalf("NewAdminClient: %v", err)
	}
	defer adminClient.Close()

	iAdminClient, err := testEnv.NewInstanceAdminClient()
	if err != nil {
		t.Fatalf("NewInstanceAdminClient: %v", err)
	}

	if iAdminClient == nil {
		return
	}

	defer iAdminClient.Close()
	profile := ProfileConf{
		ProfileID:     "app_profile1",
		InstanceID:    adminClient.instance,
		ClusterID:     testEnv.Config().Cluster,
		Description:   "creating new app profile 1",
		RoutingPolicy: SingleClusterRouting,
	}

	createdProfile, err := iAdminClient.CreateAppProfile(ctx, profile)
	if err != nil {
		t.Fatalf("Creating app profile: %v", err)

	}

	gotProfile, err := iAdminClient.GetAppProfile(ctx, adminClient.instance, "app_profile1")

	if err != nil {
		t.Fatalf("Get app profile: %v", err)
	}

	if !proto.Equal(createdProfile, gotProfile) {
		t.Fatalf("created profile: %s, got profile: %s", createdProfile.Name, gotProfile.Name)

	}

	list := func(instanceID string) ([]*btapb.AppProfile, error) {
		profiles := []*btapb.AppProfile(nil)

		it := iAdminClient.ListAppProfiles(ctx, instanceID)
		for {
			s, err := it.Next()
			if err == iterator.Done {
				break
			}
			if err != nil {
				return nil, err
			}
			profiles = append(profiles, s)
		}
		return profiles, err
	}

	profiles, err := list(adminClient.instance)
	if err != nil {
		t.Fatalf("List app profile: %v", err)
	}

	if got, want := len(profiles), 1; got != want {
		t.Fatalf("Initial app profile list len: %d, want: %d", got, want)
	}

	for _, test := range []struct {
		desc   string
		uattrs ProfileAttrsToUpdate
		want   *btapb.AppProfile // nil means error
	}{
		{
			desc:   "empty update",
			uattrs: ProfileAttrsToUpdate{},
			want:   nil,
		},

		{
			desc:   "empty description update",
			uattrs: ProfileAttrsToUpdate{Description: ""},
			want: &btapb.AppProfile{
				Name:          gotProfile.Name,
				Description:   "",
				RoutingPolicy: gotProfile.RoutingPolicy,
				Etag:          gotProfile.Etag},
		},
		{
			desc: "routing update",
			uattrs: ProfileAttrsToUpdate{
				RoutingPolicy: SingleClusterRouting,
				ClusterID:     testEnv.Config().Cluster,
			},
			want: &btapb.AppProfile{
				Name:        gotProfile.Name,
				Description: "",
				Etag:        gotProfile.Etag,
				RoutingPolicy: &btapb.AppProfile_SingleClusterRouting_{
					SingleClusterRouting: &btapb.AppProfile_SingleClusterRouting{
						ClusterId: testEnv.Config().Cluster,
					}},
			},
		},
	} {
		err = iAdminClient.UpdateAppProfile(ctx, adminClient.instance, "app_profile1", test.uattrs)
		if err != nil {
			if test.want != nil {
				t.Errorf("%s: %v", test.desc, err)
			}
			continue
		}
		if err == nil && test.want == nil {
			t.Errorf("%s: got nil, want error", test.desc)
			continue
		}

		got, _ := iAdminClient.GetAppProfile(ctx, adminClient.instance, "app_profile1")

		if !proto.Equal(got, test.want) {
			t.Fatalf("%s : got profile : %v, want profile: %v", test.desc, gotProfile, test.want)
		}

	}

	err = iAdminClient.DeleteAppProfile(ctx, adminClient.instance, "app_profile1")
	if err != nil {
		t.Fatalf("Delete app profile: %v", err)
	}

}

func TestIntegration_InstanceUpdate(t *testing.T) {
	testEnv, err := NewIntegrationEnv()
	if err != nil {
		t.Fatalf("IntegrationEnv: %v", err)
	}
	defer testEnv.Close()

	timeout := 2 * time.Second
	if testEnv.Config().UseProd {
		timeout = 5 * time.Minute
	}
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	adminClient, err := testEnv.NewAdminClient()
	if err != nil {
		t.Fatalf("NewAdminClient: %v", err)
	}

	defer adminClient.Close()

	iAdminClient, err := testEnv.NewInstanceAdminClient()
	if err != nil {
		t.Fatalf("NewInstanceAdminClient: %v", err)
	}

	if iAdminClient == nil {
		return
	}

	defer iAdminClient.Close()

	iInfo, err := iAdminClient.InstanceInfo(ctx, adminClient.instance)
	if err != nil {
		t.Errorf("InstanceInfo: %v", err)
	}

	if iInfo.Name != adminClient.instance {
		t.Errorf("InstanceInfo returned name %#v, want %#v", iInfo.Name, adminClient.instance)
	}

	if iInfo.DisplayName != adminClient.instance {
		t.Errorf("InstanceInfo returned name %#v, want %#v", iInfo.Name, adminClient.instance)
	}

	const numNodes = 4
	// update cluster nodes
	if err := iAdminClient.UpdateCluster(ctx, adminClient.instance, testEnv.Config().Cluster, int32(numNodes)); err != nil {
		t.Errorf("UpdateCluster: %v", err)
	}

	// get cluster after updating
	cis, err := iAdminClient.GetCluster(ctx, adminClient.instance, testEnv.Config().Cluster)
	if err != nil {
		t.Errorf("GetCluster %v", err)
	}
	if cis.ServeNodes != int(numNodes) {
		t.Errorf("ServeNodes returned %d, want %d", cis.ServeNodes, int(numNodes))
	}
}

func TestIntegration_AdminBackup(t *testing.T) {
	testEnv, err := NewIntegrationEnv()
	if err != nil {
		t.Fatalf("IntegrationEnv: %v", err)
	}
	defer testEnv.Close()

	if !testEnv.Config().UseProd {
		t.Skip("emulator doesn't support backups")
	}

	timeout := 5 * time.Minute
	ctx, _ := context.WithTimeout(context.Background(), timeout)

	adminClient, err := testEnv.NewAdminClient()
	if err != nil {
		t.Fatalf("NewAdminClient: %v", err)
	}
	defer adminClient.Close()

	table := testEnv.Config().Table
	cluster := testEnv.Config().Cluster

	// Delete the table at the end of the test. Schedule ahead of time
	// in case the client fails
	defer deleteTable(ctx, t, adminClient, table)

	list := func(cluster string) ([]*BackupInfo, error) {
		infos := []*BackupInfo(nil)

		it := adminClient.Backups(ctx, cluster)
		for {
			s, err := it.Next()
			if err == iterator.Done {
				break
			}
			if err != nil {
				return nil, err
			}
			infos = append(infos, s)
		}
		return infos, err
	}

	if err := adminClient.CreateTable(ctx, table); err != nil {
		t.Fatalf("Creating table: %v", err)
	}

	// Precondition: no backups
	backups, err := list(cluster)
	if err != nil {
		t.Fatalf("Initial backup list: %v", err)
	}
	if got, want := len(backups), 0; got != want {
		t.Fatalf("Initial backup list len: %d, want: %d", got, want)
	}

	// Create backup
	backupName := "mybackup"
	defer adminClient.DeleteBackup(ctx, cluster, "mybackup")

	if err = adminClient.CreateBackup(ctx, table, cluster, backupName, time.Now().Add(8*time.Hour)); err != nil {
		t.Fatalf("Creating backup: %v", err)
	}

	// List backup
	backups, err = list(cluster)
	if err != nil {
		t.Fatalf("Listing backups: %v", err)
	}
	if got, want := len(backups), 1; got != want {
		t.Fatalf("Listing backup count: %d, want: %d", got, want)
	}
	if got, want := backups[0].Name, backupName; got != want {
		t.Fatalf("Backup name: %s, want: %s", got, want)
	}
	if got, want := backups[0].SourceTable, table; got != want {
		t.Fatalf("Backup SourceTable: %s, want: %s", got, want)
	}
	if got, want := backups[0].ExpireTime, backups[0].StartTime.Add(8*time.Hour); math.Abs(got.Sub(want).Minutes()) > 1 {
		t.Fatalf("Backup ExpireTime: %s, want: %s", got, want)
	}

	// Get backup
	backup, err := adminClient.BackupInfo(ctx, cluster, backupName)
	if err != nil {
		t.Fatalf("BackupInfo: %v", backup)
	}
	if got, want := *backup, *backups[0]; got != want {
		t.Fatalf("BackupInfo: %v, want: %v", got, want)
	}

	// Update backup
	newExpireTime := time.Now().Add(10 * time.Hour)
	err = adminClient.UpdateBackup(ctx, cluster, backupName, newExpireTime)
	if err != nil {
		t.Fatalf("UpdateBackup failed: %v", err)
	}

	// Check that updated backup has the correct expire time
	updatedBackup, err := adminClient.BackupInfo(ctx, cluster, backupName)
	if err != nil {
		t.Fatalf("BackupInfo: %v", err)
	}
	backup.ExpireTime = newExpireTime
	// Server clock and local clock may not be perfectly sync'ed.
	if got, want := *updatedBackup, *backup; got.ExpireTime.Sub(want.ExpireTime) > time.Minute {
		t.Fatalf("BackupInfo: %v, want: %v", got, want)
	}

	// Restore backup
	restoredTable := table + "-restored"
	defer deleteTable(ctx, t, adminClient, restoredTable)
	if err = adminClient.RestoreTable(ctx, restoredTable, cluster, backupName); err != nil {
		t.Fatalf("RestoreTable: %v", err)
	}
	if _, err := adminClient.TableInfo(ctx, restoredTable); err != nil {
		t.Fatalf("Restored TableInfo: %v", err)
	}

	// Delete backup
	if err = adminClient.DeleteBackup(ctx, cluster, backupName); err != nil {
		t.Fatalf("DeleteBackup: %v", err)
	}
	backups, err = list(cluster)
	if err != nil {
		t.Fatalf("List after Delete: %v", err)
	}
	if got, want := len(backups), 0; got != want {
		t.Fatalf("List after delete len: %d, want: %d", got, want)
	}
}

func setupIntegration(ctx context.Context, t *testing.T) (_ IntegrationEnv, _ *Client, _ *AdminClient, table *Table, tableName string, cleanup func(), _ error) {
	testEnv, err := NewIntegrationEnv()
	if err != nil {
		return nil, nil, nil, nil, "", nil, err
	}

	var timeout time.Duration
	if testEnv.Config().UseProd {
		timeout = 10 * time.Minute
		t.Logf("Running test against production")
	} else {
		timeout = 5 * time.Minute
		t.Logf("bttest.Server running on %s", testEnv.Config().AdminEndpoint)
	}
	ctx, cancel := context.WithTimeout(ctx, timeout)

	client, err := testEnv.NewClient()
	if err != nil {
		return nil, nil, nil, nil, "", nil, err
	}

	adminClient, err := testEnv.NewAdminClient()
	if err != nil {
		return nil, nil, nil, nil, "", nil, err
	}

	if testEnv.Config().UseProd {
		// TODO: tables may not be successfully deleted in some cases, and will
		// become obsolete. We may need a way to automatically delete them.
		tableName = tableNameSpace.New()
	} else {
		tableName = testEnv.Config().Table
	}

	if err := adminClient.CreateTable(ctx, tableName); err != nil {
		cancel()
		return nil, nil, nil, nil, "", nil, err
	}
	if err := adminClient.CreateColumnFamily(ctx, tableName, "follows"); err != nil {
		cancel()
		return nil, nil, nil, nil, "", nil, err
	}

	return testEnv, client, adminClient, client.Open(tableName), tableName, func() {
		if err := adminClient.DeleteTable(ctx, tableName); err != nil {
			t.Errorf("DeleteTable got error %v", err)
		}
		cancel()
		client.Close()
		adminClient.Close()
	}, nil
}

func formatReadItem(ri ReadItem) string {
	// Use the column qualifier only to make the test data briefer.
	col := ri.Column[strings.Index(ri.Column, ":")+1:]
	return fmt.Sprintf("%s-%s-%s", ri.Row, col, ri.Value)
}

func fill(b, sub []byte) {
	for len(b) > len(sub) {
		n := copy(b, sub)
		b = b[n:]
	}
}

func clearTimestamps(r Row) {
	for _, ris := range r {
		for i := range ris {
			ris[i].Timestamp = 0
		}
	}
}

func deleteTable(ctx context.Context, t *testing.T, ac *AdminClient, name string) {
	bo := gax.Backoff{
		Initial:    100 * time.Millisecond,
		Max:        2 * time.Second,
		Multiplier: 1.2,
	}
	ctx, _ = context.WithTimeout(ctx, time.Second*30)

	err := internal.Retry(ctx, bo, func() (bool, error) {
		err := ac.DeleteTable(ctx, name)
		if err != nil {
			return true, err
		}
		return false, nil
	})
	if err != nil {
		t.Logf("DeleteTable: %v", err)
	}
}

func verifyDirectPathRemoteAddress(testEnv IntegrationEnv, t *testing.T) {
	t.Helper()
	if !testEnv.Config().AttemptDirectPath {
		return
	}
	if remoteIP, res := isDirectPathRemoteAddress(testEnv); !res {
		if testEnv.Config().DirectPathIPV4Only {
			t.Fatalf("Expect to access DirectPath via ipv4 only, but RPC was destined to %s", remoteIP)
		} else {
			t.Fatalf("Expect to access DirectPath via ipv4 or ipv6, but RPC was destined to %s", remoteIP)
		}
	}
}

func isDirectPathRemoteAddress(testEnv IntegrationEnv) (_ string, _ bool) {
	remoteIP := testEnv.Peer().Addr.String()
	// DirectPath ipv4-only can only use ipv4 traffic.
	if testEnv.Config().DirectPathIPV4Only {
		return remoteIP, strings.HasPrefix(remoteIP, directPathIPV4Prefix)
	}
	// DirectPath ipv6 can use either ipv4 or ipv6 traffic.
	return remoteIP, strings.HasPrefix(remoteIP, directPathIPV4Prefix) || strings.HasPrefix(remoteIP, directPathIPV6Prefix)
}

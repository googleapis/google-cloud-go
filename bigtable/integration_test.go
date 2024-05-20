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
	"log"
	"math"
	"math/rand"
	"os"
	"os/exec"
	"reflect"
	"sort"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"cloud.google.com/go/iam"
	"cloud.google.com/go/internal"
	"cloud.google.com/go/internal/optional"
	"cloud.google.com/go/internal/testutil"
	"cloud.google.com/go/internal/uid"
	"github.com/google/go-cmp/cmp"
	gax "github.com/googleapis/gax-go/v2"
	"google.golang.org/api/iterator"
	btapb "google.golang.org/genproto/googleapis/bigtable/admin/v2"
	grpc "google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/proto"
)

const (
	directPathIPV6Prefix      = "[2001:4860:8040"
	directPathIPV4Prefix      = "34.126"
	timeUntilResourceCleanup  = time.Hour * 12 // 12 hours
	prefixOfInstanceResources = "bt-it-"
	prefixOfClusterResources  = "bt-c-"
)

var (
	presidentsSocialGraph = map[string][]string{
		"wmckinley":   {"tjefferson"},
		"gwashington": {"j§adams"},
		"tjefferson":  {"gwashington", "j§adams"},
		"j§adams":     {"gwashington", "tjefferson"},
	}

	clusterUIDSpace  = uid.NewSpace(prefixOfClusterResources, &uid.Options{Short: true})
	tableNameSpace   = uid.NewSpace("cbt-test", &uid.Options{Short: true})
	myTableName      = fmt.Sprintf("mytable-%d", time.Now().Unix())
	myOtherTableName = fmt.Sprintf("myothertable-%d", time.Now().Unix())
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

func generateNewInstanceName() string {
	return fmt.Sprintf("%v%d", prefixOfInstanceResources, time.Now().Unix())
}

var instanceToCreate string

func init() {
	if runCreateInstanceTests {
		instanceToCreate = generateNewInstanceName()
	}
}

func TestMain(m *testing.M) {
	flag.Parse()

	env, err := NewIntegrationEnv()
	if err != nil {
		panic(fmt.Sprintf("there was an issue creating an integration env: %v", err))
	}
	c := env.Config()
	if c.UseProd {
		fmt.Printf(
			"Note: when using prod, you must first create an instance:\n"+
				"cbt createinstance %s %s %s %s %s SSD\n",
			c.Instance, c.Instance,
			c.Cluster, "us-central1-b", "1",
		)
	}
	exit := m.Run()
	if err := cleanup(c); err != nil {
		log.Printf("Post-test cleanup failed: %v", err)
	}
	os.Exit(exit)
}

func cleanup(c IntegrationTestConfig) error {
	// Cleanup resources marked with bt-it- after a time delay
	if !c.UseProd {
		return nil
	}
	ctx := context.Background()
	iac, err := NewInstanceAdminClient(ctx, c.Project)
	if err != nil {
		return err
	}
	instances, err := iac.Instances(ctx)
	if err != nil {
		return err
	}

	for _, instanceInfo := range instances {
		if strings.HasPrefix(instanceInfo.Name, prefixOfInstanceResources) {
			timestamp := instanceInfo.Name[len(prefixOfInstanceResources):]
			t, err := strconv.ParseInt(timestamp, 10, 64)
			if err != nil {
				return err
			}
			uT := time.Unix(t, 0)
			if time.Now().After(uT.Add(timeUntilResourceCleanup)) {
				iac.DeleteInstance(ctx, instanceInfo.Name)
			}
		} else {
			// Delete clusters created in existing instances
			clusters, err := iac.Clusters(ctx, instanceInfo.Name)
			if err != nil {
				return err
			}
			for _, clusterInfo := range clusters {
				if strings.HasPrefix(clusterInfo.Name, prefixOfClusterResources) {
					iac.DeleteCluster(ctx, instanceInfo.Name, clusterInfo.Name)
				}
			}
		}
	}

	return nil
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
	row, err := table.ReadRow(ctx, "j§adams")
	if err != nil {
		t.Fatalf("Reading a row: %v", err)
	}
	verifyDirectPathRemoteAddress(testEnv, t)
	wantRow := Row{
		"follows": []ReadItem{
			{Row: "j§adams", Column: "follows:gwashington", Timestamp: 1000, Value: []byte("1")},
			{Row: "j§adams", Column: "follows:tjefferson", Timestamp: 1000, Value: []byte("1")},
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
	err = table.ReadRows(ctx, RowRange{}, func(r Row) bool {
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
	keys := RowList{"wmckinley", "gwashington", "j§adams"}
	want := "gwashington-j§adams-1,j§adams-gwashington-1,j§adams-tjefferson-1,wmckinley-tjefferson-1"
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
func TestIntegration_ReadRowListReverse(t *testing.T) {
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
	rowRange := NewOpenClosedRange("gwashington", "wmckinley")
	want := "wmckinley-tjefferson-1,tjefferson-gwashington-1,tjefferson-j§adams-1,j§adams-gwashington-1,j§adams-tjefferson-1"
	err = table.ReadRows(ctx, rowRange, func(r Row) bool {
		for _, ris := range r {
			for _, ri := range ris {
				elt = append(elt, formatReadItem(ri))
			}
		}
		return true
	}, ReverseScan())

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

	if !testEnv.Config().UseProd {
		t.Skip("Skip long running tests in short mode")
	}

	ts := uid.NewSpace("ts", &uid.Options{Short: true}).New()
	if err := adminClient.CreateColumnFamily(ctx, tableName, ts); err != nil {
		t.Fatalf("Creating column family: %v", err)
	}

	bigBytes := make([]byte, 5<<20) // 5 MB is larger than current default gRPC max of 4 MB, but less than the max we set.
	nonsense := []byte("lorem ipsum dolor sit amet, ")
	fill(bigBytes, nonsense)
	mut := NewMutation()
	mut.Set(ts, "col", 1000, bigBytes)
	if err := table.Apply(ctx, "bigrow", mut); err != nil {
		t.Fatalf("Big write: %v", err)
	}
	verifyDirectPathRemoteAddress(testEnv, t)
	r, err := table.ReadRow(ctx, "bigrow")
	if err != nil {
		t.Fatalf("Big read: %v", err)
	}
	verifyDirectPathRemoteAddress(testEnv, t)
	wantRow := Row{ts: []ReadItem{
		{Row: "bigrow", Column: fmt.Sprintf("%s:col", ts), Timestamp: 1000, Value: bigBytes},
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
		mut.Set(ts, "big-scan", 1000, medBytes)
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
		t.Fatalf("Scan with row end returned %d rows, want %d", rc, wantRc)
	}

	// Test bulk mutations
	bulk := uid.NewSpace("bulk", &uid.Options{Short: true}).New()
	if err := adminClient.CreateColumnFamily(ctx, tableName, bulk); err != nil {
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
			mut.Set(bulk, name, 1000, []byte("1"))
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
			c := fmt.Sprintf("%s:%s", bulk, val)
			wantItems = append(wantItems, ReadItem{Row: rowKey, Column: c, Timestamp: 1000, Value: []byte("1")})
		}
		wantRow := Row{bulk: wantItems}
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
		"gwashington": {"j§adams"},
		"tjefferson":  {"gwashington", "j§adams", "wmckinley"},
		"j§adams":     {"gwashington", "tjefferson"},
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

func TestIntegration_FullReadStats(t *testing.T) {
	ctx := context.Background()
	testEnv, _, _, table, _, cleanup, err := setupIntegration(ctx, t)
	if err != nil {
		t.Fatal(err)
	}
	defer cleanup()

	// Insert some data.
	initialData := map[string][]string{
		"wmckinley":   {"tjefferson"},
		"gwashington": {"j§adams"},
		"tjefferson":  {"gwashington", "j§adams", "wmckinley"},
		"j§adams":     {"gwashington", "tjefferson"},
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
		desc        string
		rr          RowSet
		filter      Filter     // may be nil
		limit       ReadOption // may be nil
		reverseScan bool

		// We do the read and grab all the stats.
		cellsReturnedCount int64
		rowsReturnedCount  int64
	}{
		{
			desc:               "read all, unfiltered",
			rr:                 RowRange{},
			cellsReturnedCount: 7,
			rowsReturnedCount:  4,
		},
		{
			desc:               "read with InfiniteRange, unfiltered",
			rr:                 InfiniteRange("tjefferson"),
			cellsReturnedCount: 4,
			rowsReturnedCount:  2,
		},
		{
			desc:               "read with NewRange, unfiltered",
			rr:                 NewRange("gargamel", "hubbard"),
			cellsReturnedCount: 1,
			rowsReturnedCount:  1,
		},
		{
			desc:               "read with NewRange, no results",
			rr:                 NewRange("zany", "zebra"), // no matches
			cellsReturnedCount: 0,
			rowsReturnedCount:  0,
		},
		{
			desc:               "read with PrefixRange, unfiltered",
			rr:                 PrefixRange("j§ad"),
			cellsReturnedCount: 2,
			rowsReturnedCount:  1,
		},
		{
			desc:               "read with SingleRow, unfiltered",
			rr:                 SingleRow("wmckinley"),
			cellsReturnedCount: 1,
			rowsReturnedCount:  1,
		},
		{
			desc:               "read all, with ColumnFilter",
			rr:                 RowRange{},
			filter:             ColumnFilter(".*j.*"), // matches "j§adams" and "tjefferson"
			cellsReturnedCount: 4,
			rowsReturnedCount:  4,
		},
		{
			desc:               "read all, with ColumnFilter, prefix",
			rr:                 RowRange{},
			filter:             ColumnFilter("j"), // no matches
			cellsReturnedCount: 0,
			rowsReturnedCount:  0,
		},
		{
			desc:               "read range, with ColumnRangeFilter",
			rr:                 RowRange{},
			filter:             ColumnRangeFilter("follows", "h", "k"),
			cellsReturnedCount: 2,
			rowsReturnedCount:  2,
		},
		{
			desc:               "read range from empty, with ColumnRangeFilter",
			rr:                 RowRange{},
			filter:             ColumnRangeFilter("follows", "", "u"),
			cellsReturnedCount: 6,
			rowsReturnedCount:  4,
		},
		{
			desc:               "read range from start to empty, with ColumnRangeFilter",
			rr:                 RowRange{},
			filter:             ColumnRangeFilter("follows", "h", ""),
			cellsReturnedCount: 5,
			rowsReturnedCount:  4,
		},
		{
			desc:               "read with RowKeyFilter",
			rr:                 RowRange{},
			filter:             RowKeyFilter(".*wash.*"),
			cellsReturnedCount: 1,
			rowsReturnedCount:  1,
		},
		{
			desc:               "read with RowKeyFilter unicode",
			rr:                 RowRange{},
			filter:             RowKeyFilter(".*j§.*"),
			cellsReturnedCount: 2,
			rowsReturnedCount:  1,
		},
		{
			desc:               "read with RowKeyFilter escaped",
			rr:                 RowRange{},
			filter:             RowKeyFilter(`.*j\xC2\xA7.*`),
			cellsReturnedCount: 2,
			rowsReturnedCount:  1,
		},
		{
			desc:               "read with RowKeyFilter, prefix",
			rr:                 RowRange{},
			filter:             RowKeyFilter("gwash"),
			cellsReturnedCount: 0,
			rowsReturnedCount:  0,
		},
		{
			desc:               "read with RowKeyFilter, no matches",
			rr:                 RowRange{},
			filter:             RowKeyFilter(".*xxx.*"),
			cellsReturnedCount: 0,
			rowsReturnedCount:  0,
		},
		{
			desc:               "read with FamilyFilter, no matches",
			rr:                 RowRange{},
			filter:             FamilyFilter(".*xxx.*"),
			cellsReturnedCount: 0,
			rowsReturnedCount:  0,
		},
		{
			desc:               "read with ColumnFilter + row end",
			rr:                 RowRange{},
			filter:             ColumnFilter(".*j.*"), // matches "j§adams" and "tjefferson"
			limit:              LimitRows(2),
			cellsReturnedCount: 2,
			rowsReturnedCount:  2,
		},
		{
			desc:               "apply labels to the result rows",
			rr:                 RowRange{},
			filter:             LabelFilter("test-label"),
			limit:              LimitRows(2),
			cellsReturnedCount: 3,
			rowsReturnedCount:  2,
		},
		{
			desc:               "read all, strip values",
			rr:                 RowRange{},
			filter:             StripValueFilter(),
			cellsReturnedCount: 7,
			rowsReturnedCount:  4,
		},
		{
			desc:               "read with ColumnFilter + row end + strip values",
			rr:                 RowRange{},
			filter:             ChainFilters(ColumnFilter(".*j.*"), StripValueFilter()), // matches "j§adams" and "tjefferson"
			limit:              LimitRows(2),
			cellsReturnedCount: 2,
			rowsReturnedCount:  2,
		},
		{
			desc:               "read with condition, strip values on true",
			rr:                 RowRange{},
			filter:             ConditionFilter(ColumnFilter(".*j.*"), StripValueFilter(), nil),
			cellsReturnedCount: 7,
			rowsReturnedCount:  4,
		},
		{
			desc:               "read with condition, strip values on false",
			rr:                 RowRange{},
			filter:             ConditionFilter(ColumnFilter(".*xxx.*"), nil, StripValueFilter()),
			cellsReturnedCount: 7,
			rowsReturnedCount:  4,
		},
		{
			desc:               "read with ValueRangeFilter + row end",
			rr:                 RowRange{},
			filter:             ValueRangeFilter([]byte("1"), []byte("5")), // matches our value of "1"
			limit:              LimitRows(2),
			cellsReturnedCount: 3,
			rowsReturnedCount:  2,
		},
		{
			desc:               "read with ValueRangeFilter, no match on exclusive end",
			rr:                 RowRange{},
			filter:             ValueRangeFilter([]byte("0"), []byte("1")), // no match
			cellsReturnedCount: 0,
			rowsReturnedCount:  0,
		},
		{
			desc:               "read with ValueRangeFilter, no matches",
			rr:                 RowRange{},
			filter:             ValueRangeFilter([]byte("3"), []byte("5")), // matches nothing
			cellsReturnedCount: 0,
			rowsReturnedCount:  0,
		},
		{
			desc:               "read with InterleaveFilter, no matches on all filters",
			rr:                 RowRange{},
			filter:             InterleaveFilters(ColumnFilter(".*x.*"), ColumnFilter(".*z.*")),
			cellsReturnedCount: 0,
			rowsReturnedCount:  0,
		},
		{
			desc:               "read with InterleaveFilter, no duplicate cells",
			rr:                 RowRange{},
			filter:             InterleaveFilters(ColumnFilter(".*g.*"), ColumnFilter(".*j.*")),
			cellsReturnedCount: 6,
			rowsReturnedCount:  4,
		},
		{
			desc:               "read with InterleaveFilter, with duplicate cells",
			rr:                 RowRange{},
			filter:             InterleaveFilters(ColumnFilter(".*g.*"), ColumnFilter(".*g.*")),
			cellsReturnedCount: 4,
			rowsReturnedCount:  2,
		},
		{
			desc:               "read with a RowRangeList and no filter",
			rr:                 RowRangeList{NewRange("gargamel", "hubbard"), InfiniteRange("wmckinley")},
			cellsReturnedCount: 2,
			rowsReturnedCount:  2,
		},
		{
			desc:               "chain that excludes rows and matches nothing, in a condition",
			rr:                 RowRange{},
			filter:             ConditionFilter(ChainFilters(ColumnFilter(".*j.*"), ColumnFilter(".*mckinley.*")), StripValueFilter(), nil),
			cellsReturnedCount: 0,
			rowsReturnedCount:  0,
		},
		{
			desc:               "chain that ends with an interleave that has no match. covers #804",
			rr:                 RowRange{},
			filter:             ConditionFilter(ChainFilters(ColumnFilter(".*j.*"), InterleaveFilters(ColumnFilter(".*x.*"), ColumnFilter(".*z.*"))), StripValueFilter(), nil),
			cellsReturnedCount: 0,
			rowsReturnedCount:  0,
		},
		{
			desc:               "reverse read all, unfiltered",
			rr:                 RowRange{},
			reverseScan:        true,
			cellsReturnedCount: 7,
			rowsReturnedCount:  4,
		},
		{
			desc:               "reverse read with InfiniteRange, unfiltered",
			rr:                 InfiniteReverseRange("wmckinley"),
			reverseScan:        true,
			cellsReturnedCount: 7,
			rowsReturnedCount:  4,
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
			if test.reverseScan {
				opts = append(opts, ReverseScan())
			}
			// Define a callback for validating request stats.
			callbackInvoked := false
			statsValidator := WithFullReadStats(
				func(stats *FullReadStats) {
					if callbackInvoked {
						t.Fatalf("The request stats callback was invoked more than once. It should be invoked exactly once.")
					}
					readStats := stats.ReadIterationStats
					callbackInvoked = true
					if readStats.CellsReturnedCount != test.cellsReturnedCount {
						t.Errorf("CellsReturnedCount did not match. got: %d, want: %d",
							readStats.CellsReturnedCount, test.cellsReturnedCount)
					}
					if readStats.RowsReturnedCount != test.rowsReturnedCount {
						t.Errorf("RowsReturnedCount did not match. got: %d, want: %d",
							readStats.RowsReturnedCount, test.rowsReturnedCount)
					}
					// We use lenient checks for CellsSeenCount and RowsSeenCount. Exact checks would be brittle.
					// Note that the emulator and prod sometimes yield different values:
					// - Sometimes prod scans fewer cells due to optimizations that allow prod to skip cells.
					// - Sometimes prod scans more cells due to filters that must rescan cells.
					// Similar issues apply for RowsSeenCount.
					if got, want := readStats.CellsSeenCount, readStats.CellsReturnedCount; got < want {
						t.Errorf("CellsSeenCount should be greater than or equal to CellsReturnedCount. got: %d < want: %d",
							got, want)
					}
					if got, want := readStats.RowsSeenCount, readStats.RowsReturnedCount; got < want {
						t.Errorf("RowsSeenCount should be greater than or equal to RowsReturnedCount. got: %d < want: %d",
							got, want)
					}
				})
			opts = append(opts, statsValidator)

			err := table.ReadRows(ctx, test.rr, func(r Row) bool { return true }, opts...)
			if err != nil {
				t.Fatal(err)
			}
			if !callbackInvoked {
				t.Fatalf("The request stats callback was not invoked. It should be invoked exactly once.")
			}
			verifyDirectPathRemoteAddress(testEnv, t)
		})
	}
}

func TestIntegration_SampleRowKeys(t *testing.T) {
	ctx := context.Background()
	testEnv, client, adminClient, _, _, cleanup, err := setupIntegration(ctx, t)
	if err != nil {
		t.Fatal(err)
	}
	defer cleanup()

	presplitTable := fmt.Sprintf("presplit-table-%d", time.Now().Unix())
	if err := adminClient.CreatePresplitTable(ctx, presplitTable, []string{"follows"}); err != nil {
		t.Fatal(err)
	}
	defer adminClient.DeleteTable(ctx, presplitTable)

	cf := uid.NewSpace("follows", &uid.Options{Short: true}).New()
	if err := adminClient.CreateColumnFamily(ctx, presplitTable, cf); err != nil {
		t.Fatal(err)
	}

	table := client.Open(presplitTable)

	// Insert some data.
	initialData := map[string][]string{
		"wmckinley11":   {"tjefferson11"},
		"gwashington77": {"j§adams77"},
		"tjefferson0":   {"gwashington0", "j§adams0"},
	}

	for row, ss := range initialData {
		mut := NewMutation()
		for _, name := range ss {
			mut.Set(cf, name, 1000, []byte("1"))
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

// testing if deletionProtection works properly e.g. when set to Protected, column family and table cannot be deleted;
// then update the deletionProtection to Unprotected and check if deleting the column family and table works properly.
func TestIntegration_TableDeletionProtection(t *testing.T) {
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

	tableConf := TableConf{
		TableID: myTableName,
		Families: map[string]GCPolicy{
			"fam1": MaxVersionsPolicy(1),
			"fam2": MaxVersionsPolicy(2),
		},
		DeletionProtection: Protected,
	}

	if err := adminClient.CreateTableFromConf(ctx, &tableConf); err != nil {
		t.Fatalf("Create table from config: %v", err)
	}

	table, err := adminClient.TableInfo(ctx, myTableName)
	if err != nil {
		t.Fatalf("Getting table info: %v", err)
	}

	if table.DeletionProtection != Protected {
		t.Errorf("Expect table deletion protection to be enabled for table: %v", myTableName)
	}

	// Check if the deletion protection works properly
	if err = adminClient.DeleteColumnFamily(ctx, tableConf.TableID, "fam1"); err == nil {
		t.Errorf("We shouldn't be able to delete the column family fam1 when the deletion protection is enabled for table %v", myTableName)
	}
	if err = adminClient.DeleteTable(ctx, tableConf.TableID); err == nil {
		t.Errorf("We shouldn't be able to delete the table when the deletion protection is enabled for table %v", myTableName)
	}

	if err := adminClient.UpdateTableWithDeletionProtection(ctx, myTableName, Unprotected); err != nil {
		t.Fatalf("Update table from config: %v", err)
	}

	table, err = adminClient.TableInfo(ctx, myTableName)
	if err != nil {
		t.Fatalf("Getting table info: %v", err)
	}

	if table.DeletionProtection != Unprotected {
		t.Errorf("Expect table deletion protection to be disabled for table: %v", myTableName)
	}

	if err := adminClient.DeleteColumnFamily(ctx, tableConf.TableID, "fam1"); err != nil {
		t.Errorf("Delete column family does not work properly while deletion protection bit is disabled: %v", err)
	}
	if err = adminClient.DeleteTable(ctx, tableConf.TableID); err != nil {
		t.Errorf("Deleting the table does not work properly while deletion protection bit is disabled: %v", err)
	}
}

// testing if change stream works properly i.e. can create table with change
// stream and disable change stream on existing table and delete fails if change
// stream is enabled.
func TestIntegration_EnableChangeStream(t *testing.T) {
	t.Skip("https://github.com/googleapis/google-cloud-go/issues/8266")
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

	changeStreamRetention, err := time.ParseDuration("24h")
	if err != nil {
		t.Fatalf("ChangeStreamRetention not valid: %v", err)
	}

	tableConf := TableConf{
		TableID: myTableName,
		Families: map[string]GCPolicy{
			"fam1": MaxVersionsPolicy(1),
			"fam2": MaxVersionsPolicy(2),
		},
		ChangeStreamRetention: changeStreamRetention,
	}

	if err := adminClient.CreateTableFromConf(ctx, &tableConf); err != nil {
		t.Fatalf("Create table from config: %v", err)
	}

	table, err := adminClient.TableInfo(ctx, myTableName)
	if err != nil {
		t.Fatalf("Getting table info: %v", err)
	}

	if table.ChangeStreamRetention != changeStreamRetention {
		t.Errorf("Expect table change stream to be enabled for table: %v has info: %v", myTableName, table)
	}

	// Update retention
	changeStreamRetention, err = time.ParseDuration("70h")
	if err != nil {
		t.Fatalf("ChangeStreamRetention not valid: %v", err)
	}

	if err := adminClient.UpdateTableWithChangeStream(ctx, myTableName, changeStreamRetention); err != nil {
		t.Fatalf("Update table from config: %v", err)
	}

	table, err = adminClient.TableInfo(ctx, myTableName)
	if err != nil {
		t.Fatalf("Getting table info: %v", err)
	}

	if table.ChangeStreamRetention != changeStreamRetention {
		t.Errorf("Expect table change stream to be enabled for table: %v has info: %v", myTableName, table)
	}

	// Disable change stream
	if err := adminClient.UpdateTableDisableChangeStream(ctx, myTableName); err != nil {
		t.Fatalf("Update table from config: %v", err)
	}

	table, err = adminClient.TableInfo(ctx, myTableName)
	if err != nil {
		t.Fatalf("Getting table info: %v", err)
	}

	if table.ChangeStreamRetention != nil {
		t.Errorf("Expect table change stream to be disabled for table: %v has info: %v", myTableName, table)
	}

	if err = adminClient.DeleteTable(ctx, tableConf.TableID); err != nil {
		t.Errorf("Deleting the table failed when change stream is disabled: %v", err)
	}
}

func equalOptionalDuration(a, b optional.Duration) bool {
	if a == nil && b == nil {
		return true
	}
	if a == nil || b == nil {
		return false
	}
	return int64(a.(time.Duration).Seconds()) == int64(b.(time.Duration).Seconds())
}

// Testing if automated backups works properly i.e.
// - Can create table with Automated Backups configured
// - Can update Automated Backup Policy on an existing table
// - Can disable Automated Backups on an existing table
func TestIntegration_AutomatedBackups(t *testing.T) {
	testEnv, err := NewIntegrationEnv()
	if err != nil {
		t.Fatalf("IntegrationEnv: %v", err)
	}
	defer testEnv.Close()

	if !testEnv.Config().UseProd {
		t.Skip("emulator doesn't support Automated Backups")
	}

	timeout := 5 * time.Minute
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	adminClient, err := testEnv.NewAdminClient()
	if err != nil {
		t.Fatalf("NewAdminClient: %v", err)
	}
	defer adminClient.Close()

	retentionPeriod, err := time.ParseDuration("72h")
	if err != nil {
		t.Fatalf("RetentionPeriod not valid: %v", err)
	}
	frequency, err := time.ParseDuration("24h")
	if err != nil {
		t.Fatalf("Frequency not valid: %v", err)
	}
	automatedBackupPolicy := TableAutomatedBackupPolicy{RetentionPeriod: retentionPeriod, Frequency: frequency}

	tableConf := TableConf{
		TableID: myTableName,
		Families: map[string]GCPolicy{
			"fam1": MaxVersionsPolicy(1),
			"fam2": MaxVersionsPolicy(2),
		},
		AutomatedBackupConfig: &automatedBackupPolicy,
	}

	if err := adminClient.CreateTableFromConf(ctx, &tableConf); err != nil {
		t.Fatalf("Create table from config: %v", err)
	}
	defer deleteTable(ctx, t, adminClient, myTableName)

	table, err := adminClient.TableInfo(ctx, myTableName)
	if err != nil {
		t.Fatalf("Getting table info: %v", err)
	}

	if table.AutomatedBackupConfig == nil {
		t.Errorf("Expect Automated Backup Policy to be enabled for table: %v has info: %v", myTableName, table)
	}
	tableAbp := table.AutomatedBackupConfig.(*TableAutomatedBackupPolicy)
	if !equalOptionalDuration(tableAbp.Frequency, automatedBackupPolicy.Frequency) {
		t.Errorf("Expect automated backup policy frequency to be set for table: %v has info: %v", myTableName, table)
	}
	if !equalOptionalDuration(tableAbp.RetentionPeriod, automatedBackupPolicy.RetentionPeriod) {
		t.Errorf("Expect automated backup policy retention period to be set for table: %v has info: %v", myTableName, table)
	}

	// Test update automated backup policy
	retentionPeriod, err = time.ParseDuration("72h")
	if err != nil {
		t.Fatalf("RetentionPeriod not valid: %v", err)
	}
	frequency, err = time.ParseDuration("24h")
	if err != nil {
		t.Fatalf("Frequency not valid: %v", err)
	}
	for _, testcase := range []struct {
		desc      string
		bkpPolicy TableAutomatedBackupPolicy
	}{
		{
			desc:      "Update automated backup policy, just frequency",
			bkpPolicy: TableAutomatedBackupPolicy{Frequency: frequency},
		},
		{
			desc:      "Update automated backup policy, just retention period",
			bkpPolicy: TableAutomatedBackupPolicy{RetentionPeriod: retentionPeriod},
		},
		{
			desc:      "Update automated backup policy, all fields",
			bkpPolicy: TableAutomatedBackupPolicy{RetentionPeriod: retentionPeriod, Frequency: frequency},
		},
	} {
		if gotErr := adminClient.UpdateTableWithAutomatedBackupPolicy(ctx, myTableName, testcase.bkpPolicy); err != nil {
			t.Fatalf("%v: Update table from config: %v", testcase.desc, gotErr)
		}

		gotTable, gotErr := adminClient.TableInfo(ctx, myTableName)
		if gotErr != nil {
			t.Fatalf("%v: Getting table info: %v", testcase.desc, gotErr)
		}
		if gotTable.AutomatedBackupConfig == nil {
			t.Errorf("%v: Expect Automated Backup Policy to be enabled for table: %v has info: %v", testcase.desc, myTableName, gotTable)
		}

		gotTableAbp := gotTable.AutomatedBackupConfig.(*TableAutomatedBackupPolicy)
		if testcase.bkpPolicy.Frequency != nil && !equalOptionalDuration(gotTableAbp.Frequency, testcase.bkpPolicy.Frequency) {
			t.Errorf("%v: Expect automated backup policy frequency to be set for table: %v has info: %v", testcase.desc, myTableName, table)
		}
		if testcase.bkpPolicy.RetentionPeriod != nil && !equalOptionalDuration(gotTableAbp.RetentionPeriod, testcase.bkpPolicy.RetentionPeriod) {
			t.Errorf("%v: Expect automated backup policy retention period to be set for table: %v has info: %v", testcase.desc, myTableName, table)
		}
	}

	// Test disable automated backups
	if err := adminClient.UpdateTableDisableAutomatedBackupPolicy(ctx, myTableName); err != nil {
		t.Fatalf("Update table from config: %v", err)
	}

	table, err = adminClient.TableInfo(ctx, myTableName)
	if err != nil {
		t.Fatalf("Getting table info: %v", err)
	}

	if table.AutomatedBackupConfig != nil {
		t.Errorf("Expect table automated backups to be disabled for table: %v has info: %v", myTableName, table)
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

	defer deleteTable(ctx, t, adminClient, myTableName)

	if err := adminClient.CreateTable(ctx, myTableName); err != nil {
		t.Fatalf("Creating table: %v", err)
	}

	defer deleteTable(ctx, t, adminClient, myOtherTableName)

	if err := adminClient.CreateTable(ctx, myOtherTableName); err != nil {
		t.Fatalf("Creating table: %v", err)
	}

	if got, want := list(), []string{myOtherTableName, myTableName}; !containsAll(got, want) {
		t.Errorf("adminClient.Tables returned %#v, want %#v", got, want)
	}

	must(adminClient.WaitForReplication(ctx, myTableName))

	if err := adminClient.DeleteTable(ctx, myOtherTableName); err != nil {
		t.Fatalf("Deleting table: %v", err)
	}
	tables := list()
	if got, want := tables, []string{myTableName}; !containsAll(got, want) {
		t.Errorf("adminClient.Tables returned %#v, want %#v", got, want)
	}
	if got, unwanted := tables, []string{myOtherTableName}; containsAll(got, unwanted) {
		t.Errorf("adminClient.Tables return %#v. unwanted %#v", got, unwanted)
	}

	uniqueID := make([]byte, 4)
	rand.Read(uniqueID)
	tableID := fmt.Sprintf("conftable%x", uniqueID)

	tblConf := TableConf{
		TableID: tableID,
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
	if err = adminClient.CreateColumnFamily(ctx, myTableName, "cf"); err != nil {
		t.Fatalf("Creating column family: %v", err)
	}

	client, err := testEnv.NewClient()
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}
	defer client.Close()

	tbl := client.Open(myTableName)

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

	if err = adminClient.DropRowRange(ctx, myTableName, "a"); err != nil {
		t.Errorf("DropRowRange a: %v", err)
	}
	if err = adminClient.DropRowRange(ctx, myTableName, "c"); err != nil {
		t.Errorf("DropRowRange c: %v", err)
	}
	if err = adminClient.DropRowRange(ctx, myTableName, "x"); err != nil {
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

	if err = adminClient.DropAllRows(ctx, myTableName); err != nil {
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

	// Validate Encryption Info configured to default. (not supported by emulator)
	if testEnv.Config().UseProd {
		encryptionInfo, err := adminClient.EncryptionInfo(ctx, myTableName)
		if err != nil {
			t.Fatalf("EncryptionInfo: %v", err)
		}
		wantLen := 1
		if testEnv.Config().Cluster2 != "" {
			wantLen++
		}

		if got, want := len(encryptionInfo), wantLen; !cmp.Equal(got, want) {
			t.Fatalf("Number of Clusters with Encryption Info: %v, want: %v", got, want)
		}

		clusterEncryptionInfo := encryptionInfo[testEnv.Config().Cluster][0]
		if clusterEncryptionInfo.KMSKeyVersion != "" {
			t.Errorf("Encryption Info mismatch, got %v, want %v", clusterEncryptionInfo.KMSKeyVersion, 0)
		}
		if clusterEncryptionInfo.Type != GoogleDefaultEncryption {
			t.Errorf("Encryption Info mismatch, got %v, want %v", clusterEncryptionInfo.Type, GoogleDefaultEncryption)
		}
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
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	adminClient, err := testEnv.NewAdminClient()
	if err != nil {
		t.Fatalf("NewAdminClient: %v", err)
	}
	defer adminClient.Close()

	defer deleteTable(ctx, t, adminClient, myTableName)
	if err := adminClient.CreateTable(ctx, myTableName); err != nil {
		t.Fatalf("Creating table: %v", err)
	}

	// Verify that the IAM Controls work for Tables.
	iamHandle := adminClient.TableIAM(myTableName)
	p, err := iamHandle.Policy(ctx)
	if err != nil {
		t.Fatalf("Iam GetPolicy mytable: %v", err)
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
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

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
	opts := &uid.Options{Sep: '_'}
	backupUUID := uid.NewSpace("backup", opts)
	backup := backupUUID.New()

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

func TestIntegration_AuthorizedViewIAM(t *testing.T) {
	testEnv, err := NewIntegrationEnv()
	if err != nil {
		t.Fatalf("IntegrationEnv: %v", err)
	}
	defer testEnv.Close()

	if !testEnv.Config().UseProd {
		t.Skip("emulator doesn't support IAM Policy creation")
	}
	timeout := 5 * time.Minute
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	adminClient, err := testEnv.NewAdminClient()
	if err != nil {
		t.Fatalf("NewAdminClient: %v", err)
	}
	defer adminClient.Close()

	table := testEnv.Config().Table

	defer deleteTable(ctx, t, adminClient, table)
	if err := adminClient.CreateTable(ctx, table); err != nil {
		t.Fatalf("Creating table: %v", err)
	}

	// Create authorized view.
	opts := &uid.Options{Sep: '_'}
	authorizedViewUUID := uid.NewSpace("authorizedView", opts)
	authorizedView := authorizedViewUUID.New()

	defer adminClient.DeleteAuthorizedView(ctx, table, authorizedView)

	if err = adminClient.CreateAuthorizedView(ctx, &AuthorizedViewConf{
		TableID:            table,
		AuthorizedViewID:   authorizedView,
		AuthorizedView:     &SubsetViewConf{},
		DeletionProtection: Unprotected,
	}); err != nil {
		t.Fatalf("Creating authorizedView: %v", err)
	}
	iamHandle := adminClient.AuthorizedViewIAM(table, authorizedView)
	// Get authorized view policy.
	p, err := iamHandle.Policy(ctx)
	if err != nil {
		t.Errorf("iamHandle.Policy: %v", err)
	}
	// The resource is new, so the policy should be empty.
	if got := p.Roles(); len(got) > 0 {
		t.Errorf("got roles %v, want none", got)
	}
	// Set authorized view policy.
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
	// Test authorized view permissions.
	permissions := []string{"bigtable.authorizedViews.get", "bigtable.authorizedViews.update"}
	_, err = iamHandle.TestPermissions(ctx, permissions)
	if err != nil {
		t.Errorf("iamHandle.TestPermissions: %v", err)
	}
}

func TestIntegration_AdminCreateInstance(t *testing.T) {
	if instanceToCreate == "" {
		t.Skip("instanceToCreate not set, skipping instance creation testing")
	}
	instanceToCreate += "0"

	testEnv, err := NewIntegrationEnv()
	if err != nil {
		t.Fatalf("IntegrationEnv: %v", err)
	}
	defer testEnv.Close()

	if !testEnv.Config().UseProd {
		t.Skip("emulator doesn't support instance creation")
	}

	timeout := 7 * time.Minute
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

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

	// CreateInstance can be flaky; retry 3 times before marking as failing.
	testutil.Retry(t, 3, 5*time.Second, func(r *testutil.R) {
		if err := iAdminClient.CreateInstance(ctx, conf); err != nil {
			t.Fatalf("CreateInstance: %v", err)
		}
	})

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
			{ClusterID: clusterID, NumNodes: 5},
		},
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

	if cInfo.KMSKeyName != "" {
		t.Fatalf("KMSKeyName: %v, want: %v", cInfo.KMSKeyName, "")
	}
}

func TestIntegration_AdminEncryptionInfo(t *testing.T) {
	if instanceToCreate == "" {
		t.Skip("instanceToCreate not set, skipping instance creation testing")
	}
	instanceToCreate += "1"

	testEnv, err := NewIntegrationEnv()
	if err != nil {
		t.Fatalf("IntegrationEnv: %v", err)
	}
	defer testEnv.Close()

	if !testEnv.Config().UseProd {
		t.Skip("emulator doesn't support instance creation")
	}

	// adjust test environment to use our cluster to create
	c := testEnv.Config()
	c.Instance = instanceToCreate
	testEnv, err = NewProdEnv(c)
	if err != nil {
		t.Fatalf("NewProdEnv: %v", err)
	}

	timeout := 10 * time.Minute
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	iAdminClient, err := testEnv.NewInstanceAdminClient()
	if err != nil {
		t.Fatalf("NewInstanceAdminClient: %v", err)
	}
	defer iAdminClient.Close()

	adminClient, err := testEnv.NewAdminClient()
	if err != nil {
		t.Fatalf("NewAdminClient: %v", err)
	}
	defer adminClient.Close()

	table := instanceToCreate + "-table"
	clusterID := instanceToCreate + "-cluster"

	keyRingName := os.Getenv("GCLOUD_TESTS_BIGTABLE_KEYRING")
	if keyRingName == "" {
		// try to fall back on GOLANG keyring
		keyRingName = os.Getenv("GCLOUD_TESTS_GOLANG_KEYRING")
		if keyRingName == "" {
			t.Fatal("GCLOUD_TESTS_BIGTABLE_KEYRING or GCLOUD_TESTS_GOLANG_KEYRING must be set. See CONTRIBUTING.md for details")
		}
	}
	kmsKeyName := keyRingName + "/cryptoKeys/key1"

	conf := &InstanceWithClustersConfig{
		InstanceID:  instanceToCreate,
		DisplayName: "test instance",
		Clusters: []ClusterConfig{
			{
				ClusterID:  clusterID,
				KMSKeyName: kmsKeyName,
				Zone:       instanceToCreateZone,
				NumNodes:   1,
			},
		},
	}
	if err := iAdminClient.CreateInstanceWithClusters(ctx, conf); err != nil {
		t.Fatalf("CreateInstance: %v", err)
	}
	defer iAdminClient.DeleteInstance(ctx, instanceToCreate)

	// Delete the table at the end of the test. Schedule ahead of time
	// in case the client fails
	defer deleteTable(ctx, t, adminClient, table)
	if err := adminClient.CreateTable(ctx, table); err != nil {
		t.Fatalf("Creating table: %v", err)
	}

	var encryptionKeyVersion string

	// The encryption info can take 30-500s (currently about 120-190s) to
	// become ready.
	for i := 0; i < 50; i++ {
		encryptionInfo, err := adminClient.EncryptionInfo(ctx, table)
		if err != nil {
			t.Fatalf("EncryptionInfo: %v", err)
		}

		encryptionKeyVersion = encryptionInfo[clusterID][0].KMSKeyVersion
		if encryptionKeyVersion != "" {
			break
		}

		time.Sleep(time.Second * 10)
	}
	if encryptionKeyVersion == "" {
		t.Fatalf("Encryption Key not created within alotted time end")
	}

	// Validate Encryption Info under getTable
	table2, err := adminClient.getTable(ctx, table, btapb.Table_ENCRYPTION_VIEW)
	if err != nil {
		t.Fatalf("Getting Table: %v", err)
	}
	if got, want := len(table2.ClusterStates), 1; !cmp.Equal(got, want) {
		t.Fatalf("Table Cluster States %v, want: %v", got, want)
	}
	clusterState := table2.ClusterStates[clusterID]
	if got, want := len(clusterState.EncryptionInfo), 1; !cmp.Equal(got, want) {
		t.Fatalf("Table Encryption Info Length: %v, want: %v", got, want)
	}
	tableEncInfo := clusterState.EncryptionInfo[0]
	if got, want := int(tableEncInfo.EncryptionStatus.Code), 0; !cmp.Equal(got, want) {
		t.Fatalf("EncryptionStatus: %v, want: %v", got, want)
	}
	// NOTE: this EncryptionType is btapb.EncryptionInfo_EncryptionType
	if got, want := tableEncInfo.EncryptionType, btapb.EncryptionInfo_CUSTOMER_MANAGED_ENCRYPTION; !cmp.Equal(got, want) {
		t.Fatalf("EncryptionType: %v, want: %v", got, want)
	}
	if got, want := tableEncInfo.KmsKeyVersion, encryptionKeyVersion; !cmp.Equal(got, want) {
		t.Fatalf("KMS Key Version: %v, want: %v", got, want)
	}

	// Validate Encyrption Info retrieved via EncryptionInfo
	encryptionInfo, err := adminClient.EncryptionInfo(ctx, table)
	if err != nil {
		t.Fatalf("EncryptionInfo: %v", err)
	}
	if got, want := len(encryptionInfo), 1; !cmp.Equal(got, want) {
		t.Fatalf("Number of Clusters with Encryption Info: %v, want: %v", got, want)
	}
	encryptionInfos := encryptionInfo[clusterID]
	if got, want := len(encryptionInfos), 1; !cmp.Equal(got, want) {
		t.Fatalf("Encryption Info Length: %v, want: %v", got, want)
	}
	if len(encryptionInfos) != 1 {
		t.Fatalf("Expected Single EncryptionInfo")
	}
	v := encryptionInfos[0]
	if got, want := int(v.Status.Code), 0; !cmp.Equal(got, want) {
		t.Fatalf("EncryptionStatus: %v, want: %v", got, want)
	}
	// NOTE: this EncryptionType is EncryptionType
	if got, want := v.Type, CustomerManagedEncryption; !cmp.Equal(got, want) {
		t.Fatalf("EncryptionType: %v, want: %v", got, want)
	}
	if got, want := v.KMSKeyVersion, encryptionKeyVersion; !cmp.Equal(got, want) {
		t.Fatalf("KMS Key Version: %v, want: %v", got, want)
	}

	// Validate CMEK on Cluster Info
	cInfo, err := iAdminClient.GetCluster(ctx, instanceToCreate, clusterID)
	if err != nil {
		t.Fatalf("GetCluster: %v", err)
	}

	if got, want := cInfo.KMSKeyName, kmsKeyName; !cmp.Equal(got, want) {
		t.Fatalf("KMSKeyName: %v, want: %v", got, want)
	}

	// Create a backup with CMEK enabled, verify backup encryption info
	backupName := "backupCMEK"
	defer adminClient.DeleteBackup(ctx, clusterID, backupName)
	if err = adminClient.CreateBackup(ctx, table, clusterID, backupName, time.Now().Add(8*time.Hour)); err != nil {
		t.Fatalf("Creating backup: %v", err)
	}
	backup, err := adminClient.BackupInfo(ctx, clusterID, backupName)
	if err != nil {
		t.Fatalf("BackupInfo: %v", backup)
	}

	if got, want := backup.EncryptionInfo.Type, CustomerManagedEncryption; !cmp.Equal(got, want) {
		t.Fatalf("Backup Encryption EncryptionType: %v, want: %v", got, want)
	}
	if got, want := backup.EncryptionInfo.KMSKeyVersion, encryptionKeyVersion; !cmp.Equal(got, want) {
		t.Fatalf("Backup Encryption KMSKeyVersion: %v, want: %v", got, want)
	}
	if got, want := int(backup.EncryptionInfo.Status.Code), 2; !cmp.Equal(got, want) {
		t.Fatalf("Backup EncryptionStatus: %v, want: %v", got, want)
	}
}

func TestIntegration_AdminUpdateInstanceLabels(t *testing.T) {
	// Check the environments
	if instanceToCreate == "" {
		t.Skip("instanceToCreate not set, skipping instance creation testing")
	}
	instanceToCreate += "2"

	testEnv, err := NewIntegrationEnv()
	if err != nil {
		t.Fatalf("IntegrationEnv: %v", err)
	}
	defer testEnv.Close()
	if !testEnv.Config().UseProd {
		t.Skip("emulator doesn't support instance creation")
	}

	// Create an instance admin client
	timeout := 7 * time.Minute
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

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

	testutil.Retry(t, 3, 5*time.Second, func(R *testutil.R) {
		if err := iAdminClient.CreateInstance(ctx, conf); err != nil {
			t.Fatalf("CreateInstance: %v", err)
		}
	})
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
	instanceToCreate += "3"

	testEnv, err := NewIntegrationEnv()
	if err != nil {
		t.Fatalf("IntegrationEnv: %v", err)
	}
	defer testEnv.Close()

	if !testEnv.Config().UseProd {
		t.Skip("emulator doesn't support instance creation")
	}

	timeout := 5 * time.Minute
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	iAdminClient, err := testEnv.NewInstanceAdminClient()
	if err != nil {
		t.Fatalf("NewInstanceAdminClient: %v", err)
	}
	defer iAdminClient.Close()

	clusterID := clusterUIDSpace.New()

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
			{ClusterID: clusterID, NumNodes: 5},
		},
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
	clusterID2 := clusterUIDSpace.New()
	confWithClusters = &InstanceWithClustersConfig{
		InstanceID: instanceToCreate,
		Clusters: []ClusterConfig{
			{ClusterID: clusterID},
			{ClusterID: clusterID2, NumNodes: 3, StorageType: SSD, Zone: instanceToCreateZone2},
		},
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
			{ClusterID: clusterID, NumNodes: 4},
		},
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

func TestIntegration_Autoscaling(t *testing.T) {
	if instanceToCreate == "" {
		t.Skip("instanceToCreate not set, skipping instance update testing")
	}
	instanceToCreate += "4"

	testEnv, err := NewIntegrationEnv()
	if err != nil {
		t.Fatalf("IntegrationEnv: %v", err)
	}
	defer testEnv.Close()

	if !testEnv.Config().UseProd {
		t.Skip("emulator doesn't support instance creation")
	}

	timeout := 5 * time.Minute
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	iAdminClient, err := testEnv.NewInstanceAdminClient()
	if err != nil {
		t.Fatalf("NewInstanceAdminClient: %v", err)
	}
	defer iAdminClient.Close()

	clusterID := instanceToCreate + "-cluster"

	t.Log("creating an instance with autoscaling ON (Min = 3, Max = 4)")
	conf := &InstanceConf{
		InstanceId:   instanceToCreate,
		ClusterId:    clusterID,
		DisplayName:  "test instance",
		Zone:         instanceToCreateZone,
		InstanceType: PRODUCTION,
		AutoscalingConfig: &AutoscalingConfig{
			MinNodes:         3,
			MaxNodes:         4,
			CPUTargetPercent: 60,
		},
	}
	if err := iAdminClient.CreateInstance(ctx, conf); err != nil {
		t.Fatalf("CreateInstance: %v", err)
	}
	defer iAdminClient.DeleteInstance(ctx, instanceToCreate)

	cluster, err := iAdminClient.GetCluster(ctx, instanceToCreate, clusterID)
	if err != nil {
		t.Fatalf("GetCluster: %v", err)
	}
	wantNodes := 3
	if gotNodes := cluster.ServeNodes; gotNodes != wantNodes {
		t.Fatalf("want cluster nodes = %v, got = %v", wantNodes, gotNodes)
	}
	wantMin := 3
	if gotMin := cluster.AutoscalingConfig.MinNodes; gotMin != wantMin {
		t.Fatalf("want cluster autoscaling min = %v, got = %v", wantMin, gotMin)
	}
	wantMax := 4
	if gotMax := cluster.AutoscalingConfig.MaxNodes; gotMax != wantMax {
		t.Fatalf("want cluster autoscaling max = %v, got = %v", wantMax, gotMax)
	}
	wantCPU := 60
	if gotCPU := cluster.AutoscalingConfig.CPUTargetPercent; gotCPU != wantCPU {
		t.Fatalf("want cluster autoscaling CPU target = %v, got = %v", wantCPU, gotCPU)
	}

	serveNodes := 1
	t.Logf("setting autoscaling OFF and setting serve nodes to %v", serveNodes)
	err = iAdminClient.UpdateCluster(ctx, instanceToCreate, clusterID, int32(serveNodes))
	if err != nil {
		t.Fatalf("UpdateCluster: %v", err)
	}
	cluster, err = iAdminClient.GetCluster(ctx, instanceToCreate, clusterID)
	if err != nil {
		t.Fatalf("GetCluster: %v", err)
	}
	wantNodes = 1
	if gotNodes := cluster.ServeNodes; gotNodes != wantNodes {
		t.Fatalf("want cluster nodes = %v, got = %v", wantNodes, gotNodes)
	}
	if gotAsc := cluster.AutoscalingConfig; gotAsc != nil {
		t.Fatalf("want cluster autoscaling = nil, got = %v", gotAsc)
	}

	ac := AutoscalingConfig{
		MinNodes:         3,
		MaxNodes:         4,
		CPUTargetPercent: 80,
	}
	t.Logf("setting autoscaling ON (Min = %v, Max = %v)", ac.MinNodes, ac.MaxNodes)
	err = iAdminClient.SetAutoscaling(ctx, instanceToCreate, clusterID, ac)
	if err != nil {
		t.Fatalf("SetAutoscaling: %v", err)
	}
	cluster, err = iAdminClient.GetCluster(ctx, instanceToCreate, clusterID)
	if err != nil {
		t.Fatalf("GetCluster: %v", err)
	}
	wantMin = ac.MinNodes
	if gotMin := cluster.AutoscalingConfig.MinNodes; gotMin != wantMin {
		t.Fatalf("want cluster autoscaling min = %v, got = %v", wantMin, gotMin)
	}
	wantMax = ac.MaxNodes
	if gotMax := cluster.AutoscalingConfig.MaxNodes; gotMax != wantMax {
		t.Fatalf("want cluster autoscaling max = %v, got = %v", wantMax, gotMax)
	}
	wantCPU = ac.CPUTargetPercent
	if gotCPU := cluster.AutoscalingConfig.CPUTargetPercent; gotCPU != wantCPU {
		t.Fatalf("want cluster autoscaling CPU target = %v, got = %v", wantCPU, gotCPU)
	}

}

// instanceAdminClientMock is used to test FailedLocations field processing.
type instanceAdminClientMock struct {
	Clusters             []*btapb.Cluster
	UnavailableLocations []string
	// Imbedding the interface allows test writers to override just the methods
	// that are interesting for a test and ignore the rest.
	btapb.BigtableInstanceAdminClient
}

func (iacm *instanceAdminClientMock) ListClusters(ctx context.Context, req *btapb.ListClustersRequest, opts ...grpc.CallOption) (*btapb.ListClustersResponse, error) {
	res := btapb.ListClustersResponse{
		Clusters:        iacm.Clusters,
		FailedLocations: iacm.UnavailableLocations,
	}
	return &res, nil
}

func TestIntegration_InstanceAdminClient_Clusters_WithFailedLocations(t *testing.T) {
	testEnv, err := NewIntegrationEnv()
	if err != nil {
		t.Fatalf("IntegrationEnv: %v", err)
	}
	defer testEnv.Close()

	if !testEnv.Config().UseProd {
		t.Skip("emulator doesn't support snapshots")
	}

	iAdminClient, err := testEnv.NewInstanceAdminClient()
	if err != nil {
		t.Fatalf("NewInstanceAdminClient: %v", err)
	}
	defer iAdminClient.Close()

	cluster1 := btapb.Cluster{Name: "cluster1"}
	failedLoc := "euro1"

	iAdminClient.iClient = &instanceAdminClientMock{
		Clusters:             []*btapb.Cluster{&cluster1},
		UnavailableLocations: []string{failedLoc},
	}

	cis, err := iAdminClient.Clusters(context.Background(), "instance-id")
	convertedErr, ok := err.(ErrPartiallyUnavailable)
	if !ok {
		t.Fatalf("want error ErrPartiallyUnavailable, got other")
	}
	if got, want := len(convertedErr.Locations), 1; got != want {
		t.Fatalf("want %v failed locations, got %v", want, got)
	}
	if got, want := convertedErr.Locations[0], failedLoc; got != want {
		t.Fatalf("want failed location %v, got %v", want, got)
	}
	if got, want := len(cis), 1; got != want {
		t.Fatalf("want %v failed locations, got %v", want, got)
	}
	if got, want := cis[0].Name, cluster1.Name; got != want {
		t.Fatalf("want cluster %v, got %v", want, got)
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
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

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

	defer deleteTable(ctx, t, adminClient, myTableName)

	if err := adminClient.CreateTable(ctx, myTableName); err != nil {
		t.Fatalf("Creating table: %v", err)
	}

	tables := list()
	if got, want := tables, []string{myTableName}; !containsAll(got, want) {
		t.Errorf("adminClient.Tables returned %#v, want %#v", got, want)
	}

	// calling ModifyColumnFamilies to check the granularity of table
	prefix := adminClient.instancePrefix()
	req := &btapb.ModifyColumnFamiliesRequest{
		Name: prefix + "/tables/" + myTableName,
		Modifications: []*btapb.ModifyColumnFamiliesRequest_Modification{{
			Id:  "cf",
			Mod: &btapb.ModifyColumnFamiliesRequest_Modification_Create{Create: &btapb.ColumnFamily{}},
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

	uniqueID := make([]byte, 4)
	rand.Read(uniqueID)
	profileID := fmt.Sprintf("app_profile_id%x", uniqueID)

	err = iAdminClient.DeleteAppProfile(ctx, adminClient.instance, profileID)

	defer iAdminClient.Close()
	profile := ProfileConf{
		ProfileID:     profileID,
		InstanceID:    adminClient.instance,
		ClusterID:     testEnv.Config().Cluster,
		Description:   "creating new app profile 1",
		RoutingPolicy: SingleClusterRouting,
	}

	createdProfile, err := iAdminClient.CreateAppProfile(ctx, profile)
	if err != nil {
		t.Fatalf("Creating app profile: %v", err)
	}

	gotProfile, err := iAdminClient.GetAppProfile(ctx, adminClient.instance, profileID)
	if err != nil {
		t.Fatalf("Get app profile: %v", err)
	}

	defer func() {
		err = iAdminClient.DeleteAppProfile(ctx, adminClient.instance, profileID)
		if err != nil {
			t.Fatalf("Delete app profile: %v", err)
		}
	}()

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

	// Ensure the profiles we require exist. profiles ⊂ allProfiles
	verifyProfilesSubset := func(allProfiles []*btapb.AppProfile, profiles map[string]struct{}) {
		for _, profile := range allProfiles {
			segs := strings.Split(profile.Name, "/")
			delete(profiles, segs[len(segs)-1])
		}
		if len(profiles) > 0 {
			t.Fatalf("Initial app profile list missing profile: %v : %v", profiles, allProfiles)
		}
	}

	// App Profile list should contain default, app_profile1
	wantProfiles := map[string]struct{}{"default": {}, profileID: {}}
	verifyProfilesSubset(profiles, wantProfiles)

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
				Etag:          gotProfile.Etag,
				Isolation: &btapb.AppProfile_StandardIsolation_{
					StandardIsolation: &btapb.AppProfile_StandardIsolation{
						Priority: btapb.AppProfile_PRIORITY_HIGH,
					},
				},
			},
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
					},
				},
				Isolation: &btapb.AppProfile_StandardIsolation_{
					StandardIsolation: &btapb.AppProfile_StandardIsolation{
						Priority: btapb.AppProfile_PRIORITY_HIGH,
					},
				},
			},
		},
	} {
		err = iAdminClient.UpdateAppProfile(ctx, adminClient.instance, profileID, test.uattrs)
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

		// Retry to see if the update has been completed
		testutil.Retry(t, 10, 10*time.Second, func(r *testutil.R) {
			got, _ := iAdminClient.GetAppProfile(ctx, adminClient.instance, profileID)

			if !proto.Equal(got, test.want) {
				r.Errorf("%s : got profile : %v, want profile: %v", test.desc, gotProfile, test.want)
			}
		})

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
		t.Errorf("InstanceInfo returned name %#v, want %#v", iInfo.DisplayName, adminClient.instance)
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

func createInstance(ctx context.Context, testEnv IntegrationEnv, iAdminClient *InstanceAdminClient) (string, string, error) {
	diffInstance := generateNewInstanceName()
	diffCluster := clusterUIDSpace.New()
	conf := &InstanceConf{
		InstanceId:   diffInstance,
		ClusterId:    diffCluster,
		DisplayName:  "different test sourceInstance",
		Zone:         instanceToCreateZone2,
		InstanceType: DEVELOPMENT,
		Labels:       map[string]string{"test-label-key-diff": "test-label-value-diff"},
	}
	if err := iAdminClient.CreateInstance(ctx, conf); err != nil {
		return "", "", fmt.Errorf("CreateInstance: %v", err)
	}
	return diffInstance, diffCluster, nil
}

func TestIntegration_AdminCopyBackup(t *testing.T) {
	testEnv, err := NewIntegrationEnv()
	if err != nil {
		t.Fatalf("IntegrationEnv: %v", err)
	}
	defer testEnv.Close()

	if !testEnv.Config().UseProd {
		t.Skip("emulator doesn't support backups")
	}

	timeout := 15 * time.Minute
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	// Create source clients
	srcAdminClient, err := testEnv.NewAdminClient()
	if err != nil {
		t.Fatalf("NewAdminClient: %v", err)
	}
	defer srcAdminClient.Close()
	srcIAdminClient, err := testEnv.NewInstanceAdminClient()
	if err != nil {
		t.Fatalf("NewInstanceAdminClient: %v", err)
	}
	defer srcIAdminClient.Close()

	// Create table
	tblConf := TableConf{
		TableID: testEnv.Config().Table,
		Families: map[string]GCPolicy{
			"fam1": MaxVersionsPolicy(1),
			"fam2": MaxVersionsPolicy(2),
		},
	}
	defer deleteTable(ctx, t, srcAdminClient, tblConf.TableID)
	if err := srcAdminClient.CreateTableFromConf(ctx, &tblConf); err != nil {
		t.Fatalf("Creating table from TableConf: %v", err)
	}

	// Create source backup
	copyBackupUID := uid.NewSpace(prefixOfInstanceResources, &uid.Options{})
	backupUID := uid.NewSpace(prefixOfInstanceResources, &uid.Options{})
	srcBackupName := backupUID.New()
	srcCluster := testEnv.Config().Cluster
	defer srcAdminClient.DeleteBackup(ctx, srcCluster, srcBackupName)
	if err = srcAdminClient.CreateBackup(ctx, tblConf.TableID, srcCluster, srcBackupName, time.Now().Add(100*time.Hour)); err != nil {
		t.Fatalf("Creating backup: %v", err)
	}
	wantSourceBackup := srcAdminClient.instancePrefix() + "/clusters/" + srcCluster + "/backups/" + srcBackupName

	destProj1 := testEnv.Config().Project
	destProj1Inst1 := testEnv.Config().Instance // 1st instance in 1st destination project
	destProj1Inst1Cl1 := srcCluster             // 1st cluster in 1st instance in 1st destination project
	destIAdminClient1 := srcIAdminClient

	type testcase struct {
		desc         string
		destProject  string
		destInstance string
		destCluster  string
	}
	testcases := []testcase{
		{
			desc:         "Copy backup to same project, same instance, same cluster",
			destProject:  destProj1,
			destInstance: destProj1Inst1,
			destCluster:  destProj1Inst1Cl1,
		},
	}

	// testEnv.Config().Cluster2 will be non-empty if 'it.cluster2' flag is passed
	// or 'GCLOUD_TESTS_BIGTABLE_PRI_PROJ_SEC_CLUSTER' environment variable is set
	// Add more testcases if Cluster2 is non-empty string
	if testEnv.Config().Cluster2 != "" {
		testcases = append(testcases, testcase{
			desc:         "Copy backup to same project, same instance, different cluster",
			destProject:  destProj1,
			destInstance: destProj1Inst1,
			destCluster:  testEnv.Config().Cluster2,
		})
	}

	// If 'it.run-create-instance-tests' flag is set while running the tests,
	// instanceToCreate will be non-empty string.
	// Add more testcases if instanceToCreate is non-empty string
	if instanceToCreate != "" {
		// Create a 2nd instance in 1st destination project
		destProj1Inst2, destProj1Inst2Cl1, err := createInstance(ctx, testEnv, destIAdminClient1)
		if err != nil {
			t.Fatalf("CreateInstance: %v", err)
		}
		defer destIAdminClient1.DeleteInstance(ctx, destProj1Inst2)
		testcases = append(testcases, testcase{
			desc:         "Copy backup to same project, different instance",
			destProject:  destProj1,
			destInstance: destProj1Inst2,
			destCluster:  destProj1Inst2Cl1,
		})
	} else {
		t.Logf("WARNING: run-create-instance-tests not set, skipping tests that require instance creation")
	}

	// testEnv.Config().Project2 will be non-empty if 'it.project2' flag is passed
	// or 'GCLOUD_TESTS_GOLANG_SECONDARY_BIGTABLE_PROJECT_ID' environment variable is set
	// Add more testcases if Project2 is non-empty string
	if testEnv.Config().Project2 != "" {
		// Create admin client for 2nd project in test environment
		destProj2 := testEnv.Config().Project2
		ctx, options, err := testEnv.AdminClientOptions()
		if err != nil {
			t.Fatalf("AdminClientOptions: %v", err)
		}
		destIAdminClient2, err := NewInstanceAdminClient(ctx, destProj2, options...)
		if err != nil {
			t.Fatalf("NewInstanceAdminClient: %v", err)
		}
		defer destIAdminClient2.Close()

		// Create instance in 2nd project
		destProj2Inst1, destProj2Inst1Cl1, err := createInstance(ctx, testEnv, destIAdminClient2)
		if err != nil {
			t.Fatalf("CreateInstance: %v", err)
		}
		defer destIAdminClient2.DeleteInstance(ctx, destProj2Inst1)
		testcases = append(testcases, testcase{
			desc:         "Copy backup to different project",
			destProject:  destProj2,
			destInstance: destProj2Inst1,
			destCluster:  destProj2Inst1Cl1,
		})
	} else {
		t.Logf("WARNING: Secondary project not set, skipping copy backup to different project testing")
	}

	for _, testcase := range testcases {
		// Create destination client
		destCtx, destOpts, err := testEnv.AdminClientOptions()
		if err != nil {
			t.Fatalf("%v: AdminClientOptions: %v", testcase.desc, err)
		}

		desc := testcase.desc
		destProject := testcase.destProject
		destInstance := testcase.destInstance
		destCluster := testcase.destCluster

		destAdminClient, err := NewAdminClient(destCtx, destProject, destInstance, destOpts...)
		if err != nil {
			t.Fatalf("%v: NewAdminClient: %v", desc, err)
		}
		defer destAdminClient.Close()

		// Copy Backup
		destBackupName := copyBackupUID.New()
		defer destAdminClient.DeleteBackup(destCtx, destCluster, destBackupName)
		err = srcAdminClient.CopyBackup(destCtx, srcCluster, srcBackupName, destProject, destInstance, destCluster,
			destBackupName, time.Now().Add(24*time.Hour))
		if err != nil {
			t.Fatalf("%v: CopyBackup: %v", desc, err)
		}

		// Verify source backup field in backup info
		gotBackupInfo, err := destAdminClient.BackupInfo(ctx, destCluster, destBackupName)
		if err != nil {
			t.Fatalf("%v: BackupInfo: %v", desc, err)
		}
		if gotBackupInfo.SourceBackup != wantSourceBackup {
			t.Fatalf("%v: SourceBackup: got: %v, want: %v", desc, gotBackupInfo.SourceBackup, wantSourceBackup)
		}
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

	timeout := 15 * time.Minute
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	adminClient, err := testEnv.NewAdminClient()
	if err != nil {
		t.Fatalf("NewAdminClient: %v", err)
	}
	defer adminClient.Close()

	tblConf := TableConf{
		TableID: testEnv.Config().Table,
		Families: map[string]GCPolicy{
			"fam1": MaxVersionsPolicy(1),
			"fam2": MaxVersionsPolicy(2),
		},
	}
	if err := adminClient.CreateTableFromConf(ctx, &tblConf); err != nil {
		t.Fatalf("Creating table from TableConf: %v", err)
	}
	// Delete the table at the end of the test. Schedule ahead of time
	// in case the client fails
	defer deleteTable(ctx, t, adminClient, tblConf.TableID)

	sourceInstance := testEnv.Config().Instance
	sourceCluster := testEnv.Config().Cluster

	iAdminClient, err := testEnv.NewInstanceAdminClient()
	if err != nil {
		t.Fatalf("NewInstanceAdminClient: %v", err)
	}
	defer iAdminClient.Close()

	// Create different instance to restore table.
	diffInstance, diffCluster, err := createInstance(ctx, testEnv, iAdminClient)
	if err != nil {
		t.Fatalf("CreateInstance: %v", err)
	}
	defer iAdminClient.DeleteInstance(ctx, diffInstance)

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

	// Create backup
	if err != nil {
		t.Fatalf("Failed to generate a unique ID: %v", err)
	}

	backupUID := uid.NewSpace("mybackup-", &uid.Options{})
	backupName := backupUID.New()
	defer adminClient.DeleteBackup(ctx, sourceCluster, backupName)

	if err = adminClient.CreateBackup(ctx, tblConf.TableID, sourceCluster, backupName, time.Now().Add(8*time.Hour)); err != nil {
		t.Fatalf("Creating backup: %v", err)
	}

	// List backup
	backups, err := list(sourceCluster)
	if err != nil {
		t.Fatalf("Listing backups: %v", err)
	}
	if got, want := len(backups), 1; got < want {
		t.Fatalf("Listing backup count: %d, want: >= %d", got, want)
	}

	foundBackup := false
	for _, backup := range backups {
		if backup.Name == backupName {
			foundBackup = true

			if got, want := backup.SourceTable, tblConf.TableID; got != want {
				t.Errorf("Backup SourceTable: %s, want: %s", got, want)
			}
			if got, want := backup.ExpireTime, backup.StartTime.Add(8*time.Hour); math.Abs(got.Sub(want).Minutes()) > 1 {
				t.Errorf("Backup ExpireTime: %s, want: %s", got, want)
			}

			break
		}
	}

	if !foundBackup {
		t.Errorf("Backup not found: %v", backupName)
	}

	// Get backup
	backup, err := adminClient.BackupInfo(ctx, sourceCluster, backupName)
	if err != nil {
		t.Fatalf("BackupInfo: %v", backup)
	}
	if got, want := *backup, *backups[0]; cmp.Equal(got, &want) {
		t.Errorf("BackupInfo: %v, want: %v", got, want)
	}

	// Update backup
	newExpireTime := time.Now().Add(10 * time.Hour)
	err = adminClient.UpdateBackup(ctx, sourceCluster, backupName, newExpireTime)
	if err != nil {
		t.Fatalf("UpdateBackup failed: %v", err)
	}

	// Check that updated backup has the correct expire time
	updatedBackup, err := adminClient.BackupInfo(ctx, sourceCluster, backupName)
	if err != nil {
		t.Fatalf("BackupInfo: %v", err)
	}
	backup.ExpireTime = newExpireTime
	// Server clock and local clock may not be perfectly sync'ed.
	if got, want := *updatedBackup, *backup; got.ExpireTime.Sub(want.ExpireTime) > time.Minute {
		t.Errorf("BackupInfo: %v, want: %v", got, want)
	}

	// Restore backup
	restoredTable := tblConf.TableID + "-restored"
	defer deleteTable(ctx, t, adminClient, restoredTable)
	if err = adminClient.RestoreTable(ctx, restoredTable, sourceCluster, backupName); err != nil {
		t.Fatalf("RestoreTable: %v", err)
	}
	if _, err := adminClient.TableInfo(ctx, restoredTable); err != nil {
		t.Fatalf("Restored TableInfo: %v", err)
	}
	// Restore backup to different instance
	restoreTableName := tblConf.TableID + "-diff-restored"
	diffConf := IntegrationTestConfig{
		Project:  testEnv.Config().Project,
		Instance: diffInstance,
		Cluster:  diffCluster,
		Table:    restoreTableName,
	}
	env := &ProdEnv{
		config: diffConf,
	}
	dAdminClient, err := env.NewAdminClient()
	if err != nil {
		t.Errorf("NewAdminClient: %v", err)
	}
	defer dAdminClient.Close()

	defer deleteTable(ctx, t, dAdminClient, restoreTableName)
	if err = dAdminClient.RestoreTableFrom(ctx, sourceInstance, restoreTableName, sourceCluster, backupName); err != nil {
		t.Fatalf("RestoreTableFrom: %v", err)
	}
	tblInfo, err := dAdminClient.TableInfo(ctx, restoreTableName)
	if err != nil {
		t.Fatalf("Restored to different sourceInstance failed, TableInfo: %v", err)
	}
	families := tblInfo.Families
	sort.Strings(tblInfo.Families)
	wantFams := []string{"fam1", "fam2"}
	if !testutil.Equal(families, wantFams) {
		t.Errorf("Column family mismatch, got %v, want %v", tblInfo.Families, wantFams)
	}

	// Delete backup
	if err = adminClient.DeleteBackup(ctx, sourceCluster, backupName); err != nil {
		t.Fatalf("DeleteBackup: %v", err)
	}
	backups, err = list(sourceCluster)
	if err != nil {
		t.Fatalf("List after Delete: %v", err)
	}

	// Verify the backup was deleted.
	for _, backup := range backups {
		if backup.Name == backupName {
			t.Errorf("Backup '%v' was not deleted", backup.Name)
			break
		}
	}
}

func TestIntegration_AdminAuthorizedView(t *testing.T) {
	testEnv, err := NewIntegrationEnv()
	if err != nil {
		t.Fatalf("IntegrationEnv: %v", err)
	}
	defer testEnv.Close()

	if !testEnv.Config().UseProd {
		t.Skip("emulator doesn't support authorizedViews")
	}

	timeout := 15 * time.Minute
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	adminClient, err := testEnv.NewAdminClient()
	if err != nil {
		t.Fatalf("NewAdminClient: %v", err)
	}
	defer adminClient.Close()

	tblConf := TableConf{
		TableID: testEnv.Config().Table,
		Families: map[string]GCPolicy{
			"fam1": MaxVersionsPolicy(1),
			"fam2": MaxVersionsPolicy(2),
		},
	}
	if err := adminClient.CreateTableFromConf(ctx, &tblConf); err != nil {
		t.Fatalf("Creating table from TableConf: %v", err)
	}
	// Delete the table at the end of the test. Schedule ahead of time
	// in case the client fails
	defer deleteTable(ctx, t, adminClient, tblConf.TableID)

	// Create authorized view
	authorizedViewUUID := uid.NewSpace("authorizedView-", &uid.Options{})
	authorizedView := authorizedViewUUID.New()
	defer adminClient.DeleteAuthorizedView(ctx, tblConf.TableID, authorizedView)

	authorizedViewConf := AuthorizedViewConf{
		TableID:          tblConf.TableID,
		AuthorizedViewID: authorizedView,
		AuthorizedView: &SubsetViewConf{
			RowPrefixes: [][]byte{[]byte("r1")},
		},
		DeletionProtection: Protected,
	}
	if err = adminClient.CreateAuthorizedView(ctx, &authorizedViewConf); err != nil {
		t.Fatalf("Creating authorized view: %v", err)
	}

	// List authorized views
	authorizedViews, err := adminClient.AuthorizedViews(ctx, tblConf.TableID)
	if err != nil {
		t.Fatalf("Listing authorized views: %v", err)
	}
	if got, want := len(authorizedViews), 1; got != want {
		t.Fatalf("Listing authorized views count: %d, want: != %d", got, want)
	}
	if got, want := authorizedViews[0], authorizedView; got != want {
		t.Errorf("AuthorizedView Name: %s, want: %s", got, want)
	}

	// Get authorized view
	avInfo, err := adminClient.AuthorizedViewInfo(ctx, tblConf.TableID, authorizedView)
	if err != nil {
		t.Fatalf("Getting authorized view: %v", err)
	}
	if got, want := avInfo.AuthorizedView.(*SubsetViewInfo), authorizedViewConf.AuthorizedView.(*SubsetViewConf); cmp.Equal(got, want) {
		t.Errorf("SubsetViewConf: %v, want: %v", got, want)
	}

	// Cannot delete the authorized view because it is deletion protected
	if err = adminClient.DeleteAuthorizedView(ctx, tblConf.TableID, authorizedView); err == nil {
		t.Fatalf("Expect error when deleting authorized view")
	}

	// Update authorized view
	newAuthorizedViewConf := AuthorizedViewConf{
		TableID:            tblConf.TableID,
		AuthorizedViewID:   authorizedView,
		DeletionProtection: Unprotected,
	}
	err = adminClient.UpdateAuthorizedView(ctx, UpdateAuthorizedViewConf{
		AuthorizedViewConf: newAuthorizedViewConf,
	})
	if err != nil {
		t.Fatalf("UpdateAuthorizedView failed: %v", err)
	}

	// Check that updated authorized view has the correct deletion protection
	avInfo, err = adminClient.AuthorizedViewInfo(ctx, tblConf.TableID, authorizedView)
	if err != nil {
		t.Fatalf("Getting authorized view: %v", err)
	}
	if got, want := avInfo.DeletionProtection, Unprotected; got != want {
		t.Errorf("AuthorizedView deletion protection: %v, want: %v", got, want)
	}
	// Check that the subset_view field doesn't change
	if got, want := avInfo.AuthorizedView.(*SubsetViewInfo), authorizedViewConf.AuthorizedView.(*SubsetViewConf); cmp.Equal(got, want) {
		t.Errorf("SubsetViewConf: %v, want: %v", got, want)
	}

	// Delete authorized view
	if err = adminClient.DeleteAuthorizedView(ctx, tblConf.TableID, authorizedView); err != nil {
		t.Fatalf("DeleteAuthorizedView: %v", err)
	}

	// Verify the authorized view was deleted.
	authorizedViews, err = adminClient.AuthorizedViews(ctx, tblConf.TableID)
	if err != nil {
		t.Fatalf("Listing authorized views: %v", err)
	}
	if got, want := len(authorizedViews), 0; got != want {
		t.Fatalf("Listing authorized views count: %d, want: != %d", got, want)
	}
}

func TestIntegration_DataAuthorizedView(t *testing.T) {
	testEnv, err := NewIntegrationEnv()
	if err != nil {
		t.Fatalf("IntegrationEnv: %v", err)
	}
	defer testEnv.Close()

	if !testEnv.Config().UseProd {
		t.Skip("emulator doesn't support authorizedViews")
	}

	timeout := 15 * time.Minute
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	adminClient, err := testEnv.NewAdminClient()
	if err != nil {
		t.Fatalf("NewAdminClient: %v", err)
	}
	defer adminClient.Close()

	tblConf := TableConf{
		TableID: testEnv.Config().Table,
		Families: map[string]GCPolicy{
			"fam1": MaxVersionsPolicy(1),
			"fam2": MaxVersionsPolicy(2),
		},
	}
	if err := adminClient.CreateTableFromConf(ctx, &tblConf); err != nil {
		t.Fatalf("Creating table from TableConf: %v", err)
	}
	// Delete the table at the end of the test. Schedule ahead of time
	// in case the client fails
	defer deleteTable(ctx, t, adminClient, tblConf.TableID)

	// Create authorized view
	authorizedViewUUID := uid.NewSpace("authorizedView-", &uid.Options{})
	authorizedView := authorizedViewUUID.New()
	defer adminClient.DeleteAuthorizedView(ctx, tblConf.TableID, authorizedView)

	authorizedViewConf := AuthorizedViewConf{
		TableID:          tblConf.TableID,
		AuthorizedViewID: authorizedView,
		AuthorizedView: &SubsetViewConf{
			RowPrefixes: [][]byte{[]byte("r1")},
			FamilySubsets: map[string]FamilySubset{
				"fam1": {
					QualifierPrefixes: [][]byte{[]byte("col")},
				},
				"fam2": {
					Qualifiers: [][]byte{[]byte("col")},
				},
			},
		},
		DeletionProtection: Unprotected,
	}
	if err = adminClient.CreateAuthorizedView(ctx, &authorizedViewConf); err != nil {
		t.Fatalf("Creating authorized view: %v", err)
	}

	client, err := testEnv.NewClient()
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}
	defer client.Close()
	av := client.OpenAuthorizedView(tblConf.TableID, authorizedView)
	tbl := client.OpenTable(tblConf.TableID)

	prefix1 := "r1"
	prefix2 := "r2" // outside of the authorized view
	mut1 := NewMutation()
	mut1.Set("fam1", "col1", 1000, []byte("1"))
	mut2 := NewMutation()
	mut2.Set("fam1", "col2", 1000, []byte("1"))
	mut3 := NewMutation()
	mut3.Set("fam2", "column", 1000, []byte("1")) // outside of the authorized view

	// Test mutation
	if err := av.Apply(ctx, prefix1, mut1); err != nil {
		t.Fatalf("Mutating row from an authorized view: %v", err)
	}
	if err := av.Apply(ctx, prefix2, mut1); err == nil {
		t.Fatalf("Expect error when mutating a row outside of the authorized view: %v", err)
	}
	if err := tbl.Apply(ctx, prefix2, mut1); err != nil {
		t.Fatalf("Mutating row from a table: %v", err)
	}

	// Test bulk mutations
	status, err := av.ApplyBulk(ctx, []string{prefix1, prefix2, prefix1}, []*Mutation{mut2, mut2, mut3})
	if err != nil {
		t.Fatalf("Mutating rows from an authorized view: %v", err)
	}
	if status == nil {
		t.Fatalf("Expect error for bad bulk mutation outside of the authorized view")
	} else if status[0] != nil || status[1] == nil || status[2] == nil {
		t.Fatalf("Expect error for bad bulk mutation outside of the authorized view")
	}

	// Test ReadRow
	gotRow, err := av.ReadRow(ctx, "r1")
	if err != nil {
		t.Fatalf("Reading row from an authorized view: %v", err)
	}
	wantRow := Row{
		"fam1": []ReadItem{
			{Row: "r1", Column: "fam1:col1", Timestamp: 1000, Value: []byte("1")},
			{Row: "r1", Column: "fam1:col2", Timestamp: 1000, Value: []byte("1")},
		},
	}
	if !testutil.Equal(gotRow, wantRow) {
		t.Fatalf("Error reading row from authorized view.\n Got %v\n Want %v", gotRow, wantRow)
	}
	gotRow, err = av.ReadRow(ctx, "r2")
	if err != nil {
		t.Fatalf("Reading row from an authorized view: %v", err)
	}
	if len(gotRow) != 0 {
		t.Fatalf("Expect empty result when reading row from outside an authorized view")
	}
	gotRow, err = tbl.ReadRow(ctx, "r2")
	if err != nil {
		t.Fatalf("Reading row from a table: %v", err)
	}
	if len(gotRow) != 1 {
		t.Fatalf("Invalid row count when reading from a table: %d, want: != %d", len(gotRow), 1)
	}

	// Test ReadRows
	var elt []string
	f := func(row Row) bool {
		for _, ris := range row {
			for _, ri := range ris {
				elt = append(elt, formatReadItem(ri))
			}
		}
		return true
	}
	if err = av.ReadRows(ctx, RowRange{}, f); err != nil {
		t.Fatalf("Reading rows from an authorized view: %v", err)
	}
	want := "r1-col1-1,r1-col2-1"
	if got := strings.Join(elt, ","); got != want {
		t.Fatalf("Error bulk reading from authorized view.\n Got %v\n Want %v", got, want)
	}
	elt = nil
	if err = tbl.ReadRows(ctx, RowRange{}, f); err != nil {
		t.Fatalf("Reading rows from a table: %v", err)
	}
	want = "r1-col1-1,r1-col2-1,r2-col1-1"
	if got := strings.Join(elt, ","); got != want {
		t.Fatalf("Error bulk reading from table.\n Got %v\n Want %v", got, want)
	}

	// Test ReadModifyWrite
	rmw := NewReadModifyWrite()
	rmw.AppendValue("fam1", "col1", []byte("1"))
	gotRow, err = av.ApplyReadModifyWrite(ctx, "r1", rmw)
	if err != nil {
		t.Fatalf("Applying ReadModifyWrite from an authorized view: %v", err)
	}
	wantRow = Row{
		"fam1": []ReadItem{
			{Row: "r1", Column: "fam1:col1", Value: []byte("11")},
		},
	}
	// Make sure the modified cell returned by the RMW operation has a timestamp.
	if gotRow["fam1"][0].Timestamp == 0 {
		t.Fatalf("RMW returned cell timestamp: got %v, want > 0", gotRow["fam1"][0].Timestamp)
	}
	clearTimestamps(gotRow)
	if !testutil.Equal(gotRow, wantRow) {
		t.Fatalf("Error applying ReadModifyWrite from authorized view.\n Got %v\n Want %v", gotRow, wantRow)
	}
	if _, err = av.ApplyReadModifyWrite(ctx, "r2", rmw); err == nil {
		t.Fatalf("Expect error applying ReadModifyWrite from outside an authorized view")
	}

	// Test SampleRowKeys
	presplitTable := fmt.Sprintf("presplit-table-%d", time.Now().Unix())
	if err := adminClient.CreatePresplitTable(ctx, presplitTable, []string{"r0", "r11", "r12", "r2"}); err != nil {
		t.Fatal(err)
	}
	defer adminClient.DeleteTable(ctx, presplitTable)
	if err := adminClient.CreateColumnFamily(ctx, presplitTable, "fam1"); err != nil {
		t.Fatal(err)
	}
	defer adminClient.DeleteAuthorizedView(ctx, presplitTable, authorizedView)
	if err = adminClient.CreateAuthorizedView(ctx, &AuthorizedViewConf{
		TableID:          presplitTable,
		AuthorizedViewID: authorizedView,
		AuthorizedView: &SubsetViewConf{
			RowPrefixes: [][]byte{[]byte("r1")},
		},
		DeletionProtection: Unprotected,
	}); err != nil {
		t.Fatalf("Creating authorized view: %v", err)
	}

	av = client.OpenAuthorizedView(presplitTable, authorizedView)
	sampleKeys, err := av.SampleRowKeys(ctx)
	if err != nil {
		t.Fatalf("Sampling row keys from an authorized view: %v", err)
	}
	want = "r11,r12,r2"
	if got := strings.Join(sampleKeys, ","); got != want {
		t.Fatalf("Error sample row keys from an authorized view.\n Got %v\n Want %v", got, want)
	}
}

// TestIntegration_DirectPathFallback tests the CFE fallback when the directpath net is blackholed.
func TestIntegration_DirectPathFallback(t *testing.T) {
	ctx := context.Background()
	testEnv, _, _, table, _, cleanup, err := setupIntegration(ctx, t)
	if err != nil {
		t.Fatal(err)
	}
	defer cleanup()

	if !testEnv.Config().AttemptDirectPath {
		t.Skip()
	}

	if len(blackholeDpv6Cmd) == 0 {
		t.Fatal("-it.blackhole-dpv6-cmd unset")
	}
	if len(blackholeDpv4Cmd) == 0 {
		t.Fatal("-it.blackhole-dpv4-cmd unset")
	}
	if len(allowDpv6Cmd) == 0 {
		t.Fatal("-it.allowdpv6-cmd unset")
	}
	if len(allowDpv4Cmd) == 0 {
		t.Fatal("-it.allowdpv4-cmd unset")
	}

	if err := populatePresidentsGraph(table); err != nil {
		t.Fatal(err)
	}

	// Precondition: wait for DirectPath to connect.
	dpEnabled := examineTraffic(ctx, testEnv, table, false)
	if !dpEnabled {
		t.Fatalf("Failed to observe RPCs over DirectPath")
	}

	// Enable the blackhole, which will prevent communication with grpclb and thus DirectPath.
	blackholeDirectPath(testEnv, t)
	dpDisabled := examineTraffic(ctx, testEnv, table, true)
	if !dpDisabled {
		t.Fatalf("Failed to fallback to CFE after blackhole DirectPath")
	}

	// Disable the blackhole, and client should use DirectPath again.
	allowDirectPath(testEnv, t)
	dpEnabled = examineTraffic(ctx, testEnv, table, false)
	if !dpEnabled {
		t.Fatalf("Failed to fallback to CFE after blackhole DirectPath")
	}
}

// examineTraffic returns whether RPCs use DirectPath (blackholeDP = false) or CFE (blackholeDP = true).
func examineTraffic(ctx context.Context, testEnv IntegrationEnv, table *Table, blackholeDP bool) bool {
	numCount := 0
	const (
		numRPCsToSend  = 20
		minCompleteRPC = 40
	)

	start := time.Now()
	for time.Since(start) < 2*time.Minute {
		for i := 0; i < numRPCsToSend; i++ {
			_, _ = table.ReadRow(ctx, "j§adams")
			if _, useDP := isDirectPathRemoteAddress(testEnv); useDP != blackholeDP {
				numCount++
				if numCount >= minCompleteRPC {
					return true
				}
			}
			time.Sleep(100 * time.Millisecond)
		}
	}
	return false
}

// Retries a function if it has an unavailable code for up to 30s using a backoff
func retryOnUnavailable(ctx context.Context, inner func() error) error {
	err := internal.Retry(ctx, gax.Backoff{Initial: 100 * time.Millisecond}, func() (stop bool, err error) {
		err = inner()
		if err == nil {
			return true, nil
		}
		s, ok := status.FromError(err)
		if !ok {
			return false, err
		}
		// Retry on Unavailable
		return s.Code() != codes.Unavailable, err
	})
	return err
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
	_ = cancel // ignore for test

	client, err := testEnv.NewClient()
	if err != nil {
		t.Logf("Error creating client: %v", err)
		return nil, nil, nil, nil, "", nil, err
	}

	adminClient, err := testEnv.NewAdminClient()
	if err != nil {
		t.Logf("Error creating admin client: %v", err)

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
		t.Logf("Error creating table: %v", err)
		return nil, nil, nil, nil, "", nil, err
	}

	err = retryOnUnavailable(ctx, func() error {
		return adminClient.CreateColumnFamily(ctx, tableName, "follows")
	})
	if err != nil {
		cancel()
		t.Logf("Error creating column family: %v", err)
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
	ctx, cancel := context.WithTimeout(ctx, time.Second*60)
	defer cancel()

	err := internal.Retry(ctx, bo, func() (bool, error) {
		err := ac.DeleteTable(ctx, name)
		if err != nil {
			return false, err
		}
		return true, nil
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

func blackholeDirectPath(testEnv IntegrationEnv, t *testing.T) {
	cmdRes := exec.Command("bash", "-c", blackholeDpv4Cmd)
	out, _ := cmdRes.CombinedOutput()
	t.Logf(string(out))
	if testEnv.Config().DirectPathIPV4Only {
		return
	}
	cmdRes = exec.Command("bash", "-c", blackholeDpv6Cmd)
	out, _ = cmdRes.CombinedOutput()
	t.Logf(string(out))
}

func allowDirectPath(testEnv IntegrationEnv, t *testing.T) {
	cmdRes := exec.Command("bash", "-c", allowDpv4Cmd)
	out, _ := cmdRes.CombinedOutput()
	t.Logf(string(out))
	if testEnv.Config().DirectPathIPV4Only {
		return
	}
	cmdRes = exec.Command("bash", "-c", allowDpv6Cmd)
	out, _ = cmdRes.CombinedOutput()
	t.Logf(string(out))
}

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
	"encoding/base64"
	"encoding/binary"
	"flag"
	"fmt"
	"io"
	"log"
	"math"
	"math/rand"
	"os"
	"os/exec"
	"reflect"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	cryptorand "crypto/rand"

	btapb "cloud.google.com/go/bigtable/admin/apiv2/adminpb"
	"cloud.google.com/go/civil"
	"cloud.google.com/go/iam"
	"cloud.google.com/go/internal"
	"cloud.google.com/go/internal/optional"
	"cloud.google.com/go/internal/testutil"
	"cloud.google.com/go/internal/uid"
	monitoring "cloud.google.com/go/monitoring/apiv3/v2"
	"cloud.google.com/go/monitoring/apiv3/v2/monitoringpb"
	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"
	gax "github.com/googleapis/gax-go/v2"
	"google.golang.org/api/iterator"
	grpc "google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/timestamppb"
)

const (
	timeUntilResourceCleanup  = time.Hour * 12 // 12 hours
	prefixOfInstanceResources = "bt-it-"
	prefixOfClusterResources  = "bt-c-"
	maxCreateAttempts         = 10
	retryCreateSleep          = 10 * time.Second
)

var (
	// Backoffs: 10s 13s 16.9s 21.97s 28.561s 37.12s ......
	retryCreateBackoff = gax.Backoff{
		Initial:    retryCreateSleep, // 10s
		Max:        time.Minute,
		Multiplier: 1.30,
	}

	presidentsSocialGraph = map[string][]string{
		"wmckinley":   {"tjefferson"},
		"gwashington": {"j§adams"},
		"tjefferson":  {"gwashington", "j§adams"},
		"j§adams":     {"gwashington", "tjefferson"},
	}

	clusterUIDSpace       = uid.NewSpace(prefixOfClusterResources, &uid.Options{Short: true})
	tableNameSpace        = uid.NewSpace("cbt-test", &uid.Options{Short: true})
	myTableNameSpace      = uid.NewSpace("mytable", &uid.Options{Short: true})
	myOtherTableNameSpace = uid.NewSpace("myothertable", &uid.Options{Short: true})
)

/*
|             |              follows               |
|    _key     |------------------------------------|
|             | tjefferson | j§adams | gwashington |
|-------------|------------|---------|-------------|
| wmckinley   |      1     |         |             |
| gwashington |            |    1    |             |
| tjefferson  |            |    1    |     1       |
| j§adams     |      1     |         |     1       |
*/
func populatePresidentsGraph(table *Table, prefix string) error {
	ctx := context.Background()
	for rowKey, ss := range presidentsSocialGraph {
		mut := NewMutation()
		for _, name := range ss {
			mut.Set("follows", prefix+name, 1000, []byte("1"))
		}
		if err := table.Apply(ctx, prefix+rowKey, mut); err != nil {
			return fmt.Errorf("Mutating row %q: %v", prefix+rowKey, err)
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
	if sharedAdminClient != nil && sharedTableName != "" {
		_ = sharedAdminClient.DeleteTable(context.Background(), sharedTableName)
	}
	// Cleanup resources marked with bt-it- after a time delay
	if !c.UseProd {
		return nil
	}
	ctx := context.Background()
	iac, err := NewInstanceAdminClient(ctx, c.Project, c.ClientOpts...)
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
				deleteInstance(ctx, iac, instanceInfo.Name)
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

func TestIntegration_Pinger(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	testEnv, client, _, _, _, cleanup, err := setupIntegration(ctx, t)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { cleanup() })
	if !testEnv.Config().UseProd {
		t.Skip("emulator doesn't support PingAndWarm")
	}
	if err := client.PingAndWarm(ctx); err != nil {
		t.Fatalf("pinger failed. got %v, want %v", err, nil)
	}

}

func TestIntegration_UpdateFamilyValueType(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	_, _, adminClient, _, _, cleanup, err := setupIntegration(ctx, t)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(cleanup)

	myTableName := fmt.Sprintf("table-%d", time.Now().UnixNano())
	if err := createTable(ctx, adminClient, myTableName); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { deleteTable(context.Background(), t, adminClient, myTableName) })

	famUID := uid.NewSpace("fam-", &uid.Options{Short: true})
	familyName := famUID.New()
	// Create a new column family
	if err = createColumnFamily(ctx, t, adminClient, myTableName, familyName, nil); err != nil {
		t.Fatalf("Failed to create column family: %v", err)
	}
	// the type of the family is not aggregate
	table, err := adminClient.getTable(ctx, myTableName, btapb.Table_SCHEMA_VIEW)
	if err != nil {
		t.Fatalf("Failed to get table: %v", err)
	}
	family := table.GetColumnFamilies()[familyName]
	if family.ValueType.GetAggregateType() != nil {
		t.Fatalf("New column family cannot be aggregate type")
	}

	// Update column family type to string type should be successful
	update := Family{
		ValueType: StringType{
			Encoding: StringUtf8BytesEncoding{},
		},
	}

	err = retry(ctx, func() error { return adminClient.UpdateFamily(ctx, myTableName, familyName, update) }, nil)
	if err != nil {
		t.Fatalf("Failed to update value type of family %s with current type %v: %v", familyName, family.ValueType, err)
	}
	// Get FamilyInfo to check if the type is updated
	table, err = adminClient.getTable(ctx, myTableName, btapb.Table_SCHEMA_VIEW)
	if err != nil {
		t.Fatalf("Failed to get table info: %v", err)
	}
	family = table.GetColumnFamilies()[familyName]
	if !testutil.Equal(family.ValueType, update.ValueType.proto()) {
		t.Fatalf("got %v, want %v", family.ValueType, update.ValueType.proto())
	}
}

func TestIntegration_Aggregates(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	_, _, ac, table, tableName, cleanup, err := setupIntegration(ctx, t)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(cleanup)
	key := fmt.Sprintf("some-key-agg-%d", time.Now().UnixNano())
	family := "sum"
	column := "col"
	mut := NewMutation()
	mut.AddIntToCell(family, column, 1000, 5)

	// Add 5 to empty cell.
	if err := table.Apply(ctx, key, mut); err != nil {
		t.Fatalf("Mutating row %q: %v", key, err)
	}
	row, err := table.ReadRow(ctx, key)
	if err != nil {
		t.Fatalf("Reading a row: %v", err)
	}
	wantRow := Row{
		family: []ReadItem{
			{Row: key, Column: fmt.Sprintf("%s:%s", family, column), Timestamp: 1000, Value: binary.BigEndian.AppendUint64([]byte{}, 5)},
		},
	}
	if !testutil.Equal(row, wantRow) {
		t.Fatalf("Read row mismatch.\n got %#v\nwant %#v", row, wantRow)
	}

	// Add 5 again.
	if err := table.Apply(ctx, key, mut); err != nil {
		t.Fatalf("Mutating row %q: %v", key, err)
	}
	row, err = table.ReadRow(ctx, key)
	if err != nil {
		t.Fatalf("Reading a row: %v", err)
	}
	wantRow = Row{
		family: []ReadItem{
			{Row: key, Column: fmt.Sprintf("%s:%s", family, column), Timestamp: 1000, Value: binary.BigEndian.AppendUint64([]byte{}, 10)},
		},
	}
	if !testutil.Equal(row, wantRow) {
		t.Fatalf("Read row mismatch.\n got %#v\nwant %#v", row, wantRow)
	}

	// Merge 5, which translates in the backend to adding 5 for sum column families.
	mut2 := NewMutation()
	mut2.MergeBytesToCell(family, column, 1000, binary.BigEndian.AppendUint64([]byte{}, 5))
	if err := table.Apply(ctx, key, mut); err != nil {
		t.Fatalf("Mutating row %q: %v", key, err)
	}
	row, err = table.ReadRow(ctx, key)
	if err != nil {
		t.Fatalf("Reading a row: %v", err)
	}
	wantRow = Row{
		family: []ReadItem{
			{Row: key, Column: fmt.Sprintf("%s:%s", family, column), Timestamp: 1000, Value: binary.BigEndian.AppendUint64([]byte{}, 15)},
		},
	}
	if !testutil.Equal(row, wantRow) {
		t.Fatalf("Read row mismatch.\n got %#v\nwant %#v", row, wantRow)
	}

	err = ac.UpdateFamily(ctx, tableName, family, Family{ValueType: StringType{}})
	if err == nil {
		t.Fatalf("Expected UpdateFamily to fail, but it didn't")
	}
	wantErrorPattern := "Immutable fields 'value_type.aggregate_type.*' cannot be updated"
	if matched, _ := regexp.MatchString(wantErrorPattern, err.Error()); !matched {
		t.Errorf("Wrong error. Expected to match %q but was %v", wantErrorPattern, err)
	}
}

func TestIntegration_HighlyConcurrentReadsAndWrites(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	_, client, adminClient, _, _, cleanup, err := setupIntegration(ctx, t)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(cleanup)

	myTableName := fmt.Sprintf("it-hc-table-%d", time.Now().UnixNano())
	if err := createTable(ctx, adminClient, myTableName); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { deleteTable(context.Background(), t, adminClient, myTableName) })

	table := client.Open(myTableName)

	if err := createColumnFamily(ctx, t, adminClient, myTableName, "follows", nil); err != nil {
		t.Fatalf("Creating column family 'follows': %v", err)
	}

	uniqueID := time.Now().UnixNano()
	prefix := "hc-"

	tsUID := uid.NewSpace("ts-", &uid.Options{Short: true})
	tsFamily := tsUID.New()

	if err := populatePresidentsGraph(table, withUniqueID(prefix, prefix, uniqueID)); err != nil {
		t.Fatal(err)
	}

	if err := createColumnFamily(ctx, t, adminClient, myTableName, tsFamily, nil); err != nil {
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
				_, err := table.ReadRow(ctx, withUniqueID(prefix+"testrow", prefix, uniqueID), RowFilter(LatestNFilter(1)))
				if err != nil {
					t.Errorf("Concurrent read: %v", err)
				}
			case 30 <= r && r < 100:
				// Do a write.
				mut := NewMutation()
				mut.Set(tsFamily, "col", 1000, []byte("data"))
				if err := table.Apply(ctx, withUniqueID(prefix+"testrow", prefix, uniqueID), mut); err != nil {
					t.Errorf("Concurrent write: %v", err)
				}
			}
		}()
	}
	wg.Wait()
}

func TestIntegration_NoopMetricsProvider(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	testEnv, _, adminClient, _, _, cleanup, err := setupIntegration(ctx, t)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(cleanup)

	if testing.Short() || !testEnv.Config().UseProd {
		t.Skip("Skip long running tests in short mode or non-prod environments")
	}

	myTableName := fmt.Sprintf("it-noop-table-%d", time.Now().UnixNano())
	if err := createTable(ctx, adminClient, myTableName); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { deleteTable(context.Background(), t, adminClient, myTableName) })

	famUID := uid.NewSpace("export-", &uid.Options{Short: true})
	family := famUID.New()

	if err := createColumnFamily(ctx, t, adminClient, myTableName, family, nil); err != nil {
		t.Fatalf("Creating column family: %v", err)
	}

	// Retry client creation to bypass "FailedPrecondition: Table currently being created"
	// while the backend finishes registering the new table.
	noopClient, err := newClientWithConfigAndRetry(ctx, testEnv, ClientConfig{MetricsProvider: NoopMetricsProvider{}})
	if err != nil {
		t.Fatalf("NewClientWithConfig: %v", err)
	}

	noopTable := noopClient.Open(myTableName)

	uniqueID := time.Now().UnixNano()
	prefix := "noop-"

	for i := 0; i < 10; i++ {
		mut := NewMutation()
		mut.Set(family, "col", 1000, []byte("test"))
		if err := noopTable.Apply(ctx, fmt.Sprintf(withUniqueID(prefix+"row-%v", prefix, uniqueID), i), mut); err != nil {
			t.Fatalf("Apply: %v", err)
		}
	}

	err = noopTable.ReadRows(ctx, PrefixRange(withUniqueID(prefix+"row-", prefix, uniqueID)), func(r Row) bool {
		return true
	}, RowFilter(ColumnFilter("col")))
	if err != nil {
		t.Fatalf("ReadRows: %v", err)
	}
}

func TestIntegration_ExportBuiltInMetrics(t *testing.T) {
	// record start time
	testStartTime := time.Now()
	tsListStart := &timestamppb.Timestamp{
		Seconds: testStartTime.Unix(),
		Nanos:   int32(testStartTime.Nanosecond()),
	}

	origSamplePeriod := defaultSamplePeriod
	defaultSamplePeriod = 1 * time.Minute
	t.Cleanup(func() {
		defaultSamplePeriod = origSamplePeriod
	})

	ctx := context.Background()
	testEnv, _, adminClient, _, _, cleanup, err := setupIntegration(ctx, t)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(cleanup)

	if testing.Short() || !testEnv.Config().UseProd {
		t.Skip("Skip long running tests in short mode or non-prod environments")
	}

	client, err := testEnv.NewClient()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { closeClientAndLog(t, client, "client") })

	myTableName := fmt.Sprintf("it-exp-table-%d", time.Now().UnixNano())
	if err := createTable(ctx, adminClient, myTableName); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { deleteTable(context.Background(), t, adminClient, myTableName) })

	table := client.Open(myTableName)

	family := "export"
	if err := createColumnFamily(ctx, t, adminClient, myTableName, family, nil); err != nil {
		t.Fatalf("Creating column family: %v", err)
	}

	for i := 0; i < 10; i++ {
		mut := NewMutation()
		mut.Set(family, "col", 1000, []byte("test"))
		if err := table.Apply(ctx, fmt.Sprintf("export-row-%v", i), mut); err != nil {
			t.Fatalf("Apply: %v", err)
		}
	}

	err = table.ReadRows(ctx, PrefixRange("export-row-"), func(r Row) bool {
		return true
	}, RowFilter(ColumnFilter("col")))
	if err != nil {
		t.Fatalf("ReadRows: %v", err)
	}

	// Validate that metrics are exported
	// Rely on the retry loop below to wait for metrics propagation.

	monitoringClient, err := monitoring.NewMetricClient(ctx, testEnv.Config().ClientOpts...)
	if err != nil {
		t.Errorf("Failed to create metric client: %v", err)
	}
	metricNamesValidate := []string{
		metricNameOperationLatencies,
		metricNameAttemptLatencies,
		metricNameServerLatencies,
		metricNameFirstRespLatencies,
	}

	// Try for 5m with 10s sleep between retries
	testutil.Retry(t, 15, 10*time.Second, func(r *testutil.R) {
		for _, metricName := range metricNamesValidate {
			timeListEnd := time.Now()
			tsListEnd := &timestamppb.Timestamp{
				Seconds: timeListEnd.Unix(),
				Nanos:   int32(timeListEnd.Nanosecond()),
			}

			// ListTimeSeries can list only one metric type at a time.
			// So, call ListTimeSeries with different metric names
			iter := monitoringClient.ListTimeSeries(ctx, &monitoringpb.ListTimeSeriesRequest{
				Name: fmt.Sprintf("projects/%s", testEnv.Config().Project),
				Interval: &monitoringpb.TimeInterval{
					StartTime: tsListStart,
					EndTime:   tsListEnd,
				},
				Filter: fmt.Sprintf("metric.type = starts_with(\"bigtable.googleapis.com/client/%v\")", metricName),
			})

			// Assert at least 1 datapoint was exported
			_, err := iter.Next()
			if err != nil {
				r.Errorf("%v not exported\n", metricName)
			}
		}
	})
}

func TestIntegration_LargeReadsWritesAndScans(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	testEnv, client, adminClient, _, _, cleanup, err := setupIntegration(ctx, t)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(cleanup)

	if testing.Short() {
		t.Skip("Skip long running tests in short mode")
	}

	myTableName := fmt.Sprintf("it-large-table-%d", time.Now().UnixNano())
	if err := createTable(ctx, adminClient, myTableName); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { deleteTable(context.Background(), t, adminClient, myTableName) })

	table := client.Open(myTableName)

	uniqueID := time.Now().UnixNano()
	prefix := "large-"

	ts := uid.NewSpace("ts", &uid.Options{Short: true}).New()
	if err := createColumnFamily(ctx, t, adminClient, myTableName, ts, nil); err != nil {
		t.Fatalf("Creating column family: %v", err)
	}

	bigBytes := make([]byte, 5<<20) // 5 MB is larger than current default gRPC max of 4 MB, but less than the max we set.
	nonsense := []byte("lorem ipsum dolor sit amet, ")
	fill(bigBytes, nonsense)
	mut := NewMutation()
	mut.Set(ts, "col", 1000, bigBytes)
	if err := table.Apply(ctx, withUniqueID(prefix+"bigrow", prefix, uniqueID), mut); err != nil {
		t.Fatalf("Big write: %v", err)
	}
	verifyDirectPathRemoteAddress(testEnv, t)
	r, err := table.ReadRow(ctx, withUniqueID(prefix+"bigrow", prefix, uniqueID))
	if err != nil {
		t.Fatalf("Big read: %v", err)
	}
	verifyDirectPathRemoteAddress(testEnv, t)
	wantRow := Row{ts: []ReadItem{
		{Row: withUniqueID(prefix+"bigrow", prefix, uniqueID), Column: fmt.Sprintf("%s:col", ts), Timestamp: 1000, Value: bigBytes},
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
		row := fmt.Sprintf(withUniqueID(prefix+"row-%d", prefix, uniqueID), i)
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
	err = table.ReadRows(ctx, PrefixRange(withUniqueID(prefix+"row-", prefix, uniqueID)), func(r Row) bool {
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
	err = table.ReadRows(ctx, PrefixRange(withUniqueID(prefix+"row-", prefix, uniqueID)), func(r Row) bool {
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
	if err := createColumnFamily(ctx, t, adminClient, myTableName, bulk, nil); err != nil {
		t.Fatalf("Creating column family: %v", err)
	}
	bulkData := map[string][]string{
		withUniqueID(prefix+"red sox", prefix, uniqueID):  {"2004", "2007", "2013"},
		withUniqueID(prefix+"patriots", prefix, uniqueID): {"2001", "2003", "2004", "2014"},
		withUniqueID(prefix+"celtics", prefix, uniqueID):  {"1981", "1984", "1986", "2008"},
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
	status, err = table.ApplyBulk(ctx, []string{withUniqueID(prefix+"badrow", prefix, uniqueID), withUniqueID(prefix+"badrow2", prefix, uniqueID)}, []*Mutation{badMut, badMut2})
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

func TestIntegration_Presidents(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	testEnv, client, adminClient, _, _, cleanup, err := setupIntegration(ctx, t)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(cleanup)

	myTableName := fmt.Sprintf("it-pres-table-%d", time.Now().UnixNano())
	if err := createTable(ctx, adminClient, myTableName); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { deleteTable(context.Background(), t, adminClient, myTableName) })

	table := client.Open(myTableName)

	if err := createColumnFamily(ctx, t, adminClient, myTableName, "follows", nil); err != nil {
		t.Fatalf("Creating column family 'follows': %v", err)
	}

	uniqueID := time.Now().UnixNano()
	prefix := "pres-"

	if err := populatePresidentsGraph(table, withUniqueID("pres-", prefix, uniqueID)); err != nil {
		t.Fatal(err)
	}

	t.Run("PartialReadRows", func(t *testing.T) {
		// Do a scan and stop part way through.
		// Verify that the ReadRows callback doesn't keep running.
		stopped := false
		err = table.ReadRows(ctx, PrefixRange(withUniqueID(prefix, prefix, uniqueID)), func(r Row) bool {
			if r.Key() < withUniqueID(prefix+"h", prefix, uniqueID) {
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
	})

	t.Run("ReadRowList", func(t *testing.T) {
		// Read a RowList
		var elt []string
		keys := RowList{withUniqueID(prefix+"wmckinley", prefix, uniqueID), withUniqueID(prefix+"gwashington", prefix, uniqueID), withUniqueID(prefix+"j§adams", prefix, uniqueID)}
		want := withUniqueID("pres-gwashington-pres-j§adams-1,pres-j§adams-pres-gwashington-1,pres-j§adams-pres-tjefferson-1,pres-wmckinley-pres-tjefferson-1", prefix, uniqueID)
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
	})

	t.Run("ReadRowListReverse", func(t *testing.T) {
		// Read a RowList
		var elt []string
		rowRange := NewOpenClosedRange(withUniqueID(prefix+"gwashington", prefix, uniqueID), withUniqueID(prefix+"wmckinley", prefix, uniqueID))
		want := withUniqueID("pres-wmckinley-pres-tjefferson-1,pres-tjefferson-pres-gwashington-1,pres-tjefferson-pres-j§adams-1,pres-j§adams-pres-gwashington-1,pres-j§adams-pres-tjefferson-1", prefix, uniqueID)
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
	})

	t.Run("ConditionalMutations", func(t *testing.T) {
		// Do a conditional mutation with a complex filter.
		mutTrue := NewMutation()
		mutTrue.Set("follows", withUniqueID(prefix+"wmckinley", prefix, uniqueID), 1000, []byte("1"))
		filter := ChainFilters(ColumnFilter(withUniqueID(prefix+"gwash[iz].*", prefix, uniqueID)), ValueFilter("."))
		mut := NewCondMutation(filter, mutTrue, nil)
		if err := table.Apply(ctx, withUniqueID(prefix+"tjefferson", prefix, uniqueID), mut); err != nil {
			t.Fatalf("Conditionally mutating row: %v", err)
		}
		verifyDirectPathRemoteAddress(testEnv, t)
		// Do a second condition mutation with a filter that does not match,
		// and thus no changes should be made.
		mutTrue = NewMutation()
		mutTrue.DeleteRow()
		filter = ColumnFilter("snoop.dogg")
		mut = NewCondMutation(filter, mutTrue, nil)
		if err := table.Apply(ctx, withUniqueID(prefix+"tjefferson", prefix, uniqueID), mut); err != nil {
			t.Fatalf("Conditionally mutating row: %v", err)
		}
		verifyDirectPathRemoteAddress(testEnv, t)

		// Fetch a row.
		row, err := table.ReadRow(ctx, withUniqueID(prefix+"j§adams", prefix, uniqueID))
		if err != nil {
			t.Fatalf("Reading a row: %v", err)
		}
		verifyDirectPathRemoteAddress(testEnv, t)
		wantRow := Row{
			"follows": []ReadItem{
				{Row: withUniqueID(prefix+"j§adams", prefix, uniqueID), Column: withUniqueID("follows:pres-gwashington", prefix, uniqueID), Timestamp: 1000, Value: []byte("1")},
				{Row: withUniqueID(prefix+"j§adams", prefix, uniqueID), Column: withUniqueID("follows:pres-tjefferson", prefix, uniqueID), Timestamp: 1000, Value: []byte("1")},
			},
		}
		if !testutil.Equal(row, wantRow) {
			t.Fatalf("Read row mismatch.\n got %#v\nwant %#v", row, wantRow)
		}
	})

	t.Run("ReadModifyWrite", func(t *testing.T) {
		if err := createColumnFamily(ctx, t, adminClient, myTableName, "counter", nil); err != nil {
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
			row, err := table.ApplyReadModifyWrite(ctx, withUniqueID(prefix+"gwashington", prefix, uniqueID), step.rmw)
			if err != nil {
				t.Fatalf("ApplyReadModifyWrite %+v: %v", step.rmw, err)
			}
			verifyDirectPathRemoteAddress(testEnv, t)
			// Make sure the modified cell returned by the RMW operation has a timestamp.
			if row["counter"][0].Timestamp == 0 {
				t.Fatalf("RMW returned cell timestamp: got %v, want > 0", row["counter"][0].Timestamp)
			}
			clearTimestamps(row)
			wantRow := Row{"counter": []ReadItem{{Row: withUniqueID(prefix+"gwashington", prefix, uniqueID), Column: "counter:likes", Value: step.want}}}
			if !testutil.Equal(row, wantRow) {
				t.Fatalf("After %s,\n got %v\nwant %v", step.desc, row, wantRow)
			}
		}

		// Check for google-cloud-go/issues/723. RMWs that insert new rows should keep row order sorted in the emulator.
		_, err = table.ApplyReadModifyWrite(ctx, withUniqueID(prefix+"issue-723-2", prefix, uniqueID), appendRMW([]byte{0}))
		if err != nil {
			t.Fatalf("ApplyReadModifyWrite null string: %v", err)
		}
		verifyDirectPathRemoteAddress(testEnv, t)
		_, err = table.ApplyReadModifyWrite(ctx, withUniqueID(prefix+"issue-723-1", prefix, uniqueID), appendRMW([]byte{0}))
		if err != nil {
			t.Fatalf("ApplyReadModifyWrite null string: %v", err)
		}
		verifyDirectPathRemoteAddress(testEnv, t)
		// Get only the correct row back on read.
		r, err := table.ReadRow(ctx, withUniqueID(prefix+"issue-723-1", prefix, uniqueID))
		if err != nil {
			t.Fatalf("Reading row: %v", err)
		}
		verifyDirectPathRemoteAddress(testEnv, t)
		if r.Key() != withUniqueID(prefix+"issue-723-1", prefix, uniqueID) {
			t.Fatalf("ApplyReadModifyWrite: incorrect read after RMW,\n got %v\nwant %v", r.Key(), withUniqueID(prefix+"issue-723-1", prefix, uniqueID))
		}
	})

	t.Run("DeleteRow", func(t *testing.T) {
		// Delete a row and check it goes away.
		mut := NewMutation()
		mut.DeleteRow()
		if err := table.Apply(ctx, withUniqueID(prefix+"wmckinley", prefix, uniqueID), mut); err != nil {
			t.Fatalf("Apply DeleteRow: %v", err)
		}
		row, err := table.ReadRow(ctx, withUniqueID(prefix+"wmckinley", prefix, uniqueID))
		if err != nil {
			t.Fatalf("Reading a row after DeleteRow: %v", err)
		}
		if len(row) != 0 {
			t.Fatalf("Read non-zero row after DeleteRow: %v", row)
		}
	})
}

func TestIntegration_FullReadStats(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	testEnv, client, adminClient, _, _, cleanup, err := setupIntegration(ctx, t)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(cleanup)

	tableName := fmt.Sprintf("table-%d", time.Now().UnixNano())
	if err := createTable(ctx, adminClient, tableName); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := adminClient.DeleteTable(context.Background(), tableName); err != nil {
			t.Logf("Cleanup DeleteTable failed: %v", err)
		}
	})

	if err := createColumnFamily(ctx, t, adminClient, tableName, "follows", nil); err != nil {
		t.Fatal(err)
	}
	_ = createColumnFamilyWithConfig(context.Background(), t, adminClient, tableName, "sum", &Family{ValueType: AggregateType{
		Input:      Int64Type{},
		Aggregator: SumAggregator{},
	}}, map[codes.Code]bool{codes.Unavailable: true, codes.AlreadyExists: true})
	table := client.Open(tableName)

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
			t.Parallel()
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
	t.Parallel()
	ctx := context.Background()
	testEnv, client, adminClient, _, _, cleanup, err := setupIntegration(ctx, t)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(cleanup)

	presplitTable := fmt.Sprintf("presplit-table-%d", time.Now().UnixNano())
	if err := createPresplitTable(ctx, adminClient, presplitTable, []string{"follows"}); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := adminClient.DeleteTable(context.Background(), presplitTable); err != nil {
			t.Logf("Cleanup DeleteTable failed: %v", err)
		}
	})

	cf := uid.NewSpace("follows", &uid.Options{Short: true}).New()
	if err := createColumnFamily(ctx, t, adminClient, presplitTable, cf, nil); err != nil {
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
	t.Parallel()
	if testing.Short() {
		t.Skip("skipping heavy admin operation test in short mode")
	}

	ctx := context.Background()
	testEnv, _, adminClient, _, _, cleanup, err := setupIntegration(ctx, t)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(cleanup)

	timeout := 2 * time.Second
	if testEnv.Config().UseProd {
		timeout = 5 * time.Minute
	}
	ctx, cancel := context.WithTimeout(ctx, timeout)
	t.Cleanup(cancel)

	myTableName := myTableNameSpace.New()
	tableConf := TableConf{
		TableID: myTableName,
		Families: map[string]GCPolicy{
			"fam1": MaxVersionsPolicy(1),
			"fam2": MaxVersionsPolicy(2),
		},
		DeletionProtection: Protected,
	}

	if err := createTableFromConf(ctx, adminClient, &tableConf); err != nil {
		t.Fatalf("Create table from config: %v", err)
	}
	t.Cleanup(func() {
		// Unprotect the table first to allow deletion.
		if err := adminClient.UpdateTableWithDeletionProtection(context.Background(), tableConf.TableID, Unprotected); err != nil {
			t.Logf("Cleanup UpdateTableWithDeletionProtection failed: %v", err)
		}
		deleteTable(context.Background(), t, adminClient, tableConf.TableID)
	})

	table, err := adminClient.TableInfo(ctx, tableConf.TableID)
	if err != nil {
		t.Fatalf("Getting table info: %v", err)
	}

	if table.DeletionProtection != Protected {
		t.Errorf("Expect table deletion protection to be enabled for table: %v", tableConf.TableID)
	}

	// Check if the deletion protection works properly
	if err = adminClient.DeleteColumnFamily(ctx, tableConf.TableID, "fam1"); err == nil {
		t.Errorf("We shouldn't be able to delete the column family fam1 when the deletion protection is enabled for table %v", myTableName)
	}
	if err = adminClient.DeleteTable(ctx, tableConf.TableID); err == nil {
		t.Errorf("We shouldn't be able to delete the table when the deletion protection is enabled for table %v", myTableName)
	}

	if err := adminClient.UpdateTableWithDeletionProtection(ctx, tableConf.TableID, Unprotected); err != nil {
		t.Fatalf("Update table from config: %v", err)
	}

	table, err = adminClient.TableInfo(ctx, tableConf.TableID)
	if err != nil {
		t.Fatalf("Getting table info: %v", err)
	}

	if table.DeletionProtection != Unprotected {
		t.Errorf("Expect table deletion protection to be disabled for table: %v", tableConf.TableID)
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
	t.Parallel()
	t.Skip("flaky - blocking deps updates")
	ctx := context.Background()
	testEnv, _, adminClient, _, _, cleanup, err := setupIntegration(ctx, t)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(cleanup)

	timeout := 2 * time.Second
	if testEnv.Config().UseProd {
		timeout = 5 * time.Minute
	}
	ctx, cancel := context.WithTimeout(ctx, timeout)
	t.Cleanup(cancel)

	changeStreamRetention, err := time.ParseDuration("24h")
	if err != nil {
		t.Fatalf("ChangeStreamRetention not valid: %v", err)
	}

	myTableName := myTableNameSpace.New()
	tableConf := TableConf{
		TableID: myTableName,
		Families: map[string]GCPolicy{
			"fam1": MaxVersionsPolicy(1),
			"fam2": MaxVersionsPolicy(2),
		},
		ChangeStreamRetention: changeStreamRetention,
	}

	if err := createTableFromConf(ctx, adminClient, &tableConf); err != nil {
		t.Fatalf("Create table from config: %v", err)
	}

	table, err := adminClient.TableInfo(ctx, tableConf.TableID)
	if err != nil {
		t.Fatalf("Getting table info: %v", err)
	}

	if table.ChangeStreamRetention != changeStreamRetention {
		t.Errorf("Expect table change stream to be enabled for table: %v has info: %v", tableConf.TableID, table)
	}

	// Update retention
	changeStreamRetention, err = time.ParseDuration("70h")
	if err != nil {
		t.Fatalf("ChangeStreamRetention not valid: %v", err)
	}

	if err := adminClient.UpdateTableWithChangeStream(ctx, tableConf.TableID, changeStreamRetention); err != nil {
		t.Fatalf("Update table from config: %v", err)
	}

	table, err = adminClient.TableInfo(ctx, tableConf.TableID)
	if err != nil {
		t.Fatalf("Getting table info: %v", err)
	}

	if table.ChangeStreamRetention != changeStreamRetention {
		t.Errorf("Expect table change stream to be enabled for table: %v has info: %v", tableConf.TableID, table)
	}

	// Disable change stream
	if err := adminClient.UpdateTableDisableChangeStream(ctx, tableConf.TableID); err != nil {
		t.Fatalf("Update table from config: %v", err)
	}

	table, err = adminClient.TableInfo(ctx, tableConf.TableID)
	if err != nil {
		t.Fatalf("Getting table info: %v", err)
	}

	if table.ChangeStreamRetention != nil {
		t.Errorf("Expect table change stream to be disabled for table: %v has info: %v", tableConf.TableID, table)
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
	t.Parallel()
	ctx := context.Background()
	testEnv, _, adminClient, _, _, cleanup, err := setupIntegration(ctx, t)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(cleanup)

	if !testEnv.Config().UseProd {
		t.Skip("emulator doesn't support Automated Backups")
	}

	timeout := 5 * time.Minute
	ctx, cancel := context.WithTimeout(ctx, timeout)
	t.Cleanup(cancel)

	retentionPeriod, err := time.ParseDuration("72h")
	if err != nil {
		t.Fatalf("RetentionPeriod not valid: %v", err)
	}
	frequency, err := time.ParseDuration("24h")
	if err != nil {
		t.Fatalf("Frequency not valid: %v", err)
	}
	automatedBackupPolicy := TableAutomatedBackupPolicy{RetentionPeriod: retentionPeriod, Frequency: frequency}

	myTableName := myTableNameSpace.New()
	tableConf := TableConf{
		TableID: myTableName,
		Families: map[string]GCPolicy{
			"fam1": MaxVersionsPolicy(1),
			"fam2": MaxVersionsPolicy(2),
		},
		AutomatedBackupConfig: &automatedBackupPolicy,
	}

	if err := createTableFromConf(ctx, adminClient, &tableConf); err != nil {
		t.Fatalf("Create table from config: %v", err)
	}
	t.Cleanup(func() { deleteTable(context.Background(), t, adminClient, tableConf.TableID) })

	table, err := adminClient.TableInfo(ctx, tableConf.TableID)
	if err != nil {
		t.Fatalf("Getting table info: %v", err)
	}

	if table.AutomatedBackupConfig == nil {
		t.Errorf("Expect Automated Backup Policy to be enabled for table: %v has info: %v", tableConf.TableID, table)
	}
	tableAbp := table.AutomatedBackupConfig.(*TableAutomatedBackupPolicy)
	if !equalOptionalDuration(tableAbp.Frequency, automatedBackupPolicy.Frequency) {
		t.Errorf("Expect automated backup policy frequency to be set for table: %v has info: %v", tableConf.TableID, table)
	}
	if !equalOptionalDuration(tableAbp.RetentionPeriod, automatedBackupPolicy.RetentionPeriod) {
		t.Errorf("Expect automated backup policy retention period to be set for table: %v has info: %v", tableConf.TableID, table)
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
		if gotErr := adminClient.UpdateTableWithAutomatedBackupPolicy(ctx, tableConf.TableID, testcase.bkpPolicy); err != nil {
			t.Fatalf("%v: Update table from config: %v", testcase.desc, gotErr)
		}

		gotTable, gotErr := adminClient.TableInfo(ctx, tableConf.TableID)
		if gotErr != nil {
			t.Fatalf("%v: Getting table info: %v", testcase.desc, gotErr)
		}
		if gotTable.AutomatedBackupConfig == nil {
			t.Errorf("%v: Expect Automated Backup Policy to be enabled for table: %v has info: %v", testcase.desc, tableConf.TableID, gotTable)
		}

		gotTableAbp := gotTable.AutomatedBackupConfig.(*TableAutomatedBackupPolicy)
		if testcase.bkpPolicy.Frequency != nil && !equalOptionalDuration(gotTableAbp.Frequency, testcase.bkpPolicy.Frequency) {
			t.Errorf("%v: Expect automated backup policy frequency to be set for table: %v has info: %v", testcase.desc, tableConf.TableID, table)
		}
		if testcase.bkpPolicy.RetentionPeriod != nil && !equalOptionalDuration(gotTableAbp.RetentionPeriod, testcase.bkpPolicy.RetentionPeriod) {
			t.Errorf("%v: Expect automated backup policy retention period to be set for table: %v has info: %v", testcase.desc, tableConf.TableID, table)
		}
	}

	// Test disable automated backups
	if err := adminClient.UpdateTableDisableAutomatedBackupPolicy(ctx, tableConf.TableID); err != nil {
		t.Fatalf("Update table from config: %v", err)
	}

	table, err = adminClient.TableInfo(ctx, tableConf.TableID)
	if err != nil {
		t.Fatalf("Getting table info: %v", err)
	}

	if table.AutomatedBackupConfig != nil {
		t.Errorf("Expect table automated backups to be disabled for table: %v has info: %v", tableConf.TableID, table)
	}
}

func TestIntegration_CreateTableWithRowKeySchema(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	testEnv, _, adminClient, _, _, cleanup, err := setupIntegration(ctx, t)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(cleanup)

	if !testEnv.Config().UseProd {
		t.Skip("emulator doesn't support row key schema")
	}

	timeout := 5 * time.Minute
	ctx, cancel := context.WithTimeout(ctx, timeout)
	t.Cleanup(cancel)

	testCases := []struct {
		desc          string
		rks           StructType
		errorExpected bool
	}{
		{
			desc: "Create fail with conflict family and row key column",
			rks: StructType{
				Fields:   []StructField{{FieldName: "fam1", FieldType: Int64Type{Encoding: BigEndianBytesEncoding{}}}},
				Encoding: StructOrderedCodeBytesEncoding{},
			},
			errorExpected: true,
		},
		{
			desc: "Create fail with missing encoding in struct type",
			rks: StructType{
				Fields: []StructField{{FieldName: "myfield", FieldType: Int64Type{Encoding: BigEndianBytesEncoding{}}}},
			},
			errorExpected: true,
		},
		{
			desc: "Create fail on DelimitedBytes missing delimiter",
			rks: StructType{
				Fields:   []StructField{{FieldName: "myfield", FieldType: StringType{Encoding: StringUtf8BytesEncoding{}}}},
				Encoding: StructDelimitedBytesEncoding{},
			},
			errorExpected: true,
		},
		{
			desc: "Create with Singleton failed with more than 1 field",
			rks: StructType{
				Fields: []StructField{
					{FieldName: "myfield1", FieldType: StringType{Encoding: StringUtf8BytesEncoding{}}},
					{FieldName: "myfield2", FieldType: Int64Type{Encoding: BigEndianBytesEncoding{}}},
				},
				Encoding: StructSingletonEncoding{},
			},
			errorExpected: true,
		},
		{
			desc: "Create with Singleton ok",
			rks: StructType{
				Fields: []StructField{
					{FieldName: "myfield1", FieldType: StringType{Encoding: StringUtf8BytesEncoding{}}},
				},
				Encoding: StructSingletonEncoding{},
			},
		},
		{
			desc: "Create with OrderedCode ok",
			rks: StructType{
				Fields: []StructField{
					{FieldName: "myfield1", FieldType: StringType{Encoding: StringUtf8BytesEncoding{}}},
					{FieldName: "myfield2", FieldType: Int64Type{Encoding: BigEndianBytesEncoding{}}},
				},
				Encoding: StructOrderedCodeBytesEncoding{},
			},
		},
		{
			desc: "Create with DelimitedBytes ok",
			rks: StructType{
				Fields: []StructField{
					{FieldName: "myfield1", FieldType: StringType{Encoding: StringUtf8BytesEncoding{}}},
					{FieldName: "myfield2", FieldType: Int64Type{Encoding: BigEndianBytesEncoding{}}},
				},
				Encoding: StructDelimitedBytesEncoding{
					Delimiter: []byte{'#'},
				},
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.desc, func(t *testing.T) {
			t.Parallel()
			myTableName := myTableNameSpace.New()
			tableConf := TableConf{
				TableID: myTableName,
				Families: map[string]GCPolicy{
					"fam1": MaxVersionsPolicy(1),
					"fam2": MaxVersionsPolicy(2),
				},
			}

			tableConf.RowKeySchema = &tc.rks
			err := adminClient.CreateTableFromConf(ctx, &tableConf)

			if tc.errorExpected && err == nil {
				t.Fatalf("Want error from test: '%v', got nil", tc.desc)
			}

			if !tc.errorExpected && err != nil {
				t.Fatalf("Unexpected error: %v", err)
			}

			// get the table and see the new schema is updated
			tbl, err := adminClient.TableInfo(ctx, tableConf.TableID)
			if !tc.errorExpected && tbl.RowKeySchema == nil {
				t.Errorf("Expecting row key schema %v to be created in table, got nil", tc.rks)
			}

			if tbl != nil {
				// clean up table
				err = adminClient.DeleteTable(ctx, tableConf.TableID)
				if err != nil {
					t.Fatalf("Unexpected error trying to clean up table: %v", err)
				}
			}
		})
	}
}

func TestIntegration_UpdateRowKeySchemaInTable(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	testEnv, _, adminClient, _, _, cleanup, err := setupIntegration(ctx, t)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(cleanup)

	if !testEnv.Config().UseProd {
		t.Skip("emulator doesn't support Automated Backups")
	}

	timeout := 5 * time.Minute
	ctx, cancel := context.WithTimeout(ctx, timeout)
	t.Cleanup(cancel)

	testCases := []struct {
		desc          string
		updateRks     StructType
		errorExpected bool
		currentRks    *StructType
	}{
		{
			desc: "Update fail with conflicting family name",
			updateRks: StructType{
				Fields:   []StructField{{FieldName: "fam1", FieldType: Int64Type{Encoding: BigEndianBytesEncoding{}}}},
				Encoding: StructSingletonEncoding{},
			},
			errorExpected: true,
			currentRks:    nil,
		},
		{
			desc: "Update fail for table with existing row key schema",
			updateRks: StructType{
				Fields:   []StructField{{FieldName: "mycol", FieldType: Int64Type{Encoding: BigEndianBytesEncoding{}}}},
				Encoding: StructSingletonEncoding{},
			},
			currentRks: &StructType{
				Fields:   []StructField{{FieldName: "myfirstcol", FieldType: Int64Type{Encoding: BigEndianBytesEncoding{}}}},
				Encoding: StructDelimitedBytesEncoding{Delimiter: []byte{'#'}},
			},
			errorExpected: true,
		},
		{
			desc: "Update ok",
			updateRks: StructType{
				Fields: []StructField{
					{FieldName: "myfield", FieldType: Int64Type{Encoding: BigEndianBytesEncoding{}}},
					{FieldName: "myfield2", FieldType: StringType{Encoding: StringUtf8BytesEncoding{}}}},
				Encoding: StructDelimitedBytesEncoding{
					Delimiter: []byte{'#'},
				},
			},
			currentRks: nil,
		},
	}

	for _, tc := range testCases {
		myTableName := myTableNameSpace.New()
		tableConf := TableConf{
			TableID: myTableName,
			Families: map[string]GCPolicy{
				"fam1": MaxVersionsPolicy(1),
			},
		}
		if tc.currentRks != nil {
			tableConf.RowKeySchema = tc.currentRks
		}

		if err := adminClient.CreateTableFromConf(ctx, &tableConf); err != nil {
			t.Fatalf("Unexpected error trying to create table: %v", err)
		}
		t.Cleanup(func() {
			if err := adminClient.DeleteTable(context.Background(), tableConf.TableID); err != nil {
				t.Logf("Cleanup DeleteTable failed: %v", err)
			}
		})

		err = adminClient.UpdateTableWithRowKeySchema(ctx, tableConf.TableID, tc.updateRks)
		if tc.errorExpected && err == nil {
			t.Fatalf("Expecting error from test '%v', got nil", tc.desc)
		}

		if !tc.errorExpected && err != nil {
			t.Fatalf("Unexpected error from test '%v': %v", tc.desc, err)
		}

		// Get the table to check if the schema is updated
		tbl, err := adminClient.TableInfo(ctx, tableConf.TableID)
		if !tc.errorExpected && tbl.RowKeySchema == nil {
			t.Errorf("Expecting row key schema %v to be updated in table, got: %v", tc.updateRks, tbl)
		}

		// Clear schema ok
		if err = adminClient.UpdateTableRemoveRowKeySchema(ctx, tableConf.TableID); err != nil {
			t.Errorf("Unexpected error trying to clear row key schema: %v", err)
		}
	}
}

func TestIntegration_Admin(t *testing.T) {
	t.Parallel()
	if testing.Short() {
		t.Skip("skipping heavy admin operation test in short mode")
	}

	ctx := context.Background()
	testEnv, _, adminClient, _, _, cleanup, err := setupIntegration(ctx, t)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(cleanup)

	timeout := 2 * time.Second
	if testEnv.Config().UseProd {
		timeout = 5 * time.Minute
	}
	ctx, cancel := context.WithTimeout(ctx, timeout)
	t.Cleanup(cancel)

	iAdminClient, err := testEnv.NewInstanceAdminClient()
	if err != nil {
		t.Fatalf("NewInstanceAdminClient: %v", err)
	}
	if iAdminClient != nil {
		t.Cleanup(func() { iAdminClient.Close() })
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

	myTableName := myTableNameSpace.New()
	t.Cleanup(func() { deleteTable(context.Background(), t, adminClient, myTableName) })
	if err := createTable(ctx, adminClient, myTableName); err != nil {
		t.Fatalf("Creating table: %v", err)
	}

	myOtherTableName := myOtherTableNameSpace.New()
	t.Cleanup(func() { deleteTable(context.Background(), t, adminClient, myOtherTableName) })
	if err := createTable(ctx, adminClient, myOtherTableName); err != nil {
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
	if testEnv.Config().UseProd {
		tblConf.TieredStorageConfig = &TieredStorageConfig{
			InfrequentAccess: &TieredStorageIncludeIfOlderThan{
				Duration: 30 * 24 * time.Hour,
			},
		}
	}
	if err := createTableFromConf(ctx, adminClient, &tblConf); err != nil {
		t.Fatalf("Creating table from TableConf: %v", err)
	}
	t.Cleanup(func() { deleteTable(context.Background(), t, adminClient, tblConf.TableID) })

	tblInfo, err := adminClient.TableInfo(ctx, tblConf.TableID)
	if err != nil {
		t.Fatalf("Getting table info: %v", err)
	}
	sort.Strings(tblInfo.Families)
	wantFams := []string{"fam1", "fam2"}
	if !testutil.Equal(tblInfo.Families, wantFams) {
		t.Errorf("Column family mismatch, got %v, want %v", tblInfo.Families, wantFams)
	}

	if testEnv.Config().UseProd {
		if tblInfo.TieredStorageConfig == nil {
			t.Fatal("TieredStorageConfig is nil")
		}
		rule, ok := tblInfo.TieredStorageConfig.InfrequentAccess.(*TieredStorageIncludeIfOlderThan)
		if !ok {
			t.Fatalf("Unexpected rule type: %T", tblInfo.TieredStorageConfig.InfrequentAccess)
		}
		if optional.ToDuration(rule.Duration) != 30*24*time.Hour {
			t.Errorf("Unexpected IncludeIfOlderThan: %v, expected %v", optional.ToDuration(rule.Duration), 30*24*time.Hour)
		}

		// Update tiered storage config
		newDuration := 45 * 24 * time.Hour
		newConfig := TieredStorageConfig{
			InfrequentAccess: &TieredStorageIncludeIfOlderThan{
				Duration: newDuration,
			},
		}
		if err := adminClient.UpdateTableWithTieredStorageConfig(ctx, tableID, &newConfig); err != nil {
			t.Fatalf("UpdateTableWithTieredStorageConfig failed: %v", err)
		}

		tblInfo, err = adminClient.TableInfo(ctx, tableID)
		if err != nil {
			t.Fatalf("TableInfo failed after update: %v", err)
		}
		rule, ok = tblInfo.TieredStorageConfig.InfrequentAccess.(*TieredStorageIncludeIfOlderThan)
		if !ok {
			t.Fatalf("Unexpected rule type after update: %T", tblInfo.TieredStorageConfig.InfrequentAccess)
		}
		if optional.ToDuration(rule.Duration) != newDuration {
			t.Errorf("Unexpected IncludeIfOlderThan after update: %v, expected %v", optional.ToDuration(rule.Duration), newDuration)
		}

		// Remove tiered storage config
		if err := adminClient.UpdateTableRemoveTieredStorageConfig(ctx, tableID); err != nil {
			t.Fatalf("UpdateTableRemoveTieredStorageConfig failed: %v", err)
		}

		tblInfo, err = adminClient.TableInfo(ctx, tableID)
		if err != nil {
			t.Fatalf("TableInfo failed after removal: %v", err)
		}
		if tblInfo.TieredStorageConfig != nil {
			t.Errorf("TieredStorageConfig should be nil after removal, got %+v", tblInfo.TieredStorageConfig)
		}
	}

	// Populate mytable and drop row ranges
	if err = createColumnFamily(ctx, t, adminClient, myTableName, "cf", nil); err != nil {
		t.Fatalf("Creating column family: %v", err)
	}

	// Retry client creation to bypass "FailedPrecondition: Table currently being created"
	// while the backend finishes registering the new table.
	var client *Client
	client, err = newClientWithRetry(ctx, testEnv)
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}
	t.Cleanup(func() { closeClientAndLog(t, client, "client") })

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
		clusters, err := iAdminClient.Clusters(ctx, adminClient.instance)
		if err != nil {
			t.Fatalf("Clusters: %v", err)
		}
		wantLen := len(clusters)

		testutil.Retry(t, 10, 5*time.Second, func(r *testutil.R) {
			encryptionInfo, err := adminClient.EncryptionInfo(ctx, myTableName)
			if err != nil {
				r.Errorf("EncryptionInfo: %v", err)
				return
			}

			if got, want := len(encryptionInfo), wantLen; !cmp.Equal(got, want) {
				r.Errorf("Number of Clusters with Encryption Info: %v, want: %v", got, want)
				return
			}

			clusterEncryptionInfo := encryptionInfo[testEnv.Config().Cluster][0]
			if clusterEncryptionInfo.KMSKeyVersion != "" {
				r.Errorf("Encryption Info mismatch, got %v, want %v", clusterEncryptionInfo.KMSKeyVersion, 0)
			}
			if clusterEncryptionInfo.Type != GoogleDefaultEncryption {
				r.Errorf("Encryption Info mismatch, got %v, want %v", clusterEncryptionInfo.Type, GoogleDefaultEncryption)
			}
		})
	}

}

func TestIntegration_TableIam(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	testEnv, _, adminClient, _, _, cleanup, err := setupIntegration(ctx, t)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(cleanup)

	if !testEnv.Config().UseProd {
		t.Skip("emulator doesn't support IAM Policy creation")
	}

	timeout := 5 * time.Minute
	ctx, cancel := context.WithTimeout(ctx, timeout)
	t.Cleanup(cancel)

	myTableName := myTableNameSpace.New()
	t.Cleanup(func() { deleteTable(context.Background(), t, adminClient, myTableName) })
	if err := createTable(ctx, adminClient, myTableName); err != nil {
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
	t.Parallel()
	ctx := context.Background()
	testEnv, _, adminClient, _, _, cleanup, err := setupIntegration(ctx, t)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(cleanup)

	if !testEnv.Config().UseProd {
		t.Skip("emulator doesn't support IAM Policy creation")
	}
	timeout := 5 * time.Minute
	ctx, cancel := context.WithTimeout(ctx, timeout)
	t.Cleanup(cancel)

	cluster := testEnv.Config().Cluster

	tableUID := uid.NewSpace("it-bkpiam-", &uid.Options{Short: true})
	table := tableUID.New()

	t.Cleanup(func() { deleteTable(context.Background(), t, adminClient, table) })
	if err := createTable(ctx, adminClient, table); err != nil {
		t.Fatalf("Creating table: %v", err)
	}

	// Create backup.
	opts := &uid.Options{Sep: '_'}
	backupUUID := uid.NewSpace("backup", opts)
	backup := backupUUID.New()

	t.Cleanup(func() {
		deleteBackupAndLog(context.Background(), t, adminClient, cluster, backup)
	})
	if err = createBackup(ctx, adminClient, table, cluster, backup, time.Now().Add(8*time.Hour)); err != nil {
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
	t.Parallel()
	ctx := context.Background()
	testEnv, _, adminClient, _, _, cleanup, err := setupIntegration(ctx, t)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(cleanup)

	if !testEnv.Config().UseProd {
		t.Skip("emulator doesn't support IAM Policy creation")
	}
	timeout := 5 * time.Minute
	ctx, cancel := context.WithTimeout(ctx, timeout)
	t.Cleanup(cancel)

	table := tableNameSpace.New()

	t.Cleanup(func() { deleteTable(context.Background(), t, adminClient, table) })
	if err := createTable(ctx, adminClient, table); err != nil {
		t.Fatalf("Creating table: %v", err)
	}

	// Create authorized view.
	opts := &uid.Options{Sep: '_'}
	authorizedViewUUID := uid.NewSpace("authorizedView", opts)
	authorizedView := authorizedViewUUID.New()

	t.Cleanup(func() { adminClient.DeleteAuthorizedView(context.Background(), table, authorizedView) })

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
	t.Parallel()
	t.Skip("flaky - https://github.com/googleapis/google-cloud-go/issues/11927")
	if instanceToCreate == "" {
		t.Skip("instanceToCreate not set, skipping instance creation testing")
	}
	instanceUID := uid.NewSpace(prefixOfInstanceResources, &uid.Options{Short: true})
	instanceToCreate := instanceUID.New()

	ctx := context.Background()
	testEnv, _, _, _, _, cleanup, err := setupIntegration(ctx, t)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(cleanup)

	if !testEnv.Config().UseProd {
		t.Skip("emulator doesn't support instance creation")
	}

	timeout := 7 * time.Minute
	ctx, cancel := context.WithTimeout(ctx, timeout)
	t.Cleanup(cancel)

	iAdminClient, err := testEnv.NewInstanceAdminClient()
	if err != nil {
		t.Fatalf("NewInstanceAdminClient: %v", err)
	}
	t.Cleanup(func() { iAdminClient.Close() })

	clusterID := instanceToCreate + "-cluster"

	// Create a development instance
	conf := &InstanceConf{
		InstanceId:   instanceToCreate,
		ClusterId:    clusterID,
		DisplayName:  "test instance",
		Zone:         instanceToCreateZone,
		InstanceType: DEVELOPMENT,
		Labels:       map[string]string{"test-label-key": "test-label-value"},
		Tags:         map[string]string{"tagKeys/12345": "tagValues/6789"},
	}

	// CreateInstance can be flaky; retry before marking as failing.
	if err := createInstance(ctx, iAdminClient, conf); err != nil {
		t.Fatalf("CreateInstance: %v", err)
	}

	t.Cleanup(func() {
		if err := deleteInstance(context.Background(), iAdminClient, instanceToCreate); err != nil {
			t.Logf("Cleanup deleteInstance failed: %v", err)
		}
	})

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
		Tags:         map[string]string{"tagKeys/12345": "tagValues/6789"},
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
	t.Parallel()
	t.Skip("flaky - https://github.com/googleapis/google-cloud-go/issues/11268")
	if instanceToCreate == "" {
		t.Skip("instanceToCreate not set, skipping instance creation testing")
	}
	instanceUID := uid.NewSpace(prefixOfInstanceResources, &uid.Options{Short: true})
	instanceToCreate := instanceUID.New()

	testEnv, err := NewIntegrationEnv()
	if err != nil {
		t.Fatalf("IntegrationEnv: %v", err)
	}
	t.Cleanup(func() { testEnv.Close() })

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
	t.Cleanup(cancel)

	iAdminClient, err := testEnv.NewInstanceAdminClient()
	if err != nil {
		t.Fatalf("NewInstanceAdminClient: %v", err)
	}
	t.Cleanup(func() { closeClientAndLog(t, iAdminClient, "iAdminClient") })

	adminClient, err := testEnv.NewAdminClient()
	if err != nil {
		t.Fatalf("NewAdminClient: %v", err)
	}
	t.Cleanup(func() { closeClientAndLog(t, adminClient, "adminClient") })

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

	t.Cleanup(func() {
		if err := deleteInstance(context.Background(), iAdminClient, instanceToCreate); err != nil {
			t.Logf("Cleanup deleteInstance failed: %v", err)
		}
	})
	err = retry(ctx, func() error { return createInstanceWithClusters(ctx, iAdminClient, conf) },
		func() error { return deleteInstance(ctx, iAdminClient, conf.InstanceID) })
	if err != nil {
		t.Fatalf("CreateInstanceWithClusters: %v", err)
	}

	// Delete the table at the end of the test. Schedule ahead of time
	// in case the client fails
	t.Cleanup(func() { deleteTable(context.Background(), t, adminClient, table) })
	if err := createTable(ctx, adminClient, table); err != nil {
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

		time.Sleep(time.Second * 2)
	}
	if encryptionKeyVersion == "" {
		t.Fatalf("Encryption Key not created within allotted time end")
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

	// Validate Encryption Info retrieved via EncryptionInfo
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
	t.Cleanup(func() {
		deleteBackupAndLog(context.Background(), t, adminClient, clusterID, backupName)
	})
	if err = createBackup(ctx, adminClient, table, clusterID, backupName, time.Now().Add(8*time.Hour)); err != nil {
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

func TestIntegration_AdminUpdateInstanceAndSyncClusters(t *testing.T) {
	t.Parallel()
	if testing.Short() {
		t.Skip("skipping heavy admin operation test in short mode")
	}

	if instanceToCreate == "" {
		t.Skip("instanceToCreate not set, skipping instance update testing")
	}
	instanceUID := uid.NewSpace(prefixOfInstanceResources, &uid.Options{Short: true})
	instanceToCreate := instanceUID.New()

	ctx := context.Background()
	testEnv, _, _, _, _, cleanup, err := setupIntegration(ctx, t)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(cleanup)

	if !testEnv.Config().UseProd {
		t.Skip("emulator doesn't support instance creation")
	}

	timeout := 5 * time.Minute
	ctx, cancel := context.WithTimeout(ctx, timeout)
	t.Cleanup(cancel)

	iAdminClient, err := testEnv.NewInstanceAdminClient()
	if err != nil {
		t.Fatalf("NewInstanceAdminClient: %v", err)
	}
	t.Cleanup(func() { iAdminClient.Close() })

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
	t.Cleanup(func() {
		if err := deleteInstance(context.Background(), iAdminClient, instanceToCreate); err != nil {
			t.Logf("Cleanup deleteInstance failed: %v", err)
		}
	})
	if err := createInstance(ctx, iAdminClient, conf); err != nil {
		t.Fatalf("CreateInstance: %v", err)
	}

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

	// Label tests
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

func TestIntegration_Autoscaling(t *testing.T) {
	t.Parallel()
	if instanceToCreate == "" {
		t.Skip("instanceToCreate not set, skipping instance update testing")
	}
	instanceUID := uid.NewSpace(prefixOfInstanceResources, &uid.Options{Short: true})
	instanceToCreate := instanceUID.New()

	ctx := context.Background()
	testEnv, _, _, _, _, cleanup, err := setupIntegration(ctx, t)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(cleanup)

	if !testEnv.Config().UseProd {
		t.Skip("emulator doesn't support instance creation")
	}

	timeout := 15 * time.Minute
	ctx, cancel := context.WithTimeout(ctx, timeout)
	t.Cleanup(cancel)

	iAdminClient, err := testEnv.NewInstanceAdminClient()
	if err != nil {
		t.Fatalf("NewInstanceAdminClient: %v", err)
	}
	t.Cleanup(func() { iAdminClient.Close() })

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
	t.Cleanup(func() {
		if err := deleteInstance(context.Background(), iAdminClient, instanceToCreate); err != nil {
			t.Logf("Cleanup deleteInstance failed: %v", err)
		}
	})
	if err := createInstance(ctx, iAdminClient, conf); err != nil {
		t.Fatalf("CreateInstance: %v", err)
	}

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
	err = retry(ctx,
		func() error {
			return updateCluster(ctx, iAdminClient, instanceToCreate, clusterID, int32(serveNodes))
		}, nil)
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
	t.Parallel()
	ctx := context.Background()
	testEnv, _, _, _, _, cleanup, err := setupIntegration(ctx, t)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(cleanup)

	if !testEnv.Config().UseProd {
		t.Skip("emulator doesn't support snapshots")
	}

	iAdminClient, err := testEnv.NewInstanceAdminClient()
	if err != nil {
		t.Fatalf("NewInstanceAdminClient: %v", err)
	}
	t.Cleanup(func() { iAdminClient.Close() })

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
	t.Parallel()
	ctx := context.Background()
	testEnv, _, adminClient, _, _, cleanup, err := setupIntegration(ctx, t)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(cleanup)

	timeout := 2 * time.Second
	if testEnv.Config().UseProd {
		timeout = 5 * time.Minute
	}
	ctx, cancel := context.WithTimeout(ctx, timeout)
	t.Cleanup(cancel)

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

	myTableName := myTableNameSpace.New()
	t.Cleanup(func() { deleteTable(context.Background(), t, adminClient, myTableName) })
	if err := createTable(ctx, adminClient, myTableName); err != nil {
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

func TestIntegration_InstanceAdminClient_CreateAppProfile(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	testEnv, _, adminClient, _, _, cleanup, err := setupIntegration(ctx, t)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(cleanup)

	timeout := 2 * time.Second
	if testEnv.Config().UseProd {
		timeout = 5 * time.Minute
	}
	ctx, cancel := context.WithTimeout(ctx, timeout)
	t.Cleanup(cancel)

	iAdminClient, err := testEnv.NewInstanceAdminClient()
	if err != nil {
		t.Fatalf("NewInstanceAdminClient: %v", err)
	}
	if iAdminClient == nil {
		return
	}
	t.Cleanup(func() { iAdminClient.Close() })

	profileIDPrefix := "app_profile_id"
	uniqueID := make([]byte, 4)

	for _, testcase := range []struct {
		desc        string
		profileConf ProfileConf
		wantProfile *btapb.AppProfile
	}{
		{
			desc: "SingleClusterRouting",
			profileConf: ProfileConf{
				RoutingPolicy: SingleClusterRouting,
				ClusterID:     testEnv.Config().Cluster,
			},
			wantProfile: &btapb.AppProfile{
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
		{
			desc: "MultiClusterRouting",
			profileConf: ProfileConf{
				RoutingPolicy: MultiClusterRouting,
			},
			wantProfile: &btapb.AppProfile{
				RoutingPolicy: &btapb.AppProfile_MultiClusterRoutingUseAny_{
					MultiClusterRoutingUseAny: &btapb.AppProfile_MultiClusterRoutingUseAny{},
				},
				Isolation: &btapb.AppProfile_StandardIsolation_{
					StandardIsolation: &btapb.AppProfile_StandardIsolation{
						Priority: btapb.AppProfile_PRIORITY_HIGH,
					},
				},
			},
		},

		{
			desc: "MultiClusterRoutingUseAnyConfig no affinity",
			profileConf: ProfileConf{
				RoutingConfig: &MultiClusterRoutingUseAnyConfig{
					ClusterIDs: []string{testEnv.Config().Cluster},
				},
			},
			wantProfile: &btapb.AppProfile{
				RoutingPolicy: &btapb.AppProfile_MultiClusterRoutingUseAny_{
					MultiClusterRoutingUseAny: &btapb.AppProfile_MultiClusterRoutingUseAny{
						ClusterIds: []string{testEnv.Config().Cluster},
					},
				},
				Isolation: &btapb.AppProfile_StandardIsolation_{
					StandardIsolation: &btapb.AppProfile_StandardIsolation{
						Priority: btapb.AppProfile_PRIORITY_HIGH,
					},
				},
			},
		},
		{
			desc: "MultiClusterRoutingUseAnyConfig row affinity",
			profileConf: ProfileConf{
				RoutingConfig: &MultiClusterRoutingUseAnyConfig{
					ClusterIDs: []string{testEnv.Config().Cluster},
					Affinity:   &RowAffinity{},
				},
			},
			wantProfile: &btapb.AppProfile{
				RoutingPolicy: &btapb.AppProfile_MultiClusterRoutingUseAny_{
					MultiClusterRoutingUseAny: &btapb.AppProfile_MultiClusterRoutingUseAny{
						ClusterIds: []string{testEnv.Config().Cluster},
						Affinity:   &btapb.AppProfile_MultiClusterRoutingUseAny_RowAffinity_{},
					},
				},
				Isolation: &btapb.AppProfile_StandardIsolation_{
					StandardIsolation: &btapb.AppProfile_StandardIsolation{
						Priority: btapb.AppProfile_PRIORITY_HIGH,
					},
				},
			},
		},
		{
			desc: "SingleClusterRoutingConfig no Isolation",
			profileConf: ProfileConf{
				RoutingConfig: &SingleClusterRoutingConfig{
					ClusterID:                testEnv.Config().Cluster,
					AllowTransactionalWrites: true,
				},
			},
			wantProfile: &btapb.AppProfile{
				RoutingPolicy: &btapb.AppProfile_SingleClusterRouting_{
					SingleClusterRouting: &btapb.AppProfile_SingleClusterRouting{
						ClusterId:                testEnv.Config().Cluster,
						AllowTransactionalWrites: true,
					},
				},
				Isolation: &btapb.AppProfile_StandardIsolation_{
					StandardIsolation: &btapb.AppProfile_StandardIsolation{
						Priority: btapb.AppProfile_PRIORITY_HIGH,
					},
				},
			},
		},
		{
			desc: "SingleClusterRoutingConfig and low priority standard Isolation",
			profileConf: ProfileConf{
				RoutingConfig: &SingleClusterRoutingConfig{
					ClusterID:                testEnv.Config().Cluster,
					AllowTransactionalWrites: true,
				},
				Isolation: &StandardIsolation{
					Priority: AppProfilePriorityLow,
				},
			},
			wantProfile: &btapb.AppProfile{
				RoutingPolicy: &btapb.AppProfile_SingleClusterRouting_{
					SingleClusterRouting: &btapb.AppProfile_SingleClusterRouting{
						ClusterId:                testEnv.Config().Cluster,
						AllowTransactionalWrites: true,
					},
				},
				Isolation: &btapb.AppProfile_StandardIsolation_{
					StandardIsolation: &btapb.AppProfile_StandardIsolation{
						Priority: btapb.AppProfile_PRIORITY_LOW,
					},
				},
			},
		},
		{
			desc: "SingleClusterRoutingConfig and DataBoost Isolation HostPays ComputeBillingOwner",
			profileConf: ProfileConf{
				RoutingConfig: &SingleClusterRoutingConfig{
					ClusterID: testEnv.Config().Cluster,
				},
				Isolation: &DataBoostIsolationReadOnly{
					ComputeBillingOwner: HostPays,
				},
			},
			wantProfile: &btapb.AppProfile{
				RoutingPolicy: &btapb.AppProfile_SingleClusterRouting_{
					SingleClusterRouting: &btapb.AppProfile_SingleClusterRouting{
						ClusterId: testEnv.Config().Cluster,
					},
				},
				Isolation: &btapb.AppProfile_DataBoostIsolationReadOnly_{
					DataBoostIsolationReadOnly: &btapb.AppProfile_DataBoostIsolationReadOnly{
						ComputeBillingOwner: ptr(btapb.AppProfile_DataBoostIsolationReadOnly_HOST_PAYS),
					},
				},
			},
		},
	} {
		t.Run(testcase.desc, func(t *testing.T) {
			t.Parallel()
			cryptorand.Read(uniqueID)
			profileID := fmt.Sprintf("%s%x", profileIDPrefix, uniqueID)

			testcase.profileConf.ProfileID = profileID
			testcase.profileConf.InstanceID = adminClient.instance
			testcase.profileConf.Description = testcase.desc

			_, err := iAdminClient.CreateAppProfile(ctx, testcase.profileConf)
			if err != nil {
				t.Fatalf("Creating app profile: %v", err)
			}

			gotProfile, err := iAdminClient.GetAppProfile(ctx, adminClient.instance, profileID)
			if err != nil {
				t.Fatalf("Get app profile: %v", err)
			}

			t.Cleanup(func() {
				err = iAdminClient.DeleteAppProfile(context.Background(), adminClient.instance, profileID)
				if err != nil {
					t.Fatalf("Delete app profile: %v", err)
				}
			})

			testcase.wantProfile.Name = appProfilePath(testEnv.Config().Project, adminClient.instance, profileID)
			testcase.wantProfile.Description = testcase.desc
			if !proto.Equal(testcase.wantProfile, gotProfile) {
				t.Fatalf("profile: got: %s, want: %s", gotProfile, testcase.wantProfile)
			}

		})
	}
}

func TestIntegration_InstanceAdminClient_UpdateAppProfile(t *testing.T) {
	t.Parallel()
	if testing.Short() {
		t.Skip("skipping heavy admin operation test in short mode")
	}

	ctx := context.Background()
	testEnv, _, adminClient, _, _, cleanup, gotErr := setupIntegration(ctx, t)
	if gotErr != nil {
		t.Fatal(gotErr)
	}
	t.Cleanup(cleanup)

	timeout := 2 * time.Second
	if testEnv.Config().UseProd {
		timeout = 5 * time.Minute
	}
	ctx, cancel := context.WithTimeout(ctx, timeout)
	t.Cleanup(cancel)

	iAdminClient, gotErr := testEnv.NewInstanceAdminClient()
	if gotErr != nil {
		t.Fatalf("NewInstanceAdminClient: %v", gotErr)
	}
	if iAdminClient == nil {
		return
	}
	t.Cleanup(func() { iAdminClient.Close() })

	uniqueID := make([]byte, 4)
	rand.Read(uniqueID)
	profileID := fmt.Sprintf("app_profile_id%x", uniqueID)

	profile := ProfileConf{
		ProfileID:     profileID,
		InstanceID:    adminClient.instance,
		ClusterID:     testEnv.Config().Cluster,
		Description:   "creating new app profile 1",
		RoutingPolicy: SingleClusterRouting,
	}

	createdProfile, gotErr := iAdminClient.CreateAppProfile(ctx, profile)
	if gotErr != nil {
		t.Fatalf("Creating app profile: %v", gotErr)
	}

	gotProfile, gotErr := iAdminClient.GetAppProfile(ctx, adminClient.instance, profileID)
	if gotErr != nil {
		t.Fatalf("Get app profile: %v", gotErr)
	}

	t.Cleanup(func() {
		gotErr = iAdminClient.DeleteAppProfile(context.Background(), adminClient.instance, profileID)
		if gotErr != nil {
			t.Fatalf("Delete app profile: %v", gotErr)
		}
	})

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
		return profiles, gotErr
	}

	profiles, gotErr := list(adminClient.instance)
	if gotErr != nil {
		t.Fatalf("List app profile: %v", gotErr)
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
		desc        string
		uattrs      ProfileAttrsToUpdate
		wantProfile *btapb.AppProfile
		wantErrMsg  string
		skip        bool
	}{
		{
			desc:       "empty update",
			uattrs:     ProfileAttrsToUpdate{},
			wantErrMsg: "A non-empty 'update_mask' must be specified",
		},
		{
			desc:   "empty description update",
			uattrs: ProfileAttrsToUpdate{Description: ""},
			wantProfile: &btapb.AppProfile{
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
			desc: "routing update SingleClusterRouting",
			uattrs: ProfileAttrsToUpdate{
				RoutingPolicy: SingleClusterRouting,
				ClusterID:     testEnv.Config().Cluster,
			},
			wantProfile: &btapb.AppProfile{
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
		{
			desc: "routing only update MultiClusterRoutingUseAnyConfig",
			uattrs: ProfileAttrsToUpdate{
				RoutingConfig: &MultiClusterRoutingUseAnyConfig{
					ClusterIDs: []string{testEnv.Config().Cluster},
				},
			},
			wantProfile: &btapb.AppProfile{
				Name: gotProfile.Name,
				Etag: gotProfile.Etag,
				RoutingPolicy: &btapb.AppProfile_MultiClusterRoutingUseAny_{
					MultiClusterRoutingUseAny: &btapb.AppProfile_MultiClusterRoutingUseAny{
						ClusterIds: []string{testEnv.Config().Cluster},
					},
				},
				Isolation: &btapb.AppProfile_StandardIsolation_{
					StandardIsolation: &btapb.AppProfile_StandardIsolation{
						Priority: btapb.AppProfile_PRIORITY_HIGH,
					},
				},
			},
		},
		{
			desc: "routing only update SingleClusterRoutingConfig",
			uattrs: ProfileAttrsToUpdate{
				RoutingConfig: &SingleClusterRoutingConfig{
					ClusterID: testEnv.Config().Cluster,
				},
			},
			wantProfile: &btapb.AppProfile{
				Name: gotProfile.Name,
				Etag: gotProfile.Etag,
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
		{
			desc: "isolation only update DataBoost",
			uattrs: ProfileAttrsToUpdate{
				Isolation: &DataBoostIsolationReadOnly{
					ComputeBillingOwner: HostPays,
				},
			},
			wantProfile: &btapb.AppProfile{
				Name: gotProfile.Name,
				Etag: gotProfile.Etag,
				RoutingPolicy: &btapb.AppProfile_SingleClusterRouting_{
					SingleClusterRouting: &btapb.AppProfile_SingleClusterRouting{
						ClusterId: testEnv.Config().Cluster,
					},
				},
				Isolation: &btapb.AppProfile_DataBoostIsolationReadOnly_{
					DataBoostIsolationReadOnly: &btapb.AppProfile_DataBoostIsolationReadOnly{
						ComputeBillingOwner: ptr(btapb.AppProfile_DataBoostIsolationReadOnly_HOST_PAYS),
					},
				},
			},
			skip: true,
		},
	} {
		if test.skip {
			t.Logf("skipping test: %s", test.desc)
			continue
		}
		gotErr = iAdminClient.UpdateAppProfile(ctx, adminClient.instance, profileID, test.uattrs)
		if gotErr == nil && test.wantErrMsg != "" {
			t.Fatalf("%s: UpdateAppProfile: got: nil, want: error: %v", test.desc, test.wantErrMsg)
		}
		if gotErr != nil && test.wantErrMsg == "" {
			t.Fatalf("%s: UpdateAppProfile: got: %v, want: nil", test.desc, gotErr)
		}
		if gotErr != nil {
			continue
		}
		// Retry to see if the update has been completed
		testutil.Retry(t, 10, 10*time.Second, func(r *testutil.R) {
			got, _ := iAdminClient.GetAppProfile(ctx, adminClient.instance, profileID)
			if !proto.Equal(got, test.wantProfile) {
				r.Errorf("%s: got profile: %v,\n want profile: %v", test.desc, gotProfile, test.wantProfile)
			}
		})
	}
}

func TestIntegration_NodeScalingFactor(t *testing.T) {
	t.Parallel()
	if instanceToCreate == "" {
		t.Skip("instanceToCreate not set, skipping instance update testing")
	}
	instanceUID := uid.NewSpace(prefixOfInstanceResources, &uid.Options{Short: true})
	instanceToCreate := instanceUID.New()

	ctx := context.Background()
	testEnv, _, _, _, _, cleanup, err := setupIntegration(ctx, t)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(cleanup)

	if !testEnv.Config().UseProd {
		t.Skip("emulator doesn't support instance creation")
	}

	timeout := 10 * time.Minute
	ctx, cancel := context.WithTimeout(ctx, timeout)
	t.Cleanup(cancel)

	iAdminClient, err := testEnv.NewInstanceAdminClient()
	if err != nil {
		t.Fatalf("NewInstanceAdminClient: %v", err)
	}
	t.Cleanup(func() { iAdminClient.Close() })

	clusterID := instanceToCreate + "-cluster"
	wantNodeScalingFactor := NodeScalingFactor2X

	t.Log("creating an instance with node scaling factor")
	conf := &InstanceWithClustersConfig{
		InstanceID:  instanceToCreate,
		DisplayName: "test instance",
		Clusters: []ClusterConfig{
			{
				ClusterID:         clusterID,
				NumNodes:          2,
				NodeScalingFactor: wantNodeScalingFactor,
				Zone:              instanceToCreateZone,
			},
		},
	}
	t.Cleanup(func() {
		if err := deleteInstance(context.Background(), iAdminClient, instanceToCreate); err != nil {
			t.Logf("Cleanup deleteInstance failed: %v", err)
		}
	})
	err = retry(ctx, func() error { return createInstanceWithClusters(ctx, iAdminClient, conf) },
		func() error { return deleteInstance(ctx, iAdminClient, conf.InstanceID) })
	if err != nil {
		t.Fatalf("CreateInstanceWithClusters: %v", err)
	}

	cluster, err := iAdminClient.GetCluster(ctx, instanceToCreate, clusterID)
	if err != nil {
		t.Fatalf("GetCluster: %v", err)
	}

	if gotNodeScalingFactor := cluster.NodeScalingFactor; gotNodeScalingFactor != wantNodeScalingFactor {
		t.Fatalf("NodeScalingFactor: got: %v, want: %v", gotNodeScalingFactor, wantNodeScalingFactor)
	}
}

func TestIntegration_InstanceUpdate(t *testing.T) {
	ctx := context.Background()
	testEnv, _, adminClient, _, _, cleanup, err := setupIntegration(ctx, t)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(cleanup)

	timeout := 2 * time.Second
	if testEnv.Config().UseProd {
		timeout = 5 * time.Minute
	}
	ctx, cancel := context.WithTimeout(ctx, timeout)
	t.Cleanup(cancel)

	iAdminClient, err := testEnv.NewInstanceAdminClient()
	if err != nil {
		t.Fatalf("NewInstanceAdminClient: %v", err)
	}

	if iAdminClient == nil {
		return
	}

	t.Cleanup(func() { iAdminClient.Close() })

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

	cis, err := iAdminClient.GetCluster(ctx, adminClient.instance, testEnv.Config().Cluster)
	if err != nil {
		t.Fatalf("GetCluster: %v", err)
	}
	origNodes := cis.ServeNodes
	t.Cleanup(func() {
		_ = updateCluster(context.Background(), iAdminClient, adminClient.instance, testEnv.Config().Cluster, int32(origNodes))
	})

	const numNodes = 4
	// update cluster nodes
	if err := retry(ctx,
		func() error {
			return updateCluster(ctx, iAdminClient, adminClient.instance, testEnv.Config().Cluster, int32(numNodes))
		}, nil); err != nil {
		t.Errorf("UpdateCluster: %v", err)
	}

	// get cluster after updating
	cis, err = iAdminClient.GetCluster(ctx, adminClient.instance, testEnv.Config().Cluster)
	if err != nil {
		t.Errorf("GetCluster %v", err)
	}
	if cis.ServeNodes != int(numNodes) {
		t.Errorf("ServeNodes returned %d, want %d", cis.ServeNodes, int(numNodes))
	}
}

func createRandomInstance(ctx context.Context, iAdminClient *InstanceAdminClient) (string, string, error) {
	newConf := InstanceConf{
		InstanceId:   generateNewInstanceName(),
		ClusterId:    clusterUIDSpace.New(),
		DisplayName:  "different test sourceInstance",
		Zone:         instanceToCreateZone2,
		InstanceType: DEVELOPMENT,
		Labels:       map[string]string{"test-label-key-diff": "test-label-value-diff"},
	}
	err := createInstance(ctx, iAdminClient, &newConf)
	return newConf.InstanceId, newConf.ClusterId, err
}

func createInstance(ctx context.Context, iAdminClient *InstanceAdminClient, iConf *InstanceConf) error {
	return retry(ctx, func() error { return iAdminClient.CreateInstance(ctx, iConf) },
		func() error { return deleteInstance(ctx, iAdminClient, iConf.InstanceId) },
	)
}

func createInstanceWithClusters(ctx context.Context, iAdminClient *InstanceAdminClient, iConf *InstanceWithClustersConfig) error {
	return retry(ctx, func() error { return iAdminClient.CreateInstanceWithClusters(ctx, iConf) },
		func() error { return deleteInstance(ctx, iAdminClient, iConf.InstanceID) },
	)
}

func updateCluster(ctx context.Context, iAdminClient *InstanceAdminClient, instanceID, clusterID string, serveNodes int32) error {
	return retry(ctx, func() error {
		return iAdminClient.UpdateCluster(ctx, instanceID, clusterID, serveNodes)
	}, nil)
}

func deleteInstance(ctx context.Context, iAdminClient *InstanceAdminClient, instanceID string) error {
	return retry(ctx, func() error {
		return iAdminClient.DeleteInstance(ctx, instanceID)
	}, nil)
}

func createBackup(ctx context.Context, adminClient *AdminClient, table, cluster, backup string, expireTime time.Time) error {
	return retry(ctx, func() error {
		return adminClient.CreateBackup(ctx, table, cluster, backup, expireTime)
	}, nil)
}

func createBackupWithOptions(ctx context.Context, adminClient *AdminClient, table, cluster, backup string, opts ...BackupOption) error {
	return retry(ctx, func() error {
		return adminClient.CreateBackupWithOptions(ctx, table, cluster, backup, opts...)
	}, nil)
}

func deleteBackup(ctx context.Context, adminClient *AdminClient, cluster, backup string) error {
	return retry(ctx, func() error {
		return adminClient.DeleteBackup(ctx, cluster, backup)
	}, nil)
}

func copyBackup(ctx context.Context, adminClient *AdminClient, srcCluster, srcBackup, destProject, destInstance, destCluster, destBackup string, expireTime time.Time) error {
	return retry(ctx, func() error {
		return adminClient.CopyBackup(ctx, srcCluster, srcBackup, destProject, destInstance, destCluster, destBackup, expireTime)
	}, nil)
}

func updateBackup(ctx context.Context, adminClient *AdminClient, cluster, backup string, expireTime time.Time) error {
	return retry(ctx, func() error {
		return adminClient.UpdateBackup(ctx, cluster, backup, expireTime)
	}, nil)
}

func updateBackupHotToStandardTime(ctx context.Context, adminClient *AdminClient, cluster, backup string, hotToStandardTime time.Time) error {
	return retry(ctx, func() error {
		return adminClient.UpdateBackupHotToStandardTime(ctx, cluster, backup, hotToStandardTime)
	}, nil)
}

func updateBackupRemoveHotToStandardTime(ctx context.Context, adminClient *AdminClient, cluster, backup string) error {
	return retry(ctx, func() error {
		return adminClient.UpdateBackupRemoveHotToStandardTime(ctx, cluster, backup)
	}, nil)
}

func createLogicalView(ctx context.Context, iAdminClient *InstanceAdminClient, instance string, conf *LogicalViewInfo) error {
	return retry(ctx, func() error {
		return iAdminClient.CreateLogicalView(ctx, instance, conf)
	}, nil)
}

func updateLogicalView(ctx context.Context, iAdminClient *InstanceAdminClient, instance string, conf LogicalViewInfo) error {
	return retry(ctx, func() error {
		return iAdminClient.UpdateLogicalView(ctx, instance, conf)
	}, nil)
}

func deleteLogicalView(ctx context.Context, iAdminClient *InstanceAdminClient, instance, logicalView string) error {
	return retry(ctx, func() error {
		return iAdminClient.DeleteLogicalView(ctx, instance, logicalView)
	}, nil)
}

func TestIntegration_AdminCopyBackup(t *testing.T) {
	t.Parallel()
	if testing.Short() {
		t.Skip("skipping heavy admin operation test in short mode")
	}

	ctx := context.Background()
	testEnv, _, srcAdminClient, _, _, cleanup, err := setupIntegration(ctx, t)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(cleanup)

	if !testEnv.Config().UseProd {
		t.Skip("emulator doesn't support backups")
	}

	timeout := 15 * time.Minute
	ctx, cancel := context.WithTimeout(ctx, timeout)
	t.Cleanup(cancel)

	tableUID := uid.NewSpace("it-cpy-src-", &uid.Options{Short: true})
	tblConf := TableConf{
		TableID: tableUID.New(),
		Families: map[string]GCPolicy{
			"fam1": MaxVersionsPolicy(1),
			"fam2": MaxVersionsPolicy(2),
		},
	}
	t.Cleanup(func() { deleteTable(context.Background(), t, srcAdminClient, tblConf.TableID) })
	if err := createTableFromConf(ctx, srcAdminClient, &tblConf); err != nil {
		t.Fatalf("Creating table from TableConf: %v", err)
	}

	// Create source backup
	copyBackupUID := uid.NewSpace(prefixOfInstanceResources, &uid.Options{})
	backupUID := uid.NewSpace(prefixOfInstanceResources, &uid.Options{})
	srcBackupName := backupUID.New()
	srcCluster := testEnv.Config().Cluster
	t.Cleanup(func() {
		deleteBackupAndLog(context.Background(), t, srcAdminClient, srcCluster, srcBackupName)
	})
	if err = createBackup(ctx, srcAdminClient, tblConf.TableID, srcCluster, srcBackupName, time.Now().Add(100*time.Hour)); err != nil {
		t.Fatalf("Creating backup: %v", err)
	}
	wantSourceBackup := srcAdminClient.instancePrefix() + "/clusters/" + srcCluster + "/backups/" + srcBackupName

	destProj1 := testEnv.Config().Project
	destProj1Inst1 := testEnv.Config().Instance // 1st instance in 1st destination project
	destProj1Inst1Cl1 := srcCluster             // 1st cluster in 1st instance in 1st destination project

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
		t.Cleanup(func() { closeClientAndLog(t, destAdminClient, "destAdminClient") })

		// Copy Backup
		destBackupName := copyBackupUID.New()
		t.Cleanup(func() {
			deleteBackupAndLog(destCtx, t, destAdminClient, destCluster, destBackupName)
		})
		err = copyBackup(destCtx, srcAdminClient, srcCluster, srcBackupName, destProject, destInstance, destCluster, destBackupName, time.Now().Add(24*time.Hour))
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
	t.Parallel()
	if testing.Short() {
		t.Skip("skipping heavy admin operation test in short mode")
	}

	ctx := context.Background()
	testEnv, _, adminClient, _, _, cleanup, err := setupIntegration(ctx, t)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(cleanup)

	if !testEnv.Config().UseProd {
		t.Skip("emulator doesn't support backups")
	}

	timeout := 15 * time.Minute
	ctx, cancel := context.WithTimeout(ctx, timeout)
	t.Cleanup(cancel)

	tableUID := uid.NewSpace("it-bkp-src-", &uid.Options{Short: true})
	tblConf := TableConf{
		TableID: tableUID.New(),
		Families: map[string]GCPolicy{
			"fam1": MaxVersionsPolicy(1),
			"fam2": MaxVersionsPolicy(2),
		},
	}
	if err := createTableFromConf(ctx, adminClient, &tblConf); err != nil {
		t.Fatalf("Creating table from TableConf: %v", err)
	}
	// Delete the table at the end of the test. Schedule ahead of time
	// in case the client fails
	t.Cleanup(func() { deleteTable(context.Background(), t, adminClient, tblConf.TableID) })

	sourceInstance := testEnv.Config().Instance
	sourceCluster := testEnv.Config().Cluster

	iAdminClient, err := testEnv.NewInstanceAdminClient()
	if err != nil {
		t.Fatalf("NewInstanceAdminClient: %v", err)
	}
	t.Cleanup(func() { iAdminClient.Close() })

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

	// Create standard backup
	if err != nil {
		t.Fatalf("Failed to generate a unique ID: %v", err)
	}

	backupUID := uid.NewSpace("mybackup-", &uid.Options{})

	stdBkpName := backupUID.New()
	t.Cleanup(func() {
		deleteBackupAndLog(context.Background(), t, adminClient, sourceCluster, stdBkpName)
	})
	if err = createBackup(ctx, adminClient, tblConf.TableID, sourceCluster, stdBkpName, time.Now().Add(8*time.Hour)); err != nil {
		t.Fatalf("Creating backup: %v", err)
	}

	// Create hot backup with hot_to_standard_time
	hotBkpName1 := backupUID.New()
	t.Cleanup(func() {
		deleteBackupAndLog(context.Background(), t, adminClient, sourceCluster, hotBkpName1)
	})
	wantHtsTime := time.Now().Truncate(time.Second).Add(48 * time.Hour)
	if err = createBackupWithOptions(ctx, adminClient, tblConf.TableID, sourceCluster, hotBkpName1, WithExpiry(time.Now().Add(8*time.Hour)), WithHotToStandardBackup(wantHtsTime)); err != nil {
		t.Fatalf("Creating backup: %v", err)
	}

	// Create hot backup without hot_to_standard_time
	hotBkpName2 := backupUID.New()
	t.Cleanup(func() {
		deleteBackupAndLog(context.Background(), t, adminClient, sourceCluster, hotBkpName2)
	})
	if err = createBackupWithOptions(ctx, adminClient, tblConf.TableID, sourceCluster, hotBkpName2, WithExpiry(time.Now().Add(8*time.Hour)), WithHotBackup()); err != nil {
		t.Fatalf("Creating backup: %v", err)
	}

	// List backup
	var gotBackups []*BackupInfo
	testutil.Retry(t, 120, 5*time.Second, func(r *testutil.R) {
		var err error
		gotBackups, err = list(sourceCluster)
		if err != nil {
			r.Fatalf("Listing backups: %v", err)
		}

		wantBackups := map[string]struct {
			HotToStandardTime *time.Time
			BackupType        BackupType
		}{
			stdBkpName: {
				BackupType: BackupTypeStandard,
			},
			hotBkpName1: {
				BackupType:        BackupTypeHot,
				HotToStandardTime: &wantHtsTime,
			},
			hotBkpName2: {
				BackupType: BackupTypeHot,
			},
		}

		foundBackups := map[string]bool{}
		for _, gotBackup := range gotBackups {
			wantBackup, ok := wantBackups[gotBackup.Name]
			if !ok {
				continue
			}
			foundBackups[gotBackup.Name] = true

			if got, want := gotBackup.SourceTable, tblConf.TableID; got != want {
				r.Errorf("%v SourceTable got: %s, want: %s", gotBackup.Name, got, want)
			}
			if got, want := gotBackup.ExpireTime, gotBackup.StartTime.Add(8*time.Hour); math.Abs(got.Sub(want).Minutes()) > 1 {
				r.Errorf("%v ExpireTime got: %s, want: %s", gotBackup.Name, got, want)
			}
			if got, want := gotBackup.BackupType, wantBackup.BackupType; got != want {
				r.Errorf("%v BackupType got: %v, want: %v", gotBackup.Name, got, want)
			}
			if got, want := gotBackup.HotToStandardTime, wantBackup.HotToStandardTime; (got != nil && !got.Equal(*want)) ||
				(got == nil && got != want) || (want == nil && got != want) {
				r.Errorf("%v HotToStandardTime got: %v, want: %v", gotBackup.Name, got, want)
			}
		}

		if len(foundBackups) != len(wantBackups) {
			r.Errorf("foundBackups: %+v, wantBackups: %+v", foundBackups, wantBackups)
		}
	})

	// Get BackupInfo
	gotBackupInfo, err := adminClient.BackupInfo(ctx, sourceCluster, stdBkpName)
	if err != nil {
		t.Fatalf("BackupInfo: %v", gotBackupInfo)
	}
	if got, want := *gotBackupInfo, *gotBackups[0]; cmp.Equal(got, &want) {
		t.Errorf("BackupInfo: %v, want: %v", got, want)
	}

	// Update backup
	newExpireTime := time.Now().Add(10 * time.Hour)
	err = updateBackup(ctx, adminClient, sourceCluster, stdBkpName, newExpireTime)
	if err != nil {
		t.Fatalf("UpdateBackup failed: %v", err)
	}

	// Check that updated backup has the correct expire time
	updatedBackup, err := adminClient.BackupInfo(ctx, sourceCluster, stdBkpName)
	if err != nil {
		t.Fatalf("BackupInfo: %v", err)
	}
	gotBackupInfo.ExpireTime = newExpireTime
	// Server clock and local clock may not be perfectly sync'ed.
	if got, want := *updatedBackup, *gotBackupInfo; got.ExpireTime.Sub(want.ExpireTime) > time.Minute {
		t.Errorf("BackupInfo: %v, want: %v", got, want)
	}

	// Restore backup
	restoredTable := tblConf.TableID + "-restored"
	t.Cleanup(func() { deleteTable(context.Background(), t, adminClient, restoredTable) })
	if err = adminClient.RestoreTable(ctx, restoredTable, sourceCluster, stdBkpName); err != nil {
		t.Fatalf("RestoreTable: %v", err)
	}
	if _, err := adminClient.TableInfo(ctx, restoredTable); err != nil {
		t.Fatalf("Restored TableInfo: %v", err)
	}

	// If 'it.run-create-instance-tests' flag is set while running the tests,
	// instanceToCreate will be non-empty string.
	// Add more testcases if instanceToCreate is non-empty string
	if instanceToCreate != "" {
		// Create different instance to restore table.
		diffInstance, diffCluster, err := createRandomInstance(ctx, iAdminClient)
		if err != nil {
			t.Fatalf("CreateInstance: %v", err)
		}
		t.Cleanup(func() {
			if err := deleteInstance(context.Background(), iAdminClient, diffInstance); err != nil {
				t.Logf("Cleanup deleteInstance failed: %v", err)
			}
		})

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
			t.Fatalf("NewAdminClient: %v", err)
		}
		t.Cleanup(func() { dAdminClient.Close() })

		t.Cleanup(func() { deleteTable(context.Background(), t, dAdminClient, restoreTableName) })
		if err = dAdminClient.RestoreTableFrom(ctx, sourceInstance, restoreTableName, sourceCluster, stdBkpName); err != nil {
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
	}

	// Delete backup
	if err = deleteBackup(ctx, adminClient, sourceCluster, stdBkpName); err != nil {
		t.Fatalf("DeleteBackup: %v", err)
	}
	gotBackups, err = list(sourceCluster)
	if err != nil {
		t.Fatalf("List after Delete: %v", err)
	}

	// Verify the backup was deleted.
	for _, backup := range gotBackups {
		if backup.Name == stdBkpName {
			t.Errorf("Backup '%v' was not deleted", backup.Name)
			break
		}
	}

	// Test backup hot_to_standard_time field

	hotBkpUID := uid.NewSpace("mybackup-", &uid.Options{})
	hotBkpNameUpdate := hotBkpUID.New()
	t.Cleanup(func() {
		deleteBackupAndLog(context.Background(), t, adminClient, sourceCluster, hotBkpNameUpdate)
	})
	if err = createBackupWithOptions(ctx, adminClient, tblConf.TableID, sourceCluster, hotBkpNameUpdate, WithExpiry(time.Now().Add(8*time.Hour)), WithHotToStandardBackup(time.Now().Truncate(time.Second).Add(2*24*time.Hour))); err != nil {
		t.Fatalf("Creating backup: %v", err)
	}

	fiveDaysLater := time.Now().Truncate(time.Second).Add(5 * 24 * time.Hour)
	for _, test := range []struct {
		wantHtsTime *time.Time
		desc        string
	}{
		{
			desc:        "Unset hot_to_standard_time",
			wantHtsTime: nil,
		},
		{
			desc:        "Set hot_to_standard_time to 5 days from now",
			wantHtsTime: &fiveDaysLater,
		},
	} {
		t.Run(test.desc, func(t *testing.T) {
			// Update hot_to_standard_time
			if test.wantHtsTime == nil {
				err = updateBackupRemoveHotToStandardTime(ctx, adminClient, sourceCluster, hotBkpNameUpdate)
				if err != nil {
					t.Fatalf("UpdateBackupRemoveHotToStandardTime failed: %v", err)
				}
			} else {
				err = updateBackupHotToStandardTime(ctx, adminClient, sourceCluster, hotBkpNameUpdate, *test.wantHtsTime)
				if err != nil {
					t.Fatalf("UpdateBackupHotToStandardTime failed: %v", err)
				}
			}
			// Check that updated backup has the correct hot_to_standard_time
			updatedBackup, err := adminClient.BackupInfo(ctx, sourceCluster, hotBkpNameUpdate)
			if err != nil {
				t.Fatalf("BackupInfo: %v", err)
			}
			gotHtsTime := updatedBackup.HotToStandardTime
			if (test.wantHtsTime == nil && gotHtsTime != nil) ||
				(test.wantHtsTime != nil && gotHtsTime == nil) ||
				(test.wantHtsTime != nil && !test.wantHtsTime.Equal(*gotHtsTime)) {
				t.Errorf("hot_to_standard_time got: %v, want: %v", gotHtsTime, test.wantHtsTime)
			}
		})
	}
}

func TestIntegration_AuthorizedView(t *testing.T) {
	t.Parallel()
	t.Skip("flaky https://github.com/googleapis/google-cloud-go/issues/13398")
	ctx := context.Background()
	testEnv, _, adminClient, _, _, cleanup, err := setupIntegration(ctx, t)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(cleanup)

	if !testEnv.Config().UseProd {
		t.Skip("emulator doesn't support authorizedViews")
	}

	timeout := 15 * time.Minute
	ctx, cancel := context.WithTimeout(ctx, timeout)
	t.Cleanup(cancel)

	tableUID := uid.NewSpace("it-authview-", &uid.Options{Short: true})
	tblConf := TableConf{
		TableID: tableUID.New(),
		Families: map[string]GCPolicy{
			"fam1": MaxVersionsPolicy(1),
			"fam2": MaxVersionsPolicy(2),
		},
	}
	if err := createTableFromConf(ctx, adminClient, &tblConf); err != nil {
		t.Fatalf("Creating table from TableConf: %v", err)
	}
	// Delete the table at the end of the test. Schedule ahead of time
	// in case the client fails
	t.Cleanup(func() { deleteTable(context.Background(), t, adminClient, tblConf.TableID) })

	// Create authorized view
	authorizedViewUUID := uid.NewSpace("authorizedView-", &uid.Options{})
	authorizedView := authorizedViewUUID.New()
	t.Cleanup(func() {
		// Unprotect the view first to allow deletion, ignoring errors if it's already unprotected or deleted.
		adminClient.UpdateAuthorizedView(context.Background(), UpdateAuthorizedViewConf{
			AuthorizedViewConf: AuthorizedViewConf{
				TableID:            tblConf.TableID,
				AuthorizedViewID:   authorizedView,
				DeletionProtection: Unprotected,
			},
		})
		adminClient.DeleteAuthorizedView(context.Background(), tblConf.TableID, authorizedView)
	})

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

	// Data assertions
	client, err := testEnv.NewClient()
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}
	t.Cleanup(func() { closeClientAndLog(t, client, "client") })
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
	presplitTable := fmt.Sprintf("presplit-table-%d", time.Now().UnixNano())
	if err := createPresplitTable(ctx, adminClient, presplitTable, []string{"r0", "r11", "r12", "r2"}); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := adminClient.DeleteTable(context.Background(), presplitTable); err != nil {
			t.Logf("Cleanup DeleteTable failed: %v", err)
		}
	})
	if err := createColumnFamily(ctx, t, adminClient, presplitTable, "fam1", nil); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { adminClient.DeleteAuthorizedView(context.Background(), presplitTable, authorizedView) })
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

func TestIntegration_AdminSchemaBundle(t *testing.T) {
	t.Parallel()
	if testing.Short() {
		t.Skip("skipping heavy admin operation test in short mode")
	}

	ctx := context.Background()
	testEnv, _, adminClient, _, _, cleanup, err := setupIntegration(ctx, t)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(cleanup)

	if !testEnv.Config().UseProd {
		t.Skip("emulator doesn't support schemaBundles")
	}

	timeout := 15 * time.Minute
	ctx, cancel := context.WithTimeout(ctx, timeout)
	t.Cleanup(cancel)

	tableUID := uid.NewSpace("it-schbun-", &uid.Options{Short: true})
	tblConf := TableConf{
		TableID: tableUID.New(),
		Families: map[string]GCPolicy{
			"fam1": MaxVersionsPolicy(1),
			"fam2": MaxVersionsPolicy(2),
		},
	}
	if err := createTableFromConf(ctx, adminClient, &tblConf); err != nil {
		t.Fatalf("Creating table from TableConf: %v", err)
	}
	// Delete the table at the end of the test. Schedule ahead of time
	// in case the client fails
	t.Cleanup(func() { deleteTable(context.Background(), t, adminClient, tblConf.TableID) })

	// Create schema bundle
	schemaBundleUUID := uid.NewSpace("schemaBundle-", &uid.Options{})
	schemaBundle := schemaBundleUUID.New()
	t.Cleanup(func() { adminClient.DeleteSchemaBundle(context.Background(), tblConf.TableID, schemaBundle) })

	content, err := os.ReadFile("testdata/proto_schema_bundle.pb")
	if err != nil {
		t.Fatalf("Error reading the file: %v", err)
	}

	schemaBundleConf := SchemaBundleConf{
		TableID:        tblConf.TableID,
		SchemaBundleID: schemaBundle,
		ProtoSchema: &ProtoSchemaInfo{
			ProtoDescriptors: content,
		},
	}

	if err = adminClient.CreateSchemaBundle(ctx, &schemaBundleConf); err != nil {
		t.Fatalf("Creating schema bundle: %v", err)
	}

	// List schema bundles
	schemaBundles, err := adminClient.SchemaBundles(ctx, tblConf.TableID)
	if err != nil {
		t.Fatalf("Listing schema bundles: %v", err)
	}
	if got, want := len(schemaBundles), 1; got != want {
		t.Fatalf("Listing schema bundles count: %d, want: != %d", got, want)
	}
	if got, want := schemaBundles[0], schemaBundle; got != want {
		t.Errorf("SchemaBundle Name: %s, want: %s", got, want)
	}

	// Get schema bundle
	sbInfo, err := adminClient.GetSchemaBundle(ctx, tblConf.TableID, schemaBundle)
	if err != nil {
		t.Fatalf("Getting schema bundle: %v", err)
	}
	if got, want := sbInfo.SchemaBundle, content; !reflect.DeepEqual(got, want) {
		t.Errorf("ProtoSchema: %v, want: %v", got, want)
	}

	content, err = os.ReadFile("testdata/updated_proto_schema_bundle.pb")
	if err != nil {
		t.Fatalf("Error reading the file: %v", err)
	}

	// Update schema bundle
	newSchemaBundleConf := SchemaBundleConf{
		TableID:        tblConf.TableID,
		SchemaBundleID: schemaBundle,
		Etag:           sbInfo.Etag,
		ProtoSchema: &ProtoSchemaInfo{
			ProtoDescriptors: content,
		},
	}
	err = adminClient.UpdateSchemaBundle(ctx, UpdateSchemaBundleConf{
		SchemaBundleConf: newSchemaBundleConf,
	})
	if err != nil {
		t.Fatalf("UpdateSchemaBundle failed: %v", err)
	}

	// Get schema bundle
	sbInfo, err = adminClient.GetSchemaBundle(ctx, tblConf.TableID, schemaBundle)
	if err != nil {
		t.Fatalf("Getting schema bundle: %v", err)
	}
	if got, want := sbInfo.SchemaBundle, content; !reflect.DeepEqual(got, want) {
		t.Errorf("ProtoSchema: %v, want: %v", got, want)
	}

	// Delete schema bundle
	if err = adminClient.DeleteSchemaBundle(ctx, tblConf.TableID, schemaBundle); err != nil {
		t.Fatalf("DeleteSchemaBundle: %v", err)
	}

	// Verify the schema bundle was deleted.
	schemaBundles, err = adminClient.SchemaBundles(ctx, tblConf.TableID)
	if err != nil {
		t.Fatalf("Listing schema bundles: %v", err)
	}
	if got, want := len(schemaBundles), 0; got != want {
		t.Fatalf("Listing schema bundles count: %d, want: != %d", got, want)
	}
}

func TestIntegration_DataMaterializedView(t *testing.T) {
	t.Parallel()
	t.Skip("flaky https://github.com/googleapis/google-cloud-go/issues/12686")
	ctx := context.Background()
	testEnv, _, adminClient, _, _, cleanup, err := setupIntegration(ctx, t)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(cleanup)

	if !testEnv.Config().UseProd {
		t.Skip("emulator doesn't support materializedViews")
	}

	timeout := 15 * time.Minute
	ctx, cancel := context.WithTimeout(ctx, timeout)
	t.Cleanup(cancel)

	instanceAdminClient, err := testEnv.NewInstanceAdminClient()
	if err != nil {
		t.Fatalf("NewInstanceAdminClient: %v", err)
	}
	t.Cleanup(func() { instanceAdminClient.Close() })

	tableUID := uid.NewSpace("it-matview-", &uid.Options{Short: true})
	tblConf := TableConf{
		TableID: tableUID.New(),
		Families: map[string]GCPolicy{
			"fam1": MaxVersionsPolicy(1),
			"fam2": MaxVersionsPolicy(2),
		},
	}
	if err := createTableFromConf(ctx, adminClient, &tblConf); err != nil {
		t.Fatalf("Creating table from TableConf: %v", err)
	}
	// Delete the table at the end of the test. Schedule ahead of time
	// in case the client fails
	t.Cleanup(func() { deleteTable(context.Background(), t, adminClient, tblConf.TableID) })

	client, err := testEnv.NewClient()
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}
	t.Cleanup(func() { closeClientAndLog(t, client, "client") })

	// Populate table
	tbl := client.OpenTable(tblConf.TableID)
	mut := NewMutation()
	mut.Set("fam1", "col1", 1000, []byte("1"))
	if err := tbl.Apply(ctx, "r1", mut); err != nil {
		t.Fatalf("Mutating row: %v", err)
	}
	// Create materialized view
	materializedViewUUID := uid.NewSpace("materializedView-", &uid.Options{})
	materializedView := materializedViewUUID.New()
	t.Cleanup(func() {
		if err := instanceAdminClient.DeleteMaterializedView(context.Background(), testEnv.Config().Instance, materializedView); err != nil {
			t.Logf("Cleanup DeleteMaterializedView failed: %v", err)
		}
	})

	materializedViewInfo := MaterializedViewInfo{
		MaterializedViewID: materializedView,
		Query:              fmt.Sprintf("SELECT _key, count(fam1['col1']) as `result.count` FROM `%s` GROUP BY _key", tblConf.TableID),
		DeletionProtection: Unprotected,
	}
	if err = instanceAdminClient.CreateMaterializedView(ctx, testEnv.Config().Instance, &materializedViewInfo); err != nil {
		t.Fatalf("Creating materialized view: %v", err)
	}

	mv := client.OpenMaterializedView(materializedView)

	// Test ReadRow
	gotRow, err := mv.ReadRow(ctx, "r1")
	if err != nil {
		t.Fatalf("Reading row from a materialized view: %v", err)
	}

	wantRow := Row{
		"result": []ReadItem{
			{Row: "r1", Column: "result:count", Timestamp: 0, Value: binary.BigEndian.AppendUint64([]byte{}, 1)},
		},
		"default": []ReadItem{
			{Row: "r1", Column: "default:", Timestamp: 0},
		},
	}
	if !testutil.Equal(gotRow, wantRow) {
		t.Errorf("Error reading row from materialized view.\n Got %#v\n Want %#v", gotRow, wantRow)
	}
	gotRow, err = mv.ReadRow(ctx, "r2")
	if err != nil {
		t.Fatalf("Reading row from an materialized view: %v", err)
	}
	if len(gotRow) != 0 {
		t.Errorf("Expect empty result when reading row from outside an materialized view")
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
	if err = mv.ReadRows(ctx, RowRange{}, f); err != nil {
		t.Fatalf("Reading rows from an materialized view: %v", err)
	}
	want := "r1--,r1-count-" + string(binary.BigEndian.AppendUint64([]byte{}, 1))
	sort.Strings(elt)
	if got := strings.Join(elt, ","); got != want {
		t.Errorf("Error bulk reading from materialized view.\n Got %q\n Want %q", got, want)
	}

	// Test SampleRowKeys
	if _, err := mv.SampleRowKeys(ctx); err != nil {
		t.Errorf("Sampling row keys from an materialized view: %v", err)
	}
}

func TestIntegration_AdminLogicalView(t *testing.T) {
	t.Parallel()
	if testing.Short() {
		t.Skip("skipping heavy admin operation test in short mode")
	}

	ctx := context.Background()
	testEnv, _, adminClient, _, _, cleanup, err := setupIntegration(ctx, t)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(cleanup)

	if !testEnv.Config().UseProd {
		t.Skip("emulator doesn't support logicalViews")
	}

	timeout := 15 * time.Minute
	ctx, cancel := context.WithTimeout(ctx, timeout)
	t.Cleanup(cancel)

	instanceAdminClient, err := testEnv.NewInstanceAdminClient()
	if err != nil {
		t.Fatalf("NewInstanceAdminClient: %v", err)
	}
	t.Cleanup(func() { instanceAdminClient.Close() })

	tableUID := uid.NewSpace("it-logview-", &uid.Options{Short: true})
	tblConf := TableConf{
		TableID: tableUID.New(),
		Families: map[string]GCPolicy{
			"fam1": MaxVersionsPolicy(1),
			"fam2": MaxVersionsPolicy(2),
		},
	}
	if err := createTableFromConf(ctx, adminClient, &tblConf); err != nil {
		t.Fatalf("Creating table from TableConf: %v", err)
	}
	// Delete the table at the end of the test. Schedule ahead of time
	// in case the client fails
	t.Cleanup(func() { deleteTable(context.Background(), t, adminClient, tblConf.TableID) })

	// Create logical view
	logicalViewUUID := uid.NewSpace("logicalView-", &uid.Options{})
	logicalView := logicalViewUUID.New()
	t.Cleanup(func() {
		// Unprotect the view first to allow deletion.
		if err := updateLogicalView(context.Background(), instanceAdminClient, testEnv.Config().Instance, LogicalViewInfo{
			LogicalViewID:      logicalView,
			DeletionProtection: Unprotected,
		}); err != nil {
			if status.Code(err) != codes.NotFound {
				t.Logf("Cleanup: updateLogicalView failed: %v", err)
			}
		}
		if err := deleteLogicalView(context.Background(), instanceAdminClient, testEnv.Config().Instance, logicalView); err != nil {
			if status.Code(err) != codes.NotFound {
				t.Logf("Cleanup: deleteLogicalView failed: %v", err)
			}
		}
	})

	logicalViewInfo := LogicalViewInfo{
		LogicalViewID:      logicalView,
		Query:              fmt.Sprintf("SELECT _key, fam1['col1'] as col FROM `%s`", tblConf.TableID),
		DeletionProtection: Protected,
	}
	if err = createLogicalView(ctx, instanceAdminClient, testEnv.Config().Instance, &logicalViewInfo); err != nil {
		t.Fatalf("Creating logical view: %v", err)
	}

	// List logical views
	logicalViews, err := instanceAdminClient.LogicalViews(ctx, testEnv.Config().Instance)
	if err != nil {
		t.Fatalf("Listing logical views: %v", err)
	}
	found := false
	for _, lv := range logicalViews {
		if lv.LogicalViewID == logicalView {
			found = true
			if got, want := lv.Query, logicalViewInfo.Query; got != want {
				t.Errorf("LogicalView Query: %q, want: %q", got, want)
			}
			if got, want := lv.DeletionProtection, logicalViewInfo.DeletionProtection; got != want {
				t.Errorf("LogicalView DeletionProtection: %v, want: %v", got, want)
			}
			break
		}
	}
	if !found {
		t.Fatalf("Logical view %s not found in listed views", logicalView)
	}

	// Get logical view
	lvInfo, err := instanceAdminClient.LogicalViewInfo(ctx, testEnv.Config().Instance, logicalView)
	if err != nil {
		t.Fatalf("Getting logical view: %v", err)
	}
	if got, want := lvInfo.Query, logicalViewInfo.Query; got != want {
		t.Errorf("LogicalView Query: %q, want: %q", got, want)
	}
	if got, want := lvInfo.DeletionProtection, logicalViewInfo.DeletionProtection; got != want {
		t.Errorf("LogicalView DeletionProtection: %v, want: %v", got, want)
	}

	// Update logical view
	newLogicalViewInfo := LogicalViewInfo{
		LogicalViewID:      logicalView,
		Query:              fmt.Sprintf("SELECT _key, fam2['col1'] as col FROM `%s`", tblConf.TableID),
		DeletionProtection: Unprotected,
	}
	err = updateLogicalView(ctx, instanceAdminClient, testEnv.Config().Instance, newLogicalViewInfo)
	if err != nil {
		t.Fatalf("UpdateLogicalView failed: %v", err)
	}

	// Check that updated logical view has the correct deletion protection
	lvInfo, err = instanceAdminClient.LogicalViewInfo(ctx, testEnv.Config().Instance, logicalView)
	if err != nil {
		t.Fatalf("Getting logical view: %v", err)
	}
	if got, want := lvInfo.Query, newLogicalViewInfo.Query; got != want {
		t.Errorf("LogicalView Query: %q, want: %q", got, want)
	}
	if got, want := lvInfo.DeletionProtection, newLogicalViewInfo.DeletionProtection; got != want {
		t.Errorf("LogicalView DeletionProtection: %v, want: %v", got, want)
	}

	// Delete logical view
	if err = deleteLogicalView(ctx, instanceAdminClient, testEnv.Config().Instance, logicalView); err != nil {
		t.Fatalf("DeleteLogicalView: %v", err)
	}

	// Verify the logical view was deleted.
	logicalViews, err = instanceAdminClient.LogicalViews(ctx, testEnv.Config().Instance)
	if err != nil {
		t.Fatalf("Listing logical views: %v", err)
	}
	found = false
	for _, lv := range logicalViews {
		if lv.LogicalViewID == logicalView {
			found = true
			break
		}
	}
	if found {
		t.Fatalf("Logical view %s still found after deletion", logicalView)
	}
}

func TestIntegration_AdminMaterializedView(t *testing.T) {
	t.Parallel()
	if testing.Short() {
		t.Skip("skipping heavy admin operation test in short mode")
	}

	t.Skip("broken https://github.com/googleapis/google-cloud-go/issues/12415")
	ctx := context.Background()
	testEnv, _, adminClient, _, _, cleanup, err := setupIntegration(ctx, t)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(cleanup)

	if !testEnv.Config().UseProd {
		t.Skip("emulator doesn't support materializedViews")
	}

	timeout := 15 * time.Minute
	ctx, cancel := context.WithTimeout(ctx, timeout)
	t.Cleanup(cancel)

	instanceAdminClient, err := testEnv.NewInstanceAdminClient()
	if err != nil {
		t.Fatalf("NewInstanceAdminClient: %v", err)
	}
	t.Cleanup(func() { instanceAdminClient.Close() })

	tableUID := uid.NewSpace("it-matview-", &uid.Options{Short: true})
	tblConf := TableConf{
		TableID: tableUID.New(),
		Families: map[string]GCPolicy{
			"fam1": MaxVersionsPolicy(1),
			"fam2": MaxVersionsPolicy(2),
		},
	}
	if err := createTableFromConf(ctx, adminClient, &tblConf); err != nil {
		t.Fatalf("Creating table from TableConf: %v", err)
	}
	// Delete the table at the end of the test. Schedule ahead of time
	// in case the client fails
	t.Cleanup(func() { deleteTable(context.Background(), t, adminClient, tblConf.TableID) })

	// Create materialized view
	materializedViewUUID := uid.NewSpace("materializedView-", &uid.Options{})
	materializedView := materializedViewUUID.New()
	t.Cleanup(func() {
		instanceAdminClient.DeleteMaterializedView(context.Background(), testEnv.Config().Instance, materializedView)
	})

	materializedViewInfo := MaterializedViewInfo{
		MaterializedViewID: materializedView,
		Query:              fmt.Sprintf("SELECT _key, count(fam1['col1']) as count FROM `%s` GROUP BY _key", tblConf.TableID),
		DeletionProtection: Protected,
	}
	if err = instanceAdminClient.CreateMaterializedView(ctx, testEnv.Config().Instance, &materializedViewInfo); err != nil {
		t.Fatalf("Creating materialized view: %v", err)
	}

	// List materialized views
	materializedViews, err := instanceAdminClient.MaterializedViews(ctx, testEnv.Config().Instance)
	if err != nil {
		t.Fatalf("Listing materialized views: %v", err)
	}
	if got, want := len(materializedViews), 1; got < want {
		t.Fatalf("Listing materialized views count: %d, want: >= %d", got, want)
	}

	for _, mv := range materializedViews {
		if mv.MaterializedViewID == materializedView {
			if got, want := mv.Query, materializedViewInfo.Query; got != want {
				t.Errorf("MaterializedView Query: %q, want: %q", got, want)
			}
		}
	}

	// Get materialized view
	mvInfo, err := instanceAdminClient.MaterializedViewInfo(ctx, testEnv.Config().Instance, materializedView)
	if err != nil {
		t.Fatalf("Getting materialized view: %v", err)
	}
	if got, want := mvInfo.Query, materializedViewInfo.Query; got != want {
		t.Errorf("MaterializedView Query: %q, want: %q", got, want)
	}
	// Cannot delete the materialized view because it is deletion protected
	if err = instanceAdminClient.DeleteMaterializedView(ctx, testEnv.Config().Instance, materializedView); err == nil {
		t.Fatalf("DeleteMaterializedView: %v", err)
	}

	// Update materialized view
	newMaterializedViewInfo := MaterializedViewInfo{
		MaterializedViewID: materializedView,
		DeletionProtection: Unprotected,
	}
	err = instanceAdminClient.UpdateMaterializedView(ctx, testEnv.Config().Instance, newMaterializedViewInfo)
	if err != nil {
		t.Fatalf("UpdateMaterializedView failed: %v", err)
	}

	// Check that updated materialized view has the correct deletion protection
	mvInfo, err = instanceAdminClient.MaterializedViewInfo(ctx, testEnv.Config().Instance, materializedView)
	if err != nil {
		t.Fatalf("Getting materialized view: %v", err)
	}
	if got, want := mvInfo.DeletionProtection, Unprotected; got != want {
		t.Errorf("MaterializedViewInfo deletion protection: %v, want: %v", got, want)
	}
	// Check that the subset_view field doesn't change
	if got, want := mvInfo.Query, materializedViewInfo.Query; !cmp.Equal(got, want) {
		t.Errorf("Query: %q, want: %q", got, want)
	}

	// Delete materialized view
	if err = instanceAdminClient.DeleteMaterializedView(ctx, testEnv.Config().Instance, materializedView); err != nil {
		t.Fatalf("DeleteMaterializedView: %v", err)
	}

	// Verify the materialized view was deleted.
	materializedViews, err = instanceAdminClient.MaterializedViews(ctx, testEnv.Config().Instance)
	if err != nil {
		t.Fatalf("Listing materialized views: %v", err)
	}
	for _, mv := range materializedViews {
		if mv.MaterializedViewID == materializedView {
			t.Errorf("Found view %q that was meant to be deleted", materializedView)
		}
	}
}

// TestIntegration_DirectPathFallback tests the CFE fallback when the directpath net is blackholed.
func TestIntegration_DirectPathFallback(t *testing.T) {
	ctx := context.Background()
	testEnv, _, _, table, _, cleanup, err := setupIntegration(ctx, t)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(cleanup)

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

	if err := populatePresidentsGraph(table, ""); err != nil {
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

func TestIntegration_Execute(t *testing.T) {
	t.Parallel()
	// Set up table and clients
	ctx := context.Background()
	testEnv, client, adminClient, _, _, cleanup, err := setupIntegration(ctx, t)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(cleanup)

	if !testEnv.Config().UseProd {
		t.Skip("emulator doesn't support PrepareQuery")
	}

	tableName := fmt.Sprintf("exec-table-%d", time.Now().UnixNano())
	if err := createTable(ctx, adminClient, tableName); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := adminClient.DeleteTable(context.Background(), tableName); err != nil {
			t.Logf("Cleanup DeleteTable failed: %v", err)
		}
	})
	table := client.Open(tableName)

	colFam := "address"
	err = createColumnFamily(ctx, t, adminClient, tableName, colFam, map[codes.Code]bool{codes.Unavailable: true})
	if err != nil {
		t.Fatal("createColumnFamily: " + err.Error())
	}

	// Add data to table
	v1Timestamp := Time(time.Now().Add(-time.Minute))
	v2Timestamp := Time(time.Now())
	populateAddresses(ctx, t, table, colFam, v1Timestamp, v2Timestamp)

	base64City := base64.StdEncoding.EncodeToString([]byte("city"))
	base64State := base64.StdEncoding.EncodeToString([]byte("state"))

	// While writing, "timestamp": s"2025-03-31 03:26:33.489256 +0000 UTC"
	// When reading back, "timestamp": s"2025-03-31 03:26:33.489 +0000 UTC"
	ignoreMillisecondDiff := cmpopts.EquateApproxTime(time.Millisecond)
	cmpOpts := []cmp.Option{ignoreMillisecondDiff, cmp.AllowUnexported(Struct{})}

	tsParamValue := time.Now()
	type col struct {
		name     string
		gotDest  any
		wantDest any
	}
	// Run test cases
	for _, tc := range []struct {
		desc          string
		psQuery       string
		psParamTypes  map[string]SQLType
		bsParamValues map[string]any
		rows          [][]col
	}{
		{
			desc:    "select *",
			psQuery: "SELECT * FROM `" + table.table + "` ORDER BY _key LIMIT 5",
			rows: [][]col{
				{
					{
						name:     "_key",
						gotDest:  []byte{},
						wantDest: []byte("row-01"),
					},
					{
						name:    "address",
						gotDest: map[string][]byte{},
						wantDest: map[string][]byte{
							base64City:  []byte("San Francisco"),
							base64State: []byte("CA"),
						},
					},
				},
				{
					{
						name:     "_key",
						gotDest:  []byte{},
						wantDest: []byte("row-02"),
					},
					{
						name:    "address",
						gotDest: map[string][]byte{},
						wantDest: map[string][]byte{
							base64City:  []byte("Phoenix"),
							base64State: []byte("AZ"),
						},
					},
				},
			},
		},
		{
			desc:    "WITH_HISTORY key and column",
			psQuery: "SELECT _key, " + colFam + "['state'] AS state FROM `" + table.table + "`(WITH_HISTORY=>TRUE) LIMIT 5",
			rows: [][]col{
				{
					{
						name:     "_key",
						gotDest:  []byte{},
						wantDest: []byte("row-01"),
					},
					{
						name:    "state",
						gotDest: []Struct{},
						wantDest: []Struct{
							{
								fields: []structFieldWithValue{
									{
										Name:  "timestamp",
										Value: v2Timestamp.Time(),
									},
									{
										Name:  "value",
										Value: []byte("CA"),
									},
								},
								nameToIndex: map[string][]int{"timestamp": {0}, "value": {1}},
							},
							{
								fields: []structFieldWithValue{
									{
										Name:  "timestamp",
										Value: v1Timestamp.Time(),
									},
									{
										Name:  "value",
										Value: []byte("WA"),
									},
								},
								nameToIndex: map[string][]int{"timestamp": {0}, "value": {1}},
							},
						},
					},
				},
				{
					{
						name:     "_key",
						gotDest:  []byte{},
						wantDest: []byte("row-02"),
					},
					{
						name:    "state",
						gotDest: []Struct{},
						wantDest: []Struct{
							{
								fields: []structFieldWithValue{
									{
										Name:  "timestamp",
										Value: v1Timestamp.Time(),
									},
									{
										Name:  "value",
										Value: []byte("AZ"),
									},
								},
								nameToIndex: map[string][]int{"timestamp": {0}, "value": {1}},
							},
						},
					},
				},
			},
		},
		{
			desc:    "WITH_HISTORY column family",
			psQuery: "SELECT " + colFam + " FROM `" + table.table + "`(WITH_HISTORY=>TRUE) WHERE _key='row-01'",
			rows: [][]col{
				{
					{
						name:    "address",
						gotDest: map[string][]Struct{},
						wantDest: map[string][]Struct{
							base64State: {
								{
									fields: []structFieldWithValue{
										{
											Name:  "timestamp",
											Value: v2Timestamp.Time(),
										},
										{
											Name:  "value",
											Value: []byte("CA"),
										},
									},
									nameToIndex: map[string][]int{"timestamp": {0}, "value": {1}},
								},
								{
									fields: []structFieldWithValue{
										{
											Name:  "timestamp",
											Value: v1Timestamp.Time(),
										},
										{
											Name:  "value",
											Value: []byte("WA"),
										},
									},
									nameToIndex: map[string][]int{"timestamp": {0}, "value": {1}},
								},
							},
							base64City: {
								{
									fields: []structFieldWithValue{
										{
											Name:  "timestamp",
											Value: v2Timestamp.Time(),
										},
										{
											Name:  "value",
											Value: []byte("San Francisco"),
										},
									},
									nameToIndex: map[string][]int{"timestamp": {0}, "value": {1}},
								},
							},
						},
					},
				},
			},
		},
		{
			desc: "all types in result set",
			psQuery: "SELECT 'stringVal' AS strCol, b'foo' as bytesCol, 1 AS intCol, CAST(1.2 AS FLOAT32) as f32Col, " +
				"CAST(1.3 AS FLOAT64) as f64Col, true as boolCol, TIMESTAMP_FROM_UNIX_MILLIS(1000) AS tsCol, " +
				"DATE(2024, 06, 01) as dateCol, STRUCT(1 as a, \"foo\" as b) AS structCol, [1,2,3] AS arrCol, " +
				colFam +
				" as mapCol FROM `" +
				table.table +
				"` WHERE _key='row-01' LIMIT 1",
			rows: [][]col{
				{
					{
						name:     "strCol",
						gotDest:  "",
						wantDest: "stringVal",
					},
					{
						name:     "bytesCol",
						gotDest:  []byte{},
						wantDest: []byte("foo"),
					},
					{
						name:     "intCol",
						gotDest:  int64(0),
						wantDest: int64(1),
					},
					{
						name:     "f32Col",
						gotDest:  float32(0),
						wantDest: float32(1.2),
					},
					{
						name:     "f64Col",
						gotDest:  float64(0),
						wantDest: float64(1.3),
					},
					{
						name:     "boolCol",
						gotDest:  false,
						wantDest: true,
					},
					{
						name:     "tsCol",
						gotDest:  time.Time{},
						wantDest: time.Unix(1, 0),
					},
					{
						name:     "dateCol",
						gotDest:  civil.Date{},
						wantDest: civil.Date{Year: 2024, Month: 06, Day: 01},
					},
					{
						name:    "structCol",
						gotDest: Struct{},
						wantDest: Struct{
							fields:      []structFieldWithValue{{Name: "a", Value: int64(1)}, {Name: "b", Value: string("foo")}},
							nameToIndex: map[string][]int{"a": {0}, "b": {1}},
						},
					},
					{
						name:     "arrCol",
						gotDest:  []int64{},
						wantDest: []int64{1, 2, 3},
					},
					{
						name:    "mapCol",
						gotDest: map[string][]byte{},
						wantDest: map[string][]byte{
							base64City:  []byte("San Francisco"),
							base64State: []byte("CA"),
						},
					},
				},
			},
		},
		{
			desc: "all types in query parameters",
			psQuery: "SELECT @bytesParam as bytesCol, @stringParam AS strCol,  @int64Param AS int64Col, " +
				"@float32Param AS float32Col, @float64Param AS float64Col, @boolParam AS boolCol, " +
				"@tsParam AS tsCol, @dateParam AS dateCol, @bytesArrayParam AS bytesArrayCol, " +
				"@stringArrayParam AS stringArrayCol, @int64ArrayParam AS int64ArrayCol, " +
				"@float32ArrayParam AS float32ArrayCol, @float64ArrayParam AS float64ArrayCol, " +
				"@boolArrayParam AS boolArrayCol, @tsArrayParam AS tsArrayCol, " +
				"@dateArrayParam AS dateArrayCol",
			psParamTypes: map[string]SQLType{
				"bytesParam":   BytesSQLType{},
				"stringParam":  StringSQLType{},
				"int64Param":   Int64SQLType{},
				"float32Param": Float32SQLType{},
				"float64Param": Float64SQLType{},
				"boolParam":    BoolSQLType{},
				"tsParam":      TimestampSQLType{},
				"dateParam":    DateSQLType{},
				"bytesArrayParam": ArraySQLType{
					ElemType: BytesSQLType{},
				},
				"stringArrayParam": ArraySQLType{
					ElemType: StringSQLType{},
				},
				"int64ArrayParam": ArraySQLType{
					ElemType: Int64SQLType{},
				},
				"float32ArrayParam": ArraySQLType{
					ElemType: Float32SQLType{},
				},
				"float64ArrayParam": ArraySQLType{
					ElemType: Float64SQLType{},
				},
				"boolArrayParam": ArraySQLType{
					ElemType: BoolSQLType{},
				},
				"tsArrayParam": ArraySQLType{
					ElemType: TimestampSQLType{},
				},
				"dateArrayParam": ArraySQLType{
					ElemType: DateSQLType{},
				},
			},
			bsParamValues: map[string]any{
				"bytesParam":        []byte("foo"),
				"stringParam":       "stringVal",
				"int64Param":        int64(1),
				"float32Param":      float32(1.3),
				"float64Param":      float64(1.4),
				"boolParam":         true,
				"tsParam":           tsParamValue,
				"dateParam":         civil.DateOf(tsParamValue),
				"bytesArrayParam":   [][]byte{[]byte("foo"), nil, []byte("bar")},
				"stringArrayParam":  []string{"baz", "qux"},
				"int64ArrayParam":   []any{int64(1), nil, int64(2)},
				"float32ArrayParam": []any{float32(1.3), nil, float32(2.3)},
				"float64ArrayParam": []float64{1.4, 2.4, 3.4},
				"boolArrayParam":    []any{true, nil, false},
				"tsArrayParam":      []any{tsParamValue, nil},
				"dateArrayParam":    []civil.Date{civil.DateOf(tsParamValue), civil.DateOf(tsParamValue.Add(24 * time.Hour))},
			},
			rows: [][]col{
				{
					{
						name:     "bytesCol",
						gotDest:  []byte{},
						wantDest: []byte("foo"),
					},
					{
						name:     "strCol",
						gotDest:  "",
						wantDest: "stringVal",
					},
					{
						name:     "int64Col",
						gotDest:  int64(0),
						wantDest: int64(1),
					},
					{
						name:     "float32Col",
						gotDest:  float32(0),
						wantDest: float32(1.3),
					},
					{
						name:     "float64Col",
						gotDest:  float64(0),
						wantDest: float64(1.4),
					},
					{
						name:     "boolCol",
						gotDest:  false,
						wantDest: true,
					},
					{
						name:     "tsCol",
						gotDest:  time.Time{},
						wantDest: tsParamValue,
					},
					{
						name:     "dateCol",
						gotDest:  civil.Date{},
						wantDest: civil.DateOf(tsParamValue),
					},
					{
						name:     "bytesArrayCol",
						gotDest:  [][]byte{},
						wantDest: [][]byte{[]byte("foo"), nil, []byte("bar")},
					},
					{
						name:     "stringArrayCol",
						gotDest:  []string{},
						wantDest: []string{"baz", "qux"},
					},
					{
						name:     "int64ArrayCol",
						gotDest:  []any{},
						wantDest: []any{int64(1), nil, int64(2)},
					},
					{
						name:     "float32ArrayCol",
						gotDest:  []any{},
						wantDest: []any{float32(1.3), nil, float32(2.3)},
					},
					{
						name:     "float64ArrayCol",
						gotDest:  []float64{},
						wantDest: []float64{1.4, 2.4, 3.4},
					},
					{
						name:     "boolArrayCol",
						gotDest:  []any{},
						wantDest: []any{true, nil, false},
					},
					{
						name:     "tsArrayCol",
						gotDest:  []any{},
						wantDest: []any{tsParamValue, nil},
					},
					{
						name:     "dateArrayCol",
						gotDest:  []civil.Date{civil.DateOf(tsParamValue), civil.DateOf(tsParamValue.Add(24 * time.Hour))},
						wantDest: []civil.Date{civil.DateOf(tsParamValue), civil.DateOf(tsParamValue.Add(24 * time.Hour))},
					},
				},
			},
		},
	} {
		t.Run(tc.desc, func(t *testing.T) {
			ps, err := client.PrepareStatement(ctx, tc.psQuery, tc.psParamTypes)
			if err != nil {
				t.Fatal("PrepareStatement: " + err.Error())
			}

			bs, err := ps.Bind(tc.bsParamValues)
			if err != nil {
				t.Fatal("Bind: " + err.Error())
			}

			gotRowCount := 0
			if err = bs.Execute(ctx, func(rr ResultRow) bool {
				foundErr := false

				// Assert that rr has correct values
				if (tc.rows) != nil {
					// more rows than expected
					if gotRowCount >= len(tc.rows) {
						t.Fatalf("#%v: Unexpected row returned from Execute. gotRow: %#v", gotRowCount, rr)
					}

					var wantColCount int
					for wantColCount < len(tc.rows[gotRowCount]) {
						gotCurrCol := tc.rows[gotRowCount][wantColCount]
						destType := reflect.TypeOf(gotCurrCol.gotDest)

						// Assert GetByName returns correct value
						gotDestPtrReflectValue := reflect.New(destType)
						gotDestPtrInterface := gotDestPtrReflectValue.Interface()
						errGet := rr.GetByName(gotCurrCol.name, gotDestPtrInterface)
						if errGet != nil {
							t.Errorf("Row #%d: GetByName(name='%s', dest type='%T') failed: %v", gotRowCount, gotCurrCol.name, gotDestPtrInterface, errGet)
							foundErr = true
						}
						gotDest := reflect.ValueOf(gotDestPtrInterface).Elem().Interface()

						if diff := testutil.Diff(gotCurrCol.wantDest, gotDest, cmpOpts...); diff != "" {
							t.Errorf("[Row:%v Column:%v] GetByName: got: %#v, want: %#v, diff (-want +got):\n %+v",
								gotRowCount, wantColCount, gotDest, gotCurrCol.wantDest, diff)
							foundErr = true
						}

						// Assert GetByIndex returns correct value
						gotDestPtrReflectValue = reflect.New(destType)
						gotDestPtrInterface = gotDestPtrReflectValue.Interface()
						errGet = rr.GetByIndex(wantColCount, gotDestPtrInterface)
						if errGet != nil {
							t.Errorf("Row #%d: GetByIndex(index='%d', destType='%T') failed: %v", gotRowCount, wantColCount, gotDest, errGet)
							foundErr = true
						}
						gotDest = reflect.ValueOf(gotDestPtrInterface).Elem().Interface()
						if diff := testutil.Diff(gotCurrCol.wantDest, gotDest, cmpOpts...); diff != "" {
							t.Errorf("[Row:%v Column:%v] GetByIndex: got: %#v, want: %#v, diff (-want +got):\n %+v",
								gotRowCount, wantColCount, gotDest, gotCurrCol.wantDest, diff)
							foundErr = true
						}
						wantColCount++
					}
					if len(rr.pbValues) != len(tc.rows[gotRowCount]) {
						t.Errorf("[Row:%v] Number of columns: got: %v, want: %v", gotRowCount, len(rr.pbValues), len(tc.rows[gotRowCount]))

						// more columns than expected
						if len(rr.pbValues) > len(tc.rows[gotRowCount]) {
							i := len(tc.rows[gotRowCount])
							for i < len(rr.pbValues) {
								t.Errorf("[Row:%v Column:%v]: Unexpected column with value: %v", gotRowCount, i, rr.pbValues[i])
								i++
							}
							foundErr = true
						}

						// lesser columns than expected
						if len(rr.pbValues) < len(tc.rows[gotRowCount]) {
							i := len(rr.pbValues)
							for i < len(tc.rows[gotRowCount]) {
								t.Errorf("[Row:%v Column:%v]: Missing column with value: %v", gotRowCount, i, tc.rows[gotRowCount][i])
								i++
							}
							foundErr = true
						}
					}

					if foundErr {
						return false // Stop processing on error
					}
				}
				gotRowCount++
				return true
			}); err != nil {
				t.Fatal("Execute: " + err.Error())
			}

			// lesser rows than expected
			if gotRowCount < len(tc.rows) {
				t.Errorf("Number of rows: got: %v, want: %v", gotRowCount, len(tc.rows))
				i := gotRowCount
				for i < len(tc.rows) {
					t.Errorf("#%v: Row missing in Execute response: %#v", i, tc.rows[gotRowCount])
					i++
				}
			}
		})
	}
}

func populateAddresses(ctx context.Context, t *testing.T, table *Table, colFam string, v1Timestamp, v2Timestamp Timestamp) {
	type cell struct {
		Ts    Timestamp
		Value []byte
	}
	muts := []*Mutation{}
	rowKeys := []string{}
	for rowKey, mutData := range map[string]map[string]any{
		"row-01": {
			"state": []cell{
				{
					Ts:    v1Timestamp,
					Value: []byte("WA"),
				},
				{
					Ts:    v2Timestamp,
					Value: []byte("CA"),
				},
			},
			"city": []cell{
				{
					Ts:    v2Timestamp,
					Value: []byte("San Francisco"),
				},
			},
		},
		"row-02": {
			"state": []cell{
				{
					Ts:    v1Timestamp,
					Value: []byte("AZ"),
				},
			},
			"city": []cell{
				{
					Ts:    v1Timestamp,
					Value: []byte("Phoenix"),
				},
			},
		},
	} {
		mut := NewMutation()
		for col, v := range mutData {
			cells, ok := v.([]cell)
			if ok {
				for _, cell := range cells {
					mut.Set(colFam, col, cell.Ts, cell.Value)
				}
			}
		}
		muts = append(muts, mut)
		rowKeys = append(rowKeys, rowKey)
	}

	rowErrs, err := table.ApplyBulk(ctx, rowKeys, muts)
	if err != nil || rowErrs != nil {
		t.Fatal("ApplyBulk: ", err)
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

var (
	sharedIntegrationEnv IntegrationEnv
	sharedClient         *Client
	sharedAdminClient    *AdminClient
	sharedTableName      string
	sharedSetupOnce      sync.Once
	sharedSetupErr       error
)

func setupIntegration(ctx context.Context, t *testing.T) (_ IntegrationEnv, _ *Client, _ *AdminClient, table *Table, tableName string, cleanup func(), _ error) {
	sharedSetupOnce.Do(func() {
		sharedIntegrationEnv, sharedSetupErr = NewIntegrationEnv()
		if sharedSetupErr != nil {
			return
		}

		sharedAdminClient, sharedSetupErr = sharedIntegrationEnv.NewAdminClient()
		if sharedSetupErr != nil {
			return
		}

		if sharedIntegrationEnv.Config().UseProd {
			sharedTableName = tableNameSpace.New()
		} else {
			sharedTableName = sharedIntegrationEnv.Config().Table
		}

		sharedSetupErr = createTable(context.Background(), sharedAdminClient, sharedTableName)
		if sharedSetupErr != nil {
			return
		}

		// Retry client creation to bypass "FailedPrecondition: Table currently being created"
		// while the backend finishes registering the new table.
		sharedClient, sharedSetupErr = newClientWithRetry(context.Background(), sharedIntegrationEnv)

		if sharedSetupErr != nil {
			return
		}

		// Wait for table to be available for reads/writes
		table := sharedClient.Open(sharedTableName)
		sharedSetupErr = waitForTableReadiness(context.Background(), table)
		if sharedSetupErr != nil {
			return
		}

		// Pre-create all column families used by tests
		families := []string{"follows", "ts", "export", "bulk", "counter", "new_family", "cf", "fam1", "fam2"}
		for _, f := range families {
			if err := createColumnFamily(context.Background(), t, sharedAdminClient, sharedTableName, f, map[codes.Code]bool{codes.Unavailable: true, codes.AlreadyExists: true}); err != nil {
				sharedSetupErr = err
				return
			}
		}
		sharedSetupErr = createColumnFamilyWithConfig(context.Background(), t, sharedAdminClient, sharedTableName, "sum", &Family{ValueType: AggregateType{
			Input:      Int64Type{},
			Aggregator: SumAggregator{},
		}}, map[codes.Code]bool{codes.Unavailable: true, codes.AlreadyExists: true})
	})

	if sharedSetupErr != nil {
		t.Logf("Error in shared setup: %v", sharedSetupErr)
		return nil, nil, nil, nil, "", func() {}, sharedSetupErr
	}

	return sharedIntegrationEnv, sharedClient, sharedAdminClient, sharedClient.Open(sharedTableName), sharedTableName, func() {}, nil
}

// newClientWithRetry creates a new client and retries if it fails with
// FailedPrecondition due to table still being created.
func newClientWithRetry(ctx context.Context, env IntegrationEnv) (*Client, error) {
	var client *Client
	err := internal.Retry(ctx, gax.Backoff{
		Initial:    2 * time.Second,
		Max:        30 * time.Second,
		Multiplier: 2.0,
	}, func() (bool, error) {
		var err error
		client, err = env.NewClient()
		if err != nil {
			s, ok := status.FromError(err)
			if ok && s.Code() == codes.FailedPrecondition {
				return false, err // retry
			}
			return true, err // stop and return other errors
		}
		return true, nil // success
	})
	return client, err
}

func waitForTableReadiness(ctx context.Context, table *Table) error {
	return internal.Retry(ctx, gax.Backoff{
		Initial:    1 * time.Second,
		Max:        10 * time.Second,
		Multiplier: 2.0,
	}, func() (bool, error) {
		_, err := table.ReadRow(ctx, "non-existent-key")
		if err != nil {
			s, ok := status.FromError(err)
			if ok && s.Code() == codes.NotFound {
				return false, err // retry
			}
			return true, err // stop on other errors
		}
		return true, nil // success
	})
}

func newClientWithConfigAndRetry(ctx context.Context, env IntegrationEnv, config ClientConfig) (*Client, error) {
	var client *Client
	err := internal.Retry(ctx, gax.Backoff{
		Initial:    2 * time.Second,
		Max:        30 * time.Second,
		Multiplier: 2.0,
	}, func() (bool, error) {
		var err error
		client, err = env.NewClientWithConfig(config)
		if err != nil {
			s, ok := status.FromError(err)
			if ok && s.Code() == codes.FailedPrecondition {
				return false, err // retry
			}
			return true, err // stop and return other errors
		}
		return true, nil // success
	})
	return client, err
}

func createPresplitTable(ctx context.Context, adminClient *AdminClient, tableName string, splitKeys []string) error {
	return retry(ctx, func() error { return adminClient.CreatePresplitTable(ctx, tableName, splitKeys) },
		func() error { return adminClient.DeleteTable(ctx, tableName) })
}

func createTableFromConf(ctx context.Context, adminClient *AdminClient, conf *TableConf) error {
	return retry(ctx, func() error { return adminClient.CreateTableFromConf(ctx, conf) },
		func() error { return adminClient.DeleteTable(ctx, conf.TableID) })
}

func createTable(ctx context.Context, adminClient *AdminClient, tableName string) error {
	return retry(ctx, func() error { return adminClient.CreateTable(ctx, tableName) },
		func() error { return adminClient.DeleteTable(ctx, tableName) })
}

// retry 'f' and runs 'onExists' if 'f' returns AlreadyExists error
func retry(ctx context.Context, f func() error, onExists func() error) error {
	if f == nil {
		return nil
	}

	var lastErr error
	attemptsDone := 0

	internal.Retry(ctx, retryCreateBackoff, func() (bool, error) {
		currErr := f()
		lastErr = currErr

		if currErr == nil {
			return true, nil // Success, stop
		}

		s, ok := status.FromError(currErr)
		if !ok {
			return true, currErr // Non-status error, fail fast
		}

		code := s.Code()

		// 1. Handle AlreadyExists/FailedPrecondition
		if onExists != nil && (code == codes.AlreadyExists || code == codes.FailedPrecondition) {
			lastErr = onExists()
			if lastErr == nil {
				attemptsDone++
				return false, nil // Retry creation after delete
			}
			return true, lastErr // Cleanup failed, stop
		}

		// 2. Handle Transient Errors (including ResourceExhausted)
		if code == codes.Unavailable || code == codes.Aborted || code == codes.Internal || code == codes.Unknown || code == codes.ResourceExhausted {
			attemptsDone++
			return attemptsDone == maxCreateAttempts, currErr
		}

		// 3. Permanent Errors (Fail fast)
		return true, currErr
	})
	return lastErr
}

func createColumnFamily(ctx context.Context, t *testing.T, adminClient *AdminClient, table, family string, retryableCodes map[codes.Code]bool) error {
	return createColumnFamilyWithConfig(ctx, t, adminClient, table, family, nil, retryableCodes)
}

func createColumnFamilyWithConfig(ctx context.Context, t *testing.T, adminClient *AdminClient, table, family string, config *Family, retryableCodes map[codes.Code]bool) error {
	// Error seen on last create attempt
	var err error

	testutil.Retry(t, maxCreateAttempts, retryCreateSleep, func(r *testutil.R) {
		var createErr error
		if config != nil {
			createErr = adminClient.CreateColumnFamilyWithConfig(ctx, table, family, *config)
		} else {
			createErr = adminClient.CreateColumnFamily(ctx, table, family)
		}
		err = createErr

		if createErr != nil {
			s, ok := status.FromError(createErr)
			if ok && s.Code() == codes.AlreadyExists {
				err = nil
				return
			}
			r.Errorf("%+v", createErr.Error())
			if ok && retryableCodes != nil && !retryableCodes[s.Code()] {
				r.Fatalf("%+v", createErr.Error())
			}
		}
	})
	return err
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

func deleteTable(ctx context.Context, t *testing.T, ac *AdminClient, name string) error {
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
			if status.Code(err) == codes.NotFound || strings.Contains(err.Error(), "NotFound") {
				return true, nil
			}
			return false, err
		}
		return true, nil
	})
	if err != nil {
		t.Logf("DeleteTable: %v", err)
	}
	return err
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
	t.Logf("%+v", string(out))
	if testEnv.Config().DirectPathIPV4Only {
		return
	}
	cmdRes = exec.Command("bash", "-c", blackholeDpv6Cmd)
	out, _ = cmdRes.CombinedOutput()
	t.Logf("%+v", string(out))
}

func allowDirectPath(testEnv IntegrationEnv, t *testing.T) {
	cmdRes := exec.Command("bash", "-c", allowDpv4Cmd)
	out, _ := cmdRes.CombinedOutput()
	t.Logf("%+v", string(out))
	if testEnv.Config().DirectPathIPV4Only {
		return
	}
	cmdRes = exec.Command("bash", "-c", allowDpv6Cmd)
	out, _ = cmdRes.CombinedOutput()
	t.Logf("%+v", string(out))
}

func withUniqueID(s, prefix string, uniqueID int64) string {
	return strings.ReplaceAll(s, prefix, fmt.Sprintf("%s%d-", prefix, uniqueID))
}

func deleteBackupAndLog(ctx context.Context, t *testing.T, adminClient *AdminClient, cluster, backup string) {
	if err := deleteBackup(ctx, adminClient, cluster, backup); err != nil {
		t.Logf("Cleanup deleteBackup failed: %v", err)
	}
}

func closeClientAndLog(t *testing.T, closer io.Closer, name string) {
	if err := closer.Close(); err != nil {
		t.Logf("Cleanup %s failed: %v", name, err)
	}
}

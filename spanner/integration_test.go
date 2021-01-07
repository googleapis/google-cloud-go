/*
Copyright 2017 Google LLC

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

package spanner

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"log"
	"math"
	"math/big"
	"os"
	"os/exec"
	"reflect"
	"regexp"
	"strings"
	"sync"
	"testing"
	"time"

	"cloud.google.com/go/civil"
	"cloud.google.com/go/internal/testutil"
	"cloud.google.com/go/internal/uid"
	database "cloud.google.com/go/spanner/admin/database/apiv1"
	instance "cloud.google.com/go/spanner/admin/instance/apiv1"
	"google.golang.org/api/iterator"
	"google.golang.org/api/option"
	"google.golang.org/api/option/internaloption"
	adminpb "google.golang.org/genproto/googleapis/spanner/admin/database/v1"
	instancepb "google.golang.org/genproto/googleapis/spanner/admin/instance/v1"
	sppb "google.golang.org/genproto/googleapis/spanner/v1"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/peer"
	"google.golang.org/grpc/status"
)

const (
	directPathIPV6Prefix = "[2001:4860:8040"
	directPathIPV4Prefix = "34.126"
)

var (
	// testProjectID specifies the project used for testing. It can be changed
	// by setting environment variable GCLOUD_TESTS_GOLANG_PROJECT_ID.
	testProjectID = testutil.ProjID()

	// spannerHost specifies the spanner API host used for testing. It can be changed
	// by setting the environment variable GCLOUD_TESTS_GOLANG_SPANNER_HOST
	spannerHost = getSpannerHost()

	dbNameSpace       = uid.NewSpace("gotest", &uid.Options{Sep: '_', Short: true})
	instanceNameSpace = uid.NewSpace("gotest", &uid.Options{Sep: '-', Short: true})
	backupIDSpace     = uid.NewSpace("gotest", &uid.Options{Sep: '_', Short: true})
	testInstanceID    = instanceNameSpace.New()

	testTable        = "TestTable"
	testTableIndex   = "TestTableByValue"
	testTableColumns = []string{"Key", "StringValue"}

	databaseAdmin *database.DatabaseAdminClient
	instanceAdmin *instance.InstanceAdminClient

	dpConfig directPathTestConfig
	peerInfo *peer.Peer

	singerDBStatements = []string{
		`CREATE TABLE Singers (
				SingerId	INT64 NOT NULL,
				FirstName	STRING(1024),
				LastName	STRING(1024),
				SingerInfo	BYTES(MAX)
			) PRIMARY KEY (SingerId)`,
		`CREATE INDEX SingerByName ON Singers(FirstName, LastName)`,
		`CREATE TABLE Accounts (
				AccountId	INT64 NOT NULL,
				Nickname	STRING(100),
				Balance		INT64 NOT NULL,
			) PRIMARY KEY (AccountId)`,
		`CREATE INDEX AccountByNickname ON Accounts(Nickname) STORING (Balance)`,
		`CREATE TABLE Types (
				RowID		INT64 NOT NULL,
				String		STRING(MAX),
				StringArray	ARRAY<STRING(MAX)>,
				Bytes		BYTES(MAX),
				BytesArray	ARRAY<BYTES(MAX)>,
				Int64a		INT64,
				Int64Array	ARRAY<INT64>,
				Bool		BOOL,
				BoolArray	ARRAY<BOOL>,
				Float64		FLOAT64,
				Float64Array	ARRAY<FLOAT64>,
				Date		DATE,
				DateArray	ARRAY<DATE>,
				Timestamp	TIMESTAMP,
				TimestampArray	ARRAY<TIMESTAMP>,
			) PRIMARY KEY (RowID)`,
	}

	readDBStatements = []string{
		`CREATE TABLE TestTable (
                    Key          STRING(MAX) NOT NULL,
                    StringValue  STRING(MAX)
            ) PRIMARY KEY (Key)`,
		`CREATE INDEX TestTableByValue ON TestTable(StringValue)`,
		`CREATE INDEX TestTableByValueDesc ON TestTable(StringValue DESC)`,
	}

	simpleDBStatements = []string{
		`CREATE TABLE test (
				a	STRING(1024),
				b	STRING(1024),
			) PRIMARY KEY (a)`,
	}
	simpleDBTableColumns = []string{"a", "b"}

	ctsDBStatements = []string{
		`CREATE TABLE TestTable (
		    Key  STRING(MAX) NOT NULL,
		    Ts   TIMESTAMP OPTIONS (allow_commit_timestamp = true),
	    ) PRIMARY KEY (Key)`,
	}
	backuDBStatements = []string{
		`CREATE TABLE Singers (
						SingerId	INT64 NOT NULL,
						FirstName	STRING(1024),
						LastName	STRING(1024),
						SingerInfo	BYTES(MAX)
					) PRIMARY KEY (SingerId)`,
		`CREATE INDEX SingerByName ON Singers(FirstName, LastName)`,
		`CREATE TABLE Accounts (
						AccountId	INT64 NOT NULL,
						Nickname	STRING(100),
						Balance		INT64 NOT NULL,
					) PRIMARY KEY (AccountId)`,
		`CREATE INDEX AccountByNickname ON Accounts(Nickname) STORING (Balance)`,
		`CREATE TABLE Types (
						RowID		INT64 NOT NULL,
						String		STRING(MAX),
						StringArray	ARRAY<STRING(MAX)>,
						Bytes		BYTES(MAX),
						BytesArray	ARRAY<BYTES(MAX)>,
						Int64a		INT64,
						Int64Array	ARRAY<INT64>,
						Bool		BOOL,
						BoolArray	ARRAY<BOOL>,
						Float64		FLOAT64,
						Float64Array	ARRAY<FLOAT64>,
						Date		DATE,
						DateArray	ARRAY<DATE>,
						Timestamp	TIMESTAMP,
						TimestampArray	ARRAY<TIMESTAMP>,
					) PRIMARY KEY (RowID)`,
	}

	validInstancePattern = regexp.MustCompile("^projects/(?P<project>[^/]+)/instances/(?P<instance>[^/]+)$")

	blackholeDpv6Cmd string
	blackholeDpv4Cmd string
	allowDpv6Cmd     string
	allowDpv4Cmd     string
)

func init() {
	flag.BoolVar(&dpConfig.attemptDirectPath, "it.attempt-directpath", false, "DirectPath integration test flag")
	flag.BoolVar(&dpConfig.directPathIPv4Only, "it.directpath-ipv4-only", false, "Run DirectPath on a IPv4-only VM")
	peerInfo = &peer.Peer{}
	// Use sysctl or iptables to blackhole DirectPath IP for fallback tests.
	flag.StringVar(&blackholeDpv6Cmd, "it.blackhole-dpv6-cmd", "", "Command to make LB and backend addresses blackholed over dpv6")
	flag.StringVar(&blackholeDpv4Cmd, "it.blackhole-dpv4-cmd", "", "Command to make LB and backend addresses blackholed over dpv4")
	flag.StringVar(&allowDpv6Cmd, "it.allow-dpv6-cmd", "", "Command to make LB and backend addresses allowed over dpv6")
	flag.StringVar(&allowDpv4Cmd, "it.allow-dpv4-cmd", "", "Command to make LB and backend addresses allowed over dpv4")
}

type directPathTestConfig struct {
	attemptDirectPath  bool
	directPathIPv4Only bool
}

func parseInstanceName(inst string) (project, instance string, err error) {
	matches := validInstancePattern.FindStringSubmatch(inst)
	if len(matches) == 0 {
		return "", "", fmt.Errorf("Failed to parse instance name from %q according to pattern %q",
			inst, validInstancePattern.String())
	}
	return matches[1], matches[2], nil
}

func getSpannerHost() string {
	return os.Getenv("GCLOUD_TESTS_GOLANG_SPANNER_HOST")
}

const (
	str1 = "alice"
	str2 = "a@example.com"
)

func TestMain(m *testing.M) {
	cleanup := initIntegrationTests()
	res := m.Run()
	cleanup()
	os.Exit(res)
}

var grpcHeaderChecker = testutil.DefaultHeadersEnforcer()

func initIntegrationTests() (cleanup func()) {
	ctx := context.Background()
	flag.Parse() // Needed for testing.Short().
	noop := func() {}

	if testing.Short() {
		log.Println("Integration tests skipped in -short mode.")
		return noop
	}

	if testProjectID == "" {
		log.Println("Integration tests skipped: GCLOUD_TESTS_GOLANG_PROJECT_ID is missing")
		return noop
	}

	opts := grpcHeaderChecker.CallOptions()
	if spannerHost != "" {
		opts = append(opts, option.WithEndpoint(spannerHost))
	}
	if dpConfig.attemptDirectPath {
		// TODO(mohanli): Move EnableDirectPath internal option to client.go when DirectPath is ready for public beta.
		opts = append(opts, internaloption.EnableDirectPath(true))
		opts = append(opts, option.WithGRPCDialOption(grpc.WithDefaultCallOptions(grpc.Peer(peerInfo))))
	}
	var err error
	// Create InstanceAdmin and DatabaseAdmin clients.
	instanceAdmin, err = instance.NewInstanceAdminClient(ctx, opts...)
	if err != nil {
		log.Fatalf("cannot create instance databaseAdmin client: %v", err)
	}
	databaseAdmin, err = database.NewDatabaseAdminClient(ctx, opts...)
	if err != nil {
		log.Fatalf("cannot create databaseAdmin client: %v", err)
	}
	// Get the list of supported instance configs for the project that is used
	// for the integration tests. The supported instance configs can differ per
	// project. The integration tests will use the first instance config that
	// is returned by Cloud Spanner. This will normally be the regional config
	// that is physically the closest to where the request is coming from.
	configIterator := instanceAdmin.ListInstanceConfigs(ctx, &instancepb.ListInstanceConfigsRequest{
		Parent: fmt.Sprintf("projects/%s", testProjectID),
	})
	config, err := configIterator.Next()
	if err != nil {
		log.Fatalf("Cannot get any instance configurations.\nPlease make sure the Cloud Spanner API is enabled for the test project.\nGet error: %v", err)
	}

	// First clean up any old test instances before we start the actual testing
	// as these might cause this test run to fail.
	cleanupInstances()

	// Create a test instance to use for this test run.
	op, err := instanceAdmin.CreateInstance(ctx, &instancepb.CreateInstanceRequest{
		Parent:     fmt.Sprintf("projects/%s", testProjectID),
		InstanceId: testInstanceID,
		Instance: &instancepb.Instance{
			Config:      config.Name,
			DisplayName: testInstanceID,
			NodeCount:   1,
		},
	})
	if err != nil {
		log.Fatalf("could not create instance with id %s: %v", fmt.Sprintf("projects/%s/instances/%s", testProjectID, testInstanceID), err)
	}
	// Wait for the instance creation to finish.
	i, err := op.Wait(ctx)
	if err != nil {
		log.Fatalf("waiting for instance creation to finish failed: %v", err)
	}
	if i.State != instancepb.Instance_READY {
		log.Printf("instance state is not READY, it might be that the test instance will cause problems during tests. Got state %v\n", i.State)
	}

	return func() {
		// Delete this test instance.
		instanceName := fmt.Sprintf("projects/%v/instances/%v", testProjectID, testInstanceID)
		deleteInstanceAndBackups(ctx, instanceName)
		// Delete other test instances that may be lingering around.
		cleanupInstances()
		databaseAdmin.Close()
		instanceAdmin.Close()
	}
}

func TestIntegration_InitSessionPool(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()
	// Set up an empty testing environment.
	client, _, cleanup := prepareIntegrationTest(ctx, t, DefaultSessionPoolConfig, []string{})
	defer cleanup()
	sp := client.idleSessions
	sp.mu.Lock()
	want := sp.MinOpened
	sp.mu.Unlock()
	var numOpened int
loop:
	for {
		select {
		case <-ctx.Done():
			t.Fatalf("timed out, got %d session(s), want %d", numOpened, want)
		default:
			sp.mu.Lock()
			numOpened = sp.idleList.Len() + sp.idleWriteList.Len()
			sp.mu.Unlock()
			if uint64(numOpened) == want {
				break loop
			}
		}
	}
	// Delete all sessions in the pool on the backend and then try to execute a
	// simple query. The 'Session not found' error should cause an automatic
	// retry of the read-only transaction.
	sp.mu.Lock()
	s := sp.idleList.Front()
	for {
		if s == nil {
			break
		}
		// This will delete the session on the backend without removing it
		// from the pool.
		s.Value.(*session).delete(context.Background())
		s = s.Next()
	}
	sp.mu.Unlock()
	sql := "SELECT 1, 'FOO', 'BAR'"
	tx := client.ReadOnlyTransaction()
	defer tx.Close()
	iter := tx.Query(context.Background(), NewStatement(sql))
	rows, err := readAll(iter)
	if err != nil {
		t.Fatalf("Unexpected error for query %q: %v", sql, err)
	}
	if got, want := len(rows), 1; got != want {
		t.Fatalf("Row count mismatch for query %q\nGot: %v\nWant: %v", sql, got, want)
	}
	if got, want := len(rows[0]), 3; got != want {
		t.Fatalf("Column count mismatch for query %q\nGot: %v\nWant: %v", sql, got, want)
	}
	if got, want := rows[0][0].(int64), int64(1); got != want {
		t.Fatalf("Column value mismatch for query %q\nGot: %v\nWant: %v", sql, got, want)
	}
}

// Test SingleUse transaction.
func TestIntegration_SingleUse(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()
	// Set up testing environment.
	client, _, cleanup := prepareIntegrationTest(ctx, t, DefaultSessionPoolConfig, singerDBStatements)
	defer cleanup()

	writes := []struct {
		row []interface{}
		ts  time.Time
	}{
		{row: []interface{}{1, "Marc", "Foo"}},
		{row: []interface{}{2, "Tars", "Bar"}},
		{row: []interface{}{3, "Alpha", "Beta"}},
		{row: []interface{}{4, "Last", "End"}},
	}
	// Try to write four rows through the Apply API.
	for i, w := range writes {
		var err error
		m := InsertOrUpdate("Singers",
			[]string{"SingerId", "FirstName", "LastName"},
			w.row)
		if writes[i].ts, err = client.Apply(ctx, []*Mutation{m}, ApplyAtLeastOnce()); err != nil {
			t.Fatal(err)
		}
		verifyDirectPathRemoteAddress(t)
	}
	// Calculate time difference between Cloud Spanner server and localhost to
	// use to determine the exact staleness value to use.
	timeDiff := maxDuration(time.Now().Sub(writes[0].ts), 0)

	// Test reading rows with different timestamp bounds.
	for i, test := range []struct {
		name    string
		want    [][]interface{}
		tb      TimestampBound
		checkTs func(time.Time) error
	}{
		{
			name: "strong",
			want: [][]interface{}{{int64(1), "Marc", "Foo"}, {int64(3), "Alpha", "Beta"}, {int64(4), "Last", "End"}},
			tb:   StrongRead(),
			checkTs: func(ts time.Time) error {
				// writes[3] is the last write, all subsequent strong read
				// should have a timestamp larger than that.
				if ts.Before(writes[3].ts) {
					return fmt.Errorf("read got timestamp %v, want it to be no earlier than %v", ts, writes[3].ts)
				}
				return nil
			},
		},
		{
			name: "min_read_timestamp",
			want: [][]interface{}{{int64(1), "Marc", "Foo"}, {int64(3), "Alpha", "Beta"}, {int64(4), "Last", "End"}},
			tb:   MinReadTimestamp(writes[3].ts),
			checkTs: func(ts time.Time) error {
				if ts.Before(writes[3].ts) {
					return fmt.Errorf("read got timestamp %v, want it to be no earlier than %v", ts, writes[3].ts)
				}
				return nil
			},
		},
		{
			name: "max_staleness",
			want: [][]interface{}{{int64(1), "Marc", "Foo"}, {int64(3), "Alpha", "Beta"}, {int64(4), "Last", "End"}},
			tb:   MaxStaleness(time.Second),
			checkTs: func(ts time.Time) error {
				if ts.Before(writes[3].ts) {
					return fmt.Errorf("read got timestamp %v, want it to be no earlier than %v", ts, writes[3].ts)
				}
				return nil
			},
		},
		{
			name: "read_timestamp",
			want: [][]interface{}{{int64(1), "Marc", "Foo"}, {int64(3), "Alpha", "Beta"}},
			tb:   ReadTimestamp(writes[2].ts),
			checkTs: func(ts time.Time) error {
				if ts != writes[2].ts {
					return fmt.Errorf("read got timestamp %v, want %v", ts, writes[2].ts)
				}
				return nil
			},
		},
		{
			name: "exact_staleness",
			want: nil,
			// Specify a staleness which should be already before this test.
			tb: ExactStaleness(time.Now().Sub(writes[0].ts) + timeDiff + 30*time.Second),
			checkTs: func(ts time.Time) error {
				if !ts.Before(writes[0].ts) {
					return fmt.Errorf("read got timestamp %v, want it to be earlier than %v", ts, writes[0].ts)
				}
				return nil
			},
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			// SingleUse.Query
			su := client.Single().WithTimestampBound(test.tb)
			got, err := readAll(su.Query(
				ctx,
				Statement{
					"SELECT SingerId, FirstName, LastName FROM Singers WHERE SingerId IN (@id1, @id3, @id4) ORDER BY SingerId",
					map[string]interface{}{"id1": int64(1), "id3": int64(3), "id4": int64(4)},
				}))
			if err != nil {
				t.Fatalf("%d: SingleUse.Query returns error %v, want nil", i, err)
			}
			verifyDirectPathRemoteAddress(t)
			rts, err := su.Timestamp()
			if err != nil {
				t.Fatalf("%d: SingleUse.Query doesn't return a timestamp, error: %v", i, err)
			}
			if err := test.checkTs(rts); err != nil {
				t.Fatalf("%d: SingleUse.Query doesn't return expected timestamp: %v", i, err)
			}
			if !testEqual(got, test.want) {
				t.Fatalf("%d: got unexpected result from SingleUse.Query: %v, want %v", i, got, test.want)
			}
			// SingleUse.Read
			su = client.Single().WithTimestampBound(test.tb)
			got, err = readAll(su.Read(ctx, "Singers", KeySets(Key{1}, Key{3}, Key{4}), []string{"SingerId", "FirstName", "LastName"}))
			if err != nil {
				t.Fatalf("%d: SingleUse.Read returns error %v, want nil", i, err)
			}
			verifyDirectPathRemoteAddress(t)
			rts, err = su.Timestamp()
			if err != nil {
				t.Fatalf("%d: SingleUse.Read doesn't return a timestamp, error: %v", i, err)
			}
			if err := test.checkTs(rts); err != nil {
				t.Fatalf("%d: SingleUse.Read doesn't return expected timestamp: %v", i, err)
			}
			if !testEqual(got, test.want) {
				t.Fatalf("%d: got unexpected result from SingleUse.Read: %v, want %v", i, got, test.want)
			}
			// SingleUse.ReadRow
			got = nil
			for _, k := range []Key{{1}, {3}, {4}} {
				su = client.Single().WithTimestampBound(test.tb)
				r, err := su.ReadRow(ctx, "Singers", k, []string{"SingerId", "FirstName", "LastName"})
				if err != nil {
					continue
				}
				verifyDirectPathRemoteAddress(t)
				v, err := rowToValues(r)
				if err != nil {
					continue
				}
				got = append(got, v)
				rts, err = su.Timestamp()
				if err != nil {
					t.Fatalf("%d: SingleUse.ReadRow(%v) doesn't return a timestamp, error: %v", i, k, err)
				}
				if err := test.checkTs(rts); err != nil {
					t.Fatalf("%d: SingleUse.ReadRow(%v) doesn't return expected timestamp: %v", i, k, err)
				}
			}
			if !testEqual(got, test.want) {
				t.Fatalf("%d: got unexpected results from SingleUse.ReadRow: %v, want %v", i, got, test.want)
			}
			// SingleUse.ReadUsingIndex
			su = client.Single().WithTimestampBound(test.tb)
			got, err = readAll(su.ReadUsingIndex(ctx, "Singers", "SingerByName", KeySets(Key{"Marc", "Foo"}, Key{"Alpha", "Beta"}, Key{"Last", "End"}), []string{"SingerId", "FirstName", "LastName"}))
			if err != nil {
				t.Fatalf("%d: SingleUse.ReadUsingIndex returns error %v, want nil", i, err)
			}
			verifyDirectPathRemoteAddress(t)
			// The results from ReadUsingIndex is sorted by the index rather than primary key.
			if len(got) != len(test.want) {
				t.Fatalf("%d: got unexpected result from SingleUse.ReadUsingIndex: %v, want %v", i, got, test.want)
			}
			for j, g := range got {
				if j > 0 {
					prev := got[j-1][1].(string) + got[j-1][2].(string)
					curr := got[j][1].(string) + got[j][2].(string)
					if strings.Compare(prev, curr) > 0 {
						t.Fatalf("%d: SingleUse.ReadUsingIndex fails to order rows by index keys, %v should be after %v", i, got[j-1], got[j])
					}
				}
				found := false
				for _, w := range test.want {
					if testEqual(g, w) {
						found = true
					}
				}
				if !found {
					t.Fatalf("%d: got unexpected result from SingleUse.ReadUsingIndex: %v, want %v", i, got, test.want)
					break
				}
			}
			rts, err = su.Timestamp()
			if err != nil {
				t.Fatalf("%d: SingleUse.ReadUsingIndex doesn't return a timestamp, error: %v", i, err)
			}
			if err := test.checkTs(rts); err != nil {
				t.Fatalf("%d: SingleUse.ReadUsingIndex doesn't return expected timestamp: %v", i, err)
			}
			// SingleUse.ReadRowUsingIndex
			got = nil
			for _, k := range []Key{{"Marc", "Foo"}, {"Alpha", "Beta"}, {"Last", "End"}} {
				su = client.Single().WithTimestampBound(test.tb)
				r, err := su.ReadRowUsingIndex(ctx, "Singers", "SingerByName", k, []string{"SingerId", "FirstName", "LastName"})
				if err != nil {
					continue
				}
				verifyDirectPathRemoteAddress(t)
				v, err := rowToValues(r)
				if err != nil {
					continue
				}
				got = append(got, v)
				rts, err = su.Timestamp()
				if err != nil {
					t.Fatalf("%d: SingleUse.ReadRowUsingIndex(%v) doesn't return a timestamp, error: %v", i, k, err)
				}
				if err := test.checkTs(rts); err != nil {
					t.Fatalf("%d: SingleUse.ReadRowUsingIndex(%v) doesn't return expected timestamp: %v", i, k, err)
				}
			}
			if !testEqual(got, test.want) {
				t.Fatalf("%d: got unexpected results from SingleUse.ReadRowUsingIndex: %v, want %v", i, got, test.want)
			}
		})
	}
}

// Test custom query options provided on query-level configuration.
func TestIntegration_SingleUse_WithQueryOptions(t *testing.T) {
	skipEmulatorTest(t)
	t.Parallel()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()
	// Set up testing environment.
	client, _, cleanup := prepareIntegrationTest(ctx, t, DefaultSessionPoolConfig, singerDBStatements)
	defer cleanup()

	writes := []struct {
		row []interface{}
		ts  time.Time
	}{
		{row: []interface{}{1, "Marc", "Foo"}},
		{row: []interface{}{2, "Tars", "Bar"}},
		{row: []interface{}{3, "Alpha", "Beta"}},
		{row: []interface{}{4, "Last", "End"}},
	}
	// Try to write four rows through the Apply API.
	for i, w := range writes {
		var err error
		m := InsertOrUpdate("Singers",
			[]string{"SingerId", "FirstName", "LastName"},
			w.row)
		if writes[i].ts, err = client.Apply(ctx, []*Mutation{m}, ApplyAtLeastOnce()); err != nil {
			t.Fatal(err)
		}
	}
	qo := QueryOptions{Options: &sppb.ExecuteSqlRequest_QueryOptions{OptimizerVersion: "1"}}
	got, err := readAll(client.Single().QueryWithOptions(ctx, Statement{
		"SELECT SingerId, FirstName, LastName FROM Singers WHERE SingerId IN (@id1, @id3, @id4)",
		map[string]interface{}{"id1": int64(1), "id3": int64(3), "id4": int64(4)},
	}, qo))

	if err != nil {
		t.Errorf("ReadOnlyTransaction.QueryWithOptions returns error %v, want nil", err)
	}

	want := [][]interface{}{{int64(1), "Marc", "Foo"}, {int64(3), "Alpha", "Beta"}, {int64(4), "Last", "End"}}
	if !testEqual(got, want) {
		t.Errorf("got unexpected result from ReadOnlyTransaction.QueryWithOptions: %v, want %v", got, want)
	}
}

func TestIntegration_SingleUse_ReadingWithLimit(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()
	// Set up testing environment.
	client, _, cleanup := prepareIntegrationTest(ctx, t, DefaultSessionPoolConfig, singerDBStatements)
	defer cleanup()

	writes := []struct {
		row []interface{}
		ts  time.Time
	}{
		{row: []interface{}{1, "Marc", "Foo"}},
		{row: []interface{}{2, "Tars", "Bar"}},
		{row: []interface{}{3, "Alpha", "Beta"}},
		{row: []interface{}{4, "Last", "End"}},
	}
	// Try to write four rows through the Apply API.
	for i, w := range writes {
		var err error
		m := InsertOrUpdate("Singers",
			[]string{"SingerId", "FirstName", "LastName"},
			w.row)
		if writes[i].ts, err = client.Apply(ctx, []*Mutation{m}, ApplyAtLeastOnce()); err != nil {
			t.Fatal(err)
		}
	}

	su := client.Single()
	const limit = 1
	gotRows, err := readAll(su.ReadWithOptions(ctx, "Singers", KeySets(Key{1}, Key{3}, Key{4}),
		[]string{"SingerId", "FirstName", "LastName"}, &ReadOptions{Limit: limit}))
	if err != nil {
		t.Errorf("SingleUse.ReadWithOptions returns error %v, want nil", err)
	}
	if got, want := len(gotRows), limit; got != want {
		t.Errorf("got %d, want %d", got, want)
	}
}

// Test ReadOnlyTransaction. The testsuite is mostly like SingleUse, except it
// also tests for a single timestamp across multiple reads.
func TestIntegration_ReadOnlyTransaction(t *testing.T) {
	t.Parallel()

	ctxTimeout := 5 * time.Minute
	ctx, cancel := context.WithTimeout(context.Background(), ctxTimeout)
	defer cancel()
	// Set up testing environment.
	client, _, cleanup := prepareIntegrationTest(ctx, t, DefaultSessionPoolConfig, singerDBStatements)
	defer cleanup()

	writes := []struct {
		row []interface{}
		ts  time.Time
	}{
		{row: []interface{}{1, "Marc", "Foo"}},
		{row: []interface{}{2, "Tars", "Bar"}},
		{row: []interface{}{3, "Alpha", "Beta"}},
		{row: []interface{}{4, "Last", "End"}},
	}
	// Try to write four rows through the Apply API.
	for i, w := range writes {
		var err error
		m := InsertOrUpdate("Singers",
			[]string{"SingerId", "FirstName", "LastName"},
			w.row)
		if writes[i].ts, err = client.Apply(ctx, []*Mutation{m}, ApplyAtLeastOnce()); err != nil {
			t.Fatal(err)
		}
	}

	// For testing timestamp bound staleness.
	<-time.After(time.Second)

	// Test reading rows with different timestamp bounds.
	for i, test := range []struct {
		want    [][]interface{}
		tb      TimestampBound
		checkTs func(time.Time) error
	}{
		// Note: min_read_timestamp and max_staleness are not supported by
		// ReadOnlyTransaction. See API document for more details.
		{
			// strong
			[][]interface{}{{int64(1), "Marc", "Foo"}, {int64(3), "Alpha", "Beta"}, {int64(4), "Last", "End"}},
			StrongRead(),
			func(ts time.Time) error {
				if ts.Before(writes[3].ts) {
					return fmt.Errorf("read got timestamp %v, want it to be no later than %v", ts, writes[3].ts)
				}
				return nil
			},
		},
		{
			// read_timestamp
			[][]interface{}{{int64(1), "Marc", "Foo"}, {int64(3), "Alpha", "Beta"}},
			ReadTimestamp(writes[2].ts),
			func(ts time.Time) error {
				if ts != writes[2].ts {
					return fmt.Errorf("read got timestamp %v, expect %v", ts, writes[2].ts)
				}
				return nil
			},
		},
		{
			// exact_staleness
			nil,
			// Specify a staleness which should be already before this test.
			ExactStaleness(ctxTimeout + 1),
			func(ts time.Time) error {
				if ts.After(writes[0].ts) {
					return fmt.Errorf("read got timestamp %v, want it to be no earlier than %v", ts, writes[0].ts)
				}
				return nil
			},
		},
	} {
		// ReadOnlyTransaction.Query
		ro := client.ReadOnlyTransaction().WithTimestampBound(test.tb)
		got, err := readAll(ro.Query(
			ctx,
			Statement{
				"SELECT SingerId, FirstName, LastName FROM Singers WHERE SingerId IN (@id1, @id3, @id4) ORDER BY SingerId",
				map[string]interface{}{"id1": int64(1), "id3": int64(3), "id4": int64(4)},
			}))
		if err != nil {
			t.Errorf("%d: ReadOnlyTransaction.Query returns error %v, want nil", i, err)
		}
		if !testEqual(got, test.want) {
			t.Errorf("%d: got unexpected result from ReadOnlyTransaction.Query: %v, want %v", i, got, test.want)
		}
		rts, err := ro.Timestamp()
		if err != nil {
			t.Errorf("%d: ReadOnlyTransaction.Query doesn't return a timestamp, error: %v", i, err)
		}
		if err := test.checkTs(rts); err != nil {
			t.Errorf("%d: ReadOnlyTransaction.Query doesn't return expected timestamp: %v", i, err)
		}
		roTs := rts
		// ReadOnlyTransaction.Read
		got, err = readAll(ro.Read(ctx, "Singers", KeySets(Key{1}, Key{3}, Key{4}), []string{"SingerId", "FirstName", "LastName"}))
		if err != nil {
			t.Errorf("%d: ReadOnlyTransaction.Read returns error %v, want nil", i, err)
		}
		if !testEqual(got, test.want) {
			t.Errorf("%d: got unexpected result from ReadOnlyTransaction.Read: %v, want %v", i, got, test.want)
		}
		rts, err = ro.Timestamp()
		if err != nil {
			t.Errorf("%d: ReadOnlyTransaction.Read doesn't return a timestamp, error: %v", i, err)
		}
		if err := test.checkTs(rts); err != nil {
			t.Errorf("%d: ReadOnlyTransaction.Read doesn't return expected timestamp: %v", i, err)
		}
		if roTs != rts {
			t.Errorf("%d: got two read timestamps: %v, %v, want ReadOnlyTransaction to return always the same read timestamp", i, roTs, rts)
		}
		// ReadOnlyTransaction.ReadRow
		got = nil
		for _, k := range []Key{{1}, {3}, {4}} {
			r, err := ro.ReadRow(ctx, "Singers", k, []string{"SingerId", "FirstName", "LastName"})
			if err != nil {
				continue
			}
			v, err := rowToValues(r)
			if err != nil {
				continue
			}
			got = append(got, v)
			rts, err = ro.Timestamp()
			if err != nil {
				t.Errorf("%d: ReadOnlyTransaction.ReadRow(%v) doesn't return a timestamp, error: %v", i, k, err)
			}
			if err := test.checkTs(rts); err != nil {
				t.Errorf("%d: ReadOnlyTransaction.ReadRow(%v) doesn't return expected timestamp: %v", i, k, err)
			}
			if roTs != rts {
				t.Errorf("%d: got two read timestamps: %v, %v, want ReadOnlyTransaction to return always the same read timestamp", i, roTs, rts)
			}
		}
		if !testEqual(got, test.want) {
			t.Errorf("%d: got unexpected results from ReadOnlyTransaction.ReadRow: %v, want %v", i, got, test.want)
		}
		// SingleUse.ReadUsingIndex
		got, err = readAll(ro.ReadUsingIndex(ctx, "Singers", "SingerByName", KeySets(Key{"Marc", "Foo"}, Key{"Alpha", "Beta"}, Key{"Last", "End"}), []string{"SingerId", "FirstName", "LastName"}))
		if err != nil {
			t.Errorf("%d: ReadOnlyTransaction.ReadUsingIndex returns error %v, want nil", i, err)
		}
		// The results from ReadUsingIndex is sorted by the index rather than
		// primary key.
		if len(got) != len(test.want) {
			t.Errorf("%d: got unexpected result from ReadOnlyTransaction.ReadUsingIndex: %v, want %v", i, got, test.want)
		}
		for j, g := range got {
			if j > 0 {
				prev := got[j-1][1].(string) + got[j-1][2].(string)
				curr := got[j][1].(string) + got[j][2].(string)
				if strings.Compare(prev, curr) > 0 {
					t.Errorf("%d: ReadOnlyTransaction.ReadUsingIndex fails to order rows by index keys, %v should be after %v", i, got[j-1], got[j])
				}
			}
			found := false
			for _, w := range test.want {
				if testEqual(g, w) {
					found = true
				}
			}
			if !found {
				t.Errorf("%d: got unexpected result from ReadOnlyTransaction.ReadUsingIndex: %v, want %v", i, got, test.want)
				break
			}
		}
		rts, err = ro.Timestamp()
		if err != nil {
			t.Errorf("%d: ReadOnlyTransaction.ReadUsingIndex doesn't return a timestamp, error: %v", i, err)
		}
		if err := test.checkTs(rts); err != nil {
			t.Errorf("%d: ReadOnlyTransaction.ReadUsingIndex doesn't return expected timestamp: %v", i, err)
		}
		if roTs != rts {
			t.Errorf("%d: got two read timestamps: %v, %v, want ReadOnlyTransaction to return always the same read timestamp", i, roTs, rts)
		}
		// ReadOnlyTransaction.ReadRowUsingIndex
		got = nil
		for _, k := range []Key{{"Marc", "Foo"}, {"Alpha", "Beta"}, {"Last", "End"}} {
			r, err := ro.ReadRowUsingIndex(ctx, "Singers", "SingerByName", k, []string{"SingerId", "FirstName", "LastName"})
			if err != nil {
				continue
			}
			v, err := rowToValues(r)
			if err != nil {
				continue
			}
			got = append(got, v)
			rts, err = ro.Timestamp()
			if err != nil {
				t.Errorf("%d: ReadOnlyTransaction.ReadRowUsingIndex(%v) doesn't return a timestamp, error: %v", i, k, err)
			}
			if err := test.checkTs(rts); err != nil {
				t.Errorf("%d: ReadOnlyTransaction.ReadRowUsingIndex(%v) doesn't return expected timestamp: %v", i, k, err)
			}
			if roTs != rts {
				t.Errorf("%d: got two read timestamps: %v, %v, want ReadOnlyTransaction to return always the same read timestamp", i, roTs, rts)
			}
		}
		if !testEqual(got, test.want) {
			t.Errorf("%d: got unexpected results from ReadOnlyTransaction.ReadRowUsingIndex: %v, want %v", i, got, test.want)
		}
		ro.Close()
	}
}

// Test ReadOnlyTransaction with different timestamp bound when there's an
// update at the same time.
func TestIntegration_UpdateDuringRead(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()
	client, _, cleanup := prepareIntegrationTest(ctx, t, DefaultSessionPoolConfig, singerDBStatements)
	defer cleanup()

	for i, tb := range []TimestampBound{
		StrongRead(),
		ReadTimestamp(time.Now().Add(-time.Minute * 30)), // version GC is 1 hour
		ExactStaleness(time.Minute * 30),
	} {
		ro := client.ReadOnlyTransaction().WithTimestampBound(tb)
		_, err := ro.ReadRow(ctx, "Singers", Key{i}, []string{"SingerId"})
		if ErrCode(err) != codes.NotFound {
			t.Errorf("%d: ReadOnlyTransaction.ReadRow before write returns error: %v, want NotFound", i, err)
		}

		m := InsertOrUpdate("Singers", []string{"SingerId"}, []interface{}{i})
		if _, err := client.Apply(ctx, []*Mutation{m}, ApplyAtLeastOnce()); err != nil {
			t.Fatal(err)
		}

		_, err = ro.ReadRow(ctx, "Singers", Key{i}, []string{"SingerId"})
		if ErrCode(err) != codes.NotFound {
			t.Errorf("%d: ReadOnlyTransaction.ReadRow after write returns error: %v, want NotFound", i, err)
		}
	}
}

// Test ReadWriteTransaction.
func TestIntegration_ReadWriteTransaction(t *testing.T) {
	t.Parallel()

	// Give a longer deadline because of transaction backoffs.
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()
	client, _, cleanup := prepareIntegrationTest(ctx, t, DefaultSessionPoolConfig, singerDBStatements)
	defer cleanup()

	// Set up two accounts
	accounts := []*Mutation{
		Insert("Accounts", []string{"AccountId", "Nickname", "Balance"}, []interface{}{int64(1), "Foo", int64(50)}),
		Insert("Accounts", []string{"AccountId", "Nickname", "Balance"}, []interface{}{int64(2), "Bar", int64(1)}),
	}
	if _, err := client.Apply(ctx, accounts, ApplyAtLeastOnce()); err != nil {
		t.Fatal(err)
	}
	wg := sync.WaitGroup{}

	readBalance := func(iter *RowIterator) (int64, error) {
		defer iter.Stop()
		var bal int64
		for {
			row, err := iter.Next()
			if err == iterator.Done {
				return bal, nil
			}
			if err != nil {
				return 0, err
			}
			if err := row.Column(0, &bal); err != nil {
				return 0, err
			}
		}
	}

	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func(iter int) {
			defer wg.Done()
			_, err := client.ReadWriteTransaction(ctx, func(ctx context.Context, tx *ReadWriteTransaction) error {
				// Query Foo's balance and Bar's balance.
				bf, e := readBalance(tx.Query(ctx,
					Statement{"SELECT Balance FROM Accounts WHERE AccountId = @id", map[string]interface{}{"id": int64(1)}}))
				if e != nil {
					return e
				}
				verifyDirectPathRemoteAddress(t)
				bb, e := readBalance(tx.Read(ctx, "Accounts", KeySets(Key{int64(2)}), []string{"Balance"}))
				if e != nil {
					return e
				}
				verifyDirectPathRemoteAddress(t)
				if bf <= 0 {
					return nil
				}
				bf--
				bb++
				return tx.BufferWrite([]*Mutation{
					Update("Accounts", []string{"AccountId", "Balance"}, []interface{}{int64(1), bf}),
					Update("Accounts", []string{"AccountId", "Balance"}, []interface{}{int64(2), bb}),
				})
			})
			if err != nil {
				t.Errorf("%d: failed to execute transaction: %v", iter, err)
			}
			verifyDirectPathRemoteAddress(t)
		}(i)
	}
	// Because of context timeout, all goroutines will eventually return.
	wg.Wait()
	_, err := client.ReadWriteTransaction(ctx, func(ctx context.Context, tx *ReadWriteTransaction) error {
		var bf, bb int64
		r, e := tx.ReadRow(ctx, "Accounts", Key{int64(1)}, []string{"Balance"})
		if e != nil {
			return e
		}
		verifyDirectPathRemoteAddress(t)
		if ce := r.Column(0, &bf); ce != nil {
			return ce
		}
		bb, e = readBalance(tx.ReadUsingIndex(ctx, "Accounts", "AccountByNickname", KeySets(Key{"Bar"}), []string{"Balance"}))
		if e != nil {
			return e
		}
		verifyDirectPathRemoteAddress(t)
		if bf != 30 || bb != 21 {
			t.Errorf("Foo's balance is now %v and Bar's balance is now %v, want %v and %v", bf, bb, 30, 21)
		}
		return nil
	})
	if err != nil {
		t.Errorf("failed to check balances: %v", err)
	}
	verifyDirectPathRemoteAddress(t)
}

func TestIntegration_ReadWriteTransaction_StatementBased(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()
	client, _, cleanup := prepareIntegrationTest(ctx, t, DefaultSessionPoolConfig, singerDBStatements)
	defer cleanup()

	// Set up two accounts
	accounts := []*Mutation{
		Insert("Accounts", []string{"AccountId", "Nickname", "Balance"}, []interface{}{int64(1), "Foo", int64(50)}),
		Insert("Accounts", []string{"AccountId", "Nickname", "Balance"}, []interface{}{int64(2), "Bar", int64(1)}),
	}
	if _, err := client.Apply(ctx, accounts, ApplyAtLeastOnce()); err != nil {
		t.Fatal(err)
	}

	getBalance := func(txn *ReadWriteStmtBasedTransaction, key Key) (int64, error) {
		row, err := txn.ReadRow(ctx, "Accounts", key, []string{"Balance"})
		if err != nil {
			return 0, err
		}
		var balance int64
		if err := row.Column(0, &balance); err != nil {
			return 0, err
		}
		return balance, nil
	}

	statements := func(txn *ReadWriteStmtBasedTransaction) error {
		outBalance, err := getBalance(txn, Key{1})
		if err != nil {
			return err
		}
		const transferAmt = 20
		if outBalance >= transferAmt {
			inBalance, err := getBalance(txn, Key{2})
			if err != nil {
				return err
			}
			inBalance += transferAmt
			outBalance -= transferAmt
			cols := []string{"AccountId", "Balance"}
			txn.BufferWrite([]*Mutation{
				Update("Accounts", cols, []interface{}{1, outBalance}),
				Update("Accounts", cols, []interface{}{2, inBalance}),
			})
		}
		return nil
	}

	for {
		tx, err := NewReadWriteStmtBasedTransaction(ctx, client)
		if err != nil {
			t.Fatalf("failed to begin a transaction: %v", err)
		}
		err = statements(tx)
		if err != nil && status.Code(err) != codes.Aborted {
			tx.Rollback(ctx)
			t.Fatalf("failed to execute statements: %v", err)
		} else if err == nil {
			_, err = tx.Commit(ctx)
			if err == nil {
				break
			} else if status.Code(err) != codes.Aborted {
				t.Fatalf("failed to commit a transaction: %v", err)
			}
		}
		// Set a default sleep time if the server delay is absent.
		delay := 10 * time.Millisecond
		if serverDelay, hasServerDelay := ExtractRetryDelay(err); hasServerDelay {
			delay = serverDelay
		}
		time.Sleep(delay)
	}

	// Query the updated values.
	stmt := Statement{SQL: `SELECT AccountId, Nickname, Balance FROM Accounts ORDER BY AccountId`}
	iter := client.Single().Query(ctx, stmt)
	got, err := readAllAccountsTable(iter)
	if err != nil {
		t.Fatalf("failed to read data: %v", err)
	}
	want := [][]interface{}{
		{int64(1), "Foo", int64(30)},
		{int64(2), "Bar", int64(21)},
	}
	if !testEqual(got, want) {
		t.Errorf("\ngot %v\nwant%v", got, want)
	}
}

func TestIntegration_Reads(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()
	// Set up testing environment.
	client, _, cleanup := prepareIntegrationTest(ctx, t, DefaultSessionPoolConfig, readDBStatements)
	defer cleanup()

	// Includes k0..k14. Strings sort lexically, eg "k1" < "k10" < "k2".
	var ms []*Mutation
	for i := 0; i < 15; i++ {
		ms = append(ms, InsertOrUpdate(testTable,
			testTableColumns,
			[]interface{}{fmt.Sprintf("k%d", i), fmt.Sprintf("v%d", i)}))
	}
	// Don't use ApplyAtLeastOnce, so we can test the other code path.
	if _, err := client.Apply(ctx, ms); err != nil {
		t.Fatal(err)
	}
	verifyDirectPathRemoteAddress(t)

	// Empty read.
	rows, err := readAllTestTable(client.Single().Read(ctx, testTable,
		KeyRange{Start: Key{"k99"}, End: Key{"z"}}, testTableColumns))
	if err != nil {
		t.Fatal(err)
	}
	verifyDirectPathRemoteAddress(t)
	if got, want := len(rows), 0; got != want {
		t.Errorf("got %d, want %d", got, want)
	}

	// Index empty read.
	rows, err = readAllTestTable(client.Single().ReadUsingIndex(ctx, testTable, testTableIndex,
		KeyRange{Start: Key{"v99"}, End: Key{"z"}}, testTableColumns))
	if err != nil {
		t.Fatal(err)
	}
	verifyDirectPathRemoteAddress(t)
	if got, want := len(rows), 0; got != want {
		t.Errorf("got %d, want %d", got, want)
	}

	// Point read.
	row, err := client.Single().ReadRow(ctx, testTable, Key{"k1"}, testTableColumns)
	if err != nil {
		t.Fatal(err)
	}
	verifyDirectPathRemoteAddress(t)
	var got testTableRow
	if err := row.ToStruct(&got); err != nil {
		t.Fatal(err)
	}
	if want := (testTableRow{"k1", "v1"}); got != want {
		t.Errorf("got %v, want %v", got, want)
	}

	// Point read not found.
	_, err = client.Single().ReadRow(ctx, testTable, Key{"k999"}, testTableColumns)
	if ErrCode(err) != codes.NotFound {
		t.Fatalf("got %v, want NotFound", err)
	}
	verifyDirectPathRemoteAddress(t)

	// Index point read.
	rowIndex, err := client.Single().ReadRowUsingIndex(ctx, testTable, testTableIndex, Key{"v1"}, testTableColumns)
	if err != nil {
		t.Fatal(err)
	}
	verifyDirectPathRemoteAddress(t)
	var gotIndex testTableRow
	if err := rowIndex.ToStruct(&gotIndex); err != nil {
		t.Fatal(err)
	}
	if wantIndex := (testTableRow{"k1", "v1"}); gotIndex != wantIndex {
		t.Errorf("got %v, want %v", gotIndex, wantIndex)
	}
	// Index point read not found.
	_, err = client.Single().ReadRowUsingIndex(ctx, testTable, testTableIndex, Key{"v999"}, testTableColumns)
	if ErrCode(err) != codes.NotFound {
		t.Fatalf("got %v, want NotFound", err)
	}
	verifyDirectPathRemoteAddress(t)
	rangeReads(ctx, t, client)
	indexRangeReads(ctx, t, client)
}

func TestIntegration_EarlyTimestamp(t *testing.T) {
	t.Parallel()

	// Test that we can get the timestamp from a read-only transaction as
	// soon as we have read at least one row.
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()
	// Set up testing environment.
	client, _, cleanup := prepareIntegrationTest(ctx, t, DefaultSessionPoolConfig, readDBStatements)
	defer cleanup()

	var ms []*Mutation
	for i := 0; i < 3; i++ {
		ms = append(ms, InsertOrUpdate(testTable,
			testTableColumns,
			[]interface{}{fmt.Sprintf("k%d", i), fmt.Sprintf("v%d", i)}))
	}
	if _, err := client.Apply(ctx, ms, ApplyAtLeastOnce()); err != nil {
		t.Fatal(err)
	}

	txn := client.Single()
	iter := txn.Read(ctx, testTable, AllKeys(), testTableColumns)
	defer iter.Stop()
	// In single-use transaction, we should get an error before reading anything.
	if _, err := txn.Timestamp(); err == nil {
		t.Error("wanted error, got nil")
	}
	// After reading one row, the timestamp should be available.
	_, err := iter.Next()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := txn.Timestamp(); err != nil {
		t.Errorf("got %v, want nil", err)
	}

	txn = client.ReadOnlyTransaction()
	defer txn.Close()
	iter = txn.Read(ctx, testTable, AllKeys(), testTableColumns)
	defer iter.Stop()
	// In an ordinary read-only transaction, the timestamp should be
	// available immediately.
	if _, err := txn.Timestamp(); err != nil {
		t.Errorf("got %v, want nil", err)
	}
}

func TestIntegration_NestedTransaction(t *testing.T) {
	t.Parallel()

	// You cannot use a transaction from inside a read-write transaction.
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()
	client, _, cleanup := prepareIntegrationTest(ctx, t, DefaultSessionPoolConfig, singerDBStatements)
	defer cleanup()

	_, err := client.ReadWriteTransaction(ctx, func(ctx context.Context, tx *ReadWriteTransaction) error {
		_, err := client.ReadWriteTransaction(ctx,
			func(context.Context, *ReadWriteTransaction) error { return nil })
		if ErrCode(err) != codes.FailedPrecondition {
			t.Fatalf("got %v, want FailedPrecondition", err)
		}
		_, err = client.Single().ReadRow(ctx, "Singers", Key{1}, []string{"SingerId"})
		if ErrCode(err) != codes.FailedPrecondition {
			t.Fatalf("got %v, want FailedPrecondition", err)
		}
		rot := client.ReadOnlyTransaction()
		defer rot.Close()
		_, err = rot.ReadRow(ctx, "Singers", Key{1}, []string{"SingerId"})
		if ErrCode(err) != codes.FailedPrecondition {
			t.Fatalf("got %v, want FailedPrecondition", err)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

func TestIntegration_CreateDBRetry(t *testing.T) {
	t.Parallel()

	if databaseAdmin == nil {
		t.Skip("Integration tests skipped")
	}
	skipEmulatorTest(t)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()
	dbName := dbNameSpace.New()

	// Simulate an Unavailable error on the CreateDatabase RPC.
	hasReturnedUnavailable := false
	interceptor := func(ctx context.Context, method string, req, reply interface{}, cc *grpc.ClientConn, invoker grpc.UnaryInvoker, opts ...grpc.CallOption) error {
		err := invoker(ctx, method, req, reply, cc, opts...)
		if !hasReturnedUnavailable && err == nil {
			hasReturnedUnavailable = true
			return status.Error(codes.Unavailable, "Injected unavailable error")
		}
		return err
	}

	dbAdmin, err := database.NewDatabaseAdminClient(ctx, option.WithGRPCDialOption(grpc.WithUnaryInterceptor(interceptor)))
	if err != nil {
		log.Fatalf("cannot create dbAdmin client: %v", err)
	}
	op, err := dbAdmin.CreateDatabaseWithRetry(ctx, &adminpb.CreateDatabaseRequest{
		Parent:          fmt.Sprintf("projects/%v/instances/%v", testProjectID, testInstanceID),
		CreateStatement: "CREATE DATABASE " + dbName,
	})
	if err != nil {
		t.Fatalf("failed to create database: %v", err)
	}
	_, err = op.Wait(ctx)
	if err != nil {
		t.Fatalf("failed to create database: %v", err)
	}
	if !hasReturnedUnavailable {
		t.Fatalf("interceptor did not return Unavailable")
	}
}

// Test client recovery on database recreation.
func TestIntegration_DbRemovalRecovery(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()
	// Create a client with MinOpened=0 to prevent the session pool maintainer
	// from repeatedly trying to create sessions for the invalid database.
	client, dbPath, cleanup := prepareIntegrationTest(ctx, t, SessionPoolConfig{}, singerDBStatements)
	defer cleanup()

	// Drop the testing database.
	if err := databaseAdmin.DropDatabase(ctx, &adminpb.DropDatabaseRequest{Database: dbPath}); err != nil {
		t.Fatalf("failed to drop testing database %v: %v", dbPath, err)
	}

	// Now, send the query.
	iter := client.Single().Query(ctx, Statement{SQL: "SELECT SingerId FROM Singers"})
	defer iter.Stop()
	if _, err := iter.Next(); err == nil {
		t.Errorf("client sends query to removed database successfully, want it to fail")
	}
	verifyDirectPathRemoteAddress(t)

	// Recreate database and table.
	dbName := dbPath[strings.LastIndex(dbPath, "/")+1:]
	op, err := databaseAdmin.CreateDatabaseWithRetry(ctx, &adminpb.CreateDatabaseRequest{
		Parent:          fmt.Sprintf("projects/%v/instances/%v", testProjectID, testInstanceID),
		CreateStatement: "CREATE DATABASE " + dbName,
		ExtraStatements: []string{
			`CREATE TABLE Singers (
				SingerId        INT64 NOT NULL,
				FirstName       STRING(1024),
				LastName        STRING(1024),
				SingerInfo      BYTES(MAX)
			) PRIMARY KEY (SingerId)`,
		},
	})
	if err != nil {
		t.Fatalf("cannot recreate testing DB %v: %v", dbPath, err)
	}
	if _, err := op.Wait(ctx); err != nil {
		t.Fatalf("cannot recreate testing DB %v: %v", dbPath, err)
	}

	// Now, send the query again.
	iter = client.Single().Query(ctx, Statement{SQL: "SELECT SingerId FROM Singers"})
	defer iter.Stop()
	_, err = iter.Next()
	if err != nil && err != iterator.Done {
		t.Errorf("failed to send query to database %v: %v", dbPath, err)
	}
	verifyDirectPathRemoteAddress(t)
}

// Test encoding/decoding non-struct Cloud Spanner types.
func TestIntegration_BasicTypes(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()
	stmts := singerDBStatements
	if !isEmulatorEnvSet() {
		stmts = []string{
			`CREATE TABLE Singers (
					SingerId	INT64 NOT NULL,
					FirstName	STRING(1024),
					LastName	STRING(1024),
					SingerInfo	BYTES(MAX)
				) PRIMARY KEY (SingerId)`,
			`CREATE INDEX SingerByName ON Singers(FirstName, LastName)`,
			`CREATE TABLE Accounts (
					AccountId	INT64 NOT NULL,
					Nickname	STRING(100),
					Balance		INT64 NOT NULL,
				) PRIMARY KEY (AccountId)`,
			`CREATE INDEX AccountByNickname ON Accounts(Nickname) STORING (Balance)`,
			`CREATE TABLE Types (
					RowID		INT64 NOT NULL,
					String		STRING(MAX),
					StringArray	ARRAY<STRING(MAX)>,
					Bytes		BYTES(MAX),
					BytesArray	ARRAY<BYTES(MAX)>,
					Int64a		INT64,
					Int64Array	ARRAY<INT64>,
					Bool		BOOL,
					BoolArray	ARRAY<BOOL>,
					Float64		FLOAT64,
					Float64Array	ARRAY<FLOAT64>,
					Date		DATE,
					DateArray	ARRAY<DATE>,
					Timestamp	TIMESTAMP,
					TimestampArray	ARRAY<TIMESTAMP>,
					Numeric		NUMERIC,
					NumericArray	ARRAY<NUMERIC>
				) PRIMARY KEY (RowID)`,
		}
	}
	client, _, cleanup := prepareIntegrationTest(ctx, t, DefaultSessionPoolConfig, stmts)
	defer cleanup()

	t1, _ := time.Parse(time.RFC3339Nano, "2016-11-15T15:04:05.999999999Z")
	// Boundaries
	t2, _ := time.Parse(time.RFC3339Nano, "0001-01-01T00:00:00.000000000Z")
	t3, _ := time.Parse(time.RFC3339Nano, "9999-12-31T23:59:59.999999999Z")
	d1, _ := civil.ParseDate("2016-11-15")
	// Boundaries
	d2, _ := civil.ParseDate("0001-01-01")
	d3, _ := civil.ParseDate("9999-12-31")

	n0 := big.Rat{}
	n1p, _ := (&big.Rat{}).SetString("123456789")
	n2p, _ := (&big.Rat{}).SetString("123456789/1000000000")
	n1 := *n1p
	n2 := *n2p

	tests := []struct {
		col  string
		val  interface{}
		want interface{}
	}{
		{col: "String", val: ""},
		{col: "String", val: "", want: NullString{"", true}},
		{col: "String", val: "foo"},
		{col: "String", val: "foo", want: NullString{"foo", true}},
		{col: "String", val: NullString{"bar", true}, want: "bar"},
		{col: "String", val: NullString{"bar", false}, want: NullString{"", false}},
		{col: "String", val: nil, want: NullString{}},
		{col: "StringArray", val: []string(nil), want: []NullString(nil)},
		{col: "StringArray", val: []string{}, want: []NullString{}},
		{col: "StringArray", val: []string{"foo", "bar"}, want: []NullString{{"foo", true}, {"bar", true}}},
		{col: "StringArray", val: []NullString(nil)},
		{col: "StringArray", val: []NullString{}},
		{col: "StringArray", val: []NullString{{"foo", true}, {}}},
		{col: "Bytes", val: []byte{}},
		{col: "Bytes", val: []byte{1, 2, 3}},
		{col: "Bytes", val: []byte(nil)},
		{col: "BytesArray", val: [][]byte(nil)},
		{col: "BytesArray", val: [][]byte{}},
		{col: "BytesArray", val: [][]byte{{1}, {2, 3}}},
		{col: "Int64a", val: 0, want: int64(0)},
		{col: "Int64a", val: -1, want: int64(-1)},
		{col: "Int64a", val: 2, want: int64(2)},
		{col: "Int64a", val: int64(3)},
		{col: "Int64a", val: 4, want: NullInt64{4, true}},
		{col: "Int64a", val: NullInt64{5, true}, want: int64(5)},
		{col: "Int64a", val: NullInt64{6, true}, want: int64(6)},
		{col: "Int64a", val: NullInt64{7, false}, want: NullInt64{0, false}},
		{col: "Int64a", val: nil, want: NullInt64{}},
		{col: "Int64Array", val: []int(nil), want: []NullInt64(nil)},
		{col: "Int64Array", val: []int{}, want: []NullInt64{}},
		{col: "Int64Array", val: []int{1, 2}, want: []NullInt64{{1, true}, {2, true}}},
		{col: "Int64Array", val: []int64(nil), want: []NullInt64(nil)},
		{col: "Int64Array", val: []int64{}, want: []NullInt64{}},
		{col: "Int64Array", val: []int64{1, 2}, want: []NullInt64{{1, true}, {2, true}}},
		{col: "Int64Array", val: []NullInt64(nil)},
		{col: "Int64Array", val: []NullInt64{}},
		{col: "Int64Array", val: []NullInt64{{1, true}, {}}},
		{col: "Bool", val: false},
		{col: "Bool", val: true},
		{col: "Bool", val: false, want: NullBool{false, true}},
		{col: "Bool", val: true, want: NullBool{true, true}},
		{col: "Bool", val: NullBool{true, true}},
		{col: "Bool", val: NullBool{false, false}},
		{col: "Bool", val: nil, want: NullBool{}},
		{col: "BoolArray", val: []bool(nil), want: []NullBool(nil)},
		{col: "BoolArray", val: []bool{}, want: []NullBool{}},
		{col: "BoolArray", val: []bool{true, false}, want: []NullBool{{true, true}, {false, true}}},
		{col: "BoolArray", val: []NullBool(nil)},
		{col: "BoolArray", val: []NullBool{}},
		{col: "BoolArray", val: []NullBool{{false, true}, {true, true}, {}}},
		{col: "Float64", val: 0.0},
		{col: "Float64", val: 3.14},
		{col: "Float64", val: math.NaN()},
		{col: "Float64", val: math.Inf(1)},
		{col: "Float64", val: math.Inf(-1)},
		{col: "Float64", val: 2.78, want: NullFloat64{2.78, true}},
		{col: "Float64", val: NullFloat64{2.71, true}, want: 2.71},
		{col: "Float64", val: NullFloat64{1.41, true}, want: NullFloat64{1.41, true}},
		{col: "Float64", val: NullFloat64{0, false}},
		{col: "Float64", val: nil, want: NullFloat64{}},
		{col: "Float64Array", val: []float64(nil), want: []NullFloat64(nil)},
		{col: "Float64Array", val: []float64{}, want: []NullFloat64{}},
		{col: "Float64Array", val: []float64{2.72, 3.14, math.Inf(1)}, want: []NullFloat64{{2.72, true}, {3.14, true}, {math.Inf(1), true}}},
		{col: "Float64Array", val: []NullFloat64(nil)},
		{col: "Float64Array", val: []NullFloat64{}},
		{col: "Float64Array", val: []NullFloat64{{2.72, true}, {math.Inf(1), true}, {}}},
		{col: "Date", val: d1},
		{col: "Date", val: d1, want: NullDate{d1, true}},
		{col: "Date", val: NullDate{d1, true}},
		{col: "Date", val: NullDate{d1, true}, want: d1},
		{col: "Date", val: NullDate{civil.Date{}, false}},
		{col: "DateArray", val: []civil.Date(nil), want: []NullDate(nil)},
		{col: "DateArray", val: []civil.Date{}, want: []NullDate{}},
		{col: "DateArray", val: []civil.Date{d1, d2, d3}, want: []NullDate{{d1, true}, {d2, true}, {d3, true}}},
		{col: "Timestamp", val: t1},
		{col: "Timestamp", val: t1, want: NullTime{t1, true}},
		{col: "Timestamp", val: NullTime{t1, true}},
		{col: "Timestamp", val: NullTime{t1, true}, want: t1},
		{col: "Timestamp", val: NullTime{}},
		{col: "Timestamp", val: nil, want: NullTime{}},
		{col: "TimestampArray", val: []time.Time(nil), want: []NullTime(nil)},
		{col: "TimestampArray", val: []time.Time{}, want: []NullTime{}},
		{col: "TimestampArray", val: []time.Time{t1, t2, t3}, want: []NullTime{{t1, true}, {t2, true}, {t3, true}}},
	}

	if !isEmulatorEnvSet() {
		for _, tc := range []struct {
			col  string
			val  interface{}
			want interface{}
		}{
			{col: "Numeric", val: n1},
			{col: "Numeric", val: n2},
			{col: "Numeric", val: n1, want: NullNumeric{n1, true}},
			{col: "Numeric", val: n2, want: NullNumeric{n2, true}},
			{col: "Numeric", val: NullNumeric{n1, true}, want: n1},
			{col: "Numeric", val: NullNumeric{n1, true}, want: NullNumeric{n1, true}},
			{col: "Numeric", val: NullNumeric{n0, false}},
			{col: "Numeric", val: nil, want: NullNumeric{}},
			{col: "NumericArray", val: []big.Rat(nil), want: []NullNumeric(nil)},
			{col: "NumericArray", val: []big.Rat{}, want: []NullNumeric{}},
			{col: "NumericArray", val: []big.Rat{n1, n2}, want: []NullNumeric{{n1, true}, {n2, true}}},
			{col: "NumericArray", val: []NullNumeric(nil)},
			{col: "NumericArray", val: []NullNumeric{}},
			{col: "NumericArray", val: []NullNumeric{{n1, true}, {n2, true}, {}}},
		} {
			tests = append(tests, tc)
		}
	}

	// Write rows into table first.
	var muts []*Mutation
	for i, test := range tests {
		muts = append(muts, InsertOrUpdate("Types", []string{"RowID", test.col}, []interface{}{i, test.val}))
	}
	if _, err := client.Apply(ctx, muts, ApplyAtLeastOnce()); err != nil {
		t.Fatal(err)
	}

	for i, test := range tests {
		row, err := client.Single().ReadRow(ctx, "Types", []interface{}{i}, []string{test.col})
		if err != nil {
			t.Fatalf("Unable to fetch row %v: %v", i, err)
		}
		verifyDirectPathRemoteAddress(t)
		// Create new instance of type of test.want.
		want := test.want
		if want == nil {
			want = test.val
		}
		gotp := reflect.New(reflect.TypeOf(want))
		if err := row.Column(0, gotp.Interface()); err != nil {
			t.Errorf("%d: col:%v val:%#v, %v", i, test.col, test.val, err)
			continue
		}
		got := reflect.Indirect(gotp).Interface()

		// One of the test cases is checking NaN handling.  Given
		// NaN!=NaN, we can't use reflect to test for it.
		if isNaN(got) && isNaN(want) {
			continue
		}

		// Check non-NaN cases.
		if !testEqual(got, want) {
			t.Errorf("%d: col:%v val:%#v, got %#v, want %#v", i, test.col, test.val, got, want)
			continue
		}
	}
}

// Test decoding Cloud Spanner STRUCT type.
func TestIntegration_StructTypes(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()
	client, _, cleanup := prepareIntegrationTest(ctx, t, DefaultSessionPoolConfig, singerDBStatements)
	defer cleanup()

	tests := []struct {
		q    Statement
		want func(r *Row) error
	}{
		{
			q: Statement{SQL: `SELECT ARRAY(SELECT STRUCT(1, 2))`},
			want: func(r *Row) error {
				// Test STRUCT ARRAY decoding to []NullRow.
				var rows []NullRow
				if err := r.Column(0, &rows); err != nil {
					return err
				}
				if len(rows) != 1 {
					return fmt.Errorf("len(rows) = %d; want 1", len(rows))
				}
				if !rows[0].Valid {
					return fmt.Errorf("rows[0] is NULL")
				}
				var i, j int64
				if err := rows[0].Row.Columns(&i, &j); err != nil {
					return err
				}
				if i != 1 || j != 2 {
					return fmt.Errorf("got (%d,%d), want (1,2)", i, j)
				}
				return nil
			},
		},
		{
			q: Statement{SQL: `SELECT ARRAY(SELECT STRUCT(1 as foo, 2 as bar)) as col1`},
			want: func(r *Row) error {
				// Test Row.ToStruct.
				s := struct {
					Col1 []*struct {
						Foo int64 `spanner:"foo"`
						Bar int64 `spanner:"bar"`
					} `spanner:"col1"`
				}{}
				if err := r.ToStruct(&s); err != nil {
					return err
				}
				want := struct {
					Col1 []*struct {
						Foo int64 `spanner:"foo"`
						Bar int64 `spanner:"bar"`
					} `spanner:"col1"`
				}{
					Col1: []*struct {
						Foo int64 `spanner:"foo"`
						Bar int64 `spanner:"bar"`
					}{
						{
							Foo: 1,
							Bar: 2,
						},
					},
				}
				if !testEqual(want, s) {
					return fmt.Errorf("unexpected decoding result: %v, want %v", s, want)
				}
				return nil
			},
		},
	}
	for i, test := range tests {
		iter := client.Single().Query(ctx, test.q)
		defer iter.Stop()
		row, err := iter.Next()
		if err != nil {
			t.Errorf("%d: %v", i, err)
			continue
		}
		if err := test.want(row); err != nil {
			t.Errorf("%d: %v", i, err)
			continue
		}
	}
}

func TestIntegration_StructParametersUnsupported(t *testing.T) {
	skipEmulatorTest(t)
	t.Parallel()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()
	client, _, cleanup := prepareIntegrationTest(ctx, t, DefaultSessionPoolConfig, nil)
	defer cleanup()

	for _, test := range []struct {
		param       interface{}
		wantCode    codes.Code
		wantMsgPart string
	}{
		{
			struct {
				Field int
			}{10},
			codes.Unimplemented,
			"Unsupported query shape: " +
				"A struct value cannot be returned as a column value. " +
				"Rewrite the query to flatten the struct fields in the result.",
		},
		{
			[]struct {
				Field int
			}{{10}, {20}},
			codes.Unimplemented,
			"Unsupported query shape: " +
				"This query can return a null-valued array of struct, " +
				"which is not supported by Spanner.",
		},
	} {
		iter := client.Single().Query(ctx, Statement{
			SQL:    "SELECT @p",
			Params: map[string]interface{}{"p": test.param},
		})
		_, err := iter.Next()
		iter.Stop()
		if msg, ok := matchError(err, test.wantCode, test.wantMsgPart); !ok {
			t.Fatal(msg)
		}
	}
}

// Test queries of the form "SELECT expr".
func TestIntegration_QueryExpressions(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()
	client, _, cleanup := prepareIntegrationTest(ctx, t, DefaultSessionPoolConfig, nil)
	defer cleanup()

	newRow := func(vals []interface{}) *Row {
		row, err := NewRow(make([]string, len(vals)), vals)
		if err != nil {
			t.Fatal(err)
		}
		return row
	}

	tests := []struct {
		expr string
		want interface{}
	}{
		{"1", int64(1)},
		{"[1, 2, 3]", []NullInt64{{1, true}, {2, true}, {3, true}}},
		{"[1, NULL, 3]", []NullInt64{{1, true}, {0, false}, {3, true}}},
		{"IEEE_DIVIDE(1, 0)", math.Inf(1)},
		{"IEEE_DIVIDE(-1, 0)", math.Inf(-1)},
		{"IEEE_DIVIDE(0, 0)", math.NaN()},
		// TODO(jba): add IEEE_DIVIDE(0, 0) to the following array when we have a better equality predicate.
		{"[IEEE_DIVIDE(1, 0), IEEE_DIVIDE(-1, 0)]", []NullFloat64{{math.Inf(1), true}, {math.Inf(-1), true}}},
		{"ARRAY(SELECT AS STRUCT * FROM (SELECT 'a', 1) WHERE 0 = 1)", []NullRow{}},
		{"ARRAY(SELECT STRUCT(1, 2))", []NullRow{{Row: *newRow([]interface{}{1, 2}), Valid: true}}},
	}
	for _, test := range tests {
		iter := client.Single().Query(ctx, Statement{SQL: "SELECT " + test.expr})
		defer iter.Stop()
		row, err := iter.Next()
		if err != nil {
			t.Errorf("%q: %v", test.expr, err)
			continue
		}
		// Create new instance of type of test.want.
		gotp := reflect.New(reflect.TypeOf(test.want))
		if err := row.Column(0, gotp.Interface()); err != nil {
			t.Errorf("%q: Column returned error %v", test.expr, err)
			continue
		}
		got := reflect.Indirect(gotp).Interface()
		// TODO(jba): remove isNaN special case when we have a better equality predicate.
		if isNaN(got) && isNaN(test.want) {
			continue
		}
		if !testEqual(got, test.want) {
			t.Errorf("%q\n got  %#v\nwant %#v", test.expr, got, test.want)
		}
	}
}

func TestIntegration_QueryStats(t *testing.T) {
	skipEmulatorTest(t)
	t.Parallel()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()
	client, _, cleanup := prepareIntegrationTest(ctx, t, DefaultSessionPoolConfig, singerDBStatements)
	defer cleanup()

	accounts := []*Mutation{
		Insert("Accounts", []string{"AccountId", "Nickname", "Balance"}, []interface{}{int64(1), "Foo", int64(50)}),
		Insert("Accounts", []string{"AccountId", "Nickname", "Balance"}, []interface{}{int64(2), "Bar", int64(1)}),
	}
	if _, err := client.Apply(ctx, accounts, ApplyAtLeastOnce()); err != nil {
		t.Fatal(err)
	}
	const sql = "SELECT Balance FROM Accounts"

	qp, err := client.Single().AnalyzeQuery(ctx, Statement{sql, nil})
	if err != nil {
		t.Fatal(err)
	}
	if len(qp.PlanNodes) == 0 {
		t.Error("got zero plan nodes, expected at least one")
	}

	iter := client.Single().QueryWithStats(ctx, Statement{sql, nil})
	defer iter.Stop()
	for {
		_, err := iter.Next()
		if err == iterator.Done {
			break
		}
		if err != nil {
			t.Fatal(err)
		}
	}
	if iter.QueryPlan == nil {
		t.Error("got nil QueryPlan, expected one")
	}
	if iter.QueryStats == nil {
		t.Error("got nil QueryStats, expected some")
	}
}

func TestIntegration_InvalidDatabase(t *testing.T) {
	t.Parallel()

	if databaseAdmin == nil {
		t.Skip("Integration tests skipped")
	}
	ctx := context.Background()
	dbPath := fmt.Sprintf("projects/%v/instances/%v/databases/invalid", testProjectID, testInstanceID)
	c, err := createClient(ctx, dbPath, SessionPoolConfig{})
	// Client creation should succeed even if the database is invalid.
	if err != nil {
		t.Fatal(err)
	}
	_, err = c.Single().ReadRow(ctx, "TestTable", Key{1}, []string{"col1"})
	if msg, ok := matchError(err, codes.NotFound, ""); !ok {
		t.Fatal(msg)
	}
}

func TestIntegration_ReadErrors(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()
	client, _, cleanup := prepareIntegrationTest(ctx, t, DefaultSessionPoolConfig, readDBStatements)
	defer cleanup()

	var ms []*Mutation
	for i := 0; i < 2; i++ {
		ms = append(ms, InsertOrUpdate(testTable,
			testTableColumns,
			[]interface{}{fmt.Sprintf("k%d", i), fmt.Sprintf("v")}))
	}
	if _, err := client.Apply(ctx, ms); err != nil {
		t.Fatal(err)
	}

	// Read over invalid table fails
	_, err := client.Single().ReadRow(ctx, "badTable", Key{1}, []string{"StringValue"})
	if msg, ok := matchError(err, codes.NotFound, "badTable"); !ok {
		t.Error(msg)
	}
	// Read over invalid column fails
	_, err = client.Single().ReadRow(ctx, "TestTable", Key{1}, []string{"badcol"})
	if msg, ok := matchError(err, codes.NotFound, "badcol"); !ok {
		t.Error(msg)
	}

	// Invalid query fails
	iter := client.Single().Query(ctx, Statement{SQL: "SELECT Apples AND Oranges"})
	defer iter.Stop()
	_, err = iter.Next()
	if msg, ok := matchError(err, codes.InvalidArgument, "unrecognized name"); !ok {
		t.Error(msg)
	}

	// Read should fail on cancellation.
	cctx, cancel := context.WithCancel(ctx)
	cancel()
	_, err = client.Single().ReadRow(cctx, "TestTable", Key{1}, []string{"StringValue"})
	if msg, ok := matchError(err, codes.Canceled, ""); !ok {
		t.Error(msg)
	}
	// Read should fail if deadline exceeded.
	dctx, cancel := context.WithTimeout(ctx, time.Nanosecond)
	defer cancel()
	<-dctx.Done()
	_, err = client.Single().ReadRow(dctx, "TestTable", Key{1}, []string{"StringValue"})
	if msg, ok := matchError(err, codes.DeadlineExceeded, ""); !ok {
		t.Error(msg)
	}
	// Read should fail if there are multiple rows returned.
	_, err = client.Single().ReadRowUsingIndex(ctx, testTable, testTableIndex, Key{"v"}, testTableColumns)
	wantMsgPart := fmt.Sprintf("more than one row found by index(Table: %v, IndexKey: %v, Index: %v)", testTable, Key{"v"}, testTableIndex)
	if msg, ok := matchError(err, codes.FailedPrecondition, wantMsgPart); !ok {
		t.Error(msg)
	}
}

// Test TransactionRunner. Test that transactions are aborted and retried as
// expected.
func TestIntegration_TransactionRunner(t *testing.T) {
	skipEmulatorTest(t)
	t.Parallel()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()
	client, _, cleanup := prepareIntegrationTest(ctx, t, DefaultSessionPoolConfig, singerDBStatements)
	defer cleanup()

	// Test 1: User error should abort the transaction.
	_, _ = client.ReadWriteTransaction(ctx, func(ctx context.Context, tx *ReadWriteTransaction) error {
		tx.BufferWrite([]*Mutation{
			Insert("Accounts", []string{"AccountId", "Nickname", "Balance"}, []interface{}{int64(1), "Foo", int64(50)})})
		return errors.New("user error")
	})
	// Empty read.
	rows, err := readAllTestTable(client.Single().Read(ctx, "Accounts", Key{1}, []string{"AccountId", "Nickname", "Balance"}))
	if err != nil {
		t.Fatal(err)
	}
	if got, want := len(rows), 0; got != want {
		t.Errorf("Empty read, got %d, want %d.", got, want)
	}

	// Test 2: Expect abort and retry.
	// We run two ReadWriteTransactions concurrently and make txn1 abort txn2 by
	// committing writes to the column txn2 have read, and expect the following
	// read to abort and txn2 retries.

	// Set up two accounts
	accounts := []*Mutation{
		Insert("Accounts", []string{"AccountId", "Balance"}, []interface{}{int64(1), int64(0)}),
		Insert("Accounts", []string{"AccountId", "Balance"}, []interface{}{int64(2), int64(1)}),
	}
	if _, err := client.Apply(ctx, accounts, ApplyAtLeastOnce()); err != nil {
		t.Fatal(err)
	}

	var (
		cTxn1Start  = make(chan struct{})
		cTxn1Commit = make(chan struct{})
		cTxn2Start  = make(chan struct{})
		wg          sync.WaitGroup
	)

	// read balance, check error if we don't expect abort.
	readBalance := func(tx interface {
		ReadRow(ctx context.Context, table string, key Key, columns []string) (*Row, error)
	}, key int64, expectAbort bool) (int64, error) {
		var b int64
		r, e := tx.ReadRow(ctx, "Accounts", Key{int64(key)}, []string{"Balance"})
		if e != nil {
			if expectAbort && !isAbortedErr(e) {
				t.Errorf("ReadRow got %v, want Abort error.", e)
			}
			// Verify that we received and are able to extract retry info from
			// the aborted error.
			if _, hasRetryInfo := ExtractRetryDelay(e); !hasRetryInfo {
				t.Errorf("Got Abort error without RetryInfo\nGot: %v", e)
			}
			return b, e
		}
		if ce := r.Column(0, &b); ce != nil {
			return b, ce
		}
		return b, nil
	}

	wg.Add(2)
	// Txn 1
	go func() {
		defer wg.Done()
		var once sync.Once
		_, e := client.ReadWriteTransaction(ctx, func(ctx context.Context, tx *ReadWriteTransaction) error {
			b, e := readBalance(tx, 1, false)
			if e != nil {
				return e
			}
			// txn 1 can abort, in that case we skip closing the channel on
			// retry.
			once.Do(func() { close(cTxn1Start) })
			e = tx.BufferWrite([]*Mutation{
				Update("Accounts", []string{"AccountId", "Balance"}, []interface{}{int64(1), int64(b + 1)})})
			if e != nil {
				return e
			}
			// Wait for second transaction.
			<-cTxn2Start
			return nil
		})
		close(cTxn1Commit)
		if e != nil {
			t.Errorf("Transaction 1 commit, got %v, want nil.", e)
		}
	}()
	// Txn 2
	go func() {
		// Wait until txn 1 starts.
		<-cTxn1Start
		defer wg.Done()
		var (
			once sync.Once
			b1   int64
			b2   int64
			e    error
		)
		_, e = client.ReadWriteTransaction(ctx, func(ctx context.Context, tx *ReadWriteTransaction) error {
			if b1, e = readBalance(tx, 1, false); e != nil {
				return e
			}
			// Skip closing channel on retry.
			once.Do(func() { close(cTxn2Start) })
			// Wait until txn 1 successfully commits.
			<-cTxn1Commit
			// Txn1 has committed and written a balance to the account. Now this
			// transaction (txn2) reads and re-writes the balance. The first
			// time through, it will abort because it overlaps with txn1. Then
			// it will retry after txn1 commits, and succeed.
			if b2, e = readBalance(tx, 2, true); e != nil {
				return e
			}
			return tx.BufferWrite([]*Mutation{
				Update("Accounts", []string{"AccountId", "Balance"}, []interface{}{int64(2), int64(b1 + b2)})})
		})
		if e != nil {
			t.Errorf("Transaction 2 commit, got %v, want nil.", e)
		}
	}()
	wg.Wait()
	// Check that both transactions' effects are visible.
	for i := int64(1); i <= int64(2); i++ {
		if b, e := readBalance(client.Single(), i, false); e != nil {
			t.Fatalf("ReadBalance for key %d error %v.", i, e)
		} else if b != i {
			t.Errorf("Balance for key %d, got %d, want %d.", i, b, i)
		}
	}
}

// Test PartitionQuery of BatchReadOnlyTransaction, create partitions then
// serialize and deserialize both transaction and partition to be used in
// execution on another client, and compare results.
func TestIntegration_BatchQuery(t *testing.T) {
	t.Parallel()

	// Set up testing environment.
	var (
		client2 *Client
		err     error
	)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()
	client, dbPath, cleanup := prepareIntegrationTest(ctx, t, DefaultSessionPoolConfig, simpleDBStatements)
	defer cleanup()

	if err = populate(ctx, client); err != nil {
		t.Fatal(err)
	}
	if client2, err = createClient(ctx, dbPath, SessionPoolConfig{}); err != nil {
		t.Fatal(err)
	}
	defer client2.Close()

	// PartitionQuery
	var (
		txn        *BatchReadOnlyTransaction
		partitions []*Partition
		stmt       = Statement{SQL: "SELECT * FROM test;"}
	)

	if txn, err = client.BatchReadOnlyTransaction(ctx, StrongRead()); err != nil {
		t.Fatal(err)
	}
	defer txn.Cleanup(ctx)
	if partitions, err = txn.PartitionQuery(ctx, stmt, PartitionOptions{0, 3}); err != nil {
		t.Fatal(err)
	}

	// Reconstruct BatchReadOnlyTransactionID and execute partitions
	var (
		tid2      BatchReadOnlyTransactionID
		data      []byte
		gotResult bool // if we get matching result from two separate txns
	)
	if data, err = txn.ID.MarshalBinary(); err != nil {
		t.Fatalf("encoding failed %v", err)
	}
	if err = tid2.UnmarshalBinary(data); err != nil {
		t.Fatalf("decoding failed %v", err)
	}
	txn2 := client2.BatchReadOnlyTransactionFromID(tid2)

	// Execute Partitions and compare results
	for i, p := range partitions {
		iter := txn.Execute(ctx, p)
		defer iter.Stop()
		p2 := serdesPartition(t, i, p)
		iter2 := txn2.Execute(ctx, &p2)
		defer iter2.Stop()

		row1, err1 := iter.Next()
		row2, err2 := iter2.Next()
		if err1 != err2 {
			t.Fatalf("execution failed for different reasons: %v, %v", err1, err2)
			continue
		}
		if !testEqual(row1, row2) {
			t.Fatalf("execution returned different values: %v, %v", row1, row2)
			continue
		}
		if row1 == nil {
			continue
		}
		var a, b string
		if err = row1.Columns(&a, &b); err != nil {
			t.Fatalf("failed to parse row %v", err)
			continue
		}
		if a == str1 && b == str2 {
			gotResult = true
		}
	}
	if !gotResult {
		t.Fatalf("execution didn't return expected values")
	}
}

// Test PartitionRead of BatchReadOnlyTransaction, similar to TestBatchQuery
func TestIntegration_BatchRead(t *testing.T) {
	t.Parallel()

	// Set up testing environment.
	var (
		client2 *Client
		err     error
	)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()
	client, dbPath, cleanup := prepareIntegrationTest(ctx, t, DefaultSessionPoolConfig, simpleDBStatements)
	defer cleanup()

	if err = populate(ctx, client); err != nil {
		t.Fatal(err)
	}
	if client2, err = createClient(ctx, dbPath, SessionPoolConfig{}); err != nil {
		t.Fatal(err)
	}
	defer client2.Close()

	// PartitionRead
	var (
		txn        *BatchReadOnlyTransaction
		partitions []*Partition
	)

	if txn, err = client.BatchReadOnlyTransaction(ctx, StrongRead()); err != nil {
		t.Fatal(err)
	}
	defer txn.Cleanup(ctx)
	if partitions, err = txn.PartitionRead(ctx, "test", AllKeys(), simpleDBTableColumns, PartitionOptions{0, 3}); err != nil {
		t.Fatal(err)
	}

	// Reconstruct BatchReadOnlyTransactionID and execute partitions.
	var (
		tid2      BatchReadOnlyTransactionID
		data      []byte
		gotResult bool // if we get matching result from two separate txns
	)
	if data, err = txn.ID.MarshalBinary(); err != nil {
		t.Fatalf("encoding failed %v", err)
	}
	if err = tid2.UnmarshalBinary(data); err != nil {
		t.Fatalf("decoding failed %v", err)
	}
	txn2 := client2.BatchReadOnlyTransactionFromID(tid2)

	// Execute Partitions and compare results.
	for i, p := range partitions {
		iter := txn.Execute(ctx, p)
		defer iter.Stop()
		p2 := serdesPartition(t, i, p)
		iter2 := txn2.Execute(ctx, &p2)
		defer iter2.Stop()

		row1, err1 := iter.Next()
		row2, err2 := iter2.Next()
		if err1 != err2 {
			t.Fatalf("execution failed for different reasons: %v, %v", err1, err2)
			continue
		}
		if !testEqual(row1, row2) {
			t.Fatalf("execution returned different values: %v, %v", row1, row2)
			continue
		}
		if row1 == nil {
			continue
		}
		var a, b string
		if err = row1.Columns(&a, &b); err != nil {
			t.Fatalf("failed to parse row %v", err)
			continue
		}
		if a == str1 && b == str2 {
			gotResult = true
		}
	}
	if !gotResult {
		t.Fatalf("execution didn't return expected values")
	}
}

// Test normal txReadEnv method on BatchReadOnlyTransaction.
func TestIntegration_BROTNormal(t *testing.T) {
	t.Parallel()

	// Set up testing environment and create txn.
	var (
		txn *BatchReadOnlyTransaction
		err error
		row *Row
		i   int64
	)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()
	client, _, cleanup := prepareIntegrationTest(ctx, t, DefaultSessionPoolConfig, simpleDBStatements)
	defer cleanup()

	if txn, err = client.BatchReadOnlyTransaction(ctx, StrongRead()); err != nil {
		t.Fatal(err)
	}
	defer txn.Cleanup(ctx)
	if _, err := txn.PartitionRead(ctx, "test", AllKeys(), simpleDBTableColumns, PartitionOptions{0, 3}); err != nil {
		t.Fatal(err)
	}
	// Normal query should work with BatchReadOnlyTransaction.
	stmt2 := Statement{SQL: "SELECT 1"}
	iter := txn.Query(ctx, stmt2)
	defer iter.Stop()

	row, err = iter.Next()
	if err != nil {
		t.Errorf("query failed with %v", err)
	}
	if err = row.Columns(&i); err != nil {
		t.Errorf("failed to parse row %v", err)
	}
}

func TestIntegration_CommitTimestamp(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()
	client, _, cleanup := prepareIntegrationTest(ctx, t, DefaultSessionPoolConfig, ctsDBStatements)
	defer cleanup()

	type testTableRow struct {
		Key string
		Ts  NullTime
	}

	var (
		cts1, cts2, ts1, ts2 time.Time
		err                  error
	)

	// Apply mutation in sequence, expect to see commit timestamp in good order,
	// check also the commit timestamp returned
	for _, it := range []struct {
		k string
		t *time.Time
	}{
		{"a", &cts1},
		{"b", &cts2},
	} {
		tt := testTableRow{Key: it.k, Ts: NullTime{CommitTimestamp, true}}
		m, err := InsertStruct("TestTable", tt)
		if err != nil {
			t.Fatal(err)
		}
		*it.t, err = client.Apply(ctx, []*Mutation{m}, ApplyAtLeastOnce())
		if err != nil {
			t.Fatal(err)
		}
	}

	txn := client.ReadOnlyTransaction()
	for _, it := range []struct {
		k string
		t *time.Time
	}{
		{"a", &ts1},
		{"b", &ts2},
	} {
		if r, e := txn.ReadRow(ctx, "TestTable", Key{it.k}, []string{"Ts"}); e != nil {
			t.Fatal(err)
		} else {
			var got testTableRow
			if err := r.ToStruct(&got); err != nil {
				t.Fatal(err)
			}
			*it.t = got.Ts.Time
		}
	}
	if !cts1.Equal(ts1) {
		t.Errorf("Expect commit timestamp returned and read to match for txn1, got %v and %v.", cts1, ts1)
	}
	if !cts2.Equal(ts2) {
		t.Errorf("Expect commit timestamp returned and read to match for txn2, got %v and %v.", cts2, ts2)
	}

	// Try writing a timestamp in the future to commit timestamp, expect error.
	_, err = client.Apply(ctx, []*Mutation{InsertOrUpdate("TestTable", []string{"Key", "Ts"}, []interface{}{"a", time.Now().Add(time.Hour)})}, ApplyAtLeastOnce())
	if msg, ok := matchError(err, codes.FailedPrecondition, "Cannot write timestamps in the future"); !ok {
		t.Error(msg)
	}
}

func TestIntegration_DML(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()
	client, _, cleanup := prepareIntegrationTest(ctx, t, DefaultSessionPoolConfig, singerDBStatements)
	defer cleanup()

	// Function that reads a single row's first name from within a transaction.
	readFirstName := func(tx *ReadWriteTransaction, key int) (string, error) {
		row, err := tx.ReadRow(ctx, "Singers", Key{key}, []string{"FirstName"})
		if err != nil {
			return "", err
		}
		var fn string
		if err := row.Column(0, &fn); err != nil {
			return "", err
		}
		return fn, nil
	}

	// Function that reads multiple rows' first names from outside a read/write
	// transaction.
	readFirstNames := func(keys ...int) []string {
		var ks []KeySet
		for _, k := range keys {
			ks = append(ks, Key{k})
		}
		iter := client.Single().Read(ctx, "Singers", KeySets(ks...), []string{"FirstName"})
		var got []string
		var fn string
		err := iter.Do(func(row *Row) error {
			if err := row.Column(0, &fn); err != nil {
				return err
			}
			got = append(got, fn)
			return nil
		})
		if err != nil {
			t.Fatalf("readFirstNames(%v): %v", keys, err)
		}
		return got
	}

	// Use ReadWriteTransaction.Query to execute a DML statement.
	_, err := client.ReadWriteTransaction(ctx, func(ctx context.Context, tx *ReadWriteTransaction) error {
		iter := tx.Query(ctx, Statement{
			SQL: `INSERT INTO Singers (SingerId, FirstName, LastName) VALUES (1, "Umm", "Kulthum")`,
		})
		defer iter.Stop()
		row, err := iter.Next()
		if ErrCode(err) == codes.Aborted {
			return err
		}
		if err != iterator.Done {
			t.Fatalf("got results from iterator, want none: %#v, err = %v\n", row, err)
		}
		if iter.RowCount != 1 {
			t.Errorf("row count: got %d, want 1", iter.RowCount)
		}
		// The results of the DML statement should be visible to the transaction.
		got, err := readFirstName(tx, 1)
		if err != nil {
			return err
		}
		if want := "Umm"; got != want {
			t.Errorf("got %q, want %q", got, want)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}

	// Use ReadWriteTransaction.Update to execute a DML statement.
	_, err = client.ReadWriteTransaction(ctx, func(ctx context.Context, tx *ReadWriteTransaction) error {
		count, err := tx.Update(ctx, Statement{
			SQL: `Insert INTO Singers (SingerId, FirstName, LastName) VALUES (2, "Eduard", "Khil")`,
		})
		if err != nil {
			return err
		}
		if count != 1 {
			t.Errorf("row count: got %d, want 1", count)
		}
		got, err := readFirstName(tx, 2)
		if err != nil {
			return err
		}
		if want := "Eduard"; got != want {
			t.Errorf("got %q, want %q", got, want)
		}
		return nil

	})
	if err != nil {
		t.Fatal(err)
	}

	// Roll back a DML statement and confirm that it didn't happen.
	var fail = errors.New("fail")
	_, err = client.ReadWriteTransaction(ctx, func(ctx context.Context, tx *ReadWriteTransaction) error {
		_, err := tx.Update(ctx, Statement{
			SQL: `INSERT INTO Singers (SingerId, FirstName, LastName) VALUES (3, "Audra", "McDonald")`,
		})
		if err != nil {
			return err
		}
		return fail
	})
	if err != fail {
		t.Fatalf("rolling back: got error %v, want the error 'fail'", err)
	}
	_, err = client.Single().ReadRow(ctx, "Singers", Key{3}, []string{"FirstName"})
	if got, want := ErrCode(err), codes.NotFound; got != want {
		t.Errorf("got %s, want %s", got, want)
	}

	// Run two DML statements in the same transaction.
	_, err = client.ReadWriteTransaction(ctx, func(ctx context.Context, tx *ReadWriteTransaction) error {
		_, err := tx.Update(ctx, Statement{SQL: `UPDATE Singers SET FirstName = "Oum" WHERE SingerId = 1`})
		if err != nil {
			return err
		}
		_, err = tx.Update(ctx, Statement{SQL: `UPDATE Singers SET FirstName = "Eddie" WHERE SingerId = 2`})
		if err != nil {
			return err
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	got := readFirstNames(1, 2)
	want := []string{"Oum", "Eddie"}
	if !testEqual(got, want) {
		t.Errorf("got %v, want %v", got, want)
	}

	// Run a DML statement and an ordinary mutation in the same transaction.
	_, err = client.ReadWriteTransaction(ctx, func(ctx context.Context, tx *ReadWriteTransaction) error {
		_, err := tx.Update(ctx, Statement{
			SQL: `INSERT INTO Singers (SingerId, FirstName, LastName) VALUES (3, "Audra", "McDonald")`,
		})
		if err != nil {
			return err
		}
		return tx.BufferWrite([]*Mutation{
			Insert("Singers", []string{"SingerId", "FirstName", "LastName"},
				[]interface{}{4, "Andy", "Irvine"}),
		})
	})
	if err != nil {
		t.Fatal(err)
	}
	got = readFirstNames(3, 4)
	want = []string{"Audra", "Andy"}
	if !testEqual(got, want) {
		t.Errorf("got %v, want %v", got, want)
	}

	// Attempt to run a query using update.
	_, err = client.ReadWriteTransaction(ctx, func(ctx context.Context, tx *ReadWriteTransaction) error {
		_, err := tx.Update(ctx, Statement{SQL: `SELECT FirstName from Singers`})
		return err
	})
	if got, want := ErrCode(err), codes.InvalidArgument; got != want {
		t.Errorf("got %s, want %s", got, want)
	}
}

func TestIntegration_StructParametersBind(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()
	client, _, cleanup := prepareIntegrationTest(ctx, t, DefaultSessionPoolConfig, nil)
	defer cleanup()

	type tRow []interface{}
	type tRows []struct{ trow tRow }

	type allFields struct {
		Stringf string
		Intf    int
		Boolf   bool
		Floatf  float64
		Bytef   []byte
		Timef   time.Time
		Datef   civil.Date
	}
	allColumns := []string{
		"Stringf",
		"Intf",
		"Boolf",
		"Floatf",
		"Bytef",
		"Timef",
		"Datef",
	}
	s1 := allFields{"abc", 300, false, 3.45, []byte("foo"), t1, d1}
	s2 := allFields{"def", -300, false, -3.45, []byte("bar"), t2, d2}

	dynamicStructType := reflect.StructOf([]reflect.StructField{
		{Name: "A", Type: reflect.TypeOf(t1), Tag: `spanner:"ff1"`},
	})
	s3 := reflect.New(dynamicStructType)
	s3.Elem().Field(0).Set(reflect.ValueOf(t1))

	for i, test := range []struct {
		param interface{}
		sql   string
		cols  []string
		trows tRows
	}{
		// Struct value.
		{
			s1,
			"SELECT" +
				" @p.Stringf," +
				" @p.Intf," +
				" @p.Boolf," +
				" @p.Floatf," +
				" @p.Bytef," +
				" @p.Timef," +
				" @p.Datef",
			allColumns,
			tRows{
				{tRow{"abc", 300, false, 3.45, []byte("foo"), t1, d1}},
			},
		},
		// Array of struct value.
		{
			[]allFields{s1, s2},
			"SELECT * FROM UNNEST(@p)",
			allColumns,
			tRows{
				{tRow{"abc", 300, false, 3.45, []byte("foo"), t1, d1}},
				{tRow{"def", -300, false, -3.45, []byte("bar"), t2, d2}},
			},
		},
		// Null struct.
		{
			(*allFields)(nil),
			"SELECT @p IS NULL",
			[]string{""},
			tRows{
				{tRow{true}},
			},
		},
		// Null Array of struct.
		{
			[]allFields(nil),
			"SELECT @p IS NULL",
			[]string{""},
			tRows{
				{tRow{true}},
			},
		},
		// Empty struct.
		{
			struct{}{},
			"SELECT @p IS NULL ",
			[]string{""},
			tRows{
				{tRow{false}},
			},
		},
		// Empty array of struct.
		{
			[]allFields{},
			"SELECT * FROM UNNEST(@p) ",
			allColumns,
			tRows{},
		},
		// Struct with duplicate fields.
		{
			struct {
				A int `spanner:"field"`
				B int `spanner:"field"`
			}{10, 20},
			"SELECT * FROM UNNEST([@p]) ",
			[]string{"field", "field"},
			tRows{
				{tRow{10, 20}},
			},
		},
		// Struct with unnamed fields.
		{
			struct {
				A string `spanner:""`
			}{"hello"},
			"SELECT * FROM UNNEST([@p]) ",
			[]string{""},
			tRows{
				{tRow{"hello"}},
			},
		},
		// Mixed struct.
		{
			struct {
				DynamicStructField interface{}  `spanner:"f1"`
				ArrayStructField   []*allFields `spanner:"f2"`
			}{
				DynamicStructField: s3.Interface(),
				ArrayStructField:   []*allFields{nil},
			},
			"SELECT @p.f1.ff1, ARRAY_LENGTH(@p.f2), @p.f2[OFFSET(0)] IS NULL ",
			[]string{"ff1", "", ""},
			tRows{
				{tRow{t1, 1, true}},
			},
		},
	} {
		iter := client.Single().Query(ctx, Statement{
			SQL:    test.sql,
			Params: map[string]interface{}{"p": test.param},
		})
		var gotRows []*Row
		err := iter.Do(func(r *Row) error {
			gotRows = append(gotRows, r)
			return nil
		})
		if err != nil {
			t.Errorf("Failed to execute test case %d, error: %v", i, err)
		}

		var wantRows []*Row
		for j, row := range test.trows {
			r, err := NewRow(test.cols, row.trow)
			if err != nil {
				t.Errorf("Invalid row %d in test case %d", j, i)
			}
			wantRows = append(wantRows, r)
		}
		if !testEqual(gotRows, wantRows) {
			t.Errorf("%d: Want result %v, got result %v", i, wantRows, gotRows)
		}
	}
}

func TestIntegration_PDML(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()
	client, _, cleanup := prepareIntegrationTest(ctx, t, DefaultSessionPoolConfig, singerDBStatements)
	defer cleanup()

	columns := []string{"SingerId", "FirstName", "LastName"}

	// Populate the Singers table.
	var muts []*Mutation
	for _, row := range [][]interface{}{
		{1, "Umm", "Kulthum"},
		{2, "Eduard", "Khil"},
		{3, "Audra", "McDonald"},
		{4, "Enrique", "Iglesias"},
		{5, "Shakira", "Ripoll"},
	} {
		muts = append(muts, Insert("Singers", columns, row))
	}
	if _, err := client.Apply(ctx, muts); err != nil {
		t.Fatal(err)
	}
	// Identifiers in PDML statements must be fully qualified.
	// TODO(jba): revisit the above.
	count, err := client.PartitionedUpdate(ctx, Statement{
		SQL: `UPDATE Singers SET Singers.FirstName = "changed" WHERE Singers.SingerId >= 1 AND Singers.SingerId <= @end`,
		Params: map[string]interface{}{
			"end": 3,
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if want := int64(3); count != want {
		t.Fatalf("got %d, want %d", count, want)
	}
	got, err := readAll(client.Single().Read(ctx, "Singers", AllKeys(), columns))
	if err != nil {
		t.Fatal(err)
	}
	want := [][]interface{}{
		{int64(1), "changed", "Kulthum"},
		{int64(2), "changed", "Khil"},
		{int64(3), "changed", "McDonald"},
		{int64(4), "Enrique", "Iglesias"},
		{int64(5), "Shakira", "Ripoll"},
	}
	if !testEqual(got, want) {
		t.Errorf("\ngot %v\nwant%v", got, want)
	}
}

func TestIntegration_BatchDML(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()
	client, _, cleanup := prepareIntegrationTest(ctx, t, DefaultSessionPoolConfig, singerDBStatements)
	defer cleanup()

	columns := []string{"SingerId", "FirstName", "LastName"}

	// Populate the Singers table.
	var muts []*Mutation
	for _, row := range [][]interface{}{
		{1, "Umm", "Kulthum"},
		{2, "Eduard", "Khil"},
		{3, "Audra", "McDonald"},
	} {
		muts = append(muts, Insert("Singers", columns, row))
	}
	if _, err := client.Apply(ctx, muts); err != nil {
		t.Fatal(err)
	}

	var counts []int64
	_, err := client.ReadWriteTransaction(ctx, func(ctx context.Context, tx *ReadWriteTransaction) (err error) {
		counts, err = tx.BatchUpdate(ctx, []Statement{
			{SQL: `UPDATE Singers SET Singers.FirstName = "changed 1" WHERE Singers.SingerId = 1`},
			{SQL: `UPDATE Singers SET Singers.FirstName = "changed 2" WHERE Singers.SingerId = 2`},
			{SQL: `UPDATE Singers SET Singers.FirstName = "changed 3" WHERE Singers.SingerId = 3`},
		})
		return err
	})

	if err != nil {
		t.Fatal(err)
	}
	if want := []int64{1, 1, 1}; !testEqual(counts, want) {
		t.Fatalf("got %d, want %d", counts, want)
	}
	got, err := readAll(client.Single().Read(ctx, "Singers", AllKeys(), columns))
	if err != nil {
		t.Fatal(err)
	}
	want := [][]interface{}{
		{int64(1), "changed 1", "Kulthum"},
		{int64(2), "changed 2", "Khil"},
		{int64(3), "changed 3", "McDonald"},
	}
	if !testEqual(got, want) {
		t.Errorf("\ngot %v\nwant%v", got, want)
	}
}

func TestIntegration_BatchDML_NoStatements(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()
	client, _, cleanup := prepareIntegrationTest(ctx, t, DefaultSessionPoolConfig, singerDBStatements)
	defer cleanup()

	_, err := client.ReadWriteTransaction(ctx, func(ctx context.Context, tx *ReadWriteTransaction) (err error) {
		_, err = tx.BatchUpdate(ctx, []Statement{})
		return err
	})
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if s, ok := status.FromError(err); ok {
		if s.Code() != codes.InvalidArgument {
			t.Fatalf("expected InvalidArgument, got %v", err)
		}
	} else {
		t.Fatalf("expected InvalidArgument, got %v", err)
	}
}

func TestIntegration_BatchDML_TwoStatements(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()
	client, _, cleanup := prepareIntegrationTest(ctx, t, DefaultSessionPoolConfig, singerDBStatements)
	defer cleanup()

	columns := []string{"SingerId", "FirstName", "LastName"}

	// Populate the Singers table.
	var muts []*Mutation
	for _, row := range [][]interface{}{
		{1, "Umm", "Kulthum"},
		{2, "Eduard", "Khil"},
		{3, "Audra", "McDonald"},
	} {
		muts = append(muts, Insert("Singers", columns, row))
	}
	if _, err := client.Apply(ctx, muts); err != nil {
		t.Fatal(err)
	}

	var updateCount int64
	var batchCounts []int64
	_, err := client.ReadWriteTransaction(ctx, func(ctx context.Context, tx *ReadWriteTransaction) (err error) {
		batchCounts, err = tx.BatchUpdate(ctx, []Statement{
			{SQL: `UPDATE Singers SET Singers.FirstName = "changed 1" WHERE Singers.SingerId = 1`},
			{SQL: `UPDATE Singers SET Singers.FirstName = "changed 2" WHERE Singers.SingerId = 2`},
			{SQL: `UPDATE Singers SET Singers.FirstName = "changed 3" WHERE Singers.SingerId = 3`},
		})
		if err != nil {
			return err
		}

		updateCount, err = tx.Update(ctx, Statement{SQL: `UPDATE Singers SET Singers.FirstName = "changed 1" WHERE Singers.SingerId = 1`})
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	if want := []int64{1, 1, 1}; !testEqual(batchCounts, want) {
		t.Fatalf("got %d, want %d", batchCounts, want)
	}
	if updateCount != 1 {
		t.Fatalf("got %v, want 1", updateCount)
	}
}

func TestIntegration_BatchDML_Error(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()
	client, _, cleanup := prepareIntegrationTest(ctx, t, DefaultSessionPoolConfig, singerDBStatements)
	defer cleanup()

	columns := []string{"SingerId", "FirstName", "LastName"}

	// Populate the Singers table.
	var muts []*Mutation
	for _, row := range [][]interface{}{
		{1, "Umm", "Kulthum"},
		{2, "Eduard", "Khil"},
		{3, "Audra", "McDonald"},
	} {
		muts = append(muts, Insert("Singers", columns, row))
	}
	if _, err := client.Apply(ctx, muts); err != nil {
		t.Fatal(err)
	}

	_, err := client.ReadWriteTransaction(ctx, func(ctx context.Context, tx *ReadWriteTransaction) (err error) {
		counts, err := tx.BatchUpdate(ctx, []Statement{
			{SQL: `UPDATE Singers SET Singers.FirstName = "changed 1" WHERE Singers.SingerId = 1`},
			{SQL: `some illegal statement`},
			{SQL: `UPDATE Singers SET Singers.FirstName = "changed 3" WHERE Singers.SingerId = 3`},
		})
		if err == nil {
			t.Fatal("expected err, got nil")
		}
		// The statement may also have been aborted by Spanner, and in that
		// case we should just let the client library retry the transaction.
		if status.Code(err) == codes.Aborted {
			return err
		}
		if want := []int64{1}; !testEqual(counts, want) {
			t.Fatalf("got %d, want %d", counts, want)
		}

		got, err := readAll(tx.Read(ctx, "Singers", AllKeys(), columns))
		// Aborted error is ok, just retry the transaction.
		if status.Code(err) == codes.Aborted {
			return err
		}
		if err != nil {
			t.Fatal(err)
		}
		want := [][]interface{}{
			{int64(1), "changed 1", "Kulthum"},
			{int64(2), "Eduard", "Khil"},
			{int64(3), "Audra", "McDonald"},
		}
		if !testEqual(got, want) {
			t.Errorf("\ngot %v\nwant%v", got, want)
		}

		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

func TestIntegration_StartBackupOperation(t *testing.T) {
	skipEmulatorTest(t)
	t.Parallel()

	// Backups can be slow, so use a 30 minute timeout.
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Minute)
	defer cancel()
	_, testDatabaseName, cleanup := prepareIntegrationTest(ctx, t, DefaultSessionPoolConfig, backuDBStatements)
	defer cleanup()

	backupID := backupIDSpace.New()
	backupName := fmt.Sprintf("projects/%s/instances/%s/backups/%s", testProjectID, testInstanceID, backupID)
	// Minimum expiry time is 6 hours
	expires := time.Now().Add(time.Hour * 7)
	respLRO, err := databaseAdmin.StartBackupOperation(ctx, backupID, testDatabaseName, expires)
	if err != nil {
		t.Fatal(err)
	}

	_, err = respLRO.Wait(ctx)
	if err != nil {
		t.Fatal(err)
	}
	respMetadata, err := respLRO.Metadata()
	if err != nil {
		t.Fatalf("backup response metadata, got error %v, want nil", err)
	}
	if respMetadata.Database != testDatabaseName {
		t.Fatalf("backup database name, got %s, want %s", respMetadata.Database, testDatabaseName)
	}
	if respMetadata.Progress.ProgressPercent != 100 {
		t.Fatalf("backup progress percent, got %d, want 100", respMetadata.Progress.ProgressPercent)
	}
	respCheck, err := databaseAdmin.GetBackup(ctx, &adminpb.GetBackupRequest{Name: backupName})
	if err != nil {
		t.Fatalf("backup metadata, got error %v, want nil", err)
	}
	if respCheck.CreateTime == nil {
		t.Fatal("backup create time, got nil, want non-nil")
	}
	if respCheck.State != adminpb.Backup_READY {
		t.Fatalf("backup state, got %v, want %v", respCheck.State, adminpb.Backup_READY)
	}
	if respCheck.SizeBytes == 0 {
		t.Fatalf("backup size, got %d, want non-zero", respCheck.SizeBytes)
	}
}

// TestIntegration_DirectPathFallback tests the CFE fallback when the directpath net is blackholed.
func TestIntegration_DirectPathFallback(t *testing.T) {
	t.Parallel()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()
	// Set up testing environment.
	client, _, cleanup := prepareIntegrationTest(ctx, t, DefaultSessionPoolConfig, readDBStatements)
	defer cleanup()

	if !dpConfig.attemptDirectPath {
		return
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

	// Precondition: wait for DirectPath to connect.
	countEnough := examineTraffic(ctx, client /*blackholeDP = */, false)
	if !countEnough {
		t.Fatalf("Failed to observe RPCs over DirectPath")
	}

	// Enable the blackhole, which will prevent communication with grpclb and thus DirectPath.
	blackholeOrAllowDirectPath(t /*blackholeDP = */, true)
	countEnough = examineTraffic(ctx, client /*blackholeDP = */, true)
	if !countEnough {
		t.Fatalf("Failed to fallback to CFE after blackhole DirectPath")
	}

	// Disable the blackhole, and client should use DirectPath again.
	blackholeOrAllowDirectPath(t /*blackholeDP = */, false)
	countEnough = examineTraffic(ctx, client /*blackholeDP = */, false)
	if !countEnough {
		t.Fatalf("Failed to fallback to CFE after blackhole DirectPath")
	}
}

// Prepare initializes Cloud Spanner testing DB and clients.
func prepareIntegrationTest(ctx context.Context, t *testing.T, spc SessionPoolConfig, statements []string) (*Client, string, func()) {
	if databaseAdmin == nil {
		t.Skip("Integration tests skipped")
	}
	// Construct a unique test DB name.
	dbName := dbNameSpace.New()

	dbPath := fmt.Sprintf("projects/%v/instances/%v/databases/%v", testProjectID, testInstanceID, dbName)
	// Create database and tables.
	op, err := databaseAdmin.CreateDatabaseWithRetry(ctx, &adminpb.CreateDatabaseRequest{
		Parent:          fmt.Sprintf("projects/%v/instances/%v", testProjectID, testInstanceID),
		CreateStatement: "CREATE DATABASE " + dbName,
		ExtraStatements: statements,
	})
	if err != nil {
		t.Fatalf("cannot create testing DB %v: %v", dbPath, err)
	}
	if _, err := op.Wait(ctx); err != nil {
		t.Fatalf("cannot create testing DB %v: %v", dbPath, err)
	}
	client, err := createClient(ctx, dbPath, spc)
	if err != nil {
		t.Fatalf("cannot create data client on DB %v: %v", dbPath, err)
	}
	return client, dbPath, func() {
		client.Close()
	}
}

func cleanupInstances() {
	if instanceAdmin == nil {
		// Integration tests skipped.
		return
	}

	ctx := context.Background()
	parent := fmt.Sprintf("projects/%v", testProjectID)
	iter := instanceAdmin.ListInstances(ctx, &instancepb.ListInstancesRequest{
		Parent: parent,
		Filter: "name:gotest-",
	})
	expireAge := 24 * time.Hour

	for {
		inst, err := iter.Next()
		if err == iterator.Done {
			break
		}
		if err != nil {
			panic(err)
		}

		_, instID, err := parseInstanceName(inst.Name)
		if err != nil {
			log.Printf("Error: %v\n", err)
			continue
		}
		if instanceNameSpace.Older(instID, expireAge) {
			log.Printf("Deleting instance %s", inst.Name)
			deleteInstanceAndBackups(ctx, inst.Name)
		}
	}
}

func deleteInstanceAndBackups(ctx context.Context, instanceName string) {
	// First delete any lingering backups that might have been left on
	// the instance.
	backups := databaseAdmin.ListBackups(ctx, &adminpb.ListBackupsRequest{Parent: instanceName})
	for {
		backup, err := backups.Next()
		if err == iterator.Done {
			break
		}
		if err != nil {
			log.Printf("failed to retrieve backups from instance %s because of error %v", instanceName, err)
			break
		}
		if err := databaseAdmin.DeleteBackup(ctx, &adminpb.DeleteBackupRequest{Name: backup.Name}); err != nil {
			log.Printf("failed to delete backup %s (error %v)", backup.Name, err)
		}
	}

	if err := instanceAdmin.DeleteInstance(ctx, &instancepb.DeleteInstanceRequest{Name: instanceName}); err != nil {
		log.Printf("failed to delete instance %s (error %v), might need a manual removal",
			instanceName, err)
	}
}

func rangeReads(ctx context.Context, t *testing.T, client *Client) {
	checkRange := func(ks KeySet, wantNums ...int) {
		if msg, ok := compareRows(client.Single().Read(ctx, testTable, ks, testTableColumns), wantNums); !ok {
			t.Errorf("key set %+v: %s", ks, msg)
		}
	}

	checkRange(Key{"k1"}, 1)
	checkRange(KeyRange{Key{"k3"}, Key{"k5"}, ClosedOpen}, 3, 4)
	checkRange(KeyRange{Key{"k3"}, Key{"k5"}, ClosedClosed}, 3, 4, 5)
	checkRange(KeyRange{Key{"k3"}, Key{"k5"}, OpenClosed}, 4, 5)
	checkRange(KeyRange{Key{"k3"}, Key{"k5"}, OpenOpen}, 4)

	// Partial key specification.
	checkRange(KeyRange{Key{"k7"}, Key{}, ClosedClosed}, 7, 8, 9)
	checkRange(KeyRange{Key{"k7"}, Key{}, OpenClosed}, 8, 9)
	checkRange(KeyRange{Key{}, Key{"k11"}, ClosedOpen}, 0, 1, 10)
	checkRange(KeyRange{Key{}, Key{"k11"}, ClosedClosed}, 0, 1, 10, 11)

	// The following produce empty ranges.
	// TODO(jba): Consider a multi-part key to illustrate partial key behavior.
	// checkRange(KeyRange{Key{"k7"}, Key{}, ClosedOpen})
	// checkRange(KeyRange{Key{"k7"}, Key{}, OpenOpen})
	// checkRange(KeyRange{Key{}, Key{"k11"}, OpenOpen})
	// checkRange(KeyRange{Key{}, Key{"k11"}, OpenClosed})

	// Prefix is component-wise, not string prefix.
	checkRange(Key{"k1"}.AsPrefix(), 1)
	checkRange(KeyRange{Key{"k1"}, Key{"k2"}, ClosedOpen}, 1, 10, 11, 12, 13, 14)

	checkRange(AllKeys(), 0, 1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14)
}

func indexRangeReads(ctx context.Context, t *testing.T, client *Client) {
	checkRange := func(ks KeySet, wantNums ...int) {
		if msg, ok := compareRows(client.Single().ReadUsingIndex(ctx, testTable, testTableIndex, ks, testTableColumns),
			wantNums); !ok {
			t.Errorf("key set %+v: %s", ks, msg)
		}
	}

	checkRange(Key{"v1"}, 1)
	checkRange(KeyRange{Key{"v3"}, Key{"v5"}, ClosedOpen}, 3, 4)
	checkRange(KeyRange{Key{"v3"}, Key{"v5"}, ClosedClosed}, 3, 4, 5)
	checkRange(KeyRange{Key{"v3"}, Key{"v5"}, OpenClosed}, 4, 5)
	checkRange(KeyRange{Key{"v3"}, Key{"v5"}, OpenOpen}, 4)

	// // Partial key specification.
	checkRange(KeyRange{Key{"v7"}, Key{}, ClosedClosed}, 7, 8, 9)
	checkRange(KeyRange{Key{"v7"}, Key{}, OpenClosed}, 8, 9)
	checkRange(KeyRange{Key{}, Key{"v11"}, ClosedOpen}, 0, 1, 10)
	checkRange(KeyRange{Key{}, Key{"v11"}, ClosedClosed}, 0, 1, 10, 11)

	// // The following produce empty ranges.
	// checkRange(KeyRange{Key{"v7"}, Key{}, ClosedOpen})
	// checkRange(KeyRange{Key{"v7"}, Key{}, OpenOpen})
	// checkRange(KeyRange{Key{}, Key{"v11"}, OpenOpen})
	// checkRange(KeyRange{Key{}, Key{"v11"}, OpenClosed})

	// // Prefix is component-wise, not string prefix.
	checkRange(Key{"v1"}.AsPrefix(), 1)
	checkRange(KeyRange{Key{"v1"}, Key{"v2"}, ClosedOpen}, 1, 10, 11, 12, 13, 14)
	checkRange(AllKeys(), 0, 1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14)

	// Read from an index with DESC ordering.
	wantNums := []int{14, 13, 12, 11, 10, 9, 8, 7, 6, 5, 4, 3, 2, 1, 0}
	if msg, ok := compareRows(client.Single().ReadUsingIndex(ctx, testTable, "TestTableByValueDesc", AllKeys(), testTableColumns),
		wantNums); !ok {
		t.Errorf("desc: %s", msg)
	}
}

type testTableRow struct{ Key, StringValue string }

func compareRows(iter *RowIterator, wantNums []int) (string, bool) {
	rows, err := readAllTestTable(iter)
	if err != nil {
		return err.Error(), false
	}
	want := map[string]string{}
	for _, n := range wantNums {
		want[fmt.Sprintf("k%d", n)] = fmt.Sprintf("v%d", n)
	}
	got := map[string]string{}
	for _, r := range rows {
		got[r.Key] = r.StringValue
	}
	if !testEqual(got, want) {
		return fmt.Sprintf("got %v, want %v", got, want), false
	}
	return "", true
}

func isNaN(x interface{}) bool {
	f, ok := x.(float64)
	if !ok {
		return false
	}
	return math.IsNaN(f)
}

// createClient creates Cloud Spanner data client.
func createClient(ctx context.Context, dbPath string, spc SessionPoolConfig) (client *Client, err error) {
	opts := grpcHeaderChecker.CallOptions()
	if spannerHost != "" {
		opts = append(opts, option.WithEndpoint(spannerHost))
	}
	if dpConfig.attemptDirectPath {
		// TODO(mohanli): Move EnableDirectPath internal option to client.go when DirectPath is ready for public beta.
		opts = append(opts, internaloption.EnableDirectPath(true))
		opts = append(opts, option.WithGRPCDialOption(grpc.WithDefaultCallOptions(grpc.Peer(peerInfo))))
	}
	client, err = NewClientWithConfig(ctx, dbPath, ClientConfig{SessionPoolConfig: spc}, opts...)
	if err != nil {
		return nil, fmt.Errorf("cannot create data client on DB %v: %v", dbPath, err)
	}
	return client, nil
}

// populate prepares the database with some data.
func populate(ctx context.Context, client *Client) error {
	// Populate data
	var err error
	m := InsertMap("test", map[string]interface{}{
		"a": str1,
		"b": str2,
	})
	_, err = client.Apply(ctx, []*Mutation{m})
	return err
}

func matchError(got error, wantCode codes.Code, wantMsgPart string) (string, bool) {
	if ErrCode(got) != wantCode || !strings.Contains(strings.ToLower(ErrDesc(got)), strings.ToLower(wantMsgPart)) {
		return fmt.Sprintf("got error <%v>\n"+`want <code = %q, "...%s...">`, got, wantCode, wantMsgPart), false
	}
	return "", true
}

func rowToValues(r *Row) ([]interface{}, error) {
	var x int64
	var y, z string
	if err := r.Column(0, &x); err != nil {
		return nil, err
	}
	if err := r.Column(1, &y); err != nil {
		return nil, err
	}
	if err := r.Column(2, &z); err != nil {
		return nil, err
	}
	return []interface{}{x, y, z}, nil
}

func readAll(iter *RowIterator) ([][]interface{}, error) {
	defer iter.Stop()
	var vals [][]interface{}
	for {
		row, err := iter.Next()
		if err == iterator.Done {
			return vals, nil
		}
		if err != nil {
			return nil, err
		}
		v, err := rowToValues(row)
		if err != nil {
			return nil, err
		}
		vals = append(vals, v)
	}
}

func readAllTestTable(iter *RowIterator) ([]testTableRow, error) {
	defer iter.Stop()
	var vals []testTableRow
	for {
		row, err := iter.Next()
		if err == iterator.Done {
			if iter.Metadata == nil {
				// All queries should always return metadata, regardless whether
				// they return any rows or not.
				return nil, errors.New("missing metadata from query")
			}
			return vals, nil
		}
		if err != nil {
			return nil, err
		}
		var ttr testTableRow
		if err := row.ToStruct(&ttr); err != nil {
			return nil, err
		}
		vals = append(vals, ttr)
	}
}

func readAllAccountsTable(iter *RowIterator) ([][]interface{}, error) {
	defer iter.Stop()
	var vals [][]interface{}
	for {
		row, err := iter.Next()
		if err == iterator.Done {
			return vals, nil
		}
		if err != nil {
			return nil, err
		}
		var id, balance int64
		var nickname string
		err = row.Columns(&id, &nickname, &balance)
		if err != nil {
			return nil, err
		}
		vals = append(vals, []interface{}{id, nickname, balance})
	}
}

func maxDuration(a, b time.Duration) time.Duration {
	if a > b {
		return a
	}
	return b
}

func isEmulatorEnvSet() bool {
	return os.Getenv("SPANNER_EMULATOR_HOST") != ""
}

func skipEmulatorTest(t *testing.T) {
	if isEmulatorEnvSet() {
		t.Skip("Skipping testing against the emulator.")
	}
}

func verifyDirectPathRemoteAddress(t *testing.T) {
	t.Helper()
	if !dpConfig.attemptDirectPath {
		return
	}
	if remoteIP, res := isDirectPathRemoteAddress(); !res {
		if dpConfig.directPathIPv4Only {
			t.Fatalf("Expect to access DirectPath via ipv4 only, but RPC was destined to %s", remoteIP)
		} else {
			t.Fatalf("Expect to access DirectPath via ipv4 or ipv6, but RPC was destined to %s", remoteIP)
		}
	}
}

func isDirectPathRemoteAddress() (_ string, _ bool) {
	remoteIP := peerInfo.Addr.String()
	// DirectPath ipv4-only can only use ipv4 traffic.
	if dpConfig.directPathIPv4Only {
		return remoteIP, strings.HasPrefix(remoteIP, directPathIPV4Prefix)
	}
	// DirectPath ipv6 can use either ipv4 or ipv6 traffic.
	return remoteIP, strings.HasPrefix(remoteIP, directPathIPV4Prefix) || strings.HasPrefix(remoteIP, directPathIPV6Prefix)
}

// examineTraffic counts RPCs use DirectPath or CFE traffic.
func examineTraffic(ctx context.Context, client *Client, expectDP bool) bool {
	var numCount uint64
	const (
		numRPCsToSend  = 20
		minCompleteRPC = 40
	)
	countEnough := false
	start := time.Now()
	for !countEnough && time.Since(start) < 2*time.Minute {
		for i := 0; i < numRPCsToSend; i++ {
			_, _ = readAllTestTable(client.Single().Read(ctx, testTable, Key{"k1"}, testTableColumns))
			if _, useDP := isDirectPathRemoteAddress(); useDP != expectDP {
				numCount++
			}
			time.Sleep(100 * time.Millisecond)
			countEnough = numCount >= minCompleteRPC
		}
	}
	return countEnough
}

func blackholeOrAllowDirectPath(t *testing.T, blackholeDP bool) {
	if dpConfig.directPathIPv4Only {
		if blackholeDP {
			cmdRes := exec.Command("bash", "-c", blackholeDpv4Cmd)
			out, _ := cmdRes.CombinedOutput()
			t.Logf(string(out))
		} else {
			cmdRes := exec.Command("bash", "-c", allowDpv4Cmd)
			out, _ := cmdRes.CombinedOutput()
			t.Logf(string(out))
		}
		return
	}
	// DirectPath supports both ipv4 and ipv6
	if blackholeDP {
		cmdRes := exec.Command("bash", "-c", blackholeDpv4Cmd)
		out, _ := cmdRes.CombinedOutput()
		t.Logf(string(out))
		cmdRes = exec.Command("bash", "-c", blackholeDpv6Cmd)
		out, _ = cmdRes.CombinedOutput()
		t.Logf(string(out))
	} else {
		cmdRes := exec.Command("bash", "-c", allowDpv4Cmd)
		out, _ := cmdRes.CombinedOutput()
		t.Logf(string(out))
		cmdRes = exec.Command("bash", "-c", allowDpv6Cmd)
		out, _ = cmdRes.CombinedOutput()
		t.Logf(string(out))
	}
}

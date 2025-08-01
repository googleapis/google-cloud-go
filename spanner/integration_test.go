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
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"log"
	"math"
	"math/big"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"cloud.google.com/go/civil"
	"cloud.google.com/go/internal/testutil"
	"cloud.google.com/go/internal/uid"
	database "cloud.google.com/go/spanner/admin/database/apiv1"
	adminpb "cloud.google.com/go/spanner/admin/database/apiv1/databasepb"
	instance "cloud.google.com/go/spanner/admin/instance/apiv1"
	"cloud.google.com/go/spanner/admin/instance/apiv1/instancepb"
	v1 "cloud.google.com/go/spanner/apiv1"
	sppb "cloud.google.com/go/spanner/apiv1/spannerpb"
	"cloud.google.com/go/spanner/internal"
	stestutil "cloud.google.com/go/spanner/internal/testutil"
	pb "cloud.google.com/go/spanner/testdata/protos"
	"github.com/google/go-cmp/cmp"
	"github.com/google/uuid"
	"go.opencensus.io/stats/view"
	"go.opencensus.io/tag"
	"google.golang.org/api/iterator"
	"google.golang.org/api/option"
	"google.golang.org/api/option/internaloption"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/peer"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/testing/protocmp"
)

const (
	singerDDLStatements               = "SINGER_DDL_STATEMENTS"
	simpleDDLStatements               = "SIMPLE_DDL_STATEMENTS"
	readDDLStatements                 = "READ_DDL_STATEMENTS"
	backupDDLStatements               = "BACKUP_DDL_STATEMENTS"
	testTableDDLStatements            = "TEST_TABLE_DDL_STATEMENTS"
	fkdcDDLStatements                 = "FKDC_DDL_STATEMENTS"
	intervalDDLStatements             = "INTERVAL_DDL_STATEMENTS"
	testTableBitReversedSeqStatements = "TEST_TABLE_BIT_REVERSED_SEQUENCE_STATEMENTS"
)

var (
	// testProjectID specifies the project used for testing. It can be changed
	// by setting environment variable GCLOUD_TESTS_GOLANG_PROJECT_ID.
	testProjectID = testutil.ProjID()

	// testDialect specifies the dialect used for testing.
	testDialect adminpb.DatabaseDialect

	// spannerHost specifies the spanner API host used for testing. It can be changed
	// by setting the environment variable GCLOUD_TESTS_GOLANG_SPANNER_HOST
	spannerHost = getSpannerHost()

	// instanceConfig specifies the instance config used to create an instance for testing.
	// It can be changed by setting the environment variable
	// GCLOUD_TESTS_GOLANG_SPANNER_INSTANCE_CONFIG.
	instanceConfig = getInstanceConfig()

	isMultiplexEnabled = getMultiplexEnableFlag()

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

	singerDBPGStatements = []string{
		`CREATE TABLE Singers (
				SingerId INT8 NOT NULL,
				FirstName VARCHAR(1024),
				LastName  VARCHAR(1024),
				SingerInfo	BYTEA,
				numeric NUMERIC,
				float8 FLOAT8,
				PRIMARY KEY(SingerId)
			)`,
		`CREATE INDEX SingerByName ON Singers(FirstName, LastName)`,
		`CREATE TABLE Accounts (
				AccountId BIGINT NOT NULL,
				Nickname  VARCHAR(100),
				Balance   BIGINT NOT NULL,
				PRIMARY KEY(AccountId)
			)`,
		`CREATE INDEX AccountByNickname ON Accounts(Nickname)`,
		`CREATE TABLE Types (
			RowID		BIGINT PRIMARY KEY,
			String		VARCHAR,
			Bytes		BYTEA,
			Int64a		BIGINT,
			Bool		BOOL,
			Float64		DOUBLE PRECISION,
			Numeric		NUMERIC,
			JSONB		jsonb
		)`,
	}

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
				Numeric		NUMERIC,
				NumericArray	ARRAY<NUMERIC>
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

	readDBPGStatements = []string{
		`CREATE TABLE TestTable (
                    Key          VARCHAR PRIMARY KEY,
                    StringValue  VARCHAR
            )`,
		`CREATE INDEX TestTableByValue ON TestTable(StringValue)`,
		`CREATE INDEX TestTableByValueDesc ON TestTable(StringValue DESC)`,
	}

	simpleDBStatements = []string{
		`CREATE TABLE test (
				a	STRING(1024),
				b	STRING(1024),
			) PRIMARY KEY (a)`,
	}
	simpleDBPGStatements = []string{
		`CREATE TABLE test (
				a	VARCHAR(1024) PRIMARY KEY,
				b	VARCHAR(1024)
			)`,
	}
	simpleDBTableColumns = []string{"a", "b"}

	ctsDBStatements = []string{
		`CREATE TABLE TestTable (
		    Key  STRING(MAX) NOT NULL,
		    Ts   TIMESTAMP OPTIONS (allow_commit_timestamp = true),
	    ) PRIMARY KEY (Key)`,
	}
	backupDBStatements = []string{
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

	backupDBPGStatements = []string{
		`CREATE TABLE Singers (
						SingerId	BIGINT PRIMARY KEY,
						FirstName	VARCHAR(1024),
						LastName	VARCHAR(1024),
						SingerInfo	BYTEA
					)`,
		`CREATE INDEX SingerByName ON Singers(FirstName, LastName)`,
		`CREATE TABLE Accounts (
						AccountId	BIGINT PRIMARY KEY,
						Nickname	VARCHAR(100),
						Balance		BIGINT NOT NULL
					)`,
		`CREATE INDEX AccountByNickname ON Accounts(Nickname)`,
		`CREATE TABLE Types (
						RowID		BIGINT PRIMARY KEY,
						String		VARCHAR,
						Bytes		BYTEA,
						Int64a		BIGINT,
						Bool		BOOL,
						Float64		DOUBLE PRECISION,
						Numeric		NUMERIC
					)`,
	}

	fkdcDBStatements = []string{
		`CREATE TABLE Customers (
            CustomerId INT64 NOT NULL,
            CustomerName STRING(62) NOT NULL,
          ) PRIMARY KEY (CustomerId)`,
		`CREATE TABLE ShoppingCarts (
            CartId INT64 NOT NULL,
            CustomerId INT64 NOT NULL,
            CustomerName STRING(62) NOT NULL,
            CONSTRAINT FKShoppingCartsCustomerId FOREIGN KEY (CustomerId)
            REFERENCES Customers (CustomerId) ON DELETE CASCADE
         ) PRIMARY KEY (CartId)`,
	}

	fkdcDBPGStatements = []string{
		`CREATE TABLE Customers (
            CustomerId BIGINT,
            CustomerName VARCHAR(62) NOT NULL,
            PRIMARY KEY (CustomerId))`,
		`CREATE TABLE ShoppingCarts (
            CartId BIGINT,
            CustomerId BIGINT NOT NULL,
            CustomerName VARCHAR(62) NOT NULL,
            CONSTRAINT "FKShoppingCartsCustomerId" FOREIGN KEY (CustomerId)
            REFERENCES Customers (CustomerId) ON DELETE CASCADE,
            PRIMARY KEY (CartId))`,
	}

	bitReverseSeqDBStatments = []string{
		`CREATE SEQUENCE seqT OPTIONS (sequence_kind = "bit_reversed_positive")`,
		`CREATE TABLE T (
            id INT64 DEFAULT (GET_NEXT_SEQUENCE_VALUE(Sequence seqT)),
            value INT64,
            counter INT64 DEFAULT (GET_INTERNAL_SEQUENCE_STATE(Sequence seqT)),
            br_id INT64 AS (BIT_REVERSE(id, true)) STORED,
            CONSTRAINT id_gt_0 CHECK (id > 0),
            CONSTRAINT counter_gt_br_id CHECK (counter >= br_id),
            CONSTRAINT br_id_true CHECK (id = BIT_REVERSE(br_id, true)),
          ) PRIMARY KEY (id)`,
	}

	bitReverseSeqDBPGStatments = []string{
		`CREATE SEQUENCE seqT BIT_REVERSED_POSITIVE`,
		`CREATE TABLE T (
		  id BIGINT DEFAULT nextval('seqT'),
		  value BIGINT,
		  counter BIGINT DEFAULT spanner.get_internal_sequence_state('seqT'),
		  br_id bigint GENERATED ALWAYS AS (spanner.bit_reverse(id, true)) STORED,
		  CONSTRAINT id_gt_0 CHECK (id > 0),
		  CONSTRAINT counter_gt_br_id CHECK (counter >= br_id),
          CONSTRAINT br_id_true CHECK (id = spanner.bit_reverse(br_id, true)),
		  PRIMARY KEY (id)
		)`,
	}

	intervalDBStatements = []string{
		`CREATE TABLE IntervalTable (
			key STRING(MAX),
			create_time TIMESTAMP,
			expiry_time TIMESTAMP,
			expiry_within_month bool AS (expiry_time - create_time < INTERVAL 30 DAY),
			interval_array_len INT64 AS (ARRAY_LENGTH(ARRAY<INTERVAL>[INTERVAL '1-2 3 4:5:6' YEAR TO SECOND]))
		) PRIMARY KEY (key)`,
	}
	intervalDBPGStatements = []string{
		`CREATE TABLE IntervalTable (
		    key text primary key,
		    create_time timestamptz,
		    expiry_time timestamptz,
		    expiry_within_month bool GENERATED ALWAYS AS (expiry_time - create_time < INTERVAL '30' DAY) STORED,
		    interval_array_len bigint GENERATED ALWAYS AS (ARRAY_LENGTH(ARRAY[INTERVAL '1-2 3 4:5:6'], 1)) STORED
    	)`,
	}

	statements = map[adminpb.DatabaseDialect]map[string][]string{
		adminpb.DatabaseDialect_GOOGLE_STANDARD_SQL: {
			singerDDLStatements:               singerDBStatements,
			simpleDDLStatements:               simpleDBStatements,
			readDDLStatements:                 readDBStatements,
			backupDDLStatements:               backupDBStatements,
			testTableDDLStatements:            readDBStatements,
			fkdcDDLStatements:                 fkdcDBStatements,
			testTableBitReversedSeqStatements: bitReverseSeqDBStatments,
			intervalDDLStatements:             intervalDBStatements,
		},
		adminpb.DatabaseDialect_POSTGRESQL: {
			singerDDLStatements:               singerDBPGStatements,
			simpleDDLStatements:               simpleDBPGStatements,
			readDDLStatements:                 readDBPGStatements,
			backupDDLStatements:               backupDBPGStatements,
			testTableDDLStatements:            readDBPGStatements,
			fkdcDDLStatements:                 fkdcDBPGStatements,
			testTableBitReversedSeqStatements: bitReverseSeqDBPGStatments,
			intervalDDLStatements:             intervalDBPGStatements,
		},
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

func getInstanceConfig() string {
	return os.Getenv("GCLOUD_TESTS_GOLANG_SPANNER_INSTANCE_CONFIG")
}

func getMultiplexEnableFlag() bool {
	return os.Getenv("GOOGLE_CLOUD_SPANNER_MULTIPLEXED_SESSIONS") == "true"
}

const (
	str1 = "alice"
	str2 = "a@example.com"
)

func TestMain(m *testing.M) {
	cleanup := initIntegrationTests()
	defer cleanup()
	for _, dialect := range []adminpb.DatabaseDialect{adminpb.DatabaseDialect_GOOGLE_STANDARD_SQL, adminpb.DatabaseDialect_POSTGRESQL} {
		if isEmulatorEnvSet() && dialect == adminpb.DatabaseDialect_POSTGRESQL {
			// PG tests are not supported in emulator
			continue
		}
		testDialect = dialect
		res := m.Run()
		if res != 0 {
			cleanup()
			os.Exit(res)
		}
	}
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
	var configName string
	if instanceConfig != "" {
		configName = fmt.Sprintf("projects/%s/instanceConfigs/%s", testProjectID, instanceConfig)
	} else {
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
		configName = config.Name
	}
	log.Printf("Running test by using the instance config: %s\n", configName)

	// First clean up any old test instances before we start the actual testing
	// as these might cause this test run to fail.
	cleanupInstances()

	// Create a test instance to use for this test run.
	op, err := instanceAdmin.CreateInstance(ctx, &instancepb.CreateInstanceRequest{
		Parent:     fmt.Sprintf("projects/%s", testProjectID),
		InstanceId: testInstanceID,
		Instance: &instancepb.Instance{
			Config:      configName,
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
			numOpened = sp.idleList.Len()
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
	client, _, cleanup := prepareIntegrationTest(ctx, t, DefaultSessionPoolConfig, statements[testDialect][singerDDLStatements])
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
	timeDiff := maxDuration(time.Since(writes[0].ts), 0)

	// Test reading rows with different timestamp bounds.
	for i, test := range []struct {
		name      string
		want      [][]interface{}
		tb        TimestampBound
		checkTs   func(time.Time) error
		skipForPG bool
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
			tb:   MaxStaleness(time.Nanosecond),
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
			// PG query with exact_staleness returns error code
			// "InvalidArgument", desc = "[ERROR] relation \"singers\" does not exist
			skipForPG: true,
			want:      nil,
			// Specify a staleness which should be already before this test.
			tb: ExactStaleness(time.Since(writes[0].ts) + timeDiff + 30*time.Second),
			checkTs: func(ts time.Time) error {
				if !ts.Before(writes[0].ts) {
					return fmt.Errorf("read got timestamp %v, want it to be earlier than %v", ts, writes[0].ts)
				}
				return nil
			},
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			singersQuery := "SELECT SingerId, FirstName, LastName FROM Singers WHERE SingerId IN (@p1, @p2, @p3) ORDER BY SingerId"
			if testDialect == adminpb.DatabaseDialect_POSTGRESQL {
				if test.skipForPG {
					t.Skip("Skipping testing of unsupported tests in Postgres dialect.")
				}
				singersQuery = "SELECT SingerId, FirstName, LastName FROM Singers WHERE SingerId = $1 OR  SingerId = $2 OR  SingerId = $3 ORDER BY SingerId"
			}
			// SingleUse.Query
			su := client.Single().WithTimestampBound(test.tb)
			got, err := readAll(su.Query(
				ctx,
				Statement{
					singersQuery,
					map[string]interface{}{"p1": int64(1), "p2": int64(3), "p3": int64(4)},
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
	client, _, cleanup := prepareIntegrationTest(ctx, t, DefaultSessionPoolConfig, statements[testDialect][singerDDLStatements])
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
	singersQuery := "SELECT SingerId, FirstName, LastName FROM Singers WHERE SingerId IN (@p1, @p2, @p3)"
	if testDialect == adminpb.DatabaseDialect_POSTGRESQL {
		singersQuery = "SELECT SingerId, FirstName, LastName FROM Singers WHERE SingerId = $1 OR  SingerId = $2 OR  SingerId = $3"
	}
	qo := QueryOptions{Options: &sppb.ExecuteSqlRequest_QueryOptions{
		OptimizerVersion:           "1",
		OptimizerStatisticsPackage: "latest",
	}}
	got, err := readAll(client.Single().QueryWithOptions(ctx, Statement{
		singersQuery,
		map[string]interface{}{"p1": int64(1), "p2": int64(3), "p3": int64(4)},
	}, qo))

	if err != nil {
		t.Errorf("ReadOnlyTransaction.QueryWithOptions returns error %v, want nil", err)
	}

	want := [][]interface{}{{int64(1), "Marc", "Foo"}, {int64(3), "Alpha", "Beta"}, {int64(4), "Last", "End"}}
	if !testEqual(got, want) {
		t.Errorf("got unexpected result from ReadOnlyTransaction.QueryWithOptions: %v, want %v", got, want)
	}
}

func TestIntegration_Interval(t *testing.T) {
	skipEmulatorTest(t)
	t.Parallel()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()
	client, _, cleanup := prepareIntegrationTest(ctx, t, DefaultSessionPoolConfig, statements[testDialect][intervalDDLStatements])
	defer cleanup()

	m := InsertOrUpdate("IntervalTable", []string{"key", "create_time", "expiry_time"},
		[]interface{}{
			"test1",
			time.Date(2004, 11, 30, 4, 53, 54, 0, time.UTC),
			time.Date(2004, 12, 15, 4, 53, 54, 0, time.UTC),
		})
	_, err := client.Apply(ctx, []*Mutation{m})
	if err != nil {
		t.Fatal(err)
	}
	placeHolder := "@p1"
	timestampQuery := `TIMESTAMP('2004-11-30T10:23:54+0530')`
	if testDialect == adminpb.DatabaseDialect_POSTGRESQL {
		placeHolder = "$1"
		timestampQuery = `TIMESTAMPTZ '2004-11-30T10:23:54+0530'`
	}
	stmt := Statement{
		SQL: `SELECT expiry_within_month, interval_array_len FROM IntervalTable WHERE key = ` + placeHolder,
		Params: map[string]interface{}{
			"p1": "test1",
		},
	}
	iter := client.Single().Query(ctx, stmt)
	defer iter.Stop()

	row, err := iter.Next()
	if err != nil {
		t.Fatal(err)
	}

	var expiryWithinMonth bool
	var intervalArrayLen int64
	if err := row.Columns(&expiryWithinMonth, &intervalArrayLen); err != nil {
		t.Fatal(err)
	}

	if !expiryWithinMonth {
		t.Error("expected expiry_within_month to be true")
	}
	if intervalArrayLen != 1 {
		t.Errorf("expected interval_array_len to be 1, got %d", intervalArrayLen)
	}

	stmt = Statement{SQL: "SELECT INTERVAL '1' DAY + INTERVAL '1' MONTH AS Col1"}
	iter = client.Single().Query(ctx, stmt)
	defer iter.Stop()

	row, err = iter.Next()
	if err != nil {
		t.Fatal(err)
	}

	var interval Interval
	if err := row.Column(0, &interval); err != nil {
		t.Fatal(err)
	}

	wantInterval := Interval{
		Months: 1,
		Days:   1,
		Nanos:  big.NewInt(0),
	}

	if interval.Months != wantInterval.Months || interval.Days != wantInterval.Days || interval.Nanos.Cmp(wantInterval.Nanos) != 0 {
		t.Errorf("got interval %+v, want %+v", interval, wantInterval)
	}

	m = InsertOrUpdate("IntervalTable", []string{"key", "create_time", "expiry_time"},
		[]interface{}{
			"test2",
			time.Date(2004, 8, 30, 4, 53, 54, 0, time.UTC),
			time.Date(2004, 12, 15, 4, 53, 54, 0, time.UTC),
		})
	_, err = client.Apply(ctx, []*Mutation{m})
	if err != nil {
		t.Fatal(err)
	}

	stmt = Statement{
		SQL: `SELECT COUNT(*) FROM IntervalTable WHERE create_time < ` + timestampQuery + ` - ` + placeHolder,
		Params: map[string]interface{}{
			"p1": Interval{Days: 30},
		},
	}
	iter = client.Single().Query(ctx, stmt)
	defer iter.Stop()

	row, err = iter.Next()
	if err != nil {
		t.Fatal(err)
	}

	var count int64
	if err := row.Column(0, &count); err != nil {
		t.Fatal(err)
	}

	if count != 1 {
		t.Errorf("got count %d, want 1", count)
	}

	stmt = Statement{
		SQL: "SELECT " + placeHolder,
		Params: map[string]interface{}{
			"p1": []Interval{
				{Months: 14, Days: 3, Nanos: big.NewInt(14706000000000)},
				{},
				{Months: -14, Days: -3, Nanos: big.NewInt(-14706000000000)},
				{},
			},
		},
	}
	iter = client.Single().Query(ctx, stmt)
	defer iter.Stop()

	row, err = iter.Next()
	if err != nil {
		t.Fatal(err)
	}

	var intervals []NullInterval
	if err := row.Column(0, &intervals); err != nil {
		t.Fatal(err)
	}

	wantIntervals := []NullInterval{
		{Interval: Interval{Months: 14, Days: 3, Nanos: big.NewInt(14706000000000)}, Valid: true},
		{Valid: true},
		{Interval: Interval{Months: -14, Days: -3, Nanos: big.NewInt(-14706000000000)}, Valid: true},
		{Valid: true},
	}

	if len(intervals) != len(wantIntervals) {
		t.Fatalf("got %d intervals, want %d", len(intervals), len(wantIntervals))
	}

	for i := range intervals {
		if intervals[i].Valid != wantIntervals[i].Valid || intervals[i].Interval.Months != wantIntervals[i].Interval.Months ||
			intervals[i].Interval.Days != wantIntervals[i].Interval.Days ||
			(intervals[i].Interval.Nanos != nil && wantIntervals[i].Interval.Nanos != nil && intervals[i].Interval.Nanos.Cmp(wantIntervals[i].Interval.Nanos) != 0) {
			t.Errorf("interval %d: got %+v, want %+v", i, intervals[i], wantIntervals[i])
		}
	}

	stmt = Statement{
		SQL: `SELECT ARRAY[CAST('P1Y2M3DT4H5M6.789123S' AS INTERVAL), 
		                   null, 
		                   CAST('P-1Y-2M-3DT-4H-5M-6.789123S' AS INTERVAL)] AS Col1`,
	}
	iter = client.Single().Query(ctx, stmt)
	defer iter.Stop()

	row, err = iter.Next()
	if err != nil {
		t.Fatal(err)
	}

	if err := row.Column(0, &intervals); err != nil {
		t.Fatal(err)
	}

	wantIntervals = []NullInterval{
		{Interval: Interval{Months: 14, Days: 3, Nanos: big.NewInt(14706789123000)}, Valid: true},
		{Valid: false},
		{Interval: Interval{Months: -14, Days: -3, Nanos: big.NewInt(-14706789123000)}, Valid: true},
	}

	if len(intervals) != len(wantIntervals) {
		t.Fatalf("got %d intervals, want %d", len(intervals), len(wantIntervals))
	}

	for i := range intervals {
		if intervals[i].Valid != wantIntervals[i].Valid || intervals[i].Interval.Months != wantIntervals[i].Interval.Months ||
			intervals[i].Interval.Days != wantIntervals[i].Interval.Days ||
			(intervals[i].Interval.Nanos != nil && wantIntervals[i].Interval.Nanos != nil && intervals[i].Interval.Nanos.Cmp(wantIntervals[i].Interval.Nanos) != 0) {
			t.Errorf("interval %d: got %+v, want %+v", i, intervals[i], wantIntervals[i])
		}
	}
}

func TestIntegration_TransactionWasStartedInDifferentSession(t *testing.T) {
	t.Parallel()
	// TODO: unskip once https://b.corp.google.com/issues/309745482 is fixed
	skipOnNonProd(t)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()
	// Set up testing environment.
	client, _, cleanup := prepareIntegrationTest(ctx, t, DefaultSessionPoolConfig, statements[testDialect][singerDDLStatements])
	defer cleanup()

	attempts := 0
	_, err := client.ReadWriteTransaction(ctx, func(ctx context.Context, transaction *ReadWriteTransaction) error {
		attempts++
		if attempts == 1 {
			deleteTestSession(ctx, t, transaction.sh.getID())
		}
		if _, err := readAll(transaction.Query(ctx, NewStatement("select * from singers"))); err != nil {
			return err
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if g, w := attempts, 2; g != w {
		t.Fatalf("attempts mismatch\nGot:  %v\nWant: %v", g, w)
	}
}

func deleteTestSession(ctx context.Context, t *testing.T, sessionName string) {
	var opts []option.ClientOption
	if emulatorAddr := os.Getenv("SPANNER_EMULATOR_HOST"); emulatorAddr != "" {
		emulatorOpts := []option.ClientOption{
			option.WithEndpoint(emulatorAddr),
			option.WithGRPCDialOption(grpc.WithTransportCredentials(insecure.NewCredentials())),
			option.WithoutAuthentication(),
			internaloption.SkipDialSettingsValidation(),
		}
		opts = append(emulatorOpts, opts...)
	}
	gapic, err := v1.NewClient(ctx, opts...)
	if err != nil {
		t.Fatalf("could not create gapic client: %v", err)
	}
	defer gapic.Close()
	if err := gapic.DeleteSession(ctx, &sppb.DeleteSessionRequest{Name: sessionName}); err != nil {
		t.Fatal(err)
	}
}

func TestIntegration_BatchWrite(t *testing.T) {
	skipEmulatorTest(t)
	t.Parallel()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()
	// Set up testing environment.
	client, _, cleanup := prepareIntegrationTest(ctx, t, DefaultSessionPoolConfig, statements[testDialect][singerDDLStatements])
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
	mgs := make([]*MutationGroup, len(writes))
	// Try to write four rows through the BatchWrite API.
	for i, w := range writes {
		m := InsertOrUpdate("Singers",
			[]string{"SingerId", "FirstName", "LastName"},
			w.row)
		ms := make([]*Mutation, 1)
		ms[0] = m
		mgs[i] = &MutationGroup{Mutations: ms}
	}
	// Records the mutation group indexes received in the response.
	seen := make(map[int32]int32)
	numMutationGroups := len(mgs)
	validate := func(res *sppb.BatchWriteResponse) error {
		if status := status.ErrorProto(res.GetStatus()); status != nil {
			t.Fatalf("Invalid status: %v", status)
		}
		if ts := res.GetCommitTimestamp(); ts == nil {
			t.Fatal("Invalid commit timestamp")
		}
		for _, idx := range res.GetIndexes() {
			if idx >= 0 && idx < int32(numMutationGroups) {
				seen[idx]++
			} else {
				t.Fatalf("Index %v out of range. Expected range [%v,%v]", idx, 0, numMutationGroups-1)
			}
		}
		return nil
	}
	iter := client.BatchWrite(ctx, mgs)
	if err := iter.Do(validate); err != nil {
		t.Fatal(err)
	}
	// Validate that each mutation group index is seen exactly once.
	if numMutationGroups != len(seen) {
		t.Fatalf("Expected %v indexes, got %v indexes", numMutationGroups, len(seen))
	}
	for idx, ct := range seen {
		if ct != 1 {
			t.Fatalf("Index %v seen %v times instead of exactly once", idx, ct)
		}
	}

	// Verify the writes by reading the database.
	singersQuery := "SELECT SingerId, FirstName, LastName FROM Singers WHERE SingerId IN (@p1, @p2, @p3)"
	if testDialect == adminpb.DatabaseDialect_POSTGRESQL {
		singersQuery = "SELECT SingerId, FirstName, LastName FROM Singers WHERE SingerId = $1 OR  SingerId = $2 OR  SingerId = $3"
	}
	qo := QueryOptions{Options: &sppb.ExecuteSqlRequest_QueryOptions{
		OptimizerVersion:           "1",
		OptimizerStatisticsPackage: "latest",
	}}
	got, err := readAll(client.Single().QueryWithOptions(ctx, Statement{
		singersQuery,
		map[string]interface{}{"p1": int64(1), "p2": int64(3), "p3": int64(4)},
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
	client, _, cleanup := prepareIntegrationTest(ctx, t, DefaultSessionPoolConfig, statements[testDialect][singerDDLStatements])
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
	client, _, cleanup := prepareIntegrationTest(ctx, t, DefaultSessionPoolConfig, statements[testDialect][singerDDLStatements])
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
		want      [][]interface{}
		tb        TimestampBound
		checkTs   func(time.Time) error
		skipForPG bool
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
			false,
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
			false,
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
			// PG query with exact_staleness returns Table not found error
			true,
		},
	} {
		// ReadOnlyTransaction.Query
		ro := client.ReadOnlyTransaction().WithTimestampBound(test.tb)
		singersQuery := "SELECT SingerId, FirstName, LastName FROM Singers WHERE SingerId IN (@p1, @p2, @p3) ORDER BY SingerId"
		if testDialect == adminpb.DatabaseDialect_POSTGRESQL {
			if test.skipForPG {
				t.Skip("Skipping testing of unsupported tests in Postgres dialect.")
			}
			singersQuery = "SELECT SingerId, FirstName, LastName FROM Singers WHERE SingerId = $1 OR  SingerId = $2 OR  SingerId = $3 ORDER BY SingerId"
		}
		got, err := readAll(ro.Query(
			ctx,
			Statement{
				singersQuery,
				map[string]interface{}{"p1": int64(1), "p2": int64(3), "p3": int64(4)},
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
	client, _, cleanup := prepareIntegrationTest(ctx, t, DefaultSessionPoolConfig, statements[testDialect][singerDDLStatements])
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
	client, _, cleanup := prepareIntegrationTest(ctx, t, DefaultSessionPoolConfig, statements[testDialect][singerDDLStatements])
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

	queryAccountByID := "SELECT Balance FROM Accounts WHERE AccountId = @p1"
	if testDialect == adminpb.DatabaseDialect_POSTGRESQL {
		queryAccountByID = "SELECT Balance FROM Accounts WHERE AccountId = $1"
	}
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func(iter int) {
			defer wg.Done()
			_, err := client.ReadWriteTransaction(ctx, func(ctx context.Context, tx *ReadWriteTransaction) error {
				// Query Foo's balance and Bar's balance.
				bf, e := readBalance(tx.Query(ctx,
					Statement{queryAccountByID, map[string]interface{}{"p1": int64(1)}}))
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
		// reading non-indexed column from index is not supported in PG
		if testDialect == adminpb.DatabaseDialect_POSTGRESQL {
			bb, e = readBalance(tx.Read(ctx, "Accounts", Key{int64(2)}, []string{"Balance"}))
			if e != nil {
				return e
			}
		} else {
			bb, e = readBalance(tx.ReadUsingIndex(ctx, "Accounts", "AccountByNickname", KeySets(Key{"Bar"}), []string{"Balance"}))
			if e != nil {
				return e
			}
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

// Test ReadWriteTransactionWithOptions.
func TestIntegration_ReadWriteTransactionWithOptions(t *testing.T) {
	t.Parallel()
	skipEmulatorTest(t)

	// Give a longer deadline because of transaction backoffs.
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()
	client, _, cleanup := prepareIntegrationTest(ctx, t, DefaultSessionPoolConfig, statements[testDialect][singerDDLStatements])
	defer cleanup()

	// Set up two accounts
	accounts := []*Mutation{
		Insert("Accounts", []string{"AccountId", "Nickname", "Balance"}, []interface{}{int64(1), "Foo", int64(50)}),
		Insert("Accounts", []string{"AccountId", "Nickname", "Balance"}, []interface{}{int64(2), "Bar", int64(1)}),
	}
	if _, err := client.Apply(ctx, accounts, ApplyAtLeastOnce()); err != nil {
		t.Fatal(err)
	}

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

	duration, _ := time.ParseDuration("100ms")
	txOpts := TransactionOptions{CommitOptions: CommitOptions{ReturnCommitStats: true, MaxCommitDelay: &duration}}
	resp, err := client.ReadWriteTransactionWithOptions(ctx, func(ctx context.Context, tx *ReadWriteTransaction) error {
		// Query Foo's balance and Bar's balance.
		queryAccountByID := "SELECT Balance FROM Accounts WHERE AccountId = @p1"
		if testDialect == adminpb.DatabaseDialect_POSTGRESQL {
			queryAccountByID = "SELECT Balance FROM Accounts WHERE AccountId = $1"
		}
		bf, e := readBalance(tx.Query(ctx,
			Statement{queryAccountByID, map[string]interface{}{"p1": int64(1)}}))
		if e != nil {
			return e
		}
		bb, e := readBalance(tx.Read(ctx, "Accounts", KeySets(Key{int64(2)}), []string{"Balance"}))
		if e != nil {
			return e
		}
		if bf <= 0 {
			return nil
		}
		bf--
		bb++
		return tx.BufferWrite([]*Mutation{
			Update("Accounts", []string{"AccountId", "Balance"}, []interface{}{int64(1), bf}),
			Update("Accounts", []string{"AccountId", "Balance"}, []interface{}{int64(2), bb}),
		})
	}, txOpts)
	if err != nil {
		t.Fatalf("Failed to execute transaction: %v", err)
	}
	if resp.CommitStats == nil {
		t.Fatal("Missing commit stats in commit response")
	}
	expectedMutationCount := int64(8)
	if testDialect == adminpb.DatabaseDialect_POSTGRESQL {
		// for PG mutation count for Balance column is not added for AccountName index
		expectedMutationCount = int64(4)
	}
	if got, want := resp.CommitStats.MutationCount, expectedMutationCount; got != want {
		t.Errorf("Mismatch mutation count - got: %v, want: %v", got, want)
	}
}

func TestIntegration_ReadWriteTransaction_StatementBased(t *testing.T) {
	t.Parallel()
	skipEmulatorTest(t)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()
	client, _, cleanup := prepareIntegrationTest(ctx, t, DefaultSessionPoolConfig, statements[testDialect][singerDDLStatements])
	defer cleanup()

	// Set up two accounts
	accounts := []*Mutation{
		Insert("Accounts", []string{"AccountId", "Nickname", "Balance"}, []interface{}{int64(1), "Foo", int64(50)}),
		Insert("Accounts", []string{"AccountId", "Nickname", "Balance"}, []interface{}{int64(2), "Bar", int64(1)}),
	}
	duration, err := time.ParseDuration("100ms")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := client.Apply(ctx, accounts, ApplyAtLeastOnce(), ApplyCommitOptions(CommitOptions{ReturnCommitStats: true, MaxCommitDelay: &duration})); err != nil {
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

func TestIntegration_ReadWriteTransaction_StatementBasedWithOptions(t *testing.T) {
	t.Parallel()
	skipEmulatorTest(t)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()
	client, _, cleanup := prepareIntegrationTest(ctx, t, DefaultSessionPoolConfig, statements[testDialect][singerDDLStatements])
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

	var resp CommitResponse
	txOpts := TransactionOptions{CommitOptions: CommitOptions{ReturnCommitStats: true}}
	for {
		tx, err := NewReadWriteStmtBasedTransactionWithOptions(ctx, client, txOpts)
		if err != nil {
			t.Fatalf("failed to begin a transaction: %v", err)
		}
		err = statements(tx)
		if err != nil && status.Code(err) != codes.Aborted {
			tx.Rollback(ctx)
			t.Fatalf("failed to execute statements: %v", err)
		} else if err == nil {
			resp, err = tx.CommitWithReturnResp(ctx)
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
	if resp.CommitStats == nil {
		t.Fatal("Missing commit stats in commit response")
	}
	expectedMutationCount := int64(8)
	if testDialect == adminpb.DatabaseDialect_POSTGRESQL {
		// for PG mutation count for Balance column is not added for AccountName index
		expectedMutationCount = int64(4)
	}
	if got, want := resp.CommitStats.MutationCount, expectedMutationCount; got != want {
		t.Errorf("Mismatch mutation count - got: %v, want: %v", got, want)
	}
}

func TestIntegration_Reads(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()
	// Set up testing environment.
	_, dbPath, cleanup := prepareIntegrationTest(ctx, t, DefaultSessionPoolConfig, statements[testDialect][testTableDDLStatements])
	defer cleanup()

	client, err := createClient(ctx, dbPath, ClientConfig{SessionPoolConfig: DefaultSessionPoolConfig, Compression: "gzip"})
	if err != nil {
		t.Fatal(err)
	}
	defer client.Close()

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
	if !errors.Is(err, ErrRowNotFound) {
		t.Fatalf("got %v, want spanner.ErrRowNotFound", err)
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
	if !errors.Is(err, ErrRowNotFound) {
		t.Fatalf("got %v, want spanner.ErrRowNotFound", err)
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
	client, _, cleanup := prepareIntegrationTest(ctx, t, DefaultSessionPoolConfig, statements[testDialect][testTableDDLStatements])
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
	client, _, cleanup := prepareIntegrationTest(ctx, t, DefaultSessionPoolConfig, statements[testDialect][singerDDLStatements])
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
	skipUnsupportedPGTest(t)

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

	// Pass spanner host as options for running builds against different environments
	opts := []option.ClientOption{option.WithEndpoint(spannerHost), option.WithGRPCDialOption(grpc.WithUnaryInterceptor(interceptor))}
	dbAdmin, err := database.NewDatabaseAdminClient(ctx, opts...)
	if err != nil {
		log.Fatalf("cannot create dbAdmin client: %v", err)
	}
	op, err := dbAdmin.CreateDatabaseWithRetry(ctx, &adminpb.CreateDatabaseRequest{
		Parent:          fmt.Sprintf("projects/%v/instances/%v", testProjectID, testInstanceID),
		CreateStatement: "CREATE DATABASE " + dbName,
		DatabaseDialect: testDialect,
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
	client, dbPath, cleanup := prepareIntegrationTest(ctx, t, SessionPoolConfig{}, statements[testDialect][singerDDLStatements])
	defer cleanup()
	if isMultiplexEnabled {
		// TODO: confirm that this is the valid scenario for multiplexed sessions, and what's expected behavior.
		// wait for the multiplexed session to be created.
		waitFor(t, func() error {
			client.idleSessions.mu.Lock()
			defer client.idleSessions.mu.Unlock()
			if client.idleSessions.multiplexedSession == nil {
				return errInvalidSessionPool
			}
			return nil
		})
		// Close the multiplexed session to prevent the session pool maintainer
		// from repeatedly trying to use sessions for the invalid database.
		client.idleSessions.mu.Lock()
		client.idleSessions.multiplexedSession = nil
		client.idleSessions.mu.Unlock()
	}

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
	req := &adminpb.CreateDatabaseRequest{
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
		DatabaseDialect: testDialect,
	}
	if testDialect == adminpb.DatabaseDialect_POSTGRESQL {
		req.ExtraStatements = []string{}
	}
	op, err := databaseAdmin.CreateDatabaseWithRetry(ctx, req)
	if err != nil {
		t.Fatalf("cannot recreate testing DB %v: %v", dbPath, err)
	}
	if _, err := op.Wait(ctx); err != nil {
		t.Fatalf("cannot recreate testing DB %v: %v", dbPath, err)
	}
	if testDialect == adminpb.DatabaseDialect_POSTGRESQL {
		op, err := databaseAdmin.UpdateDatabaseDdl(ctx, &adminpb.UpdateDatabaseDdlRequest{
			Database: dbPath,
			Statements: []string{`
			CREATE TABLE Singers (
				SingerId INT8 NOT NULL,
				FirstName VARCHAR(1024),
				LastName  VARCHAR(1024),
				SingerInfo	BYTEA,
				PRIMARY KEY(SingerId)
			)`},
		})
		if err != nil {
			t.Fatalf("cannot create singers table %v: %v", dbPath, err)
		}
		if err := op.Wait(ctx); err != nil {
			t.Fatalf("timeout creating singers table %v: %v", dbPath, err)
		}
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
	skipUnsupportedPGTest(t)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()
	stmts := []string{
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
					NumericArray	ARRAY<NUMERIC>,
					JSON		JSON,
					JSONArray	ARRAY<JSON>,
				) PRIMARY KEY (RowID)`,
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

	type Message struct {
		Name       string
		Body       string
		Time       int64
		FloatValue interface{}
	}
	msg := Message{"Alice", "Hello", 145688415796432520, json.Number("0.39240506000000003")}
	unmarshalledJSONStructUsingNumber := map[string]interface{}{
		"Name":       "Alice",
		"Body":       "Hello",
		"Time":       json.Number("145688415796432520"),
		"FloatValue": json.Number("0.39240506000000003"),
	}
	unmarshalledJSONStruct := map[string]interface{}{
		"Name":       "Alice",
		"Body":       "Hello",
		"Time":       1.456884157964325e+17,
		"FloatValue": 0.39240506,
	}

	tests := []struct {
		col                   string
		val                   interface{}
		wantWithDefaultConfig interface{}
		wantWithNumber        interface{}
	}{
		{col: "String", val: ""},
		{col: "String", val: "", wantWithDefaultConfig: NullString{"", true}, wantWithNumber: NullString{"", true}},
		{col: "String", val: "foo"},
		{col: "String", val: "foo", wantWithDefaultConfig: NullString{"foo", true}, wantWithNumber: NullString{"foo", true}},
		{col: "String", val: NullString{"bar", true}, wantWithDefaultConfig: "bar", wantWithNumber: "bar"},
		{col: "String", val: NullString{"bar", false}, wantWithDefaultConfig: NullString{"", false}, wantWithNumber: NullString{"", false}},
		{col: "StringArray", val: []string(nil), wantWithDefaultConfig: []NullString(nil), wantWithNumber: []NullString(nil)},
		{col: "StringArray", val: []string{}, wantWithDefaultConfig: []NullString{}, wantWithNumber: []NullString{}},
		{col: "StringArray", val: []string{"foo", "bar"}, wantWithDefaultConfig: []NullString{{"foo", true}, {"bar", true}}, wantWithNumber: []NullString{{"foo", true}, {"bar", true}}},
		{col: "StringArray", val: []NullString(nil)},
		{col: "StringArray", val: []NullString{}},
		{col: "StringArray", val: []NullString{{"foo", true}, {}}},
		{col: "Bytes", val: []byte{}},
		{col: "Bytes", val: []byte{1, 2, 3}},
		{col: "Bytes", val: []byte(nil)},
		{col: "BytesArray", val: [][]byte(nil)},
		{col: "BytesArray", val: [][]byte{}},
		{col: "BytesArray", val: [][]byte{{1}, {2, 3}}},
		{col: "Int64a", val: 0, wantWithDefaultConfig: int64(0), wantWithNumber: int64(0)},
		{col: "Int64a", val: -1, wantWithDefaultConfig: int64(-1), wantWithNumber: int64(-1)},
		{col: "Int64a", val: 2, wantWithDefaultConfig: int64(2), wantWithNumber: int64(2)},
		{col: "Int64a", val: int64(3)},
		{col: "Int64a", val: 4, wantWithDefaultConfig: NullInt64{4, true}, wantWithNumber: NullInt64{4, true}},
		{col: "Int64a", val: NullInt64{5, true}, wantWithDefaultConfig: int64(5), wantWithNumber: int64(5)},
		{col: "Int64a", val: NullInt64{6, true}, wantWithDefaultConfig: int64(6), wantWithNumber: int64(6)},
		{col: "Int64a", val: NullInt64{7, false}, wantWithDefaultConfig: NullInt64{0, false}, wantWithNumber: NullInt64{0, false}},
		{col: "Int64Array", val: []int(nil), wantWithDefaultConfig: []NullInt64(nil), wantWithNumber: []NullInt64(nil)},
		{col: "Int64Array", val: []int{}, wantWithDefaultConfig: []NullInt64{}, wantWithNumber: []NullInt64{}},
		{col: "Int64Array", val: []int{1, 2}, wantWithDefaultConfig: []NullInt64{{1, true}, {2, true}}, wantWithNumber: []NullInt64{{1, true}, {2, true}}},
		{col: "Int64Array", val: []int64(nil), wantWithDefaultConfig: []NullInt64(nil), wantWithNumber: []NullInt64(nil)},
		{col: "Int64Array", val: []int64{}, wantWithDefaultConfig: []NullInt64{}, wantWithNumber: []NullInt64{}},
		{col: "Int64Array", val: []int64{1, 2}, wantWithDefaultConfig: []NullInt64{{1, true}, {2, true}}, wantWithNumber: []NullInt64{{1, true}, {2, true}}},
		{col: "Int64Array", val: []NullInt64(nil)},
		{col: "Int64Array", val: []NullInt64{}},
		{col: "Int64Array", val: []NullInt64{{1, true}, {}}},
		{col: "Bool", val: false},
		{col: "Bool", val: true},
		{col: "Bool", val: false, wantWithDefaultConfig: NullBool{false, true}, wantWithNumber: NullBool{false, true}},
		{col: "Bool", val: true, wantWithDefaultConfig: NullBool{true, true}, wantWithNumber: NullBool{true, true}},
		{col: "Bool", val: NullBool{true, true}},
		{col: "Bool", val: NullBool{false, false}},
		{col: "BoolArray", val: []bool(nil), wantWithDefaultConfig: []NullBool(nil), wantWithNumber: []NullBool(nil)},
		{col: "BoolArray", val: []bool{}, wantWithDefaultConfig: []NullBool{}, wantWithNumber: []NullBool{}},
		{col: "BoolArray", val: []bool{true, false}, wantWithDefaultConfig: []NullBool{{true, true}, {false, true}}, wantWithNumber: []NullBool{{true, true}, {false, true}}},
		{col: "BoolArray", val: []NullBool(nil)},
		{col: "BoolArray", val: []NullBool{}},
		{col: "BoolArray", val: []NullBool{{false, true}, {true, true}, {}}},
		{col: "Float64", val: 0.0},
		{col: "Float64", val: 3.14},
		{col: "Float64", val: math.NaN()},
		{col: "Float64", val: math.Inf(1)},
		{col: "Float64", val: math.Inf(-1)},
		{col: "Float64", val: 2.78, wantWithDefaultConfig: NullFloat64{2.78, true}, wantWithNumber: NullFloat64{2.78, true}},
		{col: "Float64", val: NullFloat64{2.71, true}, wantWithDefaultConfig: 2.71, wantWithNumber: 2.71},
		{col: "Float64", val: NullFloat64{1.41, true}, wantWithDefaultConfig: NullFloat64{1.41, true}, wantWithNumber: NullFloat64{1.41, true}},
		{col: "Float64", val: NullFloat64{0, false}},
		{col: "Float64Array", val: []float64(nil), wantWithDefaultConfig: []NullFloat64(nil), wantWithNumber: []NullFloat64(nil)},
		{col: "Float64Array", val: []float64{}, wantWithDefaultConfig: []NullFloat64{}, wantWithNumber: []NullFloat64{}},
		{col: "Float64Array", val: []float64{2.72, 3.14, math.Inf(1)}, wantWithDefaultConfig: []NullFloat64{{2.72, true}, {3.14, true}, {math.Inf(1), true}}, wantWithNumber: []NullFloat64{{2.72, true}, {3.14, true}, {math.Inf(1), true}}},
		{col: "Float64Array", val: []NullFloat64(nil)},
		{col: "Float64Array", val: []NullFloat64{}},
		{col: "Float64Array", val: []NullFloat64{{2.72, true}, {math.Inf(1), true}, {}}},
		{col: "Date", val: d1},
		{col: "Date", val: d1, wantWithDefaultConfig: NullDate{d1, true}, wantWithNumber: NullDate{d1, true}},
		{col: "Date", val: NullDate{d1, true}},
		{col: "Date", val: NullDate{d1, true}, wantWithDefaultConfig: d1, wantWithNumber: d1},
		{col: "Date", val: NullDate{civil.Date{}, false}},
		{col: "DateArray", val: []civil.Date(nil), wantWithDefaultConfig: []NullDate(nil), wantWithNumber: []NullDate(nil)},
		{col: "DateArray", val: []civil.Date{}, wantWithDefaultConfig: []NullDate{}, wantWithNumber: []NullDate{}},
		{col: "DateArray", val: []civil.Date{d1, d2, d3}, wantWithDefaultConfig: []NullDate{{d1, true}, {d2, true}, {d3, true}}, wantWithNumber: []NullDate{{d1, true}, {d2, true}, {d3, true}}},
		{col: "Timestamp", val: t1},
		{col: "Timestamp", val: t1, wantWithDefaultConfig: NullTime{t1, true}, wantWithNumber: NullTime{t1, true}},
		{col: "Timestamp", val: NullTime{t1, true}},
		{col: "Timestamp", val: NullTime{t1, true}, wantWithDefaultConfig: t1, wantWithNumber: t1},
		{col: "Timestamp", val: NullTime{}},
		{col: "TimestampArray", val: []time.Time(nil), wantWithDefaultConfig: []NullTime(nil), wantWithNumber: []NullTime(nil)},
		{col: "TimestampArray", val: []time.Time{}, wantWithDefaultConfig: []NullTime{}, wantWithNumber: []NullTime{}},
		{col: "TimestampArray", val: []time.Time{t1, t2, t3}, wantWithDefaultConfig: []NullTime{{t1, true}, {t2, true}, {t3, true}}, wantWithNumber: []NullTime{{t1, true}, {t2, true}, {t3, true}}},
		{col: "Numeric", val: n1},
		{col: "Numeric", val: n2},
		{col: "Numeric", val: n1, wantWithDefaultConfig: NullNumeric{n1, true}, wantWithNumber: NullNumeric{n1, true}},
		{col: "Numeric", val: n2, wantWithDefaultConfig: NullNumeric{n2, true}, wantWithNumber: NullNumeric{n2, true}},
		{col: "Numeric", val: NullNumeric{n1, true}, wantWithDefaultConfig: n1, wantWithNumber: n1},
		{col: "Numeric", val: NullNumeric{n1, true}, wantWithDefaultConfig: NullNumeric{n1, true}, wantWithNumber: NullNumeric{n1, true}},
		{col: "Numeric", val: NullNumeric{n0, false}},
		{col: "NumericArray", val: []big.Rat(nil), wantWithDefaultConfig: []NullNumeric(nil), wantWithNumber: []NullNumeric(nil)},
		{col: "NumericArray", val: []big.Rat{}, wantWithDefaultConfig: []NullNumeric{}, wantWithNumber: []NullNumeric{}},
		{col: "NumericArray", val: []big.Rat{n1, n2}, wantWithDefaultConfig: []NullNumeric{{n1, true}, {n2, true}}, wantWithNumber: []NullNumeric{{n1, true}, {n2, true}}},
		{col: "NumericArray", val: []NullNumeric(nil)},
		{col: "NumericArray", val: []NullNumeric{}},
		{col: "NumericArray", val: []NullNumeric{{n1, true}, {n2, true}, {}}},
		{col: "JSON", val: NullJSON{msg, true}, wantWithDefaultConfig: NullJSON{unmarshalledJSONStruct, true}, wantWithNumber: NullJSON{unmarshalledJSONStructUsingNumber, true}},
		{col: "JSON", val: NullJSON{msg, false}, wantWithDefaultConfig: NullJSON{}, wantWithNumber: NullJSON{}},
		{col: "JSONArray", val: []NullJSON(nil)},
		{col: "JSONArray", val: []NullJSON{}},
		{col: "JSONArray", val: []NullJSON{{msg, true}, {msg, true}, {}}, wantWithDefaultConfig: []NullJSON{{unmarshalledJSONStruct, true}, {unmarshalledJSONStruct, true}, {}}, wantWithNumber: []NullJSON{{unmarshalledJSONStructUsingNumber, true}, {unmarshalledJSONStructUsingNumber, true}, {}}},
		{col: "String", val: nil, wantWithDefaultConfig: NullString{}, wantWithNumber: NullString{}},
		{col: "StringArray", val: nil, wantWithDefaultConfig: []NullString(nil), wantWithNumber: []NullString(nil)},
		{col: "Bytes", val: nil, wantWithDefaultConfig: []byte(nil), wantWithNumber: []byte(nil)},
		{col: "BytesArray", val: nil, wantWithDefaultConfig: [][]byte(nil), wantWithNumber: [][]byte(nil)},
		{col: "Int64a", val: nil, wantWithDefaultConfig: NullInt64{}, wantWithNumber: NullInt64{}},
		{col: "Int64Array", val: nil, wantWithDefaultConfig: []NullInt64(nil), wantWithNumber: []NullInt64(nil)},
		{col: "Bool", val: nil, wantWithDefaultConfig: NullBool{}, wantWithNumber: NullBool{}},
		{col: "BoolArray", val: nil, wantWithDefaultConfig: []NullBool(nil), wantWithNumber: []NullBool(nil)},
		{col: "Float64", val: nil, wantWithDefaultConfig: NullFloat64{}, wantWithNumber: NullFloat64{}},
		{col: "Float64Array", val: nil, wantWithDefaultConfig: []NullFloat64(nil), wantWithNumber: []NullFloat64(nil)},
		{col: "Numeric", val: nil, wantWithDefaultConfig: NullNumeric{}, wantWithNumber: NullNumeric{}},
		{col: "NumericArray", val: nil, wantWithDefaultConfig: []NullNumeric(nil), wantWithNumber: []NullNumeric(nil)},
		{col: "JSON", val: nil, wantWithDefaultConfig: NullJSON{}, wantWithNumber: NullJSON{}},
		{col: "JSONArray", val: nil, wantWithDefaultConfig: []NullJSON(nil), wantWithNumber: []NullJSON(nil)},
	}

	// See https://github.com/GoogleCloudPlatform/cloud-spanner-emulator/issues/31
	if !isEmulatorEnvSet() {
		tests = append(tests, []struct {
			col                   string
			val                   interface{}
			wantWithDefaultConfig interface{}
			wantWithNumber        interface{}
		}{
			{col: "Date", val: nil, wantWithDefaultConfig: NullDate{}, wantWithNumber: NullDate{}},
			{col: "Timestamp", val: nil, wantWithDefaultConfig: NullTime{}, wantWithNumber: NullTime{}},
		}...)
	}

	for _, withNumberConfigOption := range []bool{false, true} {
		name := "without_number_option"
		if withNumberConfigOption {
			name = "with_number_option"
		}
		t.Run(name, func(t *testing.T) {
			if withNumberConfigOption {
				UseNumberWithJSONDecoderEncoder(withNumberConfigOption)
				defer UseNumberWithJSONDecoderEncoder(!withNumberConfigOption)
			}
			// Write rows into table first using DML.
			statements := make([]Statement, 0)
			for i, test := range tests {
				stmt := NewStatement(fmt.Sprintf("INSERT INTO Types (RowId, `%s`) VALUES (@id, @value)", test.col))
				// Note: We are not setting the parameter type here to ensure that it
				// can be automatically recognized when it is actually needed.
				stmt.Params["id"] = i
				stmt.Params["value"] = test.val
				statements = append(statements, stmt)
			}
			_, err := client.ReadWriteTransaction(ctx, func(ctx context.Context, tx *ReadWriteTransaction) error {
				rowCounts, err := tx.BatchUpdate(ctx, statements)
				if err != nil {
					return err
				}
				if len(rowCounts) != len(tests) {
					return fmt.Errorf("rowCounts length mismatch\nGot: %v\nWant: %v", len(rowCounts), len(tests))
				}
				for i, c := range rowCounts {
					if c != 1 {
						return fmt.Errorf("row count mismatch for row %v:\nGot: %v\nWant: %v", i, c, 1)
					}
				}
				return nil
			})
			if err != nil {
				t.Fatalf("failed to insert values using DML: %v", err)
			}
			// Delete all the rows so we can insert them using mutations as well.
			_, err = client.Apply(ctx, []*Mutation{Delete("Types", AllKeys())})
			if err != nil {
				t.Fatalf("failed to delete all rows: %v", err)
			}

			// Verify that we can insert the rows using mutations.
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
				want := test.wantWithDefaultConfig
				if withNumberConfigOption {
					want = test.wantWithNumber
				}
				if want == nil {
					want = test.val
				}
				gotp := reflect.New(reflect.TypeOf(want))
				if err := row.Column(0, gotp.Interface()); err != nil {
					t.Errorf("%v-%d: col:%v val:%#v, %v", withNumberConfigOption, i, test.col, test.val, err)
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
					t.Errorf("%v-%d: col:%v val:%#v, got %#v, want %#v", withNumberConfigOption, i, test.col, test.val, got, want)
					continue
				}
			}
			// cleanup
			_, err = client.Apply(ctx, []*Mutation{Delete("Types", AllKeys())})
			if err != nil {
				t.Fatal(err)
			}
		})
	}
}

// TODO: Rename it to `TestIntegration_BasicTypes` and remove the current one once UUID is enabled in production.
// Test encoding/decoding non-struct Cloud Spanner types.
func TestIntegration_UUID(t *testing.T) {
	t.Parallel()
	skipUnsupportedPGTest(t)
	onlyRunOnCloudDevel(t)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()
	stmts := []string{
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
					NumericArray	ARRAY<NUMERIC>,
					JSON		JSON,
					JSONArray	ARRAY<JSON>,
					UUID 		UUID,
					UUIDArray	Array<UUID>,
				) PRIMARY KEY (RowID)`,
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

	type Message struct {
		Name       string
		Body       string
		Time       int64
		FloatValue interface{}
	}
	msg := Message{"Alice", "Hello", 145688415796432520, json.Number("0.39240506000000003")}
	unmarshalledJSONStructUsingNumber := map[string]interface{}{
		"Name":       "Alice",
		"Body":       "Hello",
		"Time":       json.Number("145688415796432520"),
		"FloatValue": json.Number("0.39240506000000003"),
	}
	unmarshalledJSONStruct := map[string]interface{}{
		"Name":       "Alice",
		"Body":       "Hello",
		"Time":       1.456884157964325e+17,
		"FloatValue": 0.39240506,
	}

	tests := []struct {
		col                   string
		val                   interface{}
		wantWithDefaultConfig interface{}
		wantWithNumber        interface{}
	}{
		{col: "String", val: ""},
		{col: "String", val: "", wantWithDefaultConfig: NullString{"", true}, wantWithNumber: NullString{"", true}},
		{col: "String", val: "foo"},
		{col: "String", val: "foo", wantWithDefaultConfig: NullString{"foo", true}, wantWithNumber: NullString{"foo", true}},
		{col: "String", val: NullString{"bar", true}, wantWithDefaultConfig: "bar", wantWithNumber: "bar"},
		{col: "String", val: NullString{"bar", false}, wantWithDefaultConfig: NullString{"", false}, wantWithNumber: NullString{"", false}},
		{col: "StringArray", val: []string(nil), wantWithDefaultConfig: []NullString(nil), wantWithNumber: []NullString(nil)},
		{col: "StringArray", val: []string{}, wantWithDefaultConfig: []NullString{}, wantWithNumber: []NullString{}},
		{col: "StringArray", val: []string{"foo", "bar"}, wantWithDefaultConfig: []NullString{{"foo", true}, {"bar", true}}, wantWithNumber: []NullString{{"foo", true}, {"bar", true}}},
		{col: "StringArray", val: []NullString(nil)},
		{col: "StringArray", val: []NullString{}},
		{col: "StringArray", val: []NullString{{"foo", true}, {}}},
		{col: "Bytes", val: []byte{}},
		{col: "Bytes", val: []byte{1, 2, 3}},
		{col: "Bytes", val: []byte(nil)},
		{col: "BytesArray", val: [][]byte(nil)},
		{col: "BytesArray", val: [][]byte{}},
		{col: "BytesArray", val: [][]byte{{1}, {2, 3}}},
		{col: "Int64a", val: 0, wantWithDefaultConfig: int64(0), wantWithNumber: int64(0)},
		{col: "Int64a", val: -1, wantWithDefaultConfig: int64(-1), wantWithNumber: int64(-1)},
		{col: "Int64a", val: 2, wantWithDefaultConfig: int64(2), wantWithNumber: int64(2)},
		{col: "Int64a", val: int64(3)},
		{col: "Int64a", val: 4, wantWithDefaultConfig: NullInt64{4, true}, wantWithNumber: NullInt64{4, true}},
		{col: "Int64a", val: NullInt64{5, true}, wantWithDefaultConfig: int64(5), wantWithNumber: int64(5)},
		{col: "Int64a", val: NullInt64{6, true}, wantWithDefaultConfig: int64(6), wantWithNumber: int64(6)},
		{col: "Int64a", val: NullInt64{7, false}, wantWithDefaultConfig: NullInt64{0, false}, wantWithNumber: NullInt64{0, false}},
		{col: "Int64Array", val: []int(nil), wantWithDefaultConfig: []NullInt64(nil), wantWithNumber: []NullInt64(nil)},
		{col: "Int64Array", val: []int{}, wantWithDefaultConfig: []NullInt64{}, wantWithNumber: []NullInt64{}},
		{col: "Int64Array", val: []int{1, 2}, wantWithDefaultConfig: []NullInt64{{1, true}, {2, true}}, wantWithNumber: []NullInt64{{1, true}, {2, true}}},
		{col: "Int64Array", val: []int64(nil), wantWithDefaultConfig: []NullInt64(nil), wantWithNumber: []NullInt64(nil)},
		{col: "Int64Array", val: []int64{}, wantWithDefaultConfig: []NullInt64{}, wantWithNumber: []NullInt64{}},
		{col: "Int64Array", val: []int64{1, 2}, wantWithDefaultConfig: []NullInt64{{1, true}, {2, true}}, wantWithNumber: []NullInt64{{1, true}, {2, true}}},
		{col: "Int64Array", val: []NullInt64(nil)},
		{col: "Int64Array", val: []NullInt64{}},
		{col: "Int64Array", val: []NullInt64{{1, true}, {}}},
		{col: "Bool", val: false},
		{col: "Bool", val: true},
		{col: "Bool", val: false, wantWithDefaultConfig: NullBool{false, true}, wantWithNumber: NullBool{false, true}},
		{col: "Bool", val: true, wantWithDefaultConfig: NullBool{true, true}, wantWithNumber: NullBool{true, true}},
		{col: "Bool", val: NullBool{true, true}},
		{col: "Bool", val: NullBool{false, false}},
		{col: "BoolArray", val: []bool(nil), wantWithDefaultConfig: []NullBool(nil), wantWithNumber: []NullBool(nil)},
		{col: "BoolArray", val: []bool{}, wantWithDefaultConfig: []NullBool{}, wantWithNumber: []NullBool{}},
		{col: "BoolArray", val: []bool{true, false}, wantWithDefaultConfig: []NullBool{{true, true}, {false, true}}, wantWithNumber: []NullBool{{true, true}, {false, true}}},
		{col: "BoolArray", val: []NullBool(nil)},
		{col: "BoolArray", val: []NullBool{}},
		{col: "BoolArray", val: []NullBool{{false, true}, {true, true}, {}}},
		{col: "Float64", val: 0.0},
		{col: "Float64", val: 3.14},
		{col: "Float64", val: math.NaN()},
		{col: "Float64", val: math.Inf(1)},
		{col: "Float64", val: math.Inf(-1)},
		{col: "Float64", val: 2.78, wantWithDefaultConfig: NullFloat64{2.78, true}, wantWithNumber: NullFloat64{2.78, true}},
		{col: "Float64", val: NullFloat64{2.71, true}, wantWithDefaultConfig: 2.71, wantWithNumber: 2.71},
		{col: "Float64", val: NullFloat64{1.41, true}, wantWithDefaultConfig: NullFloat64{1.41, true}, wantWithNumber: NullFloat64{1.41, true}},
		{col: "Float64", val: NullFloat64{0, false}},
		{col: "Float64Array", val: []float64(nil), wantWithDefaultConfig: []NullFloat64(nil), wantWithNumber: []NullFloat64(nil)},
		{col: "Float64Array", val: []float64{}, wantWithDefaultConfig: []NullFloat64{}, wantWithNumber: []NullFloat64{}},
		{col: "Float64Array", val: []float64{2.72, 3.14, math.Inf(1)}, wantWithDefaultConfig: []NullFloat64{{2.72, true}, {3.14, true}, {math.Inf(1), true}}, wantWithNumber: []NullFloat64{{2.72, true}, {3.14, true}, {math.Inf(1), true}}},
		{col: "Float64Array", val: []NullFloat64(nil)},
		{col: "Float64Array", val: []NullFloat64{}},
		{col: "Float64Array", val: []NullFloat64{{2.72, true}, {math.Inf(1), true}, {}}},
		{col: "Date", val: d1},
		{col: "Date", val: d1, wantWithDefaultConfig: NullDate{d1, true}, wantWithNumber: NullDate{d1, true}},
		{col: "Date", val: NullDate{d1, true}},
		{col: "Date", val: NullDate{d1, true}, wantWithDefaultConfig: d1, wantWithNumber: d1},
		{col: "Date", val: NullDate{civil.Date{}, false}},
		{col: "DateArray", val: []civil.Date(nil), wantWithDefaultConfig: []NullDate(nil), wantWithNumber: []NullDate(nil)},
		{col: "DateArray", val: []civil.Date{}, wantWithDefaultConfig: []NullDate{}, wantWithNumber: []NullDate{}},
		{col: "DateArray", val: []civil.Date{d1, d2, d3}, wantWithDefaultConfig: []NullDate{{d1, true}, {d2, true}, {d3, true}}, wantWithNumber: []NullDate{{d1, true}, {d2, true}, {d3, true}}},
		{col: "Timestamp", val: t1},
		{col: "Timestamp", val: t1, wantWithDefaultConfig: NullTime{t1, true}, wantWithNumber: NullTime{t1, true}},
		{col: "Timestamp", val: NullTime{t1, true}},
		{col: "Timestamp", val: NullTime{t1, true}, wantWithDefaultConfig: t1, wantWithNumber: t1},
		{col: "Timestamp", val: NullTime{}},
		{col: "TimestampArray", val: []time.Time(nil), wantWithDefaultConfig: []NullTime(nil), wantWithNumber: []NullTime(nil)},
		{col: "TimestampArray", val: []time.Time{}, wantWithDefaultConfig: []NullTime{}, wantWithNumber: []NullTime{}},
		{col: "TimestampArray", val: []time.Time{t1, t2, t3}, wantWithDefaultConfig: []NullTime{{t1, true}, {t2, true}, {t3, true}}, wantWithNumber: []NullTime{{t1, true}, {t2, true}, {t3, true}}},
		{col: "Numeric", val: n1},
		{col: "Numeric", val: n2},
		{col: "Numeric", val: n1, wantWithDefaultConfig: NullNumeric{n1, true}, wantWithNumber: NullNumeric{n1, true}},
		{col: "Numeric", val: n2, wantWithDefaultConfig: NullNumeric{n2, true}, wantWithNumber: NullNumeric{n2, true}},
		{col: "Numeric", val: NullNumeric{n1, true}, wantWithDefaultConfig: n1, wantWithNumber: n1},
		{col: "Numeric", val: NullNumeric{n1, true}, wantWithDefaultConfig: NullNumeric{n1, true}, wantWithNumber: NullNumeric{n1, true}},
		{col: "Numeric", val: NullNumeric{n0, false}},
		{col: "NumericArray", val: []big.Rat(nil), wantWithDefaultConfig: []NullNumeric(nil), wantWithNumber: []NullNumeric(nil)},
		{col: "NumericArray", val: []big.Rat{}, wantWithDefaultConfig: []NullNumeric{}, wantWithNumber: []NullNumeric{}},
		{col: "NumericArray", val: []big.Rat{n1, n2}, wantWithDefaultConfig: []NullNumeric{{n1, true}, {n2, true}}, wantWithNumber: []NullNumeric{{n1, true}, {n2, true}}},
		{col: "NumericArray", val: []NullNumeric(nil)},
		{col: "NumericArray", val: []NullNumeric{}},
		{col: "NumericArray", val: []NullNumeric{{n1, true}, {n2, true}, {}}},
		{col: "JSON", val: NullJSON{msg, true}, wantWithDefaultConfig: NullJSON{unmarshalledJSONStruct, true}, wantWithNumber: NullJSON{unmarshalledJSONStructUsingNumber, true}},
		{col: "JSON", val: NullJSON{msg, false}, wantWithDefaultConfig: NullJSON{}, wantWithNumber: NullJSON{}},
		{col: "JSONArray", val: []NullJSON(nil)},
		{col: "JSONArray", val: []NullJSON{}},
		{col: "JSONArray", val: []NullJSON{{msg, true}, {msg, true}, {}}, wantWithDefaultConfig: []NullJSON{{unmarshalledJSONStruct, true}, {unmarshalledJSONStruct, true}, {}}, wantWithNumber: []NullJSON{{unmarshalledJSONStructUsingNumber, true}, {unmarshalledJSONStructUsingNumber, true}, {}}},
		{col: "String", val: nil, wantWithDefaultConfig: NullString{}, wantWithNumber: NullString{}},
		{col: "StringArray", val: nil, wantWithDefaultConfig: []NullString(nil), wantWithNumber: []NullString(nil)},
		{col: "Bytes", val: nil, wantWithDefaultConfig: []byte(nil), wantWithNumber: []byte(nil)},
		{col: "BytesArray", val: nil, wantWithDefaultConfig: [][]byte(nil), wantWithNumber: [][]byte(nil)},
		{col: "Int64a", val: nil, wantWithDefaultConfig: NullInt64{}, wantWithNumber: NullInt64{}},
		{col: "Int64Array", val: nil, wantWithDefaultConfig: []NullInt64(nil), wantWithNumber: []NullInt64(nil)},
		{col: "Bool", val: nil, wantWithDefaultConfig: NullBool{}, wantWithNumber: NullBool{}},
		{col: "BoolArray", val: nil, wantWithDefaultConfig: []NullBool(nil), wantWithNumber: []NullBool(nil)},
		{col: "Float64", val: nil, wantWithDefaultConfig: NullFloat64{}, wantWithNumber: NullFloat64{}},
		{col: "Float64Array", val: nil, wantWithDefaultConfig: []NullFloat64(nil), wantWithNumber: []NullFloat64(nil)},
		{col: "Numeric", val: nil, wantWithDefaultConfig: NullNumeric{}, wantWithNumber: NullNumeric{}},
		{col: "NumericArray", val: nil, wantWithDefaultConfig: []NullNumeric(nil), wantWithNumber: []NullNumeric(nil)},
		{col: "JSON", val: nil, wantWithDefaultConfig: NullJSON{}, wantWithNumber: NullJSON{}},
		{col: "JSONArray", val: nil, wantWithDefaultConfig: []NullJSON(nil), wantWithNumber: []NullJSON(nil)},
		{col: "UUID", val: uuid1},
		{col: "UUID", val: uuid1, wantWithDefaultConfig: NullUUID{uuid1, true}, wantWithNumber: NullUUID{uuid1, true}},
		{col: "UUID", val: NullUUID{uuid1, true}},
		{col: "UUID", val: NullUUID{uuid1, true}, wantWithDefaultConfig: uuid1, wantWithNumber: uuid1},
		{col: "UUID", val: NullUUID{uuid.UUID{}, false}},
		{col: "UUIDArray", val: []uuid.UUID(nil), wantWithDefaultConfig: []NullUUID(nil), wantWithNumber: []NullUUID(nil)},
		{col: "UUIDArray", val: []uuid.UUID{}, wantWithDefaultConfig: []NullUUID{}, wantWithNumber: []NullUUID{}},
		{col: "UUIDArray", val: []uuid.UUID{uuid1, uuid2}, wantWithDefaultConfig: []NullUUID{{uuid1, true}, {uuid2, true}}, wantWithNumber: []NullUUID{{uuid1, true}, {uuid2, true}}},
	}

	// See https://github.com/GoogleCloudPlatform/cloud-spanner-emulator/issues/31
	if !isEmulatorEnvSet() {
		tests = append(tests, []struct {
			col                   string
			val                   interface{}
			wantWithDefaultConfig interface{}
			wantWithNumber        interface{}
		}{
			{col: "Date", val: nil, wantWithDefaultConfig: NullDate{}, wantWithNumber: NullDate{}},
			{col: "Timestamp", val: nil, wantWithDefaultConfig: NullTime{}, wantWithNumber: NullTime{}},
		}...)
	}

	for _, withNumberConfigOption := range []bool{false, true} {
		name := "without_number_option"
		if withNumberConfigOption {
			name = "with_number_option"
		}
		t.Run(name, func(t *testing.T) {
			if withNumberConfigOption {
				UseNumberWithJSONDecoderEncoder(withNumberConfigOption)
				defer UseNumberWithJSONDecoderEncoder(!withNumberConfigOption)
			}
			// Write rows into table first using DML.
			statements := make([]Statement, 0)
			for i, test := range tests {
				stmt := NewStatement(fmt.Sprintf("INSERT INTO Types (RowId, `%s`) VALUES (@id, @value)", test.col))
				// Note: We are not setting the parameter type here to ensure that it
				// can be automatically recognized when it is actually needed.
				stmt.Params["id"] = i
				stmt.Params["value"] = test.val
				statements = append(statements, stmt)
			}
			_, err := client.ReadWriteTransaction(ctx, func(ctx context.Context, tx *ReadWriteTransaction) error {
				rowCounts, err := tx.BatchUpdate(ctx, statements)
				if err != nil {
					return err
				}
				if len(rowCounts) != len(tests) {
					return fmt.Errorf("rowCounts length mismatch\nGot: %v\nWant: %v", len(rowCounts), len(tests))
				}
				for i, c := range rowCounts {
					if c != 1 {
						return fmt.Errorf("row count mismatch for row %v:\nGot: %v\nWant: %v", i, c, 1)
					}
				}
				return nil
			})
			if err != nil {
				t.Fatalf("failed to insert values using DML: %v", err)
			}
			// Delete all the rows so we can insert them using mutations as well.
			_, err = client.Apply(ctx, []*Mutation{Delete("Types", AllKeys())})
			if err != nil {
				t.Fatalf("failed to delete all rows: %v", err)
			}

			// Verify that we can insert the rows using mutations.
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
				want := test.wantWithDefaultConfig
				if withNumberConfigOption {
					want = test.wantWithNumber
				}
				if want == nil {
					want = test.val
				}
				gotp := reflect.New(reflect.TypeOf(want))
				if err := row.Column(0, gotp.Interface()); err != nil {
					t.Errorf("%v-%d: col:%v val:%#v, %v", withNumberConfigOption, i, test.col, test.val, err)
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
					t.Errorf("%v-%d: col:%v val:%#v, got %#v, want %#v", withNumberConfigOption, i, test.col, test.val, got, want)
					continue
				}
			}
			// cleanup
			_, err = client.Apply(ctx, []*Mutation{Delete("Types", AllKeys())})
			if err != nil {
				t.Fatal(err)
			}
		})
	}
}

// Test encoding/decoding non-struct Cloud Spanner Proto or Array of Proto Column types.
func TestIntegration_BasicTypes_ProtoColumns(t *testing.T) {
	skipEmulatorTest(t)
	skipUnsupportedPGTest(t)
	t.Parallel()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()
	stmts := []string{
		`CREATE PROTO BUNDLE (
					examples.spanner.music.SingerInfo,
					examples.spanner.music.Genre,
				)`,
		`CREATE TABLE Types (
					RowID		INT64 NOT NULL,
					Int64a		INT64,
					Bytes		BYTES(MAX),
					Int64Array	ARRAY<INT64>,
					BytesArray	ARRAY<BYTES(MAX)>,
					ProtoMessage    examples.spanner.music.SingerInfo,
					ProtoEnum   examples.spanner.music.Genre,
					ProtoMessageArray   ARRAY<examples.spanner.music.SingerInfo>,
					ProtoEnumArray  ARRAY<examples.spanner.music.Genre>,
			) PRIMARY KEY (RowID)`,
	}

	protoDescriptor := readProtoDescriptorFile()
	client, _, cleanup := prepareIntegrationTestForProtoColumns(ctx, t, DefaultSessionPoolConfig, stmts, protoDescriptor)
	defer cleanup()

	singerProtoEnum := pb.Genre_ROCK
	singerProtoMessage := pb.SingerInfo{
		SingerId:    proto.Int64(1),
		BirthDate:   proto.String("January"),
		Nationality: proto.String("Country1"),
		Genre:       &singerProtoEnum,
	}
	singer2ProtoEnum := pb.Genre_FOLK
	singer2ProtoMessage := pb.SingerInfo{
		SingerId:    proto.Int64(1),
		BirthDate:   proto.String("January"),
		Nationality: proto.String("Country1"),
		Genre:       &singer2ProtoEnum,
	}
	bytesSingerProtoMessage, _ := proto.Marshal(&singerProtoMessage)

	tests := []struct {
		col  string
		val  interface{}
		want interface{}
	}{
		// Proto Message
		{col: "ProtoMessage", val: &singerProtoMessage, want: &singerProtoMessage},
		{col: "ProtoMessage", val: &singerProtoMessage, want: NullProtoMessage{&singerProtoMessage, true}},
		{col: "ProtoMessage", val: NullProtoMessage{&singerProtoMessage, true}, want: &singerProtoMessage},
		{col: "ProtoMessage", val: NullProtoMessage{&singerProtoMessage, true}, want: NullProtoMessage{&singerProtoMessage, true}},
		{col: "ProtoMessage", val: nil, want: NullProtoMessage{}},
		// Proto Enum
		{col: "ProtoEnum", val: pb.Genre_ROCK, want: singerProtoEnum},
		{col: "ProtoEnum", val: pb.Genre_ROCK, want: NullProtoEnum{&singerProtoEnum, true}},
		{col: "ProtoEnum", val: NullProtoEnum{pb.Genre_ROCK, true}, want: singerProtoEnum},
		{col: "ProtoEnum", val: NullProtoEnum{pb.Genre_ROCK, true}, want: NullProtoEnum{&singerProtoEnum, true}},
		{col: "ProtoEnum", val: nil, want: NullProtoEnum{}},
		// Test Compatibility between Int64 and ProtoEnum
		{col: "Int64a", val: pb.Genre_ROCK, want: int64(3)},
		{col: "Int64a", val: pb.Genre_ROCK, want: singerProtoEnum},
		{col: "Int64a", val: 3, want: singerProtoEnum},
		{col: "Int64a", val: int64(3)},
		{col: "ProtoEnum", val: int64(3), want: singerProtoEnum},
		{col: "ProtoEnum", val: int64(3), want: int64(3)},
		{col: "ProtoEnum", val: pb.Genre_ROCK, want: int64(3)},
		{col: "ProtoEnum", val: pb.Genre_ROCK, want: singerProtoEnum},
		// Test Compatibility between Bytes and ProtoMessage
		{col: "Bytes", val: &singerProtoMessage, want: bytesSingerProtoMessage},
		{col: "Bytes", val: &singerProtoMessage, want: &singerProtoMessage},
		{col: "Bytes", val: bytesSingerProtoMessage, want: &singerProtoMessage},
		{col: "Bytes", val: bytesSingerProtoMessage},
		{col: "ProtoMessage", val: bytesSingerProtoMessage, want: &singerProtoMessage},
		{col: "ProtoMessage", val: bytesSingerProtoMessage, want: bytesSingerProtoMessage},
		{col: "ProtoMessage", val: &singerProtoMessage, want: bytesSingerProtoMessage},
		{col: "ProtoMessage", val: &singerProtoMessage, want: &singerProtoMessage},
		// Test Compatibility between NullInt64 and NullProtoEnum
		{col: "Int64a", val: NullProtoEnum{pb.Genre_ROCK, true}, want: NullInt64{3, true}},
		{col: "Int64a", val: NullProtoEnum{pb.Genre_ROCK, true}, want: NullProtoEnum{&singerProtoEnum, true}},
		{col: "Int64a", val: NullInt64{3, true}, want: NullProtoEnum{&singerProtoEnum, true}},
		{col: "Int64a", val: NullInt64{3, false}, want: NullProtoEnum{}},
		{col: "ProtoEnum", val: NullInt64{3, true}, want: singerProtoEnum},
		{col: "ProtoEnum", val: NullInt64{3, true}, want: NullProtoEnum{&singerProtoEnum, true}},
		{col: "ProtoEnum", val: NullInt64{3, true}, want: NullInt64{3, true}},
		{col: "ProtoEnum", val: NullProtoEnum{pb.Genre_ROCK, true}, want: NullInt64{3, true}},
		// Test Compatibility between Bytes and NullProtoMessage
		{col: "Bytes", val: NullProtoMessage{&singerProtoMessage, true}, want: bytesSingerProtoMessage},
		{col: "Bytes", val: NullProtoMessage{&singerProtoMessage, true}, want: NullProtoMessage{&singerProtoMessage, true}},
		{col: "Bytes", val: bytesSingerProtoMessage, want: NullProtoMessage{&singerProtoMessage, true}},
		{col: "Bytes", val: bytesSingerProtoMessage},
		{col: "ProtoMessage", val: bytesSingerProtoMessage, want: NullProtoMessage{&singerProtoMessage, true}},
		{col: "ProtoMessage", val: []byte(nil), want: NullProtoMessage{}},
		{col: "ProtoMessage", val: NullProtoMessage{&singerProtoMessage, true}, want: bytesSingerProtoMessage},
		{col: "ProtoMessage", val: NullProtoMessage{&singerProtoMessage, true}, want: NullProtoMessage{&singerProtoMessage, true}},
		// Array of Proto Messages : Tests insert and read operations on ARRAY<PROTO> type column
		{col: "ProtoMessageArray", val: []*pb.SingerInfo{&singerProtoMessage, &singer2ProtoMessage}},
		{col: "ProtoMessageArray", val: []*pb.SingerInfo(nil)},
		{col: "ProtoMessageArray", val: []*pb.SingerInfo{}},
		// Array of Proto Enum : Tests insert and read operations on ARRAY<ENUM> type column
		// 1. Insert and Read data using Enum value array (ex: [enum1, enum2])
		{col: "ProtoEnumArray", val: []pb.Genre{pb.Genre_ROCK, pb.Genre_FOLK}, want: []pb.Genre{singerProtoEnum, singer2ProtoEnum}},
		{col: "ProtoEnumArray", val: []pb.Genre{}},
		{col: "ProtoEnumArray", val: []pb.Genre(nil)},
		// 2. Insert and Read data using Enum pointer array (ex: [*enum1, *enum2])
		{col: "ProtoEnumArray", val: []*pb.Genre{&singerProtoEnum, &singer2ProtoEnum}},
		{col: "ProtoEnumArray", val: []*pb.Genre{}},
		{col: "ProtoEnumArray", val: []*pb.Genre(nil)},
		// 3. Insert data using Enum value array (ex: [enum1, enum2]), Read data using Enum pointer array (ex: [*enum1, *enum2])
		{col: "ProtoEnumArray", val: []pb.Genre{pb.Genre_ROCK, pb.Genre_FOLK}, want: []*pb.Genre{&singerProtoEnum, &singer2ProtoEnum}},
		{col: "ProtoEnumArray", val: []pb.Genre{}, want: []*pb.Genre{}},
		{col: "ProtoEnumArray", val: []pb.Genre(nil), want: []*pb.Genre(nil)},
		// 4. Insert data using Enum pointer array (ex: [*enum1, *enum2]), Read data using Enum value array (ex: [enum1, enum2])
		{col: "ProtoEnumArray", val: []*pb.Genre{&singerProtoEnum, &singer2ProtoEnum}, want: []pb.Genre{singerProtoEnum, singer2ProtoEnum}},
		{col: "ProtoEnumArray", val: []*pb.Genre{}, want: []pb.Genre{}},
		{col: "ProtoEnumArray", val: []*pb.Genre(nil), want: []pb.Genre(nil)},
		// Tests Compatibility between Array of Int64 and Array of ProtoEnum type
		{col: "ProtoEnumArray", val: []int64{3, 2}, want: []pb.Genre{singerProtoEnum, singer2ProtoEnum}},
		{col: "ProtoEnumArray", val: []int64{3, 2}, want: []*pb.Genre{&singerProtoEnum, &singer2ProtoEnum}},
		{col: "ProtoEnumArray", val: []int64(nil), want: []pb.Genre(nil)},
		{col: "ProtoEnumArray", val: []int64{}, want: []pb.Genre{}},
		{col: "ProtoEnumArray", val: []pb.Genre{singerProtoEnum, singer2ProtoEnum}, want: []int64{3, 2}},
		{col: "ProtoEnumArray", val: []pb.Genre{}, want: []int64{}},
		{col: "ProtoEnumArray", val: []*pb.Genre{&singerProtoEnum, &singer2ProtoEnum}, want: []int64{3, 2}},
		{col: "ProtoEnumArray", val: []*pb.Genre(nil), want: []int64(nil)},
		{col: "ProtoEnumArray", val: []*pb.Genre{}, want: []int64{}},
		{col: "ProtoEnumArray", val: []*pb.Genre{&singerProtoEnum, &singer2ProtoEnum, nil}, want: []NullInt64{{3, true}, {2, true}, {}}},
		{col: "Int64Array", val: []int64{3, 2}, want: []pb.Genre{singerProtoEnum, singer2ProtoEnum}},
		{col: "Int64Array", val: []int64{3, 2}, want: []*pb.Genre{&singerProtoEnum, &singer2ProtoEnum}},
		{col: "Int64Array", val: []pb.Genre{singerProtoEnum, singer2ProtoEnum}, want: []int64{3, 2}},
		{col: "Int64Array", val: []*pb.Genre{&singerProtoEnum, &singer2ProtoEnum}, want: []int64{3, 2}},
		{col: "Int64Array", val: []pb.Genre(nil), want: []int64(nil)},
		// Tests Compatibility between Array of Bytes and Array of ProtoMessages type
		{col: "ProtoMessageArray", val: []*pb.SingerInfo{&singerProtoMessage}, want: [][]byte{bytesSingerProtoMessage}},
		{col: "ProtoMessageArray", val: [][]byte{bytesSingerProtoMessage}},
		{col: "ProtoMessageArray", val: [][]byte{bytesSingerProtoMessage}, want: []*pb.SingerInfo{&singerProtoMessage}},
		{col: "ProtoMessageArray", val: [][]byte(nil), want: []*pb.SingerInfo(nil)},
		{col: "ProtoMessageArray", val: []*pb.SingerInfo(nil), want: [][]byte(nil)},
		{col: "BytesArray", val: []*pb.SingerInfo{&singerProtoMessage}},
		{col: "BytesArray", val: []*pb.SingerInfo{&singerProtoMessage}, want: [][]byte{bytesSingerProtoMessage}},
		{col: "BytesArray", val: [][]byte{bytesSingerProtoMessage}, want: []*pb.SingerInfo{&singerProtoMessage}},
		{col: "BytesArray", val: [][]byte(nil), want: []*pb.SingerInfo(nil)},
		{col: "BytesArray", val: []*pb.SingerInfo(nil), want: [][]byte(nil)},
		// Null elements in Array of Proto Messages : Tests insert and read operations on ARRAY<PROTO> type column
		{col: "ProtoMessageArray", val: []*pb.SingerInfo{nil, nil}},
		{col: "ProtoMessageArray", val: []*pb.SingerInfo{nil, nil, &singerProtoMessage, &singer2ProtoMessage}},
		// Null elements in Array of Proto Enum : Tests insert and read operations on ARRAY<ENUM> type column
		{col: "ProtoEnumArray", val: []*pb.Genre{nil, nil}},
		{col: "ProtoEnumArray", val: []*pb.Genre{nil, nil, &singerProtoEnum, &singer2ProtoEnum}},
		// Tests Compatibility between Array of Bytes and Array of ProtoMessages type with null values
		{col: "ProtoMessageArray", val: []*pb.SingerInfo{&singerProtoMessage, nil}, want: [][]byte{bytesSingerProtoMessage, nil}},
		{col: "ProtoMessageArray", val: [][]byte{bytesSingerProtoMessage, nil}, want: []*pb.SingerInfo{&singerProtoMessage, nil}},
		{col: "BytesArray", val: []*pb.SingerInfo{&singerProtoMessage, nil}, want: [][]byte{bytesSingerProtoMessage, nil}},
		{col: "BytesArray", val: [][]byte{bytesSingerProtoMessage, nil}, want: []*pb.SingerInfo{&singerProtoMessage, nil}},
		// Tests Compatibility between Array of Int64 and Array of ProtoEnum type with null values
		{col: "ProtoEnumArray", val: []NullInt64{{3, true}, {}}, want: []*pb.Genre{&singerProtoEnum, nil}},
		{col: "ProtoEnumArray", val: []*pb.Genre{&singerProtoEnum, nil}, want: []NullInt64{{3, true}, {}}},
		{col: "Int64Array", val: []NullInt64{{3, true}, {}}, want: []*pb.Genre{&singerProtoEnum, nil}},
		{col: "Int64Array", val: []*pb.Genre{&singerProtoEnum, nil}, want: []NullInt64{{3, true}, {}}},
	}

	// Write rows into table first using DML.
	statements := make([]Statement, 0)
	for i, test := range tests {
		stmt := NewStatement(fmt.Sprintf("INSERT INTO Types (RowId, `%s`) VALUES (@id, @value)", test.col))
		// Note: We are not setting the parameter type here to ensure that it
		// can be automatically recognized when it is actually needed.
		stmt.Params["id"] = i
		stmt.Params["value"] = test.val
		statements = append(statements, stmt)
	}

	// Delete all the rows so we can insert them using mutations as well.
	_, err := client.Apply(ctx, []*Mutation{Delete("Types", AllKeys())})
	if err != nil {
		t.Fatalf("failed to delete all rows: %v", err)
	}

	// Verify that we can insert the rows using mutations.
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

		var gotp reflect.Value
		switch want.(type) {
		case proto.Message:
			// We are passing a pointer of proto message in `want` due to `go vet` issue.
			// Through the switch case, we are dereferencing the value so that we get proto message instead of its pointer.
			gotp = reflect.New(reflect.TypeOf(want).Elem())
		default:
			gotp = reflect.New(reflect.TypeOf(want))
		}
		v := gotp.Interface()

		switch nullValue := v.(type) {
		case *NullProtoMessage:
			nullValue.ProtoMessageVal = &pb.SingerInfo{}
		case *NullProtoEnum:
			var singerProtoEnumDefault pb.Genre
			nullValue.ProtoEnumVal = &singerProtoEnumDefault
		default:
		}

		if err := row.Column(0, v); err != nil {
			t.Errorf("%d: col:%v val:%#v, %v", i, test.col, test.val, err)
			continue
		}
		got := reflect.Indirect(gotp).Interface()

		// One of the test cases is checking NaN handling.  Given
		// NaN!=NaN, we can't use reflect to test for it.
		if isNaN(got) && isNaN(want) {
			continue
		}

		switch v.(type) {
		case proto.Message:
			if diff := cmp.Diff(got, test.want, protocmp.Transform()); diff != "" {
				t.Errorf("unexpected difference:\n%v", diff)
				continue
			}
		default:
			if !testEqual(got, want) {
				t.Errorf("%d: col:%v val:%#v, got %#v, want %#v", i, test.col, test.val, got, want)
				continue
			}
		}
	}
}

// Test errors during decoding non-struct Cloud Spanner Proto or Array of Proto Column types.
func TestIntegration_BasicTypes_ProtoColumns_Errors(t *testing.T) {
	skipEmulatorTest(t)
	skipUnsupportedPGTest(t)
	t.Parallel()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()
	stmts := []string{
		`CREATE PROTO BUNDLE (
					examples.spanner.music.SingerInfo,
					examples.spanner.music.Genre,
				)`,
		`CREATE TABLE Types (
					RowID		INT64 NOT NULL,
					Int64a		INT64,
					Bytes		BYTES(MAX),
					Int64Array	ARRAY<INT64>,
					BytesArray	ARRAY<BYTES(MAX)>,
					ProtoMessage    examples.spanner.music.SingerInfo,
					ProtoEnum   examples.spanner.music.Genre,
					ProtoMessageArray   ARRAY<examples.spanner.music.SingerInfo>,
					ProtoEnumArray  ARRAY<examples.spanner.music.Genre>,
			) PRIMARY KEY (RowID)`,
	}
	protoDescriptor := readProtoDescriptorFile()
	client, _, cleanup := prepareIntegrationTestForProtoColumns(ctx, t, DefaultSessionPoolConfig, stmts, protoDescriptor)
	defer cleanup()

	protoMessageNilError := "*protos.SingerInfo cannot support NULL SQL values"
	protoEnumNilError := "*protos.Genre cannot support NULL SQL values"

	tests := []struct {
		col       string
		val       interface{}
		want      interface{}
		wantCode  codes.Code
		errString string
	}{
		// Proto Message : Tests read operation on PROTO type column that has untyped/typed nil
		{col: "ProtoMessage", val: (*pb.SingerInfo)(nil), want: pb.SingerInfo{}, wantCode: codes.InvalidArgument, errString: protoMessageNilError},
		// Proto Enum : Tests read operation on ENUM type column that has untyped/typed nil
		{col: "ProtoEnum", val: (*pb.Genre)(nil), want: pb.Genre_POP, wantCode: codes.InvalidArgument, errString: protoEnumNilError},
	}

	// Delete all the rows.
	_, err := client.Apply(ctx, []*Mutation{Delete("Types", AllKeys())})
	if err != nil {
		t.Fatalf("failed to delete all rows: %v", err)
	}

	// Insert the rows using mutations.
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
		v := gotp.Interface()

		switch nullValue := v.(type) {
		case *NullProtoMessage:
			nullValue.ProtoMessageVal = &pb.SingerInfo{}
		case *NullProtoEnum:
			var singerProtoEnumDefault pb.Genre
			nullValue.ProtoEnumVal = &singerProtoEnumDefault
		default:
		}
		err = row.Column(0, v)
		if err == nil {
			t.Errorf("Column: %s, expected error %v during decoding, got %v", test.col, test.errString, err)
			continue
		}
		if msg, ok := matchError(err, test.wantCode, test.errString); !ok {
			t.Fatal(msg)
		}
	}
}

/*
The schema for Table Singers and Index SingerByNationalityAndGenre for Proto column integration tests is there in the test below.
*/
// Test DML, Parameterized query, Primary key and Indexing on Proto columns
func TestIntegration_ProtoColumns_DML_ParameterizedQueries_Pk_Indexes(t *testing.T) {
	skipEmulatorTest(t)
	skipUnsupportedPGTest(t)
	t.Parallel()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()
	stmts := []string{
		`CREATE PROTO BUNDLE (
					examples.spanner.music.SingerInfo,
					examples.spanner.music.Genre,
				)`,
		`CREATE TABLE Singers (
				 SingerId   INT64 NOT NULL,
				 FirstName  STRING(1024),
				 LastName   STRING(1024),
				 SingerInfo examples.spanner.music.SingerInfo,
				 SingerGenre examples.spanner.music.Genre,
				 SingerNationality STRING(1024) AS (SingerInfo.nationality) STORED,
				) PRIMARY KEY (SingerNationality, SingerGenre)`,
		`CREATE INDEX SingerByNationalityAndGenre ON Singers(SingerNationality, SingerGenre) STORING (SingerId, FirstName, LastName)`,
	}
	protoDescriptor := readProtoDescriptorFile()
	client, _, cleanup := prepareIntegrationTestForProtoColumns(ctx, t, DefaultSessionPoolConfig, stmts, protoDescriptor)
	defer cleanup()

	singerProtoEnum := pb.Genre_ROCK
	singerProtoMessage := pb.SingerInfo{
		SingerId:    proto.Int64(1),
		BirthDate:   proto.String("January"),
		Nationality: proto.String("Country1"),
		Genre:       &singerProtoEnum,
	}
	singer2ProtoEnum := pb.Genre_FOLK
	singer2ProtoMessage := pb.SingerInfo{
		SingerId:    proto.Int64(2),
		BirthDate:   proto.String("February"),
		Nationality: proto.String("Country2"),
		Genre:       &singer2ProtoEnum,
	}
	singer3ProtoEnum := pb.Genre_JAZZ
	singer3ProtoMessage := pb.SingerInfo{
		SingerId:    proto.Int64(3),
		BirthDate:   proto.String("March"),
		Nationality: proto.String("Country3"),
		Genre:       &singer3ProtoEnum,
	}

	for i, test := range []struct {
		sql    string
		params map[string]interface{}
	}{
		{sql: "INSERT INTO Singers (SingerId, FirstName, LastName, SingerInfo, SingerGenre) VALUES (@singerId, @firstName, @lastName, @singerInfo, @singerGenre)",
			params: map[string]interface{}{
				"singerId":    1,
				"firstName":   "Singer1",
				"lastName":    "Singer1",
				"singerInfo":  &singerProtoMessage,
				"singerGenre": singerProtoEnum,
			},
		}, {sql: "INSERT INTO Singers (SingerId, FirstName, LastName, SingerInfo, SingerGenre) VALUES (@singerId, @firstName, @lastName, @singerInfo, @singerGenre)",
			params: map[string]interface{}{
				"singerId":    2,
				"firstName":   "Singer2",
				"lastName":    "Singer2",
				"singerInfo":  &singer2ProtoMessage,
				"singerGenre": singer2ProtoEnum,
			},
		}, {sql: "INSERT INTO Singers (SingerId, FirstName, LastName, SingerInfo, SingerGenre) VALUES (@singerId, @firstName, @lastName, @singerInfo, @singerGenre)",
			params: map[string]interface{}{
				"singerId":    3,
				"firstName":   "Singer3",
				"lastName":    "Singer3",
				"singerInfo":  &singer3ProtoMessage,
				"singerGenre": singer3ProtoEnum,
			},
		},
	} {
		_, err := client.ReadWriteTransaction(ctx, func(ctx context.Context, tx *ReadWriteTransaction) error {
			count, err := tx.Update(ctx, Statement{
				SQL:    test.sql,
				Params: test.params,
			})
			if err != nil {
				return err
			}
			if count != 1 {
				t.Errorf("row count: got %d, want 1", count)
			}
			return nil
		})
		if err != nil {
			t.Fatalf("%d: failed to insert values using DML: %v", i, err)
		}
	}

	ro := client.ReadOnlyTransaction()
	defer ro.Close()

	for i, test := range []struct {
		sql    string
		params map[string]interface{}
		want   [][]interface{}
	}{
		// Filter rows using Proto Enum Parameterized query
		{sql: "SELECT SingerId, FirstName, LastName, SingerInfo, SingerGenre FROM Singers WHERE SingerGenre=@genre",
			params: map[string]interface{}{
				"genre": pb.Genre_FOLK,
			},
			want: [][]interface{}{{int64(2), "Singer2", "Singer2"}},
		},
		// Filter rows using Proto Message Field Parameterized query
		{sql: "SELECT SingerId, FirstName, LastName, SingerInfo, SingerGenre FROM Singers WHERE SingerInfo.Nationality=@country",
			params: map[string]interface{}{
				"country": "Country3",
			},
			want: [][]interface{}{{int64(3), "Singer3", "Singer3"}},
		},
	} {
		got, err := readAll(ro.Query(
			ctx,
			Statement{
				SQL:    test.sql,
				Params: test.params,
			}))
		if err != nil {
			t.Errorf("%d: Parameterized query returns error %v, want nil", i, err)
		}
		if !testEqual(got, test.want) {
			t.Errorf("%d: got unexpected result from Parameterized query: %v, want %v", i, got, test.want)
		}
	}

	// Read all rows based on Proto Message field and Proto Enum Primary key column values
	got, err := readAll(ro.Read(ctx, "Singers", KeySets(Key{"Country1", pb.Genre_ROCK}, Key{"Country3", pb.Genre_JAZZ}), []string{"SingerId", "FirstName", "LastName"}))
	if err != nil {
		t.Errorf("Reading rows from Primary key values returns error %v, want nil", err)
	}
	want := [][]interface{}{{int64(1), "Singer1", "Singer1"}, {int64(3), "Singer3", "Singer3"}}
	if !testEqual(got, want) {
		t.Errorf("got unexpected result while reading rows from Primary key values: %v, want %v", got, want)
	}

	// Read rows using Index on Proto Message field and Proto Enum column
	got, err = readAll(ro.ReadUsingIndex(ctx, "Singers", "SingerByNationalityAndGenre", KeySets(Key{"Country1", pb.Genre_ROCK}, Key{"Country2", pb.Genre_FOLK}), []string{"SingerId", "FirstName", "LastName"}))
	if err != nil {
		t.Errorf("ReadUsingIndex on Proto Enum column returns error %v, want nil", err)
	}
	// The results from ReadUsingIndex is sorted by the index rather than
	// primary key.
	want = [][]interface{}{{int64(1), "Singer1", "Singer1"}, {int64(2), "Singer2", "Singer2"}}
	if !testEqual(got, want) {
		t.Errorf("got unexpected result from ReadUsingIndex on Proto Enum column: %v, want %v", got, want)
	}

	// Delete all the rows in the Singers table.
	_, err = client.Apply(ctx, []*Mutation{Delete("Singers", AllKeys())})
	if err != nil {
		t.Fatalf("failed to delete all rows: %v", err)
	}
}

// Reads Proto descriptor file that has schema of proto messages and proto enum
func readProtoDescriptorFile() []byte {
	protoDescriptor, err := os.ReadFile(filepath.Join("testdata", "protos", "descriptors.pb"))
	if err != nil {
		panic(err)
	}
	return protoDescriptor
}

// Test decoding Cloud Spanner STRUCT type.
func TestIntegration_StructTypes(t *testing.T) {
	t.Parallel()
	skipUnsupportedPGTest(t)

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
					return errors.New("rows[0] is NULL")
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
		t.Run(strconv.Itoa(i), func(t *testing.T) {
			iter := client.Single().Query(ctx, test.q)
			defer iter.Stop()
			row, err := iter.Next()
			if err != nil {
				t.Errorf("%v", err)
				return
			}
			if err := test.want(row); err != nil {
				t.Errorf("%v", err)
				return
			}
		})
	}
}

func TestIntegration_StructParametersUnsupported(t *testing.T) {
	skipEmulatorTest(t)
	t.Parallel()
	skipUnsupportedPGTest(t)

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
	skipUnsupportedPGTest(t)

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
		t.Run(test.expr, func(t *testing.T) {
			iter := client.Single().Query(ctx, Statement{SQL: "SELECT " + test.expr})
			defer iter.Stop()
			row, err := iter.Next()
			if err != nil {
				t.Errorf("%v", err)
				return
			}
			// Create new instance of type of test.want.
			gotp := reflect.New(reflect.TypeOf(test.want))
			if err := row.Column(0, gotp.Interface()); err != nil {
				t.Errorf("Column returned error %v", err)
				return
			}
			got := reflect.Indirect(gotp).Interface()
			// TODO(jba): remove isNaN special case when we have a better equality predicate.
			if isNaN(got) && isNaN(test.want) {
				return
			}
			if !testEqual(got, test.want) {
				t.Errorf("wrong result\n got  %#v\nwant %#v", got, test.want)
			}
		})
	}
}

func TestIntegration_QueryStats(t *testing.T) {
	skipEmulatorTest(t)
	t.Parallel()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()
	client, _, cleanup := prepareIntegrationTest(ctx, t, DefaultSessionPoolConfig, statements[testDialect][singerDDLStatements])
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
	c, err := createClient(ctx, dbPath, ClientConfig{SessionPoolConfig: SessionPoolConfig{}})
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
	client, _, cleanup := prepareIntegrationTest(ctx, t, DefaultSessionPoolConfig, statements[testDialect][readDDLStatements])
	defer cleanup()

	var ms []*Mutation
	for i := 0; i < 2; i++ {
		ms = append(ms, InsertOrUpdate(testTable,
			testTableColumns,
			[]interface{}{fmt.Sprintf("k%d", i), "v"}))
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
	errorMessage := "unrecognized name"
	if testDialect == adminpb.DatabaseDialect_POSTGRESQL {
		errorMessage = "does not exist"
	}
	if msg, ok := matchError(err, codes.InvalidArgument, errorMessage); !ok {
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
	// TODO(sakthivelmani): Enable the tests once b/422916293 is fixed
	skipDirectPathTest(t)
	skipEmulatorTest(t)
	t.Parallel()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()
	client, _, cleanup := prepareIntegrationTest(ctx, t, DefaultSessionPoolConfig, statements[testDialect][singerDDLStatements])
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

func TestIntegration_QueryWithRoles(t *testing.T) {
	t.Parallel()
	// Database roles are not currently available in emulator
	skipEmulatorTest(t)

	// Set up testing environment.
	var (
		row                    *Row
		client, clientWithRole *Client
		iter                   *RowIterator
		err                    error
		id                     int64
		firstName, lastName    string
	)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()
	stmts := []string{
		`CREATE ROLE singers_reader`,
		`CREATE ROLE singers_unauthorized`,
		`CREATE ROLE singers_reader_revoked`,
		`CREATE ROLE dropped`,
		`DROP ROLE dropped`,
	}
	if testDialect == adminpb.DatabaseDialect_POSTGRESQL {
		stmts = append(stmts,
			`CREATE TABLE Singers (
					SingerId	BIGINT NOT NULL,
					FirstName	VARCHAR(1024),
					LastName	VARCHAR(1024),
					SingerInfo	BYTEA,
					PRIMARY KEY (SingerId)
				)`,
			`GRANT SELECT(SingerId, FirstName, LastName) ON Singers TO singers_reader`,
			`GRANT SELECT(SingerId, FirstName) ON TABLE Singers TO singers_unauthorized`,
			`GRANT SELECT(SingerId, FirstName, LastName) ON TABLE Singers TO singers_reader_revoked`,
			`REVOKE SELECT(LastName) ON TABLE Singers FROM singers_reader_revoked`)
	} else {
		stmts = append(stmts,
			`CREATE TABLE Singers (
					SingerId	INT64 NOT NULL,
					FirstName	STRING(1024),
					LastName	STRING(1024),
					SingerInfo	BYTES(MAX)
				) PRIMARY KEY (SingerId)`,
			`GRANT SELECT(SingerId, FirstName, LastName) ON TABLE Singers TO ROLE singers_reader`,
			`GRANT SELECT(SingerId, FirstName) ON TABLE Singers TO ROLE singers_unauthorized`,
			`GRANT SELECT(SingerId, FirstName, LastName) ON TABLE Singers TO ROLE singers_reader_revoked`,
			`REVOKE SELECT(LastName) ON TABLE Singers FROM ROLE singers_reader_revoked`)
	}
	client, dbPath, cleanup := prepareIntegrationTest(ctx, t, DefaultSessionPoolConfig, stmts)
	defer cleanup()

	singerColumns := []string{"SingerId", "FirstName", "LastName"}
	var ms = []*Mutation{
		InsertOrUpdate("Singers", singerColumns, []interface{}{1, "Marc", "Richards"}),
	}
	if _, err := client.Apply(ctx, ms, ApplyAtLeastOnce()); err != nil {
		t.Fatalf("Could not insert rows to table. Got error %v", err)
	}
	queryStmt := Statement{SQL: `SELECT SingerId, FirstName, LastName FROM Singers`}

	// A request with sufficient privileges should return all rows
	for _, dbRole := range []string{
		"",
		"singers_reader",
	} {
		t.Run("role:"+dbRole, func(t *testing.T) {
			if clientWithRole, err = createClientWithRole(ctx, dbPath, SessionPoolConfig{}, dbRole); err != nil {
				t.Fatal(err)
			}
			defer clientWithRole.Close()

			iter = clientWithRole.Single().Query(ctx, queryStmt)
			defer iter.Stop()

			row, err = iter.Next()
			if err != nil {
				t.Fatalf("Could not read row. Got error %v", err)
			}
			if err = row.Columns(&id, &firstName, &lastName); err != nil {
				t.Fatalf("failed to parse row %v", err)
			}
			if id != 1 || firstName != "Marc" || lastName != "Richards" {
				t.Fatalf("execution didn't return expected values")
			}

			_, err = iter.Next()
			if err != iterator.Done {
				t.Fatalf("got results from iterator, want none: %#v, err = %v\n", row, err)
			}
		})
	}

	// A request with insufficient privileges should return permission denied
	for _, test := range []struct {
		dbRole string
		errMsg string
	}{
		{
			"singers_unauthorized",
			"Role singers_unauthorized does not have required privileges on table Singers.",
		},
		{
			"singers_reader_revoked",
			"Role singers_reader_revoked does not have required privileges on table Singers.",
		},
		{
			"nonexistent",
			"Role not found: nonexistent.",
		},
		{
			"dropped",
			"Role not found: dropped.",
		},
	} {
		t.Run("role:"+test.dbRole, func(t *testing.T) {
			if clientWithRole, err = createClientWithRole(ctx, dbPath, SessionPoolConfig{}, test.dbRole); err != nil {
				t.Fatal(err)
			}
			defer clientWithRole.Close()
			iter = clientWithRole.Single().Query(ctx, queryStmt)
			defer iter.Stop()

			_, err = iter.Next()
			if err == nil {
				t.Fatal("expected err, got nil")
			}
			if msg, ok := matchError(err, codes.PermissionDenied, test.errMsg); !ok {
				t.Fatal(msg)
			}
		})
	}
}

func TestIntegration_ReadWithRoles(t *testing.T) {
	t.Parallel()
	// Database roles are not currently available in emulator
	skipEmulatorTest(t)

	// Set up testing environment.
	var (
		row                    *Row
		client, clientWithRole *Client
		iter                   *RowIterator
		err                    error
		id                     int64
		firstName, lastName    string
	)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()
	stmts := []string{
		`CREATE ROLE singers_reader`,
		`CREATE ROLE singers_unauthorized`,
		`CREATE ROLE singers_reader_revoked`,
		`CREATE ROLE dropped`,
		`DROP ROLE dropped`,
	}
	if testDialect == adminpb.DatabaseDialect_POSTGRESQL {
		stmts = append(stmts,
			`CREATE TABLE Singers (
					SingerId	BIGINT NOT NULL,
					FirstName	VARCHAR(1024),
					LastName	VARCHAR(1024),
					SingerInfo	BYTEA,
					PRIMARY KEY (SingerId)
				)`,
			`GRANT SELECT(SingerId, FirstName, LastName) ON Singers TO singers_reader`,
			`GRANT SELECT(SingerId, FirstName) ON TABLE Singers TO singers_unauthorized`,
			`GRANT SELECT(SingerId, FirstName, LastName) ON TABLE Singers TO singers_reader_revoked`,
			`REVOKE SELECT(LastName) ON TABLE Singers FROM singers_reader_revoked`)
	} else {
		stmts = append(stmts,
			`CREATE TABLE Singers (
					SingerId	INT64 NOT NULL,
					FirstName	STRING(1024),
					LastName	STRING(1024),
					SingerInfo	BYTES(MAX)
				) PRIMARY KEY (SingerId)`,
			`GRANT SELECT(SingerId, FirstName, LastName) ON TABLE Singers TO ROLE singers_reader`,
			`GRANT SELECT(SingerId, FirstName) ON TABLE Singers TO ROLE singers_unauthorized`,
			`GRANT SELECT(SingerId, FirstName, LastName) ON TABLE Singers TO ROLE singers_reader_revoked`,
			`REVOKE SELECT(LastName) ON TABLE Singers FROM ROLE singers_reader_revoked`)
	}
	client, dbPath, cleanup := prepareIntegrationTest(ctx, t, DefaultSessionPoolConfig, stmts)
	defer cleanup()

	singerColumns := []string{"SingerId", "FirstName", "LastName"}
	var ms = []*Mutation{
		InsertOrUpdate("Singers", singerColumns, []interface{}{1, "Marc", "Richards"}),
	}
	if _, err := client.Apply(ctx, ms, ApplyAtLeastOnce()); err != nil {
		t.Fatalf("Could not insert rows to table. Got error %v", err)
	}

	// A request with sufficient privileges should return all rows
	for _, dbRole := range []string{
		"",
		"singers_reader",
	} {
		t.Run("role:"+dbRole, func(t *testing.T) {
			if clientWithRole, err = createClientWithRole(ctx, dbPath, SessionPoolConfig{}, dbRole); err != nil {
				t.Fatal(err)
			}
			defer clientWithRole.Close()

			iter = clientWithRole.Single().Read(ctx, "Singers", AllKeys(), singerColumns)
			defer iter.Stop()

			row, err = iter.Next()
			if err != nil {
				t.Fatalf("Could not read row. Got error %v", err)
			}
			if err = row.Columns(&id, &firstName, &lastName); err != nil {
				t.Fatalf("failed to parse row %v", err)
			}
			if id != 1 || firstName != "Marc" || lastName != "Richards" {
				t.Fatalf("execution didn't return expected values")
			}

			_, err = iter.Next()
			if err != iterator.Done {
				t.Fatalf("got results from iterator, want none: %#v, err = %v\n", row, err)
			}
		})
	}

	// A request with insufficient privileges should return permission denied
	for _, test := range []struct {
		dbRole string
		errMsg string
	}{
		{
			"singers_unauthorized",
			"Role singers_unauthorized does not have required privileges on table Singers.",
		},
		{
			"singers_reader_revoked",
			"Role singers_reader_revoked does not have required privileges on table Singers.",
		},
		{
			"nonexistent",
			"Role not found: nonexistent.",
		},
		{
			"dropped",
			"Role not found: dropped.",
		},
	} {
		t.Run("role:"+test.dbRole, func(t *testing.T) {
			if clientWithRole, err = createClientWithRole(ctx, dbPath, SessionPoolConfig{}, test.dbRole); err != nil {
				t.Fatal(err)
			}
			defer clientWithRole.Close()
			iter = clientWithRole.Single().Read(ctx, "Singers", AllKeys(), singerColumns)
			defer iter.Stop()

			_, err = iter.Next()
			if err == nil {
				t.Fatal("expected err, got nil")
			}
			if msg, ok := matchError(err, codes.PermissionDenied, test.errMsg); !ok {
				t.Fatal(msg)
			}
		})
	}
}

func TestIntegration_DMLWithRoles(t *testing.T) {
	t.Parallel()
	// Database roles are not currently available in emulator
	skipEmulatorTest(t)

	// Set up testing environment.
	var (
		client, clientWithRole *Client
		err                    error
	)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()
	stmts := []string{
		`CREATE ROLE singers_updater`,
		`CREATE ROLE singers_unauthorized`,
		`CREATE ROLE singers_creator`,
		`CREATE ROLE singers_deleter`,
	}
	if testDialect == adminpb.DatabaseDialect_POSTGRESQL {
		stmts = append(stmts,
			`CREATE TABLE Singers (
					SingerId	BIGINT NOT NULL,
					FirstName	VARCHAR(1024),
					LastName	VARCHAR(1024),
					SingerInfo	BYTEA,
					PRIMARY KEY (SingerId)
				)`,
			`GRANT SELECT(SingerId), UPDATE(FirstName, LastName) ON Singers TO singers_updater`,
			`GRANT SELECT(SingerId), UPDATE(FirstName) ON TABLE Singers TO singers_unauthorized`,
			`GRANT INSERT(SingerId, FirstName, LastName) ON TABLE Singers TO singers_creator`,
			`GRANT SELECT(SingerId), DELETE ON TABLE Singers TO singers_deleter`)
	} else {
		stmts = append(stmts,
			`CREATE TABLE Singers (
					SingerId	INT64 NOT NULL,
					FirstName	STRING(1024),
					LastName	STRING(1024),
					SingerInfo	BYTES(MAX)
				) PRIMARY KEY (SingerId)`,
			`GRANT SELECT(SingerId), UPDATE(FirstName, LastName) ON TABLE Singers TO ROLE singers_updater`,
			`GRANT SELECT(SingerId), UPDATE(FirstName) ON TABLE Singers TO ROLE singers_unauthorized`,
			`GRANT INSERT(SingerId, FirstName, LastName) ON TABLE Singers TO ROLE singers_creator`,
			`GRANT SELECT(SingerId), DELETE ON TABLE Singers TO ROLE singers_deleter`)
	}
	client, dbPath, cleanup := prepareIntegrationTest(ctx, t, DefaultSessionPoolConfig, stmts)
	defer cleanup()

	singerColumns := []string{"SingerId", "FirstName", "LastName"}
	var ms = []*Mutation{
		InsertOrUpdate("Singers", singerColumns, []interface{}{1, "Marc", "Richards"}),
	}
	if _, err := client.Apply(ctx, ms, ApplyAtLeastOnce()); err != nil {
		t.Fatalf("Could not insert rows to table. Got error %v", err)
	}
	updateStmt := Statement{SQL: `UPDATE Singers SET FirstName = 'Mark', LastName = 'Richards' WHERE SingerId = 1`}

	// A request with sufficient privileges should update the row
	for _, dbRole := range []string{
		"",
		"singers_updater",
	} {
		t.Run("role:"+dbRole, func(t *testing.T) {
			if clientWithRole, err = createClientWithRole(ctx, dbPath, SessionPoolConfig{}, dbRole); err != nil {
				t.Fatal(err)
			}
			defer clientWithRole.Close()
			_, err = clientWithRole.ReadWriteTransaction(ctx, func(ctx context.Context, txn *ReadWriteTransaction) error {
				_, err := txn.Update(ctx, updateStmt)
				return err
			})
			if err != nil {
				t.Fatalf("Could not update row. Got error %v", err)
			}
		})
	}

	// A request with insufficient privileges should return permission denied
	for _, test := range []struct {
		dbRole string
		errMsg string
	}{
		{
			"singers_unauthorized",
			"Role singers_unauthorized does not have required privileges on table Singers.",
		},
		{
			"nonexistent",
			"Role not found: nonexistent.",
		},
	} {
		t.Run("role:"+test.dbRole, func(t *testing.T) {
			if clientWithRole, err = createClientWithRole(ctx, dbPath, SessionPoolConfig{}, test.dbRole); err != nil {
				t.Fatal(err)
			}
			defer clientWithRole.Close()
			_, err = clientWithRole.ReadWriteTransaction(ctx, func(ctx context.Context, txn *ReadWriteTransaction) error {
				_, err := txn.Update(ctx, updateStmt)
				return err
			})

			if err == nil {
				t.Fatal("expected err, got nil")
			}
			if msg, ok := matchError(err, codes.PermissionDenied, test.errMsg); !ok {
				t.Fatal(msg)
			}
		})
	}

	// A request with sufficient privileges should insert the row
	getInsertStmt := func(vals []interface{}) Statement {
		sql := fmt.Sprintf(`INSERT INTO Singers (SingerId, FirstName, LastName) VALUES (%d, '%s', '%s')`, vals...)
		return Statement{SQL: sql}
	}
	for _, test := range []struct {
		dbRole string
		vals   []interface{}
	}{
		{
			"",
			[]interface{}{2, "Catalina", "Smith"},
		},
		{
			"singers_creator",
			[]interface{}{3, "Alice", "Trentor"},
		},
	} {
		t.Run("role:"+test.dbRole, func(t *testing.T) {
			if clientWithRole, err = createClientWithRole(ctx, dbPath, SessionPoolConfig{}, test.dbRole); err != nil {
				t.Fatal(err)
			}
			defer clientWithRole.Close()
			_, err = clientWithRole.ReadWriteTransaction(ctx, func(ctx context.Context, txn *ReadWriteTransaction) error {
				_, err := txn.Update(ctx, getInsertStmt(test.vals))
				return err
			})
			if err != nil {
				t.Fatalf("Could not insert row. Got error %v", err)
			}
		})
	}

	// A request with sufficient privileges should delete the row
	deleteStmt := Statement{SQL: `DELETE FROM Singers WHERE TRUE`}
	for _, dbRole := range []string{
		"",
		"singers_deleter",
	} {
		t.Run("role:"+dbRole, func(t *testing.T) {
			if clientWithRole, err = createClientWithRole(ctx, dbPath, SessionPoolConfig{}, dbRole); err != nil {
				t.Fatal(err)
			}
			defer clientWithRole.Close()
			_, err = clientWithRole.ReadWriteTransaction(ctx, func(ctx context.Context, txn *ReadWriteTransaction) error {
				_, err := txn.Update(ctx, deleteStmt)
				return err
			})
			if err != nil {
				t.Fatalf("Could not delete row. Got error %v", err)
			}
		})
	}
}

func TestIntegration_MutationWithRoles(t *testing.T) {
	t.Parallel()
	// Database roles are not currently available in emulator
	skipEmulatorTest(t)

	// Set up testing environment.
	var (
		client, clientWithRole *Client
		err                    error
	)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()
	stmts := []string{
		`CREATE ROLE singers_updater`,
		`CREATE ROLE singers_unauthorized`,
		`CREATE ROLE singers_creator`,
		`CREATE ROLE singers_deleter`,
	}
	if testDialect == adminpb.DatabaseDialect_POSTGRESQL {
		stmts = append(stmts,
			`CREATE TABLE Singers (
					SingerId	BIGINT NOT NULL,
					FirstName	VARCHAR(1024),
					LastName	VARCHAR(1024),
					SingerInfo	BYTEA,
					PRIMARY KEY (SingerId)
				)`,
			`GRANT SELECT(SingerId), UPDATE(SingerId, FirstName, LastName) ON Singers TO singers_updater`,
			`GRANT SELECT(SingerId), UPDATE(SingerId, FirstName) ON TABLE Singers TO singers_unauthorized`,
			`GRANT INSERT(SingerId, FirstName, LastName) ON TABLE Singers TO singers_creator`,
			`GRANT SELECT(SingerId), DELETE ON TABLE Singers TO singers_deleter`)
	} else {
		stmts = append(stmts,
			`CREATE TABLE Singers (
					SingerId	INT64 NOT NULL,
					FirstName	STRING(1024),
					LastName	STRING(1024),
					SingerInfo	BYTES(MAX)
				) PRIMARY KEY (SingerId)`,
			`GRANT SELECT(SingerId), UPDATE(SingerId, FirstName, LastName) ON TABLE Singers TO ROLE singers_updater`,
			`GRANT SELECT(SingerId), UPDATE(SingerId, FirstName) ON TABLE Singers TO ROLE singers_unauthorized`,
			`GRANT INSERT(SingerId, FirstName, LastName) ON TABLE Singers TO ROLE singers_creator`,
			`GRANT SELECT(SingerId), DELETE ON TABLE Singers TO ROLE singers_deleter`)
	}
	client, dbPath, cleanup := prepareIntegrationTest(ctx, t, DefaultSessionPoolConfig, stmts)
	defer cleanup()

	singerColumns := []string{"SingerId", "FirstName", "LastName"}
	var ms = []*Mutation{
		InsertOrUpdate("Singers", singerColumns, []interface{}{1, "Marc", "Richards"}),
	}
	if _, err := client.Apply(ctx, ms, ApplyAtLeastOnce()); err != nil {
		t.Fatalf("Could not insert rows to table. Got error %v", err)
	}

	// A request with sufficient privileges should update the row
	for _, dbRole := range []string{
		"",
		"singers_updater",
	} {
		t.Run("role:"+dbRole, func(t *testing.T) {
			if clientWithRole, err = createClientWithRole(ctx, dbPath, SessionPoolConfig{}, dbRole); err != nil {
				t.Fatal(err)
			}
			defer clientWithRole.Close()
			_, err = clientWithRole.Apply(ctx, []*Mutation{
				Update("Singers", singerColumns, []interface{}{1, "Mark", "Richards"}),
			})
			if err != nil {
				t.Fatalf("Could not update row. Got error %v", err)
			}
		})
	}

	// A request with insufficient privileges should return permission denied
	for _, test := range []struct {
		dbRole string
		errMsg string
	}{
		{
			"singers_unauthorized",
			"Role singers_unauthorized does not have required privileges on table Singers.",
		},
		{
			"nonexistent",
			"Role not found: nonexistent.",
		},
	} {
		t.Run("role:"+test.dbRole, func(t *testing.T) {
			if clientWithRole, err = createClientWithRole(ctx, dbPath, SessionPoolConfig{}, test.dbRole); err != nil {
				t.Fatal(err)
			}
			defer clientWithRole.Close()
			_, err = clientWithRole.Apply(ctx, []*Mutation{
				Update("Singers", singerColumns, []interface{}{1, "Mark", "Richards"}),
			})

			if err == nil {
				t.Fatal("expected err, got nil")
			}
			if msg, ok := matchError(err, codes.PermissionDenied, test.errMsg); !ok {
				t.Fatal(msg)
			}
		})
	}

	// A request with sufficient privileges should insert the row
	for _, test := range []struct {
		dbRole string
		vals   []interface{}
	}{
		{
			"",
			[]interface{}{2, "Catalina", "Smith"},
		},
		{
			"singers_creator",
			[]interface{}{3, "Alice", "Trentor"},
		},
	} {
		t.Run("role:"+test.dbRole, func(t *testing.T) {
			if clientWithRole, err = createClientWithRole(ctx, dbPath, SessionPoolConfig{}, test.dbRole); err != nil {
				t.Fatal(err)
			}
			defer clientWithRole.Close()
			_, err = clientWithRole.Apply(ctx, []*Mutation{
				Insert("Singers", singerColumns, test.vals),
			})
			if err != nil {
				t.Fatalf("Could not insert row. Got error %v", err)
			}
		})
	}

	// A request with sufficient privileges should delete the row
	for _, dbRole := range []string{
		"",
		"singers_deleter",
	} {
		t.Run("role:"+dbRole, func(t *testing.T) {
			if clientWithRole, err = createClientWithRole(ctx, dbPath, SessionPoolConfig{}, dbRole); err != nil {
				t.Fatal(err)
			}
			defer clientWithRole.Close()
			_, err = clientWithRole.Apply(ctx, []*Mutation{
				Delete("Singers", Key{1}),
			})
			if err != nil {
				t.Fatalf("Could not delete row. Got error %v", err)
			}
		})
	}
}

func TestIntegration_ListDatabaseRoles(t *testing.T) {
	t.Parallel()
	// Database roles are not currently available in emulator
	skipEmulatorTest(t)

	// Set up testing environment.
	var (
		err  error
		iter *database.DatabaseRoleIterator
	)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()
	stmts := []string{
		`CREATE ROLE a`,
		`CREATE ROLE z`,
	}
	_, dbPath, cleanup := prepareIntegrationTest(ctx, t, DefaultSessionPoolConfig, stmts)
	defer cleanup()

	iter = databaseAdmin.ListDatabaseRoles(ctx, &adminpb.ListDatabaseRolesRequest{
		Parent: dbPath,
	})
	roles, err := readDatabaseRoles(iter)
	if err != nil {
		t.Fatalf("cannot list database roles in %v: %v", dbPath, err)
	}
	var got []string
	rolePrefix := dbPath + "/databaseRoles/"
	for _, role := range roles {
		if !strings.HasPrefix(role.Name, rolePrefix) {
			t.Fatalf("Role %v does not have prefix %v", role.Name, rolePrefix)
		}
		got = append(got, strings.TrimPrefix(role.Name, rolePrefix))
	}
	want := []string{"a", "public", "spanner_info_reader", "spanner_sys_reader", "z"}
	if !testEqual(got, want) {
		t.Fatalf("Database role mismatch\nGot: %v, Want: %v", got, want)
	}
}

func readDatabaseRoles(iter *database.DatabaseRoleIterator) ([]*adminpb.DatabaseRole, error) {
	var vals []*adminpb.DatabaseRole
	for {
		v, err := iter.Next()
		if err == iterator.Done {
			return vals, nil
		}
		if err != nil {
			return nil, err
		}
		vals = append(vals, v)
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
	client, dbPath, cleanup := prepareIntegrationTest(ctx, t, DefaultSessionPoolConfig, statements[testDialect][simpleDDLStatements])
	defer cleanup()

	if err = populate(ctx, client); err != nil {
		t.Fatal(err)
	}
	if client2, err = createClient(ctx, dbPath, ClientConfig{SessionPoolConfig: SessionPoolConfig{}}); err != nil {
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
	if partitions, err = txn.PartitionQueryWithOptions(ctx, stmt, PartitionOptions{0, 3}, QueryOptions{DataBoostEnabled: true}); err != nil {
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
		t.Run(strconv.Itoa(i), func(t *testing.T) {
			iter := txn.Execute(ctx, p)
			defer iter.Stop()

			p2 := serdesPartition(t, i, p)
			iter2 := txn2.Execute(ctx, &p2)
			defer iter2.Stop()

			row1, err1 := iter.Next()
			row2, err2 := iter2.Next()
			if err1 != err2 {
				t.Fatalf("execution failed for different reasons: %v, %v", err1, err2)
			}
			if !testEqual(row1, row2) {
				t.Fatalf("execution returned different values: %v, %v", row1, row2)
			}
			if row1 == nil {
				return
			}
			var a, b string
			if err = row1.Columns(&a, &b); err != nil {
				t.Fatalf("failed to parse row %v", err)
			}
			if a == str1 && b == str2 {
				gotResult = true
			}
		})
	}
	if !gotResult {
		t.Fatalf("execution didn't return expected values")
	}
}

// Test PartitionRead of BatchReadOnlyTransaction, similar to TestBatchQuery
func TestIntegration_BatchRead(t *testing.T) {
	t.Parallel()
	// skipping PG test because of rpc error: code = InvalidArgument desc = [ERROR] syntax error at or near "." in PartitionRead
	skipUnsupportedPGTest(t)
	// Set up testing environment.
	var (
		client2 *Client
		err     error
	)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()
	client, dbPath, cleanup := prepareIntegrationTest(ctx, t, DefaultSessionPoolConfig, statements[testDialect][simpleDDLStatements])
	defer cleanup()

	if err = populate(ctx, client); err != nil {
		t.Fatal(err)
	}
	if client2, err = createClient(ctx, dbPath, ClientConfig{SessionPoolConfig: SessionPoolConfig{}}); err != nil {
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
	if partitions, err = txn.PartitionReadWithOptions(ctx, "test", AllKeys(), simpleDBTableColumns, PartitionOptions{0, 3}, ReadOptions{DataBoostEnabled: true}); err != nil {
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
		t.Run(strconv.Itoa(i), func(t *testing.T) {
			iter := txn.Execute(ctx, p)
			defer iter.Stop()
			p2 := serdesPartition(t, i, p)
			iter2 := txn2.Execute(ctx, &p2)
			defer iter2.Stop()

			row1, err1 := iter.Next()
			row2, err2 := iter2.Next()
			if err1 != err2 {
				t.Fatalf("execution failed for different reasons: %v, %v", err1, err2)
				return
			}
			if !testEqual(row1, row2) {
				t.Fatalf("execution returned different values: %v, %v", row1, row2)
			}
			if row1 == nil {
				return
			}
			var a, b string
			if err = row1.Columns(&a, &b); err != nil {
				t.Fatalf("failed to parse row %v", err)
			}
			if a == str1 && b == str2 {
				gotResult = true
			}
		})
	}
	if !gotResult {
		t.Fatalf("execution didn't return expected values")
	}
}

// Test normal txReadEnv method on BatchReadOnlyTransaction.
func TestIntegration_BROTNormal(t *testing.T) {
	t.Parallel()
	// skipping PG test because of rpc error: code = InvalidArgument desc = [ERROR] syntax error at or near "." in PartitionRead
	skipUnsupportedPGTest(t)
	// Set up testing environment and create txn.
	var (
		txn *BatchReadOnlyTransaction
		err error
		row *Row
		i   int64
	)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()
	client, _, cleanup := prepareIntegrationTest(ctx, t, DefaultSessionPoolConfig, statements[testDialect][simpleDDLStatements])
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
	skipUnsupportedPGTest(t)

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
	client, _, cleanup := prepareIntegrationTest(ctx, t, DefaultSessionPoolConfig, statements[testDialect][singerDDLStatements])
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

	singersQuery := []string{`INSERT INTO Singers (SingerId, FirstName, LastName) VALUES (1, "Umm", "Kulthum")`,
		`INSERT INTO Singers (SingerId, FirstName, LastName) VALUES (2, "Eduard", "Khil")`,
		`INSERT INTO Singers (SingerId, FirstName, LastName) VALUES (3, "Audra", "McDonald")`,
		`UPDATE Singers SET FirstName = "Oum" WHERE SingerId = 1`,
		`UPDATE Singers SET FirstName = "Eddie" WHERE SingerId = 2`,
		`INSERT INTO Singers (SingerId, FirstName, LastName) VALUES (3, "Audra", "McDonald")`,
		`SELECT FirstName FROM Singers`,
	}

	if testDialect == adminpb.DatabaseDialect_POSTGRESQL {
		singersQuery = []string{`INSERT INTO Singers (SingerId, FirstName, LastName) VALUES (1, 'Umm', 'Kulthum')`,
			`INSERT INTO Singers (SingerId, FirstName, LastName) VALUES (2, 'Eduard', 'Khil')`,
			`INSERT INTO Singers (SingerId, FirstName, LastName) VALUES (3, 'Audra', 'McDonald')`,
			`UPDATE Singers SET FirstName = 'Oum' WHERE SingerId = 1`,
			`UPDATE Singers SET FirstName = 'Eddie' WHERE SingerId = 2`,
			`INSERT INTO Singers (SingerId, FirstName, LastName) VALUES (3, 'Audra', 'McDonald')`,
			`SELECT FirstName FROM Singers`,
		}
	}
	// Use ReadWriteTransaction.Query to execute a DML statement.
	_, err := client.ReadWriteTransaction(ctx, func(ctx context.Context, tx *ReadWriteTransaction) error {
		iter := tx.Query(ctx, Statement{
			SQL: singersQuery[0],
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
			SQL: singersQuery[1],
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
			SQL: singersQuery[2],
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
		_, err := tx.Update(ctx, Statement{SQL: singersQuery[3]})
		if err != nil {
			return err
		}
		_, err = tx.Update(ctx, Statement{SQL: singersQuery[4]})
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
			SQL: singersQuery[5],
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
		_, err := tx.Update(ctx, Statement{SQL: singersQuery[6]})
		return err
	})
	if got, want := ErrCode(err), codes.InvalidArgument; got != want {
		t.Errorf("got %s, want %s", got, want)
	}
}

func TestIntegration_StructParametersBind(t *testing.T) {
	t.Parallel()
	skipUnsupportedPGTest(t)

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
	client, _, cleanup := prepareIntegrationTest(ctx, t, DefaultSessionPoolConfig, statements[testDialect][singerDDLStatements])
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
	query := `UPDATE Singers SET Singers.FirstName = "changed" WHERE Singers.SingerId >= 1 AND Singers.SingerId <= @p1`
	if testDialect == adminpb.DatabaseDialect_POSTGRESQL {
		query = `UPDATE Singers SET FirstName = 'changed' WHERE SingerId >= 1 AND SingerId <= $1`
	}
	// Identifiers in PDML statements must be fully qualified.
	// TODO(jba): revisit the above.
	count, err := client.PartitionedUpdate(ctx, Statement{
		SQL: query,
		Params: map[string]interface{}{
			"p1": 3,
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
	client, _, cleanup := prepareIntegrationTest(ctx, t, DefaultSessionPoolConfig, statements[testDialect][singerDDLStatements])
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

	singersQuery := []string{`UPDATE Singers SET Singers.FirstName = "changed 1" WHERE Singers.SingerId = 1`,
		`UPDATE Singers SET Singers.FirstName = "changed 2" WHERE Singers.SingerId = 2`,
		`UPDATE Singers SET Singers.FirstName = "changed 3" WHERE Singers.SingerId = 3`,
	}
	if testDialect == adminpb.DatabaseDialect_POSTGRESQL {
		singersQuery = []string{`UPDATE Singers SET FirstName = 'changed 1' WHERE SingerId = 1`,
			`UPDATE Singers SET FirstName = 'changed 2' WHERE SingerId = 2`,
			`UPDATE Singers SET FirstName = 'changed 3' WHERE SingerId = 3`,
		}
	}
	var counts []int64
	_, err := client.ReadWriteTransaction(ctx, func(ctx context.Context, tx *ReadWriteTransaction) (err error) {
		counts, err = tx.BatchUpdate(ctx, []Statement{
			{SQL: singersQuery[0]},
			{SQL: singersQuery[1]},
			{SQL: singersQuery[2]},
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
	client, _, cleanup := prepareIntegrationTest(ctx, t, DefaultSessionPoolConfig, statements[testDialect][singerDDLStatements])
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
	client, _, cleanup := prepareIntegrationTest(ctx, t, DefaultSessionPoolConfig, statements[testDialect][singerDDLStatements])
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

	singersQuery := []string{`UPDATE Singers SET Singers.FirstName = "changed 1" WHERE Singers.SingerId = 1`,
		`UPDATE Singers SET Singers.FirstName = "changed 2" WHERE Singers.SingerId = 2`,
		`UPDATE Singers SET Singers.FirstName = "changed 3" WHERE Singers.SingerId = 3`,
		`UPDATE Singers SET Singers.FirstName = "changed 1" WHERE Singers.SingerId = 1`}

	if testDialect == adminpb.DatabaseDialect_POSTGRESQL {
		singersQuery = []string{`UPDATE Singers SET FirstName = 'changed 1' WHERE SingerId = 1`,
			`UPDATE Singers SET FirstName = 'changed 2' WHERE SingerId = 2`,
			`UPDATE Singers SET FirstName = 'changed 3' WHERE SingerId = 3`,
			`UPDATE Singers SET FirstName = 'changed 1' WHERE SingerId = 1`}
	}
	var updateCount int64
	var batchCounts []int64
	_, err := client.ReadWriteTransaction(ctx, func(ctx context.Context, tx *ReadWriteTransaction) (err error) {
		batchCounts, err = tx.BatchUpdate(ctx, []Statement{
			{SQL: singersQuery[0]},
			{SQL: singersQuery[1]},
			{SQL: singersQuery[2]},
		})
		if err != nil {
			return err
		}

		updateCount, err = tx.Update(ctx, Statement{SQL: singersQuery[3]})
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
	client, _, cleanup := prepareIntegrationTest(ctx, t, DefaultSessionPoolConfig, statements[testDialect][singerDDLStatements])
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

	singersQuery := []string{`UPDATE Singers SET Singers.FirstName = "changed 1" WHERE Singers.SingerId = 1`,
		`some illegal statement`,
		`UPDATE Singers SET Singers.FirstName = "changed 3" WHERE Singers.SingerId = 3`}

	if testDialect == adminpb.DatabaseDialect_POSTGRESQL {
		singersQuery = []string{`UPDATE Singers SET FirstName = 'changed 1' WHERE SingerId = 1`,
			`some illegal statement`,
			`UPDATE Singers SET FirstName = 'changed 3' WHERE SingerId = 3`}
	}
	_, err := client.ReadWriteTransaction(ctx, func(ctx context.Context, tx *ReadWriteTransaction) (err error) {
		counts, err := tx.BatchUpdate(ctx, []Statement{
			{SQL: singersQuery[0]},
			{SQL: singersQuery[1]},
			{SQL: singersQuery[2]},
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

func TestIntegration_PGNumeric(t *testing.T) {
	onlyRunForPGTest(t)
	skipEmulatorTest(t)
	t.Parallel()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()
	client, _, cleanup := prepareIntegrationTestForPG(ctx, t, DefaultSessionPoolConfig, singerDBPGStatements)
	defer cleanup()

	_, err := client.ReadWriteTransaction(ctx, func(ctx context.Context, tx *ReadWriteTransaction) error {
		count, err := tx.Update(ctx, Statement{
			SQL: `INSERT INTO Singers (SingerId, numeric, float8) VALUES ($1, $2, $3)`,
			Params: map[string]interface{}{
				"p1": int64(123),
				"p2": PGNumeric{"123.456789", true},
				"p3": float64(123.456),
			},
		})
		if err != nil {
			return err
		}
		if count != 1 {
			t.Errorf("row count: got %d, want 1", count)
		}

		count, err = tx.Update(ctx, Statement{
			SQL: `INSERT INTO Singers (SingerId, numeric, float8) VALUES ($1, $2, $3)`,
			Params: map[string]interface{}{
				"p1": int64(456),
				"p2": PGNumeric{"NaN", true},
				"p3": float64(12345.6),
			},
		})
		if err != nil {
			return err
		}
		if count != 1 {
			t.Errorf("row count: got %d, want 1", count)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}

	iter := client.Single().Query(ctx, Statement{SQL: "SELECT SingerId, numeric, float8 FROM Singers"})
	got, err := readPGSingerTable(iter)
	if err != nil {
		t.Fatalf("failed to read data: %v", err)
	}
	want := [][]interface{}{
		{int64(123), PGNumeric{"123.456789", true}, float64(123.456)},
		{int64(456), PGNumeric{"NaN", true}, float64(12345.6)},
	}
	if !testEqual(got, want) {
		t.Errorf("\ngot %v\nwant%v", got, want)
	}
}

func TestIntegration_PGJSONB(t *testing.T) {
	onlyRunForPGTest(t)
	skipEmulatorTest(t)
	t.Parallel()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()
	client, _, cleanup := prepareIntegrationTestForPG(ctx, t, DefaultSessionPoolConfig, singerDBPGStatements)
	defer cleanup()

	type Message struct {
		Name string
		Body string
		Time int64
	}
	msg := Message{"Alice", "Hello", 1294706395881547000}
	jsonStr := `{"Name":"Alice","Body":"Hello","Time":1294706395881547000}`
	var unmarshalledJSONstruct interface{}
	json.Unmarshal([]byte(jsonStr), &unmarshalledJSONstruct)

	tests := []struct {
		col  string
		val  interface{}
		want interface{}
	}{
		{col: "JSONB", val: PGJsonB{Value: msg, Valid: true}, want: PGJsonB{Value: unmarshalledJSONstruct, Valid: true}},
		{col: "JSONB", val: PGJsonB{Value: msg, Valid: false}, want: PGJsonB{}},
	}

	// Write rows into table first using DML.
	statements := make([]Statement, 0)
	for i, test := range tests {
		stmt := NewStatement(fmt.Sprintf("INSERT INTO Types (RowId, %s) VALUES ($1, $2)", test.col))
		// Note: We are not setting the parameter type here to ensure that it
		// can be automatically recognized when it is actually needed.
		stmt.Params["p1"] = i
		stmt.Params["p2"] = test.val
		statements = append(statements, stmt)
	}
	_, err := client.ReadWriteTransaction(ctx, func(ctx context.Context, tx *ReadWriteTransaction) error {
		rowCounts, err := tx.BatchUpdate(ctx, statements)
		if err != nil {
			return err
		}
		if len(rowCounts) != len(tests) {
			return fmt.Errorf("rowCounts length mismatch\nGot: %v\nWant: %v", len(rowCounts), len(tests))
		}
		for i, c := range rowCounts {
			if c != 1 {
				return fmt.Errorf("row count mismatch for row %v:\nGot: %v\nWant: %v", i, c, 1)
			}
		}
		return nil
	})
	if err != nil {
		t.Fatalf("failed to insert values using DML: %v", err)
	}
	// Delete all the rows so we can insert them using mutations as well.
	_, err = client.Apply(ctx, []*Mutation{Delete("Types", AllKeys())})
	if err != nil {
		t.Fatalf("failed to delete all rows: %v", err)
	}

	// Verify that we can insert the rows using mutations.
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

func readPGSingerTable(iter *RowIterator) ([][]interface{}, error) {
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
		var id int64
		var numeric PGNumeric
		var float8 float64
		err = row.Columns(&id, &numeric, &float8)
		if err != nil {
			return nil, err
		}
		vals = append(vals, []interface{}{id, numeric, float8})
	}
}

func TestIntegration_StartBackupOperation(t *testing.T) {
	t.Skip("https://github.com/googleapis/google-cloud-go/issues/6200")
	skipEmulatorTest(t)
	skipUnsupportedPGTest(t)
	t.Parallel()

	startTime := time.Now()
	// Backups can be slow, so use 1 hour timeout.
	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Hour)
	defer cancel()
	client, testDatabaseName, cleanup := prepareIntegrationTest(ctx, t, DefaultSessionPoolConfig, statements[testDialect][backupDDLStatements])
	defer cleanup()

	// Set up 1 singer to have backup size greater than zero
	singers := []*Mutation{
		Insert("Singers", []string{"SingerId", "FirstName", "LastName"}, []interface{}{int64(1), "test", "test"}),
	}
	if _, err := client.Apply(ctx, singers, ApplyAtLeastOnce()); err != nil {
		t.Fatal(err)
	}

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
	t.Logf("create backup operation took: %v\n", time.Since(startTime))
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
	client, _, cleanup := prepareIntegrationTest(ctx, t, DefaultSessionPoolConfig, statements[testDialect][readDDLStatements])
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

func compareErrors(got, want error) bool {
	if got == nil || want == nil {
		return got == want
	}
	gotStr := got.Error()
	wantStr := want.Error()
	if idx := strings.Index(gotStr, "requestID"); idx != -1 {
		gotStr = gotStr[:idx]
	}
	if idx := strings.Index(wantStr, "requestID"); idx != -1 {
		wantStr = wantStr[:idx]
	}
	gotStr = strings.ReplaceAll(gotStr, `",`, ``)
	wantStr = strings.ReplaceAll(gotStr, `",`, ``)
	return strings.EqualFold(strings.TrimSpace(gotStr), strings.TrimSpace(wantStr))
}

func TestIntegration_Foreign_Key_Delete_Cascade_Action(t *testing.T) {
	skipEmulatorTest(t)
	t.Parallel()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()
	// create table with foreign key actions
	client, dbPath, cleanup := prepareIntegrationTest(ctx, t, DefaultSessionPoolConfig, statements[testDialect][fkdcDDLStatements])
	defer cleanup()

	var tests = []struct {
		name     string
		test     func() error
		validate func()
		wantErr  error
	}{
		{
			name: "add delete cascade constraint",
			test: func() error {
				constraintName := "FKShoppingCartsCustomerName"
				if testDialect == adminpb.DatabaseDialect_POSTGRESQL {
					constraintName = `"FKShoppingCartsCustomerName"`
				}
				addConstraintDDL := fmt.Sprintf("ALTER TABLE ShoppingCarts ADD CONSTRAINT %s FOREIGN KEY (CustomerName) REFERENCES Customers(CustomerName) ON DELETE CASCADE", constraintName)
				op, err := databaseAdmin.UpdateDatabaseDdl(ctx, &adminpb.UpdateDatabaseDdlRequest{
					Database:   dbPath,
					Statements: []string{addConstraintDDL},
				})
				if err != nil {
					return err
				}
				if err := op.Wait(ctx); err != nil {
					return err
				}
				return nil
			},
			validate: func() {
				constraintName := `"FKShoppingCartsCustomerName"`
				if testDialect == adminpb.DatabaseDialect_POSTGRESQL {
					constraintName = `'FKShoppingCartsCustomerName'`
				}
				got, err := readAll(client.Single().Query(ctx, Statement{
					fmt.Sprintf(`SELECT 1, '', DELETE_RULE FROM INFORMATION_SCHEMA.REFERENTIAL_CONSTRAINTS WHERE CONSTRAINT_NAME = %s`, constraintName),
					map[string]interface{}{}}))
				if err != nil {
					t.Fatalf("Expect to read the delete_rule from information_schema, got: %v", err)
				}
				if !testEqual(got, [][]interface{}{{int64(1), "", "CASCADE"}}) {
					t.Error("DELETE_RULE is not CASCADE")
				}
			},
		},
		{
			name: "drop delete cascade constraint",
			test: func() error {
				constraintName := "FKShoppingCartsCustomerName"
				if testDialect == adminpb.DatabaseDialect_POSTGRESQL {
					constraintName = `"FKShoppingCartsCustomerName"`
				}
				dropConstraintDDL := fmt.Sprintf("ALTER TABLE ShoppingCarts DROP CONSTRAINT %s", constraintName)
				op, err := databaseAdmin.UpdateDatabaseDdl(ctx, &adminpb.UpdateDatabaseDdlRequest{
					Database:   dbPath,
					Statements: []string{dropConstraintDDL},
				})
				if err != nil {
					return err
				}
				if err := op.Wait(ctx); err != nil {
					return err
				}
				return nil
			},
			validate: func() {
				constraintName := `"FKShoppingCartsCustomerName"`
				if testDialect == adminpb.DatabaseDialect_POSTGRESQL {
					constraintName = `'FKShoppingCartsCustomerName'`
				}
				got, err := readAll(client.Single().Query(ctx, Statement{
					fmt.Sprintf(`SELECT 1, '', DELETE_RULE FROM INFORMATION_SCHEMA.REFERENTIAL_CONSTRAINTS WHERE CONSTRAINT_NAME = %s`, constraintName),
					map[string]interface{}{}}))
				if err != nil {
					t.Fatalf("Expect to read the delete_rule from information_schema, got: %v", err)
				}
				var want [][]interface{}
				if !testEqual(got, want) {
					t.Error("DELETE_RULE should be empty")
				}
			},
		},
		{
			name: "success: insert a row in referencing table",
			test: func() error {
				// Populate the parent table.
				columns := []string{"CustomerId", "CustomerName"}
				var muts []*Mutation
				for _, row := range [][]interface{}{
					{1, "FKCustomer1"},
					{2, "FKCustomer2"},
					{3, "FKCustomer3"},
				} {
					muts = append(muts, Insert("Customers", columns, row))
				}
				if _, err := client.Apply(ctx, muts); err != nil {
					return err
				}

				// Populate the referencing table.
				columns = []string{"CartId", "CustomerId", "CustomerName"}
				// Populate the parent table.
				muts = []*Mutation{}
				for _, row := range [][]interface{}{
					{1, 1, "FKCustomer1"},
					{2, 1, "FKCustomer1"},
					{3, 2, "FKCustomer2"},
				} {
					muts = append(muts, Insert("ShoppingCarts", columns, row))
				}
				if _, err := client.Apply(ctx, muts); err != nil {
					return err
				}
				return nil
			},
			validate: func() {},
		},
		{
			name: "success: deleting a row in referenced table should delete all rows referencing it",
			test: func() error {
				_, err := client.Apply(ctx, []*Mutation{Delete("Customers", Key{1})})
				return err
			},
			validate: func() {
				if _, err := client.Single().ReadRow(ctx, "ShoppingCarts", Key{1}, []string{"CartId"}); err == nil {
					t.Fatalf("expected to return not found error after deleting in referenced table")
				}
				if _, err := client.Single().ReadRow(ctx, "ShoppingCarts", Key{2}, []string{"CartId"}); err == nil {
					t.Fatalf("expected to return not found error after deleting in referenced table")
				}
			},
		},
		{
			name: "failure: conflicting insert and delete of referenced key",
			test: func() error {
				columns := []string{"CustomerId", "CustomerName"}
				var muts []*Mutation
				for _, row := range [][]interface{}{
					{3, "FKCustomer3"},
				} {
					muts = append(muts, Insert("Customers", columns, row))
				}
				muts = append(muts, Delete("Customers", Key{3}))
				_, err := client.Apply(ctx, muts)
				return err
			},
			wantErr:  spannerErrorf(codes.FailedPrecondition, "Cannot write a value for the referenced column `Customers.CustomerId` and delete it in the same transaction."),
			validate: func() {},
		},
		{
			name: "failure: insert in child table and deleting referenced key from parent table",
			test: func() error {
				columns := []string{"CartId", "CustomerId", "CustomerName"}
				// Populate the parent table.
				muts := []*Mutation{}
				for _, row := range [][]interface{}{
					{4, 3, "FKCustomer3"},
				} {
					muts = append(muts, Insert("ShoppingCarts", columns, row))
				}
				muts = append(muts, Delete("Customers", Key{3}))
				_, err := client.Apply(ctx, muts)
				return err
			},
			wantErr:  spannerErrorf(codes.FailedPrecondition, "Foreign key constraint `FKShoppingCartsCustomerId` is violated on table `ShoppingCarts`. Cannot find referenced values in Customers(CustomerId)."),
			validate: func() {},
		},
		{
			name: "failure: insert a row in referencing table with reference key not present",
			test: func() error {
				// inset in the referencing table.
				columns := []string{"CartId", "CustomerId", "CustomerName"}
				muts := []*Mutation{}
				for _, row := range [][]interface{}{
					{1, 100, "FKCustomer1"},
				} {
					muts = append(muts, Insert("ShoppingCarts", columns, row))
				}
				if _, err := client.Apply(ctx, muts); err != nil {
					return err
				}
				return nil
			},
			wantErr:  spannerErrorf(codes.FailedPrecondition, "Foreign key constraint `FKShoppingCartsCustomerId` is violated on table `ShoppingCarts`. Cannot find referenced values in Customers(CustomerId)."),
			validate: func() {},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotErr := tt.test()
			if gotErr != nil {
				if !compareErrors(gotErr, tt.wantErr) {
					t.Errorf(`FKDC error=%v, wantErr: %v`, gotErr, tt.wantErr)
				}
			} else {
				tt.validate()
			}
		})
	}
}

func TestIntegration_GFE_Latency(t *testing.T) {
	skipDirectPathTest(t)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	te := stestutil.NewTestExporter(GFEHeaderMissingCountView, GFELatencyView)
	setGFELatencyMetricsFlag(true)

	client, _, cleanup := prepareIntegrationTest(ctx, t, DefaultSessionPoolConfig, statements[testDialect][singerDDLStatements])
	defer cleanup()

	singerColumns := []string{"SingerId", "FirstName", "LastName"}
	var ms = []*Mutation{
		InsertOrUpdate("Singers", singerColumns, []interface{}{1, "Marc", "Richards"}),
	}
	_, err := client.Apply(ctx, ms)
	if err != nil {
		t.Fatalf("Could not insert rows to table. Got error %v", err)
	}
	_, err = client.Single().ReadRow(ctx, "Singers", Key{1}, []string{"SingerId", "FirstName", "LastName"})
	if err != nil {
		t.Fatalf("Could not read row. Got error %v", err)
	}
	waitErr := &Error{}
	waitFor(t, func() error {
		select {
		case stat := <-te.Stats:
			if len(stat.Rows) > 0 {
				return nil
			}
		}
		return waitErr
	})

	var viewMap = map[string]bool{statsPrefix + "gfe_latency": false,
		statsPrefix + "gfe_header_missing_count": false,
	}

	for {
		if viewMap[statsPrefix+"gfe_latency"] || viewMap[statsPrefix+"gfe_header_missing_count"] {
			break
		}
		select {
		case stat := <-te.Stats:
			if len(stat.Rows) == 0 {
				t.Fatal("No metrics are exported")
			}
			if stat.View.Measure.Name() != statsPrefix+"gfe_latency" && stat.View.Measure.Name() != statsPrefix+"gfe_header_missing_count" {
				t.Fatalf("Incorrect measure: got %v, want %v", stat.View.Measure.Name(), statsPrefix+"gfe_latency or "+statsPrefix+"gfe_header_missing_count")
			} else {
				viewMap[stat.View.Measure.Name()] = true
			}
			for _, row := range stat.Rows {
				m := getTagMap(row.Tags)
				checkCommonTagsGFELatency(t, m)
				var data string
				switch row.Data.(type) {
				default:
					data = fmt.Sprintf("%v", row.Data)
				case *view.CountData:
					data = fmt.Sprintf("%v", row.Data.(*view.CountData).Value)
				case *view.LastValueData:
					data = fmt.Sprintf("%v", row.Data.(*view.LastValueData).Value)
				case *view.DistributionData:
					data = fmt.Sprintf("%v", row.Data.(*view.DistributionData).Count)
				}
				if got, want := fmt.Sprintf("%v", data), "0"; got <= want {
					t.Fatalf("Incorrect data: got %v, wanted more than %v for metric %v", got, want, stat.View.Measure.Name())
				}
			}
		case <-time.After(10 * time.Second):
			if !viewMap[statsPrefix+"gfe_latency"] && !viewMap[statsPrefix+"gfe_header_missing_count"] {
				t.Fatal("no stats were exported before timeout")
			}
		}
	}
	DisableGfeLatencyAndHeaderMissingCountViews()
}

func TestIntegration_Bit_Reversed_Sequence(t *testing.T) {
	skipEmulatorTest(t)
	t.Parallel()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()
	// create table with bit reverse seq options
	client, dbPath, cleanup := prepareIntegrationTest(ctx, t, DefaultSessionPoolConfig, statements[testDialect][testTableBitReversedSeqStatements])
	defer cleanup()

	returningSQL := `THEN RETURN`
	internalStateSQL := `GET_INTERNAL_SEQUENCE_STATE(Sequence seqT)`
	if testDialect == adminpb.DatabaseDialect_POSTGRESQL {
		returningSQL = `RETURNING`
		internalStateSQL = `spanner.get_internal_sequence_state('seqT')`
	}

	var tests = []struct {
		name    string
		test    func() error
		wantErr error
	}{
		{
			name: "success: inserted rows should have auto generated keys",
			test: func() error {
				var (
					values [][]interface{}
					err    error
				)
				_, err = client.ReadWriteTransaction(ctx, func(ctx context.Context, tx *ReadWriteTransaction) error {
					iter := tx.Query(ctx, NewStatement("INSERT INTO T (value) VALUES (100), (200), (300) "+returningSQL+" id, counter"))
					values, err = readAllBitReversedSeqTable(iter, true)
					return err
				})
				if len(values) != 3 {
					return errors.New("expected 3 rows to be inserted")
				}
				counter := int64(0)
				for i := 0; i < 3; i++ {
					newID, newCounter := values[i][0].(int64), values[i][1].(int64)
					if newID <= 0 {
						return errors.New("expected id1, id2, id3 > 0")

					}
					if newCounter < counter {
						return errors.New("expected c3 >= c2 >= c1")
					}
					counter = newCounter
				}
				iter := client.Single().Query(ctx, NewStatement("SELECT "+internalStateSQL))
				r, err := iter.Next()
				if err != nil {
					return err
				}
				var c3 int64
				err = r.Columns(&c3)
				if err != nil {
					return err
				}
				if c3 > counter {
					return errors.New("expected c3 <= SELECT GET_INTERNAL_SEQUENCE_STATE(Sequence seqT)")
				}
				return err
			},
		},
		{
			name: "success: reduce ranges to half",
			test: func() error {
				ddl := `ALTER SEQUENCE seqT SET OPTIONS (
						skip_range_min = 0,
						skip_range_max = 4611686018427387904
					)`
				if testDialect == adminpb.DatabaseDialect_POSTGRESQL {
					ddl = `ALTER SEQUENCE seqT SKIP RANGE 0 4611686018427387904`
				}
				op, err := databaseAdmin.UpdateDatabaseDdl(ctx, &adminpb.UpdateDatabaseDdlRequest{
					Database:   dbPath,
					Statements: []string{ddl},
				})
				if err != nil {
					return err
				}
				if err := op.Wait(ctx); err != nil {
					return err
				}
				var values [][]interface{}
				_, err = client.ReadWriteTransaction(ctx, func(ctx context.Context, tx *ReadWriteTransaction) error {
					var v string
					for i := 1; i <= 100; i++ {
						v = v + " (" + strconv.Itoa(i) + ")"
						if i != 100 {
							v = v + ", "
						}
					}
					iter := tx.Query(ctx, NewStatement("INSERT INTO T (value) VALUES "+v+" "+returningSQL+" id, counter"))
					values, err = readAllBitReversedSeqTable(iter, true)
					return err
				})
				if err != nil {
					return err
				}
				if len(values) != 100 {
					return errors.New("expected 100 rows to be inserted")
				}
				counter := int64(0)
				for i := 0; i < 100; i++ {
					newID, newCounter := values[i][0].(int64), values[i][1].(int64)
					if newID <= 0 || newID < 4611686018427387904 {
						return errors.New("expected d1, id2, id3, …., id100 > 4611686018427387904")

					}
					if newCounter < counter {
						return errors.New("expected c3 >= c2 >= c1")
					}
					counter = newCounter
				}
				return err
			},
		},
		{
			name: "success: move start with counter forward",
			test: func() error {
				ddl := `ALTER SEQUENCE seqT SET OPTIONS (start_with_counter=10001)`
				if testDialect == adminpb.DatabaseDialect_POSTGRESQL {
					ddl = `ALTER SEQUENCE seqT RESTART COUNTER WITH 10001`
				}
				op, err := databaseAdmin.UpdateDatabaseDdl(ctx, &adminpb.UpdateDatabaseDdlRequest{
					Database:   dbPath,
					Statements: []string{ddl},
				})
				if err != nil {
					return err
				}
				if err := op.Wait(ctx); err != nil {
					return err
				}
				var values [][]interface{}
				_, err = client.ReadWriteTransaction(ctx, func(ctx context.Context, tx *ReadWriteTransaction) error {
					iter := tx.Query(ctx, NewStatement("INSERT INTO T (value) VALUES (4), (5), (6) "+returningSQL+" id, br_id, counter"))
					values, err = readAllBitReversedSeqTable(iter, false)
					return err
				})
				if err != nil {
					return err
				}
				if len(values) != 3 {
					return errors.New("expected 3 rows to be inserted")
				}
				counter := int64(0)
				for i := 0; i < 3; i++ {
					newID, newBrID, newCounter := values[i][0].(int64), values[i][1].(int64), values[i][2].(int64)
					if newID <= 0 {
						return errors.New("expected id > 0")
					}
					if newBrID < 10001 {
						return errors.New("expected br_idi >= 10001")
					}
					if newCounter < 10001 {
						return errors.New("expected ci >= 10001")
					}
					if counter < newCounter {
						counter = newCounter
					}
				}
				iter := client.Single().Query(ctx, NewStatement("SELECT "+internalStateSQL))
				r, err := iter.Next()
				if err != nil {
					return err
				}
				var c3 int64
				err = r.Columns(&c3)
				if err != nil {
					return err
				}
				if c3 > counter {
					return errors.New("expected max(ci) <= SELECT GET_INTERNAL_SEQUENCE_STATE(Sequence seqT)")
				}
				return err
			},
		},
		{
			name: "success: drop the sequences",
			test: func() error {
				op, err := databaseAdmin.UpdateDatabaseDdl(ctx, &adminpb.UpdateDatabaseDdlRequest{
					Database: dbPath,
					Statements: []string{
						"ALTER TABLE T ALTER COLUMN id DROP DEFAULT",
						"ALTER TABLE T ALTER COLUMN counter DROP DEFAULT",
						"ALTER TABLE T DROP CONSTRAINT id_gt_0",
						"ALTER TABLE T DROP CONSTRAINT counter_gt_br_id",
						"ALTER TABLE T DROP CONSTRAINT br_id_true",
						"DROP SEQUENCE seqT",
					},
				})
				if err != nil {
					return err
				}
				if err := op.Wait(ctx); err != nil {
					return err
				}
				return nil
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotErr := tt.test()
			if gotErr != nil && !strings.EqualFold(gotErr.Error(), tt.wantErr.Error()) {
				t.Errorf("BIT REVERSED SEQUENECES error=%v, wantErr: %v", gotErr, tt.wantErr)
			}
		})
	}
}

func TestIntegration_WithDirectedReadOptions_ReadOnlyTransaction(t *testing.T) {
	t.Parallel()
	// DirectedReadOptions for PG is supported, however we test only for Google SQL.
	skipUnsupportedPGTest(t)
	skipEmulatorTest(t)

	ctxTimeout := 5 * time.Minute
	ctx, cancel := context.WithTimeout(context.Background(), ctxTimeout)
	defer cancel()
	// Set up testing environment.
	client, _, cleanup := prepareIntegrationTest(ctx, t, DefaultSessionPoolConfig, statements[testDialect][singerDDLStatements])
	defer cleanup()

	directedReadOptions := &sppb.DirectedReadOptions{
		Replicas: &sppb.DirectedReadOptions_IncludeReplicas_{
			IncludeReplicas: &sppb.DirectedReadOptions_IncludeReplicas{
				ReplicaSelections: []*sppb.DirectedReadOptions_ReplicaSelection{
					{
						Location: "us-west1",
						Type:     sppb.DirectedReadOptions_ReplicaSelection_READ_ONLY,
					},
				},
				AutoFailoverDisabled: true,
			},
		},
	}

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

	want := [][]interface{}{{int64(1), "Marc", "Foo"}, {int64(3), "Alpha", "Beta"}, {int64(4), "Last", "End"}}

	// Test DirectedReadOptions for ReadOnlyTransaction.ReadWithOptions
	got, err := readAll(client.ReadOnlyTransaction().ReadWithOptions(ctx, "Singers", KeySets(Key{1}, Key{3}, Key{4}), []string{"SingerId", "FirstName", "LastName"},
		&ReadOptions{DirectedReadOptions: directedReadOptions}))
	if err != nil {
		t.Errorf("DirectedReadOptions using ReadOptions returns error %v, want nil", err)
	}
	if !testEqual(got, want) {
		t.Errorf("got unexpected result in DirectedReadOptions test: %v, want %v", got, want)
	}

	// Test DirectedReadOptions for ReadOnlyTransaction.QueryWithOptions
	singersQuery := "SELECT SingerId, FirstName, LastName FROM Singers WHERE SingerId IN (@p1, @p2, @p3) ORDER BY SingerId"
	got, err = readAll(client.Single().QueryWithOptions(ctx, Statement{
		singersQuery,
		map[string]interface{}{"p1": int64(1), "p2": int64(3), "p3": int64(4)},
	}, QueryOptions{DirectedReadOptions: directedReadOptions}))

	if err != nil {
		t.Errorf("DirectedReadOptions using QueryOptions returns error %v, want nil", err)
	}

	if !testEqual(got, want) {
		t.Errorf("got unexpected result in DirectedReadOptions test: %v, want %v", got, want)
	}
}

func TestIntegration_WithDirectedReadOptions_ReadWriteTransaction_ShouldThrowError(t *testing.T) {
	t.Parallel()
	// DirectedReadOptions for PG is supported, however we test only for Google SQL.
	skipUnsupportedPGTest(t)
	skipEmulatorTest(t)

	ctxTimeout := 5 * time.Minute
	ctx, cancel := context.WithTimeout(context.Background(), ctxTimeout)
	defer cancel()
	// Set up testing environment.
	client, _, cleanup := prepareIntegrationTest(ctx, t, DefaultSessionPoolConfig, statements[testDialect][singerDDLStatements])
	defer cleanup()

	directedReadOptions := &sppb.DirectedReadOptions{
		Replicas: &sppb.DirectedReadOptions_IncludeReplicas_{
			IncludeReplicas: &sppb.DirectedReadOptions_IncludeReplicas{
				ReplicaSelections: []*sppb.DirectedReadOptions_ReplicaSelection{
					{
						Location: "us-west1",
						Type:     sppb.DirectedReadOptions_ReplicaSelection_READ_ONLY,
					},
				},
				AutoFailoverDisabled: true,
			},
		},
	}

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

	_, err := client.ReadWriteTransaction(ctx, func(ctx context.Context, tx *ReadWriteTransaction) (err error) {
		_, err = readAll(tx.ReadWithOptions(ctx, "Singers", KeySets(Key{1}, Key{3}, Key{4}), []string{"SingerId", "FirstName", "LastName"}, &ReadOptions{DirectedReadOptions: directedReadOptions}))
		return err
	})
	if err == nil {
		t.Fatal("expected err, got nil")
	}
	if msg, ok := matchError(err, codes.InvalidArgument, "Directed reads can only be performed in a read-only transaction"); !ok {
		t.Fatal(msg)
	}
}

// Prepare initializes Cloud Spanner testing DB and clients.
func prepareIntegrationTest(ctx context.Context, t *testing.T, spc SessionPoolConfig, statements []string) (*Client, string, func()) {
	return prepareDBAndClient(ctx, t, spc, statements, testDialect)
}

func prepareIntegrationTestForPG(ctx context.Context, t *testing.T, spc SessionPoolConfig, statements []string) (*Client, string, func()) {
	return prepareDBAndClient(ctx, t, spc, statements, adminpb.DatabaseDialect_POSTGRESQL)
}

func prepareIntegrationTestForProtoColumns(ctx context.Context, t *testing.T, spc SessionPoolConfig, statements []string, protoDescriptor []byte) (*Client, string, func()) {
	return prepareDBAndClientForProtoColumnsDDL(ctx, t, spc, statements, testDialect, protoDescriptor)
}

func prepareDBAndClient(ctx context.Context, t *testing.T, spc SessionPoolConfig, statements []string, dbDialect adminpb.DatabaseDialect) (*Client, string, func()) {
	if databaseAdmin == nil {
		t.Skip("Integration tests skipped")
	}
	// Construct a unique test DB name.
	dbName := dbNameSpace.New()

	dbPath := fmt.Sprintf("projects/%v/instances/%v/databases/%v", testProjectID, testInstanceID, dbName)
	// Create database and tables.
	req := &adminpb.CreateDatabaseRequest{
		Parent:          fmt.Sprintf("projects/%v/instances/%v", testProjectID, testInstanceID),
		CreateStatement: "CREATE DATABASE " + dbName,
		ExtraStatements: statements,
		DatabaseDialect: dbDialect,
	}
	if dbDialect == adminpb.DatabaseDialect_POSTGRESQL {
		req.ExtraStatements = []string{}
	}
	op, err := databaseAdmin.CreateDatabaseWithRetry(ctx, req)
	if err != nil {
		t.Fatalf("cannot create testing DB %v: %v", dbPath, err)
	}
	if _, err := op.Wait(ctx); err != nil {
		t.Fatalf("cannot create testing DB %v: %v", dbPath, err)
	}
	if dbDialect == adminpb.DatabaseDialect_POSTGRESQL && len(statements) > 0 {
		op, err := databaseAdmin.UpdateDatabaseDdl(ctx, &adminpb.UpdateDatabaseDdlRequest{
			Database:   dbPath,
			Statements: statements,
		})
		if err != nil {
			t.Fatalf("cannot create testing table %v: %v", dbPath, err)
		}
		if err := op.Wait(ctx); err != nil {
			t.Fatalf("timeout creating testing table %v: %v", dbPath, err)
		}
	}
	client, err := createClient(ctx, dbPath, ClientConfig{SessionPoolConfig: spc})
	if err != nil {
		t.Fatalf("cannot create data client on DB %v: %v", dbPath, err)
	}
	return client, dbPath, func() {
		client.Close()
	}
}

func prepareDBAndClientForProtoColumnsDDL(ctx context.Context, t *testing.T, spc SessionPoolConfig, statements []string, dbDialect adminpb.DatabaseDialect, protoDescriptor []byte) (*Client, string, func()) {
	if databaseAdmin == nil {
		t.Skip("Integration tests skipped")
	}
	// Construct a unique test DB name.
	dbName := dbNameSpace.New()

	dbPath := fmt.Sprintf("projects/%v/instances/%v/databases/%v", testProjectID, testInstanceID, dbName)
	// Create database and tables.
	req := &adminpb.CreateDatabaseRequest{
		Parent:           fmt.Sprintf("projects/%v/instances/%v", testProjectID, testInstanceID),
		CreateStatement:  "CREATE DATABASE " + dbName,
		ExtraStatements:  statements,
		DatabaseDialect:  dbDialect,
		ProtoDescriptors: protoDescriptor,
	}

	op, err := databaseAdmin.CreateDatabaseWithRetry(ctx, req)
	if err != nil {
		t.Fatalf("cannot create testing DB with Proto columns %v: %v", dbPath, err)
	}
	if _, err := op.Wait(ctx); err != nil {
		t.Fatalf("cannot create testing DB with Proto columns %v: %v", dbPath, err)
	}

	client, err := createClientForProtoColumns(ctx, dbPath, spc)
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
func createClient(ctx context.Context, dbPath string, config ClientConfig) (client *Client, err error) {
	opts := grpcHeaderChecker.CallOptions()
	serverAddress := "spanner.googleapis.com:443"
	if spannerHost != "" {
		opts = append(opts, option.WithEndpoint(spannerHost))
		serverAddress = spannerHost
	}
	if dpConfig.attemptDirectPath {
		opts = append(opts, option.WithGRPCDialOption(grpc.WithDefaultCallOptions(grpc.Peer(peerInfo))))
	}
	client, err = makeClientWithConfig(ctx, dbPath, config, serverAddress, opts...)
	if err != nil {
		return nil, fmt.Errorf("cannot create data client on DB %v: %v", dbPath, err)
	}
	return client, nil
}

func createClientWithRole(ctx context.Context, dbPath string, spc SessionPoolConfig, role string) (client *Client, err error) {
	opts := grpcHeaderChecker.CallOptions()
	serverAddress := "spanner.googleapis.com:443"
	if spannerHost != "" {
		opts = append(opts, option.WithEndpoint(spannerHost))
		serverAddress = spannerHost
	}
	if dpConfig.attemptDirectPath {
		opts = append(opts, option.WithGRPCDialOption(grpc.WithDefaultCallOptions(grpc.Peer(peerInfo))))
	}
	config := ClientConfig{SessionPoolConfig: spc, DatabaseRole: role}
	client, err = makeClientWithConfig(ctx, dbPath, config, serverAddress, opts...)
	if err != nil {
		return nil, fmt.Errorf("cannot create data client on DB %v: %v", dbPath, err)
	}
	return client, nil
}

// createClientForProtoColumns creates Cloud Spanner data client for testing proto column feature.
func createClientForProtoColumns(ctx context.Context, dbPath string, spc SessionPoolConfig) (client *Client, err error) {
	opts := grpcHeaderChecker.CallOptions()
	if spannerHost != "" {
		opts = append(opts, option.WithEndpoint(spannerHost))
	}
	if dpConfig.attemptDirectPath {
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

func readAllBitReversedSeqTable(iter *RowIterator, onlyIDCounter bool) ([][]interface{}, error) {
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
		var id, brID, counter int64
		if onlyIDCounter {
			err = row.Columns(&id, &counter)
			if err != nil {
				return nil, err
			}
			vals = append(vals, []interface{}{id, counter})
			continue
		}
		err = row.Columns(&id, &brID, &counter)
		if err != nil {
			return nil, err
		}
		vals = append(vals, []interface{}{id, brID, counter})
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

func skipDirectPathTest(t *testing.T) {
	if directPathEnabled, found := os.LookupEnv("GOOGLE_SPANNER_ENABLE_DIRECT_ACCESS"); found {
		if enabled, _ := strconv.ParseBool(directPathEnabled); enabled {
			t.Skip("Skipping GFE tests when DirectPath is enabled")
		}
	}
}

func skipUnsupportedPGTest(t *testing.T) {
	if testDialect == adminpb.DatabaseDialect_POSTGRESQL {
		t.Skip("Skipping testing of unsupported tests in Postgres dialect.")
	}
}

func skipOnNonProd(t *testing.T) {
	job := os.Getenv("JOB_TYPE")
	if strings.Contains(job, "cloud-devel") || strings.Contains(job, "cloud-staging") {
		t.Skip("Skipping test on non-production environment.")
	}
}

func onlyRunOnCloudDevel(t *testing.T) {
	job := os.Getenv("JOB_TYPE")
	if !strings.Contains(job, "cloud-devel") {
		t.Skip("Skipping test on non cloud-devel environment")
	}
}

func onlyRunForPGTest(t *testing.T) {
	if testDialect != adminpb.DatabaseDialect_POSTGRESQL {
		t.Skip("Skipping tests supported only in Postgres dialect.")
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
			t.Log(string(out))
		} else {
			cmdRes := exec.Command("bash", "-c", allowDpv4Cmd)
			out, _ := cmdRes.CombinedOutput()
			t.Log(string(out))
		}
		return
	}
	// DirectPath supports both ipv4 and ipv6
	if blackholeDP {
		cmdRes := exec.Command("bash", "-c", blackholeDpv4Cmd)
		out, _ := cmdRes.CombinedOutput()
		t.Log(string(out))
		cmdRes = exec.Command("bash", "-c", blackholeDpv6Cmd)
		out, _ = cmdRes.CombinedOutput()
		t.Log(string(out))
	} else {
		cmdRes := exec.Command("bash", "-c", allowDpv4Cmd)
		out, _ := cmdRes.CombinedOutput()
		t.Log(string(out))
		cmdRes = exec.Command("bash", "-c", allowDpv6Cmd)
		out, _ = cmdRes.CombinedOutput()
		t.Log(string(out))
	}
}

func checkCommonTagsGFELatency(t *testing.T, m map[tag.Key]string) {
	// We only check prefix because client ID increases if we create
	// multiple clients for the same database.
	if !strings.HasPrefix(m[tagKeyClientID], "client") {
		t.Fatalf("Incorrect client ID: %v", m[tagKeyClientID])
	}
	if m[tagKeyLibVersion] != internal.Version {
		t.Fatalf("Incorrect library version: %v", m[tagKeyLibVersion])
	}
}

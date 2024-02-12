// Copyright 2015 Google LLC
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
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"log"
	"math/big"
	"net/http"
	"os"
	"sort"
	"strings"
	"testing"
	"time"

	connection "cloud.google.com/go/bigquery/connection/apiv1"
	"cloud.google.com/go/civil"
	datacatalog "cloud.google.com/go/datacatalog/apiv1"
	"cloud.google.com/go/datacatalog/apiv1/datacatalogpb"
	"cloud.google.com/go/httpreplay"
	"cloud.google.com/go/internal"
	"cloud.google.com/go/internal/pretty"
	"cloud.google.com/go/internal/testutil"
	"cloud.google.com/go/internal/uid"
	"cloud.google.com/go/storage"
	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"
	gax "github.com/googleapis/gax-go/v2"
	bq "google.golang.org/api/bigquery/v2"
	"google.golang.org/api/googleapi"
	"google.golang.org/api/iterator"
	"google.golang.org/api/option"
)

const replayFilename = "bigquery.replay"

var record = flag.Bool("record", false, "record RPCs")

var (
	client                 *Client
	storageOptimizedClient *Client
	storageClient          *storage.Client
	connectionsClient      *connection.Client
	policyTagManagerClient *datacatalog.PolicyTagManagerClient
	dataset                *Dataset
	otherDataset           *Dataset
	schema                 = Schema{
		{Name: "name", Type: StringFieldType},
		{Name: "nums", Type: IntegerFieldType, Repeated: true},
		{Name: "rec", Type: RecordFieldType, Schema: Schema{
			{Name: "bool", Type: BooleanFieldType},
		}},
	}
	testTableExpiration                        time.Time
	datasetIDs, tableIDs, modelIDs, routineIDs *uid.Space
)

// Note: integration tests cannot be run in parallel, because TestIntegration_Location
// modifies the client.

func TestMain(m *testing.M) {
	cleanup := initIntegrationTest()
	r := m.Run()
	cleanup()
	os.Exit(r)
}

func getClient(t *testing.T) *Client {
	if client == nil {
		t.Skip("Integration tests skipped")
	}
	return client
}

var grpcHeadersChecker = testutil.DefaultHeadersEnforcer()

// If integration tests will be run, create a unique dataset for them.
// Return a cleanup function.
func initIntegrationTest() func() {
	ctx := context.Background()
	flag.Parse() // needed for testing.Short()
	projID := testutil.ProjID()
	switch {
	case testing.Short() && *record:
		log.Fatal("cannot combine -short and -record")
		return func() {}

	case testing.Short() && httpreplay.Supported() && testutil.CanReplay(replayFilename) && projID != "":
		// go test -short with a replay file will replay the integration tests if the
		// environment variables are set.
		log.Printf("replaying from %s", replayFilename)
		httpreplay.DebugHeaders()
		replayer, err := httpreplay.NewReplayer(replayFilename)
		if err != nil {
			log.Fatal(err)
		}
		var t time.Time
		if err := json.Unmarshal(replayer.Initial(), &t); err != nil {
			log.Fatal(err)
		}
		hc, err := replayer.Client(ctx) // no creds needed
		if err != nil {
			log.Fatal(err)
		}
		client, err = NewClient(ctx, projID, option.WithHTTPClient(hc))
		if err != nil {
			log.Fatal(err)
		}
		storageOptimizedClient, err = NewClient(ctx, projID, option.WithHTTPClient(hc))
		if err != nil {
			log.Fatal(err)
		}
		err = storageOptimizedClient.EnableStorageReadClient(ctx)
		if err != nil {
			log.Fatal(err)
		}
		storageClient, err = storage.NewClient(ctx, option.WithHTTPClient(hc))
		if err != nil {
			log.Fatal(err)
		}
		connectionsClient, err = connection.NewClient(ctx, option.WithHTTPClient(hc))
		if err != nil {
			log.Fatal(err)
		}
		policyTagManagerClient, err = datacatalog.NewPolicyTagManagerClient(ctx)
		if err != nil {
			log.Fatal(err)
		}
		cleanup := initTestState(client, t)
		return func() {
			cleanup()
			_ = replayer.Close() // No actionable error returned.
		}

	case testing.Short():
		// go test -short without a replay file skips the integration tests.
		if testutil.CanReplay(replayFilename) && projID != "" {
			log.Print("replay not supported for Go versions before 1.8")
		}
		client = nil
		storageOptimizedClient = nil
		storageClient = nil
		connectionsClient = nil
		return func() {}

	default: // Run integration tests against a real backend.
		ts := testutil.TokenSource(ctx, Scope)
		if ts == nil {
			log.Println("Integration tests skipped. See CONTRIBUTING.md for details")
			return func() {}
		}
		bqOpts := []option.ClientOption{option.WithTokenSource(ts)}
		sOpts := []option.ClientOption{option.WithTokenSource(testutil.TokenSource(ctx, storage.ScopeFullControl))}
		ptmOpts := []option.ClientOption{option.WithTokenSource(testutil.TokenSource(ctx, datacatalog.DefaultAuthScopes()...))}
		connOpts := []option.ClientOption{option.WithTokenSource(testutil.TokenSource(ctx, connection.DefaultAuthScopes()...))}
		cleanup := func() {}
		now := time.Now().UTC()
		if *record {
			if !httpreplay.Supported() {
				log.Print("record not supported for Go versions before 1.8")
			} else {
				nowBytes, err := json.Marshal(now)
				if err != nil {
					log.Fatal(err)
				}
				recorder, err := httpreplay.NewRecorder(replayFilename, nowBytes)
				if err != nil {
					log.Fatalf("could not record: %v", err)
				}
				log.Printf("recording to %s", replayFilename)
				hc, err := recorder.Client(ctx, bqOpts...)
				if err != nil {
					log.Fatal(err)
				}
				bqOpts = append(bqOpts, option.WithHTTPClient(hc))
				hc, err = recorder.Client(ctx, sOpts...)
				if err != nil {
					log.Fatal(err)
				}
				sOpts = append(sOpts, option.WithHTTPClient(hc))
				cleanup = func() {
					if err := recorder.Close(); err != nil {
						log.Printf("saving recording: %v", err)
					}
				}
			}
		} else {
			// When we're not recording, do http header checking.
			// We can't check universally because option.WithHTTPClient is
			// incompatible with gRPC options.
			bqOpts = append(bqOpts, grpcHeadersChecker.CallOptions()...)
			sOpts = append(sOpts, grpcHeadersChecker.CallOptions()...)
			ptmOpts = append(ptmOpts, grpcHeadersChecker.CallOptions()...)
			connOpts = append(connOpts, grpcHeadersChecker.CallOptions()...)
		}
		var err error
		client, err = NewClient(ctx, projID, bqOpts...)
		if err != nil {
			log.Fatalf("NewClient: %v", err)
		}
		storageOptimizedClient, err = NewClient(ctx, projID, bqOpts...)
		if err != nil {
			log.Fatalf("NewClient: %v", err)
		}
		err = storageOptimizedClient.EnableStorageReadClient(ctx, bqOpts...)
		if err != nil {
			log.Fatalf("ConfigureStorageReadClient: %v", err)
		}
		storageClient, err = storage.NewClient(ctx, sOpts...)
		if err != nil {
			log.Fatalf("storage.NewClient: %v", err)
		}
		policyTagManagerClient, err = datacatalog.NewPolicyTagManagerClient(ctx, ptmOpts...)
		if err != nil {
			log.Fatalf("datacatalog.NewPolicyTagManagerClient: %v", err)
		}
		connectionsClient, err = connection.NewClient(ctx, connOpts...)
		if err != nil {
			log.Fatalf("connection.NewService: %v", err)
		}
		c := initTestState(client, now)
		return func() { c(); cleanup() }
	}
}

func initTestState(client *Client, t time.Time) func() {
	// BigQuery does not accept hyphens in dataset or table IDs, so we create IDs
	// with underscores.
	ctx := context.Background()
	opts := &uid.Options{Sep: '_', Time: t}
	datasetIDs = uid.NewSpace("dataset", opts)
	tableIDs = uid.NewSpace("table", opts)
	modelIDs = uid.NewSpace("model", opts)
	routineIDs = uid.NewSpace("routine", opts)
	testTableExpiration = t.Add(2 * time.Hour).Round(time.Second)
	// For replayability, seed the random source with t.
	Seed(t.UnixNano())

	prefixes := []string{
		"dataset_",                    // bigquery package tests
		"managedwriter_test_dataset_", // managedwriter package tests
	}
	for _, prefix := range prefixes {
		deleteDatasets(ctx, prefix)
	}

	dataset = client.Dataset(datasetIDs.New())
	otherDataset = client.Dataset(datasetIDs.New())

	if err := dataset.Create(ctx, nil); err != nil {
		log.Fatalf("creating dataset %s: %v", dataset.DatasetID, err)
	}
	if err := otherDataset.Create(ctx, nil); err != nil {
		log.Fatalf("creating other dataset %s: %v", dataset.DatasetID, err)
	}

	return func() {
		if err := dataset.DeleteWithContents(ctx); err != nil {
			log.Printf("could not delete %s", dataset.DatasetID)
		}
		if err := otherDataset.DeleteWithContents(ctx); err != nil {
			log.Printf("could not delete %s", dataset.DatasetID)
		}
	}
}

// delete a resource if it is older than a day
// that will prevent collisions with parallel CI test runs.
func isResourceStale(t time.Time) bool {
	return time.Since(t).Hours() >= 24
}

// delete old datasets
func deleteDatasets(ctx context.Context, prefix string) {
	it := client.Datasets(ctx)

	for {
		ds, err := it.Next()
		if err == iterator.Done {
			break
		}
		if err != nil {
			fmt.Printf("failed to list project datasets: %v\n", err)
			break
		}
		if !strings.HasPrefix(ds.DatasetID, prefix) {
			continue
		}

		md, err := ds.Metadata(ctx)
		if err != nil {
			fmt.Printf("failed to get dataset `%s` metadata: %v\n", ds.DatasetID, err)
			continue
		}
		if isResourceStale(md.CreationTime) {
			fmt.Printf("found old dataset to delete: %s\n", ds.DatasetID)
			err := ds.DeleteWithContents(ctx)
			if err != nil {
				fmt.Printf("failed to delete old dataset `%s`\n", ds.DatasetID)
			}
		}
	}
}

func TestIntegration_DetectProjectID(t *testing.T) {
	ctx := context.Background()
	testCreds := testutil.Credentials(ctx)
	if testCreds == nil {
		t.Skip("test credentials not present, skipping")
	}

	if _, err := NewClient(ctx, DetectProjectID, option.WithCredentials(testCreds)); err != nil {
		t.Errorf("test NewClient: %v", err)
	}

	badTS := testutil.ErroringTokenSource{}

	if badClient, err := NewClient(ctx, DetectProjectID, option.WithTokenSource(badTS)); err == nil {
		t.Errorf("expected error from bad token source, NewClient succeeded with project: %s", badClient.Project())
	}
}

func TestIntegration_JobFrom(t *testing.T) {
	if client == nil {
		t.Skip("Integration tests skipped")
	}
	ctx := context.Background()

	// Create a job we can use for referencing.
	q := client.Query("SELECT 123 as foo")
	it, err := q.Read(ctx)
	if err != nil {
		t.Fatalf("failed to run test query: %v", err)
	}
	want := it.SourceJob()

	// establish a new client that's pointed at an invalid project/location.
	otherClient, err := NewClient(ctx, "bad-project-id")
	if err != nil {
		t.Fatalf("failed to create other client: %v", err)
	}
	otherClient.Location = "badloc"

	for _, tc := range []struct {
		description string
		f           func(*Client) (*Job, error)
		wantErr     bool
	}{
		{
			description: "JobFromID",
			f:           func(c *Client) (*Job, error) { return c.JobFromID(ctx, want.jobID) },
			wantErr:     true,
		},
		{
			description: "JobFromIDLocation",
			f:           func(c *Client) (*Job, error) { return c.JobFromIDLocation(ctx, want.jobID, want.location) },
			wantErr:     true,
		},
		{
			description: "JobFromProject",
			f:           func(c *Client) (*Job, error) { return c.JobFromProject(ctx, want.projectID, want.jobID, want.location) },
		},
	} {
		got, err := tc.f(otherClient)
		if err != nil {
			if !tc.wantErr {
				t.Errorf("case %q errored: %v", tc.description, err)
			}
			continue
		}
		if tc.wantErr {
			t.Errorf("case %q got success, expected error", tc.description)
		}
		if got.projectID != want.projectID {
			t.Errorf("case %q projectID mismatch, got %s want %s", tc.description, got.projectID, want.projectID)
		}
		if got.location != want.location {
			t.Errorf("case %q location mismatch, got %s want %s", tc.description, got.location, want.location)
		}
		if got.jobID != want.jobID {
			t.Errorf("case %q jobID mismatch, got %s want %s", tc.description, got.jobID, want.jobID)
		}
		if got.Email() == "" {
			t.Errorf("case %q expected email to be populated, was empty", tc.description)
		}
	}

}

func TestIntegration_QueryContextTimeout(t *testing.T) {
	if client == nil {
		t.Skip("Integration tests skipped")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()

	q := client.Query("select count(*) from unnest(generate_array(1,1000000)), unnest(generate_array(1, 1000)) as foo")
	q.DisableQueryCache = true
	before := time.Now()
	_, err := q.Read(ctx)
	if err != context.DeadlineExceeded {
		t.Errorf("Read() error, wanted %v, got %v", context.DeadlineExceeded, err)
	}
	wantMaxDur := 500 * time.Millisecond
	if d := time.Since(before); d > wantMaxDur {
		t.Errorf("return duration too long, wanted max %v got %v", wantMaxDur, d)
	}
}

func TestIntegration_SnapshotRestoreClone(t *testing.T) {

	if client == nil {
		t.Skip("Integration tests skipped")
	}
	ctx := context.Background()

	// instantiate a base table via a CTAS
	baseTableID := tableIDs.New()
	qualified := fmt.Sprintf("`%s`.%s.%s", testutil.ProjID(), dataset.DatasetID, baseTableID)
	sql := fmt.Sprintf(`
		CREATE TABLE %s
		(
			sample_value INT64,
			groupid STRING,
		)
		AS
		SELECT
		CAST(RAND() * 100 AS INT64),
		CONCAT("group", CAST(CAST(RAND()*10 AS INT64) AS STRING))
		FROM
		UNNEST(GENERATE_ARRAY(0,999))
		`, qualified)
	if _, _, err := runQuerySQL(ctx, sql); err != nil {
		t.Fatalf("couldn't instantiate base table: %v", err)
	}

	// Create a snapshot.  We'll select our snapshot time explicitly to validate the snapshot time is the same.
	targetTime := time.Now()
	snapshotID := tableIDs.New()
	copier := dataset.Table(snapshotID).CopierFrom(dataset.Table(fmt.Sprintf("%s@%d", baseTableID, targetTime.UnixNano()/1e6)))
	copier.OperationType = SnapshotOperation
	job, err := copier.Run(ctx)
	if err != nil {
		t.Fatalf("couldn't run snapshot: %v", err)
	}
	err = wait(ctx, job)
	if err != nil {
		t.Fatalf("snapshot failed: %v", err)
	}

	// verify metadata on the snapshot
	meta, err := dataset.Table(snapshotID).Metadata(ctx)
	if err != nil {
		t.Fatalf("couldn't get metadata from snapshot: %v", err)
	}
	if meta.Type != Snapshot {
		t.Errorf("expected snapshot table type, got %s", meta.Type)
	}
	want := &SnapshotDefinition{
		BaseTableReference: dataset.Table(baseTableID),
		SnapshotTime:       targetTime,
	}
	if diff := testutil.Diff(meta.SnapshotDefinition, want, cmp.AllowUnexported(Table{}), cmpopts.IgnoreUnexported(Client{}), cmpopts.EquateApproxTime(time.Millisecond)); diff != "" {
		t.Fatalf("SnapshotDefinition differs.  got=-, want=+:\n%s", diff)
	}

	// execute a restore using the snapshot.
	restoreID := tableIDs.New()
	restorer := dataset.Table(restoreID).CopierFrom(dataset.Table(snapshotID))
	restorer.OperationType = RestoreOperation
	job, err = restorer.Run(ctx)
	if err != nil {
		t.Fatalf("couldn't run restore: %v", err)
	}
	err = wait(ctx, job)
	if err != nil {
		t.Fatalf("restore failed: %v", err)
	}

	restoreMeta, err := dataset.Table(restoreID).Metadata(ctx)
	if err != nil {
		t.Fatalf("couldn't get restored table metadata: %v", err)
	}

	if meta.NumBytes != restoreMeta.NumBytes {
		t.Errorf("bytes mismatch.  snap had %d bytes, restore had %d bytes", meta.NumBytes, restoreMeta.NumBytes)
	}
	if meta.NumRows != restoreMeta.NumRows {
		t.Errorf("row counts mismatch.  snap had %d rows, restore had %d rows", meta.NumRows, restoreMeta.NumRows)
	}
	if restoreMeta.Type != RegularTable {
		t.Errorf("table type mismatch, got %s want %s", restoreMeta.Type, RegularTable)
	}

	// Create a clone of the snapshot.
	cloneID := tableIDs.New()
	cloner := dataset.Table(cloneID).CopierFrom(dataset.Table(snapshotID))
	cloner.OperationType = CloneOperation

	job, err = cloner.Run(ctx)
	if err != nil {
		t.Fatalf("couldn't run clone: %v", err)
	}
	err = wait(ctx, job)
	if err != nil {
		t.Fatalf("clone failed: %v", err)
	}

	cloneMeta, err := dataset.Table(cloneID).Metadata(ctx)
	if err != nil {
		t.Fatalf("couldn't get restored table metadata: %v", err)
	}
	if meta.NumBytes != cloneMeta.NumBytes {
		t.Errorf("bytes mismatch.  snap had %d bytes, clone had %d bytes", meta.NumBytes, cloneMeta.NumBytes)
	}
	if meta.NumRows != cloneMeta.NumRows {
		t.Errorf("row counts mismatch.  snap had %d rows, clone had %d rows", meta.NumRows, cloneMeta.NumRows)
	}
	if cloneMeta.Type != RegularTable {
		t.Errorf("table type mismatch, got %s want %s", cloneMeta.Type, RegularTable)
	}
	if cloneMeta.CloneDefinition == nil {
		t.Errorf("expected CloneDefinition in (%q), was nil", cloneMeta.FullID)
	}
	if cloneMeta.CloneDefinition.BaseTableReference == nil {
		t.Errorf("expected CloneDefinition.BaseTableReference, was nil")
	}
	wantBase := dataset.Table(snapshotID)
	if !testutil.Equal(cloneMeta.CloneDefinition.BaseTableReference, wantBase, cmpopts.IgnoreUnexported(Table{})) {
		t.Errorf("mismatch in CloneDefinition.BaseTableReference.  Got %s, want %s", cloneMeta.CloneDefinition.BaseTableReference.FullyQualifiedName(), wantBase.FullyQualifiedName())
	}
}

func TestIntegration_HourTimePartitioning(t *testing.T) {
	if client == nil {
		t.Skip("Integration tests skipped")
	}
	ctx := context.Background()
	table := dataset.Table(tableIDs.New())

	schema := Schema{
		{Name: "name", Type: StringFieldType},
		{Name: "somevalue", Type: IntegerFieldType},
	}

	// define hourly ingestion-based partitioning.
	wantedTimePartitioning := &TimePartitioning{
		Type: HourPartitioningType,
	}

	err := table.Create(context.Background(), &TableMetadata{
		Schema:           schema,
		TimePartitioning: wantedTimePartitioning,
	})
	if err != nil {
		t.Fatal(err)
	}
	defer table.Delete(ctx)
	md, err := table.Metadata(ctx)
	if err != nil {
		t.Fatal(err)
	}

	if md.TimePartitioning == nil {
		t.Fatal("expected time partitioning, got nil")
	}
	if diff := testutil.Diff(md.TimePartitioning, wantedTimePartitioning); diff != "" {
		t.Fatalf("got=-, want=+:\n%s", diff)
	}
	if md.TimePartitioning.Type != wantedTimePartitioning.Type {
		t.Errorf("TimePartitioning interval mismatch: got %v, wanted %v", md.TimePartitioning.Type, wantedTimePartitioning.Type)
	}
}

func TestIntegration_RangePartitioning(t *testing.T) {
	if client == nil {
		t.Skip("Integration tests skipped")
	}
	ctx := context.Background()
	table := dataset.Table(tableIDs.New())

	schema := Schema{
		{Name: "name", Type: StringFieldType},
		{Name: "somevalue", Type: IntegerFieldType},
	}

	wantedRange := &RangePartitioningRange{
		Start:    0,
		End:      135,
		Interval: 25,
	}

	wantedPartitioning := &RangePartitioning{
		Field: "somevalue",
		Range: wantedRange,
	}

	err := table.Create(context.Background(), &TableMetadata{
		Schema:            schema,
		RangePartitioning: wantedPartitioning,
	})
	if err != nil {
		t.Fatal(err)
	}
	defer table.Delete(ctx)
	md, err := table.Metadata(ctx)
	if err != nil {
		t.Fatal(err)
	}

	if md.RangePartitioning == nil {
		t.Fatal("expected range partitioning, got nil")
	}
	got := md.RangePartitioning.Field
	if wantedPartitioning.Field != got {
		t.Errorf("RangePartitioning Field: got %v, want %v", got, wantedPartitioning.Field)
	}
	if md.RangePartitioning.Range == nil {
		t.Fatal("expected a range definition, got nil")
	}
	gotInt64 := md.RangePartitioning.Range.Start
	if gotInt64 != wantedRange.Start {
		t.Errorf("Range.Start: got %v, wanted %v", gotInt64, wantedRange.Start)
	}
	gotInt64 = md.RangePartitioning.Range.End
	if gotInt64 != wantedRange.End {
		t.Errorf("Range.End: got %v, wanted %v", gotInt64, wantedRange.End)
	}
	gotInt64 = md.RangePartitioning.Range.Interval
	if gotInt64 != wantedRange.Interval {
		t.Errorf("Range.Interval: got %v, wanted %v", gotInt64, wantedRange.Interval)
	}
}

func TestIntegration_RemoveTimePartitioning(t *testing.T) {
	if client == nil {
		t.Skip("Integration tests skipped")
	}
	ctx := context.Background()
	table := dataset.Table(tableIDs.New())
	want := 24 * time.Hour
	err := table.Create(ctx, &TableMetadata{
		ExpirationTime: testTableExpiration,
		TimePartitioning: &TimePartitioning{
			Expiration: want,
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	defer table.Delete(ctx)

	md, err := table.Metadata(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if got := md.TimePartitioning.Expiration; got != want {
		t.Fatalf("TimePartitioning expiration want = %v, got = %v", want, got)
	}

	// Remove time partitioning expiration
	md, err = table.Update(context.Background(), TableMetadataToUpdate{
		TimePartitioning: &TimePartitioning{Expiration: 0},
	}, md.ETag)
	if err != nil {
		t.Fatal(err)
	}

	want = time.Duration(0)
	if got := md.TimePartitioning.Expiration; got != want {
		t.Fatalf("TimeParitioning expiration want = %v, got = %v", want, got)
	}
}

// setupPolicyTag is a helper for setting up policy tags in the datacatalog service.
//
// It returns a string for a policy tag identifier and a cleanup function, or an error.
func setupPolicyTag(ctx context.Context) (string, func(), error) {
	location := "us"
	req := &datacatalogpb.CreateTaxonomyRequest{
		Parent: fmt.Sprintf("projects/%s/locations/%s", testutil.ProjID(), location),
		Taxonomy: &datacatalogpb.Taxonomy{
			// DisplayName must be unique across org.
			DisplayName: fmt.Sprintf("google-cloud-go bigquery testing taxonomy %d", time.Now().UnixNano()),
			Description: "Taxonomy created for google-cloud-go integration tests",
			ActivatedPolicyTypes: []datacatalogpb.Taxonomy_PolicyType{
				datacatalogpb.Taxonomy_FINE_GRAINED_ACCESS_CONTROL,
			},
		},
	}
	resp, err := policyTagManagerClient.CreateTaxonomy(ctx, req)
	if err != nil {
		return "", nil, fmt.Errorf("datacatalog.CreateTaxonomy: %v", err)
	}
	taxonomyID := resp.GetName()
	cleanupFunc := func() {
		policyTagManagerClient.DeleteTaxonomy(ctx, &datacatalogpb.DeleteTaxonomyRequest{
			Name: taxonomyID,
		})
	}

	tagReq := &datacatalogpb.CreatePolicyTagRequest{
		Parent: resp.GetName(),
		PolicyTag: &datacatalogpb.PolicyTag{
			DisplayName: "ExamplePolicyTag",
		},
	}
	tagResp, err := policyTagManagerClient.CreatePolicyTag(ctx, tagReq)
	if err != nil {
		// we're failed to create tags, but we did create taxonomy. clean it up and signal error.
		cleanupFunc()
		return "", nil, fmt.Errorf("datacatalog.CreatePolicyTag: %v", err)
	}
	return tagResp.GetName(), cleanupFunc, nil
}

func TestIntegration_ColumnACLs(t *testing.T) {
	if client == nil {
		t.Skip("Integration tests skipped")
	}
	ctx := context.Background()
	testSchema := Schema{
		{Name: "name", Type: StringFieldType},
		{Name: "ssn", Type: StringFieldType},
		{Name: "acct_balance", Type: NumericFieldType},
	}
	table := newTable(t, testSchema)
	defer table.Delete(ctx)

	tagID, cleanupFunc, err := setupPolicyTag(ctx)
	if err != nil {
		t.Fatalf("failed to setup policy tag resources: %v", err)
	}
	defer cleanupFunc()
	// amend the test schema to add a policy tag
	testSchema[1].PolicyTags = &PolicyTagList{
		Names: []string{tagID},
	}

	// Test: Amend an existing schema with a policy tag.
	_, err = table.Update(ctx, TableMetadataToUpdate{
		Schema: testSchema,
	}, "")
	if err != nil {
		t.Errorf("update with policyTag failed: %v", err)
	}

	// Test: Create a new table with a policy tag defined.
	newTable := dataset.Table(tableIDs.New())
	if err = newTable.Create(ctx, &TableMetadata{
		Schema:      schema,
		Description: "foo",
	}); err != nil {
		t.Errorf("failed to create new table with policy tag: %v", err)
	}
}

func TestIntegration_SimpleRowResults(t *testing.T) {
	if client == nil {
		t.Skip("Integration tests skipped")
	}
	beforePreview := client.enableQueryPreview
	// ensure we restore the preview setting on test exit
	defer func() {
		client.enableQueryPreview = beforePreview
	}()
	ctx := context.Background()

	testCases := []struct {
		description string
		query       string
		want        [][]Value
	}{
		{
			description: "literals",
			query:       "select 17 as foo",
			want:        [][]Value{{int64(17)}},
		},
		{
			description: "empty results",
			query:       "SELECT * FROM (select 17 as foo) where false",
			want:        [][]Value{},
		},
		{
			// Previously this would return rows due to the destination reference being present
			// in the job config, but switching to relying on jobs.getQueryResults allows the
			// service to decide the behavior.
			description: "ctas ddl",
			query:       fmt.Sprintf("CREATE OR REPLACE TABLE %s.%s AS SELECT 17 as foo", dataset.DatasetID, tableIDs.New()),
			want:        nil,
		},
		{
			// This is a longer running query to ensure probing works as expected.
			description: "long running",
			query:       "select count(*) from unnest(generate_array(1,1000000)), unnest(generate_array(1, 1000)) as foo",
			want:        [][]Value{{int64(1000000000)}},
		},
		{
			// Query doesn't yield a result.
			description: "DML",
			query:       fmt.Sprintf("CREATE OR REPLACE TABLE %s.%s (foo STRING, bar INT64)", dataset.DatasetID, tableIDs.New()),
			want:        [][]Value{},
		},
	}

	t.Run("nopreview_group", func(t *testing.T) {
		client.enableQueryPreview = false
		for _, tc := range testCases {
			curCase := tc
			t.Run(curCase.description, func(t *testing.T) {
				t.Parallel()
				q := client.Query(curCase.query)
				it, err := q.Read(ctx)
				if err != nil {
					t.Fatalf("%s read error: %v", curCase.description, err)
				}
				checkReadAndTotalRows(t, curCase.description, it, curCase.want)
			})
		}
	})
	t.Run("preview_group", func(t *testing.T) {
		client.enableQueryPreview = true
		for _, tc := range testCases {
			curCase := tc
			t.Run(curCase.description, func(t *testing.T) {
				t.Parallel()
				q := client.Query(curCase.query)
				it, err := q.Read(ctx)
				if err != nil {
					t.Fatalf("%s read error: %v", curCase.description, err)
				}
				checkReadAndTotalRows(t, curCase.description, it, curCase.want)
			})
		}
	})

}

func TestIntegration_QueryIterationPager(t *testing.T) {
	if client == nil {
		t.Skip("Integration tests skipped")
	}
	ctx := context.Background()

	sql := `
	SELECT
		num,
		num * 2 as double
	FROM
		UNNEST(GENERATE_ARRAY(1,5)) as num`
	q := client.Query(sql)
	it, err := q.Read(ctx)
	if err != nil {
		t.Fatalf("Read: %v", err)
	}
	pager := iterator.NewPager(it, 2, "")
	rowsFetched := 0
	for {
		var rows [][]Value
		nextPageToken, err := pager.NextPage(&rows)
		if err != nil {
			t.Fatalf("NextPage: %v", err)
		}
		rowsFetched = rowsFetched + len(rows)

		if nextPageToken == "" {
			break
		}
	}

	wantRows := 5
	if rowsFetched != wantRows {
		t.Errorf("Expected %d rows, got %d", wantRows, rowsFetched)
	}
}

func TestIntegration_RoutineStoredProcedure(t *testing.T) {
	// Verifies we're exhibiting documented behavior, where we're expected
	// to return the last resultset in a script as the response from a script
	// job.
	// https://github.com/googleapis/google-cloud-go/issues/1974
	if client == nil {
		t.Skip("Integration tests skipped")
	}
	ctx := context.Background()

	// Define a simple stored procedure via DDL.
	routineID := routineIDs.New()
	routine := dataset.Routine(routineID)
	routineSQLID, _ := routine.Identifier(StandardSQLID)
	sql := fmt.Sprintf(`
		CREATE OR REPLACE PROCEDURE %s(val INT64)
		BEGIN
			SELECT CURRENT_TIMESTAMP() as ts;
			SELECT val * 2 as f2;
		END`,
		routineSQLID)

	if _, _, err := runQuerySQL(ctx, sql); err != nil {
		t.Fatal(err)
	}
	defer routine.Delete(ctx)

	// Invoke the stored procedure.
	sql = fmt.Sprintf(`
	CALL %s(5)`,
		routineSQLID)

	q := client.Query(sql)
	it, err := q.Read(ctx)
	if err != nil {
		t.Fatalf("query.Read: %v", err)
	}

	checkReadAndTotalRows(t,
		"expect result set from procedure",
		it, [][]Value{{int64(10)}})
}

func TestIntegration_RoutineUserTVF(t *testing.T) {
	if client == nil {
		t.Skip("Integration tests skipped")
	}
	ctx := context.Background()

	routineID := routineIDs.New()
	routine := dataset.Routine(routineID)
	inMeta := &RoutineMetadata{
		Type:     "TABLE_VALUED_FUNCTION",
		Language: "SQL",
		Arguments: []*RoutineArgument{
			{Name: "filter",
				DataType: &StandardSQLDataType{TypeKind: "INT64"},
			}},
		ReturnTableType: &StandardSQLTableType{
			Columns: []*StandardSQLField{
				{Name: "x", Type: &StandardSQLDataType{TypeKind: "INT64"}},
			},
		},
		Body: "SELECT x FROM UNNEST([1,2,3]) x WHERE x = filter",
	}
	if err := routine.Create(ctx, inMeta); err != nil {
		t.Fatalf("routine create: %v", err)
	}
	defer routine.Delete(ctx)

	meta, err := routine.Metadata(ctx)
	if err != nil {
		t.Fatal(err)
	}

	// Now, compare the input meta to the output meta
	if diff := testutil.Diff(inMeta, meta, cmpopts.IgnoreFields(RoutineMetadata{}, "CreationTime", "LastModifiedTime", "ETag")); diff != "" {
		t.Errorf("routine metadata differs, got=-, want=+\n%s", diff)
	}
}

func TestIntegration_InsertErrors(t *testing.T) {
	// This test serves to verify streaming behavior in the face of oversized data.
	// BigQuery will reject insertAll payloads that exceed a defined limit (10MB).
	// Additionally, if a payload vastly exceeds this limit, the request is rejected
	// by the intermediate architecture.
	if client == nil {
		t.Skip("Integration tests skipped")
	}
	ctx := context.Background()
	table := newTable(t, schema)
	defer table.Delete(ctx)

	ins := table.Inserter()
	var saverRows []*ValuesSaver

	// badSaver represents an excessively sized (>10MB) row message for insertion.
	badSaver := &ValuesSaver{
		Schema:   schema,
		InsertID: NoDedupeID,
		Row:      []Value{strings.Repeat("X", 10485760), []Value{int64(1)}, []Value{true}},
	}

	saverRows = append(saverRows, badSaver)
	err := ins.Put(ctx, saverRows)
	if err == nil {
		t.Errorf("Wanted row size error, got successful insert.")
	}
	var e1 *googleapi.Error
	ok := errors.As(err, &e1)
	if !ok {
		t.Errorf("Wanted googleapi.Error, got: %v", err)
	}
	if e1.Code != http.StatusRequestEntityTooLarge {
		want := "Request payload size exceeds the limit"
		if !strings.Contains(e1.Message, want) {
			t.Errorf("Error didn't contain expected message (%s): %#v", want, e1)
		}
	}
	// Case 2: Very Large Request
	// Request so large it gets rejected by intermediate infra (3x 10MB rows)
	saverRows = append(saverRows, badSaver)
	saverRows = append(saverRows, badSaver)

	err = ins.Put(ctx, saverRows)
	if err == nil {
		t.Errorf("Wanted error, got successful insert.")
	}
	var e2 *googleapi.Error
	ok = errors.As(err, &e2)
	if !ok {
		t.Errorf("wanted googleapi.Error, got: %v", err)
	}
	if e2.Code != http.StatusBadRequest && e2.Code != http.StatusRequestEntityTooLarge {
		t.Errorf("Wanted HTTP 400 or 413, got %d", e2.Code)
	}
}

func TestIntegration_InsertAndRead(t *testing.T) {
	if client == nil {
		t.Skip("Integration tests skipped")
	}
	ctx := context.Background()
	table := newTable(t, schema)
	defer table.Delete(ctx)

	// Populate the table.
	ins := table.Inserter()
	var (
		wantRows  [][]Value
		saverRows []*ValuesSaver
	)
	for i, name := range []string{"a", "b", "c"} {
		row := []Value{name, []Value{int64(i)}, []Value{true}}
		wantRows = append(wantRows, row)
		saverRows = append(saverRows, &ValuesSaver{
			Schema:   schema,
			InsertID: name,
			Row:      row,
		})
	}
	if err := ins.Put(ctx, saverRows); err != nil {
		t.Fatal(putError(err))
	}

	// Wait until the data has been uploaded. This can take a few seconds, according
	// to https://cloud.google.com/bigquery/streaming-data-into-bigquery.
	if err := waitForRow(ctx, table); err != nil {
		t.Fatal(err)
	}
	// Read the table.
	checkRead(t, "upload", table.Read(ctx), wantRows)

	// Query the table.
	q := client.Query(fmt.Sprintf("select name, nums, rec from %s", table.TableID))
	q.DefaultProjectID = dataset.ProjectID
	q.DefaultDatasetID = dataset.DatasetID

	rit, err := q.Read(ctx)
	if err != nil {
		t.Fatal(err)
	}
	checkRead(t, "query", rit, wantRows)

	// Query the long way.
	job1, err := q.Run(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if job1.LastStatus() == nil {
		t.Error("no LastStatus")
	}
	job2, err := client.JobFromID(ctx, job1.ID())
	if err != nil {
		t.Fatal(err)
	}
	if job2.LastStatus() == nil {
		t.Error("no LastStatus")
	}
	rit, err = job2.Read(ctx)
	if err != nil {
		t.Fatal(err)
	}
	checkRead(t, "job.Read", rit, wantRows)

	// Get statistics.
	jobStatus, err := job2.Status(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if jobStatus.Statistics == nil {
		t.Fatal("jobStatus missing statistics")
	}
	if _, ok := jobStatus.Statistics.Details.(*QueryStatistics); !ok {
		t.Errorf("expected QueryStatistics, got %T", jobStatus.Statistics.Details)
	}

	// Test reading directly into a []Value.
	valueLists, schema, _, err := readAll(table.Read(ctx))
	if err != nil {
		t.Fatal(err)
	}
	it := table.Read(ctx)
	for i, vl := range valueLists {
		var got []Value
		if err := it.Next(&got); err != nil {
			t.Fatal(err)
		}
		if !testutil.Equal(it.Schema, schema) {
			t.Fatalf("got schema %v, want %v", it.Schema, schema)
		}
		want := []Value(vl)
		if !testutil.Equal(got, want) {
			t.Errorf("%d: got %v, want %v", i, got, want)
		}
	}

	// Test reading into a map.
	it = table.Read(ctx)
	for _, vl := range valueLists {
		var vm map[string]Value
		if err := it.Next(&vm); err != nil {
			t.Fatal(err)
		}
		if got, want := len(vm), len(vl); got != want {
			t.Fatalf("valueMap len: got %d, want %d", got, want)
		}
		// With maps, structs become nested maps.
		vl[2] = map[string]Value{"bool": vl[2].([]Value)[0]}
		for i, v := range vl {
			if got, want := vm[schema[i].Name], v; !testutil.Equal(got, want) {
				t.Errorf("%d, name=%s: got %#v, want %#v",
					i, schema[i].Name, got, want)
			}
		}
	}

}

type SubSubTestStruct struct {
	Integer int64
}

type SubTestStruct struct {
	String      string
	Record      SubSubTestStruct
	RecordArray []SubSubTestStruct
}

type TestStruct struct {
	Name      string
	Bytes     []byte
	Integer   int64
	Float     float64
	Boolean   bool
	Timestamp time.Time
	Date      civil.Date
	Time      civil.Time
	DateTime  civil.DateTime
	Numeric   *big.Rat
	Geography string

	StringArray    []string
	IntegerArray   []int64
	FloatArray     []float64
	BooleanArray   []bool
	TimestampArray []time.Time
	DateArray      []civil.Date
	TimeArray      []civil.Time
	DateTimeArray  []civil.DateTime
	NumericArray   []*big.Rat
	GeographyArray []string

	Record      SubTestStruct
	RecordArray []SubTestStruct
}

// Round times to the microsecond for comparison purposes.
var roundToMicros = cmp.Transformer("RoundToMicros",
	func(t time.Time) time.Time { return t.Round(time.Microsecond) })

func TestIntegration_InsertAndReadStructs(t *testing.T) {
	if client == nil {
		t.Skip("Integration tests skipped")
	}
	schema, err := InferSchema(TestStruct{})
	if err != nil {
		t.Fatal(err)
	}

	ctx := context.Background()
	table := newTable(t, schema)
	defer table.Delete(ctx)

	d := civil.Date{Year: 2016, Month: 3, Day: 20}
	tm := civil.Time{Hour: 15, Minute: 4, Second: 5, Nanosecond: 6000}
	ts := time.Date(2016, 3, 20, 15, 4, 5, 6000, time.UTC)
	dtm := civil.DateTime{Date: d, Time: tm}
	d2 := civil.Date{Year: 1994, Month: 5, Day: 15}
	tm2 := civil.Time{Hour: 1, Minute: 2, Second: 4, Nanosecond: 0}
	ts2 := time.Date(1994, 5, 15, 1, 2, 4, 0, time.UTC)
	dtm2 := civil.DateTime{Date: d2, Time: tm2}
	g := "POINT(-122.350220 47.649154)"
	g2 := "POINT(-122.0836791 37.421827)"

	// Populate the table.
	ins := table.Inserter()
	want := []*TestStruct{
		{
			"a",
			[]byte("byte"),
			42,
			3.14,
			true,
			ts,
			d,
			tm,
			dtm,
			big.NewRat(57, 100),
			g,
			[]string{"a", "b"},
			[]int64{1, 2},
			[]float64{1, 1.41},
			[]bool{true, false},
			[]time.Time{ts, ts2},
			[]civil.Date{d, d2},
			[]civil.Time{tm, tm2},
			[]civil.DateTime{dtm, dtm2},
			[]*big.Rat{big.NewRat(1, 2), big.NewRat(3, 5)},
			[]string{g, g2},
			SubTestStruct{
				"string",
				SubSubTestStruct{24},
				[]SubSubTestStruct{{1}, {2}},
			},
			[]SubTestStruct{
				{String: "empty"},
				{
					"full",
					SubSubTestStruct{1},
					[]SubSubTestStruct{{1}, {2}},
				},
			},
		},
		{
			Name:      "b",
			Bytes:     []byte("byte2"),
			Integer:   24,
			Float:     4.13,
			Boolean:   false,
			Timestamp: ts,
			Date:      d,
			Time:      tm,
			DateTime:  dtm,
			Numeric:   big.NewRat(4499, 10000),
		},
	}
	var savers []*StructSaver
	for _, s := range want {
		savers = append(savers, &StructSaver{Schema: schema, Struct: s})
	}
	if err := ins.Put(ctx, savers); err != nil {
		t.Fatal(putError(err))
	}

	// Wait until the data has been uploaded. This can take a few seconds, according
	// to https://cloud.google.com/bigquery/streaming-data-into-bigquery.
	if err := waitForRow(ctx, table); err != nil {
		t.Fatal(err)
	}

	// Test iteration with structs.
	it := table.Read(ctx)
	var got []*TestStruct
	for {
		var g TestStruct
		err := it.Next(&g)
		if err == iterator.Done {
			break
		}
		if err != nil {
			t.Fatal(err)
		}
		got = append(got, &g)
	}
	sort.Sort(byName(got))

	// BigQuery does not elide nils. It reports an error for nil fields.
	for i, g := range got {
		if i >= len(want) {
			t.Errorf("%d: got %v, past end of want", i, pretty.Value(g))
		} else if diff := testutil.Diff(g, want[i], roundToMicros); diff != "" {
			t.Errorf("%d: got=-, want=+:\n%s", i, diff)
		}
	}
}

type byName []*TestStruct

func (b byName) Len() int           { return len(b) }
func (b byName) Swap(i, j int)      { b[i], b[j] = b[j], b[i] }
func (b byName) Less(i, j int) bool { return b[i].Name < b[j].Name }

func TestIntegration_InsertAndReadNullable(t *testing.T) {
	if client == nil {
		t.Skip("Integration tests skipped")
	}
	ctm := civil.Time{Hour: 15, Minute: 4, Second: 5, Nanosecond: 6000}
	cdt := civil.DateTime{Date: testDate, Time: ctm}
	rat := big.NewRat(33, 100)
	rat2 := big.NewRat(66, 10e10)
	geo := "POINT(-122.198939 47.669865)"

	// Nil fields in the struct.
	testInsertAndReadNullable(t, testStructNullable{}, make([]Value, len(testStructNullableSchema)))

	// Explicitly invalidate the Null* types within the struct.
	testInsertAndReadNullable(t, testStructNullable{
		String:    NullString{Valid: false},
		Integer:   NullInt64{Valid: false},
		Float:     NullFloat64{Valid: false},
		Boolean:   NullBool{Valid: false},
		Timestamp: NullTimestamp{Valid: false},
		Date:      NullDate{Valid: false},
		Time:      NullTime{Valid: false},
		DateTime:  NullDateTime{Valid: false},
		Geography: NullGeography{Valid: false},
	},
		make([]Value, len(testStructNullableSchema)))

	// Populate the struct with values.
	testInsertAndReadNullable(t, testStructNullable{
		String:     NullString{"x", true},
		Bytes:      []byte{1, 2, 3},
		Integer:    NullInt64{1, true},
		Float:      NullFloat64{2.3, true},
		Boolean:    NullBool{true, true},
		Timestamp:  NullTimestamp{testTimestamp, true},
		Date:       NullDate{testDate, true},
		Time:       NullTime{ctm, true},
		DateTime:   NullDateTime{cdt, true},
		Numeric:    rat,
		BigNumeric: rat2,
		Geography:  NullGeography{geo, true},
		Record:     &subNullable{X: NullInt64{4, true}},
	},
		[]Value{"x", []byte{1, 2, 3}, int64(1), 2.3, true, testTimestamp, testDate, ctm, cdt, rat, rat2, geo, []Value{int64(4)}})
}

func testInsertAndReadNullable(t *testing.T, ts testStructNullable, wantRow []Value) {
	ctx := context.Background()
	table := newTable(t, testStructNullableSchema)
	defer table.Delete(ctx)

	// Populate the table.
	ins := table.Inserter()
	if err := ins.Put(ctx, []*StructSaver{{Schema: testStructNullableSchema, Struct: ts}}); err != nil {
		t.Fatal(putError(err))
	}
	// Wait until the data has been uploaded. This can take a few seconds, according
	// to https://cloud.google.com/bigquery/streaming-data-into-bigquery.
	if err := waitForRow(ctx, table); err != nil {
		t.Fatal(err)
	}

	// Read into a []Value.
	iter := table.Read(ctx)
	gotRows, _, _, err := readAll(iter)
	if err != nil {
		t.Fatal(err)
	}
	if len(gotRows) != 1 {
		t.Fatalf("got %d rows, want 1", len(gotRows))
	}
	if diff := testutil.Diff(gotRows[0], wantRow, roundToMicros); diff != "" {
		t.Error(diff)
	}

	// Read into a struct.
	want := ts
	var sn testStructNullable
	it := table.Read(ctx)
	if err := it.Next(&sn); err != nil {
		t.Fatal(err)
	}
	if diff := testutil.Diff(sn, want, roundToMicros); diff != "" {
		t.Error(diff)
	}
}

func TestIntegration_QueryStatistics(t *testing.T) {
	// Make a bunch of assertions on a simple query.
	if client == nil {
		t.Skip("Integration tests skipped")
	}
	ctx := context.Background()

	q := client.Query("SELECT 17 as foo, 3.14 as bar")
	// disable cache to ensure we have query statistics
	q.DisableQueryCache = true

	job, err := q.Run(ctx)
	if err != nil {
		t.Fatalf("job Run failure: %v", err)
	}
	status, err := job.Wait(ctx)
	if err != nil {
		t.Fatalf("job %q: Wait failure: %v", job.ID(), err)
	}
	if status.Statistics == nil {
		t.Fatal("expected job statistics, none found")
	}

	if status.Statistics.NumChildJobs != 0 {
		t.Errorf("expected no children, %d reported", status.Statistics.NumChildJobs)
	}

	if status.Statistics.ParentJobID != "" {
		t.Errorf("expected no parent, but parent present: %s", status.Statistics.ParentJobID)
	}

	if status.Statistics.Details == nil {
		t.Fatal("expected job details, none present")
	}

	qStats, ok := status.Statistics.Details.(*QueryStatistics)
	if !ok {
		t.Fatalf("expected query statistics not present")
	}

	if qStats.CacheHit {
		t.Error("unexpected cache hit")
	}

	if qStats.StatementType != "SELECT" {
		t.Errorf("expected SELECT statement type, got: %s", qStats.StatementType)
	}

	if len(qStats.QueryPlan) == 0 {
		t.Error("expected query plan, none present")
	}

	if len(qStats.Timeline) == 0 {
		t.Error("expected query timeline, none present")
	}

	if qStats.BIEngineStatistics != nil {
		expectedMode := false
		for _, m := range []string{"FULL", "PARTIAL", "DISABLED"} {
			if qStats.BIEngineStatistics.BIEngineMode == m {
				expectedMode = true
			}
		}
		if !expectedMode {
			t.Errorf("unexpected BIEngineMode for BI Engine statistics, got %s", qStats.BIEngineStatistics.BIEngineMode)
		}
	}
}

func TestIntegration_Load(t *testing.T) {
	if client == nil {
		t.Skip("Integration tests skipped")
	}
	ctx := context.Background()
	// CSV data can't be loaded into a repeated field, so we use a different schema.
	table := newTable(t, Schema{
		{Name: "name", Type: StringFieldType},
		{Name: "nums", Type: IntegerFieldType},
	})
	defer table.Delete(ctx)

	// Load the table from a reader.
	r := strings.NewReader("a,0\nb,1\nc,2\n")
	wantRows := [][]Value{
		{"a", int64(0)},
		{"b", int64(1)},
		{"c", int64(2)},
	}
	rs := NewReaderSource(r)
	loader := table.LoaderFrom(rs)
	loader.WriteDisposition = WriteTruncate
	loader.Labels = map[string]string{"test": "go"}
	loader.MediaOptions = []googleapi.MediaOption{
		googleapi.ContentType("text/csv"),
		googleapi.ChunkSize(googleapi.MinUploadChunkSize),
	}
	job, err := loader.Run(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if job.LastStatus() == nil {
		t.Error("no LastStatus")
	}
	conf, err := job.Config()
	if err != nil {
		t.Fatal(err)
	}
	config, ok := conf.(*LoadConfig)
	if !ok {
		t.Fatalf("got %T, want LoadConfig", conf)
	}
	diff := testutil.Diff(config, &loader.LoadConfig,
		cmp.AllowUnexported(Table{}),
		cmpopts.IgnoreUnexported(Client{}, ReaderSource{}),
		// returned schema is at top level, not in the config
		cmpopts.IgnoreFields(FileConfig{}, "Schema"),
		cmpopts.IgnoreFields(LoadConfig{}, "MediaOptions"))
	if diff != "" {
		t.Errorf("got=-, want=+:\n%s", diff)
	}
	if err := wait(ctx, job); err != nil {
		t.Fatal(err)
	}
	checkReadAndTotalRows(t, "reader load", table.Read(ctx), wantRows)
}

func TestIntegration_LoadWithSessionSupport(t *testing.T) {
	if client == nil {
		t.Skip("Integration tests skipped")
	}

	ctx := context.Background()
	sessionDataset := client.Dataset("_SESSION")
	sessionTable := sessionDataset.Table("test_temp_destination_table")

	schema := Schema{
		{Name: "username", Type: StringFieldType, Required: false},
		{Name: "tweet", Type: StringFieldType, Required: false},
		{Name: "timestamp", Type: StringFieldType, Required: false},
		{Name: "likes", Type: IntegerFieldType, Required: false},
	}
	sourceURIs := []string{
		"gs://cloud-samples-data/bigquery/federated-formats-reference-file-schema/a-twitter.parquet",
	}

	source := NewGCSReference(sourceURIs...)
	source.SourceFormat = Parquet
	source.Schema = schema
	loader := sessionTable.LoaderFrom(source)
	loader.CreateSession = true
	loader.CreateDisposition = CreateIfNeeded

	job, err := loader.Run(ctx)
	if err != nil {
		t.Fatalf("loader.Run: %v", err)
	}
	err = wait(ctx, job)
	if err != nil {
		t.Fatalf("wait: %v", err)
	}

	sessionInfo := job.lastStatus.Statistics.SessionInfo
	if sessionInfo == nil {
		t.Fatalf("empty job.lastStatus.Statistics.SessionInfo: %v", sessionInfo)
	}

	sessionID := sessionInfo.SessionID
	loaderWithSession := sessionTable.LoaderFrom(source)
	loaderWithSession.CreateDisposition = CreateIfNeeded
	loaderWithSession.ConnectionProperties = []*ConnectionProperty{
		{
			Key:   "session_id",
			Value: sessionID,
		},
	}
	jobWithSession, err := loaderWithSession.Run(ctx)
	if err != nil {
		t.Fatalf("loaderWithSession.Run: %v", err)
	}
	err = wait(ctx, jobWithSession)
	if err != nil {
		t.Fatalf("wait: %v", err)
	}

	sessionJobInfo := jobWithSession.lastStatus.Statistics.SessionInfo
	if sessionJobInfo == nil {
		t.Fatalf("empty jobWithSession.lastStatus.Statistics.SessionInfo: %v", sessionJobInfo)
	}

	if sessionID != sessionJobInfo.SessionID {
		t.Fatalf("expected session ID %q, but found %q", sessionID, sessionJobInfo.SessionID)
	}

	sql := "SELECT * FROM _SESSION.test_temp_destination_table;"
	q := client.Query(sql)
	q.ConnectionProperties = []*ConnectionProperty{
		{
			Key:   "session_id",
			Value: sessionID,
		},
	}
	sessionQueryJob, err := q.Run(ctx)
	err = wait(ctx, sessionQueryJob)
	if err != nil {
		t.Fatalf("wait: %v", err)
	}
}

func TestIntegration_LoadWithReferenceSchemaFile(t *testing.T) {
	if client == nil {
		t.Skip("Integration tests skipped")
	}

	formats := []DataFormat{Avro, Parquet}
	for _, format := range formats {
		ctx := context.Background()
		table := dataset.Table(tableIDs.New())
		defer table.Delete(ctx)

		expectedSchema := Schema{
			{Name: "username", Type: StringFieldType, Required: false},
			{Name: "tweet", Type: StringFieldType, Required: false},
			{Name: "timestamp", Type: StringFieldType, Required: false},
			{Name: "likes", Type: IntegerFieldType, Required: false},
		}
		ext := strings.ToLower(string(format))
		sourceURIs := []string{
			"gs://cloud-samples-data/bigquery/federated-formats-reference-file-schema/a-twitter." + ext,
			"gs://cloud-samples-data/bigquery/federated-formats-reference-file-schema/b-twitter." + ext,
			"gs://cloud-samples-data/bigquery/federated-formats-reference-file-schema/c-twitter." + ext,
		}
		referenceURI := "gs://cloud-samples-data/bigquery/federated-formats-reference-file-schema/a-twitter." + ext
		source := NewGCSReference(sourceURIs...)
		source.SourceFormat = format
		loader := table.LoaderFrom(source)
		loader.ReferenceFileSchemaURI = referenceURI
		job, err := loader.Run(ctx)
		if err != nil {
			t.Fatalf("loader.Run: %v", err)
		}
		err = wait(ctx, job)
		if err != nil {
			t.Fatalf("wait: %v", err)
		}
		metadata, err := table.Metadata(ctx)
		if err != nil {
			t.Fatalf("table.Metadata: %v", err)
		}
		diff := testutil.Diff(expectedSchema, metadata.Schema)
		if diff != "" {
			t.Errorf("got=-, want=+:\n%s", diff)
		}
	}
}

func TestIntegration_ExternalTableWithReferenceSchemaFile(t *testing.T) {
	if client == nil {
		t.Skip("Integration tests skipped")
	}

	formats := []DataFormat{Avro, Parquet}
	for _, format := range formats {
		ctx := context.Background()
		externalTable := dataset.Table(tableIDs.New())
		defer externalTable.Delete(ctx)

		expectedSchema := Schema{
			{Name: "username", Type: StringFieldType, Required: false},
			{Name: "tweet", Type: StringFieldType, Required: false},
			{Name: "timestamp", Type: StringFieldType, Required: false},
			{Name: "likes", Type: IntegerFieldType, Required: false},
		}
		ext := strings.ToLower(string(format))
		sourceURIs := []string{
			"gs://cloud-samples-data/bigquery/federated-formats-reference-file-schema/a-twitter." + ext,
			"gs://cloud-samples-data/bigquery/federated-formats-reference-file-schema/b-twitter." + ext,
			"gs://cloud-samples-data/bigquery/federated-formats-reference-file-schema/c-twitter." + ext,
		}
		referenceURI := "gs://cloud-samples-data/bigquery/federated-formats-reference-file-schema/a-twitter." + ext

		err := externalTable.Create(ctx, &TableMetadata{
			ExternalDataConfig: &ExternalDataConfig{
				SourceFormat:           format,
				SourceURIs:             sourceURIs,
				ReferenceFileSchemaURI: referenceURI,
			},
		})
		if err != nil {
			t.Fatalf("table.Create: %v", err)
		}

		metadata, err := externalTable.Metadata(ctx)
		if err != nil {
			t.Fatalf("table.Metadata: %v", err)
		}
		diff := testutil.Diff(expectedSchema, metadata.Schema)
		if diff != "" {
			t.Errorf("got=-, want=+:\n%s", diff)
		}
	}
}

func TestIntegration_DML(t *testing.T) {
	if client == nil {
		t.Skip("Integration tests skipped")
	}
	ctx := context.Background()
	table := newTable(t, schema)
	defer table.Delete(ctx)

	sql := fmt.Sprintf(`INSERT %s.%s (name, nums, rec)
						VALUES ('a', [0], STRUCT<BOOL>(TRUE)),
							   ('b', [1], STRUCT<BOOL>(FALSE)),
							   ('c', [2], STRUCT<BOOL>(TRUE))`,
		table.DatasetID, table.TableID)
	_, stats, err := runQuerySQL(ctx, sql)
	if err != nil {
		t.Fatal(err)
	}
	wantRows := [][]Value{
		{"a", []Value{int64(0)}, []Value{true}},
		{"b", []Value{int64(1)}, []Value{false}},
		{"c", []Value{int64(2)}, []Value{true}},
	}
	checkRead(t, "DML", table.Read(ctx), wantRows)
	if stats == nil {
		t.Fatalf("no query stats")
	}
	if stats.DMLStats == nil {
		t.Fatalf("no dml stats")
	}
	wantRowCount := int64(len(wantRows))
	if stats.DMLStats.InsertedRowCount != wantRowCount {
		t.Fatalf("dml stats mismatch.  got %d inserted rows, want %d", stats.DMLStats.InsertedRowCount, wantRowCount)
	}
}

// runQuerySQL runs arbitrary SQL text.
func runQuerySQL(ctx context.Context, sql string) (*JobStatistics, *QueryStatistics, error) {
	return runQueryJob(ctx, client.Query(sql))
}

// runQueryJob is useful for running queries where no row data is returned (DDL/DML).
func runQueryJob(ctx context.Context, q *Query) (*JobStatistics, *QueryStatistics, error) {
	var jobStats *JobStatistics
	var queryStats *QueryStatistics
	var err = internal.Retry(ctx, gax.Backoff{}, func() (stop bool, err error) {
		job, err := q.Run(ctx)
		if err != nil {
			var e *googleapi.Error
			if ok := errors.As(err, &e); ok && e.Code < 500 {
				return true, err // fail on 4xx
			}
			return false, err
		}
		_, err = job.Wait(ctx)
		if err != nil {
			var e *googleapi.Error
			if ok := errors.As(err, &e); ok && e.Code < 500 {
				return true, err // fail on 4xx
			}
			return false, fmt.Errorf("%q: %v", job.ID(), err)
		}
		status := job.LastStatus()
		if status.Err() != nil {
			return false, fmt.Errorf("job %q terminated in err: %v", job.ID(), status.Err())
		}
		if status.Statistics != nil {
			jobStats = status.Statistics
			if qStats, ok := status.Statistics.Details.(*QueryStatistics); ok {
				queryStats = qStats
			}
		}
		return true, nil
	})
	return jobStats, queryStats, err
}

func TestIntegration_TimeTypes(t *testing.T) {
	if client == nil {
		t.Skip("Integration tests skipped")
	}
	ctx := context.Background()
	dtSchema := Schema{
		{Name: "d", Type: DateFieldType},
		{Name: "t", Type: TimeFieldType},
		{Name: "dt", Type: DateTimeFieldType},
		{Name: "ts", Type: TimestampFieldType},
	}
	table := newTable(t, dtSchema)
	defer table.Delete(ctx)

	d := civil.Date{Year: 2016, Month: 3, Day: 20}
	tm := civil.Time{Hour: 12, Minute: 30, Second: 0, Nanosecond: 6000}
	dtm := civil.DateTime{Date: d, Time: tm}
	ts := time.Date(2016, 3, 20, 15, 04, 05, 0, time.UTC)
	wantRows := [][]Value{
		{d, tm, dtm, ts},
	}
	ins := table.Inserter()
	if err := ins.Put(ctx, []*ValuesSaver{
		{Schema: dtSchema, Row: wantRows[0]},
	}); err != nil {
		t.Fatal(putError(err))
	}
	if err := waitForRow(ctx, table); err != nil {
		t.Fatal(err)
	}

	// SQL wants DATETIMEs with a space between date and time, but the service
	// returns them in RFC3339 form, with a "T" between.
	query := fmt.Sprintf("INSERT %s.%s (d, t, dt, ts) "+
		"VALUES ('%s', '%s', '%s', '%s')",
		table.DatasetID, table.TableID,
		d, CivilTimeString(tm), CivilDateTimeString(dtm), ts.Format("2006-01-02 15:04:05"))
	if _, _, err := runQuerySQL(ctx, query); err != nil {
		t.Fatal(err)
	}
	wantRows = append(wantRows, wantRows[0])
	checkRead(t, "TimeTypes", table.Read(ctx), wantRows)
}

func TestIntegration_StandardQuery(t *testing.T) {
	if client == nil {
		t.Skip("Integration tests skipped")
	}
	ctx := context.Background()

	d := civil.Date{Year: 2016, Month: 3, Day: 20}
	tm := civil.Time{Hour: 15, Minute: 04, Second: 05, Nanosecond: 0}
	ts := time.Date(2016, 3, 20, 15, 04, 05, 0, time.UTC)
	dtm := ts.Format("2006-01-02 15:04:05")

	// Constructs Value slices made up of int64s.
	ints := func(args ...int) []Value {
		vals := make([]Value, len(args))
		for i, arg := range args {
			vals[i] = int64(arg)
		}
		return vals
	}

	testCases := []struct {
		query   string
		wantRow []Value
	}{
		{"SELECT 1", ints(1)},
		{"SELECT 1.3", []Value{1.3}},
		{"SELECT CAST(1.3  AS NUMERIC)", []Value{big.NewRat(13, 10)}},
		{"SELECT NUMERIC '0.25'", []Value{big.NewRat(1, 4)}},
		{"SELECT TRUE", []Value{true}},
		{"SELECT 'ABC'", []Value{"ABC"}},
		{"SELECT CAST('foo' AS BYTES)", []Value{[]byte("foo")}},
		{fmt.Sprintf("SELECT TIMESTAMP '%s'", dtm), []Value{ts}},
		{fmt.Sprintf("SELECT [TIMESTAMP '%s', TIMESTAMP '%s']", dtm, dtm), []Value{[]Value{ts, ts}}},
		{fmt.Sprintf("SELECT ('hello', TIMESTAMP '%s')", dtm), []Value{[]Value{"hello", ts}}},
		{fmt.Sprintf("SELECT DATETIME(TIMESTAMP '%s')", dtm), []Value{civil.DateTime{Date: d, Time: tm}}},
		{fmt.Sprintf("SELECT DATE(TIMESTAMP '%s')", dtm), []Value{d}},
		{fmt.Sprintf("SELECT TIME(TIMESTAMP '%s')", dtm), []Value{tm}},
		{"SELECT (1, 2)", []Value{ints(1, 2)}},
		{"SELECT [1, 2, 3]", []Value{ints(1, 2, 3)}},
		{"SELECT ([1, 2], 3, [4, 5])", []Value{[]Value{ints(1, 2), int64(3), ints(4, 5)}}},
		{"SELECT [(1, 2, 3), (4, 5, 6)]", []Value{[]Value{ints(1, 2, 3), ints(4, 5, 6)}}},
		{"SELECT [([1, 2, 3], 4), ([5, 6], 7)]", []Value{[]Value{[]Value{ints(1, 2, 3), int64(4)}, []Value{ints(5, 6), int64(7)}}}},
		{"SELECT ARRAY(SELECT STRUCT([1, 2]))", []Value{[]Value{[]Value{ints(1, 2)}}}},
	}
	for _, c := range testCases {
		q := client.Query(c.query)
		it, err := q.Read(ctx)
		if err != nil {
			t.Fatal(err)
		}
		checkRead(t, "StandardQuery", it, [][]Value{c.wantRow})
	}
}

func TestIntegration_LegacyQuery(t *testing.T) {
	if client == nil {
		t.Skip("Integration tests skipped")
	}
	ctx := context.Background()

	ts := time.Date(2016, 3, 20, 15, 04, 05, 0, time.UTC)
	dtm := ts.Format("2006-01-02 15:04:05")

	testCases := []struct {
		query   string
		wantRow []Value
	}{
		{"SELECT 1", []Value{int64(1)}},
		{"SELECT 1.3", []Value{1.3}},
		{"SELECT TRUE", []Value{true}},
		{"SELECT 'ABC'", []Value{"ABC"}},
		{"SELECT CAST('foo' AS BYTES)", []Value{[]byte("foo")}},
		{fmt.Sprintf("SELECT TIMESTAMP('%s')", dtm), []Value{ts}},
		{fmt.Sprintf("SELECT DATE(TIMESTAMP('%s'))", dtm), []Value{"2016-03-20"}},
		{fmt.Sprintf("SELECT TIME(TIMESTAMP('%s'))", dtm), []Value{"15:04:05"}},
	}
	for _, c := range testCases {
		q := client.Query(c.query)
		q.UseLegacySQL = true
		it, err := q.Read(ctx)
		if err != nil {
			t.Fatal(err)
		}
		checkRead(t, "LegacyQuery", it, [][]Value{c.wantRow})
	}
}

func TestIntegration_IteratorSource(t *testing.T) {
	if client == nil {
		t.Skip("Integration tests skipped")
	}
	ctx := context.Background()
	q := client.Query("SELECT 17 as foo")
	it, err := q.Read(ctx)
	if err != nil {
		t.Errorf("Read: %v", err)
	}
	src := it.SourceJob()
	if src == nil {
		t.Errorf("wanted source job, got nil")
	}
	status, err := src.Status(ctx)
	if err != nil {
		t.Errorf("Status: %v", err)
	}
	if status == nil {
		t.Errorf("got nil status")
	}
}

func TestIntegration_ExternalAutodetect(t *testing.T) {
	if client == nil {
		t.Skip("Integration tests skipped")
	}
	ctx := context.Background()

	testTable := dataset.Table(tableIDs.New())

	origExtCfg := &ExternalDataConfig{
		SourceFormat: Avro,
		SourceURIs:   []string{"gs://cloud-samples-data/bigquery/autodetect-samples/original*.avro"},
	}

	err := testTable.Create(ctx, &TableMetadata{
		ExternalDataConfig: origExtCfg,
	})
	if err != nil {
		t.Fatalf("Table.Create(%q): %v", testTable.FullyQualifiedName(), err)
	}

	origMeta, err := testTable.Metadata(ctx)
	if err != nil {
		t.Fatalf("Table.Metadata(%q): %v", testTable.FullyQualifiedName(), err)
	}

	wantSchema := Schema{
		{Name: "stringfield", Type: "STRING"},
		{Name: "int64field", Type: "INTEGER"},
	}
	if diff := testutil.Diff(origMeta.Schema, wantSchema); diff != "" {
		t.Fatalf("orig schema, got=-, want=+\n%s", diff)
	}

	// Now, point at the new files, but don't signal autodetect.
	newExtCfg := &ExternalDataConfig{
		SourceFormat: Avro,
		SourceURIs:   []string{"gs://cloud-samples-data/bigquery/autodetect-samples/widened*.avro"},
	}

	newMeta, err := testTable.Update(ctx, TableMetadataToUpdate{
		ExternalDataConfig: newExtCfg,
	}, origMeta.ETag)
	if err != nil {
		t.Fatalf("Table.Update(%q): %v", testTable.FullyQualifiedName(), err)
	}
	if diff := testutil.Diff(newMeta.Schema, wantSchema); diff != "" {
		t.Fatalf("new schema, got=-, want=+\n%s", diff)
	}

	// Now, signal autodetect in another update.
	// This should yield a new schema.
	newMeta2, err := testTable.Update(ctx, TableMetadataToUpdate{}, newMeta.ETag, WithAutoDetectSchema(true))
	if err != nil {
		t.Fatalf("Table.Update(%q) with autodetect: %v", testTable.FullyQualifiedName(), err)
	}

	wantSchema2 := Schema{
		{Name: "stringfield", Type: "STRING"},
		{Name: "int64field", Type: "INTEGER"},
		{Name: "otherfield", Type: "INTEGER"},
	}
	if diff := testutil.Diff(newMeta2.Schema, wantSchema2); diff != "" {
		t.Errorf("new schema after autodetect, got=-, want=+\n%s", diff)
	}

	id, _ := testTable.Identifier(StandardSQLID)
	q := client.Query(fmt.Sprintf("SELECT * FROM %s", id))
	it, err := q.Read(ctx)
	if err != nil {
		t.Fatalf("query read: %v", err)
	}
	wantRows := [][]Value{
		{"bar", int64(32), int64(314)},
	}
	checkReadAndTotalRows(t, "row check", it, wantRows)
}

func TestIntegration_QueryExternalHivePartitioning(t *testing.T) {
	if client == nil {
		t.Skip("Integration tests skipped")
	}
	ctx := context.Background()

	autoTable := dataset.Table(tableIDs.New())
	customTable := dataset.Table(tableIDs.New())

	err := autoTable.Create(ctx, &TableMetadata{
		ExternalDataConfig: &ExternalDataConfig{
			SourceFormat:       Parquet,
			SourceURIs:         []string{"gs://cloud-samples-data/bigquery/hive-partitioning-samples/autolayout/*"},
			AutoDetect:         true,
			DecimalTargetTypes: []DecimalTargetType{StringTargetType},
			HivePartitioningOptions: &HivePartitioningOptions{
				Mode:                   AutoHivePartitioningMode,
				SourceURIPrefix:        "gs://cloud-samples-data/bigquery/hive-partitioning-samples/autolayout/",
				RequirePartitionFilter: true,
			},
		},
	})
	if err != nil {
		t.Fatalf("table.Create(auto): %v", err)
	}
	defer autoTable.Delete(ctx)

	err = customTable.Create(ctx, &TableMetadata{
		ExternalDataConfig: &ExternalDataConfig{
			SourceFormat:       Parquet,
			SourceURIs:         []string{"gs://cloud-samples-data/bigquery/hive-partitioning-samples/customlayout/*"},
			AutoDetect:         true,
			DecimalTargetTypes: []DecimalTargetType{NumericTargetType, StringTargetType},
			HivePartitioningOptions: &HivePartitioningOptions{
				Mode:                   CustomHivePartitioningMode,
				SourceURIPrefix:        "gs://cloud-samples-data/bigquery/hive-partitioning-samples/customlayout/{pkey:STRING}/",
				RequirePartitionFilter: true,
			},
		},
	})
	if err != nil {
		t.Fatalf("table.Create(custom): %v", err)
	}
	defer customTable.Delete(ctx)

	customTableSQLID, _ := customTable.Identifier(StandardSQLID)

	// Issue a test query that prunes based on the custom hive partitioning key, and verify the result is as expected.
	sql := fmt.Sprintf("SELECT COUNT(*) as ct FROM %s WHERE pkey=\"foo\"", customTableSQLID)
	q := client.Query(sql)
	it, err := q.Read(ctx)
	if err != nil {
		t.Fatalf("Error querying: %v", err)
	}
	checkReadAndTotalRows(t, "HiveQuery", it, [][]Value{{int64(50)}})
}

func TestIntegration_QuerySessionSupport(t *testing.T) {
	if client == nil {
		t.Skip("Integration tests skipped")
	}
	ctx := context.Background()

	q := client.Query("CREATE TEMPORARY TABLE temptable AS SELECT 17 as foo")
	q.CreateSession = true
	jobStats, _, err := runQueryJob(ctx, q)
	if err != nil {
		t.Fatalf("error running CREATE TEMPORARY TABLE: %v", err)
	}
	if jobStats.SessionInfo == nil {
		t.Fatalf("expected session info, was nil")
	}
	sessionID := jobStats.SessionInfo.SessionID
	if len(sessionID) == 0 {
		t.Errorf("expected non-empty sessionID")
	}

	q2 := client.Query("SELECT * FROM temptable")
	q2.ConnectionProperties = []*ConnectionProperty{
		{Key: "session_id", Value: sessionID},
	}
	jobStats, _, err = runQueryJob(ctx, q2)
	if err != nil {
		t.Errorf("error running SELECT: %v", err)
	}
	if jobStats.SessionInfo == nil {
		t.Fatalf("expected sessionInfo in second query, was nil")
	}
	got := jobStats.SessionInfo.SessionID
	if got != sessionID {
		t.Errorf("second query mismatched session ID, got %s want %s", got, sessionID)
	}

}

var (
	queryParameterTestCases = []struct {
		query      string
		parameters []QueryParameter
		wantRow    []Value
		wantConfig interface{}
	}{}
)

func initQueryParameterTestCases() {
	d := civil.Date{Year: 2016, Month: 3, Day: 20}
	tm := civil.Time{Hour: 15, Minute: 04, Second: 05, Nanosecond: 3008}
	rtm := tm
	rtm.Nanosecond = 3000 // round to microseconds
	dtm := civil.DateTime{Date: d, Time: tm}
	ts := time.Date(2016, 3, 20, 15, 04, 05, 0, time.UTC)
	rat := big.NewRat(13, 10)
	bigRat := big.NewRat(12345, 10e10)

	type ss struct {
		String string
	}

	type s struct {
		Timestamp      time.Time
		StringArray    []string
		SubStruct      ss
		SubStructArray []ss
	}

	queryParameterTestCases = []struct {
		query      string
		parameters []QueryParameter
		wantRow    []Value
		wantConfig interface{}
	}{
		{
			"SELECT @val",
			[]QueryParameter{{Name: "val", Value: 1}},
			[]Value{int64(1)},
			int64(1),
		},
		{
			"SELECT @val",
			[]QueryParameter{{Name: "val", Value: 1.3}},
			[]Value{1.3},
			1.3,
		},
		{
			"SELECT @val",
			[]QueryParameter{{Name: "val", Value: rat}},
			[]Value{rat},
			rat,
		},
		{
			"SELECT @val",
			[]QueryParameter{{Name: "val", Value: true}},
			[]Value{true},
			true,
		},
		{
			"SELECT @val",
			[]QueryParameter{{Name: "val", Value: "ABC"}},
			[]Value{"ABC"},
			"ABC",
		},
		{
			"SELECT @val",
			[]QueryParameter{{Name: "val", Value: []byte("foo")}},
			[]Value{[]byte("foo")},
			[]byte("foo"),
		},
		{
			"SELECT @val",
			[]QueryParameter{{Name: "val", Value: ts}},
			[]Value{ts},
			ts,
		},
		{
			"SELECT @val",
			[]QueryParameter{{Name: "val", Value: []time.Time{ts, ts}}},
			[]Value{[]Value{ts, ts}},
			[]interface{}{ts, ts},
		},
		{
			"SELECT @val",
			[]QueryParameter{{Name: "val", Value: dtm}},
			[]Value{civil.DateTime{Date: d, Time: rtm}},
			civil.DateTime{Date: d, Time: rtm},
		},
		{
			"SELECT @val",
			[]QueryParameter{{Name: "val", Value: d}},
			[]Value{d},
			d,
		},
		{
			"SELECT @val",
			[]QueryParameter{{Name: "val", Value: tm}},
			[]Value{rtm},
			rtm,
		},
		{
			"SELECT @val",
			[]QueryParameter{
				{
					Name: "val",
					Value: &QueryParameterValue{
						Type: StandardSQLDataType{
							TypeKind: "JSON",
						},
						Value: "{\"alpha\":\"beta\"}",
					},
				},
			},
			[]Value{"{\"alpha\":\"beta\"}"},
			"{\"alpha\":\"beta\"}",
		},
		{
			"SELECT @val",
			[]QueryParameter{{Name: "val", Value: s{ts, []string{"a", "b"}, ss{"c"}, []ss{{"d"}, {"e"}}}}},
			[]Value{[]Value{ts, []Value{"a", "b"}, []Value{"c"}, []Value{[]Value{"d"}, []Value{"e"}}}},
			map[string]interface{}{
				"Timestamp":   ts,
				"StringArray": []interface{}{"a", "b"},
				"SubStruct":   map[string]interface{}{"String": "c"},
				"SubStructArray": []interface{}{
					map[string]interface{}{"String": "d"},
					map[string]interface{}{"String": "e"},
				},
			},
		},
		{
			"SELECT @val.Timestamp, @val.SubStruct.String",
			[]QueryParameter{{Name: "val", Value: s{Timestamp: ts, SubStruct: ss{"a"}}}},
			[]Value{ts, "a"},
			map[string]interface{}{
				"Timestamp":      ts,
				"SubStruct":      map[string]interface{}{"String": "a"},
				"StringArray":    nil,
				"SubStructArray": nil,
			},
		},
		{
			"SELECT @val",
			[]QueryParameter{
				{
					Name: "val",
					Value: &QueryParameterValue{
						Type: StandardSQLDataType{
							TypeKind: "BIGNUMERIC",
						},
						Value: BigNumericString(bigRat),
					},
				},
			},
			[]Value{bigRat},
			bigRat,
		},
		{
			"SELECT @val",
			[]QueryParameter{
				{
					Name: "val",
					Value: &QueryParameterValue{
						ArrayValue: []QueryParameterValue{
							{Value: "a"},
							{Value: "b"},
						},
						Type: StandardSQLDataType{
							ArrayElementType: &StandardSQLDataType{
								TypeKind: "STRING",
							},
						},
					},
				},
			},
			[]Value{[]Value{"a", "b"}},
			[]interface{}{"a", "b"},
		},
		{
			"SELECT @val",
			[]QueryParameter{
				{
					Name: "val",
					Value: &QueryParameterValue{
						StructValue: map[string]QueryParameterValue{
							"Timestamp": {
								Value: ts,
							},
							"BigNumericArray": {
								ArrayValue: []QueryParameterValue{
									{Value: BigNumericString(bigRat)},
									{Value: BigNumericString(rat)},
								},
							},
							"ArraySingleValueStruct": {
								ArrayValue: []QueryParameterValue{
									{StructValue: map[string]QueryParameterValue{
										"Number": {
											Value: int64(42),
										},
									}},
									{StructValue: map[string]QueryParameterValue{
										"Number": {
											Value: int64(43),
										},
									}},
								},
							},
							"SubStruct": {
								StructValue: map[string]QueryParameterValue{
									"String": {
										Value: "c",
									},
								},
							},
						},
						Type: StandardSQLDataType{
							StructType: &StandardSQLStructType{
								Fields: []*StandardSQLField{
									{
										Name: "Timestamp",
										Type: &StandardSQLDataType{
											TypeKind: "TIMESTAMP",
										},
									},
									{
										Name: "BigNumericArray",
										Type: &StandardSQLDataType{
											ArrayElementType: &StandardSQLDataType{
												TypeKind: "BIGNUMERIC",
											},
										},
									},
									{
										Name: "ArraySingleValueStruct",
										Type: &StandardSQLDataType{
											ArrayElementType: &StandardSQLDataType{
												StructType: &StandardSQLStructType{
													Fields: []*StandardSQLField{
														{
															Name: "Number",
															Type: &StandardSQLDataType{
																TypeKind: "INT64",
															},
														},
													},
												},
											},
										},
									},
									{
										Name: "SubStruct",
										Type: &StandardSQLDataType{
											StructType: &StandardSQLStructType{
												Fields: []*StandardSQLField{
													{
														Name: "String",
														Type: &StandardSQLDataType{
															TypeKind: "STRING",
														},
													},
												},
											},
										},
									},
								},
							},
						},
					},
				},
			},
			[]Value{[]Value{ts, []Value{bigRat, rat}, []Value{[]Value{int64(42)}, []Value{int64(43)}}, []Value{"c"}}},
			map[string]interface{}{
				"Timestamp":       ts,
				"BigNumericArray": []interface{}{bigRat, rat},
				"ArraySingleValueStruct": []interface{}{
					map[string]interface{}{"Number": int64(42)},
					map[string]interface{}{"Number": int64(43)},
				},
				"SubStruct": map[string]interface{}{"String": "c"},
			},
		},
	}
}

func TestIntegration_QueryParameters(t *testing.T) {
	if client == nil {
		t.Skip("Integration tests skipped")
	}
	ctx := context.Background()

	initQueryParameterTestCases()

	for _, c := range queryParameterTestCases {
		q := client.Query(c.query)
		q.Parameters = c.parameters
		job, err := q.Run(ctx)
		if err != nil {
			t.Fatal(err)
		}
		if job.LastStatus() == nil {
			t.Error("no LastStatus")
		}
		it, err := job.Read(ctx)
		if err != nil {
			t.Fatal(err)
		}
		checkRead(t, "QueryParameters", it, [][]Value{c.wantRow})
		config, err := job.Config()
		if err != nil {
			t.Fatal(err)
		}
		got := config.(*QueryConfig).Parameters[0].Value
		if !testutil.Equal(got, c.wantConfig) {
			t.Errorf("param %[1]v (%[1]T): config:\ngot %[2]v (%[2]T)\nwant %[3]v (%[3]T)",
				c.parameters[0].Value, got, c.wantConfig)
		}
	}
}

// This test can be merged with the TestIntegration_QueryParameters as soon as support for explicit typed query parameter lands.
// To test timestamps with different formats, we need to be able to specify the type explicitly.
func TestIntegration_TimestampFormat(t *testing.T) {
	if client == nil {
		t.Skip("Integration tests skipped")
	}
	ctx := context.Background()
	ts := time.Date(2020, 10, 15, 15, 04, 05, 0, time.UTC)

	testCases := []struct {
		query      string
		parameters []*bq.QueryParameter
		wantRow    []Value
		wantConfig interface{}
	}{
		{
			"SELECT @val",
			[]*bq.QueryParameter{
				{
					Name: "val",
					ParameterType: &bq.QueryParameterType{
						Type: "TIMESTAMP",
					},
					ParameterValue: &bq.QueryParameterValue{
						Value: ts.Format(timestampFormat),
					},
				},
			},
			[]Value{ts},
			ts,
		},
		{
			"SELECT @val",
			[]*bq.QueryParameter{
				{
					Name: "val",
					ParameterType: &bq.QueryParameterType{
						Type: "TIMESTAMP",
					},
					ParameterValue: &bq.QueryParameterValue{
						Value: ts.Format(time.RFC3339Nano),
					},
				},
			},
			[]Value{ts},
			ts,
		},
		{
			"SELECT @val",
			[]*bq.QueryParameter{
				{
					Name: "val",
					ParameterType: &bq.QueryParameterType{
						Type: "TIMESTAMP",
					},
					ParameterValue: &bq.QueryParameterValue{
						Value: ts.Format(dateTimeFormat),
					},
				},
			},
			[]Value{ts},
			ts,
		},
		{
			"SELECT @val",
			[]*bq.QueryParameter{
				{
					Name: "val",
					ParameterType: &bq.QueryParameterType{
						Type: "TIMESTAMP",
					},
					ParameterValue: &bq.QueryParameterValue{
						Value: ts.Format(time.RFC3339),
					},
				},
			},
			[]Value{ts},
			ts,
		},
	}
	for _, c := range testCases {
		q := client.Query(c.query)
		bqJob, err := q.newJob()
		if err != nil {
			t.Fatal(err)
		}
		bqJob.Configuration.Query.QueryParameters = c.parameters

		job, err := q.client.insertJob(ctx, bqJob, nil)
		if err != nil {
			t.Fatal(err)
		}
		if job.LastStatus() == nil {
			t.Error("no LastStatus")
		}
		it, err := job.Read(ctx)
		if err != nil {
			t.Fatal(err)
		}
		checkRead(t, "QueryParameters", it, [][]Value{c.wantRow})
		config, err := job.Config()
		if err != nil {
			t.Fatal(err)
		}
		got := config.(*QueryConfig).Parameters[0].Value
		if !testutil.Equal(got, c.wantConfig) {
			t.Errorf("param %[1]v (%[1]T): config:\ngot %[2]v (%[2]T)\nwant %[3]v (%[3]T)",
				c.parameters[0].ParameterValue.Value, got, c.wantConfig)
		}
	}
}

func TestIntegration_QueryDryRun(t *testing.T) {
	if client == nil {
		t.Skip("Integration tests skipped")
	}
	ctx := context.Background()
	q := client.Query("SELECT word from " + stdName + " LIMIT 10")
	q.DryRun = true
	job, err := q.Run(ctx)
	if err != nil {
		t.Fatal(err)
	}

	s := job.LastStatus()
	if s.State != Done {
		t.Errorf("state is %v, expected Done", s.State)
	}
	if s.Statistics == nil {
		t.Fatal("no statistics")
	}
	if s.Statistics.Details.(*QueryStatistics).Schema == nil {
		t.Fatal("no schema")
	}
	if s.Statistics.Details.(*QueryStatistics).TotalBytesProcessedAccuracy == "" {
		t.Fatal("no cost accuracy")
	}
}

func TestIntegration_Scripting(t *testing.T) {
	if client == nil {
		t.Skip("Integration tests skipped")
	}
	ctx := context.Background()
	sql := `
	-- Declare a variable to hold names as an array.
	DECLARE top_names ARRAY<STRING>;
	BEGIN TRANSACTION;
	-- Build an array of the top 100 names from the year 2017.
	SET top_names = (
	  SELECT ARRAY_AGG(name ORDER BY number DESC LIMIT 100)
	  FROM ` + "`bigquery-public-data`" + `.usa_names.usa_1910_current
	  WHERE year = 2017
	);
	-- Which names appear as words in Shakespeare's plays?
	SELECT
	  name AS shakespeare_name
	FROM UNNEST(top_names) AS name
	WHERE name IN (
	  SELECT word
	  FROM ` + "`bigquery-public-data`" + `.samples.shakespeare
	);
	COMMIT TRANSACTION;
	`
	q := client.Query(sql)
	job, err := q.Run(ctx)
	if err != nil {
		t.Fatalf("failed to run parent job: %v", err)
	}
	status, err := job.Wait(ctx)
	if err != nil {
		t.Fatalf("job %q failed to wait for completion: %v", job.ID(), err)
	}
	if status.Err() != nil {
		t.Fatalf("job %q terminated with error: %v", job.ID(), err)
	}

	queryStats, ok := status.Statistics.Details.(*QueryStatistics)
	if !ok {
		t.Fatalf("failed to fetch query statistics")
	}

	want := "SCRIPT"
	if queryStats.StatementType != want {
		t.Errorf("statement type mismatch. got %s want %s", queryStats.StatementType, want)
	}

	if status.Statistics.NumChildJobs <= 0 {
		t.Errorf("expected script to indicate nonzero child jobs, got %d", status.Statistics.NumChildJobs)
	}

	// Ensure child jobs are present.
	var childJobs []*Job

	it := job.Children(ctx)
	for {
		job, err := it.Next()
		if err == iterator.Done {
			break
		}
		if err != nil {
			t.Fatal(err)
		}
		childJobs = append(childJobs, job)
	}
	if len(childJobs) == 0 {
		t.Fatal("Script had no child jobs.")
	}

	for _, cj := range childJobs {
		cStatus := cj.LastStatus()
		if cStatus.Statistics.ParentJobID != job.ID() {
			t.Errorf("child job %q doesn't indicate parent.  got %q, want %q", cj.ID(), cStatus.Statistics.ParentJobID, job.ID())
		}
		if cStatus.Statistics.ScriptStatistics == nil {
			t.Errorf("child job %q doesn't have script statistics present", cj.ID())
		}
		if cStatus.Statistics.ScriptStatistics.EvaluationKind == "" {
			t.Errorf("child job %q didn't indicate evaluation kind", cj.ID())
		}
		if cStatus.Statistics.TransactionInfo == nil {
			t.Errorf("child job %q didn't have transaction info present", cj.ID())
		}
		if cStatus.Statistics.TransactionInfo.TransactionID == "" {
			t.Errorf("child job %q didn't have transactionID present", cj.ID())
		}
	}

}

func TestIntegration_ExtractExternal(t *testing.T) {
	// Create a table, extract it to GCS, then query it externally.
	if client == nil {
		t.Skip("Integration tests skipped")
	}
	ctx := context.Background()
	schema := Schema{
		{Name: "name", Type: StringFieldType},
		{Name: "num", Type: IntegerFieldType},
	}
	table := newTable(t, schema)
	defer table.Delete(ctx)

	// Insert table data.
	sql := fmt.Sprintf(`INSERT %s.%s (name, num)
		                VALUES ('a', 1), ('b', 2), ('c', 3)`,
		table.DatasetID, table.TableID)
	if _, _, err := runQuerySQL(ctx, sql); err != nil {
		t.Fatal(err)
	}
	// Extract to a GCS object as CSV.
	bucketName := testutil.ProjID()
	objectName := fmt.Sprintf("bq-test-%s.csv", table.TableID)
	uri := fmt.Sprintf("gs://%s/%s", bucketName, objectName)
	defer storageClient.Bucket(bucketName).Object(objectName).Delete(ctx)
	gr := NewGCSReference(uri)
	gr.DestinationFormat = CSV
	e := table.ExtractorTo(gr)
	job, err := e.Run(ctx)
	if err != nil {
		t.Fatal(err)
	}
	conf, err := job.Config()
	if err != nil {
		t.Fatal(err)
	}
	config, ok := conf.(*ExtractConfig)
	if !ok {
		t.Fatalf("got %T, want ExtractConfig", conf)
	}
	diff := testutil.Diff(config, &e.ExtractConfig,
		cmp.AllowUnexported(Table{}),
		cmpopts.IgnoreUnexported(Client{}))
	if diff != "" {
		t.Errorf("got=-, want=+:\n%s", diff)
	}
	if err := wait(ctx, job); err != nil {
		t.Fatal(err)
	}

	edc := &ExternalDataConfig{
		SourceFormat: CSV,
		SourceURIs:   []string{uri},
		Schema:       schema,
		Options: &CSVOptions{
			SkipLeadingRows: 1,
			// This is the default. Since we use edc as an expectation later on,
			// let's just be explicit.
			FieldDelimiter: ",",
		},
	}
	// Query that CSV file directly.
	q := client.Query("SELECT * FROM csv")
	q.TableDefinitions = map[string]ExternalData{"csv": edc}
	wantRows := [][]Value{
		{"a", int64(1)},
		{"b", int64(2)},
		{"c", int64(3)},
	}
	iter, err := q.Read(ctx)
	if err != nil {
		t.Fatal(err)
	}
	checkReadAndTotalRows(t, "external query", iter, wantRows)

	// Make a table pointing to the file, and query it.
	// BigQuery does not allow a Table.Read on an external table.
	table = dataset.Table(tableIDs.New())
	err = table.Create(context.Background(), &TableMetadata{
		Schema:             schema,
		ExpirationTime:     testTableExpiration,
		ExternalDataConfig: edc,
	})
	if err != nil {
		t.Fatal(err)
	}
	q = client.Query(fmt.Sprintf("SELECT * FROM %s.%s", table.DatasetID, table.TableID))
	iter, err = q.Read(ctx)
	if err != nil {
		t.Fatal(err)
	}
	checkReadAndTotalRows(t, "external table", iter, wantRows)

	// While we're here, check that the table metadata is correct.
	md, err := table.Metadata(ctx)
	if err != nil {
		t.Fatal(err)
	}
	// One difference: since BigQuery returns the schema as part of the ordinary
	// table metadata, it does not populate ExternalDataConfig.Schema.
	md.ExternalDataConfig.Schema = md.Schema
	if diff := testutil.Diff(md.ExternalDataConfig, edc); diff != "" {
		t.Errorf("got=-, want=+\n%s", diff)
	}
}

func TestIntegration_ExportDataStatistics(t *testing.T) {
	// Create a table, extract it to GCS using EXPORT DATA statement.
	if client == nil {
		t.Skip("Integration tests skipped")
	}
	ctx := context.Background()
	schema := Schema{
		{Name: "name", Type: StringFieldType},
		{Name: "num", Type: IntegerFieldType},
	}
	table := newTable(t, schema)
	defer table.Delete(ctx)

	// Extract to a GCS object as CSV.
	bucketName := testutil.ProjID()
	uri := fmt.Sprintf("gs://%s/bq-export-test-*.csv", bucketName)
	defer func() {
		it := storageClient.Bucket(bucketName).Objects(ctx, &storage.Query{
			MatchGlob: "bq-export-test-*.csv",
		})
		for {
			obj, err := it.Next()
			if err == iterator.Done {
				break
			}
			if err != nil {
				t.Logf("failed to delete bucket: %v", err)
				continue
			}
			err = storageClient.Bucket(bucketName).Object(obj.Name).Delete(ctx)
			t.Logf("deleted object %s: %v", obj.Name, err)
		}
	}()

	// EXPORT DATA to GCS object.
	sql := fmt.Sprintf(`EXPORT DATA 
		OPTIONS (
			uri = '%s',
			format = 'CSV',
			overwrite = true,
			header = true,
			field_delimiter = ';'
		)
		AS (
			SELECT 'a' as name, 1 as num
			UNION ALL
			SELECT 'b' as name, 2 as num
			UNION ALL  
			SELECT 'c' as name, 3 as num
		);`,
		uri)
	stats, _, err := runQuerySQL(ctx, sql)
	if err != nil {
		t.Fatal(err)
	}

	qStats, ok := stats.Details.(*QueryStatistics)
	if !ok {
		t.Fatalf("expected query statistics not present")
	}

	if qStats.ExportDataStatistics == nil {
		t.Fatal("jobStatus missing ExportDataStatistics")
	}
	if qStats.ExportDataStatistics.FileCount != 1 {
		t.Fatalf("expected ExportDataStatistics to have 1 file, but got %d files", qStats.ExportDataStatistics.FileCount)
	}
	if qStats.ExportDataStatistics.RowCount != 3 {
		t.Fatalf("expected ExportDataStatistics to have 3 rows, got %d rows", qStats.ExportDataStatistics.RowCount)
	}
}

func TestIntegration_ReadNullIntoStruct(t *testing.T) {
	// Reading a null into a struct field should return an error (not panic).
	if client == nil {
		t.Skip("Integration tests skipped")
	}
	ctx := context.Background()
	table := newTable(t, schema)
	defer table.Delete(ctx)

	ins := table.Inserter()
	row := &ValuesSaver{
		Schema: schema,
		Row:    []Value{nil, []Value{}, []Value{nil}},
	}
	if err := ins.Put(ctx, []*ValuesSaver{row}); err != nil {
		t.Fatal(putError(err))
	}
	if err := waitForRow(ctx, table); err != nil {
		t.Fatal(err)
	}

	q := client.Query(fmt.Sprintf("select name from %s", table.TableID))
	q.DefaultProjectID = dataset.ProjectID
	q.DefaultDatasetID = dataset.DatasetID
	it, err := q.Read(ctx)
	if err != nil {
		t.Fatal(err)
	}
	type S struct{ Name string }
	var s S
	if err := it.Next(&s); err == nil {
		t.Fatal("got nil, want error")
	}
}

const (
	stdName    = "`bigquery-public-data.samples.shakespeare`"
	legacyName = "[bigquery-public-data:samples.shakespeare]"
)

// These tests exploit the fact that the two SQL versions have different syntaxes for
// fully-qualified table names.
var useLegacySQLTests = []struct {
	t           string // name of table
	std, legacy bool   // use standard/legacy SQL
	err         bool   // do we expect an error?
}{
	{t: legacyName, std: false, legacy: true, err: false},
	{t: legacyName, std: true, legacy: false, err: true},
	{t: legacyName, std: false, legacy: false, err: true}, // standard SQL is default
	{t: legacyName, std: true, legacy: true, err: true},
	{t: stdName, std: false, legacy: true, err: true},
	{t: stdName, std: true, legacy: false, err: false},
	{t: stdName, std: false, legacy: false, err: false}, // standard SQL is default
	{t: stdName, std: true, legacy: true, err: true},
}

func TestIntegration_QueryUseLegacySQL(t *testing.T) {
	// Test the UseLegacySQL and UseStandardSQL options for queries.
	if client == nil {
		t.Skip("Integration tests skipped")
	}
	ctx := context.Background()
	for _, test := range useLegacySQLTests {
		q := client.Query(fmt.Sprintf("select word from %s limit 1", test.t))
		q.UseStandardSQL = test.std
		q.UseLegacySQL = test.legacy
		_, err := q.Read(ctx)
		gotErr := err != nil
		if gotErr && !test.err {
			t.Errorf("%+v:\nunexpected error: %v", test, err)
		} else if !gotErr && test.err {
			t.Errorf("%+v:\nsucceeded, but want error", test)
		}
	}
}

func TestIntegration_ListJobs(t *testing.T) {
	// It's difficult to test the list of jobs, because we can't easily
	// control what's in it. Also, there are many jobs in the test project,
	// and it takes considerable time to list them all.
	if client == nil {
		t.Skip("Integration tests skipped")
	}
	ctx := context.Background()

	// About all we can do is list a few jobs.
	const max = 20
	var jobs []*Job
	it := client.Jobs(ctx)
	for {
		job, err := it.Next()
		if err == iterator.Done {
			break
		}
		if err != nil {
			t.Fatal(err)
		}
		jobs = append(jobs, job)
		if len(jobs) >= max {
			break
		}
	}
	// We expect that there is at least one job in the last few months.
	if len(jobs) == 0 {
		t.Fatal("did not get any jobs")
	}
}

func TestIntegration_DeleteJob(t *testing.T) {
	if client == nil {
		t.Skip("Integration tests skipped")
	}
	ctx := context.Background()

	q := client.Query("SELECT 17 as foo")
	q.Location = "us-east1"

	job, err := q.Run(ctx)
	if err != nil {
		t.Fatalf("job Run failure: %v", err)
	}
	err = wait(ctx, job)
	if err != nil {
		t.Fatalf("job %q completion failure: %v", job.ID(), err)
	}

	if err := job.Delete(ctx); err != nil {
		t.Fatalf("job.Delete failed: %v", err)
	}
}

const tokyo = "asia-northeast1"

func TestIntegration_Location(t *testing.T) {
	if client == nil {
		t.Skip("Integration tests skipped")
	}
	client.Location = ""
	testLocation(t, tokyo)
	client.Location = tokyo
	defer func() {
		client.Location = ""
	}()
	testLocation(t, "")
}

func testLocation(t *testing.T, loc string) {
	ctx := context.Background()
	tokyoDataset := client.Dataset("tokyo")
	err := tokyoDataset.Create(ctx, &DatasetMetadata{Location: loc})
	if err != nil && !hasStatusCode(err, 409) { // 409 = already exists
		t.Fatal(err)
	}
	md, err := tokyoDataset.Metadata(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if md.Location != tokyo {
		t.Fatalf("dataset location: got %s, want %s", md.Location, tokyo)
	}
	table := tokyoDataset.Table(tableIDs.New())
	err = table.Create(context.Background(), &TableMetadata{
		Schema: Schema{
			{Name: "name", Type: StringFieldType},
			{Name: "nums", Type: IntegerFieldType},
		},
		ExpirationTime: testTableExpiration,
	})
	if err != nil {
		t.Fatal(err)
	}

	tableMetadata, err := table.Metadata(ctx)
	if err != nil {
		t.Fatalf("failed to get table metadata: %v", err)
	}
	wantLoc := loc
	if loc == "" && client.Location != "" {
		wantLoc = client.Location
	}
	if tableMetadata.Location != wantLoc {
		t.Errorf("Location on table doesn't match.  Got %s want %s", tableMetadata.Location, wantLoc)
	}
	defer table.Delete(ctx)
	loader := table.LoaderFrom(NewReaderSource(strings.NewReader("a,0\nb,1\nc,2\n")))
	loader.Location = loc
	job, err := loader.Run(ctx)
	if err != nil {
		t.Fatal("loader.Run", err)
	}
	if job.Location() != tokyo {
		t.Fatalf("job location: got %s, want %s", job.Location(), tokyo)
	}
	_, err = client.JobFromID(ctx, job.ID())
	if client.Location == "" && err == nil {
		t.Error("JobFromID with Tokyo job, no client location: want error, got nil")
	}
	if client.Location != "" && err != nil {
		t.Errorf("JobFromID with Tokyo job, with client location: want nil, got %v", err)
	}
	_, err = client.JobFromIDLocation(ctx, job.ID(), "US")
	if err == nil {
		t.Error("JobFromIDLocation with US: want error, got nil")
	}
	job2, err := client.JobFromIDLocation(ctx, job.ID(), loc)
	if loc == tokyo && err != nil {
		t.Errorf("loc=tokyo: %v", err)
	}
	if loc == "" && err == nil {
		t.Error("loc empty: got nil, want error")
	}
	if job2 != nil && (job2.ID() != job.ID() || job2.Location() != tokyo) {
		t.Errorf("got id %s loc %s, want id%s loc %s", job2.ID(), job2.Location(), job.ID(), tokyo)
	}
	if err := wait(ctx, job); err != nil {
		t.Fatal(err)
	}
	// Cancel should succeed even if the job is done.
	if err := job.Cancel(ctx); err != nil {
		t.Fatal(err)
	}

	q := client.Query(fmt.Sprintf("SELECT * FROM %s.%s", table.DatasetID, table.TableID))
	q.Location = loc
	iter, err := q.Read(ctx)
	if err != nil {
		t.Fatal(err)
	}
	wantRows := [][]Value{
		{"a", int64(0)},
		{"b", int64(1)},
		{"c", int64(2)},
	}
	checkRead(t, "location", iter, wantRows)

	table2 := tokyoDataset.Table(tableIDs.New())
	copier := table2.CopierFrom(table)
	copier.Location = loc
	if _, err := copier.Run(ctx); err != nil {
		t.Fatal(err)
	}
	bucketName := testutil.ProjID()
	objectName := fmt.Sprintf("bq-test-%s.csv", table.TableID)
	uri := fmt.Sprintf("gs://%s/%s", bucketName, objectName)
	defer storageClient.Bucket(bucketName).Object(objectName).Delete(ctx)
	gr := NewGCSReference(uri)
	gr.DestinationFormat = CSV
	e := table.ExtractorTo(gr)
	e.Location = loc
	if _, err := e.Run(ctx); err != nil {
		t.Fatal(err)
	}
}

func TestIntegration_NumericErrors(t *testing.T) {
	// Verify that the service returns an error for a big.Rat that's too large.
	if client == nil {
		t.Skip("Integration tests skipped")
	}
	ctx := context.Background()
	schema := Schema{{Name: "n", Type: NumericFieldType}}
	table := newTable(t, schema)
	defer table.Delete(ctx)
	tooBigRat := &big.Rat{}
	if _, ok := tooBigRat.SetString("1e40"); !ok {
		t.Fatal("big.Rat.SetString failed")
	}
	ins := table.Inserter()
	err := ins.Put(ctx, []*ValuesSaver{{Schema: schema, Row: []Value{tooBigRat}}})
	if err == nil {
		t.Fatal("got nil, want error")
	}
}

func TestIntegration_QueryErrors(t *testing.T) {
	// Verify that a bad query returns an appropriate error.
	if client == nil {
		t.Skip("Integration tests skipped")
	}
	ctx := context.Background()
	q := client.Query("blah blah broken")
	_, err := q.Read(ctx)
	const want = "invalidQuery"
	if !strings.Contains(err.Error(), want) {
		t.Fatalf("got %q, want substring %q", err, want)
	}
}

func TestIntegration_MaterializedViewLifecycle(t *testing.T) {
	if client == nil {
		t.Skip("Integration tests skipped")
	}
	ctx := context.Background()

	// instantiate a base table via a CTAS
	baseTableID := tableIDs.New()
	qualified := fmt.Sprintf("`%s`.%s.%s", testutil.ProjID(), dataset.DatasetID, baseTableID)
	sql := fmt.Sprintf(`
	CREATE TABLE %s
	(
		sample_value INT64,
		groupid STRING,
	)
	AS
	SELECT
	  CAST(RAND() * 100 AS INT64),
	  CONCAT("group", CAST(CAST(RAND()*10 AS INT64) AS STRING))
	FROM
	  UNNEST(GENERATE_ARRAY(0,999))
	`, qualified)
	if _, _, err := runQuerySQL(ctx, sql); err != nil {
		t.Fatalf("couldn't instantiate base table: %v", err)
	}

	// Define the SELECT aggregation to become a mat view
	sql = fmt.Sprintf(`
	SELECT
	  SUM(sample_value) as total,
	  groupid
	FROM
	  %s
	GROUP BY groupid
	`, qualified)

	// Create materialized view

	wantRefresh := 6 * time.Hour
	matViewID := tableIDs.New()
	view := dataset.Table(matViewID)
	if err := view.Create(ctx, &TableMetadata{
		MaterializedView: &MaterializedViewDefinition{
			Query:           sql,
			RefreshInterval: wantRefresh,
		}}); err != nil {
		t.Fatal(err)
	}

	// Get metadata
	curMeta, err := view.Metadata(ctx)
	if err != nil {
		t.Fatal(err)
	}

	if curMeta.MaterializedView == nil {
		t.Fatal("expected materialized view definition, was null")
	}

	if curMeta.MaterializedView.Query != sql {
		t.Errorf("mismatch on view sql.  Got %s want %s", curMeta.MaterializedView.Query, sql)
	}

	if curMeta.MaterializedView.RefreshInterval != wantRefresh {
		t.Errorf("mismatch on refresh time: got %d usec want %d usec", 1000*curMeta.MaterializedView.RefreshInterval.Nanoseconds(), 1000*wantRefresh.Nanoseconds())
	}

	// MaterializedView is a TableType constant
	want := MaterializedView
	if curMeta.Type != want {
		t.Errorf("mismatch on table type.  got %s want %s", curMeta.Type, want)
	}

	// Update metadata
	wantRefresh = time.Hour // 6hr -> 1hr
	upd := TableMetadataToUpdate{
		MaterializedView: &MaterializedViewDefinition{
			Query:           sql,
			RefreshInterval: wantRefresh,
		},
	}

	newMeta, err := view.Update(ctx, upd, curMeta.ETag)
	if err != nil {
		t.Fatalf("failed to update view definition: %v", err)
	}

	if newMeta.MaterializedView == nil {
		t.Error("MaterializeView missing in updated metadata")
	}

	if newMeta.MaterializedView.RefreshInterval != wantRefresh {
		t.Errorf("mismatch on updated refresh time: got %d usec want %d usec", 1000*curMeta.MaterializedView.RefreshInterval.Nanoseconds(), 1000*wantRefresh.Nanoseconds())
	}

	// verify implicit setting of false due to partial population of update.
	if newMeta.MaterializedView.EnableRefresh {
		t.Error("expected EnableRefresh to be false, is true")
	}

	// Verify list

	it := dataset.Tables(ctx)
	seen := false
	for {
		tbl, err := it.Next()
		if err == iterator.Done {
			break
		}
		if err != nil {
			t.Fatal(err)
		}
		if tbl.TableID == matViewID {
			seen = true
		}
	}
	if !seen {
		t.Error("materialized view not listed in dataset")
	}

	// Verify deletion
	if err := view.Delete(ctx); err != nil {
		t.Errorf("failed to delete materialized view: %v", err)
	}

}

func TestIntegration_ModelLifecycle(t *testing.T) {
	if client == nil {
		t.Skip("Integration tests skipped")
	}
	ctx := context.Background()

	// Create a model via a CREATE MODEL query
	modelID := modelIDs.New()
	model := dataset.Model(modelID)
	modelSQLID, _ := model.Identifier(StandardSQLID)

	sql := fmt.Sprintf(`
		CREATE MODEL %s
		OPTIONS (
			model_type='linear_reg',
			max_iteration=1,
			learn_rate=0.4,
			learn_rate_strategy='constant'
		) AS (
			SELECT 'a' AS f1, 2.0 AS label
			UNION ALL
			SELECT 'b' AS f1, 3.8 AS label
		)`, modelSQLID)
	if _, _, err := runQuerySQL(ctx, sql); err != nil {
		t.Fatal(err)
	}
	defer model.Delete(ctx)

	// Get the model metadata.
	curMeta, err := model.Metadata(ctx)
	if err != nil {
		t.Fatalf("couldn't get metadata: %v", err)
	}

	want := "LINEAR_REGRESSION"
	if curMeta.Type != want {
		t.Errorf("Model type mismatch.  Want %s got %s", curMeta.Type, want)
	}

	// Ensure training metadata is available.
	runs := curMeta.RawTrainingRuns()
	if runs == nil {
		t.Errorf("training runs unpopulated.")
	}
	labelCols, err := curMeta.RawLabelColumns()
	if err != nil {
		t.Fatalf("failed to get label cols: %v", err)
	}
	if labelCols == nil {
		t.Errorf("label column information unpopulated.")
	}
	featureCols, err := curMeta.RawFeatureColumns()
	if err != nil {
		t.Fatalf("failed to get feature cols: %v", err)
	}
	if featureCols == nil {
		t.Errorf("feature column information unpopulated.")
	}

	// Update mutable fields via API.
	expiry := time.Now().Add(24 * time.Hour).Truncate(time.Millisecond)

	upd := ModelMetadataToUpdate{
		Description:    "new",
		Name:           "friendly",
		ExpirationTime: expiry,
	}

	newMeta, err := model.Update(ctx, upd, curMeta.ETag)
	if err != nil {
		t.Fatalf("failed to update: %v", err)
	}

	want = "new"
	if newMeta.Description != want {
		t.Fatalf("Description not updated. got %s want %s", newMeta.Description, want)
	}
	want = "friendly"
	if newMeta.Name != want {
		t.Fatalf("Description not updated. got %s want %s", newMeta.Description, want)
	}
	if newMeta.ExpirationTime != expiry {
		t.Fatalf("ExpirationTime not updated.  got %v want %v", newMeta.ExpirationTime, expiry)
	}

	// Ensure presence when enumerating the model list.
	it := dataset.Models(ctx)
	seen := false
	for {
		mdl, err := it.Next()
		if err == iterator.Done {
			break
		}
		if err != nil {
			t.Fatal(err)
		}
		if mdl.ModelID == modelID {
			seen = true
		}
	}
	if !seen {
		t.Fatal("model not listed in dataset")
	}

	// Extract the model to GCS.
	bucketName := testutil.ProjID()
	objectName := fmt.Sprintf("bq-model-extract-%s", modelID)
	uri := fmt.Sprintf("gs://%s/%s", bucketName, objectName)
	defer storageClient.Bucket(bucketName).Object(objectName).Delete(ctx)
	gr := NewGCSReference(uri)
	gr.DestinationFormat = TFSavedModel
	extractor := model.ExtractorTo(gr)
	job, err := extractor.Run(ctx)
	if err != nil {
		t.Fatalf("failed to extract model to GCS: %v", err)
	}
	if err = wait(ctx, job); err != nil {
		t.Errorf("extract failed: %v", err)
	}

	// Delete the model.
	if err := model.Delete(ctx); err != nil {
		t.Fatalf("failed to delete model: %v", err)
	}
}

// Creates a new, temporary table with a unique name and the given schema.
func newTable(t *testing.T, s Schema) *Table {
	table := dataset.Table(tableIDs.New())
	err := table.Create(context.Background(), &TableMetadata{
		Schema:         s,
		ExpirationTime: testTableExpiration,
	})
	if err != nil {
		t.Fatal(err)
	}
	return table
}

func checkRead(t *testing.T, msg string, it *RowIterator, want [][]Value) {
	if msg2, ok := compareRead(it, want, false); !ok {
		t.Errorf("%s: %s", msg, msg2)
	}
}

func checkReadAndTotalRows(t *testing.T, msg string, it *RowIterator, want [][]Value) {
	if msg2, ok := compareRead(it, want, true); !ok {
		t.Errorf("%s: %s", msg, msg2)
	}
}

func compareRead(it *RowIterator, want [][]Value, compareTotalRows bool) (msg string, ok bool) {
	got, _, totalRows, err := readAll(it)
	jobStr := ""
	if it.SourceJob() != nil {
		jobStr = it.SourceJob().jobID
	}
	if jobStr != "" {
		jobStr = fmt.Sprintf("(Job: %s)", jobStr)
	}
	if err != nil {
		return err.Error(), false
	}
	if len(got) != len(want) {
		return fmt.Sprintf("%s got %d rows, want %d", jobStr, len(got), len(want)), false
	}
	if compareTotalRows && len(got) != int(totalRows) {
		return fmt.Sprintf("%s got %d rows, but totalRows = %d", jobStr, len(got), totalRows), false
	}
	sort.Sort(byCol0(got))
	for i, r := range got {
		gotRow := []Value(r)
		wantRow := want[i]
		if !testutil.Equal(gotRow, wantRow) {
			return fmt.Sprintf("%s #%d: got %#v, want %#v", jobStr, i, gotRow, wantRow), false
		}
	}
	return "", true
}

func readAll(it *RowIterator) ([][]Value, Schema, uint64, error) {
	var (
		rows      [][]Value
		schema    Schema
		totalRows uint64
	)
	for {
		var vals []Value
		err := it.Next(&vals)
		if err == iterator.Done {
			return rows, schema, totalRows, nil
		}
		if err != nil {
			return nil, nil, 0, err
		}
		rows = append(rows, vals)
		schema = it.Schema
		totalRows = it.TotalRows
	}
}

type byCol0 [][]Value

func (b byCol0) Len() int      { return len(b) }
func (b byCol0) Swap(i, j int) { b[i], b[j] = b[j], b[i] }
func (b byCol0) Less(i, j int) bool {
	switch a := b[i][0].(type) {
	case string:
		return a < b[j][0].(string)
	case civil.Date:
		return a.Before(b[j][0].(civil.Date))
	default:
		panic("unknown type")
	}
}

func hasStatusCode(err error, code int) bool {
	var e *googleapi.Error
	if ok := errors.As(err, &e); ok && e.Code == code {
		return true
	}
	return false
}

// wait polls the job until it is complete or an error is returned.
func wait(ctx context.Context, job *Job) error {
	status, err := job.Wait(ctx)
	if err != nil {
		return fmt.Errorf("job %q error: %v", job.ID(), err)
	}
	if status.Err() != nil {
		return fmt.Errorf("job %q status error: %#v", job.ID(), status.Err())
	}
	if status.Statistics == nil {
		return fmt.Errorf("job %q nil Statistics", job.ID())
	}
	if status.Statistics.EndTime.IsZero() {
		return fmt.Errorf("job %q EndTime is zero", job.ID())
	}
	return nil
}

// waitForRow polls the table until it contains a row.
// TODO(jba): use internal.Retry.
func waitForRow(ctx context.Context, table *Table) error {
	for {
		it := table.Read(ctx)
		var v []Value
		err := it.Next(&v)
		if err == nil {
			return nil
		}
		if err != iterator.Done {
			return err
		}
		time.Sleep(1 * time.Second)
	}
}

func putError(err error) string {
	pme, ok := err.(PutMultiError)
	if !ok {
		return err.Error()
	}
	var msgs []string
	for _, err := range pme {
		msgs = append(msgs, err.Error())
	}
	return strings.Join(msgs, "\n")
}

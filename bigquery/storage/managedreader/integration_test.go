// Copyright 2022 Google LLC
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

package managedreader

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"os"
	"testing"
	"time"

	"cloud.google.com/go/bigquery"
	bqStorage "cloud.google.com/go/bigquery/storage/apiv1"
	"cloud.google.com/go/httpreplay"
	"cloud.google.com/go/internal/testutil"
	"cloud.google.com/go/internal/uid"
	"google.golang.org/api/iterator"
	"google.golang.org/api/option"
)

const replayFilename = "bigquery.replay"

var record = flag.Bool("record", false, "record RPCs")

var (
	client                                     *bigquery.Client
	bqStorageClient                            *bqStorage.BigQueryReadClient
	dataset                                    *bigquery.Dataset
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

func getClient(t *testing.T) *bigquery.Client {
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
		client, err = bigquery.NewClient(ctx, projID, option.WithHTTPClient(hc))
		if err != nil {
			log.Fatal(err)
		}
		bqStorageClient, err = bqStorage.NewBigQueryReadClient(ctx, option.WithHTTPClient(hc))
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
		return func() {}

	default: // Run integration tests against a real backend.
		ts := testutil.TokenSource(ctx, bigquery.Scope)
		if ts == nil {
			log.Println("Integration tests skipped. See CONTRIBUTING.md for details")
			return func() {}
		}
		bqOpts := []option.ClientOption{option.WithTokenSource(ts)}
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
		}
		var err error
		client, err = bigquery.NewClient(ctx, projID, bqOpts...)
		if err != nil {
			log.Fatalf("NewClient: %v", err)
		}
		bqStorageClient, err = bqStorage.NewBigQueryReadClient(ctx, bqOpts...)
		if err != nil {
			log.Fatalf("NewBigQueryReadClient: %v", err)
		}
		c := initTestState(client, now)
		return func() { c(); cleanup() }
	}
}

func initTestState(client *bigquery.Client, t time.Time) func() {
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
	bigquery.Seed(t.UnixNano())

	dataset = client.Dataset(datasetIDs.New())

	if err := dataset.Create(ctx, nil); err != nil {
		log.Fatalf("creating dataset %s: %v", dataset.DatasetID, err)
	}

	return func() {
		if err := dataset.DeleteWithContents(ctx); err != nil {
			log.Printf("could not delete %s", dataset.DatasetID)
		}
	}
}

func TestIntegration_ReadQueryStorageAPI(t *testing.T) {
	if client == nil {
		t.Skip("Integration tests skipped")
	}
	ctx := context.Background()
	table := "`bigquery-public-data.usa_names.usa_1910_current`"
	sql := fmt.Sprintf(`SELECT name, number, state, STRUCT(name as name, number as n) as nested FROM %s where state = "CA"`, table)
	q := client.Query(sql)
	// q.StorageClient = bqStorageClient
	it, err := Upgrade(ctx, bqStorageClient, q)
	if err != nil {
		t.Fatal(err)
	}
	type S struct {
		Name   string
		Number int
		State  string
		Nested struct {
			Name string
			N    int
		}
	}
	// i := 0
	start := time.Now()
	for {
		var s S
		err := it.Next(&s)
		if err == iterator.Done {
			break
		}
		if err != nil {
			t.Fatalf("failed to fetch via storage API: %v", err)
		}
		// i++
		// fmt.Printf("got data: %v - %d of %d\n", s, i, it.TotalRows())
	}
	diff := time.Now().Sub(start).Milliseconds()
	t.Logf("took %d ms with storage API (%d rows)", diff, it.TotalRows())

	q = client.Query(sql)
	rowIt, err := q.Read(ctx)
	if err != nil {
		t.Fatal(err)
	}
	start = time.Now()
	for {
		var s S
		err := rowIt.Next(&s)
		if err == iterator.Done {
			break
		}
		if err != nil {
			t.Fatalf("failed to fetch via query API: %v", err)
		}
		// i++
		// fmt.Printf("got data: %v - %d of %d\n", s, i, it.TotalRows)
	}
	diff = time.Now().Sub(start).Milliseconds()
	t.Logf("took %d ms without storage API (%d rows)", diff, rowIt.TotalRows)
}

// Copyright 2021 Google LLC
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     https://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package storage

import (
	"context"
	"fmt"
	"io"
	"log"
	"math/rand"
	"runtime"
	"sync"
	"testing"
	"time"

	"cloud.google.com/go/bigquery"
	"cloud.google.com/go/internal/testutil"
	"cloud.google.com/go/internal/uid"
	"github.com/golang/protobuf/descriptor"
	"google.golang.org/api/option"
	"google.golang.org/protobuf/proto"

	"cloud.google.com/go/bigquery/storage/apiv1beta2/testdata"
	storagepb "google.golang.org/genproto/googleapis/cloud/bigquery/storage/v1beta2"
)

var (
	datasetIDs = uid.NewSpace("storage_test_dataset", &uid.Options{Sep: '_', Time: time.Now()})
	tableIDs   = uid.NewSpace("testtable", &uid.Options{Sep: '_', Time: time.Now()})
)

func withGRPCHeadersAssertion(t *testing.T, opts ...option.ClientOption) []option.ClientOption {
	grpcHeadersEnforcer := &testutil.HeadersEnforcer{
		OnFailure: t.Errorf,
		Checkers: []*testutil.HeaderChecker{
			testutil.XGoogClientHeaderChecker,
		},
	}
	return append(grpcHeadersEnforcer.CallOptions(), opts...)
}

// Testing necessitates access to multiple clients/services.
func getClients(ctx context.Context, t *testing.T, opts ...option.ClientOption) (*BigQueryWriteClient, *bigquery.Client) {
	if testing.Short() {
		t.Skip("Integration tests skipped in short mode")
	}
	projID := testutil.ProjID()
	if projID == "" {
		t.Skip("Integration test skipped, see CONTRIBUTING.md for details")
	}
	ts := testutil.TokenSource(ctx, bigquery.Scope)
	if ts == nil {
		t.Skip("Integration test skipped, see CONTRIBUTING.md for details")
	}

	// Construct a write client.
	opts = append(withGRPCHeadersAssertion(t, option.WithTokenSource(ts)), opts...)
	writeClient, err := NewBigQueryWriteClient(ctx)
	if err != nil {
		t.Fatalf("Creating BigQueryWriteClient error: %v", err)
	}

	// Construct a BQ client.
	bqClient, err := bigquery.NewClient(ctx, projID, option.WithTokenSource(ts))
	if err != nil {
		t.Fatalf("Creating bigquery.Client error: %v", err)
	}
	return writeClient, bqClient
}

func setupTestDataset(ctx context.Context, t *testing.T, bqClient *bigquery.Client) (ds *bigquery.Dataset, cleanup func(), err error) {
	dataset := bqClient.Dataset(datasetIDs.New())
	if err := dataset.Create(ctx, nil); err != nil {
		return nil, nil, err
	}
	return dataset, func() {
		if err := dataset.DeleteWithContents(ctx); err != nil {
			t.Logf("could not cleanup dataset %s: %v", dataset.DatasetID, err)
		}
	}, nil
}

// queryRowCount is used to issue a COUNT query against the specified table to validate the number of rows visible to
// the BQ query engine.
func queryRowCount(ctx context.Context, client *bigquery.Client, tbl *bigquery.Table) (int64, error) {

	// Verify data is present in the table with a count query.
	sql := fmt.Sprintf("SELECT COUNT(1) FROM `%s`.%s.%s", tbl.ProjectID, tbl.DatasetID, tbl.TableID)
	q := client.Query(sql)
	it, err := q.Read(ctx)
	if err != nil {
		return 0, fmt.Errorf("failed to issue validation query: %v", err)
	}
	var rowdata []bigquery.Value
	err = it.Next(&rowdata)
	if err != nil {
		return 0, fmt.Errorf("iterator error: %v", err)
	}

	if count, ok := rowdata[0].(int64); ok {
		return count, nil
	}
	return 0, fmt.Errorf("got unexpected value %v", rowdata[0])
}

func TestIntegration_BareMetalStreaming(t *testing.T) {
	ctx := context.Background()
	writeClient, bqClient := getClients(ctx, t)
	defer writeClient.Close()
	defer bqClient.Close()

	dataset, cleanupFunc, err := setupTestDataset(ctx, t, bqClient)
	if err != nil {
		t.Fatalf("failed to init test dataset: %v", err)
	}
	defer cleanupFunc()

	testTable := dataset.Table(tableIDs.New())

	schema := bigquery.Schema{
		{Name: "name", Type: bigquery.StringFieldType},
		{Name: "value", Type: bigquery.IntegerFieldType},
	}
	if err := testTable.Create(ctx, &bigquery.TableMetadata{Schema: schema}); err != nil {
		t.Fatalf("couldn't create test table %s: %v", testTable.FullyQualifiedName(), err)
	}
	writeStream := fmt.Sprintf("projects/%s/datasets/%s/tables/%s/_default", testTable.ProjectID, testTable.DatasetID, testTable.TableID)

	testData := [][]*testdata.SimpleMessage{
		[]*testdata.SimpleMessage{
			{Name: "one", Value: 1},
			{Name: "two", Value: 2},
			{Name: "three", Value: 3},
		},
		[]*testdata.SimpleMessage{
			{Name: "four", Value: 1},
			{Name: "five", Value: 2},
		},
	}

	timeoutCtx, _ := context.WithTimeout(ctx, 30*time.Second)
	stream, err := writeClient.AppendRows(timeoutCtx)
	if err != nil {
		t.Fatalf("failed to setup AppendRows stream: %v", err)
	}

	var wg sync.WaitGroup
	var reqCount, respCount, totalRows int64

	// Send Data.
	wg.Add(1)
	go func() {
		defer wg.Done()
		var serialized [][]byte
		for k, rowSet := range testData {
			for _, rowMsg := range rowSet {
				out, err := proto.Marshal(rowMsg)
				if err != nil {
					t.Fatalf("failed to serialize test data: %v", err)
				}
				serialized = append(serialized, out)
			}
			// Construct append request.
			var protoSchema *storagepb.ProtoSchema
			if k == 0 {
				// first message in stream, construct schema
				_, descriptor := descriptor.ForMessage(rowSet[0])
				protoSchema = &storagepb.ProtoSchema{
					ProtoDescriptor: descriptor,
				}
			}
			req := &storagepb.AppendRowsRequest{
				WriteStream: writeStream,
				TraceId:     "integration_test",
				Rows: &storagepb.AppendRowsRequest_ProtoRows{
					ProtoRows: &storagepb.AppendRowsRequest_ProtoData{
						Rows: &storagepb.ProtoRows{
							SerializedRows: serialized,
						},
						WriterSchema: protoSchema,
					},
				},
			}
			stream.Send(req)
			reqCount = reqCount + 1
			totalRows = totalRows + int64(len(rowSet))
			serialized = nil
		}
		stream.CloseSend()
	}()

	// Monitor updates.
	wg.Add(1)
	go func() {
		defer wg.Done()
		for {
			resp, err := stream.Recv()
			if err == io.EOF {
				// expected end of stream.
				break
			}
			if err != nil {
				t.Errorf("got err on recv: %v", err)
				break
			}

			if status := resp.GetError(); status != nil {
				t.Errorf("Response Error: %v", status)
			}
			if result := resp.GetAppendResult(); result != nil {
				// default stream doesn't provide an offset value, only an empty message.
				respCount = respCount + 1
			}
		}
	}()

	wg.Wait()

	if reqCount != respCount {
		t.Errorf("mismatched requests/responses: got %d requests, %d responses", reqCount, respCount)
	}

	gotRows, err := queryRowCount(ctx, bqClient, testTable)
	if err != nil {
		t.Errorf("failed to get row count: %v", err)
	}

	if gotRows != totalRows {
		t.Errorf("query result mismatch, got %v want %v", gotRows, totalRows)
	}

}

func TestIntegration_ThickWriter(t *testing.T) {
	setupCtx := context.Background()
	ctx, _ := context.WithTimeout(context.Background(), 15*time.Second)

	writeClient, bqClient := getClients(setupCtx, t)
	defer writeClient.Close()
	defer bqClient.Close()

	dataset, cleanupFunc, err := setupTestDataset(setupCtx, t, bqClient)
	if err != nil {
		t.Fatalf("failed to init test dataset: %v", err)
	}
	defer cleanupFunc()

	testTable := dataset.Table(tableIDs.New())

	schema := bigquery.Schema{
		{Name: "name", Type: bigquery.StringFieldType},
		{Name: "value", Type: bigquery.IntegerFieldType},
	}
	if err := testTable.Create(ctx, &bigquery.TableMetadata{Schema: schema}); err != nil {
		t.Fatalf("couldn't create test table %s: %v", testTable.FullyQualifiedName(), err)
	}
	writeStream := fmt.Sprintf("projects/%s/datasets/%s/tables/%s/_default", testTable.ProjectID, testTable.DatasetID, testTable.TableID)

	testData := [][]*testdata.SimpleMessage{
		[]*testdata.SimpleMessage{
			{Name: "one", Value: 1},
			{Name: "two", Value: 2},
			{Name: "three", Value: 3},
		},
		[]*testdata.SimpleMessage{
			{Name: "four", Value: 1},
			{Name: "five", Value: 2},
		},
	}

	tw, err := NewThickWriter(ctx, writeClient, writeStream)
	if err != nil {
		t.Fatalf("failed to create writer: %v", err)
	}
	tw.RegisterProto(&testdata.SimpleMessage{})

	var totalRows int64

	var results []*AppendResult
	for k, rowSet := range testData {
		totalRows = totalRows + int64(len(rowSet))
		res, err := tw.AppendRows(ctx, rowSet)
		if err != nil {
			t.Errorf("error on append %d: %v", k, err)
			break
		}
		results = append(results, res)
	}

	for k, result := range results {
		fmt.Printf("checking result %d ", k)
		_, err := result.GetResult(ctx)
		if err != nil {
			t.Errorf("got err: %v", err)
		}
		fmt.Printf("...done\n")
	}

	gotRows, err := queryRowCount(ctx, bqClient, testTable)
	if err != nil {
		t.Errorf("failed to get row count: %v", err)
	}

	if gotRows != totalRows {
		t.Errorf("query result mismatch, got %v want %v", gotRows, totalRows)
	}
	log.Printf("got %d rows", gotRows)

}

// A "kick the tires" test that should
func TestIntegration_ThickWriter_Scale(t *testing.T) {
	setupCtx := context.Background()
	ctx, _ := context.WithTimeout(context.Background(), 10*time.Second)

	writeClient, bqClient := getClients(setupCtx, t)
	defer writeClient.Close()
	defer bqClient.Close()

	dataset, cleanupFunc, err := setupTestDataset(setupCtx, t, bqClient)
	if err != nil {
		t.Fatalf("failed to init test dataset: %v", err)
	}
	defer cleanupFunc()

	testTable := dataset.Table(tableIDs.New())

	schema := bigquery.Schema{
		{Name: "name", Type: bigquery.StringFieldType},
		{Name: "value", Type: bigquery.IntegerFieldType},
	}
	if err := testTable.Create(ctx, &bigquery.TableMetadata{Schema: schema}); err != nil {
		t.Fatalf("couldn't create test table %s: %v", testTable.FullyQualifiedName(), err)
	}
	writeStream := fmt.Sprintf("projects/%s/datasets/%s/tables/%s/_default", testTable.ProjectID, testTable.DatasetID, testTable.TableID)

	tw, err := NewThickWriter(ctx, writeClient, writeStream)
	if err != nil {
		t.Fatalf("failed to create writer: %v", err)
	}
	tw.RegisterProto(&testdata.SimpleMessage{})

	// Note: 50k causes EOF error around insert ~33k, probably queue size?
	insertsToGenerate := []int{10, 100, 1000, 5000, 10000, 50000}
	var lastResult *AppendResult
	var expectedRows int64

	watchDogCtx, watchDogCancel := context.WithCancel(ctx)
	// watchdog so we can see progress.
	go func() {
		for {
			select {
			case <-watchDogCtx.Done():
				break
			case <-time.After(500 * time.Millisecond):
				log.Printf("remaining: %d", tw.fc.count())
			}
		}
	}()
	startInserts := time.Now()
	for k, insertCount := range insertsToGenerate {
		startGroup := time.Now()
		// for each batch, insert fake data, and then validate via query.
		var results []*AppendResult
		for i := 0; i < insertCount; i++ {
			rowCount := (i % 10) + 1
			rowData := make([]*testdata.SimpleMessage, rowCount)
			for r := 0; r < rowCount; r++ {
				rowData[r] = &testdata.SimpleMessage{
					Name:  "foo",
					Value: rand.Int63(),
				}
			}
			expectedRows = expectedRows + int64(rowCount)
			// don't retain the appendresult; we'll only check the final count via query.
			result, err := tw.AppendRows(ctx, rowData)
			if err != nil {
				t.Errorf("got insert error on insert group %d, insert %d: %v", k, i, err)
				break
			}
			results = append(results, result)
			lastResult = result
		}
		// checking results roughly halves throughput, turn off for now.
		/*
			for _, result := range results {
				_, err := result.GetResult(ctx)
				if err != nil {
					t.Errorf("got err: %v", err)
				}
			}
		*/

		// commenting out per-group query validation; severe throughput hit for these small scales.
		/*
			gotRows, err := queryRowCount(ctx, bqClient, testTable)
			if err != nil {
				t.Errorf("failed to get row count: %v", err)
			}

			if gotRows != expectedRows {
				t.Errorf("query result mismatch at end of group %d, got %v want %v", k, gotRows, expectedRows)
			}
		*/
		log.Printf("results: %d", len(results))
		log.Printf("insert group %d done (%d inserts, %d rows so far).  Duration %v for group, %v for all inserts", k, insertCount, expectedRows, time.Now().Sub(startGroup), time.Now().Sub(startInserts))
		logMemUsage()
		results = nil
	}
	// done with inserts, cancel watchdog
	watchDogCancel()

	// wait until the last append signals
	_, err = lastResult.GetResult(ctx)
	if err != nil {
		t.Errorf("got err: %v", err)
	}

	gotRows, err := queryRowCount(ctx, bqClient, testTable)
	if err != nil {
		t.Errorf("failed to get row count: %v", err)
	}
	if gotRows != expectedRows {
		t.Errorf("query result mismatch: got %v want %v", gotRows, expectedRows)
	}

}

func logMemUsage() {
	var m runtime.MemStats
	runtime.ReadMemStats(&m)
	log.Printf("Alloc = %v MiB\tTotalAlloc = %v MiB\tSys = %v MiB\tNumGC = %v", m.Alloc/1e6, m.TotalAlloc/1e6, m.Sys/1e6, m.NumGC)
}

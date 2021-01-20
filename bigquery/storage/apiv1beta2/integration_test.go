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

func TestSimpleMessageWithDefaultStream(t *testing.T) {
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
				log.Printf("got err on recv: %v", err)
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
		t.Errorf("sent %d requests, only got %d responses", reqCount, respCount)
	}

	// Verify data is present in the table with a count query.
	sql := fmt.Sprintf("SELECT COUNT(1) FROM `%s`.%s.%s", testTable.ProjectID, testTable.DatasetID, testTable.TableID)
	q := bqClient.Query(sql)
	it, err := q.Read(ctx)
	if err != nil {
		t.Fatalf("failed to issue validation query: %v", err)
	}
	var rowdata []bigquery.Value
	err = it.Next(&rowdata)
	if err != nil {
		t.Fatalf("error iterating validation results: %v", err)
	}

	if rowdata[0] != totalRows {
		t.Errorf("query result mismatch, got %v want %v", rowdata, totalRows)
	}

}

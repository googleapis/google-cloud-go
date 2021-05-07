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

package managedwriter

import (
	"context"
	"fmt"
	"log"
	"testing"
	"time"

	"cloud.google.com/go/bigquery"
	"cloud.google.com/go/internal/testutil"
	"cloud.google.com/go/internal/uid"
	"google.golang.org/api/option"
	"google.golang.org/protobuf/reflect/protodesc"

	storage "cloud.google.com/go/bigquery/storage/apiv1beta2"
	"cloud.google.com/go/bigquery/storage/managedwriter/testdata"
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
func getClients(ctx context.Context, t *testing.T, opts ...option.ClientOption) (*storage.BigQueryWriteClient, *bigquery.Client) {
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
	writeClient, err := storage.NewBigQueryWriteClient(ctx)
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

/*
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

*/

func TestIntegration_ManagedWriter_Default(t *testing.T) {
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

	testData := []*testdata.SimpleMessage{
		{Name: "one", Value: 1},
		{Name: "two", Value: 2},
		{Name: "three", Value: 3},
		{Name: "four", Value: 1},
		{Name: "five", Value: 2},
	}

	// Construct a simple serializer via reflecting on the message.
	m := &testdata.SimpleMessage{}
	rs := &simpleRowSerializer{
		DescFn:    staticDescFn(protodesc.ToDescriptorProto(m.ProtoReflect().Descriptor())),
		ConvertFn: marshalConvert,
	}

	mw, err := NewManagedWriter(ctx, writeClient, testTable,
		WithRowSerializer(rs),
		WithType(DefaultStream),
	)
	if err != nil {
		t.Fatalf("failed to create managed writer: %v", err)
	}

	var totalRows int64

	var results []*AppendResult
	for k, d := range testData {
		log.Printf("appending element %d", k)
		totalRows = totalRows + 1
		ar, err := mw.AppendRows(d, 0)
		log.Println("appended")
		if err != nil {
			t.Errorf("error on append %d: %v", totalRows, err)
			break
		}
		results = append(results, ar...)
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

func TestIntegration_ManagedWriter_Pending(t *testing.T) {
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

	testData := []*testdata.SimpleMessage{
		{Name: "one", Value: 1},
		{Name: "two", Value: 2},
		{Name: "three", Value: 3},
		{Name: "four", Value: 1},
		{Name: "five", Value: 2},
	}

	// construct a simple serializer via reflecting on the message.
	m := &testdata.SimpleMessage{}
	rs := &simpleRowSerializer{
		DescFn:    staticDescFn(protodesc.ToDescriptorProto(m.ProtoReflect().Descriptor())),
		ConvertFn: marshalConvert,
	}

	mw, err := NewManagedWriter(ctx, writeClient, testTable,
		WithRowSerializer(rs),
		WithType(PendingStream),
	)
	if err != nil {
		t.Fatalf("failed to create managed writer: %v", err)
	}

	var totalRows int64

	var results []*AppendResult
	for k, d := range testData {
		log.Printf("appending element %d", k)
		totalRows = totalRows + 1
		ar, err := mw.AppendRows(d, 0)
		if err != nil {
			t.Errorf("error on append %d: %v", totalRows, err)
			break
		}
		results = append(results, ar...)
	}

	for k, result := range results {
		fmt.Printf("checking result %d ", k)
		_, err := result.GetResult(ctx)
		if err != nil {
			t.Errorf("got err: %v", err)
		}
		fmt.Printf("...done\n")
	}

	finalizeCount, err := mw.Finalize(ctx)
	if err != nil {
		t.Fatalf("finalize errored: %v", err)
	}
	if finalizeCount != totalRows {
		t.Errorf("wanted %d total rows, finalized %d rows", totalRows, finalizeCount)
	}

	gotRows, err := queryRowCount(ctx, bqClient, testTable)
	if err != nil {
		t.Errorf("failed to get row count: %v", err)
	}

	if gotRows != 0 {
		t.Errorf("haven't commited, expected no rows got %d", gotRows)
	}

	resp, err := mw.Commit(ctx)
	if err != nil {
		t.Fatalf("failed commit: %v", err)
	}

	log.Printf("commit: %v", resp.GetCommitTime())

	gotRows, err = queryRowCount(ctx, bqClient, testTable)
	if err != nil {
		t.Errorf("failed post-commit row count: %v", err)
	}

	if gotRows != totalRows {
		t.Errorf("mismatch after commit: wanted %d got %d", totalRows, gotRows)
	}

	log.Printf("got %d rows", gotRows)

}

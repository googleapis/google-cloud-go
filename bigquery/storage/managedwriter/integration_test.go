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
	"testing"
	"time"

	"cloud.google.com/go/bigquery"
	"cloud.google.com/go/bigquery/storage/managedwriter/adapt"
	"cloud.google.com/go/bigquery/storage/managedwriter/testdata"
	"cloud.google.com/go/internal/testutil"
	"cloud.google.com/go/internal/uid"
	"google.golang.org/api/option"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/reflect/protodesc"
	"google.golang.org/protobuf/reflect/protoreflect"
	"google.golang.org/protobuf/types/descriptorpb"
)

var (
	datasetIDs         = uid.NewSpace("managedwriter_test_dataset", &uid.Options{Sep: '_', Time: time.Now()})
	tableIDs           = uid.NewSpace("table", &uid.Options{Sep: '_', Time: time.Now()})
	defaultTestTimeout = 15 * time.Second
)

func getTestClients(ctx context.Context, t *testing.T, opts ...option.ClientOption) (*Client, *bigquery.Client) {
	if testing.Short() {
		t.Skip("Integration tests skipped in short mode")
	}
	projID := testutil.ProjID()
	if projID == "" {
		t.Skip("Integration tests skipped. See CONTRIBUTING.md for details")
	}
	ts := testutil.TokenSource(ctx, "https://www.googleapis.com/auth/bigquery")
	if ts == nil {
		t.Skip("Integration tests skipped. See CONTRIBUTING.md for details")
	}
	opts = append(opts, option.WithTokenSource(ts))
	client, err := NewClient(ctx, projID, opts...)
	if err != nil {
		t.Fatalf("couldn't create managedwriter client: %v", err)
	}

	bqClient, err := bigquery.NewClient(ctx, projID, opts...)
	if err != nil {
		t.Fatalf("couldn't create bigquery client: %v", err)
	}
	return client, bqClient
}

// validateRowCount confirms the number of rows in a table visible to the query engine.
func validateRowCount(ctx context.Context, t *testing.T, client *bigquery.Client, tbl *bigquery.Table, expectedRows int64) {

	// Verify data is present in the table with a count query.
	sql := fmt.Sprintf("SELECT COUNT(1) FROM `%s`.%s.%s", tbl.ProjectID, tbl.DatasetID, tbl.TableID)
	q := client.Query(sql)
	it, err := q.Read(ctx)
	if err != nil {
		t.Errorf("failed to issue validation query: %v", err)
		return
	}
	var rowdata []bigquery.Value
	err = it.Next(&rowdata)
	if err != nil {
		t.Errorf("error fetching validation results: %v", err)
		return
	}
	count, ok := rowdata[0].(int64)
	if !ok {
		t.Errorf("got unexpected data from validation query: %v", rowdata[0])
	}
	if count != expectedRows {
		t.Errorf("rows mismatch expected rows: got %d want %d", count, expectedRows)
	}
}

// setupTestDataset generates a unique dataset for testing, and a cleanup that can be deferred.
func setupTestDataset(ctx context.Context, t *testing.T, bqc *bigquery.Client) (ds *bigquery.Dataset, cleanup func(), err error) {
	dataset := bqc.Dataset(datasetIDs.New())
	if err := dataset.Create(ctx, nil); err != nil {
		return nil, nil, err
	}
	return dataset, func() {
		if err := dataset.DeleteWithContents(ctx); err != nil {
			t.Logf("could not cleanup dataset %s: %v", dataset.DatasetID, err)
		}
	}, nil
}

// setupDynamicDescriptors aids testing when not using a supplied proto
func setupDynamicDescriptors(t *testing.T, schema bigquery.Schema) (protoreflect.MessageDescriptor, *descriptorpb.DescriptorProto) {
	convertedSchema, err := adapt.BQSchemaToStorageTableSchema(schema)
	if err != nil {
		t.Fatalf("adapt.BQSchemaToStorageTableSchema: %v", err)
	}

	descriptor, err := adapt.StorageSchemaToDescriptor(convertedSchema, "root")
	if err != nil {
		t.Fatalf("adapt.StorageSchemaToDescriptor: %v", err)
	}
	messageDescriptor, ok := descriptor.(protoreflect.MessageDescriptor)
	if !ok {
		t.Fatalf("adapted descriptor is not a message descriptor")
	}
	return messageDescriptor, protodesc.ToDescriptorProto(messageDescriptor)
}

func TestIntegration_ManagedWriter_BasicOperation(t *testing.T) {
	mwClient, bqClient := getTestClients(context.Background(), t)
	defer mwClient.Close()
	defer bqClient.Close()

	dataset, cleanup, err := setupTestDataset(context.Background(), t, bqClient)
	if err != nil {
		t.Fatalf("failed to init test dataset: %v", err)
	}
	defer cleanup()

	ctx, cancel := context.WithTimeout(context.Background(), defaultTestTimeout)
	defer cancel()

	// prep a suitable destination table.
	testTable := dataset.Table(tableIDs.New())
	schema := bigquery.Schema{
		{Name: "name", Type: bigquery.StringFieldType, Required: true},
		{Name: "value", Type: bigquery.IntegerFieldType, Required: true},
	}
	if err := testTable.Create(ctx, &bigquery.TableMetadata{Schema: schema}); err != nil {
		t.Fatalf("failed to create test table %s: %v", testTable.FullyQualifiedName(), err)
	}
	// We'll use a precompiled test proto, but we need it's corresponding descriptorproto representation
	// to send as the stream's schema.
	m := &testdata.SimpleMessage{}
	descriptorProto := protodesc.ToDescriptorProto(m.ProtoReflect().Descriptor())

	// setup a new stream.
	ms, err := mwClient.NewManagedStream(ctx,
		WithDestinationTable(fmt.Sprintf("projects/%s/datasets/%s/tables/%s", testTable.ProjectID, testTable.DatasetID, testTable.TableID)),
		WithType(DefaultStream),
		WithSchemaDescriptor(descriptorProto),
	)
	if err != nil {
		t.Fatalf("NewManagedStream: %v", err)
	}

	// prevalidate we have no data in table.
	validateRowCount(ctx, t, bqClient, testTable, 0)

	testData := []*testdata.SimpleMessage{
		{Name: "one", Value: 1},
		{Name: "two", Value: 2},
		{Name: "three", Value: 3},
		{Name: "four", Value: 1},
		{Name: "five", Value: 2},
	}

	// First, send the test rows individually.
	var results []*AppendResult
	for k, mesg := range testData {
		b, err := proto.Marshal(mesg)
		if err != nil {
			t.Errorf("failed to marshal message %d: %v", k, err)
		}
		data := [][]byte{b}
		results, err = ms.AppendRows(data, NoStreamOffset)
		if err != nil {
			t.Errorf("single-row append %d failed: %v", k, err)
		}
	}
	// wait for the result to indicate ready, then validate.
	results[0].Ready()
	wantRows := int64(len(testData))
	validateRowCount(ctx, t, bqClient, testTable, wantRows)

	// Now, send the test rows grouped into in a single append.
	var data [][]byte
	for k, mesg := range testData {
		b, err := proto.Marshal(mesg)
		if err != nil {
			t.Errorf("failed to marshal message %d: %v", k, err)
		}
		data := append(data, b)
		results, err = ms.AppendRows(data, NoStreamOffset)
		if err != nil {
			t.Errorf("grouped-row append failed: %v", err)
		}
	}
	// wait for the result to indicate ready, then validate again.
	results[0].Ready()
	wantRows = wantRows * 2
	validateRowCount(ctx, t, bqClient, testTable, wantRows)
}

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
	"math"
	"sync"
	"testing"
	"time"

	"cloud.google.com/go/bigquery"
	"cloud.google.com/go/bigquery/storage/managedwriter/adapt"
	"cloud.google.com/go/bigquery/storage/managedwriter/testdata"
	"cloud.google.com/go/internal/testutil"
	"cloud.google.com/go/internal/uid"
	"github.com/googleapis/gax-go/v2/apierror"
	"go.opencensus.io/stats/view"
	"google.golang.org/api/option"
	storagepb "google.golang.org/genproto/googleapis/cloud/bigquery/storage/v1"
	"google.golang.org/grpc/codes"
	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/reflect/protodesc"
	"google.golang.org/protobuf/reflect/protoreflect"
	"google.golang.org/protobuf/types/descriptorpb"
	"google.golang.org/protobuf/types/dynamicpb"
	"google.golang.org/protobuf/types/known/wrapperspb"
)

var (
	datasetIDs         = uid.NewSpace("managedwriter_test_dataset", &uid.Options{Sep: '_', Time: time.Now()})
	tableIDs           = uid.NewSpace("table", &uid.Options{Sep: '_', Time: time.Now()})
	defaultTestTimeout = 45 * time.Second
)

// our test data has cardinality 5 for names, 3 for values
var testSimpleData = []*testdata.SimpleMessageProto2{
	{Name: proto.String("one"), Value: proto.Int64(1)},
	{Name: proto.String("two"), Value: proto.Int64(2)},
	{Name: proto.String("three"), Value: proto.Int64(3)},
	{Name: proto.String("four"), Value: proto.Int64(1)},
	{Name: proto.String("five"), Value: proto.Int64(2)},
}

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

// setupTestDataset generates a unique dataset for testing, and a cleanup that can be deferred.
func setupTestDataset(ctx context.Context, t *testing.T, bqc *bigquery.Client, location string) (ds *bigquery.Dataset, cleanup func(), err error) {
	dataset := bqc.Dataset(datasetIDs.New())
	if err := dataset.Create(ctx, &bigquery.DatasetMetadata{Location: location}); err != nil {
		return nil, nil, err
	}
	return dataset, func() {
		if err := dataset.DeleteWithContents(ctx); err != nil {
			t.Logf("could not cleanup dataset %q: %v", dataset.DatasetID, err)
		}
	}, nil
}

// setupDynamicDescriptors aids testing when not using a supplied proto
func setupDynamicDescriptors(t *testing.T, schema bigquery.Schema) (protoreflect.MessageDescriptor, *descriptorpb.DescriptorProto) {
	convertedSchema, err := adapt.BQSchemaToStorageTableSchema(schema)
	if err != nil {
		t.Fatalf("adapt.BQSchemaToStorageTableSchema: %v", err)
	}

	descriptor, err := adapt.StorageSchemaToProto2Descriptor(convertedSchema, "root")
	if err != nil {
		t.Fatalf("adapt.StorageSchemaToDescriptor: %v", err)
	}
	messageDescriptor, ok := descriptor.(protoreflect.MessageDescriptor)
	if !ok {
		t.Fatalf("adapted descriptor is not a message descriptor")
	}
	return messageDescriptor, protodesc.ToDescriptorProto(messageDescriptor)
}

func TestIntegration_ManagedWriter(t *testing.T) {
	mwClient, bqClient := getTestClients(context.Background(), t)
	defer mwClient.Close()
	defer bqClient.Close()

	dataset, cleanup, err := setupTestDataset(context.Background(), t, bqClient, "us-east1")
	if err != nil {
		t.Fatalf("failed to init test dataset: %v", err)
	}
	defer cleanup()

	ctx, cancel := context.WithTimeout(context.Background(), defaultTestTimeout)
	defer cancel()

	t.Run("group", func(t *testing.T) {
		t.Run("DefaultStream", func(t *testing.T) {
			t.Parallel()
			testDefaultStream(ctx, t, mwClient, bqClient, dataset)
		})
		t.Run("DefaultStreamDynamicJSON", func(t *testing.T) {
			t.Parallel()
			testDefaultStreamDynamicJSON(ctx, t, mwClient, bqClient, dataset)
		})
		t.Run("CommittedStream", func(t *testing.T) {
			t.Parallel()
			testCommittedStream(ctx, t, mwClient, bqClient, dataset)
		})
		t.Run("ErrorBehaviors", func(t *testing.T) {
			t.Parallel()
			testErrorBehaviors(ctx, t, mwClient, bqClient, dataset)
		})
		t.Run("BufferedStream", func(t *testing.T) {
			t.Parallel()
			testBufferedStream(ctx, t, mwClient, bqClient, dataset)
		})
		t.Run("PendingStream", func(t *testing.T) {
			t.Parallel()
			testPendingStream(ctx, t, mwClient, bqClient, dataset)
		})
		t.Run("SchemaEvolution", func(t *testing.T) {
			t.Parallel()
			testSchemaEvolution(ctx, t, mwClient, bqClient, dataset)
		})
		t.Run("Instrumentation", func(t *testing.T) {
			// Don't run this in parallel, we only want to collect stats from this subtest.
			testInstrumentation(ctx, t, mwClient, bqClient, dataset)
		})
		t.Run("TestLargeInsert", func(t *testing.T) {
			testLargeInsert(ctx, t, mwClient, bqClient, dataset)
		})
	})
}

func testDefaultStream(ctx context.Context, t *testing.T, mwClient *Client, bqClient *bigquery.Client, dataset *bigquery.Dataset) {
	testTable := dataset.Table(tableIDs.New())
	if err := testTable.Create(ctx, &bigquery.TableMetadata{Schema: testdata.SimpleMessageSchema}); err != nil {
		t.Fatalf("failed to create test table %q: %v", testTable.FullyQualifiedName(), err)
	}
	// We'll use a precompiled test proto, but we need it's corresponding descriptorproto representation
	// to send as the stream's schema.
	m := &testdata.SimpleMessageProto2{}
	descriptorProto := protodesc.ToDescriptorProto(m.ProtoReflect().Descriptor())

	// setup a new stream.
	ms, err := mwClient.NewManagedStream(ctx,
		WithDestinationTable(TableParentFromParts(testTable.ProjectID, testTable.DatasetID, testTable.TableID)),
		WithType(DefaultStream),
		WithSchemaDescriptor(descriptorProto),
	)
	if err != nil {
		t.Fatalf("NewManagedStream: %v", err)
	}
	validateTableConstraints(ctx, t, bqClient, testTable, "before send",
		withExactRowCount(0))

	// First, send the test rows individually.
	var result *AppendResult
	for k, mesg := range testSimpleData {
		b, err := proto.Marshal(mesg)
		if err != nil {
			t.Errorf("failed to marshal message %d: %v", k, err)
		}
		data := [][]byte{b}
		result, err = ms.AppendRows(ctx, data)
		if err != nil {
			t.Errorf("single-row append %d failed: %v", k, err)
		}
	}
	// Wait for the result to indicate ready, then validate.
	o, err := result.GetResult(ctx)
	if err != nil {
		t.Errorf("result error for last send: %v", err)
	}
	if o != NoStreamOffset {
		t.Errorf("offset mismatch, got %d want %d", o, NoStreamOffset)
	}
	validateTableConstraints(ctx, t, bqClient, testTable, "after first send round",
		withExactRowCount(int64(len(testSimpleData))),
		withDistinctValues("name", int64(len(testSimpleData))))

	// Now, send the test rows grouped into in a single append.
	var data [][]byte
	for k, mesg := range testSimpleData {
		b, err := proto.Marshal(mesg)
		if err != nil {
			t.Errorf("failed to marshal message %d: %v", k, err)
		}
		data = append(data, b)
	}
	result, err = ms.AppendRows(ctx, data)
	if err != nil {
		t.Errorf("grouped-row append failed: %v", err)
	}
	// Wait for the result to indicate ready, then validate again.  Our total rows have increased, but
	// cardinality should not.
	o, err = result.GetResult(ctx)
	if err != nil {
		t.Errorf("result error for last send: %v", err)
	}
	if o != NoStreamOffset {
		t.Errorf("offset mismatch, got %d want %d", o, NoStreamOffset)
	}
	validateTableConstraints(ctx, t, bqClient, testTable, "after second send round",
		withExactRowCount(int64(2*len(testSimpleData))),
		withDistinctValues("name", int64(len(testSimpleData))),
		withDistinctValues("value", int64(3)),
	)
}

func testDefaultStreamDynamicJSON(ctx context.Context, t *testing.T, mwClient *Client, bqClient *bigquery.Client, dataset *bigquery.Dataset) {
	testTable := dataset.Table(tableIDs.New())
	if err := testTable.Create(ctx, &bigquery.TableMetadata{Schema: testdata.SimpleMessageSchema}); err != nil {
		t.Fatalf("failed to create test table %s: %v", testTable.FullyQualifiedName(), err)
	}

	md, descriptorProto := setupDynamicDescriptors(t, testdata.SimpleMessageSchema)

	ms, err := mwClient.NewManagedStream(ctx,
		WithDestinationTable(TableParentFromParts(testTable.ProjectID, testTable.DatasetID, testTable.TableID)),
		WithType(DefaultStream),
		WithSchemaDescriptor(descriptorProto),
	)
	if err != nil {
		t.Fatalf("NewManagedStream: %v", err)
	}
	validateTableConstraints(ctx, t, bqClient, testTable, "before send",
		withExactRowCount(0))

	sampleJSONData := [][]byte{
		[]byte(`{"name": "one", "value": 1}`),
		[]byte(`{"name": "two", "value": 2}`),
		[]byte(`{"name": "three", "value": 3}`),
		[]byte(`{"name": "four", "value": 4}`),
		[]byte(`{"name": "five", "value": 5}`),
	}

	var result *AppendResult
	for k, v := range sampleJSONData {
		message := dynamicpb.NewMessage(md)

		// First, json->proto message
		err = protojson.Unmarshal(v, message)
		if err != nil {
			t.Fatalf("failed to Unmarshal json message for row %d: %v", k, err)
		}
		// Then, proto message -> bytes.
		b, err := proto.Marshal(message)
		if err != nil {
			t.Fatalf("failed to marshal proto bytes for row %d: %v", k, err)
		}
		result, err = ms.AppendRows(ctx, [][]byte{b})
		if err != nil {
			t.Errorf("single-row append %d failed: %v", k, err)
		}
	}

	// Wait for the result to indicate ready, then validate.
	o, err := result.GetResult(ctx)
	if err != nil {
		t.Errorf("result error for last send: %v", err)
	}
	if o != NoStreamOffset {
		t.Errorf("offset mismatch, got %d want %d", o, NoStreamOffset)
	}
	validateTableConstraints(ctx, t, bqClient, testTable, "after send",
		withExactRowCount(int64(len(sampleJSONData))),
		withDistinctValues("name", int64(len(sampleJSONData))),
		withDistinctValues("value", int64(len(sampleJSONData))))
}

func testBufferedStream(ctx context.Context, t *testing.T, mwClient *Client, bqClient *bigquery.Client, dataset *bigquery.Dataset) {
	testTable := dataset.Table(tableIDs.New())
	if err := testTable.Create(ctx, &bigquery.TableMetadata{Schema: testdata.SimpleMessageSchema}); err != nil {
		t.Fatalf("failed to create test table %s: %v", testTable.FullyQualifiedName(), err)
	}

	m := &testdata.SimpleMessageProto2{}
	descriptorProto := protodesc.ToDescriptorProto(m.ProtoReflect().Descriptor())

	ms, err := mwClient.NewManagedStream(ctx,
		WithDestinationTable(TableParentFromParts(testTable.ProjectID, testTable.DatasetID, testTable.TableID)),
		WithType(BufferedStream),
		WithSchemaDescriptor(descriptorProto),
	)
	if err != nil {
		t.Fatalf("NewManagedStream: %v", err)
	}

	info, err := ms.c.getWriteStream(ctx, ms.streamSettings.streamID)
	if err != nil {
		t.Errorf("couldn't get stream info: %v", err)
	}
	if info.GetType().String() != string(ms.StreamType()) {
		t.Errorf("mismatch on stream type, got %s want %s", info.GetType(), ms.StreamType())
	}
	validateTableConstraints(ctx, t, bqClient, testTable, "before send",
		withExactRowCount(0))

	var expectedRows int64
	for k, mesg := range testSimpleData {
		b, err := proto.Marshal(mesg)
		if err != nil {
			t.Errorf("failed to marshal message %d: %v", k, err)
		}
		data := [][]byte{b}
		results, err := ms.AppendRows(ctx, data)
		if err != nil {
			t.Errorf("single-row append %d failed: %v", k, err)
		}
		// Wait for acknowledgement.
		offset, err := results.GetResult(ctx)
		if err != nil {
			t.Errorf("got error from pending result %d: %v", k, err)
		}
		validateTableConstraints(ctx, t, bqClient, testTable, fmt.Sprintf("before flush %d", k),
			withExactRowCount(expectedRows),
			withDistinctValues("name", expectedRows))

		// move offset and re-validate.
		flushOffset, err := ms.FlushRows(ctx, offset)
		if err != nil {
			t.Errorf("failed to flush offset to %d: %v", offset, err)
		}
		expectedRows = flushOffset + 1
		validateTableConstraints(ctx, t, bqClient, testTable, fmt.Sprintf("after flush %d", k),
			withExactRowCount(expectedRows),
			withDistinctValues("name", expectedRows))
	}
}

func testCommittedStream(ctx context.Context, t *testing.T, mwClient *Client, bqClient *bigquery.Client, dataset *bigquery.Dataset) {
	testTable := dataset.Table(tableIDs.New())
	if err := testTable.Create(ctx, &bigquery.TableMetadata{Schema: testdata.SimpleMessageSchema}); err != nil {
		t.Fatalf("failed to create test table %s: %v", testTable.FullyQualifiedName(), err)
	}

	m := &testdata.SimpleMessageProto2{}
	descriptorProto := protodesc.ToDescriptorProto(m.ProtoReflect().Descriptor())

	// setup a new stream.
	ms, err := mwClient.NewManagedStream(ctx,
		WithDestinationTable(TableParentFromParts(testTable.ProjectID, testTable.DatasetID, testTable.TableID)),
		WithType(CommittedStream),
		WithSchemaDescriptor(descriptorProto),
	)
	if err != nil {
		t.Fatalf("NewManagedStream: %v", err)
	}
	validateTableConstraints(ctx, t, bqClient, testTable, "before send",
		withExactRowCount(0))

	var result *AppendResult
	for k, mesg := range testSimpleData {
		b, err := proto.Marshal(mesg)
		if err != nil {
			t.Errorf("failed to marshal message %d: %v", k, err)
		}
		data := [][]byte{b}
		result, err = ms.AppendRows(ctx, data, WithOffset(int64(k)))
		if err != nil {
			t.Errorf("single-row append %d failed: %v", k, err)
		}
	}
	// Wait for the result to indicate ready, then validate.
	o, err := result.GetResult(ctx)
	if err != nil {
		t.Errorf("result error for last send: %v", err)
	}
	wantOffset := int64(len(testSimpleData) - 1)
	if o != wantOffset {
		t.Errorf("offset mismatch, got %d want %d", o, wantOffset)
	}
	validateTableConstraints(ctx, t, bqClient, testTable, "after send",
		withExactRowCount(int64(len(testSimpleData))))
}

// testErrorBehaviors intentionally issues problematic requests to verify error behaviors.
func testErrorBehaviors(ctx context.Context, t *testing.T, mwClient *Client, bqClient *bigquery.Client, dataset *bigquery.Dataset) {
	testTable := dataset.Table(tableIDs.New())
	if err := testTable.Create(ctx, &bigquery.TableMetadata{Schema: testdata.SimpleMessageSchema}); err != nil {
		t.Fatalf("failed to create test table %s: %v", testTable.FullyQualifiedName(), err)
	}

	m := &testdata.SimpleMessageProto2{}
	descriptorProto := protodesc.ToDescriptorProto(m.ProtoReflect().Descriptor())

	// setup a new stream.
	ms, err := mwClient.NewManagedStream(ctx,
		WithDestinationTable(TableParentFromParts(testTable.ProjectID, testTable.DatasetID, testTable.TableID)),
		WithType(CommittedStream),
		WithSchemaDescriptor(descriptorProto),
	)
	if err != nil {
		t.Fatalf("NewManagedStream: %v", err)
	}
	validateTableConstraints(ctx, t, bqClient, testTable, "before send",
		withExactRowCount(0))

	data := make([][]byte, len(testSimpleData))
	for k, mesg := range testSimpleData {
		b, err := proto.Marshal(mesg)
		if err != nil {
			t.Errorf("failed to marshal message %d: %v", k, err)
		}
		data[k] = b
	}

	// Send an append at an invalid offset.
	result, err := ms.AppendRows(ctx, data, WithOffset(99))
	if err != nil {
		t.Errorf("failed to send append: %v", err)
	}
	//
	off, err := result.GetResult(ctx)
	if err == nil {
		t.Errorf("expected error, got offset %d", off)
	}

	apiErr, ok := apierror.FromError(err)
	if !ok {
		t.Errorf("expected apierror, got %T: %v", err, err)
	}
	se := &storagepb.StorageError{}
	e := apiErr.Details().ExtractProtoMessage(se)
	if e != nil {
		t.Errorf("expected storage error, but extraction failed: %v", e)
	}
	wantCode := storagepb.StorageError_OFFSET_OUT_OF_RANGE
	if se.GetCode() != wantCode {
		t.Errorf("wanted %s, got %s", wantCode.String(), se.GetCode().String())
	}
	// Send "real" append to advance the offset.
	result, err = ms.AppendRows(ctx, data, WithOffset(0))
	if err != nil {
		t.Errorf("failed to send append: %v", err)
	}
	off, err = result.GetResult(ctx)
	if err != nil {
		t.Errorf("expected offset, got error %v", err)
	}
	wantOffset := int64(0)
	if off != wantOffset {
		t.Errorf("offset mismatch, got %d want %d", off, wantOffset)
	}
	// Now, send at the start offset again.
	result, err = ms.AppendRows(ctx, data, WithOffset(0))
	if err != nil {
		t.Errorf("failed to send append: %v", err)
	}
	off, err = result.GetResult(ctx)
	if err == nil {
		t.Errorf("expected error, got offset %d", off)
	}
	apiErr, ok = apierror.FromError(err)
	if !ok {
		t.Errorf("expected apierror, got %T: %v", err, err)
	}
	se = &storagepb.StorageError{}
	e = apiErr.Details().ExtractProtoMessage(se)
	if e != nil {
		t.Errorf("expected storage error, but extraction failed: %v", e)
	}
	wantCode = storagepb.StorageError_OFFSET_ALREADY_EXISTS
	if se.GetCode() != wantCode {
		t.Errorf("wanted %s, got %s", wantCode.String(), se.GetCode().String())
	}
	// Finalize the stream.
	if _, err := ms.Finalize(ctx); err != nil {
		t.Errorf("Finalize had error: %v", err)
	}
	// Send another append, which is disallowed for finalized streams.
	result, err = ms.AppendRows(ctx, data)
	if err != nil {
		t.Errorf("failed to send append: %v", err)
	}
	off, err = result.GetResult(ctx)
	if err == nil {
		t.Errorf("expected error, got offset %d", off)
	}
	apiErr, ok = apierror.FromError(err)
	if !ok {
		t.Errorf("expected apierror, got %T: %v", err, err)
	}
	se = &storagepb.StorageError{}
	e = apiErr.Details().ExtractProtoMessage(se)
	if e != nil {
		t.Errorf("expected storage error, but extraction failed: %v", e)
	}
	wantCode = storagepb.StorageError_STREAM_FINALIZED
	if se.GetCode() != wantCode {
		t.Errorf("wanted %s, got %s", wantCode.String(), se.GetCode().String())
	}
}

func testPendingStream(ctx context.Context, t *testing.T, mwClient *Client, bqClient *bigquery.Client, dataset *bigquery.Dataset) {
	testTable := dataset.Table(tableIDs.New())
	if err := testTable.Create(ctx, &bigquery.TableMetadata{Schema: testdata.SimpleMessageSchema}); err != nil {
		t.Fatalf("failed to create test table %s: %v", testTable.FullyQualifiedName(), err)
	}

	m := &testdata.SimpleMessageProto2{}
	descriptorProto := protodesc.ToDescriptorProto(m.ProtoReflect().Descriptor())

	ms, err := mwClient.NewManagedStream(ctx,
		WithDestinationTable(TableParentFromParts(testTable.ProjectID, testTable.DatasetID, testTable.TableID)),
		WithType(PendingStream),
		WithSchemaDescriptor(descriptorProto),
	)
	if err != nil {
		t.Fatalf("NewManagedStream: %v", err)
	}
	validateTableConstraints(ctx, t, bqClient, testTable, "before send",
		withExactRowCount(0))

	// Send data.
	var result *AppendResult
	for k, mesg := range testSimpleData {
		b, err := proto.Marshal(mesg)
		if err != nil {
			t.Errorf("failed to marshal message %d: %v", k, err)
		}
		data := [][]byte{b}
		result, err = ms.AppendRows(ctx, data, WithOffset(int64(k)))
		if err != nil {
			t.Errorf("single-row append %d failed: %v", k, err)
		}
		// Be explicit about waiting/checking each response.
		off, err := result.GetResult(ctx)
		if err != nil {
			t.Errorf("response %d error: %v", k, err)
		}
		if off != int64(k) {
			t.Errorf("offset mismatch, got %d want %d", off, k)
		}
	}
	wantRows := int64(len(testSimpleData))

	// Mark stream complete.
	trackedOffset, err := ms.Finalize(ctx)
	if err != nil {
		t.Errorf("Finalize: %v", err)
	}

	if trackedOffset != wantRows {
		t.Errorf("Finalize mismatched offset, got %d want %d", trackedOffset, wantRows)
	}

	// Commit stream and validate.
	req := &storagepb.BatchCommitWriteStreamsRequest{
		Parent:       TableParentFromStreamName(ms.StreamName()),
		WriteStreams: []string{ms.StreamName()},
	}

	resp, err := mwClient.BatchCommitWriteStreams(ctx, req)
	if err != nil {
		t.Errorf("client.BatchCommit: %v", err)
	}
	if len(resp.StreamErrors) > 0 {
		t.Errorf("stream errors present: %v", resp.StreamErrors)
	}
	validateTableConstraints(ctx, t, bqClient, testTable, "after send",
		withExactRowCount(int64(len(testSimpleData))))
}

func testLargeInsert(ctx context.Context, t *testing.T, mwClient *Client, bqClient *bigquery.Client, dataset *bigquery.Dataset) {
	testTable := dataset.Table(tableIDs.New())
	if err := testTable.Create(ctx, &bigquery.TableMetadata{Schema: testdata.SimpleMessageSchema}); err != nil {
		t.Fatalf("failed to create test table %s: %v", testTable.FullyQualifiedName(), err)
	}

	m := &testdata.SimpleMessageProto2{}
	descriptorProto := protodesc.ToDescriptorProto(m.ProtoReflect().Descriptor())

	ms, err := mwClient.NewManagedStream(ctx,
		WithDestinationTable(TableParentFromParts(testTable.ProjectID, testTable.DatasetID, testTable.TableID)),
		WithType(CommittedStream),
		WithSchemaDescriptor(descriptorProto),
	)
	if err != nil {
		t.Fatalf("NewManagedStream: %v", err)
	}
	validateTableConstraints(ctx, t, bqClient, testTable, "before send",
		withExactRowCount(0))

	// Construct a Very Large request.
	var data [][]byte
	targetSize := 11 * 1024 * 1024 // 11 MB
	b, err := proto.Marshal(testSimpleData[0])
	if err != nil {
		t.Errorf("failed to marshal message: %v", err)
	}

	numRows := targetSize / len(b)
	data = make([][]byte, numRows)

	for i := 0; i < numRows; i++ {
		data[i] = b
	}

	result, err := ms.AppendRows(ctx, data, WithOffset(0))
	if err != nil {
		t.Errorf("single append failed: %v", err)
	}
	_, err = result.GetResult(ctx)
	if err != nil {
		apiErr, ok := apierror.FromError(err)
		if !ok {
			t.Errorf("GetResult error was not an instance of ApiError")
		}
		status := apiErr.GRPCStatus()
		if status.Code() != codes.InvalidArgument {
			t.Errorf("expected InvalidArgument status, got %v", status)
		}
	}
	// send a subsequent append as verification we can proceed.
	result, err = ms.AppendRows(ctx, [][]byte{b})
	if err != nil {
		t.Fatalf("subsequent append failed: %v", err)
	}
	_, err = result.GetResult(ctx)
	if err != nil {
		t.Errorf("failure result from second append: %v", err)
	}
}

func testInstrumentation(ctx context.Context, t *testing.T, mwClient *Client, bqClient *bigquery.Client, dataset *bigquery.Dataset) {
	testedViews := []*view.View{
		AppendRequestsView,
		AppendResponsesView,
		AppendClientOpenView,
	}

	if err := view.Register(testedViews...); err != nil {
		t.Fatalf("couldn't register views: %v", err)
	}

	testTable := dataset.Table(tableIDs.New())
	if err := testTable.Create(ctx, &bigquery.TableMetadata{Schema: testdata.SimpleMessageSchema}); err != nil {
		t.Fatalf("failed to create test table %q: %v", testTable.FullyQualifiedName(), err)
	}

	m := &testdata.SimpleMessageProto2{}
	descriptorProto := protodesc.ToDescriptorProto(m.ProtoReflect().Descriptor())

	// setup a new stream.
	ms, err := mwClient.NewManagedStream(ctx,
		WithDestinationTable(TableParentFromParts(testTable.ProjectID, testTable.DatasetID, testTable.TableID)),
		WithType(DefaultStream),
		WithSchemaDescriptor(descriptorProto),
	)
	if err != nil {
		t.Fatalf("NewManagedStream: %v", err)
	}

	var result *AppendResult
	for k, mesg := range testSimpleData {
		b, err := proto.Marshal(mesg)
		if err != nil {
			t.Errorf("failed to marshal message %d: %v", k, err)
		}
		data := [][]byte{b}
		result, err = ms.AppendRows(ctx, data)
		if err != nil {
			t.Errorf("single-row append %d failed: %v", k, err)
		}
	}
	// Wait for the result to indicate ready.
	result.Ready()
	// Ick.  Stats reporting can't force flushing, and there's a race here.  Sleep to give the recv goroutine a chance
	// to report.
	time.Sleep(time.Second)

	for _, tv := range testedViews {
		// Attempt to further improve race failures by retrying metrics retrieval.
		metricData, err := func() ([]*view.Row, error) {
			attempt := 0
			for {
				data, err := view.RetrieveData(tv.Name)
				attempt = attempt + 1
				if attempt > 5 {
					return data, err
				}
				if err == nil && len(data) == 1 {
					return data, err
				}
				time.Sleep(time.Duration(attempt) * 500 * time.Millisecond)
			}
		}()
		if err != nil {
			t.Errorf("view %q RetrieveData: %v", tv.Name, err)
		}
		if mlen := len(metricData); mlen != 1 {
			t.Errorf("%q: expected 1 row of metrics, got %d", tv.Name, mlen)
			continue
		}
		if len(metricData[0].Tags) != 1 {
			t.Errorf("%q: only expected 1 tag, got %d", tv.Name, len(metricData[0].Tags))
		}
		entry := metricData[0].Data
		sum, ok := entry.(*view.SumData)
		if !ok {
			t.Errorf("unexpected metric type: %T", entry)
		}
		got := sum.Value
		var want int64
		switch tv {
		case AppendRequestsView:
			want = int64(len(testSimpleData))
		case AppendResponsesView:
			want = int64(len(testSimpleData))
		case AppendClientOpenView:
			want = 1
		}

		// float comparison; diff more than error bound is error
		if math.Abs(got-float64(want)) > 0.1 {
			t.Errorf("%q: metric mismatch, got %f want %d", tv.Name, got, want)
		}
	}
}

func testSchemaEvolution(ctx context.Context, t *testing.T, mwClient *Client, bqClient *bigquery.Client, dataset *bigquery.Dataset) {
	testTable := dataset.Table(tableIDs.New())
	if err := testTable.Create(ctx, &bigquery.TableMetadata{Schema: testdata.SimpleMessageSchema}); err != nil {
		t.Fatalf("failed to create test table %s: %v", testTable.FullyQualifiedName(), err)
	}

	m := &testdata.SimpleMessageProto2{}
	descriptorProto := protodesc.ToDescriptorProto(m.ProtoReflect().Descriptor())

	// setup a new stream.
	ms, err := mwClient.NewManagedStream(ctx,
		WithDestinationTable(TableParentFromParts(testTable.ProjectID, testTable.DatasetID, testTable.TableID)),
		WithType(CommittedStream),
		WithSchemaDescriptor(descriptorProto),
	)
	if err != nil {
		t.Fatalf("NewManagedStream: %v", err)
	}
	validateTableConstraints(ctx, t, bqClient, testTable, "before send",
		withExactRowCount(0))

	var result *AppendResult
	var curOffset int64
	var latestRow []byte
	for k, mesg := range testSimpleData {
		b, err := proto.Marshal(mesg)
		if err != nil {
			t.Errorf("failed to marshal message %d: %v", k, err)
		}
		latestRow = b
		data := [][]byte{b}
		result, err = ms.AppendRows(ctx, data, WithOffset(curOffset))
		if err != nil {
			t.Errorf("single-row append %d failed: %v", k, err)
		}
		curOffset = curOffset + int64(len(data))
	}
	// Wait for the result to indicate ready, then validate.
	_, err = result.GetResult(ctx)
	if err != nil {
		t.Errorf("error on append: %v", err)
	}

	validateTableConstraints(ctx, t, bqClient, testTable, "after send",
		withExactRowCount(int64(len(testSimpleData))))

	// Now, evolve the underlying table schema.
	_, err = testTable.Update(ctx, bigquery.TableMetadataToUpdate{Schema: testdata.SimpleMessageEvolvedSchema}, "")
	if err != nil {
		t.Errorf("failed to evolve table schema: %v", err)
	}

	// Resend latest row until we get a new schema notification.
	// It _should_ be possible to send duplicates, but this currently will not propagate the schema error.
	// Internal issue: b/211899346
	for {
		resp, err := ms.AppendRows(ctx, [][]byte{latestRow}, WithOffset(curOffset))
		if err != nil {
			t.Errorf("got error on dupe append: %v", err)
			break
		}
		curOffset = curOffset + 1
		s, err := resp.UpdatedSchema(ctx)
		if err != nil {
			t.Errorf("getting schema error: %v", err)
			break
		}
		if s != nil {
			break
		}

	}

	// ready descriptor, send an additional append
	m2 := &testdata.SimpleMessageEvolvedProto2{
		Name:  proto.String("evolved"),
		Value: proto.Int64(180),
		Other: proto.String("hello evolution"),
	}
	descriptorProto = protodesc.ToDescriptorProto(m2.ProtoReflect().Descriptor())
	b, err := proto.Marshal(m2)
	if err != nil {
		t.Errorf("failed to marshal evolved message: %v", err)
	}
	// Try to force connection errors from concurrent appends.
	// We drop setting of offset to avoid commingling out-of-order append errors.
	var wg sync.WaitGroup
	for i := 0; i < 5; i++ {
		wg.Add(1)
		go func() {
			res, err := ms.AppendRows(ctx, [][]byte{b}, UpdateSchemaDescriptor(descriptorProto))
			if err != nil {
				t.Errorf("failed evolved append: %v", err)
			}
			_, err = res.GetResult(ctx)
			if err != nil {
				t.Errorf("error on evolved append: %v", err)
			}
			wg.Done()
		}()
	}
	wg.Wait()

	validateTableConstraints(ctx, t, bqClient, testTable, "after send",
		withExactRowCount(int64(curOffset+5)),
		withNullCount("name", 0),
		withNonNullCount("other", 5),
	)
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
		t.Errorf("expected error from bad token source, NewClient succeeded with project: %s", badClient.projectID)
	}
}

func TestIntegration_ProtoNormalization(t *testing.T) {
	mwClient, bqClient := getTestClients(context.Background(), t)
	defer mwClient.Close()
	defer bqClient.Close()

	dataset, cleanup, err := setupTestDataset(context.Background(), t, bqClient, "us-east1")
	if err != nil {
		t.Fatalf("failed to init test dataset: %v", err)
	}
	defer cleanup()

	ctx, cancel := context.WithTimeout(context.Background(), defaultTestTimeout)
	defer cancel()

	t.Run("group", func(t *testing.T) {
		t.Run("ComplexType", func(t *testing.T) {
			t.Parallel()
			schema := testdata.ComplexTypeSchema
			mesg := &testdata.ComplexType{
				NestedRepeatedType: []*testdata.NestedType{
					{
						InnerType: []*testdata.InnerType{
							{Value: []string{"a", "b", "c"}},
							{Value: []string{"x", "y", "z"}},
						},
					},
				},
				InnerType: &testdata.InnerType{
					Value: []string{"top"},
				},
			}
			b, err := proto.Marshal(mesg)
			if err != nil {
				t.Fatalf("proto.Marshal: %v", err)
			}
			descriptor := (mesg).ProtoReflect().Descriptor()
			testProtoNormalization(ctx, t, mwClient, bqClient, dataset, schema, descriptor, b)
		})
		t.Run("WithWellKnownTypes", func(t *testing.T) {
			t.Parallel()
			schema := testdata.WithWellKnownTypesSchema
			mesg := &testdata.WithWellKnownTypes{
				Int64Value: proto.Int64(123),
				WrappedInt64: &wrapperspb.Int64Value{
					Value: 456,
				},
				StringValue: []string{"a", "b"},
				WrappedString: []*wrapperspb.StringValue{
					{Value: "foo"},
					{Value: "bar"},
				},
			}
			b, err := proto.Marshal(mesg)
			if err != nil {
				t.Fatalf("proto.Marshal: %v", err)
			}
			descriptor := (mesg).ProtoReflect().Descriptor()
			testProtoNormalization(ctx, t, mwClient, bqClient, dataset, schema, descriptor, b)
		})
		t.Run("WithExternalEnum", func(t *testing.T) {
			t.Parallel()
			schema := testdata.ExternalEnumMessageSchema
			mesg := &testdata.ExternalEnumMessage{
				MsgA: &testdata.EnumMsgA{
					Foo: proto.String("foo"),
					Bar: testdata.ExtEnum_THING.Enum(),
				},
				MsgB: &testdata.EnumMsgB{
					Baz: testdata.ExtEnum_OTHER_THING.Enum(),
				},
			}
			b, err := proto.Marshal(mesg)
			if err != nil {
				t.Fatalf("proto.Marshal: %v", err)
			}
			descriptor := (mesg).ProtoReflect().Descriptor()
			testProtoNormalization(ctx, t, mwClient, bqClient, dataset, schema, descriptor, b)
		})
	})
}

func testProtoNormalization(ctx context.Context, t *testing.T, mwClient *Client, bqClient *bigquery.Client, dataset *bigquery.Dataset, schema bigquery.Schema, descriptor protoreflect.MessageDescriptor, sampleRow []byte) {
	testTable := dataset.Table(tableIDs.New())
	if err := testTable.Create(ctx, &bigquery.TableMetadata{Schema: schema}); err != nil {
		t.Fatalf("failed to create test table %q: %v", testTable.FullyQualifiedName(), err)
	}

	dp, err := adapt.NormalizeDescriptor(descriptor)
	if err != nil {
		t.Fatalf("NormalizeDescriptor: %v", err)
	}

	// setup a new stream.
	ms, err := mwClient.NewManagedStream(ctx,
		WithDestinationTable(TableParentFromParts(testTable.ProjectID, testTable.DatasetID, testTable.TableID)),
		WithType(DefaultStream),
		WithSchemaDescriptor(dp),
	)
	if err != nil {
		t.Fatalf("NewManagedStream: %v", err)
	}
	result, err := ms.AppendRows(ctx, [][]byte{sampleRow})
	if err != nil {
		t.Errorf("append failed: %v", err)
	}

	_, err = result.GetResult(ctx)
	if err != nil {
		t.Errorf("error in response: %v", err)
	}
}

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
	"cloud.google.com/go/bigquery/storage/apiv1/storagepb"
	"cloud.google.com/go/bigquery/storage/managedwriter/adapt"
	"cloud.google.com/go/bigquery/storage/managedwriter/testdata"
	"cloud.google.com/go/internal/testutil"
	"cloud.google.com/go/internal/uid"
	"github.com/google/go-cmp/cmp"
	"github.com/googleapis/gax-go/v2/apierror"
	"go.opencensus.io/stats/view"
	"google.golang.org/api/option"
	"google.golang.org/grpc/codes"
	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/reflect/protodesc"
	"google.golang.org/protobuf/reflect/protoreflect"
	"google.golang.org/protobuf/testing/protocmp"
	"google.golang.org/protobuf/types/descriptorpb"
	"google.golang.org/protobuf/types/dynamicpb"
	"google.golang.org/protobuf/types/known/wrapperspb"
)

var (
	datasetIDs         = uid.NewSpace("managedwriter_test_dataset", &uid.Options{Sep: '_', Time: time.Now()})
	tableIDs           = uid.NewSpace("table", &uid.Options{Sep: '_', Time: time.Now()})
	defaultTestTimeout = 90 * time.Second
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
	dp, err := adapt.NormalizeDescriptor(messageDescriptor)
	if err != nil {
		t.Fatalf("NormalizeDescriptor: %v", err)
	}
	return messageDescriptor, dp
}

func TestIntegration_ClientGetWriteStream(t *testing.T) {
	ctx := context.Background()
	mwClient, bqClient := getTestClients(ctx, t)
	defer mwClient.Close()
	defer bqClient.Close()

	wantLocation := "us-east1"
	dataset, cleanup, err := setupTestDataset(ctx, t, bqClient, wantLocation)
	if err != nil {
		t.Fatalf("failed to init test dataset: %v", err)
	}
	defer cleanup()

	testTable := dataset.Table(tableIDs.New())
	if err := testTable.Create(ctx, &bigquery.TableMetadata{Schema: testdata.SimpleMessageSchema}); err != nil {
		t.Fatalf("failed to create test table %q: %v", testTable.FullyQualifiedName(), err)
	}

	apiSchema, _ := adapt.BQSchemaToStorageTableSchema(testdata.SimpleMessageSchema)
	parent := TableParentFromParts(testTable.ProjectID, testTable.DatasetID, testTable.TableID)
	explicitStream, err := mwClient.CreateWriteStream(ctx, &storagepb.CreateWriteStreamRequest{
		Parent: parent,
		WriteStream: &storagepb.WriteStream{
			Type: storagepb.WriteStream_PENDING,
		},
	})
	if err != nil {
		t.Fatalf("CreateWriteStream: %v", err)
	}

	testCases := []struct {
		description string
		isDefault   bool
		streamID    string
		wantType    storagepb.WriteStream_Type
	}{
		{
			description: "default",
			isDefault:   true,
			streamID:    fmt.Sprintf("%s/streams/_default", parent),
			wantType:    storagepb.WriteStream_COMMITTED,
		},
		{
			description: "explicit pending",
			streamID:    explicitStream.Name,
			wantType:    storagepb.WriteStream_PENDING,
		},
	}

	for _, tc := range testCases {
		for _, fullView := range []bool{false, true} {
			info, err := mwClient.getWriteStream(ctx, tc.streamID, fullView)
			if err != nil {
				t.Errorf("%s (%T): getWriteStream failed: %v", tc.description, fullView, err)
			}
			if info.GetType() != tc.wantType {
				t.Errorf("%s (%T): got type %d, want type %d", tc.description, fullView, info.GetType(), tc.wantType)
			}
			if info.GetLocation() != wantLocation {
				t.Errorf("%s (%T) view: got location %s, want location %s", tc.description, fullView, info.GetLocation(), wantLocation)
			}
			if info.GetCommitTime() != nil {
				t.Errorf("%s (%T)expected empty commit time, got %v", tc.description, fullView, info.GetCommitTime())
			}

			if !tc.isDefault {
				if info.GetCreateTime() == nil {
					t.Errorf("%s (%T): expected create time, was empty", tc.description, fullView)
				}
			} else {
				if info.GetCreateTime() != nil {
					t.Errorf("%s (%T): expected empty time, got %v", tc.description, fullView, info.GetCreateTime())
				}
			}

			if !fullView {
				if info.GetTableSchema() != nil {
					t.Errorf("%s (%T) basic view: expected no schema, was populated", tc.description, fullView)
				}
			} else {
				if diff := cmp.Diff(info.GetTableSchema(), apiSchema, protocmp.Transform()); diff != "" {
					t.Errorf("%s (%T) schema mismatch: -got, +want:\n%s", tc.description, fullView, diff)
				}
			}
		}
	}
}

func TestIntegration_ManagedWriter(t *testing.T) {
	mwClient, bqClient := getTestClients(context.Background(), t)
	defer mwClient.Close()
	defer bqClient.Close()

	dataset, cleanup, err := setupTestDataset(context.Background(), t, bqClient, "asia-east1")
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
		t.Run("SimpleCDC", func(t *testing.T) {
			t.Parallel()
			testSimpleCDC(ctx, t, mwClient, bqClient, dataset)
		})
		t.Run("Instrumentation", func(t *testing.T) {
			// Don't run this in parallel, we only want to collect stats from this subtest.
			testInstrumentation(ctx, t, mwClient, bqClient, dataset)
		})
		t.Run("TestLargeInsertNoRetry", func(t *testing.T) {
			testLargeInsertNoRetry(ctx, t, mwClient, bqClient, dataset)
		})
		t.Run("TestLargeInsertWithRetry", func(t *testing.T) {
			testLargeInsertWithRetry(ctx, t, mwClient, bqClient, dataset)
		})
		t.Run("DefaultValueHandling", func(t *testing.T) {
			testDefaultValueHandling(ctx, t, mwClient, bqClient, dataset)
		})
	})
}

func TestIntegration_SchemaEvolution(t *testing.T) {

	testcases := []struct {
		desc       string
		clientOpts []option.ClientOption
		writerOpts []WriterOption
	}{
		{
			desc: "Simplex_Committed",
			writerOpts: []WriterOption{
				WithType(CommittedStream),
			},
		},
		{
			desc: "Simplex_Default",
			writerOpts: []WriterOption{
				WithType(DefaultStream),
			},
		},
		{
			desc: "Multiplex_Default",
			clientOpts: []option.ClientOption{
				WithMultiplexing(),
				WithMultiplexPoolLimit(2),
			},
			writerOpts: []WriterOption{
				WithType(DefaultStream),
			},
		},
	}

	for _, tc := range testcases {
		mwClient, bqClient := getTestClients(context.Background(), t, tc.clientOpts...)
		defer mwClient.Close()
		defer bqClient.Close()

		dataset, cleanup, err := setupTestDataset(context.Background(), t, bqClient, "asia-east1")
		if err != nil {
			t.Fatalf("failed to init test dataset: %v", err)
		}
		defer cleanup()

		ctx, cancel := context.WithTimeout(context.Background(), defaultTestTimeout)
		defer cancel()
		t.Run(tc.desc, func(t *testing.T) {
			testSchemaEvolution(ctx, t, mwClient, bqClient, dataset, tc.writerOpts...)
		})
	}
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
	if ms.id == "" {
		t.Errorf("managed stream is missing ID")
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
	if err := testTable.Create(ctx, &bigquery.TableMetadata{Schema: testdata.GithubArchiveSchema}); err != nil {
		t.Fatalf("failed to create test table %s: %v", testTable.FullyQualifiedName(), err)
	}

	md, descriptorProto := setupDynamicDescriptors(t, testdata.GithubArchiveSchema)

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
		[]byte(`{"type": "foo", "public": true, "repo": {"id": 99, "name": "repo_name_1", "url": "https://one.example.com"}}`),
		[]byte(`{"type": "bar", "public": false, "repo": {"id": 101, "name": "repo_name_2", "url": "https://two.example.com"}}`),
		[]byte(`{"type": "baz", "public": true, "repo": {"id": 456, "name": "repo_name_3", "url": "https://three.example.com"}}`),
		[]byte(`{"type": "wow", "public": false, "repo": {"id": 123, "name": "repo_name_4", "url": "https://four.example.com"}}`),
		[]byte(`{"type": "yay", "public": true, "repo": {"name": "repo_name_5", "url": "https://five.example.com"}}`),
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
		withDistinctValues("type", int64(len(sampleJSONData))),
		withDistinctValues("public", int64(2)))
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

	info, err := ms.c.getWriteStream(ctx, ms.streamSettings.streamID, false)
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
			t.Fatalf("failed to marshal message %d: %v", k, err)
		}
		data := [][]byte{b}
		results, err := ms.AppendRows(ctx, data)
		if err != nil {
			t.Fatalf("single-row append %d failed: %v", k, err)
		}
		// Wait for acknowledgement.
		offset, err := results.GetResult(ctx)
		if err != nil {
			t.Fatalf("got error from pending result %d: %v", k, err)
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

// testSimpleCDC demonstrates basic Change Data Capture (CDC) functionality.   We add an initial set of
// rows to a table, then use CDC to apply updates.
func testSimpleCDC(ctx context.Context, t *testing.T, mwClient *Client, bqClient *bigquery.Client, dataset *bigquery.Dataset) {
	testTable := dataset.Table(tableIDs.New())

	if err := testTable.Create(ctx, &bigquery.TableMetadata{
		Schema: testdata.ExampleEmployeeSchema,
		Clustering: &bigquery.Clustering{
			Fields: []string{"id"},
		},
	}); err != nil {
		t.Fatalf("failed to create test table %s: %v", testTable.FullyQualifiedName(), err)
	}

	// Mark the primary key using an ALTER TABLE DDL.
	tableIdentifier, _ := testTable.Identifier(bigquery.StandardSQLID)
	sql := fmt.Sprintf("ALTER TABLE %s ADD PRIMARY KEY(id) NOT ENFORCED;", tableIdentifier)
	if _, err := bqClient.Query(sql).Read(ctx); err != nil {
		t.Fatalf("failed ALTER TABLE: %v", err)
	}

	m := &testdata.ExampleEmployeeCDC{}
	descriptorProto, err := adapt.NormalizeDescriptor(m.ProtoReflect().Descriptor())
	if err != nil {
		t.Fatalf("NormalizeDescriptor: %v", err)
	}

	// Setup an initial writer for sending initial inserts.
	writer, err := mwClient.NewManagedStream(ctx,
		WithDestinationTable(TableParentFromParts(testTable.ProjectID, testTable.DatasetID, testTable.TableID)),
		WithType(CommittedStream),
		WithSchemaDescriptor(descriptorProto),
	)
	if err != nil {
		t.Fatalf("NewManagedStream: %v", err)
	}
	defer writer.Close()
	validateTableConstraints(ctx, t, bqClient, testTable, "before send",
		withExactRowCount(0))

	initialEmployees := []*testdata.ExampleEmployeeCDC{
		{
			Id:           proto.Int64(1),
			Username:     proto.String("alice"),
			GivenName:    proto.String("Alice CEO"),
			Departments:  []string{"product", "support", "internal"},
			Salary:       proto.Int64(1),
			XCHANGE_TYPE: proto.String("INSERT"),
		},
		{
			Id:           proto.Int64(2),
			Username:     proto.String("bob"),
			GivenName:    proto.String("Bob Bobberson"),
			Departments:  []string{"research"},
			Salary:       proto.Int64(100000),
			XCHANGE_TYPE: proto.String("INSERT"),
		},
		{
			Id:           proto.Int64(3),
			Username:     proto.String("clarice"),
			GivenName:    proto.String("Clarice Clearwater"),
			Departments:  []string{"product"},
			Salary:       proto.Int64(100001),
			XCHANGE_TYPE: proto.String("INSERT"),
		},
	}

	// First append inserts all the initial employees.
	data := make([][]byte, len(initialEmployees))
	for k, mesg := range initialEmployees {
		b, err := proto.Marshal(mesg)
		if err != nil {
			t.Fatalf("failed to marshal record %d: %v", k, err)
		}
		data[k] = b
	}
	result, err := writer.AppendRows(ctx, data)
	if err != nil {
		t.Errorf("initial insert failed (%s): %v", writer.StreamName(), err)
	}
	if _, err := result.GetResult(ctx); err != nil {
		t.Errorf("result error for initial insert (%s): %v", writer.StreamName(), err)
	}
	validateTableConstraints(ctx, t, bqClient, testTable, "initial inserts",
		withExactRowCount(int64(len(initialEmployees))))

	// Create a second writer for applying modifications.
	updateWriter, err := mwClient.NewManagedStream(ctx,
		WithDestinationTable(TableParentFromParts(testTable.ProjectID, testTable.DatasetID, testTable.TableID)),
		WithType(DefaultStream),
		WithSchemaDescriptor(descriptorProto),
	)
	if err != nil {
		t.Fatalf("NewManagedStream: %v", err)
	}
	defer updateWriter.Close()

	// Change bob via an UPSERT CDC
	newBob := proto.Clone(initialEmployees[1]).(*testdata.ExampleEmployeeCDC)
	newBob.Salary = proto.Int64(105000)
	newBob.Departments = []string{"research", "product"}
	newBob.XCHANGE_TYPE = proto.String("UPSERT")
	b, err := proto.Marshal(newBob)
	if err != nil {
		t.Fatalf("failed to marshal new bob: %v", err)
	}
	result, err = updateWriter.AppendRows(ctx, [][]byte{b})
	if err != nil {
		t.Fatalf("bob modification failed (%s): %v", updateWriter.StreamName(), err)
	}
	if _, err := result.GetResult(ctx); err != nil {
		t.Fatalf("result error for bob modification (%s): %v", updateWriter.StreamName(), err)
	}
	validateTableConstraints(ctx, t, bqClient, testTable, "after bob modification",
		withExactRowCount(int64(len(initialEmployees))),
		withDistinctValues("id", int64(len(initialEmployees))))

	// remote clarice via DELETE CDC
	removeClarice := &testdata.ExampleEmployeeCDC{
		Id:           proto.Int64(3),
		XCHANGE_TYPE: proto.String("DELETE"),
	}
	b, err = proto.Marshal(removeClarice)
	if err != nil {
		t.Fatalf("failed to marshal clarice removal: %v", err)
	}
	result, err = updateWriter.AppendRows(ctx, [][]byte{b})
	if err != nil {
		t.Fatalf("clarice removal failed (%s): %v", updateWriter.StreamName(), err)
	}
	if _, err := result.GetResult(ctx); err != nil {
		t.Fatalf("result error for clarice removal (%s): %v", updateWriter.StreamName(), err)
	}

	validateTableConstraints(ctx, t, bqClient, testTable, "after clarice removal",
		withExactRowCount(int64(len(initialEmployees))-1))
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

func testLargeInsertNoRetry(ctx context.Context, t *testing.T, mwClient *Client, bqClient *bigquery.Client, dataset *bigquery.Dataset) {
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
	// our next append is small and should succeed.
	result, err = ms.AppendRows(ctx, [][]byte{b})
	if err != nil {
		t.Fatalf("second append failed: %v", err)
	}
	_, err = result.GetResult(ctx)
	if err != nil {
		t.Errorf("failure result from second append: %v", err)
	}

	validateTableConstraints(ctx, t, bqClient, testTable, "final",
		withExactRowCount(1))
}

func testLargeInsertWithRetry(ctx context.Context, t *testing.T, mwClient *Client, bqClient *bigquery.Client, dataset *bigquery.Dataset) {
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
		EnableWriteRetries(true),
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

	// The second append will succeed.
	result, err = ms.AppendRows(ctx, [][]byte{b})
	if err != nil {
		t.Fatalf("second append expected to succeed, got error: %v", err)
	}
	_, err = result.GetResult(ctx)
	if err != nil {
		t.Errorf("failure result from second append: %v", err)
	}
	if attempts, _ := result.TotalAttempts(ctx); attempts != 1 {
		t.Errorf("expected 1 attempts, got %d", attempts)
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

	// metric to key tag names
	wantTags := map[string][]string{
		"cloud.google.com/go/bigquery/storage/managedwriter/stream_open_count":       {"error"},
		"cloud.google.com/go/bigquery/storage/managedwriter/stream_open_retry_count": nil,
		"cloud.google.com/go/bigquery/storage/managedwriter/append_requests":         {"streamID"},
		"cloud.google.com/go/bigquery/storage/managedwriter/append_request_bytes":    {"streamID"},
		"cloud.google.com/go/bigquery/storage/managedwriter/append_request_errors":   {"streamID"},
		"cloud.google.com/go/bigquery/storage/managedwriter/append_rows":             {"streamID"},
	}

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
		if wantKeys, ok := wantTags[tv.Name]; ok {
			if wantKeys == nil {
				if n := len(tv.TagKeys); n != 0 {
					t.Errorf("expected view %q to have no keys, but %d present", tv.Name, n)
				}
			} else {
				for _, wk := range wantKeys {
					var found bool
					for _, gk := range tv.TagKeys {
						if gk.Name() == wk {
							found = true
							break
						}
					}
					if !found {
						t.Errorf("expected view %q to have key %q, but wasn't present", tv.Name, wk)
					}
				}
			}
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

func testSchemaEvolution(ctx context.Context, t *testing.T, mwClient *Client, bqClient *bigquery.Client, dataset *bigquery.Dataset, opts ...WriterOption) {
	testTable := dataset.Table(tableIDs.New())
	if err := testTable.Create(ctx, &bigquery.TableMetadata{Schema: testdata.SimpleMessageSchema}); err != nil {
		t.Fatalf("failed to create test table %s: %v", testTable.FullyQualifiedName(), err)
	}

	m := &testdata.SimpleMessageProto2{}
	descriptorProto := protodesc.ToDescriptorProto(m.ProtoReflect().Descriptor())

	// setup a new stream.
	opts = append(opts, WithDestinationTable(TableParentFromParts(testTable.ProjectID, testTable.DatasetID, testTable.TableID)))
	opts = append(opts, WithSchemaDescriptor(descriptorProto))
	ms, err := mwClient.NewManagedStream(ctx, opts...)
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
		result, err = ms.AppendRows(ctx, data)
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
	//
	// The alternative here would be to block on GetWriteStream until we get a different write stream, but
	// this subjects us to a possible race, as the backend that services GetWriteStream isn't necessarily the
	// one in charge of the stream, and thus may report ready early.
	for {
		resp, err := ms.AppendRows(ctx, [][]byte{latestRow})
		if err != nil {
			t.Errorf("got error on dupe append: %v", err)
			break
		}
		curOffset = curOffset + 1
		s, err := resp.UpdatedSchema(ctx)
		if err != nil {
			t.Errorf("getting schema error: %v", err)
		}
		if s != nil {
			break
		}
	}

	// ready evolved message and descriptor
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
	// Send an append with an evolved schema
	res, err := ms.AppendRows(ctx, [][]byte{b}, UpdateSchemaDescriptor(descriptorProto))
	if err != nil {
		t.Errorf("failed evolved append: %v", err)
	}
	_, err = res.GetResult(ctx)
	if err != nil {
		t.Errorf("error on evolved append: %v", err)
	}
	curOffset = curOffset + 1

	// Try to force connection errors from concurrent appends.
	// We drop setting of offset to avoid commingling out-of-order append errors.
	var wg sync.WaitGroup
	for i := 0; i < 5; i++ {
		id := i
		wg.Add(1)
		go func() {
			res, err := ms.AppendRows(ctx, [][]byte{b})
			if err != nil {
				t.Errorf("failed concurrent append %d: %v", id, err)
			}
			_, err = res.GetResult(ctx)
			if err != nil {
				t.Errorf("error on concurrent append %d: %v", id, err)
			}
			wg.Done()
		}()
	}
	wg.Wait()

	validateTableConstraints(ctx, t, bqClient, testTable, "after evolved records send",
		withExactRowCount(int64(curOffset+5)),
		withNullCount("name", 0),
		withNonNullCount("other", 6),
	)
}

func testDefaultValueHandling(ctx context.Context, t *testing.T, mwClient *Client, bqClient *bigquery.Client, dataset *bigquery.Dataset, opts ...WriterOption) {
	testTable := dataset.Table(tableIDs.New())
	if err := testTable.Create(ctx, &bigquery.TableMetadata{Schema: testdata.DefaultValueSchema}); err != nil {
		t.Fatalf("failed to create test table %s: %v", testTable.FullyQualifiedName(), err)
	}

	m := &testdata.DefaultValuesPartialSchema{
		// We only populate the id, as remaining fields are used to test default values.
		Id: proto.String("someval"),
	}
	var data []byte
	var err error
	if data, err = proto.Marshal(m); err != nil {
		t.Fatalf("failed to marshal test row data")
	}
	descriptorProto := protodesc.ToDescriptorProto(m.ProtoReflect().Descriptor())

	// setup a new stream.
	opts = append(opts, WithDestinationTable(TableParentFromParts(testTable.ProjectID, testTable.DatasetID, testTable.TableID)))
	opts = append(opts, WithSchemaDescriptor(descriptorProto))
	ms, err := mwClient.NewManagedStream(ctx, opts...)
	if err != nil {
		t.Fatalf("NewManagedStream: %v", err)
	}
	validateTableConstraints(ctx, t, bqClient, testTable, "before send",
		withExactRowCount(0))

	var result *AppendResult

	// Send one row, verify default values were set as expected.

	result, err = ms.AppendRows(ctx, [][]byte{data})
	if err != nil {
		t.Errorf("append failed: %v", err)
	}
	// Wait for the result to indicate ready, then validate.
	_, err = result.GetResult(ctx)
	if err != nil {
		t.Errorf("error on append: %v", err)
	}

	validateTableConstraints(ctx, t, bqClient, testTable, "after first row",
		withExactRowCount(1),
		withNonNullCount("id", 1),
		withNullCount("strcol_withdef", 1),
		withNullCount("intcol_withdef", 1),
		withNullCount("otherstr_withdef", 0)) // not part of partial schema

	// Change default MVI to use nulls.
	// We expect the fields in the partial schema to leverage nulls rather than default values.
	// The fields outside the partial schema continue to obey default values.
	result, err = ms.AppendRows(ctx, [][]byte{data}, UpdateDefaultMissingValueInterpretation(storagepb.AppendRowsRequest_DEFAULT_VALUE))
	if err != nil {
		t.Errorf("append failed: %v", err)
	}
	// Wait for the result to indicate ready, then validate.
	_, err = result.GetResult(ctx)
	if err != nil {
		t.Errorf("error on append: %v", err)
	}

	validateTableConstraints(ctx, t, bqClient, testTable, "after second row (default mvi is DEFAULT_VALUE)",
		withExactRowCount(2),
		withNullCount("strcol_withdef", 1), // doesn't increment, as it gets default value
		withNullCount("intcol_withdef", 1)) // doesn't increment, as it gets default value

	// Change per-column MVI to use default value
	result, err = ms.AppendRows(ctx, [][]byte{data},
		UpdateMissingValueInterpretations(map[string]storagepb.AppendRowsRequest_MissingValueInterpretation{
			"strcol_withdef": storagepb.AppendRowsRequest_NULL_VALUE,
		}))
	if err != nil {
		t.Errorf("append failed: %v", err)
	}
	// Wait for the result to indicate ready, then validate.
	_, err = result.GetResult(ctx)
	if err != nil {
		t.Errorf("error on append: %v", err)
	}

	validateTableConstraints(ctx, t, bqClient, testTable, "after third row (explicit column mvi)",
		withExactRowCount(3),
		withNullCount("strcol_withdef", 2),      // increments as it's null for this column
		withNullCount("intcol_withdef", 1),      // doesn't increment, still default value
		withNonNullCount("otherstr_withdef", 3), // not part of descriptor, always gets default value
		withNullCount("otherstr", 3),            // not part of descriptor, always gets null
		withNullCount("strcol", 3),              // no default value defined, always gets null
		withNullCount("intcol", 3),              // no default value defined, always gets null
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

func TestIntegration_MultiplexWrites(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), defaultTestTimeout)
	defer cancel()
	mwClient, bqClient := getTestClients(ctx, t,
		WithMultiplexing(),
		WithMultiplexPoolLimit(2),
	)
	defer mwClient.Close()
	defer bqClient.Close()

	dataset, cleanup, err := setupTestDataset(ctx, t, bqClient, "us-east1")
	if err != nil {
		t.Fatalf("failed to init test dataset: %v", err)
	}
	defer cleanup()

	wantWrites := 10

	testTables := []struct {
		tbl         *bigquery.Table
		schema      bigquery.Schema
		dp          *descriptorpb.DescriptorProto
		sampleRow   []byte
		constraints []constraintOption
	}{
		{
			tbl:    dataset.Table(tableIDs.New()),
			schema: testdata.SimpleMessageSchema,
			dp: func() *descriptorpb.DescriptorProto {
				m := &testdata.SimpleMessageProto2{}
				dp, _ := adapt.NormalizeDescriptor(m.ProtoReflect().Descriptor())
				return dp
			}(),
			sampleRow: func() []byte {
				msg := &testdata.SimpleMessageProto2{
					Name:  proto.String("sample_name"),
					Value: proto.Int64(1001),
				}
				b, _ := proto.Marshal(msg)
				return b
			}(),
			constraints: []constraintOption{
				withExactRowCount(int64(wantWrites)),
				withStringValueCount("name", "sample_name", int64(wantWrites)),
			},
		},
		{
			tbl:    dataset.Table(tableIDs.New()),
			schema: testdata.ValidationBaseSchema,
			dp: func() *descriptorpb.DescriptorProto {
				m := &testdata.ValidationP2Optional{}
				dp, _ := adapt.NormalizeDescriptor(m.ProtoReflect().Descriptor())
				return dp
			}(),
			sampleRow: func() []byte {
				msg := &testdata.ValidationP2Optional{
					Int64Field:  proto.Int64(69),
					StringField: proto.String("validation_string"),
				}
				b, _ := proto.Marshal(msg)
				return b
			}(),
			constraints: []constraintOption{
				withExactRowCount(int64(wantWrites)),
				withStringValueCount("string_field", "validation_string", int64(wantWrites)),
			},
		},
		{
			tbl:    dataset.Table(tableIDs.New()),
			schema: testdata.GithubArchiveSchema,
			dp: func() *descriptorpb.DescriptorProto {
				m := &testdata.GithubArchiveMessageProto2{}
				dp, _ := adapt.NormalizeDescriptor(m.ProtoReflect().Descriptor())
				return dp
			}(),
			sampleRow: func() []byte {
				msg := &testdata.GithubArchiveMessageProto2{
					Payload: proto.String("payload_string"),
					Id:      proto.String("some_id"),
				}
				b, _ := proto.Marshal(msg)
				return b
			}(),
			constraints: []constraintOption{
				withExactRowCount(int64(wantWrites)),
				withStringValueCount("payload", "payload_string", int64(wantWrites)),
			},
		},
	}

	// setup tables
	for _, testTable := range testTables {
		if err := testTable.tbl.Create(ctx, &bigquery.TableMetadata{Schema: testTable.schema}); err != nil {
			t.Fatalf("failed to create test table %q: %v", testTable.tbl.FullyQualifiedName(), err)
		}
	}

	var gotFirstPool *connectionPool
	var results []*AppendResult
	for i := 0; i < wantWrites; i++ {
		for k, testTable := range testTables {
			// create a writer and send a single append
			ms, err := mwClient.NewManagedStream(ctx,
				WithDestinationTable(TableParentFromParts(testTable.tbl.ProjectID, testTable.tbl.DatasetID, testTable.tbl.TableID)),
				WithType(DefaultStream),
				WithSchemaDescriptor(testTable.dp),
				EnableWriteRetries(true),
			)
			if err != nil {
				t.Fatalf("NewManagedStream %d: %v", k, err)
			}
			if i == 0 && k == 0 {
				if ms.pool == nil {
					t.Errorf("expected a non-nil pool reference for first writer")
				}
				gotFirstPool = ms.pool
			} else {
				if ms.pool != gotFirstPool {
					t.Errorf("expected same pool reference, got a different pool")
				}
			}
			defer ms.Close() // we won't clean these up until the end of the test, rather than per use.
			if err != nil {
				t.Fatalf("failed to create ManagedStream for table %d on iteration %d: %v", k, i, err)
			}
			res, err := ms.AppendRows(ctx, [][]byte{testTable.sampleRow})
			if err != nil {
				t.Fatalf("failed to append to table %d on iteration %d: %v", k, i, err)
			}
			results = append(results, res)
		}
	}

	// drain results
	for k, res := range results {
		if _, err := res.GetResult(ctx); err != nil {
			t.Errorf("result %d yielded error: %v", k, err)
		}
	}

	// validate the tables
	for _, testTable := range testTables {
		validateTableConstraints(ctx, t, bqClient, testTable.tbl, "", testTable.constraints...)
	}

}

func TestIntegration_MingledContexts(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), defaultTestTimeout)
	defer cancel()
	mwClient, bqClient := getTestClients(ctx, t,
		WithMultiplexing(),
		WithMultiplexPoolLimit(2),
	)
	defer mwClient.Close()
	defer bqClient.Close()

	wantLocation := "us-east4"

	dataset, cleanup, err := setupTestDataset(ctx, t, bqClient, wantLocation)
	if err != nil {
		t.Fatalf("failed to init test dataset: %v", err)
	}
	defer cleanup()

	testTable := dataset.Table(tableIDs.New())
	if err := testTable.Create(ctx, &bigquery.TableMetadata{Schema: testdata.SimpleMessageSchema}); err != nil {
		t.Fatalf("failed to create test table %s: %v", testTable.FullyQualifiedName(), err)
	}

	m := &testdata.SimpleMessageProto2{}
	descriptorProto := protodesc.ToDescriptorProto(m.ProtoReflect().Descriptor())

	numWriters := 4
	contexts := make([]context.Context, numWriters)
	cancels := make([]context.CancelFunc, numWriters)
	writers := make([]*ManagedStream, numWriters)
	for i := 0; i < numWriters; i++ {
		contexts[i], cancels[i] = context.WithCancel(ctx)
		ms, err := mwClient.NewManagedStream(contexts[i],
			WithDestinationTable(TableParentFromParts(testTable.ProjectID, testTable.DatasetID, testTable.TableID)),
			WithType(DefaultStream),
			WithSchemaDescriptor(descriptorProto),
		)
		if err != nil {
			t.Fatalf("instantating writer %d failed: %v", i, err)
		}
		writers[i] = ms
	}

	sampleRow, err := proto.Marshal(&testdata.SimpleMessageProto2{
		Name:  proto.String("datafield"),
		Value: proto.Int64(1234),
	})
	if err != nil {
		t.Fatalf("failed to generate sample row")
	}

	for i := 0; i < numWriters; i++ {
		res, err := writers[i].AppendRows(contexts[i], [][]byte{sampleRow})
		if err != nil {
			t.Errorf("initial write on %d failed: %v", i, err)
		} else {
			if _, err := res.GetResult(contexts[i]); err != nil {
				t.Errorf("GetResult initial write %d: %v", i, err)
			}
		}
	}

	// cancel the first context
	cancels[0]()
	// repeat writes on all other writers with the second context
	for i := 1; i < numWriters; i++ {
		res, err := writers[i].AppendRows(contexts[i], [][]byte{sampleRow})
		if err != nil {
			t.Errorf("second write on %d failed: %v", i, err)
		} else {
			if _, err := res.GetResult(contexts[1]); err != nil {
				t.Errorf("GetResult err on second write %d: %v", i, err)
			}
		}
	}

	// check that writes to the first writer should fail, even with a valid request context.
	if _, err := writers[0].AppendRows(contexts[1], [][]byte{sampleRow}); err == nil {
		t.Errorf("write succeeded on first writer when it should have failed")
	}

	// cancel the second context as well, ensure writer created with good context and bad request context fails
	cancels[1]()
	if _, err := writers[2].AppendRows(contexts[1], [][]byte{sampleRow}); err == nil {
		t.Errorf("write succeeded on third writer with a bad request context")
	}

	// repeat writes on remaining good writers/contexts
	for i := 2; i < numWriters; i++ {
		res, err := writers[i].AppendRows(contexts[i], [][]byte{sampleRow})
		if err != nil {
			t.Errorf("second write on %d failed: %v", i, err)
		} else {
			if _, err := res.GetResult(contexts[i]); err != nil {
				t.Errorf("GetResult err on second write %d: %v", i, err)
			}
		}
	}
}

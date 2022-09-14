// Copyright 2022 Google LLC
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
	"testing"
	"time"

	"cloud.google.com/go/bigquery"
	"cloud.google.com/go/bigquery/storage/managedwriter/adapt"
	"cloud.google.com/go/bigquery/storage/managedwriter/testdata"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/descriptorpb"
)

var (
	benchmarkTimeout = 2 * time.Minute
	benchLocation    = "us-east1"
)

func benchmarkAppend(location string, schema bigquery.Schema, dp *descriptorpb.DescriptorProto, serializedRows [][]byte, b *testing.B) {
	mwClient, bqClient := getTestClients(context.Background(), b)
	defer mwClient.Close()
	defer bqClient.Close()

	dataset, cleanup, err := setupTestDataset(context.Background(), b, bqClient, location)
	if err != nil {
		b.Fatalf("failed to init test dataset: %v", err)
	}
	defer cleanup()

	ctx, cancel := context.WithTimeout(context.Background(), benchmarkTimeout)
	defer cancel()

	testTable := dataset.Table(tableIDs.New())
	if err := testTable.Create(ctx, &bigquery.TableMetadata{Schema: schema}); err != nil {
		b.Fatalf("failed to create test table %q: %v", testTable.FullyQualifiedName(), err)
	}

	// setup default stream.
	ms, err := mwClient.NewManagedStream(ctx,
		WithDestinationTable(TableParentFromParts(testTable.ProjectID, testTable.DatasetID, testTable.TableID)),
		WithType(DefaultStream),
		WithSchemaDescriptor(dp),
	)
	if err != nil {
		b.Fatalf("NewManagedStream: %v", err)
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, err := ms.AppendRows(ctx, serializedRows)
		if err != nil {
			b.Errorf("append failed: %v", err)
		}
	}
}

func BenchmarkAppend_SingleSimpleData(b *testing.B) {

	// prepare test append payload
	by, err := proto.Marshal(testSimpleData[0])
	if err != nil {
		b.Fatalf("failed to marshall proto data: %v", err)
	}
	rowData := [][]byte{by}

	// prepare schema
	desc, err := adapt.NormalizeDescriptor(testSimpleData[0].ProtoReflect().Descriptor())
	if err != nil {
		b.Fatalf("NormalizeDescriptor: %v", err)
	}
	benchmarkAppend(benchLocation, testdata.SimpleMessageSchema, desc, rowData, b)
}

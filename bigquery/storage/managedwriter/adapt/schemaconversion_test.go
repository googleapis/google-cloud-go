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

package adapt

import (
	"testing"

	"cloud.google.com/go/bigquery"
	"cloud.google.com/go/bigquery/storage/apiv1/storagepb"
	"cloud.google.com/go/internal/testutil"
	"github.com/google/go-cmp/cmp"
	"google.golang.org/protobuf/testing/protocmp"
)

func TestFieldConversions(t *testing.T) {
	testCases := []struct {
		desc  string
		bq    *bigquery.FieldSchema
		proto *storagepb.TableFieldSchema
	}{
		{
			desc:  "nil",
			bq:    nil,
			proto: nil,
		},
		{
			desc: "string field",
			bq: &bigquery.FieldSchema{
				Name:        "name",
				Type:        bigquery.StringFieldType,
				Description: "description",
			},
			proto: &storagepb.TableFieldSchema{
				Name:        "name",
				Type:        storagepb.TableFieldSchema_STRING,
				Description: "description",
				Mode:        storagepb.TableFieldSchema_NULLABLE,
			},
		},
		{
			desc: "required integer field",
			bq: &bigquery.FieldSchema{
				Name:        "name",
				Type:        bigquery.IntegerFieldType,
				Description: "description",
				Required:    true,
			},
			proto: &storagepb.TableFieldSchema{
				Name:        "name",
				Type:        storagepb.TableFieldSchema_INT64,
				Description: "description",
				Mode:        storagepb.TableFieldSchema_REQUIRED,
			},
		},
		{
			desc: "struct with repeated bytes subfield",
			bq: &bigquery.FieldSchema{
				Name:        "name",
				Type:        bigquery.RecordFieldType,
				Description: "description",
				Required:    true,
				Schema: bigquery.Schema{
					&bigquery.FieldSchema{
						Name:        "inner1",
						Repeated:    true,
						Description: "repeat",
						Type:        bigquery.BytesFieldType,
					},
				},
			},
			proto: &storagepb.TableFieldSchema{
				Name:        "name",
				Type:        storagepb.TableFieldSchema_STRUCT,
				Description: "description",
				Mode:        storagepb.TableFieldSchema_REQUIRED,
				Fields: []*storagepb.TableFieldSchema{
					{
						Name:        "inner1",
						Mode:        storagepb.TableFieldSchema_REPEATED,
						Description: "repeat",
						Type:        storagepb.TableFieldSchema_BYTES,
					},
				},
			},
		},
	}

	for _, tc := range testCases {
		// first, bq to proto
		converted, err := bqFieldToProto(tc.bq)
		if err != nil {
			t.Errorf("case (%s) failed conversion from bq: %v", tc.desc, err)
		}
		if diff := cmp.Diff(converted, tc.proto, protocmp.Transform()); diff != "" {
			t.Errorf("conversion to proto diff (%s):\n%v", tc.desc, diff)
		}
		// reverse conversion, proto to bq
		reverse, err := protoToBQField(tc.proto)
		if err != nil {
			t.Errorf("case (%s) failed conversion from proto: %v", tc.desc, err)
		}
		if diff := cmp.Diff(reverse, tc.bq); diff != "" {
			t.Errorf("conversion to BQ diff (%s):\n%v", tc.desc, diff)
		}
	}
}

func TestSchemaConversion(t *testing.T) {

	testCases := []struct {
		description   string
		bqSchema      bigquery.Schema
		storageSchema *storagepb.TableSchema
	}{
		{
			description:   "nil",
			bqSchema:      nil,
			storageSchema: nil,
		},
		{
			description: "scalars",
			bqSchema: bigquery.Schema{
				{Name: "f1", Type: bigquery.StringFieldType},
				{Name: "f2", Type: bigquery.IntegerFieldType},
				{Name: "f3", Type: bigquery.BooleanFieldType},
			},
			storageSchema: &storagepb.TableSchema{
				Fields: []*storagepb.TableFieldSchema{
					{Name: "f1", Type: storagepb.TableFieldSchema_STRING, Mode: storagepb.TableFieldSchema_NULLABLE},
					{Name: "f2", Type: storagepb.TableFieldSchema_INT64, Mode: storagepb.TableFieldSchema_NULLABLE},
					{Name: "f3", Type: storagepb.TableFieldSchema_BOOL, Mode: storagepb.TableFieldSchema_NULLABLE},
				},
			},
		},
		{
			description: "array",
			bqSchema: bigquery.Schema{
				{Name: "arr", Type: bigquery.NumericFieldType, Repeated: true},
				{Name: "big", Type: bigquery.BigNumericFieldType, Required: true},
			},
			storageSchema: &storagepb.TableSchema{
				Fields: []*storagepb.TableFieldSchema{
					{Name: "arr", Type: storagepb.TableFieldSchema_NUMERIC, Mode: storagepb.TableFieldSchema_REPEATED},
					{Name: "big", Type: storagepb.TableFieldSchema_BIGNUMERIC, Mode: storagepb.TableFieldSchema_REQUIRED},
				},
			},
		},
		{
			description: "nested",
			bqSchema: bigquery.Schema{
				{Name: "struct1", Type: bigquery.RecordFieldType, Schema: []*bigquery.FieldSchema{
					{Name: "leaf1", Type: bigquery.DateFieldType},
					{Name: "leaf2", Type: bigquery.DateTimeFieldType},
				}},
				{Name: "field2", Type: bigquery.StringFieldType},
			},
			storageSchema: &storagepb.TableSchema{
				Fields: []*storagepb.TableFieldSchema{
					{Name: "struct1",
						Type: storagepb.TableFieldSchema_STRUCT,
						Mode: storagepb.TableFieldSchema_NULLABLE,
						Fields: []*storagepb.TableFieldSchema{
							{Name: "leaf1", Type: storagepb.TableFieldSchema_DATE, Mode: storagepb.TableFieldSchema_NULLABLE},
							{Name: "leaf2", Type: storagepb.TableFieldSchema_DATETIME, Mode: storagepb.TableFieldSchema_NULLABLE},
						}},
					{Name: "field2", Type: storagepb.TableFieldSchema_STRING, Mode: storagepb.TableFieldSchema_NULLABLE},
				},
			},
		},
	}
	for _, tc := range testCases {

		// BQ -> Storage
		storageS, err := BQSchemaToStorageTableSchema(tc.bqSchema)
		if err != nil {
			t.Errorf("BQSchemaToStorageTableSchema(%s): %v", tc.description, err)
		}
		if diff := testutil.Diff(storageS, tc.storageSchema); diff != "" {
			t.Fatalf("BQSchemaToStorageTableSchema(%s): -got, +want:\n%s", tc.description, diff)
		}

		// Storage -> BQ
		bqS, err := StorageTableSchemaToBQSchema(tc.storageSchema)
		if err != nil {
			t.Errorf("StorageTableSchemaToBQSchema(%s): %v", tc.description, err)
		}
		if diff := testutil.Diff(bqS, tc.bqSchema); diff != "" {
			t.Fatalf("StorageTableSchemaToBQSchema(%s): -got, +want:\n%s", tc.description, diff)
		}
	}
}

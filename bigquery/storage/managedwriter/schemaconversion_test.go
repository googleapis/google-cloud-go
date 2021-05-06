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
	"testing"

	"cloud.google.com/go/bigquery"
	"github.com/google/go-cmp/cmp"
	storagepb "google.golang.org/genproto/googleapis/cloud/bigquery/storage/v1beta2"
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

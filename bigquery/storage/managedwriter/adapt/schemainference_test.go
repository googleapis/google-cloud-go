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

package adapt

import (
	"testing"

	"cloud.google.com/go/bigquery/storage/managedwriter/testdata"
	"github.com/google/go-cmp/cmp"
	storagepb "google.golang.org/genproto/googleapis/cloud/bigquery/storage/v1"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/testing/protocmp"
)

func TestInferSchema(t *testing.T) {
	testCases := []struct {
		desc       string
		proto      proto.Message
		opts       []InferOption
		wantSchema *storagepb.TableSchema
		wantErr    bool
	}{
		{
			desc:    "nil",
			proto:   nil,
			wantErr: true,
		},
		{
			desc:  "SimpleMessageProto2 default",
			proto: &testdata.SimpleMessageProto2{},
			wantSchema: &storagepb.TableSchema{
				Fields: []*storagepb.TableFieldSchema{
					{Name: "name",
						Mode: storagepb.TableFieldSchema_NULLABLE,
						Type: storagepb.TableFieldSchema_STRING,
					},
					{Name: "value",
						Mode: storagepb.TableFieldSchema_NULLABLE,
						Type: storagepb.TableFieldSchema_INT64,
					},
				},
			},
		},
		{
			desc:    "SimpleMessageProto3 default",
			proto:   &testdata.SimpleMessageProto3{},
			wantErr: true,
		},
		{
			desc:  "SimpleMessageProto3 w/wrappers",
			proto: &testdata.SimpleMessageProto3{},
			opts:  []InferOption{AllowWrapperTypes(true)},
			wantSchema: &storagepb.TableSchema{
				Fields: []*storagepb.TableFieldSchema{
					{Name: "name",
						Mode: storagepb.TableFieldSchema_NULLABLE,
						Type: storagepb.TableFieldSchema_STRING,
					},
					{Name: "value",
						Mode: storagepb.TableFieldSchema_NULLABLE,
						Type: storagepb.TableFieldSchema_INT64,
					},
				},
			},
		},
		{
			desc:    "GithubArchiveMessageProto2",
			proto:   &testdata.GithubArchiveMessageProto2{},
			wantErr: true, // temporary until we do nested message parsing
		},
	}

	for _, tc := range testCases {

		gotSchema, err := InferSchemaFromProtoMessage(tc.proto, tc.opts...)
		if err != nil {
			if !tc.wantErr {
				t.Errorf("case %s failed: %v", tc.desc, err)
			}
			continue
		}
		if err == nil && tc.wantErr {
			t.Errorf("case %s expected error, was success", tc.desc)
			continue
		}
		if diff := cmp.Diff(gotSchema, tc.wantSchema, protocmp.Transform()); diff != "" {
			t.Errorf("conversion to proto diff (%s):\n%v", tc.desc, diff)
		}
	}
}

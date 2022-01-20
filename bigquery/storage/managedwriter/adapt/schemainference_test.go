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

func TestSchemaInference(t *testing.T) {
	testCases := []struct {
		desc       string
		proto      proto.Message
		wantSchema *storagepb.TableSchema
		wantErr    bool
	}{
		{
			desc:    "nil",
			proto:   nil,
			wantErr: true,
		},
		{
			desc:       "nil",
			proto:      &testdata.SimpleMessageProto2{},
			wantSchema: nil,
		},
	}

	for _, tc := range testCases {

		gotSchema, err := InferSchemaFromProtoMessage(tc.proto)
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
		if diff := cmp.Diff(gotSchema, tc.proto, protocmp.Transform()); diff != "" {
			t.Errorf("conversion to proto diff (%s):\n%v", tc.desc, diff)
		}
	}
}

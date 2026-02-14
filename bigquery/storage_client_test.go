// Copyright 2026 Google LLC
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

package bigquery

import (
	"context"
	"testing"

	"cloud.google.com/go/bigquery/storage/apiv1/storagepb"
	gax "github.com/googleapis/gax-go/v2"
)

func TestStorageReadSessionCreation(t *testing.T) {
	ctx := context.Background()

	readClient, err := newReadClient(ctx, "test-project")
	if err != nil {
		t.Fatalf("newReadClient() failed: %v", err)
	}
	defer readClient.close()

	projectID := "test-project"
	table := &Table{ProjectID: projectID, DatasetID: "test-dataset", TableID: "test-table"}
	rs, err := readClient.sessionForTable(ctx, table, projectID, false)
	if err != nil {
		t.Fatalf("sessionForTable() failed: %v", err)
	}

	receivedReq := &storagepb.CreateReadSessionRequest{}
	receivedCallOpts := []gax.CallOption{}
	rs.createReadSessionFunc = func(ctx context.Context, req *storagepb.CreateReadSessionRequest, opts ...gax.CallOption) (*storagepb.ReadSession, error) {
		receivedReq = req
		receivedCallOpts = opts
		return &storagepb.ReadSession{}, nil
	}

	err = rs.start()
	if err != nil {
		t.Fatalf("readSession.start() failed: %v", err)
	}

	if receivedReq.GetParent() != "projects/test-project" {
		t.Errorf("expected CreateReadSessionRequest.Parent = %q, want %q", receivedReq.Parent, "projects/test-project")
	}

	session := receivedReq.GetReadSession()
	if session.GetTable() != "projects/test-project/datasets/test-dataset/tables/test-table" {
		t.Errorf("expected ReadSession.Table = %q, want %q", session.Table, "projects/test-project/datasets/test-dataset/tables/test-table")
	}

	if session.GetDataFormat() != storagepb.DataFormat_ARROW {
		t.Errorf("expected ReadSession.DataFormat = %v, want %v", session.DataFormat, storagepb.DataFormat_ARROW)
	}

	settings := gax.CallSettings{}
	for _, opt := range receivedCallOpts {
		opt.Resolve(&settings)
	}
	if len(settings.GRPC) == 0 {
		// TODO: how to check that MaxCallRecvMsgSize = 128MB
		t.Errorf("expected GRPC options to override MaxCallRecvMsgSize, got none")
	}

	readOptions := session.GetReadOptions()
	arrowSerializationOptions := readOptions.GetArrowSerializationOptions()
	if arrowSerializationOptions == nil {
		t.Errorf("expected ReadSession.ArrowSerializationOptions != nil")
	}
	if arrowSerializationOptions.GetBufferCompression() != storagepb.ArrowSerializationOptions_ZSTD {
		t.Errorf("expected ReadSession.ArrowSerializationOptions.BufferCompression = %v, want %v", arrowSerializationOptions.GetBufferCompression(), storagepb.ArrowSerializationOptions_ZSTD)
	}
}

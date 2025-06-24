// Copyright 2025 Google LLC
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

package smoketests

import (
	"context"
	"fmt"
	"net/http"
	"testing"
	"time"

	"cloud.google.com/go/bigquery/v2/apiv2/bigquerypb"
	"github.com/googleapis/gax-go/v2/apierror"
	"github.com/googleapis/gax-go/v2/callctx"
	"google.golang.org/grpc/codes"
	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/types/known/wrapperspb"
)

func TestDatasetLifeCycle(t *testing.T) {
	if len(testClients) == 0 {
		t.Skip("integration tests skipped")
	}
	for k, client := range testClients {
		t.Run(k, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), defaultTestTimeout)
			defer cancel()
			dsRef := &bigquerypb.DatasetReference{
				ProjectId: testProjectID,
				DatasetId: fmt.Sprintf("testdataset_%s_%d", k, time.Now().UnixNano()),
			}
			req := &bigquerypb.InsertDatasetRequest{
				ProjectId: testProjectID,
				Dataset: &bigquerypb.Dataset{
					DatasetReference: dsRef,
					Location:         "US",
					FriendlyName:     &wrapperspb.StringValue{Value: "test dataset for apiv2 smoketests"},
					DefaultTableExpirationMs: &wrapperspb.Int64Value{
						Value: 10 * 86400 * 1000, // 10 days
					},
				},
			}
			// Insert the dataset
			ds, err := client.InsertDataset(ctx, req)
			if err != nil {
				t.Errorf("InsertDataset: %v", err)
			}

			// Now, update the dataset
			updateReq := &bigquerypb.UpdateOrPatchDatasetRequest{
				ProjectId: testProjectID,
				DatasetId: dsRef.GetDatasetId(),
				Dataset: &bigquerypb.Dataset{
					FriendlyName:             &wrapperspb.StringValue{Value: "updated friendly name"},
					DefaultTableExpirationMs: &wrapperspb.Int64Value{},
				},
			}

			// Use Etag preconditions to validate concurrent update behavior.
			badCtx := callctx.SetHeaders(ctx, "if-match", "badetagvalue")
			goodCtx := callctx.SetHeaders(ctx, "if-match", ds.GetEtag())

			// First, send a patch with a bad concurrent-update etag
			updateResp, err := client.PatchDataset(badCtx, updateReq)
			if err == nil {
				t.Errorf("expected Patch failure, but succeeded: %s", protojson.Format(updateResp))
			} else {
				if apiErr, ok := err.(*apierror.APIError); ok {
					if httpCode := apiErr.HTTPCode(); httpCode != -1 {
						// HTTP transport path
						if httpCode != http.StatusPreconditionFailed {
							t.Errorf("expected HTTP precondition failure code, got %d", httpCode)
						}
					} else {
						// GRPC transport path
						if gotCode := apiErr.GRPCStatus().Code(); gotCode != codes.FailedPrecondition {
							t.Errorf("expected FailedPrecondition, got %q", gotCode)
						}
					}
				} else {
					t.Errorf("unknown patch error: %v", err)
				}
			}

			// Send update with a matching etag
			_, err = client.PatchDataset(goodCtx, updateReq)
			if err != nil {
				t.Errorf("patch failed: %v", err)
			}

			delReq := &bigquerypb.DeleteDatasetRequest{
				ProjectId:      testProjectID,
				DatasetId:      dsRef.GetDatasetId(),
				DeleteContents: true,
			}

			if err := client.DeleteDataset(ctx, delReq); err != nil {
				t.Errorf("DeleteDataset: %v", err)
			}
		})
	}

}

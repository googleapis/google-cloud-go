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
	"testing"

	"cloud.google.com/go/bigquery/v2/apiv2/bigquerypb"
	"google.golang.org/protobuf/types/known/wrapperspb"
)

func TestInitStatelessQuery(t *testing.T) {
	if len(testClients) == 0 {
		t.Skip("integration tests skipped")
	}
	for k, client := range testClients {
		t.Run(k, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), defaultTestTimeout)
			defer cancel()

			req := &bigquerypb.PostQueryRequest{
				ProjectId: testProjectID,
				QueryRequest: &bigquerypb.QueryRequest{
					Query:           "SELECT CURRENT_TIMESTAMP() as foo, SESSION_USER() as bar",
					JobCreationMode: bigquerypb.QueryRequest_JOB_CREATION_OPTIONAL,
					UseLegacySql:    &wrapperspb.BoolValue{Value: false},
					FormatOptions: &bigquerypb.DataFormatOptions{
						UseInt64Timestamp: true,
					},
				},
			}

			queryResp, err := client.Query(ctx, req)
			if err != nil {
				t.Fatalf("Query() error: %v", err)
			}
			// Make some assertions if the job finished after the first poll.
			// This _should_ be the case, but the contract doesn't allow us to
			// assert that it must be the case.
			if bv := queryResp.GetJobComplete(); bv != nil && bv.Value {
				if jobRef := queryResp.GetJobReference(); jobRef != nil {
					// We got a job reference, at least make sure it has the expected fields populated.
					if jobRef.GetProjectId() == "" {
						t.Errorf("JobReference present but ProjectId not populated.")
					}
					if jobRef.GetJobId() == "" {
						t.Errorf("JobReference present but JobId not populated.")
					}
				} else {
					if rowcount := len(queryResp.GetRows()); rowcount != 1 {
						t.Errorf("expected one row of data, got %d", rowcount)
					}
				}
			}
		})
	}
}

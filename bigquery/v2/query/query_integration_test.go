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

package query

import (
	"context"
	"testing"
	"time"

	"cloud.google.com/go/bigquery/v2/apiv2/bigquerypb"
	"google.golang.org/protobuf/types/known/wrapperspb"
)

func TestRunQuery(t *testing.T) {
	if len(testClients) == 0 {
		t.Skip("integration tests skipped")
	}
	for k, client := range testClients {
		t.Run(k, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), defaultTestTimeout)
			defer cancel()

			req := client.FromSQL("SELECT CURRENT_TIMESTAMP() as foo, SESSION_USER() as bar")
			req.QueryRequest.JobCreationMode = bigquerypb.QueryRequest_JOB_CREATION_OPTIONAL

			q, err := client.StartQuery(ctx, req)
			if err != nil {
				t.Fatalf("Run() error: %v", err)
			}
			err = q.Wait()
			if err != nil {
				t.Fatalf("Wait() error: %v", err)
			}

			if !q.Complete() {
				t.Fatalf("expected job to be complete")
			}

			it, err := q.Read(ctx)
			if err != nil {
				t.Fatalf("Read() error: %v", err)
			}

			assertRowCount(t, it, 1)
		})
	}
}

func TestCancelWaitQuery(t *testing.T) {
	if len(testClients) == 0 {
		t.Skip("integration tests skipped")
	}
	for k, client := range testClients {
		t.Run(k, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), defaultTestTimeout)
			defer cancel()

			req := client.FromSQL("SELECT num FROM UNNEST(GENERATE_ARRAY(1,1000000)) as num")
			req.QueryRequest.UseQueryCache = wrapperspb.Bool(false)
			req.QueryRequest.JobCreationMode = bigquerypb.QueryRequest_JOB_CREATION_REQUIRED
			req.QueryRequest.TimeoutMs = wrapperspb.UInt32(500)

			wctx, wcancel := context.WithCancel(ctx)
			q, err := client.StartQuery(wctx, req)
			if err != nil {
				t.Fatalf("Run() error: %v", err)
			}

			go func(t *testing.T) {
				err = q.Wait()
				if err == nil {
					t.Logf("Wait() should throw an error: %v", err)
				}
			}(t)

			time.Sleep(1 * time.Second)
			wcancel()

			res, err := q.Cancel(ctx)
			if err != nil {
				t.Fatalf("Cancel() error: %v", err)
			}

			t.Logf("job cancelled(%s): %v", res.Kind, res.Job.Status)

			// Re-attache and see if it was cancelled
			q, err = client.AttachJob(ctx, q.JobReference())
			if err != nil {
				t.Fatalf("AttachJob() error: %v", err)
			}

			err = q.Wait()
			if err != nil {
				t.Logf("Wait() error: %v", err)
			}
		})
	}
}

func TestReadQueryJob(t *testing.T) {
	if len(testClients) == 0 {
		t.Skip("integration tests skipped")
	}
	for k, client := range testClients {
		t.Run(k, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), defaultTestTimeout)
			defer cancel()

			req := client.FromSQL("SELECT CURRENT_TIMESTAMP() as foo, SESSION_USER() as bar")
			req.QueryRequest.JobCreationMode = bigquerypb.QueryRequest_JOB_CREATION_REQUIRED

			q, err := client.StartQuery(ctx, req)
			if err != nil {
				t.Fatalf("Run() error: %v", err)
			}
			err = q.Wait()
			if err != nil {
				t.Fatalf("Wait() error: %v", err)
			}

			if !q.Complete() {
				t.Fatalf("expected job to be complete")
			}

			jobRef := q.JobReference()
			q, err = client.AttachJob(ctx, jobRef)
			if err != nil {
				t.Fatalf("AttachJob() error: %v", err)
			}

			it, err := q.Read(ctx)
			if err != nil {
				t.Fatalf("Read() error: %v", err)
			}

			assertRowCount(t, it, 1)
		})
	}
}

func TestInsertQueryJob(t *testing.T) {
	if len(testClients) == 0 {
		t.Skip("integration tests skipped")
	}
	for k, client := range testClients {
		t.Run(k, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), defaultTestTimeout)
			defer cancel()

			q, err := client.StartQueryJob(ctx, &bigquerypb.Job{
				Configuration: &bigquerypb.JobConfiguration{
					Query: &bigquerypb.JobConfigurationQuery{
						Query:        "SELECT CURRENT_TIMESTAMP() as foo, SESSION_USER() as bar",
						UseLegacySql: wrapperspb.Bool(false),
					},
				},
			})
			if err != nil {
				t.Fatalf("Run() error: %v", err)
			}
			err = q.Wait()
			if err != nil {
				t.Fatalf("Wait() error: %v", err)
			}

			if !q.Complete() {
				t.Fatalf("expected job to be complete")
			}

			it, err := q.Read(ctx)
			if err != nil {
				t.Fatalf("Read() error: %v", err)
			}

			assertRowCount(t, it, 1)
		})
	}
}

func assertRowCount(t *testing.T, it *RowIterator, n int) {
	_, total := readRows(t, it)
	if total != uint64(n) {
		t.Errorf("expected %d row of data, got %d", n, total)
	}
}

func readRows(t *testing.T, it *RowIterator) ([]*Row, uint64) {
	rows := []*Row{}
	for row, err := range it.All() {
		if err != nil {
			t.Fatalf("Next() error: %v", err)
		}
		if row == nil {
			t.Fatalf("row is nil")
		}
		rows = append(rows, row)
	}
	return rows, it.totalRows
}

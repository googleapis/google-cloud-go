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
	"sync"
	"testing"
	"time"

	"cloud.google.com/go/bigquery/v2/apiv2/bigquerypb"
	"google.golang.org/protobuf/types/known/wrapperspb"
)

func TestIntegration_RunQuery(t *testing.T) {
	if len(testQueryHelpers) == 0 {
		t.Skip("integration tests skipped")
	}
	for k, helper := range testQueryHelpers {
		t.Run(k, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), defaultTestTimeout)
			defer cancel()

			req := helper.FromSQL("SELECT CURRENT_TIMESTAMP() as foo, SESSION_USER() as bar")
			req.QueryRequest.JobCreationMode = bigquerypb.QueryRequest_JOB_CREATION_OPTIONAL
			q, err := helper.StartQuery(ctx, req)
			if err != nil {
				t.Fatalf("StartQuery() error: %v", err)
			}

			err = q.Wait(ctx)
			if err != nil {
				t.Fatalf("Wait() error: %v", err)
			}

			if !q.Complete() {
				t.Fatalf("expected job to be complete")
			}

			// TODO: read data and assert row count
		})
	}
}

func TestIntegration_QueryCancelWait(t *testing.T) {
	if len(testQueryHelpers) == 0 {
		t.Skip("integration tests skipped")
	}
	// Run a script with a known min duration, as this test relies on timing of requests.
	sql := `
	DECLARE end_target TIMESTAMP;
	DECLARE poll_check_count INT64;
	SET end_target = TIMESTAMP_ADD(CURRENT_TIMESTAMP, INTERVAL 1 SECOND);
	SET poll_check_count = 0;
	WHILE CURRENT_TIMESTAMP < end_target DO
		SET poll_check_count = poll_check_count + 1;
	END WHILE;
	SELECT CURRENT_TIMESTAMP as ts, poll_check_count AS foo
	`
	for k, helper := range testQueryHelpers {
		t.Run(k, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), defaultTestTimeout)
			defer cancel()

			req := helper.FromSQL(sql)
			// We want the first RPC to return before cancellation, so we set a low timeout
			// so that the query has populated a job reference.
			req.QueryRequest.TimeoutMs = wrapperspb.UInt32(250)
			req.QueryRequest.UseQueryCache = wrapperspb.Bool(false)

			wctx, wcancel := context.WithCancel(ctx)
			q, err := helper.StartQuery(wctx, req)
			if err != nil {
				t.Fatalf("StartQuery() error: %v", err)
			}

			// Use a wg to synchronize progress of a goroutine.
			var wg sync.WaitGroup
			var waitErr error

			wg.Add(1)
			go func(t *testing.T) {
				waitErr = q.Wait(ctx)
				wg.Done()
			}(t)

			// sleep on main thread, then cancel
			time.Sleep(1 * time.Second)
			wcancel()

			// wait for the cancellation and evaluate expectations.
			wg.Wait()
			if waitErr != context.Canceled {
				t.Errorf("Wait() should return context.Canceled, returned: %v", waitErr)
			}
			if q.Complete() {
				t.Fatalf("Complete() should be false")
			}

			// Re-attach and wait again.
			nq, err := helper.AttachJob(ctx, q.JobReference())
			if err != nil {
				t.Fatalf("AttachJob() error: %v", err)
			}

			err = nq.Wait(ctx)
			if err != nil {
				t.Fatalf("Wait() error: %v", err)
			}

			if !nq.Complete() {
				t.Fatalf("Complete() should be true after Wait()")
			}

			// TODO: read data and assert row count
		})
	}
}

func TestIntegration_InsertQueryJob(t *testing.T) {
	if len(testQueryHelpers) == 0 {
		t.Skip("integration tests skipped")
	}
	for k, helper := range testQueryHelpers {
		t.Run(k, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), defaultTestTimeout)
			defer cancel()

			q, err := helper.StartQueryJob(ctx, &bigquerypb.Job{
				Configuration: &bigquerypb.JobConfiguration{
					Query: &bigquerypb.JobConfigurationQuery{
						Query:        "SELECT CURRENT_TIMESTAMP() as foo, SESSION_USER() as bar",
						UseLegacySql: wrapperspb.Bool(false),
					},
				},
			})
			if err != nil {
				t.Fatalf("StartQueryJob() error: %v", err)
			}

			err = q.Wait(ctx)
			if err != nil {
				t.Fatalf("Wait() error: %v", err)
			}

			if !q.Complete() {
				t.Fatalf("expected job to be complete")
			}

			// TODO: read data and assert row count
		})
	}
}

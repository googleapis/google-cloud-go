// Copyright 2017 Google LLC
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//      http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package bigquery

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"reflect"
	"testing"
	"time"

	"cloud.google.com/go/internal/testutil"
	bq "google.golang.org/api/bigquery/v2"
	"google.golang.org/api/googleapi"
	"google.golang.org/api/option"
)

func TestCreateJobRef(t *testing.T) {
	defer fixRandomID("RANDOM")()
	cNoLoc := &Client{projectID: "projectID"}
	cLoc := &Client{projectID: "projectID", Location: "defaultLoc"}
	for _, test := range []struct {
		in     JobIDConfig
		client *Client
		want   *bq.JobReference
	}{
		{
			in:   JobIDConfig{JobID: "foo"},
			want: &bq.JobReference{JobId: "foo"},
		},
		{
			in:   JobIDConfig{},
			want: &bq.JobReference{JobId: "RANDOM"},
		},
		{
			in:   JobIDConfig{AddJobIDSuffix: true},
			want: &bq.JobReference{JobId: "RANDOM"},
		},
		{
			in:   JobIDConfig{JobID: "foo", AddJobIDSuffix: true},
			want: &bq.JobReference{JobId: "foo-RANDOM"},
		},
		{
			in:   JobIDConfig{JobID: "foo", Location: "loc"},
			want: &bq.JobReference{JobId: "foo", Location: "loc"},
		},
		{
			in:     JobIDConfig{JobID: "foo"},
			client: cLoc,
			want:   &bq.JobReference{JobId: "foo", Location: "defaultLoc"},
		},
		{
			in:     JobIDConfig{JobID: "foo", Location: "loc"},
			client: cLoc,
			want:   &bq.JobReference{JobId: "foo", Location: "loc"},
		},
		{
			in:   JobIDConfig{JobID: "foo", ProjectID: "anotherProj"},
			want: &bq.JobReference{JobId: "foo", ProjectId: "anotherProj"},
		},
	} {
		client := test.client
		if client == nil {
			client = cNoLoc
		}
		got := test.in.createJobRef(client)
		if test.want.ProjectId == "" {
			test.want.ProjectId = "projectID"
		}
		if !testutil.Equal(got, test.want) {
			t.Errorf("%+v: got %+v, want %+v", test.in, got, test.want)
		}
	}
}

// Ideally this would be covered by an integration test but simulating
// performance issues in a dummy project is difficult and requires a lot of set
// up.
func Test_JobPerformanceInsights(t *testing.T) {
	for _, test := range []struct {
		name string
		in   *bq.PerformanceInsights
		want *PerformanceInsights
	}{
		{
			name: "nil",
		},
		{
			name: "time only",
			in:   &bq.PerformanceInsights{AvgPreviousExecutionMs: 128},
			want: &PerformanceInsights{AvgPreviousExecution: 128 * time.Millisecond},
		},
		{
			name: "full",
			in: &bq.PerformanceInsights{
				AvgPreviousExecutionMs: 128,
				StagePerformanceChangeInsights: []*bq.StagePerformanceChangeInsight{
					{InputDataChange: &bq.InputDataChange{RecordsReadDiffPercentage: 1.23}, StageId: 123},
					{InputDataChange: &bq.InputDataChange{RecordsReadDiffPercentage: 4.56}, StageId: 456},
				},
				StagePerformanceStandaloneInsights: []*bq.StagePerformanceStandaloneInsight{
					{
						BiEngineReasons: []*bq.BiEngineReason{
							{Code: "bi-code-1", Message: "bi-message-1"},
						},
						HighCardinalityJoins: []*bq.HighCardinalityJoin{
							{LeftRows: 11, OutputRows: 22, RightRows: 33, StepIndex: 112233},
							{LeftRows: 44, OutputRows: 55, RightRows: 66, StepIndex: 445566},
						},
						InsufficientShuffleQuota: true,
						PartitionSkew: &bq.PartitionSkew{SkewSources: []*bq.SkewSource{
							{StageId: 321},
							{StageId: 654},
						}},
						StageId: 123456,
					},
					{
						BiEngineReasons: []*bq.BiEngineReason{
							{Code: "bi-code-2", Message: "bi-message-2"},
							{Code: "bi-code-3", Message: "bi-message-3"},
						},
						HighCardinalityJoins: []*bq.HighCardinalityJoin{
							{LeftRows: 77, OutputRows: 88, RightRows: 99, StepIndex: 778899},
						},
						PartitionSkew:  &bq.PartitionSkew{SkewSources: []*bq.SkewSource{{StageId: 987}}},
						SlotContention: true,
						StageId:        654321,
					},
				},
			},
			want: &PerformanceInsights{
				AvgPreviousExecution: 128 * time.Millisecond,
				StagePerformanceChangeInsights: []*StagePerformanceChangeInsight{
					{InputDataChange: &InputDataChange{RecordsReadDiffPercentage: 1.23}, StageID: 123},
					{InputDataChange: &InputDataChange{RecordsReadDiffPercentage: 4.56}, StageID: 456},
				},
				StagePerformanceStandaloneInsights: []*StagePerformanceStandaloneInsight{
					{
						BIEngineReasons: []*BIEngineReason{
							{Code: "bi-code-1", Message: "bi-message-1"},
						},
						HighCardinalityJoins: []*HighCardinalityJoin{
							{LeftRows: 11, OutputRows: 22, RightRows: 33, StepIndex: 112233},
							{LeftRows: 44, OutputRows: 55, RightRows: 66, StepIndex: 445566},
						},
						InsufficientShuffleQuota: true,
						PartitionSkew:            &PartitionSkew{SkewSources: []*SkewSource{{StageID: 321}, {StageID: 654}}},
						StageID:                  123456,
					},
					{
						BIEngineReasons: []*BIEngineReason{
							{Code: "bi-code-2", Message: "bi-message-2"},
							{Code: "bi-code-3", Message: "bi-message-3"},
						},
						HighCardinalityJoins: []*HighCardinalityJoin{
							{LeftRows: 77, OutputRows: 88, RightRows: 99, StepIndex: 778899},
						},
						PartitionSkew:  &PartitionSkew{SkewSources: []*SkewSource{{StageID: 987}}},
						SlotContention: true,
						StageID:        654321,
					},
				},
			},
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			out := bqToPerformanceInsights(test.in)
			if !reflect.DeepEqual(test.want, out) {
				t.Error("out != want")
			}
		})
	}
}

func fixRandomID(s string) func() {
	prev := randomIDFn
	randomIDFn = func() string { return s }
	return func() { randomIDFn = prev }
}

func checkJob(t *testing.T, i int, got, want *bq.Job) {
	if got.JobReference == nil {
		t.Errorf("#%d: empty job  reference", i)
		return
	}
	if got.JobReference.JobId == "" {
		t.Errorf("#%d: empty job ID", i)
		return
	}
	d := testutil.Diff(got, want)
	if d != "" {
		t.Errorf("#%d: (got=-, want=+) %s", i, d)
	}
}

// waitForQuery polls jobs.getQueryResults, so it must use
// defaultRetryReasons: transient reasons are retried, while enqueue-only
// reasons like jobRateLimitExceeded (see jobRetryReasons) must surface
// immediately instead of retrying until the context deadline.
func TestWaitForQueryRetryReasons(t *testing.T) {
	for _, test := range []struct {
		name         string
		reason       string
		wantAttempts int
		wantErr      bool
	}{
		{
			name:         "jobRateLimitExceeded is not retried",
			reason:       "jobRateLimitExceeded",
			wantAttempts: 1,
			wantErr:      true,
		},
		{
			name:         "rateLimitExceeded is retried",
			reason:       "rateLimitExceeded",
			wantAttempts: 2,
			wantErr:      false,
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			attempts := 0
			ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				attempts++
				if attempts >= test.wantAttempts && !test.wantErr {
					w.Write([]byte(`{"jobReference": {"projectId": "p", "jobId": "j"}, "jobComplete": true, "totalRows": "0"}`))
					return
				}
				w.WriteHeader(http.StatusForbidden)
				fmt.Fprintf(w, `{"error": {"code": 403, "message": "quota exceeded", "errors": [{"reason": %q, "message": "quota exceeded"}]}}`, test.reason)
			}))
			defer ts.Close()

			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			client, err := NewClient(ctx, "p", option.WithEndpoint(ts.URL), option.WithoutAuthentication())
			if err != nil {
				t.Fatalf("NewClient: %v", err)
			}
			defer client.Close()

			job := &Job{c: client, projectID: "p", jobID: "j"}
			_, _, err = job.waitForQuery(ctx, "p")
			if gotErr := err != nil; gotErr != test.wantErr {
				t.Fatalf("waitForQuery: got error %v, want error: %t", err, test.wantErr)
			}
			if test.wantErr {
				var e *googleapi.Error
				if !errors.As(err, &e) || e.Code != http.StatusForbidden {
					t.Errorf("waitForQuery: got %v, want a googleapi.Error with code 403", err)
				}
			}
			if attempts != test.wantAttempts {
				t.Errorf("waitForQuery: got %d attempts, want %d", attempts, test.wantAttempts)
			}
		})
	}
}

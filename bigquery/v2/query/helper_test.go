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
	"testing"

	"cloud.google.com/go/bigquery/v2/apiv2/bigquerypb"
	"cloud.google.com/go/bigquery/v2/apiv2_client"
	"github.com/google/go-cmp/cmp"
	"google.golang.org/api/option"
	"google.golang.org/protobuf/testing/protocmp"
	"google.golang.org/protobuf/types/known/wrapperspb"
)

func TestNewHelper(t *testing.T) {
	// Dummy client.
	client := &apiv2_client.Client{}
	projectID := "test-project"

	testCases := []struct {
		name string
		opts []option.ClientOption
		want *Helper
	}{
		{
			name: "no options",
			opts: nil,
			want: &Helper{
				c:               client,
				projectID:       projectID,
				jobCreationMode: bigquerypb.QueryRequest_JOB_CREATION_MODE_UNSPECIFIED,
				location:        nil,
			},
		},
		{
			name: "with location",
			opts: []option.ClientOption{WithDefaultLocation("us-central1")},
			want: &Helper{
				c:               client,
				projectID:       projectID,
				jobCreationMode: bigquerypb.QueryRequest_JOB_CREATION_MODE_UNSPECIFIED,
				location:        wrapperspb.String("us-central1"),
			},
		},
		{
			name: "with job creation mode",
			opts: []option.ClientOption{WithDefaultJobCreationMode(bigquerypb.QueryRequest_JOB_CREATION_REQUIRED)},
			want: &Helper{
				c:               client,
				projectID:       projectID,
				jobCreationMode: bigquerypb.QueryRequest_JOB_CREATION_REQUIRED,
				location:        nil,
			},
		},
		{
			name: "with multiple options",
			opts: []option.ClientOption{
				WithDefaultLocation("eu-west1"),
				WithDefaultJobCreationMode(bigquerypb.QueryRequest_JOB_CREATION_OPTIONAL),
			},
			want: &Helper{
				c:               client,
				projectID:       projectID,
				jobCreationMode: bigquerypb.QueryRequest_JOB_CREATION_OPTIONAL,
				location:        wrapperspb.String("eu-west1"),
			},
		},
	}

	// Custom comparer for the client, since we only care about pointer equality for this test.
	clientComparer := cmp.Comparer(func(x, y *apiv2_client.Client) bool {
		return x == y
	})

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := NewHelper(client, projectID, tc.opts...)
			if err != nil {
				t.Fatalf("NewHelper() got err: %v", err)
			}
			if diff := cmp.Diff(tc.want, got, protocmp.Transform(), cmp.AllowUnexported(Helper{}), clientComparer); diff != "" {
				t.Errorf("NewHelper() mismatch (-want +got):\n%s", diff)
			}
		})
	}
}

func TestNewHelper_NilClient(t *testing.T) {
	_, err := NewHelper(nil, "p")
	if err == nil {
		t.Error("NewHelper with nil client should have returned an error")
	}
}

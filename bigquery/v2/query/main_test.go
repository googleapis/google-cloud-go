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
	"log"
	"os"
	"testing"
	"time"

	bigquery "cloud.google.com/go/bigquery/v2/apiv2"
	"cloud.google.com/go/bigquery/v2/apiv2_client"

	"cloud.google.com/go/internal/testutil"
	"google.golang.org/api/option"
)

var testClients map[string]*Client
var testProjectID string
var defaultTestTimeout = 30 * time.Second

func TestMain(m *testing.M) {
	cleanup := setup(context.Background())
	code := m.Run()
	if cleanup != nil {
		cleanup()
	}
	os.Exit(code)
}

// setup establishes integration test env, and returns a cleanup func responsible
// closing closing clients
func setup(ctx context.Context) func() {
	projID := testutil.ProjID()
	if projID == "" {
		log.Printf("project ID undetected")
		return nil
	}
	testProjectID = projID
	ts := testutil.TokenSource(ctx, bigquery.DefaultAuthScopes()...)
	if ts == nil {
		log.Printf("invalid token source")
		return nil
	}
	var opts []option.ClientOption
	opts = append(opts, option.WithTokenSource(ts))
	testClients = make(map[string]*Client)
	var err error

	grpcClient, err := apiv2_client.NewClient(ctx, opts...)
	if err != nil {
		log.Printf("failed to create grpc client: %v", err)
		return nil
	}

	grpcOpts := []option.ClientOption{}
	copy(grpcOpts, opts)
	grpcOpts = append(opts, WithClient(grpcClient))
	testClients["GRPC"], err = NewClient(ctx, testProjectID, grpcOpts...)
	if err != nil {
		testClients = nil
		return nil
	}

	restClient, err := apiv2_client.NewRESTClient(ctx, opts...)
	if err != nil {
		log.Printf("failed to create rest client: %v", err)
		return nil
	}

	restOpts := []option.ClientOption{}
	copy(restOpts, opts)
	restOpts = append(opts, WithClient(restClient))
	testClients["REST"], err = NewClient(ctx, testProjectID, restOpts...)
	if err != nil {
		testClients = nil
		return nil
	}
	return closeClients
}

func closeClients() {
	for k, v := range testClients {
		if err := v.Close(); err != nil {
			log.Printf("closing client %q had error: %v", k, err)
		}
	}
}

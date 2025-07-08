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

// package smoketests provides basic smoke testing of the generated client surfaces.
package smoketests

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

var testClients map[string]*apiv2_client.Client
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
	testClients = make(map[string]*apiv2_client.Client)
	var err error

	testClients["GRPC"], err = apiv2_client.NewClient(ctx, opts...)
	if err != nil {
		testClients = nil
		return nil
	}
	testClients["REST"], err = apiv2_client.NewRESTClient(ctx, opts...)
	if err != nil {
		testClients["GRPC"].Close()
		testClients = nil
		return nil
	}
	return func() {
		for k, v := range testClients {
			if err := v.Close(); err != nil {
				log.Printf("closing client %q had error: %v", k, err)
			}
		}
	}
}

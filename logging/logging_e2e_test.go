// Copyright 2020 Google LLC
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

// End-to-end testing in various GCP environments.
// These tests are long-running and should be skipped by -short tests.

package logging


import (
	// "encoding/json"
	// "fmt"
	// "context"
	"os"
	// "net/http"
	// "net/url"
	// "os/exec"
	"testing"
	// "time"

	// "cloud.google.com/go/internal/testutil"
	// "github.com/golang/protobuf/proto"
	// durpb "github.com/golang/protobuf/ptypes/duration"
	// structpb "github.com/golang/protobuf/ptypes/struct"
	// "google.golang.org/api/logging/v2"
	// "google.golang.org/api/support/bundler"
	// mrpb "google.golang.org/genproto/googleapis/api/monitoredres"
	// logtypepb "google.golang.org/genproto/googleapis/logging/type"
)

// TODO: add other GPC envs, Cloud Run only right now
func TestDetectResource(t *testing.T) {
	t.Parallel()
	if testing.Short() {
		t.Skip("skipping logging e2e GCP tests in short mode")
	}

	projectID := os.Getenv("GCLOUD_TESTS_GOLANG_PROJECT_ID")
	if projectID == "" {
		t.Skip("skipping logging e2e GCP tests when GCLOUD_TESTS_GOLANG_PROJECT_ID variable is not set")
	}

	// TODO
	// 1. Scaffold cloud run 
	// 2. Http trigger the appropriate tests (http, later pubsub)
	// 3. Get log from cloud run container
	// 4. Teardown cloud run

}
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
// Tests scaffold a GCP resource, trigger log tests via http or cloud events, and teardown resources when completed
// These tests are long-running and should be skipped by -short tests.

package logging_test

import (
	"context"
	"fmt"
	"log"
	"os"
	"os/exec"
	"runtime"
	"strings"
	"testing"
	"time"

	"cloud.google.com/go/logging"
	ltesting "cloud.google.com/go/logging/internal/testing"
	"cloud.google.com/go/logging/logadmin"
	"cloud.google.com/go/pubsub"
	"github.com/google/uuid"
	"google.golang.org/api/iterator"
	mrpb "google.golang.org/genproto/googleapis/api/monitoredres"
)

type environment string

const (
	cloudRun      environment = "CloudRun"
	cloudFunction environment = "CloudFunction"
)

// Deploys a Cloud Run container with pubsub subscription
// TODO refactor this into cmd(env, project...)
func cmdCloudRun(projectID string, cmd string, topicId string) string {
	// testId used for creation of image, gcr instance, subscription
	// TODO fix this
	testId := topicId
	scaffoldGCR := &exec.Cmd{
		Path:   "./e2e/cloudrun.sh",
		Args:   []string{"./cloudrun.sh", cmd, topicId, testId},
		Stdout: os.Stdout,
		Stderr: os.Stdout,
	}
	// TODO: wait for it to complete (or run in background with scaffoldGCR.Start())
	if err := scaffoldGCR.Run(); err != nil {
		log.Fatalf("Couldn't do Cloud Run")
	}
	return testId
}

// Cloud Run only right now
func TestDetectResource(t *testing.T) {
	return // TODO remove
	t.Parallel()
	if testing.Short() {
		t.Skip("skipping logging e2e GCP tests in short mode")
	}

	projectID := os.Getenv("GCLOUD_TESTS_GOLANG_PROJECT_ID")
	if projectID == "" {
		t.Skip("skipping logging e2e GCP tests when GCLOUD_TESTS_GOLANG_PROJECT_ID variable is not set")
	}

	if runtime.GOOS == "windows" {
		log.Fatalf("Can't Execute this on a windows machine")
	}

	// **************** ENVS INIT ****************
	// Create pubsub topic
	topicId := "log-" + uuid.New().String()
	ctx := context.Background()
	psClient, err := pubsub.NewClient(ctx, projectID)
	if err != nil {
		log.Fatalf("Failed to create client: %v", err)
	}
	topic, err := psClient.CreateTopic(ctx, topicId)
	if err != nil {
		log.Fatalf("Failed to create client: %v", err)
	}
	defer topic.Stop()

	// Scaffold relevant envs & subscribe to test trigger (Cloud Run only right now)
	testId := cmdCloudRun(projectID, "scaffold", topicId)

	// // **************** TRIGGER LOGS ****************
	var results []*pubsub.PublishResult
	res := topic.Publish(ctx, &pubsub.Message{
		Data: []byte("testStdLog, testBasicLog"),
	})
	results = append(results, res)
	for _, r := range results {
		_, err := r.Get(ctx)
		if err != nil {
			log.Fatalf("Couldn't trigger log tests via pubsub")
		}
	}

	// **************** CHECK LOGS ****************
	var got []*logging.Entry
	ok := ltesting.WaitFor(func() bool {
		fmt.Println("\ninside of wait")
		got, err = getEnvEntries(ctx, testId)
		if err != nil {
			t.Log("fetching log entries: ", err)
			return false
		}
		// TODO: change to wait for testCount
		return len(got) > 0
	})
	if !ok {
		t.Fatalf("timed out, 0 entries")
	}

	// check that log entries contain the correct resource
	if msg, ok := checkLogResource(got, cloudRun, testId); !ok {
		t.Error(msg)
	}

	// **************** TEST TEARDOWN ****************
	fmt.Println("\n Tearing everything down")
	testId = cmdCloudRun(projectID, "teardown", topicId)
	if err := topic.Delete(ctx); err != nil {
		log.Fatalf("Couldn't delete e2e test topic")
	}
}

// filter by labels: testId, testname, testEnv
func getEnvEntries(ctx context.Context, testId string) ([]*logging.Entry, error) {
	logAdminClient, err := logadmin.NewClient(ctx, "log-bench")
	if err != nil {
		log.Fatalf("creating logging client: %v", err)
	}

	hourAgo := time.Now().Add(-1 * time.Hour).UTC()
	// TODO update projectID
	testFilter := fmt.Sprintf(`logName = "projects/%s/logs/%s" AND timestamp >= "%s"`,
		"log-bench", testId, hourAgo.Format(time.RFC3339))
	return getEntries(ctx, logAdminClient, testFilter)
}

func getEntries(ctx context.Context, aclient *logadmin.Client, filter string) ([]*logging.Entry, error) {
	var es []*logging.Entry
	it := aclient.Entries(ctx, logadmin.Filter(filter))
	for {
		e, err := it.Next()
		switch err {
		case nil:
			es = append(es, e)
		case iterator.Done:
			return es, nil
		default:
			return nil, err
		}
	}
}

// Check that got all has the correct field types
func checkLogResource(got []*logging.Entry, env environment, testId string) (string, bool) {
	fmt.Println("\nChecking log resource types")
	for i := range got {
		fmt.Printf("\nChecking log:  %v\n", got[i])
		switch env {
		case cloudRun:
			return isCloudRunResource(got[i].Resource, testId)
		case cloudFunction:
			return "cloud func", false
		default:
			return "lalala", false
		}
	}
	return "", true
}

func isCloudRunResource(res *mrpb.MonitoredResource, testId string) (string, bool) {
	if res.Type != "cloud_run_revision" {
		return fmt.Sprintf("\ngot resource type  %+v\nwant %+v", res, "cloud_run_revision"), false
	}
	if res.Labels["configuration_name"] != testId {
		return fmt.Sprintf("\ngot resource config name  %+v\nwant %+v", res.Labels["configuration_name"], testId), false
	}
	if res.Labels["service_name"] != testId {
		return fmt.Sprintf("\ngot resource service name  %+v\nwant %+v", res.Labels["service_name"], testId), false
	}
	if !strings.Contains(res.Labels["revision_name"], testId) {
		return fmt.Sprintf("\nresource revision name  %+v\ndoes not include substr %+v", res.Labels["revision_name"], testId), false
	}
	if len(res.Labels["project_id"]) == 0 {
		return "\ncloud run resource projectid should not be nil", false
	}
	if len(res.Labels["location"]) == 0 {
		return "\ncloud run resource location should not be nil", false
	}
	return "", true
}

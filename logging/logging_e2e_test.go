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

var (
	projectID string
)

type environment string
const (
	cloudRun      environment = "CloudRun"
	cloudFunction environment = "CloudFunction"
)

// TODO use same pubsub client
// TODO use same logging client
func init() {
	if runtime.GOOS == "windows" {
		log.Fatalf("Can't Execute this on a windows machine")
	}
	// TODO handle similar to testMain
	projectID = os.Getenv("GCLOUD_TESTS_GOLANG_PROJECT_ID")
}

func newPubSubTopic(ctx context.Context, projectID string) (*pubsub.Topic, string) {
	topicId := "log-" + uuid.New().String()
	psClient, err := pubsub.NewClient(ctx, projectID)
	if err != nil {
		log.Fatalf("Failed to create client: %v", err)
	}
	topic, err := psClient.CreateTopic(ctx, topicId)
	if err != nil {
		log.Fatalf("Failed to create client: %v", err)
	}
	return topic, topicId
}

// TODO generalize for not just CloudRun
func scaffold(projectID string, topicId string) {
	fmt.Println("\n Scaffolding the environment")
	cmdEnvironment(projectID, "scaffold", topicId)
}

func teardown(ctx context.Context, projectID string, topicId string, topic *pubsub.Topic) {
	fmt.Println("\n Tearing everything down")
	cmdEnvironment(projectID, "teardown", topicId)
	if err := topic.Delete(ctx); err != nil {
		log.Fatalf("Couldn't delete e2e test topic")
	}
}

// Deploys a Cloud Run container with pubsub subscription
func cmdEnvironment(projectID string, cmd string, topicId string) {
	scaffoldGCR := &exec.Cmd{
		Path:   "./e2e/cloudrun.sh",
		Args:   []string{"./cloudrun.sh", cmd, topicId, topicId},
		Stdout: os.Stdout,
		Stderr: os.Stdout,
	}
	if err := scaffoldGCR.Run(); err != nil {
		log.Fatalf("Couldn't do Cloud Run")
	}
}

// TODO, take in what tests are being run
func triggerTestLogs(ctx context.Context, topic *pubsub.Topic) {
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
}

func getTestLogs(ctx context.Context, topicId string, t *testing.T) []*logging.Entry {
	var got []*logging.Entry
	var err error
	ok := ltesting.WaitFor(func() bool {
		fmt.Println("\ninside of wait")
		got, err = getLogEntries(ctx, topicId)
		if err != nil {
			t.Log("fetching log entries: ", err)
			return false
		}
		return len(got) > 0
	})
	if !ok {
		t.Fatalf("timed out, 0 entries")
	}
	return got
}

func getLogEntries(ctx context.Context, topicId string) ([]*logging.Entry, error) {
	logAdminClient, err := logadmin.NewClient(ctx, "log-bench")
	if err != nil {
		log.Fatalf("creating logging client: %v", err)
	}
	hourAgo := time.Now().Add(-1 * time.Hour).UTC()
	testFilter := fmt.Sprintf(`logName = "projects/%s/logs/%s" AND timestamp >= "%s"`,
		projectID, topicId, hourAgo.Format(time.RFC3339))
	return getEntries(ctx, logAdminClient, testFilter)
}

// todo fix this weird context passing?
func getEntries(ctx context.Context, aclient *logadmin.Client, filter string) ([]*logging.Entry, error) {
	var es []*logging.Entry
	it := aclient.Entries(ctx, logadmin.Filter(filter))
	fmt.Printf("\ngetEntries api returns: %v ", it)
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

func TestGKE(t *testing.T) {
	t.Parallel()
	// TODO(nicoleczhu)
}

func TestGAE(t *testing.T) {
	t.Parallel()
	// TODO(nicoleczhu)
}

func TestGCR(t *testing.T) {
	t.Parallel()

	if testing.Short() {
		t.Skip("skipping logging e2e GCP tests in short mode")
	}
	if projectID == "" {
		t.Skip("skipping logging e2e GCP tests when GCLOUD_TESTS_GOLANG_PROJECT_ID variable is not set")
	}

	ctx := context.Background()
	topic, topicId := newPubSubTopic(ctx, projectID)
	defer topic.Stop()

	scaffold(projectID, topicId)
	defer teardown(ctx, projectID, topicId, topic)

	// TODO construct testToRun array instead of running all tests
	triggerTestLogs(ctx, topic)
	got := getTestLogs(ctx, topicId, t)

	if msg, ok := checkLogResource(got, cloudRun, topicId); !ok {
		t.Error(msg)
	}
}

// Check that got all has the correct field types
func checkLogResource(got []*logging.Entry, env environment, topicId string) (string, bool) {
	fmt.Printf("\nChecking log resource types for %v: ", got)
	for i := range got {
		fmt.Printf("\nChecking log:  %v\n", got[i])
		switch env {
		case cloudRun:
			return isCloudRunResource(got[i].Resource, topicId)
		default:
			return "lalala", false
		}
	}
	return "", true
}

func isCloudRunResource(res *mrpb.MonitoredResource, topicId string) (string, bool) {
	if res.Type != "cloud_run_revision" {
		return fmt.Sprintf("\ngot resource type  %+v\nwant %+v", res, "cloud_run_revision"), false
	}
	if res.Labels["configuration_name"] != topicId {
		return fmt.Sprintf("\ngot resource config name  %+v\nwant %+v", res.Labels["configuration_name"], topicId), false
	}
	if res.Labels["service_name"] != topicId {
		return fmt.Sprintf("\ngot resource service name  %+v\nwant %+v", res.Labels["service_name"], topicId), false
	}
	if !strings.Contains(res.Labels["revision_name"], topicId) {
		return fmt.Sprintf("\nresource revision name  %+v\ndoes not include substr %+v", res.Labels["revision_name"], topicId), false
	}
	if len(res.Labels["project_id"]) == 0 {
		return "\ncloud run resource projectid should not be nil", false
	}
	if len(res.Labels["location"]) == 0 {
		return "\ncloud run resource location should not be nil", false
	}
	return "", true
}

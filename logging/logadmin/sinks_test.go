// Copyright 2016 Google LLC
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

// TODO(jba): document in CONTRIBUTING.md that service account must be given "Logs Configuration Writer" IAM role for sink tests to pass.
// TODO(jba): [cont] (1) From top left menu, go to IAM & Admin. (2) In Roles dropdown for acct, select Logging > Logs Configuration Writer. (3) Save.
// TODO(jba): Also, cloud-logs@google.com must have Owner permission on the GCS bucket named for the test project.
// Note: log buckets are only created during integration tests. All buckets must allow logsink writerIdentity creator permissions.

package logadmin

import (
	"context"
	"fmt"
	"log"
	"testing"
	"time"

	"cloud.google.com/go/iam"
	"cloud.google.com/go/internal/testutil"
	"cloud.google.com/go/internal/uid"
	ltest "cloud.google.com/go/logging/internal/testing"
	"cloud.google.com/go/storage"
	"google.golang.org/api/iterator"
	"google.golang.org/api/option"
)

var sinkIDs = uid.NewSpace("GO-CLIENT-TEST-SINK", nil)

const testFilter = ""
const testBucketTTLDays = 1

var testSinkDestination string
var testBucket string

// Called just before TestMain calls m.Run.
// Returns a cleanup function to be called after the tests finish.
func initSinks(ctx context.Context) func() {
	// Create a unique GCS bucket so concurrent tests don't interfere with each other.
	bucketIDs := uid.NewSpace(testProjectID+"-log-sink", nil)
	testBucket = bucketIDs.New()
	testSinkDestination = "storage.googleapis.com/" + testBucket
	var storageClient *storage.Client
	if integrationTest {
		// Create a unique bucket as a sink destination, and give the cloud logging account
		// owner right.
		ts := testutil.TokenSource(ctx, storage.ScopeFullControl)
		var err error
		storageClient, err = storage.NewClient(ctx, option.WithTokenSource(ts))
		if err != nil {
			log.Fatalf("new storage client: %v", err)
		}
		bucket := storageClient.Bucket(testBucket)
		if err := bucket.Create(ctx, testProjectID, nil); err != nil {
			log.Fatalf("creating storage bucket %q: %v", testBucket, err)
		}

		// Set the bucket's lifecycle to autodelete after a period of time
		bucketAttrsToUpdate := storage.BucketAttrsToUpdate{
			Lifecycle: &storage.Lifecycle{
				Rules: []storage.LifecycleRule{
					{
						Action: storage.LifecycleAction{Type: storage.DeleteAction},
						Condition: storage.LifecycleCondition{
							AgeInDays: testBucketTTLDays,
						},
					},
				},
			},
		}
		_, err = bucket.Update(ctx, bucketAttrsToUpdate)
		if err != nil {
			log.Fatalf("updating bucket %q lifecycle rule: %v", testBucket, err)
		}

		// Grant destination permissions to sink's writer identity.
		err = addBucketCreator(testBucket, ltest.SharedServiceAccount)
		if err != nil {
			log.Fatal(err)
		}
		log.Printf("successfully created bucket %s", testBucket)

		err = addBucketIAMPolicy(testBucket, "group:cloud-logs@google.com", "roles/storage.admin")
		if err != nil {
			log.Fatal(err)
		}
	}
	// Clean up from aborted tests.
	it := client.Sinks(ctx)
	for {
		s, err := it.Next()
		if err == iterator.Done {
			break
		}
		if err != nil {
			log.Printf("listing sinks: %v", err)
			break
		}
		if sinkIDs.Older(s.ID, time.Hour) {
			client.DeleteSink(ctx, s.ID) // ignore error
		}
	}
	if integrationTest {
		for _, bn := range bucketNames(ctx, storageClient) {
			if bucketIDs.Older(bn, 36*time.Hour) {
				storageClient.Bucket(bn).Delete(ctx) // ignore error
			}
		}
		return func() {
			// Cleanup the bucket we used on this test run.
			defer storageClient.Close()
			if err := storageClient.Bucket(testBucket).Delete(ctx); err != nil {
				log.Printf("deleting %q: %v", testBucket, err)
			}
		}
	}
	return func() {}
}

// Collect the name of all buckets for the test project.
func bucketNames(ctx context.Context, client *storage.Client) []string {
	var names []string
	it := client.Buckets(ctx, testProjectID)
loop:
	for {
		b, err := it.Next()
		switch err {
		case nil:
			names = append(names, b.Name)
		case iterator.Done:
			break loop
		default:
			log.Printf("listing buckets: %v", err)
			break loop
		}
	}
	return names
}

// addBucketIAMPolicy adds the specified IAM policy to the bucket pointed to by the bucketHandle.
// Required for all new log sink service accounts.
func addBucketIAMPolicy(bucketName string, identity string, role iam.RoleName) error {
	if integrationTest {
		ctx := context.Background()
		client, err := storage.NewClient(ctx, option.WithTokenSource(testutil.TokenSource(ctx, storage.ScopeFullControl)))
		if err != nil {
			return fmt.Errorf("storage.NewClient: %v", err)
		}
		defer client.Close()

		ctx, cancel := context.WithTimeout(ctx, time.Second*10)
		defer cancel()

		bucket := client.Bucket(bucketName)
		policy, err := bucket.IAM().Policy(ctx)
		if err != nil {
			return fmt.Errorf("Bucket(%q).IAM().Policy: %v", bucketName, err)
		}

		policy.Add(identity, role)
		if err := bucket.IAM().SetPolicy(ctx, policy); err != nil {
			return fmt.Errorf("Bucket(%q).IAM().SetPolicy: %v", bucketName, err)
		}
	}

	return nil
}

// addBucketCreator adds the bucket IAM member to permission role. Required for all new log sink service accounts.
func addBucketCreator(bucketName string, identity string) error {
	return addBucketIAMPolicy(bucketName, identity, "roles/storage.objectCreator")
}

func TestCreateSink(t *testing.T) {
	ctx := context.Background()
	sink := &Sink{
		ID:              sinkIDs.New(),
		Destination:     testSinkDestination,
		Filter:          testFilter,
		IncludeChildren: true,
	}
	got, err := client.CreateSink(ctx, sink)
	if err != nil {
		t.Fatal(err)
	}
	defer client.DeleteSink(ctx, sink.ID)

	sink.WriterIdentity = ltest.SharedServiceAccount
	if want := sink; !testutil.Equal(got, want) {
		t.Errorf("got %+v, want %+v", got, want)
	}
	got, err = client.Sink(ctx, sink.ID)
	if err != nil {
		t.Fatal(err)
	}
	if want := sink; !testutil.Equal(got, want) {
		t.Errorf("got %+v, want %+v", got, want)
	}

	// UniqueWriterIdentity
	sink.ID = sinkIDs.New()
	got, err = client.CreateSinkOpt(ctx, sink, SinkOptions{UniqueWriterIdentity: true})
	if err != nil {
		t.Fatal(err)
	}
	defer client.DeleteSink(ctx, sink.ID)

	// Grant destination permissions to sink's writer identity.
	err = addBucketCreator(testBucket, got.WriterIdentity)
	if err != nil {
		t.Fatal(err)
	}
	// The WriterIdentity should be different.
	if got.WriterIdentity == sink.WriterIdentity {
		t.Errorf("got %s, want something different", got.WriterIdentity)
	}
}

func TestUpdateSink(t *testing.T) {
	ctx := context.Background()
	sink := &Sink{
		ID:              sinkIDs.New() + "-" + t.Name(),
		Destination:     testSinkDestination,
		Filter:          testFilter,
		IncludeChildren: true,
		WriterIdentity:  ltest.SharedServiceAccount,
	}

	_, err := client.CreateSink(ctx, sink)
	if err != nil {
		t.Fatal(err)
	}
	defer client.DeleteSink(ctx, sink.ID)

	got, err := client.UpdateSink(ctx, sink)
	if err != nil {
		t.Fatal(err)
	}
	if want := sink; !testutil.Equal(got, want) {
		t.Errorf("got\n%+v\nwant\n%+v", got, want)
	}
	got, err = client.Sink(ctx, sink.ID)
	if err != nil {
		t.Fatal(err)
	}
	if want := sink; !testutil.Equal(got, want) {
		t.Errorf("got\n%+v\nwant\n%+v", got, want)
	}

	// Updating an existing sink changes it.
	sink.Filter = ""
	sink.IncludeChildren = false
	if _, err := client.UpdateSink(ctx, sink); err != nil {
		t.Fatal(err)
	}
	got, err = client.Sink(ctx, sink.ID)
	if err != nil {
		t.Fatal(err)
	}
	if want := sink; !testutil.Equal(got, want) {
		t.Errorf("got\n%+v\nwant\n%+v", got, want)
	}
}

func TestUpdateSinkOpt(t *testing.T) {
	ctx := context.Background()
	id := sinkIDs.New()
	origSink := &Sink{
		ID:              id,
		Destination:     testSinkDestination,
		Filter:          testFilter,
		IncludeChildren: true,
		WriterIdentity:  ltest.SharedServiceAccount,
	}

	_, err := client.CreateSink(ctx, origSink)
	if err != nil {
		t.Fatal(err)
	}
	defer client.DeleteSink(ctx, origSink.ID)

	// Updating with empty options is an error.
	_, err = client.UpdateSinkOpt(ctx, &Sink{ID: id, Destination: testSinkDestination}, SinkOptions{})
	if err == nil {
		t.Errorf("got %v, want nil", err)
	}

	// Update selected fields.
	got, err := client.UpdateSinkOpt(ctx, &Sink{ID: id}, SinkOptions{
		UpdateFilter:          true,
		UpdateIncludeChildren: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	want := *origSink
	want.Filter = ""
	want.IncludeChildren = false
	if !testutil.Equal(got, &want) {
		t.Errorf("got\n%+v\nwant\n%+v", got, want)
	}

	// Update writer identity.
	got, err = client.UpdateSinkOpt(ctx, &Sink{ID: id, Filter: "foo"},
		SinkOptions{UniqueWriterIdentity: true})
	if err != nil {
		t.Fatal(err)
	}
	// Grant destination permissions to sink's new writer identity.
	err = addBucketCreator(testBucket, got.WriterIdentity)
	if err != nil {
		t.Fatal(err)
	}
	if got.WriterIdentity == want.WriterIdentity {
		t.Errorf("got %s, want something different", got.WriterIdentity)
	}
	want.WriterIdentity = got.WriterIdentity
	if !testutil.Equal(got, &want) {
		t.Errorf("got\n%+v\nwant\n%+v", got, want)
	}
}

func TestListSinks(t *testing.T) {
	ctx := context.Background()
	var sinks []*Sink
	want := map[string]*Sink{}
	for i := 0; i < 4; i++ {
		s := &Sink{
			ID:             sinkIDs.New(),
			Destination:    testSinkDestination,
			Filter:         testFilter,
			WriterIdentity: "serviceAccount:cloud-logs@system.gserviceaccount.com",
		}
		sinks = append(sinks, s)
		want[s.ID] = s
	}
	for _, s := range sinks {
		_, err := client.CreateSink(ctx, s)
		if err != nil {
			t.Fatalf("Create(%q): %v", s.ID, err)
		}
		defer client.DeleteSink(ctx, s.ID)
	}

	got := map[string]*Sink{}
	it := client.Sinks(ctx)
	for {
		s, err := it.Next()
		if err == iterator.Done {
			break
		}
		if err != nil {
			t.Fatal(err)
		}
		// If tests run simultaneously, we may have more sinks than we
		// created. So only check for our own.
		if _, ok := want[s.ID]; ok {
			got[s.ID] = s
		}
	}
	if !testutil.Equal(got, want) {
		t.Errorf("got %+v, want %+v", got, want)
	}
}

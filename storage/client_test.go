// Copyright 2022 Google LLC
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

package storage

import (
	"context"
	"fmt"
	"log"
	"os"
	"strconv"
	"testing"
	"time"

	"github.com/google/go-cmp/cmp"
	"google.golang.org/api/iterator"
	iampb "google.golang.org/genproto/googleapis/iam/v1"
)

var emulatorClients map[string]storageClient
var veneerClient *Client

func TestCreateBucketEmulated(t *testing.T) {
	transportClientTest(t, func(t *testing.T, project, bucket string, client storageClient) {
		want := &BucketAttrs{
			Name: bucket,
		}
		got, err := client.CreateBucket(context.Background(), project, want)
		if err != nil {
			t.Fatal(err)
		}
		want.Location = "US"
		if diff := cmp.Diff(got.Name, want.Name); diff != "" {
			t.Errorf("got(-),want(+):\n%s", diff)
		}
		if diff := cmp.Diff(got.Location, want.Location); diff != "" {
			t.Errorf("got(-),want(+):\n%s", diff)
		}
	})
}

func TestDeleteBucketEmulated(t *testing.T) {
	transportClientTest(t, func(t *testing.T, project, bucket string, client storageClient) {
		b := &BucketAttrs{
			Name: bucket,
		}
		// Create the bucket that will be deleted.
		_, err := client.CreateBucket(context.Background(), project, b)
		if err != nil {
			t.Fatalf("client.CreateBucket: %v", err)
		}
		// Delete the bucket that was just created.
		err = client.DeleteBucket(context.Background(), b.Name, nil)
		if err != nil {
			t.Fatalf("client.DeleteBucket: %v", err)
		}
	})
}

func TestGetBucketEmulated(t *testing.T) {
	transportClientTest(t, func(t *testing.T, project, bucket string, client storageClient) {
		want := &BucketAttrs{
			Name: bucket,
		}
		// Create the bucket that will be retrieved.
		_, err := client.CreateBucket(context.Background(), project, want)
		if err != nil {
			t.Fatalf("client.CreateBucket: %v", err)
		}
		got, err := client.GetBucket(context.Background(), want.Name, &BucketConditions{MetagenerationMatch: 1})
		if err != nil {
			t.Fatal(err)
		}
		if diff := cmp.Diff(got.Name, want.Name); diff != "" {
			t.Errorf("got(-),want(+):\n%s", diff)
		}
	})
}

func TestGetServiceAccountEmulated(t *testing.T) {
	transportClientTest(t, func(t *testing.T, project, bucket string, client storageClient) {
		_, err := client.GetServiceAccount(context.Background(), project)
		if err != nil {
			t.Fatalf("client.GetServiceAccount: %v", err)
		}
	})
}

func TestGetSetTestIamPolicyEmulated(t *testing.T) {
	transportClientTest(t, func(t *testing.T, project, bucket string, client storageClient) {
		battrs, err := client.CreateBucket(context.Background(), project, &BucketAttrs{
			Name: bucket,
		})
		if err != nil {
			t.Fatalf("client.CreateBucket: %v", err)
		}
		got, err := client.GetIamPolicy(context.Background(), battrs.Name, 0)
		if err != nil {
			t.Fatalf("client.GetIamPolicy: %v", err)
		}
		err = client.SetIamPolicy(context.Background(), battrs.Name, &iampb.Policy{
			Etag:     got.GetEtag(),
			Bindings: []*iampb.Binding{{Role: "roles/viewer", Members: []string{"allUsers"}}},
		})
		if err != nil {
			t.Fatalf("client.SetIamPolicy: %v", err)
		}
		want := []string{"storage.foo", "storage.bar"}
		perms, err := client.TestIamPermissions(context.Background(), battrs.Name, want)
		if err != nil {
			t.Fatalf("client.TestIamPermissions: %v", err)
		}
		if diff := cmp.Diff(perms, want); diff != "" {
			t.Errorf("got(-),want(+):\n%s", diff)
		}
	})
}

func TestListObjectsEmulated(t *testing.T) {
	transportClientTest(t, func(t *testing.T, project, bucket string, client storageClient) {
		// Populate test data.
		_, err := client.CreateBucket(context.Background(), project, &BucketAttrs{
			Name: bucket,
		})
		if err != nil {
			t.Fatalf("client.CreateBucket: %v", err)
		}
		prefix := time.Now().Nanosecond()
		want := []*ObjectAttrs{
			{
				Bucket: bucket,
				Name:   fmt.Sprintf("%d-object-%d", prefix, time.Now().Nanosecond()),
			},
			{
				Bucket: bucket,
				Name:   fmt.Sprintf("%d-object-%d", prefix, time.Now().Nanosecond()),
			},
			{
				Bucket: bucket,
				Name:   fmt.Sprintf("object-%d", time.Now().Nanosecond()),
			},
		}
		for _, obj := range want {
			w := veneerClient.Bucket(bucket).Object(obj.Name).NewWriter(context.Background())
			if _, err := w.Write(randomBytesToWrite); err != nil {
				t.Fatalf("failed to populate test data: %v", err)
			}
			if err := w.Close(); err != nil {
				t.Fatalf("closing object: %v", err)
			}
		}

		// Simple list, no query.
		it := client.ListObjects(context.Background(), bucket, nil)
		var o *ObjectAttrs
		var got int
		for i := 0; err == nil && i <= len(want); i++ {
			o, err = it.Next()
			if err != nil {
				break
			}
			got++
			if diff := cmp.Diff(o.Name, want[i].Name); diff != "" {
				t.Errorf("got(-),want(+):\n%s", diff)
			}
		}
		if err != iterator.Done {
			t.Fatalf("expected %q but got %q", iterator.Done, err)
		}
		expected := len(want)
		if got != expected {
			t.Errorf("expected to get %d objects, but got %d", expected, got)
		}
	})
}

func TestListObjectsWithPrefixEmulated(t *testing.T) {
	transportClientTest(t, func(t *testing.T, project, bucket string, client storageClient) {
		// Populate test data.
		_, err := client.CreateBucket(context.Background(), project, &BucketAttrs{
			Name: bucket,
		})
		if err != nil {
			t.Fatalf("client.CreateBucket: %v", err)
		}
		prefix := time.Now().Nanosecond()
		want := []*ObjectAttrs{
			{
				Bucket: bucket,
				Name:   fmt.Sprintf("%d-object-%d", prefix, time.Now().Nanosecond()),
			},
			{
				Bucket: bucket,
				Name:   fmt.Sprintf("%d-object-%d", prefix, time.Now().Nanosecond()),
			},
			{
				Bucket: bucket,
				Name:   fmt.Sprintf("object-%d", time.Now().Nanosecond()),
			},
		}
		for _, obj := range want {
			w := veneerClient.Bucket(bucket).Object(obj.Name).NewWriter(context.Background())
			if _, err := w.Write(randomBytesToWrite); err != nil {
				t.Fatalf("failed to populate test data: %v", err)
			}
			if err := w.Close(); err != nil {
				t.Fatalf("closing object: %v", err)
			}
		}

		// Query with Prefix.
		it := client.ListObjects(context.Background(), bucket, &Query{Prefix: strconv.Itoa(prefix)})
		var o *ObjectAttrs
		var got int
		want = want[:2]
		for i := 0; i <= len(want); i++ {
			o, err = it.Next()
			if err != nil {
				break
			}
			got++
			if diff := cmp.Diff(o.Name, want[i].Name); diff != "" {
				t.Errorf("got(-),want(+):\n%s", diff)
			}
		}
		if err != iterator.Done {
			t.Fatalf("expected %q but got %q", iterator.Done, err)
		}
		expected := len(want)
		if got != expected {
			t.Errorf("expected to get %d objects, but got %d", expected, got)
		}
	})
}

func TestListBucketsEmulated(t *testing.T) {
	transportClientTest(t, func(t *testing.T, project, bucket string, client storageClient) {
		prefix := time.Now().Nanosecond()
		want := []*BucketAttrs{
			{Name: fmt.Sprintf("%d-%s-%d", prefix, bucket, time.Now().Nanosecond())},
			{Name: fmt.Sprintf("%d-%s-%d", prefix, bucket, time.Now().Nanosecond())},
			{Name: fmt.Sprintf("%s-%d", bucket, time.Now().Nanosecond())},
		}
		// Create the buckets that will be listed.
		for _, b := range want {
			_, err := client.CreateBucket(context.Background(), project, b)
			if err != nil {
				t.Fatalf("client.CreateBucket: %v", err)
			}
		}

		it := client.ListBuckets(context.Background(), project)
		it.Prefix = strconv.Itoa(prefix)
		// Drop the non-prefixed bucket from the expected results.
		want = want[:2]
		var err error
		var b *BucketAttrs
		var got int
		for i := 0; err == nil && i <= len(want); i++ {
			b, err = it.Next()
			if err != nil {
				break
			}
			got++
			if diff := cmp.Diff(b.Name, want[i].Name); diff != "" {
				t.Errorf("got(-),want(+):\n%s", diff)
			}
		}
		if err != iterator.Done {
			t.Fatalf("expected %q but got %q", iterator.Done, err)
		}
		expected := len(want)
		if got != expected {
			t.Errorf("expected to get %d buckets, but got %d", expected, got)
		}
	})
}

func TestListBucketACLsEmulated(t *testing.T) {
	transportClientTest(t, func(t *testing.T, project, bucket string, client storageClient) {
		ctx := context.Background()
		want := &BucketAttrs{
			Name:          bucket,
			PredefinedACL: "publicRead",
		}
		// Create the bucket that will be retrieved.
		_, err := client.CreateBucket(ctx, project, want)
		if err != nil {
			t.Fatalf("client.CreateBucket: %v", err)
		}

		acls, err := client.ListBucketACLs(ctx, bucket)
		if err != nil {
			t.Fatalf("client.ListBucketACLs: %v", err)
		}
		if want, got := len(acls), 2; want != got {
			t.Errorf("ListBucketACLs: got %v, want %v items", acls, want)
		}
	})
}

func TestListDefaultObjectACLsEmulated(t *testing.T) {
	transportClientTest(t, func(t *testing.T, project, bucket string, client storageClient) {
		ctx := context.Background()
		want := &BucketAttrs{
			Name:                       bucket,
			PredefinedDefaultObjectACL: "publicRead",
		}
		// Create the bucket that will be retrieved.
		_, err := client.CreateBucket(ctx, project, want)
		if err != nil {
			t.Fatalf("client.CreateBucket: %v", err)
		}

		acls, err := client.ListDefaultObjectACLs(ctx, bucket)
		if err != nil {
			t.Fatalf("client.ListDefaultObjectACLs: %v", err)
		}
		if want, got := len(acls), 2; want != got {
			t.Errorf("ListDefaultObjectACLs: got %v, want %v items", acls, want)
		}
	})
}

func initEmulatorClients() func() error {
	noopCloser := func() error { return nil }
	if !isEmulatorEnvironmentSet() {
		return noopCloser
	}
	ctx := context.Background()

	grpcClient, err := newGRPCStorageClient(ctx)
	if err != nil {
		log.Fatalf("Error setting up gRPC client for emulator tests: %v", err)
		return noopCloser
	}
	httpClient, err := newHTTPStorageClient(ctx)
	if err != nil {
		log.Fatalf("Error setting up HTTP client for emulator tests: %v", err)
		return noopCloser
	}

	emulatorClients = map[string]storageClient{
		"http": httpClient,
		"grpc": grpcClient,
	}

	veneerClient, err = NewClient(ctx)
	if err != nil {
		log.Fatalf("Error setting up Veneer client for emulator tests: %v", err)
		return noopCloser
	}

	return func() error {
		gerr := grpcClient.Close()
		herr := httpClient.Close()
		verr := veneerClient.Close()

		if gerr != nil {
			return gerr
		} else if herr != nil {
			return herr
		}
		return verr
	}
}

// transportClienttest executes the given function with a sub-test, a project name
// based on the transport, a unique bucket name also based on the transport, and
// the transport-specific client to run the test with. It also checks the environment
// to ensure it is suitable for emulator-based tests, or skips.
func transportClientTest(t *testing.T, test func(*testing.T, string, string, storageClient)) {
	checkEmulatorEnvironment(t)

	for transport, client := range emulatorClients {
		t.Run(transport, func(t *testing.T) {
			project := fmt.Sprintf("%s-project", transport)
			bucket := fmt.Sprintf("%s-bucket-%d", transport, time.Now().Nanosecond())
			test(t, project, bucket, client)
		})
	}
}

// checkEmulatorEnvironment skips the test if the emulator environment variables
// are not set.
func checkEmulatorEnvironment(t *testing.T) {
	if !isEmulatorEnvironmentSet() {
		t.Skip("Emulator tests skipped without emulator environment variables set")
	}
}

// isEmulatorEnvironmentSet checks if the emulator environment variables are set.
func isEmulatorEnvironmentSet() bool {
	return os.Getenv("STORAGE_EMULATOR_HOST_GRPC") != "" && os.Getenv("STORAGE_EMULATOR_HOST") != ""
}

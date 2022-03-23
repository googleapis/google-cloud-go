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
	"testing"
	"time"

	"github.com/google/go-cmp/cmp"
)

var emulatorClients map[string]storageClient

func TestCreateBucketEmulated(t *testing.T) {
	checkEmulatorEnvironment(t)

	for transport, client := range emulatorClients {
		project := fmt.Sprintf("%s-project", transport)
		want := &BucketAttrs{
			Name: fmt.Sprintf("%s-bucket-%d", transport, time.Now().Nanosecond()),
		}
		got, err := client.CreateBucket(context.Background(), project, want)
		if err != nil {
			t.Fatalf("%s, %v", transport, err)
		}
		want.Location = "US"
		if diff := cmp.Diff(got.Name, want.Name); diff != "" {
			t.Errorf("%s: got(-),want(+):\n%s", transport, diff)
		}
		if diff := cmp.Diff(got.Location, want.Location); diff != "" {
			t.Errorf("%s: got(-),want(+):\n%s", transport, diff)
		}
	}
}

func TestDeleteBucketEmulated(t *testing.T) {
	checkEmulatorEnvironment(t)

	for transport, client := range emulatorClients {
		project := fmt.Sprintf("%s-project", transport)
		b := &BucketAttrs{
			Name: fmt.Sprintf("%s-bucket-%d", transport, time.Now().Nanosecond()),
		}
		// Create the bucket that will be deleted.
		_, err := client.CreateBucket(context.Background(), project, b)
		if err != nil {
			// Flag the error and continue so the Delete is skipped and the next
			// transport can run its test.
			t.Errorf("%s: %v", transport, err)
			continue
		}
		// Delete the bucket that was just created.
		err = client.DeleteBucket(context.Background(), b.Name, nil)
		if err != nil {
			t.Errorf("%s: %v", transport, err)
		}
	}
}

func TestGetBucketEmulated(t *testing.T) {
	checkEmulatorEnvironment(t)

	for transport, client := range emulatorClients {
		project := fmt.Sprintf("%s-project", transport)
		want := &BucketAttrs{
			Name: fmt.Sprintf("%s-bucket-%d", transport, time.Now().Nanosecond()),
		}
		// Create the bucket that will be retrieved.
		_, err := client.CreateBucket(context.Background(), project, want)
		if err != nil {
			// Flag the error and continue so the Get is skipped and the next
			// transport can run its test.
			t.Errorf("%s: %v", transport, err)
			continue
		}
		got, err := client.GetBucket(context.Background(), want.Name, &BucketConditions{MetagenerationMatch: 1})
		if err != nil {
			t.Errorf("%s: %v", transport, err)
			continue
		}
		if diff := cmp.Diff(got.Name, want.Name); diff != "" {
			t.Errorf("%s: got(-),want(+):\n%s", transport, diff)
		}
	}
}

func initEmulatorClients() func() error {
	noopCloser := func() error { return nil }
	if !isEmulatorEnvironmentSet() {
		return noopCloser
	}

	grpcClient, err := newGRPCStorageClient(context.Background())
	if err != nil {
		log.Fatalf("Error setting up gRPC client for emulator tests: %v", err)
		return noopCloser
	}
	httpClient, err := newHTTPStorageClient(context.Background())
	if err != nil {
		log.Fatalf("Error setting up HTTP client for emulator tests: %v", err)
		return noopCloser
	}

	emulatorClients = map[string]storageClient{
		"http": httpClient,
		"grpc": grpcClient,
	}

	return func() error {
		gerr := grpcClient.Close()
		herr := httpClient.Close()

		if gerr != nil {
			return gerr
		}
		return herr
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

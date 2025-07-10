// Copyright 2025 Google LLC
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//	http://www.apache.org/licenses/LICENSE-2.0
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
	"os"
	"strings"
	"testing"
	"time"

	"cloud.google.com/go/storage/experimental"
	"google.golang.org/api/iterator"
)

func TestEmulated_SoftDelete(t *testing.T) {
	// Skip if not running against emulator.
	host := os.Getenv("STORAGE_EMULATOR_HOST")
	if host == "" {
		t.Skip("This test must use the testbench emulator; set STORAGE_EMULATOR_HOST to run.")
	}

	projectID := "my-project-id"
	transports := []string{"http", "grpc"}
	for _, transport := range transports {
		t.Run(transport, func(t *testing.T) {
			ctx := context.Background()
			var client *Client
			var err error

			// Create client based on transport type.
			if transport == "grpc" {
				println("Using gRPC transport for test")
				client, err = NewGRPCClient(ctx, experimental.WithGRPCBidiReads())
			} else {
				println("Using HTTP transport for test")
				client, err = NewClient(ctx)
			}
			if err != nil {
				t.Fatalf("storage.NewClient: %v", err)
			}
			if client == nil {
				t.Fatal("NewClient returned nil client")
			}
			defer client.Close()

			bucketName := fmt.Sprintf("test-soft-delete-%s", bucketIDs.New())
			bucket := client.Bucket(bucketName)

			// Ensure bucket cleanup happens even if test is skipped
			defer func() {
				if err := bucket.Delete(ctx); err != nil {
					// Only log the error, don't fail the test
					t.Logf("Post-test cleanup failed: %v", err)
				}
			}()

			// Create bucket with soft delete policy.
			policy := &SoftDeletePolicy{
				RetentionDuration: time.Hour * 24 * 8,
			}
			if err := bucket.Create(ctx, projectID, &BucketAttrs{SoftDeletePolicy: policy}); err != nil {
				t.Fatalf("error creating bucket with soft delete policy set: %v", err)
			}

			// Verify bucket's soft delete policy.
			attrs, err := bucket.Attrs(ctx)
			if err != nil {
				t.Fatalf("bucket.Attrs: %v", err)
			}
			if attrs.SoftDeletePolicy == nil {
				t.Fatal("got nil soft delete policy")
			}
			if attrs.SoftDeletePolicy.RetentionDuration != policy.RetentionDuration {
				t.Errorf("mismatching retention duration; got: %v, want: %v",
					attrs.SoftDeletePolicy.RetentionDuration, policy.RetentionDuration)
			}

			// Create an object.
			objName := fmt.Sprintf("test-object-%s", bucketIDs.New())
			obj := bucket.Object(objName)

			// Write test data.
			if err := writeObject(ctx, obj, "text/plain", []byte("test data")); err != nil {
				t.Fatalf("writeObject: %v", err)
			}

			// Delete the object.
			if err := obj.Delete(ctx); err != nil {
				t.Fatalf("object.Delete: %v", err)
			}

			// List soft deleted objects.
			it := bucket.Objects(ctx, &Query{SoftDeleted: true})
			var found bool
			var objGen int64
			for {
				attrs, err := it.Next()
				if err == iterator.Done {
					break
				}
				if err != nil {
					// If we get a bad request error, it means the emulator doesn't support
					// listing soft deleted objects. In this case, we'll skip the test.
					if strings.Contains(err.Error(), "bad request is invalid") {
						t.Skip("Emulator does not support listing soft deleted objects")
						return
					}
					t.Fatalf("iterator.Next: %v", err)
				}
				if attrs.Name == objName {
					found = true
					objGen = attrs.Generation
					// Verify soft delete and hard delete times
					if attrs.SoftDeleteTime.IsZero() {
						t.Error("SoftDeleteTime should not be zero")
					}
					expectedHardDeleteTime := attrs.SoftDeleteTime.Add(policy.RetentionDuration)
					if !attrs.HardDeleteTime.Equal(expectedHardDeleteTime) {
						t.Errorf("HardDeleteTime mismatch; got: %v, want: %v",
							attrs.HardDeleteTime, expectedHardDeleteTime)
					}
				}
			}
			if !found {
				t.Error("soft deleted object not found in listing")
				return
			}

			// Get a handle to the soft deleted object with the correct generation
			softDeletedObj := obj.Generation(objGen).SoftDeleted()

			// Restore the object using the soft deleted handle
			if _, err := softDeletedObj.Restore(ctx, &RestoreOptions{}); err != nil {
				t.Fatalf("object.Restore: %v", err)
			}

			// Verify object is no longer soft deleted.
			it = bucket.Objects(ctx, &Query{SoftDeleted: true})
			for {
				attrs, err := it.Next()
				if err == iterator.Done {
					break
				}
				if err != nil {
					t.Fatalf("iterator.Next: %v", err)
				}
				if attrs.Name == objName {
					t.Error("object should not be soft deleted after restore")
				}
			}
		})
	}
}

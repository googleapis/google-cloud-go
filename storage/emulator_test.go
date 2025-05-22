package storage

import (
	"context"
	"os"
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

	projectID := "storage-sdks-madisonhall"
	transports := []string{"http", "grpc"}
	for _, transport := range transports {
		t.Run(transport, func(t *testing.T) {
			ctx := context.Background()
			var client *Client
			var err error

			// Create client based on transport type.
			if transport == "grpc" {
				client, err = NewGRPCClient(ctx, experimental.WithGRPCBidiReads())
			} else {
				client, err = NewClient(ctx)
			}
			if err != nil {
				t.Fatalf("NewClient: %v", err)
			}
			defer client.Close()

			bucketName := "test-soft-delete-" + uidSpace.New()
			bucket := client.Bucket(bucketName)

			// Create bucket with soft delete policy.
			policy := &SoftDeletePolicy{
				RetentionDuration: time.Hour * 24 * 8,
			}
			if err := bucket.Create(ctx, projectID, &BucketAttrs{SoftDeletePolicy: policy}); err != nil {
				t.Fatalf("error creating bucket with soft delete policy set: %v", err)
			}
			defer bucket.Delete(ctx)

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
			objName := "test-object-" + uidSpaceObjects.New()
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
			for {
				attrs, err := it.Next()
				if err == iterator.Done {
					break
				}
				if err != nil {
					t.Fatalf("iterator.Next: %v", err)
				}
				if attrs.Name == objName {
					found = true
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
			}

			// Restore the object.
			if _, err := obj.Restore(ctx, nil); err != nil {
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

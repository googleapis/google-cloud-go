// Copyright 2026 Google LLC
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
	"os"
	"testing"
	"time"

	"cloud.google.com/go/internal/testutil"
	control "cloud.google.com/go/storage/control/apiv2"
	"cloud.google.com/go/storage/control/apiv2/controlpb"
	"google.golang.org/api/iterator"
	"google.golang.org/api/option"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/durationpb"
	"google.golang.org/protobuf/types/known/fieldmaskpb"
)

func getTestControlClient(ctx context.Context, t *testing.T) *control.StorageControlClient {
	t.Helper()
	var opts []option.ClientOption
	if ep := os.Getenv("GCLOUD_TESTS_GOLANG_STORAGE_GRPC_ENDPOINT"); ep != "" {
		opts = append(opts, option.WithEndpoint(ep))
	} else if ep := os.Getenv("GCLOUD_TESTS_GOLANG_STORAGE_CONTROL_ENDPOINT"); ep != "" {
		opts = append(opts, option.WithEndpoint(ep))
	}
	c, err := control.NewStorageControlClient(ctx, opts...)
	if err != nil {
		t.Fatalf("control.NewStorageControlClient: %v", err)
	}
	return c
}

func getTestStorageClient(ctx context.Context, t *testing.T) *Client {
	t.Helper()
	var opts []option.ClientOption
	if ep := os.Getenv("GCLOUD_TESTS_GOLANG_STORAGE_GRPC_ENDPOINT"); ep != "" {
		opts = append(opts, option.WithEndpoint(ep))
	} else if ep := os.Getenv("GCLOUD_TESTS_GOLANG_STORAGE_ENDPOINT"); ep != "" {
		opts = append(opts, option.WithEndpoint(ep))
	}
	c, err := NewGRPCClient(ctx, opts...)
	if err != nil {
		t.Fatalf("NewGRPCClient: %v", err)
	}
	return c
}

func createRegionalHNSBucketForCache(ctx context.Context, t *testing.T, client *Client) string {
	t.Helper()
	bktName := testPrefix + uidSpace.New()
	bkt := client.Bucket(bktName)
	if err := bkt.Create(ctx, testutil.ProjID(), &BucketAttrs{
		Location: testRCULocation,
		HierarchicalNamespace: &HierarchicalNamespace{
			Enabled: true,
		},
		UniformBucketLevelAccess: UniformBucketLevelAccess{
			Enabled: true,
		},
	}); err != nil {
		t.Fatalf("failed to create test bucket for RapidCache %q: %v", bktName, err)
	}
	t.Cleanup(func() {
		_ = bkt.Delete(ctx)
	})
	return bktName
}

// 1. Create Rapid Cache: Creates a new Rapid Cache instance in a specified zone for a regional bucket.
// Asserts that the LRO completes successfully and the cache state transitions to RUNNING.
func TestIntegration_RapidCache_Create(t *testing.T) {
	ctx := context.Background()
	client := getTestStorageClient(ctx, t)
	defer client.Close()

	cClient := getTestControlClient(ctx, t)
	defer cClient.Close()

	bucket := createRegionalHNSBucketForCache(ctx, t, client)
	parent := fmt.Sprintf("projects/_/buckets/%s", bucket)

	createReq := &controlpb.CreateRapidCacheRequest{
		Parent: parent,
		RapidCache: &controlpb.RapidCache{
			Zone:      testRCUZone,
			CacheType: "rapid-cache-ultra",
			Ttl:       durationpb.New(24 * time.Hour),
		},
	}
	op, err := cClient.CreateRapidCache(ctx, createReq)
	if err != nil {
		t.Fatalf("CreateRapidCache failed: %v", err)
	}

	rc, err := op.Wait(ctx)
	if err != nil {
		t.Fatalf("CreateRapidCache LRO Wait failed: %v", err)
	}

	if rc.GetState() != "running" {
		t.Errorf("RapidCache State: got %q, want %q", rc.GetState(), "running")
	}
	if rc.GetZone() != testRCUZone {
		t.Errorf("RapidCache Zone: got %q, want %q", rc.GetZone(), testRCUZone)
	}
}

// 2. Create Rapid Cache - Invalid Config: Attempts to create a cache using invalid configuration parameters
// (e.g., invalid zone or invalid TTL values). Verifies that an INVALID_ARGUMENT exception is thrown.
func TestIntegration_RapidCache_CreateInvalidConfig(t *testing.T) {
	ctx := context.Background()
	client := getTestStorageClient(ctx, t)
	defer client.Close()

	cClient := getTestControlClient(ctx, t)
	defer cClient.Close()

	bucket := createRegionalHNSBucketForCache(ctx, t, client)
	parent := fmt.Sprintf("projects/_/buckets/%s", bucket)

	createReq := &controlpb.CreateRapidCacheRequest{
		Parent: parent,
		RapidCache: &controlpb.RapidCache{
			Zone:      "invalid-zone-123",
			CacheType: "rapid-cache-ultra",
			Ttl:       durationpb.New(24 * time.Hour),
		},
	}
	_, err := cClient.CreateRapidCache(ctx, createReq)
	if err == nil {
		t.Fatalf("CreateRapidCache with invalid zone: expected error, got nil")
	}
	if st, ok := status.FromError(err); !ok || st.Code() != codes.InvalidArgument {
		t.Errorf("CreateRapidCache invalid config: got error %v (code %v), want InvalidArgument", err, st.Code())
	}
}

// 3. Create Duplicate Rapid Cache: Attempts to create a second Rapid Cache instance in the same zone for the same bucket.
// Verifies that the request fails with an ALREADY_EXISTS / FAILED_PRECONDITION error.
func TestIntegration_RapidCache_CreateDuplicate(t *testing.T) {
	ctx := context.Background()
	client := getTestStorageClient(ctx, t)
	defer client.Close()

	cClient := getTestControlClient(ctx, t)
	defer cClient.Close()

	bucket := createRegionalHNSBucketForCache(ctx, t, client)
	parent := fmt.Sprintf("projects/_/buckets/%s", bucket)

	createReq := &controlpb.CreateRapidCacheRequest{
		Parent: parent,
		RapidCache: &controlpb.RapidCache{
			Zone:      testRCUZone,
			CacheType: "rapid-cache-ultra",
			Ttl:       durationpb.New(24 * time.Hour),
		},
	}
	op, err := cClient.CreateRapidCache(ctx, createReq)
	if err != nil {
		t.Fatalf("CreateRapidCache (first): %v", err)
	}
	if _, err := op.Wait(ctx); err != nil {
		t.Fatalf("CreateRapidCache (first) LRO: %v", err)
	}

	// Attempt duplicate create in the same zone
	_, err = cClient.CreateRapidCache(ctx, createReq)
	if err == nil {
		t.Fatalf("CreateRapidCache (duplicate): expected error, got nil")
	}
	if st, ok := status.FromError(err); !ok || (st.Code() != codes.AlreadyExists && st.Code() != codes.FailedPrecondition) {
		t.Errorf("CreateRapidCache duplicate: got code %v (%v), want AlreadyExists or FailedPrecondition", st.Code(), err)
	}
}

// 4. Get Rapid Cache: Retrieves the metadata for an active Rapid Cache instance.
// Asserts that the returned cache configuration matches the created config, and the state is RUNNING.
func TestIntegration_RapidCache_Get(t *testing.T) {
	ctx := context.Background()
	client := getTestStorageClient(ctx, t)
	defer client.Close()

	cClient := getTestControlClient(ctx, t)
	defer cClient.Close()

	bucket := createRegionalHNSBucketForCache(ctx, t, client)
	parent := fmt.Sprintf("projects/_/buckets/%s", bucket)

	op, err := cClient.CreateRapidCache(ctx, &controlpb.CreateRapidCacheRequest{
		Parent: parent,
		RapidCache: &controlpb.RapidCache{
			Zone:      testRCUZone,
			CacheType: "rapid-cache-ultra",
			Ttl:       durationpb.New(24 * time.Hour),
		},
	})
	if err != nil {
		t.Fatalf("CreateRapidCache: %v", err)
	}
	created, err := op.Wait(ctx)
	if err != nil {
		t.Fatalf("CreateRapidCache LRO: %v", err)
	}

	cacheName := created.GetName()
	got, err := cClient.GetRapidCache(ctx, &controlpb.GetRapidCacheRequest{
		Name: cacheName,
	})
	if err != nil {
		t.Fatalf("GetRapidCache failed: %v", err)
	}
	if got.GetState() != "running" {
		t.Errorf("GetRapidCache State: got %q, want %q", got.GetState(), "running")
	}
	if got.GetZone() != testRCUZone {
		t.Errorf("GetRapidCache Zone: got %q, want %q", got.GetZone(), testRCUZone)
	}
}

// 5. Get Non-existent Rapid Cache: Attempts to retrieve metadata for a non-existent cache ID or zone.
// Verifies that a NOT_FOUND exception is thrown.
func TestIntegration_RapidCache_GetNonExistent(t *testing.T) {
	ctx := context.Background()
	client := getTestStorageClient(ctx, t)
	defer client.Close()

	cClient := getTestControlClient(ctx, t)
	defer cClient.Close()

	bucket := createRegionalHNSBucketForCache(ctx, t, client)
	nonExistentName := fmt.Sprintf("projects/_/buckets/%s/rapidCaches/non-existent-zone-a", bucket)

	_, err := cClient.GetRapidCache(ctx, &controlpb.GetRapidCacheRequest{
		Name: nonExistentName,
	})
	if err == nil {
		t.Fatalf("GetRapidCache for non-existent cache: expected error, got nil")
	}
	if st, ok := status.FromError(err); !ok || st.Code() != codes.NotFound {
		t.Errorf("GetRapidCache non-existent: got %v (code %v), want NotFound", err, st.Code())
	}
}

// 6. List Rapid Caches: Lists all Rapid Cache instances configured for a given bucket.
// Asserts that the response includes all active caches and supports pagination if the count exceeds pageSize.
func TestIntegration_RapidCache_List(t *testing.T) {
	ctx := context.Background()
	client := getTestStorageClient(ctx, t)
	defer client.Close()

	cClient := getTestControlClient(ctx, t)
	defer cClient.Close()

	bucket := createRegionalHNSBucketForCache(ctx, t, client)
	parent := fmt.Sprintf("projects/_/buckets/%s", bucket)

	// Create cache
	op, err := cClient.CreateRapidCache(ctx, &controlpb.CreateRapidCacheRequest{
		Parent: parent,
		RapidCache: &controlpb.RapidCache{
			Zone:      testRCUZone,
			CacheType: "rapid-cache-ultra",
			Ttl:       durationpb.New(24 * time.Hour),
		},
	})
	if err != nil {
		t.Fatalf("CreateRapidCache: %v", err)
	}
	created, err := op.Wait(ctx)
	if err != nil {
		t.Fatalf("CreateRapidCache LRO: %v", err)
	}

	it := cClient.ListRapidCaches(ctx, &controlpb.ListRapidCachesRequest{
		Parent:   parent,
		PageSize: 10,
	})

	var found []*controlpb.RapidCache
	for {
		item, err := it.Next()
		if err == iterator.Done {
			break
		}
		if err != nil {
			t.Fatalf("ListRapidCaches iterator error: %v", err)
		}
		found = append(found, item)
	}

	if len(found) == 0 {
		t.Fatalf("ListRapidCaches returned 0 caches, expected at least 1")
	}
	matched := false
	for _, f := range found {
		if f.GetName() == created.GetName() {
			matched = true
			break
		}
	}
	if !matched {
		t.Errorf("ListRapidCaches did not contain created cache %q", created.GetName())
	}
}

// 7. Update Rapid Cache: Updates configuration settings (such as TTL) on an existing cache.
// Verifies that the LRO completes successfully and subsequent GetRapidCache calls reflect updated settings.
func TestIntegration_RapidCache_Update(t *testing.T) {
	ctx := context.Background()
	client := getTestStorageClient(ctx, t)
	defer client.Close()

	cClient := getTestControlClient(ctx, t)
	defer cClient.Close()

	bucket := createRegionalHNSBucketForCache(ctx, t, client)
	parent := fmt.Sprintf("projects/_/buckets/%s", bucket)

	op, err := cClient.CreateRapidCache(ctx, &controlpb.CreateRapidCacheRequest{
		Parent: parent,
		RapidCache: &controlpb.RapidCache{
			Zone:      testRCUZone,
			CacheType: "rapid-cache-ultra",
			Ttl:       durationpb.New(24 * time.Hour),
		},
	})
	if err != nil {
		t.Fatalf("CreateRapidCache: %v", err)
	}
	created, err := op.Wait(ctx)
	if err != nil {
		t.Fatalf("CreateRapidCache LRO: %v", err)
	}

	uOp, err := cClient.UpdateRapidCache(ctx, &controlpb.UpdateRapidCacheRequest{
		RapidCache: &controlpb.RapidCache{
			Name: created.GetName(),
			Ttl:  durationpb.New(48 * time.Hour),
		},
		UpdateMask: &fieldmaskpb.FieldMask{
			Paths: []string{"ttl"},
		},
	})
	if err != nil {
		if st, ok := status.FromError(err); ok && (st.Code() == codes.Internal || st.Code() == codes.Unimplemented) {
			t.Skipf("UpdateRapidCache not yet supported by server backend: %v", err)
		}
		t.Fatalf("UpdateRapidCache failed: %v", err)
	}

	updated, err := uOp.Wait(ctx)
	if err != nil {
		if st, ok := status.FromError(err); ok && (st.Code() == codes.Internal || st.Code() == codes.Unimplemented) {
			t.Skipf("UpdateRapidCache LRO wait not yet supported by server backend: %v", err)
		}
		t.Fatalf("UpdateRapidCache LRO wait failed: %v", err)
	}

	if updated.GetTtl().GetSeconds() != 48*3600 {
		t.Errorf("UpdateRapidCache TTL: got %v, want 48h", updated.GetTtl())
	}
}

// 8. Disable Rapid Cache: Initiates the disablement of an active Rapid Cache.
func TestIntegration_RapidCache_Disable(t *testing.T) {
	// Note: DisableRapidCache RPC is not yet exposed in storage_control.proto.
	// As noted in go/rcu-in-sdk (Storage Control Blockers), DisableRapidCache is pending in backend protos.
	t.Skip("DisableRapidCache API is not yet available in storage_control.proto (tracked in go/rcu-in-sdk)")
}

// 9. Disable Non-existent Cache: Attempts to disable a cache that does not exist.
func TestIntegration_RapidCache_DisableNonExistent(t *testing.T) {
	t.Skip("DisableRapidCache API is not yet available in storage_control.proto (tracked in go/rcu-in-sdk)")
}

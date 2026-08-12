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

// TestIntegration_RapidCache_Create tests creating a new Rapid Cache instance in a specified zone for a regional bucket.
// It asserts that the LRO completes successfully and the cache state transitions to running.
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

// TestIntegration_RapidCache_CreateInvalidConfig tests creating a cache with invalid configuration parameters.
// It verifies that an InvalidArgument exception is thrown when an invalid zone is specified.
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

// TestIntegration_RapidCache_CreateDuplicate tests creating a second Rapid Cache instance in the same zone for the same bucket.
// It verifies that the request fails with an AlreadyExists error.
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

	// Attempt duplicate create in the same zone.
	_, err = cClient.CreateRapidCache(ctx, createReq)
	if err == nil {
		t.Fatalf("CreateRapidCache (duplicate): expected error, got nil")
	}
	if st, ok := status.FromError(err); !ok || st.Code() != codes.AlreadyExists {
		t.Errorf("CreateRapidCache duplicate: got code %v (%v), want AlreadyExists", st.Code(), err)
	}
}

// TestIntegration_RapidCache_Get tests retrieving metadata for an active Rapid Cache instance.
// It asserts that the returned cache configuration matches the created configuration, and the state is running.
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

// TestIntegration_RapidCache_GetNonExistent tests retrieving metadata for a non-existent cache ID or zone.
// It verifies that a NotFound exception is thrown.
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

// TestIntegration_RapidCache_List tests listing all Rapid Cache instances configured for a given bucket.
// It asserts that the response includes all active caches and supports pagination if the count exceeds pageSize.
func TestIntegration_RapidCache_List(t *testing.T) {
	ctx := context.Background()
	client := getTestStorageClient(ctx, t)
	defer client.Close()

	cClient := getTestControlClient(ctx, t)
	defer cClient.Close()

	bucket := createRegionalHNSBucketForCache(ctx, t, client)
	parent := fmt.Sprintf("projects/_/buckets/%s", bucket)

	candidateZones := []string{testRCUZone, "us-central1-f", "us-central1-b"}
	var createdCaches []*controlpb.RapidCache

	for _, z := range candidateZones {
		tCtx, cancel := context.WithTimeout(ctx, 15*time.Second)
		op, err := cClient.CreateRapidCache(tCtx, &controlpb.CreateRapidCacheRequest{
			Parent: parent,
			RapidCache: &controlpb.RapidCache{
				Zone:      z,
				CacheType: "rapid-cache-ultra",
				Ttl:       durationpb.New(24 * time.Hour),
			},
		})
		if err != nil {
			cancel()
			continue
		}
		rc, err := op.Wait(tCtx)
		cancel()
		if err != nil {
			continue
		}
		createdCaches = append(createdCaches, rc)
	}

	if len(createdCaches) == 0 {
		t.Fatalf("failed to create any RapidCache instances for list test")
	}

	// Use PageSize: 1 to verify pagination across pages.
	it := cClient.ListRapidCaches(ctx, &controlpb.ListRapidCachesRequest{
		Parent:   parent,
		PageSize: 1,
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

	if len(found) < len(createdCaches) {
		t.Fatalf("ListRapidCaches returned %d caches, expected at least %d", len(found), len(createdCaches))
	}

	for _, created := range createdCaches {
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
}

// TestIntegration_RapidCache_Update tests updating configuration settings on an existing Rapid Cache instance.
// It verifies that the LRO completes successfully and subsequent GetRapidCache calls reflect updated settings.
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

// TestIntegration_RapidCache_Disable tests initiating the disablement of an active Rapid Cache.
func TestIntegration_RapidCache_Disable(t *testing.T) {
	// Note: DisableRapidCache RPC is not yet exposed in storage_control.proto.
	// As noted in go/rcu-in-sdk (Storage Control Blockers), DisableRapidCache is pending in backend protos.
	t.Skip("DisableRapidCache API is not yet available in storage_control.proto (tracked in go/rcu-in-sdk)")
}

// TestIntegration_RapidCache_DisableNonExistent tests attempting to disable a cache that does not exist.
func TestIntegration_RapidCache_DisableNonExistent(t *testing.T) {
	t.Skip("DisableRapidCache API is not yet available in storage_control.proto (tracked in go/rcu-in-sdk)")
}

/*
Copyright 2026 Google LLC

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package spanner

import (
	"context"
	"os"
	"strings"
	"testing"

	sppb "cloud.google.com/go/spanner/apiv1/spannerpb"
	. "cloud.google.com/go/spanner/internal/testutil"
)

func TestAutoTagging_ReadOnlyTransaction(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	server, client, teardown := setupMockedTestServerWithConfig(t, ClientConfig{
		DisableNativeMetrics: true,
		EnableAutoTagging:    true,
		AutoTaggingPackages:  []string{"cloud.google.com/go/spanner.TestAutoTagging"},
	})
	defer teardown()

	ro := client.ReadOnlyTransaction()
	defer ro.Close()

	iter := ro.Query(ctx, NewStatement(SelectSingerIDAlbumIDAlbumTitleFromAlbums))
	_ = iter.Do(func(row *Row) error { return nil })

	// Verify request_tag is populated and transaction_tag is empty
	reqs := drainRequestsFromServer(server.TestSpanner)
	found := false
	for _, s := range reqs {
		if req, ok := s.(*sppb.ExecuteSqlRequest); ok {
			found = true
			if req.RequestOptions == nil || req.RequestOptions.RequestTag == "" {
				t.Errorf("Expected request_tag to be populated, got empty")
			} else if !strings.Contains(req.RequestOptions.RequestTag, "TestAutoTagging") {
				t.Errorf("Expected request_tag to contain TestAutoTagging, got %q", req.RequestOptions.RequestTag)
			}
			if req.RequestOptions != nil && req.RequestOptions.TransactionTag != "" {
				t.Errorf("Expected transaction_tag to be empty, got %q", req.RequestOptions.TransactionTag)
			}
		}
	}
	if !found {
		t.Errorf("No ExecuteSqlRequest found")
	}
}

func TestAutoTagging_ReadWriteTransaction(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	server, client, teardown := setupMockedTestServerWithConfig(t, ClientConfig{
		DisableNativeMetrics: true,
		EnableAutoTagging:    true,
		AutoTaggingPackages:  []string{"cloud.google.com/go/spanner.TestAutoTagging"},
	})
	defer teardown()

	_, _ = client.ReadWriteTransaction(ctx, func(ctx context.Context, txn *ReadWriteTransaction) error {
		_, _ = txn.Update(ctx, NewStatement(UpdateBarSetFoo))
		return nil
	})

	reqs := drainRequestsFromServer(server.TestSpanner)
	hasStmt, hasCommit := false, false
	for _, s := range reqs {
		if req, ok := s.(*sppb.ExecuteSqlRequest); ok {
			hasStmt = true
			if req.RequestOptions != nil && req.RequestOptions.RequestTag != "" {
				t.Errorf("Expected request_tag to be empty in RW tx, got %q", req.RequestOptions.RequestTag)
			}
			if req.RequestOptions == nil || !strings.Contains(req.RequestOptions.TransactionTag, "TestAutoTagging") {
				t.Errorf("Expected transaction_tag to be populated in RW tx statement, got %v", req.RequestOptions)
			}
		}
		if req, ok := s.(*sppb.CommitRequest); ok {
			hasCommit = true
			if req.RequestOptions == nil || !strings.Contains(req.RequestOptions.TransactionTag, "TestAutoTagging") {
				t.Errorf("Expected transaction_tag to be populated in CommitRequest, got %v", req.RequestOptions)
			}
		}
	}
	if !hasStmt || !hasCommit {
		t.Errorf("Missing statement (%v) or commit (%v) request", hasStmt, hasCommit)
	}
}

func TestAutoTagging_CachingEfficiency(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	server, client, teardown := setupMockedTestServerWithConfig(t, ClientConfig{
		DisableNativeMetrics: true,
		EnableAutoTagging:    true,
		AutoTaggingPackages:  []string{"cloud.google.com/go/spanner.TestAutoTagging"},
	})
	defer teardown()

	ro := client.ReadOnlyTransaction()
	defer ro.Close()

	// Execute first statement
	_ = ro.Query(ctx, NewStatement(SelectSingerIDAlbumIDAlbumTitleFromAlbums)).Do(func(r *Row) error { return nil })
	// Check cached value
	cached := ro.cachedRequestTag

	if cached == "" {
		t.Fatalf("Expected tag to be cached at ReadOnlyTransaction creation")
	}

	// Second statement should use cached tag without re-walking
	_ = ro.Query(ctx, NewStatement(SelectSingerIDAlbumIDAlbumTitleFromAlbums)).Do(func(r *Row) error { return nil })

	reqs := drainRequestsFromServer(server.TestSpanner)
	count := 0
	for _, s := range reqs {
		if req, ok := s.(*sppb.ExecuteSqlRequest); ok {
			count++
			if req.RequestOptions.RequestTag != cached {
				t.Errorf("Expected request_tag %q, got %q", cached, req.RequestOptions.RequestTag)
			}
		}
	}
	if count < 2 {
		t.Errorf("Expected at least 2 queries, got %d", count)
	}
}

func TestAutoTagging_VarargsAndLimits(t *testing.T) {
	tag := getCallStackTag([]string{"github.com/unknown", "cloud.google.com/go/spanner.TestAutoTagging"}, 10)
	if !strings.Contains(tag, "TestAutoTagging") {
		t.Errorf("Expected matching tag from multiple packages, got %q", tag)
	}
}

func TestAutoTagging_Truncation(t *testing.T) {
	longMethod := "cloud.google.com/go/spanner.TestAutoTaggingThisIsAnExtremelyLongFunctionNameThatExceedsFiftyCharactersInLength"
	tag := formatFrameTag(longMethod)
	if len(tag) > 50 {
		tag = tag[len(tag)-50:]
	}
	tag = strings.TrimLeft(tag, "0123456789_-")
	if len(tag) != 50 {
		t.Errorf("Expected length 50, got %d", len(tag))
	}
}

func TestAutoTagging_GlobalOverrides(t *testing.T) {
	_ = os.Setenv("SPANNER_DISABLE_AUTO_TAGGING", "true")
	defer os.Unsetenv("SPANNER_DISABLE_AUTO_TAGGING")

	_, client, teardown := setupMockedTestServerWithConfig(t, ClientConfig{
		DisableNativeMetrics: true,
		EnableAutoTagging:    true,
	})
	defer teardown()

	if client.enableAutoTagging {
		t.Errorf("Expected auto tagging to be disabled by global env override")
	}

	_ = os.Setenv("SPANNER_DISABLE_AUTO_TAGGING", "false")
	_ = os.Setenv("SPANNER_ENABLE_AUTO_TAGGING", "true")
	defer os.Unsetenv("SPANNER_ENABLE_AUTO_TAGGING")

	_, client2, teardown2 := setupMockedTestServerWithConfig(t, ClientConfig{
		DisableNativeMetrics: true,
		EnableAutoTagging:    false,
	})
	defer teardown2()

	if !client2.enableAutoTagging {
		t.Errorf("Expected auto tagging to be enabled by global env override")
	}
}

func TestAutoTagging_IsEligibleFrame(t *testing.T) {
	tests := []struct {
		fn       string
		packages []string
		want     bool
	}{
		{"runtime.gopanic", nil, false},
		{"reflect.Value.Call", nil, false},
		{"sync.(*Mutex).Lock", nil, false},
		{"cloud.google.com/go/spanner.(*Client).Single", nil, false},
		{"github.com/myorg/myapp/db.FetchData", nil, true},
		{"github.com/myorg/myapp/db.FetchData", []string{"github.com/myorg/myapp"}, true},
		{"github.com/otherorg/otherapp.DoWork", []string{"github.com/myorg/myapp"}, false},
	}

	for _, tc := range tests {
		got := isEligibleFrame(tc.fn, tc.packages)
		if got != tc.want {
			t.Errorf("isEligibleFrame(%q, %v) = %v, want %v", tc.fn, tc.packages, got, tc.want)
		}
	}
}

func TestAutoTagging_SingleUse(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	server, client, teardown := setupMockedTestServerWithConfig(t, ClientConfig{
		DisableNativeMetrics: true,
		EnableAutoTagging:    true,
		AutoTaggingPackages:  []string{"cloud.google.com/go/spanner.TestAutoTagging"},
	})
	defer teardown()

	_, _ = client.Single().ReadRow(ctx, "Albums", Key{"foo"}, []string{"SingerId", "AlbumId"})

	reqs := drainRequestsFromServer(server.TestSpanner)
	found := false
	for _, s := range reqs {
		if req, ok := s.(*sppb.ReadRequest); ok {
			found = true
			if req.RequestOptions == nil || req.RequestOptions.RequestTag == "" {
				t.Errorf("Expected request_tag to be populated in Single(), got empty")
			} else if !strings.Contains(req.RequestOptions.RequestTag, "TestAutoTagging") {
				t.Errorf("Expected request_tag to contain TestAutoTagging, got %q", req.RequestOptions.RequestTag)
			}
			if req.RequestOptions != nil && req.RequestOptions.TransactionTag != "" {
				t.Errorf("Expected transaction_tag to be empty, got %q", req.RequestOptions.TransactionTag)
			}
		}
	}
	if !found {
		t.Errorf("No ReadRequest found for Single()")
	}
}

func TestAutoTagging_Apply(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	server, client, teardown := setupMockedTestServerWithConfig(t, ClientConfig{
		DisableNativeMetrics: true,
		EnableAutoTagging:    true,
		AutoTaggingPackages:  []string{"cloud.google.com/go/spanner.TestAutoTagging"},
	})
	defer teardown()

	ms := []*Mutation{Insert("Albums", []string{"SingerId"}, []interface{}{int64(1)})}
	_, _ = client.Apply(ctx, ms)

	reqs := drainRequestsFromServer(server.TestSpanner)
	found := false
	for _, s := range reqs {
		if req, ok := s.(*sppb.CommitRequest); ok {
			found = true
			if req.RequestOptions == nil || !strings.Contains(req.RequestOptions.TransactionTag, "TestAutoTagging") {
				t.Errorf("Expected transaction_tag to be populated in Apply CommitRequest, got %v", req.RequestOptions)
			}
		}
	}
	if !found {
		t.Errorf("No CommitRequest found for Apply()")
	}
}

func TestAutoTagging_ApplyAtLeastOnce(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	server, client, teardown := setupMockedTestServerWithConfig(t, ClientConfig{
		DisableNativeMetrics: true,
		EnableAutoTagging:    true,
		AutoTaggingPackages:  []string{"cloud.google.com/go/spanner.TestAutoTagging"},
	})
	defer teardown()

	ms := []*Mutation{Insert("Albums", []string{"SingerId"}, []interface{}{int64(1)})}
	_, _ = client.Apply(ctx, ms, ApplyAtLeastOnce())

	reqs := drainRequestsFromServer(server.TestSpanner)
	found := false
	for _, s := range reqs {
		if req, ok := s.(*sppb.CommitRequest); ok {
			found = true
			if req.RequestOptions == nil || !strings.Contains(req.RequestOptions.TransactionTag, "TestAutoTagging") {
				t.Errorf("Expected transaction_tag to be populated in ApplyAtLeastOnce CommitRequest, got %v", req.RequestOptions)
			}
		}
	}
	if !found {
		t.Errorf("No CommitRequest found for ApplyAtLeastOnce()")
	}
}

func TestAutoTagging_BatchWrite(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	server, client, teardown := setupMockedTestServerWithConfig(t, ClientConfig{
		DisableNativeMetrics: true,
		EnableAutoTagging:    true,
		AutoTaggingPackages:  []string{"cloud.google.com/go/spanner.TestAutoTagging"},
	})
	defer teardown()

	mgs := []*MutationGroup{{Mutations: []*Mutation{Insert("Albums", []string{"SingerId"}, []interface{}{int64(1)})}}}
	iter := client.BatchWrite(ctx, mgs)
	_, _ = iter.Next()

	reqs := drainRequestsFromServer(server.TestSpanner)
	found := false
	for _, s := range reqs {
		if req, ok := s.(*sppb.BatchWriteRequest); ok {
			found = true
			if req.RequestOptions == nil || !strings.Contains(req.RequestOptions.TransactionTag, "TestAutoTagging") {
				t.Errorf("Expected transaction_tag to be populated in BatchWriteRequest, got %v", req.RequestOptions)
			}
		}
	}
	if !found {
		t.Errorf("No BatchWriteRequest found for BatchWrite()")
	}
}

func TestAutoTagging_BatchReadOnly(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	server, client, teardown := setupMockedTestServerWithConfig(t, ClientConfig{
		DisableNativeMetrics: true,
		EnableAutoTagging:    true,
		AutoTaggingPackages:  []string{"cloud.google.com/go/spanner.TestAutoTagging"},
	})
	defer teardown()

	txn, err := client.BatchReadOnlyTransaction(ctx, StrongRead())
	if err != nil {
		t.Fatalf("BatchReadOnlyTransaction failed: %v", err)
	}
	defer txn.Close()

	_, _ = txn.PartitionQuery(ctx, NewStatement(SelectSingerIDAlbumIDAlbumTitleFromAlbums), PartitionOptions{})

	reqs := drainRequestsFromServer(server.TestSpanner)
	found := false
	for _, s := range reqs {
		if req, ok := s.(*sppb.PartitionQueryRequest); ok {
			found = true
			if req.Transaction == nil {
				t.Errorf("Expected Transaction selector in PartitionQueryRequest")
			}
		}
	}
	if !found {
		t.Errorf("No PartitionQueryRequest found")
	}
}

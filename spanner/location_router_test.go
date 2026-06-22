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
	"bytes"
	"context"
	"testing"
	"time"

	sppb "cloud.google.com/go/spanner/apiv1/spannerpb"
	structpb "google.golang.org/protobuf/types/known/structpb"
)

func waitForAsyncRoutingUpdate(t *testing.T, cond func() bool) {
	t.Helper()
	waitForCondition(t, time.Second, cond)
}

func TestChannelFinder_IsMaterialUpdate(t *testing.T) {
	finder := newChannelFinder(nil)

	for _, tc := range []struct {
		name   string
		update *sppb.CacheUpdate
		want   bool
	}{
		{name: "nil", update: nil, want: false},
		{name: "empty", update: &sppb.CacheUpdate{}, want: false},
		{name: "database only", update: &sppb.CacheUpdate{DatabaseId: 1}, want: false},
		{
			name:   "group",
			update: &sppb.CacheUpdate{Group: []*sppb.Group{{GroupUid: 1}}},
			want:   true,
		},
		{
			name:   "range",
			update: &sppb.CacheUpdate{Range: []*sppb.Range{{GroupUid: 1}}},
			want:   true,
		},
		{
			name:   "recipes",
			update: &sppb.CacheUpdate{KeyRecipes: &sppb.RecipeList{Recipe: []*sppb.KeyRecipe{{}}}},
			want:   true,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := finder.isMaterialUpdate(tc.update); got != tc.want {
				t.Fatalf("isMaterialUpdate() = %v, want %v", got, tc.want)
			}
		})
	}
}

func TestChannelFinder_ShouldProcessUpdate(t *testing.T) {
	finder := newChannelFinder(nil)
	finder.databaseID.Store(7)

	for _, tc := range []struct {
		name   string
		update *sppb.CacheUpdate
		want   bool
	}{
		{name: "nil", update: nil, want: false},
		{name: "empty", update: &sppb.CacheUpdate{}, want: false},
		{name: "same database only", update: &sppb.CacheUpdate{DatabaseId: 7}, want: false},
		{name: "different database only", update: &sppb.CacheUpdate{DatabaseId: 8}, want: true},
		{
			name: "material same database",
			update: &sppb.CacheUpdate{
				DatabaseId: 7,
				Group:      []*sppb.Group{{GroupUid: 1}},
			},
			want: true,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := finder.shouldProcessUpdate(tc.update); got != tc.want {
				t.Fatalf("shouldProcessUpdate() = %v, want %v", got, tc.want)
			}
		})
	}
}

func TestChannelFinder_UpdateAsyncCoalescesUpdates(t *testing.T) {
	finder := newChannelFinder(nil)

	recipeUpdate := &sppb.CacheUpdate{
		DatabaseId: 1,
		KeyRecipes: &sppb.RecipeList{
			SchemaGeneration: []byte{1, 2},
			Recipe: []*sppb.KeyRecipe{
				{
					Target: &sppb.KeyRecipe_TableName{TableName: "T"},
					Part: []*sppb.KeyRecipe_Part{
						{Tag: 7},
					},
				},
			},
		},
	}
	rangeUpdate := &sppb.CacheUpdate{
		DatabaseId: 1,
		Range: []*sppb.Range{
			{
				StartKey:   []byte("a"),
				LimitKey:   []byte("z"),
				GroupUid:   9,
				SplitId:    11,
				Generation: []byte{3},
			},
		},
		Group: []*sppb.Group{
			{
				GroupUid: 9,
				Tablets: []*sppb.Tablet{
					{TabletUid: 9, ServerAddress: "replica-1"},
				},
			},
		},
	}

	var flush func()
	finder.setFlushSchedulerForTest(func(_ time.Duration, fn func()) {
		flush = fn
	})
	finder.updateAsync(recipeUpdate)
	finder.updateAsync(rangeUpdate)
	if flush == nil {
		t.Fatal("expected coalesced flush to be scheduled")
	}
	flush()

	if finder.databaseID.Load() != 1 || finder.rangeCache.size() != 1 {
		t.Fatalf("expected coalesced update to apply database and range state")
	}
	hint := &sppb.RoutingHint{}
	finder.recipeCache.applySchemaGeneration(hint)
	if !bytes.Equal(hint.GetSchemaGeneration(), []byte{1, 2}) {
		t.Fatalf("expected coalesced update to apply recipe state")
	}
}

func TestIsExperimentalLocationAPIEnabled(t *testing.T) {
	t.Setenv(experimentalLocationAPIEnvVar, "true")
	if !isExperimentalLocationAPIEnabled() {
		t.Fatal("expected feature flag to be enabled for true")
	}

	t.Setenv(experimentalLocationAPIEnvVar, "false")
	if isExperimentalLocationAPIEnabled() {
		t.Fatal("expected feature flag to be disabled for false")
	}

	t.Setenv(experimentalLocationAPIEnvVar, "not-a-bool")
	if isExperimentalLocationAPIEnabled() {
		t.Fatal("expected invalid boolean env var to be treated as disabled")
	}
}

func TestLocationRouter_ObserveResultSet_CoalescesUpdatesInOrder(t *testing.T) {
	finder := newChannelFinder(nil)

	db1RecipeUpdate := &sppb.CacheUpdate{
		DatabaseId: 1,
		KeyRecipes: &sppb.RecipeList{
			SchemaGeneration: []byte{1, 1},
			Recipe: []*sppb.KeyRecipe{
				{
					Target: &sppb.KeyRecipe_TableName{TableName: "T1"},
					Part: []*sppb.KeyRecipe_Part{
						{Tag: 1},
					},
				},
			},
		},
	}
	db2RangeUpdate := &sppb.CacheUpdate{
		DatabaseId: 2,
		Range: []*sppb.Range{
			{
				StartKey:   []byte("a"),
				LimitKey:   []byte("z"),
				GroupUid:   7,
				SplitId:    9,
				Generation: []byte{2},
			},
		},
		Group: []*sppb.Group{
			{
				GroupUid: 7,
				Tablets: []*sppb.Tablet{
					{TabletUid: 7, ServerAddress: "replica-2"},
				},
			},
		},
	}

	var flush func()
	finder.setFlushSchedulerForTest(func(_ time.Duration, fn func()) {
		flush = fn
	})
	finder.updateAsync(db1RecipeUpdate)
	finder.updateAsync(db2RangeUpdate)
	if flush == nil {
		t.Fatal("expected coalesced flush to be scheduled")
	}
	flush()

	if got := finder.databaseID.Load(); got != 2 {
		t.Fatalf("databaseID=%d, want 2", got)
	}
	if got := finder.rangeCache.size(); got != 1 {
		t.Fatalf("rangeCache.size()=%d, want 1", got)
	}
	hint := &sppb.RoutingHint{}
	finder.recipeCache.applySchemaGeneration(hint)
	if len(hint.GetSchemaGeneration()) != 0 {
		t.Fatalf("expected recipe cache to be cleared on database change, got %v", hint.GetSchemaGeneration())
	}
}

func TestIsExperimentalLocationAPIEnabledForConfig(t *testing.T) {
	t.Run("experimental host enables location API by default", func(t *testing.T) {
		if !isExperimentalLocationAPIEnabledForConfig(ClientConfig{IsExperimentalHost: true}) {
			t.Fatal("expected experimental host to enable location API")
		}
	})

	t.Run("env var false overrides experimental host", func(t *testing.T) {
		t.Setenv(experimentalLocationAPIEnvVar, "false")
		if isExperimentalLocationAPIEnabledForConfig(ClientConfig{IsExperimentalHost: true}) {
			t.Fatal("expected env var false to disable location API even with experimental host")
		}
	})

	t.Run("env var true enables regardless of config", func(t *testing.T) {
		t.Setenv(experimentalLocationAPIEnvVar, "true")
		if !isExperimentalLocationAPIEnabledForConfig(ClientConfig{}) {
			t.Fatal("expected env var true to enable location API")
		}
	})
}

func TestLocationRouter_PrepareReadRequest_FromObservedResultSetUpdate(t *testing.T) {
	router := newLocationRouter(nil)
	if router == nil || router.finder == nil {
		t.Fatal("expected newLocationRouter to initialize finder")
	}

	router.observeResultSet(&sppb.ResultSet{
		CacheUpdate: &sppb.CacheUpdate{
			DatabaseId: 1,
			KeyRecipes: &sppb.RecipeList{
				SchemaGeneration: []byte("1"),
				Recipe: []*sppb.KeyRecipe{
					{
						Target: &sppb.KeyRecipe_TableName{TableName: "T"},
						Part: []*sppb.KeyRecipe_Part{
							{Tag: 1},
							{
								Order:     sppb.KeyRecipe_Part_ASCENDING,
								NullOrder: sppb.KeyRecipe_Part_NULLS_FIRST,
								Type:      &sppb.Type{Code: sppb.TypeCode_STRING},
								ValueType: &sppb.KeyRecipe_Part_Identifier{Identifier: "k"},
							},
						},
					},
				},
			},
		},
	})
	waitForAsyncRoutingUpdate(t, func() bool {
		req := &sppb.ReadRequest{
			Table:   "T",
			Columns: []string{"c1"},
			KeySet: &sppb.KeySet{
				Keys: []*structpb.ListValue{
					{Values: []*structpb.Value{structpb.NewStringValue("foo")}},
				},
			},
		}
		router.prepareReadRequest(context.Background(), req)
		hint := req.GetRoutingHint()
		return hint != nil &&
			hint.GetDatabaseId() == 1 &&
			bytes.Equal(hint.GetSchemaGeneration(), []byte("1")) &&
			len(hint.GetKey()) > 0
	})

	req := &sppb.ReadRequest{
		Table:   "T",
		Columns: []string{"c1"},
		KeySet: &sppb.KeySet{
			Keys: []*structpb.ListValue{
				{Values: []*structpb.Value{structpb.NewStringValue("foo")}},
			},
		},
	}
	router.prepareReadRequest(context.Background(), req)

	hint := req.GetRoutingHint()
	if hint == nil {
		t.Fatal("expected routing hint")
	}
	if hint.GetOperationUid() == 0 {
		t.Fatal("expected operation uid to be set")
	}
	if !bytes.Equal(hint.GetSchemaGeneration(), []byte("1")) {
		t.Fatalf("unexpected schema generation: got %q", hint.GetSchemaGeneration())
	}
	if len(hint.GetKey()) == 0 {
		t.Fatal("expected encoded key bytes")
	}
	if hint.GetDatabaseId() != 1 {
		t.Fatalf("expected database id 1, got %d", hint.GetDatabaseId())
	}
}

func TestLocationRouter_ObserveResultSet_ProcessesDatabaseIDChangeWithoutMaterialUpdate(t *testing.T) {
	router := newLocationRouter(nil)
	router.finder.databaseID.Store(3)

	router.observeResultSet(&sppb.ResultSet{
		CacheUpdate: &sppb.CacheUpdate{DatabaseId: 9},
	})
	waitForAsyncRoutingUpdate(t, func() bool {
		return router.finder.databaseID.Load() == 9
	})

	if got := router.finder.databaseID.Load(); got != 9 {
		t.Fatalf("databaseID after observeResultSet = %d, want 9", got)
	}
}

func TestNewClient_EnablesLocationRouterForExperimentalHost(t *testing.T) {
	_, client, teardown := setupMockedTestServerWithConfig(t, ClientConfig{
		DisableNativeMetrics: true,
		IsExperimentalHost:   true,
	})
	defer teardown()

	if client.locationRouter == nil {
		t.Fatal("expected location router to be enabled for experimental host")
	}
}

func TestLocationRouter_PrepareExecuteSQLRequest_FromObservedPartialResultSetUpdate(t *testing.T) {
	router := newLocationRouter(nil)
	router.observePartialResultSet(&sppb.PartialResultSet{
		CacheUpdate: &sppb.CacheUpdate{
			DatabaseId: 7,
		},
	})
	waitForAsyncRoutingUpdate(t, func() bool {
		req := &sppb.ExecuteSqlRequest{
			Sql: "SELECT * FROM T WHERE p1=@p1",
			Params: &structpb.Struct{
				Fields: map[string]*structpb.Value{
					"p1": structpb.NewStringValue("foo"),
				},
			},
		}
		router.prepareExecuteSQLRequest(context.Background(), req)
		hint := req.GetRoutingHint()
		return hint != nil && hint.GetDatabaseId() == 7
	})

	req := &sppb.ExecuteSqlRequest{
		Sql: "SELECT * FROM T WHERE p1=@p1",
		Params: &structpb.Struct{
			Fields: map[string]*structpb.Value{
				"p1": structpb.NewStringValue("foo"),
			},
		},
	}
	router.prepareExecuteSQLRequest(context.Background(), req)

	hint := req.GetRoutingHint()
	if hint == nil {
		t.Fatal("expected routing hint")
	}
	if hint.GetOperationUid() == 0 {
		t.Fatal("expected operation uid to be set")
	}
	if hint.GetDatabaseId() != 7 {
		t.Fatalf("expected database id 7, got %d", hint.GetDatabaseId())
	}
}

func TestLocationRouter_PrepareCommitRequest_MutationNotInCache(t *testing.T) {
	router := newLocationRouter(nil)
	req := &sppb.CommitRequest{
		Mutations: []*sppb.Mutation{createInsertMutation("b")},
	}

	endpoint := router.prepareCommitRequest(context.Background(), req)
	if endpoint != nil {
		t.Fatalf("expected no endpoint for commit mutation cache miss, got %v", endpoint)
	}
	if hint := req.GetRoutingHint(); hint != nil && len(hint.GetKey()) > 0 {
		t.Fatalf("expected no encoded commit key for cache miss, got %v", hint.GetKey())
	}
}

func TestLocationRouter_NilSafety(t *testing.T) {
	var router *locationRouter
	router.prepareReadRequest(context.Background(), nil)
	router.prepareReadRequest(context.Background(), &sppb.ReadRequest{})
	router.prepareExecuteSQLRequest(context.Background(), nil)
	router.prepareExecuteSQLRequest(context.Background(), &sppb.ExecuteSqlRequest{})
	router.prepareBeginTransactionRequest(context.Background(), nil)
	router.prepareBeginTransactionRequest(context.Background(), &sppb.BeginTransactionRequest{})
	router.prepareCommitRequest(context.Background(), nil)
	router.prepareCommitRequest(context.Background(), &sppb.CommitRequest{})
	router.observePartialResultSet(nil)
	router.observePartialResultSet(&sppb.PartialResultSet{})
	router.observeResultSet(nil)
	router.observeResultSet(&sppb.ResultSet{})
	router.observeTransaction(nil)
	router.observeTransaction(&sppb.Transaction{})
	router.observeCommitResponse(nil)
	router.observeCommitResponse(&sppb.CommitResponse{})
}

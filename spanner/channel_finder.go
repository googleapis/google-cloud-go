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
	"sync"
	"sync/atomic"
	"time"

	sppb "cloud.google.com/go/spanner/apiv1/spannerpb"
)

type channelFinder struct {
	updateMu sync.Mutex

	databaseID  atomic.Uint64
	recipeCache *keyRecipeCache
	rangeCache  *keyRangeCache
}

func newChannelFinder(endpointCache channelEndpointCache) *channelFinder {
	return &channelFinder{
		recipeCache: newKeyRecipeCache(),
		rangeCache:  newKeyRangeCache(endpointCache),
	}
}

func (f *channelFinder) useDeterministicRandom() {
	f.rangeCache.useDeterministicRandom()
}

func (f *channelFinder) setLifecycleManager(lifecycleManager *endpointLifecycleManager) {
	if f == nil {
		return
	}
	f.rangeCache.setLifecycleManager(lifecycleManager)
}

func (f *channelFinder) recordEndpointLatency(address string, latency time.Duration) {
	if f == nil {
		return
	}
	f.rangeCache.recordEndpointLatency(address, latency)
}

func (f *channelFinder) recordEndpointError(address string) {
	if f == nil {
		return
	}
	f.rangeCache.recordEndpointError(address)
}

func (f *channelFinder) update(update *sppb.CacheUpdate) {
	if update == nil {
		return
	}
	f.updateMu.Lock()
	defer f.updateMu.Unlock()

	currentID := f.databaseID.Load()
	if currentID != update.GetDatabaseId() {
		if currentID != 0 {
			f.recipeCache.clear()
			f.rangeCache.clear()
		}
		f.databaseID.Store(update.GetDatabaseId())
	}
	if update.GetKeyRecipes() != nil {
		f.recipeCache.addRecipes(update.GetKeyRecipes())
	}
	f.rangeCache.addRanges(update)
}

func (f *channelFinder) updateAsync(update *sppb.CacheUpdate) {
	if !f.shouldProcessUpdate(update) {
		return
	}
	f.update(update)
}

func (f *channelFinder) shouldProcessUpdate(update *sppb.CacheUpdate) bool {
	if update == nil {
		return false
	}
	if f.isMaterialUpdate(update) {
		return true
	}
	updateDatabaseID := update.GetDatabaseId()
	return updateDatabaseID != 0 && f.databaseID.Load() != updateDatabaseID
}

func (*channelFinder) isMaterialUpdate(update *sppb.CacheUpdate) bool {
	if update == nil {
		return false
	}
	return len(update.GetGroup()) > 0 ||
		len(update.GetRange()) > 0 ||
		(update.GetKeyRecipes() != nil && len(update.GetKeyRecipes().GetRecipe()) > 0)
}

func (f *channelFinder) findServerRead(ctx context.Context, req *sppb.ReadRequest, preferLeader bool) channelEndpoint {
	return f.findServerReadWithExclusions(ctx, req, preferLeader, nil)
}

func (f *channelFinder) findServerReadWithExclusions(ctx context.Context, req *sppb.ReadRequest, preferLeader bool, excludedEndpoints endpointExcluder) channelEndpoint {
	if req == nil {
		return nil
	}
	f.recipeCache.computeReadKeys(req)
	hint := ensureReadRoutingHint(req)
	return f.fillRoutingHintWithExclusions(ctx, preferLeader, rangeModeCoveringSplit, req.GetDirectedReadOptions(), hint, excludedEndpoints)
}

func (f *channelFinder) findServerReadWithTransaction(ctx context.Context, req *sppb.ReadRequest) channelEndpoint {
	if req == nil {
		return nil
	}
	return f.findServerRead(ctx, req, preferLeaderFromSelector(req.GetTransaction()))
}

func (f *channelFinder) findServerExecuteSQL(ctx context.Context, req *sppb.ExecuteSqlRequest, preferLeader bool) channelEndpoint {
	return f.findServerExecuteSQLWithExclusions(ctx, req, preferLeader, nil)
}

func (f *channelFinder) findServerExecuteSQLWithExclusions(ctx context.Context, req *sppb.ExecuteSqlRequest, preferLeader bool, excludedEndpoints endpointExcluder) channelEndpoint {
	if req == nil {
		return nil
	}
	f.recipeCache.computeQueryKeys(req)
	hint := ensureExecuteSQLRoutingHint(req)
	return f.fillRoutingHintWithExclusions(ctx, preferLeader, rangeModePickRandom, req.GetDirectedReadOptions(), hint, excludedEndpoints)
}

func (f *channelFinder) findServerExecuteSQLWithTransaction(ctx context.Context, req *sppb.ExecuteSqlRequest) channelEndpoint {
	if req == nil {
		return nil
	}
	return f.findServerExecuteSQL(ctx, req, preferLeaderFromSelector(req.GetTransaction()))
}

func (f *channelFinder) findServerBeginTransaction(ctx context.Context, req *sppb.BeginTransactionRequest) channelEndpoint {
	return f.findServerBeginTransactionWithExclusions(ctx, req, nil)
}

func (f *channelFinder) findServerBeginTransactionWithExclusions(ctx context.Context, req *sppb.BeginTransactionRequest, excludedEndpoints endpointExcluder) channelEndpoint {
	if req == nil || req.GetMutationKey() == nil {
		return nil
	}
	return f.routeMutationWithExclusions(ctx, req.GetMutationKey(), preferLeaderFromTransactionOptions(req.GetOptions()), ensureBeginTransactionRoutingHint(req), excludedEndpoints)
}

func (f *channelFinder) fillCommitRoutingHint(ctx context.Context, req *sppb.CommitRequest) channelEndpoint {
	return f.fillCommitRoutingHintWithExclusions(ctx, req, nil)
}

func (f *channelFinder) fillCommitRoutingHintWithExclusions(ctx context.Context, req *sppb.CommitRequest, excludedEndpoints endpointExcluder) channelEndpoint {
	if req == nil {
		return nil
	}
	mutation := selectMutationProtoForRouting(req.GetMutations())
	if mutation == nil {
		return nil
	}
	return f.routeMutationWithExclusions(ctx, mutation, true, ensureCommitRoutingHint(req), excludedEndpoints)
}

func (f *channelFinder) routeMutation(ctx context.Context, mutation *sppb.Mutation, preferLeader bool, hint *sppb.RoutingHint) channelEndpoint {
	return f.routeMutationWithExclusions(ctx, mutation, preferLeader, hint, nil)
}

func (f *channelFinder) routeMutationWithExclusions(ctx context.Context, mutation *sppb.Mutation, preferLeader bool, hint *sppb.RoutingHint, excludedEndpoints endpointExcluder) channelEndpoint {
	if mutation == nil || hint == nil {
		return nil
	}
	f.recipeCache.applySchemaGeneration(hint)
	target := f.recipeCache.mutationToTargetRange(mutation)
	if target == nil {
		return nil
	}
	f.recipeCache.applyTargetRange(hint, target)
	return f.fillRoutingHintWithExclusions(ctx, preferLeader, rangeModeCoveringSplit, &sppb.DirectedReadOptions{}, hint, excludedEndpoints)
}

func (f *channelFinder) fillRoutingHint(ctx context.Context, preferLeader bool, mode rangeMode, directedReadOptions *sppb.DirectedReadOptions, hint *sppb.RoutingHint) channelEndpoint {
	return f.fillRoutingHintWithExclusions(ctx, preferLeader, mode, directedReadOptions, hint, nil)
}

func (f *channelFinder) fillRoutingHintWithExclusions(ctx context.Context, preferLeader bool, mode rangeMode, directedReadOptions *sppb.DirectedReadOptions, hint *sppb.RoutingHint, excludedEndpoints endpointExcluder) channelEndpoint {
	if hint == nil {
		return nil
	}
	databaseID := f.databaseID.Load()
	if databaseID == 0 {
		return nil
	}
	hint.DatabaseId = databaseID
	return f.rangeCache.fillRoutingHintWithExclusions(ctx, preferLeader, mode, directedReadOptions, hint, excludedEndpoints)
}

func preferLeaderFromSelector(selector *sppb.TransactionSelector) bool {
	if selector == nil {
		return true
	}
	switch s := selector.GetSelector().(type) {
	case *sppb.TransactionSelector_Begin:
		if s.Begin == nil || s.Begin.GetReadOnly() == nil {
			return true
		}
		return s.Begin.GetReadOnly().GetStrong()
	case *sppb.TransactionSelector_SingleUse:
		if s.SingleUse == nil || s.SingleUse.GetReadOnly() == nil {
			return true
		}
		return s.SingleUse.GetReadOnly().GetStrong()
	default:
		return true
	}
}

func preferLeaderFromTransactionOptions(options *sppb.TransactionOptions) bool {
	if options == nil || options.GetReadOnly() == nil {
		return true
	}
	return options.GetReadOnly().GetStrong()
}

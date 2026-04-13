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
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/reflect/protoreflect"
)

type channelFinder struct {
	updateMu sync.Mutex

	databaseID  atomic.Uint64
	recipeCache *keyRecipeCache
	rangeCache  *keyRangeCache

	coalescingMu   sync.Mutex
	pendingUpdate  *sppb.CacheUpdate
	flushScheduled bool
}

const cacheUpdateCoalescingWindow = 5 * time.Millisecond

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
	f.enqueueCoalescedUpdate(update)
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

func (f *channelFinder) enqueueCoalescedUpdate(update *sppb.CacheUpdate) {
	if f == nil || update == nil {
		return
	}

	f.coalescingMu.Lock()
	if f.pendingUpdate == nil {
		f.pendingUpdate = cloneCacheUpdate(update)
	} else {
		f.pendingUpdate = mergeCacheUpdates(f.pendingUpdate, update)
	}
	if f.flushScheduled {
		f.coalescingMu.Unlock()
		return
	}
	f.flushScheduled = true
	f.coalescingMu.Unlock()

	go func() {
		time.Sleep(cacheUpdateCoalescingWindow)
		f.flushCoalescedUpdates()
	}()
}

func (f *channelFinder) flushCoalescedUpdates() {
	if f == nil {
		return
	}
	f.coalescingMu.Lock()
	update := f.pendingUpdate
	f.pendingUpdate = nil
	f.flushScheduled = false
	f.coalescingMu.Unlock()

	if update != nil {
		f.update(update)
	}
}

func cloneCacheUpdate(update *sppb.CacheUpdate) *sppb.CacheUpdate {
	if update == nil {
		return nil
	}
	return cloneProto(update)
}

func mergeCacheUpdates(base, incoming *sppb.CacheUpdate) *sppb.CacheUpdate {
	switch {
	case base == nil:
		return cloneCacheUpdate(incoming)
	case incoming == nil:
		return base
	}
	merged := cloneCacheUpdate(base)
	if merged == nil {
		return cloneCacheUpdate(incoming)
	}
	protoMergeCacheUpdate(merged, incoming)
	return merged
}

func cloneProto[M interface{ ProtoReflect() protoreflect.Message }](msg M) M {
	if any(msg) == nil {
		var zero M
		return zero
	}
	return proto.Clone(msg).(M)
}

func protoMergeCacheUpdate(dst, src *sppb.CacheUpdate) {
	if dst == nil || src == nil {
		return
	}
	proto.Merge(dst, src)
}

func (f *channelFinder) findServerRead(ctx context.Context, req *sppb.ReadRequest, preferLeader bool) channelEndpoint {
	return f.findServerReadWithExclusions(ctx, req, preferLeader, nil)
}

func (f *channelFinder) findServerReadWithExclusions(ctx context.Context, req *sppb.ReadRequest, preferLeader bool, excludedEndpoints endpointExcluder) channelEndpoint {
	endpoint, _ := f.findServerReadWithExclusionsAndDetails(ctx, req, preferLeader, excludedEndpoints)
	return endpoint
}

func (f *channelFinder) findServerReadWithExclusionsAndDetails(ctx context.Context, req *sppb.ReadRequest, preferLeader bool, excludedEndpoints endpointExcluder) (channelEndpoint, routeSelectionDetails) {
	details := newRouteSelectionDetails()
	if req == nil {
		details.defaultReasonCode = "range_cache_miss"
		return nil, details
	}
	f.recipeCache.computeReadKeys(req)
	hint := ensureReadRoutingHint(req)
	return f.fillRoutingHintWithExclusionsAndDetails(ctx, preferLeader, rangeModeCoveringSplit, req.GetDirectedReadOptions(), hint, excludedEndpoints)
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
	endpoint, _ := f.findServerExecuteSQLWithExclusionsAndDetails(ctx, req, preferLeader, excludedEndpoints)
	return endpoint
}

func (f *channelFinder) findServerExecuteSQLWithExclusionsAndDetails(ctx context.Context, req *sppb.ExecuteSqlRequest, preferLeader bool, excludedEndpoints endpointExcluder) (channelEndpoint, routeSelectionDetails) {
	details := newRouteSelectionDetails()
	if req == nil {
		details.defaultReasonCode = "range_cache_miss"
		return nil, details
	}
	f.recipeCache.computeQueryKeys(req)
	hint := ensureExecuteSQLRoutingHint(req)
	return f.fillRoutingHintWithExclusionsAndDetails(ctx, preferLeader, rangeModePickRandom, req.GetDirectedReadOptions(), hint, excludedEndpoints)
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
	endpoint, _ := f.findServerBeginTransactionWithExclusionsAndDetails(ctx, req, excludedEndpoints)
	return endpoint
}

func (f *channelFinder) findServerBeginTransactionWithExclusionsAndDetails(ctx context.Context, req *sppb.BeginTransactionRequest, excludedEndpoints endpointExcluder) (channelEndpoint, routeSelectionDetails) {
	details := newRouteSelectionDetails()
	if req == nil || req.GetMutationKey() == nil {
		details.defaultReasonCode = "range_cache_miss"
		return nil, details
	}
	return f.routeMutationWithExclusionsAndDetails(ctx, req.GetMutationKey(), preferLeaderFromTransactionOptions(req.GetOptions()), ensureBeginTransactionRoutingHint(req), excludedEndpoints)
}

func (f *channelFinder) fillCommitRoutingHint(ctx context.Context, req *sppb.CommitRequest) channelEndpoint {
	return f.fillCommitRoutingHintWithExclusions(ctx, req, nil)
}

func (f *channelFinder) fillCommitRoutingHintWithExclusions(ctx context.Context, req *sppb.CommitRequest, excludedEndpoints endpointExcluder) channelEndpoint {
	endpoint, _ := f.fillCommitRoutingHintWithExclusionsAndDetails(ctx, req, excludedEndpoints)
	return endpoint
}

func (f *channelFinder) fillCommitRoutingHintWithExclusionsAndDetails(ctx context.Context, req *sppb.CommitRequest, excludedEndpoints endpointExcluder) (channelEndpoint, routeSelectionDetails) {
	details := newRouteSelectionDetails()
	if req == nil {
		details.defaultReasonCode = "range_cache_miss"
		return nil, details
	}
	mutation := selectMutationProtoForRouting(req.GetMutations())
	if mutation == nil {
		details.defaultReasonCode = "range_cache_miss"
		return nil, details
	}
	return f.routeMutationWithExclusionsAndDetails(ctx, mutation, true, ensureCommitRoutingHint(req), excludedEndpoints)
}

func (f *channelFinder) routeMutation(ctx context.Context, mutation *sppb.Mutation, preferLeader bool, hint *sppb.RoutingHint) channelEndpoint {
	return f.routeMutationWithExclusions(ctx, mutation, preferLeader, hint, nil)
}

func (f *channelFinder) routeMutationWithExclusions(ctx context.Context, mutation *sppb.Mutation, preferLeader bool, hint *sppb.RoutingHint, excludedEndpoints endpointExcluder) channelEndpoint {
	endpoint, _ := f.routeMutationWithExclusionsAndDetails(ctx, mutation, preferLeader, hint, excludedEndpoints)
	return endpoint
}

func (f *channelFinder) routeMutationWithExclusionsAndDetails(ctx context.Context, mutation *sppb.Mutation, preferLeader bool, hint *sppb.RoutingHint, excludedEndpoints endpointExcluder) (channelEndpoint, routeSelectionDetails) {
	details := newRouteSelectionDetails()
	if mutation == nil || hint == nil {
		details.defaultReasonCode = "range_cache_miss"
		return nil, details
	}
	f.recipeCache.applySchemaGeneration(hint)
	target := f.recipeCache.mutationToTargetRange(mutation)
	if target == nil {
		details.defaultReasonCode = "range_cache_miss"
		return nil, details
	}
	f.recipeCache.applyTargetRange(hint, target)
	return f.fillRoutingHintWithExclusionsAndDetails(ctx, preferLeader, rangeModeCoveringSplit, &sppb.DirectedReadOptions{}, hint, excludedEndpoints)
}

func (f *channelFinder) fillRoutingHint(ctx context.Context, preferLeader bool, mode rangeMode, directedReadOptions *sppb.DirectedReadOptions, hint *sppb.RoutingHint) channelEndpoint {
	return f.fillRoutingHintWithExclusions(ctx, preferLeader, mode, directedReadOptions, hint, nil)
}

func (f *channelFinder) fillRoutingHintWithExclusions(ctx context.Context, preferLeader bool, mode rangeMode, directedReadOptions *sppb.DirectedReadOptions, hint *sppb.RoutingHint, excludedEndpoints endpointExcluder) channelEndpoint {
	endpoint, _ := f.fillRoutingHintWithExclusionsAndDetails(ctx, preferLeader, mode, directedReadOptions, hint, excludedEndpoints)
	return endpoint
}

func (f *channelFinder) fillRoutingHintWithExclusionsAndDetails(ctx context.Context, preferLeader bool, mode rangeMode, directedReadOptions *sppb.DirectedReadOptions, hint *sppb.RoutingHint, excludedEndpoints endpointExcluder) (channelEndpoint, routeSelectionDetails) {
	details := newRouteSelectionDetails()
	if hint == nil {
		details.defaultReasonCode = "range_cache_miss"
		return nil, details
	}
	databaseID := f.databaseID.Load()
	if databaseID == 0 {
		details.defaultReasonCode = "range_cache_miss"
		return nil, details
	}
	hint.DatabaseId = databaseID
	return f.rangeCache.fillRoutingHintWithExclusionsAndDetails(ctx, preferLeader, mode, directedReadOptions, hint, excludedEndpoints)
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

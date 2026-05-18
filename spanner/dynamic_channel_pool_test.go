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

package spanner

import (
	"context"
	"fmt"
	"sync"
	"testing"
	"time"

	"golang.org/x/sync/errgroup"
	"google.golang.org/api/iterator"
	"google.golang.org/api/option"
	gtransport "google.golang.org/api/transport/grpc"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	. "cloud.google.com/go/spanner/internal/testutil"
)

func testDCPConfig(initial, min, max int) DynamicChannelPoolConfig {
	return DynamicChannelPoolConfig{
		DCPEnabled:                           true,
		DCPInitialChannels:                   initial,
		DCPMinChannels:                       min,
		DCPMaxChannels:                       max,
		DCPMaxRPCPerChannel:                  1,
		DCPMinRPCPerChannel:                  0.5,
		DCPScaleDownCheckInterval:            20 * time.Millisecond,
		DCPScaleUpCooldown:                   time.Millisecond,
		DCPDownscaleConsecutiveLowLoadChecks: 1,
		DCPMaxScaleUpPercent:                 100,
		DCPMaxRemoveChannels:                 max,
		DCPDrainIdleGrace:                    10 * time.Millisecond,
		DCPMaxDrainTimeout:                   time.Second,
		DCPPrimeTimeout:                      time.Second,
		DCPPrimeMaxAttempts:                  3,
	}
}

func setupDCPMockedTestServer(t *testing.T, dcp DynamicChannelPoolConfig) (*MockedSpannerInMemTestServer, *Client, func()) {
	t.Helper()
	server, client, teardown := setupMockedTestServerWithConfig(t, ClientConfig{
		DisableNativeMetrics:     true,
		DynamicChannelPoolConfig: dcp,
	})
	addSelect1Result(server)
	if client.sc.dynamicPool == nil {
		teardown()
		t.Fatal("dynamic channel pool not enabled")
	}
	return server, client, teardown
}

func drainDCPQuery(ctx context.Context, client *Client) error {
	iter := client.Single().Query(ctx, NewStatement(SelectSingerIDAlbumIDAlbumTitleFromAlbums))
	defer iter.Stop()
	for {
		_, err := iter.Next()
		if err == iterator.Done {
			return nil
		}
		if err != nil {
			return err
		}
	}
}

func totalDCPOperationRefs(p *dynamicChannelPool) int32 {
	var total int32
	for _, e := range p.getEntries() {
		total += e.operationRefs.Load()
	}
	return total
}

func TestDynamicChannelPoolOptInCreatesInitialChannels(t *testing.T) {
	_, client, teardown := setupDCPMockedTestServer(t, testDCPConfig(2, 1, 4))
	defer teardown()

	if got, want := client.sc.dynamicPool.Num(), 2; got != want {
		t.Fatalf("DCP initial channel count = %d, want %d", got, want)
	}
}

func TestDynamicChannelPoolScaleUpPrimesNewChannels(t *testing.T) {
	server, client, teardown := setupDCPMockedTestServer(t, testDCPConfig(1, 1, 4))
	defer teardown()
	server.TestSpanner.PutExecutionTime(MethodExecuteStreamingSql, SimulatedExecutionTime{MinimumExecutionTime: 2 * time.Second})
	if got := len(server.TestSpanner.DumpPings()); got != 0 {
		t.Fatalf("initial DCP channel priming count = %d, want 0", got)
	}

	ctx := context.Background()
	var g errgroup.Group
	for i := 0; i < 3; i++ {
		g.Go(func() error { return drainDCPQuery(ctx, client) })
	}

	waitFor(t, func() error {
		if got := client.sc.dynamicPool.Num(); got <= 1 {
			return fmt.Errorf("DCP channel count = %d, want > 1", got)
		}
		if got := len(server.TestSpanner.DumpPings()); got == 0 {
			return fmt.Errorf("DCP scale-up priming SELECT 1 count = %d, want > 0", got)
		}
		return nil
	})
	if err := g.Wait(); err != nil {
		t.Fatalf("query workload failed: %v", err)
	}
}

func TestDynamicChannelPoolScaleDownRemovesIdleChannelsToMin(t *testing.T) {
	cfg := testDCPConfig(3, 1, 3)
	cfg.DCPDrainIdleGrace = 200 * time.Millisecond
	_, client, teardown := setupDCPMockedTestServer(t, cfg)
	defer teardown()

	if err := drainDCPQuery(context.Background(), client); err != nil {
		t.Fatalf("query failed: %v", err)
	}
	waitFor(t, func() error {
		if got, want := client.sc.dynamicPool.Num(), 1; got != want {
			return fmt.Errorf("DCP channel count after scale-down = %d, want %d", got, want)
		}
		if got := client.sc.dynamicPool.drainingCount.Load(); got == 0 {
			return fmt.Errorf("DCP draining channel count = %d, want > 0 during drain grace", got)
		}
		return nil
	})
	waitFor(t, func() error {
		if got := client.sc.dynamicPool.drainingCount.Load(); got != 0 {
			return fmt.Errorf("DCP draining channel count after grace = %d, want 0", got)
		}
		return nil
	})
}

func TestDynamicChannelPoolScaleDownRequiresRepeatedLowLoad(t *testing.T) {
	_, client, teardown := setupDCPMockedTestServer(t, testDCPConfig(3, 1, 3))
	defer teardown()
	p := client.sc.dynamicPool
	p.cfg.DCPDownscaleConsecutiveLowLoadChecks = 2

	p.evaluateScaleDown()
	if got, want := p.Num(), 3; got != want {
		t.Fatalf("DCP channel count after first low-load check = %d, want %d", got, want)
	}
	p.evaluateScaleDown()
	waitFor(t, func() error {
		if got, want := p.Num(), 1; got != want {
			return fmt.Errorf("DCP channel count after repeated low-load checks = %d, want %d", got, want)
		}
		return nil
	})
}

func TestDynamicChannelPoolPickerSkipsDrainingEntries(t *testing.T) {
	_, client, teardown := setupDCPMockedTestServer(t, testDCPConfig(3, 3, 3))
	defer teardown()
	p := client.sc.dynamicPool
	entries := p.getEntries()
	for _, e := range entries[:2] {
		e.state.Store(dcpStateDraining)
	}
	for i := 0; i < 20; i++ {
		e, release, err := p.pick(context.Background(), true)
		if err != nil {
			t.Fatalf("pick failed: %v", err)
		}
		release()
		if e != entries[2] {
			t.Fatalf("picker returned draining entry %d, want active entry %d", e.id, entries[2].id)
		}
	}
}

func TestDynamicChannelPoolRoundRobinSkipsDrainingEntries(t *testing.T) {
	_, client, teardown := setupDCPMockedTestServer(t, testDCPConfig(3, 3, 3))
	defer teardown()
	p := client.sc.dynamicPool
	p.cfg.DCPSelectionStrategy = DCPRoundRobin
	entries := p.getEntries()
	entries[1].state.Store(dcpStateDraining)

	var got []uint64
	for i := 0; i < 4; i++ {
		e, release, err := p.pick(context.Background(), true)
		if err != nil {
			t.Fatalf("pick failed: %v", err)
		}
		release()
		got = append(got, e.id)
	}
	want := []uint64{entries[0].id, entries[2].id, entries[0].id, entries[2].id}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("round-robin sequence = %v, want %v", got, want)
		}
	}
}

func TestDynamicChannelPoolErrorPenaltyAllowlist(t *testing.T) {
	_, client, teardown := setupDCPMockedTestServer(t, testDCPConfig(2, 2, 2))
	defer teardown()
	p := client.sc.dynamicPool
	entries := p.getEntries()
	base := entries[0].weightedLoad()

	entries[0].applyPenalty(context.Background(), status.Error(codes.Internal, "query bug"))
	if got := entries[0].weightedLoad(); got != base {
		t.Fatalf("Internal penalty weighted load = %d, want %d", got, base)
	}
	entries[0].applyPenalty(context.Background(), status.Error(codes.DeadlineExceeded, "slow query"))
	if got := entries[0].weightedLoad(); got != base {
		t.Fatalf("DeadlineExceeded penalty weighted load = %d, want %d", got, base)
	}
	entries[0].applyPenalty(context.Background(), status.Error(codes.Unavailable, "transport unavailable"))
	if got, wantMin := entries[0].weightedLoad(), base+p.cfg.DCPErrorPenaltyLoad; got < wantMin {
		t.Fatalf("Unavailable penalty weighted load = %d, want >= %d", got, wantMin)
	}
	entries[1].applyPenalty(context.Background(), status.Error(codes.ResourceExhausted, "overload"))
	if got, wantMin := entries[1].weightedLoad(), p.cfg.DCPErrorPenaltyLoad; got < wantMin {
		t.Fatalf("ResourceExhausted penalty weighted load = %d, want >= %d", got, wantMin)
	}
}

func TestDynamicChannelPoolMaxChannelsCapsScaleUp(t *testing.T) {
	server, client, teardown := setupDCPMockedTestServer(t, testDCPConfig(1, 1, 2))
	defer teardown()
	server.TestSpanner.PutExecutionTime(MethodExecuteStreamingSql, SimulatedExecutionTime{MinimumExecutionTime: 300 * time.Millisecond})

	var g errgroup.Group
	for i := 0; i < 8; i++ {
		g.Go(func() error { return drainDCPQuery(context.Background(), client) })
	}
	waitFor(t, func() error {
		if got, want := client.sc.dynamicPool.Num(), 2; got != want {
			return fmt.Errorf("DCP channel count under load = %d, want %d", got, want)
		}
		return nil
	})
	if err := g.Wait(); err != nil {
		t.Fatalf("query workload failed: %v", err)
	}
	if got, max := client.sc.dynamicPool.Num(), 2; got > max {
		t.Fatalf("DCP channel count = %d, want <= %d", got, max)
	}
}

func TestDynamicChannelPoolLocationAwareDisablesDCP(t *testing.T) {
	_, client, teardown := setupMockedTestServerWithConfig(t, ClientConfig{
		DisableNativeMetrics:     true,
		IsExperimentalHost:       true,
		DynamicChannelPoolConfig: testDCPConfig(1, 1, 2),
	})
	defer teardown()
	if client.sc.dynamicPool != nil {
		t.Fatal("DCP enabled with location-aware routing, want disabled")
	}
}

func TestDynamicChannelPoolDrainWaitsForOperationRefs(t *testing.T) {
	cfg := testDCPConfig(1, 1, 2)
	cfg.DCPDrainIdleGrace = 20 * time.Millisecond
	cfg.DCPMaxDrainTimeout = time.Second
	_, client, teardown := setupDCPMockedTestServer(t, cfg)
	defer teardown()

	var drainedEntry *dcpEntry
	_, err := client.ReadWriteTransaction(context.Background(), func(ctx context.Context, tx *ReadWriteTransaction) error {
		for _, e := range client.sc.dynamicPool.getEntries() {
			if e.operationRefs.Load() > 0 {
				drainedEntry = e
				break
			}
		}
		if drainedEntry == nil {
			return fmt.Errorf("no DCP entry held operation ref")
		}
		drainedEntry.state.Store(dcpStateDraining)
		client.sc.dynamicPool.drainingCount.Add(1)
		done := make(chan struct{})
		go func() {
			client.sc.dynamicPool.waitForDrainAndClose(drainedEntry)
			close(done)
		}()

		select {
		case <-done:
			return fmt.Errorf("drain closed entry while operationRefs=%d", drainedEntry.operationRefs.Load())
		case <-time.After(100 * time.Millisecond):
		}
		if got := drainedEntry.state.Load(); got == dcpStateClosed {
			return fmt.Errorf("drain closed entry while transaction still active")
		}
		iter := tx.Query(ctx, NewStatement(SelectSingerIDAlbumIDAlbumTitleFromAlbums))
		defer iter.Stop()
		if _, err := iter.Next(); err != nil {
			return fmt.Errorf("query on draining transaction entry failed: %w", err)
		}
		if got := drainedEntry.state.Load(); got == dcpStateClosed {
			return fmt.Errorf("draining entry closed after in-transaction query")
		}
		return nil
	})
	if err != nil {
		t.Fatalf("ReadWriteTransaction failed: %v", err)
	}
	waitFor(t, func() error {
		if drainedEntry == nil || drainedEntry.state.Load() != dcpStateClosed {
			return fmt.Errorf("drained entry state = %v, want closed", drainedEntry.state.Load())
		}
		return nil
	})
}

func TestDynamicChannelPoolTransactionOperationRefReleasedOnRecycle(t *testing.T) {
	_, client, teardown := setupDCPMockedTestServer(t, testDCPConfig(1, 1, 2))
	defer teardown()

	_, err := client.ReadWriteTransaction(context.Background(), func(ctx context.Context, tx *ReadWriteTransaction) error {
		if got, want := totalDCPOperationRefs(client.sc.dynamicPool), int32(1); got != want {
			return fmt.Errorf("operation refs during transaction = %d, want %d", got, want)
		}
		iter := tx.Query(ctx, NewStatement(SelectSingerIDAlbumIDAlbumTitleFromAlbums))
		defer iter.Stop()
		for {
			_, err := iter.Next()
			if err == iterator.Done {
				return nil
			}
			if err != nil {
				return err
			}
		}
	})
	if err != nil {
		t.Fatalf("ReadWriteTransaction failed: %v", err)
	}
	waitFor(t, func() error {
		if got := totalDCPOperationRefs(client.sc.dynamicPool); got != 0 {
			return fmt.Errorf("operation refs after transaction recycle = %d, want 0", got)
		}
		return nil
	})
}

type fakeDCPConnPool struct {
	invokeErr   error
	invokeCount int
	closed      bool
}

func (f *fakeDCPConnPool) Conn() *grpc.ClientConn { return nil }
func (f *fakeDCPConnPool) Num() int               { return 1 }
func (f *fakeDCPConnPool) Close() error {
	f.closed = true
	return nil
}
func (f *fakeDCPConnPool) Invoke(ctx context.Context, method string, args, reply interface{}, opts ...grpc.CallOption) error {
	f.invokeCount++
	return f.invokeErr
}
func (f *fakeDCPConnPool) NewStream(ctx context.Context, desc *grpc.StreamDesc, method string, opts ...grpc.CallOption) (grpc.ClientStream, error) {
	return nil, f.invokeErr
}

func TestDynamicChannelPoolOperationRefsReleasedAcrossReadOnlyPaths(t *testing.T) {
	server, client, teardown := setupDCPMockedTestServer(t, testDCPConfig(1, 1, 2))
	defer teardown()

	if err := drainDCPQuery(context.Background(), client); err != nil {
		t.Fatalf("single-use query failed: %v", err)
	}
	if got := totalDCPOperationRefs(client.sc.dynamicPool); got != 0 {
		t.Fatalf("operation refs after single-use query = %d, want 0", got)
	}

	server.TestSpanner.PutExecutionTime(MethodExecuteStreamingSql, SimulatedExecutionTime{Errors: []error{status.Error(codes.Internal, "single-use error")}})
	iter := client.Single().Query(context.Background(), NewStatement(SelectSingerIDAlbumIDAlbumTitleFromAlbums))
	if _, err := iter.Next(); err == nil {
		t.Fatal("single-use query error path succeeded, want error")
	}
	iter.Stop()
	waitFor(t, func() error {
		if got := totalDCPOperationRefs(client.sc.dynamicPool); got != 0 {
			return fmt.Errorf("operation refs after single-use error = %d, want 0", got)
		}
		return nil
	})

	ro := client.ReadOnlyTransaction()
	iter = ro.Query(context.Background(), NewStatement(SelectSingerIDAlbumIDAlbumTitleFromAlbums))
	if _, err := iter.Next(); err != nil {
		t.Fatalf("read-only transaction query failed: %v", err)
	}
	iter.Stop()
	if got := totalDCPOperationRefs(client.sc.dynamicPool); got == 0 {
		t.Fatal("operation refs during multi-use read-only transaction = 0, want > 0")
	}
	ro.Close()
	waitFor(t, func() error {
		if got := totalDCPOperationRefs(client.sc.dynamicPool); got != 0 {
			return fmt.Errorf("operation refs after read-only transaction close = %d, want 0", got)
		}
		return nil
	})
}

func TestDynamicChannelPoolErrorPenaltyExpires(t *testing.T) {
	_, client, teardown := setupDCPMockedTestServer(t, testDCPConfig(1, 1, 1))
	defer teardown()
	p := client.sc.dynamicPool
	p.cfg.DCPErrorPenaltyDuration = 20 * time.Millisecond
	entry := p.getEntries()[0]

	base := entry.weightedLoad()
	entry.applyPenalty(context.Background(), status.Error(codes.Unavailable, "temporary"))
	if got := entry.weightedLoad(); got <= base {
		t.Fatalf("weighted load after penalty = %d, want > %d", got, base)
	}
	waitFor(t, func() error {
		if got := entry.weightedLoad(); got != base {
			return fmt.Errorf("weighted load after penalty expiry = %d, want %d", got, base)
		}
		return nil
	})
}

func TestDynamicChannelPoolDirectPathFallbackUsesSharedState(t *testing.T) {
	state := &dcpFallbackState{}
	primary1 := &fakeDCPConnPool{invokeErr: status.Error(codes.Unavailable, "directpath unavailable")}
	cloud1 := &fakeDCPConnPool{}
	primary2 := &fakeDCPConnPool{}
	cloud2 := &fakeDCPConnPool{}
	slot1 := &dcpFallbackSlot{id: 1, direct: primary1, cloud: cloud1, state: state}
	slot2 := &dcpFallbackSlot{id: 2, direct: primary2, cloud: cloud2, state: state}

	_ = slot1.Invoke(context.Background(), "/test", nil, nil)
	if !state.fallbackActive.Load() {
		t.Fatal("shared fallback state inactive after DirectPath failure threshold")
	}
	if err := slot2.Invoke(context.Background(), "/test", nil, nil); err != nil {
		t.Fatalf("fallback slot invoke failed: %v", err)
	}
	if got := primary2.invokeCount; got != 0 {
		t.Fatalf("slot2 primary invoke count = %d, want 0 after shared fallback", got)
	}
	if got := cloud2.invokeCount; got != 1 {
		t.Fatalf("slot2 cloud invoke count = %d, want 1 after shared fallback", got)
	}
}

func TestDynamicChannelPoolScaleUpPrimeFailureDoesNotPublishEntry(t *testing.T) {
	server, client, teardown := setupDCPMockedTestServer(t, testDCPConfig(1, 1, 2))
	defer teardown()
	server.TestSpanner.PutExecutionTime(MethodExecuteStreamingSql, SimulatedExecutionTime{MinimumExecutionTime: 300 * time.Millisecond})
	server.TestSpanner.PutExecutionTime(MethodExecuteSql, SimulatedExecutionTime{
		Errors:    []error{status.Error(codes.Internal, "prime failed")},
		KeepError: true,
	})

	var g errgroup.Group
	for i := 0; i < 3; i++ {
		g.Go(func() error { return drainDCPQuery(context.Background(), client) })
	}
	waitFor(t, func() error {
		if got := client.sc.dynamicPool.totalRPCLoad.Load(); got == 0 {
			return fmt.Errorf("DCP total RPC load = %d, want in-flight workload", got)
		}
		return nil
	})
	client.sc.dynamicPool.scaleUp()
	if got, want := client.sc.dynamicPool.Num(), 1; got != want {
		t.Fatalf("DCP channel count after failed prime = %d, want %d", got, want)
	}
	for _, e := range client.sc.dynamicPool.getEntries() {
		if e.state.Load() != dcpStateActive {
			t.Fatalf("active slice contains non-active entry state=%d", e.state.Load())
		}
	}
	if _, _, err := client.sc.dynamicPool.pick(context.Background(), true); err != nil {
		t.Fatalf("pick after failed scale-up failed: %v", err)
	}
	if err := g.Wait(); err != nil {
		t.Fatalf("query workload failed: %v", err)
	}
}

func TestDynamicChannelPoolPowerOfTwoPrefersLeastLoadedEntry(t *testing.T) {
	_, client, teardown := setupDCPMockedTestServer(t, testDCPConfig(3, 3, 3))
	defer teardown()
	p := client.sc.dynamicPool
	entries := p.getEntries()
	entries[1].unaryLoad.Store(100)
	entries[2].operationRefs.Store(100)

	counts := map[uint64]int{}
	for i := 0; i < 2000; i++ {
		e, _, err := p.pick(context.Background(), false)
		if err != nil {
			t.Fatalf("pick failed: %v", err)
		}
		counts[e.id]++
	}
	low := counts[entries[0].id]
	high := counts[entries[1].id] + counts[entries[2].id]
	if low <= high {
		t.Fatalf("least-loaded entry picked %d times, higher-load entries picked %d times; want least-loaded preference", low, high)
	}
}

func TestDynamicChannelPoolCloseClosesActiveAndDrainingEntries(t *testing.T) {
	_, client, teardown := setupDCPMockedTestServer(t, testDCPConfig(3, 3, 3))
	defer teardown()
	p := client.sc.dynamicPool
	entries := append([]*dcpEntry(nil), p.getEntries()...)
	entries[1].state.Store(dcpStateDraining)
	p.drainingCount.Add(1)

	client.Close()
	if got := p.Num(); got != 0 {
		t.Fatalf("DCP pool entries after close = %d, want 0", got)
	}
	for _, e := range entries {
		if got := e.state.Load(); got != dcpStateClosed {
			t.Fatalf("entry %d state after close = %d, want closed", e.id, got)
		}
	}
}

func TestDynamicChannelPoolRequestIDUsesEntryChannelID(t *testing.T) {
	interceptorTracker := newInterceptorTracker()
	clientOpts := []option.ClientOption{
		option.WithGRPCDialOption(grpc.WithUnaryInterceptor(interceptorTracker.unaryClientInterceptor)),
		option.WithGRPCDialOption(grpc.WithStreamInterceptor(interceptorTracker.streamClientInterceptor)),
	}
	dcpConfig := testDCPConfig(1, 1, 3)
	dcpConfig.DCPSelectionStrategy = DCPRoundRobin
	server, client, teardown := setupMockedTestServerWithConfigAndClientOptions(t, ClientConfig{
		DisableNativeMetrics:     true,
		DynamicChannelPoolConfig: dcpConfig,
	}, clientOpts)
	defer teardown()
	addSelect1Result(server)
	server.TestSpanner.PutExecutionTime(MethodExecuteStreamingSql, SimulatedExecutionTime{MinimumExecutionTime: 300 * time.Millisecond})

	var g errgroup.Group
	for i := 0; i < 4; i++ {
		g.Go(func() error { return drainDCPQuery(context.Background(), client) })
	}
	waitFor(t, func() error {
		if got := client.sc.dynamicPool.Num(); got <= 1 {
			return fmt.Errorf("DCP channel count = %d, want scale-up", got)
		}
		return nil
	})
	// Run enough post-scale-up public queries to cycle through the active entries
	// and observe the newly added DCP channel id.
	for i := 0; i < client.sc.dynamicPool.Num(); i++ {
		if err := drainDCPQuery(context.Background(), client); err != nil {
			t.Fatalf("post-scale-up query failed: %v", err)
		}
	}
	if err := g.Wait(); err != nil {
		t.Fatalf("query workload failed: %v", err)
	}

	observedChannelIDs := map[uint32]bool{}
	for _, segments := range interceptorTracker.streamClientRequestIDSegments {
		if segments.ChannelID == 0 {
			t.Fatal("request id channel id is zero")
		}
		observedChannelIDs[segments.ChannelID] = true
	}
	if len(observedChannelIDs) <= 1 {
		t.Fatalf("distinct DCP request-id channel ids = %v, want cardinality growth after scale-up", observedChannelIDs)
	}
	if err := interceptorTracker.validateRequestIDsMonotonicity(); err != nil {
		t.Fatal(err)
	}
}

func TestDynamicChannelPoolFullScanFallbackFindsOnlyActiveEntry(t *testing.T) {
	_, client, teardown := setupDCPMockedTestServer(t, testDCPConfig(4, 4, 4))
	defer teardown()
	p := client.sc.dynamicPool
	entries := p.getEntries()
	for _, e := range entries[:3] {
		e.state.Store(dcpStateDraining)
	}
	entries[3].unaryLoad.Store(7)

	e, err := p.pickLeastLoaded()
	if err != nil {
		t.Fatalf("pickLeastLoaded failed: %v", err)
	}
	if e != entries[3] {
		t.Fatalf("full-scan fallback returned entry %d, want only active entry %d", e.id, entries[3].id)
	}
	picked, release, err := p.pick(context.Background(), true)
	if err != nil {
		t.Fatalf("pick fallback failed: %v", err)
	}
	defer release()
	if picked != entries[3] {
		t.Fatalf("power-of-two fallback returned entry %d, want only active entry %d", picked.id, entries[3].id)
	}
	if got := picked.operationRefs.Load(); got != 1 {
		t.Fatalf("picked active entry operationRefs = %d, want 1", got)
	}
}

func TestDynamicChannelPoolPowerOfTwoSpreadDoesNotHerd(t *testing.T) {
	_, client, teardown := setupDCPMockedTestServer(t, testDCPConfig(4, 4, 4))
	defer teardown()
	p := client.sc.dynamicPool
	entries := p.getEntries()
	overloaded := entries[0]
	overloaded.operationRefs.Store(200)
	baseRefs := map[uint64]int32{}
	for _, e := range entries {
		baseRefs[e.id] = e.operationRefs.Load()
	}

	const workers = 400
	start := make(chan struct{})
	picked := make(chan *dcpEntry, workers)
	releaseAll := make(chan struct{})
	var wg sync.WaitGroup
	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			e, release, err := p.pick(context.Background(), true)
			if err != nil {
				picked <- nil
				return
			}
			picked <- e
			<-releaseAll
			release()
		}()
	}
	close(start)

	counts := map[uint64]int{}
	for i := 0; i < workers; i++ {
		e := <-picked
		if e == nil {
			t.Fatalf("worker pick failed")
		}
		counts[e.id]++
	}
	if got := counts[overloaded.id]; got > 60 {
		t.Fatalf("overloaded entry picked %d times, want <= 60; counts=%v", got, counts)
	}
	for _, e := range entries[1:] {
		if got := counts[e.id]; got < 70 {
			t.Fatalf("entry %d picked %d times, want spread across low-load entries; counts=%v", e.id, got, counts)
		}
	}
	var maxLow int
	for _, e := range entries[1:] {
		if got := counts[e.id]; got > maxLow {
			maxLow = got
		}
	}
	if maxLow > 190 {
		t.Fatalf("parallel power-of-two picks herded onto one low-load entry: maxLow=%d counts=%v", maxLow, counts)
	}
	for _, e := range entries {
		want := baseRefs[e.id] + int32(counts[e.id])
		if got := e.operationRefs.Load(); got != want {
			t.Fatalf("entry %d operationRefs = %d, want base+held picks %d", e.id, got, want)
		}
	}
	close(releaseAll)
	wg.Wait()
	for _, e := range entries {
		if got, want := e.operationRefs.Load(), baseRefs[e.id]; got != want {
			t.Fatalf("entry %d operationRefs after release = %d, want base %d", e.id, got, want)
		}
	}
}

func TestDynamicChannelPoolScaleUpHonorsMaxScaleUpPercent(t *testing.T) {
	cfg := testDCPConfig(4, 1, 10)
	cfg.DCPMaxScaleUpPercent = 25
	_, client, teardown := setupDCPMockedTestServer(t, cfg)
	defer teardown()
	p := client.sc.dynamicPool
	p.setPrimeSession(client.sm.multiplexedSession.id)
	for _, e := range p.getEntries() {
		e.unaryLoad.Store(10)
	}

	p.scaleUp()
	if got, want := p.Num(), 5; got != want {
		t.Fatalf("DCP channel count after percent-capped scale-up = %d, want %d", got, want)
	}
}

func TestDynamicChannelPoolScaleUpDialFailureDoesNotPublishEntry(t *testing.T) {
	_, client, teardown := setupDCPMockedTestServer(t, testDCPConfig(1, 1, 2))
	defer teardown()
	p := client.sc.dynamicPool
	p.setPrimeSession(client.sm.multiplexedSession.id)
	p.dial = func(context.Context) (gtransport.ConnPool, error) {
		return nil, status.Error(codes.Unavailable, "dial failed")
	}
	initialEntries := append([]*dcpEntry(nil), p.getEntries()...)
	p.getEntries()[0].unaryLoad.Store(10)

	p.scaleUp()
	if got, want := p.Num(), 1; got != want {
		t.Fatalf("DCP channel count after failed dial = %d, want %d", got, want)
	}
	if got := p.getEntries()[0]; got != initialEntries[0] {
		t.Fatalf("active entry pointer changed after failed dial")
	}
	if got := p.lastScaleUp.Load(); got != 0 {
		t.Fatalf("lastScaleUp after failed dial = %d, want 0", got)
	}
	for _, e := range p.getEntries() {
		if e.state.Load() != dcpStateActive {
			t.Fatalf("active slice contains non-active entry state=%d", e.state.Load())
		}
	}
}

func TestDynamicChannelPoolDirectPathFallbackSlotStaysPinnedAcrossFallback(t *testing.T) {
	state := &dcpFallbackState{}
	direct1 := &fakeDCPConnPool{}
	cloud1 := &fakeDCPConnPool{}
	direct2 := &fakeDCPConnPool{}
	cloud2 := &fakeDCPConnPool{}
	slot1 := &dcpFallbackSlot{id: 7, direct: direct1, cloud: cloud1, state: state}
	slot2 := &dcpFallbackSlot{id: 8, direct: direct2, cloud: cloud2, state: state}
	p := &dynamicChannelPool{cfg: testDCPConfig(2, 1, 2)}
	entry1 := &dcpEntry{id: slot1.id, pool: slot1, parent: p}
	entry2 := &dcpEntry{id: slot2.id, pool: slot2, parent: p}
	entry1.state.Store(dcpStateActive)
	entry2.state.Store(dcpStateActive)
	entries := []*dcpEntry{entry1, entry2}
	p.entries.Store(&entries)

	picked, release, err := p.pick(context.Background(), true)
	if err != nil {
		t.Fatalf("pick failed: %v", err)
	}
	defer release()
	if err := picked.pool.Invoke(context.Background(), "/test", nil, nil); err != nil {
		t.Fatalf("direct invoke failed: %v", err)
	}
	state.fallbackActive.Store(true)
	if err := picked.pool.Invoke(context.Background(), "/test", nil, nil); err != nil {
		t.Fatalf("fallback invoke failed: %v", err)
	}

	var pickedDirect, pickedCloud, otherDirect, otherCloud *fakeDCPConnPool
	if picked.id == slot1.id {
		pickedDirect, pickedCloud, otherDirect, otherCloud = direct1, cloud1, direct2, cloud2
	} else if picked.id == slot2.id {
		pickedDirect, pickedCloud, otherDirect, otherCloud = direct2, cloud2, direct1, cloud1
	} else {
		t.Fatalf("picked unexpected slot id %d", picked.id)
	}
	if got, want := pickedDirect.invokeCount, 1; got != want {
		t.Fatalf("picked direct invoke count = %d, want %d", got, want)
	}
	if got, want := pickedCloud.invokeCount, 1; got != want {
		t.Fatalf("picked cloud invoke count = %d, want %d", got, want)
	}
	if got := otherDirect.invokeCount + otherCloud.invokeCount; got != 0 {
		t.Fatalf("other slot invoke count = %d, want 0", got)
	}
}

func TestDynamicChannelPoolTakeMultiplexedRecycleReleasesOperationRef(t *testing.T) {
	_, client, teardown := setupDCPMockedTestServer(t, testDCPConfig(1, 1, 2))
	defer teardown()

	sh, err := client.sm.takeMultiplexed(context.Background())
	if err != nil {
		t.Fatalf("takeMultiplexed failed: %v", err)
	}
	if got, want := totalDCPOperationRefs(client.sc.dynamicPool), int32(1); got != want {
		t.Fatalf("operation refs after takeMultiplexed = %d, want %d", got, want)
	}
	sh.recycle()
	if got := totalDCPOperationRefs(client.sc.dynamicPool); got != 0 {
		t.Fatalf("operation refs after sessionHandle.recycle = %d, want 0", got)
	}
	sh.recycle()
	if got := totalDCPOperationRefs(client.sc.dynamicPool); got != 0 {
		t.Fatalf("operation refs after second sessionHandle.recycle = %d, want 0", got)
	}
}

func TestDynamicChannelPoolOperationRefsReleasedOnCanceledQuery(t *testing.T) {
	server, client, teardown := setupDCPMockedTestServer(t, testDCPConfig(1, 1, 2))
	defer teardown()
	server.TestSpanner.PutExecutionTime(MethodExecuteStreamingSql, SimulatedExecutionTime{MinimumExecutionTime: time.Second})

	ctx, cancel := context.WithCancel(context.Background())
	iter := client.Single().Query(ctx, NewStatement(SelectSingerIDAlbumIDAlbumTitleFromAlbums))
	done := make(chan error, 1)
	go func() {
		_, err := iter.Next()
		done <- err
	}()
	waitFor(t, func() error {
		if got := totalDCPOperationRefs(client.sc.dynamicPool); got == 0 {
			return fmt.Errorf("operation refs during query = %d, want > 0", got)
		}
		return nil
	})
	cancel()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("canceled query did not return")
	}
	iter.Stop()
	waitFor(t, func() error {
		if got := totalDCPOperationRefs(client.sc.dynamicPool); got != 0 {
			return fmt.Errorf("operation refs after canceled query = %d, want 0", got)
		}
		return nil
	})
}

func TestDynamicChannelPoolConfigDefaultsInitialChannelsToMinWhenInitialUnset(t *testing.T) {
	cfg, err := normalizeDCPConfig(DynamicChannelPoolConfig{DCPEnabled: true, DCPMinChannels: 8, DCPMaxChannels: 10})
	if err != nil {
		t.Fatalf("normalizeDCPConfig failed: %v", err)
	}
	if got, want := cfg.DCPInitialChannels, 8; got != want {
		t.Fatalf("DCPInitialChannels = %d, want min channels %d", got, want)
	}
}

func TestDynamicChannelPoolConfigRejectsExplicitInitialBelowMin(t *testing.T) {
	_, err := normalizeDCPConfig(DynamicChannelPoolConfig{DCPEnabled: true, DCPInitialChannels: 4, DCPMinChannels: 8, DCPMaxChannels: 10})
	if err == nil {
		t.Fatal("normalizeDCPConfig succeeded, want error")
	}
}

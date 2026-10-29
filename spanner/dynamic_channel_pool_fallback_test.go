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
	"errors"
	"fmt"
	"io"
	"log"
	"net"
	"sync/atomic"
	"testing"
	"time"

	. "cloud.google.com/go/spanner/internal/testutil"
	"google.golang.org/api/option"
	gtransport "google.golang.org/api/transport/grpc"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/connectivity"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/status"
)

// dcpFallbackTestConfig is a DCP of two entries that neither scales up nor
// down by itself, so that only the test changes entries.
func dcpFallbackTestConfig() DynamicChannelPoolConfig {
	return DynamicChannelPoolConfig{
		DCPEnabled:                true,
		DCPInitialChannels:        2,
		DCPMinChannels:            1,
		DCPMaxChannels:            2,
		DCPScaleDownCheckInterval: time.Hour,
		DCPPrimeTimeout:           5 * time.Second,
		DCPPrimeMaxAttempts:       1,
	}
}

func dynamicPoolOf(t *testing.T, client *Client) *dynamicChannelPool {
	t.Helper()
	p := client.sc.dynamicPool
	if p == nil {
		t.Fatal("dynamic channel pool not enabled")
	}
	return p
}

func TestDCPDirectPathFallbackDefaults(t *testing.T) {
	skipDialectRerun(t)
	tests := []struct {
		name     string
		fallback string
		want     bool
	}{
		{name: "fallback is off by default", fallback: "", want: false},
		{name: "fallback enabled by environment", fallback: "true", want: true},
		{name: "fallback disabled by environment", fallback: "false", want: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv("GOOGLE_SPANNER_ENABLE_DIRECT_ACCESS", "true")
			t.Setenv("GOOGLE_SPANNER_ENABLE_GCP_FALLBACK", tt.fallback)
			paths := newSharedBackendPaths(t)
			client := newDirectPathFallbackTestClient(t, ClientConfig{DynamicChannelPoolConfig: dcpFallbackTestConfig()}, paths.opts...)
			p := dynamicPoolOf(t, client)
			if got := p.fallback != nil; got != tt.want {
				t.Fatalf("fallback enabled = %v, want %v", got, tt.want)
			}
			for _, e := range p.getEntries() {
				slot, ok := e.pool.(*channelSlot)
				if !ok {
					t.Fatalf("entry pool type = %T, want *channelSlot", e.pool)
				}
				if slot.id != e.id || slot.fallback != p.fallback {
					t.Fatalf("entry %d slot = {id: %d, fallback: %p}, want {id: %d, fallback: %p}", e.id, slot.id, slot.fallback, e.id, p.fallback)
				}
			}
		})
	}
}

func TestDCPDirectPathFallbackKeepsTransactionOnEntry(t *testing.T) {
	skipDialectRerun(t)
	enableDirectPathForTest(t)
	paths := newSharedBackendPaths(t)
	client := newDirectPathFallbackTestClient(t, ClientConfig{DynamicChannelPoolConfig: dcpFallbackTestConfig()}, paths.opts...)
	p := dynamicPoolOf(t, client)

	directBefore := len(paths.direct.recorded())
	_, err := client.ReadWriteTransaction(context.Background(), func(ctx context.Context, tx *ReadWriteTransaction) error {
		if _, err := tx.Update(ctx, NewStatement(UpdateBarSetFoo)); err != nil {
			return err
		}
		if !p.fallback.current().fallback {
			forceDirectPathFallback(t, p.fallback)
		}
		_, err := tx.Update(ctx, NewStatement(UpdateBarSetFoo))
		return err
	})
	if err != nil {
		t.Fatalf("ReadWriteTransaction() failed: %v", err)
	}

	direct := paths.direct.recorded()[directBefore:]
	cloud := paths.cloud.recorded()
	if len(direct) != 1 || direct[0].method != executeSQLMethod {
		t.Fatalf("DirectPath RPCs of the transaction = %+v, want the first ExecuteSql", direct)
	}
	if len(cloud) != 2 || cloud[0].method != executeSQLMethod || cloud[1].method != commitMethod {
		t.Fatalf("CloudPath RPCs of the transaction = %+v, want ExecuteSql and Commit", cloud)
	}
	entry := direct[0].channelID
	found := false
	for _, e := range p.getEntries() {
		found = found || e.id == entry
	}
	if !found {
		t.Fatalf("transaction channel %d is not a DCP entry", entry)
	}
	for _, rpc := range cloud {
		if rpc.channelID != entry {
			t.Fatalf("CloudPath RPC %s ran on channel %d, want the transaction's entry %d", rpc.method, rpc.channelID, entry)
		}
	}
	if got, want := paths.cloud.conns.Load(), int64(1); got != want {
		t.Fatalf("CloudPath connections = %d, want %d", got, want)
	}
}

func TestDCPDirectPathFallbackPrimesNewEntriesOverCloudPath(t *testing.T) {
	skipDialectRerun(t)
	enableDirectPathForTest(t)
	paths := newSharedBackendPaths(t)
	addSelect1Result(paths.backend)
	cfg := dcpFallbackTestConfig()
	cfg.DCPMaxChannels = 3
	client := newDirectPathFallbackTestClient(t, ClientConfig{DynamicChannelPoolConfig: cfg}, paths.opts...)
	p := dynamicPoolOf(t, client)

	forceDirectPathFallback(t, p.fallback)
	directBefore := paths.direct.count(executeSQLMethod)
	e, err := p.newEntry(context.Background(), true)
	if err != nil {
		t.Fatalf("newEntry() after the switch failed: %v", err)
	}
	defer e.close()
	if got, want := paths.cloud.count(executeSQLMethod), 1; got != want {
		t.Fatalf("CloudPath priming queries = %d, want %d", got, want)
	}
	if got := paths.direct.count(executeSQLMethod); got != directBefore {
		t.Fatalf("DirectPath priming queries = %d, want %d", got-directBefore, 0)
	}
}

func TestDCPDirectPathFallbackClearsErrorPenalties(t *testing.T) {
	skipDialectRerun(t)
	enableDirectPathForTest(t)
	paths := newSharedBackendPaths(t)
	client := newDirectPathFallbackTestClient(t, ClientConfig{DynamicChannelPoolConfig: dcpFallbackTestConfig()}, paths.opts...)
	p := dynamicPoolOf(t, client)

	for _, e := range p.getEntries() {
		e.applyErrorPenalty(status.Error(codes.Unavailable, "DirectPath failed"))
		if e.currentPenalty() == 0 {
			t.Fatalf("entry %d has no penalty after an UNAVAILABLE error", e.id)
		}
	}
	forceDirectPathFallback(t, p.fallback)
	for _, e := range p.getEntries() {
		if got := e.currentPenalty(); got != 0 {
			t.Fatalf("entry %d penalty after the switch = %d, want 0", e.id, got)
		}
	}
	if got := p.totalPenaltyLoad.Load(); got != 0 {
		t.Fatalf("pool penalty load after the switch = %d, want 0", got)
	}
}

func TestDCPDirectPathUnreachableFallsBackToCloudPath(t *testing.T) {
	skipDialectRerun(t)
	enableDirectPathForTest(t)
	oldPeriod := directPathFallbackPeriod
	directPathFallbackPeriod = 10 * time.Millisecond
	t.Cleanup(func() { directPathFallbackPeriod = oldPeriod })

	paths := newSharedBackendPaths(t)
	lis, err := net.Listen("tcp", "localhost:0")
	if err != nil {
		t.Fatal(err)
	}
	unreachable := lis.Addr().String()
	lis.Close()

	config := ClientConfig{DisableNativeMetrics: true, Logger: log.New(io.Discard, "", 0), DynamicChannelPoolConfig: dcpFallbackTestConfig(), CallOptions: fastSessionRetry()}
	client, err := NewClientWithConfig(context.Background(), "projects/p/instances/i/databases/d", config,
		option.WithEndpoint(unreachable),
		option.WithGRPCDialOption(grpc.WithTransportCredentials(insecure.NewCredentials())),
		option.WithoutAuthentication(),
	)
	if err != nil {
		t.Fatalf("NewClientWithConfig() failed: %v", err)
	}
	defer client.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	iter := client.Single().Query(ctx, NewStatement(SelectFooFromBar))
	defer iter.Stop()
	rows := 0
	if err := iter.Do(func(*Row) error { rows++; return nil }); err != nil {
		t.Fatalf("query with DirectPath unreachable failed: %v", err)
	}
	if rows == 0 {
		t.Fatal("query returned no rows")
	}
	if !dynamicPoolOf(t, client).fallback.current().fallback {
		t.Fatal("pool did not fall back to CloudPath")
	}
	if got := paths.cloud.count(createSessionMethod); got == 0 {
		t.Fatal("the multiplexed session was not created over CloudPath")
	}
}

func dcpOneEntryFallbackTestConfig() DynamicChannelPoolConfig {
	cfg := dcpFallbackTestConfig()
	cfg.DCPInitialChannels = 1
	cfg.DCPMaxChannels = 1
	return cfg
}

func TestDCPBlockingCloudPathDialHonorsQueryDeadline(t *testing.T) {
	skipDialectRerun(t)
	enableDirectPathForTest(t)
	paths := newSharedBackendPaths(t)
	opts := append(append([]option.ClientOption{}, paths.opts...), option.WithGRPCDialOption(grpc.WithBlock()))
	started := redirectCloudPathTo(t, unreachableAddr(t))
	client := newDirectPathFallbackTestClient(t, ClientConfig{DynamicChannelPoolConfig: dcpOneEntryFallbackTestConfig()}, opts...)
	forceDirectPathFallback(t, dynamicPoolOf(t, client).fallback)
	queryHonorsDeadlineDuringCloudPathDial(t, client, started)
}

func TestDCPPrimingHonorsTimeoutDuringBlockingCloudPathDial(t *testing.T) {
	skipDialectRerun(t)
	enableDirectPathForTest(t)
	paths := newSharedBackendPaths(t)
	addSelect1Result(paths.backend)
	cfg := dcpFallbackTestConfig()
	cfg.DCPMaxChannels = 3
	cfg.DCPPrimeTimeout = 20 * time.Millisecond
	opts := append(append([]option.ClientOption{}, paths.opts...), option.WithGRPCDialOption(grpc.WithBlock()))
	redirectCloudPathTo(t, unreachableAddr(t))
	client := newDirectPathFallbackTestClient(t, ClientConfig{DynamicChannelPoolConfig: cfg}, opts...)
	p := dynamicPoolOf(t, client)
	forceDirectPathFallback(t, p.fallback)

	start := time.Now()
	e, err := p.newEntry(context.Background(), true)
	if err == nil {
		e.close()
		t.Fatal("newEntry() primed over an unreachable CloudPath")
	}
	if ErrCode(err) != codes.DeadlineExceeded {
		t.Fatalf("newEntry() error = %v, want DEADLINE_EXCEEDED", err)
	}
	if elapsed := time.Since(start); elapsed > 2*time.Second {
		t.Fatalf("priming took %v with a %v prime timeout", elapsed, cfg.DCPPrimeTimeout)
	}
}

func TestDCPDirectPathFallbackClosesCallerSuppliedPoolOnce(t *testing.T) {
	skipDialectRerun(t)
	for _, tt := range []struct {
		name     string
		doSwitch bool
	}{
		{name: "after a switch", doSwitch: true},
		{name: "without a switch", doSwitch: false},
	} {
		t.Run(tt.name, func(t *testing.T) {
			enableDirectPathForTest(t)
			paths := newSharedBackendPaths(t)
			userPool := &closeCountingConnPool{ConnPool: dialTestPath(t, paths.opts)}
			client := newDirectPathFallbackTestClient(t, ClientConfig{DynamicChannelPoolConfig: dcpOneEntryFallbackTestConfig()}, gtransport.WithConnPool(userPool))
			if tt.doSwitch {
				forceDirectPathFallback(t, dynamicPoolOf(t, client).fallback)
			}
			queryFooFromBar(t, client)
			client.Close()
			if got := userPool.closes.Load(); got != 1 {
				t.Fatalf("caller-supplied pool closes = %d, want 1", got)
			}
		})
	}
}

// TestDCPLateDirectPathStreamErrorDoesNotPenalizeCloudPath starts a query on
// DirectPath, switches while its stream is open, and lets the old stream fail
// after the switch. The query resumes over CloudPath; the late DirectPath
// failure must not penalize the entry's CloudPath side.
func TestDCPLateDirectPathStreamErrorDoesNotPenalizeCloudPath(t *testing.T) {
	skipDialectRerun(t)
	enableDirectPathForTest(t)
	paths := newSharedBackendPaths(t)
	client := newDirectPathFallbackTestClient(t, ClientConfig{DynamicChannelPoolConfig: dcpOneEntryFallbackTestConfig()}, paths.opts...)
	p := dynamicPoolOf(t, client)
	// The first DirectPath query stream stays open until the test releases
	// it, then fails.
	release := make(chan struct{})
	var held atomic.Bool
	paths.direct.setStreamHold(func(method string) error {
		if method != executeStreamingSQLMethod || !held.CompareAndSwap(false, true) {
			return nil
		}
		<-release
		return status.Error(codes.Unavailable, "old DirectPath attempt failed after the switch")
	})
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	done := make(chan error, 1)
	go func() {
		iter := client.Single().Query(ctx, NewStatement(SelectFooFromBar))
		defer iter.Stop()
		done <- iter.Do(func(*Row) error { return nil })
	}()
	waitFor(t, func() error {
		if paths.direct.count(executeStreamingSQLMethod) == 0 {
			return errors.New("query not started")
		}
		return nil
	})
	forceDirectPathFallback(t, p.fallback)
	close(release)
	if err := <-done; err != nil {
		t.Fatalf("query failed: %v", err)
	}
	if paths.cloud.count(executeStreamingSQLMethod) == 0 {
		t.Fatal("query did not resume over CloudPath")
	}
	if got := p.totalPenaltyLoad.Load(); got != 0 {
		t.Fatalf("pool penalty load after a late DirectPath failure = %d, want 0", got)
	}
}

func TestDCPPenaltyFollowsTheLastAttemptsPath(t *testing.T) {
	skipDialectRerun(t)
	errUnavailable := status.Error(codes.Unavailable, "unavailable")
	tests := []struct {
		name string
		// lastAttemptOnCloudPath retries the operation after the switch.
		lastAttemptOnCloudPath bool
		wantPenalty            bool
	}{
		{name: "late DirectPath failure after the switch", lastAttemptOnCloudPath: false, wantPenalty: false},
		{name: "retry failed on CloudPath after the switch", lastAttemptOnCloudPath: true, wantPenalty: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			enableDirectPathForTest(t)
			paths := newSharedBackendPaths(t)
			client := newDirectPathFallbackTestClient(t, ClientConfig{DynamicChannelPoolConfig: dcpOneEntryFallbackTestConfig()}, paths.opts...)
			p := dynamicPoolOf(t, client)
			e := p.getEntries()[0]
			slot := e.pool.(*channelSlot)

			ctx, done := e.client.(*dcpSpannerClient).startUnary(context.Background())
			if _, _, err := slot.route(ctx); err != nil { // first attempt, on DirectPath
				t.Fatal(err)
			}
			forceDirectPathFallback(t, p.fallback)
			if tt.lastAttemptOnCloudPath {
				if _, _, err := slot.route(ctx); err != nil { // retry, on CloudPath
					t.Fatal(err)
				}
			}
			done(errUnavailable)
			if got := e.currentPenalty() > 0; got != tt.wantPenalty {
				t.Fatalf("entry penalized = %v, want %v", got, tt.wantPenalty)
			}
		})
	}
}

// TestDCPSwitchKeepsPenaltiesEarnedOnCloudPath covers a CloudPath failure that
// lands after the switch is published but before the switch hook clears the
// DirectPath penalties: the hook must keep it.
func TestDCPSwitchKeepsPenaltiesEarnedOnCloudPath(t *testing.T) {
	skipDialectRerun(t)
	enableDirectPathForTest(t)
	paths := newSharedBackendPaths(t)
	client := newDirectPathFallbackTestClient(t, ClientConfig{DynamicChannelPoolConfig: dcpFallbackTestConfig()}, paths.opts...)
	p := dynamicPoolOf(t, client)
	forceDirectPathFallback(t, p.fallback)
	e := p.getEntries()[0]
	e.applyErrorPenalty(status.Error(codes.Unavailable, "CloudPath failed"))
	want := e.currentPenalty()
	if want == 0 {
		t.Fatal("CloudPath failure did not penalize the entry")
	}
	p.clearErrorPenalties() // the switch hook running late
	if got := e.currentPenalty(); got != want {
		t.Fatalf("CloudPath penalty after the switch hook = %d, want %d", got, want)
	}
	if got := p.totalPenaltyLoad.Load(); got != int64(want) {
		t.Fatalf("pool penalty load = %d, want %d", got, want)
	}
}

// TestDCPDrainAndFallbackKeepTransactionOnEntry drains a transaction's entry
// and switches to CloudPath between two statements. The rest of the
// transaction stays on the entry, now over CloudPath, and closing the drained
// entry shuts down both of its connections.
func TestDCPDrainAndFallbackKeepTransactionOnEntry(t *testing.T) {
	skipDialectRerun(t)
	enableDirectPathForTest(t)
	paths := newSharedBackendPaths(t)
	client := newDirectPathFallbackTestClient(t, ClientConfig{DynamicChannelPoolConfig: dcpFallbackTestConfig()}, paths.opts...)
	p := dynamicPoolOf(t, client)
	var bound *dcpEntry
	var channelID uint64
	_, err := client.ReadWriteTransaction(context.Background(), func(ctx context.Context, tx *ReadWriteTransaction) error {
		if _, err := tx.Update(ctx, NewStatement(UpdateBarSetFoo)); err != nil {
			return err
		}
		if bound == nil {
			rpcs := paths.direct.recorded()
			channelID = rpcs[len(rpcs)-1].channelID
			for _, e := range p.getEntries() {
				if e.id == channelID {
					bound = e
				}
			}
			if bound == nil {
				return fmt.Errorf("transaction channel %d is not a DCP entry", channelID)
			}
			removeDCPEntryForTest(p, bound)
			t.Cleanup(func() {
				bound.close()
				p.drainingCount.Add(-1)
			})
			forceDirectPathFallback(t, p.fallback)
		}
		_, err := tx.Update(ctx, NewStatement(UpdateBarSetFoo))
		return err
	})
	if err != nil {
		t.Fatalf("ReadWriteTransaction() failed: %v", err)
	}
	cloud := paths.cloud.recorded()
	if len(cloud) != 2 {
		t.Fatalf("CloudPath RPCs = %+v, want the second statement and Commit", cloud)
	}
	for _, rpc := range cloud {
		if rpc.channelID != channelID {
			t.Fatalf("CloudPath RPC %s ran on channel %d, want the drained entry %d", rpc.method, rpc.channelID, channelID)
		}
	}
	slot := bound.pool.(*channelSlot)
	direct, cloudConn := slot.direct.Conn(), slot.cloud.Load().Conn()
	if !bound.closeIfIdle(0) {
		t.Fatal("drained entry did not close")
	}
	if direct.GetState() != connectivity.Shutdown || cloudConn.GetState() != connectivity.Shutdown {
		t.Fatalf("connection states after close = {direct: %v, cloud: %v}, want Shutdown", direct.GetState(), cloudConn.GetState())
	}
}

// TestDCPPrimingDoesNotFeedFallbackSwitch fails every priming query of a
// scaled-up DirectPath channel. The channel is discarded; its failures must
// not count toward moving the whole pool to CloudPath.
func TestDCPPrimingDoesNotFeedFallbackSwitch(t *testing.T) {
	skipDialectRerun(t)
	enableDirectPathForTest(t)
	paths := newSharedBackendPaths(t)
	addSelect1Result(paths.backend)
	cfg := dcpFallbackTestConfig()
	cfg.DCPMaxChannels = 3
	cfg.DCPPrimeTimeout = 50 * time.Millisecond
	client := newDirectPathFallbackTestClient(t, ClientConfig{DynamicChannelPoolConfig: cfg}, paths.opts...)
	p := dynamicPoolOf(t, client)
	p.fallback.closeWindow(time.Now())

	paths.backend.TestSpanner.PutExecutionTime(MethodExecuteSql, SimulatedExecutionTime{
		Errors:    []error{status.Error(codes.Unavailable, "new channel lands on a broken frontend")},
		KeepError: true,
	})
	if e, err := p.newEntry(context.Background(), true); err == nil {
		e.close()
		t.Fatal("newEntry() primed a channel whose priming always fails")
	}
	if got := paths.direct.count(executeSQLMethod); got == 0 {
		t.Fatal("priming sent no query")
	}
	if res := p.fallback.closeWindow(time.Now()); res.failures != 0 || res.switched {
		t.Fatalf("priming failures fed the fallback window: %+v", res)
	}
}

// TestDCPEntryAfterSwitchDialsOnlyCloudPath creates an entry after the pool
// has fallen back: it must not dial DirectPath, and closing it closes its
// one CloudPath connection once.
func TestDCPEntryAfterSwitchDialsOnlyCloudPath(t *testing.T) {
	skipDialectRerun(t)
	enableDirectPathForTest(t)
	paths := newSharedBackendPaths(t)
	addSelect1Result(paths.backend)
	cfg := dcpFallbackTestConfig()
	cfg.DCPMaxChannels = 3
	client := newDirectPathFallbackTestClient(t, ClientConfig{DynamicChannelPoolConfig: cfg}, paths.opts...)
	p := dynamicPoolOf(t, client)
	forceDirectPathFallback(t, p.fallback)

	directDials := 0
	dial := p.dial
	p.dial = func(ctx context.Context) (gtransport.ConnPool, error) {
		directDials++
		return dial(ctx)
	}
	cloudDialsBefore := paths.cloudDials.Load()
	e, err := p.newEntry(context.Background(), true)
	if err != nil {
		t.Fatalf("newEntry() after the switch failed: %v", err)
	}
	if directDials != 0 {
		t.Fatalf("DirectPath dials for an entry created after the switch = %d, want 0", directDials)
	}
	if got := paths.cloudDials.Load() - cloudDialsBefore; got != 1 {
		t.Fatalf("CloudPath dials = %d, want 1", got)
	}
	if got, want := paths.cloud.count(executeSQLMethod), 1; got != want {
		t.Fatalf("CloudPath priming queries = %d, want %d", got, want)
	}
	slot := e.pool.(*channelSlot)
	conn := slot.direct.Conn()
	if got := slot.Conn(); got != conn {
		t.Fatal("entry created after the switch does not use its CloudPath connection")
	}
	e.close()
	if got := conn.GetState(); got != connectivity.Shutdown {
		t.Fatalf("CloudPath connection state after close = %v, want Shutdown", got)
	}
}

// TestDCPEntryAfterSwitchWithCallerSuppliedPool creates an entry after a pool
// over a caller-supplied pool has fallen back. The caller's pool has no
// separate CloudPath, so the entry uses it as its direct side and is primed
// over it, and nothing reaches CloudPath.
func TestDCPEntryAfterSwitchWithCallerSuppliedPool(t *testing.T) {
	skipDialectRerun(t)
	enableDirectPathForTest(t)
	paths := newSharedBackendPaths(t)
	addSelect1Result(paths.backend)
	userPool, err := gtransport.DialPool(context.Background(), paths.opts...)
	if err != nil {
		t.Fatal(err)
	}
	cfg := dcpFallbackTestConfig()
	cfg.DCPMaxChannels = 3
	client := newDirectPathFallbackTestClient(t, ClientConfig{DynamicChannelPoolConfig: cfg}, gtransport.WithConnPool(userPool))
	p := dynamicPoolOf(t, client)
	forceDirectPathFallback(t, p.fallback)

	e, err := p.newEntry(context.Background(), true)
	if err != nil {
		t.Fatalf("newEntry() after the switch failed: %v", err)
	}
	if slot := e.pool.(*channelSlot); !sameConnPool(slot.direct, userPool) {
		t.Fatalf("entry direct side = %T, want the caller's pool", slot.direct)
	}
	if got := paths.direct.count(executeSQLMethod); got == 0 {
		t.Fatal("the entry was not primed over the caller's pool")
	}
	if got := paths.cloud.conns.Load(); got != 0 {
		t.Fatalf("CloudPath connections = %d, want 0", got)
	}
	if got := len(paths.cloud.recorded()); got != 0 {
		t.Fatalf("CloudPath front door RPCs = %d, want 0", got)
	}
}

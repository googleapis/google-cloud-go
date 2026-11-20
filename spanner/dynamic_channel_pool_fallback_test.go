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
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"cloud.google.com/go/spanner/apiv1/spannerpb"
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

func addPrimedDCPEntry(t *testing.T, client *Client) *dcpEntry {
	t.Helper()
	p := dynamicPoolOf(t, client)
	waitForDCPScaleUpWorkerIdle(p)
	before := p.GetEntries()
	if got, want := p.Cfg.DCPMaxChannels, len(before)+1; got != want {
		t.Fatalf("DCPMaxChannels = %d, want %d to scale up by one entry", got, want)
	}
	client.sm.mu.Lock()
	p.PrimeSession.Store(client.sm.multiplexedSession.id)
	client.sm.mu.Unlock()
	p.LastScaleUp.Store(0)
	load := int32(2 * p.Cfg.DCPMaxRPCPerChannel)
	for _, e := range before {
		e.UnaryLoad.Add(load)
	}
	p.ScaleUp()
	for _, e := range before {
		e.UnaryLoad.Add(-load)
	}
	after := p.GetEntries()
	if len(after) == len(before) {
		return nil
	}
	return after[len(after)-1]
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
			isolateChannelPoolEnv(t)
			t.Setenv(directAccessEnv, "true")
			t.Setenv(gcpFallbackEnv, tt.fallback)
			paths := newSharedBackendPaths(t)
			client := newDirectPathFallbackTestClient(t, ClientConfig{DynamicChannelPoolConfig: dcpFallbackTestConfig()}, paths.opts...)
			p := dynamicPoolOf(t, client)
			if got := poolFallback(p) != nil; got != tt.want {
				t.Fatalf("fallback enabled = %v, want %v", got, tt.want)
			}
			for _, e := range p.GetEntries() {
				slot, ok := e.Pool.(*channelSlot)
				if !ok {
					t.Fatalf("entry pool type = %T, want *channelSlot", e.Pool)
				}
				if slot.fallback != poolFallback(p) {
					t.Fatalf("entry %d slot fallback = %p, want %p", e.ID, slot.fallback, poolFallback(p))
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
		if !poolFallback(p).current().fallback {
			forceDirectPathFallback(t, poolFallback(p))
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
	for _, e := range p.GetEntries() {
		found = found || e.ID == entry
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

	forceDirectPathFallback(t, poolFallback(p))
	directBefore := paths.direct.count(executeSQLMethod)
	if addPrimedDCPEntry(t, client) == nil {
		t.Fatal("scale-up after the switch added no entry")
	}
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

	for _, e := range p.GetEntries() {
		e.ApplyErrorPenalty(status.Error(codes.Unavailable, "DirectPath failed"))
		if e.CurrentPenalty() == 0 {
			t.Fatalf("entry %d has no penalty after an UNAVAILABLE error", e.ID)
		}
	}
	forceDirectPathFallback(t, poolFallback(p))
	for _, e := range p.GetEntries() {
		if got := e.CurrentPenalty(); got != 0 {
			t.Fatalf("entry %d penalty after the switch = %d, want 0", e.ID, got)
		}
	}
	if got := p.TotalPenaltyLoad.Load(); got != 0 {
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
	if !poolFallback(dynamicPoolOf(t, client)).current().fallback {
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
	forceDirectPathFallback(t, poolFallback(dynamicPoolOf(t, client)))
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
	logger := &lockedBuffer{}
	client := newDirectPathFallbackTestClient(t, ClientConfig{DynamicChannelPoolConfig: cfg, Logger: log.New(logger, "", 0)}, opts...)
	p := dynamicPoolOf(t, client)
	forceDirectPathFallback(t, poolFallback(p))

	start := time.Now()
	if addPrimedDCPEntry(t, client) != nil {
		t.Fatal("scale-up added an entry over an unreachable CloudPath")
	}
	if got, want := logger.String(), `code = "DeadlineExceeded", desc = "spanner: CloudPath dial did not complete within 20ms"`; !strings.Contains(got, want) {
		t.Fatalf("scale-up log = %q, want it to contain %q", got, want)
	}
	if elapsed := time.Since(start); elapsed > 2*time.Second {
		t.Fatalf("priming took %v with a %v prime timeout", elapsed, cfg.DCPPrimeTimeout)
	}
}

// The dynamic pool closes a caller-supplied pool once per entry, so this test
// uses one entry: fallback must not add a close.
func TestDCPDirectPathFallbackDoesNotCloseCallerSuppliedPoolAgain(t *testing.T) {
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
				forceDirectPathFallback(t, poolFallback(dynamicPoolOf(t, client)))
			}
			queryFooFromBar(t, client)
			client.Close()
			if got := userPool.closes.Load(); got != 1 {
				t.Fatalf("caller-supplied pool closes = %d, want 1", got)
			}
		})
	}
}

func TestDCPLateDirectPathStreamErrorDoesNotPenalizeCloudPath(t *testing.T) {
	skipDialectRerun(t)
	enableDirectPathForTest(t)
	paths := newSharedBackendPaths(t)
	client := newDirectPathFallbackTestClient(t, ClientConfig{DynamicChannelPoolConfig: dcpOneEntryFallbackTestConfig()}, paths.opts...)
	p := dynamicPoolOf(t, client)
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
	forceDirectPathFallback(t, poolFallback(p))
	close(release)
	if err := <-done; err != nil {
		t.Fatalf("query failed: %v", err)
	}
	if paths.cloud.count(executeStreamingSQLMethod) == 0 {
		t.Fatal("query did not resume over CloudPath")
	}
	if got := p.TotalPenaltyLoad.Load(); got != 0 {
		t.Fatalf("pool penalty load after a late DirectPath failure = %d, want 0", got)
	}
}

func TestDCPPenaltySkipsOperationsThatStraddleTheSwitch(t *testing.T) {
	skipDialectRerun(t)
	errUnavailable := status.Error(codes.Unavailable, "unavailable")
	operations := []struct {
		name  string
		start func(c *dcpSpannerClient) (finish func(error))
	}{
		{name: "unary", start: func(c *dcpSpannerClient) func(error) {
			return c.startUnary(context.Background())
		}},
		{name: "stream", start: func(c *dcpSpannerClient) func(error) {
			return c.startStream(context.Background()).Done
		}},
	}
	tests := []struct {
		name              string
		startsAfterSwitch bool
		wantPenalty       bool
	}{
		{name: "started on DirectPath, failed after the switch", startsAfterSwitch: false, wantPenalty: false},
		{name: "started and failed after the switch", startsAfterSwitch: true, wantPenalty: true},
	}
	for _, op := range operations {
		for _, tt := range tests {
			t.Run(op.name+"/"+tt.name, func(t *testing.T) {
				enableDirectPathForTest(t)
				paths := newSharedBackendPaths(t)
				client := newDirectPathFallbackTestClient(t, ClientConfig{DynamicChannelPoolConfig: dcpOneEntryFallbackTestConfig()}, paths.opts...)
				p := dynamicPoolOf(t, client)
				e := p.GetEntries()[0]
				c := e.Client.(*dcpSpannerClient)

				var finish func(error)
				if !tt.startsAfterSwitch {
					finish = op.start(c)
				}
				forceDirectPathFallback(t, poolFallback(p))
				if tt.startsAfterSwitch {
					finish = op.start(c)
				}
				finish(errUnavailable)
				if got := e.CurrentPenalty() > 0; got != tt.wantPenalty {
					t.Fatalf("entry penalized = %v, want %v", got, tt.wantPenalty)
				}
				if got := p.TotalPenaltyLoad.Load() > 0; got != tt.wantPenalty {
					t.Fatalf("pool carries penalty load = %v, want %v", got, tt.wantPenalty)
				}
			})
		}
	}
}

type switchLogBarrier struct {
	reached chan struct{}
	release chan struct{}
	once    sync.Once
}

func (b *switchLogBarrier) Write(p []byte) (int, error) {
	if strings.Contains(string(p), "switching to CloudPath") {
		b.once.Do(func() { close(b.reached) })
		<-b.release
	}
	return len(p), nil
}

func TestDCPSwitchKeepsPenaltiesEarnedOnCloudPath(t *testing.T) {
	skipDialectRerun(t)
	enableDirectPathForTest(t)
	paths := newSharedBackendPaths(t)
	barrier := &switchLogBarrier{reached: make(chan struct{}), release: make(chan struct{})}
	var releaseOnce sync.Once
	release := func() { releaseOnce.Do(func() { close(barrier.release) }) }
	t.Cleanup(release)
	config := ClientConfig{DynamicChannelPoolConfig: dcpOneEntryFallbackTestConfig(), Logger: log.New(barrier, "", 0)}
	client := newDirectPathFallbackTestClient(t, config, paths.opts...)
	p := dynamicPoolOf(t, client)
	e := p.GetEntries()[0]
	f := poolFallback(p)

	paths.backend.TestSpanner.PutExecutionTime(MethodCreateSession, SimulatedExecutionTime{
		Errors:    []error{status.Error(codes.Unavailable, "unavailable")},
		KeepError: true,
	})
	failCreateSession := func() {
		t.Helper()
		_, err := spannerpb.NewSpannerClient(p).CreateSession(context.Background(), &spannerpb.CreateSessionRequest{Database: client.DatabaseName()})
		if status.Code(err) != codes.Unavailable {
			t.Fatalf("CreateSession() error = %v, want UNAVAILABLE", err)
		}
	}
	step := p.Cfg.DCPErrorPenaltyStep

	f.closeWindow(time.Now())
	failCreateSession()
	if got := e.CurrentPenalty(); got != step {
		t.Fatalf("penalty after a DirectPath failure = %d, want %d", got, step)
	}

	switched := make(chan struct{})
	go func() {
		defer close(switched)
		f.closeWindow(time.Now())
	}()
	select {
	case <-barrier.reached:
	case <-time.After(5 * time.Second):
		t.Fatal("the pool did not switch")
	}
	failCreateSession()
	if got, want := paths.cloud.count(createSessionMethod), 1; got != want {
		t.Fatalf("CloudPath CreateSession calls during the switch = %d, want %d", got, want)
	}
	release()
	<-switched

	if got := e.CurrentPenalty(); got != step {
		t.Fatalf("penalty after the switch = %d, want %d for the one CloudPath failure", got, step)
	}
	if got := p.TotalPenaltyLoad.Load(); got != int64(step) {
		t.Fatalf("pool penalty load after the switch = %d, want %d", got, step)
	}
}

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
			for _, e := range p.GetEntries() {
				if e.ID == channelID {
					bound = e
				}
			}
			if bound == nil {
				return fmt.Errorf("transaction channel %d is not a DCP entry", channelID)
			}
			removeDCPEntryForTest(p, bound)
			t.Cleanup(func() {
				bound.Close()
				p.DrainingCount.Add(-1)
			})
			forceDirectPathFallback(t, poolFallback(p))
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
	slot := bound.Pool.(*channelSlot)
	direct, cloudConn := slot.direct.Conn(), slot.cloud.Load().Conn()
	if !bound.CloseIfIdle(0) {
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
	poolFallback(p).closeWindow(time.Now())

	paths.backend.TestSpanner.PutExecutionTime(MethodExecuteSql, SimulatedExecutionTime{
		Errors:    []error{status.Error(codes.Unavailable, "new channel lands on a broken frontend")},
		KeepError: true,
	})
	if addPrimedDCPEntry(t, client) != nil {
		t.Fatal("scale-up added a channel whose priming always fails")
	}
	if got := paths.direct.count(executeSQLMethod); got == 0 {
		t.Fatal("priming sent no query")
	}
	if res := poolFallback(p).closeWindow(time.Now()); res.failures != 0 || res.switched {
		t.Fatalf("priming failures fed the fallback window: %+v", res)
	}
}

func TestDCPEntryAfterSwitchDialsOnlyCloudPath(t *testing.T) {
	skipDialectRerun(t)
	enableDirectPathForTest(t)
	paths := newSharedBackendPaths(t)
	addSelect1Result(paths.backend)
	cfg := dcpFallbackTestConfig()
	cfg.DCPMaxChannels = 3
	client := newDirectPathFallbackTestClient(t, ClientConfig{DynamicChannelPoolConfig: cfg}, paths.opts...)
	p := dynamicPoolOf(t, client)
	forceDirectPathFallback(t, poolFallback(p))

	cloudDialsBefore := paths.cloudDials.Load()
	e := addPrimedDCPEntry(t, client)
	if e == nil {
		t.Fatal("scale-up after the switch added no entry")
	}
	if got := paths.cloudDials.Load() - cloudDialsBefore; got != 1 {
		t.Fatalf("CloudPath dials = %d, want 1", got)
	}
	if got, want := paths.cloud.count(executeSQLMethod), 1; got != want {
		t.Fatalf("CloudPath priming queries = %d, want %d", got, want)
	}
	slot := e.Pool.(*channelSlot)
	if slot.direct != nil {
		t.Fatal("entry created after the switch has a direct connection")
	}
	conn := slot.Conn()
	e.Close()
	if got := conn.GetState(); got != connectivity.Shutdown {
		t.Fatalf("CloudPath connection state after close = %v, want Shutdown", got)
	}
}

// TestDCPEntryAfterSwitchKeepsItsDialContext checks that a CloudPath dial
// context outlives the Dial call and is canceled when the slot closes.
func TestDCPEntryAfterSwitchKeepsItsDialContext(t *testing.T) {
	skipDialectRerun(t)
	enableDirectPathForTest(t)
	paths := newSharedBackendPaths(t)
	cfg := dcpFallbackTestConfig()
	cfg.DCPMaxChannels = 3
	client := newDirectPathFallbackTestClient(t, ClientConfig{DynamicChannelPoolConfig: cfg}, paths.opts...)
	p := dynamicPoolOf(t, client)
	forceDirectPathFallback(t, poolFallback(p))

	var dialCtx context.Context
	dialCloud := poolFallback(p).dialCloud
	poolFallback(p).dialCloud = func(ctx context.Context) (gtransport.ConnPool, error) {
		dialCtx = ctx
		return dialCloud(ctx)
	}
	ctx, cancel := context.WithCancel(context.Background())
	slot, err := p.Dial(ctx)
	if err != nil {
		t.Fatalf("Dial() after the switch failed: %v", err)
	}
	cancel()
	if err := dialCtx.Err(); err != nil {
		t.Fatalf("CloudPath dial context after the creating call ended = %v, want it open", err)
	}
	slot.Close()
	if dialCtx.Err() == nil {
		t.Fatal("closing the slot did not end its CloudPath dial context")
	}
}

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
	forceDirectPathFallback(t, poolFallback(p))

	e := addPrimedDCPEntry(t, client)
	if e == nil {
		t.Fatal("scale-up after the switch added no entry")
	}
	if slot := e.Pool.(*channelSlot); !sameConnPool(slot.direct, userPool) {
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

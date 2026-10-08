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
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	vkit "cloud.google.com/go/spanner/apiv1"
	"cloud.google.com/go/spanner/apiv1/spannerpb"
	. "cloud.google.com/go/spanner/internal/testutil"
	"github.com/googleapis/gax-go/v2"
	"google.golang.org/api/iterator"
	"google.golang.org/api/option"
	gtransport "google.golang.org/api/transport/grpc"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/stats"
)

// frontDoor is one network path to a shared in-memory Spanner backend. It
// records the RPCs and connections it serves.
type frontDoor struct {
	addr  string
	conns atomic.Int64

	mu   sync.Mutex
	rpcs []frontDoorRPC
	// hold, when set, runs before every stream the front door serves; a
	// non-nil error ends the stream with that error instead.
	hold func(method string) error
}

func (d *frontDoor) setStreamHold(hold func(method string) error) {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.hold = hold
}

func (d *frontDoor) streamHold() func(method string) error {
	d.mu.Lock()
	defer d.mu.Unlock()
	return d.hold
}

type frontDoorRPC struct {
	method    string
	channelID uint64
}

func (d *frontDoor) record(ctx context.Context, method string) {
	rpc := frontDoorRPC{method: method}
	if md, ok := metadata.FromIncomingContext(ctx); ok {
		if ids := md.Get(xSpannerRequestIDHeader); len(ids) == 1 {
			// version.process.client.channel.request.attempt
			if parts := strings.Split(ids[0], "."); len(parts) == 6 {
				rpc.channelID, _ = strconv.ParseUint(parts[3], 10, 64)
			}
		}
	}
	d.mu.Lock()
	d.rpcs = append(d.rpcs, rpc)
	d.mu.Unlock()
}

func (d *frontDoor) recorded() []frontDoorRPC {
	d.mu.Lock()
	defer d.mu.Unlock()
	return append([]frontDoorRPC(nil), d.rpcs...)
}

func (d *frontDoor) count(method string) int {
	n := 0
	for _, rpc := range d.recorded() {
		if rpc.method == method {
			n++
		}
	}
	return n
}

func (d *frontDoor) serverOptions() []grpc.ServerOption {
	return []grpc.ServerOption{
		grpc.ChainUnaryInterceptor(func(ctx context.Context, req any, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (any, error) {
			d.record(ctx, info.FullMethod)
			return handler(ctx, req)
		}),
		grpc.ChainStreamInterceptor(func(srv any, ss grpc.ServerStream, info *grpc.StreamServerInfo, handler grpc.StreamHandler) error {
			d.record(ss.Context(), info.FullMethod)
			if hold := d.streamHold(); hold != nil {
				if err := hold(info.FullMethod); err != nil {
					return err
				}
			}
			return handler(srv, ss)
		}),
		grpc.StatsHandler(frontDoorConnCounter{d}),
	}
}

type frontDoorConnCounter struct{ d *frontDoor }

func (c frontDoorConnCounter) TagRPC(ctx context.Context, _ *stats.RPCTagInfo) context.Context {
	return ctx
}
func (c frontDoorConnCounter) HandleRPC(context.Context, stats.RPCStats) {}
func (c frontDoorConnCounter) TagConn(ctx context.Context, _ *stats.ConnTagInfo) context.Context {
	return ctx
}
func (c frontDoorConnCounter) HandleConn(_ context.Context, s stats.ConnStats) {
	if _, ok := s.(*stats.ConnBegin); ok {
		c.d.conns.Add(1)
	}
}

// sharedBackendPaths serves one in-memory Spanner backend through a DirectPath
// front door, which the client's options point at, and a CloudPath front door,
// which the CloudPath dial is redirected to. Both see the same sessions and
// transactions, the way both paths reach the same Spanner frontends.
type sharedBackendPaths struct {
	backend       *MockedSpannerInMemTestServer
	direct, cloud *frontDoor
	opts          []option.ClientOption
	cloudDials    atomic.Int32
}

func newSharedBackendPaths(t *testing.T) *sharedBackendPaths {
	t.Helper()
	p := &sharedBackendPaths{direct: &frontDoor{}, cloud: &frontDoor{}}
	backend, opts, teardown := NewMockedSpannerInMemTestServer(t, p.direct.serverOptions()...)
	t.Cleanup(teardown)
	p.backend, p.opts = backend, opts
	p.direct.addr = backend.ServerAddress

	cloudServer := grpc.NewServer(p.cloud.serverOptions()...)
	spannerpb.RegisterSpannerServer(cloudServer, backend.TestSpanner)
	lis, err := net.Listen("tcp", "localhost:0")
	if err != nil {
		t.Fatal(err)
	}
	go cloudServer.Serve(lis)
	t.Cleanup(cloudServer.Stop)
	p.cloud.addr = lis.Addr().String()
	p.redirectCloudPath(t)
	return p
}

// redirectCloudPath points the CloudPath dials of clients created after it at
// the CloudPath front door.
func (p *sharedBackendPaths) redirectCloudPath(t *testing.T) {
	t.Helper()
	old := dialDirectPathFallbackCloudPath
	dialDirectPathFallbackCloudPath = func(ctx context.Context, opts ...option.ClientOption) (gtransport.ConnPool, error) {
		p.cloudDials.Add(1)
		return gtransport.DialPool(ctx, append(opts,
			option.WithEndpoint(p.cloud.addr),
			option.WithGRPCDialOption(grpc.WithTransportCredentials(insecure.NewCredentials())),
			option.WithoutAuthentication(),
		)...)
	}
	t.Cleanup(func() { dialDirectPathFallbackCloudPath = old })
}

// enableDirectPathForTest turns DirectPath and its fallback on through the
// environment, which overrides ClientConfig. Off GCE, gtransport dials the
// endpoint without DirectPath, so the client reaches the DirectPath front door.
func enableDirectPathForTest(t *testing.T) {
	t.Setenv("GOOGLE_SPANNER_ENABLE_DIRECT_ACCESS", "true")
	t.Setenv("GOOGLE_SPANNER_ENABLE_GCP_FALLBACK", "true")
}

func newDirectPathFallbackTestClient(t *testing.T, config ClientConfig, opts ...option.ClientOption) *Client {
	t.Helper()
	config.DisableNativeMetrics = true
	config.Logger = log.New(io.Discard, "", 0)
	client, err := NewClientWithConfig(context.Background(), "projects/p/instances/i/databases/d", config, opts...)
	if err != nil {
		t.Fatalf("NewClientWithConfig() failed: %v", err)
	}
	t.Cleanup(client.Close)
	waitFor(t, func() error {
		client.sm.mu.Lock()
		defer client.sm.mu.Unlock()
		if client.sm.multiplexedSession == nil {
			return errInvalidSession
		}
		return nil
	})
	return client
}

func staticPoolOf(t *testing.T, client *Client) *staticChannelPool {
	t.Helper()
	pool, ok := client.sc.connPool.(*staticChannelPool)
	if !ok {
		t.Fatalf("connection pool type = %T, want *staticChannelPool", client.sc.connPool)
	}
	return pool
}

func queryFooFromBar(t *testing.T, client *Client) {
	t.Helper()
	iter := client.Single().Query(context.Background(), NewStatement(SelectFooFromBar))
	defer iter.Stop()
	rows := 0
	if err := iter.Do(func(*Row) error { rows++; return nil }); err != nil {
		t.Fatalf("query failed: %v", err)
	}
	if rows == 0 {
		t.Fatal("query returned no rows")
	}
}

func TestStaticPoolDirectPathFallbackDefaults(t *testing.T) {
	skipDialectRerun(t)
	tests := []struct {
		name         string
		directAccess string
		fallback     string
		want         bool
	}{
		{name: "DirectPath enables fallback by default", directAccess: "true", fallback: "", want: true},
		{name: "fallback disabled by environment", directAccess: "true", fallback: "false", want: false},
		{name: "no fallback without DirectPath", directAccess: "false", fallback: "true", want: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv("GOOGLE_SPANNER_ENABLE_DIRECT_ACCESS", tt.directAccess)
			t.Setenv("GOOGLE_SPANNER_ENABLE_GCP_FALLBACK", tt.fallback)
			paths := newSharedBackendPaths(t)
			client := newDirectPathFallbackTestClient(t, ClientConfig{NumChannels: 4}, paths.opts...)
			pool := staticPoolOf(t, client)
			if got := pool.fallback != nil; got != tt.want {
				t.Fatalf("fallback enabled = %v, want %v", got, tt.want)
			}
			if got, want := pool.Num(), 4; got != want {
				t.Fatalf("slot count = %d, want %d", got, want)
			}
		})
	}
}

func TestStaticPoolDirectPathFallbackDialsCloudPathLazily(t *testing.T) {
	skipDialectRerun(t)
	enableDirectPathForTest(t)
	paths := newSharedBackendPaths(t)
	client := newDirectPathFallbackTestClient(t, ClientConfig{NumChannels: 4}, paths.opts...)
	pool := staticPoolOf(t, client)

	// Before a switch the client holds NumChannels connections, all DirectPath.
	waitFor(t, func() error {
		if got := paths.direct.conns.Load(); got != 4 {
			return fmt.Errorf("DirectPath connections = %d, want 4", got)
		}
		return nil
	})
	queryFooFromBar(t, client)
	if got := paths.cloudDials.Load(); got != 0 {
		t.Fatalf("CloudPath dials before the switch = %d, want 0", got)
	}

	forceDirectPathFallback(t, pool.fallback)
	queryFooFromBar(t, client)
	if got, want := paths.cloud.count(executeStreamingSQLMethod), 1; got != want {
		t.Fatalf("CloudPath queries = %d, want %d", got, want)
	}
	// Only the slot that served the query dialed CloudPath.
	if got, want := paths.cloudDials.Load(), int32(1); got != want {
		t.Fatalf("CloudPath dials = %d, want %d", got, want)
	}
}

func TestStaticPoolDirectPathFallbackKeepsTransactionOnSlot(t *testing.T) {
	skipDialectRerun(t)
	enableDirectPathForTest(t)
	paths := newSharedBackendPaths(t)
	client := newDirectPathFallbackTestClient(t, ClientConfig{NumChannels: 4}, paths.opts...)
	pool := staticPoolOf(t, client)

	directBefore := len(paths.direct.recorded())
	_, err := client.ReadWriteTransaction(context.Background(), func(ctx context.Context, tx *ReadWriteTransaction) error {
		if _, err := tx.Update(ctx, NewStatement(UpdateBarSetFoo)); err != nil {
			return err
		}
		if !pool.fallback.current().fallback {
			// DirectPath fails between two statements of the transaction.
			forceDirectPathFallback(t, pool.fallback)
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
	slot := direct[0].channelID
	if slot == 0 {
		t.Fatal("transaction ran on channel 0, want a slot id")
	}
	for _, rpc := range cloud {
		if rpc.channelID != slot {
			t.Fatalf("CloudPath RPC %s ran on channel %d, want the transaction's slot %d", rpc.method, rpc.channelID, slot)
		}
	}
	if got, want := paths.cloud.conns.Load(), int64(1); got != want {
		t.Fatalf("CloudPath connections = %d, want %d", got, want)
	}
}

func TestStaticPoolDirectPathFallbackInFlightStreamStaysOnDirectPath(t *testing.T) {
	skipDialectRerun(t)
	enableDirectPathForTest(t)
	paths := newSharedBackendPaths(t)
	client := newDirectPathFallbackTestClient(t, ClientConfig{NumChannels: 4}, paths.opts...)
	pool := staticPoolOf(t, client)

	iter := client.Single().Query(context.Background(), NewStatement(SelectFooFromBar))
	defer iter.Stop()
	if _, err := iter.Next(); err != nil {
		t.Fatalf("first row failed: %v", err)
	}
	forceDirectPathFallback(t, pool.fallback)
	for {
		_, err := iter.Next()
		if errors.Is(err, iterator.Done) {
			break
		}
		if err != nil {
			t.Fatalf("Next() after the switch failed: %v", err)
		}
	}
	if got, want := paths.direct.count(executeStreamingSQLMethod), 1; got != want {
		t.Fatalf("DirectPath queries = %d, want %d", got, want)
	}
	if got := paths.cloud.count(executeStreamingSQLMethod); got != 0 {
		t.Fatalf("CloudPath queries for the in-flight stream = %d, want 0", got)
	}
	queryFooFromBar(t, client)
	if got, want := paths.cloud.count(executeStreamingSQLMethod), 1; got != want {
		t.Fatalf("CloudPath queries after the stream = %d, want %d", got, want)
	}
}

func TestStaticPoolDirectPathUnreachableFallsBackToCloudPath(t *testing.T) {
	skipDialectRerun(t)
	enableDirectPathForTest(t)
	oldPeriod := directPathFallbackPeriod
	directPathFallbackPeriod = 10 * time.Millisecond
	t.Cleanup(func() { directPathFallbackPeriod = oldPeriod })

	paths := newSharedBackendPaths(t)
	// DirectPath is unreachable: nothing listens on its endpoint.
	lis, err := net.Listen("tcp", "localhost:0")
	if err != nil {
		t.Fatal(err)
	}
	unreachable := lis.Addr().String()
	lis.Close()

	config := ClientConfig{NumChannels: 4, DisableNativeMetrics: true, Logger: log.New(io.Discard, "", 0), CallOptions: fastSessionRetry()}
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
	pool := staticPoolOf(t, client)
	if !pool.fallback.current().fallback {
		t.Fatal("pool did not fall back to CloudPath")
	}
	if got := paths.cloud.count(createSessionMethod); got == 0 {
		t.Fatal("the multiplexed session was not created over CloudPath")
	}
	if got, want := paths.cloud.count(executeStreamingSQLMethod), 1; got != want {
		t.Fatalf("CloudPath queries = %d, want %d", got, want)
	}
}

func TestStaticPoolDirectPathFallbackWithCallerSuppliedPool(t *testing.T) {
	skipDialectRerun(t)
	enableDirectPathForTest(t)
	paths := newSharedBackendPaths(t)
	dialed, err := gtransport.DialPool(context.Background(), append(paths.opts, option.WithGRPCConnectionPool(2))...)
	if err != nil {
		t.Fatal(err)
	}
	userPool := &closeCountingConnPool{ConnPool: dialed}
	client := newDirectPathFallbackTestClient(t, ClientConfig{}, gtransport.WithConnPool(userPool))
	pool := staticPoolOf(t, client)
	if pool.fallback == nil {
		t.Fatal("fallback not enabled")
	}

	forceDirectPathFallback(t, pool.fallback)
	queryFooFromBar(t, client)
	// The CloudPath dial returns the caller's pool again; the client keeps
	// using the caller's connections and serves nothing over the CloudPath
	// front door.
	if got := paths.cloudDials.Load(); got == 0 {
		t.Fatal("no CloudPath dial after the switch")
	}
	if got := len(paths.cloud.recorded()); got != 0 {
		t.Fatalf("CloudPath front door RPCs = %d, want 0", got)
	}
	client.Close()
	if got := userPool.closes.Load(); got != 1 {
		t.Fatalf("caller-supplied pool closes = %d, want 1", got)
	}
}

// fastSessionRetry retries CreateSession on UNAVAILABLE within milliseconds
// instead of the default backoff, so that a test waiting for the client to
// fall back does not wait out a backoff.
func fastSessionRetry() *vkit.CallOptions {
	return &vkit.CallOptions{CreateSession: []gax.CallOption{gax.WithRetry(func() gax.Retryer {
		return gax.OnCodes([]codes.Code{codes.Unavailable}, gax.Backoff{Initial: time.Millisecond, Max: 10 * time.Millisecond, Multiplier: 2})
	})}}
}

// unreachableAddr returns a local address nothing listens on.
func unreachableAddr(t *testing.T) string {
	t.Helper()
	lis, err := net.Listen("tcp", "localhost:0")
	if err != nil {
		t.Fatal(err)
	}
	addr := lis.Addr().String()
	lis.Close()
	return addr
}

// redirectCloudPathTo points the CloudPath dials of clients created after it
// at addr, with the clients' own dial options, and reports when the first dial
// starts.
func redirectCloudPathTo(t *testing.T, addr string) <-chan struct{} {
	t.Helper()
	old := dialDirectPathFallbackCloudPath
	started := make(chan struct{})
	var once sync.Once
	dialDirectPathFallbackCloudPath = func(ctx context.Context, opts ...option.ClientOption) (gtransport.ConnPool, error) {
		once.Do(func() { close(started) })
		return gtransport.DialPool(ctx, append(opts, option.WithEndpoint(addr))...)
	}
	t.Cleanup(func() { dialDirectPathFallbackCloudPath = old })
	return started
}

// queryHonorsDeadlineDuringCloudPathDial runs a query with a short deadline
// whose slot must first dial an unreachable CloudPath endpoint.
func queryHonorsDeadlineDuringCloudPathDial(t *testing.T, client *Client, started <-chan struct{}) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	done := make(chan error, 1)
	go func() {
		iter := client.Single().Query(ctx, NewStatement(SelectFooFromBar))
		defer iter.Stop()
		_, err := iter.Next()
		done <- err
	}()
	select {
	case <-started:
	case <-time.After(5 * time.Second):
		t.Fatal("CloudPath dial never started")
	}
	select {
	case err := <-done:
		if ErrCode(err) != codes.DeadlineExceeded {
			t.Fatalf("query error = %v, want DEADLINE_EXCEEDED", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatalf("query still blocked 2s into a 50ms deadline while CloudPath was dialing")
	}
}

func TestStaticPoolBlockingCloudPathDialHonorsQueryDeadline(t *testing.T) {
	skipDialectRerun(t)
	enableDirectPathForTest(t)
	paths := newSharedBackendPaths(t)
	// grpc.WithBlock makes every dial, including CloudPath's, wait until the
	// connection is ready.
	opts := append(append([]option.ClientOption{}, paths.opts...), option.WithGRPCDialOption(grpc.WithBlock()))
	started := redirectCloudPathTo(t, unreachableAddr(t))
	client := newDirectPathFallbackTestClient(t, ClientConfig{NumChannels: 1}, opts...)
	forceDirectPathFallback(t, staticPoolOf(t, client).fallback)
	queryHonorsDeadlineDuringCloudPathDial(t, client, started)
}

func TestStaticPoolNonBlockingCloudPathDialHonorsQueryDeadline(t *testing.T) {
	skipDialectRerun(t)
	enableDirectPathForTest(t)
	paths := newSharedBackendPaths(t)
	started := redirectCloudPathTo(t, unreachableAddr(t))
	client := newDirectPathFallbackTestClient(t, ClientConfig{NumChannels: 1}, paths.opts...)
	forceDirectPathFallback(t, staticPoolOf(t, client).fallback)
	queryHonorsDeadlineDuringCloudPathDial(t, client, started)
}

// assertCloudPathUnowned checks that a slot whose CloudPath dial returned the
// caller's connection again keeps using that connection without owning it.
func assertCloudPathUnowned(t *testing.T, slot *channelSlot, conn *grpc.ClientConn) {
	t.Helper()
	cloud := slot.cloud.Load()
	if cloud == nil {
		t.Fatal("slot has no CloudPath side after the switch")
	}
	if _, ok := cloud.ConnPool.(unownedPool); !ok {
		t.Fatalf("CloudPath side = %T, want unownedPool over the caller's connection", cloud.ConnPool)
	}
	if got := cloud.Conn(); got != conn {
		t.Fatal("CloudPath side does not use the caller's connection")
	}
}

func TestStaticPoolDirectPathFallbackKeepsCallerSuppliedConnectionUnowned(t *testing.T) {
	skipDialectRerun(t)
	enableDirectPathForTest(t)
	paths := newSharedBackendPaths(t)
	conn, err := gtransport.Dial(context.Background(), paths.opts...)
	if err != nil {
		t.Fatal(err)
	}
	client := newDirectPathFallbackTestClient(t, ClientConfig{}, option.WithGRPCConn(conn))
	pool := staticPoolOf(t, client)
	forceDirectPathFallback(t, pool.fallback)
	queryFooFromBar(t, client)
	assertCloudPathUnowned(t, pool.slots[0], conn)
}

// directPathOptionTypes names the internaloption types allClientOpts adds for
// DirectPath.
var directPathOptionTypes = []string{
	"internaloption.enableDirectPath",
	"internaloption.enableDirectPathXds",
	"internaloption.allowHardBoundTokens",
	"internaloption.allowNonDefaultServiceAccount",
}

func countDirectPathOptions(opts []option.ClientOption) int {
	n := 0
	for _, o := range opts {
		name := fmt.Sprintf("%T", o)
		for _, dp := range directPathOptionTypes {
			if name == dp {
				n++
			}
		}
	}
	return n
}

// TestCloudPathClientOptsHaveNoDirectPathOptions checks that the CloudPath
// fallback dial carries none of the DirectPath options, even with DirectPath
// turned on through the environment. EnableDirectPathXds there would disable
// S2A on CloudPath and log a DirectPath misconfiguration warning.
func TestCloudPathClientOptsHaveNoDirectPathOptions(t *testing.T) {
	skipDialectRerun(t)
	t.Setenv("GOOGLE_SPANNER_ENABLE_DIRECT_ACCESS", "true")
	if got := countDirectPathOptions(allClientOpts(1, "", directAccessEnabled(false))); got != len(directPathOptionTypes) {
		t.Fatalf("DirectPath options for a DirectPath dial = %d, want %d", got, len(directPathOptionTypes))
	}
	if got := countDirectPathOptions(cloudPathClientOpts("")); got != 0 {
		t.Fatalf("DirectPath options for the CloudPath dial = %d, want 0", got)
	}
}

func TestDirectAccessEnabled(t *testing.T) {
	skipDialectRerun(t)
	for _, tt := range []struct {
		env        string
		configured bool
		want       bool
	}{
		{env: "", configured: false, want: false},
		{env: "", configured: true, want: true},
		{env: "true", configured: false, want: true},
		{env: "false", configured: true, want: false},
		{env: "invalid", configured: true, want: false},
	} {
		t.Setenv("GOOGLE_SPANNER_ENABLE_DIRECT_ACCESS", tt.env)
		if got := directAccessEnabled(tt.configured); got != tt.want {
			t.Errorf("directAccessEnabled(%v) with GOOGLE_SPANNER_ENABLE_DIRECT_ACCESS=%q = %v, want %v", tt.configured, tt.env, got, tt.want)
		}
	}
}

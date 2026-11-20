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
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	vkit "cloud.google.com/go/spanner/apiv1"
	"cloud.google.com/go/spanner/apiv1/spannerpb"
	. "cloud.google.com/go/spanner/internal/testutil"
	"cloud.google.com/go/spanner/internal/transport"
	"github.com/googleapis/gax-go/v2"
	"google.golang.org/api/iterator"
	"google.golang.org/api/option"
	gtransport "google.golang.org/api/transport/grpc"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/stats"
	"google.golang.org/grpc/testdata"
)

// frontDoor is one network path to a shared in-memory Spanner backend. It
// records the RPCs and connections it serves.
type frontDoor struct {
	addr  string
	conns atomic.Int64

	mu          sync.Mutex
	rpcs        []frontDoorRPC
	hold        func(method string) error                // runs before a stream; an error ends the stream
	trailerHold func(ctx context.Context, method string) // runs before a stream's trailers
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

func (d *frontDoor) setTrailerHold(hold func(ctx context.Context, method string)) {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.trailerHold = hold
}

func (d *frontDoor) streamTrailerHold() func(ctx context.Context, method string) {
	d.mu.Lock()
	defer d.mu.Unlock()
	return d.trailerHold
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
			err := handler(srv, ss)
			if hold := d.streamTrailerHold(); hold != nil {
				hold(ss.Context(), info.FullMethod)
			}
			return err
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
// and a CloudPath front door, so both see the same sessions and transactions.
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
	isolateChannelPoolEnv(t)
	t.Setenv(directAccessEnv, "true")
	t.Setenv(gcpFallbackEnv, "true")
}

func newDirectPathFallbackTestClient(t *testing.T, config ClientConfig, opts ...option.ClientOption) *Client {
	t.Helper()
	config.DisableNativeMetrics = true
	if config.Logger == nil {
		config.Logger = log.New(io.Discard, "", 0)
	}
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

func TestFixedPoolDirectPathFallbackDefaults(t *testing.T) {
	skipDialectRerun(t)
	tests := []struct {
		name         string
		directAccess string
		fallback     string
		want         bool
	}{
		{name: "fallback is off by default", directAccess: "true", fallback: "", want: false},
		{name: "fallback enabled by environment", directAccess: "true", fallback: "true", want: true},
		{name: "fallback disabled by environment", directAccess: "true", fallback: "false", want: false},
		{name: "no fallback without DirectPath", directAccess: "false", fallback: "true", want: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			isolateChannelPoolEnv(t)
			t.Setenv(directAccessEnv, tt.directAccess)
			t.Setenv(gcpFallbackEnv, tt.fallback)
			paths := newSharedBackendPaths(t)
			client := newDirectPathFallbackTestClient(t, ClientConfig{NumChannels: 4}, paths.opts...)
			pool := fixedPoolOf(t, client)
			if got := poolFallback(pool) != nil; got != tt.want {
				t.Fatalf("fallback enabled = %v, want %v", got, tt.want)
			}
			if got, want := pool.Num(), 4; got != want {
				t.Fatalf("slot count = %d, want %d", got, want)
			}
		})
	}
}

func TestFixedPoolDirectPathFallbackDialsCloudPathLazily(t *testing.T) {
	skipDialectRerun(t)
	enableDirectPathForTest(t)
	paths := newSharedBackendPaths(t)
	client := newDirectPathFallbackTestClient(t, ClientConfig{NumChannels: 4}, paths.opts...)
	pool := fixedPoolOf(t, client)

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

	forceDirectPathFallback(t, poolFallback(pool))
	queryFooFromBar(t, client)
	if got, want := paths.cloud.count(executeStreamingSQLMethod), 1; got != want {
		t.Fatalf("CloudPath queries = %d, want %d", got, want)
	}
	if got, want := paths.cloudDials.Load(), int32(1); got != want {
		t.Fatalf("CloudPath dials = %d, want %d", got, want)
	}
}

func TestFixedPoolDirectPathFallbackKeepsTransactionOnSlot(t *testing.T) {
	skipDialectRerun(t)
	enableDirectPathForTest(t)
	paths := newSharedBackendPaths(t)
	client := newDirectPathFallbackTestClient(t, ClientConfig{NumChannels: 4}, paths.opts...)
	pool := fixedPoolOf(t, client)

	directBefore := len(paths.direct.recorded())
	_, err := client.ReadWriteTransaction(context.Background(), func(ctx context.Context, tx *ReadWriteTransaction) error {
		if _, err := tx.Update(ctx, NewStatement(UpdateBarSetFoo)); err != nil {
			return err
		}
		if !poolFallback(pool).current().fallback {
			forceDirectPathFallback(t, poolFallback(pool))
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

func TestFixedPoolDirectPathFallbackInFlightStreamStaysOnDirectPath(t *testing.T) {
	skipDialectRerun(t)
	enableDirectPathForTest(t)
	paths := newSharedBackendPaths(t)
	client := newDirectPathFallbackTestClient(t, ClientConfig{NumChannels: 4}, paths.opts...)
	pool := fixedPoolOf(t, client)
	endStream := make(chan struct{})
	paths.direct.setTrailerHold(func(ctx context.Context, method string) {
		if method != executeStreamingSQLMethod {
			return
		}
		select {
		case <-endStream:
		case <-ctx.Done():
		}
	})

	iter := client.Single().Query(context.Background(), NewStatement(SelectFooFromBar))
	defer iter.Stop()
	if _, err := iter.Next(); err != nil {
		t.Fatalf("first row failed: %v", err)
	}
	forceDirectPathFallback(t, poolFallback(pool))
	close(endStream)
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

func TestFixedPoolDirectPathUnreachableFallsBackToCloudPath(t *testing.T) {
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
	pool := fixedPoolOf(t, client)
	if !poolFallback(pool).current().fallback {
		t.Fatal("pool did not fall back to CloudPath")
	}
	if got := paths.cloud.count(createSessionMethod); got == 0 {
		t.Fatal("the multiplexed session was not created over CloudPath")
	}
	if got, want := paths.cloud.count(executeStreamingSQLMethod), 1; got != want {
		t.Fatalf("CloudPath queries = %d, want %d", got, want)
	}
}

// A closed slot that never dialed CloudPath must fail with CANCELED, which
// callers do not retry.
func TestFixedPoolDirectPathFallbackAfterClientClose(t *testing.T) {
	skipDialectRerun(t)
	enableDirectPathForTest(t)
	paths := newSharedBackendPaths(t)
	client := newDirectPathFallbackTestClient(t, ClientConfig{NumChannels: 1}, paths.opts...)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	ro := client.ReadOnlyTransaction()
	defer ro.Close()
	if err := ro.Query(ctx, NewStatement(SelectFooFromBar)).Do(func(*Row) error { return nil }); err != nil {
		t.Fatal(err)
	}
	forceDirectPathFallback(t, poolFallback(fixedPoolOf(t, client)))
	client.Close()
	if err := ro.Query(ctx, NewStatement(SelectFooFromBar)).Do(func(*Row) error { return nil }); ErrCode(err) != codes.Canceled {
		t.Fatalf("query after Client.Close error = %v, want CANCELED", err)
	}
	if got := paths.cloudDials.Load(); got != 0 {
		t.Fatalf("CloudPath dials after Client.Close = %d, want 0", got)
	}
}

func TestClientCloseStopsDirectPathFallback(t *testing.T) {
	skipDialectRerun(t)
	for _, dynamic := range []bool{false, true} {
		t.Run(fmt.Sprintf("dynamic=%v", dynamic), func(t *testing.T) {
			enableDirectPathForTest(t)
			paths := newSharedBackendPaths(t)
			config := ClientConfig{NumChannels: 1}
			if dynamic {
				config.DynamicChannelPoolConfig = dcpFallbackTestConfig()
			}
			client := newDirectPathFallbackTestClient(t, config, paths.opts...)
			f := poolFallback(client.sc.dynamicPool)
			if f == nil {
				t.Fatal("the pool has no fallback")
			}
			client.Close()
			select {
			case <-f.done:
			default:
				t.Fatal("Client.Close did not stop the fallback window loop")
			}
			if f.ctx.Err() == nil {
				t.Fatal("Client.Close did not cancel the fallback context")
			}
		})
	}
}

func TestDirectPathFallbackWorksWhenItsCountersCannotBeCreated(t *testing.T) {
	skipDialectRerun(t)
	for _, dynamic := range []bool{false, true} {
		t.Run(fmt.Sprintf("dynamic=%v", dynamic), func(t *testing.T) {
			enableDirectPathForTest(t)
			paths := newSharedBackendPaths(t)
			logger := &lockedBuffer{}
			config := ClientConfig{NumChannels: 1, Logger: log.New(logger, "", 0)}
			factory := &builtinMetricsTracerFactory{
				meterProvider: &failingDCPMeterProvider{meter: &failingDCPMeter{failCounterName: metricNameEEFCallStatus}},
			}
			ctx := context.Background()
			database := "projects/p/instances/i/databases/d"
			md := metadata.Pairs(resourcePrefixHeader, database)
			var pool gtransport.ConnPool
			var err error
			if dynamic {
				config.DynamicChannelPoolConfig = dcpOneEntryFallbackTestConfig()
				pool, _, err = createDCPConnPool(ctx, database, config, nil, md, factory, true, paths.opts...)
			} else {
				pool, _, _, err = createFixedConnPool(ctx, database, config, true, nil, md, factory, true, paths.opts...)
			}
			if err != nil {
				t.Fatalf("creating the channel pool failed: %v", err)
			}
			defer pool.Close()
			if got, want := logger.String(), "test counter registration failure"; !strings.Contains(got, want) {
				t.Fatalf("log = %q, want it to contain %q", got, want)
			}
			f := poolFallback(pool.(*dynamicChannelPool))
			if f == nil {
				t.Fatal("the pool has no fallback")
			}
			forceDirectPathFallback(t, f)
			if _, err := spannerpb.NewSpannerClient(pool).CreateSession(ctx, &spannerpb.CreateSessionRequest{Database: database}); err != nil {
				t.Fatalf("CreateSession() after the switch failed: %v", err)
			}
			if got, want := paths.cloud.count(createSessionMethod), 1; got != want {
				t.Fatalf("CloudPath CreateSession calls = %d, want %d", got, want)
			}
		})
	}
}

func TestChannelPoolsWithFallbackAndNoMetricsTracerFactory(t *testing.T) {
	skipDialectRerun(t)
	for _, dynamic := range []bool{false, true} {
		t.Run(fmt.Sprintf("dynamic=%v", dynamic), func(t *testing.T) {
			enableDirectPathForTest(t)
			paths := newSharedBackendPaths(t)
			config := ClientConfig{NumChannels: 1}
			ctx := context.Background()
			database := "projects/p/instances/i/databases/d"
			md := metadata.Pairs(resourcePrefixHeader, database)
			var pool gtransport.ConnPool
			var err error
			if dynamic {
				config.DynamicChannelPoolConfig = dcpOneEntryFallbackTestConfig()
				pool, _, err = createDCPConnPool(ctx, database, config, nil, md, nil, true, paths.opts...)
			} else {
				pool, _, _, err = createFixedConnPool(ctx, database, config, true, nil, md, nil, true, paths.opts...)
			}
			if err != nil {
				t.Fatalf("creating the channel pool failed: %v", err)
			}
			defer pool.Close()
			if poolFallback(pool.(*dynamicChannelPool)) == nil {
				t.Fatal("the pool has no fallback")
			}
		})
	}
}

// embeddedOption exercises option resolution through another option type.
type embeddedOption struct{ option.ClientOption }

func TestFixedPoolDirectPathFallbackWithCallerSuppliedPool(t *testing.T) {
	skipDialectRerun(t)
	for _, embedded := range []bool{false, true} {
		t.Run(fmt.Sprintf("embedded=%v", embedded), func(t *testing.T) {
			enableDirectPathForTest(t)
			paths := newSharedBackendPaths(t)
			var mu sync.Mutex
			var used []*grpc.ClientConn
			recordConn := func(ctx context.Context, method string, req, reply any, cc *grpc.ClientConn, invoker grpc.UnaryInvoker, opts ...grpc.CallOption) error {
				if method == executeSQLMethod || method == commitMethod {
					mu.Lock()
					used = append(used, cc)
					mu.Unlock()
				}
				return invoker(ctx, method, req, reply, cc, opts...)
			}
			dialed, err := gtransport.DialPool(context.Background(), append(paths.opts, option.WithGRPCConnectionPool(3), option.WithGRPCDialOption(grpc.WithUnaryInterceptor(recordConn)))...)
			if err != nil {
				t.Fatal(err)
			}
			userPool := &closeCountingConnPool{ConnPool: dialed}
			opt := gtransport.WithConnPool(userPool)
			if embedded {
				opt = embeddedOption{opt}
			}
			client := newDirectPathFallbackTestClient(t, ClientConfig{}, opt)
			pool := fixedPoolOf(t, client)
			if poolFallback(pool) == nil {
				t.Fatal("fallback not enabled")
			}
			connsPerTransaction := func() int {
				mu.Lock()
				used = nil
				mu.Unlock()
				if _, err := client.ReadWriteTransaction(context.Background(), func(ctx context.Context, tx *ReadWriteTransaction) error {
					for i := 0; i < 2; i++ {
						if _, err := tx.Update(ctx, NewStatement(UpdateBarSetFoo)); err != nil {
							return err
						}
					}
					return nil
				}); err != nil {
					t.Fatalf("ReadWriteTransaction() failed: %v", err)
				}
				mu.Lock()
				defer mu.Unlock()
				seen := map[*grpc.ClientConn]bool{}
				for _, cc := range used {
					seen[cc] = true
				}
				return len(seen)
			}

			if got := connsPerTransaction(); got != 1 {
				t.Fatalf("connections used by a transaction before the switch = %d, want 1", got)
			}
			forceDirectPathFallback(t, poolFallback(pool))
			if got := connsPerTransaction(); got != 1 {
				t.Fatalf("connections used by a transaction after the switch = %d, want 1", got)
			}
			for i := 0; i < 3; i++ {
				queryFooFromBar(t, client)
			}
			if got := paths.cloud.conns.Load(); got != 0 {
				t.Fatalf("CloudPath connections with a caller-supplied pool = %d, want 0", got)
			}
			if got := len(paths.cloud.recorded()); got != 0 {
				t.Fatalf("CloudPath front door RPCs = %d, want 0", got)
			}
			client.Close()
			if got := userPool.closes.Load(); got != 1 {
				t.Fatalf("caller-supplied pool closes = %d, want 1", got)
			}
		})
	}
}

// fastSessionRetry keeps a test that waits for the client to fall back from
// waiting out the default CreateSession backoff.
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

func TestFixedPoolBlockingCloudPathDialHonorsQueryDeadline(t *testing.T) {
	skipDialectRerun(t)
	enableDirectPathForTest(t)
	paths := newSharedBackendPaths(t)
	opts := append(append([]option.ClientOption{}, paths.opts...), option.WithGRPCDialOption(grpc.WithBlock()))
	started := redirectCloudPathTo(t, unreachableAddr(t))
	client := newDirectPathFallbackTestClient(t, ClientConfig{NumChannels: 1}, opts...)
	forceDirectPathFallback(t, poolFallback(fixedPoolOf(t, client)))
	queryHonorsDeadlineDuringCloudPathDial(t, client, started)
}

func TestFixedPoolNonBlockingCloudPathDialHonorsQueryDeadline(t *testing.T) {
	skipDialectRerun(t)
	enableDirectPathForTest(t)
	paths := newSharedBackendPaths(t)
	started := redirectCloudPathTo(t, unreachableAddr(t))
	client := newDirectPathFallbackTestClient(t, ClientConfig{NumChannels: 1}, paths.opts...)
	forceDirectPathFallback(t, poolFallback(fixedPoolOf(t, client)))
	queryHonorsDeadlineDuringCloudPathDial(t, client, started)
}

func TestDirectPathFallbackOffWithCallerSuppliedConnection(t *testing.T) {
	skipDialectRerun(t)
	for _, dynamic := range []bool{false, true} {
		for _, embedded := range []bool{false, true} {
			t.Run(fmt.Sprintf("dynamic=%v/embedded=%v", dynamic, embedded), func(t *testing.T) {
				enableDirectPathForTest(t)
				paths := newSharedBackendPaths(t)
				conn, err := gtransport.Dial(context.Background(), paths.opts...)
				if err != nil {
					t.Fatal(err)
				}
				t.Cleanup(func() { conn.Close() })
				opt := option.WithGRPCConn(conn)
				if embedded {
					opt = embeddedOption{opt}
				}
				config := ClientConfig{}
				if dynamic {
					config.DynamicChannelPoolConfig = dcpOneEntryFallbackTestConfig()
				}
				client := newDirectPathFallbackTestClient(t, config, opt)
				p := client.sc.dynamicPool
				if poolFallback(p) != nil {
					t.Fatal("the client has fallback state")
				}
				for i, e := range p.GetEntries() {
					if slot := e.Pool.(*channelSlot); slot.fallback != nil {
						t.Fatalf("entry %d has fallback state", i)
					}
				}
				queryFooFromBar(t, client)
				if got := paths.cloudDials.Load(); got != 0 {
					t.Fatalf("CloudPath dials with a caller-supplied connection = %d, want 0", got)
				}
			})
		}
	}
}

func TestDirectPathFallbackOffWithEmulator(t *testing.T) {
	skipDialectRerun(t)
	enableDirectPathForTest(t)
	server, _, teardown := NewMockedSpannerInMemTestServer(t)
	defer teardown()
	t.Setenv("SPANNER_EMULATOR_HOST", server.ServerAddress)
	client, err := NewClientWithConfig(context.Background(), "projects/p/instances/i/databases/d", ClientConfig{DisableNativeMetrics: true})
	if err != nil {
		t.Fatal(err)
	}
	defer client.Close()
	if poolFallback(fixedPoolOf(t, client)) != nil {
		t.Fatal("the emulator client has fallback state")
	}
	queryFooFromBar(t, client)
}

// A nil connection or pool option clears an earlier one, so the client dials
// its own connections and can fall back.
func TestDirectPathFallbackWithClearedConnectionOption(t *testing.T) {
	skipDialectRerun(t)
	for _, dynamic := range []bool{false, true} {
		for _, kind := range []string{"WithGRPCConn", "WithConnPool"} {
			t.Run(fmt.Sprintf("dynamic=%v/%s", dynamic, kind), func(t *testing.T) {
				enableDirectPathForTest(t)
				paths := newSharedBackendPaths(t)
				cleared, err := gtransport.DialPool(context.Background(), paths.opts...)
				if err != nil {
					t.Fatal(err)
				}
				defer cleared.Close()
				opts := append([]option.ClientOption{}, paths.opts...)
				if kind == "WithGRPCConn" {
					opts = append(opts, option.WithGRPCConn(cleared.Conn()), option.WithGRPCConn(nil))
				} else {
					opts = append(opts, gtransport.WithConnPool(cleared), gtransport.WithConnPool(nil))
				}
				config := ClientConfig{NumChannels: 1}
				if dynamic {
					config.DynamicChannelPoolConfig = dcpOneEntryFallbackTestConfig()
				}
				client := newDirectPathFallbackTestClient(t, config, opts...)
				p := client.sc.dynamicPool
				if p.Conn() == cleared.Conn() {
					t.Fatal("the client uses the cleared connection")
				}
				forceDirectPathFallback(t, poolFallback(p))
				queryFooFromBar(t, client)
				if got, want := paths.cloud.count(executeStreamingSQLMethod), 1; got != want {
					t.Fatalf("CloudPath queries = %d, want %d", got, want)
				}
			})
		}
	}
}

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

// EnableDirectPathXds on a CloudPath dial would disable S2A and log a
// DirectPath misconfiguration warning.
func TestCloudPathDialHasNoDirectPathOptions(t *testing.T) {
	skipDialectRerun(t)
	t.Setenv(directAccessEnv, "true")
	t.Setenv("GOOGLE_SPANNER_DISABLE_DIRECT_ACCESS_BOUND_TOKEN", "")
	cfg := transport.PoolConfig{NumChannels: 1}
	if got := countDirectPathOptions(cfg.ClientOptions()); got != len(directPathOptionTypes) {
		t.Fatalf("DirectPath options for a DirectPath dial = %d, want %d", got, len(directPathOptionTypes))
	}
	var cloudOpts []option.ClientOption
	old := dialDirectPathFallbackCloudPath
	dialDirectPathFallbackCloudPath = func(_ context.Context, opts ...option.ClientOption) (gtransport.ConnPool, error) {
		cloudOpts = opts
		return nil, errors.New("not dialed")
	}
	t.Cleanup(func() { dialDirectPathFallbackCloudPath = old })
	newCloudPathDialer(cfg, nil, nil)(context.Background())
	if len(cloudOpts) == 0 {
		t.Fatal("the CloudPath dial got no options")
	}
	if got := countDirectPathOptions(cloudOpts); got != 0 {
		t.Fatalf("DirectPath options for the CloudPath dial = %d, want 0", got)
	}
}

func TestConcurrentCloudPathDialsShareNoSpareOptionCapacity(t *testing.T) {
	skipDialectRerun(t)
	spare := -1
	old := dialDirectPathFallbackCloudPath
	dialDirectPathFallbackCloudPath = func(_ context.Context, opts ...option.ClientOption) (gtransport.ConnPool, error) {
		spare = cap(opts) - len(opts)
		return nil, errors.New("not dialed")
	}
	t.Cleanup(func() { dialDirectPathFallbackCloudPath = old })
	newCloudPathDialer(transport.PoolConfig{NumChannels: 1}, nil, nil)(context.Background())
	if spare != 0 {
		t.Fatalf("spare capacity of the options every CloudPath dial gets = %d, want 0", spare)
	}
}

// forceDirectPathFallbackUnderLoad is forceDirectPathFallback for a pool that
// is serving requests: a success that lands in the forced window keeps it
// from switching, so it retries until a window holds only the failure.
func forceDirectPathFallbackUnderLoad(t *testing.T, f *directPathFallback) {
	t.Helper()
	for i := 0; i < 10000 && !f.current().fallback; i++ {
		f.closeWindow(time.Now())
		f.record(f.current().generation, true)
		f.closeWindow(time.Now())
	}
	if !f.current().fallback {
		t.Fatal("pool did not fall back")
	}
}

func TestCloudPathOfCallerSuppliedConnections(t *testing.T) {
	skipDialectRerun(t)
	_, opts, teardown := NewMockedSpannerInMemTestServer(t)
	defer teardown()
	conn, err := gtransport.Dial(context.Background(), opts...)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	pool, err := gtransport.DialPool(context.Background(), opts...)
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()
	with := func(extra ...option.ClientOption) []option.ClientOption {
		return append(append([]option.ClientOption{}, opts...), extra...)
	}
	const (
		ownCloudPath = "own CloudPath"
		noFallback   = "no fallback"
		noCloudPath  = "errNoCloudPath"
	)
	for _, tt := range []struct {
		name string
		opts []option.ClientOption
		want string
	}{
		{name: "no connection option", opts: opts, want: ownCloudPath},
		{name: "pool size only", opts: with(option.WithGRPCConnectionPool(4)), want: ownCloudPath},
		{name: "WithGRPCConn", opts: with(option.WithGRPCConn(conn)), want: noFallback},
		{name: "embedded WithGRPCConn", opts: with(embeddedOption{option.WithGRPCConn(conn)}), want: noFallback},
		{name: "WithGRPCConn cleared by nil", opts: with(option.WithGRPCConn(conn), option.WithGRPCConn(nil)), want: ownCloudPath},
		{name: "nil WithGRPCConn", opts: with(option.WithGRPCConn(nil)), want: ownCloudPath},
		{name: "WithConnPool", opts: with(gtransport.WithConnPool(pool)), want: noCloudPath},
		{name: "embedded WithConnPool", opts: with(embeddedOption{gtransport.WithConnPool(pool)}), want: noCloudPath},
		{name: "WithConnPool cleared by nil", opts: with(gtransport.WithConnPool(pool), gtransport.WithConnPool(nil)), want: ownCloudPath},
		{name: "nil WithConnPool", opts: with(gtransport.WithConnPool(nil)), want: ownCloudPath},
	} {
		t.Run(tt.name, func(t *testing.T) {
			direct, err := gtransport.DialPool(context.Background(), tt.opts...)
			if err != nil {
				t.Fatal(err)
			}
			if !sameConnPool(direct, pool) && direct.Conn() != conn {
				defer direct.Close()
			}
			got := noFallback
			if !suppliesGRPCConn(tt.opts) {
				dial := newCloudPathDialer(transport.PoolConfig{NumChannels: 1}, tt.opts, func(c gtransport.ConnPool) bool { return sameConnPool(c, direct) })
				cloud, err := dial(context.Background())
				switch {
				case errors.Is(err, errNoCloudPath):
					got = noCloudPath
				case err != nil:
					t.Fatalf("CloudPath dial failed: %v", err)
				default:
					got = ownCloudPath
					if sameConnPool(cloud, pool) || cloud.Conn() == conn {
						t.Fatal("the CloudPath dial returned the caller's connection")
					}
					cloud.Close()
				}
			}
			if got != tt.want {
				t.Fatalf("CloudPath dial = %s, want %s", got, tt.want)
			}
		})
	}
}

type nonComparableConnPool struct {
	gtransport.ConnPool
	_ [0]func()
}

func TestSameConnPool(t *testing.T) {
	skipDialectRerun(t)
	a, b := &closeCountingConnPool{}, &closeCountingConnPool{}
	for _, tt := range []struct {
		name string
		x, y gtransport.ConnPool
		want bool
	}{
		{name: "same pool", x: a, y: a, want: true},
		{name: "different pools", x: a, y: b, want: false},
		{name: "different types", x: a, y: borrowedPool{a}, want: false},
		{name: "nil", x: nil, y: a, want: false},
		{name: "values that cannot be compared", x: nonComparableConnPool{}, y: nonComparableConnPool{}, want: true},
	} {
		if got := sameConnPool(tt.x, tt.y); got != tt.want {
			t.Errorf("%s: sameConnPool() = %v, want %v", tt.name, got, tt.want)
		}
	}
}

func TestCallerSuppliedPoolUnderConcurrentUseAndSwitch(t *testing.T) {
	skipDialectRerun(t)
	for _, dynamic := range []bool{false, true} {
		t.Run(fmt.Sprintf("dynamic=%v", dynamic), func(t *testing.T) {
			enableDirectPathForTest(t)
			paths := newSharedBackendPaths(t)
			dialed, err := gtransport.DialPool(context.Background(), append(paths.opts, option.WithGRPCConnectionPool(3))...)
			if err != nil {
				t.Fatal(err)
			}
			userPool := &closeCountingConnPool{ConnPool: dialed}
			config := ClientConfig{}
			if dynamic {
				// One entry: the dynamic pool closes a caller-supplied pool
				// once per entry.
				config.DynamicChannelPoolConfig = dcpOneEntryFallbackTestConfig()
			}
			client := newDirectPathFallbackTestClient(t, config, gtransport.WithConnPool(userPool))
			var fallback *directPathFallback
			if dynamic {
				fallback = poolFallback(dynamicPoolOf(t, client))
			} else {
				pool := fixedPoolOf(t, client)
				if got, want := pool.Num(), 3; got != want {
					t.Fatalf("slot count = %d, want %d", got, want)
				}
				fallback = poolFallback(pool)
			}

			stop := make(chan struct{})
			var callerUse sync.WaitGroup
			callerUse.Add(1)
			go func() {
				// The caller uses its pool too, advancing its round robin.
				defer callerUse.Done()
				for {
					select {
					case <-stop:
						return
					default:
						userPool.Conn()
					}
				}
			}()
			var queries sync.WaitGroup
			const workers = 8
			errs := make(chan error, workers)
			for g := 0; g < workers; g++ {
				queries.Add(1)
				go func() {
					defer queries.Done()
					for i := 0; i < 10; i++ {
						iter := client.Single().Query(context.Background(), NewStatement(SelectFooFromBar))
						if err := iter.Do(func(*Row) error { return nil }); err != nil {
							errs <- err
							return
						}
					}
				}()
			}
			forceDirectPathFallbackUnderLoad(t, fallback)
			queries.Wait()
			close(stop)
			callerUse.Wait()
			close(errs)
			for err := range errs {
				t.Fatalf("query failed: %v", err)
			}
			if got := userPool.closes.Load(); got != 0 {
				t.Fatalf("caller-supplied pool closed %d times while the client was in use, want 0", got)
			}
			if got := paths.cloud.conns.Load(); got != 0 {
				t.Fatalf("CloudPath connections = %d, want 0", got)
			}
			if got := len(paths.cloud.recorded()); got != 0 {
				t.Fatalf("CloudPath front door RPCs = %d, want 0", got)
			}
			client.Close()
			if got := userPool.closes.Load(); got != 1 {
				t.Fatalf("caller-supplied pool closes after Client.Close = %d, want 1", got)
			}
		})
	}
}

// TestDirectPathFallbackCountsReadsTheReaderStopped checks reads that end
// before their stream does. A stream that delivered data before its reader
// stopped it is a success; a read the caller cancelled is not counted.
func TestDirectPathFallbackCountsReadsTheReaderStopped(t *testing.T) {
	skipDialectRerun(t)
	readColumns := []string{"SingerId", "AlbumId", "AlbumTitle"}
	for _, dynamic := range []bool{false, true} {
		t.Run(fmt.Sprintf("dynamic=%v", dynamic), func(t *testing.T) {
			enableDirectPathForTest(t)
			paths := newSharedBackendPaths(t)
			config := ClientConfig{}
			if dynamic {
				config.DynamicChannelPoolConfig = dcpOneEntryFallbackTestConfig()
			}
			client := newDirectPathFallbackTestClient(t, config, paths.opts...)
			f := poolFallback(client.sc.dynamicPool)
			holdAfterFirstRow := func(sql string) {
				paths.backend.TestSpanner.AddPartialResultSetError(sql, PartialResultSetExecutionTime{
					ResumeToken:   EncodeResumeToken(2),
					ExecutionTime: 5 * time.Second,
				})
			}
			// holdTrailers keeps streams open after their last message until
			// the returned function is called.
			holdTrailers := func() func() {
				paths.direct.setTrailerHold(func(ctx context.Context, _ string) {
					select {
					case <-ctx.Done():
					case <-time.After(5 * time.Second):
					}
				})
				return func() { paths.direct.setTrailerHold(nil) }
			}
			readRowBeforeTrailers := func(t *testing.T) {
				paths.backend.TestSpanner.PutStatementResult(SelectSingerIDAlbumIDAlbumTitleFromAlbums, paths.backend.CreateSingersResults(1, true))
				defer paths.backend.TestSpanner.PutStatementResult(SelectSingerIDAlbumIDAlbumTitleFromAlbums, paths.backend.CreateSingersResults(SelectSingerIDAlbumIDAlbumTitleFromAlbumsRowCount, true))
				defer holdTrailers()()
				if _, err := client.Single().ReadRow(context.Background(), "Albums", Key{"foo"}, readColumns); err != nil {
					t.Fatalf("ReadRow() failed: %v", err)
				}
			}
			waitForOutcomes := func(t *testing.T, successes uint64) {
				t.Helper()
				waitFor(t, func() error {
					if _, got := unpackFallbackOutcomes(f.current().outcomes.Load()); got < successes {
						return fmt.Errorf("DirectPath successes = %d, want %d", got, successes)
					}
					return nil
				})
				if failures, got := unpackFallbackOutcomes(f.current().outcomes.Load()); failures != 0 || got != successes {
					t.Fatalf("window = %d failures and %d successes, want 0 and %d", failures, got, successes)
				}
			}
			for _, tt := range []struct {
				name          string
				read          func(t *testing.T)
				wantSuccesses uint64
			}{
				{name: "ReadRow before the trailers of its last message", read: readRowBeforeTrailers, wantSuccesses: 1},
				{
					name: "ReadRow before its last message",
					read: func(t *testing.T) {
						holdAfterFirstRow(SelectSingerIDAlbumIDAlbumTitleFromAlbums)
						if _, err := client.Single().ReadRow(context.Background(), "Albums", Key{"foo"}, readColumns); err != nil {
							t.Fatalf("ReadRow() failed: %v", err)
						}
					},
					wantSuccesses: 1,
				},
				{
					name: "query stopped after its first row",
					read: func(t *testing.T) {
						holdAfterFirstRow(SelectFooFromBar)
						iter := client.Single().Query(context.Background(), NewStatement(SelectFooFromBar))
						defer iter.Stop()
						if _, err := iter.Next(); err != nil {
							t.Fatal(err)
						}
					},
					wantSuccesses: 1,
				},
				{
					name: "BatchWrite stopped before its trailers",
					read: func(t *testing.T) {
						defer holdTrailers()()
						iter := client.BatchWrite(context.Background(), []*MutationGroup{{[]*Mutation{Insert("t_test", []string{"key"}, []interface{}{"foo"})}}})
						defer iter.Stop()
						if _, err := iter.Next(); err != nil {
							t.Fatal(err)
						}
					},
					wantSuccesses: 1,
				},
				{
					name: "ReadRow the caller cancelled before its first row",
					read: func(t *testing.T) {
						paths.backend.TestSpanner.AddPartialResultSetError(SelectSingerIDAlbumIDAlbumTitleFromAlbums, PartialResultSetExecutionTime{
							ResumeToken:   EncodeResumeToken(1),
							ExecutionTime: 5 * time.Second,
						})
						ctx, cancel := context.WithCancel(context.Background())
						paths.direct.setStreamHold(func(string) error { cancel(); return nil })
						defer paths.direct.setStreamHold(nil)
						if _, err := client.Single().ReadRow(ctx, "Albums", Key{"foo"}, readColumns); ErrCode(err) != codes.Canceled {
							t.Fatalf("ReadRow() error = %v, want CANCELED", err)
						}
					},
				},
				{
					name: "query the caller cancelled after its first row",
					read: func(t *testing.T) {
						holdAfterFirstRow(SelectFooFromBar)
						ctx, cancel := context.WithCancel(context.Background())
						iter := client.Single().Query(ctx, NewStatement(SelectFooFromBar))
						defer iter.Stop()
						if _, err := iter.Next(); err != nil {
							t.Fatal(err)
						}
						cancel()
						if _, err := iter.Next(); ErrCode(err) != codes.Canceled {
							t.Fatalf("Next() after cancel error = %v, want CANCELED", err)
						}
					},
				},
			} {
				t.Run(tt.name, func(t *testing.T) {
					f.closeWindow(time.Now())
					tt.read(t)
					waitForOutcomes(t, tt.wantSuccesses)
				})
			}

			// One failed attempt next to a read the reader stopped is not
			// every DirectPath attempt of the window failing.
			f.closeWindow(time.Now())
			readRowBeforeTrailers(t)
			waitForOutcomes(t, 1)
			f.record(f.current().generation, true)
			if res := f.closeWindow(time.Now()); res.switched {
				t.Fatalf("window of %d failures and %d successes switched to CloudPath", res.failures, res.successes)
			}
		})
	}
}

// TestCloudPathAuthenticatesWithLegacyAuthLibrary falls back with
// authorized_user credentials on the legacy auth library, whose token source
// fetches tokens with the context the connection was dialed with.
func TestCloudPathAuthenticatesWithLegacyAuthLibrary(t *testing.T) {
	skipDialectRerun(t)
	for _, dynamic := range []bool{false, true} {
		t.Run(fmt.Sprintf("dynamic=%v", dynamic), func(t *testing.T) {
			enableDirectPathForTest(t)
			t.Setenv("GOOGLE_API_GO_EXPERIMENTAL_DISABLE_NEW_AUTH_LIB", "true")
			// On GCE the legacy library would dial DirectPath with a
			// credentials bundle that gRPC rejects next to this test's TLS
			// credentials.
			t.Setenv("GOOGLE_CLOUD_DISABLE_DIRECT_PATH", "true")
			var tokenRequests atomic.Int32
			tokens := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				tokenRequests.Add(1)
				w.Header().Set("Content-Type", "application/json")
				fmt.Fprint(w, `{"access_token":"token","token_type":"Bearer","expires_in":3600}`)
			}))
			t.Cleanup(tokens.Close)
			credentialsJSON := fmt.Sprintf(`{"type":"authorized_user","client_id":"id","client_secret":"secret","refresh_token":"refresh","token_uri":%q}`, tokens.URL)

			backend, _, teardown := NewMockedSpannerInMemTestServer(t)
			t.Cleanup(teardown)
			addSelect1Result(backend)
			serverTLS, err := credentials.NewServerTLSFromFile(testdata.Path("x509/server1_cert.pem"), testdata.Path("x509/server1_key.pem"))
			if err != nil {
				t.Fatal(err)
			}
			serveTLS := func() *frontDoor {
				d := &frontDoor{}
				server := grpc.NewServer(append(d.serverOptions(), grpc.Creds(serverTLS))...)
				spannerpb.RegisterSpannerServer(server, backend.TestSpanner)
				lis, err := net.Listen("tcp", "localhost:0")
				if err != nil {
					t.Fatal(err)
				}
				go server.Serve(lis)
				t.Cleanup(server.Stop)
				d.addr = lis.Addr().String()
				return d
			}
			direct, cloud := serveTLS(), serveTLS()
			clientTLS, err := credentials.NewClientTLSFromFile(testdata.Path("x509/server_ca_cert.pem"), "x.test.example.com")
			if err != nil {
				t.Fatal(err)
			}
			old := dialDirectPathFallbackCloudPath
			dialDirectPathFallbackCloudPath = func(ctx context.Context, opts ...option.ClientOption) (gtransport.ConnPool, error) {
				return gtransport.DialPool(ctx, append(opts, option.WithEndpoint(cloud.addr))...)
			}
			t.Cleanup(func() { dialDirectPathFallbackCloudPath = old })

			config := ClientConfig{}
			if dynamic {
				config.DynamicChannelPoolConfig = dcpFallbackTestConfig()
				config.DynamicChannelPoolConfig.DCPMaxChannels = 3
			}
			client := newDirectPathFallbackTestClient(t, config,
				option.WithEndpoint(direct.addr),
				option.WithCredentialsJSON([]byte(credentialsJSON)),
				option.WithGRPCDialOption(grpc.WithTransportCredentials(clientTLS)),
			)
			p := client.sc.dynamicPool
			forceDirectPathFallback(t, poolFallback(p))
			queryFooFromBar(t, client)
			if got := cloud.count(executeStreamingSQLMethod); got != 1 {
				t.Fatalf("CloudPath queries = %d, want 1", got)
			}
			if dynamic {
				if addPrimedDCPEntry(t, client) == nil {
					t.Fatal("scale-up after the switch added no entry")
				}
				if got := cloud.count(executeSQLMethod); got != 1 {
					t.Fatalf("CloudPath priming queries = %d, want 1", got)
				}
			}
			if tokenRequests.Load() == 0 {
				t.Fatal("the client fetched no token")
			}
		})
	}
}

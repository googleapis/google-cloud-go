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
	"bytes"
	"context"
	"fmt"
	"log"
	"log/slog"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	. "cloud.google.com/go/spanner/internal/testutil"
	"golang.org/x/sync/errgroup"
	"google.golang.org/api/option"
	gtransport "google.golang.org/api/transport/grpc"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
)

const (
	executeSQLMethod = "/google.spanner.v1.Spanner/ExecuteSql"
	commitMethod     = "/google.spanner.v1.Spanner/Commit"
)

// runReadWriteTransactionsOnChannels runs n read-write transactions of two
// statements each and returns the channel ids each transaction used, in order.
func runReadWriteTransactionsOnChannels(t *testing.T, client *Client, recorder *dcpChannelRecorder, n int) [][]uint64 {
	t.Helper()
	var txns [][]uint64
	for i := 0; i < n; i++ {
		before := len(recorder.recorded())
		_, err := client.ReadWriteTransaction(context.Background(), func(ctx context.Context, tx *ReadWriteTransaction) error {
			for j := 0; j < 2; j++ {
				if _, err := tx.Update(ctx, NewStatement(UpdateBarSetFoo)); err != nil {
					return err
				}
			}
			return nil
		})
		if err != nil {
			t.Fatalf("ReadWriteTransaction() failed: %v", err)
		}
		var ids []uint64
		for _, c := range recorder.recorded()[before:] {
			ids = append(ids, c.channelID)
		}
		txns = append(txns, ids)
	}
	return txns
}

// fixedPoolOf returns the client's channel pool and checks that it is the
// dynamic channel pool in fixed mode.
func fixedPoolOf(t *testing.T, client *Client) *dynamicChannelPool {
	t.Helper()
	p := client.sc.dynamicPool
	if p == nil || !p.fixed {
		t.Fatalf("channel pool = %T (fixed: %v), want the dynamic channel pool in fixed mode", client.sc.connPool, p != nil && p.fixed)
	}
	return p
}

// slotOf returns the channel slot of the entry with index i.
func slotOf(t *testing.T, p *dynamicChannelPool, i int) *channelSlot {
	t.Helper()
	return p.getEntries()[i].pool.(*channelSlot)
}

func TestFixedChannelPoolBindsTransactionsToEntries(t *testing.T) {
	skipDialectRerun(t)
	recorder := &dcpChannelRecorder{methods: map[string]bool{executeSQLMethod: true, commitMethod: true}}
	_, client, teardown := setupMockedTestServerWithConfigAndClientOptions(t, ClientConfig{
		DisableNativeMetrics: true,
		NumChannels:          4,
	}, []option.ClientOption{
		option.WithGRPCDialOption(grpc.WithUnaryInterceptor(recorder.unaryInterceptor)),
	})
	defer teardown()

	p := fixedPoolOf(t, client)
	if got, want := p.Num(), 4; got != want {
		t.Fatalf("entry count = %d, want %d", got, want)
	}
	for i, e := range p.getEntries() {
		if e.id != uint64(i+1) {
			t.Fatalf("entry %d id = %d, want %d", i, e.id, i+1)
		}
	}

	used := map[uint64]int{}
	for i, ids := range runReadWriteTransactionsOnChannels(t, client, recorder, 8) {
		if len(ids) != 3 {
			t.Fatalf("transaction %d recorded %d RPCs, want 3 (%v)", i, len(ids), ids)
		}
		for _, id := range ids {
			if id != ids[0] {
				t.Fatalf("transaction %d used channels %v, want one channel", i, ids)
			}
		}
		if ids[0] < 1 || ids[0] > 4 {
			t.Fatalf("transaction %d channel id = %d, want an entry id in [1, 4]", i, ids[0])
		}
		used[ids[0]]++
	}
	if len(used) != 4 {
		t.Fatalf("transactions used entries %v, want all 4 entries", used)
	}
}

func TestFixedChannelPoolIsTheDefault(t *testing.T) {
	skipDialectRerun(t)
	for _, tt := range []struct {
		name   string
		config ClientConfig
		want   int
	}{
		{name: "default channel count", config: ClientConfig{DisableNativeMetrics: true}, want: numChannels},
		{name: "NumChannels", config: ClientConfig{DisableNativeMetrics: true, NumChannels: 3}, want: 3},
	} {
		t.Run(tt.name, func(t *testing.T) {
			_, client, teardown := setupMockedTestServerWithConfig(t, tt.config)
			defer teardown()
			p := fixedPoolOf(t, client)
			if got := p.Num(); got != tt.want {
				t.Fatalf("entry count = %d, want %d", got, tt.want)
			}
			if p.cfg.DCPSelectionStrategy != DCPRoundRobin || p.cfg.DCPMinChannels != tt.want || p.cfg.DCPMaxChannels != tt.want {
				t.Fatalf("fixed pool config = {strategy: %v, min: %d, max: %d}, want {round robin, %d, %d}", p.cfg.DCPSelectionStrategy, p.cfg.DCPMinChannels, p.cfg.DCPMaxChannels, tt.want, tt.want)
			}
		})
	}
}

func TestFixedChannelPoolWithGRPCConnectionPool(t *testing.T) {
	skipDialectRerun(t)
	_, client, teardown := setupMockedTestServerWithConfigAndClientOptions(t, ClientConfig{DisableNativeMetrics: true}, []option.ClientOption{option.WithGRPCConnectionPool(6)})
	defer teardown()
	if got, want := fixedPoolOf(t, client).Num(), 6; got != want {
		t.Fatalf("entry count = %d, want %d", got, want)
	}
}

func TestFixedChannelPoolKeepsNumChannelsMismatchCheck(t *testing.T) {
	skipDialectRerun(t)
	_, opts, teardown := NewMockedSpannerInMemTestServer(t)
	defer teardown()
	_, err := NewClientWithConfig(context.Background(), "projects/p/instances/i/databases/d", ClientConfig{DisableNativeMetrics: true, NumChannels: 2}, append(opts, option.WithGRPCConnectionPool(3))...)
	if ErrCode(err) != codes.InvalidArgument {
		t.Fatalf("NewClientWithConfig() with NumChannels 2 and a pool of 3 error = %v, want INVALID_ARGUMENT", err)
	}
}

// TestFixedChannelPoolRunsNoBackgroundWork checks that the fixed pool under
// load neither scales nor signals scale-up, records no penalties, and keeps no
// pool metrics.
func TestFixedChannelPoolRunsNoBackgroundWork(t *testing.T) {
	skipDialectRerun(t)
	_, client, teardown := setupMockedTestServerWithConfig(t, ClientConfig{DisableNativeMetrics: true, NumChannels: 2})
	defer teardown()
	p := fixedPoolOf(t, client)
	if p.metrics != nil {
		t.Fatal("fixed pool has pool metrics")
	}
	var g errgroup.Group
	for i := 0; i < 16; i++ {
		g.Go(func() error {
			iter := client.Single().Query(context.Background(), NewStatement(SelectFooFromBar))
			defer iter.Stop()
			return iter.Do(func(*Row) error { return nil })
		})
	}
	if err := g.Wait(); err != nil {
		t.Fatal(err)
	}
	for _, e := range p.getEntries() {
		e.applyErrorPenalty(status.Error(codes.Unavailable, "unavailable"))
	}
	if got := p.totalPenaltyLoad.Load(); got != 0 {
		t.Fatalf("fixed pool penalty load = %d, want 0", got)
	}
	if got := len(p.scaleUpSignal); got != 0 {
		t.Fatalf("fixed pool signalled scale-up %d times, want 0", got)
	}
	if got, want := p.Num(), 2; got != want {
		t.Fatalf("entry count = %d, want %d", got, want)
	}
}

// TestFixedChannelPoolStreamsSkipLoadAccounting holds a query stream open on a
// fixed pool and then stops it early, the way an abandoned RowIterator does.
// Fixed entries hand out their gRPC clients directly, so the stream takes no
// entry reference, counts no load, and starts no per-stream cleanup.
func TestFixedChannelPoolStreamsSkipLoadAccounting(t *testing.T) {
	skipDialectRerun(t)
	server, client, teardown := setupMockedTestServerWithConfig(t, ClientConfig{DisableNativeMetrics: true, NumChannels: 2})
	defer teardown()
	p := fixedPoolOf(t, client)
	for i, e := range p.getEntries() {
		if _, ok := e.client.(*grpcSpannerClient); !ok {
			t.Fatalf("entry %d client = %T, want *grpcSpannerClient", i, e.client)
		}
	}
	// Keep the stream open after its first row.
	server.TestSpanner.AddPartialResultSetError(SelectFooFromBar, PartialResultSetExecutionTime{
		ResumeToken:   EncodeResumeToken(2),
		ExecutionTime: 5 * time.Second,
	})
	ro := client.Single()
	iter := ro.Query(context.Background(), NewStatement(SelectFooFromBar))
	defer iter.Stop()
	if _, err := iter.Next(); err != nil {
		t.Fatal(err)
	}
	if c := ro.sh.getClient(); asGRPCSpannerClient(c) != c {
		t.Fatalf("session handle client = %T, want *grpcSpannerClient", c)
	}
	for i, e := range p.getEntries() {
		if load, refs := e.streamLoad.Load(), e.refs.Load(); load != 0 || refs != 0 {
			t.Fatalf("entry %d with an open stream: stream load %d, references %d, want 0 and 0", i, load, refs)
		}
	}
	if got := p.totalRPCLoad.Load(); got != 0 {
		t.Fatalf("pool load with an open stream = %d, want 0", got)
	}
	iter.Stop()
	for i, e := range p.getEntries() {
		if refs := e.refs.Load(); refs != 0 {
			t.Fatalf("entry %d references after Stop = %d, want 0", i, refs)
		}
	}
}

func TestFixedChannelPoolEmitsNoPoolMetrics(t *testing.T) {
	skipDialectRerun(t)
	enableOpenTelemetryMetricsForTest(t)
	reader, mp := newDCPManualReader()
	t.Cleanup(func() { mp.Shutdown(context.Background()) })
	_, client, teardown := setupMockedTestServerWithConfig(t, ClientConfig{DisableNativeMetrics: true, OpenTelemetryMeterProvider: mp})
	defer teardown()
	fixedPoolOf(t, client)
	queryFooFromBar(t, client)
	rm := collectDCPMetrics(t, reader)
	for _, sm := range rm.ScopeMetrics {
		for _, m := range sm.Metrics {
			if strings.Contains(m.Name, "dynamic_channel_pool") {
				t.Fatalf("fixed pool exported %s", m.Name)
			}
		}
	}
}

func TestFixedChannelPoolWithCallerSuppliedConnection(t *testing.T) {
	skipDialectRerun(t)
	_, opts, teardown := NewMockedSpannerInMemTestServer(t)
	defer teardown()
	conn, err := gtransport.Dial(context.Background(), opts...)
	if err != nil {
		t.Fatal(err)
	}
	client, err := NewClientWithConfig(context.Background(), "projects/p/instances/i/databases/d", ClientConfig{DisableNativeMetrics: true}, option.WithGRPCConn(conn))
	if err != nil {
		t.Fatal(err)
	}
	defer client.Close()
	if got, want := fixedPoolOf(t, client).Num(), 1; got != want {
		t.Fatalf("entry count with a caller-supplied connection = %d, want %d", got, want)
	}
	queryFooFromBar(t, client)
}

// closeCountingConnPool wraps a dialed pool and counts Close calls.
type closeCountingConnPool struct {
	gtransport.ConnPool
	closes atomic.Int32
}

func (p *closeCountingConnPool) Close() error {
	p.closes.Add(1)
	return p.ConnPool.Close()
}

// hiddenConnPool sends RPCs through a dialed pool but, like a
// GCPMultiEndpoint, does not expose its connections.
type hiddenConnPool struct{ gtransport.ConnPool }

func (hiddenConnPool) Conn() *grpc.ClientConn { return nil }

// TestFixedChannelPoolWithPoolThatHidesItsConnections gives the client a pool
// whose Conn returns nil: the fixed pool sends RPCs through the pool itself,
// and only closing the client closes it, once.
func TestFixedChannelPoolWithPoolThatHidesItsConnections(t *testing.T) {
	skipDialectRerun(t)
	_, opts, teardown := NewMockedSpannerInMemTestServer(t)
	defer teardown()
	dialed, err := gtransport.DialPool(context.Background(), append(opts, option.WithGRPCConnectionPool(2))...)
	if err != nil {
		t.Fatal(err)
	}
	pool := &closeCountingConnPool{ConnPool: hiddenConnPool{dialed}}
	client, err := NewClientWithConfig(context.Background(), "projects/p/instances/i/databases/d", ClientConfig{DisableNativeMetrics: true}, gtransport.WithConnPool(pool))
	if err != nil {
		t.Fatal(err)
	}
	if got, want := fixedPoolOf(t, client).Num(), 2; got != want {
		t.Fatalf("entry count = %d, want %d", got, want)
	}
	queryFooFromBar(t, client)
	if _, err := client.ReadWriteTransaction(context.Background(), func(ctx context.Context, tx *ReadWriteTransaction) error {
		_, err := tx.Update(ctx, NewStatement(UpdateBarSetFoo))
		return err
	}); err != nil {
		t.Fatalf("ReadWriteTransaction() failed: %v", err)
	}
	if got := pool.closes.Load(); got != 0 {
		t.Fatalf("pool closes while the client is open = %d, want 0", got)
	}
	client.Close()
	if got := pool.closes.Load(); got != 1 {
		t.Fatalf("pool closes after Client.Close = %d, want 1", got)
	}
}

func TestFixedChannelPoolClosesDialedPoolOnce(t *testing.T) {
	skipDialectRerun(t)
	_, opts, teardown := NewMockedSpannerInMemTestServer(t)
	defer teardown()
	dialed, err := gtransport.DialPool(context.Background(), append(opts, option.WithGRPCConnectionPool(3))...)
	if err != nil {
		t.Fatal(err)
	}
	direct := &closeCountingConnPool{ConnPool: dialed}
	database := "projects/p/instances/i/databases/d"
	sc := newSessionClient(nil, database, "", nil, "", false, metadata.Pairs(resourcePrefixHeader, database), 0, nil, nil)
	p, err := newFixedChannelPool(context.Background(), sc, direct, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	seen := map[*grpc.ClientConn]bool{}
	for i, e := range p.getEntries() {
		if e.id != uint64(i+1) {
			t.Fatalf("entry %d id = %d, want %d", i, e.id, i+1)
		}
		seen[slotOf(t, p, i).direct.Conn()] = true
	}
	if len(seen) != 3 {
		t.Fatalf("entries share connections: %d distinct connections for 3 entries", len(seen))
	}
	p.Close()
	p.Close()
	if got := direct.closes.Load(); got != 1 {
		t.Fatalf("dialed pool closes = %d, want 1", got)
	}
}

// TestFixedChannelPoolAfterClientClose uses a transaction, a batch read-only
// transaction and a session that outlive Client.Close. Their RPCs fail with
// CANCELED, as on any closed gRPC connection, which callers do not retry,
// and no step reports an internal error.
func TestFixedChannelPoolAfterClientClose(t *testing.T) {
	skipDialectRerun(t)
	logger := &lockedBuffer{}
	_, client, teardown := setupMockedTestServerWithConfig(t, ClientConfig{DisableNativeMetrics: true, Logger: log.New(logger, "", 0)})
	defer teardown()
	ctx := context.Background()
	ro := client.ReadOnlyTransaction()
	defer ro.Close()
	if err := ro.Query(ctx, NewStatement(SelectFooFromBar)).Do(func(*Row) error { return nil }); err != nil {
		t.Fatal(err)
	}
	batch, err := client.BatchReadOnlyTransaction(ctx, StrongRead())
	if err != nil {
		t.Fatal(err)
	}
	id := batch.ID
	client.Close()

	if err := ro.Query(ctx, NewStatement(SelectFooFromBar)).Do(func(*Row) error { return nil }); ErrCode(err) != codes.Canceled {
		t.Fatalf("query on a read-only transaction after Client.Close error = %v, want CANCELED", err)
	}
	fromID := client.BatchReadOnlyTransactionFromID(id)
	if err := fromID.Query(ctx, NewStatement(SelectFooFromBar)).Do(func(*Row) error { return nil }); ErrCode(err) != codes.Canceled {
		t.Fatalf("query on a batch read-only transaction after Client.Close error = %v, want CANCELED", err)
	}
	if _, err := client.sc.nextClient(); err != nil {
		t.Fatalf("nextClient() after Client.Close error = %v, want a client whose RPCs fail", err)
	}
	if strings.Contains(logger.String(), "internal error") {
		t.Fatalf("the client logged an internal error after Client.Close: %q", logger.String())
	}
}

// TestDynamicChannelPoolPickAfterClose picks an entry from a closed dynamic
// pool: it fails with CANCELED, which callers do not retry.
func TestDynamicChannelPoolPickAfterClose(t *testing.T) {
	skipDialectRerun(t)
	_, client, teardown := setupMockedTestServerWithConfig(t, ClientConfig{DisableNativeMetrics: true, DynamicChannelPoolConfig: testDCPConfig(1, 1, 2)})
	defer teardown()
	p := dynamicPoolOf(t, client)
	client.Close()
	if _, err := p.pick(context.Background()); ErrCode(err) != codes.Canceled {
		t.Fatalf("pick() after Close error = %v, want CANCELED", err)
	}
}

// logRecorder is a slog.Handler that records the level and message of every
// record.
type logRecorder struct {
	mu      sync.Mutex
	records []string
}

func (r *logRecorder) Enabled(context.Context, slog.Level) bool { return true }

func (r *logRecorder) Handle(_ context.Context, rec slog.Record) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.records = append(r.records, rec.Level.String()+" "+rec.Message)
	return nil
}

func (r *logRecorder) WithAttrs([]slog.Attr) slog.Handler { return r }
func (r *logRecorder) WithGroup(string) slog.Handler      { return r }

// count returns the number of records that contain s.
func (r *logRecorder) count(s string) int {
	r.mu.Lock()
	defer r.mu.Unlock()
	n := 0
	for _, rec := range r.records {
		if strings.Contains(rec, s) {
			n++
		}
	}
	return n
}

// lockedBuffer is a bytes.Buffer that is safe for concurrent use.
type lockedBuffer struct {
	mu  sync.Mutex
	buf bytes.Buffer
}

func (b *lockedBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.Write(p)
}

func (b *lockedBuffer) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.String()
}

const dcpUnavailableLog = "the dynamic channel pool is not available"

// assertDCPRequestRunsFixedMode checks that a client runs the fixed pool where
// the dynamic channel pool is not available, whether or not it requested the
// dynamic pool: NumChannels entries picked round robin, with no pool metrics.
// A request for the dynamic pool is logged at debug level only.
func assertDCPRequestRunsFixedMode(t *testing.T, client *Client, dcpEnabled bool, debug *logRecorder, logger *lockedBuffer) {
	t.Helper()
	p := fixedPoolOf(t, client)
	if got, want := p.Num(), numChannels; got != want {
		t.Fatalf("entry count = %d, want NumChannels (%d)", got, want)
	}
	if p.cfg.DCPSelectionStrategy != DCPRoundRobin || p.cfg.DCPMinChannels != numChannels || p.cfg.DCPMaxChannels != numChannels {
		t.Fatalf("fixed pool config = {strategy: %v, min: %d, max: %d}, want {round robin, %d, %d}", p.cfg.DCPSelectionStrategy, p.cfg.DCPMinChannels, p.cfg.DCPMaxChannels, numChannels, numChannels)
	}
	if p.metrics != nil {
		t.Fatal("fixed pool has pool metrics")
	}
	want := 0
	if dcpEnabled {
		want = 1
	}
	if got := debug.count(slog.LevelDebug.String() + " spanner: " + dcpUnavailableLog); got != want {
		t.Fatalf("debug records about the unavailable dynamic pool = %d, want %d", got, want)
	}
	if got := debug.count(dcpUnavailableLog); got != want {
		t.Fatalf("records about the unavailable dynamic pool at any level = %d, want %d", got, want)
	}
	if strings.Contains(logger.String(), dcpUnavailableLog) {
		t.Fatalf("ClientConfig.Logger logged %q", logger.String())
	}
}

// TestFixedChannelPoolWithEmulator runs a client against the emulator, where
// the dynamic channel pool is not available: the client uses fixed mode even
// when the dynamic pool is requested.
func TestFixedChannelPoolWithEmulator(t *testing.T) {
	skipDialectRerun(t)
	for _, dcpEnabled := range []bool{false, true} {
		t.Run(fmt.Sprintf("DCPEnabled=%v", dcpEnabled), func(t *testing.T) {
			server, _, teardown := NewMockedSpannerInMemTestServer(t)
			defer teardown()
			t.Setenv("SPANNER_EMULATOR_HOST", server.ServerAddress)
			debug, logger := &logRecorder{}, &lockedBuffer{}
			config := ClientConfig{DisableNativeMetrics: true, Logger: log.New(logger, "", 0)}
			if dcpEnabled {
				config.DynamicChannelPoolConfig = testDCPConfig(1, 1, 2)
			}
			client, err := NewClientWithConfig(context.Background(), "projects/p/instances/i/databases/d", config, option.WithLogger(slog.New(debug)))
			if err != nil {
				t.Fatal(err)
			}
			defer client.Close()
			assertDCPRequestRunsFixedMode(t, client, dcpEnabled, debug, logger)
			queryFooFromBar(t, client)
		})
	}
}

// TestFixedChannelPoolWithLocationAwareRouting runs location-aware routing,
// where the dynamic channel pool is not available: the client uses fixed
// mode even when the dynamic pool is requested, derives its default endpoint
// authority from the pool's connection, and routes queries and transactions
// through it, each transaction on one of the pool's channels.
func TestFixedChannelPoolWithLocationAwareRouting(t *testing.T) {
	skipDialectRerun(t)
	for _, dcpEnabled := range []bool{false, true} {
		t.Run(fmt.Sprintf("DCPEnabled=%v", dcpEnabled), func(t *testing.T) {
			debug, logger := &logRecorder{}, &lockedBuffer{}
			config := ClientConfig{DisableNativeMetrics: true, IsExperimentalHost: true, Logger: log.New(logger, "", 0)}
			if dcpEnabled {
				config.DynamicChannelPoolConfig = testDCPConfig(1, 1, 2)
			}
			recorder := &dcpChannelRecorder{methods: map[string]bool{executeSQLMethod: true, commitMethod: true}}
			_, client, teardown := setupMockedTestServerWithConfigAndClientOptions(t, config, []option.ClientOption{
				option.WithLogger(slog.New(debug)),
				option.WithGRPCDialOption(grpc.WithUnaryInterceptor(recorder.unaryInterceptor)),
			})
			defer teardown()
			assertDCPRequestRunsFixedMode(t, client, dcpEnabled, debug, logger)
			p := fixedPoolOf(t, client)
			if p.Conn() == nil {
				t.Fatal("fixed pool has no default connection")
			}
			if client.sc.endpointAuthority == "" {
				t.Fatal("location-aware routing has no default endpoint authority")
			}
			queryFooFromBar(t, client)
			used := map[uint64]bool{}
			for i, ids := range runReadWriteTransactionsOnChannels(t, client, recorder, 2*numChannels) {
				if len(ids) != 3 {
					t.Fatalf("transaction %d RPCs = %d, want 2 updates and a commit", i, len(ids))
				}
				for _, id := range ids {
					if id != ids[0] {
						t.Fatalf("transaction %d used channels %v, want one channel", i, ids)
					}
				}
				if ids[0] < 1 || ids[0] > numChannels {
					t.Fatalf("transaction %d channel = %d, want one of the pool's channels 1..%d", i, ids[0], numChannels)
				}
				used[ids[0]] = true
			}
			if len(used) < 2 {
				t.Fatalf("transactions used channels %v, want them spread over the pool", used)
			}
		})
	}
}

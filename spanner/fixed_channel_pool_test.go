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
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	adminpb "cloud.google.com/go/spanner/admin/database/apiv1/databasepb"
	"cloud.google.com/go/spanner/apiv1/spannerpb"
	. "cloud.google.com/go/spanner/internal/testutil"
	"golang.org/x/sync/errgroup"
	"google.golang.org/api/option"
	gtransport "google.golang.org/api/transport/grpc"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
)

const (
	executeSQLMethod = "/google.spanner.v1.Spanner/ExecuteSql"
	commitMethod     = "/google.spanner.v1.Spanner/Commit"

	legacyStaticChannelPoolEnv = "GOOGLE_SPANNER_ENABLE_LEGACY_STATIC_CHANNEL_POOL"
)

var sawGoogleSQLPass atomic.Bool

// skipDialectRerun skips a dialect-independent test in the PostgreSQL pass of
// TestMain if the GoogleSQL pass already ran it.
func skipDialectRerun(t *testing.T) {
	t.Helper()
	if testDialect != adminpb.DatabaseDialect_POSTGRESQL {
		sawGoogleSQLPass.Store(true)
		return
	}
	if sawGoogleSQLPass.Load() {
		t.Skip("dialect independent; already ran in the GoogleSQL pass")
	}
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

// transactionChannelIDs returns the channel ids used by each of n read-write
// transactions.
func transactionChannelIDs(t *testing.T, client *Client, recorder *dcpChannelRecorder, n int) [][]uint64 {
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

// assertTransactionsBindToChannels checks that each transaction stays on one
// of the channels 1..n and that all n channels are used.
func assertTransactionsBindToChannels(t *testing.T, client *Client, recorder *dcpChannelRecorder, n int) {
	t.Helper()
	used := map[uint64]int{}
	for i, ids := range transactionChannelIDs(t, client, recorder, 2*n) {
		if len(ids) != 3 {
			t.Fatalf("transaction %d recorded %d RPCs, want 2 updates and a commit (%v)", i, len(ids), ids)
		}
		for _, id := range ids {
			if id != ids[0] {
				t.Fatalf("transaction %d used channels %v, want one channel", i, ids)
			}
		}
		if ids[0] < 1 || ids[0] > uint64(n) {
			t.Fatalf("transaction %d channel id = %d, want one in [1, %d]", i, ids[0], n)
		}
		used[ids[0]]++
	}
	if len(used) != n {
		t.Fatalf("transactions used channels %v, want all %d channels", used, n)
	}
}

func fixedPoolOf(t *testing.T, client *Client) *dynamicChannelPool {
	t.Helper()
	p := client.sc.dynamicPool
	if p == nil || !p.fixed {
		t.Fatalf("channel pool = %T (fixed: %v), want the dynamic channel pool in fixed mode", client.sc.connPool, p != nil && p.fixed)
	}
	return p
}

func channelPoolKind(client *Client) string {
	switch p := client.sc.connPool.(type) {
	case *dynamicChannelPool:
		if p.fixed {
			return "fixed"
		}
		return "dynamic"
	case *fallbackWrapper:
		return "static with GCPFallback"
	default:
		return "static"
	}
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
	assertTransactionsBindToChannels(t, client, recorder, 4)
}

func TestChannelPoolSelection(t *testing.T) {
	skipDialectRerun(t)
	const (
		directAccessEnv = "GOOGLE_SPANNER_ENABLE_DIRECT_ACCESS"
		gcpFallbackEnv  = "GOOGLE_SPANNER_ENABLE_GCP_FALLBACK"
	)
	for _, tt := range []struct {
		name string
		env  map[string]string
		dcp  bool
		want string
	}{
		{name: "default", want: "fixed"},
		{name: "legacy static pool disabled", env: map[string]string{legacyStaticChannelPoolEnv: "false"}, want: "fixed"},
		{name: "legacy static pool with an invalid value", env: map[string]string{legacyStaticChannelPoolEnv: "yes please"}, want: "fixed"},
		{name: "legacy static pool", env: map[string]string{legacyStaticChannelPoolEnv: "true"}, want: "static"},
		{name: "legacy static pool set to 1", env: map[string]string{legacyStaticChannelPoolEnv: "1"}, want: "static"},
		{name: "dynamic pool", dcp: true, want: "dynamic"},
		{name: "dynamic pool with legacy static pool", env: map[string]string{legacyStaticChannelPoolEnv: "true"}, dcp: true, want: "dynamic"},
		{name: "DirectPath", env: map[string]string{directAccessEnv: "true"}, want: "fixed"},
		{name: "DirectPath with legacy static pool", env: map[string]string{directAccessEnv: "true", legacyStaticChannelPoolEnv: "true"}, want: "static"},
		{name: "fallback without DirectPath", env: map[string]string{gcpFallbackEnv: "true"}, want: "fixed"},
		{name: "DirectPath with fallback", env: map[string]string{directAccessEnv: "true", gcpFallbackEnv: "true"}, want: "static with GCPFallback"},
		{name: "DirectPath with fallback and legacy static pool", env: map[string]string{directAccessEnv: "true", gcpFallbackEnv: "true", legacyStaticChannelPoolEnv: "true"}, want: "static with GCPFallback"},
		{name: "dynamic pool with DirectPath and fallback", env: map[string]string{directAccessEnv: "true", gcpFallbackEnv: "true"}, dcp: true, want: "dynamic"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			for k, v := range tt.env {
				t.Setenv(k, v)
			}
			config := ClientConfig{DisableNativeMetrics: true}
			if tt.dcp {
				config.DynamicChannelPoolConfig = testDCPConfig(1, 1, 2)
			}
			_, client, teardown := setupMockedTestServerWithConfig(t, config)
			defer teardown()
			if got := channelPoolKind(client); got != tt.want {
				t.Fatalf("channel pool = %s (%T), want %s", got, client.sc.connPool, tt.want)
			}
			queryFooFromBar(t, client)
		})
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

type emptyConnPool struct{ gtransport.ConnPool }

func (emptyConnPool) Num() int { return 0 }

func TestFixedChannelPoolRejectsPoolWithoutConnections(t *testing.T) {
	skipDialectRerun(t)
	_, opts, teardown := NewMockedSpannerInMemTestServer(t)
	defer teardown()
	dialed, err := gtransport.DialPool(context.Background(), opts...)
	if err != nil {
		t.Fatal(err)
	}
	defer dialed.Close()
	const database = "projects/p/instances/i/databases/d"
	before := clientOrdinal(database)
	_, err = NewClientWithConfig(context.Background(), database, ClientConfig{DisableNativeMetrics: true}, gtransport.WithConnPool(emptyConnPool{dialed}))
	if ErrCode(err) != codes.InvalidArgument {
		t.Fatalf("NewClientWithConfig() with a pool of no connections error = %v, want INVALID_ARGUMENT", err)
	}
	if after := clientOrdinal(database); after != before {
		t.Fatalf("client ordinal for %s after a rejected pool = %d, want %d (unchanged)", database, after, before)
	}
}

func clientOrdinal(database string) int {
	cidGen.mu.Lock()
	defer cidGen.mu.Unlock()
	return cidGen.ids[database]
}

// repeatingConnPool returns the same connection from every Conn call.
type repeatingConnPool struct {
	gtransport.ConnPool
	conn *grpc.ClientConn
}

func (p repeatingConnPool) Conn() *grpc.ClientConn { return p.conn }

func TestFixedChannelPoolWithPoolThatRepeatsAConnection(t *testing.T) {
	skipDialectRerun(t)
	_, opts, teardown := NewMockedSpannerInMemTestServer(t)
	defer teardown()
	recorder := &dcpChannelRecorder{methods: map[string]bool{executeSQLMethod: true, commitMethod: true}}
	dialed, err := gtransport.DialPool(context.Background(), append(opts,
		option.WithGRPCConnectionPool(2),
		option.WithGRPCDialOption(grpc.WithUnaryInterceptor(recorder.unaryInterceptor)),
	)...)
	if err != nil {
		t.Fatal(err)
	}
	client, err := NewClientWithConfig(context.Background(), "projects/p/instances/i/databases/d", ClientConfig{DisableNativeMetrics: true}, gtransport.WithConnPool(repeatingConnPool{dialed, dialed.Conn()}))
	if err != nil {
		t.Fatal(err)
	}
	defer client.Close()
	if got, want := fixedPoolOf(t, client).Num(), 1; got != want {
		t.Fatalf("entry count for a pool that returns one connection = %d, want %d", got, want)
	}
	assertTransactionsBindToChannels(t, client, recorder, 1)
}

func TestFixedChannelPoolPicksWithoutActivityTracking(t *testing.T) {
	skipDialectRerun(t)
	_, client, teardown := setupMockedTestServerWithConfig(t, ClientConfig{DisableNativeMetrics: true, NumChannels: 2})
	defer teardown()
	p := fixedPoolOf(t, client)
	for _, e := range p.getEntries() {
		e.lastActivity.Store(0)
	}
	for i := 0; i < 4; i++ {
		queryFooFromBar(t, client)
	}
	for i, e := range p.getEntries() {
		if got := e.lastActivity.Load(); got != 0 {
			t.Fatalf("entry %d activity time after picks = %d, want 0", i, got)
		}
	}
}

func TestFixedChannelPoolConnPoolMethodsSkipLoadAccounting(t *testing.T) {
	skipDialectRerun(t)
	// Use a bare server: these raw RPCs lack headers that setupMockedTestServer checks.
	_, opts, teardown := NewMockedSpannerInMemTestServer(t)
	defer teardown()
	client, err := NewClientWithConfig(context.Background(), "projects/p/instances/i/databases/d", ClientConfig{DisableNativeMetrics: true, NumChannels: 1}, opts...)
	if err != nil {
		t.Fatal(err)
	}
	defer client.Close()
	p := fixedPoolOf(t, client)
	e := p.getEntries()[0]
	e.lastActivity.Store(0)
	ctx, cancel := context.WithCancel(context.Background())
	// More streams than the default DCPMaxRPCPerChannel (25).
	for i := 0; i < 26; i++ {
		if _, err := p.NewStream(ctx, &grpc.StreamDesc{ServerStreams: true}, "/google.spanner.v1.Spanner/ExecuteStreamingSql"); err != nil {
			t.Fatalf("NewStream() failed: %v", err)
		}
	}
	cancel()
	_ = p.Invoke(context.Background(), "/google.spanner.v1.Spanner/GetSession", &spannerpb.GetSessionRequest{Name: "s"}, &spannerpb.Session{})
	if load, total := e.rpcLoad(), p.totalRPCLoad.Load(); load != 0 || total != 0 {
		t.Fatalf("load after abandoned streams = {entry: %d, pool: %d}, want {0, 0}", load, total)
	}
	if got := e.lastActivity.Load(); got != 0 {
		t.Fatalf("entry activity time = %d, want 0", got)
	}
}

func TestFixedChannelPoolDoesNotRetainCallerContext(t *testing.T) {
	skipDialectRerun(t)
	_, opts, teardown := NewMockedSpannerInMemTestServer(t)
	defer teardown()
	ctx, cancel := context.WithCancel(context.Background())
	client, err := NewClientWithConfig(ctx, "projects/p/instances/i/databases/d", ClientConfig{DisableNativeMetrics: true}, opts...)
	cancel()
	if err != nil {
		t.Fatal(err)
	}
	defer client.Close()
	p := fixedPoolOf(t, client)
	if p.ctx != nil || p.cancel != nil || p.done != nil || p.scaleUpSignal != nil {
		t.Fatalf("fixed pool background state = {ctx: %v, cancel: %v, done: %v, scale-up signal: %v}, want none", p.ctx != nil, p.cancel != nil, p.done != nil, p.scaleUpSignal != nil)
	}
	queryFooFromBar(t, client)
}

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
	if got := len(p.scaleUpSignal); got != 0 {
		t.Fatalf("fixed pool signalled scale-up %d times, want 0", got)
	}
	if p.hasPrimeSession() {
		t.Fatal("fixed pool recorded a session to prime channels with")
	}
	if got, want := p.Num(), 2; got != want {
		t.Fatalf("entry count = %d, want %d", got, want)
	}
}

// TestFixedChannelPoolStreamsSkipLoadAccounting checks that an open query
// stream adds no load or references to its entry.
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
	if _, ok := ro.sh.getClient().(*grpcSpannerClient); !ok {
		t.Fatalf("session handle client = %T, want *grpcSpannerClient", ro.sh.getClient())
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

type closeCountingConnPool struct {
	gtransport.ConnPool
	closes atomic.Int32
}

func (p *closeCountingConnPool) Close() error {
	p.closes.Add(1)
	return p.ConnPool.Close()
}

// hiddenConnPool does not expose its connections, like a GCPMultiEndpoint.
type hiddenConnPool struct{ gtransport.ConnPool }

func (hiddenConnPool) Conn() *grpc.ClientConn { return nil }

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
	underlying := &closeCountingConnPool{ConnPool: dialed}
	database := "projects/p/instances/i/databases/d"
	sc := newSessionClient(nil, database, "", nil, "", false, metadata.Pairs(resourcePrefixHeader, database), 0, nil, nil)
	p, err := newFixedChannelPool(context.Background(), sc, underlying)
	if err != nil {
		t.Fatal(err)
	}
	seen := map[*grpc.ClientConn]bool{}
	for i, e := range p.getEntries() {
		if e.id != uint64(i+1) {
			t.Fatalf("entry %d id = %d, want %d", i, e.id, i+1)
		}
		seen[e.pool.Conn()] = true
	}
	if len(seen) != 3 {
		t.Fatalf("entries share connections: %d distinct connections for 3 entries", len(seen))
	}
	p.Close()
	p.Close()
	if got := underlying.closes.Load(); got != 1 {
		t.Fatalf("dialed pool closes = %d, want 1", got)
	}
}

// TestFixedChannelPoolAfterClientClose checks that RPCs after Client.Close
// fail with CANCELED, as with the static pool.
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

func TestLegacyStaticChannelPool(t *testing.T) {
	skipDialectRerun(t)
	for _, tt := range []struct {
		name   string
		config ClientConfig
		opts   []option.ClientOption
		want   int
	}{
		{name: "NumChannels", config: ClientConfig{DisableNativeMetrics: true, NumChannels: 4}, want: 4},
		{name: "WithGRPCConnectionPool", config: ClientConfig{DisableNativeMetrics: true}, opts: []option.ClientOption{option.WithGRPCConnectionPool(6)}, want: 6},
	} {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv(legacyStaticChannelPoolEnv, "true")
			recorder := &dcpChannelRecorder{methods: map[string]bool{executeSQLMethod: true, commitMethod: true}}
			opts := append([]option.ClientOption{option.WithGRPCDialOption(grpc.WithUnaryInterceptor(recorder.unaryInterceptor))}, tt.opts...)
			_, client, teardown := setupMockedTestServerWithConfigAndClientOptions(t, tt.config, opts)
			defer teardown()
			if got := channelPoolKind(client); got != "static" || client.sc.dynamicPool != nil {
				t.Fatalf("channel pool = %s (%T, dynamic pool %v), want static", got, client.sc.connPool, client.sc.dynamicPool != nil)
			}
			if got := client.sc.connPool.Num(); got != tt.want {
				t.Fatalf("connection count = %d, want %d", got, tt.want)
			}
			assertTransactionsBindToChannels(t, client, recorder, tt.want)
			ro := client.Single()
			defer ro.Close()
			iter := ro.Query(context.Background(), NewStatement(SelectFooFromBar))
			defer iter.Stop()
			if _, err := iter.Next(); err != nil {
				t.Fatal(err)
			}
			if _, ok := ro.sh.getClient().(*grpcSpannerClient); !ok {
				t.Fatalf("session handle client = %T, want *grpcSpannerClient", ro.sh.getClient())
			}
			if got := len(client.sc.channelIDMap); got != tt.want {
				t.Fatalf("static pool channel ids = %d, want %d", got, tt.want)
			}
		})
	}
}

func TestDirectPathFallbackKeepsStaticChannelPool(t *testing.T) {
	skipDialectRerun(t)
	for _, legacy := range []string{"", "true"} {
		t.Run(fmt.Sprintf("legacy static pool=%q", legacy), func(t *testing.T) {
			t.Setenv("GOOGLE_SPANNER_ENABLE_DIRECT_ACCESS", "true")
			t.Setenv("GOOGLE_SPANNER_ENABLE_GCP_FALLBACK", "true")
			if legacy != "" {
				t.Setenv(legacyStaticChannelPoolEnv, legacy)
			}
			_, client, teardown := setupMockedTestServerWithConfig(t, ClientConfig{DisableNativeMetrics: true, NumChannels: 3})
			defer teardown()
			fw, ok := client.sc.connPool.(*fallbackWrapper)
			if !ok || client.sc.dynamicPool != nil {
				t.Fatalf("channel pool = %T (dynamic pool %v), want *fallbackWrapper", client.sc.connPool, client.sc.dynamicPool != nil)
			}
			if fw.GCPFallback == nil {
				t.Fatal("fallback pool has no GCPFallback")
			}
			if p, f := fw.primaryConn.Num(), fw.fallbackConn.Num(); p != 3 || f != 3 {
				t.Fatalf("connections = {primary: %d, fallback: %d}, want {3, 3}", p, f)
			}
			if _, err := client.ReadWriteTransaction(context.Background(), func(ctx context.Context, tx *ReadWriteTransaction) error {
				_, err := tx.Update(ctx, NewStatement(UpdateBarSetFoo))
				return err
			}); err != nil {
				t.Fatalf("ReadWriteTransaction() failed: %v", err)
			}
			ro := client.Single()
			defer ro.Close()
			iter := ro.Query(context.Background(), NewStatement(SelectFooFromBar))
			defer iter.Stop()
			if _, err := iter.Next(); err != nil {
				t.Fatal(err)
			}
			c, ok := ro.sh.getClient().(*grpcSpannerClient)
			if !ok {
				t.Fatalf("session handle client = %T, want *grpcSpannerClient", ro.sh.getClient())
			}
			if c.channelID != 0 {
				t.Fatalf("session handle channel id = %d, want 0", c.channelID)
			}
		})
	}
}

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

const dcpUnavailableLog = "DynamicChannelPoolConfig is ignored"

// assertDCPRequestRunsFixedMode checks that client uses a fixed pool of
// numChannels and that logger got wantLogs dcpUnavailableLog lines.
func assertDCPRequestRunsFixedMode(t *testing.T, client *Client, wantLogs int, logger *lockedBuffer) {
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
	if got := strings.Count(logger.String(), dcpUnavailableLog); got != wantLogs {
		t.Fatalf("ClientConfig.Logger lines about the ignored DynamicChannelPoolConfig = %d, want %d; log: %q", got, wantLogs, logger.String())
	}
}

func TestFixedChannelPoolWithEmulator(t *testing.T) {
	skipDialectRerun(t)
	for _, tt := range []struct {
		dcpEnabled bool
		legacy     bool
	}{{false, false}, {true, false}, {true, true}} {
		t.Run(fmt.Sprintf("DCPEnabled=%v/legacy static pool=%v", tt.dcpEnabled, tt.legacy), func(t *testing.T) {
			server, _, teardown := NewMockedSpannerInMemTestServer(t)
			defer teardown()
			t.Setenv("SPANNER_EMULATOR_HOST", server.ServerAddress)
			if tt.legacy {
				t.Setenv(legacyStaticChannelPoolEnv, "true")
			}
			logger := &lockedBuffer{}
			config := ClientConfig{DisableNativeMetrics: true, Logger: log.New(logger, "", 0)}
			if tt.dcpEnabled {
				config.DynamicChannelPoolConfig = testDCPConfig(1, 1, 2)
			}
			client, err := NewClientWithConfig(context.Background(), "projects/p/instances/i/databases/d", config)
			if err != nil {
				t.Fatal(err)
			}
			defer client.Close()
			if tt.legacy {
				if got := channelPoolKind(client); got != "static" {
					t.Fatalf("channel pool = %s (%T), want static", got, client.sc.connPool)
				}
				if strings.Contains(logger.String(), dcpUnavailableLog) {
					t.Fatalf("ClientConfig.Logger logged %q", logger.String())
				}
			} else {
				assertDCPRequestRunsFixedMode(t, client, 0, logger)
			}
			queryFooFromBar(t, client)
		})
	}
}

func TestFixedChannelPoolWithLocationAwareRouting(t *testing.T) {
	skipDialectRerun(t)
	for _, dcpEnabled := range []bool{false, true} {
		t.Run(fmt.Sprintf("DCPEnabled=%v", dcpEnabled), func(t *testing.T) {
			logger := &lockedBuffer{}
			config := ClientConfig{DisableNativeMetrics: true, IsExperimentalHost: true, Logger: log.New(logger, "", 0)}
			if dcpEnabled {
				config.DynamicChannelPoolConfig = testDCPConfig(1, 1, 2)
			}
			recorder := &dcpChannelRecorder{methods: map[string]bool{executeSQLMethod: true, commitMethod: true}}
			_, client, teardown := setupMockedTestServerWithConfigAndClientOptions(t, config, []option.ClientOption{
				option.WithGRPCDialOption(grpc.WithUnaryInterceptor(recorder.unaryInterceptor)),
			})
			defer teardown()
			wantLogs := 0
			if dcpEnabled {
				wantLogs = 1
			}
			assertDCPRequestRunsFixedMode(t, client, wantLogs, logger)
			p := fixedPoolOf(t, client)
			if p.Conn() == nil {
				t.Fatal("fixed pool has no default connection")
			}
			if client.sc.endpointAuthority == "" {
				t.Fatal("location-aware routing has no default endpoint authority")
			}
			queryFooFromBar(t, client)
			used := map[uint64]bool{}
			for i, ids := range transactionChannelIDs(t, client, recorder, 2*numChannels) {
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

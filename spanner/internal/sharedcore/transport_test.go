// Copyright 2026 Google LLC
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package sharedcore

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log"
	"net"
	"reflect"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	sppb "cloud.google.com/go/spanner/apiv1/spannerpb"
	"cloud.google.com/go/spanner/internal/requestid"
	"cloud.google.com/go/spanner/internal/transport"
	"github.com/googleapis/gax-go/v2"
	"google.golang.org/api/option"
	gtransport "google.golang.org/api/transport/grpc"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
)

type mockSpannerServer struct {
	sppb.UnimplementedSpannerServer
	mu           sync.Mutex
	handler      func(req *sppb.ExecuteSqlRequest, stream sppb.Spanner_ExecuteStreamingSqlServer) error
	unaryHandler func(ctx context.Context, req *sppb.ExecuteSqlRequest) (*sppb.ResultSet, error)
}

func (s *mockSpannerServer) setHandler(h func(req *sppb.ExecuteSqlRequest, stream sppb.Spanner_ExecuteStreamingSqlServer) error) {
	s.mu.Lock()
	s.handler = h
	s.mu.Unlock()
}

func (s *mockSpannerServer) setUnaryHandler(h func(ctx context.Context, req *sppb.ExecuteSqlRequest) (*sppb.ResultSet, error)) {
	s.mu.Lock()
	s.unaryHandler = h
	s.mu.Unlock()
}

func (s *mockSpannerServer) ExecuteStreamingSql(req *sppb.ExecuteSqlRequest, stream sppb.Spanner_ExecuteStreamingSqlServer) error {
	s.mu.Lock()
	h := s.handler
	s.mu.Unlock()
	if h != nil {
		return h(req, stream)
	}
	return status.Error(codes.Unimplemented, "not configured")
}

func (s *mockSpannerServer) ExecuteSql(ctx context.Context, req *sppb.ExecuteSqlRequest) (*sppb.ResultSet, error) {
	s.mu.Lock()
	uh := s.unaryHandler
	s.mu.Unlock()
	if uh != nil {
		return uh(ctx, req)
	}
	return &sppb.ResultSet{}, nil
}

func startMockServer(tb testing.TB) (*mockSpannerServer, string) {
	tb.Helper()
	lis, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		tb.Fatalf("failed to listen: %v", err)
	}
	srv := grpc.NewServer()
	mock := &mockSpannerServer{}
	sppb.RegisterSpannerServer(srv, mock)
	go func() {
		_ = srv.Serve(lis)
	}()
	tb.Cleanup(func() {
		srv.Stop()
		_ = lis.Close()
	})
	return mock, lis.Addr().String()
}

type zeroConnPool struct {
	closes atomic.Int32
}

func (z *zeroConnPool) Conn() *grpc.ClientConn { return nil }
func (z *zeroConnPool) Num() int               { return 0 }
func (z *zeroConnPool) Close() error {
	z.closes.Add(1)
	return nil
}
func (z *zeroConnPool) Invoke(context.Context, string, any, any, ...grpc.CallOption) error {
	return errors.New("empty pool")
}
func (z *zeroConnPool) NewStream(context.Context, *grpc.StreamDesc, string, ...grpc.CallOption) (grpc.ClientStream, error) {
	return nil, errors.New("empty pool")
}

type panickingChannelClient struct{}

func (*panickingChannelClient) Close() error { return nil }
func (*panickingChannelClient) RequestIDHeaderInjector(context.Context) (*requestid.Wrap, error) {
	return nil, nil
}
func (*panickingChannelClient) ExecuteSql(context.Context, *sppb.ExecuteSqlRequest, ...gax.CallOption) (*sppb.ResultSet, error) {
	panic("injected unary panic")
}
func (*panickingChannelClient) ExecuteStreamingSql(context.Context, *sppb.ExecuteSqlRequest, ...gax.CallOption) (sppb.Spanner_ExecuteStreamingSqlClient, error) {
	panic("injected stream panic")
}

type failingStreamChannelClient struct {
	err error
}

func (f failingStreamChannelClient) Close() error { return nil }
func (f failingStreamChannelClient) RequestIDHeaderInjector(context.Context) (*requestid.Wrap, error) {
	return nil, f.err
}
func (f failingStreamChannelClient) ExecuteSql(context.Context, *sppb.ExecuteSqlRequest, ...gax.CallOption) (*sppb.ResultSet, error) {
	return nil, f.err
}
func (f failingStreamChannelClient) ExecuteStreamingSql(context.Context, *sppb.ExecuteSqlRequest, ...gax.CallOption) (sppb.Spanner_ExecuteStreamingSqlClient, error) {
	return nil, f.err
}

// dcpScaleRecorder observes DCP scale-up publications via a non-blocking send
// (RecordScaleUp is called while DialMu is held).
type dcpScaleRecorder struct {
	scaledUp chan int64
}

func (r *dcpScaleRecorder) RecordScaleUp(_ context.Context, channels int64) {
	select {
	case r.scaledUp <- channels:
	default:
	}
}

func (r *dcpScaleRecorder) RecordScaleDown(context.Context, int64) {}
func (r *dcpScaleRecorder) Close(*log.Logger)                      {}

func newScaleUpHooks(scaledUp chan int64, primeRetryBaseDelay time.Duration) dcpTestHooks {
	return dcpTestHooks{
		initMetrics: func(*transport.DynamicChannelPool[managedChannelClient]) transport.DCPMetricsRecorder {
			return &dcpScaleRecorder{scaledUp: scaledUp}
		},
		primeRetryBaseDelay: primeRetryBaseDelay,
	}
}

func openBlockingStreams(t *testing.T, ctx context.Context, tr *Transport, session string, count int) {
	t.Helper()
	for i := 0; i < count; i++ {
		c, err := tr.PickClient(ctx)
		if err != nil {
			t.Fatalf("PickClient(%d) failed: %v", i, err)
		}
		riw, err := c.RequestIDHeaderInjector(ctx)
		if err != nil {
			t.Fatalf("RequestIDHeaderInjector(%d) failed: %v", i, err)
		}
		st, err := c.ExecuteStreamingSql(ctx, &sppb.ExecuteSqlRequest{
			Session: session,
			Sql:     "BLOCK",
		}, riw.WithNextRetryAttempt(1))
		if err != nil {
			t.Fatalf("blocking ExecuteStreamingSql(%d) failed: %v", i, err)
		}
		if _, err := st.Recv(); err != nil {
			t.Fatalf("blocking stream.Recv(%d) failed: %v", i, err)
		}
	}
}

func TestDCPSelectionStrategyABI(t *testing.T) {
	t.Parallel()
	// C/FFI bindings encode DCPSelectionStrategy as raw integers (0 = P2C, 1 = RoundRobin).
	if transport.DCPPowerOfTwoLeastBusy != 0 {
		t.Fatalf("DCPPowerOfTwoLeastBusy = %d, want 0", transport.DCPPowerOfTwoLeastBusy)
	}
	if transport.DCPRoundRobin != 1 {
		t.Fatalf("DCPRoundRobin = %d, want 1", transport.DCPRoundRobin)
	}
	var cfg DynamicChannelPoolConfig
	var _ transport.DCPConfig = cfg
	var strat DCPSelectionStrategy = transport.DCPRoundRobin
	var _ transport.DCPSelectionStrategy = strat
}

func TestTransport_FixedPoolRoundRobin(t *testing.T) {
	t.Parallel()
	mock, addr := startMockServer(t)

	var (
		mu     sync.Mutex
		reqIDs []string
	)
	mock.setHandler(func(req *sppb.ExecuteSqlRequest, stream sppb.Spanner_ExecuteStreamingSqlServer) error {
		if md, ok := metadata.FromIncomingContext(stream.Context()); ok {
			if vals := md.Get(requestid.Header); len(vals) > 0 {
				mu.Lock()
				reqIDs = append(reqIDs, vals[0])
				mu.Unlock()
			}
		}
		return stream.Send(&sppb.PartialResultSet{})
	})

	const numCh = 4
	tr, err := NewTransport(context.Background(), ClientConfig{
		Endpoint:    addr,
		NumChannels: numCh,
		PlainText:   true,
	})
	if err != nil {
		t.Fatalf("NewTransport failed: %v", err)
	}
	defer tr.Close()

	if tr.dcp == nil || !tr.dcp.IsFixed() {
		t.Fatalf("expected default DCPEnabled=false to use fixed-mode pool")
	}
	if got := tr.dcp.Num(); got != numCh {
		t.Fatalf("expected %d fixed channels, got %d", numCh, got)
	}
	// UpdateDCPPrimeSession is a no-op on fixed pools.
	tr.UpdateDCPPrimeSession("projects/p/instances/i/databases/d/sessions/s1")
	if got := tr.dcp.PrimeSessionID(); got != "" {
		t.Fatalf("expected empty PrimeSessionID on fixed pool, got %q", got)
	}

	// 1. Pure PickClient() round-robin without intervening RPCs.
	pureSeen := make(map[ChannelClient]bool, numCh)
	var prevClient ChannelClient
	for i := 0; i < numCh*2; i++ {
		c, err := tr.PickClient(context.Background())
		if err != nil {
			t.Fatalf("pure PickClient(%d) failed: %v", i, err)
		}
		if c == prevClient {
			t.Fatalf("pure PickClient(%d) repeated previous client", i)
		}
		prevClient = c
		pureSeen[c] = true
	}
	if len(pureSeen) != numCh {
		t.Fatalf("expected %d distinct ChannelClients, got %d", numCh, len(pureSeen))
	}

	// 2. Interleave PickClient() with streaming RPCs (advancing nthRequest).
	entries := tr.dcp.GetEntries()
	seen := make(map[ChannelClient]bool, numCh)
	for i := 0; i < numCh*2; i++ {
		c, err := tr.PickClient(context.Background())
		if err != nil {
			t.Fatalf("PickClient(%d) failed: %v", i, err)
		}
		if c != entries[i%numCh].Client {
			t.Fatalf("PickClient(%d) did not select expected fixed entry %d", i, i%numCh)
		}
		seen[c] = true
		riw, err := c.RequestIDHeaderInjector(context.Background())
		if err != nil {
			t.Fatalf("RequestIDHeaderInjector(%d) failed: %v", i, err)
		}
		stream, err := c.ExecuteStreamingSql(context.Background(), &sppb.ExecuteSqlRequest{
			Session: "projects/p/instances/i/databases/d/sessions/s",
			Sql:     "SELECT 1",
		}, riw.WithNextRetryAttempt(1))
		if err != nil {
			t.Fatalf("ExecuteStreamingSql(%d) failed: %v", i, err)
		}
		if _, err := stream.Recv(); err != nil {
			t.Fatalf("stream.Recv(%d) failed: %v", i, err)
		}
		if _, err := stream.Recv(); err != io.EOF {
			t.Fatalf("drain stream.Recv(%d) = %v, want io.EOF", i, err)
		}
	}
	if len(seen) != numCh {
		t.Fatalf("expected %d distinct ChannelClients, got %d", numCh, len(seen))
	}
	mu.Lock()
	gotReqIDs := append([]string(nil), reqIDs...)
	mu.Unlock()
	if len(gotReqIDs) != numCh*2 {
		t.Fatalf("expected %d x-goog-spanner-request-id headers, got %d", numCh*2, len(gotReqIDs))
	}
	for i, id := range gotReqIDs {
		wantSuffix := "." + strconv.Itoa((i%numCh)+1) + "." + strconv.Itoa(i+1) + ".1"
		if !strings.HasSuffix(id, wantSuffix) {
			t.Fatalf("request %d: expected x-goog-spanner-request-id ending with %q, got %q", i, wantSuffix, id)
		}
	}
}

func TestTransport_FixedPoolSingleChannel(t *testing.T) {
	t.Parallel()
	_, addr := startMockServer(t)

	tr, err := NewTransport(context.Background(), ClientConfig{
		Endpoint:    addr,
		NumChannels: 1,
		PlainText:   true,
	})
	if err != nil {
		t.Fatalf("NewTransport(NumChannels=1) failed: %v", err)
	}
	defer tr.Close()

	c, err := tr.PickClient(context.Background())
	if err != nil || c == nil {
		t.Fatalf("PickClient on NumChannels=1 failed: %v", err)
	}
}

func TestTransport_DCPScaleUpAndPrimeHeaders(t *testing.T) {
	t.Parallel()
	mock, addr := startMockServer(t)

	var (
		primeMu       sync.Mutex
		primePrefixes [][]string
		primeEncs     [][]string
	)
	mock.setUnaryHandler(func(ctx context.Context, req *sppb.ExecuteSqlRequest) (*sppb.ResultSet, error) {
		if md, ok := metadata.FromIncomingContext(ctx); ok {
			primeMu.Lock()
			primePrefixes = append(primePrefixes, append([]string(nil), md.Get(transport.ResourcePrefixHeader)...))
			primeEncs = append(primeEncs, append([]string(nil), md.Get(transport.RequestsCompressionHeader)...))
			primeMu.Unlock()
		}
		return &sppb.ResultSet{}, nil
	})

	unblock := make(chan struct{})
	mock.setHandler(func(req *sppb.ExecuteSqlRequest, stream sppb.Spanner_ExecuteStreamingSqlServer) error {
		if req.GetSql() == "BLOCK" {
			if err := stream.Send(&sppb.PartialResultSet{}); err != nil {
				return err
			}
			select {
			case <-unblock:
				return nil
			case <-stream.Context().Done():
				return stream.Context().Err()
			}
		}
		return stream.Send(&sppb.PartialResultSet{Last: true})
	})

	scaledUp := make(chan int64, 4)
	tr, err := newTransportWithDCPHooks(context.Background(), ClientConfig{
		Endpoint:    addr,
		PlainText:   true,
		Compression: "gzip",
		DynamicChannelPoolConfig: DynamicChannelPoolConfig{
			DCPEnabled:          true,
			DCPInitialChannels:  2,
			DCPMinChannels:      2,
			DCPMaxChannels:      4,
			DCPMinRPCPerChannel: 1,
			DCPMaxRPCPerChannel: 2,
			DCPScaleUpCooldown:  time.Nanosecond,
		},
	}, newScaleUpHooks(scaledUp, time.Nanosecond))
	if err != nil {
		t.Fatalf("NewTransport with DCP failed: %v", err)
	}
	streamCtx, cancelStreams := context.WithCancel(context.Background())
	defer func() {
		close(unblock)
		cancelStreams()
		_ = tr.Close()
	}()

	if tr.dcp == nil || tr.dcp.IsFixed() {
		t.Fatal("expected active non-fixed DynamicChannelPool")
	}
	if got := tr.dcp.Num(); got != 2 {
		t.Fatalf("expected 2 initial channels, got %d", got)
	}

	// Exercise dcpResolvingChannelClient + dcpChannelClient unary and streaming methods.
	dcpPick, err := tr.PickClient(context.Background())
	if err != nil {
		t.Fatalf("PickClient on DCP failed: %v", err)
	}
	dcpRIW, err := dcpPick.RequestIDHeaderInjector(context.Background())
	if err != nil || dcpRIW == nil {
		t.Fatalf("DCP RequestIDHeaderInjector failed: %v", err)
	}
	if _, err := dcpPick.ExecuteSql(context.Background(), &sppb.ExecuteSqlRequest{
		Session: "projects/p/instances/i/databases/db-prime/sessions/s1",
		Sql:     "SELECT 1",
	}); err != nil {
		t.Fatalf("DCP ExecuteSql failed: %v", err)
	}
	dcpStream, err := dcpPick.ExecuteStreamingSql(context.Background(), &sppb.ExecuteSqlRequest{
		Session: "projects/p/instances/i/databases/db-prime/sessions/s1",
		Sql:     "SELECT 1",
	}, dcpRIW.WithNextRetryAttempt(1))
	if err != nil {
		t.Fatalf("DCP ExecuteStreamingSql failed: %v", err)
	}
	if _, err := dcpStream.Recv(); err != nil {
		t.Fatalf("DCP stream.Recv failed: %v", err)
	}
	if _, err := dcpStream.Recv(); err != io.EOF {
		t.Fatalf("expected io.EOF draining DCP stream, got %v", err)
	}
	if got := tr.dcp.TotalRPCLoad.Load(); got != 0 {
		t.Fatalf("expected TotalRPCLoad=0 after DCP stream EOF, got %d", got)
	}

	// Reset recorded unary headers before testing load-triggered scale-up priming.
	primeMu.Lock()
	primePrefixes = nil
	primeEncs = nil
	primeMu.Unlock()

	const primeSession = "projects/p/instances/i/databases/db-prime/sessions/s1"
	openBlockingStreams(t, streamCtx, tr, primeSession, 6)
	tr.UpdateDCPPrimeSession(primeSession)

	select {
	case added := <-scaledUp:
		if added <= 0 {
			t.Fatalf("expected positive scaled-up channel delta, got %d", added)
		}
	case <-time.After(5 * time.Second):
		t.Fatalf("timed out waiting for DCP scale-up (num=%d)", tr.dcp.Num())
	}

	if got := tr.dcp.Num(); got <= 2 {
		t.Fatalf("expected DCP to scale up above 2 channels, got %d", got)
	}

	primeMu.Lock()
	gotPrefixes := append([][]string(nil), primePrefixes...)
	gotEncs := append([][]string(nil), primeEncs...)
	primeMu.Unlock()
	if len(gotPrefixes) == 0 {
		t.Fatal("expected at least one DCP prime ExecuteSql RPC")
	}
	for i, p := range gotPrefixes {
		if len(p) != 1 || p[0] != "projects/p/instances/i/databases/db-prime" {
			t.Fatalf("prime RPC %d: resource prefix = %v, want [projects/p/instances/i/databases/db-prime]", i, p)
		}
	}
	for i, enc := range gotEncs {
		if len(enc) != 1 || enc[0] != "gzip" {
			t.Fatalf("prime RPC %d: compression header = %v, want [gzip]", i, enc)
		}
	}
}

func TestTransport_DCPContextCancelAfterStartupAllowsScaling(t *testing.T) {
	t.Parallel()
	mock, addr := startMockServer(t)

	unblock := make(chan struct{})
	mock.setHandler(func(req *sppb.ExecuteSqlRequest, stream sppb.Spanner_ExecuteStreamingSqlServer) error {
		if err := stream.Send(&sppb.PartialResultSet{}); err != nil {
			return err
		}
		select {
		case <-unblock:
			return nil
		case <-stream.Context().Done():
			return stream.Context().Err()
		}
	})

	startupCtx, cancelStartup := context.WithCancel(context.Background())
	scaledUp := make(chan int64, 4)
	tr, err := newTransportWithDCPHooks(startupCtx, ClientConfig{
		Endpoint:  addr,
		PlainText: true,
		DynamicChannelPoolConfig: DynamicChannelPoolConfig{
			DCPEnabled:          true,
			DCPInitialChannels:  2,
			DCPMinChannels:      2,
			DCPMaxChannels:      4,
			DCPMinRPCPerChannel: 1,
			DCPMaxRPCPerChannel: 2,
			DCPScaleUpCooldown:  time.Nanosecond,
		},
	}, newScaleUpHooks(scaledUp, time.Nanosecond))
	if err != nil {
		cancelStartup()
		t.Fatalf("newTransportWithDCPHooks failed: %v", err)
	}
	// Canceling the startup context immediately after construction must not kill DCP scaling.
	cancelStartup()

	streamCtx, cancelStreams := context.WithCancel(context.Background())
	defer func() {
		close(unblock)
		cancelStreams()
		_ = tr.Close()
	}()

	if tr.dcp.Ctx.Err() != nil {
		t.Fatalf("DCP background context was canceled by startup ctx: %v", tr.dcp.Ctx.Err())
	}

	const primeSession = "projects/p/instances/i/databases/db1/sessions/s1"
	openBlockingStreams(t, streamCtx, tr, primeSession, 6)
	tr.UpdateDCPPrimeSession(primeSession)

	select {
	case <-scaledUp:
	case <-time.After(5 * time.Second):
		t.Fatalf("timed out waiting for DCP scale-up after startup ctx cancellation (num=%d)", tr.dcp.Num())
	}
	if got := tr.dcp.Num(); got <= 2 {
		t.Fatalf("expected DCP to scale up above 2 channels, got %d", got)
	}
}

func TestTransport_UpdateDCPPrimeSessionContract(t *testing.T) {
	t.Parallel()
	mock, addr := startMockServer(t)

	const (
		db1Sess1 = "projects/p/instances/i/databases/db1/sessions/s1"
		db1Sess2 = "projects/p/instances/i/databases/db1/sessions/s2"
		db2Sess1 = "projects/p/instances/i/databases/db2/sessions/s1"
	)

	var (
		primeMu       sync.Mutex
		primeSessions []string
		primePrefixes []string
	)
	mock.setUnaryHandler(func(ctx context.Context, req *sppb.ExecuteSqlRequest) (*sppb.ResultSet, error) {
		var prefix string
		if md, ok := metadata.FromIncomingContext(ctx); ok {
			if vals := md.Get(transport.ResourcePrefixHeader); len(vals) > 0 {
				prefix = vals[0]
			}
		}
		primeMu.Lock()
		primeSessions = append(primeSessions, req.GetSession())
		primePrefixes = append(primePrefixes, prefix)
		primeMu.Unlock()
		return &sppb.ResultSet{}, nil
	})
	unblock := make(chan struct{})
	mock.setHandler(func(req *sppb.ExecuteSqlRequest, stream sppb.Spanner_ExecuteStreamingSqlServer) error {
		if err := stream.Send(&sppb.PartialResultSet{}); err != nil {
			return err
		}
		select {
		case <-unblock:
			return nil
		case <-stream.Context().Done():
			return stream.Context().Err()
		}
	})

	var (
		warnMu   sync.Mutex
		warnLogs []string
	)
	scaledUp := make(chan int64, 4)
	tr, err := newTransportWithDCPHooks(context.Background(), ClientConfig{
		Endpoint:  addr,
		PlainText: true,
		DynamicChannelPoolConfig: DynamicChannelPoolConfig{
			DCPEnabled:          true,
			DCPInitialChannels:  2,
			DCPMinChannels:      2,
			DCPMaxChannels:      4,
			DCPMinRPCPerChannel: 1,
			DCPMaxRPCPerChannel: 2,
			DCPScaleUpCooldown:  time.Nanosecond,
		},
		LogFunc: func(level, msg string) {
			warnMu.Lock()
			warnLogs = append(warnLogs, level+":"+msg)
			warnMu.Unlock()
		},
	}, newScaleUpHooks(scaledUp, time.Nanosecond))
	if err != nil {
		t.Fatalf("NewTransport failed: %v", err)
	}
	streamCtx, cancelStreams := context.WithCancel(context.Background())
	defer func() {
		close(unblock)
		cancelStreams()
		_ = tr.Close()
	}()

	// Malformed session names (including "") log a warning and are rejected.
	badInputs := []string{
		"",
		"/sessions/s1",
		"projects/p/instances/i/databases/d/sessions/",
		"projects//instances/i/databases/d/sessions/s1",
		"projects/p/instances/i/databases/d/sessions/s1/extra",
		"projects/p/instances/i/databases/d/sessions/s 1",
		"projects/p/instances/i/databases/d\t/sessions/s1",
		"other/p/instances/i/databases/d/sessions/s1",
	}
	for _, bad := range badInputs {
		tr.UpdateDCPPrimeSession(bad)
	}
	if got := tr.dcp.PrimeSessionID(); got != "" {
		t.Fatalf("expected empty PrimeSessionID after malformed inputs, got %q", got)
	}
	warnMu.Lock()
	gotWarnCount := len(warnLogs)
	firstWarn := ""
	if gotWarnCount > 0 {
		firstWarn = warnLogs[0]
	}
	warnMu.Unlock()
	if gotWarnCount != len(badInputs) || !strings.Contains(firstWarn, "WARN:") || !strings.Contains(firstWarn, "ignoring invalid DCP prime session") {
		t.Fatalf("malformed UpdateDCPPrimeSession warnings = %d (first=%q), want %d WARN logs", gotWarnCount, firstWarn, len(badInputs))
	}

	// Valid updates store the newest multiplexed session (same-DB rotation or another DB),
	// and duplicate calls with the same session ID are silent no-ops.
	tr.UpdateDCPPrimeSession(db1Sess1)
	tr.UpdateDCPPrimeSession(db1Sess1)
	if got := tr.dcp.PrimeSessionID(); got != db1Sess1 {
		t.Fatalf("PrimeSessionID = %q, want %q", got, db1Sess1)
	}
	tr.UpdateDCPPrimeSession(db1Sess2)
	if got := tr.dcp.PrimeSessionID(); got != db1Sess2 {
		t.Fatalf("PrimeSessionID after same-DB rotation = %q, want %q", got, db1Sess2)
	}
	tr.UpdateDCPPrimeSession(db2Sess1)
	if got := tr.dcp.PrimeSessionID(); got != db2Sess1 {
		t.Fatalf("PrimeSessionID after newer DB session = %q, want %q", got, db2Sess1)
	}
	warnMu.Lock()
	afterValidWarnCount := len(warnLogs)
	warnMu.Unlock()
	if afterValidWarnCount != len(badInputs) {
		t.Fatalf("valid UpdateDCPPrimeSession calls emitted %d unexpected warnings", afterValidWarnCount-len(badInputs))
	}

	// Scale-up primes with the latest recorded multiplexed session.
	openBlockingStreams(t, streamCtx, tr, db2Sess1, 6)

	select {
	case <-scaledUp:
	case <-time.After(5 * time.Second):
		t.Fatalf("timed out waiting for DCP scale-up (num=%d)", tr.dcp.Num())
	}

	primeMu.Lock()
	gotSessions := append([]string(nil), primeSessions...)
	gotPrefixes := append([]string(nil), primePrefixes...)
	primeMu.Unlock()
	if len(gotSessions) == 0 {
		t.Fatal("expected at least one prime RPC")
	}
	for i := range gotSessions {
		if gotSessions[i] != db2Sess1 {
			t.Fatalf("prime RPC %d session = %q, want %q", i, gotSessions[i], db2Sess1)
		}
		if gotPrefixes[i] != "projects/p/instances/i/databases/db2" {
			t.Fatalf("prime RPC %d prefix = %q, want projects/p/instances/i/databases/db2", i, gotPrefixes[i])
		}
	}
}

func TestTransport_DCPPrimeRetryMinimalBackoff(t *testing.T) {
	t.Parallel()
	mock, addr := startMockServer(t)

	const sess = "projects/p/instances/i/databases/db1/sessions/s1"
	var primeAttempts atomic.Int32
	mock.setUnaryHandler(func(ctx context.Context, req *sppb.ExecuteSqlRequest) (*sppb.ResultSet, error) {
		// Use codes.Internal (not retried by GAPIC vkit.Client.ExecuteSql) so retries go through
		// dcp.prime's PrimeRetryBaseDelay loop; 6 failures at default 100ms backoff would take 6.3s.
		if primeAttempts.Add(1) < 7 {
			return nil, status.Error(codes.Internal, "transient prime failure")
		}
		return &sppb.ResultSet{}, nil
	})
	unblock := make(chan struct{})
	mock.setHandler(func(req *sppb.ExecuteSqlRequest, stream sppb.Spanner_ExecuteStreamingSqlServer) error {
		if err := stream.Send(&sppb.PartialResultSet{}); err != nil {
			return err
		}
		select {
		case <-unblock:
			return nil
		case <-stream.Context().Done():
			return stream.Context().Err()
		}
	})

	scaledUp := make(chan int64, 4)
	tr, err := newTransportWithDCPHooks(context.Background(), ClientConfig{
		Endpoint:  addr,
		PlainText: true,
		DynamicChannelPoolConfig: DynamicChannelPoolConfig{
			DCPEnabled:          true,
			DCPInitialChannels:  1,
			DCPMinChannels:      1,
			DCPMaxChannels:      2,
			DCPMinRPCPerChannel: 1,
			DCPMaxRPCPerChannel: 2,
			DCPScaleUpCooldown:  time.Nanosecond,
			DCPPrimeMaxAttempts: 7,
			DCPPrimeTimeout:     time.Second,
		},
	}, newScaleUpHooks(scaledUp, time.Nanosecond))
	if err != nil {
		t.Fatalf("newTransportWithDCPHooks failed: %v", err)
	}
	streamCtx, cancelStreams := context.WithCancel(context.Background())
	defer func() {
		close(unblock)
		cancelStreams()
		_ = tr.Close()
	}()

	openBlockingStreams(t, streamCtx, tr, sess, 4)
	tr.UpdateDCPPrimeSession(sess)

	select {
	case <-scaledUp:
	case <-time.After(5 * time.Second):
		t.Fatalf("timed out waiting for DCP scale-up after retry (attempts=%d, num=%d)", primeAttempts.Load(), tr.dcp.Num())
	}

	if got := primeAttempts.Load(); got != 7 {
		t.Fatalf("prime attempts = %d, want 7", got)
	}
	if got := tr.dcp.Num(); got != 2 {
		t.Fatalf("pool channels after retry = %d, want 2", got)
	}
}

func TestTransport_DCPDisableBuiltInAuthBehavior(t *testing.T) {
	t.Parallel()
	_, addr := startMockServer(t)

	// PlainText + DisableBuiltInAuth + DCP stays enabled (used by host binding mock-server tests).
	plainTr, err := NewTransport(context.Background(), ClientConfig{
		Endpoint:           addr,
		PlainText:          true,
		DisableBuiltInAuth: true,
		DynamicChannelPoolConfig: DynamicChannelPoolConfig{
			DCPEnabled:         true,
			DCPInitialChannels: 2,
		},
	})
	if err != nil {
		t.Fatalf("NewTransport(PlainText+DisableBuiltInAuth+DCP) failed: %v", err)
	}
	defer plainTr.Close()
	if plainTr.dcp == nil || plainTr.dcp.IsFixed() {
		t.Fatal("expected non-fixed DCP on PlainText + DisableBuiltInAuth")
	}

	// Non-plaintext + DisableBuiltInAuth + DCP logs WARN and falls back to fixed pool.
	var (
		logMu sync.Mutex
		logs  []string
	)
	tlsCfg := ClientConfig{
		Endpoint:           addr,
		DisableBuiltInAuth: true,
		NumChannels:        2,
		DynamicChannelPoolConfig: DynamicChannelPoolConfig{
			DCPEnabled:         true,
			DCPInitialChannels: 3,
		},
		LogFunc: func(level, msg string) {
			logMu.Lock()
			logs = append(logs, level+":"+msg)
			logMu.Unlock()
		},
	}
	tlsSettings := resolveSettings(tlsCfg)
	if tlsSettings.useDCP || !tlsSettings.dcpDisabledForNoAuth {
		t.Fatalf("expected useDCP=false and dcpDisabledForNoAuth=true, got %+v", tlsSettings)
	}
	// Dial locally via extraOpts (while cfg.PlainText remains false) to verify warning + fixed fallback.
	tlsTr, err := newTransport(context.Background(), tlsCfg,
		append([]option.ClientOption{option.WithEndpoint("passthrough:///" + addr)}, transport.PlainTextClientOptions()...)...,
	)
	if err != nil {
		t.Fatalf("newTransport(TLS+DisableBuiltInAuth+DCP) failed: %v", err)
	}
	defer tlsTr.Close()
	if tlsTr.dcp == nil {
		t.Fatal("expected non-nil fixed pool")
	}
	if !tlsTr.dcp.IsFixed() || tlsTr.dcp.Num() != 2 {
		t.Fatalf("expected fixed pool of NumChannels=2, got fixed=%v num=%d", tlsTr.dcp.IsFixed(), tlsTr.dcp.Num())
	}
	logMu.Lock()
	gotLogs := strings.Join(logs, "\n")
	logMu.Unlock()
	if !strings.Contains(gotLogs, "WARN:") || !strings.Contains(gotLogs, "DisableBuiltInAuth") {
		t.Fatalf("expected WARN log mentioning DisableBuiltInAuth, got %q", gotLogs)
	}
}

func TestTransport_DCPRejectsConflictingConnectionPool(t *testing.T) {
	t.Parallel()
	_, addr := startMockServer(t)

	badDCP, err := newTransport(context.Background(), ClientConfig{
		Endpoint:  addr,
		PlainText: true,
		DynamicChannelPoolConfig: DynamicChannelPoolConfig{
			DCPEnabled:         true,
			DCPInitialChannels: 2,
		},
	}, option.WithGRPCConnectionPool(4))
	if err == nil {
		_ = badDCP.Close()
		t.Fatal("expected InvalidArgument when DCP is combined with WithGRPCConnectionPool(4)")
	}
	if status.Code(err) != codes.InvalidArgument {
		t.Fatalf("expected codes.InvalidArgument, got %v", err)
	}
}

func TestDCPChannelClient_ReleasesLoadOnPanic(t *testing.T) {
	t.Parallel()
	_, addr := startMockServer(t)

	tr, err := NewTransport(context.Background(), ClientConfig{
		Endpoint:  addr,
		PlainText: true,
		DynamicChannelPoolConfig: DynamicChannelPoolConfig{
			DCPEnabled:         true,
			DCPInitialChannels: 1,
			DCPMinChannels:     1,
			DCPMaxChannels:     2,
		},
	})
	if err != nil {
		t.Fatalf("NewTransport failed: %v", err)
	}
	defer tr.Close()

	entry := tr.dcp.GetEntries()[0]
	wrapped := &dcpChannelClient{entry: entry, delegate: &panickingChannelClient{}}

	func() {
		defer func() {
			if r := recover(); r == nil {
				t.Fatal("expected panic from ExecuteSql")
			}
		}()
		_, _ = wrapped.ExecuteSql(context.Background(), &sppb.ExecuteSqlRequest{})
	}()
	if entry.RPCLoad() != 0 || tr.dcp.TotalRPCLoad.Load() != 0 {
		t.Fatalf("unary load leaked after panic: entry=%d pool=%d", entry.RPCLoad(), tr.dcp.TotalRPCLoad.Load())
	}

	func() {
		defer func() {
			if r := recover(); r == nil {
				t.Fatal("expected panic from ExecuteStreamingSql")
			}
		}()
		_, _ = wrapped.ExecuteStreamingSql(context.Background(), &sppb.ExecuteSqlRequest{})
	}()
	if entry.RPCLoad() != 0 || tr.dcp.TotalRPCLoad.Load() != 0 {
		t.Fatalf("stream load leaked after panic: entry=%d pool=%d", entry.RPCLoad(), tr.dcp.TotalRPCLoad.Load())
	}
}

func TestDCPChannelClient_ExecuteStreamingSql_ErrorReleasesLoadAndAppliesPenalty(t *testing.T) {
	t.Parallel()
	_, addr := startMockServer(t)

	tr, err := NewTransport(context.Background(), ClientConfig{
		Endpoint:  addr,
		PlainText: true,
		DynamicChannelPoolConfig: DynamicChannelPoolConfig{
			DCPEnabled:          true,
			DCPInitialChannels:  1,
			DCPMinChannels:      1,
			DCPMaxChannels:      2,
			DCPErrorPenaltyStep: 3,
		},
	})
	if err != nil {
		t.Fatalf("NewTransport failed: %v", err)
	}
	defer tr.Close()

	entry := tr.dcp.GetEntries()[0]
	wrapped := &dcpChannelClient{
		entry:    entry,
		delegate: failingStreamChannelClient{err: status.Error(codes.Unavailable, "injected stream open error")},
	}

	_, err = wrapped.ExecuteStreamingSql(context.Background(), &sppb.ExecuteSqlRequest{Sql: "SELECT 1"})
	if w, g := codes.Unavailable, status.Code(err); w != g {
		t.Fatalf("status code mismatch:\n Want: %v\n  Got: %v", w, g)
	}
	if w, g := int32(0), entry.StreamLoad.Load(); w != g {
		t.Fatalf("entry StreamLoad leaked:\n Want: %d\n  Got: %d", w, g)
	}
	if w, g := int32(0), tr.dcp.TotalRPCLoad.Load(); w != g {
		t.Fatalf("pool TotalRPCLoad leaked:\n Want: %d\n  Got: %d", w, g)
	}
	if w, g := int64(0), entry.Refs.Load(); w != g {
		t.Fatalf("entry Refs leaked:\n Want: %d\n  Got: %d", w, g)
	}
	if entry.CurrentPenalty() <= 0 {
		t.Fatalf("expected error penalty > 0 after Unavailable, got %d", entry.CurrentPenalty())
	}
}

func TestNewTransport_DCPNonPlainText_PreConnectsChannels(t *testing.T) {
	t.Parallel()
	mock, addr := startMockServer(t)
	mock.setUnaryHandler(func(ctx context.Context, req *sppb.ExecuteSqlRequest) (*sppb.ResultSet, error) {
		return &sppb.ResultSet{}, nil
	})

	tr, err := newTransport(
		context.Background(),
		ClientConfig{
			Endpoint: addr,
			DynamicChannelPoolConfig: DynamicChannelPoolConfig{
				DCPEnabled:         true,
				DCPInitialChannels: 2,
				DCPMinChannels:     2,
				DCPMaxChannels:     4,
			},
		},
		append([]option.ClientOption{option.WithEndpoint("passthrough:///" + addr)}, transport.PlainTextClientOptions()...)...,
	)
	if err != nil {
		t.Fatalf("newTransport failed: %v", err)
	}
	defer tr.Close()

	if tr.dcp == nil || tr.dcp.IsFixed() {
		t.Fatal("expected active non-fixed DCP")
	}
	client, err := tr.PickClient(context.Background())
	if err != nil {
		t.Fatalf("PickClient failed: %v", err)
	}
	if _, err := client.ExecuteSql(context.Background(), &sppb.ExecuteSqlRequest{Sql: "SELECT 1"}); err != nil {
		t.Fatalf("ExecuteSql failed: %v", err)
	}
}

func TestTransport_PickClient_DCPNoEntriesError(t *testing.T) {
	t.Parallel()
	_, addr := startMockServer(t)

	tr, err := NewTransport(context.Background(), ClientConfig{
		Endpoint:  addr,
		PlainText: true,
		DynamicChannelPoolConfig: DynamicChannelPoolConfig{
			DCPEnabled:         true,
			DCPInitialChannels: 1,
			DCPMinChannels:     1,
			DCPMaxChannels:     2,
		},
	})
	if err != nil {
		t.Fatalf("NewTransport failed: %v", err)
	}
	defer tr.Close()

	// Mark the sole entry draining so Pick fails while Close still cleans it up.
	tr.dcp.GetEntries()[0].State.Store(transport.DCPStateDraining)

	if _, err := tr.PickClient(context.Background()); !errors.Is(err, transport.ErrDCPNoEntries) {
		t.Fatalf("PickClient error mismatch:\n Want: %v\n  Got: %v", transport.ErrDCPNoEntries, err)
	}
}

func TestDCPPrimeRPC_InvalidPrimeSession(t *testing.T) {
	t.Parallel()
	mock, addr := startMockServer(t)

	var rpcCalls atomic.Int32
	mock.setUnaryHandler(func(ctx context.Context, req *sppb.ExecuteSqlRequest) (*sppb.ResultSet, error) {
		rpcCalls.Add(1)
		return &sppb.ResultSet{}, nil
	})

	var (
		logMu sync.Mutex
		logs  []string
	)
	logged := make(chan struct{}, 1)
	tr, err := NewTransport(context.Background(), ClientConfig{
		Endpoint:  addr,
		PlainText: true,
		DynamicChannelPoolConfig: DynamicChannelPoolConfig{
			DCPEnabled:          true,
			DCPInitialChannels:  1,
			DCPMinChannels:      1,
			DCPMaxChannels:      2,
			DCPMinRPCPerChannel: 1,
			DCPMaxRPCPerChannel: 2,
			DCPPrimeMaxAttempts: 1,
			DCPScaleUpCooldown:  time.Millisecond,
		},
		LogFunc: func(level, msg string) {
			logMu.Lock()
			logs = append(logs, level+":"+msg)
			logMu.Unlock()
			select {
			case logged <- struct{}{}:
			default:
			}
		},
	})
	if err != nil {
		t.Fatalf("NewTransport failed: %v", err)
	}
	defer tr.Close()

	// Bypass UpdateDCPPrimeSession validation to verify PrimeRPC's defense-in-depth guard.
	tr.dcp.PrimeSession.Store("invalid-session")
	tr.dcp.TotalRPCLoad.Store(4)
	tr.dcp.GetEntries()[0].UnaryLoad.Store(4)
	tr.dcp.ScaleUp()
	tr.dcp.GetEntries()[0].UnaryLoad.Store(0)
	tr.dcp.TotalRPCLoad.Store(0)

	select {
	case <-logged:
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for PrimeRPC invalid-session log")
	}

	logMu.Lock()
	gotLogs := strings.Join(logs, "\n")
	logMu.Unlock()
	if !strings.Contains(gotLogs, "invalid DCP prime session") {
		t.Fatalf("expected invalid DCP prime session warning log, got %q", gotLogs)
	}
	if w, g := int32(0), rpcCalls.Load(); w != g {
		t.Fatalf("expected zero ExecuteSql RPCs on invalid prime session:\n Want: %d\n  Got: %d", w, g)
	}
}

func TestParseSessionDatabasePrefix(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		sessionID string
		want      string
		wantOK    bool
	}{
		{
			name:      "valid session name",
			sessionID: "projects/p/instances/i/databases/d/sessions/s",
			want:      "projects/p/instances/i/databases/d",
			wantOK:    true,
		},
		{
			name:      "instance literally named sessions",
			sessionID: "projects/p/instances/sessions/databases/d/sessions/s",
			want:      "projects/p/instances/sessions/databases/d",
			wantOK:    true,
		},
		{
			name:      "empty string",
			sessionID: "",
		},
		{
			name:      "missing prefix",
			sessionID: "/sessions/s1",
		},
		{
			name:      "wrong projects keyword",
			sessionID: "other/p/instances/i/databases/d/sessions/s1",
		},
		{
			name:      "empty project segment",
			sessionID: "projects//instances/i/databases/d/sessions/s1",
		},
		{
			name:      "wrong instances keyword",
			sessionID: "projects/p/wrong/i/databases/d/sessions/s",
		},
		{
			name:      "empty instance segment",
			sessionID: "projects/p/instances//databases/d/sessions/s",
		},
		{
			name:      "whitespace in instance segment",
			sessionID: "projects/p/instances/i /databases/d/sessions/s",
		},
		{
			name:      "wrong databases keyword",
			sessionID: "projects/p/instances/i/wrong/d/sessions/s",
		},
		{
			name:      "empty database segment",
			sessionID: "projects/p/instances/i/databases//sessions/s",
		},
		{
			name:      "newline in database segment",
			sessionID: "projects/p/instances/i/databases/d\n/sessions/s",
		},
		{
			name:      "tab in database segment",
			sessionID: "projects/p/instances/i/databases/d\t/sessions/s1",
		},
		{
			name:      "empty session segment",
			sessionID: "projects/p/instances/i/databases/d/sessions/",
		},
		{
			name:      "extra trailing segment",
			sessionID: "projects/p/instances/i/databases/d/sessions/s1/extra",
		},
		{
			name:      "whitespace in session segment",
			sessionID: "projects/p/instances/i/databases/d/sessions/s 1",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got, ok := parseSessionDatabasePrefix(tc.sessionID)
			if ok != tc.wantOK || got != tc.want {
				t.Fatalf("parseSessionDatabasePrefix(%q) mismatch:\n Want: (%q, %v)\n  Got: (%q, %v)", tc.sessionID, tc.want, tc.wantOK, got, ok)
			}
		})
	}
}

func TestTransport_Close_ZeroValue(t *testing.T) {
	t.Parallel()
	tr := &Transport{}
	if err := tr.Close(); err != nil {
		t.Fatalf("zero-value Close failed: %v", err)
	}
	if _, err := tr.PickClient(context.Background()); status.Code(err) != codes.FailedPrecondition {
		t.Fatalf("PickClient status mismatch:\n Want: %v\n  Got: %v", codes.FailedPrecondition, status.Code(err))
	}
}

func TestTransport_WithGRPCConnectionPoolSizingAndGzipSessionMD(t *testing.T) {
	t.Parallel()
	mock, addr := startMockServer(t)

	var (
		mu       sync.Mutex
		streamMD []metadata.MD
	)
	mock.setHandler(func(req *sppb.ExecuteSqlRequest, stream sppb.Spanner_ExecuteStreamingSqlServer) error {
		if md, ok := metadata.FromIncomingContext(stream.Context()); ok {
			mu.Lock()
			streamMD = append(streamMD, md.Copy())
			mu.Unlock()
		}
		return stream.Send(&sppb.PartialResultSet{})
	})

	if badTr, err := newTransport(context.Background(), ClientConfig{
		Endpoint:    addr,
		PlainText:   true,
		NumChannels: 4,
	}, option.WithGRPCConnectionPool(6)); err == nil {
		_ = badTr.Close()
		t.Fatal("expected InvalidArgument when NumChannels=4 conflicts with WithGRPCConnectionPool(6)")
	} else if status.Code(err) != codes.InvalidArgument {
		t.Fatalf("expected codes.InvalidArgument, got %v", err)
	}

	tr, err := newTransport(context.Background(), ClientConfig{
		Endpoint:    addr,
		PlainText:   true,
		Compression: "gzip",
	}, option.WithGRPCConnectionPool(6))
	if err != nil {
		t.Fatalf("newTransport failed: %v", err)
	}
	defer tr.Close()

	if got := tr.dcp.Num(); got != 6 {
		t.Fatalf("expected 6 fixed entries from WithGRPCConnectionPool(6), got %d", got)
	}
	seenCh := make(map[string]bool, 6)
	for i := 0; i < 6; i++ {
		c, err := tr.PickClient(context.Background())
		if err != nil {
			t.Fatalf("PickClient(%d) failed: %v", i, err)
		}
		riw, err := c.RequestIDHeaderInjector(context.Background())
		if err != nil {
			t.Fatalf("RequestIDHeaderInjector(%d) failed: %v", i, err)
		}
		key := requestid.LogicalRequestKeyFromCallOptions([]gax.CallOption{riw.WithNextRetryAttempt(1)})
		parts := strings.Split(key, ".")
		if len(parts) != 5 {
			t.Fatalf("unexpected logical request key %q", key)
		}
		seenCh[parts[3]] = true
	}
	if len(seenCh) != 6 {
		t.Fatalf("expected 6 distinct channel IDs, got %v", seenCh)
	}

	c, _ := tr.PickClient(context.Background())
	riw, _ := c.RequestIDHeaderInjector(context.Background())
	rpcCtx := metadata.NewOutgoingContext(context.Background(), tr.sessionMD)
	st, err := c.ExecuteStreamingSql(rpcCtx, &sppb.ExecuteSqlRequest{
		Session: "projects/p/instances/i/databases/d/sessions/s",
		Sql:     "SELECT 1",
	}, riw.WithNextRetryAttempt(1))
	if err != nil {
		t.Fatalf("ExecuteStreamingSql failed: %v", err)
	}
	if _, err := st.Recv(); err != nil {
		t.Fatalf("stream.Recv failed: %v", err)
	}
	if _, err := st.Recv(); err != io.EOF {
		t.Fatalf("drain stream.Recv = %v, want io.EOF", err)
	}
	mu.Lock()
	gotEnc := streamMD[0].Get(transport.RequestsCompressionHeader)
	mu.Unlock()
	if len(gotEnc) != 1 || gotEnc[0] != "gzip" {
		t.Fatalf("expected %s=[gzip] on wire, got %v", transport.RequestsCompressionHeader, gotEnc)
	}
}

func TestTransport_EmptyConnPoolFailsFast(t *testing.T) {
	t.Parallel()
	zp := &zeroConnPool{}
	tr, err := newTransport(context.Background(), ClientConfig{
		Endpoint:  "127.0.0.1:0",
		PlainText: true,
	}, gtransport.WithConnPool(zp))
	if err == nil {
		_ = tr.Close()
		t.Fatal("expected InvalidArgument for 0-connection ConnPool")
	}
	if status.Code(err) != codes.InvalidArgument {
		t.Fatalf("expected codes.InvalidArgument, got %v", err)
	}
	if got := zp.closes.Load(); got != 1 {
		t.Fatalf("expected empty ConnPool closed once, got %d", got)
	}
}

func TestTransport_PlainTextEndpointSchemes(t *testing.T) {
	t.Parallel()
	mock, addr := startMockServer(t)
	mock.setHandler(func(req *sppb.ExecuteSqlRequest, stream sppb.Spanner_ExecuteStreamingSqlServer) error {
		return stream.Send(&sppb.PartialResultSet{Last: true})
	})

	for _, tc := range []struct {
		name       string
		endpoint   string
		wantTarget string
	}{
		{"bare host:port", addr, "passthrough:///" + addr},
		{"http:// prefix stripped", "http://" + addr, "passthrough:///" + addr},
		{"https:// prefix stripped", "https://" + addr, "passthrough:///" + addr},
		{"passthrough:/// preserved", "passthrough:///" + addr, "passthrough:///" + addr},
		{"dns:/// preserved", "dns:///" + addr, "dns:///" + addr},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			tr, err := NewTransport(context.Background(), ClientConfig{
				Endpoint:    tc.endpoint,
				NumChannels: 1,
				PlainText:   true,
			})
			if err != nil {
				t.Fatalf("NewTransport(%q) failed: %v", tc.endpoint, err)
			}
			defer tr.Close()
			if got := tr.pool.Conn().Target(); got != tc.wantTarget {
				t.Fatalf("conn target = %q, want %q", got, tc.wantTarget)
			}
			c, err := tr.PickClient(context.Background())
			if err != nil {
				t.Fatalf("PickClient failed: %v", err)
			}
			riw, _ := c.RequestIDHeaderInjector(context.Background())
			st, err := c.ExecuteStreamingSql(context.Background(), &sppb.ExecuteSqlRequest{
				Session: "projects/p/instances/i/databases/d/sessions/s",
				Sql:     "SELECT 1",
			}, riw.WithNextRetryAttempt(1))
			if err != nil {
				t.Fatalf("ExecuteStreamingSql(%q) failed: %v", tc.endpoint, err)
			}
			if _, err := st.Recv(); err != nil {
				t.Fatalf("stream.Recv(%q) failed: %v", tc.endpoint, err)
			}
			if _, err := st.Recv(); err != io.EOF {
				t.Fatalf("drain stream.Recv(%q) = %v, want io.EOF", tc.endpoint, err)
			}
		})
	}
}

func TestTransport_CanceledContextFailsFast(t *testing.T) {
	t.Parallel()
	_, addr := startMockServer(t)

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	tr, err := NewTransport(ctx, ClientConfig{
		Endpoint:  addr,
		PlainText: true,
		DynamicChannelPoolConfig: DynamicChannelPoolConfig{
			DCPEnabled:         true,
			DCPInitialChannels: 2,
		},
	})
	if !errors.Is(err, context.Canceled) {
		if tr != nil {
			_ = tr.Close()
		}
		t.Fatalf("expected context.Canceled, got tr=%v err=%v", tr, err)
	}
}

func TestTransport_NilAndClosedLifecycle(t *testing.T) {
	t.Parallel()
	var nilTr *Transport
	if _, err := nilTr.PickClient(context.Background()); status.Code(err) != codes.FailedPrecondition {
		t.Fatalf("expected FailedPrecondition from nil Transport.PickClient, got %v", err)
	}
	if err := nilTr.Close(); err != nil {
		t.Fatalf("expected nil from nil Transport.Close, got %v", err)
	}
	nilTr.UpdateDCPPrimeSession("projects/p/instances/i/databases/d/sessions/s1")

	_, addr := startMockServer(t)
	tr, err := NewTransport(context.Background(), ClientConfig{Endpoint: addr, NumChannels: 1, PlainText: true})
	if err != nil {
		t.Fatalf("NewTransport failed: %v", err)
	}
	if err := tr.Close(); err != nil {
		t.Fatalf("Close failed: %v", err)
	}
	if err := tr.Close(); err != nil {
		t.Fatalf("idempotent Close failed: %v", err)
	}
	if _, err := tr.PickClient(context.Background()); status.Code(err) != codes.FailedPrecondition {
		t.Fatalf("expected FailedPrecondition after Close, got %v", err)
	}

	var nilResolver *dcpResolvingChannelClient
	if _, err := nilResolver.Acquire(context.Background()); !errors.Is(err, transport.ErrDCPNoEntries) {
		t.Fatalf("expected ErrDCPNoEntries from nil Acquire, got %v", err)
	}
	if _, err := nilResolver.RequestIDHeaderInjector(context.Background()); !errors.Is(err, transport.ErrDCPNoEntries) {
		t.Fatalf("expected ErrDCPNoEntries from nil RequestIDHeaderInjector, got %v", err)
	}
	if _, err := nilResolver.ExecuteSql(context.Background(), &sppb.ExecuteSqlRequest{}); !errors.Is(err, transport.ErrDCPNoEntries) {
		t.Fatalf("expected ErrDCPNoEntries from nil ExecuteSql, got %v", err)
	}
	if _, err := nilResolver.ExecuteStreamingSql(context.Background(), &sppb.ExecuteSqlRequest{}); !errors.Is(err, transport.ErrDCPNoEntries) {
		t.Fatalf("expected ErrDCPNoEntries from nil ExecuteStreamingSql, got %v", err)
	}
}

func TestTransport_CloseUnblocksActiveStream(t *testing.T) {
	t.Parallel()
	mock, addr := startMockServer(t)

	entered := make(chan struct{})
	mock.setHandler(func(req *sppb.ExecuteSqlRequest, stream sppb.Spanner_ExecuteStreamingSqlServer) error {
		if err := stream.Send(&sppb.PartialResultSet{}); err != nil {
			return err
		}
		close(entered)
		<-stream.Context().Done()
		return stream.Context().Err()
	})

	tr, err := NewTransport(context.Background(), ClientConfig{Endpoint: addr, NumChannels: 1, PlainText: true})
	if err != nil {
		t.Fatalf("NewTransport failed: %v", err)
	}
	c, err := tr.PickClient(context.Background())
	if err != nil {
		t.Fatalf("PickClient failed: %v", err)
	}
	riw, _ := c.RequestIDHeaderInjector(context.Background())
	st, err := c.ExecuteStreamingSql(context.Background(), &sppb.ExecuteSqlRequest{
		Session: "projects/p/instances/i/databases/d/sessions/s",
		Sql:     "SELECT 1",
	}, riw.WithNextRetryAttempt(1))
	if err != nil {
		t.Fatalf("ExecuteStreamingSql failed: %v", err)
	}
	if _, err := st.Recv(); err != nil {
		t.Fatalf("first Recv failed: %v", err)
	}
	<-entered

	if err := tr.Close(); err != nil {
		t.Fatalf("Close failed: %v", err)
	}
	if _, err := st.Recv(); err == nil {
		t.Fatal("expected non-nil error from Recv after Transport.Close")
	}
}

func TestTransport_ConcurrentPickUpdateAndClose(t *testing.T) {
	t.Parallel()
	_, addr := startMockServer(t)

	tr, err := NewTransport(context.Background(), ClientConfig{
		Endpoint:  addr,
		PlainText: true,
		DynamicChannelPoolConfig: DynamicChannelPoolConfig{
			DCPEnabled:         true,
			DCPInitialChannels: 2,
			DCPMinChannels:     2,
			DCPMaxChannels:     4,
		},
	})
	if err != nil {
		t.Fatalf("NewTransport failed: %v", err)
	}

	const workers = 8
	var (
		started sync.WaitGroup
		done    sync.WaitGroup
		stop    atomic.Bool
	)
	started.Add(workers)
	done.Add(workers)
	for g := 0; g < workers; g++ {
		go func(id int) {
			defer done.Done()
			sess := fmt.Sprintf("projects/p/instances/i/databases/db-%d/sessions/s-%d", id%2, id)
			tr.UpdateDCPPrimeSession(sess)
			_, _ = tr.PickClient(context.Background())
			started.Done()
			for i := 0; !stop.Load(); i++ {
				tr.UpdateDCPPrimeSession(sess)
				_, _ = tr.PickClient(context.Background())
				if i%32 == 0 {
					runtime.Gosched()
				}
			}
		}(g)
	}
	started.Wait()
	_ = tr.Close()
	stop.Store(true)
	done.Wait()

	// Post-Close UpdateDCPPrimeSession must not mutate PrimeSessionID.
	before := tr.dcp.PrimeSessionID()
	tr.UpdateDCPPrimeSession("projects/p/instances/i/databases/post-close/sessions/s99")
	if after := tr.dcp.PrimeSessionID(); after != before {
		t.Fatalf("UpdateDCPPrimeSession mutated PrimeSessionID after Close: %q -> %q", before, after)
	}
}

func TestApplyDCPPrimeMetadata_CanonicalAndNonMutating(t *testing.T) {
	t.Parallel()
	compressionBacking := []string{"gzip", "sentinel"}
	sessionMD := metadata.MD{
		transport.RequestsCompressionHeader: compressionBacking[:1],
		transport.ResourcePrefixHeader:      []string{"should-not-override"},
		transport.RouteToLeaderHeader:       []string{"false"},
		"X-Mixed-Session":                   []string{"v1"},
	}
	parentMD := metadata.MD{
		"X-Custom-Header":              []string{"keep-me"},
		"Google-Cloud-Resource-Prefix": []string{"old-db"},
		transport.RouteToLeaderHeader:  []string{"false"},
	}
	wantSessionMD := sessionMD.Copy()
	wantParentMD := parentMD.Copy()
	parentCtx := metadata.NewOutgoingContext(context.Background(), parentMD)

	outCtx := applyDCPPrimeMetadata(parentCtx, sessionMD, "projects/p/instances/i/databases/new-db")

	if !reflect.DeepEqual(parentMD, wantParentMD) {
		t.Fatalf("parentMD mutated on parentCtx path:\n got: %#v\nwant: %#v", parentMD, wantParentMD)
	}
	if !reflect.DeepEqual(sessionMD, wantSessionMD) {
		t.Fatalf("sessionMD mutated on parentCtx path:\n got: %#v\nwant: %#v", sessionMD, wantSessionMD)
	}
	outOutgoing, ok := metadata.FromOutgoingContext(outCtx)
	if !ok {
		t.Fatal("expected outgoing metadata on outCtx")
	}
	if got := outOutgoing.Get(transport.ResourcePrefixHeader); len(got) != 1 || got[0] != "projects/p/instances/i/databases/new-db" {
		t.Fatalf("expected new-db resource prefix, got %v", got)
	}
	if got := outOutgoing.Get(transport.RouteToLeaderHeader); len(got) != 1 || got[0] != "true" {
		t.Fatalf("expected route-to-leader=[true], got %v", got)
	}
	if got := outOutgoing.Get("x-custom-header"); len(got) != 1 || got[0] != "keep-me" {
		t.Fatalf("expected custom header to be preserved, got %v", got)
	}
	if got := outOutgoing.Get(transport.RequestsCompressionHeader); len(got) != 1 || got[0] != "gzip" {
		t.Fatalf("expected x-response-encoding=[gzip], got %v", got)
	}
	if got := outOutgoing.Get("x-mixed-session"); len(got) != 1 || got[0] != "v1" {
		t.Fatalf("expected canonicalized x-mixed-session=[v1], got %v", got)
	}

	// Verify fresh context (the production PrimeRPC path with no outgoing metadata)
	// neither mutates nor aliases sessionMD.
	freshCtx := applyDCPPrimeMetadata(context.Background(), sessionMD, "projects/p/instances/i/databases/fresh-db")
	if !reflect.DeepEqual(sessionMD, wantSessionMD) {
		t.Fatalf("sessionMD mutated on freshCtx path:\n got: %#v\nwant: %#v", sessionMD, wantSessionMD)
	}
	if compressionBacking[1] != "sentinel" {
		t.Fatalf("sessionMD spare backing capacity overwritten: %q", compressionBacking[1])
	}
	freshMD, ok := metadata.FromOutgoingContext(freshCtx)
	if !ok {
		t.Fatal("expected outgoing metadata on freshCtx")
	}
	if got := freshMD.Get(transport.ResourcePrefixHeader); len(got) != 1 || got[0] != "projects/p/instances/i/databases/fresh-db" {
		t.Fatalf("expected fresh-db resource prefix, got %v", got)
	}
	if got := freshMD.Get(transport.RouteToLeaderHeader); len(got) != 1 || got[0] != "true" {
		t.Fatalf("expected route-to-leader=[true] on freshCtx, got %v", got)
	}
	// Mutating sessionMD's backing slice after applyDCPPrimeMetadata must not affect freshCtx.
	compressionBacking[0] = "mutated"
	if got := metadata.ValueFromOutgoingContext(freshCtx, transport.RequestsCompressionHeader); len(got) != 1 || got[0] != "gzip" {
		t.Fatalf("freshCtx aliased sessionMD backing slice: got %v, want [gzip]", got)
	}
}

func TestNewLogger(t *testing.T) {
	t.Parallel()
	var gotLevel, gotMsg string
	l := newLogger(func(level, msg string) {
		gotLevel = level
		gotMsg = msg
	})
	l.Println("hello transport")
	if gotLevel != "WARN" || gotMsg != "hello transport" {
		t.Fatalf("unexpected log output: level=%q msg=%q", gotLevel, gotMsg)
	}

	nilLogger := newLogger(nil)
	nilLogger.Println("discarded")
}

// Non-parallel tests below exercise process environment variables (t.Setenv)
// or empty-Endpoint behavior affected by SPANNER_EMULATOR_HOST.

func TestTransport_DirectPathSuppressedOnPlainTextAndDisableBuiltInAuth(t *testing.T) {
	t.Setenv("GOOGLE_SPANNER_ENABLE_DIRECT_ACCESS", "true")
	t.Setenv("GOOGLE_SPANNER_ENABLE_GCP_FALLBACK", "true")

	for _, tc := range []struct {
		name string
		cfg  ClientConfig
	}{
		{"PlainText", ClientConfig{Endpoint: "plaintext.example.test:9010", PlainText: true, EnableDirectAccess: true}},
		{"DisableBuiltInAuth", ClientConfig{Endpoint: "spanner.example.test:443", DisableBuiltInAuth: true, EnableDirectAccess: true}},
	} {
		settings := resolveSettings(tc.cfg)
		if settings.directPathMode != transport.DirectPathDisabled || settings.isFallbackEnabled {
			t.Fatalf("%s: expected DirectPathDisabled and Fallback=false, got %+v", tc.name, settings)
		}
		poolOpts := (transport.PoolConfig{
			NumChannels:    1,
			DirectPathMode: settings.directPathMode,
		}).ClientOptions(buildClientOptions(tc.cfg, settings)...)
		for _, o := range poolOpts {
			desc := fmt.Sprintf("%T", o)
			if strings.Contains(desc, "DirectPath") || strings.Contains(desc, "HardBound") || strings.Contains(desc, "NonDefaultServiceAccount") {
				t.Fatalf("%s: unexpected DirectPath option %s injected despite DirectPathDisabled", tc.name, desc)
			}
		}
	}
}

func TestTransport_FallbackPoolChannelIDZeroAndSurvivesStartupCtxCancel(t *testing.T) {
	t.Setenv("GOOGLE_SPANNER_ENABLE_DIRECT_ACCESS", "true")
	t.Setenv("GOOGLE_SPANNER_ENABLE_GCP_FALLBACK", "true")

	mock, addr := startMockServer(t)
	var (
		mu    sync.Mutex
		reqID string
	)
	mock.setHandler(func(req *sppb.ExecuteSqlRequest, stream sppb.Spanner_ExecuteStreamingSqlServer) error {
		if md, ok := metadata.FromIncomingContext(stream.Context()); ok {
			if vals := md.Get(requestid.Header); len(vals) > 0 {
				mu.Lock()
				reqID = vals[0]
				mu.Unlock()
			}
		}
		return stream.Send(&sppb.PartialResultSet{})
	})

	// cfg.PlainText=false & DisableBuiltInAuth=false enables isFallbackEnabled, while extraOpts
	// override endpoint/credentials hermetically to point both primary and fallback pools at mock.
	startupCtx, cancelStartup := context.WithCancel(context.Background())
	tr, err := newTransport(startupCtx, ClientConfig{
		Endpoint:           addr,
		NumChannels:        2,
		EnableDirectAccess: true,
	}, append([]option.ClientOption{option.WithEndpoint("passthrough:///" + addr)}, transport.PlainTextClientOptions()...)...)
	if err != nil {
		cancelStartup()
		t.Fatalf("newTransport(FallbackPool) failed: %v", err)
	}
	// Canceling the startup context after construction leaves FallbackPool RPC routing healthy
	// (background rateCheck ticker survival is verified via MeterProvider spy in internal/transport).
	cancelStartup()
	defer tr.Close()

	fbPool, ok := tr.pool.(*transport.FallbackPool)
	if !ok {
		t.Fatalf("expected *transport.FallbackPool, got %T", tr.pool)
	}
	if tr.dcp != nil || tr.fallbackClient == nil {
		t.Fatalf("expected fallbackClient with dcp==nil, got dcp=%v fallbackClient=%v", tr.dcp != nil, tr.fallbackClient != nil)
	}
	if fbPool.PrimaryConn().Num() != 2 || fbPool.FallbackConn().Num() != 2 {
		t.Fatalf("unexpected fallback pool sizes: primary=%d fallback=%d", fbPool.PrimaryConn().Num(), fbPool.FallbackConn().Num())
	}

	c1, err := tr.PickClient(context.Background())
	if err != nil {
		t.Fatalf("PickClient 1 failed: %v", err)
	}
	c2, err := tr.PickClient(context.Background())
	if err != nil {
		t.Fatalf("PickClient 2 failed: %v", err)
	}
	if c1 != c2 {
		t.Fatal("expected FallbackPool PickClient to return the single shared fallbackClient")
	}

	riw, err := c1.RequestIDHeaderInjector(context.Background())
	if err != nil {
		t.Fatalf("RequestIDHeaderInjector failed: %v", err)
	}
	st, err := c1.ExecuteStreamingSql(context.Background(), &sppb.ExecuteSqlRequest{
		Session: "projects/p/instances/i/databases/d/sessions/s",
		Sql:     "SELECT 1",
	}, riw.WithNextRetryAttempt(1))
	if err != nil {
		t.Fatalf("ExecuteStreamingSql failed: %v", err)
	}
	if _, err := st.Recv(); err != nil {
		t.Fatalf("stream.Recv failed: %v", err)
	}
	if _, err := st.Recv(); err != io.EOF {
		t.Fatalf("drain stream.Recv = %v, want io.EOF", err)
	}
	mu.Lock()
	gotID := reqID
	mu.Unlock()
	parts := strings.Split(gotID, ".")
	if len(parts) != 6 || parts[3] != "0" {
		t.Fatalf("expected FallbackPool request ID with channelID=0, got %q", gotID)
	}
}

func TestTransport_EmulatorEnvHandling(t *testing.T) {
	_, addr := startMockServer(t)
	t.Setenv("SPANNER_EMULATOR_HOST", "emulator.internal:9010")

	tr, err := NewTransport(context.Background(), ClientConfig{
		Endpoint:  addr,
		PlainText: true,
		DynamicChannelPoolConfig: DynamicChannelPoolConfig{
			DCPEnabled: true,
		},
	})
	if err != nil {
		t.Fatalf("NewTransport failed: %v", err)
	}
	defer tr.Close()
	if tr.dcp == nil || tr.dcp.IsFixed() {
		t.Fatal("expected non-fixed DCP when explicit Endpoint differs from SPANNER_EMULATOR_HOST")
	}

	t.Setenv("SPANNER_EMULATOR_HOST", addr)
	emuTr, err := NewTransport(context.Background(), ClientConfig{
		Endpoint: "http://" + addr,
		DynamicChannelPoolConfig: DynamicChannelPoolConfig{
			DCPEnabled: true,
		},
	})
	if err != nil {
		t.Fatalf("NewTransport(emulator) failed: %v", err)
	}
	defer emuTr.Close()
	if emuTr.dcp == nil || !emuTr.dcp.IsFixed() {
		t.Fatal("expected DCP to be bypassed (fixed mode) when connecting to SPANNER_EMULATOR_HOST")
	}
}

func TestTransport_EmulatorBareHTTPPrefix(t *testing.T) {
	t.Setenv("SPANNER_EMULATOR_HOST", "http://")
	settings := resolveSettings(ClientConfig{Endpoint: "http://", DynamicChannelPoolConfig: DynamicChannelPoolConfig{DCPEnabled: true}})
	if !settings.isEmulator || !settings.isPlain || settings.useDCP {
		t.Fatalf("expected emulator resolution for matching bare http://, got %+v", settings)
	}
	opts := buildClientOptions(ClientConfig{Endpoint: "http://"}, settings)
	pool, err := gtransport.DialPool(context.Background(), transport.DefaultClientOptions(1, "", false, nil, nil, opts...)...)
	if err != nil {
		if !strings.Contains(err.Error(), "passthrough") {
			t.Fatalf("expected local passthrough failure, got %v", err)
		}
		return
	}
	defer pool.Close()
	if got := pool.Conn().Target(); got != "passthrough:///" {
		t.Fatalf("target = %q, want passthrough:///", got)
	}
}

func TestTransport_PlainTextEmptyEndpointFailsFast(t *testing.T) {
	t.Setenv("SPANNER_EMULATOR_HOST", "")
	tr, err := NewTransport(context.Background(), ClientConfig{
		Endpoint:  "",
		PlainText: true,
	})
	if err == nil {
		_ = tr.Close()
		t.Fatal("expected InvalidArgument when PlainText=true and Endpoint is empty without SPANNER_EMULATOR_HOST")
	}
	if status.Code(err) != codes.InvalidArgument {
		t.Fatalf("expected codes.InvalidArgument, got %v", err)
	}
}

func TestTransport_TLSWithDisableBuiltInAuthPreservesUniverseDomain(t *testing.T) {
	t.Setenv("SPANNER_EMULATOR_HOST", "")
	cfg := ClientConfig{
		UniverseDomain:     "example-universe.test",
		DisableBuiltInAuth: true,
	}
	settings := resolveSettings(cfg)
	opts := buildClientOptions(cfg, settings)
	pool, err := gtransport.DialPool(context.Background(), (transport.PoolConfig{
		NumChannels:    1,
		DirectPathMode: transport.DirectPathDisabled,
	}).ClientOptions(opts...)...)
	if err != nil {
		t.Fatalf("DialPool(DisableBuiltInAuth+UniverseDomain) failed: %v", err)
	}
	defer pool.Close()
	if got := pool.Conn().Target(); !strings.Contains(got, "spanner.example-universe.test") {
		t.Fatalf("expected target to expand UniverseDomain spanner.example-universe.test, got %q", got)
	}
}

func BenchmarkTransportPickClientStatic(b *testing.B) {
	_, addr := startMockServer(b)

	for _, numCh := range []int{1, 4} {
		b.Run("channels="+strconv.Itoa(numCh), func(b *testing.B) {
			tr, err := NewTransport(context.Background(), ClientConfig{
				Endpoint:    addr,
				NumChannels: numCh,
				PlainText:   true,
			})
			if err != nil {
				b.Fatalf("NewTransport failed: %v", err)
			}
			defer tr.Close()

			ctx := context.Background()
			b.ReportAllocs()
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				c, err := tr.PickClient(ctx)
				if err != nil || c == nil {
					b.Fatalf("PickClient failed: %v", err)
				}
			}
		})
	}
}

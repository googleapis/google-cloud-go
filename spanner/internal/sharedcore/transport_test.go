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
	"net"
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

func TestDCPSelectionStrategyABI(t *testing.T) {
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
	t.Setenv("SPANNER_EMULATOR_HOST", "")
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

	t.Run("multi-channel fixed pool round-robins independently of nthRequest", func(t *testing.T) {
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
	})

	t.Run("single-channel fixed pool", func(t *testing.T) {
		tr1, err := NewTransport(context.Background(), ClientConfig{
			Endpoint:    addr,
			NumChannels: 1,
			PlainText:   true,
		})
		if err != nil {
			t.Fatalf("NewTransport(NumChannels=1) failed: %v", err)
		}
		defer tr1.Close()
		c1, err := tr1.PickClient(context.Background())
		if err != nil || c1 == nil {
			t.Fatalf("PickClient on NumChannels=1 failed: %v", err)
		}
	})
}

func TestTransport_DynamicChannelPool(t *testing.T) {
	t.Setenv("SPANNER_EMULATOR_HOST", "")

	t.Run("scale-up via real blocking streams and prime headers", func(t *testing.T) {
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
		var activeStreams atomic.Int32
		mock.setHandler(func(req *sppb.ExecuteSqlRequest, stream sppb.Spanner_ExecuteStreamingSqlServer) error {
			if req.GetSql() == "BLOCK" {
				activeStreams.Add(1)
				defer activeStreams.Add(-1)
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

		tr, err := NewTransport(context.Background(), ClientConfig{
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
				DCPScaleUpCooldown:  time.Millisecond,
			},
		})
		if err != nil {
			t.Fatalf("NewTransport with DCP failed: %v", err)
		}
		defer func() {
			close(unblock)
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

		// Reset recorded unary headers before testing real load-triggered scale-up priming.
		primeMu.Lock()
		primePrefixes = nil
		primeEncs = nil
		primeMu.Unlock()

		// Open 6 concurrent blocking streams (> 2 channels * DCPMaxRPCPerChannel=2) to trigger real scale-up.
		streamCtx, cancelStreams := context.WithCancel(context.Background())
		defer cancelStreams()
		for i := 0; i < 6; i++ {
			c, err := tr.PickClient(streamCtx)
			if err != nil {
				t.Fatalf("PickClient(%d) failed: %v", i, err)
			}
			riw, err := c.RequestIDHeaderInjector(streamCtx)
			if err != nil {
				t.Fatalf("RequestIDHeaderInjector(%d) failed: %v", i, err)
			}
			st, err := c.ExecuteStreamingSql(streamCtx, &sppb.ExecuteSqlRequest{
				Session: "projects/p/instances/i/databases/db-prime/sessions/s1",
				Sql:     "BLOCK",
			}, riw.WithNextRetryAttempt(1))
			if err != nil {
				t.Fatalf("blocking ExecuteStreamingSql(%d) failed: %v", i, err)
			}
			if _, err := st.Recv(); err != nil {
				t.Fatalf("blocking stream.Recv(%d) failed: %v", i, err)
			}
		}

		const primeSession = "projects/p/instances/i/databases/db-prime/sessions/s1"
		tr.UpdateDCPPrimeSession(primeSession)

		deadline := time.Now().Add(3 * time.Second)
		for time.Now().Before(deadline) && tr.dcp.Num() <= 2 {
			time.Sleep(10 * time.Millisecond)
		}
		if got := tr.dcp.Num(); got <= 2 {
			t.Fatalf("expected DCP to scale up above 2 channels under real blocking stream load, got %d (activeStreams=%d)", got, activeStreams.Load())
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
	})

	t.Run("UpdateDCPPrimeSession strict validation, rotation, and dead-DB failover (M3)", func(t *testing.T) {
		mock, addr := startMockServer(t)
		const (
			deadDB1Sess1 = "projects/p/instances/i/databases/dead-db1/sessions/s1"
			deadDB1Sess2 = "projects/p/instances/i/databases/dead-db1/sessions/s2"
			liveDB2Sess1 = "projects/p/instances/i/databases/live-db2/sessions/s1"
		)

		var deadDB1Primed atomic.Int32
		mock.setUnaryHandler(func(ctx context.Context, req *sppb.ExecuteSqlRequest) (*sppb.ResultSet, error) {
			if strings.Contains(req.GetSession(), "/databases/dead-db1/") {
				deadDB1Primed.Add(1)
				return nil, status.Error(codes.NotFound, "database deleted")
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

		tr, err := NewTransport(context.Background(), ClientConfig{
			Endpoint:  addr,
			PlainText: true,
			DynamicChannelPoolConfig: DynamicChannelPoolConfig{
				DCPEnabled:          true,
				DCPInitialChannels:  2,
				DCPMinChannels:      2,
				DCPMaxChannels:      4,
				DCPMinRPCPerChannel: 1,
				DCPMaxRPCPerChannel: 2,
				DCPScaleUpCooldown:  time.Millisecond,
				DCPPrimeMaxAttempts: 1,
				DCPPrimeTimeout:     time.Second,
			},
		})
		if err != nil {
			t.Fatalf("NewTransport failed: %v", err)
		}
		defer func() {
			close(unblock)
			_ = tr.Close()
		}()

		// Strict 8-segment validation rejects malformed session names.
		for _, bad := range []string{
			"",
			"/sessions/s1",
			"projects/p/instances/i/databases/d/sessions/",
			"projects//instances/i/databases/d/sessions/s1",
			"projects/p/instances/i/databases/d/sessions/s1/extra",
			"projects/p/instances/i/databases/d/sessions/s 1",
			"projects/p/instances/i/databases/d\t/sessions/s1",
			"other/p/instances/i/databases/d/sessions/s1",
		} {
			tr.UpdateDCPPrimeSession(bad)
		}
		if got := tr.dcp.PrimeSessionID(); got != "" {
			t.Fatalf("expected empty PrimeSessionID after malformed inputs, got %q", got)
		}

		// Pin healthy DB1, verify DB2 does not thrash pin, verify same-DB rotation updates pin,
		// and verify delayed stale mark on old session does not invalidate the rotated session.
		tr.UpdateDCPPrimeSession(deadDB1Sess1)
		tr.UpdateDCPPrimeSession(liveDB2Sess1)
		if got := tr.dcp.PrimeSessionID(); got != deadDB1Sess1 {
			t.Fatalf("cross-DB update overwrote healthy pin: got %q, want %q", got, deadDB1Sess1)
		}
		tr.UpdateDCPPrimeSession(deadDB1Sess2)
		if got := tr.dcp.PrimeSessionID(); got != deadDB1Sess2 {
			t.Fatalf("same-DB rotation did not update pin: got %q, want %q", got, deadDB1Sess2)
		}
		tr.markPrimeSessionStale(deadDB1Sess1) // delayed failure for superseded session
		tr.UpdateDCPPrimeSession(liveDB2Sess1)
		if got := tr.dcp.PrimeSessionID(); got != deadDB1Sess2 {
			t.Fatalf("delayed stale mark for old session invalidated rotated session: got %q, want %q", got, deadDB1Sess2)
		}

		// Open 6 blocking streams to drive scale-up so PrimeRPC hits deadDB1Sess2 -> NotFound -> marks stale.
		streamCtx, cancelStreams := context.WithCancel(context.Background())
		defer cancelStreams()
		for i := 0; i < 6; i++ {
			c, err := tr.PickClient(streamCtx)
			if err != nil {
				t.Fatalf("PickClient(%d) failed: %v", i, err)
			}
			riw, err := c.RequestIDHeaderInjector(streamCtx)
			if err != nil {
				t.Fatalf("RequestIDHeaderInjector(%d) failed: %v", i, err)
			}
			st, err := c.ExecuteStreamingSql(streamCtx, &sppb.ExecuteSqlRequest{Session: deadDB1Sess2, Sql: "BLOCK"}, riw.WithNextRetryAttempt(1))
			if err != nil {
				t.Fatalf("ExecuteStreamingSql(%d) failed: %v", i, err)
			}
			if _, err := st.Recv(); err != nil {
				t.Fatalf("stream.Recv(%d) failed: %v", i, err)
			}
		}

		// Wait until deadDB1Sess2 has been marked stale by PrimeRPC.
		deadline := time.Now().Add(3 * time.Second)
		for time.Now().Before(deadline) {
			if st := tr.primeState.Load(); st != nil && st.stale {
				break
			}
			time.Sleep(10 * time.Millisecond)
		}
		if st := tr.primeState.Load(); st == nil || !st.stale {
			t.Fatalf("expected dead DB1 prime session to be marked stale (prime attempts=%d)", deadDB1Primed.Load())
		}

		// Dead DB1 still receiving queries must NOT un-stale itself, while live DB2 takes over the pin once.
		tr.UpdateDCPPrimeSession(deadDB1Sess2)
		if st := tr.primeState.Load(); st == nil || !st.stale {
			t.Fatal("re-sending stale dead DB1 session unexpectedly cleared stale flag")
		}
		tr.UpdateDCPPrimeSession(liveDB2Sess1)
		if got := tr.dcp.PrimeSessionID(); got != liveDB2Sess1 {
			t.Fatalf("expected live DB2 to take over stale pin: got %q, want %q", got, liveDB2Sess1)
		}
		// Subsequent dead DB1 queries must not overwrite the healthy DB2 pin.
		tr.UpdateDCPPrimeSession(deadDB1Sess2)
		if got := tr.dcp.PrimeSessionID(); got != liveDB2Sess1 {
			t.Fatalf("dead DB1 overwrote healthy DB2 pin: got %q, want %q", got, liveDB2Sess1)
		}

		// Pool now scales up cleanly using live DB2.
		deadline = time.Now().Add(3 * time.Second)
		for time.Now().Before(deadline) && tr.dcp.Num() <= 2 {
			time.Sleep(10 * time.Millisecond)
		}
		if got := tr.dcp.Num(); got <= 2 {
			t.Fatalf("expected DCP to scale up after failover to live DB2, got %d channels", got)
		}
	})

	t.Run("Unauthenticated prime error does not mark prime session stale (M3)", func(t *testing.T) {
		mock, addr := startMockServer(t)
		const sess1 = "projects/p/instances/i/databases/db1/sessions/s1"
		const sess2 = "projects/p/instances/i/databases/db2/sessions/s1"

		var primeAttempted atomic.Int32
		mock.setUnaryHandler(func(ctx context.Context, req *sppb.ExecuteSqlRequest) (*sppb.ResultSet, error) {
			primeAttempted.Add(1)
			return nil, status.Error(codes.Unauthenticated, "token expired")
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

		tr, err := NewTransport(context.Background(), ClientConfig{
			Endpoint:  addr,
			PlainText: true,
			DynamicChannelPoolConfig: DynamicChannelPoolConfig{
				DCPEnabled:          true,
				DCPInitialChannels:  2,
				DCPMinChannels:      2,
				DCPMaxChannels:      4,
				DCPMinRPCPerChannel: 1,
				DCPMaxRPCPerChannel: 2,
				DCPScaleUpCooldown:  time.Millisecond,
				DCPPrimeMaxAttempts: 2,
				DCPPrimeTimeout:     time.Second,
			},
		})
		if err != nil {
			t.Fatalf("NewTransport failed: %v", err)
		}
		defer func() {
			close(unblock)
			_ = tr.Close()
		}()

		streamCtx, cancel := context.WithCancel(context.Background())
		defer cancel()
		for i := 0; i < 6; i++ {
			c, _ := tr.PickClient(streamCtx)
			riw, _ := c.RequestIDHeaderInjector(streamCtx)
			st, err := c.ExecuteStreamingSql(streamCtx, &sppb.ExecuteSqlRequest{Session: sess1, Sql: "BLOCK"}, riw.WithNextRetryAttempt(1))
			if err != nil {
				t.Fatalf("ExecuteStreamingSql failed: %v", err)
			}
			if _, err := st.Recv(); err != nil {
				t.Fatalf("Recv failed: %v", err)
			}
		}
		tr.UpdateDCPPrimeSession(sess1)
		deadline := time.Now().Add(3 * time.Second)
		for time.Now().Before(deadline) && primeAttempted.Load() < 2 {
			time.Sleep(10 * time.Millisecond)
		}
		if primeAttempted.Load() < 2 {
			t.Fatalf("expected >= 2 PrimeRPC attempts (ensuring attempt 1 completed), got %d", primeAttempted.Load())
		}
		if st := tr.primeState.Load(); st == nil || st.stale {
			t.Fatalf("codes.Unauthenticated must not mark prime session stale, got %+v", st)
		}
		tr.UpdateDCPPrimeSession(sess2)
		if got := tr.dcp.PrimeSessionID(); got != sess1 {
			t.Fatalf("expected sess1 pin preserved after Unauthenticated, got %q", got)
		}
	})

	t.Run("DisableBuiltInAuth on TLS disables DCP with WARN log while PlainText keeps DCP (M2)", func(t *testing.T) {
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
		if tlsTr.dcp == nil || !tlsTr.dcp.IsFixed() || tlsTr.dcp.Num() != 2 {
			t.Fatalf("expected fixed pool of NumChannels=2, got dcp=%v fixed=%v num=%d", tlsTr.dcp != nil, tlsTr.dcp != nil && tlsTr.dcp.IsFixed(), tlsTr.dcp.Num())
		}
		logMu.Lock()
		gotLogs := strings.Join(logs, "\n")
		logMu.Unlock()
		if !strings.Contains(gotLogs, "WARN:") || !strings.Contains(gotLogs, "DisableBuiltInAuth") {
			t.Fatalf("expected WARN log mentioning DisableBuiltInAuth, got %q", gotLogs)
		}
	})

	t.Run("DCP rejects conflicting WithGRPCConnectionPool", func(t *testing.T) {
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
	})

	t.Run("dcpChannelClient releases load on delegate panic (M6)", func(t *testing.T) {
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
	})
}

func TestTransport_DirectPathPoolSizingAndFallback(t *testing.T) {
	t.Setenv("SPANNER_EMULATOR_HOST", "")

	t.Run("M1: GOOGLE_SPANNER_ENABLE_DIRECT_ACCESS=true suppressed on PlainText and DisableBuiltInAuth", func(t *testing.T) {
		t.Setenv("GOOGLE_SPANNER_ENABLE_DIRECT_ACCESS", "true")
		t.Setenv("GOOGLE_SPANNER_ENABLE_GCP_FALLBACK", "true")

		for _, tc := range []struct {
			name string
			cfg  ClientConfig
		}{
			{"PlainText", ClientConfig{Endpoint: "localhost:9010", PlainText: true, EnableDirectAccess: true}},
			{"DisableBuiltInAuth", ClientConfig{Endpoint: "spanner.example.test:443", DisableBuiltInAuth: true, EnableDirectAccess: true}},
		} {
			settings := resolveSettings(tc.cfg)
			if settings.isDirectPathEnabled || settings.isFallbackEnabled {
				t.Fatalf("%s: expected DirectPath and Fallback disabled, got %+v", tc.name, settings)
			}
			poolOpts := (transport.PoolConfig{
				NumChannels:        1,
				EnableDirectAccess: settings.isDirectPathEnabled,
				DisableDirectPath:  !settings.isDirectPathEnabled,
			}).ClientOptions(buildClientOptions(tc.cfg, settings)...)
			for _, o := range poolOpts {
				desc := fmt.Sprintf("%T", o)
				if strings.Contains(desc, "DirectPath") || strings.Contains(desc, "HardBound") || strings.Contains(desc, "NonDefaultServiceAccount") {
					t.Fatalf("%s: unexpected DirectPath option %s injected despite GOOGLE_SPANNER_ENABLE_DIRECT_ACCESS=true", tc.name, desc)
				}
			}
		}
	})

	t.Run("hermetic FallbackPool uses single channelID=0 client (L1)", func(t *testing.T) {
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

		t.Setenv("GOOGLE_SPANNER_ENABLE_DIRECT_ACCESS", "true")
		t.Setenv("GOOGLE_SPANNER_ENABLE_GCP_FALLBACK", "true")

		// cfg.PlainText=false & DisableBuiltInAuth=false enables isFallbackEnabled, while extraOpts
		// override endpoint/credentials hermetically to point both primary and fallback pools at mock.
		tr, err := newTransport(context.Background(), ClientConfig{
			Endpoint:           addr,
			NumChannels:        2,
			EnableDirectAccess: true,
		}, append([]option.ClientOption{option.WithEndpoint("passthrough:///" + addr)}, transport.PlainTextClientOptions()...)...)
		if err != nil {
			t.Fatalf("newTransport(FallbackPool) failed: %v", err)
		}
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
		mu.Lock()
		gotID := reqID
		mu.Unlock()
		parts := strings.Split(gotID, ".")
		if len(parts) != 6 || parts[3] != "0" {
			t.Fatalf("expected FallbackPool request ID with channelID=0, got %q", gotID)
		}
	})

	t.Run("WithGRPCConnectionPool sizing, mismatch validation, and gzip sessionMD", func(t *testing.T) {
		t.Setenv("GOOGLE_SPANNER_ENABLE_GCP_FALLBACK", "")
		t.Setenv("GOOGLE_SPANNER_ENABLE_DIRECT_ACCESS", "invalid-bool")
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
			parts := strings.Split(riw.LogicalRequestKey(), ".")
			if len(parts) != 5 {
				t.Fatalf("unexpected logical request key %q", riw.LogicalRequestKey())
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
		mu.Lock()
		gotEnc := streamMD[0].Get(transport.RequestsCompressionHeader)
		mu.Unlock()
		if len(gotEnc) != 1 || gotEnc[0] != "gzip" {
			t.Fatalf("expected %s=[gzip] on wire, got %v", transport.RequestsCompressionHeader, gotEnc)
		}
	})

	t.Run("empty custom ConnPool fails fast and closes pool (L1 guard)", func(t *testing.T) {
		zp := &zeroConnPool{}
		tr, err := newTransport(context.Background(), ClientConfig{
			Endpoint:  "localhost:9010",
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
	})
}

func TestTransport_EndpointsAndEmulator(t *testing.T) {
	t.Run("SPANNER_EMULATOR_HOST does not downgrade explicit remote endpoint", func(t *testing.T) {
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
	})

	t.Run("both SPANNER_EMULATOR_HOST and Endpoint set to bare http:// resolves as emulator", func(t *testing.T) {
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
	})

	t.Run("M5: PlainText with empty Endpoint and no emulator fails fast with InvalidArgument", func(t *testing.T) {
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
	})

	t.Run("M5: PlainText preserves dns:/// and passthrough:/// schemes and strips http://", func(t *testing.T) {
		t.Setenv("SPANNER_EMULATOR_HOST", "")
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
			{"passthrough:/// preserved", "passthrough:///" + addr, "passthrough:///" + addr},
			{"dns:/// preserved", "dns:///" + addr, "dns:///" + addr},
		} {
			t.Run(tc.name, func(t *testing.T) {
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
			})
		}
	})

	t.Run("TLS with DisableBuiltInAuth preserves WithUniverseDomain", func(t *testing.T) {
		t.Setenv("SPANNER_EMULATOR_HOST", "")
		cfg := ClientConfig{
			UniverseDomain:     "example-universe.test",
			DisableBuiltInAuth: true,
		}
		settings := resolveSettings(cfg)
		opts := buildClientOptions(cfg, settings)
		pool, err := gtransport.DialPool(context.Background(), (transport.PoolConfig{
			NumChannels:       1,
			DisableDirectPath: true,
		}).ClientOptions(opts...)...)
		if err != nil {
			t.Fatalf("DialPool(DisableBuiltInAuth+UniverseDomain) failed: %v", err)
		}
		defer pool.Close()
		if got := pool.Conn().Target(); !strings.Contains(got, "spanner.example-universe.test") {
			t.Fatalf("expected target to expand UniverseDomain spanner.example-universe.test, got %q", got)
		}
	})
}

func TestTransport_LifecycleAndConcurrency(t *testing.T) {
	t.Setenv("SPANNER_EMULATOR_HOST", "")

	t.Run("M6: canceled context fails fast in NewTransport", func(t *testing.T) {
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
	})

	t.Run("nil and closed Transport and nil dcpResolvingChannelClient", func(t *testing.T) {
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
	})

	t.Run("Close during active stream unblocks Recv cleanly", func(t *testing.T) {
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
	})

	t.Run("concurrent PickClient, UpdateDCPPrimeSession, and Close under -race", func(t *testing.T) {
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

		var wg sync.WaitGroup
		for g := 0; g < 8; g++ {
			wg.Add(1)
			go func(id int) {
				defer wg.Done()
				sess := fmt.Sprintf("projects/p/instances/i/databases/db-%d/sessions/s-%d", id%2, id)
				for i := 0; i < 50; i++ {
					tr.UpdateDCPPrimeSession(sess)
					_, _ = tr.PickClient(context.Background())
				}
			}(g)
		}
		time.Sleep(5 * time.Millisecond)
		_ = tr.Close()
		wg.Wait()

		// Post-Close UpdateDCPPrimeSession must not mutate primeState.
		before := tr.primeState.Load()
		tr.UpdateDCPPrimeSession("projects/p/instances/i/databases/post-close/sessions/s99")
		if after := tr.primeState.Load(); after != before {
			t.Fatalf("UpdateDCPPrimeSession mutated primeState after Close: %+v -> %+v", before, after)
		}
	})
}

func TestApplyDCPPrimeMetadata_CanonicalAndNonMutating(t *testing.T) {
	sessionMD := metadata.MD{
		"X-Response-Encoding":          []string{"gzip"},
		"Google-Cloud-Resource-Prefix": []string{"should-not-override"},
	}
	parentMD := metadata.MD{
		"X-Custom-Header":              []string{"keep-me"},
		"Google-Cloud-Resource-Prefix": []string{"old-db"},
	}
	parentCtx := metadata.NewOutgoingContext(context.Background(), parentMD)

	outCtx := applyDCPPrimeMetadata(parentCtx, sessionMD, "projects/p/instances/i/databases/new-db")

	if got := parentMD["Google-Cloud-Resource-Prefix"]; len(got) != 1 || got[0] != "old-db" {
		t.Fatalf("parentMD was mutated in place: %v", parentMD)
	}
	outOutgoing, ok := metadata.FromOutgoingContext(outCtx)
	if !ok {
		t.Fatal("expected outgoing metadata on outCtx")
	}
	if got := outOutgoing.Get(transport.ResourcePrefixHeader); len(got) != 1 || got[0] != "projects/p/instances/i/databases/new-db" {
		t.Fatalf("expected new-db resource prefix, got %v", got)
	}
	if got := outOutgoing.Get("x-custom-header"); len(got) != 1 || got[0] != "keep-me" {
		t.Fatalf("expected custom header to be preserved, got %v", got)
	}
	if got := outOutgoing.Get(transport.RequestsCompressionHeader); len(got) != 1 || got[0] != "gzip" {
		t.Fatalf("expected x-response-encoding=[gzip], got %v", got)
	}
}

func TestNewLogger(t *testing.T) {
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

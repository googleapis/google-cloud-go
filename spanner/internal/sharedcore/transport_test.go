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
	"io"
	"net"
	"reflect"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	sppb "cloud.google.com/go/spanner/apiv1/spannerpb"
	"cloud.google.com/go/spanner/internal/requestid"
	"cloud.google.com/go/spanner/internal/transport"
	"google.golang.org/api/option"
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

func startMockServer(t *testing.T) (*mockSpannerServer, string, func()) {
	t.Helper()
	lis, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("failed to listen: %v", err)
	}
	srv := grpc.NewServer()
	mock := &mockSpannerServer{}
	sppb.RegisterSpannerServer(srv, mock)
	go func() {
		_ = srv.Serve(lis)
	}()
	return mock, lis.Addr().String(), func() {
		srv.Stop()
		_ = lis.Close()
	}
}

func TestTransport_MultiChannelRoundRobin(t *testing.T) {
	t.Setenv("SPANNER_EMULATOR_HOST", "")
	mock, addr, cleanup := startMockServer(t)
	defer cleanup()

	var (
		mu     sync.Mutex
		reqIDs []string
	)
	mock.handler = func(req *sppb.ExecuteSqlRequest, stream sppb.Spanner_ExecuteStreamingSqlServer) error {
		if md, ok := metadata.FromIncomingContext(stream.Context()); ok {
			if vals := md.Get(requestid.Header); len(vals) > 0 {
				mu.Lock()
				reqIDs = append(reqIDs, vals[0])
				mu.Unlock()
			}
		}
		return stream.Send(&sppb.PartialResultSet{})
	}

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

	if tr.dcp != nil {
		t.Fatalf("expected default Enabled=false to use static channel pool (dcp == nil)")
	}
	if len(tr.clients) != numCh {
		t.Fatalf("expected %d static ChannelClients, got %d", numCh, len(tr.clients))
	}

	// 1. Pure PickClient() round-robin without any intervening RPCs or nthRequest increments.
	pureSeen := make(map[ChannelClient]bool, numCh)
	var prevClient ChannelClient
	for i := 0; i < numCh*2; i++ {
		c, err := tr.PickClient(context.Background())
		if err != nil {
			t.Fatalf("pure PickClient(%d) failed: %v", i, err)
		}
		if c == prevClient {
			t.Fatalf("pure PickClient(%d) repeated previous client without advancing round-robin", i)
		}
		prevClient = c
		pureSeen[c] = true
	}
	if len(pureSeen) != numCh {
		t.Fatalf("expected %d distinct ChannelClients from pure PickClient() round-robin, got %d", numCh, len(pureSeen))
	}

	// 2. Interleave PickClient() with actual streaming RPCs (which increment nthRequest)
	// to verify that round-robin uses an independent counter and hits all 4 channels starting at channel 1.
	seen := make(map[ChannelClient]bool, numCh)
	for i := 0; i < numCh*2; i++ {
		c, err := tr.PickClient(context.Background())
		if err != nil {
			t.Fatalf("PickClient(%d) failed: %v", i, err)
		}
		if c != tr.clients[i%numCh] {
			t.Fatalf("PickClient(%d) did not select expected client index %d", i, i%numCh)
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
		t.Fatalf("expected %d distinct ChannelClients under interleaved RPC round-robin, got %d", numCh, len(seen))
	}
	mu.Lock()
	gotReqIDs := append([]string(nil), reqIDs...)
	mu.Unlock()
	if len(gotReqIDs) != numCh*2 {
		t.Fatalf("expected %d x-goog-spanner-request-id headers, got %d", numCh*2, len(gotReqIDs))
	}
	for i, id := range gotReqIDs {
		wantChID := strconv.Itoa((i % numCh) + 1)
		wantReqNum := strconv.Itoa(i + 1)
		wantSuffix := "." + wantChID + "." + wantReqNum + ".1"
		if !strings.HasSuffix(id, wantSuffix) {
			t.Fatalf("request %d: expected x-goog-spanner-request-id ending with %q, got %q", i, wantSuffix, id)
		}
	}
}

func TestTransport_DynamicChannelPoolOptInAndScaleUp(t *testing.T) {
	t.Setenv("SPANNER_EMULATOR_HOST", "")
	mock, addr, cleanup := startMockServer(t)
	defer cleanup()

	var (
		primeMu       sync.Mutex
		primePrefixes [][]string
		primeEncs     [][]string
	)
	mock.unaryHandler = func(ctx context.Context, req *sppb.ExecuteSqlRequest) (*sppb.ResultSet, error) {
		if md, ok := metadata.FromIncomingContext(ctx); ok {
			primeMu.Lock()
			primePrefixes = append(primePrefixes, append([]string(nil), md.Get(transport.ResourcePrefixHeader)...))
			primeEncs = append(primeEncs, append([]string(nil), md.Get(transport.RequestsCompressionHeader)...))
			primeMu.Unlock()
		}
		return &sppb.ResultSet{}, nil
	}

	tr, err := NewTransport(context.Background(), ClientConfig{
		Endpoint:    addr,
		PlainText:   true,
		Compression: "gzip",
		DynamicChannelPoolConfig: DynamicChannelPoolConfig{
			Enabled:          true,
			InitialChannels:  2,
			MinChannels:      2,
			MaxChannels:      4,
			MinRPCPerChannel: 1,
			MaxRPCPerChannel: 2,
		},
	})
	if err != nil {
		t.Fatalf("NewTransport with DCP Enabled=true failed: %v", err)
	}
	defer tr.Close()

	if tr.dcp == nil {
		t.Fatalf("expected tr.dcp to be initialized when Enabled=true")
	}
	if got := tr.dcp.Num(); got != 2 {
		t.Fatalf("expected 2 initial channels in DynamicChannelPool, got %d", got)
	}

	mock.handler = func(req *sppb.ExecuteSqlRequest, stream sppb.Spanner_ExecuteStreamingSqlServer) error {
		return stream.Send(&sppb.PartialResultSet{Last: true})
	}

	// Verify dcpResolvingChannelClient and dcpChannelClient methods (RequestIDHeaderInjector, ExecuteSql, ExecuteStreamingSql).
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

	// Reset recorded unary headers before testing scale-up priming.
	primeMu.Lock()
	primePrefixes = nil
	primeEncs = nil
	primeMu.Unlock()

	seen := make(map[*transport.DCPEntry[managedChannelClient]]bool, 2)
	for i := 0; i < 16 && len(seen) < 2; i++ {
		c, err := tr.PickClient(context.Background())
		if err != nil {
			t.Fatalf("PickClient(%d) failed: %v", i, err)
		}
		dcpClient, ok := c.(*dcpResolvingChannelClient)
		if !ok || dcpClient.Entry.Load() == nil {
			t.Fatalf("expected *dcpResolvingChannelClient with non-nil entry, got %T", c)
		}
		entry := dcpClient.Entry.Load()
		seen[entry] = true
		entry.StreamLoad.Add(10)
		defer entry.StreamLoad.Add(-10)
	}
	if len(seen) != 2 {
		t.Fatalf("expected 2 distinct DCPEntry channels under DCPPowerOfTwoLeastBusy, got %d", len(seen))
	}

	// Malformed session paths are ignored by UpdateDCPPrimeSession.
	for _, bad := range []string{"", "/sessions/s1", "projects/p/instances/i/databases/db-prime/sessions/"} {
		tr.UpdateDCPPrimeSession(bad)
	}
	if got := tr.dcp.PrimeSessionID(); got != "" {
		t.Fatalf("expected empty PrimeSessionID after malformed sessions, got %q", got)
	}

	const primeSession = "projects/p/instances/i/databases/db-prime/sessions/s1"
	tr.UpdateDCPPrimeSession(primeSession)
	// Cross-database session on shared Transport must not thrash PrimeSession.
	tr.UpdateDCPPrimeSession("projects/p/instances/i/databases/other-db/sessions/s2")
	if got := tr.dcp.PrimeSessionID(); got != primeSession {
		t.Fatalf("cross-DB UpdateDCPPrimeSession overwrote PrimeSessionID: got %q, want %q", got, primeSession)
	}
	// Same-database session rotation must update PrimeSession.
	const rotatedSession = "projects/p/instances/i/databases/db-prime/sessions/s2"
	tr.UpdateDCPPrimeSession(rotatedSession)
	if got := tr.dcp.PrimeSessionID(); got != rotatedSession {
		t.Fatalf("same-DB UpdateDCPPrimeSession did not rotate: got %q, want %q", got, rotatedSession)
	}
	tr.UpdateDCPPrimeSession(primeSession)

	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) && tr.dcp.Num() <= 2 {
		time.Sleep(10 * time.Millisecond)
	}
	if got := tr.dcp.Num(); got <= 2 {
		t.Fatalf("expected DynamicChannelPool to scale up above 2 channels after UpdateDCPPrimeSession under load, got %d", got)
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
			t.Fatalf("prime RPC %d: expected %s=[projects/p/instances/i/databases/db-prime], got %v", i, transport.ResourcePrefixHeader, p)
		}
	}
	for i, enc := range gotEncs {
		if len(enc) != 1 || enc[0] != "gzip" {
			t.Fatalf("prime RPC %d: expected single %s=[gzip] from sessionMD, got %v", i, transport.RequestsCompressionHeader, enc)
		}
	}
}

func TestTransport_EmulatorEnvDoesNotDowngradeExplicitEndpoint(t *testing.T) {
	_, addr, cleanup := startMockServer(t)
	defer cleanup()

	t.Setenv("SPANNER_EMULATOR_HOST", "emulator.internal:9010")

	tr, err := NewTransport(context.Background(), ClientConfig{
		Endpoint:  addr,
		PlainText: true,
		DynamicChannelPoolConfig: DynamicChannelPoolConfig{
			Enabled: true,
		},
	})
	if err != nil {
		t.Fatalf("NewTransport failed: %v", err)
	}
	defer tr.Close()

	if tr.dcp == nil {
		t.Fatal("expected DCP to remain enabled when explicit cfg.Endpoint differs from SPANNER_EMULATOR_HOST")
	}

	t.Setenv("SPANNER_EMULATOR_HOST", addr)
	emuTransport, err := NewTransport(context.Background(), ClientConfig{
		Endpoint: "http://" + addr,
		DynamicChannelPoolConfig: DynamicChannelPoolConfig{
			Enabled: true,
		},
	})
	if err != nil {
		t.Fatalf("NewTransport(emulator) failed: %v", err)
	}
	defer emuTransport.Close()
	if emuTransport.dcp != nil {
		t.Fatal("expected DCP to be bypassed when connecting to SPANNER_EMULATOR_HOST")
	}
}

func TestTransport_StaticPoolClientOptionsAndFallbackDefault(t *testing.T) {
	t.Setenv("SPANNER_EMULATOR_HOST", "")
	t.Setenv("GOOGLE_SPANNER_ENABLE_GCP_FALLBACK", "")
	t.Setenv("GOOGLE_SPANNER_ENABLE_DIRECT_ACCESS", "invalid-bool")

	mock, addr, cleanup := startMockServer(t)
	defer cleanup()

	var (
		mu       sync.Mutex
		streamMD []metadata.MD
	)
	mock.handler = func(req *sppb.ExecuteSqlRequest, stream sppb.Spanner_ExecuteStreamingSqlServer) error {
		if md, ok := metadata.FromIncomingContext(stream.Context()); ok {
			mu.Lock()
			streamMD = append(streamMD, md.Copy())
			mu.Unlock()
		}
		return stream.Send(&sppb.PartialResultSet{})
	}

	// 1. Conflicting NumChannels and option.WithGRPCConnectionPool returns InvalidArgument.
	if badTransport, err := NewTransport(context.Background(), ClientConfig{
		Endpoint:    addr,
		PlainText:   true,
		NumChannels: 4,
		ClientOptions: []option.ClientOption{
			option.WithGRPCConnectionPool(6),
		},
	}); err == nil {
		_ = badTransport.Close()
		t.Fatal("expected InvalidArgument when both NumChannels=4 and WithGRPCConnectionPool(6) are specified")
	} else if status.Code(err) != codes.InvalidArgument {
		t.Fatalf("expected codes.InvalidArgument, got %v", err)
	}

	// 2. When cfg.NumChannels == 0, option.WithGRPCConnectionPool in ClientOptions is respected,
	// and sessionMD carries x-response-encoding=gzip when Compression="gzip".
	tr, err := NewTransport(context.Background(), ClientConfig{
		Endpoint:           addr,
		PlainText:          true,
		EnableDirectAccess: true,
		Compression:        "gzip",
		ClientOptions: []option.ClientOption{
			option.WithGRPCConnectionPool(6),
		},
	})
	if err != nil {
		t.Fatalf("NewTransport failed: %v", err)
	}
	defer tr.Close()

	if len(tr.clients) != 6 {
		t.Fatalf("expected 6 clients from option.WithGRPCConnectionPool(6) when NumChannels==0, got %d", len(tr.clients))
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
		t.Fatalf("expected 6 distinct channel IDs across 6 clients, got %v", seenCh)
	}
	c, err := tr.PickClient(context.Background())
	if err != nil {
		t.Fatalf("PickClient failed: %v", err)
	}
	riw, err := c.RequestIDHeaderInjector(context.Background())
	if err != nil {
		t.Fatalf("RequestIDHeaderInjector failed: %v", err)
	}
	rpcCtx := metadata.NewOutgoingContext(context.Background(), tr.sessionMD)
	stream, err := c.ExecuteStreamingSql(rpcCtx, &sppb.ExecuteSqlRequest{
		Session: "projects/p/instances/i/databases/d/sessions/s",
		Sql:     "SELECT 1",
	}, riw.WithNextRetryAttempt(1))
	if err != nil {
		t.Fatalf("ExecuteStreamingSql failed: %v", err)
	}
	if _, err := stream.Recv(); err != nil {
		t.Fatalf("stream.Recv failed: %v", err)
	}
	mu.Lock()
	if len(streamMD) != 1 {
		mu.Unlock()
		t.Fatalf("expected 1 recorded stream metadata, got %d", len(streamMD))
	}
	gotEnc := streamMD[0].Get(transport.RequestsCompressionHeader)
	mu.Unlock()
	if len(gotEnc) != 1 || gotEnc[0] != "gzip" {
		t.Fatalf("expected %s=[gzip] on the wire from sessionMD, got %v", transport.RequestsCompressionHeader, gotEnc)
	}

	// 3. PlainText: true with Endpoint: "" still configures insecure credentials and WithoutAuthentication
	// and can execute an RPC when WithEndpoint is supplied via ClientOptions.
	emptyEpTransport, err := NewTransport(context.Background(), ClientConfig{
		Endpoint:  "",
		PlainText: true,
		ClientOptions: []option.ClientOption{
			option.WithEndpoint("passthrough:///" + addr),
		},
	})
	if err != nil {
		t.Fatalf("NewTransport(Endpoint=\"\", PlainText=true) failed: %v", err)
	}
	defer emptyEpTransport.Close()

	c2, err := emptyEpTransport.PickClient(context.Background())
	if err != nil {
		t.Fatalf("emptyEpTransport.PickClient failed: %v", err)
	}
	riw2, err := c2.RequestIDHeaderInjector(context.Background())
	if err != nil {
		t.Fatalf("RequestIDHeaderInjector failed: %v", err)
	}
	stream2, err := c2.ExecuteStreamingSql(context.Background(), &sppb.ExecuteSqlRequest{
		Session: "projects/p/instances/i/databases/d/sessions/s",
		Sql:     "SELECT 1",
	}, riw2.WithNextRetryAttempt(1))
	if err != nil {
		t.Fatalf("emptyEpTransport ExecuteStreamingSql failed: %v", err)
	}
	if _, err := stream2.Recv(); err != nil {
		t.Fatalf("emptyEpTransport stream.Recv failed: %v", err)
	}
}

func TestApplyDCPPrimeMetadata_DoesNotMutateCallerContext(t *testing.T) {
	sessionMD := metadata.MD{
		"X-Response-Encoding":          []string{"gzip"},
		"Google-Cloud-Resource-Prefix": []string{"should-not-override"},
	}
	parentMD := metadata.MD{"X-Custom-Header": []string{"keep-me"}, "Google-Cloud-Resource-Prefix": []string{"old-db"}}
	parentCtx := metadata.NewOutgoingContext(context.Background(), parentMD)

	outCtx := ApplyDCPPrimeMetadata(parentCtx, sessionMD, "projects/p/instances/i/databases/new-db/sessions/s1")

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
	l := NewLogger(func(level, msg string) {
		gotLevel = level
		gotMsg = msg
	})
	l.Println("hello transport")
	if gotLevel != "WARN" || gotMsg != "hello transport" {
		t.Fatalf("unexpected log output: level=%q msg=%q", gotLevel, gotMsg)
	}

	nilLogger := NewLogger(nil)
	nilLogger.Println("discarded")
}

func TestTransport_BoundaryConditionsAndSingleChannel(t *testing.T) {
	t.Setenv("SPANNER_EMULATOR_HOST", "")
	t.Setenv("GOOGLE_SPANNER_ENABLE_DIRECT_ACCESS", "")
	t.Setenv("GOOGLE_SPANNER_ENABLE_GCP_FALLBACK", "")
	// 1. Nil and closed Transport boundary checks.
	var nilTr *Transport
	if _, err := nilTr.PickClient(context.Background()); status.Code(err) != codes.FailedPrecondition {
		t.Fatalf("expected FailedPrecondition from nil Transport.PickClient, got %v", err)
	}
	if err := nilTr.Close(); err != nil {
		t.Fatalf("expected nil error from nil Transport.Close, got %v", err)
	}
	nilTr.UpdateDCPPrimeSession("projects/p/instances/i/databases/d/sessions/s1")

	_, addr, cleanup := startMockServer(t)
	defer cleanup()

	// 2. Single-channel static pool fast path (len(t.clients) == 1) and post-Close PickClient.
	tr1, err := NewTransport(context.Background(), ClientConfig{
		Endpoint:    addr,
		NumChannels: 1,
		PlainText:   true,
	})
	if err != nil {
		t.Fatalf("NewTransport(NumChannels=1) failed: %v", err)
	}
	c1, err := tr1.PickClient(context.Background())
	if err != nil || c1 == nil {
		t.Fatalf("PickClient on NumChannels=1 failed: %v", err)
	}
	if err := tr1.Close(); err != nil {
		t.Fatalf("tr1.Close() failed: %v", err)
	}
	if err := tr1.Close(); err != nil {
		t.Fatalf("idempotent tr1.Close() failed: %v", err)
	}
	if _, err := tr1.PickClient(context.Background()); status.Code(err) != codes.FailedPrecondition {
		t.Fatalf("expected FailedPrecondition after Close(), got %v", err)
	}

	// 3. Malformed session paths in ApplyDCPPrimeMetadata and UpdateDCPPrimeSession.
	for _, badSession := range []string{"", "/sessions/s1", "projects/p/instances/i/databases/d/sessions/"} {
		ctx := ApplyDCPPrimeMetadata(context.Background(), nil, badSession)
		md, _ := metadata.FromOutgoingContext(ctx)
		if got := md.Get(transport.ResourcePrefixHeader); len(got) != 1 || got[0] != PlaceholderDB {
			t.Fatalf("ApplyDCPPrimeMetadata(%q) resource prefix = %v, want [%s]", badSession, got, PlaceholderDB)
		}
	}

	// 4. Endpoint "" with PlainText: true omits WithEndpoint (3 options), whereas an explicit
	// malformed scheme-only Endpoint ("http://" or "https://") or SPANNER_EMULATOR_HOST="http://"
	// still binds WithEndpoint so it cannot silently dial production Spanner.
	emptyPlainCfg := ClientConfig{Endpoint: "", PlainText: true}
	emptyPlainOpts := BuildClientOptions(emptyPlainCfg, ResolveSettings(emptyPlainCfg))
	if len(emptyPlainOpts) != 3 {
		t.Fatalf("expected 3 insecure/no-auth options when Endpoint is unset, got %d", len(emptyPlainOpts))
	}

	barePlainCfg := ClientConfig{Endpoint: "http://", PlainText: true}
	barePlainSettings := ResolveSettings(barePlainCfg)
	if barePlainSettings.EndpointAddr != "" {
		t.Fatalf("expected empty EndpointAddr for bare http://, got %q", barePlainSettings.EndpointAddr)
	}
	if got := len(BuildClientOptions(barePlainCfg, barePlainSettings)); got != 4 {
		t.Fatalf("expected 4 options (including WithEndpoint) for explicit Endpoint=\"http://\", got %d", got)
	}

	bareTLSCfg := ClientConfig{Endpoint: "https://"}
	if got := len(BuildClientOptions(bareTLSCfg, ResolveSettings(bareTLSCfg))); got != 1 {
		t.Fatalf("expected WithEndpoint option for explicit Endpoint=\"https://\", got %d", got)
	}

	t.Setenv("SPANNER_EMULATOR_HOST", "http://")
	emuSchemeOnlySettings := ResolveSettings(ClientConfig{})
	if !emuSchemeOnlySettings.IsEmulator || !emuSchemeOnlySettings.IsPlain || emuSchemeOnlySettings.UseDCP {
		t.Fatalf("expected SPANNER_EMULATOR_HOST=\"http://\" to resolve as emulator, got %+v", emuSchemeOnlySettings)
	}
	if got := len(BuildClientOptions(ClientConfig{}, emuSchemeOnlySettings)); got != 4 {
		t.Fatalf("expected 4 emulator options for SPANNER_EMULATOR_HOST=\"http://\", got %d", got)
	}
	t.Setenv("SPANNER_EMULATOR_HOST", "")

	// 5. Nil dcpResolvingChannelClient receiver is safe and returns Unavailable.
	var nilResolver *dcpResolvingChannelClient
	if _, err := nilResolver.Acquire(context.Background()); status.Code(err) != codes.Unavailable {
		t.Fatalf("expected Unavailable from nil dcpResolvingChannelClient.Acquire, got %v", err)
	}
	if _, err := nilResolver.RequestIDHeaderInjector(context.Background()); status.Code(err) != codes.Unavailable {
		t.Fatalf("expected Unavailable from nil dcpResolvingChannelClient.RequestIDHeaderInjector, got %v", err)
	}
	if _, err := nilResolver.ExecuteSql(context.Background(), &sppb.ExecuteSqlRequest{}); status.Code(err) != codes.Unavailable {
		t.Fatalf("expected Unavailable from nil dcpResolvingChannelClient.ExecuteSql, got %v", err)
	}
	if _, err := nilResolver.ExecuteStreamingSql(context.Background(), &sppb.ExecuteSqlRequest{}); status.Code(err) != codes.Unavailable {
		t.Fatalf("expected Unavailable from nil dcpResolvingChannelClient.ExecuteStreamingSql, got %v", err)
	}

	// 6. UniverseDomain is preserved even when DisableBuiltInAuth is true (non-plaintext),
	// while DirectPath and GCPFallback are disabled when PlainText or DisableBuiltInAuth is set.
	t.Setenv("GOOGLE_SPANNER_ENABLE_DIRECT_ACCESS", "true")
	t.Setenv("GOOGLE_SPANNER_ENABLE_GCP_FALLBACK", "true")
	noAuthUDCfg := ClientConfig{
		UniverseDomain:     "example-universe.test",
		DisableBuiltInAuth: true,
		EnableDirectAccess: true,
	}
	noAuthUDSettings := ResolveSettings(noAuthUDCfg)
	if noAuthUDSettings.IsDirectPathEnabled || noAuthUDSettings.IsFallbackEnabled {
		t.Fatalf("expected DirectPath and Fallback disabled when DisableBuiltInAuth=true, got %+v", noAuthUDSettings)
	}
	if got := len(BuildClientOptions(noAuthUDCfg, noAuthUDSettings)); got != 2 {
		t.Fatalf("expected 2 options (WithoutAuthentication + WithUniverseDomain), got %d", got)
	}
	plainDPSettings := ResolveSettings(ClientConfig{PlainText: true, EnableDirectAccess: true})
	if plainDPSettings.IsDirectPathEnabled || plainDPSettings.IsFallbackEnabled {
		t.Fatalf("expected DirectPath and Fallback disabled when PlainText=true, got %+v", plainDPSettings)
	}
	t.Setenv("GOOGLE_SPANNER_ENABLE_DIRECT_ACCESS", "")
	t.Setenv("GOOGLE_SPANNER_ENABLE_GCP_FALLBACK", "")

	// 7. DCP rejects conflicting WithGRPCConnectionPool(N != 1) instead of multiplying connections.
	if badDCP, err := NewTransport(context.Background(), ClientConfig{
		Endpoint:  addr,
		PlainText: true,
		DynamicChannelPoolConfig: DynamicChannelPoolConfig{
			Enabled:         true,
			InitialChannels: 2,
		},
		ClientOptions: []option.ClientOption{
			option.WithGRPCConnectionPool(4),
		},
	}); err == nil {
		_ = badDCP.Close()
		t.Fatal("expected InvalidArgument when DCP is combined with WithGRPCConnectionPool(4)")
	} else if status.Code(err) != codes.InvalidArgument {
		t.Fatalf("expected codes.InvalidArgument for DCP + WithGRPCConnectionPool(4), got %v", err)
	}
}

func TestToTransportDCPConfigFieldMapping(t *testing.T) {
	scType := reflect.TypeOf(DynamicChannelPoolConfig{})
	trType := reflect.TypeOf(transport.DCPConfig{})
	if scType.NumField() != trType.NumField() {
		t.Fatalf("field count mismatch: sharedcore=%d, transport=%d", scType.NumField(), trType.NumField())
	}
	var in DynamicChannelPoolConfig
	inVal := reflect.ValueOf(&in).Elem()
	for i := 0; i < scType.NumField(); i++ {
		f := inVal.Field(i)
		switch f.Kind() {
		case reflect.Bool:
			f.SetBool(true)
		case reflect.Int, reflect.Int32, reflect.Int64:
			f.SetInt(int64((i + 1) * 11))
		case reflect.Float64:
			f.SetFloat(float64((i+1)*10) + 0.25)
		default:
			t.Fatalf("unhandled field kind %v", f.Kind())
		}
	}
	trVal := reflect.ValueOf(toTransportDCPConfig(in))
	for i := 0; i < scType.NumField(); i++ {
		wantName := "DCP" + scType.Field(i).Name
		tf := trVal.FieldByName(wantName)
		if !tf.IsValid() || tf.Convert(inVal.Field(i).Type()).Interface() != inVal.Field(i).Interface() {
			t.Fatalf("field %s -> %s mismatch: got %v, want %v", scType.Field(i).Name, wantName, tf, inVal.Field(i))
		}
	}
}

func BenchmarkTransportPickClientStatic(b *testing.B) {
	lis, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		b.Fatalf("failed to listen: %v", err)
	}
	srv := grpc.NewServer()
	sppb.RegisterSpannerServer(srv, &mockSpannerServer{})
	go func() { _ = srv.Serve(lis) }()
	defer func() {
		srv.Stop()
		_ = lis.Close()
	}()

	for _, numCh := range []int{1, 4} {
		b.Run("channels="+strconv.Itoa(numCh), func(b *testing.B) {
			tr, err := NewTransport(context.Background(), ClientConfig{
				Endpoint:    lis.Addr().String(),
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

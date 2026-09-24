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

package bigtable

import (
	"context"
	"net"
	"testing"
	"time"

	btpb "cloud.google.com/go/bigtable/apiv2/bigtablepb"
	"google.golang.org/api/option"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/test/bufconn"
)

// TestClientConfig_DisableSession_SkipsSessionInit pins the
// DisableSession=true gate: NewClientWithConfig must NOT construct
// the session data-plane client, and every downstream sessionImpl /
// sessionTables field must be nil.
//
// The test uses WithContextDialer (not WithGRPCConn) so preDialed
// stays false — otherwise the preDialed path would also skip
// session.NewClient and the test would not isolate the DisableSession
// gate. With DisableSession=true AND preDialed=false, only the
// DisableSession gate can be responsible for sessionImpl being nil.
//
// The bufconn has no server behind it. If the gate is broken,
// session.NewClient would try to reach a non-responsive backend and
// the 5-second context would time out — the test would fail with a
// deadline error, distinguishing "gate broken" from "gate worked."
func TestClientConfig_DisableSession_SkipsSessionInit(t *testing.T) {
	lis := bufconn.Listen(1024 * 1024)
	t.Cleanup(func() { _ = lis.Close() })

	// Serve a minimal fake so the classic pool's Prime (PingAndWarm)
	// succeeds; without it, mPool never finishes construction and this
	// test would fail on the classic side, not the DisableSession gate.
	grpcSrv := grpc.NewServer()
	t.Cleanup(grpcSrv.Stop)
	btpb.RegisterBigtableServer(grpcSrv, newFakeBigtableServer(t))
	go func() { _ = grpcSrv.Serve(lis) }()

	dialer := func(ctx context.Context, _ string) (net.Conn, error) {
		return lis.DialContext(ctx)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	c, err := NewClientWithConfig(ctx, "test-project", "test-instance", ClientConfig{
		DisableSession:  true,
		MetricsProvider: NoopMetricsProvider{},
	},
		option.WithEndpoint("passthrough:///bufnet"),
		option.WithGRPCDialOption(grpc.WithContextDialer(dialer)),
		option.WithGRPCDialOption(grpc.WithTransportCredentials(insecure.NewCredentials())),
		option.WithoutAuthentication(),
	)
	if err != nil {
		t.Fatalf("NewClientWithConfig with DisableSession=true failed: %v", err)
	}
	t.Cleanup(func() { _ = c.Close() })

	if c.sessionImpl != nil {
		t.Errorf("sessionImpl = %T, want nil (DisableSession=true must skip session.NewClient)", c.sessionImpl)
	}
	if c.sessionTables != nil {
		t.Errorf("sessionTables = %v, want nil (DisableSession=true must skip cache init)", c.sessionTables)
	}
}

// TestClientConfig_DisableSession_ClientCloseWorks confirms Client.Close
// on a DisableSession client is a clean no-op on the session side —
// every downstream nil-guard fires as intended (client.go:322 comment
// promises "sessionTables is nil ... the cache's own close() nil-checks
// for that"; client.go:329 wraps sessionImpl.Close in a nil check).
func TestClientConfig_DisableSession_ClientCloseWorks(t *testing.T) {
	lis := bufconn.Listen(1024 * 1024)
	t.Cleanup(func() { _ = lis.Close() })

	// Serve a minimal fake so the classic pool's Prime (PingAndWarm)
	// succeeds; without it, mPool never finishes construction and this
	// test would fail on the classic side, not the DisableSession gate.
	grpcSrv := grpc.NewServer()
	t.Cleanup(grpcSrv.Stop)
	btpb.RegisterBigtableServer(grpcSrv, newFakeBigtableServer(t))
	go func() { _ = grpcSrv.Serve(lis) }()

	dialer := func(ctx context.Context, _ string) (net.Conn, error) {
		return lis.DialContext(ctx)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	c, err := NewClientWithConfig(ctx, "test-project", "test-instance", ClientConfig{
		DisableSession:  true,
		MetricsProvider: NoopMetricsProvider{},
	},
		option.WithEndpoint("passthrough:///bufnet"),
		option.WithGRPCDialOption(grpc.WithContextDialer(dialer)),
		option.WithGRPCDialOption(grpc.WithTransportCredentials(insecure.NewCredentials())),
		option.WithoutAuthentication(),
	)
	if err != nil {
		t.Fatalf("NewClientWithConfig: %v", err)
	}

	// Explicit Close — must not panic on the nil sessionImpl / sessionTables.
	if err := c.Close(); err != nil {
		t.Errorf("Close on DisableSession client returned err = %v, want nil", err)
	}
	// Second Close must also be safe (idempotent per session_pool_lifecycle
	// / mPool.Close conventions).
	_ = c.Close()
}

// TestClientConfig_DisableSession_OpenTableRoutesClassicOnly confirms
// that on a DisableSession client, TableShim's session side is nil, so
// useSession() reports false and every Apply / ReadRow routes to the
// classic path unconditionally — regardless of the Diverter's
// SessionLoad. This is the load-bearing contract for callers that
// opt out: they get classic behavior even if some future code path
// bumps the Diverter.
func TestClientConfig_DisableSession_OpenTableRoutesClassicOnly(t *testing.T) {
	lis := bufconn.Listen(1024 * 1024)
	t.Cleanup(func() { _ = lis.Close() })

	// Serve a minimal fake so the classic pool's Prime (PingAndWarm)
	// succeeds; without it, mPool never finishes construction and this
	// test would fail on the classic side, not the DisableSession gate.
	grpcSrv := grpc.NewServer()
	t.Cleanup(grpcSrv.Stop)
	btpb.RegisterBigtableServer(grpcSrv, newFakeBigtableServer(t))
	go func() { _ = grpcSrv.Serve(lis) }()

	dialer := func(ctx context.Context, _ string) (net.Conn, error) {
		return lis.DialContext(ctx)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	c, err := NewClientWithConfig(ctx, "test-project", "test-instance", ClientConfig{
		DisableSession:  true,
		MetricsProvider: NoopMetricsProvider{},
	},
		option.WithEndpoint("passthrough:///bufnet"),
		option.WithGRPCDialOption(grpc.WithContextDialer(dialer)),
		option.WithGRPCDialOption(grpc.WithTransportCredentials(insecure.NewCredentials())),
		option.WithoutAuthentication(),
	)
	if err != nil {
		t.Fatalf("NewClientWithConfig: %v", err)
	}
	t.Cleanup(func() { _ = c.Close() })

	// Force the Diverter into session-preferring mode. If DisableSession
	// works as advertised, useSession() still reports false because
	// TableShim.session is nil.
	if c.diverter != nil {
		c.diverter.SetSessionLoad(1.0)
	}

	tblAPI := c.OpenTable("mytable")
	shim, ok := tblAPI.(*TableShim)
	if !ok {
		t.Fatalf("OpenTable returned %T, want *TableShim", tblAPI)
	}
	if shim.session != nil {
		t.Errorf("shim.session = %T, want nil (DisableSession clients must not wire a session TableAPI)", shim.session)
	}
	if shim.useSession() {
		t.Error("shim.useSession() = true, want false (nil session side must gate to classic)")
	}
}

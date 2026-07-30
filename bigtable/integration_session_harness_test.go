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

// sessionTestHarness wires a fakeBigtableServer to a bufconn-backed grpc.Server
// and dials it through a real *grpc.ClientConn. Tests then build a Client over
// that conn with EnableSessionPool=true, which makes ReadRow / Apply traverse
// the SessionTable → SessionPoolImpl → vRPC plumbing end-to-end.
//
// Why a fake server (not bttest):
// bttest returns Unimplemented for OpenTable, so it cannot drive the
// session/vRPC path at all. A small inline fake is also faster (no real
// network) and lets us inspect every VirtualRpcRequest the client sent —
// which is exactly what we need to assert deadline/metadata plumbing.
//
// Why the higher-level Client path (not direct transport.NewSession):
// We want to verify the full chain — TableShim → SessionTable → SessionPoolImpl
// → Session → wire frames — actually wires up correctly with EnableSessionPool
// set on a real client. Passing option.WithGRPCConn(conn) sets
// enableBigtableConnPool=false (see client.go:175-182), which keeps the dial
// inside the simple gtransport.DialPool branch and avoids the
// BigtableChannelPool factory machinery that doesn't play nicely with
// bufconn. SessionPoolMin=1 / SessionPoolMax=2 keeps the test fleet small.
type sessionTestHarness struct {
	t      *testing.T
	server *fakeBigtableServer
	grpc   *grpc.Server
	lis    *bufconn.Listener
	client *Client

	reverseClose bool // grpc.Stop before client.Close on teardown
}

// harnessConfig is the tunable surface of newSessionTestHarness. Fields
// default to the small-fleet fixture (Min=1/Max=2, wait for routing,
// client-close-first teardown). Every hand-configured test used to
// re-derive one of these from scratch; the options below express the
// diff instead.
type harnessConfig struct {
	poolMin      int32
	poolMax      int32
	fakeSetup    func(*fakeBigtableServer)
	waitRouting  bool
	reverseClose bool // for tests that stall vRPCs: grpc.Stop must fire before client.Close or Close hangs on the graceful CloseSession the stall blocks
}

type harnessOpt func(*harnessConfig)

// withPoolSize sets BOTH sides of the sizing knob: the fake advertises
// {min,max} in GetClientConfiguration (which the client's config manager
// treats as authoritative), and the ClientConfig fed to
// NewClientWithConfig uses the same numbers. Keeping the two in one
// helper is the whole reason this exists — a mismatch silently lets the
// config manager overwrite the client's ask with the fake's default.
func withPoolSize(min, max int32) harnessOpt {
	return func(c *harnessConfig) { c.poolMin, c.poolMax = min, max }
}

// withFakeSetup runs fn against the fake BEFORE the gRPC server starts
// serving. Use this for setPeerInfoRotation, setSessionParamsKeepAlive,
// queueVRpcStalls, etc — arming after Serve races the first stream.
func withFakeSetup(fn func(*fakeBigtableServer)) harnessOpt {
	return func(c *harnessConfig) { c.fakeSetup = fn }
}

// withoutRoutingWait skips the poll for GetClientConfiguration + Diverter
// flip. Useful when the test either relies on the NewDiverter(1.0)
// bootstrap or asserts a routing-independent property (e.g. Close
// semantics) and wants tighter timing.
func withoutRoutingWait() harnessOpt {
	return func(c *harnessConfig) { c.waitRouting = false }
}

// withReverseCloseOrder tears down the gRPC server BEFORE closing the
// Client. Required for tests that stall vRPCs — otherwise client.Close
// hangs on the graceful CloseSession exchange the stall prevents the
// fake from processing.
func withReverseCloseOrder() harnessOpt {
	return func(c *harnessConfig) { c.reverseClose = true }
}

func newSessionTestHarness(t *testing.T, opts ...harnessOpt) *sessionTestHarness {
	t.Helper()
	cfg := harnessConfig{
		poolMin:     1,
		poolMax:     2,
		waitRouting: true,
	}
	for _, o := range opts {
		o(&cfg)
	}

	lis := bufconn.Listen(1 << 20)
	grpcSrv := grpc.NewServer()
	fakeSrv := newFakeBigtableServer(t)
	fakeSrv.setSessionPoolSizing(cfg.poolMin, cfg.poolMax)
	if cfg.fakeSetup != nil {
		cfg.fakeSetup(fakeSrv)
	}
	btpb.RegisterBigtableServer(grpcSrv, fakeSrv)
	go func() {
		// Serve returns when grpcSrv.Stop is called by the test cleanup.
		_ = grpcSrv.Serve(lis)
	}()

	// IMPORTANT: option.WithGRPCConn is unsafe here. session.NewSessionClient
	// dials TWO conns off the caller's opts — one for the pool, one for the
	// DirectAccessChecker probe. WithGRPCConn hands the same *grpc.ClientConn
	// to both, and CheckCompatibility Close()es the probe conn after ALTS
	// rejects bufconn — which closes the pool's conn too, and no RPC ever
	// lands on the fake. Pass a real ContextDialer instead so every
	// gtransport.Dial call cuts a fresh, independently-closable conn to the
	// bufconn.
	dialer := func(ctx context.Context, _ string) (net.Conn, error) {
		return lis.DialContext(ctx)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	// No EnableSessionPool flag on upstream ClientConfig: the session
	// data plane is constructed whenever the caller doesn't force
	// preDialed via option.WithGRPCConn. Passing option.WithContextDialer
	// keeps preDialed=false so session.NewClient runs. Pool sizing here
	// is driven by GetClientConfiguration on the fake server (see
	// setSessionPoolSizing in the harness options), not by the
	// ClientConfig struct.
	client, err := NewClientWithConfig(ctx, "test-project", "test-instance", ClientConfig{
		MetricsProvider: NoopMetricsProvider{},
	},
		option.WithEndpoint("passthrough:///bufnet"),
		option.WithGRPCDialOption(grpc.WithContextDialer(dialer)),
		option.WithGRPCDialOption(grpc.WithTransportCredentials(insecure.NewCredentials())),
		option.WithoutAuthentication(),
	)
	if err != nil {
		grpcSrv.Stop()
		t.Fatalf("NewClientWithConfig: %v", err)
	}

	h := &sessionTestHarness{
		t:            t,
		server:       fakeSrv,
		grpc:         grpcSrv,
		lis:          lis,
		client:       client,
		reverseClose: cfg.reverseClose,
	}
	t.Cleanup(h.Close)
	if cfg.waitRouting {
		// Block until the initial configManager poll has landed and flipped
		// the Diverter to SessionLoad=1.0. Without this, the TableShim
		// consults a Diverter that still carries the listener's bootstrap
		// value (0.0, set when the listener was registered with the default
		// config) and routes to the classic path, which our fake server does
		// not implement.
		h.waitForSessionRouting(5 * time.Second)
	}
	return h
}

// waitForSessionRouting polls the client's Diverter (and the server's
// GetClientConfiguration counter as a backstop) until both confirm that
// session routing is enabled. Returns once UseSession() reports true; fails
// the test on timeout.
func (h *sessionTestHarness) waitForSessionRouting(timeout time.Duration) {
	h.t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if h.server.getClientConfigCount() >= 1 && h.client.diverter.UseSession() {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	h.t.Fatalf("timed out waiting for Diverter to flip to session routing (getClientConfigCount=%d, useSession=%t)",
		h.server.getClientConfigCount(), h.client.diverter.UseSession())
}

func (h *sessionTestHarness) Close() {
	// Default: bigtable.Client first (drains session pool cleanly, closes
	// the conns it dialed itself), then the server. Tests that stall
	// vRPCs opt into reverseClose so grpc.Stop kills the stalled streams
	// before client.Close tries the graceful CloseSession exchange (which
	// would otherwise hang forever).
	if h.reverseClose {
		h.grpc.Stop()
		_ = h.lis.Close()
		_ = h.client.Close()
		return
	}
	_ = h.client.Close()
	h.grpc.Stop()
	_ = h.lis.Close()
}

// waitForVRpcs polls until the server has captured at least `want` VRpcs or
// the deadline fires. It returns the snapshot at the time of success so
// callers can assert on it without racing against late deliveries.
func waitForVRpcs(t *testing.T, srv *fakeBigtableServer, want int, timeout time.Duration) []capturedVRpc {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		got := srv.snapshotVRpcs()
		if len(got) >= want {
			return got
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("timed out after %v waiting for %d vRPCs (saw %d)", timeout, want, len(srv.snapshotVRpcs()))
	return nil
}

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

package session

import (
	"context"
	"net"
	"testing"
	"time"

	btpb "cloud.google.com/go/bigtable/apiv2/bigtablepb"
	btopt "cloud.google.com/go/bigtable/internal/option"
	btransport "cloud.google.com/go/bigtable/internal/transport"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

// testConfigServer serves GetClientConfiguration for registerScaleMonitor tests.
type testConfigServer struct {
	btpb.UnimplementedBigtableServer
}

func (s *testConfigServer) GetClientConfiguration(context.Context, *btpb.GetClientConfigurationRequest) (*btpb.ClientConfiguration, error) {
	return &btpb.ClientConfiguration{
		SessionConfiguration: &btpb.SessionClientConfiguration{
			ChannelConfiguration: &btpb.SessionClientConfiguration_ChannelPoolConfiguration{
				MinServerCount:        2,
				MaxServerCount:        8,
				PerServerSessionCount: 4,
			},
		},
	}, nil
}

type noopDirectAccessChecker struct{}

func (noopDirectAccessChecker) CheckCompatibility(context.Context) (*btransport.BigtableConn, bool) {
	return nil, false
}
func (noopDirectAccessChecker) Dialer() func() (*btransport.BigtableConn, error) { return nil }
func (noopDirectAccessChecker) LastProbeConfig() *btpb.ClientConfiguration       { return nil }

func newTestBigtableChannelPool(t *testing.T) (*btransport.BigtableChannelPool, btpb.BigtableClient) {
	t.Helper()
	lis, err := net.Listen("tcp", "localhost:0")
	if err != nil {
		t.Fatalf("net.Listen: %v", err)
	}
	srv := grpc.NewServer()
	btpb.RegisterBigtableServer(srv, &testConfigServer{})
	go func() { _ = srv.Serve(lis) }()
	t.Cleanup(func() {
		srv.Stop()
		_ = lis.Close()
	})

	addr := lis.Addr().String()
	dial := func() (*btransport.BigtableConn, error) {
		conn, dialErr := grpc.NewClient(addr, grpc.WithTransportCredentials(insecure.NewCredentials()))
		if dialErr != nil {
			return nil, dialErr
		}
		return btransport.NewBigtableConn(conn), nil
	}
	pool, err := btransport.NewBigtableChannelPool(
		context.Background(),
		2,
		btopt.RoundRobin,
		dial,
		time.Now(),
		btransport.WithChannelPrimer(btransport.NoOpChannelPrimer{}),
		btransport.WithDirectAccessChecker(noopDirectAccessChecker{}),
	)
	if err != nil {
		t.Fatalf("NewBigtableChannelPool: %v", err)
	}
	return pool, btpb.NewBigtableClient(pool)
}

// TestRegisterScaleMonitor_DefaultReplacesDSMWithSessionAware verifies that by
// default (DisableSessionAwareChannelPool == false), registerScaleMonitor stops
// and detaches any pre-started DynamicScaleMonitor, wires
// SessionAwareScaleMonitor instead, and Close() cleanly unwinds it.
func TestRegisterScaleMonitor_DefaultReplacesDSMWithSessionAware(t *testing.T) {
	pool, stub := newTestBigtableChannelPool(t)
	bgCtx, cancel := context.WithCancel(context.Background())
	dsm := btransport.NewDynamicScaleMonitor(btopt.DefaultDynamicChannelPoolConfig(), pool)
	dsm.Start(bgCtx)

	sc := newSessionClientFromParts(pool, stub, newTestFactory(t), Config{
		Project:       "p",
		Instance:      "i",
		AppProfile:    "ap",
		BackgroundCtx: bgCtx,
		FeatureFlagsProto: btransport.NewFeatureFlagsProto(btransport.FeatureFlagsInput{
			EnableDirectAccess: true,
		}),
	})
	sc.backgroundCancel = cancel
	sc.managedChannelPool = btransport.NewManagedChannelPool(pool, dsm, nil)
	sc.registerScaleMonitor(pool)

	if sc.scaleMonitor == nil {
		t.Fatal("scaleMonitor = nil, want non-nil SessionAwareScaleMonitor")
	}
	if sc.managedChannelPool.Dsm != nil {
		t.Errorf("managedChannelPool.Dsm = %v, want nil when SessionAwareScaleMonitor replaces DSM", sc.managedChannelPool.Dsm)
	}
	if err := sc.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
}

// TestRegisterScaleMonitor_OptOutFallsBackToDSM verifies that setting
// Config.DisableSessionAwareChannelPool = true skips SessionAwareScaleMonitor
// and ensures DynamicScaleMonitor remains active so the channel pool still scales.
func TestRegisterScaleMonitor_OptOutFallsBackToDSM(t *testing.T) {
	pool, stub := newTestBigtableChannelPool(t)
	bgCtx, cancel := context.WithCancel(context.Background())
	sc := newSessionClientFromParts(pool, stub, newTestFactory(t), Config{
		Project:                        "p",
		Instance:                       "i",
		AppProfile:                     "ap",
		BackgroundCtx:                  bgCtx,
		DisableSessionAwareChannelPool: true,
		FeatureFlagsProto: btransport.NewFeatureFlagsProto(btransport.FeatureFlagsInput{
			EnableDirectAccess: true,
		}),
	})
	sc.backgroundCancel = cancel
	sc.managedChannelPool = btransport.NewManagedChannelPool(pool, nil, nil)
	sc.registerScaleMonitor(pool)

	if sc.scaleMonitor != nil {
		t.Errorf("scaleMonitor = %v, want nil when DisableSessionAwareChannelPool is true", sc.scaleMonitor)
	}
	if sc.managedChannelPool.Dsm == nil {
		t.Fatal("managedChannelPool.Dsm = nil, want DynamicScaleMonitor fallback when DisableSessionAwareChannelPool is true")
	}
	if err := sc.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
}

func TestDisableSessionAwareChannelPoolEnvVar(t *testing.T) {
	if btopt.DisableSessionAwareChannelPool() {
		t.Fatal("DisableSessionAwareChannelPool() = true by default, want false")
	}
	t.Setenv(btopt.DisableSessionAwareChannelPoolEnvVar, "true")
	if !btopt.DisableSessionAwareChannelPool() {
		t.Fatal("DisableSessionAwareChannelPool() = false with env=true, want true")
	}
}

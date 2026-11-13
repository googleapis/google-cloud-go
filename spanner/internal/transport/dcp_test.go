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

package transport

import (
	"context"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	gtransport "google.golang.org/api/transport/grpc"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

type fakeDCPClient struct {
	closed atomic.Bool
}

func (f *fakeDCPClient) Close() error {
	f.closed.Store(true)
	return nil
}

type fakeConnPool struct {
	closed atomic.Bool
}

func (p *fakeConnPool) Conn() *grpc.ClientConn { return nil }
func (p *fakeConnPool) Num() int               { return 1 }
func (p *fakeConnPool) Close() error {
	p.closed.Store(true)
	return nil
}
func (p *fakeConnPool) Invoke(ctx context.Context, method string, args, reply any, opts ...grpc.CallOption) error {
	return nil
}
func (p *fakeConnPool) NewStream(ctx context.Context, desc *grpc.StreamDesc, method string, opts ...grpc.CallOption) (grpc.ClientStream, error) {
	return nil, nil
}

func TestNormalizeDCPConfig(t *testing.T) {
	cfg, err := NormalizeDCPConfig(DCPConfig{DCPEnabled: true})
	if err != nil {
		t.Fatalf("NormalizeDCPConfig default failed: %v", err)
	}
	def := DefaultDCPConfig()
	if cfg.DCPInitialChannels != def.DCPInitialChannels || cfg.DCPMinChannels != def.DCPMinChannels || cfg.DCPMaxChannels != def.DCPMaxChannels {
		t.Fatalf("unexpected normalized channel bounds: %+v", cfg)
	}

	if _, err := NormalizeDCPConfig(DCPConfig{
		DCPEnabled:     true,
		DCPMinChannels: 5,
		DCPMaxChannels: 3,
	}); err == nil || !strings.Contains(err.Error(), "DCPMaxChannels") {
		t.Fatalf("expected validation error when MinChannels > MaxChannels, got %v", err)
	}
}

func TestDynamicChannelPool_PrimeFallbackAndRoundRobin(t *testing.T) {
	// Zero-initialized DynamicChannelPool should still return FailedPrecondition from Prime
	// and Unavailable from PickRoundRobin when PrimeNotReadyErr / NoEntriesErr are unset.
	var uninitPool DynamicChannelPool[*fakeDCPClient]
	if err := uninitPool.Prime(context.Background(), &DCPEntry[*fakeDCPClient]{}); status.Code(err) != codes.FailedPrecondition {
		t.Fatalf("expected FailedPrecondition from Prime on uninitialized pool, got %v", err)
	}
	if _, err := uninitPool.PickRoundRobin(); status.Code(err) != codes.Unavailable {
		t.Fatalf("expected Unavailable from PickRoundRobin on empty pool, got %v", err)
	}

	var nilResolver *DCPResolver[*fakeDCPClient]
	if _, err := nilResolver.Acquire(context.Background()); status.Code(err) != codes.Unavailable {
		t.Fatalf("expected Unavailable from nil DCPResolver.Acquire, got %v", err)
	}

	var primedCount atomic.Int32
	pool, err := NewDynamicChannelPool[*fakeDCPClient](
		context.Background(),
		DCPConfig{
			DCPEnabled:           true,
			DCPInitialChannels:   3,
			DCPMinChannels:       2,
			DCPMaxChannels:       5,
			DCPSelectionStrategy: DCPRoundRobin,
		},
		nil,
		func(context.Context) (gtransport.ConnPool, error) {
			return &fakeConnPool{}, nil
		},
		func(_ context.Context, e *DCPEntry[*fakeDCPClient]) error {
			c := &fakeDCPClient{}
			e.Delegate = c
			e.Client = c
			return nil
		},
		func(_ context.Context, _ *DCPEntry[*fakeDCPClient], sessionID string) error {
			if sessionID != "" {
				primedCount.Add(1)
			}
			return nil
		},
		nil,
		nil,
		nil,
	)
	if err != nil {
		t.Fatalf("NewDynamicChannelPool failed: %v", err)
	}
	defer pool.Close()

	if got := pool.Num(); got != 3 {
		t.Fatalf("pool.Num() = %d, want 3", got)
	}

	for i := 0; i < 6; i++ {
		e, err := pool.Pick(context.Background())
		if err != nil {
			t.Fatalf("Pick(%d) failed: %v", i, err)
		}
		wantID := uint64((i % 3) + 1)
		if e.ID != wantID {
			t.Fatalf("Pick(%d) entry ID = %d, want %d", i, e.ID, wantID)
		}
	}

	// Verify error penalty accumulation and expiry on DCPEntry.
	entries := pool.GetEntries()
	e0 := entries[0]
	pool.Cfg.DCPErrorPenaltyDuration = 20 * time.Millisecond
	e0.ApplyErrorPenalty(status.Error(codes.Unavailable, "transient"))
	if p := e0.CurrentPenalty(); p <= 0 {
		t.Fatalf("expected positive error penalty after Unavailable, got %d", p)
	}
	time.Sleep(30 * time.Millisecond)
	if p := e0.CurrentPenalty(); p != 0 {
		t.Fatalf("expected error penalty to expire to 0, got %d", p)
	}

	// Verify DCPResolver re-picks when its bound entry is closed.
	res := &DCPResolver[*fakeDCPClient]{Pool: pool}
	res.Entry.Store(e0)
	_ = e0.CloseNow()
	acquired, err := res.Acquire(context.Background())
	if err != nil {
		t.Fatalf("DCPResolver.Acquire after entry close failed: %v", err)
	}
	defer acquired.Release()
	if acquired == e0 {
		t.Fatal("expected DCPResolver.Acquire to re-pick a live entry after e0 was closed")
	}
}

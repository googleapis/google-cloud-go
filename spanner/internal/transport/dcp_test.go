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
	"errors"
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
	closes  atomic.Int32
	onClose func() error
}

func (f *fakeDCPClient) Close() error {
	f.closes.Add(1)
	if f.onClose != nil {
		return f.onClose()
	}
	return nil
}

type fakeDCPConnPool struct {
	closes atomic.Int32
}

func (p *fakeDCPConnPool) Conn() *grpc.ClientConn { return nil }
func (p *fakeDCPConnPool) Num() int               { return 1 }
func (p *fakeDCPConnPool) Close() error {
	p.closes.Add(1)
	return nil
}
func (p *fakeDCPConnPool) Invoke(ctx context.Context, method string, args, reply any, opts ...grpc.CallOption) error {
	return nil
}
func (p *fakeDCPConnPool) NewStream(ctx context.Context, desc *grpc.StreamDesc, method string, opts ...grpc.CallOption) (grpc.ClientStream, error) {
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

func TestNewDynamicChannelPool_RejectsInvalidHooks(t *testing.T) {
	validDial := func(context.Context) (gtransport.ConnPool, error) { return &fakeDCPConnPool{}, nil }
	validNewEntry := func(_ context.Context, e *DCPEntry[*fakeDCPClient]) error {
		c := &fakeDCPClient{}
		e.Delegate = c
		e.Client = c
		return nil
	}
	validPrime := func(context.Context, *DCPEntry[*fakeDCPClient], string) error { return nil }

	for _, tc := range []struct {
		name  string
		hooks DCPHooks[*fakeDCPClient]
	}{
		{name: "nil Dial", hooks: DCPHooks[*fakeDCPClient]{NewEntryClient: validNewEntry, PrimeRPC: validPrime}},
		{name: "nil NewEntryClient", hooks: DCPHooks[*fakeDCPClient]{Dial: validDial, PrimeRPC: validPrime}},
		{name: "nil PrimeRPC", hooks: DCPHooks[*fakeDCPClient]{Dial: validDial, NewEntryClient: validNewEntry}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := NewDynamicChannelPool(context.Background(), DCPConfig{DCPEnabled: true}, tc.hooks); err == nil {
				t.Fatalf("expected error when %s is nil", tc.name)
			}
		})
	}

	t.Run("NewEntryClient leaving Client unset closes Delegate and Pool exactly once", func(t *testing.T) {
		fp := &fakeDCPConnPool{}
		var del *fakeDCPClient
		_, err := NewDynamicChannelPool(
			context.Background(),
			DCPConfig{DCPEnabled: true, DCPInitialChannels: 1, DCPMinChannels: 1, DCPMaxChannels: 2},
			DCPHooks[*fakeDCPClient]{
				Dial: func(context.Context) (gtransport.ConnPool, error) { return fp, nil },
				NewEntryClient: func(_ context.Context, e *DCPEntry[*fakeDCPClient]) error {
					// Simulate GAPIC delegate whose Close() re-entrantly calls e.Close().
					del = &fakeDCPClient{onClose: e.Close}
					e.Delegate = del
					return nil
				},
				PrimeRPC: validPrime,
			},
		)
		if err == nil || !strings.Contains(err.Error(), "DCPEntry.Client") {
			t.Fatalf("expected missing DCPEntry.Client error, got %v", err)
		}
		if got := del.closes.Load(); got != 1 {
			t.Fatalf("delegate closes = %d, want 1", got)
		}
		if got := fp.closes.Load(); got != 1 {
			t.Fatalf("entryPool closes = %d, want 1 (no double-close)", got)
		}
	})
}

func TestDynamicChannelPool_SentinelsRoundRobinPenaltyAndResolver(t *testing.T) {
	t.Run("zero-value pool fallback error sentinels", func(t *testing.T) {
		var uninitPool DynamicChannelPool[*fakeDCPClient]
		if err := uninitPool.prime(context.Background(), &DCPEntry[*fakeDCPClient]{}); status.Code(err) != codes.FailedPrecondition {
			t.Fatalf("expected FailedPrecondition from prime on uninitialized pool, got %v", err)
		}
		if _, err := uninitPool.PickRoundRobin(); status.Code(err) != codes.Unavailable {
			t.Fatalf("expected Unavailable from PickRoundRobin on empty pool, got %v", err)
		}
	})

	customNoEntries := errors.New("custom no entries")
	customPrimeNotReady := errors.New("custom prime not ready")
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
		DCPHooks[*fakeDCPClient]{
			Dial: func(context.Context) (gtransport.ConnPool, error) {
				return &fakeDCPConnPool{}, nil
			},
			NewEntryClient: func(_ context.Context, e *DCPEntry[*fakeDCPClient]) error {
				c := &fakeDCPClient{}
				e.Delegate = c
				e.Client = c
				return nil
			},
			PrimeRPC: func(_ context.Context, _ *DCPEntry[*fakeDCPClient], sessionID string) error {
				if sessionID != "" {
					primedCount.Add(1)
				}
				return nil
			},
			NoEntriesErr:     customNoEntries,
			PrimeNotReadyErr: customPrimeNotReady,
		},
	)
	if err != nil {
		t.Fatalf("NewDynamicChannelPool failed: %v", err)
	}
	defer pool.Close()

	t.Run("initial startup does not prime and PrimeSessionID tracks SetPrimeSession", func(t *testing.T) {
		if got := primedCount.Load(); got != 0 {
			t.Fatalf("primedCount after initial startup = %d, want 0", got)
		}
		if got := pool.PrimeSessionID(); got != "" {
			t.Fatalf("PrimeSessionID() before SetPrimeSession = %q, want empty", got)
		}
		if err := pool.prime(context.Background(), pool.GetEntries()[0]); !errors.Is(err, customPrimeNotReady) {
			t.Fatalf("expected customPrimeNotReady before SetPrimeSession, got %v", err)
		}
		pool.SetPrimeSession("projects/p/instances/i/databases/d/sessions/s1")
		if got := pool.PrimeSessionID(); got != "projects/p/instances/i/databases/d/sessions/s1" {
			t.Fatalf("PrimeSessionID() = %q, want session ID", got)
		}
	})

	t.Run("round-robin selection across active entries", func(t *testing.T) {
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
	})

	t.Run("deterministic error penalty accumulation and expiry", func(t *testing.T) {
		e0 := pool.GetEntries()[0]
		e0.ApplyErrorPenalty(status.Error(codes.Unavailable, "transient"))
		if p := e0.CurrentPenalty(); p <= 0 {
			t.Fatalf("expected positive error penalty after Unavailable, got %d", p)
		}
		e0.PenaltyExpiry.Store(time.Now().Add(-time.Second).UnixNano())
		if p := e0.CurrentPenalty(); p != 0 {
			t.Fatalf("expected error penalty to expire to 0, got %d", p)
		}
	})

	t.Run("resolver re-pick on closed entry and NoEntriesErr when all draining", func(t *testing.T) {
		entries := pool.GetEntries()
		e0 := entries[0]
		res := &DCPResolver[*fakeDCPClient]{Pool: pool}
		res.Entry.Store(e0)
		e0.State.Store(DCPStateDraining)
		remaining := []*DCPEntry[*fakeDCPClient]{entries[1], entries[2]}
		pool.Entries.Store(&remaining)
		_ = e0.CloseNow()

		acquired, err := res.Acquire(context.Background())
		if err != nil {
			t.Fatalf("DCPResolver.Acquire after entry close failed: %v", err)
		}
		acquired.Release()
		if acquired == e0 {
			t.Fatal("expected DCPResolver.Acquire to re-pick a live entry after e0 was closed")
		}

		entries[1].State.Store(DCPStateDraining)
		entries[2].State.Store(DCPStateDraining)
		if _, err := pool.Pick(context.Background()); !errors.Is(err, customNoEntries) {
			t.Fatalf("expected customNoEntries when all entries are draining, got %v", err)
		}
	})
}

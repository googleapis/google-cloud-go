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
	"io"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	gtransport "google.golang.org/api/transport/grpc"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
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

	for _, in := range []DCPConfig{
		{},
		{DCPMinChannels: 6},
		{DCPInitialChannels: 1, DCPMinChannels: 1, DCPMaxChannels: 1},
		{DCPMaxRPCPerChannel: 2, DCPMinRPCPerChannel: 1},
		{DCPErrorPenaltyDuration: -1, DCPPrimeTimeout: time.Second, DCPSelectionStrategy: DCPRoundRobin},
	} {
		once, err := NormalizeDCPConfig(in)
		if err != nil {
			t.Fatalf("NormalizeDCPConfig(%+v) failed: %v", in, err)
		}
		twice, err := NormalizeDCPConfig(once)
		if err != nil {
			t.Fatalf("NormalizeDCPConfig of the normalized %+v failed: %v", in, err)
		}
		if twice != once {
			t.Fatalf("NormalizeDCPConfig changed a normalized config\n Got: %+v\nWant: %+v", twice, once)
		}
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

type fakeFixedUnderlyingPool struct {
	conns  []*grpc.ClientConn
	next   atomic.Int32
	closes atomic.Int32
}

func (f *fakeFixedUnderlyingPool) Conn() *grpc.ClientConn {
	if len(f.conns) == 0 {
		return nil
	}
	i := int(f.next.Add(1)-1) % len(f.conns)
	return f.conns[i]
}
func (f *fakeFixedUnderlyingPool) Num() int { return len(f.conns) }
func (f *fakeFixedUnderlyingPool) Close() error {
	f.closes.Add(1)
	return nil
}
func (f *fakeFixedUnderlyingPool) Invoke(context.Context, string, any, any, ...grpc.CallOption) error {
	return nil
}
func (f *fakeFixedUnderlyingPool) NewStream(context.Context, *grpc.StreamDesc, string, ...grpc.CallOption) (grpc.ClientStream, error) {
	return nil, errors.New("unused")
}

func TestNewFixedChannelPool(t *testing.T) {
	t.Run("rejects nil underlying or nil NewEntryClient", func(t *testing.T) {
		if _, err := NewFixedChannelPool[*fakeDCPClient](context.Background(), nil, DCPHooks[*fakeDCPClient]{
			NewEntryClient: func(context.Context, *DCPEntry[*fakeDCPClient]) error { return nil },
		}); err == nil {
			t.Fatal("expected error for nil underlying")
		}
		if _, err := NewFixedChannelPool[*fakeDCPClient](context.Background(), &fakeFixedUnderlyingPool{conns: []*grpc.ClientConn{nil}}, DCPHooks[*fakeDCPClient]{}); err == nil {
			t.Fatal("expected error for nil NewEntryClient")
		}
	})

	t.Run("deduplicates conns, skips background workers, and closes underlying once", func(t *testing.T) {
		c1 := new(grpc.ClientConn)
		c2 := new(grpc.ClientConn)
		underlying := &fakeFixedUnderlyingPool{conns: []*grpc.ClientConn{c1, c2, c1}}
		p, err := NewFixedChannelPool[*fakeDCPClient](context.Background(), underlying, DCPHooks[*fakeDCPClient]{
			NewEntryClient: func(_ context.Context, e *DCPEntry[*fakeDCPClient]) error {
				c := &fakeDCPClient{}
				e.Delegate = c
				e.Client = c
				return nil
			},
		})
		if err != nil {
			t.Fatalf("NewFixedChannelPool() failed: %v", err)
		}
		if !p.IsFixed() {
			t.Fatal("expected IsFixed() == true")
		}
		if p.Ctx != nil || p.cancel != nil || p.done != nil || p.ScaleUpSignal != nil || p.Metrics != nil {
			t.Fatalf("expected zero background state on fixed pool, got ctx=%v cancel=%v done=%v sig=%v metrics=%v",
				p.Ctx != nil, p.cancel != nil, p.done != nil, p.ScaleUpSignal != nil, p.Metrics != nil)
		}
		if got := p.Num(); got != 2 {
			t.Fatalf("Num() = %d, want 2 after deduplication", got)
		}
		p.SetPrimeSession("projects/p/instances/i/databases/d/sessions/s1")
		if got := p.PrimeSessionID(); got != "" {
			t.Fatalf("fixed pool SetPrimeSession stored %q, want empty", got)
		}
		// Picks round-robin without updating LastActivity.
		entries := p.GetEntries()
		for _, e := range entries {
			e.LastActivity.Store(0)
		}
		e1, err := p.Pick(context.Background())
		if err != nil {
			t.Fatalf("Pick 1 failed: %v", err)
		}
		e2, err := p.Pick(context.Background())
		if err != nil {
			t.Fatalf("Pick 2 failed: %v", err)
		}
		if e1.ID != 1 || e2.ID != 2 {
			t.Fatalf("round-robin IDs = (%d, %d), want (1, 2)", e1.ID, e2.ID)
		}
		for i, e := range entries {
			if e.LastActivity.Load() != 0 {
				t.Fatalf("entry %d LastActivity mutated in fixed mode", i)
			}
		}
		if err := p.Close(); err != nil {
			t.Fatalf("first Close failed: %v", err)
		}
		if err := p.Close(); err != nil {
			t.Fatalf("second Close failed: %v", err)
		}
		if got := underlying.closes.Load(); got != 1 {
			t.Fatalf("underlying Close count = %d, want 1", got)
		}
		if got := p.Num(); got != 2 {
			t.Fatalf("fixed pool Num() after Close = %d, want 2 (entries retained)", got)
		}
	})

	t.Run("wraps nil-Conn underlying with borrowedPool", func(t *testing.T) {
		underlying := &fakeFixedUnderlyingPool{conns: []*grpc.ClientConn{nil, nil}}
		p, err := NewFixedChannelPool[*fakeDCPClient](context.Background(), underlying, DCPHooks[*fakeDCPClient]{
			NewEntryClient: func(_ context.Context, e *DCPEntry[*fakeDCPClient]) error {
				c := &fakeDCPClient{}
				e.Delegate = c
				e.Client = c
				return nil
			},
		})
		if err != nil {
			t.Fatalf("NewFixedChannelPool(nil conns) failed: %v", err)
		}
		if got := p.Num(); got != 2 {
			t.Fatalf("Num() = %d, want 2", got)
		}
		if err := p.Invoke(context.Background(), "/test/Method", nil, nil); err != nil {
			t.Fatalf("Invoke on borrowedPool failed: %v", err)
		}
		if err := p.Close(); err != nil {
			t.Fatalf("Close failed: %v", err)
		}
		if got := underlying.closes.Load(); got != 1 {
			t.Fatalf("underlying Close count = %d, want 1", got)
		}
	})
}

type scriptedDCPConnPool struct {
	fakeDCPConnPool
	onCall  func() error
	recvErr error
}

func (p *scriptedDCPConnPool) Invoke(context.Context, string, any, any, ...grpc.CallOption) error {
	return p.onCall()
}

func (p *scriptedDCPConnPool) NewStream(context.Context, *grpc.StreamDesc, string, ...grpc.CallOption) (grpc.ClientStream, error) {
	if err := p.onCall(); err != nil {
		return nil, err
	}
	return scriptedClientStream{err: p.recvErr}, nil
}

type scriptedClientStream struct {
	grpc.ClientStream
	err error
}

func (s scriptedClientStream) RecvMsg(any) error { return s.err }

func TestDynamicChannelPoolPenaltyEpoch(t *testing.T) {
	ctx := context.Background()
	unavailable := status.Error(codes.Unavailable, "down")
	conn := &scriptedDCPConnPool{recvErr: unavailable}
	var epoch atomic.Uint64
	pool, err := NewDynamicChannelPool[*fakeDCPClient](
		ctx,
		DCPConfig{DCPEnabled: true, DCPInitialChannels: 1, DCPMinChannels: 1, DCPMaxChannels: 1},
		DCPHooks[*fakeDCPClient]{
			Dial: func(context.Context) (gtransport.ConnPool, error) { return conn, nil },
			NewEntryClient: func(_ context.Context, e *DCPEntry[*fakeDCPClient]) error {
				e.Client = &fakeDCPClient{}
				return nil
			},
			PrimeRPC:     func(context.Context, *DCPEntry[*fakeDCPClient], string) error { return nil },
			PenaltyEpoch: epoch.Load,
		},
	)
	if err != nil {
		t.Fatalf("NewDynamicChannelPool failed: %v", err)
	}
	defer pool.Close()
	e := pool.GetEntries()[0]

	for _, op := range []struct {
		name                string
		failWithUnavailable func(whileInFlight func())
	}{
		{"StartUnary", func(whileInFlight func()) {
			done := e.StartUnary(ctx)
			whileInFlight()
			done(unavailable)
		}},
		{"StartStream", func(whileInFlight func()) {
			ref := e.StartStream(ctx)
			whileInFlight()
			ref.Done(unavailable)
		}},
		{"Invoke", func(whileInFlight func()) {
			conn.onCall = func() error {
				whileInFlight()
				return unavailable
			}
			pool.Invoke(ctx, "/test/Method", nil, nil)
		}},
		{"NewStream", func(whileInFlight func()) {
			conn.onCall = func() error {
				whileInFlight()
				return unavailable
			}
			pool.NewStream(ctx, &grpc.StreamDesc{}, "/test/Method")
		}},
		{"RecvMsg", func(whileInFlight func()) {
			conn.onCall = func() error { return nil }
			stream, err := pool.NewStream(ctx, &grpc.StreamDesc{}, "/test/Method")
			if err != nil {
				t.Fatalf("NewStream failed: %v", err)
			}
			whileInFlight()
			stream.RecvMsg(nil)
		}},
	} {
		t.Run(op.name, func(t *testing.T) {
			op.failWithUnavailable(func() {})
			if got := e.CurrentPenalty(); got <= 0 {
				t.Fatalf("penalty after a failure within one epoch = %d, want > 0", got)
			}
			e.ClearErrorPenalty()

			op.failWithUnavailable(func() { epoch.Add(1) })
			if got := e.CurrentPenalty(); got != 0 {
				t.Fatalf("penalty after a failure across an epoch change = %d, want 0", got)
			}
			if got := pool.TotalPenaltyLoad.Load(); got != 0 {
				t.Fatalf("TotalPenaltyLoad = %d, want 0", got)
			}
			if got := pool.TotalRPCLoad.Load(); got != 0 {
				t.Fatalf("TotalRPCLoad = %d, want 0", got)
			}
		})
	}
}

func TestDynamicChannelPoolClearStaleErrorPenalties(t *testing.T) {
	unavailable := status.Error(codes.Unavailable, "down")
	var epoch atomic.Uint64
	pool, err := NewDynamicChannelPool[*fakeDCPClient](
		context.Background(),
		DCPConfig{DCPEnabled: true, DCPInitialChannels: 3, DCPMinChannels: 3, DCPMaxChannels: 3},
		DCPHooks[*fakeDCPClient]{
			Dial: func(context.Context) (gtransport.ConnPool, error) { return &fakeDCPConnPool{}, nil },
			NewEntryClient: func(_ context.Context, e *DCPEntry[*fakeDCPClient]) error {
				e.Client = &fakeDCPClient{}
				return nil
			},
			PrimeRPC:     func(context.Context, *DCPEntry[*fakeDCPClient], string) error { return nil },
			PenaltyEpoch: epoch.Load,
		},
	)
	if err != nil {
		t.Fatalf("NewDynamicChannelPool failed: %v", err)
	}
	defer pool.Close()
	step := pool.Cfg.DCPErrorPenaltyStep
	entries := pool.GetEntries()
	stale, renewed, current := entries[0], entries[1], entries[2]

	stale.ApplyErrorPenalty(unavailable)
	renewed.ApplyErrorPenalty(unavailable)
	epoch.Add(1)
	renewed.ApplyErrorPenalty(unavailable)
	current.ApplyErrorPenalty(unavailable)
	if got := renewed.CurrentPenalty(); got != step {
		t.Fatalf("penalty earned on top of one from an earlier epoch = %d, want %d", got, step)
	}

	pool.ClearStaleErrorPenalties()
	for _, tc := range []struct {
		name  string
		entry *DCPEntry[*fakeDCPClient]
		want  int32
	}{
		{"earned in an earlier epoch", stale, 0},
		{"earned in both epochs", renewed, step},
		{"earned in the current epoch", current, step},
	} {
		if got := tc.entry.CurrentPenalty(); got != tc.want {
			t.Errorf("penalty %s after ClearStaleErrorPenalties = %d, want %d", tc.name, got, tc.want)
		}
	}
	if got, want := pool.TotalPenaltyLoad.Load(), int64(2*step); got != want {
		t.Errorf("TotalPenaltyLoad = %d, want %d", got, want)
	}

	current.ApplyErrorPenalty(unavailable)
	if got, want := current.CurrentPenalty(), 2*step; got != want {
		t.Errorf("penalty after two failures in one epoch = %d, want %d", got, want)
	}
}

func TestDynamicChannelPoolOnClose(t *testing.T) {
	newEntryClient := func(_ context.Context, e *DCPEntry[*fakeDCPClient]) error {
		e.Client = &fakeDCPClient{}
		return nil
	}
	for _, tc := range []struct {
		name string
		new  func(hooks DCPHooks[*fakeDCPClient]) (*DynamicChannelPool[*fakeDCPClient], error)
	}{
		{"dynamic", func(hooks DCPHooks[*fakeDCPClient]) (*DynamicChannelPool[*fakeDCPClient], error) {
			hooks.Dial = func(context.Context) (gtransport.ConnPool, error) { return &fakeDCPConnPool{}, nil }
			hooks.PrimeRPC = func(context.Context, *DCPEntry[*fakeDCPClient], string) error { return nil }
			return NewDynamicChannelPool(context.Background(), DCPConfig{DCPEnabled: true}, hooks)
		}},
		{"fixed", func(hooks DCPHooks[*fakeDCPClient]) (*DynamicChannelPool[*fakeDCPClient], error) {
			underlying := &fakeFixedUnderlyingPool{conns: []*grpc.ClientConn{new(grpc.ClientConn)}}
			return NewFixedChannelPool(context.Background(), underlying, hooks)
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var closes atomic.Int32
			p, err := tc.new(DCPHooks[*fakeDCPClient]{
				NewEntryClient: newEntryClient,
				OnClose:        func() { closes.Add(1) },
			})
			if err != nil {
				t.Fatalf("creating the pool failed: %v", err)
			}
			if got := closes.Load(); got != 0 {
				t.Fatalf("OnClose calls before Close = %d, want 0", got)
			}
			p.Close()
			p.Close()
			if got := closes.Load(); got != 1 {
				t.Fatalf("OnClose calls after two Close calls = %d, want 1", got)
			}
		})
	}
}

func TestDynamicChannelPoolOnCallStart(t *testing.T) {
	ctx := context.Background()
	var loads []int32
	pool, err := NewDynamicChannelPool[*fakeDCPClient](
		ctx,
		DCPConfig{DCPEnabled: true, DCPInitialChannels: 1, DCPMinChannels: 1, DCPMaxChannels: 1},
		DCPHooks[*fakeDCPClient]{
			Dial: func(context.Context) (gtransport.ConnPool, error) {
				return &scriptedDCPConnPool{onCall: func() error { return nil }}, nil
			},
			NewEntryClient: func(_ context.Context, e *DCPEntry[*fakeDCPClient]) error {
				e.Client = &fakeDCPClient{}
				return nil
			},
			PrimeRPC:    func(context.Context, *DCPEntry[*fakeDCPClient], string) error { return nil },
			OnCallStart: func(_ context.Context, load int32) { loads = append(loads, load) },
		},
	)
	if err != nil {
		t.Fatalf("NewDynamicChannelPool failed: %v", err)
	}
	defer pool.Close()
	e := pool.GetEntries()[0]

	for _, op := range []struct {
		name  string
		start func()
	}{
		{"Invoke", func() { pool.Invoke(ctx, "/test/Method", nil, nil) }},
		{"NewStream", func() {
			stream, err := pool.NewStream(ctx, &grpc.StreamDesc{}, "/test/Method")
			if err != nil {
				t.Fatalf("NewStream failed: %v", err)
			}
			stream.RecvMsg(nil)
		}},
		{"StartUnary", func() { e.StartUnary(ctx)(nil) }},
		{"StartStream", func() { e.StartStream(ctx).Done(nil) }},
	} {
		t.Run(op.name, func(t *testing.T) {
			loads = nil
			e.UnaryLoad.Store(1)
			e.StreamLoad.Store(1)
			defer func() {
				e.UnaryLoad.Store(0)
				e.StreamLoad.Store(0)
			}()
			op.start()
			if len(loads) != 1 || loads[0] != 3 {
				t.Fatalf("OnCallStart loads mismatch:\n Got: %v\nWant: [3]", loads)
			}
		})
	}
}

// callCountingFixedPool is the underlying pool of a fixed pool whose entries
// route their calls to it.
type callCountingFixedPool struct {
	fakeFixedUnderlyingPool
	invokeErr error
	streamErr error
	stream    *scriptedFixedStream
	duringRPC func()
}

func newCallCountingFixedPool(n int) *callCountingFixedPool {
	return &callCountingFixedPool{fakeFixedUnderlyingPool: fakeFixedUnderlyingPool{conns: make([]*grpc.ClientConn, n)}}
}

func (f *callCountingFixedPool) Invoke(context.Context, string, any, any, ...grpc.CallOption) error {
	if f.duringRPC != nil {
		f.duringRPC()
	}
	return f.invokeErr
}

func (f *callCountingFixedPool) NewStream(context.Context, *grpc.StreamDesc, string, ...grpc.CallOption) (grpc.ClientStream, error) {
	if f.duringRPC != nil {
		f.duringRPC()
	}
	if f.streamErr != nil {
		return nil, f.streamErr
	}
	if f.stream != nil {
		return f.stream, nil
	}
	return &scriptedFixedStream{recvErr: io.EOF}, nil
}

type scriptedFixedStream struct {
	grpc.ClientStream
	headerErr, sendErr, recvErr, closeSendErr error
}

func (s *scriptedFixedStream) Header() (metadata.MD, error) { return nil, s.headerErr }
func (s *scriptedFixedStream) SendMsg(any) error            { return s.sendErr }
func (s *scriptedFixedStream) RecvMsg(any) error            { return s.recvErr }
func (s *scriptedFixedStream) CloseSend() error             { return s.closeSendErr }

func newCallCountingFixedChannelPool(t *testing.T, underlying gtransport.ConnPool, onCallStart func(context.Context, int32)) *DynamicChannelPool[*fakeDCPClient] {
	t.Helper()
	pool, err := NewFixedChannelPool(context.Background(), underlying, DCPHooks[*fakeDCPClient]{
		NewEntryClient: func(_ context.Context, e *DCPEntry[*fakeDCPClient]) error {
			e.Client = &fakeDCPClient{}
			return nil
		},
		OnCallStart: onCallStart,
	})
	if err != nil {
		t.Fatalf("NewFixedChannelPool failed: %v", err)
	}
	t.Cleanup(func() { pool.Close() })
	return pool
}

func TestFixedChannelPoolOnCallStart(t *testing.T) {
	ctx := context.Background()
	var loads []int32
	underlying := newCallCountingFixedPool(1)
	pool := newCallCountingFixedChannelPool(t, underlying, func(_ context.Context, load int32) { loads = append(loads, load) })
	e := pool.GetEntries()[0]
	unavailable := status.Error(codes.Unavailable, "unavailable")

	for _, op := range []struct {
		name  string
		start func() error
	}{
		{"pool Invoke", func() error { return pool.Invoke(ctx, "/test/Method", nil, nil) }},
		{"pool NewStream", func() error {
			stream, err := pool.NewStream(ctx, &grpc.StreamDesc{}, "/test/Method")
			if err != nil {
				return err
			}
			return stream.RecvMsg(nil)
		}},
		{"entry Invoke", func() error { return e.Invoke(ctx, "/test/Method", nil, nil) }},
		{"entry NewStream", func() error {
			stream, err := e.NewStream(ctx, &grpc.StreamDesc{}, "/test/Method")
			if err != nil {
				return err
			}
			return stream.RecvMsg(nil)
		}},
	} {
		t.Run(op.name, func(t *testing.T) {
			loads = nil
			var loadDuringRPC int32
			underlying.duringRPC = func() { loadDuringRPC = e.RPCLoad() }
			underlying.invokeErr = unavailable
			underlying.stream = &scriptedFixedStream{recvErr: unavailable}
			e.UnaryLoad.Store(1)
			e.StreamLoad.Store(1)
			if err := op.start(); status.Code(err) != codes.Unavailable {
				t.Fatalf("call error mismatch:\n Got: %v\nWant: Unavailable", err)
			}
			if len(loads) != 1 || loads[0] != 3 {
				t.Fatalf("OnCallStart loads mismatch:\n Got: %v\nWant: [3]", loads)
			}
			if loadDuringRPC != 3 {
				t.Fatalf("entry load during the call mismatch:\n Got: %d\nWant: 3", loadDuringRPC)
			}
			if got := e.RPCLoad(); got != 2 {
				t.Fatalf("entry load after the call mismatch:\n Got: %d\nWant: 2", got)
			}
			if got := pool.TotalRPCLoad.Load(); got != 0 {
				t.Fatalf("pool load mismatch:\n Got: %d\nWant: 0", got)
			}
			if got := e.CurrentPenalty(); got != 0 {
				t.Fatalf("error penalty mismatch:\n Got: %d\nWant: 0", got)
			}
			if got := e.LastActivity.Load(); got != e.CreatedAt.Load() {
				t.Fatalf("activity time changed by the call:\n Got: %d\nWant: %d", got, e.CreatedAt.Load())
			}
		})
	}
	if pool.ScaleUpSignal != nil {
		t.Fatal("fixed pool has a scale-up signal")
	}
}

func TestFixedChannelPoolWithoutOnCallStartCountsNothing(t *testing.T) {
	ctx := context.Background()
	underlying := newCallCountingFixedPool(1)
	pool := newCallCountingFixedChannelPool(t, underlying, nil)
	e := pool.GetEntries()[0]
	underlying.duringRPC = func() {
		if got := e.RPCLoad(); got != 0 {
			t.Errorf("entry load during the call mismatch:\n Got: %d\nWant: 0", got)
		}
	}
	if err := pool.Invoke(ctx, "/test/Method", nil, nil); err != nil {
		t.Fatalf("Invoke failed: %v", err)
	}
	stream, err := pool.NewStream(ctx, &grpc.StreamDesc{}, "/test/Method")
	if err != nil {
		t.Fatalf("NewStream failed: %v", err)
	}
	if _, ok := stream.(*scriptedFixedStream); !ok {
		t.Fatalf("stream type mismatch:\n Got: %T\nWant: *scriptedFixedStream", stream)
	}
	if err := e.Invoke(ctx, "/test/Method", nil, nil); err != nil {
		t.Fatalf("entry Invoke failed: %v", err)
	}
	if _, err := e.NewStream(ctx, &grpc.StreamDesc{}, "/test/Method"); err != nil {
		t.Fatalf("entry NewStream failed: %v", err)
	}
}

func TestFixedChannelPoolCountedStreamEnds(t *testing.T) {
	failed := errors.New("failed")
	for _, tc := range []struct {
		name      string
		stream    *scriptedFixedStream
		streamErr error
		end       func(s grpc.ClientStream, cancel context.CancelFunc)
		async     bool
	}{
		{name: "RecvMsg returns io.EOF", stream: &scriptedFixedStream{recvErr: io.EOF}, end: func(s grpc.ClientStream, _ context.CancelFunc) { s.RecvMsg(nil) }},
		{name: "RecvMsg fails", stream: &scriptedFixedStream{recvErr: failed}, end: func(s grpc.ClientStream, _ context.CancelFunc) { s.RecvMsg(nil) }},
		{name: "SendMsg fails", stream: &scriptedFixedStream{sendErr: io.EOF}, end: func(s grpc.ClientStream, _ context.CancelFunc) { s.SendMsg(nil) }},
		{name: "CloseSend fails", stream: &scriptedFixedStream{closeSendErr: failed}, end: func(s grpc.ClientStream, _ context.CancelFunc) { s.CloseSend() }},
		{name: "Header fails", stream: &scriptedFixedStream{headerErr: failed}, end: func(s grpc.ClientStream, _ context.CancelFunc) { s.Header() }},
		{name: "context canceled", stream: &scriptedFixedStream{}, end: func(_ grpc.ClientStream, cancel context.CancelFunc) { cancel() }, async: true},
		{name: "NewStream fails", streamErr: failed},
	} {
		t.Run(tc.name, func(t *testing.T) {
			underlying := newCallCountingFixedPool(1)
			underlying.stream = tc.stream
			underlying.streamErr = tc.streamErr
			pool := newCallCountingFixedChannelPool(t, underlying, func(context.Context, int32) {})
			e := pool.GetEntries()[0]
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			s, err := pool.NewStream(ctx, &grpc.StreamDesc{ServerStreams: true}, "/test/Method")
			if tc.streamErr != nil {
				if !errors.Is(err, tc.streamErr) {
					t.Fatalf("NewStream error mismatch:\n Got: %v\nWant: %v", err, tc.streamErr)
				}
				if got := e.StreamLoad.Load(); got != 0 {
					t.Fatalf("stream load after a failed NewStream mismatch:\n Got: %d\nWant: 0", got)
				}
				return
			}
			if err != nil {
				t.Fatalf("NewStream failed: %v", err)
			}
			if got := e.StreamLoad.Load(); got != 1 {
				t.Fatalf("stream load while open mismatch:\n Got: %d\nWant: 1", got)
			}
			tc.end(s, cancel)
			if tc.async {
				waitForStreamLoad(t, e, 0)
			} else if got := e.StreamLoad.Load(); got != 0 {
				t.Fatalf("stream load after the stream ended mismatch:\n Got: %d\nWant: 0", got)
			}
			if s.(*dcpCountedStream[*fakeDCPClient]).stop() {
				t.Fatal("context callback still registered after the stream ended")
			}
			s.RecvMsg(nil)
			s.CloseSend()
			if got := e.StreamLoad.Load(); got != 0 {
				t.Fatalf("stream load after more ends mismatch:\n Got: %d\nWant: 0", got)
			}
		})
	}
}

func waitForStreamLoad(t *testing.T, e *DCPEntry[*fakeDCPClient], want int32) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for e.StreamLoad.Load() != want {
		if time.Now().After(deadline) {
			t.Fatalf("stream load mismatch:\n Got: %d\nWant: %d", e.StreamLoad.Load(), want)
		}
		time.Sleep(time.Millisecond)
	}
}

func TestFixedChannelPoolOnCallStartConcurrent(t *testing.T) {
	ctx := context.Background()
	var calls atomic.Int32
	var maxLoad atomic.Int32
	pool := newCallCountingFixedChannelPool(t, newCallCountingFixedPool(2), func(_ context.Context, load int32) {
		calls.Add(1)
		for {
			cur := maxLoad.Load()
			if load <= cur || maxLoad.CompareAndSwap(cur, load) {
				return
			}
		}
	})

	const goroutines, perGoroutine = 8, 100
	var wg sync.WaitGroup
	for i := 0; i < goroutines; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < perGoroutine; j++ {
				if err := pool.Invoke(ctx, "/test/Method", nil, nil); err != nil {
					t.Errorf("Invoke failed: %v", err)
				}
				if s, err := pool.NewStream(ctx, &grpc.StreamDesc{}, "/test/Method"); err == nil {
					s.RecvMsg(nil)
				}
			}
		}()
	}
	wg.Wait()
	if got, want := calls.Load(), int32(2*goroutines*perGoroutine); got != want {
		t.Fatalf("OnCallStart calls mismatch:\n Got: %d\nWant: %d", got, want)
	}
	if got := maxLoad.Load(); got < 1 || got > goroutines {
		t.Fatalf("OnCallStart max load mismatch:\n Got: %d\nWant: 1..%d", got, goroutines)
	}
	for i, e := range pool.GetEntries() {
		if got := e.RPCLoad(); got != 0 {
			t.Fatalf("entry %d load after all calls mismatch:\n Got: %d\nWant: 0", i, got)
		}
	}
}

func TestFixedChannelPoolCountedInvokeAllocations(t *testing.T) {
	ctx := context.Background()
	for _, tc := range []struct {
		name        string
		onCallStart func(context.Context, int32)
	}{
		{"without OnCallStart", nil},
		{"with OnCallStart", func(context.Context, int32) {}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			pool := newCallCountingFixedChannelPool(t, newCallCountingFixedPool(2), tc.onCallStart)
			if allocs := testing.AllocsPerRun(100, func() { pool.Invoke(ctx, "/test/Method", nil, nil) }); allocs != 0 {
				t.Fatalf("allocations per Invoke mismatch:\n Got: %v\nWant: 0", allocs)
			}
		})
	}
}

func TestDynamicChannelPoolOnCallStartConcurrent(t *testing.T) {
	ctx := context.Background()
	var calls atomic.Int32
	var maxLoad atomic.Int32
	pool, err := NewDynamicChannelPool[*fakeDCPClient](
		ctx,
		DCPConfig{DCPEnabled: true, DCPInitialChannels: 2, DCPMinChannels: 2, DCPMaxChannels: 2},
		DCPHooks[*fakeDCPClient]{
			Dial: func(context.Context) (gtransport.ConnPool, error) {
				return &scriptedDCPConnPool{onCall: func() error { return nil }}, nil
			},
			NewEntryClient: func(_ context.Context, e *DCPEntry[*fakeDCPClient]) error {
				e.Client = &fakeDCPClient{}
				return nil
			},
			PrimeRPC: func(context.Context, *DCPEntry[*fakeDCPClient], string) error { return nil },
			OnCallStart: func(_ context.Context, load int32) {
				calls.Add(1)
				for {
					cur := maxLoad.Load()
					if load <= cur || maxLoad.CompareAndSwap(cur, load) {
						return
					}
				}
			},
		},
	)
	if err != nil {
		t.Fatalf("NewDynamicChannelPool failed: %v", err)
	}
	defer pool.Close()
	entries := pool.GetEntries()

	const goroutines, perGoroutine = 8, 100
	done := make(chan struct{})
	for i := 0; i < goroutines; i++ {
		e := entries[i%len(entries)]
		go func() {
			defer func() { done <- struct{}{} }()
			for j := 0; j < perGoroutine; j++ {
				e.StartUnary(ctx)(nil)
			}
		}()
	}
	for i := 0; i < goroutines; i++ {
		<-done
	}
	if got := calls.Load(); got != goroutines*perGoroutine {
		t.Fatalf("OnCallStart calls mismatch:\n Got: %d\nWant: %d", got, goroutines*perGoroutine)
	}
	if got, want := maxLoad.Load(), int32(goroutines/len(entries)); got < 1 || got > want {
		t.Fatalf("OnCallStart max load mismatch:\n Got: %d\nWant: 1..%d", got, want)
	}
}

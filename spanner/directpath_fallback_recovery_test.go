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
	"errors"
	"fmt"
	"io"
	"log"
	"net/url"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"cloud.google.com/go/spanner/internal"
	. "cloud.google.com/go/spanner/internal/testutil"
	"cloud.google.com/go/spanner/internal/transport"
	"go.opentelemetry.io/otel/metric"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/metric/metricdata"
	gtransport "google.golang.org/api/transport/grpc"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials/alts"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/peer"
	"google.golang.org/grpc/status"
)

const (
	getSessionMethod    = "/google.spanner.v1.Spanner/GetSession"
	recoveryTestDB      = "projects/p/instances/i/databases/d"
	recoveryTestSess    = recoveryTestDB + "/sessions/s1"
	recoveryProbeIntv   = 5 * time.Millisecond
	recoveryProbeMinDur = 10 * time.Millisecond
)

// stubPool is an in-process ConnPool that counts calls and captures GetSession probe metadata.
type stubPool struct {
	closeCountingPool
	conn  *grpc.ClientConn
	calls atomic.Int32

	mu       sync.Mutex
	probeMDs []metadata.MD
	onInvoke func(context.Context, string) error
}

func newStubPool() *stubPool               { return &stubPool{conn: new(grpc.ClientConn)} }
func (p *stubPool) Conn() *grpc.ClientConn { return p.conn }
func (p *stubPool) setInvoke(fn func(context.Context, string) error) {
	p.mu.Lock()
	p.onInvoke = fn
	p.mu.Unlock()
}
func (p *stubPool) setErr(err error) {
	if err == nil {
		p.setInvoke(nil)
		return
	}
	p.setInvoke(func(context.Context, string) error { return err })
}
func (p *stubPool) Invoke(ctx context.Context, method string, _, _ any, _ ...grpc.CallOption) error {
	p.calls.Add(1)
	p.mu.Lock()
	if method == getSessionMethod {
		if md, ok := metadata.FromOutgoingContext(ctx); ok {
			p.probeMDs = append(p.probeMDs, md.Copy())
		}
	}
	fn := p.onInvoke
	p.mu.Unlock()
	if fn != nil {
		return fn(ctx, method)
	}
	return nil
}

func setRecoveryTestPeerCheck(t *testing.T, fn func(*peer.Peer) bool) {
	t.Helper()
	old := isDirectPathPeerFn
	isDirectPathPeerFn = fn
	t.Cleanup(func() { isDirectPathPeerFn = old })
}

func newTestRecoveryFallback(t *testing.T, mp metric.MeterProvider, dialCloud func(context.Context) (gtransport.ConnPool, error), interval time.Duration, minProbes int, minDur time.Duration) *directPathFallback {
	t.Helper()
	f := newDirectPathFallback(mp, log.New(io.Discard, "", 0), dialCloud, time.Hour, directPathFallbackRecoveryConfig{
		probeMD:             newDirectPathProbeMetadata(recoveryTestDB, false),
		enabled:             true,
		probeInterval:       interval,
		probeRPCTimeout:     2 * time.Second,
		minRecoveryProbes:   minProbes,
		minRecoveryDuration: minDur,
	})
	f.setProbeSession(recoveryTestSess)
	t.Cleanup(f.close)
	return f
}

func stubCloudDial(cloud *stubPool) func(context.Context) (gtransport.ConnPool, error) {
	return func(context.Context) (gtransport.ConnPool, error) { return borrowedPool{cloud}, nil }
}

func newStubSlot(t *testing.T, f *directPathFallback) (*stubPool, *channelSlot) {
	t.Helper()
	direct := newStubPool()
	slot := newChannelSlot(direct, f)
	t.Cleanup(func() { slot.Close() })
	return direct, slot
}

func stubInvoke(t *testing.T, slot *channelSlot) {
	t.Helper()
	if err := slot.Invoke(context.Background(), createSessionMethod, nil, nil); err != nil {
		t.Fatalf("slot.Invoke() failed: %v", err)
	}
}

func hasProbeLoop(slot *channelSlot) bool {
	slot.mu.Lock()
	defer slot.mu.Unlock()
	return slot.probeCancel != nil
}

func blockUntilDone(started chan<- struct{}) func(context.Context) error {
	return func(ctx context.Context) error {
		select {
		case started <- struct{}{}:
		default:
		}
		<-ctx.Done()
		return ctx.Err()
	}
}

func assertCloseUnblocks(t *testing.T, started <-chan struct{}, slot *channelSlot) {
	t.Helper()
	select {
	case <-started:
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for blocked operation to start")
	}
	done := make(chan error, 1)
	go func() { done <- slot.Close() }()
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("slot.Close() failed: %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("slot.Close() blocked while operation was in flight")
	}
}

func TestDirectPathFallbackRecoveryEnvGateAndPeriod(t *testing.T) {
	skipDialectRerun(t)
	if directPathFallbackPeriod != 3*time.Minute {
		t.Fatalf("directPathFallbackPeriod = %v, want 3m", directPathFallbackPeriod)
	}
	for _, tt := range []struct {
		val  string
		want bool
	}{
		{"", false},
		{"not-a-bool", false},
		{"true", true},
	} {
		t.Setenv(directPathFallbackRecoveryEnv, tt.val)
		f := newPoolFallback(true, recoveryTestDB, ClientConfig{Logger: log.New(io.Discard, "", 0)}, transport.PoolConfig{}, nil, func(gtransport.ConnPool) bool { return false })
		f.close()
		if isDirectPathFallbackRecoveryEnabled() != tt.want || f.recovery.enabled != tt.want || f.period != directPathFallbackPeriod {
			t.Fatalf("env=%q: got enabled=%v period=%v, want %v %v", tt.val, f.recovery.enabled, f.period, tt.want, directPathFallbackPeriod)
		}
	}

	f := newDirectPathFallback(nil, log.New(io.Discard, "", 0), nil, time.Hour, directPathFallbackRecoveryConfig{})
	t.Cleanup(f.close)
	slot := newChannelSlot(nil, f)
	t.Cleanup(func() { slot.Close() })
	f.setProbeSession(recoveryTestSess)
	forceDirectPathFallback(t, f)
	if sid, _ := f.probeSession.Load().(string); sid != "" || !slot.isInFallback() || hasProbeLoop(slot) || len(f.slots) != 0 {
		t.Fatalf("disabled recovery state: sid=%q inFallback=%v loop=%v slots=%d", sid, slot.isInFallback(), hasProbeLoop(slot), len(f.slots))
	}
}

func TestDirectPathFallbackRecoveryThresholdsAndJitter(t *testing.T) {
	skipDialectRerun(t)
	setRecoveryTestPeerCheck(t, func(*peer.Peer) bool { return true })

	t.Run("startup jitter and exact Nth-probe fencepost after initial failure", func(t *testing.T) {
		const (
			interval  = 15 * time.Millisecond
			jitter    = 25 * time.Millisecond
			minProbes = 3
		)
		var jitterCalls, probeCalls atomic.Int32
		var firstProbeAt atomic.Int64
		f := newTestRecoveryFallback(t, nil, nil, interval, minProbes, 0)
		f.recovery.probeJitter = func(time.Duration) time.Duration {
			jitterCalls.Add(1)
			return jitter
		}
		direct, slot := newStubSlot(t, f)
		direct.setInvoke(func(context.Context, string) error {
			firstProbeAt.CompareAndSwap(0, time.Now().UnixNano())
			if probeCalls.Add(1) == 1 {
				return status.Error(codes.Unavailable, "initial failure")
			}
			return nil
		})
		started := time.Now()
		forceDirectPathFallback(t, f)
		waitFor(t, func() error {
			if slot.isInFallback() {
				return errors.New("waiting for recovery")
			}
			return nil
		})
		if jitterCalls.Load() != 1 || probeCalls.Load() != minProbes+1 {
			t.Fatalf("jitterCalls=%d probes=%d, want 1 and %d", jitterCalls.Load(), probeCalls.Load(), minProbes+1)
		}
		if delay := time.Unix(0, firstProbeAt.Load()).Sub(started); delay < interval+jitter-5*time.Millisecond {
			t.Fatalf("first probe delay = %v, want >= %v", delay, interval+jitter-5*time.Millisecond)
		}
	})

	t.Run("duration boundary anchors at first passing probe and blocks until minDur elapses", func(t *testing.T) {
		const minDur = time.Hour
		f := newTestRecoveryFallback(t, nil, nil, time.Hour, 2, minDur)
		direct, slot := newStubSlot(t, f)
		forceDirectPathFallback(t, f)

		slot.mu.Lock()
		startBeforePass := slot.probeStreakStart
		slot.mu.Unlock()
		if !startBeforePass.IsZero() {
			t.Fatalf("probeStreakStart after enterFallback = %v, want zero", startBeforePass)
		}

		direct.setErr(status.Error(codes.Unavailable, "still down"))
		slot.probeOnce(context.Background())
		slot.mu.Lock()
		startAfterFail := slot.probeStreakStart
		slot.mu.Unlock()
		if !startAfterFail.IsZero() {
			t.Fatalf("probeStreakStart after failure = %v, want zero", startAfterFail)
		}

		direct.setErr(nil)
		for range 3 {
			slot.probeOnce(context.Background())
		}
		if !slot.isInFallback() {
			t.Fatal("slot recovered before minRecoveryDuration elapsed")
		}
		slot.mu.Lock()
		if slot.probeStreakStart.IsZero() {
			slot.mu.Unlock()
			t.Fatal("probeStreakStart was not anchored on first passing probe")
		}
		slot.probeStreakStart = time.Now().Add(-minDur - time.Second)
		slot.mu.Unlock()
		slot.probeOnce(context.Background())
		if slot.isInFallback() {
			t.Fatal("slot did not recover once both minProbes and minRecoveryDuration were met")
		}
	})
}

func TestDirectPathFallbackRecoveryResetsPostRecoveryWindow(t *testing.T) {
	skipDialectRerun(t)
	const period = 200 * time.Millisecond
	f := newDirectPathFallback(nil, log.New(io.Discard, "", 0), nil, period, directPathFallbackRecoveryConfig{enabled: true, probeInterval: time.Hour})
	t.Cleanup(f.close)

	forceDirectPathFallback(t, f)
	time.Sleep(150 * time.Millisecond)

	var reFallbackAt atomic.Int64
	f.addSwitchHook(func() {
		reFallbackAt.CompareAndSwap(0, time.Now().UnixNano())
	})
	allowed, reset := f.tryRecoverSlot(nil, f.current().generation)
	recoveredAt := time.Now()
	if !allowed || !reset || f.current().fallback {
		t.Fatalf("tryRecoverSlot() = (%v, %v), fallback=%v; want (true, true), false", allowed, reset, f.current().fallback)
	}

	f.recordSlotAttempt(context.Background(), f.current(), false, status.Error(codes.Unavailable, "transient"))
	if res := f.closeWindow(f.current().start.Add(-time.Millisecond)); res.switched || f.current().fallback {
		t.Fatalf("closeWindow before w.start switched: res=%+v fallback=%v", res, f.current().fallback)
	}

	// The post-recovery window must receive a full period rather than the ~50ms remaining from the prior tick.
	waitFor(t, func() error {
		if !f.current().fallback || reFallbackAt.Load() == 0 {
			return errors.New("waiting for full post-recovery window to close")
		}
		return nil
	})
	if elapsed := time.Unix(0, reFallbackAt.Load()).Sub(recoveredAt); elapsed < period-15*time.Millisecond {
		t.Fatalf("post-recovery window closed after %v, want >= %v", elapsed, period-15*time.Millisecond)
	}
}

func TestDirectPathFallbackRecoveryProbeFailures(t *testing.T) {
	skipDialectRerun(t)
	var altsOK atomic.Bool
	setRecoveryTestPeerCheck(t, func(*peer.Peer) bool { return altsOK.Load() })

	f := newTestRecoveryFallback(t, nil, nil, time.Hour, 3, 0)
	f.probeSession.Store("")
	direct, slot := newStubSlot(t, f)

	forceDirectPathFallback(t, f)
	fallbackGen := f.current().generation

	slot.probeOnce(context.Background())
	if !slot.isInFallback() {
		t.Fatal("slot left fallback without a session ID")
	}

	f.setProbeSession(recoveryTestSess)
	altsOK.Store(true)
	for _, applyFault := range []func(bool){
		func(on bool) { altsOK.Store(!on) },
		func(on bool) {
			if on {
				direct.setErr(status.Error(codes.Unavailable, "transient"))
			} else {
				direct.setErr(nil)
			}
		},
	} {
		slot.probeOnce(context.Background())
		slot.probeOnce(context.Background())
		if !slot.isInFallback() {
			t.Fatal("slot recovered before minRecoveryProbes")
		}
		applyFault(true)
		slot.probeOnce(context.Background())
		if !slot.isInFallback() {
			t.Fatal("slot recovered on failing probe")
		}
		applyFault(false)
	}
	slot.probeOnce(context.Background())
	slot.probeOnce(context.Background())
	if !slot.isInFallback() {
		t.Fatal("slot recovered before 3 consecutive successes after reset")
	}
	slot.probeOnce(context.Background())
	if slot.isInFallback() || f.current().fallback {
		t.Fatal("slot did not recover on 3rd consecutive success")
	}

	slot.enterFallback(fallbackGen)
	if slot.isInFallback() {
		t.Fatal("stale enterFallback(oldGen) re-entered fallback on recovered slot")
	}
}

func TestDirectPathFallbackRecoveryProbeHeaders(t *testing.T) {
	skipDialectRerun(t)
	setRecoveryTestPeerCheck(t, func(*peer.Peer) bool { return true })
	t.Setenv(directPathFallbackRecoveryEnv, "true")

	for _, disableLeader := range []bool{false, true} {
		f := newPoolFallback(true, recoveryTestDB, ClientConfig{
			DisableRouteToLeader: disableLeader,
			Logger:               log.New(io.Discard, "", 0),
		}, transport.PoolConfig{}, nil, func(gtransport.ConnPool) bool { return false })
		f.recovery.probeInterval, f.recovery.minRecoveryProbes, f.recovery.minRecoveryDuration = time.Hour, 1, 0
		f.setProbeSession(recoveryTestSess)
		direct, slot := newStubSlot(t, f)
		forceDirectPathFallback(t, f)
		slot.probeOnce(context.Background())
		f.close()

		if len(direct.probeMDs) != 1 {
			t.Fatalf("disableLeader=%v: captured probe MDs = %d, want 1", disableLeader, len(direct.probeMDs))
		}
		md := direct.probeMDs[0]
		if got := md.Get(resourcePrefixHeader); len(got) != 1 || got[0] != recoveryTestDB {
			t.Fatalf("%s = %v, want [%q]", resourcePrefixHeader, got, recoveryTestDB)
		}
		if got := md.Get(routeToLeaderHeader); (disableLeader && len(got) != 0) || (!disableLeader && (len(got) != 1 || got[0] != "true")) {
			t.Fatalf("disableLeader=%v: %s = %v", disableLeader, routeToLeaderHeader, got)
		}
		if got, want := md.Get("x-goog-request-params"), "name="+url.QueryEscape(recoveryTestSess); len(got) != 1 || got[0] != want {
			t.Fatalf("x-goog-request-params = %v, want [%q]", got, want)
		}
		if got := md.Get("x-goog-api-client"); len(got) != 1 || !strings.Contains(got[0], "gl-go/") || !strings.Contains(got[0], "gccl/"+internal.Version) {
			t.Fatalf("x-goog-api-client = %v, want gl-go/ and gccl/%s", got, internal.Version)
		}
	}
}

func TestDirectPathFallbackRecoveryMultiSlotAndReFallback(t *testing.T) {
	skipDialectRerun(t)
	setRecoveryTestPeerCheck(t, func(*peer.Peer) bool { return true })

	reader := sdkmetric.NewManualReader()
	provider := sdkmetric.NewMeterProvider(sdkmetric.WithReader(reader))
	t.Cleanup(func() { provider.Shutdown(context.Background()) })
	cloud := newStubPool()
	f := newTestRecoveryFallback(t, provider, stubCloudDial(cloud), time.Hour, 3, 0)

	var switchHooks atomic.Int32
	f.addSwitchHook(func() { switchHooks.Add(1) })
	direct1, slot1 := newStubSlot(t, f)
	direct2, slot2 := newStubSlot(t, f)
	direct2.setErr(status.Error(codes.Unavailable, "slot2 down"))

	forceDirectPathFallback(t, f)
	for range 3 {
		slot2.probeOnce(context.Background())
		slot1.probeOnce(context.Background())
	}
	if slot1.isInFallback() || f.current().fallback || !slot2.isInFallback() || switchHooks.Load() != 2 {
		t.Fatalf("partial recovery: slot1=%v slot2=%v window=%v hooks=%d",
			slot1.isInFallback(), slot2.isInFallback(), f.current().fallback, switchHooks.Load())
	}

	stubInvoke(t, slot2)
	if cloud.calls.Load() != 1 || slot2.Conn() != cloud.Conn() {
		t.Fatalf("slot2 CloudPath calls=%d connMatch=%v", cloud.calls.Load(), slot2.Conn() == cloud.Conn())
	}
	cloud.setErr(status.Error(codes.Unavailable, "cloudpath blip"))
	if err := slot2.Invoke(context.Background(), createSessionMethod, nil, nil); status.Code(err) != codes.Unavailable {
		t.Fatalf("slot2 CloudPath error = %v, want Unavailable", err)
	}
	cloud.setErr(nil)
	if failures, successes := unpackFallbackOutcomes(f.current().outcomes.Load()); failures != 0 || successes != 0 {
		t.Fatalf("CloudPath calls polluted DirectPath window: (%d, %d)", failures, successes)
	}

	direct1Before := direct1.calls.Load()
	stubInvoke(t, slot1)
	if failures, successes := unpackFallbackOutcomes(f.current().outcomes.Load()); direct1.calls.Load() != direct1Before+1 || failures != 0 || successes != 1 {
		t.Fatalf("slot1 DirectPath calls=%d outcomes=(%d, %d)", direct1.calls.Load()-direct1Before, failures, successes)
	}

	direct2.setErr(nil)
	slot2.probeOnce(context.Background())
	slot2.probeOnce(context.Background())
	if !slot2.isInFallback() {
		t.Fatal("slot2 recovered prematurely")
	}
	slot2.mu.Lock()
	streakStartBefore := slot2.probeStreakStart
	slot2.mu.Unlock()
	if streakStartBefore.IsZero() {
		t.Fatal("slot2 probeStreakStart was not anchored before re-fallback")
	}

	f.closeWindow(time.Now())
	direct1.setErr(status.Error(codes.Unavailable, "outage 2"))
	if err := slot1.Invoke(context.Background(), createSessionMethod, nil, nil); status.Code(err) != codes.Unavailable {
		t.Fatalf("slot1 Invoke() err = %v, want Unavailable", err)
	}
	if res := f.closeWindow(time.Now()); !res.switched || !slot1.isInFallback() || !slot2.isInFallback() {
		t.Fatalf("second fallback failed: res=%+v slot1=%v slot2=%v", res, slot1.isInFallback(), slot2.isInFallback())
	}

	slot2.mu.Lock()
	gotConsec, gotStart, gotGen := slot2.consecutiveProbes, slot2.probeStreakStart, slot2.fallbackGen
	slot2.mu.Unlock()
	if gotConsec != 0 || !gotStart.IsZero() || gotGen != f.current().generation {
		t.Fatalf("slot2 after re-fallback = (%d, %v, gen=%d), want (0, zero, gen=%d)",
			gotConsec, gotStart, gotGen, f.current().generation)
	}
	if allowed, reset := f.tryRecoverSlot(nil, 1); allowed || reset || !f.current().fallback {
		t.Fatalf("tryRecoverSlot(nil, 1) = (%v, %v), fallback=%v; want (false, false), true", allowed, reset, f.current().fallback)
	}

	slot2.probeOnce(context.Background())
	slot2.probeOnce(context.Background())
	if !slot2.isInFallback() || !f.current().fallback {
		t.Fatal("slot2 recovered before 3 fresh consecutive probes after re-fallback")
	}
	slot2.probeOnce(context.Background())
	if slot2.isInFallback() || f.current().fallback {
		t.Fatal("slot2 did not recover on 3rd fresh consecutive probe after re-fallback")
	}
	stubInvoke(t, slot2)
	direct1.setErr(nil)
	for range 3 {
		slot1.probeOnce(context.Background())
	}
	if slot1.isInFallback() {
		t.Fatal("slot1 did not recover after healing")
	}

	metrics := map[string]int64{}
	for _, sm := range collectTestMetrics(t, reader).ScopeMetrics {
		for _, m := range sm.Metrics {
			if sum, ok := m.Data.(metricdata.Sum[int64]); ok {
				for _, p := range sum.DataPoints {
					metrics[m.Name+"|"+attrString(p.Attributes)] += p.Value
				}
			}
		}
	}
	for k, want := range map[string]int64{
		metricNameEEFCallStatus + "|channel_name=fallback,status_code=OK":                  1,
		metricNameEEFCallStatus + "|channel_name=fallback,status_code=Unavailable":         1,
		metricNameEEFFallbackCount + "|from_channel_name=primary,to_channel_name=fallback": 2,
		metricNameEEFFallbackCount + "|from_channel_name=fallback,to_channel_name=primary": 2,
	} {
		if got := metrics[k]; got != want {
			t.Fatalf("metric %q = %d, want %d; all=%v", k, got, want, metrics)
		}
	}
}

func TestDirectPathFallbackRecoveryCanaryGate(t *testing.T) {
	skipDialectRerun(t)
	setRecoveryTestPeerCheck(t, func(*peer.Peer) bool { return true })

	t.Run("holds remaining slots until canary DirectPath RPC succeeds without resetting streak", func(t *testing.T) {
		cloud := newStubPool()
		f := newTestRecoveryFallback(t, nil, stubCloudDial(cloud), time.Hour, 3, 0)
		direct1, slot1 := newStubSlot(t, f)
		_, slot2 := newStubSlot(t, f)
		_, slot3 := newStubSlot(t, f)

		preFallbackWin := f.current()
		forceDirectPathFallback(t, f)

		var failData1 atomic.Bool
		failData1.Store(true)
		direct1.setInvoke(func(_ context.Context, method string) error {
			if method != getSessionMethod && failData1.Load() {
				return status.Error(codes.Unavailable, "data plane still broken")
			}
			return nil
		})
		for range 3 {
			slot1.probeOnce(context.Background())
		}
		if slot1.isInFallback() || f.current().fallback {
			t.Fatalf("slot1 canary did not recover: slot1=%v window=%v", slot1.isInFallback(), f.current().fallback)
		}

		if err := slot1.Invoke(context.Background(), createSessionMethod, nil, nil); status.Code(err) != codes.Unavailable {
			t.Fatalf("slot1 data RPC err = %v, want Unavailable", err)
		}

		for range 4 {
			slot2.probeOnce(context.Background())
		}
		if !slot2.isInFallback() {
			t.Fatal("slot2 recovered while canary gen 2 had zero DirectPath successes")
		}
		slot2.mu.Lock()
		gotConsec, gotStart := slot2.consecutiveProbes, slot2.probeStreakStart
		slot2.mu.Unlock()
		if gotConsec < 3 || gotStart.IsZero() {
			t.Fatalf("slot2 streak was reset while waiting for canary: consecutiveProbes=%d start=%v", gotConsec, gotStart)
		}
		stubInvoke(t, slot2)
		if cloud.calls.Load() != 1 {
			t.Fatalf("slot2 CloudPath calls = %d, want 1", cloud.calls.Load())
		}

		f.recordSlotAttempt(context.Background(), preFallbackWin, false, nil)
		slot2.probeOnce(context.Background())
		if !slot2.isInFallback() {
			t.Fatal("stale pre-fallback generation success unlocked slot2")
		}

		failData1.Store(false)
		stubInvoke(t, slot1)
		slot2.probeOnce(context.Background())
		if slot2.isInFallback() {
			t.Fatal("slot2 did not recover on next probe after canary DirectPath success")
		}

		if res := f.closeWindow(time.Now()); res.switched {
			t.Fatalf("unexpected switch on verified window rollover: %+v", res)
		}
		for range 3 {
			slot3.probeOnce(context.Background())
		}
		if slot3.isInFallback() {
			t.Fatal("slot3 did not recover after non-fallback window rollover on verified generation")
		}
	})

	t.Run("re-fallback before canary success never routes remaining slots to DirectPath", func(t *testing.T) {
		cloud := newStubPool()
		f := newTestRecoveryFallback(t, nil, stubCloudDial(cloud), time.Hour, 3, 0)
		direct1, slot1 := newStubSlot(t, f)
		direct2, slot2 := newStubSlot(t, f)

		var slot2DirectDataCalls atomic.Int32
		direct1.setInvoke(func(_ context.Context, method string) error {
			if method != getSessionMethod {
				return status.Error(codes.Unavailable, "data plane broken")
			}
			return nil
		})
		direct2.setInvoke(func(_ context.Context, method string) error {
			if method != getSessionMethod {
				slot2DirectDataCalls.Add(1)
			}
			return nil
		})

		forceDirectPathFallback(t, f)

		for range 3 {
			slot1.probeOnce(context.Background())
		}
		if slot1.isInFallback() || f.current().fallback {
			t.Fatalf("slot1 canary did not recover: slot1=%v window=%v", slot1.isInFallback(), f.current().fallback)
		}
		for range 4 {
			slot2.probeOnce(context.Background())
		}
		if !slot2.isInFallback() {
			t.Fatal("slot2 left CloudPath before canary verified DirectPath")
		}
		stubInvoke(t, slot2)
		if cloud.calls.Load() != 1 {
			t.Fatalf("slot2 CloudPath calls = %d, want 1", cloud.calls.Load())
		}

		if err := slot1.Invoke(context.Background(), createSessionMethod, nil, nil); status.Code(err) != codes.Unavailable {
			t.Fatalf("slot1 Invoke() err = %v, want Unavailable", err)
		}
		if res := f.closeWindow(time.Now()); !res.switched || !slot1.isInFallback() || !slot2.isInFallback() {
			t.Fatalf("re-fallback failed: res=%+v slot1=%v slot2=%v", res, slot1.isInFallback(), slot2.isInFallback())
		}
		if slot2DirectDataCalls.Load() != 0 {
			t.Fatalf("slot2 sent %d data RPCs to DirectPath before re-fallback, want 0", slot2DirectDataCalls.Load())
		}
	})

	t.Run("closing unverified canary promotes next slot without deadlock", func(t *testing.T) {
		cloud := newStubPool()
		f := newTestRecoveryFallback(t, nil, stubCloudDial(cloud), time.Hour, 2, 0)
		_, slot1 := newStubSlot(t, f)
		_, slot2 := newStubSlot(t, f)
		_, slot3 := newStubSlot(t, f)

		forceDirectPathFallback(t, f)

		for range 2 {
			slot1.probeOnce(context.Background())
		}
		if slot1.isInFallback() || f.current().fallback {
			t.Fatalf("slot1 canary did not recover: slot1=%v window=%v", slot1.isInFallback(), f.current().fallback)
		}
		if err := slot1.Close(); err != nil {
			t.Fatalf("slot1.Close() failed: %v", err)
		}

		for range 2 {
			slot2.probeOnce(context.Background())
			slot3.probeOnce(context.Background())
		}
		if slot2.isInFallback() {
			t.Fatal("slot2 was not promoted as replacement canary after slot1 closed")
		}
		if !slot3.isInFallback() {
			t.Fatal("slot3 recovered before replacement canary verified DirectPath")
		}
		stubInvoke(t, slot2)
		slot3.probeOnce(context.Background())
		if slot3.isInFallback() {
			t.Fatal("slot3 did not recover after replacement canary DirectPath success")
		}
	})
}

func TestDirectPathFallbackRecoverySlotEdgeCases(t *testing.T) {
	skipDialectRerun(t)
	setRecoveryTestPeerCheck(t, func(*peer.Peer) bool { return true })

	t.Run("DCP scale-up CloudPath-only slot lazily dials DirectPath and recovers", func(t *testing.T) {
		direct, cloud := newStubPool(), newStubPool()
		f := newTestRecoveryFallback(t, nil, stubCloudDial(cloud), time.Hour, 1, 0)
		forceDirectPathFallback(t, f)

		noDialer := newCloudPathSlot(nil, f, nil, f.current().generation)
		t.Cleanup(func() { noDialer.Close() })
		if !noDialer.isInFallback() || hasProbeLoop(noDialer) {
			t.Fatalf("no-dialer slot: inFallback=%v hasLoop=%v", noDialer.isInFallback(), hasProbeLoop(noDialer))
		}

		var dialFailing atomic.Bool
		dialFailing.Store(true)
		var dialDirectCtx atomic.Pointer[context.Context]
		slot, err := dialChannelSlot(context.Background(), func(ctx context.Context) (gtransport.ConnPool, error) {
			if dialFailing.Load() {
				return nil, status.Error(codes.Unavailable, "directpath unreachable")
			}
			dialDirectCtx.Store(&ctx)
			return direct, nil
		}, f, time.Second)
		if err != nil {
			t.Fatalf("dialChannelSlot() failed: %v", err)
		}
		t.Cleanup(func() { slot.Close() })
		slot.probeOnce(context.Background())
		if slot.direct.Load() != nil || !slot.isInFallback() {
			t.Fatal("CloudPath-only slot should stay in fallback until lazy direct dial succeeds")
		}
		stubInvoke(t, slot)
		if cloud.calls.Load() != 1 {
			t.Fatalf("CloudPath calls = %d, want 1", cloud.calls.Load())
		}

		dialFailing.Store(false)
		slot.probeOnce(context.Background())
		if slot.isInFallback() {
			t.Fatal("CloudPath-only slot did not recover after lazy direct dial succeeded")
		}
		dctxPtr := dialDirectCtx.Load()
		if dctxPtr == nil || (*dctxPtr).Err() != nil {
			t.Fatalf("lazy direct dial context canceled prematurely: %v", dctxPtr)
		}
		directBefore := direct.calls.Load()
		stubInvoke(t, slot)
		if direct.calls.Load() != directBefore+1 {
			t.Fatal("slot did not route over DirectPath after recovery")
		}
		if err := slot.Close(); err != nil || (*dctxPtr).Err() == nil {
			t.Fatalf("slot.Close() err=%v dialCtxErr=%v", err, (*dctxPtr).Err())
		}
		f.slotsMu.Lock()
		_, stillRegistered := f.slots[slot]
		f.slotsMu.Unlock()
		if stillRegistered {
			t.Fatal("closed slot remained registered in directPathFallback")
		}
	})

	t.Run("errNoCloudPath disables probing pool-wide and upfront in dialChannelSlot", func(t *testing.T) {
		noCloudDial := func(context.Context) (gtransport.ConnPool, error) {
			return nil, errNoCloudPath
		}
		f := newTestRecoveryFallback(t, nil, noCloudDial, time.Hour, 3, 0)
		direct1, slot1 := newStubSlot(t, f)
		direct2, slot2 := newStubSlot(t, f)
		forceDirectPathFallback(t, f)

		stubInvoke(t, slot1)
		slot1.probeOnce(context.Background())
		slot2.probeOnce(context.Background())
		if !f.probingDisabled.Load() {
			t.Fatal("errNoCloudPath did not disable probing pool-wide")
		}
		if direct1.calls.Load() != 1 || direct2.calls.Load() != 0 {
			t.Fatalf("unexpected probe calls after errNoCloudPath: direct1=%d direct2=%d", direct1.calls.Load(), direct2.calls.Load())
		}

		f2 := newTestRecoveryFallback(t, nil, noCloudDial, time.Hour, 3, 0)
		forceDirectPathFallback(t, f2)
		direct3 := newStubPool()
		slot3, err := dialChannelSlot(context.Background(), func(context.Context) (gtransport.ConnPool, error) {
			return direct3, nil
		}, f2, time.Second)
		if err != nil {
			t.Fatalf("dialChannelSlot() failed: %v", err)
		}
		t.Cleanup(func() { slot3.Close() })
		slot3.probeOnce(context.Background())
		if !f2.probingDisabled.Load() || hasProbeLoop(slot3) || direct3.calls.Load() != 0 {
			t.Fatal("dialChannelSlot with errNoCloudPath did not disable probing upfront")
		}
	})
}

func TestDirectPathFallbackRecoveryEndToEndFixedAndDCPClients(t *testing.T) {
	skipDialectRerun(t)
	setRecoveryTestPeerCheck(t, func(*peer.Peer) bool { return true })
	oldCfg := defaultDirectPathRecoveryConfig
	defaultDirectPathRecoveryConfig = directPathFallbackRecoveryConfig{
		probeInterval:       recoveryProbeIntv,
		probeRPCTimeout:     2 * time.Second,
		minRecoveryProbes:   3,
		minRecoveryDuration: recoveryProbeMinDur,
	}
	t.Cleanup(func() { defaultDirectPathRecoveryConfig = oldCfg })

	for _, dynamic := range []bool{false, true} {
		t.Run(fmt.Sprintf("dynamic=%v", dynamic), func(t *testing.T) {
			enableDirectPathForTest(t)
			t.Setenv(directPathFallbackRecoveryEnv, "true")

			paths := newSharedBackendPaths(t)
			config := ClientConfig{NumChannels: 2}
			if dynamic {
				config.DynamicChannelPoolConfig = dcpFallbackTestConfig()
			}
			client := newDirectPathFallbackTestClient(t, config, paths.opts...)
			p := client.sc.dynamicPool
			f := poolFallback(p)
			if f == nil || !f.recovery.enabled || (!dynamic && p.PrimeSessionID() != "") {
				t.Fatalf("unexpected setup (dynamic=%v): fallback=%v prime=%q", dynamic, f != nil, p.PrimeSessionID())
			}
			if sid, _ := f.probeSession.Load().(string); sid == "" {
				t.Fatal("probe session ID was not set on client startup")
			}

			var wg sync.WaitGroup
			ctx, cancel := context.WithCancel(context.Background())
			defer wg.Wait()
			defer cancel()
			var opErr atomic.Pointer[error]
			for range 4 {
				wg.Add(1)
				go func() {
					defer wg.Done()
					for ctx.Err() == nil {
						if _, err := client.ReadWriteTransaction(ctx, func(txCtx context.Context, tx *ReadWriteTransaction) error {
							_, err := tx.Update(txCtx, NewStatement(UpdateBarSetFoo))
							return err
						}); err != nil && ctx.Err() == nil {
							opErr.CompareAndSwap(nil, &err)
							return
						}
					}
				}()
			}

			for range 2 {
				forceDirectPathFallbackUnderLoad(t, f)
				waitFor(t, func() error {
					if f.current().fallback {
						return errors.New("shared window still in fallback")
					}
					for _, e := range p.GetEntries() {
						if e.Pool.(*channelSlot).isInFallback() {
							return fmt.Errorf("entry %d still in fallback", e.ID)
						}
					}
					return nil
				})
			}

			cancel()
			wg.Wait()
			if errPtr := opErr.Load(); errPtr != nil {
				t.Fatalf("concurrent transaction failed during fallback/recovery flapping: %v", *errPtr)
			}
			directBefore, cloudBefore := paths.direct.count(executeStreamingSQLMethod), paths.cloud.count(executeStreamingSQLMethod)
			queryFooFromBar(t, client)
			if paths.direct.count(executeStreamingSQLMethod) != directBefore+1 || paths.cloud.count(executeStreamingSQLMethod) != cloudBefore {
				t.Fatalf("post-recovery query counts: direct=%d cloud=%d", paths.direct.count(executeStreamingSQLMethod), paths.cloud.count(executeStreamingSQLMethod))
			}

			directRecBefore, cloudRecBefore := len(paths.direct.recorded()), len(paths.cloud.recorded())
			if _, err := client.ReadWriteTransaction(context.Background(), func(txCtx context.Context, tx *ReadWriteTransaction) error {
				if _, err := tx.Update(txCtx, NewStatement(UpdateBarSetFoo)); err != nil {
					return err
				}
				_, err := tx.Update(txCtx, NewStatement(UpdateBarSetFoo))
				return err
			}); err != nil {
				t.Fatalf("post-recovery ReadWriteTransaction() failed: %v", err)
			}
			if got := paths.cloud.recorded()[cloudRecBefore:]; len(got) != 0 {
				t.Fatalf("post-recovery transaction sent %d RPCs to CloudPath, want 0: %+v", len(got), got)
			}
			var txRPCs []frontDoorRPC
			for _, rpc := range paths.direct.recorded()[directRecBefore:] {
				if rpc.method == executeSQLMethod || rpc.method == commitMethod {
					txRPCs = append(txRPCs, rpc)
				}
			}
			if len(txRPCs) != 3 {
				t.Fatalf("post-recovery DirectPath transaction RPCs = %+v, want 2 ExecuteSql + 1 Commit", txRPCs)
			}
			slotID := txRPCs[0].channelID
			if slotID == 0 {
				t.Fatalf("post-recovery transaction reported channelID 0: %+v", txRPCs)
			}
			for _, rpc := range txRPCs[1:] {
				if rpc.channelID != slotID {
					t.Fatalf("post-recovery transaction hopped channels: %s used %d, want %d (%+v)", rpc.method, rpc.channelID, slotID, txRPCs)
				}
			}
		})
	}
}

func TestDirectPathFallbackRecoveryCloseRaces(t *testing.T) {
	skipDialectRerun(t)
	setRecoveryTestPeerCheck(t, func(*peer.Peer) bool { return true })

	t.Run("close during blocked GetSession probe", func(t *testing.T) {
		f := newTestRecoveryFallback(t, nil, nil, recoveryProbeIntv, 3, time.Second)
		direct, slot := newStubSlot(t, f)
		started := make(chan struct{}, 1)
		block := blockUntilDone(started)
		direct.setInvoke(func(ctx context.Context, _ string) error { return block(ctx) })
		forceDirectPathFallback(t, f)
		assertCloseUnblocks(t, started, slot)
	})

	t.Run("close during blocked lazy direct dial", func(t *testing.T) {
		f := newTestRecoveryFallback(t, nil, stubCloudDial(newStubPool()), recoveryProbeIntv, 3, time.Second)
		forceDirectPathFallback(t, f)

		started := make(chan struct{}, 1)
		block := blockUntilDone(started)
		slot, err := dialChannelSlot(context.Background(), func(ctx context.Context) (gtransport.ConnPool, error) {
			return nil, block(ctx)
		}, f, time.Second)
		if err != nil {
			t.Fatalf("dialChannelSlot() failed: %v", err)
		}
		t.Cleanup(func() { slot.Close() })
		assertCloseUnblocks(t, started, slot)
	})

	t.Run("closing slot racing with final probe cannot clear shared fallback window", func(t *testing.T) {
		f := newTestRecoveryFallback(t, nil, nil, time.Hour, 1, 0)
		direct, slot := newStubSlot(t, f)
		inRPC, releaseRPC := make(chan struct{}, 1), make(chan struct{})
		direct.setInvoke(func(context.Context, string) error {
			select {
			case inRPC <- struct{}{}:
			default:
			}
			<-releaseRPC
			return nil
		})
		forceDirectPathFallback(t, f)

		probeDone := make(chan struct{})
		go func() {
			defer close(probeDone)
			slot.probeOnce(context.Background())
		}()
		<-inRPC
		if err := slot.Close(); err != nil {
			t.Fatalf("slot.Close() failed: %v", err)
		}
		close(releaseRPC)
		<-probeDone
		if !f.current().fallback {
			t.Fatal("closed slot cleared shared fallback window")
		}
	})

	t.Run("CloudPath-only slot created after fallback.close() stays in fallback without nil panic", func(t *testing.T) {
		cloud := newStubPool()
		f := newTestRecoveryFallback(t, nil, nil, recoveryProbeIntv, 3, time.Second)
		forceDirectPathFallback(t, f)
		gen := f.current().generation
		f.close()

		slot := newCloudPathSlot(cloud, f, func(context.Context) (gtransport.ConnPool, error) {
			return newStubPool(), nil
		}, gen)
		t.Cleanup(func() { slot.Close() })
		if !slot.isInFallback() || slot.Conn() != cloud.Conn() || hasProbeLoop(slot) {
			t.Fatalf("post-close slot state: inFallback=%v conn=%v hasLoop=%v", slot.isInFallback(), slot.Conn(), hasProbeLoop(slot))
		}
		stubInvoke(t, slot)
		if cloud.calls.Load() != 1 {
			t.Fatalf("CloudPath calls = %d, want 1", cloud.calls.Load())
		}
	})
}

func TestDirectPathFallbackRecoveryRejectsNonALTSPeer(t *testing.T) {
	skipDialectRerun(t)
	enableDirectPathForTest(t)
	t.Setenv(directPathFallbackRecoveryEnv, "true")

	oldCfg := defaultDirectPathRecoveryConfig
	defaultDirectPathRecoveryConfig = directPathFallbackRecoveryConfig{
		probeInterval:       time.Hour,
		probeRPCTimeout:     2 * time.Second,
		minRecoveryProbes:   1,
		minRecoveryDuration: 0,
	}
	t.Cleanup(func() { defaultDirectPathRecoveryConfig = oldCfg })

	paths := newSharedBackendPaths(t)
	client := newDirectPathFallbackTestClient(t, ClientConfig{NumChannels: 1}, paths.opts...)
	p := fixedPoolOf(t, client)
	f := poolFallback(p)
	slot := p.GetEntries()[0].Pool.(*channelSlot)

	forceDirectPathFallback(t, f)
	slot.probeOnce(context.Background())
	if !slot.isInFallback() || !f.current().fallback {
		t.Fatalf("non-ALTS probe recovered slot: inFallback=%v windowFallback=%v", slot.isInFallback(), f.current().fallback)
	}
	if got := paths.direct.count(getSessionMethod); got != 1 {
		t.Fatalf("DirectPath GetSession probes = %d, want 1", got)
	}
}

func TestDirectPathFallbackRecoveryStaleInFlightProbeDiscarded(t *testing.T) {
	skipDialectRerun(t)
	setRecoveryTestPeerCheck(t, func(*peer.Peer) bool { return true })

	f := newTestRecoveryFallback(t, nil, nil, time.Hour, 1, 0)
	_, slot1 := newStubSlot(t, f)
	direct2, slot2 := newStubSlot(t, f)
	forceDirectPathFallback(t, f)

	slot1.probeOnce(context.Background())
	if slot1.isInFallback() || f.current().fallback {
		t.Fatalf("slot1 did not recover: slot1=%v window=%v", slot1.isInFallback(), f.current().fallback)
	}

	inProbe := make(chan struct{})
	releaseProbe := make(chan struct{})
	direct2.setInvoke(func(context.Context, string) error {
		close(inProbe)
		<-releaseProbe
		return nil
	})
	probeDone := make(chan struct{})
	go func() {
		defer close(probeDone)
		slot2.probeOnce(context.Background())
	}()
	<-inProbe

	forceDirectPathFallback(t, f)
	close(releaseProbe)
	<-probeDone

	if !slot2.isInFallback() || !f.current().fallback {
		t.Fatalf("stale in-flight probe recovered slot2 in newer generation: slot2=%v window=%v", slot2.isInFallback(), f.current().fallback)
	}
}

func TestDirectPathFallbackRecoveryLogsProbeFailureOnTransition(t *testing.T) {
	skipDialectRerun(t)
	setRecoveryTestPeerCheck(t, func(*peer.Peer) bool { return true })

	var buf bytes.Buffer
	f := newDirectPathFallback(nil, log.New(&buf, "", 0), nil, time.Hour, directPathFallbackRecoveryConfig{
		enabled:             true,
		probeInterval:       time.Hour,
		probeRPCTimeout:     time.Second,
		minRecoveryProbes:   2,
		minRecoveryDuration: 0,
	})
	t.Cleanup(f.close)
	f.setProbeSession(recoveryTestSess)
	direct, slot := newStubSlot(t, f)
	forceDirectPathFallback(t, f)

	direct.setErr(status.Error(codes.PermissionDenied, "denied"))
	slot.probeOnce(context.Background())
	slot.probeOnce(context.Background())
	if got := strings.Count(buf.String(), "PermissionDenied"); got != 1 {
		t.Fatalf("PermissionDenied log count = %d, want 1; log=%q", got, buf.String())
	}

	direct.setErr(status.Error(codes.Unavailable, "down"))
	slot.probeOnce(context.Background())
	slot.probeOnce(context.Background())
	if got := strings.Count(buf.String(), "Unavailable"); got != 1 {
		t.Fatalf("Unavailable log count = %d, want 1; log=%q", got, buf.String())
	}

	direct.setErr(nil)
	slot.probeOnce(context.Background())
	direct.setErr(status.Error(codes.Unavailable, "down again"))
	slot.probeOnce(context.Background())
	if got := strings.Count(buf.String(), "Unavailable"); got != 2 {
		t.Fatalf("Unavailable log count after success = %d, want 2; log=%q", got, buf.String())
	}
}

type fakeALTSAuthInfo struct {
	alts.AuthInfo
}

func (fakeALTSAuthInfo) AuthType() string { return "alts" }

func TestIsDirectPathPeerFn(t *testing.T) {
	skipDialectRerun(t)
	if isDirectPathPeerFn(nil) {
		t.Fatal("expected nil peer to return false")
	}
	if isDirectPathPeerFn(&peer.Peer{}) {
		t.Fatal("expected peer without AuthInfo to return false")
	}
	if !isDirectPathPeerFn(&peer.Peer{AuthInfo: fakeALTSAuthInfo{}}) {
		t.Fatal("expected peer with alts.AuthInfo to return true")
	}
}

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

package internal

import (
	"context"
	"math/bits"
	"runtime"
	"sync"
	"testing"
	"time"

	spb "cloud.google.com/go/bigtable/apiv2/bigtablepb"
)

// newBenchPool constructs a SessionPoolImpl with debugEnabled=false
// (production default) and registers ready sessions across AFEs,
// optionally adding closingPerMultiAfe Closing handles on multi-session
// AFEs to model steady-state GoAway drain overlap.
func newBenchPool(b *testing.B, singleSessionAfes, multiSessionAfes, sessionsPerMultiAfe, closingPerMultiAfe int) *SessionPoolImpl {
	b.Helper()
	factory, closeStreams := newStubStreamFactory()
	totalReady := singleSessionAfes + multiSessionAfes*sessionsPerMultiAfe
	p := NewSessionPoolImpl(
		uint64(1),
		"bench-pool",
		factory,
		&spb.OpenSessionRequest{ProtocolVersion: 1},
		nil,
		SessionTypeTable,
		false, // debugEnabled=false matches production hot path
	)
	p.sizer.UpdateConfig(&spb.SessionClientConfiguration_SessionPoolConfiguration{
		MinSessionCount: int32(totalReady),
		MaxSessionCount: int32(totalReady * 2),
	})

	addHandle := func(afeID AfeID) *SessionHandle {
		stream := newFakeStream()
		sh := newSessionHandle(nil, time.Now())
		s := NewSession("bench-s", stream, SessionHooks{
			OnStart:  p.onStart,
			OnActive: func(_ *Session) { p.onActive(sh) },
			OnSlotDrained: func() {
				p.sl.ReleaseToPool(sh)
				p.signalFree()
			},
			OnClosing: func(_ *Session) { p.onClosing(sh) },
			OnClose:   func(_ *Session, err error) { p.onClose(sh, err) },
		}, SessionTypeTable)
		s.peerInfo.Store(&spb.PeerInfo{ApplicationFrontendId: int64(afeID)})
		s.state.Store(int32(StateReady))
		sh.session = s
		sh.activated.Store(true)
		p.sl.OnSessionStarted(sh)
		return sh
	}

	nextAfe := AfeID(1)
	for i := 0; i < singleSessionAfes; i++ {
		addHandle(nextAfe)
		nextAfe++
	}
	for i := 0; i < multiSessionAfes; i++ {
		afeID := nextAfe
		nextAfe++
		for j := 0; j < sessionsPerMultiAfe; j++ {
			addHandle(afeID)
		}
		for j := 0; j < closingPerMultiAfe; j++ {
			closing := addHandle(afeID)
			closing.session.state.Store(int32(StateClosing))
			p.sl.OnSessionClosing(closing)
		}
	}

	b.Cleanup(func() {
		closeStreams()
		_ = p.Close()
	})
	return p
}

// BenchmarkCheckoutSession measures CheckoutSession throughput, allocations,
// and p50/p99/p99.9 latency across uncontended and Regime A concurrent
// workloads (400 ready sessions across 120 AFEs: 80 single-session + 40
// 8-session AFEs, with and without draining Closing sessions, plus PeakEwma).
func BenchmarkCheckoutSession(b *testing.B) {
	workers := runtime.GOMAXPROCS(0)
	singleAfes := 80
	multiAfes := 40
	perMulti := 8
	for singleAfes+multiAfes*perMulti < workers*4 {
		multiAfes += 10
		singleAfes += 20
	}

	b.Run("Uncontended_120Afes_LeastInFlight", func(b *testing.B) {
		p := newBenchPool(b, singleAfes, multiAfes, perMulti, 0)
		ctx := context.Background()
		var counts [latencyHistBuckets]uint64
		var total uint64

		b.ReportAllocs()
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			start := time.Now()
			sh, err := p.CheckoutSession(ctx)
			d := time.Since(start)
			if err != nil {
				b.Fatalf("CheckoutSession failed: %v", err)
			}
			sh.DecOutstanding()
			p.sl.ReleaseToPool(sh)
			p.signalFree()

			if d > 0 {
				idx := bits.Len64(uint64(d)) - 1
				if idx >= latencyHistBuckets {
					idx = latencyHistBuckets - 1
				}
				counts[idx]++
				total++
			}
		}
		b.StopTimer()
		if total > 0 {
			b.ReportMetric(float64(interpLatencyPercentile(counts[:], total, 50)), "p50-ns")
			b.ReportMetric(float64(interpLatencyPercentile(counts[:], total, 99)), "p99-ns")
			b.ReportMetric(float64(interpLatencyPercentile(counts[:], total, 99.9)), "p99.9-ns")
		}
	})

	for _, tc := range []struct {
		name       string
		numClosing int
		peakEwma   bool
	}{
		{name: "Parallel_120Afes_LeastInFlight", numClosing: 0, peakEwma: false},
		{name: "Parallel_120Afes_WithClosingDrain", numClosing: 1, peakEwma: false},
		{name: "Parallel_120Afes_PeakEwma", numClosing: 0, peakEwma: true},
	} {
		b.Run(tc.name, func(b *testing.B) {
			p := newBenchPool(b, singleAfes, multiAfes, perMulti, tc.numClosing)
			if tc.peakEwma {
				p.mu.Lock()
				p.picker = NewLeastLatencyAfePicker(defaultAfeRandomSubsetSize, false)
				p.mu.Unlock()
			}
			ctx := context.Background()

			var histMu sync.Mutex
			var globalCounts [latencyHistBuckets]uint64
			var globalTotal uint64

			b.ReportAllocs()
			b.ResetTimer()
			b.RunParallel(func(pb *testing.PB) {
				var localCounts [latencyHistBuckets]uint64
				var localTotal uint64
				var held *SessionHandle

				for pb.Next() {
					start := time.Now()
					sh, err := p.CheckoutSession(ctx)
					d := time.Since(start)
					if err != nil {
						b.Fatalf("CheckoutSession failed: %v", err)
					}
					if held != nil {
						held.DecOutstanding()
						p.sl.ReleaseToPool(held)
						p.signalFree()
					}
					held = sh

					if d > 0 {
						idx := bits.Len64(uint64(d)) - 1
						if idx >= latencyHistBuckets {
							idx = latencyHistBuckets - 1
						}
						localCounts[idx]++
						localTotal++
					}
				}
				if held != nil {
					held.DecOutstanding()
					p.sl.ReleaseToPool(held)
					p.signalFree()
				}

				histMu.Lock()
				for i, c := range localCounts {
					globalCounts[i] += c
				}
				globalTotal += localTotal
				histMu.Unlock()
			})
			b.StopTimer()

			if globalTotal > 0 {
				b.ReportMetric(float64(interpLatencyPercentile(globalCounts[:], globalTotal, 50)), "p50-ns")
				b.ReportMetric(float64(interpLatencyPercentile(globalCounts[:], globalTotal, 99)), "p99-ns")
				b.ReportMetric(float64(interpLatencyPercentile(globalCounts[:], globalTotal, 99.9)), "p99.9-ns")
			}
		})
	}
}

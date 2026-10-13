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
	"testing"
	"time"

	spb "cloud.google.com/go/bigtable/apiv2/bigtablepb"
)

// TestSessionPool_DebugDisabled_NoRecorderState pins the zero-alloc
// contract of session.Config.EnableDebug=false: with debugEnabled=false
// the pool records nothing on any allocating debug site. Drives the
// pool through several picker + latency + scaling paths and asserts
// the debug rings stay empty.
//
// This is the safety net for the gate itself — every other test
// hard-codes debugEnabled=true, so without this test a future refactor
// that inverts one of the gates would land green.
func TestSessionPool_DebugDisabled_NoRecorderState(t *testing.T) {
	neverDialing := func(ctx context.Context) (Stream, error) {
		<-ctx.Done()
		return nil, ctx.Err()
	}
	p := NewSessionPoolImpl(
		uint64(1),
		"test-pool-nodebug", neverDialing,
		&spb.OpenSessionRequest{ProtocolVersion: 1}, nil, SessionTypeTable,
		false, // debugEnabled — the contract under test
	)
	p.UpdateConfig(&spb.SessionClientConfiguration_SessionPoolConfiguration{
		MinSessionCount: 0, MaxSessionCount: 1,
	})
	defer p.Close()

	// Exercise the recorder call sites via direct calls. The pool
	// forwards the same recorders from its Invoke/scaling/lifecycle
	// paths; this test targets the gate itself, not the wiring.
	p.recordPickDecision(PickDecision{Winner: AfeID(1), Reason: "test"}, "least-inflight")
	p.recordLifetime(100 * time.Millisecond)
	p.recordSlowVRpc(SlowVRpcEvent{At: time.Now(), Method: "ReadRow", Latency: 100 * time.Millisecond})
	p.recordScaling(ScalingEvent{At: time.Now(), Reason: "test"})
	p.recordTimeSeries()

	p.m.pickHistoryMu.Lock()
	if n := len(p.m.pickHistory); n != 0 {
		t.Errorf("pickHistory has %d entries, want 0 (debug off)", n)
	}
	p.m.pickHistoryMu.Unlock()

	if n := len(p.m.afePickCounts); n != 0 {
		t.Errorf("afePickCounts has %d entries, want 0 (debug off)", n)
	}

	p.m.slowVRpcsMu.Lock()
	if n := len(p.m.slowVRpcs); n != 0 {
		t.Errorf("slowVRpcs has %d entries, want 0 (debug off)", n)
	}
	p.m.slowVRpcsMu.Unlock()

	p.m.lifetimesMu.Lock()
	if n := len(p.m.lifetimes); n != 0 {
		t.Errorf("lifetimes has %d entries, want 0 (debug off)", n)
	}
	p.m.lifetimesMu.Unlock()

	p.m.scalingHistoryMu.Lock()
	if n := len(p.m.scalingHistory); n != 0 {
		t.Errorf("scalingHistory has %d entries, want 0 (debug off)", n)
	}
	p.m.scalingHistoryMu.Unlock()

	p.m.timeSeriesMu.Lock()
	if n := len(p.m.timeSeries); n != 0 {
		t.Errorf("timeSeries has %d entries, want 0 (debug off)", n)
	}
	p.m.timeSeriesMu.Unlock()
}

// TestSession_DebugDisabled_NoRingBufferState pins the equivalent
// zero-alloc contract for the per-Session recorders: recordEvent /
// recordLatency / recordCluster must be no-ops when the session was
// constructed without WithSessionDebugEnabled(true).
func TestSession_DebugDisabled_NoRingBufferState(t *testing.T) {
	stream := newFakeStream()
	// NOTE: no WithSessionDebugEnabled option → debug off.
	s := NewSession("test-session-nodebug", stream, SessionHooks{}, SessionTypeTable)

	s.recordEvent(SessionEventLateFrame, "test")
	s.recordLatency(1 * time.Millisecond)
	s.recordCluster("test-cluster")

	if events := s.snapshotEvents(); len(events) != 0 {
		t.Errorf("events ring has %d entries, want 0 (debug off)", len(events))
	}
	if samples := s.snapshotLatencies(); len(samples) != 0 {
		t.Errorf("latencySamples has %d entries, want 0 (debug off)", len(samples))
	}
	if clusters := s.snapshotClusters(); len(clusters) != 0 {
		t.Errorf("clusters map has %d entries, want 0 (debug off)", len(clusters))
	}
}

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
	"fmt"
	"log"
	"sort"
	"sync"
	"sync/atomic"
	"time"

	metrics "cloud.google.com/go/bigtable/internal/metrics"
	btopt "cloud.google.com/go/bigtable/internal/option"
)

// sessionDebug groups the Session's observability state — counters,
// per-session event ring, latency histogram, tracer/logger handles, and
// the metric-attribution plumbing (close reason, pool-close gate,
// channel index). Embedded into Session so existing call sites keep
// using bare field access (s.okRpcs, s.recordEvent, s.tracer, ...).
// The split exists so session.go stays focused on the protocol state
// machine and vRPC dispatch; new observability hooks land here.
type sessionDebug struct {
	logger *log.Logger
	tracer *sessionTracer

	// lastStateChangeNano stamps each successful transitionTo with
	// time.Now().UnixNano(). Approximate ordering across transitions is
	// enough for the debug UI.
	lastStateChangeNano atomic.Int64

	// remoteAddr is the TCP remote (AFE) socket address in "ip:port" form,
	// captured from grpc peer.FromContext once the stream Header returns.
	// atomic.Pointer so the vRPC hot path can read it lock-free to stamp
	// slow-vRPC rows for the sessionz→tcpz cross-link.
	remoteAddr atomic.Pointer[string]

	// okRpcs / errorRpcs are bumped lock-free from the vRPC dispatch paths
	// so the debug UI can read a numeric total without locking.
	okRpcs    atomic.Int64
	errorRpcs atomic.Int64

	// msgsSent / msgsRecv count every Send / Recv frame on the bidi stream
	// — every session-level frame type (OpenSession, vRPC, CloseSession,
	// Heartbeat), not just successful vRPCs.
	msgsSent atomic.Int64
	msgsRecv atomic.Int64

	// Per-frame-type breakdown of msgsSent / msgsRecv. Indexed by
	// reqMsgType / respMsgType (see session_msgtype.go).
	msgsSentByType [numReqMsgTypes]atomic.Int64
	msgsRecvByType [numRespMsgTypes]atomic.Int64

	// retries counts vRPCs that arrived with VRpcAttempt>1 — i.e. the
	// retry interceptor re-issued them on this session.
	retries atomic.Int64

	// closeReason is set exactly once via setCloseReason() at session
	// teardown so SessionPoolImpl.OnClose can attribute the close to a
	// category (Heartbeat / GoAway / Error / User).
	closeReason atomic.Pointer[string]

	// poolCloseRecorded is the once-flag SessionPoolImpl consults so its
	// sessionsClosed / CloseReasons counters bump exactly once per session
	// regardless of which removal path arrives first (proactive prune,
	// CheckoutSession dead-detect, or hooks.OnClose).
	poolCloseRecorded atomic.Bool

	// latencyMu guards latencySamples — a tiny ring buffer of the last
	// latencyWindow server-reported BackendLatency values, used to compute
	// p50/p95/p99 in the debug UI.
	latencyMu      sync.Mutex
	latencySamples []time.Duration
	latencyNext    int

	// clusterCounts tallies per-ClusterInformation.ClusterId responses.
	// Values are *atomic.Int64.
	clusterCounts sync.Map

	// channelIndex is set once at construction to the BigtableChannelPool
	// connEntry index the session's bidi stream was placed on. -1 when
	// the underlying pool isn't a BigtableChannelPool (e.g. test setups
	// using option.WithGRPCConn) or when the pick wasn't observed.
	channelIndex atomic.Int32

	// eventsMu guards the per-session debug-event ring surfaced in
	// sessionz. maxSessionEvents caps the size.
	eventsMu sync.Mutex
	events   []SessionEvent
}

// init sets the non-zero defaults for a freshly-embedded sessionDebug.
// Called once from NewSession.
func (d *sessionDebug) init(sessionType SessionType) {
	d.tracer = newSessionTracer(sessionType)
	d.lastStateChangeNano.Store(time.Now().UnixNano())
	d.channelIndex.Store(-1)
}

// SessionEvent is one entry in a session's per-session debug ring buffer.
// Surfaced through SessionSnapshot.RecentEvents and merged across all
// sessions into PoolSnapshot.RecentEvents for the sessionz UI.
//
// Kinds in use:
//
//	"close"     — stream tear-down handled by handleClose; Message carries
//	              reason, age, in-flight count, last rpc id, raw err.
//	"hb-missed" — heartbeat watchdog fired ForceClose; Message carries
//	              in-flight count and last-frame age.
//	"hb-alive"  — heartbeat tick observed in-flight RPC(s) while a recent
//	              frame had already pushed the deadline; useful for spotting
//	              "server kept stream alive but lost specific vRPC response"
//	              stalls. Suppressed (not recorded) unless lastFrameAge is
//	              at least one heartbeat interval to avoid log noise.
//	"ctx-done"  — Session.Invoke's per-attempt wait was killed by the
//	              caller's context (deadline or cancel); Message carries
//	              method, rpc id, time waited, ctx err, session state.
//	"dup-deliver" — deliver's default branch fired: something tried to
//	              publish a second value on rpc.resultChan while it
//	              was still full. Either a server double-frame
//	              (protocol violation — also tagged
//	              tagSessionVRPCDuplicateResult from the receive-loop
//	              caller) or a cancel racing a completion (benign;
//	              cancel-side loser). Message carries rpc id and method.
type SessionEvent struct {
	At      time.Time
	Kind    string
	Message string
}

const maxSessionEvents = 64

// recordEvent appends a SessionEvent to the per-session ring buffer.
// Safe to call from any goroutine (readLoop, heartBeatLoop, etc.).
func (s *Session) recordEvent(kind, format string, args ...interface{}) {
	ev := SessionEvent{
		At:      time.Now(),
		Kind:    kind,
		Message: fmt.Sprintf(format, args...),
	}
	s.eventsMu.Lock()
	if len(s.events) >= maxSessionEvents {
		copy(s.events, s.events[1:])
		s.events = s.events[:len(s.events)-1]
	}
	s.events = append(s.events, ev)
	s.eventsMu.Unlock()
}

// snapshotEvents returns a copy of the session's debug-event ring buffer
// (oldest first).
func (s *Session) snapshotEvents() []SessionEvent {
	s.eventsMu.Lock()
	out := make([]SessionEvent, len(s.events))
	copy(out, s.events)
	s.eventsMu.Unlock()
	return out
}

// peerInfoSummary renders the session's PeerInfo as a compact single line
// suitable for inclusion in log messages and debug events. Returns
// "peer=unknown" when the bidi stream header hasn't been parsed yet.
func (s *Session) peerInfoSummary() string {
	p := s.peerInfo.Load()
	if p == nil {
		return "peer=unknown"
	}
	return fmt.Sprintf("peer={afe=%x/%s/%s gfe=%x transport=%s}",
		p.GetApplicationFrontendId(),
		p.GetApplicationFrontendRegion(),
		p.GetApplicationFrontendSubzone(),
		p.GetGoogleFrontendId(),
		metrics.TransportTypeName(p.GetTransportType()))
}

const latencyWindow = 256

// recordLatency appends a server-reported BackendLatency sample to the
// ring buffer. Called from Invoke whenever the response includes Stats.
func (s *Session) recordLatency(d time.Duration) {
	if d <= 0 {
		return
	}
	s.latencyMu.Lock()
	if len(s.latencySamples) < latencyWindow {
		s.latencySamples = append(s.latencySamples, d)
	} else {
		s.latencySamples[s.latencyNext] = d
		s.latencyNext = (s.latencyNext + 1) % latencyWindow
	}
	s.latencyMu.Unlock()
}

// snapshotLatencies returns a sorted copy of the current samples so
// callers can compute percentiles without holding the lock.
func (s *Session) snapshotLatencies() []time.Duration {
	s.latencyMu.Lock()
	out := make([]time.Duration, len(s.latencySamples))
	copy(out, s.latencySamples)
	s.latencyMu.Unlock()
	sort.Slice(out, func(i, j int) bool { return out[i] < out[j] })
	return out
}

// percentile returns the p-th percentile (0-100) of the sorted slice.
// Uses nearest-rank — small N, linear interpolation isn't worth the
// complexity.
func percentile(sorted []time.Duration, p float64) time.Duration {
	if len(sorted) == 0 {
		return 0
	}
	if p <= 0 {
		return sorted[0]
	}
	if p >= 100 {
		return sorted[len(sorted)-1]
	}
	idx := int(float64(len(sorted)-1) * p / 100)
	return sorted[idx]
}

// recordCluster increments the per-cluster response counter.
func (s *Session) recordCluster(id string) {
	if id == "" {
		return
	}
	c, _ := s.clusterCounts.LoadOrStore(id, new(atomic.Int64))
	c.(*atomic.Int64).Add(1)
}

// snapshotClusters returns the per-cluster response counts as a flat map.
func (s *Session) snapshotClusters() map[string]int64 {
	out := map[string]int64{}
	s.clusterCounts.Range(func(k, v interface{}) bool {
		out[k.(string)] = v.(*atomic.Int64).Load()
		return true
	})
	return out
}

// SetChannelIndex records the BigtableChannelPool connEntry index the
// session's stream landed on. Called once at session construction.
func (s *Session) SetChannelIndex(idx int) {
	s.channelIndex.Store(int32(idx))
}

// ChannelIndex returns the per-pool channel index, or -1 if unset.
func (s *Session) ChannelIndex() int {
	return int(s.channelIndex.Load())
}

// setCloseReason records the reason for the session's terminal close.
// Only the first call sticks; subsequent calls (racing teardown paths)
// are ignored so the OnClose callback sees a stable value.
func (s *Session) setCloseReason(reason string) {
	if reason == "" {
		// Every teardown path is supposed to name a reason so the
		// close-reasons dashboard bucket makes sense; an empty call
		// means someone forgot to plumb a label.
		recordDebugTag(tagSessionCloseNoReason)
		return
	}
	s.closeReason.CompareAndSwap(nil, &reason)
}

// CloseReason returns the recorded close reason, or "" if none was set.
func (s *Session) CloseReason() string {
	if p := s.closeReason.Load(); p != nil {
		return *p
	}
	return ""
}

// SampleUptime records the session's current alive time into the
// session.uptime histogram. No-op if the session hasn't finished opening.
func (s *Session) SampleUptime(ctx context.Context) {
	s.tracer.sampleUptime(ctx)
}

// RecordTransportOverhead emits a per-vRPC transport-overhead sample —
// (stream − backend) — to the transport_latencies histogram.
func (s *Session) RecordTransportOverhead(ctx context.Context, method string, overhead time.Duration) {
	s.tracer.recordTransportOverhead(ctx, method, overhead)
}

// Retries returns the number of vRPCs Invoke processed with AttemptNumber > 1.
func (s *Session) Retries() int64 { return s.retries.Load() }

// OpenedAt returns when the session reached StateReady (zero until then).
func (s *Session) OpenedAt() time.Time {
	return s.tracer.openedAtSnapshot()
}

// HasOkRpcs reports whether the session served at least one successful vRPC.
func (s *Session) HasOkRpcs() bool { return s.okRpcs.Load() > 0 }

// HasErrorRpcs reports whether the session served at least one failed vRPC.
func (s *Session) HasErrorRpcs() bool { return s.errorRpcs.Load() > 0 }

// OkRpcs returns the total number of successful vRPC responses delivered
// on this session.
func (s *Session) OkRpcs() int64 { return s.okRpcs.Load() }

// ErrorRpcs returns the total number of failed vRPC responses delivered
// on this session.
func (s *Session) ErrorRpcs() int64 { return s.errorRpcs.Load() }

// MsgsSent returns the total number of frames sent on this session's
// bidi stream (every Send, regardless of payload type).
func (s *Session) MsgsSent() int64 { return s.msgsSent.Load() }

// MsgsRecv returns the total number of frames received on this session's
// bidi stream (every successful Recv).
func (s *Session) MsgsRecv() int64 { return s.msgsRecv.Load() }

// RemoteAddr returns the TCP remote (AFE) socket address in "ip:port" form,
// or "" if the stream Header hasn't been observed yet or gRPC didn't
// populate peer info.
func (s *Session) RemoteAddr() string {
	if p := s.remoteAddr.Load(); p != nil {
		return *p
	}
	return ""
}

// debugf logs at debug level if a logger has been attached.
func (s *Session) debugf(format string, args ...interface{}) {
	if s.logger == nil {
		return
	}
	btopt.Debugf(s.logger, "bigtable_session %s: "+format, append([]interface{}{s.logName}, args...)...)
}

// WithSessionLogger attaches a logger for diagnostic output. Without it,
// the session's debugf calls no-op.
func WithSessionLogger(logger *log.Logger) SessionOption {
	return func(s *Session) { s.logger = logger }
}

// WithSessionPoolName stamps the pool-scoped name used for the
// session_name label on session lifecycle metrics. Matches java-bigtable's
// per-pool SessionPoolInfo name — cardinality stays bounded by the number
// of pools per process, not per session.
func WithSessionPoolName(name string) SessionOption {
	return func(s *Session) { s.tracer.setPoolName(name) }
}

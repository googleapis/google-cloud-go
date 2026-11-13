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
	"sort"
	"sync"
	"sync/atomic"
	"time"
)

// AFE-grouping constants for the per-AFE sessionList / afeHandle.
const (
	// afePeakEwmaTau: PeakEwma decay for both per-AFE trackers. Kept
	// aligned with the per-session tau used elsewhere so LeastLatencyPicker
	// behaves comparably to today's PeakEwmaPicker.
	afePeakEwmaTau = 10 * time.Second

	// Seeds keep a brand-new AFE from looking free-cost to
	// LeastLatencyPicker before its first samples land.
	afeTransportEwmaSeed = 500 * time.Microsecond
	afeE2eEwmaSeed       = 1 * time.Millisecond

	// afePruneMaxIdle: retention for an empty (refCount==0) AFE, measured
	// from lastConnected (touched on OnSessionStarted + ReleaseToPool).
	// NOT "empty since" — a recently-active AFE is kept even if refCount
	// just hit 0.
	afePruneMaxIdle = 10 * time.Minute
)

// State model
// -----------
// A registered SessionHandle transitions through:
//
//   NotRegistered → Idle → InFlight → Closing → Closed
//                        ↺ ReleaseToPool
//
//   NotRegistered  no entry in handleToAfe.
//   Idle           handleToAfe[sh] = afe, sh in afe.sessions, inExpectedCount=true.
//   InFlight       handleToAfe[sh] = afe, sh NOT in afe.sessions, inExpectedCount=true.
//   Closing        handleToAfe[sh] = afe, sh NOT in afe.sessions, inExpectedCount=false.
//   Closed         handleToAfe has no entry for sh.
//
// Transitions:
//   NotRegistered → Idle       OnSessionStarted
//   Idle          → InFlight   CheckoutWithPicker
//   InFlight      → Idle       ReleaseToPool     (only if inExpectedCount)
//   {Idle,InFlight} → Closing  OnSessionClosing
//   {Idle,InFlight,Closing} → Closed  OnSessionClosed
//
// Invariants (all guarded by sl.mu):
//
//   I1  sh.inExpectedCount == true  ⇒  handleToAfe[sh] != nil
//   I2  readyCount == count of sh in handleToAfe with inExpectedCount==true
//   I3  afesWithReady == { afe : len(afe.sessions) > 0 } with afe.readyIdx == slice index (-1 when absent)
//   I4  afe.refCount == count of sh in handleToAfe pointing at afe (idle + inFlight + closing)
//   I5  sh in afe.sessions  ⇒  handleToAfe[sh] == afe  and  inExpectedCount==true
//   I6  refCount is decremented ONLY on OnSessionClosed (Closing keeps the slot warm)
//   I7  afe.closingCount == count of sh in handleToAfe pointing at afe with inExpectedCount==false
//
// Lock order: sl.mu ONLY. Never take pool.mu while holding sl.mu.
//
// Every *afeHandle deref in this file happens under sl.mu with one
// documented exception: RecordVRpcOutcome deliberately drops the lock
// between the map lookup and the PeakEwma.Update — see its doc.

// AfeID identifies the AFE (Application Front End) a session is pinned
// to, derived from the server's PeerInfo header at session-open time.
// Zero is the sentinel "unknown" bucket used for sessions whose
// handshake did not carry a peer-info header (older backends / tests).
type AfeID int64

// afeHandle is the per-AFE bucket in sessionList: FIFO idle queue,
// refCount (idle + inFlight + closing per I4/I6), closingCount (handles
// draining after OnSessionClosing per I7), and the two PeakEwma
// trackers LeastLatencyPicker consumes. All fields guarded by
// sessionList.mu.
type afeHandle struct {
	id           AfeID
	sessions     []*SessionHandle // idle queue, FIFO
	refCount     int              // idle + inFlight + closing (I4/I6)
	closingCount int              // handles on this AFE with inExpectedCount==false (I7)
	// readyIdx is this handle's index in sl.afesWithReady, or -1 when
	// absent. Maintained inline by OnSessionStarted / ReleaseToPool
	// (append), swapReadyAfesLocked (in-place K-choice swap), and
	// removeFromReadyLocked (swap-with-last) so removal is O(1) instead
	// of an O(N) scan-and-shift.
	readyIdx      int
	lastConnected time.Time // touched on OnSessionStarted + ReleaseToPool; drives Prune
	transportEwma *PeakEwma // OK-gated
	e2eEwma       *PeakEwma // OK-gated
}

// SessionHandle wraps a Session with the counters the pool needs to
// account for it. Picking has moved to the two-tier AFE-aware flow
// (see afe_picker.go + the sessionList methods below); this type no
// longer tracks per-session PeakEwma or a pool wake-up signal — the
// pool drives the wake-up centrally from Invoke's defer.
type SessionHandle struct {
	session      *Session
	outstanding  atomic.Int64
	lastActivity atomic.Int64 // UnixNano timestamp of the last completed call
	picks        atomic.Int64 // Number of times the picker has picked this handle.
	// createdAt is the wall-clock time this handle joined the pool
	// (stamped in createSession after the handle is minted). Read by
	// recordLifetime and Pool.Close to bucket per-session lifetimes
	// into the ring buffer. Zero for test-constructed handles that
	// never went through the pool — code paths that consume this must
	// handle the zero-time case.
	createdAt time.Time
	// inExpectedCount tracks whether this handle currently counts toward
	// sessionList.readyCount (the scale-up budget). Set true in
	// sl.OnSessionStarted, cleared by whichever of sl.OnSessionClosing /
	// sl.OnSessionClosed fires first. Guarded by owning sessionList.mu;
	// do not touch outside sl methods.
	inExpectedCount bool
	// activated / closingRecorded / closeRecorded are one-shot dedup
	// flags for the pool's per-session hook chain. They replace the
	// prior Session→SessionHandle back-ref (Session.poolHandle) whose
	// nil-ness was abused as an "already handled" signal.
	//
	// activated fires exactly once across p.onActive calls (defensive
	// against a hook chain that re-enters onActive).
	//
	// closingRecorded and closeRecorded gate the double-fire path Pool.Close
	// otherwise would trigger: Phase-1 records lifetime + sl.OnSessionClosed
	// eagerly and flips both flags, so Phase-2's s.Close driving the hook
	// chain finds them tripped and short-circuits.
	activated       atomic.Bool
	closingRecorded atomic.Bool
	closeRecorded   atomic.Bool
}

// IncOutstanding increments outstanding calls.
func (h *SessionHandle) IncOutstanding() {
	h.outstanding.Add(1)
}

// IncPicks increments the cumulative pick counter. Called from
// CheckoutSession on every successful pick so pool callers don't reach
// into the handle's atomic field directly (same shape as IncOutstanding).
func (h *SessionHandle) IncPicks() {
	h.picks.Add(1)
}

// DecOutstanding decrements outstanding calls and stamps lastActivity.
// The pool wakes waiters and returns the session to its AFE queue from
// Invoke's defer, so this method no longer signals directly.
func (h *SessionHandle) DecOutstanding() {
	h.outstanding.Add(-1)
	h.lastActivity.Store(time.Now().UnixNano())
}

// GetLastActivity returns the time of the last activity.
func (h *SessionHandle) GetLastActivity() time.Time {
	nano := h.lastActivity.Load()
	if nano == 0 {
		return time.Time{}
	}
	return time.Unix(0, nano)
}

// sessionList groups sessions by the AFE they landed on. See the State
// model block above for the handle state machine and the seven invariants
// (I1–I7) every method preserves. Lock ordering: sl.mu is always
// innermost — pool.mu is never taken while holding sl.mu
// (SESSION_COMPONENT_SPEC §B8).
type sessionList struct {
	mu               sync.Mutex
	afeHandles       map[AfeID]*afeHandle
	afesWithReady    []*afeHandle // subset with len(sessions) > 0 (I3)
	handleToAfe      map[*SessionHandle]*afeHandle
	readyCount       int // registered handles in the pool's expected set (I2); gates scale-up
	retiredOkRpcs    int64
	retiredErrorRpcs int64
}

// newSessionList returns an empty sessionList ready for use.
func newSessionList() *sessionList {
	return &sessionList{
		afeHandles:  make(map[AfeID]*afeHandle),
		handleToAfe: make(map[*SessionHandle]*afeHandle),
	}
}

// OnSessionStarted registers a newly-Active session into its AFE bucket
// (NotRegistered → Idle). Must be called after PeerInfo is populated;
// handleOpenSession guarantees this synchronously before hooks.onActive
// fires. A session whose AfeID() is 0 (server sent no peer-info header)
// lands in the AfeID=0 bucket — still pickable, but not counted in
// AFE-fanout. Duplicate register is a silent no-op — defensive guard
// against a caller wiring the hook twice; the production path fires
// OnActive exactly once per handle by construction, but tests exercise
// the dedup branch directly.
func (sl *sessionList) OnSessionStarted(sh *SessionHandle) {
	if sh == nil {
		return
	}
	if !assertDebugTagf(sh.session != nil, tagSessionListStartedNilSession,
		"OnSessionStarted called with nil Session on handle %p", sh) {
		return
	}
	id := sh.session.AfeID()
	sl.mu.Lock()
	defer sl.mu.Unlock()
	if _, dup := sl.handleToAfe[sh]; dup {
		return
	}
	afe := sl.afeHandles[id]
	if afe == nil {
		afe = &afeHandle{
			id:            id,
			readyIdx:      -1,
			transportEwma: NewPeakEwmaSeeded(afePeakEwmaTau, afeTransportEwmaSeed),
			e2eEwma:       NewPeakEwmaSeeded(afePeakEwmaTau, afeE2eEwmaSeed),
		}
		sl.afeHandles[id] = afe
	}
	if afe.enqueueLocked(sh) { // wasEmpty → afe enters the ready set (I3)
		afe.readyIdx = len(sl.afesWithReady)
		sl.afesWithReady = append(sl.afesWithReady, afe)
	}
	afe.refCount++ // I4
	afe.lastConnected = time.Now()
	sl.handleToAfe[sh] = afe
	sh.inExpectedCount = true
	sl.readyCount++ // I2
}

// CheckoutWithPicker atomically selects a ready AFE via picker and
// dequeues one idle session under a single sl.mu hold (Idle → InFlight).
// Holding sl.mu across selection and dequeue eliminates lock-split
// checkout races by construction (every entry in afesWithReady satisfies
// len(sessions) > 0 per I3), avoids allocating a per-RPC snapshot slice,
// and evaluates cost on at most K sampled AFEs instead of all N.
func (sl *sessionList) CheckoutWithPicker(picker AfePicker) (*SessionHandle, PickDecision) {
	sl.mu.Lock()
	defer sl.mu.Unlock()
	afe, decision := picker.pickLocked(sl.afesWithReady)
	if afe == nil {
		return nil, decision
	}
	sh, nowEmpty := afe.dequeueLocked()
	if sh == nil {
		return nil, PickDecision{Reason: "no-candidates"}
	}
	if nowEmpty { // afe leaves the ready set (I3)
		sl.removeFromReadyLocked(afe)
	}
	return sh, decision
}

// ReleaseToPool re-appends sh to its AFE's idle queue (InFlight → Idle),
// making it pickable again.
//
// NO-OP when the handle has already left the pool's expected set — i.e.
// OnSessionClosing fired concurrently while the RPC was in flight
// (GOAWAY, graceful Close, or ForceClose). Enforces invariant I5: only
// Ready/InFlight handles may appear in afe.sessions. Skipping this
// guard re-picks a drained session, and every next attempt hits
// "session is not active" — the WaitServerClose retry storm.
func (sl *sessionList) ReleaseToPool(sh *SessionHandle) {
	if sh == nil {
		return
	}
	sl.mu.Lock()
	defer sl.mu.Unlock()
	if !sh.inExpectedCount { // I5 enforcement — see doc.
		return
	}
	afe := sl.handleToAfe[sh]
	if afe == nil {
		return
	}
	if afe.enqueueLocked(sh) { // wasEmpty → afe re-enters the ready set (I3)
		afe.readyIdx = len(sl.afesWithReady)
		sl.afesWithReady = append(sl.afesWithReady, afe)
	}
	// Stamp on every return so Prune's retention window measures true
	// idleness, not "time since last new session opened."
	afe.lastConnected = time.Now()
}

// OnSessionClosing transitions {Idle, InFlight} → Closing. Drops the
// handle from the pool's expected set (I2), increments closingCount so
// draining handles don't inflate NumOutstanding (I7), and, if it was
// idle, removes it from its AFE's queue (I3/I5). refCount is
// intentionally NOT decremented — I6 keeps the slot warm until
// OnSessionClosed so the sizer and Prune see stable capacity across the
// close handshake.
func (sl *sessionList) OnSessionClosing(sh *SessionHandle) {
	if sh == nil {
		return
	}
	sl.mu.Lock()
	defer sl.mu.Unlock()
	afe := sl.handleToAfe[sh]
	if afe == nil {
		sl.dropMembershipLocked(sh)
		return
	}
	if sh.inExpectedCount {
		sl.dropMembershipLocked(sh) // I2
		afe.closingCount++          // I7
	}
	if removed, nowEmpty := afe.removeIfPresentLocked(sh); removed && nowEmpty {
		sl.removeFromReadyLocked(afe) // I3
	}
}

// OnSessionClosed transitions {Idle, InFlight, Closing} → Closed.
// Decrements the AFE's refCount (I4/I6) and closingCount when
// transitioning out of Closing (I7), purges handleToAfe, and removes
// the handle from the idle queue if it was still present (force-close
// paths that skip OnSessionClosing). The AFE bucket itself stays until
// Prune GCs it so brief connect/disconnect churn doesn't churn the map.
func (sl *sessionList) OnSessionClosed(sh *SessionHandle) {
	if sh == nil {
		return
	}
	sl.mu.Lock()
	defer sl.mu.Unlock()
	afe := sl.handleToAfe[sh]
	if afe == nil {
		sl.dropMembershipLocked(sh)
		return
	}
	wasClosing := !sh.inExpectedCount
	sl.dropMembershipLocked(sh) // I2 (idempotent — Closing may have fired)
	delete(sl.handleToAfe, sh)
	if sh.session != nil {
		sl.retiredOkRpcs += sh.session.okRpcs.Load()
		sl.retiredErrorRpcs += sh.session.errorRpcs.Load()
	}
	// refCount underflow here means an OnSessionClosed fired without a
	// matching OnSessionStarted — plain double-close is already blocked
	// by the delete above. Violates I4/I6: surface via debug-tag counter
	// (don't panic — transport-layer panics kill the client) and bail
	// before the decrement so refCount stays 0 instead of corrupting to
	// -1 and skewing every downstream Prune / inFlight-count decision.
	if !assertDebugTagf(afe.refCount > 0, tagSessionListRefcountUnderflow,
		"OnSessionClosed on afe=%d with refCount=%d", afe.id, afe.refCount) {
		if wasClosing && afe.closingCount > 0 {
			afe.closingCount--
		}
		return
	}
	afe.refCount-- // I4/I6
	if wasClosing && afe.closingCount > 0 {
		afe.closingCount-- // I7
	}
	if removed, nowEmpty := afe.removeIfPresentLocked(sh); removed && nowEmpty {
		sl.removeFromReadyLocked(afe) // I3
	}
}

// RecordVRpcOutcome updates the AFE's PeakEwma trackers with an OK
// response's e2e and backend latencies. Non-OK results are dropped so a
// fast-failing AFE can't game LeastLatencyPicker into steering more
// traffic at it. transportEwma tracks e2e − backend; e2eEwma tracks e2e
// directly.
//
// Deliberately drops sl.mu between the map read and the PeakEwma
// updates (the only method in this file that does): PeakEwma is
// internally locked and this runs on every completed vRPC. If a
// concurrent OnSessionClosed + Prune detaches the bucket in between,
// the update lands on a live-but-orphan tracker (afeHandles map entry
// deleted, PeakEwma struct still allocated) and is harmlessly ignored
// once GC collects the AFE.
func (sl *sessionList) RecordVRpcOutcome(sh *SessionHandle, e2e, backend time.Duration, ok bool) {
	if !ok || sh == nil {
		return
	}
	sl.mu.Lock()
	afe := sl.handleToAfe[sh]
	sl.mu.Unlock()
	if afe == nil {
		return
	}
	if e2e > 0 {
		afe.e2eEwma.Update(e2e)
	}
	if transport := e2e - backend; transport > 0 {
		afe.transportEwma.Update(transport)
	}
}

// AllHandles returns a snapshot of every registered SessionHandle —
// every state EXCEPT NotRegistered and Closed. Order is not stable
// (map iteration); callers that need order should sort by
// sh.createdAt.
func (sl *sessionList) AllHandles() []*SessionHandle {
	sl.mu.Lock()
	defer sl.mu.Unlock()
	if len(sl.handleToAfe) == 0 {
		return nil
	}
	out := make([]*SessionHandle, 0, len(sl.handleToAfe))
	for sh := range sl.handleToAfe {
		out = append(out, sh)
	}
	return out
}

// AllHandlesWithRetiredCounts atomically snapshots both the active handles
// and the cumulative ok/error RPC counts accumulated from sessions that have
// already closed, preventing negative deltas in periodic rate calculations.
func (sl *sessionList) AllHandlesWithRetiredCounts() (handles []*SessionHandle, retiredOk, retiredErr int64) {
	sl.mu.Lock()
	defer sl.mu.Unlock()
	retiredOk = sl.retiredOkRpcs
	retiredErr = sl.retiredErrorRpcs
	if len(sl.handleToAfe) == 0 {
		return nil, retiredOk, retiredErr
	}
	handles = make([]*SessionHandle, 0, len(sl.handleToAfe))
	for sh := range sl.handleToAfe {
		handles = append(handles, sh)
	}
	return handles, retiredOk, retiredErr
}

// ReadyCount returns the O(1) count that gates scale-up — handles
// still in the pool's expected set (I2).
func (sl *sessionList) ReadyCount() int {
	sl.mu.Lock()
	defer sl.mu.Unlock()
	return sl.readyCount
}

// AfeSnapshot is a value view of a ready afeHandle returned by ReadyAfes;
// hot-path selection + dequeue uses CheckoutWithPicker directly under sl.mu.
type AfeSnapshot struct {
	ID             AfeID
	IdleCount      int
	NumOutstanding int     // refCount − IdleCount − closingCount, ≥ 0 (active in-flight only)
	TransportCost  float64 // PeakEwma nanoseconds
	E2eCost        float64 // PeakEwma nanoseconds
}

// ReadyAfes returns a snapshot of AFEs with at least one idle session
// (len(afe.sessions) > 0, matching I3). Hot-path dispatch uses
// CheckoutWithPicker under sl.mu; ReadyAfes serves slow-path readiness
// re-checks and unit tests.
func (sl *sessionList) ReadyAfes() []AfeSnapshot {
	sl.mu.Lock()
	defer sl.mu.Unlock()
	if len(sl.afesWithReady) == 0 {
		return nil
	}
	out := make([]AfeSnapshot, 0, len(sl.afesWithReady))
	for _, afe := range sl.afesWithReady {
		out = append(out, AfeSnapshot{
			ID:             afe.id,
			IdleCount:      len(afe.sessions),
			NumOutstanding: afe.inFlightLocked(),
			TransportCost:  afe.transportEwma.Value(),
			E2eCost:        afe.e2eEwma.Value(),
		})
	}
	return out
}

// AfeSnapshotRow is one row of the pool's per-AFE view — mirrors the
// fields of the internal afeHandle that operators care about. Emitted
// by sessionList.Snapshot(); consumed by the debug UI (afez / sessionz)
// and by the pool's PoolSnapshot aggregate.
type AfeSnapshotRow struct {
	// ID is the AFE identifier (PeerInfo.ApplicationFrontendId). 0 is
	// the sentinel bucket used for sessions whose handshake did not
	// carry a peer-info header (older backends / tests).
	ID AfeID
	// RefCount is idle + inUse — the total number of sessions the pool
	// still tracks on this AFE. When RefCount drops to 0 the bucket
	// waits ~afePruneMaxIdle before Prune drops it.
	RefCount int
	// IdleCount is the number of sessions currently in the AFE's queue
	// (available to Checkout). InUse = RefCount − IdleCount.
	IdleCount int
	// TransportEwma is the current PeakEwma of (e2e − backend) per-AFE.
	// Only OK responses feed this (OK-gated).
	TransportEwma time.Duration
	// E2eEwma is the current PeakEwma of e2e latency per-AFE — the
	// signal LeastLatencyAfePicker uses to steer.
	E2eEwma time.Duration
	// LastConnected is the last time a session on this AFE transitioned
	// to Active or was released back to the idle queue. Drives Prune's
	// aging window and gives operators a quick "when did I last see
	// this AFE?" answer.
	LastConnected time.Time
}

// Snapshot returns a stable, human-readable view of every AFE bucket,
// sorted by ID for deterministic rendering. Single lock acquisition.
func (sl *sessionList) Snapshot() []AfeSnapshotRow {
	sl.mu.Lock()
	defer sl.mu.Unlock()
	if len(sl.afeHandles) == 0 {
		return nil
	}
	rows := make([]AfeSnapshotRow, 0, len(sl.afeHandles))
	for _, afe := range sl.afeHandles {
		idle := len(afe.sessions)
		rows = append(rows, AfeSnapshotRow{
			ID:            afe.id,
			RefCount:      afe.refCount,
			IdleCount:     idle,
			TransportEwma: time.Duration(afe.transportEwma.Value()),
			E2eEwma:       time.Duration(afe.e2eEwma.Value()),
			LastConnected: afe.lastConnected,
		})
	}
	sort.Slice(rows, func(i, j int) bool { return rows[i].ID < rows[j].ID })
	return rows
}

// Prune deletes AfeHandles that (a) have refCount == 0 AND (b) haven't
// been touched (OnSessionStarted or ReleaseToPool) within
// afePruneMaxIdle of now. AFEs with any live session (idle, in-flight,
// or closing) are never pruned — refCount includes all three per I4/I6.
func (sl *sessionList) Prune(now time.Time) {
	sl.mu.Lock()
	defer sl.mu.Unlock()
	cutoff := now.Add(-afePruneMaxIdle)
	for id, afe := range sl.afeHandles {
		if afe.refCount > 0 {
			continue
		}
		if !afe.lastConnected.IsZero() && afe.lastConnected.After(cutoff) {
			continue
		}
		// Belt-and-braces: an empty-refCount AFE should never be in
		// afesWithReady, but drop it explicitly so a slipped I3 doesn't
		// dangle a stale pointer past the delete.
		sl.removeFromReadyLocked(afe)
		delete(sl.afeHandles, id)
	}
}

// dropMembershipLocked flips sh.inExpectedCount to false and decrements
// readyCount if the flag was true. Idempotent — a second call is a
// no-op. Both OnSessionClosing and OnSessionClosed call this so a
// handle leaves the "member" set exactly once (I2), regardless of which
// path runs first or whether both run. Bails BEFORE the decrement if
// readyCount is already 0 so the underflow debug tag doesn't leave the
// counter corrupted at -1 (mirrors the OnSessionClosed refCount
// underflow guard).
func (sl *sessionList) dropMembershipLocked(sh *SessionHandle) {
	if !sh.inExpectedCount {
		return
	}
	if !assertDebugTagf(sl.readyCount > 0, tagSessionListReadyCountUnderflow,
		"dropMembershipLocked would drive readyCount below zero (inExpectedCount=true)") {
		sh.inExpectedCount = false
		return
	}
	sh.inExpectedCount = false
	sl.readyCount--
}

// removeFromReadyLocked removes afe from sl.afesWithReady in O(1) via
// readyIdx bookkeeping. No-op when afe is not present (readyIdx == -1).
// Caller must hold sl.mu. Nils the vacated tail slot so the backing
// array doesn't retain a stale bucket pointer.
func (sl *sessionList) removeFromReadyLocked(afe *afeHandle) {
	i := afe.readyIdx
	if i < 0 {
		return
	}
	last := len(sl.afesWithReady) - 1
	if i != last {
		moved := sl.afesWithReady[last]
		sl.afesWithReady[i] = moved
		moved.readyIdx = i
	}
	sl.afesWithReady[last] = nil
	sl.afesWithReady = sl.afesWithReady[:last]
	afe.readyIdx = -1
}

// inFlightLocked returns the number of active in-flight sessions on a
// (refCount minus idle and Closing handles, clamped at 0). Caller must
// hold sl.mu.
func (a *afeHandle) inFlightLocked() int {
	inflight := a.refCount - len(a.sessions) - a.closingCount
	if inflight < 0 {
		return 0
	}
	return inflight
}

// enqueueLocked appends sh to the idle FIFO. wasEmpty=true when the
// queue was empty BEFORE the append — the caller uses that to add this
// afe to afesWithReady (I3). Caller must hold sl.mu.
func (a *afeHandle) enqueueLocked(sh *SessionHandle) (wasEmpty bool) {
	wasEmpty = len(a.sessions) == 0
	a.sessions = append(a.sessions, sh)
	return
}

// dequeueLocked pops the FIFO head; returns (nil, false) when empty.
// nowEmpty=true when the queue is empty AFTER the pop — the caller uses
// that to remove this afe from afesWithReady (I3). Caller must hold
// sl.mu.
func (a *afeHandle) dequeueLocked() (sh *SessionHandle, nowEmpty bool) {
	if len(a.sessions) == 0 {
		return nil, false
	}
	sh = a.sessions[0]
	// Nil the vacated slot before shrinking the header so the backing
	// array doesn't retain a reference to the dequeued SessionHandle
	// (which transitively pins *Session, stream, ctx).
	a.sessions[0] = nil
	a.sessions = a.sessions[1:]
	nowEmpty = len(a.sessions) == 0
	return
}

// removeIfPresentLocked removes sh from the idle queue if present.
// removed=false when sh isn't in the queue (already checked out or
// never enqueued). nowEmpty is meaningful only when removed=true.
// Caller must hold sl.mu.
func (a *afeHandle) removeIfPresentLocked(sh *SessionHandle) (removed, nowEmpty bool) {
	for i, h := range a.sessions {
		if h == sh {
			last := len(a.sessions) - 1
			copy(a.sessions[i:], a.sessions[i+1:])
			a.sessions[last] = nil // GC: drop the retained tail slot
			a.sessions = a.sessions[:last]
			return true, len(a.sessions) == 0
		}
	}
	return false, false
}

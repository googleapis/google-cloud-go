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

// debug_tracer.go — cheap counter for "this branch shouldn't reach"
// sites in the session pool, session, and configuration manager. Every
// emission is one atomic add plus one OTel Int64Counter increment; safe
// to sprinkle freely on cold paths. Metric name matches java-bigtable's
// ClientDebugTagCount so cross-language dashboards can join on the tag
// column.
//
// # How to use
//
// Add a tag string constant to the catalog block below, then call one
// of the four entry points at the site.
//
// Observation — a branch that shouldn't happen but recovers cleanly.
// Adds the tag counter alongside whatever the branch already does (log,
// return, drop). No behavior change:
//
//	default:
//	    recordDebugTag(tagSessionUnknownResponse)
//	    s.debugf("received SessionResponse with unknown payload type %T", p)
//	    return
//
// Observation at a non-default level — rare; use only when the site
// really means "this is worse than a Warn but not an assert failure":
//
//	recordDebugTagAt(lvl.Error, tagSessionVRPCIDMismatch)
//
// Precondition — an invariant the caller relies on. Both forms return
// the predicate result: use as an `if !` guard so the site records +
// logs + bails in one line. Neither panics — the counter (and any err
// the caller wants to return) is the observable signal.
//
// Format-free when the tag name is enough:
//
//	if !assertDebugTag(rpc != nil, tagSessionVRPCNil) {
//	    return
//	}
//
// With formatted diagnostic context when the site has state values / ids
// worth capturing:
//
//	if !assertDebugTagf(state == StateReady || state == StateClosing,
//	    tagSessionVRPCResponseWrongState,
//	    "vRPC response for rpc_id=%d arrived in state %s", resp.RpcId, state) {
//	    return
//	}
//
// # Style rules
//
//   - Tag names are literal constants declared in the catalog below.
//     Never fmt.Sprintf into a name — dynamic context belongs in the log
//     message alongside, not the metric attribute.
//   - Emission is additive to whatever the branch already does — do
//     not delete the existing log / err / return.
//   - Default to recordDebugTag; reach for recordDebugTagAt only when
//     a non-Warn level is genuinely warranted.
//   - lvl.Error is reserved for assertDebugTag / assertDebugTagf
//     failures + the handful of explicit "this is really wrong"
//     observations. Everything else is lvl.Warn (recordDebugTag).
//   - Adding a site = one new const in the catalog block + one call.
package internal

import (
	"context"
	"fmt"
	"sort"
	"sync"
	"sync/atomic"
	"time"

	btopt "cloud.google.com/go/bigtable/internal/option"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/metric"
)

// debugLevel gates whether a debug tag emission is admitted. Only two
// levels today — no site needs finer granularity. Values chosen to
// match Java's TelemetryConfiguration.Level so future wire-config
// plumbing is a straight cast.
type debugLevel int32

// lvl exposes the debug levels as a small namespace so call sites read
// as `lvl.Warn` / `lvl.Error`. Naming `lvl` (not `tag`) intentionally —
// the tag catalog constants below use the `tag` prefix, so a namespace
// literally named `tag` would collide visually at call sites like
// `recordDebugTagAt(tag.Error, tagSessionXxx)`. Warn is the default for
// observations; Error is used by assertDebugTag failures and rare
// explicit invariant violations.
var lvl = struct {
	Warn  debugLevel
	Error debugLevel
}{
	Warn:  1,
	Error: 2,
}

// debugTagCounterName is the OTel instrument name. It is deliberately
// SHORT — the Cloud Monitoring exporter prepends the
// `bigtable.googleapis.com/internal/client/` namespace itself, so
// using the fully-qualified name here would double-prefix (and
// normalize dots to slashes in the second half), producing e.g.
// `bigtable.googleapis.com/internal/client/bigtable/googleapis/com/internal/client/debug_tags`
// which Cloud Monitoring rejects as an unknown type. The exported
// name matches java-bigtable's ClientDebugTagCount so cross-language
// dashboards can join.
const debugTagCounterName = "debug_tags"

// debugTagAttrKey is the single OTel attribute under which the tag
// string travels. Bounded cardinality — each tag is a literal constant
// declared below.
const debugTagAttrKey = "tag"

// Debug-tag catalog. Every recordDebugTag / assertDebugTag call site in
// the transport package passes one of these constants — never an inline
// literal. Adding a tag = adding a const here. Names use snake_case on
// the wire (matches java-bigtable's tag namespace) and camelCase in
// code. Grep for a tag on either the const name or the string literal
// and you find the definition + the sole emission site (usually one, at
// most a few).
const (
	// Session-lifecycle observations.
	tagSessionUnknownResponse        = "session_unknown_response"
	tagSessionOpenWrongState         = "session_open_wrong_state"
	tagSessionGoawayAfterClose       = "session_goaway_after_close"
	tagSessionGoawayBeforeStart      = "session_goaway_before_start"
	tagSessionAbnormalClose          = "session_abnormal_close"
	tagSessionHeartbeatMissed        = "session_heartbeat_missed"
	tagSessionForceCloseNeverStarted = "session_force_close_never_started"
	tagSessionCloseNoReason          = "session_close_no_reason"

	// vRPC dispatch observations.
	tagSessionVRPCNil                = "session_vrpc_nil"
	tagSessionVRPCErrorNil           = "session_vrpc_error_nil"
	tagSessionVRPCIDMismatch         = "session_vrpc_id_mismatch"
	tagSessionVRPCResponseWrongState = "session_vrpc_response_wrong_state"
	tagSessionVRPCDuplicateResult    = "session_vrpc_duplicate_result"

	// Pool-scoped anomalies.
	tagSessionPoolStuckSessionSwept = "session_pool_stuck_session_swept"
	tagSessionPoolDrainTimeout      = "session_pool_drain_timeout"
	tagSessionPoolCreateFailed      = "session_pool_create_failed"
	tagSessionPoolPickLostRace      = "session_pool_pick_lost_race"

	// Client configuration polling.
	tagClientConfigPollFailed     = "client_config_poll_failed"
	tagClientConfigPollCtxExpired = "client_config_poll_ctx_expired"
)

var (
	// debugTagCounter is the OTel Int64Counter registered inside
	// InitializeSessionMetrics. Nil until initialization runs (or if
	// initialization was called with a nil meter provider) — every
	// emission path nil-checks it, so the tracer is safe to call before
	// InitializeSessionMetrics or in tests that don't wire OTel at all.
	debugTagCounter metric.Int64Counter

	// debugTagLevelFloor is the runtime-configurable floor. Emissions
	// with level < floor are dropped without incrementing either the
	// OTel counter or the in-memory map. Defaults to Warn so no site is
	// silent by default. Set via setDebugTagLevelFloor.
	debugTagLevelFloor atomic.Int32

	// debugTagCountsMu guards debugTagStats. Contention is negligible —
	// emissions on cold paths only, and readers (tests / debugview page)
	// are rare.
	debugTagCountsMu sync.RWMutex
	// debugTagStats is the in-process view of every tag seen since
	// process start: count + first-seen + last-seen. Kept alongside the
	// OTel counter so tests and the /debugtagsz/ page can read state
	// without an OTel exporter wired up. `firstSeen` is stamped once at
	// first emission; `lastSeen` updates on every emission.
	debugTagStats = map[string]*tagStat{}
)

// tagStat holds the per-tag counters + timestamps behind debugTagStats.
// All fields are atomics so the emission hot path stays lock-free once
// the tag's entry exists in the map (map lookup under RLock, then atomic
// bumps).
type tagStat struct {
	count     atomic.Int64 // total emissions since process start
	firstSeen atomic.Int64 // unix-nano of first emission; write-once
	lastSeen  atomic.Int64 // unix-nano of most-recent emission
}

// DebugTagSnapshot is one row of the DebugTags output — a single tag's
// count plus the timestamps of its first and most-recent emissions.
// Exported for consumption by the debugview /debugtagsz/ page.
type DebugTagSnapshot struct {
	Name      string
	Count     int64
	FirstSeen time.Time
	LastSeen  time.Time
}

func init() {
	debugTagLevelFloor.Store(int32(lvl.Warn))
}

// registerDebugTagCounter is invoked exactly once from
// InitializeSessionMetrics after the meter provider is validated
// non-nil. Split out so the session_tracer init path stays readable.
func registerDebugTagCounter(meter metric.Meter) error {
	c, err := meter.Int64Counter(
		debugTagCounterName,
		metric.WithDescription("Count of unexpected events tagged by call site — the Go client's parity with java-bigtable's ClientDebugTagCount."),
	)
	if err != nil {
		return fmt.Errorf("create debug_tags counter: %w", err)
	}
	debugTagCounter = c
	return nil
}

// setDebugTagLevelFloor overrides the emission floor. Any tag whose
// level is below the floor is dropped. Intended for future wiring from
// TelemetryConfiguration.debug_tag_level; safe to call from anywhere.
func setDebugTagLevelFloor(l debugLevel) {
	debugTagLevelFloor.Store(int32(l))
}

// recordDebugTag increments the debug_tags counter for `name` at the
// default level (lvl.Warn). Use recordDebugTagAt when the emission
// genuinely warrants a different level. `name` MUST be a stable literal
// from the tag catalog above — never format values into it (dynamic
// context belongs in the log line alongside, not the metric attribute).
//
// Safe to call before InitializeSessionMetrics: the OTel counter path
// is nil-checked, so only the in-memory map increments in that window.
func recordDebugTag(name string) {
	recordDebugTagAt(lvl.Warn, name)
}

// recordDebugTagAt is the level-explicit form of recordDebugTag. Prefer
// recordDebugTag for observations (which are always Warn); reach for
// recordDebugTagAt only when a site needs to name a non-Warn level
// (e.g. an inline invariant violation outside an assertDebugTag).
func recordDebugTagAt(level debugLevel, name string) {
	if int32(level) < debugTagLevelFloor.Load() {
		return
	}
	bumpDebugTagCountLocked(name)
	if debugTagCounter != nil {
		debugTagCounter.Add(context.Background(), 1,
			metric.WithAttributes(attribute.String(debugTagAttrKey, name)))
	}
}

// assertDebugTag returns whether `expr` holds. When it doesn't, it
// records an lvl.Error tag and logs "debug-tag assertion failed [name]".
// Use assertDebugTagf when the site has diagnostic context worth
// attaching to the log line. Does NOT panic — the counter + log is the
// observable signal; the caller decides whether to bail
// (`if !assertDebugTag(...) { return }`), drop the message, or continue.
func assertDebugTag(expr bool, name string) bool {
	if expr {
		return true
	}
	recordDebugTagAt(lvl.Error, name)
	btopt.Debugf(nil, "bigtable: debug-tag assertion failed [%s]", name)
	return false
}

// assertDebugTagf is the format-string form of assertDebugTag. Use it
// when the site has diagnostic context (state values, ids, timing)
// worth attaching to the log line. Counter increment is identical to
// assertDebugTag — the format only affects the log message.
func assertDebugTagf(expr bool, name, format string, args ...interface{}) bool {
	if expr {
		return true
	}
	recordDebugTagAt(lvl.Error, name)
	btopt.Debugf(nil, "bigtable: debug-tag assertion failed [%s]: "+format, append([]interface{}{name}, args...)...)
	return false
}

// bumpDebugTagCountLocked increments the in-memory count for `name` and
// stamps the emission timestamps. First-emission case creates the entry
// under the write lock; subsequent emissions hit the RLock fast path
// and only touch atomics on the stat.
func bumpDebugTagCountLocked(name string) {
	now := time.Now().UnixNano()
	debugTagCountsMu.RLock()
	s, ok := debugTagStats[name]
	debugTagCountsMu.RUnlock()
	if ok {
		s.count.Add(1)
		s.lastSeen.Store(now)
		return
	}
	debugTagCountsMu.Lock()
	if s, ok = debugTagStats[name]; !ok {
		s = &tagStat{}
		s.firstSeen.Store(now)
		debugTagStats[name] = s
	}
	s.count.Add(1)
	s.lastSeen.Store(now)
	debugTagCountsMu.Unlock()
}

// DebugTags returns a snapshot of every tag emitted since process
// start, sorted by LastSeen descending (most-recently-fired first).
// The number of distinct tags is bounded by the catalog above (~17
// entries today), so callers can render or serialize the whole slice
// without paging.
func DebugTags() []DebugTagSnapshot {
	debugTagCountsMu.RLock()
	defer debugTagCountsMu.RUnlock()
	out := make([]DebugTagSnapshot, 0, len(debugTagStats))
	for name, s := range debugTagStats {
		out = append(out, DebugTagSnapshot{
			Name:      name,
			Count:     s.count.Load(),
			FirstSeen: time.Unix(0, s.firstSeen.Load()),
			LastSeen:  time.Unix(0, s.lastSeen.Load()),
		})
	}
	sort.Slice(out, func(i, j int) bool {
		return out[i].LastSeen.After(out[j].LastSeen)
	})
	return out
}

// snapshotDebugTagCounts returns a bare name→count map. Kept as a
// convenience for tests that only care about counts; new callers should
// use DebugTags for the richer view.
func snapshotDebugTagCounts() map[string]int64 {
	debugTagCountsMu.RLock()
	defer debugTagCountsMu.RUnlock()
	out := make(map[string]int64, len(debugTagStats))
	for name, s := range debugTagStats {
		out[name] = s.count.Load()
	}
	return out
}

// resetDebugTagCountsForTest wipes the in-memory map so a test can
// assert on a specific tag's count without cross-test contamination.
// Exported (with the _ForTest suffix) so tests in other packages under
// the transport tree can reuse it. Never call outside tests.
func resetDebugTagCountsForTest() {
	debugTagCountsMu.Lock()
	debugTagStats = map[string]*tagStat{}
	debugTagCountsMu.Unlock()
}

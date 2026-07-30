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
	"sync/atomic"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// UnimplementedThreshold is the number of CONSECUTIVE
// codes.Unimplemented responses required to trip the sticky breaker
// inside UnimplementedErrorInterceptor. Matches Java
// (ShimImpl.java:77 MAX_CONSECUTIVE_UNIMPLEMENTED_FAILURES = 30). A
// var (not const) so tests can drop it to 1-3 for fast breaker-trip
// assertions without a public setter.
//
// "Consecutive" means the counter resets to 0 on any non-Unimplemented
// session response (success OR non-Unimplemented error). Rationale: a
// non-Unimplemented response proves the RPC is understood by
// whichever backend served it (either wire is fine, or the failure is
// transport/app-level); the previous Unimplemented streak must have
// been transient or from a different AFE in the pool.
var UnimplementedThreshold int32 = 30

// UnimplementedErrorInterceptor wraps a session-path call in a
// per-call Unimplemented → classic fallback plus a threshold-counting
// sticky breaker. Owned by the routing layer (bigtable.TableShim
// today; any future routing surface with a session/classic split
// could reuse it) — one instance per resource so trip state stays
// per-resource, matching Java's per-SessionPool granularity.
//
// Two-layer behaviour, both handled by InterceptUnimplemented:
//
//   - Per-call rescue: whenever the session op returns
//     codes.Unimplemented, InterceptUnimplemented runs the classic op
//     and returns its result. The user's request always succeeds via
//     classic — even BEFORE the sticky breaker trips. Improves UX
//     over Java (which fails the request until the breaker trips) at
//     zero extra cost.
//   - Sticky breaker: after UnimplementedThreshold consecutive
//     Unimplemented responses, Bypass() returns true so callers can
//     short-circuit BEFORE dialing session at all — saving the
//     wasted round-trip on a backend that already declared it can't
//     serve the RPC.
//
// Fresh instance (via a new routing-layer construction after any
// higher-level cache TTL-evicts a handle) starts with counter=0 and
// breaker=false, so backend rollouts are implicitly re-observed.
type UnimplementedErrorInterceptor struct {
	// count is the running number of consecutive Unimplemented
	// responses. Incremented by RecordOutcome on Unimplemented; reset
	// to 0 on any other outcome. Reaches UnimplementedThreshold →
	// tripped flips true.
	count atomic.Int32
	// tripped is the sticky breaker. Flipped exactly once (via
	// CompareAndSwap) when count first reaches threshold; never reset
	// for the interceptor's lifetime. UNIMPLEMENTED is a backend
	// capability signal, not a transient failure; recovery lives in
	// the caller's re-Open path (fresh routing layer → fresh
	// interceptor).
	tripped atomic.Bool
}

// NewUnimplementedErrorInterceptor returns a fresh interceptor with
// counter=0 and breaker=false. Zero-value would work (both fields
// are atomic zero-init) but the constructor makes the initial state
// explicit at construction sites.
func NewUnimplementedErrorInterceptor() *UnimplementedErrorInterceptor {
	return &UnimplementedErrorInterceptor{}
}

// Bypass reports whether the sticky breaker has tripped. Callers use
// this to short-circuit BEFORE dialing session (typically inside a
// routing gate like TableShim.useSession). Cheap atomic Load.
func (i *UnimplementedErrorInterceptor) Bypass() bool {
	return i.tripped.Load()
}

// Count returns the current consecutive-Unimplemented count. Exposed
// primarily for tests and observability; production routing decisions
// should call Bypass() (which encapsulates the threshold comparison).
func (i *UnimplementedErrorInterceptor) Count() int32 {
	return i.count.Load()
}

// RecordOutcome updates the counter (and possibly flips the sticky
// breaker) based on the outcome of a session-path RPC.
//
// Semantics:
//   - err == nil (session succeeded) → count reset to 0.
//   - err carries any non-Unimplemented code → count reset to 0.
//     The wire understood the RPC; failure is transport / app-level.
//   - err carries codes.Unimplemented → count incremented. On the
//     transition N-1 → N == UnimplementedThreshold, flip tripped
//     via CompareAndSwap so a follow-up debug-tag / metric hook
//     fires exactly once per interceptor under concurrent trip
//     races.
//
// Matches Java's SessionPoolImpl reset semantics
// (SessionPoolImpl.java:489, 578) adapted to a per-op observation
// surface — Java observes session-close, we observe per-RPC.
//
// Exposed as a standalone method so callers with a value-returning
// op that can't be shoe-horned into InterceptUnimplemented's generic
// contract (e.g. streaming responses with mid-stream errors) can
// still feed the counter directly.
func (i *UnimplementedErrorInterceptor) RecordOutcome(err error) {
	if err == nil || status.Code(err) != codes.Unimplemented {
		i.count.Store(0)
		return
	}
	if n := i.count.Add(1); n >= UnimplementedThreshold {
		i.tripped.CompareAndSwap(false, true)
	}
}

// InterceptUnimplemented runs sessionOp, records its outcome for the
// threshold-counting breaker, and either returns its result or — if
// the result carries codes.Unimplemented — falls back to classicOp
// and returns THAT result. Never returns a mixed session/classic
// tuple: the caller gets exactly one path's (T, error).
//
// Generic over T so both value-returning ops (e.g. a Row) and void
// ops (e.g. Apply, with T = struct{}) share one implementation
// without closure-capture-through-a-var boilerplate at each call
// site.
//
// A free function (not a method) because Go 1.24 still forbids
// methods with additional type parameters — the receiver's type is
// fixed at method declaration. See
// https://github.com/golang/go/issues/49085 for the ongoing
// discussion; when methods with type params land, this can become
// (i *UnimplementedErrorInterceptor) Intercept[T].
func InterceptUnimplemented[T any](
	i *UnimplementedErrorInterceptor,
	sessionOp func() (T, error),
	classicOp func() (T, error),
) (T, error) {
	resp, err := sessionOp()
	i.RecordOutcome(err)
	if err != nil && status.Code(err) == codes.Unimplemented {
		return classicOp()
	}
	return resp, err
}

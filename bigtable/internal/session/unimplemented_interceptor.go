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

// UnimplementedThreshold is the number of consecutive
// codes.Unimplemented responses required to trip the sticky breaker.
// Var (not const) so tests can lower it without a public setter.
var UnimplementedThreshold int32 = 30

// UnimplementedErrorInterceptor wraps a session-path call with:
//   - per-call classic fallback on codes.Unimplemented (the caller's
//     request always succeeds via classic, even before the breaker trips)
//   - a sticky breaker that trips after UnimplementedThreshold
//     consecutive Unimplemented responses; Bypass() then lets callers
//     short-circuit before dialing session again
//
// One instance per resource. Any non-Unimplemented response (success
// or other error) resets the consecutive count — a non-Unimplemented
// reply proves the RPC is understood by whichever backend served it.
type UnimplementedErrorInterceptor struct {
	count   atomic.Int32
	tripped atomic.Bool
}

// NewUnimplementedErrorInterceptor returns an interceptor with a
// zeroed counter and un-tripped breaker.
func NewUnimplementedErrorInterceptor() *UnimplementedErrorInterceptor {
	return &UnimplementedErrorInterceptor{}
}

// Bypass reports whether the sticky breaker has tripped. Cheap atomic
// Load; callers use it to skip session before dialing.
func (i *UnimplementedErrorInterceptor) Bypass() bool {
	return i.tripped.Load()
}

// Count returns the current consecutive-Unimplemented count. Exposed
// for tests and observability; routing should call Bypass().
func (i *UnimplementedErrorInterceptor) Count() int32 {
	return i.count.Load()
}

// RecordOutcome updates the counter (and possibly trips the breaker)
// based on a session-path RPC's outcome:
//
//   - nil or non-Unimplemented err → count resets to 0
//   - codes.Unimplemented → count increments; on reaching
//     UnimplementedThreshold, trip via CompareAndSwap so a follow-up
//     debug-tag / metric hook fires exactly once
func (i *UnimplementedErrorInterceptor) RecordOutcome(err error) {
	if err == nil || status.Code(err) != codes.Unimplemented {
		i.count.Store(0)
		return
	}
	if n := i.count.Add(1); n >= UnimplementedThreshold {
		i.tripped.CompareAndSwap(false, true)
	}
}

// InterceptUnimplemented runs sessionOp, records its outcome, and —
// on codes.Unimplemented — returns classicOp's result instead. Never
// mixes results: the caller gets exactly one path's (T, error).
//
// Generic over T so value-returning ops (Row) and void ops
// (Apply, with T = struct{}) share one implementation.
//
// Free function because Go still forbids methods with additional type
// parameters (https://github.com/golang/go/issues/49085).
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

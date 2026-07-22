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
	"time"

	"google.golang.org/genproto/googleapis/rpc/errdetails"
	"google.golang.org/grpc/status"
)

// RetryingOptions configures the retry behavior of RetryingVRpc.
type RetryingOptions struct {
	// MaxAttempts caps the total number of tries per Invoke (initial +
	// retries). Defaults to 3 for Java parity — RetryingVRpc.java
	// hardcodes a 3-attempt cap. Read once at RetryingVRpc construction
	// and captured in the closure; the server-driven client config has
	// no channel to swap this today (no matching proto field), so
	// mutating it after RetryingVRpc(...) returns has no effect on
	// in-flight loops.
	MaxAttempts       int32
	InitialBackoff    time.Duration
	MaxBackoff        time.Duration
	BackoffMultiplier float64
	Tracer            VRpcTracer
	// Idempotent tells the interceptor whether TransportFailure attempts
	// (frame handed to the wire, no server response observed) are safe to
	// retry. Reads set this true; non-idempotent Apply sets it false.
	// Uncommitted attempts (never left the client) retry regardless.
	// Ignored if ShouldRetry is non-nil.
	Idempotent bool
	// ShouldRetry, if non-nil, overrides the default state-based check.
	// Callers with unusual retry policies use it; the default (nil)
	// applies Java-parity classification via AttemptState + Idempotent.
	//
	// Server-attached RetryInfo is checked BEFORE ShouldRetry and always
	// wins — a server-directed retry is honored even if ShouldRetry
	// would otherwise return false (SESSION_SPEC #9 "server-only
	// inputs"). ShouldRetry sees only errors the server did not
	// explicitly grant retry for.
	ShouldRetry func(error) bool
}

// RetryingVRpc returns an Interceptor that retries failed virtual RPCs with
// exponential delay backoffs and optional custom server RetryInfo parsing.
func RetryingVRpc(opts RetryingOptions) Interceptor {
	if opts.MaxAttempts <= 0 {
		opts.MaxAttempts = 3
	}
	if opts.InitialBackoff <= 0 {
		opts.InitialBackoff = 20 * time.Millisecond
	}
	if opts.MaxBackoff <= 0 {
		opts.MaxBackoff = 32 * time.Second
	}
	if opts.BackoffMultiplier <= 0 {
		opts.BackoffMultiplier = 1.3
	}

	maxAttempts := opts.MaxAttempts

	return func(ctx context.Context, req interface{}, next Handler) (interface{}, error) {
		var attempt int32
		backoff := opts.InitialBackoff
		tracer := opts.Tracer // may be nil

		var lastErr error

		for {
			attempt++
			if attempt > maxAttempts {
				break
			}

			attemptCtx := WithAttempt(ctx, int(attempt))
			// Tag the next attempt's context with the prior attempt's
			// error so the downstream Session.Invoke can record a
			// "retry" SessionEvent carrying the original gRPC code +
			// message. lastErr is nil on the very first attempt.
			if lastErr != nil {
				attemptCtx = WithPrevAttemptErr(attemptCtx, lastErr)
			}

			if tracer != nil {
				tracer.OnAttemptStart(attemptCtx)
			}

			res, err := next(attemptCtx, req)

			if tracer != nil {
				tracer.OnAttemptComplete(attemptCtx, err)
			}

			if err == nil {
				return res, nil
			}

			lastErr = err

			// SESSION_SPEC #9 "last-observed err preserved": Java's
			// RetryingVRpc returns the last attempt's typed error, not
			// raw context.Canceled/DeadlineExceeded. lastErr carries the
			// gRPC code + AttemptState tag callers need.
			if ctx.Err() != nil {
				return nil, lastErr
			}

			var delay time.Duration
			hasServerDelay := false

			s, hasStatus := status.FromError(err)
			// Server RetryInfo is checked first so a server-directed delay
			// permits retry even for otherwise-non-retryable outcomes
			// (e.g. server DEADLINE_EXCEEDED that the server explicitly
			// says is safe to retry).
			serverPermitsRetry := false
			if hasStatus {
				for _, detail := range s.Details() {
					if retryInfo, ok := detail.(*errdetails.RetryInfo); ok {
						if retryInfo.GetRetryDelay().IsValid() {
							delay = retryInfo.GetRetryDelay().AsDuration()
							hasServerDelay = true
							serverPermitsRetry = true
							break
						}
					}
				}
			}

			// Server RetryInfo is server-only authority (SESSION_SPEC #9
			// "server-only inputs"): if the server explicitly grants
			// retry, honor it regardless of any caller-supplied
			// ShouldRetry policy or state-based default. Falling into
			// either gate would let a client-side rule veto the server.
			if !serverPermitsRetry {
				if opts.ShouldRetry != nil {
					if !opts.ShouldRetry(err) {
						return nil, err
					}
				} else if !shouldRetryDefault(err, opts.Idempotent) {
					return nil, err
				}
			}

			if !hasServerDelay {
				delay = backoff
				nextBackoff := float64(backoff) * opts.BackoffMultiplier
				if nextBackoff > float64(opts.MaxBackoff) {
					backoff = opts.MaxBackoff
				} else {
					backoff = time.Duration(nextBackoff)
				}
			}

			// SESSION_SPEC #9 "deadline-fit check": if the delay would
			// exhaust the caller's remaining deadline, skip the retry
			// entirely and surface lastErr — Java parity
			// (RetryingVRpc.java:290-298). Waiting past the deadline
			// only to have the next attempt immediately fail with
			// DeadlineExceeded loses the typed error and burns budget.
			// Applied to any delay (server RetryInfo or client
			// backoff) — the rationale is the same in either case.
			if dl, ok := ctx.Deadline(); ok && time.Until(dl) < delay {
				return nil, lastErr
			}

			// time.NewTimer + Stop() (rather than time.After) so a
			// ctx-cancel exit path releases the timer immediately
			// instead of leaking it until `delay` elapses.
			timer := time.NewTimer(delay)
			select {
			case <-ctx.Done():
				timer.Stop()
				// Preserve lastErr on the backoff-window cancel, same
				// rationale as the post-attempt ctx.Err() branch above.
				return nil, lastErr
			case <-timer.C:
			}
		}

		return nil, lastErr
	}
}

// shouldRetryDefault applies strict Java-parity retry classification for
// the default retry path (no caller-supplied ShouldRetry, no server
// RetryInfo). Server RetryInfo is handled by the caller before this fn
// is consulted — its short-circuit lives at the interceptor level so the
// custom-ShouldRetry path honors it uniformly (SESSION_SPEC #9).
//
// Retry rules (see AttemptState doc for state semantics):
//   - Uncommitted → always retry (server saw nothing).
//   - TransportFailure → retry only if idempotent (server may have applied).
//   - ServerResult → NEVER retry from this path. A bare server-explicit
//     error — even Unavailable / Aborted / DeadlineExceeded — is NOT
//     retried without an explicit server go-ahead (RetryInfo handled
//     upstream). This matches Java's RetryingVRpc: the server said
//     something specific; the client doesn't second-guess it.
//
// Callers that need the pre-parity permissive behavior (retry on
// {Aborted, Internal, ResourceExhausted, Unavailable} without RetryInfo)
// set RetryingOptions.ShouldRetry.
func shouldRetryDefault(err error, idempotent bool) bool {
	outcome := ClassifyErr(err)
	switch outcome.State {
	case StateUncommitted:
		return true
	case StateTransportFailure:
		return idempotent
	case StateServerResult:
		return false
	}
	return false
}

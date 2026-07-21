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
	"sync/atomic"
	"time"

	"google.golang.org/genproto/googleapis/rpc/errdetails"
	"google.golang.org/grpc/status"
)

// RetryingOptions configures the retry behavior of RetryingVRpc.
type RetryingOptions struct {
	MaxAttempts       int32 // Atomic race cap for max attempts.
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
	// ShouldRetry, if non-nil, overrides the default state-based check
	// entirely. Callers with unusual retry policies use it; the default
	// (nil) applies Java-parity classification via AttemptState +
	// Idempotent + server-provided RetryInfo.
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

	return func(ctx context.Context, req interface{}, next Handler) (interface{}, error) {
		var attempt int32
		backoff := opts.InitialBackoff
		tracer := opts.Tracer // may be nil

		var lastErr error

		for {
			currentAttempt := atomic.AddInt32(&attempt, 1)
			if currentAttempt > atomic.LoadInt32(&opts.MaxAttempts) {
				break
			}

			attemptCtx := WithAttempt(ctx, int(currentAttempt))
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

			if ctx.Err() != nil {
				return nil, ctx.Err()
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

			if opts.ShouldRetry != nil {
				if !opts.ShouldRetry(err) {
					return nil, err
				}
			} else if !shouldRetryDefault(err, opts.Idempotent, serverPermitsRetry) {
				return nil, err
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

			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			case <-time.After(delay):
			}
		}

		return nil, lastErr
	}
}

// shouldRetryDefault applies strict Java-parity retry classification.
// Callers with a bespoke policy set RetryingOptions.ShouldRetry to bypass
// this entirely.
//
// Retry rules (see AttemptState doc for state semantics):
//   - Uncommitted → always retry (server saw nothing).
//   - TransportFailure → retry only if idempotent (server may have applied).
//   - ServerResult → retry ONLY if the server attached RetryInfo (checked
//     by the caller and passed in as serverPermitsRetry). A bare
//     server-explicit error — even Unavailable / Aborted / DeadlineExceeded —
//     is NOT retried without an explicit server go-ahead. This matches
//     Java's RetryingVRpc: the server said something specific; the client
//     doesn't second-guess it.
//
// Callers that need the pre-parity permissive behavior (retry on
// {Aborted, Internal, ResourceExhausted, Unavailable} without RetryInfo)
// set RetryingOptions.ShouldRetry.
func shouldRetryDefault(err error, idempotent, serverPermitsRetry bool) bool {
	if serverPermitsRetry {
		return true
	}
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

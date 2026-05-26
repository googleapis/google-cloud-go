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
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// RetryingOptions configures the retry behavior of RetryingVRpc.
type RetryingOptions struct {
	MaxAttempts       int32 // Atomic race cap for max attempts.
	InitialBackoff    time.Duration
	MaxBackoff        time.Duration
	BackoffMultiplier float64
	Listener          VRpcListener
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

		var shieldedListener *closedListenerShield
		if opts.Listener != nil {
			shieldedListener = &closedListenerShield{listener: opts.Listener}
			defer shieldedListener.Close()
		}

		var lastErr error

		for {
			currentAttempt := atomic.AddInt32(&attempt, 1)
			if currentAttempt > atomic.LoadInt32(&opts.MaxAttempts) {
				break
			}

			attemptCtx := WithAttempt(ctx, int(currentAttempt))

			if shieldedListener != nil {
				shieldedListener.OnAttemptStart(attemptCtx)
			}

			res, err := next(attemptCtx, req)

			if shieldedListener != nil {
				shieldedListener.OnAttemptComplete(attemptCtx, err)
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

			if s, ok := status.FromError(err); ok {
				if !isRetryableCode(s.Code()) {
					return nil, err
				}

				for _, detail := range s.Details() {
					if retryInfo, ok := detail.(*errdetails.RetryInfo); ok {
						if retryInfo.GetRetryDelay().IsValid() {
							delay = retryInfo.GetRetryDelay().AsDuration()
							hasServerDelay = true
							break
						}
					}
				}
			} else {
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


func isRetryableCode(code codes.Code) bool {
	switch code {
	case codes.Aborted, codes.DeadlineExceeded, codes.Internal, codes.ResourceExhausted, codes.Unavailable:
		return true
	default:
		return false
	}
}

type closedListenerShield struct {
	listener VRpcListener
	closed   int32
}

func (s *closedListenerShield) Close() {
	atomic.StoreInt32(&s.closed, 1)
}

func (s *closedListenerShield) isClosed() bool {
	return atomic.LoadInt32(&s.closed) == 1
}

func (s *closedListenerShield) OnAttemptStart(ctx context.Context) {
	if !s.isClosed() && s.listener != nil {
		s.listener.OnAttemptStart(ctx)
	}
}

func (s *closedListenerShield) OnAttemptComplete(ctx context.Context, err error) {
	if !s.isClosed() && s.listener != nil {
		s.listener.OnAttemptComplete(ctx, err)
	}
}
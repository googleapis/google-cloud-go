// Copyright 2021 Google LLC
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     https://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package managedwriter

import (
	"context"
	"errors"
	"time"

	"github.com/googleapis/gax-go/v2"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

const (
	// DefaultMaxRetries is used as the default for the maxRetries property of Retryer,
	// used in case property is 0.
	DefaultMaxRetries = -1

	// DefaultInitialRetryDelay is used as the default for the InitialRetryDuration property
	// of Retryer, used in case the property is 0 (e.g. when undefined).
	//
	// Default based on suggestions made in https://cloud.google.com/bigquery/sla.
	DefaultInitialRetryDelay = time.Second * 1

	// DefaultMaxRetryDeadlineOffset is the default max amount of the the retryer will
	// allow the retry back off logic to retry, as to ensure a goroutine isn't blocked for too long on a faulty write.
	//
	// Default based on suggestions made in https://cloud.google.com/bigquery/sla.
	DefaultMaxRetryDeadlineOffset = time.Second * 32

	// DefaultRetryDelayMultiplier is the default retry delay multipler used by the defaultRetryer's
	// back off algorithm in order to increase the delay in between each sequential write-retry of the
	// same back off sequence. Used in case the property is < 2, as 2 is also the lowest possible multiplier accepted.
	DefaultRetryDelayMultiplier = 2
)

// Retryer is the default gax.Retryer type as used by
// the managed writer for its GRPC retryable operations.
type Retryer struct {
	backoff                gax.Backoff
	retries                int
	maxRetries             int
	startTime              time.Time
	maxRetryDeadlineOffset time.Duration
	deadlineCtx            context.Context
	cancelDeadlineCtx      func()
}

// compile-time interface compliance
var _ gax.Retryer = (*Retryer)(nil)

// not exposed as there is no point in defining this retryer,
// if all you want is the default retryer
func newDefaultRetryer(ctx context.Context) *Retryer {
	return NewRetryer(
		ctx,
		DefaultMaxRetries,
		DefaultInitialRetryDelay,
		DefaultMaxRetryDeadlineOffset,
		DefaultRetryDelayMultiplier,
	)
}

// NewRetryer creates a new Retryer, the packaged `gax.Retryer` implementation
// shipped with the bqwriter package. See the documentation of `Retryer` for more information
// on how it is implemented why it should be used.
func NewRetryer(ctx context.Context, maxRetries int, initialRetryDelay time.Duration, maxRetryDeadlineOffset time.Duration, retryDelayMultiplier float64) *Retryer {
	if maxRetries <= 0 {
		maxRetries = DefaultMaxRetries
	}
	if initialRetryDelay == 0 {
		initialRetryDelay = DefaultInitialRetryDelay
	}
	if maxRetryDeadlineOffset == 0 {
		maxRetryDeadlineOffset = DefaultMaxRetryDeadlineOffset
	}
	if retryDelayMultiplier <= 1 {
		retryDelayMultiplier = DefaultRetryDelayMultiplier
	}
	startTime := time.Now()
	deadlineCtx, cancelDeadlineCtx := context.WithDeadline(ctx, startTime.Add(maxRetryDeadlineOffset))
	return &Retryer{
		backoff: gax.Backoff{
			Initial:    initialRetryDelay,
			Max:        maxRetryDeadlineOffset,
			Multiplier: retryDelayMultiplier,
		},
		maxRetries:             maxRetries,
		startTime:              startTime,
		maxRetryDeadlineOffset: maxRetryDeadlineOffset,
		deadlineCtx:            deadlineCtx,
		cancelDeadlineCtx:      cancelDeadlineCtx,
	}
}

func (r *Retryer) Retry(err error) (pause time.Duration, shouldRetry bool) {
	defer func() {
		if !shouldRetry {
			r.cancelDeadlineCtx()
		}
	}()
	if err == nil {
		// no error returned, no need to retry
		return 0, false
	}
	if errors.Is(r.deadlineCtx.Err(), context.Canceled) {
		// if parent ctx is done or the deadline has been reached,
		// no retry is possible any longer either
		return 0, false
	}
	if r.maxRetries >= 0 && r.retries >= r.maxRetries {
		// no longer need to retry,
		// already exhausted our retry attempts
		return 0, false
	}
	if !grcpRetryErrorFilter(err) {
		// we do not wish to retry this kind of error either,
		// as it is not detected as retryable by us
		return 0, false
	}
	// correct the Max time, as to stay as close as possible to our max elapsed retry time
	elapsedTime := time.Since(r.startTime)
	r.backoff.Max = r.maxRetryDeadlineOffset - elapsedTime
	// retry with the pause time indicated by the gax BackOff algorithm
	r.retries += 1
	return r.backoff.Pause(), true
}

func grcpRetryErrorFilter(err error) bool {
	st, ok := status.FromError(err)
	if !ok {
		return false
	}
	switch st.Code() {
	case codes.Unavailable, codes.FailedPrecondition, codes.ResourceExhausted, codes.DataLoss:
		return true
	default:
		return false
	}
}

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
	"testing"
	"time"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func TestDefaultRetryerNoRetryBecauseOfNilError(t *testing.T) {
	retryer := NewRetryer(
		context.Background(),
		DefaultMaxRetries,
		DefaultInitialRetryDelay,
		DefaultMaxRetryDeadlineOffset,
		DefaultRetryDelayMultiplier,
	)
	pause, shouldRetry := retryer.Retry(nil)
	if shouldRetry {
		t.Error("should not retry")
	}
	if pause != 0 {
		t.Errorf("unexpected pause duration received: %v", pause)
	}
}

func TestDefaultRetryerDefaults(t *testing.T) {
	testCases := []struct {
		MaxRetries             int
		InitialRetryDelay      time.Duration
		MaxRetryDeadlineOffset time.Duration
		RetryDelayMultiplier   float64

		ExpectedMaxRetries             int
		ExpectedInitialRetryDelay      time.Duration
		ExpectedMaxRetryDeadlineOffset time.Duration
		ExpectedRetryDelayMultiplier   float64
	}{
		{
			0, 0, 0, 0,
			DefaultMaxRetries, DefaultInitialRetryDelay, DefaultMaxRetryDeadlineOffset, DefaultRetryDelayMultiplier,
		},
		{
			-1, 0, 0, 0,
			-1, DefaultInitialRetryDelay, DefaultMaxRetryDeadlineOffset, DefaultRetryDelayMultiplier,
		},
		{
			0, 0, 0, 1,
			DefaultMaxRetries, DefaultInitialRetryDelay, DefaultMaxRetryDeadlineOffset, DefaultRetryDelayMultiplier,
		},
		{
			0, 0, 0, 3,
			DefaultMaxRetries, DefaultInitialRetryDelay, DefaultMaxRetryDeadlineOffset, 3,
		},
		{
			0, 0, 42, 0,
			DefaultMaxRetries, DefaultInitialRetryDelay, 42, DefaultRetryDelayMultiplier,
		},
		{
			0, 20, 0, 0,
			DefaultMaxRetries, 20, DefaultMaxRetryDeadlineOffset, DefaultRetryDelayMultiplier,
		},
	}
	err := status.New(codes.DataLoss, "static retry error").Err()
	for testIndex, testCase := range testCases {
		retryer := NewRetryer(
			context.Background(),
			testCase.MaxRetries,
			testCase.InitialRetryDelay,
			testCase.MaxRetryDeadlineOffset,
			testCase.RetryDelayMultiplier,
		)
		if retryer.maxRetries != testCase.ExpectedMaxRetries {
			t.Errorf("%d) unexpected MaxRetries: %v != %v\n ... %v", testIndex, retryer.maxRetries, testCase.ExpectedMaxRetries, testCase)
		}
		if retryer.backoff.Max != testCase.ExpectedMaxRetryDeadlineOffset {
			t.Errorf("%d) unexpected MaxRetryDeadlineOffset: %v != %v\n ... %v", testIndex, retryer.backoff.Max, testCase.ExpectedMaxRetryDeadlineOffset, testCase)
		}
		if retryer.backoff.Initial != testCase.ExpectedInitialRetryDelay {
			t.Errorf("%d) unexpected InitialRetryDelay: %v != %v\n ... %v", testIndex, retryer.backoff.Initial, testCase.ExpectedInitialRetryDelay, testCase)
		}
		if retryer.backoff.Multiplier != testCase.ExpectedRetryDelayMultiplier {
			t.Errorf("%d) unexpected RetryDelayMultiplier: %v != %v\n ... %v", testIndex, retryer.backoff.Multiplier, testCase.ExpectedRetryDelayMultiplier, testCase)
		}
		pause, shouldRetry := retryer.Retry(err)
		if !shouldRetry {
			t.Errorf("%d) should retry", testIndex)
		}
		if pause == 0 {
			t.Errorf("%d) unexpected pause duration received: %v", testIndex, pause)
		}
	}
}

func TestDefaultRetryerNoRetryBecauseOfCanceledContext(t *testing.T) {
	ctx, cancelFunc := context.WithCancel(context.Background())
	cancelFunc()
	retryer := NewRetryer(
		ctx,
		DefaultMaxRetries,
		DefaultInitialRetryDelay,
		DefaultMaxRetryDeadlineOffset,
		DefaultRetryDelayMultiplier,
	)
	err := status.New(codes.DataLoss, "static retry error").Err()
	pause, shouldRetry := retryer.Retry(err)
	if shouldRetry {
		t.Error("should not retry")
	}
	if pause != 0 {
		t.Errorf("unexpected pause duration received: %v", pause)
	}
}

func TestDefaultRetryerNoRetryBecauseOfMaxRetries(t *testing.T) {
	retryer := NewRetryer(
		context.Background(),
		1, // retry max 1 time
		DefaultInitialRetryDelay,
		DefaultMaxRetryDeadlineOffset,
		DefaultRetryDelayMultiplier,
	)

	err := status.New(codes.DataLoss, "static retry error").Err()

	// first time will work
	pause, shouldRetry := retryer.Retry(err)
	if !shouldRetry {
		t.Error("should retry")
	}
	if pause == 0 || pause > DefaultInitialRetryDelay {
		t.Errorf("unexpeted pause duration: %v", pause)
	}

	// second time not, as we reached our limit of max retries
	pause, shouldRetry = retryer.Retry(err)
	if shouldRetry {
		t.Error("should not retry")
	}
	if pause != 0 {
		t.Errorf("unexpected pause duration received: %v", pause)
	}
}

func TestGRPCRetryErrorFilterTrue(t *testing.T) {
	testCases := []codes.Code{
		codes.Unavailable,
		codes.FailedPrecondition,
		codes.ResourceExhausted,
		codes.DataLoss,
	}
	for _, testCase := range testCases {
		err := status.New(testCase, "test error").Err()
		if !grcpRetryErrorFilter(err) {
			t.Errorf("err should trigger filter: %v", err)
		}
	}
}

func TestGRPCRetryErrorFilterFalse(t *testing.T) {
	// nil error is not an accepted error
	if grcpRetryErrorFilter(nil) {
		t.Error("nil error should not trigger filter")
	}
	// custom error is not an accepted error
	if grcpRetryErrorFilter(errors.New("todo")) {
		t.Error("custom error should not trigger filter")
	}
	// correct error, but wrong code
	err := status.New(codes.Aborted, "test error").Err()
	if grcpRetryErrorFilter(err) {
		t.Error("status error with non-retryable code should not trigger filter")
	}
}

/*
Copyright 2017 Google LLC

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package spanner

import (
	"context"
	"testing"
	"time"

	"github.com/golang/protobuf/ptypes"
	"github.com/googleapis/gax-go/v2"
	edpb "google.golang.org/genproto/googleapis/rpc/errdetails"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func TestRetryInfo(t *testing.T) {
	s := status.New(codes.Aborted, "")
	s, err := s.WithDetails(&edpb.RetryInfo{
		RetryDelay: ptypes.DurationProto(time.Second),
	})
	if err != nil {
		t.Fatalf("Error setting retry details: %v", err)
	}
	gotDelay, ok := ExtractRetryDelay(toSpannerErrorWithCommitInfo(s.Err(), true))
	if !ok || !testEqual(time.Second, gotDelay) {
		t.Errorf("<ok, retryDelay> = <%t, %v>, want <true, %v>", ok, gotDelay, time.Second)
	}
}

func TestRetryInfoInWrappedError(t *testing.T) {
	s := status.New(codes.Aborted, "")
	s, err := s.WithDetails(&edpb.RetryInfo{
		RetryDelay: ptypes.DurationProto(time.Second),
	})
	if err != nil {
		t.Fatalf("Error setting retry details: %v", err)
	}
	gotDelay, ok := ExtractRetryDelay(
		&wrappedTestError{wrapped: toSpannerErrorWithCommitInfo(s.Err(), true), msg: "Error that is wrapping a Spanner error"},
	)
	if !ok || !testEqual(time.Second, gotDelay) {
		t.Errorf("<ok, retryDelay> = <%t, %v>, want <true, %v>", ok, gotDelay, time.Second)
	}
}

func TestRetryInfoTransactionOutcomeUnknownError(t *testing.T) {
	err := toSpannerErrorWithCommitInfo(context.DeadlineExceeded, true)
	if gotDelay, ok := ExtractRetryDelay(err); ok {
		t.Errorf("Got unexpected delay\nGot: %v\nWant: %v", gotDelay, 0)
	}
	if !testEqual(err.(*Error).err, &TransactionOutcomeUnknownError{status.FromContextError(context.DeadlineExceeded).Err()}) {
		t.Errorf("Missing expected TransactionOutcomeUnknownError wrapped error")
	}
}

func TestRetryerRespectsServerDelay(t *testing.T) {
	t.Parallel()
	serverDelay := 50 * time.Millisecond
	s := status.New(codes.Aborted, "transaction was aborted")
	s, err := s.WithDetails(&edpb.RetryInfo{
		RetryDelay: ptypes.DurationProto(serverDelay),
	})
	if err != nil {
		t.Fatalf("Error setting retry details: %v", err)
	}
	retryer := onCodes(gax.Backoff{}, codes.Aborted)
	err = toSpannerErrorWithCommitInfo(s.Err(), true)
	maxSeenDelay, shouldRetry := retryer.Retry(err)
	if !shouldRetry {
		t.Fatalf("expected shouldRetry to be true")
	}
	if maxSeenDelay != serverDelay {
		t.Fatalf("Retry delay mismatch:\ngot: %v\nwant: %v", maxSeenDelay, serverDelay)
	}
}

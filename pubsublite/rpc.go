// Copyright 2020 Google LLC
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

package pubsublite

import (
	"time"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	gax "github.com/googleapis/gax-go/v2"
)

// streamRetryer implements the retry policy for establishing gRPC stream
// connections.
type streamRetryer struct {
	bo       gax.Backoff
	deadline time.Time
}

func newStreamRetryer(timeout time.Duration) *streamRetryer {
	return &streamRetryer{
		bo: gax.Backoff{
			Initial:    10 * time.Millisecond,
			Max:        10 * time.Second,
			Multiplier: 2,
		},
		deadline: time.Now().Add(timeout),
	}
}

func (r *streamRetryer) RetrySend(err error) (time.Duration, bool) {
	if time.Now().After(r.deadline) {
		return 0, false
	}
	if isRetryableSendError(err) {
		return r.bo.Pause(), true
	}
	return 0, false
}

func (r *streamRetryer) RetryRecv(err error) (time.Duration, bool) {
	if time.Now().After(r.deadline) {
		return 0, false
	}
	if isRetryableRecvError(err) {
		return r.bo.Pause(), true
	}
	return 0, false
}

func isRetryableSendCode(code codes.Code) bool {
	switch code {
	// Client-side errors that occur during grpc.ClientStream.SendMsg() have a
	// smaller set of retryable codes.
	case codes.DeadlineExceeded, codes.Unavailable:
		return true
	default:
		return false
	}
}

func isRetryableRecvCode(code codes.Code) bool {
	switch code {
	// Consistent with https://github.com/googleapis/java-pubsublite/blob/master/google-cloud-pubsublite/src/main/java/com/google/cloud/pubsublite/ErrorCodes.java
	case codes.Aborted, codes.DeadlineExceeded, codes.Internal, codes.ResourceExhausted, codes.Unavailable, codes.Unknown:
		return true
	default:
		return false
	}
}

func isRetryableSendError(err error) bool {
	return isRetryableStreamError(err, isRetryableSendCode)
}

func isRetryableRecvError(err error) bool {
	return isRetryableStreamError(err, isRetryableRecvCode)
}

func isRetryableStreamError(err error, isEligible func(codes.Code) bool) bool {
	s, ok := status.FromError(err)
	if !ok {
		// Includes io.EOF, normal stream close.
		// Consistent with https://github.com/googleapis/google-cloud-go/blob/master/pubsub/service.go
		return true
	}
	return isEligible(s.Code())
}

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

package storage

import (
	"context"
	"strconv"
	"strings"
	"sync"

	"github.com/googleapis/gax-go/v2/callctx"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/metric"
	"google.golang.org/grpc/metadata"
)

// The retry loops (run in invoke.go for HTTP and gRPC, and the resumable
// upload chunk loop in google.golang.org/api/internal/gensupport) identify
// each request with gccl-invocation-id and number its attempts with
// gccl-attempt-count in the x-goog-api-client header. gccl-attempt-count
// restarts at 1 for every chunk of a resumable upload and every page of a
// listing, so an attempt with a count greater than 1 is always a retry of a
// failed attempt of the same request, never a regular additional request.
const (
	invocationIDToken = "gccl-invocation-id/"
	attemptCountToken = "gccl-attempt-count/"
)

// retryInfo identifies an attempt within its request.
type retryInfo struct {
	invocationID string
	attempt      int
}

// isRetry reports whether the attempt retried an earlier attempt.
func (r retryInfo) isRetry() bool { return r.attempt > 1 }

// parseRetryInfo extracts the invocation ID and attempt number from the
// values of the x-goog-api-client header. Missing tokens yield the zero value
// (attempt 0), which is never counted as a retry.
func parseRetryInfo(vals []string) retryInfo {
	var r retryInfo
	for _, v := range vals {
		for _, tok := range strings.Fields(v) {
			switch {
			case strings.HasPrefix(tok, invocationIDToken):
				r.invocationID = tok[len(invocationIDToken):]
			case strings.HasPrefix(tok, attemptCountToken):
				if a, err := strconv.Atoi(tok[len(attemptCountToken):]); err == nil && a > r.attempt {
					r.attempt = a
				}
			}
		}
	}
	return r
}

// grpcRetryInfo returns the retry information of the gRPC call in ctx. The
// header is set by the storage retry loop through callctx and may also be
// present in the outgoing metadata.
func grpcRetryInfo(ctx context.Context) retryInfo {
	r := parseRetryInfo(callctx.HeadersFromContext(ctx)[xGoogHeaderKey])
	if md, ok := metadata.FromOutgoingContext(ctx); ok {
		if m := parseRetryInfo(md.Get(xGoogHeaderKey)); m.attempt > r.attempt {
			r = m
		}
	}
	return r
}

// retryTracker remembers, per request of an operation, the error.type of the
// last failed attempt so that a subsequent retry can be attributed to it.
// Requests are keyed by invocation ID because one operation can have several
// requests in flight (parallel composite upload parts, multi-range download
// ranges). The tracker lives in the operation's metricsState and is released
// with it.
type retryTracker struct {
	mu   sync.Mutex
	last map[string]string // invocation ID -> error.type of the last failed attempt
}

// observe records the outcome of an attempt and returns the error.type of the
// attempt it retried, or "" if the attempt was not a retry. A retry whose
// previous failure was not observed (for example when the request was started
// before metrics were enabled) is attributed to errorTypeUnknown.
func (t *retryTracker) observe(info retryInfo, errorType string) (reason string) {
	if t == nil {
		if info.isRetry() {
			return errorTypeUnknown
		}
		return ""
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	if info.isRetry() {
		reason = t.last[info.invocationID]
		if reason == "" {
			reason = errorTypeUnknown
		}
	}
	if info.invocationID == "" {
		return reason
	}
	if errorType == errorTypeOK {
		delete(t.last, info.invocationID)
		return reason
	}
	if t.last == nil {
		t.last = make(map[string]string)
	}
	t.last[info.invocationID] = errorType
	return reason
}

// recordRetry records gcp.storage.client.retries for an attempt that retried
// an earlier failed attempt of the same request. error.type is the error of
// the attempt that was retried, i.e. the reason for the retry.
func (cm *clientMetrics) recordRetry(ctx context.Context, system, method, server string, info retryInfo, errorType string) {
	if cm == nil || cm.retries == nil {
		return
	}
	var tracker *retryTracker
	if state := metricsStateFromContext(ctx); state != nil {
		tracker = state.retries()
	}
	reason := tracker.observe(info, errorType)
	if reason == "" {
		return
	}
	cm.retries.Add(ctx, 1, metric.WithAttributes(injectAPIMethod(ctx, []attribute.KeyValue{
		attribute.String("rpc.system.name", system),
		attribute.String("rpc.method", method),
		attribute.String("server.address", server),
		attribute.String("error.type", reason),
	})...))
}

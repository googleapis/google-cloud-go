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

type retryEntry struct {
	lastAttempt int
	lastErr     string
}

// retryTracker remembers, per request of an operation, the error.type of the
// last failed attempt so that a subsequent retry can be attributed to it.
// Tracking lastAttempt per invocation ID ensures multi-RPC attempts sharing one
// invocation ID (such as QueryWriteStatus followed by BidiWriteObject inside
// writeLoop) emit at most one retry per attempt number and preserve prior
// failure reasons across intermediate probes.
type retryTracker struct {
	mu      sync.Mutex
	entries map[string]retryEntry
}

// observe records the outcome of an attempt and returns the error.type of the
// attempt it retried, or "" if the attempt was not a retry (or was already
// counted for this attempt number). A retry whose previous failure was not
// observed is attributed to "UNKNOWN".
func (t *retryTracker) observe(info retryInfo, errorType string) (reason string) {
	if t == nil || info.invocationID == "" {
		if info.isRetry() {
			return "UNKNOWN"
		}
		return ""
	}
	t.mu.Lock()
	defer t.mu.Unlock()

	entry := t.entries[info.invocationID]
	// Reset the per-attempt high-water mark when gccl-attempt-count restarts
	// (for a new resumable chunk, list page, or gRPCWriter progress reset)
	// so retries on later chunks are not suppressed.
	if info.attempt > 0 && info.attempt < entry.lastAttempt {
		entry.lastAttempt = 0
	}
	if info.isRetry() && (info.attempt > entry.lastAttempt || (info.attempt == entry.lastAttempt && entry.lastErr != "")) {
		reason = entry.lastErr
		if reason == "" {
			reason = "UNKNOWN"
		}
	}
	if info.attempt > entry.lastAttempt {
		entry.lastAttempt = info.attempt
	}
	if errorType == "OK" {
		entry.lastErr = ""
	} else {
		entry.lastErr = errorType
	}
	if entry.lastAttempt <= 1 && entry.lastErr == "" {
		delete(t.entries, info.invocationID)
		return reason
	}
	if t.entries == nil {
		t.entries = make(map[string]retryEntry)
	}
	t.entries[info.invocationID] = entry
	return reason
}

// recordFailure updates the last error for an invocation (for example when a
// 200 OK HTTP response later fails mid-body with io.ErrUnexpectedEOF) so a
// subsequent retry attributes to that failure rather than "UNKNOWN".
func (t *retryTracker) recordFailure(info retryInfo, errorType string) {
	if t == nil || info.invocationID == "" || errorType == "" || errorType == "OK" {
		return
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.entries == nil {
		t.entries = make(map[string]retryEntry)
	}
	entry := t.entries[info.invocationID]
	if info.attempt > entry.lastAttempt {
		entry.lastAttempt = info.attempt
	}
	entry.lastErr = errorType
	t.entries[info.invocationID] = entry
}

// recordRetry records gcp.storage.client.retries for an attempt that retried
// an earlier failed attempt of the same request.
func (cm *clientMetrics) recordRetry(ctx context.Context, system, method, server, path string, info retryInfo, errorType string) {
	if cm == nil || cm.retries == nil {
		return
	}
	reason := metricsStateFromContext(ctx).retryState().observe(info, errorType)
	if reason == "" {
		return
	}
	cm.recordRetryReason(ctx, system, method, server, path, reason)
}

func (cm *clientMetrics) recordRetryReason(ctx context.Context, system, method, server, path, reason string) {
	if cm == nil || cm.retries == nil {
		return
	}
	cm.retries.Add(ctx, 1, metric.WithAttributes(
		attribute.String("rpc.system.name", system),
		attribute.String(storageOperationAttr, method),
		attribute.String("server.address", server),
		attribute.String(networkPathAttr, path),
		attribute.String("error.type", reason),
	))
}

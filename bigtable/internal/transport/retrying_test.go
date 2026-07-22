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
	"errors"
	"sync"
	"testing"
	"time"

	"google.golang.org/genproto/googleapis/rpc/errdetails"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/durationpb"
)

// mockTracer captures per-attempt tracer callbacks for assertions.
type mockTracer struct {
	mu             sync.Mutex
	starts         []int
	completes      []int
	completeErrors []error
}

func (m *mockTracer) OnAttemptStart(ctx context.Context) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.starts = append(m.starts, VRpcAttempt(ctx))
}

func (m *mockTracer) OnAttemptComplete(ctx context.Context, err error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.completes = append(m.completes, VRpcAttempt(ctx))
	m.completeErrors = append(m.completeErrors, err)
}

func TestRetryingVRpc_SuccessOnRetry(t *testing.T) {
	var attempts int
	baseHandler := func(ctx context.Context, req interface{}) (interface{}, error) {
		attempts++
		attemptInCtx := VRpcAttempt(ctx)
		if attemptInCtx != attempts {
			t.Errorf("Expected attempt %d in context, got %d", attempts, attemptInCtx)
		}

		if attempts < 3 {
			// TransportFailure matches what session_vrpc.go produces for a
			// mid-stream Unavailable (session-side cancellation, GoAway,
			// heartbeat miss). Idempotent=true gates the retry.
			return nil, tagErr(StateTransportFailure, status.Error(codes.Unavailable, "service temporarily unavailable"))
		}
		return "success_resp", nil
	}

	tracer := &mockTracer{}
	retryInterceptor := RetryingVRpc(RetryingOptions{
		MaxAttempts:    5,
		InitialBackoff: 1 * time.Millisecond,
		Tracer:         tracer,
		Idempotent:     true,
	})

	ctx := WithVRpcMetadata(context.Background(), "TestMethod", 1)
	resp, err := retryInterceptor(ctx, "request", baseHandler)
	if err != nil {
		t.Fatalf("Expected success, got error: %v", err)
	}

	if resp.(string) != "success_resp" {
		t.Errorf("Expected response 'success_resp', got %q", resp)
	}

	if attempts != 3 {
		t.Errorf("Expected 3 attempts, got %d", attempts)
	}

	tracer.mu.Lock()
	defer tracer.mu.Unlock()

	expectedStarts := []int{1, 2, 3}
	if len(tracer.starts) != len(expectedStarts) {
		t.Errorf("Expected %d starts, got %d", len(expectedStarts), len(tracer.starts))
	}
	for i, v := range expectedStarts {
		if tracer.starts[i] != v {
			t.Errorf("Expected start attempt at index %d to be %d, got %d", i, v, tracer.starts[i])
		}
	}

	expectedCompletes := []int{1, 2, 3}
	if len(tracer.completes) != len(expectedCompletes) {
		t.Errorf("Expected %d completes, got %d", len(expectedCompletes), len(tracer.completes))
	}
}

func TestRetryingVRpc_NonRetryableError(t *testing.T) {
	var attempts int
	baseHandler := func(ctx context.Context, req interface{}) (interface{}, error) {
		attempts++
		return nil, status.Error(codes.InvalidArgument, "invalid parameter value")
	}

	retryInterceptor := RetryingVRpc(RetryingOptions{
		MaxAttempts:    5,
		InitialBackoff: 1 * time.Millisecond,
	})

	ctx := WithVRpcMetadata(context.Background(), "TestMethod", 1)
	_, err := retryInterceptor(ctx, "request", baseHandler)
	if err == nil {
		t.Fatal("Expected error, got nil")
	}

	st, ok := status.FromError(err)
	if !ok || st.Code() != codes.InvalidArgument {
		t.Errorf("Expected InvalidArgument status error, got %v", err)
	}

	if attempts != 1 {
		t.Errorf("Expected non-retryable error to abort immediately (1 attempt), got %d attempts", attempts)
	}
}

func TestRetryingVRpc_HonorServerRetryDelay(t *testing.T) {
	var attempts int
	baseHandler := func(ctx context.Context, req interface{}) (interface{}, error) {
		attempts++
		if attempts == 1 {
			// Server-returned error carrying RetryInfo — permits retry
			// regardless of state (Java parity: server explicitly said
			// "try again in Nms"). Tag as ServerResult to match the real
			// production path (handleVRPCErrorResponse).
			st := status.New(codes.Unavailable, "overloaded")
			stWithDetails, err := st.WithDetails(&errdetails.RetryInfo{
				RetryDelay: durationpb.New(10 * time.Millisecond),
			})
			if err != nil {
				return nil, err
			}
			return nil, tagErr(StateServerResult, stWithDetails.Err())
		}
		return "ok", nil
	}

	retryInterceptor := RetryingVRpc(RetryingOptions{
		MaxAttempts:    3,
		InitialBackoff: 200 * time.Millisecond, // Default initial backoff is large, but server details should override it
	})

	startTime := time.Now()
	ctx := WithVRpcMetadata(context.Background(), "TestMethod", 1)
	resp, err := retryInterceptor(ctx, "request", baseHandler)
	duration := time.Since(startTime)

	if err != nil {
		t.Fatalf("Expected successful response, got %v", err)
	}

	if resp.(string) != "ok" {
		t.Errorf("Expected response 'ok', got %v", resp)
	}

	if duration >= 100*time.Millisecond {
		t.Errorf("Expected retry delay to be small (10ms) honoring server RetryInfo, but took %v (might have used 200ms client backoff)", duration)
	}
}

func TestRetryingVRpc_MaxAttemptsExceeded(t *testing.T) {
	var attempts int
	baseHandler := func(ctx context.Context, req interface{}) (interface{}, error) {
		attempts++
		return nil, tagErr(StateTransportFailure, status.Error(codes.Unavailable, "temporary failure"))
	}

	retryInterceptor := RetryingVRpc(RetryingOptions{
		MaxAttempts:    3,
		InitialBackoff: 1 * time.Millisecond,
		Idempotent:     true,
	})

	ctx := WithVRpcMetadata(context.Background(), "TestMethod", 1)
	_, err := retryInterceptor(ctx, "request", baseHandler)
	if err == nil {
		t.Fatal("Expected failure after maximum attempts, got success")
	}

	if attempts != 3 {
		t.Errorf("Expected exactly 3 attempts before giving up, got %d", attempts)
	}
}

func TestRetryingVRpc_NonGrpcError(t *testing.T) {
	var attempts int
	baseHandler := func(ctx context.Context, req interface{}) (interface{}, error) {
		attempts++
		return nil, errors.New("some custom raw go error")
	}

	retryInterceptor := RetryingVRpc(RetryingOptions{
		MaxAttempts:    3,
		InitialBackoff: 1 * time.Millisecond,
	})

	ctx := WithVRpcMetadata(context.Background(), "TestMethod", 1)
	_, err := retryInterceptor(ctx, "request", baseHandler)
	if err == nil {
		t.Fatal("Expected error, got nil")
	}

	if attempts != 1 {
		t.Errorf("Expected non-gRPC raw errors to be treated as non-retryable and fail immediately (1 attempt), got %d attempts", attempts)
	}
}

// TestRetryingVRpc_UncommittedAlwaysRetries verifies that an attempt tagged
// as StateUncommitted retries even when Idempotent is false. Uncommitted
// means the request never reached the wire; a retry cannot double-apply.
func TestRetryingVRpc_UncommittedAlwaysRetries(t *testing.T) {
	var attempts int
	baseHandler := func(ctx context.Context, req interface{}) (interface{}, error) {
		attempts++
		if attempts < 3 {
			return nil, tagErr(StateUncommitted, status.Error(codes.Unavailable, "session closing before Send"))
		}
		return "ok", nil
	}

	retryInterceptor := RetryingVRpc(RetryingOptions{
		MaxAttempts:    5,
		InitialBackoff: 1 * time.Millisecond,
		Idempotent:     false, // non-idempotent: uncommitted still retries
	})

	ctx := WithVRpcMetadata(context.Background(), "TestMethod", 1)
	resp, err := retryInterceptor(ctx, "req", baseHandler)
	if err != nil {
		t.Fatalf("expected success, got %v", err)
	}
	if resp.(string) != "ok" {
		t.Errorf("expected 'ok', got %v", resp)
	}
	if attempts != 3 {
		t.Errorf("expected 3 attempts, got %d", attempts)
	}
}

// TestRetryingVRpc_TransportFailureIdempotent verifies that TransportFailure
// retries for an idempotent op (reads).
func TestRetryingVRpc_TransportFailureIdempotent(t *testing.T) {
	var attempts int
	baseHandler := func(ctx context.Context, req interface{}) (interface{}, error) {
		attempts++
		if attempts < 3 {
			return nil, tagErr(StateTransportFailure, status.Error(codes.Unavailable, "send failed"))
		}
		return "ok", nil
	}

	retryInterceptor := RetryingVRpc(RetryingOptions{
		MaxAttempts:    5,
		InitialBackoff: 1 * time.Millisecond,
		Idempotent:     true,
	})

	ctx := WithVRpcMetadata(context.Background(), "TestMethod", 1)
	_, err := retryInterceptor(ctx, "req", baseHandler)
	if err != nil {
		t.Fatalf("expected success, got %v", err)
	}
	if attempts != 3 {
		t.Errorf("expected 3 attempts, got %d", attempts)
	}
}

// TestRetryingVRpc_TransportFailureNonIdempotentNoRetry verifies that a
// TransportFailure on a non-idempotent op (Apply of a ServerTime mutation)
// does NOT retry — the server may have already applied the mutation, and
// a retry would create a duplicate cell.
func TestRetryingVRpc_TransportFailureNonIdempotentNoRetry(t *testing.T) {
	var attempts int
	baseHandler := func(ctx context.Context, req interface{}) (interface{}, error) {
		attempts++
		return nil, tagErr(StateTransportFailure, status.Error(codes.Unavailable, "wire error"))
	}

	retryInterceptor := RetryingVRpc(RetryingOptions{
		MaxAttempts:    5,
		InitialBackoff: 1 * time.Millisecond,
		Idempotent:     false,
	})

	ctx := WithVRpcMetadata(context.Background(), "TestMethod", 1)
	_, err := retryInterceptor(ctx, "req", baseHandler)
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if attempts != 1 {
		t.Errorf("expected exactly 1 attempt (no retry for non-idempotent TransportFailure), got %d", attempts)
	}
}

// TestRetryingVRpc_ServerResultNotRetriedByDefault verifies strict
// Java parity for the ServerResult path: a server-explicit error is NOT
// retried without server-attached RetryInfo, regardless of gRPC code.
// The permissive pre-parity behavior (retry on Unavailable / Aborted /
// Internal / ResourceExhausted / DeadlineExceeded) is gone; callers who
// need it must set RetryingOptions.ShouldRetry.
func TestRetryingVRpc_ServerResultNotRetriedByDefault(t *testing.T) {
	cases := []struct {
		name string
		code codes.Code
	}{
		{"Unavailable", codes.Unavailable},
		{"Aborted", codes.Aborted},
		{"Internal", codes.Internal},
		{"ResourceExhausted", codes.ResourceExhausted},
		{"DeadlineExceeded", codes.DeadlineExceeded},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var attempts int
			baseHandler := func(ctx context.Context, req interface{}) (interface{}, error) {
				attempts++
				return nil, tagErr(StateServerResult, status.Error(tc.code, "server said no"))
			}
			retryInterceptor := RetryingVRpc(RetryingOptions{
				MaxAttempts:    5,
				InitialBackoff: 1 * time.Millisecond,
				Idempotent:     true, // even idempotent: ServerResult without RetryInfo never retries
			})
			ctx := WithVRpcMetadata(context.Background(), "TestMethod", 1)
			_, err := retryInterceptor(ctx, "req", baseHandler)
			if err == nil {
				t.Fatal("expected error, got nil")
			}
			if attempts != 1 {
				t.Errorf("expected 1 attempt (bare ServerResult %s must not retry), got %d",
					tc.name, attempts)
			}
		})
	}
}

// TestRetryingVRpc_ServerDeadlineExceededNoRetryByDefault verifies Java
// parity: a server-returned DEADLINE_EXCEEDED is NOT retried by default.
// The server said "I gave up"; blindly retrying burns budget on ops the
// server already discarded.
func TestRetryingVRpc_ServerDeadlineExceededNoRetryByDefault(t *testing.T) {
	var attempts int
	baseHandler := func(ctx context.Context, req interface{}) (interface{}, error) {
		attempts++
		return nil, tagErr(StateServerResult, status.Error(codes.DeadlineExceeded, "server timed out"))
	}

	retryInterceptor := RetryingVRpc(RetryingOptions{
		MaxAttempts:    5,
		InitialBackoff: 1 * time.Millisecond,
		Idempotent:     true, // even idempotent: server DEADLINE_EXCEEDED still no retry
	})

	ctx := WithVRpcMetadata(context.Background(), "TestMethod", 1)
	_, err := retryInterceptor(ctx, "req", baseHandler)
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if attempts != 1 {
		t.Errorf("expected 1 attempt (server DEADLINE_EXCEEDED not retried), got %d", attempts)
	}
}

// TestRetryingVRpc_ServerDeadlineExceededRetriesWithRetryInfo verifies the
// escape hatch: if the server explicitly attaches RetryInfo, the client
// honors it — even for DEADLINE_EXCEEDED — because the server has stated
// the retry is safe and given a specific backoff.
func TestRetryingVRpc_ServerDeadlineExceededRetriesWithRetryInfo(t *testing.T) {
	var attempts int
	baseHandler := func(ctx context.Context, req interface{}) (interface{}, error) {
		attempts++
		if attempts == 1 {
			st := status.New(codes.DeadlineExceeded, "overloaded, retry later")
			stWithDetails, derr := st.WithDetails(&errdetails.RetryInfo{
				RetryDelay: durationpb.New(1 * time.Millisecond),
			})
			if derr != nil {
				return nil, derr
			}
			return nil, tagErr(StateServerResult, stWithDetails.Err())
		}
		return "ok", nil
	}

	retryInterceptor := RetryingVRpc(RetryingOptions{
		MaxAttempts:    3,
		InitialBackoff: 1 * time.Millisecond,
		Idempotent:     true,
	})

	ctx := WithVRpcMetadata(context.Background(), "TestMethod", 1)
	resp, err := retryInterceptor(ctx, "req", baseHandler)
	if err != nil {
		t.Fatalf("expected success, got %v", err)
	}
	if resp.(string) != "ok" {
		t.Errorf("expected 'ok', got %v", resp)
	}
	if attempts != 2 {
		t.Errorf("expected 2 attempts, got %d", attempts)
	}
}

// TestRetryingVRpc_CtxCancelPreservesLastErr pins SESSION_SPEC #9's
// "last-observed err preserved" rule: when the caller ctx cancels
// between attempts (after a real RPC failure), the returned error MUST
// be the typed lastErr (carrying the gRPC code + AttemptState tag), not
// raw context.Canceled. Regression would strip the retry oracle's view
// of what actually failed.
func TestRetryingVRpc_CtxCancelPreservesLastErr(t *testing.T) {
	sentinel := tagErr(StateTransportFailure, status.Error(codes.Unavailable, "wire error"))
	baseHandler := func(ctx context.Context, req interface{}) (interface{}, error) {
		return nil, sentinel
	}
	ctx, cancel := context.WithCancel(WithVRpcMetadata(context.Background(), "TestMethod", 1))

	retryInterceptor := RetryingVRpc(RetryingOptions{
		MaxAttempts:    5,
		InitialBackoff: 50 * time.Millisecond,
		Idempotent:     true,
	})

	// Cancel after the first attempt lands but before the backoff timer fires.
	go func() {
		time.Sleep(5 * time.Millisecond)
		cancel()
	}()

	_, err := retryInterceptor(ctx, "req", baseHandler)
	if !errors.Is(err, sentinel) {
		t.Fatalf("expected lastErr sentinel, got %v", err)
	}
	if errors.Is(err, context.Canceled) {
		t.Errorf("expected typed lastErr, not raw context.Canceled: %v", err)
	}
}

// TestRetryingVRpc_DeadlineFitSkipsRetry pins SESSION_SPEC #9's
// "deadline-fit check": when the next delay would exhaust the caller's
// remaining deadline, skip the retry and surface lastErr. Sleeping past
// the deadline only to have the retry immediately fail with
// DeadlineExceeded burns budget and loses the typed error.
func TestRetryingVRpc_DeadlineFitSkipsRetry(t *testing.T) {
	sentinel := tagErr(StateServerResult, mustStatusWithRetryInfo(t, codes.Unavailable, "overloaded", 5*time.Second))
	attempts := 0
	baseHandler := func(ctx context.Context, req interface{}) (interface{}, error) {
		attempts++
		return nil, sentinel
	}

	// 20ms remaining, server asks for a 5s delay — skip.
	ctx, cancel := context.WithTimeout(WithVRpcMetadata(context.Background(), "TestMethod", 1), 20*time.Millisecond)
	defer cancel()

	retryInterceptor := RetryingVRpc(RetryingOptions{
		MaxAttempts:    5,
		InitialBackoff: 1 * time.Millisecond,
		Idempotent:     true,
	})

	start := time.Now()
	_, err := retryInterceptor(ctx, "req", baseHandler)
	elapsed := time.Since(start)

	if !errors.Is(err, sentinel) {
		t.Fatalf("expected lastErr sentinel, got %v", err)
	}
	if attempts != 1 {
		t.Errorf("expected 1 attempt (deadline-fit skip), got %d", attempts)
	}
	if elapsed > 100*time.Millisecond {
		t.Errorf("expected fast return (no 5s sleep), took %v", elapsed)
	}
}

// TestRetryingVRpc_ServerRetryInfoOverridesShouldRetry pins SESSION_SPEC
// #9's "server-only inputs" contract: server RetryInfo is server-only
// authority and MUST override any caller-supplied ShouldRetry callback.
// Without this, a strict client policy could veto a server-directed
// retry — losing the whole point of the RetryInfo detail.
func TestRetryingVRpc_ServerRetryInfoOverridesShouldRetry(t *testing.T) {
	attempts := 0
	baseHandler := func(ctx context.Context, req interface{}) (interface{}, error) {
		attempts++
		if attempts == 1 {
			return nil, tagErr(StateServerResult, mustStatusWithRetryInfo(t,
				codes.DeadlineExceeded, "overloaded, retry", 1*time.Millisecond))
		}
		return "ok", nil
	}

	// ShouldRetry says "never retry"; the server's RetryInfo must win.
	retryInterceptor := RetryingVRpc(RetryingOptions{
		MaxAttempts:    5,
		InitialBackoff: 1 * time.Millisecond,
		ShouldRetry:    func(error) bool { return false },
	})

	ctx := WithVRpcMetadata(context.Background(), "TestMethod", 1)
	resp, err := retryInterceptor(ctx, "req", baseHandler)
	if err != nil {
		t.Fatalf("expected success (server RetryInfo overrides ShouldRetry), got %v", err)
	}
	if resp.(string) != "ok" {
		t.Errorf("expected 'ok', got %v", resp)
	}
	if attempts != 2 {
		t.Errorf("expected 2 attempts, got %d", attempts)
	}
}

func mustStatusWithRetryInfo(t *testing.T, code codes.Code, msg string, delay time.Duration) error {
	t.Helper()
	st, err := status.New(code, msg).WithDetails(&errdetails.RetryInfo{
		RetryDelay: durationpb.New(delay),
	})
	if err != nil {
		t.Fatalf("WithDetails: %v", err)
	}
	return st.Err()
}

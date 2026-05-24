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

type mockListener struct {
	mu              sync.Mutex
	starts          []int
	completes       []int
	completeErrors  []error
}

func (m *mockListener) OnAttemptStart(ctx context.Context) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.starts = append(m.starts, VRpcAttempt(ctx))
}

func (m *mockListener) OnAttemptComplete(ctx context.Context, err error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.completes = append(m.completes, VRpcAttempt(ctx))
	m.completeErrors = append(m.completeErrors, err)
}

func TestChainedInterceptors_Success(t *testing.T) {
	var order []string

	int1 := func(ctx context.Context, req interface{}, next Handler) (interface{}, error) {
		order = append(order, "int1_start")
		res, err := next(ctx, req)
		order = append(order, "int1_end")
		return res, err
	}

	int2 := func(ctx context.Context, req interface{}, next Handler) (interface{}, error) {
		order = append(order, "int2_start")
		res, err := next(ctx, req)
		order = append(order, "int2_end")
		return res, err
	}

	baseHandler := func(ctx context.Context, req interface{}) (interface{}, error) {
		order = append(order, "base")
		return "response", nil
	}

	chained := ChainInterceptors(int1, int2)
	ctx := WithVRpcMetadata(context.Background(), "TestMethod", 1)

	resp, err := chained(ctx, "request", baseHandler)
	if err != nil {
		t.Fatalf("Expected no error, got %v", err)
	}

	if resp.(string) != "response" {
		t.Errorf("Expected response to be 'response', got %v", resp)
	}

	expectedOrder := []string{"int1_start", "int2_start", "base", "int2_end", "int1_end"}
	if len(order) != len(expectedOrder) {
		t.Fatalf("Expected order length %d, got %d", len(expectedOrder), len(order))
	}
	for i, v := range expectedOrder {
		if order[i] != v {
			t.Errorf("At index %d: expected %q, got %q", i, v, order[i])
		}
	}
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
			return nil, status.Error(codes.Unavailable, "service temporarily unavailable")
		}
		return "success_resp", nil
	}

	listener := &mockListener{}
	retryInterceptor := RetryingVRpc(RetryingOptions{
		MaxAttempts:    5,
		InitialBackoff: 1 * time.Millisecond,
		Listener:       listener,
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

	listener.mu.Lock()
	defer listener.mu.Unlock()

	expectedStarts := []int{1, 2, 3}
	if len(listener.starts) != len(expectedStarts) {
		t.Errorf("Expected %d starts, got %d", len(expectedStarts), len(listener.starts))
	}
	for i, v := range expectedStarts {
		if listener.starts[i] != v {
			t.Errorf("Expected start attempt at index %d to be %d, got %d", i, v, listener.starts[i])
		}
	}

	expectedCompletes := []int{1, 2, 3}
	if len(listener.completes) != len(expectedCompletes) {
		t.Errorf("Expected %d completes, got %d", len(expectedCompletes), len(listener.completes))
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
			// Return transient error with server-directed retry delay of 10ms
			st := status.New(codes.Unavailable, "overloaded")
			stWithDetails, err := st.WithDetails(&errdetails.RetryInfo{
				RetryDelay: durationpb.New(10 * time.Millisecond),
			})
			if err != nil {
				return nil, err
			}
			return nil, stWithDetails.Err()
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

func TestVRpc_ContextMetadataRetention(t *testing.T) {
	ctx := WithVRpcMetadata(context.Background(), "QueryMethod", 42)

	if VRpcMethod(ctx) != "QueryMethod" {
		t.Errorf("Expected method 'QueryMethod', got %q", VRpcMethod(ctx))
	}
	if VRpcAttempt(ctx) != 42 {
		t.Errorf("Expected attempt 42, got %d", VRpcAttempt(ctx))
	}

	// Wrapping with context.WithValue (e.g. standard tracing span setter)
	type extraKey struct{}
	wrappedCtx := context.WithValue(ctx, extraKey{}, "extraValue")

	// Ensure metadata is completely preserved when using standard context operations
	if VRpcMethod(wrappedCtx) != "QueryMethod" {
		t.Errorf("Expected method 'QueryMethod' to be preserved after WithValue wrapping, got %q", VRpcMethod(wrappedCtx))
	}
	if VRpcAttempt(wrappedCtx) != 42 {
		t.Errorf("Expected attempt 42 to be preserved after WithValue wrapping, got %d", VRpcAttempt(wrappedCtx))
	}
}

func TestRetryingVRpc_MaxAttemptsExceeded(t *testing.T) {
	var attempts int
	baseHandler := func(ctx context.Context, req interface{}) (interface{}, error) {
		attempts++
		return nil, status.Error(codes.Unavailable, "temporary failure")
	}

	retryInterceptor := RetryingVRpc(RetryingOptions{
		MaxAttempts:    3,
		InitialBackoff: 1 * time.Millisecond,
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

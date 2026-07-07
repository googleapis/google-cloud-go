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
	"errors"
	"net/http"
	"reflect"
	"testing"

	"cloud.google.com/go/storage/experimental"
	"google.golang.org/api/option"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

func TestRetryContextSignatures(t *testing.T) {
	t.Run("legacy signature func(error) bool", func(t *testing.T) {
		errFunc := func(err error) bool {
			return err != nil
		}

		config := &retryConfig{}
		WithErrorFunc(errFunc).apply(config)

		if config.shouldRetry == nil {
			t.Errorf("Expected shouldRetry to be set for valid signature")
		}
	})

	t.Run("new signature func(error, *RetryContext) bool", func(t *testing.T) {
		errFunc := func(err error, ctx *RetryContext) bool {
			return err != nil && ctx.Attempt < 3
		}

		config := &retryConfig{}
		WithErrorFuncWithContext(errFunc).apply(config)

		if config.shouldRetry == nil {
			t.Errorf("Expected shouldRetry to be set for valid signature")
		}
	})
}

func TestRetryContextFields(t *testing.T) {
	var capturedCtx *RetryContext

	errFunc := func(err error, ctx *RetryContext) bool {
		capturedCtx = ctx
		return false // don't actually retry
	}

	config := &retryConfig{}
	WithErrorFuncWithContext(errFunc).apply(config)

	testErr := errors.New("test error")
	ctx := &RetryContext{
		Attempt:      2,
		InvocationID: "test-id-123",
		Operation:    "GetObject",
		Bucket:       "test-bucket",
		Object:       "test-object",
	}

	config.shouldRetry(testErr, ctx)

	if capturedCtx == nil {
		t.Fatal("RetryContext was not passed to error function")
	}

	if capturedCtx.Attempt != 2 {
		t.Errorf("Attempt: got %d, want 2", capturedCtx.Attempt)
	}
	if capturedCtx.InvocationID != "test-id-123" {
		t.Errorf("InvocationID: got %q, want %q", capturedCtx.InvocationID, "test-id-123")
	}
	if capturedCtx.Operation != "GetObject" {
		t.Errorf("Operation: got %q, want %q", capturedCtx.Operation, "GetObject")
	}
	if capturedCtx.Bucket != "test-bucket" {
		t.Errorf("Bucket: got %q, want %q", capturedCtx.Bucket, "test-bucket")
	}
	if capturedCtx.Object != "test-object" {
		t.Errorf("Object: got %q, want %q", capturedCtx.Object, "test-object")
	}
}

func TestLegacySignatureStillWorks(t *testing.T) {
	var called bool

	errFunc := func(err error) bool {
		called = true
		return false
	}

	config := &retryConfig{}
	WithErrorFunc(errFunc).apply(config)

	testErr := errors.New("test error")
	ctx := &RetryContext{
		Attempt:      1,
		InvocationID: "id",
		Operation:    "op",
		Bucket:       "bucket",
		Object:       "object",
	}

	config.shouldRetry(testErr, ctx)

	if !called {
		t.Error("Legacy error function was not called")
	}
}

// TestHTTPRetryContextMetadataReaderReopen verifies that the RetryContext is correctly
// populated for HTTP range reader reopen retry logic.
func TestHTTPRetryContextMetadataReaderReopen(t *testing.T) {
	t.Parallel()

	transport := &mockTransport{}
	// First request returns 503 Service Unavailable (retriable)
	transport.addResult(&http.Response{
		StatusCode: http.StatusServiceUnavailable,
		Body:       bodyReader("Service Unavailable"),
	}, nil)
	// Second request also returns 503 Service Unavailable
	transport.addResult(&http.Response{
		StatusCode: http.StatusServiceUnavailable,
		Body:       bodyReader("Service Unavailable"),
	}, nil)
	client := mockClient(t, transport)
	defer client.Close()

	var capturedCtx *RetryContext
	var attempts []int
	reader, err := client.Bucket("test-bucket").Object("test-object").Retryer(
		WithErrorFuncWithContext(func(err error, ctx *RetryContext) bool {
			capturedCtx = ctx
			attempts = append(attempts, ctx.Attempt)
			return ctx.Attempt < 2 // Retry once
		}),
	).NewReader(context.Background())

	// We expect NewReader to fail because of the mocked 503s and our policy stopping retries.
	if err == nil {
		if reader != nil {
			reader.Close()
		}
		t.Fatalf("expected NewReader to fail, but got success")
	}
	if capturedCtx == nil {
		t.Fatal("expected RetryContext to be captured in shouldRetry, but it was nil")
	}
	if capturedCtx.Operation != "ReadObject" {
		t.Errorf("Operation: got %q, want %q", capturedCtx.Operation, "ReadObject")
	}
	if capturedCtx.Bucket != "test-bucket" {
		t.Errorf("Bucket: got %q, want %q", capturedCtx.Bucket, "test-bucket")
	}
	if capturedCtx.Object != "test-object" {
		t.Errorf("Object: got %q, want %q", capturedCtx.Object, "test-object")
	}
	wantAttempts := []int{1, 2}
	if !reflect.DeepEqual(attempts, wantAttempts) {
		t.Errorf("Attempt sequence: got %v, want %v", attempts, wantAttempts)
	}
}

// TestGRPCRetryContextMetadataListObjects verifies that the RetryContext is correctly
// populated with metadata for gRPC ListObjects operation retries.
func TestGRPCRetryContextMetadataListObjects(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	// Points to an invalid address to trigger connection failure (Unavailable error)
	client, err := NewGRPCClient(ctx,
		option.WithEndpoint("localhost:1"),
		option.WithoutAuthentication(),
		option.WithGRPCDialOption(grpc.WithTransportCredentials(insecure.NewCredentials())),
	)
	if err != nil {
		t.Fatalf("NewGRPCClient: %v", err)
	}
	defer client.Close()
	var capturedCtx *RetryContext
	var attempts []int
	it := client.Bucket("test-bucket").Retryer(
		WithErrorFuncWithContext(func(err error, ctx *RetryContext) bool {
			capturedCtx = ctx
			attempts = append(attempts, ctx.Attempt)
			return ctx.Attempt < 3 // Retry up to 3 attempts
		}),
	).Objects(ctx, &Query{Prefix: "test-prefix"})

	// it.Next() triggers the fetch, which triggers ListObjects call
	_, err = it.Next()

	if err == nil {
		t.Fatalf("expected call to fail due to invalid address, but got success")
	}
	if capturedCtx == nil {
		t.Fatal("expected RetryContext to be captured, but got nil")
	}
	if capturedCtx.Operation != "ListObjects" {
		t.Errorf("Operation: got %q, want %q", capturedCtx.Operation, "ListObjects")
	}
	if capturedCtx.Bucket != "test-bucket" {
		t.Errorf("Bucket: got %q, want %q", capturedCtx.Bucket, "test-bucket")
	}
	if capturedCtx.Object != "test-prefix" {
		t.Errorf("Object: got %q, want %q", capturedCtx.Object, "test-prefix")
	}
	wantAttempts := []int{1, 2, 3}
	if !reflect.DeepEqual(attempts, wantAttempts) {
		t.Errorf("Attempt sequence: got %v, want %v", attempts, wantAttempts)
	}
}

// TestGRPCRetryContextMetadataNewReader verifies that the RetryContext is correctly
// populated with metadata for gRPC NewRangeReader operation retries.
func TestGRPCRetryContextMetadataNewReader(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	client, err := NewGRPCClient(ctx,
		option.WithEndpoint("localhost:1"),
		option.WithoutAuthentication(),
		option.WithGRPCDialOption(grpc.WithTransportCredentials(insecure.NewCredentials())),
	)
	if err != nil {
		t.Fatalf("NewGRPCClient: %v", err)
	}
	defer client.Close()

	var capturedCtx *RetryContext
	var attempts []int
	reader, err := client.Bucket("test-bucket").Object("test-object").Retryer(
		WithErrorFuncWithContext(func(err error, ctx *RetryContext) bool {
			capturedCtx = ctx
			attempts = append(attempts, ctx.Attempt)
			return ctx.Attempt < 3
		}),
	).NewReader(ctx)

	if err == nil {
		if reader != nil {
			reader.Close()
		}
		t.Fatalf("expected NewReader to fail due to connection error, but got success")
	}
	if capturedCtx == nil {
		t.Fatal("expected RetryContext to be captured, but got nil")
	}
	if capturedCtx.Operation != "ReadObject" {
		t.Errorf("Operation: got %q, want %q", capturedCtx.Operation, "ReadObject")
	}
	if capturedCtx.Bucket != "test-bucket" {
		t.Errorf("Bucket: got %q, want %q", capturedCtx.Bucket, "test-bucket")
	}
	if capturedCtx.Object != "test-object" {
		t.Errorf("Object: got %q, want %q", capturedCtx.Object, "test-object")
	}
	wantAttempts := []int{1, 2, 3}
	if !reflect.DeepEqual(attempts, wantAttempts) {
		t.Errorf("Attempt sequence: got %v, want %v", attempts, wantAttempts)
	}
}

// TestGRPCRetryContextMetadataMultiRangeDownloader verifies that the RetryContext is
// correctly populated for gRPC MultiRangeDownloader session establishment retries.
func TestGRPCRetryContextMetadataMultiRangeDownloader(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	client, err := NewGRPCClient(ctx,
		option.WithEndpoint("localhost:1"),
		option.WithoutAuthentication(),
		option.WithGRPCDialOption(grpc.WithTransportCredentials(insecure.NewCredentials())),
		experimental.WithGRPCBidiReads(),
	)
	if err != nil {
		t.Fatalf("NewGRPCClient: %v", err)
	}
	defer client.Close()

	var capturedCtx *RetryContext
	var attempts []int
	mrd, err := client.Bucket("test-bucket").Object("test-object").Retryer(
		WithErrorFuncWithContext(func(err error, ctx *RetryContext) bool {
			capturedCtx = ctx
			attempts = append(attempts, ctx.Attempt)
			return ctx.Attempt < 3
		}),
	).NewMultiRangeDownloader(ctx)

	if err == nil {
		if mrd != nil {
			mrd.Close()
		}
		t.Fatalf("expected NewMultiRangeDownloader to fail, but got success")
	}
	if capturedCtx == nil {
		t.Fatal("expected RetryContext to be captured, but got nil")
	}
	if capturedCtx.Operation != "ReadObject" {
		t.Errorf("Operation: got %q, want %q", capturedCtx.Operation, "ReadObject")
	}
	if capturedCtx.Bucket != "test-bucket" {
		t.Errorf("Bucket: got %q, want %q", capturedCtx.Bucket, "test-bucket")
	}
	if capturedCtx.Object != "test-object" {
		t.Errorf("Object: got %q, want %q", capturedCtx.Object, "test-object")
	}
	wantAttempts := []int{1, 2, 3}
	if !reflect.DeepEqual(attempts, wantAttempts) {
		t.Errorf("Attempt sequence: got %v, want %v", attempts, wantAttempts)
	}
}

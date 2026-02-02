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
	"errors"
	"testing"
)

func TestRetryContextSignatures(t *testing.T) {
	tests := []struct {
		name        string
		errFunc     interface{}
		shouldBeNil bool
		description string
	}{
		{
			name: "legacy signature func(error) bool",
			errFunc: func(err error) bool {
				return err != nil
			},
			shouldBeNil: false,
			description: "Should accept legacy single-parameter signature",
		},
		{
			name: "new signature func(error, *RetryContext) bool",
			errFunc: func(err error, ctx *RetryContext) bool {
				return err != nil && ctx.Attempt < 3
			},
			shouldBeNil: false,
			description: "Should accept new context-aware signature",
		},
		{
			name: "invalid signature func(string) bool",
			errFunc: func(s string) bool {
				return true
			},
			shouldBeNil: true,
			description: "Should set shouldRetry to nil on invalid signature",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			config := &retryConfig{}
			WithErrorFunc(tt.errFunc).apply(config)

			if tt.shouldBeNil && config.shouldRetry != nil {
				t.Errorf("Expected shouldRetry to be nil for invalid signature")
			}
			if !tt.shouldBeNil && config.shouldRetry == nil {
				t.Errorf("Expected shouldRetry to be set for valid signature")
			}
		})
	}
}

func TestRetryContextFields(t *testing.T) {
	var capturedCtx *RetryContext
	
	errFunc := func(err error, ctx *RetryContext) bool {
		capturedCtx = ctx
		return false // don't actually retry
	}

	config := &retryConfig{}
	WithErrorFunc(errFunc).apply(config)

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

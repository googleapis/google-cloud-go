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

// TestWithErrorFuncBackwardCompatibility verifies that WithErrorFunc accepts
// both the legacy signature func(error) bool and the new signature
// func(error, *RetryContext) bool.
func TestWithErrorFuncBackwardCompatibility(t *testing.T) {
testErr := errors.New("test error")

t.Run("legacy_signature_func(error)_bool", func(t *testing.T) {
called := false
legacyFunc := func(err error) bool {
called = true
return err == testErr
}

// Apply the legacy function
opt := WithErrorFunc(legacyFunc)
rc := &retryConfig{}
opt.apply(rc)

// Verify it was wrapped and works correctly
if rc.shouldRetry == nil {
t.Fatal("shouldRetry was not set")
}

// Call with new signature - should invoke the wrapped legacy function
result := rc.shouldRetry(testErr, &RetryContext{Attempt: 1, InvocationID: "test-id"})
if !called {
t.Error("legacy function was not called")
}
if !result {
t.Error("expected shouldRetry to return true for testErr")
}

// Test with different error
called = false
result = rc.shouldRetry(errors.New("different error"), &RetryContext{Attempt: 2, InvocationID: "test-id-2"})
if !called {
t.Error("legacy function was not called on second invocation")
}
if result {
t.Error("expected shouldRetry to return false for different error")
}
})

t.Run("new_signature_func(error,_*RetryContext)_bool", func(t *testing.T) {
var capturedAttempt int
var capturedID string
var capturedOperation string
var capturedBucket string
var capturedObject string
called := false

newFunc := func(err error, ctx *RetryContext) bool {
called = true
capturedAttempt = ctx.Attempt
capturedID = ctx.InvocationID
capturedOperation = ctx.Operation
capturedBucket = ctx.Bucket
capturedObject = ctx.Object
return err == testErr
}

// Apply the new function
opt := WithErrorFunc(newFunc)
rc := &retryConfig{}
opt.apply(rc)

// Verify it was set correctly
if rc.shouldRetry == nil {
t.Fatal("shouldRetry was not set")
}

// Call with new signature - should invoke directly
result := rc.shouldRetry(testErr, &RetryContext{
Attempt:      3,
InvocationID: "invocation-123",
Operation:    "GetObject",
Bucket:       "test-bucket",
Object:       "test-object",
})
if !called {
t.Error("new function was not called")
}
if !result {
t.Error("expected shouldRetry to return true for testErr")
}
if capturedAttempt != 3 {
t.Errorf("expected attempt=3, got %d", capturedAttempt)
}
if capturedID != "invocation-123" {
t.Errorf("expected id=invocation-123, got %s", capturedID)
}
if capturedOperation != "GetObject" {
t.Errorf("expected operation=GetObject, got %s", capturedOperation)
}
if capturedBucket != "test-bucket" {
t.Errorf("expected bucket=test-bucket, got %s", capturedBucket)
}
if capturedObject != "test-object" {
t.Errorf("expected object=test-object, got %s", capturedObject)
}
})

t.Run("invalid_signature_is_handled", func(t *testing.T) {
// Test that invalid signatures are handled gracefully
invalidFunc := func(s string) bool {
return false
}

opt := WithErrorFunc(invalidFunc)
rc := &retryConfig{}
opt.apply(rc)

// Should be set to nil
if rc.shouldRetry != nil {
t.Error("expected shouldRetry to be nil for invalid signature")
}
})
}

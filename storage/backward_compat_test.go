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
// func(error, int, string) bool.
func TestWithErrorFuncBackwardCompatibility(t *testing.T) {
	testErr := errors.New("test error")

	t.Run("legacy signature func(error) bool", func(t *testing.T) {
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
		result := rc.shouldRetry(testErr, 1, "test-id")
		if !called {
			t.Error("legacy function was not called")
		}
		if !result {
			t.Error("expected shouldRetry to return true for testErr")
		}

		// Test with different error
		called = false
		result = rc.shouldRetry(errors.New("different error"), 2, "test-id-2")
		if !called {
			t.Error("legacy function was not called on second invocation")
		}
		if result {
			t.Error("expected shouldRetry to return false for different error")
		}
	})

	t.Run("new signature func(error, int, string) bool", func(t *testing.T) {
		var capturedAttempt int
		var capturedID string
		called := false

		newFunc := func(err error, attempt int, id string) bool {
			called = true
			capturedAttempt = attempt
			capturedID = id
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
		result := rc.shouldRetry(testErr, 3, "invocation-123")
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
	})

	t.Run("invalid signature is handled", func(t *testing.T) {
		// Pass an invalid signature
		invalidFunc := func(s string) bool { return true }

		opt := WithErrorFunc(invalidFunc)
		rc := &retryConfig{}
		opt.apply(rc)

		// Should result in nil shouldRetry, which triggers default behavior
		if rc.shouldRetry != nil {
			t.Error("expected shouldRetry to be nil for invalid signature")
		}
	})
}

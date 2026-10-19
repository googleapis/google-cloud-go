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
	"testing"
)

// TestVRpc_ContextMetadataRetention verifies that WithVRpcMetadata attaches
// (method, attempt) to a ctx such that VRpcMethod / VRpcAttempt can recover
// them, and that further wrapping with context.WithValue does not lose the
// vRPC metadata. Guards against a regression where a custom context-key
// scheme would only survive a single Get.
func TestVRpc_ContextMetadataRetention(t *testing.T) {
	ctx := WithVRpcMetadata(context.Background(), "QueryMethod", 42)

	if got := VRpcMethod(ctx); got != "QueryMethod" {
		t.Errorf("VRpcMethod = %q, want %q", got, "QueryMethod")
	}
	if got := VRpcAttempt(ctx); got != 42 {
		t.Errorf("VRpcAttempt = %d, want %d", got, 42)
	}

	// Wrapping with context.WithValue (e.g. a standard tracing span setter)
	// must not clobber the vRPC metadata.
	type extraKey struct{}
	wrapped := context.WithValue(ctx, extraKey{}, "extraValue")

	if got := VRpcMethod(wrapped); got != "QueryMethod" {
		t.Errorf("VRpcMethod (wrapped) = %q, want preserved %q", got, "QueryMethod")
	}
	if got := VRpcAttempt(wrapped); got != 42 {
		t.Errorf("VRpcAttempt (wrapped) = %d, want preserved %d", got, 42)
	}
}

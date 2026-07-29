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
	"errors"
	"fmt"
	"testing"
)

func TestAttemptState_String(t *testing.T) {
	tests := []struct {
		s    AttemptState
		want string
	}{
		{StateServerResult, "ServerResult"},
		{StateUncommitted, "Uncommitted"},
		{StateTransportFailure, "TransportFailure"},
		{AttemptState(99), "Unknown"},
	}
	for _, tt := range tests {
		if got := tt.s.String(); got != tt.want {
			t.Errorf("AttemptState(%d).String() = %q, want %q", int(tt.s), got, tt.want)
		}
	}
}

// TestTagErr_NilPassesThrough guarantees callers can compose without extra
// guards: tagErr(state, nil) must return nil regardless of state.
func TestTagErr_NilPassesThrough(t *testing.T) {
	if got := tagErr(StateUncommitted, nil); got != nil {
		t.Errorf("tagErr(StateUncommitted, nil) = %v, want nil", got)
	}
	if got := TagErr(StateTransportFailure, nil); got != nil {
		t.Errorf("TagErr(StateTransportFailure, nil) = %v, want nil", got)
	}
}

// TestTagErr_WrapsAndUnwraps verifies the wrapper is transparent to
// errors.Is and errors.Unwrap so upstream callers that don't know about
// the AttemptOutcome tagging still see the original sentinel.
func TestTagErr_WrapsAndUnwraps(t *testing.T) {
	sentinel := errors.New("sentinel")
	wrapped := tagErr(StateUncommitted, fmt.Errorf("wrap: %w", sentinel))

	if !errors.Is(wrapped, sentinel) {
		t.Error("errors.Is should see the underlying sentinel through the tagErr wrapper")
	}
	if wrapped.Error() == "" {
		t.Error("wrapped error should have non-empty message")
	}
}

// TestClassifyErr_Cases covers the three branches: nil, tagged, and
// untagged. Untagged errors default to StateServerResult (retry only with
// server-attached RetryInfo — Java-parity behavior).
func TestClassifyErr_Cases(t *testing.T) {
	// Nil error → zero outcome.
	if got := ClassifyErr(nil); got.State != StateServerResult || got.Err != nil {
		t.Errorf("ClassifyErr(nil) = %+v, want zero-value outcome", got)
	}

	// Tagged error → preserves state and err.
	inner := errors.New("boom")
	tagged := tagErr(StateTransportFailure, inner)
	out := ClassifyErr(tagged)
	if out.State != StateTransportFailure {
		t.Errorf("ClassifyErr(tagged).State = %v, want StateTransportFailure", out.State)
	}
	if !errors.Is(out.Err, inner) {
		t.Errorf("ClassifyErr(tagged).Err should preserve inner, got %v", out.Err)
	}

	// Untagged error → StateServerResult fallback with err preserved.
	untagged := errors.New("plain")
	out = ClassifyErr(untagged)
	if out.State != StateServerResult {
		t.Errorf("ClassifyErr(untagged).State = %v, want StateServerResult", out.State)
	}
	if out.Err != untagged {
		t.Errorf("ClassifyErr(untagged).Err = %v, want %v", out.Err, untagged)
	}
}

// TestClassifyErr_FindsThroughWrapper ensures errors.As-style traversal
// works even when the tagErr wrapper is buried behind another fmt.Errorf.
func TestClassifyErr_FindsThroughWrapper(t *testing.T) {
	tagged := tagErr(StateUncommitted, errors.New("x"))
	outer := fmt.Errorf("outer: %w", tagged)
	out := ClassifyErr(outer)
	if out.State != StateUncommitted {
		t.Errorf("ClassifyErr(outer-wrapped).State = %v, want StateUncommitted", out.State)
	}
}

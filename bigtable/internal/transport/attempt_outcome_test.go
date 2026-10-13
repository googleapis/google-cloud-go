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
	"fmt"
	"strings"
	"testing"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
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

// The fmt.Errorf-wrapped-ctx case is deliberate: real call sites wrap
// ctx.Err in fmt.Errorf, and a future grpc-go bump swapping
// FromContextError's errors.Is walk for identity comparison would
// silently regress that branch to Unknown.
func TestVRPCErr_GRPCStatus(t *testing.T) {
	tests := []struct {
		name     string
		inner    error
		wantCode codes.Code
		wantMsg  string // required substring in status message
	}{
		{
			name:     "wrapped err already carries gRPC status wins verbatim",
			inner:    status.Error(codes.FailedPrecondition, "table not ready"),
			wantCode: codes.FailedPrecondition,
			wantMsg:  "table not ready",
		},
		{
			name:     "context.DeadlineExceeded translates to DeadlineExceeded",
			inner:    context.DeadlineExceeded,
			wantCode: codes.DeadlineExceeded,
		},
		{
			name:     "context.Canceled translates to Canceled",
			inner:    context.Canceled,
			wantCode: codes.Canceled,
		},
		{
			name:     "fmt.Errorf-wrapped ctx err still translates via errors.Is walk",
			inner:    fmt.Errorf("send vRPC request: %w", context.DeadlineExceeded),
			wantCode: codes.DeadlineExceeded,
		},
		{
			name:     "plain non-status non-ctx err surfaces as Unknown",
			inner:    errors.New("something else broke"),
			wantCode: codes.Unknown,
			wantMsg:  "something else broke",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			e := &vrpcErr{outcome: AttemptOutcome{State: StateTransportFailure, Err: tt.inner}}
			s := e.GRPCStatus()
			if s.Code() != tt.wantCode {
				t.Errorf("GRPCStatus().Code = %v, want %v", s.Code(), tt.wantCode)
			}
			if tt.wantMsg != "" && !strings.Contains(s.Message(), tt.wantMsg) {
				t.Errorf("GRPCStatus().Message = %q, want to contain %q", s.Message(), tt.wantMsg)
			}
		})
	}
}

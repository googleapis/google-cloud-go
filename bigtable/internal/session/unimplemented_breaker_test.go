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

package session

import (
	"errors"
	"testing"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// TestShouldFallback pins the per-call verdict and the counter
// bookkeeping that goes with it: only Unimplemented diverts to classic
// and advances the consecutive count; everything else, success or
// failure, stays on the session path's own result and resets the count.
func TestShouldFallback(t *testing.T) {
	for _, tc := range []struct {
		name         string
		err          error
		wantFallback bool
		wantCount    int32
	}{
		{name: "success", err: nil},
		{name: "unrelated error", err: errors.New("boom")},
		{name: "non-Unimplemented status", err: status.Error(codes.NotFound, "no such table")},
		{name: "unimplemented", err: status.Error(codes.Unimplemented, "no session backend"), wantFallback: true, wantCount: 1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			b := NewUnimplementedBreaker(DefaultUnimplementedThreshold)
			if got := b.ShouldFallback(tc.err); got != tc.wantFallback {
				t.Errorf("ShouldFallback(%v) = %v, want %v", tc.err, got, tc.wantFallback)
			}
			if got := b.Count(); got != tc.wantCount {
				t.Errorf("Count() = %d, want %d", got, tc.wantCount)
			}
			if b.Bypass() {
				t.Errorf("Bypass() = true after one call, want false")
			}
		})
	}
}

// TestShouldFallback_ResetsOnNonUnimplemented pins that the count is
// consecutive: a single non-Unimplemented reply proves the backend
// understands the RPC, so accumulated Unimplementeds are forgotten.
func TestShouldFallback_ResetsOnNonUnimplemented(t *testing.T) {
	b := NewUnimplementedBreaker(DefaultUnimplementedThreshold)
	unimpl := status.Error(codes.Unimplemented, "nope")

	b.ShouldFallback(unimpl)
	b.ShouldFallback(unimpl)
	if got := b.Count(); got != 2 {
		t.Fatalf("Count() = %d after 2 Unimplementeds, want 2", got)
	}
	b.ShouldFallback(nil)
	if got := b.Count(); got != 0 {
		t.Errorf("Count() = %d after a success, want 0", got)
	}
}

// TestShouldFallback_TripsStickyBreaker pins the breaker: it flips only
// on reaching the threshold, and once tripped it stays tripped so
// routing stops dialing session even after the count resets.
func TestShouldFallback_TripsStickyBreaker(t *testing.T) {
	const threshold int32 = 3
	b := NewUnimplementedBreaker(threshold)
	unimpl := status.Error(codes.Unimplemented, "nope")

	for n := int32(0); n < threshold; n++ {
		if b.Bypass() {
			t.Fatalf("Bypass() = true after %d Unimplementeds, want false before the threshold", n)
		}
		if !b.ShouldFallback(unimpl) {
			t.Fatalf("ShouldFallback(Unimplemented) = false on call %d, want true", n)
		}
	}
	if !b.Bypass() {
		t.Fatalf("Bypass() = false after %d Unimplementeds, want true", threshold)
	}

	// A later success resets the count but must not un-trip the breaker.
	b.ShouldFallback(nil)
	if got := b.Count(); got != 0 {
		t.Errorf("Count() = %d after a success, want 0", got)
	}
	if !b.Bypass() {
		t.Errorf("Bypass() = false after a success, want true (breaker is sticky)")
	}
}

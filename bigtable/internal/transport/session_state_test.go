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
	"sync"
	"testing"
	"time"
)

func TestState_String(t *testing.T) {
	cases := []struct {
		state State
		want  string
	}{
		{StateNew, "New"},
		{StateStarting, "Starting"},
		{StateReady, "Ready"},
		{StateClosing, "Closing"},
		{StateWaitServerClose, "WaitServerClose"},
		{StateClosed, "Closed"},
		{State(99), "Unknown"},
	}
	for _, tc := range cases {
		if got := tc.state.String(); got != tc.want {
			t.Errorf("State(%d).String() = %q, want %q", int(tc.state), got, tc.want)
		}
	}
}

// TestState_OrdinalsPinned guards against accidental renumbering of the State
// constants. Several downstream paths (logs, metrics labels) rely on the
// numeric ordinals being stable across releases; bumping one shifts every
// state above it. Ordinals also match the Java SessionState.phase values so
// telemetry compares across language clients.
func TestState_OrdinalsPinned(t *testing.T) {
	cases := []struct {
		state State
		want  int
	}{
		{StateNew, 0},
		{StateStarting, 1},
		{StateReady, 2},
		{StateClosing, 3},
		{StateWaitServerClose, 4},
		{StateClosed, 5},
	}
	for _, tc := range cases {
		if got := int(tc.state); got != tc.want {
			t.Errorf("int(%s) = %d, want %d", tc.state, got, tc.want)
		}
	}
}

// TestSession_TransitionTo_Happy walks New→Starting→Ready with predicates that
// mirror the real transition sites; ensures each hop applies and stamps
// lastStateChangeNano.
func TestSession_TransitionTo_Happy(t *testing.T) {
	s := newTestSession(t)
	before := s.lastStateChangeNano.Load()
	// Ensure at least a ns of clock movement so the stamp is observably later.
	time.Sleep(time.Millisecond)

	prev, ok := s.transitionTo(StateStarting, isState(StateNew))
	if !ok || prev != StateNew {
		t.Fatalf("New→Starting: applied=%v prev=%s, want true/New", ok, prev)
	}
	if s.State() != StateStarting {
		t.Fatalf("post-transition State: got %s, want Starting", s.State())
	}
	if s.lastStateChangeNano.Load() <= before {
		t.Errorf("lastStateChangeNano did not advance on transition")
	}

	prev, ok = s.transitionTo(StateReady, isState(StateStarting))
	if !ok || prev != StateStarting {
		t.Fatalf("Starting→Ready: applied=%v prev=%s, want true/Starting", ok, prev)
	}
}

// TestSession_TransitionTo_PredicateRejects confirms a false predicate leaves
// state alone and returns (currentState, false).
func TestSession_TransitionTo_PredicateRejects(t *testing.T) {
	s := newTestSession(t)
	// New → Ready is illegal unless the predicate permits it; use one that
	// explicitly excludes New.
	prev, ok := s.transitionTo(StateReady, isState(StateStarting))
	if ok {
		t.Errorf("transition applied against predicate — should reject")
	}
	if prev != StateNew {
		t.Errorf("prev on rejection: got %s, want StateNew", prev)
	}
	if s.State() != StateNew {
		t.Errorf("state after rejected transition: got %s, want StateNew", s.State())
	}
}

// TestSession_TransitionTo_Concurrent stress-tests the CAS-plus-predicate loop
// under -race. Two goroutines both try New→Starting; exactly one must succeed.
func TestSession_TransitionTo_Concurrent(t *testing.T) {
	s := newTestSession(t)
	var wg sync.WaitGroup
	var applied [2]bool
	wg.Add(2)
	start := make(chan struct{})
	for i := 0; i < 2; i++ {
		i := i
		go func() {
			defer wg.Done()
			<-start
			_, ok := s.transitionTo(StateStarting, isState(StateNew))
			applied[i] = ok
		}()
	}
	close(start)
	wg.Wait()

	if applied[0] == applied[1] {
		t.Errorf("racing New→Starting: both goroutines saw applied=%v, want exactly one true", applied[0])
	}
	if s.State() != StateStarting {
		t.Errorf("state after racing transitions: got %s, want Starting", s.State())
	}
}

// TestIsState_And_NotState cover the two predicate builders. They're pure
// functions and callers pass them into transitionTo, so trivial coverage is
// enough — the point is the contract, not exotic inputs.
func TestIsState_And_NotState(t *testing.T) {
	pred := isState(StateReady, StateStarting)
	if !pred(StateReady) || !pred(StateStarting) {
		t.Errorf("isState: allowed states rejected")
	}
	if pred(StateNew) || pred(StateClosed) {
		t.Errorf("isState: non-allowed states admitted")
	}

	np := notState(StateClosed, StateClosing)
	if np(StateClosed) || np(StateClosing) {
		t.Errorf("notState: forbidden states admitted")
	}
	if !np(StateNew) || !np(StateReady) {
		t.Errorf("notState: allowed states rejected")
	}
}

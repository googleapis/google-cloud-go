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

import "testing"

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

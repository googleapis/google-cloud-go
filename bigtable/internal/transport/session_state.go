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

// State represents the lifecycle state of a Session. Sessions move strictly
// forward through the values; once StateClosed is reached the session is
// terminal.
type State int

const (
	// StateNew indicates the session is newly created and not yet active.
	StateNew State = iota
	// StateStarting indicates the session is dialing and handshaking.
	StateStarting
	// StateActive indicates the session is active and ready for RPCs.
	StateActive
	// StateClosing indicates the session is draining and shutting down. It
	// covers both the pre-CloseSession drain and the post-CloseSession wait
	// for the server's EOF.
	StateClosing
	// StateClosed indicates the session is closed.
	StateClosed
)

// String returns a human-readable name for the state.
func (s State) String() string {
	switch s {
	case StateNew:
		return "New"
	case StateStarting:
		return "Starting"
	case StateActive:
		return "Active"
	case StateClosing:
		return "Closing"
	case StateClosed:
		return "Closed"
	default:
		return "Unknown"
	}
}

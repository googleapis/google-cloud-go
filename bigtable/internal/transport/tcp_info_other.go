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

//go:build !linux

package internal

import "net"

// readTCPInfo returns ErrTCPInfoUnsupported on non-Linux — tcp_info is a
// Linux-specific socket option. Snapshot() will surface this via the Err
// field so the tcpz page renders a "unsupported" row rather than
// panicking or looking empty.
func readTCPInfo(_ *net.TCPConn) (TCPInfoSnapshot, error) {
	return TCPInfoSnapshot{}, ErrTCPInfoUnsupported
}

// isDeadConn is only meaningful on Linux (where errno differentiation
// exists); on other platforms every read error is "unsupported," which
// isn't dead-fd, so return false.
func isDeadConn(_ error) bool { return false }

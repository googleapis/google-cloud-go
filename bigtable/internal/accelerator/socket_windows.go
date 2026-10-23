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

//go:build windows

package accelerator

import "net"

// parentPidWatchdogSupported reports whether monitorParentPid can detect parent
// death on this platform. It cannot on Windows: there is no reparenting, so a
// process whose parent exits keeps reporting the dead parent's PID forever and
// the watchdog can never trip. Worse, Windows recycles PIDs, so that stale value
// may later belong to an unrelated process. Running the poll loop anyway would
// also be expensive -- os.Getppid on Windows enumerates every process on the
// machine via a Toolhelp snapshot, which is not something to do twice a second
// for a signal that never arrives.
//
// Parent death is still covered: monitorStdin sees EOF on the inherited stdin
// pipe as soon as the parent's handle is closed, which is the same signal the
// POSIX build relies on when ppid is ambiguous.
const parentPidWatchdogSupported = false

// listenUDS binds the daemon's AF_UNIX socket at path. Windows 10 1803+ and
// Server 2019+ support AF_UNIX and Go's net package speaks it, so the socket
// path and wire behavior match the POSIX build.
//
// Access control is what differs. Windows has no umask, and os.Chmod there only
// toggles the read-only attribute -- it cannot express "owner may connect,
// nobody else," so the POSIX 0077/0600 lockdown has no equivalent to port. The
// socket file instead inherits the ACL of the directory holding it, which makes
// socket security a property of that directory. Callers must therefore place the
// socket in a directory only the spawning user can traverse; the per-daemon
// directory the SDK creates under the user-scoped %TEMP% satisfies this, since
// that tree is ACL'd to the user by default. Applying a mode-based lockdown here
// would only look like a defense without being one.
func listenUDS(path string) (net.Listener, error) {
	return net.Listen("unix", path)
}

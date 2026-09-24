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

//go:build !windows

package accelerator

import (
	"net"
	"os"
	"syscall"
)

// parentPidWatchdogSupported reports whether monitorParentPid can detect parent
// death on this platform. POSIX reparents orphans to init (or the nearest
// subreaper), so a changed ppid is a reliable death signal.
const parentPidWatchdogSupported = true

// listenUDS binds the daemon's Unix domain socket at path and restricts it to
// the owning user.
//
// Bind under a private umask so the socket is never world-connectable, even for
// the instant between creation and the Chmod below. net.Listen -> bind(2)
// creates the socket file subject to the process umask; the common default
// (022) would leave it group/other accessible during that window, letting any
// local process connect before we lock it down. umask is process-global, so we
// narrow it only around the bind and restore it immediately after.
func listenUDS(path string) (net.Listener, error) {
	oldMask := syscall.Umask(0077)
	l, err := net.Listen("unix", path)
	syscall.Umask(oldMask)
	if err != nil {
		return nil, err
	}

	// Tighten to 0600, dropping the owner-execute bit the 0077 umask leaves at
	// 0700. The daemon is paired 1:1 with its parent process, so no other user
	// should be able to issue RPCs against it. Because the umask above already
	// guaranteed the socket was never group/other accessible, this Chmod is
	// race-free hardening rather than the sole line of defense.
	if err := os.Chmod(path, 0600); err != nil {
		_ = l.Close()
		_ = os.Remove(path)
		return nil, err
	}
	return l, nil
}

//go:build linux

package udsthin

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"syscall"

	"golang.org/x/sys/unix"
)

// Embedded-binary strategy ("go get is enough"): the module carries the
// sidecar executable for this GOOS/GOARCH (see embed_*.go). At runtime it is
// extracted once to a per-user cache dir keyed by content hash, or, when that
// directory is not executable (noexec / read-only rootfs), executed straight
// from an anonymous memfd. Both paths are measured in packaging_tests.sh.

// HowStarted records which strategy produced the binary path (for the report).
var HowStarted string

func embeddedServer() (string, error) {
	if len(serverBin) == 0 {
		return "", errors.New("no embedded gcs-core-server for this GOOS/GOARCH")
	}
	sum := sha256.Sum256(serverBin)
	key := hex.EncodeToString(sum[:8])
	dir := os.Getenv("XDG_CACHE_HOME")
	if dir == "" {
		home, _ := os.UserHomeDir()
		if home == "" {
			home = os.TempDir()
		}
		dir = filepath.Join(home, ".cache")
	}
	dir = filepath.Join(dir, "gcs-core", key)
	path := filepath.Join(dir, "gcs-core-server")
	if st, err := os.Stat(path); err == nil && st.Size() == int64(len(serverBin)) {
		if canExec(path) {
			HowStarted = "embedded:cache-hit:" + path
			return path, nil
		}
	}
	if err := os.MkdirAll(dir, 0o700); err == nil {
		tmp := path + ".tmp"
		if err := os.WriteFile(tmp, serverBin, 0o700); err == nil {
			if err := os.Rename(tmp, path); err == nil && canExec(path) {
				HowStarted = "embedded:extracted:" + path
				return path, nil
			}
		}
		os.Remove(tmp)
	}
	// Fallback: anonymous in-memory file (no filesystem exec needed).
	p, err := memfdPath()
	if err != nil {
		return "", fmt.Errorf("cache dir not executable and memfd fallback failed: %w", err)
	}
	HowStarted = "embedded:memfd:" + p
	return p, nil
}

// canExec checks the directory is not mounted noexec by probing access(X_OK)
// plus a real exec of `--version`.
func canExec(path string) bool {
	if err := syscall.Access(path, 0x1); err != nil {
		return false
	}
	attr := &syscall.ProcAttr{Files: []uintptr{0, 1, 2}}
	pid, err := syscall.ForkExec(path, []string{path, "--version"}, attr)
	if err != nil {
		return false // EACCES on noexec mounts
	}
	var ws syscall.WaitStatus
	syscall.Wait4(pid, &ws, 0, nil)
	return ws.ExitStatus() == 0
}

// memfdPath writes the embedded binary into a memfd and returns the
// /proc/self/fd/N path to exec. Blocked where vm.memfd_noexec=2 or seccomp
// MDWE is enforced; not available on macOS/Windows.
func memfdPath() (string, error) {
	fd, err := memfdCreate("gcs-core-server", 0)
	if err != nil {
		return "", err
	}
	f := os.NewFile(uintptr(fd), "memfd:gcs-core-server")
	if _, err := f.Write(serverBin); err != nil {
		f.Close()
		return "", err
	}
	// Keep f open for the process lifetime; children inherit via /proc path.
	keep = append(keep, f)
	return fmt.Sprintf("/proc/self/fd/%d", fd), nil
}

var keep []*os.File

func memfdCreate(name string, flags uint) (int, error) {
	return unix.MemfdCreate(name, int(flags))
}

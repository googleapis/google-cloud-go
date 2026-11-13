//go:build windows

package udsthin

import (
	"os"
	"syscall"
)

// Windows has no process groups in the POSIX sense and no SIGTERM; the POC
// client compiles there but the sidecar itself is only built for Linux.
func sysProcAttr() *syscall.SysProcAttr { return nil }

func terminate(p *os.Process) error { return p.Kill() }

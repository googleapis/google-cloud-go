//go:build !windows

package udsthin

import (
	"os"
	"syscall"
)

func sysProcAttr() *syscall.SysProcAttr { return &syscall.SysProcAttr{Setpgid: true} }

func terminate(p *os.Process) error { return p.Signal(syscall.SIGTERM) }

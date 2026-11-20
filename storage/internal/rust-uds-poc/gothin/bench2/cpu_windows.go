//go:build windows

package bench2

import "syscall"

// CPUSeconds returns user+kernel CPU of the whole process via GetProcessTimes.
func CPUSeconds() float64 {
	var creation, exit, kernel, user syscall.Filetime
	h, err := syscall.GetCurrentProcess()
	if err != nil {
		return 0
	}
	if err := syscall.GetProcessTimes(h, &creation, &exit, &kernel, &user); err != nil {
		return 0
	}
	return float64(kernel.Nanoseconds()+user.Nanoseconds()) / 1e9
}

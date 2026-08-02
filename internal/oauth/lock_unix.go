//go:build !windows

package oauth

import (
	"os"
	"syscall"
)

// isProcessAlive reports whether a process with the given PID is running.
// On Unix, FindProcess always succeeds; Signal(0) tests existence.
func isProcessAlive(pid int) bool {
	proc, err := os.FindProcess(pid)
	if err != nil {
		return false
	}
	err = proc.Signal(syscall.Signal(0))
	return err == nil
}

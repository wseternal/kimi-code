//go:build unix

package background

import (
	"os"
	"syscall"
)

// isProcessAlive checks if a process with the given PID exists.
// On Unix, sending signal 0 to a PID checks existence without affecting the process.
func isProcessAlive(pid int) bool {
	proc, err := os.FindProcess(pid)
	if err != nil {
		return false
	}
	err = proc.Signal(syscall.Signal(0))
	return err == nil
}

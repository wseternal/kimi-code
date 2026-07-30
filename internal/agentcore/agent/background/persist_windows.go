//go:build windows

package background

import "syscall"

// isProcessAlive checks if a process with the given PID exists on Windows.
// Uses OpenProcess with PROCESS_QUERY_LIMITED_INFORMATION to check liveness.
func isProcessAlive(pid int) bool {
	handle, err := syscall.OpenProcess(syscall.PROCESS_QUERY_LIMITED_INFORMATION, false, uint32(pid))
	if err != nil {
		return false
	}
	defer syscall.CloseHandle(handle)

	var exitCode uint32
	if err := syscall.GetExitCodeProcess(handle, &exitCode); err != nil {
		return false
	}
	return exitCode == 259 // STILL_ACTIVE
}

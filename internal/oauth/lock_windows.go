//go:build windows

package oauth

import (
	"os"

	"golang.org/x/sys/windows"
)

// flockExclusive acquires an exclusive (non-blocking) lock on the file using
// Windows LockFileEx. This prevents concurrent processes from racing on token
// refresh and storage.
func flockExclusive(f *os.File) error {
	handle := windows.Handle(f.Fd())
	var overlapped windows.Overlapped
	// LOCKFILE_EXCLUSIVE_LOCK | LOCKFILE_FAIL_IMMEDIATELY
	const flags = windows.LOCKFILE_EXCLUSIVE_LOCK | windows.LOCKFILE_FAIL_IMMEDIATELY
	// Lock the maximum possible range to cover the entire file.
	err := windows.LockFileEx(handle, flags, 0, 0xFFFFFFFF, 0xFFFFFFFF, &overlapped)
	if err != nil {
		return err
	}
	return nil
}

// flockUnlock releases an exclusive lock on the file.
func flockUnlock(f *os.File) {
	handle := windows.Handle(f.Fd())
	var overlapped windows.Overlapped
	_ = windows.UnlockFileEx(handle, 0, 0xFFFFFFFF, 0xFFFFFFFF, &overlapped)
}

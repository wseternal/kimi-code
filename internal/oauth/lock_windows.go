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
	// Lock the entire file (high+low = 0 means lock from current offset to EOF).
	err := windows.LockFileEx(handle, flags, 0, 1, 0, &overlapped)
	if err != nil {
		return err
	}
	return nil
}

// flockUnlock releases an exclusive lock on the file.
func flockUnlock(f *os.File) {
	handle := windows.Handle(f.Fd())
	var overlapped windows.Overlapped
	windows.UnlockFileEx(handle, 0, 1, 0, &overlapped)
}

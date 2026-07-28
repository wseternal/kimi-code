//go:build windows

package oauth

import (
	"os"
)

// flockExclusive on Windows is best-effort.
// TODO: implement using LockFileEx via golang.org/x/sys/windows for proper
// cross-process locking. The current no-op means concurrent processes on
// Windows may race on token refresh and storage.
func flockExclusive(f *os.File) error {
	return nil // Always succeeds on Windows
}

// flockUnlock on Windows is a no-op.
func flockUnlock(f *os.File) {
	// no-op
}

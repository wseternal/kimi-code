//go:build !windows

package oauth

import (
	"os"
	"syscall"
)

// flockExclusive attempts to acquire an exclusive lock on a file.
func flockExclusive(f *os.File) error {
	return syscall.Flock(int(f.Fd()), syscall.LOCK_EX|syscall.LOCK_NB)
}

// flockUnlock releases an exclusive lock on a file.
func flockUnlock(f *os.File) {
	syscall.Flock(int(f.Fd()), syscall.LOCK_UN)
}

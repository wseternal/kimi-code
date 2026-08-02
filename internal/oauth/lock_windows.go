//go:build windows

package oauth

// isProcessAlive reports whether a process with the given PID is running.
// On Windows, we conservatively return true to avoid removing locks that
// may still be held, since signal-based PID probing is not reliable.
func isProcessAlive(_ int) bool {
	return true
}

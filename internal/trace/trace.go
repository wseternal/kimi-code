// Package trace provides a low-overhead event tracer for diagnosing
// TUI event loop contention, network streaming, and input handling.
//
// When disabled (the default), all trace calls are no-ops that compile
// to branch-free atomic loads, so there is zero overhead in production.
//
// When enabled via Enable(), events are written as JSONL to the configured
// file with nanosecond timestamps.
package trace

import (
	"encoding/json"
	"fmt"
	"os"
	"sync"
	"sync/atomic"
	"time"
)

// enabled is an atomic flag so hot-path checks are lock-free.
var enabled atomic.Bool

// tracer holds the active file writer.
var (
	tracerMu sync.Mutex
	tracerF  *os.File
	startT   time.Time
)

// Event is a single trace record.
type Event struct {
	// T is nanoseconds since trace start (compact, monotonic).
	T int64 `json:"t"`
	// Category groups events: "tui", "stream", "http", "input", "render".
	Category string `json:"cat"`
	// Action is the event name within the category.
	Action string `json:"act"`
	// Data is optional freeform payload (omitted if nil).
	Data any `json:"data,omitempty"`
}

// Enable activates tracing to the given file path.
// Returns an error if the file cannot be created.
func Enable(path string) error {
	f, err := os.Create(path)
	if err != nil {
		return fmt.Errorf("trace: open %s: %w", path, err)
	}
	tracerMu.Lock()
	tracerF = f
	startT = time.Now()
	tracerMu.Unlock()
	enabled.Store(true)
	Log("trace", "start", map[string]any{"path": path, "pid": os.Getpid()})
	return nil
}

// Disable flushes and closes the trace file.
func Disable() {
	if !enabled.Load() {
		return
	}
	Log("trace", "stop", nil)
	enabled.Store(false)
	tracerMu.Lock()
	f := tracerF
	tracerF = nil
	tracerMu.Unlock()
	if f != nil {
		f.Sync()
		f.Close()
	}
}

// Enabled reports whether tracing is active.
func Enabled() bool {
	return enabled.Load()
}

// Log writes a trace event. This is a no-op when tracing is disabled.
// The category/action pair should be stable strings for easy grep.
func Log(category, action string, data any) {
	if !enabled.Load() {
		return
	}
	ev := Event{
		T:        time.Since(startT).Nanoseconds(),
		Category: category,
		Action:   action,
		Data:     data,
	}
	b, err := json.Marshal(ev)
	if err != nil {
		return
	}
	b = append(b, '\n')
	tracerMu.Lock()
	if tracerF != nil {
		tracerF.Write(b)
	}
	tracerMu.Unlock()
}

// Logf is a convenience wrapper for formatted data.
func Logf(category, action, format string, args ...any) {
	if !enabled.Load() {
		return
	}
	Log(category, action, fmt.Sprintf(format, args...))
}

// Since returns a duration since trace start (for relative timing).
func Since() time.Duration {
	return time.Since(startT)
}

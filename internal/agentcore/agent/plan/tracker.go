// Package plan implements lightweight task tracking for agent plan progress.
// The tracker is thread-safe: a tool goroutine writes tasks while the TUI
// goroutine reads them during rendering.
package plan

import (
	"fmt"
	"strings"
	"sync"
	"time"
)

// TaskStatus represents the completion state of a plan task.
type TaskStatus string

const (
	StatusPending TaskStatus = "pending"
	StatusActive  TaskStatus = "active"
	StatusDone    TaskStatus = "done"
	StatusFailed  TaskStatus = "failed"
)

// Task is a single tracked item in the plan.
type Task struct {
	Title  string     `json:"title"`
	Status TaskStatus `json:"status"`
	At     time.Time  `json:"at"`
}

// Tracker manages an ordered list of plan tasks.
type Tracker struct {
	mu    sync.RWMutex
	tasks []Task
}

// NewTracker creates an empty tracker.
func NewTracker() *Tracker {
	return &Tracker{}
}

// SetTasks replaces the entire task list (used by the update_plan tool).
func (t *Tracker) SetTasks(tasks []Task) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.tasks = make([]Task, len(tasks))
	copy(t.tasks, tasks)
}

// UpdateTask sets the status of a task by title. If the title doesn't exist,
// a new task is appended.
func (t *Tracker) UpdateTask(title string, status TaskStatus) {
	t.mu.Lock()
	defer t.mu.Unlock()
	for i, task := range t.tasks {
		if task.Title == title {
			t.tasks[i].Status = status
			t.tasks[i].At = time.Now()
			return
		}
	}
	t.tasks = append(t.tasks, Task{Title: title, Status: status, At: time.Now()})
}

// Clear removes all tasks.
func (t *Tracker) Clear() {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.tasks = nil
}

// Tasks returns a snapshot of the current task list.
func (t *Tracker) Tasks() []Task {
	t.mu.RLock()
	defer t.mu.RUnlock()
	result := make([]Task, len(t.tasks))
	copy(result, t.tasks)
	return result
}

// Len returns the number of tasks.
func (t *Tracker) Len() int {
	t.mu.RLock()
	defer t.mu.RUnlock()
	return len(t.tasks)
}

// Counts returns (pending, active, done, failed) counts.
func (t *Tracker) Counts() (pending, active, done, failed int) {
	t.mu.RLock()
	defer t.mu.RUnlock()
	for _, task := range t.tasks {
		switch task.Status {
		case StatusPending:
			pending++
		case StatusActive:
			active++
		case StatusDone:
			done++
		case StatusFailed:
			failed++
		}
	}
	return
}

// Summary returns a human-readable one-line summary.
func (t *Tracker) Summary() string {
	pending, active, done, failed := t.Counts()
	total := pending + active + done + failed
	if total == 0 {
		return "No tasks"
	}
	summary := fmt.Sprintf("%d/%d done", done, total)
	if failed > 0 {
		summary += fmt.Sprintf(" (%d failed)", failed)
	}
	if active > 0 {
		summary += fmt.Sprintf(", %d active", active)
	}
	if pending > 0 {
		summary += fmt.Sprintf(", %d pending", pending)
	}
	return summary
}

// UpdateTaskByKeyword attempts to auto-sync task status based on tool results.
// It matches the keyword (typically a tool name) against active task titles using
// case-insensitive substring matching.
//
// IMPORTANT: This is best-effort auto-sync, not exact correlation. Short/generic
// tool names like "read", "write", "bash" may match unrelated tasks. To reduce
// false positives:
//   - Keywords shorter than 4 characters are skipped
//   - Only active tasks are transitioned (pending/done/failed are ignored)
//   - Only the first matching task is updated
//
// For reliable task updates, the LLM should explicitly call update_plan with
// specific task IDs or titles.
func (t *Tracker) UpdateTaskByKeyword(keyword string, status TaskStatus) {
	// Skip empty or very short keywords to avoid false-positive matches
	// (e.g., "read" matching "Read and analyze config files")
	if len(keyword) < 4 {
		return
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	for i, task := range t.tasks {
		// Only transition active tasks to avoid overwriting LLM-set states
		if task.Status != StatusActive {
			continue
		}
		// Case-insensitive substring match
		if containsIgnoreCase(task.Title, keyword) {
			t.tasks[i].Status = status
			t.tasks[i].At = time.Now()
			// Only update the first matching task to avoid cascading updates
			return
		}
	}
}

// containsIgnoreCase checks if s contains substr (case-insensitive).
func containsIgnoreCase(s, substr string) bool {
	return strings.Contains(strings.ToLower(s), strings.ToLower(substr))
}

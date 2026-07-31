// Package plan implements lightweight task tracking for agent plan progress.
// The tracker is thread-safe: a tool goroutine writes tasks while the TUI
// goroutine reads them during rendering.
package plan

import (
	"fmt"
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

// UpdateTaskByKeyword updates tasks whose titles contain the given keyword (case-insensitive).
// This enables auto-sync when tools complete: if a tool succeeds and its name/args correlate
// with a plan task, mark it done; if it fails, mark it failed.
func (t *Tracker) UpdateTaskByKeyword(keyword string, status TaskStatus) {
	if keyword == "" {
		return
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	keywordLower := fmt.Sprintf("%s", keyword)
	for i, task := range t.tasks {
		titleLower := fmt.Sprintf("%s", task.Title)
		// Case-insensitive substring match
		if containsIgnoreCase(titleLower, keywordLower) {
			t.tasks[i].Status = status
			t.tasks[i].At = time.Now()
			// Only update the first matching task to avoid cascading updates
			return
		}
	}
}

// containsIgnoreCase checks if s contains substr (case-insensitive).
func containsIgnoreCase(s, substr string) bool {
	if substr == "" {
		return true
	}
	sLen := len(s)
	subLen := len(substr)
	if subLen > sLen {
		return false
	}
	for i := 0; i <= sLen-subLen; i++ {
		match := true
		for j := 0; j < subLen; j++ {
			sc := s[i+j]
			subc := substr[j]
			// Simple ASCII case-insensitive comparison
			if sc >= 'A' && sc <= 'Z' {
				sc += 32
			}
			if subc >= 'A' && subc <= 'Z' {
				subc += 32
			}
			if sc != subc {
				match = false
				break
			}
		}
		if match {
			return true
		}
	}
	return false
}

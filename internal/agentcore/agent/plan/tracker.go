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

// Counts returns (pending, active, done) counts.
func (t *Tracker) Counts() (pending, active, done int) {
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
		}
	}
	return
}

// Summary returns a human-readable one-line summary.
func (t *Tracker) Summary() string {
	pending, active, done := t.Counts()
	total := pending + active + done
	if total == 0 {
		return "No tasks"
	}
	return fmt.Sprintf("%d/%d done (%d active, %d pending)", done, total, active, pending)
}

package background

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

// PersistState is the on-disk representation of background task state.
type PersistState struct {
	Version int           `json:"version"`
	Tasks   []TaskInfo    `json:"tasks"`
	SavedAt time.Time     `json:"saved_at"`
}

// Persistence handles saving/loading background task state to disk.
type Persistence struct {
	dir string
}

// NewPersistence creates a persistence handler for a given directory.
func NewPersistence(dir string) *Persistence {
	return &Persistence{dir: dir}
}

// stateFile returns the path to the state file.
func (p *Persistence) stateFile() string {
	return filepath.Join(p.dir, ".background-tasks.json")
}

// Save persists the current task state to disk.
func (p *Persistence) Save(tasks []TaskInfo) error {
	if p.dir == "" {
		return nil
	}

	state := PersistState{
		Version: 1,
		Tasks:   tasks,
		SavedAt: time.Now(),
	}

	data, err := json.MarshalIndent(state, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal state: %w", err)
	}

	if err := os.MkdirAll(p.dir, 0755); err != nil {
		return fmt.Errorf("create dir: %w", err)
	}

	return os.WriteFile(p.stateFile(), data, 0644)
}

// Load reads the persisted task state from disk.
func (p *Persistence) Load() (*PersistState, error) {
	if p.dir == "" {
		return nil, nil
	}

	data, err := os.ReadFile(p.stateFile())
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("read state: %w", err)
	}

	var state PersistState
	if err := json.Unmarshal(data, &state); err != nil {
		return nil, fmt.Errorf("parse state: %w", err)
	}

	return &state, nil
}

// Clear removes the persisted state file.
func (p *Persistence) Clear() error {
	if p.dir == "" {
		return nil
	}
	err := os.Remove(p.stateFile())
	if os.IsNotExist(err) {
		return nil
	}
	return err
}

// ReconcileResult describes the outcome of reconciling persisted tasks.
type ReconcileResult struct {
	// StillRunning are tasks that were persisted as running and still have a live process.
	StillRunning []TaskInfo
	// Lost are tasks that were persisted as running but have no live process.
	Lost []TaskInfo
	// Completed are tasks that finished while the daemon was down.
	Completed []TaskInfo
}

// Reconcile checks persisted tasks against live processes to detect lost tasks.
func Reconcile(persisted []TaskInfo) *ReconcileResult {
	result := &ReconcileResult{}

	for _, task := range persisted {
		if task.Status.IsTerminal() {
			result.Completed = append(result.Completed, task)
			continue
		}

		// Check if the process is still running
		if task.PID > 0 && isProcessAlive(task.PID) {
			result.StillRunning = append(result.StillRunning, task)
		} else {
			// Process gone — mark as lost
			task.Status = StatusFailed
			task.EndedAt = time.Now()
			task.StopReason = "process lost during daemon restart"
			result.Lost = append(result.Lost, task)
		}
	}

	return result
}

// isProcessAlive is implemented in persist_unix.go and persist_windows.go.

// SaveSnapshot saves the current state of all tasks from a Manager.
func SaveSnapshot(m *Manager, dir string) error {
	p := NewPersistence(dir)
	tasks := m.List(false, 0)
	return p.Save(tasks)
}

// LoadAndReconcile loads persisted state and reconciles with live processes.
func LoadAndReconcile(dir string) (*ReconcileResult, error) {
	p := NewPersistence(dir)
	state, err := p.Load()
	if err != nil {
		return nil, err
	}
	if state == nil {
		return &ReconcileResult{}, nil
	}
	return Reconcile(state.Tasks), nil
}

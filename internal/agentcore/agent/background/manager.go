// Package background manages long-running tasks (shell commands, etc.)
// that execute in the background while the agent continues working.
package background

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os/exec"
	"strings"
	"sync"
	"time"
)

// Status represents the lifecycle state of a background task.
type Status string

const (
	StatusRunning   Status = "running"
	StatusCompleted Status = "completed"
	StatusFailed    Status = "failed"
	StatusKilled    Status = "killed"
	StatusTimeout   Status = "timed_out"
)

// IsTerminal reports whether the status represents a final, non-running state.
func (s Status) IsTerminal() bool {
	switch s {
	case StatusCompleted, StatusFailed, StatusKilled, StatusTimeout:
		return true
	}
	return false
}

// TaskInfo is the public metadata for a background task.
type TaskInfo struct {
	TaskID     string    `json:"taskId"`
	Command    string    `json:"command"`
	Status     Status    `json:"status"`
	PID        int       `json:"pid,omitempty"`
	StartedAt  time.Time `json:"startedAt"`
	EndedAt    time.Time `json:"endedAt,omitempty"`
	ExitCode   int       `json:"exitCode,omitempty"`
	StopReason string    `json:"stopReason,omitempty"`
	WorkDir    string    `json:"workDir"`
}

// managedTask is the internal representation of a running task.
type managedTask struct {
	info       TaskInfo
	cmd        *exec.Cmd
	cancel     context.CancelFunc
	output     *bytes.Buffer
	outputMu   sync.Mutex
	done       chan struct{}
	totalBytes int64
}

// Manager tracks all background tasks for an agent session.
type Manager struct {
	mu     sync.RWMutex
	tasks  map[string]*managedTask
	nextID int
}

// NewManager creates a new background task manager.
func NewManager() *Manager {
	return &Manager{
		tasks: make(map[string]*managedTask),
	}
}

// StartProcess starts a shell command in the background and returns the task ID.
func (m *Manager) StartProcess(ctx context.Context, command, workDir string, env []string) (string, error) {
	taskCtx, cancel := context.WithCancel(ctx)

	cmd := exec.CommandContext(taskCtx, "sh", "-c", command)
	cmd.Dir = workDir
	if len(env) > 0 {
		cmd.Env = env
	}

	m.mu.Lock()
	m.nextID++
	taskID := fmt.Sprintf("bg_%d", m.nextID)
	m.mu.Unlock()

	task := &managedTask{
		info: TaskInfo{
			TaskID:    taskID,
			Command:   command,
			Status:    StatusRunning,
			StartedAt: time.Now(),
			WorkDir:   workDir,
		},
		cmd:    cmd,
		cancel: cancel,
		output: &bytes.Buffer{},
		done:   make(chan struct{}),
	}

	// Capture stdout and stderr combined
	outputPipe, err := cmd.StdoutPipe()
	if err != nil {
		cancel()
		return "", fmt.Errorf("stdout pipe: %w", err)
	}
	cmd.Stderr = cmd.Stdout // merge stderr into stdout

	m.mu.Lock()
	m.tasks[taskID] = task
	m.mu.Unlock()

	if err := cmd.Start(); err != nil {
		m.mu.Lock()
		task.info.Status = StatusFailed
		task.info.EndedAt = time.Now()
		task.info.StopReason = err.Error()
		m.mu.Unlock()
		close(task.done)
		cancel()
		return taskID, nil // still return ID, task just failed immediately
	}

	m.mu.Lock()
	task.info.PID = cmd.Process.Pid
	m.mu.Unlock()

	// Capture output in background
	go func() {
		buf := make([]byte, 4096)
		for {
			n, readErr := outputPipe.Read(buf)
			if n > 0 {
				task.outputMu.Lock()
				task.output.Write(buf[:n])
				task.totalBytes += int64(n)
				task.outputMu.Unlock()
			}
			if readErr != nil {
				break
			}
		}
	}()

	// Wait for process to finish in background
	go func() {
		defer close(task.done)
		waitErr := cmd.Wait()

		m.mu.Lock()
		defer m.mu.Unlock()

		task.info.EndedAt = time.Now()

		if waitErr != nil {
			if taskCtx.Err() != nil {
				// Context was cancelled — killed
				if task.info.Status == StatusRunning {
					task.info.Status = StatusKilled
				}
			} else {
				task.info.Status = StatusFailed
				var exitErr *exec.ExitError
				if errors.As(waitErr, &exitErr) {
					task.info.ExitCode = exitErr.ExitCode()
				}
			}
		} else {
			task.info.Status = StatusCompleted
			task.info.ExitCode = 0
		}
	}()

	return taskID, nil
}

// List returns task info, optionally filtering to active-only tasks.
func (m *Manager) List(activeOnly bool, limit int) []TaskInfo {
	m.mu.RLock()
	defer m.mu.RUnlock()

	var result []TaskInfo
	for _, task := range m.tasks {
		if activeOnly && task.info.Status.IsTerminal() {
			continue
		}
		result = append(result, task.info)
		if len(result) >= limit {
			break
		}
	}
	return result
}

// GetTask returns info for a single task, or nil if not found.
func (m *Manager) GetTask(taskID string) *TaskInfo {
	m.mu.RLock()
	defer m.mu.RUnlock()

	task, ok := m.tasks[taskID]
	if !ok {
		return nil
	}
	info := task.info
	return &info
}

// Stop terminates a running task and returns the updated info.
func (m *Manager) Stop(taskID, reason string) (*TaskInfo, error) {
	m.mu.RLock()
	task, ok := m.tasks[taskID]
	m.mu.RUnlock()

	if !ok {
		return nil, fmt.Errorf("task not found: %s", taskID)
	}

	if task.info.Status.IsTerminal() {
		info := task.info
		return &info, nil
	}

	// Cancel context to kill the process
	task.cancel()

	// Wait for process to finish
	select {
	case <-task.done:
	case <-time.After(5 * time.Second):
		// Force kill if graceful stop takes too long
		if task.cmd.Process != nil {
			_ = task.cmd.Process.Kill()
		}
		<-task.done
	}

	m.mu.Lock()
	if task.info.Status == StatusRunning {
		task.info.Status = StatusKilled
	}
	task.info.StopReason = reason
	m.mu.Unlock()

	info := task.info
	return &info, nil
}

// GetOutput returns the tail of the task's captured output, up to maxBytes.
func (m *Manager) GetOutput(taskID string, maxBytes int) (string, error) {
	m.mu.RLock()
	task, ok := m.tasks[taskID]
	m.mu.RUnlock()

	if !ok {
		return "", fmt.Errorf("task not found: %s", taskID)
	}

	task.outputMu.Lock()
	defer task.outputMu.Unlock()

	data := task.output.Bytes()
	if len(data) > maxBytes {
		data = data[len(data)-maxBytes:]
	}
	return string(data), nil
}

// Wait blocks until the task reaches a terminal state or the timeout expires.
func (m *Manager) Wait(taskID string, timeout time.Duration) error {
	m.mu.RLock()
	task, ok := m.tasks[taskID]
	m.mu.RUnlock()

	if !ok {
		return fmt.Errorf("task not found: %s", taskID)
	}

	select {
	case <-task.done:
		return nil
	case <-time.After(timeout):
		return fmt.Errorf("timeout waiting for task %s", taskID)
	}
}

// SuppressTerminalNotification is a no-op placeholder for TS compat.
func (m *Manager) SuppressTerminalNotification(taskID string) {
	// No-op in Go port — notifications not yet implemented
}

// GetOutputSnapshot returns a structured output snapshot for the TaskOutput tool.
type OutputSnapshot struct {
	Output            string
	OutputSizeBytes   int64
	PreviewBytes      int
	Truncated         bool
	FullOutputPath    string
	FullOutputAvail   bool
}

// GetOutputSnapshot returns a preview of the task output.
func (m *Manager) GetOutputSnapshot(taskID string, maxPreviewBytes int) (*OutputSnapshot, error) {
	m.mu.RLock()
	task, ok := m.tasks[taskID]
	m.mu.RUnlock()

	if !ok {
		return nil, fmt.Errorf("task not found: %s", taskID)
	}

	task.outputMu.Lock()
	defer task.outputMu.Unlock()

	data := task.output.Bytes()
	totalSize := task.totalBytes

	preview := data
	truncated := false
	if len(preview) > maxPreviewBytes {
		preview = preview[len(preview)-maxPreviewBytes:]
		truncated = true
	}

	return &OutputSnapshot{
		Output:          string(preview),
		OutputSizeBytes: totalSize,
		PreviewBytes:    len(preview),
		Truncated:       truncated,
		FullOutputAvail: false, // no disk persistence in this simple impl
	}, nil
}

// FormatTaskList renders a human-readable task list.
func FormatTaskList(tasks []TaskInfo, activeOnly bool) string {
	label := "background_tasks"
	if activeOnly {
		label = "active_background_tasks"
	}
	header := fmt.Sprintf("%s: %d", label, len(tasks))
	if len(tasks) == 0 {
		return header + "\nNo background tasks found."
	}

	var sb strings.Builder
	sb.WriteString(header)
	for _, t := range tasks {
		sb.WriteString("\n---\n")
		sb.WriteString(fmt.Sprintf("task_id: %s\n", t.TaskID))
		sb.WriteString(fmt.Sprintf("command: %s\n", t.Command))
		sb.WriteString(fmt.Sprintf("status: %s\n", t.Status))
		if t.PID > 0 {
			sb.WriteString(fmt.Sprintf("pid: %d\n", t.PID))
		}
		sb.WriteString(fmt.Sprintf("started_at: %s\n", t.StartedAt.Format(time.RFC3339)))
		if !t.EndedAt.IsZero() {
			sb.WriteString(fmt.Sprintf("ended_at: %s\n", t.EndedAt.Format(time.RFC3339)))
		}
		if t.Status.IsTerminal() {
			sb.WriteString(fmt.Sprintf("exit_code: %d\n", t.ExitCode))
		}
		if t.StopReason != "" {
			sb.WriteString(fmt.Sprintf("stop_reason: %s\n", t.StopReason))
		}
	}
	return sb.String()
}

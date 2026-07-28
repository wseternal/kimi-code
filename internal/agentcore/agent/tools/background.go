package tools

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
	"strings"
	"time"

	"github.com/visdomtech/kimi-code/internal/agentcore/agent/background"
)

// ── TaskList ─────────────────────────────────────────────────────────

// TaskListTool lists background tasks.
type TaskListTool struct {
	manager *background.Manager
}

func NewTaskListTool(manager *background.Manager) *TaskListTool {
	return &TaskListTool{manager: manager}
}

type taskListInput struct {
	ActiveOnly *bool `json:"active_only,omitempty"`
	Limit      *int  `json:"limit,omitempty"`
}

func (t *TaskListTool) Definition() Definition {
	return Definition{
		Name:        "TaskList",
		Description: "List background tasks and their current status. Use this to discover running tasks, find task IDs for TaskOutput/TaskStop.",
		Parameters: map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"active_only": map[string]interface{}{
					"type":        "boolean",
					"description": "Whether to list only non-terminal tasks. Default: true.",
				},
				"limit": map[string]interface{}{
					"type":        "integer",
					"description": "Maximum number of tasks to return. Default: 20.",
				},
			},
		},
	}
}

func (t *TaskListTool) Execute(_ context.Context, input json.RawMessage, _ ExecContext) (*Result, error) {
	var args taskListInput
	if len(input) > 0 {
		if err := json.Unmarshal(input, &args); err != nil {
			return &Result{Output: err.Error(), IsError: true}, nil
		}
	}

	activeOnly := true
	if args.ActiveOnly != nil {
		activeOnly = *args.ActiveOnly
	}
	limit := 20
	if args.Limit != nil {
		limit = *args.Limit
	}

	tasks := t.manager.List(activeOnly, limit)
	output := background.FormatTaskList(tasks, activeOnly)
	return &Result{Output: output}, nil
}

// ── TaskOutput ───────────────────────────────────────────────────────

const outputPreviewBytes = 32 * 1024 // 32 KiB

// TaskOutputTool reads output from a background task.
type TaskOutputTool struct {
	manager *background.Manager
}

func NewTaskOutputTool(manager *background.Manager) *TaskOutputTool {
	return &TaskOutputTool{manager: manager}
}

type taskOutputInput struct {
	TaskID  string `json:"task_id"`
	Block   *bool  `json:"block,omitempty"`
	Timeout *int   `json:"timeout,omitempty"`
}

func (t *TaskOutputTool) Definition() Definition {
	return Definition{
		Name:        "TaskOutput",
		Description: "Retrieve a snapshot of a running or completed background task's status and output. Use after Bash(run_in_background=true) to check progress.",
		Parameters: map[string]interface{}{
			"type":     "object",
			"required": []string{"task_id"},
			"properties": map[string]interface{}{
				"task_id": map[string]interface{}{
					"type":        "string",
					"description": "The background task ID to inspect.",
				},
				"block": map[string]interface{}{
					"type":        "boolean",
					"description": "Whether to wait for the task to finish. Default: false.",
				},
				"timeout": map[string]interface{}{
					"type":        "integer",
					"description": "Max seconds to wait when block=true. Default: 30.",
				},
			},
		},
	}
}

func (t *TaskOutputTool) Execute(ctx context.Context, input json.RawMessage, _ ExecContext) (*Result, error) {
	var args taskOutputInput
	if err := json.Unmarshal(input, &args); err != nil {
		return &Result{Output: err.Error(), IsError: true}, nil
	}

	if args.TaskID == "" {
		return &Result{Output: "task_id is required", IsError: true}, nil
	}

	// Optionally block-wait
	block := args.Block != nil && *args.Block
	if block {
		timeout := 30
		if args.Timeout != nil {
			timeout = *args.Timeout
		}
		_ = t.manager.Wait(args.TaskID, time.Duration(timeout)*time.Second)
	}

	info := t.manager.GetTask(args.TaskID)
	if info == nil {
		return &Result{Output: fmt.Sprintf("Task not found: %s", args.TaskID), IsError: true}, nil
	}

	snapshot, err := t.manager.GetOutputSnapshot(args.TaskID, outputPreviewBytes)
	if err != nil {
		return &Result{Output: err.Error(), IsError: true}, nil
	}

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("task_id: %s\n", info.TaskID))
	sb.WriteString(fmt.Sprintf("command: %s\n", info.Command))
	sb.WriteString(fmt.Sprintf("status: %s\n", info.Status))
	if info.PID > 0 {
		sb.WriteString(fmt.Sprintf("pid: %d\n", info.PID))
	}
	sb.WriteString(fmt.Sprintf("started_at: %s\n", info.StartedAt.Format(time.RFC3339)))
	if !info.EndedAt.IsZero() {
		sb.WriteString(fmt.Sprintf("ended_at: %s\n", info.EndedAt.Format(time.RFC3339)))
		sb.WriteString(fmt.Sprintf("exit_code: %d\n", info.ExitCode))
	}
	if info.StopReason != "" {
		sb.WriteString(fmt.Sprintf("stop_reason: %s\n", info.StopReason))
	}
	sb.WriteString(fmt.Sprintf("output_size_bytes: %d\n", snapshot.OutputSizeBytes))
	sb.WriteString(fmt.Sprintf("output_preview_bytes: %d\n", snapshot.PreviewBytes))
	sb.WriteString(fmt.Sprintf("output_truncated: %v\n", snapshot.Truncated))

	if snapshot.Truncated {
		sb.WriteString("\n[Truncated — only the tail is shown]\n")
	}
	sb.WriteString("\n[output]\n")
	if snapshot.Output != "" {
		sb.WriteString(snapshot.Output)
	} else {
		sb.WriteString("[no output available]")
	}

	return &Result{Output: sb.String()}, nil
}

// ── TaskStop ─────────────────────────────────────────────────────────

// TaskStopTool stops a running background task.
type TaskStopTool struct {
	manager *background.Manager
}

func NewTaskStopTool(manager *background.Manager) *TaskStopTool {
	return &TaskStopTool{manager: manager}
}

type taskStopInput struct {
	TaskID string `json:"task_id"`
	Reason string `json:"reason,omitempty"`
}

func (t *TaskStopTool) Definition() Definition {
	return Definition{
		Name:        "TaskStop",
		Description: "Stop a running background task. Only use when a task must be cancelled — for tasks finishing normally, wait or use TaskOutput.",
		Parameters: map[string]interface{}{
			"type":     "object",
			"required": []string{"task_id"},
			"properties": map[string]interface{}{
				"task_id": map[string]interface{}{
					"type":        "string",
					"description": "The background task ID to stop.",
				},
				"reason": map[string]interface{}{
					"type":        "string",
					"description": "Short reason for stopping. Default: 'Stopped by TaskStop'.",
				},
			},
		},
	}
}

func (t *TaskStopTool) Execute(_ context.Context, input json.RawMessage, _ ExecContext) (*Result, error) {
	var args taskStopInput
	if err := json.Unmarshal(input, &args); err != nil {
		return &Result{Output: err.Error(), IsError: true}, nil
	}

	if args.TaskID == "" {
		return &Result{Output: "task_id is required", IsError: true}, nil
	}

	reason := strings.TrimSpace(args.Reason)
	if reason == "" {
		reason = "Stopped by TaskStop"
	}

	info := t.manager.GetTask(args.TaskID)
	if info == nil {
		return &Result{Output: fmt.Sprintf("Task not found: %s", args.TaskID), IsError: true}, nil
	}

	// Already terminal — just report
	if info.Status.IsTerminal() {
		reason := info.StopReason
		if reason == "" {
			reason = "Task already in terminal state"
		}
		return &Result{
			Output: fmt.Sprintf("task_id: %s\nstatus: %s\nreason: %s",
				info.TaskID, info.Status, reason),
		}, nil
	}

	result, err := t.manager.Stop(args.TaskID, reason)
	if err != nil {
		return &Result{Output: fmt.Sprintf("Failed to stop task: %s", args.TaskID), IsError: true}, nil
	}

	stopReason := result.StopReason
	if stopReason == "" {
		stopReason = reason
	}
	return &Result{
		Output: fmt.Sprintf("task_id: %s\nstatus: %s\nreason: %s",
			result.TaskID, result.Status, stopReason),
	}, nil
}

// ── BackgroundBashTool ───────────────────────────────────────────────

// BackgroundBashTool runs shell commands, optionally in the background.
// It replaces the simple BashTool when a BackgroundManager is available.
type BackgroundBashTool struct {
	manager *background.Manager
}

func NewBackgroundBashTool(manager *background.Manager) *BackgroundBashTool {
	return &BackgroundBashTool{manager: manager}
}

type backgroundBashInput struct {
	Command         string `json:"command"`
	RunInBackground bool   `json:"run_in_background,omitempty"`
	Timeout         *int   `json:"timeout,omitempty"`
}

func (t *BackgroundBashTool) Definition() Definition {
	return Definition{
		Name:        "Bash",
		Description: "Execute a shell command. Use run_in_background=true for long-running tasks that should not block the conversation.",
		Parameters: map[string]interface{}{
			"type":     "object",
			"required": []string{"command"},
			"properties": map[string]interface{}{
				"command": map[string]interface{}{
					"type":        "string",
					"description": "The shell command to execute.",
				},
				"run_in_background": map[string]interface{}{
					"type":        "boolean",
					"description": "Run the command in the background. Default: false.",
				},
				"timeout": map[string]interface{}{
					"type":        "integer",
					"description": "Timeout in seconds for foreground commands. Default: 120.",
				},
			},
		},
	}
}

func (t *BackgroundBashTool) Execute(ctx context.Context, input json.RawMessage, execCtx ExecContext) (*Result, error) {
	var args backgroundBashInput
	if err := json.Unmarshal(input, &args); err != nil {
		return &Result{Output: err.Error(), IsError: true}, nil
	}

	if args.Command == "" {
		return &Result{Output: "command is required", IsError: true}, nil
	}

	workDir := execCtx.WorkDir
	if workDir == "" {
		workDir = "."
	}

	if args.RunInBackground {
		taskID, err := t.manager.StartProcess(ctx, args.Command, workDir, nil)
		if err != nil {
			return &Result{Output: fmt.Sprintf("Failed to start background task: %v", err), IsError: true}, nil
		}
		return &Result{
			Output: fmt.Sprintf("Background task started.\ntask_id: %s\ncommand: %s", taskID, args.Command),
		}, nil
	}

	// Foreground execution
	timeout := 120
	if args.Timeout != nil {
		timeout = *args.Timeout
	}

	cmdCtx, cancel := context.WithTimeout(ctx, time.Duration(timeout)*time.Second)
	defer cancel()

	cmd := exec.CommandContext(cmdCtx, "sh", "-c", args.Command)
	cmd.Dir = workDir

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	err := cmd.Run()
	output := stdout.String()
	if stderr.Len() > 0 {
		if output != "" {
			output += "\n"
		}
		output += stderr.String()
	}

	if err != nil {
		return &Result{
			Output:  fmt.Sprintf("exit_code: %d\n%s", cmd.ProcessState.ExitCode(), output),
			IsError: true,
		}, nil
	}

	return &Result{Output: output}, nil
}

// RegisterBackgroundTools registers all background-related tools.
func RegisterBackgroundTools(registry *Registry, manager *background.Manager) {
	registry.Register(NewTaskListTool(manager))
	registry.Register(NewTaskOutputTool(manager))
	registry.Register(NewTaskStopTool(manager))
}

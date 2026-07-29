package tools

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/visdomtech/kimi-code/internal/agentcore/agent/background"
)

func TestTaskListTool_Empty(t *testing.T) {
	mgr := background.NewManager()
	tool := NewTaskListTool(mgr)
	ctx := context.Background()

	input := json.RawMessage(`{}`)
	result, err := tool.Execute(ctx, input, ExecContext{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.IsError {
		t.Fatalf("unexpected error result: %s", result.Output)
	}
	if !strings.Contains(result.Output, "No background tasks found") {
		t.Errorf("expected empty message, got: %s", result.Output)
	}
}

func TestTaskListTool_WithTasks(t *testing.T) {
	mgr := background.NewManager()
	ctx := context.Background()

	// Start a background task
	mgr.StartProcess(ctx, "sleep 5", ".", nil)
	defer func() {
		for _, task := range mgr.List(true, 20) {
			mgr.Stop(task.TaskID, "cleanup")
		}
	}()

	tool := NewTaskListTool(mgr)
	input := json.RawMessage(`{}`)
	result, err := tool.Execute(ctx, input, ExecContext{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(result.Output, "active_background_tasks: 1") {
		t.Errorf("expected 1 task in output, got: %s", result.Output)
	}
}

func TestTaskOutputTool_NotFound(t *testing.T) {
	mgr := background.NewManager()
	tool := NewTaskOutputTool(mgr)
	ctx := context.Background()

	input := json.RawMessage(`{"task_id": "nonexistent"}`)
	result, err := tool.Execute(ctx, input, ExecContext{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.IsError {
		t.Error("expected error result for non-existent task")
	}
	if !strings.Contains(result.Output, "not found") {
		t.Errorf("expected 'not found' in output, got: %s", result.Output)
	}
}

func TestTaskOutputTool_GetOutput(t *testing.T) {
	mgr := background.NewManager()
	ctx := context.Background()

	taskID, _ := mgr.StartProcess(ctx, "echo 'task output test'", ".", nil)
	if err := mgr.Wait(taskID, 5*time.Second); err != nil {
		t.Fatalf("wait for task: %v", err)
	}

	tool := NewTaskOutputTool(mgr)
	input, _ := json.Marshal(map[string]interface{}{"task_id": taskID})
	result, err := tool.Execute(ctx, json.RawMessage(input), ExecContext{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.IsError {
		t.Fatalf("unexpected error result: %s", result.Output)
	}
	if !strings.Contains(result.Output, "task output test") {
		t.Errorf("expected output content, got: %s", result.Output)
	}
}

func TestTaskStopTool_NotFound(t *testing.T) {
	mgr := background.NewManager()
	tool := NewTaskStopTool(mgr)
	ctx := context.Background()

	input := json.RawMessage(`{"task_id": "nonexistent"}`)
	result, err := tool.Execute(ctx, input, ExecContext{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.IsError {
		t.Error("expected error result for non-existent task")
	}
}

func TestTaskStopTool_Stop(t *testing.T) {
	mgr := background.NewManager()
	ctx := context.Background()

	taskID, _ := mgr.StartProcess(ctx, "sleep 10", ".", nil)

	tool := NewTaskStopTool(mgr)
	input, _ := json.Marshal(map[string]interface{}{"task_id": taskID, "reason": "test stop"})
	result, err := tool.Execute(ctx, json.RawMessage(input), ExecContext{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.IsError {
		t.Fatalf("unexpected error result: %s", result.Output)
	}
	if !strings.Contains(result.Output, taskID) {
		t.Errorf("expected task ID in output, got: %s", result.Output)
	}
}

func TestBackgroundBashTool_RunInBackground(t *testing.T) {
	mgr := background.NewManager()
	tool := NewBackgroundBashTool(mgr)
	ctx := context.Background()

	input := json.RawMessage(`{"command": "echo bg_test_output", "run_in_background": true}`)
	result, err := tool.Execute(ctx, input, ExecContext{WorkDir: "."})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.IsError {
		t.Fatalf("unexpected error result: %s", result.Output)
	}
	if !strings.Contains(result.Output, "task_id") {
		t.Fatalf("expected task_id in output, got: %s", result.Output)
	}
	// Extract task_id from output: "task_id: bg_N"
	var taskID string
	for _, line := range strings.Split(result.Output, "\n") {
		if after, ok := strings.CutPrefix(strings.TrimSpace(line), "task_id:"); ok {
			taskID = strings.TrimSpace(after)
			break
		}
	}
	if taskID == "" {
		t.Fatalf("could not parse task_id from output: %s", result.Output)
	}

	// Wait for the process to complete so output is fully buffered.
	if err := mgr.Wait(taskID, 5*time.Second); err != nil {
		t.Fatalf("wait for task: %v", err)
	}
	output, _ := mgr.GetOutput(taskID, 1024)
	if !strings.Contains(output, "bg_test_output") {
		t.Errorf("expected output in task, got: %s", output)
	}
	mgr.Stop(taskID, "cleanup")
}

func TestBackgroundBashTool_RunInForeground(t *testing.T) {
	mgr := background.NewManager()
	tool := NewBackgroundBashTool(mgr)
	ctx := context.Background()

	input := json.RawMessage(`{"command": "echo foreground_test", "run_in_background": false}`)
	result, err := tool.Execute(ctx, input, ExecContext{WorkDir: "."})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.IsError {
		t.Fatalf("unexpected error result: %s", result.Output)
	}
	if !strings.Contains(result.Output, "foreground_test") {
		t.Errorf("expected direct output, got: %s", result.Output)
	}
}

func TestToolDefinitions(t *testing.T) {
	mgr := background.NewManager()

	tl := NewTaskListTool(mgr)
	def := tl.Definition()
	if def.Name != "TaskList" {
		t.Errorf("expected name TaskList, got %s", def.Name)
	}

	to := NewTaskOutputTool(mgr)
	def = to.Definition()
	if def.Name != "TaskOutput" {
		t.Errorf("expected name TaskOutput, got %s", def.Name)
	}

	ts := NewTaskStopTool(mgr)
	def = ts.Definition()
	if def.Name != "TaskStop" {
		t.Errorf("expected name TaskStop, got %s", def.Name)
	}
}

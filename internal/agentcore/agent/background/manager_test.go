package background

import (
	"context"
	"strings"
	"testing"
	"time"
)

func TestManager_RegisterAndList(t *testing.T) {
	mgr := NewManager()
	ctx := context.Background()

	// Initially empty
	tasks := mgr.List(true, 20)
	if len(tasks) != 0 {
		t.Errorf("expected 0 tasks, got %d", len(tasks))
	}

	// Register a background task
	taskID, err := mgr.StartProcess(ctx, "echo hello", ".", nil)
	if err != nil {
		t.Fatalf("failed to start task: %v", err)
	}
	if taskID == "" {
		t.Fatal("expected non-empty task ID")
	}

	// List should show 1 active task
	tasks = mgr.List(true, 20)
	if len(tasks) != 1 {
		t.Errorf("expected 1 task, got %d", len(tasks))
	}
	if tasks[0].TaskID != taskID {
		t.Errorf("expected task ID %s, got %s", taskID, tasks[0].TaskID)
	}
}

func TestManager_GetTask(t *testing.T) {
	mgr := NewManager()
	ctx := context.Background()

	taskID, _ := mgr.StartProcess(ctx, "echo test", ".", nil)

	info := mgr.GetTask(taskID)
	if info == nil {
		t.Fatal("expected task info")
	}
	if info.TaskID != taskID {
		t.Errorf("expected task ID %s, got %s", taskID, info.TaskID)
	}

	// Non-existent task
	info = mgr.GetTask("nonexistent")
	if info != nil {
		t.Error("expected nil for non-existent task")
	}
}

func TestManager_Stop(t *testing.T) {
	mgr := NewManager()
	ctx := context.Background()

	// Start a long-running task
	taskID, _ := mgr.StartProcess(ctx, "sleep 10", ".", nil)

	// Stop it
	result, err := mgr.Stop(taskID, "test stop")
	if err != nil {
		t.Fatalf("failed to stop task: %v", err)
	}
	if result == nil {
		t.Fatal("expected stop result")
	}

	// Wait for task to be finalized
	time.Sleep(100 * time.Millisecond)

	info := mgr.GetTask(taskID)
	if info == nil {
		t.Fatal("expected task info after stop")
	}
	if info.Status != StatusKilled {
		t.Errorf("expected status killed, got %s", info.Status)
	}
}

func TestManager_GetOutput(t *testing.T) {
	mgr := NewManager()
	ctx := context.Background()

	taskID, _ := mgr.StartProcess(ctx, "echo 'hello world'", ".", nil)

	// Wait for completion
	time.Sleep(200 * time.Millisecond)

	output, err := mgr.GetOutput(taskID, 1024)
	if err != nil {
		t.Fatalf("failed to get output: %v", err)
	}
	if !strings.Contains(output, "hello world") {
		t.Errorf("expected 'hello world' in output, got: %s", output)
	}
}

func TestManager_Wait(t *testing.T) {
	mgr := NewManager()
	ctx := context.Background()

	taskID, _ := mgr.StartProcess(ctx, "sleep 0.1", ".", nil)

	// Wait for completion
	err := mgr.Wait(taskID, 5*time.Second)
	if err != nil {
		t.Fatalf("wait failed: %v", err)
	}

	info := mgr.GetTask(taskID)
	if info.Status != StatusCompleted {
		t.Errorf("expected completed status, got %s", info.Status)
	}
}

func TestManager_WaitTimeout(t *testing.T) {
	mgr := NewManager()
	ctx := context.Background()

	taskID, _ := mgr.StartProcess(ctx, "sleep 10", ".", nil)
	defer mgr.Stop(taskID, "cleanup")

	// Wait with short timeout
	err := mgr.Wait(taskID, 100*time.Millisecond)
	if err == nil {
		t.Error("expected timeout error")
	}
}

func TestManager_ListActiveOnly(t *testing.T) {
	mgr := NewManager()
	ctx := context.Background()

	// Start and complete a task
	taskID1, _ := mgr.StartProcess(ctx, "echo done", ".", nil)
	time.Sleep(200 * time.Millisecond)

	// Start a running task
	taskID2, _ := mgr.StartProcess(ctx, "sleep 10", ".", nil)
	defer mgr.Stop(taskID2, "cleanup")

	// List active only
	active := mgr.List(true, 20)
	if len(active) != 1 {
		t.Errorf("expected 1 active task, got %d", len(active))
	}
	if active[0].TaskID != taskID2 {
		t.Errorf("expected active task %s, got %s", taskID2, active[0].TaskID)
	}

	// List all
	all := mgr.List(false, 20)
	if len(all) != 2 {
		t.Errorf("expected 2 total tasks, got %d", len(all))
	}
	_ = taskID1
}

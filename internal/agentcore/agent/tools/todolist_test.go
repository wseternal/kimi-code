package tools

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
)

func TestTodoListTool_QueryEmpty(t *testing.T) {
	tool := NewTodoListTool()
	ctx := context.Background()
	exec := ExecContext{}

	// Query with no todos set
	input := json.RawMessage(`{}`)
	result, err := tool.Execute(ctx, input, exec)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.IsError {
		t.Fatalf("unexpected error result: %s", result.Output)
	}
	if !strings.Contains(result.Output, "empty") {
		t.Errorf("expected 'empty' in output, got: %s", result.Output)
	}
}

func TestTodoListTool_Write(t *testing.T) {
	tool := NewTodoListTool()
	ctx := context.Background()
	exec := ExecContext{}

	// Write 2 todos
	input := json.RawMessage(`{"todos": [{"title": "Implement feature X", "status": "in_progress"}, {"title": "Write tests", "status": "pending"}]}`)
	result, err := tool.Execute(ctx, input, exec)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.IsError {
		t.Fatalf("unexpected error result: %s", result.Output)
	}
	if !strings.Contains(result.Output, "Implement feature X") {
		t.Errorf("expected todo title in output, got: %s", result.Output)
	}
	if !strings.Contains(result.Output, "[in_progress]") {
		t.Errorf("expected status marker in output, got: %s", result.Output)
	}
	if !strings.Contains(result.Output, "Write tests") {
		t.Errorf("expected second todo in output, got: %s", result.Output)
	}
}

func TestTodoListTool_QueryAfterWrite(t *testing.T) {
	tool := NewTodoListTool()
	ctx := context.Background()
	exec := ExecContext{}

	// Write todos
	writeInput := json.RawMessage(`{"todos": [{"title": "Task A", "status": "done"}, {"title": "Task B", "status": "pending"}]}`)
	tool.Execute(ctx, writeInput, exec)

	// Query (nil todos = read mode)
	queryInput := json.RawMessage(`{}`)
	result, err := tool.Execute(ctx, queryInput, exec)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(result.Output, "Task A") {
		t.Errorf("expected Task A in query result, got: %s", result.Output)
	}
	if !strings.Contains(result.Output, "[done]") {
		t.Errorf("expected [done] status, got: %s", result.Output)
	}
}

func TestTodoListTool_Clear(t *testing.T) {
	tool := NewTodoListTool()
	ctx := context.Background()
	exec := ExecContext{}

	// Write todos first
	writeInput := json.RawMessage(`{"todos": [{"title": "Task", "status": "pending"}]}`)
	tool.Execute(ctx, writeInput, exec)

	// Clear with empty array
	clearInput := json.RawMessage(`{"todos": []}`)
	result, err := tool.Execute(ctx, clearInput, exec)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(result.Output, "cleared") {
		t.Errorf("expected 'cleared' in output, got: %s", result.Output)
	}

	// Query to verify empty
	queryInput := json.RawMessage(`{}`)
	result, err = tool.Execute(ctx, queryInput, exec)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(result.Output, "empty") {
		t.Errorf("expected 'empty' after clear, got: %s", result.Output)
	}
}

func TestTodoListTool_Replace(t *testing.T) {
	tool := NewTodoListTool()
	ctx := context.Background()
	exec := ExecContext{}

	// Write initial todos
	writeInput := json.RawMessage(`{"todos": [{"title": "Old task", "status": "pending"}]}`)
	tool.Execute(ctx, writeInput, exec)

	// Replace with new todos
	replaceInput := json.RawMessage(`{"todos": [{"title": "New task", "status": "in_progress"}]}`)
	result, err := tool.Execute(ctx, replaceInput, exec)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(result.Output, "New task") {
		t.Errorf("expected 'New task' in output, got: %s", result.Output)
	}
	if strings.Contains(result.Output, "Old task") {
		t.Errorf("old task should be replaced, got: %s", result.Output)
	}
}

func TestTodoListTool_InvalidStatus(t *testing.T) {
	tool := NewTodoListTool()
	ctx := context.Background()
	exec := ExecContext{}

	// Invalid status value
	input := json.RawMessage(`{"todos": [{"title": "Task", "status": "invalid_status"}]}`)
	result, err := tool.Execute(ctx, input, exec)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.IsError {
		t.Errorf("expected error for invalid status, got success: %s", result.Output)
	}
}

func TestTodoListTool_ConcurrentSafety(t *testing.T) {
	tool := NewTodoListTool()
	ctx := context.Background()
	exec := ExecContext{}

	// Concurrent writes should not panic
	done := make(chan bool, 10)
	for i := 0; i < 10; i++ {
		go func(n int) {
			input := json.RawMessage(`{"todos": [{"title": "Task", "status": "pending"}]}`)
			tool.Execute(ctx, input, exec)
			done <- true
		}(i)
	}
	for i := 0; i < 10; i++ {
		<-done
	}

	// Query should work after concurrent writes
	queryInput := json.RawMessage(`{}`)
	result, err := tool.Execute(ctx, queryInput, exec)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(result.Output, "Task") {
		t.Errorf("expected 'Task' after concurrent writes, got: %s", result.Output)
	}
}

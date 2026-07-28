package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"sync"
)

// TodoItem represents a single todo entry.
type TodoItem struct {
	Title  string `json:"title"`
	Status string `json:"status"` // "pending", "in_progress", "done"
}

// Valid todo statuses.
var validTodoStatuses = map[string]bool{
	"pending":     true,
	"in_progress": true,
	"done":        true,
}

// TodoListTool manages an in-memory todo list for the session.
type TodoListTool struct {
	mu    sync.Mutex
	todos []TodoItem
}

func NewTodoListTool() *TodoListTool {
	return &TodoListTool{}
}

type todoListInput struct {
	Todos *[]TodoItem `json:"todos,omitempty"` // nil = query, [] = clear, [...] = replace
}

func (t *TodoListTool) Definition() Definition {
	return Definition{
		Name:        "TodoList",
		Description: "Manage a todo list for tracking tasks. Omit 'todos' to read the current list. Pass an array to update. Pass [] to clear.",
		Parameters: map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"todos": map[string]interface{}{
					"type": "array",
					"items": map[string]interface{}{
						"type": "object",
						"properties": map[string]interface{}{
							"title":  map[string]interface{}{"type": "string", "description": "Task description"},
							"status": map[string]interface{}{"type": "string", "enum": []string{"pending", "in_progress", "done"}, "description": "Task status"},
						},
						"required": []string{"title", "status"},
					},
					"description": "The updated todo list. Omit to read current list. Pass [] to clear.",
				},
			},
		},
	}
}

func (t *TodoListTool) Execute(_ context.Context, input json.RawMessage, _ ExecContext) (*Result, error) {
	var params todoListInput
	if err := json.Unmarshal(input, &params); err != nil {
		return nil, err
	}

	t.mu.Lock()
	defer t.mu.Unlock()

	// Query mode: todos field is nil
	if params.Todos == nil {
		return &Result{Output: t.render()}, nil
	}

	// Clear mode: empty array
	if len(*params.Todos) == 0 {
		t.todos = nil
		return &Result{Output: "Todo list cleared."}, nil
	}

	// Validate statuses
	for _, item := range *params.Todos {
		if !validTodoStatuses[item.Status] {
			return &Result{
				Output:  fmt.Sprintf("Invalid status %q. Must be one of: pending, in_progress, done", item.Status),
				IsError: true,
			}, nil
		}
	}

	// Write mode: replace list
	t.todos = make([]TodoItem, len(*params.Todos))
	copy(t.todos, *params.Todos)

	return &Result{Output: t.render()}, nil
}

func (t *TodoListTool) render() string {
	if len(t.todos) == 0 {
		return "Todo list is empty."
	}

	var b strings.Builder
	b.WriteString("Current todo list:\n")
	for _, item := range t.todos {
		b.WriteString(fmt.Sprintf("  [%s] %s\n", item.Status, item.Title))
	}
	return b.String()
}

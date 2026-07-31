package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/visdomtech/kimi-code/internal/agentcore/agent/plan"
)

// UpdatePlanTool lets the LLM update the plan task list shown in the TUI drawer.
type UpdatePlanTool struct {
	Tracker *plan.Tracker
}

type updatePlanInput struct {
	Tasks []plan.Task `json:"tasks"`
}

func (t *UpdatePlanTool) Definition() Definition {
	return Definition{
		Name:        "update_plan",
		Description: "Update the plan task list shown in the progress drawer. Call this whenever you start, complete, or add tasks during your work. Replace the full list each time.",
		Parameters: map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"tasks": map[string]interface{}{
					"type":        "array",
					"description": "The complete task list with title and status for each task.",
					"items": map[string]interface{}{
						"type": "object",
						"properties": map[string]interface{}{
							"title":  map[string]interface{}{"type": "string", "description": "Short task description"},
							"status": map[string]interface{}{"type": "string", "description": "Task status: pending, active, or done"},
						},
						"required": []string{"title", "status"},
					},
				},
			},
			"required": []string{"tasks"},
		},
	}
}

func (t *UpdatePlanTool) Execute(_ context.Context, input json.RawMessage, _ ExecContext) (*Result, error) {
	var req updatePlanInput
	if err := json.Unmarshal(input, &req); err != nil {
		return nil, fmt.Errorf("parse update_plan input: %w", err)
	}

	// Normalize status values (case-insensitive, with synonyms)
	for i := range req.Tasks {
		switch strings.ToLower(string(req.Tasks[i].Status)) {
		case "pending":
			req.Tasks[i].Status = plan.StatusPending
		case "active", "in_progress", "in-progress":
			req.Tasks[i].Status = plan.StatusActive
		case "done", "complete", "completed":
			req.Tasks[i].Status = plan.StatusDone
		default:
			req.Tasks[i].Status = plan.StatusPending
		}
		// Clamp title length to prevent excessive data
		if len([]rune(req.Tasks[i].Title)) > 200 {
			req.Tasks[i].Title = string([]rune(req.Tasks[i].Title)[:200])
		}
	}

	// Cap task count to prevent unbounded storage
	const maxTasks = 50
	if len(req.Tasks) > maxTasks {
		req.Tasks = req.Tasks[:maxTasks]
	}

	t.Tracker.SetTasks(req.Tasks)

	pending, active, done, failed := t.Tracker.Counts()
	total := pending + active + done + failed
	summary := fmt.Sprintf("Plan updated: %d tasks (%d done", total, done)
	if failed > 0 {
		summary += fmt.Sprintf(", %d failed", failed)
	}
	summary += fmt.Sprintf(", %d active, %d pending)", active, pending)
	return &Result{
		Output: summary,
	}, nil
}

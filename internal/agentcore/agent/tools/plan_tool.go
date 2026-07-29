package tools

import (
	"context"
	"encoding/json"
	"fmt"

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

	// Normalize status values
	for i := range req.Tasks {
		switch req.Tasks[i].Status {
		case "pending", "active", "done":
			// valid
		default:
			req.Tasks[i].Status = plan.StatusPending
		}
	}

	t.Tracker.SetTasks(req.Tasks)

	pending, active, done := t.Tracker.Counts()
	total := pending + active + done
	return &Result{
		Output: fmt.Sprintf("Plan updated: %d tasks (%d done, %d active, %d pending)", total, done, active, pending),
	}, nil
}

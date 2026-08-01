package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/visdomtech/kimi-code/internal/agentcore/agent/cron"
)

// ── CronCreate Tool ──

// CronCreateTool allows the model to create a new cron-scheduled task.
type CronCreateTool struct {
	Manager *cron.CronManager
}

var _ Tool = (*CronCreateTool)(nil)

type cronCreateInput struct {
	Cron   string `json:"cron"`
	Prompt string `json:"prompt"`
	Model  string `json:"model,omitempty"`
}

func (t *CronCreateTool) Definition() Definition {
	return Definition{
		Name:        "CronCreate",
		Description: "Create a new cron-scheduled task. Uses standard 5-field cron expressions (minute hour day-of-month month day-of-week). The prompt will be submitted to the agent each time the schedule fires.",
		Parameters: map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"cron": map[string]interface{}{
					"type":        "string",
					"description": "Standard 5-field cron expression (e.g., '*/5 * * * *' for every 5 minutes, '0 9 * * 1' for Monday 9 AM).",
				},
				"prompt": map[string]interface{}{
					"type":        "string",
					"description": "The prompt to submit to the agent when the schedule fires.",
				},
				"model": map[string]interface{}{
					"type":        "string",
					"description": "Optional model override for the scheduled task.",
				},
			},
			"required": []string{"cron", "prompt"},
		},
	}
}

func (t *CronCreateTool) Execute(ctx context.Context, input json.RawMessage, exec ExecContext) (*Result, error) {
	var params cronCreateInput
	if err := json.Unmarshal(input, &params); err != nil {
		return nil, err
	}
	if params.Cron == "" {
		return &Result{Output: "Cron expression is required.", IsError: true}, nil
	}
	if params.Prompt == "" {
		return &Result{Output: "Prompt is required.", IsError: true}, nil
	}

	task, err := t.Manager.Create(params.Cron, params.Prompt, params.Model, exec.WorkDir)
	if err != nil {
		return &Result{Output: err.Error(), IsError: true}, nil
	}

	return &Result{Output: fmt.Sprintf("Created cron task %s\n  Schedule: %s\n  Next run: %s\n  Prompt: %s",
		task.ID, task.Expression, task.NextRunAt.Format(time.RFC3339), task.Prompt)}, nil
}

// ── CronDelete Tool ──

// CronDeleteTool removes a scheduled task.
type CronDeleteTool struct {
	Manager *cron.CronManager
}

var _ Tool = (*CronDeleteTool)(nil)

type cronDeleteInput struct {
	ID string `json:"id"`
}

func (t *CronDeleteTool) Definition() Definition {
	return Definition{
		Name:        "CronDelete",
		Description: "Delete a cron-scheduled task by its ID.",
		Parameters: map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"id": map[string]interface{}{
					"type":        "string",
					"description": "The ID of the cron task to delete (e.g., 'cron_1').",
				},
			},
			"required": []string{"id"},
		},
	}
}

func (t *CronDeleteTool) Execute(_ context.Context, input json.RawMessage, _ ExecContext) (*Result, error) {
	var params cronDeleteInput
	if err := json.Unmarshal(input, &params); err != nil {
		return nil, err
	}
	if params.ID == "" {
		return &Result{Output: "Task ID is required.", IsError: true}, nil
	}

	if !t.Manager.Delete(params.ID) {
		return &Result{Output: fmt.Sprintf("Task %s not found.", params.ID), IsError: true}, nil
	}
	return &Result{Output: fmt.Sprintf("Deleted cron task %s.", params.ID)}, nil
}

// ── CronList Tool ──

// CronListTool lists all scheduled tasks.
type CronListTool struct {
	Manager *cron.CronManager
}

var _ Tool = (*CronListTool)(nil)

func (t *CronListTool) Definition() Definition {
	return Definition{
		Name:        "CronList",
		Description: "List all cron-scheduled tasks with their IDs, schedules, next run times, and status.",
		Parameters: map[string]interface{}{
			"type":       "object",
			"properties": map[string]interface{}{},
		},
	}
}

func (t *CronListTool) Execute(_ context.Context, _ json.RawMessage, _ ExecContext) (*Result, error) {
	tasks := t.Manager.List()
	if len(tasks) == 0 {
		return &Result{Output: "No cron tasks scheduled."}, nil
	}

	var sb strings.Builder
	for i, task := range tasks {
		if i > 0 {
			sb.WriteString("\n")
		}
		status := "enabled"
		if !task.Enabled {
			status = "disabled"
		}
		sb.WriteString(fmt.Sprintf("ID: %s\n", task.ID))
		sb.WriteString(fmt.Sprintf("  Schedule: %s\n", task.Expression))
		sb.WriteString(fmt.Sprintf("  Prompt: %s\n", truncateStr(task.Prompt, 100)))
		if !task.NextRunAt.IsZero() {
			sb.WriteString(fmt.Sprintf("  Next run: %s\n", task.NextRunAt.Format(time.RFC3339)))
		}
		if !task.LastRunAt.IsZero() {
			sb.WriteString(fmt.Sprintf("  Last run: %s (run #%d)\n", task.LastRunAt.Format(time.RFC3339), task.RunCount))
		}
		sb.WriteString(fmt.Sprintf("  Status: %s\n", status))
	}

	return &Result{Output: sb.String()}, nil
}

func truncateStr(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen] + "..."
}

// RegisterCronTools registers all cron management tools.
func RegisterCronTools(registry *Registry, manager *cron.CronManager) {
	registry.Register(&CronCreateTool{Manager: manager})
	registry.Register(&CronDeleteTool{Manager: manager})
	registry.Register(&CronListTool{Manager: manager})
}

package tools

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/visdomtech/kimi-code/internal/agentcore/agent/goal"
)

// ── CreateGoal Tool ──

// CreateGoalTool allows the model to create a new autonomous goal.
type CreateGoalTool struct {
	Tracker *goal.Tracker
}

var _ Tool = (*CreateGoalTool)(nil)

type createGoalInput struct {
	Objective           string `json:"objective"`
	CompletionCriterion string `json:"completionCriterion,omitempty"`
	TurnBudget          *int   `json:"turnBudget,omitempty"`
}

func (t *CreateGoalTool) Definition() Definition {
	return Definition{
		Name:        "createGoal",
		Description: "Create a goal only when the user explicitly requests it. Do NOT proactively create goals based on your own judgment. Fails if a goal already exists; use updateGoal only for status changes.",
		Parameters: map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"objective": map[string]interface{}{
					"type":        "string",
					"description": "The concrete objective to start pursuing.",
				},
				"completionCriterion": map[string]interface{}{
					"type":        "string",
					"description": "Optional criterion to determine when the goal is complete.",
				},
				"turnBudget": map[string]interface{}{
					"type":        "integer",
					"description": "Maximum number of auto-continuation turns. Set only when an explicit turn budget is requested.",
				},
			},
			"required": []string{"objective"},
		},
	}
}

func (t *CreateGoalTool) Execute(_ context.Context, input json.RawMessage, _ ExecContext) (*Result, error) {
	var params createGoalInput
	if err := json.Unmarshal(input, &params); err != nil {
		return nil, err
	}
	if params.Objective == "" {
		return &Result{Output: "Objective is required.", IsError: true}, nil
	}

	var budget goal.BudgetLimits
	if params.TurnBudget != nil {
		budget.TurnBudget = params.TurnBudget
	}

	snap, _, err := t.Tracker.CreateGoal(params.Objective, params.CompletionCriterion, budget, "model")
	if err != nil {
		return &Result{Output: err.Error(), IsError: true}, nil
	}
	out, _ := json.Marshal(snap)
	return &Result{Output: string(out)}, nil
}

// ── GetGoal Tool ──

// GetGoalTool returns the current goal status.
type GetGoalTool struct {
	Tracker *goal.Tracker
}

var _ Tool = (*GetGoalTool)(nil)

func (t *GetGoalTool) Definition() Definition {
	return Definition{
		Name:        "getGoal",
		Description: "Get the current goal status, including objective, status, budget, and progress.",
		Parameters: map[string]interface{}{
			"type":       "object",
			"properties": map[string]interface{}{},
		},
	}
}

func (t *GetGoalTool) Execute(_ context.Context, _ json.RawMessage, _ ExecContext) (*Result, error) {
	snap := t.Tracker.Current()
	if snap == nil {
		return &Result{Output: "No goal is currently set."}, nil
	}
	out, _ := json.Marshal(snap)
	return &Result{Output: string(out)}, nil
}

// ── UpdateGoal Tool ──

// UpdateGoalTool changes the status of the current goal.
type UpdateGoalTool struct {
	Tracker *goal.Tracker
}

var _ Tool = (*UpdateGoalTool)(nil)

type updateGoalInput struct {
	Status string `json:"status"`
	Reason string `json:"reason,omitempty"`
}

func (t *UpdateGoalTool) Definition() Definition {
	return Definition{
		Name:        "updateGoal",
		Description: "Update the status of the current goal. Allowed values are \"complete\", \"blocked\", \"paused\", and \"active\". Use \"paused\" only immediately after a successful schedule task creation when there is an active goal, and use \"active\" only to resume a paused goal.",
		Parameters: map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"status": map[string]interface{}{
					"type":        "string",
					"enum":        []string{"complete", "blocked", "paused", "active"},
					"description": "The new status for the goal.",
				},
				"reason": map[string]interface{}{
					"type":        "string",
					"description": "Reason for the status change (optional).",
				},
			},
			"required": []string{"status"},
		},
	}
}

func (t *UpdateGoalTool) Execute(_ context.Context, input json.RawMessage, _ ExecContext) (*Result, error) {
	var params updateGoalInput
	if err := json.Unmarshal(input, &params); err != nil {
		return nil, err
	}

	var snap *goal.Snapshot
	var err error

	switch params.Status {
	case "complete":
		snap, _, err = t.Tracker.MarkComplete(params.Reason, "model")
	case "blocked":
		snap, _, err = t.Tracker.MarkBlocked(params.Reason, "model")
	case "paused":
		snap, _, err = t.Tracker.PauseGoal("model")
	case "active":
		snap, _, err = t.Tracker.ResumeGoal("model")
	default:
		return &Result{Output: fmt.Sprintf("Invalid status %q. Must be one of: complete, blocked, paused, active.", params.Status), IsError: true}, nil
	}

	if err != nil {
		return &Result{Output: err.Error(), IsError: true}, nil
	}
	out, _ := json.Marshal(snap)
	return &Result{Output: string(out)}, nil
}

// ── SetGoalBudget Tool ──

// SetGoalBudgetTool modifies the budget limits of the current goal.
type SetGoalBudgetTool struct {
	Tracker *goal.Tracker
}

var _ Tool = (*SetGoalBudgetTool)(nil)

type setGoalBudgetInput struct {
	TokenBudget       *int `json:"tokenBudget,omitempty"`
	TurnBudget        *int `json:"turnBudget,omitempty"`
	WallClockBudgetMs *int `json:"wallClockBudgetMs,omitempty"`
}

func (t *SetGoalBudgetTool) Definition() Definition {
	return Definition{
		Name:        "setGoalBudget",
		Description: "Set or update the resource budget limits for the current goal. Supports token budget, turn budget, and wall-clock time budget (in milliseconds).",
		Parameters: map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"tokenBudget": map[string]interface{}{
					"type":        "integer",
					"description": "Maximum number of tokens the goal can consume.",
				},
				"turnBudget": map[string]interface{}{
					"type":        "integer",
					"description": "Maximum number of turns the goal can use.",
				},
				"wallClockBudgetMs": map[string]interface{}{
					"type":        "integer",
					"description": "Maximum wall-clock time in milliseconds.",
				},
			},
		},
	}
}

func (t *SetGoalBudgetTool) Execute(_ context.Context, input json.RawMessage, _ ExecContext) (*Result, error) {
	var params setGoalBudgetInput
	if err := json.Unmarshal(input, &params); err != nil {
		return nil, err
	}

	if params.TokenBudget == nil && params.TurnBudget == nil && params.WallClockBudgetMs == nil {
		return &Result{Output: "At least one budget limit must be specified.", IsError: true}, nil
	}

	limits := goal.BudgetLimits{
		TokenBudget:       params.TokenBudget,
		TurnBudget:        params.TurnBudget,
		WallClockBudgetMs: params.WallClockBudgetMs,
	}

	snap := t.Tracker.Current()
	if snap == nil {
		return &Result{Output: "No goal is currently set.", IsError: true}, nil
	}
	t.Tracker.SetBudgetLimits(limits)
	snap = t.Tracker.Current()
	out, _ := json.Marshal(snap)
	return &Result{Output: fmt.Sprintf("Budget updated.\n%s", string(out))}, nil
}

// RegisterGoalTools registers all goal management tools.
// TODO(S1): Consider introducing a GoalTracker interface to reduce coupling
// on the concrete goal.Tracker type, enabling testing and alternative implementations.
func RegisterGoalTools(registry *Registry, tracker *goal.Tracker) {
	registry.Register(&CreateGoalTool{Tracker: tracker})
	registry.Register(&GetGoalTool{Tracker: tracker})
	registry.Register(&UpdateGoalTool{Tracker: tracker})
	registry.Register(&SetGoalBudgetTool{Tracker: tracker})
}

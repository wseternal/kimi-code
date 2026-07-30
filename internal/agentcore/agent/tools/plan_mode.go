package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"

	"github.com/visdomtech/kimi-code/internal/agentcore/agent/injection"
)

// PlanModeController manages the plan mode state and is shared between
// EnterPlanMode and ExitPlanMode tools.
type PlanModeController struct {
	mu       sync.RWMutex
	active   bool
	injector *injection.PlanModeInjector
}

// NewPlanModeController creates a plan mode controller.
func NewPlanModeController(injector *injection.PlanModeInjector) *PlanModeController {
	return &PlanModeController{injector: injector}
}

// IsActive reports whether plan mode is currently enabled.
func (c *PlanModeController) IsActive() bool {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.active
}

// SetActive sets the plan mode state.
func (c *PlanModeController) SetActive(active bool) {
	c.mu.Lock()
	c.active = active
	c.mu.Unlock()
}

// ── EnterPlanMode Tool ──

// EnterPlanModeTool switches the agent into plan mode where it focuses
// on analysis and planning rather than execution.
type EnterPlanModeTool struct {
	Controller *PlanModeController
}

func NewEnterPlanModeTool(ctrl *PlanModeController) *EnterPlanModeTool {
	return &EnterPlanModeTool{Controller: ctrl}
}

func (t *EnterPlanModeTool) Definition() Definition {
	return Definition{
		Name:        "EnterPlanMode",
		Description: "Switch to plan mode. In plan mode, focus on analyzing the problem, discussing approaches, and creating a detailed plan. No file modifications or command execution will be permitted. Use ExitPlanMode when ready to implement.",
		Parameters: map[string]interface{}{
			"type":       "object",
			"properties": map[string]interface{}{},
		},
	}
}

func (t *EnterPlanModeTool) Execute(_ context.Context, _ json.RawMessage, _ ExecContext) (*Result, error) {
	if t.Controller.IsActive() {
		return &Result{Output: "Already in plan mode."}, nil
	}
	t.Controller.SetActive(true)
	t.Controller.injector.SetPlanMode(true, "")
	return &Result{Output: "Entered plan mode. Focus on analysis, planning, and discussion. Use ExitPlanMode to return to implementation mode."}, nil
}

// ── ExitPlanMode Tool ──

// ExitPlanModeTool switches the agent out of plan mode back to implementation.
type ExitPlanModeTool struct {
	Controller *PlanModeController
}

func NewExitPlanModeTool(ctrl *PlanModeController) *ExitPlanModeTool {
	return &ExitPlanModeTool{Controller: ctrl}
}

type exitPlanModeInput struct {
	PlanSummary string `json:"planSummary,omitempty"`
}

func (t *ExitPlanModeTool) Definition() Definition {
	return Definition{
		Name:        "ExitPlanMode",
		Description: "Exit plan mode and return to implementation mode. Optionally provide a summary of the plan to guide implementation.",
		Parameters: map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"planSummary": map[string]interface{}{
					"type":        "string",
					"description": "Optional summary of the plan created during plan mode",
				},
			},
		},
	}
}

func (t *ExitPlanModeTool) Execute(_ context.Context, input json.RawMessage, _ ExecContext) (*Result, error) {
	if !t.Controller.IsActive() {
		return &Result{Output: "Not in plan mode."}, nil
	}

	var params exitPlanModeInput
	if input != nil {
		_ = json.Unmarshal(input, &params)
	}

	t.Controller.SetActive(false)
	t.Controller.injector.SetPlanMode(false, "")

	msg := "Exited plan mode. You can now make changes and execute tools."
	if params.PlanSummary != "" {
		msg += fmt.Sprintf("\n\nPlan summary:\n%s", params.PlanSummary)
	}
	return &Result{Output: msg}, nil
}

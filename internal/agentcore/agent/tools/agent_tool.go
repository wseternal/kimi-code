package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/visdomtech/kimi-code/internal/agentcore/agent/swarm"
)

// AgentTool allows the model to spawn individual sub-agents with custom
// configuration. Unlike AgentSwarm (batch), this tool spawns a single
// sub-agent and optionally waits for its result.
type AgentTool struct {
	Roster *swarm.Roster
}

var _ Tool = (*AgentTool)(nil)

type agentToolInput struct {
	Prompt         string   `json:"prompt"`
	Description    string   `json:"description,omitempty"`
	SubagentType   string   `json:"subagent_type,omitempty"`
	Model          string   `json:"model,omitempty"`
	WorkDir        string   `json:"work_dir,omitempty"`
	Tools          []string `json:"tools,omitempty"`
	TimeoutSec     int      `json:"timeout_sec,omitempty"`
	RunInBackground bool    `json:"run_in_background,omitempty"`
}

func (t *AgentTool) Definition() Definition {
	return Definition{
		Name:        "Agent",
		Description: "Launch a sub-agent to handle a task autonomously. The sub-agent gets its own context and can use tools independently. Use for tasks that benefit from isolated context or parallel execution.",
		Parameters: map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"prompt": map[string]interface{}{
					"type":        "string",
					"description": "The task prompt for the sub-agent.",
				},
				"description": map[string]interface{}{
					"type":        "string",
					"description": "Short description of what the sub-agent will do (for tracking).",
				},
				"subagent_type": map[string]interface{}{
					"type":        "string",
					"description": "Optional type hint for the sub-agent specialization.",
				},
				"model": map[string]interface{}{
					"type":        "string",
					"description": "Optional model override.",
				},
				"work_dir": map[string]interface{}{
					"type":        "string",
					"description": "Optional working directory override.",
				},
				"tools": map[string]interface{}{
					"type":        "array",
					"items":       map[string]interface{}{"type": "string"},
					"description": "Optional list of allowed tool names.",
				},
				"timeout_sec": map[string]interface{}{
					"type":        "integer",
					"description": "Timeout in seconds (default: no timeout).",
				},
				"run_in_background": map[string]interface{}{
					"type":        "boolean",
					"description": "If true, return immediately without waiting for the result.",
				},
			},
			"required": []string{"prompt"},
		},
	}
}

func (t *AgentTool) Execute(ctx context.Context, input json.RawMessage, exec ExecContext) (*Result, error) {
	var params agentToolInput
	if err := json.Unmarshal(input, &params); err != nil {
		return nil, err
	}
	if params.Prompt == "" {
		return &Result{Output: "Prompt is required.", IsError: true}, nil
	}

	workDir := exec.WorkDir
	if params.WorkDir != "" {
		workDir = params.WorkDir
	}

	// Background spawns must outlive the caller's context, otherwise the
	// sub-agent is aborted as soon as this turn ends.
	spawnCtx := ctx
	if params.RunInBackground {
		spawnCtx = context.Background()
	}

	cfg := swarm.SubagentConfig{
		Prompt:     params.Prompt,
		Model:      params.Model,
		WorkDir:    workDir,
		Tools:      params.Tools,
		TimeoutSec: params.TimeoutSec,
		Metadata: map[string]string{
			"description":    params.Description,
			"subagent_type":  params.SubagentType,
		},
	}

	id := t.Roster.Spawn(spawnCtx, cfg)

	if params.RunInBackground {
		return &Result{Output: fmt.Sprintf("Sub-agent %s spawned in background. Use AgentSwarm roster to check status.", id)}, nil
	}

	// Wait for the sub-agent to complete (with optional timeout)
	waitCtx := ctx
	if params.TimeoutSec > 0 {
		var cancel context.CancelFunc
		waitCtx, cancel = context.WithTimeout(ctx, time.Duration(params.TimeoutSec)*time.Second)
		defer cancel()
	}

	// Block on completion channel instead of polling (W12).
	doneCh := t.Roster.WaitDone(id)
	select {
	case <-waitCtx.Done():
		return &Result{Output: fmt.Sprintf("Sub-agent %s timed out or cancelled. Check status via roster.", id)}, nil
	case <-doneCh:
		result, ok := t.Roster.GetResult(id)
		if !ok {
			return &Result{Output: fmt.Sprintf("Sub-agent %s not found.", id), IsError: true}, nil
		}
		switch result.Status {
		case swarm.SubagentDone:
			out, _ := json.Marshal(map[string]interface{}{
				"agent_id": id,
				"status":   "done",
				"output":   result.Output,
			})
			return &Result{Output: string(out)}, nil
		case swarm.SubagentFailed:
			out, _ := json.Marshal(map[string]interface{}{
				"agent_id": id,
				"status":   "failed",
				"error":    result.Error,
			})
			return &Result{Output: string(out), IsError: true}, nil
		case swarm.SubagentAborted:
			return &Result{Output: fmt.Sprintf("Sub-agent %s was aborted.", id)}, nil
		default:
			return &Result{Output: fmt.Sprintf("Sub-agent %s in unexpected state: %s", id, result.Status)}, nil
		}
	}
}

// RegisterAgentTool registers the individual Agent tool.
func RegisterAgentTool(registry *Registry, roster *swarm.Roster) {
	registry.Register(&AgentTool{Roster: roster})
}

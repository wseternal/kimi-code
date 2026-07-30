// Package swarm implements sub-agent spawning and lifecycle management.
// The AgentSwarm tool allows the model to batch-spawn sub-agents that run
// independently with their own context, while the host agent tracks their
// progress and collects results.
package swarm

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"sync/atomic"
	"time"
)

// SubagentStatus is the lifecycle state of a sub-agent.
type SubagentStatus string

const (
	SubagentPending  SubagentStatus = "pending"
	SubagentRunning  SubagentStatus = "running"
	SubagentDone     SubagentStatus = "done"
	SubagentFailed   SubagentStatus = "failed"
	SubagentAborted  SubagentStatus = "aborted"
)

// SubagentConfig describes a sub-agent to spawn.
type SubagentConfig struct {
	ID          string            `json:"id,omitempty"`
	Prompt      string            `json:"prompt"`
	Model       string            `json:"model,omitempty"`
	WorkDir     string            `json:"work_dir,omitempty"`
	Tools       []string          `json:"tools,omitempty"`
	Metadata    map[string]string `json:"metadata,omitempty"`
	Exclusive   bool              `json:"exclusive,omitempty"` // exclusive access to workspace
	TimeoutSec  int               `json:"timeout_sec,omitempty"`
}

// SubagentResult is the outcome of a sub-agent run.
type SubagentResult struct {
	SubagentID string         `json:"subagent_id"`
	Status     SubagentStatus `json:"status"`
	Output     string         `json:"output,omitempty"`
	Error      string         `json:"error,omitempty"`
	StartedAt  time.Time      `json:"started_at"`
	EndedAt    time.Time      `json:"ended_at,omitempty"`
	TurnCount  int            `json:"turn_count"`
}

// Subagent is a tracked sub-agent instance.
type Subagent struct {
	Config    SubagentConfig `json:"config"`
	Result    SubagentResult `json:"result"`
	cancel    context.CancelFunc
}

// Roster tracks active sub-agents for a session.
type Roster struct {
	mu       sync.RWMutex
	agents   map[string]*Subagent
	nextID   atomic.Int64
	onChange func(roster *Roster)
}

// NewRoster creates a sub-agent roster.
func NewRoster(onChange func(*Roster)) *Roster {
	return &Roster{
		agents:   make(map[string]*Subagent),
		onChange: onChange,
	}
}

// Spawn creates and tracks a new sub-agent. Returns the assigned ID.
func (r *Roster) Spawn(ctx context.Context, cfg SubagentConfig) string {
	if cfg.ID == "" {
		id := r.nextID.Add(1)
		cfg.ID = fmt.Sprintf("subagent_%d", id)
	}

	var agentCtx context.Context
	var cancel context.CancelFunc
	if cfg.TimeoutSec > 0 {
		agentCtx, cancel = context.WithTimeout(ctx, time.Duration(cfg.TimeoutSec)*time.Second)
	} else {
		agentCtx, cancel = context.WithCancel(ctx)
	}

	sa := &Subagent{
		Config: cfg,
		Result: SubagentResult{
			SubagentID: cfg.ID,
			Status:     SubagentPending,
			StartedAt:  time.Now(),
		},
		cancel: cancel,
	}

	r.mu.Lock()
	r.agents[cfg.ID] = sa
	r.mu.Unlock()

	// Start the sub-agent in a goroutine
	go r.runAgent(agentCtx, sa)

	return cfg.ID
}

// runAgent simulates running a sub-agent. In production, this would wire
// to the actual agent loop with its own context, tools, and LLM calls.
func (r *Roster) runAgent(ctx context.Context, sa *Subagent) {
	r.mu.Lock()
	sa.Result.Status = SubagentRunning
	r.mu.Unlock()
	r.notifyChange()

	// Wait for context cancellation (abort/timeout) or completion signal
	<-ctx.Done()

	r.mu.Lock()
	if sa.Result.Status == SubagentRunning {
		sa.Result.Status = SubagentAborted
		sa.Result.EndedAt = time.Now()
		sa.Result.Error = ctx.Err().Error()
	}
	r.mu.Unlock()
	r.notifyChange()
}

// Abort cancels a running sub-agent.
func (r *Roster) Abort(subagentID string) bool {
	r.mu.RLock()
	sa, ok := r.agents[subagentID]
	r.mu.RUnlock()

	if !ok {
		return false
	}

	sa.cancel()
	return true
}

// AbortAll cancels all running sub-agents.
func (r *Roster) AbortAll() {
	r.mu.RLock()
	defer r.mu.RUnlock()
	for _, sa := range r.agents {
		sa.cancel()
	}
}

// GetResult returns the result for a sub-agent.
func (r *Roster) GetResult(subagentID string) (*SubagentResult, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	sa, ok := r.agents[subagentID]
	if !ok {
		return nil, false
	}
	result := sa.Result
	return &result, true
}

// List returns all tracked sub-agents.
func (r *Roster) List() []SubagentResult {
	r.mu.RLock()
	defer r.mu.RUnlock()
	results := make([]SubagentResult, 0, len(r.agents))
	for _, sa := range r.agents {
		results = append(results, sa.Result)
	}
	return results
}

// ListActive returns only running/pending sub-agents.
func (r *Roster) ListActive() []SubagentResult {
	r.mu.RLock()
	defer r.mu.RUnlock()
	var results []SubagentResult
	for _, sa := range r.agents {
		if sa.Result.Status == SubagentRunning || sa.Result.Status == SubagentPending {
			results = append(results, sa.Result)
		}
	}
	return results
}

// Count returns the total number of sub-agents.
func (r *Roster) Count() int {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return len(r.agents)
}

// ActiveCount returns the number of running/pending sub-agents.
func (r *Roster) ActiveCount() int {
	r.mu.RLock()
	defer r.mu.RUnlock()
	count := 0
	for _, sa := range r.agents {
		if sa.Result.Status == SubagentRunning || sa.Result.Status == SubagentPending {
			count++
		}
	}
	return count
}

// Complete marks a sub-agent as done with output.
func (r *Roster) Complete(subagentID, output string) {
	r.mu.Lock()
	sa, ok := r.agents[subagentID]
	if ok {
		sa.Result.Status = SubagentDone
		sa.Result.Output = output
		sa.Result.EndedAt = time.Now()
	}
	r.mu.Unlock()
	r.notifyChange()
}

// Fail marks a sub-agent as failed.
func (r *Roster) Fail(subagentID, errMsg string) {
	r.mu.Lock()
	sa, ok := r.agents[subagentID]
	if ok {
		sa.Result.Status = SubagentFailed
		sa.Result.Error = errMsg
		sa.Result.EndedAt = time.Now()
	}
	r.mu.Unlock()
	r.notifyChange()
}

func (r *Roster) notifyChange() {
	if r.onChange != nil {
		r.onChange(r)
	}
}

// ── AgentSwarm Tool ──

// AgentSwarmTool is a model-invokable tool for batch-spawning sub-agents.
type AgentSwarmTool struct {
	roster *Roster
}

// NewAgentSwarmTool creates an AgentSwarm tool.
func NewAgentSwarmTool(roster *Roster) *AgentSwarmTool {
	if roster == nil {
		roster = NewRoster(nil)
	}
	return &AgentSwarmTool{roster: roster}
}

type swarmInput struct {
	Agents []SubagentConfig `json:"agents"`
}

// Definition returns the tool definition.
func (t *AgentSwarmTool) Definition() ToolDefinition {
	return ToolDefinition{
		Name:        "AgentSwarm",
		Description: "Spawn multiple sub-agents to work on tasks in parallel. Each sub-agent gets its own context and can use tools independently.",
		Parameters: map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"agents": map[string]interface{}{
					"type": "array",
					"items": map[string]interface{}{
						"type": "object",
						"properties": map[string]interface{}{
							"prompt":      map[string]interface{}{"type": "string", "description": "The task prompt for the sub-agent"},
							"model":       map[string]interface{}{"type": "string", "description": "Model override (optional)"},
							"work_dir":    map[string]interface{}{"type": "string", "description": "Working directory override"},
							"tools":       map[string]interface{}{"type": "array", "items": map[string]interface{}{"type": "string"}},
							"timeout_sec": map[string]interface{}{"type": "integer", "description": "Timeout in seconds"},
						},
						"required": []string{"prompt"},
					},
				},
			},
			"required": []string{"agents"},
		},
	}
}

// Execute spawns the requested sub-agents.
func (t *AgentSwarmTool) Execute(ctx context.Context, input json.RawMessage) (*ToolResult, error) {
	var params swarmInput
	if err := json.Unmarshal(input, &params); err != nil {
		return nil, err
	}
	if len(params.Agents) == 0 {
		return nil, fmt.Errorf("at least one agent is required")
	}
	if len(params.Agents) > 10 {
		return nil, fmt.Errorf("maximum 10 agents per swarm")
	}

	var ids []string
	for _, cfg := range params.Agents {
		id := t.roster.Spawn(ctx, cfg)
		ids = append(ids, id)
	}

	output, _ := json.Marshal(map[string]interface{}{
		"spawned":  len(ids),
		"agent_ids": ids,
		"message":  fmt.Sprintf("Spawned %d sub-agent(s). Use SubagentStatus to check progress.", len(ids)),
	})

	return &ToolResult{Output: string(output)}, nil
}

// Roster returns the underlying roster.
func (t *AgentSwarmTool) Roster() *Roster {
	return t.roster
}

// ── Tool type aliases to avoid circular imports ──

// ToolDefinition mirrors tools.Definition for the swarm package.
type ToolDefinition struct {
	Name        string                 `json:"name"`
	Description string                 `json:"description"`
	Parameters  map[string]interface{} `json:"parameters"`
}

// ToolResult mirrors tools.Result for the swarm package.
type ToolResult struct {
	Output  string `json:"output"`
	IsError bool   `json:"isError"`
}

package protocol

import "encoding/json"

// SessionUsage tracks token and cost metrics for a session.
type SessionUsage struct {
	InputTokens         int     `json:"input_tokens"`
	OutputTokens        int     `json:"output_tokens"`
	CacheReadTokens     int     `json:"cache_read_tokens"`
	CacheCreationTokens int     `json:"cache_creation_tokens"`
	TotalCostUSD        float64 `json:"total_cost_usd"`
	ContextTokens       int     `json:"context_tokens"`
	ContextLimit        int     `json:"context_limit"`
	TurnCount           int     `json:"turn_count"`
}

// PermissionRuleMatcherKind enumerates matcher types.
type PermissionRuleMatcherKind string

const (
	MatcherCommandPrefix PermissionRuleMatcherKind = "command_prefix"
	MatcherPathGlob      PermissionRuleMatcherKind = "path_glob"
	MatcherExactInput    PermissionRuleMatcherKind = "exact_input"
	MatcherAlways        PermissionRuleMatcherKind = "always"
)

// PermissionRuleMatcher defines how to match a tool invocation.
type PermissionRuleMatcher struct {
	Kind  PermissionRuleMatcherKind `json:"kind"`
	Value string                   `json:"value,omitempty"`
}

// PermissionRuleCreatedBy enumerates who created the rule.
type PermissionRuleCreatedBy string

const (
	CreatedByUser  PermissionRuleCreatedBy = "user"
	CreatedByAgent PermissionRuleCreatedBy = "agent"
)

// PermissionRule defines an approved tool invocation pattern.
type PermissionRule struct {
	ID        string                `json:"id"`
	ToolName  string                `json:"tool_name"`
	Matcher   *PermissionRuleMatcher `json:"matcher,omitempty"`
	Decision  string                `json:"decision"` // always "approved"
	CreatedAt string                `json:"created_at"`
	CreatedBy PermissionRuleCreatedBy `json:"created_by"`
}

// Thinking config for the agent (matches prompt.ts).
type Thinking struct {
	BudgetTokens int    `json:"budget_tokens,omitempty"`
	Effort       string `json:"effort,omitempty"`
}

// PermissionMode for the agent (matches prompt.ts).
type PermissionMode string

const (
	PermissionManual PermissionMode = "manual"
	PermissionYolo   PermissionMode = "yolo"
	PermissionAuto   PermissionMode = "auto"
)

// SessionAgentConfig defines the agent configuration for a session.
type SessionAgentConfig struct {
	Model         string          `json:"model"`
	SystemPrompt  string          `json:"system_prompt,omitempty"`
	Tools         []string        `json:"tools,omitempty"`
	MCPServers    []string        `json:"mcp_servers,omitempty"`
	Thinking      *Thinking       `json:"thinking,omitempty"`
	PermissionMode PermissionMode `json:"permission_mode,omitempty"`
	PlanMode      bool            `json:"plan_mode,omitempty"`
	SwarmMode     bool            `json:"swarm_mode,omitempty"`
	GoalObjective string          `json:"goal_objective,omitempty"`
	GoalControl   string          `json:"goal_control,omitempty"` // "pause" | "resume" | "cancel"
}

// SessionMetadata is an open map with at minimum a "cwd" key.
type SessionMetadata map[string]json.RawMessage

// PendingInteraction represents the highest-priority pending human interaction.
type PendingInteraction string

const (
	PendingNone     PendingInteraction = "none"
	PendingApproval PendingInteraction = "approval"
	PendingQuestion PendingInteraction = "question"
)

// LastTurnReason is the outcome of the most recent agent turn.
type LastTurnReason string

const (
	TurnCompleted LastTurnReason = "completed"
	TurnCancelled LastTurnReason = "cancelled"
	TurnFailed    LastTurnReason = "failed"
)

// Session is the full session representation on the wire.
type Session struct {
	ID                string              `json:"id"`
	WorkspaceID       string              `json:"workspace_id"`
	Title             string              `json:"title"`
	CreatedAt         string              `json:"created_at"`
	UpdatedAt         string              `json:"updated_at"`
	Busy              bool                `json:"busy"`
	MainTurnActive    bool                `json:"main_turn_active,omitempty"`
	PendingInteraction *PendingInteraction `json:"pending_interaction,omitempty"`
	LastTurnReason    *LastTurnReason     `json:"last_turn_reason,omitempty"`
	Archived          bool                `json:"archived,omitempty"`
	CurrentPromptID   string              `json:"current_prompt_id,omitempty"`
	LastPrompt        string              `json:"last_prompt,omitempty"`
	Metadata          SessionMetadata     `json:"metadata"`
	AgentConfig       SessionAgentConfig  `json:"agent_config"`
	Usage             SessionUsage        `json:"usage"`
	PermissionRules   []PermissionRule    `json:"permission_rules"`
	MessageCount      int                 `json:"message_count"`
	LastSeq           int                 `json:"last_seq"`
}

// SessionCreate is the request body for creating a new session.
type SessionCreate struct {
	Title       string                `json:"title,omitempty"`
	Metadata    SessionMetadata       `json:"metadata,omitempty"`
	AgentConfig *SessionAgentConfig   `json:"agent_config,omitempty"`
	WorkspaceID string                `json:"workspace_id,omitempty"`
}

// SessionUpdate is the request body for updating a session.
type SessionUpdate struct {
	Title           string             `json:"title,omitempty"`
	Metadata        map[string]json.RawMessage `json:"metadata,omitempty"`
	AgentConfig     *SessionAgentConfig `json:"agent_config,omitempty"`
	PermissionRules []PermissionRule   `json:"permission_rules,omitempty"`
}

// SessionFork is the request body for forking a session.
type SessionFork struct {
	Title    string                 `json:"title,omitempty"`
	Metadata map[string]json.RawMessage `json:"metadata,omitempty"`
}

// SessionChildCreate is the request body for creating a child session.
type SessionChildCreate struct {
	Title    string                 `json:"title,omitempty"`
	Metadata map[string]json.RawMessage `json:"metadata,omitempty"`
}

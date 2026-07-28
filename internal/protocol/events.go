package protocol

import "encoding/json"

// FinishReason enumerates how an LLM response ended.
type FinishReason string

const (
	FinishCompleted  FinishReason = "completed"
	FinishToolCalls  FinishReason = "tool_calls"
	FinishTruncated  FinishReason = "truncated"
	FinishFiltered   FinishReason = "filtered"
	FinishPaused     FinishReason = "paused"
	FinishOther      FinishReason = "other"
)

// TokenUsage tracks per-model token consumption.
type TokenUsage struct {
	InputOther       int `json:"inputOther"`
	Output           int `json:"output"`
	InputCacheRead   int `json:"inputCacheRead"`
	InputCacheCreation int `json:"inputCacheCreation"`
}

// UsageStatus aggregates token usage across a session.
type UsageStatus struct {
	ByModel      map[string]TokenUsage `json:"byModel,omitempty"`
	CurrentTurn  *TokenUsage           `json:"currentTurn,omitempty"`
	Total        *TokenUsage           `json:"total,omitempty"`
}

// TaskLifecycleStatus enumerates task states.
type TaskLifecycleStatus string

const (
	TaskRunning   TaskLifecycleStatus = "running"
	TaskCompleted TaskLifecycleStatus = "completed"
	TaskFailed    TaskLifecycleStatus = "failed"
	TaskTimedOut  TaskLifecycleStatus = "timed_out"
	TaskKilled    TaskLifecycleStatus = "killed"
	TaskLost      TaskLifecycleStatus = "lost"
)

// SkillSource enumerates where a skill definition came from.
type SkillSource string

const (
	SkillProject SkillSource = "project"
	SkillUser    SkillSource = "user"
	SkillExtra   SkillSource = "extra"
	SkillBuiltin SkillSource = "builtin"
)

// GoalStatus enumerates goal lifecycle states.
type GoalStatus string

const (
	GoalActive  GoalStatus = "active"
	GoalPaused  GoalStatus = "paused"
	GoalBlocked GoalStatus = "blocked"
	GoalComplete GoalStatus = "complete"
)

// GoalSnapshot is the current state of a goal, as carried by the goal.updated event.
type GoalSnapshot struct {
	ID         string     `json:"id"`
	Status     GoalStatus `json:"status"`
	Objective  string     `json:"objective"`
	TurnBudget *int       `json:"turn_budget,omitempty"`
}

// Event is the top-level discriminated event envelope on the wire.
// The Type field selects the payload shape.
type Event struct {
	Type string `json:"type"`

	// Common fields shared by many event types
	SessionID string `json:"session_id,omitempty"`
	AgentID   string `json:"agent_id,omitempty"`
	TurnID    string `json:"turn_id,omitempty"`
	StepID    string `json:"step_id,omitempty"`

	// Raw payload — decoded per-type by the consumer
	Payload json.RawMessage `json:"payload,omitempty"`

	// Timestamp for when the event occurred
	Timestamp string `json:"timestamp,omitempty"`
}

// Common event type constants.
// The full set of ~200 event types from events.ts will be ported incrementally.
// These are the most critical ones used by the TUI and client SDK.
const (
	EventTypeSessionCreated   = "session.created"
	EventTypeSessionUpdated   = "session.updated"
	EventTypeSessionMeta      = "session.meta.updated"

	EventTypeTurnStarted      = "turn.started"
	EventTypeTurnEnded        = "turn.ended"

	EventTypeStepStarted      = "step.started"
	EventTypeStepEnded        = "step.ended"

	EventTypeAssistantDelta   = "assistant.delta"
	EventTypeAssistantMessage = "assistant.message"
	EventTypeThinkingDelta    = "thinking.delta"

	EventTypeToolCallStarted  = "tool_call.started"
	EventTypeToolCallEnded    = "tool_call.ended"
	EventTypeToolCallInput    = "tool_call.input"
	EventTypeToolCallOutput   = "tool_call.output"

	EventTypeApprovalRequested = "approval.requested"
	EventTypeApprovalResolved  = "approval.resolved"
	EventTypeQuestionRequested = "question.requested"
	EventTypeQuestionResolved  = "question.resolved"

	EventTypeTaskStarted      = "task.started"
	EventTypeTaskEnded        = "task.ended"

	EventTypeGoalUpdated      = "goal.updated"

	EventTypeConfigUpdated    = "config.updated"

	EventTypePromptQueued     = "prompt.queued"
	EventTypePromptDequeued   = "promptdequeued"

	EventTypeError            = "error"
)

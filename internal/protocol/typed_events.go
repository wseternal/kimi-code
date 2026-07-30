package protocol

import "encoding/json"

// TypedEvent is a generic typed event wrapper.
// Use this to send events with strongly typed payloads while keeping
// the wire format compatible with Event (raw JSON payload).
type TypedEvent[T any] struct {
	Type      string `json:"type"`
	SessionID string `json:"session_id,omitempty"`
	AgentID   string `json:"agent_id,omitempty"`
	TurnID    string `json:"turn_id,omitempty"`
	StepID    string `json:"step_id,omitempty"`
	Payload   T      `json:"payload"`
	Timestamp string `json:"timestamp,omitempty"`
}

// ToEvent converts a TypedEvent to the raw Event wire format.
func (e TypedEvent[T]) ToEvent() (Event, error) {
	raw, err := json.Marshal(e.Payload)
	if err != nil {
		return Event{}, err
	}
	return Event{
		Type:      e.Type,
		SessionID: e.SessionID,
		AgentID:   e.AgentID,
		TurnID:    e.TurnID,
		StepID:    e.StepID,
		Payload:   raw,
		Timestamp: e.Timestamp,
	}, nil
}

// ── Session Events ──

type SessionCreatedPayload struct {
	Session Session `json:"session"`
}

type SessionUpdatedPayload struct {
	Session Session `json:"session"`
	Changes map[string]interface{} `json:"changes,omitempty"`
}

type SessionMetaUpdatedPayload struct {
	Metadata map[string]json.RawMessage `json:"metadata"`
}

// ── Turn Events ──

type TurnStartedPayload struct {
	TurnID     string     `json:"turn_id"`
	PromptID   string     `json:"prompt_id,omitempty"`
	Prompt     string     `json:"prompt,omitempty"`
	Origin     *PromptOrigin `json:"origin,omitempty"`
	Phase      AgentPhase `json:"phase"`
}

type TurnEndedPayload struct {
	TurnID       string         `json:"turn_id"`
	Reason       LastTurnReason `json:"reason"`
	Usage        *TokenUsage    `json:"usage,omitempty"`
	MessageCount int            `json:"message_count,omitempty"`
}

// ── Step Events ──

type StepStartedPayload struct {
	StepID  string     `json:"step_id"`
	TurnID  string     `json:"turn_id"`
	Phase   AgentPhase `json:"phase"`
	Model   string     `json:"model,omitempty"`
}

type StepEndedPayload struct {
	StepID       string       `json:"step_id"`
	TurnID       string       `json:"turn_id"`
	Phase        AgentPhase   `json:"phase"`
	FinishReason FinishReason `json:"finish_reason,omitempty"`
	Usage        *TokenUsage  `json:"usage,omitempty"`
}

// ── Streaming Events ──

type AssistantDeltaPayload struct {
	Delta   string `json:"delta"`
	Full    string `json:"full,omitempty"`
	StepID  string `json:"step_id,omitempty"`
}

type AssistantMessagePayload struct {
	MessageID string           `json:"message_id"`
	Content   []MessageContent `json:"content"`
	StepID    string           `json:"step_id,omitempty"`
}

type ThinkingDeltaPayload struct {
	Delta  string `json:"delta"`
	StepID string `json:"step_id,omitempty"`
}

// ── Tool Call Events ──

type ToolCallStartedPayload struct {
	ToolCallID string `json:"tool_call_id"`
	ToolName   string `json:"tool_name"`
	StepID     string `json:"step_id,omitempty"`
	Display    *ToolInputDisplay `json:"display,omitempty"`
}

type ToolCallEndedPayload struct {
	ToolCallID string            `json:"tool_call_id"`
	ToolName   string            `json:"tool_name"`
	Result     *ToolResultDisplay `json:"result,omitempty"`
	DurationMs int               `json:"duration_ms,omitempty"`
	StepID     string            `json:"step_id,omitempty"`
}

type ToolCallInputPayload struct {
	ToolCallID string          `json:"tool_call_id"`
	ToolName   string          `json:"tool_name"`
	Delta      string          `json:"delta"`
	Full       json.RawMessage `json:"full,omitempty"`
}

type ToolCallOutputPayload struct {
	ToolCallID string `json:"tool_call_id"`
	ToolName   string `json:"tool_name"`
	Output     string `json:"output"`
	IsError    bool   `json:"is_error,omitempty"`
	Truncated  bool   `json:"truncated,omitempty"`
}

// ── Approval/Question Events ──

type ApprovalRequestedPayload struct {
	Approval ApprovalRequest `json:"approval"`
}

type ApprovalResolvedPayload struct {
	ApprovalID string         `json:"approval_id"`
	Status     ApprovalStatus `json:"status"`
	Reason     string         `json:"reason,omitempty"`
}

type QuestionRequestedPayload struct {
	Question QuestionRequest `json:"question"`
}

type QuestionResolvedPayload struct {
	QuestionID string         `json:"question_id"`
	Status     QuestionStatus `json:"status"`
}

// ── Task Events ──

type TaskStartedPayload struct {
	TaskID    string              `json:"task_id"`
	TaskName  string              `json:"task_name"`
	TaskType  string              `json:"task_type,omitempty"`
	Status    TaskLifecycleStatus `json:"status"`
}

type TaskEndedPayload struct {
	TaskID    string              `json:"task_id"`
	TaskName  string              `json:"task_name"`
	Status    TaskLifecycleStatus `json:"status"`
	Output    string              `json:"output,omitempty"`
	Error     string              `json:"error,omitempty"`
}

// ── Goal Events ──

type GoalUpdatedPayload struct {
	Goal     GoalSnapshot   `json:"goal"`
	Budget   *GoalBudget    `json:"budget,omitempty"`
}

type GoalBudget struct {
	TurnUsed     int     `json:"turn_used"`
	TurnLimit    int     `json:"turn_limit,omitempty"`
	TokensUsed   int     `json:"tokens_used"`
	TokenLimit   int     `json:"token_limit,omitempty"`
	WallClockSec float64 `json:"wall_clock_sec,omitempty"`
}

// ── Config Events ──

type ConfigUpdatedPayload struct {
	Changes map[string]interface{} `json:"changes"`
}

// ── Prompt Events ──

type PromptQueuedPayload struct {
	PromptID string        `json:"prompt_id"`
	Origin   *PromptOrigin `json:"origin,omitempty"`
}

type PromptDequeuedPayload struct {
	PromptID string `json:"prompt_id"`
}

// ── Error Event ──

type ErrorPayload struct {
	Error     *KimiErrorPayload `json:"error,omitempty"`
	Code      int               `json:"code,omitempty"`
	Message   string            `json:"message,omitempty"`
	Recoverable bool            `json:"recoverable,omitempty"`
}

// ── Compaction Event ──

type CompactionCompletedPayload struct {
	BeforeTokens int `json:"before_tokens"`
	AfterTokens  int `json:"after_tokens"`
	Strategy     string `json:"strategy,omitempty"`
}

// ── Phase Change Event ──

type PhaseChangedPayload struct {
	Transition PhaseTransition `json:"transition"`
}

// ── Additional Event Type Constants ──

const (
	EventTypeCompactionCompleted = "compaction.completed"
	EventTypePhaseChanged        = "phase.changed"
	EventTypeUsageUpdated        = "usage.updated"
)

type UsageUpdatedPayload struct {
	Usage UsageStatus `json:"usage"`
}
